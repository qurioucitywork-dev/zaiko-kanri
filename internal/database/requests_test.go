package database

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func requestTestProduct(t *testing.T, store *Store) Product {
	t.Helper()
	product := salesTestProduct(t, store)
	if err := store.SetProductPublication(context.Background(), "org_preview", product.ID, "usr_admin", "public"); err != nil {
		t.Fatal(err)
	}
	return product
}

func createTestRequest(t *testing.T, store *Store, productID, email string) PurchaseRequest {
	t.Helper()
	request, err := store.CreatePurchaseRequest(context.Background(), PurchaseRequestInput{
		OrganizationCode: "PREVIEW",
		ProductID:        productID, GuestName: "ゲスト", GuestEmail: email, Message: "購入希望",
	})
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func TestPurchaseRequestDoesNotReserveAndAllowsMultiplePending(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	product := requestTestProduct(t, store)
	first := createTestRequest(t, store, product.ID, "first@example.com")
	second := createTestRequest(t, store, product.ID, "second@example.com")
	if first.Status != "pending" || second.Status != "pending" {
		t.Fatalf("requests should remain pending: first=%s second=%s", first.Status, second.Status)
	}
	productAfter, err := store.Product(ctx, "org_preview", product.ID)
	if err != nil || productAfter.InventoryStatus != "in_stock" {
		t.Fatalf("request submission must not reserve: status=%s err=%v", productAfter.InventoryStatus, err)
	}
	var reservations int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM reservations WHERE product_id=?`, product.ID).Scan(&reservations); err != nil || reservations != 0 {
		t.Fatalf("unexpected reservations=%d err=%v", reservations, err)
	}
}

func TestPublicCatalogFiltersAndImageScope(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	product := requestTestProduct(t, store)
	image, err := store.AddProductImage(ctx, ProductImage{
		ProductID: product.ID, StoragePath: "catalog/test.webp", OriginalName: "test.webp",
		ContentType: "image/webp", SizeBytes: 128,
	}, "org_preview", "usr_admin")
	if err != nil {
		t.Fatal(err)
	}
	filtered, err := store.PublicProductsFiltered(ctx, "PREVIEW", PublicProductFilter{
		Query: product.ModelNumber, Brand: product.Brand, Condition: product.Condition,
	})
	if err != nil || len(filtered) != 1 || len(filtered[0].Images) != 1 {
		t.Fatalf("filtered=%+v err=%v", filtered, err)
	}
	if _, err := store.PublicProductImage(ctx, "PREVIEW", image.ID); err != nil {
		t.Fatalf("published image: %v", err)
	}
	if err := store.SetProductPublication(ctx, "org_preview", product.ID, "usr_admin", "private"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PublicProductImage(ctx, "PREVIEW", image.ID); err == nil {
		t.Fatal("private product image must not be publicly available")
	}
}

func TestSeedGuestCatalogPreviewIsIdempotent(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	if err := store.SeedInventoryPreview(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedGuestCatalogPreview(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedGuestCatalogPreview(ctx); err != nil {
		t.Fatal(err)
	}
	products, err := store.PublicProducts(ctx, "PREVIEW")
	if err != nil || len(products) < 7 {
		t.Fatalf("public products=%d err=%v", len(products), err)
	}
	var count int
	if err := store.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM products WHERE organization_id='org_preview' AND sku='GUEST-OMEGA-SPEED'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("guest seed duplicates=%d err=%v", count, err)
	}
}

func TestOnlyOneConcurrentRequestCanReserveProduct(t *testing.T) {
	store := testStore(t)
	product := requestTestProduct(t, store)
	first := createTestRequest(t, store, product.ID, "first@example.com")
	second := createTestRequest(t, store, product.ID, "second@example.com")
	requestIDs := []string{first.ID, second.ID}
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for index, id := range requestIDs {
		wait.Add(1)
		go func(index int, id string) {
			defer wait.Done()
			_, err := store.ApprovePurchaseRequest(context.Background(), "org_preview", id, []string{"usr_admin", "usr_worker"}[index])
			results <- err
		}(index, id)
	}
	wait.Wait()
	close(results)
	var succeeded, conflicted int
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrReservationConflict):
			conflicted++
		default:
			t.Fatalf("unexpected approval result: %v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("success=%d conflict=%d", succeeded, conflicted)
	}
	productAfter, _ := store.Product(context.Background(), "org_preview", product.ID)
	if productAfter.InventoryStatus != "reserved" {
		t.Fatalf("approved request should reserve product, got %s", productAfter.InventoryStatus)
	}
}

func TestReservationExpiryAndCancellationReleaseProduct(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	product := requestTestProduct(t, store)
	start := time.Date(2026, 7, 26, 3, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return start }
	request := createTestRequest(t, store, product.ID, "expiry@example.com")
	approved, err := store.ApprovePurchaseRequest(ctx, "org_preview", request.ID, "usr_admin")
	if err != nil {
		t.Fatal(err)
	}
	if approved.ReservationExpires == nil || !approved.ReservationExpires.Equal(start.Add(24*time.Hour)) {
		t.Fatalf("default reservation expiry incorrect: %+v", approved.ReservationExpires)
	}
	store.now = func() time.Time { return start.Add(25 * time.Hour) }
	if err := store.ExpireReservations(ctx, "org_preview"); err != nil {
		t.Fatal(err)
	}
	expired, _ := store.PurchaseRequest(ctx, "org_preview", request.ID)
	productAfter, _ := store.Product(ctx, "org_preview", product.ID)
	if expired.Status != "expired" || expired.ReservationStatus != "expired" || productAfter.InventoryStatus != "in_stock" {
		t.Fatalf("expiry not reflected: request=%+v product=%s", expired, productAfter.InventoryStatus)
	}

	next := createTestRequest(t, store, product.ID, "cancel@example.com")
	if _, err := store.ApprovePurchaseRequest(ctx, "org_preview", next.ID, "usr_admin"); err != nil {
		t.Fatal(err)
	}
	if err := store.CancelPurchaseRequest(ctx, "org_preview", next.ID, "usr_admin", "お客様都合"); err != nil {
		t.Fatal(err)
	}
	cancelled, _ := store.PurchaseRequest(ctx, "org_preview", next.ID)
	productAfter, _ = store.Product(ctx, "org_preview", product.ID)
	if cancelled.Status != "cancelled" || cancelled.ReservationStatus != "released" || productAfter.InventoryStatus != "in_stock" {
		t.Fatalf("cancellation not reflected: request=%+v product=%s", cancelled, productAfter.InventoryStatus)
	}
}

func TestSaleConfirmationFulfillsReservation(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	product := requestTestProduct(t, store)
	request := createTestRequest(t, store, product.ID, "buyer@example.com")
	if _, err := store.ApprovePurchaseRequest(ctx, "org_preview", request.ID, "usr_admin"); err != nil {
		t.Fatal(err)
	}
	sale := createTestSale(t, store, product.ID, 1, "JPY", 1200000)
	if _, err := store.ConfirmSale(ctx, "org_preview", sale.ID, "usr_admin"); err != nil {
		t.Fatal(err)
	}
	soldRequest, _ := store.PurchaseRequest(ctx, "org_preview", request.ID)
	productAfter, _ := store.Product(ctx, "org_preview", product.ID)
	if soldRequest.Status != "sold" || soldRequest.ReservationStatus != "fulfilled" || productAfter.InventoryStatus != "sold" {
		t.Fatalf("sale did not fulfill reservation: request=%+v product=%s", soldRequest, productAfter.InventoryStatus)
	}
}

func TestPublicProductModelContainsOnlyGuestSafeFields(t *testing.T) {
	store := testStore(t)
	product := requestTestProduct(t, store)
	products, err := store.PublicProducts(context.Background(), "PREVIEW")
	if err != nil || len(products) != 1 {
		t.Fatalf("public products: len=%d err=%v", len(products), err)
	}
	if products[0].ID != product.ID || products[0].SalePriceMinor != product.BaseSalePriceMinor {
		t.Fatalf("unexpected public product: %+v", products[0])
	}
	otherProducts, err := store.PublicProducts(context.Background(), "OTHER")
	if err != nil || len(otherProducts) != 0 {
		t.Fatalf("cross-organization public products=%d err=%v", len(otherProducts), err)
	}
	if _, err := store.CreatePurchaseRequest(context.Background(), PurchaseRequestInput{
		OrganizationCode: "OTHER", ProductID: product.ID,
		GuestName: "別組織", GuestEmail: "other@example.com",
	}); !errors.Is(err, ErrProductNotAvailable) {
		t.Fatalf("cross-organization request error=%v", err)
	}
}

func TestPurchaseRequestsAreOrganizationScoped(t *testing.T) {
	store := testStore(t)
	product := requestTestProduct(t, store)
	request := createTestRequest(t, store, product.ID, "scope@example.com")
	if _, err := store.PurchaseRequest(context.Background(), "org_other", request.ID); err == nil {
		t.Fatal("cross-organization purchase request must not be readable")
	}
}
