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

func TestWorkerTakebackPostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_WORKER_TAKEBACK_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated database required")
	}
	if !strings.Contains(dsn, "/zaiko_worker_takeback_test") {
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

	p := product("in_stock")
	shipment, err := repo.CreateShipment(t.Context(), persistence.ShipmentCreateInput{OrganizationID: org, ActorUserID: admin, BuyerCode: "B001", ShipmentDate: "2026-09-18", DisplayCurrency: "USD", ProductCodes: []string{p.Product.ProductCode}})
	must(err)
	path := "/shipments/" + shipment.ID + "/return-scan"
	body := map[string]string{"code": p.Product.ProductCode}
	request("POST", path, wcsrf, worker, body, 409) // Draft shipments remain protected.
	_, err = repo.ConfirmShipment(t.Context(), org, shipment.ID, admin)
	must(err)
	request("POST", path, "", nil, body, 401)
	request("POST", path, "", worker, body, 403)
	request("POST", path, wcsrf, worker, map[string]string{"code": "UNKNOWN"}, 404)
	request("POST", path, wcsrf, worker, body, 200)
	var status, actor string
	must(db.Table("products").Select("inventory_status").Where("id=?", p.Product.ID).Scan(&status).Error)
	if status != "in_stock" {
		t.Fatal("worker takeback did not restore stock")
	}
	must(db.Table("inventory_events").Select("actor_user_id").Where("product_id=? AND event_type='shipment_return_scanned'", p.Product.ID).Scan(&actor).Error)
	if actor != workerID {
		t.Fatal("worker audit identity lost")
	}
	request("POST", path, wcsrf, worker, body, 200) // Repeated scan is idempotent.
	var count int64
	must(db.Table("inventory_events").Where("product_id=? AND event_type='shipment_return_scanned'", p.Product.ID).Count(&count).Error)
	if count != 1 {
		t.Fatal("duplicate inventory event")
	}
	must(db.Table("approval_requests").Count(&count).Error)
	if count != 0 {
		t.Fatal("takeback must not create an approval")
	}
	request("POST", "/shipments/"+shipment.ID+"/confirm", wcsrf, worker, map[string]string{}, 403)
	request("POST", "/guest-logins", csrf, adminCookies, map[string]any{"guestCode": "G901", "buyerCode": "B901", "displayName": "Test", "company": "Test", "address": "Test", "email": "test@example.invalid", "password": "guest-test-password", "isActive": true}, 200)
	guest, gcsrf := login("G901", "guest-test-password", 200)
	request("POST", path, gcsrf, guest, body, 403)
	request("POST", path, csrf, adminCookies, body, 200)
}
