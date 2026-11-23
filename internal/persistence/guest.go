package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrGuestAccountNotFound = errors.New("guest account not found")
	ErrBoxProductStatus     = errors.New("BOXに登録できる商品は仕入中・在庫中・出荷済のみです")
	ErrBoxNotFound          = errors.New("publication box not found")
	ErrProductUnavailable   = errors.New("product is not available to this guest")
	ErrPurchaseRequestState = errors.New("purchase request cannot be changed in its current state")
)

type PublicationBoxProductVisibility struct {
	ProductCode      string `json:"productCode"`
	BasisStatus      string `json:"basisStatus"`
	VisibilityState  string `json:"visibilityState"`
	VisibilityReason string `json:"visibilityReason"`
	GuestVisible     bool   `json:"guestVisible"`
}

type PublicationBoxRecord struct {
	ProductVisibility []PublicationBoxProductVisibility `gorm:"-" json:"productVisibility"`
	ID                string                            `json:"id"`
	BoxCode           string                            `json:"boxCode"`
	Name              string                            `json:"name"`
	IsActive          bool                              `json:"isActive"`
	BuyerCodes        []string                          `gorm:"-" json:"buyerCodes"`
	ProductCodes      []string                          `gorm:"-" json:"productCodes"`
	UpdatedAt         time.Time                         `json:"updatedAt"`
}

type PublicationBoxInput struct {
	Name         string   `json:"name"`
	IsActive     bool     `json:"isActive"`
	BuyerCodes   []string `json:"buyerCodes"`
	ProductCodes []string `json:"productCodes"`
}

type GuestCatalogItem struct {
	DisplayPriceMinor  *int64     `gorm:"-" json:"displayPriceMinor"`
	DisplayCurrency    string     `gorm:"-" json:"displayCurrency"`
	Images             []string   `gorm:"-" json:"images"`
	ProductID          string     `json:"productId"`
	ProductCode        string     `json:"productCode"`
	Brand              string     `json:"brand"`
	ModelNumber        string     `json:"modelNumber"`
	ReferenceNumber    string     `json:"referenceNumber"`
	SerialNumber       string     `json:"serialNumber"`
	Condition          string     `json:"condition"`
	Accessories        string     `json:"accessories"`
	BaseSalePriceMinor int64      `json:"baseSalePriceMinor"`
	BaseSaleCurrency   string     `json:"baseSaleCurrency"`
	InventoryStatus    string     `json:"inventoryStatus"`
	PurchaseDate       DateString `json:"purchaseDate"`
	BoxCodes           string     `json:"-"`
	ReservedByMe       bool       `json:"reservedByMe"`
}

type PurchaseRequestRecord struct {
	BatchID              string                  `gorm:"-" json:"batchId,omitempty"`
	ShipmentSlipID       string                  `gorm:"-" json:"shipmentSlipId,omitempty"`
	ShipmentSlipNumber   string                  `gorm:"-" json:"shipmentSlipNumber,omitempty"`
	Items                []PurchaseRequestRecord `gorm:"-" json:"items,omitempty"`
	ReferenceNumber      string                  `json:"referenceNumber"`
	RequestedPriceMinor  *int64                  `json:"requestedPriceMinor"`
	RequestedCurrency    string                  `json:"requestedCurrency"`
	ID                   string                  `json:"id"`
	RequestNumber        string                  `json:"requestNumber"`
	GuestCode            string                  `json:"guestCode"`
	BuyerCode            string                  `json:"buyerCode"`
	BuyerName            string                  `json:"buyerName"`
	ProductID            string                  `json:"productId"`
	ProductCode          string                  `json:"productCode"`
	Brand                string                  `json:"brand"`
	ModelNumber          string                  `json:"modelNumber"`
	Status               string                  `json:"status"`
	Message              string                  `json:"message"`
	RequestedAt          time.Time               `json:"requestedAt"`
	ReviewNote           string                  `json:"reviewNote"`
	ReservationExpiresAt *time.Time              `json:"reservationExpiresAt,omitempty"`
	UpdatedAt            time.Time               `json:"updatedAt"`
}

type NotificationRecord struct {
	ID         string     `json:"id"`
	EventKey   string     `json:"eventKey"`
	Title      string     `json:"title"`
	Body       string     `json:"body"`
	TargetType string     `json:"targetType"`
	TargetID   string     `json:"targetId"`
	ReadAt     *time.Time `json:"readAt,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
}

type guestIdentity struct {
	AccountID, GuestCode, BuyerRoleID, BuyerCode, BuyerName, UserID, RegionType string
}

func lookupGuestIdentity(tx *gorm.DB, organizationID, userID string) (guestIdentity, error) {
	var guest guestIdentity
	result := tx.Table("guest_accounts AS ga").
		Select(`ga.id AS account_id,ga.guest_code,ga.buyer_role_id,pr.role_code AS buyer_code,
			bp.legal_name AS buyer_name,ga.user_id,bp.region_type`).
		Joins("JOIN partner_roles pr ON pr.id=ga.buyer_role_id AND pr.organization_id=ga.organization_id").
		Joins("JOIN business_partners bp ON bp.id=pr.partner_id AND bp.organization_id=ga.organization_id").
		Where("ga.organization_id=? AND ga.user_id=? AND ga.status='active'", organizationID, userID).Take(&guest)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return guestIdentity{}, ErrGuestAccountNotFound
	}
	return guest, result.Error
}

// GuestRegionType reads only the authenticated guest's linked customer region.
func (r *Repository) GuestRegionType(ctx context.Context, organizationID, userID string) (string, error) {
	guest, err := lookupGuestIdentity(r.db.WithContext(ctx), organizationID, userID)
	if err != nil {
		return "", err
	}
	if guest.RegionType == "overseas" {
		return "overseas", nil
	}
	return "domestic", nil
}

func (r *Repository) PublicationBoxes(ctx context.Context, organizationID string) ([]PublicationBoxRecord, error) {
	var boxes []PublicationBoxRecord
	if err := r.db.WithContext(ctx).Table("publication_boxes").
		Select("id,box_code,name,is_active,updated_at").Where("organization_id=?", organizationID).
		Order("box_code").Scan(&boxes).Error; err != nil {
		return nil, err
	}
	for index := range boxes {
		if err := r.db.WithContext(ctx).Table("publication_box_buyers AS x").
			Select("pr.role_code").Joins("JOIN partner_roles pr ON pr.id=x.buyer_role_id").
			Where("x.box_id=?", boxes[index].ID).Order("pr.role_code").Pluck("pr.role_code", &boxes[index].BuyerCodes).Error; err != nil {
			return nil, err
		}
		if err := r.db.WithContext(ctx).Table("publication_box_products AS x").
			Select("p.product_code").Joins("JOIN products p ON p.id=x.product_id").
			Where("x.box_id=?", boxes[index].ID).Order("p.product_code").Pluck("p.product_code", &boxes[index].ProductCodes).Error; err != nil {
			return nil, err
		}

		if err := r.db.WithContext(ctx).Table("publication_box_products AS x").
			Select("p.product_code,x.basis_status,x.visibility_state,x.visibility_reason,(x.visibility_state='visible' AND p.inventory_status IN ('in_stock','shipped') AND p.deleted_at IS NULL) AS guest_visible").
			Joins("JOIN products p ON p.id=x.product_id").Where("x.box_id=?", boxes[index].ID).Order("p.product_code").Scan(&boxes[index].ProductVisibility).Error; err != nil {
			return nil, err
		}
	}
	return boxes, nil
}

func (r *Repository) UpdatePublicationBox(ctx context.Context, organizationID, boxCode, actorUserID string, input PublicationBoxInput) (PublicationBoxRecord, error) {
	boxCode = strings.ToUpper(strings.TrimSpace(boxCode))
	// Fixed UI slots are created on first save, including clean installations.
	boxNumber, parseErr := strconv.Atoi(strings.TrimPrefix(boxCode, "BOX-"))
	if parseErr != nil || boxNumber < 1 || boxNumber > 10 || boxCode != fmt.Sprintf("BOX-%02d", boxNumber) {
		return PublicationBoxRecord{}, ErrBoxNotFound
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		newBoxID, err := database.NewID("box")
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		if err := tx.Exec(`INSERT INTO publication_boxes
			(id,organization_id,box_code,name,is_active,created_by,updated_by,created_at,updated_at)
			VALUES(?,?,?,?,TRUE,?,?,?,?) ON CONFLICT (organization_id,box_code) DO NOTHING`,
			newBoxID, organizationID, boxCode, boxCode, actorUserID, actorUserID, now, now).Error; err != nil {
			return err
		}
		var boxID string
		result := tx.Raw(`SELECT id FROM publication_boxes WHERE organization_id=? AND box_code=? FOR UPDATE`, organizationID, boxCode).Scan(&boxID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrBoxNotFound
		}
		name := strings.TrimSpace(input.Name)
		if name == "" {
			name = boxCode
		}
		if err := tx.Exec(`UPDATE publication_boxes SET name=?,is_active=?,updated_by=?,updated_at=?
			WHERE organization_id=? AND id=?`, name, input.IsActive, actorUserID, now, organizationID, boxID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`DELETE FROM publication_box_buyers WHERE box_id=?`, boxID).Error; err != nil {
			return err
		}
		for _, buyerCode := range normalizeCodes(input.BuyerCodes) {
			var buyerRoleID string
			result := tx.Table("partner_roles").Select("id").Where(
				"organization_id=? AND role_type='buyer' AND role_code=? AND is_active", organizationID, buyerCode).Scan(&buyerRoleID)
			if result.Error != nil || buyerRoleID == "" {
				return ErrMasterCodeNotFound
			}
			if err := tx.Exec(`INSERT INTO publication_box_buyers(box_id,buyer_role_id,created_at) VALUES(?,?,?)`, boxID, buyerRoleID, now).Error; err != nil {
				return err
			}
		}
		// Existing assignments may remain after a workflow changes the product status.
		// Validate new assignments, while still allowing BOX metadata edits and removals.
		var existingProductIDs []string
		if err := tx.Table("publication_box_products").Where("box_id=?", boxID).Pluck("product_id", &existingProductIDs).Error; err != nil {
			return err
		}
		existing := make(map[string]bool, len(existingProductIDs))
		for _, id := range existingProductIDs {
			existing[id] = true
		}
		keptProductIDs := []string{}
		for _, productCode := range normalizeCodes(input.ProductCodes) {
			var product struct {
				ID              string
				InventoryStatus string
			}
			result := tx.Table("products").Clauses(clause.Locking{Strength: "UPDATE"}).Select("id,inventory_status").Where(
				"organization_id=? AND product_code=? AND deleted_at IS NULL", organizationID, productCode).Scan(&product)
			if result.Error != nil {
				return result.Error
			}
			if product.ID == "" {
				return ErrMasterCodeNotFound
			}
			if !existing[product.ID] && product.InventoryStatus != "in_stock" && product.InventoryStatus != "purchasing" && product.InventoryStatus != "shipped" {
				return fmt.Errorf("%w（管理番号: %s）", ErrBoxProductStatus, productCode)
			}
			productID := product.ID
			keptProductIDs = append(keptProductIDs, productID)
			// Metadata saves must not reset a product's hidden state or original assignment time.
			if !existing[productID] {
				if err := tx.Exec(`INSERT INTO publication_box_products(box_id,product_id,created_at) VALUES(?,?,?)`, boxID, productID, now).Error; err != nil {
					return err
				}
			}
		}
		removals := tx.Table("publication_box_products").Where("box_id=?", boxID)
		if len(keptProductIDs) > 0 {
			removals = removals.Where("product_id NOT IN ?", keptProductIDs)
		}
		if err := removals.Delete(nil).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE products p SET publication_status='public',updated_at=?
			WHERE p.organization_id=? AND EXISTS(
				SELECT 1 FROM publication_box_products x JOIN publication_boxes b ON b.id=x.box_id
				WHERE x.product_id=p.id AND b.organization_id=p.organization_id AND b.is_active
			)`, now, organizationID).Error; err != nil {
			return err
		}
		return tx.Exec(`UPDATE products p SET publication_status='private',updated_at=?
			WHERE p.organization_id=? AND NOT EXISTS(
				SELECT 1 FROM publication_box_products x JOIN publication_boxes b ON b.id=x.box_id
				WHERE x.product_id=p.id AND b.organization_id=p.organization_id AND b.is_active
			)`, now, organizationID).Error
	})
	if err != nil {
		return PublicationBoxRecord{}, err
	}
	boxes, err := r.PublicationBoxes(ctx, organizationID)
	if err != nil {
		return PublicationBoxRecord{}, err
	}
	for _, box := range boxes {
		if box.BoxCode == boxCode {
			return box, nil
		}
	}
	return PublicationBoxRecord{}, ErrBoxNotFound
}

func (r *Repository) GuestCatalog(ctx context.Context, organizationID, userID string) ([]GuestCatalogItem, error) {
	if err := r.ExpireReservations(ctx, organizationID); err != nil {
		return nil, err
	}
	guest, err := lookupGuestIdentity(r.db.WithContext(ctx), organizationID, userID)
	if err != nil {
		return nil, err
	}
	var items []GuestCatalogItem
	err = guestVisibleProducts(r.db.WithContext(ctx), organizationID, guest.BuyerRoleID).
		Select(`p.id AS product_id,p.product_code,p.brand,p.model_number,p.reference_number,p.serial_number,
			p.condition_text AS condition,p.accessories,p.base_sale_price_minor,p.base_sale_currency,
			p.inventory_status,p.purchase_date,STRING_AGG(DISTINCT b.box_code,',' ORDER BY b.box_code) AS box_codes,
			EXISTS(SELECT 1 FROM purchase_requests prq WHERE prq.organization_id=p.organization_id
				AND prq.product_id=p.id AND prq.buyer_role_id=? AND prq.status='approved') AS reserved_by_me`, guest.BuyerRoleID).
		Group("p.id").Order("p.purchase_date DESC,p.product_code DESC").Scan(&items).Error
	if err != nil || len(items) == 0 {
		return items, err
	}
	// Display the expected price before tax, using the current configured rate.
	// Keep the original amount/currency intact for purchase-request snapshots.
	currency := "JPY"
	if guest.RegionType == "overseas" {
		currency = "USD"
	}
	rate, rateErr := latestFX(r.db.WithContext(ctx), organizationID, "USD")
	for i := range items {
		item := &items[i]
		item.DisplayCurrency = currency
		if item.BaseSaleCurrency != currency && rateErr != nil {
			continue
		}
		amount, err := convertCurrency(item.BaseSalePriceMinor, item.BaseSaleCurrency, currency, rate)
		if err != nil {
			return nil, err
		}
		if currency == "JPY" {
			if item.BaseSaleCurrency == "USD" {
				amount = convertShipmentUSDToJPYRoundUpToThousand(item.BaseSalePriceMinor, rate.RateScaled, rate.Scale)
			} else {
				amount = convertShipmentUSDToJPYRoundUpToThousand(item.BaseSalePriceMinor, 1, 1)
			}
		}
		item.DisplayPriceMinor = &amount
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ProductID)
	}
	var files []struct{ ID, ProductID, SHA256 string }
	if err := r.db.WithContext(ctx).Table("product_files").Select("id,product_id,sha256").Where("organization_id=? AND product_id IN ? AND content_type IN ?", organizationID, ids, []string{"image/jpeg", "image/png", "image/webp"}).Order("product_id,sort_order,id").Scan(&files).Error; err != nil {
		return nil, err
	}
	images := make(map[string][]string)
	for _, file := range files {
		images[file.ProductID] = append(images[file.ProductID], "/api/v1/guest/product-files/"+url.PathEscape(file.ID)+"?v="+url.QueryEscape(file.SHA256))
	}
	for i := range items {
		items[i].Images = images[items[i].ProductID]
		if items[i].Images == nil {
			items[i].Images = []string{}
		}
	}
	return items, nil
}

// Catalog and image access must use exactly the same current publication scope.
func guestVisibleProducts(tx *gorm.DB, organizationID, buyerRoleID string) *gorm.DB {
	return tx.Table("products AS p").
		Joins("JOIN publication_box_products x ON x.product_id=p.id").
		Joins("JOIN publication_boxes b ON b.id=x.box_id AND b.organization_id=p.organization_id AND b.is_active").
		Joins("JOIN publication_box_buyers xb ON xb.box_id=b.id AND xb.buyer_role_id=?", buyerRoleID).
		Where("p.organization_id=? AND p.deleted_at IS NULL AND p.publication_status='public' AND p.inventory_status IN ('in_stock','shipped') AND x.visibility_state='visible'", organizationID)
}

func (r *Repository) GuestProductFile(ctx context.Context, organizationID, userID, fileID string) (ProductFileRecord, error) {
	guest, err := lookupGuestIdentity(r.db.WithContext(ctx), organizationID, userID)
	if err != nil {
		return ProductFileRecord{}, err
	}
	var record ProductFileRecord
	err = guestVisibleProducts(r.db.WithContext(ctx), organizationID, guest.BuyerRoleID).
		Joins("JOIN product_files f ON f.product_id=p.id AND f.organization_id=p.organization_id").
		Select("f.id,f.product_id,f.object_key,f.content_type,f.size_bytes").
		Where("f.id=? AND f.content_type IN ?", fileID, []string{"image/jpeg", "image/png", "image/webp"}).Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return record, ErrProductUnavailable
	}
	return record, err
}

func (r *Repository) CreateGuestPurchaseRequest(ctx context.Context, organizationID, userID, productID, message string) (PurchaseRequestRecord, error) {
	var requestID string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		guest, err := lookupGuestIdentity(tx, organizationID, userID)
		if err != nil {
			return err
		}
		var product struct {
			ID, ProductCode, Status             string
			Brand, ModelNumber, ReferenceNumber string
			BaseSalePriceMinor                  int64
			BaseSaleCurrency                    string
		}
		result := tx.Raw(`SELECT p.id,p.product_code,p.inventory_status AS status,p.brand,p.model_number,p.reference_number,p.base_sale_price_minor,p.base_sale_currency FROM products p
			WHERE p.organization_id=? AND p.id=? AND p.deleted_at IS NULL AND p.publication_status='public'
			AND p.inventory_status='in_stock' AND EXISTS(
				SELECT 1 FROM publication_box_products x
				JOIN publication_boxes b ON b.id=x.box_id AND b.organization_id=p.organization_id AND b.is_active
				JOIN publication_box_buyers xb ON xb.box_id=b.id AND xb.buyer_role_id=?
				WHERE x.product_id=p.id AND x.visibility_state='visible'
			) FOR UPDATE`, organizationID, productID, guest.BuyerRoleID).Scan(&product)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrProductUnavailable
		}
		var existing int64
		if err := tx.Table("purchase_requests").Where(
			"organization_id=? AND guest_account_id=? AND product_id=? AND status IN ('pending','approved')",
			organizationID, guest.AccountID, productID).Count(&existing).Error; err != nil {
			return err
		}
		if existing > 0 {
			return ErrPurchaseRequestState
		}
		now := time.Now().UTC()
		sequence, err := nextPurchaseRequestSequence(tx, organizationID, now.Year(), now)
		if err != nil {
			return err
		}
		requestID, err = database.NewID("req")
		if err != nil {
			return err
		}
		requestNumber := fmt.Sprintf("RQ-%04d-%04d", now.Year(), sequence)
		snapshot, err := json.Marshal(map[string]any{"brand": product.Brand, "model": product.ModelNumber, "reference": product.ReferenceNumber, "priceMinor": product.BaseSalePriceMinor, "currency": product.BaseSaleCurrency})
		if err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO purchase_requests(
			id,organization_id,request_number,guest_account_id,buyer_role_id,product_id,status,message,
			requested_at,created_at,updated_at,product_snapshot
		) VALUES(?,?,?,?,?,?,'pending',?,?,?,?,?::jsonb)`, requestID, organizationID, requestNumber, guest.AccountID,
			guest.BuyerRoleID, productID, strings.TrimSpace(message), now, now, now, string(snapshot)).Error; err != nil {
			return err
		}
		return insertNotificationTx(tx, organizationID, "", database.RoleAdmin, "purchase_request.created",
			"購入リクエストが届きました", guest.BuyerName+" / "+product.ProductCode, "purchase_request", requestID, now)
	})
	if err != nil {
		return PurchaseRequestRecord{}, err
	}
	return r.PurchaseRequest(ctx, organizationID, requestID)
}

func nextPurchaseRequestSequence(tx *gorm.DB, organizationID string, year int, now time.Time) (int, error) {
	var sequence int
	err := tx.Raw(`INSERT INTO purchase_request_sequences(organization_id,business_year,last_sequence,updated_at)
		SELECT ?,?,COALESCE(MAX((regexp_match(request_number,'([0-9]+)$'))[1]::INTEGER),0)+1,?
		FROM purchase_requests WHERE organization_id=? AND EXTRACT(YEAR FROM requested_at)=?
		ON CONFLICT (organization_id,business_year) DO UPDATE
		SET last_sequence=purchase_request_sequences.last_sequence+1,updated_at=EXCLUDED.updated_at
		RETURNING last_sequence`, organizationID, year, now, organizationID, year).Scan(&sequence).Error
	return sequence, err
}

func (r *Repository) PurchaseRequests(ctx context.Context, organizationID, userID, role, status string) ([]PurchaseRequestRecord, error) {
	if err := r.ExpireReservations(ctx, organizationID); err != nil {
		return nil, err
	}
	query := r.db.WithContext(ctx).Table("purchase_requests AS q").
		Select(`q.id,q.request_number,ga.guest_code,br.role_code AS buyer_code,bp.legal_name AS buyer_name,
			q.product_id,p.product_code,COALESCE(q.product_snapshot->>'brand',p.brand) AS brand,
 COALESCE(q.product_snapshot->>'model',p.model_number) AS model_number,
 COALESCE(q.product_snapshot->>'reference',p.reference_number) AS reference_number,
 (q.product_snapshot->>'priceMinor')::BIGINT AS requested_price_minor,
 COALESCE(q.product_snapshot->>'currency','') AS requested_currency,
 q.status,q.message,q.requested_at,q.review_note,
			q.reservation_expires_at,q.updated_at`).
		Joins("JOIN guest_accounts ga ON ga.id=q.guest_account_id").
		Joins("JOIN partner_roles br ON br.id=q.buyer_role_id").
		Joins("JOIN business_partners bp ON bp.id=br.partner_id").
		Joins("JOIN products p ON p.id=q.product_id").Where("q.organization_id=?", organizationID)
	if role == database.RoleGuest {
		query = query.Where("ga.user_id=?", userID)
	}
	var records []PurchaseRequestRecord
	err := query.Order("q.requested_at DESC,q.request_number DESC").Scan(&records).Error
	if err != nil {
		return nil, err
	}
	records, err = r.groupPurchaseRequests(ctx, organizationID, records)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(status) != "" {
		filtered := []PurchaseRequestRecord{}
		for _, record := range records {
			if record.Status == strings.TrimSpace(status) {
				filtered = append(filtered, record)
			}
		}
		records = filtered
	}
	return records, nil
}

func (r *Repository) PurchaseRequest(ctx context.Context, organizationID, requestID string) (PurchaseRequestRecord, error) {
	var record PurchaseRequestRecord
	result := r.db.WithContext(ctx).Table("purchase_requests AS q").
		Select(`q.id,q.request_number,ga.guest_code,br.role_code AS buyer_code,bp.legal_name AS buyer_name,
			q.product_id,p.product_code,COALESCE(q.product_snapshot->>'brand',p.brand) AS brand,
 COALESCE(q.product_snapshot->>'model',p.model_number) AS model_number,
 COALESCE(q.product_snapshot->>'reference',p.reference_number) AS reference_number,
 (q.product_snapshot->>'priceMinor')::BIGINT AS requested_price_minor,
 COALESCE(q.product_snapshot->>'currency','') AS requested_currency,
 q.status,q.message,q.requested_at,q.review_note,
			q.reservation_expires_at,q.updated_at`).
		Joins("JOIN guest_accounts ga ON ga.id=q.guest_account_id").Joins("JOIN partner_roles br ON br.id=q.buyer_role_id").
		Joins("JOIN business_partners bp ON bp.id=br.partner_id").Joins("JOIN products p ON p.id=q.product_id").
		Where("q.organization_id=? AND q.id=?", organizationID, requestID).Take(&record)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return PurchaseRequestRecord{}, ErrPurchaseRequestState
	}
	return record, result.Error
}

func (r *Repository) ReviewPurchaseRequest(ctx context.Context, organizationID, requestID, actorUserID, decision, note string) (PurchaseRequestRecord, error) {
	if r.db.Migrator().HasTable("purchase_request_batches") {
		var grouped int64
		if err := r.db.WithContext(ctx).Table("purchase_requests").Where("organization_id=? AND id=? AND batch_id IS NOT NULL", organizationID, requestID).Count(&grouped).Error; err != nil {
			return PurchaseRequestRecord{}, err
		}
		if grouped > 0 {
			return PurchaseRequestRecord{}, ErrPurchaseRequestState
		}
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var request struct{ Status, ProductID, GuestAccountID, BuyerRoleID string }
		result := tx.Raw(`SELECT status,product_id,guest_account_id,buyer_role_id FROM purchase_requests
			WHERE organization_id=? AND id=? FOR UPDATE`, organizationID, requestID).Scan(&request)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 || request.Status != "pending" {
			return ErrPurchaseRequestState
		}
		var productStatus string
		if err := tx.Raw(`SELECT inventory_status FROM products WHERE organization_id=? AND id=? FOR UPDATE`, organizationID, request.ProductID).Scan(&productStatus).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		var guestUserID string
		if err := tx.Table("guest_accounts").Select("user_id").Where("id=?", request.GuestAccountID).Scan(&guestUserID).Error; err != nil {
			return err
		}
		switch decision {
		case "approved":
			if productStatus != "in_stock" {
				return ErrProductUnavailable
			}
			duration := 48 * time.Hour
			var configured string
			_ = tx.Table("organization_settings").Select("setting_value").Where(
				"organization_id=? AND setting_key='reservation.duration_hours' AND is_configured", organizationID).Scan(&configured).Error
			if parsed, err := time.ParseDuration(strings.TrimSpace(configured) + "h"); err == nil && parsed >= time.Hour && parsed <= 30*24*time.Hour {
				duration = parsed
			}
			expires := now.Add(duration)
			if err := tx.Exec(`UPDATE purchase_requests SET status='approved',reviewed_by=?,reviewed_at=?,review_note=?,
				reservation_expires_at=?,updated_at=? WHERE organization_id=? AND id=?`, actorUserID, now,
				strings.TrimSpace(note), expires, now, organizationID, requestID).Error; err != nil {
				return err
			}
			if err := tx.Exec(`UPDATE products SET inventory_status='reserved',updated_at=? WHERE organization_id=? AND id=?`,
				now, organizationID, request.ProductID).Error; err != nil {
				return err
			}
			eventID, _ := database.NewID("ive")
			if err := tx.Exec(`INSERT INTO inventory_events(
				id,organization_id,product_id,event_type,from_status,to_status,reason,actor_user_id,created_at
			) VALUES(?,?,?,'purchase_request_approved','in_stock','reserved',?,?,?)`, eventID, organizationID,
				request.ProductID, "購入リクエスト承認", actorUserID, now).Error; err != nil {
				return err
			}
			if err := tx.Exec(`UPDATE purchase_requests SET status='rejected',reviewed_by=?,reviewed_at=?,
				review_note='他の購入リクエストが承認されました',updated_at=?
				WHERE organization_id=? AND product_id=? AND id<>? AND status='pending'`, actorUserID, now, now,
				organizationID, request.ProductID, requestID).Error; err != nil {
				return err
			}
			return insertNotificationTx(tx, organizationID, guestUserID, "", "purchase_request.approved",
				"購入リクエストが承認されました", "商品を取置中です。", "purchase_request", requestID, now)
		case "rejected":
			if err := tx.Exec(`UPDATE purchase_requests SET status='rejected',reviewed_by=?,reviewed_at=?,review_note=?,updated_at=?
				WHERE organization_id=? AND id=?`, actorUserID, now, strings.TrimSpace(note), now, organizationID, requestID).Error; err != nil {
				return err
			}
			return insertNotificationTx(tx, organizationID, guestUserID, "", "purchase_request.rejected",
				"購入リクエストが見送られました", strings.TrimSpace(note), "purchase_request", requestID, now)
		default:
			return ErrPurchaseRequestState
		}
	})
	if err != nil {
		return PurchaseRequestRecord{}, err
	}
	return r.PurchaseRequest(ctx, organizationID, requestID)
}

func (r *Repository) ExpireReservations(ctx context.Context, organizationID string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var requests []struct{ ID, ProductID, GuestAccountID string }
		if err := tx.Raw(`SELECT id,product_id,guest_account_id FROM purchase_requests
			WHERE organization_id=? AND status='approved' AND reservation_expires_at<=? FOR UPDATE`,
			organizationID, time.Now().UTC()).Scan(&requests).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		for _, request := range requests {
			if err := tx.Exec(`UPDATE purchase_requests SET status='expired',updated_at=? WHERE id=?`, now, request.ID).Error; err != nil {
				return err
			}
			if err := tx.Exec(`UPDATE products SET inventory_status='in_stock',updated_at=?
				WHERE organization_id=? AND id=? AND inventory_status='reserved'`, now, organizationID, request.ProductID).Error; err != nil {
				return err
			}
			var guestUserID string
			_ = tx.Table("guest_accounts").Select("user_id").Where("id=?", request.GuestAccountID).Scan(&guestUserID).Error
			if err := insertNotificationTx(tx, organizationID, guestUserID, "", "purchase_request.expired",
				"商品の取置期限が終了しました", "必要な場合は再度ご依頼ください。", "purchase_request", request.ID, now); err != nil {
				return err
			}
		}
		return nil
	})
}

func insertNotificationTx(tx *gorm.DB, organizationID, recipientUserID, recipientRole, eventKey, title, body, targetType, targetID string, now time.Time) error {
	id, err := database.NewID("ntf")
	if err != nil {
		return err
	}
	return tx.Exec(`INSERT INTO notifications(
		id,organization_id,recipient_user_id,recipient_role_key,event_key,title,body,target_type,target_id,created_at
	) VALUES(?,?,?,?,?,?,?,?,?,?)`, id, organizationID, nullIfEmpty(recipientUserID), nullIfEmpty(recipientRole),
		eventKey, title, body, targetType, targetID, now).Error
}

func (r *Repository) Notifications(ctx context.Context, organizationID, userID, role string, limit int, unreadOnly ...bool) ([]NotificationRecord, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	var records []NotificationRecord
	query := r.db.WithContext(ctx).Table("notifications").
		Select("id,event_key,title,body,target_type,target_id,read_at,created_at").
		Where("organization_id=? AND (recipient_user_id=? OR recipient_role_key=?)", organizationID, userID, role).
		Order("created_at DESC").Limit(limit)
	if len(unreadOnly) > 0 && unreadOnly[0] {
		query = query.Where("read_at IS NULL")
	}
	err := query.Scan(&records).Error
	return records, err
}

func (r *Repository) ReadNotification(ctx context.Context, organizationID, userID, role, notificationID string) error {
	result := r.db.WithContext(ctx).Exec(`UPDATE notifications SET read_at=COALESCE(read_at,?)
		WHERE organization_id=? AND id=? AND (recipient_user_id=? OR recipient_role_key=?)`,
		time.Now().UTC(), organizationID, notificationID, userID, role)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
