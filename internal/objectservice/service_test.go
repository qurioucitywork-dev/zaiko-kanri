package objectservice

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/dataaccess"
)

type metadataStub struct {
	pending      dataaccess.ObjectMetadata
	ready        bool
	failed       string
	readyErr     error
	createCalls  int
	readyCalls   int
	failedCtxErr error
}

func (m *metadataStub) CreatePendingObject(
	_ context.Context,
	scope dataaccess.CommandScope,
	input dataaccess.PendingObjectInput,
) (dataaccess.ObjectMetadata, error) {
	m.createCalls++
	if m.pending.ID != "" {
		if m.pending.ID != input.ObjectID {
			return dataaccess.ObjectMetadata{}, dataaccess.ErrIdempotencyMismatch
		}
		return m.pending, nil
	}
	m.pending = dataaccess.ObjectMetadata{
		ID:           input.ObjectID,
		TenantID:     scope.TenantID,
		ProductID:    input.ProductID,
		OriginalName: input.OriginalName,
		ContentType:  input.ContentType,
		SortOrder:    input.SortOrder,
		Status:       dataaccess.ObjectPending,
	}
	return m.pending, nil
}

func (m *metadataStub) MarkObjectReady(
	_ context.Context,
	_ dataaccess.CommandScope,
	_ string,
	receipt dataaccess.BlobReceipt,
	readyAt time.Time,
) (dataaccess.ObjectMetadata, error) {
	m.readyCalls++
	m.ready = true
	m.pending.Status = dataaccess.ObjectReady
	m.pending.ChecksumSHA256 = receipt.ChecksumSHA256
	m.pending.SizeBytes = receipt.SizeBytes
	m.pending.ReadyAt = readyAt
	return m.pending, m.readyErr
}

func (m *metadataStub) MarkObjectFailed(
	ctx context.Context,
	_ dataaccess.CommandScope,
	_ string,
	reasonCode string,
) error {
	m.failedCtxErr = ctx.Err()
	m.failed = reasonCode
	return nil
}

func (m *metadataStub) MarkObjectDeleted(
	context.Context,
	dataaccess.CommandScope,
	string,
	time.Time,
) error {
	return nil
}

type blobStub struct {
	receipt      dataaccess.BlobReceipt
	head         dataaccess.BlobHead
	putErr       error
	headErr      error
	deleted      bool
	putCalls     int
	headCalls    int
	putHook      func()
	deleteCtxErr error
}

func (b *blobStub) Put(
	_ context.Context,
	_, _, _ string,
	_ int64,
	body io.Reader,
) (dataaccess.BlobReceipt, error) {
	b.putCalls++
	if b.putHook != nil {
		b.putHook()
	}
	_, _ = io.Copy(io.Discard, body)
	return b.receipt, b.putErr
}

func (b *blobStub) Head(context.Context, string, string) (dataaccess.BlobHead, error) {
	b.headCalls++
	return b.head, b.headErr
}

func (b *blobStub) Open(context.Context, string, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

func (b *blobStub) Delete(ctx context.Context, _ string, _ string) error {
	b.deleteCtxErr = ctx.Err()
	b.deleted = true
	return nil
}

func uploadScope() dataaccess.CommandScope {
	return dataaccess.CommandScope{
		TenantID:       "tenant-a",
		ActorID:        "user-a",
		IdempotencyKey: "upload-001",
		RequestedAt:    time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC),
	}
}

func TestUploadProductImageMarksReadyAfterVerification(t *testing.T) {
	metadata := &metadataStub{}
	blobs := &blobStub{
		receipt: dataaccess.BlobReceipt{ChecksumSHA256: "abc", SizeBytes: 3},
		head:    dataaccess.BlobHead{ChecksumSHA256: "abc", SizeBytes: 3, Exists: true},
	}
	service, err := New(metadata, blobs)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.Date(2026, 7, 31, 1, 0, 0, 0, time.UTC) }

	got, err := service.UploadProductImage(context.Background(), uploadScope(), UploadInput{
		ProductID:    "product-a",
		OriginalName: "watch.jpg",
		ContentType:  "image/jpeg",
		Body:         bytes.NewBufferString("abc"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !metadata.ready || got.Status != dataaccess.ObjectReady || blobs.deleted {
		t.Fatalf("unexpected lifecycle: ready=%v status=%q deleted=%v", metadata.ready, got.Status, blobs.deleted)
	}
}

func TestUploadProductImageCompensatesVerificationMismatch(t *testing.T) {
	metadata := &metadataStub{}
	blobs := &blobStub{
		receipt: dataaccess.BlobReceipt{ChecksumSHA256: "abc", SizeBytes: 3},
		head:    dataaccess.BlobHead{ChecksumSHA256: "different", SizeBytes: 3, Exists: true},
	}
	service, err := New(metadata, blobs)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.UploadProductImage(context.Background(), uploadScope(), UploadInput{
		ProductID:    "product-a",
		OriginalName: "watch.jpg",
		ContentType:  "image/jpeg",
		Body:         bytes.NewBufferString("abc"),
	})
	if err == nil {
		t.Fatal("expected verification error")
	}
	if !blobs.deleted || metadata.failed != failureVerify {
		t.Fatalf("compensation missing: deleted=%v failed=%q", blobs.deleted, metadata.failed)
	}
}

func TestUploadProductImageCompensatesPartialUploadFailure(t *testing.T) {
	metadata := &metadataStub{}
	blobs := &blobStub{putErr: errors.New("provider upload failed")}
	service, err := New(metadata, blobs)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.UploadProductImage(context.Background(), uploadScope(), UploadInput{
		ProductID:    "product-a",
		OriginalName: "watch.jpg",
		ContentType:  "image/jpeg",
		Body:         bytes.NewBufferString("abc"),
	})
	if err == nil {
		t.Fatal("expected upload error")
	}
	if !blobs.deleted || metadata.failed != failureUpload {
		t.Fatalf("compensation missing: deleted=%v failed=%q", blobs.deleted, metadata.failed)
	}
}

func TestUploadProductImageWholeOperationRetryReusesCommittedReadyResult(t *testing.T) {
	metadata := &metadataStub{}
	blobs := &blobStub{
		receipt: dataaccess.BlobReceipt{ChecksumSHA256: "abc", SizeBytes: 3},
		head:    dataaccess.BlobHead{ChecksumSHA256: "abc", SizeBytes: 3, Exists: true},
	}
	service, err := New(metadata, blobs)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.Date(2026, 7, 31, 1, 0, 0, 0, time.UTC) }
	scope := uploadScope()
	input := func(body string) UploadInput {
		return UploadInput{
			ProductID: "product-a", OriginalName: "watch.jpg",
			ContentType: "image/jpeg", Body: bytes.NewBufferString(body),
		}
	}

	first, err := service.UploadProductImage(context.Background(), scope, input("abc"))
	if err != nil {
		t.Fatal(err)
	}
	scope.RequestedAt = scope.RequestedAt.Add(time.Minute)
	replayed, err := service.UploadProductImage(context.Background(), scope, input("must-not-be-uploaded"))
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == "" || replayed.ID != first.ID || replayed.Status != dataaccess.ObjectReady {
		t.Fatalf("first=%#v replayed=%#v", first, replayed)
	}
	if metadata.createCalls != 2 || metadata.readyCalls != 1 ||
		blobs.putCalls != 1 || blobs.headCalls != 2 {
		t.Fatalf(
			"retry repeated side effects: create=%d ready=%d put=%d head=%d",
			metadata.createCalls, metadata.readyCalls, blobs.putCalls, blobs.headCalls,
		)
	}
}

func TestUploadProductImageReadyResponseLossDoesNotDeleteCommittedBlob(t *testing.T) {
	responseLost := errors.New("ready response lost")
	metadata := &metadataStub{readyErr: responseLost}
	blobs := &blobStub{
		receipt: dataaccess.BlobReceipt{ChecksumSHA256: "abc", SizeBytes: 3},
		head:    dataaccess.BlobHead{ChecksumSHA256: "abc", SizeBytes: 3, Exists: true},
	}
	service, err := New(metadata, blobs)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.Date(2026, 7, 31, 1, 0, 0, 0, time.UTC) }

	_, err = service.UploadProductImage(context.Background(), uploadScope(), UploadInput{
		ProductID: "product-a", OriginalName: "watch.jpg",
		ContentType: "image/jpeg", Body: bytes.NewBufferString("abc"),
	})
	if !errors.Is(err, responseLost) {
		t.Fatalf("error = %v, want response loss", err)
	}
	if blobs.deleted || metadata.failed != "" || metadata.pending.Status != dataaccess.ObjectReady {
		t.Fatalf(
			"ambiguous finalize was destructively compensated: deleted=%v failed=%q status=%q",
			blobs.deleted, metadata.failed, metadata.pending.Status,
		)
	}

	// A caller retry with the same key resolves from committed metadata and
	// verified bytes without another PUT or MarkObjectReady.
	metadata.readyErr = nil
	got, err := service.UploadProductImage(context.Background(), uploadScope(), UploadInput{
		ProductID: "product-a", OriginalName: "watch.jpg",
		ContentType: "image/jpeg", Body: bytes.NewBufferString("replacement"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != dataaccess.ObjectReady || blobs.putCalls != 1 || metadata.readyCalls != 1 {
		t.Fatalf("retry did not reuse committed result: got=%#v put=%d ready=%d", got, blobs.putCalls, metadata.readyCalls)
	}
}

func TestUploadProductImageCompensationUsesIndependentContextAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	metadata := &metadataStub{}
	blobs := &blobStub{
		putErr:  errors.New("provider upload failed"),
		putHook: cancel,
	}
	service, err := New(metadata, blobs)
	if err != nil {
		t.Fatal(err)
	}

	_, err = service.UploadProductImage(ctx, uploadScope(), UploadInput{
		ProductID: "product-a", OriginalName: "watch.jpg",
		ContentType: "image/jpeg", Body: bytes.NewBufferString("abc"),
	})
	if err == nil {
		t.Fatal("expected upload error")
	}
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("request context error = %v", ctx.Err())
	}
	if !blobs.deleted || metadata.failed != failureUpload {
		t.Fatalf("compensation missing: deleted=%v failed=%q", blobs.deleted, metadata.failed)
	}
	if blobs.deleteCtxErr != nil || metadata.failedCtxErr != nil {
		t.Fatalf(
			"compensation inherited request cancellation: delete=%v metadata=%v",
			blobs.deleteCtxErr, metadata.failedCtxErr,
		)
	}
}

func TestUploadProductImageRejectsUnsupportedMIME(t *testing.T) {
	service, err := New(&metadataStub{}, &blobStub{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.UploadProductImage(context.Background(), uploadScope(), UploadInput{
		ProductID:    "product-a",
		OriginalName: "watch.svg",
		ContentType:  "image/svg+xml",
		Body:         strings.NewReader("<svg/>"),
	})
	if !errors.Is(err, dataaccess.ErrInvalidArgument) {
		t.Fatalf("error = %v, want ErrInvalidArgument", err)
	}
}
