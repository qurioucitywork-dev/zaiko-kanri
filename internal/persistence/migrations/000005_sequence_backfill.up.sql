INSERT INTO document_sequences(organization_id, document_type, business_year, last_sequence, updated_at)
SELECT
    organization_id,
    'purchase',
    EXTRACT(YEAR FROM purchase_date)::INTEGER,
    MAX((regexp_match(slip_number, '([0-9]+)$'))[1]::INTEGER),
    CURRENT_TIMESTAMP
FROM purchase_slips
GROUP BY organization_id, EXTRACT(YEAR FROM purchase_date)::INTEGER
ON CONFLICT (organization_id, document_type, business_year)
DO UPDATE SET
    last_sequence = GREATEST(document_sequences.last_sequence, EXCLUDED.last_sequence),
    updated_at = EXCLUDED.updated_at;

INSERT INTO product_code_sequences(organization_id, business_date, last_sequence, updated_at)
SELECT
    organization_id,
    purchase_date,
    MAX(RIGHT(product_code, 3)::INTEGER),
    CURRENT_TIMESTAMP
FROM products
WHERE purchase_date IS NOT NULL
  AND product_code ~ '^[0-9]{11}$'
GROUP BY organization_id, purchase_date
ON CONFLICT (organization_id, business_date)
DO UPDATE SET
    last_sequence = GREATEST(product_code_sequences.last_sequence, EXCLUDED.last_sequence),
    updated_at = EXCLUDED.updated_at;
