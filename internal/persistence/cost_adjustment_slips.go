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

var (
	ErrCostAdjustmentSlipNotFound        = errors.New("cost adjustment slip not found")
	ErrCostAdjustmentSlipProductNotFound = errors.New("product is not on cost adjustment slip")
	ErrCostAdjustmentSlipScanState       = errors.New("product is not awaiting cost adjustment scan")
)

type CostAdjustmentScanResult struct {
	ProductCode     string `json:"productCode"`
	SlipCostJPY     int64  `json:"slipCostJpyMinor"`
	InventoryStatus string `json:"inventoryStatus"`
	Result          string `json:"result"`
}

// CostAdjustmentSlipLine is the immutable line snapshot shown on a cost
// adjustment slip. Direct adjustments have before/after values; split and
// merge adjustments have the allocated value of each generated item.
type CostAdjustmentSlipLine struct {
	Position         int    `json:"position"`
	Kind             string `json:"kind"`
	ManagementCode   string `json:"managementCode"`
	PreviousCostJPY  int64  `json:"previousCostJpyMinor"`
	AdjustmentJPY    int64  `json:"adjustmentAmountJpyMinor"`
	AdjustedCostJPY  int64  `json:"adjustedCostJpyMinor"`
	AllocatedCostJPY int64  `json:"allocatedCostJpyMinor"`
	Direction        string `json:"direction,omitempty"`
	Notes            string `json:"notes,omitempty"`
}

type CostAdjustmentSlip struct {
	ID                string                   `json:"id"`
	SlipNumber        string                   `json:"slipNumber"`
	AdjustmentType    string                   `json:"adjustmentType"`
	AdjustmentDate    DateString               `json:"adjustmentDate"`
	SourceProductCode string                   `json:"sourceProductCode"`
	SourceBrand       string                   `json:"sourceBrand"`
	SourceModel       string                   `json:"sourceModel"`
	SourceCostJPY     int64                    `json:"sourceCostJpyMinor"`
	AdjustedCostJPY   int64                    `json:"adjustedCostJpyMinor"`
	Status            string                   `json:"status"`
	Notes             string                   `json:"notes"`
	ConfirmedAt       *time.Time               `json:"confirmedAt,omitempty"`
	Lines             []CostAdjustmentSlipLine `json:"lines" gorm:"-"`
}

// ListCostAdjustmentSlips returns confirmed cost-management documents without
// mutating the original purchase slips that provide their audit provenance.
func (r *Repository) ListCostAdjustmentSlips(ctx context.Context, organizationID string, limit int) ([]CostAdjustmentSlip, error) {
	if limit < 1 || limit > 500 {
		limit = 200
	}
	records := []CostAdjustmentSlip{}
	if err := r.db.WithContext(ctx).Raw(`SELECT ca.id,ca.slip_number,ca.adjustment_type,ca.adjustment_date,
		ca.source_product_code,COALESCE(p.brand,'' ) AS source_brand,COALESCE(p.model_number,'') AS source_model,
		ca.source_cost_jpy_minor AS source_cost_jpy,ca.allocated_cost_jpy_minor AS adjusted_cost_jpy,
		ca.status,COALESCE(ca.notes,'') AS notes,ca.confirmed_at
		FROM cost_adjustments ca
		LEFT JOIN products p ON p.id=ca.source_product_id AND p.organization_id=ca.organization_id
		WHERE ca.organization_id=? AND ca.status='confirmed'
		ORDER BY ca.adjustment_date DESC,ca.confirmed_at DESC,ca.slip_number DESC LIMIT ?`, organizationID, limit).Scan(&records).Error; err != nil {
		return nil, err
	}
	for index := range records {
		record := &records[index]
		record.Lines = []CostAdjustmentSlipLine{}
		if record.AdjustmentType == "direct_increase" || record.AdjustmentType == "direct_decrease" {
			if err := r.db.WithContext(ctx).Raw(`SELECT position,'product' AS kind,product_code AS management_code,
				previous_cost_jpy_minor AS previous_cost_jpy,adjustment_amount_jpy_minor AS adjustment_jpy,
				adjusted_cost_jpy_minor AS adjusted_cost_jpy,adjusted_cost_jpy_minor AS allocated_cost_jpy,direction,notes
				FROM direct_cost_adjustment_lines WHERE cost_adjustment_id=? ORDER BY position`, record.ID).Scan(&record.Lines).Error; err != nil {
				return nil, err
			}
			continue
		}
		if err := r.db.WithContext(ctx).Raw(`SELECT position,item_kind AS kind,management_code,
			0 AS previous_cost_jpy,0 AS adjustment_jpy,allocated_cost_jpy_minor AS adjusted_cost_jpy,
			allocated_cost_jpy_minor AS allocated_cost_jpy,'' AS direction,'' AS notes
			FROM cost_adjustment_items WHERE cost_adjustment_id=? ORDER BY position`, record.ID).Scan(&record.Lines).Error; err != nil {
			return nil, err
		}
		// The original goods remain on the adjustment slip at zero, even though
		// their purchase-time value is retained in the purchase slip.
		if record.AdjustmentType == "breakdown" || record.AdjustmentType == "combine" {
			var originals []struct{ ProductCode string }
			if record.AdjustmentType == "combine" {
				if err := r.db.WithContext(ctx).Raw(`SELECT p.product_code FROM product_cost_merge_inputs mi
					JOIN product_cost_merges m ON m.id=mi.merge_id
					JOIN products p ON p.id=mi.product_id AND p.organization_id=m.organization_id
					WHERE m.organization_id=? AND m.adjustment_id=? ORDER BY p.product_code`, organizationID, record.ID).Scan(&originals).Error; err != nil {
					return nil, err
				}
			}
			if len(originals) == 0 {
				originals = append(originals, struct{ ProductCode string }{record.SourceProductCode})
			}
			for _, original := range originals {
				found := false
				for _, line := range record.Lines {
					if line.ManagementCode == original.ProductCode {
						found = true
						break
					}
				}
				if !found {
					record.Lines = append([]CostAdjustmentSlipLine{{Kind: "product", ManagementCode: original.ProductCode, Notes: "元商品"}}, record.Lines...)
				}
			}
			for i := range record.Lines {
				record.Lines[i].Position = i + 1
			}
		}
	}
	return records, nil
}

// ScanCostAdjustmentProduct settles exactly one product on a confirmed slip.
// Its slip-line amount, not the immutable purchase-line amount, determines the
// post-scan status. Repeated scans are safe and never revalue purchase records.
func (r *Repository) ScanCostAdjustmentProduct(ctx context.Context, organizationID, adjustmentID, productCode, actorUserID string) (CostAdjustmentScanResult, error) {
	result := CostAdjustmentScanResult{ProductCode: strings.TrimSpace(productCode)}
	if result.ProductCode == "" {
		return result, ErrCostAdjustmentSlipProductNotFound
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var slip struct{ ID, SlipNumber, AdjustmentType, SourceProductID string }
		if err := tx.Raw(`SELECT id,slip_number,adjustment_type,source_product_id FROM cost_adjustments
			WHERE id=? AND organization_id=? AND status='confirmed'`, adjustmentID, organizationID).Scan(&slip).Error; err != nil {
			return err
		}
		if slip.ID == "" {
			return ErrCostAdjustmentSlipNotFound
		}
		var product Product
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id=? AND product_code=? AND deleted_at IS NULL", organizationID, result.ProductCode).Take(&product).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCostAdjustmentSlipProductNotFound
			}
			return err
		}
		var line struct {
			Cost int64
			ID   string
		}
		// Direct lines take priority (their source_product_id is also present).
		if err := tx.Raw(`SELECT id,adjusted_cost_jpy_minor AS cost FROM direct_cost_adjustment_lines WHERE cost_adjustment_id=? AND product_id=?`, slip.ID, product.ID).Scan(&line).Error; err != nil {
			return err
		}
		if line.ID == "" {
			if err := tx.Raw(`SELECT id,allocated_cost_jpy_minor AS cost FROM cost_adjustment_items WHERE cost_adjustment_id=? AND output_product_id=? AND item_kind='product'`, slip.ID, product.ID).Scan(&line).Error; err != nil {
				return err
			}
		}
		if line.ID == "" && slip.AdjustmentType == "breakdown" && slip.SourceProductID == product.ID {
			line.ID = "source"
		}
		if line.ID == "" && slip.AdjustmentType == "combine" {
			var count int64
			if err := tx.Table("product_cost_merge_inputs AS mi").Joins("JOIN product_cost_merges m ON m.id=mi.merge_id").
				Where("m.organization_id=? AND m.adjustment_id=? AND mi.product_id=?", organizationID, slip.ID, product.ID).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				line.ID = "source"
			}
		}
		if line.ID == "" {
			return ErrCostAdjustmentSlipProductNotFound
		}
		result.SlipCostJPY = line.Cost
		target := "cost_adjusted"
		if line.Cost > 0 {
			target = "in_stock"
		}
		result.InventoryStatus = target
		if product.CostAdjustmentID != slip.ID {
			return ErrCostAdjustmentSlipScanState
		}
		if product.InventoryStatus == target {
			result.Result = "already_scanned"
			return nil
		}
		if product.InventoryStatus != "cost_adjustment" {
			return ErrCostAdjustmentSlipScanState
		}
		now := time.Now().UTC()
		updates := map[string]any{"inventory_status": target, "updated_at": now}
		if target == "cost_adjusted" {
			updates["inventory_valuation_excluded"] = true
			updates["inventory_valuation_exclusion_reason"] = "cost_adjustment_zero"
			updates["inventory_valuation_excluded_at"] = now
		} else {
			updates["inventory_valuation_excluded"] = false
			updates["inventory_valuation_exclusion_reason"] = ""
			updates["inventory_valuation_excluded_at"] = nil
		}
		if err := tx.Model(&Product{}).Where("id=? AND organization_id=? AND inventory_status='cost_adjustment'", product.ID, organizationID).Updates(updates).Error; err != nil {
			return err
		}
		if line.ID != "source" && slip.AdjustmentType != "direct_increase" && slip.AdjustmentType != "direct_decrease" {
			if err := tx.Table("cost_adjustment_items").Where("id=? AND cost_adjustment_id=?", line.ID, slip.ID).Update("status", "completed").Error; err != nil {
				return err
			}
		}
		eventID, err := database.NewID("ive")
		if err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO inventory_events(id,organization_id,product_id,event_type,from_status,to_status,reason,actor_user_id,created_at)
			VALUES(?,?,?,'cost_adjustment_slip_scan','cost_adjustment',?,?,?,?)`, eventID, organizationID, product.ID, target,
			fmt.Sprintf("%s のQR読取（伝票上原価 ¥%d）", slip.SlipNumber, line.Cost), actorUserID, now).Error; err != nil {
			return err
		}
		result.Result = "scanned"
		return nil
	})
	return result, err
}
