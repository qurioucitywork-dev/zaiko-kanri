UPDATE shipment_lines AS line
SET converted_sale_price_jpy = CEIL(
        (line.sale_price_usd_minor * shipment.fx_rate_scaled::numeric / shipment.fx_scale) / 1000
    )::bigint * 1000
FROM shipment_slips AS shipment
WHERE line.shipment_slip_id = shipment.id
  AND line.sale_price_usd_minor > 0;

COMMENT ON COLUMN shipment_lines.converted_sale_price_jpy IS
    'JPY sale price converted at the shipment registration FX snapshot and rounded up per line to the next JPY 1,000';
