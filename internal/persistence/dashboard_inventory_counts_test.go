package persistence

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestDashboardInventoryCountersSingleQueryMatchesExistingFilters(t *testing.T) {
	logs := &identityReadLog{Interface: logger.Default.LogMode(logger.Silent)}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logs})
	if err != nil {
		t.Fatal(err)
	}
	conn, _ := db.DB()
	conn.SetMaxOpenConns(1)
	defer conn.Close()
	for _, sql := range []string{
		`CREATE TABLE products(id TEXT PRIMARY KEY,organization_id TEXT,inventory_status TEXT,inventory_valuation_excluded BOOLEAN,deleted_at DATETIME)`,
		`CREATE TABLE cost_adjustments(source_product_id TEXT,adjustment_type TEXT,status TEXT)`,
		`INSERT INTO products VALUES
  ('purchasing','org','purchasing',FALSE,NULL),
  ('stock','org','in_stock',FALSE,NULL),
  ('reserved','org','reserved',FALSE,NULL),
  ('return','org','return_pending',FALSE,NULL),
  ('sold','org','sold',FALSE,NULL),
  ('null-status','org',NULL,FALSE,NULL),
  ('unknown-status','org','unknown',FALSE,NULL),
  ('excluded','org','in_stock',TRUE,NULL),
  ('null-flag','org','purchasing',NULL,NULL),
  ('deleted','org','in_stock',FALSE,CURRENT_TIMESTAMP),
  ('foreign','other','in_stock',FALSE,NULL),
  ('split-confirmed','org','in_stock',FALSE,NULL),
  ('split-pending','org','reserved',FALSE,NULL),
  ('adjusted','org','in_stock',FALSE,NULL),
  ('split-duplicate','org','purchasing',FALSE,NULL)`,
		`INSERT INTO cost_adjustments VALUES
  ('split-confirmed','breakdown','confirmed'),
  ('split-pending','breakdown','pending'),
  ('adjusted','direct','confirmed'),
  ('split-duplicate','breakdown','confirmed'),
  ('split-duplicate','breakdown','confirmed')`,
	} {
		if err := db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	logs.reset()
	var actual Dashboard
	if err := loadDashboardInventoryCounts(db, "org", &actual); err != nil {
		t.Fatal(err)
	}
	if logs.count() != 1 {
		t.Fatalf("inventory summary made %d queries, want 1", logs.count())
	}
	if actual.TotalProducts != 6 || actual.PurchasingProducts != 1 || actual.InStockProducts != 2 || actual.ReservedProducts != 2 {
		t.Fatalf("wrong eligible counts: total=%d purchasing=%d inStock=%d reserved=%d", actual.TotalProducts, actual.PurchasingProducts, actual.InStockProducts, actual.ReservedProducts)
	}
	// Compare each previous independent query, including SQL NULL handling.
	for _, check := range []struct {
		condition string
		actual    int64
	}{
		{"inventory_status IN ('purchasing','in_stock','reserved','return_pending')", actual.TotalProducts},
		{"inventory_status='purchasing'", actual.PurchasingProducts},
		{"inventory_status='in_stock'", actual.InStockProducts},
		{"inventory_status='reserved'", actual.ReservedProducts},
	} {
		var expected int64
		err := db.Table("products").Where(`organization_id=? AND deleted_at IS NULL AND inventory_valuation_excluded=FALSE
  AND NOT EXISTS(SELECT 1 FROM cost_adjustments ca WHERE ca.source_product_id=products.id AND ca.adjustment_type='breakdown' AND ca.status='confirmed')`, "org").Where(check.condition).Count(&expected).Error
		if err != nil {
			t.Fatal(err)
		}
		if check.actual != expected {
			t.Fatalf("%s: combined=%d previous=%d", check.condition, check.actual, expected)
		}
	}
	var empty Dashboard
	if err := loadDashboardInventoryCounts(db, "no-such-organization", &empty); err != nil {
		t.Fatal(err)
	}
	if empty.TotalProducts != 0 || empty.PurchasingProducts != 0 || empty.InStockProducts != 0 || empty.ReservedProducts != 0 {
		t.Fatal("empty organization must have zero counts")
	}
}
