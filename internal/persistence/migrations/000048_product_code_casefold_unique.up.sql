CREATE UNIQUE INDEX IF NOT EXISTS ux_products_organization_product_code_normalized
    ON products (organization_id, UPPER(BTRIM(product_code)));
