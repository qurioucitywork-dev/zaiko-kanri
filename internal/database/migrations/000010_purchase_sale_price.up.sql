ALTER TABLE purchase_slip_lines
ADD COLUMN base_sale_price_minor INTEGER NOT NULL DEFAULT 0
CHECK (base_sale_price_minor >= 0);
