package persistence

import (
	"context"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestPurchaseReturnRevisionPersistsWithoutChangingInventory(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:purchase_return_revision?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		`CREATE TABLE return_slips(adjustment_details TEXT DEFAULT '{}',id TEXT PRIMARY KEY, organization_id TEXT,slip_number TEXT,operation_type TEXT,transaction_date DATETIME,buyer_role_id TEXT,supplier_role_id TEXT,source_purchase_slip_id TEXT,carrier TEXT,tracking_number TEXT,status TEXT,reason TEXT,notes TEXT,confirmed_at DATETIME,tracking_confirmed_at DATETIME,tracking_confirmed_by TEXT,created_at DATETIME,updated_at DATETIME)`,
		`CREATE TABLE return_lines(id TEXT,return_slip_id TEXT,line_number INTEGER,product_id TEXT,from_status TEXT,to_status TEXT)`,
		`CREATE TABLE partner_roles(id TEXT,partner_id TEXT,role_code TEXT)`,
		`CREATE TABLE business_partners(id TEXT,legal_name TEXT)`,
		`CREATE TABLE purchase_slips(id TEXT,slip_number TEXT)`,
		`CREATE TABLE products(id TEXT,product_code TEXT,brand TEXT,model_number TEXT,cost_amount_minor INTEGER,cost_currency TEXT,inventory_status TEXT)`,
		`INSERT INTO return_slips(id,organization_id,slip_number,operation_type,transaction_date,source_purchase_slip_id,carrier,tracking_number,status,reason,notes,tracking_confirmed_at,tracking_confirmed_by) VALUES('return','org','SH-2026-0001','purchase_return','2026-09-17','purchase','carrier','tracking','confirmed','reason','notes','2026-09-17 01:00:00','admin')`,
		`INSERT INTO return_lines VALUES('line','return',1,'product','in_stock','cancelled')`,
		`INSERT INTO purchase_slips VALUES('purchase','SD-2026-0001')`,
		`INSERT INTO products VALUES('product','1709260001','ROLEX','model',10000,'JPY','cancelled')`,
	} {
		if err = db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	repo := &Repository{db: db, driver: "sqlite"}
	input := PurchaseReturnRevisionInput{OrganizationID: "org", ActorUserID: "admin", TransactionDate: "2026-09-17", ReturnReason: "reason", Notes: "notes", Reason: "reason only"}
	_, err = repo.RevisePurchaseReturn(context.Background(), "return", input)
	if !errors.Is(err, ErrSlipRevisionUnchanged) {
		t.Fatalf("reason-only revision: %v", err)
	}
	input.Notes = " notes "
	_, err = repo.RevisePurchaseReturn(context.Background(), "return", input)
	if !errors.Is(err, ErrSlipRevisionUnchanged) {
		t.Fatalf("whitespace revision: %v", err)
	}
	input.TransactionDate = "2026-09-18"
	input.ReturnReason = "corrected return reason"
	input.Notes = "corrected notes"
	input.Reason = "date and notes corrected"
	record, err := repo.RevisePurchaseReturn(context.Background(), "return", input)
	if err != nil {
		t.Fatal(err)
	}
	if string(record.TransactionDate) != "2026-09-18" || record.Notes != "corrected notes\n修正理由: date and notes corrected" || record.Reason != input.ReturnReason {
		t.Fatalf("revision not persisted: %+v", record)
	}
	if record.Status != "confirmed" || record.TrackingNumber != "tracking" || record.TrackingConfirmedAt == nil || record.TrackingConfirmedBy != "admin" || record.PurchaseSlipID != "purchase" || len(record.Lines) != 1 || record.Lines[0].ProductID != "product" || record.Lines[0].CostAmount != 10000 {
		t.Fatalf("unrelated return data changed: %+v", record)
	}
	var status string
	if err = db.Table("products").Select("inventory_status").Scan(&status).Error; err != nil {
		t.Fatal(err)
	}
	if status != "cancelled" {
		t.Fatalf("inventory status changed: %s", status)
	}
	for _, tc := range []struct {
		name   string
		change func(*PurchaseReturnRevisionInput)
		want   error
	}{
		{"blank reason", func(i *PurchaseReturnRevisionInput) { i.Reason = " " }, ErrPurchaseReturnRevisionInvalid},
		{"blank notes", func(i *PurchaseReturnRevisionInput) { i.Notes = " " }, ErrPurchaseReturnRevisionInvalid},
		{"invalid date", func(i *PurchaseReturnRevisionInput) { i.TransactionDate = "2026-02-30" }, ErrPurchaseReturnRevisionInvalid},
		{"other organization", func(i *PurchaseReturnRevisionInput) { i.OrganizationID = "other" }, ErrReturnNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			copy := input
			tc.change(&copy)
			_, err := repo.RevisePurchaseReturn(context.Background(), "return", copy)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
		})
	}
	for _, update := range []map[string]any{{"status": "cancelled"}, {"status": "confirmed", "operation_type": "return"}} {
		if err = db.Table("return_slips").Where("id=?", "return").Updates(update).Error; err != nil {
			t.Fatal(err)
		}
		_, err = repo.RevisePurchaseReturn(context.Background(), "return", input)
		if !errors.Is(err, ErrReturnState) {
			t.Fatalf("invalid state: %v", err)
		}
	}
	var storedNotes string
	if err = db.Table("return_slips").Select("notes").Scan(&storedNotes).Error; err != nil {
		t.Fatal(err)
	}
	if storedNotes != record.Notes {
		t.Fatalf("rejected requests changed notes: %s", storedNotes)
	}
}
