// Product-specific numeric values, keyed by accessory master code.
function accessorySupplementRecord(value) {
  const name = APP_DATA.accessoryAliases?.[value] || value;
  return allAccessoryMasterRecords().find(record => record.code === name || record.name === name);
}
function accessorySupplementText(value, values = {}) {
  const record = accessorySupplementRecord(value);
  return String(values?.[record?.code] ?? '').trim();
}
function renderAccessoryNameWithSupplement(value, values = {}) {
  const record = accessorySupplementRecord(value);
  const suffix = accessorySupplementText(value, values);
  return _mEsc(masterDisplayName('accessory', value, 'detail')) + (suffix ? ' <span class="accessory-value-display">' + _mEsc(suffix) + '</span>' : '');
}
function setAccessorySupplementInputs(groupId, values = {}) {
  const group = document.getElementById(groupId);
  if (!group) return;
  group._accessorySupplementValues = { ...values };
  group._accessorySupplementSignature = null;
  group._accessoryLayoutValuesPending = true;
  refreshAccessorySupplementInputs(group);
}
function readAccessorySupplementInputs(groupId, validate = true) {
  const group = document.getElementById(groupId);
  if (!group) return {};
  const result = {};
  const selected = [...group.querySelectorAll('input[type="checkbox"]:checked')];
  const wrap = document.getElementById(groupId + '-supplement-fields');
  for (const checkbox of selected) {
    const record = accessorySupplementRecord(checkbox.value);
    if (!record?.supplement) continue;
    const fields = [...group.querySelectorAll('[data-accessory-code]'), ...(wrap?.querySelectorAll('[data-accessory-code]') || [])].filter(input => input.dataset.accessoryCode === record.code);
    if (!fields.length) { if (group._accessorySupplementValues?.[record.code]) result[record.code] = group._accessorySupplementValues[record.code]; continue; }
    const any = fields.some(input => input.value !== '' || input.validity.badInput);
    for (const input of fields) {
      input.setCustomValidity(any && input.value === '' ? '数字を入力してください。' : '');
      if (validate && !input.reportValidity()) return null;
    }
    if (any) result[record.code] = record.supplement === 'フリーワード' ? fields[0].value.trim() : fields.map(input => input.value === '' ? '' : String(Number(input.value))).join('/');
  }
  return result;
}
function refreshAccessorySupplementInputs(group) {
  if (!['pu-accessories','ie-accessories','pep-accessories','ca-edit-product-accessories'].includes(group?.id) && !/^returned-accessories-\d+$/.test(group?.id||'')) return;
  if (typeof ensureAccessoryEntryLayout === 'function') ensureAccessoryEntryLayout(group);
  const selected = [...group.querySelectorAll('input[type="checkbox"]:checked')].map(checkbox => ({checkbox,record:accessorySupplementRecord(checkbox.value)})).filter(item => item.record?.supplement && !(typeof accessoryEntryOwnsCode === 'function' && accessoryEntryOwnsCode(group, item.record.code)));
  const signature = JSON.stringify(selected.map(({checkbox,record}) => [record.code,record.name,record.supplement,checkbox.disabled]));
  if (group._accessorySupplementSignature === signature) return;
  group._accessorySupplementSignature = signature;
  let wrap = document.getElementById(group.id + '-supplement-fields');
  if (!wrap) {wrap = document.createElement('div');wrap.id = group.id + '-supplement-fields';wrap.className = 'accessory-supplement-fields';group.after(wrap);}
  const values = group._accessorySupplementValues || {};
  wrap.innerHTML = selected.map(({checkbox,record}) => {
    const freeText = record.supplement === 'フリーワード';
    const yearMonth = record.supplement.includes('年') && record.supplement.includes('月');
    const labels = yearMonth ? ['年','月'] : [record.supplement];
    const parts = freeText ? [String(values[record.code] || '')] : String(values[record.code] || '').split('/');
    return '<div class="accessory-supplement-entry"><span>' + _mEsc(record.name) + '</span>' + labels.map((label,index) => {
      const month = label.includes('月'),year = label === '年';
      if (freeText) return '<label>' + _mEsc(label) + '<input type="text" maxlength="200" data-accessory-code="' + _mEsc(record.code) + '" aria-label="' + _mEsc(record.name + ' ' + label) + '" value="' + _mEsc(parts[0] || '') + '"' + (checkbox.disabled ? ' disabled' : '') + '></label>';
      return '<label>' + _mEsc(label) + '<input type="number" inputmode="numeric" step="1" min="' + (month || year ? 1 : 0) + '" max="' + (month ? 12 : year ? 9999 : 999999999) + '" data-accessory-code="' + _mEsc(record.code) + '" aria-label="' + _mEsc(record.name + ' ' + label) + '" value="' + _mEsc(parts[index] || '') + '"' + (checkbox.disabled ? ' disabled' : '') + '></label>';
    }).join('') + '</div>';
  }).join('');
  wrap.hidden = selected.length === 0;
  if (!group._accessorySupplementBound) {
    group._accessorySupplementBound = true;
    group.addEventListener('change', () => {
      group._accessorySupplementValues = readAccessorySupplementInputs(group.id,false) || {};
      refreshAccessorySupplementInputs(group);
    });
    wrap.addEventListener('input', () => {group._accessorySupplementValues = readAccessorySupplementInputs(group.id,false) || {};});
  }
}
