-- DESTRUCTIVE rollback for an EMPTY, isolated staging database only.
-- Never use after production data import. Point-in-time restore is the
-- production recovery mechanism.

\set ON_ERROR_STOP on

BEGIN;

SET LOCAL lock_timeout = '10s';
SET LOCAL statement_timeout = '5min';

DO $$
BEGIN
    IF current_setting('zaiko.allow_destructive_rollback', true) IS DISTINCT FROM 'on' THEN
        RAISE EXCEPTION
            'refusing destructive rollback; set zaiko.allow_destructive_rollback=on for an empty staging database';
    END IF;
END;
$$;

SELECT pg_advisory_xact_lock(hashtextextended('zaiko:postgres:migration', 0));

DROP SCHEMA zaiko CASCADE;

COMMIT;
