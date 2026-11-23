package persistence

import (
	"context"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
	"os"
	"strings"
	"testing"
)

// Run only against the disposable dashboard database; no application data is used.
func TestDashboardProductFactsPostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_DASHBOARD_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	if !strings.Contains(dsn, "/zaiko_dashboard_test?") && !strings.Contains(dsn, "/zaiko_dashboard_fx_20260928?") {
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
	must(repo.SeedPreviewIdentity(ctx, "dashboard-test-admin", "dashboard-test-worker"))
	must(repo.SeedPreviewMasters(ctx))
	var org, admin, supplier, brand, buyer string
	for _, q := range []struct {
		sql  string
		dest *string
	}{
		{"SELECT id FROM organizations WHERE code='PREVIEW'", &org},
		{"SELECT id FROM users WHERE username='admin' LIMIT 1", &admin},
		{"SELECT role_code FROM partner_roles WHERE role_type='supplier' AND is_active ORDER BY role_code LIMIT 1", &supplier},
		{"SELECT code FROM brands WHERE is_active ORDER BY code LIMIT 1", &brand},
		{"SELECT role_code FROM partner_roles WHERE role_type='buyer' AND is_active ORDER BY role_code LIMIT 1", &buyer},
	} {
		must(repo.db.Raw(q.sql).Scan(q.dest).Error)
	}
	must(repo.db.Exec("INSERT INTO exchange_rate_snapshots(id,organization_id,base_currency,quote_currency,rate_scaled,scale,provider,created_by,observed_at,created_at) VALUES('dashboard-test-rate',?,'USD','JPY',15500,100,'test',?,now(),now())", org, admin).Error)
	made, err := repo.CreateSingleProduct(ctx, SingleProductInput{OrganizationID: org, ActorUserID: admin, PurchaseDate: "2026-09-17", SupplierCode: supplier, StaffCode: "BUY-001", BrandCode: brand, CostAmountMinor: 100, CostCurrency: "USD", BaseSalePriceMinor: 200, BaseSaleCurrency: "USD"})
	must(err)
	slip, err := repo.PurchaseSlip(ctx, org, made.PurchaseSlipID)
	must(err)
	line := slip.Lines[0]
	var unitCost int64
	must(repo.db.Raw("SELECT round(unit_cost_minor*fx_rate_scaled::numeric/fx_scale)::bigint FROM purchase_slip_lines WHERE id=?", line.ID).Scan(&unitCost).Error)
	must(repo.db.Exec("UPDATE products SET inventory_status='in_stock' WHERE id=?", made.Product.ID).Error)
	sale, err := repo.CreateSale(ctx, SaleCreateInput{OrganizationID: org, ActorUserID: admin, BuyerCode: buyer, SaleDate: "2026-09-23", DisplayCurrency: "JPY", TaxMode: "taxable", TaxRateBasisPoints: 1000, Lines: []SaleLineInput{{ProductCode: made.Product.ProductCode, UnitPriceMinor: 30000}}})
	must(err)
	page, err := repo.Products(ctx, org, ProductFilter{PageSize: 100, IncludeCancelled: true})
	must(err)
	var fact productRow
	for _, p := range page.Items {
		if p.ID == made.Product.ID {
			fact = p
		}
	}
	if fact.PurchaseSlipID != made.PurchaseSlipID || fact.SaleSlipID != "" || fact.DashboardPurchaseJPY != unitCost || fact.DashboardCostJPY != unitCost {
		t.Fatalf("draft or purchase facts: %+v", fact)
	}
	_, err = repo.ConfirmSale(ctx, org, sale.ID, admin)
	must(err)
	// Current inventory cost may change independently from the purchase snapshot.
	must(repo.db.Exec("UPDATE products SET cost_amount_minor=20000,cost_currency='JPY' WHERE id=?", made.Product.ID).Error)
	page, err = repo.Products(ctx, org, ProductFilter{PageSize: 100, IncludeCancelled: true})
	must(err)
	for _, p := range page.Items {
		if p.ID == made.Product.ID {
			fact = p
		}
	}
	if fact.SaleSlipID != sale.ID || fact.SaleBuyerCode != buyer || fact.SalePriceJPYMinor != 30000 || fact.DashboardPurchaseJPY != unitCost || fact.DashboardCostJPY != 20000 {
		t.Fatalf("confirmed sale facts: %+v", fact)
	}
	// Frozen USD rates win, even when newer master rates exist.
	must(repo.db.Exec("UPDATE purchase_slip_lines SET fx_rate_scaled=15500,fx_scale=100 WHERE id=?", line.ID).Error)
	must(repo.db.Exec("UPDATE sales_lines SET fx_rate_snapshot_id='dashboard-test-rate',fx_rate_scaled=16000,fx_scale=100 WHERE sales_slip_id=?", sale.ID).Error)
	readFact := func() productRow {
		t.Helper()
		result, readErr := repo.Products(ctx, org, ProductFilter{PageSize: 100, IncludeCancelled: true})
		must(readErr)
		for _, item := range result.Items {
			if item.ID == made.Product.ID {
				return item
			}
		}
		t.Fatal("missing test product")
		return productRow{}
	}
	fact = readFact()
	if fact.DashboardPurchaseUSDRate != 155 || fact.DashboardSaleUSDRate != 160 {
		t.Fatalf("frozen USD rates: %v / %v", fact.DashboardPurchaseUSDRate, fact.DashboardSaleUSDRate)
	}
	// JPY and HKD transactions resolve USD history independently of their source rate.
	must(repo.db.Exec("INSERT INTO exchange_rate_snapshots(id,organization_id,base_currency,quote_currency,rate_scaled,scale,provider,created_by,observed_at,created_at) VALUES ('dashboard-usd-before',?,'USD','JPY',10000,100,'test',?,'2026-09-17T23:59:59+09:00',now()),('dashboard-usd-next-day',?,'USD','JPY',12000,100,'test',?,'2026-09-18T00:00:00+09:00',now()),('dashboard-usd-sale',?,'USD','JPY',20000,100,'test',?,'2026-09-23T12:00:00+09:00',now()),('dashboard-hkd',?,'HKD','JPY',2000,100,'test',?,'2026-09-01T00:00:00+09:00',now())", org, admin, org, admin, org, admin, org, admin).Error)
	must(repo.db.Exec("INSERT INTO exchange_rate_snapshots(id,organization_id,base_currency,quote_currency,rate_scaled,scale,provider,created_by,observed_at,created_at) SELECT 'dashboard-new-'||n,?,'USD','JPY',99900,100,'test',?,'2027-01-01'::timestamptz+n*interval '1 day',now() FROM generate_series(1,110) n", org, admin).Error)
	must(repo.db.Exec("UPDATE purchase_slip_lines SET cost_currency='JPY',fx_rate_snapshot_id=NULL,fx_rate_scaled=100,fx_scale=100 WHERE id=?", line.ID).Error)
	must(repo.db.Exec("UPDATE sales_lines SET sale_currency='HKD',fx_rate_snapshot_id='dashboard-hkd',fx_rate_scaled=2000,fx_scale=100 WHERE sales_slip_id=?", sale.ID).Error)
	fact = readFact()
	if fact.DashboardPurchaseUSDRate != 100 || fact.DashboardSaleUSDRate != 200 {
		t.Fatalf("historical lookup beyond latest 100, Tokyo date boundary and HKD: %v / %v", fact.DashboardPurchaseUSDRate, fact.DashboardSaleUSDRate)
	}
	must(repo.db.Exec("UPDATE products SET purchase_date='1900-01-01' WHERE id=?", made.Product.ID).Error)
	must(repo.db.Exec("UPDATE sales_slips SET sale_date='1900-01-01' WHERE id=?", sale.ID).Error)
	fact = readFact()
	if fact.DashboardPurchaseUSDRate != 0 || fact.DashboardSaleUSDRate != 0 {
		t.Fatal("missing historical rates must not use future/current rates")
	}

	empty, err := repo.Products(ctx, "another-org", ProductFilter{})
	must(err)
	if empty.Total != 0 {
		t.Fatal("cross-organization products exposed")
	}
}
