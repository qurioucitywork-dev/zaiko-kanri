package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrSaleRevisionInvalid = errors.New("invalid sale revision")

type SaleRevisionLine struct {
	ID             string `json:"id"`
	UnitPriceMinor int64  `json:"unitPriceMinor"`
}

type SaleRevisionInput struct {
	OrganizationID string             `json:"-"`
	ActorUserID    string             `json:"-"`
	SaleDate       string             `json:"saleDate"`
	BuyerCode      string             `json:"buyerCode"`
	Notes          string             `json:"notes"`
	Reason         string             `json:"reason"`
	Lines          []SaleRevisionLine `json:"lines"`
}

// ReviseSale preserves the transaction currency and the sale-time FX snapshots.
// The required reason is appended to the visible slip remarks and every linked
// product's internal comment in the same transaction.
func (r *Repository) ReviseSale(ctx context.Context, saleID string, input SaleRevisionInput) (SaleSlipRecord, error) {
	reason := strings.TrimSpace(input.Reason)
	date, dateErr := time.Parse("2006-01-02", input.SaleDate)
	if dateErr != nil || reason == "" || len([]rune(reason)) > 1000 || len(input.Lines) == 0 || len(input.Lines) > 100 {
		return SaleSlipRecord{}, ErrSaleRevisionInvalid
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var slip struct {
			ID, SlipNumber, DisplayCurrency, TaxMode string
			TaxRateBasisPoints                       int
		}
		result := tx.Table("sales_slips").Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id,slip_number,display_currency,tax_mode,tax_rate_basis_points").
			Where("organization_id=? AND id=? AND status<>'cancelled'", input.OrganizationID, saleID).Take(&slip)
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return ErrSaleNotFound
		}
		if result.Error != nil {
			return result.Error
		}
		buyerRoleID, err := lookupBuyerRole(tx, input.OrganizationID, input.BuyerCode)
		if err != nil {
			return err
		}
		var rows []struct {
			ID, ProductID, SaleCurrency, FXRateSnapshotID string
			FXRateScaled, FXScale                         int64
		}
		if err := tx.Table("sales_lines").Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id,product_id,sale_currency,fx_rate_snapshot_id,fx_rate_scaled,fx_scale").
			Where("sales_slip_id=?", saleID).Order("line_number").Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) != len(input.Lines) {
			return ErrSaleRevisionInvalid
		}
		now := time.Now().UTC()
		noteLine := "修正理由: " + reason
		notes := strings.TrimSpace(input.Notes)
		if notes != "" {
			notes += "\n"
		}
		notes += noteLine
		if err := tx.Table("sales_slips").Where("id=?", saleID).Updates(map[string]any{
			"sale_date": date, "buyer_role_id": buyerRoleID, "notes": notes, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		for index, line := range input.Lines {
			row := rows[index]
			if line.ID == "" || line.ID != row.ID || line.UnitPriceMinor < 0 || row.SaleCurrency != slip.DisplayCurrency || row.FXRateScaled <= 0 || row.FXScale <= 0 {
				return ErrSaleRevisionInvalid
			}
			tax := int64(0)
			if slip.TaxMode == "taxable" {
				tax = mulDivRound(line.UnitPriceMinor, int64(slip.TaxRateBasisPoints), 10000)
			}
			total := line.UnitPriceMinor + tax
			convertedJPY, err := convertCurrency(total, row.SaleCurrency, "JPY", fxSnapshot{
				ID: row.FXRateSnapshotID, RateScaled: row.FXRateScaled, Scale: row.FXScale,
			})
			if err != nil {
				return err
			}
			if err := tx.Table("sales_lines").Where("id=? AND sales_slip_id=?", line.ID, saleID).Updates(map[string]any{
				"unit_price_minor": line.UnitPriceMinor, "subtotal_minor": line.UnitPriceMinor,
				"tax_amount_minor": tax, "total_minor": total, "converted_total_jpy": convertedJPY,
			}).Error; err != nil {
				return err
			}
			var comment string
			if err := tx.Table("products").Select("internal_comment").Where("organization_id=? AND id=? AND deleted_at IS NULL", input.OrganizationID, row.ProductID).Scan(&comment).Error; err != nil {
				return err
			}
			comment = strings.TrimSpace(comment)
			if comment != "" {
				comment += "\n"
			}
			comment += "売上伝票 " + slip.SlipNumber + " " + noteLine
			if err := tx.Table("products").Where("organization_id=? AND id=?", input.OrganizationID, row.ProductID).
				Updates(map[string]any{"internal_comment": comment, "updated_at": now}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return SaleSlipRecord{}, err
	}
	return r.SaleSlip(ctx, input.OrganizationID, saleID)
}
