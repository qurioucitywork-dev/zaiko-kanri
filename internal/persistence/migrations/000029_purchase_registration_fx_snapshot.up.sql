-- Overseas purchase JPY values are fixed at purchase registration, not at
-- document issuance. Normalize existing overseas lines to the latest USD/JPY
-- master rate observed no later than the purchase slip creation timestamp.
WITH registration_rates AS (
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
          AND snapshot.observed_at <= slip.created_at
        ORDER BY snapshot.observed_at DESC, snapshot.created_at DESC
        LIMIT 1
    ) AS rate ON TRUE
    WHERE slip.purchase_tax_mode = 'overseas'
      AND line.cost_currency = 'USD'
)
UPDATE purchase_slip_lines AS line
SET
    converted_total_jpy = ROUND(
        line.unit_cost_minor::NUMERIC
        * line.quantity::NUMERIC
        * registration_rates.rate_scaled::NUMERIC
        / registration_rates.scale::NUMERIC
    )::BIGINT,
    fx_rate_snapshot_id = registration_rates.rate_id,
    fx_rate_scaled = registration_rates.rate_scaled,
    fx_scale = registration_rates.scale
FROM registration_rates
WHERE line.id = registration_rates.line_id;
