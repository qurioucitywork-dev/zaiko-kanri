package persistence

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

var ErrStocktakeNotFound = errors.New("stocktake session not found")

type StocktakeSession struct {
	ID             string          `gorm:"column:id;primaryKey" json:"id"`
	OrganizationID string          `gorm:"column:organization_id" json:"-"`
	Status         string          `gorm:"column:status" json:"status"`
	StartedBy      string          `gorm:"column:started_by" json:"startedBy"`
	StartedAt      time.Time       `gorm:"column:started_at" json:"startedAt"`
	SavedAt        time.Time       `gorm:"column:saved_at" json:"savedAt"`
	CompletedBy    string          `gorm:"column:completed_by" json:"completedBy,omitempty"`
	CompletedAt    *time.Time      `gorm:"column:completed_at" json:"completedAt,omitempty"`
	InventoryDate  *DateString     `gorm:"column:inventory_date;type:date" json:"inventoryDate,omitempty"`
	CreatedAt      time.Time       `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt      time.Time       `gorm:"column:updated_at" json:"updatedAt"`
	Lines          []StocktakeLine `gorm:"foreignKey:SessionID" json:"lines"`
}

func (StocktakeSession) TableName() string { return "stocktake_sessions" }

type StocktakeLine struct {
	ID                  string     `gorm:"column:id;primaryKey" json:"id"`
	OrganizationID      string     `gorm:"column:organization_id" json:"-"`
	SessionID           string     `gorm:"column:session_id" json:"sessionId"`
	ProductID           string     `gorm:"column:product_id" json:"productId,omitempty"`
	ProductCode         string     `gorm:"column:product_code" json:"productCode"`
	LineType            string     `gorm:"column:line_type" json:"lineType"`
	ResultStatus        string     `gorm:"column:result_status" json:"resultStatus"`
	Source              string     `gorm:"column:source" json:"source"`
	InventoryStatus     string     `gorm:"column:inventory_status" json:"inventoryStatus"`
	Brand               string     `gorm:"column:brand" json:"brand"`
	ModelNumber         string     `gorm:"column:model_number" json:"modelNumber"`
	ReferenceNumber     string     `gorm:"column:reference_number" json:"referenceNumber"`
	SerialNumber        string     `gorm:"column:serial_number" json:"serialNumber"`
	PurchasePriceMinor  int64      `gorm:"column:purchase_price_minor" json:"purchasePriceMinor"`
	Reason              string     `gorm:"column:reason" json:"reason"`
	Note                string     `gorm:"column:note" json:"note"`
	CheckedAt           *time.Time `gorm:"column:checked_at" json:"checkedAt,omitempty"`
	ShipmentIssuedAt    *time.Time `gorm:"column:shipment_issued_at" json:"shipmentIssuedAt,omitempty"`
	DocumentType        string     `gorm:"column:document_type" json:"documentType,omitempty"`
	DocumentID          string     `gorm:"column:document_id" json:"documentId,omitempty"`
	DocumentNumber      string     `gorm:"column:document_number" json:"documentNumber,omitempty"`
	DocumentPartnerName string     `gorm:"column:document_partner_name" json:"documentPartnerName,omitempty"`
	DocumentCheckedAt   *time.Time `gorm:"column:document_checked_at" json:"documentCheckedAt,omitempty"`
	DocumentCheckedBy   string     `gorm:"column:document_checked_by" json:"documentCheckedBy,omitempty"`
	ResolvedAt          *time.Time `gorm:"column:resolved_at" json:"resolvedAt,omitempty"`
	CreatedAt           time.Time  `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt           time.Time  `gorm:"column:updated_at" json:"updatedAt"`
}

func (StocktakeLine) TableName() string { return "stocktake_lines" }

var stocktakeTargetStatuses = []string{"in_stock", "reserved", "shipped", "consigned", "return_pending"}

func stocktakeID(prefix string) string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("%s%d", prefix, time.Now().UnixNano())
	}
	return prefix + hex.EncodeToString(buffer)
}

func (r *Repository) CurrentStocktake(ctx context.Context, organizationID string) (StocktakeSession, error) {
	var session StocktakeSession
	err := r.db.WithContext(ctx).Where("organization_id = ? AND status = ?", organizationID, "in_progress").
		Order("started_at DESC").First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return session, ErrStocktakeNotFound
	}
	if err != nil {
		return session, err
	}
	if err := attachStocktakeDocuments(r.db.WithContext(ctx), organizationID, session.ID); err != nil {
		return session, err
	}
	if err := r.db.WithContext(ctx).Where("session_id = ?", session.ID).
		Order("CASE WHEN line_type = 'expected_missing' THEN 0 ELSE 1 END, created_at, product_code").Find(&session.Lines).Error; err != nil {
		return session, err
	}
	return session, nil
}

func attachStocktakeDocuments(tx *gorm.DB, organizationID, sessionID string) error {
	if tx.Migrator().HasTable("shipment_lines") {
		if err := tx.Exec(`UPDATE stocktake_lines st SET
			document_type='shipment', document_id=x.id, document_number=x.slip_number,
			document_partner_name=x.partner_name, shipment_issued_at=x.issued_at
		FROM (SELECT DISTINCT ON (sl.product_id) sl.product_id,ss.id,ss.slip_number,
			bp.legal_name AS partner_name,COALESCE(ss.confirmed_at,ss.created_at) AS issued_at
			FROM shipment_lines sl JOIN shipment_slips ss ON ss.id=sl.shipment_slip_id
			JOIN partner_roles pr ON pr.id=ss.buyer_role_id JOIN business_partners bp ON bp.id=pr.partner_id
			WHERE ss.organization_id=? AND ss.status='confirmed'
			ORDER BY sl.product_id,COALESCE(ss.confirmed_at,ss.created_at) DESC) x
		WHERE st.session_id=? AND st.product_id=x.product_id AND st.inventory_status='shipped' AND st.document_id=''`, organizationID, sessionID).Error; err != nil {
			return err
		}
	}
	if tx.Migrator().HasTable("consignment_lines") {
		if err := tx.Exec(`UPDATE stocktake_lines st SET
			document_type='consignment', document_id=x.id, document_number=x.slip_number,
			document_partner_name=x.partner_name, shipment_issued_at=x.issued_at
		FROM (SELECT DISTINCT ON (cl.product_id) cl.product_id,cs.id,cs.slip_number,
			bp.legal_name AS partner_name,COALESCE(cs.issued_at,cs.confirmed_at,cs.created_at) AS issued_at
			FROM consignment_lines cl JOIN consignment_slips cs ON cs.id=cl.consignment_slip_id
			JOIN partner_roles pr ON pr.id=cs.consignee_role_id JOIN business_partners bp ON bp.id=pr.partner_id
			WHERE cs.organization_id=? AND cs.status='confirmed'
			ORDER BY cl.product_id,COALESCE(cs.issued_at,cs.confirmed_at,cs.created_at) DESC) x
		WHERE st.session_id=? AND st.product_id=x.product_id AND st.inventory_status='consigned' AND st.document_id=''`, organizationID, sessionID).Error; err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) StartStocktake(ctx context.Context, organizationID, actorUserID string) (StocktakeSession, error) {
	if current, err := r.CurrentStocktake(ctx, organizationID); err == nil {
		return current, nil
	}
	now := time.Now().UTC()
	session := StocktakeSession{ID: stocktakeID("stk_"), OrganizationID: organizationID, Status: "in_progress", StartedBy: actorUserID, StartedAt: now, SavedAt: now, CreatedAt: now, UpdatedAt: now}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&session).Error; err != nil {
			return err
		}
		var products []Product
		if err := tx.Where("organization_id = ? AND deleted_at IS NULL AND inventory_status IN ?", organizationID, stocktakeTargetStatuses).
			Order("product_code").Find(&products).Error; err != nil {
			return err
		}
		lines := make([]StocktakeLine, 0, len(products))
		for _, product := range products {
			lines = append(lines, StocktakeLine{ID: stocktakeID("stl_"), OrganizationID: organizationID, SessionID: session.ID,
				ProductID: product.ID, ProductCode: product.ProductCode, LineType: "expected_missing", ResultStatus: "missing", Source: "snapshot",
				InventoryStatus: product.InventoryStatus, Brand: product.Brand, ModelNumber: product.ModelNumber, ReferenceNumber: product.ReferenceNumber,
				SerialNumber: product.SerialNumber, PurchasePriceMinor: product.CostAmountMinor, CreatedAt: now, UpdatedAt: now})
		}
		if len(lines) > 0 {
			if err := tx.CreateInBatches(&lines, 200).Error; err != nil {
				return err
			}
			if !tx.Migrator().HasTable("shipment_lines") || !tx.Migrator().HasTable("shipment_slips") {
				return nil
			}
			return tx.Exec(`UPDATE stocktake_lines SET shipment_issued_at = (
				SELECT MAX(ss.created_at) FROM shipment_lines sl
				JOIN shipment_slips ss ON ss.id=sl.shipment_slip_id
				WHERE sl.product_id=stocktake_lines.product_id AND ss.organization_id=?
			) WHERE session_id=? AND EXISTS (
				SELECT 1 FROM shipment_lines sl JOIN shipment_slips ss ON ss.id=sl.shipment_slip_id
				WHERE sl.product_id=stocktake_lines.product_id AND ss.organization_id=?
			)`, organizationID, session.ID, organizationID).Error
		}
		return nil
	})
	if err != nil {
		// A concurrent start can win the partial unique index. Resume it.
		if current, currentErr := r.CurrentStocktake(ctx, organizationID); currentErr == nil {
			return current, nil
		}
		return session, err
	}
	return r.CurrentStocktake(ctx, organizationID)
}

// SyncStocktake adds inventory registered after a stocktake was started without
// replacing the original snapshot or losing already verified/discrepancy lines.
// If an unregistered QR was scanned first and the product is registered later,
// that scan is reconciled into a verified expected line.
func (r *Repository) SyncStocktake(ctx context.Context, organizationID, sessionID string) (StocktakeSession, int, error) {
	added := 0
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var session StocktakeSession
		if err := tx.Where("id = ? AND organization_id = ? AND status = ?", sessionID, organizationID, "in_progress").First(&session).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrStocktakeNotFound
			}
			return err
		}
		// 仕入返品処理済商品（内部値 cancelled）は棚卸対象外。過去の棚卸仕様で作られた
		// 不明在庫行や、棚卸開始後に仕入返品処理済へ変わった対象行も同期時に確実に除外する。
		if err := tx.Exec(`DELETE FROM stocktake_lines
			WHERE session_id = ? AND organization_id = ? AND (
				source = 'cancelled' OR inventory_status = 'cancelled' OR
				product_id IN (
					SELECT id FROM products
					WHERE organization_id = ? AND deleted_at IS NULL AND inventory_status = 'cancelled'
				)
			)`, sessionID, organizationID, organizationID).Error; err != nil {
			return err
		}

		var products []Product
		if err := tx.Where("organization_id = ? AND deleted_at IS NULL AND inventory_status IN ?", organizationID, stocktakeTargetStatuses).
			Order("product_code").Find(&products).Error; err != nil {
			return err
		}
		var lines []StocktakeLine
		if err := tx.Where("session_id = ?", sessionID).Find(&lines).Error; err != nil {
			return err
		}
		expectedByProduct := make(map[string]struct{}, len(lines))
		unknownByCode := make(map[string]StocktakeLine)
		for _, line := range lines {
			if line.LineType == "expected_missing" && line.ProductID != "" {
				expectedByProduct[line.ProductID] = struct{}{}
			}
			if line.LineType == "unknown_inventory" {
				unknownByCode[line.ProductCode] = line
			}
		}

		now := time.Now().UTC()
		for _, product := range products {
			if _, exists := expectedByProduct[product.ID]; exists {
				continue
			}
			line := StocktakeLine{ID: stocktakeID("stl_"), OrganizationID: organizationID, SessionID: sessionID,
				ProductID: product.ID, ProductCode: product.ProductCode, LineType: "expected_missing", ResultStatus: "missing", Source: "registered_during_stocktake",
				InventoryStatus: product.InventoryStatus, Brand: product.Brand, ModelNumber: product.ModelNumber, ReferenceNumber: product.ReferenceNumber,
				SerialNumber: product.SerialNumber, PurchasePriceMinor: product.CostAmountMinor, CreatedAt: now, UpdatedAt: now}
			if unknown, exists := unknownByCode[product.ProductCode]; exists {
				line.ResultStatus = "verified"
				line.Source = "registered_after_unknown_scan"
				line.CheckedAt = unknown.CheckedAt
				if err := tx.Delete(&StocktakeLine{}, "id = ?", unknown.ID).Error; err != nil {
					return err
				}
			}
			if err := tx.Create(&line).Error; err != nil {
				return err
			}
			expectedByProduct[product.ID] = struct{}{}
			added++
		}
		if added > 0 {
			return tx.Model(&StocktakeSession{}).Where("id = ?", sessionID).Updates(map[string]any{"saved_at": now, "updated_at": now}).Error
		}
		return nil
	})
	if err != nil {
		return StocktakeSession{}, added, err
	}
	session, err := r.CurrentStocktake(ctx, organizationID)
	return session, added, err
}

func (r *Repository) ScanStocktake(ctx context.Context, organizationID, sessionID, code string) (StocktakeSession, string, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return StocktakeSession{}, "", fmt.Errorf("product code is required")
	}
	message := "verified"
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var session StocktakeSession
		if err := tx.Where("id = ? AND organization_id = ? AND status = ?", sessionID, organizationID, "in_progress").First(&session).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrStocktakeNotFound
			}
			return err
		}
		var expected StocktakeLine
		if err := tx.Where("session_id = ? AND product_code = ? AND line_type = ?", sessionID, code, "expected_missing").First(&expected).Error; err == nil {
			if expected.InventoryStatus == "shipped" || expected.InventoryStatus == "consigned" {
				if expected.DocumentCheckedAt == nil {
					message = "document_unchecked"
					return nil
				}
				if expected.ResultStatus == "duplicate_presence" {
					message = "already_duplicate_presence"
					return nil
				}
				now := time.Now().UTC()
				message = "duplicate_presence"
				return tx.Model(&StocktakeLine{}).Where("id = ?", expected.ID).Updates(map[string]any{
					"result_status": "duplicate_presence", "reason": "伝票確認後に実在庫を検出（二重確認）",
					"checked_at": now, "updated_at": now,
				}).Error
			}
			if expected.ResultStatus == "verified" {
				message = "already_verified"
				return nil
			}
			now := time.Now().UTC()
			return tx.Model(&StocktakeLine{}).Where("id = ?", expected.ID).Updates(map[string]any{"result_status": "verified", "checked_at": now, "updated_at": now}).Error
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		var product Product
		source := "unregistered"
		if err := tx.Where("organization_id = ? AND deleted_at IS NULL AND product_code = ?", organizationID, code).First(&product).Error; err == nil {
			if product.InventoryStatus == "cancelled" {
				message = "cancelled_ignored"
				return nil
			}
			source = "non_target"
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		now := time.Now().UTC()
		line := StocktakeLine{ID: stocktakeID("stl_"), OrganizationID: organizationID, SessionID: sessionID, ProductID: product.ID,
			ProductCode: code, LineType: "unknown_inventory", ResultStatus: "unknown", Source: source, InventoryStatus: product.InventoryStatus,
			Brand: product.Brand, ModelNumber: product.ModelNumber, ReferenceNumber: product.ReferenceNumber, SerialNumber: product.SerialNumber,
			PurchasePriceMinor: product.CostAmountMinor, Reason: "不明在庫", CheckedAt: &now, CreatedAt: now, UpdatedAt: now}
		var existing StocktakeLine
		findErr := tx.Where("session_id = ? AND product_code = ? AND line_type = ?", sessionID, code, "unknown_inventory").First(&existing).Error
		if findErr == nil {
			message = "already_unknown"
			return nil
		}
		if !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return findErr
		}
		message = "unknown_added"
		return tx.Create(&line).Error
	})
	if err != nil {
		return StocktakeSession{}, message, err
	}
	session, err := r.CurrentStocktake(ctx, organizationID)
	return session, message, err
}

func (r *Repository) ConfirmStocktakeDocument(ctx context.Context, organizationID, sessionID, documentType, documentID, actorUserID string, productCodes []string) (StocktakeSession, int64, error) {
	if documentType != "shipment" && documentType != "consignment" {
		return StocktakeSession{}, 0, fmt.Errorf("invalid document type")
	}
	now := time.Now().UTC()
	var affected int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var session StocktakeSession
		if err := tx.Where("id=? AND organization_id=? AND status='in_progress'", sessionID, organizationID).First(&session).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrStocktakeNotFound
			}
			return err
		}
		query := tx.Model(&StocktakeLine{}).Where(
			"session_id=? AND organization_id=? AND document_type=? AND document_id=? AND line_type='expected_missing'",
			sessionID, organizationID, documentType, documentID,
		)
		if len(productCodes) > 0 {
			query = query.Where("product_code IN ?", productCodes)
		}
		result := query.Updates(map[string]any{
			"result_status": "verified", "checked_at": now, "document_checked_at": now,
			"document_checked_by": actorUserID, "reason": "", "updated_at": now,
		})
		if result.Error != nil {
			return result.Error
		}
		affected = result.RowsAffected
		if affected == 0 {
			return fmt.Errorf("stocktake document not found")
		}
		return tx.Model(&StocktakeSession{}).Where("id=?", sessionID).Updates(map[string]any{"saved_at": now, "updated_at": now}).Error
	})
	if err != nil {
		return StocktakeSession{}, affected, err
	}
	session, err := r.CurrentStocktake(ctx, organizationID)
	return session, affected, err
}

func (r *Repository) SaveStocktake(ctx context.Context, organizationID, sessionID string, updates map[string]struct{ Reason, Note string }, resolvedIDs []string) (StocktakeSession, error) {
	now := time.Now().UTC()
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&StocktakeSession{}).Where("id = ? AND organization_id = ? AND status = ?", sessionID, organizationID, "in_progress").Updates(map[string]any{"saved_at": now, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrStocktakeNotFound
		}
		for id, update := range updates {
			if err := tx.Model(&StocktakeLine{}).Where("id = ? AND session_id = ? AND organization_id = ?", id, sessionID, organizationID).
				Updates(map[string]any{"reason": strings.TrimSpace(update.Reason), "note": strings.TrimSpace(update.Note), "updated_at": now}).Error; err != nil {
				return err
			}
		}
		if len(resolvedIDs) > 0 {
			if err := tx.Model(&StocktakeLine{}).Where("session_id=? AND organization_id=? AND id IN ?", sessionID, organizationID, resolvedIDs).
				Updates(map[string]any{"resolved_at": now, "updated_at": now}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return StocktakeSession{}, err
	}
	return r.CurrentStocktake(ctx, organizationID)
}

func (r *Repository) CompleteStocktake(ctx context.Context, organizationID, sessionID, actorUserID string) (StocktakeSession, error) {
	now := time.Now().UTC()
	var session StocktakeSession
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ? AND organization_id = ? AND status = ?", sessionID, organizationID, "in_progress").First(&session).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrStocktakeNotFound
			}
			return err
		}
		var unresolved int64
		if err := tx.Model(&StocktakeLine{}).Where("session_id = ? AND line_type = ? AND result_status = ? AND reason = ''", sessionID, "expected_missing", "missing").Count(&unresolved).Error; err != nil {
			return err
		}
		if unresolved > 0 {
			return fmt.Errorf("%d expected items have no discrepancy reason", unresolved)
		}
		return tx.Model(&StocktakeSession{}).Where("id = ?", sessionID).Updates(map[string]any{"status": "completed", "completed_by": actorUserID, "completed_at": now, "inventory_date": now.Format("2006-01-02"), "saved_at": now, "updated_at": now}).Error
	})
	if err != nil {
		return session, err
	}
	if err := r.db.WithContext(ctx).Where("id = ? AND organization_id = ?", sessionID, organizationID).First(&session).Error; err != nil {
		return session, err
	}
	if err := r.db.WithContext(ctx).Where("session_id = ?", session.ID).Order("created_at, product_code").Find(&session.Lines).Error; err != nil {
		return session, err
	}
	return session, nil
}
