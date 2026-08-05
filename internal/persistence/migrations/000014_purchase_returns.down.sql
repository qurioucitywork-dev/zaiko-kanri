DROP INDEX IF EXISTS idx_return_slips_source_purchase;
ALTER TABLE return_slips DROP COLUMN IF EXISTS source_purchase_slip_id;
ALTER TABLE return_slips DROP COLUMN IF EXISTS supplier_role_id;

ALTER TABLE return_slips DROP CONSTRAINT IF EXISTS return_slips_operation_type_check;
ALTER TABLE return_slips ADD CONSTRAINT return_slips_operation_type_check
    CHECK (operation_type IN ('return', 'takeout'));

ALTER TABLE products DROP CONSTRAINT IF EXISTS products_inventory_status_check;
ALTER TABLE products ADD CONSTRAINT products_inventory_status_check
    CHECK (inventory_status IN ('purchasing', 'in_stock', 'reserved', 'sold', 'shipped', 'cancelled', 'invalid'));
