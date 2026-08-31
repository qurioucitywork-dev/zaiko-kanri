ALTER TABLE parts
    ADD COLUMN IF NOT EXISTS detail_master_type TEXT NOT NULL DEFAULT ''
        CHECK (detail_master_type IN ('', 'material', 'belt', 'dial')),
    ADD COLUMN IF NOT EXISTS detail_master_code TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS sale_price_usd_minor BIGINT NOT NULL DEFAULT 0
        CHECK (sale_price_usd_minor >= 0);
