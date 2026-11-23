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

func TestGuestPurchaseBatchFlowPostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_GUEST_BATCH_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated database required")
	}
	if !strings.Contains(dsn, "/zaiko_guest_batch_test") {
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
	for _, code := range []string{"G901", "G902"} {
		request("POST", "/guest-logins", csrf, adminCookies, map[string]any{"guestCode": code, "buyerCode": "B" + code[1:], "displayName": code, "company": "Image Test", "address": "Test address", "email": code + "@example.invalid", "password": "image-test-password", "isActive": true}, 200)
	}
	guest, gcsrf := login("G901", "image-test-password", 200)
	other, ocsrf := login("G902", "image-test-password", 200)
	worker, wcsrf := login("worker", "worker-initial-test-password", 200)
	_, err = repo.CreateExchangeRate(t.Context(), org, admin, "USD", "155", "test", time.Now())
	must(err)
	products := []struct{ ID, Code string }{}
	for i := 0; i < 11; i++ {
		p, e := repo.CreateSingleProduct(t.Context(), persistence.SingleProductInput{OrganizationID: org, ActorUserID: admin, StaffCode: "BUY-001", SupplierCode: "S001", PurchaseDate: "2026-09-17", BrandCode: brand, CostAmountMinor: 1000, CostCurrency: "JPY", BaseSalePriceMinor: int64(100 + i), BaseSaleCurrency: "USD"})
		must(e)
		must(db.Table("products").Where("id=?", p.Product.ID).Update("inventory_status", "in_stock").Error)
		products = append(products, struct{ ID, Code string }{p.Product.ID, p.Product.ProductCode})
	}
	codes := []string{}
	for _, p := range products {
		codes = append(codes, p.Code)
	}
	// A shipped item in an explicitly published BOX can also be requested.
	must(db.Table("products").Where("id=?", products[1].ID).Update("inventory_status", "shipped").Error)
	_, err = repo.UpdatePublicationBox(t.Context(), org, "BOX-01", admin, persistence.PublicationBoxInput{Name: "Requests", IsActive: true, BuyerCodes: []string{"B901", "B902"}, ProductCodes: codes})
	must(err)
	status := func(index int, want string) {
		t.Helper()
		var got string
		must(db.Table("products").Select("inventory_status").Where("id=?", products[index].ID).Scan(&got).Error)
		if got != want {
			t.Fatalf("product %d: %s want %s", index, got, want)
		}
	}
	readBatch := func(w *httptest.ResponseRecorder) persistence.PurchaseRequestRecord {
		t.Helper()
		var b persistence.PurchaseRequestRecord
		must(json.Unmarshal(w.Body.Bytes(), &b))
		return b
	}
	submit := func(indexes []int) persistence.PurchaseRequestRecord {
		t.Helper()
		ids := []string{}
		for _, i := range indexes {
			ids = append(ids, products[i].ID)
		}
		return readBatch(request("POST", "/guest/purchase-requests", gcsrf, guest, map[string]any{"productIds": ids, "message": "one cart"}, 201))
	}
	first := submit([]int{0, 1, 2})
	if len(first.Items) != 3 || first.BatchID == "" {
		t.Fatalf("not grouped: %+v", first)
	}
	for i := 0; i < 3; i++ {
		status(i, "reserved")
	}
	var n int64
	must(db.Table("notifications").Where("target_id=? AND event_key='purchase_request.created'", first.ID).Count(&n).Error)
	if n != 1 {
		t.Fatalf("notifications=%d", n)
	}
	request("POST", "/guest/purchase-requests", ocsrf, other, map[string]any{"productIds": []string{products[3].ID, products[0].ID}}, 409)
	status(3, "in_stock")
	request("POST", "/guest/purchase-requests", gcsrf, guest, map[string]any{"productIds": []string{products[3].ID, products[3].ID}}, 409)
	status(3, "in_stock")
	var list struct {
		Items []persistence.PurchaseRequestRecord `json:"items"`
	}
	must(json.Unmarshal(request("GET", "/purchase-requests", csrf, adminCookies, nil, 200).Body.Bytes(), &list))
	if len(list.Items) != 1 || len(list.Items[0].Items) != 3 {
		t.Fatal("admin grouping")
	}
	must(json.Unmarshal(request("GET", "/guest/purchase-requests", "", other, nil, 200).Body.Bytes(), &list))
	if len(list.Items) != 0 {
		t.Fatal("history leaked")
	}
	decisions := []persistence.PurchaseRequestDecision{}
	approvedCodes := map[string]bool{}
	for i, line := range first.Items {
		decision := "approved"
		if i == 2 {
			decision = "rejected"
		} else {
			approvedCodes[line.ProductCode] = true
		}
		decisions = append(decisions, persistence.PurchaseRequestDecision{ID: line.ID, Decision: decision})
	}
	must(db.Exec("UPDATE products SET base_sale_price_minor=9999 WHERE id IN ?", []string{products[0].ID, products[1].ID, products[2].ID}).Error)
	endpoint := "/purchase-requests/" + first.ID + "/decide"
	request("POST", endpoint, gcsrf, guest, map[string]any{"decisions": decisions}, 403)
	request("POST", endpoint, wcsrf, worker, map[string]any{"decisions": decisions}, 403)
	request("POST", endpoint, wcsrf, worker, map[string]any{"decisions": decisions, "adminCode": "WRONG1"}, 403)
	request("POST", endpoint, csrf, adminCookies, map[string]any{"decisions": decisions[:2]}, 409)
	request("POST", "/purchase-requests/"+first.Items[0].ID+"/approved", csrf, adminCookies, map[string]any{"note": "bypass"}, 409)
	for i := 0; i < 3; i++ {
		status(i, "reserved")
	}
	code, e := repo.AdminAccessCode(t.Context(), org, admin)
	must(e)
	// Pricing failure rolls back decisions, reservations and the draft shipment together.
	must(db.Exec("DELETE FROM exchange_rate_snapshots WHERE organization_id=? AND base_currency='USD'", org).Error)
	request("POST", endpoint, wcsrf, worker, map[string]any{"decisions": decisions, "adminCode": code.Code}, 409)
	for i := 0; i < 3; i++ {
		status(i, "reserved")
	}
	must(db.Table("shipment_slips").Count(&n).Error)
	if n != 0 {
		t.Fatal("partial shipment retained")
	}
	_, err = repo.CreateExchangeRate(t.Context(), org, admin, "USD", "155", "test", time.Now())
	must(err)
	reviewed := readBatch(request("POST", endpoint, wcsrf, worker, map[string]any{"decisions": decisions, "adminCode": code.Code}, 200))
	if reviewed.ShipmentSlipID == "" || len(reviewed.Items) != 3 {
		t.Fatal("missing shipment")
	}
	shipment, e := repo.ShipmentSlip(t.Context(), org, reviewed.ShipmentSlipID)
	must(e)
	if shipment.Status != "confirmed" || shipment.ProcessingStatus != "processing" || len(shipment.Lines) != 2 {
		t.Fatalf("bad shipment: %+v", shipment)
	}
	for _, line := range shipment.Lines {
		for _, original := range first.Items {
			if line.ProductCode == original.ProductCode && line.SalePriceUSDMinor != *original.RequestedPriceMinor {
				t.Fatal("request price snapshot changed")
			}
		}
	}
	for _, line := range shipment.Lines {
		if !line.ShipmentPending || !approvedCodes[line.ProductCode] {
			t.Fatal("unapproved product shipped")
		}
	}
	for i := 0; i < 3; i++ {
		want := "in_stock"
		if approvedCodes[products[i].Code] {
			want = "shipped"
		}
		status(i, want)
	}
	request("POST", endpoint, wcsrf, worker, map[string]any{"decisions": decisions, "adminCode": code.Code}, 409)
	must(db.Table("shipment_slips").Count(&n).Error)
	if n != 1 {
		t.Fatal("duplicate shipment")
	}
	// Approved products can be used by the existing sales workflow.
	saleLines := []persistence.SaleLineInput{}
	for _, line := range shipment.Lines {
		saleLines = append(saleLines, persistence.SaleLineInput{ProductCode: line.ProductCode, UnitPriceMinor: line.SalePriceUSDMinor})
	}
	sale, e := repo.CreateSale(t.Context(), persistence.SaleCreateInput{OrganizationID: org, ActorUserID: admin, BuyerCode: "B901", SaleDate: "2026-09-17", DisplayCurrency: "USD", TaxMode: "tax_exempt", Lines: saleLines})
	must(e)
	if len(sale.Lines) != 2 {
		t.Fatal("sale lines missing")
	}
	// Confirmed, unpaid USD sales remain editable through the document revision API.
	sale, e = repo.ConfirmSale(t.Context(), org, sale.ID, admin)
	must(e)
	revisedLines := []persistence.SaleRevisionLine{}
	for _, line := range sale.Lines {
		revisedLines = append(revisedLines, persistence.SaleRevisionLine{ID: line.ID, UnitPriceMinor: 30000})
	}
	// Legacy deployed schemas contain nullable product comments.
	must(db.Exec("ALTER TABLE products ALTER COLUMN internal_comment DROP NOT NULL").Error)
	must(db.Table("products").Where("id=?", sale.Lines[0].ProductID).Update("internal_comment", nil).Error)
	revision := persistence.SaleRevisionInput{SaleDate: "2026-09-18", BuyerCode: "B901", Notes: sale.Notes, Reason: "1000ドル引き", Lines: revisedLines}
	request("PATCH", "/sales/"+sale.ID+"/revision", csrf, adminCookies, revision, 200)
	revised, e := repo.SaleSlip(t.Context(), org, sale.ID)
	must(e)
	if revised.SubtotalMinor != 60000 || revised.ConvertedTotalJPY != 9300000 || !strings.Contains(revised.Notes, "1000ドル引き") {
		t.Fatalf("revision not persisted: %+v", revised)
	}
	for _, line := range revised.Lines {
		if line.UnitPriceMinor != 30000 || line.FXRateScaled != sale.Lines[0].FXRateScaled || line.FXScale != sale.Lines[0].FXScale {
			t.Fatal("amount or FX incorrect")
		}
		var comment string
		must(db.Table("products").Select("internal_comment").Where("id=?", line.ProductID).Scan(&comment).Error)
		if !strings.Contains(comment, "1000ドル引き") {
			t.Fatal("revision reason missing")
		}
	}
	request("PATCH", "/sales/"+sale.ID+"/revision", wcsrf, worker, revision, 403)
	_, e = repo.MarkSalePaid(t.Context(), org, sale.ID, admin)
	must(e)
	revision.Lines[0].UnitPriceMinor = 29000
	request("PATCH", "/sales/"+sale.ID+"/revision", csrf, adminCookies, revision, 409)
	afterPaid, e := repo.SaleSlip(t.Context(), org, sale.ID)
	must(e)
	if afterPaid.SubtotalMinor != 60000 {
		t.Fatal("paid sale changed")
	}

	for _, scenario := range []struct {
		indexes  []int
		decision string
	}{{[]int{3, 4}, "approved"}, {[]int{5, 6}, "rejected"}} {
		b := submit(scenario.indexes)
		ds := []persistence.PurchaseRequestDecision{}
		for _, line := range b.Items {
			ds = append(ds, persistence.PurchaseRequestDecision{ID: line.ID, Decision: scenario.decision})
		}
		done := readBatch(request("POST", "/purchase-requests/"+b.ID+"/decide", csrf, adminCookies, map[string]any{"decisions": ds}, 200))
		if scenario.decision == "approved" && done.ShipmentSlipID == "" {
			t.Fatal("bulk approval missing shipment")
		}
		if scenario.decision == "rejected" && done.ShipmentSlipID != "" {
			t.Fatal("rejected shipment")
		}
		for _, i := range scenario.indexes {
			want := "in_stock"
			if scenario.decision == "approved" {
				want = "shipped"
			}
			status(i, want)
		}
	}
	must(json.Unmarshal(request("GET", "/guest/purchase-requests", "", guest, nil, 200).Body.Bytes(), &list))
	if len(list.Items) != 3 {
		t.Fatalf("grouped history count %d", len(list.Items))
	}
	request("POST", "/guest/purchase-requests", gcsrf, guest, map[string]any{"productIds": []string{products[7].ID, "not-visible"}}, 409)
	status(7, "in_stock")
	// Competing carts for the same item: one transaction wins, the other rolls back.
	var guestUsers []string
	must(db.Table("guest_accounts").Select("user_id").Where("organization_id=? AND guest_code IN ?", org, []string{"G901", "G902"}).Scan(&guestUsers).Error)
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, u := range guestUsers {
		go func(user string) {
			<-start
			_, e := repo.CreateGuestPurchaseBatch(t.Context(), org, user, []string{products[8].ID}, "concurrent")
			results <- e
		}(u)
	}
	close(start)
	wins := 0
	for i := 0; i < 2; i++ {
		if <-results == nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("concurrent reservations succeeded %d times", wins)
	}
	status(8, "reserved")

	// A zero-price request can be approved alongside rejection, preserving the snapshot.
	must(db.Table("products").Where("id=?", products[9].ID).Update("base_sale_price_minor", 0).Error)
	zeroBatch := submit([]int{9, 10})
	must(db.Table("products").Where("id=?", products[9].ID).Update("base_sale_price_minor", 9999).Error)
	zeroDecisions := []persistence.PurchaseRequestDecision{}
	for _, line := range zeroBatch.Items {
		decision := "rejected"
		if line.ProductCode == products[9].Code {
			decision = "approved"
		}
		zeroDecisions = append(zeroDecisions, persistence.PurchaseRequestDecision{ID: line.ID, Decision: decision})
	}
	zeroEndpoint := "/purchase-requests/" + zeroBatch.ID + "/decide"
	zeroDone := readBatch(request("POST", zeroEndpoint, csrf, adminCookies, map[string]any{"decisions": zeroDecisions}, 200))
	zeroShipment, e := repo.ShipmentSlip(t.Context(), org, zeroDone.ShipmentSlipID)
	must(e)
	if zeroShipment.Status != "confirmed" || len(zeroShipment.Lines) != 1 || zeroShipment.Lines[0].SalePriceUSDMinor != 0 || zeroShipment.Lines[0].ConvertedSalePriceJPY != 0 {
		t.Fatalf("zero snapshot lost: %+v", zeroShipment)
	}
	status(9, "shipped")
	status(10, "in_stock")
	request("POST", zeroEndpoint, csrf, adminCookies, map[string]any{"decisions": zeroDecisions}, 409)
	must(db.Table("shipment_slips").Where("id=?", zeroDone.ShipmentSlipID).Count(&n).Error)
	if n != 1 {
		t.Fatal("duplicate zero-price shipment")
	}

}
