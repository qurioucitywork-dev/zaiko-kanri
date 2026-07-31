ALTER TABLE shipment_slips
ADD COLUMN sales_slip_id TEXT REFERENCES sales_slips(id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_shipment_slips_sales_slip
    ON shipment_slips (organization_id, sales_slip_id)
    WHERE sales_slip_id IS NOT NULL;
