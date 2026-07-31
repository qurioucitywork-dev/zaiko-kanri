package migrationexport

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestExportIsDeterministicTypedAndNonDestructive(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "source.sqlite3")
	db, err := sql.Open("sqlite", sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY);
		INSERT INTO schema_migrations(version) VALUES (27);
		CREATE TABLE sample (
			id INTEGER PRIMARY KEY,
			large_value INTEGER NOT NULL,
			text_value TEXT NOT NULL,
			optional_value TEXT
		);
		INSERT INTO sample(id, large_value, text_value, optional_value)
		VALUES (2, 9007199254740993, 'second', NULL),
		       (1, 42, '<first>', 'present');
	`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	readOnly, err := OpenReadOnly(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	defer readOnly.Close()
	if _, err := readOnly.Exec(`INSERT INTO sample(id, large_value, text_value) VALUES (3, 1, 'bad')`); err == nil {
		t.Fatal("read-only source accepted a write")
	}

	output := filepath.Join(t.TempDir(), "artifact")
	now := time.Date(2026, 7, 31, 1, 2, 3, 0, time.FixedZone("JST", 9*60*60))
	manifest, err := Export(context.Background(), readOnly, output, now)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != 27 || manifest.GeneratedAt.Location() != time.UTC {
		t.Fatalf("manifest = %#v", manifest)
	}
	var sample TableManifest
	for _, table := range manifest.Tables {
		if table.Name == "sample" {
			sample = table
		}
	}
	if sample.RowCount != 2 || len(sample.ChecksumSHA256) != 64 {
		t.Fatalf("sample manifest = %#v", sample)
	}
	content, err := os.ReadFile(filepath.Join(output, sample.File))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) != 2 {
		t.Fatalf("rows = %d, want 2", len(lines))
	}
	var first rowEnvelope
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatal(err)
	}
	if first.Values["id"].Text != "1" ||
		first.Values["large_value"].Kind != "integer" ||
		first.Values["text_value"].Text != "<first>" {
		t.Fatalf("first row = %#v", first)
	}
	var second rowEnvelope
	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
		t.Fatal(err)
	}
	if second.Values["large_value"].Text != "9007199254740993" ||
		second.Values["optional_value"].Kind != "null" {
		t.Fatalf("second row = %#v", second)
	}
}

func TestExportRejectsExistingOutputAndCancellation(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE sample (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	existing := t.TempDir()
	if _, err := Export(context.Background(), db, existing, time.Now()); err == nil {
		t.Fatal("existing output path was accepted")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	output := filepath.Join(t.TempDir(), "cancelled")
	if _, err := Export(ctx, db, output, time.Now()); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancelled output exists: %v", err)
	}
}
