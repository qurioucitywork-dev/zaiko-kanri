package persistence

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"reflect"

	"gorm.io/gorm"
)

var ErrAuctionFee = errors.New("invalid auction fee")
var ErrAuctionFeeChanged = errors.New("auction fee master changed")

// Amounts are whole JPY. A tier includes From and excludes To (nil means unlimited).
type AuctionFeeTier struct {
	From            int64  `json:"from"`
	To              *int64 `json:"to"`
	RateBasisPoints int64  `json:"rateBasisPoints"`
	Mode            string `json:"mode,omitempty"` // Empty means percentage for existing settings/snapshots.
	FixedAmount     int64  `json:"fixedAmount,omitempty"`
}
type AuctionFeeRule struct {
	Mode            string           `json:"mode"`
	RateBasisPoints int64            `json:"rateBasisPoints,omitempty"`
	FixedAmount     int64            `json:"fixedAmount,omitempty"`
	Tiers           []AuctionFeeTier `json:"tiers,omitempty"`
}
type AuctionFeeSettings struct {
	IsAuction bool           `json:"isAuction"`
	Purchase  AuctionFeeRule `json:"purchase"`
	Sales     AuctionFeeRule `json:"sales"`
}
type AuctionFeeSnapshot struct {
	Rule              *AuctionFeeRule `json:"rule,omitempty"`
	BaseAmountMinor   int64           `json:"baseAmountMinor,omitempty"`
	FeeMinor          int64           `json:"feeMinor,omitempty"`
	ResultAmountMinor int64           `json:"resultAmountMinor,omitempty"`
}

func scanAuctionJSON(source any, target any) error {
	var raw []byte
	switch v := source.(type) {
	case nil:
		raw = []byte(`{}`)
	case string:
		raw = []byte(v)
	case []byte:
		raw = v
	default:
		return ErrAuctionFee
	}
	return json.Unmarshal(raw, target)
}
func (v *AuctionFeeSettings) Scan(source any) error {
	*v = AuctionFeeSettings{}
	return scanAuctionJSON(source, v)
}
func (v AuctionFeeSettings) Value() (driver.Value, error) {
	b, e := json.Marshal(v)
	return string(b), e
}
func (v *AuctionFeeSnapshot) Scan(source any) error {
	*v = AuctionFeeSnapshot{}
	return scanAuctionJSON(source, v)
}
func (v AuctionFeeSnapshot) Value() (driver.Value, error) {
	b, e := json.Marshal(v)
	return string(b), e
}

const maxAuctionAmount int64 = 9007199254740991 // exact integer range shared with the browser

func normalizeAuctionFeeRule(v AuctionFeeRule) (AuctionFeeRule, error) {
	invalid := func() (AuctionFeeRule, error) { return AuctionFeeRule{}, ErrAuctionFee }
	if v.RateBasisPoints < 0 || v.RateBasisPoints > 10000 || v.FixedAmount < 0 || v.FixedAmount > maxAuctionAmount {
		return invalid()
	}
	switch v.Mode {
	case "", "none":
		if v.RateBasisPoints != 0 || v.FixedAmount != 0 || len(v.Tiers) > 0 {
			return invalid()
		}
		return AuctionFeeRule{Mode: "none"}, nil
	case "rate":
		if v.FixedAmount != 0 || len(v.Tiers) > 0 {
			return invalid()
		}
		return AuctionFeeRule{Mode: "rate", RateBasisPoints: v.RateBasisPoints}, nil
	case "fixed":
		if v.RateBasisPoints != 0 || len(v.Tiers) > 0 {
			return invalid()
		}
		return AuctionFeeRule{Mode: "fixed", FixedAmount: v.FixedAmount}, nil
	case "tiers":
		if v.RateBasisPoints != 0 || v.FixedAmount != 0 || len(v.Tiers) == 0 || len(v.Tiers) > 50 {
			return invalid()
		}
		next := int64(0)
		v.Tiers = append([]AuctionFeeTier(nil), v.Tiers...)
		for i, tier := range v.Tiers {
			if tier.From != next || tier.From < 0 || tier.From > maxAuctionAmount || tier.RateBasisPoints < 0 || tier.RateBasisPoints > 10000 {
				return invalid()
			}
			switch tier.Mode {
			case "", "rate":
				if tier.FixedAmount != 0 {
					return invalid()
				}
				v.Tiers[i].Mode = ""
			case "fixed":
				if tier.RateBasisPoints != 0 || tier.FixedAmount < 0 || tier.FixedAmount > maxAuctionAmount {
					return invalid()
				}
			default:
				return invalid()
			}
			if tier.To == nil {
				if i != len(v.Tiers)-1 {
					return invalid()
				}
			} else {
				if *tier.To <= tier.From || *tier.To > maxAuctionAmount || i == len(v.Tiers)-1 {
					return invalid()
				}
				next = *tier.To
			}
		}
		return v, nil
	default:
		return invalid()
	}
}
func normalizeAuctionFees(v AuctionFeeSettings) (AuctionFeeSettings, error) {
	var err error
	if v.Purchase, err = normalizeAuctionFeeRule(v.Purchase); err != nil {
		return v, err
	}
	if v.Sales, err = normalizeAuctionFeeRule(v.Sales); err != nil {
		return v, err
	}
	if !v.IsAuction && (v.Purchase.Mode != "none" || v.Sales.Mode != "none") {
		return v, ErrAuctionFee
	}
	return v, nil
}

// The requested rule is the previewed master value, not a client-supplied fee.
// A shared row lock prevents a master edit between verification and saving.
func resolveAuctionFee(tx *gorm.DB, org, roleID, direction string, requested *AuctionFeeRule) (*AuctionFeeRule, error) {
	if requested == nil {
		return nil, nil
	}
	var saved struct{ AuctionFees AuctionFeeSettings }
	result := tx.Raw(`SELECT bp.auction_fees FROM business_partners bp JOIN partner_roles pr ON pr.partner_id=bp.id AND pr.organization_id=bp.organization_id
 WHERE bp.organization_id=? AND pr.id=? AND bp.status='active' AND pr.is_active FOR SHARE OF bp`, org, roleID).Scan(&saved)
	if result.Error != nil {
		return nil, result.Error
	}
	settings, err := normalizeAuctionFees(saved.AuctionFees)
	if err != nil || result.RowsAffected != 1 || !settings.IsAuction {
		return nil, ErrAuctionFeeChanged
	}
	rule := settings.Purchase
	if direction == "sales" {
		rule = settings.Sales
	}
	expected, err := normalizeAuctionFeeRule(*requested)
	if err != nil {
		return nil, err
	}
	if rule.Mode == "none" || !reflect.DeepEqual(rule, expected) {
		return nil, ErrAuctionFeeChanged
	}
	return &rule, nil
}

func calculateAuctionFee(base int64, currency, direction string, rule *AuctionFeeRule) (int64, AuctionFeeSnapshot, error) {
	if rule == nil {
		return base, AuctionFeeSnapshot{}, nil
	}
	if currency != "JPY" || base < 0 || base > maxAuctionAmount || (direction != "purchase" && direction != "sales") {
		return 0, AuctionFeeSnapshot{}, ErrAuctionFee
	}
	normalized, err := normalizeAuctionFeeRule(*rule)
	if err != nil || normalized.Mode == "none" {
		return 0, AuctionFeeSnapshot{}, ErrAuctionFee
	}
	fee, rate, fixed := normalized.FixedAmount, normalized.RateBasisPoints, normalized.Mode == "fixed"
	if normalized.Mode == "tiers" {
		for _, tier := range normalized.Tiers {
			if base >= tier.From && (tier.To == nil || base < *tier.To) {
				rate, fee, fixed = tier.RateBasisPoints, tier.FixedAmount, tier.Mode == "fixed"
				break
			}
		}
	}
	if !fixed {
		value := new(big.Int).Mul(big.NewInt(base), big.NewInt(rate))
		fee = value.Quo(value, big.NewInt(10000)).Int64()
	}
	result := base + fee
	if direction == "sales" {
		result = base - fee
	}
	if result < 0 || result > maxAuctionAmount {
		return 0, AuctionFeeSnapshot{}, fmt.Errorf("%w: fee exceeds item amount or amount limit", ErrAuctionFee)
	}
	return result, AuctionFeeSnapshot{Rule: &normalized, BaseAmountMinor: base, FeeMinor: fee, ResultAmountMinor: result}, nil
}

func saveAuctionFeeSnapshot(tx *gorm.DB, table, id string, snapshot AuctionFeeSnapshot) error {
	if snapshot.Rule == nil {
		return nil
	}
	return tx.Table(table).Where("id=?", id).UpdateColumn("auction_fee", snapshot).Error
}

// Returned-draft editors and confirmed revisions edit the effective price.
// Retain the original evidence without charging again on a resubmission.
func returnedAuctionFee(identity *returnedDraftIdentity, index int, current AuctionFeeSnapshot) AuctionFeeSnapshot {
	if identity != nil && index < len(identity.lineIDs) && current.Rule == nil {
		return identity.auctionFees[identity.lineIDs[index]]
	}
	return current
}
