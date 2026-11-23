package persistence

import (
	"context"
	"fmt"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
	"os"
	"strings"
	"testing"
)

func TestCostMergeInventorySyntheticAudit(t *testing.T) {
	dsn := os.Getenv("ZAIKO_MERGE_AUDIT_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	if !strings.Contains(dsn, "/zaiko_merge_audit_test") {
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

	create := func(currency string, amount int64) string {
		created, e := repo.CreateSingleProduct(ctx, SingleProductInput{OrganizationID: org, ActorUserID: admin, PurchaseDate: "2026-09-17", SupplierCode: supplier, BrandCode: brand, CostAmountMinor: amount, CostCurrency: currency, BaseSaleCurrency: "USD"})
		must(e)
		must(repo.db.Table("products").Where("id=?", created.Product.ID).Update("inventory_status", "in_stock").Error)
		return created.Product.ID
	}
	draft := func(position int, cost int64) CostAdjustmentOutputDraft {
		return CostAdjustmentOutputDraft{Kind: "product", Position: position, AllocatedCostJPYMinor: cost, Product: &CostAdjustmentProductDraft{BrandCode: brand, ModelNumber: "synthetic audit"}}
	}
	for _, kind := range []string{"JPY", "USD", "split-output"} {
		t.Run(kind, func(t *testing.T) {
			left := create("JPY", 150000)
			right := ""
			total := int64(300000)
			switch kind {
			case "JPY":
				right = create("JPY", 150000)
			case "USD":
				right = create("USD", 3500)
				total = 692500
			case "split-output":
				original := create("JPY", 150000)
				_, e := repo.StartProductCostAdjustment(ctx, org, original, admin)
				must(e)
				split, e := repo.ConfirmCostAdjustmentBreakdown(ctx, CostAdjustmentConfirmInput{OrganizationID: org, ActorUserID: admin, SourceProductID: original, Mode: "breakdown", Outputs: []CostAdjustmentOutputDraft{draft(1, 75000), draft(2, 75000)}})
				must(e)
				right = split.Outputs[0].RecordID
				total = 225000
				must(repo.db.Table("products").Where("id=?", right).Update("inventory_status", "in_stock").Error)
			}
			session, e := repo.StartProductCostMerge(ctx, org, admin, []string{left, right})
			must(e)
			result, e := repo.ConfirmProductCostMerge(ctx, org, admin, session.ID, "", draft(1, total))
			must(e)
			if len(result.Outputs) != 1 || result.Outputs[0].CostJPYMinor != total {
				t.Fatalf("wrong output: %+v", result)
			}
			fmt.Printf("SYNTHETIC %s start+confirm PASS total=%d\n", kind, total)
		})
	}
}
