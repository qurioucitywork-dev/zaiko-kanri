package web

import (
	"bytes"
	"encoding/base64"
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

func TestGuestProductImagesPostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_GUEST_IMAGES_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated database required")
	}
	if !strings.Contains(dsn, "/zaiko_guest_images_test") {
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
	guest, _ := login("G901", "image-test-password", 200)
	other, _ := login("G902", "image-test-password", 200)
	product, err := repo.CreateSingleProduct(t.Context(), persistence.SingleProductInput{OrganizationID: org, ActorUserID: admin, StaffCode: "BUY-001", SupplierCode: "S001", PurchaseDate: "2026-09-17", BrandCode: brand, CostAmountMinor: 1000, CostCurrency: "JPY", BaseSalePriceMinor: 100, BaseSaleCurrency: "USD"})
	must(err)
	must(db.Table("products").Where("id=?", product.Product.ID).Update("inventory_status", "in_stock").Error)
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aV1cAAAAASUVORK5CYII=")
	must(err)
	for _, id := range []string{"fil-guest-1", "fil-guest-2"} {
		key := "test/" + id + ".png"
		_, err = objects.Put(t.Context(), key, "image/png", bytes.NewReader(png))
		must(err)
		_, err = repo.CreateProductFile(t.Context(), org, admin, persistence.ProductFileRecord{ID: id, ProductID: product.Product.ID, StorageDriver: "local", ObjectKey: key, OriginalName: id + ".png", ContentType: "image/png", SizeBytes: int64(len(png)), SHA256: "test"})
		must(err)
	}
	_, err = repo.ReorderProductFiles(t.Context(), org, product.Product.ID, []string{"fil-guest-2", "fil-guest-1"})
	must(err)
	_, err = repo.UpdatePublicationBox(t.Context(), org, "BOX-01", admin, persistence.PublicationBoxInput{Name: "Images", IsActive: true, BuyerCodes: []string{"B901"}, ProductCodes: []string{product.Product.ProductCode}})
	must(err)
	w := request("GET", "/guest/catalog", "", guest, nil, 200)
	var catalog struct {
		Items []persistence.GuestCatalogItem `json:"items"`
	}
	must(json.Unmarshal(w.Body.Bytes(), &catalog))
	if len(catalog.Items) != 1 || len(catalog.Items[0].Images) != 2 || !strings.Contains(catalog.Items[0].Images[0], "fil-guest-2") {
		t.Fatalf("missing/reordered images: %s", w.Body.String())
	}
	// Expected-price display uses the linked guest region and live master FX, without tax.
	checkPrice := func(currency string, want *int64) {
		t.Helper()
		response := request("GET", "/guest/catalog", "", guest, nil, 200)
		var result struct {
			Items []persistence.GuestCatalogItem `json:"items"`
		}
		must(json.Unmarshal(response.Body.Bytes(), &result))
		if len(result.Items) != 1 {
			t.Fatal("missing price item")
		}
		item := result.Items[0]
		if item.DisplayCurrency != currency || (want == nil && item.DisplayPriceMinor != nil) || (want != nil && (item.DisplayPriceMinor == nil || *item.DisplayPriceMinor != *want)) {
			t.Fatalf("wrong expected price: %s", response.Body.String())
		}
	}
	setRegion := func(region string) {
		must(db.Exec(`UPDATE business_partners SET region_type=? WHERE id IN (SELECT partner_id FROM partner_roles WHERE organization_id=? AND role_code='B901')`, region, org).Error)
	}
	// No configured rate must never silently substitute a preview rate.
	must(db.Exec("DELETE FROM exchange_rate_snapshots WHERE organization_id=?", org).Error)
	checkPrice("JPY", nil)
	_, err = repo.CreateExchangeRate(t.Context(), org, admin, "USD", "161.23", "test", time.Now().Add(time.Hour))
	must(err)
	yen, dollars := int64(17000), int64(100)
	checkPrice("JPY", &yen)
	setRegion("overseas")
	checkPrice("USD", &dollars)
	must(db.Table("products").Where("id=?", product.Product.ID).Updates(map[string]any{"base_sale_price_minor": 16123, "base_sale_currency": "JPY"}).Error)
	checkPrice("USD", &dollars)
	setRegion("domestic")
	checkPrice("JPY", &yen)
	// A newly configured rate updates converted prices, but not same-currency prices.
	must(db.Table("products").Where("id=?", product.Product.ID).Updates(map[string]any{"base_sale_price_minor": 100, "base_sale_currency": "USD"}).Error)
	_, err = repo.CreateExchangeRate(t.Context(), org, admin, "USD", "172.34", "test", time.Now().Add(2*time.Hour))
	must(err)
	yen = 18000
	checkPrice("JPY", &yen)
	// Exact thousands and zero must not advance to the next thousand.
	must(db.Table("products").Where("id=?", product.Product.ID).Updates(map[string]any{"base_sale_price_minor": 16000, "base_sale_currency": "JPY"}).Error)
	yen = 16000
	checkPrice("JPY", &yen)
	must(db.Table("products").Where("id=?", product.Product.ID).Update("base_sale_price_minor", 0).Error)
	yen = 0
	checkPrice("JPY", &yen)
	imagePath := strings.TrimPrefix(catalog.Items[0].Images[0], "/api/v1")
	w = request("GET", imagePath, "", guest, nil, 200)
	if !bytes.Equal(w.Body.Bytes(), png) || w.Header().Get("Content-Type") != "image/png" || w.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("incorrect image response")
	}
	request("GET", imagePath, "", other, nil, 404)
	request("GET", imagePath, "", nil, nil, 401)
	request("GET", "/product-files/fil-guest-2", "", guest, nil, 403)
	err = repo.SetBoxProductVisibility(t.Context(), org, "BOX-01", product.Product.ProductCode, false)
	must(err)
	request("GET", imagePath, "", guest, nil, 404)
	err = repo.SetBoxProductVisibility(t.Context(), org, "BOX-01", product.Product.ProductCode, true)
	must(err)
	request("GET", imagePath, "", guest, nil, 200)
	must(db.Table("products").Where("id=?", product.Product.ID).Update("inventory_status", "reserved").Error)
	request("GET", imagePath, "", guest, nil, 404)
	must(db.Table("products").Where("id=?", product.Product.ID).Update("inventory_status", "in_stock").Error)
	request("GET", imagePath, "", guest, nil, 404)
}
