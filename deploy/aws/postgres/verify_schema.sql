-- Read-only post-migration verification for an isolated PostgreSQL database.
\set ON_ERROR_STOP on

SET TIME ZONE 'UTC';
SET default_transaction_read_only = on;

SELECT version, name, checksum_sha256, applied_at, applied_by
FROM zaiko.schema_migrations
ORDER BY version;

WITH required_permissions(permission_key) AS (
    VALUES
        ('inventory.write'),
        ('purchase.confirm'),
        ('sales.confirm'),
        ('shipment.confirm')
)
SELECT required_permissions.permission_key AS missing_permission
FROM required_permissions
LEFT JOIN zaiko.permissions
  ON zaiko.permissions.permission_key = required_permissions.permission_key
WHERE zaiko.permissions.permission_key IS NULL
ORDER BY required_permissions.permission_key;

DO $$
BEGIN
    IF EXISTS (
        WITH required_permissions(permission_key) AS (
            VALUES
                ('inventory.write'),
                ('purchase.confirm'),
                ('sales.confirm'),
                ('shipment.confirm')
        )
        SELECT 1
        FROM required_permissions
        LEFT JOIN zaiko.permissions
          ON zaiko.permissions.permission_key = required_permissions.permission_key
        WHERE zaiko.permissions.permission_key IS NULL
    ) THEN
        RAISE EXCEPTION 'required mutation permission is missing';
    END IF;
END
$$;

SELECT COUNT(*) AS table_count
FROM information_schema.tables
WHERE table_schema = 'zaiko'
  AND table_type = 'BASE TABLE';

SELECT table_name, column_name, data_type
FROM information_schema.columns
WHERE table_schema = 'zaiko'
  AND (
      column_name LIKE '%_minor'
      OR column_name LIKE '%_at'
      OR column_name = 'organization_id'
      OR column_name = 'version'
  )
ORDER BY table_name, ordinal_position;

SELECT conrelid::regclass AS table_name,
       conname,
       contype,
       pg_get_constraintdef(oid) AS definition
FROM pg_constraint
WHERE connamespace = 'zaiko'::regnamespace
ORDER BY conrelid::regclass::text, conname;

SELECT schemaname, tablename, indexname, indexdef
FROM pg_indexes
WHERE schemaname = 'zaiko'
ORDER BY tablename, indexname;

SELECT event_object_table, trigger_name, action_timing, event_manipulation
FROM information_schema.triggers
WHERE trigger_schema = 'zaiko'
ORDER BY event_object_table, trigger_name, event_manipulation;

SELECT table_name
FROM information_schema.tables t
WHERE t.table_schema = 'zaiko'
  AND t.table_type = 'BASE TABLE'
  AND t.table_name NOT IN (
      'schema_migrations',
      'organizations',
      'permissions',
      'roles',
      'role_permissions',
      'login_csrf_tokens'
  )
  AND NOT EXISTS (
      SELECT 1
      FROM information_schema.columns c
      WHERE c.table_schema = t.table_schema
        AND c.table_name = t.table_name
        AND c.column_name = 'organization_id'
  )
ORDER BY table_name;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.tables t
        WHERE t.table_schema = 'zaiko'
          AND t.table_type = 'BASE TABLE'
          AND t.table_name NOT IN (
              'schema_migrations',
              'organizations',
              'permissions',
              'roles',
              'role_permissions',
              'login_csrf_tokens'
          )
          AND NOT EXISTS (
              SELECT 1
              FROM information_schema.columns c
              WHERE c.table_schema = t.table_schema
                AND c.table_name = t.table_name
                AND c.column_name = 'organization_id'
          )
    ) THEN
        RAISE EXCEPTION 'tenant-owned table is missing organization_id';
    END IF;
END
$$;

SELECT table_name, column_name, data_type
FROM information_schema.columns
WHERE table_schema = 'zaiko'
  AND column_name LIKE '%_minor'
  AND data_type <> 'bigint'
ORDER BY table_name, column_name;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'zaiko'
          AND column_name LIKE '%_minor'
          AND data_type <> 'bigint'
    ) THEN
        RAISE EXCEPTION 'money minor-unit column is not bigint';
    END IF;
END
$$;
