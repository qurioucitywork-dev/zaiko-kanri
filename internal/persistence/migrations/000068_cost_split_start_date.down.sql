ALTER TABLE cost_adjustments DROP COLUMN IF EXISTS notes;
ALTER TABLE products DROP COLUMN IF EXISTS cost_adjustment_started_at;
