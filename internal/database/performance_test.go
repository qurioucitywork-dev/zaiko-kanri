package database

import (
	"context"
	"errors"
	"testing"
)

func TestPerformanceAggregatesOperationalData(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	if err := store.SeedInventoryPreview(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedMarketPreview(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedSalesPreview(ctx); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"suppliers", "buyers", "sales-destinations"} {
		records, err := store.Performance(ctx, "org_preview", mode, "2026-01-01", "2026-12-31")
		if err != nil || len(records) == 0 {
			t.Fatalf("mode=%s records=%d err=%v", mode, len(records), err)
		}
		var total int64
		for _, record := range records {
			total += record.AmountJPY
		}
		if total <= 0 {
			t.Fatalf("mode=%s total=%d", mode, total)
		}
	}
	if _, err := store.Performance(ctx, "org_preview", "unknown", "2026-01-01", "2026-12-31"); !errors.Is(err, ErrInvalidPerformanceMode) {
		t.Fatalf("invalid mode error=%v", err)
	}
	buyerRecords, err := store.BuyerPerformance(ctx, "org_preview", "2026-01-01", "2026-12-31")
	if err != nil || len(buyerRecords) == 0 {
		t.Fatalf("buyer performance records=%d err=%v", len(buyerRecords), err)
	}
	var purchases, revenue int64
	for _, record := range buyerRecords {
		purchases += record.PurchaseAmountJPY
		revenue += record.RevenueJPY
		if record.PurchaseCount < record.SalesCount {
			t.Fatalf("buyer performance invalid counts: %+v", record)
		}
	}
	if purchases <= 0 || revenue <= 0 {
		t.Fatalf("buyer performance purchases=%d revenue=%d", purchases, revenue)
	}
	destinationRecords, err := store.SalesDestinationPerformance(ctx, "org_preview", "2026-01-01", "2026-12-31")
	if err != nil || len(destinationRecords) == 0 {
		t.Fatalf("sales destination performance records=%d err=%v", len(destinationRecords), err)
	}
	var destinationRevenue, destinationCost int64
	for _, record := range destinationRecords {
		destinationRevenue += record.RevenueJPY
		destinationCost += record.CostJPY
		if record.SalesCount <= 0 {
			t.Fatalf("sales destination performance invalid count: %+v", record)
		}
	}
	if destinationRevenue <= 0 || destinationCost <= 0 {
		t.Fatalf("sales destination performance revenue=%d cost=%d", destinationRevenue, destinationCost)
	}
}
