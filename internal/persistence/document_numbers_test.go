package persistence

import (
	"context"
	"errors"
	"fmt"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
	"gorm.io/gorm"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDailyDocumentNumbersPostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_DAILY_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated database required")
	}
	if !strings.Contains(dsn, "/zaiko_daily_test") {
		t.Fatal("non-test database rejected")
	}
	ctx := context.Background()
	r, err := Open(config.Config{DatabaseDriver: "postgres", DatabaseURL: dsn, Environment: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(r.Migrate(ctx))
	must(r.SeedPreviewIdentity(ctx, "daily-test-admin", "daily-test-worker"))
	must(r.SeedPreviewMasters(ctx))
	var org, admin, supplier, brand, staff, role string
	for _, q := range []struct {
		sql  string
		dest *string
	}{
		{"SELECT id FROM organizations WHERE code='PREVIEW'", &org},
		{"SELECT id FROM users WHERE username='admin' LIMIT 1", &admin},
		{"SELECT role_code FROM partner_roles WHERE role_type='supplier' AND is_active ORDER BY role_code LIMIT 1", &supplier},
		{"SELECT code FROM brands WHERE is_active ORDER BY code LIMIT 1", &brand},
		{"SELECT staff_code FROM staff_profiles WHERE is_purchase_staff ORDER BY staff_code LIMIT 1", &staff},
		{"SELECT id FROM partner_roles WHERE role_type='buyer' LIMIT 1", &role},
	} {
		must(r.db.Raw(q.sql).Scan(q.dest).Error)
	}
	must(r.db.Exec("INSERT INTO exchange_rate_snapshots(id,organization_id,base_currency,quote_currency,rate_scaled,scale,provider,created_by,observed_at,created_at) VALUES('daily-rate',?,'USD','JPY',15500,100,'test',?,now(),now())", org, admin).Error)
	makePurchase := func(date string) PurchaseSlipRecord {
		t.Helper()
		p, e := r.CreatePurchase(ctx, PurchaseCreateInput{OrganizationID: org, ActorUserID: admin, SupplierCode: supplier, StaffCode: staff, PurchaseDate: date, Lines: []PurchaseLineInput{{Quantity: 1, BrandCode: brand, ModelNumber: "unchanged model", UnitCostMinor: 10000, CostCurrency: "JPY", BaseSalePriceMinor: 150, BaseSaleCurrency: "USD"}}})
		must(e)
		return p
	}
	p := makePurchase("2026-09-17")
	if p.SlipNumber != "SD-170926001" {
		t.Fatal(p.SlipNumber)
	}
	p, err = r.ConfirmPurchase(ctx, org, p.ID, admin)
	must(err)
	archived := makePurchase("2026-09-17")
	other := makePurchase("2026-09-18")
	must(r.db.Exec("UPDATE purchase_slips SET slip_number='SD-2026-0001',notes='SD-2026-0001 / XSD-2026-0001 / SD-2026-00010' WHERE id=?", p.ID).Error)
	must(r.db.Exec("UPDATE purchase_slips SET slip_number='SD-2026-0002',status='cancelled' WHERE id=?", archived.ID).Error)
	must(r.db.Exec("UPDATE purchase_slips SET slip_number='SD-2026-0003' WHERE id=?", other.ID).Error)
	must(r.db.Exec("INSERT INTO sales_slips(id,organization_id,slip_number,buyer_role_id,sale_date,display_currency,tax_mode,status,created_by,created_at,updated_at) VALUES('daily-sale',?,'UD-2026-0001',?,'2026-09-17','JPY','taxable','confirmed',?,now(),now())", org, role, admin).Error)
	must(r.db.Exec("INSERT INTO shipment_slips(id,organization_id,slip_number,buyer_role_id,sales_slip_id,shipment_date,status,created_by,created_at,updated_at) VALUES('daily-shipment',?,'SY-2026-0001',?,'daily-sale','2026-09-17','confirmed',?,now(),now())", org, role, admin).Error)
	must(r.db.Exec("INSERT INTO consignment_slips(id,organization_id,slip_number,consignee_role_id,consignment_date,status,confirmed_at,confirmed_by,created_by,created_at,updated_at) VALUES('daily-consignment',?,'IT-2026-0001',?,'2026-09-17','confirmed',now(),?,?,now(),now())", org, role, admin, admin).Error)
	for _, item := range []struct{ prefix, operation string }{{"UH", "return"}, {"SH", "purchase_return"}, {"RT", "takeout"}} {
		must(r.db.Exec("INSERT INTO return_slips(id,organization_id,slip_number,operation_type,transaction_date,status,created_by,created_at,updated_at) VALUES(?,?,?,?,'2026-09-17','confirmed',?,now(),now())", item.prefix, org, item.prefix+"-2026-0001", item.operation, admin).Error)
	}
	for _, item := range []struct{ prefix, kind string }{{"GW", "breakdown"}, {"GA", "combine"}, {"GC", "direct_increase"}, {"CA", "swap"}} {
		must(r.db.Exec(`INSERT INTO cost_adjustments(id,organization_id,slip_number,source_product_id,source_purchase_slip_id,source_purchase_slip_line_id,adjustment_type,adjustment_date,source_product_code,source_cost_jpy_minor,allocated_cost_jpy_minor,status,created_by,created_at,updated_at)
 SELECT ?,?, ?,p.id,l.purchase_slip_id,l.id,?,'2026-09-17',p.product_code,10000,10000,'confirmed',?,now(),now() FROM products p JOIN purchase_slip_lines l ON l.id=p.purchase_slip_line_id WHERE l.purchase_slip_id=?`, item.prefix, org, item.prefix+"-2026-0001", item.kind, admin, p.ID).Error)
	}
	must(r.db.Exec("INSERT INTO sales_adjustment_slips(id,organization_id,slip_number,source_sales_slip_id,adjustment_date,direction,display_currency,reason,status,created_by,created_at,updated_at) VALUES('daily-adjustment',?,'SS-2026-0001','daily-sale','2026-09-17','increase','JPY','UD-2026-0001','confirmed',?,now(),now())", org, admin).Error)

	must(r.db.Exec("UPDATE purchase_slip_lines SET notes='SD-2026-0001' WHERE purchase_slip_id=?", p.ID).Error)
	must(r.db.Exec(`INSERT INTO approval_requests(id,organization_id,approval_type,target_type,target_id,requested_action,status,requested_by,requested_at,snapshot_json,created_at,updated_at) VALUES('daily-approval',?,'purchase','purchase',?,'confirm','pending',?,now(),'{"slipNumber":"SD-2026-0001","amount":10000}',now(),now())`, org, p.ID, admin).Error)
	must(r.db.Exec(`INSERT INTO cost_adjustment_start_snapshots(id,organization_id,operation_id,mode,product_id,previous_inventory_status,previous_base_sale_price_minor,previous_base_sale_currency,previous_internal_comment,status,created_by,created_at) SELECT 'daily-snapshot',?,'GW','breakdown',id,'in_stock',150,'USD','GW-2026-0001','active',?,now() FROM products WHERE purchase_slip_line_id IN (SELECT id FROM purchase_slip_lines WHERE purchase_slip_id=?)`, org, admin, p.ID).Error)
	// Re-run the new migration over representative legacy records in the isolated DB.
	must(r.db.Exec("DROP TABLE daily_document_sequences,document_number_aliases; DELETE FROM schema_migrations WHERE version='000096_daily_document_numbers'").Error)
	must(r.Migrate(ctx))
	var aliases []struct{ DocumentTable, DocumentID, OldNumber, NewNumber string }
	must(r.db.Table("document_number_aliases").Find(&aliases).Error)
	if len(aliases) != 14 {
		t.Fatalf("mapped %d documents, want 14", len(aliases))
	}
	for _, a := range aliases {
		var got string
		must(r.db.Table(a.DocumentTable).Select("slip_number").Where("id=?", a.DocumentID).Scan(&got).Error)
		if got != a.NewNumber || !strings.HasPrefix(got, strings.Split(a.OldNumber, "-")[0]+"-") || len(strings.Split(got, "-")[1]) != 9 {
			t.Fatalf("bad mapping: %+v => %s", a, got)
		}
	}
	var notes, status, source string
	var cost int64
	must(r.db.Raw("SELECT notes FROM purchase_slips WHERE id=?", p.ID).Scan(&notes).Error)
	if notes != "SD-170926001 / XSD-2026-0001 / SD-2026-00010" {
		t.Fatal(notes)
	}
	must(r.db.Raw("SELECT status FROM purchase_slips WHERE id=?", archived.ID).Scan(&status).Error)
	if status != "cancelled" {
		t.Fatal(status)
	}
	must(r.db.Raw("SELECT sales_slip_id FROM shipment_slips WHERE id='daily-shipment'").Scan(&source).Error)
	if source != "daily-sale" {
		t.Fatal(source)
	}
	must(r.db.Raw("SELECT unit_cost_minor FROM purchase_slip_lines WHERE purchase_slip_id=?", p.ID).Scan(&cost).Error)
	if cost != 10000 {
		t.Fatal(cost)
	}

	must(r.db.Raw("SELECT notes FROM purchase_slip_lines WHERE purchase_slip_id=?", p.ID).Scan(&notes).Error)
	if notes != "SD-170926001" {
		t.Fatal(notes)
	}
	must(r.db.Raw("SELECT snapshot_json->>'slipNumber' FROM approval_requests WHERE id='daily-approval'").Scan(&notes).Error)
	if notes != "SD-170926001" {
		t.Fatal(notes)
	}
	must(r.db.Raw("SELECT previous_internal_comment FROM cost_adjustment_start_snapshots WHERE id='daily-snapshot'").Scan(&notes).Error)
	if notes != "GW-170926001" {
		t.Fatal(notes)
	}
	if got := makePurchase("2026-09-17").SlipNumber; got != "SD-170926003" {
		t.Fatal(got)
	}
	if got := makePurchase("2026-09-18").SlipNumber; got != "SD-180926002" {
		t.Fatal(got)
	}
	if got := makePurchase("2027-01-01").SlipNumber; got != "SD-010127001" {
		t.Fatal(got)
	}
	must(r.Migrate(ctx)) // Repeated app start must not renumber saved records again.
	date, _ := time.Parse("2006-01-02", "2026-09-17")
	allocate := func(prefix string) (string, error) {
		var n string
		err := r.db.Transaction(func(tx *gorm.DB) error {
			var e error
			n, e = nextDailyDocumentNumber(tx, org, prefix, date, time.Now())
			return e
		})
		return n, err
	}
	// Concurrent requests serialize per day/type, with no duplicates.
	var wg sync.WaitGroup
	results := make(chan string, 20)
	failures := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n, e := allocate("SY")
			if e != nil {
				failures <- e
			} else {
				results <- n
			}
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	for e := range failures {
		t.Fatal(e)
	}
	seen := map[string]bool{}
	for n := range results {
		if seen[n] {
			t.Fatal("duplicate", n)
		}
		seen[n] = true
	}
	if len(seen) != 20 {
		t.Fatal(len(seen))
	}
	sentinel := errors.New("rollback")
	err = r.db.Transaction(func(tx *gorm.DB) error {
		_, e := nextDailyDocumentNumber(tx, org, "IT", date, time.Now())
		if e != nil {
			return e
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	n, e := allocate("IT")
	must(e)
	if n != "IT-170926002" {
		t.Fatal(n)
	}
	must(r.db.Exec("UPDATE daily_document_sequences SET last_sequence=998 WHERE organization_id=? AND prefix='RT'", org).Error)
	n, e = allocate("RT")
	must(e)
	if n != "RT-170926999" {
		t.Fatal(n)
	}
	_, e = allocate("RT")
	if !errors.Is(e, ErrDailyDocumentLimit) {
		t.Fatal("overflow accepted", e)
	}
	fmt.Println("All slip types, legacy migration, archived records, daily reset, references, concurrency, rollback and 999 limit verified")
}
