UPDATE products SET inventory_status = 'in_stock' WHERE inventory_status = 'cost_adjustment';

ALTER TABLE products DROP CONSTRAINT IF EXISTS products_inventory_status_check;
ALTER TABLE products ADD CONSTRAINT products_inventory_status_check
    CHECK (inventory_status IN (
        'purchasing', 'in_stock', 'reserved', 'return_pending',
        'consigned', 'sold', 'shipped', 'cancelled', 'invalid'
    ));
