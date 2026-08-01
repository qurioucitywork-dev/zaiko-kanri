package database

import (
	"context"
	"errors"
	"time"
)

var ErrInvalidPerformanceMode = errors.New("未対応の実績分類です")

type PerformanceRecord struct {
	Name      string
	Count     int
	AmountJPY int64
}

// BuyerPerformanceRecord is the purchase-to-sale performance of one purchase
// staff member for the requested business period. Monetary values are JPY.
type BuyerPerformanceRecord struct {
	Name               string
	PurchaseCount      int
	PurchaseAmountJPY  int64
	SalesCount         int
	RevenueJPY         int64
	ExpectedProfitJPY  int64
	ActualProfitJPY    int64
	InventoryAmountJPY int64
}

// SalesDestinationPerformanceRecord is the sales and gross-profit performance
// of one sales destination for the requested business period. Monetary values
// are normalized to JPY using the stored USD/JPY master rate.
type SalesDestinationPerformanceRecord struct {
	Name              string
	SalesCount        int
	RevenueJPY        int64
	CostJPY           int64
	ExpectedProfitJPY int64
	ActualProfitJPY   int64
}

func (s *Store) SalesDestinationPerformance(ctx context.Context, organizationID, dateFrom, dateTo string) ([]SalesDestinationPerformanceRecord, error) {
	if _, err := time.Parse("2006-01-02", dateFrom); err != nil {
		return nil, errors.New("集計開始日を正しく入力してください")
	}
	if _, err := time.Parse("2006-01-02", dateTo); err != nil || dateFrom > dateTo {
		return nil, errors.New("集計終了日を正しく入力してください")
	}
	rows, err := s.db.QueryContext(ctx, `
		WITH latest_rate AS (
			SELECT COALESCE((
				SELECT rate_scaled FROM exchange_rate_snapshots
				WHERE organization_id=? AND base_currency='USD' AND quote_currency='JPY'
				ORDER BY observed_at DESC LIMIT 1
			),0) AS usd_jpy
		), sales_base AS (
			SELECT ss.customer_name,
			       sl.quantity,
			       sl.converted_total_jpy AS revenue_jpy,
			       CASE WHEN p.cost_currency='JPY' THEN p.cost_amount_minor
			            WHEN p.cost_currency='USD' THEN p.cost_amount_minor*(SELECT usd_jpy FROM latest_rate)/100000000
			            ELSE 0 END AS unit_cost_jpy,
			       CASE WHEN p.base_sale_currency='JPY' THEN p.base_sale_price_minor
			            WHEN p.base_sale_currency='USD' THEN p.base_sale_price_minor*(SELECT usd_jpy FROM latest_rate)/100000000
			            ELSE 0 END AS expected_unit_sale_jpy
			FROM sales_slips ss
			JOIN sales_lines sl ON sl.sales_slip_id=ss.id
			JOIN products p ON p.id=sl.product_id
			WHERE ss.organization_id=? AND ss.sales_date BETWEEN ? AND ?
			  AND ss.status='confirmed' AND p.deleted_at IS NULL
		)
		SELECT customer_name,
		       COALESCE(SUM(quantity),0) AS sales_count,
		       COALESCE(SUM(revenue_jpy),0) AS revenue_jpy,
		       COALESCE(SUM(unit_cost_jpy*quantity),0) AS cost_jpy,
		       COALESCE(SUM(CASE WHEN expected_unit_sale_jpy>unit_cost_jpy
		                         THEN (expected_unit_sale_jpy-unit_cost_jpy)*quantity ELSE 0 END),0) AS expected_profit_jpy,
		       COALESCE(SUM(revenue_jpy-unit_cost_jpy*quantity),0) AS actual_profit_jpy
		FROM sales_base
		GROUP BY customer_name
		ORDER BY revenue_jpy DESC,customer_name`, organizationID, organizationID, dateFrom, dateTo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]SalesDestinationPerformanceRecord, 0)
	for rows.Next() {
		var record SalesDestinationPerformanceRecord
		if err := rows.Scan(&record.Name, &record.SalesCount, &record.RevenueJPY, &record.CostJPY,
			&record.ExpectedProfitJPY, &record.ActualProfitJPY); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Store) BuyerPerformance(ctx context.Context, organizationID, dateFrom, dateTo string) ([]BuyerPerformanceRecord, error) {
	if _, err := time.Parse("2006-01-02", dateFrom); err != nil {
		return nil, errors.New("集計開始日を正しく入力してください")
	}
	if _, err := time.Parse("2006-01-02", dateTo); err != nil || dateFrom > dateTo {
		return nil, errors.New("集計終了日を正しく入力してください")
	}
	rows, err := s.db.QueryContext(ctx, `
		WITH latest_rate AS (
			SELECT COALESCE((
				SELECT rate_scaled FROM exchange_rate_snapshots
				WHERE organization_id=? AND base_currency='USD' AND quote_currency='JPY'
				ORDER BY observed_at DESC LIMIT 1
			),0) AS usd_jpy
		), product_base AS (
			SELECT p.id AS product_id,
			       COALESCE(NULLIF(u.display_name,''),NULLIF(u.username,''),'未設定') AS staff_name,
			       CASE WHEN p.cost_currency='JPY' THEN p.cost_amount_minor
			            WHEN p.cost_currency='USD' THEN p.cost_amount_minor*(SELECT usd_jpy FROM latest_rate)/100000000
			            ELSE 0 END AS cost_jpy,
			       CASE WHEN p.base_sale_currency='JPY' THEN p.base_sale_price_minor
			            WHEN p.base_sale_currency='USD' THEN p.base_sale_price_minor*(SELECT usd_jpy FROM latest_rate)/100000000
			            ELSE 0 END AS expected_sale_jpy
			FROM products p
			JOIN purchase_slip_lines pl ON pl.id=p.purchase_slip_line_id
			JOIN purchase_slips ps ON ps.id=pl.purchase_slip_id
			LEFT JOIN users u ON u.id=ps.created_by
			WHERE p.organization_id=? AND p.purchase_date BETWEEN ? AND ?
			  AND p.deleted_at IS NULL AND p.inventory_status NOT IN ('cancelled','invalid')
		), sale_by_product AS (
			SELECT sl.product_id,SUM(sl.converted_total_jpy) AS revenue_jpy
			FROM sales_lines sl
			JOIN sales_slips ss ON ss.id=sl.sales_slip_id
			WHERE ss.organization_id=? AND ss.sales_date BETWEEN ? AND ? AND ss.status='confirmed'
			GROUP BY sl.product_id
		)
		SELECT pb.staff_name,
		       COUNT(pb.product_id) AS purchase_count,
		       COALESCE(SUM(pb.cost_jpy),0) AS purchase_amount_jpy,
		       COALESCE(SUM(CASE WHEN sb.product_id IS NOT NULL THEN 1 ELSE 0 END),0) AS sales_count,
		       COALESCE(SUM(COALESCE(sb.revenue_jpy,0)),0) AS revenue_jpy,
		       COALESCE(SUM(CASE WHEN pb.expected_sale_jpy>pb.cost_jpy THEN pb.expected_sale_jpy-pb.cost_jpy ELSE 0 END),0) AS expected_profit_jpy,
		       COALESCE(SUM(CASE WHEN sb.product_id IS NOT NULL THEN sb.revenue_jpy-pb.cost_jpy ELSE 0 END),0) AS actual_profit_jpy,
		       COALESCE(SUM(CASE WHEN sb.product_id IS NULL THEN pb.cost_jpy ELSE 0 END),0) AS inventory_amount_jpy
		FROM product_base pb
		LEFT JOIN sale_by_product sb ON sb.product_id=pb.product_id
		GROUP BY pb.staff_name
		ORDER BY purchase_amount_jpy DESC,pb.staff_name`,
		organizationID, organizationID, dateFrom, dateTo, organizationID, dateFrom, dateTo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]BuyerPerformanceRecord, 0)
	for rows.Next() {
		var record BuyerPerformanceRecord
		if err := rows.Scan(&record.Name, &record.PurchaseCount, &record.PurchaseAmountJPY,
			&record.SalesCount, &record.RevenueJPY, &record.ExpectedProfitJPY,
			&record.ActualProfitJPY, &record.InventoryAmountJPY); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *Store) Performance(ctx context.Context, organizationID, mode, dateFrom, dateTo string) ([]PerformanceRecord, error) {
	if _, err := time.Parse("2006-01-02", dateFrom); err != nil {
		return nil, errors.New("集計開始日を正しく入力してください")
	}
	if _, err := time.Parse("2006-01-02", dateTo); err != nil || dateFrom > dateTo {
		return nil, errors.New("集計終了日を正しく入力してください")
	}
	var (
		query string
		args  []any
	)
	switch mode {
	case "suppliers":
		query = `
			SELECT s.name,COUNT(p.id),
			       COALESCE(SUM(CASE
			         WHEN p.cost_currency='JPY' THEN p.cost_amount_minor
			         WHEN p.cost_currency='USD' THEN p.cost_amount_minor * COALESCE((
			           SELECT er.rate_scaled FROM exchange_rate_snapshots er
			           WHERE er.organization_id=p.organization_id
			             AND er.base_currency='USD' AND er.quote_currency='JPY'
			           ORDER BY er.observed_at DESC LIMIT 1
			         ),0) / 100000000
			         ELSE 0 END),0) AS amount_jpy
			FROM products p
			JOIN suppliers s ON s.id=p.supplier_id
			WHERE p.organization_id=? AND p.purchase_date BETWEEN ? AND ?
			  AND p.deleted_at IS NULL AND p.inventory_status NOT IN ('cancelled','invalid')
			GROUP BY s.id,s.name
			ORDER BY amount_jpy DESC,s.name`
		args = []any{organizationID, dateFrom, dateTo}
	case "buyers":
		query = `
			SELECT COALESCE(NULLIF(u.display_name,''),u.username),COUNT(p.id),
			       COALESCE(SUM(CASE
			         WHEN p.cost_currency='JPY' THEN p.cost_amount_minor
			         WHEN p.cost_currency='USD' THEN p.cost_amount_minor * COALESCE((
			           SELECT er.rate_scaled FROM exchange_rate_snapshots er
			           WHERE er.organization_id=p.organization_id
			             AND er.base_currency='USD' AND er.quote_currency='JPY'
			           ORDER BY er.observed_at DESC LIMIT 1
			         ),0) / 100000000
			         ELSE 0 END),0) AS amount_jpy
			FROM products p
			JOIN purchase_slip_lines pl ON pl.id=p.purchase_slip_line_id
			JOIN purchase_slips ps ON ps.id=pl.purchase_slip_id
			JOIN users u ON u.id=ps.created_by
			WHERE p.organization_id=? AND p.purchase_date BETWEEN ? AND ?
			  AND p.deleted_at IS NULL AND p.inventory_status NOT IN ('cancelled','invalid')
			GROUP BY u.id,u.display_name,u.username
			ORDER BY amount_jpy DESC,u.display_name`
		args = []any{organizationID, dateFrom, dateTo}
	case "sales-destinations":
		query = `
			SELECT ss.customer_name,COALESCE(SUM(sl.quantity),0),
			       COALESCE(SUM(sl.converted_total_jpy),0) AS amount_jpy
			FROM sales_slips ss
			JOIN sales_lines sl ON sl.sales_slip_id=ss.id
			WHERE ss.organization_id=? AND ss.sales_date BETWEEN ? AND ? AND ss.status='confirmed'
			GROUP BY ss.customer_name
			ORDER BY amount_jpy DESC,ss.customer_name`
		args = []any{organizationID, dateFrom, dateTo}
	default:
		return nil, ErrInvalidPerformanceMode
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]PerformanceRecord, 0)
	for rows.Next() {
		var record PerformanceRecord
		if err := rows.Scan(&record.Name, &record.Count, &record.AmountJPY); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}
