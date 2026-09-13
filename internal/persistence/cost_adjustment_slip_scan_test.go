package persistence

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestScanCostAdjustmentProductUsesSlipCostForStatus(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:cost_adjustment_slip_scan?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE products (id TEXT PRIMARY KEY, organization_id TEXT, product_code TEXT, inventory_status TEXT, cost_adjustment_id TEXT, inventory_valuation_excluded BOOLEAN DEFAULT FALSE, inventory_valuation_exclusion_reason TEXT DEFAULT '', inventory_valuation_excluded_at DATETIME, deleted_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE cost_adjustments (id TEXT PRIMARY KEY, organization_id TEXT, slip_number TEXT, adjustment_type TEXT, source_product_id TEXT, status TEXT)`,
		`CREATE TABLE direct_cost_adjustment_lines (id TEXT PRIMARY KEY, cost_adjustment_id TEXT, product_id TEXT, adjusted_cost_jpy_minor INTEGER)`,
		`CREATE TABLE cost_adjustment_items (id TEXT PRIMARY KEY, cost_adjustment_id TEXT, output_product_id TEXT, item_kind TEXT, allocated_cost_jpy_minor INTEGER, status TEXT)`,
		`CREATE TABLE product_cost_merges (id TEXT PRIMARY KEY, organization_id TEXT, adjustment_id TEXT)`,
		`CREATE TABLE product_cost_merge_inputs (merge_id TEXT, product_id TEXT)`,
		`CREATE TABLE inventory_events (id TEXT PRIMARY KEY, organization_id TEXT, product_id TEXT, event_type TEXT, from_status TEXT, to_status TEXT, reason TEXT, actor_user_id TEXT, created_at DATETIME)`,
		`INSERT INTO cost_adjustments VALUES('cad-1','org-1','CA-2026-0001','direct_decrease','p-zero','confirmed')`,
		`INSERT INTO products(id,organization_id,product_code,inventory_status,cost_adjustment_id) VALUES('p-zero','org-1','ZERO-1','cost_adjustment','cad-1'),('p-value','org-1','VALUE-1','cost_adjustment','cad-1')`,
		`INSERT INTO direct_cost_adjustment_lines VALUES('dcl-1','cad-1','p-zero',0),('dcl-2','cad-1','p-value',125000)`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("setup %q: %v", statement, err)
		}
	}
	repo := &Repository{db: db, driver: "sqlite"}
	zero, err := repo.ScanCostAdjustmentProduct(context.Background(), "org-1", "cad-1", "ZERO-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if zero.InventoryStatus != "cost_adjusted" || zero.SlipCostJPY != 0 {
		t.Fatalf("zero result=%#v", zero)
	}
	value, err := repo.ScanCostAdjustmentProduct(context.Background(), "org-1", "cad-1", "VALUE-1", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if value.InventoryStatus != "in_stock" || value.SlipCostJPY != 125000 {
		t.Fatalf("value result=%#v", value)
	}
	var rows []struct {
		ProductCode, InventoryStatus string
		InventoryValuationExcluded   bool
	}
	if err := db.Raw(`SELECT product_code,inventory_status,inventory_valuation_excluded FROM products ORDER BY product_code`).Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].InventoryStatus != "in_stock" || rows[0].InventoryValuationExcluded || rows[1].InventoryStatus != "cost_adjusted" || !rows[1].InventoryValuationExcluded {
		t.Fatalf("settled products=%#v", rows)
	}
}
