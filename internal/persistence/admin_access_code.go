package persistence

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

const adminAccessCodeSettingKey = "security.admin_access_code"

var japanLocation = time.FixedZone("Asia/Tokyo", 9*60*60)

// AdminAccessCodeRecord is the organization-wide, short-lived code used when a
// worker needs an administrator to authorize access to a restricted screen.
type AdminAccessCodeRecord struct {
	Code          string    `json:"code"`
	UpdatedAt     time.Time `json:"updatedAt"`
	NextRefreshAt time.Time `json:"nextRefreshAt"`
}

type adminAccessCodeRow struct {
	Value     string
	UpdatedAt string
}

func parseAdminAccessCodeTime(source string) (time.Time, error) {
	for _, layout := range []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999-07",
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999999",
	} {
		parsed, err := time.Parse(layout, source)
		if err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid admin access code time %q", source)
}

// AdminAccessCode returns today's code. A missing, malformed, or previous-day
// code is replaced before it is returned.
func (r *Repository) AdminAccessCode(ctx context.Context, organizationID, actorUserID string) (AdminAccessCodeRecord, error) {
	return r.ensureAdminAccessCode(ctx, organizationID, actorUserID, false)
}

// RotateAdminAccessCode invalidates the current code immediately and returns a
// newly generated one.
func (r *Repository) RotateAdminAccessCode(ctx context.Context, organizationID, actorUserID string) (AdminAccessCodeRecord, error) {
	return r.ensureAdminAccessCode(ctx, organizationID, actorUserID, true)
}

// VerifyAdminAccessCode refreshes the daily code when necessary and performs a
// constant-time comparison without exposing the code to the worker.
func (r *Repository) VerifyAdminAccessCode(ctx context.Context, organizationID, actorUserID, candidate string) (bool, error) {
	record, err := r.ensureAdminAccessCode(ctx, organizationID, actorUserID, false)
	if err != nil {
		return false, err
	}
	candidate = strings.ToUpper(strings.TrimSpace(candidate))
	if len(candidate) != 6 {
		return false, nil
	}
	return subtle.ConstantTimeCompare([]byte(record.Code), []byte(candidate)) == 1, nil
}

func (r *Repository) ensureAdminAccessCode(ctx context.Context, organizationID, actorUserID string, force bool) (AdminAccessCodeRecord, error) {
	var result AdminAccessCodeRecord
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row adminAccessCodeRow
		query := tx.Table("organization_settings").
			Select("setting_value AS value,CAST(updated_at AS TEXT) AS updated_at").
			Where("organization_id=? AND setting_key=?", organizationID, adminAccessCodeSettingKey).
			Take(&row)
		var updatedAt time.Time
		if errors.Is(query.Error, gorm.ErrRecordNotFound) {
			now := time.Now().UTC()
			if err := tx.Exec(`INSERT INTO organization_settings(
				organization_id,setting_key,setting_value,value_type,is_configured,updated_by,updated_at
			) VALUES(?,?,?,?,TRUE,?,?)`, organizationID, adminAccessCodeSettingKey, "", "string", actorUserID, now).Error; err != nil {
				return err
			}
			updatedAt = now
		} else if query.Error != nil {
			return query.Error
		} else {
			var err error
			updatedAt, err = parseAdminAccessCodeTime(row.UpdatedAt)
			if err != nil {
				return err
			}
		}

		now := time.Now()
		localNow := now.In(japanLocation)
		localUpdated := updatedAt.In(japanLocation)
		needsRefresh := force || !validAdminAccessCode(row.Value) ||
			localUpdated.Year() != localNow.Year() || localUpdated.YearDay() != localNow.YearDay()
		if needsRefresh {
			code := ""
			for code == "" || code == row.Value {
				generated, err := generateAdminAccessCode()
				if err != nil {
					return err
				}
				code = generated
			}
			row.Value = code
			updatedAt = now.UTC()
			if err := tx.Exec(`UPDATE organization_settings SET
				setting_value=?,value_type='string',is_configured=TRUE,updated_by=?,updated_at=?
				WHERE organization_id=? AND setting_key=?`, row.Value, actorUserID, updatedAt,
				organizationID, adminAccessCodeSettingKey).Error; err != nil {
				return err
			}
		}

		nextLocalMidnight := time.Date(localNow.Year(), localNow.Month(), localNow.Day()+1, 0, 0, 0, 0, japanLocation)
		result = AdminAccessCodeRecord{
			Code: row.Value, UpdatedAt: updatedAt, NextRefreshAt: nextLocalMidnight.UTC(),
		}
		return nil
	})
	return result, err
}

func generateAdminAccessCode() (string, error) {
	// Ambiguous characters (0/O, 1/I) are excluded while keeping the result
	// strictly alphanumeric and easy to relay verbally.
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	buffer := make([]byte, 6)
	random := make([]byte, 6)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	for index := range buffer {
		buffer[index] = alphabet[int(random[index])%len(alphabet)]
	}
	return string(buffer), nil
}

func validAdminAccessCode(value string) bool {
	if len(value) != 6 {
		return false
	}
	for _, character := range value {
		if !((character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9')) {
			return false
		}
	}
	return true
}
