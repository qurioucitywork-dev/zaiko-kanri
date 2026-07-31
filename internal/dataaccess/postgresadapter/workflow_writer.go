package postgresadapter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/qurioucitywork-dev/zaiko-kanri/internal/dataaccess"
)

const (
	operationConfirmPurchase = "purchase.confirm"
	operationConfirmSale     = "sale.confirm"
	operationConfirmShipment = "shipment.confirm"
	operationRestoreReturn   = "return.restore_inventory"
)

type workflowProduct struct {
	id, code, sku, brand, model, productType, serial string
	material, movement, condition, belt, dial, box   string
	accessories, features, status, baseSaleCurrency  string
	baseSalePrice, version                           int64
}

type masterSnapshot struct {
	name, address, contact, invoiceNumber string
}

func (a *Adapter) ConfirmPurchase(ctx context.Context, command dataaccess.ConfirmPurchaseCommand) (dataaccess.WorkflowMutationResult, error) {
	command, purchaseDate, err := normalizePurchaseCommand(command)
	if err != nil {
		return dataaccess.WorkflowMutationResult{}, err
	}
	if err := validateWorkflowAdapter(a, ctx); err != nil {
		return dataaccess.WorkflowMutationResult{}, err
	}
	tx, err := beginTx(ctx, a.db)
	if err != nil {
		return dataaccess.WorkflowMutationResult{}, err
	}
	defer tx.Rollback()
	if err := ensureActor(ctx, tx, command.Scope.TenantID, command.Scope.ActorID, permissionPurchaseConfirm); err != nil {
		return dataaccess.WorkflowMutationResult{}, err
	}
	replay, err := reserveIdempotency(ctx, tx, command.Scope, operationConfirmPurchase, command)
	if err != nil {
		return dataaccess.WorkflowMutationResult{}, err
	}
	if replay.replayed {
		return finishWorkflowReplay(ctx, tx, replay)
	}
	if err := ensureSupplier(ctx, tx, command.Scope.TenantID, command.SupplierID); err != nil {
		return dataaccess.WorkflowMutationResult{}, err
	}
	// The candidate DDL has no independent purchase staff column. Avoid
	// silently assigning StaffID to created_by/confirmed_by with unclear
	// ownership semantics.
	if command.StaffID != command.Scope.ActorID {
		return dataaccess.WorkflowMutationResult{}, dataaccess.ErrPrecondition
	}

	products := make([]workflowProduct, len(command.Lines))
	for i, line := range command.Lines {
		// A product row represents one physical inventory item. The candidate
		// schema cannot associate one product ID with a multi-unit purchase line.
		if line.Quantity != 1 {
			return dataaccess.WorkflowMutationResult{}, dataaccess.ErrPrecondition
		}
		product, loadErr := loadWorkflowProduct(ctx, tx, command.Scope.TenantID, line.ProductID)
		if loadErr != nil {
			return dataaccess.WorkflowMutationResult{}, loadErr
		}
		if product.status != "purchasing" {
			return dataaccess.WorkflowMutationResult{}, dataaccess.ErrConflict
		}
		products[i] = product
	}

	slipID, err := a.nextEntityID()
	if err != nil {
		return dataaccess.WorkflowMutationResult{}, err
	}
	number, err := allocateWorkflowNumber(ctx, tx, command.Scope.TenantID, "purchase_slip", "PI", purchaseDate, a.now())
	if err != nil {
		return dataaccess.WorkflowMutationResult{}, err
	}
	now := command.Scope.RequestedAt.UTC()
	var version int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO zaiko.purchase_slips (
			id, organization_id, slip_number, supplier_id, purchase_date,
			status, version, confirmed_at, confirmed_by, created_by, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, 'confirmed', 1, $6, $7, $8, $6, $6)
		RETURNING version`,
		slipID, command.Scope.TenantID, number, command.SupplierID, purchaseDate,
		now, command.Scope.ActorID, command.StaffID,
	).Scan(&version)
	if err != nil {
		return dataaccess.WorkflowMutationResult{}, normalizeDBError(ctx, "insert confirmed purchase slip", err)
	}
	for i, line := range command.Lines {
		product := products[i]
		lineID, idErr := a.nextEntityID()
		if idErr != nil {
			return dataaccess.WorkflowMutationResult{}, idErr
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO zaiko.purchase_slip_lines (
				id, organization_id, purchase_slip_id, line_number, quantity,
				unit_cost_minor, currency, base_sale_price_minor, base_sale_currency, brand, model_number,
				product_type, requested_product_code, sku, serial_number, material_text,
				movement_text, condition_text, belt_material_text, dial_text, box_text,
				accessories, features_text, generated_product_count, created_at
			)
			VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,
				$14, $15, $16, $17, $18, $19, $20, $21, $22, $23, 1, $24
			)`,
			lineID, command.Scope.TenantID, slipID, i+1, line.Quantity,
			line.Amount.AmountMinor, line.Amount.Currency, product.baseSalePrice, product.baseSaleCurrency,
			product.brand, product.model, product.productType, product.code, product.sku,
			product.serial, product.material, product.movement, product.condition,
			product.belt, product.dial, product.box, product.accessories, product.features, now,
		)
		if err != nil {
			return dataaccess.WorkflowMutationResult{}, normalizeDBError(ctx, "insert purchase line", err)
		}
		result, updateErr := tx.ExecContext(ctx, `
			UPDATE zaiko.products
			SET purchase_slip_line_id = $3, supplier_id = $4, purchase_date = $5,
			    cost_amount_minor = $6, cost_currency = $7,
			    inventory_status = 'in_stock', version = version + 1, updated_at = $8
			WHERE organization_id = $1 AND id = $2
			  AND deleted_at IS NULL AND inventory_status = 'purchasing' AND version = $9`,
			command.Scope.TenantID, line.ProductID, lineID, command.SupplierID, purchaseDate,
			line.Amount.AmountMinor, line.Amount.Currency, now, product.version,
		)
		if updateErr != nil {
			return dataaccess.WorkflowMutationResult{}, normalizeDBError(ctx, "transition purchased product", updateErr)
		}
		if err := requireOneRow(result); err != nil {
			return dataaccess.WorkflowMutationResult{}, err
		}
		if err := a.insertInventoryEvent(ctx, tx, command.Scope, line.ProductID, "purchase_confirmed", "purchasing", "in_stock", number); err != nil {
			return dataaccess.WorkflowMutationResult{}, err
		}
	}
	return a.finishWorkflow(ctx, tx, command.Scope, operationConfirmPurchase, "purchase_slip", slipID, number, version, "", "confirmed")
}

func (a *Adapter) ConfirmSale(ctx context.Context, command dataaccess.ConfirmSaleCommand) (dataaccess.WorkflowMutationResult, error) {
	command, saleDate, err := normalizeSaleCommand(command)
	if err != nil {
		return dataaccess.WorkflowMutationResult{}, err
	}
	// The candidate schema has no standard-tax policy/rate/rounding source,
	// and the command has no exchange-rate snapshot identity. Refuse to invent
	// either money policy; exempt JPY sales remain fully supported.
	if !command.TaxExempt || command.Currency != "JPY" {
		return dataaccess.WorkflowMutationResult{}, dataaccess.ErrPrecondition
	}
	// There is no existing SalesSlipID in this create-and-confirm command, so
	// the target of the scalar ExpectedVersion is not defined.
	if command.ExpectedVersion != 0 {
		return dataaccess.WorkflowMutationResult{}, dataaccess.ErrPrecondition
	}
	if err := validateWorkflowAdapter(a, ctx); err != nil {
		return dataaccess.WorkflowMutationResult{}, err
	}
	tx, err := beginTx(ctx, a.db)
	if err != nil {
		return dataaccess.WorkflowMutationResult{}, err
	}
	defer tx.Rollback()
	if err := ensureActor(ctx, tx, command.Scope.TenantID, command.Scope.ActorID, permissionSalesConfirm); err != nil {
		return dataaccess.WorkflowMutationResult{}, err
	}
	replay, err := reserveIdempotency(ctx, tx, command.Scope, operationConfirmSale, command)
	if err != nil {
		return dataaccess.WorkflowMutationResult{}, err
	}
	if replay.replayed {
		return finishWorkflowReplay(ctx, tx, replay)
	}
	buyer, err := loadSalesDestination(ctx, tx, command.Scope.TenantID, command.BuyerID)
	if err != nil {
		return dataaccess.WorkflowMutationResult{}, err
	}

	products := make([]workflowProduct, len(command.Lines))
	var subtotal int64
	for i, line := range command.Lines {
		if line.Quantity != 1 || line.Amount.Currency != command.Currency {
			return dataaccess.WorkflowMutationResult{}, dataaccess.ErrPrecondition
		}
		product, loadErr := loadWorkflowProduct(ctx, tx, command.Scope.TenantID, line.ProductID)
		if loadErr != nil {
			return dataaccess.WorkflowMutationResult{}, loadErr
		}
		if product.status != "in_stock" && product.status != "reserved" {
			return dataaccess.WorkflowMutationResult{}, dataaccess.ErrConflict
		}
		total, convertErr := convertExactJPY(line.Amount.AmountMinor, int64(line.Quantity), command.Currency, command.FXRateScaled, int64(command.FXRateScale))
		if convertErr != nil {
			return dataaccess.WorkflowMutationResult{}, convertErr
		}
		subtotal, convertErr = addMoney(subtotal, total)
		if convertErr != nil {
			return dataaccess.WorkflowMutationResult{}, convertErr
		}
		products[i] = product
	}

	slipID, err := a.nextEntityID()
	if err != nil {
		return dataaccess.WorkflowMutationResult{}, err
	}
	number, err := allocateWorkflowNumber(ctx, tx, command.Scope.TenantID, "sales_slip", "SL", saleDate, a.now())
	if err != nil {
		return dataaccess.WorkflowMutationResult{}, err
	}
	rate, scale := command.FXRateScaled, int64(command.FXRateScale)
	if command.Currency == "JPY" {
		rate, scale = 1, 1
	}
	now := command.Scope.RequestedAt.UTC()
	var version int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO zaiko.sales_slips (
			id, organization_id, slip_number, sales_date, customer_record_id,
			customer_name, customer_address, customer_phone, qualified_invoice_number,
			status, tax_mode, settlement_currency, fx_rate_scaled, fx_rate_scale,
			subtotal_minor, tax_minor, total_minor, version, confirmed_at, confirmed_by,
			created_by, created_at, updated_at
		)
		VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, 'confirmed', 'exempt',
			$10, $11, $12, $13, 0, $13, 1, $14, $15, $15, $14, $14
		)
		RETURNING version`,
		slipID, command.Scope.TenantID, number, saleDate, command.BuyerID,
		buyer.name, buyer.address, buyer.contact, buyer.invoiceNumber,
		command.Currency, rate, scale, subtotal, now, command.Scope.ActorID,
	).Scan(&version)
	if err != nil {
		return dataaccess.WorkflowMutationResult{}, normalizeDBError(ctx, "insert confirmed sales slip", err)
	}
	for i, line := range command.Lines {
		product := products[i]
		converted, _ := convertExactJPY(line.Amount.AmountMinor, 1, command.Currency, rate, scale)
		lineID, idErr := a.nextEntityID()
		if idErr != nil {
			return dataaccess.WorkflowMutationResult{}, idErr
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO zaiko.sales_lines (
				id, organization_id, sales_slip_id, line_number, product_id, quantity,
				unit_price_minor, sale_currency, exchange_rate_scaled, exchange_rate_scale,
				exchange_rate_observed_at, converted_unit_price_jpy, converted_total_jpy, created_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $12, $11)`,
			lineID, command.Scope.TenantID, slipID, i+1, line.ProductID, line.Quantity,
			line.Amount.AmountMinor, command.Currency, rate, scale, now, converted,
		)
		if err != nil {
			return dataaccess.WorkflowMutationResult{}, normalizeDBError(ctx, "insert sales line", err)
		}
		result, updateErr := tx.ExecContext(ctx, `
			UPDATE zaiko.products
			SET inventory_status = 'sold', version = version + 1, updated_at = $3
			WHERE organization_id = $1 AND id = $2 AND deleted_at IS NULL
			  AND inventory_status IN ('in_stock', 'reserved') AND version = $4`,
			command.Scope.TenantID, line.ProductID, now, product.version,
		)
		if updateErr != nil {
			return dataaccess.WorkflowMutationResult{}, normalizeDBError(ctx, "transition sold product", updateErr)
		}
		if err := requireOneRow(result); err != nil {
			return dataaccess.WorkflowMutationResult{}, err
		}
		if err := a.insertInventoryEvent(ctx, tx, command.Scope, line.ProductID, "sale_confirmed", product.status, "sold", number); err != nil {
			return dataaccess.WorkflowMutationResult{}, err
		}
	}
	return a.finishWorkflow(ctx, tx, command.Scope, operationConfirmSale, "sales_slip", slipID, number, version, "", "confirmed")
}

func (a *Adapter) ConfirmShipment(ctx context.Context, command dataaccess.ConfirmShipmentCommand) (dataaccess.WorkflowMutationResult, error) {
	command, shipmentDate, err := normalizeShipmentCommand(command)
	if err != nil {
		return dataaccess.WorkflowMutationResult{}, err
	}
	if err := validateWorkflowAdapter(a, ctx); err != nil {
		return dataaccess.WorkflowMutationResult{}, err
	}
	tx, err := beginTx(ctx, a.db)
	if err != nil {
		return dataaccess.WorkflowMutationResult{}, err
	}
	defer tx.Rollback()
	if err := ensureActor(ctx, tx, command.Scope.TenantID, command.Scope.ActorID, permissionShipmentConfirm); err != nil {
		return dataaccess.WorkflowMutationResult{}, err
	}
	replay, err := reserveIdempotency(ctx, tx, command.Scope, operationConfirmShipment, command)
	if err != nil {
		return dataaccess.WorkflowMutationResult{}, err
	}
	if replay.replayed {
		return finishWorkflowReplay(ctx, tx, replay)
	}
	var saleNumber, saleStatus string
	var saleVersion int64
	err = tx.QueryRowContext(ctx, `
		SELECT slip_number, status, version
		FROM zaiko.sales_slips
		WHERE organization_id = $1 AND id = $2
		FOR UPDATE`,
		command.Scope.TenantID, command.SalesSlipID,
	).Scan(&saleNumber, &saleStatus, &saleVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return dataaccess.WorkflowMutationResult{}, dataaccess.ErrNotFound
	}
	if err != nil {
		return dataaccess.WorkflowMutationResult{}, normalizeDBError(ctx, "lock source sales slip", err)
	}
	if command.ExpectedVersion > 0 && saleVersion != command.ExpectedVersion {
		return dataaccess.WorkflowMutationResult{}, dataaccess.ErrConflict
	}
	if saleStatus != "confirmed" {
		return dataaccess.WorkflowMutationResult{}, dataaccess.ErrPrecondition
	}
	destination, err := loadSalesDestination(ctx, tx, command.Scope.TenantID, command.DestinationID)
	if err != nil {
		return dataaccess.WorkflowMutationResult{}, err
	}

	type shipmentProduct struct {
		workflowProduct
		salesLineID string
		quantity    int
		price       int64
		allocated   int
	}
	items := make([]shipmentProduct, len(command.ProductIDs))
	for i, productID := range command.ProductIDs {
		var item shipmentProduct
		err = tx.QueryRowContext(ctx, `
			SELECT sl.id, sl.quantity, sl.unit_price_minor,
			       p.id, p.product_code, p.sku, p.brand, p.model_number, p.product_type,
			       p.serial_number, p.material_text, p.movement_text, p.condition_text,
			       p.belt_material_text, p.dial_text, p.box_text, p.accessories,
			       p.features_text, p.inventory_status, p.base_sale_price_minor,
			       p.base_sale_currency, p.version
			FROM zaiko.sales_lines sl
			JOIN zaiko.products p
			  ON p.organization_id = sl.organization_id AND p.id = sl.product_id
			WHERE sl.organization_id = $1 AND sl.sales_slip_id = $2
			  AND sl.product_id = $3 AND p.deleted_at IS NULL
			FOR UPDATE OF sl, p`,
			command.Scope.TenantID, command.SalesSlipID, productID,
		).Scan(
			&item.salesLineID, &item.quantity, &item.price,
			&item.id, &item.code, &item.sku, &item.brand, &item.model, &item.productType,
			&item.serial, &item.material, &item.movement, &item.condition, &item.belt,
			&item.dial, &item.box, &item.accessories, &item.features, &item.status,
			&item.baseSalePrice, &item.baseSaleCurrency, &item.version,
		)
		if errors.Is(err, sql.ErrNoRows) {
			exists, existsErr := productExists(ctx, tx, command.Scope.TenantID, productID)
			if existsErr != nil {
				return dataaccess.WorkflowMutationResult{}, existsErr
			}
			if !exists {
				return dataaccess.WorkflowMutationResult{}, dataaccess.ErrNotFound
			}
			return dataaccess.WorkflowMutationResult{}, dataaccess.ErrPrecondition
		}
		if err != nil {
			return dataaccess.WorkflowMutationResult{}, normalizeDBError(ctx, "lock shipment product allocation", err)
		}
		if item.quantity != 1 {
			return dataaccess.WorkflowMutationResult{}, dataaccess.ErrPrecondition
		}
		if item.status != "sold" {
			return dataaccess.WorkflowMutationResult{}, dataaccess.ErrConflict
		}
		err = tx.QueryRowContext(ctx, `
			SELECT COALESCE(SUM(allocated_quantity), 0)
			FROM zaiko.sales_shipment_allocations
			WHERE organization_id = $1 AND sales_line_id = $2`,
			command.Scope.TenantID, item.salesLineID,
		).Scan(&item.allocated)
		if err != nil {
			return dataaccess.WorkflowMutationResult{}, normalizeDBError(ctx, "read shipment allocation", err)
		}
		if item.allocated != 0 {
			return dataaccess.WorkflowMutationResult{}, dataaccess.ErrConflict
		}
		items[i] = item
	}

	shipmentID, err := a.nextEntityID()
	if err != nil {
		return dataaccess.WorkflowMutationResult{}, err
	}
	number, err := allocateWorkflowNumber(ctx, tx, command.Scope.TenantID, "shipment_slip", "SH", shipmentDate, a.now())
	if err != nil {
		return dataaccess.WorkflowMutationResult{}, err
	}
	now := command.Scope.RequestedAt.UTC()
	var version int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO zaiko.shipment_slips (
			id, organization_id, shipment_number, sales_slip_id, shipment_date,
			destination_record_id, recipient_name, recipient_address, recipient_phone,
			status, version, confirmed_at, confirmed_by, created_by, created_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'confirmed', 1, $10, $11, $11, $10, $10)
		RETURNING version`,
		shipmentID, command.Scope.TenantID, number, command.SalesSlipID, shipmentDate,
		command.DestinationID, destination.name, destination.address, destination.contact,
		now, command.Scope.ActorID,
	).Scan(&version)
	if err != nil {
		return dataaccess.WorkflowMutationResult{}, normalizeDBError(ctx, "insert confirmed shipment", err)
	}
	for i, item := range items {
		lineID, idErr := a.nextEntityID()
		if idErr != nil {
			return dataaccess.WorkflowMutationResult{}, idErr
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO zaiko.shipment_lines (
				id, organization_id, shipment_slip_id, line_number, product_id,
				quantity, wholesale_price_minor, created_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			lineID, command.Scope.TenantID, shipmentID, i+1, item.id, item.quantity, item.price, now,
		)
		if err != nil {
			return dataaccess.WorkflowMutationResult{}, normalizeDBError(ctx, "insert shipment line", err)
		}
		allocationID, idErr := a.nextEntityID()
		if idErr != nil {
			return dataaccess.WorkflowMutationResult{}, idErr
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO zaiko.sales_shipment_allocations (
				id, organization_id, sales_line_id, shipment_line_id, allocated_quantity, created_at
			)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			allocationID, command.Scope.TenantID, item.salesLineID, lineID, item.quantity, now,
		)
		if err != nil {
			return dataaccess.WorkflowMutationResult{}, normalizeDBError(ctx, "insert shipment allocation", err)
		}
		result, updateErr := tx.ExecContext(ctx, `
			UPDATE zaiko.products
			SET inventory_status = 'shipped', version = version + 1, updated_at = $3
			WHERE organization_id = $1 AND id = $2 AND deleted_at IS NULL
			  AND inventory_status = 'sold' AND version = $4`,
			command.Scope.TenantID, item.id, now, item.version,
		)
		if updateErr != nil {
			return dataaccess.WorkflowMutationResult{}, normalizeDBError(ctx, "transition shipped product", updateErr)
		}
		if err := requireOneRow(result); err != nil {
			return dataaccess.WorkflowMutationResult{}, err
		}
		if err := a.insertInventoryEvent(ctx, tx, command.Scope, item.id, "shipment_confirmed", "sold", "shipped", number); err != nil {
			return dataaccess.WorkflowMutationResult{}, err
		}
	}
	return a.finishWorkflow(ctx, tx, command.Scope, operationConfirmShipment, "shipment_slip", shipmentID, number, version, "", "confirmed")
}

func (a *Adapter) RestoreReturnedInventory(ctx context.Context, command dataaccess.RestoreReturnedInventoryCommand) (dataaccess.WorkflowMutationResult, error) {
	command, err := normalizeRestoreCommand(command)
	if err != nil {
		return dataaccess.WorkflowMutationResult{}, err
	}
	// The command can carry multiple independently versioned return rows but
	// WorkflowMutationResult exposes one Version. Until a batch-result contract
	// exists, only the unambiguous single-item operation is supported.
	if len(command.Items) != 1 {
		return dataaccess.WorkflowMutationResult{}, dataaccess.ErrPrecondition
	}
	if err := validateWorkflowAdapter(a, ctx); err != nil {
		return dataaccess.WorkflowMutationResult{}, err
	}
	tx, err := beginTx(ctx, a.db)
	if err != nil {
		return dataaccess.WorkflowMutationResult{}, err
	}
	defer tx.Rollback()
	if err := ensureActor(ctx, tx, command.Scope.TenantID, command.Scope.ActorID, permissionInventoryWrite); err != nil {
		return dataaccess.WorkflowMutationResult{}, err
	}
	replay, err := reserveIdempotency(ctx, tx, command.Scope, operationRestoreReturn, command)
	if err != nil {
		return dataaccess.WorkflowMutationResult{}, err
	}
	if replay.replayed {
		return finishWorkflowReplay(ctx, tx, replay)
	}
	var saleNumber string
	var saleVersion int64
	err = tx.QueryRowContext(ctx, `
		SELECT slip_number, version
		FROM zaiko.sales_slips
		WHERE organization_id = $1 AND id = $2
		FOR UPDATE`,
		command.Scope.TenantID, command.SaleID,
	).Scan(&saleNumber, &saleVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return dataaccess.WorkflowMutationResult{}, dataaccess.ErrNotFound
	}
	if err != nil {
		return dataaccess.WorkflowMutationResult{}, normalizeDBError(ctx, "lock return source sale", err)
	}

	now := command.Scope.RequestedAt.UTC()
	var resultVersion int64
	for _, requested := range command.Items {
		var action, status, productStatus string
		var quantity int
		var itemVersion, productVersion int64
		var restoredAt sql.NullTime
		err = tx.QueryRowContext(ctx, `
			SELECT r.action_type, r.status, r.quantity, r.version, r.inventory_restored_at,
			       p.inventory_status, p.version
			FROM zaiko.return_takehome_items r
			JOIN zaiko.sales_lines sl
			  ON sl.organization_id = r.organization_id AND sl.id = r.sales_line_id
			JOIN zaiko.products p
			  ON p.organization_id = sl.organization_id AND p.id = sl.product_id
			WHERE r.organization_id = $1 AND r.id = $2 AND r.sales_slip_id = $3
			  AND p.id = $4 AND p.deleted_at IS NULL
			FOR UPDATE OF r, p`,
			command.Scope.TenantID, requested.ReturnItemID, command.SaleID, requested.ProductID,
		).Scan(&action, &status, &quantity, &itemVersion, &restoredAt, &productStatus, &productVersion)
		if errors.Is(err, sql.ErrNoRows) {
			itemExists, existsErr := returnItemExists(ctx, tx, command.Scope.TenantID, requested.ReturnItemID)
			if existsErr != nil {
				return dataaccess.WorkflowMutationResult{}, existsErr
			}
			productExistsInTenant, existsErr := productExists(ctx, tx, command.Scope.TenantID, requested.ProductID)
			if existsErr != nil {
				return dataaccess.WorkflowMutationResult{}, existsErr
			}
			if !itemExists || !productExistsInTenant {
				return dataaccess.WorkflowMutationResult{}, dataaccess.ErrNotFound
			}
			return dataaccess.WorkflowMutationResult{}, dataaccess.ErrPrecondition
		}
		if err != nil {
			return dataaccess.WorkflowMutationResult{}, normalizeDBError(ctx, "lock returned inventory item", err)
		}
		if action != "return" || quantity != requested.Quantity || requested.Quantity != 1 {
			return dataaccess.WorkflowMutationResult{}, dataaccess.ErrPrecondition
		}
		if restoredAt.Valid || status == "cancelled" ||
			(requested.ExpectedVersion > 0 && itemVersion != requested.ExpectedVersion) {
			return dataaccess.WorkflowMutationResult{}, dataaccess.ErrConflict
		}
		if productStatus != "sold" && productStatus != "shipped" && productStatus != "reserved" {
			return dataaccess.WorkflowMutationResult{}, dataaccess.ErrConflict
		}
		productResult, updateErr := tx.ExecContext(ctx, `
			UPDATE zaiko.products
			SET inventory_status = 'in_stock', condition_text = $3,
			    version = version + 1, updated_at = $4
			WHERE organization_id = $1 AND id = $2 AND deleted_at IS NULL
			  AND inventory_status = $5 AND version = $6`,
			command.Scope.TenantID, requested.ProductID, requested.ConditionCode,
			now, productStatus, productVersion,
		)
		if updateErr != nil {
			return dataaccess.WorkflowMutationResult{}, normalizeDBError(ctx, "restore returned product", updateErr)
		}
		if err := requireOneRow(productResult); err != nil {
			return dataaccess.WorkflowMutationResult{}, err
		}
		err = tx.QueryRowContext(ctx, `
			UPDATE zaiko.return_takehome_items
			SET status = 'completed', inventory_restored_at = $4,
			    inventory_restored_by = $5, processed_at = COALESCE(processed_at, $4),
			    processed_by = COALESCE(processed_by, $5),
			    version = version + 1, updated_at = $4
			WHERE organization_id = $1 AND id = $2 AND sales_slip_id = $3
			  AND inventory_restored_at IS NULL AND status IN ('pending', 'completed')
			  AND version = $6
			RETURNING version`,
			command.Scope.TenantID, requested.ReturnItemID, command.SaleID,
			now, command.Scope.ActorID, itemVersion,
		).Scan(&resultVersion)
		if errors.Is(err, sql.ErrNoRows) {
			return dataaccess.WorkflowMutationResult{}, dataaccess.ErrConflict
		}
		if err != nil {
			return dataaccess.WorkflowMutationResult{}, normalizeDBError(ctx, "complete return inventory restore", err)
		}
		if err := a.insertInventoryEvent(ctx, tx, command.Scope, requested.ProductID, "return_restored", productStatus, "in_stock", saleNumber); err != nil {
			return dataaccess.WorkflowMutationResult{}, err
		}
	}
	return a.finishWorkflow(ctx, tx, command.Scope, operationRestoreReturn, "sales_slip", command.SaleID, saleNumber, resultVersion, "", "inventory_restored")
}

func validateWorkflowAdapter(a *Adapter, ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if a == nil || a.db == nil {
		return dataaccess.ErrInvalidArgument
	}
	return nil
}

func normalizeScope(scope dataaccess.CommandScope) dataaccess.CommandScope {
	scope.TenantID = strings.TrimSpace(scope.TenantID)
	scope.ActorID = strings.TrimSpace(scope.ActorID)
	scope.IdempotencyKey = strings.TrimSpace(scope.IdempotencyKey)
	scope.RequestedAt = scope.RequestedAt.UTC()
	return scope
}

func normalizePurchaseCommand(c dataaccess.ConfirmPurchaseCommand) (dataaccess.ConfirmPurchaseCommand, time.Time, error) {
	c.Scope = normalizeScope(c.Scope)
	c.PurchaseDate = strings.TrimSpace(c.PurchaseDate)
	c.SupplierID = strings.TrimSpace(c.SupplierID)
	c.StaffID = strings.TrimSpace(c.StaffID)
	normalizeLines(c.Lines)
	if err := c.Validate(); err != nil {
		return c, time.Time{}, err
	}
	date, _ := time.Parse("2006-01-02", c.PurchaseDate)
	return c, date, nil
}

func normalizeSaleCommand(c dataaccess.ConfirmSaleCommand) (dataaccess.ConfirmSaleCommand, time.Time, error) {
	c.Scope = normalizeScope(c.Scope)
	c.SaleDate = strings.TrimSpace(c.SaleDate)
	c.BuyerID = strings.TrimSpace(c.BuyerID)
	c.Currency = strings.TrimSpace(c.Currency)
	normalizeLines(c.Lines)
	if err := c.Validate(); err != nil {
		return c, time.Time{}, err
	}
	date, _ := time.Parse("2006-01-02", c.SaleDate)
	return c, date, nil
}

func normalizeShipmentCommand(c dataaccess.ConfirmShipmentCommand) (dataaccess.ConfirmShipmentCommand, time.Time, error) {
	c.Scope = normalizeScope(c.Scope)
	c.SalesSlipID = strings.TrimSpace(c.SalesSlipID)
	c.ShipmentDate = strings.TrimSpace(c.ShipmentDate)
	c.DestinationID = strings.TrimSpace(c.DestinationID)
	for i := range c.ProductIDs {
		c.ProductIDs[i] = strings.TrimSpace(c.ProductIDs[i])
	}
	if err := c.Validate(); err != nil {
		return c, time.Time{}, err
	}
	date, _ := time.Parse("2006-01-02", c.ShipmentDate)
	return c, date, nil
}

func normalizeRestoreCommand(c dataaccess.RestoreReturnedInventoryCommand) (dataaccess.RestoreReturnedInventoryCommand, error) {
	c.Scope = normalizeScope(c.Scope)
	c.SaleID = strings.TrimSpace(c.SaleID)
	for i := range c.Items {
		c.Items[i].ReturnItemID = strings.TrimSpace(c.Items[i].ReturnItemID)
		c.Items[i].ProductID = strings.TrimSpace(c.Items[i].ProductID)
		c.Items[i].ConditionCode = strings.TrimSpace(c.Items[i].ConditionCode)
	}
	return c, c.Validate()
}

func normalizeLines(lines []dataaccess.SlipLineAmount) {
	for i := range lines {
		lines[i].ProductID = strings.TrimSpace(lines[i].ProductID)
		lines[i].Amount.Currency = strings.TrimSpace(lines[i].Amount.Currency)
	}
}

func loadWorkflowProduct(ctx context.Context, tx *sql.Tx, tenantID, productID string) (workflowProduct, error) {
	var p workflowProduct
	err := tx.QueryRowContext(ctx, `
		SELECT id, product_code, sku, brand, model_number, product_type, serial_number,
		       material_text, movement_text, condition_text, belt_material_text,
		       dial_text, box_text, accessories, features_text, inventory_status,
		       base_sale_price_minor, base_sale_currency, version
		FROM zaiko.products
		WHERE organization_id = $1 AND id = $2 AND deleted_at IS NULL
		FOR UPDATE`,
		tenantID, productID,
	).Scan(
		&p.id, &p.code, &p.sku, &p.brand, &p.model, &p.productType, &p.serial,
		&p.material, &p.movement, &p.condition, &p.belt, &p.dial, &p.box,
		&p.accessories, &p.features, &p.status, &p.baseSalePrice, &p.baseSaleCurrency, &p.version,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return p, dataaccess.ErrNotFound
	}
	if err != nil {
		return p, normalizeDBError(ctx, "lock workflow product", err)
	}
	return p, nil
}

func loadSalesDestination(ctx context.Context, tx *sql.Tx, tenantID, id string) (masterSnapshot, error) {
	var m masterSnapshot
	err := tx.QueryRowContext(ctx, `
		SELECT name, address, contact, invoice_registration_number
		FROM zaiko.master_records
		WHERE organization_id = $1 AND id = $2
		  AND category = 'sales-destinations' AND is_active = TRUE`,
		tenantID, id,
	).Scan(&m.name, &m.address, &m.contact, &m.invoiceNumber)
	if errors.Is(err, sql.ErrNoRows) {
		return m, dataaccess.ErrNotFound
	}
	if err != nil {
		return m, normalizeDBError(ctx, "read sales destination", err)
	}
	return m, nil
}

func returnItemExists(ctx context.Context, tx *sql.Tx, tenantID, id string) (bool, error) {
	var exists bool
	err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM zaiko.return_takehome_items
			WHERE organization_id = $1 AND id = $2
		)`,
		tenantID, id,
	).Scan(&exists)
	if err != nil {
		return false, normalizeDBError(ctx, "verify return item ownership", err)
	}
	return exists, nil
}

func allocateWorkflowNumber(ctx context.Context, tx *sql.Tx, tenantID, kind, prefix string, date, updatedAt time.Time) (string, error) {
	dateKey := time.Date(date.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	var sequence int64
	err := tx.QueryRowContext(ctx, `
		INSERT INTO zaiko.business_number_sequences (
			organization_id, sequence_kind, date_key, last_sequence, updated_at
		)
		VALUES ($1, $2, $3, 1, $4)
		ON CONFLICT (organization_id, sequence_kind, date_key)
		DO UPDATE SET
			last_sequence = zaiko.business_number_sequences.last_sequence + 1,
			updated_at = EXCLUDED.updated_at
		WHERE zaiko.business_number_sequences.last_sequence < 9999
		RETURNING last_sequence`,
		tenantID, kind, dateKey, updatedAt.UTC(),
	).Scan(&sequence)
	if errors.Is(err, sql.ErrNoRows) {
		return "", dataaccess.ErrConflict
	}
	if err != nil {
		return "", normalizeDBError(ctx, "allocate workflow number", err)
	}
	return fmt.Sprintf("%s-%04d-%04d", prefix, date.Year(), sequence), nil
}

func (a *Adapter) insertInventoryEvent(ctx context.Context, tx *sql.Tx, scope dataaccess.CommandScope, productID, eventType, from, to, reason string) error {
	id, err := a.nextEntityID()
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO zaiko.inventory_events (
			id, organization_id, product_id, event_type, from_status, to_status,
			reason, actor_user_id, request_id, idempotency_key, created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9, $10)`,
		id, scope.TenantID, productID, eventType, from, to, reason,
		scope.ActorID, scope.IdempotencyKey, scope.RequestedAt.UTC(),
	)
	return normalizeDBError(ctx, "insert inventory event", err)
}

func (a *Adapter) finishWorkflow(ctx context.Context, tx *sql.Tx, scope dataaccess.CommandScope, operation, targetType, id, number string, version int64, before, after string) (dataaccess.WorkflowMutationResult, error) {
	if err := writeAudit(ctx, tx, a, scope, targetType, id, operation, before, after, ""); err != nil {
		return dataaccess.WorkflowMutationResult{}, err
	}
	if err := commitIdempotency(ctx, tx, scope, operation, id, number, version, a.now().UTC()); err != nil {
		return dataaccess.WorkflowMutationResult{}, err
	}
	if err := commitTx(ctx, tx); err != nil {
		return dataaccess.WorkflowMutationResult{}, err
	}
	return dataaccess.WorkflowMutationResult{ID: id, Number: number, Version: version}, nil
}

func finishWorkflowReplay(ctx context.Context, tx *sql.Tx, replay idempotencyResult) (dataaccess.WorkflowMutationResult, error) {
	if err := commitTx(ctx, tx); err != nil {
		return dataaccess.WorkflowMutationResult{}, err
	}
	return dataaccess.WorkflowMutationResult{
		ID: replay.resultID, Number: replay.resultNumber,
		Version: replay.resultVersion, Replayed: true,
	}, nil
}

func requireOneRow(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("postgresadapter: inspect compare-and-swap: %w", err)
	}
	if affected != 1 {
		return dataaccess.ErrConflict
	}
	return nil
}

func convertExactJPY(amount int64, quantity int64, currency string, rate, scale int64) (int64, error) {
	if amount < 0 || quantity < 1 {
		return 0, dataaccess.ErrInvalidArgument
	}
	if currency == "JPY" {
		rate, scale = 1, 1
	}
	if rate < 1 || scale < 1 {
		return 0, dataaccess.ErrInvalidArgument
	}
	value := new(big.Int).Mul(big.NewInt(amount), big.NewInt(quantity))
	value.Mul(value, big.NewInt(rate))
	q, rem := new(big.Int), new(big.Int)
	q.QuoRem(value, big.NewInt(scale), rem)
	if rem.Sign() != 0 {
		return 0, dataaccess.ErrPrecondition
	}
	if !q.IsInt64() {
		return 0, dataaccess.ErrPrecondition
	}
	return q.Int64(), nil
}

func addMoney(left, right int64) (int64, error) {
	value := new(big.Int).Add(big.NewInt(left), big.NewInt(right))
	if !value.IsInt64() || value.Sign() < 0 {
		return 0, dataaccess.ErrPrecondition
	}
	return value.Int64(), nil
}
