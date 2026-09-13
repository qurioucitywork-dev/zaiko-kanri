ALTER TABLE consignment_slips
    ADD COLUMN IF NOT EXISTS display_currency VARCHAR(3) NOT NULL DEFAULT 'JPY',
    ADD COLUMN IF NOT EXISTS fx_rate_snapshot_id TEXT REFERENCES exchange_rate_snapshots(id),
    ADD COLUMN IF NOT EXISTS fx_rate_scaled BIGINT NOT NULL DEFAULT 15525000000,
    ADD COLUMN IF NOT EXISTS fx_scale BIGINT NOT NULL DEFAULT 100000000,
    ADD COLUMN IF NOT EXISTS issued_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS issued_by VARCHAR(64);

ALTER TABLE consignment_lines
    ADD COLUMN IF NOT EXISTS sale_price_usd_minor BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS converted_sale_price_jpy BIGINT NOT NULL DEFAULT 0;

UPDATE consignment_slips c
SET fx_rate_snapshot_id = fx.id,
    fx_rate_scaled = fx.rate_scaled,
    fx_scale = fx.scale
FROM exchange_rate_snapshots fx
WHERE fx.id = (
    SELECT candidate.id
    FROM exchange_rate_snapshots candidate
    WHERE candidate.organization_id = c.organization_id
      AND candidate.base_currency = 'USD'
      AND candidate.quote_currency = 'JPY'
      AND candidate.observed_at <= c.created_at
    ORDER BY candidate.observed_at DESC
    LIMIT 1
);

UPDATE consignment_lines l
SET sale_price_usd_minor = CASE
        WHEN p.base_sale_currency = 'USD' THEN p.base_sale_price_minor
        WHEN p.base_sale_currency = 'JPY' THEN ROUND(p.base_sale_price_minor * c.fx_scale::numeric / c.fx_rate_scaled)::bigint
        ELSE 0
    END
FROM products p, consignment_slips c
WHERE p.id = l.product_id AND c.id = l.consignment_slip_id;

UPDATE consignment_lines l
SET converted_sale_price_jpy = CEIL(l.sale_price_usd_minor * c.fx_rate_scaled::numeric / c.fx_scale / 1000) * 1000
FROM consignment_slips c
WHERE c.id = l.consignment_slip_id;

ALTER TABLE consignment_slips
    DROP CONSTRAINT IF EXISTS consignment_slips_display_currency_check;
ALTER TABLE consignment_slips
    ADD CONSTRAINT consignment_slips_display_currency_check CHECK (display_currency = 'JPY');

ALTER TABLE consignment_slips
    DROP CONSTRAINT IF EXISTS consignment_slips_fx_rate_positive_check;
ALTER TABLE consignment_slips
    ADD CONSTRAINT consignment_slips_fx_rate_positive_check CHECK (fx_rate_scaled > 0 AND fx_scale > 0);

CREATE INDEX IF NOT EXISTS idx_consignment_slips_org_issued
    ON consignment_slips (organization_id, issued_at DESC);
