package persistence

import (
	"context"
	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
	"os"
	"strings"
	"testing"
)

func TestCashPurchasePaymentPostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_CASH_PAYMENT_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	if !strings.Contains(dsn, "/zaiko_cash_payment_test") {
		t.Fatal("non-test database rejected")
	}
	ctx := context.Background()
	repo, err := Open(config.Config{DatabaseDriver: "postgres", DatabaseURL: dsn, Environment: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(repo.Migrate(ctx))
	must(repo.SeedPreviewIdentity(ctx, "cost-test-admin-password", "cost-test-worker-password"))
	must(repo.SeedPreviewMasters(ctx))
	var org, admin, supplier, brand string
	for _, q := range []struct {
		sql   string
		value *string
	}{
		{"SELECT id FROM organizations WHERE code='PREVIEW'", &org},
		{"SELECT id FROM users WHERE username='admin' LIMIT 1", &admin},
		{"SELECT role_code FROM partner_roles WHERE role_type='supplier' AND is_active ORDER BY role_code LIMIT 1", &supplier},
		{"SELECT code FROM brands WHERE is_active ORDER BY code LIMIT 1", &brand},
	} {
		must(repo.db.Raw(q.sql).Scan(q.value).Error)
	}
	must(repo.db.Exec("INSERT INTO exchange_rate_snapshots(id,organization_id,base_currency,quote_currency,rate_scaled,scale,provider,created_by,observed_at,created_at) VALUES('consign-test-rate',?,'USD','JPY',15500,100,'test',?,now(),now())", org, admin).Error)

	create := func(method string) PurchaseSlipRecord {
		slip, e := repo.CreatePurchase(ctx, PurchaseCreateInput{OrganizationID: org, ActorUserID: admin, SupplierCode: supplier, PurchaseDate: "2026-09-17", PaymentMethod: method, Lines: []PurchaseLineInput{{Quantity: 1, BrandCode: brand, UnitCostMinor: 1000, CostCurrency: "JPY", BaseSaleCurrency: "USD"}}})
		must(e)
		return slip
	}
	for _, method := range []string{"cash", "bank_transfer", "card"} {
		draft := create(method)
		if draft.PaidAt != nil {
			t.Fatal("draft marked paid")
		}
		saved, e := repo.ConfirmPurchase(ctx, org, draft.ID, admin)
		must(e)
		if (saved.PaidAt != nil) != (method == "cash") {
			t.Fatalf("wrong payment: %s %+v", method, saved)
		}
		if saved.PendingArrivalCount != 1 {
			t.Fatal("payment incorrectly completed arrival")
		}
		if method == "cash" && (saved.PaidBy != admin || !saved.PaidAt.Equal(*saved.ConfirmedAt)) {
			t.Fatal("cash payment audit fields missing")
		}
		reloaded, e := repo.PurchaseSlip(ctx, org, saved.ID)
		must(e)
		if (reloaded.PaidAt != nil) != (method == "cash") {
			t.Fatal("payment not persisted")
		}
		again, e := repo.ConfirmPurchase(ctx, org, saved.ID, admin)
		must(e)
		if method == "cash" && !again.PaidAt.Equal(*saved.PaidAt) {
			t.Fatal("reconfirmation changed payment")
		}
		_, e = repo.ReceivePurchaseProduct(ctx, org, saved.ID, saved.CreatedProducts[0].ProductCode, admin)
		must(e)
		arrived, e := repo.PurchaseSlip(ctx, org, saved.ID)
		must(e)
		if arrived.PendingArrivalCount != 0 || (arrived.PaidAt != nil) != (method == "cash") {
			t.Fatal("arrival/payment state mismatch")
		}
	}
	// Replay the data correction on legacy cash, preserving drafts and other payment methods.
	legacy := create("cash")
	legacy, e := repo.ConfirmPurchase(ctx, org, legacy.ID, admin)
	must(e)
	must(repo.db.Exec("UPDATE purchase_slips SET paid_at=NULL,paid_by=NULL WHERE id=?", legacy.ID).Error)
	pending := create("cash")
	migration, e := migrationFS.ReadFile("migrations/000093_cash_purchase_payment.up.sql")
	must(e)
	must(repo.db.Exec(string(migration)).Error)
	fixed, e := repo.PurchaseSlip(ctx, org, legacy.ID)
	must(e)
	if fixed.PaidAt == nil || !fixed.PaidAt.Equal(*fixed.ConfirmedAt) || fixed.PaidBy != admin {
		t.Fatal("legacy cash not corrected")
	}
	must(repo.db.Exec(string(migration)).Error)
	fixedAgain, e := repo.PurchaseSlip(ctx, org, legacy.ID)
	must(e)
	if !fixedAgain.PaidAt.Equal(*fixed.PaidAt) {
		t.Fatal("migration is not idempotent")
	}
	notConfirmed, e := repo.PurchaseSlip(ctx, org, pending.ID)
	must(e)
	if notConfirmed.PaidAt != nil {
		t.Fatal("migration paid draft")
	}
	var wrong int64
	must(repo.db.Table("purchase_slips").Where("payment_method<>'cash' AND paid_at IS NOT NULL").Count(&wrong).Error)
	if wrong != 0 {
		t.Fatal("migration paid other methods")
	}
	// A failed confirmation must leave the cash draft unpaid.
	broken := create("cash")
	must(repo.db.Exec("DELETE FROM purchase_slip_lines WHERE purchase_slip_id=?", broken.ID).Error)
	if _, e = repo.ConfirmPurchase(ctx, org, broken.ID, admin); e == nil {
		t.Fatal("invalid confirmation succeeded")
	}
	failed, e := repo.PurchaseSlip(ctx, org, broken.ID)
	must(e)
	if failed.PaidAt != nil || failed.Status != "draft" {
		t.Fatal("failed confirmation marked paid")
	}
}
