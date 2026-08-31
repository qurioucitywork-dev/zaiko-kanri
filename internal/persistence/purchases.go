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
	"gorm.io/gorm/clause"
)

const purchaseArrivalSummaryJoin = `LEFT JOIN (
	SELECT entities.purchase_slip_id,
		COUNT(*) FILTER (WHERE entities.is_pending) AS pending_count,
		COUNT(*) FILTER (WHERE NOT entities.is_pending) AS arrived_count
	FROM (
		SELECT line.purchase_slip_id,product.id AS entity_id,
			product.inventory_status IN ('purchasing','cost_adjustment') AS is_pending
		FROM purchase_slip_lines line
		JOIN products product ON product.purchase_slip_line_id=line.id AND product.deleted_at IS NULL
		UNION ALL
		SELECT line.purchase_slip_id,part.id AS entity_id,
			part.status='cost_adjustment' AS is_pending
		FROM purchase_slip_lines line
		JOIN parts part ON part.purchase_slip_line_id=line.id
	) entities
	GROUP BY entities.purchase_slip_id
) arrivals ON arrivals.purchase_slip_id=p.id`

var (
	ErrPurchaseNotFound        = errors.New("purchase slip not found")
	ErrPurchaseState           = errors.New("purchase slip cannot be changed in its current state")
	ErrDuplicateSerial         = errors.New("serial number already exists")
	ErrQuantitySerialConflict  = errors.New("serial number cannot be used when quantity is greater than one")
	ErrPurchaseQuantity        = errors.New("purchase line quantity must be one")
	ErrPurchaseTaxMode         = errors.New("invalid purchase tax mode")
	ErrPurchaseTaxCategory     = errors.New("invalid purchase tax category")
	ErrPurchasePaymentMethod   = errors.New("invalid purchase payment method")
	ErrPurchaseInUse           = errors.New("purchase slip has products already used by another operation")
	ErrPurchaseProductNotFound = errors.New("product does not belong to purchase slip")
	ErrPurchaseArrivalState    = errors.New("product cannot be received in its current state")
)

const (
	PurchaseTaxModeDomestic           = "domestic"
	PurchaseTaxModePersonal           = "personal"
	PurchaseTaxModeOverseas           = "overseas"
	PurchaseTaxCategoryConsumptionTax = "consumption_tax"
	PurchaseTaxCategoryEquivalent     = "tax_equivalent"
	PurchaseTaxCategoryOutOfScope     = "out_of_scope"
	PurchasePaymentMethodCash         = "cash"
	PurchasePaymentMethodBankTransfer = "bank_transfer"
	PurchasePaymentMethodCard         = "card"
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
	SupplierName    string              `json:"supplierName"`
	StaffCode       string              `json:"staffCode"`
	PurchaseDate    string              `json:"purchaseDate"`
	PurchaseTaxMode string              `json:"purchaseTaxMode"`
	TaxCategory     string              `json:"taxCategory"`
	PaymentMethod   string              `json:"paymentMethod"`
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
	GeneratedPartCount    int        `json:"generatedPartCount"`
	LineItemKind          string     `json:"lineItemKind"`
	ConvertedTotalJPY     int64      `json:"convertedTotalJpy"`
	FXRateSnapshotID      string     `json:"fxRateSnapshotId"`
	FXRateScaled          int64      `json:"fxRateScaled"`
	FXScale               int64      `json:"fxScale"`
	FXRateObservedAt      *time.Time `json:"fxRateObservedAt,omitempty"`
	ProductID             string     `json:"productId,omitempty"`
	ProductCode           string     `json:"productCode,omitempty"`
	InventoryStatus       string     `json:"inventoryStatus,omitempty"`
	PartID                string     `json:"partId,omitempty"`
	PartCode              string     `json:"partCode,omitempty"`
	PartStatus            string     `json:"partStatus,omitempty"`
}

type PurchaseSlipRecord struct {
	ID                    string               `json:"id"`
	SlipNumber            string               `json:"slipNumber"`
	SupplierCode          string               `json:"supplierCode"`
	SupplierName          string               `json:"supplierName"`
	StaffCode             string               `json:"staffCode"`
	PurchaseDate          DateString           `json:"purchaseDate"`
	PurchaseTaxMode       string               `json:"purchaseTaxMode"`
	TaxCategory           string               `json:"taxCategory"`
	PaymentMethod         string               `json:"paymentMethod"`
	TaxRateBasisPoints    int                  `json:"taxRateBasisPoints"`
	Status                string               `json:"status"`
	IsSimple              bool                 `json:"isSimple"`
	Notes                 string               `json:"notes"`
	ConfirmedAt           *time.Time           `json:"confirmedAt,omitempty"`
	IssuedAt              *time.Time           `json:"issuedAt,omitempty"`
	IssuedBy              string               `json:"issuedBy,omitempty"`
	PaidAt                *time.Time           `json:"paidAt,omitempty"`
	PaidBy                string               `json:"paidBy,omitempty"`
	CancelledAt           *time.Time           `json:"cancelledAt,omitempty"`
	CancelledBy           string               `json:"cancelledBy,omitempty"`
	CancelReason          string               `json:"cancelReason,omitempty"`
	IssueFXRateSnapshotID string               `json:"issueFxRateSnapshotId,omitempty"`
	IssueFXRateScaled     int64                `json:"issueFxRateScaled"`
	IssueFXScale          int64                `json:"issueFxScale"`
	CreatedAt             time.Time            `json:"createdAt"`
	UpdatedAt             time.Time            `json:"updatedAt"`
	Lines                 []PurchaseLineRecord `gorm:"-" json:"lines,omitempty"`
	CreatedProducts       []Product            `gorm:"-" json:"createdProducts,omitempty"`
	OfficialPDF           *OfficialDocumentRef `gorm:"-" json:"officialPdf,omitempty"`
	PendingArrivalCount   int                  `json:"pendingArrivalCount"`
	ArrivedCount          int                  `json:"arrivedCount"`
	ArrivalStatus         string               `json:"arrivalStatus"`
}

type PurchaseArrivalResult struct {
	Result          string `json:"result"`
	ProductCode     string `json:"productCode"`
	InventoryStatus string `json:"inventoryStatus"`
}

func normalizePurchaseTaxMode(value string) (string, int, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", PurchaseTaxModeDomestic:
		return PurchaseTaxModeDomestic, 1000, nil
	case PurchaseTaxModePersonal, PurchaseTaxModeOverseas:
		return strings.ToLower(strings.TrimSpace(value)), 0, nil
	default:
		return "", 0, ErrPurchaseTaxMode
	}
}

// PurchaseSupplierRequired reports whether a supplier must be selected for
// the requested purchase mode. Unknown and omitted modes are treated as the
// safer domestic default, where the supplier remains mandatory.
func PurchaseSupplierRequired(value string) bool {
	mode, _, err := normalizePurchaseTaxMode(value)
	return err != nil || mode != PurchaseTaxModePersonal
}

func normalizePurchaseTaxCategory(value, purchaseTaxMode string) (string, int, error) {
	category := strings.ToLower(strings.TrimSpace(value))
	if category == "" {
		if purchaseTaxMode == PurchaseTaxModeDomestic {
			return PurchaseTaxCategoryConsumptionTax, 1000, nil
		}
		return PurchaseTaxCategoryOutOfScope, 0, nil
	}
	switch category {
	case PurchaseTaxCategoryConsumptionTax:
		return category, 1000, nil
	case PurchaseTaxCategoryEquivalent, PurchaseTaxCategoryOutOfScope:
		return category, 0, nil
	default:
		return "", 0, ErrPurchaseTaxCategory
	}
}

func normalizePurchasePaymentMethod(value string) (string, error) {
	method := strings.ToLower(strings.TrimSpace(value))
	if method == "" {
		return PurchasePaymentMethodBankTransfer, nil
	}
	switch method {
	case PurchasePaymentMethodCash, PurchasePaymentMethodBankTransfer, PurchasePaymentMethodCard:
		return method, nil
	default:
		return "", ErrPurchasePaymentMethod
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
		Where("organization_id=? AND status<>'cancelled'", organizationID).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var records []PurchaseSlipRecord
	err := r.db.WithContext(ctx).Table("purchase_slips AS p").
		Select(`p.id,p.slip_number,COALESCE(pr.role_code,'') AS supplier_code,COALESCE(NULLIF(p.supplier_name_text,''),bp.legal_name,'') AS supplier_name,
			COALESCE(sp.staff_code,'') AS staff_code,p.purchase_date,p.purchase_tax_mode,p.tax_category,p.payment_method,p.tax_rate_basis_points,
			p.status,p.is_simple,p.notes,
			p.confirmed_at,p.issued_at,COALESCE(p.issued_by,'') AS issued_by,p.paid_at,COALESCE(p.paid_by,'') AS paid_by,
			COALESCE(p.issue_fx_rate_snapshot_id,'') AS issue_fx_rate_snapshot_id,
			COALESCE(p.issue_fx_rate_scaled,0) AS issue_fx_rate_scaled,
			COALESCE(p.issue_fx_scale,0) AS issue_fx_scale,p.created_at,p.updated_at,
			COALESCE(arrivals.pending_count,0) AS pending_arrival_count,
			COALESCE(arrivals.arrived_count,0) AS arrived_count,
			CASE WHEN p.status<>'confirmed' OR COALESCE(arrivals.pending_count,0)>0 THEN 'processing' ELSE 'completed' END AS arrival_status`).
		Joins("LEFT JOIN partner_roles pr ON pr.id=p.supplier_role_id AND pr.organization_id=p.organization_id").
		Joins("LEFT JOIN business_partners bp ON bp.id=pr.partner_id AND bp.organization_id=p.organization_id").
		Joins("LEFT JOIN staff_profiles sp ON sp.id=p.purchase_staff_profile_id AND sp.organization_id=p.organization_id").
		Joins(purchaseArrivalSummaryJoin).
		Where("p.organization_id=? AND p.status<>'cancelled'", organizationID).
		Order("p.purchase_date DESC,p.slip_number DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Scan(&records).Error
	return records, total, err
}

// DeletedPurchaseSlipsPage returns archived (cancelled) purchase slips. They
// remain in PostgreSQL so their numbers and audit trail are never reused.
func (r *Repository) DeletedPurchaseSlipsPage(ctx context.Context, organizationID string, page, pageSize int) ([]PurchaseSlipRecord, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 100
	}
	var total int64
	if err := r.db.WithContext(ctx).Table("purchase_slips").
		Where("organization_id=? AND status='cancelled'", organizationID).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var records []PurchaseSlipRecord
	err := r.db.WithContext(ctx).Table("purchase_slips AS p").
		Select(`p.id,p.slip_number,COALESCE(pr.role_code,'') AS supplier_code,COALESCE(NULLIF(p.supplier_name_text,''),bp.legal_name,'') AS supplier_name,
			COALESCE(sp.staff_code,'') AS staff_code,p.purchase_date,p.purchase_tax_mode,p.tax_category,p.payment_method,p.tax_rate_basis_points,
			p.status,p.is_simple,p.notes,p.cancelled_at,COALESCE(p.cancelled_by,'') AS cancelled_by,
			p.cancel_reason,p.created_at,p.updated_at`).
		Joins("LEFT JOIN partner_roles pr ON pr.id=p.supplier_role_id AND pr.organization_id=p.organization_id").
		Joins("LEFT JOIN business_partners bp ON bp.id=pr.partner_id AND bp.organization_id=p.organization_id").
		Joins("LEFT JOIN staff_profiles sp ON sp.id=p.purchase_staff_profile_id AND sp.organization_id=p.organization_id").
		Where("p.organization_id=? AND p.status='cancelled'", organizationID).
		Order("p.cancelled_at DESC NULLS LAST,p.slip_number DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Scan(&records).Error
	return records, total, err
}

func (r *Repository) PurchaseSlip(ctx context.Context, organizationID, purchaseID string) (PurchaseSlipRecord, error) {
	var record PurchaseSlipRecord
	result := r.db.WithContext(ctx).Table("purchase_slips AS p").
		Select(`p.id,p.slip_number,COALESCE(pr.role_code,'') AS supplier_code,COALESCE(NULLIF(p.supplier_name_text,''),bp.legal_name,'') AS supplier_name,
			COALESCE(sp.staff_code,'') AS staff_code,p.purchase_date,p.purchase_tax_mode,p.tax_category,p.payment_method,p.tax_rate_basis_points,
			p.status,p.is_simple,p.notes,
			p.confirmed_at,p.issued_at,COALESCE(p.issued_by,'') AS issued_by,p.paid_at,COALESCE(p.paid_by,'') AS paid_by,
			COALESCE(p.issue_fx_rate_snapshot_id,'') AS issue_fx_rate_snapshot_id,
			COALESCE(p.issue_fx_rate_scaled,0) AS issue_fx_rate_scaled,
			COALESCE(p.issue_fx_scale,0) AS issue_fx_scale,p.created_at,p.updated_at,
			COALESCE(arrivals.pending_count,0) AS pending_arrival_count,
			COALESCE(arrivals.arrived_count,0) AS arrived_count,
			CASE WHEN p.status<>'confirmed' OR COALESCE(arrivals.pending_count,0)>0 THEN 'processing' ELSE 'completed' END AS arrival_status`).
		Joins("LEFT JOIN partner_roles pr ON pr.id=p.supplier_role_id AND pr.organization_id=p.organization_id").
		Joins("LEFT JOIN business_partners bp ON bp.id=pr.partner_id AND bp.organization_id=p.organization_id").
		Joins("LEFT JOIN staff_profiles sp ON sp.id=p.purchase_staff_profile_id AND sp.organization_id=p.organization_id").
		Joins(purchaseArrivalSummaryJoin).
		Where("p.organization_id=? AND p.id=? AND p.status<>'cancelled'", organizationID, purchaseID).Take(&record)
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
			l.bracelet_quantity,l.notes,l.generated_product_count,l.generated_part_count,l.line_item_kind,l.converted_total_jpy,
			COALESCE(l.fx_rate_snapshot_id,'') AS fx_rate_snapshot_id,COALESCE(l.fx_rate_scaled,0) AS fx_rate_scaled,
			COALESCE(l.fx_scale,0) AS fx_scale,fx.observed_at AS fx_rate_observed_at,
			COALESCE(product.id,'') AS product_id,COALESCE(product.product_code,'') AS product_code,
			COALESCE(product.inventory_status,'') AS inventory_status,
			COALESCE(part.id,'') AS part_id,COALESCE(part.part_code,'') AS part_code,
			COALESCE(part.status,'') AS part_status`).
		Joins("LEFT JOIN brands b ON b.id=l.brand_id").
		Joins("LEFT JOIN materials m ON m.id=l.material_id").
		Joins("LEFT JOIN movements mv ON mv.id=l.movement_id").
		Joins("LEFT JOIN product_conditions c ON c.id=l.condition_id").
		Joins("LEFT JOIN product_shapes ps ON ps.id=l.shape_id").
		Joins("LEFT JOIN markings mk ON mk.id=l.marking_id").
		Joins("LEFT JOIN exchange_rate_snapshots fx ON fx.id=l.fx_rate_snapshot_id").
		Joins("LEFT JOIN products product ON product.purchase_slip_line_id=l.id AND product.deleted_at IS NULL").
		Joins("LEFT JOIN parts part ON part.purchase_slip_line_id=l.id").
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

// DeletePurchase cancels a purchase slip and removes its still-unused products
// from active inventory. The row and sequence stay for audit, so its number is
// never issued again.
func (r *Repository) DeletePurchase(ctx context.Context, organizationID, purchaseID, actorUserID string) (PurchaseSlipRecord, error) {
	var before PurchaseSlipRecord
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Raw(`SELECT id,slip_number,status FROM purchase_slips
			WHERE organization_id=? AND id=? AND status<>'cancelled' FOR UPDATE`, organizationID, purchaseID).Scan(&before)
		if result.Error != nil {
			return result.Error
		}
		if before.ID == "" {
			return ErrPurchaseNotFound
		}

		var usedCount int64
		if err := tx.Table("products AS product").
			Joins("JOIN purchase_slip_lines AS line ON line.id=product.purchase_slip_line_id").
			Where("line.purchase_slip_id=? AND product.deleted_at IS NULL AND product.inventory_status NOT IN ?", purchaseID, []string{"purchasing", "in_stock"}).
			Count(&usedCount).Error; err != nil {
			return err
		}
		if usedCount > 0 {
			return ErrPurchaseInUse
		}

		now := time.Now().UTC()
		if err := tx.Table("purchase_slips").Where("organization_id=? AND id=?", organizationID, purchaseID).Updates(map[string]any{
			"status": "cancelled", "cancelled_at": now, "cancelled_by": actorUserID,
			"cancel_reason": "伝票一覧から削除", "updated_at": now,
		}).Error; err != nil {
			return err
		}
		return tx.Table("products AS product").
			Where(`product.organization_id=? AND product.deleted_at IS NULL AND product.purchase_slip_line_id IN
				(SELECT id FROM purchase_slip_lines WHERE purchase_slip_id=?)`, organizationID, purchaseID).
			Updates(map[string]any{"inventory_status": "cancelled", "cancelled_at": now, "cancelled_by": actorUserID,
				"cancel_reason": "仕入伝票削除", "deleted_at": now, "updated_at": now}).Error
	})
	return before, err
}

func (r *Repository) CreatePurchase(ctx context.Context, input PurchaseCreateInput) (PurchaseSlipRecord, error) {
	purchaseTaxMode, _, err := normalizePurchaseTaxMode(input.PurchaseTaxMode)
	if err != nil {
		return PurchaseSlipRecord{}, err
	}
	taxCategory, taxRateBasisPoints, err := normalizePurchaseTaxCategory(input.TaxCategory, purchaseTaxMode)
	if err != nil {
		return PurchaseSlipRecord{}, err
	}
	paymentMethod, err := normalizePurchasePaymentMethod(input.PaymentMethod)
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
	supplierName := strings.TrimSpace(input.SupplierName)
	if purchaseTaxMode != PurchaseTaxModePersonal {
		supplierName = ""
	}
	var purchaseID string
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
			id,organization_id,slip_number,supplier_role_id,supplier_name_text,purchase_staff_profile_id,purchase_date,status,is_simple,
			purchase_tax_mode,tax_category,payment_method,tax_rate_basis_points,notes,created_by,created_at,updated_at
		) VALUES(?,?,?,?,?,?,?,'draft',FALSE,?,?,?,?,?,?,?,?)`, purchaseID, input.OrganizationID, slipNumber,
			nullIfEmpty(supplierRoleID), supplierName, staffID, date, purchaseTaxMode, taxCategory, paymentMethod, taxRateBasisPoints, strings.TrimSpace(input.Notes), input.ActorUserID, now, now).Error; err != nil {
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
		result := tx.Raw(`SELECT status,purchase_date,COALESCE(supplier_role_id,'') AS supplier_role_id,COALESCE(purchase_staff_profile_id,'') AS purchase_staff_profile_id
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
				productCode := formatProductCode(slip.PurchaseDate, sequence)
				if err := tx.Exec(`INSERT INTO products(
					id,organization_id,product_code,sku,brand,brand_id,model_number,reference_number,serial_number,
					product_type,material_id,movement_id,condition_id,shape_id,marking_id,supplier_id,supplier_role_id,
					purchase_staff_profile_id,purchase_slip_line_id,purchase_date,cost_amount_minor,cost_currency,
					base_sale_price_minor,base_sale_currency,inventory_status,publication_status,condition_text,
					accessories,belt_text,dial_text,bracelet_quantity,notes,created_at,updated_at
				) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,'purchasing','private',?,?,?,?,?,?,?,?)`,
					productID, organizationID, productCode, item.SKU, item.BrandText, nullIfEmpty(item.BrandID), item.ModelNumber,
					item.ReferenceNumber, item.SerialNumber, item.ProductType, nullIfEmpty(item.MaterialID),
					nullIfEmpty(item.MovementID), nullIfEmpty(item.ConditionID), nullIfEmpty(item.ShapeID), nullIfEmpty(item.MarkingID), slip.SupplierRoleID,
					nullIfEmpty(slip.SupplierRoleID), nullIfEmpty(slip.PurchaseStaffProfileID), item.ID, slip.PurchaseDate,
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
				) VALUES(?,?,?,'purchase_confirmed','','purchasing',?,?,?)`, eventID, organizationID, productID,
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

// ReceivePurchaseProduct records physical arrival of one product. In addition
// to the initial purchasing state, a cost-adjustment output remains pending
// until its new management number has been physically scanned.
func (r *Repository) ReceivePurchaseProduct(ctx context.Context, organizationID, purchaseID, productCode, actorUserID string) (PurchaseArrivalResult, error) {
	result := PurchaseArrivalResult{ProductCode: strings.TrimSpace(productCode)}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var product struct {
			ID               string
			InventoryStatus  string
			CostAdjustmentID string
		}
		query := tx.Table("products AS product").
			Select("product.id,product.inventory_status,COALESCE(product.cost_adjustment_id,'') AS cost_adjustment_id").
			Joins("JOIN purchase_slip_lines line ON line.id=product.purchase_slip_line_id").
			Joins("JOIN purchase_slips slip ON slip.id=line.purchase_slip_id").
			Where(`slip.organization_id=? AND slip.id=? AND product.product_code=?
				AND product.deleted_at IS NULL AND slip.status='confirmed'`, organizationID, purchaseID, result.ProductCode).
			Clauses(clause.Locking{Strength: "UPDATE"}).Take(&product)
		if query.Error != nil && !errors.Is(query.Error, gorm.ErrRecordNotFound) {
			return query.Error
		}
		if errors.Is(query.Error, gorm.ErrRecordNotFound) || product.ID == "" {
			return ErrPurchaseProductNotFound
		}
		if product.InventoryStatus == "in_stock" {
			result.Result = "already_received"
			result.InventoryStatus = "in_stock"
			return nil
		}
		if product.InventoryStatus != "purchasing" && product.InventoryStatus != "cost_adjustment" {
			return ErrPurchaseArrivalState
		}
		fromStatus := product.InventoryStatus
		now := time.Now().UTC()
		if err := tx.Exec(`UPDATE products SET inventory_status='in_stock',updated_at=?
			WHERE organization_id=? AND id=? AND inventory_status=?`, now, organizationID, product.ID, fromStatus).Error; err != nil {
			return err
		}
		if fromStatus == "cost_adjustment" && product.CostAdjustmentID != "" {
			if err := tx.Table("cost_adjustment_items").Where(
				"cost_adjustment_id=? AND output_product_id=?", product.CostAdjustmentID, product.ID,
			).Update("status", "completed").Error; err != nil {
				return err
			}
		}
		eventID, err := database.NewID("ive")
		if err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO inventory_events(
			id,organization_id,product_id,event_type,from_status,to_status,reason,actor_user_id,created_at
		) VALUES(?,?,?,'purchase_arrival_scan',?,'in_stock','仕入伝票の入荷スキャン',?,?)`,
			eventID, organizationID, product.ID, fromStatus, actorUserID, now).Error; err != nil {
			return err
		}
		result.Result = "received"
		result.InventoryStatus = "in_stock"
		return nil
	})
	return result, err
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

func (r *Repository) MarkPurchasePaid(ctx context.Context, organizationID, purchaseID, actorUserID string) (PurchaseSlipRecord, error) {
	now := time.Now().UTC()
	result := r.db.WithContext(ctx).Exec(`UPDATE purchase_slips
		SET paid_at=COALESCE(paid_at,?),paid_by=COALESCE(paid_by,?),updated_at=?
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
