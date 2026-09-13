ALTER TABLE products ADD COLUMN IF NOT EXISTS cost_adjustment_started_at TIMESTAMPTZ;
ALTER TABLE cost_adjustments ADD COLUMN IF NOT EXISTS notes TEXT NOT NULL DEFAULT '';
UPDATE cost_adjustments SET notes='原価分け' WHERE adjustment_type='breakdown' AND notes='';
