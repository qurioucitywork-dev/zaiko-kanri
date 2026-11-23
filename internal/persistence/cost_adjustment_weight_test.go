package persistence

import (
	"context"
	"errors"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
	"os"
	"strings"
	"testing"
)

func TestCostAdjustmentWeightPostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_COST_WEIGHT_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	if !strings.Contains(dsn, "/zaiko_cost_weight_test") {
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
	must(repo.db.Exec("INSERT INTO exchange_rate_snapshots(id,organization_id,base_currency,quote_currency,rate_scaled,scale,provider,created_by,observed_at,created_at) VALUES('weight-rate',?,'USD','JPY',15500,100,'test',?,now(),now())", org, admin).Error)
	create := func() Product {
		t.Helper()
		v, e := repo.CreateSingleProduct(ctx, SingleProductInput{OrganizationID: org, ActorUserID: admin, PurchaseDate: "2026-09-19", SupplierCode: supplier, StaffCode: "BUY-001", BrandCode: brand, CostAmountMinor: 1000, CostCurrency: "JPY", BaseSaleCurrency: "USD", WeightGrams: "999"})
		must(e)
		must(repo.db.Table("products").Where("id=?", v.Product.ID).Update("inventory_status", "in_stock").Error)
		return v.Product
	}
	source := create()
	_, err = repo.StartProductCostAdjustment(ctx, org, source.ID, admin)
	must(err)
	output := CostAdjustmentOutputDraft{Kind: "product", Position: 1, AllocatedCostJPYMinor: 1000, Product: &CostAdjustmentProductDraft{BrandCode: brand, WeightGrams: "-1"}}
	input := CostAdjustmentConfirmInput{OrganizationID: org, ActorUserID: admin, SourceProductID: source.ID, Mode: "breakdown", Outputs: []CostAdjustmentOutputDraft{output}}
	input.Outputs = append(input.Outputs, CostAdjustmentOutputDraft{Kind: "product", Position: 2, Product: &CostAdjustmentProductDraft{BrandCode: brand}})
	_, err = repo.ConfirmCostAdjustmentBreakdown(ctx, input)
	if !errors.Is(err, ErrInvalidProductWeight) {
		t.Fatalf("invalid weight accepted: %v", err)
	}
	output.Product.WeightGrams = "123.450"

	split, err := repo.ConfirmCostAdjustmentBreakdown(ctx, input)
	must(err)
	for _, out := range split.Outputs {
		product, e := repo.ProductByID(ctx, org, out.RecordID)
		must(e)
		want := ""
		if out.Position == 1 {
			want = "123.45"
		}
		if product.WeightGrams != want {
			t.Fatalf("split weight=%q want=%q", product.WeightGrams, want)
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
	if product.WeightGrams != "0" {
		t.Fatalf("merge zero weight lost: %q", product.WeightGrams)
	}
}
