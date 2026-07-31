package database

import (
	"context"
	"strings"
	"testing"
)

func TestRateParsingSnapshotsAndProductConversion(t *testing.T) {
	store := inventoryStore(t)
	ctx := context.Background()
	productInput := singleInput("2026-08-10", "", "FX-TEST")
	productInput.CostAmountMinor = 100_000
	productInput.CostCurrency = "JPY"
	productInput.BaseSalePriceMinor = 1_000
	productInput.BaseSaleCurrency = ""
	product, err := store.CreateSingleProduct(ctx, productInput)
	if err != nil {
		t.Fatal(err)
	}
	if product.BaseSaleCurrency != "USD" {
		t.Fatalf("default currency=%s want USD", product.BaseSaleCurrency)
	}
	scaled, err := ParseRate("150.25")
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.AddExchangeRate(ctx, "org_preview", "USD", "JPY", scaled, "manual", "2026-08-10T09:30", "usr_admin")
	if err != nil {
		t.Fatal(err)
	}
	converted, err := store.Product(ctx, "org_preview", product.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !converted.RateAvailable || converted.ReferencePriceMinor != 150_250 || converted.ReferenceCurrency != "JPY" {
		t.Fatalf("converted product=%+v", converted)
	}
	if converted.GrossProfitMinor != 50_250 || converted.MarginBasisPoints == 0 {
		t.Fatalf("gross profit not calculated: %+v", converted)
	}
	newScaled, _ := ParseRate("151.00")
	if _, err := store.AddExchangeRate(ctx, "org_preview", "USD", "JPY", newScaled, "manual", "2026-08-10T10:30", "usr_admin"); err != nil {
		t.Fatal(err)
	}
	rates, err := store.ExchangeRates(ctx, "org_preview", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rates) != 2 || rates[1].ID != first.ID || rates[1].RateScaled != scaled {
		t.Fatalf("exchange snapshots were overwritten: %+v", rates)
	}
}

func TestMarketCSVPreviewReportsRowErrors(t *testing.T) {
	store := inventoryStore(t)
	ctx := context.Background()
	csvText := "\ufeffmarket_date,brand,model_number,product_type,price,currency,source\n" +
		"2026-08-11,ロレックス,126610LN,腕時計,1450000,JPY,manual\n" +
		"bad-date,オメガ,310.30,腕時計,not-number,EUR,csv\n"
	batch, err := store.PreviewMarketCSV(ctx, "org_preview", "usr_admin", "market.csv", strings.NewReader(csvText))
	if err != nil {
		t.Fatal(err)
	}
	if batch.TotalRows != 2 || batch.ValidRows != 1 || batch.ErrorRows != 1 {
		t.Fatalf("unexpected batch: %+v", batch)
	}
	if batch.Rows[1].ErrorMessage == "" || batch.Rows[1].Valid {
		t.Fatalf("invalid row not reported: %+v", batch.Rows[1])
	}
	if _, err := store.CommitMarketImport(ctx, "org_preview", batch.ID, "usr_admin", false); err == nil {
		t.Fatal("batch with errors must not commit")
	}
}

func TestMarketCSVCommitAndWorkerApprovalState(t *testing.T) {
	store := inventoryStore(t)
	ctx := context.Background()
	header := "market_date,brand,model_number,product_type,price,currency,source\n"
	valid := header + "2026-08-12,カルティエ,WSSA0018,腕時計,780000,JPY,csv\n"
	adminBatch, err := store.PreviewMarketCSV(ctx, "org_preview", "usr_admin", "admin.csv", strings.NewReader(valid))
	if err != nil {
		t.Fatal(err)
	}
	committed, err := store.CommitMarketImport(ctx, "org_preview", adminBatch.ID, "usr_admin", false)
	if err != nil {
		t.Fatal(err)
	}
	if committed.Status != "committed" {
		t.Fatalf("status=%s want committed", committed.Status)
	}
	records, err := store.MarketPrices(ctx, "org_preview", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Brand != "カルティエ" {
		t.Fatalf("records=%+v", records)
	}

	workerCSV := header + "2026-08-13,IWC,IW500705,腕時計,840000,JPY,csv\n"
	workerBatch, err := store.PreviewMarketCSV(ctx, "org_preview", "usr_worker", "worker.csv", strings.NewReader(workerCSV))
	if err != nil {
		t.Fatal(err)
	}
	pending, err := store.CommitMarketImport(ctx, "org_preview", workerBatch.ID, "usr_worker", true)
	if err != nil {
		t.Fatal(err)
	}
	if pending.Status != "pending_approval" {
		t.Fatalf("worker import status=%s want pending_approval", pending.Status)
	}
	records, _ = store.MarketPrices(ctx, "org_preview", 10)
	if len(records) != 1 {
		t.Fatalf("pending worker batch must not update market records: %+v", records)
	}
}

func TestProductMarketPricesSearchUpdateAndCSVImport(t *testing.T) {
	store := inventoryStore(t)
	ctx := context.Background()
	first, err := store.CreateSingleProduct(ctx, singleInput("2026-08-14", "MKT-SKU-1", "MKT-SERIAL-1"))
	if err != nil {
		t.Fatal(err)
	}
	secondInput := singleInput("2026-08-15", "MKT-SKU-2", "MKT-SERIAL-2")
	secondInput.Brand = "ロレックス"
	secondInput.ModelNumber = "MKT-REF-2"
	second, err := store.CreateSingleProduct(ctx, secondInput)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateProductMarketPrice(ctx, "org_preview", first.ID, "usr_admin", 700_000, 920_000); err != nil {
		t.Fatal(err)
	}
	filtered, err := store.ProductMarketPrices(ctx, "org_preview", ProductMarketPriceFilter{Query: "MKT-SKU-1"})
	if err != nil || len(filtered) != 1 {
		t.Fatalf("filtered=%+v err=%v", filtered, err)
	}
	if filtered[0].PurchaseMarketPriceMinor != 700_000 || filtered[0].SaleMarketPriceMinor != 920_000 {
		t.Fatalf("market prices=%+v", filtered[0])
	}
	csvText := "product_code,purchase_market_price,sale_market_price\n" +
		second.ProductCode + ",1250000,1580000\n"
	count, err := store.ImportProductMarketPricesCSV(ctx, "org_preview", "usr_admin", strings.NewReader(csvText))
	if err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	imported, err := store.ProductMarketPriceByProductID(ctx, "org_preview", second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !imported.HasMarketPrice || imported.PurchaseMarketPriceMinor != 1_250_000 ||
		imported.SaleMarketPriceMinor != 1_580_000 {
		t.Fatalf("imported=%+v", imported)
	}
}

func TestProductMarketPriceCSVImportIsAtomic(t *testing.T) {
	store := inventoryStore(t)
	ctx := context.Background()
	product, err := store.CreateSingleProduct(ctx, singleInput("2026-08-16", "MKT-ATOMIC", "MKT-ATOMIC-SERIAL"))
	if err != nil {
		t.Fatal(err)
	}
	csvText := "product_code,purchase_market_price,sale_market_price\n" +
		product.ProductCode + ",100,200\nUNKNOWN,300,400\n"
	if _, err := store.ImportProductMarketPricesCSV(ctx, "org_preview", "usr_admin", strings.NewReader(csvText)); err == nil {
		t.Fatal("unknown product must reject the full import")
	}
	record, err := store.ProductMarketPriceByProductID(ctx, "org_preview", product.ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.HasMarketPrice {
		t.Fatalf("failed CSV must not partially commit: %+v", record)
	}
}
