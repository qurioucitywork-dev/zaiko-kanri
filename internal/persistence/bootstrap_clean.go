package persistence

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"gorm.io/gorm"
)

var ErrCleanDatabaseNotEmpty = errors.New("clean bootstrap refuses an initialized or nonempty database")

type CleanTableCount struct {
	Table     string `json:"table"`
	Count     int64  `json:"count"`
	Exception string `json:"exception,omitempty"`
}

func cleanCounts(ctx context.Context, tx *gorm.DB) ([]CleanTableCount, error) {
	var tables []string
	if err := tx.WithContext(ctx).Raw(`SELECT tablename FROM pg_tables WHERE schemaname='public' ORDER BY tablename`).Scan(&tables).Error; err != nil {
		return nil, err
	}
	result := make([]CleanTableCount, 0, len(tables))
	for _, table := range tables {
		var count int64
		quoted := `"public"."` + strings.ReplaceAll(table, `"`, `""`) + `"`
		if err := tx.WithContext(ctx).Raw(`SELECT COUNT(*) FROM ` + quoted).Scan(&count).Error; err != nil {
			return nil, err
		}
		result = append(result, CleanTableCount{Table: table, Count: count})
	}
	return result, nil
}

// BootstrapClean runs migrations before creating the first organization, so
// historical backfills cannot manufacture rates for the new organization.
// A single connection and transaction protect the emptiness check and inserts.
// Re-execution refuses without changing passwords, permissions, or business data.
func (r *Repository) BootstrapClean(ctx context.Context, code, username, password string) error {
	if r.driver != "postgres" {
		return errors.New("clean bootstrap requires PostgreSQL")
	}
	if strings.TrimSpace(code) == "" || strings.EqualFold(code, "PREVIEW") || strings.TrimSpace(username) == "" || len(password) < 20 || len(password) > 72 {
		return errors.New("use a non-preview organization, username, and a 20-72 byte password")
	}
	return r.db.WithContext(ctx).Connection(func(conn *gorm.DB) error {
		return conn.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtext('zaiko_clean_bootstrap'))`).Error; err != nil {
				return err
			}
			before, err := cleanCounts(ctx, tx)
			if err != nil {
				return err
			}
			for _, row := range before {
				if row.Table != "schema_migrations" && row.Count != 0 {
					return ErrCleanDatabaseNotEmpty
				}
			}
			target := &Repository{db: tx, driver: "postgres"}
			if err := target.Migrate(ctx); err != nil {
				return err
			}
			if err := target.AutoMigrateCore(ctx); err != nil {
				return err
			}
			after, err := cleanCounts(ctx, tx)
			if err != nil {
				return err
			}
			for _, row := range after {
				if row.Table != "schema_migrations" && row.Count != 0 {
					return fmt.Errorf("migration populated %s; refusing bootstrap", row.Table)
				}
			}
			hash, err := database.HashPassword(password)
			if err != nil {
				return err
			}
			orgID, err := database.NewID("org")
			if err != nil {
				return err
			}
			userID, err := database.NewID("usr")
			if err != nil {
				return err
			}
			now := time.Now().UTC()
			for _, role := range []struct{ key, name string }{{database.RoleAdmin, "管理者"}, {database.RoleWorker, "作業者"}, {database.RoleGuest, "ゲスト"}} {
				if err := tx.Exec(`INSERT INTO roles(role_key,name,description) VALUES(?,?,'')`, role.key, role.name).Error; err != nil {
					return err
				}
			}
			keys := make([]string, 0, len(platformPermissions))
			for key := range platformPermissions {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				if err := tx.Exec(`INSERT INTO permissions(permission_key,description) VALUES(?,?)`, key, platformPermissions[key]).Error; err != nil {
					return err
				}
				if err := tx.Exec(`INSERT INTO role_permissions(role_key,permission_key) VALUES(?,?)`, database.RoleAdmin, key).Error; err != nil {
					return err
				}
				if !workerDeniedPermissions[key] {
					if err := tx.Exec(`INSERT INTO role_permissions(role_key,permission_key) VALUES(?,?)`, database.RoleWorker, key).Error; err != nil {
						return err
					}
				}
			}
			if err := tx.Exec(`INSERT INTO organizations(id,code,name,is_active,created_at,updated_at) VALUES(?,?,'新規環境（未設定）',TRUE,?,?)`, orgID, code, now, now).Error; err != nil {
				return err
			}
			if err := tx.Exec(`INSERT INTO organization_profiles(organization_id,updated_at) VALUES(?,?)`, orgID, now).Error; err != nil {
				return err
			}
			if err := tx.Exec(`INSERT INTO users(id,organization_id,username,password_hash,display_name,email,role_key,is_active,created_at,updated_at) VALUES(?,?,?,?,'初期管理者','',?,TRUE,?,?)`, userID, orgID, username, hash, database.RoleAdmin, now, now).Error; err != nil {
				return err
			}
			// Staff and editable settings are intentionally absent until configured.
			return nil
		})
	})
}

// AuditClean is read-only and fails if an unapproved table contains records.
func (r *Repository) AuditClean(ctx context.Context) ([]CleanTableCount, error) {
	if r.driver != "postgres" {
		return nil, errors.New("clean audit requires PostgreSQL")
	}
	var rows []CleanTableCount
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET TRANSACTION ISOLATION LEVEL REPEATABLE READ, READ ONLY`).Error; err != nil {
			return err
		}
		var err error
		rows, err = cleanCounts(ctx, tx)
		if err != nil {
			return err
		}
		allowed := map[string]struct {
			reason string
			max    int64
		}{
			"schema_migrations": {"versioned schema", 10000}, "roles": {"system roles", 3},
			"permissions": {"system permissions", int64(len(platformPermissions))}, "role_permissions": {"system role permissions", int64(2 * len(platformPermissions))},
			"organizations": {"new organization", 1}, "users": {"initial administrator", 1}, "organization_profiles": {"empty organization profile", 1},
			"sessions": {"new login sessions", 10000}, "login_csrf_tokens": {"new login CSRF tokens", 10000}, "audit_logs": {"new verification audit events", 10000},
		}
		var violations []string
		for i := range rows {
			a, ok := allowed[rows[i].Table]
			if ok {
				rows[i].Exception = a.reason
			}
			if rows[i].Count > 0 && (!ok || rows[i].Count > a.max) {
				violations = append(violations, rows[i].Table)
			}
		}
		if len(violations) > 0 {
			return fmt.Errorf("unexpected records: %s", strings.Join(violations, ", "))
		}
		return nil
	})
	return rows, err
}
