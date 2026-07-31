package database

import (
	"context"
	"errors"
	"strings"
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

func TestProductAdvancedFilters(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	if err := store.SeedInventoryPreview(ctx); err != nil {
		t.Fatal(err)
	}
	all, err := store.Products(ctx, "org_preview", ProductFilter{})
	if err != nil || len(all) == 0 {
		t.Fatalf("products=%d err=%v", len(all), err)
	}
	target := all[0]
	filtered, err := store.PagedProducts(ctx, "org_preview", ProductFilter{
		Brand: target.Brand, ModelNumber: target.ModelNumber, SerialNumber: target.SerialNumber,
		SupplierID: target.SupplierID, BuyerID: target.BuyerID,
		PurchaseDateFrom: target.PurchaseDate, PurchaseDateTo: target.PurchaseDate,
		Page: 1, PageSize: 20,
	})
	if err != nil || filtered.Total == 0 {
		t.Fatalf("advanced filter total=%d err=%v target=%+v", filtered.Total, err, target)
	}
	for _, product := range filtered.Products {
		if product.Brand != target.Brand || product.SupplierID != target.SupplierID || product.BuyerID != target.BuyerID {
			t.Fatalf("filter leaked product=%+v", product)
		}
	}

	withAccessories, err := store.PagedProducts(ctx, "org_preview", ProductFilter{
		Accessory: "BOX,GUARANTEE", Page: 1, PageSize: 20,
	})
	if err != nil || withAccessories.Total == 0 {
		t.Fatalf("multiple accessory filter total=%d err=%v", withAccessories.Total, err)
	}
	for _, product := range withAccessories.Products {
		upper := strings.ToUpper(product.Accessories)
		if !strings.Contains(upper, "BOX") || !strings.Contains(upper, "GUARANTEE") {
			t.Fatalf("multiple accessory filter leaked product=%+v", product)
		}
	}
}

func purchaseInput(date string, quantity int) CreatePurchaseInput {
	return CreatePurchaseInput{
		OrganizationID: "org_preview",
		SupplierID:     "sup_test",
		PurchaseDate:   date,
		CreatedBy:      "usr_admin",
		Lines: []PurchaseLineInput{{
			Quantity: quantity, UnitCostMinor: 100000, BaseSalePriceMinor: 150000, Currency: "JPY",
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

func updateInput(product Product) UpdateProductInput {
	return UpdateProductInput{
		OrganizationID: product.OrganizationID, ProductID: product.ID, ActorID: "usr_admin",
		BuyerID: product.BuyerID, SupplierID: product.SupplierID, PurchaseDate: product.PurchaseDate,
		Brand: product.Brand, ProductType: product.ProductType, ModelNumber: product.ModelNumber,
		SerialNumber: product.SerialNumber, InventoryStatus: product.InventoryStatus,
		Condition: product.Condition, Material: product.Material, Movement: product.Movement,
		BeltMaterial: product.BeltMaterial, Dial: product.Dial, Box: product.Box,
		Accessories: product.Accessories, Features: product.Features,
		CostAmountMinor: product.CostAmountMinor, BaseSalePriceMinor: product.BaseSalePriceMinor,
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
		if product.ProductCode != want || product.InventoryStatus != "in_stock" || product.BaseSalePriceMinor != 150000 {
			t.Fatalf("product=%+v want code=%s in_stock", product, want)
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

func TestPurchaseReturnConvertsLegacyUSDCostWithMasterRate(t *testing.T) {
	store := inventoryStore(t)
	ctx := context.Background()
	input := purchaseInput("2026-08-12", 1)
	input.Lines[0].UnitCostMinor = 1000
	input.Lines[0].Currency = "USD"
	input.Lines[0].SaleCurrency = "USD"
	slip, err := store.CreatePurchaseDraft(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	confirmed, err := store.ConfirmPurchase(ctx, "org_preview", slip.ID, "usr_admin")
	if err != nil || len(confirmed.Products) != 1 {
		t.Fatalf("confirmed=%+v err=%v", confirmed, err)
	}
	if _, err := store.AddExchangeRate(
		ctx, "org_preview", "USD", "JPY", 155_50000000, "test",
		"2026-08-12T10:00", "usr_admin",
	); err != nil {
		t.Fatal(err)
	}
	purchaseReturn, err := store.CreatePurchaseReturn(ctx, CreatePurchaseReturnInput{
		OrganizationID: "org_preview",
		PurchaseSlipID: slip.ID,
		ReturnDate:     "2026-08-13",
		Reason:         "test",
		ActorUserID:    "usr_admin",
		ProductIDs:     []string{confirmed.Products[0].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if purchaseReturn.AmountJPY != 155500 {
		t.Fatalf("return amount=%d, want 155500 JPY", purchaseReturn.AmountJPY)
	}
	lines, err := store.PurchaseReturnLines(ctx, "org_preview", purchaseReturn)
	if err != nil || len(lines) != 1 || lines[0].AmountJPY != 155500 {
		t.Fatalf("return lines=%+v err=%v", lines, err)
	}
}

func TestCreateConfirmedPurchaseImmediatelyCreatesInventory(t *testing.T) {
	store := inventoryStore(t)
	ctx := context.Background()

	slip, confirmed, err := store.CreateConfirmedPurchase(
		ctx,
		purchaseInput("2026-08-10", 2),
		"usr_admin",
	)
	if err != nil {
		t.Fatal(err)
	}
	if slip.Status != "confirmed" {
		t.Fatalf("status=%q want=confirmed", slip.Status)
	}
	if len(confirmed.Products) != 2 {
		t.Fatalf("generated=%d want=2", len(confirmed.Products))
	}

	products, err := store.Products(ctx, "org_preview", ProductFilter{})
	if err != nil {
		t.Fatal(err)
	}
	var linked int
	for _, product := range products {
		if product.ProductCode == "20260810001" || product.ProductCode == "20260810002" {
			linked++
		}
	}
	if linked != 2 {
		t.Fatalf("inventory products=%d want=2", linked)
	}
}

func TestCreateConfirmedPurchaseRollsBackDraftWhenConfirmationFails(t *testing.T) {
	store := inventoryStore(t)
	ctx := context.Background()
	input := purchaseInput("2026-08-11", 1)
	input.Lines[0].ProductCode = "INVALID"

	if _, _, err := store.CreateConfirmedPurchase(ctx, input, "usr_admin"); err == nil {
		t.Fatal("expected confirmation error")
	}
	var count int
	if err := store.db.QueryRow(`
		SELECT COUNT(*) FROM purchase_slips
		WHERE organization_id='org_preview' AND purchase_date='2026-08-11'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("drafts left behind=%d want=0", count)
	}
}

func TestPurchaseDraftProductDetailsPersistThroughConfirmation(t *testing.T) {
	store := inventoryStore(t)
	ctx := context.Background()
	input := purchaseInput("2026-08-09", 1)
	input.Lines[0] = PurchaseLineInput{
		Quantity: 1, UnitCostMinor: 850000, BaseSalePriceMinor: 1180000, Currency: "JPY",
		ProductCode: "20260809007", SKU: "SKU-ROUNDTRIP-007", Brand: "ロレックス",
		ProductType: "サブマリーナ", ModelNumber: "116610LN", SerialNumber: "ZX123456",
		Material: "ステンレスSS", Movement: "自動巻き", Condition: "極美品（S）",
		BeltMaterial: "ステンレス", Dial: "ブラック", Box: "BOX1",
		Accessories: "BOX, GUARANTEE", Features: "文字盤：黒",
	}
	slip, err := store.CreatePurchaseDraft(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := store.Purchase(ctx, "org_preview", slip.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(draft.Lines) != 1 || draft.Lines[0].ProductCode != "20260809007" ||
		draft.Lines[0].SKU != "SKU-ROUNDTRIP-007" || draft.Lines[0].Box != "BOX1" {
		t.Fatalf("draft details not persisted: %+v", draft.Lines)
	}
	nextWhileDraft, err := store.NextProductCode(ctx, "org_preview", "2026-08-09")
	if err != nil || nextWhileDraft != "20260809008" {
		t.Fatalf("next code with another draft=%q err=%v, want 20260809008", nextWhileDraft, err)
	}
	result, err := store.ConfirmPurchase(ctx, "org_preview", slip.ID, "usr_admin")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Products) != 1 {
		t.Fatalf("generated=%d want=1", len(result.Products))
	}
	product, err := store.Product(ctx, "org_preview", result.Products[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if product.ProductCode != "20260809007" || product.SKU != "SKU-ROUNDTRIP-007" ||
		product.SerialNumber != "ZX123456" || product.Material != "ステンレスSS" ||
		product.Movement != "自動巻き" || product.Condition != "極美品（S）" ||
		product.BeltMaterial != "ステンレス" || product.Dial != "ブラック" ||
		product.Box != "BOX1" || product.Accessories != "BOX, GUARANTEE" ||
		product.Features != "文字盤：黒" {
		t.Fatalf("confirmed product details not persisted: %+v", product)
	}
	next, err := store.NextProductCode(ctx, "org_preview", "2026-08-09")
	if err != nil || next != "20260809008" {
		t.Fatalf("next code=%q err=%v, want 20260809008", next, err)
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

func TestSingleProductStoresRegistrationDetailsAndManualCode(t *testing.T) {
	store := inventoryStore(t)
	ctx := context.Background()
	input := singleInput("2026-08-03", "DETAIL-SKU", "DETAIL-SERIAL")
	input.RequestedProductCode = "MANUAL-CODE-001"
	input.Material = "ステンレス"
	input.Box = "BOX1"
	input.Movement = "自動巻き"
	input.BeltMaterial = "ステンレス"
	input.Dial = "ブラック"
	input.Features = "コマ数：8"
	input.InternalComment = "社内確認済み"
	product, err := store.CreateSingleProduct(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if product.ProductCode != input.RequestedProductCode || product.Material != input.Material ||
		product.Box != input.Box || product.Movement != input.Movement ||
		product.BeltMaterial != input.BeltMaterial || product.Dial != input.Dial ||
		product.Features != input.Features || product.InternalComment != input.InternalComment {
		t.Fatalf("registration details not stored: %+v", product)
	}
}

func TestProductFilterSelectsExactNamedBox(t *testing.T) {
	store := inventoryStore(t)
	ctx := context.Background()
	box1 := singleInput("2026-08-03", "BOX-FILTER-1", "BOX-SERIAL-1")
	box1.Box = "BOX1"
	first, err := store.CreateSingleProduct(ctx, box1)
	if err != nil {
		t.Fatal(err)
	}
	box2 := singleInput("2026-08-03", "BOX-FILTER-2", "BOX-SERIAL-2")
	box2.Box = "BOX2"
	if _, err := store.CreateSingleProduct(ctx, box2); err != nil {
		t.Fatal(err)
	}

	products, err := store.Products(ctx, "org_preview", ProductFilter{Box: "BOX1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(products) != 1 || products[0].ID != first.ID || products[0].Box != "BOX1" {
		t.Fatalf("BOX1 filter returned %+v", products)
	}
}

func TestUpdateProductPreservesOptionalBrandAndSyncsSinglePurchaseLine(t *testing.T) {
	store := inventoryStore(t)
	ctx := context.Background()
	input := singleInput("2026-08-03", "EDIT-SKU", "EDIT-SERIAL")
	input.Brand = ""
	input.ProductType = ""
	product, err := store.CreateSingleProduct(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	update := updateInput(product)
	update.CostAmountMinor = 210000
	update.BaseSalePriceMinor = 320000
	update.Features = "編集済み"
	if err := store.UpdateProduct(ctx, update); err != nil {
		t.Fatal(err)
	}
	updated, err := store.Product(ctx, "org_preview", product.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Brand != "" || updated.ProductType != "" || updated.Features != "編集済み" ||
		updated.CostAmountMinor != 210000 || updated.BaseSalePriceMinor != 320000 {
		t.Fatalf("updated=%+v", updated)
	}
	var lineCost, lineSale int64
	if err := store.db.QueryRow(`
		SELECT l.unit_cost_minor,l.base_sale_price_minor
		FROM purchase_slip_lines l JOIN products p ON p.purchase_slip_line_id=l.id
		WHERE p.id=?`, product.ID).Scan(&lineCost, &lineSale); err != nil {
		t.Fatal(err)
	}
	if lineCost != 210000 || lineSale != 320000 {
		t.Fatalf("line cost=%d sale=%d", lineCost, lineSale)
	}
}

func TestUpdateProductAcceptsMockStatusAndRejectsMultiProductSlipChanges(t *testing.T) {
	store := inventoryStore(t)
	ctx := context.Background()
	product, err := store.CreateSingleProduct(ctx, singleInput("2026-08-03", "STATUS-SKU", "STATUS-SERIAL"))
	if err != nil {
		t.Fatal(err)
	}
	statusUpdate := updateInput(product)
	statusUpdate.InventoryStatus = "sold"
	if err := store.UpdateProduct(ctx, statusUpdate); err != nil {
		t.Fatalf("status update error=%v", err)
	}
	updatedStatus, err := store.Product(ctx, "org_preview", product.ID)
	if err != nil || updatedStatus.InventoryStatus != "sold" {
		t.Fatalf("updated status product=%+v err=%v", updatedStatus, err)
	}

	slip, err := store.CreatePurchaseDraft(ctx, purchaseInput("2026-08-06", 2))
	if err != nil {
		t.Fatal(err)
	}
	confirmed, err := store.ConfirmPurchase(ctx, "org_preview", slip.ID, "usr_admin")
	if err != nil || len(confirmed.Products) != 2 {
		t.Fatalf("confirmed=%+v err=%v", confirmed, err)
	}
	multi, err := store.Product(ctx, "org_preview", confirmed.Products[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	multiUpdate := updateInput(multi)
	multiUpdate.CostAmountMinor++
	if err := store.UpdateProduct(ctx, multiUpdate); err == nil || !strings.Contains(err.Error(), "複数生成") {
		t.Fatalf("multi-line update error=%v", err)
	}
	multiUpdate = updateInput(multi)
	multiUpdate.PurchaseDate = "2026-08-07"
	if err := store.UpdateProduct(ctx, multiUpdate); err == nil || !strings.Contains(err.Error(), "複数商品") {
		t.Fatalf("multi-slip update error=%v", err)
	}
}

func TestUpdateProductDuplicateSerialRequiresMemoAndRecordsOverride(t *testing.T) {
	store := inventoryStore(t)
	ctx := context.Background()
	first, err := store.CreateSingleProduct(ctx, singleInput("2026-08-07", "", "EDIT-DUP-A"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateSingleProduct(ctx, singleInput("2026-08-07", "", "EDIT-DUP-B"))
	if err != nil {
		t.Fatal(err)
	}
	update := updateInput(second)
	update.SerialNumber = first.SerialNumber
	if err := store.UpdateProduct(ctx, update); !errors.Is(err, ErrSerialReasonRequired) {
		t.Fatalf("duplicate error=%v", err)
	}
	update.ChangeMemo = "現物を照合し別個体と確認"
	if err := store.UpdateProduct(ctx, update); err != nil {
		t.Fatal(err)
	}
	var overrideCount int
	if err := store.db.QueryRow(`
		SELECT COUNT(*) FROM serial_duplicate_overrides
		WHERE product_id=? AND serial_number=? AND reason=?`,
		second.ID, first.SerialNumber, update.ChangeMemo).Scan(&overrideCount); err != nil {
		t.Fatal(err)
	}
	if overrideCount != 1 {
		t.Fatalf("override count=%d", overrideCount)
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
