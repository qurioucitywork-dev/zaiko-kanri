package persistence

import (
	"context"
	"errors"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
	"os"
	"strings"
	"testing"
)

func TestCostAdjustmentAccessoryPostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_ACCESSORY_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	if !strings.Contains(dsn, "/zaiko_accessory_test") {
		t.Fatal("non-test database rejected")
	}
	ctx := context.Background()
	repo, err := Open(config.Config{DatabaseDriver: "postgres", DatabaseURL: dsn, Environment: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(repo.Migrate(ctx))
	must(repo.SeedPreviewIdentity(ctx, "weight-test-admin-password", "weight-test-worker-password"))
	must(repo.SeedPreviewMasters(ctx))
	var org, admin, supplier, brand string
	for _, q := range []struct {
		sql   string
		value *string
	}{
		{"SELECT id FROM organizations WHERE code='PREVIEW'", &org},
		{"SELECT id FROM users WHERE username='admin' LIMIT 1", &admin},
		{"SELECT role_code FROM partner_roles WHERE role_type='supplier' AND is_active ORDER BY role_code LIMIT 1", &supplier},
		{"SELECT code FROM brands WHERE is_active ORDER BY code LIMIT 1", &brand},
	} {
		must(repo.db.Raw(q.sql).Scan(q.value).Error)
	}
	must(repo.db.Exec("INSERT INTO exchange_rate_snapshots(id,organization_id,base_currency,quote_currency,rate_scaled,scale,provider,created_by,observed_at,created_at) VALUES('accessory-cost-rate',?,'USD','JPY',15500,100,'test',?,now(),now())", org, admin).Error)
	create := func() Product {
		t.Helper()
		v, e := repo.CreateSingleProduct(ctx, SingleProductInput{OrganizationID: org, ActorUserID: admin, PurchaseDate: "2026-09-19", SupplierCode: supplier, StaffCode: "BUY-001", BrandCode: brand, CostAmountMinor: 1000, CostCurrency: "JPY", BaseSaleCurrency: "USD", WeightGrams: "999"})
		must(e)
		must(repo.db.Table("products").Where("id=?", v.Product.ID).Update("inventory_status", "in_stock").Error)
		return v.Product
	}
	for _, m := range []struct{ code, name, supp string }{{"9801", "Warranty", "年／月"}, {"9802", "LinksCost", "数字"}, {"9803", "OtherCost", "フリーワード"}} {
		_, err = repo.CreateMasterItem(ctx, org, admin, "accessories", m.code, m.name, "", "", 0, "", m.supp)
		must(err)
	}
	source := create()
	_, err = repo.StartProductCostAdjustment(ctx, org, source.ID, admin)
	must(err)
	output := CostAdjustmentOutputDraft{Kind: "product", Position: 1, AllocatedCostJPYMinor: 1000, Product: &CostAdjustmentProductDraft{BrandCode: brand, AccessoryCodes: []string{"9801", "9802", "9803"}, AccessorySupplements: AccessorySupplementValues{"9801": "2026/13", "9802": "0", "9803": "予備の留め具"}}}
	input := CostAdjustmentConfirmInput{OrganizationID: org, ActorUserID: admin, SourceProductID: source.ID, Mode: "breakdown", Outputs: []CostAdjustmentOutputDraft{output}}
	input.Outputs = append(input.Outputs, CostAdjustmentOutputDraft{Kind: "product", Position: 2, Product: &CostAdjustmentProductDraft{BrandCode: brand}})
	_, err = repo.ConfirmCostAdjustmentBreakdown(ctx, input)
	if !errors.Is(err, ErrAccessorySupplement) {
		t.Fatalf("invalid accessory month accepted: %v", err)
	}
	output.Product.AccessorySupplements["9801"] = "2026/9"

	split, err := repo.ConfirmCostAdjustmentBreakdown(ctx, input)
	must(err)
	for _, out := range split.Outputs {
		product, e := repo.ProductByID(ctx, org, out.RecordID)
		must(e)
		want := ""
		if out.Position == 1 {
			want = "2026/9"
		}
		if product.AccessorySupplements["9801"] != want {
			t.Fatalf("split warranty=%q want=%q", product.AccessorySupplements["9801"], want)
		}
	}
	a, b := create(), create()
	merge, err := repo.StartProductCostMerge(ctx, org, admin, []string{a.ID, b.ID})
	must(err)
	output.AllocatedCostJPYMinor = 2000
	output.Product.WeightGrams = "0"
	combined, err := repo.ConfirmProductCostMerge(ctx, org, admin, merge.ID, "", output)
	must(err)
	product, err := repo.ProductByID(ctx, org, combined.Outputs[0].RecordID)
	must(err)
	if product.AccessorySupplements["9801"] != "2026/9" || product.AccessorySupplements["9802"] != "0" || product.AccessorySupplements["9803"] != "予備の留め具" {
		t.Fatalf("merge accessory values lost: %+v", product.AccessorySupplements)
	}
}
