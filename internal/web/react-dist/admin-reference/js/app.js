// =====================================================
// 在庫管理ツール - アプリロジック
// =====================================================

let perfCharts = {};
let dashCharts = {};

/**
 * ブラウザのローカル日付を YYYY-MM-DD で返す。
 * toISOString() は UTC 日付になるため、日本時間の深夜〜午前中に
 * 前日が初期表示されるのを避ける。
 */
function getLocalDateISO(value = new Date()) {
  const year = value.getFullYear();
  const month = String(value.getMonth() + 1).padStart(2, '0');
  const day = String(value.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

// =====================================================
// ① ② ⑤ 入力形式の自動変換 — 完全安定版
// =====================================================
//
// 設計方針
//   ・toHalfWidth / toFullWidth の純粋変換関数
//   ・inputFormatHandler が唯一の変換エントリポイント
//   ・IME 確定前（compositionstart〜compositionend）は変換をスキップ
//   ・変換時はカーソル位置を正確に補正して input.value を上書き
//   ・app.html の oninput/onblur 属性と、
//     initInputFormatListeners() のJS登録、どちらからでも動作する
//
// ─────────────────────────────────────────────────

/**
 * 全角 → 半角 変換
 * 対象: 全角英数字・全角記号 (ＡＢＣ１２３！〜) → (ABC123!~)
 *       全角スペース → 半角スペース
 */
function toHalfWidth(str) {
  return str
    .replace(/\u3000/g, ' ')
    .replace(/[\uFF01-\uFF5E]/g, function (c) {
      return String.fromCharCode(c.charCodeAt(0) - 0xFEE0);
    });
}

/**
 * 半角 → 全角 変換
 * 対象: 半角英数字・半角記号 (!~) → (！〜)
 *       ※日本語（ひらがな・漢字）はすでに全角のためスルー
 */
function toFullWidth(str) {
  return str
    .replace(/[!-~]/g, function (c) {
      return String.fromCharCode(c.charCodeAt(0) + 0xFEE0);
    });
}

/**
 * 変換後の文字列に対してカーソルオフセットを補正する。
 *
 * 全角→半角: 変換点より前の変換文字数だけカーソルが左へ詰まる
 *   例: "ＡＢＣ" → "ABC"  pos=2(cursor after Ｂ) → pos=2 (変化なし、等幅)
 *   ※ UFF01-FF5E の各文字は UTF-16 で 1 code unit = 1 char
 *   → カーソル位置は変換前後で 1:1 対応するため補正不要
 *
 * 半角→全角: 同様に 1:1 対応のため補正不要
 *
 * よって pos はそのまま使用する（将来サロゲートペア対応時は要修正）。
 */
function _fixCursorPos(pos /*, before, after, mode */) {
  return pos;
}

/**
 * ─────────────────────────────────────────────────
 * 共通入力変換ハンドラ（唯一のエントリポイント）
 * ─────────────────────────────────────────────────
 * @param {HTMLInputElement|HTMLTextAreaElement} input 対象フィールド
 * @param {'half'|'full'} mode
 *   'half' … 全角入力を半角に強制変換（SKU / 型番 / シリアル）
 *   'full' … 半角入力を全角に強制変換（モデル名 / ベルト素材 / 文字盤）
 *
 * 呼び出しタイミング:
 *   oninput  → IME確定後の通常入力（1文字ごとリアルタイム）
 *   onblur   → フォーカスアウト時の最終整形
 *   （IME変換中は isComposing フラグで自動スキップ）
 */
function inputFormatHandler(input, mode) {
  // ── IME 変換中はスキップ（CompositionEvent と競合しない）──
  if (input._isComposing) return;

  const before = input.value;
  if (before === '') return;

  const after = (mode === 'half') ? toHalfWidth(before) : toFullWidth(before);

  // 変換が不要なら DOM 書き換えをしない（不要な再描画・カーソルずれを防止）
  if (before === after) return;

  // ── カーソル位置を保存・復元 ──
  let pos = 0;
  try { pos = input.selectionStart || 0; } catch (e) { /* textarea以外 */ }

  input.value = after;

  // 補正後のカーソル位置を設定（範囲外クランプも行う）
  const newPos = Math.min(_fixCursorPos(pos), after.length);
  try { input.setSelectionRange(newPos, newPos); } catch (e) { /* readonly等 */ }
}

// ─────────────────────────────────────────────────
// IME 制御：compositionstart/end を各フィールドに設定
// ─────────────────────────────────────────────────

/**
 * 対象フィールドに IME イベントリスナーを登録する。
 * ・compositionstart: _isComposing = true  → 変換スキップ
 * ・compositionend  : _isComposing = false → 最終確定後に変換を強制実行
 * @param {HTMLInputElement} input
 * @param {'half'|'full'} mode
 */
function _attachImeListeners(input, mode) {
  // 二重登録を防ぐ
  if (input._fmtListenerAttached) return;
  input._fmtListenerAttached = true;
  input._isComposing = false;

  input.addEventListener('compositionstart', function () {
    this._isComposing = true;
  });

  input.addEventListener('compositionend', function () {
    this._isComposing = false;
    // IME 確定直後に変換を強制実行（oninput より後に発火するブラウザ対策）
    inputFormatHandler(this, mode);
  });

  input.addEventListener('input', function () {
    inputFormatHandler(this, mode);
  });

  input.addEventListener('blur', function () {
    // フォーカスアウト時：IME 状態に関係なく最終整形
    this._isComposing = false;
    inputFormatHandler(this, mode);
  });
}

/**
 * ─────────────────────────────────────────────────
 * 仕入登録フォームの全変換対象フィールドに
 * IME リスナーを一括登録する初期化関数。
 * initPurchaseForm() から呼ぶ。
 *
 * 登録方法は 2 通り（両方対応）:
 *   A) id を直接指定（明示的リスト）
 *   B) data-fmt="half"|"full" 属性を持つ要素を自動スキャン
 * ─────────────────────────────────────────────────
 */
function initInputFormatListeners() {
  // ── A) 明示的リスト（id 指定）──
  // 半角変換: SKU / 型番 / シリアル
  [
    document.getElementById('pu-sku'),
    document.getElementById('pu-ref'),
    document.getElementById('pu-serial'),
  ].forEach(function (el) {
    if (el) _attachImeListeners(el, 'half');
  });

  // 全角変換: モデル名 / ベルト素材 / 文字盤
  [
    document.getElementById('pu-model'),
    document.getElementById('pu-belt'),
    document.getElementById('pu-dial'),
  ].forEach(function (el) {
    if (el) _attachImeListeners(el, 'full');
  });

  // ── B) data-fmt 属性スキャン（宣言的登録・将来拡張対応）──
  document.querySelectorAll('[data-fmt="half"]').forEach(function (el) {
    _attachImeListeners(el, 'half');   // 二重登録は _fmtListenerAttached で防止
  });
  document.querySelectorAll('[data-fmt="full"]').forEach(function (el) {
    _attachImeListeners(el, 'full');
  });
}

// 後方互換: oninput/onblur 属性から直接呼ばれる場合もそのまま動作する
function validateHalfAlphanumField(input) { inputFormatHandler(input, 'half'); }
function validateFullWidthField(input)    { inputFormatHandler(input, 'full'); }

// =====================================================
// ⑦ 金額フィールド — 3桁区切りリアルタイムフォーマット
// =====================================================

/**
 * 金額入力補助
 * - 全角数字 → 半角自動変換
 * - 数値以外除去
 * - 3桁区切りカンマ付与
 * - 内部値(data-raw-value)にカンマなし数値を保持
 */
function priceFormatHandler(input) {
  // 全角数字 → 半角
  let raw = input.value.replace(/[０-９]/g, function(c) {
    return String.fromCharCode(c.charCodeAt(0) - 0xFEE0);
  });
  // 数値とカンマ以外を除去
  raw = raw.replace(/[^0-9]/g, '');
  // 先頭の余分な0を除去（0始まり許可は1桁のみ）
  raw = raw.replace(/^0+(\d)/, '$1');

  // 内部値（カンマなし）を保持
  input.dataset.rawValue = raw;

  // 表示値：3桁区切り
  if (raw === '') {
    input.value = '';
  } else {
    input.value = Number(raw).toLocaleString('ja-JP');
  }
}

/**
 * 金額フィールドから内部数値（カンマなし整数）を取得
 */
function getPriceValue(input) {
  if (!input) return 0;
  const raw = String(input.value || '').replace(/[^0-9]/g, '');
  return parseInt(raw, 10) || 0;
}

/**
 * 小数を許可する金額入力補助（相場価格など）
 * - 整数部に3桁区切りカンマを付与
 * - 小数点以下は既定で2桁まで保持
 */
function decimalPriceFormatHandler(input) {
  if (!input) return;
  const decimalScale = Math.max(0, parseInt(input.dataset.decimalScale || '2', 10) || 0);
  let raw = String(input.value || '')
    .replace(/[０-９]/g, c => String.fromCharCode(c.charCodeAt(0) - 0xFEE0))
    .replace(/[．。]/g, '.')
    .replace(/[,，]/g, '')
    .replace(/[^0-9.]/g, '');

  const decimalIndex = raw.indexOf('.');
  const hasDecimalPoint = decimalIndex >= 0;
  let integerPart = (hasDecimalPoint ? raw.slice(0, decimalIndex) : raw).replace(/[^0-9]/g, '');
  let decimalPart = hasDecimalPoint
    ? raw.slice(decimalIndex + 1).replace(/[^0-9]/g, '').slice(0, decimalScale)
    : '';

  if (integerPart === '' && hasDecimalPoint) integerPart = '0';
  integerPart = integerPart.replace(/^0+(\d)/, '$1');
  if (integerPart === '') {
    input.dataset.rawValue = '';
    input.value = '';
    return;
  }

  const canonical = integerPart + (hasDecimalPoint ? `.${decimalPart}` : '');
  input.dataset.rawValue = canonical;
  input.value = Number(integerPart).toLocaleString('ja-JP') + (hasDecimalPoint ? `.${decimalPart}` : '');
}

/** 小数対応の金額フィールドからカンマなし数値を取得 */
function getDecimalPriceValue(input) {
  if (!input) return 0;
  const raw = String(input.value || '').replace(/[,，]/g, '');
  const value = parseFloat(raw);
  return Number.isFinite(value) ? value : 0;
}

/** フィールド下部にエラーメッセージを表示 */
function showFieldError(input, message) {
  let errEl = input.parentElement.querySelector('.field-error-msg');
  if (!errEl) {
    errEl = document.createElement('p');
    errEl.className = 'field-error-msg';
    errEl.style.cssText = 'color:#dc2626;font-size:11px;margin:2px 0 0;';
    input.parentElement.appendChild(errEl);
  }
  errEl.textContent = message;
}

/** フィールドエラーメッセージを除去 */
function clearFieldError(input) {
  const errEl = input.parentElement.querySelector('.field-error-msg');
  if (errEl) errEl.remove();
}

// =====================================================
// ⑤ 仕入情報の引継ぎ — sessionStorage
// =====================================================

/** 仕入日・仕入先 を sessionStorage に保存 */
function savePurchaseCarryover() {
  const d = document.getElementById('pu-date');
  const s = document.getElementById('pu-supplier');
  if (d) sessionStorage.setItem('pu_carryover_date',     d.value);
  if (s) sessionStorage.setItem('pu_carryover_supplier', s.value);
}

/** sessionStorage から仕入日・仕入先を復元（ページロード時） */
function restorePurchaseCarryover() {
  const savedDate     = sessionStorage.getItem('pu_carryover_date');
  const savedSupplier = sessionStorage.getItem('pu_carryover_supplier');
  if (savedDate) {
    const d = document.getElementById('pu-date');
    if (d) { d.value = savedDate; onPurchaseDateChange(savedDate); }
  }
  if (savedSupplier) {
    const s = document.getElementById('pu-supplier');
    if (s) s.value = savedSupplier;
  }
}

/** リセット時のみ引継ぎをクリア */
function clearPurchaseCarryover() {
  sessionStorage.removeItem('pu_carryover_date');
  sessionStorage.removeItem('pu_carryover_supplier');
}

// =====================================================
// 在庫・各種伝票 共通連動
// =====================================================
const BUSINESS_WORKFLOW_STORAGE_KEY = 'inv_business_workflow_v1';
const BUSINESS_WORKFLOW_COLLECTIONS = [
  'inventory', 'purchaseSlips', 'shipments', 'sales', 'salesReturns', 'purchaseReturns',
];

function hydrateBusinessWorkflowState() {
  try {
    const stored = JSON.parse(localStorage.getItem(BUSINESS_WORKFLOW_STORAGE_KEY) || 'null');
    if (!stored || stored.version !== 1) return false;
    BUSINESS_WORKFLOW_COLLECTIONS.forEach(key => {
      if (Array.isArray(stored[key])) APP_DATA[key] = stored[key];
    });
    if (typeof syncPurchaseRequestReservations === 'function') syncPurchaseRequestReservations();
    return true;
  } catch {
    return false;
  }
}

function persistBusinessWorkflowState() {
  try {
    if (typeof synchronizeBrandCodesAcrossData === 'function') synchronizeBrandCodesAcrossData();
    const state = { version: 1, savedAt: new Date().toISOString() };
    BUSINESS_WORKFLOW_COLLECTIONS.forEach(key => { state[key] = APP_DATA[key] || []; });
    localStorage.setItem(BUSINESS_WORKFLOW_STORAGE_KEY, JSON.stringify(state));
    return true;
  } catch {
    return false;
  }
}

/** 在庫の商品基本情報を、同じ商品コードを持つ全伝票明細へ同期する。 */
function syncInventoryItemToDocuments(inventoryItem) {
  if (!inventoryItem?.code) return;
  (APP_DATA.purchaseSlips || []).forEach(slip => {
    (slip.lines || []).forEach(line => {
      if (line.code !== inventoryItem.code) return;
      line.sku = inventoryItem.sku || line.sku || '';
      if (inventoryItem.purchaseSlipId === slip.id || !inventoryItem.purchaseSlipId) {
        line.purchasePrice = Number(inventoryItem.purchasePrice) || 0;
        line.salePrice = Number(inventoryItem.salePrice) || 0;
      }
      line.productDetail = {
        ...(line.productDetail || {}),
        brandCode: inventoryItem.brandCode || getBrandCodeByName(inventoryItem.brand) || '',
        brand: inventoryItem.brand || '', model: inventoryItem.model || '',
        ref: inventoryItem.ref || '', serial: inventoryItem.serial || '',
        material: inventoryItem.material || '', movement: inventoryItem.movement || '',
        condition: inventoryItem.condition || '', accessories: [...(inventoryItem.accessories || [])],
        boxNo: inventoryItem.boxNo ?? null, note: inventoryItem.note || '',
      };
    });
  });
  [APP_DATA.shipments, APP_DATA.sales, APP_DATA.salesReturns, APP_DATA.purchaseReturns].forEach(records => {
    (records || []).forEach(record => (record.items || []).forEach(line => {
      if (line.code !== inventoryItem.code) return;
      line.brandCode = inventoryItem.brandCode || getBrandCodeByName(inventoryItem.brand) || line.brandCode || '';
      line.brand = inventoryItem.brand || line.brand || '';
      line.model = inventoryItem.model || line.model || '';
      line.ref = inventoryItem.ref || line.ref || '';
      line.serial = inventoryItem.serial || line.serial || '';
      line.accessories = [...(inventoryItem.accessories || line.accessories || [])];
    }));
  });
}

/** 仕入伝票を正として、在庫レコードを新規作成または更新する。 */
function syncPurchaseSlipToInventory(slip) {
  if (!slip) return [];
  const affected = [];
  (slip.lines || []).forEach((line, index) => {
    if (!line?.code) return;
    const detail = line.productDetail || {};
    let item = (APP_DATA.inventory || []).find(record => record.code === line.code);
    if (!item) {
      item = { code: line.code, status: '在庫中', revisions: [], images: [] };
      APP_DATA.inventory.push(item);
    }
    Object.assign(item, {
      sku: line.sku || item.sku || '',
      brandCode: detail.brandCode || line.brandCode || getBrandCodeByName(detail.brand) || item.brandCode || '',
      brand: detail.brand || item.brand || '', model: detail.model || item.model || '',
      ref: detail.ref || item.ref || '', serial: detail.serial || item.serial || '',
      material: detail.material || item.material || '', movement: detail.movement || item.movement || '',
      condition: detail.condition || item.condition || '',
      accessories: [...(detail.accessories || item.accessories || [])],
      boxNo: detail.boxNo ?? item.boxNo ?? null,
      note: detail.note || item.note || '', supplier: slip.supplier || item.supplier || '',
      staff: slip.staff || item.staff || '', purchaseDate: slip.date || item.purchaseDate || '',
      purchasePrice: Number(line.purchasePrice) || 0, salePrice: Number(line.salePrice) || 0,
      purchaseSlipId: slip.id, purchaseLineNo: line.lineNo || index + 1,
    });
    affected.push(item);
  });
  return affected;
}

function _setRecordInventoryStatus(record, status) {
  const codes = [];
  (record?.items || []).forEach(recordItem => {
    if (recordItem.returnType) return;
    const inventoryItem = (APP_DATA.inventory || []).find(item => item.code === recordItem.code);
    if (!inventoryItem) return;
    inventoryItem.status = status;
    if (typeof clearInventoryReservationMetadata === 'function') clearInventoryReservationMetadata(inventoryItem);
    codes.push(inventoryItem.code);
  });
  if (['出荷済', '売上済', '仕入返品'].includes(status)
      && typeof unpublishGuestProducts === 'function') unpublishGuestProducts(codes);
  return codes;
}

/** 伝票の確定・承認を在庫と元伝票へ適用する共通処理。 */
function applyBusinessRecordState(type, record) {
  if (!record) return;
  if (type === 'purchase') {
    syncPurchaseSlipToInventory(record);
  } else if (type === 'shipping') {
    _setRecordInventoryStatus(record, '出荷済');
  } else if (type === 'sales') {
    _setRecordInventoryStatus(record, '売上済');
  } else if (type === 'salesreturn') {
    const sale = (APP_DATA.sales || []).find(item => item.id === record.slipId);
    (record.items || []).forEach(returnItem => {
      const saleItem = (sale?.items || []).find(item => item.code === returnItem.code);
      if (saleItem) { saleItem.returnType = 'return'; saleItem.returnStatus = 'pending'; }
      const inventoryItem = (APP_DATA.inventory || []).find(item => item.code === returnItem.code);
      if (inventoryItem) inventoryItem.status = '在庫中';
    });
  } else if (type === 'purchasereturn') {
    _setRecordInventoryStatus(record, '仕入返品');
  }
}

function syncApprovalRequestForBusinessRecord(type, recordId, status, revisionComment = '') {
  const typeMap = {
    purchase: ['purchase', 'purchase_revision'], shipping: ['shipping', 'shipping_revision'],
    sales: ['sales', 'sales_revision'], salesreturn: ['sales_return'], purchasereturn: ['purchase_return'],
  };
  const matchingTypes = typeMap[type] || [];
  const request = (APP_DATA.approvalRequests || []).find(item => {
    if (!matchingTypes.includes(item.type)) return false;
    const detailId = item.detail?.slipId || item.detail?.retId || item.detail?.id || item.detail?.record?.id;
    return detailId === recordId && item.status !== 'approved';
  });
  if (!request) return null;
  request.status = status;
  if (status === 'approved') {
    request.approvedAt = new Date().toLocaleString('ja-JP');
    request.approvedById = typeof currentUserId === 'function' ? currentUserId() : '';
    request.approvedByName = currentUser()?.name || '管理者';
  } else if (status === 'revision') {
    request.revisionComment = revisionComment;
  }
  if (typeof _approvalPersistWorkflowState === 'function') _approvalPersistWorkflowState();
  if (typeof updateApprovalBadge === 'function') updateApprovalBadge();
  return request;
}

function refreshLinkedBusinessViews({ persist = true, source = '' } = {}) {
  if (persist) persistBusinessWorkflowState();
  if (typeof _refreshTaskCounts === 'function') _refreshTaskCounts();
  if (typeof refreshSlipList === 'function' && document.getElementById('slipListBody')) refreshSlipList();
  if (typeof peRenderList === 'function' && document.getElementById('pe-list-tbody')) peRenderList();
  if (document.getElementById('page-dashboard')?.classList.contains('hidden') === false) init_dashboard();
  if (document.getElementById('page-inventory')?.classList.contains('hidden') === false
      && typeof renderInventoryTable === 'function') renderInventoryTable();
  if (document.getElementById('page-returns')?.classList.contains('hidden') === false
      && typeof renderReturnSlipList === 'function') renderReturnSlipList();
  if (document.getElementById('page-approval')?.classList.contains('hidden') === false
      && typeof renderApprovalList === 'function') renderApprovalList();
  if (document.getElementById('page-purchase-list')?.classList.contains('hidden') === false
      && typeof renderPurchaseRequests === 'function') renderPurchaseRequests();
  if (document.getElementById('page-stocktake')?.classList.contains('hidden') === false) {
    if (typeof stkRenderTable === 'function') stkRenderTable();
    if (typeof stkUpdateSummary === 'function') stkUpdateSummary();
    if (typeof stkUpdateProgress === 'function') stkUpdateProgress();
  }
  document.dispatchEvent(new CustomEvent('business-domain-updated', { detail: { source } }));
}

hydrateBusinessWorkflowState();

window.addEventListener('storage', event => {
  if (event.key !== BUSINESS_WORKFLOW_STORAGE_KEY) return;
  hydrateBusinessWorkflowState();
  refreshLinkedBusinessViews({ persist: false, source: 'storage' });
});

// =====================================================
// 初期化
// =====================================================
document.addEventListener('DOMContentLoaded', async function () {
  // REST接続時はGoのHttpOnly Cookie Sessionを正とする。sessionStorageは
  // 画面表示用の利用者情報だけを保持し、認証判定には使わない。
  if (!window.ZaikoAPI && typeof requireLogin === 'function' && !requireLogin()) return;
  if (window.ZaikoAPI) {
    try {
      await window.ZaikoAPI.hydrateAdmin();
    } catch (error) {
      console.error('REST API hydration failed', error);
      if (error?.status === 401) {
        clearSession();
        window.location.href = 'index.html';
        return;
      }
    }
  }
  const todayDate = document.getElementById('todayDate');
  if (todayDate) {
    todayDate.textContent = new Intl.DateTimeFormat('ja-JP', { year: 'numeric', month: 'long', day: 'numeric' }).format(new Date());
  }
  // ログイン切替をまたいだ承認済み操作を画面データへ再適用
  if (typeof replayApprovedApprovalOperations === 'function') replayApprovedApprovalOperations();
  // ロールUIの適用（auth.jsが読み込まれている場合）
  if (typeof applyRoleUI === 'function') applyRoleUI();
  // 通知バッジ初期化
  if (typeof updateNotifyBadge === 'function') updateNotifyBadge();
  // 承認バッジ初期化
  if (typeof updateApprovalBadge === 'function') updateApprovalBadge();

  loadBrandMasterDirectory();
  loadSupplierMasterDirectory();
  loadStaffMasterDirectory();
  loadAccessoryMasterDirectory();
  loadConditionMasterDirectory();
  loadProductSpecMasterDirectory();
  loadClientCompanyDirectory();
  reconcileClientCompanyDirectory({ persist: true });
  if (typeof syncPurchaseRequestPartyCodes === 'function' && syncPurchaseRequestPartyCodes() > 0) {
    persistPurchaseRequests();
  }
  const query = new URLSearchParams(window.location.search);
  const requestedPage = query.get('page')
    || (query.has('requestparty') ? 'purchase-list' : '')
    || (query.has('reservation') ? 'shipping' : '')
    || 'dashboard';
  const initialPage = typeof canAccessPage === 'function' && !canAccessPage(requestedPage)
    ? 'dashboard'
    : requestedPage;
  navigateTo(initialPage);
  refreshGuestRequestAdminUI(false);
  initPurchaseForm();
  initSalesForm();
  initShippingForm();
  initMasterPage();
});

function refreshGuestRequestAdminUI(showIncomingNotice = false) {
  const pending = APP_DATA.purchaseRequests.filter(request => request.status === '未対応');
  const requestBadge = document.getElementById('requestBadge');
  if (requestBadge) {
    requestBadge.textContent = pending.length;
    requestBadge.style.display = pending.length > 0 ? '' : 'none';
  }
  const pendingCount = document.getElementById('pendingCount');
  if (pendingCount) pendingCount.textContent = pending.length;

  const dashboard = document.getElementById('page-dashboard');
  if (dashboard && !dashboard.classList.contains('hidden')) init_dashboard();
  const purchaseList = document.getElementById('page-purchase-list');
  if (purchaseList && !purchaseList.classList.contains('hidden')) renderPurchaseRequests();

  if (showIncomingNotice && (typeof isAdmin !== 'function' || isAdmin())) {
    const latest = pending[0];
    showToast('info', '新しい購入リクエスト', latest
      ? `${latest.guestName}から ${latest.items.length}点の購入リクエストが届きました`
      : 'ゲストから購入リクエストが届きました');
  }
}

document.addEventListener('guest-domain-updated', event => {
  if (event.detail?.key === PURCHASE_REQUESTS_STORAGE_KEY) refreshGuestRequestAdminUI(true);
  if (event.detail?.key === GUEST_SNAPSHOT_STORAGE_KEY && document.getElementById('page-box')?.classList.contains('hidden') === false) {
    renderBoxMatrix();
  }
});

// =====================================================
// ダッシュボード
// =====================================================
const DASHBOARD_MONTH_COUNT = 24;
const DASHBOARD_EXCLUDED_SALE_STATUSES = new Set(['承認待ち', '差戻し', '取消', '取消済', '却下']);
const DASHBOARD_EXCLUDED_PURCHASE_STATUSES = new Set(['承認待ち', '差戻し', '取消', '取消済', '却下']);
let _dashboardSalesCurrency = 'USD';
let _dashboardPurchaseCurrency = 'JPY';
let _dashboardChartCurrency = 'USD';
let _dashboardSupplierMonth = '';
let _dashboardSalesEndMonth = '';
let _dashboardSalesWindow = 6;

function _dashboardMonthKey(value) {
  if (value instanceof Date && Number.isFinite(value.getTime())) {
    return `${value.getFullYear()}-${String(value.getMonth() + 1).padStart(2, '0')}`;
  }
  const match = String(value || '').trim().match(/^(\d{4})[-/](\d{1,2})/);
  return match ? `${match[1]}-${String(Number(match[2])).padStart(2, '0')}` : '';
}

function _dashboardRecentMonths(referenceDate = new Date(), count = DASHBOARD_MONTH_COUNT) {
  const anchor = referenceDate instanceof Date && Number.isFinite(referenceDate.getTime())
    ? referenceDate
    : new Date();
  return Array.from({ length: count }, (_, index) => {
    const date = new Date(anchor.getFullYear(), anchor.getMonth() - (count - 1 - index), 1);
    return {
      key: _dashboardMonthKey(date),
      label: `${date.getMonth() + 1}月`,
    };
  });
}

function _dashboardAmount(value) {
  const amount = Number(value);
  return Number.isFinite(amount) ? amount : 0;
}

function _dashboardMonthLabel(key, compact = false) {
  const match = String(key || '').match(/^(\d{4})-(\d{2})$/);
  if (!match) return String(key || '—');
  return compact ? `${Number(match[2])}月` : `${match[1]}年${Number(match[2])}月`;
}

function _dashboardSaleTotal(sale) {
  const storedJpyTotal = Number(sale?.totalJpy);
  if (Number.isFinite(storedJpyTotal)) return Math.round(storedJpyTotal / getDashboardUsdRate());
  const storedTotal = Number(sale?.total);
  if (Number.isFinite(storedTotal)) {
    return sale?.currency === 'JPY' || sale?.inputCurrency === 'JPY'
      ? Math.round(storedTotal / getDashboardUsdRate())
      : storedTotal;
  }
  return (sale?.items || []).reduce((total, item) => {
    if (!item) return total;
    return total + _dashboardAmount(item.salePrice ?? item.wholesale);
  }, 0);
}

function _isDashboardConfirmedSale(sale) {
  return Boolean(sale && !DASHBOARD_EXCLUDED_SALE_STATUSES.has(String(sale.status || '').trim()));
}

function _isDashboardRecordedPurchase(slip) {
  return Boolean(slip && !DASHBOARD_EXCLUDED_PURCHASE_STATUSES.has(String(slip.status || '').trim()));
}

/** 在庫・実伝票からダッシュボード用の集計値を都度生成する。 */
function getDashboardSummary(referenceDate = new Date()) {
  const months = _dashboardRecentMonths(referenceDate);
  const currentMonthKey = months.at(-1)?.key || _dashboardMonthKey(referenceDate);
  const previousDate = new Date(referenceDate.getFullYear(), referenceDate.getMonth() - 1, 1);
  const previousMonthKey = _dashboardMonthKey(previousDate);
  const salesByMonth = Object.fromEntries(months.map(month => [month.key, 0]));
  if (!Object.prototype.hasOwnProperty.call(salesByMonth, previousMonthKey)) salesByMonth[previousMonthKey] = 0;

  (APP_DATA.sales || []).forEach(sale => {
    if (!_isDashboardConfirmedSale(sale)) return;
    const monthKey = _dashboardMonthKey(sale.date);
    if (Object.prototype.hasOwnProperty.call(salesByMonth, monthKey)) {
      salesByMonth[monthKey] += _dashboardSaleTotal(sale);
    }
  });

  const supplierMonthlyMap = new Map();
  let monthlyPurchaseCount = 0;
  let monthlyPurchaseJpy = 0;
  (APP_DATA.purchaseSlips || []).forEach(slip => {
    if (!_isDashboardRecordedPurchase(slip)) return;
    const slipMonth = _dashboardMonthKey(slip.date);
    if (!Object.prototype.hasOwnProperty.call(salesByMonth, slipMonth)) return;
    const supplierName = getSupplierName(slip.supplier) || slip.supplier || '未設定';
    const supplierKey = `${slipMonth}\u0000${slip.supplier || supplierName}`;
    const supplierRow = supplierMonthlyMap.get(supplierKey) || {
      month: slipMonth, supplierCode: slip.supplier || '', supplierName,
      purchaseJpy: 0, purchaseUsd: 0, units: 0,
    };
    (slip.lines || []).forEach(line => {
      const quantity = Math.max(1, Math.trunc(_dashboardAmount(line?.quantity) || 1));
      const amount = _dashboardAmount(line?.purchasePrice) * quantity;
      supplierRow.units += quantity;
      supplierRow.purchaseJpy += amount;
      if (slipMonth === currentMonthKey) {
        monthlyPurchaseCount += quantity;
        monthlyPurchaseJpy += amount;
      }
    });
    supplierRow.purchaseUsd = Math.round(supplierRow.purchaseJpy / getDashboardUsdRate());
    supplierMonthlyMap.set(supplierKey, supplierRow);
  });

  // PostgreSQL接続時は、確定伝票をサーバー側で集計した値を正とする。
  // 明細データからの集計はオフライン互換表示と仕入先別チャートにだけ使用する。
  const api = APP_DATA.apiDashboard;
  if (api) {
    const apiMonths = Array.isArray(api.monthly) && api.monthly.length > 0 ? api.monthly : [];
    const apiMonth = key => apiMonths.find(month => month.month === key) || {};
    const supplierMonthly = Array.isArray(api.supplierMonthly) ? api.supplierMonthly.map(row => ({
      month: row.month, supplierCode: row.supplierCode || '', supplierName: row.supplierName || row.supplierCode || '未設定',
      purchaseJpy: _dashboardAmount(row.purchaseJpy), purchaseUsd: _dashboardAmount(row.purchaseUsd), units: _dashboardAmount(row.units),
    })) : [];
    const currentSuppliers = supplierMonthly.filter(row => row.month === currentMonthKey);
    return {
      currentMonthKey,
      inventoryCount: _dashboardAmount(api.totalProducts),
      monthlyPurchaseCount: _dashboardAmount(api.purchaseUnits),
      monthlyPurchaseJpy: _dashboardAmount(api.confirmedPurchaseJpy),
      monthlySalesUsd: _dashboardAmount(api.confirmedSalesUsd),
      previousMonthSalesUsd: _dashboardAmount(apiMonth(previousMonthKey).salesUsd),
      supplierLabels: currentSuppliers.map(row => row.supplierName),
      supplierValues: currentSuppliers.map(row => row.purchaseJpy),
      supplierMonthly,
      months: apiMonths.map(month => ({
        month: month.month, label: _dashboardMonthLabel(month.month, true),
        salesJpy: _dashboardAmount(month.salesJpy), salesUsd: _dashboardAmount(month.salesUsd),
        purchaseJpy: _dashboardAmount(month.purchaseJpy), purchaseUsd: _dashboardAmount(month.purchaseUsd),
      })),
      monthLabels: apiMonths.map(month => _dashboardMonthLabel(month.month, true)),
      monthlySalesSeries: apiMonths.map(month => _dashboardAmount(month.salesUsd)),
    };
  }

  const supplierMonthly = [...supplierMonthlyMap.values()];
  const currentSuppliers = supplierMonthly.filter(row => row.month === currentMonthKey);

  return {
    currentMonthKey,
    inventoryCount: (APP_DATA.inventory || []).filter(item => ['在庫中', '取置中'].includes(item.status)).length,
    monthlyPurchaseCount,
    monthlyPurchaseJpy,
    monthlySalesUsd: salesByMonth[currentMonthKey] || 0,
    previousMonthSalesUsd: salesByMonth[previousMonthKey] || 0,
    supplierLabels: currentSuppliers.map(row => row.supplierName),
    supplierValues: currentSuppliers.map(row => row.purchaseJpy),
    supplierMonthly,
    months: months.map(month => ({
      month: month.key, label: month.label,
      salesUsd: salesByMonth[month.key] || 0,
      salesJpy: Math.round((salesByMonth[month.key] || 0) * getDashboardUsdRate()),
      purchaseJpy: supplierMonthly.filter(row => row.month === month.key).reduce((sum, row) => sum + row.purchaseJpy, 0),
      purchaseUsd: supplierMonthly.filter(row => row.month === month.key).reduce((sum, row) => sum + row.purchaseUsd, 0),
    })),
    monthLabels: months.map(month => month.label),
    monthlySalesSeries: months.map(month => salesByMonth[month.key] || 0),
  };
}

function getDashboardUsdRate() {
  if (typeof getInventoryUsdRate === 'function') return getInventoryUsdRate();
  const masterRate = Number((APP_DATA.fxRates || []).find(rate => rate.code === 'USD')?.rate);
  if (Number.isFinite(masterRate) && masterRate > 0) return masterRate;
  return Number(SALE_PRICE_JPY_PER_USD) || 155;
}

function formatDashboardSales(usdAmount, currency = _dashboardSalesCurrency) {
  const value = Number(usdAmount) || 0;
  return currency === 'JPY'
    ? formatPrice(Math.round(value * getDashboardUsdRate()))
    : formatSalePrice(value);
}

function formatDashboardPurchase(jpyAmount, currency = _dashboardPurchaseCurrency) {
  const value = Number(jpyAmount) || 0;
  return currency === 'USD'
    ? formatSalePrice(Math.round(value / getDashboardUsdRate()))
    : formatPrice(value);
}

function _syncDashboardCurrencyUI() {
  const summary = getDashboardSummary();
  const rate = getDashboardUsdRate();
  const rateTitle = `マスタレート: 1 USD = ¥${rate.toLocaleString('ja-JP', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
  const salesLabel = document.getElementById('dashSalesLabel');
  const purchaseLabel = document.getElementById('dashPurchaseLabel');
  const salesAmount = document.getElementById('dashSalesAmount');
  const purchaseAmount = document.getElementById('dashPurchaseAmount');
  const inventoryCount = document.getElementById('dashInventoryCount');
  const monthlyInventoryPurchases = document.getElementById('dashMonthlyInventoryPurchases');
  const purchaseItemCount = document.getElementById('dashPurchaseItemCount');
  const salesComparison = document.getElementById('dashSalesComparison');
  if (salesLabel) salesLabel.textContent = `今月売上（${_dashboardSalesCurrency}）`;
  if (purchaseLabel) purchaseLabel.textContent = `今月仕入金額（${_dashboardPurchaseCurrency}）`;
  if (salesAmount) salesAmount.textContent = formatDashboardSales(summary.monthlySalesUsd);
  if (purchaseAmount) purchaseAmount.textContent = formatDashboardPurchase(summary.monthlyPurchaseJpy);
  if (inventoryCount) inventoryCount.textContent = summary.inventoryCount;
  if (monthlyInventoryPurchases) monthlyInventoryPurchases.textContent = `今月 +${summary.monthlyPurchaseCount}点仕入`;
  if (purchaseItemCount) purchaseItemCount.textContent = `${summary.monthlyPurchaseCount}点仕入`;
  if (salesComparison) {
    const previous = summary.previousMonthSalesUsd;
    const current = summary.monthlySalesUsd;
    salesComparison.classList.remove('up', 'down');
    if (previous > 0) {
      const percentage = Math.round(((current - previous) / previous) * 100);
      if (percentage > 0) salesComparison.classList.add('up');
      if (percentage < 0) salesComparison.classList.add('down');
      salesComparison.textContent = `${percentage > 0 ? '▲' : percentage < 0 ? '▼' : '—'} 前月比 ${percentage > 0 ? '+' : ''}${percentage}%`;
    } else {
      salesComparison.textContent = current > 0 ? '前月実績なし' : '前月比 —';
    }
  }

  [
    ['dash-sales-jpy', _dashboardSalesCurrency === 'JPY'],
    ['dash-sales-usd', _dashboardSalesCurrency === 'USD'],
    ['dash-purchase-jpy', _dashboardPurchaseCurrency === 'JPY'],
    ['dash-purchase-usd', _dashboardPurchaseCurrency === 'USD'],
  ].forEach(([id, active]) => {
    const button = document.getElementById(id);
    if (!button) return;
    button.classList.toggle('active', active);
    button.setAttribute('aria-pressed', active ? 'true' : 'false');
    button.title = rateTitle;
  });

  _applyDashboardSettings();
}

function switchDashboardCurrency(priceType, currency) {
  if (currency !== 'JPY' && currency !== 'USD') return;
  if (priceType === 'sales') _dashboardSalesCurrency = currency;
  else if (priceType === 'purchase') _dashboardPurchaseCurrency = currency;
  else return;
  _syncDashboardCurrencyUI();
}

function init_dashboard() {
  const summary = getDashboardSummary();
  // 最新仕入
  const tbody = document.getElementById('dashRecentInventory');
  const recent = APP_DATA.inventory.slice(-5).reverse();
  tbody.innerHTML = recent.map(item => `
    <tr style="cursor:pointer;" onclick="showItemDetail('${item.code}')">
      <td><code style="font-size:11px;">${item.code}</code></td>
      <td>${item.brand}<br><span style="font-size:11px;color:var(--text-muted);">${item.model}</span></td>
      <td>${formatPrice(item.purchasePrice)}</td>
      <td>${getStatusBadge(item.status)}</td>
    </tr>
  `).join('');

  // 購入リクエスト
  const reqArea = document.getElementById('dashRequests');
  const pending = APP_DATA.purchaseRequests.filter(r => r.status === '未対応');
  const dashboardRequestCount = document.getElementById('dashRequestCount');
  const dashboardRequestStatus = document.getElementById('dashRequestStatus');
  if (dashboardRequestCount) dashboardRequestCount.textContent = pending.length;
  if (dashboardRequestStatus) {
    dashboardRequestStatus.textContent = pending.length > 0 ? '● 未対応あり' : '未対応なし';
    dashboardRequestStatus.style.color = pending.length > 0 ? 'var(--danger)' : 'var(--success)';
  }
  if (pending.length === 0) {
    reqArea.innerHTML = '<p style="text-align:center;color:var(--text-muted);font-size:13px;padding:20px;">未対応のリクエストはありません</p>';
  } else {
    reqArea.innerHTML = pending.map(r => {
      const total = r.items.reduce((s, it) => s + (it.salePrice || 0), 0);
      return `
        <div class="request-card new" style="cursor:pointer;" onclick="navigateTo('purchase-list')">
          <div class="req-icon"><i class="fa-solid fa-cart-shopping" style="font-size:22px;color:var(--primary-light);"></i></div>
          <div class="req-info">
            <h4>${r.guestName} <span style="font-size:11px;font-weight:normal;color:var(--text-muted);">${r.items.length}点</span></h4>
            <div style="font-size:12px;color:var(--text);">${r.items.map(it => it.itemName).join('、')}</div>
            <div class="req-meta">
              <span><i class="fa-regular fa-calendar"></i> ${r.date}</span>
              <span style="font-weight:bold;color:var(--primary);">${formatSalePrice(total)}</span>
              ${r.note ? `<span><i class="fa-regular fa-comment"></i> ${r.note}</span>` : ''}
            </div>
          </div>
          <div class="req-actions">
            <button class="btn btn-success btn-sm" onclick="event.stopPropagation();openPrDetail('${r.id}')"><i class="fa-solid fa-list"></i> 明細</button>
          </div>
        </div>`;
    }).join('');
  }

  // ダッシュボードチャート
  renderDashCharts(summary);

  // KPI通貨とダッシュボード管理設定（目標・予算）を反映
  _syncDashboardCurrencyUI();
}

function _dashboardSortedMonths(summary) {
  return [...(summary?.months || [])]
    .filter(month => /^\d{4}-\d{2}$/.test(String(month?.month || '')))
    .sort((left, right) => left.month.localeCompare(right.month));
}

function _dashboardNormalizeChartState(summary) {
  const months = _dashboardSortedMonths(summary);
  const monthKeys = months.map(month => month.month);
  const defaultMonth = monthKeys.includes(summary?.currentMonthKey)
    ? summary.currentMonthKey
    : (monthKeys.at(-1) || '');
  if (!monthKeys.includes(_dashboardSupplierMonth)) _dashboardSupplierMonth = defaultMonth;
  if (!monthKeys.includes(_dashboardSalesEndMonth)) _dashboardSalesEndMonth = defaultMonth;
  if (![6, 12].includes(Number(_dashboardSalesWindow))) _dashboardSalesWindow = 6;
  return months;
}

function _dashboardSyncMonthSelect(id, months, selectedMonth) {
  const select = document.getElementById(id);
  if (!select) return;
  const currentOptions = Array.from(select.options || []).map(option => option.value).join(',');
  const nextOptions = months.map(month => month.month).join(',');
  if (currentOptions !== nextOptions) {
    select.innerHTML = months.map(month =>
      `<option value="${month.month}">${_dashboardMonthLabel(month.month)}</option>`
    ).join('');
  }
  select.value = selectedMonth;
  select.disabled = months.length === 0;
}

function _dashboardSyncMonthSteps(prevId, nextId, months, selectedMonth) {
  const index = months.findIndex(month => month.month === selectedMonth);
  const prev = document.getElementById(prevId);
  const next = document.getElementById(nextId);
  if (prev) prev.disabled = index <= 0;
  if (next) next.disabled = index < 0 || index >= months.length - 1;
}

function setDashboardSupplierMonth(month) {
  _dashboardSupplierMonth = _dashboardMonthKey(month);
  renderDashCharts(getDashboardSummary());
}

function shiftDashboardSupplierMonth(delta) {
  const summary = getDashboardSummary();
  const months = _dashboardNormalizeChartState(summary);
  const index = months.findIndex(month => month.month === _dashboardSupplierMonth);
  const nextIndex = Math.min(months.length - 1, Math.max(0, index + Number(delta || 0)));
  if (months[nextIndex]) _dashboardSupplierMonth = months[nextIndex].month;
  renderDashCharts(summary);
}

function setDashboardSalesEndMonth(month) {
  _dashboardSalesEndMonth = _dashboardMonthKey(month);
  renderDashCharts(getDashboardSummary());
}

function shiftDashboardSalesEndMonth(delta) {
  const summary = getDashboardSummary();
  const months = _dashboardNormalizeChartState(summary);
  const index = months.findIndex(month => month.month === _dashboardSalesEndMonth);
  const nextIndex = Math.min(months.length - 1, Math.max(0, index + Number(delta || 0)));
  if (months[nextIndex]) _dashboardSalesEndMonth = months[nextIndex].month;
  renderDashCharts(summary);
}

function setDashboardSalesWindow(value) {
  _dashboardSalesWindow = Number(value) === 12 ? 12 : 6;
  renderDashCharts(getDashboardSummary());
}

function switchDashboardChartCurrency(currency) {
  if (currency !== 'JPY' && currency !== 'USD') return;
  _dashboardChartCurrency = currency;
  renderDashCharts(getDashboardSummary());
}

function _dashboardChartAmount(value, currency) {
  return currency === 'JPY' ? formatPrice(value) : formatSalePrice(value);
}

function renderDashCharts(summary = getDashboardSummary()) {
  const months = _dashboardNormalizeChartState(summary);
  _dashboardSyncMonthSelect('dashSupplierMonthSelect', months, _dashboardSupplierMonth);
  _dashboardSyncMonthSelect('dashSalesEndMonthSelect', months, _dashboardSalesEndMonth);
  _dashboardSyncMonthSteps('dashSupplierMonthPrev', 'dashSupplierMonthNext', months, _dashboardSupplierMonth);
  _dashboardSyncMonthSteps('dashSalesMonthPrev', 'dashSalesMonthNext', months, _dashboardSalesEndMonth);

  const windowSelect = document.getElementById('dashSalesWindowSelect');
  if (windowSelect) windowSelect.value = String(_dashboardSalesWindow);
  ['JPY', 'USD'].forEach(currency => {
    const button = document.getElementById(`dash-chart-${currency.toLowerCase()}`);
    if (!button) return;
    const active = _dashboardChartCurrency === currency;
    button.classList.toggle('active', active);
    button.setAttribute('aria-pressed', active ? 'true' : 'false');
  });

  const supplierRows = (summary.supplierMonthly || [])
    .filter(row => row.month === _dashboardSupplierMonth && _dashboardAmount(row.purchaseJpy) > 0)
    .sort((left, right) => right.purchaseJpy - left.purchaseJpy);
  const supplierValues = supplierRows.map(row => _dashboardAmount(row.purchaseJpy));
  const supplierTotal = supplierValues.reduce((total, value) => total + value, 0);
  const supplierUnits = supplierRows.reduce((total, row) => total + _dashboardAmount(row.units), 0);
  const supplierTitle = document.getElementById('dashSupplierChartTitle');
  if (supplierTitle) {
    supplierTitle.innerHTML = `<i class="fa-solid fa-chart-pie"></i> 仕入先別 構成比（${_dashboardMonthLabel(_dashboardSupplierMonth)}）`;
  }
  const supplierSummary = document.getElementById('dashSupplierChartSummary');
  if (supplierSummary) {
    supplierSummary.textContent = supplierTotal > 0
      ? `${supplierRows.length}社・${Math.round(supplierUnits).toLocaleString('ja-JP')}点・合計 ${formatPrice(supplierTotal)}`
      : '確定仕入なし';
  }
  const supplierEmpty = document.getElementById('dashChart1Empty');
  if (supplierEmpty) supplierEmpty.classList.toggle('hidden', supplierTotal > 0);

  const salesEndIndex = months.findIndex(month => month.month === _dashboardSalesEndMonth);
  const salesStartIndex = Math.max(0, salesEndIndex - _dashboardSalesWindow + 1);
  const salesMonths = salesEndIndex >= 0 ? months.slice(salesStartIndex, salesEndIndex + 1) : [];
  const multipleYears = new Set(salesMonths.map(month => month.month.slice(0, 4))).size > 1;
  const salesLabels = salesMonths.map(month => {
    const [year, monthNumber] = month.month.split('-');
    return multipleYears ? `${year.slice(2)}/${Number(monthNumber)}月` : `${Number(monthNumber)}月`;
  });
  const salesValues = salesMonths.map(month => _dashboardAmount(
    _dashboardChartCurrency === 'JPY' ? month.salesJpy : month.salesUsd
  ));
  const salesTotal = salesValues.reduce((total, value) => total + value, 0);
  const salesTitle = document.getElementById('dashSalesChartTitle');
  if (salesTitle) {
    salesTitle.innerHTML = `<i class="fa-solid fa-chart-line"></i> 月別売上推移（${_dashboardChartCurrency}）`;
  }
  const salesEmpty = document.getElementById('dashChart2Empty');
  if (salesEmpty) salesEmpty.classList.toggle('hidden', salesTotal > 0);

  // Chart.jsは補助表示。CDNへ接続できないローカル環境でも、月選択やREST保存を止めない。
  if (typeof window.Chart !== 'function') return;
  const colors = ['#2980b9', '#e67e22', '#27ae60', '#8e44ad', '#e74c3c', '#f39c12', '#16a085', '#34495e'];

  const ctx1 = document.getElementById('dashChart1');
  if (ctx1) {
    if (dashCharts.chart1) dashCharts.chart1.destroy();
    dashCharts.chart1 = new Chart(ctx1, {
      type: 'doughnut',
      data: {
        labels: supplierRows.map(row => row.supplierName),
        datasets: [{ data: supplierValues, backgroundColor: colors, borderWidth: 2 }],
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: {
          legend: { position: 'right', labels: { font: { size: 11 }, boxWidth: 12 } },
          tooltip: {
            callbacks: {
              label: context => {
                const amount = _dashboardAmount(context.raw);
                const percentage = supplierTotal > 0 ? Math.round((amount / supplierTotal) * 1000) / 10 : 0;
                return `${context.label}: ${formatPrice(amount)}（${percentage}%）`;
              },
            },
          },
        },
      },
    });
  }

  const ctx2 = document.getElementById('dashChart2');
  if (ctx2) {
    if (dashCharts.chart2) dashCharts.chart2.destroy();
    dashCharts.chart2 = new Chart(ctx2, {
      type: 'bar',
      data: {
        labels: salesLabels,
        datasets: [{
          label: `売上金額（${_dashboardChartCurrency}）`,
          data: salesValues,
          backgroundColor: 'rgba(41,128,185,0.74)',
          borderColor: '#1f6fa3',
          borderWidth: 1,
          borderRadius: 4,
        }],
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        plugins: {
          legend: { display: false },
          tooltip: { callbacks: { label: context => _dashboardChartAmount(context.raw, _dashboardChartCurrency) } },
        },
        scales: {
          y: {
            beginAtZero: true,
            ticks: {
              callback: value => _dashboardChartCurrency === 'JPY'
                ? `¥${Number(value).toLocaleString('ja-JP')}`
                : `$${Number(value).toLocaleString('en-US')}`,
              font: { size: 10 },
            },
          },
          x: { ticks: { font: { size: 10 } } },
        },
      },
    });
  }
}

// =====================================================
// 在庫一覧
// =====================================================
let inventoryPage = 1;
const ITEMS_PER_PAGE = 10;
let _invStatusSort = 'none';

// 在庫の業務進行順。未定義のステータスは末尾にまとめる。
const INV_STATUS_SORT_ORDER = ['在庫中', '取置中', '仕入返品中', '出荷済', '売上済', '仕入返品', '保留'];

// =====================================================
// 在庫一覧 — 検索・フィルター
// =====================================================

// 検索が一度でも実行されたかのフラグ
let _invSearched = false;

// =====================================================
// 付属品フィルター — ステート管理
// =====================================================

/**
 * 現在選択中の付属品フィルター（完全一致・マスタ値の配列）。
 * 空配列 = 未選択 = 全件表示。
 * @type {string[]}
 */
let _invAccFilterState = [];

const INV_COLUMN_KEYS = [
  'code', 'brand', 'model', 'ref', 'serial', 'supplier', 'staff',
  'purchasePrice', 'salePrice', 'purchaseDate', 'sku', 'accessories',
  'status', 'box', 'qr', 'edit',
];
const INV_COLUMN_WIDTHS = {
  code: 130, brand: 100, model: 110, ref: 100, serial: 90,
  supplier: 90, staff: 80, purchasePrice: 132, salePrice: 132,
  purchaseDate: 90, sku: 90, accessories: 120, status: 80, box: 50, qr: 70, edit: 58,
};
const _invVisibleColumns = new Set(INV_COLUMN_KEYS);
let _invPurchaseCurrency = 'JPY';
let _invSaleCurrency = 'USD';

/** マスタ登録のUSドル円換算レートを返す */
function getInventoryUsdRate() {
  const masterRate = Number((APP_DATA.fxRates || []).find(rate => rate.code === 'USD')?.rate);
  if (Number.isFinite(masterRate) && masterRate > 0) return masterRate;
  return Number(SALE_PRICE_JPY_PER_USD) || 155;
}

/** JPY基準の仕入金額を、選択中の表示通貨で整形する */
function formatInventoryPurchasePrice(jpyAmount, currency = _invPurchaseCurrency) {
  const value = Number(jpyAmount) || 0;
  if (currency === 'USD') return formatSalePrice(Math.round(value / getInventoryUsdRate()));
  return formatPrice(value);
}

/** USD基準の売価を、選択中の表示通貨で整形する */
function formatInventorySalePrice(usdAmount, currency = _invSaleCurrency) {
  const value = Number(usdAmount) || 0;
  if (currency === 'JPY') return formatPrice(Math.round(value * getInventoryUsdRate()));
  return formatSalePrice(value);
}

/** 在庫一覧の価格ヘッダーと通貨ボタンを現在状態へ同期する */
function _invSyncPriceCurrencyUI() {
  const rate = getInventoryUsdRate();
  const rateTitle = `マスタレート: 1 USD = ¥${rate.toLocaleString('ja-JP', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
  const purchaseHeading = document.getElementById('inv-purchase-heading');
  const saleHeading = document.getElementById('inv-sale-heading');
  if (purchaseHeading) purchaseHeading.textContent = `仕入金額（${_invPurchaseCurrency}）`;
  if (saleHeading) saleHeading.textContent = `売価（${_invSaleCurrency}）`;

  [
    ['inv-purchase-jpy', _invPurchaseCurrency === 'JPY'],
    ['inv-purchase-usd', _invPurchaseCurrency === 'USD'],
    ['inv-sale-jpy', _invSaleCurrency === 'JPY'],
    ['inv-sale-usd', _invSaleCurrency === 'USD'],
  ].forEach(([id, active]) => {
    const button = document.getElementById(id);
    if (!button) return;
    button.classList.toggle('active', active);
    button.setAttribute('aria-pressed', active ? 'true' : 'false');
    button.title = rateTitle;
  });

  const note = document.getElementById('inv-column-panel-note');
  if (note) note.textContent = `チェックを外した項目は非表示になります / ${rateTitle}`;
}

/** 仕入金額または売価の表示通貨を切り替える（保存値は変更しない） */
function switchInventoryPriceCurrency(priceType, currency) {
  if (currency !== 'JPY' && currency !== 'USD') return;
  if (priceType === 'purchase') _invPurchaseCurrency = currency;
  else if (priceType === 'sale') _invSaleCurrency = currency;
  else return;

  _invSyncPriceCurrencyUI();
  if (_invSearched) renderInventoryTable();
}

/** 表示項目設定をテーブル・チェックボックスへ反映する */
function _invApplyColumnVisibility() {
  const table = document.getElementById('inventoryTable');
  if (table) {
    const visibleWidth = [..._invVisibleColumns]
      .reduce((sum, key) => sum + (INV_COLUMN_WIDTHS[key] || 80), 12);
    table.style.minWidth = `${Math.max(320, visibleWidth)}px`;
    table.querySelectorAll('[data-inv-col]').forEach(element => {
      element.classList.toggle('inv-col-hidden', !_invVisibleColumns.has(element.dataset.invCol));
    });
    table.querySelectorAll('td[data-inv-empty-row]').forEach(cell => {
      cell.colSpan = Math.max(1, _invVisibleColumns.size);
    });
  }
  _invSyncColumnVisibilityControls();
}

function _invSyncColumnVisibilityControls() {
  document.querySelectorAll('#inv-column-panel input[type="checkbox"]').forEach(checkbox => {
    checkbox.checked = _invVisibleColumns.has(checkbox.value);
  });
  const count = document.getElementById('inv-column-count');
  if (count) count.textContent = `${_invVisibleColumns.size}/${INV_COLUMN_KEYS.length}`;
}

function _invColumnVisibilityChanged(checkbox) {
  const key = checkbox?.value;
  if (!INV_COLUMN_KEYS.includes(key)) return;
  if (checkbox.checked) {
    _invVisibleColumns.add(key);
  } else {
    if (_invVisibleColumns.size <= 1 && _invVisibleColumns.has(key)) {
      checkbox.checked = true;
      showToast('info', '表示項目', '少なくとも1項目は表示してください');
      return;
    }
    _invVisibleColumns.delete(key);
  }
  _invApplyColumnVisibility();
}

function _invShowAllColumns() {
  INV_COLUMN_KEYS.forEach(key => _invVisibleColumns.add(key));
  _invApplyColumnVisibility();
}

function _invToggleColumnMenu(event) {
  event?.stopPropagation();
  const panel = document.getElementById('inv-column-panel');
  const trigger = document.getElementById('inv-column-trigger');
  if (!panel || !trigger) return;
  const shouldOpen = !panel.classList.contains('open');
  _invCloseColumnMenu();
  _invAccClosePanel();
  if (shouldOpen) {
    panel.classList.add('open');
    trigger.setAttribute('aria-expanded', 'true');
  }
}

function _invCloseColumnMenu() {
  const panel = document.getElementById('inv-column-panel');
  const trigger = document.getElementById('inv-column-trigger');
  if (panel) panel.classList.remove('open');
  if (trigger) trigger.setAttribute('aria-expanded', 'false');
}

function init_inventory() {
  // 毎回トップ（白紙状態）に戻す
  _invSearched = false;
  _invAccFilterState = [];       // 付属品選択状態もリセット
  _resetInvUI();
  _buildInvFilterOptions();
  _invAccBuildList();            // 付属品ドロップダウン選択肢を構築
  _invAccRenderTrigger();        // トリガー表示を初期化
  _invSyncPriceCurrencyUI();
  _invSyncStatusSortUI();
  _invApplyColumnVisibility();
}

/** ステータス列の並べ替えを「昇順→降順→解除」の順で切り替える */
function toggleInventoryStatusSort() {
  _invStatusSort = _invStatusSort === 'none'
    ? 'ascending'
    : (_invStatusSort === 'ascending' ? 'descending' : 'none');
  inventoryPage = 1;
  _invSyncStatusSortUI();
  if (_invSearched) renderInventoryTable();
}

/** ステータス列ヘッダーの表示とアクセシビリティ属性を同期する */
function _invSyncStatusSortUI() {
  const heading = document.getElementById('inv-status-sort-th');
  const button = document.getElementById('inv-status-sort-btn');
  const icon = document.getElementById('inv-status-sort-icon');
  if (heading) heading.setAttribute('aria-sort', _invStatusSort);
  if (icon) {
    icon.className = `fa-solid ${_invStatusSort === 'ascending'
      ? 'fa-sort-up'
      : (_invStatusSort === 'descending' ? 'fa-sort-down' : 'fa-sort')}`;
  }
  if (button) {
    const stateLabel = _invStatusSort === 'ascending'
      ? '業務順（在庫中から売上済）'
      : (_invStatusSort === 'descending' ? '業務逆順（売上済から在庫中）' : '元の登録順');
    button.setAttribute('aria-label', `ステータス：${stateLabel}。クリックして並べ替えを切り替える`);
    button.title = `現在：${stateLabel}`;
  }
}

// サイドバーから再クリックされた際もトップに戻す（navigateToでinit_inventoryが呼ばれる）
// → init_inventory が白紙リセットを担当するため追加処理不要

// フィルター選択肢を構築（共通マスタと連動）
function _buildInvFilterOptions() {
  // ブランド
  const brandSel = document.getElementById('inv-f-brand');
  if (brandSel) {
    const current = brandSel.dataset.brandRenameValue || brandSel.value;
    delete brandSel.dataset.brandRenameValue;
    const brands = getBrandMasterNames(APP_DATA.inventory.map(i => i.brand));
    brandSel.innerHTML = '<option value="">すべて</option>' +
      brands.map(b => `<option value="${_mEsc(b)}">${_mEsc(b)}</option>`).join('');
    brandSel.value = brands.includes(current) ? current : '';
  }
  // 仕入先
  const supplierSel = document.getElementById('inv-f-supplier');
  if (supplierSel) {
    const current = supplierSel.dataset.supplierRenameValue || supplierSel.value;
    delete supplierSel.dataset.supplierRenameValue;
    const suppliers = getSupplierMasterRecords(APP_DATA.inventory.map(item => item.supplier));
    supplierSel.innerHTML = '<option value="">すべて</option>' +
      suppliers.map(s => `<option value="${_mEsc(s.code)}">${_mEsc(s.name)}</option>`).join('');
    supplierSel.value = suppliers.some(supplier => supplier.code === current) ? current : '';
  }
  // 担当者
  const staffSel = document.getElementById('inv-f-staff');
  if (staffSel) {
    const current = staffSel.dataset.staffRenameValue || staffSel.value;
    delete staffSel.dataset.staffRenameValue;
    const staffList = getStaffMasterNames(APP_DATA.inventory.map(i => i.staff));
    staffSel.innerHTML = '<option value="">すべて</option>' +
      staffList.map(s => `<option value="${_mEsc(s)}">${_mEsc(s)}</option>`).join('');
    staffSel.value = staffList.includes(current) ? current : '';
  }
  ['material', 'movement'].forEach(type => {
    const select = document.getElementById(`inv-f-${type}`);
    if (!select) return;
    const current = select.dataset.productSpecRenameValue || select.value;
    delete select.dataset.productSpecRenameValue;
    populateProductSpecMasterSelect(`inv-f-${type}`, type, {
      emptyLabel: 'すべて',
      selected: current,
      extraCodes: APP_DATA.inventory.map(item => item[type]),
      labelMode: 'name',
    });
  });
  const conditionSel = document.getElementById('inv-f-condition');
  if (conditionSel) {
    populateConditionMasterSelect('inv-f-condition', {
      emptyLabel: 'すべて',
      selected: conditionSel.value,
      extraCodes: APP_DATA.inventory.map(item => item.condition),
      labelMode: 'name',
    });
  }
  // BOX
  const boxSel = document.getElementById('inv-f-box');
  if (boxSel) {
    const activBoxes = (APP_DATA.boxes || []).filter(b => b.no);
    boxSel.innerHTML = '<option value="">すべて</option>' +
      activBoxes.map(b => `<option value="${b.no}">BOX${b.no}${b.name ? ' — ' + b.name : ''}</option>`).join('');
  }
}

// =====================================================
// 付属品フィルター — ドロップダウン制御
// =====================================================

/**
 * マスタの付属品リストを元にドロップダウンの選択肢（チェックボックス行）を構築する。
 * init_inventory() から呼ばれる。
 */
function _invAccBuildList() {
  const list = document.getElementById('inv-acc-list');
  if (!list) return;
  const accessories = APP_DATA.accessories || [];
  list.innerHTML = accessories.map(acc => `
    <label class="inv-acc-item" id="inv-acc-item-${_invAccIdSafe(acc)}">
      <input type="checkbox" value="${_escInvAcc(acc)}"
        onchange="_invAccOnChange(this)"
        ${_invAccFilterState.includes(acc) ? 'checked' : ''}>
      ${_escInvAcc(acc)}
    </label>`).join('');
  // 既選択のものに .checked クラスを付与
  _invAccSyncItemStyles();
}

/**
 * チェックボックス変更時のハンドラ。
 * ステートを即時更新 → トリガー再描画 → 検索実行。
 * @param {HTMLInputElement} cb
 */
function _invAccOnChange(cb) {
  const val = cb.value;
  if (cb.checked) {
    // 重複追加防止
    if (!_invAccFilterState.includes(val)) {
      _invAccFilterState = [..._invAccFilterState, val];
    }
  } else {
    _invAccFilterState = _invAccFilterState.filter(v => v !== val);
  }
  _invAccSyncItemStyles();
  _invAccRenderTrigger();
  // 検索済み状態なら即時フィルタリング
  if (_invSearched) execInventorySearch();
  else _updateActiveFilterTags();
}

/**
 * 全選択ボタン。
 */
function _invAccSelectAll() {
  const accessories = APP_DATA.accessories || [];
  _invAccFilterState = [...accessories];
  _invAccSyncCheckboxes();
  _invAccSyncItemStyles();
  _invAccRenderTrigger();
  if (_invSearched) execInventorySearch();
  else _updateActiveFilterTags();
}

/**
 * 全解除ボタン。
 */
function _invAccClearAll() {
  _invAccFilterState = [];
  _invAccSyncCheckboxes();
  _invAccSyncItemStyles();
  _invAccRenderTrigger();
  if (_invSearched) execInventorySearch();
  else _updateActiveFilterTags();
}

/**
 * ステートに合わせてチェックボックスの checked を同期する。
 */
function _invAccSyncCheckboxes() {
  const list = document.getElementById('inv-acc-list');
  if (!list) return;
  list.querySelectorAll('input[type="checkbox"]').forEach(cb => {
    cb.checked = _invAccFilterState.includes(cb.value);
  });
}

/**
 * ステートに合わせて .inv-acc-item の .checked クラスを同期する。
 */
function _invAccSyncItemStyles() {
  const list = document.getElementById('inv-acc-list');
  if (!list) return;
  list.querySelectorAll('.inv-acc-item').forEach(item => {
    const cb = item.querySelector('input[type="checkbox"]');
    item.classList.toggle('checked', cb && cb.checked);
  });
}

/**
 * トリガーボタンの表示をステートに合わせて更新する。
 * - 未選択 → プレースホルダー「すべて」
 * - 選択済み → タグ列（+N オーバーフロー対応）
 */
function _invAccRenderTrigger() {
  const placeholder = document.getElementById('inv-acc-placeholder');
  const tagsEl      = document.getElementById('inv-acc-tags');
  const trigger     = document.getElementById('inv-acc-trigger');
  if (!placeholder || !tagsEl || !trigger) return;

  if (_invAccFilterState.length === 0) {
    placeholder.style.display = '';
    tagsEl.style.display = 'none';
    tagsEl.innerHTML = '';
  } else {
    placeholder.style.display = 'none';
    tagsEl.style.display = '';
    // 最大2件表示 + 残りは「+N」
    const MAX_VISIBLE = 2;
    const visible = _invAccFilterState.slice(0, MAX_VISIBLE);
    const rest    = _invAccFilterState.length - MAX_VISIBLE;
    tagsEl.innerHTML =
      visible.map(v => `
        <span class="inv-acc-tag">
          ${_escInvAcc(v)}
          <button type="button" class="inv-acc-tag-close"
            onclick="_invAccRemoveTag(event,'${_escInvAccAttr(v)}')"
            title="${_escInvAccAttr(v)} を解除">✕</button>
        </span>`).join('') +
      (rest > 0 ? `<span class="inv-acc-tag-more">+${rest}</span>` : '');
  }
}

/**
 * トリガー内タグの ✕ 押下で個別解除。
 * @param {MouseEvent} e
 * @param {string}     val
 */
function _invAccRemoveTag(e, val) {
  e.stopPropagation();  // パネル開閉を防ぐ
  _invAccFilterState = _invAccFilterState.filter(v => v !== val);
  _invAccSyncCheckboxes();
  _invAccSyncItemStyles();
  _invAccRenderTrigger();
  if (_invSearched) execInventorySearch();
  else _updateActiveFilterTags();
}

/**
 * ドロップダウンパネルの開閉。
 * @param {MouseEvent} e
 */
function _invAccTogglePanel(e) {
  e.stopPropagation();
  const panel   = document.getElementById('inv-acc-panel');
  const trigger = document.getElementById('inv-acc-trigger');
  if (!panel || !trigger) return;
  const isOpen = panel.classList.contains('open');
  // 他のドロップダウンを閉じる（将来拡張用）
  _invAccClosePanel();
  if (!isOpen) {
    panel.classList.add('open');
    trigger.classList.add('open');
    trigger.setAttribute('aria-expanded', 'true');
  }
}

/**
 * パネルを閉じる。
 */
function _invAccClosePanel() {
  const panel   = document.getElementById('inv-acc-panel');
  const trigger = document.getElementById('inv-acc-trigger');
  if (panel)   panel.classList.remove('open');
  if (trigger) { trigger.classList.remove('open'); trigger.setAttribute('aria-expanded', 'false'); }
}

// パネル外クリックで閉じる
document.addEventListener('click', function(e) {
  const wrap = document.getElementById('inv-acc-dropdown-wrap');
  if (wrap && !wrap.contains(e.target)) {
    _invAccClosePanel();
  }
  const columnWrap = document.getElementById('inv-column-menu-wrap');
  if (columnWrap && !columnWrap.contains(e.target)) {
    _invCloseColumnMenu();
  }
});

document.addEventListener('keydown', function(e) {
  if (e.key !== 'Escape') return;
  _invAccClosePanel();
  _invCloseColumnMenu();
});

// ── ユーティリティ ──────────────────────────────────────────
/** HTML エスケープ（テキストコンテンツ用） */
function _escInvAcc(str) {
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}
/** HTML 属性用エスケープ（シングルクォート含む） */
function _escInvAccAttr(str) {
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/'/g, '&#39;')
    .replace(/"/g, '&quot;');
}
/** 要素IDに使えるようスペースをアンダースコアに変換 */
function _invAccIdSafe(str) {
  return String(str).replace(/\s+/g, '_').replace(/[^a-zA-Z0-9_-]/g, '');
}

// 白紙状態にリセット
function _resetInvUI() {
  document.getElementById('inv-empty-state').style.display = '';
  document.getElementById('inv-result-area').style.display  = 'none';
  const resetBtn = document.getElementById('inv-reset-btn');
  if (resetBtn) resetBtn.style.display = 'none';
  _updateActiveFilterTags();
}

// 検索実行（「検索」ボタンまたは全件表示ボタン）
function execInventorySearch() {
  _invSearched = true;
  inventoryPage = 1;
  document.getElementById('inv-empty-state').style.display = 'none';
  document.getElementById('inv-result-area').style.display  = '';
  const resetBtn = document.getElementById('inv-reset-btn');
  if (resetBtn) resetBtn.style.display = '';
  renderInventoryTable();
  _updateActiveFilterTags();
}

// 条件クリア → 白紙に戻す
function resetInventorySearch() {
  // 全フィルターをリセット
  ['inv-f-code','inv-f-ref','inv-f-serial','inv-f-sku',
   'inv-f-date-from','inv-f-date-to'].forEach(id => {
    const el = document.getElementById(id);
    if (el) el.value = '';
  });
  ['inv-f-brand','inv-f-supplier','inv-f-staff','inv-f-material','inv-f-movement','inv-f-condition','inv-f-status','inv-f-box'].forEach(id => {
    const el = document.getElementById(id);
    if (el) el.selectedIndex = 0;
  });
  // 付属品マルチセレクトをリセット（ステートベース）
  _invAccFilterState = [];
  _invAccSyncCheckboxes();
  _invAccSyncItemStyles();
  _invAccRenderTrigger();
  _invSearched = false;
  _resetInvUI();
}

// フィルター値を収集
function _getInvFilters() {
  return {
    code:        (document.getElementById('inv-f-code')?.value        || '').trim().toLowerCase(),
    brand:        document.getElementById('inv-f-brand')?.value       || '',
    ref:         (document.getElementById('inv-f-ref')?.value         || '').trim().toLowerCase(),
    serial:      (document.getElementById('inv-f-serial')?.value      || '').trim().toLowerCase(),
    sku:         (document.getElementById('inv-f-sku')?.value         || '').trim().toLowerCase(),
    supplier:     document.getElementById('inv-f-supplier')?.value    || '',
    staff:        document.getElementById('inv-f-staff')?.value       || '',
    material:     document.getElementById('inv-f-material')?.value    || '',
    movement:     document.getElementById('inv-f-movement')?.value    || '',
    condition:    document.getElementById('inv-f-condition')?.value   || '',
    status:       document.getElementById('inv-f-status')?.value      || '',
    box:         (document.getElementById('inv-f-box')?.value         || ''),
    // 付属品はステートから取得（配列・完全一致）
    accessories:  _invAccFilterState.slice(),
    dateFrom:     document.getElementById('inv-f-date-from')?.value   || '',
    dateTo:       document.getElementById('inv-f-date-to')?.value     || '',
  };
}

// アクティブフィルタータグを更新
function _updateActiveFilterTags() {
  const container = document.getElementById('inv-active-filters');
  if (!container) return;
  const f = _getInvFilters();
  const tags = [];
  const labelMap = {
    code: '管理番号', brand: 'ブランド', ref: '型番', serial: 'シリアル',
    sku: 'SKU', supplier: '仕入先', staff: '担当者', material: '素材', movement: '駆動方式', condition: 'コンディション', status: 'ステータス',
    box: 'BOX',
    dateFrom: '仕入日(から)', dateTo: '仕入日(まで)',
  };
  // accessories 以外のキーを処理
  Object.entries(f).forEach(([key, val]) => {
    if (key === 'accessories') return;  // 付属品は別処理
    if (!val) return;
    let display = val;
    if (key === 'supplier') display = getSupplierName(val);
    if (key === 'material' || key === 'movement') display = getProductSpecName(key, val);
    if (key === 'condition') display = getConditionName(val);
    if (key === 'box') display = `BOX${val}`;
    tags.push(`
      <span style="display:inline-flex;align-items:center;gap:5px;background:#eff6ff;color:#1d4ed8;
        border:1px solid #bfdbfe;border-radius:12px;padding:2px 10px;font-size:11px;font-weight:600;">
        ${labelMap[key] || key}: ${display}
        <button onclick="_clearInvFilter('${key}')" style="background:none;border:none;cursor:pointer;
          color:#6b7280;font-size:11px;padding:0;line-height:1;margin-left:2px;">✕</button>
      </span>`);
  });
  // 付属品：選択された各値を個別タグとして表示
  f.accessories.forEach(acc => {
    tags.push(`
      <span style="display:inline-flex;align-items:center;gap:5px;background:#eff6ff;color:#1d4ed8;
        border:1px solid #bfdbfe;border-radius:12px;padding:2px 10px;font-size:11px;font-weight:600;">
        付属品: ${_escInvAcc(acc)}
        <button onclick="_clearInvAccFilter('${_escInvAccAttr(acc)}')" style="background:none;border:none;cursor:pointer;
          color:#6b7280;font-size:11px;padding:0;line-height:1;margin-left:2px;">✕</button>
      </span>`);
  });
  container.style.display = tags.length ? 'flex' : 'none';
  container.innerHTML = tags.length ? tags.join('') : '';
}

// 個別フィルタークリア（付属品以外）
function _clearInvFilter(key) {
  const textKeys = ['code','ref','serial','sku','dateFrom','dateTo'];
  const idMap = {
    code:'inv-f-code', ref:'inv-f-ref', serial:'inv-f-serial', sku:'inv-f-sku',
    dateFrom:'inv-f-date-from', dateTo:'inv-f-date-to',
    brand:'inv-f-brand', supplier:'inv-f-supplier', staff:'inv-f-staff', material:'inv-f-material', movement:'inv-f-movement', condition:'inv-f-condition', status:'inv-f-status', box:'inv-f-box',
  };
  const el = document.getElementById(idMap[key]);
  if (!el) return;
  if (textKeys.includes(key)) {
    el.value = '';
  } else {
    el.selectedIndex = 0;
  }
  if (_invSearched) execInventorySearch();
  else _updateActiveFilterTags();
}

// 付属品フィルターの個別クリア（アクティブタグの ✕ から呼ばれる）
function _clearInvAccFilter(val) {
  _invAccFilterState = _invAccFilterState.filter(v => v !== val);
  _invAccSyncCheckboxes();
  _invAccSyncItemStyles();
  _invAccRenderTrigger();
  if (_invSearched) execInventorySearch();
  else _updateActiveFilterTags();
}

// Enterキーで検索実行（各入力欄に設定）
function _invFilterKeydown(e) {
  if (e.key === 'Enter') execInventorySearch();
}

// 後方互換（他箇所から filterInventory() が呼ばれている場合）
function filterInventory() {
  if (_invSearched) execInventorySearch();
}

function renderInventoryTable() {
  if (!_invSearched) return;   // 検索前は描画しない

  const f = _getInvFilters();

  let filtered = APP_DATA.inventory.filter(item => {
    if (f.code     && !item.code.toLowerCase().includes(f.code))           return false;
    if (f.brand    && item.brand !== f.brand)                               return false;
    if (f.ref      && !(item.ref     || '').toLowerCase().includes(f.ref))  return false;
    if (f.serial   && !(item.serial  || '').toLowerCase().includes(f.serial)) return false;
    if (f.sku      && !(item.sku     || '').toLowerCase().includes(f.sku))  return false;
    if (f.supplier && item.supplier !== f.supplier)                         return false;
    if (f.staff    && (item.staff || '') !== f.staff)                       return false;
    if (f.material && (item.material || '') !== f.material)                 return false;
    if (f.movement && (item.movement || '') !== f.movement)                 return false;
    if (f.condition && (item.condition || '') !== f.condition)              return false;
    if (f.status   && item.status !== f.status)                             return false;
    if (f.box      && String(item.boxNo || '') !== f.box)                   return false;
    // 付属品：完全一致 OR 条件（選択値のいずれかが在庫の accessories に含まれていれば通過）
    if (f.accessories.length > 0) {
      const itemAccs = item.accessories || [];
      const matched  = f.accessories.some(sel => itemAccs.includes(sel));
      if (!matched) return false;
    }
    if (f.dateFrom && (item.purchaseDate || '') < f.dateFrom)               return false;
    if (f.dateTo   && (item.purchaseDate || '') > f.dateTo)                 return false;
    return true;
  });

  if (_invStatusSort !== 'none') {
    filtered = filtered
      .map((item, originalIndex) => ({ item, originalIndex }))
      .sort((a, b) => {
        const rankA = INV_STATUS_SORT_ORDER.indexOf(a.item.status);
        const rankB = INV_STATUS_SORT_ORDER.indexOf(b.item.status);
        const safeRankA = rankA === -1 ? INV_STATUS_SORT_ORDER.length : rankA;
        const safeRankB = rankB === -1 ? INV_STATUS_SORT_ORDER.length : rankB;
        const direction = _invStatusSort === 'ascending' ? 1 : -1;
        return (safeRankA - safeRankB) * direction || a.originalIndex - b.originalIndex;
      })
      .map(({ item }) => item);
  }

  const countEl = document.getElementById('inventoryCount');
  if (countEl) countEl.textContent = `${filtered.length} 件`;

  const start = (inventoryPage - 1) * ITEMS_PER_PAGE;
  const paged = filtered.slice(start, start + ITEMS_PER_PAGE);

  const tbody = document.getElementById('inventoryTableBody');
  if (paged.length === 0) {
    tbody.innerHTML = `
      <tr><td data-inv-empty-row="true" colspan="${Math.max(1, _invVisibleColumns.size)}" style="text-align:center;color:var(--text-muted);padding:40px;">
        <i class="fa-solid fa-magnifying-glass" style="font-size:20px;margin-bottom:8px;display:block;opacity:0.4;"></i>
        条件に一致する商品が見つかりません
      </td></tr>`;
  } else {
    tbody.innerHTML = paged.map(item => {
      const skuVal = item.sku || '—';
      // 付属品: 「・」区切り1行、maxwidth固定でCSS省略
      const accText = (item.accessories && item.accessories.length > 0)
        ? item.accessories.join('・')
        : '—';
      return `
        <tr style="cursor:pointer;" onclick="showItemDetail('${item.code}')">
          <td data-inv-col="code"><code style="font-size:11px;color:var(--primary-light);">${item.code}</code></td>
          <td data-inv-col="brand" style="font-weight:500;">${item.brand}</td>
          <td data-inv-col="model">${item.model}</td>
          <td data-inv-col="ref" style="font-size:12px;">${item.ref    || '—'}</td>
          <td data-inv-col="serial" style="font-size:12px;font-family:monospace;">${item.serial  || '—'}</td>
          <td data-inv-col="supplier" style="font-size:12px;">${getSupplierName(item.supplier)}</td>
          <td data-inv-col="staff" style="font-size:12px;">${item.staff  || '—'}</td>
          <td data-inv-col="purchasePrice" style="font-weight:bold;">${formatInventoryPurchasePrice(item.purchasePrice)}</td>
          <td data-inv-col="salePrice" style="font-weight:bold;color:var(--success);">${item.salePrice ? formatInventorySalePrice(item.salePrice) : '—'}</td>
          <td data-inv-col="purchaseDate" style="font-size:12px;white-space:nowrap;">${item.purchaseDate || '—'}</td>
          <td data-inv-col="sku" style="font-size:12px;">${skuVal}</td>
          <td data-inv-col="accessories" style="font-size:11px;max-width:120px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;" title="${accText}">${accText}</td>
          <td data-inv-col="status">${getStatusBadge(item.status)}</td>
          <td data-inv-col="box">${_buildBoxBadge(item.boxNo)}</td>
          <td data-inv-col="qr">
            <button class="btn btn-outline btn-sm inventory-qr-row-btn" style="white-space:nowrap;padding:3px 8px;"
                    onclick="event.stopPropagation();openInventoryQr('${item.code}')"
                    aria-label="管理番号 ${item.code} の棚卸QRを表示" title="棚卸用QRコードを表示">
              <i class="fa-solid fa-qrcode"></i>
            </button>
          </td>
          <td data-inv-col="edit">
            <button class="btn btn-primary btn-sm" style="white-space:nowrap;padding:3px 8px;" onclick="event.stopPropagation();openItemEdit('${item.code}')">
              <i class="fa-solid fa-pen-to-square"></i>
            </button>
          </td>
        </tr>`;
    }).join('');
  }

  _invApplyColumnVisibility();

  const totalPages = Math.ceil(filtered.length / ITEMS_PER_PAGE);
  renderPagination('inventoryPagination', inventoryPage, totalPages, (p) => {
    inventoryPage = p;
    renderInventoryTable();
  });
}

// 在庫一覧 CSV出力
function exportInventoryCSV() {
  const f = _getInvFilters();
  const filtered = APP_DATA.inventory.filter(item => {
    if (f.code     && !item.code.toLowerCase().includes(f.code))              return false;
    if (f.brand    && item.brand !== f.brand)                                  return false;
    if (f.ref      && !(item.ref     || '').toLowerCase().includes(f.ref))     return false;
    if (f.serial   && !(item.serial  || '').toLowerCase().includes(f.serial))  return false;
    if (f.sku      && !(item.sku     || '').toLowerCase().includes(f.sku))     return false;
    if (f.supplier && item.supplier !== f.supplier)                            return false;
    if (f.staff    && (item.staff || '') !== f.staff)                          return false;
    if (f.status   && item.status !== f.status)                                return false;
    if (f.box      && String(item.boxNo || '') !== f.box)                      return false;
    // 付属品：完全一致 OR 条件（選択値のいずれかが在庫の accessories に含まれていれば通過）
    if (f.accessories.length > 0) {
      const itemAccs = item.accessories || [];
      const matched  = f.accessories.some(sel => itemAccs.includes(sel));
      if (!matched) return false;
    }
    if (f.dateFrom && (item.purchaseDate || '') < f.dateFrom)                  return false;
    if (f.dateTo   && (item.purchaseDate || '') > f.dateTo)                    return false;
    return true;
  });
  const rows = [
    ['管理番号','ブランド','モデル','型番','シリアル','SKU','仕入先','担当者','仕入金額（JPY）','売価（USD）','仕入日','付属品','ステータス','BOX'],
    ...filtered.map(i => {
      return [
        i.code, i.brand, i.model, i.ref||'', i.serial||'', i.sku||'',
        getSupplierName(i.supplier), i.staff||'', i.purchasePrice, i.salePrice||'',
        i.purchaseDate||'',
        (i.accessories||[]).join('・'),
        i.status||'', i.boxNo||''
      ];
    })
  ];
  const csv = rows.map(r => r.map(v => `"${String(v).replace(/"/g,'""')}"`).join(',')).join('\n');
  const a = document.createElement('a');
  a.href = URL.createObjectURL(new Blob(['\uFEFF'+csv],{type:'text/csv;charset=utf-8;'}));
  a.download = `inventory_${new Date().toISOString().slice(0,10)}.csv`;
  a.click();
  showToast('success','CSV出力',`${filtered.length}件を出力しました`);
}

function renderPagination(containerId, currentPage, totalPages, callback) {
  const container = document.getElementById(containerId);
  if (!container || totalPages <= 1) { if(container) container.innerHTML = ''; return; }
  // グローバルに登録してonclickから安全に呼び出す（アロー関数toString問題を回避）
  window._paginationCallback = callback;
  let html = '';
  html += `<button class="page-btn" onclick="window._paginationCallback(${currentPage - 1})" ${currentPage === 1 ? 'disabled' : ''}>‹</button>`;
  for (let i = 1; i <= totalPages; i++) {
    html += `<button class="page-btn ${i === currentPage ? 'active' : ''}" onclick="window._paginationCallback(${i})">${i}</button>`;
  }
  html += `<button class="page-btn" onclick="window._paginationCallback(${currentPage + 1})" ${currentPage === totalPages ? 'disabled' : ''}>›</button>`;
  container.innerHTML = html;
}

// =====================================================
// 商品詳細モーダル
// =====================================================
function showItemDetail(code) {
  const item = APP_DATA.inventory.find(i => i.code === code);
  if (!item) return;

  window._itemDetailCurrentCode = item.code;

  document.getElementById('itemDetailTitle').textContent = `${item.code} — ${item.brand} ${item.model}`;
  const mat  = APP_DATA.materials.find(m => m.code === item.material);
  const mov  = APP_DATA.movements.find(m => m.code === item.movement);
  const cond = APP_DATA.conditions.find(c => c.code === item.condition);

  // ── 画像ギャラリー ──
  const imgs = (item.images || []).filter(Boolean);
  const galleryHtml = imgs.length > 0 ? `
    <div class="item-gallery mb-20">
      <div class="item-gallery-main">
        <img id="galleryMain" src="${imgs[0]}" alt="商品画像" onclick="openGalleryLightbox(this.src)">
      </div>
      ${imgs.length > 1 ? `<div class="item-gallery-thumbs">
        ${imgs.map((src, i) => `
          <img src="${src}" class="gallery-thumb ${i===0?'active':''}"
            onclick="switchGalleryMain('${src}',this)" alt="サムネイル${i+1}">
        `).join('')}
      </div>` : ''}
    </div>` : `<div class="mb-20" style="background:#f8fafc;border:1px dashed var(--border);border-radius:8px;padding:20px;text-align:center;color:var(--text-muted);font-size:12px;"><i class="fa-solid fa-image" style="font-size:24px;margin-bottom:6px;display:block;opacity:0.3;"></i>画像未登録</div>`;

  // ── 編集履歴 ──
  const hist = (item.editHistory || []);
  const histHtml = hist.length > 0 ? `
    <div class="item-edit-history">
      <div class="edit-history-title"><i class="fa-solid fa-clock-rotate-left"></i> 編集履歴 <span class="badge-count">${hist.length}件</span></div>
      ${hist.map((h, idx) => `
        <div class="edit-history-entry">
          <div class="edit-history-meta">
            <span class="edit-history-num">#${hist.length - idx}</span>
            <span><i class="fa-solid fa-calendar-days"></i> ${h.editedAt}</span>
            <span><i class="fa-solid fa-user-pen"></i> ${h.editorName}</span>
            ${h.approverName ? `<span><i class="fa-solid fa-user-shield"></i> ${h.approverName}</span>` : ''}
          </div>
          <div class="edit-history-changes">
            ${(h.changes || []).map(c => `
              <div class="edit-history-change">
                <span class="change-field">${c.field}</span>
                <span class="change-before">${c.before || '（空）'}</span>
                <i class="fa-solid fa-arrow-right" style="color:var(--text-muted);font-size:10px;"></i>
                <span class="change-after">${c.after || '（空）'}</span>
              </div>`).join('')}
            ${h.note ? `<div class="edit-history-note"><i class="fa-solid fa-comment-dots"></i> ${h.note}</div>` : ''}
          </div>
        </div>`).join('')}
    </div>` : '';

  const detailInfoHtml = `
    ${galleryHtml}
    <div class="detail-grid mb-20">
      <div class="detail-row"><div class="detail-label">商品コード</div><div class="detail-value"><code>${item.code}</code></div></div>
      <div class="detail-row"><div class="detail-label">ステータス</div><div class="detail-value">${getStatusBadge(item.status)}</div></div>
      <div class="detail-row"><div class="detail-label">ブランド</div><div class="detail-value">${item.brand}</div></div>
      <div class="detail-row"><div class="detail-label">モデル名</div><div class="detail-value">${item.model}</div></div>
      <div class="detail-row"><div class="detail-label">型番（Ref.）</div><div class="detail-value">${item.ref || '—'}</div></div>
      <div class="detail-row"><div class="detail-label">シリアルNo.</div><div class="detail-value">${item.serial || '—'}</div></div>
      <div class="detail-row"><div class="detail-label">素材（本体）</div><div class="detail-value">${mat ? mat.name : '—'}</div></div>
      <div class="detail-row"><div class="detail-label">駆動方式</div><div class="detail-value">${mov ? mov.name : '—'}</div></div>
      <div class="detail-row"><div class="detail-label">コンディション</div><div class="detail-value">${cond ? cond.name : '—'}</div></div>
      <div class="detail-row"><div class="detail-label">ベルト素材</div><div class="detail-value">${item.belt || '—'}</div></div>
      <div class="detail-row"><div class="detail-label">文字盤</div><div class="detail-value">${item.dial || '—'}</div></div>
      <div class="detail-row"><div class="detail-label">仕入先</div><div class="detail-value">${getSupplierName(item.supplier)}</div></div>
      <div class="detail-row"><div class="detail-label">仕入担当者</div><div class="detail-value">${item.staff || '—'}</div></div>
      <div class="detail-row"><div class="detail-label">仕入金額（${_invPurchaseCurrency}）</div><div class="detail-value" style="font-weight:bold;color:var(--primary);">${formatInventoryPurchasePrice(item.purchasePrice)}</div></div>
      <div class="detail-row"><div class="detail-label">売価（${_invSaleCurrency}）</div><div class="detail-value" style="font-weight:bold;color:var(--success);">${item.salePrice ? formatInventorySalePrice(item.salePrice) : '—'}</div></div>
      <div class="detail-row"><div class="detail-label">仕入日</div><div class="detail-value">${item.purchaseDate}</div></div>
      <div class="detail-row"><div class="detail-label">SKU</div><div class="detail-value"><code>${item.sku || '—'}</code></div></div>
      <div class="detail-row"><div class="detail-label">BOX</div><div class="detail-value">${item.boxNo ? `BOX${item.boxNo}` : '—'}</div></div>
    </div>
    <div class="detail-row mb-20">
      <div class="detail-label">付属品</div>
      <div class="detail-value" style="margin-top:4px;">
        ${item.accessories && item.accessories.length > 0 ?
          item.accessories.map(a => `<span class="tag tag-master" style="margin:2px;">${a}</span>`).join('') :
          '<span style="color:var(--text-muted);">なし</span>'}
      </div>
    </div>
    <div class="detail-row mb-20">
      <div class="detail-label">特徴・備考</div>
      <div class="detail-value" style="background:var(--bg);padding:10px;border-radius:6px;margin-top:4px;font-size:12px;">${item.note || '（記載なし）'}</div>
    </div>
    ${item.comment ? `
    <div class="detail-row mb-20" data-internal-only="true">
      <div class="detail-label" style="display:flex;align-items:center;gap:6px;">
        コメント
        <span style="font-size:10px;color:var(--text-muted);background:#f0f2f5;border:1px solid var(--border);border-radius:4px;padding:1px 6px;">
          <i class="fa-solid fa-lock" style="font-size:9px;"></i> 社内専用
        </span>
      </div>
      <div class="detail-value" style="background:#fffef0;border:1px solid #e8e0a0;padding:10px;border-radius:6px;margin-top:4px;font-size:12px;white-space:pre-wrap;">${item.comment}</div>
    </div>` : ''}
    ${histHtml}`;

  let tagPanelHtml = '';
  try {
    tagPanelHtml = typeof renderInventoryProductTagPanel === 'function'
      ? renderInventoryProductTagPanel(item)
      : '<div class="inventory-tag-unavailable">タグ生成機能を読み込めませんでした。</div>';
  } catch (error) {
    tagPanelHtml = `<div class="inventory-tag-unavailable">タグを生成できませんでした：${_escStrHtml(error.message)}</div>`;
  }

  document.getElementById('itemDetailBody').innerHTML = `
    <div class="item-detail-tabs" role="tablist" aria-label="商品詳細の表示切替">
      <button type="button" class="item-detail-tab active" id="itemDetailInfoTab" role="tab"
        aria-selected="true" aria-controls="itemDetailInfoPanel" onclick="switchItemDetailTab('info')">
        <i class="fa-solid fa-circle-info"></i> 商品情報
      </button>
      <button type="button" class="item-detail-tab" id="itemDetailTagTab" role="tab"
        aria-selected="false" aria-controls="itemDetailTagPanel" onclick="switchItemDetailTab('tag')">
        <i class="fa-solid fa-tags"></i> タグ
      </button>
    </div>
    <section id="itemDetailInfoPanel" class="item-detail-tab-panel active" role="tabpanel" aria-labelledby="itemDetailInfoTab">
      ${detailInfoHtml}
    </section>
    <section id="itemDetailTagPanel" class="item-detail-tab-panel" role="tabpanel" aria-labelledby="itemDetailTagTab" hidden>
      ${tagPanelHtml}
    </section>`;

  document.getElementById('itemDetailEditBtn').onclick = () => openItemEdit(code);
  switchItemDetailTab('info');
  document.getElementById('itemDetailModal').classList.remove('hidden');
}

/** 商品詳細ポップアップの商品情報／タグ表示を切り替える。 */
function switchItemDetailTab(tab) {
  const showTag = tab === 'tag';
  const infoTab = document.getElementById('itemDetailInfoTab');
  const tagTab = document.getElementById('itemDetailTagTab');
  const infoPanel = document.getElementById('itemDetailInfoPanel');
  const tagPanel = document.getElementById('itemDetailTagPanel');
  const editButton = document.getElementById('itemDetailEditBtn');
  const downloadButton = document.getElementById('itemDetailTagDownloadBtn');
  const printButton = document.getElementById('itemDetailTagPrintBtn');

  infoTab?.classList.toggle('active', !showTag);
  tagTab?.classList.toggle('active', showTag);
  infoTab?.setAttribute('aria-selected', showTag ? 'false' : 'true');
  tagTab?.setAttribute('aria-selected', showTag ? 'true' : 'false');
  infoPanel?.classList.toggle('active', !showTag);
  tagPanel?.classList.toggle('active', showTag);
  if (infoPanel) infoPanel.hidden = showTag;
  if (tagPanel) tagPanel.hidden = !showTag;
  editButton?.classList.toggle('hidden', showTag);
  downloadButton?.classList.toggle('hidden', !showTag);
  printButton?.classList.toggle('hidden', !showTag);
}

// ── ギャラリー画像切り替え ──
function switchGalleryMain(src, thumbEl) {
  const main = document.getElementById('galleryMain');
  if (main) main.src = src;
  document.querySelectorAll('.gallery-thumb').forEach(t => t.classList.remove('active'));
  if (thumbEl) thumbEl.classList.add('active');
}

// ── ライトボックス（全画面表示） ──
function openGalleryLightbox(src) {
  let lb = document.getElementById('galleryLightbox');
  if (!lb) {
    lb = document.createElement('div');
    lb.id = 'galleryLightbox';
    lb.style.cssText = 'position:fixed;inset:0;background:rgba(0,0,0,0.9);z-index:99999;display:flex;align-items:center;justify-content:center;cursor:zoom-out;';
    lb.onclick = () => lb.remove();
    document.body.appendChild(lb);
  }
  lb.innerHTML = `<img src="${src}" style="max-width:90vw;max-height:90vh;border-radius:8px;box-shadow:0 8px 40px rgba(0,0,0,0.8);">`;
}

// =====================================================
// 商品編集モーダル
// =====================================================
function openItemEdit(code) {
  const item = APP_DATA.inventory.find(i => i.code === code);
  if (!item) return;

  // 詳細モーダルを閉じて編集モーダルを開く
  document.getElementById('itemDetailModal').classList.add('hidden');

  // 各フィールドに現在値をセット
  document.getElementById('itemEditCode').value = code;

  // ブランドドロップダウン
  const brandSel = document.getElementById('ie-brand');
  populateBrandMasterSelect('ie-brand', {
    emptyLabel: null,
    selected: item.brand,
    extraValues: [item.brand],
  });

  // コンディション
  populateConditionMasterSelect('ie-condition', {
    emptyLabel: '-- 選択 --',
    selected: item.condition,
    extraCodes: [item.condition],
    labelMode: 'code-name',
  });

  // 素材・駆動方式（共通マスタ）
  populateProductSpecMasterSelect('ie-material', 'material', {
    emptyLabel: '-- 選択 --', selected: item.material, extraCodes: [item.material], labelMode: 'code-name',
  });
  populateProductSpecMasterSelect('ie-movement', 'movement', {
    emptyLabel: '-- 選択 --', selected: item.movement, extraCodes: [item.movement], labelMode: 'code-name',
  });

  // スタッフ
  populateStaffMasterSelect('ie-staff', {
    emptyLabel: '-- 選択 --',
    selected: item.staff,
    extraNames: [item.staff],
  });

  // 仕入先
  const supSel = document.getElementById('ie-supplier');
  populateSupplierMasterSelect('ie-supplier', {
    emptyLabel: '-- 選択 --',
    selected: item.supplier,
    extraCodes: [item.supplier],
    labelMode: 'code-name',
  });

  // BOX
  const boxSel = document.getElementById('ie-box');
  if (boxSel) {
    const activBoxes = (APP_DATA.boxes || []).filter(b => b.no);
    boxSel.innerHTML = '<option value="">-- なし --</option>' +
      activBoxes.map(b =>
        `<option value="${b.no}"${item.boxNo===b.no?' selected':''}>BOX${b.no}${b.name?' — '+b.name:''}</option>`).join('');
  }

  // テキスト・数値フィールド
  document.getElementById('ie-model').value         = item.model        || '';
  document.getElementById('ie-ref').value           = item.ref          || '';
  document.getElementById('ie-serial').value        = item.serial       || '';
  document.getElementById('ie-belt').value          = item.belt         || '';
  document.getElementById('ie-dial').value          = item.dial         || '';
  const purchasePriceInput = document.getElementById('ie-purchasePrice');
  const salePriceInput = document.getElementById('ie-salePrice');
  purchasePriceInput.value = item.purchasePrice || '';
  salePriceInput.value = item.salePrice || '';
  priceFormatHandler(purchasePriceInput);
  priceFormatHandler(salePriceInput);
  document.getElementById('ie-purchaseDate').value  = item.purchaseDate || '';
  document.getElementById('ie-note').value          = item.note         || '';
  document.getElementById('ie-editNote').value      = '';

  // ステータス
  const stSel = document.getElementById('ie-status');
  [...stSel.options].forEach(o => { o.selected = o.value === item.status; });

  // 付属品チェックボックス
  const accDiv = document.getElementById('ie-accessories');
  accDiv.innerHTML = getAccessoryMasterNames(item.accessories || []).map(a => {
    const checked = (item.accessories||[]).includes(a);
    return `<label class="checkbox-label${checked?' checked':''}">
      <input type="checkbox" value="${_mEsc(a)}" ${checked?'checked':''} onchange="this.parentElement.classList.toggle('checked',this.checked)">
      ${_mEsc(a)}
    </label>`;
  }).join('');

  // 作業者向けバナー表示
  const banner = document.getElementById('itemEditBuyerBanner');
  if (banner) banner.classList.toggle('hidden', !isBuyer());

  document.getElementById('itemEditModal').classList.remove('hidden');
}

function closeItemEdit() {
  document.getElementById('itemEditModal').classList.add('hidden');
}

async function saveItemEdit() {
  const code = document.getElementById('itemEditCode').value;
  const item = APP_DATA.inventory.find(i => i.code === code);
  if (!item) return;

  // フォームから新値を収集
  const newVals = {
    brand:         document.getElementById('ie-brand').value,
    model:         document.getElementById('ie-model').value,
    ref:           document.getElementById('ie-ref').value,
    serial:        document.getElementById('ie-serial').value,
    status:        document.getElementById('ie-status').value,
    condition:     document.getElementById('ie-condition').value,
    material:      document.getElementById('ie-material').value,
    movement:      document.getElementById('ie-movement').value,
    belt:          document.getElementById('ie-belt').value,
    dial:          document.getElementById('ie-dial').value,
    purchasePrice: getPriceValue(document.getElementById('ie-purchasePrice')),
    salePrice:     getPriceValue(document.getElementById('ie-salePrice')),
    purchaseDate:  document.getElementById('ie-purchaseDate').value,
    staff:         document.getElementById('ie-staff').value,
    supplier:      document.getElementById('ie-supplier').value,
    boxNo:         parseInt(document.getElementById('ie-box')?.value) || null,
    accessories:   [...document.querySelectorAll('#ie-accessories input:checked')].map(c=>c.value),
    note:          document.getElementById('ie-note').value,
  };
  const editNote = document.getElementById('ie-editNote').value.trim();

  // 差分検出
  const changes = [];
  const fieldLabels = {
    brand:'ブランド', model:'モデル名', ref:'型番', serial:'シリアルNo.',
    status:'ステータス', condition:'コンディション', material:'素材', movement:'駆動方式',
    belt:'ベルト素材', dial:'文字盤', purchasePrice:'仕入金額', salePrice:'売価（USD）',
    purchaseDate:'仕入日', staff:'仕入担当者', supplier:'仕入先', boxNo:'BOX', note:'備考'
  };
  Object.keys(fieldLabels).forEach(key => {
    const oldV = String(item[key] ?? '');
    const newV = String(newVals[key] ?? '');
    if (oldV !== newV) changes.push({ field: fieldLabels[key], before: oldV || '（空）', after: newV || '（空）' });
  });
  // 付属品は配列比較
  const oldAcc = (item.accessories||[]).slice().sort().join(',');
  const newAcc = newVals.accessories.slice().sort().join(',');
  if (oldAcc !== newAcc) changes.push({ field:'付属品', before: oldAcc||'（なし）', after: newAcc||'（なし）' });

  if (changes.length === 0 && !editNote) {
    showToast('info', '変更なし', '変更された項目がありません');
    return;
  }

  if (window.ZaikoAPI && item.apiManaged) {
    try {
      await window.ZaikoAPI.updateProduct(item, newVals, editNote);
      closeItemEdit();
      renderInventoryTable();
      refreshLinkedBusinessViews({ source: 'inventory-edit-api' });
      showToast('success', '編集完了', `${code} の情報をDBへ保存しました（${changes.length}項目）`);
    } catch (error) {
      showToast('error', '在庫情報を保存できませんでした', error.message || '入力内容を確認してください');
    }
    return;
  }

  // 保存実行
  const execSave = (approverName) => {
    // 値を反映
    Object.assign(item, newVals);

    // 編集履歴を記録
    if (!item.editHistory) item.editHistory = [];
    const now = new Date();
    const editedAt = now.toLocaleString('ja-JP', {year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'}).replace(/\//g,'-');
    item.editHistory.unshift({
      editedAt,
      editorName:   currentUser()?.name || '—',
      approverName: approverName || null,
      changes,
      note: editNote,
    });

    closeItemEdit();
    showToast('success', '編集完了', `${code} の情報を更新しました（${changes.length}項目）`);
    syncInventoryItemToDocuments(item);
    refreshLinkedBusinessViews({ source: 'inventory-edit' });
    renderInventoryTable();
  };

  if (isBuyer()) {
    // 作業者は変更内容を保存せず、管理者へ承認申請する
    requestApproval(
      'item_edit', '商品情報編集',
      {
        code,
        brand: item.brand,
        model: item.model,
        before: Object.fromEntries(Object.keys(newVals).map(key => [key, item[key]])),
        after: _approvalClone(newVals),
        changes: _approvalClone(changes),
      },
      editNote,
      null
    );
    closeItemEdit();
  } else {
    execSave(null);
  }
}

// =====================================================
// 仕入日変更時：コードフィールドには何もしない（採番ボタン方式へ移行）
function onPurchaseDateChange(dateStr) {
  // 自動採番を廃止 — 仕入日変更時はコード欄を一切変更しない
  // （採番ボタン押下時のみコードを生成）
}

/**
 * 採番ボタン：既存在庫・現フォーム伝票と重複しない商品コードを生成して
 * pu-code 入力欄に即時反映する。
 * ※ 採番後は onBlur による在庫検索を実行しない（新規前提）。
 */
function puAssignCode() {
  const dateVal = document.getElementById('pu-date')?.value || '';
  const d = dateVal ? new Date(dateVal) : new Date();
  const ymd = d.getFullYear().toString()
    + String(d.getMonth() + 1).padStart(2, '0')
    + String(d.getDate()).padStart(2, '0');

  // 既存在庫コードを収集
  if (!APP_DATA.itemNumberByDate) APP_DATA.itemNumberByDate = {};
  const usedCodes = new Set((APP_DATA.inventory || []).map(i => i.code));

  // カウンタから重複回避ループ
  let seq = APP_DATA.itemNumberByDate[ymd] || 1;
  let code;
  do {
    code = `${ymd}${String(seq).padStart(3, '0')}`;
    seq++;
  } while (usedCodes.has(code));

  // カウンタを進める（次回採番と重複しないよう）
  APP_DATA.itemNumberByDate[ymd] = seq;

  // 入力欄に反映（既存値は上書き）
  const codeEl = document.getElementById('pu-code');
  if (codeEl) {
    codeEl.value = code;
    codeEl.dataset.assignedByButton = 'true'; // 採番ボタン由来フラグ
  }

  // エラー表示をクリア
  _puCodeSetError('');
}

/**
 * 商品コード入力欄の Enter / onKeyDown ハンドラ
 * Enter キーで在庫検索を実行（手入力時のみ）
 */
function puCodeKeyDown(event) {
  if (event.key === 'Enter') {
    event.preventDefault();
    const codeEl = document.getElementById('pu-code');
    if (codeEl) puCodeSearch(codeEl.value.trim());
  }
}

let _puManagementNumberLookupTimer = null;

/** 管理番号の入力が止まった時点で既存在庫を照合する。 */
function puManagementNumberInput(input) {
  if (!input) return;
  delete input.dataset.assignedByButton;
  if (_puManagementNumberLookupTimer) clearTimeout(_puManagementNumberLookupTimer);

  const code = input.value.trim();
  if (!code) {
    _puClearMatchState();
    _puCodeSetError('');
    return;
  }
  _puManagementNumberLookupTimer = setTimeout(() => {
    if (input.value.trim() === code) puCodeSearch(code);
  }, 300);
}

/**
 * 商品コード入力欄の onBlur ハンドラ
 * ・採番ボタン由来の値のときは在庫検索しない（新規前提）
 * ・手入力のときのみ在庫検索を実行
 */
function puCodeBlur(input) {
  // 採番ボタンで入力された場合はスキップ
  if (input.dataset.assignedByButton === 'true') {
    delete input.dataset.assignedByButton;
    return;
  }
  const code = input.value.trim();
  if (code) puCodeSearch(code);
}

// =====================================================
// 商品登録フォーム 在庫反映・ロック 状態管理
// =====================================================

/**
 * 在庫検索でヒットした商品オブジェクト（null = 未ヒット）
 * @type {object|null}
 */
let _puMatchedItem = null;

/**
 * 在庫データで値が反映されてロックされているフィールドIDのセット
 * @type {Set<string>}
 */
let _puLockedFields = new Set();

// =====================================================
// 商品登録フォーム サブミット前後の状態管理
// =====================================================

/**
 * ボタン押下直前の「引き継ぐべき値」を保持するステートオブジェクト。
 * - 登録する / 不足情報を更新する 両ボタン共通で使用する。
 * - 処理完了後に _puRestoreAfterSubmit() でフォームへ再セットする。
 *
 * @type {{ purchaseDate: string, supplier: string }}
 */
let _puBeforeSubmitState = {
  purchaseDate: '',
  supplier:     '',
};

/**
 * ボタン押下直前に仕入日・仕入先をステートに保存する。
 * savePurchase() の先頭で必ず呼び出すこと。
 *
 * 【設計方針】
 *  - DOM から直接読み取り → ステートへ格納（常に最新の入力値を保持）
 *  - 未入力の場合は空文字のまま保持（削除しない）
 */
function _puCaptureBeforeSubmit() {
  const dateEl     = document.getElementById('pu-date');
  const supplierEl = document.getElementById('pu-supplier');
  _puBeforeSubmitState = {
    purchaseDate: dateEl     ? dateEl.value     : '',
    supplier:     supplierEl ? supplierEl.value : '',
  };
}

/**
 * フォーム全体リセット後に、ステートから仕入日・仕入先を復元する。
 * _puFullResetForm() の直後で必ず呼び出すこと。
 *
 * 【設計方針】
 *  - sessionStorage ではなくメモリ上のステートを参照（リロード不要保証）
 *  - 空文字の場合はセットしない（初期値 = 今日の日付を壊さない）
 */
function _puRestoreAfterSubmit() {
  const { purchaseDate, supplier } = _puBeforeSubmitState;

  const dateEl = document.getElementById('pu-date');
  if (dateEl) {
    // 未入力の場合も含め、ステート値を忠実に復元
    // （空文字は「意図的に未入力」として保持）
    dateEl.value = purchaseDate;
    if (purchaseDate) {
      // 日付変更イベントを発火させて関連 UI を同期
      if (typeof onPurchaseDateChange === 'function') {
        onPurchaseDateChange(purchaseDate);
      }
    }
  }

  const supplierEl = document.getElementById('pu-supplier');
  if (supplierEl) {
    supplierEl.value = supplier;
  }

  // ステートをクリア（使い捨て：次の押下まで保持しない）
  _puBeforeSubmitState = { purchaseDate: '', supplier: '' };
}

/**
 * フォームの全フィールドを初期状態にリセットする（仕入日・仕入先は除く）。
 *
 * 【リセット対象】
 *  商品コード / SKU / モデル名 / 型番 / シリアル / ブランド / 素材 /
 *  駆動方式 / 文字盤 / ベルト素材 / 付属品 / コマ数 / コメント /
 *  価格関連 / 担当者 / BOX / 備考 / 画像 / その他すべて
 *
 * 【保持する項目】
 *  仕入日・仕入先は _puRestoreAfterSubmit() で別途復元するため
 *  ここでは一切触れない。
 *
 * ※ 呼び出し前に _puClearMatchState() でロック解除済みであること
 */
function _puFullResetForm() {
  // ── 商品コード ──────────────────────────────────────────
  const codeEl = document.getElementById('pu-code');
  if (codeEl) {
    codeEl.value = '';
    codeEl.disabled = false;
    codeEl.classList.remove('pu-locked');
    delete codeEl.dataset.assignedByButton;
  }
  _puCodeSetError('');

  // ── 採番ボタン解除 ────────────────────────────────────
  const assignBtn = document.getElementById('pu-code-assign-btn');
  if (assignBtn) { assignBtn.disabled = false; assignBtn.classList.remove('pu-locked'); }

  // ── select 系（仕入日・仕入先を除く） ──────────────────
  ['pu-staff', 'pu-brand', 'pu-material', 'pu-movement', 'pu-condition', 'pu-box'].forEach(id => {
    const el = document.getElementById(id);
    if (el) { el.value = ''; el.disabled = false; el.classList.remove('pu-locked'); }
  });

  // ── 金額（rawValue も含めてクリア） ─────────────────────
  ['pu-price', 'pu-sale-price'].forEach(id => {
    const el = document.getElementById(id);
    if (el) { el.value = ''; el.dataset.rawValue = ''; el.disabled = false; el.classList.remove('pu-locked'); }
  });

  // ── テキスト系（仕入日・仕入先を除くすべて） ────────────
  ['pu-model', 'pu-ref', 'pu-serial', 'pu-sku', 'pu-belt', 'pu-dial', 'pu-note', 'pu-comment'].forEach(id => {
    const el = document.getElementById(id);
    if (el) { el.value = ''; el.disabled = false; el.classList.remove('pu-locked'); }
  });

  // ── 付属品チェックボックス ───────────────────────────────
  document.querySelectorAll('#pu-accessories .checkbox-label').forEach(lbl => {
    lbl.classList.remove('checked');
    const cb = lbl.querySelector('input');
    if (cb) { cb.checked = false; cb.disabled = false; }
  });
  // ロッククラスも除去
  const accArea = document.getElementById('pu-accessories');
  if (accArea) accArea.classList.remove('pu-locked');

  // ── コマ数（BRACELET PARTS 連動）：非表示＋値リセット ────
  _puToggleBraceletQty(false);

  // ── 画像 ─────────────────────────────────────────────────
  uploadedImages.filter(value => value?.startsWith?.('blob:')).forEach(value => URL.revokeObjectURL(value));
  uploadedImages = [];
  uploadedImageFiles = [];
  renderImageGrid();
}

/**
 * 商品コードで在庫を完全一致検索し、
 * ヒット時はフォームに反映してフィールドをロックする。
 * 未ヒット時はロックを解除して何も上書きしない。
 *
 * 【発火タイミング】
 *  - Enter キー押下（puCodeKeyDown）
 *  - onBlur（puCodeBlur）
 *  ※ 採番ボタン由来のときは puCodeBlur でスキップ済み
 *
 * @param {string} code - 商品コード（トリム済み）
 */
function puCodeSearch(code) {
  if (!code) {
    _puClearMatchState();
    return;
  }

  const matched = (APP_DATA.inventory || []).find(i => i.code === code) || null;

  if (matched) {
    // ── ヒット：既存商品 ──────────────────────────────
    _puMatchedItem = matched;
    _puCodeSetError(''); // エラーではなくバナーで案内
    _puShowExistingBanner(code, matched);
    _puApplyInventoryData(matched, true, true);
  } else {
    // ── 未ヒット：新規商品 ────────────────────────────
    _puClearMatchState();
    _puCodeSetError('');
    _puHideExistingBanner();
  }
}

/**
 * ヒット済み商品データをフォームに反映し、値が入ったフィールドをロックする。
 *
 * 【上書きルール】
 *  - 空欄フィールドのみ上書きする（手入力済みは保持）
 *  - 値が反映されたフィールドをロック（disabled + .pu-locked）
 *  - 商品コードは呼び出し元が引数 applyCode で制御（true=必ずセット）
 *
 * @param {object}  item       - APP_DATA.inventory の1件
 * @param {boolean} applyCode  - true のとき商品コードを必ずセット＆ロック
 */
function _puApplyInventoryData(item, applyCode = false, overwriteForm = false) {
  // まず既存ロックを一旦解除してから再適用（再検索対応）
  _puUnlockAllFields();

  // ── 商品コード（applyCode=true のときは必ずセット） ──────
  const codeEl = document.getElementById('pu-code');
  if (codeEl && applyCode && item.code) {
    codeEl.value = item.code;
    _puCodeSetError(''); // コード欄エラーをクリア
    // pu-code 自体をロック
    codeEl.disabled = true;
    codeEl.classList.add('pu-locked');
    _puLockedFields.add('pu-code');
    // 採番ボタンも無効化
    const assignBtn = document.getElementById('pu-code-assign-btn');
    if (assignBtn) {
      assignBtn.disabled = true;
      assignBtn.classList.add('pu-locked');
    }
  }

  // ── テキスト・select 系フィールド定義 ──────────────
  // [ formFieldId, itemProperty ]
  const textFields = [
    ['pu-staff',     'staff'],
    ['pu-supplier',  'supplier'],
    ['pu-sku',       'sku'],
    ['pu-brand',     'brand'],
    ['pu-model',     'model'],
    ['pu-ref',       'ref'],
    ['pu-serial',    'serial'],
    ['pu-material',  'material'],
    ['pu-movement',  'movement'],
    ['pu-condition', 'condition'],
    ['pu-belt',      'belt'],
    ['pu-dial',      'dial'],
    ['pu-note',      'note'],
    ['pu-box',       'boxNo'],
  ];

  textFields.forEach(([id, prop]) => {
    const el = document.getElementById(id);
    if (!el) return;
    const srcVal = item[prop];
    if (srcVal == null || srcVal === '') return; // 在庫側が空なら何もしない
    if (!overwriteForm && el.value !== '' && el.value !== '0') return; // フォーム側が既入力なら保持
    el.value = srcVal;
    _puLockField(id);
  });

  // ── 仕入金額（数値→カンマ整形） ─────────────────────
  const priceEl = document.getElementById('pu-price');
  if (priceEl && (overwriteForm || priceEl.value === '' || priceEl.value === '0') && item.purchasePrice) {
    priceEl.value = Number(item.purchasePrice).toLocaleString('ja-JP');
    priceEl.dataset.rawValue = item.purchasePrice;
    _puLockField('pu-price');
  }

  // ── 売価（数値→カンマ整形） ──────────────────────────
  const salePriceEl = document.getElementById('pu-sale-price');
  if (salePriceEl && (overwriteForm || salePriceEl.value === '' || salePriceEl.value === '0') && item.salePrice) {
    salePriceEl.value = Number(item.salePrice).toLocaleString('ja-JP');
    salePriceEl.dataset.rawValue = item.salePrice;
    _puLockField('pu-sale-price');
  }

  // ── 仕入日 ───────────────────────────────────────────
  const dateEl = document.getElementById('pu-date');
  if (dateEl && (overwriteForm || !dateEl.value) && item.purchaseDate) {
    dateEl.value = item.purchaseDate;
    _puLockField('pu-date');
  }

  // ── 付属品チェックボックス ───────────────────────────
  const accArea = document.getElementById('pu-accessories');
  if (accArea && item.accessories && item.accessories.length > 0) {
    // フォーム側にチェック済みが1つもない場合のみ反映
    const anyChecked = accArea.querySelectorAll('input:checked').length > 0;
    if (overwriteForm || !anyChecked) {
      accArea.querySelectorAll('input[type="checkbox"]').forEach(cb => {
        cb.checked = item.accessories.includes(cb.value);
        cb.closest('label')?.classList.toggle('checked', cb.checked);
      });
      // チェックボックス群をまとめてロック
      _puLockField('pu-accessories');
    }
  }

  // ── BRACELET PARTS コマ数 ────────────────────────────
  // accessories に BRACELET PARTS が含まれ、braceletQty が在庫データにある場合のみ表示＋復元
  const hasBraceletInItem = (item.accessories || []).includes('BRACELET PARTS');
  if (hasBraceletInItem) {
    _puToggleBraceletQty(true);
    const qtyInput = document.getElementById('pu-bracelet-qty');
    if (qtyInput && item.braceletQty != null) {
      qtyInput.value = item.braceletQty;
      // ロック（付属品ロック済みと連動して読み取り専用に）
      qtyInput.disabled = true;
      qtyInput.classList.add('pu-locked');
    }
  } else {
    // BRACELET PARTS なし → コマ数欄を必ず非表示＋リセット
    _puToggleBraceletQty(false);
  }
}

/**
 * 指定フィールドを disabled + .pu-locked でロックする。
 * ロック済みセットに登録。
 *
 * @param {string} id - フィールドの要素 ID
 */
function _puLockField(id) {
  const el = document.getElementById(id);
  if (!el) return;

  if (id === 'pu-accessories') {
    // チェックボックスグループはまとめてロック
    el.querySelectorAll('input[type="checkbox"]').forEach(cb => {
      cb.disabled = true;
    });
    el.classList.add('pu-locked');
  } else {
    el.disabled = true;
    el.classList.add('pu-locked');
  }

  _puLockedFields.add(id);
}

/**
 * ロック済みフィールドをすべて解除する（再検索・リセット時）。
 */
function _puUnlockAllFields() {
  _puLockedFields.forEach(id => {
    const el = document.getElementById(id);
    if (!el) return;

    if (id === 'pu-accessories') {
      el.querySelectorAll('input[type="checkbox"]').forEach(cb => {
        cb.disabled = false;
      });
      el.classList.remove('pu-locked');
    } else {
      el.disabled = false;
      el.classList.remove('pu-locked');
    }
  });
  _puLockedFields.clear();

  // BRACELET PARTS コマ数フィールドも必ずアンロック
  const qtyInput = document.getElementById('pu-bracelet-qty');
  if (qtyInput) {
    qtyInput.disabled = false;
    qtyInput.classList.remove('pu-locked');
  }

  // 採番ボタンも必ず解除（商品コードロック時に無効化されるため）
  const assignBtn = document.getElementById('pu-code-assign-btn');
  if (assignBtn) {
    assignBtn.disabled = false;
    assignBtn.classList.remove('pu-locked');
  }
}

/**
 * 商品登録フォーム：BRACELET PARTS 選択に応じてコマ数欄を表示/非表示する。
 * 非表示時は入力値を必ずリセット（内部状態に残さない）。
 *
 * @param {boolean} show - true: 表示、false: 非表示＋値リセット
 */
function _puToggleBraceletQty(show) {
  const row   = document.getElementById('pu-bracelet-qty-row');
  const input = document.getElementById('pu-bracelet-qty');
  if (!row) return;

  if (show) {
    row.style.display = 'flex';
    // 表示時はフォーカスを当てて入力しやすくする
    if (input) input.focus();
  } else {
    row.style.display = 'none';
    // ⚠ 非表示時は値を必ずリセット（データ不整合防止）
    if (input) {
      input.value = '';
      // dataset にキャッシュがあれば合わせてクリア
      delete input.dataset.rawValue;
    }
  }
}

/**
 * 商品登録フォーム：コマ数入力バリデーション（半角数字・0以上の整数のみ）。
 * oninput から呼ばれる。
 *
 * @param {HTMLInputElement} input
 */
function _puBraceletQtyInput(input) {
  // 半角数字以外を除去
  let v = input.value.replace(/[^0-9]/g, '');
  // 先頭ゼロ複数を除去（"007" → "7"）、ただし "0" 単体は許可
  if (v.length > 1 && v.startsWith('0')) v = v.replace(/^0+/, '') || '0';
  input.value = v;
}

/**
 * 在庫マッチ状態を完全クリアする（未ヒット・リセット時）。
 * ロック解除 + バナー非表示。
 */
function _puClearMatchState() {
  _puMatchedItem = null;
  _puUnlockAllFields();
  _puHideExistingBanner();
}

/**
 * 「既存商品」バナーを表示する。
 * 重複登録を止め、既存商品の確認・編集へ誘導する。
 *
 * @param {string} code  - 管理番号
 * @param {object} item  - 在庫オブジェクト
 */
function _puShowExistingBanner(code, item) {
  const banner = document.getElementById('pu-existing-banner');
  if (!banner) return;
  const label = [item.brand, item.model].filter(Boolean).join(' / ') || '詳細不明';
  const status = item.status || '状態未設定';
  banner.innerHTML = `
    <span class="pu-banner-icon"><i class="fa-solid fa-pen-to-square"></i></span>
    <span class="pu-banner-msg">
      管理番号 <span class="pu-banner-code">${_escStrHtml(code)}</span>
      <strong>（${_escStrHtml(label)}）</strong>
      は既に在庫登録済みです。ステータス：<strong>${_escStrHtml(status)}</strong><br>
      重複登録は行わず、既存商品の確認・編集へ進めます。
    </span>
    <span class="pu-banner-actions">
      <button type="button" class="btn btn-primary btn-sm" onclick="openMatchedInventoryFromProductRegistration()"><i class="fa-solid fa-box-open"></i> 在庫一覧で確認・編集</button>
      <button type="button" class="btn btn-outline btn-sm" onclick="resetMatchedManagementNumber()"><i class="fa-solid fa-rotate-left"></i> 別の管理番号を入力</button>
    </span>`;
  banner.classList.add('visible');
  // 登録ボタンのラベルを「更新」表示に切り替え
  _puSetSaveBtnLabel('update');
}

/** バナーを非表示にする */
function _puHideExistingBanner() {
  const banner = document.getElementById('pu-existing-banner');
  if (banner) banner.classList.remove('visible');
  // バナー非表示と同時に登録ボタンを「新規登録」表示に戻す
  _puSetSaveBtnLabel('new');
}

/**
 * 登録ボタンのラベル・アイコンを切り替える。
 * mode='update' → 「不足情報を更新する」
 * mode='new'    → 「登録する」（デフォルト）
 * @param {'new'|'update'} mode
 */
function _puSetSaveBtnLabel(mode) {
  const btn = document.querySelector('#page-purchase .btn-primary.btn-lg[onclick="savePurchase()"]');
  if (!btn) return;
  if (mode === 'update') {
    if (window.ZaikoAPI) {
      btn.innerHTML = '<i class="fa-solid fa-box-open"></i> 在庫一覧で編集する';
      btn.title = 'この管理番号の既存商品を在庫一覧で開きます';
    } else {
      btn.innerHTML = '<i class="fa-solid fa-floppy-disk"></i> 不足情報を更新する';
      btn.title = '既存在庫の未入力項目のみを更新します（既存データは上書きしません）';
    }
  } else {
    btn.innerHTML = '<i class="fa-solid fa-floppy-disk"></i> 登録する';
    btn.title = '';
  }
}

/** 照合した既存商品を在庫一覧の編集画面で開く。 */
function openMatchedInventoryFromProductRegistration() {
  const code = _puMatchedItem?.code || document.getElementById('pu-code')?.value?.trim();
  const item = (APP_DATA.inventory || []).find(candidate => candidate.code === code);
  if (!item) {
    showToast('error', '在庫情報が見つかりません', '管理番号をもう一度確認してください');
    return;
  }
  navigateTo('inventory');
  setTimeout(() => openItemEdit(item.code), 0);
}

/** 既存商品の照合状態を解除し、別の管理番号を入力できる状態へ戻す。 */
function resetMatchedManagementNumber() {
  _puClearMatchState();
  _puFullResetForm();
  const codeEl = document.getElementById('pu-code');
  if (codeEl) codeEl.focus();
}

// =====================================================
// 【3キー検索】仕入日・仕入先・SKU 一致による自動反映
// =====================================================

/**
 * 仕入日・仕入先・SKU のいずれかが変更されたときに呼ばれるトリガー。
 * 3項目すべてが入力済みの場合のみ検索を実行する。
 * 未入力が1つでもあれば検索しない（禁止事項）。
 *
 * @param {'date'|'supplier'|'sku'} changedField - 変更されたフィールド識別子
 */
function _pu3KeySearchTrigger(changedField) {
  // 変更があったときは必ず既存マッチをリセット
  // （再検索前に古い反映をクリア）
  _puClearMatchState();

  const dateVal     = (document.getElementById('pu-date')?.value     || '').trim();
  const supplierVal = (document.getElementById('pu-supplier')?.value || '').trim();
  const skuVal      = (document.getElementById('pu-sku')?.value      || '').trim();

  // ── ガード：3項目すべて入力済みのときのみ検索 ──────────
  if (!dateVal || !supplierVal || !skuVal) return;

  _pu3KeySearch(dateVal, supplierVal, skuVal);
}

/**
 * 仕入日・仕入先・SKU の完全一致で在庫を検索し、
 * ヒット時はフォームへデータを反映してフィールドをロックする。
 *
 * 完全一致のみ許可（部分一致禁止）。
 * 複数ヒット時は先頭1件を採用（仕様固定）。
 *
 * @param {string} dateVal     - 仕入日（YYYY-MM-DD）
 * @param {string} supplierVal - 仕入先コード
 * @param {string} skuVal      - SKU文字列
 */
function _pu3KeySearch(dateVal, supplierVal, skuVal) {
  const inventory = APP_DATA.inventory || [];

  // 完全一致フィルタ（部分一致禁止）
  const matched = inventory.find(i =>
    i.purchaseDate === dateVal &&
    i.supplier     === supplierVal &&
    i.sku          === skuVal
  ) || null;

  if (!matched) {
    // 未ヒット：何も変更しない（ガード済みのためロック解除は _puClearMatchState で実施済み）
    return;
  }

  // ── ヒット処理 ──────────────────────────────────────
  _puMatchedItem = matched;

  // 商品コードは必ずセット・ロック（applyCode = true）
  _puApplyInventoryData(matched, true);

  // バナーを「3キー検索でヒット」モードで表示
  _puShow3KeyBanner(matched);
}

/**
 * 3キー検索でヒットした際のバナーを表示する。
 *
 * @param {object} item - 在庫オブジェクト
 */
function _puShow3KeyBanner(item) {
  const banner = document.getElementById('pu-existing-banner');
  if (!banner) return;
  const code  = item.code  || '—';
  const label = [item.brand, item.model].filter(Boolean).join(' / ') || '詳細不明';
  banner.innerHTML = `
    <span class="pu-banner-icon"><i class="fa-solid fa-magnifying-glass-check"></i></span>
    <span class="pu-banner-msg">
      仕入日・仕入先・SKU が一致する在庫データが見つかりました。
      管理番号 <span class="pu-banner-code">${_escStrHtml(code)}</span>
      <strong>（${_escStrHtml(label)}）</strong>
      のデータを反映しました。
    </span>`;
  banner.classList.add('visible');
  // 3キー検索ヒットでも「不足情報更新」ボタンに切り替え
  _puSetSaveBtnLabel('update');
}
function _escStrHtml(str) {
  if (str == null) return '';
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

/** 商品コードエラー表示の制御 */
function _puCodeSetError(msg) {
  const el = document.getElementById('pu-code-error');
  if (!el) return;
  if (msg) {
    el.textContent = msg;
    el.style.display = 'block';
  } else {
    el.textContent = '';
    el.style.display = 'none';
  }
}

function initPurchaseForm() {
  // 仕入日（今日）
  const today = getLocalDateISO();
  document.getElementById('pu-date').value = today;
  // 商品コード：採番ボタン方式に変更 — 初期値は空欄
  const puCodeEl = document.getElementById('pu-code');
  if (puCodeEl) puCodeEl.value = '';

  // マスタ選択肢
  populateStaffMasterSelect('pu-staff', { emptyLabel: '-- 選択 --' });

  const supplierSel = document.getElementById('pu-supplier');
  populateSupplierMasterSelect('pu-supplier', {
    emptyLabel: '-- 選択 --',
    labelMode: 'code-name',
  });

  const brandSel = document.getElementById('pu-brand');
  populateBrandMasterSelect('pu-brand', { emptyLabel: '-- 選択 --' });

  populateProductSpecMasterSelect('pu-material', 'material', { emptyLabel: '-- 選択 --', labelMode: 'code-name' });
  populateProductSpecMasterSelect('pu-movement', 'movement', { emptyLabel: '-- 選択 --', labelMode: 'code-name' });

  populateConditionMasterSelect('pu-condition', {
    emptyLabel: '-- 選択 --',
    labelMode: 'code-name',
  });

  // ② BOXプルダウン（商品詳細セクション）
  const boxSel = document.getElementById('pu-box');
  if (boxSel) {
    if (APP_DATA.boxes && APP_DATA.boxes.length > 0) {
      APP_DATA.boxes.forEach(box => {
        const label = box.name ? `BOX${box.no} — ${box.name}` : `BOX${box.no}`;
        boxSel.innerHTML += `<option value="${box.no}">${label}</option>`;
      });
    } else {
      // APP_DATA.boxes 未定義時はBOX1〜10を固定表示
      for (let i = 1; i <= 10; i++) {
        boxSel.innerHTML += `<option value="${i}">BOX${i}</option>`;
      }
    }
  }

  // 付属品チェックボックス
  const accArea = document.getElementById('pu-accessories');
  accArea.innerHTML = '';
  APP_DATA.accessories.forEach(a => {
    const lbl = document.createElement('label');
    lbl.className = 'checkbox-label';
    lbl.innerHTML = `<input type="checkbox" value="${_mEsc(a)}"> ${_mEsc(a)}`;
    const cb = lbl.querySelector('input');
    cb.addEventListener('change', function () {
      lbl.classList.toggle('checked', this.checked);
      // BRACELET PARTS チェック変更時：コマ数欄をリアルタイム制御
      if (this.value === 'BRACELET PARTS') {
        _puToggleBraceletQty(this.checked);
      }
    });
    accArea.appendChild(lbl);
  });

  // 画像グリッド
  renderImageGrid();

  // ⑤ 仕入日・仕入先の引継ぎ復元（リセットなしの場合）
  restorePurchaseCarryover();

  // ③ ⑤ 変換対象フィールドに IME 対応リスナーを一括登録
  initInputFormatListeners();
}

function init_purchase() {
  populateBrandMasterSelect('pu-brand', {
    emptyLabel: '-- 選択 --',
    selected: document.getElementById('pu-brand')?.value || '',
  });
  populateSupplierMasterSelect('pu-supplier', {
    emptyLabel: '-- 選択 --',
    selected: document.getElementById('pu-supplier')?.value || '',
    labelMode: 'code-name',
  });
  populateStaffMasterSelect('pu-staff', {
    emptyLabel: '-- 選択 --',
    selected: document.getElementById('pu-staff')?.value || '',
  });
  populateProductSpecMasterSelect('pu-material', 'material', {
    emptyLabel: '-- 選択 --', selected: document.getElementById('pu-material')?.value || '', labelMode: 'code-name',
  });
  populateProductSpecMasterSelect('pu-movement', 'movement', {
    emptyLabel: '-- 選択 --', selected: document.getElementById('pu-movement')?.value || '', labelMode: 'code-name',
  });
  // 商品コード欄をクリア（採番ボタン方式）
  const puCodeEl = document.getElementById('pu-code');
  if (puCodeEl) puCodeEl.value = '';
  _puCodeSetError('');
}

let uploadedImages = [];
let uploadedImageFiles = [];

function renderImageGrid() {
  const grid = document.getElementById('imageGrid');
  grid.innerHTML = '';
  for (let i = 0; i < 10; i++) {
    const slot = document.createElement('div');
    slot.className = 'image-slot' + (uploadedImages[i] ? ' filled' : '');
    slot.dataset.index = i;
    if (uploadedImages[i]) {
      slot.innerHTML = `<img src="${uploadedImages[i]}" alt="商品画像${i+1}">
        <button class="del-btn" onclick="removeImage(${i}, event)"><i class="fa-solid fa-xmark"></i></button>`;
    } else {
      slot.innerHTML = `<span style="font-size:10px;color:var(--text-muted);">＋ 画像${i+1}</span>`;
      slot.onclick = () => triggerImageUpload(i);
    }
    grid.appendChild(slot);
  }
}

function triggerImageUpload(index) {
  const input = document.createElement('input');
  input.type = 'file';
  input.accept = 'image/jpeg,image/png,image/webp';
  input.onchange = () => {
    const file = input.files?.[0];
    if (!file) return;
    if (file.size > 10 * 1024 * 1024) {
      showToast('error', '画像サイズエラー', '画像は10MB以下にしてください');
      return;
    }
    if (uploadedImages[index]?.startsWith('blob:')) URL.revokeObjectURL(uploadedImages[index]);
    uploadedImageFiles[index] = file;
    uploadedImages[index] = URL.createObjectURL(file);
    renderImageGrid();
    showToast('success', '画像追加', `画像${index + 1}を選択しました`);
  };
  input.click();
}

function removeImage(index, clickEvent) {
  if (clickEvent) clickEvent.stopPropagation();
  if (uploadedImages[index]?.startsWith('blob:')) URL.revokeObjectURL(uploadedImages[index]);
  uploadedImages[index] = null;
  uploadedImageFiles[index] = null;
  renderImageGrid();
}

// unlockEdit / exitEditMode は廃止（PW認証削除済み）
function unlockEdit() {}
function exitEditMode() {}

function resetPurchaseForm() {
  // 在庫反映ロックをすべて解除してからフォームをリセット
  _puClearMatchState();

  // フォーム全フィールドをリセット（仕入日・仕入先含む）
  _puFullResetForm();

  // ── リセットボタン専用：仕入日・仕入先も明示的にクリア ──
  // （_puFullResetForm は仕入日・仕入先を触らないため、ここで追加クリア）
  const dateEl     = document.getElementById('pu-date');
  const supplierEl = document.getElementById('pu-supplier');
  if (dateEl)     { dateEl.value = getLocalDateISO(); }
  if (supplierEl) { supplierEl.value = ''; }

  // sessionStorage の引継ぎ値もクリア
  clearPurchaseCarryover();

  showToast('info', 'リセット', 'フォームをリセットしました');
}

async function savePurchase() {
  // ── ステート保存（最重要：処理の最初に仕入日・仕入先を確保）──
  _puCaptureBeforeSubmit();

  const dateVal  = document.getElementById('pu-date').value;
  const supplier = document.getElementById('pu-supplier').value;
  const brand    = document.getElementById('pu-brand').value;
  const code     = document.getElementById('pu-code').value.trim();

  if (!code) {
    showToast('error', '入力エラー', '管理番号を入力または採番してください');
    _puCodeSetError('管理番号を入力または採番してください');
    document.getElementById('pu-code')?.focus();
    return;
  }

  const existingItem = (APP_DATA.inventory || []).find(i => i.code === code);
  if (window.ZaikoAPI && existingItem) {
    openMatchedInventoryFromProductRegistration();
    return;
  }

  // ── 必須バリデーション ──────────────────────────────────
  const purchasePrice = getPriceValue(document.getElementById('pu-price'));
  const requiresMasterBrand = Boolean(window.ZaikoAPI);
  if (!dateVal || !supplier || (requiresMasterBrand && !brand) || purchasePrice === 0) {
    showToast('error', '入力エラー', '仕入日・仕入先・ブランド・仕入金額は必須です');
    return;
  }

  // ── 入力変換：保存直前に最終変換をかける ──────────────
  ['pu-ref','pu-serial','pu-sku'].forEach(id => {
    const el = document.getElementById(id);
    if (el) inputFormatHandler(el, 'half');
  });
  ['pu-model','pu-belt','pu-dial'].forEach(id => {
    const el = document.getElementById(id);
    if (el) inputFormatHandler(el, 'full');
  });
  ['pu-price','pu-sale-price'].forEach(id => {
    const el = document.getElementById(id);
    if (el) priceFormatHandler(el);
  });

  _puCodeSetError('');

  // フォームは在庫登録処理の中でリセットされるため、先に伝票用の値を退避する。
  const singlePurchaseSlip = _puBuildSinglePurchaseSlip(code, dateVal, supplier, purchasePrice);

  if (window.ZaikoAPI) {
    const brandCode = APP_DATA.brandRecords?.find(item => item.name === brand)?.code || brand;
    const staffName = document.getElementById('pu-staff')?.value || '';
    const staffCode = APP_DATA.staffRecords?.find(item => item.name === staffName)?.code || '';
    const accessoryNames = [...document.querySelectorAll('#pu-accessories input:checked')].map(item => item.value);
    const accessoryCodes = accessoryNames.map(name =>
      (APP_DATA.accessoryRecords || []).find(record => record.name === name)?.code || name);
    try {
      const result = await window.ZaikoAPI.createSingleProduct({
        supplierCode: supplier, staffCode, purchaseDate: dateVal,
        sku: document.getElementById('pu-sku')?.value || '', brandCode,
        modelNumber: document.getElementById('pu-model')?.value || '',
        referenceNumber: document.getElementById('pu-ref')?.value || '',
        serialNumber: document.getElementById('pu-serial')?.value || '', productType: '腕時計',
        materialCode: document.getElementById('pu-material')?.value || '',
        movementCode: document.getElementById('pu-movement')?.value || '',
        conditionCode: document.getElementById('pu-condition')?.value || '', accessoryCodes,
        beltText: document.getElementById('pu-belt')?.value || '',
        dialText: document.getElementById('pu-dial')?.value || '',
        braceletQuantity: Number(document.getElementById('pu-bracelet-qty')?.value) || null,
        costAmountMinor: purchasePrice, costCurrency: 'JPY',
        baseSalePriceMinor: getPriceValue(document.getElementById('pu-sale-price')), baseSaleCurrency: 'USD',
        notes: [document.getElementById('pu-note')?.value, document.getElementById('pu-comment')?.value].filter(Boolean).join('\n'),
      }, uploadedImageFiles);
      savePurchaseCarryover();
      _puClearMatchState(); _puFullResetForm(); _puRestoreAfterSubmit();
      showToast('success', '仕入・商品登録完了', `${result.product.productCode} / ${result.purchaseSlipNumber} をDBへ保存しました`);
    } catch (error) {
      showToast('error', '登録エラー', error.message);
    }
    return;
  }

  if (existingItem) {
    // ════════════════════════════════════════════════════════
    // 【既存商品】未入力項目のみ追加更新（新規レコード生成禁止）
    // ════════════════════════════════════════════════════════
    _puPartialUpdateInventory(existingItem);
    return;
  } else {
    // ════════════════════════════════════════════════════════
    // 【新規商品】在庫に新規追加
    // ════════════════════════════════════════════════════════
    _puRegisterNewInventory(code, brand, dateVal, supplier, purchasePrice);
  }

  _puIssueSinglePurchaseSlip(singlePurchaseSlip);
}

/** 単品の商品登録フォームから、仕入伝票1明細分のスナップショットを作る */
function _puBuildSinglePurchaseSlip(code, dateVal, supplier, purchasePrice) {
  const accessories = [...document.querySelectorAll('#pu-accessories input:checked')].map(c => c.value);
  const braceletQtyRaw = document.getElementById('pu-bracelet-qty')?.value;
  const boxRaw = document.getElementById('pu-box')?.value;
  return {
    date: dateVal,
    supplier,
    staff: document.getElementById('pu-staff')?.value || '',
    note: document.getElementById('pu-comment')?.value || '',
    line: {
      lineNo: 1,
      code,
      sku: document.getElementById('pu-sku')?.value || '',
      purchasePrice,
      salePrice: getPriceValue(document.getElementById('pu-sale-price')),
      productDetail: {
        brand: document.getElementById('pu-brand')?.value || '',
        model: document.getElementById('pu-model')?.value || '',
        ref: document.getElementById('pu-ref')?.value || '',
        serial: document.getElementById('pu-serial')?.value || '',
        material: document.getElementById('pu-material')?.value || '',
        movement: document.getElementById('pu-movement')?.value || '',
        condition: document.getElementById('pu-condition')?.value || '',
        belt: document.getElementById('pu-belt')?.value || '',
        dial: document.getElementById('pu-dial')?.value || '',
        note: document.getElementById('pu-note')?.value || '',
        accessories,
        braceletQty: braceletQtyRaw === '' || braceletQtyRaw == null ? null : Number(braceletQtyRaw),
        boxNo: boxRaw ? Number(boxRaw) : null,
      },
    },
  };
}

/** 単品の商品登録でも仕入伝票を自動起票し、在庫と伝票を相互参照できるようにする */
function _puIssueSinglePurchaseSlip(snapshot) {
  if (!snapshot?.line?.code) return;
  if (!Array.isArray(APP_DATA.purchaseSlips)) APP_DATA.purchaseSlips = [];

  const slipId = typeof peGenerateSlipId === 'function'
    ? peGenerateSlipId()
    : `PI-${new Date().getFullYear()}-${String(APP_DATA.purchaseSlips.length + 1).padStart(4, '0')}`;
  const slip = {
    id: slipId,
    date: snapshot.date,
    supplier: snapshot.supplier,
    staff: snapshot.staff,
    note: snapshot.note,
    status: '処理済',
    source: 'single-product',
    registeredAt: new Date().toISOString().slice(0, 16).replace('T', ' '),
    revisions: [],
    lines: [snapshot.line],
  };

  APP_DATA.purchaseSlips.push(JSON.parse(JSON.stringify(slip)));
  const inventoryItem = (APP_DATA.inventory || []).find(item => item.code === snapshot.line.code);
  if (inventoryItem) inventoryItem.purchaseSlipId = slipId;

  if (typeof _refreshTaskCounts === 'function') _refreshTaskCounts();
  if (typeof refreshLinkedBusinessViews === 'function') refreshLinkedBusinessViews({ source: 'single-product-purchase' });
  showToast('success', '仕入伝票を起票しました', `${slipId} を伝票一覧へ反映しました`);
}

/**
 * 既存在庫レコードに対して「未入力項目のみ」を追加更新する。
 *
 * 【更新ルール】
 *  - 在庫データに値あり → 更新しない（上書き禁止）
 *  - 在庫データが空    → フォーム入力値で更新
 *  - 商品コードは変更不可・既存コードを維持
 *  - 新規レコード生成は一切しない
 *
 * @param {object} existingItem - APP_DATA.inventory の既存レコード（参照）
 */
function _puPartialUpdateInventory(existingItem) {
  // ── 各フィールドの更新対象を収集 ──────────────────────────
  /** @type {string[]} */
  const updatedFields = [];

  /**
   * スカラー値フィールドの更新判定ヘルパー
   * @param {string} prop    - existingItem のプロパティ名
   * @param {*}      newVal  - フォームから取得した新しい値
   * @param {string} label   - ログ用ラベル
   */
  function _applyIfEmpty(prop, newVal, label) {
    const oldVal = existingItem[prop];
    const isEmpty = (oldVal == null || oldVal === '' || oldVal === 0);
    const hasNew  = (newVal != null && newVal !== '' && newVal !== 0);
    if (isEmpty && hasNew) {
      existingItem[prop] = newVal;
      updatedFields.push(label);
    }
  }

  // ── テキスト / select 系 ────────────────────────────────
  _applyIfEmpty('brand',     document.getElementById('pu-brand')?.value    || '', 'ブランド');
  _applyIfEmpty('model',     document.getElementById('pu-model')?.value    || '', 'モデル');
  _applyIfEmpty('ref',       document.getElementById('pu-ref')?.value      || '', '型番');
  _applyIfEmpty('serial',    document.getElementById('pu-serial')?.value   || '', 'シリアル');
  _applyIfEmpty('sku',       document.getElementById('pu-sku')?.value      || '', 'SKU');
  _applyIfEmpty('staff',     document.getElementById('pu-staff')?.value    || '', '担当者');
  _applyIfEmpty('supplier',  document.getElementById('pu-supplier')?.value || '', '仕入先');
  _applyIfEmpty('purchaseDate', document.getElementById('pu-date')?.value  || '', '仕入日');
  _applyIfEmpty('material',  document.getElementById('pu-material')?.value || '', '素材');
  _applyIfEmpty('movement',  document.getElementById('pu-movement')?.value || '', '駆動方式');
  _applyIfEmpty('condition', document.getElementById('pu-condition')?.value|| '', 'コンディション');
  _applyIfEmpty('belt',      document.getElementById('pu-belt')?.value     || '', 'ベルト');
  _applyIfEmpty('dial',      document.getElementById('pu-dial')?.value     || '', '文字盤');
  _applyIfEmpty('note',      document.getElementById('pu-note')?.value     || '', '備考');

  // ── BOX番号 ─────────────────────────────────────────────
  const boxEl  = document.getElementById('pu-box');
  const newBox = boxEl?.value ? parseInt(boxEl.value, 10) : null;
  _applyIfEmpty('boxNo', newBox, 'BOX');

  // ── 仕入金額 ─────────────────────────────────────────────
  const newPurchasePrice = getPriceValue(document.getElementById('pu-price'));
  _applyIfEmpty('purchasePrice', newPurchasePrice, '仕入金額');

  // ── 売価 ─────────────────────────────────────────────────
  const newSalePrice = getPriceValue(document.getElementById('pu-sale-price'));
  _applyIfEmpty('salePrice', newSalePrice, '売価（USD）');

  // ── 付属品チェックボックス ───────────────────────────────
  const existingAccs = existingItem.accessories || [];
  const newAccs = [...document.querySelectorAll('#pu-accessories input:checked')].map(c => c.value);
  if (existingAccs.length === 0 && newAccs.length > 0) {
    existingItem.accessories = newAccs;
    updatedFields.push('付属品');
  }

  // ── BRACELET PARTS コマ数 ────────────────────────────────
  // 付属品に BRACELET PARTS が含まれている場合のみコマ数を更新対象にする
  const currentAccs = existingItem.accessories || [];
  const hasBracelet = currentAccs.includes('BRACELET PARTS');
  if (hasBracelet) {
    const qtyRaw = document.getElementById('pu-bracelet-qty')?.value;
    const newQty = (qtyRaw !== '' && qtyRaw != null) ? parseInt(qtyRaw, 10) : null;
    // 既存データにコマ数がない場合のみ更新（上書き禁止ルール）
    if (existingItem.braceletQty == null && newQty !== null && !isNaN(newQty)) {
      existingItem.braceletQty = newQty;
      updatedFields.push('コマ数');
    }
  } else {
    // BRACELET PARTS が含まれない場合は braceletQty を必ず削除
    if ('braceletQty' in existingItem) {
      delete existingItem.braceletQty;
    }
  }

  // ── 更新日時スタンプ ─────────────────────────────────────
  if (updatedFields.length > 0) {
    if (!existingItem.revisions) existingItem.revisions = [];
    existingItem.revisions.push({
      type:      '不足情報追加',
      updatedAt: new Date().toISOString().slice(0, 16).replace('T', ' '),
      fields:    updatedFields,
    });
  }

  // ── フィードバック ───────────────────────────────────────
  if (updatedFields.length > 0) {
    showToast(
      'success',
      '不足情報を更新しました',
      `${existingItem.code} ／ 更新項目：${updatedFields.join('・')}`
    );
  } else {
    showToast(
      'info',
      '更新不要',
      `${existingItem.code} — すべての項目が既に登録済みです（更新なし）`
    );
  }

  // ── 保存後リセット：フォーム全体をクリアして仕入情報を復元 ──
  // 【処理順序】
  //   1. マッチ状態・ロックをすべてクリア
  //   2. フォーム全フィールドをリセット（仕入日・仕入先は含まない）
  //   3. ステートから仕入日・仕入先を復元
  _puMatchedItem = null;
  _puClearMatchState();
  _puFullResetForm();
  _puRestoreAfterSubmit();
}

/**
 * 新規在庫レコードを APP_DATA.inventory に追加する。
 * （商品コードが既存と重複しない場合のみ呼ばれる）
 *
 * @param {string} code
 * @param {string} brand
 * @param {string} dateVal
 * @param {string} supplier
 * @param {number} purchasePrice
 */
function _puRegisterNewInventory(code, brand, dateVal, supplier, purchasePrice) {
  const accs  = [...document.querySelectorAll('#pu-accessories input:checked')].map(c => c.value);
  const boxEl = document.getElementById('pu-box');
  const boxNo = boxEl?.value ? parseInt(boxEl.value, 10) : null;

  // BRACELET PARTS 選択時のみコマ数を取得（未選択時は null）
  const hasBracelet  = accs.includes('BRACELET PARTS');
  const braceletQtyRaw = hasBracelet
    ? document.getElementById('pu-bracelet-qty')?.value
    : null;
  const braceletQty  = hasBracelet
    ? (braceletQtyRaw !== '' && braceletQtyRaw !== null ? parseInt(braceletQtyRaw, 10) : null)
    : null;

  const newItem = {
    code, brand,
    model:         document.getElementById('pu-model')?.value      || '',
    ref:           document.getElementById('pu-ref')?.value        || '',
    serial:        document.getElementById('pu-serial')?.value     || '',
    sku:           document.getElementById('pu-sku')?.value        || '',
    supplier,
    staff:         document.getElementById('pu-staff')?.value      || '',
    purchasePrice,
    purchaseDate:  dateVal,
    status:        '在庫中',
    material:      document.getElementById('pu-material')?.value   || '',
    movement:      document.getElementById('pu-movement')?.value   || '',
    condition:     document.getElementById('pu-condition')?.value  || '',
    belt:          document.getElementById('pu-belt')?.value       || '',
    dial:          document.getElementById('pu-dial')?.value       || '',
    salePrice:     getPriceValue(document.getElementById('pu-sale-price')),
    accessories:   accs,
    braceletQty:   braceletQty,   // BRACELET PARTS 未選択時は null
    images:        uploadedImages.filter(Boolean),
    note:          document.getElementById('pu-note')?.value       || '',
    comment:       document.getElementById('pu-comment')?.value    || '',
    boxNo,
    revisions: [],
  };

  // 仕入日・仕入先の引継ぎを保存
  savePurchaseCarryover();

  APP_DATA.inventory.push(newItem);
  showToast('success', '仕入登録完了', `${code} を登録しました`);

  // ── 登録後リセット：フォーム全体をクリアして仕入情報を復元 ──
  // 【処理順序】
  //   1. マッチ状態・ロックをすべてクリア
  //   2. フォーム全フィールドをリセット（仕入日・仕入先は含まない）
  //   3. ステートから仕入日・仕入先を復元（_puCaptureBeforeSubmit で保存済み）
  _puClearMatchState();
  _puFullResetForm();
  _puRestoreAfterSubmit();
}

/**
 * @deprecated _puFullResetForm() に統合。後方互換のために残す。
 * 呼び出し元がなくなれば将来削除可。
 */
function _resetPurchaseFormFields() {
  _puFullResetForm();
}

// =====================================================
// 売上登録
// =====================================================
let salesLineCount = 0;
let _salesEntryCurrency = 'USD';
let _salesSourceShipmentId = null;

/** マスタ登録のUSドル円換算レートを返す */
function getSalesUsdRate() {
  const masterRate = Number((APP_DATA.fxRates || []).find(rate => rate.code === 'USD')?.rate);
  if (Number.isFinite(masterRate) && masterRate > 0) return masterRate;
  return Number(SALE_PRICE_JPY_PER_USD) || 155;
}

/** 入力通貨の金額を、内部基準のUSDへ換算する */
function convertSalesEntryToUSD(amount, currency) {
  const value = Number(amount) || 0;
  if (currency === 'JPY') return Math.round(value / getSalesUsdRate());
  return Math.round(value);
}

/** USD基準金額を、指定した入力通貨の表示値へ換算する */
function formatSalesEntryAmount(usdAmount, currency) {
  const usd = Number(usdAmount) || 0;
  if (usd <= 0) return '';
  const displayAmount = currency === 'JPY'
    ? Math.round(usd * getSalesUsdRate())
    : Math.round(usd);
  return displayAmount.toLocaleString('ja-JP');
}

/** USD基準金額を、現在選択中の表示通貨へ換算する */
function convertSalesUSDToDisplay(usdAmount, currency = _salesEntryCurrency) {
  const usd = Number(usdAmount) || 0;
  if (currency === 'JPY') return Math.round(usd * getSalesUsdRate());
  return Math.round(usd);
}

/** USD基準金額を、現在選択中の通貨記号付きで表示する */
function formatSalesDisplayAmount(usdAmount, currency = _salesEntryCurrency) {
  const displayAmount = convertSalesUSDToDisplay(usdAmount, currency);
  return currency === 'JPY' ? formatPrice(displayAmount) : formatSalePrice(displayAmount);
}

/** 販売金額入力欄からUSD基準金額を取得する */
function getSalesLinePriceUSD(input) {
  if (!input) return 0;
  const entryCurrency = input.dataset.entryCurrency || _salesEntryCurrency;
  const usdValue = convertSalesEntryToUSD(parseSalesPrice(input.value), entryCurrency);
  input.dataset.usdValue = usdValue > 0 ? String(usdValue) : '';
  return usdValue;
}

/** 販売金額列の通貨表示・アクセシビリティ状態を同期する */
function _syncSalesCurrencyUI() {
  const rate = getSalesUsdRate();
  const isJPY = _salesEntryCurrency === 'JPY';
  const heading = document.getElementById('sl-price-heading');
  const rateNote = document.getElementById('sl-price-rate');
  const usdButton = document.getElementById('sl-currency-usd');
  const jpyButton = document.getElementById('sl-currency-jpy');
  const subtotalLabel = document.getElementById('salesSubtotalLabel');
  const taxLabel = document.getElementById('salesTaxLabel');
  const grandLabel = document.getElementById('salesGrandLabel');

  if (heading) heading.textContent = isJPY ? '販売金額（円入力・税抜）' : '販売金額（USD・税抜）';
  if (rateNote) rateNote.textContent = `1 USD = ¥${rate.toLocaleString('ja-JP', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
  if (subtotalLabel) subtotalLabel.textContent = isJPY ? '合計金額（円・税抜）' : '合計金額（USD・税抜）';
  if (taxLabel) taxLabel.textContent = isJPY ? '消費税（10%・円）' : '消費税（10%・USD）';
  if (grandLabel) grandLabel.textContent = isJPY ? '税込合計（円）' : '税込合計（USD）';
  [
    [usdButton, !isJPY],
    [jpyButton, isJPY],
  ].forEach(([button, active]) => {
    if (!button) return;
    button.classList.toggle('active', active);
    button.setAttribute('aria-pressed', active ? 'true' : 'false');
  });

  document.querySelectorAll('#salesLines .sl-input-price').forEach(input => {
    const lineId = input.id.replace('sl-price-', '');
    const prefix = document.getElementById(`sl-price-prefix-${lineId}`);
    if (prefix) prefix.textContent = isJPY ? '¥' : '\u0024';
    input.setAttribute('aria-label', isJPY ? '販売金額（円）' : '販売金額（USD）');
    input.placeholder = isJPY ? '0円' : '0 USD';
  });
}

/** 販売金額の入力通貨を切り替える（内部基準は常にUSD） */
function switchSalesEntryCurrency(currency) {
  if (currency !== 'USD' && currency !== 'JPY') return;

  const inputs = [...document.querySelectorAll('#salesLines .sl-input-price')];
  const usdValues = inputs.map(input => getSalesLinePriceUSD(input));
  _salesEntryCurrency = currency;
  inputs.forEach((input, index) => {
    input.dataset.entryCurrency = currency;
    input.dataset.usdValue = usdValues[index] > 0 ? String(usdValues[index]) : '';
    input.value = formatSalesEntryAmount(usdValues[index], currency);
  });

  _syncSalesCurrencyUI();
  calcSalesTotal();
}

// ── 新規売上伝票番号を生成して返す ──
function newSalesId() {
  const now = new Date();
  return `SL-${now.getFullYear()}-${String(APP_DATA.sales.length + 1).padStart(4, '0')}`;
}

// ── 「出荷なし（新規発番）」ボタン ──
function setNewSalesId() {
  _salesSourceShipmentId = null;
  const id = newSalesId();
  document.getElementById('sl-id').value = id;
  setSlIdStatus('new', `新規発番: ${id}`);
  // 明細・取引先・日付をクリアして空行を1行追加
  clearSalesLines();
}

// ── 出荷伝票番号入力時の処理 ──
function onSalesIdInput(val) {
  const trimmed = val.trim();
  if (!trimmed) {
    // 入力した出荷伝票番号を消した場合は、連動して展開した明細も残さない。
    _salesSourceShipmentId = null;
    clearSalesLines();
    setSlIdStatus('', '');
    return;
  }
  // SH- で始まる場合は出荷伝票と照合
  if (/^SH-/i.test(trimmed)) {
    const ship = APP_DATA.shipments.find(s => s.id.toUpperCase() === trimmed.toUpperCase());
    if (ship) {
      applyShipmentToSales(ship);
    } else {
      setSlIdStatus('notfound', `出荷伝票「${trimmed}」は見つかりません`);
    }
  } else if (/^SL-/i.test(trimmed)) {
    // SL- 番号はそのまま手入力として受け付け
    setSlIdStatus('manual', '手動入力の売上伝票番号');
  } else {
    setSlIdStatus('', '');
  }
}

// ── 出荷伝票の内容を売上フォームへ反映 ──
function applyShipmentToSales(ship) {
  _salesSourceShipmentId = ship.id;
  // 伝票番号を出荷IDベースの売上IDへ（SH→SL に置換）
  const slId = ship.id.replace(/^SH-/i, 'SL-');
  document.getElementById('sl-id').value = slId;

  // 出荷日を売上日にセット
  document.getElementById('sl-date').value = ship.date || getLocalDateISO();

  // 出荷先（destination）を販売先としてセット
  const buyerSel = document.getElementById('sl-buyer');
  if (buyerSel) buyerSel.value = ship.destination || '';

  // 明細行をクリアして出荷商品をテーブル行で反映
  salesLineCount = 0;
  const tbody = document.getElementById('salesLines');
  if (tbody) tbody.innerHTML = '';
  (ship.items || []).forEach(it => {
    salesLineCount++;
    const inv = APP_DATA.inventory.find(i => i.code === it.code);
    tbody.insertAdjacentHTML('beforeend',
      buildSalesLineHTML(salesLineCount, it.code, it.brand || inv?.brand || '', it.model || inv?.model || '',
        it.salePrice || it.wholesale || inv?.salePrice || 0, inv || null)
    );
  });
  if (salesLineCount === 0) addSalesLine();
  calcSalesTotal();

  // 備考に出荷伝票番号を記入
  const noteEl = document.getElementById('sl-note');
  if (noteEl && !noteEl.value) noteEl.value = `出荷伝票: ${ship.id}`;

  setSlIdStatus('linked', `出荷伝票「${ship.id}」の内容を反映しました（${ship.items?.length || 0}点）`);
  showToast('success', '出荷伝票を反映', `${ship.id} の内容を売上伝票に自動入力しました`);
}

// ── 明細行 <tr> を組み立てる ──
function buildSalesLineHTML(lineId, code, brand, model, price, itemData) {
  const ref   = itemData?.ref    || '';
  const serial= itemData?.serial || '';
  const accs  = Array.isArray(itemData?.accessories) && itemData.accessories.length
    ? itemData.accessories.join('、') : '';
  const brandHtml  = brand  || '<span class="sl-placeholder">—</span>';
  const modelHtml  = model  || '<span class="sl-placeholder">—</span>';
  const refHtml    = ref    || '<span class="sl-placeholder">—</span>';
  const serialHtml = serial || '<span class="sl-placeholder">—</span>';
  const accsHtml   = accs   || '<span class="sl-placeholder">—</span>';
  return `<tr class="sl-tbody-row" data-line-id="${lineId}">
  <td class="sl-td sl-td-chk">
    <label class="sl-chk-label">
      <input type="checkbox" class="sl-include-chk" id="sl-chk-${lineId}"
        checked onchange="onSalesLineCheck(${lineId})">
    </label>
  </td>
  <td class="sl-td sl-td-code">
    <input class="sl-input" type="text" id="sl-code-${lineId}"
      placeholder="管理番号" value="${code || ''}"
      oninput="autoFillItem(this,${lineId},'sales')">
  </td>
  <td class="sl-td sl-td-text" id="sl-brand-${lineId}">${brandHtml}</td>
  <td class="sl-td sl-td-text" id="sl-model-${lineId}">${modelHtml}</td>
  <td class="sl-td sl-td-text sl-td-sub" id="sl-ref-${lineId}">${refHtml}</td>
  <td class="sl-td sl-td-text sl-td-sub" id="sl-serial-${lineId}">${serialHtml}</td>
  <td class="sl-td sl-td-text sl-td-sub" id="sl-accs-${lineId}">${accsHtml}</td>
  <td class="sl-td sl-td-price">
    <div class="sl-price-field">
      <span class="sl-price-prefix" id="sl-price-prefix-${lineId}" aria-hidden="true">${_salesEntryCurrency === 'JPY' ? '¥' : '\u0024'}</span>
      <input class="sl-input sl-input-price" type="text" inputmode="numeric" id="sl-price-${lineId}"
        aria-label="${_salesEntryCurrency === 'JPY' ? '販売金額（円）' : '販売金額（USD）'}"
        placeholder="${_salesEntryCurrency === 'JPY' ? '0円' : '0 USD'}"
        value="${price ? formatSalesEntryAmount(price, _salesEntryCurrency) : ''}"
        data-entry-currency="${_salesEntryCurrency}"
        data-usd-value="${price || ''}"
        oninput="onSalesPriceInput(this)"
        onblur="onSalesPriceBlur(this)"
        onchange="calcSalesTotal()">
    </div>
  </td>
  <td class="sl-td sl-td-del">
    <button class="sl-del-btn" type="button" title="この行を削除"
      onclick="removeSalesLine(this)"><i class="fa-solid fa-xmark"></i></button>
  </td>
</tr>`;
}

// ── 売上計上チェック変更 ──
function onSalesLineCheck(lineId) {
  const row = document.querySelector(`tr[data-line-id="${lineId}"]`);
  if (!row) return;
  const chk = document.getElementById(`sl-chk-${lineId}`);
  row.classList.toggle('sl-row-excluded', !chk?.checked);
  calcSalesTotal();
}

// ── 明細行・関連フィールドをクリア ──
function clearSalesLines() {
  salesLineCount = 0;
  const tbody = document.getElementById('salesLines');
  if (tbody) tbody.innerHTML = '';
  const noteEl = document.getElementById('sl-note');
  if (noteEl) noteEl.value = '';
  addSalesLine();
  calcSalesTotal();
}

// ── 伝票番号欄のステータス表示 ──
function setSlIdStatus(type, msg) {
  const statusEl = document.getElementById('slIdStatus');
  const hintEl   = document.getElementById('slIdHint');
  if (!statusEl || !hintEl) return;

  statusEl.className = 'sl-id-status';
  hintEl.className   = 'sl-id-hint';

  if (!type || !msg) {
    statusEl.innerHTML = '';
    hintEl.innerHTML   = '';
    return;
  }

  const icons = {
    linked:   '<i class="fa-solid fa-link"></i>',
    new:      '<i class="fa-solid fa-plus-circle"></i>',
    manual:   '<i class="fa-solid fa-pen"></i>',
    notfound: '<i class="fa-solid fa-circle-xmark"></i>',
  };
  statusEl.innerHTML = icons[type] || '';
  statusEl.classList.add(`sl-status-${type}`);
  hintEl.innerHTML   = msg;
  hintEl.classList.add(`sl-hint-${type}`);
}

function initSalesForm() {
  _salesSourceShipmentId = null;
  _salesEntryCurrency = 'USD';
  // 初期状態は空欄（手入力 or 「出荷なし」ボタンで発番）
  document.getElementById('sl-id').value = '';
  setSlIdStatus('', '');
  document.getElementById('sl-date').value = getLocalDateISO();

  const buyerSel = document.getElementById('sl-buyer');
  if (typeof populateBuyerMasterSelect === 'function') {
    populateBuyerMasterSelect('sl-buyer', { emptyLabel: '-- 選択 --', selected: buyerSel?.value || '', labelMode: 'code-name' });
  }

  salesLineCount = 0;
  document.getElementById('salesLines').innerHTML = '';
  addSalesLine();
  _syncSalesCurrencyUI();
  calcSalesTotal();
}

function init_sales() {
  if (typeof populateBuyerMasterSelect === 'function') {
    populateBuyerMasterSelect('sl-buyer', {
      emptyLabel: '-- 選択 --', selected: document.getElementById('sl-buyer')?.value || '', labelMode: 'code-name',
    });
  }
}

// =====================================================
// 伝票一覧（仕入 / 出荷 / 売上 タブ切替）
// =====================================================
let currentSlipTab = 'purchase'; // 現在のタブ

// =====================================================
// ① タスク判定関数（カウント・絞り込み・再計算の共通ロジック）
// 「承認待ち」または「差戻し」の伝票をタスクとして扱う。
// カウント用と表示用で必ずこの同一関数を使用する。
// =====================================================
function _isTaskItem(r) {
  const st = (r && r.status) ? r.status : '';
  return st === '承認待ち' || st === '差戻し';
}

/** ④ ステータス変更後の再計算トリガー（どこからでも呼べる） */
function _refreshTaskCounts() {
  // renderSlipList に data=null を渡すと未検索状態にリセットされるため、
  // カウントだけを更新するために各バッジ要素を直接書き換える
  const _count = (list) => (list || []).filter(_isTaskItem).length;
  const counts = {
    purchase:       _count(APP_DATA.purchaseSlips),
    shipping:       _count(APP_DATA.shipments),
    sales:          _count(APP_DATA.sales),
    salesreturn:    _count(APP_DATA.salesReturns),
    purchasereturn: _count(APP_DATA.purchaseReturns),
  };
  Object.entries(counts).forEach(([tab, cnt]) => {
    const el = document.getElementById(`sltab-count-${tab}`);
    if (!el) return;
    el.textContent    = cnt;
    el.style.display  = cnt > 0 ? '' : 'none';
  });
}

function init_sales_list() {
  initSlipList();
}

function initSlipList() {
  switchSlipTab(currentSlipTab);
}

// ── タブ切替 ──
function switchSlipTab(type) {
  currentSlipTab = type;
  ['purchase','shipping','sales','salesreturn','purchasereturn'].forEach(t => {
    document.getElementById('sltab-' + t)?.classList.toggle('active', t === type);
  });
  // 取引先セレクト更新
  rebuildSlipPartySelect(type);
  // 登録元との連動結果をすぐ確認できるよう、タブ切替時は全件表示する。
  _slipFilterState = { executed: true, showAll: true };
  ['slip-filter-from','slip-filter-to','slip-filter-party','slip-filter-keyword'].forEach(id => {
    const el = document.getElementById(id);
    if (el) el.value = '';
  });
  // 売上・出荷伝票の選択状態をリセット
  _slResetSelection();
  _shResetSelection();
  filterSlipList();
}

// ── バッジクリック：承認待ち+差戻し のある伝票のみ絞り込み表示 ──
// ① カウントと絞り込みは完全に同じ filter 条件を使用する
function switchSlipTabPending(type) {
  currentSlipTab = type;
  ['purchase','shipping','sales','salesreturn','purchasereturn'].forEach(t => {
    document.getElementById('sltab-' + t)?.classList.toggle('active', t === type);
  });
  rebuildSlipPartySelect(type);

  // 売上伝票選択状態をリセット
  _slResetSelection();
  // 出荷伝票選択状態をリセット
  _shResetSelection();

  // ① タスク条件（グローバル _isTaskItem と同一）: 承認待ち OR 差戻し
  // ① 同じ配列から同じ filter で絞り込み
  let data = [];
  if (type === 'purchase') {
    data = (APP_DATA.purchaseSlips || []).filter(_isTaskItem);
  } else if (type === 'shipping') {
    data = (APP_DATA.shipments || []).filter(_isTaskItem);
  } else if (type === 'sales') {
    data = (APP_DATA.sales || []).filter(_isTaskItem);
  } else if (type === 'salesreturn') {
    data = (APP_DATA.salesReturns || []).filter(_isTaskItem);
  } else if (type === 'purchasereturn') {
    data = (APP_DATA.purchaseReturns || []).filter(_isTaskItem);
  }

  // 検索プレースホルダーを非表示
  const promptEl = document.getElementById('slipSearchPrompt');
  if (promptEl) promptEl.style.display = 'none';

  // ③ フィルタバーに「承認タスク表示中」インジケーターを表示
  _showPendingFilterBadge(type);

  // ② 一覧に完全一致した件数を表示（0件でも renderSlipList を呼ぶ）
  _slipFilterState = { executed: true, showAll: false };
  renderSlipList(data);
}

// ── 承認タスク絞り込み中インジケーター ──
let _pendingFilterActive = false;
function _showPendingFilterBadge(type) {
  _pendingFilterActive = true;
  let bar = document.getElementById('pendingFilterBar');
  if (!bar) {
    bar = document.createElement('div');
    bar.id = 'pendingFilterBar';
    bar.style.cssText = 'display:flex;align-items:center;gap:8px;padding:6px 12px;background:#fffbeb;border:1px solid #fcd34d;border-radius:6px;font-size:12px;color:#92400e;margin-bottom:6px;';
    const filterRow = document.getElementById('slipFilterRow');
    if (filterRow) filterRow.parentNode.insertBefore(bar, filterRow.nextSibling);
  }
  // ③「承認タスク表示中」状態ラベルを表示
  bar.innerHTML = `
    <i class="fa-solid fa-triangle-exclamation" style="color:#d97706;"></i>
    <b>承認タスク表示中</b>：<span style="color:#78350f;">「承認待ち」または「差戻し」の伝票のみ表示しています</span>
    <button class="btn btn-sm btn-outline" style="margin-left:auto;font-size:11px;padding:2px 8px;"
      onclick="clearPendingFilter()"><i class="fa-solid fa-xmark"></i> 解除（全件表示）</button>
  `;
  bar.style.display = 'flex';
}

function clearPendingFilter() {
  _pendingFilterActive = false;
  const bar = document.getElementById('pendingFilterBar');
  if (bar) bar.style.display = 'none';
  clearSlipFilter();
}

// ── 取引先セレクト再構築 ──
function rebuildSlipPartySelect(type) {
  const sel   = document.getElementById('slip-filter-party');
  const label = document.getElementById('slipFilterPartyLabel');
  if (!sel) return;
  sel.innerHTML = '<option value="">すべて</option>';
  if (type === 'purchase') {
    if (label) label.innerHTML = '<i class="fa-solid fa-industry"></i> 仕入先';
    APP_DATA.suppliers.forEach(s => { sel.innerHTML += `<option value="${s.code}">${s.name}</option>`; });
  } else if (type === 'purchasereturn') {
    if (label) label.innerHTML = '<i class="fa-solid fa-industry"></i> 仕入先';
    APP_DATA.suppliers.forEach(s => { sel.innerHTML += `<option value="${s.code}">${s.name}</option>`; });
  } else {
    if (label) label.innerHTML = '<i class="fa-solid fa-building"></i> 取引先';
    APP_DATA.buyers.forEach(b => { sel.innerHTML += `<option value="${b.code}">${b.name}</option>`; });
  }
}

// ── フィルタ実行 ──
// =====================================================
// 伝票一覧 絞り込み状態管理
// =====================================================

/** フィルター実行状態を保持
 *  executed: 絞り込みボタンが押された
 *  showAll:  全件表示ボタンが押された
 */
let _slipFilterState = { executed: false, showAll: false };

/**
 * 【絞り込むボタン】押下時のみ検索を実行
 * – Enterキーでも同じ動作
 */
function execSlipFilter() {
  _slipFilterState = { executed: true, showAll: false };
  filterSlipList();
}

/**
 * 【全件表示ボタン】すべての検索条件をリセットしてデータ全件を表示
 */
function showAllSlipList() {
  // フィルタ入力をクリア
  ['slip-filter-from','slip-filter-to','slip-filter-party','slip-filter-keyword'].forEach(id => {
    const el = document.getElementById(id);
    if (el) el.value = '';
  });
  _slipFilterState = { executed: true, showAll: true };
  filterSlipList();
}

/**
 * 登録・修正完了後の内部更新用
 * 現在の状態（絞り込み中 or 全件表示中 or 未実行）を維持して再描画
 */
function refreshSlipList() {
  if (!document.getElementById('slipListBody')) return;
  if (_slipFilterState.executed) {
    filterSlipList();
  } else {
    renderSlipList(null); // 未検索状態を維持
  }
}

function filterSlipList() {
  const from    = document.getElementById('slip-filter-from')?.value    || '';
  const to      = document.getElementById('slip-filter-to')?.value      || '';
  const party   = document.getElementById('slip-filter-party')?.value   || '';
  const keyword = (document.getElementById('slip-filter-keyword')?.value || '').toLowerCase();

  // 全件表示モード：条件なしで全データ表示
  if (_slipFilterState.showAll) {
    // フォールスルーして全データを収集（hasFilter チェックをスキップ）
  } else if (!_slipFilterState.executed) {
    // 未実行状態（ページ初期表示・タブ切替後）は白紙
    renderSlipList(null);
    return;
  }

  // フィルター未入力 & 全件表示でないなら白紙（通常の絞り込み時）
  const hasFilter = from || to || party || keyword;
  if (!_slipFilterState.showAll && !hasFilter) {
    renderSlipList(null);
    return;
  }

  let data = [];
  if (currentSlipTab === 'purchase') {
    data = (APP_DATA.purchaseSlips || []).filter(slip => {
      if (from && slip.date < from) return false;
      if (to   && slip.date > to)   return false;
      if (party && slip.supplier !== party) return false;
      if (keyword) {
        const lineText = (slip.lines || []).flatMap(l => [
          l.code, l.sku,
          l.productDetail?.brand  || '',
          l.productDetail?.model  || '',
          l.productDetail?.ref    || '',
          l.productDetail?.serial || '',
        ]).join(' ');
        const h = [slip.id, slip.date, getSupplierName(slip.supplier),
                   slip.staff || '', lineText].join(' ').toLowerCase();
        if (!h.includes(keyword)) return false;
      }
      return true;
    });
  } else if (currentSlipTab === 'shipping') {
    data = APP_DATA.shipments.filter(s => {
      if (from && s.date < from) return false;
      if (to   && s.date > to)   return false;
      if (party && s.destination !== party) return false;
      if (keyword) {
        const h = [s.id, getBuyerName(s.destination), s.note||'',
                   ...(s.items||[]).flatMap(it=>[it.code,it.brand,it.model])].join(' ').toLowerCase();
        if (!h.includes(keyword)) return false;
      }
      return true;
    });
  } else if (currentSlipTab === 'salesreturn') {
    // 売上返品伝票（独立した salesReturns データ）
    data = (APP_DATA.salesReturns || []).filter(r => {
      if (from && r.date < from) return false;
      if (to   && r.date > to)   return false;
      if (party && r.buyer !== party) return false;
      if (keyword) {
        const h = [r.id, r.slipId||'', getBuyerName(r.buyer), r.note||'', r.reason||'',
                   ...(r.items||[]).flatMap(it=>[it.code,it.brand,it.model])].join(' ').toLowerCase();
        if (!h.includes(keyword)) return false;
      }
      return true;
    });
  } else if (currentSlipTab === 'purchasereturn') {
    data = (APP_DATA.purchaseReturns || []).filter(r => {
      if (from && r.date < from) return false;
      if (to   && r.date > to)   return false;
      if (party && r.supplier !== party) return false;
      if (keyword) {
        const h = [r.id, getSupplierName(r.supplier), r.note||'',
                   ...(r.items||[]).flatMap(it=>[it.code,it.brand,it.model])].join(' ').toLowerCase();
        if (!h.includes(keyword)) return false;
      }
      return true;
    });
  } else {
    data = APP_DATA.sales.filter(s => {
      if (from && s.date < from) return false;
      if (to   && s.date > to)   return false;
      if (party && s.buyer !== party) return false;
      if (keyword) {
        const h = [s.id, getBuyerName(s.buyer), s.note||'',
                   ...(s.items||[]).flatMap(it=>[it.code,it.brand,it.model])].join(' ').toLowerCase();
        if (!h.includes(keyword)) return false;
      }
      return true;
    });
  }

  renderSlipList(data);
}

// ── フィルタリセット（リセットボタン） ──
function clearSlipFilter() {
  ['slip-filter-from','slip-filter-to','slip-filter-party','slip-filter-keyword'].forEach(id => {
    const el = document.getElementById(id);
    if (el) el.value = '';
  });
  _slipFilterState = { executed: false, showAll: false };
  _pendingFilterActive = false;
  // ⑨ フィルターバーを非表示にしてUIを一致させる
  const bar = document.getElementById('pendingFilterBar');
  if (bar) bar.style.display = 'none';
  renderSlipList(null);
}

// ── 一覧レンダリング ──
function renderSlipList(data) {
  const tbody  = document.getElementById('slipListBody');
  const head   = document.getElementById('slipTableHead');
  const noData = document.getElementById('slipNoData');
  if (!tbody) return;

  // ── タスク数（ロール共通） ──
  // グローバルの _isTaskItem() を使用する（カウントと絞り込みで同一関数を共有）
  const _taskCountLocal = (list) => (list || []).filter(_isTaskItem).length;
  const countPurch   = _taskCountLocal(APP_DATA.purchaseSlips    || []);
  const countShip    = _taskCountLocal(APP_DATA.shipments        || []);
  const countSales   = _taskCountLocal(APP_DATA.sales            || []);
  const countSRet    = _taskCountLocal(APP_DATA.salesReturns     || []);
  const countPRet    = _taskCountLocal(APP_DATA.purchaseReturns  || []);

  const elPurchase       = document.getElementById('sltab-count-purchase');
  const elShipping       = document.getElementById('sltab-count-shipping');
  const elSales          = document.getElementById('sltab-count-sales');
  const elSalesReturn    = document.getElementById('sltab-count-salesreturn');
  const elPurchaseReturn = document.getElementById('sltab-count-purchasereturn');

  if (elPurchase)       { elPurchase.textContent       = countPurch; elPurchase.style.display       = countPurch > 0 ? '' : 'none'; }
  if (elShipping)       { elShipping.textContent       = countShip;  elShipping.style.display       = countShip  > 0 ? '' : 'none'; }
  if (elSales)          { elSales.textContent          = countSales; elSales.style.display          = countSales > 0 ? '' : 'none'; }
  if (elSalesReturn)    { elSalesReturn.textContent    = countSRet;  elSalesReturn.style.display    = countSRet  > 0 ? '' : 'none'; }
  if (elPurchaseReturn) { elPurchaseReturn.textContent = countPRet;  elPurchaseReturn.style.display = countPRet  > 0 ? '' : 'none'; }

  // null = 未検索状態 → 白紙表示
  if (data === null) {
    tbody.innerHTML = '';
    head.innerHTML  = '';
    noData?.classList.add('hidden');
    const summaryEl = document.getElementById('slipSummaryBar');
    if (summaryEl) summaryEl.style.display = 'none';
    const promptEl = document.getElementById('slipSearchPrompt');
    if (promptEl) promptEl.style.display = '';
    return;
  }

  // サマリーバーを表示、プレースホルダーを非表示
  const summaryEl = document.getElementById('slipSummaryBar');
  if (summaryEl) summaryEl.style.display = '';
  const promptEl = document.getElementById('slipSearchPrompt');
  if (promptEl) promptEl.style.display = 'none';


  let totalAmt = 0, totalItems = 0, revisedCount = 0;
  if (currentSlipTab === 'purchase') {
    data.forEach(slip => {
      const slipTotal = (slip.lines || []).reduce((s, l) => s + (l.purchasePrice || 0), 0);
      totalAmt   += slipTotal;
      totalItems += (slip.lines || []).length;
    });
  } else if (currentSlipTab === 'purchasereturn') {
    data.forEach(r => {
      totalAmt += getPurchaseReturnOriginalAmountInfo(r).subtotal;
      totalItems += (r.items||[]).length;
    });
  } else if (currentSlipTab === 'salesreturn') {
    data.forEach(r => {
      totalAmt += getSalesReturnOriginalAmountInfo(r).grandTotal;
      totalItems += (r.items||[]).length;
    });
  } else {
    data.forEach(s => { totalAmt += s.total || 0; totalItems += s.items?.length || 0; if ((s.revisions||[]).length) revisedCount++; });
  }
  document.getElementById('slipSummaryCount').textContent   = `${data.length}件`;
  const summaryUsesUSD = ['shipping', 'sales', 'salesreturn'].includes(currentSlipTab);
  document.getElementById('slipSummaryTotal').textContent   = summaryUsesUSD
    ? formatSalePrice(totalAmt)
    : formatPrice(totalAmt);
  document.getElementById('slipSummaryItems').textContent   = `${totalItems}点`;
  document.getElementById('slipSummaryRevised').textContent = `${revisedCount}件`;

  if (data.length === 0) {
    tbody.innerHTML = '';
    head.innerHTML  = '';
    // ⑥ 0件時は「該当データなし」を表示
    if (noData) {
      noData.classList.remove('hidden');
      // タスク絞り込み中の場合はメッセージを差し替え
      if (_pendingFilterActive) {
        noData.textContent = '承認待ち・差戻しの伝票はありません';
      } else {
        noData.textContent = '該当データなし';
      }
    }
    return;
  }
  noData?.classList.add('hidden');

  // ヘッダー
  if (currentSlipTab === 'purchase') {
    head.innerHTML = `<tr>
      <th>伝票番号</th><th>仕入日</th><th>仕入先</th>
      <th>仕入担当者</th><th style="text-align:center;">明細数</th>
      <th style="text-align:right;">合計仕入金額</th>
      <th>備考</th><th style="width:90px;text-align:center;">ステータス/修正</th>
    </tr>`;
  } else if (currentSlipTab === 'shipping') {
    head.innerHTML = `<tr>
      <th style="width:36px;text-align:center;padding:6px 4px;">
        <input type="checkbox" id="shSelectAll" title="全選択"
          onchange="shToggleSelectAll(this.checked)"
          style="cursor:pointer;width:15px;height:15px;">
      </th>
      <th>伝票番号</th><th>出荷日</th><th>出荷先</th>
      <th style="text-align:center;">点数</th><th style="text-align:right;">合計金額</th>
      <th>備考</th><th>ステータス</th><th style="width:60px;text-align:center;">修正</th>
    </tr>`;
    // 出荷伝票専用コントロールを集計バーに表示
    const shBulkCtrl = document.getElementById('shBulkControls');
    if (shBulkCtrl) shBulkCtrl.style.display = 'flex';
    // 売上伝票コントロールは隠す
    const slBulkCtrl2 = document.getElementById('slBulkControls');
    if (slBulkCtrl2) slBulkCtrl2.style.display = 'none';
  } else if (currentSlipTab === 'salesreturn') {
    head.innerHTML = `<tr>
      <th>売上返品伝票番号</th><th>返品日</th><th>販売先</th>
      <th style="text-align:center;">点数</th><th style="text-align:right;">販売時合計</th>
      <th>ステータス</th>
      <th style="width:130px;text-align:center;">操作</th>
    </tr>`;
  } else if (currentSlipTab === 'purchasereturn') {
    head.innerHTML = `<tr>
      <th style="width:36px;text-align:center;padding:6px 4px;">
        <input type="checkbox" id="prSelectAll" title="全選択"
          onchange="prToggleSelectAll(this.checked)"
          style="cursor:pointer;width:15px;height:15px;">
      </th>
      <th>仕入返品伝票番号</th><th>返品日</th>
      <th class="pr-sort-th" onclick="prToggleSort()" style="cursor:pointer;user-select:none;white-space:nowrap;">
        仕入先 <span id="prSortIcon" class="pr-sort-icon"></span>
      </th>
      <th style="text-align:center;">点数</th>
      <th style="text-align:right;">仕入金額合計</th>
      <th>返品理由</th>
      <th>ステータス</th>
      <th style="min-width:160px;">配送番号</th>
      <th style="width:180px;text-align:center;">操作</th>
    </tr>`;
    // 請求書発行ボタンを集計バーの右に注入
    _prInjectInvoiceBtn();
  } else if (currentSlipTab === 'sales') {
    head.innerHTML = `<tr>
      <th style="width:36px;text-align:center;padding:6px 4px;">
        <input type="checkbox" id="slSelectAll" title="全選択"
          onchange="slToggleSelectAll(this.checked)"
          style="cursor:pointer;width:15px;height:15px;">
      </th>
      <th>伝票番号</th><th>売上日</th><th>販売先</th>
      <th style="text-align:center;">点数</th><th style="text-align:right;">合計金額</th>
      <th>備考</th><th>ステータス</th><th style="width:60px;text-align:center;">修正</th>
    </tr>`;
    // 売上伝票専用コントロールを集計バーに表示
    const slBulkCtrl = document.getElementById('slBulkControls');
    if (slBulkCtrl) slBulkCtrl.style.display = 'flex';
    // 出荷伝票コントロールは隠す
    const shBulkCtrl2 = document.getElementById('shBulkControls');
    if (shBulkCtrl2) shBulkCtrl2.style.display = 'none';
  } else {
    head.innerHTML = `<tr>
      <th>伝票番号</th><th>売上日</th><th>販売先</th>
      <th style="text-align:center;">点数</th><th style="text-align:right;">合計金額</th>
      <th>備考</th><th>ステータス</th><th style="width:60px;text-align:center;">修正</th>
    </tr>`;
    // 売上・出荷以外はどちらのコントロールも隠す
    const slBulkCtrl = document.getElementById('slBulkControls');
    if (slBulkCtrl) slBulkCtrl.style.display = 'none';
    const shBulkCtrl = document.getElementById('shBulkControls');
    if (shBulkCtrl) shBulkCtrl.style.display = 'none';
  }

  // 行
  // purchasereturn: _prLastData にキャッシュし、仕入先ソートを適用
  // sales:          _slLastData にキャッシュし、チェックボックス全選択に使用
  // shipping:       _shLastData にキャッシュし、チェックボックス全選択に使用
  // 他タブへ切替時: 選択・ソート状態をリセット
  if (currentSlipTab === 'purchasereturn') {
    _prLastData = [...data];
  } else if (currentSlipTab === 'sales') {
    _slLastData = [...data];
    // 選択状態を再同期（既存選択がデータに含まれない IDは除去）
    const validIds = new Set(data.map(r => r.id));
    _slSelectedIds.forEach(id => { if (!validIds.has(id)) _slSelectedIds.delete(id); });
    _slUpdateControls();
  } else if (currentSlipTab === 'shipping') {
    _shLastData = [...data];
    // 選択状態を再同期（存在しない IDを除去）
    const validShIds = new Set(data.map(r => r.id));
    _shSelectedIds.forEach(id => { if (!validShIds.has(id)) _shSelectedIds.delete(id); });
    _shUpdateControls();
  } else {
    _prSelectedIds = new Set();
    _prSortDir = 0;
    _prLastData = [];
  }

  const sorted = currentSlipTab === 'purchasereturn'
    ? _prSortedData(data)
    : [...data].sort((a, b) => {
        const da = currentSlipTab === 'purchase' ? (a.date||'') : (a.date||'');
        const db = currentSlipTab === 'purchase' ? (b.date||'') : (b.date||'');
        return db.localeCompare(da);
      });

  tbody.innerHTML = sorted.map(row => buildSlipRow(row)).join('');
}

function buildSlipRow(row) {
  const hasRevision = (row.revisions||[]).length > 0;
  const revBadge = hasRevision
    ? `<span class="slip-revised-badge"><i class="fa-solid fa-pen-to-square"></i> 修正済</span>`
    : '';

  // 承認ペンディングバッジ
  const aprReqs = APP_DATA.approvalRequests || [];
  const slipId = row.code || row.id;
  const pendingApr = row.pendingApprovalId
    ? aprReqs.find(r => r.id === row.pendingApprovalId && r.status === 'pending')
    : aprReqs.find(r => r.status === 'pending' && (
        r.detail?.slipId === slipId ||
        r.detail?.shipId === slipId ||
        r.detail?.code   === slipId
      ));
  const aprBadge = pendingApr
    ? `<span class="slip-appr-badge" onclick="event.stopPropagation();openApprovalDetail('${pendingApr.id}')" title="承認リクエスト: ${pendingApr.typeLabel}">
        <i class="fa-solid fa-clock"></i> 要承認
      </span>`
    : '';

  // ──────────────────────────────────────
  // 仕入伝票
  // ──────────────────────────────────────
  if (currentSlipTab === 'purchase') {
    const slipTotal = (row.lines || []).reduce((s, l) => s + (l.purchasePrice || 0), 0);
    const lineCount = (row.lines || []).length;
    const hasRev = (row.revisions||[]).length > 0;
    const revIcon = hasRev ? `<i class="fa-solid fa-circle-check" style="color:#e07b39;" title="修正済"></i>` : '—';
    // ③④ 要承認ラベル（承認待ちのときのみ）
    const stBadge = _slipStatusBadge(row.status, row.id, 'purchase');
    return `<tr class="slip-list-row${row.status === '承認待ち' ? ' slip-row-pending' : ''}" onclick="openSlipDetail('purchase','${row.id}')">
      <td><code style="font-size:12px;font-weight:bold;">${row.id}</code>${revBadge}</td>
      <td style="white-space:nowrap;">${row.date||'—'}</td>
      <td>${getSupplierName(row.supplier)}</td>
      <td style="font-size:12px;">${row.staff||'—'}</td>
      <td style="text-align:center;">${lineCount}点</td>
      <td style="text-align:right;font-weight:bold;color:var(--primary);">${formatPrice(slipTotal)}</td>
      <td style="font-size:12px;color:var(--text-muted);max-width:120px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">${row.note||'—'}</td>
      <td style="text-align:center;">${stBadge || revIcon}</td>
    </tr>`;

  // ──────────────────────────────────────
  // 出荷伝票
  // ──────────────────────────────────────
  } else if (currentSlipTab === 'shipping') {
    const stBadge = _slipStatusBadge(row.status, row.id, 'shipping');
    const isChecked = _shSelectedIds.has(row.id);
    // ⑧ 出荷伝票の伝票番号横に aprBadge を表示しない（誤表示削除）
    return `<tr id="sh-row-${row.id}" class="slip-list-row${pendingApr ? ' slip-row-pending' : ''}${isChecked ? ' sh-row-selected' : ''}" onclick="openSlipDetail('shipping','${row.id}')">
      <td style="text-align:center;padding:8px 4px;" onclick="event.stopPropagation()">
        <input type="checkbox" class="sh-row-chk" data-id="${row.id}"
          ${isChecked ? 'checked' : ''}
          onchange="shToggleRow('${row.id}', this.checked)"
          style="cursor:pointer;width:15px;height:15px;">
      </td>
      <td><code style="font-size:12px;font-weight:bold;">${row.id}</code>${revBadge}</td>
      <td style="white-space:nowrap;">${row.date||'—'}</td>
      <td>${getBuyerName(row.destination)}</td>
      <td style="text-align:center;">${row.items?.length||0}点</td>
      <td style="text-align:right;font-weight:bold;color:var(--primary);">${formatPrice(row.total)}</td>
      <td style="font-size:12px;color:var(--text-muted);max-width:120px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">${row.note||'—'}</td>
      <td>${stBadge}</td>
      <td style="text-align:center;">${hasRevision ? '<i class="fa-solid fa-circle-check" style="color:#e07b39;"></i>' : '—'}</td>
    </tr>`;

  // ──────────────────────────────────────
  // 売上返品
  // ──────────────────────────────────────
  } else if (currentSlipTab === 'salesreturn') {
    const stBadge = _slipStatusBadge(row.status, row.id, 'salesreturn');
    const itemCount = (row.items||[]).length;
    const saleAmount = getSalesReturnOriginalAmountInfo(row);
    return `<tr class="slip-list-row" onclick="openSalesReturnDetail('${row.id}')">
      <td><code style="font-size:12px;font-weight:bold;">${row.id}</code></td>
      <td style="white-space:nowrap;">${row.date||'—'}</td>
      <td>${getBuyerName(row.buyer)}</td>
      <td style="text-align:center;">${itemCount}点</td>
      <td style="text-align:right;font-weight:bold;color:#7c3aed;">${saleAmount.formatAmount(saleAmount.grandTotal)}</td>
      <td>${stBadge}</td>
      <td style="text-align:center;">
        <div style="display:flex;gap:6px;justify-content:center;">
          <button class="btn btn-outline btn-sm" onclick="event.stopPropagation();openSalesReturnDetail('${row.id}')">
            <i class="fa-solid fa-magnifying-glass"></i> 詳細
          </button>
          <button class="btn btn-primary btn-sm" onclick="event.stopPropagation();openSrInvoiceDirect('${row.id}')"
            style="background:#7c3aed;border-color:#7c3aed;gap:4px;">
            <i class="fa-solid fa-file-invoice"></i> 売上返品伝票
          </button>
        </div>
      </td>
    </tr>`;

  // ──────────────────────────────────────
  // 仕入返品
  // ──────────────────────────────────────
  } else if (currentSlipTab === 'purchasereturn') {
    const stBadge = _slipStatusBadge(row.status, row.id, 'purchasereturn');
    const isChecked = _prSelectedIds.has(row.id);
    const retTotal = getPurchaseReturnOriginalAmountInfo(row).subtotal;

    // 配送番号セル: 処理済のみ入力可、それ以外はグレーアウト disabled
    // 伝票に紐づく items の trackingNo を先頭から取得して表示
    const firstTracking = (row.items||[]).find(i => i.trackingNo)?.trackingNo || '';
    const isDone = row.status === '処理済';
    const trackingCell = isDone
      ? `<input type="text" class="form-control"
           style="font-size:11px;padding:3px 8px;width:140px;min-width:100px;"
           placeholder="配送番号を入力"
           value="${firstTracking.replace(/"/g,'&quot;')}"
           oninput="prUpdateTrackingFromList('${row.id}',this.value)"
           onclick="event.stopPropagation()">`
      : `<input type="text" class="form-control"
           style="font-size:11px;padding:3px 8px;width:140px;min-width:100px;
                  background:#f1f5f9;color:#9ca3af;cursor:not-allowed;border-color:#e2e8f0;"
           placeholder="配送番号を入力"
           value="${firstTracking.replace(/"/g,'&quot;')}"
           disabled>`;

    return `<tr class="slip-list-row${isChecked ? ' pr-row-selected' : ''}" id="pr-row-${row.id}" onclick="openPurchaseReturnDetail('${row.id}')">
      <td style="width:36px;text-align:center;padding:6px 4px;" onclick="event.stopPropagation()">
        <input type="checkbox" class="pr-row-chk" data-id="${row.id}"
          ${isChecked ? 'checked' : ''}
          onchange="prToggleRow('${row.id}',this.checked)"
          style="cursor:pointer;width:15px;height:15px;">
      </td>
      <td><code style="font-size:12px;font-weight:bold;">${row.id}</code></td>
      <td style="white-space:nowrap;">${row.date||'—'}</td>
      <td>${getSupplierName(row.supplier)}</td>
      <td style="text-align:center;">${(row.items||[]).length}点</td>
      <td style="text-align:right;font-weight:bold;color:var(--primary);">${formatPrice(retTotal)}</td>
      <td style="font-size:12px;color:var(--text-muted);">${row.reason||'—'}</td>
      <td>${stBadge}</td>
      <td onclick="event.stopPropagation()" style="white-space:nowrap;">${trackingCell}</td>
      <td style="text-align:center;">
        <div style="display:flex;gap:4px;justify-content:center;flex-wrap:wrap;">
          <button class="btn btn-outline btn-sm" onclick="event.stopPropagation();openPurchaseReturnDetail('${row.id}')">
            <i class="fa-solid fa-magnifying-glass"></i> 詳細
          </button>
          <button class="btn btn-primary btn-sm" onclick="event.stopPropagation();openPrInvoiceDirect('${row.id}')"
            style="background:#1d4ed8;border-color:#1d4ed8;gap:4px;">
            <i class="fa-solid fa-file-invoice"></i> 仕入返品伝票
          </button>
        </div>
      </td>
    </tr>`;

  // ──────────────────────────────────────
  // 売上伝票
  // ──────────────────────────────────────
  } else {
    const stBadge = _slipStatusBadge(row.status, row.id, 'sales');
    const isChecked = _slSelectedIds.has(row.id);
    // ⑧ 売上伝票の伝票番号横に aprBadge を表示しない（誤表示削除）
    return `<tr id="sl-row-${row.id}" class="slip-list-row${pendingApr ? ' slip-row-pending' : ''}${isChecked ? ' sl-row-selected' : ''}" onclick="openSlipDetail('sales','${row.id}')">
      <td style="text-align:center;padding:8px 4px;" onclick="event.stopPropagation()">
        <input type="checkbox" class="sl-row-chk" data-id="${row.id}"
          ${isChecked ? 'checked' : ''}
          onchange="slToggleRow('${row.id}', this.checked)"
          style="cursor:pointer;width:15px;height:15px;">
      </td>
      <td><code style="font-size:12px;font-weight:bold;">${row.id}</code>${revBadge}</td>
      <td style="white-space:nowrap;">${row.date||'—'}</td>
      <td>${getBuyerName(row.buyer)}</td>
      <td style="text-align:center;">${row.items?.length||0}点</td>
      <td style="text-align:right;font-weight:bold;color:var(--primary);">${formatSalePrice(row.total)}</td>
      <td style="font-size:12px;color:var(--text-muted);max-width:120px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">${row.note||'—'}</td>
      <td>${stBadge}</td>
      <td style="text-align:center;">${hasRevision ? '<i class="fa-solid fa-circle-check" style="color:#e07b39;"></i>' : '—'}</td>
    </tr>`;
  }
}

/** ステータスバッジ（統一表記）
 *  ⑦ 未処理は非表示。承認待ち/差戻しはクリックでポップアップ。
 */
function _slipStatusBadge(status, slipId, tabType) {
  // 未処理は非表示
  if (!status || status === '未処理') return '';
  const map = {
    '承認待ち': ['#d97706','#fffbeb','#fcd34d'],
    '差戻し':   ['#dc2626','#fef2f2','#fca5a5'],
    '処理済':   ['#16a34a','#f0fdf4','#86efac'],
    '承認済':   ['#16a34a','#f0fdf4','#86efac'],
  };
  const [col, bg, border] = map[status] || ['#64748b','#f1f5f9','#cbd5e1'];
  // 承認待ち/差戻しはクリックでダイアログ表示
  if ((status === '承認待ち' || status === '差戻し') && slipId) {
    return `<span style="display:inline-block;padding:2px 8px;border-radius:12px;font-size:11px;font-weight:600;background:${bg};color:${col};border:1px solid ${border};cursor:pointer;" onclick="event.stopPropagation();openStatusDetailModal('${slipId}','${tabType||currentSlipTab}')" title="クリックで詳細を表示">${status} <i class="fa-solid fa-circle-info" style="font-size:10px;"></i></span>`;
  }
  return `<span style="display:inline-block;padding:2px 8px;border-radius:12px;font-size:11px;font-weight:600;background:${bg};color:${col};border:1px solid ${border};">${status}</span>`;
}

// =====================================================
// 売上伝票一覧 ── チェックボックス選択・一括処理
// =====================================================

/**
 * 選択済み売上伝票ID の Set（伝票単位で管理）。
 * 空 = 未選択。タブ切替・データ更新時にリセットする。
 * @type {Set<string>}
 */
let _slSelectedIds = new Set();

/**
 * 現在表示中の売上伝票データを保持（チェックボックス全選択・選択件数に使用）。
 * @type {Array}
 */
let _slLastData = [];

// ── 全選択トグル ──────────────────────────────────────
function slToggleSelectAll(checked) {
  _slSelectedIds = new Set();
  if (checked) {
    _slLastData.forEach(r => _slSelectedIds.add(r.id));
  }
  // 各行チェックボックスを同期
  document.querySelectorAll('.sl-row-chk').forEach(chk => {
    chk.checked = checked;
    const tr = document.getElementById('sl-row-' + chk.dataset.id);
    if (tr) tr.classList.toggle('sl-row-selected', checked);
  });
  _slUpdateControls();
}

// ── 行選択トグル ──────────────────────────────────────
function slToggleRow(id, checked) {
  if (checked) {
    _slSelectedIds.add(id);
  } else {
    _slSelectedIds.delete(id);
  }
  const tr = document.getElementById('sl-row-' + id);
  if (tr) tr.classList.toggle('sl-row-selected', checked);

  // 全選択チェックボックスの indeterminate / checked を同期
  const allChk = document.getElementById('slSelectAll');
  if (allChk) {
    const total = _slLastData.length;
    allChk.checked       = _slSelectedIds.size === total && total > 0;
    allChk.indeterminate = _slSelectedIds.size > 0 && _slSelectedIds.size < total;
  }
  _slUpdateControls();
}

// ── ボタン活性制御・件数表示の更新 ───────────────────
function _slUpdateControls() {
  const count = _slSelectedIds.size;

  // 選択件数ラベル
  const badge = document.getElementById('slSelectCountBadge');
  if (badge) {
    badge.textContent = count > 0 ? `${count}件選択中` : '';
    badge.style.display = count > 0 ? '' : 'none';
  }

  // 請求書発行ボタン
  const invoiceBtn = document.getElementById('slBulkInvoiceBtn');
  if (invoiceBtn) {
    invoiceBtn.disabled      = count === 0;
    invoiceBtn.style.opacity = count > 0 ? '1' : '0.45';
  }
}

// ── 選択状態リセット（タブ切替・データ更新時に呼ぶ） ───
function _slResetSelection() {
  _slSelectedIds = new Set();
  _slLastData    = [];
  _slUpdateControls();
  // 全選択チェックもリセット
  const allChk = document.getElementById('slSelectAll');
  if (allChk) { allChk.checked = false; allChk.indeterminate = false; }
}

// =====================================================
// 出荷伝票一覧 ── チェックボックス選択・CSV出力制御
// =====================================================

/**
 * 選択済み出荷伝票ID の Set（伝票単位で管理）。
 * 空 = 未選択。タブ切替・データ更新時にリセットする。
 * @type {Set<string>}
 */
let _shSelectedIds = new Set();

/**
 * 現在表示中の出荷伝票データを保持（全選択・件数表示に使用）。
 * @type {Array}
 */
let _shLastData = [];

// ── 全選択トグル ──────────────────────────────────────
function shToggleSelectAll(checked) {
  _shSelectedIds = new Set();
  if (checked) {
    _shLastData.forEach(r => _shSelectedIds.add(r.id));
  }
  document.querySelectorAll('.sh-row-chk').forEach(chk => {
    chk.checked = checked;
    const tr = document.getElementById('sh-row-' + chk.dataset.id);
    if (tr) tr.classList.toggle('sh-row-selected', checked);
  });
  _shUpdateControls();
}

// ── 行選択トグル ──────────────────────────────────────
function shToggleRow(id, checked) {
  if (checked) {
    _shSelectedIds.add(id);
  } else {
    _shSelectedIds.delete(id);
  }
  const tr = document.getElementById('sh-row-' + id);
  if (tr) tr.classList.toggle('sh-row-selected', checked);

  // ヘッダー全選択チェックボックスの checked / indeterminate を同期
  const allChk = document.getElementById('shSelectAll');
  if (allChk) {
    const total = _shLastData.length;
    allChk.checked       = _shSelectedIds.size === total && total > 0;
    allChk.indeterminate = _shSelectedIds.size > 0 && _shSelectedIds.size < total;
  }
  _shUpdateControls();
}

// ── 件数バッジ更新 ────────────────────────────────────
function _shUpdateControls() {
  const count = _shSelectedIds.size;
  const badge = document.getElementById('shSelectCountBadge');
  if (badge) {
    badge.textContent = count > 0 ? `${count}件選択中` : '';
    badge.style.display = count > 0 ? '' : 'none';
  }
}

// ── 選択状態リセット（タブ切替・データ更新時に呼ぶ） ───
function _shResetSelection() {
  _shSelectedIds = new Set();
  _shLastData    = [];
  _shUpdateControls();
  const allChk = document.getElementById('shSelectAll');
  if (allChk) { allChk.checked = false; allChk.indeterminate = false; }
}

// =====================================================
// 仕入返品一覧 ── ソート・選択・請求書一括発行
// =====================================================

/** 選択済み仕入返品ID の Set */
let _prSelectedIds = new Set();

/** ソート状態: 0=デフォルト, 1=昇順, -1=降順 */
let _prSortDir = 0;

/** ソート用ヘルパー: 現在フィルタ済みデータを保持 */
let _prLastData = [];

// ── 全選択トグル ──
function prToggleSelectAll(checked) {
  _prSelectedIds = new Set();
  if (checked) {
    _prLastData.forEach(r => _prSelectedIds.add(r.id));
  }
  // 各行チェックボックスを同期
  document.querySelectorAll('.pr-row-chk').forEach(chk => {
    chk.checked = checked;
    const tr = document.getElementById('pr-row-' + chk.dataset.id);
    if (tr) tr.classList.toggle('pr-row-selected', checked);
  });
  _prUpdateInvoiceBtn();
}

// ── 行選択トグル ──
function prToggleRow(id, checked) {
  if (checked) {
    _prSelectedIds.add(id);
  } else {
    _prSelectedIds.delete(id);
  }
  const tr = document.getElementById('pr-row-' + id);
  if (tr) tr.classList.toggle('pr-row-selected', checked);

  // 全選択チェックボックスの状態を同期
  const allChk = document.getElementById('prSelectAll');
  if (allChk) {
    const total = _prLastData.length;
    allChk.checked       = _prSelectedIds.size === total && total > 0;
    allChk.indeterminate = _prSelectedIds.size > 0 && _prSelectedIds.size < total;
  }
  _prUpdateInvoiceBtn();
}

// ── 仕入先ソートトグル ──
function prToggleSort() {
  // 0 → 1(昇順) → -1(降順) → 0(デフォルト)
  _prSortDir = _prSortDir === 0 ? 1 : _prSortDir === 1 ? -1 : 0;

  const icon = document.getElementById('prSortIcon');
  if (icon) {
    icon.textContent = _prSortDir === 1 ? '▲' : _prSortDir === -1 ? '▼' : '';
    icon.className   = 'pr-sort-icon' + (_prSortDir !== 0 ? ' pr-sort-active' : '');
  }

  // 現在のデータを再ソートして tbody のみ再描画
  const tbody = document.getElementById('slipListBody');
  if (!tbody || _prLastData.length === 0) return;

  const sorted = _prSortedData(_prLastData);
  tbody.innerHTML = sorted.map(row => buildSlipRow(row)).join('');
}

// ── ソート適用 ──
function _prSortedData(data) {
  if (_prSortDir === 0) return [...data];
  return [...data].sort((a, b) => {
    const na = getSupplierName(a.supplier) || '';
    const nb = getSupplierName(b.supplier) || '';
    return _prSortDir * na.localeCompare(nb, 'ja');
  });
}

// ── 仕入返品伝票ボタン注入（集計バーの右端） ──
function _prInjectInvoiceBtn() {
  // 既存ボタンがあれば削除
  const old = document.getElementById('prBulkInvoiceBtn');
  if (old) old.remove();

  const bar = document.getElementById('slipSummaryBar');
  if (!bar) return;

  const btn = document.createElement('button');
  btn.id        = 'prBulkInvoiceBtn';
  btn.className = 'btn btn-primary btn-sm pr-bulk-invoice-btn';
  btn.innerHTML = '<i class="fa-solid fa-file-invoice"></i> 仕入返品伝票発行';
  btn.onclick   = prOpenBulkInvoiceModal;
  bar.appendChild(btn);
}

// ── ボタン活性制御 ──
function _prUpdateInvoiceBtn() {
  const btn = document.getElementById('prBulkInvoiceBtn');
  if (!btn) return;
  const hasSelection = _prSelectedIds.size > 0;
  btn.disabled = !hasSelection;
  btn.style.opacity = hasSelection ? '1' : '0.45';
}

// ── 仕入返品伝票発行確認モーダルを開く ──
function prOpenBulkInvoiceModal() {
  if (_prSelectedIds.size === 0) {
    showToast('warning', '明細を選択してください', '仕入返品伝票を発行する返品データを1件以上選択してください。');
    return;
  }

  const selected = _prLastData.filter(r => _prSelectedIds.has(r.id));

  // 仕入先ごとにグルーピング
  const groupMap = {};
  selected.forEach(r => {
    const key  = r.supplier || '_none';
    const name = getSupplierName(r.supplier) || '仕入先不明';
    if (!groupMap[key]) groupMap[key] = { name, rows: [] };
    groupMap[key].rows.push(r);
  });

  // プレビュー生成
  const previewHtml = Object.values(groupMap).map(g => {
    const total = g.rows.reduce((sum, record) => sum + getPurchaseReturnOriginalAmountInfo(record).subtotal, 0);
    const rowsHtml = g.rows.map(r => {
      const purchaseAmount = getPurchaseReturnOriginalAmountInfo(r);
      return `<tr>
        <td style="font-size:11px;font-family:monospace;">${r.id}</td>
        <td style="font-size:11px;">${r.date||'—'}</td>
        <td style="font-size:11px;">${purchaseAmount.items.length}点</td>
        <td style="font-size:11px;color:var(--text-muted);">${r.reason||'—'}</td>
        <td style="font-size:11px;text-align:right;font-weight:bold;">${purchaseAmount.formatAmount(purchaseAmount.subtotal)}</td>
      </tr>`;
    }).join('');
    return `
      <div class="pr-invoice-group">
        <div class="pr-invoice-group-header">
          <i class="fa-solid fa-industry"></i> ${g.name}
          <span class="pr-invoice-group-count">${g.rows.length}件</span>
          <span class="pr-invoice-group-total">${formatPrice(total)}</span>
        </div>
        <table class="pr-invoice-preview-table">
          <thead><tr>
            <th>仕入返品伝票番号</th><th>返品日</th><th>点数</th><th>返品理由</th><th style="text-align:right;">仕入金額</th>
          </tr></thead>
          <tbody>${rowsHtml}</tbody>
        </table>
      </div>`;
  }).join('');

  const body = document.getElementById('prBulkInvoicePreview');
  if (body) body.innerHTML = previewHtml;

  const countEl = document.getElementById('prBulkInvoiceCount');
  if (countEl) countEl.textContent = `${selected.length}件 / ${Object.keys(groupMap).length}仕入先`;

  const modal = document.getElementById('prBulkInvoiceModal');
  if (modal) modal.classList.remove('hidden');
}

// ── モーダルを閉じる ──
function prCloseBulkInvoiceModal() {
  const modal = document.getElementById('prBulkInvoiceModal');
  if (modal) modal.classList.add('hidden');
}

// ── CSV一括ダウンロード ──
function prBulkDownloadCSV() {
  const selected = _prLastData.filter(r => _prSelectedIds.has(r.id));
  if (selected.length === 0) return;

  const bom  = '\uFEFF';
  const headers = ['仕入先', '仕入返品伝票番号', '返品日', '商品コード', 'ブランド', 'モデル名', '型番', 'シリアル', '仕入金額', '返品理由', 'ステータス'];
  const dataRows = [];
  selected.forEach(r => {
    const supplierName = getSupplierName(r.supplier);
    const purchaseAmount = getPurchaseReturnOriginalAmountInfo(r);
    (purchaseAmount.items.length > 0 ? purchaseAmount.items : [{ code:'—', brand:'—', model:'—', ref:'—', serial:'—', purchasePrice:0 }]).forEach(it => {
      dataRows.push([
        supplierName, r.id, r.date||'', it.code||'', it.brand||'', it.model||'',
        it.ref||'', it.serial||'', it.purchasePrice||0, r.reason||'', r.status||''
      ]);
    });
  });

  const csv  = bom + [headers, ...dataRows].map(r =>
    r.map(v => `"${String(v).replace(/"/g,'""')}"`).join(',')
  ).join('\r\n');

  const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' });
  const url  = URL.createObjectURL(blob);
  const a    = document.createElement('a');
  a.href = url; a.download = `仕入返品伝票_${new Date().toISOString().slice(0,10)}.csv`;
  a.click();
  URL.revokeObjectURL(url);
  prCloseBulkInvoiceModal();
  showToast('success', 'CSVダウンロード', `${selected.length}件の仕入返品データを出力しました。`);
}

// ── 一括印刷 ──
/** @deprecated 雛形適用前の仕入返品一括帳票。比較参照用に保持 */
function prBulkPrintLegacy() {
  const selected = _prLastData.filter(r => _prSelectedIds.has(r.id));
  if (selected.length === 0) return;

  // 仕入先ごとにグルーピング
  const groupMap = {};
  selected.forEach(r => {
    const key  = r.supplier || '_none';
    const name = getSupplierName(r.supplier) || '仕入先不明';
    if (!groupMap[key]) groupMap[key] = { name, rows: [] };
    groupMap[key].rows.push(r);
  });

  // 自社情報
  const company = {
    name:    '株式会社ウォッチプレミアム',
    zip:     '〒105-0001',
    address: '東京都港区虎ノ門1-2-3 ウォッチビル5F',
    tel:     '03-1234-0000',
    invoice: 'T1234560000',
  };

  const today = new Date().toLocaleDateString('ja-JP', { year:'numeric', month:'long', day:'numeric' });

  const invoicePages = Object.values(groupMap).map(g => {
    const itemRows = g.rows.flatMap(r =>
      (r.items||[{ code:'—', brand:'—', model:'—', ref:'—', serial:'—', purchasePrice:0 }]).map(it => `
        <tr>
          <td>${r.id}</td>
          <td>${r.date||'—'}</td>
          <td>${it.code||'—'}</td>
          <td>${it.brand||'—'} ${it.model||''}</td>
          <td>${it.ref||'—'}</td>
          <td>${it.serial||'—'}</td>
          <td style="text-align:right;">¥${(it.purchasePrice||0).toLocaleString('ja-JP')}</td>
          <td>${r.reason||'—'}</td>
        </tr>`)
    ).join('');
    const total = g.rows.reduce((s,r)=>s+(r.items||[]).reduce((ss,it)=>ss+(it.purchasePrice||0),0),0);
    const sup   = (APP_DATA.suppliers||[]).find(s => getSupplierName(g.rows[0]?.supplier) === s.name) ||
                  (APP_DATA.suppliers||[]).find(s => s.code === g.rows[0]?.supplier);

    return `
      <div class="invoice-page">
        <div class="inv-header-row">
          <div>
            <div class="inv-title">仕入返品 請求書</div>
            <div style="font-size:11px;color:#555;">発行日：${today}</div>
          </div>
          <div class="inv-company">
            <div style="font-weight:700;">${company.name}</div>
            <div style="font-size:10px;">${company.zip} ${company.address}</div>
            <div style="font-size:10px;">TEL: ${company.tel}　適格請求書番号: ${company.invoice}</div>
          </div>
        </div>
        <div class="inv-to">
          <b>返品先：${g.name}</b>
          ${sup ? `<span style="font-size:10px;color:#555;margin-left:8px;">${sup.address||''}</span>` : ''}
        </div>
        <table class="inv-table">
          <thead><tr>
            <th>返品伝票番号</th><th>返品日</th><th>商品コード</th>
            <th>商品名</th><th>型番</th><th>シリアル</th>
            <th style="text-align:right;">仕入金額</th><th>返品理由</th>
          </tr></thead>
          <tbody>${itemRows}</tbody>
          <tfoot><tr>
            <td colspan="6" style="text-align:right;font-weight:700;">合計金額</td>
            <td style="text-align:right;font-weight:700;">¥${total.toLocaleString('ja-JP')}</td>
            <td></td>
          </tr></tfoot>
        </table>
      </div>`;
  }).join('<div class="page-break"></div>');

  const html = `<!DOCTYPE html><html lang="ja"><head><meta charset="UTF-8">
    <title>仕入返品 請求書</title>
    <style>
      body { font-family: 'Helvetica Neue',sans-serif; font-size:12px; margin:0; color:#111; }
      .invoice-page { padding:32px 36px; max-width:960px; margin:0 auto; }
      .inv-header-row { display:flex; justify-content:space-between; align-items:flex-start; margin-bottom:20px; }
      .inv-title { font-size:20px; font-weight:900; color:#1a3a5c; margin-bottom:4px; }
      .inv-company { text-align:right; }
      .inv-to { margin-bottom:16px; padding:8px 12px; background:#f0f4f8; border-left:4px solid #1a3a5c; border-radius:4px; }
      .inv-table { width:100%; border-collapse:collapse; font-size:11px; margin-top:12px; }
      .inv-table th { background:#1a3a5c; color:#fff; padding:6px 8px; text-align:left; }
      .inv-table td { padding:5px 8px; border-bottom:1px solid #ddd; }
      .inv-table tfoot td { background:#f0f4f8; font-size:13px; }
      .page-break { page-break-after:always; }
      @media print { button { display:none; } .page-break { page-break-after:always; } }
    </style></head><body>
    ${invoicePages}
    <script>window.onload=function(){window.print();}<\/script>
    </body></html>`;

  const w = window.open('', '_blank');
  if (w) { w.document.write(html); w.document.close(); }
  prCloseBulkInvoiceModal();
}

/** 選択中の仕入返品を雛形準拠の伝票として一括印刷する */
function prBulkPrint() {
  const selected = _prLastData.filter(record => _prSelectedIds.has(record.id));
  if (selected.length === 0) {
    showToast('warning', '明細を選択してください', '仕入返品伝票を発行する返品データを1件以上選択してください。');
    return;
  }
  selected.forEach(markPurchaseReturnDocumentIssued);
  _openTemplatePrintWindow('仕入返品伝票', selected.map(buildPurchaseReturnRecordTemplateHTML).join(''));
  prCloseBulkInvoiceModal();
}

// ── renderSlipListをラップしてpurchasereturn時のデータを保持 ──
// ※ function宣言の再代入は不可のため、renderSlipList内の先頭で
//    _prLastData のセットを行う専用フックとして別途呼び出す形を採用。
//    実際には renderSlipList 冒頭の purchasereturn 分岐時に
//    _prLastData = data としてキャッシュするよう renderSlipList 内で処理。

// =====================================================
// 売上伝票 ── 一括請求書発行
// =====================================================

// =====================================================
// 出荷伝票 ── 出荷明細書発行
// =====================================================

/** 保存済み仕入伝票を雛形準拠の帳票HTMLへ変換する。 */
function buildPurchaseRecordTemplateHTML(slip) {
  const supplier = (APP_DATA.suppliers || []).find(record => record.code === slip.supplier)
    || { name: getSupplierName(slip.supplier) || '（仕入先未設定）' };
  const items = (slip.lines || []).map((line, index) => {
    const detailRecord = line.productDetail || {};
    const inventoryItem = (APP_DATA.inventory || []).find(item => item.code === line.code) || {};
    const detail = [
      [detailRecord.brand || inventoryItem.brand, detailRecord.model || inventoryItem.model].filter(Boolean).join(' / '),
      [(detailRecord.ref || inventoryItem.ref) && `型番: ${detailRecord.ref || inventoryItem.ref}`, (detailRecord.serial || inventoryItem.serial) && `シリアル: ${detailRecord.serial || inventoryItem.serial}`].filter(Boolean).join('　'),
      (line.sku || inventoryItem.sku) ? `SKU: ${line.sku || inventoryItem.sku}` : '',
      (detailRecord.accessories || inventoryItem.accessories)?.length ? `付属品: ${(detailRecord.accessories || inventoryItem.accessories).join('・')}` : '',
      detailRecord.note || inventoryItem.note ? `備考: ${detailRecord.note || inventoryItem.note}` : '',
    ].filter(Boolean).join('\n') || line.code || '—';
    return { no: index + 1, detail, amount: Number(line.purchasePrice) || 0, code: line.code || '' };
  });
  const note = [
    slip.staff ? `仕入担当者：${slip.staff}` : '',
    slip.note || '',
  ].filter(Boolean).join('\n');
  return buildTemplateStyleSlipDocument({
    title: '仕入伝票',
    slipId: slip.id,
    transactionDate: slip.date,
    transactionDateLabel: '仕入日',
    counterpartyLabel: '仕入先',
    counterparty: supplier,
    items,
    note,
    formatAmount: amount => formatPrice(amount),
    currencyLabel: 'JPY（円）',
    taxMode: 'none',
    includeBank: false,
    summaryMessage: '商品代金として、弊社より下記金額をお支払いいたします。',
    amountCaption: '仕入合計金額',
  });
}

/**
 * 保存済み出荷伝票を雛形準拠の帳票HTMLへ変換する。
 */
function _shBulkMeisai() {
  const base = _shLastData.length > 0 ? _shLastData : (APP_DATA.shipments || []);
  const target = _shSelectedIds.size > 0
    ? base.filter(slip => _shSelectedIds.has(slip.id))
    : base;
  if (target.length === 0) {
    showToast('warn', '出荷伝票発行', '対象の出荷伝票がありません');
    return;
  }

  const documents = target.map(buildShipmentRecordTemplateHTML).join('');
  _openTemplatePrintWindow('出荷伝票', documents);
}

function buildShipmentRecordTemplateHTML(slip) {
  const destination = (APP_DATA.buyers || []).find(buyer => buyer.code === slip.destination)
    || { name: slip.destination || '（出荷先未設定）' };
  const items = (slip.items || []).map((line, index) => {
    const inventoryItem = (APP_DATA.inventory || []).find(item => item.code === line.code) || {};
    const detail = [
      [line.brand || inventoryItem.brand, line.model || inventoryItem.model].filter(Boolean).join(' / '),
      [inventoryItem.ref && `型番: ${inventoryItem.ref}`, inventoryItem.serial && `シリアル: ${inventoryItem.serial}`].filter(Boolean).join('　'),
      inventoryItem.accessories?.length ? `付属品: ${inventoryItem.accessories.join('・')}` : '',
      inventoryItem.note ? `備考: ${inventoryItem.note}` : '',
    ].filter(Boolean).join('\n') || line.code || '—';
    return { no: index + 1, detail, amount: Number(inventoryItem.purchasePrice) || 0, code: line.code || '' };
  });
  return buildTemplateStyleSlipDocument({
    title: '出荷伝票',
    slipId: slip.id,
    transactionDate: slip.date,
    transactionDateLabel: '出荷日',
    counterpartyLabel: '出荷先',
    counterparty: destination,
    items,
    note: slip.note || '',
    formatAmount: amount => formatPrice(amount),
    currencyLabel: 'JPY（円）',
    taxMode: 'none',
    includeBank: false,
    summaryMessage: '下記商品を出荷いたします。',
    amountCaption: '仕入金額合計',
  });
}

function _openTemplatePrintWindow(title, documents) {
  const printWindow = window.open('', '_blank');
  if (!printWindow) {
    showToast('warn', `${title}発行`, 'ポップアップがブロックされました');
    return;
  }
  printWindow.document.write(`<!DOCTYPE html><html lang="ja"><head><meta charset="UTF-8"><title>${_escHtml(title)}</title></head>
    <body style="margin:0;background:#eee;">${documents}<script>window.onload=function(){window.print();}<\/script></body></html>`);
  printWindow.document.close();
}

/** 雛形帳票を、ブラウザで再表示・印刷できるHTMLとして保存する。 */
function _downloadTemplateDocument(title, filename, documentHtml) {
  if (!documentHtml) {
    showToast('warning', '帳票ダウンロード', '保存できる帳票内容がありません。');
    return;
  }
  const html = `<!DOCTYPE html><html lang="ja"><head><meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>${_escHtml(title)}</title></head>
    <body style="margin:0;background:#eee;">${documentHtml}</body></html>`;
  const blob = new Blob(['\uFEFF', html], { type: 'text/html;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = String(filename || `${title}.html`).replace(/[\\/:*?"<>|]/g, '_');
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  setTimeout(() => URL.revokeObjectURL(url), 0);
  showToast('success', 'ダウンロードしました', `${title}を保存しました。`);
}

/** @deprecated 雛形変更前の出荷明細書。比較参照用に保持 */
function _shBulkMeisaiLegacy() {
  // 対象データを決定: 選択あり → 選択分、なし → 表示中全件
  const base   = _shLastData.length > 0 ? _shLastData : (APP_DATA.shipments || []);
  const target = _shSelectedIds.size > 0
    ? base.filter(s => _shSelectedIds.has(s.id))
    : base;

  if (target.length === 0) {
    showToast('warn', '明細書発行', '対象の出荷伝票がありません');
    return;
  }

  // 自社情報（ヘッダー表示用）
  const company = APP_DATA.company || {
    name:    '株式会社ウォッチプレミアム',
    zip:     '〒105-0001',
    address: '東京都港区虎ノ門1-2-3 ウォッチビル5F',
    tel:     '03-1234-0000',
  };

  const today = new Date().toLocaleDateString('ja-JP',
    { year:'numeric', month:'long', day:'numeric' });

  // 伝票ごとに1ページ生成
  const pages = target.map(slip => {
    const destObj = (APP_DATA.buyers || []).find(b => b.code === slip.destination);
    const destName = destObj ? destObj.name : (slip.destination || '—');
    const destAddr = destObj ? (destObj.address || '') : '';

    const itemRows = (slip.items || []).map((it, idx) => `
      <tr>
        <td style="text-align:center;">${idx + 1}</td>
        <td><code style="font-size:11px;">${it.code || '—'}</code></td>
        <td>${it.brand || '—'} ${it.model || ''}</td>
        <td style="text-align:right;">${formatSalePrice(it.wholesale || 0)}</td>
      </tr>`).join('');

    const totalWholesale = (slip.items || [])
      .reduce((s, it) => s + (it.wholesale || 0), 0);

    const statusBadge = slip.status && slip.status !== '未処理'
      ? `<span style="display:inline-block;padding:2px 8px;border-radius:4px;font-size:10px;font-weight:700;background:#f0fdf4;color:#16a34a;border:1px solid #86efac;">${slip.status}</span>`
      : '';

    return `
      <div class="meisai-page">
        <!-- ページヘッダー -->
        <div class="meisai-header-row">
          <div>
            <div class="meisai-title">出 荷 明 細 書</div>
            <div style="font-size:11px;color:#555;margin-top:4px;">発行日：${today}</div>
          </div>
          <div class="meisai-company">
            <div style="font-weight:700;font-size:13px;">${company.name || '—'}</div>
            <div style="font-size:10px;color:#555;margin-top:2px;">${company.zip || ''} ${company.address || ''}</div>
            <div style="font-size:10px;color:#555;">TEL: ${company.tel || '—'}</div>
          </div>
        </div>

        <!-- 伝票情報ブロック -->
        <div class="meisai-meta-row">
          <div class="meisai-meta-block">
            <div class="meisai-meta-label">伝票番号</div>
            <div class="meisai-meta-val"><code style="font-size:13px;font-weight:800;">${slip.id}</code></div>
          </div>
          <div class="meisai-meta-block">
            <div class="meisai-meta-label">出荷日</div>
            <div class="meisai-meta-val" style="font-weight:700;">${slip.date || '—'}</div>
          </div>
          <div class="meisai-meta-block">
            <div class="meisai-meta-label">ステータス</div>
            <div class="meisai-meta-val">${statusBadge || '—'}</div>
          </div>
        </div>

        <!-- 出荷先 -->
        <div class="meisai-dest-block">
          <div class="meisai-dest-label">出 荷 先</div>
          <div class="meisai-dest-name">${destName} 御中</div>
          ${destAddr ? `<div class="meisai-dest-addr">${destAddr}</div>` : ''}
        </div>

        <!-- 商品明細テーブル -->
        <table class="meisai-table">
          <thead>
            <tr>
              <th style="width:36px;text-align:center;">No.</th>
              <th style="width:130px;">商品コード</th>
              <th>商品名</th>
              <th style="width:120px;text-align:right;">卸価格</th>
            </tr>
          </thead>
          <tbody>${itemRows}</tbody>
          <tfoot>
            <tr>
              <td colspan="3" style="text-align:right;font-weight:700;">合計</td>
              <td style="text-align:right;font-weight:700;">${formatSalePrice(totalWholesale)}</td>
            </tr>
          </tfoot>
        </table>

        <!-- 備考 -->
        ${slip.note ? `
        <div class="meisai-note-block">
          <span class="meisai-note-label">備考：</span>
          <span class="meisai-note-text">${slip.note}</span>
        </div>` : ''}

        <!-- フッター -->
        <div class="meisai-footer">
          <span>本明細書は出荷内容の確認用です。支払・請求条件については別途ご確認ください。</span>
        </div>
      </div>`;
  }).join('<div class="page-break"></div>');

  // 印刷用HTML（請求・支払情報は一切含まない）
  const html = `<!DOCTYPE html>
<html lang="ja">
<head>
  <meta charset="UTF-8">
  <title>出荷明細書</title>
  <style>
    *, *::before, *::after { box-sizing: border-box; }
    body {
      font-family: 'Hiragino Kaku Gothic ProN', 'Helvetica Neue', sans-serif;
      font-size: 12px;
      margin: 0;
      color: #111;
      background: #fff;
    }
    .meisai-page {
      padding: 28px 36px;
      max-width: 960px;
      margin: 0 auto;
    }

    /* ── ヘッダー ── */
    .meisai-header-row {
      display: flex;
      justify-content: space-between;
      align-items: flex-start;
      border-bottom: 2px solid #1a3a5c;
      padding-bottom: 12px;
      margin-bottom: 16px;
    }
    .meisai-title {
      font-size: 22px;
      font-weight: 900;
      color: #1a3a5c;
      letter-spacing: 4px;
    }
    .meisai-company { text-align: right; }

    /* ── 伝票メタ情報 ── */
    .meisai-meta-row {
      display: flex;
      gap: 24px;
      background: #f8fafc;
      border: 1px solid #e2e8f0;
      border-radius: 6px;
      padding: 10px 16px;
      margin-bottom: 16px;
    }
    .meisai-meta-block { min-width: 100px; }
    .meisai-meta-label {
      font-size: 9px;
      font-weight: 700;
      text-transform: uppercase;
      letter-spacing: 0.6px;
      color: #6b7280;
      margin-bottom: 3px;
    }
    .meisai-meta-val { font-size: 13px; }

    /* ── 出荷先 ── */
    .meisai-dest-block {
      border-left: 4px solid #1a3a5c;
      padding: 8px 14px;
      background: #f0f4f8;
      border-radius: 0 4px 4px 0;
      margin-bottom: 18px;
    }
    .meisai-dest-label {
      font-size: 9px;
      font-weight: 700;
      letter-spacing: 1px;
      color: #6b7280;
      margin-bottom: 4px;
    }
    .meisai-dest-name { font-size: 16px; font-weight: 700; }
    .meisai-dest-addr { font-size: 11px; color: #555; margin-top: 2px; }

    /* ── 明細テーブル ── */
    .meisai-table {
      width: 100%;
      border-collapse: collapse;
      font-size: 12px;
      margin-bottom: 14px;
    }
    .meisai-table th {
      background: #1a3a5c;
      color: #fff;
      padding: 7px 10px;
      text-align: left;
      font-size: 11px;
    }
    .meisai-table td {
      padding: 6px 10px;
      border-bottom: 1px solid #e5e7eb;
      vertical-align: middle;
    }
    .meisai-table tbody tr:nth-child(even) td { background: #f9fafb; }
    .meisai-table tfoot td {
      background: #f0f4f8;
      font-size: 13px;
      padding: 8px 10px;
      border-top: 2px solid #1a3a5c;
    }

    /* ── 備考 ── */
    .meisai-note-block {
      background: #fffbeb;
      border: 1px solid #fcd34d;
      border-radius: 4px;
      padding: 8px 12px;
      font-size: 11px;
      margin-bottom: 14px;
    }
    .meisai-note-label { font-weight: 700; color: #92400e; }

    /* ── フッター ── */
    .meisai-footer {
      border-top: 1px solid #e5e7eb;
      padding-top: 8px;
      font-size: 10px;
      color: #9ca3af;
      text-align: center;
    }

    /* ── 印刷 ── */
    .page-break { page-break-after: always; }
    @media print {
      button { display: none; }
      .page-break { page-break-after: always; }
      body { margin: 0; }
    }
  </style>
</head>
<body>
  ${pages}
  <script>window.onload = function() { window.print(); };<\/script>
</body>
</html>`;

  const w = window.open('', '_blank');
  if (w) {
    w.document.write(html);
    w.document.close();
  } else {
    showToast('warn', '明細書発行', 'ポップアップがブロックされました');
  }
}

// =====================================================

/**
 * 選択中の売上伝票（なければ全件）を雛形準拠の請求書として印刷する。
 */
function _slBulkInvoice() {
  const base = _slLastData.length > 0 ? _slLastData : (APP_DATA.sales || []);
  const target = _slSelectedIds.size > 0
    ? base.filter(slip => _slSelectedIds.has(slip.id))
    : base;
  if (target.length === 0) {
    showToast('warn', '請求書発行', '対象の伝票がありません');
    return;
  }

  const documents = target.map(buildSalesRecordTemplateHTML).join('');
  _openTemplatePrintWindow('請求書', documents);
}

function buildSalesRecordTemplateHTML(slip) {
  const buyer = (APP_DATA.buyers || []).find(record => record.code === slip.buyer)
    || { name: getBuyerName(slip.buyer) || '（販売先未設定）' };
  const inputCurrency = slip.inputCurrency === 'JPY' ? 'JPY' : 'USD';
  const rate = Number(slip.usdJpyRate) > 0 ? Number(slip.usdJpyRate) : getSalesUsdRate();
  const formatAmount = amount => inputCurrency === 'JPY'
    ? formatPrice(Math.round((Number(amount) || 0) * rate))
    : formatSalePrice(amount);
  const items = (slip.items || []).filter(item => !item.returnType).map((line, index) => {
    const inventoryItem = (APP_DATA.inventory || []).find(item => item.code === line.code) || {};
    const detail = [
      [line.brand || inventoryItem.brand, line.model || inventoryItem.model].filter(Boolean).join(' / '),
      [(line.ref || inventoryItem.ref) && `型番: ${line.ref || inventoryItem.ref}`, (line.serial || inventoryItem.serial) && `シリアル: ${line.serial || inventoryItem.serial}`].filter(Boolean).join('　'),
      inventoryItem.accessories?.length ? `付属品: ${inventoryItem.accessories.join('・')}` : '',
      inventoryItem.note ? `備考: ${inventoryItem.note}` : '',
    ].filter(Boolean).join('\n') || line.code || '—';
    return { no: index + 1, detail, amount: Number(line.salePrice) || 0, code: line.code || '' };
  });

  return buildTemplateStyleSlipDocument({
    title: '請求書',
    slipId: slip.id,
    transactionDate: slip.date,
    transactionDateLabel: '売上日',
    counterpartyLabel: 'ご請求先',
    counterparty: buyer,
    items,
    note: slip.note || '',
    formatAmount,
    currencyLabel: inputCurrency === 'JPY' ? 'JPY（円）' : 'USD',
    taxMode: slip.taxFree ? 'exempt' : 'standard',
    includeBank: true,
    summaryMessage: '商品代金として、下記金額をご請求申し上げます。',
    amountCaption: slip.taxFree ? '合計金額（免税）' : '合計金額（税込）',
    currencyNote: `基準通貨：USD　換算レート：1 USD = ¥${rate.toLocaleString('ja-JP', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`,
  });
}

/** @deprecated 雛形変更前の一括請求書。比較参照用に保持 */
function _slBulkInvoiceLegacy() {
  // 対象データを取得: 選択あり→選択分、なし→表示中全件
  const base = _slLastData.length > 0 ? _slLastData : (APP_DATA.sales || []);
  const target = _slSelectedIds.size > 0
    ? base.filter(s => _slSelectedIds.has(s.id))
    : base;

  if (target.length === 0) {
    showToast('warn', '請求書発行', '対象の伝票がありません');
    return;
  }

  // 販売先でグループ化
  const groups = {};
  target.forEach(slip => {
    const key = slip.buyer || '不明';
    if (!groups[key]) groups[key] = { name: getBuyerName(slip.buyer), rows: [] };
    groups[key].rows.push(slip);
  });

  const company = APP_DATA.company || {};
  const today   = new Date().toLocaleDateString('ja-JP', { year:'numeric', month:'2-digit', day:'2-digit' });

  const invoicePages = Object.values(groups).map(g => {
    const itemRows = g.rows.map(slip =>
      (slip.items || [{ code:'—', brand:'—', model:'—', ref:'—', serial:'—', salePrice:0 }]).map(it => `
        <tr>
          <td>${slip.id}</td>
          <td>${slip.date||'—'}</td>
          <td>${it.code||'—'}</td>
          <td>${it.brand||'—'} ${it.model||''}</td>
          <td>${it.ref||'—'}</td>
          <td>${it.serial||'—'}</td>
          <td style="text-align:right;">${formatSalePrice(it.salePrice||0)}</td>
          <td>${slip.note||'—'}</td>
        </tr>`).join('')
    ).join('');

    const total = g.rows.reduce((s, slip) =>
      s + (slip.items||[]).reduce((ss, it) => ss + (it.salePrice||0), 0), 0);

    // 販売先情報
    const buyers = APP_DATA.buyers || [];
    const buyerObj = buyers.find(b => b.id === g.rows[0]?.buyer || b.name === g.name) ||
                     buyers.find(b => b.name === g.name);

    return `
      <div class="invoice-page">
        <div class="inv-header-row">
          <div>
            <div class="inv-title">売上 請求書</div>
            <div style="font-size:11px;color:#555;">発行日：${today}</div>
          </div>
          <div class="inv-company">
            <div style="font-weight:700;">${company.name||'—'}</div>
            <div style="font-size:10px;">${company.zip||''} ${company.address||''}</div>
            <div style="font-size:10px;">TEL: ${company.tel||'—'}　適格請求書番号: ${company.invoice||'—'}</div>
          </div>
        </div>
        <div class="inv-to">
          <b>請求先：${g.name} 御中</b>
          ${buyerObj ? `<span style="font-size:10px;color:#555;margin-left:8px;">${buyerObj.address||''}</span>` : ''}
        </div>
        <table class="inv-table">
          <thead><tr>
            <th>伝票番号</th><th>売上日</th><th>商品コード</th>
            <th>商品名</th><th>型番</th><th>シリアル</th>
            <th style="text-align:right;">販売金額（USD）</th><th>備考</th>
          </tr></thead>
          <tbody>${itemRows}</tbody>
          <tfoot><tr>
            <td colspan="6" style="text-align:right;font-weight:700;">合計金額</td>
            <td style="text-align:right;font-weight:700;">${formatSalePrice(total)}</td>
            <td></td>
          </tr></tfoot>
        </table>
      </div>`;
  }).join('<div class="page-break"></div>');

  const html = `<!DOCTYPE html><html lang="ja"><head><meta charset="UTF-8">
    <title>売上 請求書</title>
    <style>
      body { font-family: 'Helvetica Neue',sans-serif; font-size:12px; margin:0; color:#111; }
      .invoice-page { padding:32px 36px; max-width:960px; margin:0 auto; }
      .inv-header-row { display:flex; justify-content:space-between; align-items:flex-start; margin-bottom:20px; }
      .inv-title { font-size:20px; font-weight:900; color:#1a3a5c; margin-bottom:4px; }
      .inv-company { text-align:right; }
      .inv-to { margin-bottom:16px; padding:8px 12px; background:#f0f4f8; border-left:4px solid #1a3a5c; border-radius:4px; }
      .inv-table { width:100%; border-collapse:collapse; font-size:11px; margin-top:12px; }
      .inv-table th { background:#1a3a5c; color:#fff; padding:6px 8px; text-align:left; }
      .inv-table td { padding:5px 8px; border-bottom:1px solid #ddd; }
      .inv-table tfoot td { background:#f0f4f8; font-size:13px; }
      .page-break { page-break-after:always; }
      @media print { button { display:none; } .page-break { page-break-after:always; } }
    </style></head><body>
    ${invoicePages}
    <script>window.onload=function(){window.print();}<\/script>
    </body></html>`;

  const w = window.open('', '_blank');
  if (w) { w.document.write(html); w.document.close(); }
  else    { showToast('warn', '請求書発行', 'ポップアップがブロックされました'); }
}

// ── CSV出力 ──
function exportSlipCSV() {
  let data = getFilteredSlipData();
  let rows = [];
  if (currentSlipTab === 'purchase') {
    rows = [['商品コード','仕入日','ブランド','モデル','仕入先','担当者','仕入金額','ステータス','修正回数']];
    data.forEach(item => rows.push([item.code, item.purchaseDate, item.brand, item.model,
      getSupplierName(item.supplier), item.staff||'', item.purchasePrice, item.status, (item.revisions||[]).length]));
  } else if (currentSlipTab === 'shipping') {
    // 選択中の伝票があればその伝票のみ、なければ全件
    const exportData = _shSelectedIds.size > 0
      ? data.filter(s => _shSelectedIds.has(s.id))
      : data;
    rows = [['伝票番号','出荷日','出荷先','商品コード','ブランド','モデル','卸価格','備考','修正回数']];
    exportData.forEach(s => (s.items||[]).forEach(it =>
      rows.push([s.id, s.date, getBuyerName(s.destination), it.code, it.brand, it.model, it.wholesale, s.note||'', (s.revisions||[]).length])));
    const csvSh = rows.map(r => r.map(v => `"${String(v).replace(/"/g,'""')}"`).join(',')).join('\n');
    const aSh = document.createElement('a');
    aSh.href = URL.createObjectURL(new Blob(['\uFEFF' + csvSh], {type:'text/csv;charset=utf-8;'}));
    aSh.download = `slip_shipping_${new Date().toISOString().slice(0,10)}.csv`;
    aSh.click();
    const shLabel = _shSelectedIds.size > 0 ? `${_shSelectedIds.size}件（選択分）` : `${exportData.length}件（全件）`;
    showToast('success', 'CSV出力', `${shLabel}を出力しました`);
    return;
  } else if (currentSlipTab === 'sales') {
    // 選択中の伝票があればその伝票のみ、なければ全件
    const exportData = _slSelectedIds.size > 0
      ? data.filter(s => _slSelectedIds.has(s.id))
      : data;
    rows = [['伝票番号','売上日','販売先','商品コード','ブランド','モデル','販売金額（USD）','備考','修正回数']];
    exportData.forEach(s => (s.items||[]).forEach(it =>
      rows.push([s.id, s.date, getBuyerName(s.buyer), it.code, it.brand, it.model, it.salePrice, s.note||'', (s.revisions||[]).length])));
    const csv = rows.map(r => r.map(v => `"${String(v).replace(/"/g,'""')}"`).join(',')).join('\n');
    const a = document.createElement('a');
    a.href = URL.createObjectURL(new Blob(['\uFEFF' + csv], {type:'text/csv;charset=utf-8;'}));
    a.download = `slip_sales_${new Date().toISOString().slice(0,10)}.csv`;
    a.click();
    const label = _slSelectedIds.size > 0 ? `${_slSelectedIds.size}件（選択分）` : `${exportData.length}件（全件）`;
    showToast('success', 'CSV出力', `${label}を出力しました`);
    return;
  } else {
    rows = [['伝票番号','売上日','販売先','商品コード','ブランド','モデル','販売金額（USD）','備考','修正回数']];
    data.forEach(s => (s.items||[]).forEach(it =>
      rows.push([s.id, s.date, getBuyerName(s.buyer), it.code, it.brand, it.model, it.salePrice, s.note||'', (s.revisions||[]).length])));
  }
  const csv = rows.map(r => r.map(v => `"${String(v).replace(/"/g,'""')}"`).join(',')).join('\n');
  const a = document.createElement('a');
  a.href = URL.createObjectURL(new Blob(['\uFEFF' + csv], {type:'text/csv;charset=utf-8;'}));
  a.download = `slip_${currentSlipTab}_${new Date().toISOString().slice(0,10)}.csv`;
  a.click();
  showToast('success', 'CSV出力', `${data.length}件を出力しました`);
}

function getFilteredSlipData() {
  // フィルタ済みデータを返す（renderSlipListのフィルタロジックを再利用）
  const from    = document.getElementById('slip-filter-from')?.value    || '';
  const to      = document.getElementById('slip-filter-to')?.value      || '';
  const party   = document.getElementById('slip-filter-party')?.value   || '';
  const keyword = (document.getElementById('slip-filter-keyword')?.value || '').toLowerCase();
  if (currentSlipTab === 'purchase') {
    return APP_DATA.inventory.filter(item => {
      if (from && item.purchaseDate < from) return false;
      if (to   && item.purchaseDate > to)   return false;
      if (party && item.supplier !== party)  return false;
      if (keyword) {
        const h = [item.code, item.brand, item.model, getSupplierName(item.supplier), item.note||''].join(' ').toLowerCase();
        if (!h.includes(keyword)) return false;
      }
      return true;
    });
  } else if (currentSlipTab === 'shipping') {
    return APP_DATA.shipments.filter(s => {
      if (from && s.date < from) return false;
      if (to   && s.date > to)   return false;
      if (party && s.destination !== party) return false;
      return true;
    });
  } else {
    return APP_DATA.sales.filter(s => {
      if (from && s.date < from) return false;
      if (to   && s.date > to)   return false;
      if (party && s.buyer !== party) return false;
      return true;
    });
  }
}

// =====================================================
// 伝票詳細モーダル
// =====================================================
function openSlipDetail(type, id) {
  let record = null;
  if (type === 'purchase') record = (APP_DATA.purchaseSlips || []).find(s => s.id === id);
  else if (type === 'shipping') record = APP_DATA.shipments.find(s => s.id === id);
  else record = APP_DATA.sales.find(s => s.id === id);
  if (!record) return;

  const modal = document.getElementById('slipDetailOverlay');
  if (!modal) return;

  const typeLabel = { purchase:'仕入伝票', shipping:'出荷伝票', sales:'売上伝票' }[type];
  const typeIcon  = { purchase:'fa-file-import', shipping:'fa-truck', sales:'fa-yen-sign' }[type];
  document.getElementById('slipDetailTitle').textContent = typeLabel;
  document.getElementById('slipDetailIcon').innerHTML = `<i class="fa-solid ${typeIcon}"></i>`;

  // ── サブヘッダー（伝票ID・日付・取引先）を構築 ──
  let slipId   = record.id || record.code;
  let slipDate = type === 'purchase' ? (record.date || '—') : (record.date || '—');
  let party    = '';
  if (type === 'purchase') {
    party = getSupplierName(record.supplier) || '—';
  } else if (type === 'shipping') {
    party = getBuyerName(record.destination) || '—';
  } else {
    party = getBuyerName(record.buyer) || '—';
  }
  const revCount = (record.revisions || []).length;
  const revBadge = revCount > 0
    ? `<span class="slip-revised-badge"><i class="fa-solid fa-pen-to-square"></i> 修正済 ${revCount}件</span>`
    : '';
  document.getElementById('slipDetailHeaderSub').innerHTML = `
    <span class="sdhs-id"><i class="fa-solid fa-barcode"></i> ${slipId}</span>
    <span class="sdhs-divider"></span>
    <span class="sdhs-date"><i class="fa-regular fa-calendar"></i> ${slipDate}</span>
    <span class="sdhs-divider"></span>
    <span class="sdhs-party"><i class="fa-solid fa-building"></i> ${party}</span>
    ${revBadge}
  `;

  document.getElementById('slipDetailBody').innerHTML  = buildSlipDetailBody(type, record);
  document.getElementById('slipDetailFooter').innerHTML = buildSlipDetailFooter(type, record);
  modal.classList.remove('hidden');
}

function closeSlipDetail() {
  document.getElementById('slipDetailOverlay')?.classList.add('hidden');
}

function buildSlipDetailBody(type, rec) {
  let metaHtml = '';
  let itemsHtml = '';

  if (type === 'purchase') {
    const slipTotal = (rec.lines || []).reduce((s, l) => s + (l.purchasePrice || 0), 0);
    const saleTotal = (rec.lines || []).reduce((s, l) => s + (l.salePrice     || 0), 0);
    metaHtml = `
      <div class="slip-detail-meta">
        ${metaRow('<i class="fa-solid fa-file-invoice"></i> 伝票番号',  `<code style="font-size:13px;font-weight:bold;">${rec.id}</code>`)}
        ${metaRow('<i class="fa-regular fa-calendar"></i> 仕入日',       rec.date||'—')}
        ${metaRow('<i class="fa-solid fa-industry"></i> 仕入先',         getSupplierName(rec.supplier))}
        ${metaRow('<i class="fa-solid fa-user"></i> 仕入担当者',          rec.staff||'—')}
        ${metaRow('<i class="fa-solid fa-yen-sign"></i> 合計仕入金額',   `<span class="slip-detail-price">${formatPrice(slipTotal)}</span>`)}
        ${metaRow('<i class="fa-solid fa-tag"></i> 合計売価（USD）',       `<span class="slip-detail-price" style="color:var(--success);">${formatSalePrice(saleTotal)}</span>`)}
        ${rec.registeredAt ? metaRow('<i class="fa-solid fa-clock"></i> 登録日時', rec.registeredAt) : ''}
      </div>`;
    // 明細テーブル
    itemsHtml = `
      <div class="slip-detail-items">
        <div class="sdi-heading"><i class="fa-solid fa-list"></i> 明細一覧 <span style="font-size:11px;color:var(--text-muted);font-weight:normal;">${(rec.lines||[]).length}件</span></div>
        <table class="data-table" style="font-size:12px;margin-top:6px;">
          <thead>
            <tr>
              <th style="width:36px;text-align:center;">No.</th>
              <th>商品コード</th>
              <th>SKU</th>
              <th>ブランド</th>
              <th>モデル</th>
              <th style="text-align:right;">仕入金額</th>
              <th style="text-align:right;">売価（USD）</th>
            </tr>
          </thead>
          <tbody>
            ${(rec.lines || []).map(l => {
              const d = l.productDetail || {};
              return `<tr>
                <td style="text-align:center;color:var(--text-muted);">${l.lineNo}</td>
                <td><code style="font-size:11px;">${l.code}</code></td>
                <td>${l.sku || '—'}</td>
                <td>${d.brand  || '—'}</td>
                <td style="font-size:11px;color:var(--text-muted);">${d.model || '—'}</td>
                <td style="text-align:right;font-weight:bold;color:var(--primary);">${formatPrice(l.purchasePrice||0)}</td>
                <td style="text-align:right;font-weight:bold;color:var(--success);">${formatSalePrice(l.salePrice||0)}</td>
              </tr>`;
            }).join('')}
          </tbody>
          <tfoot>
            <tr style="background:var(--bg);font-weight:bold;">
              <td colspan="5" style="text-align:right;">合　計</td>
              <td style="text-align:right;color:var(--primary);">${formatPrice(slipTotal)}</td>
              <td style="text-align:right;color:var(--success);">${formatSalePrice(saleTotal)}</td>
            </tr>
          </tfoot>
        </table>
      </div>`;
  } else if (type === 'shipping') {
    const buyer = APP_DATA.buyers.find(b => b.code === rec.destination);
    metaHtml = `
      <div class="slip-detail-meta">
        ${metaRow('<i class="fa-solid fa-file-lines"></i> 伝票番号', `<code style="font-size:13px;">${rec.id}</code>`)}
        ${metaRow('<i class="fa-regular fa-calendar"></i> 出荷日', rec.date||'—')}
        ${metaRow('<i class="fa-solid fa-building"></i> 出荷先', `<b>${getBuyerName(rec.destination)}</b>`)}
        ${buyer?.address ? metaRow('<i class="fa-solid fa-location-dot"></i> 住所', buyer.address) : ''}
        ${buyer?.contact ? metaRow('<i class="fa-solid fa-phone"></i> 連絡先', buyer.contact) : ''}
        ${buyer?.invoice ? metaRow('<i class="fa-solid fa-receipt"></i> 適格番号', buyer.invoice) : ''}
        ${metaRow('<i class="fa-solid fa-dollar-sign"></i> 合計金額（USD）', `<span class="slip-detail-price">${formatSalePrice(rec.total)}</span>`)}
        ${rec.note ? metaRow('<i class="fa-solid fa-note-sticky"></i> 備考', rec.note) : ''}
      </div>`;
    itemsHtml = buildItemsTable(rec.items||[], 'shipping');
  } else {
    const buyer = APP_DATA.buyers.find(b => b.code === rec.buyer);
    metaHtml = `
      <div class="slip-detail-meta">
        ${metaRow('<i class="fa-solid fa-file-lines"></i> 伝票番号', `<code style="font-size:13px;">${rec.id}</code>`)}
        ${metaRow('<i class="fa-regular fa-calendar"></i> 売上日', rec.date||'—')}
        ${metaRow('<i class="fa-solid fa-building"></i> 販売先', `<b>${getBuyerName(rec.buyer)}</b>`)}
        ${buyer?.address ? metaRow('<i class="fa-solid fa-location-dot"></i> 住所', buyer.address) : ''}
        ${buyer?.contact ? metaRow('<i class="fa-solid fa-phone"></i> 連絡先', buyer.contact) : ''}
        ${buyer?.invoice ? metaRow('<i class="fa-solid fa-receipt"></i> 適格番号', buyer.invoice) : ''}
        ${metaRow('<i class="fa-solid fa-dollar-sign"></i> 合計金額（USD）', `<span class="slip-detail-price">${formatSalePrice(rec.total)}</span>`)}
        ${rec.note ? metaRow('<i class="fa-solid fa-note-sticky"></i> 備考', rec.note) : ''}
      </div>`;
    itemsHtml = buildItemsTable(rec.items||[], 'sales');
  }

  const revHtml = buildRevisionHistory(rec.revisions||[]);

  return `${metaHtml}${itemsHtml}${revHtml}`;
}

function metaRow(label, val) {
  return `<div class="slip-meta-row">
    <span class="slip-meta-label">${label}</span>
    <span class="slip-meta-val">${val}</span>
  </div>`;
}

function buildItemsTable(items, type) {
  if (!items.length) return '';
  const priceKey = type === 'shipping' ? 'wholesale' : 'salePrice';
  return `
    <hr class="appr-detail-divider">
    <div class="appr-detail-content-title"><i class="fa-solid fa-list-check"></i> 商品明細（${items.length}点）</div>
    <table class="appr-items-table">
      <thead><tr><th>商品コード</th><th>ブランド</th><th>モデル</th><th style="text-align:right;">金額（USD）</th></tr></thead>
      <tbody>
        ${items.map(it => `<tr>
          <td><code style="font-size:11px;">${it.code||'—'}</code></td>
          <td>${it.brand||'—'}</td>
          <td>${it.model||'—'}</td>
          <td style="text-align:right;font-weight:bold;">${formatSalePrice(it[priceKey]||0)}</td>
        </tr>`).join('')}
        <tr class="appr-items-total">
          <td colspan="3" style="text-align:right;">合計</td>
          <td style="text-align:right;">${formatSalePrice(items.reduce((s,i)=>s+(i[priceKey]||0),0))}</td>
        </tr>
      </tbody>
    </table>`;
}

function buildRevisionHistory(revisions) {
  if (!revisions.length) return '';
  return `
    <hr class="appr-detail-divider">
    <div class="appr-detail-content-title" style="color:#e07b39;margin-top:4px;">
      <i class="fa-solid fa-clock-rotate-left"></i> 修正履歴（${revisions.length}件）
    </div>
    <div class="slip-revision-list">
      ${revisions.map((r, i) => `
        <div class="slip-revision-item">
          <div class="slip-revision-num">${i + 1}</div>
          <div class="slip-revision-body">
            <div class="slip-revision-row">
              <span class="slip-revision-label"><i class="fa-regular fa-calendar-days"></i> 修正日時</span>
              <span style="font-weight:600;">${r.revisedAt || '—'}</span>
            </div>
            <div class="slip-revision-row">
              <span class="slip-revision-label"><i class="fa-solid fa-id-badge"></i> 担当作業者</span>
              <span><b>${r.buyerName || '—'}</b></span>
            </div>
            <div class="slip-revision-row">
              <span class="slip-revision-label"><i class="fa-solid fa-user-tie"></i> 承認管理者</span>
              <span style="color:${r.approverName && r.approverName.includes('承認待') ? '#f59e0b' : 'var(--text-dark)'};">
                ${r.approverName || '—'}
                ${r.approverName && r.approverName.includes('承認待') ? '<i class="fa-solid fa-hourglass-half" style="margin-left:4px;color:#f59e0b;"></i>' : ''}
              </span>
            </div>
            <div class="slip-revision-row" style="align-items:flex-start;">
              <span class="slip-revision-label" style="padding-top:1px;"><i class="fa-solid fa-comment-dots"></i> 修正内容</span>
              <span style="flex:1;line-height:1.6;">${r.note || '—'}</span>
            </div>
          </div>
        </div>
      `).join('')}
    </div>`;
}

function buildSlipDetailFooter(type, rec) {
  const recId    = rec.id || rec.code;
  const revCount = (rec.revisions || []).length;
  const revInfo  = revCount > 0
    ? `<span style="font-size:12px;color:#c2410c;display:flex;align-items:center;gap:5px;">
         <i class="fa-solid fa-clock-rotate-left"></i> 修正履歴: ${revCount}件
       </span>`
    : `<span style="font-size:12px;color:var(--text-muted);">修正履歴なし</span>`;

  // ゲスト（guest ロール）以外は常に修正ボタンを表示
  const isGuest_ = typeof isGuest === 'function' && isGuest();
  const reviseBtn = !isGuest_
    ? `<button class="btn btn-warning" style="display:flex;align-items:center;gap:6px;"
         onclick="openSlipRevise('${type}','${recId}')">
         <i class="fa-solid fa-pen-to-square"></i> 伝票修正
       </button>`
    : '';

  // 仕入伝票のみ「仕入返品を起票」ボタンを表示
  const purchaseReturnBtn = (type === 'purchase' && !isGuest_)
    ? `<button class="btn btn-danger" style="display:flex;align-items:center;gap:6px;"
         onclick="openPurchaseReturnModal('${recId}')">
         <i class="fa-solid fa-boxes-packing"></i> 仕入返品を起票
       </button>`
    : '';

  // 売上伝票のみ「売上返品を起票」ボタンを表示
  const salesReturnBtn = (type === 'sales' && !isGuest_)
    ? `<button class="btn btn-danger" style="display:flex;align-items:center;gap:6px;"
         onclick="openSalesReturnModal('${recId}')">
         <i class="fa-solid fa-rotate-left"></i> 売上返品を起票
       </button>`
    : '';

  const documentLabel = { purchase: '仕入伝票', shipping: '出荷伝票', sales: '請求書' }[type] || '伝票';
  const documentBtn = `
    <button class="btn btn-primary" style="display:flex;align-items:center;gap:6px;"
      onclick="openSavedSlipDocument('${type}','${recId}')">
      <i class="fa-solid fa-file-invoice"></i> ${documentLabel}を表示
    </button>`;

  return `
    <div style="display:flex;align-items:center;">${revInfo}</div>
    <div style="display:flex;gap:8px;flex-wrap:wrap;">
      <button class="btn btn-outline" onclick="closeSlipDetail()">
        <i class="fa-solid fa-xmark"></i> 閉じる
      </button>
      ${documentBtn}
      ${reviseBtn}
      ${purchaseReturnBtn}
      ${salesReturnBtn}
    </div>
  `;
}

// =====================================================
// 伝票一覧 > 保存済み帳票プレビュー・ダウンロード
// =====================================================
let _savedSlipDocumentContext = null;

function _getSavedSlipDocumentRecord(type, id) {
  if (type === 'purchase') return (APP_DATA.purchaseSlips || []).find(record => record.id === id) || null;
  if (type === 'shipping') return (APP_DATA.shipments || []).find(record => record.id === id) || null;
  if (type === 'sales') return (APP_DATA.sales || []).find(record => record.id === id) || null;
  return null;
}

function _getSavedSlipDocumentMeta(type, record) {
  if (type === 'purchase') {
    return { title: '仕入伝票', filenameLabel: '仕入伝票', html: buildPurchaseRecordTemplateHTML(record) };
  }
  if (type === 'shipping') {
    return { title: '出荷伝票', filenameLabel: '出荷伝票', html: buildShipmentRecordTemplateHTML(record) };
  }
  return { title: '請求書（売上伝票）', filenameLabel: '請求書', html: buildSalesRecordTemplateHTML(record) };
}

function openSavedSlipDocument(type, id) {
  const record = _getSavedSlipDocumentRecord(type, id);
  if (!record) {
    showToast('warning', '帳票プレビュー', '対象の伝票が見つかりません。');
    return;
  }
  const meta = _getSavedSlipDocumentMeta(type, record);
  _savedSlipDocumentContext = { type, id, record, ...meta };
  const title = document.getElementById('savedSlipDocumentTitle');
  const printArea = document.getElementById('savedSlipDocumentPrintArea');
  if (title) title.textContent = `${meta.title}プレビュー（表紙＋明細表 A4縦）`;
  if (printArea) printArea.innerHTML = meta.html;
  document.getElementById('savedSlipDocumentModal')?.classList.remove('hidden');
}

function closeSavedSlipDocument() {
  document.getElementById('savedSlipDocumentModal')?.classList.add('hidden');
}

function downloadSavedSlipDocument() {
  const context = _savedSlipDocumentContext;
  if (!context) {
    showToast('warning', '帳票ダウンロード', '対象の伝票が選択されていません。');
    return;
  }
  _downloadTemplateDocument(
    context.title,
    `${context.id}_${context.filenameLabel}.html`,
    context.html,
  );
}

function printSavedSlipDocument() {
  const context = _savedSlipDocumentContext;
  if (!context) {
    showToast('warning', '帳票印刷', '対象の伝票が選択されていません。');
    return;
  }
  _openTemplatePrintWindow(`${context.title} — ${context.id}`, context.html);
}

// =====================================================
// 仕入返品起票 ── コード入力統一方式
// =====================================================
let _purchaseReturnTargetCode = null;
// 追加済みアイテムを管理する配列（{code, brand, model, ref, serial, sku, purchasePrice}）
let _prAddedItems = [];

function openPurchaseReturnModal(slipId) {
  const slip = (APP_DATA.purchaseSlips || []).find(s => s.id === slipId);
  if (!slip) { showToast('error', 'エラー', '仕入伝票が見つかりません'); return; }
  _purchaseReturnTargetCode = slipId;
  _prAddedItems = [];

  // ヘッダー情報のみ表示（チェックボックステーブルは出さない）
  document.getElementById('purchaseReturnItemInfo').innerHTML = `
    <div style="background:#f8fafc;border:1px solid var(--border);border-radius:8px;padding:10px 16px;font-size:12px;">
      <span style="color:var(--text-muted);">返品元伝票：</span>
      <code style="color:var(--primary-light);font-weight:bold;">${slip.id}</code>
      <span style="margin-left:12px;">仕入先: <b>${getSupplierName(slip.supplier)}</b></span>
      <span style="margin-left:12px;color:var(--text-muted);">仕入日: ${slip.date}</span>
    </div>`;

  _prRenderAddedItems();

  document.getElementById('pr-return-date').value = getLocalDateISO();
  document.getElementById('pr-return-reason').value = '';
  document.getElementById('pr-return-note').value = '';
  const inp = document.getElementById('pr-add-code-input');
  if (inp) { inp.value = ''; setTimeout(() => inp.focus(), 300); }

  closeSlipDetail();
  document.getElementById('purchaseReturnModal').classList.remove('hidden');
}

/** 追加済みアイテムリストを再描画 */
function _prRenderAddedItems() {
  const listEl  = document.getElementById('pr-selected-items-list');
  const countEl = document.getElementById('pr-item-count-badge');
  const totalEl = document.getElementById('pr-total-badge');
  if (!listEl) return;

  const total = _prAddedItems.reduce((s, it) => s + (it.purchasePrice || 0), 0);
  if (countEl) countEl.textContent = `${_prAddedItems.length}点`;
  if (totalEl) totalEl.textContent = _prAddedItems.length > 0 ? `合計: ${formatPrice(total)}` : '';

  if (_prAddedItems.length === 0) {
    listEl.innerHTML = `<div style="padding:14px;text-align:center;color:var(--text-muted);font-size:12px;">
      <i class="fa-solid fa-barcode" style="font-size:20px;display:block;margin-bottom:6px;opacity:0.4;"></i>
      商品コードを入力またはスキャンして追加してください
    </div>`;
    return;
  }

  listEl.innerHTML = _prAddedItems.map((it, i) => `
    <div style="display:flex;align-items:center;gap:10px;padding:9px 14px;border-bottom:1px solid var(--border);font-size:12px;background:#fff;">
      <div style="flex:1;min-width:0;">
        <div style="font-weight:700;color:var(--text);">${it.brand} ${it.model}</div>
        <div style="color:var(--text-muted);margin-top:2px;">
          <code style="font-size:10px;">${it.code}</code>
          ${it.sku    ? `　SKU: ${it.sku}`       : ''}
          ${it.ref    ? `　型番: ${it.ref}`       : ''}
          ${it.serial ? `　S/N: ${it.serial}`     : ''}
        </div>
      </div>
      <span style="font-weight:700;color:var(--primary);white-space:nowrap;">${formatPrice(it.purchasePrice || 0)}</span>
      <button onclick="prRemoveItem(${i})" title="削除"
        style="background:none;border:none;color:#dc2626;cursor:pointer;font-size:14px;padding:2px 4px;flex-shrink:0;">
        <i class="fa-solid fa-trash-can"></i>
      </button>
    </div>`).join('');
}

/** リストから削除 */
function prRemoveItem(idx) {
  _prAddedItems.splice(idx, 1);
  _prRenderAddedItems();
}

/**
 * 商品コード入力で追加（仕入返品）
 * 検索順: ① purchaseSlips の lines ② inventory
 * 重複禁止・該当なしエラー
 */
function prAddItemByCode(codeArg) {
  const inp  = document.getElementById('pr-add-code-input');
  const code = (codeArg || (inp ? inp.value.trim() : '')).trim();
  if (!code) { showToast('error', '入力エラー', '商品コードを入力してください'); return; }

  const slipId = _purchaseReturnTargetCode;
  const slip   = slipId ? (APP_DATA.purchaseSlips || []).find(s => s.id === slipId) : null;

  // 重複チェック
  if (_prAddedItems.some(it => it.code === code)) {
    showToast('warning', '重複', `商品コード "${code}" はすでに追加されています`);
    if (inp) inp.value = '';
    return;
  }

  // ① 対象伝票の明細から検索
  let found = null;
  if (slip) {
    const line = (slip.lines || []).find(l => l.code === code || (l.sku || '').toLowerCase() === code.toLowerCase());
    if (line) {
      const d = line.productDetail || {};
      found = {
        code:          line.code,
        brand:         d.brand  || '',
        model:         d.model  || '',
        ref:           d.ref    || '',
        serial:        d.serial || '',
        sku:           line.sku || '',
        purchasePrice: line.purchasePrice || 0,
      };
    }
  }

  // ② 在庫から検索
  if (!found) {
    const inv = (APP_DATA.inventory || []).find(item => item.code === code);
    if (inv) {
      found = {
        code:          inv.code,
        brand:         inv.brand  || '',
        model:         inv.model  || '',
        ref:           inv.ref    || '',
        serial:        inv.serial || '',
        sku:           inv.sku    || '',
        purchasePrice: inv.purchasePrice || 0,
      };
    }
  }

  if (!found) {
    showToast('error', '該当商品がありません', `商品コード "${code}" は登録されていません`);
    if (inp) { inp.value = ''; inp.focus(); }
    return;
  }

  _prAddedItems.push(found);
  _prRenderAddedItems();
  showToast('success', '追加しました', `${found.brand} ${found.model}`);
  if (inp) { inp.value = ''; inp.focus(); }
}

function closePurchaseReturnModal() {
  document.getElementById('purchaseReturnModal').classList.add('hidden');
  _purchaseReturnTargetCode = null;
  _prAddedItems = [];
}

async function submitPurchaseReturn() {
  const slipId = _purchaseReturnTargetCode;
  const slip   = slipId ? (APP_DATA.purchaseSlips || []).find(s => s.id === slipId) : null;
  if (!slip) { showToast('error', 'エラー', '対象伝票が見つかりません'); return; }

  const date   = document.getElementById('pr-return-date').value;
  const reason = document.getElementById('pr-return-reason').value;
  const note   = document.getElementById('pr-return-note').value;
  if (!date)                    { showToast('error', '入力エラー', '返品日を入力してください'); return; }
  if (_prAddedItems.length === 0) { showToast('error', '選択エラー', '返品する商品を1点以上追加してください'); return; }

  if (window.ZaikoAPI) {
    try {
      const result = await window.ZaikoAPI.saveReturn({
        operationType: 'purchase_return', transactionDate: date, supplierCode: slip.supplier,
        sourcePurchaseSlipNumber: slip.id, reason, notes: note,
        productCodes: _prAddedItems.map(item => item.code),
      }, isBuyer());
      closePurchaseReturnModal();
      if (result.approval) {
        showToast('info', '承認申請を送信しました', `${result.record.slipNumber} は管理者の承認待ちです`);
      } else {
        showToast('success', '仕入返品伝票を確定しました', `${result.record.slipNumber} をDBへ保存しました`);
      }
      refreshLinkedBusinessViews({ source: 'purchase-return' });
    } catch (error) {
      showToast('error', '仕入返品登録エラー', error.message);
    }
    return;
  }

  if (!APP_DATA.purchaseReturns) APP_DATA.purchaseReturns = [];
  const id = `PR-RET-${String((APP_DATA.purchaseReturns.length + 1)).padStart(4, '0')}`;

  const retItems = _prAddedItems.map(it => ({
    code:          it.code,
    brand:         it.brand,
    model:         it.model,
    ref:           it.ref,
    serial:        it.serial,
    sku:           it.sku,
    purchasePrice: it.purchasePrice,
    status:        '未処理',
    trackingNo:    '',
  }));

  const ret = {
    id, date,
    supplier:   slip.supplier,
    slipId:     slip.id,
    items:      retItems,
    reason, note,
    status:     '未処理',
    createdBy:  currentUser()?.name || '—',
    createdAt:  new Date().toLocaleString('ja-JP'),
    invoicePrinted: false,
  };

  if (isBuyer()) {
    ret.status = '承認待ち';
    APP_DATA.purchaseReturns.push(ret);
    requestApproval(
      'purchase_return', '仕入返品起票',
      { retId: id, slipId: slip.id, supplier: slip.supplier, date, reason, note, record: _approvalClone(ret) },
      note, null
    );
    closePurchaseReturnModal();
    showToast('info', '承認申請を送信しました', `管理者の承認待ちです（${id}）`);
  } else {
    APP_DATA.purchaseReturns.push(ret);
    closePurchaseReturnModal();
    showToast('success', '仕入返品伝票を起票しました', `${id} を作成しました`);
  }
  applyBusinessRecordState('purchasereturn', ret);
  refreshLinkedBusinessViews({ source: 'purchase-return' });
}

// 現在開いている仕入返品伝票IDを保持
let _currentPrRetId = null;

/** 仕入返品に対応する元仕入伝票・在庫から、仕入時点の仕入価格を復元する */
function getPurchaseReturnOriginalAmountInfo(ret) {
  const returnCodes = new Set((ret?.items || []).map(item => item.code).filter(Boolean));
  const sourcePurchase = (APP_DATA.purchaseSlips || []).find(slip => slip.id === ret?.slipId)
    || (APP_DATA.purchaseSlips || []).find(slip => returnCodes.size > 0
      && [...returnCodes].every(code => (slip.lines || []).some(line => line.code === code)))
    || null;
  const items = (ret?.items || []).map((returnItem, index) => {
    const sourceLine = (sourcePurchase?.lines || []).find(line => line.code === returnItem.code) || null;
    const inventoryItem = (APP_DATA.inventory || []).find(item => item.code === returnItem.code) || {};
    const sourceDetail = sourceLine?.productDetail || {};
    let purchasePrice = Number(returnItem.purchasePrice) || 0;
    if (Number.isFinite(Number(inventoryItem.purchasePrice))) purchasePrice = Number(inventoryItem.purchasePrice);
    if (sourceLine && Number.isFinite(Number(sourceLine.purchasePrice))) purchasePrice = Number(sourceLine.purchasePrice);
    return {
      ...returnItem,
      no: index + 1,
      code: returnItem.code || sourceLine?.code || '',
      brand: sourceDetail.brand || returnItem.brand || inventoryItem.brand || '',
      model: sourceDetail.model || returnItem.model || inventoryItem.model || '',
      ref: sourceDetail.ref || returnItem.ref || inventoryItem.ref || '',
      serial: sourceDetail.serial || returnItem.serial || inventoryItem.serial || '',
      sku: sourceLine?.sku || returnItem.sku || inventoryItem.sku || '',
      accessories: sourceDetail.accessories || inventoryItem.accessories || [],
      note: sourceDetail.note || inventoryItem.note || '',
      purchasePrice,
    };
  });
  return {
    sourcePurchase,
    items,
    subtotal: items.reduce((sum, item) => sum + (Number(item.purchasePrice) || 0), 0),
    formatAmount: amount => formatPrice(amount),
  };
}

function openPurchaseReturnDetail(retId) {
  _currentPrRetId = retId;
  _renderPurchaseReturnDetail();
}

function _renderPurchaseReturnDetail() {
  const ret = (APP_DATA.purchaseReturns||[]).find(r => r.id === _currentPrRetId);
  if (!ret) return;

  const stBadge = _slipStatusBadge(ret.status || '未処理');
  const purchaseAmount = getPurchaseReturnOriginalAmountInfo(ret);
  const totalAmt = purchaseAmount.subtotal;

  // 明細テーブル行
  // 配送番号入力可能条件: 明細ステータスが「処理済」の場合のみ
  // それ以外（未処理・承認待ち・差戻し・承認済）は disabled グレーアウト
  const itemRows = purchaseAmount.items.map((it, idx) => {
    const isDone = it.status === '処理済';

    // 行背景: 処理済はわずかにグレー
    const rowBg = isDone ? 'background:#f8fafc;' : '';

    // 明細ステータスバッジ
    const stBadgeMap = {
      '処理済':  `<span style="display:inline-flex;align-items:center;gap:3px;padding:2px 8px;border-radius:10px;font-size:11px;font-weight:700;background:#f0fdf4;color:#16a34a;border:1px solid #86efac;"><i class="fa-solid fa-circle-check"></i> 処理済</span>`,
      '承認済':  `<span style="display:inline-flex;align-items:center;gap:3px;padding:2px 8px;border-radius:10px;font-size:11px;font-weight:700;background:#eff6ff;color:#2563eb;border:1px solid #93c5fd;"><i class="fa-solid fa-circle-check"></i> 承認済</span>`,
      '承認待ち':`<span style="display:inline-flex;align-items:center;gap:3px;padding:2px 8px;border-radius:10px;font-size:11px;font-weight:700;background:#fffbeb;color:#d97706;border:1px solid #fcd34d;"><i class="fa-solid fa-hourglass-half"></i> 承認待ち</span>`,
      '差戻し':  `<span style="display:inline-flex;align-items:center;gap:3px;padding:2px 8px;border-radius:10px;font-size:11px;font-weight:700;background:#fef2f2;color:#dc2626;border:1px solid #fca5a5;"><i class="fa-solid fa-rotate-left"></i> 差戻し</span>`,
    };
    const stIcon = stBadgeMap[it.status] ||
      `<span style="display:inline-flex;align-items:center;gap:3px;padding:2px 8px;border-radius:10px;font-size:11px;font-weight:600;background:#f1f5f9;color:#64748b;border:1px solid #cbd5e1;">未処理</span>`;

    // 配送番号セル
    // 処理済 → 入力可能（白背景・通常カーソル）
    // それ以外 → disabled（グレー背景・not-allowed カーソル）
    let trackingCell;
    if (isDone) {
      trackingCell = `<input
        class="form-control"
        id="pr-tracking-${idx}"
        type="text"
        placeholder="配送番号を入力"
        value="${(it.trackingNo||'').replace(/"/g,'&quot;')}"
        oninput="prUpdateTracking(${idx},this.value)"
        style="font-size:11px;padding:3px 8px;width:150px;min-width:120px;">`;
    } else {
      trackingCell = `<input
        class="form-control"
        type="text"
        placeholder="配送番号を入力"
        value="${(it.trackingNo||'').replace(/"/g,'&quot;')}"
        disabled
        style="font-size:11px;padding:3px 8px;width:150px;min-width:120px;
               background:#f1f5f9;color:#9ca3af;cursor:not-allowed;
               border-color:#e2e8f0;">`;
    }

    return `<tr id="pr-item-row-${idx}" style="${rowBg}">
      <td style="font-size:11px;"><code style="font-size:11px;">${it.code}</code></td>
      <td style="font-size:12px;">${it.brand||'—'}</td>
      <td style="font-size:12px;">${it.model||'—'}</td>
      <td style="font-size:11px;color:var(--text-muted);">${it.sku||'—'}</td>
      <td style="text-align:right;font-weight:bold;">${formatPrice(it.purchasePrice||0)}</td>
      <td style="text-align:center;">${stIcon}</td>
      <td style="white-space:nowrap;">${trackingCell}</td>
    </tr>`;
  }).join('');

  document.getElementById('prDetailModalTitle').textContent = `仕入返品伝票 ${ret.id}`;
  document.getElementById('purchaseReturnDetailBody').innerHTML = `
    <div class="detail-grid mb-20">
      <div class="detail-row"><div class="detail-label">伝票番号</div><div class="detail-value"><code>${ret.id}</code></div></div>
      <div class="detail-row"><div class="detail-label">返品日</div><div class="detail-value">${ret.date}</div></div>
      <div class="detail-row"><div class="detail-label">仕入先</div><div class="detail-value">${getSupplierName(ret.supplier)}</div></div>
      <div class="detail-row"><div class="detail-label">ステータス</div><div class="detail-value">${stBadge}</div></div>
      <div class="detail-row"><div class="detail-label">返品理由</div><div class="detail-value">${ret.reason||'—'}</div></div>
      <div class="detail-row"><div class="detail-label">起票者</div><div class="detail-value">${ret.createdBy}</div></div>
    </div>
    <div class="form-group mb-20">
      <div class="detail-label">備考</div>
      <div style="background:var(--bg);padding:10px;border-radius:6px;font-size:12px;">${ret.note||'（なし）'}</div>
    </div>
    <table class="data-table" style="width:100%;">
      <thead><tr>
        <th>商品コード</th><th>ブランド</th><th>モデル</th><th>SKU</th>
        <th style="text-align:right;">仕入金額</th>
        <th style="text-align:center;">明細ステータス</th>
        <th style="min-width:165px;">配送番号<span style="font-size:10px;color:#94a3b8;font-weight:normal;margin-left:4px;">（処理済のみ入力可）</span></th>
      </tr></thead>
      <tbody>${itemRows}</tbody>
      <tfoot><tr style="background:var(--bg);font-weight:bold;">
        <td colspan="4" style="text-align:right;">合計</td>
        <td style="text-align:right;color:var(--primary);">${formatPrice(totalAmt)}</td>
        <td colspan="2"></td>
      </tr></tfoot>
    </table>`;

  // フッターボタン更新
  // 完了ボタン: 請求書発行済み の場合のみ有効（処理済ステータスになった後）
  const canComplete = ret.invoicePrinted;
  const footer = document.getElementById('purchaseReturnDetailFooter');
  footer.innerHTML = `
    <button class="btn btn-outline" onclick="document.getElementById('purchaseReturnDetailModal').classList.add('hidden')">
      <i class="fa-solid fa-xmark"></i> 閉じる
    </button>
    <button class="btn btn-primary" id="prInvoiceBtn" onclick="_prInvoiceAndSetFlag()" style="gap:6px;">
      <i class="fa-solid fa-file-invoice"></i> 仕入返品伝票を表示
    </button>
    <button class="btn btn-success" id="prCompleteBtn"
      ${canComplete ? '' : 'disabled title="仕入返品伝票の表示・ダウンロード・印刷後に押せます"'}
      onclick="prCompleteItems()"
      style="gap:6px;${canComplete ? '' : 'opacity:0.4;cursor:not-allowed;'}">
      <i class="fa-solid fa-circle-check"></i> 完了
    </button>`;

  document.getElementById('purchaseReturnDetailModal').classList.remove('hidden');
}

/** 仕入返品伝票の表示フラグをセット */
function _prInvoiceAndSetFlag() {
  const ret = (APP_DATA.purchaseReturns||[]).find(r => r.id === _currentPrRetId);
  if (ret) markPurchaseReturnDocumentIssued(ret);
  openPurchaseReturnInvoice();
}

/** 配送番号の更新（処理済明細のみ呼ばれる） */
async function prUpdateTracking(idx, val) {
  const ret = (APP_DATA.purchaseReturns||[]).find(r => r.id === _currentPrRetId);
  if (ret && ret.items[idx]) {
    ret.items[idx].trackingNo = val;
    if (ret.apiManaged && ret._id && window.ZaikoAPI?.updateReturnTracking) {
      try {
        await window.ZaikoAPI.updateReturnTracking(ret._id, ret.carrier || '', val);
        showToast('success', '追跡番号を保存しました', val || '追跡番号を未設定にしました');
      } catch (error) {
        showToast('error', '追跡番号を保存できませんでした', error.message);
      }
    } else {
      persistBusinessWorkflowState();
    }
  }
}

/** 伝票一覧から配送番号を更新（処理済行のみ呼ばれる） */
async function prUpdateTrackingFromList(retId, val) {
  const ret = (APP_DATA.purchaseReturns||[]).find(r => r.id === retId);
  if (!ret) return;
  // 全明細の trackingNo を同一値で更新（1伝票1配送番号の運用想定）
  // 明細が複数ある場合は先頭のみ更新し、詳細モーダルで個別管理
  if (ret.items && ret.items.length > 0) {
    ret.items[0].trackingNo = val;
    if (ret.apiManaged && ret._id && window.ZaikoAPI?.updateReturnTracking) {
      try {
        await window.ZaikoAPI.updateReturnTracking(ret._id, ret.carrier || '', val);
        showToast('success', '追跡番号を保存しました', val || '追跡番号を未設定にしました');
      } catch (error) {
        showToast('error', '追跡番号を保存できませんでした', error.message);
      }
    } else {
      persistBusinessWorkflowState();
    }
  }
}

/** 完了ボタン押下：全明細を処理済に変更し在庫から削除 */
function prCompleteItems() {
  const ret = (APP_DATA.purchaseReturns||[]).find(r => r.id === _currentPrRetId);
  if (!ret) return;
  if (!confirm('全明細を「処理済」にして在庫から削除します。よろしいですか？')) return;

  let removedCount = 0;
  (ret.items||[]).forEach(it => {
    if (it.status !== '処理済') {
      it.status = '処理済';
      // 在庫から削除
      const idx = (APP_DATA.inventory||[]).findIndex(inv => inv.code === it.code);
      if (idx !== -1) {
        APP_DATA.inventory.splice(idx, 1);
        removedCount++;
      }
    }
  });
  ret.status = '処理済';

  showToast('success', '処理済に更新', `${removedCount}点を在庫から削除しました`);
  refreshLinkedBusinessViews({ source: 'purchase-return-complete' });
  _renderPurchaseReturnDetail();
}

// =====================================================
// ③④ 仕入伝票「要承認」ポップアップ
// =====================================================

function openPurchaseApprovalModal(slipId) {
  const slip = (APP_DATA.purchaseSlips||[]).find(s => s.id === slipId);
  if (!slip) return;
  const isAdm = typeof isAdmin === 'function' && isAdmin();
  document.getElementById('purchaseApprovalModalTitle').textContent = `承認リクエスト: ${slipId}`;

  const changes = (slip.approvalChanges||[]).map(ch => `
    <tr>
      <td style="font-weight:600;font-size:12px;">${ch.field}</td>
      <td style="font-size:12px;color:#dc2626;">${ch.before}</td>
      <td style="font-size:12px;color:#16a34a;">${ch.after}</td>
      <td style="font-size:12px;color:var(--text-muted);">${ch.reason||'—'}</td>
    </tr>`).join('');

  document.getElementById('purchaseApprovalModalBody').innerHTML = `
    <div style="margin-bottom:14px;padding:10px 14px;background:#fffbeb;border:1px solid #fcd34d;border-radius:8px;font-size:12px;">
      <b style="color:#92400e;"><i class="fa-solid fa-triangle-exclamation"></i> 申請者:</b>
      <span style="margin-left:8px;">${slip.approvalBy||'—'}</span>
      <span style="margin-left:12px;color:var(--text-muted);">${slip.approvalAt||''}</span>
    </div>
    ${slip.approvalNote ? `<div style="margin-bottom:14px;font-size:12px;color:var(--text-muted);padding:8px 12px;background:var(--bg);border-radius:6px;">${slip.approvalNote}</div>` : ''}
    ${changes ? `
    <div style="font-size:12px;font-weight:700;color:var(--text-muted);margin-bottom:8px;">変更内容</div>
    <table class="data-table" style="font-size:12px;width:100%;margin-bottom:14px;">
      <thead><tr><th>項目</th><th>変更前</th><th>変更後</th><th>理由</th></tr></thead>
      <tbody>${changes}</tbody>
    </table>` : '<p style="font-size:12px;color:var(--text-muted);">変更内容の詳細はありません</p>'}
    ${isAdm ? `
    <div class="form-group" style="margin-top:10px;">
      <label class="form-label" style="font-size:12px;">差戻しコメント（差戻し時のみ入力）</label>
      <textarea class="form-control" id="paRevisionComment" rows="2" placeholder="差戻し理由を入力…" style="font-size:12px;"></textarea>
    </div>` : ''}`;

  document.getElementById('purchaseApprovalModalFooter').innerHTML = `
    <button class="btn btn-outline" onclick="closePurchaseApprovalModal()"><i class="fa-solid fa-xmark"></i> 閉じる</button>
    ${isAdm ? `
    <button class="btn btn-danger" onclick="purchaseApprovalAction('${slipId}','revision')">
      <i class="fa-solid fa-rotate-left"></i> 差戻し
    </button>
    <button class="btn btn-success" onclick="purchaseApprovalAction('${slipId}','approve')">
      <i class="fa-solid fa-circle-check"></i> 承認する
    </button>` : `
    <button class="btn btn-primary" onclick="purchaseReRequestApproval('${slipId}')">
      <i class="fa-solid fa-paper-plane"></i> 再申請する
    </button>`}`;

  document.getElementById('purchaseApprovalModal').classList.remove('hidden');
}

function closePurchaseApprovalModal() {
  document.getElementById('purchaseApprovalModal').classList.add('hidden');
}

function purchaseApprovalAction(slipId, action) {
  const slip = (APP_DATA.purchaseSlips||[]).find(s => s.id === slipId);
  if (!slip) return;
  if (action === 'approve') {
    slip.status = '処理済';
    applyBusinessRecordState('purchase', slip);
    syncApprovalRequestForBusinessRecord('purchase', slipId, 'approved');
    delete slip.approvalNote;
    delete slip.approvalChanges;
    delete slip.approvalBy;
    delete slip.approvalAt;
    showToast('success', '承認しました', `${slipId} を承認済にしました`);
  } else {
    const comment = document.getElementById('paRevisionComment')?.value || '';
    slip.status = '差戻し';
    slip.revisionComment = comment;
    syncApprovalRequestForBusinessRecord('purchase', slipId, 'revision', comment);
    showToast('info', '差戻しました', `${slipId} を差戻しました`);
  }
  closePurchaseApprovalModal();
  refreshLinkedBusinessViews({ source: 'purchase-approval' });
}

function purchaseReRequestApproval(slipId) {
  const slip = (APP_DATA.purchaseSlips||[]).find(s => s.id === slipId);
  if (!slip) return;
  slip.status = '承認待ち';
  slip.approvalAt = new Date().toLocaleString('ja-JP');
  slip.approvalBy = currentUser()?.name || '—';
  syncApprovalRequestForBusinessRecord('purchase', slipId, 'pending');
  showToast('info', '再申請しました', `${slipId} を承認待ちにしました`);
  closePurchaseApprovalModal();
  refreshLinkedBusinessViews({ source: 'purchase-approval-resubmit' });
}

// =====================================================
// ⑥ ステータス詳細ポップアップ（承認待ち/差戻し 共通）
// =====================================================

function openStatusDetailModal(recordId, tabType) {
  let rec = null;
  if (tabType === 'purchase') {
    rec = (APP_DATA.purchaseSlips||[]).find(r => r.id === recordId);
  } else if (tabType === 'shipping') {
    rec = (APP_DATA.shipments||[]).find(r => r.id === recordId);
  } else if (tabType === 'sales') {
    rec = (APP_DATA.sales||[]).find(r => r.id === recordId);
  } else if (tabType === 'salesreturn') {
    rec = (APP_DATA.salesReturns||[]).find(r => r.id === recordId);
  } else if (tabType === 'purchasereturn') {
    rec = (APP_DATA.purchaseReturns||[]).find(r => r.id === recordId);
  }
  if (!rec) { showToast('error', 'エラー', 'レコードが見つかりません'); return; }

  const status    = rec.status || '未処理';
  const isAdm     = typeof isAdmin === 'function' && isAdmin();
  const isApprove = status === '承認待ち';
  const isDiff    = status === '差戻し';

  // ヘッダー色
  const headerBg  = isApprove ? 'background:linear-gradient(135deg,#92400e,#d97706);' : 'background:linear-gradient(135deg,#991b1b,#dc2626);';
  const headerEl  = document.getElementById('statusDetailModalHeader');
  const iconEl    = document.getElementById('statusDetailModalIcon');
  const titleEl   = document.getElementById('statusDetailModalTitle');
  if (headerEl) headerEl.style.cssText += headerBg + 'border-bottom:none;';
  if (iconEl)   iconEl.innerHTML = isApprove
    ? '<i class="fa-solid fa-clock" style="color:#fcd34d;"></i>'
    : '<i class="fa-solid fa-rotate-left" style="color:#fca5a5;"></i>';
  if (titleEl)  titleEl.textContent = `${status}: ${recordId}`;
  if (titleEl)  titleEl.style.color = '#fff';
  const closeBtn = document.querySelector('#statusDetailModalHeader .modal-close');
  if (closeBtn) closeBtn.style.color = '#fff';

  let bodyHtml = '';

  if (isApprove) {
    bodyHtml += `
      <div style="background:#fffbeb;border:1px solid #fcd34d;border-radius:8px;padding:12px 16px;margin-bottom:14px;font-size:12px;">
        <b style="color:#92400e;"><i class="fa-solid fa-clock"></i> 承認待ち</b>
        <div style="margin-top:6px;color:#78350f;">${rec.approvalNote || '承認申請中です。管理者の確認をお待ちください。'}</div>
      </div>
      ${rec.approvalBy ? `<div style="font-size:12px;color:var(--text-muted);margin-bottom:4px;">申請者: <b>${rec.approvalBy}</b>　${rec.approvalAt||''}</div>` : ''}`;
    if ((rec.approvalChanges||[]).length > 0) {
      const chRows = (rec.approvalChanges||[]).map(ch => `
        <tr><td style="font-size:12px;font-weight:600;">${ch.field}</td>
        <td style="font-size:12px;color:#dc2626;">${ch.before}</td>
        <td style="font-size:12px;color:#16a34a;">${ch.after}</td>
        <td style="font-size:12px;color:var(--text-muted);">${ch.reason||'—'}</td></tr>`).join('');
      bodyHtml += `<table class="data-table" style="font-size:12px;width:100%;margin-top:10px;">
        <thead><tr><th>項目</th><th>変更前</th><th>変更後</th><th>理由</th></tr></thead>
        <tbody>${chRows}</tbody></table>`;
    }
    if (isAdm) {
      bodyHtml += `
        <div class="form-group" style="margin-top:14px;">
          <label class="form-label" style="font-size:12px;">差戻しコメント（差戻し時のみ入力）</label>
          <textarea class="form-control" id="sdRevisionComment" rows="2" placeholder="差戻し理由を入力…" style="font-size:12px;"></textarea>
        </div>`;
    }
  } else if (isDiff) {
    bodyHtml += `
      <div style="background:#fef2f2;border:1px solid #fca5a5;border-radius:8px;padding:12px 16px;margin-bottom:14px;font-size:12px;">
        <b style="color:#dc2626;"><i class="fa-solid fa-rotate-left"></i> 差戻し</b>
        <div style="margin-top:6px;color:#991b1b;">${rec.revisionComment || '差戻しされました。内容を修正して再申請してください。'}</div>
      </div>`;
    if (!isAdm) {
      bodyHtml += `
        <div class="form-group" style="margin-top:10px;">
          <label class="form-label" style="font-size:12px;">修正内容・コメント</label>
          <textarea class="form-control" id="sdResubmitComment" rows="2" placeholder="修正内容を入力…" style="font-size:12px;"></textarea>
        </div>`;
    }
  }

  document.getElementById('statusDetailModalBody').innerHTML = bodyHtml;

  // フッターボタン
  let footerHtml = `<button class="btn btn-outline" onclick="closeStatusDetailModal()"><i class="fa-solid fa-xmark"></i> 閉じる</button>`;
  if (isApprove && isAdm) {
    footerHtml += `
      <button class="btn btn-danger" onclick="statusDetailAction('${recordId}','${tabType}','revision')">
        <i class="fa-solid fa-rotate-left"></i> 差戻し
      </button>
      <button class="btn btn-success" onclick="statusDetailAction('${recordId}','${tabType}','approve')">
        <i class="fa-solid fa-circle-check"></i> 承認する
      </button>`;
  } else if (isDiff && !isAdm) {
    footerHtml += `
      <button class="btn btn-primary" onclick="statusDetailResubmit('${recordId}','${tabType}')">
        <i class="fa-solid fa-paper-plane"></i> 再申請する
      </button>`;
  }
  document.getElementById('statusDetailModalFooter').innerHTML = footerHtml;

  document.getElementById('statusDetailModal').classList.remove('hidden');
}

function closeStatusDetailModal() {
  document.getElementById('statusDetailModal').classList.add('hidden');
}

function statusDetailAction(recordId, tabType, action) {
  let rec = null;
  if (tabType === 'purchase')       rec = (APP_DATA.purchaseSlips||[]).find(r => r.id === recordId);
  else if (tabType === 'shipping')  rec = (APP_DATA.shipments||[]).find(r => r.id === recordId);
  else if (tabType === 'sales')     rec = (APP_DATA.sales||[]).find(r => r.id === recordId);
  else if (tabType === 'salesreturn')    rec = (APP_DATA.salesReturns||[]).find(r => r.id === recordId);
  else if (tabType === 'purchasereturn') rec = (APP_DATA.purchaseReturns||[]).find(r => r.id === recordId);
  if (!rec) return;

  if (action === 'approve') {
    // 承認管理と同じ業務処理を通し、在庫・元伝票へも反映する。
    rec.status = tabType === 'sales' ? '確定'
      : ['salesreturn', 'purchasereturn'].includes(tabType) ? '承認済'
      : '処理済';
    if (tabType === 'purchasereturn') {
      (rec.items || []).forEach(item => { item.status = '承認済'; });
    }
    applyBusinessRecordState(tabType, rec);
    syncApprovalRequestForBusinessRecord(tabType, recordId, 'approved');
    delete rec.approvalNote; delete rec.approvalChanges; delete rec.approvalBy; delete rec.approvalAt;
    showToast('success', '承認しました', `${recordId} を${rec.status}にし、在庫へ反映しました`);
  } else {
    const comment = document.getElementById('sdRevisionComment')?.value || '';
    rec.status = '差戻し';
    rec.revisionComment = comment;
    const linkedApproval = syncApprovalRequestForBusinessRecord(tabType, recordId, 'revision', comment);
    if (tabType === 'sales' && linkedApproval) {
      (rec.items || []).forEach(recordItem => {
        const inventoryItem = (APP_DATA.inventory || []).find(item => item.code === recordItem.code);
        if (!inventoryItem || inventoryItem.reservationApprovalId !== linkedApproval.id) return;
        inventoryItem.status = inventoryItem.reservationPreviousStatus || '在庫中';
        clearInventoryReservationMetadata(inventoryItem);
      });
    }
    showToast('info', '差戻しました', `${recordId} を差戻しました`);
  }
  closeStatusDetailModal();
  refreshLinkedBusinessViews({ source: 'slip-status-approval' });
}

function statusDetailResubmit(recordId, tabType) {
  let rec = null;
  if (tabType === 'purchase')       rec = (APP_DATA.purchaseSlips||[]).find(r => r.id === recordId);
  else if (tabType === 'salesreturn')    rec = (APP_DATA.salesReturns||[]).find(r => r.id === recordId);
  else if (tabType === 'purchasereturn') rec = (APP_DATA.purchaseReturns||[]).find(r => r.id === recordId);
  if (!rec) return;
  const comment = document.getElementById('sdResubmitComment')?.value || '';
  rec.status = '承認待ち';
  rec.approvalNote = comment || '再申請されました。確認をお願いします。';
  rec.approvalBy = currentUser()?.name || '—';
  rec.approvalAt = new Date().toLocaleString('ja-JP');
  syncApprovalRequestForBusinessRecord(tabType, recordId, 'pending');
  showToast('info', '再申請しました', `${recordId} を承認待ちにしました`);
  closeStatusDetailModal();
  refreshLinkedBusinessViews({ source: 'slip-status-resubmit' });
}

// =====================================================

// =====================================================

// 一覧行の仕入返品伝票ボタンから直接プレビューを開く（詳細モーダルを開かずに）
function openPrInvoiceDirect(retId) {
  _currentPrRetId = retId;
  const ret = (APP_DATA.purchaseReturns || []).find(record => record.id === retId);
  if (ret) markPurchaseReturnDocumentIssued(ret);
  openPurchaseReturnInvoice();
}

/** @deprecated 雛形適用前の仕入返品請求書。比較参照用に保持 */
function openPurchaseReturnInvoiceLegacy() {
  const ret = (APP_DATA.purchaseReturns||[]).find(r => r.id === _currentPrRetId);
  if (!ret) return;

  const supplier   = APP_DATA.suppliers.find(s => s.code === ret.supplier) || {};
  const own        = getSlipCompanyInfo();
  const issueDate  = new Date().toLocaleDateString('ja-JP', { year:'numeric', month:'long', day:'numeric' });
  const returnDate = ret.date;

  // 合計金額
  const subtotal = (ret.items||[]).reduce((s, i) => s + (i.purchasePrice||0), 0);

  // 品目行HTML
  const itemRows = (ret.items||[]).map((it, idx) => `
    <tr>
      <td style="text-align:center;padding:9px 10px;border-bottom:1px solid #e2e8f0;font-size:13px;color:#374151;">${idx + 1}</td>
      <td style="padding:9px 10px;border-bottom:1px solid #e2e8f0;">
        <div style="font-size:13px;font-weight:600;color:#111827;">${it.brand} ${it.model}</div>
        <div style="font-size:11px;color:#6b7280;margin-top:2px;">
          商品コード: ${it.code}
          ${it.ref    ? `　型番: ${it.ref}`    : ''}
          ${it.serial ? `　シリアル: ${it.serial}` : ''}
        </div>
      </td>
      <td style="text-align:center;padding:9px 10px;border-bottom:1px solid #e2e8f0;font-size:13px;color:#374151;">1</td>
      <td style="text-align:right;padding:9px 10px;border-bottom:1px solid #e2e8f0;font-size:13px;font-weight:600;color:#1d4ed8;">${formatPrice(it.purchasePrice)}</td>
      <td style="text-align:right;padding:9px 10px;border-bottom:1px solid #e2e8f0;font-size:13px;font-weight:700;color:#111827;">${formatPrice(it.purchasePrice)}</td>
    </tr>`).join('');

  const html = `
    <div id="invoice-content" style="color:#111827;">
      <!-- ヘッダー -->
      <div style="display:flex;justify-content:space-between;align-items:flex-start;margin-bottom:32px;padding-bottom:20px;border-bottom:3px solid #1d4ed8;">
        <div>
          <div style="font-size:26px;font-weight:800;color:#1d4ed8;letter-spacing:1px;margin-bottom:4px;">仕入返品 請求書</div>
          <div style="font-size:12px;color:#6b7280;">PURCHASE RETURN INVOICE</div>
        </div>
        <div style="text-align:right;">
          <div style="font-size:16px;font-weight:700;color:#111827;">${own.companyName || '—'}</div>
          <div style="font-size:11px;color:#6b7280;margin-top:4px;line-height:1.8;">
            ${own.zip || ''} ${own.address || ''}<br>
            TEL: ${own.tel || '—'}　FAX: ${own.fax || '—'}<br>
            ${own.email || ''}<br>
            適格請求書登録番号: ${own.invoice || '—'}
          </div>
        </div>
      </div>

      <!-- 請求書番号・日付 -->
      <div style="display:flex;justify-content:space-between;margin-bottom:28px;">
        <div>
          <div style="font-size:11px;color:#6b7280;margin-bottom:3px;">請求書番号</div>
          <div style="font-size:18px;font-weight:800;color:#111827;font-family:monospace;letter-spacing:1px;">INV-${ret.id}</div>
          <div style="font-size:11px;color:#6b7280;margin-top:8px;">仕入返品伝票番号: <span style="font-weight:600;color:#111827;">${ret.id}</span></div>
        </div>
        <div style="text-align:right;">
          <table style="font-size:12px;border-collapse:collapse;margin-left:auto;">
            <tr><td style="color:#6b7280;padding:2px 10px 2px 0;">発行日</td><td style="font-weight:600;">${issueDate}</td></tr>
            <tr><td style="color:#6b7280;padding:2px 10px 2px 0;">返品日</td><td style="font-weight:600;">${returnDate}</td></tr>
            <tr><td style="color:#6b7280;padding:2px 10px 2px 0;">ステータス</td><td><span style="background:#fef2f2;color:#dc2626;border:1px solid #fca5a5;border-radius:4px;padding:1px 8px;font-size:11px;">${ret.status||'未処理'}</span></td></tr>
          </table>
        </div>
      </div>

      <!-- 請求先（仕入先） -->
      <div style="background:#f8fafc;border:1px solid #e2e8f0;border-radius:8px;padding:16px 20px;margin-bottom:24px;">
        <div style="font-size:10px;font-weight:700;color:#6b7280;text-transform:uppercase;letter-spacing:.8px;margin-bottom:8px;">請求先（仕入先）</div>
        <div style="font-size:16px;font-weight:700;color:#111827;margin-bottom:4px;">${supplier.name || '—'} 御中</div>
        <div style="font-size:12px;color:#4b5563;line-height:1.8;">
          ${supplier.address || ''}
          ${supplier.contact ? `<br>TEL: ${supplier.contact}` : ''}
          ${supplier.invoice ? `<br>適格請求書登録番号: ${supplier.invoice}` : ''}
        </div>
      </div>

      <!-- 返品理由 -->
      ${ret.reason || ret.note ? `
      <div style="background:#fff7ed;border:1px solid #fed7aa;border-radius:8px;padding:12px 16px;margin-bottom:20px;font-size:12px;">
        <span style="font-weight:700;color:#92400e;margin-right:8px;"><i class="fa-solid fa-triangle-exclamation"></i> 返品理由:</span>
        <span style="color:#78350f;">${ret.reason || ''}${ret.note ? `　${ret.note}` : ''}</span>
      </div>` : ''}

      <!-- 品目テーブル -->
      <table style="width:100%;border-collapse:collapse;margin-bottom:20px;">
        <thead>
          <tr style="background:#1d4ed8;color:#fff;">
            <th style="padding:10px;font-size:12px;font-weight:600;width:40px;text-align:center;">No.</th>
            <th style="padding:10px;font-size:12px;font-weight:600;text-align:left;">商品名 / 詳細</th>
            <th style="padding:10px;font-size:12px;font-weight:600;width:50px;text-align:center;">数量</th>
            <th style="padding:10px;font-size:12px;font-weight:600;width:130px;text-align:right;">単価</th>
            <th style="padding:10px;font-size:12px;font-weight:600;width:130px;text-align:right;">金額</th>
          </tr>
        </thead>
        <tbody>${itemRows}</tbody>
        <tfoot>
          <tr>
            <td colspan="4" style="padding:10px;text-align:right;font-size:13px;font-weight:700;border-top:2px solid #e2e8f0;">小計</td>
            <td style="padding:10px;text-align:right;font-size:13px;font-weight:700;border-top:2px solid #e2e8f0;">${formatPrice(subtotal)}</td>
          </tr>
          <tr style="background:#eff6ff;">
            <td colspan="4" style="padding:12px 10px;text-align:right;font-size:15px;font-weight:800;color:#1d4ed8;border-top:2px solid #1d4ed8;">ご請求金額（税込）</td>
            <td style="padding:12px 10px;text-align:right;font-size:17px;font-weight:900;color:#1d4ed8;border-top:2px solid #1d4ed8;">${formatPrice(subtotal)}</td>
          </tr>
        </tfoot>
      </table>

      <!-- 振込先 -->
      <div style="border:1px solid #e2e8f0;border-radius:8px;padding:14px 18px;margin-bottom:20px;">
        <div style="font-size:11px;font-weight:700;color:#6b7280;text-transform:uppercase;letter-spacing:.8px;margin-bottom:8px;">お振込先 / お支払い先</div>
        <div style="font-size:12px;color:#374151;line-height:2;">
          ${own.bankName || '—'} ${own.branchName || '—'}　${own.accountType || '普通'}　口座番号: ${own.accountNumber || '—'}<br>
          口座名義: ${own.accountHolder || '—'}
        </div>
      </div>

      <!-- 備考・起票者 -->
      <div style="display:flex;justify-content:space-between;align-items:flex-end;margin-top:16px;font-size:11px;color:#9ca3af;">
        <div>起票者: ${ret.createdBy || '—'}　起票日時: ${ret.createdAt || '—'}</div>
        <div>本請求書に関するお問合せ: ${own.tel || '—'}</div>
      </div>
    </div>`;

  document.getElementById('prInvoicePrintArea').innerHTML = html;
  document.getElementById('prInvoiceModal').classList.remove('hidden');
}

/** Excel雛形に合わせ、元仕入伝票に保存された仕入価格で仕入返品伝票を組み立てる */
function buildPurchaseReturnRecordTemplateHTML(ret) {
  const purchaseAmount = getPurchaseReturnOriginalAmountInfo(ret);
  const supplierCode = ret.supplier || purchaseAmount.sourcePurchase?.supplier || '';
  const supplier = (APP_DATA.suppliers || []).find(record => record.code === supplierCode)
    || { name: getSupplierName(supplierCode) || '（仕入先未設定）' };
  const items = purchaseAmount.items.map(item => {
    const detail = [
      [item.brand, item.model].filter(Boolean).join(' / '),
      [item.ref && `型番: ${item.ref}`, item.serial && `シリアル: ${item.serial}`].filter(Boolean).join('　'),
      item.sku ? `SKU: ${item.sku}` : '',
      item.accessories?.length ? `付属品: ${item.accessories.join('・')}` : '',
      item.note ? `備考: ${item.note}` : '',
    ].filter(Boolean).join('\n') || item.code || '—';
    return { no: item.no, detail, amount: item.purchasePrice, code: item.code };
  });
  const sourceSlipId = purchaseAmount.sourcePurchase?.id || ret.slipId || '—';
  const note = [
    `元仕入伝票：${sourceSlipId}`,
    ret.reason ? `返品理由：${ret.reason}` : '',
    ret.note ? `備考：${ret.note}` : '',
    ret.trackingNo ? `配送番号：${ret.trackingNo}` : '',
  ].filter(Boolean).join('\n');

  return buildTemplateStyleSlipDocument({
    title: '仕入返品伝票',
    slipId: ret.id,
    transactionDate: ret.date,
    transactionDateLabel: '返品日',
    counterpartyLabel: '仕入先',
    counterparty: supplier,
    items,
    note,
    formatAmount: purchaseAmount.formatAmount,
    currencyLabel: 'JPY（円）',
    taxMode: 'none',
    includeBank: false,
    summaryMessage: '仕入時の商品金額は下記の通りです。',
    amountCaption: '仕入金額合計',
    currencyNote: `元仕入伝票：${sourceSlipId}`,
  });
}

function openPurchaseReturnInvoice() {
  const ret = (APP_DATA.purchaseReturns || []).find(record => record.id === _currentPrRetId);
  if (!ret) return;
  const printArea = document.getElementById('prInvoicePrintArea');
  if (!printArea) return;
  printArea.innerHTML = buildPurchaseReturnRecordTemplateHTML(ret);
  document.getElementById('prInvoiceModal')?.classList.remove('hidden');
}

function closePurchaseReturnInvoice() {
  document.getElementById('prInvoiceModal').classList.add('hidden');
}

/** @deprecated 雛形適用前の仕入返品印刷処理。比較参照用に保持 */
function downloadInvoicePDFLegacy() {
  // 印刷ダイアログを開く（ブラウザのPDF保存機能を利用）
  const printArea = document.getElementById('prInvoicePrintArea');
  if (!printArea) return;

  // 印刷用クローンをbodyに追加してwindow.print()
  const printContent = printArea.innerHTML;
  const ret = (APP_DATA.purchaseReturns||[]).find(r => r.id === _currentPrRetId);
  // 別ウィンドウで印刷
  const win = window.open('', '_blank', 'width=900,height=700');
  win.document.write(`<!DOCTYPE html>
<html lang="ja">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>請求書 — ${ret ? ret.id : ''}</title>
<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/@fortawesome/fontawesome-free@6.4.0/css/all.min.css">
<style>
  * { margin:0; padding:0; box-sizing:border-box; }
  body { font-family: 'Helvetica Neue', Arial, 'Hiragino Sans', 'Yu Gothic', sans-serif; background:#fff; color:#111827; }
  .invoice-wrap { max-width:740px; margin:0 auto; padding:40px; }
  @media print {
    body { -webkit-print-color-adjust: exact; print-color-adjust: exact; }
    @page { size: A4; margin: 15mm 12mm; }
    .no-print { display: none !important; }
  }
  table { border-collapse: collapse; }
</style>
</head>
<body>
<div class="invoice-wrap">${printContent}</div>
<div class="no-print" style="text-align:center;margin:20px;padding:12px;background:#f0f9ff;border-radius:8px;font-size:13px;color:#0369a1;">
  <i class="fa-solid fa-print"></i> ブラウザの印刷機能（Ctrl+P / Cmd+P）でPDF保存または印刷できます。
</div>
<script>
  window.onload = function() {
    setTimeout(function() { window.print(); }, 500);
  };
<\/script>
</body>
</html>`);
  win.document.close();
}

/** 仕入返品伝票を発行済みにし、詳細画面の完了操作も有効化する */
function markPurchaseReturnDocumentIssued(ret) {
  if (!ret) return;
  ret.invoicePrinted = true;
  persistBusinessWorkflowState();
  const btn = document.getElementById('prCompleteBtn');
  if (!btn) return;
  btn.disabled = false;
  btn.style.opacity = '1';
  btn.style.cursor = 'pointer';
  btn.title = '';
}

/** 雛形準拠の仕入返品伝票を単体で保存できるHTMLへ包む */
function buildPurchaseReturnDownloadHTML(ret) {
  return `<!DOCTYPE html><html lang="ja"><head><meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>${_escHtml(`仕入返品伝票 — ${ret.id}`)}</title></head>
    <body style="margin:0;background:#eee;">${buildPurchaseReturnRecordTemplateHTML(ret)}</body></html>`;
}

/** 仕入返品伝票をレイアウト保持HTMLとしてダウンロードする */
function downloadPurchaseReturnDocument() {
  const ret = (APP_DATA.purchaseReturns || []).find(record => record.id === _currentPrRetId);
  if (!ret) {
    showToast('warning', '仕入返品伝票', '対象の仕入返品データが見つかりません。');
    return;
  }
  markPurchaseReturnDocumentIssued(ret);
  const blob = new Blob(['\uFEFF', buildPurchaseReturnDownloadHTML(ret)], { type: 'text/html;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  const safeId = String(ret.id || 'purchase-return').replace(/[^0-9A-Za-z_-]/g, '_');
  anchor.href = url;
  anchor.download = `${safeId}_仕入返品伝票.html`;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  setTimeout(() => URL.revokeObjectURL(url), 0);
  showToast('success', 'ダウンロードしました', `${ret.id} の仕入返品伝票を保存しました。`);
}

/** 雛形準拠の仕入返品伝票を印刷ダイアログへ送る */
function printPurchaseReturnDocument() {
  const ret = (APP_DATA.purchaseReturns || []).find(record => record.id === _currentPrRetId);
  if (!ret) {
    showToast('warning', '仕入返品伝票', '対象の仕入返品データが見つかりません。');
    return;
  }
  markPurchaseReturnDocumentIssued(ret);
  _openTemplatePrintWindow(`仕入返品伝票 — ${ret.id}`, buildPurchaseReturnRecordTemplateHTML(ret));
}
// =====================================================

// =====================================================
// 売上返品起票
// =====================================================
let _salesReturnTargetId = null;  // 対象売上伝票ID
let _currentSrRetId      = null;  // 現在開いている売上返品伝票ID
let _srAddedItems        = [];    // 追加済み返品商品リスト

function openSalesReturnModal(slipId) {
  const sale = APP_DATA.sales.find(s => s.id === slipId);
  if (!sale) return;
  _salesReturnTargetId = slipId;
  _srAddedItems = [];

  // 伝票ヘッダー情報表示
  document.getElementById('salesReturnSlipInfo').innerHTML = `
    <div style="background:#f8fafc;border:1px solid var(--border);border-radius:8px;padding:12px 16px;">
      <div style="font-size:11px;color:var(--text-muted);margin-bottom:6px;text-transform:uppercase;font-weight:700;">返品元 売上伝票</div>
      <div style="display:flex;gap:16px;flex-wrap:wrap;align-items:center;font-size:13px;">
        <span><code style="color:var(--primary-light);">${sale.id}</code></span>
        <span><i class="fa-regular fa-calendar"></i> ${sale.date}</span>
        <span><i class="fa-solid fa-building"></i> ${getBuyerName(sale.buyer)}</span>
        <span style="font-weight:bold;color:var(--primary);">${formatSalePrice(sale.total)}</span>
      </div>
    </div>`;

  _srRenderAddedItems();

  document.getElementById('sr-return-date').value = getLocalDateISO();
  document.getElementById('sr-return-reason').value = '';
  document.getElementById('sr-return-note').value = '';

  const inp = document.getElementById('sr-add-code-input');
  if (inp) { inp.value = ''; setTimeout(() => inp.focus(), 300); }

  closeSlipDetail();
  document.getElementById('salesReturnModal').classList.remove('hidden');
}

/** 追加済みアイテムリストを再描画（売上返品） */
function _srRenderAddedItems() {
  const listEl  = document.getElementById('salesReturnItemList');
  const countEl = document.getElementById('sr-item-count-badge');
  const totalEl = document.getElementById('sr-total-badge');
  if (!listEl) return;

  const total = _srAddedItems.reduce((s, it) => s + (it.salePrice || 0), 0);
  if (countEl) countEl.textContent = `${_srAddedItems.length}点`;
  if (totalEl) totalEl.textContent = _srAddedItems.length > 0 ? `合計: ${formatSalePrice(total)}` : '';

  if (_srAddedItems.length === 0) {
    listEl.innerHTML = `<div style="padding:14px;text-align:center;color:var(--text-muted);font-size:12px;">
      <i class="fa-solid fa-barcode" style="font-size:20px;display:block;margin-bottom:6px;opacity:0.4;"></i>
      商品コードを入力またはスキャンして追加してください
    </div>`;
    return;
  }

  listEl.innerHTML = _srAddedItems.map((it, i) => `
    <div style="display:flex;align-items:center;gap:10px;padding:9px 14px;border-bottom:1px solid var(--border);font-size:12px;background:#fff;">
      <div style="flex:1;min-width:0;">
        <div style="font-weight:700;color:var(--text);">${it.brand} ${it.model}</div>
        <div style="color:var(--text-muted);margin-top:2px;">
          <code style="font-size:10px;">${it.code}</code>
          ${it.ref    ? `　型番: ${it.ref}`    : ''}
          ${it.serial ? `　S/N: ${it.serial}`  : ''}
        </div>
      </div>
      <span style="font-weight:700;color:#7c3aed;white-space:nowrap;">${formatSalePrice(it.salePrice || 0)}</span>
      <button onclick="srRemoveItem(${i})" title="削除"
        style="background:none;border:none;color:#dc2626;cursor:pointer;font-size:14px;padding:2px 4px;flex-shrink:0;">
        <i class="fa-solid fa-trash-can"></i>
      </button>
    </div>`).join('');
}

/** リストから削除（売上返品） */
function srRemoveItem(idx) {
  _srAddedItems.splice(idx, 1);
  _srRenderAddedItems();
}

/**
 * 商品コード入力で追加（売上返品）
 * 検索順: ① 売上伝票の items ② inventory
 * 重複禁止・該当なしエラー
 */
function srAddItemByCode(codeArg) {
  const inp  = document.getElementById('sr-add-code-input');
  const code = (codeArg || (inp ? inp.value.trim() : '')).trim();
  if (!code) { showToast('error', '入力エラー', '商品コードを入力してください'); return; }

  const slipId = _salesReturnTargetId;
  const sale   = slipId ? APP_DATA.sales.find(s => s.id === slipId) : null;

  // 重複チェック
  if (_srAddedItems.some(it => it.code === code)) {
    showToast('warning', '重複', `商品コード "${code}" はすでに追加されています`);
    if (inp) inp.value = '';
    return;
  }

  let found = null;

  // ① 売上伝票の items から検索（returnTypeがnullの返品可能商品）
  if (sale) {
    const saleItem = (sale.items || []).find(it =>
      it.code === code && (!it.returnType || it.returnType === null)
    );
    if (saleItem) {
      const inv = (APP_DATA.inventory || []).find(i => i.code === code);
      found = {
        code:      saleItem.code,
        brand:     saleItem.brand  || inv?.brand  || '',
        model:     saleItem.model  || inv?.model  || '',
        ref:       saleItem.ref    || inv?.ref    || '',
        serial:    saleItem.serial || inv?.serial || '',
        salePrice: saleItem.salePrice || 0,
      };
    }
  }

  // ② 在庫から検索（売上伝票に紐付いていない場合も許容）
  if (!found) {
    const inv = (APP_DATA.inventory || []).find(i => i.code === code);
    if (inv) {
      found = {
        code:      inv.code,
        brand:     inv.brand  || '',
        model:     inv.model  || '',
        ref:       inv.ref    || '',
        serial:    inv.serial || '',
        salePrice: inv.salePrice || 0,
      };
    }
  }

  if (!found) {
    showToast('error', '該当商品がありません', `商品コード "${code}" は登録されていません`);
    if (inp) { inp.value = ''; inp.focus(); }
    return;
  }

  _srAddedItems.push(found);
  _srRenderAddedItems();
  showToast('success', '追加しました', `${found.brand} ${found.model}`);
  if (inp) { inp.value = ''; inp.focus(); }
}

function closeSalesReturnModal() {
  document.getElementById('salesReturnModal').classList.add('hidden');
  _salesReturnTargetId = null;
  _srAddedItems = [];
}

async function submitSalesReturn() {
  const slipId = _salesReturnTargetId;
  const sale   = slipId ? APP_DATA.sales.find(s => s.id === slipId) : null;
  if (!sale) { showToast('error', 'エラー', '対象売上伝票が見つかりません'); return; }

  if (_srAddedItems.length === 0) { showToast('error', '入力エラー', '返品する商品を1点以上追加してください'); return; }

  const date   = document.getElementById('sr-return-date').value;
  const reason = document.getElementById('sr-return-reason').value;
  const note   = document.getElementById('sr-return-note').value;
  if (!date) { showToast('error', '入力エラー', '返品日を入力してください'); return; }

  const retItems = _srAddedItems.map(it => ({
    code:      it.code,
    brand:     it.brand,
    model:     it.model,
    ref:       it.ref,
    serial:    it.serial,
    salePrice: it.salePrice,
  }));

  if (window.ZaikoAPI) {
    try {
      const result = await window.ZaikoAPI.saveReturn({ operationType: 'return', transactionDate: date,
        buyerCode: sale.buyer || '', reason: reason || '売上返品', notes: note || `元売上伝票: ${sale.id}`,
        productCodes: retItems.map(item => item.code) }, typeof isWorker === 'function' && isWorker());
      closeSalesReturnModal();
      const slipNo = result.record?.slipNumber || result.record?.id || '';
      showToast('success', result.approval ? '承認申請を送信しました' : '売上返品を確定しました',
        result.approval ? `${slipNo} を管理者の承認待ちにしました` : `${slipNo} をDBへ保存し在庫へ戻しました`);
      refreshLinkedBusinessViews({ source: 'sales-return-api' });
    } catch (error) {
      showToast('error', '売上返品を登録できませんでした', error.message || '商品状態を確認してください');
    }
    return;
  }

  if (!APP_DATA.salesReturns) APP_DATA.salesReturns = [];
  const id  = `SR-${String(APP_DATA.salesReturns.length + 1).padStart(4, '0')}`;
  const ret = {
    id, date,
    slipId,
    buyer:   sale.buyer,
    items:   retItems,
    total:   retItems.reduce((s, i) => s + (i.salePrice||0), 0),
    reason, note,
    status:  '未処理',
    createdBy:  currentUser()?.name || '—',
    createdAt:  new Date().toLocaleString('ja-JP'),
    invoicePrinted: false,
  };

  const applyReturnMark = (codes) => {
    codes.forEach(code => {
      const it = (sale.items||[]).find(i => i.code === code);
      if (it) { it.returnType = 'return'; it.returnStatus = 'pending'; }
      const inv = (APP_DATA.inventory||[]).find(i => i.code === code);
      if (inv) inv.status = '在庫中';
    });
  };

  if (isBuyer()) {
    ret.status = '承認待ち';
    APP_DATA.salesReturns.push(ret);
    requestApproval(
      'sales_return', '売上返品起票',
      { retId: id, slipId, buyer: sale.buyer, items: retItems, total: ret.total, date, reason, note, record: _approvalClone(ret) },
      note, null
    );
    closeSalesReturnModal();
    showToast('info', '承認申請を送信しました', `管理者の承認待ちです（${id}）`);
  } else {
    APP_DATA.salesReturns.push(ret);
    applyReturnMark(retItems.map(i => i.code));
    closeSalesReturnModal();
    showToast('success', '売上返品伝票を起票しました', `${id}（${retItems.length}点）を作成しました`);
  }
  refreshLinkedBusinessViews({ source: 'sales-return' });
}

/** 売上返品に対応する元売上伝票から、販売時点の金額・通貨・税区分を復元する */
function getSalesReturnOriginalAmountInfo(ret) {
  const sourceSale = (APP_DATA.sales || []).find(sale => sale.id === ret?.slipId) || null;
  const inputCurrency = sourceSale?.inputCurrency === 'JPY' ? 'JPY' : 'USD';
  const rate = Number(sourceSale?.usdJpyRate) > 0 ? Number(sourceSale.usdJpyRate) : getSalesUsdRate();
  const formatAmount = amount => inputCurrency === 'JPY'
    ? formatPrice(Math.round((Number(amount) || 0) * rate))
    : formatSalePrice(amount);
  const taxMode = sourceSale ? (sourceSale.taxFree ? 'exempt' : 'standard') : 'none';

  const items = (ret?.items || []).map((returnItem, index) => {
    const sourceLine = (sourceSale?.items || []).find(line => line.code === returnItem.code) || null;
    const inventoryItem = (APP_DATA.inventory || []).find(item => item.code === returnItem.code) || {};
    const sourcePrice = sourceLine && Number.isFinite(Number(sourceLine.salePrice))
      ? Number(sourceLine.salePrice)
      : Number(returnItem.salePrice) || 0;
    return {
      ...returnItem,
      ...sourceLine,
      no: index + 1,
      code: returnItem.code || sourceLine?.code || '',
      brand: sourceLine?.brand || returnItem.brand || inventoryItem.brand || '',
      model: sourceLine?.model || returnItem.model || inventoryItem.model || '',
      ref: sourceLine?.ref || returnItem.ref || inventoryItem.ref || '',
      serial: sourceLine?.serial || returnItem.serial || inventoryItem.serial || '',
      accessories: inventoryItem.accessories || [],
      note: inventoryItem.note || '',
      salePrice: sourcePrice,
    };
  });
  const subtotal = items.reduce((sum, item) => sum + (Number(item.salePrice) || 0), 0);
  const taxAmount = taxMode === 'standard' ? Math.floor(subtotal * 0.1) : 0;

  return {
    sourceSale,
    inputCurrency,
    rate,
    taxMode,
    items,
    subtotal,
    taxAmount,
    grandTotal: subtotal + taxAmount,
    formatAmount,
  };
}

// ── 売上返品詳細モーダル ──
function openSalesReturnDetail(retId) {
  _currentSrRetId = retId;
  const ret  = (APP_DATA.salesReturns||[]).find(r => r.id === retId);
  if (!ret) return;
  const saleAmount = getSalesReturnOriginalAmountInfo(ret);

  // ステータスバッジ（統一ルールを使用）
  const statusBadge = _slipStatusBadge(ret.status, ret.id, 'salesreturn');

  document.getElementById('srDetailModalTitle').textContent = `売上返品伝票 ${ret.id}`;
  document.getElementById('salesReturnDetailBody').innerHTML = `
    <div class="detail-grid mb-20">
      <div class="detail-row"><div class="detail-label">伝票番号</div><div class="detail-value"><code>${ret.id}</code></div></div>
      <div class="detail-row"><div class="detail-label">返品日</div><div class="detail-value">${ret.date}</div></div>
      <div class="detail-row"><div class="detail-label">元売上伝票</div><div class="detail-value"><code>${ret.slipId||'—'}</code></div></div>
      <div class="detail-row"><div class="detail-label">販売先</div><div class="detail-value">${getBuyerName(ret.buyer)}</div></div>
      <div class="detail-row"><div class="detail-label">ステータス</div><div class="detail-value">${statusBadge || '<span style="color:var(--text-muted);font-size:12px;">—</span>'}</div></div>
      <div class="detail-row"><div class="detail-label">返品理由</div><div class="detail-value">${ret.reason||'—'}</div></div>
      <div class="detail-row"><div class="detail-label">起票者</div><div class="detail-value">${ret.createdBy}</div></div>
      <div class="detail-row"><div class="detail-label">起票日時</div><div class="detail-value">${ret.createdAt}</div></div>
    </div>
    ${ret.note ? `<div class="form-group mb-20"><div class="detail-label">備考</div>
      <div style="background:var(--bg);padding:10px;border-radius:6px;font-size:12px;">${ret.note}</div></div>` : ''}
    <div style="font-size:12px;font-weight:700;color:var(--text-muted);margin-bottom:8px;text-transform:uppercase;letter-spacing:.5px;">
      <i class="fa-solid fa-list-check"></i> 返品商品（${(ret.items||[]).length}点）
    </div>
    <table class="data-table" style="width:100%;">
      <thead><tr>
        <th>商品コード</th><th>ブランド</th><th>モデル</th><th>型番</th><th>シリアル</th>
        <th style="text-align:right;">販売時金額（${saleAmount.inputCurrency}）</th>
      </tr></thead>
      <tbody>
        ${saleAmount.items.map(it => `<tr>
          <td><code style="font-size:11px;">${it.code}</code></td>
          <td>${it.brand}</td>
          <td>${it.model}</td>
          <td style="font-size:11px;color:var(--text-muted);">${it.ref||'—'}</td>
          <td style="font-size:11px;color:var(--text-muted);">${it.serial||'—'}</td>
          <td style="text-align:right;font-weight:bold;">${saleAmount.formatAmount(it.salePrice||0)}</td>
        </tr>`).join('')}
        <tr style="background:#f5f3ff;font-weight:700;">
          <td colspan="5" style="text-align:right;">販売時合計${saleAmount.taxMode === 'standard' ? '（税込）' : saleAmount.taxMode === 'exempt' ? '（免税）' : ''}</td>
          <td style="text-align:right;color:#7c3aed;">${saleAmount.formatAmount(saleAmount.grandTotal)}</td>
        </tr>
      </tbody>
    </table>`;

  // ⑤ 完了ボタンの有効/無効をinvoicePrintedで制御
  const completeBtn = document.getElementById('srCompleteBtn');
  if (completeBtn) {
    if (ret.invoicePrinted || ret.status === '処理済') {
      completeBtn.disabled = false;
      completeBtn.style.opacity = '1';
      completeBtn.style.cursor  = 'pointer';
      completeBtn.title = '';
    } else {
      completeBtn.disabled = true;
      completeBtn.style.opacity = '0.4';
      completeBtn.style.cursor  = 'not-allowed';
      completeBtn.title = '売上返品伝票の表示・ダウンロード・印刷後に押せます';
    }
    if (ret.status === '処理済') {
      completeBtn.innerHTML = '<i class="fa-solid fa-circle-check"></i> 完了済';
      completeBtn.disabled  = true;
      completeBtn.style.opacity = '0.7';
    }
  }

  document.getElementById('salesReturnDetailModal').classList.remove('hidden');
}

/** ⑤ 売上返品伝票の表示フラグをセット */
function srIssueInvoice() {
  const ret = (APP_DATA.salesReturns||[]).find(r => r.id === _currentSrRetId);
  if (ret) { ret.invoicePrinted = true; persistBusinessWorkflowState(); }
  openSalesReturnInvoice();
  // 完了ボタンを有効化
  setTimeout(() => {
    const btn = document.getElementById('srCompleteBtn');
    if (btn && btn.innerHTML.indexOf('完了済') === -1) {
      btn.disabled      = false;
      btn.style.opacity = '1';
      btn.style.cursor  = 'pointer';
      btn.title         = '';
    }
  }, 200);
}

/** ⑤ 売上返品 完了処理 */
function srCompleteReturn() {
  const ret = (APP_DATA.salesReturns||[]).find(r => r.id === _currentSrRetId);
  if (!ret) return;
  if (ret.status === '処理済') { showToast('info', 'すでに完了済です', ''); return; }
  if (!confirm('この売上返品を「処理済」に変更します。よろしいですか？')) return;
  ret.status = '処理済';
  showToast('success', '処理済に更新しました', `${ret.id} を完了しました`);
  refreshLinkedBusinessViews({ source: 'sales-return-complete' });
  openSalesReturnDetail(_currentSrRetId); // 再描画
}

function closeSalesReturnDetailModal() {
  document.getElementById('salesReturnDetailModal').classList.add('hidden');
}

// ── 売上返品伝票プレビュー ──
/** @deprecated 雛形適用前の売上返品請求書。比較参照用に保持 */
function openSalesReturnInvoiceLegacy() {
  const ret = (APP_DATA.salesReturns||[]).find(r => r.id === _currentSrRetId);
  if (!ret) return;

  const buyer      = APP_DATA.buyers.find(b => b.code === ret.buyer) || {};
  const own        = getSlipCompanyInfo();
  const issueDate  = new Date().toLocaleDateString('ja-JP', { year:'numeric', month:'long', day:'numeric' });

  const itemRows = (ret.items||[]).map((it, idx) => `
    <tr>
      <td style="text-align:center;padding:9px 10px;border-bottom:1px solid #e2e8f0;font-size:13px;color:#374151;">${idx + 1}</td>
      <td style="padding:9px 10px;border-bottom:1px solid #e2e8f0;">
        <div style="font-size:13px;font-weight:600;color:#111827;">${it.brand} ${it.model}</div>
        <div style="font-size:11px;color:#6b7280;margin-top:2px;">
          商品コード: ${it.code}
          ${it.ref    ? `　型番: ${it.ref}`    : ''}
          ${it.serial ? `　シリアル: ${it.serial}` : ''}
        </div>
      </td>
      <td style="text-align:center;padding:9px 10px;border-bottom:1px solid #e2e8f0;font-size:13px;">1</td>
      <td style="text-align:right;padding:9px 10px;border-bottom:1px solid #e2e8f0;font-size:13px;font-weight:600;color:#7c3aed;">${formatSalePrice(it.salePrice||0)}</td>
      <td style="text-align:right;padding:9px 10px;border-bottom:1px solid #e2e8f0;font-size:13px;font-weight:700;color:#111827;">${formatSalePrice(it.salePrice||0)}</td>
    </tr>`).join('');

  const html = `
    <div id="sr-invoice-content" style="color:#111827;">
      <!-- ヘッダー -->
      <div style="display:flex;justify-content:space-between;align-items:flex-start;margin-bottom:32px;padding-bottom:20px;border-bottom:3px solid #7c3aed;">
        <div>
          <div style="font-size:26px;font-weight:800;color:#7c3aed;letter-spacing:1px;margin-bottom:4px;">売上返品 請求書</div>
          <div style="font-size:12px;color:#6b7280;">SALES RETURN INVOICE</div>
        </div>
        <div style="text-align:right;">
          <div style="font-size:16px;font-weight:700;color:#111827;">${own.companyName||'—'}</div>
          <div style="font-size:11px;color:#6b7280;margin-top:4px;line-height:1.8;">
            ${own.zip||''} ${own.address||''}<br>
            TEL: ${own.tel||'—'}　FAX: ${own.fax||'—'}<br>
            ${own.email||''}<br>
            適格請求書登録番号: ${own.invoice||'—'}
          </div>
        </div>
      </div>
      <!-- 請求書番号・日付 -->
      <div style="display:flex;justify-content:space-between;margin-bottom:28px;">
        <div>
          <div style="font-size:11px;color:#6b7280;margin-bottom:3px;">請求書番号</div>
          <div style="font-size:18px;font-weight:800;color:#111827;font-family:monospace;letter-spacing:1px;">INV-${ret.id}</div>
          <div style="font-size:11px;color:#6b7280;margin-top:8px;">元売上伝票番号: <span style="font-weight:600;color:#111827;">${ret.slipId||'—'}</span></div>
          <div style="font-size:11px;color:#6b7280;">売上返品伝票番号: <span style="font-weight:600;color:#111827;">${ret.id}</span></div>
        </div>
        <div style="text-align:right;">
          <table style="font-size:12px;border-collapse:collapse;margin-left:auto;">
            <tr><td style="color:#6b7280;padding:2px 10px 2px 0;">発行日</td><td style="font-weight:600;">${issueDate}</td></tr>
            <tr><td style="color:#6b7280;padding:2px 10px 2px 0;">返品日</td><td style="font-weight:600;">${ret.date}</td></tr>
            <tr><td style="color:#6b7280;padding:2px 10px 2px 0;">ステータス</td>
              <td><span style="background:#f5f3ff;color:#7c3aed;border:1px solid #c4b5fd;border-radius:4px;padding:1px 8px;font-size:11px;">${ret.status||'未処理'}</span></td></tr>
          </table>
        </div>
      </div>
      <!-- 請求先（販売先） -->
      <div style="background:#f8fafc;border:1px solid #e2e8f0;border-radius:8px;padding:16px 20px;margin-bottom:24px;">
        <div style="font-size:10px;font-weight:700;color:#6b7280;text-transform:uppercase;letter-spacing:.8px;margin-bottom:8px;">請求先（販売先）</div>
        <div style="font-size:16px;font-weight:700;color:#111827;margin-bottom:4px;">${buyer.name||'—'} 御中</div>
        <div style="font-size:12px;color:#4b5563;line-height:1.8;">
          ${buyer.address||''}
          ${buyer.contact ? `<br>TEL: ${buyer.contact}` : ''}
          ${buyer.invoice ? `<br>適格請求書登録番号: ${buyer.invoice}` : ''}
        </div>
      </div>
      <!-- 返品理由 -->
      ${ret.reason||ret.note ? `
      <div style="background:#f5f3ff;border:1px solid #c4b5fd;border-radius:8px;padding:12px 16px;margin-bottom:20px;font-size:12px;">
        <span style="font-weight:700;color:#5b21b6;margin-right:8px;"><i class="fa-solid fa-rotate-left"></i> 返品理由:</span>
        <span style="color:#4c1d95;">${ret.reason||''}${ret.note ? `　${ret.note}` : ''}</span>
      </div>` : ''}
      <!-- 品目テーブル -->
      <table style="width:100%;border-collapse:collapse;margin-bottom:20px;">
        <thead>
          <tr style="background:#7c3aed;color:#fff;">
            <th style="padding:10px;font-size:12px;font-weight:600;width:40px;text-align:center;">No.</th>
            <th style="padding:10px;font-size:12px;font-weight:600;text-align:left;">商品名 / 詳細</th>
            <th style="padding:10px;font-size:12px;font-weight:600;width:50px;text-align:center;">数量</th>
            <th style="padding:10px;font-size:12px;font-weight:600;width:130px;text-align:right;">単価</th>
            <th style="padding:10px;font-size:12px;font-weight:600;width:130px;text-align:right;">返品金額</th>
          </tr>
        </thead>
        <tbody>${itemRows}</tbody>
        <tfoot>
          <tr>
            <td colspan="4" style="padding:10px;text-align:right;font-size:13px;font-weight:700;border-top:2px solid #e2e8f0;">小計</td>
            <td style="padding:10px;text-align:right;font-size:13px;font-weight:700;border-top:2px solid #e2e8f0;">${formatSalePrice(ret.total||0)}</td>
          </tr>
          <tr style="background:#f5f3ff;">
            <td colspan="4" style="padding:12px 10px;text-align:right;font-size:15px;font-weight:800;color:#7c3aed;border-top:2px solid #7c3aed;">ご請求金額（税込）</td>
            <td style="padding:12px 10px;text-align:right;font-size:17px;font-weight:900;color:#7c3aed;border-top:2px solid #7c3aed;">${formatSalePrice(ret.total||0)}</td>
          </tr>
        </tfoot>
      </table>
      <!-- フッター -->
      <div style="margin-top:32px;padding-top:16px;border-top:1px solid #e2e8f0;font-size:11px;color:#9ca3af;text-align:center;">
        ${own.companyName||''} / ${own.address||''} / TEL: ${own.tel||''} / ${own.email||''}
      </div>
    </div>`;

  document.getElementById('srInvoicePrintArea').innerHTML = html;
  document.getElementById('srInvoiceModal').classList.remove('hidden');
}

/** Excel雛形に合わせ、元売上伝票に保存された販売時点の金額で売上返品伝票を組み立てる */
function buildSalesReturnRecordTemplateHTML(ret) {
  const saleAmount = getSalesReturnOriginalAmountInfo(ret);
  const buyerCode = ret.buyer || saleAmount.sourceSale?.buyer || '';
  const buyer = (APP_DATA.buyers || []).find(record => record.code === buyerCode)
    || { name: getBuyerName(buyerCode) || '（販売先未設定）' };
  const items = saleAmount.items.map(item => {
    const detail = [
      [item.brand, item.model].filter(Boolean).join(' / '),
      [item.ref && `型番: ${item.ref}`, item.serial && `シリアル: ${item.serial}`].filter(Boolean).join('　'),
      item.accessories?.length ? `付属品: ${item.accessories.join('・')}` : '',
      item.note ? `備考: ${item.note}` : '',
    ].filter(Boolean).join('\n') || item.code || '—';
    return { no: item.no, detail, amount: item.salePrice, code: item.code };
  });
  const note = [
    `元売上伝票：${ret.slipId || '—'}`,
    ret.reason ? `返品理由：${ret.reason}` : '',
    ret.note ? `備考：${ret.note}` : '',
  ].filter(Boolean).join('\n');
  const rateLabel = saleAmount.rate.toLocaleString('ja-JP', {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });

  return buildTemplateStyleSlipDocument({
    title: '売上返品伝票',
    slipId: ret.id,
    transactionDate: ret.date,
    transactionDateLabel: '返品日',
    counterpartyLabel: '販売先',
    counterparty: buyer,
    items,
    note,
    formatAmount: saleAmount.formatAmount,
    currencyLabel: saleAmount.inputCurrency === 'JPY' ? 'JPY（円）' : 'USD',
    taxMode: saleAmount.taxMode,
    includeBank: false,
    summaryMessage: '販売時の商品金額は下記の通りです。',
    amountCaption: saleAmount.taxMode === 'standard'
      ? '販売時合計金額（税込）'
      : saleAmount.taxMode === 'exempt' ? '販売時合計金額（免税）' : '販売時合計金額',
    currencyNote: `元売上伝票：${ret.slipId || '—'}　基準通貨：USD　販売時レート：1 USD = ¥${rateLabel}`,
  });
}

function openSalesReturnInvoice() {
  const ret = (APP_DATA.salesReturns || []).find(record => record.id === _currentSrRetId);
  if (!ret) return;
  const printArea = document.getElementById('srInvoicePrintArea');
  if (!printArea) return;
  printArea.innerHTML = buildSalesReturnRecordTemplateHTML(ret);
  document.getElementById('srInvoiceModal')?.classList.remove('hidden');
}

function closeSalesReturnInvoice() {
  document.getElementById('srInvoiceModal').classList.add('hidden');
}

// 一覧行の売上返品伝票ボタンから直接プレビューを開く
function openSrInvoiceDirect(retId) {
  _currentSrRetId = retId;
  // 詳細モーダルが開いていない状態でも帳票表示済みフラグを立てる
  const ret = (APP_DATA.salesReturns||[]).find(r => r.id === retId);
  if (ret) { ret.invoicePrinted = true; persistBusinessWorkflowState(); }
  openSalesReturnInvoice();
}

/** @deprecated 雛形適用前の印刷処理。比較参照用に保持 */
function printSalesReturnInvoiceLegacy() {
  const ret = (APP_DATA.salesReturns||[]).find(r => r.id === _currentSrRetId);
  // 印刷フラグをセット → 完了ボタン有効化
  if (ret) { ret.invoicePrinted = true; persistBusinessWorkflowState(); }
  setTimeout(() => {
    const btn = document.getElementById('srCompleteBtn');
    if (btn && !btn.innerHTML.includes('完了済')) {
      btn.disabled = false; btn.style.opacity = '1'; btn.style.cursor = 'pointer'; btn.title = '';
    }
  }, 200);
  const printArea = document.getElementById('srInvoicePrintArea');
  if (!printArea) return;
  const win = window.open('', '_blank', 'width=900,height=700');
  win.document.write(`<!DOCTYPE html>
<html lang="ja"><head>
<meta charset="UTF-8">
<title>売上返品請求書 — ${ret ? ret.id : ''}</title>
<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/@fortawesome/fontawesome-free@6.4.0/css/all.min.css">
<style>
  * { margin:0; padding:0; box-sizing:border-box; }
  body { font-family: 'Helvetica Neue', Arial, 'Hiragino Sans', 'Yu Gothic', sans-serif; background:#fff; color:#111827; }
  .invoice-wrap { max-width:740px; margin:0 auto; padding:40px; }
  @media print {
    body { -webkit-print-color-adjust: exact; print-color-adjust: exact; }
    @page { size: A4; margin: 15mm 12mm; }
    .no-print { display: none !important; }
  }
  table { border-collapse: collapse; }
</style>
</head><body>
<div class="invoice-wrap">${printArea.innerHTML}</div>
<div class="no-print" style="text-align:center;margin:20px;padding:12px;background:#f5f3ff;border-radius:8px;font-size:13px;color:#7c3aed;">
  <i class="fa-solid fa-print"></i> ブラウザの印刷機能（Ctrl+P / Cmd+P）でPDF保存または印刷できます。
</div>
<script>window.onload=function(){setTimeout(function(){window.print();},500);};<\/script>
</body></html>`);
  win.document.close();
}

function markSalesReturnDocumentIssued(ret) {
  if (!ret) return;
  ret.invoicePrinted = true;
  persistBusinessWorkflowState();
  const btn = document.getElementById('srCompleteBtn');
  if (btn && !btn.innerHTML.includes('完了済')) {
    btn.disabled = false;
    btn.style.opacity = '1';
    btn.style.cursor = 'pointer';
    btn.title = '';
  }
}

function buildSalesReturnDownloadHTML(ret) {
  return `<!DOCTYPE html>
<html lang="ja"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>売上返品伝票 — ${_escHtml(ret.id || '')}</title></head>
<body style="margin:0;background:#eee;">${buildSalesReturnRecordTemplateHTML(ret)}</body></html>`;
}

/** プレビュー中の売上返品伝票を、雛形レイアウトを保持したHTML帳票として保存する */
function downloadSalesReturnDocument() {
  const ret = (APP_DATA.salesReturns || []).find(record => record.id === _currentSrRetId);
  if (!ret) {
    showToast('warn', 'ダウンロード', '売上返品伝票が見つかりません');
    return;
  }
  markSalesReturnDocumentIssued(ret);
  const blob = new Blob(['\uFEFF', buildSalesReturnDownloadHTML(ret)], { type: 'text/html;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = `${String(ret.id || 'sales-return').replace(/[\\/:*?"<>|]/g, '_')}_売上返品伝票.html`;
  document.body.appendChild(link);
  link.click();
  link.remove();
  setTimeout(() => URL.revokeObjectURL(url), 0);
  showToast('success', 'ダウンロード完了', `${ret.id} の売上返品伝票を保存しました`);
}

function printSalesReturnInvoice() {
  const ret = (APP_DATA.salesReturns || []).find(record => record.id === _currentSrRetId);
  if (!ret) return;
  markSalesReturnDocumentIssued(ret);
  _openTemplatePrintWindow(`売上返品伝票 — ${ret.id}`, buildSalesReturnRecordTemplateHTML(ret));
}

// =====================================================

// =====================================================
// 現在開いている詳細モーダルの type / id を保持
let _reviseContext = { type: null, id: null };

function openSlipRevise(type, id) {
  const modal = document.getElementById('slipReviseOverlay');
  if (!modal) return;

  _reviseContext = { type, id };

  // 対象レコードを取得
  let rec = null;
  if (type === 'purchase')      rec = (APP_DATA.purchaseSlips || []).find(s => s.id === id);
  else if (type === 'shipping') rec = APP_DATA.shipments.find(s => s.id === id);
  else                          rec = APP_DATA.sales.find(s => s.id === id);
  if (!rec) return;

  document.getElementById('slipReviseId').value   = id;
  document.getElementById('slipReviseType').value = type;

  const typeLabel = { purchase:'仕入伝票', shipping:'出荷伝票', sales:'売上伝票' }[type];
  document.getElementById('slipReviseTarget').textContent = `${typeLabel}　${id}`;
  document.getElementById('slipReviseNote').value = '';

  // タイプ別フィールドを描画
  document.getElementById('slipReviseFields').innerHTML = buildReviseFields(type, rec);

  // 詳細モーダルは閉じずに修正モーダルを上に重ねて表示
  modal.classList.remove('hidden');
}

// ── タイプ別フォームを生成 ──────────────────────────
function buildReviseFields(type, rec) {
  if (type === 'purchase') {
    // 明細行HTML
    const linesHtml = (rec.lines || []).map((l, idx) => {
      const d = l.productDetail || {};
      return `
      <div class="revise-item-row" id="rv-pu-line-${idx}" style="display:grid;grid-template-columns:30px 1fr 1fr 110px 110px;gap:6px;align-items:center;padding:6px 0;border-bottom:1px solid var(--border);">
        <span style="font-size:11px;font-weight:bold;color:var(--text-muted);text-align:center;">${l.lineNo}</span>
        <div>
          <div style="font-size:10px;color:var(--text-muted);margin-bottom:2px;">商品コード</div>
          <code style="font-size:11px;color:var(--primary);">${l.code}</code>
        </div>
        <div>
          <div style="font-size:10px;color:var(--text-muted);margin-bottom:2px;">SKU</div>
          <input class="form-control" style="font-size:12px;" id="rv-pu-sku-${idx}" value="${l.sku||''}">
        </div>
        <div>
          <div style="font-size:10px;color:var(--text-muted);margin-bottom:2px;">仕入金額</div>
          <input type="text" inputmode="numeric" class="form-control" style="font-size:12px;" id="rv-pu-pp-${idx}"
            value="${formatPriceInput(l.purchasePrice||0)}" data-raw-value="${l.purchasePrice||0}"
            oninput="priceFormatHandler(this)" onblur="priceFormatHandler(this)">
        </div>
        <div>
          <div style="font-size:10px;color:var(--text-muted);margin-bottom:2px;">売価（USD）</div>
          <input type="text" inputmode="numeric" class="form-control" style="font-size:12px;" id="rv-pu-sp-${idx}"
            value="${formatPriceInput(l.salePrice||0)}" data-raw-value="${l.salePrice||0}"
            oninput="priceFormatHandler(this)" onblur="priceFormatHandler(this)">
        </div>
      </div>`;
    }).join('');

    return `
      <div class="revise-section-title"><i class="fa-solid fa-file-import"></i> 仕入伝票の内容を編集</div>
      <div class="revise-grid-2">
        <div class="form-group">
          <label class="form-label">仕入日</label>
          <input type="date" class="form-control" id="rv-purchaseDate" value="${rec.date||''}">
        </div>
        <div class="form-group">
          <label class="form-label">仕入先</label>
          <select class="form-control" id="rv-supplier">
            <option value="">-- 選択 --</option>
            ${APP_DATA.suppliers.map(s=>`<option value="${s.code}"${rec.supplier===s.code?' selected':''}>${s.code} — ${s.name}</option>`).join('')}
          </select>
        </div>
        <div class="form-group">
          <label class="form-label">担当者</label>
          <select class="form-control" id="rv-staff">
            <option value="">-- 選択 --</option>
            ${APP_DATA.staff.map(s=>`<option${rec.staff===s?' selected':''}>${s}</option>`).join('')}
          </select>
        </div>
        <div class="form-group">
          <label class="form-label">備考</label>
          <input class="form-control" id="rv-pu-note" value="${rec.note||''}">
        </div>
      </div>
      ${(rec.lines||[]).length > 0 ? `
      <div class="revise-section-subtitle" style="margin-top:12px;"><i class="fa-solid fa-list-check"></i> 明細編集（SKU・金額）</div>
      ${linesHtml}` : ''}`;

  } else if (type === 'shipping') {
    const itemsHtml = (rec.items||[]).map((it, idx) => `
      <div class="revise-item-row" id="rv-ship-item-${idx}">
        <span class="revise-item-num">${idx+1}</span>
        <input class="form-control" style="flex:1.2;font-size:12px;" placeholder="商品コード"
          id="rv-ship-code-${idx}" value="${it.code||''}">
        <input class="form-control" style="flex:1;font-size:12px;" placeholder="ブランド"
          id="rv-ship-brand-${idx}" value="${it.brand||''}">
        <input class="form-control" style="flex:1;font-size:12px;" placeholder="モデル"
          id="rv-ship-model-${idx}" value="${it.model||''}">
        <input type="text" inputmode="numeric" class="form-control" style="width:110px;font-size:12px;" placeholder="卸値（USD）"
          id="rv-ship-wholesale-${idx}" value="${formatPriceInput(it.wholesale||0)}" data-raw-value="${it.wholesale||0}"
          oninput="priceFormatHandler(this)" onblur="priceFormatHandler(this)">
      </div>`).join('');
    return `
      <div class="revise-section-title"><i class="fa-solid fa-truck"></i> 出荷伝票の内容を編集</div>
      <div class="revise-grid-2">
        <div class="form-group">
          <label class="form-label">出荷日</label>
          <input type="date" class="form-control" id="rv-shipDate" value="${rec.date||''}">
        </div>
        <div class="form-group">
          <label class="form-label">出荷先</label>
          <select class="form-control" id="rv-destination">
            <option value="">-- 選択 --</option>
            ${APP_DATA.buyers.map(b=>`<option value="${b.code}"${rec.destination===b.code?' selected':''}>${b.code} — ${b.name}</option>`).join('')}
          </select>
        </div>
      </div>
      <div class="revise-section-subtitle"><i class="fa-solid fa-list-check"></i> 商品明細</div>
      <div class="revise-item-header">
        <span style="width:20px;"></span>
        <span style="flex:1.2;font-size:11px;color:var(--text-muted);">商品コード</span>
        <span style="flex:1;font-size:11px;color:var(--text-muted);">ブランド</span>
        <span style="flex:1;font-size:11px;color:var(--text-muted);">モデル</span>
        <span style="width:110px;font-size:11px;color:var(--text-muted);">卸値（USD）</span>
      </div>
      <div id="rv-ship-items">${itemsHtml}</div>
      <div class="form-group" style="margin-top:10px;">
        <label class="form-label">備考</label>
        <textarea class="form-control" id="rv-note" rows="2">${rec.note||''}</textarea>
      </div>`;

  } else { // sales
    const itemsHtml = (rec.items||[]).map((it, idx) => `
      <div class="revise-item-row" id="rv-sale-item-${idx}">
        <span class="revise-item-num">${idx+1}</span>
        <input class="form-control" style="flex:1.2;font-size:12px;" placeholder="商品コード"
          id="rv-sale-code-${idx}" value="${it.code||''}">
        <input class="form-control" style="flex:1;font-size:12px;" placeholder="ブランド"
          id="rv-sale-brand-${idx}" value="${it.brand||''}">
        <input class="form-control" style="flex:1;font-size:12px;" placeholder="モデル"
          id="rv-sale-model-${idx}" value="${it.model||''}">
        <input type="text" inputmode="numeric" class="form-control" style="width:110px;font-size:12px;" placeholder="販売金額（USD）"
          id="rv-sale-price-${idx}" value="${formatPriceInput(it.salePrice||0)}" data-raw-value="${it.salePrice||0}"
          oninput="priceFormatHandler(this)" onblur="priceFormatHandler(this)">
      </div>`).join('');
    return `
      <div class="revise-section-title"><i class="fa-solid fa-yen-sign"></i> 売上伝票の内容を編集</div>
      <div class="revise-grid-2">
        <div class="form-group">
          <label class="form-label">売上日</label>
          <input type="date" class="form-control" id="rv-saleDate" value="${rec.date||''}">
        </div>
        <div class="form-group">
          <label class="form-label">販売先</label>
          <select class="form-control" id="rv-buyer">
            <option value="">-- 選択 --</option>
            ${APP_DATA.buyers.map(b=>`<option value="${b.code}"${rec.buyer===b.code?' selected':''}>${b.code} — ${b.name}</option>`).join('')}
          </select>
        </div>
      </div>
      <div class="revise-section-subtitle"><i class="fa-solid fa-list-check"></i> 商品明細</div>
      <div class="revise-item-header">
        <span style="width:20px;"></span>
        <span style="flex:1.2;font-size:11px;color:var(--text-muted);">商品コード</span>
        <span style="flex:1;font-size:11px;color:var(--text-muted);">ブランド</span>
        <span style="flex:1;font-size:11px;color:var(--text-muted);">モデル</span>
        <span style="width:110px;font-size:11px;color:var(--text-muted);">販売金額（USD）</span>
      </div>
      <div id="rv-sale-items">${itemsHtml}</div>
      <div class="form-group" style="margin-top:10px;">
        <label class="form-label">備考</label>
        <textarea class="form-control" id="rv-note" rows="2">${rec.note||''}</textarea>
      </div>`;
  }
}

// ── val() ヘルパー（取得を簡略化） ──
function _rv(id) {
  const el = document.getElementById(id);
  return el ? el.value : null;
}

// ── 差分テキストを生成 ──────────────────────────────
function buildDiffNote(diffs) {
  if (!diffs.length) return '（変更なし）';
  return diffs.map(d => `[${d.field}] ${d.before} → ${d.after}`).join(' / ');
}

function closeSlipRevise() {
  document.getElementById('slipReviseOverlay')?.classList.add('hidden');
}

function submitSlipRevise() {
  const id   = document.getElementById('slipReviseId').value;
  const type = document.getElementById('slipReviseType').value;
  const memo = document.getElementById('slipReviseNote').value.trim();

  let record = null;
  if (type === 'purchase')      record = (APP_DATA.purchaseSlips || []).find(s => s.id === id);
  else if (type === 'shipping') record = APP_DATA.shipments.find(s => s.id === id);
  else                          record = APP_DATA.sales.find(s => s.id === id);
  if (!record) { showToast('error', 'エラー', '対象伝票が見つかりません'); return; }
  const recordBefore = _approvalClone(record);

  // ── フィールドごとに差分を収集 & 値を更新 ──────────
  const diffs = [];

  const chk = (fieldName, oldVal, newVal, setter) => {
    const o = String(oldVal ?? '');
    const n = String(newVal ?? '');
    if (o !== n) {
      diffs.push({ field: fieldName, before: o || '（空）', after: n || '（空）' });
      setter(n);
    }
  };

  if (type === 'purchase') {
    chk('仕入日',  record.date,     _rv('rv-purchaseDate'),  v => { record.date     = v; });
    chk('仕入先',  getSupplierName(record.supplier), getSupplierName(_rv('rv-supplier')), () => { record.supplier = _rv('rv-supplier'); });
    chk('担当者',  record.staff,    _rv('rv-staff'),         v => { record.staff    = v; });
    const noteVal = _rv('rv-pu-note');
    if (noteVal !== null) chk('備考', record.note||'', noteVal, v => { record.note = v; });
    // 明細（SKU・金額）
    (record.lines || []).forEach((l, idx) => {
      chk(`明細${l.lineNo} SKU`,   l.sku,           _rv(`rv-pu-sku-${idx}`) ?? l.sku,  v => { l.sku           = v; });
      const newPP = getPriceValue(document.getElementById(`rv-pu-pp-${idx}`)) || l.purchasePrice;
      chk(`明細${l.lineNo} 仕入金額`, l.purchasePrice, newPP, v => { l.purchasePrice = parseInt(v)||0; });
      const newSP = getPriceValue(document.getElementById(`rv-pu-sp-${idx}`)) || l.salePrice;
      chk(`明細${l.lineNo} 売価（USD）`, l.salePrice, newSP, v => { l.salePrice = parseInt(v)||0; });
    });

  } else if (type === 'shipping') {
    chk('出荷日',   record.date,        _rv('rv-shipDate'),     v => { record.date        = v; });
    chk('出荷先',   getBuyerName(record.destination), getBuyerName(_rv('rv-destination')),
      () => { record.destination = _rv('rv-destination'); });
    chk('備考',     record.note,        _rv('rv-note'),         v => { record.note        = v; });
    // 明細を更新
    let newTotal = 0;
    (record.items||[]).forEach((it, idx) => {
      chk(`明細${idx+1}コード`,  it.code,      _rv(`rv-ship-code-${idx}`),      v => { it.code      = v; });
      chk(`明細${idx+1}ブランド`, it.brand,    _rv(`rv-ship-brand-${idx}`),     v => { it.brand     = v; });
      chk(`明細${idx+1}モデル`,  it.model,     _rv(`rv-ship-model-${idx}`),     v => { it.model     = v; });
      const newW = getPriceValue(document.getElementById(`rv-ship-wholesale-${idx}`));
      chk(`明細${idx+1}卸値`,    it.wholesale, newW,                             v => { it.wholesale = parseInt(v)||0; });
      newTotal += getPriceValue(document.getElementById(`rv-ship-wholesale-${idx}`));
    });
    if (record.total !== newTotal) {
      diffs.push({ field: '合計卸値（USD）', before: formatSalePrice(record.total), after: formatSalePrice(newTotal) });
      record.total = newTotal;
    }

  } else { // sales
    chk('売上日',   record.date,  _rv('rv-saleDate'),  v => { record.date  = v; });
    chk('販売先',   getBuyerName(record.buyer), getBuyerName(_rv('rv-buyer')),
      () => { record.buyer = _rv('rv-buyer'); });
    chk('備考',     record.note,  _rv('rv-note'),       v => { record.note  = v; });
    // 明細を更新
    let newTotal = 0;
    (record.items||[]).forEach((it, idx) => {
      chk(`明細${idx+1}コード`,  it.code,     _rv(`rv-sale-code-${idx}`),   v => { it.code     = v; });
      chk(`明細${idx+1}ブランド`, it.brand,   _rv(`rv-sale-brand-${idx}`),  v => { it.brand    = v; });
      chk(`明細${idx+1}モデル`,  it.model,    _rv(`rv-sale-model-${idx}`),  v => { it.model    = v; });
      const newP = getPriceValue(document.getElementById(`rv-sale-price-${idx}`));
      chk(`明細${idx+1}販売金額（USD）`, it.salePrice, newP, v => { it.salePrice = parseInt(v)||0; });
      newTotal += getPriceValue(document.getElementById(`rv-sale-price-${idx}`));
    });
    if (record.total !== newTotal) {
      diffs.push({ field: '合計金額', before: formatPrice(record.total), after: formatPrice(newTotal) });
      record.total = newTotal;
    }
  }

  // 変更なし & メモも空なら何もしない
  if (diffs.length === 0 && !memo) {
    showToast('info', '変更なし', '内容に変更がありません');
    return;
  }

  // 修正履歴エントリを生成（差分を自動テキスト化）
  const diffText  = buildDiffNote(diffs);
  const finalNote = memo ? `${diffText}　※${memo}` : diffText;

  const revEntry = {
    revisedAt:    new Date().toLocaleString('ja-JP'),
    buyerName:    currentUser()?.name || '—',
    approverName: isAdmin() ? (currentUser()?.name || '管理者') : '（承認待ち）',
    note: finalNote,
  };

  const execRevise = () => {
    if (!record.revisions) record.revisions = [];
    if (isAdmin()) revEntry.approverName = currentUser()?.name || '管理者';
    record.revisions.push(revEntry);
    applyBusinessRecordState(type, record);
    closeSlipRevise();
    refreshLinkedBusinessViews({ source: `${type}-slip-revision` });
    const typeJa = type === 'purchase' ? '仕入' : type === 'shipping' ? '出荷' : '売上';
    showToast('success', '伝票修正を記録しました', `${typeJa}伝票 ${id} / ${diffs.length}項目変更`);
    // 詳細モーダルが開いていれば再描画
    const detailOverlay = document.getElementById('slipDetailOverlay');
    if (detailOverlay && !detailOverlay.classList.contains('hidden')) {
      openSlipDetail(type, id);
    }
  };

  if (typeof isBuyer === 'function' && isBuyer()) {
    const typeLabel = { purchase:'仕入伝票修正', shipping:'出荷伝票修正', sales:'売上伝票修正' }[type];
    if (typeof requestApproval === 'function') {
      const recordAfter = _approvalClone(record);
      if (!Array.isArray(recordAfter.revisions)) recordAfter.revisions = [];
      recordAfter.revisions.push({ ...revEntry });
      Object.keys(record).forEach(key => { delete record[key]; });
      Object.assign(record, _approvalClone(recordBefore));
      requestApproval(type + '_revision', typeLabel, {
        id, note: finalNote, before: recordBefore, after: recordAfter, changes: _approvalClone(diffs),
      }, '', null);
    }
    closeSlipRevise();
    showToast('info', '修正申請を送信しました', '管理者が確認後に処理されます');
    refreshLinkedBusinessViews({ source: `${type}-revision-request` });
  } else {
    execRevise();
  }
}

// 旧関数の互換エイリアス（saveSalesから呼び出されるケース対応）
function initSalesListFilter() { initSlipList(); }
function filterSalesList()     { refreshSlipList(); }
function clearSalesFilter()    { clearSlipFilter(); }
function exportSalesCSV()      { exportSlipCSV(); }

// =====================================================
// QRコード / バーコードスキャン機能 (jsQR + getUserMedia)
// USB/Bluetoothスキャナーは Enter 確定で同一ロジック処理
// =====================================================
let _barcodeMode     = null;  // 'pr' | 'sr' | 'stocktake' | 'inventory-search' | 'product-registration'
let _barcodeStream   = null;  // MediaStream
let _barcodeAnimId   = null;  // requestAnimationFrame ID
let _barcodeScanCount = 0;
let _barcodeLastCode  = '';
let _barcodeCooldown  = false; // 連続読み取り防止（同一コードの連打防止）

/**
 * スキャナーモーダルを開く
 * mode: 'pr' = 仕入返品、'sr' = 売上返品、'stocktake' = 棚卸、
 *       'inventory-search' = 在庫一覧、'product-registration' = 商品登録
 */
function openBarcodeScanner(mode) {
  _barcodeMode      = mode;
  _barcodeScanCount = 0;
  _barcodeLastCode  = '';
  _barcodeCooldown  = false;

  const modal = document.getElementById('barcodeScanModal');
  const title = document.getElementById('barcodeScanModalTitle');
  if (!modal) return;
  const titleMap = {
    pr: '仕入返品 QRコード / バーコードスキャン',
    sr: '売上返品 QRコード / バーコードスキャン',
    stocktake: '棚卸用QRコードを読み取る',
    'inventory-search': '在庫一覧の管理番号をQRコードから読み取る',
    'product-registration': '商品登録の管理番号をQRコードから読み取る',
  };
  title.textContent = titleMap[mode] || 'QRコード / バーコードスキャン';
  document.getElementById('barcodeScanCount').textContent = '0';
  document.getElementById('barcodeLastResult').style.display = 'none';
  document.getElementById('barcodeScanStatus').textContent  = 'カメラを起動中...';
  document.getElementById('barcodeScanStatus').style.color  = 'var(--text-muted)';
  modal.classList.remove('hidden');

  _startCamera();
}

/** カメラ起動 */
async function _startCamera() {
  const video  = document.getElementById('barcodeVideo');
  const status = document.getElementById('barcodeScanStatus');
  if (!video) return;

  // jsQR はローカル同梱。古いキャッシュなどで未読込の場合だけ再読込する。
  if (typeof jsQR === 'undefined') {
    await _loadScript('js/jsQR.js?v=1.4.0');
  }

  try {
    const constraints = {
      video: {
        facingMode: { ideal: 'environment' }, // 背面カメラ優先
        width:  { ideal: 1280 },
        height: { ideal: 720  },
      }
    };
    const stream = await navigator.mediaDevices.getUserMedia(constraints);
    _barcodeStream = stream;
    video.srcObject = stream;
    await video.play();
    status.textContent = '読み取り中... QRコードを枠内に合わせてください';
    status.style.color  = '#0ea5e9';
    _barcodeAnimId = requestAnimationFrame(_scanFrame);
  } catch (err) {
    console.error('Camera error:', err);
    const msg = err.name === 'NotAllowedError'
      ? 'カメラへのアクセスが拒否されました。ブラウザの設定を確認してください。'
      : `カメラを起動できませんでした: ${err.message}`;
    status.textContent = msg;
    status.style.color  = '#dc2626';
  }
}

/** 動的スクリプトロード */
function _loadScript(src) {
  return new Promise((resolve, reject) => {
    const el = document.createElement('script');
    el.src = src;
    el.onload  = resolve;
    el.onerror = reject;
    document.head.appendChild(el);
  });
}

/** 毎フレーム QRコード解析 */
function _scanFrame() {
  const video  = document.getElementById('barcodeVideo');
  const canvas = document.getElementById('barcodeCanvas');
  const modal  = document.getElementById('barcodeScanModal');

  // モーダルが閉じられていたらスキャン停止
  if (!modal || modal.classList.contains('hidden')) {
    _stopCamera();
    return;
  }
  if (!video || video.readyState !== 4) {
    _barcodeAnimId = requestAnimationFrame(_scanFrame);
    return;
  }

  canvas.width  = video.videoWidth;
  canvas.height = video.videoHeight;
  const ctx = canvas.getContext('2d');
  ctx.drawImage(video, 0, 0, canvas.width, canvas.height);

  const imageData = ctx.getImageData(0, 0, canvas.width, canvas.height);

  if (typeof jsQR !== 'undefined') {
    const code = jsQR(imageData.data, imageData.width, imageData.height, {
      inversionAttempts: 'dontInvert',
    });
    if (code && code.data) {
      _onBarcodeDetected(code.data);
    }
  }

  _barcodeAnimId = requestAnimationFrame(_scanFrame);
}

/** バーコード検出時の処理 */
function _onBarcodeDetected(rawCode) {
  // クールダウン中は無視（同じコードの連打防止：1.2秒）
  if (_barcodeCooldown) return;
  // 直前と同じコードは0.8秒無視
  if (rawCode === _barcodeLastCode) return;

  _barcodeCooldown = true;
  _barcodeLastCode = rawCode;
  setTimeout(() => { _barcodeCooldown = false; }, 1200);

  const code   = rawCode.trim();
  const status = document.getElementById('barcodeScanStatus');
  const lastEl = document.getElementById('barcodeLastResult');
  const codeEl = document.getElementById('barcodeLastCode');
  const countEl= document.getElementById('barcodeScanCount');

  // 1件読取画面は、管理番号を所定の入力欄へ反映してカメラを閉じる。
  const singleEntryModes = {
    stocktake: {
      inputId: 'stkCodeInput',
      toastTitle: '棚卸用QRを読み取りました',
      toastMessage: item => `管理番号 ${item.code} を入力欄へ反映しました`,
    },
    'inventory-search': {
      inputId: 'inv-f-code',
      toastTitle: '在庫の管理番号を読み取りました',
      toastMessage: item => `管理番号 ${item.code} で在庫一覧を検索しました`,
    },
    'product-registration': {
      inputId: 'pu-code',
      toastTitle: '商品の管理番号を読み取りました',
      toastMessage: item => `管理番号 ${item.code} を反映し、既存在庫を照合しました`,
    },
  };
  const singleEntryConfig = singleEntryModes[_barcodeMode];
  if (singleEntryConfig) {
    const item = (APP_DATA.inventory || []).find(candidate => candidate.code === code);
    if (!item) {
      if (status) {
        status.textContent = `「${code}」は在庫一覧に登録されていません。`;
        status.style.color = '#f59e0b';
      }
      setTimeout(() => {
        if (_barcodeLastCode === rawCode) _barcodeLastCode = '';
      }, 1200);
      return;
    }

    const input = document.getElementById(singleEntryConfig.inputId);
    if (input) input.value = item.code;
    if (_barcodeMode === 'inventory-search' && typeof execInventorySearch === 'function') {
      execInventorySearch();
    } else if (_barcodeMode === 'product-registration' && typeof puCodeSearch === 'function') {
      if (_puManagementNumberLookupTimer) clearTimeout(_puManagementNumberLookupTimer);
      puCodeSearch(item.code);
    }
    _barcodeScanCount = 1;
    if (countEl) countEl.textContent = '1';
    if (codeEl) codeEl.textContent = item.code;
    if (lastEl) lastEl.style.display = '';
    if (status) {
      status.textContent = `✓ 読み取り成功: ${item.code}`;
      status.style.color = '#16a34a';
    }
    setTimeout(() => {
      closeBarcodeScanner();
      if (input && !input.disabled) {
        input.focus();
        input.select();
      }
      if (typeof showToast === 'function') {
        showToast('success', singleEntryConfig.toastTitle, singleEntryConfig.toastMessage(item));
      }
    }, 450);
    return;
  }

  // 返品の商品追加処理（モード別）
  let added = false;
  if (_barcodeMode === 'pr') {
    const before = _prAddedItems.length;
    prAddItemByCode(code);
    added = _prAddedItems.length > before;
  } else if (_barcodeMode === 'sr') {
    const before = _srAddedItems.length;
    srAddItemByCode(code);
    added = _srAddedItems.length > before;
  }

  if (added) {
    _barcodeScanCount++;
    if (countEl) countEl.textContent = String(_barcodeScanCount);
    if (codeEl)  codeEl.textContent  = code;
    if (lastEl)  { lastEl.style.display = ''; }
    if (status)  {
      status.textContent = `✓ 読み取り成功: ${code}`;
      status.style.color  = '#16a34a';
      setTimeout(() => {
        if (status) {
          status.textContent = '読み取り中... 次のQRコードを合わせてください';
          status.style.color  = '#0ea5e9';
        }
      }, 1500);
    }
  } else {
    if (status) {
      status.textContent = `「${code}」は登録されていないか重複しています。再スキャンしてください。`;
      status.style.color  = '#f59e0b';
      setTimeout(() => {
        if (status) {
          status.textContent = '読み取り中... QRコードを枠内に合わせてください';
          status.style.color  = '#0ea5e9';
        }
      }, 2000);
    }
  }
}

/** カメラ停止 */
function _stopCamera() {
  if (_barcodeAnimId) { cancelAnimationFrame(_barcodeAnimId); _barcodeAnimId = null; }
  if (_barcodeStream) {
    _barcodeStream.getTracks().forEach(t => t.stop());
    _barcodeStream = null;
  }
  const video = document.getElementById('barcodeVideo');
  if (video) { video.srcObject = null; }
}

/** スキャンモーダルを閉じる */
function closeBarcodeScanner() {
  _stopCamera();
  const modal = document.getElementById('barcodeScanModal');
  if (modal) modal.classList.add('hidden');
  document.getElementById('barcodeScanStatus').textContent = 'カメラを起動中...';
  document.getElementById('barcodeScanStatus').style.color = 'var(--text-muted)';
  document.getElementById('barcodeLastResult').style.display = 'none';
  _barcodeMode = null;
}


function addSalesLine() {
  salesLineCount++;
  const tbody = document.getElementById('salesLines');
  if (!tbody) return;
  tbody.insertAdjacentHTML('beforeend', buildSalesLineHTML(salesLineCount, '', '', '', '', null));
}

function autoFillItem(input, lineId, type) {
  const code   = input.value.trim();
  const item   = APP_DATA.inventory.find(i => i.code === code && i.status === '在庫中');
  const prefix = type === 'sales' ? 'sl' : 'sh';

  if (item) {
    // ブランド・モデル
    const setCell = (id, val, muted) => {
      const el = document.getElementById(id);
      if (!el) return;
      el.textContent = val || '—';
      el.style.color = (val && !muted) ? 'var(--text)' : 'var(--text-muted)';
    };
    setCell(`${prefix}-brand-${lineId}`, item.brand);
    setCell(`${prefix}-model-${lineId}`, item.model);
    // 売上登録のみ：型番・シリアル・付属品
    if (type === 'sales') {
      setCell(`sl-ref-${lineId}`,    item.ref    || '—', true);
      setCell(`sl-serial-${lineId}`, item.serial || '—', true);
      const accsText = Array.isArray(item.accessories) && item.accessories.length
        ? item.accessories.join('、') : '—';
      setCell(`sl-accs-${lineId}`, accsText, true);
    }
    input.style.borderColor = 'var(--success)';
  } else if (code) {
    const setErr = (id, val) => {
      const el = document.getElementById(id);
      if (el) { el.textContent = val; el.style.color = 'var(--danger)'; }
    };
    setErr(`${prefix}-brand-${lineId}`, '未登録または在庫なし');
    setErr(`${prefix}-model-${lineId}`, '');
    if (type === 'sales') {
      ['ref','serial','accs'].forEach(f => {
        const el = document.getElementById(`sl-${f}-${lineId}`);
        if (el) { el.textContent = '—'; el.style.color = 'var(--text-muted)'; }
      });
    }
    input.style.borderColor = 'var(--danger)';
  } else {
    const reset = (id) => {
      const el = document.getElementById(id);
      if (el) { el.textContent = '—'; el.style.color = 'var(--text-muted)'; }
    };
    reset(`${prefix}-brand-${lineId}`);
    reset(`${prefix}-model-${lineId}`);
    if (type === 'sales') {
      ['ref','serial','accs'].forEach(f => reset(`sl-${f}-${lineId}`));
    }
    input.style.borderColor = '';
  }
}

function removeLine(btn, type) {
  // 出荷明細行削除（.slip-line div 対応）
  const line = btn.closest('.slip-line') || btn.closest('tr');
  if (!line) return;
  line.remove();
  if (type === 'sales') calcSalesTotal();
  else calcShippingTotal();
}

// 売上伝票テーブル行削除
function removeSalesLine(btn) {
  const row = btn.closest('tr');
  if (!row) return;
  // 1行以上残す（最後の行は消せない）
  const tbody = document.getElementById('salesLines');
  if (tbody && tbody.querySelectorAll('tr[data-line-id]').length <= 1) {
    showToast('info', '削除不可', '少なくとも1行は必要です');
    return;
  }
  row.remove();
  calcSalesTotal();
}

function calcSalesTotal() {
  let grossTotal   = 0;
  let excludeTotal = 0;
  const takebackItems = [];

  document.querySelectorAll('#salesLines tr[data-line-id]').forEach(row => {
    const lineId   = row.dataset.lineId;
    const priceInput = document.getElementById(`sl-price-${lineId}`);
    const price    = getSalesLinePriceUSD(priceInput);
    const chk      = document.getElementById(`sl-chk-${lineId}`);
    const included = chk ? chk.checked : true;

    grossTotal += price;
    if (!included) {
      excludeTotal += price;
      const code  = document.getElementById(`sl-code-${lineId}`)?.value?.trim() || '';
      const brand = document.getElementById(`sl-brand-${lineId}`)?.textContent?.trim() || '';
      const model = document.getElementById(`sl-model-${lineId}`)?.textContent?.trim() || '';
      takebackItems.push({ code, brand, model, price });
    }

    row.classList.toggle('sl-row-excluded', !included);
  });

  const netTotal  = grossTotal - excludeTotal;
  const taxFree   = isTaxFreeMode();
  const taxRate   = 0.10;
  const taxAmount = taxFree ? 0 : Math.floor(netTotal * taxRate);
  const grandTotal = netTotal + taxAmount;

  // 税抜合計
  const displayEl = document.getElementById('salesTotalDisplay');
  if (displayEl) {
    if (excludeTotal > 0) {
      displayEl.innerHTML =
        `<span style="text-decoration:line-through;color:var(--text-muted);font-size:13px;">${formatSalesDisplayAmount(grossTotal)}</span>` +
        `<span style="margin:0 6px;color:#dc2626;font-size:12px;">− ${formatSalesDisplayAmount(excludeTotal)}</span>` +
        `<span style="color:var(--primary);font-weight:bold;">${formatSalesDisplayAmount(netTotal)}</span>`;
    } else {
      displayEl.textContent = formatSalesDisplayAmount(netTotal);
    }
  }

  // 消費税・税込合計
  const taxEl   = document.getElementById('salesTotalTax');
  const grandEl = document.getElementById('salesTotalGrand');
  const taxRow  = document.getElementById('slipTaxRow');
  const grandRow = document.getElementById('slipGrandRow');

  if (taxFree) {
    if (taxRow)  taxRow.style.display  = 'none';
    if (grandRow) grandRow.style.display = 'none';
  } else {
    if (taxRow)  taxRow.style.display  = '';
    if (grandRow) grandRow.style.display = '';
    if (taxEl)   taxEl.textContent  = formatSalesDisplayAmount(taxAmount);
    if (grandEl) grandEl.textContent = formatSalesDisplayAmount(grandTotal);
  }

  const fxReference = document.getElementById('salesTotalFxReference');
  if (fxReference) {
    const rate = getSalesUsdRate();
    const referenceBase = taxFree ? netTotal : grandTotal;
    const rateText = rate.toLocaleString('ja-JP', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
    fxReference.textContent = _salesEntryCurrency === 'JPY'
      ? `参考USD換算: ${formatSalePrice(referenceBase)}（1 USD = ¥${rateText}）`
      : `参考円換算: ${formatPrice(Math.round(referenceBase * rate))}（1 USD = ¥${rateText}）`;
  }

  // 持ち帰りプレビューパネル更新
  updateTakebackPanel(takebackItems);
}

// 持ち帰りプレビューパネルを更新
function updateTakebackPanel(items) {
  const panel   = document.getElementById('slTakebackPanel');
  const countEl = document.getElementById('slTakebackCount');
  const listEl  = document.getElementById('slTakebackList');
  if (!panel || !countEl || !listEl) return;

  if (items.length === 0) {
    panel.classList.add('hidden');
    return;
  }

  panel.classList.remove('hidden');
  countEl.textContent = `${items.length}件`;
  listEl.innerHTML = items.map(it => {
    const label = [it.brand, it.model].filter(Boolean).join(' ') || it.code || '（未入力）';
    const priceStr = it.price ? ` — ${formatSalesDisplayAmount(it.price)}` : '';
    return `<div class="sl-takeback-item">
      <i class="fa-solid fa-rotate-left" style="font-size:10px;opacity:0.6;"></i>
      <span style="flex:1;">${label}</span>
      <span style="color:#c2410c;font-weight:600;">${priceStr}</span>
    </div>`;
  }).join('');
}

function unlockSalesEdit() {
  // 管理者はPW不要
}

// =====================================================
// 売上登録 — 追加機能（金額フォーマット / 免税 / 印刷）
// =====================================================

/** カンマ付き数値文字列 → 純粋な数値（内部保持用） */
function parseSalesPrice(val) {
  if (val === null || val === undefined) return 0;
  // 全角→半角変換してから数値以外を除去
  const half = String(val).replace(/[０-９]/g, c => String.fromCharCode(c.charCodeAt(0) - 0xFEE0));
  return parseInt(half.replace(/[^0-9]/g, ''), 10) || 0;
}

/** 数値 → カンマ区切り表示用文字列（¥なし） */
function formatPriceInput(num) {
  if (!num) return '';
  return Number(num).toLocaleString('ja-JP');
}

/** 金額入力欄 oninput: リアルタイムで3桁カンマ付与 */
function onSalesPriceInput(el) {
  const entryAmount = parseSalesPrice(el.value);
  const entryCurrency = el.dataset.entryCurrency || _salesEntryCurrency;
  const usdAmount = convertSalesEntryToUSD(entryAmount, entryCurrency);
  el.dataset.entryCurrency = entryCurrency;
  el.dataset.usdValue = usdAmount > 0 ? String(usdAmount) : '';
  const pos = el.selectionStart;
  const oldLen = el.value.length;
  el.value = entryAmount > 0 ? formatPriceInput(entryAmount) : '';
  // カーソル位置補正
  const newLen = el.value.length;
  try { el.setSelectionRange(pos + (newLen - oldLen), pos + (newLen - oldLen)); } catch(e) {}
  calcSalesTotal();
}

/** 金額入力欄 onblur: 最終整形 */
function onSalesPriceBlur(el) {
  const entryAmount = parseSalesPrice(el.value);
  const entryCurrency = el.dataset.entryCurrency || _salesEntryCurrency;
  const usdAmount = convertSalesEntryToUSD(entryAmount, entryCurrency);
  el.dataset.entryCurrency = entryCurrency;
  el.dataset.usdValue = usdAmount > 0 ? String(usdAmount) : '';
  el.value = entryAmount > 0 ? formatPriceInput(entryAmount) : '';
  calcSalesTotal();
}

/** 販売先エラーをクリア */
function clearSalesBuyerError() {
  const errEl = document.getElementById('sl-buyer-error');
  if (errEl) errEl.style.display = 'none';
}

// =====================================================
// 免税モード
// =====================================================

let _taxFreeMode = false;

function isTaxFreeMode() { return _taxFreeMode; }

function onTaxFreeToggle(checked) {
  _taxFreeMode = checked;
  const labelEl = document.getElementById('taxFreeLabel');
  if (labelEl) labelEl.textContent = checked ? '免税' : '通常';
  // ページ全体に免税クラスを付与（背景・透かし）
  document.body.classList.toggle('tax-free-mode', checked);
  // 税表示を再計算
  calcSalesTotal();
}

// =====================================================
// 印刷プレビュー
// =====================================================

/** 会社情報マスタを正規化し、全画面・帳票で同じオブジェクトを参照する。 */
function getCompanyInfo() {
  if (!APP_DATA.companyInfo || typeof APP_DATA.companyInfo !== 'object') APP_DATA.companyInfo = {};
  const ci = APP_DATA.companyInfo;

  // 旧画面で保存されたフィールドは一度だけ正式名称へ移行する。
  if (!ci.accountNumber && ci.accountNo) ci.accountNumber = ci.accountNo;
  if (!ci.accountHolder && ci.accountName) ci.accountHolder = ci.accountName;
  delete ci.accountNo;
  delete ci.accountName;
  return ci;
}

/** 会社情報マスタから帳票共通の自社情報を取得する。 */
function getSlipCompanyInfo() {
  const ci = getCompanyInfo();
  return {
    companyName: ci.companyName || '（会社名未設定）',
    zip: ci.zip || '',
    address: ci.address || '',
    tel: ci.tel || '',
    fax: ci.fax || '',
    email: ci.email || '',
    invoice: ci.invoice || '',
    bankName: ci.bankName || '',
    branchName: ci.branchName || '',
    accountType: ci.accountType || '普通',
    accountNumber: ci.accountNumber || '',
    accountHolder: ci.accountHolder || '',
  };
}

/** Excel雛形（表紙＋明細表）をWeb印刷用HTMLとして組み立てる */
function buildTemplateStyleSlipDocument(options) {
  const {
    title,
    slipId,
    transactionDate,
    transactionDateLabel,
    counterpartyLabel,
    counterparty,
    items = [],
    note = '',
    formatAmount = amount => formatPrice(amount),
    currencyLabel = 'JPY（円）',
    taxMode = 'none',
    includeBank = false,
    summaryMessage = '下記の通りご案内いたします。',
    amountCaption = '合計金額',
    currencyNote = '',
  } = options || {};

  const ci = getSlipCompanyInfo();
  const safeItems = items.length > 0 ? items : [{ no: 1, detail: '（明細が入力されていません）', amount: 0, code: '' }];
  const subtotal = safeItems.reduce((sum, item) => sum + (Number(item.amount) || 0), 0);
  const taxFree = taxMode === 'exempt';
  const taxable = taxMode === 'standard';
  const taxAmount = taxable ? Math.floor(subtotal * 0.1) : 0;
  const grandTotal = subtotal + taxAmount;
  const issuedDate = new Date().toLocaleDateString('ja-JP');
  const partyName = counterparty?.name || '（相手先未設定）';
  const partyAddress = counterparty?.address || '';
  const partyContact = counterparty?.contact || '';
  const partyInvoice = counterparty?.invoice || '';
  const taxLabel = taxFree ? '免税（0%）' : (taxable ? '消費税（10%）' : '税対象外');
  const itemTaxLabel = taxFree ? '免税（0%）' : (taxable ? '消費税（10%）' : '—');
  const chunkSize = 18;
  const chunks = [];
  for (let index = 0; index < safeItems.length; index += chunkSize) {
    chunks.push(safeItems.slice(index, index + chunkSize));
  }

  const companyBlock = `
    <div class="tpl-company-block">
      <strong>${_escHtml(ci.companyName)}</strong>
      ${ci.zip ? `<span>${_escHtml(ci.zip)}</span>` : ''}
      ${ci.address ? `<span>${_escHtml(ci.address)}</span>` : ''}
      ${ci.tel ? `<span>TEL：${_escHtml(ci.tel)}</span>` : ''}
      ${ci.fax ? `<span>FAX：${_escHtml(ci.fax)}</span>` : ''}
      ${ci.email ? `<span>${_escHtml(ci.email)}</span>` : ''}
      ${ci.invoice ? `<span>登録番号：${_escHtml(ci.invoice)}</span>` : ''}
    </div>`;

  const bankBlock = includeBank ? `
    <div class="tpl-bank-block">
      <div class="tpl-bank-title">お振込先</div>
      <div class="tpl-bank-grid">
        <span>銀行名</span><strong>${_escHtml(ci.bankName || '—')}</strong>
        <span>支店名</span><strong>${_escHtml(ci.branchName || '—')}</strong>
        <span>口座番号</span><strong>${_escHtml(ci.accountType)}　${_escHtml(ci.accountNumber || '—')}</strong>
        <span>口座名義</span><strong>${_escHtml(ci.accountHolder || '—')}</strong>
      </div>
    </div>` : '';

  const coverPage = `
    <section class="tpl-doc-page tpl-cover-page">
      <h1 class="tpl-document-title">${_escHtml(title)}</h1>
      <div class="tpl-date-block">
        <div><span>発行日</span><strong>${_escHtml(issuedDate)}</strong></div>
        <div><span>${_escHtml(transactionDateLabel || '取引日')}</span><strong>${_escHtml(transactionDate || '—')}</strong></div>
      </div>
      <div class="tpl-parties">
        <div class="tpl-counterparty">
          <div class="tpl-party-label">${_escHtml(counterpartyLabel || 'お取引先')}</div>
          <div class="tpl-party-name">${_escHtml(partyName)} <span>様</span></div>
          ${partyAddress ? `<div>${_escHtml(partyAddress)}</div>` : ''}
          ${partyContact ? `<div>TEL：${_escHtml(partyContact)}</div>` : ''}
          ${partyInvoice ? `<div>登録番号：${_escHtml(partyInvoice)}</div>` : ''}
        </div>
        ${companyBlock}
      </div>
      <div class="tpl-slip-number"><span>伝票No.</span><strong>${_escHtml(slipId || '（未設定）')}</strong></div>
      <p class="tpl-summary-message">${_escHtml(summaryMessage)}</p>
      <div class="tpl-cover-total">
        <span>${_escHtml(amountCaption)}</span>
        <strong>${formatAmount(grandTotal)}</strong>
      </div>
      <div class="tpl-tax-caption">
        税区分：${_escHtml(taxLabel)}${currencyNote ? `　${_escHtml(currencyNote)}` : ''}
      </div>
      ${bankBlock}
    </section>`;

  const detailPages = chunks.map((chunk, chunkIndex) => {
    const blankCount = Math.max(0, 12 - chunk.length);
    const rows = chunk.map((item, index) => {
      const amount = Number(item.amount) || 0;
      const rowTax = taxable ? Math.floor(amount * 0.1) : 0;
      return `
        <tr>
          <td class="tpl-cell-number">${item.no || chunkIndex * chunkSize + index + 1}</td>
          <td class="tpl-cell-detail">${_escHtml(item.detail || '—')}</td>
          <td class="tpl-cell-amount">${amount ? formatAmount(amount) : '—'}</td>
          <td class="tpl-cell-tax">${taxable ? formatAmount(rowTax) : _escHtml(itemTaxLabel)}</td>
          <td class="tpl-cell-code">${_escHtml(item.code || '—')}</td>
        </tr>`;
    }).join('') + Array.from({ length: blankCount }, () => `
        <tr class="tpl-blank-row"><td>&nbsp;</td><td></td><td></td><td></td><td></td></tr>`).join('');

    return `
      <section class="tpl-doc-page tpl-detail-page">
        <div class="tpl-detail-heading">
          <h2>明細表</h2>
          <div><span>伝票No.</span><strong>${_escHtml(slipId || '（未設定）')}</strong></div>
        </div>
        <div class="tpl-currency-note">表示通貨：${_escHtml(currencyLabel)}${currencyNote ? `　${_escHtml(currencyNote)}` : ''}</div>
        <table class="tpl-detail-table">
          <colgroup><col class="tpl-col-no"><col class="tpl-col-detail"><col class="tpl-col-amount"><col class="tpl-col-tax"><col class="tpl-col-code"></colgroup>
          <thead><tr><th>通し番号</th><th>摘要</th><th>金額</th><th>${taxFree ? '免税（0%）' : (taxable ? '消費税（10%）' : '税区分')}</th><th>弊社管理番号</th></tr></thead>
          <tbody>${rows}</tbody>
          <tfoot>
            <tr><td></td><td class="tpl-total-label">小計</td><td>${formatAmount(subtotal)}</td><td>${taxable ? formatAmount(taxAmount) : _escHtml(itemTaxLabel)}</td><td></td></tr>
            <tr class="tpl-grand-row"><td></td><td class="tpl-total-label">${taxFree ? '合計（免税）' : (taxable ? '合計（税込）' : '合計')}</td><td colspan="2">${formatAmount(grandTotal)}</td><td></td></tr>
          </tfoot>
        </table>
        <div class="tpl-note-block"><strong>備考</strong><span>${note ? _escHtml(note) : '—'}</span></div>
        <div class="tpl-detail-company">${_escHtml(ci.companyName)}　${ci.address ? _escHtml(ci.address) : ''}</div>
      </section>`;
  }).join('<div class="tpl-page-divider">── ページ区切り ──</div>');

  return `
    <style class="tpl-document-styles">
      .tpl-document-wrap{font-family:"Yu Gothic","Hiragino Sans","Meiryo",sans-serif;color:#222;line-height:1.5}
      .tpl-doc-page{box-sizing:border-box;width:100%;max-width:794px;min-height:1020px;margin:0 auto;background:#fff;padding:38px 44px;position:relative;box-shadow:0 2px 12px rgba(0,0,0,.14)}
      .tpl-document-title{text-align:center;font-size:28px;letter-spacing:.32em;margin:4px 0 38px;padding-left:.32em}
      .tpl-date-block{margin-left:auto;width:280px;font-size:12px}.tpl-date-block div{display:grid;grid-template-columns:80px 1fr;gap:10px;padding:4px 0;border-bottom:1px solid #ddd}.tpl-date-block span{color:#666}.tpl-date-block strong{font-weight:500}
      .tpl-parties{display:grid;grid-template-columns:1fr 1fr;gap:34px;align-items:start;margin:28px 0 42px;min-height:150px}.tpl-counterparty{font-size:12px}.tpl-party-label{font-size:10px;color:#777;margin-bottom:10px}.tpl-party-name{font-size:21px;border-bottom:1px solid #222;padding-bottom:7px;margin-bottom:8px}.tpl-party-name span{font-size:16px;margin-left:6px}.tpl-company-block{display:flex;flex-direction:column;font-size:11px;text-align:right}.tpl-company-block strong{font-size:15px;margin-bottom:6px}
      .tpl-slip-number{display:grid;grid-template-columns:110px 1fr;width:55%;margin-left:auto;border-bottom:3px double #333;padding:5px 4px;font-size:12px}.tpl-slip-number strong{text-align:right}
      .tpl-summary-message{margin:30px 0 8px;font-size:12px}.tpl-cover-total{display:grid;grid-template-columns:160px 1fr;align-items:center;width:78%;border-top:1px solid #333;border-bottom:3px double #333}.tpl-cover-total span{font-size:15px;font-weight:700;text-align:center;padding:12px}.tpl-cover-total strong{font-size:22px;text-align:right;padding:10px 18px}.tpl-tax-caption{font-size:10px;color:#666;margin-top:7px;width:78%;text-align:right}
      .tpl-bank-block{position:absolute;left:44px;right:44px;bottom:58px;border:1px solid #333;padding:14px 18px}.tpl-bank-title{font-weight:700;margin-bottom:8px}.tpl-bank-grid{display:grid;grid-template-columns:90px 1fr 90px 1fr;gap:5px 12px;font-size:11px}.tpl-bank-grid span{color:#666}
      .tpl-detail-heading{display:flex;justify-content:center;align-items:flex-end;position:relative;margin-bottom:6px}.tpl-detail-heading h2{font-size:19px;font-weight:500;letter-spacing:.2em;margin:0}.tpl-detail-heading div{position:absolute;right:0;bottom:0;display:flex;gap:12px;border-bottom:3px double #333;padding:3px 5px;font-size:11px}.tpl-currency-note{text-align:right;color:#666;font-size:9px;margin:4px 0 8px}
      .tpl-detail-table{width:100%;border-collapse:collapse;table-layout:fixed;font-size:10px}.tpl-detail-table th{background:#d7d7d7;border:1px solid #555;padding:6px 4px;text-align:center}.tpl-detail-table td{border:1px solid #666;padding:6px 7px;height:32px;vertical-align:middle}.tpl-col-no{width:10%}.tpl-col-detail{width:47%}.tpl-col-amount{width:17%}.tpl-col-tax{width:13%}.tpl-col-code{width:13%}.tpl-cell-number,.tpl-cell-amount,.tpl-cell-tax{text-align:right;font-variant-numeric:tabular-nums}.tpl-cell-detail{white-space:pre-line;line-height:1.35}.tpl-cell-code{text-align:center;font-size:9px;overflow-wrap:anywhere}.tpl-blank-row td{height:30px}.tpl-detail-table tfoot td{font-weight:600;text-align:right;background:#f6f6f6}.tpl-detail-table tfoot .tpl-total-label{background:#d7d7d7}.tpl-grand-row td{border-top:3px double #333}.tpl-note-block{display:grid;grid-template-columns:90px 1fr;min-height:78px;border:1px solid #666;border-top:0}.tpl-note-block strong{background:#d7d7d7;display:flex;align-items:center;justify-content:center}.tpl-note-block span{padding:10px;white-space:pre-line}.tpl-detail-company{text-align:right;font-size:9px;color:#777;margin-top:10px}.tpl-page-divider{text-align:center;color:#999;font-size:10px;padding:12px 0}
      @media(max-width:700px){.tpl-doc-page{min-height:0;padding:24px 18px}.tpl-parties{grid-template-columns:1fr;gap:16px}.tpl-company-block{text-align:left}.tpl-bank-block{position:static;margin-top:40px}.tpl-bank-grid{grid-template-columns:80px 1fr}.tpl-cover-total{width:100%}.tpl-detail-table{font-size:9px}.tpl-doc-page{overflow-x:auto}}
      @media print{@page{size:A4 portrait;margin:0}.tpl-document-wrap{background:#fff}.tpl-doc-page{width:210mm;min-height:297mm;max-width:none;padding:13mm 14mm;margin:0;box-shadow:none;break-after:page;page-break-after:always}.tpl-doc-page:last-child{break-after:auto;page-break-after:auto}.tpl-page-divider{display:none}.tpl-bank-block{left:14mm;right:14mm;bottom:16mm}}
    </style>
    <div class="tpl-document-wrap">${coverPage}<div class="tpl-page-divider">── ページ区切り ──</div>${detailPages}</div>`;
}

/** 雛形準拠の請求書を現在の売上フォームから生成する */
function _buildTemplateSalesSlipHTML() {
  const slipId = document.getElementById('sl-id')?.value?.trim() || '（未設定）';
  const slipDate = document.getElementById('sl-date')?.value || '—';
  const buyerCode = document.getElementById('sl-buyer')?.value || '';
  const buyer = APP_DATA.buyers.find(record => record.code === buyerCode) || { name: '（販売先未設定）' };
  const taxFree = isTaxFreeMode();
  const note = document.getElementById('sl-note')?.value?.trim() || '';
  const currencyLabel = _salesEntryCurrency === 'JPY' ? 'JPY（円）' : 'USD';
  const rateNote = `基準通貨：USD　換算レート：1 USD = ¥${getSalesUsdRate().toLocaleString('ja-JP', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
  const items = [];

  document.querySelectorAll('#salesLines tr[data-line-id]').forEach(row => {
    const lineId = row.dataset.lineId;
    if (!document.getElementById(`sl-chk-${lineId}`)?.checked) return;
    const code = document.getElementById(`sl-code-${lineId}`)?.value?.trim() || '';
    const inventoryItem = APP_DATA.inventory.find(item => item.code === code) || {};
    const brand = document.getElementById(`sl-brand-${lineId}`)?.textContent?.trim().replace(/^—$/, '') || inventoryItem.brand || '';
    const model = document.getElementById(`sl-model-${lineId}`)?.textContent?.trim().replace(/^—$/, '') || inventoryItem.model || '';
    const ref = document.getElementById(`sl-ref-${lineId}`)?.textContent?.trim().replace(/^—$/, '') || inventoryItem.ref || '';
    const serial = document.getElementById(`sl-serial-${lineId}`)?.textContent?.trim().replace(/^—$/, '') || inventoryItem.serial || '';
    const detail = [
      [brand, model].filter(Boolean).join(' / '),
      [ref && `型番: ${ref}`, serial && `シリアル: ${serial}`].filter(Boolean).join('　'),
      inventoryItem.accessories?.length ? `付属品: ${inventoryItem.accessories.join('・')}` : '',
      inventoryItem.note ? `備考: ${inventoryItem.note}` : '',
    ].filter(Boolean).join('\n') || code || '—';
    items.push({ no: items.length + 1, detail, amount: getSalesLinePriceUSD(document.getElementById(`sl-price-${lineId}`)), code });
  });

  return buildTemplateStyleSlipDocument({
    title: '請求書',
    slipId,
    transactionDate: slipDate,
    transactionDateLabel: '売上日',
    counterpartyLabel: 'ご請求先',
    counterparty: buyer,
    items,
    note,
    formatAmount: amount => formatSalesDisplayAmount(amount, _salesEntryCurrency),
    currencyLabel,
    taxMode: taxFree ? 'exempt' : 'standard',
    includeBank: true,
    summaryMessage: '商品代金として、下記金額をご請求申し上げます。',
    amountCaption: taxFree ? '合計金額（免税）' : '合計金額（税込）',
    currencyNote: rateNote,
  });
}

function openSalesPrintPreview() {
  const body = document.getElementById('salesPrintModalBody');
  if (!body) return;
  body.innerHTML = buildSalesSlip2PageHTML();
  document.getElementById('salesPrintModal').classList.remove('hidden');
}

function closeSalesPrintModal() {
  document.getElementById('salesPrintModal').classList.add('hidden');
}

function downloadCurrentSalesDocument() {
  const content = document.getElementById('salesPrintModalBody')?.innerHTML || '';
  const slipId = document.getElementById('sl-id')?.value?.trim() || 'sales-slip';
  _downloadTemplateDocument('請求書（売上伝票）', `${slipId}_請求書.html`, content);
}

function execSalesPrint() {
  // ① 既存クローンがあれば削除
  const OLD = document.getElementById('salesPrintCloneTarget');
  if (OLD) OLD.remove();

  // ② プレビュー内容を body 直下にクローン
  const src   = document.getElementById('salesPrintModalBody');
  const clone = document.createElement('div');
  clone.id    = 'salesPrintCloneTarget';
  clone.style.cssText = 'display:none;';
  clone.innerHTML = src.innerHTML;
  document.body.appendChild(clone);

  // ③ 印刷実行
  window.print();

  // ④ 印刷ダイアログが閉じたらクローンを削除
  setTimeout(() => {
    const t = document.getElementById('salesPrintCloneTarget');
    if (t) t.remove();
  }, 2500);
}

// ─────────────────────────────────────────────────────────
// 2ページ構成HTML生成
//   ページ1：売上明細書
//   ページ2：精算書
// ─────────────────────────────────────────────────────────
function buildSalesSlip2PageHTML() {
  return _buildTemplateSalesSlipHTML();
}

/** @deprecated 雛形変更前の帳票。比較参照用に保持 */
function _buildLegacySalesSlip2PageHTML() {
  /* ── フォームから現在値取得 ── */
  const slipId   = document.getElementById('sl-id')?.value?.trim()  || '（未設定）';
  const slipDate = document.getElementById('sl-date')?.value        || '—';
  const taxFree  = isTaxFreeMode();
  const note     = document.getElementById('sl-note')?.value?.trim() || '';

  // 販売先名称
  const buyerSel  = document.getElementById('sl-buyer');
  const buyerName = buyerSel
    ? (buyerSel.options[buyerSel.selectedIndex]?.text || '—')
    : '—';

  /* ── 明細行収集（計上チェック済み行のみ） ── */
  const lineItems = [];
  document.querySelectorAll('#salesLines tr[data-line-id]').forEach(row => {
    const lid = row.dataset.lineId;
    const chk = document.getElementById(`sl-chk-${lid}`);
    if (!chk?.checked) return;            // 持ち帰り行を除外

    const code  = document.getElementById(`sl-code-${lid}`)?.value?.trim()   || '';
    const brand = document.getElementById(`sl-brand-${lid}`)?.textContent?.trim().replace(/^—$/, '') || '';
    const model = document.getElementById(`sl-model-${lid}`)?.textContent?.trim().replace(/^—$/, '') || '';
    const ref   = document.getElementById(`sl-ref-${lid}`)?.textContent?.trim().replace(/^—$/, '')   || '';
    const serial= document.getElementById(`sl-serial-${lid}`)?.textContent?.trim().replace(/^—$/, '')|| '';
    const inv   = code ? APP_DATA.inventory.find(i => i.code === code) : null;
    const sku   = inv?.sku || '';

    // 商品明細テキスト：ブランド + モデル + 型番
    let detail = [brand, model].filter(Boolean).join(' ');
    if (ref)    detail += `（${ref}）`;
    if (!detail) detail = code || '—';

    const price = getSalesLinePriceUSD(document.getElementById(`sl-price-${lid}`));
    // 数量は常に1（時計業務は1点単位）
    const qty   = 1;
    const amount = price * qty;

    lineItems.push({ no: lineItems.length + 1, code, detail, sku, qty, price, amount });
  });

  /* ── 金額計算 ── */
  const subtotal   = lineItems.reduce((s, i) => s + i.amount, 0);
  const taxAmount  = taxFree ? 0 : Math.floor(subtotal * 0.1);
  const grandTotal = subtotal + taxAmount;
  const taxLabel   = taxFree ? '免税（0%）' : '消費税（10%）';
  const usdJpyRate = getSalesUsdRate();
  const printCurrency = _salesEntryCurrency;
  const printCurrencyLabel = printCurrency === 'JPY' ? 'JPY（円）' : 'USD';
  const formatPrintAmount = usdAmount => formatSalesDisplayAmount(usdAmount, printCurrency);
  const printCurrencyNote = `表示通貨: ${printCurrencyLabel}　基準通貨: USD　換算レート: 1 USD = ¥${usdJpyRate.toLocaleString('ja-JP', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;

  /* ── 会社情報 ── */
  const ci = getSlipCompanyInfo();

  /* ── 発行日 ── */
  const issuedDate = new Date().toLocaleDateString('ja-JP', {
    year: 'numeric', month: 'long', day: 'numeric'
  });

  /* ── 免税バッジ ── */
  const taxBadgeHTML = taxFree
    ? `<span style="display:inline-block;background:#ea580c;color:#fff;font-size:10px;font-weight:700;padding:2px 8px;border-radius:4px;letter-spacing:.1em;margin-left:8px;vertical-align:middle;">免税</span>`
    : '';

  /* ════════════════════════════════
     ページ1：売上明細書
  ════════════════════════════════ */
  const hasItems = lineItems.length > 0;
  const detailRows = hasItems
    ? lineItems.map(item => `
        <tr>
          <td class="sl-td-r">${item.no}</td>
          <td>${_escHtml(item.detail)}</td>
          <td style="font-size:11px;color:#6b7280;">${_escHtml(item.sku)}</td>
          <td class="sl-td-r">${item.qty}</td>
          <td class="sl-td-r">${item.price > 0 ? formatPrintAmount(item.price) : '—'}</td>
          <td class="sl-td-r">${item.amount > 0 ? formatPrintAmount(item.amount) : '—'}</td>
        </tr>`).join('')
    : `<tr><td colspan="6" style="text-align:center;padding:20px;color:#9ca3af;">（明細行が入力されていません）</td></tr>`;

  const page1 = `
    <div class="sl-doc-page">
      <!-- ヘッダー -->
      <div class="sl-page-header">
        <div>
          <div class="sl-page-title">売 上 明 細 書${taxBadgeHTML}</div>
          <div class="sl-page-title-sub">Sales Detail Statement</div>
        </div>
        <div class="sl-slip-no-block">
          <div class="sl-slip-no-label">伝票番号</div>
          <div class="sl-slip-no-val">${_escHtml(slipId)}</div>
          <div style="font-size:11px;color:#9ca3af;margin-top:2px;">発行日：${issuedDate}</div>
        </div>
      </div>

      <!-- メタ情報 -->
      <div class="sl-meta-grid">
        <div class="sl-meta-item">
          <span class="sl-meta-lbl">販売先</span>
          <span class="sl-meta-val">${_escHtml(buyerName)}</span>
        </div>
        <div class="sl-meta-item">
          <span class="sl-meta-lbl">売上日</span>
          <span class="sl-meta-val">${_escHtml(slipDate)}</span>
        </div>
        <div class="sl-meta-item">
          <span class="sl-meta-lbl">課税区分</span>
          <span class="sl-meta-val">${taxFree ? '<strong style="color:#ea580c;">免税</strong>' : '課税（10%）'}</span>
        </div>
        <div class="sl-meta-item">
          <span class="sl-meta-lbl">明細件数</span>
          <span class="sl-meta-val">${lineItems.length}件</span>
        </div>
      </div>
      <div style="margin:-4px 0 10px;text-align:right;font-size:10px;color:#6b7280;">
        ${printCurrencyNote}
      </div>

      <!-- 明細テーブル -->
      <table class="sl-detail-table">
        <colgroup>
          <col class="sl-col-no">
          <col class="sl-col-detail">
          <col class="sl-col-sku">
          <col class="sl-col-qty">
          <col class="sl-col-price">
          <col class="sl-col-amount">
        </colgroup>
        <thead>
          <tr>
            <th class="sl-th-r">No</th>
            <th>商品明細</th>
            <th>SKU</th>
            <th class="sl-th-r">数量</th>
            <th class="sl-th-r">単価（${printCurrencyLabel}）</th>
            <th class="sl-th-r">金額（${printCurrencyLabel}）</th>
          </tr>
        </thead>
        <tbody>${detailRows}</tbody>
      </table>

      <!-- 小計（ページ1右下） -->
      <div class="sl-p1-totals-wrap">
        <div class="sl-p1-totals">
          <div class="sl-p1-totals-row">
            <span style="color:#6b7280;font-size:11px;">小計（税抜）</span>
            <span style="font-weight:600;">${formatPrintAmount(subtotal)}</span>
          </div>
          ${taxFree ? '' : `<div class="sl-p1-totals-row">
            <span style="color:#6b7280;font-size:11px;">${taxLabel}</span>
            <span>${formatPrintAmount(taxAmount)}</span>
          </div>`}
          <div class="sl-p1-totals-row" style="background:#f0f7ff;">
            <span style="font-weight:700;color:#1e3a5f;">${taxFree ? '合計' : '税込合計'}</span>
            <span style="font-weight:700;color:#1e3a5f;font-size:14px;">${formatPrintAmount(grandTotal)}</span>
          </div>
        </div>
      </div>
    </div>`;

  /* ════════════════════════════════
     ページ2：精算書
  ════════════════════════════════ */
  const companyHTML = `
    <div class="sl-p2-company">
      <strong>${_escHtml(ci.companyName || '（会社名未設定）')}</strong>
      ${ci.zip     ? `${_escHtml(ci.zip)}<br>` : ''}
      ${ci.address ? `${_escHtml(ci.address)}<br>` : ''}
      ${ci.tel     ? `TEL：${_escHtml(ci.tel)}<br>` : ''}
      ${ci.fax     ? `FAX：${_escHtml(ci.fax)}<br>` : ''}
      ${ci.invoice ? `登録番号：${_escHtml(ci.invoice)}<br>` : ''}
      ${ci.email   ? _escHtml(ci.email)              : ''}
    </div>`;

  const page2 = `
    <div class="sl-doc-page">
      <!-- ヘッダー -->
      <div class="sl-page-header">
        <div>
          <div class="sl-page-title">精 算 書</div>
          <div class="sl-page-title-sub">Settlement Statement</div>
        </div>
        <div class="sl-slip-no-block">
          <div class="sl-slip-no-label">伝票番号</div>
          <div class="sl-slip-no-val">${_escHtml(slipId)}</div>
          <div style="font-size:11px;color:#9ca3af;margin-top:2px;">発行日：${issuedDate}</div>
        </div>
      </div>

      <!-- 販売先・売上日 -->
      <div class="sl-meta-grid" style="margin-bottom:8px;">
        <div class="sl-meta-item">
          <span class="sl-meta-lbl">販売先</span>
          <span class="sl-meta-val">${_escHtml(buyerName)}</span>
        </div>
        <div class="sl-meta-item">
          <span class="sl-meta-lbl">売上日</span>
          <span class="sl-meta-val">${_escHtml(slipDate)}</span>
        </div>
      </div>
      <div style="margin:-2px 0 10px;text-align:right;font-size:10px;color:#6b7280;">
        ${printCurrencyNote}
      </div>

      <!-- 金額セクション -->
      <div class="sl-p2-section-title">金額</div>
      <table class="sl-p2-amount-table">
        <tbody>
          <tr>
            <td style="color:#6b7280;width:200px;">小計（税抜）</td>
            <td>${formatPrintAmount(subtotal)}</td>
          </tr>
          <tr>
            <td style="color:#6b7280;">${taxLabel}</td>
            <td>${formatPrintAmount(taxAmount)}</td>
          </tr>
          <tr class="sl-row-grand">
            <td>${taxFree ? '合計金額' : '合計金額（税込）'}</td>
            <td>${formatPrintAmount(grandTotal)}</td>
          </tr>
        </tbody>
      </table>

      <!-- 備考欄 -->
      <div class="sl-p2-section-title">備考</div>
      <div class="sl-p2-note-box">
        ${note ? _escHtml(note) : '<span style="color:#d1d5db;">—</span>'}
      </div>

      <!-- 会社情報 -->
      ${companyHTML}

      <!-- 署名欄（ページ下部） -->
      <div class="sl-p2-sign-area">
        <div class="sl-p2-sign-title">署名・押印欄</div>
        <div class="sl-p2-sign-boxes">
          <div class="sl-p2-sign-box">販売先　確認印</div>
          <div class="sl-p2-sign-box">担当者　署名</div>
          <div class="sl-p2-sign-box">承認者　印</div>
        </div>
      </div>
    </div>`;

  /* ── 2ページをラップして返す ── */
  return `
    <div class="sl-doc-wrap">
      ${page1}
      <div class="sl-page-divider">── ページ区切り ──</div>
      ${page2}
    </div>`;
}

/** 数値を3桁区切りで返す（¥なし） */
function _fmtNum(n) {
  return Number(n).toLocaleString('ja-JP');
}

/** 数値を ¥xxx,xxx 形式で返す */
function _fmtYen(n) {
  if (n == null || isNaN(n)) return '—';
  return '¥' + Number(n).toLocaleString('ja-JP');
}

// ── 互換性のため旧関数も残す（他箇所から参照されている場合） ──
function _getSlipPrintCss() { return ''; }
function buildSlipPreviewHtml() { return buildSalesSlip2PageHTML(); }

function resetSalesForm() {
  _salesSourceShipmentId = null;
  _salesEntryCurrency = 'USD';
  salesLineCount = 0;
  const tbody = document.getElementById('salesLines');
  if (tbody) tbody.innerHTML = '';
  const idEl = document.getElementById('sl-id');
  if (idEl) idEl.value = '';
  const noteEl = document.getElementById('sl-note');
  if (noteEl) noteEl.value = '';
  // 販売先リセット
  const buyerEl = document.getElementById('sl-buyer');
  if (buyerEl) buyerEl.value = '';
  clearSalesBuyerError();
  setSlIdStatus('', '');
  addSalesLine();
  _syncSalesCurrencyUI();
  calcSalesTotal();
  showToast('info', 'リセット', 'フォームをリセットしました');
}

function canUseInventoryItemForSales(inventoryItem, sourceShipmentId = _salesSourceShipmentId) {
  if (!inventoryItem) return false;
  if (inventoryItem.status === '在庫中') return true;
  if (inventoryItem.status !== '出荷済' || !sourceShipmentId) return false;
  const sourceShipment = (APP_DATA.shipments || []).find(shipment => shipment.id === sourceShipmentId);
  return Boolean(sourceShipment?.items?.some(item => item.code === inventoryItem.code));
}

function clearInventoryReservationMetadata(inventoryItem) {
  if (!inventoryItem) return;
  delete inventoryItem.reservationRequestId;
  delete inventoryItem.reservedForGuestId;
  delete inventoryItem.reservedForGuestName;
  delete inventoryItem.reservedAt;
  delete inventoryItem.reservationApprovalId;
  delete inventoryItem.reservationPreviousStatus;
}

async function saveSales() {
  // ① 販売先バリデーション
  const buyerVal = document.getElementById('sl-buyer').value;
  if (!buyerVal) {
    const errEl = document.getElementById('sl-buyer-error');
    if (errEl) errEl.style.display = '';
    document.getElementById('sl-buyer').focus();
    showToast('error', '入力エラー', '販売先を選択してください');
    return;
  }

  const rows = document.querySelectorAll('#salesLines tr[data-line-id]');
  let items = [];
  const unavailableItems = [];
  rows.forEach(row => {
    const id    = row.dataset.lineId;
    const code  = document.getElementById(`sl-code-${id}`)?.value?.trim();
    const priceInput = document.getElementById(`sl-price-${id}`);
    const price = getSalesLinePriceUSD(priceInput);
    const inputCurrency = priceInput?.dataset.entryCurrency || _salesEntryCurrency;
    const inputAmount = parseSalesPrice(priceInput?.value);
    const chk   = document.getElementById(`sl-chk-${id}`);
    const included = chk ? chk.checked : true;
    if (code && price > 0) {
      const inv = APP_DATA.inventory.find(i => i.code === code);
      if (!inv) {
        unavailableItems.push(`${code}（未登録）`);
      } else if (included && !canUseInventoryItemForSales(inv)) {
        unavailableItems.push(`${code}（${inv.status}）`);
      }
      items.push({
        code,
        brand:  inv?.brand  || '',
        model:  inv?.model  || '',
        ref:    inv?.ref    || '',
        serial: inv?.serial || '',
        accessories: inv?.accessories || [],
        salePrice: price,
        inputCurrency,
        inputAmount,
        usdJpyRate: getSalesUsdRate(),
        returnType:   included ? null : 'takeback',
        returnStatus: included ? null : 'pending',
      });
    }
  });
  if (unavailableItems.length > 0) {
    showToast('error', '使用できない商品があります', `取置中・売上済の商品は売上登録できません: ${unavailableItems.join('、')}`);
    return;
  }
  if (items.length === 0) { showToast('error', '入力エラー', '有効な明細行がありません'); return; }

  let slipId = document.getElementById('sl-id').value.trim();
  if (!slipId) slipId = newSalesId();

  const total = items.reduce((s, i) => s + (i.returnType ? 0 : i.salePrice), 0);
  const taxFree   = isTaxFreeMode();
  const taxAmount = taxFree ? 0 : Math.floor(total * 0.10);
  const displayTotal = formatSalesDisplayAmount(total, _salesEntryCurrency);
  const newSale = {
    id: slipId,
    date:  document.getElementById('sl-date').value,
    items, total,
    currency: 'USD',
    inputCurrency: _salesEntryCurrency,
    sourceShipmentId: _salesSourceShipmentId || '',
    usdJpyRate: getSalesUsdRate(),
    taxFree,
    taxAmount,
    grandTotal: total + taxAmount,
    buyer: document.getElementById('sl-buyer').value,
    note:  document.getElementById('sl-note').value,
    revisions: [],
  };

  if (window.ZaikoAPI) {
    const salesItems = items.filter(item => !item.returnType);
    if (salesItems.length === 0) {
      showToast('error', '入力エラー', '売上対象の商品がありません');
      return;
    }
    try {
      const result = await window.ZaikoAPI.saveSale({
        buyerCode: newSale.buyer, saleDate: newSale.date, displayCurrency: newSale.inputCurrency,
        taxMode: newSale.taxFree ? 'tax_exempt' : 'taxable', taxRateBasisPoints: newSale.taxFree ? 0 : 1000,
        notes: newSale.note, lines: salesItems.map(item => ({ productCode: item.code,
          unitPriceMinor: newSale.inputCurrency === 'JPY' ? item.inputAmount : item.salePrice })),
      }, isBuyer());
      if (result.approval) {
        showToast('info', '承認申請を送信しました', `${result.record.slipNumber} はDBで取置中です。管理者承認後に売上確定します`);
      } else {
        showToast('success', '売上登録完了', `${result.record.slipNumber} / ${salesItems.length}点をDBへ保存しました`);
      }
      resetSalesForm();
      if (document.getElementById('slipListBody')) refreshSlipList();
    } catch (error) {
      showToast('error', '売上登録エラー', error.message);
    }
    return;
  }

  // ── 作業者の場合は管理者承認申請を送信 ──
  if (isBuyer()) {
    newSale.status = '承認待ち';
    APP_DATA.sales.push(newSale);
    const approvalRequest = requestApproval(
      'sales',
      '売上登録 承認申請',
      {
        slipId,
        buyer:  newSale.buyer,
        total,
        items,
        currency: 'USD',
        inputCurrency: newSale.inputCurrency,
        sourceShipmentId: newSale.sourceShipmentId,
        usdJpyRate: newSale.usdJpyRate,
        date:   newSale.date,
        note:   newSale.note,
        taxFree: newSale.taxFree,
        taxAmount: newSale.taxAmount,
        grandTotal: newSale.grandTotal,
        requestedBy: currentUser()?.name || '—',
      },
      `伝票: ${slipId} / ${items.length}点 / 合計 ${displayTotal}`,
      null
    );
    if (approvalRequest) {
      const unpublishedCodes = [];
      items.forEach(saleItem => {
        if (saleItem.returnType) return;
        const inventoryItem = APP_DATA.inventory.find(item => item.code === saleItem.code);
        if (!inventoryItem) return;
        inventoryItem.reservationPreviousStatus = inventoryItem.status;
        inventoryItem.status = '保留';
        inventoryItem.reservationApprovalId = approvalRequest.id;
        unpublishedCodes.push(inventoryItem.code);
      });
      if (typeof unpublishGuestProducts === 'function') unpublishGuestProducts(unpublishedCodes);
    }
    refreshLinkedBusinessViews({ source: 'sales-approval-request' });
    showToast('info', '承認申請を送信しました', `${slipId} の売上登録を管理者へ申請しました。承認後に確定されます。`);
    resetSalesForm();
    if (document.getElementById('slipListBody')) refreshSlipList();
    return;
  }

  // ── 管理者の場合は即時登録 ──
  APP_DATA.sales.push(newSale);
  const soldProductCodes = [];
  items.forEach(saleItem => {
    if (saleItem.returnType) return;
    const item = APP_DATA.inventory.find(record => record.code === saleItem.code);
    if (item) {
      item.status = '売上済';
      clearInventoryReservationMetadata(item);
      soldProductCodes.push(item.code);
    }
  });
  if (typeof unpublishGuestProducts === 'function') unpublishGuestProducts(soldProductCodes);
  refreshLinkedBusinessViews({ source: 'sales-register' });
  showToast('success', '売上登録完了', `${slipId} / ${items.length}点 合計${displayTotal} を登録しました`);
  resetSalesForm();
  if (document.getElementById('slipListBody')) refreshSlipList();
}

// =====================================================
// 出荷登録
// =====================================================
let shippingLineCount = 0;
let _shippingPurchaseRequestId = null;

function initShippingForm() {
  const now = new Date();
  document.getElementById('sh-id').value = `SH-${now.getFullYear()}-${String(APP_DATA.shipments.length + 1).padStart(4, '0')}`;
  document.getElementById('sh-date').value = getLocalDateISO(now);

  const destSel = document.getElementById('sh-dest');
  if (typeof populateBuyerMasterSelect === 'function') {
    populateBuyerMasterSelect('sh-dest', { emptyLabel: '-- 選択 --', selected: destSel?.value || '', labelMode: 'code-name' });
  }

  shippingLineCount = 0;
  document.getElementById('shippingLines').innerHTML = '';
  addShippingLine();
}

function init_shipping() {
  if (typeof populateBuyerMasterSelect === 'function') {
    populateBuyerMasterSelect('sh-dest', {
      emptyLabel: '-- 選択 --', selected: document.getElementById('sh-dest')?.value || '', labelMode: 'code-name',
    });
  }
}

function addShippingLine() {
  shippingLineCount++;
  const div = document.createElement('div');
  div.className = 'slip-line';
  div.dataset.lineId = shippingLineCount;
  div.innerHTML = `
    <div>
      <input class="form-control" style="font-size:12px;" placeholder="20260412-001"
        oninput="autoFillItem(this, ${shippingLineCount}, 'shipping')" id="sh-code-${shippingLineCount}">
    </div>
    <div id="sh-brand-${shippingLineCount}" style="font-size:12px;color:var(--text-muted);">—</div>
    <div id="sh-model-${shippingLineCount}" style="font-size:12px;color:var(--text-muted);">—</div>
    <div>
      <input type="text" inputmode="numeric" class="form-control" style="font-size:12px;" placeholder="0"
        oninput="priceFormatHandler(this);calcShippingTotal()" onblur="priceFormatHandler(this)" id="sh-price-${shippingLineCount}">
    </div>
    <div>
      <button class="btn btn-ghost btn-sm" style="color:var(--danger);" onclick="removeLine(this, 'shipping')"><i class="fa-solid fa-xmark"></i></button>
    </div>
  `;
  document.getElementById('shippingLines').appendChild(div);
}

function calcShippingTotal() {
  let total = 0;
  document.querySelectorAll('[id^="sh-price-"]').forEach(input => {
    total += getPriceValue(input);
  });
  document.getElementById('shippingTotalDisplay').textContent = formatSalePrice(total);
}

function unlockShippingEdit() {
  // 管理者はPW不要
}

function resetShippingForm() {
  _shippingPurchaseRequestId = null;
  shippingLineCount = 0;
  document.getElementById('shippingLines').innerHTML = '';
  const now = new Date();
  document.getElementById('sh-id').value = `SH-${now.getFullYear()}-${String(APP_DATA.shipments.length + 1).padStart(4, '0')}`;
  document.getElementById('sh-note').value = '';
  addShippingLine();
  calcShippingTotal();
  showToast('info', 'リセット', 'フォームをリセットしました');
}

function canUseInventoryItemForShipping(inventoryItem, purchaseRequestId = _shippingPurchaseRequestId) {
  if (!inventoryItem) return false;
  if (!purchaseRequestId) return inventoryItem.status === '在庫中';
  const request = (APP_DATA.purchaseRequests || []).find(item => item.id === purchaseRequestId);
  return inventoryItem.status === '取置中'
    && inventoryItem.reservationRequestId === purchaseRequestId
    && Boolean(request?.items?.some(item => item.itemCode === inventoryItem.code && item.itemStatus === 'approved'));
}

async function saveShipping() {
  const lines = document.querySelectorAll('#shippingLines .slip-line');
  let items = [];
  const unavailableItems = [];
  lines.forEach(line => {
    const id = line.dataset.lineId;
    const code = document.getElementById(`sh-code-${id}`)?.value?.trim();
    const price = getPriceValue(document.getElementById(`sh-price-${id}`));
    if (code && price > 0) {
      const item = APP_DATA.inventory.find(i => i.code === code);
      if (!canUseInventoryItemForShipping(item)) {
        unavailableItems.push(`${code}（${item?.status || '未登録'}）`);
      }
      items.push({ code, brand: item?.brand || '', model: item?.model || '', wholesale: price, salePrice: price });
    }
  });
  if (unavailableItems.length > 0) {
    showToast('error', '使用できない商品があります', `取置中の商品は該当リクエスト経由でのみ出荷できます: ${unavailableItems.join('、')}`);
    return;
  }
  if (items.length === 0) { showToast('error', '入力エラー', '有効な明細行がありません'); return; }

  const total = items.reduce((s, i) => s + i.wholesale, 0);
  const shipId    = document.getElementById('sh-id').value;
  const shipDate  = document.getElementById('sh-date').value;
  const shipDest  = document.getElementById('sh-dest').value;
  const shipNote  = document.getElementById('sh-note').value;

  const purchaseRequestId = _shippingPurchaseRequestId;
  const sourceRequest = purchaseRequestId
    ? (APP_DATA.purchaseRequests || []).find(request => request.id === purchaseRequestId)
    : null;
  if (sourceRequest && (!sourceRequest.buyerCode || shipDest !== sourceRequest.buyerCode)) {
    showToast('error', '販売先コードが一致しません', `${sourceRequest.id} は ${sourceRequest.buyerCode || '未連携'} に紐づいています`);
    return;
  }
  const clientCompanyCode = sourceRequest?.clientCompanyCode
    || (APP_DATA.clientCompanies || []).find(company => company.buyerCode === shipDest)?.id
    || '';
  const newShip = {
    id: shipId, date: shipDate, destination: shipDest, items, total, note: shipNote,
    purchaseRequestId: purchaseRequestId || '',
    buyerCode: shipDest,
    clientCompanyCode,
  };

  if (window.ZaikoAPI) {
    const buyer = (APP_DATA.buyers || []).find(item => item.code === shipDest) || {};
    try {
      const result = await window.ZaikoAPI.saveShipment({
        buyerCode: shipDest, shipmentDate: shipDate, recipientName: buyer.name || '',
        recipientAddress: buyer.address || '', carrier: '', trackingNumber: '', notes: shipNote,
        productCodes: items.map(item => item.code),
      }, typeof isWorker === 'function' && isWorker());
      const saved = result.record;
      if (result.approval) {
        showToast('success', '承認申請を送信しました', `${saved.slipNumber} / ${items.length}点を取置中にして管理者へ申請しました`);
        resetShippingForm();
        return;
      }
      const persistedShip = { ...newShip, _id: saved.id, id: saved.slipNumber, date: saved.shipmentDate };
      showToast('success', '出荷登録完了', `${saved.slipNumber} / ${items.length}点をDBへ保存し、在庫を出荷済へ更新しました`);
      doShippingToSales(items, saved.slipNumber, saved.shipmentDate, shipDest, shipNote, persistedShip);
    } catch (error) {
      showToast('error', '出荷登録エラー', error.message);
    }
    return;
  }

  // 作業者・管理者ともに認証不要で即時登録
  APP_DATA.shipments.push(newShip);
  const shippedProductCodes = [];
  items.forEach(shippedItem => {
    const item = APP_DATA.inventory.find(record => record.code === shippedItem.code);
    if (item) {
      item.status = '出荷済';
      clearInventoryReservationMetadata(item);
      shippedProductCodes.push(item.code);
    }
  });
  if (purchaseRequestId) {
    const request = (APP_DATA.purchaseRequests || []).find(item => item.id === purchaseRequestId);
    if (request) {
      request.status = '対応済';
      request.fulfillmentStatus = '出荷済';
      request.shipmentId = shipId;
      request.shippedAt = new Date().toISOString();
      if (typeof persistPurchaseRequests === 'function') persistPurchaseRequests();
      persistBusinessWorkflowState();
    }
  }
  if (typeof unpublishGuestProducts === 'function') unpublishGuestProducts(shippedProductCodes);
  refreshLinkedBusinessViews({ source: 'shipping-register' });
  doShippingToSales(items, shipId, shipDate, shipDest, shipNote, newShip);
}

function doShippingToSales(items, shipId, shipDate, shipDest, shipNote, sourceShip = {}) {
  const total = items.reduce((s, i) => s + i.wholesale, 0);
  resetShippingForm();
  navigateTo('sales');

  setTimeout(() => {
    // applyShipmentToSales で統一処理
    const pseudoShip = {
      id: shipId,
      date: shipDate,
      destination: shipDest,
      items: items,
      total: total,
      note: shipNote,
      purchaseRequestId: sourceShip.purchaseRequestId || '',
      buyerCode: sourceShip.buyerCode || shipDest,
      clientCompanyCode: sourceShip.clientCompanyCode || '',
    };
    applyShipmentToSales(pseudoShip);
    // 売上一覧も更新
    initSalesListFilter();
    filterSalesList();
  }, 200);
}

// 伝票印刷・CSV出力（モック）
function printSlip(type) {
  if (type === 'shipping') {
    openShipPrintModal();
    return;
  }
  showToast('info', '印刷', '印刷ダイアログを開きます（モック）');
}

// =====================================================
// 出荷登録 > 印刷プレビュー
// =====================================================

/** HTML特殊文字をエスケープ（XSS防止・文字化け防止） */
function _escHtml(str) {
  if (str == null) return '';
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

/**
 * 出荷明細表HTMLを組み立てて返す
 * フォームの現在値（未保存状態でも）を使用する
 */
function buildShipmentSlipHTML() {
  const slipId = document.getElementById('sh-id')?.value || '—';
  const slipDate = document.getElementById('sh-date')?.value || '—';
  const destinationCode = document.getElementById('sh-dest')?.value || '';
  const destination = APP_DATA.buyers.find(record => record.code === destinationCode)
    || { name: destinationCode || '（出荷先未設定）' };
  const note = document.getElementById('sh-note')?.value || '';
  const items = [];

  document.querySelectorAll('#shippingLines .slip-line').forEach(line => {
    const lineId = line.dataset.lineId;
    const code = document.getElementById(`sh-code-${lineId}`)?.value?.trim() || '';
    if (!code) return;
    const inventoryItem = APP_DATA.inventory.find(item => item.code === code) || {};
    const detail = [
      [inventoryItem.brand, inventoryItem.model].filter(Boolean).join(' / '),
      [inventoryItem.ref && `型番: ${inventoryItem.ref}`, inventoryItem.serial && `シリアル: ${inventoryItem.serial}`].filter(Boolean).join('　'),
      inventoryItem.sku ? `SKU: ${inventoryItem.sku}` : '',
      inventoryItem.accessories?.length ? `付属品: ${inventoryItem.accessories.join('・')}` : '',
      inventoryItem.note ? `備考: ${inventoryItem.note}` : '',
    ].filter(Boolean).join('\n') || code;
    items.push({
      no: items.length + 1,
      detail,
      amount: Number(inventoryItem.purchasePrice) || 0,
      code,
    });
  });

  return buildTemplateStyleSlipDocument({
    title: '出荷伝票',
    slipId,
    transactionDate: slipDate,
    transactionDateLabel: '出荷日',
    counterpartyLabel: '出荷先',
    counterparty: destination,
    items,
    note,
    formatAmount: amount => formatPrice(amount),
    currencyLabel: 'JPY（円）',
    taxMode: 'none',
    includeBank: false,
    summaryMessage: '下記商品を出荷いたします。',
    amountCaption: '仕入金額合計',
  });
}

/** @deprecated 雛形変更前の帳票。比較参照用に保持 */
function _buildLegacyShipmentSlipHTML() {
  // ── フォームから現在値を取得 ──
  const slipId   = document.getElementById('sh-id')?.value   || '—';
  const slipDate = document.getElementById('sh-date')?.value || '—';
  const destCode = document.getElementById('sh-dest')?.value || '';
  const note     = document.getElementById('sh-note')?.value || '';

  // 出荷先名称を解決
  const buyerRec  = APP_DATA.buyers.find(b => b.code === destCode);
  const destName  = buyerRec ? `${buyerRec.code} — ${buyerRec.name}` : (destCode || '—');

  // ── 明細行を収集 ──
  const lines = document.querySelectorAll('#shippingLines .slip-line');
  const items = [];
  lines.forEach(line => {
    const lid   = line.dataset.lineId;
    const code  = document.getElementById(`sh-code-${lid}`)?.value?.trim() || '';
    const price = getPriceValue(document.getElementById(`sh-price-${lid}`));
    if (!code && price === 0) return;  // 完全空行はスキップ
    const inv     = APP_DATA.inventory.find(i => i.code === code);
    const brand   = inv?.brand  || document.getElementById(`sh-brand-${lid}`)?.textContent || '';
    const model   = inv?.model  || document.getElementById(`sh-model-${lid}`)?.textContent || '';
    const sku     = inv?.sku    || '';
    // 商品明細テキスト：ブランド＋モデル＋型番
    let detail = [brand, model].filter(Boolean).join(' ');
    if (inv?.ref) detail += `（${inv.ref}）`;
    if (!detail && sku) detail = sku;
    if (!detail && code) detail = code;
    items.push({ code, detail: detail || '—', price });
  });

  // 明細ゼロのときはサンプル1行表示
  const displayItems = items.length > 0 ? items : [
    { code: '—', detail: '（明細行が入力されていません）', price: 0 }
  ];

  // ── 金額計算 ──
  const subtotal  = displayItems.reduce((s, i) => s + i.price, 0);
  const taxAmount = Math.floor(subtotal * 0.1);
  const grandTotal = subtotal + taxAmount;

  // ── 明細行HTML ──
  const rowsHTML = displayItems.map((item, idx) => `
    <tr>
      <td>${idx + 1}</td>
      <td style="font-size:11px;color:#6b7280;">${_escHtml(item.code)}</td>
      <td>${_escHtml(item.detail)}</td>
      <td class="right">${item.price > 0 ? formatSalePrice(item.price) : '—'}</td>
    </tr>`).join('');

  // ── 会社情報 ──
  const ci = getSlipCompanyInfo();
  const companyHTML = `
    <div class="sh-company-block">
      <strong>${_escHtml(ci.companyName || '（会社名未設定）')}</strong>
      ${ci.zip     ? `${_escHtml(ci.zip)}<br>` : ''}
      ${ci.address ? `${_escHtml(ci.address)}<br>` : ''}
      ${ci.tel     ? `TEL: ${_escHtml(ci.tel)}<br>` : ''}
      ${ci.fax     ? `FAX: ${_escHtml(ci.fax)}<br>` : ''}
      ${ci.invoice ? `登録番号: ${_escHtml(ci.invoice)}<br>` : ''}
      ${ci.email   ? _escHtml(ci.email)              : ''}
    </div>`;

  // ── 備考HTML ──
  const noteHTML = `
    <div class="sh-note-block">
      <strong>備考</strong>
      ${note ? _escHtml(note) : '<span style="color:#9ca3af;">なし</span>'}
    </div>`;

  // ── 全体組み立て ──
  return `
    <div class="sh-slip">
      <div class="sh-slip-title">出 荷 明 細 表</div>

      <div class="sh-meta-grid">
        <div class="sh-meta-item">
          <span class="sh-meta-label">出荷伝票番号</span>
          <span><strong>${_escHtml(slipId)}</strong></span>
        </div>
        <div class="sh-meta-item">
          <span class="sh-meta-label">出荷日</span>
          <span>${_escHtml(slipDate)}</span>
        </div>
        <div class="sh-meta-item">
          <span class="sh-meta-label">出荷先</span>
          <span>${_escHtml(destName)}</span>
        </div>
        <div class="sh-meta-item">
          <span class="sh-meta-label">発行日</span>
          <span>${new Date().toLocaleDateString('ja-JP')}</span>
        </div>
      </div>

      <table class="sh-items-table">
        <colgroup>
          <col class="col-no">
          <col class="col-code">
          <col class="col-detail">
          <col class="col-amount">
        </colgroup>
        <thead>
          <tr>
            <th>No.</th>
            <th>商品コード</th>
            <th>商品明細</th>
            <th class="right">金額（売価・USD）</th>
          </tr>
        </thead>
        <tbody>${rowsHTML}</tbody>
      </table>

      <div class="sh-totals-wrap">
        <div class="sh-totals">
          <div class="sh-totals-row">
            <span>小計（税抜）</span>
            <span>${formatSalePrice(subtotal)}</span>
          </div>
          <div class="sh-totals-row">
            <span>消費税（10%）</span>
            <span>${formatSalePrice(taxAmount)}</span>
          </div>
          <div class="sh-totals-row sh-grand">
            <span>合計（税込）</span>
            <span>${formatSalePrice(grandTotal)}</span>
          </div>
        </div>
      </div>

      ${noteHTML}
      ${companyHTML}
    </div>`;
}

/** 出荷印刷モーダルを開く */
function openShipPrintModal() {
  const html = buildShipmentSlipHTML();
  document.getElementById('shipPrintPreviewContent').innerHTML = html;
  document.getElementById('shipPrintModal').style.display = 'flex';
}

/** 出荷印刷モーダルを閉じる */
function closeShipPrintModal() {
  document.getElementById('shipPrintModal').style.display = 'none';
}

function downloadCurrentShipmentDocument() {
  const content = document.getElementById('shipPrintPreviewContent')?.innerHTML || '';
  const slipId = document.getElementById('sh-id')?.value?.trim() || 'shipment';
  _downloadTemplateDocument('出荷伝票', `${slipId}_出荷伝票.html`, content);
}

/** 出荷明細表を印刷実行 */
function execShipPrint() {
  // ① すでにあれば削除
  const OLD_TARGET = document.getElementById('shipPrintCloneTarget');
  if (OLD_TARGET) OLD_TARGET.remove();

  // ② プレビュー内容をbody直下にクローン（@media printで body>* を制御するため）
  const src   = document.getElementById('shipPrintPreviewContent');
  const clone = document.createElement('div');
  clone.id = 'shipPrintCloneTarget';
  clone.style.cssText = 'display:none;';  // 通常時は非表示
  clone.innerHTML = src.innerHTML;
  document.body.appendChild(clone);

  // ③ 印刷
  window.print();

  // ④ 印刷ダイアログが閉じた後にクローンを削除
  setTimeout(() => {
    const target = document.getElementById('shipPrintCloneTarget');
    if (target) target.remove();
  }, 2000);
}

// =====================================================
// 出荷登録 > 通関書類 印刷プレビュー
// =====================================================

/**
 * 現在の出荷フォーム情報から明細アイテム配列を組み立てる。
 * 各要素に通し番号(seq)を付与し、Page1/Page2で共通利用する。
 */
function _buildCustomsItems() {
  const lines = document.querySelectorAll('#shippingLines .slip-line');
  const items = [];
  let seq = 0;

  lines.forEach(line => {
    const lid   = line.dataset.lineId;
    const code  = document.getElementById(`sh-code-${lid}`)?.value?.trim() || '';
    const price = getPriceValue(document.getElementById(`sh-price-${lid}`));
    if (!code && price === 0) return; // 空行スキップ

    seq++;
    const inv = APP_DATA.inventory.find(i => i.code === code);

    // 素材
    const matCode = inv?.material || '';
    const matRec  = APP_DATA.materials?.find(m => m.code === matCode);
    const matName = matRec ? matRec.name : (matCode || '—');

    // 駆動方式
    const movCode = inv?.movement || '';
    const movRec  = APP_DATA.movements?.find(m => m.code === movCode);
    const movName = movRec ? movRec.name : (movCode || '—');

    // ブランド・モデル・型番・シリアル・文字盤・ベルト素材・備考
    const brand = inv?.brand  || document.getElementById(`sh-brand-${lid}`)?.textContent?.trim() || '—';
    const model = inv?.model  || document.getElementById(`sh-model-${lid}`)?.textContent?.trim() || '—';
    const ref   = inv?.ref    || '—';
    const serial = inv?.serial || '—';
    const dial  = inv?.dialColor || inv?.dial || '—';
    const belt  = inv?.beltMaterial || inv?.strapMaterial || '—';
    const itemNote = inv?.note || '';

    // 画像（最初の1枚）
    const images = inv?.images || [];
    const imgUrl = images.length > 0 ? images[0] : '';

    items.push({ seq, code, brand, model, ref, serial, matName, movName, dial, belt, price, itemNote, imgUrl });
  });

  return items;
}

/**
 * 通関書類HTMLを2ページ構成で組み立てて返す
 */
function buildCustomsDocsHTML() {
  // ── フォームから現在値を取得 ──
  const slipId   = document.getElementById('sh-id')?.value   || '—';
  const slipDate = document.getElementById('sh-date')?.value || '—';
  const destCode = document.getElementById('sh-dest')?.value || '';
  const issueDate = new Date().toLocaleDateString('ja-JP', { year: 'numeric', month: '2-digit', day: '2-digit' });

  // 出荷先名称を解決
  const buyerRec = APP_DATA.buyers.find(b => b.code === destCode);
  const destName = buyerRec ? `${buyerRec.code} — ${buyerRec.name}` : (destCode || '—');

  // 明細アイテム（通し番号付き）
  const items = _buildCustomsItems();

  // ── ページ1: 通関用明細表 ──
  const tableRowsHTML = items.length > 0
    ? items.map(it => `
        <tr>
          <td style="text-align:center;">${it.seq}</td>
          <td style="font-size:9px;">${_escHtml(it.code)}</td>
          <td style="font-size:9px;">${_escHtml(it.matName)}</td>
          <td style="font-size:9px;">${_escHtml(it.movName)}</td>
          <td style="font-size:9px;">${_escHtml(it.ref)}</td>
          <td style="font-size:9px;">${_escHtml(it.serial)}</td>
          <td style="font-size:9px;">${_escHtml(it.brand)}</td>
          <td style="font-size:9px;">${_escHtml(it.dial)}</td>
          <td style="font-size:9px;">${_escHtml(it.belt)}</td>
          <td style="text-align:right;font-size:9px;">${it.price > 0 ? formatSalePrice(it.price) : '—'}</td>
          <td style="font-size:9px;">${_escHtml(it.itemNote)}</td>
          <td style="font-size:9px;">${_escHtml(it.code)}</td>
        </tr>`).join('')
    : `<tr><td colspan="12" style="text-align:center;color:#9ca3af;padding:16px;">（明細行が入力されていません）</td></tr>`;

  const page1HTML = `
    <div class="cd-doc-page">
      <div class="cd-page-header">
        <div class="cd-title">通関用明細表</div>
        <div class="cd-slip-no">
          伝票番号：<strong>${_escHtml(slipId)}</strong><br>
          発行日：${issueDate}
        </div>
      </div>
      <div class="cd-meta">
        <div class="cd-meta-item">
          <span class="cd-meta-label">出荷先</span>
          <span class="cd-meta-value">${_escHtml(destName)}</span>
        </div>
        <div class="cd-meta-item">
          <span class="cd-meta-label">出荷日</span>
          <span class="cd-meta-value">${_escHtml(slipDate)}</span>
        </div>
        <div class="cd-meta-item">
          <span class="cd-meta-label">商品数</span>
          <span class="cd-meta-value">${items.length} 点</span>
        </div>
        <div class="cd-meta-item">
          <span class="cd-meta-label">合計売価（USD）</span>
          <span class="cd-meta-value">${formatSalePrice(items.reduce((s,i)=>s+i.price,0))}</span>
        </div>
      </div>
      <table class="cd-items-table">
        <colgroup>
          <col class="cd-col-no">
          <col class="cd-col-code">
          <col class="cd-col-mat">
          <col class="cd-col-mov">
          <col class="cd-col-ref">
          <col class="cd-col-ser">
          <col class="cd-col-brand">
          <col class="cd-col-dial">
          <col class="cd-col-belt">
          <col class="cd-col-price">
          <col class="cd-col-note">
          <col class="cd-col-coderef">
        </colgroup>
        <thead>
          <tr>
            <th>通し番号</th>
            <th>商品コード</th>
            <th>素材</th>
            <th>駆動方式</th>
            <th>型番</th>
            <th>シリアル</th>
            <th>ブランド</th>
            <th>文字盤</th>
            <th>ベルト素材</th>
            <th>売価（USD）</th>
            <th>備考</th>
            <th>コード</th>
          </tr>
        </thead>
        <tbody>${tableRowsHTML}</tbody>
      </table>
    </div>`;

  // ── ページ2: 商品画像一覧 ──
  const imgBlocksHTML = items.length > 0
    ? items.map(it => `
        <div class="cd-img-block">
          <div class="cd-img-block-header">
            <span class="cd-img-badge">No.${it.seq}</span>
            <span class="cd-img-code">${_escHtml(it.code)}</span>
          </div>
          ${it.imgUrl
            ? `<img src="${_escHtml(it.imgUrl)}" alt="商品画像" onerror="this.style.display='none';this.nextElementSibling.style.display='flex';">
               <div class="cd-img-placeholder" style="display:none;"><i class="fa-solid fa-image" style="font-size:24px;"></i></div>`
            : `<div class="cd-img-placeholder"><i class="fa-solid fa-image" style="font-size:24px;"></i><span style="margin-top:4px;">画像なし</span></div>`
          }
          <div style="font-size:9px;margin-top:4px;color:#555;text-align:center;">${_escHtml(it.brand)} ${_escHtml(it.model)}</div>
        </div>`).join('')
    : `<div style="grid-column:1/-1;text-align:center;color:#9ca3af;padding:32px;">（明細行が入力されていません）</div>`;

  const page2HTML = `
    <div class="cd-doc-page">
      <div class="cd-page-header">
        <div class="cd-title">商品画像一覧</div>
        <div class="cd-slip-no">
          伝票番号：<strong>${_escHtml(slipId)}</strong><br>
          発行日：${issueDate}
        </div>
      </div>
      <div class="cd-meta">
        <div class="cd-meta-item">
          <span class="cd-meta-label">出荷先</span>
          <span class="cd-meta-value">${_escHtml(destName)}</span>
        </div>
        <div class="cd-meta-item">
          <span class="cd-meta-label">出荷日</span>
          <span class="cd-meta-value">${_escHtml(slipDate)}</span>
        </div>
        <div class="cd-meta-item">
          <span class="cd-meta-label">商品数</span>
          <span class="cd-meta-value">${items.length} 点</span>
        </div>
      </div>
      <p style="font-size:10px;color:#555;margin-bottom:10px;">
        ※ 通し番号はPage1「通関用明細表」と対応しています。
      </p>
      <div class="cd-img-grid">${imgBlocksHTML}</div>
    </div>`;

  // 2ページをラップして返す
  return `<div class="cd-doc-wrap">${page1HTML}<div class="cd-page-divider">── ページ区切り（2 / 2） ──</div>${page2HTML}</div>`;
}

/** 通関書類モーダルを開く */
function openCustomsDocs() {
  const html = buildCustomsDocsHTML();
  document.getElementById('customsModalBody').innerHTML = html;
  document.getElementById('customsModal').style.display = 'flex';
}

/** 通関書類モーダルを閉じる */
function closeCustomsModal() {
  document.getElementById('customsModal').style.display = 'none';
}

/** 通関書類を印刷実行（body直下クローン方式） */
function execCustomsPrint() {
  // ① 既存クローン削除
  const OLD = document.getElementById('customsPrintCloneTarget');
  if (OLD) OLD.remove();

  // ② プレビュー内容をbody直下にクローン
  const src   = document.getElementById('customsModalBody');
  const clone = document.createElement('div');
  clone.id = 'customsPrintCloneTarget';
  clone.style.cssText = 'display:none;';
  clone.innerHTML = src.innerHTML;
  document.body.appendChild(clone);

  // ③ 印刷実行
  window.print();

  // ④ 印刷ダイアログが閉じた後にクローンを削除
  setTimeout(() => {
    const target = document.getElementById('customsPrintCloneTarget');
    if (target) target.remove();
  }, 2500);
}

function _downloadDraftCSV(filename, rows) {
  const csv = rows
    .map(row => row.map(value => `"${String(value ?? '').replace(/"/g, '""')}"`).join(','))
    .join('\r\n');
  const url = URL.createObjectURL(new Blob(['\uFEFF' + csv], { type: 'text/csv;charset=utf-8;' }));
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = filename;
  anchor.style.display = 'none';
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  setTimeout(() => URL.revokeObjectURL(url), 0);
  return { filename, csv, rowCount: Math.max(0, rows.length - 1) };
}

function _downloadSalesDraftCSV() {
  const slipId = document.getElementById('sl-id')?.value?.trim() || '未採番';
  const saleDate = document.getElementById('sl-date')?.value || '';
  const buyerCode = document.getElementById('sl-buyer')?.value || '';
  const buyerName = getBuyerName(buyerCode);
  const currency = _salesEntryCurrency === 'JPY' ? 'JPY' : 'USD';
  const taxFree = isTaxFreeMode();
  const rate = getSalesUsdRate();
  const note = document.getElementById('sl-note')?.value || '';
  const items = [];

  document.querySelectorAll('#salesLines tr[data-line-id]').forEach(row => {
    const lineId = row.dataset.lineId;
    if (!document.getElementById(`sl-chk-${lineId}`)?.checked) return;
    const code = document.getElementById(`sl-code-${lineId}`)?.value?.trim() || '';
    const priceInput = document.getElementById(`sl-price-${lineId}`);
    const amountUSD = getSalesLinePriceUSD(priceInput);
    if (!code && amountUSD === 0) return;
    const inventoryItem = APP_DATA.inventory.find(item => item.code === code);
    items.push({ code, amountUSD, inventoryItem });
  });

  const subtotalUSD = items.reduce((sum, item) => sum + item.amountUSD, 0);
  const taxUSD = taxFree ? 0 : Math.floor(subtotalUSD * 0.1);
  const grandTotalUSD = subtotalUSD + taxUSD;
  const display = amount => convertSalesUSDToDisplay(amount, currency);
  const headers = [
    '伝票番号', '売上日', '販売先コード', '販売先名', '商品コード', 'ブランド', 'モデル',
    '型番', 'シリアル', '付属品', `販売金額（${currency}・税抜）`, '税区分',
    `伝票小計（${currency}）`, `消費税（${currency}）`, `伝票合計（${currency}）`, '換算レート', '備考',
  ];
  const rows = [headers];
  items.forEach(({ code, amountUSD, inventoryItem }) => rows.push([
    slipId,
    saleDate,
    buyerCode,
    buyerName,
    code,
    inventoryItem?.brand || '',
    inventoryItem?.model || '',
    inventoryItem?.ref || '',
    inventoryItem?.serial || '',
    (inventoryItem?.accessories || []).join('・'),
    display(amountUSD),
    taxFree ? '免税（0%）' : '課税（10%）',
    display(subtotalUSD),
    display(taxUSD),
    display(grandTotalUSD),
    `1 USD = ${rate} JPY`,
    note,
  ]));

  const result = _downloadDraftCSV(`sales_slip_${saleDate || new Date().toISOString().slice(0, 10)}.csv`, rows);
  showToast('success', 'ダウンロード完了', `売上伝票の明細${items.length}件をダウンロードしました`);
  return result;
}

function _downloadShippingDraftCSV() {
  const slipId = document.getElementById('sh-id')?.value?.trim() || '未採番';
  const shippingDate = document.getElementById('sh-date')?.value || '';
  const destinationCode = document.getElementById('sh-dest')?.value || '';
  const destinationName = getBuyerName(destinationCode);
  const note = document.getElementById('sh-note')?.value || '';
  const headers = [
    '伝票番号', '出荷日', '出荷先コード', '出荷先名', '商品コード', 'ブランド', 'モデル',
    '型番', 'シリアル', '付属品', '仕入金額（JPY）', '卸値（USD・税抜）', '備考',
  ];
  const rows = [headers];

  document.querySelectorAll('#shippingLines .slip-line').forEach(line => {
    const lineId = line.dataset.lineId;
    const code = document.getElementById(`sh-code-${lineId}`)?.value?.trim() || '';
    const wholesale = parseSalesPrice(document.getElementById(`sh-price-${lineId}`)?.value || '0');
    if (!code && wholesale === 0) return;
    const inventoryItem = APP_DATA.inventory.find(item => item.code === code);
    rows.push([
      slipId,
      shippingDate,
      destinationCode,
      destinationName,
      code,
      inventoryItem?.brand || '',
      inventoryItem?.model || '',
      inventoryItem?.ref || '',
      inventoryItem?.serial || '',
      (inventoryItem?.accessories || []).join('・'),
      inventoryItem?.purchasePrice || 0,
      wholesale,
      note,
    ]);
  });

  const result = _downloadDraftCSV(`shipping_slip_${shippingDate || new Date().toISOString().slice(0, 10)}.csv`, rows);
  showToast('success', 'ダウンロード完了', `出荷伝票の明細${result.rowCount}件をダウンロードしました`);
  return result;
}

function exportCSV(type) {
  if (type === 'sales') return _downloadSalesDraftCSV();
  if (type === 'shipping') return _downloadShippingDraftCSV();
  if (type === 'performance' && isWorker() && getLatestPerformanceApproval()?.status !== 'approved') {
    showToast('warning', '承認が必要です', '実績データの閲覧承認後にCSVを出力できます');
    return;
  }
  showToast('success', 'CSV出力', 'CSVファイルをダウンロードします（モック）');
}

// =====================================================
// マスタ登録
// =====================================================
const BRAND_MASTER_STORAGE_KEY = 'inv_brand_master_v1';
const BRAND_CODE_PREFIX = 'BRD-';
let _brandNextSequence = 1;

/** PostgreSQL/API接続後はブラウザ保存をマスタの参照元にしない。 */
function shouldUseBrowserMasterStorage() {
  return !(window.ZaikoAPI?.state?.hydrated);
}

function _formatBrandCode(sequence) {
  return `${BRAND_CODE_PREFIX}${String(sequence).padStart(3, '0')}`;
}

function _brandCodeSequence(code) {
  const match = String(code || '').toUpperCase().match(/^BRD-(\d+)$/);
  return match ? Number(match[1]) : 0;
}

function _syncLegacyBrandNames() {
  APP_DATA.brands = (APP_DATA.brandRecords || []).map(record => record.name);
}

function getBrandMasterRecords() {
  return (APP_DATA.brandRecords || []).map(record => ({ ...record }));
}

function getNextBrandCode() {
  const records = APP_DATA.brandRecords || [];
  const maxExisting = records.reduce((max, record) => Math.max(max, _brandCodeSequence(record.code)), 0);
  _brandNextSequence = Math.max(_brandNextSequence, maxExisting + 1);
  return _formatBrandCode(_brandNextSequence);
}

function getBrandRecordByName(name) {
  const target = String(name || '').trim();
  if (!target) return null;
  const records = APP_DATA.brandRecords || [];
  let record = records.find(item => item.name === target);
  if (record) return record;
  const aliases = APP_DATA.brandAliases || {};
  const visited = new Set();
  let resolved = target;
  while (aliases[resolved] && !visited.has(resolved)) {
    visited.add(resolved);
    resolved = aliases[resolved];
  }
  return records.find(item => item.name === resolved) || null;
}

function getBrandCodeByName(name) {
  return getBrandRecordByName(name)?.code || '';
}

function _normalizeBrandReference(target) {
  if (!target || typeof target !== 'object') return;
  const records = APP_DATA.brandRecords || [];
  const byCode = target.brandCode
    ? records.find(record => record.code === String(target.brandCode).toUpperCase())
    : null;
  const record = byCode || getBrandRecordByName(target.brand);
  if (!record) return;
  target.brandCode = record.code;
  target.brand = record.name;
}

/** 名称表示を維持しつつ、在庫・相場表・全伝票明細に固定ブランドコードを付与する。 */
function synchronizeBrandCodesAcrossData() {
  [...(APP_DATA.inventory || []), ...(APP_DATA.marketPrices || [])]
    .forEach(_normalizeBrandReference);
  ['purchaseSlips', 'shipments', 'sales', 'salesReturns', 'purchaseReturns', 'purchaseRequests']
    .forEach(collection => {
      (APP_DATA[collection] || []).forEach(record => {
        _normalizeBrandReference(record);
        [...(record.lines || []), ...(record.items || [])].forEach(item => {
          _normalizeBrandReference(item);
          _normalizeBrandReference(item.productDetail);
        });
      });
    });
  if (typeof _peSlipData !== 'undefined' && _peSlipData) {
    (_peSlipData.lines || []).forEach(line => {
      _normalizeBrandReference(line);
      _normalizeBrandReference(line.productDetail);
    });
  }
}

function getBrandMasterNames(extraValues = []) {
  const values = [...(APP_DATA.brandRecords || []).map(record => record.name), ...(extraValues || [])]
    .map(value => String(value || '').trim())
    .filter(Boolean);
  return [...new Set(values)];
}

function populateBrandMasterSelect(id, options = {}) {
  const select = document.getElementById(id);
  if (!select) return;
  const emptyLabel = Object.prototype.hasOwnProperty.call(options, 'emptyLabel')
    ? options.emptyLabel
    : '-- 選択 --';
  const selected = Object.prototype.hasOwnProperty.call(options, 'selected')
    ? String(options.selected || '')
    : select.value;
  const brands = getBrandMasterNames(options.extraValues || []);
  const emptyOption = emptyLabel === null
    ? ''
    : `<option value="">${_mEsc(emptyLabel)}</option>`;
  select.innerHTML = emptyOption + brands
    .map(brand => `<option value="${_mEsc(brand)}">${_mEsc(brand)}</option>`)
    .join('');
  if (brands.includes(selected)) select.value = selected;
  else if (emptyLabel !== null) select.value = '';
}

function _renameBrandReferences(oldName, newName, brandCode = '') {
  if (!oldName || !newName || oldName === newName) return;
  (APP_DATA.inventory || []).forEach(item => {
    if (item.brand === oldName || (brandCode && item.brandCode === brandCode)) {
      item.brand = newName;
      if (brandCode) item.brandCode = brandCode;
    }
  });
  (APP_DATA.marketPrices || []).forEach(item => {
    if (item.brand === oldName || (brandCode && item.brandCode === brandCode)) {
      item.brand = newName;
      if (brandCode) item.brandCode = brandCode;
    }
  });
  ['purchaseSlips', 'shipments', 'sales', 'salesReturns', 'purchaseReturns', 'purchaseRequests']
    .forEach(collection => {
      (APP_DATA[collection] || []).forEach(record => {
        [...(record.lines || []), ...(record.items || [])].forEach(line => {
          const detail = line.productDetail || line;
          if (detail.brand === oldName || (brandCode && detail.brandCode === brandCode)) {
            detail.brand = newName;
            if (brandCode) detail.brandCode = brandCode;
          }
        });
      });
    });
  if (typeof _peSlipData !== 'undefined' && _peSlipData) {
    (_peSlipData.lines || []).forEach(line => {
      if (line.productDetail?.brand === oldName || (brandCode && line.productDetail?.brandCode === brandCode)) {
        line.productDetail.brand = newName;
        if (brandCode) line.productDetail.brandCode = brandCode;
      }
    });
  }
  ['pu-brand', 'pep-brand', 'ie-brand', 'me-brand', 'market-f-brand', 'inv-f-brand'].forEach(id => {
    const element = document.getElementById(id);
    if (element?.value === oldName) element.dataset.brandRenameValue = newName;
  });
}

function _brandUsageCount(name) {
  let count = (APP_DATA.inventory || []).filter(item => item.brand === name).length;
  count += (APP_DATA.marketPrices || []).filter(item => item.brand === name).length;
  count += (APP_DATA.purchaseSlips || []).reduce((total, slip) =>
    total + (slip.lines || []).filter(line => line.productDetail?.brand === name).length, 0);
  if (typeof _peSlipData !== 'undefined' && _peSlipData) {
    count += (_peSlipData.lines || []).filter(line => line.productDetail?.brand === name).length;
  }
  return count;
}

function loadBrandMasterDirectory() {
  if (!shouldUseBrowserMasterStorage()) {
    APP_DATA.brandAliases = {};
    _syncLegacyBrandNames();
    const maxExisting = (APP_DATA.brandRecords || []).reduce((max, record) =>
      Math.max(max, _brandCodeSequence(record.code)), 0);
    _brandNextSequence = maxExisting + 1;
    return;
  }
  const seededRecords = Array.isArray(APP_DATA.brandRecords)
    ? APP_DATA.brandRecords
      .filter(record => record && /^BRD-\d+$/.test(String(record.code || '').toUpperCase()) && String(record.name || '').trim())
      .map(record => ({ code: String(record.code).toUpperCase(), name: String(record.name).trim() }))
    : [];
  const defaultRecords = seededRecords.length
    ? seededRecords
    : (APP_DATA.brands || []).map((name, index) => ({ code: _formatBrandCode(index + 1), name: String(name || '').trim() }));
  const defaultNames = defaultRecords.map(record => record.name);
  const defaultCodeByName = new Map(defaultRecords.map(record => [record.name, record.code]));
  const defaultNextSequence = defaultRecords.reduce((max, record) => Math.max(max, _brandCodeSequence(record.code)), 0) + 1;
  let records = defaultRecords.map(record => ({ ...record }));
  let aliases = {};
  let storedNextSequence = 0;
  try {
    const stored = JSON.parse(localStorage.getItem(BRAND_MASTER_STORAGE_KEY) || 'null');
    if (stored) {
      aliases = !Array.isArray(stored) && stored.aliases && typeof stored.aliases === 'object'
        ? stored.aliases
        : {};
      storedNextSequence = !Array.isArray(stored) ? Number(stored.nextSequence) || 0 : 0;
      const storedRecords = !Array.isArray(stored) ? stored.brandRecords : null;
      if (Array.isArray(storedRecords) && storedRecords.length) {
        const usedCodes = new Set();
        const usedNames = new Set();
        records = storedRecords.reduce((result, item) => {
          const code = String(item?.code || '').trim().toUpperCase();
          const name = String(item?.name || '').trim();
          if (!/^BRD-\d+$/.test(code) || !name || usedCodes.has(code) || usedNames.has(name)) return result;
          usedCodes.add(code);
          usedNames.add(name);
          result.push({ code, name });
          return result;
        }, []);
      } else {
        const legacyBrands = Array.isArray(stored) ? stored : stored.brands;
        if (Array.isArray(legacyBrands)) {
          const usedCodes = new Set();
          let migrationSequence = defaultNextSequence;
          records = [...new Set(legacyBrands.map(name => String(name || '').trim()).filter(Boolean))]
            .map(name => {
              const originalName = Object.keys(aliases).find(sourceName => aliases[sourceName] === name);
              let code = defaultCodeByName.get(name) || defaultCodeByName.get(originalName);
              if (!code || usedCodes.has(code)) {
                while (usedCodes.has(_formatBrandCode(migrationSequence))) migrationSequence += 1;
                code = _formatBrandCode(migrationSequence++);
              }
              usedCodes.add(code);
              return { code, name };
            });
        }
      }
    }
  } catch (error) {
    console.warn('ブランドマスタの保存データを読み込めませんでした', error);
  }
  APP_DATA.brandRecords = records;
  APP_DATA.brandAliases = { ...aliases };
  _syncLegacyBrandNames();
  const maxExisting = records.reduce((max, record) => Math.max(max, _brandCodeSequence(record.code)), 0);
  _brandNextSequence = Math.max(storedNextSequence, maxExisting + 1, defaultNextSequence);
  Object.entries(aliases).forEach(([oldName, newName]) => {
    _renameBrandReferences(oldName, newName, getBrandCodeByName(newName));
  });
  synchronizeBrandCodesAcrossData();
  persistBrandMasterDirectory();
  if (typeof persistBusinessWorkflowState === 'function') persistBusinessWorkflowState();
}

function persistBrandMasterDirectory() {
  if (!shouldUseBrowserMasterStorage()) return;
  _syncLegacyBrandNames();
  synchronizeBrandCodesAcrossData();
  localStorage.setItem(BRAND_MASTER_STORAGE_KEY, JSON.stringify({
    brands: getBrandMasterNames(),
    brandRecords: getBrandMasterRecords(),
    nextSequence: _brandNextSequence,
    aliases: APP_DATA.brandAliases || {},
  }));
}

function refreshBrandMasterConsumers(previousName = '', nextName = '') {
  const renamedSelection = element => {
    if (!element) return '';
    if (element.dataset.brandRenameValue) {
      const value = element.dataset.brandRenameValue;
      delete element.dataset.brandRenameValue;
      return value;
    }
    return element.value === previousName ? nextName : element.value;
  };

  const productBrand = document.getElementById('pu-brand');
  const purchaseBrand = document.getElementById('pep-brand');
  const itemEditBrand = document.getElementById('ie-brand');
  const marketEditBrand = document.getElementById('me-brand');
  populateBrandMasterSelect('pu-brand', { emptyLabel: '-- 選択 --', selected: renamedSelection(productBrand) });
  populateBrandMasterSelect('pep-brand', { emptyLabel: '-- 選択 --', selected: renamedSelection(purchaseBrand) });
  populateBrandMasterSelect('ie-brand', { emptyLabel: null, selected: renamedSelection(itemEditBrand) });
  populateBrandMasterSelect('me-brand', { emptyLabel: null, selected: renamedSelection(marketEditBrand) });

  _buildInvFilterOptions();
  if (typeof _marketBuildFilterOptions === 'function') _marketBuildFilterOptions();
  if (typeof _invSearched !== 'undefined' && _invSearched &&
      document.getElementById('page-inventory')?.classList.contains('hidden') === false) {
    renderInventoryTable();
  }
  if (typeof marketRenderTable === 'function' &&
      document.getElementById('page-market')?.classList.contains('hidden') === false) {
    marketRenderTable();
  }
}

const SUPPLIER_MASTER_STORAGE_KEY = 'inv_supplier_master_v1';

function getSupplierMasterRecords(extraCodes = []) {
  const records = (APP_DATA.suppliers || []).map(supplier => ({ ...supplier }));
  (extraCodes || []).map(code => String(code || '').trim()).filter(Boolean).forEach(code => {
    if (!records.some(supplier => supplier.code === code)) {
      records.push({ code, name: getSupplierName(code) || code, address: '', contact: '', invoice: '' });
    }
  });
  return records;
}

function populateSupplierMasterSelect(id, options = {}) {
  const select = document.getElementById(id);
  if (!select) return;
  const emptyLabel = Object.prototype.hasOwnProperty.call(options, 'emptyLabel')
    ? options.emptyLabel
    : '-- 選択 --';
  const selected = Object.prototype.hasOwnProperty.call(options, 'selected')
    ? String(options.selected || '')
    : select.value;
  const labelMode = options.labelMode || 'name';
  const suppliers = getSupplierMasterRecords(options.extraCodes || []);
  const emptyOption = emptyLabel === null ? '' : `<option value="">${_mEsc(emptyLabel)}</option>`;
  select.innerHTML = emptyOption + suppliers.map(supplier => {
    const label = labelMode === 'code-name' ? `${supplier.code} — ${supplier.name}` : supplier.name;
    return `<option value="${_mEsc(supplier.code)}">${_mEsc(label)}</option>`;
  }).join('');
  if (suppliers.some(supplier => supplier.code === selected)) select.value = selected;
  else if (emptyLabel !== null) select.value = '';
}

function _renameSupplierReferences(oldCode, newCode) {
  if (!oldCode || !newCode || oldCode === newCode) return;
  (APP_DATA.inventory || []).forEach(item => {
    if (item.supplier === oldCode) item.supplier = newCode;
  });
  (APP_DATA.marketPrices || []).forEach(item => {
    if (item.supplier === oldCode) item.supplier = newCode;
  });
  (APP_DATA.purchaseSlips || []).forEach(slip => {
    if (slip.supplier === oldCode) slip.supplier = newCode;
  });
  (APP_DATA.purchaseReturns || []).forEach(record => {
    if (record.supplier === oldCode) record.supplier = newCode;
  });
  if (typeof _peSlipData !== 'undefined' && _peSlipData?.supplier === oldCode) {
    _peSlipData.supplier = newCode;
  }
  (APP_DATA.clientCompanies || []).forEach(company => {
    if (company.supplierCode === oldCode) company.supplierCode = newCode;
  });
  ['pu-supplier', 'pe-supplier', 'ie-supplier', 'me-supplier', 'market-f-supplier', 'inv-f-supplier'].forEach(id => {
    const element = document.getElementById(id);
    if (element?.value === oldCode) element.dataset.supplierRenameValue = newCode;
  });
}

function _supplierUsageCount(code) {
  let count = (APP_DATA.inventory || []).filter(item => item.supplier === code).length;
  count += (APP_DATA.marketPrices || []).filter(item => item.supplier === code).length;
  count += (APP_DATA.purchaseSlips || []).filter(slip => slip.supplier === code).length;
  count += (APP_DATA.purchaseReturns || []).filter(record => record.supplier === code).length;
  if (typeof _peSlipData !== 'undefined' && _peSlipData?.supplier === code) count += 1;
  return count;
}

function loadSupplierMasterDirectory() {
  if (!shouldUseBrowserMasterStorage()) {
    APP_DATA.supplierAliases = {};
    return;
  }
  try {
    const stored = JSON.parse(localStorage.getItem(SUPPLIER_MASTER_STORAGE_KEY) || 'null');
    if (!stored) return;
    const suppliers = Array.isArray(stored) ? stored : stored.suppliers;
    if (Array.isArray(suppliers)) {
      APP_DATA.suppliers = suppliers
        .filter(supplier => supplier && typeof supplier.code === 'string' && typeof supplier.name === 'string')
        .map(supplier => ({
          code: supplier.code.trim().toUpperCase(),
          name: supplier.name.trim(),
          address: supplier.address || '',
          contact: supplier.contact || '',
          invoice: supplier.invoice || '',
        }));
    }
    const aliases = !Array.isArray(stored) && stored.aliases && typeof stored.aliases === 'object'
      ? stored.aliases
      : {};
    APP_DATA.supplierAliases = { ...aliases };
    Object.entries(aliases).forEach(([oldCode, newCode]) => _renameSupplierReferences(oldCode, newCode));
  } catch (error) {
    console.warn('仕入先マスタの保存データを読み込めませんでした', error);
  }
}

function persistSupplierMasterDirectory() {
  if (!shouldUseBrowserMasterStorage()) return;
  localStorage.setItem(SUPPLIER_MASTER_STORAGE_KEY, JSON.stringify({
    suppliers: (APP_DATA.suppliers || []).map(supplier => ({ ...supplier })),
    aliases: APP_DATA.supplierAliases || {},
  }));
}

function refreshSupplierMasterConsumers(previousCode = '', nextCode = '') {
  const renamedSelection = element => {
    if (!element) return '';
    if (element.dataset.supplierRenameValue) {
      const value = element.dataset.supplierRenameValue;
      delete element.dataset.supplierRenameValue;
      return value;
    }
    return element.value === previousCode ? nextCode : element.value;
  };

  const settings = [
    ['pu-supplier', '-- 選択 --', 'code-name'],
    ['pe-supplier', '-- 選択 --', 'name'],
    ['ie-supplier', '-- 選択 --', 'code-name'],
    ['me-supplier', '-- 選択 --', 'name'],
  ];
  settings.forEach(([id, emptyLabel, labelMode]) => {
    const element = document.getElementById(id);
    populateSupplierMasterSelect(id, {
      emptyLabel,
      selected: renamedSelection(element),
      labelMode,
    });
  });

  _buildInvFilterOptions();
  if (typeof _marketBuildFilterOptions === 'function') _marketBuildFilterOptions();
  if (typeof _invSearched !== 'undefined' && _invSearched &&
      document.getElementById('page-inventory')?.classList.contains('hidden') === false) {
    renderInventoryTable();
  }
  if (typeof marketRenderTable === 'function' &&
      document.getElementById('page-market')?.classList.contains('hidden') === false) {
    marketRenderTable();
  }
  if (nextCode) {
    const supplier = (APP_DATA.suppliers || []).find(record => record.code === nextCode);
    if (supplier) syncClientCompanyFromSupplier(supplier, { preferTrade: true });
  }
  reconcileClientCompanyDirectory({ persist: true });
}

// =====================================================
// 共通取引先会社台帳（販売先・仕入先の相互参照）
// =====================================================
const CLIENT_COMPANY_STORAGE_KEY = 'inv_client_company_directory_v1';
let _clientCompanyNextSequence = 1;

function _clientCompanySequence(code) {
  const match = String(code || '').toUpperCase().match(/^CLI-(\d+)$/);
  return match ? Number(match[1]) : 0;
}

function getNextClientCompanyCode() {
  const maxExisting = (APP_DATA.clientCompanies || []).reduce((max, company) =>
    Math.max(max, _clientCompanySequence(company.id)), 0);
  _clientCompanyNextSequence = Math.max(_clientCompanyNextSequence, maxExisting + 1);
  return `CLI-${String(_clientCompanyNextSequence).padStart(3, '0')}`;
}

function _normalizeCompanyMatchName(value) {
  return String(value || '')
    .toLocaleLowerCase('ja')
    .replace(/[\s　・･]/g, '')
    .replace(/株式会社|有限会社|合同会社|合資会社|合名会社|\(株\)|（株）|\(有\)|（有）/g, '');
}

function getClientCompanyTradeTypes(company) {
  const types = new Set(Array.isArray(company?.tradeTypes) ? company.tradeTypes : []);
  if (company?.buyerCode) types.add('buyer');
  if (company?.supplierCode) types.add('supplier');
  return ['buyer', 'supplier'].filter(type => types.has(type));
}

function getClientCompanyTradeLabel(company) {
  const types = getClientCompanyTradeTypes(company);
  if (types.includes('buyer') && types.includes('supplier')) return '販売先・仕入先';
  if (types.includes('supplier')) return '仕入先取引';
  if (types.includes('buyer')) return '販売先取引';
  return '未分類';
}

function _normalizeClientCompanyRecord(company) {
  const record = { ...(company || {}) };
  record.id = String(record.id || getNextClientCompanyCode()).toUpperCase();
  record.companyName = String(record.companyName || record.name || '').trim();
  record.buyerCode = String(record.buyerCode || '').trim().toUpperCase();
  record.supplierCode = String(record.supplierCode || '').trim().toUpperCase();
  record.tradeTypes = getClientCompanyTradeTypes(record);
  record.representative = String(record.representative || '');
  record.contactPerson = String(record.contactPerson || '');
  record.email = String(record.email || '');
  record.tel = String(record.tel || record.contact || '');
  record.address = String(record.address || '');
  record.invoice = String(record.invoice || '');
  record.note = String(record.note || record.notes || '');
  record.guestId = String(record.guestId || '');
  return record;
}

function loadClientCompanyDirectory() {
  let companies = (APP_DATA.clientCompanies || []).map(_normalizeClientCompanyRecord);
  if (!shouldUseBrowserMasterStorage()) {
    APP_DATA.clientCompanies = companies.filter((company, index, all) =>
      company.companyName && all.findIndex(candidate => candidate.id === company.id) === index);
    const maxExisting = APP_DATA.clientCompanies.reduce((max, company) =>
      Math.max(max, _clientCompanySequence(company.id)), 0);
    _clientCompanyNextSequence = maxExisting + 1;
    return;
  }
  let storedNextSequence = 0;
  try {
    const stored = JSON.parse(localStorage.getItem(CLIENT_COMPANY_STORAGE_KEY) || 'null');
    if (stored) {
      const storedCompanies = Array.isArray(stored) ? stored : stored.companies;
      if (Array.isArray(storedCompanies)) companies = storedCompanies.map(_normalizeClientCompanyRecord);
      storedNextSequence = !Array.isArray(stored) ? Number(stored.nextSequence) || 0 : 0;
    }
  } catch (error) {
    console.warn('取引先会社台帳の保存データを読み込めませんでした', error);
  }
  APP_DATA.clientCompanies = companies.filter((company, index, all) =>
    company.companyName && all.findIndex(candidate => candidate.id === company.id) === index);
  const maxExisting = APP_DATA.clientCompanies.reduce((max, company) =>
    Math.max(max, _clientCompanySequence(company.id)), 0);
  _clientCompanyNextSequence = Math.max(storedNextSequence, maxExisting + 1);
}

function persistClientCompanyDirectory() {
  if (!shouldUseBrowserMasterStorage()) return;
  localStorage.setItem(CLIENT_COMPANY_STORAGE_KEY, JSON.stringify({
    version: 1,
    updatedAt: new Date().toISOString(),
    nextSequence: _clientCompanyNextSequence,
    companies: (APP_DATA.clientCompanies || []).map(company => ({
      ...company,
      tradeTypes: getClientCompanyTradeTypes(company),
    })),
  }));
}

function _findClientCompanyForTrade(type, tradeRecord) {
  const codeField = type === 'buyer' ? 'buyerCode' : 'supplierCode';
  const direct = (APP_DATA.clientCompanies || []).find(company => company[codeField] === tradeRecord.code);
  if (direct) return direct;
  if (type === 'buyer') {
    const guest = (APP_DATA.guestAccounts || []).find(account => account.buyerCode === tradeRecord.code);
    if (guest) {
      const guestLinked = (APP_DATA.clientCompanies || []).find(company => company.guestId === guest.id);
      if (guestLinked) return guestLinked;
    }
  }
  const normalizedNames = new Set([_normalizeCompanyMatchName(tradeRecord.name)]);
  if (type === 'buyer') {
    (APP_DATA.guestAccounts || [])
      .filter(account => account.buyerCode === tradeRecord.code)
      .forEach(account => normalizedNames.add(_normalizeCompanyMatchName(account.company)));
  }
  return (APP_DATA.clientCompanies || []).find(company =>
    normalizedNames.has(_normalizeCompanyMatchName(company.companyName))) || null;
}

function _syncClientCompanyFromTrade(type, tradeRecord, options = {}) {
  if (!tradeRecord?.code || !tradeRecord?.name) return null;
  if (!Array.isArray(APP_DATA.clientCompanies)) APP_DATA.clientCompanies = [];
  let company = _findClientCompanyForTrade(type, tradeRecord);
  const isNew = !company;
  if (!company) {
    const id = getNextClientCompanyCode();
    company = _normalizeClientCompanyRecord({ id, companyName: tradeRecord.name, autoManaged: true });
    APP_DATA.clientCompanies.push(company);
    _clientCompanyNextSequence = Math.max(_clientCompanyNextSequence, _clientCompanySequence(id) + 1);
  }
  const codeField = type === 'buyer' ? 'buyerCode' : 'supplierCode';
  company[codeField] = tradeRecord.code;
  company.tradeTypes = getClientCompanyTradeTypes(company);
  const preferTrade = options.preferTrade === true;
  const copy = (targetKey, sourceValue) => {
    if (preferTrade || isNew || !company[targetKey]) company[targetKey] = String(sourceValue || '');
  };
  copy('companyName', tradeRecord.name);
  copy('address', tradeRecord.address);
  copy('tel', tradeRecord.contact);
  copy('invoice', tradeRecord.invoice);
  if (type === 'buyer') {
    copy('email', tradeRecord.email);
    const guest = options.guest || (APP_DATA.guestAccounts || []).find(account => account.buyerCode === tradeRecord.code);
    if (guest) {
      company.guestId = guest.id;
      if (!company.email) company.email = guest.email || '';
      if (isNew && guest.company) company.companyName = guest.company;
    }
  }
  return company;
}

function syncClientCompanyFromBuyer(buyer, options = {}) {
  return _syncClientCompanyFromTrade('buyer', buyer, options);
}

function syncClientCompanyFromSupplier(supplier, options = {}) {
  return _syncClientCompanyFromTrade('supplier', supplier, options);
}

function reconcileClientCompanyDirectory(options = {}) {
  if (!Array.isArray(APP_DATA.clientCompanies)) APP_DATA.clientCompanies = [];
  (APP_DATA.buyers || []).forEach(buyer => syncClientCompanyFromBuyer(buyer));
  (APP_DATA.suppliers || []).forEach(supplier => syncClientCompanyFromSupplier(supplier));
  APP_DATA.clientCompanies = APP_DATA.clientCompanies.filter(company => {
    const buyer = company.buyerCode
      ? (APP_DATA.buyers || []).find(record => record.code === company.buyerCode)
      : null;
    const supplier = company.supplierCode
      ? (APP_DATA.suppliers || []).find(record => record.code === company.supplierCode)
      : null;
    if (!buyer) company.buyerCode = '';
    if (!supplier) company.supplierCode = '';
    company.tradeTypes = [buyer ? 'buyer' : '', supplier ? 'supplier' : ''].filter(Boolean);
    if (buyer) {
      const guest = (APP_DATA.guestAccounts || []).find(account => account.buyerCode === buyer.code);
      company.guestId = guest?.id || '';
    }
    return company.tradeTypes.length > 0 || company.autoManaged !== true;
  });
  if (options.persist !== false) persistClientCompanyDirectory();
  return APP_DATA.clientCompanies;
}

function _nextTradeMasterCode(prefix, records) {
  const max = (records || []).reduce((current, record) => {
    const match = String(record.code || '').toUpperCase().match(new RegExp('^' + prefix + '(\\d+)'));
    return match ? Math.max(current, Number(match[1])) : current;
  }, 0);
  return `${prefix}${String(max + 1).padStart(3, '0')}`;
}

function applyClientCompanyToTradeMasters(company) {
  const types = getClientCompanyTradeTypes(company);
  if (types.includes('buyer')) {
    if (!company.buyerCode) company.buyerCode = _nextTradeMasterCode('B', APP_DATA.buyers || []);
    let buyer = (APP_DATA.buyers || []).find(record => record.code === company.buyerCode);
    if (!buyer) {
      buyer = { code: company.buyerCode, guestManaged: false };
      APP_DATA.buyers.push(buyer);
    }
    Object.assign(buyer, {
      name: company.companyName,
      address: company.address || '',
      contact: company.tel || '',
      invoice: company.invoice || '',
      email: company.email || '',
    });
  }
  if (types.includes('supplier')) {
    if (!company.supplierCode) company.supplierCode = _nextTradeMasterCode('S', APP_DATA.suppliers || []);
    let supplier = (APP_DATA.suppliers || []).find(record => record.code === company.supplierCode);
    if (!supplier) {
      supplier = { code: company.supplierCode };
      APP_DATA.suppliers.push(supplier);
    }
    Object.assign(supplier, {
      name: company.companyName,
      address: company.address || '',
      contact: company.tel || '',
      invoice: company.invoice || '',
      email: company.email || '',
    });
  }
  company.tradeTypes = getClientCompanyTradeTypes(company);
  return company;
}

const STAFF_MASTER_STORAGE_KEY = 'inv_staff_master_v1';

function _syncLegacyStaffNames() {
  APP_DATA.staff = (APP_DATA.staffRecords || []).map(record => record.name);
}

function getStaffMasterRecords(extraNames = []) {
  const records = (APP_DATA.staffRecords || []).map(record => ({ ...record }));
  (extraNames || []).map(name => String(name || '').trim()).filter(Boolean).forEach(name => {
    if (!records.some(record => record.name === name)) records.push({ code: '', name });
  });
  return records;
}

function getStaffMasterNames(extraNames = []) {
  return [...new Set(getStaffMasterRecords(extraNames).map(record => record.name).filter(Boolean))];
}

function getNextStaffCode() {
  const max = (APP_DATA.staffRecords || []).reduce((current, record) => {
    const match = String(record.code || '').match(/^STF-(\d+)$/i);
    return match ? Math.max(current, Number(match[1])) : current;
  }, 0);
  return `STF-${String(max + 1).padStart(3, '0')}`;
}

function populateStaffMasterSelect(id, options = {}) {
  const select = document.getElementById(id);
  if (!select) return;
  const emptyLabel = Object.prototype.hasOwnProperty.call(options, 'emptyLabel')
    ? options.emptyLabel
    : '-- 選択 --';
  const selected = Object.prototype.hasOwnProperty.call(options, 'selected')
    ? String(options.selected || '')
    : select.value;
  const names = getStaffMasterNames(options.extraNames || []);
  const emptyOption = emptyLabel === null ? '' : `<option value="">${_mEsc(emptyLabel)}</option>`;
  select.innerHTML = emptyOption + names
    .map(name => `<option value="${_mEsc(name)}">${_mEsc(name)}</option>`)
    .join('');
  if (names.includes(selected)) select.value = selected;
  else if (emptyLabel !== null) select.value = '';
}

function _renameStaffReferences(oldName, newName) {
  if (!oldName || !newName || oldName === newName) return;
  (APP_DATA.inventory || []).forEach(item => { if (item.staff === oldName) item.staff = newName; });
  (APP_DATA.marketPrices || []).forEach(item => { if (item.staff === oldName) item.staff = newName; });
  (APP_DATA.purchaseSlips || []).forEach(slip => {
    if (slip.staff === oldName) slip.staff = newName;
    (slip.lines || []).forEach(line => {
      if (line.productDetail?.staff === oldName) line.productDetail.staff = newName;
    });
  });
  if (typeof _peSlipData !== 'undefined' && _peSlipData?.staff === oldName) _peSlipData.staff = newName;
  ['pu-staff', 'pe-staff', 'ie-staff', 'me-staff', 'market-f-staff', 'inv-f-staff'].forEach(id => {
    const element = document.getElementById(id);
    if (element?.value === oldName) element.dataset.staffRenameValue = newName;
  });
}

function _staffUsageCount(name) {
  let count = (APP_DATA.inventory || []).filter(item => item.staff === name).length;
  count += (APP_DATA.marketPrices || []).filter(item => item.staff === name).length;
  count += (APP_DATA.purchaseSlips || []).filter(slip => slip.staff === name).length;
  count += (APP_DATA.purchaseSlips || []).reduce((total, slip) => total +
    (slip.lines || []).filter(line => line.productDetail?.staff === name).length, 0);
  if (typeof _peSlipData !== 'undefined' && _peSlipData?.staff === name) count += 1;
  return count;
}

function _ownCompanyName() {
  return getCompanyInfo().companyName || '当社';
}

function _nextStaffUserId() {
  if (typeof loginInfoNextId === 'function') return loginInfoNextId('U', APP_DATA.users || []);
  const max = (APP_DATA.users || []).reduce((current, user) => {
    const match = String(user.id || '').match(/^U(\d+)$/i);
    return match ? Math.max(current, Number(match[1])) : current;
  }, 0);
  return `U${String(max + 1).padStart(3, '0')}`;
}

function _uniqueStaffLoginId(code) {
  const used = new Set([
    ...(APP_DATA.users || []).map(user => String(user.loginId || '').toLowerCase()),
    ...(APP_DATA.guestAccounts || []).map(guest => String(guest.id || '').toLowerCase()),
  ]);
  const base = String(code || 'staff').toLowerCase();
  if (!used.has(base)) return base;
  let suffix = 2;
  while (used.has(`${base}-${suffix}`)) suffix += 1;
  return `${base}-${suffix}`;
}

function _ensureWorkerAccountForStaff(record) {
  if (!record?.code || !record?.name) return { user: null, changed: false };
  let changed = false;
  let user = (APP_DATA.users || []).find(item =>
    (item.role === 'buyer' || item.role === 'worker') && item.staffCode === record.code
  );
  if (!user) {
    user = (APP_DATA.users || []).find(item =>
      (item.role === 'buyer' || item.role === 'worker') && !item.staffCode && item.name === record.name
    );
  }
  if (!user) {
    const number = String(record.code).replace(/\D/g, '').padStart(3, '0');
    const domain = String(getCompanyInfo().email || '').split('@')[1] || 'company.local';
    user = {
      id: _nextStaffUserId(),
      role: 'buyer',
      name: record.name,
      loginId: _uniqueStaffLoginId(record.code),
      email: `staff${number}@${domain}`,
      password: `staff${number}`,
      avatar: record.name.slice(0, 1),
      active: true,
    };
    APP_DATA.users.push(user);
    changed = true;
  }
  const linkedValues = {
    role: 'buyer',
    name: record.name,
    staffCode: record.code,
    companyType: 'own',
    company: _ownCompanyName(),
    avatar: record.name.slice(0, 1),
  };
  Object.entries(linkedValues).forEach(([key, value]) => {
    if (user[key] !== value) {
      user[key] = value;
      changed = true;
    }
  });
  return { user, changed };
}

function ensureStaffWorkerAccounts() {
  let changed = false;
  (APP_DATA.staffRecords || []).forEach(record => {
    if (_ensureWorkerAccountForStaff(record).changed) changed = true;
  });
  if (changed && typeof persistLoginDirectory === 'function') persistLoginDirectory();
  return changed;
}

function loadStaffMasterDirectory() {
  if (!shouldUseBrowserMasterStorage()) {
    APP_DATA.staffAliases = {};
    _syncLegacyStaffNames();
    return;
  }
  let records = (APP_DATA.staff || []).map((name, index) => ({
    code: `STF-${String(index + 1).padStart(3, '0')}`,
    name: String(name || '').trim(),
  }));
  let aliases = {};
  try {
    const stored = JSON.parse(localStorage.getItem(STAFF_MASTER_STORAGE_KEY) || 'null');
    if (stored) {
      const storedRecords = Array.isArray(stored) ? stored : stored.staff;
      if (Array.isArray(storedRecords)) {
        records = storedRecords
          .filter(record => record && typeof record.code === 'string' && typeof record.name === 'string')
          .map(record => ({ code: record.code.trim().toUpperCase(), name: record.name.trim() }));
      }
      aliases = !Array.isArray(stored) && stored.aliases && typeof stored.aliases === 'object'
        ? stored.aliases
        : {};
    }
  } catch (error) {
    console.warn('仕入担当者マスタの保存データを読み込めませんでした', error);
  }
  APP_DATA.staffRecords = records.filter(record => record.code && record.name);
  APP_DATA.staffAliases = { ...aliases };
  Object.entries(aliases).forEach(([oldName, newName]) => _renameStaffReferences(oldName, newName));
  _syncLegacyStaffNames();
  ensureStaffWorkerAccounts();
  persistStaffMasterDirectory();
}

function persistStaffMasterDirectory() {
  if (!shouldUseBrowserMasterStorage()) return;
  localStorage.setItem(STAFF_MASTER_STORAGE_KEY, JSON.stringify({
    staff: (APP_DATA.staffRecords || []).map(record => ({ ...record })),
    aliases: APP_DATA.staffAliases || {},
  }));
}

function syncWorkerAccountToStaffMaster(user, previousName = '', preferredCode = '') {
  if (!user || (user.role !== 'buyer' && user.role !== 'worker')) return null;
  let record = (APP_DATA.staffRecords || []).find(item => item.code === (user.staffCode || preferredCode));
  if (!record && previousName) record = APP_DATA.staffRecords.find(item => item.name === previousName);
  if (!record) record = APP_DATA.staffRecords.find(item => item.name === user.name);
  if (!record) {
    record = { code: preferredCode || getNextStaffCode(), name: user.name };
    APP_DATA.staffRecords.push(record);
  }
  const oldName = record.name;
  record.name = user.name;
  user.staffCode = record.code;
  user.companyType = 'own';
  user.company = _ownCompanyName();
  if (oldName !== record.name) {
    _renameStaffReferences(oldName, record.name);
    if (!APP_DATA.staffAliases) APP_DATA.staffAliases = {};
    APP_DATA.staffAliases[oldName] = record.name;
  }
  _syncLegacyStaffNames();
  persistStaffMasterDirectory();
  refreshStaffMasterConsumers(oldName, record.name);
  return record;
}

function refreshStaffMasterConsumers(previousName = '', nextName = '') {
  const renamedSelection = element => {
    if (!element) return '';
    if (element.dataset.staffRenameValue) {
      const value = element.dataset.staffRenameValue;
      delete element.dataset.staffRenameValue;
      return value;
    }
    return element.value === previousName ? nextName : element.value;
  };
  ['pu-staff', 'pe-staff', 'ie-staff', 'me-staff'].forEach(id => {
    const element = document.getElementById(id);
    populateStaffMasterSelect(id, { emptyLabel: '-- 選択 --', selected: renamedSelection(element) });
  });
  _buildInvFilterOptions();
  if (typeof _marketBuildFilterOptions === 'function') _marketBuildFilterOptions();
  if (typeof renderMasterTabs === 'function') renderMasterTabs();
  if (typeof refreshPasswordMasterDirectory === 'function') refreshPasswordMasterDirectory();
}

const ACCESSORY_MASTER_STORAGE_KEY = 'inv_accessory_master_v1';

function _syncLegacyAccessoryNames() {
  APP_DATA.accessories = (APP_DATA.accessoryRecords || []).map(record => record.name);
}

function getAccessoryMasterRecords(extraNames = []) {
  const records = (APP_DATA.accessoryRecords || []).map(record => ({ ...record }));
  (extraNames || []).map(name => String(name || '').trim()).filter(Boolean).forEach(name => {
    if (!records.some(record => record.name === name)) records.push({ code: '', name });
  });
  return records;
}

function getAccessoryMasterNames(extraNames = []) {
  return [...new Set(getAccessoryMasterRecords(extraNames).map(record => record.name).filter(Boolean))];
}

function getNextAccessoryCode() {
  const max = (APP_DATA.accessoryRecords || []).reduce((current, record) => {
    const match = String(record.code || '').match(/^ACC-([0-9]+)$/);
    return match ? Math.max(current, Number(match[1])) : current;
  }, 0);
  return `ACC-${String(max + 1).padStart(3, '0')}`;
}

function _renameAccessoryReferences(oldName, newName) {
  if (!oldName || !newName || oldName === newName) return;
  const renameList = list => Array.isArray(list) ? list.map(name => name === oldName ? newName : name) : list;
  (APP_DATA.inventory || []).forEach(item => { item.accessories = renameList(item.accessories); });
  (APP_DATA.marketPrices || []).forEach(item => { item.accessories = renameList(item.accessories); });
  (APP_DATA.purchaseSlips || []).forEach(slip => {
    (slip.lines || []).forEach(line => {
      if (line.productDetail) line.productDetail.accessories = renameList(line.productDetail.accessories);
    });
  });
  if (typeof _peSlipData !== 'undefined' && _peSlipData) {
    (_peSlipData.lines || []).forEach(line => {
      if (line.productDetail) line.productDetail.accessories = renameList(line.productDetail.accessories);
    });
  }
  if (typeof _invAccFilterState !== 'undefined') {
    _invAccFilterState = renameList(_invAccFilterState);
  }
  const marketFilter = document.getElementById('market-f-accessory');
  if (marketFilter?.value === oldName) marketFilter.dataset.accessoryRenameValue = newName;
}

function _accessoryUsageCount(name) {
  let count = (APP_DATA.inventory || []).filter(item => (item.accessories || []).includes(name)).length;
  count += (APP_DATA.marketPrices || []).filter(item => (item.accessories || []).includes(name)).length;
  count += (APP_DATA.purchaseSlips || []).reduce((total, slip) =>
    total + (slip.lines || []).filter(line => (line.productDetail?.accessories || []).includes(name)).length, 0);
  if (typeof _peSlipData !== 'undefined' && _peSlipData) {
    count += (_peSlipData.lines || []).filter(line => (line.productDetail?.accessories || []).includes(name)).length;
  }
  return count;
}

function loadAccessoryMasterDirectory() {
  if (!shouldUseBrowserMasterStorage()) {
    APP_DATA.accessoryAliases = {};
    _syncLegacyAccessoryNames();
    return;
  }
  const defaults = (APP_DATA.accessories || []).map((name, index) => ({ code: `ACC-${String(index + 1).padStart(3, '0')}`, name }));
  APP_DATA.accessoryRecords = defaults;
  APP_DATA.accessoryAliases = {};
  try {
    const stored = JSON.parse(localStorage.getItem(ACCESSORY_MASTER_STORAGE_KEY) || 'null');
    if (stored && typeof stored === 'object') {
      const records = Array.isArray(stored) ? stored : stored.records;
      if (Array.isArray(records)) {
        APP_DATA.accessoryRecords = records
          .map(record => ({ code: String(record?.code || '').trim().toUpperCase(), name: String(record?.name || '').trim().toUpperCase() }))
          .filter(record => /^ACC-[0-9]+$/.test(record.code) && record.name)
          .filter((record, index, all) => all.findIndex(candidate => candidate.code === record.code) === index);
      }
      const aliases = !Array.isArray(stored) && stored.aliases && typeof stored.aliases === 'object' ? stored.aliases : {};
      APP_DATA.accessoryAliases = { ...aliases };
      Object.entries(aliases).forEach(([oldName, newName]) => _renameAccessoryReferences(oldName, newName));
    }
  } catch (error) {
    console.warn('付属品マスタの保存データを読み込めませんでした', error);
  }
  _syncLegacyAccessoryNames();
}

function persistAccessoryMasterDirectory() {
  if (!shouldUseBrowserMasterStorage()) return;
  localStorage.setItem(ACCESSORY_MASTER_STORAGE_KEY, JSON.stringify({
    records: getAccessoryMasterRecords(),
    aliases: APP_DATA.accessoryAliases || {},
  }));
}

function refreshAccessoryMasterConsumers(previousName = '', nextName = '') {
  _syncLegacyAccessoryNames();
  if (typeof _invAccBuildList === 'function') _invAccBuildList();
  if (typeof _invAccRenderTrigger === 'function') _invAccRenderTrigger();
  document.querySelectorAll('#pu-accessories input:checked').forEach(input => {
    if (input.value === previousName) input.value = nextName;
  });
  if (typeof _rebuildAccessoryCheckboxes === 'function') _rebuildAccessoryCheckboxes();
  const itemEditArea = document.getElementById('ie-accessories');
  if (itemEditArea) {
    const selected = [...itemEditArea.querySelectorAll('input:checked')].map(input => input.value === previousName ? nextName : input.value);
    itemEditArea.innerHTML = getAccessoryMasterNames(selected).map(name => `
      <label class="checkbox-label ${selected.includes(name) ? 'checked' : ''}"><input type="checkbox" value="${_mEsc(name)}" ${selected.includes(name) ? 'checked' : ''}> ${_mEsc(name)}</label>`).join('');
  }
  if (typeof _pepRenderAccessories === 'function') {
    const selected = [...document.querySelectorAll('#pep-accessories input:checked')].map(input => input.value === previousName ? nextName : input.value);
    _pepRenderAccessories(selected, Number(document.getElementById('pep-bracelet-qty')?.value || 1));
  }
  if (typeof _marketBuildFilterOptions === 'function') _marketBuildFilterOptions();
  if (typeof _marketRenderAccessoryOptions === 'function') {
    const selected = typeof _marketSelectedAccessories === 'function' ? _marketSelectedAccessories() : [];
    _marketRenderAccessoryOptions(selected.map(name => name === previousName ? nextName : name));
  }
  if (typeof renderMasterTabs === 'function') renderMasterTabs();
  if (typeof renderInventoryTable === 'function' && typeof _invSearched !== 'undefined' && _invSearched) renderInventoryTable();
  if (typeof marketRenderTable === 'function' && document.getElementById('page-market')?.classList.contains('hidden') === false) marketRenderTable();
}

const CONDITION_MASTER_STORAGE_KEY = 'inv_condition_master_v1';

function getConditionMasterRecords(extraCodes = []) {
  const records = (APP_DATA.conditions || []).map(record => ({ ...record }));
  (extraCodes || []).map(code => String(code || '').trim().toUpperCase()).filter(Boolean).forEach(code => {
    if (!records.some(record => record.code === code)) records.push({ code, name: code });
  });
  return records;
}

function getConditionName(code) {
  if (!code) return '';
  return getConditionMasterRecords([code]).find(record => record.code === code)?.name || code;
}

function resolveConditionCode(value) {
  const normalized = String(value || '').trim();
  if (!normalized) return '';
  const record = getConditionMasterRecords().find(item =>
    item.code.toLocaleLowerCase('ja') === normalized.toLocaleLowerCase('ja') ||
    item.name.toLocaleLowerCase('ja') === normalized.toLocaleLowerCase('ja')
  );
  return record?.code || normalized.toUpperCase();
}

function getNextConditionCode() {
  const max = (APP_DATA.conditions || []).reduce((current, record) => {
    const match = String(record.code || '').match(/^C([0-9]+)$/);
    return match ? Math.max(current, Number(match[1])) : current;
  }, 0);
  return `C${String(max + 1).padStart(2, '0')}`;
}

function populateConditionMasterSelect(id, options = {}) {
  const select = document.getElementById(id);
  if (!select) return;
  const emptyLabel = Object.prototype.hasOwnProperty.call(options, 'emptyLabel')
    ? options.emptyLabel
    : '-- 選択 --';
  const selected = Object.prototype.hasOwnProperty.call(options, 'selected')
    ? String(options.selected || '')
    : select.value;
  const records = getConditionMasterRecords(options.extraCodes || []);
  const emptyOption = emptyLabel === null ? '' : `<option value="">${_mEsc(emptyLabel)}</option>`;
  select.innerHTML = emptyOption + records.map(record => {
    const label = options.labelMode === 'name' ? record.name : `${record.code} — ${record.name}`;
    return `<option value="${_mEsc(record.code)}">${_mEsc(label)}</option>`;
  }).join('');
  if (records.some(record => record.code === selected)) select.value = selected;
  else if (emptyLabel !== null) select.value = '';
}

function _conditionUsageCount(code) {
  let count = (APP_DATA.inventory || []).filter(item => item.condition === code).length;
  count += (APP_DATA.marketPrices || []).filter(item => item.condition === code).length;
  count += (APP_DATA.purchaseSlips || []).reduce((total, slip) =>
    total + (slip.lines || []).filter(line => line.productDetail?.condition === code).length, 0);
  count += (APP_DATA.boxes || []).reduce((total, box) =>
    total + (box.items || []).filter(item => item.condition === code).length, 0);
  if (typeof _peSlipData !== 'undefined' && _peSlipData) {
    count += (_peSlipData.lines || []).filter(line => line.productDetail?.condition === code).length;
  }
  return count;
}

function loadConditionMasterDirectory() {
  if (!shouldUseBrowserMasterStorage()) return;
  try {
    const stored = JSON.parse(localStorage.getItem(CONDITION_MASTER_STORAGE_KEY) || 'null');
    const records = Array.isArray(stored) ? stored : stored?.records;
    if (!Array.isArray(records)) return;
    const validated = records
      .map(record => ({ code: String(record?.code || '').trim().toUpperCase(), name: String(record?.name || '').trim() }))
      .filter(record => /^C[0-9]+$/.test(record.code) && record.name)
      .filter((record, index, all) => all.findIndex(candidate => candidate.code === record.code) === index);
    if (validated.length > 0) APP_DATA.conditions = validated;
  } catch (error) {
    console.warn('コンディションマスタの保存データを読み込めませんでした', error);
  }
}

function persistConditionMasterDirectory() {
  if (!shouldUseBrowserMasterStorage()) return;
  localStorage.setItem(CONDITION_MASTER_STORAGE_KEY, JSON.stringify({
    records: getConditionMasterRecords(),
  }));
}

function refreshConditionMasterConsumers() {
  ['pu-condition', 'pep-condition', 'ie-condition', 'me-condition'].forEach(id => {
    const element = document.getElementById(id);
    populateConditionMasterSelect(id, {
      emptyLabel: '-- 選択 --',
      selected: element?.value || '',
      labelMode: id.startsWith('pep-') || id.startsWith('me-') ? 'name' : 'code-name',
    });
  });
  _buildInvFilterOptions();
  if (typeof _marketBuildFilterOptions === 'function') _marketBuildFilterOptions();
  if (typeof renderMasterTabs === 'function') renderMasterTabs();
  if (typeof renderInventoryTable === 'function' && typeof _invSearched !== 'undefined' && _invSearched) renderInventoryTable();
  if (typeof marketRenderTable === 'function' && document.getElementById('page-market')?.classList.contains('hidden') === false) marketRenderTable();
}

function _renameBuyerReferences(oldCode, newCode) {
  if (!oldCode || !newCode || oldCode === newCode) return;
  (APP_DATA.sales || []).forEach(record => {
    if (record.buyer === oldCode) record.buyer = newCode;
  });
  (APP_DATA.shipments || []).forEach(record => {
    if (record.destination === oldCode) record.destination = newCode;
    if (record.buyerCode === oldCode) record.buyerCode = newCode;
  });
  (APP_DATA.salesReturns || []).forEach(record => {
    if (record.buyer === oldCode) record.buyer = newCode;
  });
  (APP_DATA.guestAccounts || []).forEach(guest => {
    if (guest.buyerCode === oldCode) guest.buyerCode = newCode;
  });
  (APP_DATA.purchaseRequests || []).forEach(request => {
    if (request.buyerCode === oldCode) request.buyerCode = newCode;
  });
  (APP_DATA.inventory || []).forEach(item => {
    if (item.reservedForBuyerCode === oldCode) item.reservedForBuyerCode = newCode;
  });
  (APP_DATA.boxes || []).forEach(box => {
    box.publicTo = (box.publicTo || []).map(code => code === oldCode ? newCode : code);
  });
  (APP_DATA.publishedSnapshot?.boxes || []).forEach(box => {
    box.publicTo = (box.publicTo || []).map(code => code === oldCode ? newCode : code);
  });
  (APP_DATA.clientCompanies || []).forEach(company => {
    if (company.buyerCode === oldCode) company.buyerCode = newCode;
  });
  ['sl-buyer', 'sh-dest', 'slip-filter-party'].forEach(id => {
    const element = document.getElementById(id);
    if (element?.value === oldCode) element.dataset.buyerRenameValue = newCode;
  });
  if (typeof persistGuestBoxState === 'function') persistGuestBoxState();
  if (typeof persistGuestSnapshot === 'function') persistGuestSnapshot();
  if (typeof persistPurchaseRequests === 'function') persistPurchaseRequests();
}

function _buyerUsageCount(code) {
  let count = (APP_DATA.guestAccounts || []).filter(guest => guest.buyerCode === code).length;
  count += (APP_DATA.sales || []).filter(record => record.buyer === code).length;
  count += (APP_DATA.shipments || []).filter(record => record.destination === code).length;
  count += (APP_DATA.salesReturns || []).filter(record => record.buyer === code).length;
  count += (APP_DATA.purchaseRequests || []).filter(request => request.buyerCode === code).length;
  count += (APP_DATA.boxes || []).filter(box => (box.publicTo || []).includes(code)).length;
  count += (APP_DATA.publishedSnapshot?.boxes || []).filter(box => (box.publicTo || []).includes(code)).length;
  return count;
}

function refreshBuyerMasterConsumers(previousCode = '', nextCode = '') {
  const renamedSelection = element => {
    if (!element) return '';
    if (element.dataset.buyerRenameValue) {
      const value = element.dataset.buyerRenameValue;
      delete element.dataset.buyerRenameValue;
      return value;
    }
    return element.value === previousCode ? nextCode : element.value;
  };
  ['sl-buyer', 'sh-dest'].forEach(id => {
    const element = document.getElementById(id);
    if (typeof populateBuyerMasterSelect === 'function') {
      populateBuyerMasterSelect(id, { emptyLabel: '-- 選択 --', selected: renamedSelection(element), labelMode: 'code-name' });
    }
  });
  if (typeof rebuildSlipPartySelect === 'function' && typeof currentSlipTab !== 'undefined') rebuildSlipPartySelect(currentSlipTab);
  if (typeof renderBoxMatrix === 'function') renderBoxMatrix();
  if (typeof renderLoginInfoPage === 'function') renderLoginInfoPage();
  if (typeof refreshPasswordMasterDirectory === 'function') refreshPasswordMasterDirectory();
  if (typeof renderMasterTabs === 'function') renderMasterTabs();
  if (nextCode) {
    const buyer = (APP_DATA.buyers || []).find(record => record.code === nextCode);
    if (buyer) syncClientCompanyFromBuyer(buyer, { preferTrade: true });
  }
  reconcileClientCompanyDirectory({ persist: true });
}

const PRODUCT_SPEC_MASTER_STORAGE_KEY = 'inv_product_spec_master_v1';

function _productSpecConfig(type) {
  return type === 'movement'
    ? { key: 'movement', plural: 'movements', label: '駆動方式', codePattern: /^D[0-9]+$/ }
    : { key: 'material', plural: 'materials', label: '素材', codePattern: /^M[0-9]+$/ };
}

function getProductSpecMasterRecords(type, extraCodes = []) {
  const config = _productSpecConfig(type);
  const records = (APP_DATA[config.plural] || []).map(record => ({ ...record }));
  (extraCodes || []).map(code => String(code || '').trim()).filter(Boolean).forEach(code => {
    if (!records.some(record => record.code === code)) records.push({ code, name: code });
  });
  return records;
}

function getProductSpecName(type, code) {
  if (!code) return '';
  return getProductSpecMasterRecords(type, [code]).find(record => record.code === code)?.name || code;
}

function resolveProductSpecCode(type, value) {
  const normalized = String(value || '').trim();
  if (!normalized) return '';
  const record = getProductSpecMasterRecords(type).find(item =>
    item.code.toLocaleLowerCase('ja') === normalized.toLocaleLowerCase('ja') ||
    item.name.toLocaleLowerCase('ja') === normalized.toLocaleLowerCase('ja')
  );
  return record?.code || normalized.toUpperCase();
}

function populateProductSpecMasterSelect(id, type, options = {}) {
  const select = document.getElementById(id);
  if (!select) return;
  const emptyLabel = Object.prototype.hasOwnProperty.call(options, 'emptyLabel')
    ? options.emptyLabel
    : '-- 選択 --';
  const selected = Object.prototype.hasOwnProperty.call(options, 'selected')
    ? String(options.selected || '')
    : select.value;
  const records = getProductSpecMasterRecords(type, options.extraCodes || []);
  const emptyOption = emptyLabel === null ? '' : `<option value="">${_mEsc(emptyLabel)}</option>`;
  select.innerHTML = emptyOption + records.map(record => {
    const label = options.labelMode === 'name' ? record.name : `${record.code} — ${record.name}`;
    return `<option value="${_mEsc(record.code)}">${_mEsc(label)}</option>`;
  }).join('');
  if (records.some(record => record.code === selected)) select.value = selected;
  else if (emptyLabel !== null) select.value = '';
}

function _renameProductSpecReferences(type, oldCode, newCode) {
  if (!oldCode || !newCode || oldCode === newCode) return;
  const property = _productSpecConfig(type).key;
  (APP_DATA.inventory || []).forEach(item => {
    if (item[property] === oldCode) item[property] = newCode;
  });
  (APP_DATA.marketPrices || []).forEach(item => {
    if (item[property] === oldCode) item[property] = newCode;
  });
  (APP_DATA.purchaseSlips || []).forEach(slip => {
    (slip.lines || []).forEach(line => {
      if (line.productDetail?.[property] === oldCode) line.productDetail[property] = newCode;
    });
  });
  if (typeof _peSlipData !== 'undefined' && _peSlipData) {
    (_peSlipData.lines || []).forEach(line => {
      if (line.productDetail?.[property] === oldCode) line.productDetail[property] = newCode;
    });
  }
  const prefix = type === 'movement' ? 'movement' : 'material';
  [`pu-${prefix}`, `pep-${prefix}`, `ie-${prefix}`, `me-${prefix}`, `market-f-${prefix}`, `inv-f-${prefix}`].forEach(id => {
    const element = document.getElementById(id);
    if (element?.value === oldCode) element.dataset.productSpecRenameValue = newCode;
  });
}

function _productSpecUsageCount(type, code) {
  const property = _productSpecConfig(type).key;
  let count = (APP_DATA.inventory || []).filter(item => item[property] === code).length;
  count += (APP_DATA.marketPrices || []).filter(item => item[property] === code).length;
  count += (APP_DATA.purchaseSlips || []).reduce((total, slip) =>
    total + (slip.lines || []).filter(line => line.productDetail?.[property] === code).length, 0);
  if (typeof _peSlipData !== 'undefined' && _peSlipData) {
    count += (_peSlipData.lines || []).filter(line => line.productDetail?.[property] === code).length;
  }
  return count;
}

function loadProductSpecMasterDirectory() {
  APP_DATA.productSpecAliases = { material: {}, movement: {} };
  if (!shouldUseBrowserMasterStorage()) return;
  try {
    const stored = JSON.parse(localStorage.getItem(PRODUCT_SPEC_MASTER_STORAGE_KEY) || 'null');
    if (!stored || typeof stored !== 'object') return;
    ['material', 'movement'].forEach(type => {
      const config = _productSpecConfig(type);
      const records = stored[config.plural];
      if (Array.isArray(records)) {
        const validated = records
          .map(record => ({ code: String(record?.code || '').trim().toUpperCase(), name: String(record?.name || '').trim() }))
          .filter(record => config.codePattern.test(record.code) && record.name);
        if (validated.length > 0) {
          APP_DATA[config.plural] = validated.filter((record, index, all) =>
            all.findIndex(candidate => candidate.code === record.code) === index
          );
        }
      }
      const aliases = stored.aliases?.[type];
      if (aliases && typeof aliases === 'object') {
        APP_DATA.productSpecAliases[type] = { ...aliases };
        Object.entries(aliases).forEach(([oldCode, newCode]) => _renameProductSpecReferences(type, oldCode, newCode));
      }
    });
  } catch (error) {
    console.warn('素材・駆動方式マスタの保存データを読み込めませんでした', error);
  }
}

function persistProductSpecMasterDirectory() {
  if (!shouldUseBrowserMasterStorage()) return;
  localStorage.setItem(PRODUCT_SPEC_MASTER_STORAGE_KEY, JSON.stringify({
    materials: getProductSpecMasterRecords('material'),
    movements: getProductSpecMasterRecords('movement'),
    aliases: APP_DATA.productSpecAliases || { material: {}, movement: {} },
  }));
}

function refreshProductSpecMasterConsumers(type, previousCode = '', nextCode = '') {
  const prefix = type === 'movement' ? 'movement' : 'material';
  const renamedSelection = element => {
    if (!element) return '';
    if (element.dataset.productSpecRenameValue) {
      const value = element.dataset.productSpecRenameValue;
      delete element.dataset.productSpecRenameValue;
      return value;
    }
    return element.value === previousCode ? nextCode : element.value;
  };
  [`pu-${prefix}`, `pep-${prefix}`, `ie-${prefix}`, `me-${prefix}`].forEach(id => {
    const element = document.getElementById(id);
    populateProductSpecMasterSelect(id, type, {
      emptyLabel: '-- 選択 --',
      selected: renamedSelection(element),
      labelMode: id.startsWith('me-') || id.startsWith('pep-') ? 'name' : 'code-name',
    });
  });
  _buildInvFilterOptions();
  if (typeof _marketBuildFilterOptions === 'function') _marketBuildFilterOptions();
  if (typeof renderMasterTabs === 'function') renderMasterTabs();
  if (typeof renderInventoryTable === 'function' && typeof _invSearched !== 'undefined' && _invSearched) renderInventoryTable();
  if (typeof marketRenderTable === 'function' && document.getElementById('page-market')?.classList.contains('hidden') === false) marketRenderTable();
}

const MASTER_TABS = [
  { key: 'brand',     icon: '<i class="fa-solid fa-tag"></i>',            label: 'ブランド名',    data: () => APP_DATA.brandRecords || [] },
  { key: 'supplier',  icon: '<i class="fa-solid fa-industry"></i>',       label: '仕入先',        data: () => APP_DATA.suppliers },
  { key: 'staff',     icon: '<i class="fa-solid fa-user"></i>',           label: '仕入担当者',    data: () => APP_DATA.staffRecords || [] },
  { key: 'material',  icon: '<i class="fa-solid fa-gem"></i>',            label: '素材',          data: () => APP_DATA.materials },
  { key: 'movement',  icon: '<i class="fa-solid fa-gears"></i>',          label: '駆動方式',      data: () => APP_DATA.movements },
  { key: 'buyer',     icon: '<i class="fa-solid fa-building"></i>',       label: '販売先',        data: () => APP_DATA.buyers },
  { key: 'accessory', icon: '<i class="fa-solid fa-box"></i>',            label: '付属品',        data: () => APP_DATA.accessoryRecords || [] },
  { key: 'condition', icon: '<i class="fa-solid fa-star"></i>',           label: 'コンディション', data: () => APP_DATA.conditions },
  { key: 'fxrate',    icon: '<i class="fa-solid fa-coins"></i>',          label: '外貨レート',    data: () => APP_DATA.fxRates || [] },
  { key: 'box',       icon: '<i class="fa-solid fa-users-gear"></i>',     label: 'ゲスト管理',    data: () => typeof getGuestManagedBuyers === 'function' ? getGuestManagedBuyers() : APP_DATA.guestAccounts },
  // ── 管理者専用タブ ──
  { key: 'password',  icon: '<i class="fa-solid fa-key"></i>',            label: 'パスワード管理',    data: () => APP_DATA.users, adminOnly: true },
  { key: 'client',    icon: '<i class="fa-solid fa-handshake"></i>',      label: '取引先会社',        data: () => APP_DATA.clientCompanies || [], adminOnly: true },
  { key: 'company',   icon: '<i class="fa-solid fa-building-user"></i>',  label: '会社情報',          data: () => [], adminOnly: true },
  { key: 'dashboard', icon: '<i class="fa-solid fa-gauge"></i>',          label: 'ダッシュボード管理', data: () => [], adminOnly: true },
];

let currentMasterTab = 'brand';

function initMasterPage() {}

function init_master() {
  renderMasterTabs();
  switchMasterTab('brand');
}

function renderMasterTabs() {
  const list = document.getElementById('masterTabList');
  // adminのみに表示するタブはisAdmin()チェックでフィルタリング
  const visibleTabs = MASTER_TABS.filter(tab => !tab.adminOnly || isAdmin());
  list.innerHTML = visibleTabs.map(tab => {
    // 外貨レート・会社情報・パスワード管理・ダッシュボード管理はカウントを独自表示
    let count;
    if (tab.key === 'fxrate') {
      count = (APP_DATA.fxRates || []).length;
    } else if (tab.key === 'company' || tab.key === 'dashboard') {
      count = '—';
    } else if (tab.key === 'password') {
      count = APP_DATA.users.length;
    } else {
      count = tab.data().length;
    }
    return `
      <div class="master-tab-item ${tab.key === currentMasterTab ? 'active' : ''}"
        onclick="switchMasterTab('${tab.key}')">
        <span>${tab.icon}</span> ${tab.label}
        <span class="count-badge">${count}</span>
      </div>`;
  }).join('');
}

function switchMasterTab(key) {
  currentMasterTab = key;
  renderMasterTabs();
  const tab = MASTER_TABS.find(t => t.key === key);
  if (!tab) return;

  const data = tab.data();
  const area = document.getElementById('masterContentArea');

  // BOXタブはサイドメニューのゲスト管理と同じデータ・操作を描画
  if (key === 'box') {
    if (typeof renderBoxMasterTab === 'function') renderBoxMasterTab(area);
    return;
  }

  // 外貨レートタブ専用レンダリング
  if (key === 'fxrate') {
    renderFxRateTab(area);
    return;
  }

  // ===== パスワード管理タブ（マスタ内インライン描画） =====
  if (key === 'password') {
    renderPasswordMasterTab(area);
    return;
  }

  // ===== 取引先会社タブ（マスタ内インライン描画） =====
  if (key === 'client') {
    renderClientMasterTab(area);
    return;
  }

  // ===== 会社情報タブ（マスタ内インライン描画） =====
  if (key === 'company') {
    renderCompanyMasterTab(area);
    return;
  }

  // ===== ダッシュボード管理タブ（マスタ内インライン描画） =====
  if (key === 'dashboard') {
    renderDashboardMasterTab(area);
    return;
  }

  // ===== 編集機能付きテーブル生成 =====
  let tableHtml = '';
  if (key === 'buyer') {
    tableHtml = `
      <table class="data-table">
        <thead><tr><th>コード</th><th>名称</th><th>住所</th><th>連絡先</th><th>インボイス番号</th><th>取引区分</th><th>操作</th></tr></thead>
        <tbody>
          ${data.map((d, idx) => {
            const channel = typeof getBuyerChannel === 'function' ? getBuyerChannel(d.code) : { type: 'direct', label: '直取引（未発行）', guest: null };
            const channelBadge = channel.type === 'guest'
              ? `<span class="badge badge-stock"><i class="fa-solid fa-key"></i> ${_mEsc(channel.label)}${channel.guest?.id ? ` / ${_mEsc(channel.guest.id)}` : ''}</span>`
              : '<span class="badge" style="background:#fff7ed;color:#c2410c;"><i class="fa-solid fa-handshake"></i> 直取引（未発行）</span>';
            const guestAction = channel.type === 'guest'
              ? `<button class="btn btn-outline btn-sm" onclick="openLoginInfoModal('guest','${_mEsc(channel.guest.id)}')"><i class="fa-solid fa-key"></i> ゲスト編集</button>`
              : `<button class="btn btn-accent btn-sm" onclick="openGuestLoginForBuyer('${_mEsc(d.code)}')"><i class="fa-solid fa-key"></i> ゲスト発行</button>`;
            return `
            <tr data-buyer-code="${_mEsc(d.code)}" data-buyer-channel="${channel.type}">
              <td><code style="font-size:11px;">${_mEsc(d.code)}</code></td>
              <td style="font-weight:bold;">${_mEsc(d.name)}</td>
              <td style="font-size:12px;">${_mEsc(d.address || '—')}</td>
              <td style="font-size:12px;">${_mEsc(d.contact || '—')}</td>
              <td style="font-size:12px;">${_mEsc(d.invoice || '—')}</td>
              <td>${channelBadge}</td>
              <td><div style="display:flex;gap:4px;flex-wrap:wrap;">
                ${guestAction}
                <button class="btn btn-outline btn-sm" onclick="showEditMasterModal('buyer',${idx})"><i class="fa-solid fa-pen"></i> 編集</button>
                <button class="btn btn-ghost btn-sm" style="color:var(--danger);" onclick="showDeleteMasterModal('buyer',${idx})"><i class="fa-solid fa-trash"></i> 削除</button>
              </div></td>
            </tr>`;
          }).join('')}
        </tbody>
      </table>
    `;
  } else if (key === 'supplier') {
    tableHtml = `
      <table class="data-table">
        <thead><tr><th>コード</th><th>名称</th><th>住所</th><th>連絡先</th><th>インボイス番号</th><th>操作</th></tr></thead>
        <tbody>
          ${data.map((d, idx) => `
            <tr>
              <td><code style="font-size:11px;">${_mEsc(d.code)}</code></td>
              <td style="font-weight:bold;">${_mEsc(d.name)}</td>
              <td style="font-size:12px;">${_mEsc(d.address || '—')}</td>
              <td style="font-size:12px;">${_mEsc(d.contact || '—')}</td>
              <td style="font-size:12px;">${_mEsc(d.invoice || '—')}</td>
              <td><div style="display:flex;gap:4px;">
                <button class="btn btn-outline btn-sm" onclick="showEditMasterModal('${key}',${idx})"><i class="fa-solid fa-pen"></i> 編集</button>
                <button class="btn btn-ghost btn-sm" style="color:var(--danger);" onclick="showDeleteMasterModal('${key}',${idx})"><i class="fa-solid fa-trash"></i> 削除</button>
              </div></td>
            </tr>
          `).join('')}
        </tbody>
      </table>
    `;
  } else {
    tableHtml = `
      <table class="data-table">
        <thead><tr><th>コード</th><th>名称</th><th>操作</th></tr></thead>
        <tbody>
          ${data.map((d, idx) => `
            <tr>
              <td><code style="font-size:11px;">${_mEsc(d.code || '—')}</code></td>
              <td>${_mEsc(d.name || d)}</td>
              <td><div style="display:flex;gap:4px;">
                <button class="btn btn-outline btn-sm" onclick="showEditMasterModal('${key}',${idx})"><i class="fa-solid fa-pen"></i> 編集</button>
                <button class="btn btn-ghost btn-sm" style="color:var(--danger);" onclick="showDeleteMasterModal('${key}',${idx})"><i class="fa-solid fa-trash"></i> 削除</button>
              </div></td>
            </tr>
          `).join('')}
        </tbody>
      </table>
    `;
  }

  area.innerHTML = `
    <div class="master-content">
      <div style="display:flex;align-items:center;gap:12px;margin-bottom:16px;">
        <h3 style="font-size:15px;font-weight:bold;color:var(--primary);">${tab.icon} ${tab.label}</h3>
        <button class="btn btn-accent btn-sm" onclick="showAddMasterModal('${key}')"><i class="fa-solid fa-plus"></i> 新規追加</button>
      </div>
      ${key === 'brand' ? `
        <div style="display:flex;align-items:flex-start;gap:9px;padding:10px 12px;margin-bottom:14px;background:#eff6ff;border:1px solid #bfdbfe;border-radius:8px;color:#1e3a5f;font-size:12px;line-height:1.6;">
          <i class="fa-solid fa-link" style="margin-top:3px;"></i>
          <div><strong>共通ブランドマスタ</strong><br>相場表・在庫一覧の検索と、仕入登録・商品登録の選択肢へ同じ内容が反映されます。</div>
        </div>` : key === 'supplier' ? `
        <div style="display:flex;align-items:flex-start;gap:9px;padding:10px 12px;margin-bottom:14px;background:#eff6ff;border:1px solid #bfdbfe;border-radius:8px;color:#1e3a5f;font-size:12px;line-height:1.6;">
          <i class="fa-solid fa-link" style="margin-top:3px;"></i>
          <div><strong>共通仕入先マスタ</strong><br>相場表・在庫一覧の検索と、仕入登録・商品登録の選択肢へ同じ内容が反映されます。</div>
        </div>` : key === 'staff' ? `
        <div style="display:flex;align-items:flex-start;gap:9px;padding:10px 12px;margin-bottom:14px;background:#eff6ff;border:1px solid #bfdbfe;border-radius:8px;color:#1e3a5f;font-size:12px;line-height:1.6;">
          <i class="fa-solid fa-link" style="margin-top:3px;"></i>
          <div><strong>共通仕入担当者・作業者マスタ</strong><br>当社の作業者アカウントと同じ仕入担当者コードで連動し、相場表・在庫一覧の検索、仕入登録・商品登録の選択肢へ反映されます。</div>
        </div>` : key === 'buyer' ? `
        <div style="display:flex;align-items:flex-start;gap:9px;padding:10px 12px;margin-bottom:14px;background:#eff6ff;border:1px solid #bfdbfe;border-radius:8px;color:#1e3a5f;font-size:12px;line-height:1.6;">
          <i class="fa-solid fa-link" style="margin-top:3px;"></i>
          <div><strong>共通販売先マスタ</strong><br>直取引のみの販売先も登録できます。ゲストログインを発行した会社は必ずこの販売先へ追加・連携され、BOX公開、出荷伝票、売上伝票で同じ販売先コードを使用します。</div>
        </div>` : key === 'accessory' ? `
        <div style="display:flex;align-items:flex-start;gap:9px;padding:10px 12px;margin-bottom:14px;background:#eff6ff;border:1px solid #bfdbfe;border-radius:8px;color:#1e3a5f;font-size:12px;line-height:1.6;">
          <i class="fa-solid fa-link" style="margin-top:3px;"></i>
          <div><strong>共通付属品マスタ</strong><br>在庫一覧・相場表の検索と、仕入登録・商品登録・各編集画面のチェック項目へ同じコードと名称が反映されます。</div>
        </div>` : key === 'condition' ? `
        <div style="display:flex;align-items:flex-start;gap:9px;padding:10px 12px;margin-bottom:14px;background:#eff6ff;border:1px solid #bfdbfe;border-radius:8px;color:#1e3a5f;font-size:12px;line-height:1.6;">
          <i class="fa-solid fa-link" style="margin-top:3px;"></i>
          <div><strong>共通コンディションマスタ</strong><br>在庫一覧・相場表の検索と、仕入登録・商品登録・各編集画面の選択肢へ同じ固定コードと名称が反映されます。</div>
        </div>` : key === 'material' || key === 'movement' ? `
        <div style="display:flex;align-items:flex-start;gap:9px;padding:10px 12px;margin-bottom:14px;background:#eff6ff;border:1px solid #bfdbfe;border-radius:8px;color:#1e3a5f;font-size:12px;line-height:1.6;">
          <i class="fa-solid fa-link" style="margin-top:3px;"></i>
          <div><strong>共通${key === 'material' ? '素材' : '駆動方式'}マスタ</strong><br>相場表・在庫一覧の検索と、仕入登録・商品登録・各編集画面の選択肢へ同じコードと名称が反映されます。</div>
        </div>` : ''}
      <div class="data-table-wrapper">${tableHtml}</div>
    </div>
  `;
}

// =====================================================
// マスタ編集モーダル — 共通実装
// =====================================================

/** 内部状態: 現在編集中のキー・インデックス・モード */
let _masterEditState = { key: null, idx: null, mode: null }; // mode: 'edit' | 'add'
let _masterDeleteState = { key: null, idx: null };

/** HTML エスケープ（マスタ表示専用） */
function _mEsc(str) {
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

/**
 * マスタデータを「生の配列」として返す
 * brand/staff/accessory は安定したコード付きレコードで管理
 */
function _getMasterRawArray(key) {
  switch (key) {
    case 'brand':     return APP_DATA.brandRecords || [];
    case 'supplier':  return APP_DATA.suppliers;
    case 'staff':     return APP_DATA.staffRecords || [];
    case 'material':  return APP_DATA.materials;
    case 'movement':  return APP_DATA.movements;
    case 'buyer':     return APP_DATA.buyers;
    case 'accessory': return APP_DATA.accessoryRecords || [];
    case 'condition': return APP_DATA.conditions;
    default:          return [];
  }
}

/**
 * 各マスタの「フォーム定義」を返す
 * fields: [ { id, label, type('text'|'select'), required, options?, placeholder? } ]
 */
function _getMasterFormDef(key) {
  const defs = {
    brand: {
      label: 'ブランド名', icon: '<i class="fa-solid fa-tag"></i>',
      fields: [
        { id: 'code', label: 'ブランドコード', type: 'text', required: true, readonly: true, placeholder: '自動採番' },
        { id: 'name', label: 'ブランド名', type: 'text', required: true, placeholder: '例: ロレックス' },
      ],
      getValues: (arr, idx) => ({ ...arr[idx] }),
      applyNew:  (arr, vals) => {
        arr.push({ code: vals.code, name: vals.name });
        _brandNextSequence = Math.max(_brandNextSequence, _brandCodeSequence(vals.code) + 1);
        _syncLegacyBrandNames();
      },
      applyEdit: (arr, idx, vals) => {
        const old = { ...arr[idx] };
        arr[idx] = { code: old.code, name: vals.name };
        _renameBrandReferences(old.name, vals.name, old.code);
        if (!APP_DATA.brandAliases) APP_DATA.brandAliases = {};
        Object.keys(APP_DATA.brandAliases).forEach(sourceName => {
          if (APP_DATA.brandAliases[sourceName] === old.name) APP_DATA.brandAliases[sourceName] = vals.name;
        });
        APP_DATA.brandAliases[old.name] = vals.name;
        _syncLegacyBrandNames();
      },
      applyDelete: (arr, idx) => {
        const deleted = { ...arr[idx] };
        arr.splice(idx, 1);
        if (APP_DATA.brandAliases) {
          Object.keys(APP_DATA.brandAliases).forEach(sourceName => {
            if (sourceName === deleted.name || APP_DATA.brandAliases[sourceName] === deleted.name) {
              delete APP_DATA.brandAliases[sourceName];
            }
          });
        }
        _syncLegacyBrandNames();
      },
    },
    staff: {
      label: '仕入担当者', icon: '<i class="fa-solid fa-user"></i>',
      fields: [
        { id: 'code', label: '仕入担当者コード', type: 'text', required: true, readonly: true, placeholder: '自動採番' },
        { id: 'name', label: '担当者名', type: 'text', required: true, placeholder: '例: 山本 太郎' },
      ],
      getValues: (arr, idx) => ({ ...arr[idx] }),
      applyNew:  (arr, vals) => {
        const record = { code: vals.code, name: vals.name };
        arr.push(record);
        _syncLegacyStaffNames();
        _ensureWorkerAccountForStaff(record);
      },
      applyEdit: (arr, idx, vals) => {
        const old = { ...arr[idx] };
        arr[idx] = { code: vals.code, name: vals.name };
        _renameStaffReferences(old.name, vals.name);
        if (!APP_DATA.staffAliases) APP_DATA.staffAliases = {};
        if (old.name !== vals.name) APP_DATA.staffAliases[old.name] = vals.name;
        const linkedUser = (APP_DATA.users || []).find(user => user.staffCode === old.code);
        if (linkedUser) {
          linkedUser.staffCode = vals.code;
          linkedUser.name = vals.name;
          linkedUser.avatar = vals.name.slice(0, 1);
          linkedUser.companyType = 'own';
          linkedUser.company = _ownCompanyName();
        } else {
          _ensureWorkerAccountForStaff(arr[idx]);
        }
        _syncLegacyStaffNames();
      },
      applyDelete: (arr, idx) => {
        const record = arr[idx];
        arr.splice(idx, 1);
        APP_DATA.users = (APP_DATA.users || []).filter(user => user.staffCode !== record.code);
        _syncLegacyStaffNames();
      },
    },
    accessory: {
      label: '付属品', icon: '<i class="fa-solid fa-box"></i>',
      fields: [
        { id: 'code', label: '付属品コード', type: 'text', required: true, readonly: true, placeholder: '自動採番' },
        { id: 'name', label: '付属品名（大文字英語）', type: 'text', required: true, placeholder: '例: BOX' },
      ],
      getValues: (arr, idx) => ({ ...arr[idx] }),
      applyNew:  (arr, vals) => { arr.push({ code: vals.code, name: vals.name.toUpperCase() }); },
      applyEdit: (arr, idx, vals) => {
        const oldName = arr[idx].name;
        const newName = vals.name.toUpperCase();
        arr[idx] = { code: vals.code, name: newName };
        _renameAccessoryReferences(oldName, newName);
        if (!APP_DATA.accessoryAliases) APP_DATA.accessoryAliases = {};
        Object.keys(APP_DATA.accessoryAliases).forEach(sourceName => {
          if (APP_DATA.accessoryAliases[sourceName] === oldName) APP_DATA.accessoryAliases[sourceName] = newName;
        });
        if (oldName !== newName) APP_DATA.accessoryAliases[oldName] = newName;
      },
      applyDelete: (arr, idx) => {
        const deletedName = arr[idx].name;
        arr.splice(idx, 1);
        if (APP_DATA.accessoryAliases) {
          Object.keys(APP_DATA.accessoryAliases).forEach(sourceName => {
            if (sourceName === deletedName || APP_DATA.accessoryAliases[sourceName] === deletedName) {
              delete APP_DATA.accessoryAliases[sourceName];
            }
          });
        }
      },
    },
    material: {
      label: '素材', icon: '<i class="fa-solid fa-gem"></i>',
      fields: [
        { id: 'code', label: 'コード', type: 'text', required: true, placeholder: '例: M07' },
        { id: 'name', label: '素材名', type: 'text', required: true, placeholder: '例: チタン合金' },
      ],
      getValues: (arr, idx) => ({ code: arr[idx].code, name: arr[idx].name }),
      applyNew:  (arr, vals) => { arr.push({ code: vals.code, name: vals.name }); },
      applyEdit: (arr, idx, vals) => {
        const oldCode = arr[idx].code;
        arr[idx] = { code: vals.code, name: vals.name };
        if (oldCode !== vals.code) {
          _renameProductSpecReferences('material', oldCode, vals.code);
          if (!APP_DATA.productSpecAliases) APP_DATA.productSpecAliases = { material: {}, movement: {} };
          const aliases = APP_DATA.productSpecAliases.material || (APP_DATA.productSpecAliases.material = {});
          Object.keys(aliases).forEach(sourceCode => {
            if (aliases[sourceCode] === oldCode) aliases[sourceCode] = vals.code;
          });
          aliases[oldCode] = vals.code;
        }
      },
      applyDelete: (arr, idx) => {
        const deletedCode = arr[idx].code;
        arr.splice(idx, 1);
        const aliases = APP_DATA.productSpecAliases?.material || {};
        Object.keys(aliases).forEach(sourceCode => {
          if (sourceCode === deletedCode || aliases[sourceCode] === deletedCode) delete aliases[sourceCode];
        });
      },
    },
    movement: {
      label: '駆動方式', icon: '<i class="fa-solid fa-gears"></i>',
      fields: [
        { id: 'code', label: 'コード', type: 'text', required: true, placeholder: '例: D06' },
        { id: 'name', label: '駆動方式名', type: 'text', required: true, placeholder: '例: ソーラー' },
      ],
      getValues: (arr, idx) => ({ code: arr[idx].code, name: arr[idx].name }),
      applyNew:  (arr, vals) => { arr.push({ code: vals.code, name: vals.name }); },
      applyEdit: (arr, idx, vals) => {
        const oldCode = arr[idx].code;
        arr[idx] = { code: vals.code, name: vals.name };
        if (oldCode !== vals.code) {
          _renameProductSpecReferences('movement', oldCode, vals.code);
          if (!APP_DATA.productSpecAliases) APP_DATA.productSpecAliases = { material: {}, movement: {} };
          const aliases = APP_DATA.productSpecAliases.movement || (APP_DATA.productSpecAliases.movement = {});
          Object.keys(aliases).forEach(sourceCode => {
            if (aliases[sourceCode] === oldCode) aliases[sourceCode] = vals.code;
          });
          aliases[oldCode] = vals.code;
        }
      },
      applyDelete: (arr, idx) => {
        const deletedCode = arr[idx].code;
        arr.splice(idx, 1);
        const aliases = APP_DATA.productSpecAliases?.movement || {};
        Object.keys(aliases).forEach(sourceCode => {
          if (sourceCode === deletedCode || aliases[sourceCode] === deletedCode) delete aliases[sourceCode];
        });
      },
    },
    condition: {
      label: 'コンディション', icon: '<i class="fa-solid fa-star"></i>',
      fields: [
        { id: 'code', label: 'コンディションコード', type: 'text', required: true, readonly: true, placeholder: '自動採番' },
        { id: 'name', label: 'コンディション名', type: 'text', required: true, placeholder: '例: ジャンク品 (J)' },
      ],
      getValues: (arr, idx) => ({ ...arr[idx] }),
      applyNew:  (arr, vals) => { arr.push({ code: vals.code, name: vals.name }); },
      applyEdit: (arr, idx, vals) => { arr[idx] = { code: vals.code, name: vals.name }; },
      applyDelete: (arr, idx) => { arr.splice(idx, 1); },
    },
    supplier: {
      label: '仕入先', icon: '<i class="fa-solid fa-industry"></i>',
      fields: [
        { id: 'code',    label: 'コード',         type: 'text', required: true,  placeholder: '例: S006' },
        { id: 'name',    label: '名称',           type: 'text', required: true,  placeholder: '例: ○○時計商事' },
        { id: 'address', label: '住所',           type: 'text', required: false, placeholder: '例: 東京都渋谷区' },
        { id: 'contact', label: '連絡先',         type: 'text', required: false, placeholder: '例: 03-0000-0000' },
        { id: 'invoice', label: 'インボイス番号', type: 'text', required: false, placeholder: '例: T0000000000' },
      ],
      getValues: (arr, idx) => ({ ...arr[idx] }),
      applyNew:  (arr, vals) => { arr.push({ code: vals.code, name: vals.name, address: vals.address || '', contact: vals.contact || '', invoice: vals.invoice || '' }); },
      applyEdit: (arr, idx, vals) => {
        const oldCode = arr[idx].code;
        arr[idx] = { code: vals.code, name: vals.name, address: vals.address || '', contact: vals.contact || '', invoice: vals.invoice || '' };
        if (oldCode !== vals.code) {
          _renameSupplierReferences(oldCode, vals.code);
          if (!APP_DATA.supplierAliases) APP_DATA.supplierAliases = {};
          Object.keys(APP_DATA.supplierAliases).forEach(sourceCode => {
            if (APP_DATA.supplierAliases[sourceCode] === oldCode) APP_DATA.supplierAliases[sourceCode] = vals.code;
          });
          APP_DATA.supplierAliases[oldCode] = vals.code;
        }
      },
      applyDelete: (arr, idx) => {
        const deletedCode = arr[idx].code;
        arr.splice(idx, 1);
        if (APP_DATA.supplierAliases) {
          Object.keys(APP_DATA.supplierAliases).forEach(sourceCode => {
            if (sourceCode === deletedCode || APP_DATA.supplierAliases[sourceCode] === deletedCode) {
              delete APP_DATA.supplierAliases[sourceCode];
            }
          });
        }
      },
    },
    buyer: {
      label: '販売先', icon: '<i class="fa-solid fa-building"></i>',
      fields: [
        { id: 'code',    label: 'コード',         type: 'text', required: true,  placeholder: '例: B005' },
        { id: 'name',    label: '名称',           type: 'text', required: true,  placeholder: '例: ○○商会' },
        { id: 'address', label: '住所',           type: 'text', required: false, placeholder: '例: 大阪府大阪市' },
        { id: 'contact', label: '連絡先',         type: 'text', required: false, placeholder: '例: 06-0000-0000' },
        { id: 'invoice', label: 'インボイス番号', type: 'text', required: false, placeholder: '例: T0000000000' },
      ],
      getValues: (arr, idx) => ({ ...arr[idx] }),
      applyNew:  (arr, vals) => { arr.push({ code: vals.code, name: vals.name, address: vals.address || '', contact: vals.contact || '', invoice: vals.invoice || '', email: '', guestManaged: false }); },
      applyEdit: (arr, idx, vals) => {
        const old = { ...arr[idx] };
        arr[idx] = { ...arr[idx], code: vals.code, name: vals.name, address: vals.address || '', contact: vals.contact || '', invoice: vals.invoice || '' };
        if (old.code !== vals.code) _renameBuyerReferences(old.code, vals.code);
        const linkedGuest = (APP_DATA.guestAccounts || []).find(guest => guest.buyerCode === vals.code);
        if (linkedGuest) linkedGuest.company = vals.name;
      },
      applyDelete: (arr, idx) => { arr.splice(idx, 1); },
    },
  };
  return defs[key] || null;
}

/** 編集モーダルを開く */
function showEditMasterModal(key, idx) {
  const def = _getMasterFormDef(key);
  if (!def) return;
  const arr = _getMasterRawArray(key);
  const vals = def.getValues(arr, idx);

  _masterEditState = { key, idx, mode: 'edit' };

  document.getElementById('masterEditModalIcon').innerHTML = def.icon;
  document.getElementById('masterEditModalTitle').textContent = `${def.label} — 編集`;

  document.getElementById('masterEditModalBody').innerHTML = _buildMasterForm(def.fields, vals);
  document.getElementById('masterEditModal').classList.remove('hidden');
}

/** 新規追加モーダルを開く */
function showAddMasterModal(key) {
  const def = _getMasterFormDef(key);
  if (!def) {
    showToast('info', '新規追加', `${key} の新規追加モーダルを開きます（モック）`);
    return;
  }

  _masterEditState = { key, idx: null, mode: 'add' };

  document.getElementById('masterEditModalIcon').innerHTML = def.icon;
  document.getElementById('masterEditModalTitle').textContent = `${def.label} — 新規追加`;

  const nextShortCode = (prefix, records) => {
    const width = prefix === 'M' || prefix === 'D' || prefix === 'C' ? 2 : 3;
    const max = (records || []).reduce((current, record) => {
      const match = String(record?.code || '').toUpperCase().match(new RegExp('^' + prefix.replace('-', '\\-') + '(\\d+)'));
      return match ? Math.max(current, Number(match[1])) : current;
    }, 0);
    return `${prefix}${String(max + 1).padStart(width, '0')}`;
  };
  const initialValues = {
    brand: { code: getNextBrandCode() },
    staff: { code: getNextStaffCode() },
    accessory: { code: getNextAccessoryCode() },
    condition: { code: getNextConditionCode() },
    material: { code: nextShortCode('M', APP_DATA.materials) },
    movement: { code: nextShortCode('D', APP_DATA.movements) },
    supplier: { code: _nextTradeMasterCode('S', APP_DATA.suppliers || []) },
    buyer: { code: _nextTradeMasterCode('B', APP_DATA.buyers || []) },
  }[key] || {};
  document.getElementById('masterEditModalBody').innerHTML = _buildMasterForm(def.fields, initialValues);
  document.getElementById('masterEditModal').classList.remove('hidden');
}

/** フォームHTMLを組み立てる */
function _buildMasterForm(fields, vals) {
  return fields.map(f => {
    const val = _mEsc(vals[f.id] || '');
    const req = f.required ? '<span class="required">*</span>' : '';
    const readonly = f.readonly || (Boolean(window.ZaikoAPI) && _masterEditState.mode === 'edit' && f.id === 'code');
    return `
      <div class="form-group" style="margin-bottom:14px;">
        <label class="form-label" style="font-size:12px;">${f.label} ${req}</label>
        <input
          type="text"
          id="medit-${f.id}"
          class="form-control"
          value="${val}"
          placeholder="${_mEsc(f.placeholder || '')}"
          ${readonly ? 'readonly aria-readonly="true"' : ''}
          style="font-size:13px;"
        >
      </div>`;
  }).join('');
}

/** 保存処理 */
async function saveMasterEdit() {
  const { key, idx, mode } = _masterEditState;
  if (!key) return;

  const def = _getMasterFormDef(key);
  if (!def) return;

  // フィールド値を収集
  const vals = {};
  let hasError = false;
  def.fields.forEach(f => {
    const el = document.getElementById(`medit-${f.id}`);
    if (!el) return;
    const v = el.value.trim();
    if (f.required && !v) {
      el.style.borderColor = 'var(--danger)';
      hasError = true;
    } else {
      el.style.borderColor = '';
      vals[f.id] = v;
    }
  });

  if (hasError) {
    showToast('error', '入力エラー', '必須項目を入力してください');
    return;
  }

  const arr = _getMasterRawArray(key);
  const previousBrandName = key === 'brand' && mode === 'edit' ? arr[idx].name : '';
  const previousSupplierCode = key === 'supplier' && mode === 'edit' ? arr[idx].code : '';
  const previousStaffName = key === 'staff' && mode === 'edit' ? arr[idx].name : '';
  const previousProductSpecCode = (key === 'material' || key === 'movement') && mode === 'edit' ? arr[idx].code : '';
  const previousBuyerCode = key === 'buyer' && mode === 'edit' ? arr[idx].code : '';
  const previousAccessoryName = key === 'accessory' && mode === 'edit' ? arr[idx].name : '';
  const previousConditionCode = key === 'condition' && mode === 'edit' ? arr[idx].code : '';

  if (key === 'brand') {
    vals.code = vals.code.toUpperCase();
    if (!/^BRD-[0-9]+$/.test(vals.code)) {
      showToast('error', 'ブランドコードを確認してください', 'BRD-001 の形式で自動採番されます');
      return;
    }
    if (mode === 'edit' && vals.code !== arr[idx].code) {
      showToast('error', 'ブランドコードは変更できません', '登録時に発行された固定コードを使用してください');
      return;
    }
    const duplicateCode = arr.some((brand, brandIndex) =>
      brandIndex !== idx && brand.code.toUpperCase() === vals.code
    );
    const normalized = vals.name.toLocaleLowerCase('ja');
    const duplicate = arr.some((brand, brandIndex) =>
      brandIndex !== idx && brand.name.trim().toLocaleLowerCase('ja') === normalized
    );
    if (duplicateCode || duplicate) {
      showToast('error', duplicateCode ? 'ブランドコードが重複しています' : 'ブランド名が重複しています',
        duplicateCode ? '固定コードの採番状態を確認してください' : '登録済みではないブランド名を入力してください');
      return;
    }
  }

  if (key === 'supplier') {
    vals.code = vals.code.toUpperCase();
    if (!/^[A-Z0-9_-]+$/.test(vals.code)) {
      showToast('error', '仕入先コードを確認してください', '半角英数字・ハイフン・アンダースコアで入力してください');
      return;
    }
    const duplicateCode = arr.some((supplier, supplierIndex) =>
      supplierIndex !== idx && supplier.code.toUpperCase() === vals.code
    );
    const normalizedName = vals.name.toLocaleLowerCase('ja');
    const duplicateName = arr.some((supplier, supplierIndex) =>
      supplierIndex !== idx && supplier.name.trim().toLocaleLowerCase('ja') === normalizedName
    );
    if (duplicateCode || duplicateName) {
      showToast('error', '仕入先が重複しています', duplicateCode ? '登録済みではない仕入先コードを入力してください' : '同じ名称の仕入先が登録されています');
      return;
    }
  }

  if (key === 'staff') {
    vals.code = vals.code.toUpperCase();
    if (!/^STF-[0-9]+$/.test(vals.code)) {
      showToast('error', '仕入担当者コードを確認してください', 'STF-001 の形式で入力してください');
      return;
    }
    const duplicateCode = arr.some((record, recordIndex) => recordIndex !== idx && record.code === vals.code);
    const normalizedName = vals.name.toLocaleLowerCase('ja');
    const duplicateName = arr.some((record, recordIndex) =>
      recordIndex !== idx && record.name.trim().toLocaleLowerCase('ja') === normalizedName
    );
    if (duplicateCode || duplicateName) {
      showToast('error', '仕入担当者が重複しています', duplicateCode ? '登録済みではない仕入担当者コードを入力してください' : '同じ担当者名が登録されています');
      return;
    }
  }

  if (key === 'material' || key === 'movement') {
    const config = _productSpecConfig(key);
    vals.code = vals.code.toUpperCase();
    if (!config.codePattern.test(vals.code)) {
      showToast('error', `${config.label}コードを確認してください`, `${key === 'material' ? 'M07' : 'D06'} の形式で入力してください`);
      return;
    }
    const duplicateCode = arr.some((record, recordIndex) => recordIndex !== idx && record.code.toUpperCase() === vals.code);
    const normalizedName = vals.name.toLocaleLowerCase('ja');
    const duplicateName = arr.some((record, recordIndex) =>
      recordIndex !== idx && record.name.trim().toLocaleLowerCase('ja') === normalizedName
    );
    if (duplicateCode || duplicateName) {
      showToast('error', `${config.label}が重複しています`, duplicateCode ? `登録済みではない${config.label}コードを入力してください` : `同じ名称の${config.label}が登録されています`);
      return;
    }
  }

  if (key === 'buyer') {
    vals.code = vals.code.toUpperCase();
    if (!/^B[0-9]+$/.test(vals.code)) {
      showToast('error', '販売先コードを確認してください', 'B005 の形式で入力してください');
      return;
    }
    const duplicateCode = arr.some((buyer, buyerIndex) => buyerIndex !== idx && buyer.code.toUpperCase() === vals.code);
    const normalizedName = vals.name.toLocaleLowerCase('ja');
    const duplicateName = arr.some((buyer, buyerIndex) =>
      buyerIndex !== idx && buyer.name.trim().toLocaleLowerCase('ja') === normalizedName
    );
    if (duplicateCode || duplicateName) {
      showToast('error', '販売先が重複しています', duplicateCode ? '登録済みではない販売先コードを入力してください' : '同じ名称の販売先が登録されています');
      return;
    }
  }

  if (key === 'accessory') {
    vals.code = vals.code.toUpperCase();
    vals.name = vals.name.toUpperCase();
    if (!/^ACC-[0-9]+$/.test(vals.code)) {
      showToast('error', '付属品コードを確認してください', 'ACC-007 の形式で入力してください');
      return;
    }
    const duplicateCode = arr.some((record, recordIndex) => recordIndex !== idx && record.code === vals.code);
    const duplicateName = arr.some((record, recordIndex) => recordIndex !== idx && record.name.toUpperCase() === vals.name);
    if (duplicateCode || duplicateName) {
      showToast('error', '付属品が重複しています', duplicateCode ? '登録済みではない付属品コードを入力してください' : '同じ名称の付属品が登録されています');
      return;
    }
  }

  if (key === 'condition') {
    vals.code = vals.code.toUpperCase();
    if (!/^C[0-9]+$/.test(vals.code)) {
      showToast('error', 'コンディションコードを確認してください', 'C08 の形式で入力してください');
      return;
    }
    const duplicateCode = arr.some((record, recordIndex) => recordIndex !== idx && record.code === vals.code);
    const normalizedName = vals.name.toLocaleLowerCase('ja');
    const duplicateName = arr.some((record, recordIndex) =>
      recordIndex !== idx && record.name.trim().toLocaleLowerCase('ja') === normalizedName
    );
    if (duplicateCode || duplicateName) {
      showToast('error', 'コンディションが重複しています', duplicateCode ? '登録済みではないコンディションコードを入力してください' : '同じ名称のコンディションが登録されています');
      return;
    }
  }

  if (window.ZaikoAPI) {
    try {
      const current = mode === 'edit' ? { ...arr[idx] } : null;
      const saved = await window.ZaikoAPI.saveMasterRecord(key, current, vals, mode);
      closeMasterEditModal();
      switchMasterTab(key);
      if (saved.temporaryPassword) {
        showToast('success', '仕入担当者と作業者を登録しました',
          `ログインID: ${saved.username} / 初期パスワード: ${saved.temporaryPassword}（パスワード管理で変更してください）`);
      } else {
        showToast('success', mode === 'add' ? '追加完了' : '保存完了', `${def.label}をDBへ保存しました`);
      }
    } catch (error) {
      showToast('error', `${def.label}を保存できませんでした`, error.message || '入力内容を確認してください');
    }
    return;
  }

  if (mode === 'add') {
    def.applyNew(arr, vals);
    showToast('success', '追加完了', `${def.label} に新しいレコードを追加しました`);
  } else {
    def.applyEdit(arr, idx, vals);
    showToast('success', '保存完了', `${def.label} を更新しました`);
  }

  closeMasterEditModal();
  // テーブルをリアルタイム再描画
  switchMasterTab(key);
  if (key === 'brand') {
    persistBrandMasterDirectory();
    refreshBrandMasterConsumers(previousBrandName, vals.name);
  }
  if (key === 'supplier') {
    persistSupplierMasterDirectory();
    refreshSupplierMasterConsumers(previousSupplierCode, vals.code);
  }
  if (key === 'staff') {
    persistStaffMasterDirectory();
    if (typeof persistLoginDirectory === 'function') persistLoginDirectory();
    refreshStaffMasterConsumers(previousStaffName, vals.name);
  }
  if (key === 'material' || key === 'movement') {
    persistProductSpecMasterDirectory();
    refreshProductSpecMasterConsumers(key, previousProductSpecCode, vals.code);
  }
  if (key === 'buyer') {
    if (typeof persistLoginDirectory === 'function') persistLoginDirectory();
    refreshBuyerMasterConsumers(previousBuyerCode, vals.code);
  }
  if (key === 'accessory') {
    persistAccessoryMasterDirectory();
    refreshAccessoryMasterConsumers(previousAccessoryName, vals.name);
  }
  if (key === 'condition') {
    persistConditionMasterDirectory();
    refreshConditionMasterConsumers(previousConditionCode, vals.code);
  }
}

/** 削除確認モーダルを開く */
function showDeleteMasterModal(key, idx) {
  const def = _getMasterFormDef(key);
  if (!def) return;
  const arr = _getMasterRawArray(key);
  const item = arr[idx];
  const name = typeof item === 'string' ? item : (item.name || item.code || String(idx));

  _masterDeleteState = { key, idx };

  document.getElementById('masterDeleteMsg').textContent =
    `「${name}」を削除してもよろしいですか？`;
  document.getElementById('masterDeleteModal').classList.remove('hidden');
}

/** 削除を実行する */
async function confirmMasterDelete() {
  const { key, idx } = _masterDeleteState;
  if (key === null) return;

  const def = _getMasterFormDef(key);
  if (!def) return;
  const arr = _getMasterRawArray(key);
  const deletedBrandName = key === 'brand' ? arr[idx].name : '';
  const deletedSupplierCode = key === 'supplier' ? arr[idx].code : '';
  const deletedStaff = key === 'staff' ? { ...arr[idx] } : null;
  const deletedProductSpecCode = key === 'material' || key === 'movement' ? arr[idx].code : '';
  const deletedBuyerCode = key === 'buyer' ? arr[idx].code : '';
  const deletedAccessoryName = key === 'accessory' ? arr[idx].name : '';
  const deletedConditionCode = key === 'condition' ? arr[idx].code : '';

  if (key === 'brand') {
    const usageCount = _brandUsageCount(deletedBrandName);
    if (usageCount > 0) {
      showToast('error', '使用中のブランドは削除できません', `在庫・相場表・仕入明細で ${usageCount} 件使用されています`);
      closeMasterDeleteModal();
      return;
    }
  }

  if (key === 'supplier') {
    const usageCount = _supplierUsageCount(deletedSupplierCode);
    if (usageCount > 0) {
      showToast('error', '使用中の仕入先は削除できません', `在庫・相場表・仕入伝票で ${usageCount} 件使用されています`);
      closeMasterDeleteModal();
      return;
    }
  }

  if (key === 'staff') {
    const usageCount = _staffUsageCount(deletedStaff.name);
    if (usageCount > 0) {
      showToast('error', '使用中の仕入担当者は削除できません', `在庫・相場表・仕入伝票で ${usageCount} 件使用されています`);
      closeMasterDeleteModal();
      return;
    }
  }

  if (key === 'material' || key === 'movement') {
    const usageCount = _productSpecUsageCount(key, deletedProductSpecCode);
    if (usageCount > 0) {
      const label = _productSpecConfig(key).label;
      showToast('error', `使用中の${label}は削除できません`, `在庫・相場表・仕入明細で ${usageCount} 件使用されています`);
      closeMasterDeleteModal();
      return;
    }
  }

  if (key === 'buyer') {
    const usageCount = _buyerUsageCount(deletedBuyerCode);
    if (usageCount > 0) {
      const guestIssued = (APP_DATA.guestAccounts || []).some(guest => guest.buyerCode === deletedBuyerCode);
      showToast('error', '使用中の販売先は削除できません', guestIssued
        ? 'ゲストログインが発行されています。ログインを解除しても取引履歴がある場合は削除できません'
        : `BOX公開・出荷・売上・返品で ${usageCount} 件使用されています`);
      closeMasterDeleteModal();
      return;
    }
  }

  if (key === 'accessory') {
    const usageCount = _accessoryUsageCount(deletedAccessoryName);
    if (usageCount > 0) {
      showToast('error', '使用中の付属品は削除できません', `在庫・相場表・仕入明細で ${usageCount} 件使用されています`);
      closeMasterDeleteModal();
      return;
    }
  }

  if (key === 'condition') {
    const usageCount = _conditionUsageCount(deletedConditionCode);
    if (usageCount > 0) {
      showToast('error', '使用中のコンディションは削除できません', `在庫・相場表・仕入明細・BOXで ${usageCount} 件使用されています`);
      closeMasterDeleteModal();
      return;
    }
  }

  if (window.ZaikoAPI) {
    try {
      await window.ZaikoAPI.deactivateMasterRecord(key, { ...arr[idx] });
      closeMasterDeleteModal();
      switchMasterTab(key);
      showToast('success', '無効化完了', `${def.label}をDB上で無効化しました`);
    } catch (error) {
      closeMasterDeleteModal();
      showToast('error', `${def.label}を無効化できませんでした`, error.message || '利用状況を確認してください');
    }
    return;
  }

  def.applyDelete(arr, idx);
  showToast('success', '削除完了', `${def.label} のレコードを削除しました`);

  closeMasterDeleteModal();
  switchMasterTab(key);
  if (key === 'brand') {
    persistBrandMasterDirectory();
    refreshBrandMasterConsumers(deletedBrandName, '');
  }
  if (key === 'supplier') {
    persistSupplierMasterDirectory();
    refreshSupplierMasterConsumers(deletedSupplierCode, '');
  }
  if (key === 'staff') {
    persistStaffMasterDirectory();
    if (typeof persistLoginDirectory === 'function') persistLoginDirectory();
    refreshStaffMasterConsumers(deletedStaff.name, '');
  }
  if (key === 'material' || key === 'movement') {
    persistProductSpecMasterDirectory();
    refreshProductSpecMasterConsumers(key, deletedProductSpecCode, '');
  }
  if (key === 'buyer') {
    if (typeof persistLoginDirectory === 'function') persistLoginDirectory();
    refreshBuyerMasterConsumers(deletedBuyerCode, '');
  }
  if (key === 'accessory') {
    persistAccessoryMasterDirectory();
    refreshAccessoryMasterConsumers(deletedAccessoryName, '');
  }
  if (key === 'condition') {
    persistConditionMasterDirectory();
    refreshConditionMasterConsumers(deletedConditionCode, '');
  }
}

/** 編集モーダルを閉じる */
function closeMasterEditModal() {
  document.getElementById('masterEditModal').classList.add('hidden');
  _masterEditState = { key: null, idx: null, mode: null };
}

/** 削除モーダルを閉じる */
function closeMasterDeleteModal() {
  document.getElementById('masterDeleteModal').classList.add('hidden');
  _masterDeleteState = { key: null, idx: null };
}

/**
 * 付属品マスタ変更後に商品登録フォームのチェックボックスを再構築する
 * 既存の選択状態を保ったまま最新マスタで再構築する
 */
function _rebuildAccessoryCheckboxes() {
  const accArea = document.getElementById('pu-accessories');
  if (!accArea) return;
  const selected = [...accArea.querySelectorAll('input:checked')].map(input => input.value);
  accArea.innerHTML = '';
  APP_DATA.accessories.forEach(a => {
    const lbl = document.createElement('label');
    lbl.className = `checkbox-label${selected.includes(a) ? ' checked' : ''}`;
    lbl.innerHTML = `<input type="checkbox" value="${_mEsc(a)}" ${selected.includes(a) ? 'checked' : ''}> ${_mEsc(a)}`;
    lbl.querySelector('input').addEventListener('change', function () {
      lbl.classList.toggle('checked', this.checked);
      if (this.value === 'BRACELET PARTS' && typeof _puToggleBraceletQty === 'function') _puToggleBraceletQty(this.checked);
    });
    accArea.appendChild(lbl);
  });
}

// =====================================================
// 外貨レート設定（マスタ登録タブ）
// =====================================================

function renderFxRateTab(area) {
  if (!APP_DATA.fxRates) APP_DATA.fxRates = [];
  const rates = APP_DATA.fxRates;

  // 換算例で使う参考金額
  const SAMPLE_AMOUNT = 1000;

  const cards = rates.map(fx => {
    const jpyEquiv = (SAMPLE_AMOUNT * fx.rate).toLocaleString('ja-JP');
    return `
      <div class="fx-rate-card" id="fx-card-${fx.code}">
        <!-- カードヘッダー -->
        <div class="fx-card-header">
          <div style="display:flex;align-items:center;gap:10px;">
            <span style="font-size:28px;line-height:1;">${fx.flag}</span>
            <div>
              <div style="font-size:16px;font-weight:800;color:#111827;">${fx.name}</div>
              <div style="font-size:12px;color:var(--text-muted);">${fx.code} / ${fx.symbol}</div>
            </div>
          </div>
          <div class="fx-rate-display" id="fx-display-${fx.code}">
            <span style="font-size:12px;color:var(--text-muted);">1 ${fx.symbol} =</span>
            <span style="font-size:22px;font-weight:900;color:var(--primary);margin:0 4px;">${fx.rate.toLocaleString('ja-JP', {minimumFractionDigits:2, maximumFractionDigits:2})}</span>
            <span style="font-size:13px;font-weight:600;color:var(--text-muted);">円</span>
          </div>
        </div>

        <!-- 入力エリア -->
        <div class="fx-input-area">
          <label class="form-label" style="font-size:12px;margin-bottom:6px;">
            <i class="fa-solid fa-pen"></i> レートを手入力（円）
          </label>
          <div style="display:flex;gap:8px;align-items:center;">
            <div style="position:relative;flex:1;">
              <span style="position:absolute;left:10px;top:50%;transform:translateY(-50%);font-size:12px;color:var(--text-muted);pointer-events:none;">1 ${fx.symbol} =</span>
              <input
                type="number"
                id="fx-input-${fx.code}"
                class="form-control fx-rate-input"
                value="${fx.rate}"
                min="0.01"
                step="0.01"
                placeholder="例: 155.00"
                style="padding-left:72px;font-size:15px;font-weight:700;height:42px;"
                oninput="previewFxRate('${fx.code}')"
                onkeydown="if(event.key==='Enter') saveFxRate('${fx.code}')"
              >
              <span style="position:absolute;right:10px;top:50%;transform:translateY(-50%);font-size:12px;color:var(--text-muted);pointer-events:none;">円</span>
            </div>
            <button class="btn btn-primary" onclick="saveFxRate('${fx.code}')"
              style="height:42px;padding:0 18px;white-space:nowrap;gap:6px;">
              <i class="fa-solid fa-check"></i> 更新
            </button>
          </div>

          <!-- リアルタイムプレビュー -->
          <div class="fx-preview-bar" id="fx-preview-${fx.code}">
            <i class="fa-solid fa-arrow-right-arrow-left" style="color:#6366f1;"></i>
            <span style="font-size:12px;color:var(--text-muted);">換算例:</span>
            <span style="font-size:13px;font-weight:700;color:#111827;">
              ${fx.symbol}${SAMPLE_AMOUNT.toLocaleString()} = ¥${jpyEquiv}
            </span>
          </div>
        </div>

        <!-- 最終更新情報 -->
        <div class="fx-updated-bar">
          <i class="fa-solid fa-clock-rotate-left" style="font-size:10px;"></i>
          最終更新: <b>${fx.updatedAt || '—'}</b>　更新者: <b>${fx.updatedBy || '—'}</b>
        </div>
      </div>`;
  }).join('');

  area.innerHTML = `
    <div class="master-content">
      <!-- ページタイトル -->
      <div style="display:flex;align-items:center;gap:12px;margin-bottom:6px;">
        <h3 style="font-size:15px;font-weight:bold;color:var(--primary);">
          <i class="fa-solid fa-coins"></i> 外貨レート設定
        </h3>
      </div>
      <p style="font-size:12px;color:var(--text-muted);margin-bottom:20px;">
        各通貨の円換算レートを手入力で設定します。設定したレートは仕入・売上登録画面の外貨換算に使用されます。
      </p>

      <!-- 通貨カード群 -->
      <div class="fx-cards-grid">${cards}</div>

      <!-- 一括更新日時メモ -->
      <div style="margin-top:24px;padding:12px 16px;background:#f8fafc;border:1px solid var(--border);border-radius:8px;font-size:12px;color:var(--text-muted);display:flex;align-items:center;gap:8px;">
        <i class="fa-solid fa-circle-info" style="color:#6366f1;"></i>
        レートは自動取得ではなく手動入力です。最新レートは各金融機関・為替情報サービスを参照してください。
      </div>
    </div>`;
}

// レートをリアルタイムプレビュー（入力中に換算例を更新）
function previewFxRate(code) {
  const input   = document.getElementById(`fx-input-${code}`);
  const preview = document.getElementById(`fx-preview-${code}`);
  if (!input || !preview) return;

  const newRate = parseFloat(input.value);
  if (isNaN(newRate) || newRate <= 0) {
    preview.innerHTML = `<i class="fa-solid fa-triangle-exclamation" style="color:#f59e0b;"></i>
      <span style="font-size:12px;color:#f59e0b;">有効なレートを入力してください</span>`;
    return;
  }

  const fx = (APP_DATA.fxRates || []).find(f => f.code === code);
  const sym = fx?.symbol || code;
  const SAMPLE = 1000;
  const equiv  = (SAMPLE * newRate).toLocaleString('ja-JP');

  preview.innerHTML = `
    <i class="fa-solid fa-arrow-right-arrow-left" style="color:#6366f1;"></i>
    <span style="font-size:12px;color:var(--text-muted);">換算例:</span>
    <span style="font-size:13px;font-weight:700;color:#111827;">${sym}${SAMPLE.toLocaleString()} = ¥${equiv}</span>`;
}

// レートを保存して画面を更新
async function saveFxRate(code) {
  const input = document.getElementById(`fx-input-${code}`);
  if (!input) return;

  const newRate = parseFloat(input.value);
  if (isNaN(newRate) || newRate <= 0) {
    showToast('error', '入力エラー', '有効なレートを入力してください（0より大きい数値）');
    return;
  }

  const fx = (APP_DATA.fxRates || []).find(f => f.code === code);
  if (!fx) return;

  if (window.ZaikoAPI) {
    try {
      await window.ZaikoAPI.saveExchangeRate(code, newRate);
      renderFxRateTab(document.getElementById('masterContentArea'));
      if (code === 'USD' && typeof _syncDashboardCurrencyUI === 'function') _syncDashboardCurrencyUI();
      showToast('success', 'USドルレートを更新しました', `1 USD = ¥${newRate.toLocaleString('ja-JP')}`);
    } catch (error) {
      showToast('error', '為替レートを保存できませんでした', error.message || '入力内容を確認してください');
    }
    return;
  }

  const oldRate = fx.rate;
  fx.rate = newRate;
  fx.updatedAt  = new Date().toLocaleString('ja-JP', {
    year:'numeric', month:'2-digit', day:'2-digit',
    hour:'2-digit', minute:'2-digit'
  }).replace(/\//g, '-');
  fx.updatedBy  = currentUser()?.name || '管理者';

  // 表示レートをリアルタイム更新
  const display = document.getElementById(`fx-display-${code}`);
  if (display) {
    display.innerHTML = `
      <span style="font-size:12px;color:var(--text-muted);">1 ${fx.symbol} =</span>
      <span style="font-size:22px;font-weight:900;color:var(--primary);margin:0 4px;">${newRate.toLocaleString('ja-JP', {minimumFractionDigits:2, maximumFractionDigits:2})}</span>
      <span style="font-size:13px;font-weight:600;color:var(--text-muted);">円</span>`;
  }

  // 最終更新バーを更新
  const card = document.getElementById(`fx-card-${code}`);
  if (card) {
    const bar = card.querySelector('.fx-updated-bar');
    if (bar) {
      bar.innerHTML = `
        <i class="fa-solid fa-clock-rotate-left" style="font-size:10px;"></i>
        最終更新: <b>${fx.updatedAt}</b>　更新者: <b>${fx.updatedBy}</b>`;
    }
  }

  // プレビューも更新
  previewFxRate(code);
  if (code === 'USD' && typeof _syncDashboardCurrencyUI === 'function') _syncDashboardCurrencyUI();

  const diff = newRate - oldRate;
  const arrow = diff > 0 ? '↑' : diff < 0 ? '↓' : '→';
  const diffColor = diff > 0 ? '#16a34a' : diff < 0 ? '#dc2626' : '#6b7280';
  showToast('success', `${fx.flag} ${fx.name} レートを更新しました`,
    `${oldRate.toFixed(2)} → <b style="color:${diffColor};">${newRate.toFixed(2)} 円 ${arrow}</b>`);
}
// =====================================================
let _perfWorkerPanelTemplates = null;

function _capturePerfPanelTemplates() {
  if (_perfWorkerPanelTemplates) return;
  _perfWorkerPanelTemplates = {};
  ['perf-supplier','perf-staff','perf-buyer'].forEach(id => {
    const panel = document.getElementById(id);
    if (panel) _perfWorkerPanelTemplates[id] = panel.innerHTML;
  });
}

function _restorePerfPanelTemplates() {
  if (!_perfWorkerPanelTemplates) return;
  Object.entries(_perfWorkerPanelTemplates).forEach(([id, html]) => {
    const panel = document.getElementById(id);
    if (panel && panel.querySelector('[data-perf-worker-gate]')) panel.innerHTML = html;
  });
}

function init_performance() {
  if (!isWorker()) {
    _setPerfApprovalStatus(null);
    _restorePerfPanelTemplates();
    _execRenderPerformance();
    return;
  }

  const approval = getLatestPerformanceApproval();
  if (approval?.status === 'approved') {
    _setPerfApprovalStatus(approval);
    _restorePerfPanelTemplates();
    _execRenderPerformance();
    return;
  }
  _showPerfWorkerGate(approval);
}

function _setPerfApprovalStatus(approval) {
  const status = document.getElementById('perf-approval-status');
  if (!status) return;
  status.className = 'perf-approval-status hidden';
  status.innerHTML = '';
  if (!approval || !isWorker()) return;

  const content = {
    pending: `<i class="fa-solid fa-hourglass-half"></i><div><strong>管理者の承認待ちです</strong><br>申請ID ${approval.id}。承認後、この画面で実績データを閲覧できます。</div>`,
    approved: `<i class="fa-solid fa-circle-check"></i><div><strong>閲覧が承認されています</strong><br>申請ID ${approval.id}${approval.approvedByName ? ` / 承認者 ${approval.approvedByName}` : ''}</div>`,
    revision: `<i class="fa-solid fa-rotate-left"></i><div><strong>申請が差し戻されました</strong><br>${_escHtml(approval.revisionComment || '内容を確認して再申請してください。')}</div>`,
  }[approval.status];
  if (!content) return;
  status.className = `perf-approval-status ${approval.status}`;
  status.innerHTML = content;
}

function _showPerfWorkerGate(approval = null) {
  _capturePerfPanelTemplates();
  _setPerfApprovalStatus(approval);
  const isPending = approval?.status === 'pending';
  const isRevision = approval?.status === 'revision';
  const heading = isPending ? '実績データの閲覧を申請済みです'
    : isRevision ? '差戻し内容を確認して再申請してください'
      : '実績データの閲覧には管理者の承認が必要です';
  const description = isPending
    ? `申請ID ${approval.id} を管理者が確認しています。`
    : isRevision
      ? _escHtml(approval.revisionComment || '管理者からの差戻し内容を確認してください。')
      : '「承認申請を送信する」ボタンから管理者へ申請してください。';
  const action = isPending ? '' : `
    <button class="btn btn-primary" onclick="renderPerformance()">
      <i class="fa-solid fa-paper-plane"></i> ${isRevision ? '再申請を送信する' : '承認申請を送信する'}
    </button>`;

  ['perf-supplier','perf-staff','perf-buyer'].forEach(id => {
    const panel = document.getElementById(id);
    if (!panel) return;
    panel.innerHTML = `
      <div data-perf-worker-gate="true" style="text-align:center;padding:48px 24px;">
        <div style="font-size:40px;color:#f59e0b;margin-bottom:16px;"><i class="fa-solid fa-lock"></i></div>
        <div style="font-size:16px;font-weight:700;color:var(--text);margin-bottom:8px;">${heading}</div>
        <div style="font-size:13px;color:var(--text-muted);margin-bottom:20px;">
          ${description}<br>管理者が承認後、実績データが表示されます。
        </div>
        ${action}
      </div>`;
  });
}

function switchPerfTab(tab, btn) {
  const perfPage = document.getElementById('page-performance');
  if (!perfPage) return;
  perfPage.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
  btn.classList.add('active');
  perfPage.querySelectorAll('.tab-panel').forEach(p => p.classList.remove('active'));
  const panel = document.getElementById('perf-' + tab);
  if (panel) panel.classList.add('active');
}

function renderPerformance() {
  // 作業者は承認済みの場合だけ集計を表示する
  if (isWorker()) {
    const existing = getLatestPerformanceApproval();
    if (existing?.status === 'approved') {
      _setPerfApprovalStatus(existing);
      _restorePerfPanelTemplates();
      _execRenderPerformance();
      return;
    }
    if (existing?.status === 'pending') {
      _showPerfWorkerGate(existing);
      showToast('info', '承認待ちです', `申請ID ${existing.id} を管理者が確認しています`);
      return;
    }
    const from = document.getElementById('perf-from')?.value || '';
    const to   = document.getElementById('perf-to')?.value   || '';
    const request = requestApproval(
      'performance', '実績管理 集計閲覧',
      {
        targetMonth: from && to ? `${from} 〜 ${to}` : '全期間',
        targetField: '集計実行',
        from, to,
        requestedBy: currentUser()?.name || '—',
      },
      `集計期間: ${from || '—'} 〜 ${to || '—'}`,
      null,
      existing?.status === 'revision' ? existing.id : null
    );
    _showPerfWorkerGate(request);
    showToast('info', '承認申請を送信しました', '管理者の承認後に集計結果が閲覧できます');
    return;
  }
  // 管理者は即時集計
  _execRenderPerformance();
}

function _execRenderPerformance() {
  renderSupplierPerf();
  renderStaffPerf();
  renderBuyerPerf();
}

// ── 集計期間クイック設定 ──
// key: 'month'=当月, '3m'=3ヶ月, '6m'=6ヶ月, '12m'=12ヶ月
function setPerfPeriod(key) {
  const today = new Date();
  const fmt   = d => d.toISOString().slice(0, 10);
  let from;
  if (key === 'month') {
    // 当月: 当月1日〜今日
    from = new Date(today.getFullYear(), today.getMonth(), 1);
  } else if (key === '3m') {
    // 3ヶ月: 3ヶ月前の月初〜今日
    const d = new Date(today.getFullYear(), today.getMonth() - 3, 1);
    from = d;
  } else if (key === '6m') {
    // 6ヶ月: 6ヶ月前の月初〜今日
    from = new Date(today.getFullYear(), today.getMonth() - 6, 1);
  } else if (key === '12m') {
    // 12ヶ月: 12ヶ月前の月初〜今日
    from = new Date(today.getFullYear(), today.getMonth() - 12, 1);
  } else {
    return;
  }
  const fromEl = document.getElementById('perf-from');
  const toEl   = document.getElementById('perf-to');
  if (fromEl) fromEl.value = fmt(from);
  if (toEl)   toEl.value   = fmt(today);
  // アクティブスタイル
  document.querySelectorAll('.perf-quick-btn').forEach(b => b.classList.remove('active'));
  const keyMap = { month:'当月', '3m':'3ヶ月', '6m':'6ヶ月', '12m':'12ヶ月' };
  document.querySelectorAll('.perf-quick-btn').forEach(b => {
    if (b.textContent.trim() === keyMap[key]) b.classList.add('active');
  });
  // 日付設定後、即時集計実行
  const performanceApproval = isWorker() ? getLatestPerformanceApproval() : null;
  if (!isWorker() || performanceApproval?.status === 'approved') _execRenderPerformance();
}

// ── 状態変数 ──
let _staffMetric   = 'sales';   // 'sales' | 'profit'
let _buyerMetric   = 'sales';   // 'sales' | 'profit'
let _staffPerfData = null;      // キャッシュ
let _buyerPerfData = null;      // キャッシュ

function setStaffMetric(m) {
  _staffMetric = m;
  document.getElementById('staffMetricSales')?.classList.toggle('active',  m === 'sales');
  document.getElementById('staffMetricProfit')?.classList.toggle('active', m === 'profit');
  if (_staffPerfData) _drawStaffChart(_staffPerfData);
}
function setBuyerMetric(m) {
  _buyerMetric = m;
  document.getElementById('buyerMetricSales')?.classList.toggle('active',  m === 'sales');
  document.getElementById('buyerMetricProfit')?.classList.toggle('active', m === 'profit');
  if (_buyerPerfData) _drawBuyerChart(_buyerPerfData);
}


function renderSupplierPerf() {
  const from = document.getElementById('perf-from')?.value || '';
  const to   = document.getElementById('perf-to')?.value   || '';
  const supplierMap = {};
  APP_DATA.inventory.forEach(item => {
    if (from && item.purchaseDate < from) return;
    if (to   && item.purchaseDate > to)   return;
    const name = getSupplierName(item.supplier);
    if (!supplierMap[name]) supplierMap[name] = { count: 0, total: 0 };
    supplierMap[name].count++;
    supplierMap[name].total += item.purchasePrice;
  });

  const totalAll = Object.values(supplierMap).reduce((s, v) => s + v.total, 0);
  const labels = Object.keys(supplierMap);
  const vals = labels.map(k => supplierMap[k].total);
  const colors = ['#2980b9','#e67e22','#27ae60','#8e44ad','#e74c3c','#16a085','#d35400','#7f8c8d'];

  if (perfCharts.chart1) perfCharts.chart1.destroy();
  const ctx = document.getElementById('perfChart1');
  if (ctx) {
    perfCharts.chart1 = new Chart(ctx, {
      type: 'pie',
      data: { labels, datasets: [{ data: vals, backgroundColor: colors, borderWidth: 2 }] },
      options: { responsive: true, maintainAspectRatio: false, plugins: { legend: { position: 'right', labels: { font: { size: 11 }, boxWidth: 12 } } } }
    });
  }

  const tbody = document.getElementById('perfSupplierTable');
  if (tbody) {
    tbody.innerHTML = labels.length === 0
      ? `<tr><td colspan="4" style="text-align:center;color:var(--text-muted);padding:20px;">データがありません</td></tr>`
      : labels.map((name, i) => `
        <tr>
          <td><span style="display:inline-block;width:10px;height:10px;background:${colors[i%colors.length]};border-radius:2px;margin-right:6px;"></span>${name}</td>
          <td style="text-align:right;">${supplierMap[name].count}点</td>
          <td style="text-align:right;font-weight:bold;">${formatPrice(supplierMap[name].total)}</td>
          <td style="text-align:right;">${totalAll > 0 ? Math.round(supplierMap[name].total / totalAll * 1000) / 10 : 0}%</td>
        </tr>`).join('');
  }
}

function renderStaffPerf() {
  const from = document.getElementById('perf-from')?.value || '';
  const to   = document.getElementById('perf-to')?.value   || '';
  const colors = ['#2980b9','#e67e22','#27ae60','#8e44ad','#e74c3c','#16a085','#d35400','#7f8c8d'];

  // 担当者ごとのデータ集計
  const staffMap = {};
  const ensureStaff = name => {
    if (!staffMap[name]) staffMap[name] = {
      buyCount:0, buyTotal:0,           // 仕入れ件数・金額（期間内）
      sellCount:0, sellTotal:0,         // 売上件数・金額（出来高）
      actualCost:0,                     // 売上商品の実際の仕入原価
      plannedCost:0,                    // 売上商品の仕入予定額（purchasePlannedPrice or purchasePrice）
      stockCount:0, stockTotal:0,       // 現在在庫件数・金額（全期間・ステータス在庫中）
    };
  };

  // 仕入れ集計（期間フィルタ）
  APP_DATA.inventory.forEach(item => {
    const name = item.staff || '未設定';
    ensureStaff(name);
    // 期間内仕入れ
    if ((!from || item.purchaseDate >= from) && (!to || item.purchaseDate <= to)) {
      staffMap[name].buyCount++;
      staffMap[name].buyTotal += item.purchasePrice || 0;
    }
    // 在庫中・取置中の在庫額（期間問わず）
    if (['在庫中', '取置中'].includes(item.status)) {
      staffMap[name].stockCount++;
      staffMap[name].stockTotal += item.purchasePrice || 0;
    }
  });

  // 売上集計（期間内）
  APP_DATA.sales.forEach(sale => {
    if (from && sale.date < from) return;
    if (to   && sale.date > to)   return;
    (sale.items || []).forEach(it => {
      const inv  = APP_DATA.inventory.find(i => i.code === it.code);
      const name = inv?.staff || '未設定';
      ensureStaff(name);
      if (!it.returnType) {  // 返品・持ち帰り除外
        staffMap[name].sellCount++;
        staffMap[name].sellTotal  += it.salePrice || 0;
        staffMap[name].actualCost += convertJPYToSalePriceUSD(inv?.purchasePrice || 0);
        // 予想粗利用: purchasePlannedPrice があればそれ、なければ purchasePrice を使用
        staffMap[name].plannedCost += convertJPYToSalePriceUSD(inv?.purchasePlannedPrice || inv?.purchasePrice || 0);
      }
    });
  });

  // 消化率計算（選択期間内の仕入れのうち売上済み比率）
  const digestCutoff = from || (() => {
    const d = new Date();
    d.setMonth(d.getMonth() - 3);
    return d.toISOString().slice(0, 10);
  })();

  const digestMap = {};
  APP_DATA.inventory.forEach(item => {
    if (!item.purchaseDate || item.purchaseDate < digestCutoff) return;
    if (to && item.purchaseDate > to) return;
    const name = item.staff || '未設定';
    if (!digestMap[name]) digestMap[name] = { total: 0, sold: 0 };
    digestMap[name].total++;
    if (item.status === '売上済') digestMap[name].sold++;
  });

  const labels   = Object.keys(staffMap).sort();
  _staffPerfData = { staffMap, digestMap, labels, colors };

  // KPIカード（予想粗利 と 実粗利 を並べて表示）
  const totalBuy         = labels.reduce((s, n) => s + staffMap[n].buyTotal,     0);
  const totalSell        = labels.reduce((s, n) => s + staffMap[n].sellTotal,    0);
  const totalActualCost  = labels.reduce((s, n) => s + staffMap[n].actualCost,   0);
  const totalPlannedCost = labels.reduce((s, n) => s + staffMap[n].plannedCost,  0);
  const totalStock       = labels.reduce((s, n) => s + staffMap[n].stockTotal,   0);
  const totalActualProfit  = totalSell - totalActualCost;
  const totalPlannedProfit = totalSell - totalPlannedCost;
  const actualProfitRate   = totalSell > 0 ? Math.round(totalActualProfit  / totalSell * 1000) / 10 : 0;
  const plannedProfitRate  = totalSell > 0 ? Math.round(totalPlannedProfit / totalSell * 1000) / 10 : 0;
  const kpiEl = document.getElementById('perfStaffKpiRow');
  if (kpiEl) kpiEl.innerHTML = [
    { icon:'fa-file-import',   label:'仕入金額',   val:formatPrice(totalBuy),            sub:'期間内',                      color:'#2980b9' },
    { icon:'fa-dollar-sign',   label:'出来高（USD）', val:formatSalePrice(totalSell),          sub:'期間内売上',                  color:'#27ae60' },
    { icon:'fa-chart-line',    label:'予想粗利（USD）', val:formatSalePrice(totalPlannedProfit), sub:`予想粗利率 ${plannedProfitRate}%`, color: totalPlannedProfit >= 0 ? '#e67e22' : '#e74c3c' },
    { icon:'fa-circle-check',  label:'実粗利（USD）', val:formatSalePrice(totalActualProfit),  sub:`実粗利率 ${actualProfitRate}%`,   color: totalActualProfit  >= 0 ? '#16a085' : '#e74c3c' },
  ].map(k => `
    <div class="perf-kpi-card" style="border-top:3px solid ${k.color};">
      <div class="perf-kpi-icon" style="color:${k.color};"><i class="fa-solid ${k.icon}"></i></div>
      <div class="perf-kpi-label">${k.label}</div>
      <div class="perf-kpi-val">${k.val}</div>
      <div class="perf-kpi-sub">${k.sub}</div>
    </div>`).join('');

  _drawStaffChart(_staffPerfData);
  _drawDigestChart(_staffPerfData);

  // 明細テーブル（予想粗利 + 実粗利 を横並びで追加）
  const tbody = document.getElementById('perfStaffTable');
  if (tbody) {
    tbody.innerHTML = labels.length === 0
      ? `<tr><td colspan="10" style="text-align:center;color:var(--text-muted);padding:20px;">データがありません</td></tr>`
      : labels.map(name => {
          const d = staffMap[name];
          const actualProfit   = d.sellTotal - d.actualCost;
          const plannedProfit  = d.sellTotal - d.plannedCost;
          const profitRate     = d.sellTotal > 0 ? Math.round(actualProfit / d.sellTotal * 1000) / 10 : 0;
          const digestAll  = digestMap[name]?.total || 0;
          const digestSold = digestMap[name]?.sold  || 0;
          const digestRate = digestAll > 0 ? Math.round(digestSold / digestAll * 1000) / 10 : 0;
          const digestColor  = digestRate  >= 70 ? '#16a085' : digestRate  >= 40 ? '#e67e22' : '#e74c3c';
          const actualColor  = actualProfit  >= 0 ? '#16a085' : '#e74c3c';
          const plannedColor = plannedProfit >= 0 ? '#e67e22' : '#e74c3c';
          return `<tr>
            <td style="font-weight:600;">${name}</td>
            <td style="text-align:right;">${d.buyCount}点</td>
            <td style="text-align:right;">${formatPrice(d.buyTotal)}</td>
            <td style="text-align:right;">${d.sellCount}点</td>
            <td style="text-align:right;font-weight:bold;color:var(--primary);">${formatSalePrice(d.sellTotal)}</td>
            <td style="text-align:right;font-weight:bold;color:${plannedColor};" title="売価 − 仕入予定額（USD換算）">${formatSalePrice(plannedProfit)}</td>
            <td style="text-align:right;font-weight:bold;color:${actualColor};"  title="売価 − 実際の仕入額（USD換算）">${formatSalePrice(actualProfit)}</td>
            <td style="text-align:right;color:${actualColor};">${profitRate}%</td>
            <td style="text-align:right;color:#8e44ad;">${formatPrice(d.stockTotal)}</td>
            <td style="text-align:right;">
              <div style="display:flex;align-items:center;gap:6px;justify-content:flex-end;">
                <div class="digest-bar-wrap">
                  <div class="digest-bar-fill" style="width:${digestRate}%;background:${digestColor};"></div>
                </div>
                <span style="color:${digestColor};font-weight:bold;min-width:36px;">${digestRate}%</span>
              </div>
            </td>
          </tr>`;
        }).join('');
  }
}

function _drawStaffChart(data) {
  const { staffMap, labels, colors } = data;
  const vals = labels.map(n => _staffMetric === 'profit'
    ? staffMap[n].sellTotal - staffMap[n].actualCost
    : staffMap[n].sellTotal);
  const label = _staffMetric === 'profit' ? '粗利' : '出来高';
  const barColors = labels.map((_, i) => colors[i % colors.length]);

  if (perfCharts.chart2) perfCharts.chart2.destroy();
  const ctx = document.getElementById('perfChart2');
  if (!ctx) return;
  perfCharts.chart2 = new Chart(ctx, {
    type: 'bar',
    data: { labels, datasets: [{ label, data: vals, backgroundColor: barColors, borderRadius: 5 }] },
    options: {
      responsive: true, maintainAspectRatio: false,
      plugins: { legend: { display: false } },
      scales: {
        y: { ticks: { callback: v => '\u0024' + Number(v).toLocaleString('en-US'), font: { size: 10 } } },
        x: { ticks: { font: { size: 10 } } }
      }
    }
  });
}

function _drawDigestChart(data) {
  const { digestMap, labels, colors } = data;
  // 消化率チャートは _staffPerfData の digestMap をそのまま使用（選択期間に連動）
  const chartLabels = labels.filter(n => digestMap[n]);
  const rateVals  = chartLabels.map(n => digestMap[n].total > 0 ? Math.round(digestMap[n].sold / digestMap[n].total * 1000) / 10 : 0);
  const remainVals = rateVals.map(r => Math.max(0, 100 - r));

  if (perfCharts.chart2b) perfCharts.chart2b.destroy();
  const ctx = document.getElementById('perfChart2b');
  if (!ctx) return;
  perfCharts.chart2b = new Chart(ctx, {
    type: 'bar',
    data: {
      labels: chartLabels,
      datasets: [
        { label: '売上済', data: rateVals,   backgroundColor: '#27ae60', borderRadius: 4, stack: 's' },
        { label: '未消化', data: remainVals, backgroundColor: '#e0e0e0', borderRadius: 4, stack: 's' },
      ]
    },
    options: {
      responsive: true, maintainAspectRatio: false,
      plugins: { legend: { position: 'bottom', labels: { font: { size: 10 }, boxWidth: 10 } },
        tooltip: { callbacks: { label: ctx => `${ctx.dataset.label}: ${ctx.parsed.y}%` } } },
      scales: {
        y: { max: 100, ticks: { callback: v => v + '%', font: { size: 10 } } },
        x: { ticks: { font: { size: 10 } } }
      }
    }
  });
}

function renderBuyerPerf() {
  const from = document.getElementById('perf-from')?.value || '';
  const to   = document.getElementById('perf-to')?.value   || '';
  const colors = ['#2980b9','#e67e22','#27ae60','#8e44ad','#e74c3c','#16a085','#d35400','#7f8c8d'];

  const buyerMap = {};
  APP_DATA.sales.forEach(sale => {
    if (from && sale.date < from) return;
    if (to   && sale.date > to)   return;
    const name = getBuyerName(sale.buyer) || '直接販売';
    if (!buyerMap[name]) buyerMap[name] = { count: 0, sellTotal: 0, actualCost: 0, plannedCost: 0 };
    (sale.items || []).forEach(it => {
      if (it.returnType) return;  // 返品除外
      buyerMap[name].count++;
      buyerMap[name].sellTotal   += it.salePrice || 0;
      const inv = APP_DATA.inventory.find(i => i.code === it.code);
      buyerMap[name].actualCost  += convertJPYToSalePriceUSD(inv?.purchasePrice || 0);
      buyerMap[name].plannedCost += convertJPYToSalePriceUSD(inv?.purchasePlannedPrice || inv?.purchasePrice || 0);
    });
  });

  const labels   = Object.keys(buyerMap).sort((a, b) => buyerMap[b].sellTotal - buyerMap[a].sellTotal);
  const totalSell = labels.reduce((s, n) => s + buyerMap[n].sellTotal, 0);
  _buyerPerfData  = { buyerMap, labels, colors, totalSell };

  // KPIカード（予想粗利 と 実粗利 を並べて表示）
  const totalActualCost   = labels.reduce((s, n) => s + buyerMap[n].actualCost,  0);
  const totalPlannedCost  = labels.reduce((s, n) => s + buyerMap[n].plannedCost, 0);
  const totalActualProfit  = totalSell - totalActualCost;
  const totalPlannedProfit = totalSell - totalPlannedCost;
  const actualProfitRate   = totalSell > 0 ? Math.round(totalActualProfit  / totalSell * 1000) / 10 : 0;
  const plannedProfitRate  = totalSell > 0 ? Math.round(totalPlannedProfit / totalSell * 1000) / 10 : 0;
  const totalCount  = labels.reduce((s, n) => s + buyerMap[n].count, 0);
  const kpiEl = document.getElementById('perfBuyerKpiRow');
  if (kpiEl) kpiEl.innerHTML = [
    { icon:'fa-dollar-sign',   label:'総出来高（USD）', val:formatSalePrice(totalSell),           sub:'期間内売上合計',              color:'#27ae60' },
    { icon:'fa-chart-line',    label:'予想粗利（USD）', val:formatSalePrice(totalPlannedProfit),  sub:`予想粗利率 ${plannedProfitRate}%`, color: totalPlannedProfit >= 0 ? '#e67e22' : '#e74c3c' },
    { icon:'fa-circle-check',  label:'実粗利（USD）', val:formatSalePrice(totalActualProfit),   sub:`実粗利率 ${actualProfitRate}%`,   color: totalActualProfit  >= 0 ? '#16a085' : '#e74c3c' },
    { icon:'fa-box',           label:'販売点数',  val:`${totalCount}点`,                 sub:'期間内',                      color:'#2980b9' },
  ].map(k => `
    <div class="perf-kpi-card" style="border-top:3px solid ${k.color};">
      <div class="perf-kpi-icon" style="color:${k.color};"><i class="fa-solid ${k.icon}"></i></div>
      <div class="perf-kpi-label">${k.label}</div>
      <div class="perf-kpi-val">${k.val}</div>
      <div class="perf-kpi-sub">${k.sub}</div>
    </div>`).join('');

  _drawBuyerChart(_buyerPerfData);
  _drawBuyerCompareChart(_buyerPerfData);

  // 明細テーブル（予想粗利 + 実粗利 を横並びで追加）
  const tbody = document.getElementById('perfBuyerTable');
  if (tbody) {
    tbody.innerHTML = labels.length === 0
      ? `<tr><td colspan="8" style="text-align:center;color:var(--text-muted);padding:20px;">データがありません</td></tr>`
      : labels.map((name, i) => {
          const d = buyerMap[name];
          const actualProfit   = d.sellTotal - d.actualCost;
          const plannedProfit  = d.sellTotal - d.plannedCost;
          const profitRate     = d.sellTotal > 0 ? Math.round(actualProfit / d.sellTotal * 1000) / 10 : 0;
          const share          = totalSell > 0 ? Math.round(d.sellTotal / totalSell * 1000) / 10 : 0;
          const actualColor    = actualProfit  >= 0 ? '#16a085' : '#e74c3c';
          const plannedColor   = plannedProfit >= 0 ? '#e67e22' : '#e74c3c';
          return `<tr>
            <td><span style="display:inline-block;width:10px;height:10px;background:${colors[i%colors.length]};border-radius:2px;margin-right:6px;"></span><b>${name}</b></td>
            <td style="text-align:right;">${d.count}点</td>
            <td style="text-align:right;font-weight:bold;color:var(--primary);">${formatSalePrice(d.sellTotal)}</td>
            <td style="text-align:right;color:var(--text-muted);">${formatSalePrice(d.actualCost)}</td>
            <td style="text-align:right;font-weight:bold;color:${plannedColor};" title="売価 − 仕入予定額（USD換算）">${formatSalePrice(plannedProfit)}</td>
            <td style="text-align:right;font-weight:bold;color:${actualColor};"  title="売価 − 実際の仕入額（USD換算）">${formatSalePrice(actualProfit)}</td>
            <td style="text-align:right;color:${actualColor};">${profitRate}%</td>
            <td style="text-align:right;">${share}%</td>
          </tr>`;
        }).join('');
  }
}

function _drawBuyerChart(data) {
  const { buyerMap, labels, colors } = data;
  const vals = labels.map(n => _buyerMetric === 'profit'
    ? buyerMap[n].sellTotal - buyerMap[n].actualCost
    : buyerMap[n].sellTotal);

  if (perfCharts.chart3) perfCharts.chart3.destroy();
  const ctx = document.getElementById('perfChart3');
  if (!ctx) return;
  perfCharts.chart3 = new Chart(ctx, {
    type: 'doughnut',
    data: { labels, datasets: [{ data: vals, backgroundColor: colors, borderWidth: 2 }] },
    options: { responsive: true, maintainAspectRatio: false,
      plugins: { legend: { position: 'right', labels: { font: { size: 11 }, boxWidth: 12 } } } }
  });
}

function _drawBuyerCompareChart(data) {
  const { buyerMap, labels, colors } = data;
  const sellVals   = labels.map(n => buyerMap[n].sellTotal);
  const profitVals = labels.map(n => buyerMap[n].sellTotal - buyerMap[n].actualCost);

  if (perfCharts.chart3b) perfCharts.chart3b.destroy();
  const ctx = document.getElementById('perfChart3b');
  if (!ctx) return;
  perfCharts.chart3b = new Chart(ctx, {
    type: 'bar',
    data: {
      labels,
      datasets: [
        { label: '出来高', data: sellVals,   backgroundColor: 'rgba(41,128,185,0.7)',  borderRadius: 4 },
        { label: '粗利',   data: profitVals, backgroundColor: 'rgba(22,160,133,0.85)', borderRadius: 4 },
      ]
    },
    options: {
      responsive: true, maintainAspectRatio: false,
      plugins: { legend: { position: 'bottom', labels: { font: { size: 10 }, boxWidth: 10 } } },
      scales: {
        y: { ticks: { callback: v => '\u0024' + Number(v).toLocaleString('en-US'), font: { size: 10 } } },
        x: { ticks: { font: { size: 10 } } }
      }
    }
  });
}

// =====================================================
// 購入一覧
// =====================================================
// 購入一覧 初期化
// =====================================================
function init_purchase_list() {
  renderPurchaseRequests();
}

// =====================================================
// 購入リクエスト一覧をレンダリング
// =====================================================
function renderPurchaseRequests() {
  if (typeof syncPurchaseRequestPartyCodes === 'function' && syncPurchaseRequestPartyCodes() > 0) {
    persistPurchaseRequests();
  }
  const list = document.getElementById('purchaseRequestList');
  const pendingCount = APP_DATA.purchaseRequests.filter(r => r.status === '未対応').length;
  document.getElementById('pendingCount').textContent = pendingCount;
  document.getElementById('requestBadge').textContent = pendingCount;
  document.getElementById('requestBadge').style.display = pendingCount > 0 ? '' : 'none';

  if (APP_DATA.purchaseRequests.length === 0) {
    list.innerHTML = '<p style="text-align:center;color:var(--text-muted);padding:30px;">購入リクエストはありません</p>';
    return;
  }

  list.innerHTML = APP_DATA.purchaseRequests.map(r => {
    const statusCls = r.status === '未対応' ? 'status-new' : r.status === '保留中' ? 'status-hold' : 'status-done';
    const statusBadgeCls = r.status === '未対応' ? 's-new' : r.status === '保留中' ? 's-hold' : 's-done';
    const iconHtml = r.status === '未対応'
      ? '<i class="fa-solid fa-cart-shopping" style="color:var(--primary-light);"></i>'
      : r.status === '保留中'
      ? '<i class="fa-solid fa-clock" style="color:#f59e0b;"></i>'
      : '<i class="fa-solid fa-circle-check" style="color:var(--success);"></i>';

    // 承認済み商品の合計金額
    const approvedItems = r.items.filter(it => it.itemStatus === 'approved');
    const approvedTotal = approvedItems.reduce((s, it) => s + (it.salePrice || 0), 0);
    // 全商品の合計（pendingも含む）
    const requestTotal = r.items.reduce((s, it) => s + (it.salePrice || 0), 0);
    const displayTotal = approvedTotal > 0 ? approvedTotal : requestTotal;
    const totalLabel  = approvedTotal > 0 ? '承認済合計' : 'リクエスト合計';

    const noteHtml = r.note
      ? `<span><i class="fa-regular fa-comment"></i> ${r.note}</span>`
      : '';

    return `
      <div class="req-list-card ${statusCls}" id="reqcard-${r.id}" onclick="openPrDetail('${r.id}')">
        <div class="req-list-header">
          <div class="req-list-icon">${iconHtml}</div>
          <div class="req-list-main">
            <div style="font-size:13px;font-weight:bold;">${r.guestName}</div>
            <div style="font-size:10px;color:var(--text-muted);margin-top:2px;">
              販売先 ${r.buyerCode || '未連携'} / 取引先 ${r.clientCompanyCode || '未連携'}
            </div>
            <div class="req-list-meta">
              <span><i class="fa-regular fa-calendar"></i> ${r.date}</span>
              <span><i class="fa-solid fa-box-open"></i> ${r.items.length}点</span>
              ${noteHtml}
            </div>
          </div>
          <div class="req-list-amount">${formatSalePrice(displayTotal)}<br><span style="font-size:10px;font-weight:normal;color:var(--text-muted);">${totalLabel}（USD）</span></div>
          <span class="req-status-badge ${statusBadgeCls}">
            ${r.status === '未対応' ? '<i class="fa-solid fa-circle-exclamation"></i>' :
              r.status === '保留中' ? '<i class="fa-solid fa-clock"></i>' :
              '<i class="fa-solid fa-circle-check"></i>'} ${r.status}
          </span>
          <div class="req-list-chevron-wrap"><i class="fa-solid fa-chevron-down req-list-chevron"></i></div>
        </div>
      </div>`;
  }).join('');
}

// =====================================================
// 購入明細モーダルを開く
// =====================================================
function openPrDetail(id) {
  const r = APP_DATA.purchaseRequests.find(x => x.id === id);
  if (!r) return;

  document.getElementById('prDetailTitle').textContent = `${r.id} — ${r.guestName}`;
  renderPrDetailBody(r);
  document.getElementById('prDetailModal').classList.remove('hidden');
}

function closePrDetail() {
  document.getElementById('prDetailModal').classList.add('hidden');
  renderPurchaseRequests(); // 一覧も更新
}

// =====================================================
// 明細モーダル本体をレンダリング
// =====================================================
function renderPrDetailBody(r) {
  const approvedItems = r.items.filter(it => it.itemStatus === 'approved');
  const approvedTotal = approvedItems.reduce((s, it) => s + (it.salePrice || 0), 0);
  const approvedCount = approvedItems.length;

  const statusBadgeCls = r.status === '未対応' ? 's-new' : r.status === '保留中' ? 's-hold' : 's-done';

  const infoHtml = `
    <div class="pr-detail-info">
      <div class="pr-detail-field">
        <label><i class="fa-solid fa-id-card"></i> リクエストID</label>
        <span>${r.id}</span>
      </div>
      <div class="pr-detail-field">
        <label><i class="fa-solid fa-building"></i> 購入者名</label>
        <span style="font-weight:bold;">${r.guestName}</span>
      </div>
      <div class="pr-detail-field">
        <label><i class="fa-solid fa-hashtag"></i> 販売先コード</label>
        <span><code>${r.buyerCode || '未連携'}</code></span>
      </div>
      <div class="pr-detail-field">
        <label><i class="fa-solid fa-handshake"></i> 取引先コード</label>
        <span><code>${r.clientCompanyCode || '未連携'}</code></span>
      </div>
      <div class="pr-detail-field">
        <label><i class="fa-regular fa-calendar"></i> 日時</label>
        <span>${r.date}</span>
      </div>
      <div class="pr-detail-field">
        <label><i class="fa-solid fa-box-open"></i> 購入点数</label>
        <span>${r.items.length}点</span>
      </div>
      <div class="pr-detail-field">
        <label><i class="fa-solid fa-flag"></i> ステータス</label>
        <span class="req-status-badge ${statusBadgeCls}" style="display:inline-flex;">${r.status}</span>
      </div>
      <div class="pr-detail-field">
        <label><i class="fa-regular fa-comment"></i> 備考</label>
        <span>${r.note || '—'}</span>
      </div>
    </div>`;

  const rowsHtml = r.items.map((it, idx) => {
    const isApproved = it.itemStatus === 'approved';
    const isRejected = it.itemStatus === 'rejected';
    const inventoryItem = (APP_DATA.inventory || []).find(item => item.code === it.itemCode);
    const isReserved = isApproved
      && inventoryItem?.status === '取置中'
      && inventoryItem?.reservationRequestId === r.id;
    const rowCls = isApproved ? 'item-approved' : isRejected ? 'item-rejected' : '';
    const approveActive = isApproved ? 'active' : '';
    const rejectActive  = isRejected ? 'active'  : '';
    const itemBadge = isApproved
      ? `<span style="font-size:11px;font-weight:bold;color:var(--success);"><i class="fa-solid fa-lock"></i> ${isReserved ? '承認・取置中' : `承認（${inventoryItem?.status || '在庫不明'}）`}</span>`
      : isRejected
      ? '<span style="font-size:11px;font-weight:bold;color:var(--danger);"><i class="fa-solid fa-circle-xmark"></i> 拒否</span>'
      : '<span style="font-size:11px;color:var(--text-muted);">未決定</span>';
    return `
      <tr class="${rowCls}" id="pr-item-row-${r.id}-${idx}">
        <td style="font-weight:bold;font-size:12px;">${it.itemCode}</td>
        <td>${it.itemName}</td>
        <td style="font-weight:bold;color:var(--primary);">${formatSalePrice(it.salePrice)}</td>
        <td>${itemBadge}</td>
        <td>
          <div class="pr-item-actions">
            <button class="btn-approve ${approveActive}" onclick="setPrItemStatus('${r.id}',${idx},'approved')">
              <i class="fa-solid fa-check"></i> 承認
            </button>
            <button class="btn-reject ${rejectActive}" onclick="setPrItemStatus('${r.id}',${idx},'rejected')">
              <i class="fa-solid fa-xmark"></i> 拒否
            </button>
          </div>
        </td>
      </tr>`;
  }).join('');

  const tableHtml = `
    <div class="pr-items-scroll">
    <table class="pr-items-table">
      <thead>
        <tr>
          <th>商品コード</th>
          <th>商品名</th>
          <th>販売価格（USD）</th>
          <th>判定</th>
          <th>操作</th>
        </tr>
      </thead>
      <tbody>${rowsHtml}</tbody>
    </table>
    </div>`;

  const totalHtml = `
    <div class="pr-total-row" id="pr-total-row">
      <span class="pr-total-label"><i class="fa-solid fa-circle-check" style="color:var(--success);"></i> 承認済み合計</span>
      <span class="pr-total-items" id="pr-approved-count">${approvedCount}点</span>
      <span class="pr-total-val" id="pr-approved-total">${formatSalePrice(approvedTotal)}</span>
    </div>`;

  const footerHtml = `
    <div class="pr-modal-footer">
      <button class="btn btn-outline" style="color:var(--danger);border-color:var(--danger);" onclick="setPrRequestStatus('${r.id}','却下')">
        <i class="fa-solid fa-ban"></i> リクエストを却下
      </button>
      <button class="btn btn-outline" onclick="setPrRequestStatus('${r.id}','保留中')">
        <i class="fa-solid fa-clock"></i> 出荷を保留
      </button>
      <button class="btn btn-outline" onclick="closePrDetail()">閉じる</button>
      <button class="btn btn-primary btn-lg" onclick="proceedToShipping('${r.id}')" id="pr-ship-btn"
        ${approvedCount === 0 ? 'disabled style="opacity:0.4;cursor:not-allowed;"' : ''}>
        <i class="fa-solid fa-truck"></i> 出荷登録に進む
      </button>
    </div>`;

  document.getElementById('prDetailBody').innerHTML = infoHtml + tableHtml + totalHtml + footerHtml;
}

// =====================================================
// 商品ごとの承認/拒否をセット
// =====================================================
async function setPrItemStatus(reqId, itemIdx, status) {
  const r = APP_DATA.purchaseRequests.find(x => x.id === reqId);
  const requestItem = r?.items?.[itemIdx];
  if (!r || !requestItem) return;
  if (window.ZaikoAPI && r.apiManaged) {
    if (requestItem.itemStatus !== 'pending') {
      showToast('info', '処理済みです', 'DBで確定した購入リクエストは元に戻せません');
      return;
    }
    try {
      await window.ZaikoAPI.reviewPurchaseRequest(r._id, status === 'approved' ? 'approved' : 'rejected', '');
      const updated = APP_DATA.purchaseRequests.find(item => item._id === r._id);
      if (updated) renderPrDetailBody(updated);
      renderPurchaseRequests();
      showToast('success', status === 'approved' ? '商品を取り置きました' : '購入リクエストを却下しました',
        status === 'approved' ? `${requestItem.itemCode} をDB上で取置中にしました` : requestItem.itemCode);
    } catch (error) {
      showToast('error', '購入リクエストを処理できませんでした', error.message || '在庫状態を確認してください');
    }
    return;
  }
  const previousStatus = requestItem.itemStatus;

  // トグル（再クリックで pending に戻す）
  if (previousStatus === status) {
    if (previousStatus === 'approved' && typeof releasePurchaseRequestItem === 'function') {
      releasePurchaseRequestItem(r, requestItem);
    }
    requestItem.itemStatus = 'pending';
  } else {
    if (status === 'approved') {
      const reservation = typeof reservePurchaseRequestItem === 'function'
        ? reservePurchaseRequestItem(r, requestItem)
        : { ok: false, reason: '取置処理を利用できません' };
      if (!reservation.ok) {
        showToast('error', '承認できません', reservation.reason);
        return;
      }
      requestItem.itemStatus = 'approved';
      showToast('success', '商品を取り置きました', `${requestItem.itemCode} を ${r.id} 専用に確保しました`);
    } else {
      if (previousStatus === 'approved' && typeof releasePurchaseRequestItem === 'function') {
        releasePurchaseRequestItem(r, requestItem);
      }
      requestItem.itemStatus = status;
    }
  }

  // 承認済みがあれば対応済、全件拒否なら却下、未決定が残れば未対応。
  const hasApproved = r.items.some(it => it.itemStatus === 'approved');
  const hasPending  = r.items.some(it => it.itemStatus === 'pending');
  r.status = hasApproved ? '対応済' : hasPending ? '未対応' : '却下';

  if (typeof persistPurchaseRequests === 'function') persistPurchaseRequests();
  refreshLinkedBusinessViews({ source: 'purchase-request-item-status' });

  renderPrDetailBody(r);
}

// =====================================================
// リクエスト全体のステータスを変更（保留など）
// =====================================================
async function setPrRequestStatus(reqId, newStatus) {
  const r = APP_DATA.purchaseRequests.find(x => x.id === reqId);
  if (!r) return;
  if (window.ZaikoAPI && r.apiManaged) {
    if (newStatus === '保留中') {
      showToast('info', '保留にしました', `${r.id} は未対応のまま保持されます`);
      closePrDetail();
      return;
    }
    try {
      await window.ZaikoAPI.reviewPurchaseRequest(r._id, 'rejected', newStatus);
      closePrDetail();
      showToast('warning', '購入リクエストを却下しました', `${r.id} の取置対象から除外しました`);
    } catch (error) {
      showToast('error', '購入リクエストを処理できませんでした', error.message || '状態を確認してください');
    }
    return;
  }
  if (['却下', '取消', '取消済'].includes(newStatus)) {
    if (typeof releasePurchaseRequestReservations === 'function') releasePurchaseRequestReservations(r);
    r.items.forEach(item => {
      if (item.itemStatus !== 'rejected') item.itemStatus = 'rejected';
    });
  }
  r.status = newStatus;
  if (typeof persistPurchaseRequests === 'function') persistPurchaseRequests();
  refreshLinkedBusinessViews({ source: 'purchase-request-status' });
  const isClosed = ['却下', '取消', '取消済'].includes(newStatus);
  showToast(isClosed ? 'warning' : 'info', isClosed ? '取置を解除しました' : '保留にしました',
    isClosed ? `${r.id} を${newStatus}にし、対象商品の取置を解除しました` : `${r.id} を保留中にしました`);
  closePrDetail();
}

// =====================================================
// 出荷登録へ進む
// =====================================================
function proceedToShipping(reqId) {
  const r = APP_DATA.purchaseRequests.find(x => x.id === reqId);
  if (!r) return;
  const party = typeof resolvePurchaseRequestPartyCodes === 'function'
    ? resolvePurchaseRequestPartyCodes(r)
    : { buyerCode: r.buyerCode || '', clientCompanyCode: r.clientCompanyCode || '' };
  const buyerMatch = (APP_DATA.buyers || []).find(buyer => buyer.code === party.buyerCode);
  const clientMatch = (APP_DATA.clientCompanies || []).find(company =>
    company.id === party.clientCompanyCode && company.buyerCode === party.buyerCode);
  if (!buyerMatch || !clientMatch) {
    showToast('error', '取引先コードが未連携です', `${r.id} の販売先コード／取引先コードを確認してください`);
    return;
  }
  r.buyerCode = party.buyerCode;
  r.clientCompanyCode = party.clientCompanyCode;
  const approvedItems = r.items.filter(it => it.itemStatus === 'approved');
  if (approvedItems.length === 0) {
    showToast('error', '承認済み商品がありません', '1件以上の商品を承認してください');
    return;
  }

  for (const item of approvedItems) {
    const reservation = typeof reservePurchaseRequestItem === 'function'
      ? reservePurchaseRequestItem(r, item)
      : { ok: false, reason: '取置処理を利用できません' };
    if (!reservation.ok) {
      showToast('error', '出荷登録へ進めません', reservation.reason);
      return;
    }
  }

  // ステータス更新
  r.status = '対応済';
  if (typeof persistPurchaseRequests === 'function') persistPurchaseRequests();

  // モーダルを閉じて出荷登録へ遷移
  document.getElementById('prDetailModal').classList.add('hidden');
  renderPurchaseRequests();

  navigateTo('shipping');
  _shippingPurchaseRequestId = r.id;

  // navigateTo の init_shipping が完了した直後に明細を差し込む
  // init_shippingBaseData() で伝票番号・日付・セレクトが構築されるのを待つ
  setTimeout(() => {
    const container = document.getElementById('shippingLines');
    if (!container) return;

    // 既存の明細を全クリア
    container.innerHTML = '';
    shippingLineCount = 0;

    // 購入者は名称ではなく、リクエストに保存された固定販売先コードで指定する。
    const destSel = document.getElementById('sh-dest');
    if (destSel) destSel.value = r.buyerCode;

    // 備考にリクエストIDをセット
    const noteField = document.getElementById('sh-note');
    if (noteField) noteField.value = `購入リクエスト ${r.id} より / ${r.guestName}${r.note ? ' / ' + r.note : ''}`;

    // 承認済み商品を1行ずつ追加
    approvedItems.forEach(it => {
      shippingLineCount++;
      const lineId = shippingLineCount;
      const inv = APP_DATA.inventory.find(x => x.code === it.itemCode);
      const brand = inv ? inv.brand : '—';
      const model = inv ? inv.model : '—';
      const price = it.salePrice || 0;

      const div = document.createElement('div');
      div.className = 'slip-line';
      div.dataset.lineId = lineId;
      div.innerHTML = `
        <div>
          <input class="form-control" style="font-size:12px;border-color:var(--success);" placeholder="INV-2026-XXXX"
            oninput="autoFillItem(this, ${lineId}, 'shipping')" id="sh-code-${lineId}" value="${it.itemCode}">
        </div>
        <div id="sh-brand-${lineId}" style="font-size:12px;color:var(--text);">${brand}</div>
        <div id="sh-model-${lineId}" style="font-size:12px;color:var(--text);">${model}</div>
        <div>
          <input type="text" inputmode="numeric" class="form-control" style="font-size:12px;" placeholder="0"
            oninput="priceFormatHandler(this);calcShippingTotal()" onblur="priceFormatHandler(this)"
            id="sh-price-${lineId}" value="${formatPriceInput(price)}" data-raw-value="${price}">
        </div>
        <div>
          <button class="btn btn-ghost btn-sm" style="color:var(--danger);" onclick="removeLine(this, 'shipping')"><i class="fa-solid fa-xmark"></i></button>
        </div>
      `;
      container.appendChild(div);
    });

    // 合計を再計算
    calcShippingTotal();

    showToast('success', '出荷登録に移動しました',
      `${approvedItems.length}点 の商品を自動入力しました`);
  }, 200);
}

// =====================================================
// =====================================================
// 管理者認証ガード（作業者による制限ページアクセス用）
// =====================================================
const ADMIN_RESTRICTED_PAGES = ['master', 'box'];  // 認証が必要なページ

// 認証済みセッション（タブを閉じるまで有効）
let _adminAuthGranted = false;

function openAdminAuthModal(targetPage) {
  const pageNames = { master: 'マスタ登録', box: 'ゲスト管理' };
  const label = document.getElementById('adminAuthPageLabel');
  if (label) label.textContent = `「${pageNames[targetPage] || targetPage}」ページ`;

  const codeInput = document.getElementById('adminAuthCode');
  if (codeInput) codeInput.value = '';
  const err = document.getElementById('adminAuthError');
  if (err) err.classList.add('hidden');

  // 認証後に遷移するページを記憶
  document.getElementById('adminAuthModal').dataset.targetPage = targetPage;
  document.getElementById('adminAuthModal').classList.remove('hidden');
  setTimeout(() => { if (codeInput) codeInput.focus(); }, 100);
}

function closeAdminAuthModal() {
  document.getElementById('adminAuthModal').classList.add('hidden');
}

function toggleAdminAuthPw() {
  const inp  = document.getElementById('adminAuthCode');
  const icon = document.getElementById('adminAuthEyeIcon');
  if (!inp) return;
  if (inp.type === 'password') {
    inp.type = 'text';
    icon.className = 'fa-solid fa-eye-slash';
  } else {
    inp.type = 'password';
    icon.className = 'fa-solid fa-eye';
  }
}

async function submitAdminAuth() {
  const input = document.getElementById('adminAuthCode');
  const code = String(input?.value || '').toUpperCase().replace(/[^A-Z0-9]/g, '').slice(0, 6);
  const err = document.getElementById('adminAuthError');

  let authenticated = false;
  try {
    if (window.ZaikoAPI?.verifyAdminAccessCode) {
      const result = await window.ZaikoAPI.verifyAdminAccessCode(code);
      authenticated = result?.valid === true;
    } else {
      authenticated = _getLocalAdminAccessCode(false).code === code;
    }
  } catch (error) {
    authenticated = false;
  }
  if (!authenticated) {
    err.classList.remove('hidden');
    if (input) {
      input.value = '';
      input.focus();
    }
    return;
  }

  // 認証成功後もAPIセッションは作業者のまま維持する。
  _adminAuthGranted = true;
  closeAdminAuthModal();
  const targetPage = document.getElementById('adminAuthModal').dataset.targetPage;
  if (targetPage) navigateTo(targetPage);
}

// =====================================================
// navigateTo: purchase-list 対応（data.jsを上書き）
// =====================================================
const _navOrig = navigateTo;
window.navigateTo = function(page) {
  // ── 権限ガード ──
  if (typeof canAccessPage === 'function' && !canAccessPage(page)) {
    showToast('error', 'アクセス拒否', 'この画面は管理者のみ利用できます');
    return;
  }
  // admin専用ページ: admin以外はアクセス不可
  const ADMIN_ONLY_PAGES = ['approval', 'password', 'company'];
  if (ADMIN_ONLY_PAGES.includes(page) && !isAdmin()) {
    showToast('error', 'アクセス拒否', 'このページは管理者のみアクセスできます');
    return;
  }

  // 作業者が制限ページへアクセスしようとした場合は管理者認証モーダルを表示
  const needsAdminAuth = typeof needsAdminAuthentication === 'function'
    ? needsAdminAuthentication(page)
    : isWorker() && ADMIN_RESTRICTED_PAGES.includes(page);
  if (needsAdminAuth && !_adminAuthGranted) {
    openAdminAuthModal(page);
    return;
  }
  // 認証フラグは1回の遷移で消費（再訪問時は再認証）
  _adminAuthGranted = false;

  // password/client/company はマスタ登録パネル内で表示するため page-master を使う
  const MASTER_SUB_PAGES = ['password', 'client', 'company'];
  const effectivePage = MASTER_SUB_PAGES.includes(page) ? 'master' : page;

  document.querySelectorAll('.nav-item').forEach(el => {
    // password/client/company はマスタとして扱いサイドバーもマスタをアクティブに
    const activeKey = MASTER_SUB_PAGES.includes(page) ? 'master' : page;
    el.classList.toggle('active', el.dataset.page === activeKey);
  });
  document.querySelectorAll('.page-panel').forEach(el => {
    el.classList.add('hidden');
  });

  // purchase-list は page-purchase-list
  const panelId = 'page-' + effectivePage;
  const target = document.getElementById(panelId);
  if (target) {
    target.classList.remove('hidden');
    // 初期化
    const initFn = {
      'dashboard': init_dashboard,
      'market': () => {
        if (typeof init_market === 'function') init_market();
      },
      'inventory': init_inventory,
      'purchase': init_purchase,
      'purchase-entry': () => {
        if (typeof init_purchase_entry === 'function') init_purchase_entry();
      },
      'sales': init_sales,
      'shipping': init_shipping,
      'master': init_master,
      'performance': init_performance,
      'purchase-list': () => {
        renderPurchaseRequests();
      },
      'approval': () => {
        renderApprovalList();
        updateApprovalBadge();
        const pendingEl = document.getElementById('pendingApprovalCount');
        if (pendingEl) pendingEl.textContent = APP_DATA.approvalRequests.filter(r => r.status === 'pending').length;
      },
      'sales-list': () => {
        initSalesListFilter();
        filterSalesList();
      },
      'box': () => {
        renderBoxMatrix();
      },
      'returns': () => {
        renderReturnsList();
      },
      'stocktake': () => {
        if (typeof initStocktake === 'function') initStocktake();
      },
      'password': () => {
        // マスタ登録内タブに切り替え（インライン描画）
        if (!isAdmin()) { showToast('error', 'アクセス拒否', 'このページは管理者のみアクセスできます'); return; }
        _checkApprovalCodeRefresh();
        switchMasterTab('password');
      },
      'client':   () => {
        if (!isAdmin()) { showToast('error', 'アクセス拒否', 'このページは管理者のみアクセスできます'); return; }
        switchMasterTab('client');
      },
      'company':  () => {
        if (!isAdmin()) { showToast('error', 'アクセス拒否', 'このページは管理者のみアクセスできます'); return; }
        switchMasterTab('company');
      },
    };
    if (initFn[page]) initFn[page]();
  }

  const pageNames = {
    'dashboard': 'ダッシュボード',
    'market': '相場表',
    'purchase': '商品登録',
    'purchase-entry': '仕入登録',
    'sales': '売上登録',
    'shipping': '出荷登録',
    'master': 'マスタ登録',
    'performance': '実績管理',
    'inventory': '在庫一覧',
    'purchase-list': '購入一覧',
    'sales-list': '伝票一覧',
    'approval': '承認管理',
    'box': 'ゲスト管理',
    'returns': '返品/持ち帰り',
    'stocktake': '棚卸',
    'password': 'パスワード管理',
    'client': '取引先会社',
    'company': '会社情報',
  };

  const titleEl = document.getElementById('pageTitle');
  const subEl   = document.getElementById('pageTitleSub');
  const mobEl   = document.getElementById('mobPageTitle');
  const pageName = pageNames[page] || page;
  if (titleEl) titleEl.textContent = pageName;
  if (subEl)   subEl.textContent   = pageName;
  if (mobEl)   mobEl.textContent   = pageName;

  // スマホではナビ後にドロワーを閉じる
  closeAppDrawer();
};

// =====================================================
// 返品/持ち帰り ページ
// =====================================================

// =====================================================
// =====================================================
// パスワード管理ページ
// =====================================================

const ADMIN_ACCESS_CODE_STORAGE_KEY = 'inv_admin_access_code_v1';

/** ランダム6桁英数字コード生成（紛らわしい文字は除外） */
function _genCode6() {
  const chars = 'ABCDEFGHJKLMNPQRSTUVWXYZ23456789';
  let code = '';
  for (let i = 0; i < 6; i++) code += chars[Math.floor(Math.random() * chars.length)];
  return code;
}

function _localDateKey(date = new Date()) {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`;
}

function _getLocalAdminAccessCode(forceRefresh = false) {
  const now = new Date();
  let stored = null;
  try { stored = JSON.parse(localStorage.getItem(ADMIN_ACCESS_CODE_STORAGE_KEY) || 'null'); } catch (_) { stored = null; }
  const valid = stored && /^[A-Z0-9]{6}$/.test(String(stored.code || '')) && stored.date === _localDateKey(now);
  if (forceRefresh || !valid) {
    stored = { code: _genCode6(), date: _localDateKey(now), updatedAt: now.toISOString() };
    localStorage.setItem(ADMIN_ACCESS_CODE_STORAGE_KEY, JSON.stringify(stored));
  }
  const next = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1, 0, 0, 0, 0);
  return { code: stored.code, updatedAt: stored.updatedAt, nextRefreshAt: next.toISOString() };
}

/** ランダムパスワード生成（8文字英数字） */
function _genPassword() {
  const chars = 'abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789';
  let pw = '';
  for (let i = 0; i < 8; i++) pw += chars[Math.floor(Math.random() * chars.length)];
  return pw;
}

// =====================================================
// マスタ内インライン描画 — パスワード管理
// =====================================================

/**
 * パスワード管理タブをマスタコンテンツエリアにインライン描画
 * 4タブ（管理者・作業者・ゲスト・管理者認証コード）を持つ
 */
function renderPasswordMasterTab(area) {
  _checkApprovalCodeRefresh();
  const adminCount = APP_DATA.users.filter(user => user.role === 'admin').length;
  const workerCount = APP_DATA.users.filter(user => user.role === 'buyer' || user.role === 'worker').length;
  const guestCount = APP_DATA.guestAccounts.length;
  area.innerHTML = `
    <div class="master-content">
      <div style="display:flex;align-items:flex-start;justify-content:space-between;gap:16px;margin-bottom:16px;">
        <div>
          <h3 style="font-size:15px;font-weight:bold;color:var(--primary);margin-bottom:4px;"><i class="fa-solid fa-key"></i> パスワード管理</h3>
          <p style="font-size:12px;color:var(--text-muted);margin:0;">管理者・作業者・ゲストのログイン情報と、ゲストに紐づく顧客情報を一元管理します。</p>
        </div>
        <span class="badge badge-stock"><i class="fa-solid fa-link"></i> ログイン認証と連動</span>
      </div>
      <div style="border-bottom:1px solid var(--border);margin-bottom:0;">
        <div style="display:flex;gap:0;overflow-x:auto;">
          <button class="tab-btn active" id="mpw-tab-admin-btn" onclick="switchMpwTab('admin')" style="font-size:12px;padding:8px 16px;white-space:nowrap;"><i class="fa-solid fa-shield-halved"></i> 管理者 <span class="count-badge">${adminCount}</span></button>
          <button class="tab-btn"        id="mpw-tab-buyer-btn" onclick="switchMpwTab('buyer')" style="font-size:12px;padding:8px 16px;white-space:nowrap;"><i class="fa-solid fa-user-tie"></i> 作業者 <span class="count-badge">${workerCount}</span></button>
          <button class="tab-btn"        id="mpw-tab-guest-btn" onclick="switchMpwTab('guest')" style="font-size:12px;padding:8px 16px;white-space:nowrap;"><i class="fa-solid fa-users"></i> ゲスト・顧客 <span class="count-badge">${guestCount}</span></button>
          <button class="tab-btn"        id="mpw-tab-authcode-btn" onclick="switchMpwTab('authcode')" style="font-size:12px;padding:8px 16px;white-space:nowrap;"><i class="fa-solid fa-key"></i> 管理者認証コード</button>
        </div>
      </div>
      <!-- 管理者パネル -->
      <div id="mpw-panel-admin" style="padding:16px 0;">
        <div style="display:flex;align-items:center;gap:12px;margin-bottom:12px;">
          <h4 style="font-size:13px;font-weight:700;color:var(--primary);margin:0;"><i class="fa-solid fa-shield-halved"></i> 管理者アカウント</h4>
          <button class="btn btn-accent btn-sm" onclick="openLoginInfoModal('admin')"><i class="fa-solid fa-plus"></i> 新規追加</button>
        </div>
        <div class="data-table-wrapper">
          <table class="data-table">
            <thead><tr><th>名前</th><th>ログインID</th><th>メールアドレス</th><th>ステータス</th><th>操作</th></tr></thead>
            <tbody id="mpw-admin-tbody"></tbody>
          </table>
        </div>
      </div>
      <!-- 作業者パネル -->
      <div id="mpw-panel-buyer" style="padding:16px 0;display:none;">
        <div style="display:flex;align-items:center;gap:12px;margin-bottom:12px;flex-wrap:wrap;">
          <div>
            <h4 style="font-size:13px;font-weight:700;color:var(--primary);margin:0;"><i class="fa-solid fa-user-tie"></i> 当社・作業者アカウント</h4>
            <p style="font-size:11px;color:var(--text-muted);margin:4px 0 0;">仕入担当者マスタと同じコードで連動します。氏名・コードの追加や変更は仕入担当者から行えます。</p>
          </div>
          <button class="btn btn-accent btn-sm" style="margin-left:auto;" onclick="switchMasterTab('staff')"><i class="fa-solid fa-user-plus"></i> 仕入担当者を追加</button>
        </div>
        <div class="data-table-wrapper">
          <table class="data-table">
            <thead><tr><th>仕入担当者コード</th><th>名前</th><th>所属</th><th>ログイン情報</th><th>ステータス</th><th>操作</th></tr></thead>
            <tbody id="mpw-buyer-tbody"></tbody>
          </table>
        </div>
      </div>
      <!-- ゲストパネル -->
      <div id="mpw-panel-guest" style="padding:16px 0;display:none;">
        <div style="display:flex;align-items:center;gap:12px;margin-bottom:12px;">
          <div>
            <h4 style="font-size:13px;font-weight:700;color:var(--primary);margin:0;"><i class="fa-solid fa-users"></i> 販売先・ゲストアカウント</h4>
            <p style="font-size:11px;color:var(--text-muted);margin:4px 0 0;">ゲスト管理と同じアカウントを表示します。追加・編集・削除は両画面へ即時反映されます。</p>
          </div>
          <div style="margin-left:auto;display:flex;gap:8px;">
            <button class="btn btn-accent btn-sm" onclick="openLoginInfoModal('guest')"><i class="fa-solid fa-plus"></i> 新規追加</button>
            <button class="btn btn-outline btn-sm" onclick="bulkChangeGuestPasswords()"><i class="fa-solid fa-rotate"></i> パスワード一括変更</button>
            <button class="btn btn-outline btn-sm" onclick="sendGuestPasswordEmails()"><i class="fa-solid fa-envelope"></i> 通知メール送信</button>
          </div>
        </div>
        <div class="data-table-wrapper">
          <table class="data-table">
            <thead><tr><th>会社名</th><th>顧客コード</th><th>ゲストID</th><th>顧客情報</th><th>メールアドレス</th><th>ステータス</th><th>パスワード</th><th>操作</th></tr></thead>
            <tbody id="mpw-guest-tbody"></tbody>
          </table>
        </div>
      </div>
      <!-- 管理者認証コードパネル -->
      <div id="mpw-panel-authcode" style="padding:16px 0;display:none;">
        <div style="max-width:720px;border:1px solid #bfdbfe;background:linear-gradient(135deg,#f8fbff,#eff6ff);border-radius:12px;padding:22px;">
          <div style="display:flex;align-items:flex-start;justify-content:space-between;gap:20px;flex-wrap:wrap;">
            <div>
              <h4 style="font-size:14px;font-weight:700;color:var(--primary);margin:0 0 6px;"><i class="fa-solid fa-shield-halved"></i> 作業者向け・管理者認証コード</h4>
              <p style="font-size:12px;color:var(--text-muted);margin:0;line-height:1.8;">作業者が制限ページを開く際に入力する6桁の英数字です。毎日0:00（日本時間）に自動更新されます。</p>
            </div>
            <span class="badge badge-stock"><i class="fa-solid fa-database"></i> DB共有</span>
          </div>
          <div id="mpw-authcode-status" style="margin-top:20px;">
            <div style="font-size:12px;color:var(--text-muted);">認証コードを読み込んでいます…</div>
          </div>
          <div style="border-top:1px solid #dbeafe;margin-top:18px;padding-top:16px;display:flex;align-items:center;justify-content:space-between;gap:12px;flex-wrap:wrap;">
            <div style="font-size:11px;color:var(--text-muted);"><i class="fa-solid fa-circle-info"></i> 手動更新すると現在のコードは直ちに無効になります。</div>
            <button class="btn btn-primary btn-sm" id="mpw-authcode-rotate" onclick="rotateAdminAccessCodeNow()"><i class="fa-solid fa-rotate"></i> 今すぐ更新</button>
          </div>
        </div>
      </div>
    </div>
  `;
  renderMpwAdminTable();
}

/** マスタ内パスワードタブ切替 */
function switchMpwTab(tab) {
  ['admin','buyer','guest','authcode'].forEach(t => {
    const panel = document.getElementById(`mpw-panel-${t}`);
    const btn   = document.getElementById(`mpw-tab-${t}-btn`);
    if (panel) panel.style.display = t === tab ? '' : 'none';
    if (btn)   btn.classList.toggle('active', t === tab);
  });
  if (tab === 'admin') renderMpwAdminTable();
  if (tab === 'buyer') renderMpwBuyerTable();
  if (tab === 'guest') renderMpwGuestTable();
  if (tab === 'authcode') renderMpwAccessCodePanel();
}

/** 管理者テーブル描画（マスタ内） */
function renderMpwAdminTable() {
  const tbody = document.getElementById('mpw-admin-tbody');
  if (!tbody) return;
  const admins = APP_DATA.users.filter(u => u.role === 'admin');
  tbody.innerHTML = admins.length === 0
    ? '<tr><td colspan="5" style="text-align:center;color:var(--text-muted);padding:24px;">管理者アカウントがありません</td></tr>'
    : admins.map(u => `
      <tr>
        <td style="font-weight:700;">${_mEsc(u.name)}</td>
        <td><code style="font-size:11px;">${_mEsc(u.loginId || u.email || '—')}</code></td>
        <td style="font-size:12px;">${_mEsc(u.email || '—')}</td>
        <td>${_mpwStatusBadge(u.active !== false)}</td>
        <td>
          <button class="btn btn-outline btn-sm" onclick="openLoginInfoModal('admin','${u.id}')"><i class="fa-solid fa-pen"></i> 編集</button>
          <button class="btn btn-outline btn-sm" onclick="showMpwChangeModal('${u.id}', false)"><i class="fa-solid fa-key"></i> PW変更</button>
        </td>
      </tr>`).join('');
}

function _formatAdminAccessCodeTime(value) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '—';
  return new Intl.DateTimeFormat('ja-JP', {
    timeZone: 'Asia/Tokyo', year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', hour12: false,
  }).format(date);
}

function _paintAdminAccessCode(record) {
  const area = document.getElementById('mpw-authcode-status');
  if (!area) return;
  area.innerHTML = `
    <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:14px;">
      <div style="background:#fff;border:1px solid #dbeafe;border-radius:10px;padding:18px;">
        <div style="font-size:11px;font-weight:700;color:var(--text-muted);margin-bottom:8px;">本日の認証コード</div>
        <div id="mpw-authcode-value" style="font-family:ui-monospace,SFMono-Regular,Consolas,monospace;font-size:30px;font-weight:800;letter-spacing:8px;color:#1d4ed8;white-space:nowrap;">${_mEsc(record.code || '------')}</div>
      </div>
      <div style="background:#fff;border:1px solid #dbeafe;border-radius:10px;padding:18px;display:grid;gap:10px;">
        <div><span style="font-size:11px;color:var(--text-muted);display:block;">最終更新</span><strong style="font-size:12px;">${_mEsc(_formatAdminAccessCodeTime(record.updatedAt))}</strong></div>
        <div><span style="font-size:11px;color:var(--text-muted);display:block;">次回自動更新</span><strong style="font-size:12px;">${_mEsc(_formatAdminAccessCodeTime(record.nextRefreshAt))}</strong></div>
      </div>
    </div>`;
}

async function renderMpwAccessCodePanel() {
  const area = document.getElementById('mpw-authcode-status');
  if (!area) return;
  area.innerHTML = '<div style="font-size:12px;color:var(--text-muted);"><i class="fa-solid fa-spinner fa-spin"></i> 認証コードを読み込んでいます…</div>';
  try {
    const record = window.ZaikoAPI?.getAdminAccessCode
      ? await window.ZaikoAPI.getAdminAccessCode()
      : _getLocalAdminAccessCode(false);
    _paintAdminAccessCode(record);
  } catch (error) {
    area.innerHTML = `<div style="color:var(--danger);font-size:12px;"><i class="fa-solid fa-circle-exclamation"></i> ${_mEsc(error?.message || '認証コードを取得できませんでした')}</div>`;
  }
}

async function rotateAdminAccessCodeNow() {
  if (!confirm('管理者認証コードを今すぐ更新しますか？\n現在のコードは直ちに使用できなくなります。')) return;
  const button = document.getElementById('mpw-authcode-rotate');
  if (button) {
    button.disabled = true;
    button.innerHTML = '<i class="fa-solid fa-spinner fa-spin"></i> 更新中';
  }
  try {
    const record = window.ZaikoAPI?.rotateAdminAccessCode
      ? await window.ZaikoAPI.rotateAdminAccessCode()
      : _getLocalAdminAccessCode(true);
    _paintAdminAccessCode(record);
    showToast('success', '認証コード更新', '新しい6桁の管理者認証コードへ更新しました');
  } catch (error) {
    showToast('error', '更新失敗', error?.message || '管理者認証コードを更新できませんでした');
  } finally {
    if (button) {
      button.disabled = false;
      button.innerHTML = '<i class="fa-solid fa-rotate"></i> 今すぐ更新';
    }
  }
}

/** 作業者テーブル描画（マスタ内） */
function renderMpwBuyerTable() {
  const tbody = document.getElementById('mpw-buyer-tbody');
  if (!tbody) return;
  const buyers = APP_DATA.users.filter(u => u.role === 'buyer' || u.role === 'worker');
  tbody.innerHTML = buyers.length === 0
    ? '<tr><td colspan="6" style="text-align:center;color:var(--text-muted);padding:24px;">作業者アカウントがありません</td></tr>'
    : buyers.map(u => `
      <tr data-staff-code="${_mEsc(u.staffCode || '')}">
        <td><code style="font-size:11px;font-weight:700;">${_mEsc(u.staffCode || '未連携')}</code></td>
        <td style="font-weight:700;">${_mEsc(u.name)}</td>
        <td><span class="badge badge-stock" title="${_mEsc(u.company || _ownCompanyName())}">当社</span></td>
        <td><code style="font-size:11px;">${_mEsc(u.loginId || u.email || '—')}</code><span style="display:block;font-size:10px;color:var(--text-muted);margin-top:3px;word-break:break-word;">${_mEsc(u.email || '—')}</span></td>
        <td>${_mpwStatusBadge(u.active !== false)}</td>
        <td>
          <button class="btn btn-outline btn-sm" onclick="openLoginInfoModal('buyer','${u.id}')"><i class="fa-solid fa-pen"></i> 編集</button>
          <button class="btn btn-outline btn-sm" onclick="showMpwChangeModal('${u.id}', false)"><i class="fa-solid fa-key"></i> PW変更</button>
          ${u.staffCode
            ? `<button class="btn btn-ghost btn-sm" title="仕入担当者マスタで管理" onclick="switchMasterTab('staff')"><i class="fa-solid fa-link"></i></button>`
            : `<button class="btn btn-ghost btn-sm" style="color:var(--danger);" onclick="deleteMpwUser('${u.id}')"><i class="fa-solid fa-trash"></i></button>`}
        </td>
      </tr>`).join('');
}

/** ゲストテーブル描画（マスタ内） */
function renderMpwGuestTable() {
  const tbody = document.getElementById('mpw-guest-tbody');
  if (!tbody) return;
  if (typeof reconcileGuestBuyerDirectory === 'function') reconcileGuestBuyerDirectory();
  const rows = APP_DATA.guestAccounts.map(guest => ({
    guest,
    customer: APP_DATA.buyers.find(customer => customer.code === guest.buyerCode) || {
      code: guest.buyerCode || '未連携',
      name: guest.company || guest.name,
      address: '',
      contact: '',
      email: guest.email || '',
    },
  }));

  tbody.innerHTML = rows.length === 0
    ? '<tr><td colspan="8" style="text-align:center;color:var(--text-muted);padding:24px;">販売先が登録されていません</td></tr>'
    : rows.map(({ customer, guest }) => {
      const guestId = guest?.id || '';
      const companyName = guest?.company || customer.name;
      const email = guest?.email || customer.email || '—';
      const passwordCell = `<span class="mpw-mask" data-gid="${_mEsc(guestId)}">••••••••</span>
           <button class="btn btn-ghost btn-sm" style="padding:2px 6px;" onclick="toggleMpwGuestMask('${_mEsc(guestId)}')" aria-label="パスワードを表示">
             <i class="fa-regular fa-eye" id="mpw-eye-${_mEsc(guestId)}"></i>
           </button>
           <span class="mpw-plain" id="mpw-plain-${_mEsc(guestId)}" style="display:none;color:#1d4ed8;">再設定のみ</span>`;
      const operationCell = `<div style="display:flex;gap:4px;flex-wrap:wrap;">
             <button class="btn btn-outline btn-sm" onclick="openLoginInfoModal('guest','${_mEsc(guestId)}')"><i class="fa-solid fa-pen"></i> 編集</button>
             <button class="btn btn-outline btn-sm" onclick="showMpwChangeModal('${_mEsc(guestId)}', true)"><i class="fa-solid fa-key"></i> PW変更</button>
             <button class="btn btn-ghost btn-sm" style="color:var(--danger);" onclick="deleteMpwGuest('${_mEsc(guestId)}')" aria-label="ログイン情報を削除"><i class="fa-solid fa-trash"></i></button>
           </div>`;
      return `
        <tr data-buyer-code="${_mEsc(customer.code)}">
          <td style="font-weight:700;">${_mEsc(companyName)}</td>
          <td><code style="font-size:11px;">${_mEsc(customer.code || '未連携')}</code></td>
          <td style="font-size:12px;font-family:monospace;">${_mEsc(guestId)}</td>
          <td style="font-size:12px;min-width:180px;">
            <div>${_mEsc(customer.address || '住所未登録')}</div>
            <div style="font-size:10px;color:var(--text-muted);margin-top:2px;">${_mEsc(customer.contact || '連絡先未登録')}</div>
          </td>
          <td style="font-size:12px;">${_mEsc(email)}</td>
          <td>${_mpwStatusBadge(guest.active !== false)}</td>
          <td style="font-family:monospace;letter-spacing:2px;white-space:nowrap;">${passwordCell}</td>
          <td>${operationCell}</td>
        </tr>`;
    }).join('');
}

function openGuestLoginForBuyer(buyerCode) {
  const customer = APP_DATA.buyers.find(buyer => buyer.code === buyerCode);
  if (!customer) return;
  const existingGuest = APP_DATA.guestAccounts.find(guest => guest.buyerCode === buyerCode);
  if (existingGuest) {
    openLoginInfoModal('guest', existingGuest.id);
    return;
  }
  openLoginInfoModal('guest');
  document.getElementById('loginInfoName').value = customer.name || '';
  document.getElementById('loginInfoCompany').value = customer.name || '';
  document.getElementById('loginInfoBuyerCode').value = customer.code || '';
  document.getElementById('loginInfoAddress').value = customer.address || '';
  document.getElementById('loginInfoContact').value = customer.contact || '';
  document.getElementById('loginInfoInvoice').value = customer.invoice || '';
  document.getElementById('loginInfoEmail').value = customer.email || '';
}

function _mpwStatusBadge(active) {
  return active
    ? '<span class="badge badge-stock">● 有効</span>'
    : '<span class="badge" style="background:#f3f4f6;color:#6b7280;">● 停止中</span>';
}

function refreshPasswordMasterDirectory() {
  renderMpwAdminTable();
  renderMpwBuyerTable();
  renderMpwGuestTable();
  renderMasterTabs();
  if (typeof renderBoxMatrix === 'function') renderBoxMatrix();
  const counts = {
    admin: APP_DATA.users.filter(user => user.role === 'admin').length,
    buyer: APP_DATA.users.filter(user => user.role === 'buyer' || user.role === 'worker').length,
    guest: APP_DATA.guestAccounts.length,
  };
  Object.entries(counts).forEach(([role, count]) => {
    const badge = document.querySelector(`#mpw-tab-${role}-btn .count-badge`);
    if (badge) badge.textContent = count;
  });
  if (typeof currentMasterTab !== 'undefined' && currentMasterTab === 'buyer') switchMasterTab('buyer');
}

function persistPasswordMasterDirectory() {
  if (typeof persistLoginDirectory === 'function') persistLoginDirectory();
  refreshPasswordMasterDirectory();
}

/** ゲストPWマスク切替（マスタ内） */
function toggleMpwGuestMask(gid) {
  const mask  = document.querySelector(`.mpw-mask[data-gid="${gid}"]`);
  const plain = document.getElementById(`mpw-plain-${gid}`);
  const eye   = document.getElementById(`mpw-eye-${gid}`);
  if (!mask || !plain) return;
  const showing = plain.style.display !== 'none';
  mask.style.display  = showing ? '' : 'none';
  plain.style.display = showing ? 'none' : '';
  if (eye) eye.className = showing ? 'fa-regular fa-eye' : 'fa-regular fa-eye-slash';
}

/** パスワード変更モーダル（マスタ内） */
let _mpwEditState = { userId: null, isGuest: false };
function showMpwChangeModal(userId, isGuest) {
  _mpwEditState = { userId, isGuest };
  let target = isGuest
    ? APP_DATA.guestAccounts.find(g => g.id === userId)
    : APP_DATA.users.find(u => u.id === userId);
  if (!target) return;
  const name = isGuest ? (target.company || target.name) : target.name;
  document.getElementById('mpwChangeModalTitle').textContent = `パスワード変更 — ${name}`;
  document.getElementById('mpwChangeModalBody').innerHTML = `
    <div style="margin-bottom:14px;padding:10px 14px;background:#f0f9ff;border:1px solid #bae6fd;border-radius:8px;font-size:12px;color:#0c4a6e;">
      <i class="fa-solid fa-user"></i> <strong>${_mEsc(name)}</strong> のパスワードを変更します
    </div>
    <div class="form-group" style="margin-bottom:14px;">
      <label class="form-label" style="font-size:12px;">新しいパスワード <span class="required">*</span></label>
      <div style="display:flex;gap:8px;">
        <input type="password" class="form-control" id="mpwChange-new" placeholder="8文字以上の新しいパスワード" style="font-size:13px;font-family:monospace;" value="">
        <button class="btn btn-outline btn-sm" style="white-space:nowrap;" onclick="document.getElementById('mpwChange-new').value=_genPassword()">
          <i class="fa-solid fa-rotate"></i> 自動生成
        </button>
      </div>
    </div>`;
  document.getElementById('mpwChangeModal').classList.remove('hidden');
}

function closeMpwChangeModal() {
  document.getElementById('mpwChangeModal').classList.add('hidden');
  _mpwEditState = { userId: null, isGuest: false };
}

async function saveMpwChange() {
  const { userId, isGuest } = _mpwEditState;
  const newPw = document.getElementById('mpwChange-new')?.value?.trim();
  if (!newPw) { showToast('error', '入力エラー', 'パスワードを入力してください'); return; }
  if (newPw.length < 8) { showToast('error', '入力エラー', 'パスワードは8文字以上で入力してください'); return; }
  if (!window.ZaikoAPI) {
    const target = isGuest ? APP_DATA.guestAccounts.find(x => x.id === userId) : APP_DATA.users.find(x => x.id === userId);
    if (target) target.password = newPw;
    closeMpwChangeModal(); persistPasswordMasterDirectory();
    showToast('success', 'パスワード変更完了', 'パスワードを更新しました');
    return;
  }
  try {
    await window.ZaikoAPI.changePassword(userId, isGuest, newPw);
    closeMpwChangeModal();
    refreshPasswordMasterDirectory();
    showToast('success', 'パスワード変更完了', 'DB上のパスワードハッシュを更新しました');
  } catch (error) {
    showToast('error', '更新エラー', error.message);
  }
}

/** 作業者削除（マスタ内） */
async function deleteMpwUser(userId) {
  const u = APP_DATA.users.find(x => x.id === userId);
  if (!u) return;
  if (u.staffCode) {
    showToast('info', '仕入担当者マスタと連動しています', `${u.staffCode} の削除は「仕入担当者」から行ってください`);
    switchMasterTab('staff');
    return;
  }
  if (!confirm(`「${u.name}」を削除してよろしいですか？`)) return;
  if (!window.ZaikoAPI) {
    APP_DATA.users = APP_DATA.users.filter(x => x.id !== userId);
    persistPasswordMasterDirectory();
    showToast('success', '削除完了', `${u.name} を削除しました`);
    return;
  }
  try {
    await window.ZaikoAPI.setUserActive(userId, false, false);
    u.active = false;
    refreshPasswordMasterDirectory();
    showToast('success', '停止完了', `${u.name} のログインを停止しました`);
  } catch (error) { showToast('error', '停止エラー', error.message); }
}

/** ゲスト削除（マスタ内） */
async function deleteMpwGuest(gid) {
  const g = APP_DATA.guestAccounts.find(x => x.id === gid);
  if (!g) return;
  if (!confirm(`「${g.company || g.name}」を削除してよろしいですか？`)) return;
  if (!window.ZaikoAPI) {
    APP_DATA.guestAccounts = APP_DATA.guestAccounts.filter(x => x.id !== gid);
    if (typeof setBuyerGuestManaged === 'function') setBuyerGuestManaged(g.buyerCode, false);
    persistPasswordMasterDirectory();
    return;
  }
  try {
    await window.ZaikoAPI.setUserActive(gid, true, false);
    APP_DATA.guestAccounts = APP_DATA.guestAccounts.filter(x => x.id !== gid);
    if (typeof setBuyerGuestManaged === 'function') setBuyerGuestManaged(g.buyerCode, false);
    refreshPasswordMasterDirectory();
    showToast('success', '停止完了', `${g.company || g.name} のゲストログインを停止しました`);
  } catch (error) { showToast('error', '停止エラー', error.message); }
}

// =====================================================
// 新規ユーザー登録（管理者・作業者共用モーダル）
// =====================================================

let _addUserRole = 'buyer';

/** 新規ユーザー追加モーダルを開く（管理者・作業者共用）
 *  呼び出し元は showAddUserModal('admin') or showAddUserModal('buyer')
 */
function showAddUserModal(role) {
  _addUserRole = role;
  const labels = { admin: '管理者', buyer: '作業者' };
  document.getElementById('addUserModalTitle').textContent = `新規${labels[role] || role}登録`;
  document.getElementById('addUser-name').value = '';
  document.getElementById('addUser-email').value = '';
  document.getElementById('addUserModal').classList.remove('hidden');
}

function closeAddUserModal() {
  document.getElementById('addUserModal').classList.add('hidden');
}

async function saveAddUser() {
  const name  = document.getElementById('addUser-name').value.trim();
  const email = document.getElementById('addUser-email').value.trim();
  if (!name)  { showToast('error', '入力エラー', '名前を入力してください'); return; }
  if (!email) { showToast('error', '入力エラー', 'メールアドレス（ログインID）を入力してください'); return; }
  if (APP_DATA.users.some(u => u.email === email || u.loginId === email)) {
    showToast('error', '重複エラー', 'このメールアドレスはすでに登録されています'); return;
  }
  const newPw  = _genPassword();
  if (!window.ZaikoAPI) {
    const newId = 'U' + String(Date.now()).slice(-6);
    APP_DATA.users.push({ id: newId, role: _addUserRole, name, loginId: email, email,
      password: newPw, avatar: name.slice(0, 1) });
    closeAddUserModal(); persistPasswordMasterDirectory();
    showToast('success', '登録完了', `${name} を登録しました。初期パスワード: ${newPw}`);
    return;
  }
  try {
    const created = await window.ZaikoAPI.createUser({ username: email, password: newPw, displayName: name,
      email, role: _addUserRole === 'buyer' ? 'worker' : _addUserRole, isPurchaseStaff: _addUserRole === 'buyer' });
    APP_DATA.users.push({ id: created.id, role: created.role, name: created.displayName, loginId: created.username,
      email: created.email, staffCode: created.staffCode, isPurchaseStaff: created.isPurchaseStaff,
      active: created.isActive, avatar: name.slice(0, 1), password: '', apiManaged: true });
    closeAddUserModal();
    refreshPasswordMasterDirectory();
    showToast('success', '登録完了', `${name} をDBへ登録しました。初期パスワードは安全な方法で本人へ共有してください`);
  } catch (error) {
    showToast('error', '登録エラー', error.message);
  }
}

// =====================================================
// 新規ゲスト登録モーダル
// =====================================================

/** ゲスト新規登録モーダルを開く */
function showAddGuestModal() {
  document.getElementById('addGuest-company').value = '';
  document.getElementById('addGuest-name').value    = '';
  document.getElementById('addGuest-email').value   = '';
  document.getElementById('addGuest-buyer').value   = '';
  document.getElementById('addGuest-pw').value      = _genPassword();
  document.getElementById('addGuestModal').classList.remove('hidden');
}

function closeAddGuestModal() {
  document.getElementById('addGuestModal').classList.add('hidden');
}

async function saveAddGuest() {
  const company   = document.getElementById('addGuest-company').value.trim();
  const name      = document.getElementById('addGuest-name').value.trim();
  const email     = document.getElementById('addGuest-email').value.trim();
  const buyerCode = document.getElementById('addGuest-buyer').value.trim().toUpperCase();
  const password  = document.getElementById('addGuest-pw').value.trim();

  if (!company)  { showToast('error', '入力エラー', '会社名を入力してください'); return; }
  if (!email)    { showToast('error', '入力エラー', 'メールアドレスを入力してください'); return; }
  if (!password) { showToast('error', '入力エラー', 'パスワードを入力してください'); return; }

  // 重複チェック
  if (APP_DATA.guestAccounts.some(g => g.email === email)) {
    showToast('error', '重複エラー', 'このメールアドレスはすでに登録されています'); return;
  }
  if (buyerCode && APP_DATA.guestAccounts.some(g => g.buyerCode === buyerCode)) {
    showToast('error', '重複エラー', 'この販売先にはすでにゲストログインが発行されています'); return;
  }

  if (!window.ZaikoAPI) {
    const existingIds = APP_DATA.guestAccounts.map(g => g.id).filter(id => /^G\d+$/.test(id)).map(id => parseInt(id.slice(1), 10));
    const newId = 'G' + String((existingIds.length ? Math.max(...existingIds) : 0) + 1).padStart(3, '0');
    const newGuest = { id: newId, name: name || company, company, email, password, buyerCode };
    APP_DATA.guestAccounts.push(newGuest);
    if (typeof ensureBuyerForGuest === 'function') ensureBuyerForGuest(newGuest, { code: buyerCode, name: company, email });
    closeAddGuestModal(); persistPasswordMasterDirectory();
    showToast('success', 'ゲスト登録完了', `${company} を登録しました。ゲストID: ${newId}`);
    return;
  }
  try {
    const created = await window.ZaikoAPI.createGuestWithPartner({ company, name, email, buyerCode, password });
    APP_DATA.guestAccounts.push({ id: created.guestCode, userId: created.id, name: created.displayName,
      company: created.companyName || company, email: created.email, buyerCode: created.buyerCode,
      partnerCode: created.partnerCode, active: created.isActive, password: '', apiManaged: true });
    closeAddGuestModal();
    refreshPasswordMasterDirectory();
    showToast('success', 'ゲスト登録完了', `${company} をDBへ登録しました。ゲストID: ${created.guestCode}`);
  } catch (error) {
    showToast('error', '登録エラー', error.message);
  }
}

// =====================================================
// マスタ内インライン描画 — 取引先会社
// =====================================================

function _clientTradeBadge(company) {
  const types = getClientCompanyTradeTypes(company);
  const both = types.length === 2;
  const color = both ? '#6d28d9' : types[0] === 'supplier' ? '#b45309' : '#0369a1';
  const background = both ? '#f3e8ff' : types[0] === 'supplier' ? '#fef3c7' : '#e0f2fe';
  return `<span style="display:inline-flex;align-items:center;white-space:nowrap;padding:3px 8px;border-radius:999px;font-size:11px;font-weight:700;color:${color};background:${background};">${_mEsc(getClientCompanyTradeLabel(company))}</span>`;
}

let _clientCompanyTradeFilter = 'all';

function _normalizeClientCompanyTradeFilter(value) {
  return ['all', 'buyer', 'supplier'].includes(value) ? value : 'all';
}

function _getFilteredClientCompanies() {
  const list = APP_DATA.clientCompanies || [];
  const filter = _normalizeClientCompanyTradeFilter(_clientCompanyTradeFilter);
  if (filter === 'all') return list;
  return list.filter(company => getClientCompanyTradeTypes(company).includes(filter));
}

function _clientCompanyFilterOptions() {
  return `
    <option value="all"${_clientCompanyTradeFilter === 'all' ? ' selected' : ''}>すべて</option>
    <option value="buyer"${_clientCompanyTradeFilter === 'buyer' ? ' selected' : ''}>販売先取引</option>
    <option value="supplier"${_clientCompanyTradeFilter === 'supplier' ? ' selected' : ''}>仕入先取引</option>`;
}

function _clientCompanyFilterCountText() {
  const total = (APP_DATA.clientCompanies || []).length;
  return `表示 ${_getFilteredClientCompanies().length} / 全 ${total} 件`;
}

function _renderClientCompanyFilter(filterId, countId) {
  return `
    <div role="search" aria-label="取引先会社の絞り込み" style="display:flex;align-items:flex-end;justify-content:space-between;gap:12px;flex-wrap:wrap;margin-bottom:12px;padding:10px 12px;background:#fafbfc;border:1px solid var(--border);border-radius:8px;">
      <div class="form-group" style="margin:0;min-width:190px;">
        <label class="form-label" for="${filterId}" style="font-size:11px;margin-bottom:4px;"><i class="fa-solid fa-filter"></i> 取引区分</label>
        <select class="form-control" id="${filterId}" onchange="setClientCompanyTradeFilter(this.value)" style="height:34px;font-size:12px;">
          ${_clientCompanyFilterOptions()}
        </select>
      </div>
      <span id="${countId}" aria-live="polite" style="font-size:12px;color:var(--text-muted);font-weight:600;white-space:nowrap;">${_clientCompanyFilterCountText()}</span>
    </div>`;
}

function _renderClientCompanyRows(list, emptyMessage = '取引先会社が登録されていません') {
  if (!list.length) {
    return `<tr><td colspan="12" style="text-align:center;padding:32px;color:var(--text-muted);">${_mEsc(emptyMessage)}</td></tr>`;
  }
  return list.map(company => `
    <tr data-client-id="${_mEsc(company.id)}" data-trade-types="${_mEsc(getClientCompanyTradeTypes(company).join(','))}">
      <td><code style="font-size:11px;white-space:nowrap;">${_mEsc(company.id)}</code></td>
      <td>${_clientTradeBadge(company)}</td>
      <td><code style="font-size:11px;white-space:nowrap;">${_mEsc(company.buyerCode || '—')}</code></td>
      <td><code style="font-size:11px;white-space:nowrap;">${_mEsc(company.supplierCode || '—')}</code></td>
      <td style="font-weight:700;min-width:170px;">${_mEsc(company.companyName)}</td>
      <td style="font-size:12px;white-space:nowrap;">${_mEsc(company.invoice || '—')}</td>
      <td style="font-size:12px;" title="${_mEsc(company.contactPerson || company.representative || '')}">${_mEsc(company.contactPerson || company.representative || '—')}</td>
      <td style="font-size:12px;" title="${_mEsc(company.email || '')}">${_mEsc(company.email || '—')}</td>
      <td style="font-size:12px;white-space:nowrap;" title="${_mEsc(company.tel || '')}">${_mEsc(company.tel || '—')}</td>
      <td style="font-size:12px;max-width:180px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;" title="${_mEsc(company.address || '')}">${_mEsc(company.address || '—')}</td>
      <td style="font-size:12px;max-width:130px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;" title="${_mEsc(company.note || '')}">${_mEsc(company.note || '—')}</td>
      <td>
        <div style="display:flex;gap:4px;white-space:nowrap;">
          <button class="btn btn-outline btn-sm" onclick="showClientModal('${company.id}')"><i class="fa-solid fa-pen"></i> 編集</button>
          <button class="btn btn-ghost btn-sm" style="color:var(--danger);" onclick="showClientDeleteModal('${company.id}')" title="取引マスタ連動中は削除できません"><i class="fa-solid fa-trash"></i></button>
        </div>
      </td>
    </tr>`).join('');
}

const CLIENT_COMPANY_TABLE_HEADER = `
  <tr>
    <th>取引先コード</th><th>取引区分</th><th>販売先コード</th><th>仕入先コード</th>
    <th>会社名</th><th>インボイス番号</th><th>担当者名</th><th>メールアドレス</th>
    <th>電話番号</th><th>住所</th><th>備考</th><th>操作</th>
  </tr>`;

const CLIENT_COMPANY_TABLE_COLGROUP = `
  <colgroup>
    <col style="width:82px"><col style="width:94px"><col style="width:80px"><col style="width:80px">
    <col style="width:160px"><col style="width:108px"><col style="width:88px"><col style="width:160px">
    <col style="width:100px"><col style="width:145px"><col style="width:80px"><col style="width:103px">
  </colgroup>`;

function renderClientMasterTab(area) {
  const list = _getFilteredClientCompanies();
  const emptyMessage = _clientCompanyTradeFilter === 'all'
    ? '取引先会社が登録されていません'
    : '選択した取引区分の取引先会社はありません';
  const rows = _renderClientCompanyRows(list, emptyMessage);

  area.innerHTML = `
    <div class="master-content">
      <div style="display:flex;align-items:center;gap:12px;margin-bottom:16px;">
        <h3 style="font-size:15px;font-weight:bold;color:var(--primary);"><i class="fa-solid fa-handshake"></i> 取引先会社</h3>
        <button class="btn btn-accent btn-sm" onclick="showClientModal()"><i class="fa-solid fa-plus"></i> 新規登録</button>
      </div>
      <div style="margin-bottom:12px;padding:10px 12px;border:1px solid #bae6fd;background:#f0f9ff;border-radius:8px;font-size:12px;color:#0c4a6e;">
        <i class="fa-solid fa-link"></i> 販売先・仕入先マスタと共通化されています。ゲスト発行先は販売先取引として自動連動します。
      </div>
      ${_renderClientCompanyFilter('mClientTradeFilter', 'mClientTradeFilterCount')}
      <div class="data-table-wrapper client-company-table-wrapper" tabindex="0" aria-label="取引先会社一覧。画面幅が狭い場合は横にスクロールできます">
        <table class="data-table client-company-table">
          ${CLIENT_COMPANY_TABLE_COLGROUP}
          <thead>${CLIENT_COMPANY_TABLE_HEADER}</thead>
          <tbody id="mClient-tbody">${rows}</tbody>
        </table>
      </div>
    </div>
  `;
}

function renderMasterClientTable() {
  const tbody = document.getElementById('mClient-tbody');
  if (!tbody) return;
  const list = _getFilteredClientCompanies();
  const emptyMessage = _clientCompanyTradeFilter === 'all'
    ? '取引先会社が登録されていません'
    : '選択した取引区分の取引先会社はありません';
  tbody.innerHTML = _renderClientCompanyRows(list, emptyMessage);
  const count = document.getElementById('mClientTradeFilterCount');
  if (count) count.textContent = _clientCompanyFilterCountText();
}

/** 取引先削除（マスタ内） */
function deleteMasterClient(id) {
  showClientDeleteModal(id);
}

// =====================================================
// マスタ内インライン描画 — 会社情報
// =====================================================

let _companyEditMode = false;
let _bankEditMode    = false;

function renderCompanyMasterTab(area) {
  _companyEditMode = false;
  _bankEditMode    = false;
  const ci = getCompanyInfo();
  area.innerHTML = `
    <div class="master-content">
      <div style="display:flex;align-items:center;gap:12px;margin-bottom:16px;">
        <h3 style="font-size:15px;font-weight:bold;color:var(--primary);"><i class="fa-solid fa-building-user"></i> 会社情報（自社）</h3>
      </div>

      <!-- 基本情報カード -->
      <div class="card mb-16" style="margin-bottom:16px;">
        <div class="card-header" style="padding:12px 16px;">
          <span style="font-weight:700;font-size:13px;"><i class="fa-solid fa-building"></i> 基本情報</span>
          <button class="btn btn-primary btn-sm" id="mCompanyEditBtn" onclick="toggleMCompanyEdit()"><i class="fa-solid fa-pen"></i> 編集</button>
        </div>
        <div class="card-body" style="padding:16px;">
          <div id="mCompanyViewArea">${_renderCompanyView(ci)}</div>
          <div id="mCompanyFormArea" style="display:none;">${_renderCompanyForm(ci)}</div>
          <div id="mCompanyEditActions" style="display:none;margin-top:12px;gap:8px;display:none;">
            <button class="btn btn-outline btn-sm" onclick="cancelMCompanyEdit()"><i class="fa-solid fa-xmark"></i> キャンセル</button>
            <button class="btn btn-primary btn-sm" onclick="saveMCompanyInfo()"><i class="fa-solid fa-check"></i> 保存</button>
          </div>
        </div>
      </div>

      <!-- 振込先カード -->
      <div class="card">
        <div class="card-header" style="padding:12px 16px;">
          <span style="font-weight:700;font-size:13px;"><i class="fa-solid fa-building-columns"></i> 振込先情報</span>
          <button class="btn btn-outline btn-sm" id="mBankEditBtn" onclick="toggleMBankEdit()"><i class="fa-solid fa-pen"></i> 振込先を編集</button>
        </div>
        <div class="card-body" style="padding:16px;">
          <div id="mBankViewArea">${_renderBankView(ci)}</div>
          <div id="mBankFormArea" style="display:none;">${_renderBankForm(ci)}</div>
          <div id="mBankEditActions" style="display:none;margin-top:12px;gap:8px;">
            <button class="btn btn-outline btn-sm" onclick="cancelMBankEdit()"><i class="fa-solid fa-xmark"></i> キャンセル</button>
            <button class="btn btn-primary btn-sm" onclick="saveMBankInfo()"><i class="fa-solid fa-check"></i> 保存</button>
          </div>
        </div>
      </div>
    </div>
  `;
}

function _renderCompanyView(ci) {
  const fields = [
    { label: '会社名',    val: ci.companyName  || '未設定' },
    { label: '郵便番号',  val: ci.zip          || '未設定' },
    { label: '住所',      val: ci.address      || '未設定' },
    { label: '電話番号',  val: ci.tel          || '未設定' },
    { label: 'FAX番号',   val: ci.fax          || '未設定' },
    { label: 'メール',    val: ci.email        || '未設定' },
    { label: '適格請求書登録番号', val: ci.invoice || '未設定' },
  ];
  return `<div class="form-row cols-2" style="gap:12px;">${
    fields.map(f => `
      <div>
        <div style="font-size:11px;color:var(--text-muted);margin-bottom:2px;">${f.label}</div>
        <div style="font-size:13px;font-weight:600;">${_mEsc(f.val)}</div>
      </div>`).join('')
  }</div>`;
}

function _renderCompanyForm(ci) {
  return `
    <div class="form-row cols-2" style="gap:12px;">
      <div class="form-group">
        <label class="form-label" style="font-size:12px;">会社名 <span class="required">*</span></label>
        <input type="text" class="form-control" id="mComp-name" value="${_mEsc(ci.companyName || '')}" placeholder="株式会社◯◯">
      </div>
      <div class="form-group">
        <label class="form-label" style="font-size:12px;">電話番号</label>
        <input type="text" class="form-control" id="mComp-tel" value="${_mEsc(ci.tel || '')}" placeholder="03-XXXX-XXXX">
      </div>
      <div class="form-group">
        <label class="form-label" style="font-size:12px;">郵便番号</label>
        <input type="text" class="form-control" id="mComp-zip" value="${_mEsc(ci.zip || '')}" placeholder="〒100-0001">
      </div>
      <div class="form-group">
        <label class="form-label" style="font-size:12px;">メールアドレス</label>
        <input type="email" class="form-control" id="mComp-email" value="${_mEsc(ci.email || '')}" placeholder="info@example.co.jp">
      </div>
      <div class="form-group">
        <label class="form-label" style="font-size:12px;">FAX番号</label>
        <input type="text" class="form-control" id="mComp-fax" value="${_mEsc(ci.fax || '')}" placeholder="03-XXXX-XXXX">
      </div>
      <div class="form-group">
        <label class="form-label" style="font-size:12px;">住所</label>
        <input type="text" class="form-control" id="mComp-address" value="${_mEsc(ci.address || '')}" placeholder="東京都◯◯区...">
      </div>
      <div class="form-group">
        <label class="form-label" style="font-size:12px;">適格請求書登録番号</label>
        <input type="text" class="form-control" id="mComp-invoice" value="${_mEsc(ci.invoice || '')}" placeholder="T1234567890123">
      </div>
    </div>`;
}

function _renderBankView(ci) {
  const fields = [
    { label: '銀行名',    val: ci.bankName     || '未設定' },
    { label: '支店名',    val: ci.branchName   || '未設定' },
    { label: '口座種別',  val: ci.accountType  || '普通' },
    { label: '口座番号',  val: ci.accountNumber || '未設定' },
    { label: '口座名義',  val: ci.accountHolder || '未設定' },
  ];
  return `<div class="form-row cols-2" style="gap:12px;">${
    fields.map(f => `
      <div>
        <div style="font-size:11px;color:var(--text-muted);margin-bottom:2px;">${f.label}</div>
        <div style="font-size:13px;font-weight:600;">${_mEsc(f.val)}</div>
      </div>`).join('')
  }</div>`;
}

function _renderBankForm(ci) {
  return `
    <div class="form-row cols-2" style="gap:12px;">
      <div class="form-group">
        <label class="form-label" style="font-size:12px;">銀行名</label>
        <input type="text" class="form-control" id="mBank-name" value="${_mEsc(ci.bankName || '')}" placeholder="◯◯銀行">
      </div>
      <div class="form-group">
        <label class="form-label" style="font-size:12px;">支店名</label>
        <input type="text" class="form-control" id="mBank-branch" value="${_mEsc(ci.branchName || '')}" placeholder="◯◯支店">
      </div>
      <div class="form-group">
        <label class="form-label" style="font-size:12px;">口座種別</label>
        <select class="form-control" id="mBank-type">
          <option value="普通" ${(ci.accountType || '普通') === '普通' ? 'selected' : ''}>普通</option>
          <option value="当座" ${ci.accountType === '当座' ? 'selected' : ''}>当座</option>
          <option value="貯蓄" ${ci.accountType === '貯蓄' ? 'selected' : ''}>貯蓄</option>
        </select>
      </div>
      <div class="form-group">
        <label class="form-label" style="font-size:12px;">口座番号</label>
        <input type="text" class="form-control" id="mBank-no" value="${_mEsc(ci.accountNumber || '')}" placeholder="1234567">
      </div>
      <div class="form-group">
        <label class="form-label" style="font-size:12px;">口座名義</label>
        <input type="text" class="form-control" id="mBank-holder" value="${_mEsc(ci.accountHolder || '')}" placeholder="カ）ウォッチプレミアム">
      </div>
    </div>`;
}

function toggleMCompanyEdit() {
  _companyEditMode = !_companyEditMode;
  document.getElementById('mCompanyViewArea').style.display  = _companyEditMode ? 'none' : '';
  document.getElementById('mCompanyFormArea').style.display  = _companyEditMode ? '' : 'none';
  document.getElementById('mCompanyEditActions').style.display = _companyEditMode ? 'flex' : 'none';
  document.getElementById('mCompanyEditBtn').innerHTML = _companyEditMode
    ? '<i class="fa-solid fa-xmark"></i> キャンセル'
    : '<i class="fa-solid fa-pen"></i> 編集';
}

function cancelMCompanyEdit() {
  _companyEditMode = false;
  const ci = getCompanyInfo();
  document.getElementById('mCompanyViewArea').innerHTML   = _renderCompanyView(ci);
  document.getElementById('mCompanyFormArea').innerHTML   = _renderCompanyForm(ci);
  document.getElementById('mCompanyViewArea').style.display  = '';
  document.getElementById('mCompanyFormArea').style.display  = 'none';
  document.getElementById('mCompanyEditActions').style.display = 'none';
  document.getElementById('mCompanyEditBtn').innerHTML = '<i class="fa-solid fa-pen"></i> 編集';
}

async function saveMCompanyInfo() {
  const name    = document.getElementById('mComp-name')?.value?.trim();
  const tel     = document.getElementById('mComp-tel')?.value?.trim();
  const email   = document.getElementById('mComp-email')?.value?.trim();
  const address = document.getElementById('mComp-address')?.value?.trim();
  if (!name) { showToast('error', '入力エラー', '会社名を入力してください'); return; }
  const ci = getCompanyInfo();
  ci.companyName = name;
  ci.zip         = document.getElementById('mComp-zip')?.value?.trim() || '';
  ci.address     = address;
  ci.tel         = tel;
  ci.fax         = document.getElementById('mComp-fax')?.value?.trim() || '';
  ci.email       = email;
  ci.invoice     = document.getElementById('mComp-invoice')?.value?.trim() || '';
  try {
    if (!window.ZaikoAPI) throw null;
    await window.ZaikoAPI.saveCompany();
  } catch (error) {
    if (!error) { /* isolated UI test fallback */ }
    else {
    showToast('error', '保存エラー', error.message);
    return;
    }
  }
  document.getElementById('mCompanyViewArea').innerHTML   = _renderCompanyView(ci);
  document.getElementById('mCompanyFormArea').innerHTML   = _renderCompanyForm(ci);
  document.getElementById('mCompanyViewArea').style.display  = '';
  document.getElementById('mCompanyFormArea').style.display  = 'none';
  document.getElementById('mCompanyEditActions').style.display = 'none';
  document.getElementById('mCompanyEditBtn').innerHTML = '<i class="fa-solid fa-pen"></i> 編集';
  _companyEditMode = false;
  showToast('success', '保存完了', '会社情報を更新しました');
}

function toggleMBankEdit() {
  _bankEditMode = !_bankEditMode;
  document.getElementById('mBankViewArea').style.display    = _bankEditMode ? 'none' : '';
  document.getElementById('mBankFormArea').style.display    = _bankEditMode ? '' : 'none';
  document.getElementById('mBankEditActions').style.display = _bankEditMode ? 'flex' : 'none';
  document.getElementById('mBankEditBtn').innerHTML = _bankEditMode
    ? '<i class="fa-solid fa-xmark"></i> キャンセル'
    : '<i class="fa-solid fa-pen"></i> 振込先を編集';
}

function cancelMBankEdit() {
  _bankEditMode = false;
  const ci = getCompanyInfo();
  document.getElementById('mBankViewArea').innerHTML  = _renderBankView(ci);
  document.getElementById('mBankFormArea').innerHTML  = _renderBankForm(ci);
  document.getElementById('mBankViewArea').style.display    = '';
  document.getElementById('mBankFormArea').style.display    = 'none';
  document.getElementById('mBankEditActions').style.display = 'none';
  document.getElementById('mBankEditBtn').innerHTML = '<i class="fa-solid fa-pen"></i> 振込先を編集';
}

async function saveMBankInfo() {
  const bankName    = document.getElementById('mBank-name')?.value?.trim();
  const branchName  = document.getElementById('mBank-branch')?.value?.trim();
  const accountNumber = document.getElementById('mBank-no')?.value?.trim();
  const accountHolder = document.getElementById('mBank-holder')?.value?.trim();
  const ci = getCompanyInfo();
  ci.bankName      = bankName;
  ci.branchName    = branchName;
  ci.accountType   = document.getElementById('mBank-type')?.value || '普通';
  ci.accountNumber = accountNumber;
  ci.accountHolder = accountHolder;
  try {
    if (!window.ZaikoAPI) throw null;
    await window.ZaikoAPI.saveCompany();
  } catch (error) {
    if (!error) { /* isolated UI test fallback */ }
    else {
    showToast('error', '保存エラー', error.message);
    return;
    }
  }
  document.getElementById('mBankViewArea').innerHTML  = _renderBankView(ci);
  document.getElementById('mBankFormArea').innerHTML  = _renderBankForm(ci);
  document.getElementById('mBankViewArea').style.display    = '';
  document.getElementById('mBankFormArea').style.display    = 'none';
  document.getElementById('mBankEditActions').style.display = 'none';
  document.getElementById('mBankEditBtn').innerHTML = '<i class="fa-solid fa-pen"></i> 振込先を編集';
  _bankEditMode = false;
  showToast('success', '保存完了', '振込先情報を更新しました');
}

/** パスワード管理ページ初期化 */
function init_password() {
  if (!isAdmin()) {
    showToast('error', 'アクセス拒否', 'このページは管理者のみアクセスできます');
    navigateTo('dashboard');
    return;
  }
  // API未接続時も日次の管理者認証コードを準備する
  _checkApprovalCodeRefresh();
  // アクティブタブを管理者にリセット
  switchPwTab('admin', document.getElementById('pw-tab-admin-btn'));
}

/** 後方互換: API未接続時の日次管理者認証コードを準備 */
function _checkApprovalCodeRefresh() {
  _getLocalAdminAccessCode(false);
}

/** タブ切替 */
function switchPwTab(tab, btn) {
  ['admin', 'buyer', 'guest'].forEach(t => {
    const panel = document.getElementById(`pw-tab-${t}`);
    const b     = document.getElementById(`pw-tab-${t}-btn`);
    if (panel) panel.style.display = t === tab ? '' : 'none';
    if (b)     b.classList.toggle('active', t === tab);
  });
  if (tab === 'admin')  renderPwAdminTable();
  if (tab === 'buyer')  renderPwBuyerTable();
  if (tab === 'guest')  renderPwGuestTable();
}

/** 管理者テーブル描画 */
function renderPwAdminTable() {
  const tbody = document.getElementById('pw-admin-tbody');
  if (!tbody) return;
  const admins = APP_DATA.users.filter(u => u.role === 'admin');
  tbody.innerHTML = admins.length === 0
    ? '<tr><td colspan="5" style="text-align:center;color:var(--text-muted);padding:24px;">管理者アカウントがありません</td></tr>'
    : admins.map(u => `
      <tr>
        <td style="font-weight:700;">${_mEsc(u.name)}</td>
        <td style="font-size:12px;">${_mEsc(u.email || u.loginId)}</td>
        <td><span class="badge badge-stock">● 有効</span></td>
        <td>
          <span style="font-family:monospace;font-size:14px;font-weight:700;letter-spacing:3px;color:#1d4ed8;background:#eff6ff;padding:3px 10px;border-radius:6px;">${_mEsc(u.approvalCode || '——')}</span>
          <span style="font-size:10px;color:var(--text-muted);margin-left:6px;">更新: ${_mEsc(u.approvalCodeUpdatedAt || '—')}</span>
        </td>
        <td>
          <button class="btn btn-outline btn-sm" onclick="showPwChangeModal('${u.id}', false)"><i class="fa-solid fa-key"></i> PW変更</button>
        </td>
      </tr>`).join('');
}

/** 作業者テーブル描画 */
function renderPwBuyerTable() {
  const tbody = document.getElementById('pw-buyer-tbody');
  if (!tbody) return;
  const buyers = APP_DATA.users.filter(u => u.role === 'buyer');
  tbody.innerHTML = buyers.length === 0
    ? '<tr><td colspan="4" style="text-align:center;color:var(--text-muted);padding:24px;">作業者アカウントがありません</td></tr>'
    : buyers.map(u => `
      <tr>
        <td style="font-weight:700;">${_mEsc(u.name)}</td>
        <td style="font-size:12px;">${_mEsc(u.email || u.loginId)}</td>
        <td><span class="badge badge-stock">● 有効</span></td>
        <td>
          <button class="btn btn-outline btn-sm" onclick="showPwChangeModal('${u.id}', false)"><i class="fa-solid fa-key"></i> PW変更</button>
          <button class="btn btn-ghost btn-sm" style="color:var(--danger);" onclick="deleteUserAccount('${u.id}')"><i class="fa-solid fa-trash"></i></button>
        </td>
      </tr>`).join('');
}

/** ゲストテーブル描画 */
function renderPwGuestTable() {
  const tbody = document.getElementById('pw-guest-tbody');
  if (!tbody) return;
  tbody.innerHTML = APP_DATA.guestAccounts.length === 0
    ? '<tr><td colspan="5" style="text-align:center;color:var(--text-muted);padding:24px;">ゲストアカウントがありません</td></tr>'
    : APP_DATA.guestAccounts.map(g => `
      <tr>
        <td style="font-weight:700;">${_mEsc(g.company || g.name)}</td>
        <td style="font-size:12px;font-family:monospace;">${_mEsc(g.id)}</td>
        <td style="font-size:12px;">${_mEsc(g.email || '—')}</td>
        <td style="font-family:monospace;letter-spacing:2px;">
          <span class="pw-mask" data-gid="${g.id}">••••••••</span>
          <button class="btn btn-ghost btn-sm" style="padding:2px 6px;" onclick="toggleGuestPwMask('${g.id}')">
            <i class="fa-regular fa-eye" id="pw-eye-${g.id}"></i>
          </button>
          <span class="pw-plain" id="pw-plain-${g.id}" style="display:none;color:#1d4ed8;">再設定のみ</span>
        </td>
        <td>
          <button class="btn btn-outline btn-sm" onclick="showPwChangeModal('${g.id}', true)"><i class="fa-solid fa-key"></i> PW変更</button>
        </td>
      </tr>`).join('');
}

/** パスワードマスク切替 */
function toggleGuestPwMask(gid) {
  const mask  = document.querySelector(`.pw-mask[data-gid="${gid}"]`);
  const plain = document.getElementById(`pw-plain-${gid}`);
  const eye   = document.getElementById(`pw-eye-${gid}`);
  if (!mask || !plain) return;
  const showing = plain.style.display !== 'none';
  mask.style.display  = showing ? '' : 'none';
  plain.style.display = showing ? 'none' : '';
  if (eye) eye.className = showing ? 'fa-regular fa-eye' : 'fa-regular fa-eye-slash';
}

/** パスワード変更モーダルを開く */
function showPwChangeModal(userId, isGuest) {
  _pwEditState = { userId, isGuest };
  let target;
  if (isGuest) {
    target = APP_DATA.guestAccounts.find(g => g.id === userId);
  } else {
    target = APP_DATA.users.find(u => u.id === userId);
  }
  if (!target) return;

  const name = isGuest ? (target.company || target.name) : target.name;
  document.getElementById('pwChangeModalTitle').textContent = `パスワード変更 — ${name}`;
  document.getElementById('pwChangeModalBody').innerHTML = `
    <div style="margin-bottom:14px;padding:10px 14px;background:#f0f9ff;border:1px solid #bae6fd;border-radius:8px;font-size:12px;color:#0c4a6e;">
      <i class="fa-solid fa-user"></i> <strong>${_mEsc(name)}</strong> のパスワードを変更します
    </div>
    <div class="form-group" style="margin-bottom:14px;">
      <label class="form-label" style="font-size:12px;">新しいパスワード <span class="required">*</span></label>
      <div style="display:flex;gap:8px;">
        <input type="password" class="form-control" id="pwChange-new" placeholder="8文字以上の新しいパスワード" style="font-size:13px;font-family:monospace;" value="">
        <button class="btn btn-outline btn-sm" style="white-space:nowrap;" onclick="document.getElementById('pwChange-new').value=_genPassword()">
          <i class="fa-solid fa-rotate"></i> 自動生成
        </button>
      </div>
    </div>`;
  document.getElementById('pwChangeModal').classList.remove('hidden');
}

/** パスワード変更を保存 */
async function savePwChange() {
  const { userId, isGuest } = _pwEditState;
  const newPw = document.getElementById('pwChange-new')?.value?.trim();
  if (!newPw) { showToast('error', '入力エラー', 'パスワードを入力してください'); return; }

  if (newPw.length < 8) { showToast('error', '入力エラー', 'パスワードは8文字以上で入力してください'); return; }
  if (!window.ZaikoAPI) {
    const target = isGuest ? APP_DATA.guestAccounts.find(x => x.id === userId) : APP_DATA.users.find(x => x.id === userId);
    if (target) target.password = newPw;
    closePwChangeModal(); renderPwAdminTable(); renderPwBuyerTable(); renderPwGuestTable();
    return;
  }
  try {
    await window.ZaikoAPI.changePassword(userId, isGuest, newPw);
    closePwChangeModal();
    showToast('success', 'パスワード変更完了', 'DB上のパスワードハッシュを更新しました');
    renderPwAdminTable(); renderPwBuyerTable(); renderPwGuestTable();
  } catch (error) { showToast('error', '更新エラー', error.message); }
}

function closePwChangeModal() {
  document.getElementById('pwChangeModal').classList.add('hidden');
  _pwEditState = { userId: null, isGuest: false };
}

// showAddUserModal / closeAddUserModal / saveAddUser は新実装に統合済み（上部で定義）

/** ユーザー削除（作業者のみ） */
async function deleteUserAccount(userId) {
  const u = APP_DATA.users.find(x => x.id === userId);
  if (!u) return;
  if (!confirm(`「${u.name}」を削除してよろしいですか？`)) return;
  if (!window.ZaikoAPI) {
    APP_DATA.users = APP_DATA.users.filter(x => x.id !== userId);
    renderPwBuyerTable();
    return;
  }
  try {
    await window.ZaikoAPI.setUserActive(userId, false, false);
    u.active = false;
    renderPwBuyerTable();
    showToast('success', '停止完了', `${u.name} のログインを停止しました`);
  } catch (error) { showToast('error', '停止エラー', error.message); }
}

/** ゲストパスワード一括変更 */
async function bulkChangeGuestPasswords() {
  if (!confirm('全ゲストへパスワード再設定案内を発行してもよろしいですか？')) return;
  if (!window.ZaikoAPI) {
    APP_DATA.guestAccounts.forEach(g => { g.password = _genPassword(); });
    renderPwGuestTable(); persistPasswordMasterDirectory();
    return;
  }
  try {
    await Promise.all(APP_DATA.guestAccounts.filter(g => g.active !== false).map(g => window.ZaikoAPI.queuePasswordReset(g.id, true)));
    showToast('success', '再設定案内を発行しました', `${APP_DATA.guestAccounts.filter(g => g.active !== false).length} 件をメール送信キューへ登録しました`);
  } catch (error) { showToast('error', '発行エラー', error.message); }
}

/** ゲストのパスワード再設定案内を送信キューへ登録 */
async function sendGuestPasswordEmails() {
  if (!window.ZaikoAPI) {
    showToast('success', '通知メール送信（テスト）', `${APP_DATA.guestAccounts.length} 社へ通知しました`);
    return;
  }
  try {
    const guests = APP_DATA.guestAccounts.filter(g => g.active !== false);
    await Promise.all(guests.map(g => window.ZaikoAPI.queuePasswordReset(g.id, true)));
    showToast('success', '通知を受け付けました', `${guests.length} 社分をメール送信キューへ登録しました`);
  } catch (error) { showToast('error', '通知エラー', error.message); }
}

// =====================================================
// 取引先会社ページ
// =====================================================

let _clientDeleteId = null;

function init_client() {
  if (!isAdmin()) {
    showToast('error', 'アクセス拒否', 'このページは管理者のみアクセスできます');
    navigateTo('dashboard');
    return;
  }
  renderClientTable();
}

function renderClientTable() {
  const tbody = document.getElementById('clientTableBody');
  if (!tbody) return;
  const list = _getFilteredClientCompanies();
  const emptyMessage = _clientCompanyTradeFilter === 'all'
    ? '取引先会社が登録されていません'
    : '選択した取引区分の取引先会社はありません';
  tbody.innerHTML = _renderClientCompanyRows(list, emptyMessage);
  const count = document.getElementById('clientTradeFilterCount');
  if (count) count.textContent = _clientCompanyFilterCountText();
}

function setClientCompanyTradeFilter(value) {
  _clientCompanyTradeFilter = _normalizeClientCompanyTradeFilter(value);
  ['mClientTradeFilter', 'clientTradeFilter'].forEach(id => {
    const select = document.getElementById(id);
    if (select) select.value = _clientCompanyTradeFilter;
  });
  renderMasterClientTable();
  renderClientTable();
}

function showClientModal(id) {
  const modal = document.getElementById('clientModal');
  if (!modal) return;
  if (id) {
    const c = (APP_DATA.clientCompanies || []).find(x => x.id === id);
    if (!c) return;
    document.getElementById('clientModalTitle').textContent = '取引先会社 編集';
    document.getElementById('clientModal-id').value             = c.id;
    document.getElementById('clientModal-code').value           = c.id;
    const types = getClientCompanyTradeTypes(c);
    document.getElementById('clientModal-tradeType').value      = types.length === 2 ? 'both' : (types[0] || 'buyer');
    document.getElementById('clientModal-buyerCode').value      = c.buyerCode || '';
    document.getElementById('clientModal-supplierCode').value   = c.supplierCode || '';
    document.getElementById('clientModal-companyName').value   = c.companyName || '';
    document.getElementById('clientModal-representative').value = c.representative || '';
    document.getElementById('clientModal-contactPerson').value  = c.contactPerson || '';
    document.getElementById('clientModal-email').value          = c.email || '';
    document.getElementById('clientModal-tel').value            = c.tel || '';
    document.getElementById('clientModal-address').value        = c.address || '';
    document.getElementById('clientModal-invoice').value        = c.invoice || '';
    document.getElementById('clientModal-note').value           = c.note || '';
  } else {
    document.getElementById('clientModalTitle').textContent = '取引先会社 登録';
    ['id','companyName','representative','contactPerson','email','tel','address','invoice','note'].forEach(f => {
      const el = document.getElementById(`clientModal-${f}`);
      if (el) el.value = '';
    });
    document.getElementById('clientModal-code').value = getNextClientCompanyCode();
    document.getElementById('clientModal-tradeType').value = 'buyer';
    document.getElementById('clientModal-buyerCode').value = _nextTradeMasterCode('B', APP_DATA.buyers || []);
    document.getElementById('clientModal-supplierCode').value = '';
  }
  clientTradeTypeChanged();
  modal.classList.remove('hidden');
}

function clientTradeTypeChanged() {
  const tradeType = document.getElementById('clientModal-tradeType')?.value || 'buyer';
  const buyerEnabled = tradeType === 'buyer' || tradeType === 'both';
  const supplierEnabled = tradeType === 'supplier' || tradeType === 'both';
  const buyerInput = document.getElementById('clientModal-buyerCode');
  const supplierInput = document.getElementById('clientModal-supplierCode');
  if (buyerInput) {
    if (buyerEnabled && !buyerInput.value) buyerInput.value = _nextTradeMasterCode('B', APP_DATA.buyers || []);
    buyerInput.disabled = !buyerEnabled;
  }
  if (supplierInput) {
    if (supplierEnabled && !supplierInput.value) supplierInput.value = _nextTradeMasterCode('S', APP_DATA.suppliers || []);
    supplierInput.disabled = !supplierEnabled;
  }
}

function closeClientModal() {
  document.getElementById('clientModal')?.classList.add('hidden');
}

async function saveClientModal() {
  const companyName = document.getElementById('clientModal-companyName').value.trim();
  if (!companyName) { showToast('error', '入力エラー', '会社名を入力してください'); return; }

  if (!APP_DATA.clientCompanies) APP_DATA.clientCompanies = [];
  const existingId = document.getElementById('clientModal-id').value;
  const companyCode = document.getElementById('clientModal-code').value.trim().toUpperCase();
  const tradeType = document.getElementById('clientModal-tradeType').value;
  const original = existingId ? APP_DATA.clientCompanies.find(company => company.id === existingId) : null;
  const selectedTypes = tradeType === 'both' ? ['buyer', 'supplier'] : [tradeType];
  const tradeTypes = [...new Set([...getClientCompanyTradeTypes(original), ...selectedTypes])];
  if (!/^CLI-[0-9]+$/.test(companyCode)) {
    showToast('error', '取引先コードを確認してください', '取引先コードは CLI-001 の形式で自動採番されます');
    return;
  }
  const entry = {
    ...(original || {}),
    autoManaged: original?.autoManaged ?? true,
    id: existingId || companyCode,
    tradeTypes,
    buyerCode: tradeTypes.includes('buyer') ? document.getElementById('clientModal-buyerCode').value.trim().toUpperCase() : '',
    supplierCode: tradeTypes.includes('supplier') ? document.getElementById('clientModal-supplierCode').value.trim().toUpperCase() : '',
    companyName,
    representative: document.getElementById('clientModal-representative').value.trim(),
    contactPerson:  document.getElementById('clientModal-contactPerson').value.trim(),
    email:          document.getElementById('clientModal-email').value.trim(),
    tel:            document.getElementById('clientModal-tel').value.trim(),
    address:        document.getElementById('clientModal-address').value.trim(),
    invoice:        document.getElementById('clientModal-invoice').value.trim().toUpperCase(),
    note:           document.getElementById('clientModal-note').value.trim(),
  };

  const duplicateBuyer = entry.buyerCode && (APP_DATA.clientCompanies || []).some(company =>
    company.id !== entry.id && company.buyerCode === entry.buyerCode);
  const duplicateSupplier = entry.supplierCode && (APP_DATA.clientCompanies || []).some(company =>
    company.id !== entry.id && company.supplierCode === entry.supplierCode);
  if (duplicateBuyer || duplicateSupplier) {
    showToast('error', '取引コードが重複しています', duplicateBuyer ? '販売先コードは別の取引先会社に使用されています' : '仕入先コードは別の取引先会社に使用されています');
    return;
  }

  let saved;
  if (!window.ZaikoAPI) {
    applyClientCompanyToTradeMasters(entry);
    if (existingId) {
      const idx = APP_DATA.clientCompanies.findIndex(x => x.id === existingId);
      if (idx >= 0) APP_DATA.clientCompanies[idx] = entry;
    } else APP_DATA.clientCompanies.push(entry);
    persistSupplierMasterDirectory();
    if (typeof persistLoginDirectory === 'function') persistLoginDirectory();
    reconcileClientCompanyDirectory({ persist: true }); closeClientModal(); renderClientTable(); renderMasterClientTable(); renderMasterTabs();
    return;
  }
  try {
    saved = await window.ZaikoAPI.savePartner(entry);
  } catch (error) {
    showToast('error', '保存エラー', error.message);
    return;
  }
  entry._id = saved.id;
  entry.id = saved.partnerCode;
  entry.buyerCode = saved.roles?.find(role => role.roleType === 'buyer')?.roleCode || '';
  entry.supplierCode = saved.roles?.find(role => role.roleType === 'supplier')?.roleCode || '';
  applyClientCompanyToTradeMasters(entry);

  if (existingId) {
    const idx = APP_DATA.clientCompanies.findIndex(x => x.id === existingId);
    if (idx >= 0) APP_DATA.clientCompanies[idx] = entry;
  } else {
    APP_DATA.clientCompanies.push(entry);
    _clientCompanyNextSequence = Math.max(_clientCompanyNextSequence, _clientCompanySequence(entry.id) + 1);
  }
  persistSupplierMasterDirectory();
  if (typeof persistLoginDirectory === 'function') persistLoginDirectory();
  reconcileClientCompanyDirectory({ persist: true });
  closeClientModal();
  renderClientTable();
  renderMasterClientTable();
  renderMasterTabs();
  showToast('success', existingId ? '更新完了' : '登録完了', `「${companyName}」を${existingId ? '更新' : '登録'}しました`);
}

function showClientDeleteModal(id) {
  const c = (APP_DATA.clientCompanies || []).find(x => x.id === id);
  if (!c) return;
  if (getClientCompanyTradeTypes(c).length > 0) {
    showToast('error', '連動中の取引先会社は削除できません', '販売先または仕入先マスタで使用されています。各マスタの取引履歴を確認してください');
    return;
  }
  _clientDeleteId = id;
  document.getElementById('clientDeleteMsg').textContent = `「${c.companyName}」を削除してもよろしいですか？`;
  document.getElementById('clientDeleteModal').classList.remove('hidden');
}

function closeClientDeleteModal() {
  document.getElementById('clientDeleteModal')?.classList.add('hidden');
  _clientDeleteId = null;
}

function confirmClientDelete() {
  if (!_clientDeleteId) return;
  const c = (APP_DATA.clientCompanies || []).find(x => x.id === _clientDeleteId);
  APP_DATA.clientCompanies = (APP_DATA.clientCompanies || []).filter(x => x.id !== _clientDeleteId);
  persistClientCompanyDirectory();
  closeClientDeleteModal();
  renderClientTable();
  renderMasterClientTable();
  renderMasterTabs();
  showToast('success', '削除完了', `「${c?.companyName || ''}」を削除しました`);
}

// =====================================================
// 会社情報ページ（自社）— 旧スタンドアロン実装（page-company用）
// =====================================================

function init_company() {
  if (!isAdmin()) {
    showToast('error', 'アクセス拒否', 'このページは管理者のみアクセスできます');
    navigateTo('dashboard');
    return;
  }
  _companyEditMode = false;
  _bankEditMode    = false;
  renderCompanyInfo();
  renderBankInfo();
}

/** 会社情報エリアを描画 */
function renderCompanyInfo() {
  const area = document.getElementById('companyFormArea');
  if (!area) return;
  const ci = getCompanyInfo();
  const editBtn    = document.getElementById('companyEditBtn');
  const editActions = document.getElementById('companyEditActions');

  if (_companyEditMode) {
    area.innerHTML = `
      <div class="form-group" style="margin-bottom:14px;">
        <label class="form-label" style="font-size:12px;">会社名 <span class="required">*</span></label>
        <input type="text" class="form-control" id="ci-companyName" value="${_mEsc(ci.companyName || '')}" style="font-size:13px;">
      </div>
      <div class="form-group" style="margin-bottom:14px;">
        <label class="form-label" style="font-size:12px;">郵便番号</label>
        <input type="text" class="form-control" id="ci-zip" value="${_mEsc(ci.zip || '')}" style="font-size:13px;">
      </div>
      <div class="form-group" style="margin-bottom:14px;">
        <label class="form-label" style="font-size:12px;">住所</label>
        <input type="text" class="form-control" id="ci-address" value="${_mEsc(ci.address || '')}" style="font-size:13px;">
      </div>
      <div class="form-group" style="margin-bottom:14px;">
        <label class="form-label" style="font-size:12px;">電話番号</label>
        <input type="text" class="form-control" id="ci-tel" value="${_mEsc(ci.tel || '')}" style="font-size:13px;">
      </div>
      <div class="form-group" style="margin-bottom:14px;">
        <label class="form-label" style="font-size:12px;">FAX番号</label>
        <input type="text" class="form-control" id="ci-fax" value="${_mEsc(ci.fax || '')}" style="font-size:13px;">
      </div>
      <div class="form-group" style="margin-bottom:14px;">
        <label class="form-label" style="font-size:12px;">メールアドレス</label>
        <input type="email" class="form-control" id="ci-email" value="${_mEsc(ci.email || '')}" style="font-size:13px;">
      </div>
      <div class="form-group" style="margin-bottom:14px;">
        <label class="form-label" style="font-size:12px;">適格請求書登録番号</label>
        <input type="text" class="form-control" id="ci-invoice" value="${_mEsc(ci.invoice || '')}" style="font-size:13px;">
      </div>`;
    if (editBtn)     editBtn.style.display     = 'none';
    if (editActions) editActions.style.display = 'flex';
  } else {
    area.innerHTML = `
      <div class="form-group" style="margin-bottom:14px;">
        <label class="form-label" style="font-size:12px;color:var(--text-muted);">会社名</label>
        <div style="font-size:14px;font-weight:700;color:var(--text);padding:8px 0;">${_mEsc(ci.companyName || '—')}</div>
      </div>
      <div class="form-group" style="margin-bottom:14px;">
        <label class="form-label" style="font-size:12px;color:var(--text-muted);">郵便番号</label>
        <div style="font-size:13px;color:var(--text);padding:6px 0;">${_mEsc(ci.zip || '—')}</div>
      </div>
      <div class="form-group" style="margin-bottom:14px;">
        <label class="form-label" style="font-size:12px;color:var(--text-muted);">住所</label>
        <div style="font-size:13px;color:var(--text);padding:6px 0;">${_mEsc(ci.address || '—')}</div>
      </div>
      <div class="form-group" style="margin-bottom:14px;">
        <label class="form-label" style="font-size:12px;color:var(--text-muted);">電話番号</label>
        <div style="font-size:13px;color:var(--text);padding:6px 0;">${_mEsc(ci.tel || '—')}</div>
      </div>
      <div class="form-group" style="margin-bottom:14px;">
        <label class="form-label" style="font-size:12px;color:var(--text-muted);">FAX番号</label>
        <div style="font-size:13px;color:var(--text);padding:6px 0;">${_mEsc(ci.fax || '—')}</div>
      </div>
      <div class="form-group" style="margin-bottom:14px;">
        <label class="form-label" style="font-size:12px;color:var(--text-muted);">メールアドレス</label>
        <div style="font-size:13px;color:var(--text);padding:6px 0;">${_mEsc(ci.email || '—')}</div>
      </div>
      <div class="form-group" style="margin-bottom:14px;">
        <label class="form-label" style="font-size:12px;color:var(--text-muted);">適格請求書登録番号</label>
        <div style="font-size:13px;color:var(--text);padding:6px 0;">${_mEsc(ci.invoice || '—')}</div>
      </div>`;
    if (editBtn)     editBtn.style.display     = '';
    if (editActions) editActions.style.display = 'none';
  }
}

function toggleCompanyEdit() {
  _companyEditMode = true;
  renderCompanyInfo();
}

function cancelCompanyEdit() {
  _companyEditMode = false;
  renderCompanyInfo();
}

async function saveCompanyInfo() {
  const name = document.getElementById('ci-companyName')?.value?.trim();
  if (!name) { showToast('error', '入力エラー', '会社名を入力してください'); return; }
  const ci = getCompanyInfo();
  ci.companyName = name;
  ci.zip         = document.getElementById('ci-zip')?.value?.trim() || '';
  ci.address     = document.getElementById('ci-address')?.value?.trim() || '';
  ci.tel         = document.getElementById('ci-tel')?.value?.trim() || '';
  ci.fax         = document.getElementById('ci-fax')?.value?.trim() || '';
  ci.email       = document.getElementById('ci-email')?.value?.trim() || '';
  ci.invoice     = document.getElementById('ci-invoice')?.value?.trim() || '';
  try {
    if (!window.ZaikoAPI) throw null;
    await window.ZaikoAPI.saveCompany();
  } catch (error) {
    if (!error) { /* isolated UI test fallback */ }
    else {
    showToast('error', '保存エラー', error.message);
    return;
    }
  }
  _companyEditMode = false;
  renderCompanyInfo();
  showToast('success', '保存完了', '会社情報を更新しました');
}

/** 振込先情報エリアを描画 */
function renderBankInfo() {
  const area = document.getElementById('bankFormArea');
  if (!area) return;
  const ci = getCompanyInfo();
  const editBtn     = document.getElementById('bankEditBtn');
  const editActions = document.getElementById('bankEditActions');

  if (_bankEditMode) {
    area.innerHTML = `
      <div class="form-group" style="margin-bottom:14px;">
        <label class="form-label" style="font-size:12px;">銀行名</label>
        <input type="text" class="form-control" id="bank-bankName" value="${_mEsc(ci.bankName || '')}" style="font-size:13px;" placeholder="例: 三菱UFJ銀行">
      </div>
      <div class="form-group" style="margin-bottom:14px;">
        <label class="form-label" style="font-size:12px;">支店名</label>
        <input type="text" class="form-control" id="bank-branchName" value="${_mEsc(ci.branchName || '')}" style="font-size:13px;" placeholder="例: 虎ノ門支店">
      </div>
      <div class="form-group" style="margin-bottom:14px;">
        <label class="form-label" style="font-size:12px;">口座種別</label>
        <select class="form-control" id="bank-accountType" style="font-size:13px;">
          <option value="普通" ${(ci.accountType || '普通') === '普通' ? 'selected' : ''}>普通</option>
          <option value="当座" ${ci.accountType === '当座' ? 'selected' : ''}>当座</option>
          <option value="貯蓄" ${ci.accountType === '貯蓄' ? 'selected' : ''}>貯蓄</option>
        </select>
      </div>
      <div class="form-group" style="margin-bottom:14px;">
        <label class="form-label" style="font-size:12px;">口座番号</label>
        <input type="text" class="form-control" id="bank-accountNumber" value="${_mEsc(ci.accountNumber || '')}" style="font-size:13px;" placeholder="例: 1234567">
      </div>
      <div class="form-group" style="margin-bottom:14px;">
        <label class="form-label" style="font-size:12px;">口座名義</label>
        <input type="text" class="form-control" id="bank-accountHolder" value="${_mEsc(ci.accountHolder || '')}" style="font-size:13px;" placeholder="例: カ）ウォッチプレミアム">
      </div>`;
    if (editBtn)     editBtn.style.display     = 'none';
    if (editActions) editActions.style.display = 'flex';
  } else {
    area.innerHTML = `
      <div class="form-group" style="margin-bottom:14px;">
        <label class="form-label" style="font-size:12px;color:var(--text-muted);">銀行名</label>
        <div style="font-size:13px;color:var(--text);padding:6px 0;">${_mEsc(ci.bankName || '—')}</div>
      </div>
      <div class="form-group" style="margin-bottom:14px;">
        <label class="form-label" style="font-size:12px;color:var(--text-muted);">支店名</label>
        <div style="font-size:13px;color:var(--text);padding:6px 0;">${_mEsc(ci.branchName || '—')}</div>
      </div>
      <div class="form-group" style="margin-bottom:14px;">
        <label class="form-label" style="font-size:12px;color:var(--text-muted);">口座種別</label>
        <div style="font-size:13px;color:var(--text);padding:6px 0;">${_mEsc(ci.accountType || '普通')}</div>
      </div>
      <div class="form-group" style="margin-bottom:14px;">
        <label class="form-label" style="font-size:12px;color:var(--text-muted);">口座番号</label>
        <div style="font-size:13px;color:var(--text);padding:6px 0;">${_mEsc(ci.accountNumber || '—')}</div>
      </div>
      <div class="form-group" style="margin-bottom:14px;">
        <label class="form-label" style="font-size:12px;color:var(--text-muted);">口座名義</label>
        <div style="font-size:13px;color:var(--text);padding:6px 0;">${_mEsc(ci.accountHolder || '—')}</div>
      </div>`;
    if (editBtn)     editBtn.style.display     = '';
    if (editActions) editActions.style.display = 'none';
  }
}

function toggleBankEdit() {
  _bankEditMode = true;
  renderBankInfo();
}

function cancelBankEdit() {
  _bankEditMode = false;
  renderBankInfo();
}

async function saveBankInfo() {
  const ci = getCompanyInfo();
  ci.bankName      = document.getElementById('bank-bankName')?.value?.trim() || '';
  ci.branchName    = document.getElementById('bank-branchName')?.value?.trim() || '';
  ci.accountType   = document.getElementById('bank-accountType')?.value || '普通';
  ci.accountNumber = document.getElementById('bank-accountNumber')?.value?.trim() || '';
  ci.accountHolder = document.getElementById('bank-accountHolder')?.value?.trim() || '';
  try {
    if (!window.ZaikoAPI) throw null;
    await window.ZaikoAPI.saveCompany();
  } catch (error) {
    if (!error) { /* isolated UI test fallback */ }
    else {
    showToast('error', '保存エラー', error.message);
    return;
    }
  }
  _bankEditMode = false;
  renderBankInfo();
  showToast('success', '保存完了', '振込先情報を更新しました');
}

// =====================================================
// 返品/持ち帰り ページ
// =====================================================

// =====================================================
// 返品/持ち帰り ページ — 新仕様
// ・伝票単位でリスト表示
// ・クリックで伝票詳細モーダル（対象商品にチェックボックス）
// ・作業者 → 承認リクエスト送信
// ・管理者 → BOX振り分け選択 → 即時在庫戻し
// =====================================================

// 返品/持ち帰り商品が含まれる売上伝票をまとめる
function getReturnSlips() {
  const result = [];
  APP_DATA.sales.forEach(sale => {
    const returnItems = (sale.items || []).filter(it => it.returnType);
    if (returnItems.length === 0) return;
    const pendingCount = returnItems.filter(it => (it.returnStatus || 'pending') !== 'done').length;
    result.push({
      saleId:      sale.id,
      saleDate:    sale.date,
      buyer:       sale.buyer,
      items:       returnItems,
      pendingCount,
      totalCount:  returnItems.length,
      sourceType:  'sale-takeback',
    });
  });
  (APP_DATA.salesReturns || []).forEach(returnSlip => {
    const items = returnSlip.items || [];
    if (items.length === 0) return;
    const isDone = returnSlip.status === '処理済';
    result.push({
      saleId: returnSlip.id,
      saleDate: returnSlip.date,
      buyer: returnSlip.buyer,
      items,
      pendingCount: isDone ? 0 : items.length,
      totalCount: items.length,
      sourceType: 'return-document',
    });
  });
  return result;
}

// 伝票リストを描画
function renderReturnSlipList() {
  const statusFilter = document.getElementById('ret-filter-status')?.value || '';
  const keyword      = (document.getElementById('ret-filter-keyword')?.value || '').toLowerCase();
  const tbody  = document.getElementById('returnsSlipBody');
  const noData = document.getElementById('returnsNoData');
  if (!tbody) return;

  let slips = getReturnSlips();

  // フィルタ
  if (statusFilter === 'pending') slips = slips.filter(s => s.pendingCount > 0);
  if (statusFilter === 'done')    slips = slips.filter(s => s.pendingCount === 0);
  if (keyword) slips = slips.filter(s =>
    [s.saleId, getBuyerName(s.buyer), ...s.items.map(it => `${it.brand} ${it.model} ${it.code}`)].join(' ').toLowerCase().includes(keyword)
  );

  if (slips.length === 0) {
    tbody.innerHTML = '';
    noData?.classList.remove('hidden');
    return;
  }
  noData?.classList.add('hidden');

  tbody.innerHTML = slips.map(slip => {
    const pendingBadge = slip.pendingCount > 0
      ? `<span style="display:inline-flex;align-items:center;gap:4px;background:#fef2f2;color:#dc2626;border:1px solid #fca5a5;border-radius:12px;padding:2px 10px;font-size:11px;font-weight:bold;">
           <i class="fa-solid fa-clock"></i> ${slip.pendingCount}件 処理待ち
         </span>`
      : `<span style="display:inline-flex;align-items:center;gap:4px;background:#f0fdf4;color:#16a34a;border:1px solid #86efac;border-radius:12px;padding:2px 10px;font-size:11px;">
           <i class="fa-solid fa-circle-check"></i> 完了
         </span>`;
    const detailAction = slip.sourceType === 'return-document'
      ? `openSalesReturnDetail('${slip.saleId}')`
      : `openReturnSlipModal('${slip.saleId}')`;
    return `<tr style="cursor:pointer;" onclick="${detailAction}">
      <td><code style="font-size:12px;font-weight:bold;">${slip.saleId}</code></td>
      <td style="white-space:nowrap;">${slip.saleDate}</td>
      <td>${getBuyerName(slip.buyer)}</td>
      <td style="text-align:center;">${slip.totalCount}件</td>
      <td>${pendingBadge}</td>
      <td style="text-align:center;">
        <button class="btn btn-outline btn-sm" onclick="event.stopPropagation();${detailAction}">
          <i class="fa-solid fa-magnifying-glass"></i>
        </button>
      </td>
    </tr>`;
  }).join('');
}

// 伝票詳細モーダルを開く
function openReturnSlipModal(saleId) {
  const sale = APP_DATA.sales.find(s => s.id === saleId);
  if (!sale) return;

  document.getElementById('ret-slip-saleid').value = saleId;
  document.getElementById('returnSlipTitle').textContent = `返品/持ち帰り確認 — ${saleId}`;

  // 伝票情報ヘッダー
  const netTotal   = sale.items.reduce((s, i) => s + (i.returnType ? 0 : i.salePrice), 0);
  const grossTotal = sale.items.reduce((s, i) => s + i.salePrice, 0);
  const returnAmt  = grossTotal - netTotal;
  document.getElementById('returnSlipInfo').innerHTML = `
    <div style="display:grid;grid-template-columns:repeat(3,1fr);gap:10px 20px;margin-bottom:10px;">
      <div>
        <span style="font-size:11px;color:var(--text-muted);">売上伝票番号</span><br>
        <code style="font-size:14px;font-weight:bold;">${sale.id}</code>
      </div>
      <div>
        <span style="font-size:11px;color:var(--text-muted);">売上日</span><br>
        <span style="font-size:13px;">${sale.date}</span>
      </div>
      <div>
        <span style="font-size:11px;color:var(--text-muted);">販売先</span><br>
        <span style="font-size:13px;font-weight:500;">${getBuyerName(sale.buyer)}</span>
      </div>
    </div>
    <div style="display:flex;gap:16px;flex-wrap:wrap;padding-top:8px;border-top:1px solid var(--border);">
      <span style="font-size:12px;">伝票合計（USD）: <b>${formatSalePrice(grossTotal)}</b></span>
      ${returnAmt > 0
        ? `<span style="font-size:12px;color:#dc2626;">返品/持ち帰り計: <b>− ${formatSalePrice(returnAmt)}</b></span>
           <span style="font-size:12px;color:var(--primary);">実売上合計（USD）: <b>${formatSalePrice(netTotal)}</b></span>`
        : ''}
      ${sale.note ? `<span style="font-size:12px;color:var(--text-muted);">備考: ${sale.note}</span>` : ''}
    </div>`;

  // 全明細を表示 — 返品/持ち帰り商品にはチェックボックス、通常商品は参照表示
  const allItems = sale.items || [];
  const returnItems = allItems.filter(it => it.returnType);

  if (returnItems.length === 0) {
    document.getElementById('returnItemsList').innerHTML = `
      <div style="text-align:center;padding:24px;color:var(--text-muted);font-size:13px;">
        <i class="fa-solid fa-circle-info" style="font-size:24px;margin-bottom:8px;display:block;"></i>
        この伝票に返品・持ち帰り対象の商品はありません
      </div>`;
    document.getElementById('returnToStockBtn').style.display = 'none';
    document.getElementById('returnSlipModal').classList.remove('hidden');
    return;
  }
  document.getElementById('returnToStockBtn').style.display = '';

  let html = '';

  // ① 返品/持ち帰り対象商品（チェックボックス付き）
  html += `<div style="font-size:11px;font-weight:bold;color:var(--text-muted);text-transform:uppercase;letter-spacing:.5px;margin-bottom:8px;">
    <i class="fa-solid fa-rotate-left"></i> 返品・持ち帰り対象商品
  </div>`;
  returnItems.forEach((it, idx) => {
    const isDone = it.returnStatus === 'done';
    const invItem = APP_DATA.inventory.find(i => i.code === it.code);
    const conditionCode = invItem?.condition || '';
    const condName = APP_DATA.conditions.find(c => c.code === conditionCode)?.name || conditionCode || '—';

    const typeBadge = it.returnType === 'return'
      ? `<span style="display:inline-flex;align-items:center;gap:3px;background:#fef2f2;color:#dc2626;border:1px solid #fca5a5;border-radius:10px;padding:2px 9px;font-size:11px;font-weight:bold;"><i class="fa-solid fa-rotate-left"></i> 返品</span>`
      : `<span style="display:inline-flex;align-items:center;gap:3px;background:#fff7ed;color:#ea580c;border:1px solid #fed7aa;border-radius:10px;padding:2px 9px;font-size:11px;font-weight:bold;"><i class="fa-solid fa-hand-holding-box"></i> 持ち帰り</span>`;
    const doneBadge = isDone
      ? `<span style="display:inline-flex;align-items:center;gap:3px;background:#f0fdf4;color:#16a34a;border:1px solid #86efac;border-radius:10px;padding:2px 9px;font-size:11px;"><i class="fa-solid fa-circle-check"></i> 在庫戻し済</span>`
      : `<span style="display:inline-flex;align-items:center;gap:3px;background:#fef9c3;color:#854d0e;border:1px solid #fde047;border-radius:10px;padding:2px 9px;font-size:11px;"><i class="fa-solid fa-clock"></i> 処理待ち</span>`;

    html += `
      <div style="display:flex;align-items:flex-start;gap:12px;padding:14px;border:2px solid ${isDone ? '#bbf7d0' : '#bfdbfe'};border-radius:10px;margin-bottom:10px;background:${isDone ? '#f0fdf4' : '#f8faff'};">
        <div style="padding-top:1px;flex-shrink:0;">
          ${isDone
            ? `<i class="fa-solid fa-circle-check" style="font-size:18px;color:#16a34a;"></i>`
            : `<input type="checkbox" class="return-item-chk" data-code="${it.code}" id="retChk_${idx}"
                style="width:18px;height:18px;cursor:pointer;accent-color:var(--primary);margin-top:1px;">`
          }
        </div>
        <div style="flex:1;min-width:0;">
          <div style="display:flex;gap:8px;flex-wrap:wrap;align-items:center;margin-bottom:8px;">
            <code style="font-size:12px;background:#eff6ff;color:#1d4ed8;padding:1px 6px;border-radius:4px;">${it.code}</code>
            <strong style="font-size:14px;">${it.brand} ${it.model}</strong>
            ${typeBadge}
            ${doneBadge}
          </div>
          <div style="display:flex;gap:20px;flex-wrap:wrap;font-size:12px;color:var(--text-muted);margin-bottom:${isDone ? 0 : '10px'};">
            <span>売価（USD）: <b style="color:var(--text);font-size:13px;">${formatSalePrice(it.salePrice)}</b></span>
            <span>現コンディション: <b style="color:var(--text);">${condName}</b></span>
            ${invItem?.serial ? `<span>シリアル: <code style="font-size:11px;">${invItem.serial}</code></span>` : ''}
          </div>
          ${!isDone ? `
          <div style="display:flex;gap:10px;flex-wrap:wrap;align-items:center;background:#fff;border:1px solid #dbeafe;border-radius:8px;padding:8px 12px;">
            <div style="display:flex;gap:6px;align-items:center;">
              <label style="font-size:12px;color:var(--text-muted);white-space:nowrap;font-weight:500;">確認後のコンディション:</label>
              <select class="form-control return-item-condition" data-code="${it.code}"
                style="min-width:190px;font-size:12px;height:30px;padding:2px 8px;">
                <option value="">変更なし（現状のまま）</option>
                ${APP_DATA.conditions.map(c =>
                  `<option value="${c.code}"${conditionCode === c.code ? ' selected' : ''}>${c.code} — ${c.name}</option>`
                ).join('')}
              </select>
            </div>
            <div style="display:flex;gap:6px;align-items:center;">
              <label style="font-size:12px;color:var(--text-muted);white-space:nowrap;font-weight:500;">数量確認:</label>
              <input type="number" class="form-control return-item-qty" data-code="${it.code}"
                value="1" min="1" style="width:65px;font-size:12px;height:30px;padding:2px 6px;">
              <span style="font-size:12px;color:var(--text-muted);">点</span>
            </div>
          </div>` : ''}
        </div>
      </div>`;
  });

  // ② 通常販売商品（参照用・チェックボックスなし）
  const normalItems = allItems.filter(it => !it.returnType);
  if (normalItems.length > 0) {
    html += `<div style="font-size:11px;font-weight:bold;color:var(--text-muted);text-transform:uppercase;letter-spacing:.5px;margin:16px 0 8px;">
      <i class="fa-solid fa-check"></i> 通常販売商品（参照）
    </div>`;
    normalItems.forEach(it => {
      html += `
        <div style="display:flex;align-items:center;gap:12px;padding:10px 14px;border:1px solid #e2e8f0;border-radius:8px;margin-bottom:8px;background:#fff;opacity:0.7;">
          <i class="fa-solid fa-circle-minus" style="color:#9ca3af;font-size:14px;flex-shrink:0;"></i>
          <div>
            <code style="font-size:11px;">${it.code}</code>
            <span style="font-size:13px;margin-left:8px;">${it.brand} ${it.model}</span>
            <span style="font-size:12px;color:var(--text-muted);margin-left:12px;">${formatSalePrice(it.salePrice)}</span>
          </div>
        </div>`;
    });
  }

  document.getElementById('returnItemsList').innerHTML = html;

  // ボタンラベル
  const btnLabel = document.getElementById('returnToStockBtnLabel');
  if (btnLabel) btnLabel.textContent = isBuyer() ? '在庫に戻す（承認申請）' : '在庫に戻す';

  document.getElementById('returnSlipModal').classList.remove('hidden');
}

function closeReturnSlipModal() {
  document.getElementById('returnSlipModal').classList.add('hidden');
}

// 「在庫に戻す」ボタン押下
function submitReturnToStock() {
  const saleId = document.getElementById('ret-slip-saleid')?.value;
  const sale = APP_DATA.sales.find(s => s.id === saleId);
  if (!sale) return;

  // チェックされた商品を収集
  const checked = [...document.querySelectorAll('.return-item-chk:checked')];
  if (checked.length === 0) {
    showToast('warning', '商品を選択してください', '在庫に戻す商品のチェックボックスにチェックを入れてください。');
    return;
  }

  // 選択商品データ収集
  const targets = checked.map(chk => {
    const code = chk.dataset.code;
    const condEl  = document.querySelector(`.return-item-condition[data-code="${code}"]`);
    const qtyEl   = document.querySelector(`.return-item-qty[data-code="${code}"]`);
    const saleItem = sale.items.find(it => it.code === code);
    const invItem  = APP_DATA.inventory.find(i => i.code === code);
    return {
      code,
      saleItem,
      invItem,
      condition: condEl?.value || '',
      qty: parseInt(qtyEl?.value) || 1,
    };
  }).filter(t => t.saleItem && t.invItem);

  if (targets.length === 0) {
    showToast('error', 'エラー', '対象商品が在庫データに見つかりません。');
    return;
  }

  if (isBuyer()) {
    // 作業者 → 承認リクエスト送信
    const detail = {
      saleId,
      items: targets.map(t => ({ code: t.code, brand: t.invItem.brand, model: t.invItem.model, condition: t.condition, qty: t.qty })),
    };
    requestApproval(
      'return_to_stock', '返品/持ち帰り 在庫戻し',
      detail, '',
      () => _execReturnToStock(saleId, targets, '管理者（承認済）', null)
    );
    closeReturnSlipModal();
    showToast('info', '承認申請を送信しました', `${targets.length}件の在庫戻し申請を管理者へ送信しました。`);
  } else {
    // 管理者 → BOX振り分けモーダルへ
    _openReturnBoxModal(saleId, targets);
  }
}

// 管理者用BOX振り分けモーダルを開く
function _openReturnBoxModal(saleId, targets) {
  // boxOptionsを生成
  const boxOpts = `<option value="">振り分けなし</option>` +
    [1,2,3,4,5,6,7,8,9,10].map(n => {
      const box = (APP_DATA.boxes || []).find(b => b.no === n);
      return `<option value="${n}">BOX ${n}${box ? ` — ${box.name}` : ''}</option>`;
    }).join('');

  const rows = targets.map((t, i) => `
    <div style="display:flex;align-items:center;gap:10px;padding:10px 0;border-bottom:1px solid var(--border);flex-wrap:wrap;">
      <div style="flex:1;min-width:0;">
        <div style="display:flex;gap:6px;align-items:center;flex-wrap:wrap;">
          <code style="font-size:12px;">${t.code}</code>
          <strong style="font-size:13px;">${t.invItem.brand} ${t.invItem.model}</strong>
        </div>
        ${t.condition ? `<div style="font-size:11px;color:var(--text-muted);margin-top:2px;">コンディション変更: ${t.condition}</div>` : ''}
      </div>
      <div style="display:flex;gap:8px;align-items:center;flex-shrink:0;">
        <label style="font-size:12px;color:var(--text-muted);white-space:nowrap;">BOX:</label>
        <select class="form-control ret-box-select" data-code="${t.code}"
          style="width:180px;font-size:12px;height:30px;padding:2px 8px;">
          ${boxOpts}
        </select>
      </div>
    </div>`).join('');

  // グローバルにターゲットを保存（scriptタグ不要）
  window._returnBoxSaleId  = saleId;
  window._returnBoxTargets = targets.map(t => ({ code: t.code, condition: t.condition, qty: t.qty }));

  document.getElementById('returnBoxItemsList').innerHTML =
    `${rows}
     <div class="form-group" style="margin-top:14px;">
       <label class="form-label">備考・管理者コメント（任意）</label>
       <textarea class="form-control" id="ret-box-note" rows="2" placeholder="処理内容のメモなど"></textarea>
     </div>`;

  document.getElementById('returnBoxModal').classList.remove('hidden');
}

function closeReturnBoxModal() {
  document.getElementById('returnBoxModal').classList.add('hidden');
}

// 管理者：BOX振り分け確定 → 在庫に戻す実行
function execReturnToStock() {
  const saleId  = window._returnBoxSaleId;
  const note    = document.getElementById('ret-box-note')?.value || '';
  const rawTargets = window._returnBoxTargets || [];
  const sale = APP_DATA.sales.find(s => s.id === saleId);
  if (!sale) return;

  // BOX選択値をtargetsにマージ
  const targets = rawTargets.map(t => {
    const boxEl = document.querySelector(`.ret-box-select[data-code="${t.code}"]`);
    const boxNo = parseInt(boxEl?.value) || null;
    const saleItem = sale.items.find(it => it.code === t.code);
    const invItem  = APP_DATA.inventory.find(i => i.code === t.code);
    return { ...t, boxNo, saleItem, invItem };
  }).filter(t => t.saleItem && t.invItem);

  _execReturnToStock(saleId, targets, null, note);
  closeReturnBoxModal();
}

// 在庫戻し共通実行関数
function _execReturnToStock(saleId, targets, approverName, note) {
  const now = new Date();
  const at  = now.toLocaleString('ja-JP', {
    year:'numeric', month:'2-digit', day:'2-digit', hour:'2-digit', minute:'2-digit'
  }).replace(/\//g, '-');

  targets.forEach(t => {
    const inv = t.invItem;
    const si  = t.saleItem;
    if (!inv || !si) return;

    const beforeCond = inv.condition;
    const beforeBox  = inv.boxNo || '—';

    inv.status = '在庫中';
    if (t.condition) inv.condition = t.condition;
    if (t.boxNo)     inv.boxNo     = t.boxNo;

    if (!inv.editHistory) inv.editHistory = [];
    const changes = [
      { field:'ステータス', before:'売上済', after:'在庫中' },
    ];
    if (t.condition && t.condition !== beforeCond)
      changes.push({ field:'コンディション', before: beforeCond || '—', after: t.condition });
    if (t.boxNo)
      changes.push({ field:'BOX', before: beforeBox, after: `BOX${t.boxNo}` });

    inv.editHistory.unshift({
      editedAt:     at,
      editorName:   currentUser()?.name || '—',
      approverName: approverName || null,
      changes,
      note: `${si.returnType === 'return' ? '返品' : '持ち帰り'}処理で在庫に戻す。${note || ''}`,
    });

    // 売上伝票の返品ステータスを完了に
    si.returnStatus = 'done';
  });

  const boxMsg = targets.some(t => t.boxNo)
    ? '（BOX振り分け済）'
    : '';
  showToast('success', '在庫に戻しました',
    `${targets.length}件を「在庫中」に変更しました${boxMsg}`);

  // 画面更新
  closeReturnSlipModal();
  renderReturnSlipList();
  refreshLinkedBusinessViews({ source: 'return-to-stock' });
}

// 旧関数の後方互換エイリアス（他箇所から呼ばれている場合に備え）
function renderReturnsList() { renderReturnSlipList(); }
function openReturnDetail(type, saleId, itemCode) { openReturnSlipModal(saleId); }
function closeReturnDetail() { closeReturnSlipModal(); }

// =====================================================
// ドロワー開閉（スマホ用）
// =====================================================
function toggleAppDrawer() {
  const sidebar = document.querySelector('.sidebar');
  const overlay = document.getElementById('drawerOverlay');
  const isOpen  = sidebar.classList.contains('open');
  if (isOpen) {
    closeAppDrawer();
  } else {
    sidebar.classList.add('open');
    overlay.classList.add('open');
    document.body.style.overflow = 'hidden';
  }
}

function closeAppDrawer() {
  const sidebar = document.querySelector('.sidebar');
  const overlay = document.getElementById('drawerOverlay');
  if (!sidebar) return;
  sidebar.classList.remove('open');
  overlay?.classList.remove('open');
  document.body.style.overflow = '';
}

// =====================================================
// ダッシュボード管理 — マスタ内インライン描画
// =====================================================

/**
 * カンマ区切り文字列 → 数値（内部保持用）
 * 全角数字・カンマ・数値以外を除去
 */
function _parseDashAmount(val) {
  if (val === null || val === undefined) return 0;
  const half = String(val).replace(/[０-９]/g, c =>
    String.fromCharCode(c.charCodeAt(0) - 0xFEE0)
  );
  return parseInt(half.replace(/[^0-9]/g, ''), 10) || 0;
}

/** 数値 → カンマ区切り表示文字列（¥なし） */
function _formatDashAmountInput(num) {
  if (!num) return '';
  return Number(num).toLocaleString('ja-JP');
}

/** 金額入力欄 oninput: リアルタイムカンマ付与 */
function onDashAmountInput(el) {
  const num    = _parseDashAmount(el.value);
  const pos    = el.selectionStart;
  const oldLen = el.value.length;
  el.value     = num > 0 ? _formatDashAmountInput(num) : '';
  const newLen = el.value.length;
  try { el.setSelectionRange(pos + (newLen - oldLen), pos + (newLen - oldLen)); } catch(e) {}
}

/** 金額入力欄 onblur: 最終整形 */
function onDashAmountBlur(el) {
  const num = _parseDashAmount(el.value);
  el.value  = num > 0 ? _formatDashAmountInput(num) : '';
}

/**
 * ダッシュボード管理タブを描画
 */
function renderDashboardMasterTab(area) {
  const ds  = APP_DATA.dashboardSettings || {};
  const stV = ds.salesTarget    > 0 ? _formatDashAmountInput(ds.salesTarget)    : '';
  const pbV = ds.purchaseBudget > 0 ? _formatDashAmountInput(ds.purchaseBudget) : '';

  area.innerHTML = `
    <div class="master-content">
      <div style="display:flex;align-items:center;gap:12px;margin-bottom:20px;">
        <h3 style="font-size:15px;font-weight:bold;color:var(--primary);">
          <i class="fa-solid fa-gauge"></i> ダッシュボード管理
        </h3>
      </div>

      <!-- 説明バナー -->
      <div style="background:#eff6ff;border:1px solid #bfdbfe;border-radius:8px;padding:14px 18px;
                  display:flex;align-items:flex-start;gap:12px;margin-bottom:20px;">
        <i class="fa-solid fa-circle-info" style="color:var(--primary);font-size:16px;margin-top:2px;"></i>
        <div style="font-size:12px;color:#1e40af;line-height:1.7;">
          ここで設定した金額はダッシュボードの各KPIカードにリアルタイム反映されます。<br>
          <b>今月売上目標</b> → 「今月売上」カードの前月比の下に表示<br>
          <b>今月仕入予算</b> → 「今月仕入金額」カードの仕入点数の下に表示
        </div>
      </div>

      <div class="card" style="margin-bottom:16px;">
        <div class="card-header" style="padding:12px 16px;">
          <span style="font-weight:700;font-size:13px;">
            <i class="fa-solid fa-bullseye"></i> 今月の目標・予算設定
          </span>
        </div>
        <div class="card-body" style="padding:20px;">

          <!-- 今月売上目標金額 -->
          <div class="form-group" style="margin-bottom:20px;">
            <label class="form-label" style="font-size:13px;font-weight:700;">
              <i class="fa-solid fa-dollar-sign" style="color:#f97316;margin-right:4px;"></i>
              今月売上目標金額（USD）
            </label>
            <div style="display:flex;align-items:center;gap:8px;">
              <span style="font-size:14px;color:var(--text-muted);">$</span>
              <input type="text" inputmode="numeric"
                class="form-control"
                id="dashMasterSalesTarget"
                value="${_mEsc(stV)}"
                placeholder="例: 35,000"
                style="max-width:240px;font-size:15px;font-family:monospace;letter-spacing:1px;"
                oninput="onDashAmountInput(this)"
                onchange="onDashAmountInput(this)"
                onblur="onDashAmountBlur(this)">
            </div>
            <div style="font-size:11px;color:var(--text-muted);margin-top:5px;">
              ダッシュボード「今月売上」エリアの前月比の下に表示されます
            </div>
          </div>

          <!-- 今月仕入予算額 -->
          <div class="form-group" style="margin-bottom:20px;">
            <label class="form-label" style="font-size:13px;font-weight:700;">
              <i class="fa-solid fa-file-import" style="color:#10b981;margin-right:4px;"></i>
              今月仕入予算額
            </label>
            <div style="display:flex;align-items:center;gap:8px;">
              <span style="font-size:14px;color:var(--text-muted);">¥</span>
              <input type="text" inputmode="numeric"
                class="form-control"
                id="dashMasterPurchaseBudget"
                value="${_mEsc(pbV)}"
                placeholder="例: 8,000,000"
                style="max-width:240px;font-size:15px;font-family:monospace;letter-spacing:1px;"
                oninput="onDashAmountInput(this)"
                onchange="onDashAmountInput(this)"
                onblur="onDashAmountBlur(this)">
            </div>
            <div style="font-size:11px;color:var(--text-muted);margin-top:5px;">
              ダッシュボード「今月仕入金額」エリアの仕入点数の下に表示されます
            </div>
          </div>

          <!-- 保存ボタン -->
          <div style="display:flex;gap:10px;align-items:center;margin-top:4px;">
            <button class="btn btn-primary" onclick="saveDashboardSettings()">
              <i class="fa-solid fa-floppy-disk"></i> 設定を保存
            </button>
            <button class="btn btn-outline btn-sm" onclick="resetDashboardSettings()">
              <i class="fa-solid fa-rotate"></i> リセット
            </button>
          </div>
        </div>
      </div>

      <!-- 現在の設定値プレビュー -->
      <div class="card">
        <div class="card-header" style="padding:12px 16px;">
          <span style="font-weight:700;font-size:13px;">
            <i class="fa-solid fa-eye"></i> ダッシュボード表示プレビュー
          </span>
        </div>
        <div class="card-body" style="padding:16px;" id="dashMasterPreview">
          ${_buildDashPreview(ds)}
        </div>
      </div>
    </div>
  `;
}

/** ダッシュボード設定プレビューHTML */
function _buildDashPreview(ds) {
  const st = ds?.salesTarget    || 0;
  const pb = ds?.purchaseBudget || 0;
  return `
    <div style="display:grid;grid-template-columns:1fr 1fr;gap:12px;">
      <div style="background:#fff7ed;border:1px solid #fed7aa;border-radius:8px;padding:12px 16px;">
        <div style="font-size:11px;color:#c2410c;font-weight:700;margin-bottom:4px;">
          <i class="fa-solid fa-dollar-sign"></i> 今月売上エリア
        </div>
        <div style="font-size:11px;color:var(--text-muted);">目標金額</div>
        <div style="font-size:16px;font-weight:700;color:#f97316;">
          ${st > 0 ? '目標 ' + formatSalePrice(st) : '<span style="color:var(--text-muted);">未設定</span>'}
        </div>
      </div>
      <div style="background:#f0fdf4;border:1px solid #bbf7d0;border-radius:8px;padding:12px 16px;">
        <div style="font-size:11px;color:#15803d;font-weight:700;margin-bottom:4px;">
          <i class="fa-solid fa-file-import"></i> 今月仕入エリア
        </div>
        <div style="font-size:11px;color:var(--text-muted);">予算金額</div>
        <div style="font-size:16px;font-weight:700;color:#10b981;">
          ${pb > 0 ? '予算 ¥' + pb.toLocaleString('ja-JP') : '<span style="color:var(--text-muted);">未設定</span>'}
        </div>
      </div>
    </div>
  `;
}

/** ダッシュボード設定を保存 → ダッシュボードへ即時反映 */
async function saveDashboardSettings() {
  const stEl = document.getElementById('dashMasterSalesTarget');
  const pbEl = document.getElementById('dashMasterPurchaseBudget');
  if (!stEl || !pbEl) return;

  const salesTarget    = _parseDashAmount(stEl.value);
  const purchaseBudget = _parseDashAmount(pbEl.value);

  // ⑥ 数値として内部保持（文字列保持禁止）
  if (!APP_DATA.dashboardSettings) APP_DATA.dashboardSettings = {};
  APP_DATA.dashboardSettings.salesTarget    = salesTarget;
  APP_DATA.dashboardSettings.purchaseBudget = purchaseBudget;
  try {
    if (!window.ZaikoAPI) throw null;
    await window.ZaikoAPI.saveDashboardSettings(salesTarget, purchaseBudget);
  } catch (error) {
    if (!error) { /* isolated UI test fallback */ }
    else {
    showToast('error', '保存エラー', error.message);
    return;
    }
  }

  // プレビュー更新
  const prevEl = document.getElementById('dashMasterPreview');
  if (prevEl) prevEl.innerHTML = _buildDashPreview(APP_DATA.dashboardSettings);

  // ④ ダッシュボードへ即時反映
  _applyDashboardSettings();

  showToast('success', '設定を保存しました', 'ダッシュボードに反映されました');
}

/** ダッシュボード設定をリセット */
async function resetDashboardSettings() {
  if (!confirm('設定をリセットしてよろしいですか？')) return;
  if (!APP_DATA.dashboardSettings) APP_DATA.dashboardSettings = {};
  APP_DATA.dashboardSettings.salesTarget    = 0;
  APP_DATA.dashboardSettings.purchaseBudget = 0;
  try {
    if (!window.ZaikoAPI) throw null;
    await window.ZaikoAPI.saveDashboardSettings(0, 0);
  } catch (error) {
    if (!error) { /* isolated UI test fallback */ }
    else {
    showToast('error', 'リセットエラー', error.message);
    return;
    }
  }

  // 入力欄をクリア
  const stEl = document.getElementById('dashMasterSalesTarget');
  const pbEl = document.getElementById('dashMasterPurchaseBudget');
  if (stEl) stEl.value = '';
  if (pbEl) pbEl.value = '';

  // プレビュー更新
  const prevEl = document.getElementById('dashMasterPreview');
  if (prevEl) prevEl.innerHTML = _buildDashPreview(APP_DATA.dashboardSettings);

  _applyDashboardSettings();
  showToast('info', 'リセットしました', 'ダッシュボード目標・予算を削除しました');
}

/**
 * ④⑤ APP_DATA.dashboardSettings の値をダッシュボード KPI カードへ反映
 * ・dashSalesTarget   : 今月売上カードの「前月比」の下
 * ・dashPurchaseBudget: 今月仕入カードの「仕入点数」の下
 */
function _applyDashboardSettings() {
  const ds = APP_DATA.dashboardSettings || {};
  const st = ds.salesTarget    || 0;
  const pb = ds.purchaseBudget || 0;

  // 今月売上目標
  const stEl = document.getElementById('dashSalesTarget');
  if (stEl) {
    if (st > 0) {
      stEl.style.display = '';
      stEl.innerHTML =
        `<i class="fa-solid fa-bullseye" style="color:#f97316;margin-right:3px;"></i>` +
        `<span style="color:#c2410c;font-weight:600;">目標 ${formatDashboardSales(st)}</span>`;
    } else {
      stEl.style.display = 'none';
      stEl.innerHTML = '';
    }
  }

  // 今月仕入予算
  const pbEl = document.getElementById('dashPurchaseBudget');
  if (pbEl) {
    if (pb > 0) {
      pbEl.style.display = '';
      pbEl.innerHTML =
        `<i class="fa-solid fa-coins" style="color:#10b981;margin-right:3px;"></i>` +
        `<span style="color:#15803d;font-weight:600;">予算 ${formatDashboardPurchase(pb)}</span>`;
    } else {
      pbEl.style.display = 'none';
      pbEl.innerHTML = '';
    }
  }
}
