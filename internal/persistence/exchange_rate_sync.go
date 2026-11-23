package persistence

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/exchangerate"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ExchangeRateSyncState struct {
	OrganizationID      string     `json:"-"`
	PublishedDate       string     `json:"publishedDate"`
	SourceURL           string     `json:"sourceUrl"`
	LastAttemptAt       *time.Time `json:"lastAttemptAt"`
	LastSuccessAt       *time.Time `json:"lastSuccessAt"`
	LastError           string     `json:"lastError"`
	FailureNotifiedDate string     `json:"-"`
}

func (r *Repository) ExchangeRateOrganization(ctx context.Context, code string) (string, error) {
	var id string
	err := r.db.WithContext(ctx).Table("organizations").Select("id").Where("code=?", code).Scan(&id).Error
	if err == nil && id == "" {
		err = errors.New("exchange rate organization not found")
	}
	return id, err
}

func (r *Repository) ExchangeRateSyncStatus(ctx context.Context, org string) (ExchangeRateSyncState, error) {
	var state ExchangeRateSyncState
	err := r.db.WithContext(ctx).Table("exchange_rate_sync_state").Where("organization_id=?", org).Find(&state).Error
	return state, err
}

// A database-backed lease prevents simultaneous polling by multiple app instances.
func (r *Repository) ClaimExchangeRateSync(ctx context.Context, org string, now time.Time) (bool, error) {
	if err := r.db.WithContext(ctx).Exec("INSERT INTO exchange_rate_sync_state(organization_id) VALUES(?) ON CONFLICT(organization_id) DO NOTHING", org).Error; err != nil {
		return false, err
	}
	result := r.db.WithContext(ctx).Exec("UPDATE exchange_rate_sync_state SET last_attempt_at=? WHERE organization_id=? AND (last_attempt_at IS NULL OR last_attempt_at<=?)", now, org, now.Add(-2*time.Minute))
	return result.RowsAffected == 1, result.Error
}

// Inclusive 1% comparison in integer arithmetic, independent of display rounding.
func significantRateChange(previous, current int64) bool {
	if previous <= 0 || current <= 0 {
		return false
	}
	delta := new(big.Int).Sub(big.NewInt(current), big.NewInt(previous))
	delta.Abs(delta)
	delta.Mul(delta, big.NewInt(100))
	return delta.Cmp(big.NewInt(previous)) >= 0
}

func notifyRateAdmins(tx *gorm.DB, org, event, title, body string, now time.Time) error {
	var ids []string
	if err := tx.Table("users").Where("organization_id=? AND role_key='admin' AND is_active AND deleted_at IS NULL", org).Pluck("id", &ids).Error; err != nil {
		return err
	}
	for _, id := range ids {
		if err := insertNotificationTx(tx, org, id, "", event, title, body, "exchange_rate", org, now); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) CompleteExchangeRateSync(ctx context.Context, org string, p exchangerate.Publication, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var state ExchangeRateSyncState
		if err := tx.Table("exchange_rate_sync_state").Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id=?", org).Take(&state).Error; err != nil {
			return err
		}
		if p.Date < state.PublishedDate {
			return errors.New("refusing older SMBC publication")
		}
		if p.Date > state.PublishedDate {
			if len(p.Rates) != len(exchangerate.Currencies) || p.SourceURL == "" || p.SHA256 == "" {
				return errors.New("incomplete SMBC publication")
			}
			for _, currency := range exchangerate.Currencies {
				rate := p.Rates[currency]
				if rate <= 0 {
					return ErrExchangeRate
				}
				var previous struct {
					RateScaled    int64
					PublishedDate string
				}
				if err := tx.Table("exchange_rate_snapshots").Select("rate_scaled,published_date").Where("organization_id=? AND base_currency=? AND provider=?", org, currency, exchangerate.Provider).Order("published_date DESC").Limit(1).Find(&previous).Error; err != nil {
					return err
				}
				id, err := database.NewID("fxr")
				if err != nil {
					return err
				}
				if err = tx.Exec(`INSERT INTO exchange_rate_snapshots(id,organization_id,base_currency,quote_currency,rate_scaled,scale,provider,observed_at,created_by,created_at,published_date,source_url,source_sha256) VALUES(?,?,?,'JPY',?,?,?,?,NULL,?,?,?,?)`, id, org, currency, rate, exchangerate.Scale, exchangerate.Provider, now, now, p.Date, p.SourceURL, p.SHA256).Error; err != nil {
					return err
				}
				if significantRateChange(previous.RateScaled, rate) {
					change := float64(rate-previous.RateScaled) / float64(previous.RateScaled) * 100
					body := fmt.Sprintf("%s/JPY 仲値: %s円 → %s円（%+.2f%%）。前回公表日 %s / 今回公表日 %s。起票済み伝票のレートは変更されません。", currency, formatRateValue(previous.RateScaled, exchangerate.Scale), formatRateValue(rate, exchangerate.Scale), change, previous.PublishedDate, p.Date)
					if err := notifyRateAdmins(tx, org, "exchange_rate.changed", currency+"の仲値が1％以上変動しました", body, now); err != nil {
						return err
					}
				}
			}
		}
		return tx.Table("exchange_rate_sync_state").Where("organization_id=?", org).Updates(map[string]any{"published_date": p.Date, "source_url": p.SourceURL, "last_success_at": now, "last_error": ""}).Error
	})
}

func (r *Repository) FailExchangeRateSync(ctx context.Context, org string, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var state ExchangeRateSyncState
		if err := tx.Table("exchange_rate_sync_state").Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id=?", org).Take(&state).Error; err != nil {
			return err
		}
		day := now.In(exchangerate.JST).Format("2006-01-02")
		message := "SMBCの仲値を取得できませんでした。直近の正常なレートを維持し、次回の自動確認で再試行します。"
		if state.FailureNotifiedDate != day {
			if err := notifyRateAdmins(tx, org, "exchange_rate.failed", "為替レートの自動取得に失敗しました", message, now); err != nil {
				return err
			}
		}
		return tx.Table("exchange_rate_sync_state").Where("organization_id=?", org).Updates(map[string]any{"last_error": message, "failure_notified_date": day}).Error
	})
}
