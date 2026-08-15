ALTER TABLE shipment_slips
    ADD COLUMN IF NOT EXISTS display_currency TEXT NOT NULL DEFAULT 'USD'
        CHECK (display_currency IN ('JPY', 'USD')),
    ADD COLUMN IF NOT EXISTS fx_rate_snapshot_id TEXT REFERENCES exchange_rate_snapshots(id),
    ADD COLUMN IF NOT EXISTS fx_rate_scaled BIGINT NOT NULL DEFAULT 15500000000 CHECK (fx_rate_scaled > 0),
    ADD COLUMN IF NOT EXISTS fx_scale BIGINT NOT NULL DEFAULT 100000000 CHECK (fx_scale > 0);

ALTER TABLE shipment_lines
    ADD COLUMN IF NOT EXISTS sale_price_usd_minor BIGINT NOT NULL DEFAULT 0 CHECK (sale_price_usd_minor >= 0),
    ADD COLUMN IF NOT EXISTS converted_sale_price_jpy BIGINT NOT NULL DEFAULT 0 CHECK (converted_sale_price_jpy >= 0);

UPDATE shipment_slips AS shipment
SET fx_rate_snapshot_id = COALESCE((
        SELECT rate.id FROM exchange_rate_snapshots AS rate
        WHERE rate.organization_id = shipment.organization_id
          AND rate.base_currency = 'USD' AND rate.quote_currency = 'JPY'
          AND rate.observed_at <= shipment.created_at
        ORDER BY rate.observed_at DESC, rate.created_at DESC LIMIT 1
    ), shipment.fx_rate_snapshot_id),
    fx_rate_scaled = COALESCE((
        SELECT rate.rate_scaled FROM exchange_rate_snapshots AS rate
        WHERE rate.organization_id = shipment.organization_id
          AND rate.base_currency = 'USD' AND rate.quote_currency = 'JPY'
          AND rate.observed_at <= shipment.created_at
        ORDER BY rate.observed_at DESC, rate.created_at DESC LIMIT 1
    ), shipment.fx_rate_scaled),
    fx_scale = COALESCE((
        SELECT rate.scale FROM exchange_rate_snapshots AS rate
        WHERE rate.organization_id = shipment.organization_id
          AND rate.base_currency = 'USD' AND rate.quote_currency = 'JPY'
          AND rate.observed_at <= shipment.created_at
        ORDER BY rate.observed_at DESC, rate.created_at DESC LIMIT 1
    ), shipment.fx_scale);

UPDATE shipment_lines AS line
SET sale_price_usd_minor = CASE
        WHEN product.base_sale_currency = 'USD' THEN product.base_sale_price_minor
        ELSE ROUND(product.base_sale_price_minor * shipment.fx_scale::numeric / shipment.fx_rate_scaled)::bigint
    END
FROM products AS product, shipment_slips AS shipment
WHERE line.product_id = product.id
  AND line.shipment_slip_id = shipment.id
  AND line.sale_price_usd_minor = 0;

UPDATE shipment_lines AS line
SET converted_sale_price_jpy = CEIL(
        (line.sale_price_usd_minor * shipment.fx_rate_scaled::numeric / shipment.fx_scale) / 100
    )::bigint * 100
FROM shipment_slips AS shipment
WHERE line.shipment_slip_id = shipment.id
  AND line.converted_sale_price_jpy = 0;
