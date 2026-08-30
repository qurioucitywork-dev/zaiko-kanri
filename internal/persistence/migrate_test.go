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

func TestMarketResearchCurrencyRateMigrationPersistsHistoricalRate(t *testing.T) {
	migrations, err := migrationCatalog()
	if err != nil {
		t.Fatal(err)
	}
	var sql string
	for _, migration := range migrations {
		if migration.Version == "000056_market_research_currency_rate" {
			sql = migration.SQL
			break
		}
	}
	if sql == "" {
		t.Fatal("market research currency/rate migration 000056 is missing")
	}
	for _, fragment := range []string{"market_fx_rate_scaled", "market_fx_scale", "'HKD'", "exchange_rate_snapshots"} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("market research currency/rate migration is missing %q", fragment)
		}
	}
}

func TestBeltAndDialMasterMigrationCreatesStableCatalogs(t *testing.T) {
	migrations, err := migrationCatalog()
	if err != nil {
		t.Fatal(err)
	}
	var sql string
	for _, migration := range migrations {
		if migration.Version == "000032_belt_dial_masters" {
			sql = migration.SQL
			break
		}
	}
	if sql == "" {
		t.Fatal("belt and dial master migration 000032 is missing")
	}
	for _, fragment := range []string{"CREATE TABLE IF NOT EXISTS belt_materials", "CREATE TABLE IF NOT EXISTS dials", "UNIQUE (organization_id, code)"} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("belt/dial master migration is missing %q", fragment)
		}
	}
	for _, kind := range []string{"belt-materials", "dials"} {
		if _, ok := masterTable(kind); !ok {
			t.Fatalf("master kind %q is not registered", kind)
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

func TestPurchaseInventoryConsistencyMigrationEnforcesOneToOneLink(t *testing.T) {
	migrations, err := migrationCatalog()
	if err != nil {
		t.Fatal(err)
	}
	var sql string
	for _, migration := range migrations {
		if migration.Version == "000036_purchase_inventory_consistency" {
			sql = migration.SQL
			break
		}
	}
	if sql == "" {
		t.Fatal("purchase inventory consistency migration 000036 is missing")
	}
	for _, fragment := range []string{
		"idx_products_one_per_purchase_line",
		"assert_purchase_inventory_consistency",
		"purchase_slips_inventory_consistency",
		"purchase_lines_inventory_consistency",
		"products_purchase_inventory_consistency",
		"expected_count <> actual_count",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("purchase inventory consistency migration is missing %q", fragment)
		}
	}
}

func TestPurchaseTaxModeMigrationFixesTaxSnapshot(t *testing.T) {
	migrations, err := migrationCatalog()
	if err != nil {
		t.Fatal(err)
	}
	var sql string
	for _, migration := range migrations {
		if migration.Version == "000019_purchase_tax_mode" {
			sql = migration.SQL
			break
		}
	}
	if sql == "" {
		t.Fatal("purchase tax mode migration 000019 is missing")
	}
	for _, fragment := range []string{"purchase_tax_mode", "tax_rate_basis_points", "domestic", "overseas"} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("purchase tax mode migration is missing %q", fragment)
		}
	}
}

func TestPurchaseIssueMigrationPersistsTimestampAndIssuer(t *testing.T) {
	migrations, err := migrationCatalog()
	if err != nil {
		t.Fatal(err)
	}
	var sql string
	for _, migration := range migrations {
		if migration.Version == "000021_purchase_issue_audit" {
			sql = migration.SQL
			break
		}
	}
	if sql == "" {
		t.Fatal("purchase issue migration 000021 is missing")
	}
	for _, fragment := range []string{"issued_at", "issued_by", "idx_purchase_slips_issued_at"} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("purchase issue migration is missing %q", fragment)
		}
	}
}

func TestPurchaseDateFXSnapshotMigrationUsesHistoricalRate(t *testing.T) {
	migrations, err := migrationCatalog()
	if err != nil {
		t.Fatal(err)
	}
	var sql string
	for _, migration := range migrations {
		if migration.Version == "000022_purchase_date_fx_snapshot" {
			sql = migration.SQL
			break
		}
	}
	if sql == "" {
		t.Fatal("purchase date FX snapshot migration 000022 is missing")
	}
	for _, fragment := range []string{
		"snapshot.observed_at < slip.purchase_date + INTERVAL '1 day'",
		"converted_total_jpy",
		"fx_rate_snapshot_id",
		"fx_rate_scaled",
		"fx_scale",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("purchase date FX snapshot migration is missing %q", fragment)
		}
	}
}

func TestPurchaseIssueFXSnapshotMigrationUsesIssuanceRate(t *testing.T) {
	migrations, err := migrationCatalog()
	if err != nil {
		t.Fatal(err)
	}
	var sql string
	for _, migration := range migrations {
		if migration.Version == "000024_purchase_issue_fx_snapshot" {
			sql = migration.SQL
			break
		}
	}
	if sql == "" {
		t.Fatal("purchase issue FX snapshot migration 000024 is missing")
	}
	for _, fragment := range []string{
		"snapshot.observed_at <= slip.issued_at",
		"issue_fx_rate_snapshot_id",
		"issue_fx_rate_scaled",
		"issue_fx_scale",
		"idx_purchase_slips_issue_fx_rate",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("purchase issue FX snapshot migration is missing %q", fragment)
		}
	}
}

func TestPurchaseRegistrationFXSnapshotMigrationUsesRegistrationTime(t *testing.T) {
	migrations, err := migrationCatalog()
	if err != nil {
		t.Fatal(err)
	}
	var sql string
	for _, migration := range migrations {
		if migration.Version == "000029_purchase_registration_fx_snapshot" {
			sql = migration.SQL
			break
		}
	}
	if sql == "" {
		t.Fatal("purchase registration FX snapshot migration 000029 is missing")
	}
	for _, fragment := range []string{
		"snapshot.observed_at <= slip.created_at",
		"slip.purchase_tax_mode = 'overseas'",
		"converted_total_jpy",
		"fx_rate_snapshot_id",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("purchase registration FX snapshot migration is missing %q", fragment)
		}
	}
}

func TestSalesIssueMigrationPersistsTimestampAndIssuer(t *testing.T) {
	migrations, err := migrationCatalog()
	if err != nil {
		t.Fatal(err)
	}
	var sql string
	for _, migration := range migrations {
		if migration.Version == "000025_sales_issue_audit" {
			sql = migration.SQL
			break
		}
	}
	if sql == "" {
		t.Fatal("sales issue migration 000025 is missing")
	}
	for _, fragment := range []string{"issued_at", "issued_by", "idx_sales_slips_issued_at"} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("sales issue migration is missing %q", fragment)
		}
	}
}

func TestSalesTaxMigrationSeparatesUSDOutOfScopeFromJPYExemption(t *testing.T) {
	migrations, err := migrationCatalog()
	if err != nil {
		t.Fatal(err)
	}
	var sql string
	for _, migration := range migrations {
		if migration.Version == "000026_sales_tax_out_of_scope" {
			sql = migration.SQL
			break
		}
	}
	if sql == "" {
		t.Fatal("sales tax migration 000026 is missing")
	}
	for _, fragment := range []string{
		"'out_of_scope'",
		"WHERE display_currency = 'USD'",
		"tax_rate_basis_points = 0",
		"sales_slips_tax_mode_chk",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("sales tax migration is missing %q", fragment)
		}
	}
}

func TestSalesOutOfScopeTotalsMigrationRemovesLegacyUSDTax(t *testing.T) {
	migrations, err := migrationCatalog()
	if err != nil {
		t.Fatal(err)
	}
	var sql string
	for _, migration := range migrations {
		if migration.Version == "000027_sales_out_of_scope_totals" {
			sql = migration.SQL
			break
		}
	}
	if sql == "" {
		t.Fatal("sales out-of-scope totals migration 000027 is missing")
	}
	for _, fragment := range []string{
		"UPDATE sales_lines AS line",
		"tax_amount_minor = 0",
		"total_minor = line.subtotal_minor",
		"slip.display_currency = 'USD'",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("sales out-of-scope totals migration is missing %q", fragment)
		}
	}
}

func TestShipmentJPYThousandRoundingMigrationRepairsExistingLines(t *testing.T) {
	migrations, err := migrationCatalog()
	if err != nil {
		t.Fatal(err)
	}
	var sql string
	for _, migration := range migrations {
		if migration.Version == "000028_shipment_jpy_thousand_rounding" {
			sql = migration.SQL
			break
		}
	}
	if sql == "" {
		t.Fatal("shipment JPY rounding migration 000028 is missing")
	}
	for _, fragment := range []string{
		"UPDATE shipment_lines AS line",
		"/ 1000",
		")::bigint * 1000",
		"shipment.fx_rate_scaled",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("shipment JPY rounding migration is missing %q", fragment)
		}
	}
}

func TestConsignmentFinancialSnapshotMigrationPersistsRegistrationAndIssueEvidence(t *testing.T) {
	migrations, err := migrationCatalog()
	if err != nil {
		t.Fatal(err)
	}
	var sql string
	for _, migration := range migrations {
		if migration.Version == "000033_consignment_financial_issue_snapshot" {
			sql = migration.SQL
			break
		}
	}
	if sql == "" {
		t.Fatal("consignment financial snapshot migration 000033 is missing")
	}
	for _, fragment := range []string{
		"display_currency", "fx_rate_snapshot_id", "fx_rate_scaled", "fx_scale",
		"sale_price_usd_minor", "converted_sale_price_jpy", "issued_at", "issued_by",
		"UPDATE consignment_lines", "candidate.observed_at <= c.created_at",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("consignment financial snapshot migration is missing %q", fragment)
		}
	}
}

func TestUnifiedBusinessPartnersMigrationAddsClassificationFields(t *testing.T) {
	migrations, err := migrationCatalog()
	if err != nil {
		t.Fatal(err)
	}
	var sql string
	for _, migration := range migrations {
		if migration.Version == "000037_unify_business_partners" {
			sql = migration.SQL
			break
		}
	}
	if sql == "" {
		t.Fatal("unified business partners migration 000037 is missing")
	}
	for _, fragment := range []string{"region_type", "closing_day", "is_other", "domestic", "overseas"} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("unified business partners migration is missing %q", fragment)
		}
	}
}

func TestPartnerContactDetailsMigrationAddsOperationalFields(t *testing.T) {
	migrations, err := migrationCatalog()
	if err != nil {
		t.Fatal(err)
	}
	var sql string
	for _, migration := range migrations {
		if migration.Version == "000038_partner_contact_details" {
			sql = migration.SQL
			break
		}
	}
	if sql == "" {
		t.Fatal("partner contact details migration 000038 is missing")
	}
	for _, fragment := range []string{"contact_phone", "antique_license_number", "LPAD", "^T[0-9]{13}$"} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("partner contact details migration is missing %q", fragment)
		}
	}
}

func TestProductCodeDDMMYYMigrationRenumbersProductsAndReferences(t *testing.T) {
	migrations, err := migrationCatalog()
	if err != nil {
		t.Fatal(err)
	}
	var sql string
	for _, migration := range migrations {
		if migration.Version == "000049_product_code_ddmmyy_sequence" {
			sql = migration.SQL
			break
		}
	}
	if sql == "" {
		t.Fatal("product code DDMMYY migration 000049 is missing")
	}
	for _, fragment := range []string{
		"product_code_migration_history",
		"TO_CHAR(business_date, 'DDMMYY')",
		"LPAD(sequence::TEXT, 4, '0')",
		"UPDATE stocktake_lines AS line",
		"CHECK (product_code ~ '^[0-9]{10}$')",
		"LEFT(NEW.product_code, 6) <> TO_CHAR(source_purchase_date, 'DDMMYY')",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("product code DDMMYY migration is missing %q", fragment)
		}
	}
}

func TestPersonalPurchaseTaxModeMigrationAllowsThreePurchaseCategories(t *testing.T) {
	migrations, err := migrationCatalog()
	if err != nil {
		t.Fatal(err)
	}
	var sql string
	for _, migration := range migrations {
		if migration.Version == "000052_personal_purchase_tax_mode" {
			sql = migration.SQL
			break
		}
	}
	if sql == "" {
		t.Fatal("personal purchase tax mode migration 000052 is missing")
	}
	for _, fragment := range []string{"'domestic', 'personal', 'overseas'", "purchase_tax_mode IN ('personal', 'overseas')", "tax_rate_basis_points = 0"} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("personal purchase tax mode migration is missing %q", fragment)
		}
	}
}

func TestPurchasePaymentMethodMigrationAddsValidatedDefault(t *testing.T) {
	migrations, err := migrationCatalog()
	if err != nil {
		t.Fatal(err)
	}
	var sql string
	for _, migration := range migrations {
		if migration.Version == "000053_purchase_payment_method" {
			sql = migration.SQL
			break
		}
	}
	if sql == "" {
		t.Fatal("purchase payment method migration 000053 is missing")
	}
	for _, fragment := range []string{"payment_method", "DEFAULT 'bank_transfer'", "'cash', 'bank_transfer', 'card'"} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("purchase payment method migration is missing %q", fragment)
		}
	}
}

func TestPurchaseTaxCategoryMigrationAddsThreeValidatedChoices(t *testing.T) {
	migrations, err := migrationCatalog()
	if err != nil {
		t.Fatal(err)
	}
	var sql string
	for _, migration := range migrations {
		if migration.Version == "000054_purchase_tax_category" {
			sql = migration.SQL
			break
		}
	}
	if sql == "" {
		t.Fatal("purchase tax category migration 000054 is missing")
	}
	for _, fragment := range []string{"tax_category", "consumption_tax", "tax_equivalent", "out_of_scope"} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("purchase tax category migration is missing %q", fragment)
		}
	}
}

func TestPersonalPurchaseTemporarySupplierMigrationAddsSlipOnlyName(t *testing.T) {
	migrations, err := migrationCatalog()
	if err != nil {
		t.Fatal(err)
	}
	var sql string
	for _, migration := range migrations {
		if migration.Version == "000057_personal_purchase_temporary_supplier" {
			sql = migration.SQL
			break
		}
	}
	if sql == "" {
		t.Fatal("personal purchase temporary supplier migration 000057 is missing")
	}
	for _, fragment := range []string{"purchase_slips", "supplier_name_text", "VARCHAR(200)", "DEFAULT ''"} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("personal purchase temporary supplier migration is missing %q", fragment)
		}
	}
}
