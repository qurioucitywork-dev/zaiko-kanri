-- Some merge sources are themselves cost-adjustment outputs and do not have a
-- direct purchase-slip-line link. Restore those source suppliers from the
-- immutable snapshot captured when the merge started.
WITH source_snapshot AS (
    SELECT
        merge_record.organization_id,
        snapshot->>'id' AS product_id,
        NULLIF(snapshot->>'supplierRoleId', '') AS supplier_role_id
    FROM product_cost_merges merge_record
    CROSS JOIN LATERAL jsonb_array_elements(merge_record.inputs_json::jsonb) snapshot
    WHERE merge_record.status = 'confirmed'
)
UPDATE products p
SET supplier_id = COALESCE(source_snapshot.supplier_role_id, ''),
    supplier_role_id = source_snapshot.supplier_role_id,
    updated_at = NOW()
FROM source_snapshot
WHERE p.id = source_snapshot.product_id
  AND p.organization_id = source_snapshot.organization_id
  AND p.deleted_at IS NULL;

-- The reserved management code identifies the single newly generated product.
UPDATE products p
SET supplier_id = '',
    supplier_role_id = NULL,
    updated_at = NOW()
FROM product_cost_merges merge_record
WHERE merge_record.organization_id = p.organization_id
  AND merge_record.adjustment_id = p.cost_adjustment_id
  AND merge_record.status = 'confirmed'
  AND p.product_code = merge_record.management_code
  AND p.deleted_at IS NULL
  AND (COALESCE(p.supplier_id, '') <> '' OR p.supplier_role_id IS NOT NULL);
