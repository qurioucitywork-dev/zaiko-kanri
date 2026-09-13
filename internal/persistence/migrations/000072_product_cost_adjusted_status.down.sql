ALTER TABLE products DROP CONSTRAINT IF EXISTS products_inventory_status_check;

UPDATE products
SET inventory_status = 'broken_down', updated_at = CURRENT_TIMESTAMP
WHERE inventory_status = 'cost_adjusted';

SET CONSTRAINTS ALL IMMEDIATE;

ALTER TABLE products ADD CONSTRAINT products_inventory_status_check
    CHECK (inventory_status IN (
        'purchasing', 'in_stock', 'cost_adjustment', 'broken_down', 'reserved', 'return_pending',
        'consigned', 'sold', 'shipped', 'cancelled', 'invalid'
    ));
