package persistence

import (
	"context"
	"errors"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
	"os"
	"strings"
	"testing"
)

func TestDirectConsignmentSalePostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_DIRECT_CONSIGNMENT_SALE_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	if !strings.Contains(dsn, "/zaiko_direct_consignment_sale_test") {
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
	var buyer string
	must(repo.db.Raw("SELECT role_code FROM partner_roles WHERE role_type='buyer' AND is_active ORDER BY role_code LIMIT 1").Scan(&buyer).Error)
	created, err := repo.CreateSingleProduct(ctx, SingleProductInput{OrganizationID: org, ActorUserID: admin, PurchaseDate: "2026-09-17", SupplierCode: supplier, BrandCode: brand, CostAmountMinor: 1000, CostCurrency: "JPY", BaseSalePriceMinor: 101, BaseSaleCurrency: "USD"})
	must(err)
	must(repo.db.Table("products").Where("id=?", created.Product.ID).Update("inventory_status", "in_stock").Error)
	before, err := repo.CreateConsignment(ctx, ConsignmentCreateInput{OrganizationID: org, ActorUserID: admin, ConsigneeCode: buyer, ConsignmentDate: "2026-09-17", ProductCodes: []string{created.Product.ProductCode}})
	must(err)
	var otherBuyer string
	must(repo.db.Raw("SELECT role_code FROM partner_roles WHERE role_type='buyer' AND is_active AND role_code<>? ORDER BY role_code LIMIT 1", buyer).Scan(&otherBuyer).Error)
	input := SaleCreateInput{OrganizationID: org, ActorUserID: admin, BuyerCode: otherBuyer, SaleDate: "2026-09-17", DisplayCurrency: "JPY", TaxMode: "taxable", TaxRateBasisPoints: 1000, Lines: []SaleLineInput{{ProductCode: created.Product.ProductCode, UnitPriceMinor: 123456}}}
	_, err = repo.CreateSale(ctx, input)
	if !errors.Is(err, ErrProductConflict) {
		t.Fatalf("wrong buyer accepted: %v", err)
	}
	input.BuyerCode = buyer
	sale, err := repo.CreateSale(ctx, input)
	must(err)
	confirmed, err := repo.ConfirmSale(ctx, org, sale.ID, admin)
	must(err)
	reloaded, err := repo.SaleSlip(ctx, org, confirmed.ID)
	must(err)
	if len(reloaded.Lines) != 1 || reloaded.Lines[0].UnitPriceMinor != 123456 {
		t.Fatalf("direct sale not persisted: %+v", reloaded)
	}
	product, err := repo.ProductByID(ctx, org, created.Product.ID)
	must(err)
	if product.InventoryStatus != "sold" {
		t.Fatalf("product status: %s", product.InventoryStatus)
	}
	after, err := repo.ConsignmentSlip(ctx, org, before.ID)
	must(err)
	if after.ProcessingStatus != "completed" {
		t.Fatalf("consignment still open: %+v", after)
	}
	_, err = repo.CreateSale(ctx, input)
	if !errors.Is(err, ErrProductUnavailable) {
		t.Fatalf("sold product accepted again: %v", err)
	}
}
