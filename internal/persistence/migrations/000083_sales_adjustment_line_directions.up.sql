ALTER TABLE sales_adjustment_lines ADD COLUMN direction TEXT NOT NULL DEFAULT '' CHECK (direction IN ('','increase','decrease'));
UPDATE sales_adjustment_lines l SET direction=a.direction FROM sales_adjustment_slips a WHERE a.id=l.sales_adjustment_slip_id;
ALTER TABLE sales_adjustment_slips DROP CONSTRAINT sales_adjustment_slips_direction_check;
ALTER TABLE sales_adjustment_slips ADD CONSTRAINT sales_adjustment_slips_direction_check CHECK (direction IN ('increase','decrease','mixed'));
