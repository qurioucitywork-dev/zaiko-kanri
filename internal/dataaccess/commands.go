package dataaccess

import (
	"fmt"
	"strings"
	"time"
)

// CommandScope is required for every state-changing operation.
// IdempotencyKey is unique within a tenant and operation name. Providers must
// persist both the key and a canonical request hash in the same transaction as
// the business change.
type CommandScope struct {
	TenantID       string
	ActorID        string
	IdempotencyKey string
	RequestedAt    time.Time
}

func (s CommandScope) Validate() error {
	if strings.TrimSpace(s.TenantID) == "" ||
		strings.TrimSpace(s.ActorID) == "" ||
		strings.TrimSpace(s.IdempotencyKey) == "" ||
		len(s.IdempotencyKey) > 128 ||
		s.RequestedAt.IsZero() {
		return ErrInvalidArgument
	}
	return nil
}

// ProductDraft contains provider-independent values required to register an
// inventory product. ProductCode is allocated atomically by the repository
// when empty; callers must not perform a separate "get next number" request.
type ProductDraft struct {
	ProductCode       string
	SKU               string
	Brand             string
	ModelNumber       string
	SerialNumber      string
	ProductType       string
	SupplierID        string
	BuyerID           string
	PurchaseDate      string
	Cost              Money
	BaseSalePrice     Money
	InventoryStatus   string
	PublicationStatus string
	Condition         string
	AccessoryCodes    []string
	BoxID             string
}

func (d ProductDraft) Validate() error {
	if strings.TrimSpace(d.SKU) == "" ||
		strings.TrimSpace(d.Brand) == "" ||
		strings.TrimSpace(d.PurchaseDate) == "" ||
		d.Cost.AmountMinor < 0 ||
		d.BaseSalePrice.AmountMinor < 0 ||
		!validCurrency(d.Cost.Currency) ||
		!validCurrency(d.BaseSalePrice.Currency) {
		return ErrInvalidArgument
	}
	if _, err := time.Parse("2006-01-02", d.PurchaseDate); err != nil {
		return fmt.Errorf("%w: purchase date", ErrInvalidArgument)
	}
	return nil
}

type ProductMutationResult struct {
	ProductID   string
	ProductCode string
	Version     int64
	Replayed    bool
}

type SlipLineAmount struct {
	ProductID string
	Amount    Money
	Quantity  int
}

func (l SlipLineAmount) validate() error {
	if strings.TrimSpace(l.ProductID) == "" ||
		l.Quantity < 1 ||
		l.Amount.AmountMinor < 0 ||
		!validCurrency(l.Amount.Currency) {
		return ErrInvalidArgument
	}
	return nil
}

// ConfirmPurchaseCommand is one atomic operation: allocate the slip/product
// numbers, persist header and lines, register inventory events, and transition
// each product. Partial success is forbidden.
type ConfirmPurchaseCommand struct {
	Scope        CommandScope
	PurchaseDate string
	SupplierID   string
	StaffID      string
	Lines        []SlipLineAmount
}

func (c ConfirmPurchaseCommand) Validate() error {
	if err := c.Scope.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(c.SupplierID) == "" ||
		strings.TrimSpace(c.StaffID) == "" ||
		len(c.Lines) == 0 {
		return ErrInvalidArgument
	}
	if _, err := time.Parse("2006-01-02", c.PurchaseDate); err != nil {
		return fmt.Errorf("%w: purchase date", ErrInvalidArgument)
	}
	return validateLines(c.Lines)
}

type ConfirmSaleCommand struct {
	Scope           CommandScope
	SaleDate        string
	BuyerID         string
	TaxExempt       bool
	Currency        string
	FXRateScaled    int64
	FXRateScale     int32
	Lines           []SlipLineAmount
	ExpectedVersion int64
}

func (c ConfirmSaleCommand) Validate() error {
	if err := c.Scope.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(c.BuyerID) == "" ||
		!validCurrency(c.Currency) ||
		c.ExpectedVersion < 0 ||
		len(c.Lines) == 0 {
		return ErrInvalidArgument
	}
	if _, err := time.Parse("2006-01-02", c.SaleDate); err != nil {
		return fmt.Errorf("%w: sale date", ErrInvalidArgument)
	}
	if c.Currency != "JPY" && (c.FXRateScaled < 1 || c.FXRateScale < 1) {
		return ErrInvalidArgument
	}
	return validateLines(c.Lines)
}

type ConfirmShipmentCommand struct {
	Scope           CommandScope
	SalesSlipID     string
	ShipmentDate    string
	DestinationID   string
	ProductIDs      []string
	ExpectedVersion int64
}

func (c ConfirmShipmentCommand) Validate() error {
	if err := c.Scope.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(c.SalesSlipID) == "" ||
		strings.TrimSpace(c.DestinationID) == "" ||
		c.ExpectedVersion < 0 ||
		len(c.ProductIDs) == 0 {
		return ErrInvalidArgument
	}
	if _, err := time.Parse("2006-01-02", c.ShipmentDate); err != nil {
		return fmt.Errorf("%w: shipment date", ErrInvalidArgument)
	}
	return validateIDs(c.ProductIDs)
}

type RestoreReturnedInventoryItem struct {
	ReturnItemID    string
	ProductID       string
	ConditionCode   string
	Quantity        int
	ExpectedVersion int64
}

type RestoreReturnedInventoryCommand struct {
	Scope  CommandScope
	SaleID string
	Items  []RestoreReturnedInventoryItem
}

func (c RestoreReturnedInventoryCommand) Validate() error {
	if err := c.Scope.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(c.SaleID) == "" || len(c.Items) == 0 {
		return ErrInvalidArgument
	}
	seen := make(map[string]struct{}, len(c.Items))
	for _, item := range c.Items {
		if strings.TrimSpace(item.ReturnItemID) == "" ||
			strings.TrimSpace(item.ProductID) == "" ||
			strings.TrimSpace(item.ConditionCode) == "" ||
			item.Quantity < 1 ||
			item.ExpectedVersion < 0 {
			return ErrInvalidArgument
		}
		if _, exists := seen[item.ReturnItemID]; exists {
			return ErrInvalidArgument
		}
		seen[item.ReturnItemID] = struct{}{}
	}
	return nil
}

type WorkflowMutationResult struct {
	ID       string
	Number   string
	Version  int64
	Replayed bool
}

func validateLines(lines []SlipLineAmount) error {
	seen := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		if err := line.validate(); err != nil {
			return err
		}
		if _, exists := seen[line.ProductID]; exists {
			return ErrInvalidArgument
		}
		seen[line.ProductID] = struct{}{}
	}
	return nil
}

func validateIDs(ids []string) error {
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			return ErrInvalidArgument
		}
		if _, exists := seen[id]; exists {
			return ErrInvalidArgument
		}
		seen[id] = struct{}{}
	}
	return nil
}

func validCurrency(value string) bool {
	if len(value) != 3 {
		return false
	}
	for _, r := range value {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}
