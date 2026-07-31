package database

import (
	"context"
	"database/sql"
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
	var beforeCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM products WHERE organization_id='org_preview'`).Scan(&beforeCount); err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migration must be idempotent: %v", err)
	}
	after, err := store.Products(ctx, "org_preview", ProductFilter{})
	if err != nil || len(after) != beforeCount {
		t.Fatalf("products before=%d after=%d err=%v", beforeCount, len(after), err)
	}
	versions, err := store.MigrationVersions(ctx)
	if err != nil || len(versions) != 31 || versions[30] != "000031_purchase_request_shipment_link" {
		t.Fatalf("versions=%v err=%v", versions, err)
	}
}

func TestPhase8RateMigrationUpgradesReferencedLegacySnapshots(t *testing.T) {
	store, err := Open("file:" + filepath.Join(t.TempDir(), "phase7-rates.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `
		CREATE TABLE schema_migrations(version TEXT PRIMARY KEY,applied_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	migrations := []struct{ version, path string }{
		{"000001_phase1", "migrations/000001_phase1.up.sql"},
		{"000002_inventory", "migrations/000002_inventory.up.sql"},
		{"000003_market", "migrations/000003_market.up.sql"},
		{"000004_sales_shipments", "migrations/000004_sales_shipments.up.sql"},
		{"000005_requests_reservations", "migrations/000005_requests_reservations.up.sql"},
		{"000006_approvals", "migrations/000006_approvals.up.sql"},
		{"000007_masters", "migrations/000007_masters.up.sql"},
		{"000008_stocktakes", "migrations/000008_stocktakes.up.sql"},
		{"000009_returns", "migrations/000009_returns.up.sql"},
		{"000010_purchase_sale_price", "migrations/000010_purchase_sale_price.up.sql"},
		{"000011_product_registration_details", "migrations/000011_product_registration_details.up.sql"},
		{"000012_purchase_returns", "migrations/000012_purchase_returns.up.sql"},
		{"000013_purchase_slip_workflow", "migrations/000013_purchase_slip_workflow.up.sql"},
		{"000014_shipment_slip_workflow", "migrations/000014_shipment_slip_workflow.up.sql"},
		{"000015_sales_slip_workflow", "migrations/000015_sales_slip_workflow.up.sql"},
		{"000016_sales_return_invoice", "migrations/000016_sales_return_invoice.up.sql"},
		{"000017_purchase_return_invoice", "migrations/000017_purchase_return_invoice.up.sql"},
		{"000018_purchase_line_product_details", "migrations/000018_purchase_line_product_details.up.sql"},
		{"000019_return_inventory_restore", "migrations/000019_return_inventory_restore.up.sql"},
	}
	for _, migration := range migrations {
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
	var productID string
	if err := store.db.QueryRowContext(ctx,
		`SELECT id FROM products WHERE organization_id='org_preview' ORDER BY id LIMIT 1`).Scan(&productID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO exchange_rate_snapshots(
			id,organization_id,base_currency,quote_currency,rate_scaled,scale,
			provider,observed_at,created_by,created_at
		) VALUES
			('rate_keep','org_preview','USD','JPY',15000000000,100000000,'test','2026-07-01T00:00:00Z','usr_admin','2026-07-01T00:00:00Z'),
			('rate_legacy','org_preview','JPY','USD',666667,100000000,'test','2026-07-01T00:00:00Z','usr_admin','2026-07-01T00:00:00Z');
		INSERT INTO sales_slips(
			id,organization_id,slip_number,sales_date,customer_name,status,created_by,created_at,updated_at
		) VALUES(
			'sale_upgrade','org_preview','SL-UPGRADE','2026-07-01','移行テスト','draft',
			'usr_admin','2026-07-01T00:00:00Z','2026-07-01T00:00:00Z'
		);
		INSERT INTO sales_lines(
			id,organization_id,sales_slip_id,line_number,product_id,quantity,
			unit_price_minor,sale_currency,exchange_rate_snapshot_id,
			exchange_rate_scaled,exchange_rate_scale,exchange_rate_observed_at,
			converted_unit_price_jpy,converted_total_jpy,created_at
		) VALUES(
			'line_upgrade','org_preview','sale_upgrade',1,?,1,1000,'USD','rate_legacy',
			666667,100000000,'2026-07-01T00:00:00Z',150000,150000,'2026-07-01T00:00:00Z'
		)`, productID); err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("upgrade referenced exchange-rate snapshot: %v", err)
	}
	var snapshotID sql.NullString
	var capturedRate int64
	if err := store.db.QueryRowContext(ctx, `
		SELECT exchange_rate_snapshot_id,exchange_rate_scaled
		FROM sales_lines WHERE id='line_upgrade'`).Scan(&snapshotID, &capturedRate); err != nil {
		t.Fatal(err)
	}
	if snapshotID.Valid || capturedRate != 666667 {
		t.Fatalf("snapshot=%v captured_rate=%d", snapshotID, capturedRate)
	}
	var violations int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_foreign_key_check`).Scan(&violations); err != nil {
		t.Fatal(err)
	}
	if violations != 0 {
		t.Fatalf("foreign-key violations=%d", violations)
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
