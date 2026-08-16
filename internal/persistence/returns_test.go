package persistence

import (
	"context"
	"errors"
	"testing"
)

func TestCreatePurchaseReturnRequiresNotes(t *testing.T) {
	repository := &Repository{}
	_, err := repository.CreateReturn(context.Background(), ReturnCreateInput{
		OperationType:   "purchase_return",
		TransactionDate: "2026-08-16",
		SupplierCode:    "S001",
		PurchaseSlipNo:  "PI-2026-0001",
		ProductCodes:    []string{"20260816001"},
		Notes:           "   ",
	})
	if !errors.Is(err, ErrReturnState) {
		t.Fatalf("expected ErrReturnState for blank purchase-return notes, got %v", err)
	}
}
