package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"gorm.io/gorm"
)

var (
	ErrConsignmentPrice           = errors.New("invalid consignment sale price")
	ErrConsignmentNotFound        = errors.New("consignment slip not found")
	ErrConsignmentState           = errors.New("consignment slip cannot be changed in its current state")
	ErrConsignmentProductNotFound = errors.New("product is not included in consignment slip")
	ErrConsignmentReturnState     = errors.New("consignment product cannot be returned in its current state")
)

type ConsignmentPriceInput struct {
	ProductCode       string `json:"productCode"`
	SalePriceUSDMinor *int64 `json:"salePriceUsdMinor"`
}

// Omitted prices retain the legacy inventory defaults; explicit zero is a price.
func consignmentInputPrices(codes []string, items []ConsignmentPriceInput) (map[string]int64, error) {
	prices := make(map[string]int64)
	if items == nil {
		return prices, nil
	}
	if len(items) != len(codes) {
		return nil, ErrConsignmentPrice
	}
	allowed := make(map[string]bool, len(codes))
	for _, code := range codes {
		allowed[code] = true
	}
	for _, item := range items {
		code := strings.ToUpper(strings.TrimSpace(item.ProductCode))
		_, duplicate := prices[code]
		if !allowed[code] || duplicate || item.SalePriceUSDMinor == nil || *item.SalePriceUSDMinor < 0 || *item.SalePriceUSDMinor > 1000000000000 {
			return nil, ErrConsignmentPrice
		}
		prices[code] = *item.SalePriceUSDMinor
	}
	return prices, nil
}

type ConsignmentCreateInput struct {
	rebuild         *returnedDraftIdentity // Set only inside the owner-checked resubmission transaction.
	draft           bool
	Items           []ConsignmentPriceInput `json:"items,omitempty"`
	OrganizationID  string
	ActorUserID     string
	ConsigneeCode   string   `json:"consigneeCode"`
	ConsignmentDate string   `json:"consignmentDate"`
	Notes           string   `json:"notes"`
	ProductCodes    []string `json:"productCodes"`
}

type ConsignmentLineRecord struct {
	ConsignmentPending    bool   `json:"consignmentPending"`
	ID                    string `json:"id"`
	LineNumber            int    `json:"lineNumber"`
	ProductID             string `json:"productId"`
	ProductCode           string `json:"productCode"`
	Brand                 string `json:"brand"`
	ModelNumber           string `json:"modelNumber"`
	SalePriceUSDMinor     int64  `json:"salePriceUsdMinor"`
	ConvertedSalePriceJPY int64  `json:"convertedSalePriceJpy"`
}

type ConsignmentSlipRecord struct {
	Revisions        []ConsignmentRevisionRecord `gorm:"-" json:"revisions,omitempty"`
	ProcessingStatus string                      `json:"processingStatus"`
	ID               string                      `json:"id"`
	SlipNumber       string                      `json:"slipNumber"`
	ConsigneeCode    string                      `json:"consigneeCode"`
	ConsigneeName    string                      `json:"consigneeName"`
	ConsignmentDate  DateString                  `json:"consignmentDate"`
	Status           string                      `json:"status"`
	Notes            string                      `json:"notes"`
	DisplayCurrency  string                      `json:"displayCurrency"`
	FXRateSnapshotID string                      `json:"fxRateSnapshotId,omitempty"`
	FXRateScaled     int64                       `json:"fxRateScaled"`
	FXScale          int64                       `json:"fxScale"`
	IssuedAt         *time.Time                  `json:"issuedAt,omitempty"`
	IssuedBy         string                      `json:"issuedBy,omitempty"`
	ConfirmedAt      time.Time                   `json:"confirmedAt"`
	CreatedAt        time.Time                   `json:"createdAt"`
	UpdatedAt        time.Time                   `json:"updatedAt"`
	Lines            []ConsignmentLineRecord     `gorm:"-" json:"lines,omitempty"`
	OfficialPDF      *OfficialDocumentRef        `gorm:"-" json:"officialPdf,omitempty"`
}

// ReturnConsignmentProduct moves one product on a confirmed consignment back
// into stock. Membership and state are checked under locks so a QR from another
// slip or a duplicate scan cannot change unrelated inventory.
func (r *Repository) ReturnConsignmentProduct(ctx context.Context, organizationID, consignmentID, productCode, actorUserID string) (ShipmentReturnScanResult, error) {
	var scan ShipmentReturnScanResult
	productCode = strings.TrimSpace(productCode)
	if productCode == "" {
		return scan, ErrConsignmentProductNotFound
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var slipStatus string
		result := tx.Raw(`SELECT status FROM consignment_slips WHERE organization_id=? AND id=? FOR UPDATE`, organizationID, consignmentID).Scan(&slipStatus)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrConsignmentNotFound
		}
		if slipStatus != "confirmed" {
			return ErrConsignmentState
		}
		var product struct{ ID, ProductCode, InventoryStatus string }
		result = tx.Raw(`SELECT p.id,p.product_code,p.inventory_status
			FROM consignment_lines l JOIN products p ON p.id=l.product_id
			WHERE l.consignment_slip_id=? AND p.organization_id=? AND p.product_code=? AND p.deleted_at IS NULL
			FOR UPDATE`, consignmentID, organizationID, productCode).Scan(&product)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrConsignmentProductNotFound
		}
		scan = ShipmentReturnScanResult{ProductCode: product.ProductCode, PreviousStatus: product.InventoryStatus, CurrentStatus: product.InventoryStatus}
		if product.InventoryStatus == "in_stock" {
			scan.Result = "already_in_stock"
			return nil
		}
		if product.InventoryStatus != "consigned" {
			return ErrConsignmentReturnState
		}
		var pending bool
		if err := tx.Table("consignment_lines AS l").Select("("+consignmentLinePendingSQL+")").
			Joins("JOIN consignment_slips c ON c.id=l.consignment_slip_id").Joins("JOIN products p ON p.id=l.product_id").
			Where("c.organization_id=? AND c.id=? AND p.id=?", organizationID, consignmentID, product.ID).Scan(&pending).Error; err != nil {
			return err
		}
		if !pending {
			return ErrShipmentLineCompleted
		}
		now := time.Now().UTC()
		if err := tx.Exec(`UPDATE products SET inventory_status='in_stock',updated_at=? WHERE id=?`, now, product.ID).Error; err != nil {
			return err
		}
		eventID, err := database.NewID("ive")
		if err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO inventory_events(
			id,organization_id,product_id,event_type,from_status,to_status,reason,actor_user_id,created_at
		) VALUES(?,?,?,'consignment_return_scanned','consigned','in_stock','委託伝票詳細で返却QRを読取',?,?)`,
			eventID, organizationID, product.ID, actorUserID, now).Error; err != nil {
			return err
		}
		scan.CurrentStatus = "in_stock"
		scan.Result = "returned"
		return nil
	})
	return scan, err
}

func (r *Repository) CreateConsignment(ctx context.Context, input ConsignmentCreateInput) (ConsignmentSlipRecord, error) {
	date, err := time.Parse("2006-01-02", input.ConsignmentDate)
	if err != nil {
		return ConsignmentSlipRecord{}, err
	}
	productCodes := normalizeCodes(input.ProductCodes)
	if len(productCodes) == 0 || len(productCodes) > 100 {
		return ConsignmentSlipRecord{}, ErrConsignmentState
	}

	prices, err := consignmentInputPrices(productCodes, input.Items)
	if err != nil {
		return ConsignmentSlipRecord{}, err
	}
	var consignmentID string
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		consigneeRoleID, err := lookupBuyerRole(tx, input.OrganizationID, input.ConsigneeCode)
		if err != nil {
			return err
		}
		rate, err := returnedDraftFX(tx, input.OrganizationID, "USD", input.rebuild)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		var slipNumber string
		consignmentID, slipNumber, err = returnedDraftNumber(tx, input.OrganizationID, "consignment_slips", "IT", "cns", date, now, input.rebuild)
		if err != nil {
			return err
		}
		status := "confirmed"
		var confirmedAt any = now
		var confirmedBy any = input.ActorUserID
		if input.draft {
			status, confirmedAt, confirmedBy = "draft", nil, nil
		}
		if err := tx.Exec(`INSERT INTO consignment_slips(
			id,organization_id,slip_number,consignee_role_id,consignment_date,status,notes,
			display_currency,fx_rate_snapshot_id,fx_rate_scaled,fx_scale,
			confirmed_at,confirmed_by,created_by,created_at,updated_at
		) VALUES(?,?,?,?,?,?,?,'JPY',?,?,?,?,?,?,?,?) ON CONFLICT (id) DO UPDATE SET consignee_role_id=EXCLUDED.consignee_role_id,consignment_date=EXCLUDED.consignment_date,notes=EXCLUDED.notes,display_currency=EXCLUDED.display_currency,fx_rate_snapshot_id=EXCLUDED.fx_rate_snapshot_id,fx_rate_scaled=EXCLUDED.fx_rate_scaled,fx_scale=EXCLUDED.fx_scale,status=EXCLUDED.status,updated_at=EXCLUDED.updated_at`, consignmentID, input.OrganizationID, slipNumber,
			consigneeRoleID, date, status, strings.TrimSpace(input.Notes), rate.ID, rate.RateScaled, rate.Scale,
			confirmedAt, confirmedBy, input.ActorUserID, now, now).Error; err != nil {
			return err
		}

		for index, productCode := range productCodes {
			var product struct {
				ID, InventoryStatus, BaseSaleCurrency string
				BaseSalePriceMinor                    int64
			}
			result := tx.Raw(`SELECT id,inventory_status,base_sale_price_minor,base_sale_currency FROM products
				WHERE organization_id=? AND product_code=? AND deleted_at IS NULL FOR UPDATE`,
				input.OrganizationID, productCode).Scan(&product)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 || product.InventoryStatus != "in_stock" {
				return ErrProductUnavailable
			}
			salePriceUSD, err := convertCurrency(product.BaseSalePriceMinor, product.BaseSaleCurrency, "USD", rate)
			if err != nil {
				return err
			}
			if price, supplied := prices[productCode]; supplied {
				salePriceUSD = price
			}
			convertedJPY := convertShipmentUSDToJPYRoundUpToThousand(salePriceUSD, rate.RateScaled, rate.Scale)
			lineID, err := returnedDraftLineID(input.rebuild, index, "cnl")
			if err != nil {
				return err
			}
			if err := tx.Exec(`INSERT INTO consignment_lines(
				id,consignment_slip_id,line_number,product_id,quantity,sale_price_usd_minor,converted_sale_price_jpy,created_at
			) VALUES(?,?,?,?,1,?,?,?)`, lineID, consignmentID, index+1, product.ID, salePriceUSD, convertedJPY, now).Error; err != nil {
				return err
			}
			if input.draft {
				continue
			}
			if err := tx.Exec(`UPDATE products SET inventory_status='consigned',updated_at=? WHERE id=?`, now, product.ID).Error; err != nil {
				return err
			}
			eventID, err := database.NewID("ive")
			if err != nil {
				return err
			}
			if err := tx.Exec(`INSERT INTO inventory_events(
				id,organization_id,product_id,event_type,from_status,to_status,reason,actor_user_id,created_at
			) VALUES(?,?,?,'consignment_registered','in_stock','consigned','委託登録',?,?)`,
				eventID, input.OrganizationID, product.ID, input.ActorUserID, now).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return ConsignmentSlipRecord{}, err
	}
	return r.ConsignmentSlip(ctx, input.OrganizationID, consignmentID)
}

func (r *Repository) ConsignmentSlips(ctx context.Context, organizationID string, limit int) ([]ConsignmentSlipRecord, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	var records []ConsignmentSlipRecord
	err := r.db.WithContext(ctx).Table("consignment_slips AS c").
		Select(`c.id,c.slip_number,cr.role_code AS consignee_code,bp.legal_name AS consignee_name,
			c.consignment_date,c.status,c.notes,c.display_currency,c.fx_rate_snapshot_id,c.fx_rate_scaled,c.fx_scale,
			c.issued_at,COALESCE(c.issued_by,'') AS issued_by,c.confirmed_at,c.created_at,c.updated_at,`+consignmentProcessingStatusSQL).
		Joins("JOIN partner_roles cr ON cr.id=c.consignee_role_id").
		Joins("JOIN business_partners bp ON bp.id=cr.partner_id").
		Where("c.organization_id=?", organizationID).
		Order("c.consignment_date DESC,c.slip_number DESC").Limit(limit).Scan(&records).Error
	return records, err
}

func (r *Repository) ConsignmentSlip(ctx context.Context, organizationID, consignmentID string) (ConsignmentSlipRecord, error) {
	var record ConsignmentSlipRecord
	result := r.db.WithContext(ctx).Table("consignment_slips AS c").
		Select(`c.id,c.slip_number,cr.role_code AS consignee_code,bp.legal_name AS consignee_name,
			c.consignment_date,c.status,c.notes,c.display_currency,c.fx_rate_snapshot_id,c.fx_rate_scaled,c.fx_scale,
			c.issued_at,COALESCE(c.issued_by,'') AS issued_by,c.confirmed_at,c.created_at,c.updated_at,`+consignmentProcessingStatusSQL).
		Joins("JOIN partner_roles cr ON cr.id=c.consignee_role_id").
		Joins("JOIN business_partners bp ON bp.id=cr.partner_id").
		Where("c.organization_id=? AND c.id=?", organizationID, consignmentID).Take(&record)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return ConsignmentSlipRecord{}, ErrConsignmentNotFound
	}
	if result.Error != nil {
		return ConsignmentSlipRecord{}, result.Error
	}
	if err := r.db.WithContext(ctx).Table("consignment_lines AS l").
		Select("l.id,l.line_number,l.product_id,p.product_code,p.brand,p.model_number,l.sale_price_usd_minor,l.converted_sale_price_jpy,("+consignmentLinePendingSQL+") AS consignment_pending").
		Joins("JOIN consignment_slips c ON c.id=l.consignment_slip_id").
		Joins("JOIN products p ON p.id=l.product_id").Where("l.consignment_slip_id=?", consignmentID).
		Order("l.line_number").Scan(&record.Lines).Error; err != nil {
		return ConsignmentSlipRecord{}, err
	}
	if r.db.Migrator().HasTable("audit_logs") {
		if err := r.db.WithContext(ctx).Table("audit_logs AS a").Select("a.created_at AS revised_at,COALESCE(u.display_name,'管理者') AS actor_name,a.reason").
			Joins("LEFT JOIN users u ON u.id=a.actor_user_id").Where("a.organization_id=? AND a.target_type='consignment_slip' AND a.target_id=? AND a.action='consignment.revised' AND a.result='success'", organizationID, consignmentID).
			Order("a.created_at,a.id").Scan(&record.Revisions).Error; err != nil {
			return ConsignmentSlipRecord{}, err
		}
	}
	return record, nil
}

// IssueConsignment records issuance evidence only. Registration-time prices and
// exchange rates are immutable snapshots and are never recalculated here.
func (r *Repository) IssueConsignment(ctx context.Context, organizationID, consignmentID, actorUserID string) (ConsignmentSlipRecord, error) {
	now := time.Now().UTC()
	result := r.db.WithContext(ctx).Exec(`UPDATE consignment_slips SET issued_at=?,issued_by=?,updated_at=?
		WHERE organization_id=? AND id=? AND status='confirmed'`, now, actorUserID, now, organizationID, consignmentID)
	if result.Error != nil {
		return ConsignmentSlipRecord{}, result.Error
	}
	if result.RowsAffected == 0 {
		return ConsignmentSlipRecord{}, ErrConsignmentNotFound
	}
	return r.ConsignmentSlip(ctx, organizationID, consignmentID)
}

const consignmentLinePendingSQL = `c.status='confirmed' AND c.confirmed_at IS NOT NULL
 AND p.inventory_status='consigned'
 AND NOT EXISTS (SELECT 1 FROM inventory_events e WHERE e.organization_id=c.organization_id
   AND e.product_id=l.product_id AND e.created_at>=c.confirmed_at AND e.from_status='consigned' AND e.to_status<>'consigned')
 AND NOT EXISTS (SELECT 1 FROM consignment_lines nl JOIN consignment_slips nc ON nc.id=nl.consignment_slip_id
   WHERE nl.product_id=l.product_id AND nc.organization_id=c.organization_id AND nc.status='confirmed' AND nc.confirmed_at>c.confirmed_at)`
const consignmentProcessingStatusSQL = `CASE WHEN c.status='draft' OR
 (c.status='confirmed' AND EXISTS (SELECT 1 FROM consignment_lines l JOIN products p ON p.id=l.product_id
   WHERE l.consignment_slip_id=c.id AND ` + consignmentLinePendingSQL + `)) THEN 'processing' ELSE 'completed' END AS processing_status`
