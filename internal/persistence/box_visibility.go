package persistence

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

var ErrBoxVisibilityStatus = errors.New("表示に切り替えられる商品は在庫中・出荷済のみです。仕入中・取置中はゲストに表示できません")

func (r *Repository) SetBoxProductVisibility(ctx context.Context, org, boxCode, productCode string, visible bool) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var product struct{ ID, InventoryStatus string }
		if err := tx.Table("products").Clauses(clause.Locking{Strength: "UPDATE"}).Select("id,inventory_status").Where("organization_id=? AND product_code=? AND deleted_at IS NULL", org, strings.TrimSpace(productCode)).Take(&product).Error; err != nil {
			return ErrProductUnavailable
		}
		if visible && product.InventoryStatus != "in_stock" && product.InventoryStatus != "shipped" {
			return ErrBoxVisibilityStatus
		}
		state, reason := "hidden", "manual"
		if visible {
			state, reason = "visible", ""
		}
		result := tx.Exec(`UPDATE publication_box_products x SET visibility_state=?,visibility_reason=?,basis_status=CASE WHEN ? THEN ? ELSE x.basis_status END
   FROM publication_boxes b WHERE x.box_id=b.id AND b.organization_id=? AND b.box_code=? AND x.product_id=?`, state, reason, visible, product.InventoryStatus, org, strings.ToUpper(strings.TrimSpace(boxCode)), product.ID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrBoxNotFound
		}
		if visible {
			return tx.Exec(`UPDATE products p SET publication_status='public',updated_at=? WHERE p.id=? AND EXISTS(SELECT 1 FROM publication_box_products x JOIN publication_boxes b ON b.id=x.box_id AND b.is_active WHERE x.product_id=p.id)`, time.Now().UTC(), product.ID).Error
		}
		return nil
	})
}
