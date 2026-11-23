package persistence

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ProductCostMerge struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"-"`
	ManagementCode string     `json:"managementCode"`
	BusinessDate   DateString `json:"businessDate"`
	InputsJSON     string     `json:"-"`
	Status         string     `json:"status"`
	AdjustmentID   *string    `json:"adjustmentId,omitempty"`
	CreatedBy      string     `json:"-"`
	CreatedAt      time.Time  `json:"-"`
}

func (ProductCostMerge) TableName() string { return "product_cost_merges" }

// StartProductCostMerge reserves the new code on the start date and locks all
// source products in one transaction. No new inventory is created until confirmation.
func (r *Repository) StartProductCostMerge(ctx context.Context, org, actor string, ids []string) (ProductCostMerge, error) {
	result := ProductCostMerge{}
	if len(ids) < 2 || len(ids) > 20 {
		return result, ErrCostAdjustmentState
	}
	ids = append([]string(nil), ids...)
	sort.Strings(ids)
	for i, id := range ids {
		if strings.TrimSpace(id) == "" || (i > 0 && ids[i-1] == id) {
			return result, ErrCostAdjustmentState
		}
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		date, _ := time.Parse("2006-01-02", now.In(time.FixedZone("JST", 9*3600)).Format("2006-01-02"))
		inputs := make([]Product, 0, len(ids))
		for _, id := range ids {
			var p Product
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id=? AND id=? AND deleted_at IS NULL", org, id).Take(&p).Error; err != nil {
				return ErrProductUnavailable
			}
			if p.InventoryStatus != "in_stock" || p.InventoryValuationExcluded {
				return ErrCostAdjustmentState
			}
			_, cost, err := resolveCostAdjustmentSource(tx, org, p)
			if err != nil {
				return err
			}
			p.CostAmountMinor, p.CostCurrency = cost, "JPY"
			inputs = append(inputs, p)
		}
		seq, err := nextProductSequence(tx, org, date, now)
		if err != nil {
			return err
		}
		id, err := database.NewID("cmg")
		if err != nil {
			return err
		}
		data, err := json.Marshal(inputs)
		if err != nil {
			return err
		}
		result = ProductCostMerge{ID: id, OrganizationID: org, ManagementCode: formatProductCode(date, seq), BusinessDate: DateString(date.Format("2006-01-02")), InputsJSON: string(data), Status: "draft", CreatedBy: actor, CreatedAt: now}
		if err := tx.Create(&result).Error; err != nil {
			return err
		}
		for _, p := range inputs {
			if err := createCostAdjustmentStartSnapshot(tx, p, result.ID, "combine", actor, now); err != nil {
				return err
			}
			if err := tx.Exec("INSERT INTO product_cost_merge_inputs(merge_id,product_id) VALUES(?,?)", id, p.ID).Error; err != nil {
				return ErrCostAdjustmentExists
			}
			if err := tx.Model(&Product{}).Where("id=? AND organization_id=?", p.ID, org).Updates(costAdjustmentStartProductUpdates(p, now)).Error; err != nil {
				return err
			}
			eventID, err := database.NewID("ive")
			if err != nil {
				return err
			}
			if err := tx.Exec(`INSERT INTO inventory_events(id,organization_id,product_id,event_type,from_status,to_status,reason,actor_user_id,created_at)
    VALUES(?,?,?,'cost_merge_started','in_stock','cost_adjustment',?,?,?)`, eventID, org, p.ID, "原価合わせ開始: "+result.ManagementCode, actor, now).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return result, err
}

func (r *Repository) ConfirmProductCostMerge(ctx context.Context, org, actor, id, sku string, output CostAdjustmentOutputDraft) (CostAdjustmentConfirmResult, error) {
	result := CostAdjustmentConfirmResult{}
	if output.Kind != "product" || output.Product == nil || output.AllocatedCostJPYMinor < 0 {
		return result, ErrCostAdjustmentState
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var session ProductCostMerge
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND organization_id=?", id, org).Take(&session).Error; err != nil {
			return ErrProductUnavailable
		}
		if session.Status != "draft" {
			return ErrCostAdjustmentExists
		}
		var inputs []Product
		if err := json.Unmarshal([]byte(session.InputsJSON), &inputs); err != nil {
			return err
		}
		if len(inputs) < 2 {
			return ErrCostAdjustmentState
		}
		var total int64
		for _, p := range inputs {
			if p.CostAmountMinor < 0 || total > math.MaxInt64-p.CostAmountMinor {
				return ErrCostAllocation
			}
			total += p.CostAmountMinor
			var current Product
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND organization_id=? AND deleted_at IS NULL", p.ID, org).Take(&current).Error; err != nil {
				return ErrProductUnavailable
			}
			if current.InventoryStatus != "cost_adjustment" {
				return ErrCostAdjustmentState
			}
		}
		if total != output.AllocatedCostJPYMinor {
			return ErrCostAllocation
		}
		now := time.Now().UTC()
		date, err := time.Parse("2006-01-02", string(session.BusinessDate))
		if err != nil {
			return err
		}
		slipNumber, err := nextCostAdjustmentSlipNumber(tx, org, date, now, "combine")
		if err != nil {
			return err
		}
		adjustmentID, err := database.NewID("cad")
		if err != nil {
			return err
		}
		source := inputs[0]
		source.SKU = strings.TrimSpace(sku)
		output.Product.InternalComment = costMergeInternalComment(inputs, output.Product.InternalComment)
		sourceLine, _, err := resolveCostAdjustmentSource(tx, org, source)
		if err != nil {
			return err
		}
		slipID := sourceLine.PurchaseSlipID
		if err := tx.Exec(`INSERT INTO cost_adjustments(id,slip_number,organization_id,source_product_id,source_purchase_slip_id,source_purchase_slip_line_id,adjustment_type,adjustment_date,source_product_code,source_cost_jpy_minor,allocated_cost_jpy_minor,status,confirmed_at,confirmed_by,created_by,created_at,updated_at)
   VALUES(?,?,?,?,?,?,'combine',?,?,?,?,'confirmed',?,?,?,?,?)`, adjustmentID, slipNumber, org, source.ID, slipID, sourceLine.ID, date, source.ProductCode, total, total, now, actor, actor, now, now).Error; err != nil {
			return err
		}
		itemID, err := database.NewID("cai")
		if err != nil {
			return err
		}
		output.Position = 1
		created, err := r.createCostAdjustmentProduct(tx, source, adjustmentID, slipNumber, itemID, output, date, now, CostAdjustmentConfirmInput{OrganizationID: org, ActorUserID: actor, Mode: "combine", ReservedProductCode: session.ManagementCode})
		if err != nil {
			return err
		}
		for _, p := range inputs {
			if err := tx.Model(&Product{}).Where("id=? AND organization_id=?", p.ID, org).Updates(map[string]any{
				"notes": appendCostAdjustmentSlipNumber(p.Notes, slipNumber), "inventory_status": "cost_adjustment",
				"inventory_valuation_excluded": true, "inventory_valuation_exclusion_reason": "cost_merge_source",
				"inventory_valuation_excluded_at": now, "cost_adjustment_id": adjustmentID, "updated_at": now,
			}).Error; err != nil {
				return err
			}
			eventID, err := database.NewID("ive")
			if err != nil {
				return err
			}
			if err := tx.Exec(`INSERT INTO inventory_events(id,organization_id,product_id,event_type,from_status,to_status,reason,actor_user_id,created_at)
    VALUES(?,?,?,'cost_merge_confirmed','cost_adjustment','cost_adjustment',?,?,?)`, eventID, org, p.ID, fmt.Sprintf("%s 原価合わせ先: %s", slipNumber, session.ManagementCode), actor, now).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&session).Updates(map[string]any{"status": "confirmed", "adjustment_id": adjustmentID}).Error; err != nil {
			return err
		}
		if err := completeCostAdjustmentStartSnapshots(tx, org, session.ID, "", actor, "confirmed", now); err != nil {
			return err
		}
		result = CostAdjustmentConfirmResult{ID: adjustmentID, SlipNumber: slipNumber, AdjustmentDate: string(session.BusinessDate), SourceProductCode: source.ProductCode, SourceStatus: "cost_adjustment", Outputs: []CostAdjustmentOutputResult{created}}
		return nil
	})
	return result, err
}

func costMergeInternalComment(inputs []Product, value string) string {
	seen := map[string]bool{}
	lines := []string{}
	for _, input := range inputs {
		for _, raw := range strings.Split(input.InternalComment, "\n") {
			if number, ok := costAdjustmentSlipNumberFromLine(strings.TrimSpace(raw)); ok && !seen[number] {
				lines = append(lines, "調整伝票番号: "+number)
				seen[number] = true
			}
		}
	}
	for _, raw := range strings.Split(strings.TrimSpace(value), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if number, ok := costAdjustmentSlipNumberFromLine(line); ok {
			if !seen[number] {
				lines = append(lines, "調整伝票番号: "+number)
				seen[number] = true
			}
			continue
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
