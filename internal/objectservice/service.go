// Package objectservice coordinates database metadata and object bytes without
// pretending that D1/R2 or PostgreSQL/S3 share a distributed transaction.
package objectservice

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/dataaccess"
)

const (
	defaultMaxImageBytes = 15 << 20
	compensationTimeout  = 5 * time.Second
	failureUpload        = "blob_upload_failed"
	failureVerify        = "blob_verify_failed"
)

var allowedImageTypes = map[string]struct{}{
	"image/jpeg": {},
	"image/png":  {},
	"image/webp": {},
}

type IDGenerator func() (string, error)

type Service struct {
	metadata dataaccess.ObjectMetadataWriter
	blobs    dataaccess.ObjectBlobStore
	now      func() time.Time
	maxBytes int64
}

func New(
	metadata dataaccess.ObjectMetadataWriter,
	blobs dataaccess.ObjectBlobStore,
) (*Service, error) {
	if metadata == nil || blobs == nil {
		return nil, dataaccess.ErrInvalidArgument
	}
	return &Service{
		metadata: metadata,
		blobs:    blobs,
		now:      func() time.Time { return time.Now().UTC() },
		maxBytes: defaultMaxImageBytes,
	}, nil
}

type UploadInput struct {
	ProductID    string
	OriginalName string
	ContentType  string
	SortOrder    int
	Body         io.Reader
}

// UploadProductImage follows:
// pending metadata -> object upload -> HEAD checksum/size verification -> ready.
// Determinate upload/verification failures are compensated best-effort.
// Ambiguous metadata-finalization results are left intact for an idempotent
// retry or reconciliation. The method never silently switches storage provider.
func (s *Service) UploadProductImage(
	ctx context.Context,
	scope dataaccess.CommandScope,
	input UploadInput,
) (dataaccess.ObjectMetadata, error) {
	if err := scope.Validate(); err != nil {
		return dataaccess.ObjectMetadata{}, err
	}
	contentType := strings.ToLower(strings.TrimSpace(input.ContentType))
	if strings.TrimSpace(input.ProductID) == "" ||
		strings.TrimSpace(input.OriginalName) == "" ||
		input.SortOrder < 0 ||
		input.Body == nil {
		return dataaccess.ObjectMetadata{}, dataaccess.ErrInvalidArgument
	}
	if _, allowed := allowedImageTypes[contentType]; !allowed {
		return dataaccess.ObjectMetadata{}, dataaccess.ErrInvalidArgument
	}

	objectID := objectIDForScope(scope)
	pending, err := s.metadata.CreatePendingObject(ctx, scope, dataaccess.PendingObjectInput{
		ObjectID:     objectID,
		ProductID:    input.ProductID,
		OriginalName: input.OriginalName,
		ContentType:  contentType,
		SortOrder:    input.SortOrder,
	})
	if err != nil {
		return dataaccess.ObjectMetadata{}, err
	}
	switch pending.Status {
	case dataaccess.ObjectReady:
		// A whole-operation retry may arrive after MarkObjectReady committed but
		// its response was lost. Reuse that committed result without rewriting
		// bytes or consuming the new request body.
		head, headErr := s.blobs.Head(ctx, scope.TenantID, pending.ID)
		if headErr != nil {
			return dataaccess.ObjectMetadata{}, fmt.Errorf("objectservice: verify replayed ready object: %w", headErr)
		}
		if !head.Exists ||
			head.ChecksumSHA256 != pending.ChecksumSHA256 ||
			head.SizeBytes != pending.SizeBytes {
			return dataaccess.ObjectMetadata{}, fmt.Errorf("%w: replayed ready object is inconsistent", dataaccess.ErrPrecondition)
		}
		return pending, nil
	case dataaccess.ObjectPending:
		// Continue the two-phase lifecycle.
	default:
		return dataaccess.ObjectMetadata{}, dataaccess.ErrConflict
	}

	receipt, err := s.blobs.Put(ctx, scope.TenantID, pending.ID, contentType, s.maxBytes, input.Body)
	if err != nil {
		deleteErr, markErr := s.compensateFailure(ctx, scope, pending.ID, failureUpload)
		return dataaccess.ObjectMetadata{}, errors.Join(
			fmt.Errorf("objectservice: upload: %w", err),
			deleteErr,
			markErr,
		)
	}
	head, err := s.blobs.Head(ctx, scope.TenantID, pending.ID)
	if err != nil || !head.Exists ||
		head.ChecksumSHA256 != receipt.ChecksumSHA256 ||
		head.SizeBytes != receipt.SizeBytes {
		deleteErr, markErr := s.compensateFailure(ctx, scope, pending.ID, failureVerify)
		return dataaccess.ObjectMetadata{}, errors.Join(
			fmt.Errorf("objectservice: verify uploaded object"),
			err,
			deleteErr,
			markErr,
		)
	}

	ready, err := s.metadata.MarkObjectReady(
		ctx,
		childScope(scope, "ready"),
		pending.ID,
		receipt,
		s.now(),
	)
	if err != nil {
		// MarkObjectReady may have committed before its response was lost.
		// Deleting bytes or forcing failed metadata here could turn a valid
		// ready row into a dangling reference. Leave both sides intact for the
		// idempotent whole-operation retry or reconciliation.
		return dataaccess.ObjectMetadata{}, fmt.Errorf("objectservice: finalize metadata: %w", err)
	}
	return ready, nil
}

func (s *Service) compensateFailure(
	parent context.Context,
	scope dataaccess.CommandScope,
	objectID, reason string,
) (deleteErr, markErr error) {
	// Compensation must still run after the request deadline or cancellation.
	// Each side gets an independent bounded attempt so a slow blob delete does
	// not prevent failed metadata from being recorded.
	base := context.WithoutCancel(parent)
	deleteCtx, cancelDelete := context.WithTimeout(base, compensationTimeout)
	deleteErr = s.blobs.Delete(deleteCtx, scope.TenantID, objectID)
	cancelDelete()

	markCtx, cancelMark := context.WithTimeout(base, compensationTimeout)
	markErr = s.metadata.MarkObjectFailed(
		markCtx,
		childScope(scope, failureScopeSuffix(reason)),
		objectID,
		reason,
	)
	cancelMark()
	return deleteErr, markErr
}

func failureScopeSuffix(reason string) string {
	switch reason {
	case failureUpload:
		return "upload-failed"
	case failureVerify:
		return "verify-failed"
	default:
		return "failed"
	}
}

func childScope(scope dataaccess.CommandScope, suffix string) dataaccess.CommandScope {
	value := sha256.Sum256([]byte(scope.IdempotencyKey + ":" + suffix))
	scope.IdempotencyKey = hex.EncodeToString(value[:])
	return scope
}

func objectIDForScope(scope dataaccess.CommandScope) string {
	// Domain separation plus length-delimited fields avoids concatenation
	// ambiguity. The same tenant/actor/idempotency key produces the same opaque
	// collision-resistant ID across processes and retries.
	hash := sha256.New()
	for _, value := range []string{
		"zaiko:product-object:v1",
		strings.TrimSpace(scope.TenantID),
		strings.TrimSpace(scope.ActorID),
		strings.TrimSpace(scope.IdempotencyKey),
	} {
		_, _ = fmt.Fprintf(hash, "%d:", len(value))
		_, _ = io.WriteString(hash, value)
	}
	return hex.EncodeToString(hash.Sum(nil))
}
