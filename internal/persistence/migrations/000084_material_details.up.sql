-- The shared master API exposes the material detail as meaning.
ALTER TABLE materials ADD COLUMN IF NOT EXISTS meaning TEXT NOT NULL DEFAULT '';
