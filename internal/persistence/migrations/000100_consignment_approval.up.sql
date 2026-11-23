ALTER TABLE consignment_slips DROP CONSTRAINT consignment_slips_status_check;
ALTER TABLE consignment_slips ADD CONSTRAINT consignment_slips_status_check CHECK (status IN ('draft','confirmed','cancelled'));
ALTER TABLE consignment_slips ALTER COLUMN confirmed_at DROP NOT NULL;
ALTER TABLE consignment_slips ALTER COLUMN confirmed_by DROP NOT NULL;
