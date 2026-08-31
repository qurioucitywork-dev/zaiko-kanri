package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestStocktakeSnapshotScanAndPersistence(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:stocktake_snapshot?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Product{}, &StocktakeSession{}, &StocktakeLine{}); err != nil {
		t.Fatal(err)
	}
	repo := &Repository{db: db, driver: "sqlite"}
	now := time.Now().UTC()
	products := []Product{
		{ID: "prd_stock", OrganizationID: "org", ProductCode: "STOCK-1", InventoryStatus: "in_stock", Brand: "A", CostAmountMinor: 1000, CreatedAt: now, UpdatedAt: now},
		{ID: "prd_return", OrganizationID: "org", ProductCode: "RETURN-1", InventoryStatus: "return_pending", Brand: "B", CostAmountMinor: 2000, CreatedAt: now, UpdatedAt: now},
		{ID: "prd_cancel", OrganizationID: "org", ProductCode: "CANCEL-1", InventoryStatus: "cancelled", Brand: "C", CostAmountMinor: 3000, CreatedAt: now, UpdatedAt: now},
		{ID: "prd_sold", OrganizationID: "org", ProductCode: "SOLD-1", InventoryStatus: "sold", Brand: "D", CostAmountMinor: 4000, CreatedAt: now, UpdatedAt: now},
	}
	if err := db.Create(&products).Error; err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	session, err := repo.StartStocktake(ctx, "org", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if len(session.Lines) != 2 {
		t.Fatalf("snapshot lines=%d, want 2 (in_stock and return_pending)", len(session.Lines))
	}

	session, message, err := repo.ScanStocktake(ctx, "org", session.ID, "RETURN-1")
	if err != nil || message != "verified" {
		t.Fatalf("scan expected message=%q err=%v", message, err)
	}
	if line := findStocktakeLine(session.Lines, "RETURN-1"); line == nil || line.ResultStatus != "verified" {
		t.Fatalf("return item was not verified: %+v", line)
	}

	session, message, err = repo.ScanStocktake(ctx, "org", session.ID, "CANCEL-1")
	if err != nil || message != "cancelled_ignored" {
		t.Fatalf("scan cancelled message=%q err=%v", message, err)
	}
	if line := findStocktakeLine(session.Lines, "CANCEL-1"); line != nil {
		t.Fatalf("cancelled item must not appear in stocktake lines: %+v", line)
	}

	session, message, err = repo.ScanStocktake(ctx, "org", session.ID, "NOT-REGISTERED")
	if err != nil || message != "unknown_added" {
		t.Fatalf("scan unregistered message=%q err=%v", message, err)
	}
	if line := findStocktakeLine(session.Lines, "NOT-REGISTERED"); line == nil || line.LineType != "unknown_inventory" || line.Source != "unregistered" {
		t.Fatalf("unregistered code was not added as unknown: %+v", line)
	}

	missing := findStocktakeLine(session.Lines, "STOCK-1")
	if missing == nil {
		t.Fatal("missing snapshot line not found")
	}
	session, err = repo.SaveStocktake(ctx, "org", session.ID, map[string]struct{ Reason, Note string }{
		missing.ID: {Reason: "紛失", Note: "保管場所を確認中"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if line := findStocktakeLine(session.Lines, "STOCK-1"); line == nil || line.Reason != "紛失" || line.Note != "保管場所を確認中" {
		t.Fatalf("saved discrepancy was not restored: %+v", line)
	}

	completed, err := repo.CompleteStocktake(ctx, "org", session.ID, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != "completed" || completed.CompletedAt == nil {
		t.Fatalf("completed session=%+v", completed)
	}
}

func TestStocktakeSyncAddsNewInventoryAndPreservesProgress(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:stocktake_sync?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Product{}, &StocktakeSession{}, &StocktakeLine{}); err != nil {
		t.Fatal(err)
	}
	repo := &Repository{db: db, driver: "sqlite"}
	now := time.Now().UTC()
	first := Product{ID: "prd_first", OrganizationID: "org", ProductCode: "FIRST-1", InventoryStatus: "in_stock", Brand: "A", CreatedAt: now, UpdatedAt: now}
	if err := db.Create(&first).Error; err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	session, err := repo.StartStocktake(ctx, "org", "admin")
	if err != nil {
		t.Fatal(err)
	}
	session, _, err = repo.ScanStocktake(ctx, "org", session.ID, first.ProductCode)
	if err != nil {
		t.Fatal(err)
	}

	newProduct := Product{ID: "prd_new", OrganizationID: "org", ProductCode: "NEW-1", InventoryStatus: "in_stock", Brand: "B", CreatedAt: now, UpdatedAt: now}
	ignored := Product{ID: "prd_sold_new", OrganizationID: "org", ProductCode: "SOLD-NEW", InventoryStatus: "sold", Brand: "C", CreatedAt: now, UpdatedAt: now}
	if err := db.Create(&[]Product{newProduct, ignored}).Error; err != nil {
		t.Fatal(err)
	}
	session, added, err := repo.SyncStocktake(ctx, "org", session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if added != 1 {
		t.Fatalf("added=%d, want 1", added)
	}
	if line := findStocktakeLine(session.Lines, first.ProductCode); line == nil || line.ResultStatus != "verified" {
		t.Fatalf("existing progress was not preserved: %+v", line)
	}
	if line := findStocktakeLine(session.Lines, newProduct.ProductCode); line == nil || line.ResultStatus != "missing" || line.Source != "registered_during_stocktake" {
		t.Fatalf("new inventory was not added: %+v", line)
	}
	if line := findStocktakeLine(session.Lines, ignored.ProductCode); line != nil {
		t.Fatalf("non-target inventory was added: %+v", line)
	}
}

func TestStocktakeSyncRemovesProductsChangedToCancelled(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:stocktake_cancelled_sync?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Product{}, &StocktakeSession{}, &StocktakeLine{}); err != nil {
		t.Fatal(err)
	}
	repo := &Repository{db: db, driver: "sqlite"}
	now := time.Now().UTC()
	product := Product{ID: "prd_cancel_after_start", OrganizationID: "org", ProductCode: "CANCEL-AFTER-START", InventoryStatus: "in_stock", CreatedAt: now, UpdatedAt: now}
	if err := db.Create(&product).Error; err != nil {
		t.Fatal(err)
	}
	session, err := repo.StartStocktake(context.Background(), "org", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if findStocktakeLine(session.Lines, product.ProductCode) == nil {
		t.Fatal("product was not included in initial stocktake snapshot")
	}
	if err := db.Model(&Product{}).Where("id = ?", product.ID).Update("inventory_status", "cancelled").Error; err != nil {
		t.Fatal(err)
	}
	session, _, err = repo.SyncStocktake(context.Background(), "org", session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if line := findStocktakeLine(session.Lines, product.ProductCode); line != nil {
		t.Fatalf("cancelled product remained after stocktake sync: %+v", line)
	}
}

func findStocktakeLine(lines []StocktakeLine, code string) *StocktakeLine {
	for index := range lines {
		if lines[index].ProductCode == code {
			return &lines[index]
		}
	}
	return nil
}
