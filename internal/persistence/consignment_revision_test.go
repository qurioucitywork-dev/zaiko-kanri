package persistence

import (
	"context"
	"errors"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
	"os"
	"strings"
	"testing"
)

func TestConsignmentRevisionPostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_CONSIGNMENT_REVISION_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	if !strings.Contains(dsn, "/zaiko_consignment_revision_test") {
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
	must(repo.db.Exec("INSERT INTO exchange_rate_snapshots(id,organization_id,base_currency,quote_currency,rate_scaled,scale,provider,created_by,observed_at,created_at) VALUES('consign-test-rate',?,'USD','JPY',15500,100,'test',?,now(),now()) ON CONFLICT(id) DO NOTHING", org, admin).Error)
	var buyer string
	must(repo.db.Raw("SELECT role_code FROM partner_roles WHERE role_type='buyer' AND is_active ORDER BY role_code LIMIT 1").Scan(&buyer).Error)
	created, err := repo.CreateSingleProduct(ctx, SingleProductInput{OrganizationID: org, ActorUserID: admin, PurchaseDate: "2026-09-17", SupplierCode: supplier, StaffCode: "BUY-001", BrandCode: brand, CostAmountMinor: 1000, CostCurrency: "JPY", BaseSalePriceMinor: 101, BaseSaleCurrency: "USD"})
	must(err)
	must(repo.db.Table("products").Where("id=?", created.Product.ID).Update("inventory_status", "in_stock").Error)
	before, err := repo.CreateConsignment(ctx, ConsignmentCreateInput{OrganizationID: org, ActorUserID: admin, ConsigneeCode: buyer, ConsignmentDate: "2026-09-17", ProductCodes: []string{created.Product.ProductCode}})
	must(err)
	input := ConsignmentRevisionInput{OrganizationID: org, ActorUserID: admin, ConsignmentDate: "2026-09-17", ConsigneeCode: buyer, Notes: before.Notes, Reason: "reason only", ExpectedUpdatedAt: before.UpdatedAt, Lines: []ConsignmentRevisionLine{{ID: before.Lines[0].ID, UnitPriceMinor: before.Lines[0].ConvertedSalePriceJPY}}}
	_, err = repo.ReviseConsignment(ctx, before.ID, input)
	if !errors.Is(err, ErrSlipRevisionUnchanged) {
		t.Fatalf("reason-only revision allowed: %v", err)
	}
	input.Notes = "notes corrected"
	notesOnly, err := repo.ReviseConsignment(ctx, before.ID, input)
	must(err)
	if notesOnly.Lines[0].SalePriceUSDMinor != 101 {
		t.Fatal("notes correction altered original USD price")
	}
	input.ExpectedUpdatedAt = notesOnly.UpdatedAt
	input.ConsignmentDate = "2026-09-18"
	input.Lines[0].UnitPriceMinor = 123456
	input.Reason = "correct price"
	saved, err := repo.ReviseConsignment(ctx, before.ID, input)
	must(err)
	reloaded, err := repo.ConsignmentSlip(ctx, org, before.ID)
	must(err)
	if reloaded.Lines[0].ConvertedSalePriceJPY != 123456 || reloaded.Lines[0].SalePriceUSDMinor != mulDivRound(123456, before.FXScale, before.FXRateScaled) || reloaded.FXRateScaled != before.FXRateScaled || !strings.Contains(reloaded.Notes, "修正理由: correct price") || string(reloaded.ConsignmentDate) != "2026-09-18" {
		t.Fatalf("revision not persisted: %+v", reloaded)
	}
	product, err := repo.ProductByID(ctx, org, created.Product.ID)
	must(err)
	if product.InventoryStatus != "consigned" || product.CostAmountMinor != 1000 || product.BaseSalePriceMinor != 101 {
		t.Fatalf("inventory altered: %+v", product)
	}
	_, err = repo.ReviseConsignment(ctx, before.ID, input)
	if !errors.Is(err, ErrConsignmentState) {
		t.Fatalf("stale edit allowed: %v", err)
	}
	input.ExpectedUpdatedAt = saved.UpdatedAt
	input.Lines[0].ID = "wrong-line"
	input.Notes = "bad line"
	_, err = repo.ReviseConsignment(ctx, before.ID, input)
	if !errors.Is(err, ErrConsignmentRevisionInvalid) {
		t.Fatalf("wrong line allowed: %v", err)
	}
	input.Lines[0].ID = before.Lines[0].ID
	input.OrganizationID = "other-org"
	_, err = repo.ReviseConsignment(ctx, before.ID, input)
	if !errors.Is(err, ErrConsignmentNotFound) {
		t.Fatalf("cross-org edit allowed: %v", err)
	}
	input.OrganizationID = org
	_, err = repo.ReturnConsignmentProduct(ctx, org, before.ID, created.Product.ProductCode, admin)
	must(err)
	input.Notes = "completed slip correction"
	input.Lines[0].UnitPriceMinor = 0
	_, err = repo.ReviseConsignment(ctx, before.ID, input)
	must(err)
	product, err = repo.ProductByID(ctx, org, created.Product.ID)
	must(err)
	if product.InventoryStatus != "in_stock" {
		t.Fatal("completed consignment restarted")
	}
	after, err := repo.ConsignmentSlip(ctx, org, before.ID)
	must(err)
	// Register the returned product with a price independent of its master default.
	for _, price := range []int64{203, 0} {
		override, err := repo.CreateConsignment(ctx, ConsignmentCreateInput{OrganizationID: org, ActorUserID: admin, ConsigneeCode: buyer, ConsignmentDate: "2026-09-17", ProductCodes: []string{created.Product.ProductCode}, Items: []ConsignmentPriceInput{{ProductCode: created.Product.ProductCode, SalePriceUSDMinor: &price}}})
		must(err)
		loaded, err := repo.ConsignmentSlip(ctx, org, override.ID)
		must(err)
		if loaded.Lines[0].SalePriceUSDMinor != price || loaded.Lines[0].ConvertedSalePriceJPY != convertShipmentUSDToJPYRoundUpToThousand(price, 15500, 100) {
			t.Fatalf("manual price lost: %+v", loaded.Lines)
		}
		product, err := repo.ProductByID(ctx, org, created.Product.ID)
		must(err)
		if product.BaseSalePriceMinor != 101 {
			t.Fatal("master price overwritten")
		}
		_, err = repo.ReturnConsignmentProduct(ctx, org, override.ID, created.Product.ProductCode, admin)
		must(err)
	}
	if after.ProcessingStatus != "completed" || after.Lines[0].ConvertedSalePriceJPY != 0 {
		t.Fatal("completion/zero price not preserved")
	}
}
