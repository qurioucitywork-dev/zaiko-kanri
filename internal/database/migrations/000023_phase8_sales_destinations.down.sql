UPDATE master_records SET record_code='SAL-001',name='店頭販売'
WHERE organization_id='org_preview' AND category='sales-destinations' AND record_code='B001';
UPDATE master_records SET record_code='SAL-002',name='業者販売'
WHERE organization_id='org_preview' AND category='sales-destinations' AND record_code='B002';
UPDATE master_records SET record_code='SAL-003',name='EC販売'
WHERE organization_id='org_preview' AND category='sales-destinations' AND record_code='B003';
UPDATE master_records SET record_code='SAL-004',name='オークション'
WHERE organization_id='org_preview' AND category='sales-destinations' AND record_code='B004';

UPDATE guest_companies SET company_code='GUEST-001',name='クロノス東京'
WHERE organization_id='org_preview' AND id='gco_preview_001';
UPDATE guest_companies SET company_code='GUEST-002',name='タイムレス商会'
WHERE organization_id='org_preview' AND id='gco_preview_002';
UPDATE guest_companies SET company_code='GUEST-003',name='ラグジュアリーアイランド'
WHERE organization_id='org_preview' AND id='gco_preview_003';
UPDATE guest_companies SET is_active=0
WHERE organization_id='org_preview' AND id='gco_preview_004';
