package persistence

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"gorm.io/gorm"
)

var (
	ErrMarketImportHeader = errors.New("invalid market import header")
	ErrMarketImportRows   = errors.New("market import contains invalid rows")
	ErrMarketImportState  = errors.New("market import cannot be changed in its current state")
	ErrApprovalSelf       = errors.New("requester cannot approve own request")
)

type MarketPriceRecord struct {
	ID                 string     `json:"id"`
	ImportDate         DateString `json:"importDate"`
	BrandID            string     `json:"brandId"`
	BrandCode          string     `json:"brandCode"`
	BrandName          string     `json:"brandName"`
	ModelNumber        string     `json:"modelNumber"`
	ReferenceNumber    string     `json:"referenceNumber"`
	SerialNumber       string     `json:"serialNumber"`
	SKU                string     `json:"sku"`
	ConditionID        string     `json:"conditionId"`
	ConditionCode      string     `json:"conditionCode"`
	ConditionName      string     `json:"conditionName"`
	WarrantyYearMonth  string     `json:"warrantyYearMonth,omitempty"`
	PurchasePriceMinor int64      `json:"purchasePriceMinor"`
	PurchaseCurrency   string     `json:"purchaseCurrency"`
	MarketPriceMinor   int64      `json:"marketPriceMinor"`
	MarketCurrency     string     `json:"marketCurrency"`
	MarketFXRateScaled int64      `json:"marketFxRateScaled"`
	MarketFXScale      int64      `json:"marketFxScale"`
	MarketFXRate       string     `gorm:"-" json:"marketFxRate"`
	SupplierCode       string     `json:"supplierCode,omitempty"`
	StaffCode          string     `json:"staffCode,omitempty"`
	StaffName          string     `json:"staffName,omitempty"`
	MaterialCode       string     `json:"materialCode,omitempty"`
	MovementCode       string     `json:"movementCode,omitempty"`
	PurchaseDate       DateString `json:"purchaseDate,omitempty"`
	StatusText         string     `json:"statusText,omitempty"`
	BoxCode            string     `json:"boxCode,omitempty"`
	AccessoryCodes     string     `json:"accessoryCodes,omitempty"`
	BraceletQuantity   *int       `json:"braceletQuantity,omitempty"`
	AuctionCode        string     `json:"auctionCode,omitempty"`
	AuctionName        string     `json:"auctionName,omitempty"`
	Source             string     `json:"source"`
	Notes              string     `json:"notes"`
	IsActive           bool       `json:"isActive"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

type MarketPriceInput struct {
	OrganizationID     string
	ActorUserID        string
	ImportDate         string
	BrandCode          string
	ModelNumber        string
	ReferenceNumber    string
	SerialNumber       string
	SKU                string
	ConditionCode      string
	WarrantyYearMonth  string
	PurchasePriceMinor int64
	PurchaseCurrency   string
	MarketPriceMinor   int64
	MarketCurrency     string
	MarketFXRateScaled int64
	MarketFXScale      int64
	SupplierCode       string
	StaffCode          string
	MaterialCode       string
	MovementCode       string
	PurchaseDate       string
	StatusText         string
	BoxCode            string
	AccessoryCodes     []string
	BraceletQuantity   *int
	AuctionCode        string
	Source             string
	Notes              string
}

func marketPriceQuery(db *gorm.DB) *gorm.DB {
	return db.Table("market_price_records AS m").
		Select(`m.id,m.import_date,m.brand_id,COALESCE(b.code,'') AS brand_code,
			COALESCE(NULLIF(m.brand_text,''),b.name,'') AS brand_name,m.model_number,
			m.reference_number,m.serial_number,m.sku,m.condition_id,m.warranty_year_month,
			m.purchase_price_minor,m.purchase_currency,m.market_price_minor,m.market_currency,
			m.market_fx_rate_scaled,m.market_fx_scale,
			COALESCE(sr.role_code,'') AS supplier_code,COALESCE(sp.staff_code,'') AS staff_code,
			COALESCE(su.display_name,'') AS staff_name,
			COALESCE(NULLIF(m.material_text,''),mt.code,'') AS material_code,
			COALESCE(NULLIF(m.movement_text,''),mv.code,'') AS movement_code,
			COALESCE(NULLIF(m.condition_text,''),c.code,'') AS condition_code,
			COALESCE(NULLIF(m.condition_text,''),c.name,'') AS condition_name,
			COALESCE(TO_CHAR(m.purchase_date,'YYYY-MM-DD'),'') AS purchase_date,
			m.status_text,COALESCE(bx.box_code,'') AS box_code,m.bracelet_quantity,
			COALESCE(NULLIF(m.accessory_text,''),(SELECT STRING_AGG(a.code,',' ORDER BY a.sort_order,a.code)
				FROM market_price_accessories ma JOIN accessories a ON a.id=ma.accessory_id
				WHERE ma.market_price_record_id=m.id),'') AS accessory_codes,
			COALESCE(ah.code,'') AS auction_code,
			COALESCE(ah.name,CASE WHEN m.source !~* '^(manual|csv|preview-seed|domestic-auction|overseas|domestic-retail)$' THEN m.source ELSE '' END,'') AS auction_name,
			m.source,m.notes,m.is_active,m.created_at,m.updated_at`).
		Joins("LEFT JOIN brands b ON b.id=m.brand_id AND b.organization_id=m.organization_id").
		Joins("LEFT JOIN product_conditions c ON c.id=m.condition_id AND c.organization_id=m.organization_id").
		Joins("LEFT JOIN partner_roles sr ON sr.id=m.supplier_role_id AND sr.organization_id=m.organization_id").
		Joins("LEFT JOIN staff_profiles sp ON sp.id=m.purchase_staff_profile_id AND sp.organization_id=m.organization_id").
		Joins("LEFT JOIN users su ON su.id=sp.user_id AND su.organization_id=m.organization_id").
		Joins("LEFT JOIN materials mt ON mt.id=m.material_id AND mt.organization_id=m.organization_id").
		Joins("LEFT JOIN movements mv ON mv.id=m.movement_id AND mv.organization_id=m.organization_id").
		Joins("LEFT JOIN auction_houses ah ON ah.id=m.auction_house_id AND ah.organization_id=m.organization_id").
		Joins("LEFT JOIN publication_boxes bx ON bx.id=m.box_id AND bx.organization_id=m.organization_id")
}

func (r *Repository) MarketPrices(ctx context.Context, organizationID string, limit int) ([]MarketPriceRecord, error) {
	if limit < 1 || limit > 1000 {
		limit = 200
	}
	var records []MarketPriceRecord
	err := marketPriceQuery(r.db.WithContext(ctx)).
		Where("m.organization_id = ? AND m.is_active", organizationID).
		Order("m.import_date DESC,m.created_at DESC").Limit(limit).Scan(&records).Error
	for index := range records {
		records[index].MarketFXRate = formatRateValue(records[index].MarketFXRateScaled, records[index].MarketFXScale)
	}
	return records, err
}

func resolveMarketFXRate(tx *gorm.DB, organizationID, currency string, importDate time.Time, rateScaled, scale int64) (int64, int64, error) {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if currency == "JPY" {
		return 100000000, 100000000, nil
	}
	if currency != "USD" && currency != "HKD" {
		return 0, 0, ErrMarketImportRows
	}
	if rateScaled > 0 && scale > 0 {
		return rateScaled, scale, nil
	}
	var snapshot struct {
		RateScaled int64
		Scale      int64
	}
	result := tx.Table("exchange_rate_snapshots").
		Select("rate_scaled,scale").
		Where("organization_id=? AND base_currency=? AND quote_currency='JPY' AND observed_at < ?",
			organizationID, currency, importDate.AddDate(0, 0, 1)).
		Order("observed_at DESC,created_at DESC").Limit(1).Scan(&snapshot)
	if result.Error != nil || snapshot.RateScaled <= 0 || snapshot.Scale <= 0 {
		return 0, 0, ErrExchangeRate
	}
	return snapshot.RateScaled, snapshot.Scale, nil
}

func (r *Repository) CreateMarketPrice(ctx context.Context, input MarketPriceInput) (MarketPriceRecord, error) {
	date, err := time.Parse("2006-01-02", input.ImportDate)
	if err != nil {
		return MarketPriceRecord{}, err
	}
	input.WarrantyYearMonth, err = normalizeMarketWarrantyYearMonth(input.WarrantyYearMonth)
	if err != nil {
		return MarketPriceRecord{}, err
	}
	id, err := database.NewID("mkt")
	if err != nil {
		return MarketPriceRecord{}, err
	}
	now := time.Now().UTC()
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		input.MarketFXRateScaled, input.MarketFXScale, err = resolveMarketFXRate(tx, input.OrganizationID,
			input.MarketCurrency, date, input.MarketFXRateScaled, input.MarketFXScale)
		if err != nil {
			return err
		}
		brandID, brandName, err := lookupCatalog(tx, "brands", input.OrganizationID, input.BrandCode, true)
		if err != nil {
			return err
		}
		conditionID, _, err := lookupCatalog(tx, "product_conditions", input.OrganizationID, input.ConditionCode, false)
		if err != nil {
			return err
		}
		source := strings.TrimSpace(input.Source)
		if source == "" {
			source = "manual"
		}
		var supplierRoleID, staffProfileID, materialID, movementID, boxID, auctionHouseID string
		if strings.TrimSpace(input.AuctionCode) != "" {
			auctionHouseID, _, err = lookupCatalog(tx, "auction_houses", input.OrganizationID, input.AuctionCode, true)
			if err != nil {
				return err
			}
		}
		if strings.TrimSpace(input.SupplierCode) != "" {
			supplierRoleID, err = lookupSupplierRole(tx, input.OrganizationID, input.SupplierCode)
			if err != nil {
				return err
			}
		}
		if strings.TrimSpace(input.StaffCode) != "" {
			staffProfileID, err = lookupStaffProfile(tx, input.OrganizationID, input.ActorUserID, input.StaffCode)
			if err != nil {
				return err
			}
		}
		materialID, _, err = lookupCatalog(tx, "materials", input.OrganizationID, input.MaterialCode, false)
		if err != nil {
			return err
		}
		movementID, _, err = lookupCatalog(tx, "movements", input.OrganizationID, input.MovementCode, false)
		if err != nil {
			return err
		}
		if strings.TrimSpace(input.BoxCode) != "" {
			result := tx.Table("publication_boxes").Select("id").Where("organization_id=? AND box_code=?", input.OrganizationID, strings.TrimSpace(input.BoxCode)).Scan(&boxID)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return ErrMasterNotFound
			}
		}
		var purchaseDate any
		if strings.TrimSpace(input.PurchaseDate) != "" {
			parsed, parseErr := time.Parse("2006-01-02", input.PurchaseDate)
			if parseErr != nil {
				return parseErr
			}
			purchaseDate = parsed
		}
		if err := tx.Exec(`
			INSERT INTO market_price_records(
				id,organization_id,import_date,brand_id,brand_text,model_number,reference_number,serial_number,sku,condition_id,warranty_year_month,
				purchase_price_minor,purchase_currency,market_price_minor,market_currency,market_fx_rate_scaled,market_fx_scale,supplier_role_id,purchase_staff_profile_id,
				material_id,movement_id,purchase_date,status_text,box_id,bracelet_quantity,auction_house_id,source,notes,is_active,
				created_by,updated_by,created_at,updated_at
			) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,TRUE,?,?,?,?)`,
			id, input.OrganizationID, date, brandID, brandName, strings.TrimSpace(input.ModelNumber),
			strings.TrimSpace(input.ReferenceNumber), strings.TrimSpace(input.SerialNumber), strings.TrimSpace(input.SKU),
			nullIfEmpty(conditionID), input.WarrantyYearMonth, input.PurchasePriceMinor, input.PurchaseCurrency, input.MarketPriceMinor,
			input.MarketCurrency, input.MarketFXRateScaled, input.MarketFXScale, nullIfEmpty(supplierRoleID), nullIfEmpty(staffProfileID), nullIfEmpty(materialID),
			nullIfEmpty(movementID), purchaseDate, strings.TrimSpace(input.StatusText), nullIfEmpty(boxID), input.BraceletQuantity, nullIfEmpty(auctionHouseID), source,
			strings.TrimSpace(input.Notes), input.ActorUserID, input.ActorUserID, now, now).Error; err != nil {
			return err
		}
		return replaceMarketPriceAccessories(tx, input.OrganizationID, id, input.AccessoryCodes, now)
	})
	if err != nil {
		return MarketPriceRecord{}, err
	}
	return r.MarketPrice(ctx, input.OrganizationID, id)
}

func (r *Repository) MarketPrice(ctx context.Context, organizationID, marketPriceID string) (MarketPriceRecord, error) {
	var record MarketPriceRecord
	result := marketPriceQuery(r.db.WithContext(ctx)).
		Where("m.organization_id=? AND m.id=? AND m.is_active", organizationID, marketPriceID).Take(&record)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return MarketPriceRecord{}, ErrMasterNotFound
	}
	record.MarketFXRate = formatRateValue(record.MarketFXRateScaled, record.MarketFXScale)
	return record, result.Error
}

func replaceMarketPriceAccessories(tx *gorm.DB, organizationID, marketPriceID string, codes []string, now time.Time) error {
	if err := tx.Exec("DELETE FROM market_price_accessories WHERE market_price_record_id=?", marketPriceID).Error; err != nil {
		return err
	}
	accessoryIDs, _, err := lookupAccessories(tx, organizationID, codes)
	if err != nil {
		return err
	}
	for _, accessoryID := range accessoryIDs {
		if err := tx.Exec(`INSERT INTO market_price_accessories(market_price_record_id,accessory_id,created_at)
			VALUES(?,?,?)`, marketPriceID, accessoryID, now).Error; err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) UpdateMarketPrice(ctx context.Context, organizationID, marketPriceID, actorUserID string, input MarketPriceInput) (MarketPriceRecord, error) {
	date, err := time.Parse("2006-01-02", input.ImportDate)
	if err != nil || strings.TrimSpace(input.ModelNumber) == "" || input.PurchasePriceMinor < 0 || input.MarketPriceMinor < 0 {
		return MarketPriceRecord{}, ErrMarketImportRows
	}
	input.WarrantyYearMonth, err = normalizeMarketWarrantyYearMonth(input.WarrantyYearMonth)
	if err != nil {
		return MarketPriceRecord{}, ErrMarketImportRows
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		input.MarketFXRateScaled, input.MarketFXScale, err = resolveMarketFXRate(tx, organizationID,
			input.MarketCurrency, date, input.MarketFXRateScaled, input.MarketFXScale)
		if err != nil {
			return err
		}
		var exists int64
		if err := tx.Table("market_price_records").Where("organization_id=? AND id=? AND is_active", organizationID, marketPriceID).Count(&exists).Error; err != nil {
			return err
		}
		if exists == 0 {
			return ErrMasterNotFound
		}
		brandID, brandName, err := lookupCatalog(tx, "brands", organizationID, input.BrandCode, true)
		if err != nil {
			return err
		}
		conditionID, _, err := lookupCatalog(tx, "product_conditions", organizationID, input.ConditionCode, false)
		if err != nil {
			return err
		}
		materialID, _, err := lookupCatalog(tx, "materials", organizationID, input.MaterialCode, false)
		if err != nil {
			return err
		}
		movementID, _, err := lookupCatalog(tx, "movements", organizationID, input.MovementCode, false)
		if err != nil {
			return err
		}
		var supplierRoleID, staffProfileID, boxID, auctionHouseID string
		if strings.TrimSpace(input.AuctionCode) != "" {
			auctionHouseID, _, err = lookupCatalog(tx, "auction_houses", organizationID, input.AuctionCode, true)
			if err != nil {
				return err
			}
		}
		if strings.TrimSpace(input.SupplierCode) != "" {
			supplierRoleID, err = lookupSupplierRole(tx, organizationID, input.SupplierCode)
			if err != nil {
				return err
			}
		}
		if strings.TrimSpace(input.StaffCode) != "" {
			staffProfileID, err = lookupStaffProfile(tx, organizationID, actorUserID, input.StaffCode)
			if err != nil {
				return err
			}
		}
		if strings.TrimSpace(input.BoxCode) != "" {
			result := tx.Table("publication_boxes").Select("id").Where("organization_id=? AND box_code=?", organizationID, strings.TrimSpace(input.BoxCode)).Scan(&boxID)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return ErrMasterNotFound
			}
		}
		var purchaseDate any
		if strings.TrimSpace(input.PurchaseDate) != "" {
			parsed, parseErr := time.Parse("2006-01-02", input.PurchaseDate)
			if parseErr != nil {
				return parseErr
			}
			purchaseDate = parsed
		}
		now := time.Now().UTC()
		result := tx.Exec(`UPDATE market_price_records SET import_date=?,brand_id=?,brand_text=?,model_number=?,
			reference_number=?,serial_number=?,sku=?,condition_id=?,warranty_year_month=?,purchase_price_minor=?,purchase_currency=?,
			market_price_minor=?,market_currency=?,market_fx_rate_scaled=?,market_fx_scale=?,supplier_role_id=?,purchase_staff_profile_id=?,material_id=?,movement_id=?,
			purchase_date=?,status_text=?,box_id=?,bracelet_quantity=?,auction_house_id=?,source=?,notes=?,updated_by=?,updated_at=?
			WHERE organization_id=? AND id=? AND is_active`, date, brandID, brandName, strings.TrimSpace(input.ModelNumber),
			strings.TrimSpace(input.ReferenceNumber), strings.TrimSpace(input.SerialNumber), strings.TrimSpace(input.SKU),
			nullIfEmpty(conditionID), input.WarrantyYearMonth, input.PurchasePriceMinor, input.PurchaseCurrency, input.MarketPriceMinor, input.MarketCurrency,
			input.MarketFXRateScaled, input.MarketFXScale,
			nullIfEmpty(supplierRoleID), nullIfEmpty(staffProfileID), nullIfEmpty(materialID), nullIfEmpty(movementID),
			purchaseDate, strings.TrimSpace(input.StatusText), nullIfEmpty(boxID), input.BraceletQuantity, nullIfEmpty(auctionHouseID), strings.TrimSpace(input.Source), strings.TrimSpace(input.Notes),
			actorUserID, now, organizationID, marketPriceID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrMasterNotFound
		}
		return replaceMarketPriceAccessories(tx, organizationID, marketPriceID, input.AccessoryCodes, now)
	})
	if err != nil {
		return MarketPriceRecord{}, err
	}
	return r.MarketPrice(ctx, organizationID, marketPriceID)
}

type MarketImportRowRecord struct {
	ID                   string `json:"id"`
	RowNumber            int    `json:"rowNumber"`
	ImportDate           string `json:"importDate"`
	BrandText            string `json:"brandText"`
	ModelNumber          string `json:"modelNumber"`
	ReferenceNumber      string `json:"referenceNumber"`
	ConditionText        string `json:"conditionText"`
	WarrantyYearMonth    string `json:"warrantyYearMonth"`
	SKU                  string `json:"sku"`
	MaterialText         string `json:"materialText"`
	MovementText         string `json:"movementText"`
	AccessoryText        string `json:"accessoryText"`
	BraceletQuantity     *int   `json:"braceletQuantity,omitempty"`
	AuctionCode          string `json:"auctionCode"`
	PurchasePriceMinor   int64  `json:"purchasePriceMinor"`
	PurchaseCurrency     string `json:"purchaseCurrency"`
	MarketPriceMinor     int64  `json:"marketPriceMinor"`
	MarketCurrency       string `json:"marketCurrency"`
	MarketFXRateScaled   int64  `json:"marketFxRateScaled"`
	MarketFXScale        int64  `json:"marketFxScale"`
	MarketFXRate         string `gorm:"-" json:"marketFxRate"`
	Source               string `json:"source"`
	Notes                string `json:"notes"`
	Valid                bool   `json:"valid"`
	ErrorMessage         string `json:"errorMessage"`
	DuplicateCandidateID string `json:"duplicateCandidateId,omitempty"`
}

type MarketImportBatchRecord struct {
	ID            string                  `json:"id"`
	FileName      string                  `json:"fileName"`
	Status        string                  `json:"status"`
	TotalRows     int                     `json:"totalRows"`
	ValidRows     int                     `json:"validRows"`
	ErrorRows     int                     `json:"errorRows"`
	DuplicateRows int                     `json:"duplicateRows"`
	CreatedBy     string                  `json:"createdBy"`
	CreatedAt     time.Time               `json:"createdAt"`
	CommittedBy   string                  `json:"committedBy,omitempty"`
	CommittedAt   *time.Time              `json:"committedAt,omitempty"`
	Rows          []MarketImportRowRecord `gorm:"-" json:"rows,omitempty"`
}

var marketHeaderAliases = map[string]string{
	"import_date": "import_date", "取込日付": "import_date", "取り込み日付": "import_date", "オークション開催日": "import_date", "市場調査日": "import_date",
	"market_category": "market_category", "市場区分": "market_category",
	"brand_text": "brand_text", "brand": "brand_text", "ブランド": "brand_text", "ブランド名": "brand_text", "ブランドコード": "brand_text",
	"model_number": "model_number", "モデル番号": "model_number", "モデル名": "model_number", "モデル": "model_number",
	"reference_number": "reference_number", "リファレンス番号": "reference_number", "リファレンス": "reference_number", "型番": "reference_number",
	"condition_text": "condition_text", "condition": "condition_text", "コンディション": "condition_text", "コンディションコード": "condition_text", "状態": "condition_text",
	"warranty_year_month": "warranty_year_month", "warranty": "warranty_year_month", "保証年月": "warranty_year_month", "保証年/月": "warranty_year_month",
	"sku": "sku", "SKU": "sku",
	"material_text": "material_text", "material": "material_text", "素材": "material_text",
	"movement_text": "movement_text", "movement": "movement_text", "駆動方式": "movement_text",
	"accessory_text": "accessory_text", "accessories": "accessory_text", "付属品": "accessory_text", "付属品コード": "accessory_text",
	"bracelet_quantity": "bracelet_quantity", "braceletqty": "bracelet_quantity", "コマ数": "bracelet_quantity", "BRACELET PARTS数量": "bracelet_quantity",
	"auction_code": "auction_code", "オークションコード": "auction_code",
	"purchase_price": "purchase_price", "仕入価格": "purchase_price", "仕入れ価格": "purchase_price",
	"purchase_currency": "purchase_currency", "仕入通貨": "purchase_currency",
	"market_price": "market_price", "相場価格": "market_price", "取引価格": "market_price", "取引価格（JPY）": "market_price", "落札価格": "market_price", "落札価格（JPY）": "market_price",
	"market_currency": "market_currency", "相場通貨": "market_currency", "取引通貨": "market_currency", "市場調査通貨": "market_currency",
	"market_fx_rate": "market_fx_rate", "市場調査レート": "market_fx_rate",
	"notes": "notes", "備考": "notes",
}

var requiredMarketHeaders = []string{
	"import_date", "brand_text", "model_number", "market_price",
}

func splitMarketCodes(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '，' || r == '・' || r == ';' || r == '；' || r == '|'
	})
	seen := make(map[string]struct{}, len(parts))
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		code := strings.ToUpper(strings.TrimSpace(part))
		if code == "" {
			continue
		}
		if _, exists := seen[code]; exists {
			continue
		}
		seen[code] = struct{}{}
		result = append(result, code)
	}
	return result
}

func normalizeMarketWarrantyYearMonth(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}
	value = strings.NewReplacer("年", "-", "月", "", "/", "-", ".", "-").Replace(value)
	parts := strings.Split(value, "-")
	if len(parts) != 2 || len(parts[0]) != 4 {
		return "", errors.New("保証年月はYYYY-MMで指定してください")
	}
	year, yearErr := strconv.Atoi(parts[0])
	month, monthErr := strconv.Atoi(parts[1])
	if yearErr != nil || monthErr != nil || year < 1900 || year > 2200 || month < 1 || month > 12 {
		return "", errors.New("保証年月はYYYY-MMで指定してください")
	}
	return fmt.Sprintf("%04d-%02d", year, month), nil
}

func normalizeMarketCategory(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "", "domestic-auction", "国内オークション":
		return "domestic-auction", nil
	case "overseas", "海外":
		return "overseas", nil
	case "domestic-retail", "国内小売":
		return "domestic-retail", nil
	default:
		return "", errors.New("市場区分は国内オークション・海外・国内小売から指定してください")
	}
}

func normalizeMarketDuplicateText(value string) string {
	return strings.ToUpper(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func marketDuplicateKey(importDate, brand, model, reference, condition, accessoryText string, braceletQuantity *int, warrantyYearMonth, auctionCode string) string {
	accessories := splitMarketCodes(accessoryText)
	sort.Strings(accessories)
	if normalizedWarranty, err := normalizeMarketWarrantyYearMonth(warrantyYearMonth); err == nil {
		warrantyYearMonth = normalizedWarranty
	}
	return strings.Join([]string{
		normalizeMarketDuplicateText(importDate),
		normalizeMarketDuplicateText(brand),
		normalizeMarketDuplicateText(model),
		normalizeMarketDuplicateText(reference),
		normalizeMarketDuplicateText(condition),
		strings.Join(accessories, ","),
		func() string {
			if braceletQuantity == nil {
				return ""
			}
			return strconv.Itoa(*braceletQuantity)
		}(),
		normalizeMarketDuplicateText(warrantyYearMonth),
		normalizeMarketDuplicateText(auctionCode),
	}, "\x00")
}

func findMarketDuplicateCandidate(tx *gorm.DB, organizationID string, row MarketImportRowRecord, auctionHouseID string) (string, error) {
	type candidate struct {
		ID                string
		ImportDate        string
		BrandText         string
		ModelNumber       string
		ReferenceNumber   string
		ConditionText     string
		AccessoryText     string
		BraceletQuantity  *int
		WarrantyYearMonth string
		AuctionCode       string
	}
	var candidates []candidate
	result := tx.Table("market_price_records AS m").
		Select(`m.id,TO_CHAR(m.import_date,'YYYY-MM-DD') AS import_date,m.brand_text,m.model_number,m.reference_number,
			COALESCE(NULLIF(m.condition_text,''),c.code,'') AS condition_text,
			COALESCE(NULLIF(m.accessory_text,''),(SELECT STRING_AGG(a.code,',' ORDER BY a.code)
				FROM market_price_accessories ma JOIN accessories a ON a.id=ma.accessory_id
				WHERE ma.market_price_record_id=m.id),'') AS accessory_text,
			m.bracelet_quantity,m.warranty_year_month,COALESCE(ah.code,'') AS auction_code`).
		Joins("LEFT JOIN product_conditions c ON c.id=m.condition_id AND c.organization_id=m.organization_id").
		Joins("LEFT JOIN auction_houses ah ON ah.id=m.auction_house_id AND ah.organization_id=m.organization_id").
		Where(`m.organization_id=? AND m.import_date=? AND COALESCE(m.auction_house_id,'')=? AND m.is_active`,
			organizationID, row.ImportDate, auctionHouseID).
		Scan(&candidates)
	if result.Error != nil {
		return "", result.Error
	}
	want := marketDuplicateKey(row.ImportDate, row.BrandText, row.ModelNumber, row.ReferenceNumber,
		row.ConditionText, row.AccessoryText, row.BraceletQuantity, row.WarrantyYearMonth, row.AuctionCode)
	for _, item := range candidates {
		if marketDuplicateKey(item.ImportDate, item.BrandText, item.ModelNumber, item.ReferenceNumber,
			item.ConditionText, item.AccessoryText, item.BraceletQuantity, item.WarrantyYearMonth, item.AuctionCode) == want {
			return item.ID, nil
		}
	}
	return "", nil
}

func normalizeMarketHeaders(values []string) (map[string]int, error) {
	indexes := make(map[string]int, len(values))
	for index, raw := range values {
		value := strings.TrimSpace(strings.TrimPrefix(raw, "\ufeff"))
		canonical, ok := marketHeaderAliases[value]
		if !ok {
			return nil, fmt.Errorf("%w: unsupported column %q", ErrMarketImportHeader, value)
		}
		if _, duplicate := indexes[canonical]; duplicate {
			return nil, fmt.Errorf("%w: duplicate column %q", ErrMarketImportHeader, canonical)
		}
		indexes[canonical] = index
	}
	for _, required := range requiredMarketHeaders {
		if _, ok := indexes[required]; !ok {
			return nil, fmt.Errorf("%w: missing column %q", ErrMarketImportHeader, required)
		}
	}
	return indexes, nil
}

func parseImportAmount(raw string) (int64, error) {
	value := strings.TrimSpace(raw)
	value = strings.NewReplacer(",", "", "￥", "", "¥", "", "$", "").Replace(value)
	if value == "" {
		return 0, nil
	}
	amount, err := strconv.ParseInt(value, 10, 64)
	if err != nil || amount < 0 {
		return 0, errors.New("金額は0以上の半角整数で指定してください")
	}
	return amount, nil
}

func parseMarketBraceletQuantity(raw string) (*int, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, nil
	}
	quantity, err := strconv.Atoi(value)
	if err != nil || quantity < 0 {
		return nil, errors.New("コマ数は0以上の半角整数で指定してください")
	}
	return &quantity, nil
}

func csvValue(values []string, indexes map[string]int, key string) string {
	index, ok := indexes[key]
	if !ok || index >= len(values) {
		return ""
	}
	return strings.TrimSpace(values[index])
}

func (r *Repository) PreviewMarketCSV(ctx context.Context, organizationID, actorUserID, fileName string, reader io.Reader) (MarketImportBatchRecord, error) {
	csvReader := csv.NewReader(reader)
	csvReader.FieldsPerRecord = -1
	csvReader.TrimLeadingSpace = true
	header, err := csvReader.Read()
	if err != nil {
		return MarketImportBatchRecord{}, fmt.Errorf("%w: CSVヘッダーを読み取れません", ErrMarketImportHeader)
	}
	indexes, err := normalizeMarketHeaders(header)
	if err != nil {
		return MarketImportBatchRecord{}, err
	}
	batchID, err := database.NewID("mib")
	if err != nil {
		return MarketImportBatchRecord{}, err
	}
	now := time.Now().UTC()
	batch := MarketImportBatchRecord{
		ID: batchID, FileName: strings.TrimSpace(fileName), Status: "previewed", CreatedBy: actorUserID, CreatedAt: now,
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`INSERT INTO market_import_batches(
			id,organization_id,file_name,status,total_rows,valid_rows,error_rows,duplicate_rows,created_by,created_at
		) VALUES(?,?,?,'previewed',0,0,0,0,?,?)`, batchID, organizationID, batch.FileName, actorUserID, now).Error; err != nil {
			return err
		}
		seen := map[string]int{}
		for rowNumber := 2; rowNumber <= 5001; rowNumber++ {
			values, readErr := csvReader.Read()
			if errors.Is(readErr, io.EOF) {
				break
			}
			row := MarketImportRowRecord{RowNumber: rowNumber}
			var dateValue any
			var auctionHouseID string
			if readErr != nil {
				row.ErrorMessage = "CSV行を解析できません"
			} else if len(values) != len(header) {
				row.ErrorMessage = fmt.Sprintf("列数が不正です（%d列）", len(values))
			} else {
				row.ImportDate = csvValue(values, indexes, "import_date")
				row.Source, err = normalizeMarketCategory(csvValue(values, indexes, "market_category"))
				row.BrandText = csvValue(values, indexes, "brand_text")
				row.ModelNumber = csvValue(values, indexes, "model_number")
				row.ReferenceNumber = csvValue(values, indexes, "reference_number")
				row.ConditionText = csvValue(values, indexes, "condition_text")
				row.WarrantyYearMonth = csvValue(values, indexes, "warranty_year_month")
				row.SKU = csvValue(values, indexes, "sku")
				row.MaterialText = csvValue(values, indexes, "material_text")
				row.MovementText = csvValue(values, indexes, "movement_text")
				row.AccessoryText = csvValue(values, indexes, "accessory_text")
				braceletQuantity, braceletErr := parseMarketBraceletQuantity(csvValue(values, indexes, "bracelet_quantity"))
				row.BraceletQuantity = braceletQuantity
				row.AuctionCode = strings.ToUpper(csvValue(values, indexes, "auction_code"))
				row.PurchaseCurrency = strings.ToUpper(csvValue(values, indexes, "purchase_currency"))
				row.MarketCurrency = strings.ToUpper(csvValue(values, indexes, "market_currency"))
				row.Notes = csvValue(values, indexes, "notes")
				if row.PurchaseCurrency == "" {
					row.PurchaseCurrency = "JPY"
				}
				if row.MarketCurrency == "" {
					row.MarketCurrency = "JPY"
				}
				parsedDate, dateErr := time.Parse("2006-01-02", row.ImportDate)
				normalizedWarranty, warrantyErr := normalizeMarketWarrantyYearMonth(row.WarrantyYearMonth)
				row.WarrantyYearMonth = normalizedWarranty
				purchaseAmount, purchaseErr := parseImportAmount(csvValue(values, indexes, "purchase_price"))
				marketAmount, marketErr := parseImportAmount(csvValue(values, indexes, "market_price"))
				row.PurchasePriceMinor, row.MarketPriceMinor = purchaseAmount, marketAmount
				if rawRate := csvValue(values, indexes, "market_fx_rate"); rawRate != "" {
					row.MarketFXRateScaled, err = parseRate(rawRate)
					row.MarketFXScale = 100000000
				}
				switch {
				case err != nil:
					row.ErrorMessage = err.Error()
				case dateErr != nil:
					row.ErrorMessage = "市場調査日はYYYY-MM-DDで指定してください"
				case purchaseErr != nil:
					row.ErrorMessage = purchaseErr.Error()
				case marketErr != nil:
					row.ErrorMessage = marketErr.Error()
				case braceletErr != nil:
					row.ErrorMessage = braceletErr.Error()
				case warrantyErr != nil:
					row.ErrorMessage = warrantyErr.Error()
				case row.BrandText == "":
					row.ErrorMessage = "ブランドは必須です"
				case strings.TrimSpace(row.ModelNumber) == "":
					row.ErrorMessage = "モデル名は必須です"
				case row.PurchaseCurrency != "JPY" && row.PurchaseCurrency != "USD":
					row.ErrorMessage = "仕入通貨はJPYまたはUSDで指定してください"
				case row.MarketCurrency != "JPY" && row.MarketCurrency != "USD" && row.MarketCurrency != "HKD":
					row.ErrorMessage = "相場通貨はJPY・USD・HKDから指定してください"
				default:
					dateValue = parsedDate
					row.MarketFXRateScaled, row.MarketFXScale, err = resolveMarketFXRate(tx, organizationID,
						row.MarketCurrency, parsedDate, row.MarketFXRateScaled, row.MarketFXScale)
					if err != nil {
						row.ErrorMessage = "市場調査レートを取得できません"
					}
					if row.AuctionCode != "" {
						auctionHouseID, _, err = lookupCatalog(tx, "auction_houses", organizationID, row.AuctionCode, true)
						if err != nil {
							row.ErrorMessage = "オークションコードがマスタにありません"
						}
					}
				}
				if row.ErrorMessage == "" {
					duplicateID, duplicateErr := findMarketDuplicateCandidate(tx, organizationID, row, auctionHouseID)
					if duplicateErr != nil {
						return duplicateErr
					}
					key := marketDuplicateKey(row.ImportDate, row.BrandText, row.ModelNumber, row.ReferenceNumber,
						row.ConditionText, row.AccessoryText, row.BraceletQuantity, row.WarrantyYearMonth, row.AuctionCode)
					if duplicateID != "" {
						row.DuplicateCandidateID = duplicateID
						row.ErrorMessage = "市場調査日・ブランド・モデル・型番・状態・付属品・保証年月・オークションが同一の相場が登録済みです"
					} else if previous, ok := seen[key]; ok {
						row.ErrorMessage = fmt.Sprintf("CSV内の%d行目と重複しています", previous)
					} else {
						seen[key] = rowNumber
						row.Valid = true
					}
				}
			}
			rawJSON, _ := json.Marshal(values)
			rowID, _ := database.NewID("mir")
			row.ID = rowID
			if err := tx.Exec(`INSERT INTO market_import_rows(
				id,batch_id,row_number,import_date,brand_text,model_number,reference_number,condition_text,warranty_year_month,
				sku,material_text,movement_text,accessory_text,bracelet_quantity,auction_code,purchase_price_minor,purchase_currency,market_price_minor,market_currency,market_fx_rate_scaled,market_fx_scale,source,notes,
				raw_json,is_valid,error_message,duplicate_candidate_id
			) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,CAST(? AS JSONB),?,?,?)`, row.ID, batchID, row.RowNumber, dateValue,
				row.BrandText, row.ModelNumber, row.ReferenceNumber, row.ConditionText, row.WarrantyYearMonth, row.SKU, row.MaterialText, row.MovementText, row.AccessoryText, row.BraceletQuantity, row.AuctionCode, row.PurchasePriceMinor,
				row.PurchaseCurrency, row.MarketPriceMinor, row.MarketCurrency, row.MarketFXRateScaled, row.MarketFXScale, row.Source, row.Notes,
				string(rawJSON), row.Valid, row.ErrorMessage, nullIfEmpty(row.DuplicateCandidateID)).Error; err != nil {
				return err
			}
			batch.Rows = append(batch.Rows, row)
			batch.TotalRows++
			if row.Valid {
				batch.ValidRows++
			} else {
				batch.ErrorRows++
				if row.DuplicateCandidateID != "" {
					batch.DuplicateRows++
				}
			}
		}
		if batch.TotalRows == 5000 {
			if _, extraErr := csvReader.Read(); !errors.Is(extraErr, io.EOF) {
				return errors.New("CSVは5000データ行以内にしてください")
			}
		}
		if batch.TotalRows == 0 {
			return errors.New("CSVにデータ行がありません")
		}
		return tx.Exec(`UPDATE market_import_batches SET total_rows=?,valid_rows=?,error_rows=?,duplicate_rows=?
			WHERE id=? AND organization_id=?`, batch.TotalRows, batch.ValidRows, batch.ErrorRows,
			batch.DuplicateRows, batchID, organizationID).Error
	})
	if err != nil {
		return MarketImportBatchRecord{}, err
	}
	return batch, nil
}

func (r *Repository) MarketImportBatch(ctx context.Context, organizationID, batchID string) (MarketImportBatchRecord, error) {
	var batch MarketImportBatchRecord
	result := r.db.WithContext(ctx).Table("market_import_batches").
		Select("id,file_name,status,total_rows,valid_rows,error_rows,duplicate_rows,created_by,created_at,COALESCE(committed_by,'') AS committed_by,committed_at").
		Where("organization_id=? AND id=?", organizationID, batchID).Take(&batch)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return MarketImportBatchRecord{}, ErrMarketImportState
	}
	if result.Error != nil {
		return MarketImportBatchRecord{}, result.Error
	}
	if err := r.db.WithContext(ctx).Table("market_import_rows").
		Select(`id,row_number,COALESCE(TO_CHAR(import_date,'YYYY-MM-DD'),'') AS import_date,brand_text,
			model_number,reference_number,condition_text,warranty_year_month,sku,material_text,movement_text,accessory_text,bracelet_quantity,auction_code,
			purchase_price_minor,purchase_currency,market_price_minor,market_currency,market_fx_rate_scaled,market_fx_scale,source,notes,is_valid AS valid,
			error_message,COALESCE(duplicate_candidate_id,'') AS duplicate_candidate_id`).
		Where("batch_id=?", batchID).Order("row_number").Scan(&batch.Rows).Error; err != nil {
		return MarketImportBatchRecord{}, err
	}
	for index := range batch.Rows {
		batch.Rows[index].MarketFXRate = formatRateValue(batch.Rows[index].MarketFXRateScaled, batch.Rows[index].MarketFXScale)
	}
	return batch, nil
}

func (r *Repository) CommitMarketImport(ctx context.Context, organizationID, batchID, actorUserID string, requireApproval bool) (MarketImportBatchRecord, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var batch struct {
			Status    string
			ErrorRows int
		}
		result := tx.Raw(`SELECT status,error_rows FROM market_import_batches
			WHERE organization_id=? AND id=? FOR UPDATE`, organizationID, batchID).Scan(&batch)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrMarketImportState
		}
		if batch.Status == "committed" {
			return nil
		}
		if batch.Status != "previewed" && !(batch.Status == "pending_approval" && !requireApproval) {
			return ErrMarketImportState
		}
		if batch.ErrorRows > 0 {
			return ErrMarketImportRows
		}
		if requireApproval {
			now := time.Now().UTC()
			approvalID, _ := database.NewID("apr")
			actionID, _ := database.NewID("apa")
			snapshot, _ := json.Marshal(map[string]any{"batchId": batchID, "errorRows": batch.ErrorRows})
			if err := tx.Exec(`INSERT INTO approval_requests(
				id,organization_id,approval_type,target_type,target_id,requested_action,status,requested_by,
				requested_at,snapshot_json,created_at,updated_at
			) VALUES(?,?,'market_import','market_import',?,'market_import.commit','pending',?,?,CAST(? AS JSONB),?,?)`,
				approvalID, organizationID, batchID, actorUserID, now, string(snapshot), now, now).Error; err != nil {
				return err
			}
			if err := tx.Exec(`INSERT INTO approval_actions(id,approval_request_id,actor_user_id,action,note,created_at)
				VALUES(?,?,?,'requested','',?)`, actionID, approvalID, actorUserID, now).Error; err != nil {
				return err
			}
			if err := tx.Exec(`UPDATE market_import_batches SET status='pending_approval'
				WHERE organization_id=? AND id=?`, organizationID, batchID).Error; err != nil {
				return err
			}
			return insertNotificationTx(tx, organizationID, "", database.RoleAdmin, "approval.requested",
				"相場表CSVの承認申請が届きました", "作業者が相場表CSVの確定を申請しました。", "approval_request", approvalID, now)
		}
		var pendingApproval struct{ ID, RequestedBy string }
		if batch.Status == "pending_approval" {
			if err := tx.Raw(`SELECT id,requested_by FROM approval_requests
				WHERE organization_id=? AND target_type='market_import' AND target_id=?
				AND requested_action='market_import.commit' AND status='pending' FOR UPDATE`,
				organizationID, batchID).Scan(&pendingApproval).Error; err != nil {
				return err
			}
			if pendingApproval.ID == "" {
				return ErrMarketImportState
			}
			if pendingApproval.RequestedBy == actorUserID {
				return ErrApprovalSelf
			}
		}
		type row struct {
			ImportDate, BrandText, ModelNumber, ReferenceNumber, ConditionText      string
			WarrantyYearMonth                                                       string
			SKU, MaterialText, MovementText, AccessoryText, AuctionCode             string
			BraceletQuantity                                                        *int
			PurchaseCurrency, MarketCurrency, Source, Notes                         string
			PurchasePriceMinor, MarketPriceMinor, MarketFXRateScaled, MarketFXScale int64
		}
		var rows []row
		if err := tx.Table("market_import_rows").
			Select("TO_CHAR(import_date,'YYYY-MM-DD') AS import_date,brand_text,model_number,reference_number,condition_text,warranty_year_month,sku,material_text,movement_text,accessory_text,bracelet_quantity,auction_code,purchase_price_minor,purchase_currency,market_price_minor,market_currency,market_fx_rate_scaled,market_fx_scale,source,notes").
			Where("batch_id=? AND is_valid", batchID).Order("row_number").Scan(&rows).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		for _, item := range rows {
			var auctionHouseID string
			if strings.TrimSpace(item.AuctionCode) != "" {
				resolvedAuctionHouseID, _, lookupErr := lookupCatalog(tx, "auction_houses", organizationID, item.AuctionCode, true)
				if lookupErr != nil {
					return lookupErr
				}
				auctionHouseID = resolvedAuctionHouseID
			}
			id, _ := database.NewID("mkt")
			if err := tx.Exec(`INSERT INTO market_price_records(
				id,organization_id,import_date,brand_text,model_number,reference_number,sku,
				condition_text,warranty_year_month,material_text,movement_text,accessory_text,bracelet_quantity,auction_house_id,
				purchase_price_minor,purchase_currency,market_price_minor,market_currency,market_fx_rate_scaled,market_fx_scale,source,notes,
				import_batch_id,is_active,created_by,updated_by,created_at,updated_at
			) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,TRUE,?,?,?,?)`, id, organizationID, item.ImportDate,
				item.BrandText, item.ModelNumber, item.ReferenceNumber, item.SKU, item.ConditionText, item.WarrantyYearMonth, item.MaterialText, item.MovementText,
				item.AccessoryText, item.BraceletQuantity, nullIfEmpty(auctionHouseID), item.PurchasePriceMinor, item.PurchaseCurrency,
				item.MarketPriceMinor, item.MarketCurrency, item.MarketFXRateScaled, item.MarketFXScale, item.Source, item.Notes, batchID, actorUserID,
				actorUserID, now, now).Error; err != nil {
				return err
			}
		}
		if err := tx.Exec(`UPDATE market_import_batches SET status='committed',committed_by=?,committed_at=?
			WHERE organization_id=? AND id=?`, actorUserID, now, organizationID, batchID).Error; err != nil {
			return err
		}
		if pendingApproval.ID != "" {
			if err := tx.Exec(`UPDATE approval_requests SET status='approved',decided_by=?,decided_at=?,updated_at=?
				WHERE organization_id=? AND id=? AND status='pending'`, actorUserID, now, now, organizationID, pendingApproval.ID).Error; err != nil {
				return err
			}
			actionID, _ := database.NewID("apa")
			if err := tx.Exec(`INSERT INTO approval_actions(id,approval_request_id,actor_user_id,action,note,created_at)
				VALUES(?,?,?,'approved','',?)`, actionID, pendingApproval.ID, actorUserID, now).Error; err != nil {
				return err
			}
			if err := insertNotificationTx(tx, organizationID, pendingApproval.RequestedBy, "", "approval.approved",
				"相場表CSVが承認されました", "相場表へデータを反映しました。", "approval_request", pendingApproval.ID, now); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return MarketImportBatchRecord{}, err
	}
	return r.MarketImportBatch(ctx, organizationID, batchID)
}
