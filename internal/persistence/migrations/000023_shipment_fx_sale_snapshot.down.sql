ALTER TABLE shipment_lines
    DROP COLUMN IF EXISTS converted_sale_price_jpy,
    DROP COLUMN IF EXISTS sale_price_usd_minor;

ALTER TABLE shipment_slips
    DROP COLUMN IF EXISTS fx_scale,
    DROP COLUMN IF EXISTS fx_rate_scaled,
    DROP COLUMN IF EXISTS fx_rate_snapshot_id,
    DROP COLUMN IF EXISTS display_currency;
