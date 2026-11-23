-- Overseas purchase costs must keep the USD/JPY rate applicable on the
-- purchase date. Existing line snapshots are corrected only when historical
-- rate data exists on or before that date; otherwise their current snapshot is
-- preserved so no amount is silently replaced by a future rate.
WITH purchase_date_rates AS (
    SELECT
        line.id AS line_id,
        rate.id AS rate_id,
        rate.rate_scaled,
        rate.scale
    FROM purchase_slip_lines AS line
    JOIN purchase_slips AS slip
      ON slip.id = line.purchase_slip_id
    JOIN LATERAL (
        SELECT snapshot.id, snapshot.rate_scaled, snapshot.scale
        FROM exchange_rate_snapshots AS snapshot
        WHERE snapshot.organization_id = slip.organization_id
          AND snapshot.base_currency = 'USD'
          AND snapshot.quote_currency = 'JPY'
          AND snapshot.observed_at < slip.purchase_date + INTERVAL '1 day'
        ORDER BY snapshot.observed_at DESC, snapshot.created_at DESC
        LIMIT 1
    ) AS rate ON TRUE
    WHERE line.cost_currency = 'USD'
)
UPDATE purchase_slip_lines AS line
SET
    converted_total_jpy = ROUND(
        line.unit_cost_minor::NUMERIC
        * line.quantity::NUMERIC
        * purchase_date_rates.rate_scaled::NUMERIC
        / purchase_date_rates.scale::NUMERIC
    )::BIGINT,
    fx_rate_snapshot_id = purchase_date_rates.rate_id,
    fx_rate_scaled = purchase_date_rates.rate_scaled,
    fx_scale = purchase_date_rates.scale
FROM purchase_date_rates
WHERE line.id = purchase_date_rates.line_id;

