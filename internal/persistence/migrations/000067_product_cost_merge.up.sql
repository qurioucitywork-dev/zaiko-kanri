CREATE TABLE product_cost_merges (
 id TEXT PRIMARY KEY,
 organization_id TEXT NOT NULL REFERENCES organizations(id),
 management_code TEXT NOT NULL,
 business_date DATE NOT NULL,
 inputs_json TEXT NOT NULL,
 status TEXT NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','confirmed')),
 adjustment_id TEXT REFERENCES cost_adjustments(id),
 created_by TEXT NOT NULL REFERENCES users(id),
 created_at TIMESTAMPTZ NOT NULL,
 UNIQUE(organization_id,management_code)
);
CREATE TABLE product_cost_merge_inputs (
 merge_id TEXT NOT NULL REFERENCES product_cost_merges(id),
 product_id TEXT NOT NULL REFERENCES products(id),
 PRIMARY KEY(merge_id,product_id),
 UNIQUE(product_id)
);
