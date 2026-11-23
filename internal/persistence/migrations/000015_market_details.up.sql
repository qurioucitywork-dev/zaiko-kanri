ALTER TABLE market_price_records
    ADD COLUMN IF NOT EXISTS serial_number TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS sku TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS supplier_role_id TEXT REFERENCES partner_roles(id),
    ADD COLUMN IF NOT EXISTS purchase_staff_profile_id TEXT REFERENCES staff_profiles(id),
    ADD COLUMN IF NOT EXISTS material_id TEXT REFERENCES materials(id),
    ADD COLUMN IF NOT EXISTS movement_id TEXT REFERENCES movements(id),
    ADD COLUMN IF NOT EXISTS purchase_date DATE,
    ADD COLUMN IF NOT EXISTS status_text TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS box_id TEXT REFERENCES publication_boxes(id);

CREATE TABLE IF NOT EXISTS market_price_accessories (
    market_price_record_id TEXT NOT NULL REFERENCES market_price_records(id) ON DELETE CASCADE,
    accessory_id TEXT NOT NULL REFERENCES accessories(id),
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (market_price_record_id, accessory_id)
);

CREATE INDEX IF NOT EXISTS idx_market_price_records_supplier
    ON market_price_records (organization_id, supplier_role_id, import_date DESC);
