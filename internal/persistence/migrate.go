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

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
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
		// Git may check text files out with CRLF on Windows. Migration identity
		// must remain stable across operating systems, so hash canonical LF bytes
		// while executing the checked-out SQL unchanged.
		canonicalContents := strings.ReplaceAll(string(contents), "\r\n", "\n")
		canonicalContents = strings.ReplaceAll(canonicalContents, "\r", "\n")
		sum := sha256.Sum256([]byte(canonicalContents))
		migrations = append(migrations, schemaMigration{
			Version: version, Path: path, SQL: string(contents), Checksum: hex.EncodeToString(sum[:]),
		})
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].Version < migrations[j].Version })
	return migrations, nil
}

// MigrateConfigured uses a direct connection for DDL while the application can
// continue using a pooled runtime URL. SQLite is still migrated by database.Store.
func MigrateConfigured(ctx context.Context, cfg config.Config) error {
	if cfg.DatabaseDriver != "postgres" {
		return nil
	}
	if cfg.DatabaseMigrationURL != "" {
		cfg.DatabaseURL = cfg.DatabaseMigrationURL
	}
	repository, err := Open(cfg)
	if err != nil {
		return err
	}
	defer repository.Close()
	if err := repository.Migrate(ctx); err != nil {
		return err
	}
	return repository.AutoMigrateCore(ctx)
}

// Migrate applies versioned PostgreSQL DDL exactly once and verifies that an
// already-applied migration has not been edited afterward.
func (r *Repository) Migrate(ctx context.Context) error {
	if r.driver != "postgres" {
		return nil
	}
	// PostgreSQL advisory locks belong to a session. Pin every migration query,
	// including unlock, to the same physical connection instead of the pool.
	return r.db.WithContext(ctx).Connection(func(conn *gorm.DB) error {
		var lockAcquired bool
		if err := conn.Raw(`SELECT pg_try_advisory_lock(hashtext('zaiko_schema_migrations'))`).
			Scan(&lockAcquired).Error; err != nil {
			return fmt.Errorf("lock PostgreSQL migrations: %w", err)
		}
		if !lockAcquired {
			return fmt.Errorf("another process is applying PostgreSQL migrations")
		}
		defer func() {
			unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			conn.WithContext(unlockCtx).Exec(`SELECT pg_advisory_unlock(hashtext('zaiko_schema_migrations'))`)
		}()
		if err := conn.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
            version TEXT PRIMARY KEY, checksum TEXT NOT NULL, applied_at TIMESTAMPTZ NOT NULL
        )`).Error; err != nil {
			return fmt.Errorf("create PostgreSQL migration table: %w", err)
		}
		migrations, err := migrationCatalog()
		if err != nil {
			return err
		}
		for _, migration := range migrations {
			var applied struct{ Checksum string }
			result := conn.Raw(`SELECT checksum FROM schema_migrations WHERE version = ?`, migration.Version).Scan(&applied)
			if result.Error != nil {
				return fmt.Errorf("check PostgreSQL migration %s: %w", migration.Version, result.Error)
			}
			if result.RowsAffected > 0 {
				if applied.Checksum != migration.Checksum {
					return fmt.Errorf("PostgreSQL migration %s checksum mismatch", migration.Version)
				}
				continue
			}
			if err := conn.Transaction(func(tx *gorm.DB) error {
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
	})
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
