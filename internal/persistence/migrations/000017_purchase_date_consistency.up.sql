-- Products generated from a purchase slip must keep the same business date as
-- the source slip. The product code also embeds that date as YYYYMMDD.

UPDATE products AS product
SET purchase_date = slip.purchase_date,
    updated_at = NOW()
FROM purchase_slip_lines AS line
JOIN purchase_slips AS slip ON slip.id = line.purchase_slip_id
WHERE product.purchase_slip_line_id = line.id
  AND product.purchase_date IS DISTINCT FROM slip.purchase_date;

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
