package persistence

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/exchangerate"
)

func TestSignificantRateChange(t *testing.T) {
	for _, v := range []struct {
		old, next int64
		want      bool
	}{{10000, 10100, true}, {10000, 9900, true}, {10000, 10099, false}, {10000, 9901, false}, {0, 10000, false}, {10000, 10000, false}, {15799000000, 15956990000, true}} {
		if significantRateChange(v.old, v.next) != v.want {
			t.Fatal(v)
		}
	}
}

func TestSMBCSyncPostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_SMBC_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL test database required")
	}
	if !strings.Contains(dsn, "/zaiko_smbc_test_") {
		t.Fatal("non-test database rejected")
	}
	ctx := context.Background()
	repo, err := Open(config.Config{DatabaseDriver: "postgres", DatabaseURL: dsn, Environment: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(repo.Migrate(ctx))
	must(repo.SeedPreviewIdentity(ctx, "smbc-test-admin", "smbc-test-worker"))
	db := repo.db
	repo.db = db.Begin()
	defer func() { repo.db.Rollback(); repo.db = db }()
	org, err := repo.ExchangeRateOrganization(ctx, "PREVIEW")
	must(err)
	var admin, worker string
	must(repo.db.Raw("SELECT id FROM users WHERE organization_id=? AND role_key='admin' LIMIT 1", org).Scan(&admin).Error)
	must(repo.db.Raw("SELECT id FROM users WHERE organization_id=? AND role_key='worker' LIMIT 1", org).Scan(&worker).Error)
	now := time.Date(2026, 10, 14, 2, 0, 0, 0, time.UTC)
	must(repo.db.Exec(`INSERT INTO users(id,organization_id,username,password_hash,display_name,role_key,is_active,created_at,updated_at) SELECT 'smbc-admin-2',organization_id,'smbc-admin-2',password_hash,'Test admin 2','admin',true,?,? FROM users WHERE id=?`, now, now, admin).Error)
	_, err = repo.CreateExchangeRate(ctx, org, admin, "USD", "250", "legacy manual", now.Add(-24*time.Hour))
	must(err)
	claimed, err := repo.ClaimExchangeRateSync(ctx, org, now)
	must(err)
	if !claimed {
		t.Fatal("first claim failed")
	}
	claimed, err = repo.ClaimExchangeRateSync(ctx, org, now.Add(time.Second))
	must(err)
	if claimed {
		t.Fatal("duplicate fetch lease")
	}
	p := exchangerate.Publication{Date: "2026-10-14", SourceURL: exchangerate.IndexURL, SHA256: strings.Repeat("a", 64), Rates: map[string]int64{"USD": 100 * exchangerate.Scale, "HKD": 100 * exchangerate.Scale, "EUR": 100 * exchangerate.Scale}}
	must(repo.CompleteExchangeRateSync(ctx, org, p, now))
	count := func(table string) int64 {
		t.Helper()
		var n int64
		must(repo.db.Table(table).Where("organization_id=?", org).Count(&n).Error)
		return n
	}
	if count("notifications") != 0 {
		t.Fatal("first import compared with manual test rates")
	}
	old, err := repo.exchangeRateAtCreation(ctx, org, "USD", now.Add(time.Minute))
	must(err)
	if old != 100 {
		t.Fatal(old)
	}
	p.Date = "2026-10-15"
	p.Rates = map[string]int64{"USD": 101 * exchangerate.Scale, "HKD": 99 * exchangerate.Scale, "EUR": 10099000000}
	must(repo.CompleteExchangeRateSync(ctx, org, p, now.Add(24*time.Hour)))
	if count("exchange_rate_snapshots") != 7 || count("notifications") != 4 {
		t.Fatal("snapshots/independent admin notifications missing")
	}
	must(repo.CompleteExchangeRateSync(ctx, org, p, now.Add(25*time.Hour)))
	if count("exchange_rate_snapshots") != 7 || count("notifications") != 4 {
		t.Fatal("same publication duplicated")
	}
	historical, err := repo.exchangeRateAtCreation(ctx, org, "USD", now.Add(time.Minute))
	must(err)
	if historical != 100 {
		t.Fatal("historical rate changed")
	}
	items, err := repo.Notifications(ctx, org, admin, "admin", 200, true)
	must(err)
	if len(items) != 2 {
		t.Fatal(items)
	}
	must(repo.ReadNotification(ctx, org, admin, "admin", items[0].ID))
	second, err := repo.Notifications(ctx, org, "smbc-admin-2", "admin", 200, true)
	must(err)
	if len(second) != 2 {
		t.Fatal("one admin read another admin notification")
	}
	hidden, err := repo.Notifications(ctx, org, worker, "worker", 200, true)
	must(err)
	if len(hidden) != 0 {
		t.Fatal("admin alert leaked")
	}
	must(repo.FailExchangeRateSync(ctx, org, now.Add(26*time.Hour)))
	must(repo.FailExchangeRateSync(ctx, org, now.Add(27*time.Hour)))
	if count("notifications") != 6 || count("exchange_rate_snapshots") != 7 {
		t.Fatal("failure alert duplicated or rates overwritten")
	}
	state, err := repo.ExchangeRateSyncStatus(ctx, org)
	must(err)
	if state.LastError == "" || state.PublishedDate != "2026-10-15" {
		t.Fatal(state)
	}
	must(repo.CompleteExchangeRateSync(ctx, org, exchangerate.Publication{Date: p.Date, SourceURL: p.SourceURL, Unchanged: true}, now.Add(28*time.Hour)))
	state, err = repo.ExchangeRateSyncStatus(ctx, org)
	must(err)
	if state.LastError != "" {
		t.Fatal("recovery did not clear error")
	}
	p.Date = "2026-10-16"
	delete(p.Rates, "HKD")
	if repo.CompleteExchangeRateSync(ctx, org, p, now.Add(48*time.Hour)) == nil {
		t.Fatal("partial rates accepted")
	}
	if count("exchange_rate_snapshots") != 7 {
		t.Fatal("partial import wrote rates")
	}
	p.Date = "2026-10-14"
	if repo.CompleteExchangeRateSync(ctx, org, p, now.Add(48*time.Hour)) == nil {
		t.Fatal("old publication accepted")
	}
}
