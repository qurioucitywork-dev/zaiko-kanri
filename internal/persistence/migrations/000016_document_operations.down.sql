DROP INDEX IF EXISTS idx_document_generation_events_created;
DROP INDEX IF EXISTS idx_document_generation_events_document;
DROP TABLE IF EXISTS document_generation_events;
ALTER TABLE return_slips
    DROP COLUMN IF EXISTS tracking_number,
    DROP COLUMN IF EXISTS carrier;
