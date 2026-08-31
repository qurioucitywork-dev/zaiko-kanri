package persistence

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestReceivePurchaseProductCompletesCostAdjustmentArrival(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:purchase_arrival_cost_adjustment?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE purchase_slips (id TEXT PRIMARY KEY, organization_id TEXT NOT NULL, status TEXT NOT NULL)`,
		`CREATE TABLE purchase_slip_lines (id TEXT PRIMARY KEY, purchase_slip_id TEXT NOT NULL)`,
		`CREATE TABLE products (
			id TEXT PRIMARY KEY, organization_id TEXT NOT NULL, product_code TEXT NOT NULL,
			purchase_slip_line_id TEXT NOT NULL, inventory_status TEXT NOT NULL,
			cost_adjustment_id TEXT, deleted_at DATETIME, updated_at DATETIME
		)`,
		`CREATE TABLE cost_adjustment_items (cost_adjustment_id TEXT, output_product_id TEXT, status TEXT)`,
		`CREATE TABLE inventory_events (
			id TEXT PRIMARY KEY, organization_id TEXT NOT NULL, product_id TEXT NOT NULL,
			event_type TEXT NOT NULL, from_status TEXT, to_status TEXT, reason TEXT,
			actor_user_id TEXT, created_at DATETIME
		)`,
		`INSERT INTO purchase_slips(id,organization_id,status) VALUES('purchase-1','org-1','confirmed')`,
		`INSERT INTO purchase_slip_lines(id,purchase_slip_id) VALUES('line-1','purchase-1')`,
		`INSERT INTO products(id,organization_id,product_code,purchase_slip_line_id,inventory_status,cost_adjustment_id)
			VALUES('product-1','org-1','3108269001','line-1','cost_adjustment','adjustment-1')`,
		`INSERT INTO cost_adjustment_items(cost_adjustment_id,output_product_id,status)
			VALUES('adjustment-1','product-1','cost_adjustment')`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("setup failed for %q: %v", statement, err)
		}
	}

	repo := &Repository{db: db, driver: "sqlite"}
	result, err := repo.ReceivePurchaseProduct(context.Background(), "org-1", "purchase-1", "3108269001", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if result.Result != "received" || result.InventoryStatus != "in_stock" {
		t.Fatalf("unexpected arrival result: %#v", result)
	}

	var productStatus, itemStatus, eventFrom string
	if err := db.Raw(`SELECT inventory_status FROM products WHERE id='product-1'`).Scan(&productStatus).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(`SELECT status FROM cost_adjustment_items WHERE output_product_id='product-1'`).Scan(&itemStatus).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw(`SELECT from_status FROM inventory_events WHERE product_id='product-1'`).Scan(&eventFrom).Error; err != nil {
		t.Fatal(err)
	}
	if productStatus != "in_stock" || itemStatus != "completed" || eventFrom != "cost_adjustment" {
		t.Fatalf("arrival state mismatch: product=%q item=%q event_from=%q", productStatus, itemStatus, eventFrom)
	}

	second, err := repo.ReceivePurchaseProduct(context.Background(), "org-1", "purchase-1", "3108269001", "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if second.Result != "already_received" {
		t.Fatalf("second scan result=%q, want already_received", second.Result)
	}
}
