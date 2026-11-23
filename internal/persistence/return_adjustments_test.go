package persistence

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestReturnAdjustmentValidation(t *testing.T) {
	good := ReturnCreateInput{OperationType: "return", TransactionDate: "2026-10-13", AdjustmentLines: []ReturnAdjustmentLine{{"送料", 1000}, {"値引き", -500}}}
	if !validReturnAdjustments(good) {
		t.Fatal("signed financial-only lines rejected")
	}
	for _, change := range []func(*ReturnCreateInput){
		func(v *ReturnCreateInput) { v.ProductCodes = []string{"99999"} },
		func(v *ReturnCreateInput) { v.OperationType = "takeout" },
		func(v *ReturnCreateInput) { v.AdjustmentLines = []ReturnAdjustmentLine{{" ", 1}} },
		func(v *ReturnCreateInput) { v.AdjustmentLines = []ReturnAdjustmentLine{{"送料", 0}} },
		func(v *ReturnCreateInput) { v.AdjustmentLines = []ReturnAdjustmentLine{{"送料", 1000000000001}} },
	} {
		input := good
		change(&input)
		if _, err := (&Repository{}).CreateReturn(context.Background(), input); !errors.Is(err, ErrReturnState) {
			t.Fatalf("invalid input accepted: %v", err)
		}
	}
}

// Runs only against a disposable, explicitly named test database.
func TestReturnAdjustmentInventoryIsolation(t *testing.T) {
	dsn := os.Getenv("ZAIKO_RETURN_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires disposable zaiko_return_test_* database")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(parsed.Path, "/zaiko_return_test_") {
		t.Fatal("refusing non-test database")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal("open test database")
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	for _, sql := range []string{
		`CREATE TABLE return_slips(id TEXT PRIMARY KEY,organization_id TEXT,slip_number TEXT,operation_type TEXT,transaction_date DATE,buyer_role_id TEXT,supplier_role_id TEXT,source_purchase_slip_id TEXT,carrier TEXT,tracking_number TEXT,status TEXT,reason TEXT,notes TEXT,confirmed_at TIMESTAMPTZ,confirmed_by TEXT,tracking_confirmed_at TIMESTAMPTZ,tracking_confirmed_by TEXT,created_by TEXT,created_at TIMESTAMPTZ,updated_at TIMESTAMPTZ)`,
		`ALTER TABLE return_slips ADD COLUMN adjustment_details JSONB NOT NULL DEFAULT '{}'`,
		`CREATE TABLE return_lines(id TEXT,return_slip_id TEXT,line_number INTEGER,product_id TEXT,from_status TEXT,to_status TEXT,created_at TIMESTAMPTZ)`,
		`CREATE TABLE partner_roles(id TEXT,organization_id TEXT,role_type TEXT,partner_id TEXT,role_code TEXT,is_active BOOLEAN)`,
		`CREATE TABLE business_partners(id TEXT,legal_name TEXT)`,
		`CREATE TABLE purchase_slips(id TEXT,organization_id TEXT,slip_number TEXT,supplier_role_id TEXT,status TEXT,tax_category TEXT,tax_rate_basis_points INTEGER)`,
		`CREATE TABLE purchase_slip_lines(id TEXT,purchase_slip_id TEXT,line_number INTEGER,cost_currency TEXT,fx_rate_scaled BIGINT,fx_scale BIGINT)`,
		`CREATE TABLE sales_slips(id TEXT,organization_id TEXT,slip_number TEXT,buyer_role_id TEXT,status TEXT,display_currency TEXT,tax_mode TEXT,tax_rate_basis_points INTEGER,fx_rate_scaled BIGINT,fx_scale BIGINT)`,
		`CREATE TABLE products(id TEXT,organization_id TEXT,product_code TEXT,brand TEXT,model_number TEXT,cost_amount_minor BIGINT,cost_currency TEXT,inventory_status TEXT,purchase_slip_line_id TEXT,cancelled_at TIMESTAMPTZ,cancelled_by TEXT,cancel_reason TEXT,deleted_at TIMESTAMPTZ,updated_at TIMESTAMPTZ)`,
		`CREATE TABLE inventory_events(id TEXT,organization_id TEXT,product_id TEXT,event_type TEXT,from_status TEXT,to_status TEXT,reason TEXT,actor_user_id TEXT,created_at TIMESTAMPTZ)`,
		`CREATE TABLE approval_requests(id TEXT,organization_id TEXT,target_type TEXT,target_id TEXT,status TEXT)`,
		`CREATE TABLE daily_document_sequences(organization_id TEXT,prefix TEXT,business_date DATE,last_sequence INTEGER,updated_at TIMESTAMPTZ,PRIMARY KEY(organization_id,prefix,business_date))`,
		`INSERT INTO partner_roles VALUES('sr','org','supplier','partner','S',true),('br','org','buyer','partner','B',true)`,
		`INSERT INTO business_partners VALUES('partner','テスト会社')`,
		`INSERT INTO purchase_slips VALUES('purchase','org','SD-TEST','sr','confirmed','consumption_tax',1000)`,
		`INSERT INTO purchase_slip_lines VALUES('pl','purchase',1,'JPY',1,1)`,
		`INSERT INTO sales_slips VALUES('sale','org','UD-TEST','br','confirmed','USD','out_of_scope',0,155,1)`,
		`INSERT INTO products(id,organization_id,product_code,brand,model_number,cost_amount_minor,cost_currency,inventory_status,purchase_slip_line_id,updated_at) VALUES('p','org','PRODUCT','ROLEX','model',480000,'JPY','in_stock','pl','2026-10-01'),('reserved-code','org','99999','unchanged','unchanged',123,'JPY','sold','pl','2026-10-01')`,
	} {
		if err := db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	repo := &Repository{db: db, driver: "postgres"}
	ctx := context.Background()
	var before string
	db.Raw(`SELECT json_agg(p ORDER BY id)::text FROM products p`).Scan(&before)
	for _, kind := range []string{"purchase_return", "return"} {
		input := ReturnCreateInput{OrganizationID: "org", ActorUserID: "admin", OperationType: kind, TransactionDate: "2026-10-13", SupplierCode: "S", PurchaseSlipNo: "SD-TEST", BuyerCode: "B", SourceSaleSlipNumber: "UD-TEST", Notes: "テスト", AdjustmentLines: []ReturnAdjustmentLine{{"送料", 1000}, {"値引き", -300}}}
		record, err := repo.CreateReturn(ctx, input)
		if err != nil {
			t.Fatal(err)
		}
		if len(record.Lines) != 0 || len(record.AdjustmentDetails.Lines) != 2 || record.AdjustmentDetails.Lines[1].AmountMinor != -300 {
			t.Fatalf("wrong lines: %+v", record)
		}
		if kind == "return" && (record.AdjustmentDetails.Currency != "USD" || record.AdjustmentDetails.FXRateScaled != 155 || record.AdjustmentDetails.TaxMode != "out_of_scope") {
			t.Fatal("source sale snapshot not retained")
		}
		if kind == "purchase_return" && (record.AdjustmentDetails.Currency != "JPY" || record.AdjustmentDetails.TaxRateBasisPoints != 1000 || record.AdjustmentDetails.TaxMode != "standard") {
			t.Fatal("source purchase tax not retained")
		}
		if _, err = repo.ConfirmReturn(ctx, "org", record.ID, "admin"); err != nil {
			t.Fatal(err)
		}
		if _, err = repo.UpdateReturnTracking(ctx, "org", record.ID, "admin", "", "TRACK", true); err != nil {
			t.Fatal(err)
		}
		if _, err = repo.UpdateReturnTracking(ctx, "org", record.ID, "admin", "", "TRACK", false); err != nil {
			t.Fatal(err)
		}
		if kind == "purchase_return" {
			if _, err = repo.WithdrawPurchaseReturn(ctx, "org", record.ID, "admin"); err != nil {
				t.Fatal(err)
			}
		}
		if _, err = repo.ReturnSlip(ctx, "other", record.ID); !errors.Is(err, ErrReturnNotFound) {
			t.Fatal("cross-organization access")
		}
		var after string
		db.Raw(`SELECT json_agg(p ORDER BY id)::text FROM products p`).Scan(&after)
		if before != after {
			t.Fatal("financial-only return changed inventory")
		}
		var events, lines int64
		db.Table("inventory_events").Count(&events)
		db.Table("return_lines").Count(&lines)
		if events != 0 || lines != 0 {
			t.Fatal("financial line generated inventory event or product link")
		}
	}
	// Product rows retain their normal transition alongside multiple 99999 rows.
	mixed, err := repo.CreateReturn(ctx, ReturnCreateInput{OrganizationID: "org", ActorUserID: "admin", OperationType: "purchase_return", TransactionDate: "2026-10-13", SupplierCode: "S", PurchaseSlipNo: "SD-TEST", Notes: "混在", ProductCodes: []string{"PRODUCT"}, AdjustmentLines: []ReturnAdjustmentLine{{"送料", 1000}, {"値引き", -300}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(mixed.Lines) != 1 || len(mixed.AdjustmentDetails.Lines) != 2 {
		t.Fatal("mixed lines lost")
	}
	if _, err = repo.ConfirmReturn(ctx, "org", mixed.ID, "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.UpdateReturnTracking(ctx, "org", mixed.ID, "admin", "", "TRACK", true); err != nil {
		t.Fatal(err)
	}
	var state string
	db.Table("products").Select("inventory_status").Where("product_code='PRODUCT'").Scan(&state)
	if state != "cancelled" {
		t.Fatal("normal product transition failed")
	}
	var cost int64
	db.Table("products").Select("cost_amount_minor").Where("product_code='PRODUCT'").Scan(&cost)
	if cost != 480000 {
		t.Fatal("adjustment changed product cost")
	}
}
