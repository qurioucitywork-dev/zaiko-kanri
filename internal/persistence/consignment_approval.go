package persistence

import (
	"context"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"gorm.io/gorm"
	"time"
)

// Save the draft and notify administrators in one transaction.
func (r *Repository) RequestConsignmentApproval(ctx context.Context, input ConsignmentCreateInput) (ConsignmentSlipRecord, error) {
	var record ConsignmentSlipRecord
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := &Repository{db: tx, driver: r.driver}
		input.draft = true
		var err error
		record, err = repo.CreateConsignment(ctx, input)
		if err != nil {
			return err
		}
		_, err = repo.CreateApproval(ctx, input.OrganizationID, input.ActorUserID, ApprovalCreateInput{ApprovalType: "委託登録", TargetType: "consignment_slip", TargetID: record.ID, RequestedAction: "consignment.confirm"})
		return err
	})
	return record, err
}

func (r *Repository) ConfirmConsignment(ctx context.Context, org, id, actor string) (ConsignmentSlipRecord, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var status string
		result := tx.Raw("SELECT status FROM consignment_slips WHERE organization_id=? AND id=? FOR UPDATE", org, id).Scan(&status)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrConsignmentNotFound
		}
		if status != "draft" {
			return ErrConsignmentState
		}
		var products []struct{ ID, InventoryStatus string }
		if err := tx.Raw(`SELECT p.id,p.inventory_status FROM consignment_lines l JOIN products p ON p.id=l.product_id
   WHERE l.consignment_slip_id=? AND p.organization_id=? AND p.deleted_at IS NULL ORDER BY p.id FOR UPDATE OF p`, id, org).Scan(&products).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Table("consignment_lines").Where("consignment_slip_id=?", id).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 || int64(len(products)) != count {
			return ErrProductUnavailable
		}
		now := time.Now().UTC()
		for _, p := range products {
			if p.InventoryStatus != "in_stock" {
				return ErrProductUnavailable
			}
			if err := tx.Exec("UPDATE products SET inventory_status='consigned',updated_at=? WHERE organization_id=? AND id=?", now, org, p.ID).Error; err != nil {
				return err
			}
			eid, err := database.NewID("ive")
			if err != nil {
				return err
			}
			if err := tx.Exec(`INSERT INTO inventory_events(id,organization_id,product_id,event_type,from_status,to_status,reason,actor_user_id,created_at)
    VALUES(?,?,?,'consignment_registered','in_stock','consigned','委託登録（管理者承認）',?,?)`, eid, org, p.ID, actor, now).Error; err != nil {
				return err
			}
		}
		return tx.Exec("UPDATE consignment_slips SET status='confirmed',confirmed_at=?,confirmed_by=?,updated_at=? WHERE organization_id=? AND id=?", now, actor, now, org, id).Error
	})
	if err != nil {
		return ConsignmentSlipRecord{}, err
	}
	return r.ConsignmentSlip(ctx, org, id)
}

// Consignment stock changes, approval, and notifications commit together.
func (r *Repository) DecideApproval(ctx context.Context, org, id, actor, decision, note string) (ApprovalRequestRecord, error) {
	record, err := r.ApprovalRequest(ctx, org, id)
	if err != nil {
		return record, err
	}
	if !isRevisableApprovalTarget(record.TargetType) {
		return r.decideApproval(ctx, org, id, actor, decision, note)
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked string
		if err := tx.Raw("SELECT id FROM approval_requests WHERE organization_id=? AND id=? FOR UPDATE", org, id).Scan(&locked).Error; err != nil {
			return err
		}
		repo := &Repository{db: tx, driver: r.driver}
		var e error
		record, e = repo.decideApproval(ctx, org, id, actor, decision, note)
		return e
	})
	return record, err
}
