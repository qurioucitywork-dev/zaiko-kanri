// =====================================================
// 相場表
// 在庫一覧と同じ商品項目を使い、オークション開催日・オークション名・落札価格（JPY）を管理する。
// PostgreSQLの相場表データをREST API経由でAPP_DATAへ同期する。API未接続時のみ初期データを表示する。
// =====================================================

let marketPage = 1;
const MARKET_ITEMS_PER_PAGE = 10;
let _marketInitialized = false;
let _marketEditingId = null;
let _marketImportSequence = 0;
let _marketPendingImport = null;

const MARKET_PRICE_USD_SEED = [
  7900, 3350, 19500, 4800, 16500,
  5600, 2900, 4100, 14700, 4750,
];

const MARKET_COLUMN_KEYS = [
  'importDate', 'brand', 'model', 'ref', 'auctionName',
  'marketPrice', 'sku', 'accessories', 'note', 'edit',
];
const MARKET_COLUMN_WIDTHS = {
  importDate: 156, brand: 150, model: 160, ref: 150,
  auctionName: 180, marketPrice: 158, sku: 130, accessories: 180,
  note: 330, edit: 72,
};
const _marketVisibleColumns = new Set(MARKET_COLUMN_KEYS);

/** マスタ登録のUSドル円換算レートを返す */
function getMarketUsdRate() {
  const masterRate = Number((APP_DATA.fxRates || []).find(rate => rate.code === 'USD')?.rate);
  if (Number.isFinite(masterRate) && masterRate > 0) return masterRate;
  return Number(globalThis.SALE_PRICE_JPY_PER_USD) || 155;
}

/** 落札価格はJPY固定で表示する */
function formatMarketPrice(jpyAmount) {
  return formatPrice(Number(jpyAmount) || 0);
}

/** APIが日時を返した場合も、一覧ではオークション開催日の年月日だけを表示する。 */
function _marketDisplayDate(value) {
  const text = String(value || '').trim();
  if (!text) return '—';
  const match = text.match(/^\d{4}-\d{2}-\d{2}/u);
  return match ? match[0] : text;
}

function _marketSyncColumnPanelNote() {
  const note = document.getElementById('market-column-panel-note');
  if (note) note.textContent = 'チェックを外した項目は一覧から非表示になります / 落札価格はJPY固定です';
}

/** 表示項目設定をテーブル・チェックボックスへ反映する */
function _marketApplyColumnVisibility() {
  const table = document.getElementById('marketTable');
  if (table) {
    const visibleWidth = [..._marketVisibleColumns]
      .reduce((sum, key) => sum + (MARKET_COLUMN_WIDTHS[key] || 80), 12);
    table.style.minWidth = `${Math.max(320, visibleWidth)}px`;
    table.querySelectorAll('[data-market-col]').forEach(element => {
      element.classList.toggle('market-col-hidden', !_marketVisibleColumns.has(element.dataset.marketCol));
    });
    table.querySelectorAll('td[data-market-empty-row]').forEach(cell => {
      cell.colSpan = Math.max(1, _marketVisibleColumns.size);
    });
  }
  _marketSyncColumnVisibilityControls();
}

function _marketSyncColumnVisibilityControls() {
  document.querySelectorAll('#market-column-panel input[type="checkbox"]').forEach(checkbox => {
    checkbox.checked = _marketVisibleColumns.has(checkbox.value);
  });
  const count = document.getElementById('market-column-count');
  if (count) count.textContent = `${_marketVisibleColumns.size}/${MARKET_COLUMN_KEYS.length}`;
}

function marketColumnVisibilityChanged(checkbox) {
  const key = checkbox?.value;
  if (!MARKET_COLUMN_KEYS.includes(key)) return;
  if (checkbox.checked) {
    _marketVisibleColumns.add(key);
  } else {
    if (_marketVisibleColumns.size <= 1 && _marketVisibleColumns.has(key)) {
      checkbox.checked = true;
      showToast('info', '表示項目', '少なくとも1項目は表示してください');
      return;
    }
    _marketVisibleColumns.delete(key);
  }
  _marketApplyColumnVisibility();
}

function marketShowAllColumns() {
  MARKET_COLUMN_KEYS.forEach(key => _marketVisibleColumns.add(key));
  _marketApplyColumnVisibility();
}

function marketToggleColumnMenu(event) {
  event?.stopPropagation();
  const panel = document.getElementById('market-column-panel');
  const trigger = document.getElementById('market-column-trigger');
  if (!panel || !trigger) return;
  const shouldOpen = !panel.classList.contains('open');
  marketCloseColumnMenu();
  if (shouldOpen) {
    panel.classList.add('open');
    trigger.setAttribute('aria-expanded', 'true');
  }
}

function marketCloseColumnMenu() {
  const panel = document.getElementById('market-column-panel');
  const trigger = document.getElementById('market-column-trigger');
  if (panel) panel.classList.remove('open');
  if (trigger) trigger.setAttribute('aria-expanded', 'false');
}

function _marketToday() {
  const now = new Date();
  const local = new Date(now.getTime() - now.getTimezoneOffset() * 60000);
  return local.toISOString().slice(0, 10);
}

function _marketAuctionRecord(value) {
  const normalized = String(value || '').trim();
  if (!normalized) return null;
  return (APP_DATA.auctionRecords || []).find(record =>
    String(record.code || '').toUpperCase() === normalized.toUpperCase() || record.name === normalized) || null;
}

function _marketAuctionRecords() {
  return (APP_DATA.auctionRecords || []).filter(record =>
    String(record?.code || '').trim() && String(record?.name || '').trim());
}

function _marketPopulateAuctionEditSelect(selectedCode = '', selectedName = '') {
  const select = document.getElementById('me-auctionCode');
  if (!select) return;
  const selected = _marketAuctionRecord(selectedCode) || _marketAuctionRecord(selectedName);
  select.innerHTML = '<option value="">-- オークションを選択 --</option>' +
    _marketAuctionRecords().map(record =>
      `<option value="${_marketEscapeAttr(record.code)}">${_marketEscape(record.code)} — ${_marketEscape(record.name)}</option>`).join('');
  select.value = selected?.code || '';
}

function _marketEnsureData() {
  if (Array.isArray(APP_DATA.marketPrices)) {
    APP_DATA.marketPrices.forEach(row => {
      if (!Number.isFinite(Number(row.marketPriceJpy))) {
        if (row.marketCurrency === 'JPY' && Number.isFinite(Number(row.marketPrice))) {
          row.marketPriceJpy = Number(row.marketPrice);
        } else {
          row.marketPriceJpy = Math.round((Number(row.marketPriceUsd) || 0) * getMarketUsdRate());
        }
      }
      if (row.auctionName == null) {
        const source = String(row.source || '').trim();
        const genericSource = /^(manual|csv|preview-seed)$/i.test(source);
        row.auctionName = !genericSource && source ? source : (getSupplierName(row.supplier) || '');
      }
      const auction = _marketAuctionRecord(row.auctionCode) || _marketAuctionRecord(row.auctionName);
      if (auction) {
        row.auctionCode = auction.code;
        row.auctionName = auction.name;
      }
      if (row.note == null) row.note = row.notes || '';
    });
    if (typeof synchronizeBrandCodesAcrossData === 'function') synchronizeBrandCodesAcrossData();
    return;
  }
  const importDate = _marketToday();
  APP_DATA.marketPrices = (APP_DATA.inventory || []).map((item, index) => ({
    id: `MP-${String(index + 1).padStart(4, '0')}`,
    importDate,
    brandCode: item.brandCode || (typeof getBrandCodeByName === 'function' ? getBrandCodeByName(item.brand) : ''),
    brand: item.brand || '',
    model: item.model || '',
    ref: item.ref || '',
    material: item.material || '',
    movement: item.movement || '',
    condition: item.condition || '',
    auctionCode: '',
    auctionName: '',
    marketPriceJpy: Math.round((MARKET_PRICE_USD_SEED[index] || 0) * getMarketUsdRate()),
    sku: item.sku || '',
    accessories: Array.isArray(item.accessories) ? [...item.accessories] : [],
    note: '',
  }));
}

function init_market() {
  _marketEnsureData();
  _marketBuildFilterOptions();
  if (!_marketInitialized) {
    marketResetFilters(false);
    _marketInitialized = true;
  }
  _marketSyncColumnPanelNote();
  marketRenderTable();
}

function _marketBuildFilterOptions() {
  const rows = APP_DATA.marketPrices || [];
  const brandSelect = document.getElementById('market-f-brand');
  if (brandSelect) {
    const current = brandSelect.dataset.brandRenameValue || brandSelect.value;
    delete brandSelect.dataset.brandRenameValue;
    const brands = typeof getBrandMasterNames === 'function'
      ? getBrandMasterNames(rows.map(row => row.brand))
      : [...new Set(rows.map(row => row.brand).filter(Boolean))];
    brandSelect.innerHTML = '<option value="">すべて</option>' +
      brands.map(brand => `<option value="${_marketEscapeAttr(brand)}">${_marketEscape(brand)}</option>`).join('');
    brandSelect.value = brands.includes(current) ? current : '';
  }

  const auctionSelect = document.getElementById('market-f-auction');
  if (auctionSelect) {
    const current = _marketAuctionRecord(auctionSelect.value);
    const auctions = _marketAuctionRecords();
    auctionSelect.innerHTML = '<option value="">すべて</option>' +
      auctions.map(record => `<option value="${_marketEscapeAttr(record.code)}">${_marketEscape(record.name)}</option>`).join('');
    auctionSelect.value = current && auctions.some(record => record.code === current.code) ? current.code : '';
  }

  ['material', 'movement'].forEach(type => {
    const select = document.getElementById(`market-f-${type}`);
    if (!select) return;
    const current = select.dataset.productSpecRenameValue || select.value;
    delete select.dataset.productSpecRenameValue;
    if (typeof populateProductSpecMasterSelect === 'function') {
      populateProductSpecMasterSelect(`market-f-${type}`, type, {
        emptyLabel: 'すべて',
        selected: current,
        extraCodes: rows.map(row => row[type]),
        labelMode: 'name',
      });
    }
  });

  const conditionSelect = document.getElementById('market-f-condition');
  if (conditionSelect && typeof populateConditionMasterSelect === 'function') {
    populateConditionMasterSelect('market-f-condition', {
      emptyLabel: 'すべて',
      selected: conditionSelect.value,
      extraCodes: rows.map(row => row.condition),
      labelMode: 'name',
    });
  }

  const accessorySelect = document.getElementById('market-f-accessory');
  if (accessorySelect) {
    const current = accessorySelect.dataset.accessoryRenameValue || accessorySelect.value;
    delete accessorySelect.dataset.accessoryRenameValue;
    const rowAccessories = rows.flatMap(row => row.accessories || []);
    const accessories = typeof getAccessoryMasterNames === 'function'
      ? getAccessoryMasterNames(rowAccessories)
      : [...new Set(rowAccessories)];
    accessorySelect.innerHTML = '<option value="">すべて</option>' + accessories
      .map(name => `<option value="${_marketEscapeAttr(name)}">${_marketEscape(name)}</option>`).join('');
    accessorySelect.value = accessories.includes(current) ? current : '';
  }
}

function _marketFilters() {
  return {
    importFrom: document.getElementById('market-f-import-from')?.value || '',
    importTo: document.getElementById('market-f-import-to')?.value || '',
    brand: document.getElementById('market-f-brand')?.value || '',
    model: (document.getElementById('market-f-model')?.value || '').trim().toLowerCase(),
    ref: (document.getElementById('market-f-ref')?.value || '').trim().toLowerCase(),
    auctionCode: document.getElementById('market-f-auction')?.value || '',
    material: document.getElementById('market-f-material')?.value || '',
    movement: document.getElementById('market-f-movement')?.value || '',
    condition: document.getElementById('market-f-condition')?.value || '',
    accessory: document.getElementById('market-f-accessory')?.value || '',
  };
}

function _marketFilteredRows() {
  const f = _marketFilters();
  return (APP_DATA.marketPrices || [])
    .filter(row => {
      if (f.importFrom && row.importDate < f.importFrom) return false;
      if (f.importTo && row.importDate > f.importTo) return false;
      if (f.brand && row.brand !== f.brand) return false;
      if (f.model && !(row.model || '').toLowerCase().includes(f.model)) return false;
      if (f.ref && !(row.ref || '').toLowerCase().includes(f.ref)) return false;
      const auction = _marketAuctionRecord(row.auctionCode) || _marketAuctionRecord(row.auctionName);
      if (f.auctionCode && auction?.code !== f.auctionCode) return false;
      if (f.material && row.material !== f.material) return false;
      if (f.movement && row.movement !== f.movement) return false;
      if (f.condition && row.condition !== f.condition) return false;
      if (f.accessory && !(row.accessories || []).includes(f.accessory)) return false;
      return true;
    })
    .sort((a, b) => (b.importDate || '').localeCompare(a.importDate || '') || String(a.id).localeCompare(String(b.id)));
}

function marketApplyFilters() {
  marketPage = 1;
  marketRenderTable();
}

function marketFilterKeydown(event) {
  if (event.key === 'Enter') {
    event.preventDefault();
    marketApplyFilters();
  }
}

function marketResetFilters(render = true) {
  [
    'market-f-import-from', 'market-f-import-to', 'market-f-model',
    'market-f-ref',
  ].forEach(id => {
    const element = document.getElementById(id);
    if (element) element.value = '';
  });
  ['market-f-brand', 'market-f-auction', 'market-f-material', 'market-f-movement', 'market-f-condition', 'market-f-accessory'].forEach(id => {
    const element = document.getElementById(id);
    if (element) element.selectedIndex = 0;
  });
  marketPage = 1;
  if (render) marketRenderTable();
}

function marketRenderTable() {
  _marketEnsureData();
  if (typeof synchronizeBrandCodesAcrossData === 'function') synchronizeBrandCodesAcrossData();
  const filtered = _marketFilteredRows();
  const count = document.getElementById('marketCount');
  if (count) count.textContent = `${filtered.length} 件`;

  const totalPages = Math.max(1, Math.ceil(filtered.length / MARKET_ITEMS_PER_PAGE));
  if (marketPage > totalPages) marketPage = totalPages;
  const start = (marketPage - 1) * MARKET_ITEMS_PER_PAGE;
  const pageRows = filtered.slice(start, start + MARKET_ITEMS_PER_PAGE);
  const tbody = document.getElementById('marketTableBody');
  if (!tbody) return;

  if (pageRows.length === 0) {
    tbody.innerHTML = `
      <tr>
        <td data-market-empty-row="true" colspan="${Math.max(1, _marketVisibleColumns.size)}" style="text-align:center;color:var(--text-muted);padding:44px 20px;">
          <i class="fa-solid fa-chart-line" style="display:block;font-size:22px;margin-bottom:8px;opacity:.35;"></i>
          条件に一致する相場データがありません
        </td>
      </tr>`;
  } else {
    tbody.innerHTML = pageRows.map(row => {
      const accessories = (row.accessories || []).length ? row.accessories.join('・') : '—';
      const auctionName = (_marketAuctionRecord(row.auctionCode) || _marketAuctionRecord(row.auctionName))?.name
        || row.auctionName || '—';
      return `
        <tr tabindex="0" aria-label="${_marketEscapeAttr(row.brand)} ${_marketEscapeAttr(row.model)}を編集"
          onclick="marketOpenEdit('${row.id}')"
          onkeydown="marketRowKeydown(event,'${row.id}')">
          <td data-market-col="importDate"><span class="market-import-date"><i class="fa-regular fa-calendar"></i>${_marketEscape(_marketDisplayDate(row.importDate))}</span></td>
          <td data-market-col="brand" style="font-weight:600;">${_marketEscape(row.brand || '—')}</td>
          <td data-market-col="model">${_marketEscape(row.model || '—')}</td>
          <td data-market-col="ref">${_marketEscape(row.ref || '—')}</td>
          <td data-market-col="auctionName">${_marketEscape(auctionName)}</td>
          <td data-market-col="marketPrice" class="market-price-yen">${formatMarketPrice(row.marketPriceJpy)}</td>
          <td data-market-col="sku">${_marketEscape(row.sku || '—')}</td>
          <td data-market-col="accessories" class="acc-cell" title="${_marketEscapeAttr(accessories)}">${_marketEscape(accessories)}</td>
          <td data-market-col="note" title="${_marketEscapeAttr(row.note || '')}">${_marketEscape(row.note || '—')}</td>
          <td data-market-col="edit">
            <button class="btn btn-primary btn-sm" type="button"
              style="white-space:nowrap;padding:3px 8px;"
              aria-label="${_marketEscapeAttr(row.brand)} ${_marketEscapeAttr(row.model)}を編集"
              onclick="event.stopPropagation();marketOpenEdit('${row.id}')">
              <i class="fa-solid fa-pen-to-square"></i>
            </button>
          </td>
        </tr>`;
    }).join('');
  }

  _marketApplyColumnVisibility();

  renderPagination('marketPagination', marketPage, Math.ceil(filtered.length / MARKET_ITEMS_PER_PAGE), page => {
    marketPage = page;
    marketRenderTable();
  });
}

function marketRowKeydown(event, id) {
  if (event.key === 'Enter' || event.key === ' ') {
    event.preventDefault();
    marketOpenEdit(id);
  }
}

function _marketFormatUSD(value) {
  if (value == null || value === '') return '—';
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    minimumFractionDigits: Number(value) % 1 === 0 ? 0 : 2,
    maximumFractionDigits: 2,
  }).format(Number(value));
}

function marketOpenEdit(id) {
  _marketEnsureData();
  const row = APP_DATA.marketPrices.find(item => item.id === id);
  if (!row) return;
  _marketEditingId = id;

  const modal = document.getElementById('marketEditModal');
  document.getElementById('marketEditTitle').textContent = `${row.brand || '相場データ'} ${row.model || ''}`.trim();
  document.getElementById('marketEditId').value = id;

  _marketFillSelect('me-brand', [...new Set([...(APP_DATA.brands || []), row.brand].filter(Boolean))], row.brand);
  if (typeof populateProductSpecMasterSelect === 'function') {
    populateProductSpecMasterSelect('me-material', 'material', {
      emptyLabel: '-- 選択 --', selected: row.material, extraCodes: [row.material], labelMode: 'name',
    });
    populateProductSpecMasterSelect('me-movement', 'movement', {
      emptyLabel: '-- 選択 --', selected: row.movement, extraCodes: [row.movement], labelMode: 'name',
    });
  }
  if (typeof populateConditionMasterSelect === 'function') {
    populateConditionMasterSelect('me-condition', {
      emptyLabel: '-- 選択 --', selected: row.condition, extraCodes: [row.condition], labelMode: 'name',
    });
  }

  document.getElementById('me-model').value = row.model || '';
  document.getElementById('me-ref').value = row.ref || '';
  document.getElementById('me-sku').value = row.sku || '';
  document.getElementById('me-importDate').value = row.importDate || _marketToday();
  const marketPriceInput = document.getElementById('me-marketPriceJpy');
  marketPriceInput.value = row.marketPriceJpy ?? '';
  priceFormatHandler(marketPriceInput);
  _marketPopulateAuctionEditSelect(row.auctionCode || '', row.auctionName || '');
  document.getElementById('me-note').value = row.note || '';
  _marketRenderAccessoryOptions(row.accessories || []);
  document.getElementById('marketEditError').classList.remove('show');
  modal.classList.remove('hidden');
  setTimeout(() => document.getElementById('me-brand')?.focus(), 0);
}

function _marketFillSelect(id, values, selected, allowEmpty = false) {
  const element = document.getElementById(id);
  if (!element) return;
  element.innerHTML = (allowEmpty ? '<option value="">-- 選択 --</option>' : '') +
    values.map(value => `<option value="${_marketEscapeAttr(value)}">${_marketEscape(value)}</option>`).join('');
  element.value = selected || '';
}

function marketCloseEdit() {
  document.getElementById('marketEditModal')?.classList.add('hidden');
  _marketEditingId = null;
}

async function marketSaveEdit() {
  const id = document.getElementById('marketEditId').value || _marketEditingId;
  const row = (APP_DATA.marketPrices || []).find(item => item.id === id);
  if (!row) return;

  const brand = document.getElementById('me-brand').value.trim();
  const model = document.getElementById('me-model').value.trim();
  const importDate = document.getElementById('me-importDate').value;
  const marketPriceJpy = _marketParseNumber(document.getElementById('me-marketPriceJpy').value);
  const auctionCode = document.getElementById('me-auctionCode').value;
  const auctionRecord = _marketAuctionRecord(auctionCode);
  const error = document.getElementById('marketEditError');

  if (!brand || !model || !importDate || !auctionRecord || marketPriceJpy < 0) {
    error.textContent = 'ブランド・モデル名・オークション開催日・オークション名を入力し、落札価格は0以上で指定してください。';
    error.classList.add('show');
    return;
  }

  const nextValues = {
    brandCode: typeof getBrandCodeByName === 'function' ? getBrandCodeByName(brand) : (row.brandCode || ''),
    brand,
    model,
    ref: document.getElementById('me-ref').value.trim(),
    sku: document.getElementById('me-sku').value.trim(),
    material: document.getElementById('me-material').value,
    movement: document.getElementById('me-movement').value,
    condition: document.getElementById('me-condition').value,
    importDate,
    marketPriceJpy,
    auctionCode: auctionRecord.code,
    auctionName: auctionRecord.name,
    accessories: _marketSelectedAccessories(),
    note: document.getElementById('me-note').value.trim(),
  };

  const comparableKeys = Object.keys(nextValues);
  const changed = comparableKeys.some(key => JSON.stringify(row[key] ?? null) !== JSON.stringify(nextValues[key] ?? null));

  if (!changed) {
    showToast('info', '変更なし', '変更された項目がありません');
    return;
  }

  if (window.ZaikoAPI && row.apiManaged) {
    try {
      await window.ZaikoAPI.updateMarketPrice(row, nextValues);
      marketCloseEdit();
      _marketBuildFilterOptions();
      marketRenderTable();
      showToast('success', '相場データ更新', `${brand} ${model} をDBへ保存しました`);
    } catch (apiError) {
      error.textContent = apiError.message || '相場データを更新できませんでした。';
      error.classList.add('show');
    }
    return;
  }

  Object.assign(row, nextValues, { updatedAt: new Date().toISOString() });

  marketCloseEdit();
  _marketBuildFilterOptions();
  marketRenderTable();
  showToast('success', '相場データ更新', `${brand} ${model} を更新しました`);
}

function marketOpenCsvPicker() {
  document.getElementById('market-csv-file-input')?.click();
}

async function marketHandleCSVImport(input) {
  const file = input.files?.[0];
  if (!file) return;
  try {
    const text = await file.text();
    marketImportCsvText(text, file.name);
  } catch (error) {
    showToast('error', 'CSV取込エラー', `ファイルを読み込めませんでした: ${error.message}`);
  } finally {
    input.value = '';
  }
}

function marketImportCsvText(text, fileName = 'CSVファイル') {
  _marketEnsureData();
  const rows = marketParseCSV(String(text || '').replace(/^\uFEFF/, ''))
    .filter(row => row.some(cell => String(cell).trim() !== ''));
  if (rows.length < 2) {
    showToast('error', 'CSV取込エラー', 'ヘッダー行と1件以上のデータ行が必要です');
    return { imported: 0, skipped: 0 };
  }

  const headers = rows[0].map(_marketNormalizeHeader);
  const columns = _marketResolveColumns(headers);
  if (columns.brand < 0 || columns.model < 0 || columns.auctionCode < 0) {
    showToast('error', 'CSV取込エラー', '「ブランド」「モデル名」「オークションコード」列が必要です');
    return { imported: 0, skipped: rows.length - 1 };
  }

  const stagedRows = [];
  let skipped = 0;
  const today = _marketToday();

  rows.slice(1).forEach(values => {
    const value = key => columns[key] >= 0 ? String(values[columns[key]] ?? '').trim() : '';
    const brand = value('brand');
    const model = value('model');
    const auction = _marketAuctionRecord(value('auctionCode'));
    if (!brand || !model || !auction) {
      skipped += 1;
      return;
    }

    stagedRows.push({
      importDate: _marketNormalizeDate(value('importDate')) || today,
      brandCode: typeof getBrandCodeByName === 'function' ? getBrandCodeByName(brand) : '',
      brand,
      model,
      ref: value('ref'),
      material: value('material'),
      movement: value('movement'),
      condition: value('condition'),
      auctionCode: auction.code,
      auctionName: auction.name,
      marketPriceJpy: Math.max(0, _marketParseNumber(value('marketPriceJpy')))
        || Math.round(Math.max(0, _marketParseNumber(value('marketPriceUsdLegacy'), true)) * getMarketUsdRate()),
      sku: value('sku'),
      accessories: _marketParseAccessories(value('accessories')),
      note: value('note'),
      sourceFile: fileName,
    });
  });

  if (stagedRows.length > 0) {
    _marketPendingImport = { fileName, rows: stagedRows, skipped };
    _marketRenderCsvPreview();
    document.getElementById('marketCsvPreviewModal')?.classList.remove('hidden');
    showToast('success', 'CSVを明細へ読み込みました', `${stagedRows.length}件を確認してください。まだ登録は完了していません`);
  } else {
    showToast('warning', 'CSV取込結果', '取り込めるデータがありませんでした');
  }
  return { imported: 0, staged: stagedRows.length, skipped };
}

function _marketRenderCsvPreview() {
  const pending = _marketPendingImport;
  const body = document.getElementById('marketCsvPreviewBody');
  const summary = document.getElementById('marketCsvPreviewSummary');
  if (!pending || !body) return;

  if (summary) {
    summary.innerHTML = `<i class="fa-solid fa-file-csv"></i><span><strong>${_marketEscape(pending.fileName)}</strong> から ${pending.rows.length}件を読み込みました${pending.skipped ? `（${pending.skipped}件スキップ）` : ''}</span>`;
  }
  body.innerHTML = pending.rows.map((row, index) => `
    <tr>
      <td>${index + 1}</td>
      <td>${_marketEscape(_marketDisplayDate(row.importDate))}</td>
      <td>${_marketEscape(row.brand || '—')}</td>
      <td>${_marketEscape(row.model || '—')}</td>
      <td>${_marketEscape(row.ref || '—')}</td>
      <td>${_marketEscape(row.auctionName || row.auctionCode || '—')}</td>
      <td class="market-price-yen">${_marketEscape(formatMarketPrice(row.marketPriceJpy))}</td>
      <td>${_marketEscape(row.sku || '—')}</td>
      <td>${_marketEscape((row.accessories || []).join('・') || '—')}</td>
      <td>${_marketEscape(row.note || '—')}</td>
    </tr>
  `).join('');
}

function marketCancelCSVImport() {
  _marketPendingImport = null;
  document.getElementById('marketCsvPreviewModal')?.classList.add('hidden');
  const body = document.getElementById('marketCsvPreviewBody');
  if (body) body.innerHTML = '';
}

function _marketPendingRowsToCSV(rows) {
  const csvCell = value => `"${String(value ?? '').replace(/"/g, '""')}"`;
  const header = ['import_date','brand_text','model_number','reference_number','condition_text',
    'market_price','market_currency','auction_code','notes','sku','material_text','movement_text','accessory_text'];
  const dataRows = rows.map(row => [
    row.importDate, row.brand, row.model, row.ref, row.condition,
    row.marketPriceJpy, 'JPY', row.auctionCode, row.note, row.sku,
    row.material, row.movement, (row.accessories || []).join('・'),
  ]);
  return [header, ...dataRows].map(row => row.map(csvCell).join(',')).join('\r\n');
}

async function marketConfirmCSVImport() {
  const pending = _marketPendingImport;
  if (!pending?.rows?.length) {
    showToast('warning', 'CSV取込', '確認中の明細がありません');
    return { imported: 0, skipped: 0 };
  }

  const button = document.getElementById('marketCsvConfirmButton');
  if (button) {
    button.disabled = true;
    button.innerHTML = '<i class="fa-solid fa-spinner fa-spin"></i> 登録中';
  }

  try {
    let approvalPending = false;
    if (window.ZaikoAPI) {
      const normalizedText = _marketPendingRowsToCSV(pending.rows);
      const normalizedFile = new File([normalizedText], pending.fileName || 'market.csv', { type: 'text/csv;charset=utf-8' });
      const result = await window.ZaikoAPI.importMarketCSV(normalizedFile);
      approvalPending = result?.status === 'pending_approval';
    } else {
      pending.rows.forEach(row => APP_DATA.marketPrices.push({ ...row, id: _marketNextId() }));
    }

    const imported = pending.rows.length;
    const skipped = pending.skipped;
    const fileName = pending.fileName;
    marketCancelCSVImport();
    _marketBuildFilterOptions();
    marketResetFilters(false);
    marketRenderTable();
    _marketShowImportSummary(fileName, imported, skipped);
    showToast('success', approvalPending ? '承認申請を送信しました' : 'CSV取込完了',
      approvalPending ? `${imported}件を管理者の承認待ちにしました` : `${imported}件を登録しました${skipped ? `（${skipped}件スキップ）` : ''}`);
    return { imported, staged: 0, skipped, approvalPending };
  } catch (error) {
    showToast('error', 'CSV登録エラー', error?.message || '相場明細を登録できませんでした');
    throw error;
  } finally {
    if (button) {
      button.disabled = false;
      button.innerHTML = '<i class="fa-solid fa-check"></i> この内容で登録';
    }
  }
}

function marketParseCSV(text) {
  const rows = [];
  let row = [];
  let cell = '';
  let quoted = false;

  for (let index = 0; index < text.length; index += 1) {
    const char = text[index];
    const next = text[index + 1];
    if (char === '"') {
      if (quoted && next === '"') {
        cell += '"';
        index += 1;
      } else {
        quoted = !quoted;
      }
    } else if (char === ',' && !quoted) {
      row.push(cell);
      cell = '';
    } else if ((char === '\n' || char === '\r') && !quoted) {
      if (char === '\r' && next === '\n') index += 1;
      row.push(cell);
      rows.push(row);
      row = [];
      cell = '';
    } else {
      cell += char;
    }
  }

  if (cell !== '' || row.length > 0) {
    row.push(cell);
    rows.push(row);
  }
  return rows;
}

function _marketResolveColumns(headers) {
  const aliases = {
    importDate: ['オークション開催日', '開催日', '取り込み日付', '取込日付', '取込日', 'importdate', 'import_date'],
    brand: ['ブランド', 'brand'],
    model: ['モデル名', 'モデル', 'model'],
    ref: ['型番', 'ref', 'reference'],
    material: ['素材', 'material'],
    movement: ['駆動方式', 'ムーブメント', 'movement'],
    condition: ['コンディション', '状態', 'condition'],
    auctionCode: ['オークションコード', 'auctioncode', 'auction_code'],
    marketPriceJpy: ['落札価格jpy', '落札価格', '相場価格jpy', 'marketpricejpy', 'market_price_jpy'],
    marketPriceUsdLegacy: ['相場価格usd', 'marketpriceusd', 'market_price_usd'],
    sku: ['sku'],
    accessories: ['付属品', 'accessories'],
    note: ['備考', 'notes', 'note'],
  };
  return Object.fromEntries(Object.entries(aliases).map(([key, candidates]) => [
    key,
    headers.findIndex(header => candidates.map(_marketNormalizeHeader).includes(header)),
  ]));
}

function _marketNormalizeHeader(value) {
  return String(value || '')
    .trim()
    .toLowerCase()
    .replace(/[\s　]/g, '')
    .replace(/[（）()・\-]/g, '');
}

function _marketNormalizeDate(value) {
  if (!value) return '';
  const normalized = String(value).trim().replace(/[./]/g, '-');
  const match = normalized.match(/^(\d{4})-(\d{1,2})-(\d{1,2})/);
  if (!match) return '';
  return `${match[1]}-${match[2].padStart(2, '0')}-${match[3].padStart(2, '0')}`;
}

function _marketResolveProductSpec(type, value) {
  if (!value) return '';
  return typeof resolveProductSpecCode === 'function' ? resolveProductSpecCode(type, value) : String(value).trim();
}

function _marketParseAccessories(value) {
  if (!value) return [];
  return [...new Set(String(value).split(/[・,，;；|\/]/).map(item => item.trim()).filter(Boolean))];
}

function _marketRenderAccessoryOptions(selected = []) {
  const wrap = document.getElementById('me-accessories');
  if (!wrap) return;
  const values = typeof getAccessoryMasterNames === 'function'
    ? getAccessoryMasterNames(selected)
    : [...new Set([...(APP_DATA.accessories || []), ...selected])];
  wrap.innerHTML = values.map(name => `
    <label class="checkbox-label ${selected.includes(name) ? 'checked' : ''}">
      <input type="checkbox" value="${_marketEscapeAttr(name)}" ${selected.includes(name) ? 'checked' : ''}
        onchange="this.parentElement.classList.toggle('checked',this.checked)"> ${_marketEscape(name)}
    </label>`).join('');
}

function _marketSelectedAccessories() {
  return [...document.querySelectorAll('#me-accessories input:checked')].map(input => input.value);
}

function _marketParseNumber(value, allowDecimal = false) {
  const sanitized = String(value ?? '')
    .replace(/[０-９]/g, char => String.fromCharCode(char.charCodeAt(0) - 0xFEE0))
    .replace(/[$¥￥,，\s]/g, '')
    .replace(allowDecimal ? /[^0-9.\-]/g : /[^0-9\-]/g, '');
  const number = allowDecimal ? parseFloat(sanitized) : parseInt(sanitized, 10);
  return Number.isFinite(number) ? number : 0;
}

function _marketNextId() {
  _marketImportSequence += 1;
  return `MP-${Date.now()}-${_marketImportSequence}`;
}

function _marketShowImportSummary(fileName, imported, skipped) {
  const summary = document.getElementById('marketImportSummary');
  if (!summary) return;
  summary.innerHTML = `<i class="fa-solid fa-circle-check"></i><strong>${_marketEscape(fileName)}</strong>：${imported}件を取り込み${skipped ? `、${skipped}件をスキップ` : ''}`;
  summary.classList.add('show');
}

function marketExportCSV() {
  const rows = _marketFilteredRows();
  const output = [
    ['オークション開催日', 'ブランド', 'モデル名', '型番', '素材', '駆動方式', 'コンディション', 'オークションコード', '落札価格（JPY）', 'SKU', '付属品', '備考'],
    ...rows.map(row => [
      row.importDate, row.brand, row.model, row.ref || '',
      getProductSpecName('material', row.material), getProductSpecName('movement', row.movement), getConditionName(row.condition),
      row.auctionCode || '', row.marketPriceJpy || 0, row.sku || '', (row.accessories || []).join('・'), row.note || '',
    ]),
  ];
  const csv = output.map(row => row.map(value => `"${String(value ?? '').replace(/"/g, '""')}"`).join(',')).join('\r\n');
  const anchor = document.createElement('a');
  anchor.href = URL.createObjectURL(new Blob(['\uFEFF' + csv], { type: 'text/csv;charset=utf-8;' }));
  anchor.download = `相場表_${_marketToday()}.csv`;
  anchor.click();
  showToast('success', 'CSV出力', `${rows.length}件を出力しました`);
}

function marketDownloadTemplate() {
  const header = ['オークション開催日', 'ブランド', 'モデル名', '型番', '素材', '駆動方式', 'コンディション', 'オークションコード', '落札価格（JPY）', 'SKU', '付属品', '備考'];
  const auction = (APP_DATA.auctionRecords || [])[0] || {};
  const brand = (APP_DATA.brandRecords || [])[0] || {};
  const sample = [
    _marketToday(), brand.name || 'ロレックス', 'サブマリーナ', '116610LN',
    'ステンレス', '自動巻き', '美品', auction.code || '', 1200000,
    `MARKET-SAMPLE-${_marketToday().replace(/-/g, '')}-001`, 'BOX・GUARANTEE',
    '入力例：必要に応じて変更してください',
  ];
  const rows = [header, sample];
  const csv = rows.map(row => row.map(value => `"${String(value ?? '').replace(/"/g, '""')}"`).join(',')).join('\r\n');
  const anchor = document.createElement('a');
  anchor.href = URL.createObjectURL(new Blob(['\uFEFF' + csv], { type: 'text/csv;charset=utf-8;' }));
  anchor.download = '相場表_取込テンプレート.csv';
  anchor.click();
  showToast('info', 'テンプレート', '入力例1行付きの相場CSV取込テンプレートをダウンロードしました');
  return { filename: anchor.download, rows };
}

function _marketEscape(value) {
  return String(value ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

function _marketEscapeAttr(value) {
  return _marketEscape(value).replace(/`/g, '&#96;');
}

document.addEventListener('keydown', event => {
  const modal = document.getElementById('marketEditModal');
  if (event.key !== 'Escape') return;
  marketCloseColumnMenu();
  if (modal && !modal.classList.contains('hidden')) marketCloseEdit();
});

document.addEventListener('click', event => {
  const columnWrap = document.getElementById('market-column-menu-wrap');
  if (columnWrap && !columnWrap.contains(event.target)) marketCloseColumnMenu();
});
