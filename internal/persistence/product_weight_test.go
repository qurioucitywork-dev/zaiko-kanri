package persistence

import (
	"errors"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
	"os"
	"strings"
	"testing"
)

func TestProductWeightPostgres(t *testing.T) {
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
	for _, m := range []struct{ code, name, supp string }{{"9901", "Monthly", "月"}, {"9902", "YearMonth", "年／月"}, {"9903", "Links", "数字"}} {
		_, e = repo.CreateMasterItem(ctx, org, admin, "accessories", m.code, m.name, "", "", 0, "", m.supp)
		must(e)
	}
	codes := []string{"9901", "9902", "9903"}
	values := AccessorySupplementValues{"9901": "9", "9902": "2026/9", "9903": "3"}
	created, e := repo.CreateSingleProduct(ctx, SingleProductInput{OrganizationID: org, ActorUserID: admin, WeightGrams: "100.25", StaffCode: "BUY-001", SupplierCode: supplier, BrandCode: brand, PurchaseDate: "2026-09-18", CostCurrency: "JPY", CostAmountMinor: 1000, BaseSaleCurrency: "USD", AccessoryCodes: codes, AccessorySupplements: values})
	must(e)

	if created.Product.WeightGrams != "100.25" {
		t.Fatal("new product weight lost")
	}
	weight := "125.50"
	product, err := repo.UpdateProduct(ctx, org, created.Product.ID, admin, ProductUpdateInput{WeightGrams: &weight})
	must(err)
	if product.WeightGrams != "125.5" {
		t.Fatalf("save: %q", product.WeightGrams)
	}
	loaded, err := repo.ProductByID(ctx, org, product.ID)
	must(err)
	if loaded.WeightGrams != "125.5" {
		t.Fatal("reload lost weight")
	}
	note := "weight preserved"
	loaded, err = repo.UpdateProduct(ctx, org, product.ID, admin, ProductUpdateInput{Notes: &note})
	must(err)
	if loaded.WeightGrams != "125.5" {
		t.Fatal("unrelated update lost weight")
	}
	bad := "-1"
	_, err = repo.UpdateProduct(ctx, org, product.ID, admin, ProductUpdateInput{WeightGrams: &bad})
	if !errors.Is(err, ErrInvalidProductWeight) {
		t.Fatalf("invalid weight accepted: %v", err)
	}
	empty := ""
	loaded, err = repo.UpdateProduct(ctx, org, product.ID, admin, ProductUpdateInput{WeightGrams: &empty})
	must(err)
	if loaded.WeightGrams != "" {
		t.Fatal("clear failed")
	}
	slip, err := repo.CreatePurchase(ctx, PurchaseCreateInput{OrganizationID: org, ActorUserID: admin, StaffCode: "BUY-001", SupplierCode: supplier, PurchaseDate: "2026-09-19", Lines: []PurchaseLineInput{{Quantity: 1, BrandCode: brand, UnitCostMinor: 1000, CostCurrency: "JPY", BaseSaleCurrency: "USD", WeightGrams: "90.750"}}})
	must(err)
	if len(slip.Lines) != 1 || slip.Lines[0].WeightGrams != "90.75" {
		t.Fatal("draft weight lost")
	}
	_, err = repo.ConfirmPurchase(ctx, org, slip.ID, admin)
	must(err)
	var confirmed Product
	must(repo.db.Where("purchase_slip_line_id=?", slip.Lines[0].ID).Take(&confirmed).Error)
	if confirmed.WeightGrams != "90.75" {
		t.Fatal("confirmed weight lost")
	}
}
func TestProductWeightValidation(t *testing.T) {
	for _, c := range []struct {
		in, want string
		bad      bool
	}{{"", "", false}, {"125.50", "125.5", false}, {"0", "0", false}, {"0.001", "0.001", false}, {"-1", "", true}, {"NaN", "", true}, {"Inf", "", true}, {"abc", "", true}} {
		got, err := normalizeProductWeight(c.in)
		if got != c.want || (err != nil) != c.bad {
			t.Fatalf("%+v: %q %v", c, got, err)
		}
	}
}
