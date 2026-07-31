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
}
