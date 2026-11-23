ALTER TABLE purchase_slip_lines DROP CONSTRAINT purchase_slip_lines_cost_currency_check;
ALTER TABLE purchase_slip_lines ADD CONSTRAINT purchase_slip_lines_cost_currency_check CHECK (cost_currency IN ('JPY', 'USD', 'HKD'));

ALTER TABLE products DROP CONSTRAINT products_cost_currency_check;
ALTER TABLE products ADD CONSTRAINT products_cost_currency_check CHECK (cost_currency IN ('JPY', 'USD', 'HKD'));
