package persistence

import (
	"gorm.io/gorm"
	"strings"
)

// Product accessories have stable ID links and a display-name cache. Refresh the
// cache in the same transaction as a master rename, including previously stale names.
func refreshProductAccessoryNames(tx *gorm.DB, org, accessoryID string) error {
	var rows []struct{ ProductID, Name string }
	err := tx.Table("product_accessories AS pa").
		Select("pa.product_id, a.name").
		Joins("JOIN products p ON p.id=pa.product_id").
		Joins("JOIN accessories a ON a.id=pa.accessory_id AND a.organization_id=p.organization_id").
		Where("p.organization_id=? AND EXISTS (SELECT 1 FROM product_accessories changed WHERE changed.product_id=p.id AND changed.accessory_id=?)", org, accessoryID).
		Order("pa.product_id,a.sort_order,a.code").Scan(&rows).Error
	if err != nil {
		return err
	}
	names := map[string][]string{}
	for _, row := range rows {
		names[row.ProductID] = append(names[row.ProductID], row.Name)
	}
	for id, values := range names {
		if err := tx.Table("products").Where("organization_id=? AND id=?", org, id).
			UpdateColumn("accessories", strings.Join(values, ", ")).Error; err != nil {
			return err
		}
	}
	return nil
}
