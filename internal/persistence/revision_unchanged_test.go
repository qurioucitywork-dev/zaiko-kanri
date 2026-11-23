package persistence

import (
	"context"
	"errors"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"testing"
)

func TestSlipRevisionsRequireBusinessChanges(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:revision_unchanged?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		`CREATE TABLE partner_roles(id TEXT,organization_id TEXT,role_type TEXT,role_code TEXT,is_active BOOLEAN)`,
		`CREATE TABLE staff_profiles(id TEXT,organization_id TEXT,staff_code TEXT,user_id TEXT)`,
		`INSERT INTO partner_roles VALUES('supplier','org','supplier','0001',1),('buyer','org','buyer','0002',1)`,
		`INSERT INTO staff_profiles VALUES('staff','org','0003','user')`,
		`CREATE TABLE purchase_slips(id TEXT,organization_id TEXT,slip_number TEXT,purchase_date DATETIME,status TEXT,supplier_role_id TEXT,purchase_staff_profile_id TEXT,notes TEXT,updated_at DATETIME)`,
		`INSERT INTO purchase_slips VALUES('purchase','org','SD-TEST','2026-09-17','confirmed','supplier','staff','original',NULL)`,
		`CREATE TABLE purchase_slip_lines(id TEXT,purchase_slip_id TEXT,line_number INTEGER,sku TEXT,unit_cost_minor INTEGER,cost_currency TEXT,converted_total_jpy INTEGER,base_sale_price_minor INTEGER)`,
		`INSERT INTO purchase_slip_lines VALUES('purchase-line','purchase',1,'SKU-1',1000,'JPY',1000,0)`,
		`CREATE TABLE sales_slips(id TEXT,organization_id TEXT,slip_number TEXT,sale_date DATETIME,status TEXT,display_currency TEXT,tax_mode TEXT,tax_rate_basis_points INTEGER,buyer_role_id TEXT,notes TEXT,updated_at DATETIME)`,
		`INSERT INTO sales_slips VALUES('sale','org','UD-TEST','2026-09-17','confirmed','JPY','taxable',1000,'buyer','original',NULL)`,
		`CREATE TABLE sales_lines(id TEXT,sales_slip_id TEXT,line_number INTEGER,product_id TEXT,sale_currency TEXT,fx_rate_snapshot_id TEXT,fx_rate_scaled INTEGER,fx_scale INTEGER,unit_price_minor INTEGER)`,
		`INSERT INTO sales_lines VALUES('sale-line','sale',1,'product','JPY','rate',1,1,5000)`,
	} {
		if err = db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err = db.Exec(`ALTER TABLE purchase_slips ADD COLUMN paid_at DATETIME`).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`ALTER TABLE sales_slips ADD COLUMN paid_at DATETIME`).Error; err != nil {
		t.Fatal(err)
	}
	repo := &Repository{db: db, driver: "sqlite"}
	purchase := PurchaseRevisionInput{OrganizationID: "org", ActorUserID: "user", PurchaseDate: "2026-09-17", SupplierCode: "0001", StaffCode: "0003", Notes: "original", Reason: "reason only", Lines: []PurchaseRevisionLine{{ID: "purchase-line", SKU: "SKU-1", UnitCostMinor: 1000, BaseSalePriceMinor: 0}}}
	sale := SaleRevisionInput{OrganizationID: "org", ActorUserID: "user", SaleDate: "2026-09-17", BuyerCode: "0002", Notes: "original", Reason: "reason only", Lines: []SaleRevisionLine{{ID: "sale-line", UnitPriceMinor: 5000}}}
	// A callback detects any attempted writes and rolls them back before touching records.
	attemptedWrite := errors.New("test detected business update")
	writes := 0
	if err = db.Callback().Update().Before("gorm:update").Register("test:detect_revision_write", func(tx *gorm.DB) { writes++; tx.AddError(attemptedWrite) }); err != nil {
		t.Fatal(err)
	}
	for _, padded := range []bool{false, true} {
		if padded {
			purchase.Notes = " original "
			purchase.Lines[0].SKU = " SKU-1 "
			sale.Notes = " original "
		}
		_, err = repo.RevisePurchase(context.Background(), "purchase", purchase)
		if !errors.Is(err, ErrSlipRevisionUnchanged) {
			t.Fatalf("purchase no-op=%v", err)
		}
		_, err = repo.ReviseSale(context.Background(), "sale", sale)
		if !errors.Is(err, ErrSlipRevisionUnchanged) {
			t.Fatalf("sale no-op=%v", err)
		}
	}
	if writes != 0 {
		t.Fatalf("no-op revisions attempted %d writes", writes)
	}
	purchase.Lines[0].UnitCostMinor = 1001
	_, err = repo.RevisePurchase(context.Background(), "purchase", purchase)
	if !errors.Is(err, attemptedWrite) {
		t.Fatalf("changed purchase did not pass guard: %v", err)
	}
	sale.Lines[0].UnitPriceMinor = 5001
	_, err = repo.ReviseSale(context.Background(), "sale", sale)
	if !errors.Is(err, attemptedWrite) {
		t.Fatalf("changed sale did not pass guard: %v", err)
	}
	if writes != 2 {
		t.Fatalf("changed revision writes=%d", writes)
	}

	if err = db.Exec("UPDATE sales_slips SET paid_at='2026-09-17 12:00:00'").Error; err != nil {
		t.Fatal(err)
	}
	if _, err = repo.ReviseSale(context.Background(), "sale", sale); !errors.Is(err, ErrSaleAlreadyPaid) {
		t.Fatalf("paid sale revision: %v", err)
	}
	if writes != 2 {
		t.Fatalf("paid sale attempted writes: %d", writes)
	}
	// Simulate payment after the editor was opened. A paid slip must reject
	// amount and non-amount changes without attempting a business write.
	if err = db.Exec("UPDATE purchase_slips SET paid_at='2026-09-17 12:00:00'").Error; err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"cost", "sale", "sku", "notes", "supplier"} {
		edited := purchase
		edited.Lines = append([]PurchaseRevisionLine(nil), purchase.Lines...)
		switch field {
		case "cost":
			edited.Lines[0].UnitCostMinor = 9999
		case "sale":
			edited.Lines[0].BaseSalePriceMinor = 9999
		case "sku":
			edited.Lines[0].SKU = "changed"
		case "notes":
			edited.Notes = "changed"
		case "supplier":
			edited.SupplierCode = ""
		}
		if _, err = repo.RevisePurchase(context.Background(), "purchase", edited); !errors.Is(err, ErrPurchaseAlreadyPaid) {
			t.Fatalf("paid %s revision: %v", field, err)
		}
	}
	if writes != 2 {
		t.Fatalf("paid revision attempted writes: %d", writes)
	}
	var cost int64
	if err = db.Table("purchase_slip_lines").Select("unit_cost_minor").Scan(&cost).Error; err != nil {
		t.Fatal(err)
	}
	if cost != 1000 {
		t.Fatalf("paid line changed: %d", cost)
	}
	for _, table := range []string{"purchase_slips", "sales_slips"} {
		var notes string
		if err = db.Table(table).Select("notes").Scan(&notes).Error; err != nil {
			t.Fatal(err)
		}
		if notes != "original" {
			t.Fatalf("%s notes changed: %s", table, notes)
		}
	}
}
