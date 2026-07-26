package database

import (
	"context"
	"errors"
	"testing"
	"time"
)

func createConfirmedApprovalSale(t *testing.T, store *Store) SalesSlip {
	t.Helper()
	product := salesTestProduct(t, store)
	sale := createTestSale(t, store, product.ID, 1, "JPY", 1200000)
	confirmed, err := store.ConfirmSale(context.Background(), "org_preview", sale.ID, "usr_admin")
	if err != nil {
		t.Fatal(err)
	}
	return confirmed
}

func createSaleCancelApproval(t *testing.T, store *Store, saleID, applicantID string) ApprovalRequest {
	t.Helper()
	approval, err := store.CreateApprovalRequest(context.Background(), CreateApprovalInput{
		OrganizationID: "org_preview", ApprovalType: "important_operation", TargetType: "sales_slip",
		TargetID: saleID, ActionKey: "sale.cancel", ApplicantUserID: applicantID,
		RequestReason: "入力訂正", ActionPayload: map[string]string{"reason": "入力訂正"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return approval
}

func addSecondAdmin(t *testing.T, store *Store) string {
	t.Helper()
	hash, err := HashPassword("second-admin-password")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := store.db.Exec(`
		INSERT INTO users(id,organization_id,username,password_hash,display_name,role_key,created_at,updated_at)
		VALUES('usr_admin2','org_preview','admin2',?,'承認管理者','admin',?,?)`, hash, now, now); err != nil {
		t.Fatal(err)
	}
	return "usr_admin2"
}

func TestApprovalRejectsSelfApprovalAndExecutesForDifferentAdmin(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	sale := createConfirmedApprovalSale(t, store)
	approval := createSaleCancelApproval(t, store, sale.ID, "usr_worker")
	if _, err := store.Approve(ctx, "org_preview", approval.ID, "usr_worker", "自己承認"); !errors.Is(err, ErrSelfApproval) {
		t.Fatalf("self approval should fail, got %v", err)
	}
	approved, err := store.Approve(ctx, "org_preview", approval.ID, "usr_admin", "内容確認済み")
	if err != nil {
		t.Fatal(err)
	}
	if approved.Status != "approved" || approved.ExecutedAt == nil || len(approved.Actions) != 2 {
		t.Fatalf("approval history incorrect: %+v", approved)
	}
	cancelled, err := store.Sale(ctx, "org_preview", sale.ID)
	if err != nil || cancelled.Status != "cancelled" {
		t.Fatalf("approved action not executed: status=%s err=%v", cancelled.Status, err)
	}
}

func TestAdminAlsoCannotApproveOwnRequest(t *testing.T) {
	store := testStore(t)
	sale := createConfirmedApprovalSale(t, store)
	approval := createSaleCancelApproval(t, store, sale.ID, "usr_admin")
	if _, err := store.Approve(context.Background(), "org_preview", approval.ID, "usr_admin", "自己承認"); !errors.Is(err, ErrSelfApproval) {
		t.Fatalf("admin self approval should fail, got %v", err)
	}
	if approval.Status != "pending" {
		t.Fatalf("approval should remain pending: %+v", approval)
	}
}

func TestReturnRequiresCommentAndPreservesHistory(t *testing.T) {
	store := testStore(t)
	sale := createConfirmedApprovalSale(t, store)
	approval := createSaleCancelApproval(t, store, sale.ID, "usr_worker")
	if err := store.ReturnApproval(context.Background(), "org_preview", approval.ID, "usr_admin", ""); err == nil {
		t.Fatal("return without comment must fail")
	}
	if err := store.ReturnApproval(context.Background(), "org_preview", approval.ID, "usr_admin", "取消理由を詳しく記載してください"); err != nil {
		t.Fatal(err)
	}
	returned, err := store.ApprovalRequest(context.Background(), "org_preview", approval.ID)
	if err != nil || returned.Status != "returned" || len(returned.Actions) != 2 ||
		returned.Actions[1].Comment != "取消理由を詳しく記載してください" {
		t.Fatalf("return history incorrect: %+v err=%v", returned, err)
	}
}

func TestChangedTargetExpiresOldApproval(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	sale := createConfirmedApprovalSale(t, store)
	approval := createSaleCancelApproval(t, store, sale.ID, "usr_worker")
	if _, err := store.db.ExecContext(ctx, `
		UPDATE sales_slips SET customer_name='変更後の販売先',updated_at=? WHERE id=?`,
		time.Now().UTC().Format(time.RFC3339Nano), sale.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Approve(ctx, "org_preview", approval.ID, "usr_admin", "承認"); !errors.Is(err, ErrApprovalStale) {
		t.Fatalf("stale approval should fail, got %v", err)
	}
	expired, err := store.ApprovalRequest(ctx, "org_preview", approval.ID)
	if err != nil || expired.Status != "expired" {
		t.Fatalf("approval should expire: %+v err=%v", expired, err)
	}
	unchanged, _ := store.Sale(ctx, "org_preview", sale.ID)
	if unchanged.Status != "confirmed" {
		t.Fatalf("stale action must not execute, got %s", unchanged.Status)
	}
}

func TestSaleApprovalThresholdsAndAdminMode(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	product := salesTestProduct(t, store)
	sale := createTestSale(t, store, product.ID, 1, "JPY", 500000)
	needed, total, err := store.NeedsSaleApproval(ctx, "org_preview", sale.ID, RoleWorker)
	if err != nil || needed || total != 500000 {
		t.Fatalf("unset worker threshold: needed=%v total=%d err=%v", needed, total, err)
	}
	if _, _, err := store.UpdateSetting(ctx, "org_preview", "usr_admin", "approval.sales_threshold_jpy", "400000"); err != nil {
		t.Fatal(err)
	}
	needed, _, _ = store.NeedsSaleApproval(ctx, "org_preview", sale.ID, RoleWorker)
	if !needed {
		t.Fatal("worker high-value sale should require approval")
	}
	needed, _, _ = store.NeedsSaleApproval(ctx, "org_preview", sale.ID, RoleAdmin)
	if needed {
		t.Fatal("admin initial mode should bypass approval")
	}
	if _, _, err := store.UpdateSetting(ctx, "org_preview", "usr_admin", "approval.admin_high_value_threshold_jpy", "400000"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.UpdateSetting(ctx, "org_preview", "usr_admin", "approval.admin_high_value_enabled", "true"); err != nil {
		t.Fatal(err)
	}
	needed, _, err = store.NeedsSaleApproval(ctx, "org_preview", sale.ID, RoleAdmin)
	if err != nil || !needed {
		t.Fatalf("future admin mode should require approval: needed=%v err=%v", needed, err)
	}

	adminApproval, err := store.CreateApprovalRequest(ctx, CreateApprovalInput{
		OrganizationID: "org_preview", ApprovalType: "high_value_sale", TargetType: "sales_slip",
		TargetID: sale.ID, ActionKey: "sale.confirm", ApplicantUserID: "usr_admin",
		RequestReason: "管理者高額取引", ActionPayload: map[string]string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	secondAdmin := addSecondAdmin(t, store)
	approved, err := store.Approve(ctx, "org_preview", adminApproval.ID, secondAdmin, "別管理者確認")
	if err != nil || approved.Status != "approved" {
		t.Fatalf("second admin approval failed: %+v err=%v", approved, err)
	}
	confirmed, _ := store.Sale(ctx, "org_preview", sale.ID)
	if confirmed.Status != "confirmed" {
		t.Fatalf("approved high-value sale not confirmed: %s", confirmed.Status)
	}
}
