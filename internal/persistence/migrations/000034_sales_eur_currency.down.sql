DELETE FROM exchange_rate_snapshots WHERE provider='initial EUR master rate';

ALTER TABLE sales_lines DROP CONSTRAINT IF EXISTS sales_lines_sale_currency_check;
ALTER TABLE sales_lines ADD CONSTRAINT sales_lines_sale_currency_check CHECK (sale_currency IN ('JPY','USD'));
ALTER TABLE sales_slips DROP CONSTRAINT IF EXISTS sales_slips_display_currency_check;
ALTER TABLE sales_slips ADD CONSTRAINT sales_slips_display_currency_check CHECK (display_currency IN ('JPY','USD'));
ALTER TABLE exchange_rate_snapshots DROP CONSTRAINT IF EXISTS exchange_rate_snapshots_base_currency_check;
ALTER TABLE exchange_rate_snapshots ADD CONSTRAINT exchange_rate_snapshots_base_currency_check CHECK (base_currency IN ('JPY','USD'));
