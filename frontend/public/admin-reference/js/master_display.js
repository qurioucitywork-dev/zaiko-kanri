// Display labels are resolved at the output boundary; stored codes/names stay unchanged.
function masterDisplayRecord(type, value) {
  const keys = {staff:'staffRecords', belt:'beltMaterialRecords', accessory:'accessoryRecords', brand:'brandRecords', material:'materials', movement:'movements', shape:'shapeRecords', marking:'markingRecords', condition:'conditions'};
  const records = type === 'accessory' ? allAccessoryMasterRecords() : (APP_DATA[keys[type]] || []);
  const raw = String(value ?? '').trim();
  const text = type === 'accessory' ? (APP_DATA.accessoryAliases?.[raw] || raw) : raw;
  if (!text) return null;
  const fold = value => String(value ?? '').trim().toLocaleLowerCase('ja');
  return records.find(record => [record.code,record.id,record.name].some(v => v != null && fold(v) === fold(text)))
    || (/^\d{1,4}$/.test(text) ? records.find(record => /^\d{1,4}$/.test(String(record.code)) && Number(record.code) === Number(text)) : null);
}

function masterDisplayName(type, value, purpose = 'detail') {
  const record = masterDisplayRecord(type, value);
  const field = {detail:'name', inventory:'inventoryDetail', overseas:'overseasDetail', tag:'tagDetail', domestic:'domesticDetail'}[purpose] || 'name';
  // An empty accessory tag label is intentional (e.g. bracelet count only).
  // The API omits empty fields, so an existing record is sufficient.
  if (type === 'accessory' && purpose === 'tag' && record) return String(record.tagDetail || '').trim();
  // Catalog instructions such as （表記無し） are literal master values, as requested.
  return String(record?.[field] || record?.name || value || '').trim();
}

function accessoryDisplayNames(values, supplements = {}, purpose = 'detail', options = {}) {
  return (Array.isArray(values) ? values : []).map(value => {
    const record = masterDisplayRecord('accessory', value);
    // Purchase returns identify the RX box explicitly, regardless of destination.
    const useDetail = options.documentType === 'purchaseReturn' && (Number(record?.code) === 1 || record?.name === 'RX箱');
    const label = masterDisplayName('accessory', value, useDetail ? 'detail' : purpose);
    const suffix = String(supplements?.[record?.code] ?? '').trim();
    return [label, suffix].filter(Boolean).join(' ');
  }).filter(Boolean);
}

function documentDisplayPurpose(counterparty) {
  return counterparty?.regionType === 'overseas' ? 'overseas' : 'domestic';
}
