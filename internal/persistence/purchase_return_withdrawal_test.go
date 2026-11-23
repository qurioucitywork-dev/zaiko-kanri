package persistence

import (
	"context"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func withdrawalTestRepository(t *testing.T, productStatus string) *Repository {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		`CREATE TABLE return_slips(adjustment_details TEXT DEFAULT '{}',id TEXT PRIMARY KEY,organization_id TEXT,slip_number TEXT,operation_type TEXT,transaction_date DATETIME,buyer_role_id TEXT,supplier_role_id TEXT,source_purchase_slip_id TEXT,carrier TEXT,tracking_number TEXT,status TEXT,reason TEXT,notes TEXT,confirmed_at DATETIME,tracking_confirmed_at DATETIME,tracking_confirmed_by TEXT,created_at DATETIME,updated_at DATETIME)`,
		`CREATE TABLE return_lines(id TEXT,return_slip_id TEXT,line_number INTEGER,product_id TEXT,from_status TEXT,to_status TEXT)`,
		`CREATE TABLE partner_roles(id TEXT,partner_id TEXT,role_code TEXT)`,
		`CREATE TABLE business_partners(id TEXT,legal_name TEXT)`,
		`CREATE TABLE purchase_slips(id TEXT,slip_number TEXT,status TEXT,paid_at TEXT)`,
		`CREATE TABLE purchase_slip_lines(id TEXT,purchase_slip_id TEXT)`,
		`CREATE TABLE parts(id TEXT,purchase_slip_line_id TEXT,status TEXT)`,
		`CREATE TABLE products(id TEXT,organization_id TEXT,product_code TEXT,brand TEXT,model_number TEXT,cost_amount_minor INTEGER,cost_currency TEXT,inventory_status TEXT,purchase_slip_line_id TEXT,cost_adjustment_id TEXT,cancelled_at DATETIME,cancelled_by TEXT,cancel_reason TEXT,deleted_at DATETIME,updated_at DATETIME)`,
		`CREATE TABLE inventory_events(id TEXT,organization_id TEXT,product_id TEXT,event_type TEXT,from_status TEXT,to_status TEXT,reason TEXT,actor_user_id TEXT,created_at DATETIME)`,
		`CREATE TABLE approval_requests(id TEXT,organization_id TEXT,target_type TEXT,target_id TEXT,status TEXT,decided_by TEXT,decided_at DATETIME,decision_note TEXT,updated_at DATETIME)`,
		`CREATE TABLE approval_actions(id TEXT,approval_request_id TEXT,actor_user_id TEXT,action TEXT,note TEXT,created_at DATETIME)`,
		`INSERT INTO return_slips(id,organization_id,slip_number,operation_type,transaction_date,source_purchase_slip_id,carrier,tracking_number,status,reason,notes) VALUES('ret','org','SH-TEST','purchase_return','2026-09-17','purchase','carrier','TRACK','confirmed','reason','notes')`,
		`INSERT INTO return_lines VALUES('rl1','ret',1,'p1','purchasing','cancelled'),('rl2','ret',2,'p2','in_stock','cancelled')`,
		`INSERT INTO purchase_slips VALUES('purchase','SD-TEST','confirmed','keep-paid')`,
		`INSERT INTO purchase_slip_lines VALUES('pl1','purchase'),('pl2','purchase')`,
		`INSERT INTO products(id,organization_id,product_code,brand,model_number,cost_amount_minor,cost_currency,purchase_slip_line_id) VALUES('p1','org','CODE-1','ROLEX','model',1000,'JPY','pl1'),('p2','org','CODE-2','ROLEX','model',2000,'JPY','pl2')`,
		`INSERT INTO approval_requests(id,organization_id,target_type,target_id,status) VALUES('approval','org','return_slip','ret','pending')`,
	} {
		if err = db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	updates := map[string]any{"inventory_status": productStatus}
	if productStatus == "cancelled" {
		updates["cancelled_at"] = "2026-09-17 01:00:00"
		updates["cancelled_by"] = "admin"
		updates["cancel_reason"] = "tracking"
		if err = db.Exec(`UPDATE return_slips SET tracking_confirmed_at='2026-09-17 01:00:00',tracking_confirmed_by='admin'`).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err = db.Table("products").Where("organization_id=?", "org").Updates(updates).Error; err != nil {
		t.Fatal(err)
	}
	return &Repository{db: db, driver: "sqlite"}
}

func TestPurchaseReturnWithdrawal(t *testing.T) {
	for _, state := range []string{"return_pending", "cancelled"} {
		t.Run(state, func(t *testing.T) {
			repo := withdrawalTestRepository(t, state)
			record, err := repo.WithdrawPurchaseReturn(context.Background(), "org", "ret", "admin")
			if err != nil {
				t.Fatal(err)
			}
			if record.Status != "cancelled" || record.TrackingNumber != "TRACK" || record.Notes != "notes" || len(record.Lines) != 2 {
				t.Fatalf("unexpected return %+v", record)
			}
			var products []struct {
				InventoryStatus, CancelledBy, CancelReason string
				CancelledAt                                *string
			}
			if err = repo.db.Table("products").Find(&products).Error; err != nil {
				t.Fatal(err)
			}
			for _, p := range products {
				if p.InventoryStatus != "purchasing" || p.CancelledAt != nil || p.CancelledBy != "" || p.CancelReason != "" {
					t.Fatalf("product not restored %+v", p)
				}
			}
			var counts struct{ PendingCount, ArrivedCount int }
			if err = repo.db.Table("purchase_slips AS p").Select("arrivals.pending_count,arrivals.arrived_count").Joins(purchaseArrivalSummaryJoin).Where("p.id=?", "purchase").Scan(&counts).Error; err != nil {
				t.Fatal(err)
			}
			if counts.PendingCount != 2 || counts.ArrivedCount != 0 {
				t.Fatalf("source purchase not linked %+v", counts)
			}
			var paid, approval string
			repo.db.Table("purchase_slips").Select("paid_at").Scan(&paid)
			repo.db.Table("approval_requests").Select("status").Scan(&approval)
			if paid != "keep-paid" || approval != "cancelled" {
				t.Fatalf("paid=%s approval=%s", paid, approval)
			}
			var events int64
			repo.db.Table("inventory_events").Where("event_type=? AND from_status=? AND to_status=?", "purchase_return_withdrawn", state, "purchasing").Count(&events)
			if events != 2 {
				t.Fatalf("events=%d", events)
			}
			// Duplicate withdrawal must not revert later receiving or create more history.
			repo.db.Table("products").Where("id=?", "p1").Update("inventory_status", "in_stock")
			if _, err = repo.WithdrawPurchaseReturn(context.Background(), "org", "ret", "admin"); err != nil {
				t.Fatal(err)
			}
			var latest string
			repo.db.Table("products").Select("inventory_status").Where("id=?", "p1").Scan(&latest)
			repo.db.Table("inventory_events").Count(&events)
			if latest != "in_stock" || events != 2 {
				t.Fatalf("duplicate changed data status=%s events=%d", latest, events)
			}
		})
	}
	t.Run("rollback", func(t *testing.T) {
		repo := withdrawalTestRepository(t, "return_pending")
		repo.db.Table("products").Where("id=?", "p2").Update("inventory_status", "sold")
		_, err := repo.WithdrawPurchaseReturn(context.Background(), "org", "ret", "admin")
		if !errors.Is(err, ErrReturnState) {
			t.Fatalf("error=%v", err)
		}
		var product, slip, approval string
		var events int64
		repo.db.Table("products").Select("inventory_status").Where("id=?", "p1").Scan(&product)
		repo.db.Table("return_slips").Select("status").Scan(&slip)
		repo.db.Table("approval_requests").Select("status").Scan(&approval)
		repo.db.Table("inventory_events").Count(&events)
		if product != "return_pending" || slip != "confirmed" || approval != "pending" || events != 0 {
			t.Fatalf("partial write: %s %s %s %d", product, slip, approval, events)
		}
	})
	t.Run("organization_and_type", func(t *testing.T) {
		repo := withdrawalTestRepository(t, "return_pending")
		_, err := repo.WithdrawPurchaseReturn(context.Background(), "other", "ret", "admin")
		if !errors.Is(err, ErrReturnNotFound) {
			t.Fatalf("cross org=%v", err)
		}
		repo.db.Table("return_slips").Where("id=?", "ret").Update("operation_type", "return")
		_, err = repo.WithdrawPurchaseReturn(context.Background(), "org", "ret", "admin")
		if !errors.Is(err, ErrReturnState) {
			t.Fatalf("wrong type=%v", err)
		}
	})
}
