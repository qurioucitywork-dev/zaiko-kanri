ALTER TABLE products ADD COLUMN IF NOT EXISTS accessory_supplements TEXT NOT NULL DEFAULT '{}';
ALTER TABLE purchase_slip_lines ADD COLUMN IF NOT EXISTS accessory_supplements TEXT NOT NULL DEFAULT '{}';
