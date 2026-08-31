CREATE TABLE IF NOT EXISTS product_files (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    product_id TEXT NOT NULL REFERENCES products(id),
    storage_driver TEXT NOT NULL CHECK (storage_driver IN ('local', 's3')),
    object_key TEXT NOT NULL,
    original_name TEXT NOT NULL,
    content_type TEXT NOT NULL,
    size_bytes INTEGER NOT NULL CHECK (size_bytes > 0),
    sha256 TEXT NOT NULL DEFAULT '',
    sort_order INTEGER NOT NULL DEFAULT 0,
    uploaded_by TEXT NOT NULL REFERENCES users(id),
    created_at DATETIME NOT NULL,
    UNIQUE (organization_id, object_key)
);

CREATE INDEX IF NOT EXISTS idx_product_files_product
    ON product_files (organization_id, product_id, sort_order);
