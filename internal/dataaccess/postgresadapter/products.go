package postgresadapter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/dataaccess"
)

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
		COALESCE(ps.created_by, ''),
		COALESCE(u.display_name, ''),
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
			FROM zaiko.product_objects image_count
			WHERE image_count.organization_id = p.organization_id
			  AND image_count.product_id = p.id
			  AND image_count.status = 'ready'
		),
		p.created_at`

const productBaseWhere = `
	FROM zaiko.products p
	JOIN zaiko.suppliers s
	  ON s.id = p.supplier_id
	 AND s.organization_id = p.organization_id
	LEFT JOIN zaiko.purchase_slip_lines psl
	  ON psl.id = p.purchase_slip_line_id
	 AND psl.organization_id = p.organization_id
	LEFT JOIN zaiko.purchase_slips ps
	  ON ps.id = psl.purchase_slip_id
	 AND ps.organization_id = p.organization_id
	LEFT JOIN zaiko.users u
	  ON u.id = ps.created_by
	 AND u.organization_id = p.organization_id
	WHERE p.organization_id = $1
	  AND p.deleted_at IS NULL`

// SearchProducts returns stable offset pages ordered by purchase date
// descending and tenant-unique product code ascending.
func (a *Adapter) SearchProducts(
	ctx context.Context,
	tenantID string,
	search dataaccess.ProductSearch,
) (dataaccess.ProductPage, error) {
	if err := validateReader(a, ctx, tenantID); err != nil {
		return dataaccess.ProductPage{}, err
	}
	if err := validateProductSearch(search); err != nil {
		return dataaccess.ProductPage{}, err
	}
	tenantID = strings.TrimSpace(tenantID)
	where, args := productWhere(tenantID, search)

	var total64 int64
	if err := a.db.QueryRowContext(ctx, `SELECT COUNT(*)`+where, args...).Scan(&total64); err != nil {
		return dataaccess.ProductPage{}, normalizeDBError(ctx, "count products", err)
	}
	if total64 > int64(maxInt()) {
		return dataaccess.ProductPage{}, fmt.Errorf("postgresadapter: product count exceeds platform int")
	}
	total := int(total64)
	totalPages := (total + search.PageSize - 1) / search.PageSize

	limitPosition := len(args) + 1
	offsetPosition := limitPosition + 1
	query := productSelect + where +
		fmt.Sprintf(` ORDER BY p.purchase_date DESC, p.product_code ASC LIMIT $%d OFFSET $%d`,
			limitPosition, offsetPosition)
	queryArgs := append(append([]any(nil), args...), int64(search.PageSize), int64(search.Page-1)*int64(search.PageSize))
	rows, err := a.db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return dataaccess.ProductPage{}, normalizeDBError(ctx, "search products", err)
	}
	defer rows.Close()

	products := make([]dataaccess.Product, 0)
	for rows.Next() {
		product, scanErr := scanProduct(rows)
		if scanErr != nil {
			return dataaccess.ProductPage{}, normalizeDBError(ctx, "scan product", scanErr)
		}
		products = append(products, product)
	}
	if err := rows.Err(); err != nil {
		return dataaccess.ProductPage{}, normalizeDBError(ctx, "iterate products", err)
	}
	return dataaccess.ProductPage{
		Items: products, Total: total, Page: search.Page,
		PageSize: search.PageSize, TotalPages: totalPages,
	}, nil
}

// GetProduct intentionally uses the same not-found result for missing and
// cross-tenant IDs.
func (a *Adapter) GetProduct(
	ctx context.Context,
	tenantID, productID string,
) (dataaccess.Product, error) {
	if err := validateReader(a, ctx, tenantID, productID); err != nil {
		return dataaccess.Product{}, err
	}
	product, err := scanProduct(a.db.QueryRowContext(
		ctx,
		productSelect+productBaseWhere+` AND p.id = $2`,
		strings.TrimSpace(tenantID),
		strings.TrimSpace(productID),
	))
	if errors.Is(err, sql.ErrNoRows) {
		return dataaccess.Product{}, dataaccess.ErrNotFound
	}
	if err != nil {
		return dataaccess.Product{}, normalizeDBError(ctx, "get product", err)
	}
	return product, nil
}

func validateProductSearch(search dataaccess.ProductSearch) error {
	if search.Page < 1 || search.PageSize < 1 ||
		int64(search.Page-1) > math.MaxInt64/int64(search.PageSize) {
		return dataaccess.ErrInvalidArgument
	}
	var from, to time.Time
	var err error
	if value := strings.TrimSpace(search.PurchaseDateFrom); value != "" {
		from, err = time.Parse("2006-01-02", value)
		if err != nil {
			return fmt.Errorf("%w: purchase date from", dataaccess.ErrInvalidArgument)
		}
	}
	if value := strings.TrimSpace(search.PurchaseDateTo); value != "" {
		to, err = time.Parse("2006-01-02", value)
		if err != nil {
			return fmt.Errorf("%w: purchase date to", dataaccess.ErrInvalidArgument)
		}
	}
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		return fmt.Errorf("%w: purchase date range", dataaccess.ErrInvalidArgument)
	}
	return nil
}

func productWhere(tenantID string, search dataaccess.ProductSearch) (string, []any) {
	where := productBaseWhere
	args := []any{tenantID}
	add := func(fragment string, value any) {
		args = append(args, value)
		where += fmt.Sprintf(fragment, len(args))
	}

	if query := strings.TrimSpace(search.Query); query != "" {
		pattern := "%" + escapeLike(query) + "%"
		positions := make([]int, 5)
		for index := range positions {
			args = append(args, pattern)
			positions[index] = len(args)
		}
		where += fmt.Sprintf(` AND (
			p.product_code ILIKE $%d ESCAPE '\'
			OR p.sku ILIKE $%d ESCAPE '\'
			OR p.brand ILIKE $%d ESCAPE '\'
			OR p.model_number ILIKE $%d ESCAPE '\'
			OR p.serial_number ILIKE $%d ESCAPE '\'
		)`, positions[0], positions[1], positions[2], positions[3], positions[4])
	}
	for _, filter := range []struct {
		sql   string
		value string
	}{
		{` AND p.brand = $%d`, search.Brand},
		{` AND p.supplier_id = $%d`, search.SupplierID},
		{` AND p.inventory_status = $%d`, search.InventoryStatus},
		{` AND p.purchase_date >= $%d`, search.PurchaseDateFrom},
		{` AND p.purchase_date <= $%d`, search.PurchaseDateTo},
	} {
		if value := strings.TrimSpace(filter.value); value != "" {
			add(filter.sql, value)
		}
	}
	return where, args
}

func escapeLike(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
}

func maxInt() int {
	return int(^uint(0) >> 1)
}

type dateValue struct {
	value string
}

func (d *dateValue) Scan(src any) error {
	switch value := src.(type) {
	case time.Time:
		// PostgreSQL DATE has no timezone. Preserve its calendar components;
		// converting a driver-provided local midnight to UTC could shift the
		// business date to the previous day.
		d.value = value.Format("2006-01-02")
	case string:
		parsed, err := time.Parse("2006-01-02", value)
		if err != nil {
			return err
		}
		d.value = parsed.Format("2006-01-02")
	case []byte:
		return d.Scan(string(value))
	default:
		return fmt.Errorf("unsupported DATE value %T", src)
	}
	return nil
}

func scanProduct(row scanner) (dataaccess.Product, error) {
	var product dataaccess.Product
	var purchaseDate dateValue
	var createdAt time.Time
	var imageCount64 int64
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
		&purchaseDate,
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
		&imageCount64,
		&createdAt,
	)
	if err != nil {
		return dataaccess.Product{}, err
	}
	if imageCount64 < 0 || imageCount64 > int64(maxInt()) {
		return dataaccess.Product{}, fmt.Errorf("invalid image count %s", strconv.FormatInt(imageCount64, 10))
	}
	product.PurchaseDate = purchaseDate.value
	product.ImageCount = int(imageCount64)
	product.CreatedAt = createdAt.UTC()
	return product, nil
}
