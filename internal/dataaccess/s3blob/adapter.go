// Package s3blob implements dataaccess.ObjectBlobStore using the AWS S3 HTTP
// API. It intentionally uses the standard library and a narrow signing
// boundary instead of pulling the AWS SDK into the application.
package s3blob

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/dataaccess"
)

const emptyPayloadSHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

var (
	safeObjectID = regexp.MustCompile(`\A[a-zA-Z0-9_-]{1,128}\z`)
	safeBucket   = regexp.MustCompile(`\A[a-zA-Z0-9][a-zA-Z0-9._-]{1,61}[a-zA-Z0-9]\z`)
	safePrefix   = regexp.MustCompile(`\A[a-zA-Z0-9._/-]{1,256}\z`)

	_ dataaccess.ObjectBlobStore = (*Adapter)(nil)
)

// RequestSigner signs one already-constructed S3 request. Implementations must
// not mutate the request URL or body.
type RequestSigner interface {
	Sign(ctx context.Context, request *http.Request, payloadSHA256 string, at time.Time) error
}

// Config contains provider-private connection details. Values are retained by
// the adapter but are never included in returned errors.
type Config struct {
	Endpoint        string
	Bucket          string
	Region          string
	Prefix          string
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string

	HTTPClient    *http.Client
	Signer        RequestSigner
	Clock         func() time.Time
	AllowInsecure bool
}

// Adapter stores immutable bytes under a tenant-hashed key prefix.
type Adapter struct {
	endpoint *url.URL
	bucket   string
	region   string
	prefix   string
	client   *http.Client
	signer   RequestSigner
	now      func() time.Time
}

// New validates configuration without making a remote request.
func New(config Config) (*Adapter, error) {
	endpoint, err := url.Parse(strings.TrimSpace(config.Endpoint))
	if err != nil || endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return nil, dataaccess.ErrInvalidArgument
	}
	if endpoint.Scheme != "https" && !(config.AllowInsecure && endpoint.Scheme == "http") {
		return nil, dataaccess.ErrInvalidArgument
	}
	bucket := strings.TrimSpace(config.Bucket)
	region := strings.TrimSpace(config.Region)
	prefix := strings.Trim(strings.TrimSpace(config.Prefix), "/")
	if !safeBucket.MatchString(bucket) || region == "" || !validPrefix(prefix) {
		return nil, dataaccess.ErrInvalidArgument
	}

	signer := config.Signer
	if signer == nil {
		accessKeyID := strings.TrimSpace(config.AccessKeyID)
		if accessKeyID == "" || config.SecretAccessKey == "" {
			return nil, dataaccess.ErrInvalidArgument
		}
		signer = &sigV4Signer{
			accessKeyID:     accessKeyID,
			secretAccessKey: config.SecretAccessKey,
			sessionToken:    config.SessionToken,
			region:          region,
		}
	}
	clock := config.Clock
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}

	client := &http.Client{Timeout: 30 * time.Second}
	if config.HTTPClient != nil {
		clone := *config.HTTPClient
		client = &clone
	}
	// A signed Authorization header must never be forwarded to an endpoint
	// selected by a remote redirect.
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	return &Adapter{
		endpoint: endpoint,
		bucket:   bucket,
		region:   region,
		prefix:   prefix,
		client:   client,
		signer:   signer,
		now:      clock,
	}, nil
}

// Put uploads one immutable object and returns its locally calculated
// checksum and size. If-None-Match prevents accidental overwrite.
func (a *Adapter) Put(
	ctx context.Context,
	tenantID, objectID, contentType string,
	maxBytes int64,
	body io.Reader,
) (dataaccess.BlobReceipt, error) {
	if err := ctx.Err(); err != nil {
		return dataaccess.BlobReceipt{}, err
	}
	if err := validateLocator(tenantID, objectID); err != nil || maxBytes < 1 || body == nil {
		return dataaccess.BlobReceipt{}, dataaccess.ErrInvalidArgument
	}
	contentType, err := normalizeContentType(contentType)
	if err != nil {
		return dataaccess.BlobReceipt{}, err
	}
	payload, checksum, err := readPayload(ctx, body, maxBytes, contentType)
	if err != nil {
		return dataaccess.BlobReceipt{}, err
	}

	request, err := a.newRequest(ctx, http.MethodPut, tenantID, objectID, bytes.NewReader(payload), checksum)
	if err != nil {
		return dataaccess.BlobReceipt{}, err
	}
	request.ContentLength = int64(len(payload))
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("If-None-Match", "*")
	request.Header.Set("x-amz-meta-sha256", checksum)
	if err := a.signer.Sign(ctx, request, checksum, a.now().UTC()); err != nil {
		return dataaccess.BlobReceipt{}, normalizeSignerError(ctx, "put", err)
	}

	response, err := a.client.Do(request)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return dataaccess.BlobReceipt{}, ctxErr
		}
		return dataaccess.BlobReceipt{}, sanitizeError("put", true, err)
	}
	defer drainAndClose(response.Body)
	switch response.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent:
		return dataaccess.BlobReceipt{
			ChecksumSHA256: checksum,
			SizeBytes:      int64(len(payload)),
		}, nil
	case http.StatusConflict, http.StatusPreconditionFailed:
		return dataaccess.BlobReceipt{}, dataaccess.ErrConflict
	default:
		return dataaccess.BlobReceipt{}, statusError("put", response.StatusCode)
	}
}

// Head reads verification metadata without opening object bytes.
func (a *Adapter) Head(ctx context.Context, tenantID, objectID string) (dataaccess.BlobHead, error) {
	if err := ctx.Err(); err != nil {
		return dataaccess.BlobHead{}, err
	}
	if err := validateLocator(tenantID, objectID); err != nil {
		return dataaccess.BlobHead{}, err
	}
	request, err := a.newRequest(ctx, http.MethodHead, tenantID, objectID, nil, emptyPayloadSHA256)
	if err != nil {
		return dataaccess.BlobHead{}, err
	}
	if err := a.signer.Sign(ctx, request, emptyPayloadSHA256, a.now().UTC()); err != nil {
		return dataaccess.BlobHead{}, normalizeSignerError(ctx, "head", err)
	}
	response, err := a.client.Do(request)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return dataaccess.BlobHead{}, ctxErr
		}
		return dataaccess.BlobHead{}, sanitizeError("head", true, err)
	}
	defer drainAndClose(response.Body)
	switch response.StatusCode {
	case http.StatusNotFound:
		return dataaccess.BlobHead{Exists: false}, nil
	case http.StatusOK:
		checksum := strings.ToLower(strings.TrimSpace(response.Header.Get("x-amz-meta-sha256")))
		if !validSHA256(checksum) || response.ContentLength < 0 {
			return dataaccess.BlobHead{}, sanitizeError("head", false, errors.New("invalid verification metadata"))
		}
		return dataaccess.BlobHead{
			ChecksumSHA256: checksum,
			SizeBytes:      response.ContentLength,
			Exists:         true,
		}, nil
	default:
		return dataaccess.BlobHead{}, statusError("head", response.StatusCode)
	}
}

// Open returns a streaming response body. The caller must close it.
func (a *Adapter) Open(ctx context.Context, tenantID, objectID string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateLocator(tenantID, objectID); err != nil {
		return nil, err
	}
	request, err := a.newRequest(ctx, http.MethodGet, tenantID, objectID, nil, emptyPayloadSHA256)
	if err != nil {
		return nil, err
	}
	if err := a.signer.Sign(ctx, request, emptyPayloadSHA256, a.now().UTC()); err != nil {
		return nil, normalizeSignerError(ctx, "open", err)
	}
	response, err := a.client.Do(request)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, sanitizeError("open", true, err)
	}
	switch response.StatusCode {
	case http.StatusOK:
		return response.Body, nil
	case http.StatusNotFound:
		drainAndClose(response.Body)
		return nil, dataaccess.ErrNotFound
	default:
		drainAndClose(response.Body)
		return nil, statusError("open", response.StatusCode)
	}
}

// Delete is idempotent. A missing object is already deleted.
func (a *Adapter) Delete(ctx context.Context, tenantID, objectID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateLocator(tenantID, objectID); err != nil {
		return err
	}
	request, err := a.newRequest(ctx, http.MethodDelete, tenantID, objectID, nil, emptyPayloadSHA256)
	if err != nil {
		return err
	}
	if err := a.signer.Sign(ctx, request, emptyPayloadSHA256, a.now().UTC()); err != nil {
		return normalizeSignerError(ctx, "delete", err)
	}
	response, err := a.client.Do(request)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return sanitizeError("delete", true, err)
	}
	defer drainAndClose(response.Body)
	switch response.StatusCode {
	case http.StatusOK, http.StatusAccepted, http.StatusNoContent, http.StatusNotFound:
		return nil
	default:
		return statusError("delete", response.StatusCode)
	}
}

func (a *Adapter) newRequest(
	ctx context.Context,
	method, tenantID, objectID string,
	body io.Reader,
	payloadSHA256 string,
) (*http.Request, error) {
	if a == nil || a.endpoint == nil || a.client == nil || a.signer == nil {
		return nil, dataaccess.ErrInvalidArgument
	}
	target := *a.endpoint
	basePath := strings.TrimRight(target.Path, "/")
	objectKey := a.objectKey(tenantID, objectID)
	target.Path = basePath + "/" + a.bucket + "/" + objectKey
	target.RawPath = ""
	request, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return nil, sanitizeError(strings.ToLower(method), false, err)
	}
	request.Header.Set("x-amz-content-sha256", payloadSHA256)
	return request, nil
}

func (a *Adapter) objectKey(tenantID, objectID string) string {
	tenantHash := sha256.Sum256([]byte(strings.TrimSpace(tenantID)))
	return a.prefix + "/" + hex.EncodeToString(tenantHash[:]) + "/" + objectID
}

func validateLocator(tenantID, objectID string) error {
	if strings.TrimSpace(tenantID) == "" || !safeObjectID.MatchString(objectID) {
		return dataaccess.ErrInvalidArgument
	}
	return nil
}

func validPrefix(prefix string) bool {
	if prefix == "" || !safePrefix.MatchString(prefix) {
		return false
	}
	for _, segment := range strings.Split(prefix, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

func normalizeContentType(value string) (string, error) {
	mediaType, parameters, err := mime.ParseMediaType(strings.ToLower(strings.TrimSpace(value)))
	if err != nil || len(parameters) != 0 {
		return "", dataaccess.ErrInvalidArgument
	}
	switch mediaType {
	case "image/jpeg", "image/png", "image/webp":
		return mediaType, nil
	default:
		return "", dataaccess.ErrInvalidArgument
	}
}

func readPayload(
	ctx context.Context,
	body io.Reader,
	maxBytes int64,
	contentType string,
) ([]byte, string, error) {
	var buffer bytes.Buffer
	hasher := sha256.New()
	limited := io.LimitReader(body, maxBytes+1)
	written, err := copyContext(ctx, io.MultiWriter(&buffer, hasher), limited)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, "", ctxErr
		}
		return nil, "", sanitizeError("read upload", false, err)
	}
	if written == 0 || written > maxBytes {
		return nil, "", dataaccess.ErrInvalidArgument
	}
	if detected := http.DetectContentType(buffer.Bytes()); detected != contentType {
		return nil, "", dataaccess.ErrInvalidArgument
	}
	return buffer.Bytes(), hex.EncodeToString(hasher.Sum(nil)), nil
}

func copyContext(ctx context.Context, destination io.Writer, source io.Reader) (int64, error) {
	buffer := make([]byte, 32<<10)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		count, readErr := source.Read(buffer)
		if count > 0 {
			written, writeErr := destination.Write(buffer[:count])
			total += int64(written)
			if writeErr != nil {
				return total, writeErr
			}
			if written != count {
				return total, io.ErrShortWrite
			}
		}
		if errors.Is(readErr, io.EOF) {
			return total, nil
		}
		if readErr != nil {
			return total, readErr
		}
	}
}

func drainAndClose(body io.ReadCloser) {
	if body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(body, 4<<10))
	_ = body.Close()
}

type operationError struct {
	operation string
	status    int
	retryable bool
}

func (e *operationError) Error() string {
	if e.status > 0 {
		return fmt.Sprintf("s3blob: %s failed with HTTP status %d", e.operation, e.status)
	}
	return "s3blob: " + e.operation + " failed"
}

func (e *operationError) Temporary() bool { return e.retryable }

// IsRetryable reports whether an operation can be retried by a higher-level
// bounded backoff policy. The adapter itself does not hide retries.
func IsRetryable(err error) bool {
	var temporary interface{ Temporary() bool }
	return errors.As(err, &temporary) && temporary.Temporary()
}

func statusError(operation string, status int) error {
	retryable := status == http.StatusRequestTimeout ||
		status == http.StatusTooManyRequests ||
		status == http.StatusInternalServerError ||
		status == http.StatusBadGateway ||
		status == http.StatusServiceUnavailable ||
		status == http.StatusGatewayTimeout
	return &operationError{operation: operation, status: status, retryable: retryable}
}

func sanitizeError(operation string, retryable bool, cause error) error {
	_ = cause // Provider locators and signer internals must not cross the port.
	return &operationError{operation: operation, retryable: retryable}
}

func normalizeSignerError(ctx context.Context, operation string, cause error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	switch {
	case errors.Is(cause, context.Canceled):
		return context.Canceled
	case errors.Is(cause, context.DeadlineExceeded):
		return context.DeadlineExceeded
	default:
		return sanitizeError(operation, false, cause)
	}
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

type sigV4Signer struct {
	accessKeyID     string
	secretAccessKey string
	sessionToken    string
	region          string
}

func (s *sigV4Signer) Sign(
	ctx context.Context,
	request *http.Request,
	payloadSHA256 string,
	at time.Time,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	at = at.UTC()
	amzDate := at.Format("20060102T150405Z")
	shortDate := at.Format("20060102")
	request.Header.Set("x-amz-date", amzDate)
	request.Header.Set("x-amz-content-sha256", payloadSHA256)
	if s.sessionToken != "" {
		request.Header.Set("x-amz-security-token", s.sessionToken)
	}

	canonicalHeaders, signedHeaders := canonicalHeaders(request)
	canonicalRequest := strings.Join([]string{
		request.Method,
		request.URL.EscapedPath(),
		request.URL.Query().Encode(),
		canonicalHeaders,
		signedHeaders,
		payloadSHA256,
	}, "\n")
	scope := shortDate + "/" + s.region + "/s3/aws4_request"
	canonicalHash := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		hex.EncodeToString(canonicalHash[:]),
	}, "\n")

	dateKey := hmacSHA256([]byte("AWS4"+s.secretAccessKey), shortDate)
	regionKey := hmacSHA256(dateKey, s.region)
	serviceKey := hmacSHA256(regionKey, "s3")
	signingKey := hmacSHA256(serviceKey, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(signingKey, stringToSign))
	request.Header.Set("Authorization",
		"AWS4-HMAC-SHA256 Credential="+s.accessKeyID+"/"+scope+
			", SignedHeaders="+signedHeaders+
			", Signature="+signature,
	)
	return nil
}

func canonicalHeaders(request *http.Request) (string, string) {
	values := map[string]string{
		"host":                 request.URL.Host,
		"x-amz-content-sha256": request.Header.Get("x-amz-content-sha256"),
		"x-amz-date":           request.Header.Get("x-amz-date"),
	}
	for _, name := range []string{
		"content-type",
		"if-none-match",
		"x-amz-meta-sha256",
		"x-amz-security-token",
	} {
		if value := request.Header.Get(name); value != "" {
			values[name] = canonicalHeaderValue(value)
		}
	}
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	var canonical strings.Builder
	for _, name := range names {
		canonical.WriteString(name)
		canonical.WriteByte(':')
		canonical.WriteString(canonicalHeaderValue(values[name]))
		canonical.WriteByte('\n')
	}
	return canonical.String(), strings.Join(names, ";")
}

func canonicalHeaderValue(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func hmacSHA256(key []byte, value string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}
