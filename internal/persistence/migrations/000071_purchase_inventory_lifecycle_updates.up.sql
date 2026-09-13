-- A confirmed purchase slip is an immutable acquisition snapshot. Product
-- lifecycle changes (sale, shipment, cost adjustment, and similar status/cost
-- updates) must not revalidate the original generated-item count. Revalidate
-- only when the product is attached to/detached from a purchase line or when
-- its deletion state changes.
CREATE OR REPLACE FUNCTION enforce_product_purchase_inventory_consistency()
RETURNS TRIGGER AS $$
DECLARE
    target_line_id TEXT;
    target_purchase_id TEXT;
BEGIN
    IF TG_OP = 'UPDATE'
       AND NEW.purchase_slip_line_id IS NOT DISTINCT FROM OLD.purchase_slip_line_id
       AND NEW.deleted_at IS NOT DISTINCT FROM OLD.deleted_at THEN
        RETURN NULL;
    END IF;

    target_line_id := COALESCE(NEW.purchase_slip_line_id, OLD.purchase_slip_line_id);
    IF target_line_id IS NULL OR target_line_id = '' THEN
        RETURN NULL;
    END IF;

    SELECT purchase_slip_id INTO target_purchase_id
    FROM purchase_slip_lines
    WHERE id = target_line_id;

    PERFORM assert_purchase_inventory_consistency(target_purchase_id);
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
