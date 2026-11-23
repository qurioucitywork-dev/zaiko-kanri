package persistence

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrCostAdjustmentResetUnavailable = errors.New("cost adjustment start cannot be reset")

type costAdjustmentStartSnapshot struct {
	ID                              string
	OrganizationID                  string
	OperationID                     string
	Mode                            string
	ProductID                       string
	PreviousInventoryStatus         string
	PreviousBaseSalePriceMinor      int64
	PreviousBaseSaleCurrency        string
	PreviousInternalComment         string
	PreviousCostAdjustmentStartedAt *time.Time
	Status                          string
	CreatedBy                       string
	CreatedAt                       time.Time
}

func (costAdjustmentStartSnapshot) TableName() string { return "cost_adjustment_start_snapshots" }

func createCostAdjustmentStartSnapshot(tx *gorm.DB, current Product, operationID, mode, actor string, now time.Time) error {
	id, err := database.NewID("cas")
	if err != nil {
		return err
	}
	if strings.TrimSpace(operationID) == "" {
		operationID = id
	}
	return tx.Create(&costAdjustmentStartSnapshot{
		ID: id, OrganizationID: current.OrganizationID, OperationID: operationID, Mode: mode, ProductID: current.ID,
		PreviousInventoryStatus: current.InventoryStatus, PreviousBaseSalePriceMinor: current.BaseSalePriceMinor,
		PreviousBaseSaleCurrency: current.BaseSaleCurrency, PreviousInternalComment: current.InternalComment,
		PreviousCostAdjustmentStartedAt: current.CostAdjustmentStartedAt, Status: "active", CreatedBy: actor, CreatedAt: now,
	}).Error
}

func completeCostAdjustmentStartSnapshots(tx *gorm.DB, organizationID, operationID, productID, actor, status string, now time.Time) error {
	query := tx.Model(&costAdjustmentStartSnapshot{}).Where("organization_id=? AND status='active'", organizationID)
	if strings.TrimSpace(operationID) != "" {
		query = query.Where("operation_id=?", operationID)
	} else {
		query = query.Where("product_id=?", productID)
	}
	return query.Updates(map[string]any{"status": status, "completed_by": actor, "completed_at": now}).Error
}

type CostAdjustmentResetInput struct {
	OrganizationID string   `json:"-"`
	ActorUserID    string   `json:"-"`
	ProductIDs     []string `json:"productIds"`
}

type CostAdjustmentResetResult struct {
	Mode        string    `json:"mode"`
	OperationID string    `json:"operationId"`
	Products    []Product `json:"products"`
}

// ResetCostAdjustmentStart restores exactly one active, unconfirmed start
// operation. Confirmed adjustment slips are deliberately outside its scope.
func (r *Repository) ResetCostAdjustmentStart(ctx context.Context, input CostAdjustmentResetInput) (CostAdjustmentResetResult, error) {
	result := CostAdjustmentResetResult{}
	seen := map[string]bool{}
	ids := make([]string, 0, len(input.ProductIDs))
	for _, raw := range input.ProductIDs {
		id := strings.TrimSpace(raw)
		if id == "" || seen[id] {
			return result, ErrCostAdjustmentResetUnavailable
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 || len(ids) > 20 {
		return result, ErrCostAdjustmentResetUnavailable
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var snapshots []costAdjustmentStartSnapshot
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(
			"organization_id=? AND product_id IN ? AND status='active'", input.OrganizationID, ids,
		).Order("created_at DESC").Find(&snapshots).Error; err != nil {
			return err
		}
		if len(snapshots) != len(ids) {
			return ErrCostAdjustmentResetUnavailable
		}
		operationID, mode := snapshots[0].OperationID, snapshots[0].Mode
		for _, snapshot := range snapshots {
			if snapshot.OperationID != operationID || snapshot.Mode != mode {
				return ErrCostAdjustmentResetUnavailable
			}
		}
		var operationCount int64
		if err := tx.Model(&costAdjustmentStartSnapshot{}).Where(
			"organization_id=? AND operation_id=? AND status='active'", input.OrganizationID, operationID,
		).Count(&operationCount).Error; err != nil {
			return err
		}
		if operationCount != int64(len(snapshots)) {
			return ErrCostAdjustmentResetUnavailable
		}

		now := time.Now().UTC()
		result.Mode, result.OperationID = mode, operationID
		for _, snapshot := range snapshots {
			var current Product
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where(
				"organization_id=? AND id=? AND deleted_at IS NULL", input.OrganizationID, snapshot.ProductID,
			).Take(&current).Error; err != nil {
				return ErrCostAdjustmentResetUnavailable
			}
			if current.InventoryStatus != "cost_adjustment" {
				return ErrCostAdjustmentResetUnavailable
			}
			if err := tx.Model(&Product{}).Where("organization_id=? AND id=?", input.OrganizationID, current.ID).Updates(map[string]any{
				"inventory_status": snapshot.PreviousInventoryStatus, "base_sale_price_minor": snapshot.PreviousBaseSalePriceMinor,
				"base_sale_currency": snapshot.PreviousBaseSaleCurrency, "internal_comment": snapshot.PreviousInternalComment,
				"cost_adjustment_started_at": snapshot.PreviousCostAdjustmentStartedAt, "updated_at": now,
			}).Error; err != nil {
				return err
			}
			eventID, err := database.NewID("ive")
			if err != nil {
				return err
			}
			if err := tx.Exec(`INSERT INTO inventory_events(id,organization_id,product_id,event_type,from_status,to_status,reason,actor_user_id,created_at)
				VALUES(?,?,?,'cost_adjustment_start_reset','cost_adjustment',?,'開始済みの原価管理作業をリセット',?,?)`,
				eventID, input.OrganizationID, current.ID, snapshot.PreviousInventoryStatus, input.ActorUserID, now).Error; err != nil {
				return err
			}
		}
		if err := completeCostAdjustmentStartSnapshots(tx, input.OrganizationID, operationID, "", input.ActorUserID, "reset", now); err != nil {
			return err
		}
		if mode == "combine" {
			if err := tx.Exec("DELETE FROM product_cost_merge_inputs WHERE merge_id=?", operationID).Error; err != nil {
				return err
			}
			if update := tx.Model(&ProductCostMerge{}).Where("id=? AND organization_id=? AND status='draft'", operationID, input.OrganizationID).
				Update("status", "cancelled"); update.Error != nil || update.RowsAffected != 1 {
				return ErrCostAdjustmentResetUnavailable
			}
		}
		return nil
	})
	if err != nil {
		return CostAdjustmentResetResult{}, err
	}
	for _, id := range ids {
		product, err := r.ProductByID(ctx, input.OrganizationID, id)
		if err != nil {
			return CostAdjustmentResetResult{}, err
		}
		result.Products = append(result.Products, product)
	}
	return result, nil
}
