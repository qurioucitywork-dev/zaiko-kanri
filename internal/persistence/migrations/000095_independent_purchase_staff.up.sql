-- Preserve existing buyer IDs/codes and document references. Legacy user_id is
-- retained only for historical compatibility; login management no longer uses it.
ALTER TABLE staff_profiles ALTER COLUMN user_id DROP NOT NULL;
ALTER TABLE staff_profiles ADD COLUMN display_name TEXT NOT NULL DEFAULT '';
ALTER TABLE staff_profiles ADD COLUMN is_active BOOLEAN NOT NULL DEFAULT TRUE;
UPDATE staff_profiles sp SET display_name=u.display_name, is_active=u.is_active
FROM users u WHERE u.id=sp.user_id AND u.organization_id=sp.organization_id;
