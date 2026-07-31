ALTER TABLE purchase_return_slips ADD COLUMN invoice_issued_at TEXT;
ALTER TABLE purchase_return_slips ADD COLUMN invoice_issued_by TEXT REFERENCES users(id);
ALTER TABLE purchase_return_slips ADD COLUMN invoice_printed_at TEXT;
ALTER TABLE purchase_return_slips ADD COLUMN invoice_printed_by TEXT REFERENCES users(id);
