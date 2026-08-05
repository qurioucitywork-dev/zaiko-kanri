package database

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

//go:embed migrations/000001_phase1.up.sql migrations/000002_inventory.up.sql migrations/000003_market.up.sql migrations/000004_sales_shipments.up.sql migrations/000005_requests_reservations.up.sql migrations/000006_approvals.up.sql migrations/000007_document_operations.up.sql
var schemaFS embed.FS

const (
	RoleAdmin  = "admin"
	RoleWorker = "worker"
	RoleGuest  = "guest"
)

type Store struct {
	db  *sql.DB
	now func() time.Time
}

type User struct {
	ID             string
	OrganizationID string
	Organization   string
	Username       string
	DisplayName    string
	Role           string
	Active         bool
	PasswordHash   string
}

type Session struct {
	TokenHash     string
	CSRFTokenHash string
	User          User
	ExpiresAt     time.Time
}

type Setting struct {
	Key          string
	Value        string
	ValueType    string
	IsConfigured bool
}

type AuditEntry struct {
	ID             string
	OrganizationID string
	ActorUserID    string
	ActorName      string
	TargetType     string
	TargetID       string
	Action         string
	BeforeJSON     string
	AfterJSON      string
	Reason         string
	Comment        string
	IPAddress      string
	UserAgent      string
	RequestID      string
	Result         string
	CreatedAt      time.Time
}

func Open(path string) (*Store, error) {
	dsn := path
	if path != ":memory:" && !strings.HasPrefix(path, "file:") {
		dsn = "file:" + path
	}
	orm, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db, err := orm.DB()
	if err != nil {
		return nil, fmt.Errorf("access database connection: %w", err)
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db, now: func() time.Time { return time.Now().UTC() }}
	if _, err := db.Exec(`PRAGMA foreign_keys = ON; PRAGMA busy_timeout = 5000; PRAGMA journal_mode = WAL;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("configure database: %w", err)
	}
	return store, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL
		)`); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}
	for _, migration := range []struct {
		version string
		path    string
	}{
		{"000001_phase1", "migrations/000001_phase1.up.sql"},
		{"000002_inventory", "migrations/000002_inventory.up.sql"},
		{"000003_market", "migrations/000003_market.up.sql"},
		{"000004_sales_shipments", "migrations/000004_sales_shipments.up.sql"},
		{"000005_requests_reservations", "migrations/000005_requests_reservations.up.sql"},
		{"000006_approvals", "migrations/000006_approvals.up.sql"},
		{"000007_document_operations", "migrations/000007_document_operations.up.sql"},
	} {
		var count int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version=?`, migration.version).Scan(&count); err != nil {
			return fmt.Errorf("check migration %s: %w", migration.version, err)
		}
		if count > 0 {
			continue
		}
		schema, err := schemaFS.ReadFile(migration.path)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", migration.version, err)
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(schema)); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", migration.version, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version,applied_at) VALUES(?,?)`, migration.version, s.now().Format(time.RFC3339Nano)); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %s: %w", migration.version, err)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func NewID(prefix string) (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return prefix + "_" + hex.EncodeToString(b[:]), nil
}

func RandomToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func TokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func HashPassword(password string) (string, error) {
	if len(password) < 12 {
		return "", errors.New("password must contain at least 12 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

func VerifyPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

var permissions = map[string]string{
	"dashboard.read":    "ダッシュボード閲覧",
	"inventory.read":    "在庫閲覧",
	"inventory.write":   "在庫更新",
	"purchase.read":     "仕入閲覧",
	"purchase.write":    "仕入更新",
	"purchase.confirm":  "仕入確定",
	"sales.read":        "売上閲覧",
	"sales.write":       "売上更新",
	"sales.confirm":     "売上確定",
	"sales.cancel":      "売上取消",
	"shipment.read":     "出荷閲覧",
	"shipment.write":    "出荷更新",
	"shipment.confirm":  "出荷確定",
	"shipment.cancel":   "出荷取消",
	"market.read":       "相場閲覧",
	"market.write":      "相場更新",
	"market.import":     "相場CSV取込",
	"inventory.publish": "商品公開管理",
	"request.read":      "購入依頼閲覧",
	"request.review":    "購入依頼審査",
	"approval.request":  "承認申請",
	"approval.read":     "承認閲覧",
	"approval.approve":  "承認実行",
	"audit.read":        "監査ログ閲覧",
	"users.manage":      "利用者管理",
	"settings.manage":   "設定管理",
}

func (s *Store) SeedPreview(ctx context.Context, adminPassword, workerPassword string) error {
	now := s.now().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := seedRolesAndPermissions(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO organizations(id,code,name,created_at,updated_at) VALUES('org_preview','PREVIEW','株式会社ウォッチプレミアム',?,?)`, now, now); err != nil {
		return err
	}
	for _, seed := range []struct{ id, username, display, role, password string }{
		{"usr_admin", "admin", "管理者", RoleAdmin, adminPassword},
		{"usr_worker", "worker", "山本 太郎", RoleWorker, workerPassword},
	} {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE organization_id='org_preview' AND username=?`, seed.username).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			hash, err := HashPassword(seed.password)
			if err != nil {
				return fmt.Errorf("hash preview password: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO users(id,organization_id,username,password_hash,display_name,role_key,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`,
				seed.id, "org_preview", seed.username, hash, seed.display, seed.role, now, now); err != nil {
				return err
			}
		}
	}
	for _, setting := range defaultOrganizationSettings() {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO organization_settings(organization_id,setting_key,setting_value,value_type,is_configured,updated_at) VALUES('org_preview',?,?,?,?,?)`,
			setting.Key, setting.Value, setting.ValueType, setting.IsConfigured, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func seedRolesAndPermissions(ctx context.Context, tx *sql.Tx) error {
	for _, role := range []struct{ key, name, description string }{
		{RoleAdmin, "管理者", "すべての業務機能と設定を管理"},
		{RoleWorker, "作業者", "仕入・在庫・売上・出荷を担当"},
	} {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO roles(role_key,name,description) VALUES(?,?,?)`, role.key, role.name, role.description); err != nil {
			return err
		}
	}
	for key, description := range permissions {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO permissions(permission_key,description) VALUES(?,?)`, key, description); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO role_permissions(role_key,permission_key) VALUES(?,?)`, RoleAdmin, key); err != nil {
			return err
		}
		if key != "approval.approve" && key != "audit.read" && key != "users.manage" && key != "settings.manage" &&
			key != "sales.cancel" && key != "shipment.cancel" {
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO role_permissions(role_key,permission_key) VALUES(?,?)`, RoleWorker, key); err != nil {
				return err
			}
		}
	}
	return nil
}

func defaultOrganizationSettings() []Setting {
	return []Setting{
		{"approval.purchase_threshold_jpy", "", "integer", false},
		{"approval.sales_threshold_jpy", "", "integer", false},
		{"approval.admin_high_value_enabled", "false", "boolean", false},
		{"approval.admin_high_value_threshold_jpy", "", "integer", false},
		{"reservation.duration_hours", "", "integer", false},
		{"exchange_rate.provider", "manual", "string", false},
		{"csv.encoding", "UTF-8-BOM", "string", false},
		{"security.admin_access_code", "", "string", false},
	}
}

func (s *Store) BootstrapOrganizationAdmin(ctx context.Context, organizationCode, organizationName, username, displayName, password string) (User, error) {
	organizationCode = strings.ToUpper(strings.TrimSpace(organizationCode))
	organizationName = strings.TrimSpace(organizationName)
	username = strings.TrimSpace(username)
	displayName = strings.TrimSpace(displayName)
	if organizationCode == "" || organizationName == "" || username == "" || displayName == "" {
		return User{}, errors.New("組織コード、組織名、ユーザー名、表示名は必須です")
	}
	passwordHash, err := HashPassword(password)
	if err != nil {
		return User{}, err
	}
	organizationID, err := NewID("org")
	if err != nil {
		return User{}, err
	}
	userID, err := NewID("usr")
	if err != nil {
		return User{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM organizations WHERE code=?`, organizationCode).Scan(&count); err != nil {
		return User{}, err
	}
	if count > 0 {
		return User{}, errors.New("同じ組織コードが既に存在します")
	}
	if err := seedRolesAndPermissions(ctx, tx); err != nil {
		return User{}, err
	}
	now := s.now().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO organizations(id,code,name,created_at,updated_at) VALUES(?,?,?,?,?)`,
		organizationID, organizationCode, organizationName, now, now); err != nil {
		return User{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO users(id,organization_id,username,password_hash,display_name,role_key,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?)`,
		userID, organizationID, username, passwordHash, displayName, RoleAdmin, now, now); err != nil {
		return User{}, err
	}
	for _, setting := range defaultOrganizationSettings() {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO organization_settings(
				organization_id,setting_key,setting_value,value_type,is_configured,updated_at
			) VALUES(?,?,?,?,?,?)`,
			organizationID, setting.Key, setting.Value, setting.ValueType, setting.IsConfigured, now); err != nil {
			return User{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return User{}, err
	}
	return User{
		ID: userID, OrganizationID: organizationID, Organization: organizationName,
		Username: username, DisplayName: displayName, Role: RoleAdmin, Active: true,
	}, nil
}

func (s *Store) Authenticate(ctx context.Context, organizationCode, username, password string) (User, error) {
	var u User
	var active int
	err := s.db.QueryRowContext(ctx, `
		SELECT u.id,u.organization_id,o.name,u.username,u.display_name,u.role_key,u.is_active,u.password_hash
		FROM users u JOIN organizations o ON o.id=u.organization_id
		WHERE o.code=? AND u.username=? AND u.deleted_at IS NULL AND o.is_active=1`,
		organizationCode, username,
	).Scan(&u.ID, &u.OrganizationID, &u.Organization, &u.Username, &u.DisplayName, &u.Role, &active, &u.PasswordHash)
	if err != nil {
		return User{}, errors.New("invalid credentials")
	}
	u.Active = active == 1
	if !u.Active || !VerifyPassword(u.PasswordHash, password) {
		return User{}, errors.New("invalid credentials")
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE users SET last_login_at=?,updated_at=? WHERE id=? AND organization_id=?`, s.now().Format(time.RFC3339Nano), s.now().Format(time.RFC3339Nano), u.ID, u.OrganizationID)
	u.PasswordHash = ""
	return u, nil
}

func (s *Store) CreateSession(ctx context.Context, user User, token, csrf, ip, userAgent string, ttl time.Duration) error {
	now := s.now()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions(token_hash,user_id,organization_id,csrf_token_hash,expires_at,created_at,last_seen_at,ip_address,user_agent)
		VALUES(?,?,?,?,?,?,?,?,?)`,
		TokenHash(token), user.ID, user.OrganizationID, TokenHash(csrf), now.Add(ttl).Format(time.RFC3339Nano),
		now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), ip, userAgent)
	return err
}

func (s *Store) CreateLoginCSRF(ctx context.Context, token string, ttl time.Duration) error {
	now := s.now()
	_, _ = s.db.ExecContext(ctx, `DELETE FROM login_csrf_tokens WHERE expires_at <= ?`, now.Format(time.RFC3339Nano))
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO login_csrf_tokens(token_hash,expires_at,created_at) VALUES(?,?,?)`,
		TokenHash(token), now.Add(ttl).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	return err
}

func (s *Store) ConsumeLoginCSRF(ctx context.Context, token string) bool {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false
	}
	defer tx.Rollback()
	var expires string
	if err := tx.QueryRowContext(ctx, `SELECT expires_at FROM login_csrf_tokens WHERE token_hash=?`, TokenHash(token)).Scan(&expires); err != nil {
		return false
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM login_csrf_tokens WHERE token_hash=?`, TokenHash(token)); err != nil {
		return false
	}
	parsed, err := time.Parse(time.RFC3339Nano, expires)
	if err != nil || !parsed.After(s.now()) {
		return false
	}
	return tx.Commit() == nil
}

func (s *Store) Session(ctx context.Context, token string) (Session, error) {
	var session Session
	var expires string
	var active int
	err := s.db.QueryRowContext(ctx, `
		SELECT s.token_hash,s.csrf_token_hash,s.expires_at,
		       u.id,u.organization_id,o.name,u.username,u.display_name,u.role_key,u.is_active
		FROM sessions s
		JOIN users u ON u.id=s.user_id AND u.organization_id=s.organization_id
		JOIN organizations o ON o.id=s.organization_id
		WHERE s.token_hash=? AND u.deleted_at IS NULL AND o.is_active=1`,
		TokenHash(token),
	).Scan(&session.TokenHash, &session.CSRFTokenHash, &expires,
		&session.User.ID, &session.User.OrganizationID, &session.User.Organization,
		&session.User.Username, &session.User.DisplayName, &session.User.Role, &active)
	if err != nil {
		return Session{}, errors.New("session not found")
	}
	session.User.Active = active == 1
	session.ExpiresAt, err = time.Parse(time.RFC3339Nano, expires)
	if err != nil || !session.User.Active || !session.ExpiresAt.After(s.now()) {
		_ = s.DeleteSession(ctx, token)
		return Session{}, errors.New("session expired")
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE sessions SET last_seen_at=? WHERE token_hash=?`, s.now().Format(time.RFC3339Nano), session.TokenHash)
	return session, nil
}

func (s *Store) DeleteSession(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash=?`, TokenHash(token))
	return err
}

func (s *Store) HasPermission(ctx context.Context, user User, permission string) bool {
	var effect string
	err := s.db.QueryRowContext(ctx, `SELECT effect FROM user_permissions WHERE user_id=? AND permission_key=?`, user.ID, permission).Scan(&effect)
	if err == nil {
		return effect == "allow"
	}
	var count int
	err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM role_permissions WHERE role_key=? AND permission_key=?`, user.Role, permission).Scan(&count)
	return err == nil && count > 0
}

func (s *Store) Users(ctx context.Context, organizationID string) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id,organization_id,username,display_name,role_key,is_active
		FROM users WHERE organization_id=? AND deleted_at IS NULL ORDER BY role_key,username`, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		var u User
		var active int
		if err := rows.Scan(&u.ID, &u.OrganizationID, &u.Username, &u.DisplayName, &u.Role, &active); err != nil {
			return nil, err
		}
		u.Active = active == 1
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *Store) Settings(ctx context.Context, organizationID string) ([]Setting, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT setting_key,setting_value,value_type,is_configured
		FROM organization_settings WHERE organization_id=? ORDER BY setting_key`, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var settings []Setting
	for rows.Next() {
		var setting Setting
		if err := rows.Scan(&setting.Key, &setting.Value, &setting.ValueType, &setting.IsConfigured); err != nil {
			return nil, err
		}
		settings = append(settings, setting)
	}
	return settings, rows.Err()
}

func (s *Store) UpdateSetting(ctx context.Context, organizationID, userID, key, value string) (Setting, Setting, error) {
	var before Setting
	err := s.db.QueryRowContext(ctx, `
		SELECT setting_key,setting_value,value_type,is_configured
		FROM organization_settings WHERE organization_id=? AND setting_key=?`, organizationID, key).
		Scan(&before.Key, &before.Value, &before.ValueType, &before.IsConfigured)
	if err != nil {
		return Setting{}, Setting{}, err
	}
	after := before
	after.Value = strings.TrimSpace(value)
	if err := validateSetting(after); err != nil {
		return Setting{}, Setting{}, err
	}
	after.IsConfigured = after.Value != ""
	_, err = s.db.ExecContext(ctx, `
		UPDATE organization_settings SET setting_value=?,is_configured=?,updated_by=?,updated_at=?
		WHERE organization_id=? AND setting_key=?`,
		after.Value, after.IsConfigured, userID, s.now().Format(time.RFC3339Nano), organizationID, key)
	return before, after, err
}

func validateSetting(setting Setting) error {
	if setting.Value == "" {
		return nil
	}
	switch setting.Key {
	case "approval.purchase_threshold_jpy", "approval.sales_threshold_jpy", "approval.admin_high_value_threshold_jpy", "reservation.duration_hours":
		value, err := strconv.ParseInt(setting.Value, 10, 64)
		if err != nil || value <= 0 {
			return errors.New("設定値には1以上の整数を入力してください")
		}
	case "exchange_rate.provider":
		if setting.Value != "manual" && setting.Value != "csv" {
			return errors.New("為替レート取得方法はmanualまたはcsvを指定してください")
		}
	case "csv.encoding":
		if setting.Value != "UTF-8" && setting.Value != "UTF-8-BOM" && setting.Value != "Shift_JIS" {
			return errors.New("対応していないCSV文字コードです")
		}
	case "approval.admin_high_value_enabled":
		if setting.Value != "true" && setting.Value != "false" {
			return errors.New("管理者高額承認モードはtrueまたはfalseを指定してください")
		}
	}
	return nil
}

func (s *Store) WriteAudit(ctx context.Context, entry AuditEntry) error {
	if entry.ID == "" {
		var err error
		entry.ID, err = NewID("aud")
		if err != nil {
			return err
		}
	}
	if entry.BeforeJSON == "" {
		entry.BeforeJSON = "{}"
	}
	if entry.AfterJSON == "" {
		entry.AfterJSON = "{}"
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = s.now()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO audit_logs(
			id,organization_id,actor_user_id,target_type,target_id,action,before_json,after_json,
			reason,comment,ip_address,user_agent,request_id,result,created_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		entry.ID, nullString(entry.OrganizationID), nullString(entry.ActorUserID), entry.TargetType, entry.TargetID,
		entry.Action, entry.BeforeJSON, entry.AfterJSON, entry.Reason, entry.Comment, entry.IPAddress,
		entry.UserAgent, entry.RequestID, entry.Result, entry.CreatedAt.Format(time.RFC3339Nano))
	return err
}

func (s *Store) AuditLogs(ctx context.Context, organizationID string, limit int) ([]AuditEntry, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.id,a.organization_id,COALESCE(a.actor_user_id,''),COALESCE(u.display_name,'システム'),
		       a.target_type,a.target_id,a.action,a.result,a.reason,a.request_id,a.created_at
		FROM audit_logs a LEFT JOIN users u ON u.id=a.actor_user_id
		WHERE a.organization_id=? ORDER BY a.created_at DESC LIMIT ?`, organizationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []AuditEntry
	for rows.Next() {
		var e AuditEntry
		var created string
		if err := rows.Scan(&e.ID, &e.OrganizationID, &e.ActorUserID, &e.ActorName, &e.TargetType, &e.TargetID, &e.Action, &e.Result, &e.Reason, &e.RequestID, &created); err != nil {
			return nil, err
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
