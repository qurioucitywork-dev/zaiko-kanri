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
	SerialNumber   string
	ProductType    string
	SalePriceMinor int64
	SaleCurrency   string
	Condition      string
	Accessories    string
	Images         []ProductImage
}

type PublicProductFilter struct {
	Query     string
	Brand     string
	Condition string
}

type PurchaseRequestInput struct {
	OrganizationCode string
	GuestCompanyID   string
	RequestGroupID   string
	ProductID        string
	GuestName        string
	GuestEmail       string
	GuestPhone       string
	Message          string
}

type PurchaseRequestGroupInput struct {
	OrganizationCode string
	GuestCompanyID   string
	RequestGroupID   string
	ProductIDs       []string
	GuestName        string
	GuestEmail       string
	GuestPhone       string
	Message          string
}

type PurchaseRequest struct {
	ID                 string
	RequestGroupID     string
	OrganizationID     string
	ProductID          string
	ProductCode        string
	Brand              string
	ModelNumber        string
	CostAmountMinor    int64
	CostCurrency       string
	SalePriceMinor     int64
	SaleCurrency       string
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

type PurchaseRequestGroup struct {
	ID    string
	Items []PurchaseRequest
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
	return s.PublicProductsFiltered(ctx, organizationCode, PublicProductFilter{})
}

func (s *Store) PublicProductsForGuest(ctx context.Context, organizationCode, guestCompanyCode string, filter PublicProductFilter) ([]PublicProduct, error) {
	query := `
		SELECT DISTINCT bp.product_id,bp.product_code,bp.brand,bp.reference_number,bp.serial_number,bp.model_name,
		       bp.sale_price_minor,bp.sale_currency,bp.condition_text,bp.accessories
		FROM guest_box_published_products bp
		JOIN organizations o ON o.id=bp.organization_id
		JOIN guest_box_publications pub
		  ON pub.organization_id=bp.organization_id AND pub.company_id=bp.company_id
		  AND pub.box_id=bp.box_id AND pub.is_published=1
		JOIN guest_companies c
		  ON c.organization_id=pub.organization_id AND c.id=pub.company_id AND c.is_active=1
		WHERE o.code=? AND o.is_active=1 AND c.company_code=?`
	args := []any{
		strings.ToUpper(strings.TrimSpace(organizationCode)),
		strings.ToUpper(strings.TrimSpace(guestCompanyCode)),
	}
	if strings.TrimSpace(filter.Query) != "" {
		like := "%" + strings.TrimSpace(filter.Query) + "%"
		query += ` AND (bp.brand LIKE ? OR bp.reference_number LIKE ? OR bp.model_name LIKE ?)`
		args = append(args, like, like, like)
	}
	if strings.TrimSpace(filter.Brand) != "" {
		query += ` AND bp.brand=?`
		args = append(args, strings.TrimSpace(filter.Brand))
	}
	if strings.TrimSpace(filter.Condition) != "" {
		query += ` AND bp.condition_text=?`
		args = append(args, strings.TrimSpace(filter.Condition))
	}
	query += ` ORDER BY bp.product_code DESC LIMIT 200`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var products []PublicProduct
	for rows.Next() {
		var product PublicProduct
		if err := rows.Scan(&product.ID, &product.ProductCode, &product.Brand, &product.ModelNumber,
			&product.SerialNumber, &product.ProductType, &product.SalePriceMinor,
			&product.SaleCurrency, &product.Condition, &product.Accessories); err != nil {
			return nil, err
		}
		products = append(products, product)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for index := range products {
		images, imageErr := s.GuestSnapshotProductImages(ctx, organizationCode, guestCompanyCode, products[index].ID)
		if imageErr != nil {
			return nil, imageErr
		}
		products[index].Images = images
	}
	return products, nil
}

func (s *Store) GuestSnapshotProductImages(ctx context.Context, organizationCode, guestCompanyCode, productID string) ([]ProductImage, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT i.image_id,i.product_id,i.storage_path,i.original_name,
		       i.content_type,i.size_bytes,i.sort_order
		FROM guest_box_published_images i
		JOIN organizations o ON o.id=i.organization_id AND o.is_active=1
		JOIN guest_companies c
		  ON c.organization_id=i.organization_id AND c.id=i.company_id AND c.is_active=1
		JOIN guest_box_publications pub
		  ON pub.organization_id=i.organization_id AND pub.company_id=i.company_id
		  AND pub.box_id=i.box_id AND pub.is_published=1
		WHERE o.code=? AND c.company_code=? AND i.product_id=?
		ORDER BY i.sort_order`,
		strings.ToUpper(strings.TrimSpace(organizationCode)),
		strings.ToUpper(strings.TrimSpace(guestCompanyCode)), productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var images []ProductImage
	for rows.Next() {
		var image ProductImage
		if err := rows.Scan(&image.ID, &image.ProductID, &image.StoragePath, &image.OriginalName,
			&image.ContentType, &image.SizeBytes, &image.SortOrder); err != nil {
			return nil, err
		}
		images = append(images, image)
	}
	return images, rows.Err()
}

func (s *Store) PublicProductsFiltered(ctx context.Context, organizationCode string, filter PublicProductFilter) ([]PublicProduct, error) {
	query := `
		SELECT p.id,p.product_code,p.brand,p.model_number,p.serial_number,p.product_type,
		       p.base_sale_price_minor,p.base_sale_currency,p.condition_text,p.accessories
		FROM products p JOIN organizations o ON o.id=p.organization_id
		WHERE o.code=? AND o.is_active=1 AND p.publication_status='public'
		  AND p.inventory_status='in_stock' AND p.deleted_at IS NULL`
	args := []any{strings.ToUpper(strings.TrimSpace(organizationCode))}
	if strings.TrimSpace(filter.Query) != "" {
		like := "%" + strings.TrimSpace(filter.Query) + "%"
		query += ` AND (p.brand LIKE ? OR p.model_number LIKE ? OR p.product_type LIKE ?)`
		args = append(args, like, like, like)
	}
	if strings.TrimSpace(filter.Brand) != "" {
		query += ` AND p.brand=?`
		args = append(args, strings.TrimSpace(filter.Brand))
	}
	if strings.TrimSpace(filter.Condition) != "" {
		query += ` AND p.condition_text=?`
		args = append(args, strings.TrimSpace(filter.Condition))
	}
	query += ` ORDER BY p.purchase_date DESC,p.product_code DESC LIMIT 200`
	rows, err := s.db.QueryContext(ctx, `
		`+query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var products []PublicProduct
	for rows.Next() {
		var product PublicProduct
		if err := rows.Scan(&product.ID, &product.ProductCode, &product.Brand, &product.ModelNumber, &product.SerialNumber,
			&product.ProductType, &product.SalePriceMinor, &product.SaleCurrency,
			&product.Condition, &product.Accessories); err != nil {
			return nil, err
		}
		products = append(products, product)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for index := range products {
		images, imageErr := s.PublicProductImages(ctx, organizationCode, products[index].ID)
		if imageErr != nil {
			return nil, imageErr
		}
		products[index].Images = images
	}
	return products, nil
}

func (s *Store) PublicProductImages(ctx context.Context, organizationCode, productID string) ([]ProductImage, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT i.id,i.product_id,i.storage_path,i.original_name,i.content_type,i.size_bytes,i.sort_order
		FROM product_images i
		JOIN products p ON p.id=i.product_id AND p.organization_id=i.organization_id
		JOIN organizations o ON o.id=p.organization_id
		WHERE o.code=? AND o.is_active=1 AND p.id=? AND p.publication_status='public'
		  AND p.inventory_status='in_stock' AND p.deleted_at IS NULL
		ORDER BY i.sort_order`, strings.ToUpper(strings.TrimSpace(organizationCode)), productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var images []ProductImage
	for rows.Next() {
		var image ProductImage
		if err := rows.Scan(&image.ID, &image.ProductID, &image.StoragePath, &image.OriginalName,
			&image.ContentType, &image.SizeBytes, &image.SortOrder); err != nil {
			return nil, err
		}
		images = append(images, image)
	}
	return images, rows.Err()
}

func (s *Store) PublicProductImage(ctx context.Context, organizationCode, imageID string) (ProductImage, error) {
	var image ProductImage
	err := s.db.QueryRowContext(ctx, `
		SELECT i.id,i.product_id,i.storage_path,i.original_name,i.content_type,i.size_bytes,i.sort_order
		FROM product_images i
		JOIN products p ON p.id=i.product_id AND p.organization_id=i.organization_id
		JOIN organizations o ON o.id=p.organization_id
		WHERE o.code=? AND o.is_active=1 AND i.id=? AND p.publication_status='public'
		  AND p.inventory_status='in_stock' AND p.deleted_at IS NULL`,
		strings.ToUpper(strings.TrimSpace(organizationCode)), imageID).
		Scan(&image.ID, &image.ProductID, &image.StoragePath, &image.OriginalName,
			&image.ContentType, &image.SizeBytes, &image.SortOrder)
	if err == nil {
		return image, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return image, err
	}
	err = s.db.QueryRowContext(ctx, `
		SELECT i.image_id,i.product_id,i.storage_path,i.original_name,i.content_type,i.size_bytes,i.sort_order
		FROM guest_box_published_images i
		JOIN organizations o ON o.id=i.organization_id AND o.is_active=1
		WHERE o.code=? AND i.image_id=?
		ORDER BY i.published_at DESC LIMIT 1`,
		strings.ToUpper(strings.TrimSpace(organizationCode)), imageID).
		Scan(&image.ID, &image.ProductID, &image.StoragePath, &image.OriginalName,
			&image.ContentType, &image.SizeBytes, &image.SortOrder)
	return image, err
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
	availabilityQuery := `
		SELECT p.organization_id FROM products p JOIN organizations o ON o.id=p.organization_id
		WHERE p.id=? AND o.code=? AND o.is_active=1 AND p.publication_status='public'
		  AND p.inventory_status='in_stock' AND p.deleted_at IS NULL`
	availabilityArgs := []any{input.ProductID, strings.ToUpper(strings.TrimSpace(input.OrganizationCode))}
	if strings.TrimSpace(input.GuestCompanyID) != "" {
		availabilityQuery = `
			SELECT bp.organization_id
			FROM guest_box_published_products bp
			JOIN guest_box_publications pub
			  ON pub.organization_id=bp.organization_id AND pub.company_id=bp.company_id
			  AND pub.box_id=bp.box_id AND pub.is_published=1
			JOIN guest_companies c
			  ON c.organization_id=bp.organization_id AND c.id=bp.company_id AND c.is_active=1
			JOIN organizations o ON o.id=bp.organization_id AND o.is_active=1
			WHERE bp.product_id=? AND bp.company_id=? AND o.code=?
			LIMIT 1`
		availabilityArgs = []any{input.ProductID, strings.TrimSpace(input.GuestCompanyID),
			strings.ToUpper(strings.TrimSpace(input.OrganizationCode))}
	}
	if err := tx.QueryRowContext(ctx, availabilityQuery, availabilityArgs...).Scan(&organizationID); err != nil {
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
	requestGroupID := strings.TrimSpace(input.RequestGroupID)
	if requestGroupID == "" {
		requestGroupID, _ = NewID("reqgrp")
	}
	if len(requestGroupID) > 100 {
		return PurchaseRequest{}, errors.New("購入依頼グループIDが正しくありません")
	}
	number := fmt.Sprintf("RQ-%s-%04d", nowTime.Format("2006"), count+1)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO purchase_requests(
			id,organization_id,request_group_id,product_id,request_number,guest_name,guest_email,guest_phone,message,
			status,requested_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,'pending',?,?)`,
		id, organizationID, requestGroupID, input.ProductID, number, input.GuestName, input.GuestEmail,
		strings.TrimSpace(input.GuestPhone), strings.TrimSpace(input.Message), now, now); err != nil {
		return PurchaseRequest{}, err
	}
	if err := tx.Commit(); err != nil {
		return PurchaseRequest{}, err
	}
	return s.PurchaseRequest(ctx, organizationID, id)
}

func (s *Store) CreatePurchaseRequestGroup(ctx context.Context, input PurchaseRequestGroupInput) (PurchaseRequestGroup, error) {
	input.GuestName = strings.TrimSpace(input.GuestName)
	input.GuestEmail = strings.TrimSpace(strings.ToLower(input.GuestEmail))
	if input.GuestName == "" || !strings.Contains(input.GuestEmail, "@") || len(input.GuestEmail) > 254 {
		return PurchaseRequestGroup{}, errors.New("お名前と正しいメールアドレスを入力してください")
	}
	if len([]rune(input.GuestName)) > 100 || len([]rune(input.GuestPhone)) > 40 || len([]rune(input.Message)) > 2000 {
		return PurchaseRequestGroup{}, errors.New("入力文字数を確認してください")
	}
	productIDs := make([]string, 0, len(input.ProductIDs))
	seen := make(map[string]struct{}, len(input.ProductIDs))
	for _, rawID := range input.ProductIDs {
		productID := strings.TrimSpace(rawID)
		if productID == "" {
			continue
		}
		if _, exists := seen[productID]; exists {
			continue
		}
		seen[productID] = struct{}{}
		productIDs = append(productIDs, productID)
	}
	if len(productIDs) == 0 {
		return PurchaseRequestGroup{}, errors.New("購入依頼する商品を選択してください")
	}
	if len(productIDs) > 100 {
		return PurchaseRequestGroup{}, errors.New("一度に購入依頼できる商品は100点までです")
	}
	requestGroupID := strings.TrimSpace(input.RequestGroupID)
	if requestGroupID == "" {
		requestGroupID, _ = NewID("reqgrp")
	}
	if len(requestGroupID) > 100 {
		return PurchaseRequestGroup{}, errors.New("購入依頼グループIDが正しくありません")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PurchaseRequestGroup{}, err
	}
	defer tx.Rollback()
	var organizationID string
	for index, productID := range productIDs {
		availabilityQuery := `
			SELECT p.organization_id FROM products p JOIN organizations o ON o.id=p.organization_id
			WHERE p.id=? AND o.code=? AND o.is_active=1 AND p.publication_status='public'
			  AND p.inventory_status='in_stock' AND p.deleted_at IS NULL`
		availabilityArgs := []any{productID, strings.ToUpper(strings.TrimSpace(input.OrganizationCode))}
		if strings.TrimSpace(input.GuestCompanyID) != "" {
			availabilityQuery = `
				SELECT bp.organization_id
				FROM guest_box_published_products bp
				JOIN guest_box_publications pub
				  ON pub.organization_id=bp.organization_id AND pub.company_id=bp.company_id
				  AND pub.box_id=bp.box_id AND pub.is_published=1
				JOIN guest_companies c
				  ON c.organization_id=bp.organization_id AND c.id=bp.company_id AND c.is_active=1
				JOIN organizations o ON o.id=bp.organization_id AND o.is_active=1
				WHERE bp.product_id=? AND bp.company_id=? AND o.code=?
				LIMIT 1`
			availabilityArgs = []any{productID, strings.TrimSpace(input.GuestCompanyID),
				strings.ToUpper(strings.TrimSpace(input.OrganizationCode))}
		}
		var candidateOrganizationID string
		if err := tx.QueryRowContext(ctx, availabilityQuery, availabilityArgs...).Scan(&candidateOrganizationID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return PurchaseRequestGroup{}, ErrProductNotAvailable
			}
			return PurchaseRequestGroup{}, err
		}
		if index == 0 {
			organizationID = candidateOrganizationID
		} else if candidateOrganizationID != organizationID {
			return PurchaseRequestGroup{}, ErrProductNotAvailable
		}
	}

	nowTime := s.now()
	now := nowTime.Format(time.RFC3339Nano)
	var count int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM purchase_requests
		WHERE organization_id=? AND substr(requested_at,1,4)=?`,
		organizationID, nowTime.Format("2006")).Scan(&count); err != nil {
		return PurchaseRequestGroup{}, err
	}
	requestIDs := make([]string, 0, len(productIDs))
	for index, productID := range productIDs {
		id, _ := NewID("reqbuy")
		number := fmt.Sprintf("RQ-%s-%04d", nowTime.Format("2006"), count+index+1)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO purchase_requests(
				id,organization_id,request_group_id,product_id,request_number,guest_name,guest_email,guest_phone,message,
				status,requested_at,updated_at
			) VALUES(?,?,?,?,?,?,?,?,?,'pending',?,?)`,
			id, organizationID, requestGroupID, productID, number, input.GuestName, input.GuestEmail,
			strings.TrimSpace(input.GuestPhone), strings.TrimSpace(input.Message), now, now); err != nil {
			return PurchaseRequestGroup{}, err
		}
		requestIDs = append(requestIDs, id)
	}
	if err := tx.Commit(); err != nil {
		return PurchaseRequestGroup{}, err
	}
	group := PurchaseRequestGroup{ID: requestGroupID, Items: make([]PurchaseRequest, 0, len(requestIDs))}
	for _, requestID := range requestIDs {
		request, err := s.PurchaseRequest(ctx, organizationID, requestID)
		if err != nil {
			return PurchaseRequestGroup{}, err
		}
		group.Items = append(group.Items, request)
	}
	return group, nil
}

func (s *Store) PurchaseRequestGroups(ctx context.Context, organizationID, status string) ([]PurchaseRequestGroup, error) {
	requests, err := s.PurchaseRequests(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	groups := make([]PurchaseRequestGroup, 0, len(requests))
	indexByID := make(map[string]int, len(requests))
	for _, request := range requests {
		if status != "" && request.Status != status {
			continue
		}
		groupID := request.RequestGroupID
		if groupID == "" {
			groupID = request.ID
		}
		index, exists := indexByID[groupID]
		if !exists {
			index = len(groups)
			indexByID[groupID] = index
			groups = append(groups, PurchaseRequestGroup{ID: groupID})
		}
		groups[index].Items = append(groups[index].Items, request)
	}
	return groups, nil
}

func (s *Store) PurchaseRequestGroup(ctx context.Context, organizationID, groupID string) (PurchaseRequestGroup, error) {
	rows, err := s.db.QueryContext(ctx, purchaseRequestSelect+`
		WHERE r.organization_id=? AND r.request_group_id=?
		ORDER BY r.requested_at,r.request_number`, organizationID, groupID)
	if err != nil {
		return PurchaseRequestGroup{}, err
	}
	defer rows.Close()
	group := PurchaseRequestGroup{ID: groupID}
	for rows.Next() {
		request, scanErr := scanPurchaseRequest(rows)
		if scanErr != nil {
			return PurchaseRequestGroup{}, scanErr
		}
		group.Items = append(group.Items, request)
	}
	if err := rows.Err(); err != nil {
		return PurchaseRequestGroup{}, err
	}
	if len(group.Items) == 0 {
		return PurchaseRequestGroup{}, sql.ErrNoRows
	}
	return group, nil
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
	SELECT r.id,r.request_group_id,r.organization_id,r.product_id,p.product_code,p.brand,p.model_number,
	       p.cost_amount_minor,p.cost_currency,p.base_sale_price_minor,p.base_sale_currency,r.request_number,
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
	err := row.Scan(&request.ID, &request.RequestGroupID, &request.OrganizationID, &request.ProductID, &request.ProductCode,
		&request.Brand, &request.ModelNumber, &request.CostAmountMinor, &request.CostCurrency,
		&request.SalePriceMinor, &request.SaleCurrency,
		&request.RequestNumber, &request.GuestName,
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
		) VALUES(?,?,?,?,'active',?,?,?,?,?)
		ON CONFLICT(purchase_request_id) DO UPDATE SET
			id=excluded.id,product_id=excluded.product_id,status='active',
			starts_at=excluded.starts_at,expires_at=excluded.expires_at,released_at=NULL,
			release_reason='',created_by=excluded.created_by,updated_at=excluded.updated_at`,
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
	// Purchase requests do not expose an "expired" business status. Reopen any
	// legacy expired request so it can be decided again after its reservation was
	// released. The reservation itself keeps its expiry audit state.
	if _, err := tx.ExecContext(ctx, `
		UPDATE purchase_requests
		SET status='pending',reviewed_at=NULL,reviewed_by=NULL,updated_at=?
		WHERE organization_id=? AND status='expired'`, now, organizationID); err != nil {
		return err
	}
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
			UPDATE purchase_requests
			SET status='pending',reviewed_at=NULL,reviewed_by=NULL,updated_at=?
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
		if product.PublicationStatus != "public" {
			if err := s.SetProductPublication(ctx, "org_preview", product.ID, "usr_admin", "public"); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) SeedGuestCatalogPreview(ctx context.Context) error {
	seeds := []SingleProductInput{
		{SupplierID: "sup_001", SKU: "GUEST-ROLEX-DAYTONA", Brand: "ロレックス", ModelNumber: "116519LN", SerialNumber: "GUEST-RLX-002", ProductType: "デイトナ（ホワイトゴールド）", CostAmountMinor: 1680000, BaseSalePriceMinor: 2200000, Condition: "未使用展示品 (N-)", Accessories: "BOX, GUARANTEE"},
		{SupplierID: "sup_002", SKU: "GUEST-OMEGA-SPEED", Brand: "オメガ", ModelNumber: "311.30.42.30.01.005", SerialNumber: "GUEST-OMG-001", ProductType: "スピードマスター", CostAmountMinor: 320000, BaseSalePriceMinor: 498000, Condition: "美品 (A)", Accessories: "BOX"},
		{SupplierID: "sup_003", SKU: "GUEST-CARTIER-SANTOS", Brand: "カルティエ", ModelNumber: "WSSA0009", SerialNumber: "GUEST-CAR-001", ProductType: "サントス", CostAmountMinor: 480000, BaseSalePriceMinor: 720000, Condition: "美品 (A)", Accessories: "BOX, GUARANTEE"},
		{SupplierID: "sup_001", SKU: "GUEST-IWC-PORT", Brand: "IWC", ModelNumber: "IW500705", SerialNumber: "GUEST-IWC-001", ProductType: "ポルトギーゼ", CostAmountMinor: 560000, BaseSalePriceMinor: 840000, Condition: "美品 (A)", Accessories: "BOX"},
		{SupplierID: "sup_002", SKU: "GUEST-GS-ELEGANCE", Brand: "グランドセイコー", ModelNumber: "SBGW047", SerialNumber: "GUEST-GS-001", ProductType: "エレガンスコレクション", CostAmountMinor: 280000, BaseSalePriceMinor: 430000, Condition: "極美品 (S)", Accessories: "BOX, GUARANTEE"},
		{SupplierID: "sup_003", SKU: "GUEST-BREITLING-NAVI", Brand: "ブライトリング", ModelNumber: "AB0121211B1A1", SerialNumber: "GUEST-BRI-001", ProductType: "ナビタイマー", CostAmountMinor: 420000, BaseSalePriceMinor: 610000, Condition: "良品 (AB)", Accessories: "GUARANTEE"},
		{SupplierID: "sup_001", SKU: "GUEST-BREITLING-BLACK", Brand: "ブライトリング", ModelNumber: "AB0127211B1A1", SerialNumber: "GUEST-BRI-002", ProductType: "ナビタイマー（ブラック）", CostAmountMinor: 480000, BaseSalePriceMinor: 720000, Condition: "極美品 (S)", Accessories: "BOX, GUARANTEE"},
	}
	for _, seed := range seeds {
		var productID, status, publication string
		err := s.db.QueryRowContext(ctx, `
			SELECT id,inventory_status,publication_status FROM products
			WHERE organization_id='org_preview' AND sku=? AND deleted_at IS NULL`, seed.SKU).
			Scan(&productID, &status, &publication)
		if errors.Is(err, sql.ErrNoRows) {
			seed.OrganizationID = "org_preview"
			seed.PurchaseDate = "2026-07-27"
			seed.CostCurrency = "JPY"
			seed.BaseSaleCurrency = "JPY"
			seed.CreatedBy = "usr_admin"
			product, createErr := s.CreateSingleProduct(ctx, seed)
			if createErr != nil {
				return createErr
			}
			productID, status, publication = product.ID, product.InventoryStatus, product.PublicationStatus
		} else if err != nil {
			return err
		}
		if status == "in_stock" && publication != "public" {
			if err := s.SetProductPublication(ctx, "org_preview", productID, "usr_admin", "public"); err != nil {
				return err
			}
		}
	}
	return nil
}
