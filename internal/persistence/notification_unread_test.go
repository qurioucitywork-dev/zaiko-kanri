package persistence

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

func TestUnreadNotificationTargetsPostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_NOTIFICATION_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated notification database required")
	}
	if !strings.Contains(dsn, "/zaiko_notification_test") {
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
	must(repo.SeedPreviewIdentity(ctx, "notification-test-admin", "notification-test-worker"))
	var org, admin, worker string
	must(repo.db.Raw("SELECT id FROM organizations WHERE code='PREVIEW'").Scan(&org).Error)
	must(repo.db.Raw("SELECT id FROM users WHERE organization_id=? AND role_key='admin' LIMIT 1", org).Scan(&admin).Error)
	must(repo.db.Raw("SELECT id FROM users WHERE organization_id=? AND role_key='worker' LIMIT 1", org).Scan(&worker).Error)
	now := time.Now().UTC()
	must(insertNotificationTx(repo.db, org, "", database.RoleAdmin, "purchase_request.created", "Purchase", "Guest / Product", "purchase_request", "req-target", now))
	must(insertNotificationTx(repo.db, org, "", database.RoleAdmin, "approval.requested", "Approval", "Worker approval", "approval_request", "apr-target", now.Add(time.Second)))
	for i := 0; i < 60; i++ {
		must(insertNotificationTx(repo.db, org, "", database.RoleAdmin, "old.read", "Old", "Old", "system", "old", now.Add(time.Minute)))
	}
	must(repo.db.Exec("UPDATE notifications SET read_at=now() WHERE event_key='old.read'").Error)
	items, err := repo.Notifications(ctx, org, admin, database.RoleAdmin, 50, true)
	must(err)
	if len(items) != 2 || items[0].TargetID != "apr-target" || items[1].TargetID != "req-target" {
		t.Fatalf("unread targets buried/misordered: %+v", items)
	}
	if err = repo.ReadNotification(ctx, org, worker, database.RoleWorker, items[0].ID); err == nil {
		t.Fatal("worker can mark administrator notification read")
	}
	hidden, err := repo.Notifications(ctx, org, worker, database.RoleWorker, 200, true)
	must(err)
	if len(hidden) != 0 {
		t.Fatal("admin notifications leaked to worker")
	}
	hidden, err = repo.Notifications(ctx, "other-org", admin, database.RoleAdmin, 200, true)
	must(err)
	if len(hidden) != 0 {
		t.Fatal("cross-organization notifications leaked")
	}
	must(repo.ReadNotification(ctx, org, admin, database.RoleAdmin, items[0].ID))
	fresh, err := Open(config.Config{DatabaseDriver: "postgres", DatabaseURL: dsn, Environment: "test"})
	must(err)
	defer fresh.Close()
	remaining, err := fresh.Notifications(ctx, org, admin, database.RoleAdmin, 200, true)
	must(err)
	if len(remaining) != 1 || remaining[0].TargetID != "req-target" {
		t.Fatal("read state did not persist")
	}
	must(repo.ReadNotification(ctx, org, admin, database.RoleAdmin, items[0].ID))
	all, err := repo.Notifications(ctx, org, admin, database.RoleAdmin, 200)
	must(err)
	if len(all) != 62 {
		t.Fatal("legacy notification listing lost records")
	}
}
