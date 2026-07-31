package database

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type PurchaseReturnLine struct {
	ProductID   string
	ProductCode string
	SKU         string
	Brand       string
	ModelNumber string
	AmountJPY   int64
	Status      string
}

func (s *Store) PurchaseReturn(ctx context.Context, organizationID, id string) (PurchaseReturnSlip, error) {
	var item PurchaseReturnSlip
	var createdAt string
	var invoiceIssuedAt, invoicePrintedAt *string
	err := s.db.QueryRowContext(ctx, `
		SELECT r.id,COALESCE(r.purchase_slip_id,''),r.return_number,r.return_date,r.supplier_name,
		       r.item_count,r.amount_jpy,r.reason,r.status,r.delivery_number,r.notes,
		       COALESCE(u.display_name,'—'),r.created_at,r.invoice_issued_at,r.invoice_printed_at
		FROM purchase_return_slips r
		LEFT JOIN users u ON u.id=r.created_by
		WHERE r.organization_id=? AND r.id=?`, organizationID, id).Scan(
		&item.ID, &item.PurchaseSlipID, &item.ReturnNumber, &item.ReturnDate, &item.SupplierName,
		&item.ItemCount, &item.AmountJPY, &item.Reason, &item.Status, &item.DeliveryNumber,
		&item.Notes, &item.CreatedByName, &createdAt, &invoiceIssuedAt, &invoicePrintedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return PurchaseReturnSlip{}, errors.New("仕入返品伝票が見つかりません")
	}
	if err != nil {
		return PurchaseReturnSlip{}, err
	}
	item.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	item.InvoiceIssuedAt = parseOptionalTime(invoiceIssuedAt)
	item.InvoicePrintedAt = parseOptionalTime(invoicePrintedAt)
	return item, nil
}

func (s *Store) PurchaseReturnLines(ctx context.Context, organizationID string, slip PurchaseReturnSlip) ([]PurchaseReturnLine, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT l.product_id,l.product_code,l.sku,l.brand,l.model_number,l.amount_jpy,
		       COALESCE(p.inventory_status,'cancelled')
		FROM purchase_return_lines l
		LEFT JOIN products p ON p.id=l.product_id AND p.organization_id=l.organization_id
		WHERE l.organization_id=? AND l.purchase_return_slip_id=?
		ORDER BY l.product_code`, organizationID, slip.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var lines []PurchaseReturnLine
	for rows.Next() {
		var line PurchaseReturnLine
		if err := rows.Scan(&line.ProductID, &line.ProductCode, &line.SKU, &line.Brand,
			&line.ModelNumber, &line.AmountJPY, &line.Status); err != nil {
			return nil, err
		}
		lines = append(lines, line)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(lines) > 0 || slip.PurchaseSlipID == "" {
		return lines, nil
	}
	products, err := s.PurchaseProducts(ctx, organizationID, slip.PurchaseSlipID)
	if err != nil {
		return nil, err
	}
	limit := slip.ItemCount
	if limit > len(products) {
		limit = len(products)
	}
	lines = make([]PurchaseReturnLine, 0, limit)
	for _, product := range products[:limit] {
		lines = append(lines, PurchaseReturnLine{
			ProductID: product.ProductID, ProductCode: product.ProductCode, SKU: product.SKU,
			Brand: product.Brand, ModelNumber: product.ModelNumber,
			AmountJPY: product.CostAmountMinor, Status: product.InventoryStatus,
		})
	}
	return lines, nil
}

func (s *Store) IssuePurchaseReturnInvoice(ctx context.Context, organizationID, id, actorUserID string) error {
	now := s.now().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, `
		UPDATE purchase_return_slips
		SET invoice_issued_at=?,invoice_issued_by=?,invoice_printed_at=?,invoice_printed_by=?,updated_at=?
		WHERE organization_id=? AND id=?`,
		now, actorUserID, now, actorUserID, now, organizationID, id)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return errors.New("仕入返品伝票が見つかりません")
	}
	return nil
}

func (s *Store) CompletePurchaseReturn(ctx context.Context, organizationID, id string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE purchase_return_slips SET status='completed',updated_at=?
		WHERE organization_id=? AND id=? AND status<>'completed'`,
		s.now().Format(time.RFC3339Nano), organizationID, id)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		var count int
		if err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM purchase_return_slips WHERE organization_id=? AND id=?`,
			organizationID, id).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			return errors.New("仕入返品伝票が見つかりません")
		}
	}
	return nil
}
