package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	ErrPurchaseAlreadyConfirmed = errors.New("purchase slip is already confirmed")
	ErrDailyProductLimit        = errors.New("同一仕入日の商品コードが999件に達しました")
	ErrSerialReasonRequired     = errors.New("重複シリアルを登録する理由が必要です")
	ErrInvalidStatusTransition  = errors.New("許可されていない在庫状態の変更です")
)

const previewSalePriceJPYPerUSD int64 = 155

type Supplier struct {
	ID   string
	Code string
	Name string
}

type PurchaseLineInput struct {
	Quantity      int
	UnitCostMinor int64
	Currency      string
	Brand         string
	ModelNumber   string
	ProductType   string
}

type CreatePurchaseInput struct {
	OrganizationID string
	SupplierID     string
	PurchaseDate   string
	Notes          string
	CreatedBy      string
	Lines          []PurchaseLineInput
}

type PurchaseSlip struct {
	ID             string
	OrganizationID string
	SlipNumber     string
	SupplierID     string
	SupplierName   string
	PurchaseDate   string
	Status         string
	IsSimple       bool
	Notes          string
	LineCount      int
	TotalMinor     int64
	Currency       string
	CreatedAt      time.Time
	ConfirmedAt    *time.Time
	Lines          []PurchaseLine
}

type PurchaseLine struct {
	ID                    string
	LineNumber            int
	Quantity              int
	UnitCostMinor         int64
	Currency              string
	Brand                 string
	ModelNumber           string
	ProductType           string
	GeneratedProductCount int
}

type ConfirmResult struct {
	Products         []Product
	AlreadyConfirmed bool
}

type SingleProductInput struct {
	OrganizationID     string
	SupplierID         string
	PurchaseDate       string
	SKU                string
	Brand              string
	ModelNumber        string
	SerialNumber       string
	ProductType        string
	CostAmountMinor    int64
	CostCurrency       string
	BaseSalePriceMinor int64
	BaseSaleCurrency   string
	Condition          string
	Accessories        string
	DuplicateReason    string
	CreatedBy          string
}

type Product struct {
	ID                     string
	OrganizationID         string
	ProductCode            string
	SKU                    string
	Brand                  string
	ModelNumber            string
	SerialNumber           string
	ProductType            string
	SupplierID             string
	SupplierName           string
	PurchaseDate           string
	CostAmountMinor        int64
	CostCurrency           string
	BaseSalePriceMinor     int64
	BaseSaleCurrency       string
	ReferencePriceMinor    int64
	ReferenceCurrency      string
	ExchangeRateScaled     int64
	ExchangeRateObservedAt time.Time
	GrossProfitMinor       int64
	GrossProfitCurrency    string
	MarginBasisPoints      int64
	RateAvailable          bool
	InventoryStatus        string
	PublicationStatus      string
	Condition              string
	Accessories            string
	CreatedAt              time.Time
	ImageCount             int
	Images                 []ProductImage
	Events                 []InventoryEvent
}

type ProductImage struct {
	ID           string
	ProductID    string
	StoragePath  string
	OriginalName string
	ContentType  string
	SizeBytes    int64
	SortOrder    int
}

type InventoryEvent struct {
	EventType  string
	FromStatus string
	ToStatus   string
	Reason     string
	ActorName  string
	CreatedAt  time.Time
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
	Products   []Product
	Total      int
	Page       int
	PageSize   int
	TotalPages int
}

type InventoryStats struct {
	Total      int
	Purchasing int
	InStock    int
}

type SerialDuplicateError struct {
	Candidates []Product
}

func (e *SerialDuplicateError) Error() string { return ErrSerialReasonRequired.Error() }

func (s *Store) SeedInventoryPreview(ctx context.Context) error {
	now := s.now().Format(time.RFC3339Nano)
	for _, supplier := range []Supplier{
		{ID: "sup_001", Code: "S001", Name: "田中商事"},
		{ID: "sup_002", Code: "S002", Name: "山田時計店"},
		{ID: "sup_003", Code: "S003", Name: "ゴールデンウォッチ"},
	} {
		if _, err := s.db.ExecContext(ctx, `
			INSERT OR IGNORE INTO suppliers(id,organization_id,supplier_code,name,created_at,updated_at)
			VALUES(?,'org_preview',?,?,?,?)`, supplier.ID, supplier.Code, supplier.Name, now, now); err != nil {
			return err
		}
	}
	if err := s.migratePreviewSalePricesToUSD(ctx, now); err != nil {
		return err
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM products WHERE organization_id='org_preview'`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	_, err := s.CreateSingleProduct(ctx, SingleProductInput{
		OrganizationID: "org_preview", SupplierID: "sup_001", PurchaseDate: "2026-07-26",
		SKU: "ROLEX-SUB-001", Brand: "ロレックス", ModelNumber: "116610LN",
		SerialNumber: "ZX123456", ProductType: "腕時計", CostAmountMinor: 850000, CostCurrency: "JPY",
		BaseSalePriceMinor: 7613, BaseSaleCurrency: "USD", Condition: "極美品 (S)",
		Accessories: "BOX, GUARANTEE", CreatedBy: "usr_admin",
	})
	return err
}

// migratePreviewSalePricesToUSD upgrades local preview rows created before the
// USD policy. The fixed rate matches the reference admin's manually managed
// preview rate and intentionally does not affect non-preview organizations.
func (s *Store) migratePreviewSalePricesToUSD(ctx context.Context, updatedAt string) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id,base_sale_price_minor FROM products
		WHERE organization_id='org_preview' AND base_sale_currency='JPY'`)
	if err != nil {
		return err
	}
	type legacySalePrice struct {
		id    string
		price int64
	}
	var legacy []legacySalePrice
	for rows.Next() {
		var item legacySalePrice
		if err := rows.Scan(&item.id, &item.price); err != nil {
			rows.Close()
			return err
		}
		legacy = append(legacy, item)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, item := range legacy {
		usd := (item.price + previewSalePriceJPYPerUSD/2) / previewSalePriceJPYPerUSD
		if _, err := s.db.ExecContext(ctx, `
			UPDATE products SET base_sale_price_minor=?,base_sale_currency='USD',updated_at=?
			WHERE id=?`, usd, updatedAt, item.id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Suppliers(ctx context.Context, organizationID string) ([]Supplier, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id,supplier_code,name FROM suppliers
		WHERE organization_id=? AND is_active=1 ORDER BY supplier_code`, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var suppliers []Supplier
	for rows.Next() {
		var supplier Supplier
		if err := rows.Scan(&supplier.ID, &supplier.Code, &supplier.Name); err != nil {
			return nil, err
		}
		suppliers = append(suppliers, supplier)
	}
	return suppliers, rows.Err()
}

func (s *Store) CreatePurchaseDraft(ctx context.Context, input CreatePurchaseInput) (PurchaseSlip, error) {
	if err := validatePurchaseInput(input); err != nil {
		return PurchaseSlip{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PurchaseSlip{}, err
	}
	defer tx.Rollback()
	slipID, _ := NewID("pur")
	slipNumber, err := nextSlipNumberTx(ctx, tx, input.OrganizationID, input.PurchaseDate)
	if err != nil {
		return PurchaseSlip{}, err
	}
	now := s.now().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO purchase_slips(
			id,organization_id,slip_number,supplier_id,purchase_date,status,notes,created_by,created_at,updated_at
		) VALUES(?,?,?,?,?,'draft',?,?,?,?)`,
		slipID, input.OrganizationID, slipNumber, input.SupplierID, input.PurchaseDate,
		strings.TrimSpace(input.Notes), input.CreatedBy, now, now); err != nil {
		return PurchaseSlip{}, err
	}
	for index, line := range input.Lines {
		lineID, _ := NewID("pul")
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO purchase_slip_lines(
				id,purchase_slip_id,line_number,quantity,unit_cost_minor,currency,brand,model_number,product_type,created_at
			) VALUES(?,?,?,?,?,?,?,?,?,?)`,
			lineID, slipID, index+1, line.Quantity, line.UnitCostMinor, line.Currency,
			strings.TrimSpace(line.Brand), strings.TrimSpace(line.ModelNumber), strings.TrimSpace(line.ProductType), now); err != nil {
			return PurchaseSlip{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return PurchaseSlip{}, err
	}
	return s.Purchase(ctx, input.OrganizationID, slipID)
}

func validatePurchaseInput(input CreatePurchaseInput) error {
	if input.OrganizationID == "" || input.SupplierID == "" || input.CreatedBy == "" {
		return errors.New("仕入先と操作者は必須です")
	}
	if _, err := time.Parse("2006-01-02", input.PurchaseDate); err != nil {
		return errors.New("仕入日を正しく入力してください")
	}
	if len(input.Lines) == 0 {
		return errors.New("仕入明細を1件以上入力してください")
	}
	currency := input.Lines[0].Currency
	for _, line := range input.Lines {
		if line.Quantity < 1 || line.UnitCostMinor < 0 || strings.TrimSpace(line.Brand) == "" || strings.TrimSpace(line.ProductType) == "" {
			return errors.New("明細の数量、ブランド、商品種別を確認してください")
		}
		if line.Currency != "JPY" && line.Currency != "USD" {
			return errors.New("通貨はJPYまたはUSDを選択してください")
		}
		if line.Currency != currency {
			return errors.New("1つの仕入伝票では明細通貨を統一してください")
		}
	}
	return nil
}

func (s *Store) InventoryStats(ctx context.Context, organizationID string) (InventoryStats, error) {
	var stats InventoryStats
	err := s.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN inventory_status NOT IN ('cancelled','invalid') THEN 1 ELSE 0 END),0),
			COALESCE(SUM(CASE WHEN inventory_status='purchasing' THEN 1 ELSE 0 END),0),
			COALESCE(SUM(CASE WHEN inventory_status='in_stock' THEN 1 ELSE 0 END),0)
		FROM products WHERE organization_id=? AND deleted_at IS NULL`, organizationID).
		Scan(&stats.Total, &stats.Purchasing, &stats.InStock)
	return stats, err
}

func nextSlipNumberTx(ctx context.Context, tx *sql.Tx, organizationID, purchaseDate string) (string, error) {
	year := purchaseDate[:4]
	var count int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM purchase_slips WHERE organization_id=? AND substr(purchase_date,1,4)=?`,
		organizationID, year).Scan(&count); err != nil {
		return "", err
	}
	return fmt.Sprintf("PI-%s-%04d", year, count+1), nil
}

func (s *Store) ConfirmPurchase(ctx context.Context, organizationID, slipID, actorUserID string) (ConfirmResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ConfirmResult{}, err
	}
	defer tx.Rollback()
	var status, purchaseDate, supplierID string
	if err := tx.QueryRowContext(ctx, `
		SELECT status,purchase_date,supplier_id FROM purchase_slips
		WHERE id=? AND organization_id=?`, slipID, organizationID).Scan(&status, &purchaseDate, &supplierID); err != nil {
		return ConfirmResult{}, err
	}
	if status == "confirmed" {
		products, err := productsForPurchaseTx(ctx, tx, organizationID, slipID)
		return ConfirmResult{Products: products, AlreadyConfirmed: true}, err
	}
	if status != "draft" {
		return ConfirmResult{}, errors.New("下書きの仕入伝票だけ確定できます")
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id,quantity,unit_cost_minor,currency,brand,model_number,product_type
		FROM purchase_slip_lines WHERE purchase_slip_id=? ORDER BY line_number`, slipID)
	if err != nil {
		return ConfirmResult{}, err
	}
	var lines []PurchaseLine
	for rows.Next() {
		var line PurchaseLine
		if err := rows.Scan(&line.ID, &line.Quantity, &line.UnitCostMinor, &line.Currency, &line.Brand, &line.ModelNumber, &line.ProductType); err != nil {
			rows.Close()
			return ConfirmResult{}, err
		}
		lines = append(lines, line)
	}
	rows.Close()

	now := s.now().Format(time.RFC3339Nano)
	var products []Product
	for _, line := range lines {
		for index := 0; index < line.Quantity; index++ {
			code, err := nextProductCodeTx(ctx, tx, organizationID, purchaseDate)
			if err != nil {
				return ConfirmResult{}, err
			}
			productID, _ := NewID("prd")
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO products(
					id,organization_id,product_code,brand,model_number,product_type,purchase_slip_line_id,
					supplier_id,purchase_date,cost_amount_minor,cost_currency,inventory_status,created_at,updated_at
				) VALUES(?,?,?,?,?,?,?,?,?,?,?,'purchasing',?,?)`,
				productID, organizationID, code, line.Brand, line.ModelNumber, line.ProductType, line.ID,
				supplierID, purchaseDate, line.UnitCostMinor, line.Currency, now, now); err != nil {
				return ConfirmResult{}, err
			}
			eventID, _ := NewID("evt")
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO inventory_events(id,organization_id,product_id,event_type,to_status,actor_user_id,created_at)
				VALUES(?,?,?,'purchase.confirmed','purchasing',?,?)`,
				eventID, organizationID, productID, actorUserID, now); err != nil {
				return ConfirmResult{}, err
			}
			products = append(products, Product{
				ID: productID, OrganizationID: organizationID, ProductCode: code, Brand: line.Brand,
				ModelNumber: line.ModelNumber, ProductType: line.ProductType, PurchaseDate: purchaseDate,
				CostAmountMinor: line.UnitCostMinor, CostCurrency: line.Currency, InventoryStatus: "purchasing",
			})
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE purchase_slip_lines SET generated_product_count=? WHERE id=?`,
			line.Quantity, line.ID); err != nil {
			return ConfirmResult{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE purchase_slips SET status='confirmed',confirmed_at=?,confirmed_by=?,updated_at=?
		WHERE id=? AND organization_id=? AND status='draft'`,
		now, actorUserID, now, slipID, organizationID); err != nil {
		return ConfirmResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return ConfirmResult{}, err
	}
	return ConfirmResult{Products: products}, nil
}

func nextProductCodeTx(ctx context.Context, tx *sql.Tx, organizationID, purchaseDate string) (string, error) {
	var sequence int
	err := tx.QueryRowContext(ctx, `
		INSERT INTO product_code_sequences(organization_id,purchase_date,last_sequence)
		VALUES(?,?,1)
		ON CONFLICT(organization_id,purchase_date) DO UPDATE SET last_sequence=last_sequence+1
		WHERE last_sequence < 9999
		RETURNING last_sequence`, organizationID, purchaseDate).Scan(&sequence)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrDailyProductLimit
	}
	if err != nil {
		return "", err
	}
	date, err := time.Parse("2006-01-02", purchaseDate)
	if err != nil {
		return "", err
	}
	return date.Format("020106") + fmt.Sprintf("%04d", sequence), nil
}

func productsForPurchaseTx(ctx context.Context, tx *sql.Tx, organizationID, slipID string) ([]Product, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT p.id,p.organization_id,p.product_code,p.brand,p.model_number,p.product_type,p.purchase_date,
		       p.cost_amount_minor,p.cost_currency,p.inventory_status
		FROM products p JOIN purchase_slip_lines l ON l.id=p.purchase_slip_line_id
		WHERE p.organization_id=? AND l.purchase_slip_id=? ORDER BY p.product_code`, organizationID, slipID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var products []Product
	for rows.Next() {
		var p Product
		if err := rows.Scan(&p.ID, &p.OrganizationID, &p.ProductCode, &p.Brand, &p.ModelNumber, &p.ProductType, &p.PurchaseDate, &p.CostAmountMinor, &p.CostCurrency, &p.InventoryStatus); err != nil {
			return nil, err
		}
		products = append(products, p)
	}
	return products, rows.Err()
}

func (s *Store) CreateSingleProduct(ctx context.Context, input SingleProductInput) (Product, error) {
	if input.BaseSaleCurrency == "" {
		input.BaseSaleCurrency = "USD"
	}
	if err := validateSingleProduct(input); err != nil {
		return Product{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Product{}, err
	}
	defer tx.Rollback()
	candidates, err := serialDuplicatesTx(ctx, tx, input.OrganizationID, input.SerialNumber, "")
	if err != nil {
		return Product{}, err
	}
	if len(candidates) > 0 && strings.TrimSpace(input.DuplicateReason) == "" {
		return Product{}, &SerialDuplicateError{Candidates: candidates}
	}
	now := s.now().Format(time.RFC3339Nano)
	slipID, _ := NewID("pur")
	lineID, _ := NewID("pul")
	productID, _ := NewID("prd")
	slipNumber, err := nextSlipNumberTx(ctx, tx, input.OrganizationID, input.PurchaseDate)
	if err != nil {
		return Product{}, err
	}
	code, err := nextProductCodeTx(ctx, tx, input.OrganizationID, input.PurchaseDate)
	if err != nil {
		return Product{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO purchase_slips(
			id,organization_id,slip_number,supplier_id,purchase_date,status,is_simple,confirmed_at,confirmed_by,created_by,created_at,updated_at
		) VALUES(?,?,?,?,?,'confirmed',1,?,?,?,?,?)`,
		slipID, input.OrganizationID, slipNumber, input.SupplierID, input.PurchaseDate,
		now, input.CreatedBy, input.CreatedBy, now, now); err != nil {
		return Product{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO purchase_slip_lines(
			id,purchase_slip_id,line_number,quantity,unit_cost_minor,currency,brand,model_number,product_type,generated_product_count,created_at
		) VALUES(?,?,1,1,?,?,?,?,?,1,?)`,
		lineID, slipID, input.CostAmountMinor, input.CostCurrency, input.Brand, input.ModelNumber, input.ProductType, now); err != nil {
		return Product{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO products(
			id,organization_id,product_code,sku,brand,model_number,serial_number,product_type,purchase_slip_line_id,
			supplier_id,purchase_date,cost_amount_minor,cost_currency,base_sale_price_minor,base_sale_currency,
			inventory_status,condition_text,accessories,created_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,'in_stock',?,?,?,?)`,
		productID, input.OrganizationID, code, strings.TrimSpace(input.SKU), strings.TrimSpace(input.Brand),
		strings.TrimSpace(input.ModelNumber), strings.TrimSpace(input.SerialNumber), strings.TrimSpace(input.ProductType),
		lineID, input.SupplierID, input.PurchaseDate, input.CostAmountMinor, input.CostCurrency,
		input.BaseSalePriceMinor, input.BaseSaleCurrency, strings.TrimSpace(input.Condition),
		strings.TrimSpace(input.Accessories), now, now); err != nil {
		return Product{}, err
	}
	eventID, _ := NewID("evt")
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO inventory_events(id,organization_id,product_id,event_type,to_status,actor_user_id,created_at)
		VALUES(?,?,?,'single.created','in_stock',?,?)`, eventID, input.OrganizationID, productID, input.CreatedBy, now); err != nil {
		return Product{}, err
	}
	if len(candidates) > 0 {
		candidateIDs := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			candidateIDs = append(candidateIDs, candidate.ID)
		}
		encoded, _ := json.Marshal(candidateIDs)
		overrideID, _ := NewID("sdo")
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO serial_duplicate_overrides(
				id,organization_id,product_id,serial_number,candidate_product_ids_json,reason,actor_user_id,created_at
			) VALUES(?,?,?,?,?,?,?,?)`,
			overrideID, input.OrganizationID, productID, input.SerialNumber, string(encoded),
			strings.TrimSpace(input.DuplicateReason), input.CreatedBy, now); err != nil {
			return Product{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Product{}, err
	}
	return s.Product(ctx, input.OrganizationID, productID)
}

func validateSingleProduct(input SingleProductInput) error {
	if input.OrganizationID == "" || input.SupplierID == "" || input.CreatedBy == "" {
		return errors.New("仕入先と操作者は必須です")
	}
	if _, err := time.Parse("2006-01-02", input.PurchaseDate); err != nil {
		return errors.New("仕入日を正しく入力してください")
	}
	if strings.TrimSpace(input.Brand) == "" || strings.TrimSpace(input.ProductType) == "" {
		return errors.New("ブランドと商品種別は必須です")
	}
	if input.CostAmountMinor < 0 || input.BaseSalePriceMinor < 0 {
		return errors.New("金額は0以上で入力してください")
	}
	if (input.CostCurrency != "JPY" && input.CostCurrency != "USD") ||
		(input.BaseSaleCurrency != "JPY" && input.BaseSaleCurrency != "USD") {
		return errors.New("通貨はJPYまたはUSDを選択してください")
	}
	return nil
}

func serialDuplicatesTx(ctx context.Context, tx *sql.Tx, organizationID, serialNumber, excludeID string) ([]Product, error) {
	serialNumber = strings.TrimSpace(serialNumber)
	if serialNumber == "" {
		return nil, nil
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT p.id,p.organization_id,p.product_code,p.brand,p.model_number,p.serial_number,p.purchase_date,
		       p.inventory_status,s.name
		FROM products p JOIN suppliers s ON s.id=p.supplier_id
		WHERE p.organization_id=? AND p.serial_number=? AND p.id<>? AND p.deleted_at IS NULL
		ORDER BY p.created_at DESC`, organizationID, serialNumber, excludeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var products []Product
	for rows.Next() {
		var product Product
		if err := rows.Scan(&product.ID, &product.OrganizationID, &product.ProductCode, &product.Brand,
			&product.ModelNumber, &product.SerialNumber, &product.PurchaseDate, &product.InventoryStatus,
			&product.SupplierName); err != nil {
			return nil, err
		}
		products = append(products, product)
	}
	return products, rows.Err()
}

func (s *Store) SerialDuplicates(ctx context.Context, organizationID, serialNumber, excludeID string) ([]Product, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	products, err := serialDuplicatesTx(ctx, tx, organizationID, serialNumber, excludeID)
	if err != nil {
		return nil, err
	}
	return products, tx.Commit()
}

func (s *Store) Products(ctx context.Context, organizationID string, filter ProductFilter) ([]Product, error) {
	query := `
		SELECT p.id,p.organization_id,p.product_code,p.sku,p.brand,p.model_number,p.serial_number,p.product_type,
		       p.supplier_id,s.name,p.purchase_date,p.cost_amount_minor,p.cost_currency,p.base_sale_price_minor,
		       p.base_sale_currency,p.inventory_status,p.publication_status,p.condition_text,p.accessories,p.created_at,
		       (SELECT COUNT(*) FROM product_images i WHERE i.organization_id=p.organization_id AND i.product_id=p.id)
		FROM products p JOIN suppliers s ON s.id=p.supplier_id
		WHERE p.organization_id=? AND p.deleted_at IS NULL`
	args := []any{organizationID}
	if filter.Status != "" {
		query += ` AND p.inventory_status=?`
		args = append(args, filter.Status)
	}
	if strings.TrimSpace(filter.Query) != "" {
		like := "%" + strings.TrimSpace(filter.Query) + "%"
		query += ` AND (p.product_code LIKE ? OR p.sku LIKE ? OR p.brand LIKE ? OR p.model_number LIKE ? OR p.serial_number LIKE ?)`
		args = append(args, like, like, like, like, like)
	}
	query += ` ORDER BY p.purchase_date DESC,p.product_code DESC LIMIT 500`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var products []Product
	for rows.Next() {
		product, err := scanProduct(rows)
		if err != nil {
			return nil, err
		}
		products = append(products, product)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for index := range products {
		if err := s.enrichProductPricing(ctx, &products[index]); err != nil {
			return nil, err
		}
	}
	return products, nil
}

func (s *Store) PagedProducts(ctx context.Context, organizationID string, filter ProductFilter) (ProductPage, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 || filter.PageSize > 100 {
		filter.PageSize = 20
	}
	where := ` FROM products p JOIN suppliers s ON s.id=p.supplier_id
		WHERE p.organization_id=? AND p.deleted_at IS NULL`
	args := []any{organizationID}
	if !filter.IncludeCancelled {
		where += ` AND p.inventory_status <> 'cancelled'`
	}
	if filter.Status != "" {
		where += ` AND p.inventory_status=?`
		args = append(args, filter.Status)
	}
	if strings.TrimSpace(filter.Query) != "" {
		like := "%" + strings.TrimSpace(filter.Query) + "%"
		where += ` AND (p.product_code LIKE ? OR p.sku LIKE ? OR p.brand LIKE ? OR p.model_number LIKE ? OR p.serial_number LIKE ?)`
		args = append(args, like, like, like, like, like)
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*)`+where, args...).Scan(&total); err != nil {
		return ProductPage{}, err
	}
	totalPages := (total + filter.PageSize - 1) / filter.PageSize
	if totalPages == 0 {
		totalPages = 1
	}
	if filter.Page > totalPages {
		filter.Page = totalPages
	}
	order := map[string]string{
		"purchase_asc":    "p.purchase_date ASC,p.product_code ASC",
		"code_asc":        "p.product_code ASC",
		"code_desc":       "p.product_code DESC",
		"brand_asc":       "p.brand ASC,p.model_number ASC,p.product_code ASC",
		"sale_price_desc": "p.base_sale_price_minor DESC,p.product_code ASC",
	}[filter.Sort]
	if order == "" {
		filter.Sort = "purchase_desc"
		order = "p.purchase_date DESC,p.product_code DESC"
	}
	query := `
		SELECT p.id,p.organization_id,p.product_code,p.sku,p.brand,p.model_number,p.serial_number,p.product_type,
		       p.supplier_id,s.name,p.purchase_date,p.cost_amount_minor,p.cost_currency,p.base_sale_price_minor,
		       p.base_sale_currency,p.inventory_status,p.publication_status,p.condition_text,p.accessories,p.created_at,
		       (SELECT COUNT(*) FROM product_images i WHERE i.organization_id=p.organization_id AND i.product_id=p.id)` +
		where + ` ORDER BY ` + order + ` LIMIT ? OFFSET ?`
	queryArgs := append(append([]any{}, args...), filter.PageSize, (filter.Page-1)*filter.PageSize)
	rows, err := s.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return ProductPage{}, err
	}
	defer rows.Close()
	var products []Product
	for rows.Next() {
		product, err := scanProduct(rows)
		if err != nil {
			return ProductPage{}, err
		}
		products = append(products, product)
	}
	if err := rows.Err(); err != nil {
		return ProductPage{}, err
	}
	if err := rows.Close(); err != nil {
		return ProductPage{}, err
	}
	for index := range products {
		if err := s.enrichProductPricing(ctx, &products[index]); err != nil {
			return ProductPage{}, err
		}
	}
	return ProductPage{
		Products: products, Total: total, Page: filter.Page,
		PageSize: filter.PageSize, TotalPages: totalPages,
	}, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanProduct(row scanner) (Product, error) {
	var product Product
	var created string
	err := row.Scan(
		&product.ID, &product.OrganizationID, &product.ProductCode, &product.SKU, &product.Brand,
		&product.ModelNumber, &product.SerialNumber, &product.ProductType, &product.SupplierID,
		&product.SupplierName, &product.PurchaseDate, &product.CostAmountMinor, &product.CostCurrency,
		&product.BaseSalePriceMinor, &product.BaseSaleCurrency, &product.InventoryStatus,
		&product.PublicationStatus, &product.Condition, &product.Accessories, &created, &product.ImageCount,
	)
	product.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	return product, err
}

func (s *Store) Product(ctx context.Context, organizationID, productID string) (Product, error) {
	product, err := scanProduct(s.db.QueryRowContext(ctx, `
		SELECT p.id,p.organization_id,p.product_code,p.sku,p.brand,p.model_number,p.serial_number,p.product_type,
		       p.supplier_id,s.name,p.purchase_date,p.cost_amount_minor,p.cost_currency,p.base_sale_price_minor,
		       p.base_sale_currency,p.inventory_status,p.publication_status,p.condition_text,p.accessories,p.created_at,
		       (SELECT COUNT(*) FROM product_images i WHERE i.organization_id=p.organization_id AND i.product_id=p.id)
		FROM products p JOIN suppliers s ON s.id=p.supplier_id
		WHERE p.organization_id=? AND p.id=? AND p.deleted_at IS NULL`, organizationID, productID))
	if err != nil {
		return Product{}, err
	}
	if err := s.enrichProductPricing(ctx, &product); err != nil {
		return Product{}, err
	}
	images, err := s.ProductImages(ctx, organizationID, productID)
	if err != nil {
		return Product{}, err
	}
	product.Images = images
	rows, err := s.db.QueryContext(ctx, `
		SELECT e.event_type,e.from_status,e.to_status,e.reason,u.display_name,e.created_at
		FROM inventory_events e JOIN users u ON u.id=e.actor_user_id
		WHERE e.organization_id=? AND e.product_id=? ORDER BY e.created_at DESC`, organizationID, productID)
	if err != nil {
		return Product{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var event InventoryEvent
		var created string
		if err := rows.Scan(&event.EventType, &event.FromStatus, &event.ToStatus, &event.Reason, &event.ActorName, &created); err != nil {
			return Product{}, err
		}
		event.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		product.Events = append(product.Events, event)
	}
	return product, rows.Err()
}

func (s *Store) PurchaseSlips(ctx context.Context, organizationID string) ([]PurchaseSlip, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id,p.organization_id,p.slip_number,p.supplier_id,s.name,p.purchase_date,p.status,p.is_simple,
		       p.notes,p.created_at,p.confirmed_at,
		       COUNT(l.id),COALESCE(SUM(l.quantity*l.unit_cost_minor),0),COALESCE(MIN(l.currency),'JPY')
		FROM purchase_slips p JOIN suppliers s ON s.id=p.supplier_id
		LEFT JOIN purchase_slip_lines l ON l.purchase_slip_id=p.id
		WHERE p.organization_id=?
		GROUP BY p.id ORDER BY p.purchase_date DESC,p.slip_number DESC LIMIT 500`, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var slips []PurchaseSlip
	for rows.Next() {
		slip, err := scanPurchase(rows)
		if err != nil {
			return nil, err
		}
		slips = append(slips, slip)
	}
	return slips, rows.Err()
}

func scanPurchase(row scanner) (PurchaseSlip, error) {
	var slip PurchaseSlip
	var isSimple int
	var created string
	var confirmed sql.NullString
	err := row.Scan(&slip.ID, &slip.OrganizationID, &slip.SlipNumber, &slip.SupplierID, &slip.SupplierName,
		&slip.PurchaseDate, &slip.Status, &isSimple, &slip.Notes, &created, &confirmed,
		&slip.LineCount, &slip.TotalMinor, &slip.Currency)
	slip.IsSimple = isSimple == 1
	slip.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	if confirmed.Valid {
		value, _ := time.Parse(time.RFC3339Nano, confirmed.String)
		slip.ConfirmedAt = &value
	}
	return slip, err
}

func (s *Store) Purchase(ctx context.Context, organizationID, slipID string) (PurchaseSlip, error) {
	slip, err := scanPurchase(s.db.QueryRowContext(ctx, `
		SELECT p.id,p.organization_id,p.slip_number,p.supplier_id,s.name,p.purchase_date,p.status,p.is_simple,
		       p.notes,p.created_at,p.confirmed_at,
		       COUNT(l.id),COALESCE(SUM(l.quantity*l.unit_cost_minor),0),COALESCE(MIN(l.currency),'JPY')
		FROM purchase_slips p JOIN suppliers s ON s.id=p.supplier_id
		LEFT JOIN purchase_slip_lines l ON l.purchase_slip_id=p.id
		WHERE p.organization_id=? AND p.id=? GROUP BY p.id`, organizationID, slipID))
	if err != nil {
		return PurchaseSlip{}, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id,line_number,quantity,unit_cost_minor,currency,brand,model_number,product_type,generated_product_count
		FROM purchase_slip_lines WHERE purchase_slip_id=? ORDER BY line_number`, slipID)
	if err != nil {
		return PurchaseSlip{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var line PurchaseLine
		if err := rows.Scan(&line.ID, &line.LineNumber, &line.Quantity, &line.UnitCostMinor, &line.Currency,
			&line.Brand, &line.ModelNumber, &line.ProductType, &line.GeneratedProductCount); err != nil {
			return PurchaseSlip{}, err
		}
		slip.Lines = append(slip.Lines, line)
	}
	return slip, rows.Err()
}

func (s *Store) UpdateProductStatus(ctx context.Context, organizationID, productID, actorID, toStatus, reason string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var fromStatus string
	if err := tx.QueryRowContext(ctx, `
		SELECT inventory_status FROM products WHERE organization_id=? AND id=? AND deleted_at IS NULL`,
		organizationID, productID).Scan(&fromStatus); err != nil {
		return err
	}
	allowed := fromStatus == "purchasing" && toStatus == "in_stock"
	if !allowed {
		return ErrInvalidStatusTransition
	}
	now := s.now().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `
		UPDATE products SET inventory_status=?,updated_at=? WHERE organization_id=? AND id=?`,
		toStatus, now, organizationID, productID); err != nil {
		return err
	}
	eventID, _ := NewID("evt")
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO inventory_events(id,organization_id,product_id,event_type,from_status,to_status,reason,actor_user_id,created_at)
		VALUES(?,?,?,'status.changed',?,?,?,?,?)`,
		eventID, organizationID, productID, fromStatus, toStatus, strings.TrimSpace(reason), actorID, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) CancelProduct(ctx context.Context, organizationID, productID, actorID, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return errors.New("取消理由は必須です")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var fromStatus string
	if err := tx.QueryRowContext(ctx, `SELECT inventory_status FROM products WHERE organization_id=? AND id=?`, organizationID, productID).Scan(&fromStatus); err != nil {
		return err
	}
	now := s.now().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `
		UPDATE products SET inventory_status='cancelled',cancelled_at=?,cancelled_by=?,cancel_reason=?,updated_at=?
		WHERE organization_id=? AND id=?`, now, actorID, strings.TrimSpace(reason), now, organizationID, productID); err != nil {
		return err
	}
	eventID, _ := NewID("evt")
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO inventory_events(id,organization_id,product_id,event_type,from_status,to_status,reason,actor_user_id,created_at)
		VALUES(?,?,?,'product.cancelled',?,'cancelled',?,?,?)`,
		eventID, organizationID, productID, fromStatus, strings.TrimSpace(reason), actorID, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) AddProductImage(ctx context.Context, image ProductImage, organizationID, actorID string) (ProductImage, error) {
	if image.ID == "" {
		image.ID, _ = NewID("img")
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM product_images WHERE organization_id=? AND product_id=?`,
		organizationID, image.ProductID).Scan(&count); err != nil {
		return ProductImage{}, err
	}
	if count >= 10 {
		return ProductImage{}, errors.New("商品画像は10枚までです")
	}
	image.SortOrder = count + 1
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO product_images(
			id,organization_id,product_id,storage_path,original_name,content_type,size_bytes,sort_order,uploaded_by,created_at
		) SELECT ?,?,?,?,?,?,?,?,?,? WHERE EXISTS(
			SELECT 1 FROM products WHERE id=? AND organization_id=? AND deleted_at IS NULL
		)`,
		image.ID, organizationID, image.ProductID, image.StoragePath, image.OriginalName, image.ContentType,
		image.SizeBytes, image.SortOrder, actorID, s.now().Format(time.RFC3339Nano), image.ProductID, organizationID)
	return image, err
}

func (s *Store) ProductImages(ctx context.Context, organizationID, productID string) ([]ProductImage, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id,product_id,storage_path,original_name,content_type,size_bytes,sort_order
		FROM product_images WHERE organization_id=? AND product_id=? ORDER BY sort_order`,
		organizationID, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var images []ProductImage
	for rows.Next() {
		var image ProductImage
		if err := rows.Scan(&image.ID, &image.ProductID, &image.StoragePath, &image.OriginalName, &image.ContentType, &image.SizeBytes, &image.SortOrder); err != nil {
			return nil, err
		}
		images = append(images, image)
	}
	return images, rows.Err()
}

func (s *Store) ProductImage(ctx context.Context, organizationID, imageID string) (ProductImage, error) {
	var image ProductImage
	err := s.db.QueryRowContext(ctx, `
		SELECT id,product_id,storage_path,original_name,content_type,size_bytes,sort_order
		FROM product_images WHERE organization_id=? AND id=?`, organizationID, imageID).
		Scan(&image.ID, &image.ProductID, &image.StoragePath, &image.OriginalName, &image.ContentType, &image.SizeBytes, &image.SortOrder)
	return image, err
}

func ParseMinorAmount(value string) (int64, error) {
	value = strings.ReplaceAll(strings.TrimSpace(value), ",", "")
	if value == "" {
		return 0, nil
	}
	amount, err := strconv.ParseInt(value, 10, 64)
	if err != nil || amount < 0 {
		return 0, errors.New("金額は0以上の整数で入力してください")
	}
	return amount, nil
}
