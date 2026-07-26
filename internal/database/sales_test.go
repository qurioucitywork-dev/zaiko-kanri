package database

import (
	"context"
	"errors"
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
