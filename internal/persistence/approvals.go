package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"gorm.io/gorm"
)

var ErrApprovalInvalid = errors.New("invalid approval request")

type ApprovalCreateInput struct {
	ApprovalType    string `json:"approvalType"`
	TargetType      string `json:"targetType"`
	TargetID        string `json:"targetId"`
	RequestedAction string `json:"requestedAction"`
}

type ApprovalRequestRecord struct {
	ID              string     `json:"id"`
	ApprovalType    string     `json:"approvalType"`
	TargetType      string     `json:"targetType"`
	TargetID        string     `json:"targetId"`
	RequestedAction string     `json:"requestedAction"`
	Status          string     `json:"status"`
	RequestedBy     string     `json:"requestedBy"`
	RequesterName   string     `json:"requesterName"`
	RequestedAt     time.Time  `json:"requestedAt"`
	DecidedBy       string     `json:"decidedBy,omitempty"`
	DeciderName     string     `json:"deciderName,omitempty"`
	DecidedAt       *time.Time `json:"decidedAt,omitempty"`
	DecisionNote    string     `json:"decisionNote"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

func (r *Repository) ApprovalRequests(ctx context.Context, organizationID, userID, role, status string) ([]ApprovalRequestRecord, error) {
	query := r.db.WithContext(ctx).Table("approval_requests AS a").
		Select(`a.id,a.approval_type,a.target_type,a.target_id,a.requested_action,a.status,a.requested_by,
			rq.display_name AS requester_name,a.requested_at,COALESCE(a.decided_by,'') AS decided_by,
			COALESCE(dc.display_name,'') AS decider_name,a.decided_at,a.decision_note,a.updated_at`).
		Joins("JOIN users rq ON rq.id=a.requested_by").Joins("LEFT JOIN users dc ON dc.id=a.decided_by").
		Where("a.organization_id=?", organizationID)
	if role != database.RoleAdmin {
		query = query.Where("a.requested_by=?", userID)
	}
	if strings.TrimSpace(status) != "" {
		query = query.Where("a.status=?", strings.TrimSpace(status))
	}
	var records []ApprovalRequestRecord
	err := query.Order("a.requested_at DESC").Scan(&records).Error
	return records, err
}

func (r *Repository) ApprovalRequest(ctx context.Context, organizationID, approvalID string) (ApprovalRequestRecord, error) {
	var record ApprovalRequestRecord
	result := r.db.WithContext(ctx).Table("approval_requests AS a").
		Select(`a.id,a.approval_type,a.target_type,a.target_id,a.requested_action,a.status,a.requested_by,
			rq.display_name AS requester_name,a.requested_at,COALESCE(a.decided_by,'') AS decided_by,
			COALESCE(dc.display_name,'') AS decider_name,a.decided_at,a.decision_note,a.updated_at`).
		Joins("JOIN users rq ON rq.id=a.requested_by").Joins("LEFT JOIN users dc ON dc.id=a.decided_by").
		Where("a.organization_id=? AND a.id=?", organizationID, approvalID).Take(&record)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return ApprovalRequestRecord{}, ErrMarketImportState
	}
	return record, result.Error
}

func (r *Repository) CreateApproval(ctx context.Context, organizationID, actorUserID string, input ApprovalCreateInput) (ApprovalRequestRecord, error) {
	input.ApprovalType = strings.TrimSpace(input.ApprovalType)
	input.TargetType = strings.TrimSpace(input.TargetType)
	input.TargetID = strings.TrimSpace(input.TargetID)
	input.RequestedAction = strings.TrimSpace(input.RequestedAction)
	allowed := map[string]string{
		"purchase.confirm": "purchase_slip",
		"sale.confirm":     "sales_slip",
		"shipment.confirm": "shipment_slip",
		"return.confirm":   "return_slip",
		"access.approve":   "access_request",
	}
	if input.ApprovalType == "" || input.TargetID == "" || allowed[input.RequestedAction] != input.TargetType {
		return ApprovalRequestRecord{}, ErrApprovalInvalid
	}

	var snapshot any
	var status string
	var err error
	switch input.TargetType {
	case "purchase_slip":
		var record PurchaseSlipRecord
		record, err = r.PurchaseSlip(ctx, organizationID, input.TargetID)
		status, snapshot = record.Status, record
	case "sales_slip":
		var record SaleSlipRecord
		record, err = r.SaleSlip(ctx, organizationID, input.TargetID)
		status, snapshot = record.Status, record
	case "shipment_slip":
		var record ShipmentSlipRecord
		record, err = r.ShipmentSlip(ctx, organizationID, input.TargetID)
		status, snapshot = record.Status, record
	case "return_slip":
		var record ReturnSlipRecord
		record, err = r.ReturnSlip(ctx, organizationID, input.TargetID)
		status, snapshot = record.Status, record
	case "access_request":
		status, snapshot = "draft", map[string]string{"resource": input.TargetID}
	default:
		return ApprovalRequestRecord{}, ErrApprovalInvalid
	}
	if err != nil || status != "draft" {
		return ApprovalRequestRecord{}, ErrApprovalInvalid
	}

	var existingID string
	if err := r.db.WithContext(ctx).Table("approval_requests").Select("id").Where(
		"organization_id=? AND target_type=? AND target_id=? AND requested_action=? AND status='pending'",
		organizationID, input.TargetType, input.TargetID, input.RequestedAction).Scan(&existingID).Error; err != nil {
		return ApprovalRequestRecord{}, err
	}
	if existingID != "" {
		return r.ApprovalRequest(ctx, organizationID, existingID)
	}

	snapshotJSON, _ := json.Marshal(snapshot)
	approvalID, err := database.NewID("apr")
	if err != nil {
		return ApprovalRequestRecord{}, err
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		if err := tx.Exec(`INSERT INTO approval_requests(
			id,organization_id,approval_type,target_type,target_id,requested_action,status,requested_by,
			requested_at,snapshot_json,created_at,updated_at
		) VALUES(?,?,?,?,?,?,'pending',?,?,CAST(? AS JSONB),?,?)`, approvalID, organizationID,
			input.ApprovalType, input.TargetType, input.TargetID, input.RequestedAction, actorUserID,
			now, string(snapshotJSON), now, now).Error; err != nil {
			return err
		}
		actionID, _ := database.NewID("apa")
		if err := tx.Exec(`INSERT INTO approval_actions(id,approval_request_id,actor_user_id,action,note,created_at)
			VALUES(?,?,?,'requested','',?)`, actionID, approvalID, actorUserID, now).Error; err != nil {
			return err
		}
		return insertNotificationTx(tx, organizationID, "", database.RoleAdmin, "approval.requested",
			"承認申請が届きました", input.ApprovalType+"の承認申請が届きました。", "approval_request", approvalID, now)
	})
	if err != nil {
		return ApprovalRequestRecord{}, err
	}
	return r.ApprovalRequest(ctx, organizationID, approvalID)
}

func (r *Repository) DecideApproval(ctx context.Context, organizationID, approvalID, actorUserID, decision, note string) (ApprovalRequestRecord, error) {
	record, err := r.ApprovalRequest(ctx, organizationID, approvalID)
	if err != nil {
		return ApprovalRequestRecord{}, err
	}
	if record.Status != "pending" {
		return ApprovalRequestRecord{}, ErrMarketImportState
	}
	if record.RequestedBy == actorUserID {
		return ApprovalRequestRecord{}, ErrApprovalSelf
	}
	if decision == "approved" {
		switch record.TargetType {
		case "market_import":
			if _, err := r.CommitMarketImport(ctx, organizationID, record.TargetID, actorUserID, false); err != nil {
				return ApprovalRequestRecord{}, err
			}
			return r.ApprovalRequest(ctx, organizationID, approvalID)
		case "purchase_slip":
			if record.RequestedAction != "purchase.confirm" {
				return ApprovalRequestRecord{}, ErrApprovalInvalid
			}
			_, err = r.ConfirmPurchase(ctx, organizationID, record.TargetID, actorUserID)
		case "sales_slip":
			if record.RequestedAction != "sale.confirm" {
				return ApprovalRequestRecord{}, ErrApprovalInvalid
			}
			_, err = r.ConfirmSale(ctx, organizationID, record.TargetID, actorUserID)
		case "shipment_slip":
			if record.RequestedAction != "shipment.confirm" {
				return ApprovalRequestRecord{}, ErrApprovalInvalid
			}
			_, err = r.ConfirmShipment(ctx, organizationID, record.TargetID, actorUserID)
		case "return_slip":
			if record.RequestedAction != "return.confirm" {
				return ApprovalRequestRecord{}, ErrApprovalInvalid
			}
			_, err = r.ConfirmReturn(ctx, organizationID, record.TargetID, actorUserID)
		case "access_request":
			if record.RequestedAction != "access.approve" {
				return ApprovalRequestRecord{}, ErrApprovalInvalid
			}
		default:
			return ApprovalRequestRecord{}, ErrApprovalInvalid
		}
		if err != nil {
			return ApprovalRequestRecord{}, err
		}
	}
	if decision != "approved" && decision != "returned" && decision != "rejected" {
		return ApprovalRequestRecord{}, ErrMarketImportState
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		result := tx.Exec(`UPDATE approval_requests SET status=?,decided_by=?,decided_at=?,decision_note=?,updated_at=?
			WHERE organization_id=? AND id=? AND status='pending'`, decision, actorUserID, now,
			strings.TrimSpace(note), now, organizationID, approvalID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrMarketImportState
		}
		if record.TargetType == "market_import" {
			targetStatus := "rejected"
			if decision == "returned" {
				targetStatus = "previewed"
			}
			if err := tx.Exec(`UPDATE market_import_batches SET status=? WHERE organization_id=? AND id=? AND status='pending_approval'`,
				targetStatus, organizationID, record.TargetID).Error; err != nil {
				return err
			}
		}
		if decision == "rejected" {
			if err := cancelRejectedApprovalTarget(tx, organizationID, record, now); err != nil {
				return err
			}
		}
		actionID, _ := database.NewID("apa")
		if err := tx.Exec(`INSERT INTO approval_actions(id,approval_request_id,actor_user_id,action,note,created_at)
			VALUES(?,?,?,?,?,?)`, actionID, approvalID, actorUserID, decision, strings.TrimSpace(note), now).Error; err != nil {
			return err
		}
		return insertNotificationTx(tx, organizationID, record.RequestedBy, "", "approval."+decision,
			"承認申請が処理されました", strings.TrimSpace(note), "approval_request", approvalID, now)
	})
	if err != nil {
		return ApprovalRequestRecord{}, err
	}
	return r.ApprovalRequest(ctx, organizationID, approvalID)
}

func cancelRejectedApprovalTarget(tx *gorm.DB, organizationID string, record ApprovalRequestRecord, now time.Time) error {
	switch record.TargetType {
	case "purchase_slip":
		return tx.Exec(`UPDATE purchase_slips SET status='cancelled',updated_at=? WHERE organization_id=? AND id=? AND status='draft'`,
			now, organizationID, record.TargetID).Error
	case "sales_slip", "shipment_slip":
		lineTable, slipTable, foreignKey := "sales_lines", "sales_slips", "sales_slip_id"
		if record.TargetType == "shipment_slip" {
			lineTable, slipTable, foreignKey = "shipment_lines", "shipment_slips", "shipment_slip_id"
		}
		var productIDs []string
		if err := tx.Table(lineTable).Where(foreignKey+"=?", record.TargetID).Pluck("product_id", &productIDs).Error; err != nil {
			return err
		}
		for _, productID := range productIDs {
			var approvedRequestCount int64
			if err := tx.Table("purchase_requests").Where("organization_id=? AND product_id=? AND status='approved'", organizationID, productID).
				Count(&approvedRequestCount).Error; err != nil {
				return err
			}
			if approvedRequestCount == 0 {
				if err := tx.Exec(`UPDATE products SET inventory_status='in_stock',updated_at=? WHERE organization_id=? AND id=? AND inventory_status='reserved'`,
					now, organizationID, productID).Error; err != nil {
					return err
				}
			}
		}
		return tx.Exec("UPDATE "+slipTable+" SET status='cancelled',updated_at=? WHERE organization_id=? AND id=? AND status='draft'",
			now, organizationID, record.TargetID).Error
	case "return_slip":
		var operationType string
		if err := tx.Table("return_slips").Select("operation_type").Where(
			"organization_id=? AND id=? AND status='draft'", organizationID, record.TargetID).Scan(&operationType).Error; err != nil {
			return err
		}
		if operationType == "takeout" || operationType == "purchase_return" {
			var lines []struct{ ProductID, FromStatus string }
			if err := tx.Table("return_lines").Select("product_id,from_status").Where("return_slip_id=?", record.TargetID).Scan(&lines).Error; err != nil {
				return err
			}
			for _, line := range lines {
				if err := tx.Exec(`UPDATE products SET inventory_status=?,updated_at=?
					WHERE organization_id=? AND id=? AND inventory_status IN ('reserved','return_pending')`,
					line.FromStatus, now, organizationID, line.ProductID).Error; err != nil {
					return err
				}
			}
		}
		return tx.Exec(`UPDATE return_slips SET status='cancelled',updated_at=? WHERE organization_id=? AND id=? AND status='draft'`,
			now, organizationID, record.TargetID).Error
	}
	return nil
}
