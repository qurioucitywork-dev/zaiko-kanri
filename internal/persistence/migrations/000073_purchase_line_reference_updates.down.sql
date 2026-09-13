CREATE OR REPLACE FUNCTION enforce_purchase_line_inventory_consistency()
RETURNS TRIGGER AS $$
BEGIN
    PERFORM assert_purchase_inventory_consistency(COALESCE(NEW.purchase_slip_id, OLD.purchase_slip_id));
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
