package persistence

import (
	"context"
	"errors"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"testing"
)

func TestProductRegistrationArrivalUpdatesLinkedPurchase(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:registration_arrival?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		`CREATE TABLE purchase_slips(id TEXT PRIMARY KEY,organization_id TEXT,status TEXT,paid_at TEXT)`,
		`CREATE TABLE purchase_slip_lines(id TEXT PRIMARY KEY,purchase_slip_id TEXT)`,
		`CREATE TABLE products(id TEXT PRIMARY KEY,organization_id TEXT,product_code TEXT,purchase_slip_line_id TEXT,inventory_status TEXT,cost_adjustment_id TEXT,deleted_at DATETIME,updated_at DATETIME)`,
		`CREATE TABLE parts(id TEXT,purchase_slip_line_id TEXT,status TEXT)`,
		`CREATE TABLE inventory_events(id TEXT PRIMARY KEY,organization_id TEXT,product_id TEXT,event_type TEXT,from_status TEXT,to_status TEXT,reason TEXT,actor_user_id TEXT,created_at DATETIME)`,
		`INSERT INTO purchase_slips VALUES('purchase-1','org-1','confirmed','keep-payment')`,
		`INSERT INTO purchase_slip_lines VALUES('line-1','purchase-1'),('line-2','purchase-1')`,
		`INSERT INTO products(id,organization_id,product_code,purchase_slip_line_id,inventory_status) VALUES('product-1','org-1','REG-ONE','line-1','purchasing'),('product-2','org-1','REG-TWO','line-2','purchasing')`,
	} {
		if err = db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	repo := &Repository{db: db, driver: "sqlite"}
	assertCounts := func(pending, arrived int) {
		t.Helper()
		var counts struct {
			Pending int
			Arrived int
		}
		err = db.Table("purchase_slips AS p").Select("arrivals.pending_count AS pending,arrivals.arrived_count AS arrived").Joins(purchaseArrivalSummaryJoin).Where("p.id=?", "purchase-1").Scan(&counts).Error
		if err != nil {
			t.Fatal(err)
		}
		if counts.Pending != pending || counts.Arrived != arrived {
			t.Fatalf("counts=%+v, expected %d/%d", counts, pending, arrived)
		}
	}
	assertCounts(2, 0)
	for index, code := range []string{"REG-ONE", "REG-TWO"} {
		result, receiveErr := repo.ReceivePurchaseProduct(context.Background(), "org-1", "purchase-1", code, "user-1")
		if receiveErr != nil || result.Result != "received" || result.InventoryStatus != "in_stock" {
			t.Fatalf("result=%+v err=%v", result, receiveErr)
		}
		assertCounts(1-index, 1+index)
		var lineStatus string
		err = db.Table("purchase_slip_lines AS line").Select("product.inventory_status").Joins("JOIN products product ON product.purchase_slip_line_id=line.id").Where("line.purchase_slip_id=? AND product.product_code=?", "purchase-1", code).Scan(&lineStatus).Error
		if err != nil || lineStatus != "in_stock" {
			t.Fatalf("purchase line status=%s err=%v", lineStatus, err)
		}
	}
	repeated, err := repo.ReceivePurchaseProduct(context.Background(), "org-1", "purchase-1", "REG-ONE", "user-1")
	if err != nil || repeated.Result != "already_received" {
		t.Fatalf("retry=%+v err=%v", repeated, err)
	}
	var events int64
	db.Table("inventory_events").Count(&events)
	if events != 2 {
		t.Fatalf("duplicate arrival events: %d", events)
	}
	var paidAt string
	db.Table("purchase_slips").Select("paid_at").Scan(&paidAt)
	if paidAt != "keep-payment" {
		t.Fatal("arrival must not change payment")
	}
	_, err = repo.ReceivePurchaseProduct(context.Background(), "org-1", "wrong-purchase", "REG-ONE", "user-1")
	if !errors.Is(err, ErrPurchaseProductNotFound) {
		t.Fatalf("wrong-purchase err=%v", err)
	}
	if err = db.Exec("UPDATE products SET inventory_status='sold' WHERE product_code='REG-TWO'").Error; err != nil {
		t.Fatal(err)
	}
	_, err = repo.ReceivePurchaseProduct(context.Background(), "org-1", "purchase-1", "REG-TWO", "user-1")
	if !errors.Is(err, ErrPurchaseArrivalState) {
		t.Fatalf("sold item err=%v", err)
	}
}
