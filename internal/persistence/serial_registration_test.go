package persistence

import (
	"context"
	"errors"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
	"os"
	"strings"
	"testing"
)

func TestRepeatedSerialRegistrationPostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_SERIAL_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	if !strings.Contains(dsn, "/zaiko_serial_test") {
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
	must(repo.SeedPreviewIdentity(ctx, "serial-test-admin-password", "serial-test-worker-password"))
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
	input := SingleProductInput{OrganizationID: org, ActorUserID: admin, PurchaseDate: "2026-09-17", SupplierCode: supplier, BrandCode: brand, SerialNumber: "SAME-SERIAL", CostAmountMinor: 1000, CostCurrency: "JPY", BaseSalePriceMinor: 100, BaseSaleCurrency: "USD"}
	first, err := repo.CreateSingleProduct(ctx, input)
	must(err)
	second, err := repo.CreateSingleProduct(ctx, input)
	must(err)
	if first.Product.ProductCode == second.Product.ProductCode {
		t.Fatal("management codes must differ")
	}
	duplicateCode := input
	duplicateCode.ProductCode = first.Product.ProductCode
	_, err = repo.CreateSingleProduct(ctx, duplicateCode)
	if !errors.Is(err, ErrDuplicateProductCode) {
		t.Fatalf("management duplicate not rejected: %v", err)
	}
	input.SerialNumber = "OTHER-SERIAL"
	third, err := repo.CreateSingleProduct(ctx, input)
	must(err)
	serial := "SAME-SERIAL"
	updated, err := repo.UpdateProduct(ctx, org, third.Product.ID, admin, ProductUpdateInput{SerialNumber: &serial, Reason: "serial correction"})
	must(err)
	if updated.SerialNumber != serial {
		t.Fatal("edited serial lost")
	}
	line := PurchaseLineInput{Quantity: 1, BrandCode: brand, SerialNumber: serial, CostCurrency: "JPY", UnitCostMinor: 1000, BaseSaleCurrency: "USD", BaseSalePriceMinor: 100}
	purchase, err := repo.CreatePurchase(ctx, PurchaseCreateInput{OrganizationID: org, ActorUserID: admin, SupplierCode: supplier, PurchaseDate: "2026-09-17", Lines: []PurchaseLineInput{line, line}})
	must(err)
	purchase, err = repo.ConfirmPurchase(ctx, org, purchase.ID, admin)
	must(err)
	if len(purchase.Lines) != 2 {
		t.Fatal("purchase lines lost")
	}
	source, err := repo.StartProductCostAdjustment(ctx, org, first.Product.ID, admin)
	must(err)
	_, cost, err := resolveCostAdjustmentSource(repo.db, org, source)
	must(err)
	adjustment, err := repo.ConfirmCostAdjustmentBreakdown(ctx, CostAdjustmentConfirmInput{OrganizationID: org, ActorUserID: admin, SourceProductID: source.ID, Mode: "breakdown", Outputs: []CostAdjustmentOutputDraft{
		{Position: 1, Kind: "product", AllocatedCostJPYMinor: cost, Product: &CostAdjustmentProductDraft{BrandCode: brand, SerialNumber: serial}},
		{Position: 2, Kind: "product", AllocatedCostJPYMinor: 0, Product: &CostAdjustmentProductDraft{BrandCode: brand, SerialNumber: serial}},
	}})
	must(err)
	if len(adjustment.Outputs) != 2 {
		t.Fatal("cost outputs lost")
	}
	reloaded, err := Open(config.Config{DatabaseDriver: "postgres", DatabaseURL: dsn, Environment: "test"})
	must(err)
	defer reloaded.Close()
	var count, uniqueCodes int64
	must(reloaded.db.Table("products").Where("organization_id=? AND serial_number=?", org, serial).Count(&count).Error)
	must(reloaded.db.Table("products").Where("organization_id=? AND serial_number=?", org, serial).Distinct("product_code").Count(&uniqueCodes).Error)
	if count != 7 || uniqueCodes != count {
		t.Fatalf("saved duplicate serials=%d unique management codes=%d", count, uniqueCodes)
	}
}
