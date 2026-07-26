package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	ErrProductNotAvailable  = errors.New("この商品は現在購入依頼できません")
	ErrReservationConflict  = errors.New("この商品にはすでに有効な取置があります")
	ErrRequestNotPending    = errors.New("申請中の購入依頼だけ承認できます")
	ErrReservationNotActive = errors.New("有効な取置が見つかりません")
)

type PublicProduct struct {
	ID             string
	ProductCode    string
	Brand          string
	ModelNumber    string
	ProductType    string
	SalePriceMinor int64
	SaleCurrency   string
	Condition      string
	Accessories    string
}

type PurchaseRequestInput struct {
	OrganizationCode string
	ProductID        string
	GuestName        string
	GuestEmail       string
	GuestPhone       string
	Message          string
}

type PurchaseRequest struct {
	ID                 string
	OrganizationID     string
	ProductID          string
	ProductCode        string
	Brand              string
	ModelNumber        string
	RequestNumber      string
	GuestName          string
	GuestEmail         string
	GuestPhone         string
	Message            string
	Status             string
	RequestedAt        time.Time
	ReviewedAt         *time.Time
	ReservationID      string
	ReservationStatus  string
	ReservationStarts  *time.Time
	ReservationExpires *time.Time
}

type Reservation struct {
	ID                string
	OrganizationID    string
	ProductID         string
	PurchaseRequestID string
	Status            string
	StartsAt          time.Time
	ExpiresAt         time.Time
	ReleasedAt        *time.Time
	ReleaseReason     string
}

func (s *Store) OrganizationIDByCode(ctx context.Context, organizationCode string) (string, error) {
	var organizationID string
	err := s.db.QueryRowContext(ctx, `
		SELECT id FROM organizations WHERE code=? AND is_active=1`,
		strings.ToUpper(strings.TrimSpace(organizationCode))).Scan(&organizationID)
	return organizationID, err
}

func (s *Store) PublicProducts(ctx context.Context, organizationCode string) ([]PublicProduct, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id,p.product_code,p.brand,p.model_number,p.product_type,p.base_sale_price_minor,p.base_sale_currency,
		       p.condition_text,p.accessories
		FROM products p JOIN organizations o ON o.id=p.organization_id
		WHERE o.code=? AND o.is_active=1 AND p.publication_status='public'
		  AND p.inventory_status='in_stock' AND p.deleted_at IS NULL
		ORDER BY p.purchase_date DESC,p.product_code DESC LIMIT 200`,
		strings.ToUpper(strings.TrimSpace(organizationCode)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var products []PublicProduct
	for rows.Next() {
		var product PublicProduct
		if err := rows.Scan(&product.ID, &product.ProductCode, &product.Brand, &product.ModelNumber,
			&product.ProductType, &product.SalePriceMinor, &product.SaleCurrency,
			&product.Condition, &product.Accessories); err != nil {
			return nil, err
		}
		products = append(products, product)
	}
	return products, rows.Err()
}

func (s *Store) SetProductPublication(ctx context.Context, organizationID, productID, actorID, status string) error {
	if status != "private" && status != "public" {
		return errors.New("公開状態が正しくありません")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var inventoryStatus, before string
	if err := tx.QueryRowContext(ctx, `
		SELECT inventory_status,publication_status FROM products
		WHERE id=? AND organization_id=? AND deleted_at IS NULL`,
		productID, organizationID).Scan(&inventoryStatus, &before); err != nil {
		return err
	}
	if status == "public" && inventoryStatus != "in_stock" {
		return errors.New("在庫中の商品だけ公開できます")
	}
	now := s.now().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `
		UPDATE products SET publication_status=?,updated_at=? WHERE id=? AND organization_id=?`,
		status, now, productID, organizationID); err != nil {
		return err
	}
	eventID, _ := NewID("evt")
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO inventory_events(
			id,organization_id,product_id,event_type,from_status,to_status,reason,actor_user_id,created_at
		) VALUES(?,?,?,'publication.changed',?,?,?, ?,?)`,
		eventID, organizationID, productID, before, status, "公開状態変更", actorID, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) CreatePurchaseRequest(ctx context.Context, input PurchaseRequestInput) (PurchaseRequest, error) {
	input.GuestName = strings.TrimSpace(input.GuestName)
	input.GuestEmail = strings.TrimSpace(strings.ToLower(input.GuestEmail))
	if input.GuestName == "" || !strings.Contains(input.GuestEmail, "@") || len(input.GuestEmail) > 254 {
		return PurchaseRequest{}, errors.New("お名前と正しいメールアドレスを入力してください")
	}
	if len([]rune(input.GuestName)) > 100 || len([]rune(input.GuestPhone)) > 40 || len([]rune(input.Message)) > 2000 {
		return PurchaseRequest{}, errors.New("入力文字数を確認してください")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PurchaseRequest{}, err
	}
	defer tx.Rollback()
	var organizationID string
	if err := tx.QueryRowContext(ctx, `
		SELECT p.organization_id FROM products p JOIN organizations o ON o.id=p.organization_id
		WHERE p.id=? AND o.code=? AND o.is_active=1 AND p.publication_status='public'
		  AND p.inventory_status='in_stock' AND p.deleted_at IS NULL`,
		input.ProductID, strings.ToUpper(strings.TrimSpace(input.OrganizationCode))).Scan(&organizationID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PurchaseRequest{}, ErrProductNotAvailable
		}
		return PurchaseRequest{}, err
	}
	nowTime := s.now()
	now := nowTime.Format(time.RFC3339Nano)
	var count int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM purchase_requests
		WHERE organization_id=? AND substr(requested_at,1,4)=?`,
		organizationID, nowTime.Format("2006")).Scan(&count); err != nil {
		return PurchaseRequest{}, err
	}
	id, _ := NewID("reqbuy")
	number := fmt.Sprintf("RQ-%s-%04d", nowTime.Format("2006"), count+1)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO purchase_requests(
			id,organization_id,product_id,request_number,guest_name,guest_email,guest_phone,message,
			status,requested_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,'pending',?,?)`,
		id, organizationID, input.ProductID, number, input.GuestName, input.GuestEmail,
		strings.TrimSpace(input.GuestPhone), strings.TrimSpace(input.Message), now, now); err != nil {
		return PurchaseRequest{}, err
	}
	if err := tx.Commit(); err != nil {
		return PurchaseRequest{}, err
	}
	return s.PurchaseRequest(ctx, organizationID, id)
}

func (s *Store) PurchaseRequests(ctx context.Context, organizationID string) ([]PurchaseRequest, error) {
	if err := s.ExpireReservations(ctx, organizationID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, purchaseRequestSelect+`
		WHERE r.organization_id=? ORDER BY r.requested_at DESC LIMIT 500`, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var requests []PurchaseRequest
	for rows.Next() {
		request, err := scanPurchaseRequest(rows)
		if err != nil {
			return nil, err
		}
		requests = append(requests, request)
	}
	return requests, rows.Err()
}

func (s *Store) PurchaseRequest(ctx context.Context, organizationID, requestID string) (PurchaseRequest, error) {
	return scanPurchaseRequest(s.db.QueryRowContext(ctx, purchaseRequestSelect+`
		WHERE r.organization_id=? AND r.id=?`, organizationID, requestID))
}

const purchaseRequestSelect = `
	SELECT r.id,r.organization_id,r.product_id,p.product_code,p.brand,p.model_number,r.request_number,
	       r.guest_name,r.guest_email,r.guest_phone,r.message,r.status,r.requested_at,r.reviewed_at,
	       COALESCE(v.id,''),COALESCE(v.status,''),v.starts_at,v.expires_at
	FROM purchase_requests r
	JOIN products p ON p.id=r.product_id AND p.organization_id=r.organization_id
	LEFT JOIN reservations v ON v.purchase_request_id=r.id
`

func scanPurchaseRequest(row rowScanner) (PurchaseRequest, error) {
	var request PurchaseRequest
	var requested string
	var reviewed, starts, expires sql.NullString
	err := row.Scan(&request.ID, &request.OrganizationID, &request.ProductID, &request.ProductCode,
		&request.Brand, &request.ModelNumber, &request.RequestNumber, &request.GuestName,
		&request.GuestEmail, &request.GuestPhone, &request.Message, &request.Status, &requested,
		&reviewed, &request.ReservationID, &request.ReservationStatus, &starts, &expires)
	if err != nil {
		return PurchaseRequest{}, err
	}
	request.RequestedAt, _ = time.Parse(time.RFC3339Nano, requested)
	if reviewed.Valid {
		value, _ := time.Parse(time.RFC3339Nano, reviewed.String)
		request.ReviewedAt = &value
	}
	if starts.Valid {
		value, _ := time.Parse(time.RFC3339Nano, starts.String)
		request.ReservationStarts = &value
	}
	if expires.Valid {
		value, _ := time.Parse(time.RFC3339Nano, expires.String)
		request.ReservationExpires = &value
	}
	return request, nil
}

func (s *Store) ApprovePurchaseRequest(ctx context.Context, organizationID, requestID, actorID string) (PurchaseRequest, error) {
	if err := s.ExpireReservations(ctx, organizationID); err != nil {
		return PurchaseRequest{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PurchaseRequest{}, err
	}
	defer tx.Rollback()
	var status, productID, productStatus string
	if err := tx.QueryRowContext(ctx, `
		SELECT r.status,r.product_id,p.inventory_status
		FROM purchase_requests r JOIN products p ON p.id=r.product_id AND p.organization_id=r.organization_id
		WHERE r.id=? AND r.organization_id=?`, requestID, organizationID).
		Scan(&status, &productID, &productStatus); err != nil {
		return PurchaseRequest{}, err
	}
	if status != "pending" {
		return PurchaseRequest{}, ErrRequestNotPending
	}
	if productStatus != "in_stock" {
		return PurchaseRequest{}, ErrReservationConflict
	}
	hours, err := reservationDurationHoursTx(ctx, tx, organizationID)
	if err != nil {
		return PurchaseRequest{}, err
	}
	nowTime := s.now()
	now := nowTime.Format(time.RFC3339Nano)
	reservationID, _ := NewID("res")
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO reservations(
			id,organization_id,product_id,purchase_request_id,status,starts_at,expires_at,
			created_by,created_at,updated_at
		) VALUES(?,?,?,?,'active',?,?,?,?,?)`,
		reservationID, organizationID, productID, requestID, now,
		nowTime.Add(time.Duration(hours)*time.Hour).Format(time.RFC3339Nano), actorID, now, now); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return PurchaseRequest{}, ErrReservationConflict
		}
		return PurchaseRequest{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE purchase_requests SET status='approved',reviewed_at=?,reviewed_by=?,updated_at=?
		WHERE id=? AND organization_id=? AND status='pending'`,
		now, actorID, now, requestID, organizationID); err != nil {
		return PurchaseRequest{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE products SET inventory_status='reserved',updated_at=?
		WHERE id=? AND organization_id=? AND inventory_status='in_stock'`,
		now, productID, organizationID); err != nil {
		return PurchaseRequest{}, err
	}
	if err := addInventoryEventTx(ctx, tx, organizationID, productID, actorID,
		"reservation.started", "in_stock", "reserved", "購入依頼 "+requestID, now); err != nil {
		return PurchaseRequest{}, err
	}
	if err := tx.Commit(); err != nil {
		return PurchaseRequest{}, err
	}
	return s.PurchaseRequest(ctx, organizationID, requestID)
}

func (s *Store) RejectPurchaseRequest(ctx context.Context, organizationID, requestID, actorID, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return errors.New("却下理由は必須です")
	}
	return s.closePurchaseRequest(ctx, organizationID, requestID, actorID, "rejected", reason)
}

func (s *Store) CancelPurchaseRequest(ctx context.Context, organizationID, requestID, actorID, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return errors.New("取消理由は必須です")
	}
	return s.closePurchaseRequest(ctx, organizationID, requestID, actorID, "cancelled", reason)
}

func (s *Store) closePurchaseRequest(ctx context.Context, organizationID, requestID, actorID, toStatus, reason string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var status, productID string
	if err := tx.QueryRowContext(ctx, `
		SELECT status,product_id FROM purchase_requests WHERE id=? AND organization_id=?`,
		requestID, organizationID).Scan(&status, &productID); err != nil {
		return err
	}
	if status != "pending" && status != "approved" {
		return errors.New("申請中または承認済みの依頼だけ終了できます")
	}
	now := s.now().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `
		UPDATE purchase_requests SET status=?,reviewed_at=?,reviewed_by=?,
			rejection_reason=CASE WHEN ?='rejected' THEN ? ELSE rejection_reason END,
			cancelled_at=CASE WHEN ?='cancelled' THEN ? ELSE cancelled_at END,
			cancel_reason=CASE WHEN ?='cancelled' THEN ? ELSE cancel_reason END,updated_at=?
		WHERE id=? AND organization_id=?`,
		toStatus, now, actorID, toStatus, reason, toStatus, now, toStatus, reason, now, requestID, organizationID); err != nil {
		return err
	}
	var reservationID string
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM reservations WHERE purchase_request_id=? AND organization_id=? AND status='active'`,
		requestID, organizationID).Scan(&reservationID)
	if err == nil {
		if _, err := tx.ExecContext(ctx, `
			UPDATE reservations SET status='released',released_at=?,release_reason=?,updated_at=? WHERE id=?`,
			now, reason, now, reservationID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE products SET inventory_status='in_stock',updated_at=?
			WHERE id=? AND organization_id=? AND inventory_status='reserved'`,
			now, productID, organizationID); err != nil {
			return err
		}
		if err := addInventoryEventTx(ctx, tx, organizationID, productID, actorID,
			"reservation.released", "reserved", "in_stock", reason, now); err != nil {
			return err
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	return tx.Commit()
}

func (s *Store) ExpireReservations(ctx context.Context, organizationID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := s.now().Format(time.RFC3339Nano)
	rows, err := tx.QueryContext(ctx, `
		SELECT id,product_id,purchase_request_id,created_by FROM reservations
		WHERE organization_id=? AND status='active' AND expires_at<=?`, organizationID, now)
	if err != nil {
		return err
	}
	type expired struct{ id, productID, requestID, actorID string }
	var values []expired
	for rows.Next() {
		var value expired
		if err := rows.Scan(&value.id, &value.productID, &value.requestID, &value.actorID); err != nil {
			rows.Close()
			return err
		}
		values = append(values, value)
	}
	rows.Close()
	for _, value := range values {
		if _, err := tx.ExecContext(ctx, `
			UPDATE reservations SET status='expired',released_at=?,release_reason='期限切れ',updated_at=?
			WHERE id=? AND status='active'`, now, now, value.id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE purchase_requests SET status='expired',updated_at=?
			WHERE id=? AND organization_id=? AND status='approved'`, now, value.requestID, organizationID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE products SET inventory_status='in_stock',updated_at=?
			WHERE id=? AND organization_id=? AND inventory_status='reserved'`,
			now, value.productID, organizationID); err != nil {
			return err
		}
		if err := addInventoryEventTx(ctx, tx, organizationID, value.productID, value.actorID,
			"reservation.expired", "reserved", "in_stock", "取置期限切れ", now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func reservationDurationHoursTx(ctx context.Context, tx *sql.Tx, organizationID string) (int64, error) {
	var raw string
	var configured bool
	err := tx.QueryRowContext(ctx, `
		SELECT setting_value,is_configured FROM organization_settings
		WHERE organization_id=? AND setting_key='reservation.duration_hours'`, organizationID).
		Scan(&raw, &configured)
	if err != nil {
		return 0, err
	}
	if !configured || strings.TrimSpace(raw) == "" {
		return 24, nil
	}
	hours, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || hours < 1 {
		return 0, errors.New("取置期限設定が正しくありません")
	}
	return hours, nil
}

func addInventoryEventTx(ctx context.Context, tx *sql.Tx, organizationID, productID, actorID, eventType, from, to, reason, now string) error {
	eventID, _ := NewID("evt")
	_, err := tx.ExecContext(ctx, `
		INSERT INTO inventory_events(
			id,organization_id,product_id,event_type,from_status,to_status,reason,actor_user_id,created_at
		) VALUES(?,?,?,?,?,?,?,?,?)`,
		eventID, organizationID, productID, eventType, from, to, reason, actorID, now)
	return err
}

func (s *Store) SeedRequestPreview(ctx context.Context) error {
	products, err := s.Products(ctx, "org_preview", ProductFilter{Status: "in_stock"})
	if err != nil {
		return err
	}
	if len(products) == 0 {
		return nil
	}
	for _, product := range products {
		if product.PublicationStatus == "public" {
			return nil
		}
	}
	return s.SetProductPublication(ctx, "org_preview", products[0].ID, "usr_admin", "public")
}
