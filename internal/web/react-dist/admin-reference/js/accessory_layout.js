// Shared accessory entry layout. Keep the original checkboxes and their events
// as the selection model used by registration, editing, and slip serializers.
const ACCESSORY_ENTRY_GROUPS = ['pu-accessories', 'ie-accessories', 'pep-accessories', 'ca-edit-product-accessories'];
const ACCESSORY_WARRANTY_CODES = ['0021', '0022', '0023', '0024', '0025', '0026'];
function accessoryEntryIsStructured(group) {
  return ACCESSORY_ENTRY_GROUPS.includes(group?.id) && ['0021', '0040', '0041'].every(code =>
    (APP_DATA.accessoryRecords || []).some(record => record.code === code));
}
function accessoryEntryOwnsCode(group, code) {
  return accessoryEntryIsStructured(group) && [...ACCESSORY_WARRANTY_CODES, '0040', '0041'].includes(code);
}
function ensureAccessoryEntryLayout(group) {
  if (!accessoryEntryIsStructured(group)) return false;
  const checkboxes = [...group.querySelectorAll('input[type="checkbox"]')];
  if (!checkboxes.length) return true;
  const entries = checkboxes.map(input => ({ input, record: accessorySupplementRecord(input.value), label: input.closest('label') }));
  const byCode = code => entries.find(entry => entry.record?.code === code);
  let layout = group.querySelector('.accessory-entry-layout');
  if (!layout) {
    const oldPicker = group._masterAccessoryPicker;
    oldPicker?._partnerSearchController?.wrapper?.remove();
    oldPicker?.remove();
    group._masterAccessoryPicker = null;
    layout = document.createElement('div');
    layout.className = 'accessory-entry-layout';
    const field = (label, control, cls = '') => '<label class="accessory-entry-field ' + cls + '"><span>' + label + '</span>' + control + '</label>';
    layout.innerHTML = '<div class="accessory-entry-top">' +
      field('保証書', '<select class="form-control" data-accessory-warranty aria-label="保証書"><option value="">選択なし</option>' + ACCESSORY_WARRANTY_CODES.map(code => {
        const record = byCode(code)?.record;
        return record ? '<option value="' + code + '">' + _mEsc(record.name) + '</option>' : '';
      }).join('') + '</select>') +
      '<fieldset class="accessory-entry-date"><legend>年月</legend><div>' +
      '<input class="form-control" type="number" inputmode="numeric" min="1" max="9999" step="1" data-accessory-year aria-label="保証書の年" placeholder="年"><span>年</span>' +
      '<input class="form-control" type="number" inputmode="numeric" min="1" max="12" step="1" data-accessory-month aria-label="保証書の月" placeholder="月"><span>月</span></div></fieldset>' +
      field('コマ', '<input class="form-control" type="number" inputmode="numeric" min="0" max="999999999" step="1" data-accessory-code="0040" aria-label="コマ数" placeholder="数値を入力">') + '</div>' +
      '<div class="accessory-entry-bottom"><div class="accessory-entry-checks" data-accessory-left></div><div class="accessory-entry-checks" data-accessory-right></div>' +
      field('その他', '<textarea class="form-control" rows="3" maxlength="200" data-accessory-code="0041" aria-label="その他の付属品" placeholder="自由入力"></textarea>', 'accessory-entry-other') + '</div>' +
      '<div class="accessory-entry-extra"></div><div class="accessory-entry-selection" hidden></div>';
    group.classList.add('accessory-entry-group');
    group.append(layout);
    const leftCodes = ['0001','0002','0003','0004','0005','0006'];
    const rightCodes = ['0007','0008','0009','0010','0011','0012'];
    entries.forEach(entry => {
      const code = entry.record?.code;
      let target = layout.querySelector('.accessory-entry-extra');
      if (leftCodes.includes(code)) target = layout.querySelector('[data-accessory-left]');
      else if (rightCodes.includes(code)) target = layout.querySelector('[data-accessory-right]');
      else if (accessoryEntryOwnsCode(group, code)) target = layout.querySelector('.accessory-entry-selection');
      if (entry.label) target.append(entry.label);
    });
    const warranty = layout.querySelector('[data-accessory-warranty]');
    warranty.addEventListener('change', () => {
      ACCESSORY_WARRANTY_CODES.forEach(code => {
        const input = byCode(code)?.input;
        if (input && !input.disabled) { input.checked = code === warranty.value; input.closest('label')?.classList.toggle('checked', input.checked); }
      });
      layout._warrantyCode = null;
      syncAccessoryEntryLayout(group, layout, entries, true);
      group.dispatchEvent(new Event('change', { bubbles: true }));
    });
    layout.addEventListener('input', event => {
      const input = event.target;
      const code = input.dataset.accessoryCode;
      if (['0040','0041'].includes(code)) {
        const checkbox = byCode(code)?.input;
        if (checkbox && !checkbox.disabled) {
          checkbox.checked = Boolean(input.value.trim()) || Boolean(input.validity.badInput);
          checkbox.closest('label')?.classList.toggle('checked', checkbox.checked);
        }
      }
      if (code) group._accessorySupplementValues = readAccessorySupplementInputs(group.id, false) || {};
    });
  }
  syncAccessoryEntryLayout(group, layout, entries, Boolean(group._accessoryLayoutValuesPending));
  group._accessoryLayoutValuesPending = false;
  return true;
}
function syncAccessoryEntryLayout(group, layout, entries, force = false) {
  const values = group._accessorySupplementValues || {};
  const warrantyEntries = entries.filter(entry => ACCESSORY_WARRANTY_CODES.includes(entry.record?.code));
  const selected = warrantyEntries.filter(entry => entry.input.checked);
  const code = selected[0]?.record.code || '';
  const select = layout.querySelector('[data-accessory-warranty]');
  if (select.value !== code) select.value = code;
  select.disabled = !warrantyEntries.some(entry => !entry.input.disabled);
  [...select.options].forEach(option => { if (option.value) option.disabled = Boolean(warrantyEntries.find(entry => entry.record.code === option.value)?.input.disabled); });
  const year = layout.querySelector('[data-accessory-year]');
  const month = layout.querySelector('[data-accessory-month]');
  if (layout._warrantyCode !== code || force) {
    const parts = String(values[code] || '').split('/');
    year.value = parts[0] || ''; month.value = parts[1] || '';
    year.dataset.accessoryCode = code; month.dataset.accessoryCode = code;
    year.setCustomValidity(''); month.setCustomValidity('');
    layout._warrantyCode = code;
  }
  year.disabled = month.disabled = !code || Boolean(selected[0]?.input.disabled);
  // Existing multiple selections are visible until the user chooses one warranty.
  warrantyEntries.forEach(entry => {
    const target = selected.length > 1 && entry.input.checked
      ? layout.querySelector('.accessory-entry-extra') : layout.querySelector('.accessory-entry-selection');
    if (entry.label?.parentElement !== target) target.append(entry.label);
  });
  for (const key of ['0040','0041']) {
    const entry = entries.find(item => item.record?.code === key);
    const field = layout.querySelector('[data-accessory-code="' + key + '"]');
    field.disabled = !entry || entry.input.disabled;
    if (!layout._valuesInitialized || force) field.value = entry?.input.checked ? String(values[key] ?? '') : '';
  }
  layout._valuesInitialized = true;
}
