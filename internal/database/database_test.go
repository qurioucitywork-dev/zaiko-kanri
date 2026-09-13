package database

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open("file:" + t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedPreview(context.Background(), "preview-admin-2026", "preview-worker-2026"); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestAuthenticateAndPermissions(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	admin, err := store.Authenticate(ctx, "PREVIEW", "admin", "preview-admin-2026")
	if err != nil {
		t.Fatalf("authenticate admin: %v", err)
	}
	if !store.HasPermission(ctx, admin, "settings.manage") {
		t.Fatal("admin should manage settings")
	}

	worker, err := store.Authenticate(ctx, "PREVIEW", "worker", "preview-worker-2026")
	if err != nil {
		t.Fatalf("authenticate worker: %v", err)
	}
	if store.HasPermission(ctx, worker, "settings.manage") {
		t.Fatal("worker must not manage settings")
	}
	for _, permission := range []string{"purchase.confirm", "sales.confirm", "shipment.confirm", "market.write"} {
		if store.HasPermission(ctx, worker, permission) {
			t.Fatalf("worker must not have sensitive direct-mutation permission %q", permission)
		}
	}
	if _, err := store.Authenticate(ctx, "PREVIEW", "admin", "wrong-password"); err == nil {
		t.Fatal("invalid password must be rejected")
	}
}

func TestSessionExpiryAndCSRFHash(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	admin, _ := store.Authenticate(ctx, "PREVIEW", "admin", "preview-admin-2026")
	if err := store.CreateSession(ctx, admin, "session-token", "csrf-token", "", "", time.Hour); err != nil {
		t.Fatal(err)
	}
	session, err := store.Session(ctx, "session-token")
	if err != nil {
		t.Fatal(err)
	}
	if session.CSRFTokenHash != TokenHash("csrf-token") {
		t.Fatal("csrf token must be stored as a hash")
	}
	if _, err := store.Session(ctx, "different-token"); err == nil {
		t.Fatal("unknown session must be rejected")
	}
}

func TestOrganizationScopeOnUserQueries(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := store.db.ExecContext(ctx, `INSERT INTO organizations(id,code,name,created_at,updated_at) VALUES('org_other','OTHER','別組織',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	hash, _ := HashPassword("another-user-password")
	if _, err := store.db.ExecContext(ctx, `INSERT INTO users(id,organization_id,username,password_hash,display_name,role_key,created_at,updated_at) VALUES('usr_other','org_other','other',?,'別組織ユーザー','admin',?,?)`, hash, now, now); err != nil {
		t.Fatal(err)
	}
	users, err := store.Users(ctx, "org_preview")
	if err != nil {
		t.Fatal(err)
	}
	for _, user := range users {
		if user.OrganizationID != "org_preview" || user.Username == "other" {
			t.Fatalf("cross-organization user leaked: %+v", user)
		}
	}
}

func TestSettingsAndAuditAreOrganizationScoped(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	before, after, err := store.UpdateSetting(ctx, "org_preview", "usr_admin", "reservation.duration_hours", "48")
	if err != nil {
		t.Fatal(err)
	}
	if before.IsConfigured || !after.IsConfigured || after.Value != "48" {
		t.Fatalf("unexpected setting transition: before=%+v after=%+v", before, after)
	}
	if err := store.WriteAudit(ctx, AuditEntry{
		OrganizationID: "org_preview", ActorUserID: "usr_admin", TargetType: "setting",
		TargetID: after.Key, Action: "setting.updated", Result: "success", RequestID: "req_test",
	}); err != nil {
		t.Fatal(err)
	}
	entries, err := store.AuditLogs(ctx, "org_preview", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].RequestID != "req_test" {
		t.Fatalf("unexpected audit entries: %+v", entries)
	}
	if _, err := store.db.ExecContext(ctx, `DELETE FROM audit_logs WHERE id=?`, entries[0].ID); err == nil {
		t.Fatal("audit logs must be immutable")
	}
}

func TestBackupCreatesConsistentDatabaseCopy(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	if err := store.SeedInventoryPreview(ctx); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "backup.db")
	if err := store.Backup(ctx, target); err != nil {
		t.Fatal(err)
	}
	backup, err := Open(target)
	if err != nil {
		t.Fatal(err)
	}
	defer backup.Close()
	if err := backup.IntegrityCheck(ctx); err != nil {
		t.Fatal(err)
	}
	products, err := backup.Products(ctx, "org_preview", ProductFilter{})
	if err != nil || len(products) == 0 {
		t.Fatalf("backup products=%d err=%v", len(products), err)
	}
}

func TestMigrateExistingPhase5DatabasePreservesData(t *testing.T) {
	store, err := Open("file:" + filepath.Join(t.TempDir(), "phase5.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `
		CREATE TABLE schema_migrations(version TEXT PRIMARY KEY,applied_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	for _, migration := range []struct{ version, path string }{
		{"000001_phase1", "migrations/000001_phase1.up.sql"},
		{"000002_inventory", "migrations/000002_inventory.up.sql"},
		{"000003_market", "migrations/000003_market.up.sql"},
		{"000004_sales_shipments", "migrations/000004_sales_shipments.up.sql"},
		{"000005_requests_reservations", "migrations/000005_requests_reservations.up.sql"},
	} {
		schema, readErr := schemaFS.ReadFile(migration.path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if _, err := store.db.ExecContext(ctx, string(schema)); err != nil {
			t.Fatalf("apply %s: %v", migration.version, err)
		}
		if _, err := store.db.ExecContext(ctx,
			`INSERT INTO schema_migrations(version,applied_at) VALUES(?,?)`,
			migration.version, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.SeedPreview(ctx, "preview-admin-2026", "preview-worker-2026"); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedInventoryPreview(ctx); err != nil {
		t.Fatal(err)
	}
	before, err := store.Products(ctx, "org_preview", ProductFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migration must be idempotent: %v", err)
	}
	after, err := store.Products(ctx, "org_preview", ProductFilter{})
	if err != nil || len(after) != len(before) {
		t.Fatalf("products before=%d after=%d err=%v", len(before), len(after), err)
	}
	versions, err := store.MigrationVersions(ctx)
	if err != nil || len(versions) != 8 || versions[7] != "000008_product_files" {
		t.Fatalf("versions=%v err=%v", versions, err)
	}
}

func TestBootstrapOrganizationAdminCreatesProductionLoginAndCatalog(t *testing.T) {
	store, err := Open("file:" + filepath.Join(t.TempDir(), "production.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	user, err := store.BootstrapOrganizationAdmin(
		ctx, "watch", "ウォッチ株式会社", "release-admin", "初期管理者", "release-password-2026",
	)
	if err != nil {
		t.Fatal(err)
	}
	if user.Role != RoleAdmin || user.Organization == "" {
		t.Fatalf("user=%+v", user)
	}
	authenticated, err := store.Authenticate(ctx, "WATCH", "release-admin", "release-password-2026")
	if err != nil || authenticated.ID != user.ID {
		t.Fatalf("authenticated=%+v err=%v", authenticated, err)
	}
	if !store.HasPermission(ctx, authenticated, "settings.manage") ||
		!store.HasPermission(ctx, authenticated, "approval.approve") {
		t.Fatal("bootstrap admin permissions missing")
	}
	settings, err := store.Settings(ctx, user.OrganizationID)
	if err != nil || len(settings) != len(defaultOrganizationSettings()) {
		t.Fatalf("settings=%d err=%v", len(settings), err)
	}
	if _, err := store.BootstrapOrganizationAdmin(
		ctx, "WATCH", "重複", "other", "重複", "release-password-2026",
	); err == nil {
		t.Fatal("duplicate organization code must be rejected")
	}
}
