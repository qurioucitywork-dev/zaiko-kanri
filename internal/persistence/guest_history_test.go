package persistence

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

func TestGuestRequestHistoryPostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_GUEST_HISTORY_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated history database required")
	}
	if !strings.Contains(dsn, "/zaiko_guest_history_test") {
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
	must(repo.SeedPreviewIdentity(ctx, "guest-test-admin-password", "guest-test-worker-password"))
	must(repo.SeedPreviewMasters(ctx))
	must(repo.SeedPreviewInventory(ctx))
	var org, admin, first, second string
	must(repo.db.Raw("SELECT id FROM organizations WHERE code='PREVIEW'").Scan(&org).Error)
	must(repo.db.Raw("SELECT id FROM users WHERE organization_id=? AND username='admin'", org).Scan(&admin).Error)
	must(repo.db.Raw("SELECT user_id FROM guest_accounts WHERE organization_id=? AND guest_code='G001'", org).Scan(&first).Error)
	must(repo.db.Raw("SELECT user_id FROM guest_accounts WHERE organization_id=? AND guest_code='G002'", org).Scan(&second).Error)
	_, err = repo.UpdatePublicationBox(ctx, org, "BOX-01", admin, PublicationBoxInput{Name: "Test", IsActive: true, BuyerCodes: []string{"B001", "B002"}, ProductCodes: []string{"2607260001"}})
	must(err)
	one, err := repo.CreateGuestPurchaseRequest(ctx, org, first, "product_preview_001", "First guest note")
	must(err)
	_, err = repo.CreateGuestPurchaseRequest(ctx, org, second, "product_preview_001", "Second guest private note")
	must(err)
	if one.RequestedPriceMinor == nil || *one.RequestedPriceMinor != 7613 || one.RequestedCurrency != "USD" || one.ReferenceNumber != "116610LN" {
		t.Fatalf("incorrect snapshot: %+v", one)
	}
	// Product edits, unpublishing and removal from the catalogue cannot rewrite history.
	must(repo.db.Exec("UPDATE products SET brand='Changed',model_number='Changed',reference_number='Changed',base_sale_price_minor=1,publication_status='private' WHERE id='product_preview_001'").Error)
	for _, status := range []string{"pending", "approved", "rejected", "cancelled", "expired", "sold"} {
		must(repo.db.Exec("UPDATE purchase_requests SET status=? WHERE id=?", status, one.ID).Error)
		records, err := repo.PurchaseRequests(ctx, org, first, database.RoleGuest, "")
		must(err)
		if len(records) != 1 || records[0].ID != one.ID || records[0].Status != status || records[0].Message != "First guest note" || records[0].Brand != one.Brand || records[0].ModelNumber != one.ModelNumber || *records[0].RequestedPriceMinor != 7613 {
			t.Fatalf("history not isolated/stable: %+v", records)
		}
	}
	records, err := repo.PurchaseRequests(ctx, org, second, database.RoleGuest, "")
	must(err)
	if len(records) != 1 || records[0].Message != "Second guest private note" {
		t.Fatal("other guest history leaked")
	}
	records, err = repo.PurchaseRequests(ctx, org, "unknown-user", database.RoleGuest, "")
	must(err)
	if len(records) != 0 {
		t.Fatal("unknown user sees history")
	}
	records, err = repo.PurchaseRequests(ctx, "other-org", first, database.RoleGuest, "")
	must(err)
	if len(records) != 0 {
		t.Fatal("cross-organization history leaked")
	}
	// Old requests have no historical price. Never pretend today's price was sent.
	must(repo.db.Exec("UPDATE purchase_requests SET product_snapshot=NULL WHERE id=?", one.ID).Error)
	legacy, err := repo.PurchaseRequest(ctx, org, one.ID)
	must(err)
	if legacy.RequestedPriceMinor != nil || legacy.RequestedCurrency != "" {
		t.Fatal("invented historical price")
	}
}
