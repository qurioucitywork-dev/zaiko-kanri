DROP INDEX IF EXISTS idx_stocktake_lines_review;
-- SQLite cannot safely drop these compatibility columns on older versions.
-- The application ignores them after rolling back the matching binary.
