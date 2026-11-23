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

func TestWorkerConsignmentApprovalPostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_CONSIGNMENT_APPROVAL_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated database required")
	}
	if !strings.Contains(dsn, "/zaiko_consignment_approval_test") {
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
	request := func(method, path, csrf string, cookies []*http.Cookie, body any, want int) *httptest.ResponseRecorder {
		t.Helper()
		payload, _ := json.Marshal(body)
		r := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(string(payload)))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", csrf)
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

	for _, decision := range []string{"approved", "rejected", "returned", "conflict"} {
		p := product("in_stock")
		body := map[string]any{"consigneeCode": "B001", "consignmentDate": "2026-09-18", "productCodes": []string{p.Product.ProductCode}}
		w := request("POST", "/consignments", wcsrf, worker, body, 201)
		var slip persistence.ConsignmentSlipRecord
		must(json.Unmarshal(w.Body.Bytes(), &slip))
		if slip.Status != "draft" || !slip.ConfirmedAt.IsZero() {
			t.Fatal("worker confirmed early")
		}
		var status string
		must(db.Table("products").Select("inventory_status").Where("id=?", p.Product.ID).Scan(&status).Error)
		if status != "in_stock" {
			t.Fatal("inventory changed before approval")
		}
		var aid string
		must(db.Table("approval_requests").Select("id").Where("target_id=? AND requested_by=? AND status='pending'", slip.ID, workerID).Scan(&aid).Error)
		if aid == "" {
			t.Fatal("approval not created")
		}
		approval, e := repo.ApprovalRequest(t.Context(), org, aid)
		must(e)
		if !strings.Contains(approval.SnapshotJSON, slip.SlipNumber) || !strings.Contains(approval.SnapshotJSON, p.Product.ProductCode) {
			t.Fatal("snapshot missing details")
		}
		var count int64
		must(db.Table("notifications").Where("target_id=? AND event_key='approval.requested'", aid).Count(&count).Error)
		if count != 1 {
			t.Fatal("admin notification missing")
		}
		request("POST", "/consignments/"+slip.ID+"/issue", csrf, adminCookies, map[string]any{}, 404)
		request("POST", "/approvals/"+aid+"/approved", wcsrf, worker, map[string]string{}, 403)
		expectedStatus, wantProduct := "draft", "in_stock"
		if decision == "conflict" {
			must(db.Table("products").Where("id=?", p.Product.ID).Update("inventory_status", "shipped").Error)
			request("POST", "/approvals/"+aid+"/approved", csrf, adminCookies, map[string]string{}, 409)
			wantProduct = "shipped"
			approval, e = repo.ApprovalRequest(t.Context(), org, aid)
			must(e)
			if approval.Status != "pending" {
				t.Fatal("failed approval mutated")
			}
		} else {
			request("POST", "/approvals/"+aid+"/"+decision, csrf, adminCookies, map[string]string{"note": "test"}, 200)
			if decision == "approved" {
				expectedStatus, wantProduct = "confirmed", "consigned"
			}
			if decision == "rejected" {
				expectedStatus = "draft"
			}
			request("POST", "/approvals/"+aid+"/approved", csrf, adminCookies, map[string]string{}, 409)
		}
		saved, e := repo.ConsignmentSlip(t.Context(), org, slip.ID)
		must(e)
		if saved.Status != expectedStatus {
			t.Fatalf("state %s", saved.Status)
		}
		must(db.Table("products").Select("inventory_status").Where("id=?", p.Product.ID).Scan(&status).Error)
		if status != wantProduct {
			t.Fatalf("product %s", status)
		}
		if decision == "approved" {
			var actor string
			must(db.Table("inventory_events").Select("actor_user_id").Where("product_id=? AND event_type='consignment_registered'", p.Product.ID).Scan(&actor).Error)
			if actor != admin {
				t.Fatal("wrong confirming actor")
			}
		}
	}
	p := product("in_stock")
	w := request("POST", "/consignments", csrf, adminCookies, map[string]any{"consigneeCode": "B001", "consignmentDate": "2026-09-18", "productCodes": []string{p.Product.ProductCode}}, 201)
	var direct persistence.ConsignmentSlipRecord
	must(json.Unmarshal(w.Body.Bytes(), &direct))
	if direct.Status != "confirmed" {
		t.Fatal("admin immediate confirmation changed")
	}
}
