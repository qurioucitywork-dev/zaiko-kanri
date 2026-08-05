package persistence

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
)

type ExchangeRateRecord struct {
	ID            string    `json:"id"`
	BaseCurrency  string    `json:"baseCurrency"`
	QuoteCurrency string    `json:"quoteCurrency"`
	RateScaled    int64     `json:"rateScaled"`
	Scale         int64     `json:"scale"`
	Rate          string    `json:"rate"`
	Provider      string    `json:"provider"`
	ObservedAt    time.Time `json:"observedAt"`
	CreatedAt     time.Time `json:"createdAt"`
}

func parseRate(value string) (int64, error) {
	rat, ok := new(big.Rat).SetString(strings.TrimSpace(value))
	if !ok || rat.Sign() <= 0 {
		return 0, ErrExchangeRate
	}
	scaled := new(big.Rat).Mul(rat, big.NewRat(100000000, 1))
	quotient := new(big.Int).Quo(scaled.Num(), scaled.Denom())
	if !quotient.IsInt64() {
		return 0, ErrExchangeRate
	}
	return quotient.Int64(), nil
}

func formatRateValue(rate, scale int64) string {
	if rate <= 0 || scale <= 0 {
		return ""
	}
	return new(big.Rat).SetFrac(big.NewInt(rate), big.NewInt(scale)).FloatString(8)
}

func (r *Repository) ExchangeRates(ctx context.Context, organizationID string, limit int) ([]ExchangeRateRecord, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	var records []ExchangeRateRecord
	err := r.db.WithContext(ctx).Table("exchange_rate_snapshots").
		Select("id,base_currency,quote_currency,rate_scaled,scale,provider,observed_at,created_at").
		Where("organization_id=?", organizationID).Order("observed_at DESC,created_at DESC").Limit(limit).Scan(&records).Error
	for index := range records {
		records[index].Rate = formatRateValue(records[index].RateScaled, records[index].Scale)
	}
	return records, err
}

func (r *Repository) CreateExchangeRate(ctx context.Context, organizationID, actorUserID, rate, provider string, observedAt time.Time) (ExchangeRateRecord, error) {
	scaled, err := parseRate(rate)
	if err != nil {
		return ExchangeRateRecord{}, err
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	id, err := database.NewID("fxr")
	if err != nil {
		return ExchangeRateRecord{}, err
	}
	now := time.Now().UTC()
	provider = strings.TrimSpace(provider)
	if provider == "" {
		provider = "manual"
	}
	if err := r.db.WithContext(ctx).Exec(`INSERT INTO exchange_rate_snapshots(
		id,organization_id,base_currency,quote_currency,rate_scaled,scale,provider,observed_at,created_by,created_at
	) VALUES(?,?,'USD','JPY',?,100000000,?,?,?,?)`, id, organizationID, scaled, provider, observedAt,
		actorUserID, now).Error; err != nil {
		return ExchangeRateRecord{}, err
	}
	records, err := r.ExchangeRates(ctx, organizationID, 1)
	if err != nil || len(records) == 0 {
		if err == nil {
			err = errors.New("exchange rate was not created")
		}
		return ExchangeRateRecord{}, err
	}
	return records[0], nil
}
