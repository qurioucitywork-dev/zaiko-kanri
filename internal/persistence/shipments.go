package persistence

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"gorm.io/gorm"
)

var (
	ErrShipmentNotFound = errors.New("shipment slip not found")
	ErrShipmentState    = errors.New("shipment slip cannot be changed in its current state")
)

type ShipmentCreateInput struct {
	OrganizationID   string
	ActorUserID      string
	BuyerCode        string   `json:"buyerCode"`
	SalesSlipNumber  string   `json:"salesSlipNumber"`
	ShipmentDate     string   `json:"shipmentDate"`
	RecipientName    string   `json:"recipientName"`
	RecipientAddress string   `json:"recipientAddress"`
	Carrier          string   `json:"carrier"`
	TrackingNumber   string   `json:"trackingNumber"`
	DisplayCurrency  string   `json:"displayCurrency"`
	Notes            string   `json:"notes"`
	ProductCodes     []string `json:"productCodes"`
}

type ShipmentLineRecord struct {
	ID                    string `json:"id"`
	LineNumber            int    `json:"lineNumber"`
	ProductID             string `json:"productId"`
	ProductCode           string `json:"productCode"`
	Brand                 string `json:"brand"`
	ModelNumber           string `json:"modelNumber"`
	SalePriceUSDMinor     int64  `json:"salePriceUsdMinor"`
	ConvertedSalePriceJPY int64  `json:"convertedSalePriceJpy"`
}

type ShipmentSlipRecord struct {
	ID               string               `json:"id"`
	SlipNumber       string               `json:"slipNumber"`
	BuyerCode        string               `json:"buyerCode"`
	BuyerName        string               `json:"buyerName"`
	SalesSlipNumber  string               `json:"salesSlipNumber,omitempty"`
	ShipmentDate     DateString           `json:"shipmentDate"`
	RecipientName    string               `json:"recipientName"`
	RecipientAddress string               `json:"recipientAddress"`
	Carrier          string               `json:"carrier"`
	TrackingNumber   string               `json:"trackingNumber"`
	DisplayCurrency  string               `json:"displayCurrency"`
	FXRateSnapshotID string               `json:"fxRateSnapshotId,omitempty"`
	FXRateScaled     int64                `json:"fxRateScaled"`
	FXScale          int64                `json:"fxScale"`
	Status           string               `json:"status"`
	Notes            string               `json:"notes"`
	ConfirmedAt      *time.Time           `json:"confirmedAt,omitempty"`
	CreatedAt        time.Time            `json:"createdAt"`
	UpdatedAt        time.Time            `json:"updatedAt"`
	Lines            []ShipmentLineRecord `gorm:"-" json:"lines,omitempty"`
}

func (r *Repository) UpdateShipmentTracking(ctx context.Context, organizationID, shipmentID, carrier, trackingNumber string) (ShipmentSlipRecord, error) {
	result := r.db.WithContext(ctx).Table("shipment_slips").Where("organization_id=? AND id=? AND status<>'cancelled'", organizationID, shipmentID).
		Updates(map[string]any{"carrier": strings.TrimSpace(carrier), "tracking_number": strings.TrimSpace(trackingNumber), "updated_at": time.Now().UTC()})
	if result.Error != nil {
		return ShipmentSlipRecord{}, result.Error
	}
	if result.RowsAffected == 0 {
		return ShipmentSlipRecord{}, ErrShipmentNotFound
	}
	return r.ShipmentSlip(ctx, organizationID, shipmentID)
}

func (r *Repository) CreateShipment(ctx context.Context, input ShipmentCreateInput) (ShipmentSlipRecord, error) {
	date, err := time.Parse("2006-01-02", input.ShipmentDate)
	if err != nil {
		return ShipmentSlipRecord{}, err
	}
	productCodes := normalizeCodes(input.ProductCodes)
	if len(productCodes) == 0 || len(productCodes) > 100 {
		return ShipmentSlipRecord{}, ErrShipmentState
	}
	input.DisplayCurrency = strings.ToUpper(strings.TrimSpace(input.DisplayCurrency))
	if input.DisplayCurrency == "" {
		input.DisplayCurrency = "USD"
	}
	if input.DisplayCurrency != "USD" && input.DisplayCurrency != "JPY" {
		return ShipmentSlipRecord{}, ErrShipmentState
	}
	var shipmentID string
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		buyerRoleID, err := lookupBuyerRole(tx, input.OrganizationID, input.BuyerCode)
		if err != nil {
			return err
		}
		rate, err := latestFX(tx, input.OrganizationID, "USD")
		if err != nil {
			return err
		}
		var salesSlipID string
		if strings.TrimSpace(input.SalesSlipNumber) != "" {
			result := tx.Table("sales_slips").Select("id").Where(
				"organization_id=? AND slip_number=? AND buyer_role_id=? AND status<>'cancelled'",
				input.OrganizationID, strings.TrimSpace(input.SalesSlipNumber), buyerRoleID).Scan(&salesSlipID)
			if result.Error != nil || salesSlipID == "" {
				return ErrSaleNotFound
			}
		}
		now := time.Now().UTC()
		sequence, err := nextDocumentSequence(tx, input.OrganizationID, "shipment", date.Year(), now)
		if err != nil {
			return err
		}
		shipmentID, err = database.NewID("shp")
		if err != nil {
			return err
		}
		slipNumber := fmt.Sprintf("SH-%04d-%04d", date.Year(), sequence)
		if err := tx.Exec(`INSERT INTO shipment_slips(
			id,organization_id,slip_number,buyer_role_id,sales_slip_id,shipment_date,recipient_name,
			recipient_address,carrier,tracking_number,display_currency,fx_rate_snapshot_id,fx_rate_scaled,fx_scale,
			status,notes,created_by,created_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,'draft',?,?,?,?)`, shipmentID, input.OrganizationID, slipNumber,
			buyerRoleID, nullIfEmpty(salesSlipID), date, strings.TrimSpace(input.RecipientName),
			strings.TrimSpace(input.RecipientAddress), strings.TrimSpace(input.Carrier),
			strings.TrimSpace(input.TrackingNumber), input.DisplayCurrency, rate.ID, rate.RateScaled, rate.Scale,
			strings.TrimSpace(input.Notes), input.ActorUserID, now, now).Error; err != nil {
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
			if result.RowsAffected == 0 || !map[string]bool{"in_stock": true, "reserved": true}[product.InventoryStatus] {
				return ErrProductUnavailable
			}
			if product.InventoryStatus == "reserved" {
				linked, err := productBelongsToBuyer(tx, input.OrganizationID, product.ID, buyerRoleID)
				if err != nil || !linked {
					return ErrProductConflict
				}
			}
			salePriceUSD, err := convertCurrency(product.BaseSalePriceMinor, product.BaseSaleCurrency, "USD", rate)
			if err != nil {
				return err
			}
			convertedJPY := convertShipmentUSDToJPYRoundUpToThousand(
				salePriceUSD, rate.RateScaled, rate.Scale,
			)
			lineID, _ := database.NewID("shl")
			if err := tx.Exec(`INSERT INTO shipment_lines(
				id,shipment_slip_id,line_number,product_id,quantity,sale_price_usd_minor,converted_sale_price_jpy,created_at
			) VALUES(?,?,?,?,1,?,?,?)`, lineID, shipmentID, index+1, product.ID, salePriceUSD, convertedJPY, now).Error; err != nil {
				return err
			}
			if product.InventoryStatus == "in_stock" {
				if err := tx.Exec(`UPDATE products SET inventory_status='reserved',updated_at=? WHERE id=?`, now, product.ID).Error; err != nil {
					return err
				}
				eventID, _ := database.NewID("ive")
				if err := tx.Exec(`INSERT INTO inventory_events(
					id,organization_id,product_id,event_type,from_status,to_status,reason,actor_user_id,created_at
				) VALUES(?,?,?,'shipment_draft_created','in_stock','reserved','出荷伝票へ割当',?,?)`,
					eventID, input.OrganizationID, product.ID, input.ActorUserID, now).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return ShipmentSlipRecord{}, err
	}
	return r.ShipmentSlip(ctx, input.OrganizationID, shipmentID)
}

func convertShipmentUSDToJPYRoundUpToThousand(amountUSD, rateScaled, scale int64) int64 {
	if amountUSD <= 0 || rateScaled <= 0 || scale <= 0 {
		return 0
	}
	numerator := new(big.Int).Mul(big.NewInt(amountUSD), big.NewInt(rateScaled))
	denominator := new(big.Int).Mul(big.NewInt(scale), big.NewInt(1000))
	thousands, remainder := new(big.Int), new(big.Int)
	thousands.QuoRem(numerator, denominator, remainder)
	if remainder.Sign() > 0 {
		thousands.Add(thousands, big.NewInt(1))
	}
	result := thousands.Mul(thousands, big.NewInt(1000))
	if !result.IsInt64() {
		return 0
	}
	return result.Int64()
}

func (r *Repository) ShipmentSlips(ctx context.Context, organizationID string, limit int) ([]ShipmentSlipRecord, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	var records []ShipmentSlipRecord
	err := r.db.WithContext(ctx).Table("shipment_slips AS s").
		Select(`s.id,s.slip_number,br.role_code AS buyer_code,bp.legal_name AS buyer_name,
			COALESCE(sa.slip_number,'') AS sales_slip_number,s.shipment_date,s.recipient_name,s.recipient_address,
			s.carrier,s.tracking_number,s.display_currency,s.fx_rate_snapshot_id,s.fx_rate_scaled,s.fx_scale,
			s.status,s.notes,s.confirmed_at,s.created_at,s.updated_at`).
		Joins("JOIN partner_roles br ON br.id=s.buyer_role_id").Joins("JOIN business_partners bp ON bp.id=br.partner_id").
		Joins("LEFT JOIN sales_slips sa ON sa.id=s.sales_slip_id").Where("s.organization_id=?", organizationID).
		Order("s.shipment_date DESC,s.slip_number DESC").Limit(limit).Scan(&records).Error
	return records, err
}

func (r *Repository) ShipmentSlip(ctx context.Context, organizationID, shipmentID string) (ShipmentSlipRecord, error) {
	var record ShipmentSlipRecord
	result := r.db.WithContext(ctx).Table("shipment_slips AS s").
		Select(`s.id,s.slip_number,br.role_code AS buyer_code,bp.legal_name AS buyer_name,
			COALESCE(sa.slip_number,'') AS sales_slip_number,s.shipment_date,s.recipient_name,s.recipient_address,
			s.carrier,s.tracking_number,s.display_currency,s.fx_rate_snapshot_id,s.fx_rate_scaled,s.fx_scale,
			s.status,s.notes,s.confirmed_at,s.created_at,s.updated_at`).
		Joins("JOIN partner_roles br ON br.id=s.buyer_role_id").Joins("JOIN business_partners bp ON bp.id=br.partner_id").
		Joins("LEFT JOIN sales_slips sa ON sa.id=s.sales_slip_id").Where("s.organization_id=? AND s.id=?", organizationID, shipmentID).Take(&record)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return ShipmentSlipRecord{}, ErrShipmentNotFound
	}
	if result.Error != nil {
		return ShipmentSlipRecord{}, result.Error
	}
	if err := r.db.WithContext(ctx).Table("shipment_lines AS l").
		Select("l.id,l.line_number,l.product_id,p.product_code,p.brand,p.model_number,l.sale_price_usd_minor,l.converted_sale_price_jpy").
		Joins("JOIN products p ON p.id=l.product_id").Where("l.shipment_slip_id=?", shipmentID).
		Order("l.line_number").Scan(&record.Lines).Error; err != nil {
		return ShipmentSlipRecord{}, err
	}
	return record, nil
}

func (r *Repository) ConfirmShipment(ctx context.Context, organizationID, shipmentID, actorUserID string) (ShipmentSlipRecord, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var slip struct{ Status, BuyerRoleID string }
		result := tx.Raw(`SELECT status,buyer_role_id FROM shipment_slips WHERE organization_id=? AND id=? FOR UPDATE`, organizationID, shipmentID).Scan(&slip)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrShipmentNotFound
		}
		if slip.Status == "confirmed" {
			return nil
		}
		if slip.Status != "draft" {
			return ErrShipmentState
		}
		var productIDs []string
		if err := tx.Table("shipment_lines").Where("shipment_slip_id=?", shipmentID).Order("line_number").Pluck("product_id", &productIDs).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		for _, productID := range productIDs {
			var status string
			if err := tx.Raw(`SELECT inventory_status FROM products WHERE organization_id=? AND id=? FOR UPDATE`, organizationID, productID).Scan(&status).Error; err != nil {
				return err
			}
			if status != "reserved" && status != "in_stock" {
				return ErrProductConflict
			}
			if status == "reserved" {
				linked, err := productBelongsToBuyer(tx, organizationID, productID, slip.BuyerRoleID)
				if err != nil || !linked {
					return ErrProductConflict
				}
			}
			if err := tx.Exec(`UPDATE products SET inventory_status='shipped',updated_at=? WHERE id=?`, now, productID).Error; err != nil {
				return err
			}
			eventID, _ := database.NewID("ive")
			if err := tx.Exec(`INSERT INTO inventory_events(
				id,organization_id,product_id,event_type,from_status,to_status,reason,actor_user_id,created_at
			) VALUES(?,?,?,'shipment_confirmed',?,'shipped','出荷伝票確定',?,?)`, eventID, organizationID,
				productID, status, actorUserID, now).Error; err != nil {
				return err
			}
		}
		return tx.Exec(`UPDATE shipment_slips SET status='confirmed',confirmed_at=?,confirmed_by=?,updated_at=?
			WHERE organization_id=? AND id=?`, now, actorUserID, now, organizationID, shipmentID).Error
	})
	if err != nil {
		return ShipmentSlipRecord{}, err
	}
	return r.ShipmentSlip(ctx, organizationID, shipmentID)
}
