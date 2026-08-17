package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"gorm.io/gorm"
)

var (
	ErrPurchaseNotFound       = errors.New("purchase slip not found")
	ErrPurchaseState          = errors.New("purchase slip cannot be changed in its current state")
	ErrDuplicateSerial        = errors.New("serial number already exists")
	ErrQuantitySerialConflict = errors.New("serial number cannot be used when quantity is greater than one")
	ErrPurchaseQuantity       = errors.New("purchase line quantity must be one")
	ErrPurchaseTaxMode        = errors.New("invalid purchase tax mode")
)

const (
	PurchaseTaxModeDomestic = "domestic"
	PurchaseTaxModeOverseas = "overseas"
)

type PurchaseLineInput struct {
	Quantity              int      `json:"quantity"`
	SKU                   string   `json:"sku"`
	BrandCode             string   `json:"brandCode"`
	ModelNumber           string   `json:"modelNumber"`
	ReferenceNumber       string   `json:"referenceNumber"`
	SerialNumber          string   `json:"serialNumber"`
	ProductType           string   `json:"productType"`
	ShapeCode             string   `json:"shapeCode"`
	MarkingCode           string   `json:"markingCode"`
	MaterialCode          string   `json:"materialCode"`
	MovementCode          string   `json:"movementCode"`
	ConditionCode         string   `json:"conditionCode"`
	AccessoryCodes        []string `gorm:"-" json:"accessoryCodes"`
	BeltText              string   `json:"beltText"`
	DialText              string   `json:"dialText"`
	BraceletQuantity      *int     `json:"braceletQuantity,omitempty"`
	UnitCostMinor         int64    `json:"unitCostMinor"`
	CostCurrency          string   `json:"costCurrency"`
	BaseSalePriceMinor    int64    `json:"baseSalePriceMinor"`
	BaseSaleCurrency      string   `json:"baseSaleCurrency"`
	Notes                 string   `json:"notes"`
	DuplicateSerialReason string   `json:"duplicateSerialReason"`
}

type PurchaseCreateInput struct {
	OrganizationID  string
	ActorUserID     string
	SupplierCode    string              `json:"supplierCode"`
	StaffCode       string              `json:"staffCode"`
	PurchaseDate    string              `json:"purchaseDate"`
	PurchaseTaxMode string              `json:"purchaseTaxMode"`
	Notes           string              `json:"notes"`
	Lines           []PurchaseLineInput `json:"lines"`
}

type PurchaseLineRecord struct {
	ID                    string     `json:"id"`
	LineNumber            int        `json:"lineNumber"`
	Quantity              int        `json:"quantity"`
	UnitCostMinor         int64      `json:"unitCostMinor"`
	CostCurrency          string     `json:"costCurrency"`
	BaseSalePriceMinor    int64      `json:"baseSalePriceMinor"`
	BaseSaleCurrency      string     `json:"baseSaleCurrency"`
	BrandCode             string     `json:"brandCode"`
	BrandName             string     `json:"brandName"`
	MaterialCode          string     `json:"materialCode"`
	MovementCode          string     `json:"movementCode"`
	ConditionCode         string     `json:"conditionCode"`
	ModelNumber           string     `json:"modelNumber"`
	ReferenceNumber       string     `json:"referenceNumber"`
	SerialNumber          string     `json:"serialNumber"`
	ProductType           string     `json:"productType"`
	ShapeCode             string     `json:"shapeCode"`
	MarkingCode           string     `json:"markingCode"`
	SKU                   string     `json:"sku"`
	AccessoryCodes        []string   `gorm:"-" json:"accessoryCodes"`
	BeltText              string     `json:"beltText"`
	DialText              string     `json:"dialText"`
	BraceletQuantity      *int       `json:"braceletQuantity,omitempty"`
	Notes                 string     `json:"notes"`
	GeneratedProductCount int        `json:"generatedProductCount"`
	ConvertedTotalJPY     int64      `json:"convertedTotalJpy"`
	FXRateSnapshotID      string     `json:"fxRateSnapshotId"`
	FXRateScaled          int64      `json:"fxRateScaled"`
	FXScale               int64      `json:"fxScale"`
	FXRateObservedAt      *time.Time `json:"fxRateObservedAt,omitempty"`
}

type PurchaseSlipRecord struct {
	ID                    string               `json:"id"`
	SlipNumber            string               `json:"slipNumber"`
	SupplierCode          string               `json:"supplierCode"`
	SupplierName          string               `json:"supplierName"`
	StaffCode             string               `json:"staffCode"`
	PurchaseDate          DateString           `json:"purchaseDate"`
	PurchaseTaxMode       string               `json:"purchaseTaxMode"`
	TaxRateBasisPoints    int                  `json:"taxRateBasisPoints"`
	Status                string               `json:"status"`
	IsSimple              bool                 `json:"isSimple"`
	Notes                 string               `json:"notes"`
	ConfirmedAt           *time.Time           `json:"confirmedAt,omitempty"`
	IssuedAt              *time.Time           `json:"issuedAt,omitempty"`
	IssuedBy              string               `json:"issuedBy,omitempty"`
	IssueFXRateSnapshotID string               `json:"issueFxRateSnapshotId,omitempty"`
	IssueFXRateScaled     int64                `json:"issueFxRateScaled"`
	IssueFXScale          int64                `json:"issueFxScale"`
	CreatedAt             time.Time            `json:"createdAt"`
	UpdatedAt             time.Time            `json:"updatedAt"`
	Lines                 []PurchaseLineRecord `gorm:"-" json:"lines,omitempty"`
	CreatedProducts       []Product            `gorm:"-" json:"createdProducts,omitempty"`
	OfficialPDF           *OfficialDocumentRef `gorm:"-" json:"officialPdf,omitempty"`
}

func normalizePurchaseTaxMode(value string) (string, int, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", PurchaseTaxModeDomestic:
		return PurchaseTaxModeDomestic, 1000, nil
	case PurchaseTaxModeOverseas:
		return PurchaseTaxModeOverseas, 0, nil
	default:
		return "", 0, ErrPurchaseTaxMode
	}
}

func (r *Repository) PurchaseSlips(ctx context.Context, organizationID string, limit int) ([]PurchaseSlipRecord, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	records, _, err := r.PurchaseSlipsPage(ctx, organizationID, 1, limit)
	return records, err
}

// PurchaseSlipsPage returns one deterministic page together with the total
// number of slips.  The UI must walk every page; silently truncating at 500
// slips makes the purchase product count diverge from the inventory count.
func (r *Repository) PurchaseSlipsPage(ctx context.Context, organizationID string, page, pageSize int) ([]PurchaseSlipRecord, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 100
	}
	var total int64
	if err := r.db.WithContext(ctx).Table("purchase_slips").
		Where("organization_id=?", organizationID).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var records []PurchaseSlipRecord
	err := r.db.WithContext(ctx).Table("purchase_slips AS p").
		Select(`p.id,p.slip_number,pr.role_code AS supplier_code,bp.legal_name AS supplier_name,
			COALESCE(sp.staff_code,'') AS staff_code,p.purchase_date,p.purchase_tax_mode,p.tax_rate_basis_points,
			p.status,p.is_simple,p.notes,
			p.confirmed_at,p.issued_at,COALESCE(p.issued_by,'') AS issued_by,
			COALESCE(p.issue_fx_rate_snapshot_id,'') AS issue_fx_rate_snapshot_id,
			COALESCE(p.issue_fx_rate_scaled,0) AS issue_fx_rate_scaled,
			COALESCE(p.issue_fx_scale,0) AS issue_fx_scale,p.created_at,p.updated_at`).
		Joins("JOIN partner_roles pr ON pr.id=p.supplier_role_id AND pr.organization_id=p.organization_id").
		Joins("JOIN business_partners bp ON bp.id=pr.partner_id AND bp.organization_id=p.organization_id").
		Joins("LEFT JOIN staff_profiles sp ON sp.id=p.purchase_staff_profile_id AND sp.organization_id=p.organization_id").
		Where("p.organization_id=?", organizationID).
		Order("p.purchase_date DESC,p.slip_number DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Scan(&records).Error
	return records, total, err
}

func (r *Repository) PurchaseSlip(ctx context.Context, organizationID, purchaseID string) (PurchaseSlipRecord, error) {
	var record PurchaseSlipRecord
	result := r.db.WithContext(ctx).Table("purchase_slips AS p").
		Select(`p.id,p.slip_number,pr.role_code AS supplier_code,bp.legal_name AS supplier_name,
			COALESCE(sp.staff_code,'') AS staff_code,p.purchase_date,p.purchase_tax_mode,p.tax_rate_basis_points,
			p.status,p.is_simple,p.notes,
			p.confirmed_at,p.issued_at,COALESCE(p.issued_by,'') AS issued_by,
			COALESCE(p.issue_fx_rate_snapshot_id,'') AS issue_fx_rate_snapshot_id,
			COALESCE(p.issue_fx_rate_scaled,0) AS issue_fx_rate_scaled,
			COALESCE(p.issue_fx_scale,0) AS issue_fx_scale,p.created_at,p.updated_at`).
		Joins("JOIN partner_roles pr ON pr.id=p.supplier_role_id AND pr.organization_id=p.organization_id").
		Joins("JOIN business_partners bp ON bp.id=pr.partner_id AND bp.organization_id=p.organization_id").
		Joins("LEFT JOIN staff_profiles sp ON sp.id=p.purchase_staff_profile_id AND sp.organization_id=p.organization_id").
		Where("p.organization_id=? AND p.id=?", organizationID, purchaseID).Take(&record)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return PurchaseSlipRecord{}, ErrPurchaseNotFound
	}
	if result.Error != nil {
		return PurchaseSlipRecord{}, result.Error
	}
	type lineRow struct {
		PurchaseLineRecord
		AccessoryCodesJSON string `gorm:"column:accessory_codes_json"`
	}
	var rows []lineRow
	if err := r.db.WithContext(ctx).Table("purchase_slip_lines AS l").
		Select(`l.id,l.line_number,l.quantity,l.unit_cost_minor,l.cost_currency,l.base_sale_price_minor,
			l.base_sale_currency,COALESCE(b.code,'') AS brand_code,l.brand_text AS brand_name,
			COALESCE(m.code,'') AS material_code,COALESCE(mv.code,'') AS movement_code,
			COALESCE(c.code,'') AS condition_code,COALESCE(ps.code,'') AS shape_code,COALESCE(mk.code,'') AS marking_code,l.model_number,l.reference_number,l.serial_number,
			l.product_type,l.sku,l.accessory_codes::TEXT AS accessory_codes_json,l.belt_text,l.dial_text,
			l.bracelet_quantity,l.notes,l.generated_product_count,l.converted_total_jpy,
			COALESCE(l.fx_rate_snapshot_id,'') AS fx_rate_snapshot_id,COALESCE(l.fx_rate_scaled,0) AS fx_rate_scaled,
			COALESCE(l.fx_scale,0) AS fx_scale,fx.observed_at AS fx_rate_observed_at`).
		Joins("LEFT JOIN brands b ON b.id=l.brand_id").
		Joins("LEFT JOIN materials m ON m.id=l.material_id").
		Joins("LEFT JOIN movements mv ON mv.id=l.movement_id").
		Joins("LEFT JOIN product_conditions c ON c.id=l.condition_id").
		Joins("LEFT JOIN product_shapes ps ON ps.id=l.shape_id").
		Joins("LEFT JOIN markings mk ON mk.id=l.marking_id").
		Joins("LEFT JOIN exchange_rate_snapshots fx ON fx.id=l.fx_rate_snapshot_id").
		Where("l.purchase_slip_id=?", purchaseID).Order("l.line_number").Scan(&rows).Error; err != nil {
		return PurchaseSlipRecord{}, err
	}
	record.Lines = make([]PurchaseLineRecord, 0, len(rows))
	for _, row := range rows {
		_ = json.Unmarshal([]byte(row.AccessoryCodesJSON), &row.AccessoryCodes)
		record.Lines = append(record.Lines, row.PurchaseLineRecord)
	}
	return record, nil
}

func (r *Repository) CreatePurchase(ctx context.Context, input PurchaseCreateInput) (PurchaseSlipRecord, error) {
	purchaseTaxMode, taxRateBasisPoints, err := normalizePurchaseTaxMode(input.PurchaseTaxMode)
	if err != nil {
		return PurchaseSlipRecord{}, err
	}
	date, err := time.Parse("2006-01-02", input.PurchaseDate)
	if err != nil {
		return PurchaseSlipRecord{}, fmt.Errorf("invalid purchase date: %w", err)
	}
	if len(input.Lines) == 0 || len(input.Lines) > 100 {
		return PurchaseSlipRecord{}, fmt.Errorf("purchase must contain between 1 and 100 lines")
	}
	var purchaseID string
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
		sequence, err := nextDocumentSequence(tx, input.OrganizationID, "purchase", date.Year(), now)
		if err != nil {
			return err
		}
		purchaseID, err = database.NewID("pur")
		if err != nil {
			return err
		}
		slipNumber := fmt.Sprintf("PI-%04d-%04d", date.Year(), sequence)
		if err := tx.Exec(`INSERT INTO purchase_slips(
			id,organization_id,slip_number,supplier_role_id,purchase_staff_profile_id,purchase_date,status,is_simple,
			purchase_tax_mode,tax_rate_basis_points,notes,created_by,created_at,updated_at
		) VALUES(?,?,?,?,?,?,'draft',FALSE,?,?,?,?,?,?)`, purchaseID, input.OrganizationID, slipNumber,
			supplierRoleID, staffID, date, purchaseTaxMode, taxRateBasisPoints, strings.TrimSpace(input.Notes), input.ActorUserID, now, now).Error; err != nil {
			return fmt.Errorf("insert purchase slip: %w", err)
		}

		seenSerials := map[string]bool{}
		for index, line := range input.Lines {
			if line.Quantity != 1 {
				return ErrPurchaseQuantity
			}
			if line.UnitCostMinor < 0 || line.BaseSalePriceMinor < 0 {
				return fmt.Errorf("invalid purchase line %d", index+1)
			}
			if line.Quantity > 1 && strings.TrimSpace(line.SerialNumber) != "" {
				return ErrQuantitySerialConflict
			}
			serial := strings.TrimSpace(line.SerialNumber)
			if serial != "" {
				key := strings.ToUpper(serial)
				if seenSerials[key] {
					return ErrDuplicateSerial
				}
				seenSerials[key] = true
				var count int64
				if err := tx.Table("products").Where("organization_id=? AND UPPER(serial_number)=? AND deleted_at IS NULL AND inventory_status<>'cancelled'", input.OrganizationID, key).Count(&count).Error; err != nil {
					return err
				}
				if count > 0 && strings.TrimSpace(line.DuplicateSerialReason) == "" {
					return ErrDuplicateSerialReason
				}
			}
			brandID, brandName, err := lookupCatalog(tx, "brands", input.OrganizationID, line.BrandCode, false)
			if err != nil {
				return err
			}
			materialID, _, err := lookupCatalog(tx, "materials", input.OrganizationID, line.MaterialCode, false)
			if err != nil {
				return err
			}
			movementID, _, err := lookupCatalog(tx, "movements", input.OrganizationID, line.MovementCode, false)
			if err != nil {
				return err
			}
			conditionID, _, err := lookupCatalog(tx, "product_conditions", input.OrganizationID, line.ConditionCode, false)
			if err != nil {
				return err
			}
			shapeID, shapeName, err := lookupCatalog(tx, "product_shapes", input.OrganizationID, line.ShapeCode, false)
			if err != nil {
				return err
			}
			markingID, _, err := lookupCatalog(tx, "markings", input.OrganizationID, line.MarkingCode, false)
			if err != nil {
				return err
			}
			_, _, err = lookupAccessories(tx, input.OrganizationID, line.AccessoryCodes)
			if err != nil {
				return err
			}
			accessoryJSON, _ := json.Marshal(normalizeCodes(line.AccessoryCodes))
			lineID, err := database.NewID("pul")
			if err != nil {
				return err
			}
			productType := strings.TrimSpace(line.ProductType)
			if productType == "" {
				productType = shapeName
				if productType == "" {
					productType = "腕時計"
				}
			}
			costCurrency := strings.ToUpper(strings.TrimSpace(line.CostCurrency))
			convertedCostJPY, fxRateID, fxRateScaled, fxScale, err := purchaseCostSnapshot(
				tx, input.OrganizationID, line.UnitCostMinor, line.Quantity, costCurrency)
			if err != nil {
				return err
			}
			if err := tx.Exec(`INSERT INTO purchase_slip_lines(
				id,purchase_slip_id,line_number,quantity,unit_cost_minor,cost_currency,base_sale_price_minor,
				base_sale_currency,brand_id,material_id,movement_id,condition_id,shape_id,marking_id,brand_text,model_number,
				reference_number,serial_number,product_type,sku,accessory_codes,notes,belt_text,dial_text,bracelet_quantity,duplicate_serial_reason,
				generated_product_count,converted_total_jpy,fx_rate_snapshot_id,fx_rate_scaled,fx_scale,created_at
			) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,CAST(? AS JSONB),?,?,?,?,?,0,?,?,?,?,?)`, lineID, purchaseID, index+1,
				line.Quantity, line.UnitCostMinor, costCurrency,
				line.BaseSalePriceMinor, strings.ToUpper(strings.TrimSpace(line.BaseSaleCurrency)), nullIfEmpty(brandID),
				nullIfEmpty(materialID), nullIfEmpty(movementID), nullIfEmpty(conditionID), nullIfEmpty(shapeID), nullIfEmpty(markingID), brandName,
				strings.TrimSpace(line.ModelNumber), strings.TrimSpace(line.ReferenceNumber), serial, productType,
				strings.TrimSpace(line.SKU), string(accessoryJSON), strings.TrimSpace(line.Notes), strings.TrimSpace(line.BeltText),
				strings.TrimSpace(line.DialText), line.BraceletQuantity,
				strings.TrimSpace(line.DuplicateSerialReason), convertedCostJPY, fxRateID, fxRateScaled, fxScale, now).Error; err != nil {
				return fmt.Errorf("insert purchase line %d: %w", index+1, err)
			}
		}
		return nil
	})
	if err != nil {
		return PurchaseSlipRecord{}, err
	}
	return r.PurchaseSlip(ctx, input.OrganizationID, purchaseID)
}

func (r *Repository) ConfirmPurchase(ctx context.Context, organizationID, purchaseID, actorUserID string) (PurchaseSlipRecord, error) {
	createdIDs := []string{}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var slip struct {
			Status                 string
			PurchaseDate           time.Time
			SupplierRoleID         string
			PurchaseStaffProfileID string
		}
		result := tx.Raw(`SELECT status,purchase_date,supplier_role_id,COALESCE(purchase_staff_profile_id,'') AS purchase_staff_profile_id
			FROM purchase_slips WHERE organization_id=? AND id=? FOR UPDATE`, organizationID, purchaseID).Scan(&slip)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrPurchaseNotFound
		}
		if slip.Status == "confirmed" {
			return nil
		}
		if slip.Status != "draft" {
			return ErrPurchaseState
		}
		type line struct {
			ID, SKU, BrandID, BrandText, ModelNumber, ReferenceNumber, SerialNumber, ProductType    string
			MaterialID, MovementID, ConditionID, ShapeID, MarkingID, CostCurrency, BaseSaleCurrency string
			AccessoryCodesJSON, Notes, BeltText, DialText, DuplicateSerialReason                    string
			BraceletQuantity                                                                        *int
			Quantity                                                                                int
			UnitCostMinor, BaseSalePriceMinor                                                       int64
		}
		var lines []line
		if err := tx.Raw(`SELECT id,sku,COALESCE(brand_id,'') AS brand_id,brand_text,model_number,reference_number,serial_number,product_type,
			COALESCE(material_id,'') AS material_id,COALESCE(movement_id,'') AS movement_id,
			COALESCE(condition_id,'') AS condition_id,COALESCE(shape_id,'') AS shape_id,COALESCE(marking_id,'') AS marking_id,cost_currency,base_sale_currency,
			accessory_codes::TEXT AS accessory_codes_json,notes,belt_text,dial_text,bracelet_quantity,
			duplicate_serial_reason,quantity,
			unit_cost_minor,base_sale_price_minor
			FROM purchase_slip_lines WHERE purchase_slip_id=? ORDER BY line_number FOR UPDATE`, purchaseID).Scan(&lines).Error; err != nil {
			return err
		}
		if len(lines) == 0 {
			return ErrPurchaseState
		}
		now := time.Now().UTC()
		for _, item := range lines {
			var accessoryCodes []string
			_ = json.Unmarshal([]byte(item.AccessoryCodesJSON), &accessoryCodes)
			accessoryIDs, accessoryNames, err := lookupAccessories(tx, organizationID, accessoryCodes)
			if err != nil {
				return err
			}
			conditionName := ""
			if item.ConditionID != "" {
				if err := tx.Table("product_conditions").Select("name").Where("id=? AND organization_id=?", item.ConditionID, organizationID).Scan(&conditionName).Error; err != nil {
					return err
				}
			}
			for unit := 0; unit < item.Quantity; unit++ {
				sequence, err := nextProductSequence(tx, organizationID, slip.PurchaseDate, now)
				if err != nil {
					return err
				}
				productID, err := database.NewID("prd")
				if err != nil {
					return err
				}
				productCode := slip.PurchaseDate.Format("20060102") + fmt.Sprintf("%03d", sequence)
				if err := tx.Exec(`INSERT INTO products(
					id,organization_id,product_code,sku,brand,brand_id,model_number,reference_number,serial_number,
					product_type,material_id,movement_id,condition_id,shape_id,marking_id,supplier_id,supplier_role_id,
					purchase_staff_profile_id,purchase_slip_line_id,purchase_date,cost_amount_minor,cost_currency,
					base_sale_price_minor,base_sale_currency,inventory_status,publication_status,condition_text,
					accessories,belt_text,dial_text,bracelet_quantity,notes,created_at,updated_at
				) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,'in_stock','private',?,?,?,?,?,?,?,?)`,
					productID, organizationID, productCode, item.SKU, item.BrandText, nullIfEmpty(item.BrandID), item.ModelNumber,
					item.ReferenceNumber, item.SerialNumber, item.ProductType, nullIfEmpty(item.MaterialID),
					nullIfEmpty(item.MovementID), nullIfEmpty(item.ConditionID), nullIfEmpty(item.ShapeID), nullIfEmpty(item.MarkingID), slip.SupplierRoleID,
					slip.SupplierRoleID, nullIfEmpty(slip.PurchaseStaffProfileID), item.ID, slip.PurchaseDate,
					item.UnitCostMinor, item.CostCurrency, item.BaseSalePriceMinor, item.BaseSaleCurrency,
					conditionName, strings.Join(accessoryNames, ", "), item.BeltText, item.DialText,
					item.BraceletQuantity, item.Notes, now, now).Error; err != nil {
					return fmt.Errorf("insert product from purchase line: %w", err)
				}
				for _, accessoryID := range accessoryIDs {
					if err := tx.Exec(`INSERT INTO product_accessories(product_id,accessory_id,quantity) VALUES(?,?,1)`, productID, accessoryID).Error; err != nil {
						return err
					}
				}
				eventID, _ := database.NewID("ive")
				reason := "仕入伝票確定"
				if item.DuplicateSerialReason != "" {
					reason += ": " + item.DuplicateSerialReason
				}
				if err := tx.Exec(`INSERT INTO inventory_events(
					id,organization_id,product_id,event_type,from_status,to_status,reason,actor_user_id,created_at
				) VALUES(?,?,?,'purchase_confirmed','','in_stock',?,?,?)`, eventID, organizationID, productID,
					reason, actorUserID, now).Error; err != nil {
					return err
				}
				createdIDs = append(createdIDs, productID)
			}
			if err := tx.Exec(`UPDATE purchase_slip_lines SET generated_product_count=? WHERE id=?`, item.Quantity, item.ID).Error; err != nil {
				return err
			}
		}
		return tx.Exec(`UPDATE purchase_slips SET status='confirmed',confirmed_at=?,confirmed_by=?,updated_at=?
			WHERE organization_id=? AND id=?`, now, actorUserID, now, organizationID, purchaseID).Error
	})
	if err != nil {
		return PurchaseSlipRecord{}, err
	}
	record, err := r.PurchaseSlip(ctx, organizationID, purchaseID)
	if err != nil {
		return PurchaseSlipRecord{}, err
	}
	for _, id := range createdIDs {
		product, productErr := r.ProductByID(ctx, organizationID, id)
		if productErr != nil {
			return PurchaseSlipRecord{}, productErr
		}
		record.CreatedProducts = append(record.CreatedProducts, product)
	}
	return record, nil
}

// IssuePurchase records only the exact time and administrator who issued the
// document. The purchase line already contains the registration-time FX
// snapshot, so issuance and reissuance must not update any exchange rate.
func (r *Repository) IssuePurchase(ctx context.Context, organizationID, purchaseID, actorUserID string) (PurchaseSlipRecord, error) {
	now := time.Now().UTC()
	result := r.db.WithContext(ctx).Exec(`UPDATE purchase_slips
		SET issued_at=?,issued_by=?,updated_at=?
		WHERE organization_id=? AND id=?`, now, actorUserID, now, organizationID, purchaseID)
	if result.Error != nil {
		return PurchaseSlipRecord{}, result.Error
	}
	if result.RowsAffected == 0 {
		return PurchaseSlipRecord{}, ErrPurchaseNotFound
	}
	return r.PurchaseSlip(ctx, organizationID, purchaseID)
}

func normalizeCodes(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		code := strings.ToUpper(strings.TrimSpace(value))
		if code != "" && !seen[code] {
			seen[code] = true
			result = append(result, code)
		}
	}
	return result
}
