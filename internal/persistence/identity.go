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
	}
	tokenHash := database.TokenHash(token)
	result := r.db.WithContext(ctx).Table("sessions AS s").
		Select(`s.token_hash, s.csrf_token_hash, s.expires_at, u.id AS user_id,
			u.organization_id, o.name AS organization, u.username, u.display_name,
			u.role_key AS role, u.is_active AS active, o.is_active AS organization_on`).
		Joins("JOIN users AS u ON u.id = s.user_id AND u.organization_id = s.organization_id").
		Joins("JOIN organizations AS o ON o.id = s.organization_id").
		Where("s.token_hash = ? AND u.deleted_at IS NULL", tokenHash).
		Take(&row)
	if result.Error != nil {
		return database.Session{}, errors.New("session not found")
	}
	if !row.Active || !row.OrganizationOn || !row.ExpiresAt.After(time.Now().UTC()) {
		_ = r.DeleteSession(ctx, token)
		return database.Session{}, errors.New("session expired")
	}
	_ = r.db.WithContext(ctx).Table("sessions").Where("token_hash = ?", tokenHash).
		Update("last_seen_at", time.Now().UTC()).Error
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
	var override struct{ Effect string }
	result := r.db.WithContext(ctx).Table("user_permissions").Select("effect").
		Where("user_id = ? AND permission_key = ?", user.ID, permission).Take(&override)
	if result.Error == nil {
		return override.Effect == "allow"
	}
	if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return false
	}
	var count int64
	if err := r.db.WithContext(ctx).Table("role_permissions").
		Where("role_key = ? AND permission_key = ?", user.Role, permission).Count(&count).Error; err != nil {
		return false
	}
	return count > 0
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
