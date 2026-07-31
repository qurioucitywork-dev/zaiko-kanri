DROP INDEX IF EXISTS idx_shipment_slips_sales_slip;
ALTER TABLE shipment_slips DROP COLUMN sales_slip_id;
