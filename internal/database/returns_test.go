package database

import (
	"context"
	"errors"
	"testing"
)

func TestReturnTakehomeWorkflowAndFilters(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	if err := store.SeedInventoryPreview(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedSalesPreview(ctx); err != nil {
		t.Fatal(err)
	}
	sales, err := store.Sales(ctx, "org_preview")
	if err != nil || len(sales) == 0 {
		t.Fatalf("sales=%d err=%v", len(sales), err)
	}
	sale, err := store.Sale(ctx, "org_preview", sales[0].ID)
	if err != nil || len(sale.Lines) == 0 {
		t.Fatalf("sale=%+v err=%v", sale, err)
	}
	item, err := store.CreateReturnTakehome(ctx, "org_preview", sale.ID, sale.Lines[0].ID, "return", 1, "状態確認", "usr_admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateReturnTakehome(ctx, "org_preview", sale.ID, sale.Lines[0].ID, "return", 1, "重複", "usr_admin"); !errors.Is(err, ErrReturnAlreadyPending) {
		t.Fatalf("duplicate err=%v", err)
	}
	pending, err := store.ReturnTakehomeSummaries(ctx, "org_preview", "pending", sale.SlipNumber)
	if err != nil || len(pending) != 1 || pending[0].PendingCount != 1 {
		t.Fatalf("pending=%+v err=%v", pending, err)
	}
	byProduct, err := store.ReturnTakehomeSummaries(ctx, "org_preview", "", sale.Lines[0].Brand)
	if err != nil || len(byProduct) != 1 || byProduct[0].SalesDate != sale.SalesDate {
		t.Fatalf("product search/date=%+v err=%v", byProduct, err)
	}
	if err := store.CompleteReturnTakehome(ctx, "org_preview", sale.ID, item.ID, "検品完了", "usr_admin"); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteReturnTakehome(ctx, "org_preview", sale.ID, item.ID, "", "usr_admin"); !errors.Is(err, ErrReturnAlreadyHandled) {
		t.Fatalf("second complete err=%v", err)
	}
	awaitingRestore, err := store.ReturnTakehomeSummaries(ctx, "org_preview", "pending", sale.CustomerName)
	if err != nil || len(awaitingRestore) != 1 || awaitingRestore[0].PendingCount != 1 {
		t.Fatalf("awaiting restore=%+v err=%v", awaitingRestore, err)
	}
	other, err := store.ReturnTakehomeSummaries(ctx, "unknown_org", "", "")
	if err != nil || len(other) != 0 {
		t.Fatalf("cross-org records=%+v err=%v", other, err)
	}
}

func TestSalesReturnRequiresPrintedInvoiceBeforeCompletion(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	if err := store.SeedInventoryPreview(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedSalesPreview(ctx); err != nil {
		t.Fatal(err)
	}
	sales, err := store.Sales(ctx, "org_preview")
	if err != nil || len(sales) == 0 {
		t.Fatalf("sales=%+v err=%v", sales, err)
	}
	sale, err := store.Sale(ctx, "org_preview", sales[0].ID)
	if err != nil || len(sale.Lines) == 0 {
		t.Fatalf("sale=%+v err=%v", sale, err)
	}
	if _, err := store.CreateReturnTakehome(ctx, "org_preview", sale.ID, sale.Lines[0].ID,
		"return", 1, "請求書制御", "usr_admin"); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteSalesReturn(ctx, "org_preview", sale.ID, "usr_admin"); !errors.Is(err, ErrSalesReturnInvoiceRequired) {
		t.Fatalf("completion before invoice err=%v", err)
	}
	if err := store.IssueSalesReturnInvoice(ctx, "org_preview", sale.ID, "usr_admin"); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteSalesReturn(ctx, "org_preview", sale.ID, "usr_admin"); err != nil {
		t.Fatal(err)
	}
	items, err := store.ReturnTakehomeItems(ctx, "org_preview", sale.ID)
	if err != nil || !SalesReturnInvoiceReady(items) || !SalesReturnCompleted(items) {
		t.Fatalf("items=%+v err=%v", items, err)
	}
}

func TestRestoreReturnTakehomeMovesProductBackToInventory(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	if err := store.SeedInventoryPreview(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedSalesPreview(ctx); err != nil {
		t.Fatal(err)
	}
	sales, err := store.Sales(ctx, "org_preview")
	if err != nil || len(sales) == 0 {
		t.Fatalf("sales=%+v err=%v", sales, err)
	}
	sale, err := store.Sale(ctx, "org_preview", sales[0].ID)
	if err != nil || len(sale.Lines) == 0 {
		t.Fatalf("sale=%+v err=%v", sale, err)
	}
	item, err := store.CreateReturnTakehome(ctx, "org_preview", sale.ID, sale.Lines[0].ID,
		"return", 1, "在庫戻しテスト", "usr_admin")
	if err != nil {
		t.Fatal(err)
	}
	err = store.RestoreReturnTakehomeItems(ctx, RestoreReturnTakehomeInput{
		OrganizationID: "org_preview",
		SaleID:         sale.ID,
		ActorID:        "usr_admin",
		Comment:        "BOX確認済み",
		Items: []ReturnRestoreItemInput{{
			ItemID: item.ID, Condition: "美品 (A)", Quantity: 1, Box: "BOX1",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	product, err := store.Product(ctx, "org_preview", sale.Lines[0].ProductID)
	if err != nil {
		t.Fatal(err)
	}
	if product.InventoryStatus != "in_stock" || product.Condition != "美品 (A)" || product.Box != "BOX1" {
		t.Fatalf("restored product=%+v", product)
	}
	items, err := store.ReturnTakehomeItems(ctx, "org_preview", sale.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Status != "completed" {
		t.Fatalf("items=%+v", items)
	}
	summaries, err := store.ReturnTakehomeSummaries(ctx, "org_preview", "completed", sale.SlipNumber)
	if err != nil || len(summaries) != 1 || summaries[0].TotalJPY == 0 {
		t.Fatalf("completed amount disappeared: %+v err=%v", summaries, err)
	}
	if err := store.RestoreReturnTakehomeItems(ctx, RestoreReturnTakehomeInput{
		OrganizationID: "org_preview", SaleID: sale.ID, ActorID: "usr_admin",
		Items: []ReturnRestoreItemInput{{ItemID: item.ID, Condition: "美品 (A)", Quantity: 1}},
	}); !errors.Is(err, ErrReturnAlreadyHandled) {
		t.Fatalf("second restore err=%v", err)
	}
}

func TestWorkerReturnRestoreApprovalDoesNotChangeInventoryUntilApproved(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	if err := store.SeedInventoryPreview(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedSalesPreview(ctx); err != nil {
		t.Fatal(err)
	}
	sales, err := store.Sales(ctx, "org_preview")
	if err != nil || len(sales) == 0 {
		t.Fatalf("sales=%+v err=%v", sales, err)
	}
	sale, err := store.Sale(ctx, "org_preview", sales[0].ID)
	if err != nil || len(sale.Lines) == 0 {
		t.Fatalf("sale=%+v err=%v", sale, err)
	}
	item, err := store.CreateReturnTakehome(ctx, "org_preview", sale.ID, sale.Lines[0].ID,
		"return", 1, "承認テスト", "usr_worker")
	if err != nil {
		t.Fatal(err)
	}
	payload := RestoreReturnTakehomeInput{
		OrganizationID: "org_preview", SaleID: sale.ID, ActorID: "usr_worker",
		Items: []ReturnRestoreItemInput{{ItemID: item.ID, Quantity: 1, Box: "BOX2"}},
	}
	approval, err := store.CreateApprovalRequest(ctx, CreateApprovalInput{
		OrganizationID: "org_preview", ApprovalType: "inventory", TargetType: "return_takehome",
		TargetID: sale.ID, ActionKey: "return_takehome.restore", ApplicantUserID: "usr_worker",
		RequestReason: "在庫戻し", ActionPayload: payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	product, err := store.Product(ctx, "org_preview", sale.Lines[0].ProductID)
	if err != nil || product.InventoryStatus == "in_stock" {
		t.Fatalf("inventory changed before approval: %+v err=%v", product, err)
	}
	if _, err := store.Approve(ctx, "org_preview", approval.ID, addSecondAdmin(t, store), "確認済み"); err != nil {
		t.Fatal(err)
	}
	product, err = store.Product(ctx, "org_preview", sale.Lines[0].ProductID)
	if err != nil || product.InventoryStatus != "in_stock" || product.Box != "BOX2" {
		t.Fatalf("inventory not restored after approval: %+v err=%v", product, err)
	}
}
