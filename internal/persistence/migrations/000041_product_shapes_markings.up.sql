CREATE TABLE IF NOT EXISTS product_shapes (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_by TEXT REFERENCES users(id),
    updated_by TEXT REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (organization_id, code)
);

CREATE INDEX IF NOT EXISTS idx_product_shapes_org_active
    ON product_shapes (organization_id, is_active, sort_order, code);

CREATE TABLE IF NOT EXISTS markings (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    meaning TEXT NOT NULL DEFAULT '',
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_by TEXT REFERENCES users(id),
    updated_by TEXT REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (organization_id, code)
);

CREATE INDEX IF NOT EXISTS idx_markings_org_active
    ON markings (organization_id, is_active, sort_order, code);

ALTER TABLE products ADD COLUMN IF NOT EXISTS shape_id TEXT REFERENCES product_shapes(id);
ALTER TABLE products ADD COLUMN IF NOT EXISTS marking_id TEXT REFERENCES markings(id);
ALTER TABLE purchase_slip_lines ADD COLUMN IF NOT EXISTS shape_id TEXT REFERENCES product_shapes(id);
ALTER TABLE purchase_slip_lines ADD COLUMN IF NOT EXISTS marking_id TEXT REFERENCES markings(id);

CREATE INDEX IF NOT EXISTS idx_products_shape_id ON products(shape_id);
CREATE INDEX IF NOT EXISTS idx_products_marking_id ON products(marking_id);
