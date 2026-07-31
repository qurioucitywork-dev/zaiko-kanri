package database

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"
)

var (
	ErrReturnInvalidQuantity  = errors.New("対象数量を正しく入力してください")
	ErrReturnNotEligible      = errors.New("確定済みの売上明細のみ対象にできます")
	ErrReturnAlreadyPending   = errors.New("同じ区分の処理待ち案件がすでにあります")
	ErrReturnAlreadyHandled   = errors.New("この案件はすでに処理されています")
	ErrReturnRestoreSelection = errors.New("在庫に戻す商品を選択してください")
	ErrReturnRestoreCondition = errors.New("確認後のコンディションを選択してください")
)

type ReturnTakehomeSummary struct {
	SaleID       string
	SlipNumber   string
	SalesDate    string
	CustomerName string
	ItemCount    int
	PendingCount int
	TotalJPY     int64
}

type ReturnTakehomeItem struct {
	ID                  string
	SaleID              string
	SalesLineID         string
	ProductID           string
	ProductCode         string
	Brand               string
	ModelNumber         string
	SerialNumber        string
	Condition           string
	RestoreCondition    string
	InventoryStatus     string
	ActionType          string
	Status              string
	Quantity            int
	AmountJPY           int64
	Reason              string
	Notes               string
	ReturnDate          string
	RequestedByName     string
	RequestedAt         time.Time
	ProcessedAt         *time.Time
	InvoiceIssuedAt     *time.Time
	InvoicePrintedAt    *time.Time
	InventoryRestoredAt *time.Time
	RestoreBox          string
	RestoreComment      string
}

type ReturnRestoreItemInput struct {
	ItemID    string
	Condition string
	Quantity  int
	Box       string
}

type RestoreReturnTakehomeInput struct {
	OrganizationID string
	SaleID         string
	ActorID        string
	Comment        string
	Items          []ReturnRestoreItemInput
}

func (s *Store) ReturnTakehomeSummaries(ctx context.Context, organizationID, status, query string) ([]ReturnTakehomeSummary, error) {
	status = strings.TrimSpace(status)
	if status != "" && status != "pending" && status != "completed" {
		status = ""
	}
	like := "%" + strings.TrimSpace(query) + "%"
	rows, err := s.db.QueryContext(ctx, `
		SELECT ss.id,ss.slip_number,ss.sales_date,ss.customer_name,
		       COALESCE(SUM(CASE WHEN r.status<>'cancelled' THEN 1 ELSE 0 END),0),
		       COALESCE(SUM(CASE WHEN r.status<>'cancelled' AND r.inventory_restored_at IS NULL THEN 1 ELSE 0 END),0),
		       COALESCE(SUM(CASE WHEN r.status<>'cancelled' THEN
		           (CASE WHEN sl.converted_unit_price_jpy>0 THEN sl.converted_unit_price_jpy
		                 WHEN sl.sale_currency='JPY' THEN sl.unit_price_minor ELSE 0 END)*r.quantity
		       ELSE 0 END),0)
		FROM sales_slips ss
		JOIN return_takehome_items r
		  ON r.sales_slip_id=ss.id AND r.organization_id=ss.organization_id
		JOIN sales_lines sl ON sl.id=r.sales_line_id AND sl.sales_slip_id=ss.id
		JOIN products p ON p.id=sl.product_id AND p.organization_id=ss.organization_id
		WHERE ss.organization_id=? AND (
			ss.slip_number LIKE ? OR ss.customer_name LIKE ? OR
			p.product_code LIKE ? OR p.brand LIKE ? OR p.model_number LIKE ?
		)
		GROUP BY ss.id,ss.slip_number,ss.sales_date,ss.customer_name
		HAVING (?='' OR (?='pending' AND SUM(CASE WHEN r.status<>'cancelled' AND r.inventory_restored_at IS NULL THEN 1 ELSE 0 END)>0)
		            OR (?='completed' AND SUM(CASE WHEN r.status<>'cancelled' AND r.inventory_restored_at IS NULL THEN 1 ELSE 0 END)=0))
		ORDER BY ss.sales_date DESC,ss.slip_number DESC`,
		organizationID, like, like, like, like, like, status, status, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ReturnTakehomeSummary
	for rows.Next() {
		var item ReturnTakehomeSummary
		if err := rows.Scan(&item.SaleID, &item.SlipNumber, &item.SalesDate, &item.CustomerName, &item.ItemCount, &item.PendingCount, &item.TotalJPY); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) SalesReturnSummaries(ctx context.Context, organizationID string) ([]ReturnTakehomeSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT ss.id,ss.slip_number,COALESCE(MAX(NULLIF(r.return_date,'')),ss.sales_date),ss.customer_name,
		       COALESCE(SUM(CASE WHEN r.status<>'cancelled' THEN r.quantity ELSE 0 END),0),
		       COALESCE(SUM(CASE WHEN r.status='pending' THEN r.quantity ELSE 0 END),0),
		       COALESCE(SUM(CASE WHEN r.status<>'cancelled' THEN
		           (CASE WHEN sl.converted_unit_price_jpy>0 THEN sl.converted_unit_price_jpy
		                 WHEN sl.sale_currency='JPY' THEN sl.unit_price_minor ELSE 0 END)*r.quantity
		       ELSE 0 END),0)
		FROM sales_slips ss
		JOIN return_takehome_items r
		  ON r.sales_slip_id=ss.id AND r.organization_id=ss.organization_id AND r.action_type='return'
		JOIN sales_lines sl ON sl.id=r.sales_line_id AND sl.sales_slip_id=ss.id
		WHERE ss.organization_id=?
		GROUP BY ss.id,ss.slip_number,ss.sales_date,ss.customer_name
		ORDER BY COALESCE(MAX(NULLIF(r.return_date,'')),ss.sales_date) DESC,ss.slip_number DESC`,
		organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ReturnTakehomeSummary
	for rows.Next() {
		var item ReturnTakehomeSummary
		if err := rows.Scan(&item.SaleID, &item.SlipNumber, &item.SalesDate, &item.CustomerName,
			&item.ItemCount, &item.PendingCount, &item.TotalJPY); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) ReturnTakehomeItems(ctx context.Context, organizationID, saleID string) ([]ReturnTakehomeItem, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.id,r.sales_slip_id,r.sales_line_id,p.id,p.product_code,p.brand,p.model_number,
		       p.serial_number,p.condition_text,p.inventory_status,
		       r.action_type,r.status,r.quantity,
		       (CASE WHEN sl.converted_unit_price_jpy>0 THEN sl.converted_unit_price_jpy
		             WHEN sl.sale_currency='JPY' THEN sl.unit_price_minor ELSE 0 END)*r.quantity,
		       r.reason,r.notes,r.return_date,u.display_name,
		       r.requested_at,r.processed_at,r.invoice_issued_at,r.invoice_printed_at,
		       r.inventory_restored_at,r.restore_box_text,r.restore_comment_text
		FROM return_takehome_items r
		JOIN sales_lines sl ON sl.id=r.sales_line_id AND sl.sales_slip_id=r.sales_slip_id
		JOIN products p ON p.id=sl.product_id AND p.organization_id=r.organization_id
		JOIN users u ON u.id=r.requested_by
		WHERE r.organization_id=? AND r.sales_slip_id=?
		ORDER BY r.requested_at DESC`, organizationID, saleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ReturnTakehomeItem
	for rows.Next() {
		var item ReturnTakehomeItem
		var requested string
		var processed, issued, printed, restored sql.NullString
		if err := rows.Scan(&item.ID, &item.SaleID, &item.SalesLineID, &item.ProductID, &item.ProductCode,
			&item.Brand, &item.ModelNumber, &item.SerialNumber, &item.Condition, &item.InventoryStatus,
			&item.ActionType, &item.Status, &item.Quantity, &item.AmountJPY,
			&item.Reason, &item.Notes, &item.ReturnDate, &item.RequestedByName,
			&requested, &processed, &issued, &printed, &restored, &item.RestoreBox, &item.RestoreComment); err != nil {
			return nil, err
		}
		item.RequestedAt, _ = time.Parse(time.RFC3339Nano, requested)
		if processed.Valid {
			value, parseErr := time.Parse(time.RFC3339Nano, processed.String)
			if parseErr == nil {
				item.ProcessedAt = &value
			}
		}
		if issued.Valid {
			value, parseErr := time.Parse(time.RFC3339Nano, issued.String)
			if parseErr == nil {
				item.InvoiceIssuedAt = &value
			}
		}
		if printed.Valid {
			value, parseErr := time.Parse(time.RFC3339Nano, printed.String)
			if parseErr == nil {
				item.InvoicePrintedAt = &value
			}
		}
		if restored.Valid {
			value, parseErr := time.Parse(time.RFC3339Nano, restored.String)
			if parseErr == nil {
				item.InventoryRestoredAt = &value
			}
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) CreateReturnTakehome(ctx context.Context, organizationID, saleID, lineID, actionType string, quantity int, reason, actor string) (ReturnTakehomeItem, error) {
	if actionType != "return" && actionType != "take_home" {
		return ReturnTakehomeItem{}, ErrReturnNotEligible
	}
	var maxQuantity int
	err := s.db.QueryRowContext(ctx, `
		SELECT sl.quantity FROM sales_lines sl
		JOIN sales_slips ss ON ss.id=sl.sales_slip_id AND ss.organization_id=sl.organization_id
		WHERE sl.id=? AND ss.id=? AND ss.organization_id=? AND ss.status='confirmed'`,
		lineID, saleID, organizationID).Scan(&maxQuantity)
	if err != nil {
		return ReturnTakehomeItem{}, ErrReturnNotEligible
	}
	if quantity <= 0 || quantity > maxQuantity {
		return ReturnTakehomeItem{}, ErrReturnInvalidQuantity
	}
	var pending int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM return_takehome_items
		WHERE organization_id=? AND sales_line_id=? AND action_type=? AND status='pending'`,
		organizationID, lineID, actionType).Scan(&pending); err != nil {
		return ReturnTakehomeItem{}, err
	}
	if pending > 0 {
		return ReturnTakehomeItem{}, ErrReturnAlreadyPending
	}
	id, err := NewID("rtn")
	if err != nil {
		return ReturnTakehomeItem{}, err
	}
	now := s.now().Format(time.RFC3339Nano)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO return_takehome_items(
			id,organization_id,sales_slip_id,sales_line_id,action_type,status,quantity,reason,
			requested_by,requested_at,created_at,updated_at
		) VALUES(?,?,?,?,?,'pending',?,?,?,?,?,?)`,
		id, organizationID, saleID, lineID, actionType, quantity, strings.TrimSpace(reason), actor, now, now, now)
	if err != nil {
		return ReturnTakehomeItem{}, err
	}
	items, err := s.ReturnTakehomeItems(ctx, organizationID, saleID)
	if err != nil {
		return ReturnTakehomeItem{}, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	return ReturnTakehomeItem{}, sql.ErrNoRows
}

func (s *Store) CompleteReturnTakehome(ctx context.Context, organizationID, saleID, itemID, notes, actor string) error {
	now := s.now().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, `
		UPDATE return_takehome_items
		SET status='completed',notes=?,processed_by=?,processed_at=?,updated_at=?
		WHERE id=? AND sales_slip_id=? AND organization_id=? AND status='pending'`,
		strings.TrimSpace(notes), actor, now, now, itemID, saleID, organizationID)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return ErrReturnAlreadyHandled
	}
	return nil
}

func (s *Store) RestoreReturnTakehomeItems(ctx context.Context, input RestoreReturnTakehomeInput) error {
	if len(input.Items) == 0 {
		return ErrReturnRestoreSelection
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := s.now().Format(time.RFC3339Nano)
	for _, requested := range input.Items {
		requested.ItemID = strings.TrimSpace(requested.ItemID)
		requested.Condition = strings.TrimSpace(requested.Condition)
		requested.Box = strings.TrimSpace(requested.Box)
		if requested.ItemID == "" {
			return ErrReturnRestoreSelection
		}
		if requested.Box != "" {
			validBox := false
			for index := 1; index <= 10; index++ {
				if requested.Box == "BOX"+strconv.Itoa(index) {
					validBox = true
					break
				}
			}
			if !validBox {
				return errors.New("BOXの選択内容を確認してください")
			}
		}
		var productID, fromStatus, currentCondition string
		var restoredAt sql.NullString
		var quantity int
		if err := tx.QueryRowContext(ctx, `
			SELECT p.id,r.inventory_restored_at,r.quantity,p.inventory_status,p.condition_text
			FROM return_takehome_items r
			JOIN sales_lines sl ON sl.id=r.sales_line_id AND sl.sales_slip_id=r.sales_slip_id
			JOIN products p ON p.id=sl.product_id AND p.organization_id=r.organization_id
			WHERE r.id=? AND r.sales_slip_id=? AND r.organization_id=?`,
			requested.ItemID, input.SaleID, input.OrganizationID).
			Scan(&productID, &restoredAt, &quantity, &fromStatus, &currentCondition); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrReturnAlreadyHandled
			}
			return err
		}
		if restoredAt.Valid {
			return ErrReturnAlreadyHandled
		}
		if requested.Quantity <= 0 || requested.Quantity != quantity {
			return ErrReturnInvalidQuantity
		}
		if fromStatus != "sold" && fromStatus != "shipped" && fromStatus != "reserved" {
			return ErrReturnAlreadyHandled
		}
		if requested.Condition == "" {
			requested.Condition = currentCondition
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE products
			SET inventory_status='in_stock',condition_text=?,box_text=?,updated_at=?
			WHERE id=? AND organization_id=?`,
			requested.Condition, requested.Box, now, productID, input.OrganizationID); err != nil {
			return err
		}
		note := "在庫戻し（確認後コンディション: " + requested.Condition
		if requested.Box != "" {
			note += "、BOX: " + requested.Box
		}
		if comment := strings.TrimSpace(input.Comment); comment != "" {
			note += "、管理者コメント: " + comment
		}
		note += "）"
		result, err := tx.ExecContext(ctx, `
			UPDATE return_takehome_items
			SET status='completed',notes=?,processed_by=?,processed_at=?,
			    inventory_restored_at=?,inventory_restored_by=?,restore_box_text=?,restore_comment_text=?,updated_at=?
			WHERE id=? AND sales_slip_id=? AND organization_id=? AND inventory_restored_at IS NULL AND status<>'cancelled'`,
			note, input.ActorID, now, now, input.ActorID, requested.Box, strings.TrimSpace(input.Comment), now,
			requested.ItemID, input.SaleID, input.OrganizationID)
		if err != nil {
			return err
		}
		affected, _ := result.RowsAffected()
		if affected != 1 {
			return ErrReturnAlreadyHandled
		}
		eventID, _ := NewID("evt")
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO inventory_events(
				id,organization_id,product_id,event_type,from_status,to_status,reason,actor_user_id,created_at
			) VALUES(?,?,?,'return.restored',?,'in_stock',?,?,?)`,
			eventID, input.OrganizationID, productID, fromStatus, note, input.ActorID, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SeedReturnTakehomePreview(ctx context.Context) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM return_takehome_items WHERE organization_id='org_preview'`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT ss.id,sl.id,sl.quantity FROM sales_slips ss
		JOIN sales_lines sl ON sl.sales_slip_id=ss.id AND sl.organization_id=ss.organization_id
		WHERE ss.organization_id='org_preview' AND ss.status='confirmed'
		ORDER BY ss.sales_date DESC,ss.id LIMIT 2`)
	if err != nil {
		return err
	}
	type seed struct {
		saleID, lineID string
		quantity       int
	}
	var seeds []seed
	for rows.Next() {
		var item seed
		if err := rows.Scan(&item.saleID, &item.lineID, &item.quantity); err != nil {
			rows.Close()
			return err
		}
		seeds = append(seeds, item)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for index, item := range seeds {
		action := "return"
		if index%2 == 1 {
			action = "take_home"
		}
		if _, err := s.CreateReturnTakehome(ctx, "org_preview", item.saleID, item.lineID, action, 1, "プレビュー用受付", "usr_admin"); err != nil {
			return err
		}
	}
	if len(seeds) == 1 {
		_, err = s.CreateReturnTakehome(ctx, "org_preview", seeds[0].saleID, seeds[0].lineID, "take_home", 1, "プレビュー用持ち帰り", "usr_admin")
	}
	return err
}
