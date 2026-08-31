ALTER TABLE business_partners
    ADD COLUMN IF NOT EXISTS contact_phone TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS antique_license_number TEXT NOT NULL DEFAULT '';

-- 旧サンプルの短い登録番号は T + 13桁へ正規化する。
UPDATE business_partners
SET invoice_number = 'T' || LPAD(regexp_replace(invoice_number, '[^0-9]', '', 'g'), 13, '0')
WHERE invoice_number <> '' AND invoice_number !~ '^T[0-9]{13}$';

CREATE INDEX IF NOT EXISTS idx_business_partners_contact_search
    ON business_partners (organization_id, phone, contact_phone, antique_license_number);
