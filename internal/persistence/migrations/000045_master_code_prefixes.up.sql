-- Standardize user-facing master codes while preserving stable row IDs and all ID-based references.
UPDATE staff_profiles
SET staff_code = 'BUY-' || substring(staff_code FROM '([0-9]+)$'), updated_at = NOW()
WHERE staff_code ~ '^STF-[0-9]+$';

UPDATE materials
SET code = 'MAT-' || lpad(substring(code FROM '([0-9]+)$'), 3, '0'), updated_at = NOW()
WHERE code ~ '^M[0-9]+$';

UPDATE movements
SET code = 'MOV-' || lpad(substring(code FROM '([0-9]+)$'), 3, '0'), updated_at = NOW()
WHERE code ~ '^D[0-9]+$';

UPDATE product_shapes
SET code = 'TYP-' || lpad(substring(code FROM '([0-9]+)$'), 3, '0'), updated_at = NOW()
WHERE code ~ '^SHP-[0-9]+$';

UPDATE product_conditions
SET code = 'CON-' || lpad(substring(code FROM '([0-9]+)$'), 3, '0'), updated_at = NOW()
WHERE code ~ '^C[0-9]+$';
