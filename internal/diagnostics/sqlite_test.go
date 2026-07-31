package diagnostics

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestCollectSQLiteReport(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`
		PRAGMA foreign_keys = ON;
		CREATE TABLE schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL
		);
		CREATE TABLE parents (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL
		);
		CREATE TABLE children (
			id TEXT PRIMARY KEY,
			parent_id TEXT NOT NULL REFERENCES parents(id)
		);
		INSERT INTO schema_migrations(version, applied_at)
		VALUES ('000001_test', '2026-07-30T00:00:00Z');
		INSERT INTO parents(id, name) VALUES ('p1', 'parent');
		INSERT INTO children(id, parent_id) VALUES ('c1', 'p1');
	`); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	report, err := CollectSQLiteReport(context.Background(), db, now)
	if err != nil {
		t.Fatal(err)
	}
	if report.GeneratedAt.Location() != time.UTC {
		t.Fatalf("generated_at location = %v, want UTC", report.GeneratedAt.Location())
	}
	if report.IntegrityCheck != "ok" {
		t.Fatalf("integrity_check = %q, want ok", report.IntegrityCheck)
	}
	if report.ForeignKeyViolations != 0 {
		t.Fatalf("foreign_key_violations = %d, want 0", report.ForeignKeyViolations)
	}
	if len(report.Migrations) != 1 || report.Migrations[0] != "000001_test" {
		t.Fatalf("migrations = %#v", report.Migrations)
	}
	counts := map[string]int64{}
	for _, table := range report.Tables {
		counts[table.Name] = table.RowCount
	}
	if counts["parents"] != 1 || counts["children"] != 1 {
		t.Fatalf("row counts = %#v", counts)
	}
}

func TestQuoteIdentifierEscapesDoubleQuotes(t *testing.T) {
	if got, want := quoteIdentifier(`odd"name`), `"odd""name"`; got != want {
		t.Fatalf("quoteIdentifier() = %q, want %q", got, want)
	}
}
