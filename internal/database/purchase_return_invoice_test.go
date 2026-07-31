package database

import (
	"testing"
	"time"
)

func TestPurchaseReturnCanCompleteWithoutInvoice(t *testing.T) {
	store := testStore(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedPurchaseReturnPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	purchases, err := store.PurchaseSlips(t.Context(), "org_preview")
	if err != nil {
		t.Fatal(err)
	}
	if len(purchases) == 0 {
		t.Fatal("purchase fixture not found")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := store.db.ExecContext(t.Context(), `
		INSERT INTO purchase_return_slips(
			id,organization_id,purchase_slip_id,return_number,return_date,supplier_name,
			item_count,amount_jpy,reason,status,delivery_number,created_at,updated_at,notes,created_by
		) VALUES('pret_invoice_gate','org_preview',?,'PR-RET-TEST','2026-07-28',?,
			1,850000,'商品不良','pending','',?,?,?,'usr_admin')`,
		purchases[0].ID, purchases[0].SupplierName, now, now, "請求書任意テスト"); err != nil {
		t.Fatal(err)
	}
	pending, err := store.PurchaseReturn(t.Context(), "org_preview", "pret_invoice_gate")
	if err != nil {
		t.Fatal(err)
	}
	if pending.InvoicePrintedAt != nil {
		t.Fatal("fixture must start without a printed invoice")
	}
	if err := store.CompletePurchaseReturn(t.Context(), "org_preview", pending.ID); err != nil {
		t.Fatalf("complete without invoice: %v", err)
	}
	completed, err := store.PurchaseReturn(t.Context(), "org_preview", pending.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != "completed" {
		t.Fatalf("status=%q, want completed", completed.Status)
	}
	if completed.InvoicePrintedAt != nil {
		t.Fatal("completion must not fabricate an invoice print record")
	}
}

func TestPurchaseReturnInvoiceRecordsIssueAndPrint(t *testing.T) {
	store := testStore(t)
	if err := store.SeedInventoryPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedPurchaseReturnPreview(t.Context()); err != nil {
		t.Fatal(err)
	}
	items, err := store.PurchaseReturnSlips(t.Context(), "org_preview")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 {
		t.Fatal("purchase return fixture not found")
	}
	if err := store.IssuePurchaseReturnInvoice(
		t.Context(), "org_preview", items[0].ID, "usr_admin",
	); err != nil {
		t.Fatal(err)
	}
	updated, err := store.PurchaseReturn(t.Context(), "org_preview", items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.InvoiceIssuedAt == nil || updated.InvoicePrintedAt == nil {
		t.Fatal("invoice issue and print timestamps were not recorded")
	}
}
