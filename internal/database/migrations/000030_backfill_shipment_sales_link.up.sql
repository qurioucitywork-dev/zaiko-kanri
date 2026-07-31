DROP INDEX IF EXISTS idx_shipment_slips_sales_slip;

UPDATE shipment_slips
SET sales_slip_id = (
    SELECT MIN(sales_line.sales_slip_id)
    FROM shipment_lines AS shipment_line
    JOIN sales_shipment_allocations AS allocation
      ON allocation.shipment_line_id = shipment_line.id
    JOIN sales_lines AS sales_line
      ON sales_line.id = allocation.sales_line_id
    WHERE shipment_line.shipment_slip_id = shipment_slips.id
)
WHERE sales_slip_id IS NULL
  AND EXISTS (
    SELECT 1
    FROM shipment_lines AS shipment_line
    JOIN sales_shipment_allocations AS allocation
      ON allocation.shipment_line_id = shipment_line.id
    WHERE shipment_line.shipment_slip_id = shipment_slips.id
  );
