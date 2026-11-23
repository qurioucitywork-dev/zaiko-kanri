package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
)

func TestReturnedRevisionNoChangePostgres(t *testing.T) {
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
		v, e := repo.CreateSingleProduct(ctx, SingleProductInput{OrganizationID: org, ActorUserID: admin, PurchaseDate: "2026-09-19", SupplierCode: supplier, StaffCode: "BUY-001", BrandCode: brand, CostAmountMinor: 1000, CostCurrency: "JPY", BaseSaleCurrency: "USD", WeightGrams: "999"})
		must(e)
		must(repo.db.Table("products").Where("id=?", v.Product.ID).Update("inventory_status", "in_stock").Error)
		return v.Product
	}

	var worker string
	must(repo.db.Raw("SELECT id FROM users WHERE username='worker'").Scan(&worker).Error)
	type fixture struct {
		kind, id string
		original map[string]any
	}
	var fixtures []fixture
	purchase, e := repo.CreatePurchase(ctx, PurchaseCreateInput{OrganizationID: org, ActorUserID: admin, PurchaseDate: "2026-09-19", SupplierCode: supplier, StaffCode: "BUY-001", PaymentMethod: "bank_transfer", Lines: []PurchaseLineInput{{Quantity: 1, BrandCode: brand, UnitCostMinor: 1000, CostCurrency: "JPY", BaseSaleCurrency: "USD"}}})
	must(e)
	purchase, e = repo.ConfirmPurchase(ctx, org, purchase.ID, admin)
	must(e)
	fixtures = append(fixtures, fixture{"purchase.revision", purchase.ID, map[string]any{"purchaseDate": purchase.PurchaseDate, "supplierCode": supplier, "staffCode": "BUY-001", "notes": purchase.Notes, "lines": []map[string]any{{"id": purchase.Lines[0].ID, "sku": purchase.Lines[0].SKU, "unitCostMinor": purchase.Lines[0].UnitCostMinor, "baseSalePriceMinor": purchase.Lines[0].BaseSalePriceMinor}}}})
	product := create()
	sale, e := repo.CreateSale(ctx, SaleCreateInput{OrganizationID: org, ActorUserID: admin, SaleDate: "2026-09-19", BuyerCode: "B001", DisplayCurrency: "USD", TaxMode: "out_of_scope", Lines: []SaleLineInput{{ProductCode: product.ProductCode, UnitPriceMinor: 100}}})
	must(e)
	sale, e = repo.ConfirmSale(ctx, org, sale.ID, admin)
	must(e)
	fixtures = append(fixtures, fixture{"sale.revision", sale.ID, map[string]any{"saleDate": sale.SaleDate, "buyerCode": "B001", "notes": sale.Notes, "lines": []map[string]any{{"id": sale.Lines[0].ID, "unitPriceMinor": sale.Lines[0].UnitPriceMinor}}}})
	product = create()
	price := int64(100)
	shipment, e := repo.CreateShipment(ctx, ShipmentCreateInput{OrganizationID: org, ActorUserID: admin, ShipmentDate: "2026-09-19", BuyerCode: "B001", DisplayCurrency: "USD", ProductCodes: []string{product.ProductCode}, Items: []ShipmentPriceInput{{ProductCode: product.ProductCode, SalePriceUSDMinor: &price}}})
	must(e)
	shipment, e = repo.ConfirmShipment(ctx, org, shipment.ID, admin)
	must(e)
	fixtures = append(fixtures, fixture{"shipment.revision", shipment.ID, map[string]any{"shipmentDate": shipment.ShipmentDate, "buyerCode": "B001", "notes": shipment.Notes, "expectedUpdatedAt": shipment.UpdatedAt, "lines": []map[string]any{{"id": shipment.Lines[0].ID, "unitPriceMinor": shipment.Lines[0].SalePriceUSDMinor}}}})
	product = create()
	consignment, e := repo.CreateConsignment(ctx, ConsignmentCreateInput{OrganizationID: org, ActorUserID: admin, ConsignmentDate: "2026-09-19", ConsigneeCode: "B001", ProductCodes: []string{product.ProductCode}})
	must(e)
	fixtures = append(fixtures, fixture{"consignment.revision", consignment.ID, map[string]any{"consignmentDate": consignment.ConsignmentDate, "consigneeCode": "B001", "notes": consignment.Notes, "expectedUpdatedAt": consignment.UpdatedAt, "lines": []map[string]any{{"id": consignment.Lines[0].ID, "unitPriceMinor": consignment.Lines[0].ConvertedSalePriceJPY}}}})
	for _, f := range fixtures {
		t.Run(f.kind, func(t *testing.T) {
			f.original["reason"] = "差戻し指摘に従い元の内容で再申請"
			raw, e := json.Marshal(f.original)
			must(e)
			v := DocumentChangeInput{Kind: f.kind, TargetID: f.id, Payload: raw}
			before, updated, e := repo.changeDocument(ctx, org, v)
			must(e)
			businessJSON := func(document any) string {
				t.Helper()
				raw, err := json.Marshal(document)
				must(err)
				var fields map[string]any
				must(json.Unmarshal(raw, &fields))
				// Approval adds audit history, without changing the document itself.
				delete(fields, "revisions")
				raw, err = json.Marshal(fields)
				must(err)
				return string(raw)
			}
			beforeJSON := businessJSON(before)
			_, e = repo.RequestDocumentChange(ctx, org, worker, v, "", time.Time{})
			if !errors.Is(e, ErrSlipRevisionUnchanged) {
				t.Fatalf("initial no-op was allowed: %v", e)
			}
			proposal := map[string]any{}
			must(json.Unmarshal(raw, &proposal))
			proposal["notes"] = "訂正案"
			v.Payload, e = json.Marshal(proposal)
			must(e)
			a, e := repo.RequestDocumentChange(ctx, org, worker, v, "", time.Time{})
			must(e)
			a, e = repo.DecideApproval(ctx, org, a.ID, admin, "returned", "元に戻してください")
			must(e)
			v.Payload = raw
			_, e = repo.RequestDocumentChange(ctx, org, admin, v, a.ID, a.UpdatedAt)
			if e == nil {
				t.Fatal("wrong owner allowed")
			}
			_, e = repo.RequestDocumentChange(ctx, org, worker, v, a.ID, a.UpdatedAt.Add(-time.Second))
			if e == nil {
				t.Fatal("stale approval allowed")
			}
			saved, e := repo.RequestDocumentChange(ctx, org, worker, v, a.ID, a.UpdatedAt)
			must(e)
			if saved.ID != a.ID || saved.Status != "pending" {
				t.Fatal("request identity/status changed")
			}
			var snap DocumentChangeSnapshot
			must(json.Unmarshal([]byte(saved.SnapshotJSON), &snap))
			if !snap.UnchangedRevision {
				t.Fatal("no-op flag missing")
			}
			if !snap.ExpectedUpdatedAt.Equal(updated) {
				t.Fatal("document modified during resubmit")
			}
			approved, e := repo.DecideApproval(ctx, org, a.ID, admin, "approved", "確認済み")
			must(e)
			if approved.Status != "approved" {
				t.Fatal("not approved")
			}
			after, _, e := repo.changeDocument(ctx, org, v)
			must(e)
			afterJSON := businessJSON(after)
			if string(beforeJSON) != string(afterJSON) {
				t.Fatalf("no-op approval altered document: before=%s after=%s", beforeJSON, afterJSON)
			}
		})
	}
}
