package persistence

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Repository is the GORM-backed read model used by the REST API. The legacy
// database.Store remains in place while write operations are migrated safely.
type Repository struct {
	db     *gorm.DB
	driver string
}

type Product struct {
	WeightGrams                       string                    `json:"weightGrams"`
	AccessorySupplements              AccessorySupplementValues `json:"accessorySupplements"`
	ID                                string                    `gorm:"column:id;primaryKey" json:"id"`
	OrganizationID                    string                    `gorm:"column:organization_id;index" json:"-"`
	ProductCode                       string                    `gorm:"column:product_code" json:"productCode"`
	SKU                               string                    `gorm:"column:sku" json:"sku"`
	Brand                             string                    `gorm:"column:brand" json:"brand"`
	ModelNumber                       string                    `gorm:"column:model_number" json:"modelNumber"`
	ReferenceNumber                   string                    `gorm:"column:reference_number" json:"referenceNumber"`
	SerialNumber                      string                    `gorm:"column:serial_number" json:"serialNumber"`
	ProductType                       string                    `gorm:"column:product_type" json:"productType"`
	ShapeID                           string                    `gorm:"column:shape_id" json:"shapeId"`
	MarkingID                         string                    `gorm:"column:marking_id" json:"markingId"`
	SupplierID                        string                    `gorm:"column:supplier_id" json:"-"`
	BrandID                           string                    `gorm:"column:brand_id" json:"brandId"`
	MaterialID                        string                    `gorm:"column:material_id" json:"materialId"`
	MovementID                        string                    `gorm:"column:movement_id" json:"movementId"`
	ConditionID                       string                    `gorm:"column:condition_id" json:"conditionId"`
	SupplierRoleID                    string                    `gorm:"column:supplier_role_id" json:"supplierRoleId"`
	PurchaseStaffID                   string                    `gorm:"column:purchase_staff_profile_id" json:"purchaseStaffProfileId"`
	PurchaseSlipLineID                string                    `gorm:"column:purchase_slip_line_id" json:"purchaseSlipLineId"`
	PurchaseDate                      DateString                `gorm:"column:purchase_date;type:date" json:"purchaseDate"`
	CostAmountMinor                   int64                     `gorm:"column:cost_amount_minor" json:"costAmountMinor"`
	CostCurrency                      string                    `gorm:"column:cost_currency" json:"costCurrency"`
	BaseSalePriceMinor                int64                     `gorm:"column:base_sale_price_minor" json:"baseSalePriceMinor"`
	BaseSaleCurrency                  string                    `gorm:"column:base_sale_currency" json:"baseSaleCurrency"`
	InventoryStatus                   string                    `gorm:"column:inventory_status" json:"inventoryStatus"`
	PublicationStatus                 string                    `gorm:"column:publication_status" json:"publicationStatus"`
	Condition                         string                    `gorm:"column:condition_text" json:"condition"`
	Accessories                       string                    `gorm:"column:accessories" json:"accessories"`
	BeltText                          string                    `gorm:"column:belt_text" json:"beltText"`
	DialText                          string                    `gorm:"column:dial_text" json:"dialText"`
	BraceletQuantity                  *int                      `gorm:"column:bracelet_quantity" json:"braceletQuantity,omitempty"`
	Notes                             string                    `gorm:"column:notes" json:"notes"`
	InternalComment                   string                    `gorm:"column:internal_comment" json:"internalComment"`
	CostAdjustmentID                  string                    `gorm:"column:cost_adjustment_id" json:"costAdjustmentId,omitempty"`
	CostAdjustmentStartedAt           *time.Time                `gorm:"column:cost_adjustment_started_at" json:"costAdjustmentStartedAt,omitempty"`
	InventoryValuationExcluded        bool                      `gorm:"column:inventory_valuation_excluded;not null;default:false" json:"inventoryValuationExcluded"`
	InventoryValuationExclusionReason string                    `gorm:"column:inventory_valuation_exclusion_reason;not null;default:''" json:"inventoryValuationExclusionReason,omitempty"`
	InventoryValuationExcludedAt      *time.Time                `gorm:"column:inventory_valuation_excluded_at" json:"inventoryValuationExcludedAt,omitempty"`
	DeletedAt                         *time.Time                `gorm:"column:deleted_at" json:"-"`
	CreatedAt                         time.Time                 `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt                         time.Time                 `gorm:"column:updated_at" json:"updatedAt"`
}

func (Product) TableName() string { return "products" }

type productRow struct {
	DashboardPurchaseUSDRate float64 `gorm:"column:dashboard_purchase_usd_rate" json:"dashboardPurchaseUsdRate"`
	DashboardSaleUSDRate     float64 `gorm:"column:dashboard_sale_usd_rate" json:"dashboardSaleUsdRate"`
	DashboardPurchaseJPY     int64   `gorm:"column:dashboard_purchase_jpy" json:"dashboardPurchaseJpy"`
	DashboardCostJPY         int64   `gorm:"column:dashboard_cost_jpy" json:"dashboardCostJpy"`
	PurchaseSlipID           string  `gorm:"column:purchase_slip_id" json:"purchaseSlipId"`
	SaleSlipID               string  `gorm:"column:sale_slip_id" json:"saleSlipId"`
	SaleBuyerCode            string  `gorm:"column:sale_buyer_code" json:"saleBuyerCode"`
	Product
	PurchaseTaxMode              string     `gorm:"column:purchase_tax_mode" json:"purchaseTaxMode"`
	SupplierName                 string     `gorm:"column:supplier_name" json:"supplierName"`
	ImageCount                   int        `gorm:"column:image_count" json:"imageCount"`
	FixedPurchaseCostJPYMinor    int64      `gorm:"column:fixed_purchase_cost_jpy_minor" json:"fixedPurchaseCostJpyMinor"`
	InventoryCostJPYMinor        int64      `gorm:"column:inventory_cost_jpy_minor" json:"inventoryCostJpyMinor"`
	PurchaseSourceAmountMinor    int64      `gorm:"column:purchase_source_amount_minor" json:"purchaseSourceAmountMinor"`
	PurchaseSourceCurrency       string     `gorm:"column:purchase_source_currency" json:"purchaseSourceCurrency"`
	PurchaseFXRateSnapshotID     string     `gorm:"column:purchase_fx_rate_snapshot_id" json:"purchaseFxRateSnapshotId"`
	PurchaseFXRateScaled         int64      `gorm:"column:purchase_fx_rate_scaled" json:"purchaseFxRateScaled"`
	PurchaseFXScale              int64      `gorm:"column:purchase_fx_scale" json:"purchaseFxScale"`
	PurchaseFXRateObservedAt     *time.Time `gorm:"column:purchase_fx_rate_observed_at" json:"purchaseFxRateObservedAt,omitempty"`
	HistoricalSourceCostJPYMinor int64      `gorm:"column:historical_source_cost_jpy_minor" json:"-"`
	HistoricalBreakdownSource    bool       `gorm:"column:historical_breakdown_source" json:"-"`
	SaleDate                     *time.Time `gorm:"column:sale_date" json:"saleDate,omitempty"`
	SaleCurrency                 string     `gorm:"column:sale_currency" json:"saleCurrency,omitempty"`
	SaleUnitPriceMinor           int64      `gorm:"column:sale_unit_price_minor" json:"saleUnitPriceMinor,omitempty"`
	SaleAdjustedPriceMinor       int64      `gorm:"column:sale_adjusted_price_minor" json:"saleAdjustedPriceMinor,omitempty"`
	SalePriceJPYMinor            int64      `gorm:"column:sale_price_jpy_minor" json:"salePriceJpyMinor,omitempty"`
	SaleFXRateScaled             int64      `gorm:"column:sale_fx_rate_scaled" json:"saleFxRateScaled,omitempty"`
	SaleFXScale                  int64      `gorm:"column:sale_fx_scale" json:"saleFxScale,omitempty"`
	SaleFXRateObservedAt         *time.Time `gorm:"column:sale_fx_rate_observed_at" json:"saleFxRateObservedAt,omitempty"`
}

type Dashboard struct {
	TotalProducts        int64                    `json:"totalProducts"`
	PurchasingProducts   int64                    `json:"purchasingProducts"`
	InStockProducts      int64                    `json:"inStockProducts"`
	ReservedProducts     int64                    `json:"reservedProducts"`
	ConfirmedSales       int64                    `json:"confirmedSales"`
	ConfirmedSalesJPY    int64                    `json:"confirmedSalesJpy"`
	ConfirmedSalesUSD    int64                    `json:"confirmedSalesUsd"`
	ConfirmedPurchases   int64                    `json:"confirmedPurchases"`
	PurchaseUnits        int64                    `json:"purchaseUnits"`
	ConfirmedPurchaseJPY int64                    `json:"confirmedPurchaseJpy"`
	ConfirmedPurchaseUSD int64                    `json:"confirmedPurchaseUsd"`
	PendingRequests      int64                    `json:"pendingRequests"`
	PendingApprovals     int64                    `json:"pendingApprovals"`
	Monthly              []DashboardMonth         `json:"monthly"`
	SupplierMonthly      []DashboardSupplierMonth `json:"supplierMonthly"`
}

type DashboardMonth struct {
	Month       string `json:"month"`
	SalesJPY    int64  `json:"salesJpy"`
	SalesUSD    int64  `json:"salesUsd"`
	PurchaseJPY int64  `json:"purchaseJpy"`
	PurchaseUSD int64  `json:"purchaseUsd"`
}

type DashboardSupplierMonth struct {
	Month        string `json:"month"`
	SupplierCode string `json:"supplierCode"`
	SupplierName string `json:"supplierName"`
	PurchaseJPY  int64  `json:"purchaseJpy"`
	PurchaseUSD  int64  `json:"purchaseUsd"`
	Units        int64  `json:"units"`
}

type ProductFilter struct {
	Query            string
	Status           string
	Sort             string
	Page             int
	PageSize         int
	IncludeCancelled bool
}

type ProductPage struct {
	Items      []productRow `json:"items"`
	Total      int64        `json:"total"`
	Page       int          `json:"page"`
	PageSize   int          `json:"pageSize"`
	TotalPages int          `json:"totalPages"`
}

func Open(cfg config.Config) (*Repository, error) {
	var dialector gorm.Dialector
	switch cfg.DatabaseDriver {
	case "postgres":
		dialector = postgres.Open(cfg.DatabaseURL)
	case "sqlite":
		dialector = sqlite.Open(cfg.DatabasePath)
	default:
		return nil, fmt.Errorf("unsupported database driver %q", cfg.DatabaseDriver)
	}
	level := logger.Silent
	if cfg.Environment == "development" {
		level = logger.Warn
	}
	db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(level)})
	if err != nil {
		return nil, fmt.Errorf("open gorm database: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("access gorm connection pool: %w", err)
	}
	if cfg.DatabaseDriver == "sqlite" {
		sqlDB.SetMaxOpenConns(1)
	} else {
		sqlDB.SetMaxOpenConns(20)
		// Retain one eight-read batch worth of connections to avoid reopening
		// remote TLS sessions between waves; the maximum remains twenty.
		sqlDB.SetMaxIdleConns(8)
		sqlDB.SetConnMaxLifetime(30 * time.Minute)
	}
	return &Repository{db: db, driver: cfg.DatabaseDriver}, nil
}

func (r *Repository) Close() error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func (r *Repository) Driver() string { return r.driver }

func (r *Repository) Ping(ctx context.Context) error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

// AutoMigrateCore creates only the core catalog tables for a new PostgreSQL
// environment. Full legacy-schema conversion remains an explicit cutover step.
func (r *Repository) AutoMigrateCore(ctx context.Context) error {
	if r.driver != "postgres" {
		return nil
	}
	return r.db.WithContext(ctx).AutoMigrate(&Product{})
}

func (r *Repository) Dashboard(ctx context.Context, organizationID string, months int) (Dashboard, error) {
	if months != 6 && months != 12 && months != 24 {
		months = 12
	}
	if r.driver == "postgres" {
		return r.dashboardPostgres(ctx, organizationID, months)
	}
	var result Dashboard
	base := r.db.WithContext(ctx).Model(&Product{}).
		Where("organization_id = ? AND deleted_at IS NULL", organizationID)
	// Legacy SQLite preview databases predate the PostgreSQL valuation split.
	// Their behavior is unchanged until the compatibility schema gains the flag.
	if r.db.Migrator().HasColumn(&Product{}, "inventory_valuation_excluded") {
		base = base.Where("inventory_valuation_excluded = ?", false)
	}
	if err := base.Count(&result.TotalProducts).Error; err != nil {
		return result, err
	}
	if err := base.Where("inventory_status = ?", "purchasing").Count(&result.PurchasingProducts).Error; err != nil {
		return result, err
	}
	if err := base.Where("inventory_status = ?", "in_stock").Count(&result.InStockProducts).Error; err != nil {
		return result, err
	}
	if err := r.db.WithContext(ctx).Table("sales_slips").
		Where("organization_id = ? AND status = ?", organizationID, "confirmed").
		Count(&result.ConfirmedSales).Error; err != nil {
		return result, err
	}
	var total struct{ Value int64 }
	if err := r.db.WithContext(ctx).Table("sales_slips AS slips").
		Select("COALESCE(SUM(lines.converted_total_jpy), 0) AS value").
		Joins("JOIN sales_lines AS lines ON lines.sales_slip_id = slips.id").
		Where("slips.organization_id = ? AND slips.status = ?", organizationID, "confirmed").
		Scan(&total).Error; err != nil {
		return result, err
	}
	result.ConfirmedSalesJPY = total.Value
	if err := r.db.WithContext(ctx).Table("sales_slips AS slips").
		Select("COALESCE(SUM(lines.unit_price_minor * lines.quantity), 0) AS value").
		Joins("JOIN sales_lines AS lines ON lines.sales_slip_id = slips.id").
		Where("slips.organization_id = ? AND slips.status = ? AND lines.sale_currency = ?", organizationID, "confirmed", "USD").
		Scan(&total).Error; err != nil {
		return result, err
	}
	result.ConfirmedSalesUSD = total.Value
	if err := r.db.WithContext(ctx).Table("purchase_requests").
		Where("organization_id = ? AND status = ?", organizationID, "pending").
		Count(&result.PendingRequests).Error; err != nil {
		return result, err
	}
	return result, nil
}

// These counters share the exact same eligible inventory set. Aggregate them
// together so a remote database needs one read instead of four serial reads.
func loadDashboardInventoryCounts(db *gorm.DB, organizationID string, result *Dashboard) error {
	var counts struct {
		TotalProducts, PurchasingProducts, InStockProducts, ReservedProducts int64
	}
	err := db.Raw(`SELECT
		COUNT(*) FILTER (WHERE inventory_status IN ('purchasing','in_stock','reserved','return_pending')) AS total_products,
		COUNT(*) FILTER (WHERE inventory_status='purchasing') AS purchasing_products,
		COUNT(*) FILTER (WHERE inventory_status='in_stock') AS in_stock_products,
		COUNT(*) FILTER (WHERE inventory_status='reserved') AS reserved_products
		FROM products
		WHERE organization_id=? AND deleted_at IS NULL AND inventory_valuation_excluded=FALSE
		AND NOT EXISTS (SELECT 1 FROM cost_adjustments ca WHERE ca.source_product_id=products.id
		AND ca.adjustment_type='breakdown' AND ca.status='confirmed')`, organizationID).Scan(&counts).Error
	if err != nil {
		return err
	}
	result.TotalProducts = counts.TotalProducts
	result.PurchasingProducts = counts.PurchasingProducts
	result.InStockProducts = counts.InStockProducts
	result.ReservedProducts = counts.ReservedProducts
	return nil
}

func (r *Repository) dashboardPostgres(ctx context.Context, organizationID string, months int) (Dashboard, error) {
	var result Dashboard
	db := r.db.WithContext(ctx)
	if err := loadDashboardInventoryCounts(db, organizationID, &result); err != nil {
		return result, err
	}

	jst := time.FixedZone("JST", 9*60*60)
	now := time.Now().In(jst)
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, jst)
	nextMonth := monthStart.AddDate(0, 1, 0)
	if err := db.Table("sales_slips").Where(
		"organization_id=? AND status='confirmed' AND sale_date>=? AND sale_date<?", organizationID, monthStart, nextMonth).
		Count(&result.ConfirmedSales).Error; err != nil {
		return result, err
	}
	var salesTotal struct{ Value int64 }
	if err := db.Table("sales_slips AS s").Select("COALESCE(SUM(l.converted_total_jpy),0) AS value").
		Joins("JOIN sales_lines l ON l.sales_slip_id=s.id").Where(
		"s.organization_id=? AND s.status='confirmed' AND s.sale_date>=? AND s.sale_date<?", organizationID, monthStart, nextMonth).
		Scan(&salesTotal).Error; err != nil {
		return result, err
	}
	result.ConfirmedSalesJPY = salesTotal.Value

	if err := db.Table("purchase_slips").Where(
		"organization_id=? AND status='confirmed' AND purchase_date>=? AND purchase_date<?", organizationID, monthStart, nextMonth).
		Count(&result.ConfirmedPurchases).Error; err != nil {
		return result, err
	}
	type currencyTotals struct{ JPY, USD, Units int64 }
	var purchases currencyTotals
	if err := db.Table("purchase_slips AS p").Select(`
		COALESCE(SUM(CASE WHEN l.cost_currency='JPY' THEN l.unit_cost_minor*l.quantity ELSE 0 END),0) AS jpy,
		COALESCE(SUM(CASE WHEN l.cost_currency='USD' THEN l.unit_cost_minor*l.quantity ELSE 0 END),0) AS usd,
		COALESCE(SUM(l.quantity),0) AS units`).Joins("JOIN purchase_slip_lines l ON l.purchase_slip_id=p.id").Where(
		"p.organization_id=? AND p.status='confirmed' AND p.purchase_date>=? AND p.purchase_date<?", organizationID, monthStart, nextMonth).
		Scan(&purchases).Error; err != nil {
		return result, err
	}
	result.PurchaseUnits = purchases.Units

	rate, rateErr := latestFX(db, organizationID, "USD")
	if rateErr == nil {
		result.ConfirmedSalesUSD, _ = convertCurrency(result.ConfirmedSalesJPY, "JPY", "USD", rate)
		purchaseUSDInJPY, _ := convertCurrency(purchases.USD, "USD", "JPY", rate)
		purchaseJPYInUSD, _ := convertCurrency(purchases.JPY, "JPY", "USD", rate)
		result.ConfirmedPurchaseJPY = purchases.JPY + purchaseUSDInJPY
		result.ConfirmedPurchaseUSD = purchases.USD + purchaseJPYInUSD
	} else {
		result.ConfirmedPurchaseJPY = purchases.JPY
		result.ConfirmedPurchaseUSD = purchases.USD
	}
	if err := db.Table("purchase_requests").Where("organization_id=? AND status='pending'", organizationID).
		Count(&result.PendingRequests).Error; err != nil {
		return result, err
	}
	if err := db.Table("approval_requests").Where("organization_id=? AND status='pending'", organizationID).
		Count(&result.PendingApprovals).Error; err != nil {
		return result, err
	}

	type monthRow struct {
		Month    string
		JPY, USD int64
	}
	var salesRows []monthRow
	seriesStart := monthStart.AddDate(0, -(months - 1), 0)
	if err := db.Table("sales_slips AS s").Select(`TO_CHAR(s.sale_date,'YYYY-MM') AS month,
		COALESCE(SUM(l.converted_total_jpy),0) AS jpy,0 AS usd`).
		Joins("JOIN sales_lines l ON l.sales_slip_id=s.id").Where(
		"s.organization_id=? AND s.status='confirmed' AND s.sale_date>=? AND s.sale_date<?", organizationID, seriesStart, nextMonth).
		Group("TO_CHAR(s.sale_date,'YYYY-MM')").Scan(&salesRows).Error; err != nil {
		return result, err
	}
	var purchaseRows []monthRow
	if err := db.Table("purchase_slips AS p").Select(`TO_CHAR(p.purchase_date,'YYYY-MM') AS month,
		COALESCE(SUM(CASE WHEN l.cost_currency='JPY' THEN l.unit_cost_minor*l.quantity ELSE 0 END),0) AS jpy,
		COALESCE(SUM(CASE WHEN l.cost_currency='USD' THEN l.unit_cost_minor*l.quantity ELSE 0 END),0) AS usd`).
		Joins("JOIN purchase_slip_lines l ON l.purchase_slip_id=p.id").Where(
		"p.organization_id=? AND p.status='confirmed' AND p.purchase_date>=? AND p.purchase_date<?", organizationID, seriesStart, nextMonth).
		Group("TO_CHAR(p.purchase_date,'YYYY-MM')").Scan(&purchaseRows).Error; err != nil {
		return result, err
	}
	type supplierMonthRow struct {
		Month        string
		SupplierCode string
		SupplierName string
		JPY          int64
		USD          int64
		Units        int64
	}
	var supplierRows []supplierMonthRow
	if err := db.Table("purchase_slips AS p").Select(`TO_CHAR(p.purchase_date,'YYYY-MM') AS month,
		COALESCE(pr.role_code,'') AS supplier_code,COALESCE(NULLIF(p.supplier_name_text,''),bp.legal_name,'未設定') AS supplier_name,
		COALESCE(SUM(CASE WHEN l.cost_currency='JPY' THEN l.unit_cost_minor*l.quantity ELSE 0 END),0) AS jpy,
		COALESCE(SUM(CASE WHEN l.cost_currency='USD' THEN l.unit_cost_minor*l.quantity ELSE 0 END),0) AS usd,
		COALESCE(SUM(l.quantity),0) AS units`).
		Joins("JOIN purchase_slip_lines l ON l.purchase_slip_id=p.id").
		Joins("LEFT JOIN partner_roles pr ON pr.id=p.supplier_role_id AND pr.organization_id=p.organization_id").
		Joins("LEFT JOIN business_partners bp ON bp.id=pr.partner_id AND bp.organization_id=p.organization_id").
		Where("p.organization_id=? AND p.status='confirmed' AND p.purchase_date>=? AND p.purchase_date<?", organizationID, seriesStart, nextMonth).
		Group("TO_CHAR(p.purchase_date,'YYYY-MM'),pr.role_code,p.supplier_name_text,bp.legal_name").
		Order("month,supplier_code").Scan(&supplierRows).Error; err != nil {
		return result, err
	}
	salesByMonth, purchasesByMonth := map[string]monthRow{}, map[string]monthRow{}
	for _, row := range salesRows {
		salesByMonth[row.Month] = row
	}
	for _, row := range purchaseRows {
		purchasesByMonth[row.Month] = row
	}
	for offset := 0; offset < months; offset++ {
		month := seriesStart.AddDate(0, offset, 0).Format("2006-01")
		sale := salesByMonth[month]
		purchase := purchasesByMonth[month]
		item := DashboardMonth{Month: month, SalesJPY: sale.JPY}
		if rateErr == nil {
			item.SalesUSD, _ = convertCurrency(item.SalesJPY, "JPY", "USD", rate)
			usdInJPY, _ := convertCurrency(purchase.USD, "USD", "JPY", rate)
			jpyInUSD, _ := convertCurrency(purchase.JPY, "JPY", "USD", rate)
			item.PurchaseJPY = purchase.JPY + usdInJPY
			item.PurchaseUSD = purchase.USD + jpyInUSD
		} else {
			item.PurchaseJPY, item.PurchaseUSD = purchase.JPY, purchase.USD
		}
		result.Monthly = append(result.Monthly, item)
	}
	for _, row := range supplierRows {
		item := DashboardSupplierMonth{Month: row.Month, SupplierCode: row.SupplierCode, SupplierName: row.SupplierName, Units: row.Units}
		if rateErr == nil {
			usdInJPY, _ := convertCurrency(row.USD, "USD", "JPY", rate)
			jpyInUSD, _ := convertCurrency(row.JPY, "JPY", "USD", rate)
			item.PurchaseJPY = row.JPY + usdInJPY
			item.PurchaseUSD = row.USD + jpyInUSD
		} else {
			item.PurchaseJPY, item.PurchaseUSD = row.JPY, row.USD
		}
		result.SupplierMonthly = append(result.SupplierMonthly, item)
	}
	return result, nil
}

func (r *Repository) Products(ctx context.Context, organizationID string, filter ProductFilter) (ProductPage, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 || filter.PageSize > 100 {
		filter.PageSize = 20
	}
	query := r.db.WithContext(ctx).Table("products AS p").
		Where("p.organization_id = ? AND p.deleted_at IS NULL", organizationID)
	if !filter.IncludeCancelled {
		query = query.Where("p.inventory_status <> ?", "cancelled")
	}
	if filter.Status != "" {
		query = query.Where("p.inventory_status = ?", filter.Status)
	}
	if value := strings.TrimSpace(filter.Query); value != "" {
		like := "%" + value + "%"
		query = query.Where("p.product_code LIKE ? OR p.sku LIKE ? OR p.brand LIKE ? OR p.model_number LIKE ? OR p.serial_number LIKE ?", like, like, like, like, like)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return ProductPage{}, err
	}
	orders := map[string]string{
		"purchase_asc":    "p.purchase_date ASC, p.product_code ASC",
		"code_asc":        "p.product_code ASC",
		"code_desc":       "p.product_code DESC",
		"brand_asc":       "p.brand ASC, p.model_number ASC, p.product_code ASC",
		"sale_price_desc": "p.base_sale_price_minor DESC, p.product_code ASC",
	}
	order := orders[filter.Sort]
	if order == "" {
		order = "p.purchase_date DESC, p.product_code DESC"
	}
	var items []productRow
	if r.driver == "postgres" {
		query = query.
			Select(`p.*, COALESCE(NULLIF(ps.supplier_name_text,''),bp.legal_name,'') AS supplier_name,
				COALESCE(ps.purchase_tax_mode,'domestic') AS purchase_tax_mode,
 COALESCE(
 CASE WHEN (psl.cost_currency='USD' OR (fx.base_currency='USD' AND fx.quote_currency='JPY')) AND psl.fx_rate_scaled>0 AND psl.fx_scale>0
 THEN psl.fx_rate_scaled::numeric/psl.fx_scale END, purchase_usd.rate, 0) AS dashboard_purchase_usd_rate,
 COALESCE(
 CASE WHEN (sale.sale_currency='USD' OR (sale.fx_base_currency='USD' AND sale.fx_quote_currency='JPY')) AND sale.fx_rate_scaled>0 AND sale.fx_scale>0
 THEN sale.fx_rate_scaled::numeric/sale.fx_scale END, sale_usd.rate, 0) AS dashboard_sale_usd_rate,
 COALESCE(ps.id,'') AS purchase_slip_id,COALESCE(sale.sale_slip_id,'') AS sale_slip_id,COALESCE(sale.buyer_code,'') AS sale_buyer_code,
                CASE WHEN COALESCE(psl.cost_currency,p.cost_currency)='JPY' THEN COALESCE(psl.unit_cost_minor,p.cost_amount_minor)
                     ELSE COALESCE(ROUND(psl.unit_cost_minor*psl.fx_rate_scaled::numeric/NULLIF(psl.fx_scale,0)),0)::bigint END AS dashboard_purchase_jpy,
                CASE WHEN p.cost_currency='JPY' THEN p.cost_amount_minor
                     ELSE COALESCE(ROUND(p.cost_amount_minor*psl.fx_rate_scaled::numeric/NULLIF(psl.fx_scale,0)),0)::bigint END AS dashboard_cost_jpy,
				CASE WHEN ca.adjustment_type='breakdown' AND ca.source_product_id=p.id AND ca.status='confirmed' THEN ca.source_cost_jpy_minor
				     ELSE COALESCE(psl.converted_total_jpy,
					CASE WHEN p.cost_currency='JPY' THEN p.cost_amount_minor ELSE 0 END) END AS fixed_purchase_cost_jpy_minor,
				CASE WHEN p.cost_currency='JPY' THEN p.cost_amount_minor
				     ELSE COALESCE(psl.converted_total_jpy,0) END AS inventory_cost_jpy_minor,
				COALESCE(ca.source_cost_jpy_minor,0) AS historical_source_cost_jpy_minor,
				CASE WHEN ca.adjustment_type='breakdown' AND ca.source_product_id=p.id AND ca.status='confirmed' THEN TRUE ELSE FALSE END AS historical_breakdown_source,
				COALESCE(psl.unit_cost_minor,p.cost_amount_minor) AS purchase_source_amount_minor,
				COALESCE(psl.cost_currency,p.cost_currency) AS purchase_source_currency,
				COALESCE(psl.fx_rate_snapshot_id,'') AS purchase_fx_rate_snapshot_id,
				COALESCE(psl.fx_rate_scaled,0) AS purchase_fx_rate_scaled,
				COALESCE(psl.fx_scale,0) AS purchase_fx_scale,
				fx.observed_at AS purchase_fx_rate_observed_at,
				sale.sale_date,sale.sale_currency,sale.unit_price_minor AS sale_unit_price_minor,
				GREATEST(0,sale.unit_price_minor + COALESCE(sadj.amount_minor,0)) AS sale_adjusted_price_minor,
				GREATEST(0,CASE WHEN sale.sale_currency='JPY' THEN sale.unit_price_minor ELSE ROUND(sale.unit_price_minor*sale.fx_rate_scaled::numeric/sale.fx_scale)::bigint END + COALESCE(sadj.amount_jpy,0)) AS sale_price_jpy_minor,
				COALESCE(sale.fx_rate_scaled,0) AS sale_fx_rate_scaled,COALESCE(sale.fx_scale,0) AS sale_fx_scale,
				sale.fx_rate_observed_at AS sale_fx_rate_observed_at,
				(SELECT COUNT(*) FROM product_files i WHERE i.organization_id = p.organization_id AND i.product_id = p.id) AS image_count`).
			Joins("LEFT JOIN partner_roles pr ON pr.id = p.supplier_role_id AND pr.organization_id = p.organization_id").
			Joins("LEFT JOIN business_partners bp ON bp.id = pr.partner_id AND bp.organization_id = p.organization_id").
			Joins("LEFT JOIN purchase_slip_lines psl ON psl.id = p.purchase_slip_line_id").
			Joins("LEFT JOIN purchase_slips ps ON ps.id = psl.purchase_slip_id AND ps.organization_id = p.organization_id").
			Joins("LEFT JOIN cost_adjustments ca ON ca.id = p.cost_adjustment_id AND ca.organization_id = p.organization_id").
			Joins("LEFT JOIN exchange_rate_snapshots fx ON fx.id = psl.fx_rate_snapshot_id").
			Joins(`LEFT JOIN LATERAL (SELECT s.sale_date,s.id AS sale_slip_id,br.role_code AS buyer_code,l.id AS sale_line_id,l.unit_price_minor,l.sale_currency,l.fx_rate_scaled,l.fx_scale,sfx.observed_at AS fx_rate_observed_at,sfx.base_currency AS fx_base_currency,sfx.quote_currency AS fx_quote_currency
				FROM sales_lines l JOIN sales_slips s ON s.id=l.sales_slip_id
 LEFT JOIN partner_roles br ON br.id=s.buyer_role_id AND br.organization_id=s.organization_id
				LEFT JOIN exchange_rate_snapshots sfx ON sfx.id=l.fx_rate_snapshot_id
				WHERE l.product_id=p.id AND s.organization_id=p.organization_id AND s.status='confirmed'
				ORDER BY s.confirmed_at DESC NULLS LAST,s.created_at DESC LIMIT 1) sale ON TRUE`).
			// Resolve non-USD transactions against all historical rates, not the UI's latest 100.
			Joins(`LEFT JOIN LATERAL (SELECT r.rate_scaled::numeric/r.scale AS rate
 FROM exchange_rate_snapshots r WHERE r.organization_id=p.organization_id
 AND r.base_currency='USD' AND r.quote_currency='JPY' AND r.rate_scaled>0 AND r.scale>0
 AND r.observed_at < ((p.purchase_date + INTERVAL '1 day') AT TIME ZONE 'Asia/Tokyo')
 ORDER BY r.observed_at DESC,r.created_at DESC,r.id DESC LIMIT 1) purchase_usd ON TRUE`).
			Joins(`LEFT JOIN LATERAL (SELECT r.rate_scaled::numeric/r.scale AS rate
 FROM exchange_rate_snapshots r WHERE r.organization_id=p.organization_id
 AND r.base_currency='USD' AND r.quote_currency='JPY' AND r.rate_scaled>0 AND r.scale>0
 AND r.observed_at < ((sale.sale_date + INTERVAL '1 day') AT TIME ZONE 'Asia/Tokyo')
 ORDER BY r.observed_at DESC,r.created_at DESC,r.id DESC LIMIT 1) sale_usd ON TRUE`).
			Joins(`LEFT JOIN LATERAL (SELECT
				COALESCE(SUM(CASE WHEN COALESCE(NULLIF(al.direction,''),a.direction)='decrease' THEN -al.adjustment_amount_minor ELSE al.adjustment_amount_minor END),0) AS amount_minor,
				COALESCE(SUM(CASE WHEN COALESCE(NULLIF(al.direction,''),a.direction)='decrease' THEN -al.converted_adjustment_jpy ELSE al.converted_adjustment_jpy END),0) AS amount_jpy
				FROM sales_adjustment_lines al JOIN sales_adjustment_slips a ON a.id=al.sales_adjustment_slip_id
				WHERE al.source_sales_line_id=sale.sale_line_id AND a.status='confirmed') sadj ON TRUE`)
	} else {
		query = query.
			Select(`p.*, s.name AS supplier_name,
				(SELECT COUNT(*) FROM product_images i WHERE i.organization_id = p.organization_id AND i.product_id = p.id) AS image_count`).
			Joins("JOIN suppliers s ON s.id = p.supplier_id")
	}
	err := query.
		Order(order).
		Limit(filter.PageSize).
		Offset((filter.Page - 1) * filter.PageSize).
		Scan(&items).Error
	if err != nil {
		return ProductPage{}, err
	}
	for index := range items {
		if items[index].HistoricalBreakdownSource {
			items[index].InventoryValuationExcluded = true
			if items[index].InventoryValuationExclusionReason == "" {
				items[index].InventoryValuationExclusionReason = "cost_split_source"
			}
			if items[index].FixedPurchaseCostJPYMinor == 0 {
				items[index].FixedPurchaseCostJPYMinor = items[index].HistoricalSourceCostJPYMinor
			}
		}
		if items[index].FixedPurchaseCostJPYMinor == 0 && items[index].PurchaseSlipLineID == "" && items[index].CostCurrency == "JPY" {
			items[index].FixedPurchaseCostJPYMinor = items[index].CostAmountMinor
		}
		if items[index].InventoryCostJPYMinor == 0 && items[index].CostAmountMinor > 0 {
			items[index].InventoryCostJPYMinor = items[index].FixedPurchaseCostJPYMinor
		}
		if items[index].PurchaseSourceCurrency == "" {
			items[index].PurchaseSourceCurrency = items[index].CostCurrency
			items[index].PurchaseSourceAmountMinor = items[index].CostAmountMinor
		}
	}
	totalPages := int((total + int64(filter.PageSize) - 1) / int64(filter.PageSize))
	if totalPages == 0 {
		totalPages = 1
	}
	return ProductPage{Items: items, Total: total, Page: filter.Page, PageSize: filter.PageSize, TotalPages: totalPages}, nil
}
