package persistence

import (
	"context"
	"errors"
	"net/url"
	"os"
	"testing"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/config"
	"gorm.io/gorm"
)

func TestAuctionFeesPostgres(t *testing.T) {
	dsn := os.Getenv("ZAIKO_AUCTION_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Path != "/zaiko_auction_fee_test" {
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
	must(repo.SeedPreviewIdentity(ctx, "auction-test-admin-password", "auction-test-worker-password"))
	must(repo.SeedPreviewMasters(ctx))
	var org, admin, brand string
	for _, q := range []struct {
		sql   string
		value *string
	}{{"SELECT id FROM organizations WHERE code='PREVIEW'", &org}, {"SELECT id FROM users WHERE username='admin' LIMIT 1", &admin}, {"SELECT code FROM brands WHERE is_active ORDER BY code LIMIT 1", &brand}} {
		must(repo.db.Raw(q.sql).Scan(q.value).Error)
	}
	must(repo.db.Exec(`INSERT INTO exchange_rate_snapshots(id,organization_id,base_currency,quote_currency,rate_scaled,scale,provider,created_by,observed_at,created_at) VALUES('auction-fee-test-rate',?,'USD','JPY',15500,100,'test',?,now(),now()) ON CONFLICT(id) DO NOTHING`, org, admin).Error)
	settings := AuctionFeeSettings{IsAuction: true, Purchase: AuctionFeeRule{Mode: "tiers", Tiers: []AuctionFeeTier{{From: 0, To: auctionPtr(100000), RateBasisPoints: 1000}, {From: 100000, RateBasisPoints: 500}}}, Sales: AuctionFeeRule{Mode: "fixed", FixedAmount: 550}}
	partner, err := repo.CreatePartner(ctx, org, admin, PartnerCreateInput{LegalName: "手数料テストオークション", Address: "テスト専用住所", Roles: []PartnerRoleInput{{RoleType: "supplier"}, {RoleType: "buyer"}}, AuctionFees: settings})
	must(err)
	supplier, buyer := "", ""
	for _, role := range partner.Roles {
		if role.RoleType == "supplier" {
			supplier = role.RoleCode
		} else {
			buyer = role.RoleCode
		}
	}
	reloaded, err := repo.Partner(ctx, org, partner.ID)
	must(err)
	if reloaded.AuctionFees.Sales.FixedAmount != 550 || reloaded.AuctionFees.Purchase.Tiers[0].To == nil {
		t.Fatal("master round-trip failed")
	}
	makePurchase := func(rule *AuctionFeeRule, currency string, amounts ...int64) (PurchaseSlipRecord, error) {
		lines := []PurchaseLineInput{}
		for _, amount := range amounts {
			lines = append(lines, PurchaseLineInput{Quantity: 1, BrandCode: brand, UnitCostMinor: amount, CostCurrency: currency, BaseSaleCurrency: "USD", BaseSalePriceMinor: 1000})
		}
		return repo.CreatePurchase(ctx, PurchaseCreateInput{OrganizationID: org, ActorUserID: admin, StaffCode: "BUY-000", SupplierCode: supplier, PurchaseDate: "2026-10-11", AuctionFeeRule: rule, Lines: lines})
	}
	purchase, err := makePurchase(&settings.Purchase, "JPY", 99999, 100000)
	must(err)
	if purchase.Lines[0].UnitCostMinor != 109998 || purchase.Lines[1].UnitCostMinor != 105000 || purchase.Lines[0].AuctionFee.FeeMinor != 9999 {
		t.Fatalf("purchase %+v", purchase.Lines)
	}
	purchase, err = repo.ConfirmPurchase(ctx, org, purchase.ID, admin)
	must(err)
	for _, line := range purchase.Lines {
		_, err = repo.ReceivePurchaseProduct(ctx, org, purchase.ID, line.ProductCode, admin)
		must(err)
	}
	product, err := repo.ProductByID(ctx, org, purchase.Lines[0].ProductID)
	must(err)
	if product.CostAmountMinor != 109998 {
		t.Fatal("fee not included in inventory cost", product.CostAmountMinor)
	}
	off, err := makePurchase(nil, "JPY", 100000)
	must(err)
	if off.Lines[0].UnitCostMinor != 100000 || off.Lines[0].AuctionFee.Rule != nil {
		t.Fatal("OFF changed cost")
	}
	foreignOff, err := makePurchase(nil, "USD", 1000)
	must(err)
	if foreignOff.Lines[0].UnitCostMinor != 1000 {
		t.Fatal("foreign OFF changed cost")
	}
	if _, err = makePurchase(&settings.Purchase, "USD", 1000); !errors.Is(err, ErrAuctionFee) {
		t.Fatal("foreign fee accepted", err)
	}
	makeSale := func(code string, amount int64, rule *AuctionFeeRule, currency string) (SaleSlipRecord, error) {
		return repo.CreateSale(ctx, SaleCreateInput{OrganizationID: org, ActorUserID: admin, BuyerCode: buyer, SaleDate: "2026-10-11", DisplayCurrency: currency, TaxMode: "taxable", TaxRateBasisPoints: 1000, AuctionFeeRule: rule, Lines: []SaleLineInput{{ProductCode: code, UnitPriceMinor: amount}}})
	}
	sale, err := makeSale(product.ProductCode, 100019, &settings.Sales, "JPY")
	must(err)
	if sale.Lines[0].UnitPriceMinor != 99469 || sale.Lines[0].TaxAmountMinor != 9947 || sale.TotalMinor != 109416 || sale.Lines[0].AuctionFee.BaseAmountMinor != 100019 {
		t.Fatalf("sale %+v", sale)
	}
	sale, err = repo.ConfirmSale(ctx, org, sale.ID, admin)
	must(err)
	// Updating master data never recomputes registered records or inventory.
	changed := settings
	changed.Sales = AuctionFeeRule{Mode: "rate", RateBasisPoints: 100}
	changed.Purchase = AuctionFeeRule{Mode: "fixed", FixedAmount: 5000}
	_, err = repo.UpdatePartner(ctx, org, partner.ID, admin, PartnerUpdateInput{AuctionFees: &changed})
	must(err)
	savedSale, err := repo.SaleSlip(ctx, org, sale.ID)
	must(err)
	if savedSale.Lines[0].UnitPriceMinor != 99469 || savedSale.Lines[0].AuctionFee.Rule.FixedAmount != 550 {
		t.Fatal("historical rule changed")
	}
	if _, err = makePurchase(&settings.Purchase, "JPY", 1000); !errors.Is(err, ErrAuctionFeeChanged) {
		t.Fatal("stale master accepted", err)
	}
	_, err = repo.UpdatePartner(ctx, org, partner.ID, admin, PartnerUpdateInput{AuctionFees: &settings})
	must(err)
	// An excessive sales fee rolls back the entire draft and inventory reservation.
	secondCode := purchase.Lines[1].ProductCode
	if _, err = makeSale(secondCode, 100, &settings.Sales, "JPY"); !errors.Is(err, ErrAuctionFee) {
		t.Fatal("negative sales amount accepted", err)
	}
	var draftCount int64
	must(repo.db.Table("sales_lines").Where("product_id=?", purchase.Lines[1].ProductID).Count(&draftCount).Error)
	if draftCount != 0 {
		t.Fatal("failed registration leaked a line")
	}
	zeroSale, err := makeSale(secondCode, 550, &settings.Sales, "JPY")
	must(err)
	if zeroSale.Lines[0].UnitPriceMinor != 0 {
		t.Fatal("zero net wrong")
	}
	// Rebuilding returned drafts edits already-adjusted prices and must never charge twice.
	must(repo.db.Transaction(func(tx *gorm.DB) error {
		txRepo := &Repository{db: tx, driver: "postgres"}
		before := ReturnedApprovalDocument{Approval: ApprovalRequestRecord{TargetID: zeroSale.ID}, Document: zeroSale}
		after, e := txRepo.rebuildReturnedDraft(ctx, org, admin, before, ReturnedDraftInput{Reason: "テスト差戻し", OriginalLineIDs: []string{zeroSale.Lines[0].ID}, Sale: &SaleCreateInput{BuyerCode: buyer, SaleDate: "2026-10-11", DisplayCurrency: "JPY", TaxMode: "taxable", TaxRateBasisPoints: 1000, Lines: []SaleLineInput{{ProductCode: secondCode, UnitPriceMinor: 0}}}})
		if e != nil {
			return e
		}
		row := after.(SaleSlipRecord).Lines[0]
		if row.UnitPriceMinor != 0 || row.AuctionFee.FeeMinor != 550 {
			t.Fatal("resubmission changed effective amount or lost snapshot", row)
		}
		return nil
	}))
	// Mixed price-band units persist and flow through authoritative purchase/sales registration.
	mixed := AuctionFeeSettings{IsAuction: true,
		Purchase: AuctionFeeRule{Mode: "tiers", Tiers: []AuctionFeeTier{{From: 0, To: auctionPtr(100000), RateBasisPoints: 333}, {From: 100000, Mode: "fixed", FixedAmount: 550}}},
		Sales:    AuctionFeeRule{Mode: "tiers", Tiers: []AuctionFeeTier{{From: 0, To: auctionPtr(100000), Mode: "fixed", FixedAmount: 100}, {From: 100000, RateBasisPoints: 250}}}}
	_, err = repo.UpdatePartner(ctx, org, partner.ID, admin, PartnerUpdateInput{AuctionFees: &mixed})
	must(err)
	reloaded, err = repo.Partner(ctx, org, partner.ID)
	must(err)
	if reloaded.AuctionFees.Purchase.Tiers[1].Mode != "fixed" || reloaded.AuctionFees.Purchase.Tiers[1].FixedAmount != 550 {
		t.Fatal("mixed master not retained")
	}
	mixedPurchase, err := makePurchase(&mixed.Purchase, "JPY", 99999, 100000)
	must(err)
	if mixedPurchase.Lines[0].UnitCostMinor != 103328 || mixedPurchase.Lines[1].UnitCostMinor != 100550 {
		t.Fatal("mixed purchase amounts wrong", mixedPurchase.Lines)
	}
	mixedPurchase, err = repo.ConfirmPurchase(ctx, org, mixedPurchase.ID, admin)
	must(err)
	for i, line := range mixedPurchase.Lines {
		_, err = repo.ReceivePurchaseProduct(ctx, org, mixedPurchase.ID, line.ProductCode, admin)
		must(err)
		product, err = repo.ProductByID(ctx, org, line.ProductID)
		must(err)
		if product.CostAmountMinor != []int64{103328, 100550}[i] {
			t.Fatal("mixed fee inventory cost wrong")
		}
		sold, err := makeSale(line.ProductCode, []int64{99999, 100000}[i], &mixed.Sales, "JPY")
		must(err)
		if sold.Lines[0].UnitPriceMinor != []int64{99899, 97500}[i] || sold.Lines[0].AuctionFee.FeeMinor != []int64{100, 2500}[i] {
			t.Fatal("mixed sale amounts wrong", sold.Lines)
		}
		loaded, err := repo.SaleSlip(ctx, org, sold.ID)
		must(err)
		if loaded.Lines[0].AuctionFee.Rule.Tiers[0].FixedAmount != 100 {
			t.Fatal("mixed fee snapshot lost")
		}
	}
	if _, err = makePurchase(&settings.Purchase, "JPY", 100000); !errors.Is(err, ErrAuctionFeeChanged) {
		t.Fatal("stale percentage-only master accepted", err)
	}
	// Existing fee-free companies remain usable and cannot request another company's rule.
	ordinary, err := repo.CreatePartner(ctx, org, admin, PartnerCreateInput{LegalName: "通常会社テスト", Address: "テスト住所", Roles: []PartnerRoleInput{{RoleType: "supplier"}}})
	must(err)
	supplier = ordinary.Roles[0].RoleCode
	if _, err = makePurchase(&settings.Purchase, "JPY", 1000); !errors.Is(err, ErrAuctionFeeChanged) {
		t.Fatal("non-auction company applied fee", err)
	}
	ordinaryPurchase, err := makePurchase(nil, "JPY", 1000)
	must(err)
	if ordinaryPurchase.Lines[0].UnitCostMinor != 1000 {
		t.Fatal("ordinary company affected")
	}
}
