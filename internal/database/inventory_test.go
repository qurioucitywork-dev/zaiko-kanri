package database

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func inventoryStore(t *testing.T) *Store {
	t.Helper()
	store := testStore(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := store.db.Exec(`
		INSERT INTO suppliers(id,organization_id,supplier_code,name,created_at,updated_at)
		VALUES('sup_test','org_preview','TEST','テスト仕入先',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	return store
}

func purchaseInput(date string, quantity int) CreatePurchaseInput {
	return CreatePurchaseInput{
		OrganizationID: "org_preview",
		SupplierID:     "sup_test",
		PurchaseDate:   date,
		CreatedBy:      "usr_admin",
		Lines: []PurchaseLineInput{{
			Quantity: quantity, UnitCostMinor: 100000, Currency: "JPY",
			Brand: "ロレックス", ModelNumber: "TEST-01", ProductType: "腕時計",
		}},
	}
}

func singleInput(date, sku, serial string) SingleProductInput {
	return SingleProductInput{
		OrganizationID: "org_preview", SupplierID: "sup_test", PurchaseDate: date,
		SKU: sku, Brand: "オメガ", ModelNumber: "TEST-02", SerialNumber: serial,
		ProductType: "腕時計", CostAmountMinor: 200000, CostCurrency: "JPY",
		BaseSalePriceMinor: 300000, BaseSaleCurrency: "JPY", CreatedBy: "usr_admin",
	}
}

func TestSeedInventoryPreviewUsesAndMigratesUSDPrices(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	if err := store.SeedInventoryPreview(ctx); err != nil {
		t.Fatal(err)
	}
	var id, currency string
	var price int64
	if err := store.db.QueryRow(`
		SELECT id,base_sale_price_minor,base_sale_currency
		FROM products WHERE organization_id='org_preview' LIMIT 1`).Scan(&id, &price, &currency); err != nil {
		t.Fatal(err)
	}
	if price != 7613 || currency != "USD" {
		t.Fatalf("seed sale price=%d %s want 7613 USD", price, currency)
	}
	if _, err := store.db.Exec(`
		UPDATE products SET base_sale_price_minor=1180000,base_sale_currency='JPY' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedInventoryPreview(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`
		SELECT base_sale_price_minor,base_sale_currency FROM products WHERE id=?`, id).Scan(&price, &currency); err != nil {
		t.Fatal(err)
	}
	if price != 7613 || currency != "USD" {
		t.Fatalf("migrated sale price=%d %s want 7613 USD", price, currency)
	}
}

func TestConfirmPurchaseGeneratesQuantityAndIsIdempotent(t *testing.T) {
	store := inventoryStore(t)
	ctx := context.Background()
	slip, err := store.CreatePurchaseDraft(ctx, purchaseInput("2026-08-01", 3))
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.ConfirmPurchase(ctx, "org_preview", slip.ID, "usr_admin")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Products) != 3 {
		t.Fatalf("generated=%d want=3", len(result.Products))
	}
	for index, product := range result.Products {
		want := "20260801" + []string{"001", "002", "003"}[index]
		if product.ProductCode != want || product.InventoryStatus != "purchasing" {
			t.Fatalf("product=%+v want code=%s purchasing", product, want)
		}
	}
	repeated, err := store.ConfirmPurchase(ctx, "org_preview", slip.ID, "usr_admin")
	if err != nil {
		t.Fatal(err)
	}
	if !repeated.AlreadyConfirmed || len(repeated.Products) != 3 {
		t.Fatalf("repeat result=%+v", repeated)
	}
}

func TestConcurrentConfirmProducesUniqueCodes(t *testing.T) {
	store := inventoryStore(t)
	ctx := context.Background()
	first, _ := store.CreatePurchaseDraft(ctx, purchaseInput("2026-08-02", 2))
	second, _ := store.CreatePurchaseDraft(ctx, purchaseInput("2026-08-02", 2))
	var wg sync.WaitGroup
	var results [2]ConfirmResult
	var errs [2]error
	for index, id := range []string{first.ID, second.ID} {
		wg.Add(1)
		go func(index int, id string) {
			defer wg.Done()
			results[index], errs[index] = store.ConfirmPurchase(ctx, "org_preview", id, "usr_admin")
		}(index, id)
	}
	wg.Wait()
	codes := map[string]bool{}
	for index := range results {
		if errs[index] != nil {
			t.Fatalf("confirm %d: %v", index, errs[index])
		}
		for _, product := range results[index].Products {
			if codes[product.ProductCode] {
				t.Fatalf("duplicate product code: %s", product.ProductCode)
			}
			codes[product.ProductCode] = true
		}
	}
	if len(codes) != 4 {
		t.Fatalf("unique codes=%d want=4", len(codes))
	}
}

func TestSingleProductCreatesSimplePurchaseAndAllowsDuplicateSKU(t *testing.T) {
	store := inventoryStore(t)
	ctx := context.Background()
	first, err := store.CreateSingleProduct(ctx, singleInput("2026-08-03", "DUP-SKU", "SERIAL-A"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateSingleProduct(ctx, singleInput("2026-08-03", "DUP-SKU", "SERIAL-B"))
	if err != nil {
		t.Fatal(err)
	}
	if first.ProductCode == second.ProductCode || first.SKU != second.SKU {
		t.Fatalf("unexpected products: first=%+v second=%+v", first, second)
	}
	var simpleCount int
	if err := store.db.QueryRow(`
		SELECT COUNT(*) FROM purchase_slips p
		JOIN purchase_slip_lines l ON l.purchase_slip_id=p.id
		JOIN products pr ON pr.purchase_slip_line_id=l.id
		WHERE pr.id IN (?,?) AND p.is_simple=1 AND p.status='confirmed'`, first.ID, second.ID).Scan(&simpleCount); err != nil {
		t.Fatal(err)
	}
	if simpleCount != 2 {
		t.Fatalf("simple purchase count=%d want=2", simpleCount)
	}
}

func TestDuplicateSerialRequiresReasonAndRecordsOverride(t *testing.T) {
	store := inventoryStore(t)
	ctx := context.Background()
	if _, err := store.CreateSingleProduct(ctx, singleInput("2026-08-04", "", "DUPLICATE-SERIAL")); err != nil {
		t.Fatal(err)
	}
	input := singleInput("2026-08-04", "", "DUPLICATE-SERIAL")
	_, err := store.CreateSingleProduct(ctx, input)
	var duplicateErr *SerialDuplicateError
	if !errors.As(err, &duplicateErr) || len(duplicateErr.Candidates) != 1 {
		t.Fatalf("error=%v candidates=%d", err, len(duplicateErr.Candidates))
	}
	input.DuplicateReason = "現物と仕入先を確認し、別個体と判断"
	product, err := store.CreateSingleProduct(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM serial_duplicate_overrides WHERE product_id=?`, product.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("override count=%d want=1", count)
	}
}

func TestCancelledProductCodeIsNotReused(t *testing.T) {
	store := inventoryStore(t)
	ctx := context.Background()
	first, err := store.CreateSingleProduct(ctx, singleInput("2026-08-05", "", "CANCEL-A"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CancelProduct(ctx, "org_preview", first.ID, "usr_admin", "誤登録"); err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateSingleProduct(ctx, singleInput("2026-08-05", "", "CANCEL-B"))
	if err != nil {
		t.Fatal(err)
	}
	if first.ProductCode != "20260805001" || second.ProductCode != "20260805002" {
		t.Fatalf("codes were reused: %s %s", first.ProductCode, second.ProductCode)
	}
}

func TestThousandthProductReturnsBusinessError(t *testing.T) {
	store := inventoryStore(t)
	ctx := context.Background()
	if _, err := store.db.Exec(`
		INSERT INTO product_code_sequences(organization_id,purchase_date,last_sequence)
		VALUES('org_preview','2026-08-06',999)`); err != nil {
		t.Fatal(err)
	}
	_, err := store.CreateSingleProduct(ctx, singleInput("2026-08-06", "", "LIMIT"))
	if !errors.Is(err, ErrDailyProductLimit) {
		t.Fatalf("error=%v want=%v", err, ErrDailyProductLimit)
	}
}

func TestPagedProductsSortsPagesAndHidesCancelledByDefault(t *testing.T) {
	store := inventoryStore(t)
	ctx := context.Background()
	var created []Product
	for index, serial := range []string{"PAGE-A", "PAGE-B", "PAGE-C"} {
		product, err := store.CreateSingleProduct(ctx, singleInput(
			[]string{"2026-08-09", "2026-08-07", "2026-08-08"}[index], "", serial,
		))
		if err != nil {
			t.Fatal(err)
		}
		created = append(created, product)
	}
	if err := store.CancelProduct(ctx, "org_preview", created[1].ID, "usr_admin", "取消検索テスト"); err != nil {
		t.Fatal(err)
	}
	page, err := store.PagedProducts(ctx, "org_preview", ProductFilter{
		Sort: "purchase_asc", Page: 1, PageSize: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || page.TotalPages != 2 || len(page.Products) != 1 ||
		page.Products[0].SerialNumber != "PAGE-C" {
		t.Fatalf("unexpected first page: %+v products=%+v", page, page.Products)
	}
	withCancelled, err := store.PagedProducts(ctx, "org_preview", ProductFilter{
		Status: "cancelled", IncludeCancelled: true, Page: 1, PageSize: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if withCancelled.Total != 1 || withCancelled.Products[0].SerialNumber != "PAGE-B" {
		t.Fatalf("cancelled products=%+v", withCancelled.Products)
	}
}
