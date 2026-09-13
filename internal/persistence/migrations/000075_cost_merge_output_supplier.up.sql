UPDATE products p
SET supplier_id = '',
    supplier_role_id = NULL,
    updated_at = NOW()
WHERE p.deleted_at IS NULL
  AND EXISTS (
      SELECT 1
      FROM product_cost_merges merge_record
      WHERE merge_record.organization_id = p.organization_id
        AND merge_record.adjustment_id = p.cost_adjustment_id
        AND merge_record.status = 'confirmed'
  )
  AND (COALESCE(p.supplier_id, '') <> '' OR p.supplier_role_id IS NOT NULL);
