ALTER TABLE publication_box_products
 ADD COLUMN basis_status TEXT NOT NULL DEFAULT 'purchasing',
 ADD COLUMN visibility_state TEXT NOT NULL DEFAULT 'waiting' CHECK(visibility_state IN ('waiting','visible','hidden')),
 ADD COLUMN visibility_reason TEXT NOT NULL DEFAULT 'purchasing';

-- Existing memberships begin with the current status; subsequent changes latch.
DELETE FROM publication_box_products x USING products p
 WHERE p.id=x.product_id AND (p.deleted_at IS NOT NULL OR p.inventory_status NOT IN ('purchasing','in_stock','shipped','reserved'));
UPDATE publication_box_products x SET basis_status=p.inventory_status,
 visibility_state=CASE p.inventory_status WHEN 'purchasing' THEN 'waiting' WHEN 'reserved' THEN 'hidden' ELSE 'visible' END,
 visibility_reason=CASE WHEN p.inventory_status IN ('purchasing','reserved') THEN p.inventory_status ELSE '' END
 FROM products p WHERE p.id=x.product_id;
UPDATE products p SET publication_status='private'
 WHERE p.publication_status='public' AND NOT EXISTS(SELECT 1 FROM publication_box_products x JOIN publication_boxes b ON b.id=x.box_id AND b.is_active WHERE x.product_id=p.id);

CREATE FUNCTION initialize_box_product_visibility() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE product_status TEXT; product_deleted TIMESTAMPTZ; product_org TEXT; box_org TEXT;
BEGIN
 SELECT inventory_status,deleted_at,organization_id INTO product_status,product_deleted,product_org FROM products WHERE id=NEW.product_id FOR UPDATE;
 SELECT organization_id INTO box_org FROM publication_boxes WHERE id=NEW.box_id;
 IF product_deleted IS NOT NULL OR product_status IS NULL OR product_status NOT IN ('purchasing','in_stock','shipped') OR product_org IS DISTINCT FROM box_org THEN
  RAISE EXCEPTION 'BOX accepts only purchasing, in_stock or shipped products';
 END IF;
 NEW.basis_status:=product_status;
 NEW.visibility_state:=CASE WHEN product_status='purchasing' THEN 'waiting' ELSE 'visible' END;
 NEW.visibility_reason:=CASE WHEN product_status='purchasing' THEN 'purchasing' ELSE '' END;
 RETURN NEW;
END $$;
CREATE TRIGGER initialize_box_product_visibility BEFORE INSERT ON publication_box_products
 FOR EACH ROW EXECUTE FUNCTION initialize_box_product_visibility();

-- Runs in the same transaction as every workflow's inventory status change.
CREATE FUNCTION track_box_product_visibility() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.inventory_status IS NOT DISTINCT FROM OLD.inventory_status AND NEW.deleted_at IS NOT DISTINCT FROM OLD.deleted_at THEN RETURN NEW; END IF;
 IF NEW.deleted_at IS NOT NULL OR NEW.inventory_status NOT IN ('purchasing','in_stock','shipped','reserved') THEN
  DELETE FROM publication_box_products WHERE product_id=NEW.id;
  NEW.publication_status:='private';
 ELSE
  UPDATE publication_box_products SET
   visibility_state=CASE
    WHEN visibility_state='waiting' AND basis_status='purchasing' AND OLD.inventory_status='purchasing' AND NEW.inventory_status='in_stock' THEN 'visible'
    WHEN NEW.inventory_status IN ('purchasing','reserved') OR NEW.inventory_status<>basis_status THEN 'hidden'
    ELSE visibility_state END,
   visibility_reason=CASE
    WHEN visibility_state='waiting' AND basis_status='purchasing' AND OLD.inventory_status='purchasing' AND NEW.inventory_status='in_stock' THEN ''
    WHEN NEW.inventory_status='reserved' THEN 'reserved'
    WHEN NEW.inventory_status='purchasing' OR NEW.inventory_status<>basis_status THEN 'status_changed'
    ELSE visibility_reason END,
   basis_status=CASE
    WHEN visibility_state='waiting' AND basis_status='purchasing' AND OLD.inventory_status='purchasing' AND NEW.inventory_status='in_stock' THEN 'in_stock'
    ELSE basis_status END
   WHERE product_id=NEW.id;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER track_box_product_visibility BEFORE UPDATE OF inventory_status,deleted_at ON products
 FOR EACH ROW EXECUTE FUNCTION track_box_product_visibility();
