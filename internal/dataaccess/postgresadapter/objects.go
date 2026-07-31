package postgresadapter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/dataaccess"
)

const objectSelect = `
	SELECT
		o.id,
		o.organization_id,
		o.product_id,
		COALESCE(o.checksum_sha256, ''),
		o.original_name,
		o.content_type,
		COALESCE(o.size_bytes, 0),
		o.sort_order,
		o.status,
		o.created_at,
		o.ready_at,
		o.deleted_at
	FROM zaiko.product_objects o`

const objectReturning = `
		RETURNING
			id,
			organization_id,
			product_id,
			COALESCE(checksum_sha256, ''),
			original_name,
			content_type,
			COALESCE(size_bytes, 0),
			sort_order,
			status,
			created_at,
			ready_at,
			deleted_at`

// ListProductObjects returns lifecycle metadata in deterministic display order.
// A missing and a cross-tenant product both return an empty list.
func (a *Adapter) ListProductObjects(
	ctx context.Context,
	tenantID, productID string,
) ([]dataaccess.ObjectMetadata, error) {
	if err := validateReader(a, ctx, tenantID, productID); err != nil {
		return nil, err
	}
	rows, err := a.db.QueryContext(ctx, objectSelect+`
		JOIN zaiko.products p
		  ON p.id = o.product_id
		 AND p.organization_id = o.organization_id
		 AND p.deleted_at IS NULL
		WHERE o.organization_id = $1 AND o.product_id = $2
		ORDER BY o.sort_order ASC, o.id ASC`,
		strings.TrimSpace(tenantID),
		strings.TrimSpace(productID),
	)
	if err != nil {
		return nil, normalizeDBError(ctx, "list product objects", err)
	}
	defer rows.Close()

	objects := make([]dataaccess.ObjectMetadata, 0)
	for rows.Next() {
		object, scanErr := scanObject(rows)
		if scanErr != nil {
			return nil, normalizeDBError(ctx, "scan object metadata", scanErr)
		}
		objects = append(objects, object)
	}
	if err := rows.Err(); err != nil {
		return nil, normalizeDBError(ctx, "iterate object metadata", err)
	}
	return objects, nil
}

// GetObjectMetadata intentionally uses the same not-found result for missing
// and cross-tenant IDs.
func (a *Adapter) GetObjectMetadata(
	ctx context.Context,
	tenantID, objectID string,
) (dataaccess.ObjectMetadata, error) {
	if err := validateReader(a, ctx, tenantID, objectID); err != nil {
		return dataaccess.ObjectMetadata{}, err
	}
	object, err := scanObject(a.db.QueryRowContext(ctx, objectSelect+`
		JOIN zaiko.products p
		  ON p.id = o.product_id
		 AND p.organization_id = o.organization_id
		 AND p.deleted_at IS NULL
		WHERE o.organization_id = $1 AND o.id = $2`,
		strings.TrimSpace(tenantID),
		strings.TrimSpace(objectID),
	))
	if errors.Is(err, sql.ErrNoRows) {
		return dataaccess.ObjectMetadata{}, dataaccess.ErrNotFound
	}
	if err != nil {
		return dataaccess.ObjectMetadata{}, normalizeDBError(ctx, "get object metadata", err)
	}
	return object, nil
}

func scanObject(row scanner) (dataaccess.ObjectMetadata, error) {
	var object dataaccess.ObjectMetadata
	var readyAt, deletedAt sql.NullTime
	var createdAt time.Time
	err := row.Scan(
		&object.ID,
		&object.TenantID,
		&object.ProductID,
		&object.ChecksumSHA256,
		&object.OriginalName,
		&object.ContentType,
		&object.SizeBytes,
		&object.SortOrder,
		&object.Status,
		&createdAt,
		&readyAt,
		&deletedAt,
	)
	if err != nil {
		return dataaccess.ObjectMetadata{}, err
	}
	switch object.Status {
	case dataaccess.ObjectPending, dataaccess.ObjectReady, dataaccess.ObjectFailed, dataaccess.ObjectDeleted:
	default:
		return dataaccess.ObjectMetadata{}, fmt.Errorf("unsupported object status %q", object.Status)
	}
	object.CreatedAt = createdAt.UTC()
	if readyAt.Valid {
		object.ReadyAt = readyAt.Time.UTC()
	}
	if deletedAt.Valid {
		object.DeletedAt = deletedAt.Time.UTC()
	}
	return object, nil
}
