package persistence

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"gorm.io/gorm"
)

var (
	ErrSupplierNotFound      = errors.New("supplier role not found")
	ErrStaffNotFound         = errors.New("staff profile not found")
	ErrMasterCodeNotFound    = errors.New("master code not found")
	ErrDuplicateSerialReason = errors.New("duplicate serial reason is required")
	ErrPurchaseDateMismatch  = errors.New("product purchase date must match its purchase slip")
	ErrDuplicateProductCode  = errors.New("duplicate product code")
	ErrInvalidProductCode    = errors.New("invalid product code format")
	ErrDailyProductLimit     = errors.New("daily product code limit reached")
)

type SingleProductInput struct {
	WeightGrams           string
	AccessorySupplements  AccessorySupplementValues
	BoxCode               string
	OrganizationID        string
	ActorUserID           string
	ProductCode           string
	SupplierCode          string
	StaffCode             string
	PurchaseDate          string
	PurchaseTaxMode       string
	TaxCategory           string
	SKU                   string
	BrandCode             string
	ModelNumber           string
	ReferenceNumber       string
	SerialNumber          string
	ProductType           string
	ShapeCode             string
	MarkingCode           string
	MaterialCode          string
	MovementCode          string
	ConditionCode         string
	AccessoryCodes        []string
	BeltText              string
	DialText              string
	BraceletQuantity      *int
	CostAmountMinor       int64
	CostCurrency          string
	BaseSalePriceMinor    int64
	BaseSaleCurrency      string
	Notes                 string
	InternalComment       string
	DuplicateSerialReason string
}

type SingleProductResult struct {
	PurchaseSlipID     string  `json:"purchaseSlipId"`
	PurchaseSlipNumber string  `json:"purchaseSlipNumber"`
	Product            Product `json:"product"`
}

func (r *Repository) CreateSingleProduct(ctx context.Context, input SingleProductInput) (SingleProductResult, error) {
	date, err := time.Parse("2006-01-02", input.PurchaseDate)
	if err != nil {
		return SingleProductResult{}, fmt.Errorf("invalid purchase date: %w", err)
	}
	purchaseTaxMode, _, err := normalizePurchaseTaxMode(input.PurchaseTaxMode)
	if err != nil {
		return SingleProductResult{}, err
	}
	taxCategory, taxRateBasisPoints, err := normalizePurchaseTaxCategory(input.TaxCategory, purchaseTaxMode)
	if err != nil {
		return SingleProductResult{}, err
	}
	var result SingleProductResult
	var createdProductID string
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		var supplierRoleID string
		if strings.TrimSpace(input.SupplierCode) == "" {
			if PurchaseSupplierRequired(purchaseTaxMode) {
				return ErrSupplierNotFound
			}
		} else {
			supplierRoleID, err = lookupSupplierRole(tx, input.OrganizationID, input.SupplierCode)
			if err != nil {
				return err
			}
		}
		staffID, err := lookupStaffProfile(tx, input.OrganizationID, input.ActorUserID, input.StaffCode)
		if err != nil {
			return err
		}
		brandID, brandName, err := lookupCatalog(tx, "brands", input.OrganizationID, input.BrandCode, true)
		if err != nil {
			return err
		}
		materialID, _, err := lookupCatalog(tx, "materials", input.OrganizationID, input.MaterialCode, false)
		if err != nil {
			return err
		}
		movementID, _, err := lookupCatalog(tx, "movements", input.OrganizationID, input.MovementCode, false)
		if err != nil {
			return err
		}
		conditionID, conditionName, err := lookupCatalog(tx, "product_conditions", input.OrganizationID, input.ConditionCode, false)
		if err != nil {
			return err
		}
		shapeID, shapeName, err := lookupCatalog(tx, "product_shapes", input.OrganizationID, input.ShapeCode, false)
		if err != nil {
			return err
		}
		markingID, _, err := lookupCatalog(tx, "markings", input.OrganizationID, input.MarkingCode, false)
		if err != nil {
			return err
		}
		accessoryIDs, accessoryNames, err := lookupAccessories(tx, input.OrganizationID, input.AccessoryCodes)
		if err != nil {
			return err
		}

		slipNumber, err := nextDailyDocumentNumber(tx, input.OrganizationID, "SD", date, now)
		if err != nil {
			return err
		}
		productCode := strings.TrimSpace(input.ProductCode)
		if productCode == "" {
			productSequence, err := nextProductSequence(tx, input.OrganizationID, date, now)
			if err != nil {
				return err
			}
			productCode = formatProductCode(date, productSequence)
		} else {
			if !isProductCodeForDate(productCode, date) {
				return ErrInvalidProductCode
			}
			var duplicates int64
			if err := tx.Table("products").Where(
				"organization_id = ? AND UPPER(BTRIM(product_code)) = ?",
				input.OrganizationID, strings.ToUpper(productCode)).Count(&duplicates).Error; err != nil {
				return err
			}
			if duplicates > 0 {
				return ErrDuplicateProductCode
			}
			if err := reserveProductSequence(tx, input.OrganizationID, date, productCode, now); err != nil {
				return err
			}
		}
		purchaseID, err := database.NewID("pur")
		if err != nil {
			return err
		}
		lineID, err := database.NewID("pul")
		if err != nil {
			return err
		}
		productID, err := database.NewID("prd")
		if err != nil {
			return err
		}
		eventID, err := database.NewID("ive")
		if err != nil {
			return err
		}
		result.PurchaseSlipID = purchaseID
		result.PurchaseSlipNumber = slipNumber
		createdProductID = productID

		if err := tx.Exec(`
			INSERT INTO purchase_slips(
				id,organization_id,slip_number,supplier_role_id,purchase_staff_profile_id,purchase_date,status,
				is_simple,purchase_tax_mode,tax_category,tax_rate_basis_points,notes,confirmed_at,confirmed_by,created_by,created_at,updated_at
			) VALUES(?,?,?,?,?,?,'confirmed',TRUE,?,?,?,?,?,?,?,?,?)`,
			purchaseID, input.OrganizationID, result.PurchaseSlipNumber, nullIfEmpty(supplierRoleID), staffID, date,
			purchaseTaxMode, taxCategory, taxRateBasisPoints, strings.TrimSpace(input.Notes), now, input.ActorUserID, input.ActorUserID, now, now).Error; err != nil {
			return fmt.Errorf("insert purchase slip: %w", err)
		}
		productType := strings.TrimSpace(input.ProductType)
		if productType == "" {
			productType = shapeName
			if productType == "" {
				productType = "腕時計"
			}
		}
		convertedCostJPY, fxRateID, fxRateScaled, fxScale, err := purchaseCostSnapshot(
			tx, input.OrganizationID, input.CostAmountMinor, 1, input.CostCurrency)
		if err != nil {
			return err
		}
		if err := tx.Exec(`
			INSERT INTO purchase_slip_lines(
				id,purchase_slip_id,line_number,quantity,unit_cost_minor,cost_currency,base_sale_price_minor,
				base_sale_currency,brand_id,material_id,movement_id,condition_id,shape_id,marking_id,brand_text,model_number,
				reference_number,serial_number,product_type,sku,generated_product_count,converted_total_jpy,
				fx_rate_snapshot_id,fx_rate_scaled,fx_scale,created_at
			) VALUES(?,?,1,1,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,1,?,?,?,?,?)`,
			lineID, purchaseID, input.CostAmountMinor, input.CostCurrency, input.BaseSalePriceMinor,
			input.BaseSaleCurrency, brandID, nullIfEmpty(materialID), nullIfEmpty(movementID), nullIfEmpty(conditionID), nullIfEmpty(shapeID), nullIfEmpty(markingID),
			brandName, strings.TrimSpace(input.ModelNumber), strings.TrimSpace(input.ReferenceNumber),
			strings.TrimSpace(input.SerialNumber), productType, strings.TrimSpace(input.SKU), convertedCostJPY,
			fxRateID, fxRateScaled, fxScale, now).Error; err != nil {
			return fmt.Errorf("insert purchase slip line: %w", err)
		}
		if err := tx.Exec(`
			INSERT INTO products(
				id,organization_id,product_code,sku,brand,brand_id,model_number,reference_number,serial_number,product_type,
				material_id,movement_id,condition_id,shape_id,marking_id,supplier_id,supplier_role_id,purchase_staff_profile_id,purchase_slip_line_id,
				purchase_date,cost_amount_minor,cost_currency,base_sale_price_minor,base_sale_currency,inventory_status,
				publication_status,condition_text,accessories,belt_text,dial_text,bracelet_quantity,notes,internal_comment,created_at,updated_at
			) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,'in_stock','private',?,?,?,?,?,?,?,?,?)`,
			productID, input.OrganizationID, productCode, strings.TrimSpace(input.SKU), brandName, brandID,
			strings.TrimSpace(input.ModelNumber), strings.TrimSpace(input.ReferenceNumber), strings.TrimSpace(input.SerialNumber), productType,
			nullIfEmpty(materialID), nullIfEmpty(movementID), nullIfEmpty(conditionID), nullIfEmpty(shapeID), nullIfEmpty(markingID), supplierRoleID, nullIfEmpty(supplierRoleID), staffID, lineID,
			date, input.CostAmountMinor, input.CostCurrency, input.BaseSalePriceMinor, input.BaseSaleCurrency,
			conditionName, strings.Join(accessoryNames, ", "), strings.TrimSpace(input.BeltText), strings.TrimSpace(input.DialText),
			input.BraceletQuantity, strings.TrimSpace(input.Notes), strings.TrimSpace(input.InternalComment), now, now).Error; err != nil {
			if isProductCodeUniqueViolation(err) {
				return ErrDuplicateProductCode
			}
			return fmt.Errorf("insert product: %w", err)
		}
		weight, err := normalizeProductWeight(input.WeightGrams)
		if err != nil {
			return err
		}
		if err := tx.Table("products").Where("id=?", productID).UpdateColumn("weight_grams", weight).Error; err != nil {
			return err
		}
		if err := tx.Table("purchase_slip_lines").Where("id=?", lineID).UpdateColumn("weight_grams", weight).Error; err != nil {
			return err
		}
		supplements, err := normalizeAccessorySupplements(tx, input.OrganizationID, input.AccessoryCodes, input.AccessorySupplements)
		if err != nil {
			return err
		}
		if err := saveAccessorySupplementValues(tx, "products", productID, supplements); err != nil {
			return err
		}
		if err := saveAccessorySupplementValues(tx, "purchase_slip_lines", lineID, supplements); err != nil {
			return err
		}
		for _, accessoryID := range accessoryIDs {
			if err := tx.Exec(`INSERT INTO product_accessories(product_id,accessory_id,quantity) VALUES(?,?,1)`, productID, accessoryID).Error; err != nil {
				return fmt.Errorf("insert product accessory: %w", err)
			}
		}
		reason := "単品商品登録"
		if strings.TrimSpace(input.DuplicateSerialReason) != "" {
			reason += ": " + strings.TrimSpace(input.DuplicateSerialReason)
		}
		if err := tx.Exec(`
			INSERT INTO inventory_events(id,organization_id,product_id,event_type,from_status,to_status,reason,actor_user_id,created_at)
			VALUES(?,?,?,'single_product_registered','','in_stock',?,?,?)`,
			eventID, input.OrganizationID, productID, reason, input.ActorUserID, now).Error; err != nil {
			return fmt.Errorf("insert inventory event: %w", err)
		}
		return assignCreatedProductBox(tx, input.OrganizationID, productID, input.BoxCode, input.ActorUserID, now)
	})
	if err != nil {
		return SingleProductResult{}, err
	}
	product, err := r.ProductByID(ctx, input.OrganizationID, createdProductID)
	if err != nil {
		return SingleProductResult{}, err
	}
	result.Product = product
	return result, nil
}

func (r *Repository) ProductByID(ctx context.Context, organizationID, id string) (Product, error) {
	var product Product
	result := r.db.WithContext(ctx).Table("products").Where("organization_id = ? AND id = ? AND deleted_at IS NULL", organizationID, id).Take(&product)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return Product{}, ErrMasterNotFound
	}
	return product, result.Error
}

func nextProductSequence(tx *gorm.DB, organizationID string, date time.Time, now time.Time) (int, error) {
	var sequence int
	result := tx.Raw(`
		INSERT INTO product_code_sequences(organization_id,business_date,last_sequence,updated_at)
		SELECT ?,?,COALESCE(MAX(RIGHT(product_code,4)::INTEGER),0)+1,?
		FROM products
		WHERE organization_id=? AND purchase_date=? AND product_code ~ '^[0-9]{10}$'
		ON CONFLICT (organization_id,business_date)
		DO UPDATE SET last_sequence=product_code_sequences.last_sequence+1,updated_at=EXCLUDED.updated_at
		WHERE product_code_sequences.last_sequence < 9999
		RETURNING last_sequence`, organizationID, date, now, organizationID, date).Scan(&sequence)
	if result.Error != nil {
		return 0, result.Error
	}
	if result.RowsAffected == 0 || sequence < 1 || sequence > 9999 {
		return 0, ErrDailyProductLimit
	}
	return sequence, nil
}

// formatProductCode returns DDMMYY + a four-digit daily sequence.
func formatProductCode(date time.Time, sequence int) string {
	return date.Format("020106") + fmt.Sprintf("%04d", sequence)
}

func isProductCodeForDate(code string, date time.Time) bool {
	if len(code) != 10 || !strings.HasPrefix(code, date.Format("020106")) {
		return false
	}
	for _, char := range code {
		if char < '0' || char > '9' {
			return false
		}
	}
	return code[6:] != "0000"
}

func reserveProductSequence(tx *gorm.DB, organizationID string, date time.Time, code string, now time.Time) error {
	sequence, err := strconv.Atoi(code[6:])
	if err != nil || sequence < 1 || sequence > 9999 {
		return ErrInvalidProductCode
	}
	return tx.Exec(`
		INSERT INTO product_code_sequences(organization_id,business_date,last_sequence,updated_at)
		VALUES(?,?,?,?)
		ON CONFLICT (organization_id,business_date)
		DO UPDATE SET last_sequence=GREATEST(product_code_sequences.last_sequence,EXCLUDED.last_sequence),updated_at=EXCLUDED.updated_at`,
		organizationID, date, sequence, now).Error
}

func lookupSupplierRole(tx *gorm.DB, organizationID, code string) (string, error) {
	var id string
	result := tx.Table("partner_roles").Select("id").
		Where("organization_id = ? AND role_type = 'supplier' AND role_code = ? AND is_active", organizationID, strings.ToUpper(strings.TrimSpace(code))).
		Scan(&id)
	if result.Error != nil || id == "" {
		return "", ErrSupplierNotFound
	}
	return id, nil
}

func lookupStaffProfile(tx *gorm.DB, organizationID, actorUserID, code string) (string, error) {
	query := tx.Table("staff_profiles").Select("id").Where("organization_id = ?", organizationID)
	// A login account never supplies a buyer implicitly. The caller selects one.
	if strings.TrimSpace(code) == "" {
		return "", ErrStaffNotFound
	}
	query = query.Where("staff_code = ?", strings.ToUpper(strings.TrimSpace(code)))
	var id string
	result := query.Scan(&id)
	if result.Error != nil || id == "" {
		return "", ErrStaffNotFound
	}
	return id, nil
}

func lookupCatalog(tx *gorm.DB, table, organizationID, code string, required bool) (string, string, error) {
	if strings.TrimSpace(code) == "" && !required {
		return "", "", nil
	}
	if !isCatalogLookupTable(table) {
		return "", "", ErrUnsupportedMaster
	}
	var row struct{ ID, Name string }
	result := tx.Table(table).Select("id,name").
		Where("organization_id = ? AND code = ? AND is_active", organizationID, strings.ToUpper(strings.TrimSpace(code))).Take(&row)
	if result.Error != nil || row.ID == "" {
		return "", "", ErrMasterCodeNotFound
	}
	return row.ID, row.Name, nil
}

func isCatalogLookupTable(table string) bool {
	return map[string]bool{
		"brands": true, "materials": true, "movements": true,
		"product_conditions": true, "auction_houses": true,
		"product_shapes": true, "markings": true,
		"part_names": true, "belt_materials": true, "dials": true,
	}[table]
}

func lookupAccessories(tx *gorm.DB, organizationID string, codes []string) ([]string, []string, error) {
	ids := make([]string, 0, len(codes))
	names := make([]string, 0, len(codes))
	seen := map[string]bool{}
	for _, raw := range codes {
		code := strings.ToUpper(strings.TrimSpace(raw))
		if code == "" || seen[code] {
			continue
		}
		seen[code] = true
		var row struct{ ID, Name string }
		result := tx.Table("accessories").Select("id,name").
			Where("organization_id = ? AND code = ? AND is_active", organizationID, code).Take(&row)
		// Retired catalog entries can remain on existing products and pending slips.
		// They are returned separately from current master choices by the API.
		if errors.Is(result.Error, gorm.ErrRecordNotFound) && strings.HasPrefix(code, "ACC-9") {
			result = tx.Table("accessories").Select("id,name").
				Where("organization_id = ? AND code = ? AND is_legacy", organizationID, code).Take(&row)
		}
		if result.Error != nil || row.ID == "" {
			return nil, nil, ErrMasterCodeNotFound
		}
		ids = append(ids, row.ID)
		names = append(names, row.Name)
	}
	return ids, names, nil
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func isProductCodeUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "ux_products_organization_product_code_normalized") ||
		strings.Contains(message, "products_org_product_code_key")
}
