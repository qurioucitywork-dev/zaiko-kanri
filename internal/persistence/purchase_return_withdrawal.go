package persistence

import (
	"context"
	"errors"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// WithdrawPurchaseReturn restores every returned product to purchasing, including
// completed returns. The cancelled document remains as history and cannot be reused.
func (r *Repository) WithdrawPurchaseReturn(ctx context.Context, organizationID, returnID, actorUserID string) (ReturnSlipRecord, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var slip struct{ Status, OperationType string }
		result := tx.Table("return_slips").Clauses(clause.Locking{Strength: "UPDATE"}).Select("status,operation_type").Where("organization_id=? AND id=?", organizationID, returnID).Take(&slip)
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return ErrReturnNotFound
		}
		if result.Error != nil {
			return result.Error
		}
		if slip.OperationType != "purchase_return" {
			return ErrReturnState
		}
		// A repeated request must not rewind a product that has since been received or sold.
		if slip.Status == "cancelled" {
			return nil
		}
		if slip.Status != "draft" && slip.Status != "confirmed" {
			return ErrReturnState
		}
		var lines []struct{ ProductID string }
		if err := tx.Table("return_lines").Select("product_id").Where("return_slip_id=?", returnID).Order("product_id").Scan(&lines).Error; err != nil {
			return err
		}
		if len(lines) == 0 {
			var adjustmentJSON string
			if err := tx.Table("return_slips").Select("adjustment_details").Where("id=?", returnID).Scan(&adjustmentJSON).Error; err != nil {
				return err
			}
			if adjustmentJSON == "" || adjustmentJSON == "{}" {
				return ErrReturnState
			}
		}
		now := time.Now().UTC()
		for _, line := range lines {
			var product struct{ InventoryStatus string }
			result := tx.Table("products").Clauses(clause.Locking{Strength: "UPDATE"}).Select("inventory_status").Where("organization_id=? AND id=? AND deleted_at IS NULL", organizationID, line.ProductID).Take(&product)
			if errors.Is(result.Error, gorm.ErrRecordNotFound) {
				return ErrProductUnavailable
			}
			if result.Error != nil {
				return result.Error
			}
			if product.InventoryStatus != "return_pending" && product.InventoryStatus != "cancelled" {
				return ErrReturnState
			}
			if err := tx.Table("products").Where("organization_id=? AND id=?", organizationID, line.ProductID).Updates(map[string]any{
				"inventory_status": "purchasing", "cancelled_at": nil, "cancelled_by": nil, "cancel_reason": "", "updated_at": now,
			}).Error; err != nil {
				return err
			}
			eventID, err := database.NewID("ive")
			if err != nil {
				return err
			}
			if err := tx.Table("inventory_events").Create(map[string]any{
				"id": eventID, "organization_id": organizationID, "product_id": line.ProductID, "event_type": "purchase_return_withdrawn",
				"from_status": product.InventoryStatus, "to_status": "purchasing", "reason": "仕入返品伝票の取り下げにより仕入中へ変更", "actor_user_id": actorUserID, "created_at": now,
			}).Error; err != nil {
				return err
			}
		}
		var approvals []struct{ ID string }
		if err := tx.Table("approval_requests").Select("id").Where("organization_id=? AND target_type='return_slip' AND target_id=? AND status='pending'", organizationID, returnID).Scan(&approvals).Error; err != nil {
			return err
		}
		for _, approval := range approvals {
			result := tx.Table("approval_requests").Where("id=? AND organization_id=? AND status='pending'", approval.ID, organizationID).Updates(map[string]any{
				"status": "cancelled", "decided_by": actorUserID, "decided_at": now, "decision_note": "仕入返品伝票の取り下げ", "updated_at": now,
			})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				continue
			}
			actionID, err := database.NewID("apa")
			if err != nil {
				return err
			}
			if err := tx.Table("approval_actions").Create(map[string]any{"id": actionID, "approval_request_id": approval.ID, "actor_user_id": actorUserID, "action": "cancelled", "note": "仕入返品伝票の取り下げ", "created_at": now}).Error; err != nil {
				return err
			}
		}
		return tx.Table("return_slips").Where("organization_id=? AND id=?", organizationID, returnID).Updates(map[string]any{"status": "cancelled", "updated_at": now}).Error
	})
	if err != nil {
		return ReturnSlipRecord{}, err
	}
	return r.ReturnSlip(ctx, organizationID, returnID)
}
