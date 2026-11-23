package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

var ErrSaleBuyerLocked = errors.New("売上伝票の差戻し修正では販売先を変更できません。元の販売先で再申請してください。")

var ErrReturnedApprovalState = errors.New("本人の棄却・差戻し済み・未確定伝票のみ修正できます。最新の通知から開き直してください。")

type ReturnedApprovalDocument struct {
	Approval ApprovalRequestRecord `json:"approval"`
	Document any                   `json:"document"`
}
type ApprovalResubmitInput struct {
	Rebuild                   *ReturnedDraftInput          `json:"rebuild,omitempty"`
	ShipmentProducts          map[string]string            `json:"shipmentProducts,omitempty"`
	PurchaseProducts          map[string]PurchaseLineInput `json:"purchaseProducts,omitempty"`
	ExpectedApprovalUpdatedAt time.Time                    `json:"expectedApprovalUpdatedAt"`
	ExpectedDocumentUpdatedAt time.Time                    `json:"expectedDocumentUpdatedAt"`
	Purchase                  *PurchaseRevisionInput       `json:"purchase,omitempty"`
	Sale                      *SaleRevisionInput           `json:"sale,omitempty"`
	Shipment                  *ShipmentRevisionInput       `json:"shipment,omitempty"`
	Consignment               *ConsignmentRevisionInput    `json:"consignment,omitempty"`
	Return                    *PurchaseReturnRevisionInput `json:"return,omitempty"`
}

func (r *Repository) returnedDocument(ctx context.Context, org, actor, id string) (ReturnedApprovalDocument, time.Time, error) {
	var result ReturnedApprovalDocument
	// Serialize resubmission and reject stale editors before any document changes.
	var locked struct{ ID string }
	if err := r.db.WithContext(ctx).Table("approval_requests").Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Where("organization_id=? AND id=? AND requested_by=? AND status IN ('returned','rejected')", org, id, actor).Take(&locked).Error; err != nil {
		return result, time.Time{}, ErrReturnedApprovalState
	}
	approval, err := r.ApprovalRequest(ctx, org, id)
	if err != nil {
		return result, time.Time{}, err
	}
	table, action := "", ""
	switch approval.TargetType {
	case "purchase_slip":
		table, action = "purchase_slips", "purchase.confirm"
	case "sales_slip":
		table, action = "sales_slips", "sale.confirm"
	case "shipment_slip":
		table, action = "shipment_slips", "shipment.confirm"
	case "consignment_slip":
		table, action = "consignment_slips", "consignment.confirm"
	case "return_slip":
		table, action = "return_slips", "return.confirm"
	default:
		return result, time.Time{}, ErrReturnedApprovalState
	}
	if approval.RequestedAction != action {
		return result, time.Time{}, ErrReturnedApprovalState
	}
	var target struct {
		Status    string
		UpdatedAt time.Time
	}
	if err := r.db.Table(table).Clauses(clause.Locking{Strength: "UPDATE"}).Select("status,updated_at").Where("organization_id=? AND id=?", org, approval.TargetID).Take(&target).Error; err != nil || (target.Status != "draft" && !(approval.Status == "rejected" && target.Status == "cancelled")) {
		return result, time.Time{}, ErrReturnedApprovalState
	}
	var latest string
	if err := r.db.Table("approval_requests").Select("id").Where("organization_id=? AND target_type=? AND target_id=? AND requested_action=?", org, approval.TargetType, approval.TargetID, action).Order("requested_at DESC,id DESC").Limit(1).Scan(&latest).Error; err != nil {
		return result, time.Time{}, err
	}
	if latest != id {
		return result, time.Time{}, ErrReturnedApprovalState
	}
	result.Approval = approval
	switch approval.TargetType {
	case "purchase_slip":
		result.Document, err = r.PurchaseSlip(ctx, org, approval.TargetID)
	case "sales_slip":
		result.Document, err = r.SaleSlip(ctx, org, approval.TargetID)
	case "shipment_slip":
		result.Document, err = r.ShipmentSlip(ctx, org, approval.TargetID)
	case "consignment_slip":
		result.Document, err = r.ConsignmentSlip(ctx, org, approval.TargetID)
	case "return_slip":
		result.Document, err = r.ReturnSlip(ctx, org, approval.TargetID)
	}
	return result, target.UpdatedAt, err
}
func (r *Repository) ReturnedApprovalDocument(ctx context.Context, org, actor, id string) (ReturnedApprovalDocument, error) {
	var result ReturnedApprovalDocument
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := &Repository{db: tx, driver: r.driver}
		var err error
		result, _, err = repo.returnedDocument(ctx, org, actor, id)
		return err
	})
	return result, err
}
func (r *Repository) ResubmitApproval(ctx context.Context, org, actor, id string, input ApprovalResubmitInput) (ApprovalRequestRecord, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := &Repository{db: tx, driver: r.driver}
		before, updatedAt, err := repo.returnedDocument(ctx, org, actor, id)
		if err != nil {
			return err
		}
		if input.ExpectedApprovalUpdatedAt.IsZero() || input.ExpectedDocumentUpdatedAt.IsZero() || !before.Approval.UpdatedAt.Equal(input.ExpectedApprovalUpdatedAt) || !updatedAt.Equal(input.ExpectedDocumentUpdatedAt) {
			return ErrReturnedApprovalState
		}
		if original, ok := before.Document.(SaleSlipRecord); ok {
			buyer := original.BuyerCode
			if input.Rebuild != nil && input.Rebuild.Sale != nil {
				buyer = input.Rebuild.Sale.BuyerCode
			}
			if input.Sale != nil {
				buyer = input.Sale.BuyerCode
			}
			if strings.TrimSpace(buyer) != original.BuyerCode {
				return ErrSaleBuyerLocked
			}
		}
		if input.Rebuild == nil {
			if err := repo.restoreRejectedDraft(ctx, org, actor, before.Approval); err != nil {
				return err
			}
		}
		count := 0
		for _, ok := range []bool{input.Purchase != nil, input.Sale != nil, input.Shipment != nil, input.Return != nil, input.Consignment != nil, input.Rebuild != nil} {
			if ok {
				count++
			}
		}
		if count != 1 || (input.Rebuild != nil && (len(input.PurchaseProducts) > 0 || len(input.ShipmentProducts) > 0)) {
			return ErrApprovalInvalid
		}
		var after any
		reason := ""
		target := before.Approval.TargetID
		if input.Rebuild != nil {
			reason = input.Rebuild.Reason
			after, err = repo.rebuildReturnedDraft(ctx, org, actor, before, *input.Rebuild)
		} else {
			switch before.Approval.TargetType {
			case "purchase_slip":
				if input.Purchase == nil {
					return ErrApprovalInvalid
				}
				v := *input.Purchase
				v.OrganizationID = org
				v.ActorUserID = actor
				reason = v.Reason
				after, err = repo.RevisePurchase(ctx, target, v)
			case "sales_slip":
				if input.Sale == nil {
					return ErrApprovalInvalid
				}
				v := *input.Sale
				v.OrganizationID = org
				v.ActorUserID = actor
				reason = v.Reason
				after, err = repo.ReviseSale(ctx, target, v)
			case "shipment_slip":
				if input.Shipment == nil {
					return ErrApprovalInvalid
				}
				v := *input.Shipment
				v.OrganizationID = org
				v.ActorUserID = actor
				v.ExpectedUpdatedAt = updatedAt
				reason = v.Reason
				after, err = repo.ReviseShipment(ctx, target, v)
			case "consignment_slip":
				if input.Consignment == nil {
					return ErrApprovalInvalid
				}
				v := *input.Consignment
				v.OrganizationID = org
				v.ActorUserID = actor
				v.ExpectedUpdatedAt = updatedAt
				reason = v.Reason
				after, err = repo.ReviseConsignment(ctx, target, v)
			case "return_slip":
				if input.Return == nil {
					return ErrApprovalInvalid
				}
				v := *input.Return
				reason = v.Reason
				date, dateErr := time.Parse("2006-01-02", v.TransactionDate)
				if dateErr != nil || strings.TrimSpace(v.ReturnReason) == "" || len([]rune(v.ReturnReason)) > 2000 || len([]rune(v.Notes)) > 10000 {
					return ErrPurchaseReturnRevisionInvalid
				}
				if strings.TrimSpace(reason) == "" || len([]rune(reason)) > 1000 {
					return ErrApprovalInvalid
				}
				err = tx.Table("return_slips").Where("organization_id=? AND id=? AND status='draft'", org, target).Updates(map[string]any{"transaction_date": date, "reason": strings.TrimSpace(v.ReturnReason), "notes": strings.TrimSpace(v.Notes) + "\n修正理由: " + strings.TrimSpace(reason), "updated_at": time.Now().UTC()}).Error
				if err == nil {
					after, err = repo.ReturnSlip(ctx, org, target)
				}
			}
		}
		// A correction note may explain that the original document was already correct.
		if errors.Is(err, ErrSlipRevisionUnchanged) {
			after = before.Document
			err = nil
		}
		if err == nil && len(input.ShipmentProducts) > 0 {
			if before.Approval.TargetType != "shipment_slip" {
				return ErrApprovalInvalid
			}
			err = repo.replaceReturnedShipmentProducts(ctx, org, actor, target, reason, input.ShipmentProducts)
			if err == nil {
				after, err = repo.ShipmentSlip(ctx, org, target)
			}
		}
		if err == nil && len(input.PurchaseProducts) > 0 {
			if before.Approval.TargetType != "purchase_slip" {
				return ErrApprovalInvalid
			}
			err = repo.reviseReturnedPurchaseProducts(ctx, org, target, input.PurchaseProducts)
			if err == nil {
				after, err = repo.PurchaseSlip(ctx, org, target)
			}
		}
		if err != nil {
			return err
		}
		if strings.TrimSpace(reason) == "" || len([]rune(reason)) > 1000 {
			return ErrApprovalInvalid
		}
		snapshot, err := json.Marshal(after)
		if err != nil {
			return err
		}
		previousApproval := before.Approval
		previousApproval.ResubmissionJSON = ""
		history, err := json.Marshal(map[string]any{"previousApproval": previousApproval, "reason": strings.TrimSpace(reason), "previousDocument": before.Document})
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		if err := tx.Table("approval_requests").Where("organization_id=? AND id=? AND status IN ('returned','rejected')", org, id).Updates(map[string]any{"status": "pending", "requested_at": now, "decided_by": nil, "decided_at": nil, "decision_note": "", "snapshot_json": string(snapshot), "updated_at": now}).Error; err != nil {
			return err
		}
		actionID, err := database.NewID("apa")
		if err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO approval_actions(id,approval_request_id,actor_user_id,action,note,created_at) VALUES(?,?,?,'requested',?,?)`, actionID, id, actor, string(history), now).Error; err != nil {
			return err
		}
		return insertNotificationTx(tx, org, "", database.RoleAdmin, "approval.requested", "差戻し伝票が再申請されました", before.Approval.ApprovalType+"："+strings.TrimSpace(reason), "approval_request", id, now)
	})
	if err != nil {
		return ApprovalRequestRecord{}, err
	}
	return r.ApprovalRequest(ctx, org, id)
}
