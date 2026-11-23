package persistence

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrPurchaseReturnProductLocked = errors.New("product is being returned to supplier")

func checkPurchaseReturnEditLock(tx *gorm.DB, organizationID, productID, status string) error {
	if status != "return_pending" {
		return nil
	}
	var count int64
	if err := tx.Table("return_lines AS line").Joins("JOIN return_slips slip ON slip.id=line.return_slip_id").
		Where("line.product_id=? AND slip.organization_id=? AND slip.operation_type='purchase_return' AND slip.status<>'cancelled'", productID, organizationID).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrPurchaseReturnProductLocked
	}
	return nil
}

func lockProductForContentEdit(tx *gorm.DB, organizationID, productID string) error {
	var product struct{ InventoryStatus string }
	result := tx.Table("products").Clauses(clause.Locking{Strength: "UPDATE"}).Select("inventory_status").
		Where("organization_id=? AND id=? AND deleted_at IS NULL", organizationID, productID).Take(&product)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return ErrProductUnavailable
	}
	if result.Error != nil {
		return result.Error
	}
	return checkPurchaseReturnEditLock(tx, organizationID, productID, product.InventoryStatus)
}
