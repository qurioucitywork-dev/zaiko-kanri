package dataaccess

import "errors"

var (
	// ErrInvalidArgument identifies a caller input that cannot be executed.
	ErrInvalidArgument = errors.New("dataaccess: invalid argument")

	// ErrNotFound is intentionally shared by missing resources and resources
	// owned by another tenant. This avoids disclosing cross-tenant existence.
	ErrNotFound = errors.New("dataaccess: not found")

	// ErrConflict identifies an optimistic-concurrency, uniqueness, or status
	// transition conflict. Callers may reload current state before retrying.
	ErrConflict = errors.New("dataaccess: conflict")

	// ErrPrecondition identifies a business prerequisite that has not been
	// completed, such as attempting shipment before a sale is confirmed.
	ErrPrecondition = errors.New("dataaccess: precondition failed")

	// ErrIdempotencyMismatch means a key was already used with a different
	// command payload. Implementations must never execute the second payload.
	ErrIdempotencyMismatch = errors.New("dataaccess: idempotency mismatch")
)
