package database

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func salesTestProduct(t *testing.T, store *Store) Product {
	t.Helper()
	ctx := context.Background()
	if err := store.SeedInventoryPreview(ctx); err != nil {
		t.Fatal(err)
	}
	products, err := store.Products(ctx, "org_preview", ProductFilter{})
	if err != nil || len(products) == 0 {
		t.Fatalf("seed product: %v, products=%d", err, len(products))
	}
	return products[0]
}

func createTestSale(t *testing.T, store *Store, productID string, quantity int, currency string, price int64) SalesSlip {
	t.Helper()
	sale, err := store.CreateSaleDraft(context.Background(), CreateSaleInput{
		OrganizationID: "org_preview", SalesDate: "2026-07-26", CustomerName: "テスト販売先",
		CreatedBy: "usr_admin", Lines: []SalesLineInput{{
			ProductID: productID, Quantity: quantity, UnitPriceMinor: price, Currency: currency,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return sale
}

func TestCreateSaleDraftUsesOptionalValidatedSlipNumber(t *testing.T) {
	store := testStore(t)
	product := salesTestProduct(t, store)
	ctx := context.Background()
	base := CreateSaleInput{
		OrganizationID: "org_preview",
		SalesDate:      "2026-07-26",
		CustomerName:   "テスト販売先",
		CreatedBy:      "usr_admin",
		Lines: []SalesLineInput{{
			ProductID: product.ID, Quantity: 1, UnitPriceMinor: 120000, Currency: "JPY",
		}},
	}

	custom := base
	custom.SlipNumber = "SL-2026-0002"
	created, err := store.CreateSaleDraft(ctx, custom)
	if err != nil {
		t.Fatal(err)
	}
	if created.SlipNumber != custom.SlipNumber {
		t.Fatalf("slip number=%q want=%q", created.SlipNumber, custom.SlipNumber)
	}

	invalid := base
	invalid.SlipNumber = "sales-99"
	if _, err := store.CreateSaleDraft(ctx, invalid); err == nil ||
		!strings.Contains(err.Error(), "SL-YYYY-NNNN") {
		t.Fatalf("invalid slip number error=%v", err)
	}

	if _, err := store.CreateSaleDraft(ctx, custom); err == nil ||
		!strings.Contains(err.Error(), "既に登録") {
		t.Fatalf("duplicate slip number error=%v", err)
	}

	automatic, err := store.CreateSaleDraft(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	if !salesSlipNumberPattern.MatchString(automatic.SlipNumber) {
		t.Fatalf("automatic slip number=%q", automatic.SlipNumber)
	}
	if automatic.SlipNumber != "SL-2026-0003" {
		t.Fatalf("automatic slip number=%q want=%q after manually reserving 0002", automatic.SlipNumber, "SL-2026-0003")
	}
}

func createTestShipment(t *testing.T, store *Store, productID string, quantity int) ShipmentSlip {
	t.Helper()
	shipment, err := store.CreateShipmentDraft(context.Background(), CreateShipmentInput{
		OrganizationID: "org_preview", ShipmentDate: "2026-07-26", RecipientName: "テスト届け先",
		CreatedBy: "usr_admin", Lines: []ShipmentLineInput{{ProductID: productID, Quantity: quantity}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return shipment
}

func TestCreateShipmentDraftStoresMockWholesalePrice(t *testing.T) {
	store := testStore(t)
	product := salesTestProduct(t, store)
	shipment, err := store.CreateShipmentDraft(context.Background(), CreateShipmentInput{
		OrganizationID: "org_preview", ShipmentDate: "2026-07-26", RecipientName: "クロノス東京",
		CreatedBy: "usr_admin", Lines: []ShipmentLineInput{{
			ProductID: product.ID, Quantity: 1, WholesalePriceMinor: 680000,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(shipment.Lines) != 1 || shipment.Lines[0].WholesalePriceMinor != 680000 || shipment.TotalJPY != 680000 {
		t.Fatalf("shipment wholesale price was not preserved: %+v", shipment)
	}
}

func TestShipmentNumberImportCreatesExclusiveSaleWithNewSalesNumber(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	product := salesTestProduct(t, store)
	shipment, err := store.CreateShipmentDraft(ctx, CreateShipmentInput{
		OrganizationID: "org_preview", ShipmentDate: "2026-07-31",
		RecipientName: "クロノス東京", CreatedBy: "usr_admin",
		Lines: []ShipmentLineInput{{
			ProductID: product.ID, Quantity: 1, WholesalePriceMinor: 680000,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	shipment, err = store.ConfirmShipment(ctx, "org_preview", shipment.ID, "usr_admin")
	if err != nil {
		t.Fatal(err)
	}
	byNumber, err := store.ShipmentByNumber(ctx, "org_preview", shipment.ShipmentNumber)
	if err != nil || byNumber.ID != shipment.ID {
		t.Fatalf("lookup=%+v err=%v", byNumber, err)
	}
	sale, err := store.CreateSaleDraft(ctx, CreateSaleInput{
		OrganizationID: "org_preview", SlipNumber: shipment.ShipmentNumber,
		SourceShipmentID: shipment.ID, SalesDate: "2026-07-31",
		CustomerName: "改ざんされた販売先", CreatedBy: "usr_admin",
		Lines: []SalesLineInput{{
			ProductID: product.ID, Quantity: 99, UnitPriceMinor: 1, Currency: "JPY",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if sale.SlipNumber == shipment.ShipmentNumber || !salesSlipNumberPattern.MatchString(sale.SlipNumber) {
		t.Fatalf("shipment number was not replaced by sales number: %+v", sale)
	}
	if sale.CustomerName != shipment.RecipientName || len(sale.Lines) != 1 ||
		sale.Lines[0].Quantity != 1 || sale.Lines[0].UnitPriceMinor != 680000 {
		t.Fatalf("sale was not derived from shipment database values: %+v", sale)
	}
	if _, err := store.CreateSaleDraft(ctx, CreateSaleInput{
		OrganizationID: "org_preview", SourceShipmentID: shipment.ID,
		SalesDate: "2026-07-31", CreatedBy: "usr_admin",
	}); !errors.Is(err, ErrShipmentAlreadyUsed) {
		t.Fatalf("duplicate shipment import error=%v", err)
	}
	confirmed, err := store.ConfirmSale(ctx, "org_preview", sale.ID, "usr_admin")
	if err != nil || confirmed.ShipmentStatus != "complete" {
		t.Fatalf("confirm=%+v err=%v", confirmed, err)
	}
	linked, err := store.ShipmentByNumber(ctx, "org_preview", shipment.ShipmentNumber)
	if err != nil || linked.LinkedSalesSlipID != sale.ID ||
		linked.Lines[0].SalesSlipNumber != sale.SlipNumber {
		t.Fatalf("linked shipment=%+v err=%v", linked, err)
	}
	// Databases upgraded from the legacy allocation model may temporarily have
	// an allocation without the new shipment header link. The allocation must
	// still make the shipment unavailable for another sale.
	if _, err := store.db.ExecContext(ctx,
		`UPDATE shipment_slips SET sales_slip_id=NULL WHERE id=?`, shipment.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateSaleDraft(ctx, CreateSaleInput{
		OrganizationID: "org_preview", SourceShipmentID: shipment.ID,
		SalesDate: "2026-07-31", CreatedBy: "usr_admin",
	}); !errors.Is(err, ErrShipmentAlreadyUsed) {
		t.Fatalf("legacy allocated shipment import error=%v", err)
	}
}

func TestSaleCanConfirmBeforeShipmentAndLocksRateSnapshot(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	product := salesTestProduct(t, store)
	firstRate, err := store.AddExchangeRate(ctx, "org_preview", "USD", "JPY", 150*RateScale, "test", "2026-07-26T09:00", "usr_admin")
	if err != nil {
		t.Fatal(err)
	}
	sale := createTestSale(t, store, product.ID, 1, "USD", 1000)
	confirmed, err := store.ConfirmSale(ctx, "org_preview", sale.ID, "usr_admin")
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.Status != "confirmed" || confirmed.ShipmentStatus != "unshipped" || confirmed.Warning == "" {
		t.Fatalf("unexpected sale state: %+v", confirmed)
	}
	line := confirmed.Lines[0]
	if line.ExchangeRateSnapshotID != firstRate.ID || line.ConvertedTotalJPY != 150000 {
		t.Fatalf("rate snapshot not fixed: %+v", line)
	}
	if _, err := store.AddExchangeRate(ctx, "org_preview", "USD", "JPY", 160*RateScale, "test", "2026-07-26T10:00", "usr_admin"); err != nil {
		t.Fatal(err)
	}
	after, err := store.Sale(ctx, "org_preview", sale.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Lines[0].ExchangeRateSnapshotID != firstRate.ID || after.Lines[0].ConvertedTotalJPY != 150000 {
		t.Fatalf("confirmed sale changed after later rate: %+v", after.Lines[0])
	}
	productAfter, err := store.Product(ctx, "org_preview", product.ID)
	if err != nil || productAfter.InventoryStatus != "sold" {
		t.Fatalf("product should be sold: status=%s err=%v", productAfter.InventoryStatus, err)
	}
}

func TestShipmentCanConfirmBeforeSaleAndIsLaterAllocated(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	product := salesTestProduct(t, store)
	shipment := createTestShipment(t, store, product.ID, 1)
	confirmedShipment, err := store.ConfirmShipment(ctx, "org_preview", shipment.ID, "usr_admin")
	if err != nil {
		t.Fatal(err)
	}
	if confirmedShipment.Warning == "" || confirmedShipment.Lines[0].SalesLineID != "" {
		t.Fatalf("shipment-before-sale warning missing: %+v", confirmedShipment)
	}
	sale := createTestSale(t, store, product.ID, 1, "JPY", 1200000)
	confirmedSale, err := store.ConfirmSale(ctx, "org_preview", sale.ID, "usr_admin")
	if err != nil {
		t.Fatal(err)
	}
	if confirmedSale.ShipmentStatus != "complete" || confirmedSale.Lines[0].RemainingQuantity != 0 {
		t.Fatalf("shipment was not allocated: %+v", confirmedSale)
	}
	linkedShipment, err := store.Shipment(ctx, "org_preview", shipment.ID)
	if err != nil || linkedShipment.Warning != "" || linkedShipment.Lines[0].SalesSlipNumber != confirmedSale.SlipNumber {
		t.Fatalf("shipment link incorrect: %+v err=%v", linkedShipment, err)
	}
}

func TestMultipleProductsShipBeforeSaleAndAllocateToOneSale(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	if err := store.SeedInventoryPreview(ctx); err != nil {
		t.Fatal(err)
	}
	products, err := store.Products(ctx, "org_preview", ProductFilter{})
	if err != nil || len(products) == 0 {
		t.Fatalf("products=%d err=%v", len(products), err)
	}
	first := products[0]
	second, err := store.CreateSingleProduct(ctx, SingleProductInput{
		OrganizationID: "org_preview", SupplierID: "sup_001", PurchaseDate: "2026-07-30",
		SKU: "PHASE5-MULTI-002", Brand: "オメガ", ModelNumber: "PHASE5-002",
		SerialNumber: "PHASE5-SERIAL-002", ProductType: "シーマスター",
		CostAmountMinor: 500000, CostCurrency: "JPY", BaseSalePriceMinor: 720000,
		BaseSaleCurrency: "JPY", Condition: "美品 (A)", CreatedBy: "usr_admin",
	})
	if err != nil {
		t.Fatal(err)
	}
	shipment, err := store.CreateShipmentDraft(ctx, CreateShipmentInput{
		OrganizationID: "org_preview", ShipmentDate: "2026-07-30",
		RecipientName: "クロノス東京", CreatedBy: "usr_admin",
		Lines: []ShipmentLineInput{
			{ProductID: first.ID, Quantity: 1, WholesalePriceMinor: 610000},
			{ProductID: second.ID, Quantity: 1, WholesalePriceMinor: 720000},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ConfirmShipment(ctx, "org_preview", shipment.ID, "usr_admin"); err != nil {
		t.Fatal(err)
	}
	sale, err := store.CreateSaleDraft(ctx, CreateSaleInput{
		OrganizationID: "org_preview", SalesDate: "2026-07-30",
		CustomerName: "クロノス東京", CreatedBy: "usr_admin",
		Lines: []SalesLineInput{
			{ProductID: first.ID, Quantity: 1, UnitPriceMinor: 610000, Currency: "JPY"},
			{ProductID: second.ID, Quantity: 1, UnitPriceMinor: 720000, Currency: "JPY"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	confirmed, err := store.ConfirmSale(ctx, "org_preview", sale.ID, "usr_admin")
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.ShipmentStatus != "complete" || len(confirmed.Lines) != 2 {
		t.Fatalf("sale was not completed by the source shipment: %+v", confirmed)
	}
	for _, line := range confirmed.Lines {
		if line.ShippedQuantity != 1 || line.RemainingQuantity != 0 {
			t.Fatalf("line allocation mismatch: %+v", line)
		}
	}
	linked, err := store.Shipment(ctx, "org_preview", shipment.ID)
	if err != nil || linked.Warning != "" || len(linked.Lines) != 2 {
		t.Fatalf("linked shipment=%+v err=%v", linked, err)
	}
	for _, line := range linked.Lines {
		if line.SalesSlipNumber != confirmed.SlipNumber || line.AllocatedQuantity != 1 {
			t.Fatalf("shipment line allocation mismatch: %+v", line)
		}
	}
}

func TestSecondPreSaleShipmentForSameProductIsRejected(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	product := salesTestProduct(t, store)
	first := createTestShipment(t, store, product.ID, 1)
	if _, err := store.ConfirmShipment(ctx, "org_preview", first.ID, "usr_admin"); err != nil {
		t.Fatal(err)
	}
	second := createTestShipment(t, store, product.ID, 1)
	if _, err := store.ConfirmShipment(ctx, "org_preview", second.ID, "usr_admin"); !errors.Is(err, ErrProductAlreadyShipped) {
		t.Fatalf("expected duplicate pre-sale shipment rejection, got %v", err)
	}
}

func TestPartialShipmentsAndCumulativeLimit(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	product := salesTestProduct(t, store)
	sale := createTestSale(t, store, product.ID, 3, "JPY", 400000)
	if _, err := store.ConfirmSale(ctx, "org_preview", sale.ID, "usr_admin"); err != nil {
		t.Fatal(err)
	}
	first := createTestShipment(t, store, product.ID, 1)
	if _, err := store.ConfirmShipment(ctx, "org_preview", first.ID, "usr_admin"); err != nil {
		t.Fatal(err)
	}
	partial, err := store.Sale(ctx, "org_preview", sale.ID)
	if err != nil || partial.ShipmentStatus != "partial" || partial.Lines[0].RemainingQuantity != 2 {
		t.Fatalf("partial quantity incorrect: %+v err=%v", partial, err)
	}
	second := createTestShipment(t, store, product.ID, 2)
	if _, err := store.ConfirmShipment(ctx, "org_preview", second.ID, "usr_admin"); err != nil {
		t.Fatal(err)
	}
	complete, _ := store.Sale(ctx, "org_preview", sale.ID)
	if complete.ShipmentStatus != "complete" || complete.Lines[0].ShippedQuantity != 3 {
		t.Fatalf("completion incorrect: %+v", complete)
	}
	excess := createTestShipment(t, store, product.ID, 1)
	if _, err := store.ConfirmShipment(ctx, "org_preview", excess.ID, "usr_admin"); !errors.Is(err, ErrShipmentExceedsSale) {
		t.Fatalf("expected cumulative shipment rejection, got %v", err)
	}
}

func TestCancellationRecomputesAllocationsAndProductState(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	product := salesTestProduct(t, store)
	sale := createTestSale(t, store, product.ID, 2, "JPY", 500000)
	if _, err := store.ConfirmSale(ctx, "org_preview", sale.ID, "usr_admin"); err != nil {
		t.Fatal(err)
	}
	shipment := createTestShipment(t, store, product.ID, 1)
	if _, err := store.ConfirmShipment(ctx, "org_preview", shipment.ID, "usr_admin"); err != nil {
		t.Fatal(err)
	}
	if err := store.CancelShipment(ctx, "org_preview", shipment.ID, "usr_admin", "配送中止"); err != nil {
		t.Fatal(err)
	}
	afterShipmentCancel, _ := store.Sale(ctx, "org_preview", sale.ID)
	if afterShipmentCancel.ShipmentStatus != "unshipped" || afterShipmentCancel.Lines[0].ShippedQuantity != 0 {
		t.Fatalf("shipment cancellation not reflected: %+v", afterShipmentCancel)
	}
	productAfter, _ := store.Product(ctx, "org_preview", product.ID)
	if productAfter.InventoryStatus != "sold" {
		t.Fatalf("product should return to sold, got %s", productAfter.InventoryStatus)
	}
	if err := store.CancelSale(ctx, "org_preview", sale.ID, "usr_admin", "売上訂正"); err != nil {
		t.Fatal(err)
	}
	productAfter, _ = store.Product(ctx, "org_preview", product.ID)
	if productAfter.InventoryStatus != "in_stock" {
		t.Fatalf("product should return to stock, got %s", productAfter.InventoryStatus)
	}
}

func TestSalesAndShipmentsAreOrganizationScoped(t *testing.T) {
	store := testStore(t)
	product := salesTestProduct(t, store)
	sale := createTestSale(t, store, product.ID, 1, "JPY", 100)
	if _, err := store.Sale(context.Background(), "org_other", sale.ID); err == nil {
		t.Fatal("cross-organization sale must not be readable")
	}
	shipment := createTestShipment(t, store, product.ID, 1)
	if _, err := store.Shipment(context.Background(), "org_other", shipment.ID); err == nil {
		t.Fatal("cross-organization shipment must not be readable")
	}
}
