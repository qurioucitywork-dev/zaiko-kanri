DROP INDEX IF EXISTS idx_consignment_slips_org_issued;
ALTER TABLE consignment_lines
    DROP COLUMN IF EXISTS converted_sale_price_jpy,
    DROP COLUMN IF EXISTS sale_price_usd_minor;
ALTER TABLE consignment_slips
    DROP COLUMN IF EXISTS issued_by,
    DROP COLUMN IF EXISTS issued_at,
    DROP COLUMN IF EXISTS fx_scale,
    DROP COLUMN IF EXISTS fx_rate_scaled,
    DROP COLUMN IF EXISTS fx_rate_snapshot_id,
    DROP COLUMN IF EXISTS display_currency;

