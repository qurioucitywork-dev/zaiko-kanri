ALTER TABLE products DROP CONSTRAINT IF EXISTS products_inventory_status_check;

UPDATE products
SET inventory_status = 'cost_adjusted', updated_at = CURRENT_TIMESTAMP
WHERE inventory_status = 'broken_down';

-- The product/purchase consistency trigger is deferred. Flush the status-only
-- update events before altering the same table again in this transaction.
SET CONSTRAINTS ALL IMMEDIATE;

ALTER TABLE products ADD CONSTRAINT products_inventory_status_check
    CHECK (inventory_status IN (
        'purchasing', 'in_stock', 'cost_adjustment', 'cost_adjusted', 'reserved', 'return_pending',
        'consigned', 'sold', 'shipped', 'cancelled', 'invalid'
    ));
