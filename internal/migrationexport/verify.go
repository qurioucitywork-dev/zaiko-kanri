package migrationexport

import (
	"bufio"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Verify validates an export artifact without connecting to, creating, or
// changing any database. It is intended as the first gate before a separate
// provider-specific transform/import step.
func Verify(directory string) (Manifest, error) {
	absolute, err := filepath.Abs(strings.TrimSpace(directory))
	if err != nil || strings.TrimSpace(directory) == "" {
		return Manifest{}, errors.New("migrationexport: invalid artifact path")
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return Manifest{}, fmt.Errorf("migrationexport: inspect artifact: %w", err)
	}
	if !info.IsDir() {
		return Manifest{}, errors.New("migrationexport: artifact path is not a directory")
	}

	manifestFile, err := os.Open(filepath.Join(absolute, "manifest.json"))
	if err != nil {
		return Manifest{}, fmt.Errorf("migrationexport: open manifest: %w", err)
	}
	defer manifestFile.Close()

	var manifest Manifest
	decoder := json.NewDecoder(io.LimitReader(manifestFile, 16<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("migrationexport: decode manifest: %w", err)
	}
	if manifest.FormatVersion != FormatVersion ||
		manifest.GeneratedAt.IsZero() ||
		manifest.SchemaVersion < 0 {
		return Manifest{}, errors.New("migrationexport: invalid manifest header")
	}

	seenTables := make(map[string]struct{}, len(manifest.Tables))
	seenFiles := make(map[string]struct{}, len(manifest.Tables))
	for _, table := range manifest.Tables {
		if err := validateTableManifest(table, seenTables, seenFiles); err != nil {
			return Manifest{}, err
		}
		if err := verifyTableFile(absolute, table); err != nil {
			return Manifest{}, err
		}
	}
	return manifest, nil
}

func validateTableManifest(
	table TableManifest,
	seenTables, seenFiles map[string]struct{},
) error {
	if !validIdentifier(table.Name) ||
		table.File != table.Name+".ndjson" ||
		filepath.Base(table.File) != table.File ||
		table.RowCount < 0 ||
		len(table.Columns) == 0 ||
		len(table.ChecksumSHA256) != 64 {
		return errors.New("migrationexport: invalid table manifest")
	}
	if _, err := hex.DecodeString(table.ChecksumSHA256); err != nil {
		return errors.New("migrationexport: invalid table checksum")
	}
	if _, exists := seenTables[table.Name]; exists {
		return errors.New("migrationexport: duplicate table manifest")
	}
	if _, exists := seenFiles[table.File]; exists {
		return errors.New("migrationexport: duplicate artifact file")
	}
	seenTables[table.Name] = struct{}{}
	seenFiles[table.File] = struct{}{}

	columns := make(map[string]struct{}, len(table.Columns))
	for _, column := range table.Columns {
		if !validIdentifier(column) {
			return errors.New("migrationexport: invalid manifest column")
		}
		if _, exists := columns[column]; exists {
			return errors.New("migrationexport: duplicate manifest column")
		}
		columns[column] = struct{}{}
	}
	for _, column := range table.PrimaryKey {
		if _, exists := columns[column]; !exists {
			return errors.New("migrationexport: primary key is not a table column")
		}
	}
	return nil
}

func verifyTableFile(directory string, table TableManifest) error {
	path := filepath.Join(directory, table.File)
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("migrationexport: open %s: %w", table.Name, err)
	}
	defer file.Close()

	hasher := sha256.New()
	scanner := bufio.NewScanner(io.TeeReader(file, hasher))
	scanner.Buffer(make([]byte, 64<<10), 16<<20)
	var rowCount int64
	for scanner.Scan() {
		if err := verifyRow(table, scanner.Bytes()); err != nil {
			return err
		}
		rowCount++
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("migrationexport: read %s: %w", table.Name, err)
	}
	if rowCount != table.RowCount {
		return fmt.Errorf(
			"migrationexport: row count mismatch for %s: got %d, want %d",
			table.Name,
			rowCount,
			table.RowCount,
		)
	}
	if actual := hex.EncodeToString(hasher.Sum(nil)); actual != table.ChecksumSHA256 {
		return fmt.Errorf("migrationexport: checksum mismatch for %s", table.Name)
	}
	return nil
}

func verifyRow(table TableManifest, encoded []byte) error {
	var row rowEnvelope
	decoder := json.NewDecoder(strings.NewReader(string(encoded)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&row); err != nil {
		return fmt.Errorf("migrationexport: decode %s row: %w", table.Name, err)
	}
	if row.Table != table.Name || len(row.Values) != len(table.Columns) {
		return fmt.Errorf("migrationexport: invalid row shape for %s", table.Name)
	}
	for _, column := range table.Columns {
		value, exists := row.Values[column]
		if !exists || !validExportValue(value) {
			return fmt.Errorf("migrationexport: invalid %s.%s value", table.Name, column)
		}
	}
	return nil
}

func validExportValue(value exportValue) bool {
	switch value.Kind {
	case "null":
		return value.Text == ""
	case "integer":
		_, err := strconv.ParseInt(value.Text, 10, 64)
		return err == nil
	case "real":
		parsed, err := strconv.ParseFloat(value.Text, 64)
		return err == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0)
	case "blob":
		_, err := base64.StdEncoding.DecodeString(value.Text)
		return err == nil
	case "text":
		return true
	default:
		return false
	}
}
