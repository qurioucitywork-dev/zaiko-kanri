ALTER TABLE sales_adjustment_lines ADD COLUMN before_sale_price_minor BIGINT;
ALTER TABLE sales_adjustment_lines ADD COLUMN after_sale_price_minor BIGINT;

-- Reconstruct existing revisions in creation order, then keep the prices fixed.
WITH history AS (
    SELECT l.id, sl.unit_price_minor + COALESCE(SUM(
        CASE WHEN a.status = 'confirmed' THEN
            CASE WHEN a.direction = 'decrease' THEN -l.adjustment_amount_minor ELSE l.adjustment_amount_minor END
        ELSE 0 END
    ) OVER (PARTITION BY l.source_sales_line_id ORDER BY a.created_at, a.slip_number, l.id
            ROWS BETWEEN UNBOUNDED PRECEDING AND 1 PRECEDING), 0) AS before_price,
    CASE WHEN a.direction = 'decrease' THEN -l.adjustment_amount_minor ELSE l.adjustment_amount_minor END AS delta
    FROM sales_adjustment_lines l
    JOIN sales_adjustment_slips a ON a.id = l.sales_adjustment_slip_id
    JOIN sales_lines sl ON sl.id = l.source_sales_line_id
)
UPDATE sales_adjustment_lines l
SET before_sale_price_minor = h.before_price, after_sale_price_minor = h.before_price + h.delta
FROM history h WHERE h.id = l.id;

ALTER TABLE sales_adjustment_lines ALTER COLUMN before_sale_price_minor SET NOT NULL;
ALTER TABLE sales_adjustment_lines ALTER COLUMN after_sale_price_minor SET NOT NULL;
