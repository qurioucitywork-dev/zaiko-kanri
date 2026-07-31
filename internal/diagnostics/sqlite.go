package diagnostics

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"
)

// SQLiteReport is a read-only snapshot used before data migration.
// It intentionally contains schema and aggregate information only.
type SQLiteReport struct {
	GeneratedAt          time.Time         `json:"generated_at"`
	IntegrityCheck       string            `json:"integrity_check"`
	ForeignKeyViolations int               `json:"foreign_key_violations"`
	Migrations           []string          `json:"migrations"`
	Tables               []SQLiteTableInfo `json:"tables"`
}

type SQLiteTableInfo struct {
	Name     string             `json:"name"`
	RowCount int64              `json:"row_count"`
	Columns  []SQLiteColumnInfo `json:"columns"`
}

type SQLiteColumnInfo struct {
	Position     int            `json:"position"`
	Name         string         `json:"name"`
	Type         string         `json:"type"`
	NotNull      bool           `json:"not_null"`
	DefaultValue sql.NullString `json:"default_value"`
	PrimaryKey   bool           `json:"primary_key"`
}

// CollectSQLiteReport only executes SELECT and PRAGMA diagnostic statements.
// The caller must open the database read-only and enable query_only.
func CollectSQLiteReport(ctx context.Context, db *sql.DB, now time.Time) (SQLiteReport, error) {
	report := SQLiteReport{GeneratedAt: now.UTC()}

	if err := db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&report.IntegrityCheck); err != nil {
		return SQLiteReport{}, fmt.Errorf("integrity check: %w", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_foreign_key_check`).Scan(&report.ForeignKeyViolations); err != nil {
		return SQLiteReport{}, fmt.Errorf("foreign key check: %w", err)
	}

	migrations, err := collectMigrations(ctx, db)
	if err != nil {
		return SQLiteReport{}, err
	}
	report.Migrations = migrations

	tableNames, err := collectTableNames(ctx, db)
	if err != nil {
		return SQLiteReport{}, err
	}
	for _, name := range tableNames {
		table, err := collectTable(ctx, db, name)
		if err != nil {
			return SQLiteReport{}, err
		}
		report.Tables = append(report.Tables, table)
	}
	return report, nil
}

func collectMigrations(ctx context.Context, db *sql.DB) ([]string, error) {
	var exists int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM sqlite_schema
		WHERE type = 'table' AND name = 'schema_migrations'
	`).Scan(&exists); err != nil {
		return nil, fmt.Errorf("check schema_migrations: %w", err)
	}
	if exists == 0 {
		return []string{}, nil
	}
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, fmt.Errorf("list migrations: %w", err)
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("scan migration: %w", err)
		}
		result = append(result, version)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate migrations: %w", err)
	}
	return result, nil
}

func collectTableNames(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT name
		FROM sqlite_schema
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%'
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan table name: %w", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tables: %w", err)
	}
	sort.Strings(names)
	return names, nil
}

func collectTable(ctx context.Context, db *sql.DB, name string) (SQLiteTableInfo, error) {
	table := SQLiteTableInfo{Name: name}
	countQuery := `SELECT COUNT(*) FROM ` + quoteIdentifier(name)
	if err := db.QueryRowContext(ctx, countQuery).Scan(&table.RowCount); err != nil {
		return SQLiteTableInfo{}, fmt.Errorf("count %q: %w", name, err)
	}

	rows, err := db.QueryContext(ctx, `
		SELECT cid, name, type, "notnull", dflt_value, pk
		FROM pragma_table_info(?)
		ORDER BY cid
	`, name)
	if err != nil {
		return SQLiteTableInfo{}, fmt.Errorf("describe %q: %w", name, err)
	}
	defer rows.Close()
	for rows.Next() {
		var column SQLiteColumnInfo
		var notNull, primaryKey int
		if err := rows.Scan(
			&column.Position,
			&column.Name,
			&column.Type,
			&notNull,
			&column.DefaultValue,
			&primaryKey,
		); err != nil {
			return SQLiteTableInfo{}, fmt.Errorf("scan column for %q: %w", name, err)
		}
		column.NotNull = notNull != 0
		column.PrimaryKey = primaryKey != 0
		table.Columns = append(table.Columns, column)
	}
	if err := rows.Err(); err != nil {
		return SQLiteTableInfo{}, fmt.Errorf("iterate columns for %q: %w", name, err)
	}
	return table, nil
}

func quoteIdentifier(value string) string {
	result := `"`
	for _, r := range value {
		if r == '"' {
			result += `""`
		} else {
			result += string(r)
		}
	}
	return result + `"`
}
