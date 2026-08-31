ALTER TABLE purchase_slip_lines
    ADD COLUMN IF NOT EXISTS accessory_codes JSONB NOT NULL DEFAULT '[]'::JSONB;

ALTER TABLE purchase_slip_lines
    ADD COLUMN IF NOT EXISTS notes TEXT NOT NULL DEFAULT '';

ALTER TABLE purchase_slip_lines
    ADD COLUMN IF NOT EXISTS duplicate_serial_reason TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_purchase_lines_slip
    ON purchase_slip_lines (purchase_slip_id, line_number);
