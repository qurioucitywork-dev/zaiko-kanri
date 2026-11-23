package persistence

import (
	"context"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
	"os"
	"strings"
	"testing"
)

func TestDeletedPurchaseDetailPostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_DELETED_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated database required")
	}
	if !strings.Contains(dsn, "/zaiko_deleted_test") {
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
	must(repo.SeedPreviewIdentity(ctx, "deleted-test-admin", "deleted-test-worker"))
	must(repo.SeedPreviewMasters(ctx))
	var org, admin, supplier, brand, staff string
	for _, q := range []struct {
		sql   string
		value *string
	}{
		{"SELECT id FROM organizations WHERE code='PREVIEW'", &org},
		{"SELECT id FROM users WHERE username='admin' LIMIT 1", &admin},
		{"SELECT role_code FROM partner_roles WHERE role_type='supplier' AND is_active ORDER BY role_code LIMIT 1", &supplier},
		{"SELECT code FROM brands WHERE is_active ORDER BY code LIMIT 1", &brand},
		{"SELECT staff_code FROM staff_profiles WHERE is_purchase_staff ORDER BY staff_code LIMIT 1", &staff},
	} {
		must(repo.db.Raw(q.sql).Scan(q.value).Error)
	}
	must(repo.db.Exec("INSERT INTO exchange_rate_snapshots(id,organization_id,base_currency,quote_currency,rate_scaled,scale,provider,created_by,observed_at,created_at) VALUES('deleted-test-rate',?,'USD','JPY',15500,100,'test',?,now(),now())", org, admin).Error)
	slip, err := repo.CreatePurchase(ctx, PurchaseCreateInput{OrganizationID: org, ActorUserID: admin, SupplierCode: supplier, StaffCode: staff, PurchaseDate: "2026-09-17", Notes: "Archived note", Lines: []PurchaseLineInput{{Quantity: 1, BrandCode: brand, ModelNumber: "Archived model", ReferenceNumber: "REF-ARCHIVE", SerialNumber: "SERIAL-ARCHIVE", UnitCostMinor: 10000, CostCurrency: "JPY", BaseSalePriceMinor: 150, BaseSaleCurrency: "USD"}}})
	must(err)
	if _, err = repo.DeletedPurchaseSlip(ctx, org, slip.ID); err != ErrPurchaseNotFound {
		t.Fatal("active slip exposed as archived", err)
	}
	slip, err = repo.ConfirmPurchase(ctx, org, slip.ID, admin)
	must(err)
	code := slip.CreatedProducts[0].ProductCode
	_, err = repo.DeletePurchase(ctx, org, slip.ID, admin)
	must(err)
	archived, err := repo.DeletedPurchaseSlip(ctx, org, slip.ID)
	must(err)
	if archived.Status != "cancelled" || archived.CancelledAt == nil || archived.CancelReason == "" || archived.StaffName == "" {
		t.Fatal("archive metadata missing")
	}
	if len(archived.Lines) != 1 || archived.Lines[0].ProductCode != code || archived.Lines[0].ModelNumber != "Archived model" || archived.Lines[0].UnitCostMinor != 10000 || archived.Lines[0].BaseSalePriceMinor != 150 {
		t.Fatal("deleted line data lost")
	}
	if _, err = repo.PurchaseSlip(ctx, org, slip.ID); err != ErrPurchaseNotFound {
		t.Fatal("deleted slip exposed via active endpoint", err)
	}
	if _, err = repo.DeletedPurchaseSlip(ctx, "other-org", slip.ID); err != ErrPurchaseNotFound {
		t.Fatal("organization scope bypassed", err)
	}
	if _, err = repo.ConfirmPurchase(ctx, org, slip.ID, admin); err == nil {
		t.Fatal("archive re-confirmed")
	}
	again, err := repo.DeletedPurchaseSlip(ctx, org, slip.ID)
	must(err)
	if again.Status != "cancelled" || !again.CancelledAt.Equal(*archived.CancelledAt) {
		t.Fatal("reading archive mutated it")
	}
}
