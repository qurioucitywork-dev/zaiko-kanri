package persistence

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestGuestCatalogHidesPurchasingUntilReceived(t *testing.T) {
	dsn := os.Getenv("ZAIKO_GUEST_VISIBILITY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires disposable guest-visibility PostgreSQL database")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(parsed.Path, "/zaiko_clean_test_") {
		t.Fatal("refusing non-test database")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	for _, sql := range []string{
		`CREATE TABLE product_files(id TEXT,organization_id TEXT,product_id TEXT,sha256 TEXT,sort_order INTEGER,content_type TEXT)`,
		`CREATE TABLE publication_boxes(id TEXT PRIMARY KEY,organization_id TEXT,box_code TEXT,name TEXT,is_active BOOLEAN,created_by TEXT,updated_by TEXT,created_at TIMESTAMPTZ,updated_at TIMESTAMPTZ,UNIQUE(organization_id,box_code))`,
		`CREATE TABLE publication_box_buyers(box_id TEXT,buyer_role_id TEXT,created_at TIMESTAMPTZ,PRIMARY KEY(box_id,buyer_role_id))`,
		`CREATE TABLE publication_box_products(box_id TEXT,product_id TEXT,created_at TIMESTAMPTZ,PRIMARY KEY(box_id,product_id))`,
		`CREATE TABLE partner_roles(id TEXT PRIMARY KEY,organization_id TEXT,partner_id TEXT,role_type TEXT,role_code TEXT,is_active BOOLEAN)`,
		`CREATE TABLE business_partners(id TEXT,organization_id TEXT,legal_name TEXT)`,
		`CREATE TABLE guest_accounts(id TEXT,organization_id TEXT,user_id TEXT,guest_code TEXT,buyer_role_id TEXT,status TEXT)`,
		`CREATE TABLE products(id TEXT PRIMARY KEY,organization_id TEXT,product_code TEXT,brand TEXT,model_number TEXT,reference_number TEXT,serial_number TEXT,condition_text TEXT,accessories TEXT,base_sale_price_minor BIGINT,base_sale_currency TEXT,inventory_status TEXT,purchase_date DATE,publication_status TEXT,purchase_slip_line_id TEXT,cost_adjustment_id TEXT,deleted_at TIMESTAMPTZ,updated_at TIMESTAMPTZ)`,
		`CREATE TABLE purchase_slips(id TEXT,organization_id TEXT,status TEXT)`,
		`CREATE TABLE purchase_slip_lines(id TEXT,purchase_slip_id TEXT)`,
		`CREATE TABLE inventory_events(id TEXT,organization_id TEXT,product_id TEXT,event_type TEXT,from_status TEXT,to_status TEXT,reason TEXT,actor_user_id TEXT,created_at TIMESTAMPTZ)`,
		`CREATE TABLE purchase_requests(id TEXT,organization_id TEXT,product_id TEXT,buyer_role_id TEXT,guest_account_id TEXT,status TEXT,reservation_expires_at TIMESTAMPTZ)`,
		`INSERT INTO partner_roles VALUES('buyer','org','partner','buyer','0001',TRUE),('other-buyer','org','other-partner','buyer','0002',TRUE)`,
		`INSERT INTO business_partners VALUES('partner','org','Company'),('other-partner','org','Other company')`,
		`INSERT INTO guest_accounts VALUES('guest','org','guest-user','G1','buyer','active'),('other-guest','org','other-user','G2','other-buyer','active')`,
		`INSERT INTO purchase_slips VALUES('purchase','org','confirmed')`,
		`INSERT INTO purchase_slip_lines VALUES('line','purchase')`,
		`INSERT INTO products(id,organization_id,product_code,brand,model_number,reference_number,serial_number,condition_text,accessories,base_sale_price_minor,base_sale_currency,inventory_status,purchase_date,publication_status,purchase_slip_line_id) VALUES('product','org','1709260001','ROLEX','model','ref','serial','A','',1000,'USD','purchasing','2026-09-17','private','line')`,
	} {
		if err = db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}

	migration, err := migrationFS.ReadFile("migrations/000097_box_product_visibility.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Transaction(func(tx *gorm.DB) error { return tx.Exec(string(migration)).Error }); err != nil {
		t.Fatal(err)
	}
	repo := &Repository{db: db, driver: "postgres"}
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	input := PublicationBoxInput{Name: "Box", IsActive: true, BuyerCodes: []string{"0001"}, ProductCodes: []string{"1709260001"}}
	status := func(value string) {
		t.Helper()
		must(db.Table("products").Where("id='product'").Update("inventory_status", value).Error)
	}
	save := func() { t.Helper(); _, e := repo.UpdatePublicationBox(ctx, "org", "BOX-01", "admin", input); must(e) }
	assertVisible := func(want bool) {
		t.Helper()
		items, e := repo.GuestCatalog(ctx, "org", "guest-user")
		must(e)
		if (len(items) == 1) != want {
			t.Fatalf("visible=%v items=%+v", want, items)
		}
		other, e := repo.GuestCatalog(ctx, "org", "other-user")
		must(e)
		if len(other) != 0 {
			t.Fatal("cross-buyer visibility")
		}
		boxes, e := repo.PublicationBoxes(ctx, "org")
		must(e)
		for _, b := range boxes {
			if b.BoxCode == "BOX-01" && len(b.ProductVisibility) > 0 && b.ProductVisibility[0].GuestVisible != want {
				t.Fatal("admin and guest disagree", b)
			}
		}
	}
	reset := func(initial string) {
		t.Helper()
		empty := input
		empty.ProductCodes = nil
		_, e := repo.UpdatePublicationBox(ctx, "org", "BOX-01", "admin", empty)
		must(e)
		status(initial)
		save()
	}
	// Purchasing is retained but hidden; physical arrival is the only automatic release.
	reset("purchasing")
	assertVisible(false)
	if e := repo.SetBoxProductVisibility(ctx, "org", "BOX-01", "1709260001", true); !errors.Is(e, ErrBoxVisibilityStatus) {
		t.Fatal(e)
	}
	received, e := repo.ReceivePurchaseProduct(ctx, "org", "purchase", "1709260001", "admin")
	must(e)
	if received.InventoryStatus != "in_stock" {
		t.Fatal(received)
	}
	assertVisible(true)
	status("shipped")
	assertVisible(false)
	status("in_stock")
	assertVisible(false)
	save()
	assertVisible(false) // metadata/publication saves cannot re-publish hidden members
	// Status at insertion defines which direction hides it.
	reset("shipped")
	assertVisible(true)
	status("in_stock")
	assertVisible(false)
	status("shipped")
	assertVisible(false)
	// Reservations latch the hide, regardless of the state to which they return.
	for _, initial := range []string{"in_stock", "shipped", "purchasing"} {
		for _, returned := range []string{"in_stock", "shipped"} {
			reset(initial)
			status("reserved")
			assertVisible(false)
			if e := repo.SetBoxProductVisibility(ctx, "org", "BOX-01", "1709260001", true); !errors.Is(e, ErrBoxVisibilityStatus) {
				t.Fatal(e)
			}
			status(returned)
			assertVisible(false)
			save()
			assertVisible(false)
		}
	}
	// Explicit release establishes a new baseline; a manual hide survives saves.
	must(repo.SetBoxProductVisibility(ctx, "org", "BOX-01", "1709260001", true))
	assertVisible(true)
	status("in_stock")
	assertVisible(false)
	must(repo.SetBoxProductVisibility(ctx, "org", "BOX-01", "1709260001", true))
	assertVisible(true)
	must(repo.SetBoxProductVisibility(ctx, "org", "BOX-01", "1709260001", false))
	assertVisible(false)
	save()
	assertVisible(false)
	if _, e := repo.CreateGuestPurchaseRequest(ctx, "org", "guest-user", "product", ""); !errors.Is(e, ErrProductUnavailable) {
		t.Fatal("hidden item request allowed", e)
	}
	if e := repo.SetBoxProductVisibility(ctx, "other", "BOX-01", "1709260001", true); e == nil {
		t.Fatal("cross-org update allowed")
	}
	// Each BOX keeps an independent visibility latch for the same product.
	_, e = repo.UpdatePublicationBox(ctx, "org", "BOX-02", "admin", input)
	must(e)
	items, e := repo.GuestCatalog(ctx, "org", "guest-user")
	must(e)
	if len(items) != 1 || items[0].BoxCodes != "BOX-02" {
		t.Fatal(items)
	}
	must(repo.SetBoxProductVisibility(ctx, "org", "BOX-02", "1709260001", false))
	assertVisible(false)
	// Unsupported states remove all BOX links and returning to stock does not re-add them.
	for _, invalid := range []string{"sold", "consigned", "return_pending", "cancelled", "cost_adjustment", "cost_adjusted", "combined", "broken_down", "unknown"} {
		reset("in_stock")
		status(invalid)
		var count int64
		must(db.Table("publication_box_products").Where("product_id='product'").Count(&count).Error)
		if count != 0 {
			t.Fatal("BOX membership retained", invalid)
		}
		if _, e := repo.UpdatePublicationBox(ctx, "org", "BOX-01", "admin", input); !errors.Is(e, ErrBoxProductStatus) {
			t.Fatal("invalid insertion accepted", invalid, e)
		}
		status("in_stock")
		assertVisible(false)
	}
	reset("in_stock")
	must(db.Exec("UPDATE products SET deleted_at=now() WHERE id='product'").Error)
	var count int64
	must(db.Table("publication_box_products").Count(&count).Error)
	if count != 0 {
		t.Fatal("deleted product retained")
	}
}
