package persistence

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type DirectCostAdjustmentItemInput struct {
	ProductID           string `json:"productId"`
	AdjustmentAmountJPY int64  `json:"adjustmentAmountJpyMinor"`
}

type DirectCostAdjustmentInput struct {
	OrganizationID string                          `json:"-"`
	ActorUserID    string                          `json:"-"`
	Direction      string                          `json:"direction"`
	Notes          string                          `json:"notes"`
	Items          []DirectCostAdjustmentItemInput `json:"items"`
}

type DirectCostAdjustmentLineResult struct {
	ProductID       string `json:"productId"`
	ProductCode     string `json:"productCode"`
	PreviousCostJPY int64  `json:"previousCostJpyMinor"`
	AdjustmentJPY   int64  `json:"adjustmentAmountJpyMinor"`
	AdjustedCostJPY int64  `json:"adjustedCostJpyMinor"`
}

type DirectCostAdjustmentResult struct {
	ID             string                           `json:"id"`
	SlipNumber     string                           `json:"slipNumber"`
	AdjustmentDate string                           `json:"adjustmentDate"`
	Direction      string                           `json:"direction"`
	Items          []DirectCostAdjustmentLineResult `json:"items"`
}

func (r *Repository) ConfirmDirectCostAdjustment(ctx context.Context, input DirectCostAdjustmentInput) (DirectCostAdjustmentResult, error) {
	result := DirectCostAdjustmentResult{}
	if input.Direction != "increase" && input.Direction != "decrease" {
		return result, ErrCostAdjustmentState
	}
	if len(input.Items) != 1 {
		return result, ErrCostAdjustmentState
	}
	reason := strings.TrimSpace(input.Notes)
	if reason == "" {
		return result, ErrCostAdjustmentState
	}
	seen := map[string]bool{}
	for index := range input.Items {
		item := &input.Items[index]
		item.ProductID = strings.TrimSpace(item.ProductID)
		if item.ProductID == "" || item.AdjustmentAmountJPY <= 0 || seen[item.ProductID] {
			return result, ErrCostAdjustmentState
		}
		seen[item.ProductID] = true
	}

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		jst := time.FixedZone("JST", 9*60*60)
		date, _ := time.Parse("2006-01-02", now.In(jst).Format("2006-01-02"))
		products := make([]Product, 0, len(input.Items))
		var primarySource costAdjustmentSourceLine
		var originalTotal, adjustedTotal int64
		for index, item := range input.Items {
			var product Product
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(
				"organization_id=? AND id=? AND deleted_at IS NULL", input.OrganizationID, item.ProductID,
			).Take(&product).Error; err != nil {
				return ErrProductUnavailable
			}
			if product.InventoryStatus != "cost_adjustment" || product.CostAdjustmentStartedAt == nil || product.InventoryValuationExcluded {
				return ErrCostAdjustmentState
			}
			sourceLine, cost, err := resolveCostAdjustmentSource(tx, input.OrganizationID, product)
			if err != nil {
				return err
			}
			if index == 0 {
				primarySource = sourceLine
			}
			adjusted := cost
			if input.Direction == "increase" {
				if cost > math.MaxInt64-item.AdjustmentAmountJPY {
					return ErrCostAllocation
				}
				adjusted += item.AdjustmentAmountJPY
			} else {
				if item.AdjustmentAmountJPY > cost {
					return ErrCostAllocation
				}
				adjusted -= item.AdjustmentAmountJPY
			}
			if originalTotal > math.MaxInt64-cost || adjustedTotal > math.MaxInt64-adjusted {
				return ErrCostAllocation
			}
			originalTotal += cost
			adjustedTotal += adjusted
			product.CostAmountMinor = cost
			products = append(products, product)
		}

		sourceSlipID := primarySource.PurchaseSlipID
		slipNumber, err := nextCostAdjustmentSlipNumber(tx, input.OrganizationID, date, now, "direct")
		if err != nil {
			return err
		}
		adjustmentID, err := database.NewID("cad")
		if err != nil {
			return err
		}
		adjustmentType := "direct_" + input.Direction
		directionLabel := map[string]string{"increase": "増額", "decrease": "減額"}[input.Direction]
		sign := map[string]string{"increase": "+", "decrease": "-"}[input.Direction]
		note := fmt.Sprintf("%s｜理由：%s｜金額：%s¥%d", directionLabel, reason, sign, input.Items[0].AdjustmentAmountJPY)
		if err := tx.Exec(`INSERT INTO cost_adjustments(
			id,slip_number,organization_id,source_product_id,source_purchase_slip_id,source_purchase_slip_line_id,
			adjustment_type,adjustment_date,source_product_code,source_cost_jpy_minor,allocated_cost_jpy_minor,
			status,confirmed_at,confirmed_by,created_by,created_at,updated_at,notes
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,'confirmed',?,?,?,?,?,?)`,
			adjustmentID, slipNumber, input.OrganizationID, products[0].ID, sourceSlipID, primarySource.ID,
			adjustmentType, date, products[0].ProductCode, originalTotal, adjustedTotal,
			now, input.ActorUserID, input.ActorUserID, now, now, note).Error; err != nil {
			return err
		}

		result = DirectCostAdjustmentResult{ID: adjustmentID, SlipNumber: slipNumber, AdjustmentDate: date.Format("2006-01-02"), Direction: input.Direction, Items: make([]DirectCostAdjustmentLineResult, 0, len(products))}
		for index, product := range products {
			item := input.Items[index]
			lineNote := fmt.Sprintf("%s｜理由：%s｜金額：%s¥%d", directionLabel, reason, sign, item.AdjustmentAmountJPY)
			adjusted := product.CostAmountMinor
			if input.Direction == "increase" {
				adjusted += item.AdjustmentAmountJPY
			} else {
				adjusted -= item.AdjustmentAmountJPY
			}
			lineID, err := database.NewID("dcl")
			if err != nil {
				return err
			}
			if err := tx.Exec(`INSERT INTO direct_cost_adjustment_lines(
				id,cost_adjustment_id,position,product_id,product_code,previous_cost_jpy_minor,
				adjustment_amount_jpy_minor,adjusted_cost_jpy_minor,direction,notes,created_at
			) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, lineID, adjustmentID, index+1, product.ID, product.ProductCode,
				product.CostAmountMinor, item.AdjustmentAmountJPY, adjusted, input.Direction, lineNote, now).Error; err != nil {
				return err
			}
			if err := tx.Model(&Product{}).Where("organization_id=? AND id=?", input.OrganizationID, product.ID).Updates(map[string]any{
				"cost_amount_minor": adjusted, "cost_currency": "JPY", "inventory_status": "cost_adjustment",
				"cost_adjustment_id": adjustmentID, "cost_adjustment_started_at": nil,
				"notes": appendCostAdjustmentSlipNumber(product.Notes, slipNumber), "updated_at": now,
			}).Error; err != nil {
				return err
			}
			eventID, _ := database.NewID("ive")
			if err := tx.Exec(`INSERT INTO inventory_events(id,organization_id,product_id,event_type,from_status,to_status,reason,actor_user_id,created_at)
				VALUES(?,?,?,'direct_cost_adjustment_confirmed',?,'cost_adjustment',?,?,?)`, eventID, input.OrganizationID, product.ID, product.InventoryStatus,
				fmt.Sprintf("%s %s ¥%d", slipNumber, directionLabel, item.AdjustmentAmountJPY), input.ActorUserID, now).Error; err != nil {
				return err
			}
			result.Items = append(result.Items, DirectCostAdjustmentLineResult{ProductID: product.ID, ProductCode: product.ProductCode,
				PreviousCostJPY: product.CostAmountMinor, AdjustmentJPY: item.AdjustmentAmountJPY, AdjustedCostJPY: adjusted})
		}
		if err := completeCostAdjustmentStartSnapshots(tx, input.OrganizationID, "", products[0].ID, input.ActorUserID, "confirmed", now); err != nil {
			return err
		}
		return nil
	})
	return result, err
}
