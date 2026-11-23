package persistence

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Called only inside the owner/state/version-checked resubmission transaction.
// Draft specifications do not change inventory until the administrator approves.
func (r *Repository) reviseReturnedPurchaseProducts(ctx context.Context, org, slipID string, details map[string]PurchaseLineInput) error {
	if len(details) > 100 {
		return ErrPurchaseRevisionInvalid
	}
	var slip struct{ Status string }
	if err := r.db.WithContext(ctx).Table("purchase_slips").Where("organization_id=? AND id=?", org, slipID).Take(&slip).Error; err != nil {
		return err
	}
	if slip.Status != "draft" {
		return ErrReturnedApprovalState
	}
	for id, detail := range details {
		var line struct {
			ID                                        string
			GeneratedProductCount, GeneratedPartCount int
			ProductType                               string
		}
		if err := r.db.Table("purchase_slip_lines").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND purchase_slip_id=?", id, slipID).Take(&line).Error; err != nil {
			return ErrPurchaseRevisionInvalid
		}
		if line.GeneratedProductCount != 0 || line.GeneratedPartCount != 0 {
			return ErrReturnedApprovalState
		}
		var products int64
		if err := r.db.Table("products").Where("organization_id=? AND purchase_slip_line_id=?", org, id).Count(&products).Error; err != nil {
			return err
		}
		if products != 0 {
			return ErrReturnedApprovalState
		}
		updates := map[string]any{}
		for _, spec := range []struct{ table, column, code string }{
			{"brands", "brand_id", detail.BrandCode}, {"materials", "material_id", detail.MaterialCode},
			{"movements", "movement_id", detail.MovementCode}, {"product_conditions", "condition_id", detail.ConditionCode},
			{"product_shapes", "shape_id", detail.ShapeCode}, {"markings", "marking_id", detail.MarkingCode},
		} {
			catalogID, name, err := lookupCatalog(r.db, spec.table, org, spec.code, false)
			if err != nil {
				return err
			}
			updates[spec.column] = nullIfEmpty(catalogID)
			if spec.column == "brand_id" {
				updates["brand_text"] = name
			}
			if spec.column == "shape_id" {
				if name == "" {
					name = line.ProductType
				}
				updates["product_type"] = name
			}
		}
		for key, value := range map[string]string{"model_number": detail.ModelNumber, "reference_number": detail.ReferenceNumber, "serial_number": detail.SerialNumber, "belt_text": detail.BeltText, "dial_text": detail.DialText, "notes": detail.Notes} {
			if len([]rune(value)) > 10000 {
				return ErrPurchaseRevisionInvalid
			}
			updates[key] = strings.TrimSpace(value)
		}
		if detail.BraceletQuantity != nil && *detail.BraceletQuantity < 1 {
			return ErrPurchaseRevisionInvalid
		}
		updates["bracelet_quantity"] = detail.BraceletQuantity
		weight, err := normalizeProductWeight(detail.WeightGrams)
		if err != nil {
			return err
		}
		updates["weight_grams"] = weight
		if err := validateRegistrationBox(detail.BoxCode); err != nil {
			return err
		}
		updates["box_code"] = strings.TrimSpace(detail.BoxCode)
		if _, _, err := lookupAccessories(r.db, org, detail.AccessoryCodes); err != nil {
			return err
		}
		supplements, err := normalizeAccessorySupplements(r.db, org, detail.AccessoryCodes, detail.AccessorySupplements)
		if err != nil {
			return err
		}
		updates["accessory_supplements"] = supplements
		codes, err := json.Marshal(normalizeCodes(detail.AccessoryCodes))
		if err != nil {
			return err
		}
		updates["accessory_codes"] = gorm.Expr("CAST(? AS JSONB)", string(codes))
		if err := r.db.Table("purchase_slip_lines").Where("id=? AND purchase_slip_id=?", id, slipID).Updates(updates).Error; err != nil {
			return err
		}
	}
	if len(details) == 0 {
		return nil
	}
	return r.db.Table("purchase_slips").Where("organization_id=? AND id=?", org, slipID).Update("updated_at", time.Now().UTC()).Error
}
