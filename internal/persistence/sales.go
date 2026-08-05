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
	ErrBuyerNotFound   = errors.New("buyer role not found")
	ErrSaleNotFound    = errors.New("sales slip not found")
	ErrSaleState       = errors.New("sales slip cannot be changed in its current state")
	ErrExchangeRate    = errors.New("exchange rate is not configured")
	ErrProductConflict = errors.New("product is already used by another transaction")
)

type SaleLineInput struct {
	ProductCode    string `json:"productCode"`
	UnitPriceMinor int64  `json:"unitPriceMinor"`
}

type SaleCreateInput struct {
	OrganizationID     string
	ActorUserID        string
	BuyerCode          string          `json:"buyerCode"`
	SaleDate           string          `json:"saleDate"`
	DisplayCurrency    string          `json:"displayCurrency"`
	TaxMode            string          `json:"taxMode"`
	TaxRateBasisPoints int             `json:"taxRateBasisPoints"`
	Notes              string          `json:"notes"`
	Lines              []SaleLineInput `json:"lines"`
}

type SaleLineRecord struct {
	ID                string `json:"id"`
	LineNumber        int    `json:"lineNumber"`
	ProductID         string `json:"productId"`
	ProductCode       string `json:"productCode"`
	Brand             string `json:"brand"`
	ModelNumber       string `json:"modelNumber"`
	UnitPriceMinor    int64  `json:"unitPriceMinor"`
	SaleCurrency      string `json:"saleCurrency"`
	SubtotalMinor     int64  `json:"subtotalMinor"`
	TaxAmountMinor    int64  `json:"taxAmountMinor"`
	TotalMinor        int64  `json:"totalMinor"`
	ConvertedTotalJPY int64  `json:"convertedTotalJpy"`
}

type SaleSlipRecord struct {
	ID                 string           `json:"id"`
	SlipNumber         string           `json:"slipNumber"`
	BuyerCode          string           `json:"buyerCode"`
	BuyerName          string           `json:"buyerName"`
	SaleDate           DateString       `json:"saleDate"`
	DisplayCurrency    string           `json:"displayCurrency"`
	TaxMode            string           `json:"taxMode"`
	TaxRateBasisPoints int              `json:"taxRateBasisPoints"`
	Status             string           `json:"status"`
	Notes              string           `json:"notes"`
	SubtotalMinor      int64            `json:"subtotalMinor"`
	TaxAmountMinor     int64            `json:"taxAmountMinor"`
	TotalMinor         int64            `json:"totalMinor"`
	ConvertedTotalJPY  int64            `json:"convertedTotalJpy"`
	ConfirmedAt        *time.Time       `json:"confirmedAt,omitempty"`
	CreatedAt          time.Time        `json:"createdAt"`
	UpdatedAt          time.Time        `json:"updatedAt"`
	Lines              []SaleLineRecord `gorm:"-" json:"lines,omitempty"`
}

type fxSnapshot struct {
	ID         string
	RateScaled int64
	Scale      int64
}

func latestFX(tx *gorm.DB, organizationID string) (fxSnapshot, error) {
	var rate fxSnapshot
	result := tx.Table("exchange_rate_snapshots").Select("id,rate_scaled,scale").Where(
		"organization_id=? AND base_currency='USD' AND quote_currency='JPY'", organizationID).
		Order("observed_at DESC,created_at DESC").Limit(1).Scan(&rate)
	if result.Error != nil || rate.ID == "" || rate.RateScaled <= 0 || rate.Scale <= 0 {
		return fxSnapshot{}, ErrExchangeRate
	}
	return rate, nil
}

func mulDivRound(value, multiplier, divisor int64) int64 {
	if value == 0 || multiplier == 0 || divisor <= 0 {
		return 0
	}
	numerator := new(big.Int).Mul(big.NewInt(value), big.NewInt(multiplier))
	numerator.Add(numerator, big.NewInt(divisor/2))
	return numerator.Div(numerator, big.NewInt(divisor)).Int64()
}

func convertCurrency(amount int64, from, to string, rate fxSnapshot) (int64, error) {
	if from == to {
		return amount, nil
	}
	if from == "USD" && to == "JPY" {
		return mulDivRound(amount, rate.RateScaled, rate.Scale), nil
	}
	if from == "JPY" && to == "USD" {
		return mulDivRound(amount, rate.Scale, rate.RateScaled), nil
	}
	return 0, ErrExchangeRate
}

func purchaseCostSnapshot(tx *gorm.DB, organizationID string, unitAmount int64, quantity int, currency string) (int64, any, any, any, error) {
	total := unitAmount * int64(quantity)
	if currency == "JPY" {
		return total, nil, nil, nil, nil
	}
	rate, err := latestFX(tx, organizationID)
	if err != nil {
		return 0, nil, nil, nil, err
	}
	converted, err := convertCurrency(total, "USD", "JPY", rate)
	if err != nil {
		return 0, nil, nil, nil, err
	}
	return converted, rate.ID, rate.RateScaled, rate.Scale, nil
}

func lookupBuyerRole(tx *gorm.DB, organizationID, buyerCode string) (string, error) {
	var id string
	result := tx.Table("partner_roles").Select("id").Where(
		"organization_id=? AND role_type='buyer' AND role_code=? AND is_active", organizationID,
		strings.ToUpper(strings.TrimSpace(buyerCode))).Scan(&id)
	if result.Error != nil || id == "" {
		return "", ErrBuyerNotFound
	}
	return id, nil
}

func productBelongsToBuyer(tx *gorm.DB, organizationID, productID, buyerRoleID string) (bool, error) {
	var count int64
	err := tx.Raw(`SELECT COUNT(*) FROM (
		SELECT 1 FROM purchase_requests WHERE organization_id=? AND product_id=? AND buyer_role_id=? AND status='approved'
		UNION ALL
		SELECT 1 FROM shipment_lines l JOIN shipment_slips s ON s.id=l.shipment_slip_id
			WHERE s.organization_id=? AND l.product_id=? AND s.buyer_role_id=? AND s.status IN ('draft','confirmed')
		UNION ALL
		SELECT 1 FROM sales_lines l JOIN sales_slips s ON s.id=l.sales_slip_id
			WHERE s.organization_id=? AND l.product_id=? AND s.buyer_role_id=? AND s.status IN ('draft','pending_approval','confirmed')
	) linked`, organizationID, productID, buyerRoleID, organizationID, productID, buyerRoleID,
		organizationID, productID, buyerRoleID).Scan(&count).Error
	return count > 0, err
}

func (r *Repository) CreateSale(ctx context.Context, input SaleCreateInput) (SaleSlipRecord, error) {
	date, err := time.Parse("2006-01-02", input.SaleDate)
	if err != nil {
		return SaleSlipRecord{}, err
	}
	if len(input.Lines) == 0 || len(input.Lines) > 100 {
		return SaleSlipRecord{}, ErrSaleState
	}
	var saleID string
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		buyerRoleID, err := lookupBuyerRole(tx, input.OrganizationID, input.BuyerCode)
		if err != nil {
			return err
		}
		rate, err := latestFX(tx, input.OrganizationID)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		sequence, err := nextDocumentSequence(tx, input.OrganizationID, "sale", date.Year(), now)
		if err != nil {
			return err
		}
		saleID, err = database.NewID("sal")
		if err != nil {
			return err
		}
		slipNumber := fmt.Sprintf("SI-%04d-%04d", date.Year(), sequence)
		if err := tx.Exec(`INSERT INTO sales_slips(
			id,organization_id,slip_number,buyer_role_id,sale_date,display_currency,tax_mode,
			tax_rate_basis_points,status,notes,created_by,created_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,'draft',?,?,?,?)`, saleID, input.OrganizationID, slipNumber, buyerRoleID,
			date, input.DisplayCurrency, input.TaxMode, input.TaxRateBasisPoints, strings.TrimSpace(input.Notes),
			input.ActorUserID, now, now).Error; err != nil {
			return err
		}
		seen := map[string]bool{}
		for index, line := range input.Lines {
			productCode := strings.ToUpper(strings.TrimSpace(line.ProductCode))
			if productCode == "" || seen[productCode] {
				return ErrProductConflict
			}
			seen[productCode] = true
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
			if result.RowsAffected == 0 || !map[string]bool{"in_stock": true, "reserved": true, "shipped": true}[product.InventoryStatus] {
				return ErrProductUnavailable
			}
			if product.InventoryStatus != "in_stock" {
				linked, err := productBelongsToBuyer(tx, input.OrganizationID, product.ID, buyerRoleID)
				if err != nil {
					return err
				}
				if !linked {
					return ErrProductConflict
				}
			}
			price := line.UnitPriceMinor
			if price == 0 {
				price, err = convertCurrency(product.BaseSalePriceMinor, product.BaseSaleCurrency, input.DisplayCurrency, rate)
				if err != nil {
					return err
				}
			}
			if price < 0 {
				return ErrSaleState
			}
			tax := int64(0)
			if input.TaxMode == "taxable" {
				tax = mulDivRound(price, int64(input.TaxRateBasisPoints), 10000)
			}
			total := price + tax
			convertedJPY, err := convertCurrency(total, input.DisplayCurrency, "JPY", rate)
			if err != nil {
				return err
			}
			lineID, _ := database.NewID("sln")
			if err := tx.Exec(`INSERT INTO sales_lines(
				id,sales_slip_id,line_number,product_id,quantity,unit_price_minor,sale_currency,subtotal_minor,
				tax_amount_minor,total_minor,converted_total_jpy,fx_rate_snapshot_id,fx_rate_scaled,fx_scale,created_at
			) VALUES(?,?,?,?,1,?,?,?,?,?,?,?,?,?,?)`, lineID, saleID, index+1, product.ID, price,
				input.DisplayCurrency, price, tax, total, convertedJPY, rate.ID, rate.RateScaled, rate.Scale, now).Error; err != nil {
				return err
			}
			if product.InventoryStatus == "in_stock" {
				if err := tx.Exec(`UPDATE products SET inventory_status='reserved',updated_at=? WHERE id=?`, now, product.ID).Error; err != nil {
					return err
				}
				eventID, _ := database.NewID("ive")
				if err := tx.Exec(`INSERT INTO inventory_events(
					id,organization_id,product_id,event_type,from_status,to_status,reason,actor_user_id,created_at
				) VALUES(?,?,?,'sale_draft_created','in_stock','reserved','売上伝票へ割当',?,?)`,
					eventID, input.OrganizationID, product.ID, input.ActorUserID, now).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return SaleSlipRecord{}, err
	}
	return r.SaleSlip(ctx, input.OrganizationID, saleID)
}

func (r *Repository) SaleSlips(ctx context.Context, organizationID string, limit int) ([]SaleSlipRecord, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	var records []SaleSlipRecord
	err := r.db.WithContext(ctx).Table("sales_slips AS s").
		Select(`s.id,s.slip_number,br.role_code AS buyer_code,bp.legal_name AS buyer_name,s.sale_date,
			s.display_currency,s.tax_mode,s.tax_rate_basis_points,s.status,s.notes,
			COALESCE(SUM(l.subtotal_minor),0) AS subtotal_minor,COALESCE(SUM(l.tax_amount_minor),0) AS tax_amount_minor,
			COALESCE(SUM(l.total_minor),0) AS total_minor,COALESCE(SUM(l.converted_total_jpy),0) AS converted_total_jpy,
			s.confirmed_at,s.created_at,s.updated_at`).
		Joins("JOIN partner_roles br ON br.id=s.buyer_role_id").Joins("JOIN business_partners bp ON bp.id=br.partner_id").
		Joins("LEFT JOIN sales_lines l ON l.sales_slip_id=s.id").Where("s.organization_id=?", organizationID).
		Group("s.id,br.role_code,bp.legal_name").Order("s.sale_date DESC,s.slip_number DESC").Limit(limit).Scan(&records).Error
	return records, err
}

func (r *Repository) SaleSlip(ctx context.Context, organizationID, saleID string) (SaleSlipRecord, error) {
	var record SaleSlipRecord
	result := r.db.WithContext(ctx).Table("sales_slips AS s").
		Select(`s.id,s.slip_number,br.role_code AS buyer_code,bp.legal_name AS buyer_name,s.sale_date,
			s.display_currency,s.tax_mode,s.tax_rate_basis_points,s.status,s.notes,
			COALESCE(SUM(l.subtotal_minor),0) AS subtotal_minor,COALESCE(SUM(l.tax_amount_minor),0) AS tax_amount_minor,
			COALESCE(SUM(l.total_minor),0) AS total_minor,COALESCE(SUM(l.converted_total_jpy),0) AS converted_total_jpy,
			s.confirmed_at,s.created_at,s.updated_at`).
		Joins("JOIN partner_roles br ON br.id=s.buyer_role_id").Joins("JOIN business_partners bp ON bp.id=br.partner_id").
		Joins("LEFT JOIN sales_lines l ON l.sales_slip_id=s.id").Where("s.organization_id=? AND s.id=?", organizationID, saleID).
		Group("s.id,br.role_code,bp.legal_name").Take(&record)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return SaleSlipRecord{}, ErrSaleNotFound
	}
	if result.Error != nil {
		return SaleSlipRecord{}, result.Error
	}
	if err := r.db.WithContext(ctx).Table("sales_lines AS l").
		Select(`l.id,l.line_number,l.product_id,p.product_code,p.brand,p.model_number,l.unit_price_minor,
			l.sale_currency,l.subtotal_minor,l.tax_amount_minor,l.total_minor,l.converted_total_jpy`).
		Joins("JOIN products p ON p.id=l.product_id").Where("l.sales_slip_id=?", saleID).
		Order("l.line_number").Scan(&record.Lines).Error; err != nil {
		return SaleSlipRecord{}, err
	}
	return record, nil
}

func (r *Repository) ConfirmSale(ctx context.Context, organizationID, saleID, actorUserID string) (SaleSlipRecord, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var slip struct{ Status, BuyerRoleID string }
		result := tx.Raw(`SELECT status,buyer_role_id FROM sales_slips WHERE organization_id=? AND id=? FOR UPDATE`, organizationID, saleID).Scan(&slip)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrSaleNotFound
		}
		if slip.Status == "confirmed" {
			return nil
		}
		if slip.Status != "draft" {
			return ErrSaleState
		}
		var productIDs []string
		if err := tx.Table("sales_lines").Where("sales_slip_id=?", saleID).Order("line_number").Pluck("product_id", &productIDs).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		for _, productID := range productIDs {
			var status string
			if err := tx.Raw(`SELECT inventory_status FROM products WHERE organization_id=? AND id=? FOR UPDATE`, organizationID, productID).Scan(&status).Error; err != nil {
				return err
			}
			if !map[string]bool{"in_stock": true, "reserved": true, "shipped": true}[status] {
				return ErrProductConflict
			}
			if status != "in_stock" {
				linked, err := productBelongsToBuyer(tx, organizationID, productID, slip.BuyerRoleID)
				if err != nil || !linked {
					return ErrProductConflict
				}
			}
			if err := tx.Exec(`UPDATE products SET inventory_status='sold',updated_at=? WHERE id=?`, now, productID).Error; err != nil {
				return err
			}
			eventID, _ := database.NewID("ive")
			if err := tx.Exec(`INSERT INTO inventory_events(
				id,organization_id,product_id,event_type,from_status,to_status,reason,actor_user_id,created_at
			) VALUES(?,?,?,'sale_confirmed',?,'sold','売上伝票確定',?,?)`, eventID, organizationID,
				productID, status, actorUserID, now).Error; err != nil {
				return err
			}
			var request struct{ ID, GuestAccountID string }
			_ = tx.Table("purchase_requests").Select("id,guest_account_id").Where(
				"organization_id=? AND product_id=? AND buyer_role_id=? AND status='approved'", organizationID, productID, slip.BuyerRoleID).Scan(&request).Error
			if request.ID != "" {
				if err := tx.Exec(`UPDATE purchase_requests SET status='sold',updated_at=? WHERE id=?`, now, request.ID).Error; err != nil {
					return err
				}
				var guestUserID string
				_ = tx.Table("guest_accounts").Select("user_id").Where("id=?", request.GuestAccountID).Scan(&guestUserID).Error
				if err := insertNotificationTx(tx, organizationID, guestUserID, "", "purchase_request.sold",
					"購入が確定しました", "売上伝票が確定しました。", "purchase_request", request.ID, now); err != nil {
					return err
				}
			}
		}
		return tx.Exec(`UPDATE sales_slips SET status='confirmed',confirmed_at=?,confirmed_by=?,updated_at=?
			WHERE organization_id=? AND id=?`, now, actorUserID, now, organizationID, saleID).Error
	})
	if err != nil {
		return SaleSlipRecord{}, err
	}
	return r.SaleSlip(ctx, organizationID, saleID)
}
