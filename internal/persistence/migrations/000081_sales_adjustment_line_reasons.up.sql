-- Keep old voucher reasons intact; readers use them as the fallback for old lines.
ALTER TABLE sales_adjustment_lines
    ADD COLUMN IF NOT EXISTS reason TEXT NOT NULL DEFAULT '';
