// Command cloud-prepare-data removes old credentials from an isolated recovery DB.
// It cannot connect to Neon or any non-local PostgreSQL server.
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

type account struct {
	Organization string `json:"organization"`
	ID           string `json:"id"`
	Username     string `json:"username"`
	Role         string `json:"role"`
	Active       bool   `json:"active"`
	Password     string `json:"password"`
}
type report struct {
	RotatedUsers           int   `json:"rotated_users"`
	DeletedSessions        int64 `json:"deleted_sessions"`
	DeletedCSRF            int64 `json:"deleted_login_csrf"`
	DeletedResetTokens     int64 `json:"deleted_reset_tokens"`
	ClearedAdminCodes      int64 `json:"cleared_admin_codes"`
	ScrubbedResetEmails    int64 `json:"scrubbed_reset_emails"`
	AuditCredentialMatches int64 `json:"audit_credential_matches"`
}

func main() {
	dbName := flag.String("staging-database", "", "explicit recovery DB name (zaiko_recovery prefix required)")
	org := flag.String("organization-code", "CLEAN", "organization owning the primary administrator")
	admin := flag.String("admin-username", "admin", "existing primary administrator username")
	passwordFile := flag.String("admin-password-file", "", "protected file containing the NEW cloud administrator password")
	accountsFile := flag.String("accounts-file", "", "new protected JSON output file; existing files are never overwritten")
	flag.Parse()
	result, err := prepare(os.Getenv("ZAIKO_DATABASE_URL"), *dbName, *org, *admin, *passwordFile, *accountsFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	_ = json.NewEncoder(os.Stdout).Encode(result)
}

func localOnly(host string) bool {
	if host == "localhost" || host == "/var/run/postgresql" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
func validateTarget(cfg *pgx.ConnConfig, expected string) error {
	if expected == "" || !strings.HasPrefix(expected, "zaiko_recovery") || cfg.Database != expected {
		return errors.New("refused: explicit zaiko_recovery staging database required")
	}
	if !localOnly(cfg.Host) {
		return errors.New("refused: only local PostgreSQL or its standard Unix socket is permitted")
	}
	for _, fallback := range cfg.Fallbacks {
		if !localOnly(fallback.Host) {
			return errors.New("refused: non-local PostgreSQL fallback")
		}
	}
	return nil
}
func prepare(connection, expected, org, admin, passwordFile, accountsFile string) (report, error) {
	var summary report
	cfg, err := pgx.ParseConfig(connection)
	if err != nil {
		return summary, errors.New("invalid protected staging database configuration")
	}
	if err = validateTarget(cfg, expected); err != nil {
		return summary, err
	}
	if passwordFile == "" || accountsFile == "" {
		return summary, errors.New("both protected credential file paths are required")
	}
	raw, err := os.ReadFile(passwordFile)
	if err != nil {
		return summary, errors.New("cannot read new administrator password file")
	}
	adminPassword := strings.TrimSpace(string(raw))
	if len(adminPassword) < 20 || len(adminPassword) > 72 {
		return summary, errors.New("new administrator password must be 20-72 bytes")
	}
	output, err := os.OpenFile(accountsFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return summary, errors.New("cannot create exclusive protected credentials output; no changes made")
	}
	committed := false
	defer func() {
		_ = output.Close()
		if !committed {
			_ = os.Remove(accountsFile)
		}
	}()
	if err = output.Chmod(0600); err != nil {
		return summary, errors.New("cannot protect credentials output")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cfg.ConnectTimeout = 10 * time.Second
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return summary, errors.New("staging database connection failed")
	}
	defer conn.Close(context.Background())
	var actual string
	if err = conn.QueryRow(ctx, "SELECT current_database()").Scan(&actual); err != nil || actual != expected {
		return summary, errors.New("staging database identity check failed")
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		return summary, errors.New("cannot begin staging transaction")
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('zaiko_cloud_prepare_data'))"); err != nil {
		return summary, errors.New("cannot lock staging preparation")
	}
	// Existing audit records normally exclude credentials. Detect unexpected keys
	// and stop for review instead of modifying immutable business history.
	pattern := `"(password(_?hash)?|secret|api_?key|authorization|csrf_?token(_?hash)?|access_?token|refresh_?token|reset_?url|admin_?access_?code)"[[:space:]]*:`
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE before_json::text ~* $1 OR after_json::text ~* $1`, pattern).Scan(&summary.AuditCredentialMatches); err != nil {
		return summary, errors.New("cannot audit historical credential keys")
	}
	if summary.AuditCredentialMatches != 0 {
		return summary, errors.New("unexpected credential keys in audit history; review required before any changes")
	}
	var unknownSettings int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM organization_settings WHERE setting_key <> 'security.admin_access_code' AND setting_key ~* '(password|secret|credential|api.?key|token)' AND setting_value <> ''`).Scan(&unknownSettings); err != nil {
		return summary, errors.New("cannot audit credential settings")
	}
	if unknownSettings != 0 {
		return summary, errors.New("unexpected credential-bearing setting; review required before changes")
	}
	before, err := tableCounts(ctx, tx)
	if err != nil {
		return summary, errors.New("cannot count staging tables")
	}
	rows, err := tx.Query(ctx, `SELECT o.code,u.id,u.username,u.role_key,u.is_active FROM users u JOIN organizations o ON o.id=u.organization_id ORDER BY o.code,u.username FOR UPDATE OF u`)
	if err != nil {
		return summary, errors.New("cannot read staging account identities")
	}
	var accounts []account
	primaryFound := false
	for rows.Next() {
		var item account
		if err = rows.Scan(&item.Organization, &item.ID, &item.Username, &item.Role, &item.Active); err != nil {
			rows.Close()
			return summary, errors.New("cannot read staging account identities")
		}
		if item.Organization == org && item.Username == admin {
			if item.Role != "admin" || !item.Active {
				rows.Close()
				return summary, errors.New("primary administrator must already be active and administrator role")
			}
			item.Password = adminPassword
			primaryFound = true
		} else {
			buffer := make([]byte, 24)
			if _, err = rand.Read(buffer); err != nil {
				rows.Close()
				return summary, errors.New("secure password generation failed")
			}
			item.Password = base64.RawURLEncoding.EncodeToString(buffer)
		}
		accounts = append(accounts, item)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return summary, errors.New("cannot finish staging account scan")
	}
	if !primaryFound {
		return summary, errors.New("existing primary administrator was not found")
	}
	for _, item := range accounts {
		hash, hashErr := bcrypt.GenerateFromPassword([]byte(item.Password), bcrypt.DefaultCost)
		if hashErr != nil {
			return summary, errors.New("new password hashing failed")
		}
		command, updateErr := tx.Exec(ctx, "UPDATE users SET password_hash=$1,updated_at=now() WHERE id=$2", string(hash), item.ID)
		if updateErr != nil || command.RowsAffected() != 1 {
			return summary, errors.New("staging account update failed")
		}
		summary.RotatedUsers++
	}
	commands := []struct {
		sql   string
		count *int64
	}{
		{"DELETE FROM sessions", &summary.DeletedSessions},
		{"DELETE FROM login_csrf_tokens", &summary.DeletedCSRF},
		{"DELETE FROM password_reset_tokens", &summary.DeletedResetTokens},
		{"UPDATE organization_settings SET setting_value='',is_configured=FALSE,updated_by=NULL,updated_at=now() WHERE setting_key='security.admin_access_code'", &summary.ClearedAdminCodes},
		{`UPDATE email_outbox SET payload_json=payload_json-'resetUrl'-'token'-'password'-'secret',last_error='',status=CASE WHEN status IN ('pending','sending','failed') THEN 'cancelled' ELSE status END,updated_at=now() WHERE template_key='password_reset'`, &summary.ScrubbedResetEmails},
	}
	for _, command := range commands {
		result, execErr := tx.Exec(ctx, command.sql)
		if execErr != nil {
			return summary, errors.New("staging authentication cleanup failed")
		}
		*command.count = result.RowsAffected()
	}
	after, err := tableCounts(ctx, tx)
	if err != nil {
		return summary, errors.New("cannot verify staging row counts")
	}
	for table, count := range before {
		if table == "sessions" || table == "login_csrf_tokens" || table == "password_reset_tokens" {
			continue
		}
		if after[table] != count {
			return summary, fmt.Errorf("refused: non-auth table row count changed: %s", table)
		}
	}
	if err = json.NewEncoder(output).Encode(map[string]any{"createdAt": time.Now().UTC(), "stagingDatabase": expected, "accounts": accounts}); err != nil {
		return summary, errors.New("cannot save protected new credentials")
	}
	if err = output.Sync(); err != nil {
		return summary, errors.New("cannot flush protected new credentials")
	}
	if err = tx.Commit(ctx); err != nil {
		return summary, errors.New("staging credential transaction failed")
	}
	committed = true
	return summary, nil
}

func tableCounts(ctx context.Context, tx pgx.Tx) (map[string]int64, error) {
	rows, err := tx.Query(ctx, "SELECT tablename FROM pg_tables WHERE schemaname='public' ORDER BY tablename")
	if err != nil {
		return nil, err
	}
	var tables []string
	for rows.Next() {
		var table string
		if err = rows.Scan(&table); err != nil {
			rows.Close()
			return nil, err
		}
		tables = append(tables, table)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	result := make(map[string]int64, len(tables))
	for _, table := range tables {
		var count int64
		if err = tx.QueryRow(ctx, "SELECT COUNT(*) FROM "+pgx.Identifier{"public", table}.Sanitize()).Scan(&count); err != nil {
			return nil, err
		}
		result[table] = count
	}
	return result, nil
}
