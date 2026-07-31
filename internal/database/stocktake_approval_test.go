package database

import (
	"context"
	"testing"
)

func TestStocktakeDifferenceApprovalSupportsReturnResubmitAndApprove(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	if err := store.SeedInventoryPreview(ctx); err != nil {
		t.Fatal(err)
	}
	stocktake, err := store.CreateStocktake(ctx, "org_preview", "2026-07-30", "", "usr_worker")
	if err != nil {
		t.Fatal(err)
	}
	line := stocktake.Lines[0]
	approval, err := store.RequestStocktakeDifferenceApproval(
		ctx, "org_preview", stocktake.ID, line.ID, "商品が見つからない", "倉庫を再確認", "usr_worker",
	)
	if err != nil || approval.Status != "pending" {
		t.Fatalf("approval=%+v err=%v", approval, err)
	}
	pending, err := store.Stocktake(ctx, "org_preview", stocktake.ID)
	if err != nil || pending.Lines[0].ReviewStatus != "pending" {
		t.Fatalf("pending=%+v err=%v", pending, err)
	}
	if err := store.ReturnApproval(ctx, "org_preview", approval.ID, "usr_admin", "保管場所を再確認してください"); err != nil {
		t.Fatal(err)
	}
	returned, err := store.Stocktake(ctx, "org_preview", stocktake.ID)
	if err != nil || returned.Lines[0].ApprovalStatus != "returned" {
		t.Fatalf("returned=%+v err=%v", returned.Lines[0], err)
	}
	resubmitted, err := store.RequestStocktakeDifferenceApproval(
		ctx, "org_preview", stocktake.ID, line.ID, "商品が見つからない", "再確認済み", "usr_worker",
	)
	if err != nil || resubmitted.ID != approval.ID || resubmitted.Status != "pending" {
		t.Fatalf("resubmitted=%+v err=%v", resubmitted, err)
	}
	if len(resubmitted.Actions) < 3 {
		t.Fatalf("approval history was not preserved: %+v", resubmitted.Actions)
	}
	approved, err := store.Approve(ctx, "org_preview", approval.ID, "usr_admin", "確認済み")
	if err != nil || approved.Status != "approved" {
		t.Fatalf("approved=%+v err=%v", approved, err)
	}
	final, err := store.Stocktake(ctx, "org_preview", stocktake.ID)
	if err != nil || final.Lines[0].ReviewStatus != "approved" ||
		final.Lines[0].ApprovalStatus != "approved" {
		t.Fatalf("final=%+v err=%v", final.Lines[0], err)
	}
	if _, err := store.CompleteStocktake(ctx, "org_preview", stocktake.ID, "usr_admin"); err != nil {
		t.Fatalf("approved mismatch must be completable: %v", err)
	}
}
