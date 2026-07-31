ALTER TABLE zaiko.products
    ALTER COLUMN base_sale_currency SET DEFAULT 'JPY';

ALTER TABLE zaiko.purchase_slip_lines
    DROP COLUMN base_sale_currency;
