package postgresadapter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/dataaccess"
)

const (
	operationCreateProduct = "product.create"
	productSequenceKind    = "product"
)

var productCodePattern = regexp.MustCompile(`\A[0-9]{11}\z`)

// CreateProduct persists a direct product registration and all supported
// relations in one SERIALIZABLE transaction.
func (a *Adapter) CreateProduct(
	ctx context.Context,
	scope dataaccess.CommandScope,
	draft dataaccess.ProductDraft,
) (dataaccess.ProductMutationResult, error) {
	if err := validateObjectCommand(a, ctx, scope); err != nil {
		return dataaccess.ProductMutationResult{}, err
	}
	scope.TenantID = strings.TrimSpace(scope.TenantID)
	scope.ActorID = strings.TrimSpace(scope.ActorID)
	normalized, purchaseDate, err := normalizeProductDraft(draft)
	if err != nil {
		return dataaccess.ProductMutationResult{}, err
	}

	tx, err := beginTx(ctx, a.db)
	if err != nil {
		return dataaccess.ProductMutationResult{}, err
	}
	defer tx.Rollback()
	if err := ensureActor(ctx, tx, scope.TenantID, scope.ActorID, permissionInventoryWrite); err != nil {
		return dataaccess.ProductMutationResult{}, err
	}
	replay, err := reserveIdempotency(ctx, tx, scope, operationCreateProduct, normalized)
	if err != nil {
		return dataaccess.ProductMutationResult{}, err
	}
	if replay.replayed {
		if err := commitTx(ctx, tx); err != nil {
			return dataaccess.ProductMutationResult{}, err
		}
		return dataaccess.ProductMutationResult{
			ProductID: replay.resultID, ProductCode: replay.resultNumber,
			Version: replay.resultVersion, Replayed: true,
		}, nil
	}

	if normalized.BuyerID != "" {
		if err := ensureUserActive(ctx, tx, scope.TenantID, normalized.BuyerID); err != nil {
			return dataaccess.ProductMutationResult{}, err
		}
	}
	if err := ensureSupplier(ctx, tx, scope.TenantID, normalized.SupplierID); err != nil {
		return dataaccess.ProductMutationResult{}, err
	}
	if err := ensureAccessories(ctx, tx, scope.TenantID, normalized.AccessoryCodes); err != nil {
		return dataaccess.ProductMutationResult{}, err
	}
	if normalized.BoxID != "" {
		if err := ensureBox(ctx, tx, scope.TenantID, normalized.BoxID); err != nil {
			return dataaccess.ProductMutationResult{}, err
		}
	}

	productCode, err := allocateProductCode(
		ctx, tx, scope.TenantID, normalized.ProductCode, purchaseDate, a.now().UTC(),
	)
	if err != nil {
		return dataaccess.ProductMutationResult{}, err
	}
	productID, err := a.nextEntityID()
	if err != nil {
		return dataaccess.ProductMutationResult{}, err
	}
	var version int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO zaiko.products (
			id, organization_id, product_code, sku, brand, model_number,
			serial_number, product_type, supplier_id, purchase_date,
			cost_amount_minor, cost_currency,
			base_sale_price_minor, base_sale_currency,
			inventory_status, publication_status, condition_text,
			accessories, box_text, version, created_at, updated_at
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15, $16, $17, '', '', 1, $18, $18
		)
		ON CONFLICT DO NOTHING
		RETURNING version`,
		productID, scope.TenantID, productCode,
		normalized.SKU, normalized.Brand, normalized.ModelNumber,
		normalized.SerialNumber, normalized.ProductType, normalized.SupplierID,
		normalized.PurchaseDate,
		normalized.Cost.AmountMinor, normalized.Cost.Currency,
		normalized.BaseSalePrice.AmountMinor, normalized.BaseSalePrice.Currency,
		normalized.InventoryStatus, normalized.PublicationStatus, normalized.Condition,
		scope.RequestedAt.UTC(),
	).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		return dataaccess.ProductMutationResult{}, dataaccess.ErrConflict
	}
	if err != nil {
		return dataaccess.ProductMutationResult{}, normalizeDBError(ctx, "insert product", err)
	}

	if err := insertProductAccessories(
		ctx, tx, scope.TenantID, productID, normalized.AccessoryCodes, scope.RequestedAt.UTC(),
	); err != nil {
		return dataaccess.ProductMutationResult{}, err
	}
	if normalized.BoxID != "" {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO zaiko.guest_box_products (
				organization_id, box_id, product_id, sort_order, added_by, added_at
			)
			VALUES ($1, $2, $3, 0, $4, $5)`,
			scope.TenantID, normalized.BoxID, productID,
			scope.ActorID, scope.RequestedAt.UTC(),
		); err != nil {
			return dataaccess.ProductMutationResult{}, normalizeDBError(ctx, "insert product box relation", err)
		}
	}

	eventID, err := a.nextEntityID()
	if err != nil {
		return dataaccess.ProductMutationResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO zaiko.inventory_events (
			id, organization_id, product_id, event_type, from_status, to_status,
			reason, actor_user_id, request_id, idempotency_key, created_at
		)
		VALUES (
			$1, $2, $3, 'product_registered', '', $4,
			'direct_registration', $5, $6, $6, $7
		)`,
		eventID, scope.TenantID, productID, normalized.InventoryStatus,
		scope.ActorID, scope.IdempotencyKey, scope.RequestedAt.UTC(),
	); err != nil {
		return dataaccess.ProductMutationResult{}, normalizeDBError(ctx, "insert initial inventory event", err)
	}
	if err := writeAudit(
		ctx, tx, a, scope, "product", productID, operationCreateProduct,
		"", normalized.InventoryStatus, "",
	); err != nil {
		return dataaccess.ProductMutationResult{}, err
	}
	if err := commitIdempotency(
		ctx, tx, scope, operationCreateProduct,
		productID, productCode, version, a.now().UTC(),
	); err != nil {
		return dataaccess.ProductMutationResult{}, err
	}
	if err := commitTx(ctx, tx); err != nil {
		return dataaccess.ProductMutationResult{}, err
	}
	return dataaccess.ProductMutationResult{
		ProductID: productID, ProductCode: productCode, Version: version,
	}, nil
}

func normalizeProductDraft(draft dataaccess.ProductDraft) (dataaccess.ProductDraft, time.Time, error) {
	draft.ProductCode = strings.TrimSpace(draft.ProductCode)
	draft.SKU = strings.TrimSpace(draft.SKU)
	draft.Brand = strings.TrimSpace(draft.Brand)
	draft.ModelNumber = strings.TrimSpace(draft.ModelNumber)
	draft.SerialNumber = strings.TrimSpace(draft.SerialNumber)
	draft.ProductType = strings.TrimSpace(draft.ProductType)
	draft.SupplierID = strings.TrimSpace(draft.SupplierID)
	draft.BuyerID = strings.TrimSpace(draft.BuyerID)
	draft.PurchaseDate = strings.TrimSpace(draft.PurchaseDate)
	draft.Cost.Currency = strings.TrimSpace(draft.Cost.Currency)
	draft.BaseSalePrice.Currency = strings.TrimSpace(draft.BaseSalePrice.Currency)
	draft.InventoryStatus = strings.TrimSpace(draft.InventoryStatus)
	draft.PublicationStatus = strings.TrimSpace(draft.PublicationStatus)
	draft.Condition = strings.TrimSpace(draft.Condition)
	draft.BoxID = strings.TrimSpace(draft.BoxID)

	if err := draft.Validate(); err != nil {
		return dataaccess.ProductDraft{}, time.Time{}, err
	}
	if draft.ProductType == "" || draft.SupplierID == "" ||
		!validInventoryStatus(draft.InventoryStatus) ||
		!validPublicationStatus(draft.PublicationStatus) {
		return dataaccess.ProductDraft{}, time.Time{}, dataaccess.ErrInvalidArgument
	}
	purchaseDate, err := time.Parse("2006-01-02", draft.PurchaseDate)
	if err != nil {
		return dataaccess.ProductDraft{}, time.Time{}, fmt.Errorf("%w: purchase date", dataaccess.ErrInvalidArgument)
	}
	if draft.ProductCode != "" {
		datePrefix := strings.ReplaceAll(draft.PurchaseDate, "-", "")
		if !productCodePattern.MatchString(draft.ProductCode) ||
			!strings.HasPrefix(draft.ProductCode, datePrefix) ||
			draft.ProductCode[8:] == "000" {
			return dataaccess.ProductDraft{}, time.Time{}, fmt.Errorf("%w: product code", dataaccess.ErrInvalidArgument)
		}
	}
	accessories := make([]string, 0, len(draft.AccessoryCodes))
	seen := make(map[string]struct{}, len(draft.AccessoryCodes))
	for _, rawCode := range draft.AccessoryCodes {
		code := strings.TrimSpace(rawCode)
		if code == "" {
			return dataaccess.ProductDraft{}, time.Time{}, fmt.Errorf("%w: accessory code", dataaccess.ErrInvalidArgument)
		}
		if _, exists := seen[code]; exists {
			return dataaccess.ProductDraft{}, time.Time{}, fmt.Errorf("%w: duplicate accessory code", dataaccess.ErrInvalidArgument)
		}
		seen[code] = struct{}{}
		accessories = append(accessories, code)
	}
	sort.Strings(accessories)
	draft.AccessoryCodes = accessories
	return draft, purchaseDate, nil
}

func validInventoryStatus(value string) bool {
	switch value {
	case "purchasing", "in_stock", "reserved", "sold", "shipped", "cancelled", "invalid":
		return true
	default:
		return false
	}
}

func validPublicationStatus(value string) bool {
	return value == "private" || value == "public"
}

func allocateProductCode(
	ctx context.Context,
	tx *sql.Tx,
	tenantID, requestedCode string,
	purchaseDate, updatedAt time.Time,
) (string, error) {
	dateKey := purchaseDate.Format("2006-01-02")
	datePrefix := purchaseDate.Format("20060102")
	var sequence int64
	if requestedCode == "" {
		err := tx.QueryRowContext(ctx, `
			INSERT INTO zaiko.business_number_sequences (
				organization_id, sequence_kind, date_key, last_sequence, updated_at
			)
			VALUES ($1, $2, $3, 1, $4)
			ON CONFLICT (organization_id, sequence_kind, date_key)
			DO UPDATE
			SET last_sequence = zaiko.business_number_sequences.last_sequence + 1,
			    updated_at = EXCLUDED.updated_at
			WHERE zaiko.business_number_sequences.last_sequence < 999
			RETURNING last_sequence`,
			tenantID, productSequenceKind, dateKey, updatedAt.UTC(),
		).Scan(&sequence)
		if errors.Is(err, sql.ErrNoRows) {
			return "", dataaccess.ErrConflict
		}
		if err != nil {
			return "", normalizeDBError(ctx, "allocate product code", err)
		}
		return fmt.Sprintf("%s%03d", datePrefix, sequence), nil
	}

	parsed, err := strconv.ParseInt(requestedCode[8:], 10, 64)
	if err != nil || parsed < 1 || parsed > 999 {
		return "", fmt.Errorf("%w: product code", dataaccess.ErrInvalidArgument)
	}
	err = tx.QueryRowContext(ctx, `
		INSERT INTO zaiko.business_number_sequences (
			organization_id, sequence_kind, date_key, last_sequence, updated_at
		)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (organization_id, sequence_kind, date_key)
		DO UPDATE
		SET last_sequence = GREATEST(
		        zaiko.business_number_sequences.last_sequence,
		        EXCLUDED.last_sequence
		    ),
		    updated_at = EXCLUDED.updated_at
		RETURNING last_sequence`,
		tenantID, productSequenceKind, dateKey, parsed, updatedAt.UTC(),
	).Scan(&sequence)
	if err != nil {
		return "", normalizeDBError(ctx, "reserve requested product code", err)
	}
	return requestedCode, nil
}

func ensureSupplier(ctx context.Context, tx *sql.Tx, tenantID, supplierID string) error {
	var one int
	err := tx.QueryRowContext(ctx, `
		SELECT 1
		FROM zaiko.suppliers
		WHERE organization_id = $1 AND id = $2 AND is_active = TRUE`,
		tenantID, supplierID,
	).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return dataaccess.ErrNotFound
	}
	if err != nil {
		return normalizeDBError(ctx, "verify supplier ownership", err)
	}
	return nil
}

func ensureAccessories(
	ctx context.Context,
	tx *sql.Tx,
	tenantID string,
	accessoryCodes []string,
) error {
	if len(accessoryCodes) == 0 {
		return nil
	}
	placeholders := make([]string, len(accessoryCodes))
	args := make([]any, 1, len(accessoryCodes)+1)
	args[0] = tenantID
	for index, code := range accessoryCodes {
		placeholders[index] = fmt.Sprintf("$%d", index+2)
		args = append(args, code)
	}
	var count int
	err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM zaiko.accessory_masters
		WHERE organization_id = $1
		  AND is_active = TRUE
		  AND accessory_code IN (`+strings.Join(placeholders, ", ")+`)`,
		args...,
	).Scan(&count)
	if err != nil {
		return normalizeDBError(ctx, "verify accessory ownership", err)
	}
	if count != len(accessoryCodes) {
		return dataaccess.ErrNotFound
	}
	return nil
}

func ensureBox(ctx context.Context, tx *sql.Tx, tenantID, boxID string) error {
	var one int
	err := tx.QueryRowContext(ctx, `
		SELECT 1
		FROM zaiko.guest_boxes
		WHERE organization_id = $1 AND id = $2`,
		tenantID, boxID,
	).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return dataaccess.ErrNotFound
	}
	if err != nil {
		return normalizeDBError(ctx, "verify box ownership", err)
	}
	return nil
}

func insertProductAccessories(
	ctx context.Context,
	tx *sql.Tx,
	tenantID, productID string,
	accessoryCodes []string,
	createdAt time.Time,
) error {
	if len(accessoryCodes) == 0 {
		return nil
	}
	values := make([]string, len(accessoryCodes))
	args := make([]any, 0, 2+len(accessoryCodes))
	args = append(args, tenantID, productID)
	for index, code := range accessoryCodes {
		args = append(args, code)
		values[index] = fmt.Sprintf("($1, $2, $%d, $%d)", index+3, len(accessoryCodes)+3)
	}
	args = append(args, createdAt.UTC())
	_, err := tx.ExecContext(ctx, `
		INSERT INTO zaiko.product_accessories (
			organization_id, product_id, accessory_code, created_at
		)
		VALUES `+strings.Join(values, ", "),
		args...,
	)
	return normalizeDBError(ctx, "insert product accessories", err)
}

func (a *Adapter) nextEntityID() (string, error) {
	id, err := a.newID()
	if err != nil {
		return "", fmt.Errorf("postgresadapter: generate entity ID: %w", err)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return "", fmt.Errorf("postgresadapter: generate entity ID: empty ID")
	}
	return id, nil
}
