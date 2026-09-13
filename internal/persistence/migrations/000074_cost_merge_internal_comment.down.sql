UPDATE products p
SET internal_comment = CONCAT_WS(
        E'\n',
        '原価管理番号: ' || ca.source_product_code,
        NULLIF(BTRIM(COALESCE(p.internal_comment, ''), E'\r\n '), '')
    ),
    updated_at = NOW()
FROM cost_adjustments ca
WHERE ca.id = p.cost_adjustment_id
  AND ca.organization_id = p.organization_id
  AND ca.adjustment_type = 'combine'
  AND ca.status = 'confirmed'
  AND p.deleted_at IS NULL
  AND p.internal_comment NOT LIKE '原価管理番号:%';
