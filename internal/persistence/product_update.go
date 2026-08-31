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
	ProductCode           *string   `json:"productCode"`
	SKU                   *string   `json:"sku"`
	BrandCode             *string   `json:"brandCode"`
	ModelNumber           *string   `json:"modelNumber"`
	ReferenceNumber       *string   `json:"referenceNumber"`
	SerialNumber          *string   `json:"serialNumber"`
	MaterialCode          *string   `json:"materialCode"`
	MovementCode          *string   `json:"movementCode"`
	ConditionCode         *string   `json:"conditionCode"`
	ShapeCode             *string   `json:"shapeCode"`
	MarkingCode           *string   `json:"markingCode"`
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
	InternalComment       *string   `json:"internalComment"`
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
		var effectiveProductCodeDate time.Time
		if input.ProductCode != nil && strings.TrimSpace(*input.ProductCode) != current.ProductCode {
			purchaseDate := string(current.PurchaseDate)
			if input.PurchaseDate != nil {
				purchaseDate = strings.TrimSpace(*input.PurchaseDate)
			}
			parsedDate, parseErr := time.Parse("2006-01-02", purchaseDate)
			if parseErr != nil {
				return ErrPurchaseDateMismatch
			}
			effectiveProductCodeDate = parsedDate
			if current.CostAdjustmentID != "" {
				var adjustment struct {
					AdjustmentType string
					AdjustmentDate time.Time
				}
				if err := tx.Table("cost_adjustments").
					Select("adjustment_type, adjustment_date").
					Where("organization_id=? AND id=?", organizationID, current.CostAdjustmentID).
					Take(&adjustment).Error; err != nil && err != gorm.ErrRecordNotFound {
					return err
				}
				if adjustment.AdjustmentType == "combine" && !adjustment.AdjustmentDate.IsZero() {
					effectiveProductCodeDate = adjustment.AdjustmentDate
				}
			}
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
		setText(input.InternalComment, "internal_comment")
		if input.ProductCode != nil {
			productCode := strings.TrimSpace(*input.ProductCode)
			if productCode == "" {
				return ErrInvalidProductCode
			}
			if productCode == current.ProductCode {
				updates["product_code"] = productCode
			} else {
				if !isProductCodeForDate(productCode, effectiveProductCodeDate) {
					return ErrInvalidProductCode
				}
				var duplicateCount int64
				if err := tx.Table("products").Where(
					"organization_id=? AND id<>? AND UPPER(BTRIM(product_code))=?",
					organizationID, productID, strings.ToUpper(productCode)).Count(&duplicateCount).Error; err != nil {
					return err
				}
				if duplicateCount > 0 {
					return ErrDuplicateProductCode
				}
				if err := reserveProductSequence(tx, organizationID, effectiveProductCodeDate, productCode, time.Now().UTC()); err != nil {
					return err
				}
				updates["product_code"] = productCode
			}
		}

		if input.BrandCode != nil {
			id, name, err := lookupCatalog(tx, "brands", organizationID, *input.BrandCode, false)
			if err != nil {
				return err
			}
			updates["brand_id"], updates["brand"] = nullIfEmpty(id), name
		}
		for _, field := range []struct {
			value  *string
			table  string
			column string
		}{
			{input.MaterialCode, "materials", "material_id"},
			{input.MovementCode, "movements", "movement_id"},
			{input.ConditionCode, "product_conditions", "condition_id"},
			{input.ShapeCode, "product_shapes", "shape_id"},
			{input.MarkingCode, "markings", "marking_id"},
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
			if field.table == "product_shapes" && name != "" {
				updates["product_type"] = name
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
			if value != "JPY" && value != "USD" && value != "HKD" {
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
			if isProductCodeUniqueViolation(err) {
				return ErrDuplicateProductCode
			}
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

// StartProductCostAdjustment moves an available product into the dedicated
// cost-adjustment workflow while recording an immutable inventory event.
func (r *Repository) StartProductCostAdjustment(ctx context.Context, organizationID, productID, actorUserID string) (Product, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current Product
		result := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(
			"organization_id=? AND id=? AND deleted_at IS NULL", organizationID, productID).Take(&current)
		if result.Error != nil {
			return ErrProductUnavailable
		}
		if current.InventoryStatus == "cost_adjustment" {
			return nil
		}
		if current.InventoryStatus != "in_stock" {
			return ErrProductConflict
		}
		now := time.Now().UTC()
		if err := tx.Model(&Product{}).Where("organization_id=? AND id=?", organizationID, productID).
			Updates(map[string]any{"inventory_status": "cost_adjustment", "updated_at": now}).Error; err != nil {
			return err
		}
		eventID, _ := database.NewID("ive")
		return tx.Exec(`INSERT INTO inventory_events(
			id,organization_id,product_id,event_type,from_status,to_status,reason,actor_user_id,created_at
		) VALUES(?,?,?,'cost_adjustment_started',?,'cost_adjustment','崩し作業を開始',?,?)`,
			eventID, organizationID, productID, current.InventoryStatus, actorUserID, now).Error
	})
	if err != nil {
		return Product{}, err
	}
	return r.ProductByID(ctx, organizationID, productID)
}

// StartCombineCostAdjustment locks the selected product and every input part
// into the same cost-adjustment workflow before the operator starts dragging
// parts onto the product. The operation is idempotent for an already-started
// selection, but rejects products or parts in any other business workflow.
func (r *Repository) StartCombineCostAdjustment(ctx context.Context, organizationID, productID string, partIDs []string, actorUserID string) (Product, error) {
	uniquePartIDs := make([]string, 0, len(partIDs))
	seen := map[string]bool{}
	for _, raw := range partIDs {
		id := strings.TrimSpace(raw)
		if id != "" && !seen[id] {
			seen[id] = true
			uniquePartIDs = append(uniquePartIDs, id)
		}
	}
	if len(uniquePartIDs) == 0 || len(uniquePartIDs) > 20 {
		return Product{}, ErrCostAdjustmentState
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current Product
		result := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(
			"organization_id=? AND id=? AND deleted_at IS NULL", organizationID, productID).Take(&current)
		if result.Error != nil {
			return ErrProductUnavailable
		}
		if current.InventoryStatus != "in_stock" && current.InventoryStatus != "cost_adjustment" {
			return ErrProductConflict
		}
		var parts []struct {
			ID     string
			Status string
		}
		query := tx.Raw(`SELECT id,status FROM parts WHERE organization_id=? AND id IN ? FOR UPDATE`, organizationID, uniquePartIDs).Scan(&parts)
		if query.Error != nil {
			return query.Error
		}
		if len(parts) != len(uniquePartIDs) {
			return ErrCostAdjustmentState
		}
		for _, part := range parts {
			if part.Status != "in_stock" && part.Status != "cost_adjustment" {
				return ErrCostAdjustmentState
			}
		}
		now := time.Now().UTC()
		if err := tx.Model(&Product{}).Where("organization_id=? AND id=?", organizationID, productID).
			Updates(map[string]any{"inventory_status": "cost_adjustment", "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Table("parts").Where("organization_id=? AND id IN ?", organizationID, uniquePartIDs).
			Updates(map[string]any{"status": "cost_adjustment", "updated_by": actorUserID, "updated_at": now}).Error; err != nil {
			return err
		}
		eventID, _ := database.NewID("ive")
		return tx.Exec(`INSERT INTO inventory_events(
			id,organization_id,product_id,event_type,from_status,to_status,reason,actor_user_id,created_at
		) VALUES(?,?,?,'cost_adjustment_started',?,'cost_adjustment','結合作業を開始',?,?)`,
			eventID, organizationID, productID, current.InventoryStatus, actorUserID, now).Error
	})
	if err != nil {
		return Product{}, err
	}
	return r.ProductByID(ctx, organizationID, productID)
}
