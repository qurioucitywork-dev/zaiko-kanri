-- High-value inventory is tracked individually. Split the one historical
-- two-quantity preview line into two distinct products and prevent future
-- purchase lines from representing more than one product.

INSERT INTO purchase_slip_lines(
    id,purchase_slip_id,line_number,quantity,unit_cost_minor,cost_currency,
    base_sale_price_minor,base_sale_currency,brand_id,material_id,movement_id,
    condition_id,brand_text,model_number,reference_number,serial_number,
    product_type,sku,generated_product_count,created_at,accessory_codes,notes,
    duplicate_serial_reason,converted_total_jpy,fx_rate_snapshot_id,
    fx_rate_scaled,fx_scale,belt_text,dial_text,bracelet_quantity
)
SELECT
    'pul_preview_pi0003_line3',line.purchase_slip_id,3,1,line.unit_cost_minor,line.cost_currency,
    line.base_sale_price_minor,line.base_sale_currency,line.brand_id,line.material_id,line.movement_id,
    line.condition_id,line.brand_text,'Tank Must Small','WSTA0051','CT-LOCAL-002',
    line.product_type,'BULK-002-B',1,line.created_at,line.accessory_codes,line.notes,
    line.duplicate_serial_reason,line.converted_total_jpy,line.fx_rate_snapshot_id,
    line.fx_rate_scaled,line.fx_scale,line.belt_text,line.dial_text,line.bracelet_quantity
FROM purchase_slip_lines AS line
JOIN purchase_slips AS slip ON slip.id=line.purchase_slip_id
JOIN organizations AS organization ON organization.id=slip.organization_id
WHERE organization.code='PREVIEW'
  AND slip.slip_number='PI-2026-0003'
  AND line.line_number=2
  AND line.quantity=2
ON CONFLICT DO NOTHING;

UPDATE purchase_slip_lines AS line
SET quantity=1,
    generated_product_count=1,
    sku='BULK-002-A',
    model_number='Tank Must Large',
    reference_number='WSTA0041',
    serial_number='CT-LOCAL-001'
FROM purchase_slips AS slip
JOIN organizations AS organization ON organization.id=slip.organization_id
WHERE line.purchase_slip_id=slip.id
  AND organization.code='PREVIEW'
  AND slip.slip_number='PI-2026-0003'
  AND line.line_number=2;

UPDATE products
SET sku='BULK-002-A',
    model_number='Tank Must Large',
    reference_number='WSTA0041',
    serial_number='CT-LOCAL-001',
    updated_at=NOW()
WHERE organization_id=(SELECT id FROM organizations WHERE code='PREVIEW')
  AND product_code='20260802003';

UPDATE products
SET purchase_slip_line_id='pul_preview_pi0003_line3',
    sku='BULK-002-B',
    model_number='Tank Must Small',
    reference_number='WSTA0051',
    serial_number='CT-LOCAL-002',
    updated_at=NOW()
WHERE organization_id=(SELECT id FROM organizations WHERE code='PREVIEW')
  AND product_code='20260802004'
  AND EXISTS(SELECT 1 FROM purchase_slip_lines WHERE id='pul_preview_pi0003_line3');

ALTER TABLE purchase_slip_lines
    DROP CONSTRAINT IF EXISTS purchase_slip_lines_single_item_chk;
ALTER TABLE purchase_slip_lines
    ADD CONSTRAINT purchase_slip_lines_single_item_chk CHECK (quantity=1);
