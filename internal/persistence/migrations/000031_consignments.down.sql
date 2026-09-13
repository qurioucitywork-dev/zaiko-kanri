DROP TABLE IF EXISTS consignment_lines;
DROP TABLE IF EXISTS consignment_slips;

ALTER TABLE products DROP CONSTRAINT IF EXISTS products_inventory_status_check;
ALTER TABLE products ADD CONSTRAINT products_inventory_status_check
    CHECK (inventory_status IN (
        'purchasing', 'in_stock', 'reserved', 'return_pending',
        'sold', 'shipped', 'cancelled', 'invalid'
    ));
