package persistence

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type identityReadLog struct {
	logger.Interface
	mu      sync.Mutex
	queries []string
}

func (l *identityReadLog) Trace(_ context.Context, _ time.Time, query func() (string, int64), _ error) {
	sql, _ := query()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.queries = append(l.queries, sql)
}
func (l *identityReadLog) reset()     { l.mu.Lock(); defer l.mu.Unlock(); l.queries = nil }
func (l *identityReadLog) count() int { l.mu.Lock(); defer l.mu.Unlock(); return len(l.queries) }

func identityReadFixture(t *testing.T) (*Repository, *identityReadLog, string, time.Time) {
	t.Helper()
	logs := &identityReadLog{Interface: logger.Default.LogMode(logger.Silent)}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logs})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	for _, sql := range []string{
		`CREATE TABLE organizations(id TEXT PRIMARY KEY,name TEXT,is_active BOOLEAN)`,
		`CREATE TABLE users(id TEXT PRIMARY KEY,organization_id TEXT,username TEXT,display_name TEXT,role_key TEXT,is_active BOOLEAN,deleted_at DATETIME)`,
		`CREATE TABLE sessions(token_hash TEXT PRIMARY KEY,csrf_token_hash TEXT,expires_at DATETIME,user_id TEXT,organization_id TEXT,last_seen_at DATETIME)`,
		`CREATE TABLE user_permissions(user_id TEXT,permission_key TEXT,effect TEXT,PRIMARY KEY(user_id,permission_key))`,
		`CREATE TABLE role_permissions(role_key TEXT,permission_key TEXT,PRIMARY KEY(role_key,permission_key))`,
		`INSERT INTO organizations VALUES('org','Test organization',TRUE)`,
		`INSERT INTO users VALUES('user','org','worker','Worker','worker',TRUE,NULL)`,
		`INSERT INTO role_permissions VALUES('worker','products.read')`,
	} {
		if err := db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	token := "identity-read-test-token"
	expiry := time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)
	if err := db.Exec(`INSERT INTO sessions VALUES(?,?,?,?,?,?)`, database.TokenHash(token), "csrf", expiry, "user", "org", time.Now().UTC()).Error; err != nil {
		t.Fatal(err)
	}
	logs.reset()
	return &Repository{db: db, driver: "sqlite"}, logs, token, expiry
}

func TestSessionRecentActivityDoesNotWriteOrExtendExpiry(t *testing.T) {
	repo, logs, token, expiry := identityReadFixture(t)
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		logs.reset()
		session, err := repo.Session(ctx, token)
		if err != nil {
			t.Fatal(err)
		}
		if !session.ExpiresAt.Equal(expiry) {
			t.Fatal("read changed absolute session expiry")
		}
		if logs.count() != 1 {
			t.Fatalf("recent session must require exactly one SELECT, got %d queries", logs.count())
		}
	}
	// Authorization is reloaded rather than cached during the activity interval.
	if err := repo.db.Exec(`UPDATE users SET is_active=FALSE WHERE id='user'`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Session(ctx, token); err == nil {
		t.Fatal("disabled user retained authorization")
	}
}

func TestSessionOldActivityTouchesOnceAndStillExpires(t *testing.T) {
	repo, logs, token, expiry := identityReadFixture(t)
	old := time.Now().UTC().Add(-2 * sessionActivityInterval)
	if err := repo.db.Exec(`UPDATE sessions SET last_seen_at=?`, old).Error; err != nil {
		t.Fatal(err)
	}
	logs.reset()
	session, err := repo.Session(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	if logs.count() != 2 {
		t.Fatalf("old activity should require one SELECT and one atomic UPDATE, got %d", logs.count())
	}
	if !session.ExpiresAt.Equal(expiry) {
		t.Fatal("activity extended expiry")
	}
	var row struct {
		LastSeenAt time.Time
		ExpiresAt  time.Time
	}
	if err := repo.db.Table("sessions").Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	if !row.LastSeenAt.After(old) || !row.ExpiresAt.Equal(expiry) {
		t.Fatal("activity update changed the wrong session fields")
	}
	logs.reset()
	if _, err = repo.Session(context.Background(), token); err != nil {
		t.Fatal(err)
	}
	if logs.count() != 1 {
		t.Fatal("second read still wrote activity")
	}
	if err := repo.db.Exec(`UPDATE sessions SET expires_at=?`, time.Now().UTC().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = repo.Session(context.Background(), token); err == nil {
		t.Fatal("recent activity bypassed absolute expiry")
	}
}

func TestSessionRevalidatesDeletedUsersAndOrganizations(t *testing.T) {
	for _, sql := range []string{`UPDATE organizations SET is_active=FALSE`, `UPDATE users SET deleted_at=CURRENT_TIMESTAMP`} {
		t.Run(sql, func(t *testing.T) {
			repo, _, token, _ := identityReadFixture(t)
			if _, err := repo.Session(context.Background(), token); err != nil {
				t.Fatal(err)
			}
			if err := repo.db.Exec(sql).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := repo.Session(context.Background(), token); err == nil {
				t.Fatal("revocation not visible on the next request")
			}
		})
	}
}

func TestPermissionSingleQueryPreservesLiveOverridePrecedence(t *testing.T) {
	repo, logs, _, _ := identityReadFixture(t)
	user := database.User{ID: "user", OrganizationID: "org", Role: "worker", Active: true}
	check := func(permission string, want bool) {
		t.Helper()
		logs.reset()
		if got := repo.HasPermission(context.Background(), user, permission); got != want {
			t.Fatalf("permission %s = %v, want %v", permission, got, want)
		}
		if logs.count() != 1 {
			t.Fatalf("permission must use one query, got %d", logs.count())
		}
	}
	check("products.read", true)
	check("settings.manage", false)
	if err := repo.db.Exec(`INSERT INTO user_permissions VALUES('user','products.read','deny')`).Error; err != nil {
		t.Fatal(err)
	}
	check("products.read", false)
	if err := repo.db.Exec(`UPDATE user_permissions SET effect='allow' WHERE user_id='user'`).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.db.Exec(`DELETE FROM role_permissions`).Error; err != nil {
		t.Fatal(err)
	}
	check("products.read", true)
	if err := repo.db.Exec(`DELETE FROM user_permissions`).Error; err != nil {
		t.Fatal(err)
	}
	check("products.read", false)
	user.Active = false
	logs.reset()
	if repo.HasPermission(context.Background(), user, "products.read") || logs.count() != 0 {
		t.Fatal("inactive user queried or received permission")
	}
	user.Active = true
	if err := repo.db.Exec(`DROP TABLE user_permissions`).Error; err != nil {
		t.Fatal(err)
	}
	if repo.HasPermission(context.Background(), user, "products.read") {
		t.Fatal("database error must deny access")
	}
	for _, query := range logs.queries {
		if strings.HasPrefix(strings.ToUpper(query), "BEGIN") {
			t.Fatal("permission check started a write transaction")
		}
	}
}

func TestPermissionDecisionsSingleQueryMatchesIndividualChecks(t *testing.T) {
	repo, logs, _, _ := identityReadFixture(t)
	user := database.User{ID: "user", OrganizationID: "org", Role: "worker", Active: true}
	for _, statement := range []string{
		`INSERT INTO role_permissions VALUES('worker','purchase.read')`,
		`INSERT INTO user_permissions VALUES('user','products.read','deny')`,
		`INSERT INTO user_permissions VALUES('user','settings.manage','allow')`,
		`INSERT INTO user_permissions VALUES('other-user','purchase.read','deny')`,
	} {
		if err := repo.db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	logs.reset()
	permissions, err := repo.PermissionDecisions(context.Background(), user)
	if err != nil || logs.count() != 1 {
		t.Fatalf("permissions error=%v queries=%d", err, logs.count())
	}
	for _, key := range []string{"products.read", "purchase.read", "settings.manage", "unknown"} {
		if permissions[key] != repo.HasPermission(context.Background(), user, key) {
			t.Fatalf("permission mismatch: %s", key)
		}
	}
	if err := repo.db.Exec(`UPDATE user_permissions SET effect='deny' WHERE user_id='user' AND permission_key='settings.manage'`).Error; err != nil {
		t.Fatal(err)
	}
	refreshed, err := repo.PermissionDecisions(context.Background(), user)
	if err != nil || refreshed["settings.manage"] || !permissions["settings.manage"] {
		t.Fatal("new request must reload permissions without mutating previous request snapshot")
	}
	user.Active = false
	logs.reset()
	disabled, err := repo.PermissionDecisions(context.Background(), user)
	if err != nil || len(disabled) != 0 || logs.count() != 0 {
		t.Fatal("disabled user received permissions")
	}
	user.Active = true
	if err := repo.db.Exec(`DROP TABLE user_permissions`).Error; err != nil {
		t.Fatal(err)
	}
	if decisions, err := repo.PermissionDecisions(context.Background(), user); err == nil || len(decisions) != 0 {
		t.Fatal("permission DB error must fail closed")
	}
}
