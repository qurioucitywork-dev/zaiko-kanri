DELETE FROM master_records
WHERE organization_id='org_preview'
  AND category='sales-destinations'
  AND id IN (
    'mst_preview_sales_destinations_001',
    'mst_preview_sales_destinations_002',
    'mst_preview_sales_destinations_003',
    'mst_preview_sales_destinations_004'
  );

INSERT INTO master_records(
  id,organization_id,category,record_code,name,is_active,
  created_by,created_at,updated_by,updated_at
)
SELECT 'mst_preview_sales_destinations_001',id,'sales-destinations','B001','ウォッチマート',1,
       NULL,strftime('%Y-%m-%dT%H:%M:%fZ','now'),NULL,strftime('%Y-%m-%dT%H:%M:%fZ','now')
FROM organizations WHERE id='org_preview';
INSERT INTO master_records(
  id,organization_id,category,record_code,name,is_active,
  created_by,created_at,updated_by,updated_at
)
SELECT 'mst_preview_sales_destinations_002',id,'sales-destinations','B002','タイムレス商会',1,
       NULL,strftime('%Y-%m-%dT%H:%M:%fZ','now'),NULL,strftime('%Y-%m-%dT%H:%M:%fZ','now')
FROM organizations WHERE id='org_preview';
INSERT INTO master_records(
  id,organization_id,category,record_code,name,is_active,
  created_by,created_at,updated_by,updated_at
)
SELECT 'mst_preview_sales_destinations_003',id,'sales-destinations','B003','ラグジュアリーアイランド',1,
       NULL,strftime('%Y-%m-%dT%H:%M:%fZ','now'),NULL,strftime('%Y-%m-%dT%H:%M:%fZ','now')
FROM organizations WHERE id='org_preview';
INSERT INTO master_records(
  id,organization_id,category,record_code,name,is_active,
  created_by,created_at,updated_by,updated_at
)
SELECT 'mst_preview_sales_destinations_004',id,'sales-destinations','B004','クロノス東京',1,
       NULL,strftime('%Y-%m-%dT%H:%M:%fZ','now'),NULL,strftime('%Y-%m-%dT%H:%M:%fZ','now')
FROM organizations WHERE id='org_preview';

UPDATE guest_companies
SET company_code='B004',name='クロノス東京',is_active=1,
    updated_by=NULL,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
WHERE organization_id='org_preview' AND id='gco_preview_001';

UPDATE guest_companies
SET company_code='B002',name='タイムレス商会',is_active=1,
    updated_by=NULL,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
WHERE organization_id='org_preview' AND id='gco_preview_002';

UPDATE guest_companies
SET company_code='B003',name='ラグジュアリーアイランド',is_active=1,
    updated_by=NULL,updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now')
WHERE organization_id='org_preview' AND id='gco_preview_003';

INSERT OR IGNORE INTO guest_companies(
  id,organization_id,company_code,name,is_active,
  created_by,created_at,updated_by,updated_at
)
SELECT 'gco_preview_004',id,'B001','ウォッチマート',1,
       NULL,strftime('%Y-%m-%dT%H:%M:%fZ','now'),
       NULL,strftime('%Y-%m-%dT%H:%M:%fZ','now')
FROM organizations WHERE id='org_preview';
