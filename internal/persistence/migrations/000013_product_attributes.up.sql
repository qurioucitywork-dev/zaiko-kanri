ALTER TABLE products
    ADD COLUMN IF NOT EXISTS belt_text TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS dial_text TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS bracelet_quantity INTEGER CHECK (bracelet_quantity IS NULL OR bracelet_quantity > 0);

ALTER TABLE purchase_slip_lines
    ADD COLUMN IF NOT EXISTS belt_text TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS dial_text TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS bracelet_quantity INTEGER CHECK (bracelet_quantity IS NULL OR bracelet_quantity > 0);
