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
	ErrGuestCompanyNotFound = errors.New("ゲスト企業が見つかりません")
	ErrGuestBoxNotFound     = errors.New("BOXが見つかりません")
	ErrGuestBoxEmpty        = errors.New("商品がないBOXは公開できません")
)

type GuestCompany struct {
	ID        string
	Code      string
	Name      string
	IsActive  bool
	UpdatedAt time.Time
}

type GuestBox struct {
	ID                    string
	Number                int
	Code                  string
	Name                  string
	ProductCount          int
	PublishedCompanyCount int
	UpdatedAt             time.Time
}

type GuestBoxMatrixCell struct {
	CompanyID   string
	BoxID       string
	Draft       bool
	Published   bool
	CanPublish  bool
	PublishedAt *time.Time
}

type GuestBoxProduct struct {
	ID                string
	ProductID         string
	ProductCode       string
	Brand             string
	Model             string
	ReferenceNumber   string
	PurchaseDate      string
	SalePriceMinor    int64
	SaleCurrency      string
	Condition         string
	InventoryStatus   string
	PublicationStatus string
	SortOrder         int
	AssignedBoxID     string
	AssignedBoxCode   string
}

type GuestPublicationSummary struct {
	LastPublishedAt       *time.Time
	PublishedBoxCount     int
	PublishedProductCount int
}

func (s *Store) GuestPublicationSummary(ctx context.Context, organizationID string) (GuestPublicationSummary, error) {
	var summary GuestPublicationSummary
	var last sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT MAX(published_at),
		       COUNT(DISTINCT CASE WHEN is_published=1 THEN box_id END)
		FROM guest_box_publications WHERE organization_id=?`, organizationID).
		Scan(&last, &summary.PublishedBoxCount)
	if err != nil {
		return summary, err
	}
	if last.Valid {
		value, parseErr := time.Parse(time.RFC3339Nano, last.String)
		if parseErr == nil {
			summary.LastPublishedAt = &value
		}
	}
	err = s.db.QueryRowContext(ctx, `
		SELECT COUNT(DISTINCT product_id)
		FROM guest_box_published_products WHERE organization_id=?`, organizationID).
		Scan(&summary.PublishedProductCount)
	return summary, err
}

func (s *Store) GuestBoxPublishedCompanies(ctx context.Context, organizationID, boxID string) ([]GuestCompany, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id,c.company_code,c.name,c.is_active,c.updated_at
		FROM guest_box_publications pub
		JOIN guest_companies c ON c.organization_id=pub.organization_id AND c.id=pub.company_id
		WHERE pub.organization_id=? AND pub.box_id=? AND pub.is_published=1 AND c.is_active=1
		ORDER BY c.company_code`, organizationID, boxID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var companies []GuestCompany
	for rows.Next() {
		var company GuestCompany
		var active int
		var updated string
		if err := rows.Scan(&company.ID, &company.Code, &company.Name, &active, &updated); err != nil {
			return nil, err
		}
		company.IsActive = active == 1
		company.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		companies = append(companies, company)
	}
	return companies, rows.Err()
}

func (s *Store) GuestCompanies(ctx context.Context, organizationID string) ([]GuestCompany, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id,company_code,name,is_active,updated_at
		FROM guest_companies WHERE organization_id=? AND is_active=1 ORDER BY company_code`, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var companies []GuestCompany
	for rows.Next() {
		var company GuestCompany
		var active int
		var updated string
		if err := rows.Scan(&company.ID, &company.Code, &company.Name, &active, &updated); err != nil {
			return nil, err
		}
		company.IsActive = active == 1
		company.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		companies = append(companies, company)
	}
	return companies, rows.Err()
}

func (s *Store) GuestBoxes(ctx context.Context, organizationID string) ([]GuestBox, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT b.id,b.box_number,b.box_name,b.updated_at,
		       (SELECT COUNT(*) FROM guest_box_products bp
		        WHERE bp.organization_id=b.organization_id AND bp.box_id=b.id),
		       (SELECT COUNT(*) FROM guest_box_publications pub
		        WHERE pub.organization_id=b.organization_id AND pub.box_id=b.id AND pub.is_published=1)
		FROM guest_boxes b WHERE b.organization_id=? ORDER BY b.box_number`, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var boxes []GuestBox
	for rows.Next() {
		var box GuestBox
		var updated string
		if err := rows.Scan(&box.ID, &box.Number, &box.Name, &updated,
			&box.ProductCount, &box.PublishedCompanyCount); err != nil {
			return nil, err
		}
		box.Code = fmt.Sprintf("BOX%d", box.Number)
		box.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		boxes = append(boxes, box)
	}
	return boxes, rows.Err()
}

func (s *Store) GuestBoxMatrix(ctx context.Context, organizationID string) ([]GuestBoxMatrixCell, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id,b.id,
		       COALESCE(d.is_selected,0),COALESCE(pub.is_published,0),
		       CASE WHEN EXISTS(
		         SELECT 1 FROM guest_box_products bp
		         JOIN products p ON p.id=bp.product_id AND p.organization_id=bp.organization_id
		         WHERE bp.organization_id=b.organization_id AND bp.box_id=b.id
		           AND p.deleted_at IS NULL
		       ) THEN 1 ELSE 0 END,
		       pub.published_at
		FROM guest_companies c CROSS JOIN guest_boxes b
		LEFT JOIN guest_box_drafts d
		  ON d.organization_id=c.organization_id AND d.company_id=c.id AND d.box_id=b.id
		LEFT JOIN guest_box_publications pub
		  ON pub.organization_id=c.organization_id AND pub.company_id=c.id AND pub.box_id=b.id
		WHERE c.organization_id=? AND c.is_active=1 AND b.organization_id=c.organization_id
		ORDER BY c.company_code,b.box_number`, organizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cells []GuestBoxMatrixCell
	for rows.Next() {
		var cell GuestBoxMatrixCell
		var draft, published, canPublish int
		var publishedAt sql.NullString
		if err := rows.Scan(&cell.CompanyID, &cell.BoxID, &draft, &published, &canPublish, &publishedAt); err != nil {
			return nil, err
		}
		cell.Draft = draft == 1
		cell.Published = published == 1
		cell.CanPublish = canPublish == 1
		if publishedAt.Valid {
			value, _ := time.Parse(time.RFC3339Nano, publishedAt.String)
			cell.PublishedAt = &value
		}
		cells = append(cells, cell)
	}
	return cells, rows.Err()
}

func (s *Store) SaveGuestBoxDraft(ctx context.Context, organizationID, companyID, boxID, actorID string, selected bool) error {
	var productCount int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(bp.product_id)
		FROM guest_companies c JOIN guest_boxes b ON b.organization_id=c.organization_id
		LEFT JOIN guest_box_products bp ON bp.organization_id=b.organization_id AND bp.box_id=b.id
		WHERE c.organization_id=? AND c.id=? AND c.is_active=1 AND b.id=?
		GROUP BY c.id,b.id`, organizationID, companyID, boxID).Scan(&productCount); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrGuestBoxNotFound
		}
		return err
	}
	if selected && productCount == 0 {
		return ErrGuestBoxEmpty
	}
	value := 0
	if selected {
		value = 1
	}
	now := s.now().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO guest_box_drafts(organization_id,company_id,box_id,is_selected,updated_by,updated_at)
		VALUES(?,?,?,?,?,?)
		ON CONFLICT(organization_id,company_id,box_id) DO UPDATE SET
		  is_selected=excluded.is_selected,updated_by=excluded.updated_by,updated_at=excluded.updated_at`,
		organizationID, companyID, boxID, value, actorID, now)
	return err
}

func (s *Store) PublishGuestBoxSnapshot(ctx context.Context, organizationID, actorID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var invalid int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM guest_box_drafts d
		WHERE d.organization_id=? AND d.is_selected=1 AND NOT EXISTS(
		  SELECT 1 FROM guest_box_products bp
		  JOIN products p ON p.id=bp.product_id AND p.organization_id=bp.organization_id
		  WHERE bp.organization_id=d.organization_id AND bp.box_id=d.box_id
		    AND p.deleted_at IS NULL
		)`, organizationID).Scan(&invalid); err != nil {
		return err
	}
	if invalid > 0 {
		return ErrGuestBoxEmpty
	}
	now := s.now().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO guest_box_publications(
		  organization_id,company_id,box_id,is_published,published_by,published_at
		)
		SELECT organization_id,company_id,box_id,is_selected,?,?
		FROM guest_box_drafts WHERE organization_id=?
		ON CONFLICT(organization_id,company_id,box_id) DO UPDATE SET
		  is_published=excluded.is_published,published_by=excluded.published_by,
		  published_at=excluded.published_at`, actorID, now, organizationID); err != nil {
		return err
	}
	// The public line-up is a snapshot too. Editing a BOX after publication must
	// never change what a guest sees until the next explicit batch publish.
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM guest_box_published_products WHERE organization_id=?`, organizationID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM guest_box_published_images WHERE organization_id=?`, organizationID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO guest_box_published_products(
		  organization_id,company_id,box_id,product_id,sort_order,published_by,published_at,
		  product_code,brand,model_name,reference_number,serial_number,sale_price_minor,
		  sale_currency,condition_text,accessories,inventory_status,publication_status,box_name
		)
		SELECT d.organization_id,d.company_id,d.box_id,bp.product_id,bp.sort_order,?,?,
		       p.product_code,p.brand,p.product_type,p.model_number,p.serial_number,
		       p.base_sale_price_minor,p.base_sale_currency,p.condition_text,p.accessories,
		       p.inventory_status,p.publication_status,b.box_name
		FROM guest_box_drafts d
		JOIN guest_box_products bp
		  ON bp.organization_id=d.organization_id AND bp.box_id=d.box_id
		JOIN guest_boxes b
		  ON b.organization_id=bp.organization_id AND b.id=bp.box_id
		JOIN products p
		  ON p.organization_id=bp.organization_id AND p.id=bp.product_id
		WHERE d.organization_id=? AND d.is_selected=1
		  AND p.deleted_at IS NULL`,
		actorID, now, organizationID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO guest_box_published_images(
		  organization_id,company_id,box_id,product_id,image_id,storage_path,
		  original_name,content_type,size_bytes,sort_order,published_at
		)
		SELECT d.organization_id,d.company_id,d.box_id,bp.product_id,i.id,i.storage_path,
		       i.original_name,i.content_type,i.size_bytes,i.sort_order,?
		FROM guest_box_drafts d
		JOIN guest_box_products bp
		  ON bp.organization_id=d.organization_id AND bp.box_id=d.box_id
		JOIN product_images i
		  ON i.organization_id=bp.organization_id AND i.product_id=bp.product_id
		WHERE d.organization_id=? AND d.is_selected=1`,
		now, organizationID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) RenameGuestBox(ctx context.Context, organizationID, boxID, actorID, name string) error {
	name = strings.TrimSpace(name)
	if len([]rune(name)) > 100 {
		return errors.New("BOX名は100文字以内で入力してください")
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE guest_boxes SET box_name=?,updated_by=?,updated_at=?
		WHERE id=? AND organization_id=?`,
		name, actorID, s.now().Format(time.RFC3339Nano), boxID, organizationID)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrGuestBoxNotFound
	}
	return nil
}

func (s *Store) GuestBoxProducts(ctx context.Context, organizationID, boxID string) ([]GuestBoxProduct, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT bp.product_id,p.id,p.product_code,p.brand,p.product_type,p.model_number,
		       p.purchase_date,p.base_sale_price_minor,p.base_sale_currency,p.condition_text,
		       p.inventory_status,p.publication_status,bp.sort_order,b.id,'BOX'||b.box_number
		FROM guest_box_products bp
		JOIN products p ON p.id=bp.product_id AND p.organization_id=bp.organization_id
		JOIN guest_boxes b ON b.id=bp.box_id AND b.organization_id=bp.organization_id
		WHERE bp.organization_id=? AND bp.box_id=? AND p.deleted_at IS NULL
		ORDER BY bp.sort_order,p.product_code`, organizationID, boxID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var products []GuestBoxProduct
	for rows.Next() {
		var product GuestBoxProduct
		if err := rows.Scan(&product.ID, &product.ProductID, &product.ProductCode, &product.Brand,
			&product.Model, &product.ReferenceNumber, &product.PurchaseDate,
			&product.SalePriceMinor, &product.SaleCurrency, &product.Condition,
			&product.InventoryStatus, &product.PublicationStatus, &product.SortOrder,
			&product.AssignedBoxID, &product.AssignedBoxCode); err != nil {
			return nil, err
		}
		products = append(products, product)
	}
	return products, rows.Err()
}

func (s *Store) AddGuestBoxProduct(ctx context.Context, organizationID, boxID, productID, actorID string) error {
	return s.AddGuestBoxProducts(ctx, organizationID, boxID, []string{productID}, actorID)
}

func (s *Store) AddGuestBoxProducts(ctx context.Context, organizationID, boxID string, productIDs []string, actorID string) error {
	if len(productIDs) == 0 {
		return errors.New("追加する商品を選択してください")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var boxExists int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM guest_boxes WHERE organization_id=? AND id=?`,
		organizationID, boxID).Scan(&boxExists); err != nil {
		return err
	}
	if boxExists == 0 {
		return ErrGuestBoxNotFound
	}
	var next int
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(sort_order),0) FROM guest_box_products
		WHERE organization_id=? AND box_id=?`, organizationID, boxID).Scan(&next); err != nil {
		return err
	}
	seen := make(map[string]bool)
	for _, productID := range productIDs {
		productID = strings.TrimSpace(productID)
		if productID == "" || seen[productID] {
			continue
		}
		seen[productID] = true
		var exists int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM products
			WHERE organization_id=? AND id=? AND deleted_at IS NULL`,
			organizationID, productID).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			return errors.New("選択した商品が見つかりません")
		}
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM guest_box_products WHERE organization_id=? AND product_id=?`,
			organizationID, productID); err != nil {
			return err
		}
		next++
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO guest_box_products(organization_id,box_id,product_id,sort_order,added_by,added_at)
			VALUES(?,?,?,?,?,?)`, organizationID, boxID, productID, next, actorID,
			s.now().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	if len(seen) == 0 {
		return errors.New("追加する商品を選択してください")
	}
	return tx.Commit()
}

func (s *Store) GuestBoxProductCandidates(ctx context.Context, organizationID, dateFrom, dateTo, brand, query string) ([]GuestBoxProduct, error) {
	sqlQuery := `
		SELECT p.id,p.id,p.product_code,p.brand,p.product_type,p.model_number,p.purchase_date,
		       p.base_sale_price_minor,p.base_sale_currency,p.condition_text,
		       p.inventory_status,p.publication_status,0,
		       COALESCE(bp.box_id,''),CASE WHEN b.box_number IS NULL THEN '' ELSE 'BOX'||b.box_number END
		FROM products p
		LEFT JOIN guest_box_products bp
		  ON bp.organization_id=p.organization_id AND bp.product_id=p.id
		LEFT JOIN guest_boxes b
		  ON b.organization_id=bp.organization_id AND b.id=bp.box_id
		WHERE p.organization_id=? AND p.deleted_at IS NULL`
	args := []any{organizationID}
	if strings.TrimSpace(dateFrom) != "" {
		sqlQuery += ` AND p.purchase_date>=?`
		args = append(args, strings.TrimSpace(dateFrom))
	}
	if strings.TrimSpace(dateTo) != "" {
		sqlQuery += ` AND p.purchase_date<=?`
		args = append(args, strings.TrimSpace(dateTo))
	}
	if strings.TrimSpace(brand) != "" {
		sqlQuery += ` AND p.brand=?`
		args = append(args, strings.TrimSpace(brand))
	}
	if strings.TrimSpace(query) != "" {
		like := "%" + strings.TrimSpace(query) + "%"
		sqlQuery += ` AND (p.product_code LIKE ? OR p.brand LIKE ? OR p.product_type LIKE ? OR p.model_number LIKE ?)`
		args = append(args, like, like, like, like)
	}
	sqlQuery += ` ORDER BY p.purchase_date DESC,p.product_code DESC LIMIT 200`
	rows, err := s.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var products []GuestBoxProduct
	for rows.Next() {
		var product GuestBoxProduct
		if err := rows.Scan(&product.ID, &product.ProductID, &product.ProductCode, &product.Brand,
			&product.Model, &product.ReferenceNumber, &product.PurchaseDate,
			&product.SalePriceMinor, &product.SaleCurrency, &product.Condition,
			&product.InventoryStatus, &product.PublicationStatus, &product.SortOrder,
			&product.AssignedBoxID, &product.AssignedBoxCode); err != nil {
			return nil, err
		}
		products = append(products, product)
	}
	return products, rows.Err()
}

func (s *Store) RemoveGuestBoxProduct(ctx context.Context, organizationID, boxID, productID string) error {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM guest_box_products WHERE organization_id=? AND box_id=? AND product_id=?`,
		organizationID, boxID, productID)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return errors.New("BOXの商品が見つかりません")
	}
	return nil
}

func (s *Store) MoveGuestBoxProduct(ctx context.Context, organizationID, boxID, productID, targetBoxID, actorID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var valid int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM guest_boxes WHERE organization_id=? AND id IN (?,?)`,
		organizationID, boxID, targetBoxID).Scan(&valid); err != nil {
		return err
	}
	if valid != 2 {
		return ErrGuestBoxNotFound
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM guest_box_products WHERE organization_id=? AND box_id=? AND product_id=?`,
		organizationID, boxID, productID); err != nil {
		return err
	}
	var next int
	_ = tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(sort_order),0)+1 FROM guest_box_products
		WHERE organization_id=? AND box_id=?`, organizationID, targetBoxID).Scan(&next)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO guest_box_products(organization_id,box_id,product_id,sort_order,added_by,added_at)
		VALUES(?,?,?,?,?,?)`,
		organizationID, targetBoxID, productID, next, actorID, s.now().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) GuestPublishedProductIDs(ctx context.Context, organizationID, companyID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT bp.product_id
		FROM guest_box_publications pub
		JOIN guest_box_published_products bp
		  ON bp.organization_id=pub.organization_id AND bp.company_id=pub.company_id
		  AND bp.box_id=pub.box_id
		JOIN guest_companies c ON c.id=pub.company_id AND c.organization_id=pub.organization_id
		WHERE pub.organization_id=? AND pub.company_id=? AND pub.is_published=1 AND c.is_active=1
		GROUP BY bp.product_id
		ORDER BY MIN(bp.product_code)`, organizationID, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) SeedGuestManagementPreview(ctx context.Context) error {
	now := s.now().Format(time.RFC3339Nano)
	for number := 1; number <= 10; number++ {
		name := ""
		switch number {
		case 1:
			name = "ロレックス特集"
		case 2:
			name = "高額品セレクト"
		case 3:
			name = "春の新入荷"
		}
		id := fmt.Sprintf("gbox_preview_%02d", number)
		if _, err := s.db.ExecContext(ctx, `
			INSERT INTO guest_boxes(id,organization_id,box_number,box_name,updated_by,updated_at)
			VALUES(?,'org_preview',?,?, 'usr_admin',?)
			ON CONFLICT(organization_id,box_number) DO UPDATE SET
			  box_name=CASE WHEN guest_boxes.box_name='' THEN excluded.box_name ELSE guest_boxes.box_name END,
			  updated_at=excluded.updated_at`,
			id, number, name, now); err != nil {
			return err
		}
	}
	companies := []struct{ id, code, name string }{
		{"gco_preview_004", "B001", "ウォッチマート"},
		{"gco_preview_002", "B002", "タイムレス商会"},
		{"gco_preview_003", "B003", "ラグジュアリーアイランド"},
		{"gco_preview_001", "B004", "クロノス東京"},
	}
	for _, company := range companies {
		if _, err := s.db.ExecContext(ctx, `
			INSERT INTO guest_companies(
			  id,organization_id,company_code,name,is_active,created_by,created_at,updated_by,updated_at
			) VALUES(?,'org_preview',?,?,1,'usr_admin',?,'usr_admin',?)
			ON CONFLICT(id) DO UPDATE SET name=excluded.name,is_active=1,updated_at=excluded.updated_at`,
			company.id, company.code, company.name, now, now); err != nil {
			return err
		}
	}
	guestHash, err := HashPassword("guest-preview-2026")
	if err != nil {
		return err
	}
	for index, company := range companies {
		if _, err := s.db.ExecContext(ctx, `
			INSERT INTO guest_credentials(
			  organization_id,company_id,guest_id,email,password_hash,updated_by,updated_at
			) VALUES('org_preview',?,?,?,?,'usr_admin',?)
			ON CONFLICT(organization_id,company_id) DO UPDATE SET
			  guest_id=excluded.guest_id,email=excluded.email,updated_at=excluded.updated_at`,
			company.id, company.code, fmt.Sprintf("guest%02d@example.jp", index+1),
			guestHash, now); err != nil {
			return err
		}
	}
	// Existing product BOX assignments become the starting line-up. This is idempotent.
	if _, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO guest_box_products(
		  organization_id,box_id,product_id,sort_order,added_by,added_at
		)
		SELECT p.organization_id,b.id,p.id,
		       ROW_NUMBER() OVER(PARTITION BY b.id ORDER BY p.purchase_date DESC,p.product_code),
		       'usr_admin',?
		FROM products p JOIN guest_boxes b
		  ON b.organization_id=p.organization_id AND ('BOX'||b.box_number)=p.box_text
		WHERE p.organization_id='org_preview' AND p.deleted_at IS NULL`, now); err != nil {
		return err
	}
	return nil
}
