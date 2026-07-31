package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type PurchaseReturnSlip struct {
	ID               string
	PurchaseSlipID   string
	ReturnNumber     string
	ReturnDate       string
	SupplierName     string
	ItemCount        int
	AmountJPY        int64
	Reason           string
	Status           string
	DeliveryNumber   string
	Notes            string
	CreatedByName    string
	CreatedAt        time.Time
	InvoiceIssuedAt  *time.Time
	InvoicePrintedAt *time.Time
}

func (s *Store) PurchaseReturnSlips(ctx context.Context, organizationID string) ([]PurchaseReturnSlip, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.id,COALESCE(r.purchase_slip_id,''),r.return_number,r.return_date,r.supplier_name,r.item_count,
		       r.amount_jpy,r.reason,r.status,r.delivery_number,r.notes,COALESCE(u.display_name,'—'),
		       r.created_at,r.invoice_issued_at,r.invoice_printed_at
		FROM purchase_return_slips r
		LEFT JOIN users u ON u.id=r.created_by
		WHERE r.organization_id=? ORDER BY r.return_date DESC,r.return_number DESC`, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []PurchaseReturnSlip
	for rows.Next() {
		var item PurchaseReturnSlip
		var createdAt string
		var invoiceIssuedAt, invoicePrintedAt *string
		if err := rows.Scan(&item.ID, &item.PurchaseSlipID, &item.ReturnNumber, &item.ReturnDate,
			&item.SupplierName, &item.ItemCount, &item.AmountJPY, &item.Reason, &item.Status,
			&item.DeliveryNumber, &item.Notes, &item.CreatedByName, &createdAt,
			&invoiceIssuedAt, &invoicePrintedAt); err != nil {
			return nil, err
		}
		item.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		item.InvoiceIssuedAt = parseOptionalTime(invoiceIssuedAt)
		item.InvoicePrintedAt = parseOptionalTime(invoicePrintedAt)
		result = append(result, item)
	}
	return result, rows.Err()
}

func parseOptionalTime(value *string) *time.Time {
	if value == nil {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, *value)
	if err != nil {
		return nil
	}
	return &parsed
}

func (s *Store) UpdatePurchaseReturnDelivery(ctx context.Context, organizationID, id, deliveryNumber string) error {
	deliveryNumber = strings.TrimSpace(deliveryNumber)
	if deliveryNumber == "" {
		return errors.New("配送番号を入力してください")
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE purchase_return_slips SET delivery_number=?,updated_at=?
		WHERE id=? AND organization_id=?`, deliveryNumber, s.now().Format(time.RFC3339Nano), id, organizationID)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return errors.New("仕入返品伝票が見つかりません")
	}
	return nil
}

func (s *Store) SeedPurchaseReturnPreview(ctx context.Context) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM purchase_return_slips WHERE organization_id='org_preview'`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	purchases, err := s.PurchaseSlips(ctx, "org_preview")
	if err != nil {
		return err
	}
	if len(purchases) == 0 {
		return nil
	}
	reasons := []string{"商品不良", "注文違い", "品質不適合", "説明と相違", "シリアル番号不一致", "商品説明相違", "梱包不良", "不良品"}
	statuses := []string{"completed", "completed", "pending", "pending", "pending", "returned", "returned", "returned"}
	now := s.now().Format(time.RFC3339Nano)
	limit := len(purchases)
	if limit > len(reasons) {
		limit = len(reasons)
	}
	for index := 0; index < limit; index++ {
		purchase := purchases[len(purchases)-1-index]
		id, _ := NewID("pret")
		number := fmt.Sprintf("PR-RET-%04d", index+1)
		date := fmt.Sprintf("2026-04-%02d", []int{20, 2, 10, 14, 16, 15, 17, 17}[index])
		itemCount := 1
		if index == 1 {
			itemCount = 2
		}
		if _, err := s.db.ExecContext(ctx, `
			INSERT INTO purchase_return_slips(
				id,organization_id,purchase_slip_id,return_number,return_date,supplier_name,item_count,
				amount_jpy,reason,status,delivery_number,created_at,updated_at
			) VALUES(?,?,?,?,?,?,?,?,?,?,?, ?,?)`,
			id, "org_preview", purchase.ID, number, date, purchase.SupplierName, itemCount,
			purchase.TotalMinor, reasons[index], statuses[index], "", now, now); err != nil {
			return err
		}
	}
	return nil
}
