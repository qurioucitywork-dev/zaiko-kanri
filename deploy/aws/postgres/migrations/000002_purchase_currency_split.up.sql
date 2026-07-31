ALTER TABLE zaiko.purchase_slip_lines
    ADD COLUMN base_sale_currency CHAR(3) NOT NULL DEFAULT 'JPY'
    CHECK (base_sale_currency ~ '^[A-Z]{3}$');

UPDATE zaiko.purchase_slip_lines AS purchase_line
SET base_sale_currency = product.base_sale_currency
FROM zaiko.products AS product
WHERE product.organization_id = purchase_line.organization_id
  AND product.purchase_slip_line_id = purchase_line.id;

ALTER TABLE zaiko.products
    ALTER COLUMN base_sale_currency SET DEFAULT 'USD';
