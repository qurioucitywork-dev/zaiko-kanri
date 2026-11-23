ALTER TABLE products DROP CONSTRAINT IF EXISTS chk_products_product_code_format;

CREATE OR REPLACE FUNCTION enforce_product_purchase_date_consistency()
RETURNS TRIGGER AS $$
DECLARE
    source_purchase_date DATE;
BEGIN
    IF NEW.purchase_slip_line_id IS NULL THEN
        RETURN NEW;
    END IF;

    SELECT slip.purchase_date
    INTO source_purchase_date
    FROM purchase_slip_lines AS line
    JOIN purchase_slips AS slip ON slip.id = line.purchase_slip_id
    WHERE line.id = NEW.purchase_slip_line_id;

    IF source_purchase_date IS NULL THEN
        RAISE EXCEPTION 'purchase slip date was not found for product %', NEW.id
            USING ERRCODE = '23503';
    END IF;

    IF NEW.purchase_date IS DISTINCT FROM source_purchase_date THEN
        RAISE EXCEPTION 'product purchase date must match its purchase slip date'
            USING ERRCODE = '23514';
    END IF;

    IF NEW.product_code ~ '^[0-9]{11}$'
       AND LEFT(NEW.product_code, 8) <> TO_CHAR(source_purchase_date, 'YYYYMMDD') THEN
        RAISE EXCEPTION 'product code date prefix must match its purchase slip date'
            USING ERRCODE = '23514';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_products_purchase_date_consistency ON products;
CREATE TRIGGER trg_products_purchase_date_consistency
BEFORE INSERT OR UPDATE OF purchase_date, purchase_slip_line_id, product_code
ON products
FOR EACH ROW
EXECUTE FUNCTION enforce_product_purchase_date_consistency();

UPDATE stocktake_lines AS line
SET product_code = history.old_product_code,
    updated_at = NOW()
FROM product_code_migration_history AS history
WHERE line.organization_id = history.organization_id
  AND (
      line.product_id = history.product_id
      OR (line.product_id IS NULL AND line.product_code = history.new_product_code)
  );

UPDATE products AS product
SET product_code = history.old_product_code,
    updated_at = NOW()
FROM product_code_migration_history AS history
WHERE product.id = history.product_id;

DELETE FROM product_code_sequences;

ALTER TABLE product_code_sequences
    DROP CONSTRAINT IF EXISTS product_code_sequences_last_sequence_check;
ALTER TABLE product_code_sequences
    ADD CONSTRAINT product_code_sequences_last_sequence_check
    CHECK (last_sequence BETWEEN 0 AND 999);

INSERT INTO product_code_sequences(organization_id, business_date, last_sequence, updated_at)
SELECT organization_id, business_date, MAX(RIGHT(old_product_code, 3)::INTEGER), NOW()
FROM product_code_migration_history
WHERE old_product_code ~ '^[0-9]{11}$'
GROUP BY organization_id, business_date;

DROP TABLE IF EXISTS product_code_migration_history;
