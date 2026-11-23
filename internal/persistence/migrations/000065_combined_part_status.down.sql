UPDATE parts SET status = 'invalid' WHERE status = 'combined';

ALTER TABLE parts DROP CONSTRAINT IF EXISTS parts_status_check;
ALTER TABLE parts ADD CONSTRAINT parts_status_check
    CHECK (status IN ('in_stock', 'cost_adjustment', 'invalid'));
