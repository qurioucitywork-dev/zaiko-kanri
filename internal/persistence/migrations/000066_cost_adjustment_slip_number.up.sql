ALTER TABLE cost_adjustments
    ADD COLUMN IF NOT EXISTS slip_number TEXT;

UPDATE cost_adjustments
SET slip_number = 'CA-' || TO_CHAR(adjustment_date, 'YYYYMMDD') || '-' || UPPER(SUBSTRING(MD5(id), 1, 8))
WHERE slip_number IS NULL OR BTRIM(slip_number) = '';

ALTER TABLE cost_adjustments
    ALTER COLUMN slip_number SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_cost_adjustments_slip_number
    ON cost_adjustments (organization_id, slip_number);
