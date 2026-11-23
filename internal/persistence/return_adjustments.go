package persistence

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"
)

const ReturnAdjustmentCode = "99999"

type ReturnAdjustmentLine struct {
	Name        string `json:"name"`
	AmountMinor int64  `json:"amountMinor"`
}

// Stored independently of return_lines: no product, stock, cost or event links.
type ReturnAdjustmentDetails struct {
	SourceSaleSlipNumber string                 `json:"sourceSaleSlipNumber,omitempty"`
	Currency             string                 `json:"currency"`
	TaxMode              string                 `json:"taxMode"`
	TaxRateBasisPoints   int                    `json:"taxRateBasisPoints"`
	FXRateScaled         int64                  `json:"fxRateScaled"`
	FXScale              int64                  `json:"fxScale"`
	Lines                []ReturnAdjustmentLine `json:"lines"`
}

func validReturnAdjustments(input ReturnCreateInput) bool {
	count := len(input.ProductCodes) + len(input.AdjustmentLines)
	if count < 1 || count > 100 || (input.OperationType == "takeout" && len(input.AdjustmentLines) > 0) {
		return false
	}
	for _, code := range input.ProductCodes {
		if strings.TrimSpace(code) == ReturnAdjustmentCode {
			return false
		}
	}
	for _, line := range input.AdjustmentLines {
		if strings.TrimSpace(line.Name) == "" || utf8.RuneCountInString(line.Name) > 200 || line.AmountMinor == 0 || line.AmountMinor < -1000000000000 || line.AmountMinor > 1000000000000 {
			return false
		}
	}
	return true
}

func returnAdjustmentSnapshot(tx *gorm.DB, input ReturnCreateInput) (string, error) {
	if len(input.AdjustmentLines) == 0 {
		return "{}", nil
	}
	details := ReturnAdjustmentDetails{Currency: "JPY", TaxMode: "out_of_scope", FXRateScaled: 1, FXScale: 1}
	if input.OperationType == "purchase_return" {
		var source struct {
			Currency, TaxCategory string
			TaxRateBasisPoints    int
			FXRateScaled, FXScale int64
		}
		result := tx.Raw(`SELECT l.cost_currency AS currency,p.tax_category,p.tax_rate_basis_points,
   COALESCE(l.fx_rate_scaled,1) AS fx_rate_scaled,COALESCE(l.fx_scale,1) AS fx_scale
   FROM purchase_slips p JOIN partner_roles r ON r.id=p.supplier_role_id
   JOIN purchase_slip_lines l ON l.purchase_slip_id=p.id
   WHERE p.organization_id=? AND p.slip_number=? AND r.role_code=? AND p.status<>'cancelled'
   ORDER BY l.line_number LIMIT 1`, input.OrganizationID, input.PurchaseSlipNo, input.SupplierCode).Scan(&source)
		if result.Error != nil {
			return "", result.Error
		}
		if result.RowsAffected != 1 {
			return "", ErrReturnState
		}
		details.Currency, details.TaxRateBasisPoints = source.Currency, source.TaxRateBasisPoints
		details.FXRateScaled, details.FXScale = source.FXRateScaled, source.FXScale
		if source.TaxCategory == "consumption_tax" {
			details.TaxMode = "standard"
		}
	} else {
		var source struct {
			Currency, TaxMode     string
			TaxRateBasisPoints    int
			FXRateScaled, FXScale int64
		}
		result := tx.Raw(`SELECT s.display_currency AS currency,s.tax_mode,s.tax_rate_basis_points,s.fx_rate_scaled,s.fx_scale
   FROM sales_slips s JOIN partner_roles r ON r.id=s.buyer_role_id
   WHERE s.organization_id=? AND s.slip_number=? AND r.role_code=? AND s.status='confirmed'`, input.OrganizationID, strings.TrimSpace(input.SourceSaleSlipNumber), input.BuyerCode).Scan(&source)
		if result.Error != nil {
			return "", result.Error
		}
		if result.RowsAffected != 1 {
			return "", ErrReturnState
		}
		details.SourceSaleSlipNumber = strings.TrimSpace(input.SourceSaleSlipNumber)
		details.Currency, details.TaxRateBasisPoints = source.Currency, source.TaxRateBasisPoints
		details.FXRateScaled, details.FXScale = source.FXRateScaled, source.FXScale
		if source.TaxMode == "taxable" {
			details.TaxMode = "standard"
		} else if source.TaxMode == "tax_exempt" || source.TaxMode == "exempt" {
			details.TaxMode = "exempt"
		}
	}
	if details.Currency == "JPY" {
		details.FXRateScaled, details.FXScale = 1, 1
	}
	if details.FXRateScaled <= 0 || details.FXScale <= 0 {
		return "", ErrReturnState
	}
	for _, line := range input.AdjustmentLines {
		line.Name = strings.TrimSpace(line.Name)
		details.Lines = append(details.Lines, line)
	}
	encoded, err := json.Marshal(details)
	return string(encoded), err
}
