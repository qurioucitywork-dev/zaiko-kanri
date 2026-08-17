// =====================================================
// 仕入登録（Purchase Entry）モジュール
// =====================================================
// ・すべての状態はフロントエンドで完結（モック）
// ・商品コード・伝票番号の重複は絶対禁止
// =====================================================

'use strict';

// ── 状態変数 ──────────────────────────────────────────
let _peSlipData  = null;   // 現在編集中の伝票オブジェクト
let _peLineCount = 0;      // 明細の累積カウンタ（行ID用）
let _peProductTargetLineId = null; // 商品登録ポップアップの対象明細ID
let _peSaveInFlight = false; // 登録APIの多重送信防止
const PE_PURCHASE_TAX_DOMESTIC = 'domestic';
const PE_PURCHASE_TAX_OVERSEAS = 'overseas';
const PE_DOMESTIC_TAX_RATE_BASIS_POINTS = 1000;
const PE_PURCHASE_CSV_HEADERS = Object.freeze([
  '仕入日', '仕入先コード', '仕入担当者コード', '仕入区分',
  'SKU', '売価（USD）', '仕入金額（JPY・税抜）',
  'ブランドコード', 'モデル名', '型番（Ref.）', 'シリアルNo.',
  '素材コード', '駆動方式コード', 'ベルト素材コード', '特徴・備考',
]);

// ── 伝票番号採番 ───────────────────────────────────────
/**
 * 既存の purchaseSlips から次の伝票番号を採番する
 * 形式: PI-YYYY-NNNN（4桁）
 * 既存番号と絶対重複しない
 */
function peGenerateSlipId() {
  const year = new Date().getFullYear();
  const prefix = `PI-${year}-`;
  const slips = APP_DATA.purchaseSlips || [];
  let maxNum = 0;
  slips.forEach(s => {
    if (s.id && s.id.startsWith(prefix)) {
      const n = parseInt(s.id.slice(prefix.length), 10);
      if (!isNaN(n) && n > maxNum) maxNum = n;
    }
  });
  return `${prefix}${String(maxNum + 1).padStart(4, '0')}`;
}

/**
 * Resolve a stable purchase-staff code to the current account/master name.
 * The slip keeps its code for API/DB linkage; presentation surfaces use the name.
 */
function peGetStaffDisplayName(value, fallback = '—') {
  const normalized = String(value || '').trim();
  if (!normalized) return fallback;

  const normalizedCode = normalized.toUpperCase();
  const masterRecord = (APP_DATA.staffRecords || []).find(record =>
    String(record?.code || '').trim().toUpperCase() === normalizedCode
    || String(record?.name || '').trim() === normalized
  );
  if (masterRecord?.name) return masterRecord.name;

  const linkedUser = (APP_DATA.users || []).find(user =>
    String(user?.staffCode || '').trim().toUpperCase() === normalizedCode
    || String(user?.name || user?.displayName || '').trim() === normalized
  );
  return linkedUser?.name || linkedUser?.displayName || normalized;
}

/** 仕入区分を国内／海外の永続値へ正規化する。既存伝票は国内仕入として扱う。 */
function peNormalizePurchaseTaxMode(value) {
  return value === PE_PURCHASE_TAX_OVERSEAS || String(value) === '2'
    ? PE_PURCHASE_TAX_OVERSEAS
    : PE_PURCHASE_TAX_DOMESTIC;
}

function peGetPurchaseTaxInfo(slip = _peSlipData) {
  const mode = peNormalizePurchaseTaxMode(slip?.purchaseTaxMode);
  return {
    mode,
    rateBasisPoints: mode === PE_PURCHASE_TAX_DOMESTIC ? PE_DOMESTIC_TAX_RATE_BASIS_POINTS : 0,
    modeLabel: mode === PE_PURCHASE_TAX_DOMESTIC ? '国内仕入' : '海外仕入',
    taxLabel: mode === PE_PURCHASE_TAX_DOMESTIC ? '消費税（10%）' : '対象外',
  };
}

/**
 * APIを使わない制作プレビューでも、仕入登録を確定した瞬間のマスタレートを
 * 明細へ保存する。以後の発行・再発行やマスタレート変更では上書きしない。
 */
function peApplyPurchaseRegistrationFXSnapshot(slip) {
  if (!slip || peNormalizePurchaseTaxMode(slip.purchaseTaxMode) !== PE_PURCHASE_TAX_OVERSEAS) return slip;
  const rate = Number(typeof getInventoryUsdRate === 'function' ? getInventoryUsdRate() : 0);
  if (!(rate > 0)) return slip;
  const scale = 100000000;
  const observedAt = new Date().toISOString();
  slip.registrationUsdJpyRate = rate;
  (slip.lines || []).forEach(line => {
    const quantity = Number(line.quantity) > 0 ? Number(line.quantity) : 1;
    line.purchaseCurrency = 'USD';
    line.purchaseFxRateScaled = Math.round(rate * scale);
    line.purchaseFxScale = scale;
    line.purchaseFxRateObservedAt = observedAt;
    line.convertedPurchasePriceJpy = Math.round((Number(line.purchasePrice) || 0) * quantity * rate);
  });
  return slip;
}

/** 税額は各商品明細で端数切捨てし、その合計を伝票税額とする。 */
function peGetPurchaseTotals(slip = _peSlipData) {
  if (typeof getPurchaseSlipTaxSummary === 'function') {
    return getPurchaseSlipTaxSummary(slip);
  }
  const info = peGetPurchaseTaxInfo(slip);
  const lines = slip?.lines || [];
  let subtotal = 0;
  let taxAmount = 0;
  let saleTotal = 0;
  lines.forEach(line => {
    const amount = Number(line.purchasePrice) || 0;
    subtotal += amount;
    saleTotal += Number(line.salePrice) || 0;
    if (info.mode === PE_PURCHASE_TAX_DOMESTIC) {
      taxAmount += Math.floor(amount * info.rateBasisPoints / 10000);
    }
  });
  return { ...info, subtotal, taxAmount, grandTotal: subtotal + taxAmount, saleTotal };
}

function _peUpdatePurchaseTaxModeUI() {
  const info = peGetPurchaseTaxInfo();
  const domesticButton = document.getElementById('pe-tax-domestic');
  const overseasButton = document.getElementById('pe-tax-overseas');
  const description = document.getElementById('pe-tax-mode-description');
  const badge = document.getElementById('pe-tax-mode-badge');
  const domestic = info.mode === PE_PURCHASE_TAX_DOMESTIC;

  domesticButton?.classList.toggle('active', domestic);
  overseasButton?.classList.toggle('active', !domestic);
  domesticButton?.setAttribute('aria-checked', domestic ? 'true' : 'false');
  overseasButton?.setAttribute('aria-checked', domestic ? 'false' : 'true');
  if (description) {
    description.textContent = domestic
      ? '国内仕入：各明細に消費税（10%）を計算します'
      : '海外仕入：各明細の税区分を対象外として扱います';
  }
  if (badge) {
    badge.textContent = info.taxLabel;
    badge.classList.toggle('overseas', !domestic);
  }
  const purchaseHeading = document.getElementById('pe-purchase-price-heading');
  const subtotalCaption = document.getElementById('pe-subtotal-caption');
  const totalCaption = document.getElementById('pe-total-purchase-caption');
  const productPriceLabel = document.getElementById('pep-purchase-price-label');
  if (purchaseHeading) purchaseHeading.textContent = `仕入金額（${domestic ? '円' : 'USD'}）`;
  if (subtotalCaption) subtotalCaption.textContent = `仕入小計（${domestic ? 'JPY' : 'USD'}）:`;
  if (totalCaption) totalCaption.textContent = `仕入合計（${domestic ? 'JPY' : 'USD'}）:`;
  if (productPriceLabel) productPriceLabel.textContent = `仕入金額（${domestic ? '円' : 'USD'}）`;
}

/** 仕入区分スイッチ。入力中の明細税額と合計を即時に再計算する。 */
function peSetPurchaseTaxMode(mode) {
  if (!_peSlipData) return;
  const info = peGetPurchaseTaxInfo({ purchaseTaxMode: mode });
  _peSlipData.purchaseTaxMode = info.mode;
  _peSlipData.taxRateBasisPoints = info.rateBasisPoints;
  _peUpdatePurchaseTaxModeUI();
  _peUpdateDetailUI();
}

/** 商品コードの日付部分（YYYYMMDD）を返す */
function _peItemCodeDatePrefix(dateStr) {
  const matched = /^(\d{4})-(\d{2})-(\d{2})$/.exec(dateStr || '');
  if (matched) return `${matched[1]}${matched[2]}${matched[3]}`;

  const d = new Date();
  return d.getFullYear().toString()
    + String(d.getMonth() + 1).padStart(2, '0')
    + String(d.getDate()).padStart(2, '0');
}

/** 登録済みデータに存在する、指定日の最大連番を返す */
function _peMaxRegisteredItemSequence(ymd) {
  const codes = [];
  (APP_DATA.inventory || []).forEach(item => codes.push(item.code));
  (APP_DATA.purchaseSlips || []).forEach(slip => {
    (slip.lines || []).forEach(line => codes.push(line.code));
  });

  let maxSeq = 0;
  codes.forEach(code => {
    const value = String(code || '');
    if (!value.startsWith(ymd)) return;
    const suffix = value.slice(ymd.length);
    if (!/^\d+$/.test(suffix)) return;
    maxSeq = Math.max(maxSeq, Number(suffix));
  });
  return maxSeq;
}

/**
 * 明細の商品コードを自動生成する。
 * 登録済みコードの次から、現在の明細順に連番となる番号を返す。
 */
function peGenerateItemCode(dateStr) {
  if (!APP_DATA.itemNumberByDate) APP_DATA.itemNumberByDate = {};
  const ymd = _peItemCodeDatePrefix(dateStr);
  const registeredMax = _peMaxRegisteredItemSequence(ymd);
  const draftMax = (_peSlipData?.lines || []).reduce((maxSeq, line) => {
    const value = String(line.code || '');
    const suffix = value.startsWith(ymd) ? value.slice(ymd.length) : '';
    return /^\d+$/.test(suffix) ? Math.max(maxSeq, Number(suffix)) : maxSeq;
  }, 0);
  const seq = Math.max(registeredMax, draftMax) + 1;
  APP_DATA.itemNumberByDate[ymd] = seq + 1;
  return `${ymd}${String(seq).padStart(3, '0')}`;
}

// ── 初期化 ─────────────────────────────────────────────
function init_purchase_entry() {
  // purchaseSlips 配列が未定義なら初期化
  if (!APP_DATA.purchaseSlips) APP_DATA.purchaseSlips = [];

  // 新規伝票を準備
  _peInitNewSlip();

  // セレクトボックスを展開
  _peFillSelects();

  // 一覧を描画
  peRenderList();
}

/** 新規伝票オブジェクトを作成して UI に反映 */
function _peInitNewSlip() {
  _peLineCount = 0;
  _peProductTargetLineId = null;
  _peSlipData = {
    id: peGenerateSlipId(),
    date: '',
    supplier: '',
    staff: '',
    purchaseTaxMode: PE_PURCHASE_TAX_DOMESTIC,
    taxRateBasisPoints: PE_DOMESTIC_TAX_RATE_BASIS_POINTS,
    note: '',
    lines: [],
    status: '未処理',
    revisions: [],
    registeredAt: null,
    issuedAt: null,
    issuedBy: null,
  };

  // ヘッダーフィールドに反映
  const idEl = document.getElementById('pe-slip-id');
  if (idEl) idEl.value = _peSlipData.id;

  const dateEl = document.getElementById('pe-date');
  if (dateEl) {
    dateEl.value = '';
  }

  const supplierEl = document.getElementById('pe-supplier');
  if (supplierEl) supplierEl.value = '';

  const staffEl = document.getElementById('pe-staff');
  if (staffEl) staffEl.value = '';

  const addCountEl = document.getElementById('pe-line-add-count');
  if (addCountEl) addCountEl.value = '1';

  const csvFileEl = document.getElementById('pe-csv-file-input');
  if (csvFileEl) csvFileEl.value = '';

  _peUpdatePurchaseTaxModeUI();

  // 明細テーブルをクリア
  const tbody = document.getElementById('pe-detail-tbody');
  if (tbody) tbody.innerHTML = '';

  _peUpdateDetailUI();
}

/** 登録処理中はボタンを無効化し、二重クリックによる多重登録を防ぐ。 */
function _peSetSavePending(pending) {
  _peSaveInFlight = Boolean(pending);
  const button = document.getElementById('pe-save-button');
  if (!button) return;

  if (pending) {
    if (!button.dataset.idleHtml) button.dataset.idleHtml = button.innerHTML;
    button.disabled = true;
    button.setAttribute('aria-busy', 'true');
    button.innerHTML = '<i class="fa-solid fa-spinner fa-spin"></i> 登録中...';
    return;
  }

  button.disabled = false;
  button.removeAttribute('aria-busy');
  if (button.dataset.idleHtml) button.innerHTML = button.dataset.idleHtml;
}

/** 仕入先・担当者セレクトを APP_DATA から埋める */
function _peFillSelects() {
  // 仕入先
  const suppEl = document.getElementById('pe-supplier');
  if (suppEl) {
    const selected = suppEl.value;
    if (typeof populateSupplierMasterSelect === 'function') {
      populateSupplierMasterSelect('pe-supplier', {
        emptyLabel: '-- 選択 --',
        selected,
        labelMode: 'name',
      });
    } else {
      suppEl.innerHTML = '<option value="">-- 選択 --</option>' +
        (APP_DATA.suppliers || []).map(s => `<option value="${s.code}">${_escHtml(s.name)}</option>`).join('');
      suppEl.value = selected;
    }
  }
  // 担当者
  const staffEl = document.getElementById('pe-staff');
  if (staffEl) {
    const selected = staffEl.dataset.staffRenameValue || staffEl.value;
    delete staffEl.dataset.staffRenameValue;
    if (typeof populateStaffMasterSelect === 'function') {
      populateStaffMasterSelect('pe-staff', { emptyLabel: '-- 選択 --', selected });
    } else {
      staffEl.innerHTML = '<option value="">-- 選択 --</option>';
      (APP_DATA.staff || []).forEach(name => {
        staffEl.innerHTML += `<option value="${name}">${name}</option>`;
      });
    }
  }
}

// ── ヘッダーイベント ────────────────────────────────────
function peOnDateChange() {
  if (!_peSlipData) return;
  const val = document.getElementById('pe-date')?.value || '';
  _peSlipData.date = val;
  // 既存明細の商品コードはそのまま維持（日付変更時には再採番しない）
}

// ── 明細操作 ────────────────────────────────────────────
const PE_MAX_BULK_LINES = 100;

/** 追加行数は半角数字だけを受け付ける */
function peNormalizeAddCountInput(input) {
  if (!input) return;
  input.value = String(input.value || '').replace(/[^0-9]/g, '').slice(0, 3);
}

/** 追加行数欄で Enter を押した場合も明細追加を実行する */
function peHandleAddCountKey(event) {
  if (event.key !== 'Enter') return;
  event.preventDefault();
  peAddLine();
}

/** 入力された追加行数を検証して返す */
function _peGetAddLineCount() {
  const input = document.getElementById('pe-line-add-count');
  const raw = input?.value?.trim() || '';
  const count = Number(raw);
  if (!/^\d+$/.test(raw) || !Number.isInteger(count) || count < 1 || count > PE_MAX_BULK_LINES) {
    showToast('error', '入力エラー', `追加行数は1〜${PE_MAX_BULK_LINES}の半角数字で入力してください`);
    input?.focus();
    input?.select();
    return null;
  }
  return count;
}

/** 現在の明細順に、明細番号と商品コードを連番で振り直す */
function _peRenumberDraftLines(dateStr) {
  if (!_peSlipData) return;
  if (!APP_DATA.itemNumberByDate) APP_DATA.itemNumberByDate = {};

  const ymd = _peItemCodeDatePrefix(dateStr);
  const firstSeq = _peMaxRegisteredItemSequence(ymd) + 1;
  _peSlipData.lines.forEach((line, index) => {
    line.lineNo = index + 1;
    line.code = `${ymd}${String(firstSeq + index).padStart(3, '0')}`;

    const row = document.getElementById(`pe-row-${line.lineId}`);
    const lineNoEl = row?.querySelector('[data-role="pe-line-no"]');
    const codeEl = row?.querySelector('[data-role="pe-item-code"]');
    if (lineNoEl) lineNoEl.textContent = line.lineNo;
    if (codeEl) codeEl.textContent = line.code;
  });
  APP_DATA.itemNumberByDate[ymd] = firstSeq + _peSlipData.lines.length;
}

/** 入力された行数分の明細を一括追加 */
function peAddLine() {
  if (!_peSlipData) return;

  const addCount = _peGetAddLineCount();
  if (addCount === null) return;

  const dateVal = document.getElementById('pe-date')?.value || '';
  const addedLines = [];
  for (let index = 0; index < addCount; index++) {
    const lineObj = {
      lineId: ++_peLineCount,
      lineNo: _peSlipData.lines.length + 1,
      code: peGenerateItemCode(dateVal),
      sku: '',
      purchasePrice: 0,
      salePrice: 0,
      productDetail: null,
    };
    _peSlipData.lines.push(lineObj);
    addedLines.push(lineObj);
  }

  addedLines.forEach(line => _peRenderLine(line));
  _peRenumberDraftLines(dateVal);
  _peUpdateDetailUI();
  if (addCount > 1) {
    showToast('success', '明細を追加しました', `${addCount}行を一括追加しました`);
  }
}

/** 1行分のテーブル行を描画（tbody への追加） */
function _peRenderLine(lineObj) {
  const tbody = document.getElementById('pe-detail-tbody');
  if (!tbody) return;
  const hasProductDetail = Boolean(lineObj.productDetail);

  const tr = document.createElement('tr');
  tr.id = `pe-row-${lineObj.lineId}`;
  tr.innerHTML = `
    <td data-role="pe-line-no" style="text-align:center;font-weight:600;color:var(--text-muted);vertical-align:middle;">${lineObj.lineNo}</td>
    <td style="vertical-align:middle;">
      <span class="auto-tag" style="font-size:10px;display:block;margin-bottom:2px;">自動</span>
      <code data-role="pe-item-code" style="font-size:11px;color:var(--primary);">${lineObj.code}</code>
    </td>
    <td>
      <input type="text" class="form-control form-control-sm pe-sku-input"
        id="pe-sku-${lineObj.lineId}"
        placeholder="SKU必須"
        value="${_escHtml(lineObj.sku)}"
        data-line="${lineObj.lineId}"
        oninput="peOnSkuInput(this)"
        onblur="inputFormatHandler(this,'half')"
        onkeydown="peHandleTabEnter(event, this)">
    </td>
    <td>
      <input type="text" inputmode="numeric" class="form-control form-control-sm pe-price-input"
        id="pe-pp-${lineObj.lineId}"
        placeholder="例: 850,000"
        value="${lineObj.purchasePrice ? lineObj.purchasePrice.toLocaleString('ja-JP') : ''}"
        data-line="${lineObj.lineId}"
        data-raw-value="${lineObj.purchasePrice || ''}"
        oninput="pePriceInput(this,'purchase')"
        onblur="pePriceInput(this,'purchase')"
        onkeydown="peHandleTabEnter(event, this)">
    </td>
    <td style="text-align:right;vertical-align:middle;">
      <div class="pe-line-tax" data-role="pe-line-tax">
        <span class="pe-line-tax-label" data-role="pe-line-tax-label"></span>
        <strong class="pe-line-tax-amount" data-role="pe-line-tax-amount"></strong>
      </div>
    </td>
    <td>
      <input type="text" inputmode="numeric" class="form-control form-control-sm pe-price-input"
        id="pe-sp-${lineObj.lineId}"
        placeholder="例: 1,180,000"
        value="${lineObj.salePrice ? lineObj.salePrice.toLocaleString('ja-JP') : ''}"
        data-line="${lineObj.lineId}"
        data-raw-value="${lineObj.salePrice || ''}"
        oninput="pePriceInput(this,'sale')"
        onblur="pePriceInput(this,'sale')"
        onkeydown="peHandleTabEnter(event, this)">
    </td>
    <td style="text-align:center;vertical-align:middle;">
      <button class="btn btn-outline btn-sm pe-regbtn"
        style="font-size:10px;padding:3px 6px;white-space:nowrap;"
        onclick="peOpenProductModal(${lineObj.lineId})"
        title="${hasProductDetail ? 'CSVから読み込んだ商品情報を確認・編集' : '商品登録ポップアップを開く'}">
        <i class="fa-solid fa-box-open"></i> ${hasProductDetail ? '確認・編集' : '登録'}
      </button>
    </td>
    <td style="text-align:center;vertical-align:middle;">
      <button class="btn btn-sm" style="background:none;border:none;color:#e74c3c;cursor:pointer;font-size:16px;"
        onclick="peRemoveLine(${lineObj.lineId})" title="この行を削除">
        <i class="fa-solid fa-trash-can"></i>
      </button>
    </td>
  `;
  tbody.appendChild(tr);
}

/** 明細行を削除 */
function peRemoveLine(lineId) {
  if (!_peSlipData) return;
  _peSlipData.lines = _peSlipData.lines.filter(l => l.lineId !== lineId);
  const tr = document.getElementById(`pe-row-${lineId}`);
  if (tr) tr.remove();
  const dateVal = document.getElementById('pe-date')?.value || '';
  _peRenumberDraftLines(dateVal);
  _peUpdateDetailUI();
}

/** 明細エリアの表示制御と集計更新 */
function _peUpdateDetailUI() {
  const lines = _peSlipData?.lines || [];
  const isEmpty = lines.length === 0;

  const table = document.getElementById('pe-detail-table');
  const empty = document.getElementById('pe-detail-empty');
  if (table) table.style.display = isEmpty ? 'none' : '';
  if (empty) empty.style.display = isEmpty ? '' : 'none';

  const totals = peGetPurchaseTotals();
  lines.forEach(line => {
    const row = document.getElementById(`pe-row-${line.lineId}`);
    const taxCell = row?.querySelector('[data-role="pe-line-tax"]');
    const label = row?.querySelector('[data-role="pe-line-tax-label"]');
    const amount = row?.querySelector('[data-role="pe-line-tax-amount"]');
    const isDomestic = totals.mode === PE_PURCHASE_TAX_DOMESTIC;
    taxCell?.classList.toggle('out-of-scope', !isDomestic);
    if (label) label.textContent = totals.taxLabel;
    if (amount) {
      amount.textContent = isDomestic
        ? `¥${Math.floor((Number(line.purchasePrice) || 0) * totals.rateBasisPoints / 10000).toLocaleString('ja-JP')}`
        : '';
    }
  });

  const cntEl = document.getElementById('pe-line-count');
  const subtotalEl = document.getElementById('pe-subtotal-purchase');
  const taxWrapEl = document.getElementById('pe-total-tax-wrap');
  const taxEl = document.getElementById('pe-total-tax');
  const tpEl  = document.getElementById('pe-total-purchase');
  const tsEl  = document.getElementById('pe-total-sale');
  if (cntEl) cntEl.textContent = lines.length;
  if (subtotalEl) subtotalEl.textContent = `¥${totals.subtotal.toLocaleString('ja-JP')}`;
  if (taxWrapEl) taxWrapEl.firstChild.textContent = `${totals.taxLabel}: `;
  if (taxEl) taxEl.textContent = totals.mode === PE_PURCHASE_TAX_DOMESTIC
    ? `¥${totals.taxAmount.toLocaleString('ja-JP')}` : '対象外';
  const purchaseAmount = typeof formatPurchaseSlipAmount === 'function'
    ? amount => formatPurchaseSlipAmount(amount, _peSlipData)
    : amount => `${totals.mode === PE_PURCHASE_TAX_OVERSEAS ? '$' : '¥'}${Number(amount || 0).toLocaleString(totals.mode === PE_PURCHASE_TAX_OVERSEAS ? 'en-US' : 'ja-JP')}`;
  if (subtotalEl) subtotalEl.textContent = purchaseAmount(totals.subtotal);
  if (tpEl) tpEl.textContent = purchaseAmount(totals.grandTotal);
  if (tsEl) tsEl.textContent = formatSalePrice(totals.saleTotal);
}

// ── 入力ハンドラ ────────────────────────────────────────
/** SKU 入力 */
function peOnSkuInput(input) {
  inputFormatHandler(input, 'half');
  const lineId = parseInt(input.dataset.line, 10);
  const line = _peSlipData?.lines.find(l => l.lineId === lineId);
  if (line) line.sku = input.value;
}

/**
 * 金額フィールド共通ハンドラ（仕入金額・売価）
 * priceFormatHandler と同一ロジックで共通化
 */
function pePriceInput(input, kind) {
  // priceFormatHandler と完全同一処理
  priceFormatHandler(input);

  const lineId = parseInt(input.dataset.line, 10);
  const line = _peSlipData?.lines.find(l => l.lineId === lineId);
  if (!line) return;

  const numVal = getPriceValue(input);
  if (kind === 'purchase') line.purchasePrice = numVal;
  else if (kind === 'sale') line.salePrice = numVal;

  _peUpdateDetailUI();
}

/**
 * Tab / Enter キーで次の入力欄へフォーカス移動
 * 順序: SKU → 仕入金額 → 売価 → 次行のSKU
 */
function peHandleTabEnter(event, input) {
  if (event.key !== 'Enter' && event.key !== 'Tab') return;
  // Tab はブラウザのデフォルト動作と競合するため Enter のみ制御する
  if (event.key !== 'Enter') return;

  event.preventDefault();

  const lineId = parseInt(input.dataset.line, 10);
  const id = input.id;

  if (id.startsWith('pe-sku-')) {
    // SKU → 仕入金額
    document.getElementById(`pe-pp-${lineId}`)?.focus();
  } else if (id.startsWith('pe-pp-')) {
    // 仕入金額 → 売価
    document.getElementById(`pe-sp-${lineId}`)?.focus();
  } else if (id.startsWith('pe-sp-')) {
    // 売価 → 次行のSKU（なければ明細追加して次行に移動）
    const lines = _peSlipData?.lines || [];
    const idx = lines.findIndex(l => l.lineId === lineId);
    if (idx >= 0 && idx < lines.length - 1) {
      const nextLine = lines[idx + 1];
      document.getElementById(`pe-sku-${nextLine.lineId}`)?.focus();
    }
    // 最終行なら次行追加しない（自然な終了）
  }
}

// ── 伝票保存 ────────────────────────────────────────────
async function peSave() {
  if (!_peSlipData || _peSaveInFlight) return;

  // ヘッダーの現在値を同期
  _peSlipData.date     = document.getElementById('pe-date')?.value || '';
  _peSlipData.supplier = document.getElementById('pe-supplier')?.value || '';
  _peSlipData.staff    = document.getElementById('pe-staff')?.value || '';
  _peSlipData.purchaseTaxMode = peGetPurchaseTaxInfo().mode;
  _peSlipData.taxRateBasisPoints = peGetPurchaseTaxInfo().rateBasisPoints;

  // バリデーション
  if (!_peSlipData.date) {
    showToast('error', '入力エラー', '仕入日を入力してください');
    document.getElementById('pe-date')?.focus();
    return;
  }
  if (!_peSlipData.supplier) {
    showToast('error', '入力エラー', '仕入先を選択してください');
    document.getElementById('pe-supplier')?.focus();
    return;
  }
  if (_peSlipData.lines.length === 0) {
    showToast('error', '入力エラー', '明細を1件以上追加してください');
    return;
  }
  const noSku = _peSlipData.lines.filter(l => !l.sku.trim());
  if (noSku.length > 0) {
    showToast('error', '入力エラー', `明細No.${noSku.map(l => l.lineNo).join(', ')} のSKUを入力してください`);
    return;
  }

  if (window.ZaikoAPI) {
    _peSetSavePending(true);
    try {
      const result = await window.ZaikoAPI.savePurchaseSlip(_peSlipData, typeof isWorker === 'function' && isWorker());
      const slipNo = result.record?.slipNumber || result.record?.id || _peSlipData.id;
      showToast('success', result.approval ? '承認申請を送信しました' : '仕入登録完了',
        result.approval ? `伝票 ${slipNo} を管理者の承認待ちにしました` : `伝票 ${slipNo} をDBへ保存し在庫へ反映しました`);
      _peInitNewSlip();
      _peFillSelects();
      peRenderList();
    } catch (error) {
      showToast('error', '仕入登録できませんでした', error.message || '入力内容を確認してください');
    } finally {
      _peSetSavePending(false);
    }
    return;
  }

  _peSetSavePending(true);
  try {
    // 保存
    _peSlipData.registeredAt = new Date().toISOString().slice(0, 16).replace('T', ' ');
    peApplyPurchaseRegistrationFXSnapshot(_peSlipData);

    // 全明細を在庫に一括登録（重複コードはスキップ）
    _peRegisterAllToInventory(_peSlipData);

    APP_DATA.purchaseSlips.push(JSON.parse(JSON.stringify(_peSlipData)));

    const inventoryCount = (_peSlipData.lines || []).length;
    showToast('success', '仕入登録完了', `伝票 ${_peSlipData.id} を登録し、${inventoryCount}件を在庫に追加しました`);

    // ④ 伝票追加後にタスク数を再計算
    if (typeof refreshLinkedBusinessViews === 'function') refreshLinkedBusinessViews({ source: 'purchase-entry' });

    // 正常完了後だけ入力内容を消去し、次の伝票番号で新規状態へ戻す
    _peInitNewSlip();
    _peFillSelects();
    peRenderList();
  } finally {
    _peSetSavePending(false);
  }
}

/** フォームリセット */
function peReset() {
  if (!confirm('入力内容をリセットしますか？')) return;
  _peInitNewSlip();
  _peFillSelects();
}

// ── 一覧描画 ────────────────────────────────────────────
function peRenderList() {
  const slips = APP_DATA.purchaseSlips || [];
  const tbody = document.getElementById('pe-list-tbody');
  const emptyEl = document.getElementById('pe-list-empty');
  const countEl = document.getElementById('pe-list-count');
  if (!tbody) return;

  const reflectedLineCount = (slip, line) => slip.apiManaged
    ? Math.max(0, Number(line.generatedProductCount) || 0)
    : Math.max(0, Number(line.quantity) || 1);
  const reflectedProductCount = slips.reduce((total, slip) => total + (slip.lines || []).reduce(
    (lineTotal, line) => lineTotal + reflectedLineCount(slip, line), 0), 0);
  const pendingProductCount = slips.reduce((total, slip) => total + (slip.lines || []).reduce(
    (lineTotal, line) => lineTotal + Math.max(0, (Number(line.quantity) || 1) - reflectedLineCount(slip, line)), 0), 0);
  if (countEl) {
    countEl.textContent = `全 ${slips.length}伝票 / 在庫反映 ${reflectedProductCount}点${pendingProductCount ? ` / 未反映 ${pendingProductCount}点` : ''}`;
    countEl.title = pendingProductCount
      ? '未反映点数は下書き伝票です。在庫点数との比較には「在庫反映」を使用します。'
      : '在庫反映点数は在庫一覧の全ステータス件数と一致します。';
  }
  if (emptyEl) emptyEl.style.display = slips.length === 0 ? '' : 'none';

  // 降順（新しい順）
  const sorted = [...slips].sort((a, b) => (b.id > a.id ? 1 : -1));

  tbody.innerHTML = sorted.map(slip => {
    const totals = peGetPurchaseTotals(slip);
    const suppName = slip.supplier ? getSupplierName(slip.supplier) : '—';
    const staffName = peGetStaffDisplayName(slip.staff);
    const canIssue = typeof canIssuePurchaseSlip === 'function' ? canIssuePurchaseSlip() : true;
    const issueLabel = slip.issuedAt ? '再発行' : '発行';
    const formatPurchaseAmount = amount => typeof formatPurchaseSlipAmount === 'function'
      ? formatPurchaseSlipAmount(amount, slip)
      : `¥${Number(amount || 0).toLocaleString('ja-JP')}`;
    const grandTotalLabel = totals.mode === PE_PURCHASE_TAX_OVERSEAS
      ? '仕入合計（対象外）'
      : '仕入合計（税込）';
    return `<tr>
      <td><strong>${_escHtml(slip.id)}</strong></td>
      <td>${_escHtml(slip.date)}</td>
      <td>${_escHtml(typeof formatPurchaseIssuedAt === 'function' ? formatPurchaseIssuedAt(slip.issuedAt) : (slip.issuedAt || '未発行'))}</td>
      <td>${_escHtml(suppName)}</td>
      <td>${_escHtml(staffName)}</td>
      <td><span class="status-badge">${_escHtml(totals.modeLabel)}</span><br><small>${_escHtml(totals.taxLabel)}</small></td>
      <td style="text-align:right;">${(slip.lines || []).reduce((sum, line) => sum + reflectedLineCount(slip, line), 0)} 点${slip.rawStatus === 'draft' ? '<br><small>下書き・未反映</small>' : ''}</td>
      <td class="pe-purchase-total-cell">
        <div class="pe-purchase-total-stack">
          <div class="pe-purchase-total-line">
            <span class="pe-purchase-total-label">仕入小計（税抜）</span>
            <strong class="pe-purchase-total-value">${formatPurchaseAmount(totals.subtotal)}</strong>
          </div>
          <div class="pe-purchase-total-line pe-purchase-total-line-grand">
            <span class="pe-purchase-total-label">${grandTotalLabel}</span>
            <strong class="pe-purchase-total-value">${formatPurchaseAmount(totals.grandTotal)}</strong>
          </div>
        </div>
      </td>
      <td style="text-align:right;font-weight:bold;color:var(--success);">${formatSalePrice(totals.saleTotal)}</td>
      <td style="text-align:center;">
        <button class="btn btn-outline btn-sm" onclick="peViewSlip('${_escHtml(slip.id)}')">
          <i class="fa-solid fa-eye"></i>
        </button>
        <button class="btn btn-sm" style="background:none;border:none;color:#e74c3c;cursor:pointer;font-size:14px;"
          onclick="peDeleteSlip('${_escHtml(slip.id)}')" title="削除">
          <i class="fa-solid fa-trash-can"></i>
        </button>
      </td>
      <td style="text-align:center;">
        <button type="button" class="btn btn-primary btn-sm purchase-issue-button" ${canIssue ? '' : 'disabled'}
          onclick="issuePurchaseSlipDocument('${_escHtml(slip.id)}', event)" title="${canIssue ? '発行日時を記録して仕入伝票をダウンロード' : '管理者のみ発行できます'}">
          <i class="fa-solid fa-file-arrow-down"></i> ${issueLabel}
        </button>
      </td>
    </tr>`;
  }).join('');
}

/** 伝票詳細モーダルを開く */
function peViewSlip(slipId) {
  const slip = (APP_DATA.purchaseSlips || []).find(s => s.id === slipId);
  if (!slip) return;

  const suppName = slip.supplier ? getSupplierName(slip.supplier) : '—';
  const staffName = peGetStaffDisplayName(slip.staff);
  const totals = peGetPurchaseTotals(slip);

  const titleEl = document.getElementById('peViewModalTitle');
  const bodyEl  = document.getElementById('peViewModalBody');
  if (titleEl) titleEl.innerHTML = `<i class="fa-solid fa-file-invoice"></i> ${_escHtml(slip.id)}`;
  if (bodyEl) {
    bodyEl.innerHTML = `
      <div class="form-row cols-2" style="margin-bottom:12px;">
        <div><label class="form-label">仕入日</label><p>${_escHtml(slip.date)}</p></div>
        <div><label class="form-label">発行日時</label><p>${_escHtml(typeof formatPurchaseIssuedAt === 'function' ? formatPurchaseIssuedAt(slip.issuedAt) : (slip.issuedAt || '未発行'))}</p></div>
        <div><label class="form-label">仕入先</label><p>${_escHtml(suppName)}</p></div>
        <div><label class="form-label">担当者</label><p>${_escHtml(staffName)}</p></div>
        <div><label class="form-label">仕入区分</label><p>${_escHtml(totals.modeLabel)}（${_escHtml(totals.taxLabel)}）</p></div>
        <div><label class="form-label">登録日時</label><p>${_escHtml(slip.registeredAt || '—')}</p></div>
      </div>
      <table class="data-table" style="margin-bottom:0;">
        <thead>
          <tr>
            <th>No.</th><th>商品コード</th><th>SKU</th>
            <th style="text-align:right;">仕入金額（${totals.mode === PE_PURCHASE_TAX_OVERSEAS ? 'USD' : 'JPY'}・税抜）</th><th style="text-align:right;">税区分 / 税額</th><th style="text-align:right;">売価（USD）</th>
          </tr>
        </thead>
        <tbody>
          ${(slip.lines || []).map(l => `<tr>
            <td>${l.lineNo}</td>
            <td><code style="font-size:11px;">${_escHtml(l.code)}</code></td>
            <td>${_escHtml(l.sku)}</td>
            <td style="text-align:right;">${typeof formatPurchaseSlipAmount === 'function' ? formatPurchaseSlipAmount(l.purchasePrice || 0, slip) : `¥${(l.purchasePrice || 0).toLocaleString('ja-JP')}`}</td>
            <td style="text-align:right;">${totals.mode === PE_PURCHASE_TAX_DOMESTIC ? `${_escHtml(totals.taxLabel)}<br><strong>¥${Math.floor((Number(l.purchasePrice) || 0) * totals.rateBasisPoints / 10000).toLocaleString('ja-JP')}</strong>` : '対象外'}</td>
            <td style="text-align:right;">${formatSalePrice(l.salePrice || 0)}</td>
          </tr>`).join('')}
        </tbody>
        <tfoot>
          <tr style="background:var(--bg);">
            <td colspan="3" style="text-align:right;font-weight:bold;">小計 / ${_escHtml(totals.taxLabel)} / 合計</td>
            <td style="text-align:right;font-weight:bold;color:var(--primary);">${formatPurchaseSlipAmount(totals.subtotal, slip)}<br>${totals.mode === PE_PURCHASE_TAX_DOMESTIC ? formatPurchaseSlipAmount(totals.taxAmount, slip) : '対象外'}<br>${formatPurchaseSlipAmount(totals.grandTotal, slip)}</td>
            <td></td>
            <td style="text-align:right;font-weight:bold;color:var(--success);">${formatSalePrice(totals.saleTotal)}</td>
          </tr>
        </tfoot>
      </table>`;
  }
  document.getElementById('peViewModal').style.display = 'flex';
}

/** 伝票削除 */
function peDeleteSlip(slipId) {
  if (!confirm(`伝票 ${slipId} を削除しますか？`)) return;
  APP_DATA.purchaseSlips = (APP_DATA.purchaseSlips || []).filter(s => s.id !== slipId);
  peRenderList();
  showToast('info', '削除完了', `伝票 ${slipId} を削除しました`);
}

// ── 商品登録ポップアップ ────────────────────────────────
/** ポップアップを開く */
function peOpenProductModal(lineId) {
  _peProductTargetLineId = lineId;
  const line = _peSlipData?.lines.find(l => l.lineId === lineId);
  if (!line) return;

  const modal = document.getElementById('peProductModal');
  if (!modal) return;

  // 基本情報を反映（読み取り専用フィールド）
  const codeEl    = document.getElementById('pep-code');
  const dateEl    = document.getElementById('pep-date');
  const skuEl     = document.getElementById('pep-sku');
  const suppEl    = document.getElementById('pep-supplier-label');
  const ppEl      = document.getElementById('pep-purchase-price');
  const spEl      = document.getElementById('pep-sale-price');

  if (codeEl)  codeEl.value  = line.code;
  if (dateEl)  dateEl.value  = document.getElementById('pe-date')?.value || '';
  if (skuEl)   skuEl.value   = line.sku;
  if (suppEl) {
    const suppCode = document.getElementById('pe-supplier')?.value || '';
    suppEl.value = suppCode ? getSupplierName(suppCode) : '—';
  }
  if (ppEl) {
    ppEl.value = line.purchasePrice ? line.purchasePrice.toLocaleString('ja-JP') : '';
    ppEl.dataset.rawValue = line.purchasePrice || '';
  }
  if (spEl) {
    spEl.value = line.salePrice ? line.salePrice.toLocaleString('ja-JP') : '';
    spEl.dataset.rawValue = line.salePrice || '';
  }

  // 既存入力がある場合は復元
  const detail = line.productDetail || {};
  _pepSetField('pep-brand',    detail.brand    || '');
  _pepSetField('pep-model',    detail.model    || '');
  _pepSetField('pep-ref',      detail.ref      || '');
  _pepSetField('pep-serial',   detail.serial   || '');
  _pepSetField('pep-material', detail.material || '');
  _pepSetField('pep-movement', detail.movement || '');
  _pepSetField('pep-condition',detail.condition|| '');
  _pepSetField('pep-shape', detail.shape || '');
  _pepSetField('pep-marking', detail.marking || '');
  _pepSetField('pep-belt',     detail.belt     || '');
  _pepSetField('pep-dial',     detail.dial     || '');
  _pepSetField('pep-note',     detail.note     || '');

  // セレクトボックスを補充（BOX含む）
  _pepFillSelects(detail.brand || '');

  // BOX 番号の復元
  const boxEl = document.getElementById('pep-box');
  if (boxEl) boxEl.value = detail.boxNo || '';

  // 付属品チェックボックス（BRACELET PARTS 数量も復元）
  _pepRenderAccessories(detail.accessories || [], detail.braceletQty || 1);

  modal.style.display = 'flex';
}

function _pepSetField(id, value) {
  const el = document.getElementById(id);
  if (el) el.value = value;
}

function _pepFillSelects(selectedBrand = '') {
  // ブランド
  const brandEl = document.getElementById('pep-brand');
  if (brandEl) {
    if (typeof populateBrandMasterSelect === 'function') {
      populateBrandMasterSelect('pep-brand', {
        emptyLabel: '-- 選択 --',
        selected: selectedBrand || brandEl.value,
        extraValues: selectedBrand ? [selectedBrand] : [],
      });
    } else {
      brandEl.innerHTML = '<option value="">-- 選択 --</option>' +
        (APP_DATA.brands || []).map(b => `<option value="${_escHtml(b)}">${_escHtml(b)}</option>`).join('');
      brandEl.value = selectedBrand;
    }
  }
  // 素材
  const matEl = document.getElementById('pep-material');
  if (matEl) {
    const selected = matEl.value;
    if (typeof populateProductSpecMasterSelect === 'function') {
      populateProductSpecMasterSelect('pep-material', 'material', {
        emptyLabel: '-- 選択 --', selected, extraCodes: selected ? [selected] : [], labelMode: 'name',
      });
    } else if (matEl.options.length <= 1) {
      (APP_DATA.materials || []).forEach(m => {
        matEl.innerHTML += `<option value="${m.code}">${_escHtml(m.name)}</option>`;
      });
    }
  }
  // 駆動方式
  const movEl = document.getElementById('pep-movement');
  if (movEl) {
    const selected = movEl.value;
    if (typeof populateProductSpecMasterSelect === 'function') {
      populateProductSpecMasterSelect('pep-movement', 'movement', {
        emptyLabel: '-- 選択 --', selected, extraCodes: selected ? [selected] : [], labelMode: 'name',
      });
    } else if (movEl.options.length <= 1) {
      (APP_DATA.movements || []).forEach(m => {
        movEl.innerHTML += `<option value="${m.code}">${_escHtml(m.name)}</option>`;
      });
    }
  }
  // コンディション
  const condEl = document.getElementById('pep-condition');
  if (condEl && typeof populateConditionMasterSelect === 'function') {
    const selected = condEl.value;
    populateConditionMasterSelect('pep-condition', {
      emptyLabel: '-- 選択 --', selected, extraCodes: selected ? [selected] : [], labelMode: 'name',
    });
  } else if (condEl && condEl.options.length <= 1) {
    (APP_DATA.conditions || []).forEach(c => {
      condEl.innerHTML += `<option value="${c.code}">${_escHtml(c.name)}</option>`;
    });
  }
  populateProductSpecMasterSelect('pep-shape', 'shape', { selected: detail.shape || '', labelMode: 'name' });
  populateProductSpecMasterSelect('pep-marking', 'marking', { selected: detail.marking || '', labelMode: 'name' });
  // BOX（毎回再描画して最新の BOX 一覧を反映）
  const boxEl = document.getElementById('pep-box');
  if (boxEl) {
    boxEl.innerHTML = '<option value="">-- 選択なし --</option>';
    (APP_DATA.boxes || []).forEach(box => {
      const label = box.name ? `${box.no} - ${_escHtml(box.name)}` : String(box.no);
      boxEl.innerHTML += `<option value="${box.no}">${label}</option>`;
    });
  }
}

function _pepRenderAccessories(checked, braceletQty) {
  const wrap = document.getElementById('pep-accessories');
  if (!wrap) return;
  checked = Array.isArray(checked) ? checked : [];
  const accessoryNames = typeof getAccessoryMasterNames === 'function'
    ? getAccessoryMasterNames(checked)
    : [...new Set([...(APP_DATA.accessories || []), ...(checked || [])])];
  wrap.innerHTML = accessoryNames.map(acc => `
    <label class="checkbox-label">
      <input type="checkbox" name="pep-acc" value="${_escHtml(acc)}"
        onchange="_pepOnAccessoryChange(this)"
        ${checked.includes(acc) ? 'checked' : ''}> ${_escHtml(acc)}
    </label>`).join('');

  // BRACELET PARTSの数量フィールドの初期表示
  _pepToggleBraceletQty(checked.includes('BRACELET PARTS'), braceletQty);
}

/** BRACELET PARTS チェック変更時 */
function _pepOnAccessoryChange(cb) {
  if (cb.value === 'BRACELET PARTS') {
    _pepToggleBraceletQty(cb.checked, 1);
  }
}

/** BRACELET PARTS 数量行の表示/非表示 */
function _pepToggleBraceletQty(show, qty) {
  const row = document.getElementById('pep-bracelet-qty-row');
  const input = document.getElementById('pep-bracelet-qty');
  if (!row) return;
  if (show) {
    row.style.display = 'flex';
    if (input && qty != null) input.value = qty || 1;
  } else {
    row.style.display = 'none';
    if (input) input.value = '1';
  }
}

/** BRACELET PARTS 数量変更時にラインデータへ同期 */
function _pepBraceletQtyChange(input) {
  // 値が空・負・ゼロの場合は 1 にフォールバック
  if (!input.value || parseInt(input.value, 10) < 1) input.value = 1;
}

/** ポップアップを閉じる */
function pecloseProductModal() {
  const modal = document.getElementById('peProductModal');
  if (modal) modal.style.display = 'none';
  _peProductTargetLineId = null;
}

/**
 * ポップアップ「確定」ボタン
 * – 入力内容を line.productDetail に保存するだけ（在庫登録はしない）
 * – 在庫登録は「仕入登録する」ボタン（peSave）内で一括実施する
 */
function peSaveProduct() {
  const lineId = _peProductTargetLineId;
  if (lineId == null || !_peSlipData) return;

  const line = _peSlipData.lines.find(l => l.lineId === lineId);
  if (!line) return;

  // 必須チェック
  const skuVal = document.getElementById('pep-sku')?.value?.trim() || '';
  if (!skuVal) {
    showToast('error', '入力エラー', 'SKUを入力してください');
    return;
  }

  // 付属品チェック済みリスト
  const accChecked = Array.from(
    document.querySelectorAll('#pep-accessories input[name="pep-acc"]:checked')
  ).map(cb => cb.value);

  // BRACELET PARTS 数量
  const hasBracelet = accChecked.includes('BRACELET PARTS');
  const braceletQty = hasBracelet
    ? (parseInt(document.getElementById('pep-bracelet-qty')?.value, 10) || 1)
    : null;

  // BOX 番号
  const boxNo = document.getElementById('pep-box')?.value || null;

  const detail = {
    brand:       document.getElementById('pep-brand')?.value    || '',
    model:       document.getElementById('pep-model')?.value    || '',
    ref:         document.getElementById('pep-ref')?.value      || '',
    serial:      document.getElementById('pep-serial')?.value   || '',
    material:    document.getElementById('pep-material')?.value || '',
    movement:    document.getElementById('pep-movement')?.value || '',
    condition:   document.getElementById('pep-condition')?.value|| '',
    shape:       document.getElementById('pep-shape')?.value || '',
    marking:     document.getElementById('pep-marking')?.value || '',
    belt:        document.getElementById('pep-belt')?.value     || '',
    dial:        document.getElementById('pep-dial')?.value     || '',
    note:        document.getElementById('pep-note')?.value     || '',
    accessories: accChecked,
    braceletQty: braceletQty,  // null = BRACELET PARTS 未選択
    boxNo:       boxNo,
  };
  line.productDetail = detail;

  // SKU を明細行に同期
  line.sku = skuVal;
  const skuInput = document.getElementById(`pe-sku-${lineId}`);
  if (skuInput) skuInput.value = skuVal;

  showToast('success', '確定', `明細 ${line.lineNo} の商品情報を保存しました（在庫登録は「仕入登録する」で行います）`);
  pecloseProductModal();
}

/**
 * 「仕入登録する」（peSave）内で呼ばれる一括在庫登録
 * 伝票の全明細を APP_DATA.inventory に追加する
 */
function _peRegisterAllToInventory(slip) {
  const suppCode = slip.supplier || '';
  const staffVal = peGetStaffDisplayName(slip.staff, '');
  const dateVal  = slip.date     || '';
  const overseas = peNormalizePurchaseTaxMode(slip.purchaseTaxMode) === PE_PURCHASE_TAX_OVERSEAS;

  // 登録済みコードセット（重複防止）
  const existingCodes = new Set((APP_DATA.inventory || []).map(i => i.code));

  (slip.lines || []).forEach(line => {
    if (existingCodes.has(line.code)) return; // 重複スキップ
    existingCodes.add(line.code);

    const detail = line.productDetail || {};
    const newItem = {
      code:          line.code,
      brand:         detail.brand    || '',
      brandEn:       '',
      model:         detail.model    || '',
      modelEn:       '',
      ref:           detail.ref      || '',
      serial:        detail.serial   || '',
      supplier:      suppCode,
      staff:         staffVal,
      purchasePrice: overseas ? (Number(line.convertedPurchasePriceJpy) || 0) : (Number(line.purchasePrice) || 0),
      purchaseCurrency: overseas ? 'USD' : 'JPY',
      purchaseOriginalPrice: Number(line.purchasePrice) || 0,
      fixedPurchaseCostJpyMinor: overseas ? (Number(line.convertedPurchasePriceJpy) || 0) : (Number(line.purchasePrice) || 0),
      purchaseFxRateScaled: Number(line.purchaseFxRateScaled) || 0,
      purchaseFxScale: Number(line.purchaseFxScale) || 0,
      purchaseFxRateObservedAt: line.purchaseFxRateObservedAt || null,
      salePrice:     line.salePrice     || 0,
      purchaseDate:  dateVal,
      status:        '在庫中',
      material:      detail.material   || '',
      movement:      detail.movement   || '',
      condition:     detail.condition  || '',
      accessories:   detail.accessories || [],
      braceletQty:   detail.braceletQty || null,
      boxNo:         detail.boxNo      || null,
      images:        [],
      note:          detail.note       || '',
      sku:           line.sku          || '',
      revisions:     [],
      purchaseSlipId: slip.id,
    };
    APP_DATA.inventory.push(newItem);
  });
}

// ── CSV出力 ────────────────────────────────────────────
function _peDownloadCSVRows(filename, rows) {
  const csv = rows
    .map(row => row.map(value => `"${String(value ?? '').replace(/"/g, '""')}"`).join(','))
    .join('\r\n');
  const blob = new Blob(['\uFEFF' + csv], { type: 'text/csv;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = filename;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  setTimeout(() => URL.revokeObjectURL(url), 0);
  return { filename, rows };
}

function _peCSVTemplateSampleRow() {
  const now = new Date();
  const localToday = new Date(now.getTime() - now.getTimezoneOffset() * 60000).toISOString().slice(0, 10);
  const supplier = (APP_DATA.suppliers || [])[0] || {};
  const staff = (APP_DATA.staffRecords || [])[0] || {};
  const brand = (typeof getBrandMasterRecords === 'function' ? getBrandMasterRecords() : (APP_DATA.brandRecords || []))[0] || {};
  const material = (APP_DATA.materials || [])[0] || {};
  const movement = (APP_DATA.movements || [])[0] || {};
  const beltMaterial = (APP_DATA.beltMaterialRecords || [])[0] || {};
  return [
    localToday, supplier.code || '', staff.code || '', '1',
    `SAMPLE-${localToday.replace(/-/g, '')}-001`, 10000, 1000000,
    brand.code || '', 'サブマリーナ', '116610LN', 'SAMPLE-SERIAL-001',
    material.code || '', movement.code || '', beltMaterial.code || '',
    '入力例：必要に応じて変更してください',
  ];
}

/** 仕入伝票CSV取込と同じ列順で、取込可能な入力例付きテンプレートを出力する。 */
function peDownloadCSVTemplate() {
  const result = _peDownloadCSVRows('仕入登録テンプレート.csv', [PE_PURCHASE_CSV_HEADERS, _peCSVTemplateSampleRow()]);
  showToast('success', 'CSVテンプレート', '入力例1行付きの仕入CSVテンプレートをダウンロードしました');
  return result;
}

function _peCSVFindRecord(records, value) {
  const normalized = String(value || '').trim().toUpperCase();
  if (!normalized) return null;
  return (records || []).find(record =>
    String(record?.code || '').trim().toUpperCase() === normalized
    || String(record?.name || record?.displayName || '').trim().toUpperCase() === normalized
  ) || null;
}

function _peCSVStaffRecord(value) {
  return _peCSVFindRecord(APP_DATA.staffRecords || [], value);
}

function _peCSVExportRow(slip, line) {
  const detail = line.productDetail || {};
  const supplier = _peCSVFindRecord(APP_DATA.suppliers || [], slip.supplier);
  const staff = _peCSVStaffRecord(slip.staffCode || slip.staff);
  const brand = _peCSVFindRecord(typeof getBrandMasterRecords === 'function' ? getBrandMasterRecords() : (APP_DATA.brandRecords || []), detail.brandCode || detail.brand);
  const material = _peCSVFindRecord(APP_DATA.materials || [], detail.material);
  const movement = _peCSVFindRecord(APP_DATA.movements || [], detail.movement);
  const beltMaterial = _peCSVFindRecord(APP_DATA.beltMaterialRecords || [], detail.belt);
  const taxInfo = peGetPurchaseTaxInfo(slip);
  return [
    slip.date || '', supplier?.code || slip.supplier || '',
    staff?.code || slip.staffCode || slip.staff || '', taxInfo.mode === PE_PURCHASE_TAX_DOMESTIC ? '1' : '2', line.sku || '',
    Number(line.salePrice) || 0, Number(line.purchasePrice) || 0,
    brand?.code || detail.brandCode || '', detail.model || '', detail.ref || '', detail.serial || '',
    material?.code || detail.material || '', movement?.code || detail.movement || '',
    beltMaterial?.code || '', detail.note || '',
  ];
}

function peExportCSV() {
  const slips = APP_DATA.purchaseSlips || [];
  if (slips.length === 0 && (!_peSlipData || _peSlipData.lines.length === 0)) {
    showToast('info', 'CSV出力', '出力するデータがありません');
    return;
  }

  const rows = [
    PE_PURCHASE_CSV_HEADERS,
  ];

  // 登録済み + 現在編集中を対象
  const targets = [
    ...slips,
    // 編集中の伝票（未保存）も含める
    ...((_peSlipData && _peSlipData.lines.length > 0) ? [_peSlipData] : []),
  ];

  targets.forEach(slip => {
    (slip.lines || []).forEach(l => {
      rows.push(_peCSVExportRow(slip, l));
    });
  });

  const result = _peDownloadCSVRows(`仕入登録_${new Date().toISOString().slice(0, 10)}.csv`, rows);
  showToast('success', 'CSV出力', 'CSVファイルをダウンロードしました');
  return result;
}

// ── CSV取込 ────────────────────────────────────────────
function peImportCSV() {
  document.getElementById('pe-csv-file-input')?.click();
}

function _peSetCSVImportBusy(busy) {
  const button = document.getElementById('pe-csv-import-button');
  if (!button) return;
  button.disabled = Boolean(busy);
  button.innerHTML = busy
    ? '<i class="fa-solid fa-spinner fa-spin"></i> 読込中'
    : '<i class="fa-solid fa-file-import"></i> CSV取込';
}

function peHandleCSVImport(input) {
  const file = input.files[0];
  if (!file) return;

  const reader = new FileReader();
  reader.onload = async e => {
    _peSetCSVImportBusy(true);
    try {
      await peImportCSVText(String(e.target.result || ''));
    } catch (error) {
      showToast('error', 'CSV一括登録できません', error?.message || '入力内容を確認してください');
    } finally {
      _peSetCSVImportBusy(false);
      input.value = '';
    }
  };
  reader.onerror = () => {
    showToast('error', 'CSV読込エラー', 'ファイルを読み込めませんでした');
    input.value = '';
  };
  reader.readAsText(file, 'UTF-8');
}

/** CSV全体をパース。カンマ・改行・ダブルクォートを含む備考に対応する。 */
function _peParseCSVText(text) {
  const rows = [];
  let row = [];
  let current = '';
  let inQuotes = false;
  const source = String(text || '').replace(/^\uFEFF/, '');
  for (let i = 0; i < source.length; i++) {
    const ch = source[i];
    if (ch === '"') {
      if (inQuotes && source[i + 1] === '"') {
        current += '"'; i++;
      } else {
        inQuotes = !inQuotes;
      }
    } else if (ch === ',' && !inQuotes) {
      row.push(current); current = '';
    } else if ((ch === '\r' || ch === '\n') && !inQuotes) {
      if (ch === '\r' && source[i + 1] === '\n') i++;
      row.push(current); current = '';
      if (row.some(value => String(value || '').trim())) rows.push(row);
      row = [];
    } else {
      current += ch;
    }
  }
  row.push(current);
  if (row.some(value => String(value || '').trim())) rows.push(row);
  return rows;
}

/** 旧来の1行パーサー呼出にも互換性を残す。 */
function _peParseCSVLine(line) {
  return _peParseCSVText(String(line || ''))[0] || [];
}

function _peCSVHeaderGetter(headers, row) {
  const indexes = new Map(headers.map((header, index) => [String(header || '').replace(/^\uFEFF/, '').trim(), index]));
  return (...names) => {
    for (const name of names) {
      if (indexes.has(name)) return String(row[indexes.get(name)] ?? '').trim();
    }
    return '';
  };
}

function _peCSVHasHeader(headers, ...names) {
  const normalized = new Set(headers.map(header => String(header || '').replace(/^\uFEFF/, '').trim()));
  return names.some(name => normalized.has(name));
}

function _peCSVResolveMaster(records, code, name, label, rowNumber, errors, required = false) {
  const codeValue = String(code || '').trim();
  const nameValue = String(name || '').trim();
  const byCode = codeValue ? _peCSVFindRecord(records, codeValue) : null;
  const byName = nameValue ? _peCSVFindRecord(records, nameValue) : null;
  if (codeValue && !byCode) errors.push(`${rowNumber}行目: ${label}コード「${codeValue}」はマスタにありません`);
  if (nameValue && !byName) errors.push(`${rowNumber}行目: ${label}名「${nameValue}」はマスタにありません`);
  if (byCode && byName && byCode.code !== byName.code) {
    errors.push(`${rowNumber}行目: ${label}コードと${label}名が一致しません`);
  }
  const record = byCode || byName;
  if (required && !record && !codeValue && !nameValue) errors.push(`${rowNumber}行目: ${label}を入力してください`);
  return record;
}

function _peCSVParseAmount(value, label, rowNumber, errors) {
  const raw = String(value || '').trim();
  if (!raw) return 0;
  const normalized = raw.replace(/[\s,￥¥$]/g, '');
  const amount = Number(normalized);
  if (!Number.isFinite(amount) || amount < 0) {
    errors.push(`${rowNumber}行目: ${label}は0以上の数字で入力してください`);
    return 0;
  }
  return Math.round(amount);
}

function _peCSVNormalizeTaxMode(value, rowNumber, errors) {
  const normalized = String(value || '').trim().toLowerCase();
  if (!normalized || normalized === '1' || normalized === 'domestic' || normalized === '国内' || normalized === '国内仕入') return PE_PURCHASE_TAX_DOMESTIC;
  if (normalized === '2' || normalized === 'overseas' || normalized === '海外' || normalized === '海外仕入') return PE_PURCHASE_TAX_OVERSEAS;
  errors.push(`${rowNumber}行目: 仕入区分は国内仕入「1」または海外仕入「2」で入力してください`);
  return PE_PURCHASE_TAX_DOMESTIC;
}

function _peCSVGenerateSlipId(date, reserved) {
  const year = /^\d{4}/.test(date || '') ? String(date).slice(0, 4) : String(new Date().getFullYear());
  const prefix = `PI-${year}-`;
  let max = 0;
  [...(APP_DATA.purchaseSlips || []).map(slip => slip.id), ...reserved].forEach(id => {
    const match = String(id || '').match(new RegExp(`^${prefix}(\\d+)$`));
    if (match) max = Math.max(max, Number(match[1]));
  });
  return `${prefix}${String(max + 1).padStart(4, '0')}`;
}

function _peCSVGenerateManagementCode(date, reserved) {
  const prefix = _peItemCodeDatePrefix(date);
  let max = _peMaxRegisteredItemSequence(prefix);
  reserved.forEach(code => {
    const value = String(code || '');
    if (value.startsWith(prefix) && /^\d+$/.test(value.slice(prefix.length))) {
      max = Math.max(max, Number(value.slice(prefix.length)));
    }
  });
  return `${prefix}${String(max + 1).padStart(3, '0')}`;
}

function _peCSVValidateHeaders(headers) {
  const missing = [];
  if (!_peCSVHasHeader(headers, '仕入日')) missing.push('仕入日');
  if (!_peCSVHasHeader(headers, '仕入先コード')) missing.push('仕入先コード');
  if (!_peCSVHasHeader(headers, 'SKU')) missing.push('SKU');
  if (!_peCSVHasHeader(headers, '仕入金額（JPY・税抜）')) missing.push('仕入金額（JPY・税抜）');
  if (!_peCSVHasHeader(headers, '売価（USD）')) missing.push('売価（USD）');
  if (!_peCSVHasHeader(headers, 'ブランドコード')) missing.push('ブランドコード');
  if (missing.length) throw new Error(`必須列がありません: ${missing.join('、')}。最新のCSVテンプレートを使用してください`);
}

function _peBuildPurchaseSlipsFromCSV(text) {
  const rows = _peParseCSVText(text);
  if (rows.length < 2) throw new Error('データ行がありません');
  const headers = rows[0];
  _peCSVValidateHeaders(headers);

  const errors = [];
  const groups = new Map();
  const existingCodes = new Set((APP_DATA.inventory || []).map(item => String(item.code || '').trim()).filter(Boolean));
  const reservedCodes = new Set(existingCodes);
  const reservedSlipIds = new Set((APP_DATA.purchaseSlips || []).map(slip => String(slip.id || '').trim()).filter(Boolean));
  const brands = typeof getBrandMasterRecords === 'function' ? getBrandMasterRecords() : (APP_DATA.brandRecords || []);

  rows.slice(1).forEach((row, index) => {
    const rowNumber = index + 2;
    const get = _peCSVHeaderGetter(headers, row);
    const date = get('仕入日');
    if (!/^\d{4}-\d{2}-\d{2}$/.test(date) || Number.isNaN(new Date(`${date}T00:00:00`).getTime())) {
      errors.push(`${rowNumber}行目: 仕入日はYYYY-MM-DD形式で入力してください`);
    }

    const supplier = _peCSVResolveMaster(APP_DATA.suppliers || [], get('仕入先コード'), '', '仕入先', rowNumber, errors, true);
    const staff = _peCSVResolveMaster(APP_DATA.staffRecords || [], get('仕入担当者コード'), '', '仕入担当者', rowNumber, errors, false);
    // ブランドは仕入時点で不明でも登録可能。値が入力された場合だけマスタ整合性を検証する。
    const brand = _peCSVResolveMaster(brands, get('ブランドコード'), '', 'ブランド', rowNumber, errors, false);
    const material = _peCSVResolveMaster(APP_DATA.materials || [], get('素材コード'), '', '素材', rowNumber, errors, false);
    const movement = _peCSVResolveMaster(APP_DATA.movements || [], get('駆動方式コード'), '', '駆動方式', rowNumber, errors, false);
    const beltMaterial = _peCSVResolveMaster(APP_DATA.beltMaterialRecords || [], get('ベルト素材コード'), '', 'ベルト素材', rowNumber, errors, false);

    const sku = get('SKU');
    if (!sku) errors.push(`${rowNumber}行目: SKUを入力してください`);
    const purchasePrice = _peCSVParseAmount(get('仕入金額（JPY・税抜）'), '仕入金額', rowNumber, errors);
    const salePrice = _peCSVParseAmount(get('売価（USD）'), '売価', rowNumber, errors);
    const purchaseTaxMode = _peCSVNormalizeTaxMode(get('仕入区分'), rowNumber, errors);

    // CSV 1ファイルを1枚の仕入伝票として扱う。伝票番号は登録時に自動採番する。
    const groupKey = '__AUTO__';
    if (!groups.has(groupKey)) {
      const slipId = _peCSVGenerateSlipId(date, reservedSlipIds);
      reservedSlipIds.add(slipId);
      groups.set(groupKey, {
        id: slipId, date, supplier: supplier?.code || '', staff: staff?.code || '',
        purchaseTaxMode, taxRateBasisPoints: purchaseTaxMode === PE_PURCHASE_TAX_OVERSEAS ? 0 : PE_DOMESTIC_TAX_RATE_BASIS_POINTS,
        note: '', lines: [], status: '未処理', source: 'csv-import', revisions: [],
      });
    }

    const slip = groups.get(groupKey);
    const mismatches = [
      [slip.date, date, '仕入日'], [slip.supplier, supplier?.code || '', '仕入先'],
      [slip.staff, staff?.code || '', '仕入担当者'], [slip.purchaseTaxMode, purchaseTaxMode, '仕入区分'],
    ];
    mismatches.forEach(([expected, actual, label]) => {
      if (String(expected || '') !== String(actual || '')) errors.push(`${rowNumber}行目: 同じ仕入伝票内で${label}が一致しません`);
    });

    const code = _peCSVGenerateManagementCode(date, reservedCodes);
    if (reservedCodes.has(code)) errors.push(`${rowNumber}行目: 管理番号「${code}」は既に使用されています`);
    else reservedCodes.add(code);

    slip.lines.push({
      lineId: slip.lines.length + 1,
      lineNo: slip.lines.length + 1,
      code, sku, purchasePrice, salePrice,
      productDetail: {
        brand: brand?.name || '', brandCode: brand?.code || '', model: get('モデル名'), ref: get('型番（Ref.）'),
        serial: get('シリアルNo.'), material: material?.code || '', movement: movement?.code || '', condition: '',
        belt: beltMaterial?.name || '', dial: '', boxNo: null,
        accessories: [], braceletQty: null, note: get('特徴・備考'),
      },
    });
  });

  if (errors.length) {
    const visibleErrors = errors.slice(0, 8).join(' / ');
    const suffix = errors.length > 8 ? ` / 他${errors.length - 8}件` : '';
    throw new Error(`${visibleErrors}${suffix}`);
  }
  return [...groups.values()];
}

async function _peCommitPurchaseCSVSlips(slips) {
  const productCount = slips.reduce((sum, slip) => sum + slip.lines.length, 0);
  let slipCount = 0;
  let approvalCount = 0;
  try {
    for (const slip of slips) {
      if (window.ZaikoAPI) {
        const result = await window.ZaikoAPI.savePurchaseSlip(slip, typeof isWorker === 'function' && isWorker());
        if (result?.approval) approvalCount++;
      } else {
        slip.registeredAt = new Date().toISOString().slice(0, 16).replace('T', ' ');
        peApplyPurchaseRegistrationFXSnapshot(slip);
        _peRegisterAllToInventory(slip);
        APP_DATA.purchaseSlips.push(JSON.parse(JSON.stringify(slip)));
      }
      slipCount++;
    }
  } catch (error) {
    if (slipCount > 0) error.message = `${slipCount}伝票は登録済みです。${error.message || '後続の伝票を登録できませんでした'}`;
    throw error;
  }
  return { slipCount, productCount, approvalCount };
}

/**
 * CSVの内容を仕入登録フォームへ展開する。
 * この時点では伝票・在庫を保存せず、利用者が明細を確認して
 * 「仕入登録する」を押した時だけ通常の登録処理へ進む。
 */
function peImportCSVText(text) {
  const slips = _peBuildPurchaseSlipsFromCSV(text);
  if (!slips.length || !slips[0]?.lines?.length) throw new Error('取込対象の仕入明細がありません');

  const stagedSlip = JSON.parse(JSON.stringify(slips[0]));
  _peSlipData = stagedSlip;
  _peLineCount = stagedSlip.lines.reduce((max, line) => Math.max(max, Number(line.lineId) || 0), 0);
  _peProductTargetLineId = null;

  _peFillSelects();
  const idEl = document.getElementById('pe-slip-id');
  const dateEl = document.getElementById('pe-date');
  const supplierEl = document.getElementById('pe-supplier');
  const staffEl = document.getElementById('pe-staff');
  if (idEl) idEl.value = stagedSlip.id;
  if (dateEl) dateEl.value = stagedSlip.date || '';
  if (supplierEl) supplierEl.value = stagedSlip.supplier || '';
  if (staffEl) staffEl.value = stagedSlip.staff || '';

  _peUpdatePurchaseTaxModeUI();
  const tbody = document.getElementById('pe-detail-tbody');
  if (tbody) tbody.innerHTML = '';
  stagedSlip.lines.forEach(line => _peRenderLine(line));
  _peUpdateDetailUI();

  showToast('success', 'CSVを明細へ読み込みました', `${stagedSlip.lines.length}件を確認してください。まだ登録は完了していません`);
  return {
    staged: stagedSlip.lines.length,
    slipCount: 0,
    productCount: 0,
    approvalCount: 0,
  };
}

// ── 印刷プレビュー ────────────────────────────────────
function pePrintPreview() {
  if (!_peSlipData) return;
  _peSlipData.date = document.getElementById('pe-date')?.value || '';
  _peSlipData.supplier = document.getElementById('pe-supplier')?.value || '';
  _peSlipData.staff = document.getElementById('pe-staff')?.value || '';
  const purchaseTax = peGetPurchaseTaxInfo(_peSlipData);

  const supplier = APP_DATA.suppliers.find(record => record.code === _peSlipData.supplier)
    || { name: _peSlipData.supplier ? getSupplierName(_peSlipData.supplier) : '（仕入先未設定）' };
  const items = (_peSlipData.lines || []).map((line, index) => {
    const inventoryItem = APP_DATA.inventory.find(item => item.code === line.code) || {};
    const detail = line.productDetail || {};
    const accessories = Array.isArray(detail.accessories) && detail.accessories.length
      ? detail.accessories
      : (inventoryItem.accessories || []);
    const description = [
      [detail.brand || inventoryItem.brand, detail.model || inventoryItem.model].filter(Boolean).join(' / '),
      [
        (detail.ref || inventoryItem.ref) && `型番: ${detail.ref || inventoryItem.ref}`,
        (detail.serial || inventoryItem.serial) && `シリアル: ${detail.serial || inventoryItem.serial}`,
      ].filter(Boolean).join('　'),
      `付属品: ${accessories.length ? accessories.join('・') : 'なし'}`,
      (detail.note || inventoryItem.note) ? `備考: ${detail.note || inventoryItem.note}` : '',
    ].filter(Boolean).join('\n') || line.code || '—';
    return {
      no: line.lineNo || index + 1,
      detail: description,
      amount: Number(line.purchasePrice) || 0,
      code: line.code || '',
    };
  });

  const html = buildTemplateStyleSlipDocument({
    title: '仕入伝票',
    slipId: _peSlipData.id,
    transactionDate: _peSlipData.date,
    transactionDateLabel: '仕入日',
    counterpartyLabel: '仕入先',
    counterparty: supplier,
    items,
    note: _peSlipData.note || '',
    formatAmount: amount => formatPurchaseSlipAmount(amount, _peSlipData),
    currencyLabel: getPurchaseSlipCurrencyLabel(_peSlipData),
    issuedAt: _peSlipData.issuedAt,
    issuedDateLabel: '発行日時',
    taxMode: purchaseTax.mode === PE_PURCHASE_TAX_DOMESTIC ? 'standard' : 'out_of_scope',
    includeBank: false,
    summaryMessage: '商品代金として、弊社より下記金額をお支払いいたします。',
    amountCaption: purchaseTax.mode === PE_PURCHASE_TAX_DOMESTIC ? '仕入合計金額（税込）' : '仕入合計金額',
    itemCountCaption: '商品点数',
  });

  const contentEl = document.getElementById('pePrintPreviewContent');
  if (contentEl) contentEl.innerHTML = html;
  const modal = document.getElementById('pePrintModal');
  if (modal) modal.style.display = 'flex';
}

/** @deprecated 雛形変更前の帳票。比較参照用に保持 */
function _peLegacyPrintPreview() {
  if (!_peSlipData) return;
  _peSlipData.date     = document.getElementById('pe-date')?.value || '';
  _peSlipData.supplier = document.getElementById('pe-supplier')?.value || '';
  _peSlipData.staff    = document.getElementById('pe-staff')?.value || '';

  const suppName = _peSlipData.supplier ? getSupplierName(_peSlipData.supplier) : '—';
  const totalP = (_peSlipData.lines || []).reduce((s, l) => s + (l.purchasePrice || 0), 0);
  const totalS = (_peSlipData.lines || []).reduce((s, l) => s + (l.salePrice || 0), 0);

  const html = `
    <div style="font-size:18px;font-weight:bold;text-align:center;margin-bottom:20px;border-bottom:2px solid #333;padding-bottom:10px;">
      仕　入　伝　票
    </div>
    <div style="display:grid;grid-template-columns:1fr 1fr;gap:8px;font-size:13px;margin-bottom:16px;">
      <div><strong>伝票番号：</strong>${_escHtml(_peSlipData.id)}</div>
      <div><strong>仕入日：</strong>${_escHtml(_peSlipData.date)}</div>
      <div><strong>仕入先：</strong>${_escHtml(suppName)}</div>
      <div><strong>担当者：</strong>${_escHtml(peGetStaffDisplayName(_peSlipData.staff))}</div>
    </div>
    <table style="width:100%;border-collapse:collapse;font-size:12px;margin-bottom:16px;">
      <thead>
        <tr style="background:#f0f2f5;">
          <th style="border:1px solid #ccc;padding:6px;text-align:center;width:36px;">No.</th>
          <th style="border:1px solid #ccc;padding:6px;">商品コード</th>
          <th style="border:1px solid #ccc;padding:6px;">SKU</th>
          <th style="border:1px solid #ccc;padding:6px;text-align:right;">仕入金額（JPY）</th>
          <th style="border:1px solid #ccc;padding:6px;text-align:right;">売価（USD）</th>
        </tr>
      </thead>
      <tbody>
        ${(_peSlipData.lines || []).map(l => `
        <tr>
          <td style="border:1px solid #ccc;padding:6px;text-align:center;">${l.lineNo}</td>
          <td style="border:1px solid #ccc;padding:6px;font-family:monospace;font-size:11px;">${_escHtml(l.code)}</td>
          <td style="border:1px solid #ccc;padding:6px;">${_escHtml(l.sku)}</td>
          <td style="border:1px solid #ccc;padding:6px;text-align:right;">¥${(l.purchasePrice || 0).toLocaleString('ja-JP')}</td>
          <td style="border:1px solid #ccc;padding:6px;text-align:right;">${formatSalePrice(l.salePrice || 0)}</td>
        </tr>`).join('')}
      </tbody>
      <tfoot>
        <tr style="background:#f0f2f5;font-weight:bold;">
          <td colspan="3" style="border:1px solid #ccc;padding:6px;text-align:right;">合　計</td>
          <td style="border:1px solid #ccc;padding:6px;text-align:right;color:#1a3a5c;">¥${totalP.toLocaleString('ja-JP')}</td>
          <td style="border:1px solid #ccc;padding:6px;text-align:right;color:#27ae60;">${formatSalePrice(totalS)}</td>
        </tr>
      </tfoot>
    </table>
    <div style="font-size:11px;color:#888;text-align:right;">
      出力日時: ${new Date().toLocaleString('ja-JP')}
    </div>`;

  const contentEl = document.getElementById('pePrintPreviewContent');
  if (contentEl) contentEl.innerHTML = html;

  const modal = document.getElementById('pePrintModal');
  if (modal) modal.style.display = 'flex';
}

function peclosePrintModal() {
  const modal = document.getElementById('pePrintModal');
  if (modal) modal.style.display = 'none';
}

function peDownloadDocument() {
  const content = document.getElementById('pePrintPreviewContent')?.innerHTML || '';
  const slipId = _peSlipData?.id || document.getElementById('pe-id')?.value?.trim() || 'purchase-slip';
  if (typeof _downloadTemplateDocument !== 'function') {
    showToast('warning', '帳票ダウンロード', 'ダウンロード機能を読み込めませんでした。');
    return;
  }
  _downloadTemplateDocument('仕入伝票', `${slipId}_仕入伝票.html`, content);
}

function peExecPrint() {
  const content = document.getElementById('pePrintPreviewContent')?.innerHTML || '';
  const w = window.open('', '_blank', 'width=820,height=700');
  w.document.write(`<!DOCTYPE html><html><head>
    <meta charset="UTF-8">
    <title>仕入伝票</title>
    <style>
      body { font-family: "Hiragino Sans","Meiryo",sans-serif; margin: 24px 32px; }
      @media print { body { margin: 0; } }
    </style>
  </head><body>${content}</body></html>`);
  w.document.close();
  w.focus();
  setTimeout(() => { w.print(); }, 300);
}

// ── ユーティリティ ──────────────────────────────────────
function _escHtml(str) {
  if (str == null) return '';
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}
