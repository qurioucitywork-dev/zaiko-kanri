package persistence

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestCostAdjustmentStartResetPostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_MERGE_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZAIKO_MERGE_TEST_DSN for transactional PostgreSQL verification")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	tx := db.Begin()
	defer tx.Rollback()

	var original Product
	if err := tx.Where(`inventory_status='in_stock' AND deleted_at IS NULL AND purchase_slip_line_id IS NOT NULL
		AND NOT EXISTS(SELECT 1 FROM cost_adjustment_start_snapshots s WHERE s.product_id=products.id AND s.status='active')
		AND NOT EXISTS(SELECT 1 FROM product_cost_merge_inputs i WHERE i.product_id=products.id)`).
		Order("organization_id,id").Take(&original).Error; err != nil {
		t.Fatal(err)
	}
	var actor string
	if err := tx.Raw("SELECT id FROM users WHERE organization_id=? LIMIT 1", original.OrganizationID).Scan(&actor).Error; err != nil {
		t.Fatal(err)
	}
	repo := &Repository{db: tx, driver: "postgres"}
	ctx := context.Background()

	started, err := repo.StartDirectProductCostAdjustment(ctx, original.OrganizationID, original.ID, actor)
	if err != nil {
		t.Fatal(err)
	}
	if started.InventoryStatus != "cost_adjustment" || started.BaseSalePriceMinor != 0 {
		t.Fatalf("start state=%q sale=%d", started.InventoryStatus, started.BaseSalePriceMinor)
	}
	result, err := repo.ResetCostAdjustmentStart(ctx, CostAdjustmentResetInput{
		OrganizationID: original.OrganizationID, ActorUserID: actor, ProductIDs: []string{original.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != "adjustment" || len(result.Products) != 1 {
		t.Fatalf("reset result=%+v", result)
	}
	restored := result.Products[0]
	if restored.InventoryStatus != original.InventoryStatus || restored.BaseSalePriceMinor != original.BaseSalePriceMinor ||
		restored.BaseSaleCurrency != original.BaseSaleCurrency || restored.InternalComment != original.InternalComment {
		t.Fatalf("product was not restored: before=%+v after=%+v", original, restored)
	}
	if !sameOptionalTime(restored.CostAdjustmentStartedAt, original.CostAdjustmentStartedAt) {
		t.Fatalf("start timestamp was not restored: before=%v after=%v", original.CostAdjustmentStartedAt, restored.CostAdjustmentStartedAt)
	}
	if _, err := repo.ResetCostAdjustmentStart(ctx, CostAdjustmentResetInput{
		OrganizationID: original.OrganizationID, ActorUserID: actor, ProductIDs: []string{original.ID},
	}); !errors.Is(err, ErrCostAdjustmentResetUnavailable) {
		t.Fatalf("second reset error=%v, want ErrCostAdjustmentResetUnavailable", err)
	}
	if _, err := repo.StartDirectProductCostAdjustment(ctx, original.OrganizationID, original.ID, actor); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ConfirmDirectCostAdjustment(ctx, DirectCostAdjustmentInput{
		OrganizationID: original.OrganizationID, ActorUserID: actor, Direction: "increase", Notes: "確定後はリセット不可",
		Items: []DirectCostAdjustmentItemInput{{ProductID: original.ID, AdjustmentAmountJPY: 1}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ResetCostAdjustmentStart(ctx, CostAdjustmentResetInput{
		OrganizationID: original.OrganizationID, ActorUserID: actor, ProductIDs: []string{original.ID},
	}); !errors.Is(err, ErrCostAdjustmentResetUnavailable) {
		t.Fatalf("confirmed operation reset error=%v, want ErrCostAdjustmentResetUnavailable", err)
	}
}

func TestProductCostMergeResetRequiresWholeOperationPostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_MERGE_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZAIKO_MERGE_TEST_DSN for transactional PostgreSQL verification")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	tx := db.Begin()
	defer tx.Rollback()

	var originals []Product
	if err := tx.Where(`inventory_status='in_stock' AND deleted_at IS NULL AND purchase_slip_line_id IS NOT NULL
		AND NOT EXISTS(SELECT 1 FROM cost_adjustment_start_snapshots s WHERE s.product_id=products.id AND s.status='active')
		AND NOT EXISTS(SELECT 1 FROM product_cost_merge_inputs i WHERE i.product_id=products.id)`).
		Order("organization_id,id").Limit(8).Find(&originals).Error; err != nil {
		t.Fatal(err)
	}
	if len(originals) < 2 {
		t.Fatal("requires two available products")
	}
	first := originals[0]
	secondIndex := -1
	for index := 1; index < len(originals); index++ {
		if originals[index].OrganizationID == first.OrganizationID {
			secondIndex = index
			break
		}
	}
	if secondIndex < 0 {
		t.Fatal("requires two available products in one organization")
	}
	selected := []Product{first, originals[secondIndex]}
	ids := []string{selected[0].ID, selected[1].ID}
	var actor string
	if err := tx.Raw("SELECT id FROM users WHERE organization_id=? LIMIT 1", first.OrganizationID).Scan(&actor).Error; err != nil {
		t.Fatal(err)
	}
	repo := &Repository{db: tx, driver: "postgres"}
	ctx := context.Background()
	session, err := repo.StartProductCostMerge(ctx, first.OrganizationID, actor, ids)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ResetCostAdjustmentStart(ctx, CostAdjustmentResetInput{
		OrganizationID: first.OrganizationID, ActorUserID: actor, ProductIDs: ids[:1],
	}); !errors.Is(err, ErrCostAdjustmentResetUnavailable) {
		t.Fatalf("partial merge reset error=%v, want ErrCostAdjustmentResetUnavailable", err)
	}
	result, err := repo.ResetCostAdjustmentStart(ctx, CostAdjustmentResetInput{
		OrganizationID: first.OrganizationID, ActorUserID: actor, ProductIDs: ids,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != "combine" || result.OperationID != session.ID || len(result.Products) != 2 {
		t.Fatalf("merge reset result=%+v", result)
	}
	for _, original := range selected {
		var restored Product
		if err := tx.Where("id=?", original.ID).Take(&restored).Error; err != nil {
			t.Fatal(err)
		}
		if restored.InventoryStatus != original.InventoryStatus || restored.BaseSalePriceMinor != original.BaseSalePriceMinor || restored.InternalComment != original.InternalComment {
			t.Fatalf("merge input %s was not restored", original.ID)
		}
	}
	var sessionStatus string
	tx.Raw("SELECT status FROM product_cost_merges WHERE id=?", session.ID).Scan(&sessionStatus)
	if sessionStatus != "cancelled" {
		t.Fatalf("merge session status=%q, want cancelled", sessionStatus)
	}
	var links int64
	tx.Table("product_cost_merge_inputs").Where("merge_id=?", session.ID).Count(&links)
	if links != 0 {
		t.Fatalf("merge input links=%d, want 0", links)
	}
}

func sameOptionalTime(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}
