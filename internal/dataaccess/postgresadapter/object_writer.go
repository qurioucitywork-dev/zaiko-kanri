package postgresadapter

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/dataaccess"
)

const (
	operationCreatePending = "product_object.create_pending"
	operationMarkReady     = "product_object.mark_ready"
	operationMarkFailed    = "product_object.mark_failed"
	operationMarkDeleted   = "product_object.mark_deleted"
)

var validChecksum = regexp.MustCompile(`\A[0-9a-f]{64}\z`)

type idempotencyResult struct {
	replayed      bool
	resultID      string
	resultNumber  string
	resultVersion int64
}

// CreatePendingObject atomically stores pending metadata, idempotency state,
// and an append-only audit row. Object bytes are uploaded separately.
func (a *Adapter) CreatePendingObject(
	ctx context.Context,
	scope dataaccess.CommandScope,
	input dataaccess.PendingObjectInput,
) (dataaccess.ObjectMetadata, error) {
	if err := validateObjectCommand(a, ctx, scope); err != nil {
		return dataaccess.ObjectMetadata{}, err
	}
	scope.TenantID = strings.TrimSpace(scope.TenantID)
	scope.ActorID = strings.TrimSpace(scope.ActorID)
	input.ObjectID = strings.TrimSpace(input.ObjectID)
	input.ProductID = strings.TrimSpace(input.ProductID)
	input.OriginalName = strings.TrimSpace(input.OriginalName)
	input.ContentType = strings.TrimSpace(input.ContentType)
	if !safeObjectID.MatchString(input.ObjectID) ||
		input.ProductID == "" ||
		input.OriginalName == "" ||
		!allowedContentType(input.ContentType) ||
		input.SortOrder < 0 {
		return dataaccess.ObjectMetadata{}, dataaccess.ErrInvalidArgument
	}

	payload := struct {
		ObjectID, ProductID, OriginalName, ContentType string
		SortOrder                                      int
	}{input.ObjectID, input.ProductID, input.OriginalName, input.ContentType, input.SortOrder}
	tx, err := beginTx(ctx, a.db)
	if err != nil {
		return dataaccess.ObjectMetadata{}, err
	}
	defer tx.Rollback()
	if err := ensureActor(ctx, tx, scope.TenantID, scope.ActorID, permissionInventoryWrite); err != nil {
		return dataaccess.ObjectMetadata{}, err
	}
	replay, err := reserveIdempotency(ctx, tx, scope, operationCreatePending, payload)
	if err != nil {
		return dataaccess.ObjectMetadata{}, err
	}
	if replay.replayed {
		object, scanErr := getObjectTx(ctx, tx, scope.TenantID, replay.resultID)
		if scanErr != nil {
			return dataaccess.ObjectMetadata{}, scanErr
		}
		if err := commitTx(ctx, tx); err != nil {
			return dataaccess.ObjectMetadata{}, err
		}
		return object, nil
	}

	object, err := scanObject(tx.QueryRowContext(ctx, `
		INSERT INTO zaiko.product_objects (
			id, organization_id, product_id, original_name, content_type,
			sort_order, status, storage_provider, storage_bucket, storage_key,
			created_by, created_at
		)
		SELECT
			$3, $1, p.id, $4, $5, $6, 'pending', $7, $8, $9, $10, $11
		FROM zaiko.products p
		WHERE p.organization_id = $1 AND p.id = $2 AND p.deleted_at IS NULL
		ON CONFLICT DO NOTHING`+objectReturning,
		scope.TenantID,
		input.ProductID,
		input.ObjectID,
		input.OriginalName,
		input.ContentType,
		input.SortOrder,
		a.storageProvider,
		a.storageBucket,
		a.objectKey(scope.TenantID, input.ObjectID),
		scope.ActorID,
		scope.RequestedAt.UTC(),
	))
	if errors.Is(err, sql.ErrNoRows) {
		exists, existsErr := productExists(ctx, tx, scope.TenantID, input.ProductID)
		if existsErr != nil {
			return dataaccess.ObjectMetadata{}, existsErr
		}
		if !exists {
			return dataaccess.ObjectMetadata{}, dataaccess.ErrNotFound
		}
		return dataaccess.ObjectMetadata{}, dataaccess.ErrConflict
	}
	if err != nil {
		return dataaccess.ObjectMetadata{}, normalizeDBError(ctx, "create pending object", err)
	}
	if err := writeAudit(
		ctx, tx, a, scope, "product_object", input.ObjectID,
		operationCreatePending, "", string(dataaccess.ObjectPending), "",
	); err != nil {
		return dataaccess.ObjectMetadata{}, err
	}
	if err := commitIdempotency(
		ctx, tx, scope, operationCreatePending,
		input.ObjectID, "", 0, a.now().UTC(),
	); err != nil {
		return dataaccess.ObjectMetadata{}, err
	}
	if err := commitTx(ctx, tx); err != nil {
		return dataaccess.ObjectMetadata{}, err
	}
	return object, nil
}

// MarkObjectReady performs pending -> ready as a compare-and-swap.
func (a *Adapter) MarkObjectReady(
	ctx context.Context,
	scope dataaccess.CommandScope,
	objectID string,
	receipt dataaccess.BlobReceipt,
	readyAt time.Time,
) (dataaccess.ObjectMetadata, error) {
	objectID = strings.TrimSpace(objectID)
	receipt.ChecksumSHA256 = strings.ToLower(strings.TrimSpace(receipt.ChecksumSHA256))
	if err := validateObjectCommand(a, ctx, scope); err != nil {
		return dataaccess.ObjectMetadata{}, err
	}
	scope.TenantID = strings.TrimSpace(scope.TenantID)
	scope.ActorID = strings.TrimSpace(scope.ActorID)
	if !safeObjectID.MatchString(objectID) ||
		!validChecksum.MatchString(receipt.ChecksumSHA256) ||
		receipt.SizeBytes < 1 || receipt.SizeBytes > maxObjectBytes ||
		readyAt.IsZero() {
		return dataaccess.ObjectMetadata{}, dataaccess.ErrInvalidArgument
	}
	payload := struct {
		ObjectID, Checksum string
		SizeBytes          int64
		ReadyAt            string
	}{objectID, receipt.ChecksumSHA256, receipt.SizeBytes, readyAt.UTC().Format(time.RFC3339Nano)}
	return a.transitionObject(ctx, scope, operationMarkReady, objectID, payload, func(tx *sql.Tx) (dataaccess.ObjectMetadata, error) {
		return scanObject(tx.QueryRowContext(ctx, `
			UPDATE zaiko.product_objects
			SET status = 'ready',
			    checksum_sha256 = $3,
			    size_bytes = $4,
			    ready_at = $5,
			    failure_code = ''
			WHERE organization_id = $1 AND id = $2 AND status = 'pending'`+objectReturning,
			scope.TenantID, objectID, receipt.ChecksumSHA256, receipt.SizeBytes, readyAt.UTC(),
		))
	}, string(dataaccess.ObjectPending), string(dataaccess.ObjectReady), "")
}

// MarkObjectFailed performs pending -> failed as a compare-and-swap.
func (a *Adapter) MarkObjectFailed(
	ctx context.Context,
	scope dataaccess.CommandScope,
	objectID, reasonCode string,
) error {
	objectID = strings.TrimSpace(objectID)
	reasonCode = strings.TrimSpace(reasonCode)
	if err := validateObjectCommand(a, ctx, scope); err != nil {
		return err
	}
	scope.TenantID = strings.TrimSpace(scope.TenantID)
	scope.ActorID = strings.TrimSpace(scope.ActorID)
	if !safeObjectID.MatchString(objectID) || reasonCode == "" || len(reasonCode) > 128 {
		return dataaccess.ErrInvalidArgument
	}
	payload := struct{ ObjectID, ReasonCode string }{objectID, reasonCode}
	_, err := a.transitionObject(ctx, scope, operationMarkFailed, objectID, payload, func(tx *sql.Tx) (dataaccess.ObjectMetadata, error) {
		return scanObject(tx.QueryRowContext(ctx, `
			UPDATE zaiko.product_objects
			SET status = 'failed', failure_code = $3
			WHERE organization_id = $1 AND id = $2 AND status = 'pending'`+objectReturning,
			scope.TenantID, objectID, reasonCode,
		))
	}, string(dataaccess.ObjectPending), string(dataaccess.ObjectFailed), reasonCode)
	return err
}

// MarkObjectDeleted performs pending|ready|failed -> deleted as a
// compare-and-swap. A different idempotency key cannot replay the transition.
func (a *Adapter) MarkObjectDeleted(
	ctx context.Context,
	scope dataaccess.CommandScope,
	objectID string,
	deletedAt time.Time,
) error {
	objectID = strings.TrimSpace(objectID)
	if err := validateObjectCommand(a, ctx, scope); err != nil {
		return err
	}
	scope.TenantID = strings.TrimSpace(scope.TenantID)
	scope.ActorID = strings.TrimSpace(scope.ActorID)
	if !safeObjectID.MatchString(objectID) || deletedAt.IsZero() {
		return dataaccess.ErrInvalidArgument
	}
	payload := struct {
		ObjectID, DeletedAt string
	}{objectID, deletedAt.UTC().Format(time.RFC3339Nano)}
	_, err := a.transitionObject(ctx, scope, operationMarkDeleted, objectID, payload, func(tx *sql.Tx) (dataaccess.ObjectMetadata, error) {
		return scanObject(tx.QueryRowContext(ctx, `
			UPDATE zaiko.product_objects
			SET status = 'deleted', deleted_at = $3
			WHERE organization_id = $1
			  AND id = $2
			  AND status IN ('pending', 'ready', 'failed')`+objectReturning,
			scope.TenantID, objectID, deletedAt.UTC(),
		))
	}, "active", string(dataaccess.ObjectDeleted), "")
	return err
}

func (a *Adapter) transitionObject(
	ctx context.Context,
	scope dataaccess.CommandScope,
	operation, objectID string,
	payload any,
	update func(*sql.Tx) (dataaccess.ObjectMetadata, error),
	beforeStatus, afterStatus, reason string,
) (dataaccess.ObjectMetadata, error) {
	tx, err := beginTx(ctx, a.db)
	if err != nil {
		return dataaccess.ObjectMetadata{}, err
	}
	defer tx.Rollback()
	if err := ensureActor(ctx, tx, scope.TenantID, scope.ActorID, permissionInventoryWrite); err != nil {
		return dataaccess.ObjectMetadata{}, err
	}
	replay, err := reserveIdempotency(ctx, tx, scope, operation, payload)
	if err != nil {
		return dataaccess.ObjectMetadata{}, err
	}
	if replay.replayed {
		object, scanErr := getObjectTx(ctx, tx, scope.TenantID, replay.resultID)
		if scanErr != nil {
			return dataaccess.ObjectMetadata{}, scanErr
		}
		if err := commitTx(ctx, tx); err != nil {
			return dataaccess.ObjectMetadata{}, err
		}
		return object, nil
	}

	object, err := update(tx)
	if errors.Is(err, sql.ErrNoRows) {
		exists, existsErr := objectExists(ctx, tx, scope.TenantID, objectID)
		if existsErr != nil {
			return dataaccess.ObjectMetadata{}, existsErr
		}
		if !exists {
			return dataaccess.ObjectMetadata{}, dataaccess.ErrNotFound
		}
		return dataaccess.ObjectMetadata{}, dataaccess.ErrConflict
	}
	if err != nil {
		return dataaccess.ObjectMetadata{}, normalizeDBError(ctx, operation, err)
	}
	if err := writeAudit(
		ctx, tx, a, scope, "product_object", objectID,
		operation, beforeStatus, afterStatus, reason,
	); err != nil {
		return dataaccess.ObjectMetadata{}, err
	}
	if err := commitIdempotency(
		ctx, tx, scope, operation,
		objectID, "", 0, a.now().UTC(),
	); err != nil {
		return dataaccess.ObjectMetadata{}, err
	}
	if err := commitTx(ctx, tx); err != nil {
		return dataaccess.ObjectMetadata{}, err
	}
	return object, nil
}

func validateObjectCommand(a *Adapter, ctx context.Context, scope dataaccess.CommandScope) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if a == nil || a.db == nil || a.newID == nil {
		return dataaccess.ErrInvalidArgument
	}
	if err := scope.Validate(); err != nil {
		return err
	}
	return nil
}

func allowedContentType(value string) bool {
	switch value {
	case "image/jpeg", "image/png", "image/webp":
		return true
	default:
		return false
	}
}

func reserveIdempotency(
	ctx context.Context,
	tx *sql.Tx,
	scope dataaccess.CommandScope,
	operation string,
	payload any,
) (idempotencyResult, error) {
	requestHash, err := canonicalHash(operation, scope, payload)
	if err != nil {
		return idempotencyResult{}, err
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO zaiko.idempotency_records (
			organization_id, operation_name, idempotency_key,
			canonical_request_hash, state, actor_user_id, requested_at
		)
		VALUES ($1, $2, $3, $4, 'processing', $5, $6)
		ON CONFLICT DO NOTHING`,
		scope.TenantID, operation, scope.IdempotencyKey,
		requestHash, scope.ActorID, scope.RequestedAt.UTC(),
	)
	if err != nil {
		return idempotencyResult{}, normalizeDBError(ctx, "reserve idempotency key", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return idempotencyResult{}, normalizeDBError(ctx, "inspect idempotency reservation", err)
	}
	if affected == 1 {
		return idempotencyResult{}, nil
	}

	var storedHash, state, resultID, resultNumber string
	var resultVersion int64
	err = tx.QueryRowContext(ctx, `
		SELECT canonical_request_hash, state, result_id, result_number, result_version
		FROM zaiko.idempotency_records
		WHERE organization_id = $1
		  AND operation_name = $2
		  AND idempotency_key = $3
		FOR UPDATE`,
		scope.TenantID, operation, scope.IdempotencyKey,
	).Scan(&storedHash, &state, &resultID, &resultNumber, &resultVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return idempotencyResult{}, dataaccess.ErrConflict
	}
	if err != nil {
		return idempotencyResult{}, normalizeDBError(ctx, "read idempotency key", err)
	}
	if storedHash != requestHash {
		return idempotencyResult{}, dataaccess.ErrIdempotencyMismatch
	}
	if state != "committed" || strings.TrimSpace(resultID) == "" {
		return idempotencyResult{}, dataaccess.ErrConflict
	}
	return idempotencyResult{
		replayed: true, resultID: resultID,
		resultNumber: resultNumber, resultVersion: resultVersion,
	}, nil
}

func canonicalHash(operation string, scope dataaccess.CommandScope, payload any) (string, error) {
	value := struct {
		Operation string `json:"operation"`
		TenantID  string `json:"tenant_id"`
		ActorID   string `json:"actor_id"`
		Payload   any    `json:"payload"`
	}{
		Operation: operation,
		TenantID:  strings.TrimSpace(scope.TenantID),
		ActorID:   strings.TrimSpace(scope.ActorID),
		Payload:   canonicalBusinessPayload(payload),
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("postgresadapter: canonical request: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func canonicalBusinessPayload(payload any) any {
	// Workflow commands carry CommandScope for execution/audit purposes. The
	// outer canonical envelope already binds tenant and actor, while
	// RequestedAt is a transport-attempt timestamp and must not make an
	// otherwise identical retry look like a different business command.
	switch command := payload.(type) {
	case dataaccess.ConfirmPurchaseCommand:
		return struct {
			PurchaseDate string
			SupplierID   string
			StaffID      string
			Lines        []dataaccess.SlipLineAmount
		}{
			PurchaseDate: command.PurchaseDate,
			SupplierID:   command.SupplierID,
			StaffID:      command.StaffID,
			Lines:        command.Lines,
		}
	case dataaccess.ConfirmSaleCommand:
		return struct {
			SaleDate        string
			BuyerID         string
			TaxExempt       bool
			Currency        string
			FXRateScaled    int64
			FXRateScale     int32
			Lines           []dataaccess.SlipLineAmount
			ExpectedVersion int64
		}{
			SaleDate:        command.SaleDate,
			BuyerID:         command.BuyerID,
			TaxExempt:       command.TaxExempt,
			Currency:        command.Currency,
			FXRateScaled:    command.FXRateScaled,
			FXRateScale:     command.FXRateScale,
			Lines:           command.Lines,
			ExpectedVersion: command.ExpectedVersion,
		}
	case dataaccess.ConfirmShipmentCommand:
		return struct {
			SalesSlipID     string
			ShipmentDate    string
			DestinationID   string
			ProductIDs      []string
			ExpectedVersion int64
		}{
			SalesSlipID:     command.SalesSlipID,
			ShipmentDate:    command.ShipmentDate,
			DestinationID:   command.DestinationID,
			ProductIDs:      command.ProductIDs,
			ExpectedVersion: command.ExpectedVersion,
		}
	case dataaccess.RestoreReturnedInventoryCommand:
		return struct {
			SaleID string
			Items  []dataaccess.RestoreReturnedInventoryItem
		}{
			SaleID: command.SaleID,
			Items:  command.Items,
		}
	default:
		return payload
	}
}

func commitIdempotency(
	ctx context.Context,
	tx *sql.Tx,
	scope dataaccess.CommandScope,
	operation, resultID, resultNumber string,
	resultVersion int64,
	committedAt time.Time,
) error {
	result, err := tx.ExecContext(ctx, `
		UPDATE zaiko.idempotency_records
		SET state = 'committed',
		    result_id = $4,
		    result_number = $5,
		    result_version = $6,
		    committed_at = $7,
		    response_json = jsonb_build_object(
		        'result_id', $4::text,
		        'result_number', $5::text,
		        'result_version', $6::bigint
		    )
		WHERE organization_id = $1
		  AND operation_name = $2
		  AND idempotency_key = $3
		  AND state = 'processing'`,
		scope.TenantID, operation, scope.IdempotencyKey,
		resultID, resultNumber, resultVersion, committedAt.UTC(),
	)
	if err != nil {
		return normalizeDBError(ctx, "commit idempotency key", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return normalizeDBError(ctx, "inspect idempotency commit", err)
	}
	if affected != 1 {
		return dataaccess.ErrConflict
	}
	return nil
}

func writeAudit(
	ctx context.Context,
	tx *sql.Tx,
	a *Adapter,
	scope dataaccess.CommandScope,
	targetType, targetID, action, beforeStatus, afterStatus, reason string,
) error {
	auditID, err := a.newID()
	if err != nil || strings.TrimSpace(auditID) == "" {
		if err == nil {
			err = errors.New("empty audit ID")
		}
		return fmt.Errorf("postgresadapter: generate audit record: %w", err)
	}
	beforeJSON, _ := json.Marshal(map[string]string{"status": beforeStatus})
	afterJSON, _ := json.Marshal(map[string]string{"status": afterStatus})
	_, err = tx.ExecContext(ctx, `
		INSERT INTO zaiko.audit_logs (
			id, organization_id, actor_user_id, target_type, target_id,
			action, before_json, after_json, reason, request_id,
			idempotency_key, result, created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8::jsonb,
		        $9, $10, $11, 'committed', $12)`,
		auditID, scope.TenantID, scope.ActorID, targetType, targetID, action,
		string(beforeJSON), string(afterJSON), reason,
		scope.IdempotencyKey, scope.IdempotencyKey, a.now().UTC(),
	)
	return normalizeDBError(ctx, "append audit log", err)
}

func productExists(ctx context.Context, tx *sql.Tx, tenantID, productID string) (bool, error) {
	var exists bool
	err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM zaiko.products
			WHERE organization_id = $1 AND id = $2 AND deleted_at IS NULL
		)`,
		tenantID, productID,
	).Scan(&exists)
	if err != nil {
		return false, normalizeDBError(ctx, "verify product ownership", err)
	}
	return exists, nil
}

func objectExists(ctx context.Context, tx *sql.Tx, tenantID, objectID string) (bool, error) {
	var exists bool
	err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM zaiko.product_objects
			WHERE organization_id = $1 AND id = $2
		)`,
		tenantID, objectID,
	).Scan(&exists)
	if err != nil {
		return false, normalizeDBError(ctx, "verify object ownership", err)
	}
	return exists, nil
}

func getObjectTx(
	ctx context.Context,
	tx *sql.Tx,
	tenantID, objectID string,
) (dataaccess.ObjectMetadata, error) {
	object, err := scanObject(tx.QueryRowContext(ctx, objectSelect+`
		WHERE o.organization_id = $1 AND o.id = $2`,
		tenantID, objectID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return dataaccess.ObjectMetadata{}, dataaccess.ErrNotFound
	}
	if err != nil {
		return dataaccess.ObjectMetadata{}, normalizeDBError(ctx, "read replay object", err)
	}
	return object, nil
}
