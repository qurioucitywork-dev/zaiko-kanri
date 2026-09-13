WITH cleaned AS (
    SELECT
        p.id,
        p.organization_id,
        ca.slip_number,
        BTRIM(
            REGEXP_REPLACE(
                COALESCE(p.internal_comment, ''),
                '(^|\n)[[:space:]]*(対象商品管理番号|原価管理番号|原価調整伝票番号|調整伝票番号):[^\n]*',
                '',
                'g'
            ),
            E'\r\n '
        ) AS retained_comment
    FROM products p
    JOIN cost_adjustments ca
      ON ca.id = p.cost_adjustment_id
     AND ca.organization_id = p.organization_id
    WHERE ca.adjustment_type = 'combine'
      AND ca.status = 'confirmed'
      AND p.deleted_at IS NULL
)
UPDATE products p
SET internal_comment = CONCAT_WS(
        E'\n',
        '調整伝票番号: ' || cleaned.slip_number,
        NULLIF(cleaned.retained_comment, '')
    ),
    updated_at = NOW()
FROM cleaned
WHERE p.id = cleaned.id
  AND p.organization_id = cleaned.organization_id
  AND p.internal_comment IS DISTINCT FROM CONCAT_WS(
        E'\n',
        '調整伝票番号: ' || cleaned.slip_number,
        NULLIF(cleaned.retained_comment, '')
    );
