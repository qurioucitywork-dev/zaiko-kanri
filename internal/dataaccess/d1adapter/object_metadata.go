package d1adapter

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/dataaccess"
)

var (
	safeObjectID   = regexp.MustCompile(`\A[A-Za-z0-9_-]{1,128}\z`)
	safeReasonCode = regexp.MustCompile(`\A[a-z][a-z0-9_]{0,63}\z`)

	_ dataaccess.ObjectMetadataWriter = (*Adapter)(nil)
)

const (
	operationCreatePending = "object.create_pending"
	operationMarkReady     = "object.mark_ready"
	operationMarkFailed    = "object.mark_failed"
	operationMarkDeleted   = "object.mark_deleted"
)

type commandEnvelope struct {
	ActorID     string `json:"actor_id"`
	RequestedAt string `json:"requested_at"`
}

type createPendingRequest struct {
	commandEnvelope
	ObjectID     string `json:"object_id"`
	ProductID    string `json:"product_id"`
	OriginalName string `json:"original_name"`
	ContentType  string `json:"content_type"`
	SortOrder    int    `json:"sort_order"`
}

type markReadyRequest struct {
	commandEnvelope
	ChecksumSHA256 string `json:"checksum_sha256"`
	SizeBytes      int64  `json:"size_bytes"`
	ReadyAt        string `json:"ready_at"`
}

type markFailedRequest struct {
	commandEnvelope
	ReasonCode string `json:"reason_code"`
}

type markDeletedRequest struct {
	commandEnvelope
	DeletedAt string `json:"deleted_at"`
}

func (a *Adapter) CreatePendingObject(
	ctx context.Context,
	scope dataaccess.CommandScope,
	input dataaccess.PendingObjectInput,
) (dataaccess.ObjectMetadata, error) {
	if err := scope.Validate(); err != nil {
		return dataaccess.ObjectMetadata{}, err
	}
	contentType := strings.ToLower(strings.TrimSpace(input.ContentType))
	if !safeObjectID.MatchString(input.ObjectID) ||
		!boundedRequired(input.ProductID, 128) ||
		!boundedRequired(input.OriginalName, 512) ||
		input.SortOrder < 0 ||
		!allowedObjectContentType(contentType) {
		return dataaccess.ObjectMetadata{}, dataaccess.ErrInvalidArgument
	}
	payload := createPendingRequest{
		commandEnvelope: envelope(scope),
		ObjectID:        input.ObjectID,
		ProductID:       strings.TrimSpace(input.ProductID),
		OriginalName:    input.OriginalName,
		ContentType:     contentType,
		SortOrder:       input.SortOrder,
	}
	var response struct {
		Object objectDTO `json:"object"`
	}
	if err := a.postObjectCommand(
		ctx,
		scope,
		operationCreatePending,
		"/internal/v1/object-metadata/pending",
		payload,
		&response,
	); err != nil {
		return dataaccess.ObjectMetadata{}, err
	}
	return response.Object.object()
}

func (a *Adapter) MarkObjectReady(
	ctx context.Context,
	scope dataaccess.CommandScope,
	objectID string,
	receipt dataaccess.BlobReceipt,
	readyAt time.Time,
) (dataaccess.ObjectMetadata, error) {
	if err := scope.Validate(); err != nil {
		return dataaccess.ObjectMetadata{}, err
	}
	checksum := strings.ToLower(strings.TrimSpace(receipt.ChecksumSHA256))
	if !safeObjectID.MatchString(objectID) ||
		!validObjectChecksum(checksum) ||
		receipt.SizeBytes < 1 ||
		readyAt.IsZero() {
		return dataaccess.ObjectMetadata{}, dataaccess.ErrInvalidArgument
	}
	payload := markReadyRequest{
		commandEnvelope: envelope(scope),
		ChecksumSHA256:  checksum,
		SizeBytes:       receipt.SizeBytes,
		ReadyAt:         readyAt.UTC().Format(time.RFC3339Nano),
	}
	var response struct {
		Object objectDTO `json:"object"`
	}
	path := "/internal/v1/object-metadata/" + url.PathEscape(objectID) + "/ready"
	if err := a.postObjectCommand(
		ctx,
		scope,
		operationMarkReady,
		path,
		payload,
		&response,
	); err != nil {
		return dataaccess.ObjectMetadata{}, err
	}
	return response.Object.object()
}

func (a *Adapter) MarkObjectFailed(
	ctx context.Context,
	scope dataaccess.CommandScope,
	objectID string,
	reasonCode string,
) error {
	if err := scope.Validate(); err != nil {
		return err
	}
	reasonCode = strings.TrimSpace(reasonCode)
	if !safeObjectID.MatchString(objectID) || !safeReasonCode.MatchString(reasonCode) {
		return dataaccess.ErrInvalidArgument
	}
	payload := markFailedRequest{
		commandEnvelope: envelope(scope),
		ReasonCode:      reasonCode,
	}
	path := "/internal/v1/object-metadata/" + url.PathEscape(objectID) + "/failed"
	return a.postObjectCommand(
		ctx,
		scope,
		operationMarkFailed,
		path,
		payload,
		nil,
	)
}

func (a *Adapter) MarkObjectDeleted(
	ctx context.Context,
	scope dataaccess.CommandScope,
	objectID string,
	deletedAt time.Time,
) error {
	if err := scope.Validate(); err != nil {
		return err
	}
	if !safeObjectID.MatchString(objectID) || deletedAt.IsZero() {
		return dataaccess.ErrInvalidArgument
	}
	payload := markDeletedRequest{
		commandEnvelope: envelope(scope),
		DeletedAt:       deletedAt.UTC().Format(time.RFC3339Nano),
	}
	path := "/internal/v1/object-metadata/" + url.PathEscape(objectID) + "/deleted"
	return a.postObjectCommand(
		ctx,
		scope,
		operationMarkDeleted,
		path,
		payload,
		nil,
	)
}

func envelope(scope dataaccess.CommandScope) commandEnvelope {
	return commandEnvelope{
		ActorID:     strings.TrimSpace(scope.ActorID),
		RequestedAt: scope.RequestedAt.UTC().Format(time.RFC3339Nano),
	}
}

func (a *Adapter) postObjectCommand(
	ctx context.Context,
	scope dataaccess.CommandScope,
	operation string,
	path string,
	payload any,
	destination any,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if a == nil || a.baseURL == nil || a.client == nil {
		return dataaccess.ErrInvalidArgument
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return errors.New("d1adapter: encode object command")
	}
	canonicalHash, err := canonicalObjectCommandHash(operation, scope, payload)
	if err != nil {
		return errors.New("d1adapter: encode canonical object command")
	}
	endpoint := *a.baseURL
	endpoint.Path += path
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint.String(),
		bytes.NewReader(body),
	)
	if err != nil {
		return errors.New("d1adapter: create object command")
	}
	request.Header.Set("accept", "application/json")
	request.Header.Set("content-type", "application/json")
	request.Header.Set("x-zaiko-tenant-id", strings.TrimSpace(scope.TenantID))
	request.Header.Set("x-zaiko-idempotency-key", strings.TrimSpace(scope.IdempotencyKey))
	request.Header.Set("x-zaiko-canonical-hash", canonicalHash)

	response, err := a.client.Do(request)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		// A transport error can contain the private Worker URL. It must not
		// cross the provider-neutral port.
		return errors.New("d1adapter: object metadata request failed")
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return decodeObjectCommandError(response)
	}
	if destination == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
		return nil
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	if err := decoder.Decode(destination); err != nil {
		return errors.New("d1adapter: invalid object metadata response")
	}
	return nil
}

func canonicalObjectCommandHash(
	operation string,
	scope dataaccess.CommandScope,
	payload any,
) (string, error) {
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	var businessPayload map[string]any
	if err := json.Unmarshal(encodedPayload, &businessPayload); err != nil {
		return "", err
	}
	delete(businessPayload, "actor_id")
	delete(businessPayload, "requested_at")

	canonical := struct {
		Operation string         `json:"operation"`
		TenantID  string         `json:"tenant_id"`
		ActorID   string         `json:"actor_id"`
		Payload   map[string]any `json:"payload"`
	}{
		Operation: operation,
		TenantID:  strings.TrimSpace(scope.TenantID),
		ActorID:   strings.TrimSpace(scope.ActorID),
		Payload:   businessPayload,
	}
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	// JSON.stringify does not HTML-escape <, >, or &. Disable Go's optional
	// HTML escaping so the client and Worker hash the same canonical bytes.
	encoder.SetEscapeHTML(false)
	err = encoder.Encode(canonical)
	if err != nil {
		return "", err
	}
	canonicalBytes := bytes.TrimSuffix(encoded.Bytes(), []byte{'\n'})
	sum := sha256.Sum256(canonicalBytes)
	return hex.EncodeToString(sum[:]), nil
}

func decodeObjectCommandError(response *http.Response) error {
	var payload struct {
		Error string `json:"error"`
	}
	_ = json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&payload)
	switch payload.Error {
	case "invalid_argument":
		return dataaccess.ErrInvalidArgument
	case "not_found":
		return dataaccess.ErrNotFound
	case "conflict":
		return dataaccess.ErrConflict
	case "idempotency_mismatch":
		return dataaccess.ErrIdempotencyMismatch
	}
	switch response.StatusCode {
	case http.StatusBadRequest:
		return dataaccess.ErrInvalidArgument
	case http.StatusNotFound:
		return dataaccess.ErrNotFound
	case http.StatusConflict:
		return dataaccess.ErrConflict
	default:
		return errors.New("d1adapter: object metadata service failed")
	}
}

func boundedRequired(value string, max int) bool {
	length := len(strings.TrimSpace(value))
	return length > 0 && length <= max
}

func allowedObjectContentType(value string) bool {
	switch value {
	case "image/jpeg", "image/png", "image/webp":
		return true
	default:
		return false
	}
}

func validObjectChecksum(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
