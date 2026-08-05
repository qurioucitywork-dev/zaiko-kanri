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
  _peSlipData = {
    id: peGenerateSlipId(),
    date: '',
    supplier: '',
    staff: '',
    note: '',
    lines: [],
    status: '未処理',
    revisions: [],
    registeredAt: null,
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

  // 明細テーブルをクリア
  const tbody = document.getElementById('pe-detail-tbody');
  if (tbody) tbody.innerHTML = '';

  _peUpdateDetailUI();
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
        title="商品登録ポップアップを開く">
        <i class="fa-solid fa-box-open"></i> 登録
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

  // 集計
  let totalP = 0, totalS = 0;
  lines.forEach(l => {
    totalP += l.purchasePrice || 0;
    totalS += l.salePrice || 0;
  });

  const cntEl = document.getElementById('pe-line-count');
  const tpEl  = document.getElementById('pe-total-purchase');
  const tsEl  = document.getElementById('pe-total-sale');
  if (cntEl) cntEl.textContent = lines.length;
  if (tpEl)  tpEl.textContent  = '¥' + totalP.toLocaleString('ja-JP');
  if (tsEl)  tsEl.textContent  = formatSalePrice(totalS);
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
  if (!_peSlipData) return;

  // ヘッダーの現在値を同期
  _peSlipData.date     = document.getElementById('pe-date')?.value || '';
  _peSlipData.supplier = document.getElementById('pe-supplier')?.value || '';
  _peSlipData.staff    = document.getElementById('pe-staff')?.value || '';

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
    const incomplete = _peSlipData.lines.filter(line => !line.productDetail?.brand);
    if (incomplete.length > 0) {
      showToast('error', '入力エラー', `明細No.${incomplete.map(line => line.lineNo).join(', ')} の商品登録からブランドを選択してください`);
      return;
    }
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
    }
    return;
  }

  // 保存
  _peSlipData.registeredAt = new Date().toISOString().slice(0, 16).replace('T', ' ');

  // 全明細を在庫に一括登録（重複コードはスキップ）
  _peRegisterAllToInventory(_peSlipData);

  APP_DATA.purchaseSlips.push(JSON.parse(JSON.stringify(_peSlipData)));

  const inventoryCount = (_peSlipData.lines || []).length;
  showToast('success', '仕入登録完了', `伝票 ${_peSlipData.id} を登録し、${inventoryCount}件を在庫に追加しました`);

  // ④ 伝票追加後にタスク数を再計算
  if (typeof refreshLinkedBusinessViews === 'function') refreshLinkedBusinessViews({ source: 'purchase-entry' });

  // 次の伝票を準備
  _peInitNewSlip();
  _peFillSelects();
  peRenderList();
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

  if (countEl) countEl.textContent = `全 ${slips.length} 件`;
  if (emptyEl) emptyEl.style.display = slips.length === 0 ? '' : 'none';

  // 降順（新しい順）
  const sorted = [...slips].sort((a, b) => (b.id > a.id ? 1 : -1));

  tbody.innerHTML = sorted.map(slip => {
    const totalP = (slip.lines || []).reduce((s, l) => s + (l.purchasePrice || 0), 0);
    const totalS = (slip.lines || []).reduce((s, l) => s + (l.salePrice || 0), 0);
    const suppName = slip.supplier ? getSupplierName(slip.supplier) : '—';
    const staffName = peGetStaffDisplayName(slip.staff);
    return `<tr>
      <td><strong>${_escHtml(slip.id)}</strong></td>
      <td>${_escHtml(slip.date)}</td>
      <td>${_escHtml(suppName)}</td>
      <td>${_escHtml(staffName)}</td>
      <td style="text-align:right;">${(slip.lines || []).length} 件</td>
      <td style="text-align:right;font-weight:bold;color:var(--primary);">¥${totalP.toLocaleString('ja-JP')}</td>
      <td style="text-align:right;font-weight:bold;color:var(--success);">${formatSalePrice(totalS)}</td>
      <td style="text-align:center;">
        <button class="btn btn-outline btn-sm" onclick="peViewSlip('${_escHtml(slip.id)}')">
          <i class="fa-solid fa-eye"></i>
        </button>
        <button class="btn btn-sm" style="background:none;border:none;color:#e74c3c;cursor:pointer;font-size:14px;"
          onclick="peDeleteSlip('${_escHtml(slip.id)}')" title="削除">
          <i class="fa-solid fa-trash-can"></i>
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
  const totalP = (slip.lines || []).reduce((s, l) => s + (l.purchasePrice || 0), 0);
  const totalS = (slip.lines || []).reduce((s, l) => s + (l.salePrice || 0), 0);

  const titleEl = document.getElementById('peViewModalTitle');
  const bodyEl  = document.getElementById('peViewModalBody');
  if (titleEl) titleEl.innerHTML = `<i class="fa-solid fa-file-invoice"></i> ${_escHtml(slip.id)}`;
  if (bodyEl) {
    bodyEl.innerHTML = `
      <div class="form-row cols-2" style="margin-bottom:12px;">
        <div><label class="form-label">仕入日</label><p>${_escHtml(slip.date)}</p></div>
        <div><label class="form-label">仕入先</label><p>${_escHtml(suppName)}</p></div>
        <div><label class="form-label">担当者</label><p>${_escHtml(staffName)}</p></div>
        <div><label class="form-label">登録日時</label><p>${_escHtml(slip.registeredAt || '—')}</p></div>
      </div>
      <table class="data-table" style="margin-bottom:0;">
        <thead>
          <tr>
            <th>No.</th><th>商品コード</th><th>SKU</th>
            <th style="text-align:right;">仕入金額（JPY）</th><th style="text-align:right;">売価（USD）</th>
          </tr>
        </thead>
        <tbody>
          ${(slip.lines || []).map(l => `<tr>
            <td>${l.lineNo}</td>
            <td><code style="font-size:11px;">${_escHtml(l.code)}</code></td>
            <td>${_escHtml(l.sku)}</td>
            <td style="text-align:right;">¥${(l.purchasePrice || 0).toLocaleString('ja-JP')}</td>
            <td style="text-align:right;">${formatSalePrice(l.salePrice || 0)}</td>
          </tr>`).join('')}
        </tbody>
        <tfoot>
          <tr style="background:var(--bg);">
            <td colspan="3" style="text-align:right;font-weight:bold;">合計</td>
            <td style="text-align:right;font-weight:bold;color:var(--primary);">¥${totalP.toLocaleString('ja-JP')}</td>
            <td style="text-align:right;font-weight:bold;color:var(--success);">${formatSalePrice(totalS)}</td>
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
      purchasePrice: line.purchasePrice || 0,
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
function peExportCSV() {
  const slips = APP_DATA.purchaseSlips || [];
  if (slips.length === 0 && (!_peSlipData || _peSlipData.lines.length === 0)) {
    showToast('info', 'CSV出力', '出力するデータがありません');
    return;
  }

  const rows = [
    ['仕入伝票番号', '仕入日', '仕入先コード', '仕入先名', '担当者',
     '明細No.', '商品コード', 'SKU', '仕入金額（JPY）', '売価（USD）', '登録日時'],
  ];

  // 登録済み + 現在編集中を対象
  const targets = [
    ...slips,
    // 編集中の伝票（未保存）も含める
    ...((_peSlipData && _peSlipData.lines.length > 0) ? [_peSlipData] : []),
  ];

  targets.forEach(slip => {
    const suppName = slip.supplier ? getSupplierName(slip.supplier) : '';
    (slip.lines || []).forEach(l => {
      rows.push([
        slip.id,
        slip.date,
        slip.supplier || '',
        suppName,
        peGetStaffDisplayName(slip.staff, ''),
        l.lineNo,
        l.code,
        l.sku,
        l.purchasePrice || 0,
        l.salePrice || 0,
        slip.registeredAt || '',
      ]);
    });
  });

  const csv = rows.map(r => r.map(v => `"${String(v).replace(/"/g, '""')}"`).join(',')).join('\r\n');
  const bom  = '\uFEFF';
  const blob = new Blob([bom + csv], { type: 'text/csv;charset=utf-8;' });
  const url  = URL.createObjectURL(blob);
  const a    = document.createElement('a');
  a.href     = url;
  a.download = `仕入登録_${new Date().toISOString().slice(0, 10)}.csv`;
  a.click();
  URL.revokeObjectURL(url);
  showToast('success', 'CSV出力', 'CSVファイルをダウンロードしました');
}

// ── CSV取込 ────────────────────────────────────────────
function peImportCSV() {
  document.getElementById('pe-csv-file-input')?.click();
}

function peHandleCSVImport(input) {
  const file = input.files[0];
  if (!file) return;

  const reader = new FileReader();
  reader.onload = e => {
    const text = e.target.result.replace(/^\uFEFF/, ''); // BOM除去
    const lines = text.split(/\r?\n/).filter(l => l.trim());
    if (lines.length < 2) {
      showToast('error', 'CSV取込', 'データ行がありません');
      return;
    }

    // ヘッダー行をスキップ（1行目）
    const dataLines = lines.slice(1);
    const imported = {};
    let addedCount = 0;
    let skippedCount = 0;

    // 既存在庫コードを収集
    const existingCodes = new Set((APP_DATA.inventory || []).map(i => i.code));

    dataLines.forEach(line => {
      const cols = _peParseCSVLine(line);
      if (cols.length < 9) return;

      const [slipId, date, suppCode, , staff, lineNo, code, sku, purchasePrice, salePrice, registeredAt] = cols;

      // 商品コード重複チェック
      if (existingCodes.has(code)) {
        skippedCount++;
        return;
      }
      existingCodes.add(code);

      if (!imported[slipId]) {
        // 新しい伝票IDが既存と重複しないようにチェック
        const isDupSlip = (APP_DATA.purchaseSlips || []).some(s => s.id === slipId);
        imported[slipId] = {
          id: isDupSlip ? peGenerateSlipId() : slipId,
          date: date || '',
          supplier: suppCode || '',
          staff: staff || '',
          lines: [],
          status: '未処理',
          source: 'csv-import',
          revisions: [],
          registeredAt: registeredAt || new Date().toISOString().slice(0, 16).replace('T', ' '),
        };
      }

      const slip = imported[slipId];
      slip.lines.push({
        lineId: slip.lines.length + 1,
        lineNo: parseInt(lineNo, 10) || slip.lines.length + 1,
        code: code || '',
        sku: sku || '',
        purchasePrice: parseInt(String(purchasePrice).replace(/[^0-9]/g, ''), 10) || 0,
        salePrice: parseInt(String(salePrice).replace(/[^0-9]/g, ''), 10) || 0,
        productDetail: null,
      });
      addedCount++;
    });

    // APP_DATA に追加
    Object.values(imported).forEach(slip => {
      if (slip.lines.length > 0) {
        APP_DATA.purchaseSlips.push(slip);
        if (typeof syncPurchaseSlipToInventory === 'function') syncPurchaseSlipToInventory(slip);
      }
    });

    peRenderList();
    if (typeof refreshLinkedBusinessViews === 'function') refreshLinkedBusinessViews({ source: 'purchase-csv-import' });
    showToast('success', 'CSV取込完了',
      `${addedCount}件取込 / ${skippedCount}件スキップ（重複コード）`);
    input.value = ''; // リセット
  };
  reader.readAsText(file, 'UTF-8');
}

/** CSV 1行をパース（ダブルクォート対応） */
function _peParseCSVLine(line) {
  const result = [];
  let current = '';
  let inQuotes = false;
  for (let i = 0; i < line.length; i++) {
    const ch = line[i];
    if (ch === '"') {
      if (inQuotes && line[i + 1] === '"') {
        current += '"'; i++;
      } else {
        inQuotes = !inQuotes;
      }
    } else if (ch === ',' && !inQuotes) {
      result.push(current); current = '';
    } else {
      current += ch;
    }
  }
  result.push(current);
  return result;
}

// ── 印刷プレビュー ────────────────────────────────────
function pePrintPreview() {
  if (!_peSlipData) return;
  _peSlipData.date = document.getElementById('pe-date')?.value || '';
  _peSlipData.supplier = document.getElementById('pe-supplier')?.value || '';
  _peSlipData.staff = document.getElementById('pe-staff')?.value || '';

  const supplier = APP_DATA.suppliers.find(record => record.code === _peSlipData.supplier)
    || { name: _peSlipData.supplier ? getSupplierName(_peSlipData.supplier) : '（仕入先未設定）' };
  const items = (_peSlipData.lines || []).map((line, index) => {
    const inventoryItem = APP_DATA.inventory.find(item => item.code === line.code) || {};
    const detail = line.productDetail || {};
    const accessories = detail.accessories || inventoryItem.accessories || [];
    const description = [
      [detail.brand || inventoryItem.brand, detail.model || inventoryItem.model].filter(Boolean).join(' / '),
      [
        (detail.ref || inventoryItem.ref) && `型番: ${detail.ref || inventoryItem.ref}`,
        (detail.serial || inventoryItem.serial) && `シリアル: ${detail.serial || inventoryItem.serial}`,
      ].filter(Boolean).join('　'),
      line.sku ? `SKU: ${line.sku}` : '',
      accessories.length ? `付属品: ${accessories.join('・')}` : '',
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
    formatAmount: amount => formatPrice(amount),
    currencyLabel: 'JPY（円）',
    taxMode: 'none',
    includeBank: false,
    summaryMessage: '商品代金として、弊社より下記金額をお支払いいたします。',
    amountCaption: '仕入合計金額',
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
