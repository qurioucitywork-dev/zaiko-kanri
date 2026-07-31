CREATE TABLE IF NOT EXISTS guest_box_published_products (
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    company_id TEXT NOT NULL REFERENCES guest_companies(id),
    box_id TEXT NOT NULL REFERENCES guest_boxes(id),
    product_id TEXT NOT NULL REFERENCES products(id),
    sort_order INTEGER NOT NULL DEFAULT 0,
    published_by TEXT REFERENCES users(id),
    published_at TEXT NOT NULL,
    PRIMARY KEY (organization_id, company_id, box_id, product_id)
);

CREATE INDEX IF NOT EXISTS idx_guest_box_published_products_lookup
    ON guest_box_published_products (organization_id, company_id, box_id, sort_order, product_id);

CREATE TABLE IF NOT EXISTS guest_credentials (
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    company_id TEXT NOT NULL REFERENCES guest_companies(id),
    guest_id TEXT NOT NULL,
    email TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    updated_by TEXT REFERENCES users(id),
    updated_at TEXT NOT NULL,
    PRIMARY KEY (organization_id, company_id),
    UNIQUE (organization_id, guest_id)
);
