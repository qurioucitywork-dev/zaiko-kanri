package persistence

import (
	"context"
	"errors"
	"fmt"
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
)

type SingleProductInput struct {
	OrganizationID        string
	ActorUserID           string
	SupplierCode          string
	StaffCode             string
	PurchaseDate          string
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
	var result SingleProductResult
	var createdProductID string
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		supplierRoleID, err := lookupSupplierRole(tx, input.OrganizationID, input.SupplierCode)
		if err != nil {
			return err
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

		if strings.TrimSpace(input.SerialNumber) != "" {
			var duplicates int64
			if err := tx.Table("products").Where(
				"organization_id = ? AND serial_number = ? AND deleted_at IS NULL AND inventory_status <> 'cancelled'",
				input.OrganizationID, strings.TrimSpace(input.SerialNumber)).Count(&duplicates).Error; err != nil {
				return err
			}
			if duplicates > 0 && strings.TrimSpace(input.DuplicateSerialReason) == "" {
				return ErrDuplicateSerialReason
			}
		}

		documentSequence, err := nextDocumentSequence(tx, input.OrganizationID, "purchase", date.Year(), now)
		if err != nil {
			return err
		}
		productSequence, err := nextProductSequence(tx, input.OrganizationID, date, now)
		if err != nil {
			return err
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
		result.PurchaseSlipNumber = fmt.Sprintf("PI-%04d-%04d", date.Year(), documentSequence)
		createdProductID = productID
		productCode := date.Format("20060102") + fmt.Sprintf("%03d", productSequence)

		if err := tx.Exec(`
			INSERT INTO purchase_slips(
				id,organization_id,slip_number,supplier_role_id,purchase_staff_profile_id,purchase_date,status,
				is_simple,notes,confirmed_at,confirmed_by,created_by,created_at,updated_at
			) VALUES(?,?,?,?,?,?,'confirmed',TRUE,?,?,?,?,?,?)`,
			purchaseID, input.OrganizationID, result.PurchaseSlipNumber, supplierRoleID, staffID, date,
			strings.TrimSpace(input.Notes), now, input.ActorUserID, input.ActorUserID, now, now).Error; err != nil {
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
				publication_status,condition_text,accessories,belt_text,dial_text,bracelet_quantity,notes,created_at,updated_at
			) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,'in_stock','private',?,?,?,?,?,?,?,?)`,
			productID, input.OrganizationID, productCode, strings.TrimSpace(input.SKU), brandName, brandID,
			strings.TrimSpace(input.ModelNumber), strings.TrimSpace(input.ReferenceNumber), strings.TrimSpace(input.SerialNumber), productType,
			nullIfEmpty(materialID), nullIfEmpty(movementID), nullIfEmpty(conditionID), nullIfEmpty(shapeID), nullIfEmpty(markingID), supplierRoleID, supplierRoleID, staffID, lineID,
			date, input.CostAmountMinor, input.CostCurrency, input.BaseSalePriceMinor, input.BaseSaleCurrency,
			conditionName, strings.Join(accessoryNames, ", "), strings.TrimSpace(input.BeltText), strings.TrimSpace(input.DialText),
			input.BraceletQuantity, strings.TrimSpace(input.Notes), now, now).Error; err != nil {
			return fmt.Errorf("insert product: %w", err)
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
		return nil
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

func nextDocumentSequence(tx *gorm.DB, organizationID, documentType string, year int, now time.Time) (int, error) {
	var sequence int
	config, ok := map[string]struct {
		table      string
		dateColumn string
	}{
		"purchase":    {"purchase_slips", "purchase_date"},
		"sale":        {"sales_slips", "sale_date"},
		"shipment":    {"shipment_slips", "shipment_date"},
		"consignment": {"consignment_slips", "consignment_date"},
		"return":      {"return_slips", "transaction_date"},
	}[documentType]
	if !ok {
		return 0, fmt.Errorf("unsupported document sequence type %q", documentType)
	}
	query := fmt.Sprintf(`
		INSERT INTO document_sequences(organization_id,document_type,business_year,last_sequence,updated_at)
		SELECT ?,?,?,COALESCE(MAX((regexp_match(slip_number, '([0-9]+)$'))[1]::INTEGER),0)+1,?
		FROM %s
		WHERE organization_id=? AND EXTRACT(YEAR FROM %s)=?
		ON CONFLICT (organization_id,document_type,business_year)
		DO UPDATE SET last_sequence=document_sequences.last_sequence+1,updated_at=EXCLUDED.updated_at
		RETURNING last_sequence`, config.table, config.dateColumn)
	err := tx.Raw(query, organizationID, documentType, year, now, organizationID, year).Scan(&sequence).Error
	return sequence, err
}

func nextProductSequence(tx *gorm.DB, organizationID string, date time.Time, now time.Time) (int, error) {
	var sequence int
	err := tx.Raw(`
		INSERT INTO product_code_sequences(organization_id,business_date,last_sequence,updated_at)
		SELECT ?,?,COALESCE(MAX(RIGHT(product_code,3)::INTEGER),0)+1,?
		FROM products
		WHERE organization_id=? AND purchase_date=? AND product_code ~ '^[0-9]{11}$'
		ON CONFLICT (organization_id,business_date)
		DO UPDATE SET last_sequence=product_code_sequences.last_sequence+1,updated_at=EXCLUDED.updated_at
		RETURNING last_sequence`, organizationID, date, now, organizationID, date).Scan(&sequence).Error
	return sequence, err
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
	if strings.TrimSpace(code) == "" {
		query = query.Where("user_id = ?", actorUserID)
	} else {
		query = query.Where("staff_code = ?", strings.ToUpper(strings.TrimSpace(code)))
	}
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
