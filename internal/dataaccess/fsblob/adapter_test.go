package fsblob

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/dataaccess"
)

var onePixelPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
}

func TestAdapterLifecycleAndTenantIsolation(t *testing.T) {
	adapter, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := adapter.Put(
		context.Background(),
		"tenant-a",
		"object-a",
		"image/png",
		1024,
		bytes.NewReader(onePixelPNG),
	)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.SizeBytes != int64(len(onePixelPNG)) || receipt.ChecksumSHA256 == "" {
		t.Fatalf("unexpected receipt: %+v", receipt)
	}
	head, err := adapter.Head(context.Background(), "tenant-a", "object-a")
	if err != nil {
		t.Fatal(err)
	}
	if !head.Exists || head != (dataaccess.BlobHead{
		ChecksumSHA256: receipt.ChecksumSHA256,
		SizeBytes:      receipt.SizeBytes,
		Exists:         true,
	}) {
		t.Fatalf("unexpected head: %+v", head)
	}
	otherTenant, err := adapter.Head(context.Background(), "tenant-b", "object-a")
	if err != nil {
		t.Fatal(err)
	}
	if otherTenant.Exists {
		t.Fatal("object leaked across tenants")
	}
	body, err := adapter.Open(context.Background(), "tenant-a", "object-a")
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil || !bytes.Equal(got, onePixelPNG) {
		t.Fatalf("read = %x, %v", got, err)
	}
	if err := adapter.Delete(context.Background(), "tenant-a", "object-a"); err != nil {
		t.Fatal(err)
	}
	head, err = adapter.Head(context.Background(), "tenant-a", "object-a")
	if err != nil || head.Exists {
		t.Fatalf("deleted head = %+v, %v", head, err)
	}
}

func TestAdapterRejectsTraversalOversizeAndMIMEClaim(t *testing.T) {
	adapter, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name        string
		objectID    string
		contentType string
		maxBytes    int64
		body        []byte
	}{
		{name: "traversal", objectID: "../escape", contentType: "image/png", maxBytes: 1024, body: onePixelPNG},
		{name: "oversize", objectID: "large", contentType: "image/png", maxBytes: 4, body: onePixelPNG},
		{name: "mime mismatch", objectID: "text", contentType: "image/png", maxBytes: 1024, body: []byte("plain text")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := adapter.Put(
				context.Background(),
				"tenant-a",
				test.objectID,
				test.contentType,
				test.maxBytes,
				bytes.NewReader(test.body),
			)
			if !errors.Is(err, dataaccess.ErrInvalidArgument) {
				t.Fatalf("error = %v, want ErrInvalidArgument", err)
			}
		})
	}
}

func TestAdapterRejectsOverwrite(t *testing.T) {
	adapter, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 2; index++ {
		_, err = adapter.Put(
			context.Background(),
			"tenant-a",
			"object-a",
			"image/png",
			1024,
			bytes.NewReader(onePixelPNG),
		)
		if index == 0 && err != nil {
			t.Fatal(err)
		}
	}
	if !errors.Is(err, dataaccess.ErrConflict) {
		t.Fatalf("second Put error = %v, want ErrConflict", err)
	}
}
