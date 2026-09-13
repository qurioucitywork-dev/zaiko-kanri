WITH source_slips AS (
    SELECT
        merge_record.organization_id,
        merge_record.management_code,
        BTRIM(REGEXP_REPLACE(comment_line, '^(原価調整伝票番号|調整伝票番号):[[:space:]]*', '')) AS slip_number
    FROM product_cost_merges merge_record
    CROSS JOIN LATERAL jsonb_array_elements(merge_record.inputs_json::jsonb) source_snapshot
    CROSS JOIN LATERAL regexp_split_to_table(COALESCE(source_snapshot->>'internalComment', ''), E'\\r?\\n') comment_line
    WHERE merge_record.status = 'confirmed'
      AND comment_line ~ '^(原価調整伝票番号|調整伝票番号):[[:space:]]*[^[:space:]]+'
), all_slips AS (
    SELECT organization_id, management_code, slip_number FROM source_slips
    UNION
    SELECT merge_record.organization_id, merge_record.management_code, adjustment.slip_number
    FROM product_cost_merges merge_record
    JOIN cost_adjustments adjustment
      ON adjustment.id = merge_record.adjustment_id
     AND adjustment.organization_id = merge_record.organization_id
    WHERE merge_record.status = 'confirmed'
), slip_history AS (
    SELECT
        organization_id,
        management_code,
        string_agg('調整伝票番号: ' || slip_number, E'\n' ORDER BY slip_number) AS slip_lines
    FROM all_slips
    WHERE BTRIM(slip_number) <> ''
    GROUP BY organization_id, management_code
), cleaned_output AS (
    SELECT
        p.id,
        p.organization_id,
        history.slip_lines,
        NULLIF(BTRIM(REGEXP_REPLACE(
            COALESCE(p.internal_comment, ''),
            '(^|\n)[[:space:]]*(原価調整伝票番号|調整伝票番号):[^\n]*',
            '',
            'g'
        ), E'\r\n '), '') AS retained_comment
    FROM products p
    JOIN slip_history history
      ON history.organization_id = p.organization_id
     AND history.management_code = p.product_code
    WHERE p.deleted_at IS NULL
)
UPDATE products p
SET internal_comment = CONCAT_WS(E'\n', cleaned_output.slip_lines, cleaned_output.retained_comment),
    updated_at = NOW()
FROM cleaned_output
WHERE p.id = cleaned_output.id
  AND p.organization_id = cleaned_output.organization_id
  AND p.internal_comment IS DISTINCT FROM CONCAT_WS(E'\n', cleaned_output.slip_lines, cleaned_output.retained_comment);
