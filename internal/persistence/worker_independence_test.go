package persistence

import (
	"context"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
	"os"
	"strings"
	"testing"
)

func TestWorkerBuyerIndependencePostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_WORKER_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated database required")
	}
	if !strings.Contains(dsn, "/zaiko_worker_test") {
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
	must(repo.SeedPreviewIdentity(ctx, "worker-test-admin", "worker-test-worker"))
	var org, admin string
	must(repo.db.Raw("SELECT id FROM organizations WHERE code='PREVIEW'").Scan(&org).Error)
	must(repo.db.Raw("SELECT id FROM users WHERE organization_id=? AND username='admin'", org).Scan(&admin).Error)
	if _, err := lookupStaffProfile(repo.db, org, admin, ""); err == nil {
		t.Fatal("login implicitly supplied buyer")
	}
	initial, err := repo.PurchaseStaff(ctx, org)
	must(err)
	created, err := repo.CreateUser(ctx, org, admin, UserCreateInput{Username: "independent-worker", Password: "worker-test-password", DisplayName: "Worker", Email: "worker@example.invalid", Role: "worker"})
	must(err)
	if created.StaffCode != "" || created.StaffProfileID != "" || created.IsPurchaseStaff {
		t.Fatal("worker acquired a buyer code")
	}
	after, err := repo.PurchaseStaff(ctx, org)
	must(err)
	if len(after) != len(initial) {
		t.Fatal("worker created buyer")
	}
	name := "Independent Buyer"
	buyer, err := repo.SavePurchaseStaff(ctx, org, "", PurchaseStaffInput{StaffCode: "0901", DisplayName: &name})
	must(err)
	resolved, err := lookupStaffProfile(repo.db, org, created.ID, buyer.StaffCode)
	must(err)
	if resolved != buyer.ID {
		t.Fatal("independent buyer unavailable for purchase")
	}
	var linked int64
	must(repo.db.Table("staff_profiles").Where("id=? AND user_id IS NOT NULL", buyer.ID).Count(&linked).Error)
	if linked != 0 {
		t.Fatal("buyer linked to login")
	}
	users, err := repo.Users(ctx, org, true)
	must(err)
	name = "Renamed buyer"
	_, err = repo.SavePurchaseStaff(ctx, org, buyer.ID, PurchaseStaffInput{DisplayName: &name})
	must(err)
	persisted, err := repo.User(ctx, org, created.ID)
	must(err)
	if persisted.DisplayName != "Worker" {
		t.Fatal("buyer renamed worker")
	}
	login := "independent-renamed"
	workerName := "Renamed worker"
	_, err = repo.UpdateUser(ctx, org, created.ID, admin, UserUpdateInput{Username: &login, DisplayName: &workerName})
	must(err)
	_, err = repo.Authenticate(ctx, "PREVIEW", login, "worker-test-password")
	must(err)
	off := false
	_, err = repo.SavePurchaseStaff(ctx, org, buyer.ID, PurchaseStaffInput{IsActive: &off})
	must(err)
	_, err = repo.Authenticate(ctx, "PREVIEW", login, "worker-test-password")
	must(err)
	usersAfter, err := repo.Users(ctx, org, true)
	must(err)
	if len(usersAfter) != len(users) {
		t.Fatal("buyer changed login count")
	}
	// Legacy accounts retain credentials, while buyer name/activity is independent.
	var legacy struct{ ID, UserID, DisplayName string }
	must(repo.db.Table("staff_profiles").Where("organization_id=? AND user_id IS NOT NULL", org).First(&legacy).Error)
	oldUser, err := repo.User(ctx, org, legacy.UserID)
	must(err)
	_, err = repo.UpdateUser(ctx, org, legacy.UserID, admin, UserUpdateInput{DisplayName: &workerName})
	must(err)
	var unchanged string
	must(repo.db.Table("staff_profiles").Select("display_name").Where("id=?", legacy.ID).Scan(&unchanged).Error)
	if unchanged != legacy.DisplayName {
		t.Fatal("legacy user edit renamed buyer")
	}
	_, err = repo.SavePurchaseStaff(ctx, org, legacy.ID, PurchaseStaffInput{DisplayName: &name, IsActive: &off})
	must(err)
	preserved, err := repo.User(ctx, org, oldUser.ID)
	must(err)
	if preserved.DisplayName != workerName || preserved.IsActive != oldUser.IsActive {
		t.Fatal("legacy buyer edit changed login")
	}
	if _, err = repo.SavePurchaseStaff(ctx, "other-org", buyer.ID, PurchaseStaffInput{DisplayName: &name}); err == nil {
		t.Fatal("cross-organization update allowed")
	}
	// Duplicate buyer code has no side effects on existing logins.
	if _, err = repo.SavePurchaseStaff(ctx, org, "", PurchaseStaffInput{StaffCode: "0901", DisplayName: &name}); err == nil {
		t.Fatal("duplicate code allowed")
	}
	must(repo.ChangeUserPassword(ctx, org, created.ID, "updated-worker-password"))
	_, err = repo.Authenticate(ctx, "PREVIEW", login, "updated-worker-password")
	must(err)
}
