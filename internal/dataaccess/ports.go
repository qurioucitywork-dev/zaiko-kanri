package dataaccess

import "context"

// DiagnosticReader performs read-only health and capability checks. Diagnose
// must not run migrations, repair data, create buckets, or write probe rows.
type DiagnosticReader interface {
	Diagnose(ctx context.Context) (DiagnosticReport, error)
}

// ProductReader provides tenant-scoped product lookup and search.
//
// Implementations must:
//   - reject an empty tenant ID with ErrInvalidArgument;
//   - return ErrNotFound for both a missing product and a product belonging to
//     another tenant;
//   - honor context cancellation;
//   - return deterministic pagination ordered by purchase date descending,
//     then product code ascending. Product code is unique within a tenant.
type ProductReader interface {
	SearchProducts(ctx context.Context, tenantID string, search ProductSearch) (ProductPage, error)
	GetProduct(ctx context.Context, tenantID, productID string) (Product, error)
}

// ObjectMetadataReader reads tenant-scoped metadata only. Opening, uploading,
// signing, or deleting object bytes belongs to a separate ObjectStore port
// introduced with a storage implementation.
type ObjectMetadataReader interface {
	ListProductObjects(ctx context.Context, tenantID, productID string) ([]ObjectMetadata, error)
	GetObjectMetadata(ctx context.Context, tenantID, objectID string) (ObjectMetadata, error)
}
