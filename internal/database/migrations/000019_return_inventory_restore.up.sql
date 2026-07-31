ALTER TABLE return_takehome_items ADD COLUMN inventory_restored_at TEXT;
ALTER TABLE return_takehome_items ADD COLUMN inventory_restored_by TEXT REFERENCES users(id);
ALTER TABLE return_takehome_items ADD COLUMN restore_box_text TEXT NOT NULL DEFAULT '';
ALTER TABLE return_takehome_items ADD COLUMN restore_comment_text TEXT NOT NULL DEFAULT '';

CREATE INDEX idx_return_takehome_restore
ON return_takehome_items(organization_id,inventory_restored_at,sales_slip_id);
