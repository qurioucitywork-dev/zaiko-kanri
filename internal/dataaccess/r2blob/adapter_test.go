package r2blob

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/dataaccess"
)

var onePixelPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
}

type fakeR2 struct {
	mu      sync.Mutex
	objects map[string][]byte
	hashes  map[string]string
	auth    []string
}

func newFakeR2() *fakeR2 {
	return &fakeR2{
		objects: map[string][]byte{},
		hashes:  map[string]string{},
	}
}

func (r *fakeR2) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.auth = append(r.auth, request.Header.Get("Authorization"))
	body, exists := r.objects[request.URL.Path]
	switch request.Method {
	case http.MethodPut:
		if exists {
			writer.WriteHeader(http.StatusPreconditionFailed)
			return
		}
		value, err := io.ReadAll(request.Body)
		if err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		r.objects[request.URL.Path] = value
		r.hashes[request.URL.Path] = request.Header.Get("x-amz-meta-sha256")
		writer.WriteHeader(http.StatusOK)
	case http.MethodHead:
		if !exists {
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		writer.Header().Set("Content-Length", strconv.Itoa(len(body)))
		writer.Header().Set("x-amz-meta-sha256", r.hashes[request.URL.Path])
		writer.WriteHeader(http.StatusOK)
	case http.MethodGet:
		if !exists {
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write(body)
	case http.MethodDelete:
		delete(r.objects, request.URL.Path)
		delete(r.hashes, request.URL.Path)
		writer.WriteHeader(http.StatusNoContent)
	default:
		writer.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func TestAdapterUsesR2AutoRegionAndBlobContract(t *testing.T) {
	backend := newFakeR2()
	server := httptest.NewServer(backend)
	defer server.Close()
	adapter, err := New(Config{
		Endpoint: server.URL, Bucket: "private-images", Prefix: "preview",
		AccessKeyID: "R2ACCESS", SecretAccessKey: "r2-contract-secret",
		AllowInsecure: true,
		Clock: func() time.Time {
			return time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := adapter.Put(
		context.Background(), "tenant-a", "object-a", "image/png",
		1024, bytes.NewReader(onePixelPNG),
	)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(onePixelPNG)
	if receipt != (dataaccess.BlobReceipt{
		ChecksumSHA256: hex.EncodeToString(sum[:]),
		SizeBytes:      int64(len(onePixelPNG)),
	}) {
		t.Fatalf("receipt = %#v", receipt)
	}
	head, err := adapter.Head(context.Background(), "tenant-a", "object-a")
	if err != nil || !head.Exists ||
		head.ChecksumSHA256 != receipt.ChecksumSHA256 ||
		head.SizeBytes != receipt.SizeBytes {
		t.Fatalf("head = %#v, %v", head, err)
	}
	crossTenant, err := adapter.Head(context.Background(), "tenant-b", "object-a")
	if err != nil || crossTenant.Exists {
		t.Fatalf("cross-tenant head = %#v, %v", crossTenant, err)
	}
	body, err := adapter.Open(context.Background(), "tenant-a", "object-a")
	if err != nil {
		t.Fatal(err)
	}
	opened, readErr := io.ReadAll(body)
	_ = body.Close()
	if readErr != nil || !bytes.Equal(opened, onePixelPNG) {
		t.Fatalf("opened = %x, %v", opened, readErr)
	}
	if err := adapter.Delete(context.Background(), "tenant-a", "object-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Open(context.Background(), "tenant-a", "object-a"); !errors.Is(err, dataaccess.ErrNotFound) {
		t.Fatalf("deleted Open error = %v, want ErrNotFound", err)
	}

	backend.mu.Lock()
	defer backend.mu.Unlock()
	for _, authorization := range backend.auth {
		if !strings.Contains(authorization, "Credential=R2ACCESS/20260731/auto/s3/aws4_request") {
			t.Fatalf("R2 did not use auto region: %q", authorization)
		}
		if strings.Contains(authorization, "r2-contract-secret") {
			t.Fatalf("secret exposed in authorization: %q", authorization)
		}
	}
}

func TestAdapterCancellationAndRetryClassification(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	adapter, err := New(Config{
		Endpoint: server.URL, Bucket: "private-images", Prefix: "preview",
		AccessKeyID: "R2ACCESS", SecretAccessKey: "r2-contract-secret",
		AllowInsecure: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.Head(context.Background(), "tenant", "object")
	if err == nil || !IsRetryable(err) {
		t.Fatalf("error = %v, want retryable", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := adapter.Head(ctx, "tenant", "object"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled error = %v", err)
	}
}

func TestNewRejectsMissingOrInsecureR2Configuration(t *testing.T) {
	base := Config{
		Endpoint: "https://account.r2.cloudflarestorage.com",
		Bucket:   "private-images", Prefix: "preview",
		AccessKeyID: "access", SecretAccessKey: "secret",
	}
	tests := []Config{
		func() Config { value := base; value.Endpoint = ""; return value }(),
		func() Config { value := base; value.Endpoint = "http://account.r2.example"; return value }(),
		func() Config { value := base; value.Bucket = ""; return value }(),
		func() Config { value := base; value.Prefix = "../preview"; return value }(),
		func() Config { value := base; value.AccessKeyID = ""; return value }(),
	}
	for index, config := range tests {
		if _, err := New(config); !errors.Is(err, dataaccess.ErrInvalidArgument) {
			t.Fatalf("case %d error = %v, want ErrInvalidArgument", index, err)
		}
	}
}
