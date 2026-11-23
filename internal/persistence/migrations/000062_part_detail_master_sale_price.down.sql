ALTER TABLE parts
    DROP COLUMN IF EXISTS sale_price_usd_minor,
    DROP COLUMN IF EXISTS detail_master_code,
    DROP COLUMN IF EXISTS detail_master_type;
