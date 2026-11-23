-- Preserve document IDs and retain an old/new number map for traceability.
CREATE TABLE daily_document_sequences (
 organization_id TEXT NOT NULL REFERENCES organizations(id), prefix TEXT NOT NULL,
 business_date DATE NOT NULL, last_sequence INTEGER NOT NULL CHECK(last_sequence BETWEEN 1 AND 999),
 updated_at TIMESTAMPTZ NOT NULL, PRIMARY KEY(organization_id,prefix,business_date)
);
CREATE TABLE document_number_aliases (
 organization_id TEXT NOT NULL REFERENCES organizations(id), document_table TEXT NOT NULL,
 document_id TEXT NOT NULL, old_number TEXT NOT NULL, new_number TEXT NOT NULL,
 business_date DATE NOT NULL, prefix TEXT NOT NULL, sequence INTEGER NOT NULL,
 changed_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY(organization_id,document_table,document_id)
);
LOCK TABLE purchase_slips,sales_slips,shipment_slips,consignment_slips,return_slips,cost_adjustments,sales_adjustment_slips IN SHARE ROW EXCLUSIVE MODE;
CREATE TEMP TABLE document_renumber_plan ON COMMIT DROP AS
WITH documents AS (
SELECT organization_id,'purchase_slips'::TEXT AS document_table,id AS document_id,slip_number AS old_number,purchase_date::date AS business_date,substring(slip_number from '^([A-Z]+)-') AS prefix,created_at FROM purchase_slips
UNION ALL
SELECT organization_id,'sales_slips'::TEXT AS document_table,id AS document_id,slip_number AS old_number,sale_date::date AS business_date,substring(slip_number from '^([A-Z]+)-') AS prefix,created_at FROM sales_slips
UNION ALL
SELECT organization_id,'shipment_slips'::TEXT AS document_table,id AS document_id,slip_number AS old_number,shipment_date::date AS business_date,substring(slip_number from '^([A-Z]+)-') AS prefix,created_at FROM shipment_slips
UNION ALL
SELECT organization_id,'consignment_slips'::TEXT AS document_table,id AS document_id,slip_number AS old_number,consignment_date::date AS business_date,substring(slip_number from '^([A-Z]+)-') AS prefix,created_at FROM consignment_slips
UNION ALL
SELECT organization_id,'return_slips'::TEXT AS document_table,id AS document_id,slip_number AS old_number,transaction_date::date AS business_date,substring(slip_number from '^([A-Z]+)-') AS prefix,created_at FROM return_slips
UNION ALL
SELECT organization_id,'cost_adjustments'::TEXT AS document_table,id AS document_id,slip_number AS old_number,adjustment_date::date AS business_date,substring(slip_number from '^([A-Z]+)-') AS prefix,created_at FROM cost_adjustments
UNION ALL
SELECT organization_id,'sales_adjustment_slips'::TEXT AS document_table,id AS document_id,slip_number AS old_number,adjustment_date::date AS business_date,substring(slip_number from '^([A-Z]+)-') AS prefix,created_at FROM sales_adjustment_slips
), numbered AS (
 SELECT *,ROW_NUMBER() OVER(PARTITION BY organization_id,prefix,business_date ORDER BY created_at,old_number,document_id)::INTEGER AS sequence FROM documents
)
SELECT *,prefix||'-'||to_char(business_date,'DDMMYY')||lpad(sequence::TEXT,3,'0') AS new_number FROM numbered;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM document_renumber_plan WHERE sequence>999 OR prefix IS NULL OR business_date IS NULL) THEN
  RAISE EXCEPTION 'Document renumbering requires valid dates/prefixes and at most 999 documents per day/type';
 END IF;
END $$;
INSERT INTO document_number_aliases(organization_id,document_table,document_id,old_number,new_number,business_date,prefix,sequence)
SELECT organization_id,document_table,document_id,old_number,new_number,business_date,prefix,sequence FROM document_renumber_plan;
UPDATE purchase_slips d SET slip_number='__renumber__'||d.id FROM document_renumber_plan m WHERE m.document_table='purchase_slips' AND m.organization_id=d.organization_id AND m.document_id=d.id;
UPDATE sales_slips d SET slip_number='__renumber__'||d.id FROM document_renumber_plan m WHERE m.document_table='sales_slips' AND m.organization_id=d.organization_id AND m.document_id=d.id;
UPDATE shipment_slips d SET slip_number='__renumber__'||d.id FROM document_renumber_plan m WHERE m.document_table='shipment_slips' AND m.organization_id=d.organization_id AND m.document_id=d.id;
UPDATE consignment_slips d SET slip_number='__renumber__'||d.id FROM document_renumber_plan m WHERE m.document_table='consignment_slips' AND m.organization_id=d.organization_id AND m.document_id=d.id;
UPDATE return_slips d SET slip_number='__renumber__'||d.id FROM document_renumber_plan m WHERE m.document_table='return_slips' AND m.organization_id=d.organization_id AND m.document_id=d.id;
UPDATE cost_adjustments d SET slip_number='__renumber__'||d.id FROM document_renumber_plan m WHERE m.document_table='cost_adjustments' AND m.organization_id=d.organization_id AND m.document_id=d.id;
UPDATE sales_adjustment_slips d SET slip_number='__renumber__'||d.id FROM document_renumber_plan m WHERE m.document_table='sales_adjustment_slips' AND m.organization_id=d.organization_id AND m.document_id=d.id;
UPDATE purchase_slips d SET slip_number=m.new_number FROM document_renumber_plan m WHERE m.document_table='purchase_slips' AND m.organization_id=d.organization_id AND m.document_id=d.id;
UPDATE sales_slips d SET slip_number=m.new_number FROM document_renumber_plan m WHERE m.document_table='sales_slips' AND m.organization_id=d.organization_id AND m.document_id=d.id;
UPDATE shipment_slips d SET slip_number=m.new_number FROM document_renumber_plan m WHERE m.document_table='shipment_slips' AND m.organization_id=d.organization_id AND m.document_id=d.id;
UPDATE consignment_slips d SET slip_number=m.new_number FROM document_renumber_plan m WHERE m.document_table='consignment_slips' AND m.organization_id=d.organization_id AND m.document_id=d.id;
UPDATE return_slips d SET slip_number=m.new_number FROM document_renumber_plan m WHERE m.document_table='return_slips' AND m.organization_id=d.organization_id AND m.document_id=d.id;
UPDATE cost_adjustments d SET slip_number=m.new_number FROM document_renumber_plan m WHERE m.document_table='cost_adjustments' AND m.organization_id=d.organization_id AND m.document_id=d.id;
UPDATE sales_adjustment_slips d SET slip_number=m.new_number FROM document_renumber_plan m WHERE m.document_table='sales_adjustment_slips' AND m.organization_id=d.organization_id AND m.document_id=d.id;
INSERT INTO daily_document_sequences(organization_id,prefix,business_date,last_sequence,updated_at)
 SELECT organization_id,prefix,business_date,MAX(sequence),now() FROM document_renumber_plan GROUP BY organization_id,prefix,business_date;

-- Operational notes and pending workflow snapshots follow the renamed slips.
-- Issuance PDFs and audit entries retain their original historical contents.
DO $$
DECLARE c RECORD; m RECORD; pattern TEXT;
BEGIN
 FOR c IN SELECT col.table_name,col.column_name,col.data_type FROM information_schema.columns col
 WHERE col.table_schema='public'
 AND col.table_name IN ('products','parts','purchase_slips','sales_slips','shipment_slips','consignment_slips','return_slips','cost_adjustments','sales_adjustment_slips','product_cost_merges','approval_requests','notifications','cost_adjustment_start_snapshots')
 AND col.column_name IN ('notes','internal_comment','reason','cancel_reason','decision_note','snapshot_json','inputs_json','title','body','previous_internal_comment')
 AND col.data_type IN ('text','jsonb') LOOP
  FOR m IN SELECT * FROM document_renumber_plan WHERE old_number<>new_number LOOP
   pattern := '(?<![A-Za-z0-9_-])'||m.old_number||'(?![A-Za-z0-9_-])';
   EXECUTE format('UPDATE %I SET %I=regexp_replace(%I::text,$1,$2,''g'')%s WHERE organization_id=$3 AND %I::text ~ $1',
    c.table_name,c.column_name,c.column_name,CASE WHEN c.data_type='jsonb' THEN '::jsonb' ELSE '' END,c.column_name)
   USING pattern, m.new_number,m.organization_id;
  END LOOP;
 END LOOP;
END $$;
UPDATE stocktake_lines l SET document_number=m.new_number
FROM stocktake_sessions session,document_renumber_plan m
WHERE l.session_id=session.id AND m.organization_id=session.organization_id AND l.document_id=m.document_id AND l.document_number=m.old_number;

-- Line notes belong to the parent document organization.
DO $$ DECLARE m RECORD; pattern TEXT; BEGIN
 FOR m IN SELECT * FROM document_number_aliases WHERE old_number<>new_number LOOP
  pattern := '(?<![A-Za-z0-9_-])'||m.old_number||'(?![A-Za-z0-9_-])';
  UPDATE purchase_slip_lines l SET notes=regexp_replace(l.notes,pattern,m.new_number,'g')
   FROM purchase_slips d WHERE l.purchase_slip_id=d.id AND d.organization_id=m.organization_id AND l.notes ~ pattern;
  UPDATE direct_cost_adjustment_lines l SET notes=regexp_replace(l.notes,pattern,m.new_number,'g')
   FROM cost_adjustments d WHERE l.cost_adjustment_id=d.id AND d.organization_id=m.organization_id AND l.notes ~ pattern;
 END LOOP;
END $$;
