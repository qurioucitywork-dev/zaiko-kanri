package persistence

import (
	"context"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestPurchaseReturnProductEditLock(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:return_edit_lock?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		`CREATE TABLE purchase_slips(id TEXT,organization_id TEXT,slip_number TEXT,purchase_date DATETIME,status TEXT,supplier_role_id TEXT,purchase_staff_profile_id TEXT,notes TEXT,updated_at DATETIME)`,
		`CREATE TABLE purchase_slip_lines(id TEXT,purchase_slip_id TEXT,line_number INTEGER,sku TEXT,unit_cost_minor INTEGER,cost_currency TEXT,converted_total_jpy INTEGER,base_sale_price_minor INTEGER)`,
		`CREATE TABLE partner_roles(id TEXT,organization_id TEXT,role_type TEXT,role_code TEXT,is_active BOOLEAN)`,
		`CREATE TABLE return_slips(id TEXT,organization_id TEXT,operation_type TEXT,status TEXT)`,
		`CREATE TABLE return_lines(return_slip_id TEXT,product_id TEXT)`,
		`CREATE TABLE products(id TEXT,organization_id TEXT,product_code TEXT,purchase_slip_line_id TEXT,inventory_status TEXT,sku TEXT,cost_amount_minor INTEGER,base_sale_price_minor INTEGER,internal_comment TEXT,cost_adjustment_id TEXT,deleted_at DATETIME,updated_at DATETIME)`,
		`CREATE TABLE product_files(id TEXT,organization_id TEXT,product_id TEXT,storage_driver TEXT,object_key TEXT,original_name TEXT,content_type TEXT,size_bytes INTEGER,sha256 TEXT,sort_order INTEGER,created_at DATETIME)`,
		`INSERT INTO purchase_slips VALUES('purchase','org','SD-TEST','2026-09-17','confirmed',NULL,NULL,'original',NULL)`,
		`INSERT INTO purchase_slip_lines VALUES('l1','purchase',1,'SKU1',1000,'JPY',1000,10),('l2','purchase',2,'SKU2',2000,'JPY',2000,20)`,
		`INSERT INTO products VALUES('p1','org','CODE1','l1','return_pending','SKU1',1000,10,'keep comment','',NULL,NULL),('p2','org','CODE2','l2','in_stock','SKU2',2000,20,'normal','',NULL,NULL)`,
		`INSERT INTO return_slips VALUES('return','org','purchase_return','confirmed')`,
		`INSERT INTO return_lines VALUES('return','p1')`,
		`INSERT INTO product_files(id,organization_id,product_id,sort_order) VALUES('file','org','p1',0)`,
	} {
		if err = db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err = db.Exec(`ALTER TABLE purchase_slips ADD COLUMN paid_at DATETIME`).Error; err != nil {
		t.Fatal(err)
	}
	for _, column := range []string{"supplier_role_id", "purchase_staff_profile_id", "cost_currency"} {
		if err := db.Exec("ALTER TABLE products ADD COLUMN " + column + " TEXT").Error; err != nil {
			t.Fatal(err)
		}
	}
	repo := &Repository{db: db, driver: "sqlite"}
	ctx := context.Background()
	base := func() PurchaseRevisionInput {
		return PurchaseRevisionInput{OrganizationID: "org", ActorUserID: "admin", PurchaseDate: "2026-09-17", Notes: "original", Reason: "change", Lines: []PurchaseRevisionLine{{ID: "l1", SKU: "SKU1", UnitCostMinor: 1000, BaseSalePriceMinor: 10}, {ID: "l2", SKU: "SKU2", UnitCostMinor: 2000, BaseSalePriceMinor: 20}}}
	}
	for _, field := range []string{"sku", "cost", "sale"} {
		input := base()
		switch field {
		case "sku":
			input.Lines[0].SKU = "changed"
		case "cost":
			input.Lines[0].UnitCostMinor++
		case "sale":
			input.Lines[0].BaseSalePriceMinor++
		}
		_, err = repo.RevisePurchase(ctx, "purchase", input)
		if !errors.Is(err, ErrPurchaseReturnProductLocked) {
			t.Fatalf("%s edit not blocked: %v", field, err)
		}
	}
	model := "changed"
	_, err = repo.UpdateProduct(ctx, "org", "p1", "admin", ProductUpdateInput{ModelNumber: &model})
	if !errors.Is(err, ErrPurchaseReturnProductLocked) {
		t.Fatalf("direct edit not blocked: %v", err)
	}
	if _, err = repo.PrepareProductFile(ctx, "org", "p1"); !errors.Is(err, ErrPurchaseReturnProductLocked) {
		t.Fatal("file prepare", err)
	}
	if _, err = repo.CreateProductFile(ctx, "org", "admin", ProductFileRecord{ID: "new-file", ProductID: "p1"}); !errors.Is(err, ErrPurchaseReturnProductLocked) {
		t.Fatal("file create", err)
	}
	if _, err = repo.DeleteProductFile(ctx, "org", "file"); !errors.Is(err, ErrPurchaseReturnProductLocked) {
		t.Fatal("file delete", err)
	}
	if _, err = repo.ReorderProductFiles(ctx, "org", "p1", []string{"file"}); !errors.Is(err, ErrPurchaseReturnProductLocked) {
		t.Fatal("file reorder", err)
	}
	var notes string
	db.Table("purchase_slips").Select("notes").Scan(&notes)
	if notes != "original" {
		t.Fatal("rejected edit changed header", notes)
	}
	// Observe the allowed second line inside the transaction, then roll back deliberately.
	observed := errors.New("verified other line update")
	verified := false
	db.Callback().Update().After("gorm:update").Register("test:observe_unlocked_product", func(tx *gorm.DB) {
		if tx.Statement.Table != "products" {
			return
		}
		var rows []struct {
			ID, SKU, InternalComment string
			CostAmountMinor          int64
		}
		if err := tx.Session(&gorm.Session{NewDB: true}).Table("products").Order("id").Find(&rows).Error; err != nil {
			tx.AddError(err)
			return
		}
		if len(rows) != 2 || rows[0].SKU != "SKU1" || rows[0].CostAmountMinor != 1000 || rows[0].InternalComment != "keep comment" || rows[1].CostAmountMinor != 2001 {
			t.Errorf("locked product changed or other edit missing: %+v", rows)
		}
		verified = true
		tx.AddError(observed)
	})
	input := base()
	input.Lines[1].UnitCostMinor = 2001
	_, err = repo.RevisePurchase(ctx, "purchase", input)
	if !errors.Is(err, observed) || !verified {
		t.Fatalf("other line blocked: %v", err)
	}
	db.Callback().Update().Remove("test:observe_unlocked_product")
	if err = db.Table("products").Where("id=?", "p1").Update("inventory_status", "purchasing").Error; err != nil {
		t.Fatal(err)
	}
	if err = checkPurchaseReturnEditLock(db, "org", "p1", "purchasing"); err != nil {
		t.Fatal("withdrawn product remains locked", err)
	}
	db.Table("return_slips").Where("id=?", "return").Update("operation_type", "return")
	if err = checkPurchaseReturnEditLock(db, "org", "p1", "return_pending"); err != nil {
		t.Fatal("sales return incorrectly locked", err)
	}
}
