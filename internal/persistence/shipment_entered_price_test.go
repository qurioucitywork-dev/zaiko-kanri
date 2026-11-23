package persistence

import (
	"context"
	"errors"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"os"
	"testing"
)

func TestShipmentInputPrices(t *testing.T) {
	price := int64(750)
	zero := int64(0)
	negative := int64(-1)
	for _, tc := range []struct {
		name  string
		items []ShipmentPriceInput
		bad   bool
	}{
		{"legacy", nil, false},
		{"edited", []ShipmentPriceInput{{ProductCode: " a ", SalePriceUSDMinor: &price}}, false},
		{"empty", []ShipmentPriceInput{}, true},
		{"missing price", []ShipmentPriceInput{{ProductCode: "A"}}, true},
		{"unknown code", []ShipmentPriceInput{{ProductCode: "B", SalePriceUSDMinor: &price}}, true},
		{"zero", []ShipmentPriceInput{{ProductCode: "A", SalePriceUSDMinor: &zero}}, true},
		{"negative", []ShipmentPriceInput{{ProductCode: "A", SalePriceUSDMinor: &negative}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := shipmentInputPrices([]string{"A"}, tc.items)
			if tc.bad {
				if !errors.Is(err, ErrShipmentPrice) {
					t.Fatalf("error=%v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			} else if tc.items != nil && got["A"] != price {
				t.Fatalf("prices=%v", got)
			}
		})
	}
	if _, err := shipmentInputPrices([]string{"A", "B"}, []ShipmentPriceInput{{ProductCode: "A", SalePriceUSDMinor: &price}, {ProductCode: "a", SalePriceUSDMinor: &price}}); !errors.Is(err, ErrShipmentPrice) {
		t.Fatalf("duplicate error=%v", err)
	}
}

func TestShipmentEnteredPricePostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_SHIPMENT_PRICE_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL DSN required")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	// Transaction-local tables keep this test independent of application data.
	schema := []string{
		`CREATE TEMP TABLE business_partners(id text PRIMARY KEY,legal_name text,region_type text)`,
		`CREATE TEMP TABLE partner_roles(id text PRIMARY KEY,organization_id text,partner_id text,role_type text,role_code text,is_active boolean)`,
		`CREATE TEMP TABLE exchange_rate_snapshots(id text,organization_id text,base_currency text,quote_currency text,rate_scaled bigint,scale bigint,observed_at timestamptz,created_at timestamptz)`,
		`CREATE TEMP TABLE document_sequences(organization_id text,document_type text,business_year integer,last_sequence integer,updated_at timestamptz,PRIMARY KEY(organization_id,document_type,business_year))`,
		`CREATE TEMP TABLE products(id text PRIMARY KEY,organization_id text,product_code text,inventory_status text,base_sale_price_minor bigint,base_sale_currency text,brand text,model_number text,deleted_at timestamptz,updated_at timestamptz)`,
		`CREATE TEMP TABLE shipment_slips(id text PRIMARY KEY,organization_id text,slip_number text,buyer_role_id text,sales_slip_id text,shipment_date date,recipient_name text,recipient_address text,carrier text,tracking_number text,display_currency text,fx_rate_snapshot_id text,fx_rate_scaled bigint,fx_scale bigint,status text,notes text,created_by text,created_at timestamptz,updated_at timestamptz,confirmed_at timestamptz,confirmed_by text)`,
		`CREATE TEMP TABLE shipment_lines(id text,shipment_slip_id text,line_number integer,product_id text,quantity integer,sale_price_usd_minor bigint,converted_sale_price_jpy bigint,created_at timestamptz)`,
		`CREATE TEMP TABLE inventory_events(id text,organization_id text,product_id text,event_type text,from_status text,to_status text,reason text,actor_user_id text,created_at timestamptz)`,
		`CREATE TEMP TABLE sales_slips(id text,organization_id text,slip_number text,buyer_role_id text,status text)`,
		`CREATE TEMP TABLE sales_lines(sales_slip_id text,product_id text)`,
		`CREATE TEMP TABLE purchase_requests(organization_id text,product_id text,buyer_role_id text,status text)`,
		`CREATE TEMP TABLE consignment_slips(id text,organization_id text,consignee_role_id text,status text)`,
		`CREATE TEMP TABLE consignment_lines(consignment_slip_id text,product_id text)`,
		`INSERT INTO business_partners VALUES('partner','Buyer','domestic')`,
		`INSERT INTO partner_roles VALUES('buyer','org','partner','buyer','0001',true)`,
		`INSERT INTO exchange_rate_snapshots VALUES('fx','org','USD','JPY',15500,100,now(),now())`,
		`INSERT INTO products(id,organization_id,product_code,inventory_status,base_sale_price_minor,base_sale_currency,brand,model_number) VALUES('a','org','A','in_stock',1000,'USD','Brand','Model'),('b','org','B','in_stock',2000,'USD','Brand','Model'),('c','org','C','in_stock',3000,'USD','Brand','Model')`,
	}
	for _, q := range schema {
		if err := tx.Exec(q).Error; err != nil {
			t.Fatal(err)
		}
	}
	migration, err := migrationFS.ReadFile("migrations/000090_shipment_tax_snapshot.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(string(migration)).Error; err != nil {
		t.Fatal(err)
	}
	repo := &Repository{db: tx, driver: "postgres"}
	ctx := context.Background()
	a, b := int64(750), int64(1350)
	input := ShipmentCreateInput{OrganizationID: "org", ActorUserID: "actor", BuyerCode: "0001", ShipmentDate: "2026-09-17", ProductCodes: []string{"A", "B"}, DisplayCurrency: "JPY", Items: []ShipmentPriceInput{{ProductCode: "B", SalePriceUSDMinor: &b}, {ProductCode: "A", SalePriceUSDMinor: &a}}}
	record, err := repo.CreateShipment(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	for _, confirm := range []bool{false, true} {
		if confirm {
			record, err = repo.ConfirmShipment(ctx, "org", record.ID, "actor")
		} else {
			record, err = repo.ShipmentSlip(ctx, "org", record.ID)
		}
		if err != nil {
			t.Fatal(err)
		}
		if record.TaxRateBasisPoints != 1000 {
			t.Fatalf("domestic tax not saved: %+v", record)
		}
		if len(record.Lines) != 2 || record.Lines[0].SalePriceUSDMinor != 750 || record.Lines[1].SalePriceUSDMinor != 1350 || record.Lines[0].ConvertedSalePriceJPY != 117000 || record.Lines[1].ConvertedSalePriceJPY != 210000 {
			t.Fatalf("saved prices=%+v", record.Lines)
		}
	}
	if err := tx.Exec(`UPDATE products SET base_sale_price_minor=9999 WHERE id IN ('a','b')`).Error; err != nil {
		t.Fatal(err)
	}
	record, err = repo.ShipmentSlip(ctx, "org", record.ID)
	if err != nil || record.Lines[0].SalePriceUSDMinor != 750 {
		t.Fatalf("snapshot changed: %+v %v", record, err)
	}
	if err := tx.Exec("UPDATE business_partners SET region_type='overseas'").Error; err != nil {
		t.Fatal(err)
	}
	reloaded, err := repo.ShipmentSlip(ctx, "org", record.ID)
	if err != nil || reloaded.TaxRateBasisPoints != 1000 {
		t.Fatalf("tax snapshot changed: %+v %v", reloaded, err)
	}
	input.ProductCodes = []string{"C"}
	input.Items = nil
	input.DisplayCurrency = "USD"
	legacy, err := repo.CreateShipment(ctx, input)
	if err != nil || legacy.Lines[0].SalePriceUSDMinor != 3000 || legacy.TaxRateBasisPoints != 0 {
		t.Fatalf("legacy=%+v err=%v", legacy, err)
	}
	// Returning A while B remains shipped completes only A's original line.
	oldID := record.ID
	checkState := func(id, expected string, expectedPending int) {
		t.Helper()
		got, err := repo.ShipmentSlip(ctx, "org", id)
		if err != nil {
			t.Fatal(err)
		}
		pending := 0
		for _, line := range got.Lines {
			if line.ShipmentPending {
				pending++
			}
		}
		if got.ProcessingStatus != expected || pending != expectedPending {
			t.Fatalf("shipment %s state=%s pending=%d want %s/%d", id, got.ProcessingStatus, pending, expected, expectedPending)
		}
		list, err := repo.ShipmentSlips(ctx, "org", 500)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range list {
			if entry.ID == id {
				if entry.ProcessingStatus != expected {
					t.Fatalf("list state=%s want %s", entry.ProcessingStatus, expected)
				}
				return
			}
		}
		t.Fatal("shipment missing from list")
	}
	checkState(oldID, "processing", 2)
	if _, err := repo.ReturnShipmentProduct(ctx, "org", oldID, "A", "actor"); err != nil {
		t.Fatal(err)
	}
	checkState(oldID, "processing", 1)
	input.ProductCodes = []string{"A"}
	newer, err := repo.CreateShipment(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	newer, err = repo.ConfirmShipment(ctx, "org", newer.ID, "actor")
	if err != nil {
		t.Fatal(err)
	}
	checkState(oldID, "processing", 1)
	checkState(newer.ID, "processing", 1)
	// Old-document QR must never return A from the newer shipment.
	if _, err := repo.ReturnShipmentProduct(ctx, "org", oldID, "A", "actor"); !errors.Is(err, ErrShipmentLineCompleted) {
		t.Fatalf("old scan=%v", err)
	}
	var activeStatus string
	if err := tx.Table("products").Select("inventory_status").Where("id='a'").Scan(&activeStatus).Error; err != nil {
		t.Fatal(err)
	}
	if activeStatus != "shipped" {
		t.Fatalf("old QR changed active shipment: %s", activeStatus)
	}
	if _, err := repo.ReturnShipmentProduct(ctx, "org", oldID, "B", "actor"); err != nil {
		t.Fatal(err)
	}
	checkState(oldID, "completed", 0)
	checkState(newer.ID, "processing", 1)
	// Repeat scans are harmless, and completion survives another DB read.
	if scan, err := repo.ReturnShipmentProduct(ctx, "org", oldID, "B", "actor"); err != nil || scan.Result != "already_in_stock" {
		t.Fatalf("duplicate=%+v %v", scan, err)
	}
	if _, err := repo.ReturnShipmentProduct(ctx, "org", newer.ID, "A", "actor"); err != nil {
		t.Fatal(err)
	}
	checkState(newer.ID, "completed", 0)
	third, err := repo.CreateShipment(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	third, err = repo.ConfirmShipment(ctx, "org", third.ID, "actor")
	if err != nil {
		t.Fatal(err)
	}
	checkState(oldID, "completed", 0)
	checkState(newer.ID, "completed", 0)
	checkState(third.ID, "processing", 1)
	// A sale also closes the shipment, even after a sales return and reshipment.
	if err := tx.Exec(`UPDATE products SET inventory_status='sold' WHERE id='a'`).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO inventory_events(id,organization_id,product_id,event_type,from_status,to_status,created_at) VALUES('sold-a','org','a','sale_confirmed','shipped','sold',clock_timestamp())`).Error; err != nil {
		t.Fatal(err)
	}
	checkState(third.ID, "completed", 0)
	if err := tx.Exec(`UPDATE products SET inventory_status='in_stock' WHERE id='a'`).Error; err != nil {
		t.Fatal(err)
	}
	fourth, err := repo.CreateShipment(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	fourth, err = repo.ConfirmShipment(ctx, "org", fourth.ID, "actor")
	if err != nil {
		t.Fatal(err)
	}
	checkState(third.ID, "completed", 0)
	checkState(fourth.ID, "processing", 1)
	if _, err := repo.ReturnShipmentProduct(ctx, "org", third.ID, "A", "actor"); !errors.Is(err, ErrShipmentLineCompleted) {
		t.Fatalf("sold old QR=%v", err)
	}

}
