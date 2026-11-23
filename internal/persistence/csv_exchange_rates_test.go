package persistence

import (
	"context"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"testing"
	"time"
)

func TestCSVExchangeRateAtCreation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:csv_fx_history?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE exchange_rate_snapshots(id TEXT,organization_id TEXT,base_currency TEXT,quote_currency TEXT,rate_scaled BIGINT,scale BIGINT,observed_at DATETIME,created_at DATETIME)`).Error; err != nil {
		t.Fatal(err)
	}
	repo := &Repository{db: db}
	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	insert := func(id, org, currency string, rate int64, observed, created time.Time) {
		t.Helper()
		if err := db.Exec(`INSERT INTO exchange_rate_snapshots VALUES(?,?,?,'JPY',?,100,?,?)`, id, org, currency, rate, observed, created).Error; err != nil {
			t.Fatal(err)
		}
	}
	insert("old", "org", "USD", 15550, base, base)
	for i := 1; i <= 110; i++ {
		insert(time.Duration(i).String(), "org", "USD", 99900, base.Add(time.Duration(i)*24*time.Hour), base.Add(time.Duration(i)*24*time.Hour))
	}
	insert("other-org", "other", "USD", 88800, base, base)
	insert("other-currency", "org", "HKD", 2000, base, base)
	insert("backdated-later", "org", "USD", 77700, base.Add(time.Minute), base.Add(48*time.Hour))
	got, err := repo.exchangeRateAtCreation(context.Background(), "org", "USD", base.Add(time.Hour))
	if err != nil || got != 155.5 {
		t.Fatalf("want frozen 155.5 despite 110 later rates, got %v, %v", got, err)
	}
	got, err = repo.exchangeRateAtCreation(context.Background(), "org", "USD", base.Add(-time.Hour))
	if err != nil || got != 0 {
		t.Fatalf("missing rate must remain unknown, got %v, %v", got, err)
	}
	got, err = repo.exchangeRateAtCreation(context.Background(), "org", "USD", time.Time{})
	if err != nil || got != 0 {
		t.Fatalf("missing creation timestamp must remain unknown, got %v, %v", got, err)
	}
}
