package persistence

import (
	"context"
	"testing"
	"time"
)

func TestAppendOriginalPlannedSalePrice(t *testing.T) {
	got := appendOriginalPlannedSalePrice("検品済み", 1234567, "USD")
	if got != "検品済み\nもともとの想定売価: $1,234,567" {
		t.Fatalf("internal comment=%q", got)
	}
	if duplicate := appendOriginalPlannedSalePrice(got, 999, "USD"); duplicate != got {
		t.Fatalf("original planned sale price duplicated: %q", duplicate)
	}
	if yen := appendOriginalPlannedSalePrice("", 850000, "JPY"); yen != "もともとの想定売価: ¥850,000" {
		t.Fatalf("JPY original planned sale price=%q", yen)
	}
}

func TestDirectCostAdjustmentRequiresExactlyOneProduct(t *testing.T) {
	repository := &Repository{}
	_, err := repository.ConfirmDirectCostAdjustment(context.Background(), DirectCostAdjustmentInput{
		Direction: "increase",
		Notes:     "single product only",
		Items: []DirectCostAdjustmentItemInput{
			{ProductID: "one", AdjustmentAmountJPY: 1},
			{ProductID: "two", AdjustmentAmountJPY: 1},
		},
	})
	if err != ErrCostAdjustmentState {
		t.Fatalf("two-product direct adjustment error=%v, want ErrCostAdjustmentState", err)
	}
}

func TestCostAdjustmentSlipPrefix(t *testing.T) {
	tests := map[string]string{
		"breakdown":       "GW",
		"combine":         "GA",
		"direct":          "GC",
		"direct_increase": "GC",
		"direct_decrease": "GC",
	}
	for adjustmentType, want := range tests {
		if got := costAdjustmentSlipPrefix(adjustmentType); got != want {
			t.Fatalf("costAdjustmentSlipPrefix(%q)=%q, want %q", adjustmentType, got, want)
		}
	}
}

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

func TestAppendCostAdjustmentSlipNumber(t *testing.T) {
	const slipNumber = "CA-2026-0001"
	if got := appendCostAdjustmentSlipNumber("検品済み", slipNumber); got != "検品済み\n原価調整伝票番号: "+slipNumber {
		t.Fatalf("notes=%q", got)
	}
	if got := appendCostAdjustmentSlipNumber("原価調整伝票番号: "+slipNumber, slipNumber); got != "原価調整伝票番号: "+slipNumber {
		t.Fatalf("slip number must not be duplicated: %q", got)
	}
}

func TestCostAdjustmentInternalComment(t *testing.T) {
	got := costAdjustmentInternalComment(
		"3108260009",
		"CA-2026-0042",
		"対象商品管理番号: 3108260009\n原価調整伝票番号: CA-OLD\n確認メモ",
	)
	want := "原価管理番号: 3108260009\n調整伝票番号: CA-2026-0042\n確認メモ"
	if got != want {
		t.Fatalf("internal_comment=%q, want %q", got, want)
	}
}

func TestCostMergeInternalCommentOmitsCostManagementNumber(t *testing.T) {
	got := costAdjustmentInternalCommentForMode(
		"combine",
		"1309260008",
		"CA-2026-0043",
		"原価管理番号: 1309260008\n原価調整伝票番号: CA-2026-0042\n原価合わせ元: 1309260006, 1309260007\n確認メモ",
	)
	want := "調整伝票番号: CA-2026-0042\n調整伝票番号: CA-2026-0043\n原価合わせ元: 1309260006, 1309260007\n確認メモ"
	if got != want {
		t.Fatalf("internal_comment=%q, want %q", got, want)
	}
}

func TestCostMergeInternalCommentAccumulatesUniqueSourceSlips(t *testing.T) {
	inputs := []Product{
		{InternalComment: "原価管理番号: 1\n調整伝票番号: CA-2026-0001"},
		{InternalComment: "原価調整伝票番号: CA-2026-0001\n調整伝票番号: CA-2026-0002"},
	}
	got := costMergeInternalComment(inputs, "原価合わせ元: 1, 2")
	want := "調整伝票番号: CA-2026-0001\n調整伝票番号: CA-2026-0002\n原価合わせ元: 1, 2"
	if got != want {
		t.Fatalf("internal_comment=%q, want %q", got, want)
	}
}

func TestCostSplitStartBusinessDate(t *testing.T) {
	started := time.Date(2026, 9, 11, 16, 0, 0, 0, time.UTC)
	date := costSplitBusinessDate(started)
	if got := formatProductCode(date, 1); got != "1209260001" {
		t.Fatalf("code=%s", got)
	}
	if got := costSplitNotes("検品済み"); got != "検品済み\n原価分け" {
		t.Fatal(got)
	}
	if got := costSplitNotes("検品済み\n原価分け"); got != "検品済み\n原価分け" {
		t.Fatal(got)
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
