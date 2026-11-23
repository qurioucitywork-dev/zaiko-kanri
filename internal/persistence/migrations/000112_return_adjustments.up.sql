-- Financial-only lines never reference products or participate in inventory transitions.
ALTER TABLE return_slips ADD COLUMN adjustment_details JSONB NOT NULL DEFAULT '{}';
