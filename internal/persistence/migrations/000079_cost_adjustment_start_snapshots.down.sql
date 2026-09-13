DROP TABLE IF EXISTS cost_adjustment_start_snapshots;
UPDATE product_cost_merges SET status = 'draft' WHERE status = 'cancelled';
ALTER TABLE product_cost_merges DROP CONSTRAINT product_cost_merges_status_check;
ALTER TABLE product_cost_merges ADD CONSTRAINT product_cost_merges_status_check
    CHECK (status IN ('draft', 'confirmed'));
