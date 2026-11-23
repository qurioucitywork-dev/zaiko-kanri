package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"gorm.io/gorm"
	"strings"
	"time"
)

var ErrDocumentChanged = errors.New("対象伝票が更新されています。最新の内容で修正・再申請してください。")

type DocumentChangeInput struct {
	Kind     string          `json:"kind"`
	TargetID string          `json:"targetId"`
	Payload  json.RawMessage `json:"payload"`
}
type DocumentChangeSnapshot struct {
	UnchangedRevision bool `json:"unchangedRevision,omitempty"`
	DocumentChangeInput
	Before            any       `json:"before"`
	After             any       `json:"after"`
	ExpectedUpdatedAt time.Time `json:"expectedUpdatedAt"`
}

var documentChangeLabels = map[string]string{"purchase.revision": "仕入伝票修正", "sale.revision": "売上伝票修正", "shipment.revision": "出荷伝票修正", "consignment.revision": "委託伝票修正", "return.revision": "返品伝票修正", "sale.adjustments": "売上変更", "purchase.paid": "仕入支払確認", "sale.paid": "売上入金確認", "shipment.issue": "出荷伝票発行", "return.issue": "返品伝票発行", "purchase.issue": "仕入伝票発行", "sale.issue": "売上伝票発行", "consignment.issue": "委託伝票発行", "purchase.delete": "仕入伝票削除", "return.withdraw": "仕入返品取り下げ", "shipment.tracking": "出荷配送情報変更", "return.tracking": "返品配送情報変更", "consignment.return-scan": "委託商品返却"}

func changeTable(kind string) string {
	switch strings.Split(kind, ".")[0] {
	case "purchase":
		return "purchase_slips"
	case "sale":
		return "sales_slips"
	case "shipment":
		return "shipment_slips"
	case "consignment":
		return "consignment_slips"
	case "return":
		return "return_slips"
	}
	return ""
}
func (r *Repository) changeDocument(ctx context.Context, org string, v DocumentChangeInput) (any, time.Time, error) {
	table := changeTable(v.Kind)
	if documentChangeLabels[v.Kind] == "" || table == "" {
		return nil, time.Time{}, ErrApprovalInvalid
	}
	var row struct {
		UpdatedAt time.Time
		Status    string
	}
	err := r.db.WithContext(ctx).Raw("SELECT updated_at,status FROM "+table+" WHERE organization_id=? AND id=? FOR UPDATE", org, v.TargetID).Scan(&row).Error
	if err != nil {
		return nil, time.Time{}, err
	}
	if row.UpdatedAt.IsZero() || row.Status != "confirmed" {
		return nil, time.Time{}, ErrApprovalInvalid
	}
	var doc any
	switch table {
	case "purchase_slips":
		doc, err = r.PurchaseSlip(ctx, org, v.TargetID)
	case "sales_slips":
		doc, err = r.SaleSlip(ctx, org, v.TargetID)
	case "shipment_slips":
		doc, err = r.ShipmentSlip(ctx, org, v.TargetID)
	case "consignment_slips":
		doc, err = r.ConsignmentSlip(ctx, org, v.TargetID)
	case "return_slips":
		doc, err = r.ReturnSlip(ctx, org, v.TargetID)
	}
	return doc, row.UpdatedAt, err
}
func (r *Repository) applyDocumentChange(ctx context.Context, org, actor string, v DocumentChangeInput) (any, error) {
	decode := func(out any) error { return json.Unmarshal(v.Payload, out) }
	switch v.Kind {
	case "purchase.revision":
		var i PurchaseRevisionInput
		if e := decode(&i); e != nil {
			return nil, e
		}
		i.OrganizationID = org
		i.ActorUserID = actor
		return r.RevisePurchase(ctx, v.TargetID, i)
	case "sale.revision":
		var i SaleRevisionInput
		if e := decode(&i); e != nil {
			return nil, e
		}
		i.OrganizationID = org
		i.ActorUserID = actor
		return r.ReviseSale(ctx, v.TargetID, i)
	case "shipment.revision":
		var i ShipmentRevisionInput
		if e := decode(&i); e != nil {
			return nil, e
		}
		i.OrganizationID = org
		i.ActorUserID = actor
		return r.ReviseShipment(ctx, v.TargetID, i)
	case "consignment.revision":
		var i ConsignmentRevisionInput
		if e := decode(&i); e != nil {
			return nil, e
		}
		i.OrganizationID = org
		i.ActorUserID = actor
		return r.ReviseConsignment(ctx, v.TargetID, i)
	case "return.revision":
		var i PurchaseReturnRevisionInput
		if e := decode(&i); e != nil {
			return nil, e
		}
		i.OrganizationID = org
		i.ActorUserID = actor
		return r.RevisePurchaseReturn(ctx, v.TargetID, i)
	case "sale.adjustments":
		var i SalesAdjustmentInput
		if e := decode(&i); e != nil {
			return nil, e
		}
		i.OrganizationID = org
		i.ActorUserID = actor
		return r.CreateSalesAdjustment(ctx, v.TargetID, i)
	case "purchase.paid":
		return r.MarkPurchasePaid(ctx, org, v.TargetID, actor)
	case "sale.paid":
		return r.MarkSalePaid(ctx, org, v.TargetID, actor)
	case "shipment.issue":
		return r.ShipmentSlip(ctx, org, v.TargetID)
	case "return.issue":
		return r.ReturnSlip(ctx, org, v.TargetID)
	case "purchase.issue":
		return r.IssuePurchase(ctx, org, v.TargetID, actor)
	case "sale.issue":
		return r.IssueSale(ctx, org, v.TargetID, actor)
	case "consignment.issue":
		return r.IssueConsignment(ctx, org, v.TargetID, actor)
	case "purchase.delete":
		return r.DeletePurchase(ctx, org, v.TargetID, actor)
	case "return.withdraw":
		return r.WithdrawPurchaseReturn(ctx, org, v.TargetID, actor)
	case "shipment.tracking", "return.tracking":
		var i struct {
			Carrier        string `json:"carrier"`
			TrackingNumber string `json:"trackingNumber"`
			Confirmed      bool   `json:"confirmed"`
		}
		if e := decode(&i); e != nil {
			return nil, e
		}
		if len(i.Carrier) > 100 || len(i.TrackingNumber) > 200 {
			return nil, ErrApprovalInvalid
		}
		if v.Kind == "shipment.tracking" {
			return r.UpdateShipmentTracking(ctx, org, v.TargetID, i.Carrier, i.TrackingNumber)
		}
		return r.UpdateReturnTracking(ctx, org, v.TargetID, actor, i.Carrier, i.TrackingNumber, i.Confirmed)
	case "consignment.return-scan":
		var i struct {
			Code string `json:"code"`
		}
		if e := decode(&i); e != nil {
			return nil, e
		}
		return r.ReturnConsignmentProduct(ctx, org, v.TargetID, i.Code, actor)
	}
	return nil, ErrApprovalInvalid
}
func (r *Repository) RequestDocumentChange(ctx context.Context, org, actor string, v DocumentChangeInput, resubmitID string, expected time.Time) (ApprovalRequestRecord, error) {
	var id string
	var previousSnapshot json.RawMessage
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := &Repository{db: tx, driver: r.driver}
		before, updated, err := repo.changeDocument(ctx, org, v)
		if err != nil {
			return err
		}
		if resubmitID != "" && (v.Kind == "shipment.revision" || v.Kind == "consignment.revision") {
			var fields map[string]any
			if err = json.Unmarshal(v.Payload, &fields); err != nil {
				return err
			}
			fields["expectedUpdatedAt"] = updated
			v.Payload, err = json.Marshal(fields)
			if err != nil {
				return err
			}
		}
		if resubmitID != "" {
			var locked string
			if err = tx.Raw("SELECT id FROM approval_requests WHERE organization_id=? AND id=? FOR UPDATE", org, resubmitID).Scan(&locked).Error; err != nil {
				return err
			}
			a, e := repo.ApprovalRequest(ctx, org, resubmitID)
			if e != nil || a.RequestedBy != actor || a.TargetType != "document_change" || a.TargetID != v.TargetID || a.RequestedAction != v.Kind || !a.UpdatedAt.Equal(expected) || (a.Status != "rejected" && a.Status != "returned") {
				return ErrApprovalInvalid
			}
			previousSnapshot = json.RawMessage(a.SnapshotJSON)
			id = resubmitID
		} else {
			var existing string
			if err = tx.Table("approval_requests").Select("id").Where("organization_id=? AND requested_by=? AND target_type='document_change' AND target_id=? AND requested_action=? AND status='pending'", org, actor, v.TargetID, v.Kind).Scan(&existing).Error; err != nil {
				return err
			}
			if existing != "" {
				return errors.New("同じ操作の承認申請がすでに承認待ちです。承認管理で確認してください。")
			}
			id, err = database.NewID("apr")
			if err != nil {
				return err
			}
		}
		if resubmitID != "" && v.Kind == "sale.revision" {
			var revision SaleRevisionInput
			if err := json.Unmarshal(v.Payload, &revision); err != nil {
				return err
			}
			original, ok := before.(SaleSlipRecord)
			if !ok || strings.TrimSpace(revision.BuyerCode) != original.BuyerCode {
				return ErrSaleBuyerLocked
			}
		}
		// Validate with the real business rules, then roll back the preview completely.
		if err = tx.SavePoint("document_change_preview").Error; err != nil {
			return err
		}
		after, applyErr := repo.applyDocumentChange(ctx, org, actor, v)
		if err = tx.RollbackTo("document_change_preview").Error; err != nil {
			return err
		}
		// A returned proposal can be restored to the current document, or resubmitted
		// with an explanation only. The validated no-op still requires admin approval.
		unchanged := resubmitID != "" && errors.Is(applyErr, ErrSlipRevisionUnchanged)
		if unchanged {
			after, applyErr = before, nil
		}
		if applyErr != nil {
			return applyErr
		}
		snapshot, e := json.Marshal(DocumentChangeSnapshot{DocumentChangeInput: v, Before: before, After: after, ExpectedUpdatedAt: updated, UnchangedRevision: unchanged})
		if e != nil {
			return e
		}
		now := time.Now().UTC()
		if resubmitID == "" {
			err = tx.Exec(`INSERT INTO approval_requests(id,organization_id,approval_type,target_type,target_id,requested_action,status,requested_by,requested_at,snapshot_json,created_at,updated_at) VALUES(?,?,?,'document_change',?,?,'pending',?,?,CAST(? AS JSONB),?,?)`, id, org, documentChangeLabels[v.Kind], v.TargetID, v.Kind, actor, now, string(snapshot), now, now).Error
		} else {
			err = tx.Exec(`UPDATE approval_requests SET status='pending',requested_at=?,snapshot_json=CAST(? AS JSONB),decided_at=NULL,decided_by=NULL,decision_note='',updated_at=? WHERE id=?`, now, string(snapshot), now, id).Error
		}
		if err != nil {
			return err
		}
		aid, e := database.NewID("apa")
		if e != nil {
			return e
		}
		history := snapshot
		if len(previousSnapshot) > 0 {
			history, e = json.Marshal(map[string]any{"previousDocument": previousSnapshot})
			if e != nil {
				return e
			}
		}
		if err = tx.Exec("INSERT INTO approval_actions(id,approval_request_id,actor_user_id,action,note,created_at) VALUES(?,?,?,'requested',?,?)", aid, id, actor, string(history), now).Error; err != nil {
			return err
		}
		return insertNotificationTx(tx, org, "", database.RoleAdmin, "approval.requested", "伝票操作の承認申請が届きました", documentChangeLabels[v.Kind], "approval_request", id, now)
	})
	if err != nil {
		return ApprovalRequestRecord{}, err
	}
	return r.ApprovalRequest(ctx, org, id)
}
func (r *Repository) ApplyApprovedDocumentChange(ctx context.Context, org, actor string, a ApprovalRequestRecord) error {
	var snap DocumentChangeSnapshot
	if err := json.Unmarshal([]byte(a.SnapshotJSON), &snap); err != nil {
		return err
	}
	current, updated, err := r.changeDocument(ctx, org, snap.DocumentChangeInput)
	if err != nil {
		return err
	}
	if !updated.Equal(snap.ExpectedUpdatedAt) {
		return ErrDocumentChanged
	}
	after, err := r.applyDocumentChange(ctx, org, actor, snap.DocumentChangeInput)
	if snap.UnchangedRevision && errors.Is(err, ErrSlipRevisionUnchanged) {
		after, err = current, nil
	}
	if err != nil {
		return err
	}
	beforeJSON, _ := json.Marshal(snap.Before)
	afterJSON, _ := json.Marshal(after)
	id, err := database.NewID("aud")
	if err != nil {
		return err
	}
	targetType := map[string]string{"purchase": "purchase", "sale": "sales_slip", "shipment": "shipment_slip", "consignment": "consignment_slip", "return": "return_slip"}[strings.Split(a.RequestedAction, ".")[0]]
	action := strings.Replace(a.RequestedAction, ".revision", ".revised", 1)
	if action == "return.revised" {
		action = "purchase_return.revised"
	}
	return r.db.Exec(`INSERT INTO audit_logs(id,organization_id,actor_user_id,target_type,target_id,action,before_json,after_json,reason,request_id,result,created_at) VALUES(?,?,?,?,?,?,CAST(? AS JSONB),CAST(? AS JSONB),?,'approval','success',?)`, id, org, actor, targetType, a.TargetID, action, string(beforeJSON), string(afterJSON), "承認申請 "+a.ID, time.Now().UTC()).Error
}
