package database

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type SalesRevision struct {
	ID        string
	ActorName string
	Memo      string
	CreatedAt time.Time
}

type SalesEditLine struct {
	LineID         string
	SalePriceMinor int64
}

type UpdateSalesSlipInput struct {
	OrganizationID string
	SalesSlipID    string
	SalesDate      string
	Notes          string
	Memo           string
	ActorUserID    string
	Lines          []SalesEditLine
}

type CreateSalesReturnInput struct {
	OrganizationID string
	SalesSlipID    string
	SalesLineIDs   []string
	ReturnDate     string
	Reason         string
	Notes          string
	ActorUserID    string
}

func (s *Store) SalesRevisions(ctx context.Context, organizationID, salesSlipID string) ([]SalesRevision, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.id,u.display_name,r.memo,r.created_at
		FROM sales_slip_revisions r
		JOIN users u ON u.id=r.actor_user_id
		WHERE r.organization_id=? AND r.sales_slip_id=?
		ORDER BY r.created_at DESC`, organizationID, salesSlipID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var revisions []SalesRevision
	for rows.Next() {
		var revision SalesRevision
		var createdAt string
		if err := rows.Scan(&revision.ID, &revision.ActorName, &revision.Memo, &createdAt); err != nil {
			return nil, err
		}
		revision.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		revisions = append(revisions, revision)
	}
	return revisions, rows.Err()
}

func (s *Store) UpdateSalesSlip(ctx context.Context, input UpdateSalesSlipInput) error {
	input.SalesDate = strings.TrimSpace(input.SalesDate)
	input.Memo = strings.TrimSpace(input.Memo)
	if input.SalesDate == "" || input.Memo == "" {
		return errors.New("売上日と修正メモを入力してください")
	}
	if _, err := time.Parse("2006-01-02", input.SalesDate); err != nil {
		return errors.New("売上日を正しく入力してください")
	}
	before, err := s.Sale(ctx, input.OrganizationID, input.SalesSlipID)
	if err != nil {
		return err
	}
	beforeJSON, _ := json.Marshal(before)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := s.now().Format(time.RFC3339Nano)
	result, err := tx.ExecContext(ctx, `
		UPDATE sales_slips SET sales_date=?,notes=?,updated_at=?
		WHERE id=? AND organization_id=?`,
		input.SalesDate, strings.TrimSpace(input.Notes), now,
		input.SalesSlipID, input.OrganizationID)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return errors.New("売上伝票が見つかりません")
	}
	for _, line := range input.Lines {
		if line.SalePriceMinor < 0 {
			return errors.New("販売金額は0以上で入力してください")
		}
		result, err = tx.ExecContext(ctx, `
			UPDATE sales_lines
			SET unit_price_minor=?,
			    converted_unit_price_jpy=CASE WHEN sale_currency='JPY' THEN ? ELSE converted_unit_price_jpy END,
			    converted_total_jpy=CASE WHEN sale_currency='JPY' THEN ?*quantity ELSE converted_total_jpy END
			WHERE id=? AND organization_id=? AND sales_slip_id=?`,
			line.SalePriceMinor, line.SalePriceMinor, line.SalePriceMinor,
			line.LineID, input.OrganizationID, input.SalesSlipID)
		if err != nil {
			return err
		}
		affected, _ = result.RowsAffected()
		if affected != 1 {
			return errors.New("修正対象の商品明細が見つかりません")
		}
	}
	afterJSON, _ := json.Marshal(input)
	revisionID, _ := NewID("sarev")
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO sales_slip_revisions(
			id,organization_id,sales_slip_id,actor_user_id,memo,before_json,after_json,created_at
		) VALUES(?,?,?,?,?,?,?,?)`,
		revisionID, input.OrganizationID, input.SalesSlipID, input.ActorUserID,
		input.Memo, string(beforeJSON), string(afterJSON), now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) CreateSalesReturn(ctx context.Context, input CreateSalesReturnInput) ([]string, error) {
	input.ReturnDate = strings.TrimSpace(input.ReturnDate)
	input.Reason = strings.TrimSpace(input.Reason)
	if len(input.SalesLineIDs) == 0 {
		return nil, errors.New("返品対象商品を1点以上選択してください")
	}
	if _, err := time.Parse("2006-01-02", input.ReturnDate); err != nil {
		return nil, errors.New("返品日を正しく入力してください")
	}
	if input.Reason == "" {
		return nil, errors.New("返品理由を選択してください")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	now := s.now().Format(time.RFC3339Nano)
	created := make([]string, 0, len(input.SalesLineIDs))
	seen := make(map[string]bool, len(input.SalesLineIDs))
	for _, lineID := range input.SalesLineIDs {
		if seen[lineID] {
			continue
		}
		seen[lineID] = true
		var quantity, pending int
		if err := tx.QueryRowContext(ctx, `
			SELECT sl.quantity FROM sales_lines sl
			JOIN sales_slips ss ON ss.id=sl.sales_slip_id AND ss.organization_id=sl.organization_id
			WHERE sl.id=? AND ss.id=? AND ss.organization_id=? AND ss.status='confirmed'`,
			lineID, input.SalesSlipID, input.OrganizationID).Scan(&quantity); err != nil {
			return nil, ErrReturnNotEligible
		}
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM return_takehome_items
			WHERE organization_id=? AND sales_line_id=? AND action_type='return' AND status='pending'`,
			input.OrganizationID, lineID).Scan(&pending); err != nil {
			return nil, err
		}
		if pending > 0 {
			return nil, ErrReturnAlreadyPending
		}
		id, _ := NewID("rtn")
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO return_takehome_items(
				id,organization_id,sales_slip_id,sales_line_id,action_type,status,quantity,reason,notes,
				requested_by,requested_at,created_at,updated_at,return_date
			) VALUES(?,?,?,?,'return','pending',?,?,?,?,?,?,?,?)`,
			id, input.OrganizationID, input.SalesSlipID, lineID, quantity, input.Reason,
			strings.TrimSpace(input.Notes), input.ActorUserID, now, now, now, input.ReturnDate); err != nil {
			return nil, err
		}
		created = append(created, id)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return created, nil
}

func (s *Store) SeedSalesWorkflowPreview(ctx context.Context) error {
	now := s.now().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
		UPDATE sales_slips
		SET customer_address=CASE WHEN customer_address='' THEN '東京都新宿区' ELSE customer_address END,
		    customer_phone=CASE WHEN customer_phone='' THEN '03-9999-0000' ELSE customer_phone END,
		    qualified_invoice_number=CASE WHEN qualified_invoice_number='' THEN 'T7777888899' ELSE qualified_invoice_number END,
		    updated_at=?
		WHERE organization_id='org_preview'`, now)
	return err
}
