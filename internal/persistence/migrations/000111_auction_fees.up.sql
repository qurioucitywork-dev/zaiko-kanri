ALTER TABLE business_partners ADD COLUMN auction_fees JSONB NOT NULL DEFAULT '{}';
ALTER TABLE purchase_slip_lines ADD COLUMN auction_fee JSONB NOT NULL DEFAULT '{}';
ALTER TABLE sales_lines ADD COLUMN auction_fee JSONB NOT NULL DEFAULT '{}';
