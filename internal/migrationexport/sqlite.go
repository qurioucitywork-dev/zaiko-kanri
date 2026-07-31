// Package migrationexport creates deterministic, read-only migration
// artifacts from the current SQLite database. It never mutates source data.
package migrationexport

import (
	"bufio"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const FormatVersion = 1

type Manifest struct {
	FormatVersion int             `json:"format_version"`
	GeneratedAt   time.Time       `json:"generated_at"`
	SchemaVersion int64           `json:"schema_version"`
	Tables        []TableManifest `json:"tables"`
}

type TableManifest struct {
	Name           string   `json:"name"`
	File           string   `json:"file"`
	Columns        []string `json:"columns"`
	PrimaryKey     []string `json:"primary_key"`
	RowCount       int64    `json:"row_count"`
	ChecksumSHA256 string   `json:"checksum_sha256"`
}

type rowEnvelope struct {
	Table  string                 `json:"table"`
	Values map[string]exportValue `json:"values"`
}

// exportValue retains SQLite storage classes without routing 64-bit integers
// through JavaScript's floating-point number representation.
type exportValue struct {
	Kind string `json:"kind"`
	Text string `json:"text,omitempty"`
}

// OpenReadOnly opens an existing SQLite file in immutable migration-source
// mode. The caller owns the returned handle.
func OpenReadOnly(path string) (*sql.DB, error) {
	absolute, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil || strings.TrimSpace(path) == "" {
		return nil, errors.New("migrationexport: invalid source path")
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, fmt.Errorf("migrationexport: inspect source: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("migrationexport: source is not a regular file")
	}

	// Use an opaque file URI so Windows drive letters are not interpreted as
	// URI authorities by the SQLite driver (file:C:/... rather than
	// file://C:/...).
	dsnURL := &url.URL{Scheme: "file", Opaque: filepath.ToSlash(absolute)}
	query := dsnURL.Query()
	query.Set("mode", "ro")
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "query_only(1)")
	dsnURL.RawQuery = query.Encode()

	db, err := sql.Open("sqlite", dsnURL.String())
	if err != nil {
		return nil, fmt.Errorf("migrationexport: open source: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrationexport: ping source: %w", err)
	}
	return db, nil
}

// Export creates outputDirectory exclusively. Existing paths are rejected so
// a previous evidence artifact can never be overwritten accidentally.
func Export(
	ctx context.Context,
	db *sql.DB,
	outputDirectory string,
	now time.Time,
) (Manifest, error) {
	if db == nil || strings.TrimSpace(outputDirectory) == "" || now.IsZero() {
		return Manifest{}, errors.New("migrationexport: invalid argument")
	}
	if err := ctx.Err(); err != nil {
		return Manifest{}, err
	}
	if err := os.Mkdir(outputDirectory, 0o750); err != nil {
		if errors.Is(err, os.ErrExist) {
			return Manifest{}, errors.New("migrationexport: output path already exists")
		}
		return Manifest{}, fmt.Errorf("migrationexport: create output: %w", err)
	}

	success := false
	defer func() {
		if !success {
			_ = os.RemoveAll(outputDirectory)
		}
	}()

	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Manifest{}, fmt.Errorf("migrationexport: begin snapshot: %w", err)
	}
	defer tx.Rollback()

	tableNames, err := listTables(ctx, tx)
	if err != nil {
		return Manifest{}, err
	}
	manifest := Manifest{
		FormatVersion: FormatVersion,
		GeneratedAt:   now.UTC(),
		SchemaVersion: readSchemaVersion(ctx, tx),
		Tables:        make([]TableManifest, 0, len(tableNames)),
	}
	for _, tableName := range tableNames {
		table, exportErr := exportTable(ctx, tx, outputDirectory, tableName)
		if exportErr != nil {
			return Manifest{}, exportErr
		}
		manifest.Tables = append(manifest.Tables, table)
	}
	if err := tx.Commit(); err != nil {
		return Manifest{}, fmt.Errorf("migrationexport: finish snapshot: %w", err)
	}
	if err := writeJSONExclusive(
		filepath.Join(outputDirectory, "manifest.json"),
		manifest,
	); err != nil {
		return Manifest{}, err
	}
	success = true
	return manifest, nil
}

func listTables(ctx context.Context, tx *sql.Tx) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT name
		FROM sqlite_schema
		WHERE type = 'table'
		  AND name NOT LIKE 'sqlite_%'
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("migrationexport: list tables: %w", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("migrationexport: scan table name: %w", err)
		}
		if !validIdentifier(name) {
			return nil, errors.New("migrationexport: unsafe table identifier")
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("migrationexport: list tables: %w", err)
	}
	return names, nil
}

func readSchemaVersion(ctx context.Context, tx *sql.Tx) int64 {
	var exists int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM sqlite_schema
		WHERE type = 'table' AND name = 'schema_migrations'
	`).Scan(&exists); err != nil || exists == 0 {
		return 0
	}
	var version sql.NullInt64
	if err := tx.QueryRowContext(
		ctx, `SELECT MAX(version) FROM schema_migrations`,
	).Scan(&version); err != nil || !version.Valid {
		return 0
	}
	return version.Int64
}

type columnDefinition struct {
	Name       string
	PrimaryKey int
}

func exportTable(
	ctx context.Context,
	tx *sql.Tx,
	outputDirectory string,
	tableName string,
) (TableManifest, error) {
	columns, err := tableColumns(ctx, tx, tableName)
	if err != nil {
		return TableManifest{}, err
	}
	if len(columns) == 0 {
		return TableManifest{}, errors.New("migrationexport: table has no columns")
	}
	columnNames := make([]string, 0, len(columns))
	var primaryKeyColumns []columnDefinition
	for _, column := range columns {
		columnNames = append(columnNames, column.Name)
		if column.PrimaryKey > 0 {
			primaryKeyColumns = append(primaryKeyColumns, column)
		}
	}
	sort.Slice(primaryKeyColumns, func(i, j int) bool {
		return primaryKeyColumns[i].PrimaryKey < primaryKeyColumns[j].PrimaryKey
	})
	primaryKey := make([]string, 0, len(primaryKeyColumns))
	for _, column := range primaryKeyColumns {
		primaryKey = append(primaryKey, column.Name)
	}

	orderColumns := primaryKey
	if len(orderColumns) == 0 {
		orderColumns = columnNames
	}
	query := "SELECT " + quoteIdentifiers(columnNames) +
		" FROM " + quoteIdentifier(tableName) +
		" ORDER BY " + quoteIdentifiers(orderColumns)
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return TableManifest{}, fmt.Errorf("migrationexport: read table %s: %w", tableName, err)
	}
	defer rows.Close()

	fileName := tableName + ".ndjson"
	filePath := filepath.Join(outputDirectory, fileName)
	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return TableManifest{}, fmt.Errorf("migrationexport: create table artifact: %w", err)
	}
	hasher := sha256.New()
	writer := bufio.NewWriter(io.MultiWriter(file, hasher))
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(true)

	var rowCount int64
	values := make([]any, len(columnNames))
	destinations := make([]any, len(columnNames))
	for index := range values {
		destinations[index] = &values[index]
	}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			_ = file.Close()
			return TableManifest{}, err
		}
		if err := rows.Scan(destinations...); err != nil {
			_ = file.Close()
			return TableManifest{}, fmt.Errorf("migrationexport: scan %s: %w", tableName, err)
		}
		envelope := rowEnvelope{
			Table:  tableName,
			Values: make(map[string]exportValue, len(columnNames)),
		}
		for index, columnName := range columnNames {
			envelope.Values[columnName] = encodeValue(values[index])
		}
		if err := encoder.Encode(envelope); err != nil {
			_ = file.Close()
			return TableManifest{}, fmt.Errorf("migrationexport: encode %s: %w", tableName, err)
		}
		rowCount++
	}
	if err := rows.Err(); err != nil {
		_ = file.Close()
		return TableManifest{}, fmt.Errorf("migrationexport: iterate %s: %w", tableName, err)
	}
	if err := writer.Flush(); err != nil {
		_ = file.Close()
		return TableManifest{}, fmt.Errorf("migrationexport: flush %s: %w", tableName, err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return TableManifest{}, fmt.Errorf("migrationexport: sync %s: %w", tableName, err)
	}
	if err := file.Close(); err != nil {
		return TableManifest{}, fmt.Errorf("migrationexport: close %s: %w", tableName, err)
	}
	return TableManifest{
		Name:           tableName,
		File:           fileName,
		Columns:        columnNames,
		PrimaryKey:     primaryKey,
		RowCount:       rowCount,
		ChecksumSHA256: hex.EncodeToString(hasher.Sum(nil)),
	}, nil
}

func tableColumns(ctx context.Context, tx *sql.Tx, tableName string) ([]columnDefinition, error) {
	rows, err := tx.QueryContext(ctx, "PRAGMA table_info("+quoteIdentifier(tableName)+")")
	if err != nil {
		return nil, fmt.Errorf("migrationexport: describe %s: %w", tableName, err)
	}
	defer rows.Close()
	var columns []columnDefinition
	for rows.Next() {
		var cid int
		var name, declaredType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(
			&cid, &name, &declaredType, &notNull, &defaultValue, &primaryKey,
		); err != nil {
			return nil, fmt.Errorf("migrationexport: describe %s: %w", tableName, err)
		}
		if !validIdentifier(name) {
			return nil, errors.New("migrationexport: unsafe column identifier")
		}
		columns = append(columns, columnDefinition{Name: name, PrimaryKey: primaryKey})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("migrationexport: describe %s: %w", tableName, err)
	}
	return columns, nil
}

func encodeValue(value any) exportValue {
	switch typed := value.(type) {
	case nil:
		return exportValue{Kind: "null"}
	case int64:
		return exportValue{Kind: "integer", Text: strconv.FormatInt(typed, 10)}
	case float64:
		return exportValue{Kind: "real", Text: strconv.FormatFloat(typed, 'g', -1, 64)}
	case []byte:
		return exportValue{Kind: "blob", Text: base64.StdEncoding.EncodeToString(typed)}
	case string:
		return exportValue{Kind: "text", Text: typed}
	case bool:
		return exportValue{Kind: "integer", Text: strconv.FormatBool(typed)}
	case time.Time:
		return exportValue{Kind: "text", Text: typed.UTC().Format(time.RFC3339Nano)}
	default:
		return exportValue{Kind: "text", Text: fmt.Sprint(typed)}
	}
}

func writeJSONExclusive(path string, value any) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return fmt.Errorf("migrationexport: create manifest: %w", err)
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		_ = file.Close()
		return fmt.Errorf("migrationexport: write manifest: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("migrationexport: sync manifest: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("migrationexport: close manifest: %w", err)
	}
	return nil
}

func validIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for index, r := range value {
		if (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			r == '_' ||
			(index > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

func quoteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func quoteIdentifiers(values []string) string {
	quoted := make([]string, len(values))
	for index, value := range values {
		quoted[index] = quoteIdentifier(value)
	}
	return strings.Join(quoted, ", ")
}
