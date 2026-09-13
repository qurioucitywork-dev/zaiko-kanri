-- Migration 000075 intentionally removed the supplier from cost-merge records,
-- but source products must retain their immutable acquisition information.
UPDATE products p
SET supplier_id = COALESCE(purchase.supplier_role_id, ''),
    supplier_role_id = purchase.supplier_role_id,
    updated_at = NOW()
FROM product_cost_merge_inputs merge_input
JOIN product_cost_merges merge_record
  ON merge_record.id = merge_input.merge_id
 AND merge_record.status = 'confirmed'
JOIN purchase_slip_lines purchase_line ON TRUE
JOIN purchase_slips purchase
  ON purchase.id = purchase_line.purchase_slip_id
WHERE merge_input.product_id = p.id
  AND merge_record.organization_id = p.organization_id
  AND purchase_line.id = p.purchase_slip_line_id
  AND purchase.organization_id = p.organization_id
  AND p.deleted_at IS NULL;

-- Only the newly generated output product has no purchase supplier.
UPDATE products p
SET supplier_id = '',
    supplier_role_id = NULL,
    updated_at = NOW()
WHERE p.deleted_at IS NULL
  AND EXISTS (
      SELECT 1
      FROM cost_adjustment_items adjustment_item
      JOIN product_cost_merges merge_record
        ON merge_record.adjustment_id = adjustment_item.cost_adjustment_id
       AND merge_record.organization_id = p.organization_id
       AND merge_record.status = 'confirmed'
      WHERE adjustment_item.output_product_id = p.id
  )
  AND (COALESCE(p.supplier_id, '') <> '' OR p.supplier_role_id IS NOT NULL);
