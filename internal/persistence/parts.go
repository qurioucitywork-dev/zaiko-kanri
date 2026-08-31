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
	ErrDuplicatePartCode        = errors.New("duplicate part code")
	ErrInvalidPartCode          = errors.New("invalid part code format")
	ErrDailyPartLimit           = errors.New("daily part code limit reached")
	ErrPartStatus               = errors.New("invalid part status")
	ErrPartAdjustmentCostLocked = errors.New("cost-adjustment part cost is locked")
)

type PartInput struct {
	OrganizationID    string
	ActorUserID       string
	PartCode          string
	PurchaseDate      string
	StaffCode         string
	SupplierCode      string
	PurchaseTaxMode   string
	TaxCategory       string
	CostAmountMinor   int64
	CostCurrency      string
	SKU               string
	BrandCode         string
	ModelName         string
	ReferenceNumber   string
	PartNameCode      string
	DetailText        string
	DetailMasterType  string
	DetailMasterCode  string
	BraceletQuantity  *int
	SalePriceUSDMinor int64
	Notes             string
	InternalComment   string
}

type PartUpdateInput struct {
	OrganizationID    string
	ActorUserID       string
	PartID            string
	StaffCode         string
	SupplierCode      string
	PurchaseTaxMode   string
	TaxCategory       string
	CostAmountMinor   int64
	CostCurrency      string
	SKU               string
	BrandCode         string
	ModelName         string
	ReferenceNumber   string
	PartNameCode      string
	DetailText        string
	DetailMasterType  string
	DetailMasterCode  string
	BraceletQuantity  *int
	SalePriceUSDMinor int64
	Notes             string
	InternalComment   string
	Status            string
}

type Part struct {
	ID                 string    `json:"id"`
	PartCode           string    `json:"partCode"`
	PurchaseDate       string    `json:"purchaseDate" gorm:"column:purchase_date"`
	StaffCode          string    `json:"staffCode" gorm:"column:staff_code"`
	SupplierCode       string    `json:"supplierCode" gorm:"column:supplier_code"`
	PurchaseTaxMode    string    `json:"purchaseTaxMode"`
	TaxCategory        string    `json:"taxCategory"`
	CostAmountMinor    int64     `json:"costAmountMinor"`
	CostCurrency       string    `json:"costCurrency"`
	FixedCostJPYMinor  int64     `json:"fixedCostJpyMinor" gorm:"column:fixed_cost_jpy_minor"`
	FXRateScaled       *int64    `json:"fxRateScaled,omitempty"`
	FXScale            *int64    `json:"fxScale,omitempty"`
	SKU                string    `json:"sku"`
	BrandCode          string    `json:"brandCode" gorm:"column:brand_code"`
	BrandName          string    `json:"brandName" gorm:"column:brand_name"`
	ModelName          string    `json:"modelName"`
	ReferenceNumber    string    `json:"referenceNumber"`
	PartNameCode       string    `json:"partNameCode" gorm:"column:part_name_code"`
	PartName           string    `json:"partName" gorm:"column:part_name"`
	DetailText         string    `json:"detailText"`
	DetailMasterType   string    `json:"detailMasterType"`
	DetailMasterCode   string    `json:"detailMasterCode"`
	BraceletQuantity   *int      `json:"braceletQuantity,omitempty"`
	SalePriceUSDMinor  int64     `json:"salePriceUsdMinor"`
	Notes              string    `json:"notes"`
	InternalComment    string    `json:"internalComment"`
	CostAdjustmentID   string    `json:"costAdjustmentId,omitempty"`
	PurchaseSlipLineID string    `json:"purchaseSlipLineId,omitempty"`
	Status             string    `json:"status"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

func (r *Repository) Parts(ctx context.Context, organizationID string) ([]Part, error) {
	var items []Part
	err := r.db.WithContext(ctx).Raw(`
		SELECT p.id,p.part_code,p.purchase_date::text,p.purchase_tax_mode,p.tax_category,
			p.cost_amount_minor,p.cost_currency,p.fixed_cost_jpy_minor,p.fx_rate_scaled,p.fx_scale,p.sku,
			COALESCE(sp.staff_code,'') AS staff_code,COALESCE(pr.role_code,'') AS supplier_code,
			COALESCE(b.code,'') AS brand_code,p.brand_text AS brand_name,p.model_name,p.reference_number,
			pn.code AS part_name_code,p.part_name_text AS part_name,p.detail_text,p.detail_master_type,p.detail_master_code,p.bracelet_quantity,p.sale_price_usd_minor,
			p.notes,p.internal_comment,p.cost_adjustment_id,COALESCE(p.purchase_slip_line_id,'') AS purchase_slip_line_id,p.status,p.created_at,p.updated_at
		FROM parts p
		LEFT JOIN staff_profiles sp ON sp.id=p.purchase_staff_profile_id
		LEFT JOIN partner_roles pr ON pr.id=p.supplier_role_id
		LEFT JOIN brands b ON b.id=p.brand_id
		JOIN part_names pn ON pn.id=p.part_name_id
		WHERE p.organization_id=?
		ORDER BY p.purchase_date DESC,p.part_code DESC`, organizationID).Scan(&items).Error
	return items, err
}

func (r *Repository) CreatePart(ctx context.Context, input PartInput) (Part, error) {
	date, err := time.Parse("2006-01-02", strings.TrimSpace(input.PurchaseDate))
	if err != nil {
		return Part{}, err
	}
	purchaseTaxMode, _, err := normalizePurchaseTaxMode(input.PurchaseTaxMode)
	if err != nil {
		return Part{}, err
	}
	taxCategory, _, err := normalizePurchaseTaxCategory(input.TaxCategory, purchaseTaxMode)
	if err != nil {
		return Part{}, err
	}
	var createdID string
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		var supplierRoleID string
		if strings.TrimSpace(input.SupplierCode) == "" {
			if PurchaseSupplierRequired(purchaseTaxMode) {
				return ErrSupplierNotFound
			}
		} else if supplierRoleID, err = lookupSupplierRole(tx, input.OrganizationID, input.SupplierCode); err != nil {
			return err
		}
		staffID, err := lookupStaffProfile(tx, input.OrganizationID, input.ActorUserID, input.StaffCode)
		if err != nil {
			return err
		}
		brandID, brandName, err := lookupCatalog(tx, "brands", input.OrganizationID, input.BrandCode, false)
		if err != nil {
			return err
		}
		partNameID, partName, err := lookupCatalog(tx, "part_names", input.OrganizationID, input.PartNameCode, true)
		if err != nil {
			return err
		}
		detailMasterType, detailMasterCode, detailText, err := resolvePartDetailMaster(tx, input.OrganizationID, partName, input.DetailMasterType, input.DetailMasterCode, input.DetailText)
		if err != nil {
			return err
		}
		quantity := input.BraceletQuantity
		if !strings.EqualFold(strings.TrimSpace(partName), "BRACELET PARTS") {
			quantity = nil
		}
		convertedCostJPY, fxRateID, fxRateScaled, fxScale, err := purchaseCostSnapshot(tx, input.OrganizationID, input.CostAmountMinor, 1, strings.ToUpper(strings.TrimSpace(input.CostCurrency)))
		if err != nil {
			return err
		}
		partCode := strings.ToUpper(strings.TrimSpace(input.PartCode))
		if partCode == "" {
			sequence, err := nextPartSequence(tx, input.OrganizationID, date, now)
			if err != nil {
				return err
			}
			partCode = formatPartCode(date, sequence)
		} else {
			if !isPartCodeForDate(partCode, date) {
				return ErrInvalidPartCode
			}
			var duplicate int64
			if err := tx.Table("parts").Where("organization_id=? AND UPPER(BTRIM(part_code))=?", input.OrganizationID, partCode).Count(&duplicate).Error; err != nil {
				return err
			}
			if duplicate > 0 {
				return ErrDuplicatePartCode
			}
			if err := reservePartSequence(tx, input.OrganizationID, date, partCode, now); err != nil {
				return err
			}
		}
		createdID, err = database.NewID("prt")
		if err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO parts(
			id,organization_id,part_code,purchase_date,purchase_staff_profile_id,supplier_role_id,
			purchase_tax_mode,tax_category,cost_amount_minor,cost_currency,fixed_cost_jpy_minor,
			fx_rate_id,fx_rate_scaled,fx_scale,sku,brand_id,brand_text,model_name,reference_number,
			part_name_id,part_name_text,detail_text,detail_master_type,detail_master_code,bracelet_quantity,sale_price_usd_minor,notes,internal_comment,status,
			created_by,updated_by,created_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?, 'in_stock',?,?,?,?)`,
			createdID, input.OrganizationID, partCode, date, staffID, nullIfEmpty(supplierRoleID),
			purchaseTaxMode, taxCategory, input.CostAmountMinor, strings.ToUpper(strings.TrimSpace(input.CostCurrency)), convertedCostJPY,
			fxRateID, fxRateScaled, fxScale, strings.TrimSpace(input.SKU), nullIfEmpty(brandID), brandName,
			strings.TrimSpace(input.ModelName), strings.TrimSpace(input.ReferenceNumber), partNameID, partName,
			detailText, detailMasterType, detailMasterCode, quantity, input.SalePriceUSDMinor, strings.TrimSpace(input.Notes), strings.TrimSpace(input.InternalComment), input.ActorUserID, input.ActorUserID, now, now).Error
	})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "ux_parts_organization_part_code_normalized") {
			return Part{}, ErrDuplicatePartCode
		}
		return Part{}, err
	}
	items, err := r.Parts(ctx, input.OrganizationID)
	if err != nil {
		return Part{}, err
	}
	for _, item := range items {
		if item.ID == createdID {
			return item, nil
		}
	}
	return Part{}, gorm.ErrRecordNotFound
}

func (r *Repository) UpdatePart(ctx context.Context, input PartUpdateInput) (Part, error) {
	purchaseTaxMode, _, err := normalizePurchaseTaxMode(input.PurchaseTaxMode)
	if err != nil {
		return Part{}, err
	}
	taxCategory, _, err := normalizePurchaseTaxCategory(input.TaxCategory, purchaseTaxMode)
	if err != nil {
		return Part{}, err
	}
	status := strings.TrimSpace(input.Status)
	if status == "" {
		status = "in_stock"
	}
	if status != "in_stock" && status != "cost_adjustment" && status != "invalid" {
		return Part{}, ErrPartStatus
	}

	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current struct {
			ID                 string
			PurchaseSlipLineID string
			CostAdjustmentID   string
			CostAmountMinor    int64
			CostCurrency       string
			FixedCostJPYMinor  int64
		}
		result := tx.Raw(`SELECT id,COALESCE(purchase_slip_line_id,'') AS purchase_slip_line_id,
			COALESCE(cost_adjustment_id,'') AS cost_adjustment_id,cost_amount_minor,cost_currency,fixed_cost_jpy_minor
			FROM parts WHERE organization_id=? AND id=? FOR UPDATE`, input.OrganizationID, input.PartID).Scan(&current)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 || current.ID == "" {
			return gorm.ErrRecordNotFound
		}

		var supplierRoleID string
		if strings.TrimSpace(input.SupplierCode) == "" {
			if PurchaseSupplierRequired(purchaseTaxMode) {
				return ErrSupplierNotFound
			}
		} else if supplierRoleID, err = lookupSupplierRole(tx, input.OrganizationID, input.SupplierCode); err != nil {
			return err
		}
		staffID, err := lookupStaffProfile(tx, input.OrganizationID, input.ActorUserID, input.StaffCode)
		if err != nil {
			return err
		}
		brandID, brandName, err := lookupCatalog(tx, "brands", input.OrganizationID, input.BrandCode, false)
		if err != nil {
			return err
		}
		partNameID, partName, err := lookupCatalog(tx, "part_names", input.OrganizationID, input.PartNameCode, true)
		if err != nil {
			return err
		}
		detailMasterType, detailMasterCode, detailText, err := resolvePartDetailMaster(tx, input.OrganizationID, partName,
			input.DetailMasterType, input.DetailMasterCode, input.DetailText)
		if err != nil {
			return err
		}
		quantity := input.BraceletQuantity
		if !strings.EqualFold(strings.TrimSpace(partName), "BRACELET PARTS") {
			quantity = nil
		}
		currency := strings.ToUpper(strings.TrimSpace(input.CostCurrency))
		convertedCostJPY, fxRateID, fxRateScaled, fxScale, err := purchaseCostSnapshot(tx, input.OrganizationID, input.CostAmountMinor, 1, currency)
		if err != nil {
			return err
		}
		if current.CostAdjustmentID != "" && (currency != current.CostCurrency || input.CostAmountMinor != current.CostAmountMinor || convertedCostJPY != current.FixedCostJPYMinor) {
			return ErrPartAdjustmentCostLocked
		}

		now := time.Now().UTC()
		if err := tx.Table("parts").Where("organization_id=? AND id=?", input.OrganizationID, input.PartID).Updates(map[string]any{
			"purchase_staff_profile_id": staffID,
			"supplier_role_id":          nullIfEmpty(supplierRoleID),
			"purchase_tax_mode":         purchaseTaxMode,
			"tax_category":              taxCategory,
			"cost_amount_minor":         input.CostAmountMinor,
			"cost_currency":             currency,
			"fixed_cost_jpy_minor":      convertedCostJPY,
			"fx_rate_id":                fxRateID,
			"fx_rate_scaled":            fxRateScaled,
			"fx_scale":                  fxScale,
			"sku":                       strings.TrimSpace(input.SKU),
			"brand_id":                  nullIfEmpty(brandID),
			"brand_text":                brandName,
			"model_name":                strings.TrimSpace(input.ModelName),
			"reference_number":          strings.TrimSpace(input.ReferenceNumber),
			"part_name_id":              partNameID,
			"part_name_text":            partName,
			"detail_text":               detailText,
			"detail_master_type":        detailMasterType,
			"detail_master_code":        detailMasterCode,
			"bracelet_quantity":         quantity,
			"sale_price_usd_minor":      input.SalePriceUSDMinor,
			"notes":                     strings.TrimSpace(input.Notes),
			"internal_comment":          strings.TrimSpace(input.InternalComment),
			"status":                    status,
			"updated_by":                input.ActorUserID,
			"updated_at":                now,
		}).Error; err != nil {
			return err
		}

		if current.PurchaseSlipLineID != "" {
			if err := tx.Exec(`UPDATE purchase_slip_lines SET
				unit_cost_minor=?,cost_currency=?,base_sale_price_minor=?,base_sale_currency='USD',brand_id=?,brand_text=?,
				model_number=?,reference_number=?,product_type=?,sku=?,converted_total_jpy=?,fx_rate_snapshot_id=?,fx_rate_scaled=?,fx_scale=?,notes=?
				WHERE id=? AND purchase_slip_id IN (SELECT id FROM purchase_slips WHERE organization_id=?)`,
				input.CostAmountMinor, currency, input.SalePriceUSDMinor, nullIfEmpty(brandID), brandName,
				strings.TrimSpace(input.ModelName), strings.TrimSpace(input.ReferenceNumber), "パーツ: "+partName,
				strings.TrimSpace(input.SKU), convertedCostJPY, fxRateID, fxRateScaled, fxScale, strings.TrimSpace(input.Notes),
				current.PurchaseSlipLineID, input.OrganizationID).Error; err != nil {
				return err
			}
		}
		if current.CostAdjustmentID != "" {
			if err := tx.Exec(`UPDATE cost_adjustment_items SET status=? WHERE cost_adjustment_id=? AND output_part_id=?`,
				status, current.CostAdjustmentID, input.PartID).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Part{}, err
	}
	items, err := r.Parts(ctx, input.OrganizationID)
	if err != nil {
		return Part{}, err
	}
	for _, item := range items {
		if item.ID == input.PartID {
			return item, nil
		}
	}
	return Part{}, gorm.ErrRecordNotFound
}

func resolvePartDetailMaster(tx *gorm.DB, organizationID, partName, masterType, masterCode, freeText string) (string, string, string, error) {
	expectedType := map[string]string{"素材": "material", "ベルト素材": "belt", "文字盤": "dial"}[strings.TrimSpace(partName)]
	if expectedType == "" {
		return "", "", strings.TrimSpace(freeText), nil
	}
	code := strings.ToUpper(strings.TrimSpace(masterCode))
	if code == "" {
		return expectedType, "", "", nil
	}
	if strings.TrimSpace(masterType) != expectedType {
		return "", "", "", ErrMasterCodeNotFound
	}
	table := map[string]string{"material": "materials", "belt": "belt_materials", "dial": "dials"}[expectedType]
	_, name, err := lookupCatalog(tx, table, organizationID, code, true)
	if err != nil {
		return "", "", "", err
	}
	return expectedType, code, name, nil
}

func nextPartSequence(tx *gorm.DB, organizationID string, date, now time.Time) (int, error) {
	var sequence int
	result := tx.Raw(`
		INSERT INTO part_code_sequences(organization_id,business_date,last_sequence,updated_at)
		SELECT ?,?,COALESCE(MAX(RIGHT(part_code,4)::INTEGER),0)+1,?
		FROM parts WHERE organization_id=? AND purchase_date=? AND part_code ~ '^P[0-9]{10}$'
		ON CONFLICT (organization_id,business_date)
		DO UPDATE SET last_sequence=part_code_sequences.last_sequence+1,updated_at=EXCLUDED.updated_at
		WHERE part_code_sequences.last_sequence < 9999 RETURNING last_sequence`, organizationID, date, now, organizationID, date).Scan(&sequence)
	if result.Error != nil {
		return 0, result.Error
	}
	if result.RowsAffected == 0 || sequence < 1 || sequence > 9999 {
		return 0, ErrDailyPartLimit
	}
	return sequence, nil
}

func formatPartCode(date time.Time, sequence int) string {
	return "P" + date.Format("020106") + fmt.Sprintf("%04d", sequence)
}

func isPartCodeForDate(code string, date time.Time) bool {
	if len(code) != 11 || !strings.HasPrefix(code, "P"+date.Format("020106")) {
		return false
	}
	for _, char := range code[1:] {
		if char < '0' || char > '9' {
			return false
		}
	}
	return code[7:] != "0000"
}

func reservePartSequence(tx *gorm.DB, organizationID string, date time.Time, code string, now time.Time) error {
	sequence, err := strconv.Atoi(code[7:])
	if err != nil || sequence < 1 || sequence > 9999 {
		return ErrInvalidPartCode
	}
	return tx.Exec(`INSERT INTO part_code_sequences(organization_id,business_date,last_sequence,updated_at)
		VALUES(?,?,?,?) ON CONFLICT (organization_id,business_date)
		DO UPDATE SET last_sequence=GREATEST(part_code_sequences.last_sequence,EXCLUDED.last_sequence),updated_at=EXCLUDED.updated_at`,
		organizationID, date, sequence, now).Error
}
