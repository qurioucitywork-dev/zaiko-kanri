UPDATE staff_profiles
SET staff_code = 'STF-' || substring(staff_code FROM '([0-9]+)$'), updated_at = NOW()
WHERE staff_code ~ '^BUY-[0-9]+$';

UPDATE materials
SET code = 'M' || lpad(substring(code FROM '([0-9]+)$'), 2, '0'), updated_at = NOW()
WHERE code ~ '^MAT-[0-9]+$';

UPDATE movements
SET code = 'D' || lpad(substring(code FROM '([0-9]+)$'), 2, '0'), updated_at = NOW()
WHERE code ~ '^MOV-[0-9]+$';

UPDATE product_shapes
SET code = 'SHP-' || lpad(substring(code FROM '([0-9]+)$'), 3, '0'), updated_at = NOW()
WHERE code ~ '^TYP-[0-9]+$';

UPDATE product_conditions
SET code = 'C' || lpad(substring(code FROM '([0-9]+)$'), 2, '0'), updated_at = NOW()
WHERE code ~ '^CON-[0-9]+$';
