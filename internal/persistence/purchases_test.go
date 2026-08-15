package persistence

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestNormalizePurchaseTaxMode(t *testing.T) {
	tests := []struct {
		input string
		mode  string
		rate  int
	}{
		{"", PurchaseTaxModeDomestic, 1000},
		{"domestic", PurchaseTaxModeDomestic, 1000},
		{"OVERSEAS", PurchaseTaxModeOverseas, 0},
	}
	for _, test := range tests {
		mode, rate, err := normalizePurchaseTaxMode(test.input)
		if err != nil || mode != test.mode || rate != test.rate {
			t.Fatalf("normalizePurchaseTaxMode(%q) = %q, %d, %v", test.input, mode, rate, err)
		}
	}
	if _, _, err := normalizePurchaseTaxMode("unknown"); err != ErrPurchaseTaxMode {
		t.Fatalf("unexpected invalid mode error: %v", err)
	}
}

func TestPurchaseCostSnapshotUsesLatestRateAtRegistration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:purchase_registration_fx?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE exchange_rate_snapshots (
		id TEXT PRIMARY KEY, organization_id TEXT, base_currency TEXT, quote_currency TEXT,
		rate_scaled INTEGER, scale INTEGER, observed_at DATETIME, created_at DATETIME
	)`).Error; err != nil {
		t.Fatal(err)
	}
	oldObserved := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	newObserved := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	if err := db.Exec(`INSERT INTO exchange_rate_snapshots
		(id,organization_id,base_currency,quote_currency,rate_scaled,scale,observed_at,created_at)
		VALUES ('fx_old','org','USD','JPY',150,1,?,?),('fx_registration','org','USD','JPY',160,1,?,?)`,
		oldObserved, oldObserved, newObserved, newObserved).Error; err != nil {
		t.Fatal(err)
	}

	converted, rateID, rateScaled, scale, err := purchaseCostSnapshot(db, "org", 100, 2, "USD")
	if err != nil {
		t.Fatal(err)
	}
	if converted != 32000 || rateID != "fx_registration" || rateScaled != int64(160) || scale != int64(1) {
		t.Fatalf("registration snapshot = converted:%d id:%v rate:%v scale:%v", converted, rateID, rateScaled, scale)
	}
}
