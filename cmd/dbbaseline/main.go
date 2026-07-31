// Command dbbaseline generates a flat, empty-database bootstrap schema from
// the reviewed legacy replay candidate. It never opens the application DB.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

func main() {
	var input string
	var output string
	flag.StringVar(&input, "input", "", "reviewed replay SQL")
	flag.StringVar(&output, "output", "", "flat baseline output SQL")
	flag.Parse()
	if input == "" || output == "" {
		fatal(errors.New("-input and -output are required"))
	}

	replay, err := os.ReadFile(input)
	if err != nil {
		fatal(err)
	}
	tempDir, err := os.MkdirTemp("", "zaiko-d1-baseline-*")
	if err != nil {
		fatal(err)
	}
	defer os.RemoveAll(tempDir)

	db, err := sql.Open("sqlite", filepath.Join(tempDir, "schema.sqlite3"))
	if err != nil {
		fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		fatal(err)
	}
	if _, err := db.ExecContext(ctx, string(replay)); err != nil {
		fatal(fmt.Errorf("apply reviewed replay: %w", err))
	}

	rows, err := db.QueryContext(ctx, `
		SELECT type, name, sql
		FROM sqlite_schema
		WHERE sql IS NOT NULL
		  AND name NOT LIKE 'sqlite_%'
		ORDER BY CASE type
			WHEN 'table' THEN 1
			WHEN 'index' THEN 2
			WHEN 'trigger' THEN 3
			ELSE 4
		END, name
	`)
	if err != nil {
		fatal(err)
	}
	defer rows.Close()

	var generated strings.Builder
	generated.WriteString("-- GENERATED D1 TEST BASELINE THROUGH LEGACY MIGRATION 000027.\n")
	generated.WriteString("-- Empty database bootstrap only. No seed data. Never apply to the current SQLite database.\n\n")
	for rows.Next() {
		var objectType string
		var name string
		var statement string
		if err := rows.Scan(&objectType, &name, &statement); err != nil {
			fatal(err)
		}
		fmt.Fprintf(&generated, "-- %s: %s\n%s;\n\n", objectType, name, strings.TrimSuffix(statement, ";"))
	}
	if err := rows.Err(); err != nil {
		fatal(err)
	}

	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		fatal(err)
	}
	if err := os.WriteFile(output, []byte(generated.String()), 0o644); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
