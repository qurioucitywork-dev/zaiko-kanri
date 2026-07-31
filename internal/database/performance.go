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
