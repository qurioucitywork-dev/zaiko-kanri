package diagnostics

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// TestD1BaselineCandidateAppliesToIsolatedSQLite validates only the generated
// review candidate. It never opens the application's configured database.
func TestD1BaselineCandidateAppliesToIsolatedSQLite(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file")
	}
	candidatePath := filepath.Join(filepath.Dir(filename), "..", "..", "docs", "db-api", "baseline-candidate-000027.sql")
	script, err := os.ReadFile(candidatePath)
	if err != nil {
		t.Fatal(err)
	}

	dbPath := filepath.Join(t.TempDir(), "baseline-validation.sqlite3")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, string(script)); err != nil {
		t.Fatalf("apply baseline candidate to isolated database: %v", err)
	}

	var integrity string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil {
		t.Fatal(err)
	}
	if integrity != "ok" {
		t.Fatalf("integrity_check = %q, want ok", integrity)
	}

	var foreignKeyViolations int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM pragma_foreign_key_check").Scan(&foreignKeyViolations); err != nil {
		t.Fatal(err)
	}
	if foreignKeyViolations != 0 {
		t.Fatalf("foreign key violations = %d, want 0", foreignKeyViolations)
	}

	assertSchemaObjectCount(t, ctx, db, "table", 49)
	assertSchemaObjectCount(t, ctx, db, "trigger", 2)
	assertRequiredColumns(t, ctx, db, "products", []string{
		"id", "organization_id", "product_code", "sku", "cost_amount_minor",
		"base_sale_price_minor", "material_text", "box_text", "features_text",
	})
	assertRequiredColumns(t, ctx, db, "purchase_requests", []string{
		"id", "organization_id", "product_id", "guest_name", "request_group_id", "status",
	})
}

func TestD1FlatBaselineMatchesReplaySchema(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file")
	}
	root := filepath.Join(filepath.Dir(filename), "..", "..")
	replay := readTestSQL(t, filepath.Join(root, "docs", "db-api", "baseline-candidate-000027.sql"))
	flat := readTestSQL(t, filepath.Join(root, "deploy", "cloudflare", "d1-service", "migrations", "0001_baseline.sql"))

	replaySchema := applyAndReadSchema(t, replay)
	flatSchema := applyAndReadSchema(t, flat)
	if replaySchema != flatSchema {
		t.Fatal("flat D1 baseline schema differs from reviewed replay schema")
	}
}

func readTestSQL(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func applyAndReadSchema(t *testing.T, script string) string {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "schema.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(script); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(`
		SELECT type, name, sql
		FROM sqlite_schema
		WHERE sql IS NOT NULL AND name NOT LIKE 'sqlite_%'
		ORDER BY type, name
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var schema strings.Builder
	for rows.Next() {
		var objectType string
		var name string
		var statement string
		if err := rows.Scan(&objectType, &name, &statement); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&schema, "%s|%s|%s\n", objectType, name, normalizeSchemaSQL(statement))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return schema.String()
}

func normalizeSchemaSQL(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(value)), " ")
}

func assertSchemaObjectCount(t *testing.T, ctx context.Context, db *sql.DB, objectType string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM sqlite_schema WHERE type = ? AND name NOT LIKE 'sqlite_%'",
		objectType,
	).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("%s count = %d, want %d", objectType, got, want)
	}
}

func assertRequiredColumns(t *testing.T, ctx context.Context, db *sql.DB, table string, required []string) {
	t.Helper()
	rows, err := db.QueryContext(ctx, "SELECT name FROM pragma_table_info(?)", table)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	columns := make(map[string]struct{})
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		columns[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for _, name := range required {
		if _, ok := columns[name]; !ok {
			t.Errorf("%s.%s is missing", table, name)
		}
	}
}
