package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"gorm.io/gorm"
)

func (r *Repository) Authenticate(ctx context.Context, organizationCode, username, password string) (database.User, error) {
	var row struct {
		ID             string
		OrganizationID string
		Organization   string
		Username       string
		DisplayName    string
		Role           string
		Active         bool
		PasswordHash   string
	}
	result := r.db.WithContext(ctx).Table("users AS u").
		Select(`u.id, u.organization_id, o.name AS organization, u.username,
			u.display_name, u.role_key AS role, u.is_active AS active, u.password_hash`).
		Joins("JOIN organizations AS o ON o.id = u.organization_id").
		Where("o.code = ? AND u.username = ? AND u.deleted_at IS NULL AND o.is_active", organizationCode, strings.TrimSpace(username)).
		Take(&row)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		resolved, err := r.guestLoginUsername(ctx, organizationCode, username)
		if err == nil && resolved != strings.TrimSpace(username) {
			return r.Authenticate(ctx, organizationCode, resolved, password)
		}
	}
	if result.Error != nil || !row.Active || !database.VerifyPassword(row.PasswordHash, password) {
		return database.User{}, errors.New("invalid credentials")
	}
	now := time.Now().UTC()
	if err := r.db.WithContext(ctx).Table("users").Where("id = ? AND organization_id = ?", row.ID, row.OrganizationID).
		Updates(map[string]any{"last_login_at": now, "updated_at": now}).Error; err != nil {
		return database.User{}, err
	}
	return database.User{
		ID: row.ID, OrganizationID: row.OrganizationID, Organization: row.Organization,
		Username: row.Username, DisplayName: row.DisplayName, Role: row.Role, Active: row.Active,
	}, nil
}

func (r *Repository) CreateSession(ctx context.Context, user database.User, token, csrf, ip, userAgent string, ttl time.Duration) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`DELETE FROM sessions WHERE expires_at <= ?`, now).Error; err != nil {
			return err
		}
		return tx.Exec(`
			INSERT INTO sessions(token_hash,user_id,organization_id,csrf_token_hash,expires_at,created_at,last_seen_at,ip_address,user_agent)
			VALUES(?,?,?,?,?,?,?,?,?)`,
			database.TokenHash(token), user.ID, user.OrganizationID, database.TokenHash(csrf), now.Add(ttl), now, now, ip, userAgent).Error
	})
}

// Only the informational activity timestamp is throttled. Every request still
// reloads session expiry and the current user/organization authorization.
const sessionActivityInterval = time.Minute

func (r *Repository) Session(ctx context.Context, token string) (database.Session, error) {
	var row struct {
		TokenHash      string
		CSRFTokenHash  string
		ExpiresAt      time.Time
		UserID         string
		OrganizationID string
		Organization   string
		Username       string
		DisplayName    string
		Role           string
		Active         bool
		OrganizationOn bool
		LastSeenAt     time.Time
	}
	tokenHash := database.TokenHash(token)
	result := r.db.WithContext(ctx).Table("sessions AS s").
		Select(`s.token_hash, s.csrf_token_hash, s.expires_at, s.last_seen_at, u.id AS user_id,
			u.organization_id, o.name AS organization, u.username, u.display_name,
			u.role_key AS role, u.is_active AS active, o.is_active AS organization_on`).
		Joins("JOIN users AS u ON u.id = s.user_id AND u.organization_id = s.organization_id").
		Joins("JOIN organizations AS o ON o.id = s.organization_id").
		Where("s.token_hash = ? AND u.deleted_at IS NULL", tokenHash).
		Take(&row)
	if result.Error != nil {
		return database.Session{}, errors.New("session not found")
	}
	now := time.Now().UTC()
	if !row.Active || !row.OrganizationOn || !row.ExpiresAt.After(now) {
		_ = r.DeleteSession(ctx, token)
		return database.Session{}, errors.New("session expired")
	}
	cutoff := now.Add(-sessionActivityInterval)
	if row.LastSeenAt.Before(cutoff) {
		// A single conditional UPDATE avoids a transaction/row lock for every
		// parallel page request. The guard also handles concurrent stale reads.
		_ = r.db.WithContext(ctx).Exec(`UPDATE sessions SET last_seen_at = ?
			WHERE token_hash = ? AND last_seen_at < ?`, now, tokenHash, cutoff).Error
	}
	return database.Session{
		TokenHash: row.TokenHash, CSRFTokenHash: row.CSRFTokenHash, ExpiresAt: row.ExpiresAt,
		User: database.User{
			ID: row.UserID, OrganizationID: row.OrganizationID, Organization: row.Organization,
			Username: row.Username, DisplayName: row.DisplayName, Role: row.Role, Active: row.Active,
		},
	}, nil
}

func (r *Repository) DeleteSession(ctx context.Context, token string) error {
	return r.db.WithContext(ctx).Exec(`DELETE FROM sessions WHERE token_hash = ?`, database.TokenHash(token)).Error
}

func (r *Repository) HasPermission(ctx context.Context, user database.User, permission string) bool {
	if !user.Active {
		return false
	}
	// Explicit user overrides win, including deny. Keep authorization live in
	// the database while resolving both sources in one network round trip.
	var allowed bool
	err := r.db.WithContext(ctx).Raw(`SELECT COALESCE(
		(SELECT effect = 'allow' FROM user_permissions WHERE user_id = ? AND permission_key = ?),
		EXISTS(SELECT 1 FROM role_permissions WHERE role_key = ? AND permission_key = ?)
	) AS allowed`, user.ID, permission, user.Role, permission).Scan(&allowed).Error
	return err == nil && allowed
}

// PermissionDecisions returns one live authorization snapshot for a single
// read-batch request. It must not be retained across HTTP requests.
func (r *Repository) PermissionDecisions(ctx context.Context, user database.User) (map[string]bool, error) {
	decisions := make(map[string]bool)
	if !user.Active {
		return decisions, nil
	}
	var rows []struct {
		PermissionKey string
		Effect        string
		Priority      int
	}
	err := r.db.WithContext(ctx).Raw(`
        SELECT permission_key, 'allow' AS effect, 0 AS priority FROM role_permissions WHERE role_key = ?
        UNION ALL
        SELECT permission_key, effect, 1 AS priority FROM user_permissions WHERE user_id = ?
        ORDER BY priority`, user.Role, user.ID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		decisions[row.PermissionKey] = row.Effect == "allow"
	}
	return decisions, nil
}

func (r *Repository) WriteAudit(ctx context.Context, entry database.AuditEntry) error {
	if entry.ID == "" {
		id, err := database.NewID("aud")
		if err != nil {
			return err
		}
		entry.ID = id
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	entry.BeforeJSON = validJSONObject(entry.BeforeJSON)
	entry.AfterJSON = validJSONObject(entry.AfterJSON)
	return r.db.WithContext(ctx).Exec(`
		INSERT INTO audit_logs(
			id,organization_id,actor_user_id,target_type,target_id,action,before_json,after_json,
			reason,comment,ip_address,user_agent,request_id,result,created_at
		) VALUES(?,NULLIF(?,''),NULLIF(?,''),?,?,?,?::jsonb,?::jsonb,?,?,?,?,?,?,?)`,
		entry.ID, entry.OrganizationID, entry.ActorUserID, entry.TargetType, entry.TargetID, entry.Action,
		entry.BeforeJSON, entry.AfterJSON, entry.Reason, entry.Comment, entry.IPAddress, entry.UserAgent,
		entry.RequestID, entry.Result, entry.CreatedAt).Error
}

func validJSONObject(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "{}"
	}
	var object map[string]any
	if json.Unmarshal([]byte(value), &object) != nil {
		return "{}"
	}
	return value
}
