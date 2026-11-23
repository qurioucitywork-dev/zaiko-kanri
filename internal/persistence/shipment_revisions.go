package persistence

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

var ErrShipmentRevisionInvalid = errors.New("invalid shipment revision")

type ShipmentRevisionRecord struct {
	RevisedAt time.Time `json:"revisedAt"`
	ActorName string    `json:"actorName"`
	Reason    string    `json:"reason"`
}

type ShipmentRevisionLine struct {
	ID             string `json:"id"`
	UnitPriceMinor int64  `json:"unitPriceMinor"`
}
type ShipmentRevisionInput struct {
	OrganizationID    string                 `json:"-"`
	ActorUserID       string                 `json:"-"`
	ShipmentDate      string                 `json:"shipmentDate"`
	BuyerCode         string                 `json:"buyerCode"`
	Notes             string                 `json:"notes"`
	Reason            string                 `json:"reason"`
	ExpectedUpdatedAt time.Time              `json:"expectedUpdatedAt"`
	Lines             []ShipmentRevisionLine `json:"lines"`
}

func (r *Repository) ReviseShipment(ctx context.Context, id string, input ShipmentRevisionInput) (ShipmentSlipRecord, error) {
	date, err := time.Parse("2006-01-02", input.ShipmentDate)
	reason := strings.TrimSpace(input.Reason)
	if err != nil || reason == "" || len([]rune(reason)) > 1000 || len(input.Lines) == 0 || len(input.Lines) > 100 {
		return ShipmentSlipRecord{}, ErrShipmentRevisionInvalid
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var slip struct {
			ID, SlipNumber, Status, BuyerRoleID, DisplayCurrency, Notes, SalesSlipID string
			ShipmentDate, UpdatedAt                                                  time.Time
			FXRateScaled, FXScale                                                    int64
			TaxRateBasisPoints                                                       int
		}
		err := tx.Table("shipment_slips").Clauses(clause.Locking{Strength: "UPDATE"}).Select("*,COALESCE(sales_slip_id,'') AS sales_slip_id").Where("organization_id=? AND id=?", input.OrganizationID, id).Take(&slip).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrShipmentNotFound
		}
		if err != nil {
			return err
		}
		if slip.Status != "draft" && slip.Status != "confirmed" {
			return ErrShipmentState
		}
		if !input.ExpectedUpdatedAt.IsZero() && !slip.UpdatedAt.Equal(input.ExpectedUpdatedAt) {
			return ErrShipmentState
		}
		role, err := lookupBuyerRole(tx, input.OrganizationID, input.BuyerCode)
		if err != nil {
			return err
		}
		// A shipment already attached to a sale cannot be reassigned to another buyer.
		if slip.SalesSlipID != "" && role != slip.BuyerRoleID {
			return ErrShipmentState
		}
		var lines []struct {
			ID, ProductID                            string
			SalePriceUSDMinor, ConvertedSalePriceJPY int64
		}
		if err := tx.Table("shipment_lines").Clauses(clause.Locking{Strength: "UPDATE"}).Where("shipment_slip_id=?", id).Order("line_number").Find(&lines).Error; err != nil {
			return err
		}
		if len(lines) != len(input.Lines) || slip.FXRateScaled <= 0 || slip.FXScale <= 0 {
			return ErrShipmentRevisionInvalid
		}
		changed := date.Format("2006-01-02") != slip.ShipmentDate.Format("2006-01-02") || role != slip.BuyerRoleID || strings.TrimSpace(input.Notes) != strings.TrimSpace(slip.Notes)
		for i, line := range input.Lines {
			if line.ID != lines[i].ID || line.UnitPriceMinor <= 0 {
				return ErrShipmentRevisionInvalid
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
		updates := map[string]any{"shipment_date": date, "buyer_role_id": role, "notes": note, "updated_at": now}
		if role != slip.BuyerRoleID {
			var partner struct{ LegalName, Address, RegionType string }
			if err := tx.Table("business_partners AS bp").Select("bp.legal_name,bp.address,bp.region_type").Joins("JOIN partner_roles pr ON pr.partner_id=bp.id").Where("pr.id=? AND pr.organization_id=?", role, input.OrganizationID).Take(&partner).Error; err != nil {
				return err
			}
			tax := 0
			if partner.RegionType == "domestic" {
				tax = 1000
			}
			updates["tax_rate_basis_points"] = tax
			updates["recipient_name"] = partner.LegalName
			updates["recipient_address"] = partner.Address
		}
		if err := tx.Table("shipment_slips").Where("id=?", id).Updates(updates).Error; err != nil {
			return err
		}
		for i, line := range input.Lines {
			usd := line.UnitPriceMinor
			jpy := convertShipmentUSDToJPYRoundUpToThousand(usd, slip.FXRateScaled, slip.FXScale)
			if slip.DisplayCurrency == "JPY" {
				jpy = line.UnitPriceMinor
				usd = mulDivRound(jpy, slip.FXScale, slip.FXRateScaled)
			}
			if jpy <= 0 {
				return ErrShipmentPrice
			}
			if err := tx.Table("shipment_lines").Where("id=? AND shipment_slip_id=?", line.ID, id).Updates(map[string]any{"sale_price_usd_minor": usd, "converted_sale_price_jpy": jpy}).Error; err != nil {
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
			comment += "出荷伝票 " + slip.SlipNumber + " 修正理由: " + reason
			if err := tx.Table("products").Where("id=?", lines[i].ProductID).Updates(map[string]any{"internal_comment": comment, "updated_at": now}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return ShipmentSlipRecord{}, err
	}
	return r.ShipmentSlip(ctx, input.OrganizationID, id)
}
