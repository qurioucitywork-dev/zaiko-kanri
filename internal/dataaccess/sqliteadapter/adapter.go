// Package sqliteadapter implements the provider-neutral dataaccess read ports
// against the current SQLite schema.
//
// The adapter deliberately accepts *sql.DB instead of database.Store. This
// keeps the existing Store encapsulation and write behavior unchanged while a
// later composition phase decides how the adapter is wired into the app.
package sqliteadapter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/dataaccess"
)

var (
	_ dataaccess.ProductReader        = (*Adapter)(nil)
	_ dataaccess.ObjectMetadataReader = (*Adapter)(nil)
	_ dataaccess.DiagnosticReader     = (*Adapter)(nil)
)

// Adapter is a read-only view over the current SQLite schema. Every business
// lookup includes organization_id (the canonical tenant ID) in its predicate.
type Adapter struct {
	db  *sql.DB
	now func() time.Time
}

// New creates a read adapter. The caller retains ownership of db and must
// configure its connection as appropriate for the deployment. In particular,
// read-only diagnostic environments should enable SQLite query_only.
func New(db *sql.DB) *Adapter {
	return &Adapter{
		db:  db,
		now: func() time.Time { return time.Now().UTC() },
	}
}

// SearchProducts returns a stable tenant-scoped page ordered by purchase date
// descending and product code ascending.
func (a *Adapter) SearchProducts(ctx context.Context, tenantID string, search dataaccess.ProductSearch) (dataaccess.ProductPage, error) {
	if err := ctx.Err(); err != nil {
		return dataaccess.ProductPage{}, err
	}
	tenantID = strings.TrimSpace(tenantID)
	if a == nil || a.db == nil || tenantID == "" || search.Page < 1 || search.PageSize < 1 {
		return dataaccess.ProductPage{}, dataaccess.ErrInvalidArgument
	}

	where, args := productWhere(tenantID, search)
	var total int
	if err := a.db.QueryRowContext(ctx, `SELECT COUNT(*)`+where, args...).Scan(&total); err != nil {
		return dataaccess.ProductPage{}, fmt.Errorf("sqliteadapter: count products: %w", err)
	}
	totalPages := (total + search.PageSize - 1) / search.PageSize

	query := productSelect + where +
		` ORDER BY p.purchase_date DESC, p.product_code ASC LIMIT ? OFFSET ?`
	queryArgs := append(append([]any(nil), args...), search.PageSize, (search.Page-1)*search.PageSize)
	rows, err := a.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return dataaccess.ProductPage{}, fmt.Errorf("sqliteadapter: search products: %w", err)
	}
	defer rows.Close()

	products := make([]dataaccess.Product, 0)
	for rows.Next() {
		product, err := scanProduct(rows)
		if err != nil {
			return dataaccess.ProductPage{}, fmt.Errorf("sqliteadapter: scan product: %w", err)
		}
		products = append(products, product)
	}
	if err := rows.Err(); err != nil {
		return dataaccess.ProductPage{}, fmt.Errorf("sqliteadapter: iterate products: %w", err)
	}
	return dataaccess.ProductPage{
		Items: products, Total: total, Page: search.Page,
		PageSize: search.PageSize, TotalPages: totalPages,
	}, nil
}

// GetProduct returns ErrNotFound for both missing and cross-tenant IDs.
func (a *Adapter) GetProduct(ctx context.Context, tenantID, productID string) (dataaccess.Product, error) {
	if err := ctx.Err(); err != nil {
		return dataaccess.Product{}, err
	}
	tenantID = strings.TrimSpace(tenantID)
	productID = strings.TrimSpace(productID)
	if a == nil || a.db == nil || tenantID == "" || productID == "" {
		return dataaccess.Product{}, dataaccess.ErrInvalidArgument
	}
	product, err := scanProduct(a.db.QueryRowContext(
		ctx,
		productSelect+productBaseWhere+` AND p.id=?`,
		tenantID,
		productID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return dataaccess.Product{}, dataaccess.ErrNotFound
	}
	if err != nil {
		return dataaccess.Product{}, fmt.Errorf("sqliteadapter: get product: %w", err)
	}
	return product, nil
}

const productSelect = `
	SELECT
		p.id,
		p.organization_id,
		p.product_code,
		p.sku,
		p.brand,
		p.model_number,
		p.serial_number,
		p.product_type,
		p.supplier_id,
		s.name,
		ps.created_by,
		u.display_name,
		p.purchase_date,
		p.cost_amount_minor,
		p.cost_currency,
		p.base_sale_price_minor,
		p.base_sale_currency,
		p.inventory_status,
		p.publication_status,
		p.condition_text,
		p.accessories,
		p.material_text,
		p.box_text,
		p.movement_text,
		p.belt_material_text,
		p.dial_text,
		p.features_text,
		(
			SELECT COUNT(*)
			FROM product_images image_count
			WHERE image_count.organization_id=p.organization_id
			  AND image_count.product_id=p.id
		),
		p.created_at`

const productBaseWhere = `
	FROM products p
	JOIN suppliers s
	  ON s.id=p.supplier_id
	 AND s.organization_id=p.organization_id
	JOIN purchase_slip_lines psl
	  ON psl.id=p.purchase_slip_line_id
	JOIN purchase_slips ps
	  ON ps.id=psl.purchase_slip_id
	 AND ps.organization_id=p.organization_id
	JOIN users u
	  ON u.id=ps.created_by
	 AND u.organization_id=p.organization_id
	WHERE p.organization_id=?
	  AND p.deleted_at IS NULL`

func productWhere(tenantID string, search dataaccess.ProductSearch) (string, []any) {
	where := productBaseWhere
	args := []any{tenantID}

	if query := strings.TrimSpace(search.Query); query != "" {
		pattern := "%" + escapeLike(strings.ToLower(query)) + "%"
		where += ` AND (
			LOWER(p.product_code) LIKE ? ESCAPE '\'
			OR LOWER(p.sku) LIKE ? ESCAPE '\'
			OR LOWER(p.brand) LIKE ? ESCAPE '\'
			OR LOWER(p.model_number) LIKE ? ESCAPE '\'
			OR LOWER(p.serial_number) LIKE ? ESCAPE '\'
		)`
		args = append(args, pattern, pattern, pattern, pattern, pattern)
	}
	for _, filter := range []struct {
		sql   string
		value string
	}{
		{` AND p.brand=?`, search.Brand},
		{` AND p.supplier_id=?`, search.SupplierID},
		{` AND p.inventory_status=?`, search.InventoryStatus},
		{` AND p.purchase_date>=?`, search.PurchaseDateFrom},
		{` AND p.purchase_date<=?`, search.PurchaseDateTo},
	} {
		if value := strings.TrimSpace(filter.value); value != "" {
			where += filter.sql
			args = append(args, value)
		}
	}
	return where, args
}

func escapeLike(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(value)
}

type scanner interface {
	Scan(dest ...any) error
}

func scanProduct(row scanner) (dataaccess.Product, error) {
	var product dataaccess.Product
	var createdAt string
	err := row.Scan(
		&product.ID,
		&product.TenantID,
		&product.Code,
		&product.SKU,
		&product.Brand,
		&product.ModelNumber,
		&product.SerialNumber,
		&product.ProductType,
		&product.SupplierID,
		&product.SupplierName,
		&product.BuyerID,
		&product.BuyerName,
		&product.PurchaseDate,
		&product.Cost.AmountMinor,
		&product.Cost.Currency,
		&product.BaseSalePrice.AmountMinor,
		&product.BaseSalePrice.Currency,
		&product.InventoryStatus,
		&product.PublicationStatus,
		&product.Condition,
		&product.Accessories,
		&product.Material,
		&product.Box,
		&product.Movement,
		&product.BeltMaterial,
		&product.Dial,
		&product.Features,
		&product.ImageCount,
		&createdAt,
	)
	if err != nil {
		return dataaccess.Product{}, err
	}
	product.CreatedAt, err = parseSQLiteTime(createdAt)
	if err != nil {
		return dataaccess.Product{}, fmt.Errorf("created_at: %w", err)
	}
	return product, nil
}

// ListProductObjects exposes current product_images rows as ready legacy
// object metadata. Provider paths stay private to the adapter.
func (a *Adapter) ListProductObjects(ctx context.Context, tenantID, productID string) ([]dataaccess.ObjectMetadata, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	tenantID = strings.TrimSpace(tenantID)
	productID = strings.TrimSpace(productID)
	if a == nil || a.db == nil || tenantID == "" || productID == "" {
		return nil, dataaccess.ErrInvalidArgument
	}
	rows, err := a.db.QueryContext(ctx, objectSelect+`
		JOIN products p
		  ON p.id=i.product_id
		 AND p.organization_id=i.organization_id
		 AND p.deleted_at IS NULL
		WHERE i.organization_id=? AND i.product_id=?
		ORDER BY i.sort_order ASC, i.id ASC`,
		tenantID, productID,
	)
	if err != nil {
		return nil, fmt.Errorf("sqliteadapter: list product objects: %w", err)
	}
	defer rows.Close()
	objects := make([]dataaccess.ObjectMetadata, 0)
	for rows.Next() {
		object, err := scanObject(rows)
		if err != nil {
			return nil, fmt.Errorf("sqliteadapter: scan object metadata: %w", err)
		}
		objects = append(objects, object)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqliteadapter: iterate object metadata: %w", err)
	}
	return objects, nil
}

// GetObjectMetadata returns ErrNotFound for both missing and cross-tenant IDs.
func (a *Adapter) GetObjectMetadata(ctx context.Context, tenantID, objectID string) (dataaccess.ObjectMetadata, error) {
	if err := ctx.Err(); err != nil {
		return dataaccess.ObjectMetadata{}, err
	}
	tenantID = strings.TrimSpace(tenantID)
	objectID = strings.TrimSpace(objectID)
	if a == nil || a.db == nil || tenantID == "" || objectID == "" {
		return dataaccess.ObjectMetadata{}, dataaccess.ErrInvalidArgument
	}
	object, err := scanObject(a.db.QueryRowContext(ctx, objectSelect+`
		JOIN products p
		  ON p.id=i.product_id
		 AND p.organization_id=i.organization_id
		 AND p.deleted_at IS NULL
		WHERE i.organization_id=? AND i.id=?`,
		tenantID, objectID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return dataaccess.ObjectMetadata{}, dataaccess.ErrNotFound
	}
	if err != nil {
		return dataaccess.ObjectMetadata{}, fmt.Errorf("sqliteadapter: get object metadata: %w", err)
	}
	return object, nil
}

const objectSelect = `
	SELECT
		i.id,
		i.organization_id,
		i.product_id,
		i.original_name,
		i.content_type,
		i.size_bytes,
		i.sort_order,
		i.created_at
	FROM product_images i`

func scanObject(row scanner) (dataaccess.ObjectMetadata, error) {
	var object dataaccess.ObjectMetadata
	var createdAt string
	err := row.Scan(
		&object.ID,
		&object.TenantID,
		&object.ProductID,
		&object.OriginalName,
		&object.ContentType,
		&object.SizeBytes,
		&object.SortOrder,
		&createdAt,
	)
	if err != nil {
		return dataaccess.ObjectMetadata{}, err
	}
	object.CreatedAt, err = parseSQLiteTime(createdAt)
	if err != nil {
		return dataaccess.ObjectMetadata{}, fmt.Errorf("created_at: %w", err)
	}
	// The current schema represents only complete local files. It has no
	// checksum or lifecycle columns, so those remain empty rather than
	// exposing storage_path as a provider locator.
	object.Status = dataaccess.ObjectReady
	object.ReadyAt = object.CreatedAt
	return object, nil
}

// Diagnose executes read-only connectivity and SQLite capability probes. It
// reports query_only as degraded when the caller has not enabled it, but never
// changes the connection setting itself.
func (a *Adapter) Diagnose(ctx context.Context) (dataaccess.DiagnosticReport, error) {
	if err := ctx.Err(); err != nil {
		return dataaccess.DiagnosticReport{}, err
	}
	if a == nil || a.db == nil {
		return dataaccess.DiagnosticReport{}, dataaccess.ErrInvalidArgument
	}
	checkedAt := a.now().UTC()
	report := dataaccess.DiagnosticReport{
		Provider:  "sqlite",
		CheckedAt: checkedAt,
	}

	started := time.Now()
	var one int
	err := a.db.QueryRowContext(ctx, `SELECT 1`).Scan(&one)
	connectivity := dataaccess.ComponentDiagnostic{
		Name:      "connectivity",
		Status:    dataaccess.DiagnosticOK,
		Message:   "read probe succeeded",
		Latency:   time.Since(started),
		CheckedAt: checkedAt,
	}
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return dataaccess.DiagnosticReport{}, ctxErr
		}
		connectivity.Status = dataaccess.DiagnosticFailed
		connectivity.Message = "read probe failed"
	}
	report.Components = append(report.Components, connectivity)

	started = time.Now()
	var queryOnly int
	err = a.db.QueryRowContext(ctx, `PRAGMA query_only`).Scan(&queryOnly)
	queryOnlyDiagnostic := dataaccess.ComponentDiagnostic{
		Name:      "query_only",
		Status:    dataaccess.DiagnosticOK,
		Message:   "query-only mode enabled",
		Latency:   time.Since(started),
		CheckedAt: checkedAt,
	}
	switch {
	case err != nil:
		if ctxErr := ctx.Err(); ctxErr != nil {
			return dataaccess.DiagnosticReport{}, ctxErr
		}
		queryOnlyDiagnostic.Status = dataaccess.DiagnosticFailed
		queryOnlyDiagnostic.Message = "query-only mode could not be inspected"
	case queryOnly != 1:
		queryOnlyDiagnostic.Status = dataaccess.DiagnosticDegraded
		queryOnlyDiagnostic.Message = "query-only mode is not enabled"
	}
	report.Components = append(report.Components, queryOnlyDiagnostic)

	return report, nil
}

func parseSQLiteTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, err
	}
	return parsed, nil
}
