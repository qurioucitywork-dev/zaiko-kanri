DROP INDEX IF EXISTS idx_cost_adjustments_slip_number;
ALTER TABLE cost_adjustments DROP COLUMN IF EXISTS slip_number;
