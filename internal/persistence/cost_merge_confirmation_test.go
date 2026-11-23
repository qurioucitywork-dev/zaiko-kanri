package persistence

import (
	"context"
	"errors"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
	"os"
	"strings"
	"testing"
)

func TestCostMergeConfirmationPostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_MERGE_CONFIRM_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	if !strings.Contains(dsn, "/zaiko_merge_confirm_test") {
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
	must(repo.SeedPreviewIdentity(ctx, "cost-test-admin-password", "cost-test-worker-password"))
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
	must(repo.db.Exec("INSERT INTO exchange_rate_snapshots(id,organization_id,base_currency,quote_currency,rate_scaled,scale,provider,created_by,observed_at,created_at) VALUES('consign-test-rate',?,'USD','JPY',15500,100,'test',?,now(),now())", org, admin).Error)
	var ids []string
	for i := 0; i < 2; i++ {
		created, err := repo.CreateSingleProduct(ctx, SingleProductInput{OrganizationID: org, ActorUserID: admin, PurchaseDate: "2026-09-17", SupplierCode: supplier, BrandCode: brand, CostAmountMinor: 150000, CostCurrency: "JPY", BaseSalePriceMinor: 101, BaseSaleCurrency: "USD"})
		must(err)
		must(repo.db.Table("products").Where("id=?", created.Product.ID).Update("inventory_status", "in_stock").Error)
		ids = append(ids, created.Product.ID)
	}
	merge, err := repo.StartProductCostMerge(ctx, org, admin, ids)
	must(err)
	output := CostAdjustmentOutputDraft{Kind: "product", Position: 1, AllocatedCostJPYMinor: 300000, Product: &CostAdjustmentProductDraft{BrandCode: brand, ModelNumber: "国内DX", ReferenceNumber: "DX1234", BeltText: "SS", DialText: "グリーン"}}
	var accessory struct{ Code, Name string }
	must(repo.db.Raw("SELECT code,name FROM accessories WHERE organization_id=? AND is_active ORDER BY code LIMIT 1", org).Scan(&accessory).Error)
	output.Product.AccessoryCodes = []string{accessory.Name}
	_, err = repo.ConfirmProductCostMerge(ctx, org, admin, merge.ID, "", output)
	if !errors.Is(err, ErrMasterCodeNotFound) {
		t.Fatalf("name payload did not reproduce error: %v", err)
	}
	output.Product.AccessoryCodes = []string{accessory.Code}
	result, err := repo.ConfirmProductCostMerge(ctx, org, admin, merge.ID, "", output)
	must(err)
	if len(result.Outputs) != 1 || result.Outputs[0].CostJPYMinor != 300000 || result.Outputs[0].ManagementCode != merge.ManagementCode {
		t.Fatalf("bad output: %+v", result)
	}
	saved, err := repo.ProductByID(ctx, org, result.Outputs[0].RecordID)
	must(err)
	if saved.Accessories != accessory.Name {
		t.Fatalf("accessory not persisted: %s", saved.Accessories)
	}
	var accessoryCount int64
	must(repo.db.Table("product_accessories").Where("product_id=?", saved.ID).Count(&accessoryCount).Error)
	if accessoryCount != 1 {
		t.Fatal("missing accessory relation")
	}
	_, err = repo.ConfirmProductCostMerge(ctx, org, admin, merge.ID, "", output)
	if !errors.Is(err, ErrCostAdjustmentExists) {
		t.Fatalf("duplicate allowed: %v", err)
	}
}
