package persistence

import (
	"context"
	"errors"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
	"os"
	"strings"
	"testing"
)

func TestPurchaseCostStatusPostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_COST_STATUS_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	if !strings.Contains(dsn, "/zaiko_cost_status_test") {
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
	created, err := repo.CreateSingleProduct(ctx, SingleProductInput{OrganizationID: org, ActorUserID: admin, PurchaseDate: "2026-09-17", SupplierCode: supplier, BrandCode: brand, CostAmountMinor: 1000, CostCurrency: "JPY", BaseSalePriceMinor: 100, BaseSaleCurrency: "USD"})
	must(err)
	for _, status := range []string{"purchasing", "in_stock", "shipped", "reserved", "sold", "consigned", "return_pending", "cancelled", "invalid", "cost_adjustment", "cost_adjusted"} {
		t.Run(status, func(t *testing.T) {
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			must(repo.db.Table("products").Where("id=?", created.Product.ID).Update("inventory_status", status).Error)
			before, err := repo.PurchaseSlip(ctx, org, created.PurchaseSlipID)
			must(err)
			line := before.Lines[0]
			revision := PurchaseRevisionInput{OrganizationID: org, ActorUserID: admin, PurchaseDate: "2026-09-17", SupplierCode: supplier, Notes: before.Notes, Reason: "status test", Lines: []PurchaseRevisionLine{{ID: line.ID, SKU: line.SKU, UnitCostMinor: line.UnitCostMinor + 1, BaseSalePriceMinor: line.BaseSalePriceMinor}}}
			_, err = repo.RevisePurchase(ctx, before.ID, revision)
			allowed := status == "purchasing" || status == "in_stock" || status == "shipped"
			if allowed {
				must(err)
			} else if !errors.Is(err, ErrPurchaseCostStatusLocked) && !errors.Is(err, ErrPurchaseReturnProductLocked) {
				t.Fatalf("cost not rejected: %v", err)
			}
			after, err := repo.PurchaseSlip(ctx, org, before.ID)
			must(err)
			expected := line.UnitCostMinor
			if allowed {
				expected++
			}
			if after.Lines[0].UnitCostMinor != expected {
				t.Fatal("incorrect persisted cost")
			}
			if !allowed && after.Notes != before.Notes {
				t.Fatal("rejected cost partially updated header")
			}
			// Other fields remain editable outside the preexisting return-processing lock.
			if !allowed && status != "return_pending" {
				revision.Lines[0].UnitCostMinor = line.UnitCostMinor
				revision.Lines[0].BaseSalePriceMinor++
				_, err = repo.RevisePurchase(ctx, before.ID, revision)
				must(err)
			}
		})
	}
	// The editor's old allowed state cannot bypass the current server state.
	must(repo.db.Table("products").Where("id=?", created.Product.ID).Update("inventory_status", "sold").Error)
	// Existing paid-slip protection takes precedence even when product status is allowed.
	must(repo.db.Table("products").Where("id=?", created.Product.ID).Update("inventory_status", "in_stock").Error)
	must(repo.db.Exec("UPDATE purchase_slips SET paid_at=now() WHERE id=?", created.PurchaseSlipID).Error)
	_, err = repo.RevisePurchase(ctx, created.PurchaseSlipID, PurchaseRevisionInput{OrganizationID: org, ActorUserID: admin, PurchaseDate: "2026-09-17", Reason: "paid test"})
	if !errors.Is(err, ErrPurchaseAlreadyPaid) {
		t.Fatalf("paid slip lock lost: %v", err)
	}
}
