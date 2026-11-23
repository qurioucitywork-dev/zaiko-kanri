ALTER TABLE products DROP CONSTRAINT IF EXISTS products_inventory_status_check;
ALTER TABLE products ADD CONSTRAINT products_inventory_status_check
    CHECK (inventory_status IN ('purchasing', 'in_stock', 'reserved', 'return_pending', 'sold', 'shipped', 'cancelled', 'invalid'));

ALTER TABLE return_slips DROP CONSTRAINT IF EXISTS return_slips_operation_type_check;
ALTER TABLE return_slips ADD CONSTRAINT return_slips_operation_type_check
    CHECK (operation_type IN ('return', 'takeout', 'purchase_return'));

ALTER TABLE return_slips
    ADD COLUMN IF NOT EXISTS supplier_role_id TEXT REFERENCES partner_roles(id),
    ADD COLUMN IF NOT EXISTS source_purchase_slip_id TEXT REFERENCES purchase_slips(id);

CREATE INDEX IF NOT EXISTS idx_return_slips_source_purchase
    ON return_slips (organization_id, source_purchase_slip_id)
    WHERE source_purchase_slip_id IS NOT NULL;
