package persistence

import (
	"errors"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
	"os"
	"strings"
	"testing"
)

func TestAccessorySupplementValidation(t *testing.T) {
	for _, test := range []struct {
		label, value, want string
		bad                bool
	}{
		{"フリーワード", "  予備の留め具  ", "予備の留め具", false}, {"フリーワード", strings.Repeat("字", 201), "", true}, {"月", "09", "9", false}, {"月", "13", "", true}, {"数字", "3", "3", false}, {"数字", "-1", "", true}, {"数字", "1.5", "", true},
		{"年／月", "2026/09", "2026/9", false}, {"年／月", "2026/", "", true}, {"年／月", "2026/13", "", true}, {"月", "", "", false},
	} {
		got, e := normalizeAccessorySupplement(test.label, test.value)
		if (e != nil) != test.bad || got != test.want {
			t.Fatalf("%+v: %q %v", test, got, e)
		}
	}
}
func TestAccessorySupplementPostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_ACCESSORY_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated database required")
	}
	if !strings.Contains(dsn, "/zaiko_accessory_test") {
		t.Fatal("test database required")
	}
	repo, e := Open(config.Config{DatabaseDriver: "postgres", DatabaseURL: dsn, Environment: "test"})
	if e != nil {
		t.Fatal(e)
	}
	defer repo.Close()
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	ctx := t.Context()
	must(repo.Migrate(ctx))
	must(repo.SeedPreviewIdentity(ctx, "accessory-test-password", "accessory-worker-password"))
	must(repo.SeedPreviewMasters(ctx))
	var org, admin, supplier, brand string
	for _, q := range []struct {
		sql string
		out *string
	}{{"SELECT id FROM organizations WHERE code='PREVIEW'", &org}, {"SELECT id FROM users WHERE username='admin' LIMIT 1", &admin}, {"SELECT role_code FROM partner_roles WHERE role_type='supplier' AND is_active LIMIT 1", &supplier}, {"SELECT code FROM brands WHERE is_active LIMIT 1", &brand}} {
		must(repo.db.Raw(q.sql).Scan(q.out).Error)
	}
	for _, m := range []struct{ code, name, supp string }{{"9901", "Monthly", "月"}, {"9902", "YearMonth", "年／月"}, {"9903", "Links", "数字"}, {"9904", "Other", "フリーワード"}} {
		_, e = repo.CreateMasterItem(ctx, org, admin, "accessories", m.code, m.name, "", "", 0, "", m.supp)
		must(e)
	}
	codes := []string{"9901", "9902", "9903", "9904"}
	values := AccessorySupplementValues{"9901": "9", "9902": "2026/9", "9903": "3", "9904": "予備の留め具"}
	created, e := repo.CreateSingleProduct(ctx, SingleProductInput{OrganizationID: org, ActorUserID: admin, StaffCode: "BUY-001", SupplierCode: supplier, BrandCode: brand, PurchaseDate: "2026-09-18", CostCurrency: "JPY", CostAmountMinor: 1000, BaseSaleCurrency: "USD", AccessoryCodes: codes, AccessorySupplements: values})
	must(e)
	check := func(p Product) {
		t.Helper()
		if p.AccessorySupplements["9901"] != "9" || p.AccessorySupplements["9902"] != "2026/9" || p.AccessorySupplements["9903"] != "3" || p.AccessorySupplements["9904"] != "予備の留め具" {
			t.Fatalf("values lost: %+v", p.AccessorySupplements)
		}
	}
	check(created.Product)
	loaded, e := repo.ProductByID(ctx, org, created.Product.ID)
	must(e)
	check(loaded)
	note := "note only"
	loaded, e = repo.UpdateProduct(ctx, org, loaded.ID, admin, ProductUpdateInput{Notes: &note})
	must(e)
	check(loaded)
	_, e = repo.UpdateProduct(ctx, org, loaded.ID, admin, ProductUpdateInput{AccessorySupplements: AccessorySupplementValues{"9901": "13"}})
	if !errors.Is(e, ErrAccessorySupplement) {
		t.Fatalf("invalid month accepted: %v", e)
	}
	fewer := []string{"9902", "9903"}
	loaded, e = repo.UpdateProduct(ctx, org, loaded.ID, admin, ProductUpdateInput{AccessoryCodes: &fewer})
	must(e)
	if _, ok := loaded.AccessorySupplements["9901"]; ok {
		t.Fatal("removed accessory value survived")
	}
	loaded, e = repo.UpdateProduct(ctx, org, loaded.ID, admin, ProductUpdateInput{AccessorySupplements: AccessorySupplementValues{}})
	must(e)
	if len(loaded.AccessorySupplements) != 0 {
		t.Fatal("clear failed")
	}
	slip, e := repo.CreatePurchase(ctx, PurchaseCreateInput{OrganizationID: org, ActorUserID: admin, StaffCode: "BUY-001", SupplierCode: supplier, PurchaseDate: "2026-09-18", Lines: []PurchaseLineInput{{Quantity: 1, BrandCode: brand, UnitCostMinor: 1000, CostCurrency: "JPY", BaseSaleCurrency: "USD", AccessoryCodes: codes, AccessorySupplements: values}}})
	must(e)
	if len(slip.Lines) != 1 || slip.Lines[0].AccessorySupplements["9902"] != "2026/9" {
		t.Fatalf("purchase draft values lost: %+v", slip.Lines)
	}
	_, e = repo.ConfirmPurchase(ctx, org, slip.ID, admin)
	must(e)
	var p Product
	must(repo.db.Where("purchase_slip_line_id=?", slip.Lines[0].ID).Take(&p).Error)
	check(p)
}
