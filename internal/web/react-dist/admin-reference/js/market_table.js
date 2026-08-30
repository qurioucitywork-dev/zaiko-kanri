// =====================================================
// 相場表
// 在庫一覧と同じ商品項目を使い、市場調査日・市場区分・オークション名・取引価格を管理する。
// PostgreSQLの相場表データをREST API経由でAPP_DATAへ同期する。API未接続時のみ初期データを表示する。
// =====================================================

let marketPage = 1;
let marketEntryPage = 1;
const MARKET_ITEMS_PER_PAGE = 10;
let _marketInitialized = false;
let _marketEditingId = null;
let _marketImportSequence = 0;
let _marketPendingImport = null;
let _marketDraftInvalidIndexes = new Set();
let _marketDisplayCurrency = 'JPY';

const MARKET_CATEGORY_LABELS = {
  'domestic-auction': '国内オークション',
  overseas: '海外',
  'domestic-retail': '国内小売',
};

const MARKET_PRICE_USD_SEED = [
  7900, 3350, 19500, 4800, 16500,
  5600, 2900, 4100, 14700, 4750,
];

const MARKET_COLUMN_KEYS = [
  'importDate', 'brand', 'ref', 'model', 'auctionName', 'marketCategory',
  'marketPrice', 'marketResearchRate', 'sku', 'accessories', 'material', 'movement', 'condition',
  'warrantyYearMonth', 'note', 'edit',
];
const MARKET_COLUMN_WIDTHS = {
  importDate: 156, brand: 150, model: 160, ref: 150,
  auctionName: 160, marketCategory: 150, marketPrice: 220, marketResearchRate: 190, sku: 130, accessories: 180, warrantyYearMonth: 118,
  material: 150, movement: 150, condition: 160, note: 330, edit: 72,
};
const _marketVisibleColumns = new Set(MARKET_COLUMN_KEYS);

/** マスタ登録のUSドル円換算レートを返す */
function getMarketUsdRate() {
  const masterRate = Number((APP_DATA.fxRates || []).find(rate => rate.code === 'USD')?.rate);
  if (Number.isFinite(masterRate) && masterRate > 0) return masterRate;
  return Number(globalThis.SALE_PRICE_JPY_PER_USD) || 155;
}

function getMarketJpyRate(currency) {
  const normalized = String(currency || 'JPY').trim().toUpperCase();
  if (normalized === 'JPY') return 1;
  const masterRate = Number((APP_DATA.fxRates || []).find(rate => String(rate.code || '').toUpperCase() === normalized)?.rate);
  if (Number.isFinite(masterRate) && masterRate > 0) return masterRate;
  if (normalized === 'USD') return getMarketUsdRate();
  if (normalized === 'HKD') return 19.8;
  return 0;
}

function _marketNormalizeCurrency(value, category = 'domestic-auction') {
  const normalized = String(value || '').trim().toUpperCase();
  return ['JPY', 'USD', 'HKD'].includes(normalized) ? normalized : 'JPY';
}

function _marketCurrencySymbol(currency) {
  return currency === 'USD' ? '$' : (currency === 'HKD' ? 'HK$' : '¥');
}

function _marketFXRate(row) {
  const scaled = Number(row?.marketFxRateScaled);
  const scale = Number(row?.marketFxScale);
  if (scaled > 0 && scale > 0) return scaled / scale;
  const stored = Number(row?.marketFxRate);
  if (stored > 0) return stored;
  return getMarketJpyRate(row?.marketCurrency || 'JPY');
}

function _marketPriceToJpy(amount, currency, rate) {
  const number = Number(amount) || 0;
  return currency === 'JPY' ? Math.round(number) : Math.round(number * (Number(rate) || 0));
}

function formatMarketAmount(amount, currency = 'JPY') {
  const normalized = String(currency || 'JPY').toUpperCase();
  const number = Number(amount) || 0;
  if (normalized === 'JPY') return `¥${Math.round(number).toLocaleString('ja-JP')}`;
  return `${normalized === 'HKD' ? 'HK$' : '$'}${number.toLocaleString('en-US', { maximumFractionDigits: 2 })}`;
}

function formatMarketResearchRate(rowOrCurrency, rateOverride) {
  const currency = typeof rowOrCurrency === 'string'
    ? String(rowOrCurrency || 'JPY').toUpperCase()
    : String(rowOrCurrency?.marketCurrency || 'JPY').toUpperCase();
  const rate = Number(rateOverride ?? (typeof rowOrCurrency === 'string' ? getMarketJpyRate(currency) : _marketFXRate(rowOrCurrency)));
  if (!(rate > 0)) return '—';
  return `1 ${currency} = ¥${rate.toLocaleString('ja-JP', { minimumFractionDigits: 2, maximumFractionDigits: 4 })}`;
}

/** 市場調査日以前の最新レートを取得し、履歴がない場合は現在のマスタレートを使う。 */
function getMarketJpyRateAtDate(currency, marketDate) {
  const normalized = _marketNormalizeCurrency(currency);
  if (normalized === 'JPY') return 1;
  const target = Date.parse(`${String(marketDate || '').slice(0, 10)}T23:59:59+09:00`);
  if (Number.isFinite(target)) {
    const matched = (APP_DATA.fxRateHistory || [])
      .filter(record => String(record?.code || '').trim().toUpperCase() === normalized
        && Number(record?.rate) > 0)
      .map(record => ({ ...record, timestamp: Date.parse(record.observedAt || record.createdAt || '') }))
      .filter(record => Number.isFinite(record.timestamp) && record.timestamp <= target)
      .sort((left, right) => right.timestamp - left.timestamp)[0];
    if (matched) return Number(matched.rate);
  }
  return getMarketJpyRate(normalized);
}

function _marketJpyAmount(row) {
  const stored = Number(row?.marketPriceJpy);
  if (Number.isFinite(stored)) return Math.round(stored);
  const originalCurrency = _marketNormalizeCurrency(row?.marketCurrency || 'JPY');
  return _marketPriceToJpy(row?.marketPrice, originalCurrency, _marketFXRate(row));
}

/** 保存済み金額を変更せず、相場表の表示だけを指定通貨へ換算する。 */
function getMarketDisplayAmount(row, currency = _marketDisplayCurrency) {
  const targetCurrency = _marketNormalizeCurrency(currency);
  const originalCurrency = _marketNormalizeCurrency(row?.marketCurrency || 'JPY');
  const jpyAmount = _marketJpyAmount(row);
  if (targetCurrency === 'JPY') {
    return { amount: jpyAmount, currency: 'JPY', rate: 1, jpyAmount, originalCurrency };
  }
  if (targetCurrency === originalCurrency && Number.isFinite(Number(row?.marketPrice))) {
    return {
      amount: Number(row.marketPrice),
      currency: targetCurrency,
      rate: _marketFXRate(row),
      jpyAmount,
      originalCurrency,
    };
  }
  const rate = getMarketJpyRateAtDate(targetCurrency, row?.importDate);
  return {
    amount: rate > 0 ? Number((jpyAmount / rate).toFixed(2)) : 0,
    currency: targetCurrency,
    rate,
    jpyAmount,
    originalCurrency,
  };
}

function _marketSyncDisplayCurrencyUI() {
  document.querySelectorAll('[data-market-display-currency]').forEach(button => {
    const active = button.dataset.marketDisplayCurrency === _marketDisplayCurrency;
    button.classList.toggle('active', active);
    button.setAttribute('aria-pressed', String(active));
  });
  const note = _marketDisplayCurrency === 'JPY'
    ? '登録時の円換算額'
    : `市場調査日レートで${_marketDisplayCurrency}換算`;
  document.querySelectorAll('[data-market-display-rate]').forEach(element => {
    element.textContent = note;
  });
}

function marketSwitchDisplayCurrency(currency) {
  const normalized = _marketNormalizeCurrency(currency);
  if (!['JPY', 'USD', 'HKD'].includes(normalized)) return;
  _marketDisplayCurrency = normalized;
  marketRenderTable();
  marketRenderEntryTable();
  _marketSyncDisplayCurrencyUI();
}

/** 取引価格の円換算額を表示する */
function formatMarketPrice(jpyAmount) {
  return formatPrice(Number(jpyAmount) || 0);
}

/** APIが日時を返した場合も、一覧では市場調査日の年月日だけを表示する。 */
function _marketDisplayDate(value) {
  const text = String(value || '').trim();
  if (!text) return '—';
  const match = text.match(/^\d{4}-\d{2}-\d{2}/u);
  return match ? match[0] : text;
}

function _marketSyncColumnPanelNote() {
  const note = document.getElementById('market-column-panel-note');
  if (note) note.textContent = 'チェックを外した項目は一覧から非表示になります / 市場調査レートは登録時点で固定されます';
}

/** 表示項目設定をテーブル・チェックボックスへ反映する */
function _marketApplyColumnVisibility() {
  [document.getElementById('marketTable'), document.getElementById('marketEntryTable')].filter(Boolean).forEach(table => {
    const visibleWidth = [..._marketVisibleColumns]
      .reduce((sum, key) => sum + (MARKET_COLUMN_WIDTHS[key] || 80), 12);
    table.style.minWidth = `${Math.max(320, visibleWidth)}px`;
    table.querySelectorAll('[data-market-col]').forEach(element => {
      element.classList.toggle('market-col-hidden', !_marketVisibleColumns.has(element.dataset.marketCol));
    });
    table.querySelectorAll('td[data-market-empty-row]').forEach(cell => {
      cell.colSpan = Math.max(1, _marketVisibleColumns.size);
    });
  });
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
  select.innerHTML = '<option value="">-- 選択なし --</option>' +
    _marketAuctionRecords().map(record =>
      `<option value="${_marketEscapeAttr(record.code)}">${_marketEscape(record.name)}</option>`).join('');
  select.value = selected?.code || '';
}

function _marketNormalizeCategory(value) {
  const normalized = String(value || '').trim().toLowerCase();
  if (MARKET_CATEGORY_LABELS[normalized]) return normalized;
  const labelMatch = Object.entries(MARKET_CATEGORY_LABELS).find(([, label]) => label === String(value || '').trim());
  return labelMatch?.[0] || 'domestic-auction';
}

function _marketSyncBasicCurrency() {
  const group = document.getElementById('market-basic-currency-group');
  const select = document.getElementById('market-basic-currency');
  const rateText = document.getElementById('market-basic-rate');
  const lockNote = document.getElementById('market-basic-currency-lock-note');
  const category = _marketNormalizeCategory(document.getElementById('market-basic-category')?.value);
  const canSelectForeignCurrency = category === 'overseas';
  if (group) group.hidden = false;
  if (select) {
    if (!canSelectForeignCurrency) select.value = 'JPY';
    select.disabled = !canSelectForeignCurrency;
    select.setAttribute('aria-disabled', canSelectForeignCurrency ? 'false' : 'true');
    select.title = canSelectForeignCurrency ? '取引通貨を選択してください' : '市場区分が海外の場合のみ変更できます';
    if (!['JPY', 'USD', 'HKD'].includes(select.value)) select.value = 'JPY';
  }
  group?.classList.toggle('is-locked', !canSelectForeignCurrency);
  if (lockNote) lockNote.hidden = canSelectForeignCurrency;
  const currency = _marketNormalizeCurrency(select?.value || 'JPY');
  const rate = getMarketJpyRate(currency);
  if (rateText) rateText.textContent = formatMarketResearchRate(currency, rate);
  const draftHeading = document.getElementById('market-draft-price-heading');
  if (draftHeading) draftHeading.textContent = `取引価格（${currency}）`;
  return { currency, rate };
}

function _marketPopulateBasicInfo() {
  const dateInput = document.getElementById('market-basic-research-date');
  if (dateInput && !dateInput.value) dateInput.value = _marketToday();
  const auctionSelect = document.getElementById('market-basic-auction');
  if (auctionSelect) {
    const selected = _marketAuctionRecord(auctionSelect.value)?.code || '';
    auctionSelect.innerHTML = '<option value="">-- 選択なし --</option>' + _marketAuctionRecords().map(record =>
      `<option value="${_marketEscapeAttr(record.code)}">${_marketEscape(record.name)}</option>`).join('');
    auctionSelect.value = selected;
  }
  _marketSyncBasicCurrency();
}

function _marketBasicInfo(showError = false) {
  const category = _marketNormalizeCategory(document.getElementById('market-basic-category')?.value);
  const importDate = document.getElementById('market-basic-research-date')?.value || '';
  const auction = _marketAuctionRecord(document.getElementById('market-basic-auction')?.value || '');
  const currencyInfo = _marketSyncBasicCurrency();
  const marketCurrency = _marketNormalizeCurrency(currencyInfo.currency, category);
  const marketFxRate = Number(currencyInfo.rate) || 0;
  const valid = Boolean(category && importDate && marketCurrency && marketFxRate > 0);
  const error = document.getElementById('marketBasicError');
  if (error) {
    error.textContent = valid ? '' : '市場区分と市場調査日を入力してください。';
    error.classList.toggle('show', showError && !valid);
  }
  return {
    valid,
    marketCategory: category,
    importDate,
    auctionCode: auction?.code || '',
    auctionName: auction?.name || '',
    marketCurrency,
    marketFxRate,
    marketFxRateScaled: Math.round(marketFxRate * 100000000),
    marketFxScale: 100000000,
  };
}

function marketApplyBasicInfoToDrafts() {
  const basic = _marketBasicInfo(false);
  (_marketPendingImport?.rows || []).forEach(row => {
    Object.assign(row, {
      importDate: basic.importDate,
      marketCategory: basic.marketCategory,
      auctionCode: basic.auctionCode,
      auctionName: basic.auctionName,
      marketCurrency: basic.marketCurrency,
      marketFxRate: basic.marketFxRate,
      marketFxRateScaled: basic.marketFxRateScaled,
      marketFxScale: basic.marketFxScale,
    });
    row.marketPriceJpy = _marketPriceToJpy(row.marketPrice, basic.marketCurrency, basic.marketFxRate);
  });
  marketRenderDraftRows();
}

function _marketEnsureData() {
  if (Array.isArray(APP_DATA.marketPrices)) {
    APP_DATA.marketPrices.forEach(row => {
      row.marketCategory = _marketNormalizeCategory(row.marketCategory || row.source);
      row.marketCurrency = ['JPY', 'USD', 'HKD'].includes(String(row.marketCurrency || '').toUpperCase())
        ? String(row.marketCurrency).toUpperCase()
        : (row.marketCategory === 'overseas' ? 'USD' : 'JPY');
      row.marketFxRate = _marketFXRate(row);
      row.marketFxRateScaled = Number(row.marketFxRateScaled) > 0
        ? Number(row.marketFxRateScaled)
        : Math.round(row.marketFxRate * 100000000);
      row.marketFxScale = Number(row.marketFxScale) > 0 ? Number(row.marketFxScale) : 100000000;
      if (!Number.isFinite(Number(row.marketPrice))) {
        row.marketPrice = row.marketCurrency === 'JPY'
          ? Number(row.marketPriceJpy) || 0
          : Number(row.marketPriceUsd) || 0;
      }
      if (!Number.isFinite(Number(row.marketPriceJpy))) {
        row.marketPriceJpy = _marketPriceToJpy(row.marketPrice, row.marketCurrency, row.marketFxRate);
      }
      if (row.auctionName == null) {
        const source = String(row.source || '').trim();
        const genericSource = /^(manual|csv|preview-seed|domestic-auction|overseas|domestic-retail)$/i.test(source);
        row.auctionName = !genericSource && source ? source : (getSupplierName(row.supplier) || '');
      }
      const auction = _marketAuctionRecord(row.auctionCode) || _marketAuctionRecord(row.auctionName);
      if (auction) {
        row.auctionCode = auction.code;
        row.auctionName = auction.name;
      }
      if (row.note == null) row.note = row.notes || '';
      if (row.warrantyYearMonth == null) row.warrantyYearMonth = '';
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
    marketPrice: Math.round((MARKET_PRICE_USD_SEED[index] || 0) * getMarketUsdRate()),
    marketCurrency: 'JPY',
    marketFxRate: 1,
    marketFxRateScaled: 100000000,
    marketFxScale: 100000000,
    sku: item.sku || '',
    accessories: Array.isArray(item.accessories) ? [...item.accessories] : [],
    warrantyYearMonth: item.warrantyYearMonth || '',
    note: '',
    marketCategory: 'domestic-auction',
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
  _marketPopulateBasicInfo();
  marketRenderTable();
  marketRenderEntryTable();
  marketRenderDraftRows();
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
    marketCategory: document.getElementById('market-f-category')?.value || '',
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
      if (f.marketCategory && _marketNormalizeCategory(row.marketCategory || row.source) !== f.marketCategory) return false;
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
  ['market-f-brand', 'market-f-auction', 'market-f-category', 'market-f-material', 'market-f-movement', 'market-f-condition', 'market-f-accessory'].forEach(id => {
    const element = document.getElementById(id);
    if (element) element.selectedIndex = 0;
  });
  marketPage = 1;
  if (render) marketRenderTable();
}

function _marketRenderRows(tbody, pageRows, emptyMessage) {
  if (pageRows.length === 0) {
    tbody.innerHTML = `
      <tr>
        <td data-market-empty-row="true" colspan="${Math.max(1, _marketVisibleColumns.size)}" style="text-align:center;color:var(--text-muted);padding:44px 20px;">
          <i class="fa-solid fa-chart-line" style="display:block;font-size:22px;margin-bottom:8px;opacity:.35;"></i>
          ${_marketEscape(emptyMessage)}
        </td>
      </tr>`;
  } else {
    tbody.innerHTML = pageRows.map(row => {
      const accessories = _marketFormatAccessories(row.accessories || [], row.braceletQty);
      const material = row.material
        ? (typeof getProductSpecName === 'function' ? getProductSpecName('material', row.material) : row.material)
        : '—';
      const movement = row.movement
        ? (typeof getProductSpecName === 'function' ? getProductSpecName('movement', row.movement) : row.movement)
        : '—';
      const condition = row.condition
        ? (typeof getConditionName === 'function' ? getConditionName(row.condition) : row.condition)
        : '—';
      const auctionName = (_marketAuctionRecord(row.auctionCode) || _marketAuctionRecord(row.auctionName))?.name
        || row.auctionName || '—';
      const marketCategory = MARKET_CATEGORY_LABELS[_marketNormalizeCategory(row.marketCategory || row.source)] || '—';
      const displayPrice = getMarketDisplayAmount(row);
      const price = formatMarketAmount(displayPrice.amount, displayPrice.currency);
      const priceDetail = displayPrice.currency === 'JPY'
        ? ''
        : `<small class="market-price-jpy-equivalent">円換算 ${formatMarketPrice(displayPrice.jpyAmount)}</small>`;
      return `
        <tr tabindex="0" aria-label="${_marketEscapeAttr(row.brand)} ${_marketEscapeAttr(row.model)}を編集"
          onclick="marketOpenEdit('${row.id}')"
          onkeydown="marketRowKeydown(event,'${row.id}')">
          <td data-market-col="importDate"><span class="market-import-date"><i class="fa-regular fa-calendar"></i>${_marketEscape(_marketDisplayDate(row.importDate))}</span></td>
          <td data-market-col="brand" style="font-weight:600;">${_marketEscape(row.brand || '—')}</td>
          <td data-market-col="ref">${_marketEscape(row.ref || '—')}</td>
          <td data-market-col="model">${_marketEscape(row.model || '—')}</td>
          <td data-market-col="auctionName">${_marketEscape(auctionName)}</td>
          <td data-market-col="marketCategory"><span class="market-category-badge">${_marketEscape(marketCategory)}</span></td>
          <td data-market-col="marketPrice" class="market-price-yen market-price-cell"
            data-display-currency="${displayPrice.currency}" data-jpy-amount="${displayPrice.jpyAmount}">
            <strong>${_marketEscape(price)}</strong>${priceDetail}
          </td>
          <td data-market-col="marketResearchRate" class="market-research-rate">${_marketEscape(formatMarketResearchRate(row))}</td>
          <td data-market-col="sku">${_marketEscape(row.sku || '—')}</td>
          <td data-market-col="accessories" class="acc-cell" title="${_marketEscapeAttr(accessories)}">${_marketEscape(accessories)}</td>
          <td data-market-col="material">${_marketEscape(material)}</td>
          <td data-market-col="movement">${_marketEscape(movement)}</td>
          <td data-market-col="condition">${_marketEscape(condition)}</td>
          <td data-market-col="warrantyYearMonth">${_marketEscape(row.warrantyYearMonth || '—')}</td>
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

  _marketRenderRows(tbody, pageRows, '条件に一致する相場データがありません');

  _marketApplyColumnVisibility();
  _marketSyncDisplayCurrencyUI();

  renderPagination('marketPagination', marketPage, Math.ceil(filtered.length / MARKET_ITEMS_PER_PAGE), page => {
    marketPage = page;
    marketRenderTable();
  });
}

function marketRenderEntryTable() {
  _marketEnsureData();
  const rows = [...(APP_DATA.marketPrices || [])]
    .sort((a, b) => (b.importDate || '').localeCompare(a.importDate || '') || String(a.id).localeCompare(String(b.id)));
  const count = document.getElementById('marketEntryCount');
  if (count) count.textContent = `${rows.length} 件`;

  const totalPages = Math.max(1, Math.ceil(rows.length / MARKET_ITEMS_PER_PAGE));
  if (marketEntryPage > totalPages) marketEntryPage = totalPages;
  const start = (marketEntryPage - 1) * MARKET_ITEMS_PER_PAGE;
  const pageRows = rows.slice(start, start + MARKET_ITEMS_PER_PAGE);
  const tbody = document.getElementById('marketEntryTableBody');
  if (!tbody) return;

  _marketRenderRows(tbody, pageRows, '登録済みの相場データがありません');
  _marketApplyColumnVisibility();
  _marketSyncDisplayCurrencyUI();
  renderPagination('marketEntryPagination', marketEntryPage, Math.ceil(rows.length / MARKET_ITEMS_PER_PAGE), page => {
    marketEntryPage = page;
    marketRenderEntryTable();
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
  document.getElementById('me-warrantyYearMonth').value = row.warrantyYearMonth || '';
  document.getElementById('me-marketCategory').value = _marketNormalizeCategory(row.marketCategory || row.source);
  const currencySelect = document.getElementById('me-marketCurrency');
  const marketCurrency = _marketNormalizeCurrency(row.marketCurrency || 'JPY');
  if (currencySelect) {
    currencySelect.value = marketCurrency;
    currencySelect.dataset.previousCurrency = marketCurrency;
  }
  const rateText = document.getElementById('me-marketFxRate');
  if (rateText) rateText.dataset.rate = String(_marketFXRate(row));
  const marketPriceInput = document.getElementById('me-marketPriceJpy');
  marketPriceInput.value = row.marketPrice ?? row.marketPriceJpy ?? '';
  marketPriceInput.dataset.jpyAmount = String(_marketJpyAmount(row));
  priceFormatHandler(marketPriceInput);
  marketPriceInput.dataset.lastPrice = String(_marketParseNumber(marketPriceInput.value));
  marketEditCurrencyChanged({ preserveStoredRate: true, recalculatePrice: false });
  _marketPopulateAuctionEditSelect(row.auctionCode || '', row.auctionName || '');
  document.getElementById('me-note').value = row.note || '';
  _marketRenderAccessoryOptions(row.accessories || []);
  const braceletInput = document.getElementById('me-braceletQty');
  if (braceletInput) braceletInput.value = row.braceletQty ?? '';
  _marketToggleEditBraceletQuantity((row.accessories || []).includes('BRACELET PARTS'));
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
  const marketPrice = _marketParseNumber(document.getElementById('me-marketPriceJpy').value);
  const auctionCode = document.getElementById('me-auctionCode').value;
  const auctionRecord = _marketAuctionRecord(auctionCode);
  const marketCategory = _marketNormalizeCategory(document.getElementById('me-marketCategory').value);
  const currencyInfo = marketEditCurrencyChanged(true);
  const marketCurrency = _marketNormalizeCurrency(currencyInfo.currency, marketCategory);
  const marketFxRate = Number(currencyInfo.rate) || 0;
  const marketPriceJpy = Number.isFinite(Number(currencyInfo.jpyAmount))
    ? Math.round(Number(currencyInfo.jpyAmount))
    : _marketPriceToJpy(marketPrice, marketCurrency, marketFxRate);
  const error = document.getElementById('marketEditError');

  if (!brand || !model || !importDate || !marketCategory || marketPrice < 0 || marketFxRate <= 0) {
    error.textContent = 'ブランド・モデル名・市場調査日・市場区分を入力し、取引価格は0以上で指定してください。';
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
    warrantyYearMonth: document.getElementById('me-warrantyYearMonth').value,
    importDate,
    marketCategory,
    source: marketCategory,
    marketPrice,
    marketPriceJpy,
    marketCurrency,
    marketFxRate,
    marketFxRateScaled: Math.round(marketFxRate * 100000000),
    marketFxScale: 100000000,
    auctionCode: auctionRecord?.code || '',
    auctionName: auctionRecord?.name || '',
    accessories: _marketSelectedAccessories(),
    braceletQty: _marketSelectedAccessories().includes('BRACELET PARTS')
      ? _marketNormalizeBraceletQuantity(document.getElementById('me-braceletQty')?.value)
      : null,
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
      marketRenderEntryTable();
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
  marketRenderEntryTable();
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
  const basic = _marketBasicInfo(true);
  if (!basic.valid) {
    showToast('warning', '基本情報を確認してください', '市場区分と市場調査日を入力してからCSVを取り込んでください');
    return { imported: 0, staged: 0, skipped: 0, basicInfoError: true };
  }
  const rows = marketParseCSV(String(text || '').replace(/^\uFEFF/, ''))
    .filter(row => row.some(cell => String(cell).trim() !== ''));
  if (rows.length < 2) {
    showToast('error', 'CSV取込エラー', 'ヘッダー行と1件以上のデータ行が必要です');
    return { imported: 0, skipped: 0 };
  }

  const headers = rows[0].map(_marketNormalizeHeader);
  const columns = _marketResolveColumns(headers);
  if (columns.brand < 0 || columns.model < 0 || columns.marketPriceJpy < 0) {
    showToast('error', 'CSV取込エラー', '「ブランドコード」「モデル」「取引価格」列が必要です');
    return { imported: 0, skipped: rows.length - 1 };
  }

  const stagedRows = [];
  let skipped = 0;
  rows.slice(1).forEach(values => {
    const value = key => columns[key] >= 0 ? String(values[columns[key]] ?? '').trim() : '';
    const brandValue = value('brand');
    const brandRecord = (APP_DATA.brandRecords || []).find(record => record.code === brandValue || record.name === brandValue);
    const brand = brandRecord?.name || '';
    const model = value('model');
    if (!brand || !model) {
      skipped += 1;
      return;
    }

    const marketPrice = Math.max(0, _marketParseNumber(value('marketPriceJpy')))
      || Math.max(0, _marketParseNumber(value('marketPriceUsdLegacy'), true));
    stagedRows.push({
      importDate: basic.importDate,
      marketCategory: basic.marketCategory,
      brandCode: brandRecord?.code || (typeof getBrandCodeByName === 'function' ? getBrandCodeByName(brand) : ''),
      brand,
      model,
      ref: value('ref'),
      material: _marketResolveProductSpec('material', value('material')),
      movement: _marketResolveProductSpec('movement', value('movement')),
      condition: typeof resolveConditionCode === 'function' ? resolveConditionCode(value('condition')) : value('condition'),
      warrantyYearMonth: _marketNormalizeWarrantyYearMonth(value('warrantyYearMonth')),
      auctionCode: basic.auctionCode,
      auctionName: basic.auctionName,
      marketPrice,
      marketPriceJpy: _marketPriceToJpy(marketPrice, basic.marketCurrency, basic.marketFxRate),
      marketCurrency: basic.marketCurrency,
      marketFxRate: basic.marketFxRate,
      marketFxRateScaled: basic.marketFxRateScaled,
      marketFxScale: basic.marketFxScale,
      sku: value('sku'),
      accessories: _marketParseAccessories(value('accessories')).map(code => (APP_DATA.accessoryRecords || []).find(record => record.code === code || record.name === code)?.name || code),
      braceletQty: _marketNormalizeBraceletQuantity(value('braceletQty')),
      note: value('note'),
      sourceFile: fileName,
    });
  });

  if (stagedRows.length > 0) {
    if (_marketPendingImport?.rows?.length) {
      _marketPendingImport.rows.push(...stagedRows);
      _marketPendingImport.skipped = Number(_marketPendingImport.skipped || 0) + skipped;
      _marketPendingImport.fileName = `${_marketPendingImport.fileName || '手入力明細'} + ${fileName}`;
    } else {
      _marketPendingImport = { fileName, rows: stagedRows, skipped };
    }
    _marketDraftInvalidIndexes.clear();
    marketRenderDraftRows();
    document.getElementById('market-entry-draft-area')?.scrollIntoView({ behavior: 'smooth', block: 'start' });
    showToast('success', 'CSVを登録明細へ読み込みました', `${stagedRows.length}件を確認・編集できます。まだDBには登録されていません`);
  } else {
    showToast('warning', 'CSV取込結果', '取り込めるデータがありませんでした');
  }
  return { imported: 0, staged: stagedRows.length, skipped };
}

function marketEditPriceInputChanged(input) {
  if (!input) return;
  priceFormatHandler(input);
  const currency = _marketNormalizeCurrency(document.getElementById('me-marketCurrency')?.value || 'JPY');
  const rate = Number(document.getElementById('me-marketFxRate')?.dataset.rate)
    || getMarketJpyRateAtDate(currency, document.getElementById('me-importDate')?.value);
  const price = _marketParseNumber(input.value);
  input.dataset.jpyAmount = String(_marketPriceToJpy(price, currency, rate));
  input.dataset.lastPrice = String(price);
}

function marketEditCurrencyChanged(options = false) {
  const settings = typeof options === 'object' && options !== null
    ? options
    : { preserveStoredRate: Boolean(options), recalculatePrice: false };
  const preserveStoredRate = Boolean(settings.preserveStoredRate);
  const recalculatePrice = Boolean(settings.recalculatePrice);
  const currencySelect = document.getElementById('me-marketCurrency');
  const currencyGroup = document.getElementById('me-marketCurrencyGroup');
  const rateText = document.getElementById('me-marketFxRate');
  const priceLabel = document.getElementById('me-marketPriceLabel');
  const prefix = document.getElementById('me-marketCurrencyPrefix');
  const priceInput = document.getElementById('me-marketPriceJpy');
  const conversionNote = document.getElementById('me-marketConversionNote');
  const marketDate = document.getElementById('me-importDate')?.value || _marketToday();
  if (currencyGroup) currencyGroup.hidden = false;
  if (currencySelect) {
    currencySelect.disabled = false;
    if (!['JPY', 'USD', 'HKD'].includes(currencySelect.value)) currencySelect.value = 'JPY';
  }
  const currency = _marketNormalizeCurrency(currencySelect?.value || 'JPY');
  const previousCurrency = _marketNormalizeCurrency(currencySelect?.dataset.previousCurrency || currency);
  const previousRate = Number(rateText?.dataset.rate) || getMarketJpyRateAtDate(previousCurrency, marketDate);
  let jpyAmount = Number(priceInput?.dataset.jpyAmount);
  const currentPrice = _marketParseNumber(priceInput?.value);
  const lastPrice = Number(priceInput?.dataset.lastPrice);
  if (Number.isFinite(lastPrice) && currentPrice !== lastPrice) {
    jpyAmount = _marketPriceToJpy(currentPrice, previousCurrency, previousRate);
  }
  if (!Number.isFinite(jpyAmount)) {
    jpyAmount = _marketPriceToJpy(currentPrice, previousCurrency, previousRate);
  }
  let rate = preserveStoredRate ? Number(rateText?.dataset.rate) : 0;
  if (!(rate > 0)) rate = getMarketJpyRateAtDate(currency, marketDate);
  if (recalculatePrice && priceInput && Number.isFinite(jpyAmount) && rate > 0) {
    const convertedPrice = currency === 'JPY' ? Math.round(jpyAmount) : Math.round(jpyAmount / rate);
    priceInput.value = String(convertedPrice);
    priceFormatHandler(priceInput);
    priceInput.dataset.jpyAmount = String(Math.round(jpyAmount));
    priceInput.dataset.lastPrice = String(convertedPrice);
  }
  if (rateText) {
    rateText.dataset.rate = String(rate);
    rateText.textContent = formatMarketResearchRate(currency, rate);
  }
  if (priceLabel) priceLabel.textContent = `取引価格（${currency}）`;
  if (prefix) prefix.textContent = _marketCurrencySymbol(currency);
  if (currencySelect) currencySelect.dataset.previousCurrency = currency;
  if (conversionNote) {
    conversionNote.textContent = currency === 'JPY'
      ? `円換算基準額：${formatMarketPrice(jpyAmount)}`
      : `${marketDate} 時点のレートで換算（円換算基準額：${formatMarketPrice(jpyAmount)}）`;
  }
  return { currency, rate, jpyAmount };
}

function _marketDraftBrandRecords() {
  const records = new Map();
  (APP_DATA.brandRecords || []).forEach(record => {
    const name = String(record?.name || '').trim();
    if (name) records.set(name, { code: String(record?.code || '').trim(), name });
  });
  (APP_DATA.brands || []).forEach(value => {
    const name = String(typeof value === 'string' ? value : value?.name || '').trim();
    if (name && !records.has(name)) records.set(name, { code: '', name });
  });
  (APP_DATA.marketPrices || []).forEach(row => {
    const name = String(row?.brand || '').trim();
    if (name && !records.has(name)) records.set(name, { code: row?.brandCode || '', name });
  });
  return [...records.values()].sort((left, right) => left.name.localeCompare(right.name, 'ja'));
}

function _marketCreateDraftRow() {
  const basic = _marketBasicInfo(false);
  return {
    importDate: basic.importDate || _marketToday(),
    marketCategory: basic.marketCategory,
    brandCode: '',
    brand: '',
    model: '',
    ref: '',
    material: '',
    movement: '',
    condition: '',
    warrantyYearMonth: '',
    auctionCode: basic.auctionCode,
    auctionName: basic.auctionName,
    marketPrice: '',
    marketPriceJpy: 0,
    marketCurrency: basic.marketCurrency,
    marketFxRate: basic.marketFxRate,
    marketFxRateScaled: basic.marketFxRateScaled,
    marketFxScale: basic.marketFxScale,
    sku: '',
    accessories: [],
    braceletQty: null,
    note: '',
    sourceFile: '手入力明細',
  };
}

/** 手入力用の空明細を、CSV取込と共通の登録前プレビューへ追加する。 */
function marketAddDraftRow() {
  return marketAddDraftRows(1);
}

/** 指定行数分の空明細を一括追加する。 */
function marketAddDraftRows(countOverride) {
  const input = document.getElementById('marketDraftAddCount');
  const requested = countOverride == null ? Number(input?.value || 1) : Number(countOverride);
  const count = Math.min(100, Math.max(1, Math.floor(Number.isFinite(requested) ? requested : 1)));
  if (input) input.value = String(count);
  if (!_marketPendingImport) _marketPendingImport = { fileName: '手入力明細', rows: [], skipped: 0 };
  const startIndex = _marketPendingImport.rows.length;
  for (let index = 0; index < count; index += 1) _marketPendingImport.rows.push(_marketCreateDraftRow());
  _marketDraftInvalidIndexes.clear();
  marketRenderDraftRows();
  setTimeout(() => document.getElementById(`market-draft-brand-${startIndex}`)?.focus(), 0);
  return count;
}

/** 登録前明細の入力値を下書きへ反映する。 */
function marketUpdateDraftRow(index, field, rawValue) {
  const row = _marketPendingImport?.rows?.[index];
  if (!row) return;
  const value = String(rawValue ?? '');
  if (field === 'brand') {
    const record = _marketDraftBrandRecords().find(candidate => candidate.name === value || candidate.code === value);
    row.brand = record?.name || value.trim();
    row.brandCode = record?.code || (typeof getBrandCodeByName === 'function' ? getBrandCodeByName(row.brand) : '');
  } else if (field === 'auctionCode') {
    const record = _marketAuctionRecord(value);
    row.auctionCode = record?.code || '';
    row.auctionName = record?.name || '';
  } else if (field === 'marketPrice') {
    row.marketPrice = value.trim() === '' ? '' : Math.max(0, _marketParseNumber(value));
    row.marketPriceJpy = _marketPriceToJpy(row.marketPrice, row.marketCurrency || 'JPY', row.marketFxRate || 1);
  } else if (field === 'accessories') {
    row.accessories = _marketNormalizeDraftAccessories(Array.isArray(rawValue) ? rawValue : value);
    if (!row.accessories.includes('BRACELET PARTS')) row.braceletQty = null;
  } else if (field === 'braceletQty') {
    row.braceletQty = _marketNormalizeBraceletQuantity(value);
  } else {
    row[field] = value;
  }
  _marketDraftInvalidIndexes.delete(index);
  document.querySelector(`#marketDraftTableBody tr[data-draft-index="${index}"]`)?.classList.remove('market-draft-row-invalid');
  const error = document.getElementById('marketDraftError');
  if (error) error.classList.remove('show');
}

function marketRemoveDraftRow(index) {
  if (!_marketPendingImport?.rows?.[index]) return;
  _marketPendingImport.rows.splice(index, 1);
  if (_marketPendingImport.rows.length === 0) _marketPendingImport = null;
  _marketDraftInvalidIndexes.clear();
  marketRenderDraftRows();
}

function marketResetDraftRows() {
  _marketPendingImport = null;
  _marketDraftInvalidIndexes.clear();
  marketRenderDraftRows();
  document.getElementById('marketCsvPreviewModal')?.classList.add('hidden');
}

function _marketDraftBrandOptions(selected) {
  return '<option value="">-- 選択 --</option>' + _marketDraftBrandRecords().map(record =>
    `<option value="${_marketEscapeAttr(record.name)}" ${record.name === selected ? 'selected' : ''}>${_marketEscape(record.name)}</option>`).join('');
}

function _marketDraftAuctionOptions(selectedCode, selectedName) {
  const selected = (_marketAuctionRecord(selectedCode) || _marketAuctionRecord(selectedName))?.code || '';
  return '<option value="">-- 選択 --</option>' + _marketAuctionRecords().map(record =>
    `<option value="${_marketEscapeAttr(record.code)}" ${record.code === selected ? 'selected' : ''}>${_marketEscape(record.name)}</option>`).join('');
}

function _marketDraftProductSpecOptions(type, selectedValue) {
  const selected = _marketResolveProductSpec(type, selectedValue);
  const records = typeof getProductSpecMasterRecords === 'function'
    ? getProductSpecMasterRecords(type, selected ? [selected] : [])
    : [...(type === 'movement' ? APP_DATA.movements || [] : APP_DATA.materials || [])];
  return '<option value="">-- 選択 --</option>' + records.map(record =>
    `<option value="${_marketEscapeAttr(record.code)}" ${record.code === selected ? 'selected' : ''}>${_marketEscape(record.name || record.code)}</option>`).join('');
}

function _marketDraftConditionOptions(selectedValue) {
  const selected = typeof resolveConditionCode === 'function'
    ? resolveConditionCode(selectedValue)
    : String(selectedValue || '').trim();
  const records = typeof getConditionMasterRecords === 'function'
    ? getConditionMasterRecords(selected ? [selected] : [])
    : [...(APP_DATA.conditions || [])];
  return '<option value="">-- 選択 --</option>' + records.map(record =>
    `<option value="${_marketEscapeAttr(record.code)}" ${record.code === selected ? 'selected' : ''}>${_marketEscape(record.name || record.code)}</option>`).join('');
}

/** 付属品マスタと取込済みの値を統合し、明細用の複数選択候補を返す。 */
function _marketDraftAccessoryRecords(selected = []) {
  const records = new Map();
  (APP_DATA.accessoryRecords || []).forEach(record => {
    const name = String(record?.name || '').trim();
    if (name && !records.has(name)) records.set(name, { code: String(record?.code || '').trim(), name });
  });
  (APP_DATA.accessories || []).forEach(value => {
    const name = String(typeof value === 'string' ? value : value?.name || '').trim();
    if (name && !records.has(name)) records.set(name, { code: '', name });
  });
  _marketNormalizeDraftAccessories(selected).forEach(name => {
    if (!records.has(name)) records.set(name, { code: '', name });
  });
  return [...records.values()];
}

/** コード・名称・区切り文字列のいずれからでも、重複のない付属品名称配列へ正規化する。 */
function _marketNormalizeDraftAccessories(value) {
  const values = Array.isArray(value) ? value : _marketParseAccessories(value);
  return [...new Set(values.map(item => {
    const normalized = String(item || '').trim();
    return (APP_DATA.accessoryRecords || []).find(record => record.code === normalized || record.name === normalized)?.name || normalized;
  }).filter(Boolean))];
}

function _marketDraftAccessorySelector(row, index) {
  const selected = _marketNormalizeDraftAccessories(row.accessories || []);
  row.accessories = selected;
  const selectedSet = new Set(selected);
  const hasBraceletParts = selectedSet.has('BRACELET PARTS');
  const summary = selected.length ? _marketFormatAccessories(selected, row.braceletQty) : '-- 選択 --';
  const options = _marketDraftAccessoryRecords(selected).map(record => `
    <label class="market-draft-accessory-option ${selectedSet.has(record.name) ? 'checked' : ''}">
      <input type="checkbox" value="${_marketEscapeAttr(record.name)}" ${selectedSet.has(record.name) ? 'checked' : ''}
        onchange="marketDraftAccessoryChanged(${index})">
      <span>${_marketEscape(record.name)}</span>
    </label>`).join('');
  return `
    <div class="market-draft-accessory-select" data-market-draft-accessories="${index}">
      <button type="button" class="market-draft-accessory-button" aria-haspopup="true" aria-expanded="false"
        aria-controls="market-draft-accessory-menu-${index}"
        aria-label="明細${index + 1} 付属品を複数選択（${selected.length}件選択中）"
        onclick="marketToggleDraftAccessoryMenu(${index},event)">
        <span class="market-draft-accessory-summary" data-market-draft-accessory-summary="${index}" title="${_marketEscapeAttr(summary)}">${_marketEscape(summary)}</span>
        <span class="market-draft-accessory-count" data-market-draft-accessory-count="${index}" ${selected.length ? '' : 'hidden'}>${selected.length}件</span>
        <i class="fa-solid fa-chevron-down" aria-hidden="true"></i>
      </button>
      <div class="market-draft-accessory-menu" id="market-draft-accessory-menu-${index}" role="group"
        aria-label="明細${index + 1} 付属品候補" hidden onclick="event.stopPropagation()">
        ${options || '<span class="market-draft-accessory-empty">付属品マスタがありません</span>'}
      </div>
      <label class="market-draft-bracelet-qty" data-market-draft-bracelet-qty="${index}" ${hasBraceletParts ? '' : 'hidden'}>
        <span>コマ数</span>
        <input type="number" min="0" step="1" inputmode="numeric" class="form-control"
          aria-label="明細${index + 1} BRACELET PARTS コマ数" value="${row.braceletQty ?? ''}" placeholder="例: 8"
          oninput="marketDraftBraceletQtyChanged(${index},this)">
        <span>コマ</span>
      </label>
    </div>`;
}

/** 明細ごとの付属品候補を開閉する。チェック欄は開いたまま複数選択できる。 */
function marketToggleDraftAccessoryMenu(index, event) {
  event?.stopPropagation?.();
  const menu = document.getElementById(`market-draft-accessory-menu-${index}`);
  const select = document.querySelector(`[data-market-draft-accessories="${index}"]`);
  const button = select?.querySelector('.market-draft-accessory-button');
  if (!menu || !button) return;
  const shouldOpen = menu.hidden;
  document.querySelectorAll('.market-draft-accessory-menu').forEach(candidate => { candidate.hidden = true; });
  document.querySelectorAll('.market-draft-accessory-button').forEach(candidate => candidate.setAttribute('aria-expanded', 'false'));
  menu.hidden = !shouldOpen;
  button.setAttribute('aria-expanded', shouldOpen ? 'true' : 'false');
}

/** チェック状態を下書きへ即時反映し、選択内容と件数をセル内へ表示する。 */
function marketDraftAccessoryChanged(index) {
  const select = document.querySelector(`[data-market-draft-accessories="${index}"]`);
  const menu = document.getElementById(`market-draft-accessory-menu-${index}`);
  if (!select || !menu) return;
  const selected = [...menu.querySelectorAll('input[type="checkbox"]:checked')].map(input => input.value);
  marketUpdateDraftRow(index, 'accessories', selected);
  menu.querySelectorAll('.market-draft-accessory-option').forEach(option => {
    option.classList.toggle('checked', Boolean(option.querySelector('input')?.checked));
  });
  const row = _marketPendingImport?.rows?.[index];
  const hasBraceletParts = selected.includes('BRACELET PARTS');
  const quantityRow = select.querySelector(`[data-market-draft-bracelet-qty="${index}"]`);
  const quantityInput = quantityRow?.querySelector('input');
  if (quantityRow) quantityRow.hidden = !hasBraceletParts;
  if (!hasBraceletParts && quantityInput) quantityInput.value = '';
  const summaryText = selected.length ? _marketFormatAccessories(selected, row?.braceletQty) : '-- 選択 --';
  const summary = select.querySelector('[data-market-draft-accessory-summary]');
  const count = select.querySelector('[data-market-draft-accessory-count]');
  const button = select.querySelector('.market-draft-accessory-button');
  if (summary) {
    summary.textContent = summaryText;
    summary.title = summaryText;
  }
  if (count) {
    count.textContent = `${selected.length}件`;
    count.hidden = selected.length === 0;
  }
  button?.setAttribute('aria-label', `明細${index + 1} 付属品を複数選択（${selected.length}件選択中）`);
}

function marketDraftBraceletQtyChanged(index, input) {
  if (!input) return;
  input.value = String(input.value || '').replace(/[^0-9]/g, '');
  marketUpdateDraftRow(index, 'braceletQty', input.value);
  const row = _marketPendingImport?.rows?.[index];
  const select = document.querySelector(`[data-market-draft-accessories="${index}"]`);
  const summaryText = _marketFormatAccessories(row?.accessories || [], row?.braceletQty);
  const summary = select?.querySelector('[data-market-draft-accessory-summary]');
  if (summary) {
    summary.textContent = summaryText;
    summary.title = summaryText;
  }
}

/** CSVと手入力の下書きを、ページ内の編集可能な明細表として表示する。 */
function marketRenderDraftRows() {
  const rows = _marketPendingImport?.rows || [];
  const body = document.getElementById('marketDraftTableBody');
  const empty = document.getElementById('marketDraftEmpty');
  const wrap = document.getElementById('marketDraftTableWrap');
  const count = document.getElementById('marketDraftCount');
  const summary = document.getElementById('marketDraftSummary');
  const registerButton = document.getElementById('marketDraftRegisterButton');
  const resetButton = document.getElementById('marketDraftResetButton');
  if (count) count.textContent = `${rows.length} 件`;
  if (empty) empty.hidden = rows.length > 0;
  if (wrap) wrap.hidden = rows.length === 0;
  if (registerButton) registerButton.disabled = rows.length === 0;
  if (resetButton) resetButton.disabled = rows.length === 0;
  if (summary) {
    summary.innerHTML = rows.length
      ? `<i class="fa-solid fa-pen-to-square"></i><span><strong>${rows.length}件</strong>の登録前明細です。表の各項目を直接修正できます${_marketPendingImport?.skipped ? `（CSVで${_marketPendingImport.skipped}件スキップ）` : ''}。</span>`
      : '<i class="fa-solid fa-circle-info"></i><span>CSVを取り込むか「明細を追加」から手入力してください。</span>';
  }
  if (!body) return;
  body.innerHTML = rows.map((row, index) => `
    <tr data-draft-index="${index}" class="${_marketDraftInvalidIndexes.has(index) ? 'market-draft-row-invalid' : ''}">
      <td class="market-draft-number">${index + 1}</td>
      <td><select class="form-control" id="market-draft-brand-${index}" aria-label="明細${index + 1} ブランド" onchange="marketUpdateDraftRow(${index},'brand',this.value)">${_marketDraftBrandOptions(row.brand)}</select></td>
      <td><input type="text" class="form-control" aria-label="明細${index + 1} 型番" value="${_marketEscapeAttr(row.ref || '')}" placeholder="型番" oninput="marketUpdateDraftRow(${index},'ref',this.value)"></td>
      <td><input type="text" class="form-control" aria-label="明細${index + 1} モデル名" value="${_marketEscapeAttr(row.model || '')}" placeholder="モデル名" oninput="marketUpdateDraftRow(${index},'model',this.value)"></td>
      <td><select class="form-control" id="market-draft-material-${index}" aria-label="明細${index + 1} 素材" onchange="marketUpdateDraftRow(${index},'material',this.value)">${_marketDraftProductSpecOptions('material', row.material)}</select></td>
      <td><select class="form-control" id="market-draft-movement-${index}" aria-label="明細${index + 1} 駆動方式" onchange="marketUpdateDraftRow(${index},'movement',this.value)">${_marketDraftProductSpecOptions('movement', row.movement)}</select></td>
      <td><select class="form-control" id="market-draft-condition-${index}" aria-label="明細${index + 1} コンディション" onchange="marketUpdateDraftRow(${index},'condition',this.value)">${_marketDraftConditionOptions(row.condition)}</select></td>
      <td><input type="month" class="form-control" aria-label="明細${index + 1} 保証年月" value="${_marketEscapeAttr(row.warrantyYearMonth || '')}" onchange="marketUpdateDraftRow(${index},'warrantyYearMonth',this.value)"></td>
      <td><div class="market-draft-price"><span>${_marketEscape(_marketCurrencySymbol(row.marketCurrency || 'JPY'))}</span><input type="text" inputmode="numeric" class="form-control" aria-label="明細${index + 1} 取引価格" value="${row.marketPrice === '' ? '' : Number(row.marketPrice || 0).toLocaleString('ja-JP')}" placeholder="0" oninput="marketUpdateDraftRow(${index},'marketPrice',this.value)" onblur="priceFormatHandler(this)"></div></td>
      <td><input type="text" class="form-control" aria-label="明細${index + 1} SKU" value="${_marketEscapeAttr(row.sku || '')}" placeholder="任意" oninput="marketUpdateDraftRow(${index},'sku',this.value)"></td>
      <td>${_marketDraftAccessorySelector(row, index)}</td>
      <td><button type="button" class="market-draft-remove" aria-label="明細${index + 1}を削除" onclick="marketRemoveDraftRow(${index})"><i class="fa-solid fa-xmark"></i></button></td>
    </tr>`).join('');
}

function _marketValidateDraftRows() {
  const rows = _marketPendingImport?.rows || [];
  const basic = _marketBasicInfo(true);
  const normalizedRows = [];
  _marketDraftInvalidIndexes.clear();
  rows.forEach((row, index) => {
    const brandRecord = _marketDraftBrandRecords().find(record => record.name === row.brand || record.code === row.brandCode);
    const priceMissing = row.marketPrice === '' || row.marketPrice == null || !Number.isFinite(Number(row.marketPrice));
    if (!basic.valid || !brandRecord || !String(row.model || '').trim() || priceMissing) {
      _marketDraftInvalidIndexes.add(index);
      return;
    }
    normalizedRows.push({
      ...row,
      importDate: basic.importDate,
      marketCategory: basic.marketCategory,
      source: basic.marketCategory,
      brandCode: brandRecord.code || (typeof getBrandCodeByName === 'function' ? getBrandCodeByName(brandRecord.name) : ''),
      brand: brandRecord.name,
      model: String(row.model || '').trim(),
      ref: String(row.ref || '').trim(),
      auctionCode: basic.auctionCode,
      auctionName: basic.auctionName,
      marketPrice: Math.max(0, Math.round(Number(row.marketPrice))),
      marketPriceJpy: _marketPriceToJpy(row.marketPrice, basic.marketCurrency, basic.marketFxRate),
      marketCurrency: basic.marketCurrency,
      marketFxRate: basic.marketFxRate,
      marketFxRateScaled: basic.marketFxRateScaled,
      marketFxScale: basic.marketFxScale,
      sku: String(row.sku || '').trim(),
      braceletQty: (row.accessories || []).includes('BRACELET PARTS')
        ? _marketNormalizeBraceletQuantity(row.braceletQty)
        : null,
      note: String(row.note || '').trim(),
    });
  });
  return { valid: _marketDraftInvalidIndexes.size === 0 && normalizedRows.length > 0, rows: normalizedRows };
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
      <td>${_marketEscape(row.ref || '—')}</td>
      <td>${_marketEscape(row.model || '—')}</td>
      <td>${_marketEscape(row.auctionName || row.auctionCode || '—')}</td>
      <td class="market-price-yen">${_marketEscape(formatMarketAmount(row.marketPrice, row.marketCurrency))}</td>
      <td>${_marketEscape(formatMarketResearchRate(row))}</td>
      <td>${_marketEscape(row.sku || '—')}</td>
      <td>${_marketEscape(_marketFormatAccessories(row.accessories || [], row.braceletQty))}</td>
      <td>${_marketEscape(row.warrantyYearMonth || '—')}</td>
      <td>${_marketEscape(row.note || '—')}</td>
    </tr>
  `).join('');
}

function marketCancelCSVImport() {
  _marketPendingImport = null;
  _marketDraftInvalidIndexes.clear();
  document.getElementById('marketCsvPreviewModal')?.classList.add('hidden');
  const body = document.getElementById('marketCsvPreviewBody');
  if (body) body.innerHTML = '';
  marketRenderDraftRows();
}

function _marketPendingRowsToCSV(rows) {
  const csvCell = value => `"${String(value ?? '').replace(/"/g, '""')}"`;
  const header = ['import_date','market_category','brand_text','model_number','reference_number','condition_text','warranty_year_month',
    'market_price','market_currency','market_fx_rate','auction_code','notes','sku','material_text','movement_text','accessory_text','bracelet_quantity'];
  const dataRows = rows.map(row => [
    row.importDate, row.marketCategory, row.brand, row.model, row.ref, row.condition, row.warrantyYearMonth,
    row.marketPrice, row.marketCurrency, row.marketFxRate, row.auctionCode, row.note, row.sku,
    row.material, row.movement, (row.accessories || []).join('・'), row.braceletQty ?? '',
  ]);
  return [header, ...dataRows].map(row => row.map(csvCell).join(',')).join('\r\n');
}

async function marketConfirmCSVImport() {
  const pending = _marketPendingImport;
  if (!pending?.rows?.length) {
    showToast('warning', '相場登録', '登録する明細がありません');
    return { imported: 0, skipped: 0 };
  }

  const validation = _marketValidateDraftRows();
  if (!validation.valid) {
    marketRenderDraftRows();
    const invalidNumbers = [..._marketDraftInvalidIndexes].map(index => index + 1).join('、');
    const error = document.getElementById('marketDraftError');
    if (error) {
      error.textContent = `明細 ${invalidNumbers} の必須項目（ブランド・モデル名・取引価格）と基本情報を確認してください。`;
      error.classList.add('show');
    }
    showToast('warning', '登録明細を確認してください', '未入力の必須項目があります');
    return { imported: 0, staged: pending.rows.length, skipped: pending.skipped || 0, validationError: true };
  }
  pending.rows = validation.rows;

  const button = document.getElementById('marketDraftRegisterButton') || document.getElementById('marketCsvConfirmButton');
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
    marketEntryPage = 1;
    marketRenderEntryTable();
    _marketShowImportSummary(fileName, imported, skipped);
    showToast('success', approvalPending ? '承認申請を送信しました' : '相場登録完了',
      approvalPending ? `${imported}件を管理者の承認待ちにしました` : `${imported}件を登録しました${skipped ? `（${skipped}件スキップ）` : ''}`);
    return { imported, staged: 0, skipped, approvalPending };
  } catch (error) {
    showToast('error', 'CSV登録エラー', error?.message || '相場明細を登録できませんでした');
    throw error;
  } finally {
    if (button) {
      button.disabled = !(_marketPendingImport?.rows?.length);
      button.innerHTML = '<i class="fa-solid fa-floppy-disk"></i> 相場を登録する';
    }
  }
}

function marketConfirmDraftRegistration() {
  return marketConfirmCSVImport();
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
    importDate: ['市場調査日', 'オークション開催日', '開催日', '取り込み日付', '取込日付', '取込日', 'importdate', 'import_date'],
    brand: ['ブランドコード', 'ブランド', 'brand'],
    model: ['モデル', 'モデル名', 'model'],
    ref: ['型番', 'ref', 'reference'],
    material: ['素材コード', '素材', 'material'],
    movement: ['駆動方式コード', '駆動方式', 'ムーブメント', 'movement'],
    condition: ['コンディションコード', 'コンディション', '状態', 'condition'],
    warrantyYearMonth: ['保証年月', '保証年/月', 'warrantyyearmonth', 'warranty_year_month', 'warranty'],
    auctionCode: ['オークションコード', 'auctioncode', 'auction_code'],
    marketPriceJpy: ['取引価格jpy', '取引価格usd', '取引価格hkd', '取引価格', '落札価格jpy', '落札価格usd', '落札価格hkd', '落札価格', '市場価格', '相場価格jpy', 'marketpricejpy', 'market_price_jpy'],
    marketPriceUsdLegacy: ['相場価格usd', 'marketpriceusd', 'market_price_usd'],
    sku: ['sku'],
    accessories: ['付属品コード', '付属品', 'accessories'],
    braceletQty: ['コマ数', 'braceletparts数量', 'braceletquantity', 'bracelet_quantity'],
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

function _marketNormalizeWarrantyYearMonth(value) {
  const normalized = String(value || '').trim().replace(/年|[./]/g, '-').replace(/月/g, '');
  const match = normalized.match(/^(\d{4})-(\d{1,2})$/u);
  if (!match) return '';
  const month = Number(match[2]);
  return month >= 1 && month <= 12 ? `${match[1]}-${String(month).padStart(2, '0')}` : '';
}

function _marketResolveProductSpec(type, value) {
  if (!value) return '';
  return typeof resolveProductSpecCode === 'function' ? resolveProductSpecCode(type, value) : String(value).trim();
}

function _marketParseAccessories(value) {
  if (!value) return [];
  return [...new Set(String(value).split(/[・,，;；|\/]/).map(item => item.trim()).filter(Boolean))];
}

function _marketNormalizeBraceletQuantity(value) {
  if (value == null || String(value).trim() === '') return null;
  const quantity = Math.floor(_marketParseNumber(value));
  return Number.isFinite(quantity) && quantity >= 0 ? quantity : null;
}

function _marketFormatAccessories(accessories = [], braceletQty = null) {
  const values = _marketNormalizeDraftAccessories(accessories);
  if (!values.length) return '—';
  const quantity = _marketNormalizeBraceletQuantity(braceletQty);
  return values.map(name => name === 'BRACELET PARTS' && quantity != null
    ? `${name}（${quantity}コマ）`
    : name).join('・');
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
        onchange="this.parentElement.classList.toggle('checked',this.checked);marketEditAccessoryChanged()"> ${_marketEscape(name)}
    </label>`).join('');
}

function _marketToggleEditBraceletQuantity(show) {
  const row = document.getElementById('me-braceletQtyRow');
  const input = document.getElementById('me-braceletQty');
  if (row) row.hidden = !show;
  if (!show && input) input.value = '';
}

function marketEditAccessoryChanged() {
  _marketToggleEditBraceletQuantity(_marketSelectedAccessories().includes('BRACELET PARTS'));
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
    ['市場調査日', '市場区分', 'ブランドコード', '型番', 'モデル', '素材コード', '駆動方式コード', 'コンディションコード', '保証年月', 'オークションコード', '取引価格', '取引通貨', '市場調査レート', 'SKU', '付属品コード', 'コマ数', '備考'],
    ...rows.map(row => [
      row.importDate, MARKET_CATEGORY_LABELS[_marketNormalizeCategory(row.marketCategory || row.source)], row.brandCode || getBrandCodeByName(row.brand), row.ref || '', row.model,
      row.material || '', row.movement || '', row.condition || '', row.warrantyYearMonth || '',
      row.auctionCode || '', row.marketPrice || 0, row.marketCurrency || 'JPY', _marketFXRate(row), row.sku || '', (row.accessories || []).map(name => (APP_DATA.accessoryRecords || []).find(record => record.name === name || record.code === name)?.code || name).join('・'), row.braceletQty ?? '', row.note || '',
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
  const header = ['ブランドコード', '型番', 'モデル', '素材コード', '駆動方式コード', 'コンディションコード', '保証年月', '取引価格', 'SKU', '付属品コード', 'コマ数', '備考'];
  const brand = (APP_DATA.brandRecords || [])[0] || {};
  const material = (getProductSpecMasterRecords('material') || [])[0] || {};
  const movement = (getProductSpecMasterRecords('movement') || [])[0] || {};
  const condition = (getConditionMasterRecords() || [])[0] || {};
  const accessory = (APP_DATA.accessoryRecords || [])[0] || {};
  const sample = [
    brand.code || '', '116610LN', 'サブマリーナ',
    material.code || '', movement.code || '', condition.code || '', '2026-08', 1200000,
    `MARKET-SAMPLE-${_marketToday().replace(/-/g, '')}-001`, accessory.code || '', '',
    '入力例：必要に応じて変更してください',
  ];
  const rows = [header, sample];
  const csv = rows.map(row => row.map(value => `"${String(value ?? '').replace(/"/g, '""')}"`).join(',')).join('\r\n');
  const anchor = document.createElement('a');
  anchor.href = URL.createObjectURL(new Blob(['\uFEFF' + csv], { type: 'text/csv;charset=utf-8;' }));
  anchor.download = '相場表テンプレート.csv';
  anchor.click();
  showToast('info', '相場表テンプレート', '入力例1行付きの相場表テンプレートをダウンロードしました');
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
