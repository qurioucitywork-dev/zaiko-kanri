package persistence

import (
	"testing"
	"time"
)

func TestCombinedPartInternalComment(t *testing.T) {
	const target = "3108260123"
	tests := []struct {
		name     string
		existing string
		want     string
	}{
		{name: "empty", want: "結合先商品管理番号: " + target},
		{name: "preserve existing", existing: "検品済み", want: "検品済み\n結合先商品管理番号: " + target},
		{name: "idempotent", existing: "検品済み\n結合先商品管理番号: " + target, want: "検品済み\n結合先商品管理番号: " + target},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := combinedPartInternalComment(test.existing, target); got != test.want {
				t.Fatalf("comment=%q, want %q", got, test.want)
			}
		})
	}
}

func TestCombinedPartUpdatesZeroInventoryCostAndLockStatus(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	updates := combinedPartUpdates(costAdjustmentCombinePart{InternalComment: "検品済み"}, "cad-1", "3108260123", "user-1", now)

	if updates["status"] != "combined" {
		t.Fatalf("status=%v, want combined", updates["status"])
	}
	if updates["cost_amount_minor"] != int64(0) || updates["fixed_cost_jpy_minor"] != int64(0) {
		t.Fatalf("combined part costs must be zero: %#v", updates)
	}
	if updates["internal_comment"] != "検品済み\n結合先商品管理番号: 3108260123" {
		t.Fatalf("internal_comment=%q", updates["internal_comment"])
	}
	if updates["cost_adjustment_id"] != "cad-1" || updates["updated_by"] != "user-1" || updates["updated_at"] != now {
		t.Fatalf("audit fields mismatch: %#v", updates)
	}
}
