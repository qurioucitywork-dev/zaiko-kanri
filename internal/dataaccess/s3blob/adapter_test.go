package s3blob

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

type storedObject struct {
	body        []byte
	contentType string
	checksum    string
}

type fakeS3 struct {
	mu      sync.Mutex
	objects map[string]storedObject
	paths   []string
	auth    []string
}

func newFakeS3() *fakeS3 {
	return &fakeS3{objects: map[string]storedObject{}}
}

func (s *fakeS3) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.paths = append(s.paths, request.URL.Path)
	s.auth = append(s.auth, request.Header.Get("Authorization"))
	if request.Header.Get("Authorization") == "" ||
		request.Header.Get("x-amz-date") == "" ||
		request.Header.Get("x-amz-content-sha256") == "" {
		writer.WriteHeader(http.StatusForbidden)
		return
	}
	object, exists := s.objects[request.URL.Path]
	switch request.Method {
	case http.MethodPut:
		if exists && request.Header.Get("If-None-Match") == "*" {
			writer.WriteHeader(http.StatusPreconditionFailed)
			return
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		sum := sha256.Sum256(body)
		checksum := hex.EncodeToString(sum[:])
		if request.Header.Get("x-amz-content-sha256") != checksum ||
			request.Header.Get("x-amz-meta-sha256") != checksum {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		s.objects[request.URL.Path] = storedObject{
			body:        body,
			contentType: request.Header.Get("Content-Type"),
			checksum:    checksum,
		}
		writer.WriteHeader(http.StatusOK)
	case http.MethodHead:
		if !exists {
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		writer.Header().Set("Content-Length", strconv.Itoa(len(object.body)))
		writer.Header().Set("Content-Type", object.contentType)
		writer.Header().Set("x-amz-meta-sha256", object.checksum)
		writer.WriteHeader(http.StatusOK)
	case http.MethodGet:
		if !exists {
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		writer.Header().Set("Content-Type", object.contentType)
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write(object.body)
	case http.MethodDelete:
		delete(s.objects, request.URL.Path)
		writer.WriteHeader(http.StatusNoContent)
	default:
		writer.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func testConfig(endpoint string) Config {
	return Config{
		Endpoint: endpoint, Bucket: "private-images", Region: "ap-northeast-1",
		Prefix: "test", AccessKeyID: "AKIDEXAMPLE",
		SecretAccessKey: "contract-secret-value",
		AllowInsecure:   true,
		Clock: func() time.Time {
			return time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)
		},
	}
}

func TestAdapterLifecycleTenantIsolationAndSigning(t *testing.T) {
	backend := newFakeS3()
	server := httptest.NewServer(backend)
	defer server.Close()
	adapter, err := New(testConfig(server.URL))
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
	if receipt.SizeBytes != int64(len(onePixelPNG)) || !validSHA256(receipt.ChecksumSHA256) {
		t.Fatalf("receipt = %#v", receipt)
	}
	head, err := adapter.Head(context.Background(), "tenant-a", "object-a")
	if err != nil {
		t.Fatal(err)
	}
	if head != (dataaccess.BlobHead{
		ChecksumSHA256: receipt.ChecksumSHA256,
		SizeBytes:      receipt.SizeBytes,
		Exists:         true,
	}) {
		t.Fatalf("head = %#v", head)
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
	opened, readErr := io.ReadAll(body)
	closeErr := body.Close()
	if readErr != nil || closeErr != nil || !bytes.Equal(opened, onePixelPNG) {
		t.Fatalf("opened = %x, read=%v close=%v", opened, readErr, closeErr)
	}
	if _, err := adapter.Put(
		context.Background(), "tenant-a", "object-a", "image/png",
		1024, bytes.NewReader(onePixelPNG),
	); !errors.Is(err, dataaccess.ErrConflict) {
		t.Fatalf("overwrite error = %v, want ErrConflict", err)
	}
	if err := adapter.Delete(context.Background(), "tenant-a", "object-a"); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Delete(context.Background(), "tenant-a", "object-a"); err != nil {
		t.Fatalf("idempotent delete: %v", err)
	}
	deleted, err := adapter.Head(context.Background(), "tenant-a", "object-a")
	if err != nil || deleted.Exists {
		t.Fatalf("deleted head = %#v, %v", deleted, err)
	}

	backend.mu.Lock()
	defer backend.mu.Unlock()
	if len(backend.paths) == 0 || strings.Contains(strings.Join(backend.paths, "\n"), "tenant-a") {
		t.Fatalf("tenant ID was not hashed in paths: %#v", backend.paths)
	}
	for _, authorization := range backend.auth {
		if !strings.Contains(authorization, "Credential=AKIDEXAMPLE/20260731/ap-northeast-1/s3/aws4_request") {
			t.Fatalf("unexpected signature scope: %q", authorization)
		}
	}
}

func TestAdapterRejectsInvalidInputBeforeHTTP(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests++
	}))
	defer server.Close()
	adapter, err := New(testConfig(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name        string
		tenantID    string
		objectID    string
		contentType string
		maxBytes    int64
		body        []byte
	}{
		{name: "empty tenant", objectID: "object", contentType: "image/png", maxBytes: 1024, body: onePixelPNG},
		{name: "unsafe object", tenantID: "tenant", objectID: "../object", contentType: "image/png", maxBytes: 1024, body: onePixelPNG},
		{name: "unsupported MIME", tenantID: "tenant", objectID: "object", contentType: "text/plain", maxBytes: 1024, body: []byte("text")},
		{name: "MIME mismatch", tenantID: "tenant", objectID: "object", contentType: "image/png", maxBytes: 1024, body: []byte("text")},
		{name: "oversize", tenantID: "tenant", objectID: "object", contentType: "image/png", maxBytes: 4, body: onePixelPNG},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := adapter.Put(
				context.Background(), test.tenantID, test.objectID,
				test.contentType, test.maxBytes, bytes.NewReader(test.body),
			)
			if !errors.Is(err, dataaccess.ErrInvalidArgument) {
				t.Fatalf("error = %v, want ErrInvalidArgument", err)
			}
		})
	}
	if requests != 0 {
		t.Fatalf("invalid input made %d HTTP request(s)", requests)
	}
}

func TestAdapterHonorsCancellation(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests++
	}))
	defer server.Close()
	adapter, err := New(testConfig(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := adapter.Put(
		ctx, "tenant", "object", "image/png", 1024, bytes.NewReader(onePixelPNG),
	); !errors.Is(err, context.Canceled) {
		t.Fatalf("Put error = %v, want context.Canceled", err)
	}
	if _, err := adapter.Head(ctx, "tenant", "object"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Head error = %v, want context.Canceled", err)
	}
	if _, err := adapter.Open(ctx, "tenant", "object"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Open error = %v, want context.Canceled", err)
	}
	if err := adapter.Delete(ctx, "tenant", "object"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Delete error = %v, want context.Canceled", err)
	}
	if requests != 0 {
		t.Fatalf("canceled operations made %d HTTP request(s)", requests)
	}
}

type signerFunc func(context.Context, *http.Request, string, time.Time) error

func (fn signerFunc) Sign(
	ctx context.Context,
	request *http.Request,
	payloadSHA256 string,
	at time.Time,
) error {
	return fn(ctx, request, payloadSHA256, at)
}

func TestSignerCancellationIsNotSanitized(t *testing.T) {
	config := testConfig("https://s3.ap-northeast-1.amazonaws.com")
	config.AllowInsecure = false
	config.Signer = signerFunc(func(
		context.Context,
		*http.Request,
		string,
		time.Time,
	) error {
		return context.DeadlineExceeded
	})
	adapter, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Head(
		context.Background(), "tenant", "object",
	); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Head error = %v, want context.DeadlineExceeded", err)
	}
}

func TestErrorsAreSanitizedAndClassifiedForRetry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusServiceUnavailable)
		_, _ = writer.Write([]byte(
			"contract-secret-value private-images " + request.URL.Path,
		))
	}))
	defer server.Close()
	config := testConfig(server.URL + "/private-provider-path")
	adapter, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.Head(context.Background(), "tenant-secret", "object-secret")
	if err == nil || !IsRetryable(err) {
		t.Fatalf("error = %v, want retryable", err)
	}
	text := err.Error()
	for _, forbidden := range []string{
		"contract-secret-value", "private-images", "private-provider-path",
		"tenant-secret", "object-secret", server.URL,
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("error exposed %q: %q", forbidden, text)
		}
	}
}

func TestSignedRedirectIsNotFollowed(t *testing.T) {
	var redirectedRequests int
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		redirectedRequests++
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, target.URL+"/capture", http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	adapter, err := New(testConfig(source.URL))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Head(context.Background(), "tenant", "object"); err == nil {
		t.Fatal("redirect response unexpectedly succeeded")
	}
	if redirectedRequests != 0 {
		t.Fatalf("signed request followed redirect %d time(s)", redirectedRequests)
	}
}

func TestNewRejectsUnsafeConfiguration(t *testing.T) {
	valid := testConfig("https://s3.ap-northeast-1.amazonaws.com")
	valid.AllowInsecure = false
	tests := []Config{
		func() Config { value := valid; value.Endpoint = "http://plain.example"; return value }(),
		func() Config { value := valid; value.Endpoint = "https://user:pass@example.com"; return value }(),
		func() Config { value := valid; value.Bucket = "../bucket"; return value }(),
		func() Config { value := valid; value.Prefix = "../production"; return value }(),
		func() Config { value := valid; value.Region = ""; return value }(),
		func() Config { value := valid; value.SecretAccessKey = ""; return value }(),
	}
	for index, config := range tests {
		if _, err := New(config); !errors.Is(err, dataaccess.ErrInvalidArgument) {
			t.Fatalf("case %d error = %v, want ErrInvalidArgument", index, err)
		}
	}
}
