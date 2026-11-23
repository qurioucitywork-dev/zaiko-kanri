package persistence

import (
	"context"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"strings"
	"time"
)

func isRevisableApprovalTarget(t string) bool {
	return t == "purchase_slip" || t == "sales_slip" || t == "shipment_slip" || t == "consignment_slip" || t == "return_slip" || t == "document_change"
}

// Older rejections cancelled drafts. Restore only an owned, latest rejected draft;
// never overwrite a product that has since moved into another workflow.
func (r *Repository) restoreRejectedDraft(ctx context.Context, org, actor string, a ApprovalRequestRecord) error {
	if a.Status != "rejected" {
		return nil
	}
	tables := map[string]string{"purchase_slip": "purchase_slips", "sales_slip": "sales_slips", "shipment_slip": "shipment_slips", "consignment_slip": "consignment_slips", "return_slip": "return_slips"}
	table := tables[a.TargetType]
	if table == "" {
		return ErrApprovalInvalid
	}
	var status string
	if e := r.db.Table(table).Select("status").Where("organization_id=? AND id=?", org, a.TargetID).Scan(&status).Error; e != nil {
		return e
	}
	if status != "cancelled" {
		return nil
	}
	lineTable, fk, next := "", "", ""
	switch a.TargetType {
	case "sales_slip":
		lineTable, fk, next = "sales_lines", "sales_slip_id", "reserved"
	case "shipment_slip":
		lineTable, fk, next = "shipment_lines", "shipment_slip_id", "reserved"
	case "return_slip":
		var op string
		if e := r.db.Table("return_slips").Select("operation_type").Where("id=?", a.TargetID).Scan(&op).Error; e != nil {
			return e
		}
		if op == "purchase_return" {
			lineTable, fk, next = "return_lines", "return_slip_id", "return_pending"
		}
	}
	if lineTable != "" {
		var ids []string
		if e := r.db.Table(lineTable).Where(fk+"=?", a.TargetID).Pluck("product_id", &ids).Error; e != nil {
			return e
		}
		for _, id := range ids {
			var current string
			if e := r.db.Raw("SELECT inventory_status FROM products WHERE organization_id=? AND id=? FOR UPDATE", org, id).Scan(&current).Error; e != nil {
				return e
			}
			if current != "in_stock" && !(next == "return_pending" && current == "purchasing") {
				return ErrProductUnavailable
			}
			now := time.Now().UTC()
			if e := r.db.Exec("UPDATE products SET inventory_status=?,updated_at=? WHERE id=?", next, now, id).Error; e != nil {
				return e
			}
			eid, e := database.NewID("ive")
			if e != nil {
				return e
			}
			if e = r.db.Exec("INSERT INTO inventory_events(id,organization_id,product_id,event_type,from_status,to_status,reason,actor_user_id,created_at) VALUES(?,?,?,'approval_resubmitted',?,?,'棄却伝票の修正再申請',?,?)", eid, org, id, current, next, actor, now).Error; e != nil {
				return e
			}
		}
	}
	// Keep version unchanged here; the revision updates it and retains optimistic locking.
	return r.db.Exec("UPDATE "+table+" SET status='draft' WHERE organization_id=? AND id=? AND status='cancelled'", org, strings.TrimSpace(a.TargetID)).Error
}
