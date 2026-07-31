package dataaccess

import "context"

// ProductWriter owns one product registration transaction. Implementations
// must include number allocation, idempotency recording, product persistence,
// relations, and the initial inventory event in that transaction.
type ProductWriter interface {
	CreateProduct(ctx context.Context, scope CommandScope, draft ProductDraft) (ProductMutationResult, error)
}

// InventoryWorkflowWriter exposes business-sized atomic commands instead of
// provider transaction handles. This is required because a D1 transaction
// cannot be held across multiple Container-to-Worker HTTP requests.
//
// Every implementation must:
//   - validate tenant ownership before mutation;
//   - persist idempotency key and canonical request hash atomically;
//   - perform compare-and-swap where ExpectedVersion is supplied;
//   - update the slip, product state, inventory event and audit log together;
//   - return ErrNotFound for cross-tenant resources;
//   - return ErrConflict for uniqueness/version/status races;
//   - honor context cancellation before committing.
type InventoryWorkflowWriter interface {
	ConfirmPurchase(ctx context.Context, command ConfirmPurchaseCommand) (WorkflowMutationResult, error)
	ConfirmSale(ctx context.Context, command ConfirmSaleCommand) (WorkflowMutationResult, error)
	ConfirmShipment(ctx context.Context, command ConfirmShipmentCommand) (WorkflowMutationResult, error)
	RestoreReturnedInventory(ctx context.Context, command RestoreReturnedInventoryCommand) (WorkflowMutationResult, error)
}
