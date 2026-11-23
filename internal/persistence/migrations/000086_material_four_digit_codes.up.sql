-- Keep the material IDs and references unchanged; normalize numeric codes only.
DO $$
BEGIN
 IF EXISTS (
  SELECT 1 FROM materials WHERE code ~ '^[0-9]{1,4}$'
  GROUP BY organization_id, lpad(code,4,'0') HAVING count(*) > 1
 ) THEN
  RAISE EXCEPTION 'Cannot pad material codes: duplicate numeric codes exist';
 END IF;
END $$;
UPDATE materials SET code=lpad(code,4,'0'), updated_at=NOW()
WHERE code ~ '^[0-9]{1,3}$';
