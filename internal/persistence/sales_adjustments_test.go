package persistence

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestSalesAdjustmentClickDate(t *testing.T) {
	now := time.Date(2026, 9, 13, 20, 0, 0, 0, time.UTC)
	for _, tc := range []struct{ input, want string }{
		{"2026-09-12", "2026-09-12"}, // Keep the click date even if saving happens later.
		{"", "2026-09-14"},           // Legacy fallback must not use the previous UTC date.
		{"2026-02-30", ""},
		{"2026-9-14", ""},
		{"invalid", ""},
	} {
		got, err := salesAdjustmentDate(tc.input, now)
		if tc.want == "" {
			if !errors.Is(err, ErrSalesAdjustmentInvalid) {
				t.Errorf("%q: expected invalid date", tc.input)
			}
		} else if err != nil || got.Format("2006-01-02") != tc.want {
			t.Errorf("%q: got %v, %v; want %s", tc.input, got, err, tc.want)
		}
	}
}

func TestSalesAdjustmentLineReasons(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input SalesAdjustmentInput
		want  []string
	}{
		{"individual reasons", SalesAdjustmentInput{Direction: "decrease", Lines: []SalesAdjustmentLineInput{{SaleLineID: "l1", AdjustmentAmountMinor: 200, Reason: " 傷のため "}, {SaleLineID: "l2", AdjustmentAmountMinor: 350, Reason: "付属品変更"}}}, []string{"傷のため", "付属品変更"}},
		{"legacy common reason", SalesAdjustmentInput{Direction: "increase", Reason: " 共通理由 ", Lines: []SalesAdjustmentLineInput{{SaleLineID: "l1", AdjustmentAmountMinor: 1}}}, []string{"共通理由"}},
		{"missing row reason", SalesAdjustmentInput{Direction: "increase", Lines: []SalesAdjustmentLineInput{{SaleLineID: "l1", AdjustmentAmountMinor: 1, Reason: "理由"}, {SaleLineID: "l2", AdjustmentAmountMinor: 1}}}, nil},
		{"blank reason", SalesAdjustmentInput{Direction: "increase", Lines: []SalesAdjustmentLineInput{{SaleLineID: "l1", AdjustmentAmountMinor: 1, Reason: " \n "}}}, nil},
		{"long reason", SalesAdjustmentInput{Direction: "increase", Lines: []SalesAdjustmentLineInput{{SaleLineID: "l1", AdjustmentAmountMinor: 1, Reason: strings.Repeat("字", 1001)}}}, nil},
		{"invalid direction", SalesAdjustmentInput{Direction: "subtract", Reason: "理由", Lines: []SalesAdjustmentLineInput{{SaleLineID: "l1", AdjustmentAmountMinor: 1}}}, nil},
		{"duplicate line", SalesAdjustmentInput{Direction: "increase", Reason: "理由", Lines: []SalesAdjustmentLineInput{{SaleLineID: "l1", AdjustmentAmountMinor: 1}, {SaleLineID: "l1", AdjustmentAmountMinor: 2}}}, nil},
		{"zero adjustment", SalesAdjustmentInput{Direction: "increase", Reason: "理由", Lines: []SalesAdjustmentLineInput{{SaleLineID: "l1"}}}, nil},
		{"negative adjustment", SalesAdjustmentInput{Direction: "decrease", Reason: "理由", Lines: []SalesAdjustmentLineInput{{SaleLineID: "l1", AdjustmentAmountMinor: -1}}}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := salesAdjustmentLineReasons(tc.input)
			if tc.want == nil {
				if !errors.Is(err, ErrSalesAdjustmentInvalid) {
					t.Fatalf("error=%v", err)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got=%v, err=%v, want=%v", got, err, tc.want)
			}
		})
	}
}

func TestSalesAdjustmentNotesAndCommentPreserveReasons(t *testing.T) {
	input := SalesAdjustmentInput{Direction: "decrease", Lines: []SalesAdjustmentLineInput{{Reason: "傷のため"}, {Reason: "付属品変更"}}}
	notes := salesAdjustmentNotes(input, []string{"傷のため", "付属品変更"}, []string{"1808260001", "1808260002"})
	if notes != "商品管理番号 1808260001 修正理由: 傷のため\n商品管理番号 1808260002 修正理由: 付属品変更" {
		t.Fatalf("notes=%q", notes)
	}
	legacy := salesAdjustmentNotes(SalesAdjustmentInput{Reason: " 共通理由 ", Lines: []SalesAdjustmentLineInput{{}}}, []string{"共通理由"}, []string{"1808260001"})
	if legacy != "共通理由" {
		t.Fatalf("legacy notes=%q", legacy)
	}
	oldComment := "既存のコメント\n売上修正伝票 SRV-2026-0001 修正理由: 以前の変更"
	got := appendSalesAdjustmentComment(oldComment, "SRV-2026-0002", "decrease", "USD", 200, "傷のため")
	if got != oldComment+"\n売上修正伝票 SRV-2026-0002 減額 USD 200 修正理由: 傷のため" {
		t.Fatalf("comment=%q", got)
	}
	if got := appendSalesAdjustmentComment("", "SRV-2026-0003", "increase", "JPY", 1000, "増額理由"); got != "売上修正伝票 SRV-2026-0003 増額 JPY 1000 修正理由: 増額理由" {
		t.Fatalf("increase comment=%q", got)
	}
}

// The fixture uses standalone SQLite in read tests, or PostgreSQL TEMP tables
// in write tests. It never reads or changes business rows from a supplied DSN.
func salesAdjustmentFixture(t *testing.T, db *gorm.DB, temporary bool) {
	t.Helper()
	create := "CREATE TABLE "
	if temporary {
		create = "CREATE TEMP TABLE "
	}
	for _, definition := range []string{
		`document_sequences (organization_id TEXT,document_type TEXT,business_year INTEGER,last_sequence INTEGER,updated_at TIMESTAMP,PRIMARY KEY(organization_id,document_type,business_year))`,
		`sales_slips (id TEXT PRIMARY KEY,organization_id TEXT,slip_number TEXT,sale_date TIMESTAMP,display_currency TEXT,status TEXT,buyer_role_id TEXT,notes TEXT,tax_mode TEXT DEFAULT 'out_of_scope',tax_rate_basis_points INTEGER DEFAULT 0)`,
		`sales_lines (id TEXT PRIMARY KEY,sales_slip_id TEXT,product_id TEXT,line_number INTEGER,sale_currency TEXT,unit_price_minor BIGINT,fx_rate_scaled BIGINT,fx_scale BIGINT)`,
		`products (id TEXT PRIMARY KEY,organization_id TEXT,product_code TEXT,brand TEXT,model_number TEXT,internal_comment TEXT,updated_at TIMESTAMP)`,
		`partner_roles (id TEXT PRIMARY KEY,role_code TEXT,partner_id TEXT)`,
		`business_partners (id TEXT PRIMARY KEY,legal_name TEXT)`,
		`sales_adjustment_slips (id TEXT PRIMARY KEY,organization_id TEXT,slip_number TEXT,source_sales_slip_id TEXT,adjustment_date TIMESTAMP,direction TEXT,display_currency TEXT,reason TEXT,status TEXT,created_by TEXT,created_at TIMESTAMP,updated_at TIMESTAMP)`,
		`sales_adjustment_lines (id TEXT PRIMARY KEY,sales_adjustment_slip_id TEXT,source_sales_line_id TEXT,product_id TEXT,line_number INTEGER,adjustment_amount_minor BIGINT,converted_adjustment_jpy BIGINT,reason TEXT DEFAULT '',created_at TIMESTAMP,before_sale_price_minor BIGINT,after_sale_price_minor BIGINT,direction TEXT NOT NULL DEFAULT '')`,
	} {
		if err := db.Exec(create + definition).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, statement := range []string{
		`INSERT INTO sales_slips VALUES ('s1','org1','SI-2026-0005','2026-08-18','USD','confirmed','buyer1','元の備考','out_of_scope',0)`,
		`INSERT INTO partner_roles VALUES ('buyer1','BUYER-01','bp1')`,
		`INSERT INTO business_partners VALUES ('bp1','取引先')`,
		`INSERT INTO products VALUES ('p1','org1','1808260001','ブランド1','モデル1','元のコメント1',NULL),('p2','org1','1808260002','ブランド2','モデル2','元のコメント2',NULL)`,
		`INSERT INTO sales_lines VALUES ('l1','s1','p1',1,'USD',9200,15000,100),('l2','s1','p2',2,'USD',9850,15000,100)`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func TestSalesAdjustmentDetailReadsPerLineReasonsAndLegacyFallback(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	defer sqlDB.Close()
	salesAdjustmentFixture(t, db, false)
	now := time.Now().UTC()
	if err := db.Exec(`INSERT INTO sales_adjustment_slips VALUES ('a1','org1','SRV-2026-0001','s1',?,'decrease','USD','旧共通理由','confirmed','u1',?,?)`, now, now, now).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO sales_adjustment_lines VALUES ('al1','a1','l1','p1',1,200,30000,'商品ごとの理由',?,9200,9000,''),('al2','a1','l2','p2',2,350,52500,'',?,9850,9500,'')`, now, now).Error; err != nil {
		t.Fatal(err)
	}
	repo := &Repository{db: db, driver: "sqlite"}
	got, err := repo.SalesAdjustment(context.Background(), "org1", "a1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Lines) != 2 || got.Lines[0].SourceSaleLineID != "l1" || got.Lines[0].Reason != "商品ごとの理由" || got.Lines[1].Reason != "旧共通理由" {
		t.Fatalf("lines=%+v", got.Lines)
	}
	if got.TotalMinor != 550 || got.ConvertedTotalJPY != 82500 {
		t.Fatalf("record=%+v", got)
	}
	if got.Lines[0].BeforeSalePriceMinor != 9200 || got.Lines[0].AfterSalePriceMinor != 9000 {
		t.Fatalf("price snapshots=%+v", got.Lines)
	}
	if err := db.Exec("UPDATE sales_slips SET tax_mode='taxable',tax_rate_basis_points=1000 WHERE id='s1'").Error; err != nil {
		t.Fatal(err)
	}
	taxed, err := repo.SalesAdjustment(context.Background(), "org1", "a1")
	if err != nil || taxed.SubtotalMinor != 550 || taxed.TaxAmountMinor != 55 || taxed.TotalMinor != 605 || taxed.ConvertedTotalJPY != 90750 {
		t.Fatalf("taxed=%+v err=%v", taxed, err)
	}
	if err := db.Exec("UPDATE sales_adjustment_slips SET direction='mixed' WHERE id='a1'").Error; err != nil {
		t.Fatal(err)
	}
	db.Exec("UPDATE sales_adjustment_lines SET direction=CASE WHEN id='al1' THEN 'increase' ELSE 'decrease' END")
	mixedTaxed, err := repo.SalesAdjustment(context.Background(), "org1", "a1")
	if err != nil || mixedTaxed.SubtotalMinor != -150 || mixedTaxed.TaxAmountMinor != -15 || mixedTaxed.TotalMinor != -165 {
		t.Fatalf("mixed taxed=%+v err=%v", mixedTaxed, err)
	}
	db.Exec("UPDATE sales_slips SET tax_mode='tax_exempt' WHERE id='s1'")
	exempt, err := repo.SalesAdjustment(context.Background(), "org1", "a1")
	if err != nil || exempt.TaxAmountMinor != 0 || exempt.TotalMinor != -150 {
		t.Fatalf("exempt=%+v err=%v", exempt, err)
	}
	if _, err := repo.SalesAdjustment(context.Background(), "another-org", "a1"); !errors.Is(err, ErrSaleNotFound) {
		t.Fatalf("cross-organization error=%v", err)
	}
}

func TestSalesAdjustmentLineReasonMigration(t *testing.T) {
	catalog, err := migrationCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, migration := range catalog {
		if migration.Version != "000081_sales_adjustment_line_reasons" {
			continue
		}
		if !strings.Contains(migration.SQL, "ALTER TABLE sales_adjustment_lines") || !strings.Contains(migration.SQL, "reason TEXT NOT NULL DEFAULT ''") {
			t.Fatalf("unexpected migration: %s", migration.SQL)
		}
		return
	}
	t.Fatal("line reason migration not found")
}

func TestSalesAdjustmentCreateWithLineReasonsPostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_SALES_ADJUSTMENT_TEST_DSN")
	if dsn == "" {
		t.Skip("PostgreSQL test DSN not set; this test uses only temporary tables and always rolls back")
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
	salesAdjustmentFixture(t, tx, true)
	repo := &Repository{db: tx, driver: "postgres"}
	input := SalesAdjustmentInput{OrganizationID: "org1", ActorUserID: "u1", Direction: "decrease", Lines: []SalesAdjustmentLineInput{{SaleLineID: "l1", AdjustmentAmountMinor: 200, Reason: "傷のため"}, {SaleLineID: "l2", AdjustmentAmountMinor: 350, Reason: "付属品変更"}}}
	got, err := repo.CreateSalesAdjustment(context.Background(), "s1", input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got.SlipNumber, "-0001") || len(got.Lines) != 2 || got.TotalMinor != 550 || got.Lines[0].Reason != "傷のため" || got.Lines[1].Reason != "付属品変更" {
		t.Fatalf("adjustment=%+v", got)
	}
	if !strings.Contains(got.Reason, "1808260001 修正理由: 傷のため") || !strings.Contains(got.Reason, "1808260002 修正理由: 付属品変更") {
		t.Fatalf("voucher notes=%q", got.Reason)
	}
	var comments []struct{ ID, InternalComment string }
	if err := tx.Table("products").Select("id,internal_comment").Order("id").Find(&comments).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(comments[0].InternalComment, "元のコメント1\n") || !strings.Contains(comments[0].InternalComment, "傷のため") || strings.Contains(comments[0].InternalComment, "付属品変更") {
		t.Fatalf("product1=%+v", comments[0])
	}
	if !strings.Contains(comments[1].InternalComment, "付属品変更") {
		t.Fatalf("product2=%+v", comments[1])
	}
	input.Direction = "increase"
	input.Lines = []SalesAdjustmentLineInput{{SaleLineID: "l1", AdjustmentAmountMinor: 100, Reason: "追加変更"}}
	second, err := repo.CreateSalesAdjustment(context.Background(), "s1", input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(second.SlipNumber, "-0002") {
		t.Fatalf("second sequence=%q", second.SlipNumber)
	}
	if second.Lines[0].BeforeSalePriceMinor != 9000 || second.Lines[0].AfterSalePriceMinor != 9100 {
		t.Fatalf("second price snapshots=%+v", second.Lines)
	}
	// Subsequent source edits must not change a saved revision's prices.
	if err := tx.Exec("UPDATE sales_lines SET unit_price_minor=9999 WHERE id='l1'").Error; err != nil {
		t.Fatal(err)
	}
	firstAgain, err := repo.SalesAdjustment(context.Background(), "org1", got.ID)
	if err != nil || firstAgain.Lines[0].BeforeSalePriceMinor != 9200 || firstAgain.Lines[0].AfterSalePriceMinor != 9000 {
		t.Fatalf("saved snapshot changed: %+v err=%v", firstAgain, err)
	}
	tx.Exec("UPDATE sales_lines SET unit_price_minor=9200 WHERE id='l1'")
	var comment string
	tx.Table("products").Select("internal_comment").Where("id='p1'").Scan(&comment)
	if !strings.Contains(comment, got.SlipNumber) || !strings.Contains(comment, second.SlipNumber) {
		t.Fatalf("history was overwritten: %q", comment)
	}
	var originalNotes string
	tx.Table("sales_slips").Select("notes").Where("id='s1'").Scan(&originalNotes)
	var originalPrices []int64
	tx.Table("sales_lines").Order("line_number").Pluck("unit_price_minor", &originalPrices)
	if originalNotes != "元の備考" || !reflect.DeepEqual(originalPrices, []int64{9200, 9850}) {
		t.Fatalf("original sale changed: notes=%q prices=%v", originalNotes, originalPrices)
	}
	input.Direction = "decrease"
	input.Lines = []SalesAdjustmentLineInput{{SaleLineID: "l1", AdjustmentAmountMinor: 1, Reason: "一時変更"}, {SaleLineID: "l2", AdjustmentAmountMinor: 999999, Reason: "範囲外"}}
	if _, err := repo.CreateSalesAdjustment(context.Background(), "s1", input); !errors.Is(err, ErrSalesAdjustmentInvalid) {
		t.Fatalf("excessive decrease error=%v", err)
	}
	var count int64
	tx.Table("sales_adjustment_slips").Count(&count)
	if count != 2 {
		t.Fatalf("failed change leaked voucher: count=%d", count)
	}
	var afterFailed string
	tx.Table("products").Select("internal_comment").Where("id='p1'").Scan(&afterFailed)
	if afterFailed != comment {
		t.Fatalf("failed change leaked comment: %q", afterFailed)
	}
	// Mixed directions must persist per line, net the header total, and affect subsequent prices.
	input = SalesAdjustmentInput{OrganizationID: "org1", ActorUserID: "u1", Direction: "increase", Lines: []SalesAdjustmentLineInput{
		{SaleLineID: "l1", Direction: "increase", AdjustmentAmountMinor: 200, Reason: "増額理由"},
		{SaleLineID: "l2", Direction: "decrease", AdjustmentAmountMinor: 350, Reason: "減額理由"},
	}}
	mixed, err := repo.CreateSalesAdjustment(context.Background(), "s1", input)
	if err != nil {
		t.Fatal(err)
	}
	if mixed.Direction != "mixed" || mixed.TotalMinor != -150 || mixed.ConvertedTotalJPY != -22500 || mixed.Lines[0].Direction != "increase" || mixed.Lines[1].Direction != "decrease" || mixed.Lines[0].AfterSalePriceMinor != 9300 || mixed.Lines[1].AfterSalePriceMinor != 9150 {
		t.Fatalf("mixed=%+v lines=%+v", mixed, mixed.Lines)
	}
	input.Lines = []SalesAdjustmentLineInput{{SaleLineID: "l2", Direction: "increase", AdjustmentAmountMinor: 100, Reason: "再調整"}}
	afterMixed, err := repo.CreateSalesAdjustment(context.Background(), "s1", input)
	if err != nil || afterMixed.Lines[0].BeforeSalePriceMinor != 9150 || afterMixed.Lines[0].AfterSalePriceMinor != 9250 {
		t.Fatalf("after mixed=%+v err=%v", afterMixed, err)
	}
}
