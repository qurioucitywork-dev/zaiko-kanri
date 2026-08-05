package persistence

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

//go:embed migrations/*.up.sql
var migrationFS embed.FS

type schemaMigration struct {
	Version  string
	Path     string
	Checksum string
	SQL      string
}

func migrationCatalog() ([]schemaMigration, error) {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("read PostgreSQL migrations: %w", err)
	}
	migrations := make([]schemaMigration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".up.sql") {
			continue
		}
		path := "migrations/" + entry.Name()
		contents, readErr := migrationFS.ReadFile(path)
		if readErr != nil {
			return nil, fmt.Errorf("read PostgreSQL migration %s: %w", path, readErr)
		}
		version := strings.TrimSuffix(entry.Name(), ".up.sql")
		sum := sha256.Sum256(contents)
		migrations = append(migrations, schemaMigration{
			Version: version, Path: path, SQL: string(contents), Checksum: hex.EncodeToString(sum[:]),
		})
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].Version < migrations[j].Version })
	return migrations, nil
}

// Migrate applies versioned PostgreSQL DDL exactly once and verifies that an
// already-applied migration has not been edited afterward.
func (r *Repository) Migrate(ctx context.Context) error {
	if r.driver != "postgres" {
		return nil
	}
	if err := r.db.WithContext(ctx).Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			checksum TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL
		)`).Error; err != nil {
		return fmt.Errorf("create PostgreSQL migration table: %w", err)
	}

	var lockAcquired bool
	if err := r.db.WithContext(ctx).
		Raw(`SELECT pg_try_advisory_lock(hashtext('zaiko_schema_migrations'))`).
		Scan(&lockAcquired).Error; err != nil {
		return fmt.Errorf("lock PostgreSQL migrations: %w", err)
	}
	if !lockAcquired {
		return fmt.Errorf("another process is applying PostgreSQL migrations")
	}
	defer r.db.WithContext(context.Background()).Exec(`SELECT pg_advisory_unlock(hashtext('zaiko_schema_migrations'))`)

	migrations, err := migrationCatalog()
	if err != nil {
		return err
	}
	for _, migration := range migrations {
		var applied struct{ Checksum string }
		result := r.db.WithContext(ctx).
			Raw(`SELECT checksum FROM schema_migrations WHERE version = ?`, migration.Version).
			Scan(&applied)
		if result.Error != nil {
			return fmt.Errorf("check PostgreSQL migration %s: %w", migration.Version, result.Error)
		}
		if result.RowsAffected > 0 {
			if applied.Checksum != migration.Checksum {
				return fmt.Errorf("PostgreSQL migration %s checksum mismatch", migration.Version)
			}
			continue
		}

		if err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if execErr := tx.Exec(migration.SQL).Error; execErr != nil {
				return execErr
			}
			return tx.Exec(`INSERT INTO schema_migrations(version, checksum, applied_at) VALUES(?, ?, ?)`,
				migration.Version, migration.Checksum, time.Now().UTC()).Error
		}); err != nil {
			return fmt.Errorf("apply PostgreSQL migration %s: %w", migration.Version, err)
		}
	}
	return nil
}

func (r *Repository) MigrationVersions(ctx context.Context) ([]string, error) {
	if r.driver != "postgres" {
		return nil, nil
	}
	var versions []string
	if err := r.db.WithContext(ctx).Table("schema_migrations").
		Order("version").Pluck("version", &versions).Error; err != nil {
		return nil, err
	}
	return versions, nil
}
