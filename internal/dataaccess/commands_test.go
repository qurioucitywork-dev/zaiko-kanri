package dataaccess

import (
	"errors"
	"testing"
	"time"
)

func validScope() CommandScope {
	return CommandScope{
		TenantID:       "tenant-a",
		ActorID:        "user-a",
		IdempotencyKey: "req-001",
		RequestedAt:    time.Date(2026, 7, 31, 1, 2, 3, 0, time.UTC),
	}
}

func TestProductDraftValidationRejectsFloatLikeCurrencyAndBadDate(t *testing.T) {
	draft := ProductDraft{
		SKU:           "SKU-001",
		Brand:         "Rolex",
		PurchaseDate:  "2026/07/31",
		Cost:          Money{AmountMinor: 100, Currency: "jpy"},
		BaseSalePrice: Money{AmountMinor: 200, Currency: "JPY"},
	}
	if err := draft.Validate(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("Validate() error = %v, want ErrInvalidArgument", err)
	}
}

func TestConfirmSaleRequiresRateForForeignCurrency(t *testing.T) {
	command := ConfirmSaleCommand{
		Scope:           validScope(),
		SaleDate:        "2026-07-31",
		BuyerID:         "buyer-a",
		Currency:        "USD",
		ExpectedVersion: 1,
		Lines: []SlipLineAmount{{
			ProductID: "product-a",
			Amount:    Money{AmountMinor: 1_000, Currency: "USD"},
			Quantity:  1,
		}},
	}
	if err := command.Validate(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("Validate() error = %v, want ErrInvalidArgument", err)
	}
}

func TestConfirmShipmentRejectsDuplicateProducts(t *testing.T) {
	command := ConfirmShipmentCommand{
		Scope:           validScope(),
		SalesSlipID:     "sale-a",
		ShipmentDate:    "2026-07-31",
		DestinationID:   "buyer-a",
		ProductIDs:      []string{"product-a", "product-a"},
		ExpectedVersion: 1,
	}
	if err := command.Validate(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("Validate() error = %v, want ErrInvalidArgument", err)
	}
}

func TestRestoreReturnedInventoryRejectsDuplicateReturnItems(t *testing.T) {
	item := RestoreReturnedInventoryItem{
		ReturnItemID:    "return-line-a",
		ProductID:       "product-a",
		ConditionCode:   "C04",
		Quantity:        1,
		ExpectedVersion: 1,
	}
	command := RestoreReturnedInventoryCommand{
		Scope:  validScope(),
		SaleID: "sale-a",
		Items:  []RestoreReturnedInventoryItem{item, item},
	}
	if err := command.Validate(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("Validate() error = %v, want ErrInvalidArgument", err)
	}
}
