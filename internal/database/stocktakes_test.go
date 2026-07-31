package database

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestStocktakeLifecycleRequiresEveryLine(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	if err := store.SeedInventoryPreview(ctx); err != nil {
		t.Fatal(err)
	}
	stocktake, err := store.CreateStocktake(ctx, "org_preview", "2026-07-27", "テスト棚卸", "usr_admin")
	if err != nil {
		t.Fatal(err)
	}
	if stocktake.Status != "draft" || stocktake.ExpectedCount != 1 || len(stocktake.Lines) != 1 {
		t.Fatalf("stocktake=%+v lines=%d", stocktake, len(stocktake.Lines))
	}
	existing, err := store.CreateStocktake(ctx, "org_preview", "2026-07-28", "", "usr_admin")
	if !errors.Is(err, ErrStocktakeInProgress) {
		t.Fatalf("second stocktake error=%v", err)
	}
	if existing.ID != stocktake.ID {
		t.Fatalf("in-progress stocktake=%q, want %q", existing.ID, stocktake.ID)
	}
	if _, err := store.CompleteStocktake(ctx, "org_preview", stocktake.ID, "usr_admin"); !errors.Is(err, ErrStocktakeUnchecked) {
		t.Fatalf("unchecked complete error=%v", err)
	}
	if err := store.UpdateStocktakeLine(
		ctx, "org_preview", stocktake.ID, stocktake.Lines[0].ID, false, "所在確認中", "usr_admin",
	); err != nil {
		t.Fatal(err)
	}
	completed, err := store.CompleteStocktake(ctx, "org_preview", stocktake.ID, "usr_admin")
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != "completed" || completed.CountedCount != 1 || completed.DifferenceCount != 1 {
		t.Fatalf("completed=%+v", completed)
	}
	if err := store.UpdateStocktakeLine(
		ctx, "org_preview", stocktake.ID, stocktake.Lines[0].ID, true, "", "usr_admin",
	); !errors.Is(err, ErrStocktakeCompleted) {
		t.Fatalf("completed update error=%v", err)
	}
}

func TestCreateStocktakeConcurrentCallsReturnOneDraft(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	if err := store.SeedInventoryPreview(ctx); err != nil {
		t.Fatal(err)
	}

	const callers = 8
	start := make(chan struct{})
	results := make(chan Stocktake, callers)
	errs := make(chan error, callers)
	var workers sync.WaitGroup
	workers.Add(callers)
	for range callers {
		go func() {
			defer workers.Done()
			<-start
			stocktake, err := store.CreateStocktake(ctx, "org_preview", "2026-07-30", "", "usr_worker")
			results <- stocktake
			errs <- err
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	close(errs)

	var draftID string
	for result := range results {
		if result.ID == "" {
			t.Fatal("concurrent caller did not receive the active draft")
		}
		if draftID == "" {
			draftID = result.ID
		} else if result.ID != draftID {
			t.Fatalf("multiple draft IDs returned: %q and %q", draftID, result.ID)
		}
	}
	for err := range errs {
		if err != nil && !errors.Is(err, ErrStocktakeInProgress) {
			t.Fatalf("unexpected concurrent create error: %v", err)
		}
	}
	var count int
	if err := store.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM stocktakes
		WHERE organization_id='org_preview' AND status='draft'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("draft count=%d, want 1", count)
	}
	now := store.now().Format("2006-01-02T15:04:05.999999999Z07:00")
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO stocktakes(
			id,organization_id,stocktake_number,stocktake_date,status,created_by,created_at,updated_at
		) VALUES('stk_duplicate','org_preview','STK-DIRECT-999','2026-07-30','draft','usr_worker',?,?)`,
		now, now); err == nil {
		t.Fatal("database accepted a second draft for the same organization")
	}
}

func TestStocktakeMockWorkflowPersistsDraftAndBlocksPendingDifference(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	if err := store.SeedInventoryPreview(ctx); err != nil {
		t.Fatal(err)
	}
	stocktake, err := store.CreateStocktake(ctx, "org_preview", "2026-07-28", "差異テスト", "usr_worker")
	if err != nil {
		t.Fatal(err)
	}
	if stocktake.ExpectedTotalMinor <= 0 {
		t.Fatalf("expected total was not snapshotted: %+v", stocktake)
	}
	if err := store.SaveStocktake(ctx, "org_preview", stocktake.ID); err != nil {
		t.Fatal(err)
	}
	saved, err := store.Stocktake(ctx, "org_preview", stocktake.ID)
	if err != nil || saved.SavedAt == nil {
		t.Fatalf("saved=%+v err=%v", saved, err)
	}
	line := saved.Lines[0]
	if err := store.RecordStocktakeDifference(
		ctx, "org_preview", saved.ID, line.ID, "商品が見つからない", "保管場所を再確認", "usr_worker", true,
	); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkStocktakePresent(ctx, "org_preview", saved.ID, line.ID, "usr_worker"); !errors.Is(err, ErrStocktakeDuplicate) {
		t.Fatalf("duplicate result error=%v", err)
	}
	pending, err := store.Stocktake(ctx, "org_preview", saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if pending.Lines[0].ReviewStatus != "pending" || pending.Lines[0].DifferenceReason != "商品が見つからない" {
		t.Fatalf("pending line=%+v", pending.Lines[0])
	}
	if _, err := store.CompleteStocktake(ctx, "org_preview", saved.ID, "usr_admin"); !errors.Is(err, ErrStocktakePending) {
		t.Fatalf("pending complete error=%v", err)
	}
}

func TestStocktakeCompletionStoresFinalizedDate(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	if err := store.SeedInventoryPreview(ctx); err != nil {
		t.Fatal(err)
	}
	stocktake, err := store.CreateStocktake(ctx, "org_preview", "2026-07-29", "", "usr_admin")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MarkStocktakePresent(ctx, "org_preview", stocktake.ID, stocktake.Lines[0].ID, "usr_admin"); err != nil {
		t.Fatal(err)
	}
	completed, err := store.CompleteStocktake(ctx, "org_preview", stocktake.ID, "usr_admin")
	if err != nil {
		t.Fatal(err)
	}
	if completed.Lines[0].FinalizedAt == nil {
		t.Fatalf("finalized date missing: %+v", completed.Lines[0])
	}
}
