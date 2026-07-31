package dataaccess

import (
	"context"
	"io"
	"time"
)

// PendingObjectInput is metadata persisted before uploading bytes. ObjectID is
// opaque outside the adapters and must be collision-resistant.
type PendingObjectInput struct {
	ObjectID     string
	ProductID    string
	OriginalName string
	ContentType  string
	SortOrder    int
}

type BlobReceipt struct {
	ChecksumSHA256 string
	SizeBytes      int64
}

type BlobHead struct {
	ChecksumSHA256 string
	SizeBytes      int64
	Exists         bool
}

// ObjectMetadataWriter controls the D1/PostgreSQL half of the two-phase object
// lifecycle. Each call is independently transactional and idempotent.
type ObjectMetadataWriter interface {
	CreatePendingObject(ctx context.Context, scope CommandScope, input PendingObjectInput) (ObjectMetadata, error)
	MarkObjectReady(ctx context.Context, scope CommandScope, objectID string, receipt BlobReceipt, readyAt time.Time) (ObjectMetadata, error)
	MarkObjectFailed(ctx context.Context, scope CommandScope, objectID, reasonCode string) error
	MarkObjectDeleted(ctx context.Context, scope CommandScope, objectID string, deletedAt time.Time) error
}

// ObjectBlobStore controls bytes in R2, S3, or a local test store. The opaque
// object ID is the only locator crossing this boundary; bucket, key, region,
// signed URL, provider request IDs and credentials remain adapter-private.
type ObjectBlobStore interface {
	Put(ctx context.Context, tenantID, objectID, contentType string, maxBytes int64, body io.Reader) (BlobReceipt, error)
	Head(ctx context.Context, tenantID, objectID string) (BlobHead, error)
	Open(ctx context.Context, tenantID, objectID string) (io.ReadCloser, error)
	Delete(ctx context.Context, tenantID, objectID string) error
}
