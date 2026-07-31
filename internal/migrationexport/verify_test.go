package migrationexport

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestVerifyAcceptsExportAndRejectsTampering(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`
		CREATE TABLE sample (
			id INTEGER PRIMARY KEY,
			value TEXT,
			payload BLOB
		);
		INSERT INTO sample(id, value, payload) VALUES (1, 'ok', X'0102');
	`); err != nil {
		t.Fatal(err)
	}

	artifact := filepath.Join(t.TempDir(), "artifact")
	manifest, err := Export(
		context.Background(),
		db,
		artifact,
		time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := Verify(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if len(verified.Tables) != len(manifest.Tables) {
		t.Fatalf("verified tables = %d, want %d", len(verified.Tables), len(manifest.Tables))
	}

	path := filepath.Join(artifact, "sample.ndjson")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("tampered\n"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(artifact); err == nil {
		t.Fatal("tampered artifact was accepted")
	}
}

func TestVerifyRejectsTraversalAndUnknownValueKind(t *testing.T) {
	t.Run("traversal", func(t *testing.T) {
		artifact := t.TempDir()
		manifest := `{
			"format_version": 1,
			"generated_at": "2026-07-31T00:00:00Z",
			"schema_version": 27,
			"tables": [{
				"name": "sample",
				"file": "../sample.ndjson",
				"columns": ["id"],
				"primary_key": ["id"],
				"row_count": 0,
				"checksum_sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
			}]
		}`
		if err := os.WriteFile(
			filepath.Join(artifact, "manifest.json"),
			[]byte(manifest),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
		if _, err := Verify(artifact); err == nil {
			t.Fatal("path traversal was accepted")
		}
	})

	t.Run("unknown-kind", func(t *testing.T) {
		artifact := t.TempDir()
		row := []byte("{\"table\":\"sample\",\"values\":{\"id\":{\"kind\":\"future\",\"text\":\"1\"}}}\n")
		sum := sha256.Sum256(row)
		manifest := fmt.Sprintf(`{
			"format_version": 1,
			"generated_at": "2026-07-31T00:00:00Z",
			"schema_version": 27,
			"tables": [{
				"name": "sample",
				"file": "sample.ndjson",
				"columns": ["id"],
				"primary_key": ["id"],
				"row_count": 1,
				"checksum_sha256": "%s"
			}]
		}`, hex.EncodeToString(sum[:]))
		if err := os.WriteFile(
			filepath.Join(artifact, "manifest.json"),
			[]byte(manifest),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(artifact, "sample.ndjson"), row, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Verify(artifact); err == nil {
			t.Fatal("unknown value kind was accepted")
		}
	})
}
