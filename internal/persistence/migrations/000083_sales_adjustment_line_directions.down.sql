-- Refuse rollback if mixed vouchers exist: removing line directions would lose meaning.
ALTER TABLE sales_adjustment_slips DROP CONSTRAINT sales_adjustment_slips_direction_check;
ALTER TABLE sales_adjustment_slips ADD CONSTRAINT sales_adjustment_slips_direction_check CHECK (direction IN ('increase','decrease'));
ALTER TABLE sales_adjustment_lines DROP COLUMN direction;
