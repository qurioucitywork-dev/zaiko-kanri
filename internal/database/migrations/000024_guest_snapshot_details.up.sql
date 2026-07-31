ALTER TABLE guest_box_published_products ADD COLUMN product_code TEXT NOT NULL DEFAULT '';
ALTER TABLE guest_box_published_products ADD COLUMN brand TEXT NOT NULL DEFAULT '';
ALTER TABLE guest_box_published_products ADD COLUMN model_name TEXT NOT NULL DEFAULT '';
ALTER TABLE guest_box_published_products ADD COLUMN reference_number TEXT NOT NULL DEFAULT '';
ALTER TABLE guest_box_published_products ADD COLUMN serial_number TEXT NOT NULL DEFAULT '';
ALTER TABLE guest_box_published_products ADD COLUMN sale_price_minor INTEGER NOT NULL DEFAULT 0;
ALTER TABLE guest_box_published_products ADD COLUMN sale_currency TEXT NOT NULL DEFAULT 'JPY';
ALTER TABLE guest_box_published_products ADD COLUMN condition_text TEXT NOT NULL DEFAULT '';
ALTER TABLE guest_box_published_products ADD COLUMN accessories TEXT NOT NULL DEFAULT '';
ALTER TABLE guest_box_published_products ADD COLUMN inventory_status TEXT NOT NULL DEFAULT '';
ALTER TABLE guest_box_published_products ADD COLUMN publication_status TEXT NOT NULL DEFAULT '';
ALTER TABLE guest_box_published_products ADD COLUMN box_name TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS guest_box_published_images (
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    company_id TEXT NOT NULL REFERENCES guest_companies(id),
    box_id TEXT NOT NULL REFERENCES guest_boxes(id),
    product_id TEXT NOT NULL REFERENCES products(id),
    image_id TEXT NOT NULL,
    storage_path TEXT NOT NULL,
    original_name TEXT NOT NULL,
    content_type TEXT NOT NULL,
    size_bytes INTEGER NOT NULL DEFAULT 0,
    sort_order INTEGER NOT NULL DEFAULT 0,
    published_at TEXT NOT NULL,
    PRIMARY KEY (organization_id, company_id, box_id, product_id, image_id)
);

CREATE INDEX IF NOT EXISTS idx_guest_box_published_images_lookup
    ON guest_box_published_images (organization_id, company_id, product_id, sort_order, image_id);
