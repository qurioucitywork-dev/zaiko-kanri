package persistence

import (
	"context"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ProductUpdateInput struct {
	SKU                   *string   `json:"sku"`
	BrandCode             *string   `json:"brandCode"`
	ModelNumber           *string   `json:"modelNumber"`
	ReferenceNumber       *string   `json:"referenceNumber"`
	SerialNumber          *string   `json:"serialNumber"`
	MaterialCode          *string   `json:"materialCode"`
	MovementCode          *string   `json:"movementCode"`
	ConditionCode         *string   `json:"conditionCode"`
	SupplierCode          *string   `json:"supplierCode"`
	StaffCode             *string   `json:"staffCode"`
	PurchaseDate          *string   `json:"purchaseDate"`
	CostAmountMinor       *int64    `json:"costAmountMinor"`
	CostCurrency          *string   `json:"costCurrency"`
	BaseSalePriceMinor    *int64    `json:"baseSalePriceMinor"`
	BaseSaleCurrency      *string   `json:"baseSaleCurrency"`
	AccessoryCodes        *[]string `json:"accessoryCodes"`
	BeltText              *string   `json:"beltText"`
	DialText              *string   `json:"dialText"`
	BraceletQuantity      *int      `json:"braceletQuantity"`
	Notes                 *string   `json:"notes"`
	InventoryStatus       *string   `json:"inventoryStatus"`
	DuplicateSerialReason string    `json:"duplicateSerialReason"`
	Reason                string    `json:"reason"`
}

func (r *Repository) UpdateProduct(ctx context.Context, organizationID, productID, actorUserID string, input ProductUpdateInput) (Product, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current Product
		result := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(
			"organization_id=? AND id=? AND deleted_at IS NULL", organizationID, productID).Take(&current)
		if result.Error != nil {
			return ErrProductUnavailable
		}
		updates := map[string]any{"updated_at": time.Now().UTC()}
		setText := func(value *string, column string) {
			if value != nil {
				updates[column] = strings.TrimSpace(*value)
			}
		}
		setText(input.SKU, "sku")
		setText(input.ModelNumber, "model_number")
		setText(input.ReferenceNumber, "reference_number")
		setText(input.BeltText, "belt_text")
		setText(input.DialText, "dial_text")
		setText(input.Notes, "notes")

		if input.BrandCode != nil {
			id, name, err := lookupCatalog(tx, "brands", organizationID, *input.BrandCode, true)
			if err != nil {
				return err
			}
			updates["brand_id"], updates["brand"] = id, name
		}
		for _, field := range []struct {
			value  *string
			table  string
			column string
		}{
			{input.MaterialCode, "materials", "material_id"},
			{input.MovementCode, "movements", "movement_id"},
			{input.ConditionCode, "product_conditions", "condition_id"},
		} {
			if field.value == nil {
				continue
			}
			id, name, err := lookupCatalog(tx, field.table, organizationID, *field.value, false)
			if err != nil {
				return err
			}
			updates[field.column] = nullIfEmpty(id)
			if field.table == "product_conditions" {
				updates["condition_text"] = name
			}
		}
		if input.SupplierCode != nil {
			id, err := lookupSupplierRole(tx, organizationID, *input.SupplierCode)
			if err != nil {
				return err
			}
			updates["supplier_id"], updates["supplier_role_id"] = id, id
		}
		if input.StaffCode != nil {
			id, err := lookupStaffProfile(tx, organizationID, actorUserID, *input.StaffCode)
			if err != nil {
				return err
			}
			updates["purchase_staff_profile_id"] = nullIfEmpty(id)
		}
		if input.PurchaseDate != nil {
			date, err := time.Parse("2006-01-02", strings.TrimSpace(*input.PurchaseDate))
			if err != nil {
				return err
			}
			if current.PurchaseSlipLineID != "" {
				var slipDate time.Time
				result := tx.Table("purchase_slip_lines AS l").
					Select("p.purchase_date").
					Joins("JOIN purchase_slips p ON p.id=l.purchase_slip_id").
					Where("l.id=? AND p.organization_id=?", current.PurchaseSlipLineID, organizationID).
					Scan(&slipDate)
				if result.Error != nil || result.RowsAffected == 0 || date.Format("2006-01-02") != slipDate.Format("2006-01-02") {
					return ErrPurchaseDateMismatch
				}
			}
			updates["purchase_date"] = date
		}
		if input.CostAmountMinor != nil {
			if *input.CostAmountMinor < 0 {
				return ErrProductConflict
			}
			updates["cost_amount_minor"] = *input.CostAmountMinor
		}
		if input.BaseSalePriceMinor != nil {
			if *input.BaseSalePriceMinor < 0 {
				return ErrProductConflict
			}
			updates["base_sale_price_minor"] = *input.BaseSalePriceMinor
		}
		for _, currency := range []struct {
			value  *string
			column string
		}{{input.CostCurrency, "cost_currency"}, {input.BaseSaleCurrency, "base_sale_currency"}} {
			if currency.value == nil {
				continue
			}
			value := strings.ToUpper(strings.TrimSpace(*currency.value))
			if value != "JPY" && value != "USD" {
				return ErrProductConflict
			}
			updates[currency.column] = value
		}
		if input.SerialNumber != nil {
			serial := strings.TrimSpace(*input.SerialNumber)
			if !strings.EqualFold(serial, current.SerialNumber) && serial != "" {
				var duplicateCount int64
				if err := tx.Table("products").Where(
					"organization_id=? AND id<>? AND UPPER(serial_number)=? AND deleted_at IS NULL AND inventory_status<>'cancelled'",
					organizationID, productID, strings.ToUpper(serial)).Count(&duplicateCount).Error; err != nil {
					return err
				}
				if duplicateCount > 0 && strings.TrimSpace(input.DuplicateSerialReason) == "" {
					return ErrDuplicateSerialReason
				}
			}
			updates["serial_number"] = serial
		}
		if input.InventoryStatus != nil && strings.TrimSpace(*input.InventoryStatus) != current.InventoryStatus {
			return ErrProductConflict
		}
		if input.BraceletQuantity != nil {
			if *input.BraceletQuantity < 1 {
				updates["bracelet_quantity"] = nil
			} else {
				updates["bracelet_quantity"] = *input.BraceletQuantity
			}
		}
		if input.AccessoryCodes != nil {
			ids, names, err := lookupAccessories(tx, organizationID, *input.AccessoryCodes)
			if err != nil {
				return err
			}
			updates["accessories"] = strings.Join(names, ", ")
			if err := tx.Exec("DELETE FROM product_accessories WHERE product_id=?", productID).Error; err != nil {
				return err
			}
			for _, accessoryID := range ids {
				if err := tx.Exec(`INSERT INTO product_accessories(product_id,accessory_id,quantity) VALUES(?,?,1)`, productID, accessoryID).Error; err != nil {
					return err
				}
			}
		}
		if err := tx.Model(&Product{}).Where("organization_id=? AND id=?", organizationID, productID).Updates(updates).Error; err != nil {
			return err
		}
		eventID, _ := database.NewID("ive")
		return tx.Exec(`INSERT INTO inventory_events(
			id,organization_id,product_id,event_type,from_status,to_status,reason,actor_user_id,created_at
		) VALUES(?,?,?,'product_updated',?,?,?,?,?)`, eventID, organizationID, productID,
			current.InventoryStatus, current.InventoryStatus, strings.TrimSpace(input.Reason), actorUserID, time.Now().UTC()).Error
	})
	if err != nil {
		return Product{}, err
	}
	return r.ProductByID(ctx, organizationID, productID)
}
