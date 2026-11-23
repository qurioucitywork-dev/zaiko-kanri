package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrSaleAlreadyConfirmed     = errors.New("売上伝票はすでに確定されています")
	ErrShipmentAlreadyConfirmed = errors.New("出荷伝票はすでに確定されています")
	ErrShipmentExceedsSale      = errors.New("累計出荷数量が売上数量を超えます")
	ErrProductAlreadySold       = errors.New("この商品は別の売上で確定済みです")
)

type SalesLineInput struct {
	ProductID      string
	Quantity       int
	UnitPriceMinor int64
	Currency       string
}

type CreateSaleInput struct {
	OrganizationID string
	SalesDate      string
	CustomerName   string
	Notes          string
	CreatedBy      string
	Lines          []SalesLineInput
}

type SalesSlip struct {
	ID             string
	OrganizationID string
	SlipNumber     string
	SalesDate      string
	CustomerName   string
	Status         string
	Notes          string
	TotalJPY       int64
	TotalUSD       int64
	ShipmentStatus string
	Warning        string
	CreatedAt      time.Time
	ConfirmedAt    *time.Time
	Lines          []SalesLine
}

type SalesLine struct {
	ID                     string
	ProductID              string
	ProductCode            string
	Brand                  string
	ModelNumber            string
	Quantity               int
	UnitPriceMinor         int64
	SaleCurrency           string
	ExchangeRateSnapshotID string
	ExchangeRateScaled     int64
	ExchangeRateScale      int64
	ExchangeRateObservedAt *time.Time
	ConvertedUnitPriceJPY  int64
	ConvertedTotalJPY      int64
	ShippedQuantity        int
	RemainingQuantity      int
}

type ShipmentLineInput struct {
	ProductID string
	Quantity  int
}

type CreateShipmentInput struct {
	OrganizationID string
	ShipmentDate   string
	RecipientName  string
	Notes          string
	CreatedBy      string
	Lines          []ShipmentLineInput
}

type ShipmentSlip struct {
	ID             string
	OrganizationID string
	ShipmentNumber string
	ShipmentDate   string
	RecipientName  string
	Status         string
	Notes          string
	Warning        string
	CreatedAt      time.Time
	ConfirmedAt    *time.Time
	Lines          []ShipmentLine
}

type ShipmentLine struct {
	ID                string
	ProductID         string
	ProductCode       string
	Brand             string
	ModelNumber       string
	Quantity          int
	SalesLineID       string
	SalesSlipNumber   string
	AllocatedQuantity int
	Warning           string
}

func (s *Store) SeedSalesPreview(ctx context.Context) error {
	if err := s.migratePreviewSalesToUSD(ctx); err != nil {
		return err
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sales_slips WHERE organization_id='org_preview'`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	product, err := s.CreateSingleProduct(ctx, SingleProductInput{
		OrganizationID: "org_preview", SupplierID: "sup_002", PurchaseDate: "2026-07-26",
		SKU: "GS-SBGW301", Brand: "グランドセイコー", ModelNumber: "SBGW301",
		SerialNumber: "GS-PREVIEW-001", ProductType: "腕時計", CostAmountMinor: 430000, CostCurrency: "JPY",
		BaseSalePriceMinor: 4000, BaseSaleCurrency: "USD", Condition: "良品 (A)",
		Accessories: "BOX", CreatedBy: "usr_admin",
	})
	if err != nil {
		return err
	}
	if _, err := s.CreateSingleProduct(ctx, SingleProductInput{
		OrganizationID: "org_preview", SupplierID: "sup_003", PurchaseDate: "2026-07-26",
		SKU: "CARTIER-WSSA0018", Brand: "カルティエ", ModelNumber: "WSSA0018",
		SerialNumber: "CA-PREVIEW-001", ProductType: "腕時計", CostAmountMinor: 520000, CostCurrency: "JPY",
		BaseSalePriceMinor: 5032, BaseSaleCurrency: "USD", Condition: "極美品 (S)",
		Accessories: "BOX, GUARANTEE", CreatedBy: "usr_admin",
	}); err != nil {
		return err
	}
	sale, err := s.CreateSaleDraft(ctx, CreateSaleInput{
		OrganizationID: "org_preview", SalesDate: "2026-07-26", CustomerName: "鈴木 様",
		Notes: "", CreatedBy: "usr_admin",
		Lines: []SalesLineInput{{ProductID: product.ID, Quantity: 2, UnitPriceMinor: 2000, Currency: "USD"}},
	})
	if err != nil {
		return err
	}
	if _, err := s.ConfirmSale(ctx, "org_preview", sale.ID, "usr_admin"); err != nil {
		return err
	}
	shipment, err := s.CreateShipmentDraft(ctx, CreateShipmentInput{
		OrganizationID: "org_preview", ShipmentDate: "2026-07-26", RecipientName: "鈴木 様",
		Notes: "", CreatedBy: "usr_admin",
		Lines: []ShipmentLineInput{{ProductID: product.ID, Quantity: 1}},
	})
	if err != nil {
		return err
	}
	_, err = s.ConfirmShipment(ctx, "org_preview", shipment.ID, "usr_admin")
	return err
}

func (s *Store) migratePreviewSalesToUSD(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT l.id,l.unit_price_minor
		FROM sales_lines l
		JOIN sales_slips s ON s.id=l.sales_slip_id
		WHERE s.organization_id='org_preview' AND l.sale_currency='JPY'`)
	if err != nil {
		return err
	}
	type legacySaleLine struct {
		id    string
		price int64
	}
	var legacy []legacySaleLine
	for rows.Next() {
		var line legacySaleLine
		if err := rows.Scan(&line.id, &line.price); err != nil {
			rows.Close()
			return err
		}
		legacy = append(legacy, line)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, line := range legacy {
		usd := (line.price + previewSalePriceJPYPerUSD/2) / previewSalePriceJPYPerUSD
		if _, err := s.db.ExecContext(ctx, `
			UPDATE sales_lines SET unit_price_minor=?,sale_currency='USD'
			WHERE id=?`, usd, line.id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) CreateSaleDraft(ctx context.Context, input CreateSaleInput) (SalesSlip, error) {
	if input.OrganizationID == "" || input.CreatedBy == "" || strings.TrimSpace(input.CustomerName) == "" {
		return SalesSlip{}, errors.New("販売先と操作者は必須です")
	}
	if _, err := time.Parse("2006-01-02", input.SalesDate); err != nil {
		return SalesSlip{}, errors.New("売上日を正しく入力してください")
	}
	if len(input.Lines) == 0 {
		return SalesSlip{}, errors.New("売上明細を1件以上入力してください")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SalesSlip{}, err
	}
	defer tx.Rollback()
	number, err := nextTransactionNumberTx(ctx, tx, "sales_slips", "sales_date", "SL", input.OrganizationID, input.SalesDate)
	if err != nil {
		return SalesSlip{}, err
	}
	id, _ := NewID("sal")
	now := s.now().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO sales_slips(
			id,organization_id,slip_number,sales_date,customer_name,status,notes,created_by,created_at,updated_at
		) VALUES(?,?,?,?,?,'draft',?,?,?,?)`,
		id, input.OrganizationID, number, input.SalesDate, strings.TrimSpace(input.CustomerName),
		strings.TrimSpace(input.Notes), input.CreatedBy, now, now); err != nil {
		return SalesSlip{}, err
	}
	for index, line := range input.Lines {
		if line.Quantity < 1 || line.UnitPriceMinor < 0 || (line.Currency != "JPY" && line.Currency != "USD") {
			return SalesSlip{}, errors.New("数量、売価、通貨を確認してください")
		}
		var count int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM products
			WHERE id=? AND organization_id=? AND deleted_at IS NULL
			  AND inventory_status IN ('in_stock','reserved','sold','shipped')`,
			line.ProductID, input.OrganizationID).Scan(&count); err != nil || count == 0 {
			return SalesSlip{}, errors.New("販売対象の商品が見つかりません")
		}
		lineID, _ := NewID("sln")
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO sales_lines(
				id,organization_id,sales_slip_id,line_number,product_id,quantity,unit_price_minor,sale_currency,created_at
			) VALUES(?,?,?,?,?,?,?,?,?)`,
			lineID, input.OrganizationID, id, index+1, line.ProductID, line.Quantity,
			line.UnitPriceMinor, line.Currency, now); err != nil {
			return SalesSlip{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return SalesSlip{}, err
	}
	return s.Sale(ctx, input.OrganizationID, id)
}

func (s *Store) ConfirmSale(ctx context.Context, organizationID, saleID, actorID string) (SalesSlip, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SalesSlip{}, err
	}
	defer tx.Rollback()
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM sales_slips WHERE id=? AND organization_id=?`, saleID, organizationID).Scan(&status); err != nil {
		return SalesSlip{}, err
	}
	if status == "confirmed" {
		return SalesSlip{}, ErrSaleAlreadyConfirmed
	}
	if status != "draft" {
		return SalesSlip{}, errors.New("下書きの売上伝票だけ確定できます")
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id,product_id,quantity,unit_price_minor,sale_currency
		FROM sales_lines WHERE sales_slip_id=? ORDER BY line_number`, saleID)
	if err != nil {
		return SalesSlip{}, err
	}
	var lines []SalesLine
	for rows.Next() {
		var line SalesLine
		if err := rows.Scan(&line.ID, &line.ProductID, &line.Quantity, &line.UnitPriceMinor, &line.SaleCurrency); err != nil {
			rows.Close()
			return SalesSlip{}, err
		}
		lines = append(lines, line)
	}
	rows.Close()
	now := s.now().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `
		UPDATE sales_slips SET status='confirmed',confirmed_at=?,confirmed_by=?,updated_at=?
		WHERE id=? AND organization_id=? AND status='draft'`,
		now, actorID, now, saleID, organizationID); err != nil {
		return SalesSlip{}, err
	}
	for _, line := range lines {
		var other int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM sales_lines l JOIN sales_slips s ON s.id=l.sales_slip_id
			WHERE l.organization_id=? AND l.product_id=? AND s.status='confirmed' AND s.id<>?`,
			organizationID, line.ProductID, saleID).Scan(&other); err != nil {
			return SalesSlip{}, err
		}
		if other > 0 {
			return SalesSlip{}, ErrProductAlreadySold
		}
		var rateID, observed any
		var rateScaled int64
		rateScale := RateScale
		convertedUnit := line.UnitPriceMinor
		if line.SaleCurrency == "USD" {
			var observedText string
			if err := tx.QueryRowContext(ctx, `
				SELECT id,rate_scaled,scale,observed_at FROM exchange_rate_snapshots
				WHERE organization_id=? AND base_currency='USD' AND quote_currency='JPY'
				ORDER BY observed_at DESC LIMIT 1`, organizationID).
				Scan(&rateID, &rateScaled, &rateScale, &observedText); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return SalesSlip{}, errors.New("USD売上の確定には為替レートが必要です")
				}
				return SalesSlip{}, err
			}
			observed = observedText
			convertedUnit, err = ConvertMinor(line.UnitPriceMinor, rateScaled, rateScale, false)
			if err != nil {
				return SalesSlip{}, err
			}
		}
		if convertedUnit > (1<<63-1)/int64(line.Quantity) {
			return SalesSlip{}, errors.New("売上金額が大きすぎます")
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE sales_lines SET exchange_rate_snapshot_id=?,exchange_rate_scaled=?,exchange_rate_scale=?,
				exchange_rate_observed_at=?,converted_unit_price_jpy=?,converted_total_jpy=?
			WHERE id=? AND organization_id=?`,
			rateID, rateScaled, rateScale, observed, convertedUnit, convertedUnit*int64(line.Quantity),
			line.ID, organizationID); err != nil {
			return SalesSlip{}, err
		}
		var shipped int
		if err := tx.QueryRowContext(ctx, `
			SELECT COALESCE(SUM(sl.quantity),0)
			FROM shipment_lines sl JOIN shipment_slips ss ON ss.id=sl.shipment_slip_id
			WHERE sl.organization_id=? AND sl.product_id=? AND ss.status='confirmed'`,
			organizationID, line.ProductID).Scan(&shipped); err != nil {
			return SalesSlip{}, err
		}
		if shipped > line.Quantity {
			return SalesSlip{}, ErrShipmentExceedsSale
		}
		if shipped > 0 {
			shipmentRows, queryErr := tx.QueryContext(ctx, `
				SELECT sl.id,sl.quantity FROM shipment_lines sl
				JOIN shipment_slips ss ON ss.id=sl.shipment_slip_id
				WHERE sl.organization_id=? AND sl.product_id=? AND ss.status='confirmed'
				ORDER BY ss.shipment_date,ss.created_at`, organizationID, line.ProductID)
			if queryErr != nil {
				return SalesSlip{}, queryErr
			}
			for shipmentRows.Next() {
				var shipmentLineID string
				var quantity int
				if err := shipmentRows.Scan(&shipmentLineID, &quantity); err != nil {
					shipmentRows.Close()
					return SalesSlip{}, err
				}
				allocationID, _ := NewID("alc")
				if _, err := tx.ExecContext(ctx, `
					INSERT OR IGNORE INTO sales_shipment_allocations(
						id,organization_id,sales_line_id,shipment_line_id,allocated_quantity,created_at
					) VALUES(?,?,?,?,?,?)`,
					allocationID, organizationID, line.ID, shipmentLineID, quantity, now); err != nil {
					shipmentRows.Close()
					return SalesSlip{}, err
				}
			}
			shipmentRows.Close()
		}
		if err := recomputeProductTransactionStateTx(ctx, tx, organizationID, line.ProductID, actorID, "sale.confirmed", now); err != nil {
			return SalesSlip{}, err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE reservations SET status='fulfilled',released_at=?,release_reason='売上確定',updated_at=?
			WHERE organization_id=? AND product_id=? AND status='active'`,
			now, now, organizationID, line.ProductID); err != nil {
			return SalesSlip{}, err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE purchase_requests SET status='sold',updated_at=?
			WHERE organization_id=? AND product_id=? AND status='approved'`,
			now, organizationID, line.ProductID); err != nil {
			return SalesSlip{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return SalesSlip{}, err
	}
	return s.Sale(ctx, organizationID, saleID)
}

func (s *Store) CancelSale(ctx context.Context, organizationID, saleID, actorID, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return errors.New("取消理由は必須です")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM sales_slips WHERE id=? AND organization_id=?`, saleID, organizationID).Scan(&status); err != nil {
		return err
	}
	if status != "confirmed" && status != "draft" {
		return errors.New("取消できない売上伝票です")
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,product_id FROM sales_lines WHERE sales_slip_id=?`, saleID)
	if err != nil {
		return err
	}
	type item struct{ lineID, productID string }
	var items []item
	for rows.Next() {
		var value item
		if err := rows.Scan(&value.lineID, &value.productID); err != nil {
			rows.Close()
			return err
		}
		items = append(items, value)
	}
	rows.Close()
	now := s.now().Format(time.RFC3339Nano)
	for _, value := range items {
		if _, err := tx.ExecContext(ctx, `DELETE FROM sales_shipment_allocations WHERE organization_id=? AND sales_line_id=?`, organizationID, value.lineID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE sales_slips SET status='cancelled',cancelled_at=?,cancelled_by=?,cancel_reason=?,updated_at=?
		WHERE id=? AND organization_id=?`, now, actorID, strings.TrimSpace(reason), now, saleID, organizationID); err != nil {
		return err
	}
	for _, value := range items {
		if err := recomputeProductTransactionStateTx(ctx, tx, organizationID, value.productID, actorID, "sale.cancelled", now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) CreateShipmentDraft(ctx context.Context, input CreateShipmentInput) (ShipmentSlip, error) {
	if input.OrganizationID == "" || input.CreatedBy == "" || strings.TrimSpace(input.RecipientName) == "" {
		return ShipmentSlip{}, errors.New("届け先と操作者は必須です")
	}
	if _, err := time.Parse("2006-01-02", input.ShipmentDate); err != nil {
		return ShipmentSlip{}, errors.New("出荷日を正しく入力してください")
	}
	if len(input.Lines) == 0 {
		return ShipmentSlip{}, errors.New("出荷明細を1件以上入力してください")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ShipmentSlip{}, err
	}
	defer tx.Rollback()
	number, err := nextTransactionNumberTx(ctx, tx, "shipment_slips", "shipment_date", "SH", input.OrganizationID, input.ShipmentDate)
	if err != nil {
		return ShipmentSlip{}, err
	}
	id, _ := NewID("shp")
	now := s.now().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO shipment_slips(
			id,organization_id,shipment_number,shipment_date,recipient_name,status,notes,created_by,created_at,updated_at
		) VALUES(?,?,?,?,?,'draft',?,?,?,?)`,
		id, input.OrganizationID, number, input.ShipmentDate, strings.TrimSpace(input.RecipientName),
		strings.TrimSpace(input.Notes), input.CreatedBy, now, now); err != nil {
		return ShipmentSlip{}, err
	}
	for index, line := range input.Lines {
		if line.Quantity < 1 {
			return ShipmentSlip{}, errors.New("出荷数量は1以上で入力してください")
		}
		var count int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM products WHERE id=? AND organization_id=? AND deleted_at IS NULL
			  AND inventory_status IN ('in_stock','reserved','sold','shipped')`,
			line.ProductID, input.OrganizationID).Scan(&count); err != nil || count == 0 {
			return ShipmentSlip{}, errors.New("出荷対象の商品が見つかりません")
		}
		lineID, _ := NewID("shl")
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO shipment_lines(
				id,organization_id,shipment_slip_id,line_number,product_id,quantity,created_at
			) VALUES(?,?,?,?,?,?,?)`,
			lineID, input.OrganizationID, id, index+1, line.ProductID, line.Quantity, now); err != nil {
			return ShipmentSlip{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return ShipmentSlip{}, err
	}
	return s.Shipment(ctx, input.OrganizationID, id)
}

func (s *Store) ConfirmShipment(ctx context.Context, organizationID, shipmentID, actorID string) (ShipmentSlip, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ShipmentSlip{}, err
	}
	defer tx.Rollback()
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM shipment_slips WHERE id=? AND organization_id=?`, shipmentID, organizationID).Scan(&status); err != nil {
		return ShipmentSlip{}, err
	}
	if status == "confirmed" {
		return ShipmentSlip{}, ErrShipmentAlreadyConfirmed
	}
	if status != "draft" {
		return ShipmentSlip{}, errors.New("下書きの出荷伝票だけ確定できます")
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,product_id,quantity FROM shipment_lines WHERE shipment_slip_id=? ORDER BY line_number`, shipmentID)
	if err != nil {
		return ShipmentSlip{}, err
	}
	var lines []ShipmentLine
	for rows.Next() {
		var line ShipmentLine
		if err := rows.Scan(&line.ID, &line.ProductID, &line.Quantity); err != nil {
			rows.Close()
			return ShipmentSlip{}, err
		}
		lines = append(lines, line)
	}
	rows.Close()
	now := s.now().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `
		UPDATE shipment_slips SET status='confirmed',confirmed_at=?,confirmed_by=?,updated_at=?
		WHERE id=? AND organization_id=? AND status='draft'`,
		now, actorID, now, shipmentID, organizationID); err != nil {
		return ShipmentSlip{}, err
	}
	for _, line := range lines {
		var saleLineID string
		var soldQuantity, allocated int
		err := tx.QueryRowContext(ctx, `
			SELECT l.id,l.quantity,COALESCE((
				SELECT SUM(a.allocated_quantity) FROM sales_shipment_allocations a WHERE a.sales_line_id=l.id
			),0)
			FROM sales_lines l JOIN sales_slips s ON s.id=l.sales_slip_id
			WHERE l.organization_id=? AND l.product_id=? AND s.status='confirmed'
			ORDER BY s.confirmed_at DESC LIMIT 1`, organizationID, line.ProductID).
			Scan(&saleLineID, &soldQuantity, &allocated)
		if err == nil {
			if allocated+line.Quantity > soldQuantity {
				return ShipmentSlip{}, ErrShipmentExceedsSale
			}
			allocationID, _ := NewID("alc")
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO sales_shipment_allocations(
					id,organization_id,sales_line_id,shipment_line_id,allocated_quantity,created_at
				) VALUES(?,?,?,?,?,?)`,
				allocationID, organizationID, saleLineID, line.ID, line.Quantity, now); err != nil {
				return ShipmentSlip{}, err
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return ShipmentSlip{}, err
		}
		if err := recomputeProductTransactionStateTx(ctx, tx, organizationID, line.ProductID, actorID, "shipment.confirmed", now); err != nil {
			return ShipmentSlip{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return ShipmentSlip{}, err
	}
	return s.Shipment(ctx, organizationID, shipmentID)
}

func (s *Store) CancelShipment(ctx context.Context, organizationID, shipmentID, actorID, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return errors.New("取消理由は必須です")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM shipment_slips WHERE id=? AND organization_id=?`, shipmentID, organizationID).Scan(&status); err != nil {
		return err
	}
	if status != "confirmed" && status != "draft" {
		return errors.New("取消できない出荷伝票です")
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,product_id FROM shipment_lines WHERE shipment_slip_id=?`, shipmentID)
	if err != nil {
		return err
	}
	type item struct{ lineID, productID string }
	var items []item
	for rows.Next() {
		var value item
		if err := rows.Scan(&value.lineID, &value.productID); err != nil {
			rows.Close()
			return err
		}
		items = append(items, value)
	}
	rows.Close()
	for _, value := range items {
		if _, err := tx.ExecContext(ctx, `DELETE FROM sales_shipment_allocations WHERE organization_id=? AND shipment_line_id=?`, organizationID, value.lineID); err != nil {
			return err
		}
	}
	now := s.now().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `
		UPDATE shipment_slips SET status='cancelled',cancelled_at=?,cancelled_by=?,cancel_reason=?,updated_at=?
		WHERE id=? AND organization_id=?`, now, actorID, strings.TrimSpace(reason), now, shipmentID, organizationID); err != nil {
		return err
	}
	for _, value := range items {
		if err := recomputeProductTransactionStateTx(ctx, tx, organizationID, value.productID, actorID, "shipment.cancelled", now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Sales(ctx context.Context, organizationID string) ([]SalesSlip, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.id,s.organization_id,s.slip_number,s.sales_date,s.customer_name,s.status,s.notes,s.created_at,s.confirmed_at,
		       COALESCE(SUM(l.converted_total_jpy),0),
		       COALESCE(SUM(CASE WHEN l.sale_currency='USD' THEN l.unit_price_minor*l.quantity ELSE 0 END),0)
		FROM sales_slips s LEFT JOIN sales_lines l ON l.sales_slip_id=s.id
		WHERE s.organization_id=? GROUP BY s.id ORDER BY s.sales_date DESC,s.created_at DESC`, organizationID)
	if err != nil {
		return nil, err
	}
	var sales []SalesSlip
	for rows.Next() {
		sale, scanErr := scanSaleHeader(rows)
		if scanErr != nil {
			rows.Close()
			return nil, scanErr
		}
		sales = append(sales, sale)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for index := range sales {
		if sales[index].Status != "confirmed" {
			sales[index].ShipmentStatus = "not_confirmed"
			continue
		}
		status, warning, statusErr := s.saleShipmentStatus(ctx, organizationID, sales[index].ID)
		if statusErr != nil {
			return nil, statusErr
		}
		sales[index].ShipmentStatus, sales[index].Warning = status, warning
	}
	return sales, nil
}

func (s *Store) Sale(ctx context.Context, organizationID, saleID string) (SalesSlip, error) {
	sale, err := scanSaleHeader(s.db.QueryRowContext(ctx, `
		SELECT s.id,s.organization_id,s.slip_number,s.sales_date,s.customer_name,s.status,s.notes,s.created_at,s.confirmed_at,
		       COALESCE(SUM(l.converted_total_jpy),0),
		       COALESCE(SUM(CASE WHEN l.sale_currency='USD' THEN l.unit_price_minor*l.quantity ELSE 0 END),0)
		FROM sales_slips s LEFT JOIN sales_lines l ON l.sales_slip_id=s.id
		WHERE s.organization_id=? AND s.id=? GROUP BY s.id`, organizationID, saleID))
	if err != nil {
		return SalesSlip{}, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT l.id,l.product_id,p.product_code,p.brand,p.model_number,l.quantity,l.unit_price_minor,l.sale_currency,
		       COALESCE(l.exchange_rate_snapshot_id,''),l.exchange_rate_scaled,l.exchange_rate_scale,l.exchange_rate_observed_at,
		       l.converted_unit_price_jpy,l.converted_total_jpy,
		       COALESCE((SELECT SUM(a.allocated_quantity) FROM sales_shipment_allocations a
		                JOIN shipment_lines sl ON sl.id=a.shipment_line_id
		                JOIN shipment_slips ss ON ss.id=sl.shipment_slip_id
		                WHERE a.sales_line_id=l.id AND ss.status='confirmed'),0)
		FROM sales_lines l JOIN products p ON p.id=l.product_id AND p.organization_id=l.organization_id
		WHERE l.sales_slip_id=? AND l.organization_id=? ORDER BY l.line_number`, saleID, organizationID)
	if err != nil {
		return SalesSlip{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var line SalesLine
		var observed sql.NullString
		if err := rows.Scan(&line.ID, &line.ProductID, &line.ProductCode, &line.Brand, &line.ModelNumber,
			&line.Quantity, &line.UnitPriceMinor, &line.SaleCurrency, &line.ExchangeRateSnapshotID,
			&line.ExchangeRateScaled, &line.ExchangeRateScale, &observed, &line.ConvertedUnitPriceJPY,
			&line.ConvertedTotalJPY, &line.ShippedQuantity); err != nil {
			return SalesSlip{}, err
		}
		if observed.Valid {
			value, _ := time.Parse(time.RFC3339Nano, observed.String)
			line.ExchangeRateObservedAt = &value
		}
		line.RemainingQuantity = line.Quantity - line.ShippedQuantity
		sale.Lines = append(sale.Lines, line)
	}
	if sale.Status == "confirmed" {
		sale.ShipmentStatus, sale.Warning, err = s.saleShipmentStatus(ctx, organizationID, saleID)
	} else {
		sale.ShipmentStatus = "not_confirmed"
	}
	return sale, err
}

func (s *Store) Shipments(ctx context.Context, organizationID string) ([]ShipmentSlip, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id,organization_id,shipment_number,shipment_date,recipient_name,status,notes,created_at,confirmed_at
		FROM shipment_slips WHERE organization_id=? ORDER BY shipment_date DESC,created_at DESC`, organizationID)
	if err != nil {
		return nil, err
	}
	var shipments []ShipmentSlip
	for rows.Next() {
		shipment, scanErr := scanShipmentHeader(rows)
		if scanErr != nil {
			rows.Close()
			return nil, scanErr
		}
		shipments = append(shipments, shipment)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for index := range shipments {
		shipment := &shipments[index]
		var missing int
		if shipment.Status == "confirmed" {
			_ = s.db.QueryRowContext(ctx, `
				SELECT COUNT(*) FROM shipment_lines sl
				WHERE sl.shipment_slip_id=? AND NOT EXISTS(
					SELECT 1 FROM sales_shipment_allocations a WHERE a.shipment_line_id=sl.id
				)`, shipment.ID).Scan(&missing)
		}
		if missing > 0 {
			shipment.Warning = "出荷済み・売上未確定"
		}
	}
	return shipments, nil
}

func (s *Store) Shipment(ctx context.Context, organizationID, shipmentID string) (ShipmentSlip, error) {
	shipment, err := scanShipmentHeader(s.db.QueryRowContext(ctx, `
		SELECT id,organization_id,shipment_number,shipment_date,recipient_name,status,notes,created_at,confirmed_at
		FROM shipment_slips WHERE organization_id=? AND id=?`, organizationID, shipmentID))
	if err != nil {
		return ShipmentSlip{}, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT sl.id,sl.product_id,p.product_code,p.brand,p.model_number,sl.quantity,
		       COALESCE(a.sales_line_id,''),COALESCE(ss.slip_number,''),COALESCE(a.allocated_quantity,0)
		FROM shipment_lines sl
		JOIN products p ON p.id=sl.product_id AND p.organization_id=sl.organization_id
		LEFT JOIN sales_shipment_allocations a ON a.shipment_line_id=sl.id
		LEFT JOIN sales_lines sal ON sal.id=a.sales_line_id
		LEFT JOIN sales_slips ss ON ss.id=sal.sales_slip_id AND ss.status='confirmed'
		WHERE sl.shipment_slip_id=? AND sl.organization_id=? ORDER BY sl.line_number`,
		shipmentID, organizationID)
	if err != nil {
		return ShipmentSlip{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var line ShipmentLine
		if err := rows.Scan(&line.ID, &line.ProductID, &line.ProductCode, &line.Brand, &line.ModelNumber,
			&line.Quantity, &line.SalesLineID, &line.SalesSlipNumber, &line.AllocatedQuantity); err != nil {
			return ShipmentSlip{}, err
		}
		if shipment.Status == "confirmed" && line.SalesLineID == "" {
			line.Warning = "売上未確定"
			shipment.Warning = "出荷済み・売上未確定"
		}
		shipment.Lines = append(shipment.Lines, line)
	}
	return shipment, rows.Err()
}

func (s *Store) TransactionProducts(ctx context.Context, organizationID string) ([]Product, error) {
	return s.Products(ctx, organizationID, ProductFilter{})
}

func (s *Store) saleShipmentStatus(ctx context.Context, organizationID, saleID string) (string, string, error) {
	var sold, shipped int
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(l.quantity),0),COALESCE(SUM((
			SELECT COALESCE(SUM(a.allocated_quantity),0) FROM sales_shipment_allocations a
			JOIN shipment_lines sl ON sl.id=a.shipment_line_id
			JOIN shipment_slips ss ON ss.id=sl.shipment_slip_id
			WHERE a.sales_line_id=l.id AND ss.status='confirmed'
		)),0)
		FROM sales_lines l WHERE l.organization_id=? AND l.sales_slip_id=?`,
		organizationID, saleID).Scan(&sold, &shipped)
	if err != nil {
		return "", "", err
	}
	if shipped == 0 {
		return "unshipped", "売上済み・未出荷", nil
	}
	if shipped < sold {
		return "partial", "", nil
	}
	return "complete", "", nil
}

func recomputeProductTransactionStateTx(ctx context.Context, tx *sql.Tx, organizationID, productID, actorID, eventType, now string) error {
	var fromStatus string
	if err := tx.QueryRowContext(ctx, `SELECT inventory_status FROM products WHERE organization_id=? AND id=?`, organizationID, productID).Scan(&fromStatus); err != nil {
		return err
	}
	var sold, shipped int
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(l.quantity),0) FROM sales_lines l JOIN sales_slips s ON s.id=l.sales_slip_id
		WHERE l.organization_id=? AND l.product_id=? AND s.status='confirmed'`,
		organizationID, productID).Scan(&sold); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(l.quantity),0) FROM shipment_lines l JOIN shipment_slips s ON s.id=l.shipment_slip_id
		WHERE l.organization_id=? AND l.product_id=? AND s.status='confirmed'`,
		organizationID, productID).Scan(&shipped); err != nil {
		return err
	}
	toStatus := "in_stock"
	if shipped > 0 && (sold == 0 || shipped >= sold) {
		toStatus = "shipped"
	} else if sold > 0 {
		toStatus = "sold"
	}
	if fromStatus == toStatus {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE products SET inventory_status=?,updated_at=? WHERE organization_id=? AND id=?`, toStatus, now, organizationID, productID); err != nil {
		return err
	}
	eventID, _ := NewID("evt")
	_, err := tx.ExecContext(ctx, `
		INSERT INTO inventory_events(id,organization_id,product_id,event_type,from_status,to_status,actor_user_id,created_at)
		VALUES(?,?,?,?,?,?,?,?)`, eventID, organizationID, productID, eventType, fromStatus, toStatus, actorID, now)
	return err
}

func nextTransactionNumberTx(ctx context.Context, tx *sql.Tx, table, dateColumn, prefix, organizationID, date string) (string, error) {
	if table != "sales_slips" && table != "shipment_slips" {
		return "", errors.New("unsupported transaction table")
	}
	query := fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE organization_id=? AND substr(%s,1,4)=?`, table, dateColumn)
	var count int
	if err := tx.QueryRowContext(ctx, query, organizationID, date[:4]).Scan(&count); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s-%04d", prefix, date[:4], count+1), nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSaleHeader(row rowScanner) (SalesSlip, error) {
	var sale SalesSlip
	var created string
	var confirmed sql.NullString
	err := row.Scan(&sale.ID, &sale.OrganizationID, &sale.SlipNumber, &sale.SalesDate, &sale.CustomerName,
		&sale.Status, &sale.Notes, &created, &confirmed, &sale.TotalJPY, &sale.TotalUSD)
	if err != nil {
		return SalesSlip{}, err
	}
	sale.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	if confirmed.Valid {
		value, _ := time.Parse(time.RFC3339Nano, confirmed.String)
		sale.ConfirmedAt = &value
	}
	return sale, nil
}

func scanShipmentHeader(row rowScanner) (ShipmentSlip, error) {
	var shipment ShipmentSlip
	var created string
	var confirmed sql.NullString
	err := row.Scan(&shipment.ID, &shipment.OrganizationID, &shipment.ShipmentNumber, &shipment.ShipmentDate,
		&shipment.RecipientName, &shipment.Status, &shipment.Notes, &created, &confirmed)
	if err != nil {
		return ShipmentSlip{}, err
	}
	shipment.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	if confirmed.Valid {
		value, _ := time.Parse(time.RFC3339Nano, confirmed.String)
		shipment.ConfirmedAt = &value
	}
	return shipment, nil
}
