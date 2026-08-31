/**
 * consignment.js — 委託登録
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
  renderRegisteredConsignmentSlips();
}

/** 委託登録ページ下部に、登録済み委託伝票を一覧表示する。 */
function renderRegisteredConsignmentSlips() {
  const tbody = document.getElementById('registered-consignment-list-body');
  const empty = document.getElementById('registered-consignment-list-empty');
  const count = document.getElementById('registered-consignment-list-count');
  if (!tbody || !empty) return;

  const records = [...(APP_DATA.consignments || [])].sort((a, b) => {
    const dateDiff = String(b.date || '').localeCompare(String(a.date || ''));
    return dateDiff || String(b.id || '').localeCompare(String(a.id || ''));
  });
  if (count) count.textContent = `${records.length}伝票`;
  empty.style.display = records.length ? 'none' : '';
  tbody.innerHTML = records.map(row => {
    const id = _escHtml(row.id || '');
    const note = _escHtml(row.note || '—');
    const statusBadge = _slipStatusBadge(getConsignmentProcessingStatus(row), row.id, 'consignment');
    const totalJpy = Number(row.totalJpy) || getShippingSaleTotalJPY(row.items || [], row);
    return `<tr class="slip-list-row" onclick="openSlipDetail('consignment','${id}')">
      <td><code style="font-size:12px;font-weight:bold;white-space:nowrap;">${id || '—'}</code></td>
      <td style="white-space:nowrap;">${_escHtml(row.date || '—')}</td>
      <td style="white-space:nowrap;">${_escHtml(getBuyerName(row.destination))}</td>
      <td style="text-align:center;white-space:nowrap;">${(row.items || []).length}点</td>
      <td style="text-align:right;font-weight:bold;color:var(--primary);white-space:nowrap;">${formatPrice(totalJpy)}</td>
      <td style="max-width:180px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;" title="${note}">${note}</td>
      <td style="text-align:center;white-space:nowrap;">${statusBadge}</td>
      <td style="text-align:center;white-space:nowrap;" onclick="event.stopPropagation()">
        <button type="button" class="btn btn-outline btn-sm" onclick="openSlipDetail('consignment','${id}')">
          <i class="fa-solid fa-magnifying-glass"></i> 詳細
        </button>
      </td>
      <td style="text-align:center;white-space:nowrap;" onclick="event.stopPropagation()">
        <button type="button" class="btn btn-primary btn-sm" onclick="issueConsignmentSlipDocument('${id}',event)" ${canIssuePurchaseSlip() ? '' : 'disabled'}>
          <i class="fa-solid fa-file-arrow-down"></i> ${row.issuedAt ? '再発行' : '発行'}
        </button>
      </td>
      <td style="text-align:center;white-space:nowrap;">${formatIssuedAtStacked(row.issuedAt)}</td>
    </tr>`;
  }).join('');
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
  renderRegisteredConsignmentSlips();
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
        placeholder="例：2908260001" oninput="onConsignmentCodeInput(this, ${id})"
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
    const salePriceUsd = Number(item.salePrice) || 0;
    const rate = typeof getSalesUsdRate === 'function' ? getSalesUsdRate() : 155.25;
    items.push({
      code: item.code, brand: item.brand || '', model: item.model || '',
      salePrice: salePriceUsd, salePriceUsd,
      convertedSalePriceJpy: typeof convertShippingUSDToJPY === 'function'
        ? convertShippingUSDToJPY(salePriceUsd, rate) : Math.ceil(salePriceUsd * rate / 1000) * 1000,
    });
  });
  return { items, unavailable, duplicates };
}

async function saveConsignment() {
  if (!requireAdminForSensitiveOperation('委託伝票の登録')) return;
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
    registeredAt: new Date().toISOString(), revisions: [], displayCurrency: 'JPY', inputCurrency: 'JPY',
    usdJpyRate: typeof getSalesUsdRate === 'function' ? getSalesUsdRate() : 155.25,
    totalJpy: items.reduce((sum, item) => sum + (Number(item.convertedSalePriceJpy) || 0), 0),
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
        status: '処理済', apiManaged: true, displayCurrency: 'JPY', inputCurrency: 'JPY',
        fxRateScaled: Number(saved.fxRateScaled) || 0, fxScale: Number(saved.fxScale) || 0,
        usdJpyRate: Number(saved.fxRateScaled) > 0 && Number(saved.fxScale) > 0 ? Number(saved.fxRateScaled) / Number(saved.fxScale) : record.usdJpyRate,
        issuedAt: saved.issuedAt || null, issuedBy: saved.issuedBy || '',
        items: (saved.lines || []).map(line => ({
          code: line.productCode, brand: line.brand || '', model: line.modelNumber || '',
          salePrice: Number(line.salePriceUsdMinor) || 0,
          salePriceUsd: Number(line.salePriceUsdMinor) || 0,
          convertedSalePriceJpy: Number(line.convertedSalePriceJpy) || 0,
        })),
      };
      record.totalJpy = record.items.reduce((sum, item) => sum + item.convertedSalePriceJpy, 0);
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
    renderRegisteredConsignmentSlips();
  } catch (error) {
    showToast('error', '委託登録エラー', error.message || '委託伝票を登録できませんでした');
  }
}

window.init_consignment = init_consignment;
window.renderRegisteredConsignmentSlips = renderRegisteredConsignmentSlips;
window.addConsignmentLine = addConsignmentLine;
window.addConsignmentItemByCode = addConsignmentItemByCode;
window.onConsignmentCodeInput = onConsignmentCodeInput;
window.removeConsignmentLine = removeConsignmentLine;
window.resetConsignmentForm = resetConsignmentForm;
window.saveConsignment = saveConsignment;
