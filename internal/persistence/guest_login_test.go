package persistence

import (
	"context"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
	"os"
	"strings"
	"testing"
)

func TestGuestLoginRoundTripPostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_GUEST_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated guest test database required")
	}
	if !strings.Contains(dsn, "/zaiko_guest_test") {
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
	var org, admin string
	must(repo.db.Raw("SELECT id FROM organizations WHERE code='PREVIEW'").Scan(&org).Error)
	must(repo.db.Raw("SELECT id FROM users WHERE organization_id=? AND username='admin'", org).Scan(&admin).Error)
	address, phone, invoice := "Test address", "000-0000-0000", "T1234567890123"
	region := "overseas"
	input := GuestLoginInput{RegionType: &region, GuestCode: "G800", BuyerCode: "B800", DisplayName: "Guest Test", Company: "Guest Test Company", Email: "guest-test@example.invalid", Password: "guest-test-password-one", IsActive: true, Address: &address, Contact: &phone, InvoiceNumber: &invoice}
	created, err := repo.SaveGuestLogin(ctx, org, admin, input)
	must(err)
	if created.GuestCode != "G800" || created.Username != "G800" {
		t.Fatal("guest ID mismatch")
	}
	// Reopen the repository, as a new browser session reads persisted state.
	reloaded, err := Open(config.Config{DatabaseDriver: "postgres", DatabaseURL: dsn, Environment: "test"})
	must(err)
	defer reloaded.Close()
	stored, err := reloaded.User(ctx, org, created.ID)
	must(err)
	if stored.DisplayName != input.DisplayName || stored.BuyerCode != "B800" {
		t.Fatal("account lost on reload")
	}
	user, err := reloaded.Authenticate(ctx, "PREVIEW", "G800", input.Password)
	must(err)
	if user.Role != "guest" {
		t.Fatal("incorrect role")
	}
	_, err = reloaded.GuestCatalog(ctx, org, user.ID)
	must(err)

	checkRegion := func(want string) {
		t.Helper()
		var got string
		must(reloaded.db.Table("business_partners").Select("region_type").Where("organization_id=? AND partner_code=?", org, created.PartnerCode).Scan(&got).Error)
		guestRegion, err := reloaded.GuestRegionType(ctx, org, created.ID)
		must(err)
		if guestRegion != want {
			t.Fatalf("guest region=%s want %s", guestRegion, want)
		}
		if _, err := reloaded.GuestRegionType(ctx, org, admin); err == nil {
			t.Fatal("non-guest region lookup accepted")
		}
		if _, err := reloaded.GuestRegionType(ctx, created.ID, created.ID); err == nil {
			t.Fatal("other organization lookup accepted")
		}
		if got != want {
			t.Fatalf("partner region=%s want %s", got, want)
		}
	}
	checkRegion("overseas")
	input.UserID = created.ID
	input.RegionType = nil // Older callers must preserve the existing overseas region.
	input.Password = "guest-test-password-two"
	input.DisplayName = "Changed Guest"
	_, err = repo.SaveGuestLogin(ctx, org, admin, input)
	must(err)
	if _, err = reloaded.Authenticate(ctx, "PREVIEW", "G800", "guest-test-password-one"); err == nil {
		t.Fatal("old password accepted")
	}
	_, err = reloaded.Authenticate(ctx, "PREVIEW", "G800", input.Password)
	must(err)
	checkRegion("overseas")
	domestic := "domestic"
	input.RegionType = &domestic
	_, err = repo.SaveGuestLogin(ctx, org, admin, input)
	must(err)
	checkRegion("domestic")
	badRegion := "invalid"
	invalidRegionInput := input
	invalidRegionInput.RegionType = &badRegion
	if _, err = repo.SaveGuestLogin(ctx, org, admin, invalidRegionInput); err == nil {
		t.Fatal("invalid region accepted")
	}
	checkRegion("domestic")
	// Duplicate creation must roll back even the preceding customer-master edits.
	duplicate := input
	duplicate.UserID = ""
	duplicate.GuestCode = "G801"
	duplicate.Company = "Must not persist"
	if _, err = repo.SaveGuestLogin(ctx, org, admin, duplicate); err == nil {
		t.Fatal("duplicate buyer accepted")
	}
	var company string
	must(repo.db.Raw("SELECT legal_name FROM business_partners WHERE organization_id=? AND partner_code=?", org, created.PartnerCode).Scan(&company).Error)
	if company != input.Company {
		t.Fatal("partial update persisted")
	}
	invalid := input
	invalid.Password = "short"
	invalid.DisplayName = "Must not persist"
	if _, err = repo.SaveGuestLogin(ctx, org, admin, invalid); err == nil {
		t.Fatal("short password accepted")
	}
	stored, err = reloaded.User(ctx, org, created.ID)
	must(err)
	if stored.DisplayName != input.DisplayName {
		t.Fatal("invalid update persisted")
	}
	// Support already registered guests with legacy internal usernames.
	must(repo.db.Exec("UPDATE users SET username='guest-internal-legacy' WHERE id=?", created.ID).Error)
	_, err = reloaded.Authenticate(ctx, "PREVIEW", "G800", input.Password)
	must(err)
	if _, err = reloaded.Authenticate(ctx, "OTHER", "G800", input.Password); err == nil {
		t.Fatal("cross-organization authentication")
	}
	input.IsActive = false
	_, err = repo.SaveGuestLogin(ctx, org, admin, input)
	must(err)
	if _, err = reloaded.Authenticate(ctx, "PREVIEW", "G800", input.Password); err == nil {
		t.Fatal("inactive guest authenticated")
	}
	// No plaintext passwords in returned records or stored hashes.
	var hash string
	must(repo.db.Raw("SELECT password_hash FROM users WHERE id=?", created.ID).Scan(&hash).Error)
	if hash == input.Password || hash == "" {
		t.Fatal("password not hashed")
	}
}
