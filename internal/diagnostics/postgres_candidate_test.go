package diagnostics

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// TestPostgresCandidateSafetyInvariants is deliberately static. The candidate
// still requires an isolated PostgreSQL 16 apply test before staging use, but
// these checks prevent accidental loss of the most important safety guards.
func TestPostgresCandidateSafetyInvariants(t *testing.T) {
	root := diagnosticsRepositoryRoot(t)
	up := readTestSQL(t, filepath.Join(
		root, "deploy", "aws", "postgres", "migrations", "000001_initial_schema.up.sql",
	))
	down := readTestSQL(t, filepath.Join(
		root, "deploy", "aws", "postgres", "migrations", "000001_initial_schema.down.sql",
	))
	verify := readTestSQL(t, filepath.Join(
		root, "deploy", "aws", "postgres", "verify_schema.sql",
	))

	for _, required := range []string{
		`\set ON_ERROR_STOP on`,
		`BEGIN;`,
		`SET LOCAL TIME ZONE 'UTC';`,
		`pg_advisory_xact_lock`,
		`CREATE TABLE zaiko.idempotency_records`,
		`CREATE TABLE zaiko.audit_logs`,
		`CREATE TABLE zaiko.product_objects`,
		`CREATE TABLE zaiko.inventory_events`,
		`COMMIT;`,
	} {
		if !strings.Contains(up, required) {
			t.Errorf("PostgreSQL candidate is missing %q", required)
		}
	}
	if count := strings.Count(up, "CREATE TABLE zaiko."); count < 50 {
		t.Errorf("PostgreSQL candidate table count = %d, want at least 50", count)
	}
	if regexp.MustCompile(`(?i)\b(?:REAL|DOUBLE\s+PRECISION)\b`).MatchString(up) {
		t.Error("PostgreSQL candidate contains a floating-point storage type")
	}
	if regexp.MustCompile(`(?i)\bTIMESTAMP\s+(?:WITHOUT\s+TIME\s+ZONE)?\b`).MatchString(up) {
		t.Error("PostgreSQL candidate contains a timestamp without time zone")
	}
	if !regexp.MustCompile(`(?i)amount_minor\s+BIGINT`).MatchString(up) {
		t.Error("PostgreSQL candidate has no BIGINT amount_minor column")
	}
	for _, permission := range []string{
		"inventory.write",
		"purchase.confirm",
		"sales.confirm",
		"shipment.confirm",
	} {
		if !strings.Contains(up, "('"+permission+"'") {
			t.Errorf("PostgreSQL candidate is missing mutation permission %q", permission)
		}
		if !strings.Contains(verify, "('"+permission+"')") {
			t.Errorf("verification SQL does not check mutation permission %q", permission)
		}
	}

	for _, required := range []string{
		`zaiko.allow_destructive_rollback`,
		`IS DISTINCT FROM 'on'`,
		`DROP SCHEMA zaiko CASCADE`,
	} {
		if !strings.Contains(down, required) {
			t.Errorf("destructive rollback guard is missing %q", required)
		}
	}
	if !strings.Contains(verify, "SET default_transaction_read_only = on;") {
		t.Error("verification SQL is not explicitly read-only")
	}
	for _, required := range []string{
		"RAISE EXCEPTION 'required mutation permission is missing'",
		"RAISE EXCEPTION 'tenant-owned table is missing organization_id'",
		"RAISE EXCEPTION 'money minor-unit column is not bigint'",
	} {
		if !strings.Contains(verify, required) {
			t.Errorf("verification SQL is missing fail-fast guard %q", required)
		}
	}
}

func diagnosticsRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return root
}
