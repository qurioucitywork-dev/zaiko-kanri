/**
 * consignment.js — 委託伝票登録
 * 委託登録と同時に商品を「委託中」へ移し、在庫一覧・棚卸・伝票一覧へ反映する。
 */
'use strict';

let consignmentLineCount = 0;

function _consignmentToday() {
  const now = new Date();
  const offset = now.getTimezoneOffset() * 60000;
  return new Date(now.getTime() - offset).toISOString().slice(0, 10);
}

function _nextConsignmentNumber() {
  const year = new Date().getFullYear();
  const max = (APP_DATA.consignments || []).reduce((current, record) => {
    const match = String(record.id || '').match(/^CO-(\d{4})-(\d+)$/);
    return match && Number(match[1]) === year ? Math.max(current, Number(match[2])) : current;
  }, 0);
  return `CO-${year}-${String(max + 1).padStart(4, '0')}`;
}

function init_consignment() {
  if (!Array.isArray(APP_DATA.consignments)) APP_DATA.consignments = [];
  if (!document.querySelector('#consignmentLines .consignment-line')) resetConsignmentForm(false);
  if (typeof populateBuyerMasterSelect === 'function') {
    populateBuyerMasterSelect('co-dest', {
      emptyLabel: '-- 選択 --', selected: document.getElementById('co-dest')?.value || '', labelMode: 'code-name',
    });
  }
}

function resetConsignmentForm(notify = false) {
  const id = document.getElementById('co-id');
  const date = document.getElementById('co-date');
  const dest = document.getElementById('co-dest');
  const note = document.getElementById('co-note');
  const lines = document.getElementById('consignmentLines');
  if (id) id.value = _nextConsignmentNumber();
  if (date) date.value = _consignmentToday();
  if (dest) dest.value = '';
  if (note) note.value = '';
  if (lines) lines.innerHTML = '';
  consignmentLineCount = 0;
  addConsignmentLine();
  if (typeof populateBuyerMasterSelect === 'function') {
    populateBuyerMasterSelect('co-dest', { emptyLabel: '-- 選択 --', selected: '', labelMode: 'code-name' });
  }
  if (notify && typeof showToast === 'function') showToast('info', 'リセット', '委託伝票の入力内容をクリアしました');
}

function addConsignmentLine() {
  const container = document.getElementById('consignmentLines');
  if (!container) return null;
  consignmentLineCount += 1;
  const id = consignmentLineCount;
  const row = document.createElement('div');
  row.className = 'consignment-line';
  row.dataset.lineId = String(id);
  row.innerHTML = `
    <div>
      <input class="form-control" id="co-code-${id}" aria-label="商品管理番号" autocomplete="off"
        placeholder="例：20260815001" oninput="onConsignmentCodeInput(this, ${id})"
        onkeydown="if(event.key==='Enter'){event.preventDefault();onConsignmentCodeInput(this, ${id}, true);}">
    </div>
    <div class="consignment-readonly-cell" id="co-brand-${id}">—</div>
    <div class="consignment-readonly-cell" id="co-model-${id}">—</div>
    <div>
      <button type="button" class="btn btn-ghost btn-sm" style="color:var(--danger);"
        onclick="removeConsignmentLine(this)" aria-label="この委託明細行を削除">
        <i class="fa-solid fa-xmark"></i>
      </button>
    </div>`;
  container.appendChild(row);
  return row;
}

function removeConsignmentLine(button) {
  button?.closest('.consignment-line')?.remove();
  ensureConsignmentTrailingBlankLine();
}

function _consignmentRows() {
  return Array.from(document.querySelectorAll('#consignmentLines .consignment-line'));
}

function _consignmentCode(row) {
  const id = row?.dataset?.lineId;
  return id ? String(document.getElementById(`co-code-${id}`)?.value || '').trim() : '';
}

function _consignableInventoryItem(rawCode) {
  const code = String(rawCode || '').trim().toUpperCase();
  if (!code) return null;
  return (APP_DATA.inventory || []).find(item => String(item.code || '').trim().toUpperCase() === code) || null;
}

function ensureConsignmentTrailingBlankLine() {
  let rows = _consignmentRows();
  if (rows.length === 0 || _consignmentCode(rows.at(-1))) {
    addConsignmentLine();
    rows = _consignmentRows();
  }
  return rows.at(-1) || null;
}

function onConsignmentCodeInput(input, lineId, moveFocus = false) {
  const item = _consignableInventoryItem(input?.value);
  const brand = document.getElementById(`co-brand-${lineId}`);
  const model = document.getElementById(`co-model-${lineId}`);
  if (!item) {
    if (brand) brand.textContent = '—';
    if (model) model.textContent = '—';
    return false;
  }
  input.value = item.code;
  if (brand) brand.textContent = item.brand || '—';
  if (model) model.textContent = item.model || '—';
  const trailing = ensureConsignmentTrailingBlankLine();
  if (moveFocus && trailing) document.getElementById(`co-code-${trailing.dataset.lineId}`)?.focus();
  return true;
}

function addConsignmentItemByCode(rawCode, { notify = true, focusNext = true } = {}) {
  const item = _consignableInventoryItem(rawCode);
  if (!item || item.status !== '在庫中') {
    if (notify && typeof showToast === 'function') {
      showToast('error', '読み取りできません', `商品管理番号「${String(rawCode || '').trim()}」は未登録または委託できない状態です`);
    }
    return false;
  }
  if (_consignmentRows().some(row => _consignmentCode(row).toUpperCase() === item.code.toUpperCase())) {
    if (notify && typeof showToast === 'function') showToast('warning', '重複しています', `${item.code} はすでに委託明細へ追加されています`);
    return false;
  }
  let row = _consignmentRows().find(candidate => !_consignmentCode(candidate));
  if (!row) row = addConsignmentLine();
  const id = row?.dataset?.lineId;
  const input = id ? document.getElementById(`co-code-${id}`) : null;
  if (!input) return false;
  input.value = item.code;
  onConsignmentCodeInput(input, id, false);
  const trailing = ensureConsignmentTrailingBlankLine();
  if (focusNext && trailing) document.getElementById(`co-code-${trailing.dataset.lineId}`)?.focus();
  return true;
}

function _collectConsignmentItems() {
  const items = [];
  const unavailable = [];
  const duplicates = [];
  const seen = new Set();
  _consignmentRows().forEach(row => {
    const code = _consignmentCode(row);
    if (!code) return;
    const item = _consignableInventoryItem(code);
    const normalized = String(item?.code || code).toUpperCase();
    if (seen.has(normalized)) {
      duplicates.push(item?.code || code);
      return;
    }
    seen.add(normalized);
    if (!item || item.status !== '在庫中') {
      unavailable.push(`${item?.code || code}（${item?.status || '未登録'}）`);
      return;
    }
    items.push({ code: item.code, brand: item.brand || '', model: item.model || '' });
  });
  return { items, unavailable, duplicates };
}

async function saveConsignment() {
  const date = document.getElementById('co-date')?.value || '';
  const destination = document.getElementById('co-dest')?.value || '';
  const note = document.getElementById('co-note')?.value || '';
  const { items, unavailable, duplicates } = _collectConsignmentItems();
  if (!date || !destination) {
    showToast('error', '入力エラー', '委託日と委託先を選択してください');
    return;
  }
  if (duplicates.length) {
    showToast('error', '商品管理番号が重複しています', duplicates.join('、'));
    return;
  }
  if (unavailable.length) {
    showToast('error', '委託できない商品があります', unavailable.join('、'));
    return;
  }
  if (!items.length) {
    showToast('error', '入力エラー', '商品管理番号を1件以上入力してください');
    return;
  }

  let record = {
    id: document.getElementById('co-id')?.value || _nextConsignmentNumber(),
    date, destination, consignee: destination, note, status: '処理済', items,
    registeredAt: new Date().toISOString(), revisions: [],
  };
  try {
    if (window.ZaikoAPI?.saveConsignment) {
      const saved = await window.ZaikoAPI.saveConsignment({
        consigneeCode: destination,
        consignmentDate: date,
        notes: note,
        productCodes: items.map(item => item.code),
      });
      record = {
        ...record, _id: saved.id, id: saved.slipNumber, date: saved.consignmentDate,
        status: '処理済', apiManaged: true,
        items: (saved.lines || []).map(line => ({
          code: line.productCode, brand: line.brand || '', model: line.modelNumber || '',
        })),
      };
    } else {
      if (!Array.isArray(APP_DATA.consignments)) APP_DATA.consignments = [];
      APP_DATA.consignments.push(record);
      items.forEach(line => {
        const inventory = (APP_DATA.inventory || []).find(item => item.code === line.code);
        if (inventory) inventory.status = '委託中';
      });
      if (typeof refreshLinkedBusinessViews === 'function') refreshLinkedBusinessViews({ source: 'consignment-register' });
    }
    showToast('success', '委託登録完了', `${record.id} / ${items.length}点を委託中へ更新しました`);
    resetConsignmentForm(false);
  } catch (error) {
    showToast('error', '委託登録エラー', error.message || '委託伝票を登録できませんでした');
  }
}

window.init_consignment = init_consignment;
window.addConsignmentLine = addConsignmentLine;
window.addConsignmentItemByCode = addConsignmentItemByCode;
window.onConsignmentCodeInput = onConsignmentCodeInput;
window.removeConsignmentLine = removeConsignmentLine;
window.resetConsignmentForm = resetConsignmentForm;
window.saveConsignment = saveConsignment;
