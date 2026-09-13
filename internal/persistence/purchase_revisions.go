package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrPurchaseRevisionInvalid = errors.New("invalid purchase revision")

type PurchaseRevisionLine struct {
	ID                 string `json:"id"`
	SKU                string `json:"sku"`
	UnitCostMinor      int64  `json:"unitCostMinor"`
	BaseSalePriceMinor int64  `json:"baseSalePriceMinor"`
}

type PurchaseRevisionInput struct {
	OrganizationID string                 `json:"-"`
	ActorUserID    string                 `json:"-"`
	PurchaseDate   string                 `json:"purchaseDate"`
	SupplierCode   string                 `json:"supplierCode"`
	StaffCode      string                 `json:"staffCode"`
	Notes          string                 `json:"notes"`
	Reason         string                 `json:"reason"`
	Lines          []PurchaseRevisionLine `json:"lines"`
}

// RevisePurchase keeps the slip and its linked product comments in one transaction.
// A generated product fixes the purchase date; cost-adjusted products retain their
// current inventory valuation even if the purchase document is corrected.
func (r *Repository) RevisePurchase(ctx context.Context, purchaseID string, input PurchaseRevisionInput) (PurchaseSlipRecord, error) {
	reason := strings.TrimSpace(input.Reason)
	if reason == "" || len([]rune(reason)) > 1000 || len(input.Lines) > 100 {
		return PurchaseSlipRecord{}, ErrPurchaseRevisionInvalid
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var slip struct {
			ID           string
			SlipNumber   string
			PurchaseDate time.Time
			Status       string
		}
		if err := tx.Table("purchase_slips").Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id,slip_number,purchase_date,status").Where("organization_id=? AND id=? AND status<>'cancelled'", input.OrganizationID, purchaseID).
			Take(&slip).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrPurchaseNotFound
			}
			return err
		}
		if input.PurchaseDate != slip.PurchaseDate.Format("2006-01-02") {
			return ErrPurchaseDateMismatch
		}
		var lineRows []struct {
			ID                string
			UnitCostMinor     int64
			CostCurrency      string
			ConvertedTotalJPY int64
		}
		if err := tx.Table("purchase_slip_lines").Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id,unit_cost_minor,cost_currency,converted_total_jpy").
			Where("purchase_slip_id=?", purchaseID).Order("line_number").Find(&lineRows).Error; err != nil {
			return err
		}
		if len(lineRows) != len(input.Lines) {
			return ErrPurchaseRevisionInvalid
		}
		for index, line := range input.Lines {
			old := lineRows[index]
			if line.ID != old.ID || line.UnitCostMinor < 0 || line.BaseSalePriceMinor < 0 {
				return ErrPurchaseRevisionInvalid
			}
			if old.CostCurrency != "JPY" && line.UnitCostMinor != old.UnitCostMinor {
				return ErrPurchaseRevisionInvalid
			}
		}
		roleID, err := lookupSupplierRole(tx, input.OrganizationID, strings.TrimSpace(input.SupplierCode))
		if strings.TrimSpace(input.SupplierCode) == "" {
			roleID = ""
			err = nil
		}
		if err != nil {
			return err
		}
		staffID, err := lookupStaffProfile(tx, input.OrganizationID, input.ActorUserID, strings.TrimSpace(input.StaffCode))
		if err != nil {
			return err
		}
		baseNotes := strings.TrimSpace(input.Notes)
		// The reason is part of the visible slip remarks, not merely an audit event.
		noteLine := "修正理由: " + reason
		if baseNotes != "" {
			baseNotes += "\n"
		}
		baseNotes += noteLine
		now := time.Now().UTC()
		if err := tx.Table("purchase_slips").Where("id=?", purchaseID).Updates(map[string]any{
			"supplier_role_id": nullIfEmpty(roleID), "purchase_staff_profile_id": nullIfEmpty(staffID),
			"notes": baseNotes, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		for index, line := range input.Lines {
			old := lineRows[index]
			if err := tx.Table("purchase_slip_lines").Where("id=? AND purchase_slip_id=?", line.ID, purchaseID).
				Updates(map[string]any{"sku": strings.TrimSpace(line.SKU), "unit_cost_minor": line.UnitCostMinor,
					"converted_total_jpy": func() int64 {
						if old.CostCurrency == "JPY" {
							return line.UnitCostMinor
						}
						return old.ConvertedTotalJPY
					}(),
					"base_sale_price_minor": line.BaseSalePriceMinor}).Error; err != nil {
				return err
			}
			var products []struct {
				ID               string
				InternalComment  string
				CostAdjustmentID string
			}
			if err := tx.Table("products").Clauses(clause.Locking{Strength: "UPDATE"}).
				Select("id,internal_comment,cost_adjustment_id").Where("organization_id=? AND purchase_slip_line_id=? AND deleted_at IS NULL", input.OrganizationID, line.ID).
				Find(&products).Error; err != nil {
				return err
			}
			for _, product := range products {
				comment := strings.TrimSpace(product.InternalComment)
				if comment != "" {
					comment += "\n"
				}
				comment += "仕入伝票 " + slip.SlipNumber + " " + noteLine
				updates := map[string]any{"internal_comment": comment, "sku": strings.TrimSpace(line.SKU),
					"base_sale_price_minor": line.BaseSalePriceMinor, "updated_at": now}
				if product.CostAdjustmentID == "" && old.CostCurrency == "JPY" {
					updates["cost_amount_minor"] = line.UnitCostMinor
				}
				if err := tx.Table("products").Where("id=?", product.ID).Updates(updates).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return PurchaseSlipRecord{}, err
	}
	return r.PurchaseSlip(ctx, input.OrganizationID, purchaseID)
}
