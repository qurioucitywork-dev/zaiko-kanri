ALTER TABLE purchase_slip_lines
ADD COLUMN base_sale_currency TEXT NOT NULL DEFAULT 'JPY'
CHECK (base_sale_currency IN ('JPY', 'USD'));

UPDATE purchase_slip_lines
SET base_sale_currency = currency;
