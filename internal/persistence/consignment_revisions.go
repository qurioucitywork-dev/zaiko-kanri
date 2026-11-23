package persistence

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

var ErrConsignmentRevisionInvalid = errors.New("invalid consignment revision")

type ConsignmentRevisionRecord struct {
	RevisedAt time.Time `json:"revisedAt"`
	ActorName string    `json:"actorName"`
	Reason    string    `json:"reason"`
}

type ConsignmentRevisionLine struct {
	ID             string `json:"id"`
	UnitPriceMinor int64  `json:"unitPriceMinor"`
}
type ConsignmentRevisionInput struct {
	OrganizationID    string                    `json:"-"`
	ActorUserID       string                    `json:"-"`
	ConsignmentDate   string                    `json:"consignmentDate"`
	ConsigneeCode     string                    `json:"consigneeCode"`
	Notes             string                    `json:"notes"`
	Reason            string                    `json:"reason"`
	ExpectedUpdatedAt time.Time                 `json:"expectedUpdatedAt"`
	Lines             []ConsignmentRevisionLine `json:"lines"`
}

func (r *Repository) ReviseConsignment(ctx context.Context, id string, input ConsignmentRevisionInput) (ConsignmentSlipRecord, error) {
	date, err := time.Parse("2006-01-02", input.ConsignmentDate)
	reason := strings.TrimSpace(input.Reason)
	if err != nil || reason == "" || len([]rune(reason)) > 1000 || len(input.Lines) == 0 || len(input.Lines) > 100 {
		return ConsignmentSlipRecord{}, ErrConsignmentRevisionInvalid
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var slip struct {
			ID, SlipNumber, Status, ConsigneeRoleID, DisplayCurrency, Notes string
			ConsignmentDate, UpdatedAt                                      time.Time
			FXRateScaled, FXScale                                           int64
			TaxRateBasisPoints                                              int
		}
		err := tx.Table("consignment_slips").Clauses(clause.Locking{Strength: "UPDATE"}).Select("*").Where("organization_id=? AND id=?", input.OrganizationID, id).Take(&slip).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrConsignmentNotFound
		}
		if err != nil {
			return err
		}
		if slip.Status != "draft" && slip.Status != "confirmed" {
			return ErrConsignmentState
		}
		if !input.ExpectedUpdatedAt.IsZero() && !slip.UpdatedAt.Equal(input.ExpectedUpdatedAt) {
			return ErrConsignmentState
		}
		role, err := lookupBuyerRole(tx, input.OrganizationID, input.ConsigneeCode)
		if err != nil {
			return err
		}
		var lines []struct {
			ID, ProductID                            string
			SalePriceUSDMinor, ConvertedSalePriceJPY int64
		}
		if err := tx.Table("consignment_lines").Clauses(clause.Locking{Strength: "UPDATE"}).Where("consignment_slip_id=?", id).Order("line_number").Find(&lines).Error; err != nil {
			return err
		}
		if len(lines) != len(input.Lines) || slip.FXRateScaled <= 0 || slip.FXScale <= 0 {
			return ErrConsignmentRevisionInvalid
		}
		changed := date.Format("2006-01-02") != slip.ConsignmentDate.Format("2006-01-02") || role != slip.ConsigneeRoleID || strings.TrimSpace(input.Notes) != strings.TrimSpace(slip.Notes)
		for i, line := range input.Lines {
			if line.ID != lines[i].ID || line.UnitPriceMinor < 0 {
				return ErrConsignmentRevisionInvalid
			}
			old := lines[i].SalePriceUSDMinor
			if slip.DisplayCurrency == "JPY" {
				old = lines[i].ConvertedSalePriceJPY
			}
			changed = changed || old != line.UnitPriceMinor
		}
		if !changed {
			return ErrSlipRevisionUnchanged
		}
		now := time.Now().UTC()
		note := strings.TrimSpace(input.Notes)
		if note != "" {
			note += "\n"
		}
		note += "修正理由: " + reason
		updates := map[string]any{"consignment_date": date, "consignee_role_id": role, "notes": note, "updated_at": now}
		if err := tx.Table("consignment_slips").Where("id=?", id).Updates(updates).Error; err != nil {
			return err
		}
		for i, line := range input.Lines {
			oldAmount := lines[i].SalePriceUSDMinor
			if slip.DisplayCurrency == "JPY" {
				oldAmount = lines[i].ConvertedSalePriceJPY
			}
			if line.UnitPriceMinor == oldAmount {
				continue
			}
			usd := line.UnitPriceMinor
			jpy := convertShipmentUSDToJPYRoundUpToThousand(usd, slip.FXRateScaled, slip.FXScale)
			if slip.DisplayCurrency == "JPY" {
				jpy = line.UnitPriceMinor
				usd = mulDivRound(jpy, slip.FXScale, slip.FXRateScaled)
			}
			if jpy < 0 {
				return ErrConsignmentRevisionInvalid
			}
			if err := tx.Table("consignment_lines").Where("id=? AND consignment_slip_id=?", line.ID, id).Updates(map[string]any{"sale_price_usd_minor": usd, "converted_sale_price_jpy": jpy}).Error; err != nil {
				return err
			}
			var product struct{ InternalComment string }
			if err := tx.Table("products").Clauses(clause.Locking{Strength: "UPDATE"}).Select("internal_comment").Where("id=? AND organization_id=?", lines[i].ProductID, input.OrganizationID).Take(&product).Error; err != nil {
				return err
			}
			comment := strings.TrimSpace(product.InternalComment)
			if comment != "" {
				comment += "\n"
			}
			comment += "委託伝票 " + slip.SlipNumber + " 修正理由: " + reason
			if err := tx.Table("products").Where("id=?", lines[i].ProductID).Updates(map[string]any{"internal_comment": comment, "updated_at": now}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return ConsignmentSlipRecord{}, err
	}
	return r.ConsignmentSlip(ctx, input.OrganizationID, id)
}
