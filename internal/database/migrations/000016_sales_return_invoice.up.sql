ALTER TABLE return_takehome_items ADD COLUMN invoice_issued_at TEXT;
ALTER TABLE return_takehome_items ADD COLUMN invoice_issued_by TEXT REFERENCES users(id);
ALTER TABLE return_takehome_items ADD COLUMN invoice_printed_at TEXT;
ALTER TABLE return_takehome_items ADD COLUMN invoice_printed_by TEXT REFERENCES users(id);
