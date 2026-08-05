ALTER TABLE purchase_slip_lines
    DROP COLUMN IF EXISTS bracelet_quantity,
    DROP COLUMN IF EXISTS dial_text,
    DROP COLUMN IF EXISTS belt_text;

ALTER TABLE products
    DROP COLUMN IF EXISTS bracelet_quantity,
    DROP COLUMN IF EXISTS dial_text,
    DROP COLUMN IF EXISTS belt_text;
