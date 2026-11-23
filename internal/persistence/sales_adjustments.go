package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrSalesAdjustmentInvalid = errors.New("invalid sales adjustment")
var ErrSalePaymentRequired = errors.New("sale payment confirmation required")

type SalesAdjustmentLineInput struct {
	Direction             string `json:"direction"`
	SaleLineID            string `json:"saleLineId"`
	AdjustmentAmountMinor int64  `json:"adjustmentAmountMinor"`
	Reason                string `json:"reason"`
}

type SalesAdjustmentInput struct {
	OrganizationID string                     `json:"-"`
	ActorUserID    string                     `json:"-"`
	AdjustmentDate string                     `json:"adjustmentDate"`
	Direction      string                     `json:"direction"`
	Reason         string                     `json:"reason"`
	Lines          []SalesAdjustmentLineInput `json:"lines"`
}

type SalesAdjustmentLineRecord struct {
	Direction              string `json:"direction"`
	ID                     string `json:"id"`
	LineNumber             int    `json:"lineNumber"`
	SourceSaleLineID       string `json:"sourceSaleLineId"`
	ProductID              string `json:"productId"`
	ProductCode            string `json:"productCode"`
	Brand                  string `json:"brand"`
	ModelNumber            string `json:"modelNumber"`
	AdjustmentAmountMinor  int64  `json:"adjustmentAmountMinor"`
	ConvertedAdjustmentJPY int64  `json:"convertedAdjustmentJpy"`
	Reason                 string `json:"reason"`
	BeforeSalePriceMinor   int64  `json:"beforeSalePriceMinor"`
	AfterSalePriceMinor    int64  `json:"afterSalePriceMinor"`
}

type SalesAdjustmentRecord struct {
	ID                 string                      `json:"id"`
	SlipNumber         string                      `json:"slipNumber"`
	DocumentType       string                      `gorm:"-" json:"documentType"`
	SourceSaleID       string                      `json:"sourceSaleId"`
	SourceSlipNumber   string                      `json:"sourceSlipNumber"`
	BuyerCode          string                      `json:"buyerCode"`
	BuyerName          string                      `json:"buyerName"`
	AdjustmentDate     DateString                  `json:"saleDate"`
	Direction          string                      `json:"adjustmentDirection"`
	DisplayCurrency    string                      `json:"displayCurrency"`
	Reason             string                      `json:"notes"`
	Status             string                      `json:"status"`
	TotalMinor         int64                       `json:"totalMinor"`
	SubtotalMinor      int64                       `gorm:"-" json:"subtotalMinor"`
	TaxAmountMinor     int64                       `gorm:"-" json:"taxAmountMinor"`
	TaxMode            string                      `json:"taxMode"`
	TaxRateBasisPoints int                         `json:"taxRateBasisPoints"`
	ConvertedTotalJPY  int64                       `json:"convertedTotalJpy"`
	FXRateScaled       int64                       `json:"fxRateScaled"`
	FXScale            int64                       `json:"fxScale"`
	CreatedAt          time.Time                   `json:"createdAt"`
	Lines              []SalesAdjustmentLineRecord `gorm:"-" json:"lines,omitempty"`
}

func salesAdjustmentLineReasons(input SalesAdjustmentInput) ([]string, error) {
	commonReason := strings.TrimSpace(input.Reason)
	if (input.Direction != "" && input.Direction != "mixed" && input.Direction != "increase" && input.Direction != "decrease") || len([]rune(commonReason)) > 1000 || len(input.Lines) == 0 || len(input.Lines) > 100 {
		return nil, ErrSalesAdjustmentInvalid
	}
	reasons := make([]string, len(input.Lines))
	seen := make(map[string]bool, len(input.Lines))
	for index, line := range input.Lines {
		direction := line.Direction
		if direction == "" {
			direction = input.Direction
		}
		if direction != "increase" && direction != "decrease" {
			return nil, ErrSalesAdjustmentInvalid
		}
		reason := strings.TrimSpace(line.Reason)
		// Preserve clients that still submit one common reason for the voucher.
		if reason == "" {
			reason = commonReason
		}
		if strings.TrimSpace(line.SaleLineID) == "" || seen[line.SaleLineID] || line.AdjustmentAmountMinor <= 0 || reason == "" || len([]rune(reason)) > 1000 {
			return nil, ErrSalesAdjustmentInvalid
		}
		seen[line.SaleLineID] = true
		reasons[index] = reason
	}
	return reasons, nil
}

func salesAdjustmentNotes(input SalesAdjustmentInput, reasons, productCodes []string) string {
	perLineReasons := false
	for _, line := range input.Lines {
		perLineReasons = perLineReasons || strings.TrimSpace(line.Reason) != ""
	}
	if !perLineReasons {
		return strings.TrimSpace(input.Reason)
	}
	notes := make([]string, len(reasons))
	for index, reason := range reasons {
		notes[index] = "商品管理番号 " + productCodes[index] + " 修正理由: " + reason
	}
	return strings.Join(notes, "\n")
}

func appendSalesAdjustmentComment(comment, slipNumber, direction, currency string, amount int64, reason string) string {
	label := "増額"
	if direction == "decrease" {
		label = "減額"
	}
	if comment != "" && !strings.HasSuffix(comment, "\n") {
		comment += "\n"
	}
	return comment + fmt.Sprintf("売上修正伝票 %s %s %s %d 修正理由: %s", slipNumber, label, currency, amount, reason)
}

func salesAdjustmentDate(value string, now time.Time) (time.Time, error) {
	// Legacy clients do not send the button-click date. Use the Japanese business date, not UTC.
	if value == "" {
		value = now.In(time.FixedZone("JST", 9*60*60)).Format("2006-01-02")
	}
	date, err := time.Parse("2006-01-02", value)
	if err != nil || date.Format("2006-01-02") != value {
		return time.Time{}, ErrSalesAdjustmentInvalid
	}
	return date, nil
}

func (r *Repository) CreateSalesAdjustment(ctx context.Context, sourceSaleID string, input SalesAdjustmentInput) (SalesAdjustmentRecord, error) {
	adjustmentDate, err := salesAdjustmentDate(input.AdjustmentDate, time.Now())
	if err != nil {
		return SalesAdjustmentRecord{}, err
	}
	reasons, err := salesAdjustmentLineReasons(input)
	if err != nil {
		return SalesAdjustmentRecord{}, err
	}
	var adjustmentID string
	// Each line owns its direction; the header only summarizes the selection.
	commonDirection := ""
	for index := range input.Lines {
		if input.Lines[index].Direction == "" {
			input.Lines[index].Direction = input.Direction
		}
		if index == 0 {
			commonDirection = input.Lines[index].Direction
		} else if commonDirection != input.Lines[index].Direction {
			commonDirection = "mixed"
		}
	}
	input.Direction = commonDirection
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var sale struct {
			ID, DisplayCurrency, Status string
			PaidAt                      *time.Time
			SaleDate                    time.Time
		}
		result := tx.Table("sales_slips").Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("paid_at,id,display_currency,status,sale_date").Where("organization_id=? AND id=?", input.OrganizationID, sourceSaleID).Take(&sale)
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return ErrSaleNotFound
		}
		if result.Error != nil || sale.Status != "confirmed" {
			return ErrSalesAdjustmentInvalid
		}
		if sale.PaidAt == nil {
			return ErrSalePaymentRequired
		}
		var sourceLines []struct {
			ID, ProductID, ProductCode, SaleCurrency string
			UnitPriceMinor, FXRateScaled, FXScale    int64
		}
		if err := tx.Table("sales_lines AS l").Select("l.id,l.product_id,p.product_code,l.sale_currency,l.unit_price_minor,l.fx_rate_scaled,l.fx_scale").
			Joins("JOIN products p ON p.id=l.product_id AND p.organization_id=?", input.OrganizationID).
			Where("l.sales_slip_id=?", sourceSaleID).Order("l.line_number").Find(&sourceLines).Error; err != nil {
			return err
		}
		byID := make(map[string]struct {
			ProductID, ProductCode, SaleCurrency  string
			UnitPriceMinor, FXRateScaled, FXScale int64
		}, len(sourceLines))
		for _, line := range sourceLines {
			byID[line.ID] = struct {
				ProductID, ProductCode, SaleCurrency  string
				UnitPriceMinor, FXRateScaled, FXScale int64
			}{line.ProductID, line.ProductCode, line.SaleCurrency, line.UnitPriceMinor, line.FXRateScaled, line.FXScale}
		}
		productCodes := make([]string, len(input.Lines))
		for index, requested := range input.Lines {
			source, ok := byID[requested.SaleLineID]
			if !ok || source.SaleCurrency != sale.DisplayCurrency || source.FXRateScaled <= 0 || source.FXScale <= 0 {
				return ErrSalesAdjustmentInvalid
			}
			productCodes[index] = source.ProductCode
		}
		reason := salesAdjustmentNotes(input, reasons, productCodes)
		now := time.Now().UTC()
		slipNumber, err := nextDailyDocumentNumber(tx, input.OrganizationID, "SS", adjustmentDate, now)
		if err != nil {
			return err
		}
		adjustmentID, err = database.NewID("saj")
		if err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO sales_adjustment_slips(id,organization_id,slip_number,source_sales_slip_id,adjustment_date,direction,display_currency,reason,status,created_by,created_at,updated_at)
			VALUES(?,?,?,?,?,?,?,?,'confirmed',?,?,?)`, adjustmentID, input.OrganizationID, slipNumber, sourceSaleID, adjustmentDate, input.Direction, sale.DisplayCurrency, reason, input.ActorUserID, now, now).Error; err != nil {
			return err
		}
		seen := map[string]bool{}
		for index, requested := range input.Lines {
			source, ok := byID[requested.SaleLineID]
			if !ok || seen[requested.SaleLineID] || requested.AdjustmentAmountMinor <= 0 || source.SaleCurrency != sale.DisplayCurrency || source.FXRateScaled <= 0 || source.FXScale <= 0 {
				return ErrSalesAdjustmentInvalid
			}
			seen[requested.SaleLineID] = true
			var priorSigned int64
			if err := tx.Table("sales_adjustment_lines AS al").
				Select("COALESCE(SUM(CASE WHEN COALESCE(NULLIF(al.direction,''),a.direction)='decrease' THEN -al.adjustment_amount_minor ELSE al.adjustment_amount_minor END),0)").
				Joins("JOIN sales_adjustment_slips a ON a.id=al.sales_adjustment_slip_id").
				Where("al.source_sales_line_id=? AND a.status='confirmed' AND a.id<>?", requested.SaleLineID, adjustmentID).
				Scan(&priorSigned).Error; err != nil {
				return err
			}
			beforePrice := source.UnitPriceMinor + priorSigned
			afterPrice := beforePrice + requested.AdjustmentAmountMinor
			if requested.Direction == "decrease" {
				if requested.AdjustmentAmountMinor > beforePrice {
					return ErrSalesAdjustmentInvalid
				}
				afterPrice = beforePrice - requested.AdjustmentAmountMinor
			}
			if afterPrice < 0 {
				return ErrSalesAdjustmentInvalid
			}
			converted, err := convertCurrency(requested.AdjustmentAmountMinor, source.SaleCurrency, "JPY", fxSnapshot{RateScaled: source.FXRateScaled, Scale: source.FXScale})
			if err != nil {
				return err
			}
			lineID, err := database.NewID("saladj")
			if err != nil {
				return err
			}
			if err := tx.Exec(`INSERT INTO sales_adjustment_lines(id,sales_adjustment_slip_id,source_sales_line_id,product_id,line_number,adjustment_amount_minor,converted_adjustment_jpy,reason,created_at,before_sale_price_minor,after_sale_price_minor,direction)
				VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, lineID, adjustmentID, requested.SaleLineID, source.ProductID, index+1, requested.AdjustmentAmountMinor, converted, reasons[index], now, beforePrice, afterPrice, requested.Direction).Error; err != nil {
				return err
			}
			var product struct{ ID, InternalComment string }
			if err := tx.Table("products").Clauses(clause.Locking{Strength: "UPDATE"}).Select("id,internal_comment").
				Where("organization_id=? AND id=?", input.OrganizationID, source.ProductID).Take(&product).Error; err != nil {
				return err
			}
			comment := appendSalesAdjustmentComment(product.InternalComment, slipNumber, requested.Direction, sale.DisplayCurrency, requested.AdjustmentAmountMinor, reasons[index])
			if err := tx.Table("products").Where("organization_id=? AND id=?", input.OrganizationID, source.ProductID).
				Updates(map[string]any{"internal_comment": comment, "updated_at": now}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return SalesAdjustmentRecord{}, err
	}
	return r.SalesAdjustment(ctx, input.OrganizationID, adjustmentID)
}

func (r *Repository) SalesAdjustments(ctx context.Context, organizationID string, limit int) ([]SalesAdjustmentRecord, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	var records []SalesAdjustmentRecord
	err := r.db.WithContext(ctx).Table("sales_adjustment_slips AS a").Select(`a.id,a.slip_number,a.source_sales_slip_id AS source_sale_id,s.slip_number AS source_slip_number,
		br.role_code AS buyer_code,bp.legal_name AS buyer_name,a.adjustment_date,a.direction,a.display_currency,a.reason,a.status,s.tax_mode,s.tax_rate_basis_points,
		COALESCE(SUM(CASE WHEN a.direction='mixed' AND l.direction='decrease' THEN -l.adjustment_amount_minor ELSE l.adjustment_amount_minor END),0) AS total_minor,
		COALESCE(SUM(CASE WHEN a.direction='mixed' AND l.direction='decrease' THEN -l.converted_adjustment_jpy ELSE l.converted_adjustment_jpy END),0) AS converted_total_jpy,
		COALESCE(MAX(sl.fx_rate_scaled),0) AS fx_rate_scaled,COALESCE(MAX(sl.fx_scale),0) AS fx_scale,a.created_at`).
		Joins("JOIN sales_slips s ON s.id=a.source_sales_slip_id").Joins("JOIN partner_roles br ON br.id=s.buyer_role_id").Joins("JOIN business_partners bp ON bp.id=br.partner_id").
		Joins("LEFT JOIN sales_adjustment_lines l ON l.sales_adjustment_slip_id=a.id").Joins("LEFT JOIN sales_lines sl ON sl.id=l.source_sales_line_id").
		Where("a.organization_id=?", organizationID).Group("a.id,s.id,br.role_code,bp.legal_name").Order("a.adjustment_date DESC,a.slip_number DESC").Limit(limit).Scan(&records).Error
	for index := range records {
		records[index].DocumentType = "sales_revision"
		records[index].SubtotalMinor = records[index].TotalMinor
	}
	if err != nil {
		return records, err
	}
	// Calculate display totals from the source voucher's tax treatment, without
	// modifying the saved pre-tax adjustments or product sales prices.
	byID := make(map[string]*SalesAdjustmentRecord)
	ids := make([]string, 0)
	for index := range records {
		if records[index].TaxMode == "taxable" && records[index].TaxRateBasisPoints > 0 {
			byID[records[index].ID] = &records[index]
			ids = append(ids, records[index].ID)
		}
	}
	if len(ids) > 0 {
		var lines []struct {
			SlipID, Direction, SaleCurrency string
			Amount, FXRateScaled, FXScale   int64
		}
		if err := r.db.WithContext(ctx).Table("sales_adjustment_lines AS l").
			Select("l.sales_adjustment_slip_id AS slip_id,l.direction,l.adjustment_amount_minor AS amount,sl.sale_currency,sl.fx_rate_scaled,sl.fx_scale").
			Joins("JOIN sales_lines sl ON sl.id=l.source_sales_line_id").Where("l.sales_adjustment_slip_id IN ?", ids).Scan(&lines).Error; err != nil {
			return nil, err
		}
		for _, line := range lines {
			record := byID[line.SlipID]
			tax := mulDivRound(line.Amount, int64(record.TaxRateBasisPoints), 10000)
			converted, err := convertCurrency(tax, line.SaleCurrency, "JPY", fxSnapshot{RateScaled: line.FXRateScaled, Scale: line.FXScale})
			if err != nil {
				return nil, err
			}
			if record.Direction == "mixed" && line.Direction == "decrease" {
				tax, converted = -tax, -converted
			}
			record.TaxAmountMinor += tax
			record.TotalMinor += tax
			record.ConvertedTotalJPY += converted
		}
	}
	return records, nil
}

func (r *Repository) SalesAdjustment(ctx context.Context, organizationID, id string) (SalesAdjustmentRecord, error) {
	records, err := r.SalesAdjustments(ctx, organizationID, 500)
	if err != nil {
		return SalesAdjustmentRecord{}, err
	}
	var record SalesAdjustmentRecord
	for _, candidate := range records {
		if candidate.ID == id {
			record = candidate
			break
		}
	}
	if record.ID == "" {
		return SalesAdjustmentRecord{}, ErrSaleNotFound
	}
	if err := r.db.WithContext(ctx).Table("sales_adjustment_lines AS l").Select(`l.id,l.line_number,l.source_sales_line_id AS source_sale_line_id,l.product_id,p.product_code,p.brand,p.model_number,l.adjustment_amount_minor,l.converted_adjustment_jpy,COALESCE(NULLIF(l.reason,''),a.reason) AS reason,l.before_sale_price_minor,l.after_sale_price_minor,COALESCE(NULLIF(l.direction,''),a.direction) AS direction`).
		Joins("JOIN products p ON p.id=l.product_id").Joins("JOIN sales_adjustment_slips a ON a.id=l.sales_adjustment_slip_id").
		Where("l.sales_adjustment_slip_id=? AND a.organization_id=?", id, organizationID).Order("l.line_number").Scan(&record.Lines).Error; err != nil {
		return SalesAdjustmentRecord{}, err
	}
	return record, nil
}
