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
	ErrStocktakeInProgress = errors.New("実施中の棚卸があります")
	ErrStocktakeNotFound   = errors.New("棚卸が見つかりません")
	ErrStocktakeCompleted  = errors.New("確定済みの棚卸は変更できません")
	ErrStocktakeUnchecked  = errors.New("未確認の商品があります")
	ErrStocktakePending    = errors.New("承認待ちの商品があります")
	ErrStocktakeMismatch   = errors.New("棚卸の件数または金額が一致しません")
	ErrStocktakeDuplicate  = errors.New("この商品は確認済みです")
)

type Stocktake struct {
	ID                 string
	Number             string
	Date               string
	Status             string
	ExpectedCount      int
	ExpectedTotalMinor int64
	CountedCount       int
	MatchedCount       int
	DifferenceCount    int
	Notes              string
	CreatedAt          time.Time
	CompletedAt        *time.Time
	SavedAt            *time.Time
	Lines              []StocktakeLine
}

type StocktakeLine struct {
	ID               string
	ProductID        string
	ProductCode      string
	Brand            string
	Model            string
	ReferenceNumber  string
	ModelNumber      string
	SerialNumber     string
	Condition        string
	BuyerName        string
	CostAmountMinor  int64
	PurchaseDate     string
	InventoryStatus  string
	ExpectedPresent  bool
	CountedPresent   *bool
	Notes            string
	DifferenceReason string
	ReviewStatus     string
	ApprovalStatus   string
	CountedAt        *time.Time
	FinalizedAt      *time.Time
}

func (s *Store) Stocktakes(ctx context.Context, organizationID string) ([]Stocktake, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT st.id,st.stocktake_number,st.stocktake_date,st.status,st.expected_count,st.expected_total_minor,st.notes,st.created_at,st.completed_at,st.saved_at,
		       COALESCE(SUM(CASE WHEN sl.counted_present IS NOT NULL THEN 1 ELSE 0 END),0),
		       COALESCE(SUM(CASE WHEN sl.counted_present=sl.expected_present THEN 1 ELSE 0 END),0),
		       COALESCE(SUM(CASE WHEN sl.counted_present IS NOT NULL AND sl.counted_present<>sl.expected_present THEN 1 ELSE 0 END),0)
		FROM stocktakes st
		LEFT JOIN stocktake_lines sl ON sl.stocktake_id=st.id
		WHERE st.organization_id=?
		GROUP BY st.id
		ORDER BY st.stocktake_date DESC,st.created_at DESC`, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	stocktakes := make([]Stocktake, 0)
	for rows.Next() {
		stocktake, err := scanStocktake(rows)
		if err != nil {
			return nil, err
		}
		stocktakes = append(stocktakes, stocktake)
	}
	return stocktakes, rows.Err()
}

func (s *Store) Stocktake(ctx context.Context, organizationID, id string) (Stocktake, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT st.id,st.stocktake_number,st.stocktake_date,st.status,st.expected_count,st.expected_total_minor,st.notes,st.created_at,st.completed_at,st.saved_at,
		       COALESCE(SUM(CASE WHEN sl.counted_present IS NOT NULL THEN 1 ELSE 0 END),0),
		       COALESCE(SUM(CASE WHEN sl.counted_present=sl.expected_present THEN 1 ELSE 0 END),0),
		       COALESCE(SUM(CASE WHEN sl.counted_present IS NOT NULL AND sl.counted_present<>sl.expected_present THEN 1 ELSE 0 END),0)
		FROM stocktakes st
		LEFT JOIN stocktake_lines sl ON sl.stocktake_id=st.id
		WHERE st.organization_id=? AND st.id=?
		GROUP BY st.id`, organizationID, id)
	stocktake, err := scanStocktake(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Stocktake{}, ErrStocktakeNotFound
	}
	if err != nil {
		return Stocktake{}, err
	}
	lines, err := s.stocktakeLines(ctx, id)
	if err != nil {
		return Stocktake{}, err
	}
	stocktake.Lines = lines
	return stocktake, nil
}

func scanStocktake(scanner rowScanner) (Stocktake, error) {
	var (
		stocktake Stocktake
		created   string
		completed sql.NullString
		saved     sql.NullString
	)
	err := scanner.Scan(
		&stocktake.ID, &stocktake.Number, &stocktake.Date, &stocktake.Status,
		&stocktake.ExpectedCount, &stocktake.ExpectedTotalMinor, &stocktake.Notes, &created, &completed, &saved,
		&stocktake.CountedCount, &stocktake.MatchedCount, &stocktake.DifferenceCount,
	)
	if err != nil {
		return Stocktake{}, err
	}
	stocktake.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	if completed.Valid {
		value, _ := time.Parse(time.RFC3339Nano, completed.String)
		stocktake.CompletedAt = &value
	}
	if saved.Valid {
		value, _ := time.Parse(time.RFC3339Nano, saved.String)
		stocktake.SavedAt = &value
	}
	return stocktake, nil
}

func (s *Store) stocktakeLines(ctx context.Context, stocktakeID string) ([]StocktakeLine, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT sl.id,sl.product_id,p.product_code,p.brand,p.product_type,p.model_number,p.serial_number,
		       p.condition_text,u.display_name,p.cost_amount_minor,p.purchase_date,p.inventory_status,
		       sl.expected_present,sl.counted_present,sl.notes,sl.difference_reason,sl.review_status,
		       COALESCE((
		         SELECT ar.status FROM approval_requests ar
		         WHERE ar.organization_id=st.organization_id
		           AND ar.target_type='stocktake_line' AND ar.target_id=sl.id
		           AND ar.action_key='stocktake.difference.approve'
		         ORDER BY ar.requested_at DESC LIMIT 1
		       ),''),
		       sl.counted_at,sl.finalized_at
		FROM stocktake_lines sl
		JOIN stocktakes st ON st.id=sl.stocktake_id
		JOIN products p ON p.id=sl.product_id
		JOIN purchase_slip_lines psl ON psl.id=p.purchase_slip_line_id
		JOIN purchase_slips ps ON ps.id=psl.purchase_slip_id
		JOIN users u ON u.id=ps.created_by
		WHERE sl.stocktake_id=?
		ORDER BY p.product_code`, stocktakeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lines := make([]StocktakeLine, 0)
	for rows.Next() {
		var (
			line        StocktakeLine
			expected    int
			counted     sql.NullInt64
			countedAt   sql.NullString
			finalizedAt sql.NullString
		)
		if err := rows.Scan(
			&line.ID, &line.ProductID, &line.ProductCode, &line.Brand, &line.Model, &line.ReferenceNumber,
			&line.SerialNumber, &line.Condition, &line.BuyerName, &line.CostAmountMinor,
			&line.PurchaseDate, &line.InventoryStatus, &expected, &counted, &line.Notes,
			&line.DifferenceReason, &line.ReviewStatus, &line.ApprovalStatus, &countedAt, &finalizedAt,
		); err != nil {
			return nil, err
		}
		line.ExpectedPresent = expected == 1
		if counted.Valid {
			value := counted.Int64 == 1
			line.CountedPresent = &value
		}
		if countedAt.Valid {
			value, _ := time.Parse(time.RFC3339Nano, countedAt.String)
			line.CountedAt = &value
		}
		if finalizedAt.Valid {
			value, _ := time.Parse(time.RFC3339Nano, finalizedAt.String)
			line.FinalizedAt = &value
		}
		line.ModelNumber = line.ReferenceNumber
		lines = append(lines, line)
	}
	return lines, rows.Err()
}

func (s *Store) RequestStocktakeDifferenceApproval(
	ctx context.Context, organizationID, stocktakeID, lineID, reason, note, actorUserID string,
) (ApprovalRequest, error) {
	stocktake, err := s.Stocktake(ctx, organizationID, stocktakeID)
	if err != nil {
		return ApprovalRequest{}, err
	}
	var target *StocktakeLine
	for index := range stocktake.Lines {
		if stocktake.Lines[index].ID == lineID {
			target = &stocktake.Lines[index]
			break
		}
	}
	if target == nil {
		return ApprovalRequest{}, ErrStocktakeNotFound
	}
	recorded := false
	if target.CountedPresent == nil {
		if err := s.RecordStocktakeDifference(
			ctx, organizationID, stocktakeID, lineID, reason, note, actorUserID, true,
		); err != nil {
			return ApprovalRequest{}, err
		}
		recorded = true
	} else if *target.CountedPresent || target.ReviewStatus != "pending" || target.ApprovalStatus != "returned" {
		return ApprovalRequest{}, ErrStocktakeDuplicate
	}
	approval, err := s.CreateApprovalRequest(ctx, CreateApprovalInput{
		OrganizationID: organizationID, ApprovalType: "棚卸不一致",
		TargetType: "stocktake_line", TargetID: lineID,
		ActionKey: "stocktake.difference.approve", ApplicantUserID: actorUserID,
		RequestReason: strings.TrimSpace(reason) + " " + strings.TrimSpace(note),
		ActionPayload: map[string]string{"stocktake_id": stocktakeID},
	})
	if err == nil {
		return approval, nil
	}
	if recorded {
		_, _ = s.db.ExecContext(ctx, `
			UPDATE stocktake_lines
			SET counted_present=NULL,notes='',difference_reason='',review_status='none',
			    counted_by=NULL,counted_at=NULL,updated_at=?
			WHERE id=? AND stocktake_id=? AND review_status='pending'`,
			s.now().Format(time.RFC3339Nano), lineID, stocktakeID)
	}
	return ApprovalRequest{}, err
}

func (s *Store) ApproveStocktakeDifference(ctx context.Context, organizationID, lineID, actorUserID string) error {
	now := s.now().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, `
		UPDATE stocktake_lines
		SET review_status='approved',updated_at=?
		WHERE id=? AND counted_present=0 AND review_status='pending'
		  AND EXISTS (
		    SELECT 1 FROM stocktakes st
		    WHERE st.id=stocktake_lines.stocktake_id AND st.organization_id=? AND st.status='draft'
		  )`, now, lineID, organizationID)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected != 1 {
		return ErrStocktakeNotFound
	}
	return nil
}

func (s *Store) CreateStocktake(ctx context.Context, organizationID, stocktakeDate, notes, actorUserID string) (Stocktake, error) {
	if organizationID == "" || actorUserID == "" {
		return Stocktake{}, errors.New("組織と操作者は必須です")
	}
	if _, err := time.Parse("2006-01-02", stocktakeDate); err != nil {
		return Stocktake{}, errors.New("棚卸日を正しく入力してください")
	}
	if current, found, err := s.activeDraftStocktake(ctx, organizationID); err != nil {
		return Stocktake{}, err
	} else if found {
		return current, ErrStocktakeInProgress
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Stocktake{}, err
	}
	defer tx.Rollback()
	prefix := "STK-" + strings.ReplaceAll(stocktakeDate, "-", "")
	var sequence int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*)+1 FROM stocktakes WHERE organization_id=? AND stocktake_number LIKE ?`,
		organizationID, prefix+"-%").Scan(&sequence); err != nil {
		return Stocktake{}, err
	}
	id, _ := NewID("stk")
	number := fmt.Sprintf("%s-%03d", prefix, sequence)
	now := s.now().Format(time.RFC3339Nano)
	result, err := tx.ExecContext(ctx, `
		INSERT INTO stocktakes(
			id,organization_id,stocktake_number,stocktake_date,status,notes,created_by,created_at,updated_at
		) VALUES(?,?,?,?,'draft',?,?,?,?)`,
		id, organizationID, number, stocktakeDate, strings.TrimSpace(notes), actorUserID, now, now)
	if err != nil {
		_ = tx.Rollback()
		if current, found, lookupErr := s.activeDraftStocktake(ctx, organizationID); lookupErr == nil && found {
			return current, ErrStocktakeInProgress
		}
		return Stocktake{}, err
	}
	if _, err := result.RowsAffected(); err != nil {
		return Stocktake{}, err
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id,cost_amount_minor FROM products
		WHERE organization_id=? AND deleted_at IS NULL
		  AND inventory_status IN ('purchasing','in_stock','reserved','sold')
		ORDER BY product_code`, organizationID)
	if err != nil {
		return Stocktake{}, err
	}
	productIDs := make([]string, 0)
	var expectedTotal int64
	for rows.Next() {
		var productID string
		var cost int64
		if err := rows.Scan(&productID, &cost); err != nil {
			rows.Close()
			return Stocktake{}, err
		}
		productIDs = append(productIDs, productID)
		expectedTotal += cost
	}
	if err := rows.Close(); err != nil {
		return Stocktake{}, err
	}
	for _, productID := range productIDs {
		lineID, _ := NewID("stl")
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO stocktake_lines(
				id,stocktake_id,product_id,expected_present,created_at,updated_at
			) VALUES(?,?,?,1,?,?)`, lineID, id, productID, now, now); err != nil {
			return Stocktake{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE stocktakes SET expected_count=?,expected_total_minor=? WHERE id=?`, len(productIDs), expectedTotal, id); err != nil {
		return Stocktake{}, err
	}
	if err := tx.Commit(); err != nil {
		if current, found, lookupErr := s.activeDraftStocktake(ctx, organizationID); lookupErr == nil && found {
			return current, ErrStocktakeInProgress
		}
		return Stocktake{}, err
	}
	return s.Stocktake(ctx, organizationID, id)
}

func (s *Store) activeDraftStocktake(ctx context.Context, organizationID string) (Stocktake, bool, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `
		SELECT id
		FROM stocktakes
		WHERE organization_id=? AND status='draft'
		ORDER BY created_at DESC
		LIMIT 1`, organizationID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return Stocktake{}, false, nil
	}
	if err != nil {
		return Stocktake{}, false, err
	}
	stocktake, err := s.Stocktake(ctx, organizationID, id)
	if err != nil {
		return Stocktake{}, false, err
	}
	return stocktake, true, nil
}

func (s *Store) UpdateStocktakeLine(ctx context.Context, organizationID, stocktakeID, lineID string, present bool, notes, actorUserID string) error {
	value := 0
	if present {
		value = 1
	}
	now := s.now().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, `
		UPDATE stocktake_lines
		SET counted_present=?,notes=?,counted_by=?,counted_at=?,updated_at=?
		WHERE id=? AND stocktake_id=? AND EXISTS(
			SELECT 1 FROM stocktakes st
			WHERE st.id=stocktake_lines.stocktake_id AND st.organization_id=? AND st.status='draft'
		)`, value, strings.TrimSpace(notes), actorUserID, now, now, lineID, stocktakeID, organizationID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		stocktake, lookupErr := s.Stocktake(ctx, organizationID, stocktakeID)
		if lookupErr != nil {
			return lookupErr
		}
		if stocktake.Status == "completed" {
			return ErrStocktakeCompleted
		}
		return ErrStocktakeNotFound
	}
	return nil
}

func (s *Store) SaveStocktake(ctx context.Context, organizationID, stocktakeID string) error {
	now := s.now().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, `
		UPDATE stocktakes SET saved_at=?,updated_at=?
		WHERE id=? AND organization_id=? AND status='draft'`,
		now, now, stocktakeID, organizationID)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrStocktakeNotFound
	}
	return nil
}

func (s *Store) RecordStocktakeDifference(ctx context.Context, organizationID, stocktakeID, lineID, reason, note, actorUserID string, pending bool) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return errors.New("不一致理由を選択してください")
	}
	review := "approved"
	if pending {
		review = "pending"
	}
	now := s.now().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, `
		UPDATE stocktake_lines
		SET counted_present=0,notes=?,difference_reason=?,review_status=?,
		    counted_by=?,counted_at=?,updated_at=?
		WHERE id=? AND stocktake_id=? AND counted_present IS NULL AND EXISTS(
			SELECT 1 FROM stocktakes st
			WHERE st.id=stocktake_lines.stocktake_id AND st.organization_id=? AND st.status='draft'
		)`, strings.TrimSpace(note), reason, review, actorUserID, now, now,
		lineID, stocktakeID, organizationID)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		var counted sql.NullInt64
		err := s.db.QueryRowContext(ctx, `SELECT counted_present FROM stocktake_lines WHERE id=? AND stocktake_id=?`, lineID, stocktakeID).Scan(&counted)
		if err == nil && counted.Valid {
			return ErrStocktakeDuplicate
		}
		return ErrStocktakeNotFound
	}
	return nil
}

func (s *Store) MarkStocktakePresent(ctx context.Context, organizationID, stocktakeID, lineID, actorUserID string) error {
	now := s.now().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, `
		UPDATE stocktake_lines
		SET counted_present=1,notes='',difference_reason='',review_status='none',
		    counted_by=?,counted_at=?,updated_at=?
		WHERE id=? AND stocktake_id=? AND counted_present IS NULL AND EXISTS(
			SELECT 1 FROM stocktakes st
			WHERE st.id=stocktake_lines.stocktake_id AND st.organization_id=? AND st.status='draft'
		)`, actorUserID, now, now, lineID, stocktakeID, organizationID)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		var counted sql.NullInt64
		err := s.db.QueryRowContext(ctx, `SELECT counted_present FROM stocktake_lines WHERE id=? AND stocktake_id=?`, lineID, stocktakeID).Scan(&counted)
		if err == nil && counted.Valid {
			return ErrStocktakeDuplicate
		}
		return ErrStocktakeNotFound
	}
	return nil
}

func (s *Store) CompleteStocktake(ctx context.Context, organizationID, stocktakeID, actorUserID string) (Stocktake, error) {
	stocktake, err := s.Stocktake(ctx, organizationID, stocktakeID)
	if err != nil {
		return Stocktake{}, err
	}
	if stocktake.Status == "completed" {
		return Stocktake{}, ErrStocktakeCompleted
	}
	if stocktake.CountedCount != stocktake.ExpectedCount {
		return Stocktake{}, ErrStocktakeUnchecked
	}
	var count, pending int
	var total int64
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*),COALESCE(SUM(p.cost_amount_minor),0),
		       COALESCE(SUM(CASE WHEN sl.review_status='pending' THEN 1 ELSE 0 END),0)
		FROM stocktake_lines sl JOIN products p ON p.id=sl.product_id
		WHERE sl.stocktake_id=?`, stocktakeID).Scan(&count, &total, &pending); err != nil {
		return Stocktake{}, err
	}
	if pending > 0 {
		return Stocktake{}, ErrStocktakePending
	}
	if count != stocktake.ExpectedCount || total != stocktake.ExpectedTotalMinor {
		return Stocktake{}, ErrStocktakeMismatch
	}
	now := s.now().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Stocktake{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE stocktake_lines SET finalized_at=?,updated_at=? WHERE stocktake_id=?`, now, now, stocktakeID); err != nil {
		return Stocktake{}, err
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE stocktakes SET status='completed',completed_by=?,completed_at=?,updated_at=?
		WHERE id=? AND organization_id=? AND status='draft'`,
		actorUserID, now, now, stocktakeID, organizationID)
	if err != nil {
		return Stocktake{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return Stocktake{}, err
	}
	if affected == 0 {
		return Stocktake{}, ErrStocktakeCompleted
	}
	if err := tx.Commit(); err != nil {
		return Stocktake{}, err
	}
	return s.Stocktake(ctx, organizationID, stocktakeID)
}

func (s *Store) SeedStocktakePreview(ctx context.Context) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM stocktakes WHERE organization_id='org_preview'`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	stocktake, err := s.CreateStocktake(ctx, "org_preview", "2026-07-27", "月次棚卸（プレビュー）", "usr_admin")
	if err != nil {
		return err
	}
	for index, line := range stocktake.Lines {
		if index >= 2 {
			break
		}
		present := index == 0
		note := ""
		if !present {
			note = "保管場所を再確認中"
		}
		if err := s.UpdateStocktakeLine(ctx, "org_preview", stocktake.ID, line.ID, present, note, "usr_admin"); err != nil {
			return err
		}
	}
	return nil
}
