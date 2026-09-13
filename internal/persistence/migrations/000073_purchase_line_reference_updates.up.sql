-- A reference-only update does not change the confirmed purchase snapshot.
-- Revalidate only fields that can change the expected/generated item counts.
CREATE OR REPLACE FUNCTION enforce_purchase_line_inventory_consistency()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'UPDATE'
       AND NEW.purchase_slip_id IS NOT DISTINCT FROM OLD.purchase_slip_id
       AND NEW.quantity IS NOT DISTINCT FROM OLD.quantity
       AND NEW.line_item_kind IS NOT DISTINCT FROM OLD.line_item_kind
       AND NEW.generated_product_count IS NOT DISTINCT FROM OLD.generated_product_count
       AND NEW.generated_part_count IS NOT DISTINCT FROM OLD.generated_part_count THEN
        RETURN NULL;
    END IF;

    PERFORM assert_purchase_inventory_consistency(COALESCE(NEW.purchase_slip_id, OLD.purchase_slip_id));
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
