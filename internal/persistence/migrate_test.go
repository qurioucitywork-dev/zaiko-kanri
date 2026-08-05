package persistence

import (
	"strings"
	"testing"
)

func TestMigrationCatalogIsOrderedAndChecksummed(t *testing.T) {
	migrations, err := migrationCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) == 0 {
		t.Fatal("migration catalog is empty")
	}
	for index, migration := range migrations {
		if migration.Version == "" || migration.Checksum == "" || migration.SQL == "" {
			t.Fatalf("incomplete migration: %#v", migration)
		}
		if index > 0 && migrations[index-1].Version >= migration.Version {
			t.Fatalf("migrations are not ordered: %s then %s", migrations[index-1].Version, migration.Version)
		}
	}
}

func TestPlatformMigrationContainsCriticalTables(t *testing.T) {
	migrations, err := migrationCatalog()
	if err != nil {
		t.Fatal(err)
	}
	sql := migrations[0].SQL
	for _, table := range []string{
		"organizations", "organization_profiles", "organization_bank_accounts", "users",
		"staff_profiles", "sessions", "organization_settings", "audit_logs",
	} {
		if !strings.Contains(sql, "CREATE TABLE IF NOT EXISTS "+table) {
			t.Fatalf("critical table %s is missing", table)
		}
	}
	if !strings.Contains(sql, "prevent_audit_log_mutation") {
		t.Fatal("audit immutability trigger is missing")
	}
}

func TestDocumentOperationsMigrationKeepsTrackingAndHistory(t *testing.T) {
	migrations, err := migrationCatalog()
	if err != nil {
		t.Fatal(err)
	}
	var sql string
	for _, migration := range migrations {
		if migration.Version == "000016_document_operations" {
			sql = migration.SQL
			break
		}
	}
	if sql == "" {
		t.Fatal("document operations migration 000016 is missing")
	}
	for _, fragment := range []string{"tracking_number", "carrier", "CREATE TABLE IF NOT EXISTS document_generation_events", "storage_driver", "object_key"} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("document operations migration is missing %q", fragment)
		}
	}
}

func TestPurchaseDateConsistencyMigrationGuardsSlipProducts(t *testing.T) {
	migrations, err := migrationCatalog()
	if err != nil {
		t.Fatal(err)
	}
	var sql string
	for _, migration := range migrations {
		if migration.Version == "000017_purchase_date_consistency" {
			sql = migration.SQL
			break
		}
	}
	if sql == "" {
		t.Fatal("purchase date consistency migration 000017 is missing")
	}
	for _, fragment := range []string{
		"UPDATE products AS product",
		"enforce_product_purchase_date_consistency",
		"product purchase date must match its purchase slip date",
		"product code date prefix must match its purchase slip date",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("purchase date consistency migration is missing %q", fragment)
		}
	}
}

func TestUniquePurchaseItemsMigrationSplitsBulkLines(t *testing.T) {
	migrations, err := migrationCatalog()
	if err != nil {
		t.Fatal(err)
	}
	var sql string
	for _, migration := range migrations {
		if migration.Version == "000018_unique_purchase_items" {
			sql = migration.SQL
			break
		}
	}
	if sql == "" {
		t.Fatal("unique purchase items migration 000018 is missing")
	}
	for _, fragment := range []string{
		"pul_preview_pi0003_line3",
		"Tank Must Large",
		"Tank Must Small",
		"purchase_slip_lines_single_item_chk CHECK (quantity=1)",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("unique purchase items migration is missing %q", fragment)
		}
	}
}
