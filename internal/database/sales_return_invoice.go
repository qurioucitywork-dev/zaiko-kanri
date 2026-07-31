package database

import (
	"context"
	"errors"
	"time"
)

var ErrSalesReturnInvoiceRequired = errors.New("請求書を発行・印刷してから完了してください")

func (s *Store) IssueSalesReturnInvoice(ctx context.Context, organizationID, saleID, actorID string) error {
	now := s.now().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, `
		UPDATE return_takehome_items
		SET invoice_issued_at=COALESCE(invoice_issued_at,?),
		    invoice_issued_by=COALESCE(invoice_issued_by,?),
		    invoice_printed_at=?,invoice_printed_by=?,updated_at=?
		WHERE organization_id=? AND sales_slip_id=? AND action_type='return' AND status<>'cancelled'`,
		now, actorID, now, actorID, now, organizationID, saleID)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return errors.New("売上返品伝票が見つかりません")
	}
	return nil
}

func (s *Store) CompleteSalesReturn(ctx context.Context, organizationID, saleID, actorID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var total, unprinted int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*),COALESCE(SUM(CASE WHEN invoice_printed_at IS NULL THEN 1 ELSE 0 END),0)
		FROM return_takehome_items
		WHERE organization_id=? AND sales_slip_id=? AND action_type='return' AND status<>'cancelled'`,
		organizationID, saleID).Scan(&total, &unprinted); err != nil {
		return err
	}
	if total == 0 {
		return errors.New("売上返品伝票が見つかりません")
	}
	if unprinted > 0 {
		return ErrSalesReturnInvoiceRequired
	}
	now := s.now().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `
		UPDATE return_takehome_items
		SET status='completed',processed_by=?,processed_at=?,updated_at=?
		WHERE organization_id=? AND sales_slip_id=? AND action_type='return' AND status='pending'`,
		actorID, now, now, organizationID, saleID); err != nil {
		return err
	}
	return tx.Commit()
}

func SalesReturnInvoiceReady(items []ReturnTakehomeItem) bool {
	hasReturn := false
	for _, item := range items {
		if item.ActionType != "return" || item.Status == "cancelled" {
			continue
		}
		hasReturn = true
		if item.InvoicePrintedAt == nil {
			return false
		}
	}
	return hasReturn
}

func SalesReturnCompleted(items []ReturnTakehomeItem) bool {
	hasReturn := false
	for _, item := range items {
		if item.ActionType != "return" || item.Status == "cancelled" {
			continue
		}
		hasReturn = true
		if item.Status != "completed" {
			return false
		}
	}
	return hasReturn
}
