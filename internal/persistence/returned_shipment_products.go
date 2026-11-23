package persistence

import (
	"context"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

// Used only by the owner/version-checked returned-approval transaction.
func (r *Repository) replaceReturnedShipmentProducts(ctx context.Context, org, actor, slipID, reason string, replacements map[string]string) error {
	var slip struct{ Status, SalesSlipID string }
	if err := r.db.WithContext(ctx).Table("shipment_slips").Select("status,COALESCE(sales_slip_id,'') AS sales_slip_id").Where("organization_id=? AND id=?", org, slipID).Take(&slip).Error; err != nil {
		return err
	}
	if slip.Status != "draft" || slip.SalesSlipID != "" {
		return ErrShipmentState
	}
	var lines []struct{ ID, ProductID string }
	if err := r.db.Table("shipment_lines").Clauses(clause.Locking{Strength: "UPDATE"}).Where("shipment_slip_id=?", slipID).Order("line_number").Find(&lines).Error; err != nil {
		return err
	}
	if len(replacements) > len(lines) {
		return ErrShipmentRevisionInvalid
	}
	known := map[string]bool{}
	final := map[string]bool{}
	targets := map[string]string{}
	for _, line := range lines {
		known[line.ID] = true
		targetID := line.ProductID
		if code, ok := replacements[line.ID]; ok {
			var product struct{ ID, InventoryStatus string }
			if err := r.db.Table("products").Clauses(clause.Locking{Strength: "UPDATE"}).Select("id,inventory_status").Where("organization_id=? AND product_code=? AND deleted_at IS NULL", org, strings.TrimSpace(code)).Take(&product).Error; err != nil {
				return ErrProductUnavailable
			}
			targetID = product.ID
			if targetID != line.ProductID && product.InventoryStatus != "in_stock" {
				return ErrProductUnavailable
			}
		}
		if final[targetID] {
			return ErrProductConflict
		}
		final[targetID] = true
		targets[line.ID] = targetID
	}
	for id := range replacements {
		if !known[id] {
			return ErrShipmentRevisionInvalid
		}
	}
	now := time.Now().UTC()
	changeStatus := func(id, from, to string) error {
		if from == to {
			return nil
		}
		if err := r.db.Table("products").Where("organization_id=? AND id=?", org, id).Updates(map[string]any{"inventory_status": to, "updated_at": now}).Error; err != nil {
			return err
		}
		eventID, err := database.NewID("ive")
		if err != nil {
			return err
		}
		return r.db.Exec(`INSERT INTO inventory_events(id,organization_id,product_id,event_type,from_status,to_status,reason,actor_user_id,created_at) VALUES(?,?,?,'shipment_product_replaced',?,?,?,?,?)`, eventID, org, id, from, to, "差戻し出荷伝票の商品差替: "+strings.TrimSpace(reason), actor, now).Error
	}
	for _, line := range lines {
		target := targets[line.ID]
		if target == line.ProductID {
			continue
		}
		var oldStatus string
		if err := r.db.Raw("SELECT inventory_status FROM products WHERE organization_id=? AND id=? FOR UPDATE", org, line.ProductID).Scan(&oldStatus).Error; err != nil {
			return err
		}
		if oldStatus != "reserved" && oldStatus != "in_stock" {
			return ErrProductConflict
		}
		if err := r.db.Table("shipment_lines").Where("id=? AND shipment_slip_id=?", line.ID, slipID).Update("product_id", target).Error; err != nil {
			return err
		}
		if err := changeStatus(target, "in_stock", "reserved"); err != nil {
			return err
		}
		// Preserve another document's reservation; historical completed shipments do not hold stock.
		var holds int64
		if err := r.db.Raw(`SELECT COUNT(*) FROM (
   SELECT 1 FROM shipment_lines l JOIN shipment_slips s ON s.id=l.shipment_slip_id WHERE s.organization_id=? AND l.product_id=? AND s.status='draft'
   UNION ALL SELECT 1 FROM sales_lines l JOIN sales_slips s ON s.id=l.sales_slip_id WHERE s.organization_id=? AND l.product_id=? AND s.status IN ('draft','pending_approval')
   UNION ALL SELECT 1 FROM purchase_requests WHERE organization_id=? AND product_id=? AND status IN ('pending','approved')
   UNION ALL SELECT 1 FROM consignment_lines l JOIN consignment_slips s ON s.id=l.consignment_slip_id WHERE s.organization_id=? AND l.product_id=? AND s.status IN ('draft','confirmed')
  ) held`, org, line.ProductID, org, line.ProductID, org, line.ProductID, org, line.ProductID).Scan(&holds).Error; err != nil {
			return err
		}
		if holds == 0 {
			if err := changeStatus(line.ProductID, oldStatus, "in_stock"); err != nil {
				return err
			}
		}
	}
	return r.db.Table("shipment_slips").Where("organization_id=? AND id=?", org, slipID).Update("updated_at", now).Error
}
