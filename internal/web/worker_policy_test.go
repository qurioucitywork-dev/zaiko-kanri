package web

import (
	"encoding/json"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/persistence"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/storage"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestWorkerPolicyPostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_WORKER_POLICY_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated database required")
	}
	if !strings.Contains(dsn, "/zaiko_worker_policy_test") {
		t.Fatal("non-test database rejected")
	}
	cfg := config.Config{Environment: "test", DatabaseDriver: "postgres", DatabaseURL: dsn, OrganizationCode: "PREVIEW", SessionTTL: time.Hour, StorageDriver: "local", UploadDirectory: t.TempDir()}
	repo, err := persistence.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	must(repo.Migrate(t.Context()))
	must(repo.SeedPreviewIdentity(t.Context(), "worker-admin-test-password", "worker-initial-test-password"))
	must(repo.SeedPreviewMasters(t.Context()))
	objects, err := storage.New(t.Context(), cfg)
	must(err)
	app, err := New(cfg, nil, repo, objects, slog.New(slog.NewTextHandler(io.Discard, nil)))
	must(err)
	page, pageCode := "", ""
	request := func(method, path, csrf string, cookies []*http.Cookie, body any, want int) *httptest.ResponseRecorder {
		t.Helper()
		payload, _ := json.Marshal(body)
		r := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(string(payload)))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", csrf)
		r.Header.Set("X-Admin-Page", page)
		r.Header.Set("X-Admin-Page-Code", pageCode)
		for _, c := range cookies {
			r.AddCookie(c)
		}
		w := httptest.NewRecorder()
		app.Handler().ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d want %d: %s", method, path, w.Code, want, w.Body.String())
		}
		return w
	}
	login := func(username, password string, want int) ([]*http.Cookie, string) {
		t.Helper()
		w := request("POST", "/auth/login", "", nil, map[string]string{"username": username, "password": password}, want)
		var p struct {
			CSRFToken string `json:"csrfToken"`
		}
		must(json.Unmarshal(w.Body.Bytes(), &p))
		return w.Result().Cookies(), p.CSRFToken
	}

	adminCookies, csrf := login("admin", "worker-admin-test-password", 200)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	must(err)
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	var org, admin, brand string
	must(db.Raw("SELECT id FROM organizations WHERE code='PREVIEW'").Scan(&org).Error)
	must(db.Raw("SELECT id FROM users WHERE username='admin'").Scan(&admin).Error)
	must(db.Raw("SELECT code FROM brands ORDER BY code LIMIT 1").Scan(&brand).Error)

	worker, wcsrf := login("worker", "worker-initial-test-password", 200)
	var workerID string
	must(db.Raw("SELECT id FROM users WHERE username='worker'").Scan(&workerID).Error)
	_, err = repo.CreateExchangeRate(t.Context(), org, admin, "USD", "155", "test", time.Now())
	must(err)
	product := func(status string) persistence.SingleProductResult {
		p, e := repo.CreateSingleProduct(t.Context(), persistence.SingleProductInput{OrganizationID: org, ActorUserID: admin, StaffCode: "BUY-001", SupplierCode: "S001", PurchaseDate: "2026-09-18", BrandCode: brand, CostAmountMinor: 1000, CostCurrency: "JPY", BaseSalePriceMinor: 100, BaseSaleCurrency: "USD"})
		must(e)
		must(db.Table("products").Where("id=?", p.Product.ID).Update("inventory_status", status).Error)
		return p
	}

	code, e := repo.AdminAccessCode(t.Context(), org, admin)
	must(e)
	// PW is checked for each scoped write; it never grants approval authority.
	request("POST", "/masters/materials", wcsrf, worker, map[string]any{"code": "9998", "name": "Test"}, 403)
	page, pageCode = "master", "WRONG1"
	request("POST", "/masters/materials", wcsrf, worker, map[string]any{"code": "9998", "name": "Test"}, 403)
	pageCode = code.Code
	request("POST", "/masters/materials", wcsrf, worker, map[string]any{"code": "9998", "name": "Test"}, 201)
	request("POST", "/approvals/not-an-id/approved", wcsrf, worker, map[string]any{}, 403)
	request("GET", "/auth/me", wcsrf, worker, nil, 200)
	me := request("GET", "/auth/me", wcsrf, worker, nil, 200)
	if !strings.Contains(me.Body.String(), `"role":"worker"`) {
		t.Fatal("role changed")
	}
	page, pageCode = "", ""
	// A proposal must leave stock, tracking and the document untouched until approval.
	p := product("in_stock")
	shipment, e := repo.CreateShipment(t.Context(), persistence.ShipmentCreateInput{OrganizationID: org, ActorUserID: admin, BuyerCode: "B001", ShipmentDate: "2026-09-18", DisplayCurrency: "USD", ProductCodes: []string{p.Product.ProductCode}})
	must(e)
	_, e = repo.ConfirmShipment(t.Context(), org, shipment.ID, admin)
	must(e)
	input := map[string]any{"kind": "shipment.tracking", "targetId": shipment.ID, "payload": map[string]any{"carrier": "Test carrier", "trackingNumber": "TRACK-1"}}
	request("PATCH", "/shipments/"+shipment.ID+"/tracking", wcsrf, worker, input["payload"], 403)
	w := request("POST", "/document-change-requests", wcsrf, worker, input, 202)
	var response struct {
		Approval persistence.ApprovalRequestRecord
	}
	must(json.Unmarshal(w.Body.Bytes(), &response))
	before, e := repo.ShipmentSlip(t.Context(), org, shipment.ID)
	must(e)
	if before.TrackingNumber != "" {
		t.Fatal("proposal changed tracking")
	}
	request("POST", "/document-change-requests", wcsrf, worker, input, 409)
	request("POST", "/approvals/"+response.Approval.ID+"/rejected", csrf, adminCookies, map[string]string{"note": "訂正"}, 200)
	rejected, e := repo.ApprovalRequest(t.Context(), org, response.Approval.ID)
	must(e)
	input["approvalId"] = rejected.ID
	input["expectedUpdatedAt"] = rejected.UpdatedAt
	input["payload"] = map[string]any{"carrier": "Updated carrier", "trackingNumber": "TRACK-2"}
	request("POST", "/document-change-requests", wcsrf, worker, input, 202)
	request("POST", "/approvals/"+rejected.ID+"/approved", csrf, adminCookies, map[string]string{}, 200)
	after, e := repo.ShipmentSlip(t.Context(), org, shipment.ID)
	must(e)
	if after.TrackingNumber != "TRACK-2" {
		t.Fatal("approval did not apply updated proposal")
	}
	request("POST", "/approvals/"+rejected.ID+"/approved", csrf, adminCookies, map[string]string{}, 409)
	// Rejected consignment creation can be edited and resubmitted by its requester.
	p = product("in_stock")
	w = request("POST", "/consignments", wcsrf, worker, map[string]any{"consigneeCode": "B001", "consignmentDate": "2026-09-18", "productCodes": []string{p.Product.ProductCode}}, 201)
	var consignment persistence.ConsignmentSlipRecord
	must(json.Unmarshal(w.Body.Bytes(), &consignment))
	var aid string
	must(db.Table("approval_requests").Select("id").Where("target_id=?", consignment.ID).Scan(&aid).Error)
	request("POST", "/approvals/"+aid+"/rejected", csrf, adminCookies, map[string]string{"note": "価格修正"}, 200)
	d, e := repo.ReturnedApprovalDocument(t.Context(), org, workerID, aid)
	must(e)
	consignment, e = repo.ConsignmentSlip(t.Context(), org, consignment.ID)
	must(e)
	if consignment.Status != "draft" {
		t.Fatal("rejected document cannot be edited")
	}
	resubmit := persistence.ApprovalResubmitInput{ExpectedApprovalUpdatedAt: d.Approval.UpdatedAt, ExpectedDocumentUpdatedAt: consignment.UpdatedAt, Consignment: &persistence.ConsignmentRevisionInput{ConsigneeCode: "B001", ConsignmentDate: "2026-09-18", Notes: "修正", Reason: "再申請", Lines: []persistence.ConsignmentRevisionLine{{ID: consignment.Lines[0].ID, UnitPriceMinor: 16000}}}}
	_, e = repo.ResubmitApproval(t.Context(), org, workerID, aid, resubmit)
	must(e)
	request("POST", "/approvals/"+aid+"/approved", csrf, adminCookies, map[string]string{}, 200)
	consignment, e = repo.ConsignmentSlip(t.Context(), org, consignment.ID)
	must(e)
	if consignment.Status != "confirmed" {
		t.Fatal("resubmitted consignment not confirmed")
	}
	// A stale approval must fail without recording an approved decision.
	delete(input, "approvalId")
	delete(input, "expectedUpdatedAt")
	input["payload"] = map[string]any{"carrier": "Test", "trackingNumber": "TRACK-3"}
	w = request("POST", "/document-change-requests", wcsrf, worker, input, 202)
	must(json.Unmarshal(w.Body.Bytes(), &response))
	_, e = repo.UpdateShipmentTracking(t.Context(), org, shipment.ID, "Other", "LATEST")
	must(e)
	request("POST", "/approvals/"+response.Approval.ID+"/approved", csrf, adminCookies, map[string]string{}, 409)
	a, e := repo.ApprovalRequest(t.Context(), org, response.Approval.ID)
	must(e)
	if a.Status != "pending" {
		t.Fatal("failed approval committed")
	}
	// Shipment takeback remains immediate and approval-free.
	var countBefore, countAfter int64
	must(db.Table("approval_requests").Count(&countBefore).Error)
	request("POST", "/shipments/"+shipment.ID+"/return-scan", wcsrf, worker, map[string]string{"code": before.Lines[0].ProductCode}, 200)
	must(db.Table("approval_requests").Count(&countAfter).Error)
	if countBefore != countAfter {
		t.Fatal("takeback requested approval")
	}

	// Sales price, payment and issuance all remain unchanged until the administrator approves.
	sp := product("in_stock")
	sale, e := repo.CreateSale(t.Context(), persistence.SaleCreateInput{OrganizationID: org, ActorUserID: admin, BuyerCode: "B001", SaleDate: "2026-09-18", DisplayCurrency: "USD", TaxMode: "tax_free", Lines: []persistence.SaleLineInput{{ProductCode: sp.Product.ProductCode, UnitPriceMinor: 100}}})
	must(e)
	sale, e = repo.ConfirmSale(t.Context(), org, sale.ID, admin)
	must(e)
	propose := func(kind string, payload any) persistence.ApprovalRequestRecord {
		t.Helper()
		w := request("POST", "/document-change-requests", wcsrf, worker, map[string]any{"kind": kind, "targetId": sale.ID, "payload": payload}, 202)
		var v struct {
			Approval persistence.ApprovalRequestRecord
		}
		must(json.Unmarshal(w.Body.Bytes(), &v))
		return v.Approval
	}
	a = propose("sale.revision", map[string]any{"saleDate": "2026-09-18", "buyerCode": "B001", "notes": "修正内容", "reason": "価格訂正", "lines": []map[string]any{{"id": sale.Lines[0].ID, "unitPriceMinor": 120}}})
	savedSale, e := repo.SaleSlip(t.Context(), org, sale.ID)
	must(e)
	if savedSale.Lines[0].UnitPriceMinor != 100 {
		t.Fatal("revision applied before approval")
	}
	request("POST", "/approvals/"+a.ID+"/approved", csrf, adminCookies, map[string]string{}, 200)
	savedSale, e = repo.SaleSlip(t.Context(), org, sale.ID)
	must(e)
	if savedSale.Lines[0].UnitPriceMinor != 120 {
		t.Fatal("revision not applied")
	}
	a = propose("sale.paid", map[string]any{})
	request("POST", "/approvals/"+a.ID+"/approved", csrf, adminCookies, map[string]string{}, 200)
	a = propose("sale.issue", map[string]any{})
	request("GET", "/approvals/"+a.ID+"/document", wcsrf, worker, nil, 409)
	request("POST", "/approvals/"+a.ID+"/approved", csrf, adminCookies, map[string]string{}, 200)
	w = request("GET", "/approvals/"+a.ID+"/document", wcsrf, worker, nil, 200)
	var ref persistence.OfficialDocumentRef
	must(json.Unmarshal(w.Body.Bytes(), &ref))
	if ref.DownloadURL == "" {
		t.Fatal("approved PDF missing")
	}
	pdf := request("GET", strings.TrimPrefix(ref.DownloadURL, "/api/v1"), wcsrf, worker, nil, 200)
	if !strings.HasPrefix(pdf.Body.String(), "%PDF") {
		t.Fatal("invalid PDF")
	}
}
