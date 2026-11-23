ALTER TABLE purchase_slips
    ADD COLUMN IF NOT EXISTS issue_fx_rate_snapshot_id TEXT REFERENCES exchange_rate_snapshots(id),
    ADD COLUMN IF NOT EXISTS issue_fx_rate_scaled BIGINT CHECK (issue_fx_rate_scaled > 0),
    ADD COLUMN IF NOT EXISTS issue_fx_scale BIGINT CHECK (issue_fx_scale > 0);

-- 既に発行済みの伝票は、発行日時以前で最新のUSD/JPYレートを発行時レートとして固定する。
WITH issue_rates AS (
    SELECT slip.id AS purchase_slip_id,
           rate.id AS rate_id,
           rate.rate_scaled,
           rate.scale
    FROM purchase_slips AS slip
    JOIN LATERAL (
        SELECT snapshot.id, snapshot.rate_scaled, snapshot.scale
        FROM exchange_rate_snapshots AS snapshot
        WHERE snapshot.organization_id = slip.organization_id
          AND snapshot.base_currency = 'USD'
          AND snapshot.quote_currency = 'JPY'
          AND snapshot.observed_at <= slip.issued_at
        ORDER BY snapshot.observed_at DESC, snapshot.created_at DESC
        LIMIT 1
    ) AS rate ON TRUE
    WHERE slip.issued_at IS NOT NULL
      AND slip.issue_fx_rate_snapshot_id IS NULL
)
UPDATE purchase_slips AS slip
SET issue_fx_rate_snapshot_id = issue_rates.rate_id,
    issue_fx_rate_scaled = issue_rates.rate_scaled,
    issue_fx_scale = issue_rates.scale
FROM issue_rates
WHERE slip.id = issue_rates.purchase_slip_id;

CREATE INDEX IF NOT EXISTS idx_purchase_slips_issue_fx_rate
    ON purchase_slips(organization_id, issue_fx_rate_snapshot_id)
    WHERE issue_fx_rate_snapshot_id IS NOT NULL;
