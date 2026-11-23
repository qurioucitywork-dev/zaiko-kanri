DROP INDEX IF EXISTS idx_purchase_slips_issue_fx_rate;

ALTER TABLE purchase_slips
    DROP COLUMN IF EXISTS issue_fx_scale,
    DROP COLUMN IF EXISTS issue_fx_rate_scaled,
    DROP COLUMN IF EXISTS issue_fx_rate_snapshot_id;
