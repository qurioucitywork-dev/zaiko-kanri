package database

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

const RateScale int64 = 100_000_000

type ExchangeRate struct {
	ID             string
	OrganizationID string
	BaseCurrency   string
	QuoteCurrency  string
	RateScaled     int64
	Scale          int64
	Provider       string
	ObservedAt     time.Time
	CreatedAt      time.Time
}

type MarketPrice struct {
	ID             string
	OrganizationID string
	MarketDate     string
	Brand          string
	ModelNumber    string
	ProductType    string
	PriceMinor     int64
	Currency       string
	Source         string
	Notes          string
	CreatedAt      time.Time
}

type MarketPriceInput struct {
	OrganizationID string
	MarketDate     string
	Brand          string
	ModelNumber    string
	ProductType    string
	PriceMinor     int64
	Currency       string
	Source         string
	Notes          string
	CreatedBy      string
}

type ProductMarketPrice struct {
	ProductID                string
	ProductCode              string
	SKU                      string
	Brand                    string
	Model                    string
	ModelNumber              string
	SerialNumber             string
	InventoryStatus          string
	PurchasePriceMinor       int64
	BaseSalePriceMinor       int64
	PurchaseMarketPriceMinor int64
	SaleMarketPriceMinor     int64
	HasMarketPrice           bool
}

type ProductMarketPriceFilter struct {
	Query       string
	Brand       string
	ModelNumber string
}

type MarketImportBatch struct {
	ID             string
	OrganizationID string
	FileName       string
	Status         string
	TotalRows      int
	ValidRows      int
	ErrorRows      int
	DuplicateRows  int
	CreatedBy      string
	CreatedAt      time.Time
	Rows           []MarketImportRow
}

type MarketImportRow struct {
	ID                   string
	RowNumber            int
	MarketDate           string
	Brand                string
	ModelNumber          string
	ProductType          string
	PriceMinor           int64
	Currency             string
	Source               string
	Valid                bool
	ErrorMessage         string
	DuplicateCandidateID string
}

func ParseRate(value string) (int64, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, ",", ""))
	if value == "" {
		return 0, errors.New("為替レートは必須です")
	}
	parts := strings.Split(value, ".")
	if len(parts) > 2 {
		return 0, errors.New("為替レートの形式が正しくありません")
	}
	integer, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || integer < 0 {
		return 0, errors.New("為替レートは正の数で入力してください")
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	if len(fraction) > 8 {
		return 0, errors.New("為替レートは小数点以下8桁以内で入力してください")
	}
	for len(fraction) < 8 {
		fraction += "0"
	}
	fractionValue := int64(0)
	if fraction != "" {
		fractionValue, err = strconv.ParseInt(fraction, 10, 64)
		if err != nil {
			return 0, errors.New("為替レートの形式が正しくありません")
		}
	}
	if integer > (1<<63-1-fractionValue)/RateScale {
		return 0, errors.New("為替レートが大きすぎます")
	}
	scaled := integer*RateScale + fractionValue
	if scaled <= 0 {
		return 0, errors.New("為替レートは正の数で入力してください")
	}
	return scaled, nil
}

func (s *Store) AddExchangeRate(ctx context.Context, organizationID, base, quote string, rateScaled int64, provider, observedAt, actorID string) (ExchangeRate, error) {
	base = strings.ToUpper(strings.TrimSpace(base))
	quote = strings.ToUpper(strings.TrimSpace(quote))
	supportedBase := base == "USD" || base == "EUR" || base == "HKD" || base == "CHF"
	if !supportedBase || quote != "JPY" || base == quote {
		return ExchangeRate{}, errors.New("通貨ペアを確認してください")
	}
	if rateScaled <= 0 {
		return ExchangeRate{}, errors.New("為替レートは正の値が必要です")
	}
	observed, err := time.ParseInLocation("2006-01-02T15:04", observedAt, time.FixedZone("JST", 9*60*60))
	if err != nil {
		return ExchangeRate{}, errors.New("レート取得日時を確認してください")
	}
	id, _ := NewID("fx")
	now := s.now()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO exchange_rate_snapshots(
			id,organization_id,base_currency,quote_currency,rate_scaled,scale,provider,observed_at,created_by,created_at
		) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		id, organizationID, base, quote, rateScaled, RateScale, strings.TrimSpace(provider),
		observed.UTC().Format(time.RFC3339Nano), actorID, now.Format(time.RFC3339Nano))
	if err != nil {
		return ExchangeRate{}, err
	}
	return ExchangeRate{
		ID: id, OrganizationID: organizationID, BaseCurrency: base, QuoteCurrency: quote,
		RateScaled: rateScaled, Scale: RateScale, Provider: strings.TrimSpace(provider),
		ObservedAt: observed.UTC(), CreatedAt: now,
	}, nil
}

func (s *Store) SeedMarketPreview(ctx context.Context) error {
	var rateCount int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM exchange_rate_snapshots WHERE organization_id='org_preview'`).Scan(&rateCount); err != nil {
		return err
	}
	if rateCount == 0 {
		rate, err := ParseRate("154.25")
		if err != nil {
			return err
		}
		if _, err := s.AddExchangeRate(ctx, "org_preview", "USD", "JPY", rate, "手動登録", "2026-07-26T10:00", "usr_admin"); err != nil {
			return err
		}
	}
	var marketCount int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM market_price_records WHERE organization_id='org_preview'`).Scan(&marketCount); err != nil {
		return err
	}
	if marketCount == 0 {
		_, err := s.AddMarketPrice(ctx, MarketPriceInput{
			OrganizationID: "org_preview", MarketDate: "2026-07-26", Brand: "ロレックス",
			ModelNumber: "116610LN", ProductType: "腕時計", PriceMinor: 1_200_000,
			Currency: "JPY", Source: "手動登録", Notes: "プレビュー用相場", CreatedBy: "usr_admin",
		})
		return err
	}
	return nil
}

func (s *Store) ExchangeRates(ctx context.Context, organizationID string, limit int) ([]ExchangeRate, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id,organization_id,base_currency,quote_currency,rate_scaled,scale,provider,observed_at,created_at
		FROM exchange_rate_snapshots WHERE organization_id=?
		ORDER BY observed_at DESC LIMIT ?`, organizationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rates []ExchangeRate
	for rows.Next() {
		var rate ExchangeRate
		var observed, created string
		if err := rows.Scan(&rate.ID, &rate.OrganizationID, &rate.BaseCurrency, &rate.QuoteCurrency,
			&rate.RateScaled, &rate.Scale, &rate.Provider, &observed, &created); err != nil {
			return nil, err
		}
		rate.ObservedAt, _ = time.Parse(time.RFC3339Nano, observed)
		rate.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		rates = append(rates, rate)
	}
	return rates, rows.Err()
}

func (s *Store) LatestExchangeRate(ctx context.Context, organizationID, base, quote string) (ExchangeRate, error) {
	var rate ExchangeRate
	var observed, created string
	err := s.db.QueryRowContext(ctx, `
		SELECT id,organization_id,base_currency,quote_currency,rate_scaled,scale,provider,observed_at,created_at
		FROM exchange_rate_snapshots
		WHERE organization_id=? AND base_currency=? AND quote_currency=?
		ORDER BY observed_at DESC,created_at DESC,id DESC LIMIT 1`, organizationID, base, quote).
		Scan(&rate.ID, &rate.OrganizationID, &rate.BaseCurrency, &rate.QuoteCurrency,
			&rate.RateScaled, &rate.Scale, &rate.Provider, &observed, &created)
	if err != nil {
		return ExchangeRate{}, err
	}
	rate.ObservedAt, _ = time.Parse(time.RFC3339Nano, observed)
	rate.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	return rate, nil
}

func ConvertMinor(amount, rateScaled, scale int64, inverse bool) (int64, error) {
	if amount < 0 || rateScaled <= 0 || scale <= 0 {
		return 0, errors.New("換算値が正しくありません")
	}
	if inverse {
		if amount > (1<<63-1)/scale {
			return 0, errors.New("換算金額が大きすぎます")
		}
		return amount * scale / rateScaled, nil
	}
	if amount > (1<<63-1)/rateScaled {
		return 0, errors.New("換算金額が大きすぎます")
	}
	return amount * rateScaled / scale, nil
}

func (s *Store) enrichProductPricing(ctx context.Context, product *Product) error {
	rate, err := s.LatestExchangeRate(ctx, product.OrganizationID, "USD", "JPY")
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	product.RateAvailable = true
	product.ExchangeRateScaled = rate.RateScaled
	product.ExchangeRateObservedAt = rate.ObservedAt
	var revenueJPY int64
	if product.BaseSaleCurrency == "USD" {
		product.ReferencePriceMinor, err = ConvertMinor(product.BaseSalePriceMinor, rate.RateScaled, rate.Scale, false)
		product.ReferenceCurrency = "JPY"
		revenueJPY = product.ReferencePriceMinor
	} else {
		product.ReferencePriceMinor, err = ConvertMinor(product.BaseSalePriceMinor, rate.RateScaled, rate.Scale, true)
		product.ReferenceCurrency = "USD"
		revenueJPY = product.BaseSalePriceMinor
	}
	if err != nil {
		return err
	}
	var costJPY int64
	if product.CostCurrency == "USD" {
		costJPY, err = ConvertMinor(product.CostAmountMinor, rate.RateScaled, rate.Scale, false)
	} else {
		costJPY = product.CostAmountMinor
	}
	if err != nil {
		return err
	}
	product.GrossProfitMinor = revenueJPY - costJPY
	product.GrossProfitCurrency = "JPY"
	if revenueJPY > 0 {
		product.MarginBasisPoints = product.GrossProfitMinor * 10_000 / revenueJPY
	}
	return nil
}

func (s *Store) AddMarketPrice(ctx context.Context, input MarketPriceInput) (MarketPrice, error) {
	if err := validateMarketPrice(input); err != nil {
		return MarketPrice{}, err
	}
	id, _ := NewID("mkt")
	now := s.now()
	source := strings.TrimSpace(input.Source)
	if source == "" {
		source = "manual"
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO market_price_records(
			id,organization_id,market_date,brand,model_number,product_type,price_minor,currency,source,notes,created_by,created_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		id, input.OrganizationID, input.MarketDate, strings.TrimSpace(input.Brand),
		strings.TrimSpace(input.ModelNumber), strings.TrimSpace(input.ProductType), input.PriceMinor,
		input.Currency, source, strings.TrimSpace(input.Notes), input.CreatedBy,
		now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err != nil {
		return MarketPrice{}, err
	}
	return MarketPrice{
		ID: id, OrganizationID: input.OrganizationID, MarketDate: input.MarketDate,
		Brand: strings.TrimSpace(input.Brand), ModelNumber: strings.TrimSpace(input.ModelNumber),
		ProductType: strings.TrimSpace(input.ProductType), PriceMinor: input.PriceMinor,
		Currency: input.Currency, Source: source, Notes: strings.TrimSpace(input.Notes), CreatedAt: now,
	}, nil
}

func validateMarketPrice(input MarketPriceInput) error {
	if _, err := time.Parse("2006-01-02", input.MarketDate); err != nil {
		return errors.New("相場日を確認してください")
	}
	if strings.TrimSpace(input.Brand) == "" || strings.TrimSpace(input.ProductType) == "" {
		return errors.New("ブランドと商品種別は必須です")
	}
	if input.PriceMinor < 0 {
		return errors.New("相場価格は0以上で入力してください")
	}
	if input.Currency != "JPY" && input.Currency != "USD" {
		return errors.New("通貨はJPYまたはUSDを選択してください")
	}
	return nil
}

func (s *Store) MarketPrices(ctx context.Context, organizationID string, limit int) ([]MarketPrice, error) {
	if limit < 1 || limit > 1000 {
		limit = 500
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id,organization_id,market_date,brand,model_number,product_type,price_minor,currency,source,notes,created_at
		FROM market_price_records WHERE organization_id=? AND is_active=1
		ORDER BY market_date DESC,created_at DESC LIMIT ?`, organizationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []MarketPrice
	for rows.Next() {
		var record MarketPrice
		var created string
		if err := rows.Scan(&record.ID, &record.OrganizationID, &record.MarketDate, &record.Brand,
			&record.ModelNumber, &record.ProductType, &record.PriceMinor, &record.Currency,
			&record.Source, &record.Notes, &created); err != nil {
			return nil, err
		}
		record.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Store) ProductMarketPrices(ctx context.Context, organizationID string, filter ProductMarketPriceFilter) ([]ProductMarketPrice, error) {
	query := `
		SELECT p.id,p.product_code,p.sku,p.brand,p.product_type,p.model_number,p.serial_number,
		       p.inventory_status,p.cost_amount_minor,p.base_sale_price_minor,
		       COALESCE(mp.purchase_market_price_minor,0),COALESCE(mp.sale_market_price_minor,0),
		       CASE WHEN mp.product_id IS NULL THEN 0 ELSE 1 END
		FROM products p
		LEFT JOIN product_market_prices mp
		  ON mp.organization_id=p.organization_id AND mp.product_id=p.id
		WHERE p.organization_id=? AND p.deleted_at IS NULL`
	args := []any{organizationID}
	if value := strings.TrimSpace(filter.Query); value != "" {
		like := "%" + value + "%"
		query += ` AND (p.product_code LIKE ? OR p.sku LIKE ? OR p.brand LIKE ? OR p.product_type LIKE ? OR p.model_number LIKE ? OR p.serial_number LIKE ?)`
		args = append(args, like, like, like, like, like, like)
	}
	if value := strings.TrimSpace(filter.Brand); value != "" {
		query += ` AND p.brand=?`
		args = append(args, value)
	}
	if value := strings.TrimSpace(filter.ModelNumber); value != "" {
		query += ` AND p.model_number LIKE ?`
		args = append(args, "%"+value+"%")
	}
	query += ` ORDER BY p.purchase_date DESC,p.product_code DESC LIMIT 500`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ProductMarketPrice
	for rows.Next() {
		var row ProductMarketPrice
		var hasPrice int
		if err := rows.Scan(
			&row.ProductID, &row.ProductCode, &row.SKU, &row.Brand, &row.Model,
			&row.ModelNumber, &row.SerialNumber, &row.InventoryStatus,
			&row.PurchasePriceMinor, &row.BaseSalePriceMinor,
			&row.PurchaseMarketPriceMinor, &row.SaleMarketPriceMinor, &hasPrice,
		); err != nil {
			return nil, err
		}
		row.HasMarketPrice = hasPrice == 1
		result = append(result, row)
	}
	return result, rows.Err()
}

func (s *Store) ProductMarketPriceByProductID(ctx context.Context, organizationID, productID string) (ProductMarketPrice, error) {
	var row ProductMarketPrice
	var hasPrice int
	err := s.db.QueryRowContext(ctx, `
		SELECT p.id,p.product_code,p.sku,p.brand,p.product_type,p.model_number,p.serial_number,
		       p.inventory_status,p.cost_amount_minor,p.base_sale_price_minor,
		       COALESCE(mp.purchase_market_price_minor,0),COALESCE(mp.sale_market_price_minor,0),
		       CASE WHEN mp.product_id IS NULL THEN 0 ELSE 1 END
		FROM products p
		LEFT JOIN product_market_prices mp
		  ON mp.organization_id=p.organization_id AND mp.product_id=p.id
		WHERE p.organization_id=? AND p.id=? AND p.deleted_at IS NULL`,
		organizationID, productID).Scan(
		&row.ProductID, &row.ProductCode, &row.SKU, &row.Brand, &row.Model,
		&row.ModelNumber, &row.SerialNumber, &row.InventoryStatus,
		&row.PurchasePriceMinor, &row.BaseSalePriceMinor,
		&row.PurchaseMarketPriceMinor, &row.SaleMarketPriceMinor, &hasPrice,
	)
	if err != nil {
		return ProductMarketPrice{}, err
	}
	row.HasMarketPrice = hasPrice == 1
	return row, nil
}

func (s *Store) UpdateProductMarketPrice(ctx context.Context, organizationID, productID, actorID string, purchaseMarketPriceMinor, saleMarketPriceMinor int64) error {
	if purchaseMarketPriceMinor < 0 || saleMarketPriceMinor < 0 {
		return errors.New("相場価格は0以上で入力してください")
	}
	var exists int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM products
		WHERE organization_id=? AND id=? AND deleted_at IS NULL`,
		organizationID, productID).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return sql.ErrNoRows
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO product_market_prices(
		  organization_id,product_id,purchase_market_price_minor,sale_market_price_minor,updated_by,updated_at
		) VALUES(?,?,?,?,?,?)
		ON CONFLICT(organization_id,product_id) DO UPDATE SET
		  purchase_market_price_minor=excluded.purchase_market_price_minor,
		  sale_market_price_minor=excluded.sale_market_price_minor,
		  updated_by=excluded.updated_by,updated_at=excluded.updated_at`,
		organizationID, productID, purchaseMarketPriceMinor, saleMarketPriceMinor,
		actorID, s.now().Format(time.RFC3339Nano))
	return err
}

func (s *Store) ImportProductMarketPricesCSV(ctx context.Context, organizationID, actorID string, reader io.Reader) (int, error) {
	csvReader := csv.NewReader(reader)
	csvReader.TrimLeadingSpace = true
	header, err := csvReader.Read()
	if err != nil {
		return 0, errors.New("CSVヘッダーを読み取れませんでした")
	}
	if len(header) > 0 {
		header[0] = strings.TrimPrefix(header[0], "\ufeff")
	}
	expected := []string{"product_code", "purchase_market_price", "sale_market_price"}
	if len(header) != len(expected) {
		return 0, fmt.Errorf("CSV列数は%d列必要です", len(expected))
	}
	for index := range expected {
		if strings.TrimSpace(header[index]) != expected[index] {
			return 0, fmt.Errorf("CSVヘッダー%d列目は%sが必要です", index+1, expected[index])
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	count := 0
	for line := 2; line <= 5001; line++ {
		values, readErr := csvReader.Read()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil || len(values) != len(expected) {
			return 0, fmt.Errorf("%d行目のCSV形式を確認してください", line)
		}
		productCode := strings.TrimSpace(values[0])
		purchasePrice, parseErr := ParseMinorAmount(values[1])
		if parseErr != nil || purchasePrice < 0 {
			return 0, fmt.Errorf("%d行目の仕入相場価格を確認してください", line)
		}
		salePrice, parseErr := ParseMinorAmount(values[2])
		if parseErr != nil || salePrice < 0 {
			return 0, fmt.Errorf("%d行目の売値相場価格を確認してください", line)
		}
		var productID string
		if err := tx.QueryRowContext(ctx, `
			SELECT id FROM products
			WHERE organization_id=? AND product_code=? AND deleted_at IS NULL`,
			organizationID, productCode).Scan(&productID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return 0, fmt.Errorf("%d行目の商品コードが見つかりません", line)
			}
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO product_market_prices(
			  organization_id,product_id,purchase_market_price_minor,sale_market_price_minor,updated_by,updated_at
			) VALUES(?,?,?,?,?,?)
			ON CONFLICT(organization_id,product_id) DO UPDATE SET
			  purchase_market_price_minor=excluded.purchase_market_price_minor,
			  sale_market_price_minor=excluded.sale_market_price_minor,
			  updated_by=excluded.updated_by,updated_at=excluded.updated_at`,
			organizationID, productID, purchasePrice, salePrice, actorID,
			s.now().Format(time.RFC3339Nano)); err != nil {
			return 0, err
		}
		count++
	}
	if count == 0 {
		return 0, errors.New("CSVにデータ行がありません")
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) PreviewMarketCSV(ctx context.Context, organizationID, actorID, fileName string, reader io.Reader) (MarketImportBatch, error) {
	csvReader := csv.NewReader(reader)
	csvReader.FieldsPerRecord = -1
	csvReader.TrimLeadingSpace = true
	header, err := csvReader.Read()
	if err != nil {
		return MarketImportBatch{}, errors.New("CSVヘッダーを読み取れませんでした")
	}
	if len(header) > 0 {
		header[0] = strings.TrimPrefix(header[0], "\ufeff")
	}
	expected := []string{"market_date", "brand", "model_number", "product_type", "price", "currency", "source"}
	if len(header) != len(expected) {
		return MarketImportBatch{}, fmt.Errorf("CSV列数は%d列必要です", len(expected))
	}
	for index := range expected {
		if strings.TrimSpace(header[index]) != expected[index] {
			return MarketImportBatch{}, fmt.Errorf("CSVヘッダー%d列目は%sが必要です", index+1, expected[index])
		}
	}
	batchID, _ := NewID("mib")
	now := s.now().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return MarketImportBatch{}, err
	}
	defer tx.Rollback()
	batch := MarketImportBatch{
		ID: batchID, OrganizationID: organizationID, FileName: fileName,
		Status: "previewed", CreatedBy: actorID,
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO market_import_batches(
			id,organization_id,file_name,status,total_rows,valid_rows,error_rows,duplicate_rows,created_by,created_at
		) VALUES(?,?,?,'previewed',0,0,0,0,?,?)`,
		batchID, organizationID, fileName, actorID, now); err != nil {
		return MarketImportBatch{}, err
	}
	seenRows := make(map[string]int)
	for rowNumber := 2; rowNumber <= 5001; rowNumber++ {
		values, readErr := csvReader.Read()
		if errors.Is(readErr, io.EOF) {
			break
		}
		row := MarketImportRow{RowNumber: rowNumber}
		if readErr != nil {
			row.ErrorMessage = "CSV形式を解析できません"
		} else if len(values) != len(expected) {
			row.ErrorMessage = fmt.Sprintf("列数が不正です（%d列）", len(values))
		} else {
			row.MarketDate = strings.TrimSpace(values[0])
			row.Brand = strings.TrimSpace(values[1])
			row.ModelNumber = strings.TrimSpace(values[2])
			row.ProductType = strings.TrimSpace(values[3])
			row.Currency = strings.TrimSpace(values[5])
			row.Source = strings.TrimSpace(values[6])
			row.PriceMinor, readErr = ParseMinorAmount(values[4])
			input := MarketPriceInput{
				OrganizationID: organizationID, MarketDate: row.MarketDate, Brand: row.Brand,
				ModelNumber: row.ModelNumber, ProductType: row.ProductType, PriceMinor: row.PriceMinor,
				Currency: row.Currency, Source: row.Source, CreatedBy: actorID,
			}
			if readErr != nil {
				row.ErrorMessage = readErr.Error()
			} else if validationErr := validateMarketPrice(input); validationErr != nil {
				row.ErrorMessage = validationErr.Error()
			} else {
				var duplicateID string
				duplicateErr := tx.QueryRowContext(ctx, `
					SELECT id FROM market_price_records
					WHERE organization_id=? AND market_date=? AND brand=? AND model_number=? AND currency=? AND is_active=1
					LIMIT 1`, organizationID, row.MarketDate, row.Brand, row.ModelNumber, row.Currency).Scan(&duplicateID)
				if duplicateErr == nil {
					row.DuplicateCandidateID = duplicateID
					row.ErrorMessage = "同じ日付・ブランド・型番・通貨の相場が登録済みです"
				} else if !errors.Is(duplicateErr, sql.ErrNoRows) {
					return MarketImportBatch{}, duplicateErr
				} else {
					key := strings.Join([]string{row.MarketDate, row.Brand, row.ModelNumber, row.Currency}, "\x00")
					if previousRow, exists := seenRows[key]; exists {
						row.ErrorMessage = fmt.Sprintf("CSV内の%d行目と重複しています", previousRow)
					} else {
						seenRows[key] = row.RowNumber
						row.Valid = true
					}
				}
			}
		}
		raw, _ := json.Marshal(values)
		row.ID, _ = NewID("mir")
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO market_import_rows(
				id,batch_id,row_number,market_date,brand,model_number,product_type,price_minor,currency,source,
				raw_json,is_valid,error_message,duplicate_candidate_id
			) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			row.ID, batchID, row.RowNumber, row.MarketDate, row.Brand, row.ModelNumber, row.ProductType,
			row.PriceMinor, row.Currency, row.Source, string(raw), row.Valid, row.ErrorMessage,
			nullString(row.DuplicateCandidateID)); err != nil {
			return MarketImportBatch{}, err
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
			return MarketImportBatch{}, errors.New("CSVは5000データ行以内にしてください")
		}
	}
	if batch.TotalRows == 0 {
		return MarketImportBatch{}, errors.New("CSVにデータ行がありません")
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE market_import_batches
		SET total_rows=?,valid_rows=?,error_rows=?,duplicate_rows=?
		WHERE id=? AND organization_id=?`,
		batch.TotalRows, batch.ValidRows, batch.ErrorRows, batch.DuplicateRows,
		batchID, organizationID); err != nil {
		return MarketImportBatch{}, err
	}
	if err := tx.Commit(); err != nil {
		return MarketImportBatch{}, err
	}
	batch.CreatedAt, _ = time.Parse(time.RFC3339Nano, now)
	return batch, nil
}

func (s *Store) MarketImportBatch(ctx context.Context, organizationID, batchID string) (MarketImportBatch, error) {
	var batch MarketImportBatch
	var created string
	err := s.db.QueryRowContext(ctx, `
		SELECT id,organization_id,file_name,status,total_rows,valid_rows,error_rows,duplicate_rows,created_by,created_at
		FROM market_import_batches WHERE organization_id=? AND id=?`, organizationID, batchID).
		Scan(&batch.ID, &batch.OrganizationID, &batch.FileName, &batch.Status, &batch.TotalRows,
			&batch.ValidRows, &batch.ErrorRows, &batch.DuplicateRows, &batch.CreatedBy, &created)
	if err != nil {
		return MarketImportBatch{}, err
	}
	batch.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	rows, err := s.db.QueryContext(ctx, `
		SELECT id,row_number,market_date,brand,model_number,product_type,price_minor,currency,source,
		       is_valid,error_message,COALESCE(duplicate_candidate_id,'')
		FROM market_import_rows WHERE batch_id=? ORDER BY row_number`, batchID)
	if err != nil {
		return MarketImportBatch{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var row MarketImportRow
		if err := rows.Scan(&row.ID, &row.RowNumber, &row.MarketDate, &row.Brand, &row.ModelNumber,
			&row.ProductType, &row.PriceMinor, &row.Currency, &row.Source, &row.Valid,
			&row.ErrorMessage, &row.DuplicateCandidateID); err != nil {
			return MarketImportBatch{}, err
		}
		batch.Rows = append(batch.Rows, row)
	}
	return batch, rows.Err()
}

func (s *Store) CommitMarketImport(ctx context.Context, organizationID, batchID, actorID string, requireApproval bool) (MarketImportBatch, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return MarketImportBatch{}, err
	}
	defer tx.Rollback()
	var status string
	var errorRows int
	if err := tx.QueryRowContext(ctx, `
		SELECT status,error_rows FROM market_import_batches WHERE organization_id=? AND id=?`,
		organizationID, batchID).Scan(&status, &errorRows); err != nil {
		return MarketImportBatch{}, err
	}
	if status != "previewed" {
		return MarketImportBatch{}, errors.New("プレビュー済みの未確定バッチだけ処理できます")
	}
	if errorRows > 0 {
		return MarketImportBatch{}, errors.New("エラー行があるため確定できません")
	}
	if requireApproval {
		if _, err := tx.ExecContext(ctx, `
			UPDATE market_import_batches SET status='pending_approval' WHERE id=? AND organization_id=?`,
			batchID, organizationID); err != nil {
			return MarketImportBatch{}, err
		}
		if err := tx.Commit(); err != nil {
			return MarketImportBatch{}, err
		}
		return s.MarketImportBatch(ctx, organizationID, batchID)
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT market_date,brand,model_number,product_type,price_minor,currency,source
		FROM market_import_rows WHERE batch_id=? AND is_valid=1 ORDER BY row_number`, batchID)
	if err != nil {
		return MarketImportBatch{}, err
	}
	type validRow struct {
		date, brand, model, productType, currency, source string
		price                                             int64
	}
	var validRows []validRow
	for rows.Next() {
		var row validRow
		if err := rows.Scan(&row.date, &row.brand, &row.model, &row.productType, &row.price, &row.currency, &row.source); err != nil {
			rows.Close()
			return MarketImportBatch{}, err
		}
		validRows = append(validRows, row)
	}
	rows.Close()
	now := s.now().Format(time.RFC3339Nano)
	for _, row := range validRows {
		id, _ := NewID("mkt")
		source := row.source
		if source == "" {
			source = "csv"
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO market_price_records(
				id,organization_id,market_date,brand,model_number,product_type,price_minor,currency,source,
				import_batch_id,created_by,created_at,updated_at
			) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			id, organizationID, row.date, row.brand, row.model, row.productType, row.price, row.currency,
			source, batchID, actorID, now, now); err != nil {
			return MarketImportBatch{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE market_import_batches SET status='committed',committed_by=?,committed_at=?
		WHERE id=? AND organization_id=?`, actorID, now, batchID, organizationID); err != nil {
		return MarketImportBatch{}, err
	}
	if err := tx.Commit(); err != nil {
		return MarketImportBatch{}, err
	}
	return s.MarketImportBatch(ctx, organizationID, batchID)
}
