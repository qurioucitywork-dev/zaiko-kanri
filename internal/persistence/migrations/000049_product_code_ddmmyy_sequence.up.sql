-- Standardize every management number as DDMMYY + a four-digit daily
-- sequence. Keep a durable one-to-one mapping so the migration is auditable.

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM (
            SELECT organization_id, COALESCE(purchase_date, created_at::date), COUNT(*) AS product_count
            FROM products
            GROUP BY organization_id, COALESCE(purchase_date, created_at::date)
            HAVING COUNT(*) > 9999
        ) AS daily_counts
    ) THEN
        RAISE EXCEPTION 'more than 9999 products exist for one organization and business date';
    END IF;
END;
$$;

CREATE TABLE IF NOT EXISTS product_code_migration_history (
    product_id TEXT PRIMARY KEY REFERENCES products(id),
    organization_id TEXT NOT NULL REFERENCES organizations(id),
    old_product_code TEXT NOT NULL,
    new_product_code TEXT NOT NULL,
    business_date DATE NOT NULL,
    migrated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, old_product_code),
    UNIQUE (organization_id, new_product_code)
);

CREATE TEMP TABLE product_code_renumbering ON COMMIT DROP AS
WITH ranked AS (
    SELECT
        id AS product_id,
        organization_id,
        product_code AS old_product_code,
        COALESCE(purchase_date, created_at::date) AS business_date,
        ROW_NUMBER() OVER (
            PARTITION BY organization_id, COALESCE(purchase_date, created_at::date)
            ORDER BY
                CASE
                    WHEN product_code ~ '^[0-9]{11}$' THEN RIGHT(product_code, 3)::INTEGER
                    ELSE 1000000
                END,
                created_at,
                product_code,
                id
        ) AS sequence
    FROM products
)
SELECT
    product_id,
    organization_id,
    old_product_code,
    business_date,
    sequence,
    TO_CHAR(business_date, 'DDMMYY') || LPAD(sequence::TEXT, 4, '0') AS new_product_code
FROM ranked;

INSERT INTO product_code_migration_history(
    product_id, organization_id, old_product_code, new_product_code, business_date, migrated_at
)
SELECT
    product_id, organization_id, old_product_code, new_product_code, business_date, NOW()
FROM product_code_renumbering
ON CONFLICT (product_id) DO UPDATE SET
    organization_id = EXCLUDED.organization_id,
    old_product_code = EXCLUDED.old_product_code,
    new_product_code = EXCLUDED.new_product_code,
    business_date = EXCLUDED.business_date,
    migrated_at = EXCLUDED.migrated_at;

-- Add the new-format check before updating rows. NOT VALID allows the legacy
-- rows to exist until the UPDATE below, while still enforcing every changed
-- or newly inserted row. Migration 000050 validates the whole table after
-- this transaction has committed and all deferred trigger events are clear.
ALTER TABLE products DROP CONSTRAINT IF EXISTS chk_products_product_code_format;
ALTER TABLE products ADD CONSTRAINT chk_products_product_code_format
    CHECK (product_code ~ '^[0-9]{10}$') NOT VALID;

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

    IF LEFT(NEW.product_code, 6) <> TO_CHAR(source_purchase_date, 'DDMMYY') THEN
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
SET product_code = mapping.new_product_code,
    updated_at = NOW()
FROM product_code_renumbering AS mapping
WHERE line.organization_id = mapping.organization_id
  AND (
      line.product_id = mapping.product_id
      OR (line.product_id IS NULL AND line.product_code = mapping.old_product_code)
  );

UPDATE products AS product
SET product_code = mapping.new_product_code,
    purchase_date = mapping.business_date,
    updated_at = NOW()
FROM product_code_renumbering AS mapping
WHERE product.id = mapping.product_id;

DELETE FROM product_code_sequences;

ALTER TABLE product_code_sequences
    DROP CONSTRAINT IF EXISTS product_code_sequences_last_sequence_check;
ALTER TABLE product_code_sequences
    ADD CONSTRAINT product_code_sequences_last_sequence_check
    CHECK (last_sequence BETWEEN 0 AND 9999);

INSERT INTO product_code_sequences(organization_id, business_date, last_sequence, updated_at)
SELECT organization_id, business_date, MAX(sequence)::INTEGER, NOW()
FROM product_code_renumbering
GROUP BY organization_id, business_date;
