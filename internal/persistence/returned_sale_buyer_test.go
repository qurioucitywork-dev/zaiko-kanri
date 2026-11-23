package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"os"
	"strings"
	"testing"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
)

func TestReturnedSaleBuyerLockedPostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_RESUBMIT_NOCHANGE_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	if !strings.Contains(dsn, "/zaiko_resubmit_nochange_test") {
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
	must(repo.SeedPreviewIdentity(ctx, "weight-test-admin-password", "weight-test-worker-password"))
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
	must(repo.db.Exec("INSERT INTO exchange_rate_snapshots(id,organization_id,base_currency,quote_currency,rate_scaled,scale,provider,created_by,observed_at,created_at) VALUES('weight-rate',?,'USD','JPY',15500,100,'test',?,now(),now())", org, admin).Error)
	create := func() Product {
		t.Helper()
		v, e := repo.CreateSingleProduct(ctx, SingleProductInput{OrganizationID: org, ActorUserID: admin, PurchaseDate: "2026-09-19", SupplierCode: supplier, StaffCode: "BUY-001", BrandCode: brand, CostAmountMinor: 1000, CostCurrency: "JPY", BaseSaleCurrency: "USD", BaseSalePriceMinor: 1000, WeightGrams: "999"})
		must(e)
		must(repo.db.Table("products").Where("id=?", v.Product.ID).Update("inventory_status", "in_stock").Error)
		return v.Product
	}

	var worker string
	must(repo.db.Raw("SELECT id FROM users WHERE username='worker'").Scan(&worker).Error)

	for _, state := range []string{"shipped", "reserved", "consigned"} {
		t.Run(state, func(t *testing.T) {
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			p1, p2, foreign := create(), create(), create()
			if state == "shipped" {
				s, e := repo.CreateShipment(ctx, ShipmentCreateInput{OrganizationID: org, ActorUserID: admin, ShipmentDate: "2026-09-19", BuyerCode: "B002", DisplayCurrency: "JPY", ProductCodes: []string{p1.ProductCode, p2.ProductCode}})
				must(e)
				_, e = repo.ConfirmShipment(ctx, org, s.ID, admin)
				must(e)
				f, e := repo.CreateShipment(ctx, ShipmentCreateInput{OrganizationID: org, ActorUserID: admin, ShipmentDate: "2026-09-19", BuyerCode: "B001", DisplayCurrency: "JPY", ProductCodes: []string{foreign.ProductCode}})
				must(e)
				_, e = repo.ConfirmShipment(ctx, org, f.ID, admin)
				must(e)
			} else if state == "consigned" {
				_, e := repo.CreateConsignment(ctx, ConsignmentCreateInput{OrganizationID: org, ActorUserID: admin, ConsignmentDate: "2026-09-19", ConsigneeCode: "B002", ProductCodes: []string{p1.ProductCode, p2.ProductCode}})
				must(e)
			}
			if state == "consigned" {
				_, e := repo.CreateConsignment(ctx, ConsignmentCreateInput{OrganizationID: org, ActorUserID: admin, ConsignmentDate: "2026-09-19", ConsigneeCode: "B001", ProductCodes: []string{foreign.ProductCode}})
				must(e)
			}
			sale, e := repo.CreateSale(ctx, SaleCreateInput{OrganizationID: org, ActorUserID: worker, SaleDate: "2026-09-19", BuyerCode: "B002", DisplayCurrency: "JPY", TaxMode: "taxable", TaxRateBasisPoints: 1000, Lines: []SaleLineInput{{ProductCode: p1.ProductCode, UnitPriceMinor: 992000}, {ProductCode: p2.ProductCode, UnitPriceMinor: 397000}}})
			must(e)
			if state == "reserved" {
				_, e = repo.CreateSale(ctx, SaleCreateInput{OrganizationID: org, ActorUserID: worker, SaleDate: "2026-09-19", BuyerCode: "B001", DisplayCurrency: "JPY", TaxMode: "taxable", Lines: []SaleLineInput{{ProductCode: foreign.ProductCode, UnitPriceMinor: 1000}}})
				must(e)
			}
			a, e := repo.CreateApproval(ctx, org, worker, ApprovalCreateInput{ApprovalType: "売上伝票の確定", TargetType: "sales_slip", TargetID: sale.ID, RequestedAction: "sale.confirm"})
			must(e)
			a, e = repo.DecideApproval(ctx, org, a.ID, admin, "returned", "修正してください")
			must(e)
			loaded, e := repo.ReturnedApprovalDocument(ctx, org, worker, a.ID)
			must(e)
			original := loaded.Document.(SaleSlipRecord)
			replacement := SaleCreateInput{SaleDate: "2026-09-20", BuyerCode: "B001", DisplayCurrency: "USD", TaxMode: "exempt", Notes: "明細削除・販売先・通貨・金額を修正", Lines: []SaleLineInput{{ProductCode: p2.ProductCode, UnitPriceMinor: 2500}}}
			input := ApprovalResubmitInput{ExpectedApprovalUpdatedAt: a.UpdatedAt, ExpectedDocumentUpdatedAt: original.UpdatedAt, Rebuild: &ReturnedDraftInput{Reason: "修正テスト", OriginalLineIDs: []string{sale.Lines[1].ID}, Sale: &replacement}}
			_, e = repo.ResubmitApproval(ctx, org, worker, a.ID, input)
			if !errors.Is(e, ErrSaleBuyerLocked) {
				t.Fatalf("buyer change not rejected: %v", e)
			}
			// The old revision payload is subject to the same rule.
			_, e = repo.ResubmitApproval(ctx, org, worker, a.ID, ApprovalResubmitInput{ExpectedApprovalUpdatedAt: a.UpdatedAt, ExpectedDocumentUpdatedAt: original.UpdatedAt, Sale: &SaleRevisionInput{BuyerCode: "B001"}})
			if !errors.Is(e, ErrSaleBuyerLocked) {
				t.Fatalf("legacy buyer change not rejected: %v", e)
			}
			replacement.BuyerCode = "B002"
			// New products still obey their current buyer reservation; a retained line ID is no bypass.
			replacement.Lines[0].ProductCode = foreign.ProductCode
			_, e = repo.ResubmitApproval(ctx, org, worker, a.ID, input)
			if e == nil {
				t.Fatal("unrelated reserved product was allowed")
			}
			replacement.Lines[0].ProductCode = p2.ProductCode
			saved, e := repo.ResubmitApproval(ctx, org, worker, a.ID, input)
			must(e)
			if saved.ID != a.ID || saved.Status != "pending" {
				t.Fatal("approval identity/status changed")
			}
			revised, e := repo.SaleSlip(ctx, org, sale.ID)
			must(e)
			if revised.SlipNumber != sale.SlipNumber || revised.BuyerCode != "B002" || revised.DisplayCurrency != "USD" || revised.TaxMode != "out_of_scope" || len(revised.Lines) != 1 || revised.Lines[0].UnitPriceMinor != 2500 || revised.Lines[0].ProductCode != p2.ProductCode {
				t.Fatalf("changes lost: %+v", revised)
			}
			var status string
			must(repo.db.Table("products").Select("inventory_status").Where("id=?", p2.ID).Scan(&status).Error)
			if status != state {
				t.Fatalf("inventory changed before approval: %s", status)
			}
			_, e = repo.DecideApproval(ctx, org, a.ID, admin, "approved", "確認済み")
			must(e)
			must(repo.db.Table("products").Select("inventory_status").Where("id=?", p2.ID).Scan(&status).Error)
			if status != "sold" {
				t.Fatalf("not sold on approval: %s", status)
			}
			must(repo.db.Table("products").Select("inventory_status").Where("id=?", p1.ID).Scan(&status).Error)
			raw, _ := json.Marshal(SaleRevisionInput{SaleDate: string(revised.SaleDate), BuyerCode: "B002", Notes: "再修正", Reason: "修正", Lines: []SaleRevisionLine{{ID: revised.Lines[0].ID, UnitPriceMinor: 2500}}})
			change := DocumentChangeInput{Kind: "sale.revision", TargetID: sale.ID, Payload: raw}
			revision, e := repo.RequestDocumentChange(ctx, org, worker, change, "", time.Time{})
			must(e)
			revision, e = repo.DecideApproval(ctx, org, revision.ID, admin, "returned", "修正してください")
			must(e)
			var payload map[string]any
			must(json.Unmarshal(raw, &payload))
			payload["buyerCode"] = "B001"
			change.Payload, e = json.Marshal(payload)
			must(e)
			_, e = repo.RequestDocumentChange(ctx, org, worker, change, revision.ID, revision.UpdatedAt)
			if !errors.Is(e, ErrSaleBuyerLocked) {
				t.Fatalf("confirmed revision buyer change not rejected: %v", e)
			}
			change.Payload = raw
			_, e = repo.RequestDocumentChange(ctx, org, worker, change, revision.ID, revision.UpdatedAt)
			must(e)
			want := state
			if state == "reserved" {
				want = "in_stock"
			}
			if status != want {
				t.Fatalf("removed product status %s", status)
			}
		})
	}
}
