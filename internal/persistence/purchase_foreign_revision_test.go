package persistence

import (
	"context"
	"errors"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
	"os"
	"strings"
	"testing"
)

func TestForeignPurchaseRevisionPostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_FOREIGN_REVISION_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	if !strings.Contains(dsn, "/zaiko_foreign_revision_test") {
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
	must(repo.SeedPreviewIdentity(ctx, "foreign-test-admin-password", "foreign-test-worker-password"))
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
	for _, tc := range []struct {
		currency   string
		rate, want int64
	}{{"USD", 15500, 155155}, {"HKD", 1976, 19780}} {
		t.Run(tc.currency, func(t *testing.T) {
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			rateID := "foreign-revision-" + tc.currency
			must(repo.db.Exec(`INSERT INTO exchange_rate_snapshots(id,organization_id,base_currency,quote_currency,rate_scaled,scale,provider,created_by,observed_at,created_at) VALUES(?,?,?,'JPY',?,100,'test',?,now(),now())`, rateID, org, tc.currency, tc.rate, admin).Error)
			purchase, err := repo.CreatePurchase(ctx, PurchaseCreateInput{OrganizationID: org, ActorUserID: admin, SupplierCode: supplier, PurchaseDate: "2026-09-17", PurchaseTaxMode: PurchaseTaxModeOverseas, Lines: []PurchaseLineInput{{Quantity: 1, BrandCode: brand, CostCurrency: tc.currency, UnitCostMinor: 1000, BaseSaleCurrency: "USD", BaseSalePriceMinor: 100}}})
			must(err)
			purchase, err = repo.ConfirmPurchase(ctx, org, purchase.ID, admin)
			must(err)
			line := purchase.Lines[0]
			var productID string
			must(repo.db.Table("products").Select("id").Where("purchase_slip_line_id=?", line.ID).Scan(&productID).Error)
			// Later master rates must not replace the registration snapshot on revision.
			must(repo.db.Exec(`INSERT INTO exchange_rate_snapshots(id,organization_id,base_currency,quote_currency,rate_scaled,scale,provider,created_by,observed_at,created_at) VALUES(?,?,?,'JPY',99999,100,'test',?,now()+interval '1 hour',now())`, rateID+"-later", org, tc.currency, admin).Error)
			input := PurchaseRevisionInput{OrganizationID: org, ActorUserID: admin, PurchaseDate: "2026-09-17", SupplierCode: supplier, StaffCode: purchase.StaffCode, Notes: purchase.Notes, Reason: "foreign amount correction", Lines: []PurchaseRevisionLine{{ID: line.ID, SKU: line.SKU, UnitCostMinor: 1001, BaseSalePriceMinor: line.BaseSalePriceMinor}}}
			saved, err := repo.RevisePurchase(ctx, purchase.ID, input)
			must(err)
			if saved.Lines[0].UnitCostMinor != 1001 || saved.Lines[0].ConvertedTotalJPY != tc.want || saved.Lines[0].FXRateSnapshotID != rateID || saved.Lines[0].FXRateScaled != tc.rate {
				t.Fatalf("snapshot or revised amount wrong: %+v", saved.Lines[0])
			}
			reloaded, err := repo.PurchaseSlip(ctx, org, purchase.ID)
			must(err)
			if reloaded.Lines[0].ConvertedTotalJPY != tc.want {
				t.Fatal("revision lost after reload")
			}
			product, err := repo.ProductByID(ctx, org, productID)
			must(err)
			if product.CostAmountMinor != 1001 || product.CostCurrency != tc.currency {
				t.Fatal("inventory cost not updated")
			}
			// Block prohibited states even for foreign currencies, rolling back changes.
			must(repo.db.Table("products").Where("id=?", productID).Update("inventory_status", "sold").Error)
			input.Lines[0].UnitCostMinor = 2000
			_, err = repo.RevisePurchase(ctx, purchase.ID, input)
			if !errors.Is(err, ErrPurchaseCostStatusLocked) {
				t.Fatalf("status lock: %v", err)
			}
			must(repo.db.Table("products").Where("id=?", productID).Update("inventory_status", "shipped").Error)
			_, err = repo.RevisePurchase(ctx, purchase.ID, input)
			must(err)
			must(repo.db.Exec("UPDATE purchase_slips SET paid_at=now() WHERE id=?", purchase.ID).Error)
			input.Lines[0].UnitCostMinor = 3000
			_, err = repo.RevisePurchase(ctx, purchase.ID, input)
			if !errors.Is(err, ErrPurchaseAlreadyPaid) {
				t.Fatalf("payment lock: %v", err)
			}
		})
	}
}
