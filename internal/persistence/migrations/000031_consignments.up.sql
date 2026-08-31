ALTER TABLE products DROP CONSTRAINT IF EXISTS products_inventory_status_check;
ALTER TABLE products ADD CONSTRAINT products_inventory_status_check
    CHECK (inventory_status IN (
        'purchasing', 'in_stock', 'reserved', 'return_pending',
        'consigned', 'sold', 'shipped', 'cancelled', 'invalid'
    ));

CREATE TABLE IF NOT EXISTS consignment_slips (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    slip_number TEXT NOT NULL,
    consignee_role_id TEXT NOT NULL REFERENCES partner_roles(id),
    consignment_date DATE NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('confirmed', 'cancelled')),
    notes TEXT NOT NULL DEFAULT '',
    confirmed_at TIMESTAMPTZ NOT NULL,
    confirmed_by TEXT NOT NULL REFERENCES users(id),
    created_by TEXT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (organization_id, slip_number)
);

CREATE INDEX IF NOT EXISTS idx_consignment_slips_org_date
    ON consignment_slips (organization_id, consignment_date DESC, status);

CREATE TABLE IF NOT EXISTS consignment_lines (
    id TEXT PRIMARY KEY,
    consignment_slip_id TEXT NOT NULL REFERENCES consignment_slips(id),
    line_number INTEGER NOT NULL CHECK (line_number > 0),
    product_id TEXT NOT NULL REFERENCES products(id),
    quantity INTEGER NOT NULL DEFAULT 1 CHECK (quantity = 1),
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (consignment_slip_id, line_number),
    UNIQUE (consignment_slip_id, product_id)
);

CREATE INDEX IF NOT EXISTS idx_consignment_lines_product
    ON consignment_lines (product_id, consignment_slip_id);
