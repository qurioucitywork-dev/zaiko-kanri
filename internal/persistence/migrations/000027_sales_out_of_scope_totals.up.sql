UPDATE sales_lines AS line
SET tax_amount_minor = 0,
    total_minor = line.subtotal_minor,
    converted_total_jpy = ROUND(
        line.subtotal_minor::numeric * line.fx_rate_scaled::numeric / NULLIF(line.fx_scale, 0)
    )::bigint
FROM sales_slips AS slip
WHERE slip.id = line.sales_slip_id
  AND slip.display_currency = 'USD'
  AND (
      line.tax_amount_minor <> 0
      OR line.total_minor <> line.subtotal_minor
  );
