package persistence

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
)

func TestBootstrapCleanPostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_CLEAN_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires a disposable zaiko_clean_test_* PostgreSQL database")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasPrefix(parsed.Path, "/zaiko_clean_test_") {
		t.Fatal("refusing non-test database")
	}
	repo, err := Open(config.Config{DatabaseDriver: "postgres", DatabaseURL: dsn, Environment: "test"})
	if err != nil {
		t.Fatal("cannot open test database")
	}
	defer repo.Close()
	bytes := make([]byte, 24)
	if _, err := rand.Read(bytes); err != nil {
		t.Fatal(err)
	}
	password := hex.EncodeToString(bytes)
	ctx := context.Background()
	if err := repo.BootstrapClean(ctx, "CLEAN_TEST", "admin", password); err != nil {
		t.Fatal(err)
	}
	before, err := repo.AuditClean(ctx)
	if err != nil {
		t.Fatal(err)
	}
	versions, err := repo.MigrationVersions(ctx)
	catalog, catalogErr := migrationCatalog()
	if err != nil || catalogErr != nil || len(versions) != len(catalog) {
		t.Fatalf("full schema was not migrated: %d versions, %v", len(versions), err)
	}
	if len(before) == 0 {
		t.Fatal("no tables audited")
	}
	var hashBefore string
	repo.db.Raw(`SELECT password_hash FROM users WHERE username='admin'`).Scan(&hashBefore)
	if err := repo.BootstrapClean(ctx, "CLEAN_TEST", "admin", strings.Repeat("x", 32)); !errors.Is(err, ErrCleanDatabaseNotEmpty) {
		t.Fatalf("repeat must refuse: %v", err)
	}
	var hashAfter string
	repo.db.Raw(`SELECT password_hash FROM users WHERE username='admin'`).Scan(&hashAfter)
	if hashBefore != hashAfter {
		t.Fatal("password changed on repeated bootstrap")
	}
	if _, err := repo.AuditClean(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Authenticate(ctx, "CLEAN_TEST", "admin", password); err != nil {
		t.Fatal("administrator login failed")
	}
	var orgID string
	repo.db.Raw(`SELECT id FROM organizations`).Scan(&orgID)
	if _, err := repo.CompanyInfo(ctx, orgID); err != nil {
		t.Fatal(err)
	}
	// Nonempty unknown tables also block initialization, without destructive cleanup.
	if err := repo.db.Exec(`CREATE TABLE clean_test_canary(value INTEGER); INSERT INTO clean_test_canary VALUES(7)`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.AuditClean(ctx); err == nil {
		t.Fatal("audit accepted an unapproved table")
	}
	if err := repo.BootstrapClean(ctx, "CLEAN_TEST", "admin", password); !errors.Is(err, ErrCleanDatabaseNotEmpty) {
		t.Fatal("nonempty bootstrap was not rejected")
	}
	var count int64
	repo.db.Raw(`SELECT COUNT(*) FROM clean_test_canary WHERE value=7`).Scan(&count)
	if count != 1 {
		t.Fatal("existing records changed")
	}
}
