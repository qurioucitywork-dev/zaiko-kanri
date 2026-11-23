package persistence

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestCostAdjustmentSlipProcessingFollowsProductScans(t *testing.T) {
	for _, kind := range []string{"breakdown", "combine", "direct_increase", "direct_decrease"} {
		t.Run(kind, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open("file:cost_processing_"+kind+"?mode=memory&cache=shared"), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			sqlDB, _ := db.DB()
			defer sqlDB.Close()
			for _, sql := range []string{
				`CREATE TABLE products(id TEXT PRIMARY KEY,organization_id TEXT,product_code TEXT,brand TEXT,model_number TEXT,inventory_status TEXT,cost_adjustment_id TEXT,inventory_valuation_excluded BOOLEAN DEFAULT FALSE,inventory_valuation_exclusion_reason TEXT,inventory_valuation_excluded_at DATETIME,deleted_at DATETIME,updated_at DATETIME)`,
				`CREATE TABLE cost_adjustments(id TEXT PRIMARY KEY,organization_id TEXT,slip_number TEXT,adjustment_type TEXT,adjustment_date DATE,source_product_id TEXT,source_product_code TEXT,source_cost_jpy_minor INTEGER,allocated_cost_jpy_minor INTEGER,status TEXT,notes TEXT,confirmed_at DATETIME)`,
				`CREATE TABLE cost_adjustment_items(id TEXT PRIMARY KEY,cost_adjustment_id TEXT,position INTEGER,item_kind TEXT,output_product_id TEXT,management_code TEXT,allocated_cost_jpy_minor INTEGER,status TEXT)`,
				`CREATE TABLE direct_cost_adjustment_lines(id TEXT PRIMARY KEY,cost_adjustment_id TEXT,position INTEGER,product_id TEXT,product_code TEXT,previous_cost_jpy_minor INTEGER,adjustment_amount_jpy_minor INTEGER,adjusted_cost_jpy_minor INTEGER,direction TEXT,notes TEXT)`,
				`CREATE TABLE product_cost_merges(id TEXT PRIMARY KEY,organization_id TEXT,adjustment_id TEXT)`,
				`CREATE TABLE product_cost_merge_inputs(merge_id TEXT,product_id TEXT)`,
				`CREATE TABLE inventory_events(id TEXT PRIMARY KEY,organization_id TEXT,product_id TEXT,event_type TEXT,from_status TEXT,to_status TEXT,reason TEXT,actor_user_id TEXT,created_at DATETIME)`,
				`INSERT INTO products(id,organization_id,product_code,inventory_status,cost_adjustment_id) VALUES('source','org','SOURCE','cost_adjustment','cad'),('output','org','OUTPUT','cost_adjustment','cad'),('unrelated','org','UNRELATED','cost_adjustment','another'),('foreign','other-org','FOREIGN','cost_adjustment','cad')`,
			} {
				if err := db.Exec(sql).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Exec(`INSERT INTO cost_adjustments VALUES('cad','org','GW-TEST',?,'2026-09-17','source','SOURCE',100,100,'confirmed','',?)`, kind, time.Now().UTC()).Error; err != nil {
				t.Fatal(err)
			}
			if kind == "breakdown" || kind == "combine" {
				if err := db.Exec(`INSERT INTO cost_adjustment_items VALUES('out-line','cad',1,'product','output','OUTPUT',100,'pending')`).Error; err != nil {
					t.Fatal(err)
				}
				if kind == "combine" {
					db.Exec(`INSERT INTO product_cost_merges VALUES('merge','org','cad')`)
					db.Exec(`INSERT INTO product_cost_merge_inputs VALUES('merge','source')`)
				}
			} else {
				if err := db.Exec(`INSERT INTO direct_cost_adjustment_lines VALUES('zero','cad',1,'source','SOURCE',100,-100,0,'decrease',''),('value','cad',2,'output','OUTPUT',50,50,100,'increase','')`).Error; err != nil {
					t.Fatal(err)
				}
			}
			ctx := context.Background()
			repo := &Repository{db: db, driver: "sqlite"}
			check := func(want string, count int64) {
				t.Helper()
				// Re-query persisted state, as a new screen/session does after every scan.
				rows, err := (&Repository{db: db, driver: "sqlite"}).ListCostAdjustmentSlips(ctx, "org", 200)
				if err != nil {
					t.Fatal(err)
				}
				if len(rows) != 1 {
					t.Fatalf("rows=%+v", rows)
				}
				if rows[0].ProcessingStatus != want || rows[0].PendingProductCount != count || rows[0].Status != "confirmed" {
					t.Fatalf("state=%+v want=%s pending=%d", rows[0], want, count)
				}
				if len(rows[0].Lines) != 2 {
					t.Fatalf("source/output lines missing: %+v", rows[0].Lines)
				}
			}
			check("processing", 2)
			output, err := repo.ScanCostAdjustmentProduct(ctx, "org", "cad", "OUTPUT", "admin")
			if err != nil || output.InventoryStatus != "in_stock" {
				t.Fatal(output, err)
			}
			check("processing", 1)
			again, err := repo.ScanCostAdjustmentProduct(ctx, "org", "cad", "OUTPUT", "admin")
			if err != nil || again.Result != "already_scanned" {
				t.Fatal(again, err)
			}
			check("processing", 1)
			source, err := repo.ScanCostAdjustmentProduct(ctx, "org", "cad", "SOURCE", "admin")
			if err != nil || source.InventoryStatus != "cost_adjusted" {
				t.Fatal(source, err)
			}
			check("completed", 0)
			if err := db.Table("products").Where("id=?", "output").Update("inventory_status", "sold").Error; err != nil {
				t.Fatal(err)
			}
			check("completed", 0)
			var events int64
			db.Table("inventory_events").Count(&events)
			if events != 2 {
				t.Fatal(fmt.Sprintf("duplicate scan added events: %d", events))
			}
		})
	}
}
