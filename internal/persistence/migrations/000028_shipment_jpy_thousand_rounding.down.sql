UPDATE shipment_lines AS line
SET converted_sale_price_jpy = CEIL(
        (line.sale_price_usd_minor * shipment.fx_rate_scaled::numeric / shipment.fx_scale) / 100
    )::bigint * 100
FROM shipment_slips AS shipment
WHERE line.shipment_slip_id = shipment.id
  AND line.sale_price_usd_minor > 0;

COMMENT ON COLUMN shipment_lines.converted_sale_price_jpy IS NULL;
