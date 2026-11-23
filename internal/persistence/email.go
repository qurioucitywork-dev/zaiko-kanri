package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"gorm.io/gorm"
)

var (
	ErrPasswordResetInvalid = errors.New("password reset token is invalid")
	ErrEmailUnavailable     = errors.New("user email is unavailable")
)

type EmailOutboxRecord struct {
	ID                string          `json:"id"`
	RecipientUserID   string          `json:"recipientUserId,omitempty"`
	RecipientEmail    string          `json:"recipientEmail"`
	TemplateKey       string          `json:"templateKey"`
	Subject           string          `json:"subject"`
	Payload           json.RawMessage `gorm:"column:payload_json" json:"payload"`
	Status            string          `json:"status"`
	ProviderMessageID string          `json:"providerMessageId,omitempty"`
	Attempts          int             `json:"attempts"`
	LastError         string          `json:"lastError,omitempty"`
	SentAt            *time.Time      `json:"sentAt,omitempty"`
	CreatedAt         time.Time       `json:"createdAt"`
	UpdatedAt         time.Time       `json:"updatedAt"`
}

type PasswordResetResult struct {
	OrganizationID string
	UserID         string
}

func (r *Repository) EmailOutbox(ctx context.Context, organizationID string, limit int) ([]EmailOutboxRecord, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	var records []EmailOutboxRecord
	err := r.db.WithContext(ctx).Table("email_outbox").
		Select(`id,COALESCE(recipient_user_id,'') AS recipient_user_id,recipient_email,template_key,subject,
			payload_json,status,provider_message_id,attempts,last_error,sent_at,created_at,updated_at`).
		Where("organization_id=?", organizationID).Order("created_at DESC,id DESC").Limit(limit).Scan(&records).Error
	return records, err
}

func (r *Repository) QueuePasswordReset(ctx context.Context, organizationID, userID, actorUserID, baseURL string) (EmailOutboxRecord, string, error) {
	var record EmailOutboxRecord
	rawToken, err := database.RandomToken()
	if err != nil {
		return record, "", err
	}
	resetID, err := database.NewID("rst")
	if err != nil {
		return record, "", err
	}
	mailID, err := database.NewID("eml")
	if err != nil {
		return record, "", err
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var target struct {
			Email, DisplayName string
			IsActive           bool
		}
		result := tx.Table("users").Select("email,display_name,is_active").Where(
			"organization_id=? AND id=? AND deleted_at IS NULL", organizationID, userID).Take(&target)
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return ErrUserNotFound
		}
		if result.Error != nil {
			return result.Error
		}
		if !target.IsActive || strings.TrimSpace(target.Email) == "" {
			return ErrEmailUnavailable
		}
		now := time.Now().UTC()
		expiresAt := now.Add(time.Hour)
		if err := tx.Exec(`UPDATE password_reset_tokens SET expires_at=?
			WHERE organization_id=? AND user_id=? AND used_at IS NULL AND expires_at>?`, now, organizationID, userID, now).Error; err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO password_reset_tokens(
			id,organization_id,user_id,token_hash,expires_at,created_by,created_at
		) VALUES(?,?,?,?,?,?,?)`, resetID, organizationID, userID, database.TokenHash(rawToken), expiresAt, actorUserID, now).Error; err != nil {
			return err
		}
		resetURL := strings.TrimRight(baseURL, "/") + "/app/reset-password?token=" + url.QueryEscape(rawToken)
		payload, _ := json.Marshal(map[string]any{"displayName": target.DisplayName, "resetUrl": resetURL, "expiresAt": expiresAt})
		record = EmailOutboxRecord{ID: mailID, RecipientUserID: userID, RecipientEmail: target.Email,
			TemplateKey: "password_reset", Subject: "在庫管理ツール パスワード再設定", Payload: payload,
			Status: "pending", CreatedAt: now, UpdatedAt: now}
		return tx.Exec(`INSERT INTO email_outbox(
			id,organization_id,recipient_user_id,recipient_email,template_key,subject,payload_json,status,
			requested_by,created_at,updated_at
		) VALUES(?,?,?,?,?,?,?::jsonb,'pending',?,?,?)`, record.ID, organizationID, userID, record.RecipientEmail,
			record.TemplateKey, record.Subject, string(payload), actorUserID, now, now).Error
	})
	if err != nil {
		return EmailOutboxRecord{}, "", err
	}
	return record, rawToken, nil
}

func (r *Repository) CompletePasswordReset(ctx context.Context, rawToken, newPassword string) (PasswordResetResult, error) {
	if strings.TrimSpace(rawToken) == "" || len(newPassword) < 8 || len(newPassword) > 256 {
		return PasswordResetResult{}, ErrPasswordResetInvalid
	}
	hash, err := database.HashPassword(newPassword)
	if err != nil {
		return PasswordResetResult{}, err
	}
	var completed PasswordResetResult
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		var token struct {
			ID, OrganizationID, UserID string
		}
		result := tx.Raw(`SELECT id,organization_id,user_id FROM password_reset_tokens
			WHERE token_hash=? AND used_at IS NULL AND expires_at>? FOR UPDATE`, database.TokenHash(rawToken), now).Scan(&token)
		if result.Error != nil {
			return result.Error
		}
		if token.ID == "" {
			return ErrPasswordResetInvalid
		}
		update := tx.Table("users").Where("id=? AND organization_id=? AND is_active AND deleted_at IS NULL", token.UserID, token.OrganizationID).
			Updates(map[string]any{"password_hash": hash, "updated_at": now})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected == 0 {
			return ErrPasswordResetInvalid
		}
		if err := tx.Table("password_reset_tokens").Where("id=?", token.ID).Update("used_at", now).Error; err != nil {
			return err
		}
		if err := tx.Exec(`DELETE FROM sessions WHERE user_id=?`, token.UserID).Error; err != nil {
			return err
		}
		completed = PasswordResetResult{OrganizationID: token.OrganizationID, UserID: token.UserID}
		return nil
	})
	return completed, err
}
