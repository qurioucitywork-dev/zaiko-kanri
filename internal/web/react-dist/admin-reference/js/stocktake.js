/**
 * stocktake.js — 棚卸機能
 * 既存 APP_DATA / formatPrice / showToast / requestApproval を利用。
 * 既存コード・画面には一切影響を与えない独立モジュール。
 */

'use strict';

/* ============================================================
   状態管理
   ============================================================ */
/**
 * _stkState[code] = {
 *   code          : string,
 *   state         : '未処理' | '棚卸済' | '不一致' | '承認待ち',
 *   reason        : string | null,   // 不一致理由
 *   note          : string | null,   // 備考
 *   checkedAt     : string | null,   // 棚卸済にした日時
 *   inventoryDate : string | null,   // ① 棚卸実施日（棚卸確定時に一括設定）
 * }
 */
let _stkState = {};

/** 現在アクティブなタブ  'instock' | 'shipped' */
let _stkCurrentTab = 'instock';

/** 理由入力モーダルで選択中の商品コード */
let _stkReasonTargetCode = null;

/** 選択中の理由 */
let _stkSelectedReason = null;

/** 一時保存済みフラグ */
let _stkSaved = false;

/** 棚卸完了フラグ */
let _stkCompleted = false;

/** DBへ保存される棚卸セッション。開始ボタン押下時点の対象在庫を固定する。 */
let _stkSession = null;
let _stkLoading = false;

const _STK_TARGET_STATUS = new Set(['在庫中', '取置中', '出荷済', '委託中', '仕入返品中']);

function _stkNormalizeInventoryStatus(status) {
  const value = String(status || '').trim();
  if (['仕入返品中', '仕入返品'].includes(value)) return '仕入返品中';
  if (['仕入返品済', '取消済', '取り消し', '取消'].includes(value)) return '仕入返品済';
  return value;
}

function _stkLineToItem(line) {
  const live = (APP_DATA.inventory || []).find(item => item.code === line.productCode) || {};
  return {
    ...live,
    code: line.productCode,
    brand: line.brand || live.brand || '—',
    model: line.modelNumber || live.model || '—',
    ref: line.referenceNumber || live.ref || '—',
    serial: line.serialNumber || live.serial || '—',
    purchasePrice: Number(line.purchasePriceMinor || live.purchasePrice || 0),
    status: _stkNormalizeInventoryStatus(live.status || _stkStatusLabel(line.inventoryStatus)),
    _stocktakeLine: line,
  };
}

function _stkStatusLabel(value) {
  return ({ in_stock:'在庫中', reserved:'取置中', shipped:'出荷済', consigned:'委託中', return_pending:'仕入返品中', cancelled:'仕入返品済' })[value] || value || '—';
}

function _stkSnapshotItems() {
  if (!_stkSession) return [];
  return (_stkSession.lines || [])
    .filter(line => line.lineType === 'expected_missing')
    .map(_stkLineToItem)
    .filter(item => _stkNormalizeInventoryStatus(item.status) !== '仕入返品済');
}

function _stkUnknownItems() {
  if (!_stkSession) return [];
  return (_stkSession.lines || [])
    .filter(line => line.lineType === 'unknown_inventory')
    .map(_stkLineToItem)
    .filter(item => _stkNormalizeInventoryStatus(item.status) !== '仕入返品済');
}

function _stkSyncStateFromSession() {
  _stkState = {};
  (_stkSession?.lines || []).forEach(line => {
    _stkState[line.productCode] = {
      code: line.productCode,
      lineId: line.id,
      lineType: line.lineType,
      state: line.lineType === 'unknown_inventory' ? '不明在庫' : (line.resultStatus === 'verified' ? '棚卸済' : (line.reason ? '不一致' : '未処理')),
      reason: line.reason || null,
      note: line.note || null,
      checkedAt: line.checkedAt ? new Date(line.checkedAt).toLocaleString('ja-JP') : null,
      inventoryDate: _stkSession.completedAt ? String(_stkSession.completedAt).slice(0, 10) : null,
    };
  });
  _stkSaved = Boolean(_stkSession?.savedAt);
  _stkCompleted = _stkSession?.status === 'completed';
}

async function _stkLoadOrStartSession() {
  if (!window.ZaikoAPI?.request) return null;
  const current = await window.ZaikoAPI.request('/stocktakes/current');
  _stkSession = current.session || null;
  _stkSyncStateFromSession();
  return _stkSession;
}

async function stkStartSession() {
  if (_stkSession || !window.ZaikoAPI?.request) return;
  const result = await window.ZaikoAPI.request('/stocktakes/start', { method:'POST', body:'{}' });
  _stkSession = result.session;
  _stkSyncStateFromSession();
  await initStocktake();
  showToast('success', '棚卸を開始しました', '開始時点の対象データを固定しました。');
}

/* ============================================================
   初期化
   ============================================================ */
async function initStocktake() {
  if (_stkLoading) return;
  _stkLoading = true;
  try {
    await _stkLoadOrStartSession();
  } catch (error) {
    console.error('stocktake session load failed', error);
    if (typeof showToast === 'function') showToast('error', '棚卸データ取得エラー', error.message || '棚卸途中データを取得できませんでした。');
  } finally { _stkLoading = false; }
  const startButton = document.getElementById('stkStartButton');
  if (startButton) {
    startButton.classList.remove('hidden');
    startButton.disabled = Boolean(_stkSession);
    startButton.innerHTML = _stkSession
      ? '<i class="fa-solid fa-lock"></i> 棚卸開始済み（データ固定中）'
      : '<i class="fa-solid fa-play"></i> 棚卸開始';
  }
  const sessionLabel = document.getElementById('stkSessionLabel');
  if (sessionLabel && _stkSession) {
    const scanner = (APP_DATA.users || []).find(user => user.id === _stkSession.startedBy)?.name || _stkSession.startedBy;
    sessionLabel.textContent = `実施日: ${String(_stkSession.startedAt).slice(0,10)} / スキャン担当者: ${scanner}`;
  }
  // 既に完了している場合
  if (_stkCompleted) {
    _stkUpdateBadge('complete', '完了済み');
    stkRenderTable();
    stkUpdateSummary();
    stkUpdateProgress();
    return;
  }

  _stkCurrentTab = 'instock';

  // タブのアクティブ化
  const tabInStock = document.getElementById('stkTabInStock');
  const tabShipped = document.getElementById('stkTabShipped');
  if (tabInStock) tabInStock.classList.add('active');
  if (tabShipped) tabShipped.classList.remove('active');

  // バッジ更新
  if (_stkSaved) {
    _stkUpdateBadge('saved', '途中保存');
  } else {
    _stkUpdateBadge('active', '進行中');
  }

  // 入力フォーカス
  const input = document.getElementById('stkCodeInput');
  if (input) {
    input.value = '';
    input.addEventListener('keydown', _stkOnKeydown);
    input.focus();
  }

  stkRenderTable();
  stkUpdateSummary();
  stkUpdateProgress();
  _stkUpdateMismatchBadge();
}

/* ============================================================
   コード入力ハンドラ
   ============================================================ */
function _stkOnKeydown(e) {
  if (e.key === 'Enter') stkHandleInput();
}

async function stkHandleInput() {
  const input = document.getElementById('stkCodeInput');
  if (!input) return;
  const code = input.value.trim();
  if (!code) return;

  const item = (APP_DATA.inventory || []).find(i => i.code === code);

  if (_stkSession && window.ZaikoAPI?.request) {
    try {
      const result = await window.ZaikoAPI.request(`/stocktakes/${encodeURIComponent(_stkSession.id)}/scan`, { method:'POST', body:JSON.stringify({ code }) });
      _stkSession = result.session;
      _stkSyncStateFromSession();
      stkRenderTable(); stkUpdateSummary(); stkUpdateProgress(); _stkUpdateMismatchBadge();
      input.value = ''; input.focus();
      if (result.result === 'cancelled_ignored') {
        _stkShowPopup('info', '仕入返品済商品は棚卸対象外です', `管理番号「${code}」は仕入返品済のため、棚卸リスト・不一致リストには追加しません。`);
      } else if (['unknown_added','already_unknown'].includes(result.result)) {
        _stkShowPopup('warning', '不明在庫として追加しました', `管理番号「${code}」は棚卸対象外または未登録のため、不一致リストへ追加しました。`);
      } else if (result.result === 'already_verified') {
        _stkShowPopup('info', 'すでに棚卸済です', `管理番号「${code}」はすでに棚卸済です。`);
      } else if (result.result === 'document_unchecked') {
        _stkShowPopup('warning', '先に伝票の商品を確認してください', `管理番号「${code}」は出荷・委託中です。「出荷済・委託中」タブで該当伝票の商品にチェックを入れて確定してください。`);
      } else if (['duplicate_presence','already_duplicate_presence'].includes(result.result)) {
        _stkShowPopup('warning', '二重確認を不一致へ追加しました', `伝票側で確認済みの管理番号「${code}」が実在庫としてスキャンされました。在庫戻し忘れの可能性があるため、不一致リストへ追加しました。`);
      } else {
        _stkShowToast(`<i class="fa-solid fa-check-circle" style="color:#16a34a;"></i> 棚卸済にしました: <b>${_esc(item?.brand || code)} ${_esc(item?.model || '')}</b>`);
      }
      return;
    } catch (error) {
      _stkShowPopup('warning', '棚卸結果を保存できません', error.message || '通信状態を確認してください。'); input.select(); return;
    }
  }

  if (item && _stkNormalizeInventoryStatus(item.status) === '仕入返品済') {
    _stkShowPopup('info', '仕入返品済商品は棚卸対象外です', `管理番号「${code}」は仕入返品済のため、棚卸リスト・不一致リストには追加しません。`);
    input.value = ''; input.focus(); return;
  }

  if (!item || !_STK_TARGET_STATUS.has(_stkNormalizeInventoryStatus(item.status))) {
    _stkState[code] = { code, state:'不明在庫', reason:'不明在庫', note:'', checkedAt:new Date().toLocaleString('ja-JP'), lineType:'unknown_inventory' };
    _stkShowPopup('warning', '不明在庫として追加しました', `管理番号「${code}」を不一致リストへ追加しました。`);
    input.value = ''; _stkUpdateMismatchBadge(); return;
  }

  // 既に棚卸済の場合
  if (_stkState[code]?.state === '棚卸済') {
    _stkShowPopup('info', 'すでに棚卸済です', `${item.brand} ${item.model}（${code}）はすでに棚卸済です。`);
    input.select();
    return;
  }

  // 状態を棚卸済に更新
  _stkState[code] = {
    ..._stkState[code],
    state: '棚卸済',
    checkedAt: new Date().toLocaleString('ja-JP'),
  };

  // 対応するタブに切り替え
  const targetTab = ['出荷済', '委託中'].includes(_stkNormalizeInventoryStatus(item.status)) ? 'shipped' : 'instock';
  if (_stkCurrentTab !== targetTab) {
    const btn = document.getElementById(targetTab === 'instock' ? 'stkTabInStock' : 'stkTabShipped');
    if (btn) stkSwitchTab(targetTab, btn);
  }

  stkRenderTable();
  stkUpdateSummary();
  stkUpdateProgress();
  _stkUpdateMismatchBadge();

  // 入力欄をクリアしてフォーカス
  input.value = '';
  input.focus();

  _stkShowToast(`<i class="fa-solid fa-check-circle" style="color:#16a34a;"></i> 棚卸済にしました: <b>${item.brand} ${item.model}</b>`);
}

/* ============================================================
   タブ切替
   ============================================================ */
function stkSwitchTab(tab, btn) {
  _stkCurrentTab = tab;
  const tabInStock = document.getElementById('stkTabInStock');
  const tabShipped = document.getElementById('stkTabShipped');
  if (tabInStock) tabInStock.classList.toggle('active', tab === 'instock');
  if (tabShipped) tabShipped.classList.toggle('active', tab === 'shipped');
  stkRenderTable();
}

/* ============================================================
   テーブル描画
   ============================================================ */
function stkRenderTable() {
  const tbody = document.getElementById('stkTableBody');
  if (!tbody) return;

  // 現在タブのステータスで絞り込み
  const targetStatuses = _stkCurrentTab === 'instock' ? ['在庫中', '取置中', '仕入返品中'] : ['出荷済', '委託中'];

  // 取置中も物理在庫なので在庫中タブの棚卸対象に含める。
  let items = (_stkSession ? _stkSnapshotItems() : (APP_DATA.inventory || [])).filter(i => targetStatuses.includes(_stkNormalizeInventoryStatus(i.status)));

  // 棚卸済を末尾へ
  items = [
    ...items.filter(i => (_stkState[i.code]?.state || '未処理') !== '棚卸済'),
    ...items.filter(i => (_stkState[i.code]?.state || '未処理') === '棚卸済'),
  ];

  // タブカウント更新
  const snapshot = _stkSession ? _stkSnapshotItems() : (APP_DATA.inventory || []);
  const inStockCount  = snapshot.filter(i => ['在庫中', '取置中', '仕入返品中'].includes(_stkNormalizeInventoryStatus(i.status))).length;
  const shippedCount  = snapshot.filter(i => ['出荷済', '委託中'].includes(_stkNormalizeInventoryStatus(i.status))).length;
  const countInStk = document.getElementById('stkTabCountInStock');
  const countShp  = document.getElementById('stkTabCountShipped');
  if (countInStk) countInStk.textContent = inStockCount;
  if (countShp)   countShp.textContent  = shippedCount;

  if (items.length === 0) {
    tbody.innerHTML = `<tr><td colspan="13" class="stk-td" style="text-align:center;color:var(--text-muted);padding:24px;">データがありません</td></tr>`;
    return;
  }

  if (_stkCurrentTab === 'shipped' && _stkSession) {
    _stkRenderDocumentGroups(tbody, items);
    return;
  }
  tbody.innerHTML = items.map(item => _stkBuildRow(item)).join('');
}

function _stkRenderDocumentGroups(tbody, items) {
  const groups = new Map();
  items.forEach(item => {
    const line = item._stocktakeLine || {};
    const key = `${line.documentType || 'unlinked'}:${line.documentId || item.code}`;
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key).push(item);
  });
  tbody.innerHTML = [...groups.values()].map(group => {
    const line = group[0]._stocktakeLine || {};
    const typeLabel = line.documentType === 'consignment' ? '委託伝票' : (line.documentType === 'shipment' ? '出荷伝票' : '伝票未関連');
    const checked = group.filter(item => item._stocktakeLine?.documentCheckedAt).length;
    const issuedAt = line.shipmentIssuedAt ? new Date(line.shipmentIssuedAt).toLocaleString('ja-JP') : '—';
    const action = line.documentId ? `<button class="btn btn-primary btn-sm" onclick="stkConfirmDocumentItems('${_esc(line.documentType)}','${_esc(line.documentId)}')">
      <i class="fa-solid fa-check-double"></i> 選択商品を伝票確認
    </button>` : '<span class="stk-document-warning">伝票との関連を確認してください</span>';
    return `<tr class="stk-document-row"><td colspan="13">
      <div class="stk-document-header">
        <div><strong>${typeLabel} ${_esc(line.documentNumber || '—')}</strong>
          <span>起票日時：${_esc(issuedAt)}</span><span>出荷・委託先：${_esc(line.documentPartnerName || '—')}</span>
          <span>確認済み：${checked} / ${group.length}点</span>
        </div>${action}
      </div>
    </td></tr>${group.map(item => _stkBuildRow(item, true)).join('')}`;
  }).join('');
}

async function stkConfirmDocumentItems(documentType, documentId) {
  const productCodes = [...document.querySelectorAll(`#stkTableBody .stk-document-check[data-document-id="${CSS.escape(documentId)}"]:checked`)]
    .map(input => input.value);
  if (!productCodes.length) {
    _stkShowPopup('info', '商品を選択してください', '伝票に記載された現物を確認し、該当商品のチェックボックスを選択してください。');
    return;
  }
  try {
    const result = await window.ZaikoAPI.request(`/stocktakes/${encodeURIComponent(_stkSession.id)}/documents/${encodeURIComponent(documentType)}/${encodeURIComponent(documentId)}/confirm`, {
      method:'POST', body:JSON.stringify({ productCodes }),
    });
    _stkSession = result.session; _stkSyncStateFromSession();
    stkRenderTable(); stkUpdateSummary(); stkUpdateProgress(); _stkUpdateMismatchBadge();
    _stkShowToast(`<i class="fa-solid fa-check-circle" style="color:#16a34a;"></i> 伝票の商品 ${result.affected}点を確認しました。続けて店内の実在庫をスキャンしてください。`);
  } catch (error) {
    _stkShowPopup('warning', '伝票確認を保存できません', error.message || '通信状態を確認してください。');
  }
}

function _stkBuildRow(item, documentMode = false) {
  const st = _stkState[item.code] || { state: '未処理', reason: null };
  const state = st.state;

  // 行CSSクラス
  const rowClass = {
    '未処理' : '',
    '棚卸済' : 'stk-row-done',
    '不一致' : 'stk-row-mismatch',
    '承認待ち': 'stk-row-pending',
    '不明在庫': 'stk-row-unknown',
  }[state] || '';

  // バッジ
  const line = item._stocktakeLine || {};
  const documentCheck = documentMode ? `<input type="checkbox" class="stk-document-check" data-document-id="${_esc(line.documentId || '')}" value="${_esc(item.code)}" ${line.documentCheckedAt ? 'checked disabled' : ''} aria-label="${_esc(item.code)}を伝票確認"> ` : '';
  const badgeHtml = `${documentCheck}<span class="stk-state-badge stk-state-${state}">${_stkStateIcon(state)} ${state}</span>`;

  // コンディション名
  const cond = (APP_DATA.conditions || []).find(c => c.code === item.condition);
  const condName = cond ? cond.name.replace(/\s*\(.*\)/, '') : (item.condition || '—');

  const price = typeof formatPrice === 'function'
    ? formatPrice(item.purchasePrice || 0)
    : `¥${(item.purchasePrice || 0).toLocaleString('ja-JP')}`;
  const scanner = (APP_DATA.users || []).find(user => user.id === _stkSession?.startedBy)?.name || _stkSession?.startedBy || '—';

  return `<tr class="${rowClass}" data-code="${_esc(item.code)}">
    <td class="stk-td stk-td-status">${badgeHtml}</td>
    <td class="stk-td stk-td-code">${_esc(item.code)}</td>
    <td class="stk-td stk-td-inventory-date${st.inventoryDate ? ' stk-date-set' : ''}">${_esc(st.inventoryDate || '—')}</td>
    <td class="stk-td">${_esc(item._stocktakeLine?.shipmentIssuedAt ? new Date(item._stocktakeLine.shipmentIssuedAt).toLocaleString('ja-JP') : '—')}</td>
    <td class="stk-td">${_esc(scanner)}</td>
    <td class="stk-td">${_esc(item.brand || '—')}</td>
    <td class="stk-td">${_esc(item.model || '—')}</td>
    <td class="stk-td">${_esc(item.ref   || '—')}</td>
    <td class="stk-td">${_esc(item.serial|| '—')}</td>
    <td class="stk-td">${_esc(condName)}</td>
    <td class="stk-td">${_esc(item.staff || '—')}</td>
    <td class="stk-td stk-td-price">${price}</td>
    <td class="stk-td">${_esc(item.purchaseDate || '—')}</td>
  </tr>`;
}

function _stkStateIcon(state) {
  const icons = {
    '未処理' : '<i class="fa-regular fa-circle"></i>',
    '棚卸済' : '<i class="fa-solid fa-circle-check"></i>',
    '不一致' : '<i class="fa-solid fa-triangle-exclamation"></i>',
    '承認待ち': '<i class="fa-solid fa-hourglass-half"></i>',
    '不明在庫': '<i class="fa-solid fa-circle-plus"></i>',
  };
  return icons[state] || '';
}

/* ============================================================
   進捗カウンタ更新
   ============================================================ */
function stkUpdateProgress() {
  const inv = _stkSession ? _stkSnapshotItems() : (APP_DATA.inventory || []);

  const inStockItems  = inv.filter(i => ['在庫中', '取置中', '仕入返品中'].includes(_stkNormalizeInventoryStatus(i.status)));
  const shippedItems  = inv.filter(i => ['出荷済', '委託中'].includes(i.status));

  const doneInStock  = inStockItems.filter(i => _stkState[i.code]?.state === '棚卸済').length;
  const doneShipped  = shippedItems.filter(i => _stkState[i.code]?.state === '棚卸済').length;

  const elDoneIS = document.getElementById('stkDoneInStock');
  const elTotIS  = document.getElementById('stkTotalInStock');
  const elDoneSH = document.getElementById('stkDoneShipped');
  const elTotSH  = document.getElementById('stkTotalShipped');
  if (elDoneIS) elDoneIS.textContent = doneInStock;
  if (elTotIS)  elTotIS.textContent  = inStockItems.length;
  if (elDoneSH) elDoneSH.textContent = doneShipped;
  if (elTotSH)  elTotSH.textContent  = shippedItems.length;
}

/* ============================================================
   集計サマリー更新
   ============================================================ */
function stkUpdateSummary() {
  const inv = _stkSession ? _stkSnapshotItems() : (APP_DATA.inventory || []);

  const inStockItems  = inv.filter(i => ['在庫中', '取置中', '仕入返品中'].includes(_stkNormalizeInventoryStatus(i.status)));
  const shippedItems  = inv.filter(i => ['出荷済', '委託中'].includes(i.status));

  const sum = arr => arr.reduce((s, i) => s + (i.purchasePrice || 0), 0);
  const fmt = v => typeof formatPrice === 'function'
    ? formatPrice(v)
    : `¥${v.toLocaleString('ja-JP')}`;

  // リスト合計
  const totInStock  = sum(inStockItems);
  const totShipped  = sum(shippedItems);

  // 棚卸済合計
  const doneInStock = sum(inStockItems.filter(i => _stkState[i.code]?.state === '棚卸済'));
  const doneShipped = sum(shippedItems.filter(i => _stkState[i.code]?.state === '棚卸済'));

  // 不一致（不一致・承認待ち）
  const mismatchItems = inv.filter(i => ['不一致','承認待ち'].includes(_stkState[i.code]?.state));
  const mismatchTotal = sum(mismatchItems);

  const grandTotal    = totInStock + totShipped;
  const doneTotal     = doneInStock + doneShipped;

  _el('sumInStockTotal', fmt(totInStock));
  _el('sumInStockDone',  fmt(doneInStock));
  _el('sumShippedTotal', fmt(totShipped));
  _el('sumShippedDone',  fmt(doneShipped));
  _el('sumGrandTotal',   fmt(grandTotal));
  _el('sumMismatch',     fmt(mismatchTotal));
  _el('sumDoneTotal',    fmt(doneTotal));

  // チェック行（全件処理後に表示）
  const allProcessed = inv.every(i => _stkState[i.code]?.state !== '未処理');
  const checkRow = document.getElementById('sumCheckRow');
  if (checkRow) {
    checkRow.style.display = allProcessed ? '' : 'none';
    if (allProcessed) {
      const ok = Math.abs(grandTotal - (mismatchTotal + doneTotal)) < 1;
      const el = document.getElementById('sumCheckResult');
      if (el) {
        el.textContent = ok ? '✓ 一致' : '✗ 不一致';
        el.style.color = ok ? '#16a34a' : '#dc2626';
      }
    }
  }

  // 不一致バッジも更新
  _stkUpdateMismatchBadge();
}

/* ============================================================
   一時保存
   ============================================================ */
async function stkTempSave() {
  if (_stkSession && window.ZaikoAPI?.request) {
    try {
      const lines = Object.values(_stkState).filter(state => state.lineId).map(state => ({ id:state.lineId, reason:state.reason || '', note:state.note || '' }));
      const result = await window.ZaikoAPI.request(`/stocktakes/${encodeURIComponent(_stkSession.id)}`, { method:'PATCH', body:JSON.stringify({ lines }) });
      _stkSession = result.session; _stkSyncStateFromSession();
    } catch (error) { if (typeof showToast === 'function') showToast('error', '保存できませんでした', error.message); return; }
  }
  _stkSaved = true;
  _stkUpdateBadge('saved', '途中保存');
  if (typeof showToast === 'function') {
    showToast('success', '一時保存しました', '棚卸の途中状態を保存しました。');
  } else {
    _stkShowToast('<i class="fa-solid fa-floppy-disk" style="color:#16a34a;"></i> 一時保存しました');
  }
}

/* ============================================================
   不一致リストモーダル
   ============================================================ */
function stkOpenMismatch() {
  const modal = document.getElementById('stkMismatchModal');
  if (!modal) return;
  modal.classList.remove('hidden');
  _stkRenderMismatchTable();
}

function stkCloseMismatch() {
  const modal = document.getElementById('stkMismatchModal');
  if (modal) modal.classList.add('hidden');
}

function _stkRenderMismatchTable() {
  const tbody = document.getElementById('stkMismatchBody');
  if (!tbody) return;

  const expected = _stkSession ? _stkSnapshotItems() : (APP_DATA.inventory || []);
  const unknown = _stkSession ? _stkUnknownItems() : Object.values(_stkState).filter(s => s.lineType === 'unknown_inventory').map(s => ({ code:s.code, brand:'未登録', model:'—', ref:'—', purchasePrice:0, status:'対象外' }));
  // 管理者かどうかをここで確定する（描画中に変わらないよう1回だけ評価）
  const admin = typeof isAdmin === 'function' && isAdmin();

  // 棚卸済以外を表示（未処理・不一致・承認待ち）
  const unprocessed = expected.filter(i => {
    const s = _stkState[i.code]?.state || '未処理';
    return s !== '棚卸済' && !i._stocktakeLine?.resolvedAt;
  });
  const visibleUnknown = unknown.filter(i => !i._stocktakeLine?.resolvedAt);

  if (unprocessed.length === 0 && visibleUnknown.length === 0) {
    tbody.innerHTML = `<tr><td colspan="8" class="stk-td" style="text-align:center;color:var(--text-muted);padding:20px;">未処理の商品はありません ✓</td></tr>`;
    return;
  }

  const fmt = v => typeof formatPrice === 'function'
    ? formatPrice(v) : `¥${v.toLocaleString('ja-JP')}`;

  const rows = [
    ...unprocessed.map(item => ({ item, kind:'expected' })),
    ...visibleUnknown.map(item => ({ item, kind:'unknown' })),
  ];
  tbody.innerHTML = rows.map(({item, kind}) => {
    const st = _stkState[item.code] || { state: '未処理' };
    const classification = kind === 'unknown' ? '無いはずなのにある' : 'あるはずなのに無い';
    const badge = `<span class="stk-state-badge stk-state-${st.state}">${_stkStateIcon(st.state)} ${classification}${st.reason && st.reason !== '不明在庫' ? '・' + st.reason : ''}</span>`;

    let btns;
    if (kind === 'unknown') {
      btns = `<span style="font-size:12px;color:#be123c;font-weight:700;"><i class="fa-solid fa-circle-plus"></i> 不明在庫</span>`;
    } else if (st.state === '不一致') {
      // 確定済み（管理者即時確定 or 承認済み）→ 操作不要
      btns = `<span style="font-size:11px;color:#16a34a;font-weight:600;">
                <i class="fa-solid fa-circle-check"></i> 確定済み
              </span>`;
    } else if (st.state === '承認待ち' && !admin) {
      // 作業者の承認待ち → 再操作不可
      btns = `<span style="font-size:11px;color:#1d4ed8;font-weight:600;">
                <i class="fa-solid fa-hourglass-half"></i> 承認待ち
              </span>`;
    } else {
      // 未処理、または管理者が承認待ちを上書きできる状態
      btns = `<div class="stk-mismatch-btn-row">
          <button class="stk-mismatch-action-btn" onclick="stkOpenReason('${_esc(item.code)}','紛失')">紛失</button>
          <button class="stk-mismatch-action-btn" onclick="stkOpenReason('${_esc(item.code)}','返品忘れ')">返品忘れ</button>
          <button class="stk-mismatch-action-btn" onclick="stkOpenReason('${_esc(item.code)}','破損')">破損</button>
          <button class="stk-mismatch-action-btn" onclick="stkOpenReason('${_esc(item.code)}','理由不明')">理由不明</button>
        </div>`;
    }

    return `<tr class="${kind === 'unknown' ? 'stk-row-unknown' : ''}">
      <td class="stk-td" style="text-align:center"><input type="checkbox" class="stk-resolve-check" value="${_esc(st.lineId || '')}" aria-label="${_esc(item.code)}を確定"></td>
      <td class="stk-td stk-td-code">${_esc(item.code)}</td>
      <td class="stk-td">${_esc(item.brand || '—')}</td>
      <td class="stk-td">${_esc(item.model || '—')}</td>
      <td class="stk-td">${_esc(item.ref   || '—')}</td>
      <td class="stk-td stk-td-price">${fmt(item.purchasePrice || 0)}</td>
      <td class="stk-td stk-td-status">${badge}</td>
      <td class="stk-td">${btns}</td>
    </tr>`;
  }).join('');
}

async function stkResolveSelectedMismatches() {
  const resolvedIds = [...document.querySelectorAll('.stk-resolve-check:checked')].map(el => el.value).filter(Boolean);
  if (!resolvedIds.length) { showToast('info', '不一致確定', '確定する項目を選択してください。'); return; }
  const result = await window.ZaikoAPI.request(`/stocktakes/${encodeURIComponent(_stkSession.id)}`, {
    method: 'PATCH', body: JSON.stringify({ lines: [], resolvedIds }),
  });
  _stkSession = result.session; _stkSyncStateFromSession(); _stkRenderMismatchTable(); _stkUpdateMismatchBadge();
  showToast('success', '不一致を確定しました', `${resolvedIds.length}件を不一致リストから除外しました。`);
}

function _stkUpdateMismatchBadge() {
  const inv = _stkSession ? _stkSnapshotItems() : (APP_DATA.inventory || []);
  const count = inv.filter(i => {
    const s = _stkState[i.code]?.state || '未処理';
    return s === '未処理';
  }).length + (_stkSession ? _stkUnknownItems().length : Object.values(_stkState).filter(s => s.lineType === 'unknown_inventory').length);
  const badge = document.getElementById('stkMismatchBadge');
  if (!badge) return;
  badge.textContent = count;
  badge.style.display = count > 0 ? 'flex' : 'none';
}

/* ============================================================
   理由入力モーダル
   ============================================================ */
function stkOpenReason(code, presetReason) {
  // 不一致リストモーダルを一時的に隠す（Zインデックス制御）
  const mismatchModal = document.getElementById('stkMismatchModal');
  if (mismatchModal) mismatchModal.style.zIndex = '1100';

  _stkReasonTargetCode = code;
  _stkSelectedReason   = presetReason || null;

  const item = (_stkSession ? [..._stkSnapshotItems(), ..._stkUnknownItems()] : (APP_DATA.inventory || [])).find(i => i.code === code);
  if (!item) return;

  // 商品詳細カード
  const fmt = v => typeof formatPrice === 'function'
    ? formatPrice(v) : `¥${v.toLocaleString('ja-JP')}`;

  const detailEl = document.getElementById('stkReasonItemDetail');
  if (detailEl) {
    detailEl.innerHTML = `
      <div class="stk-ric-row"><span class="stk-ric-label">商品コード</span><span class="stk-ric-val">${_esc(item.code)}</span></div>
      <div class="stk-ric-row"><span class="stk-ric-label">ブランド</span><span class="stk-ric-val">${_esc(item.brand || '—')}</span></div>
      <div class="stk-ric-row"><span class="stk-ric-label">モデル名</span><span class="stk-ric-val">${_esc(item.model || '—')}</span></div>
      <div class="stk-ric-row"><span class="stk-ric-label">型番</span><span class="stk-ric-val">${_esc(item.ref || '—')}</span></div>
      <div class="stk-ric-row"><span class="stk-ric-label">シリアル</span><span class="stk-ric-val">${_esc(item.serial || '—')}</span></div>
      <div class="stk-ric-row"><span class="stk-ric-label">仕入金額</span><span class="stk-ric-val" style="color:var(--primary);">${fmt(item.purchasePrice || 0)}</span></div>
      <div class="stk-ric-row"><span class="stk-ric-label">ステータス</span><span class="stk-ric-val">${_esc(_stkNormalizeInventoryStatus(item.status) || '—')}</span></div>`;
  }

  // 既存の理由・備考をセット
  const existingSt = _stkState[code] || {};
  const noteEl = document.getElementById('stkReasonNote');
  if (noteEl) noteEl.value = existingSt.note || '';

  // 理由ボタンのアクティブ状態を初期化
  _stkSelectReasonUI(presetReason || existingSt.reason || null);

  // ④ ロールに応じてボタンラベルを切り替え
  const confirmBtn = document.getElementById('stkReasonConfirmBtn');
  if (confirmBtn) {
    const admin = typeof isAdmin === 'function' && isAdmin();
    if (admin) {
      // 管理者：承認プロセスなしで即時確定
      confirmBtn.innerHTML = '<i class="fa-solid fa-check"></i> 確定（即時処理済み）';
    } else {
      // 作業者：確定後に管理者へ承認申請を送信
      confirmBtn.innerHTML = '<i class="fa-solid fa-paper-plane"></i> 確定・承認申請を送信';
    }
  }

  const modal = document.getElementById('stkReasonModal');
  if (modal) modal.classList.remove('hidden');
}

function stkCloseReason() {
  const modal = document.getElementById('stkReasonModal');
  if (modal) modal.classList.add('hidden');
  // 不一致リストモーダルのz-indexを戻す
  const mismatchModal = document.getElementById('stkMismatchModal');
  if (mismatchModal) mismatchModal.style.zIndex = '1200';
  _stkReasonTargetCode = null;
  _stkSelectedReason   = null;
}

function stkSelectReason(reason) {
  _stkSelectedReason = reason;
  _stkSelectReasonUI(reason);
}

function _stkSelectReasonUI(reason) {
  document.querySelectorAll('#stkReasonBtnRow .stk-reason-btn').forEach(btn => {
    btn.classList.toggle('active', btn.dataset.reason === reason);
  });
}

async function stkConfirmReason() {
  if (!_stkReasonTargetCode) return;
  if (!_stkSelectedReason) {
    _stkShowPopup('warning', '理由を選択してください', '不一致理由を4つの選択肢から選んでください。');
    return;
  }

  const noteEl = document.getElementById('stkReasonNote');
  const note = noteEl ? noteEl.value.trim() : '';

  const code = _stkReasonTargetCode;
  const item = (_stkSession ? [..._stkSnapshotItems(), ..._stkUnknownItems()] : (APP_DATA.inventory || [])).find(i => i.code === code);

  // ④ 権限判定：isAdmin() が true なら管理者、それ以外は作業者扱い
  const admin = typeof isAdmin === 'function' && isAdmin();

  if (admin) {
    // ── 管理者：承認プロセスをスキップして即時「不一致（処理済み）」に確定 ──
    _stkState[code] = {
      ..._stkState[code],
      state: '不一致',
      reason: _stkSelectedReason,
      note,
    };
    if (typeof showToast === 'function') {
      showToast('success', '不一致を確定しました（管理者）',
        `${_stkSelectedReason} の理由で即時確定しました。`);
    }
  } else {
    // ── 作業者：承認待ちに設定し管理者へ承認申請を送信 ──
    _stkState[code] = {
      ..._stkState[code],
      state: '承認待ち',
      reason: _stkSelectedReason,
      note,
    };
    if (typeof requestApproval === 'function') {
      requestApproval(
        'stocktake_mismatch',
        '棚卸不一致',
        {
          code,
          brand:       item?.brand  || '—',
          model:       item?.model  || '—',
          reason:      _stkSelectedReason,
          note,
          status:      item?.status || '—',
          price:       item?.purchasePrice || 0,
          requestedBy: typeof currentUser === 'function' ? (currentUser()?.name || '—') : '—',
        },
        `商品 ${code}（${item?.brand || ''} ${item?.model || ''}）の不一致理由: ${_stkSelectedReason}`,
        null
      );
    }
    if (typeof showToast === 'function') {
      showToast('success', '承認申請を送信しました（作業者）',
        `${_stkSelectedReason} の理由で記録し、管理者へ承認申請を送信しました。`);
    }
  }

  if (_stkSession && window.ZaikoAPI?.request) {
    try {
      const state = _stkState[code];
      const result = await window.ZaikoAPI.request(`/stocktakes/${encodeURIComponent(_stkSession.id)}`, {
        method:'PATCH', body:JSON.stringify({ lines:[{ id:state.lineId, reason:_stkSelectedReason, note }] })
      });
      _stkSession = result.session; _stkSyncStateFromSession();
    } catch (error) { if (typeof showToast === 'function') showToast('error', '不一致を保存できませんでした', error.message); return; }
  }

  stkCloseReason();
  stkRenderTable();
  stkUpdateSummary();
  stkUpdateProgress();
  _stkUpdateMismatchBadge();
  // 不一致リストを再描画
  _stkRenderMismatchTable();
}

/* ============================================================
   棚卸確定
   ============================================================ */
function stkTryComplete() {
  const inv = _stkSession ? _stkSnapshotItems() : (APP_DATA.inventory || []);
  const fmt = v => typeof formatPrice === 'function'
    ? formatPrice(v) : `¥${v.toLocaleString('ja-JP')}`;

  // 条件① 未処理（承認待ちでもない）の商品がないこと
  const unprocessed = inv.filter(i => {
    const s = _stkState[i.code]?.state || '未処理';
    return s === '未処理';
  });
  if (unprocessed.length > 0) {
    if (typeof showToast === 'function') {
      showToast('error', '棚卸を確定できません', `未処理の商品が ${unprocessed.length} 件あります。「不一致リスト」から理由を記録してください。`);
    }
    return;
  }

  // 条件② 金額チェック
  const sum = arr => arr.reduce((s, i) => s + (i.purchasePrice || 0), 0);
  const grandTotal    = sum(inv);
  const mismatchTotal = sum(inv.filter(i => ['不一致','承認待ち'].includes(_stkState[i.code]?.state)));
  const doneTotal     = sum(inv.filter(i => _stkState[i.code]?.state === '棚卸済'));
  const diff = Math.abs(grandTotal - (mismatchTotal + doneTotal));

  if (diff >= 1) {
    if (typeof showToast === 'function') {
      showToast('error', '棚卸を確定できません',
        `金額が一致していません。システム合計 ${fmt(grandTotal)} ≠ 不一致 ${fmt(mismatchTotal)} + 棚卸済 ${fmt(doneTotal)}`);
    }
    return;
  }

  // 最終集計表示
  const statsEl = document.getElementById('stkCompleteStats');
  if (statsEl) {
    statsEl.innerHTML = [
      { label: 'システム合計（在庫中・取置中・出荷済・委託中・仕入返品中）', val: fmt(grandTotal) },
      { label: '棚卸済合計',                    val: fmt(doneTotal) },
      { label: '不一致合計',                    val: fmt(mismatchTotal) },
      { label: 'チェック',                      val: diff < 1 ? '✓ 一致' : '✗ 不一致' },
    ].map(r => `
      <div class="stk-complete-stat-row">
        <span class="label">${r.label}</span>
        <span class="val">${r.val}</span>
      </div>`).join('');
  }

  const modal = document.getElementById('stkCompleteModal');
  if (modal) modal.classList.remove('hidden');
}

function stkCloseComplete() {
  const modal = document.getElementById('stkCompleteModal');
  if (modal) modal.classList.add('hidden');
}

async function stkExecuteComplete() {
  if (_stkSession && window.ZaikoAPI?.request) {
    try {
      const result = await window.ZaikoAPI.request(`/stocktakes/${encodeURIComponent(_stkSession.id)}/complete`, { method:'POST', body:'{}' });
      _stkSession = result.session; _stkSyncStateFromSession();
    } catch (error) { if (typeof showToast === 'function') showToast('error', '棚卸を確定できません', error.message); return; }
  }
  // ③ 棚卸実施日：確定ボタン押下時の現在日付（YYYY-MM-DD）
  const todayDate = _today();

  // 全対象明細に同一日付を設定（未処理・棚卸済・不一致・承認待ち 全て対象）
  Object.keys(_stkState).forEach(code => {
    _stkState[code].inventoryDate = todayDate;
  });

  _stkCompleted = true;
  _stkSaved     = true;
  stkCloseComplete();
  _stkUpdateBadge('complete', '完了');

  // リアルタイム再描画（棚卸実施日カラムを即時反映）
  stkRenderTable();
  stkUpdateSummary();

  if (typeof showToast === 'function') {
    showToast('success', '棚卸が完了しました',
      `${new Date().toLocaleString('ja-JP')} 棚卸を確定しました。棚卸実施日：${todayDate}`);
  }
}

/* ============================================================
   CSV ダウンロード
   ============================================================ */
function stkDownloadCSV() {
  const inv = _stkSession ? [..._stkSnapshotItems(), ..._stkUnknownItems()] : (APP_DATA.inventory || []);
  const fmt = v => v || '';

  const headers = ['管理番号','ブランド','モデル','型番','シリアル','原価','ステータス','棚卸状態','不一致理由','備考','棚卸日時'];
  const rows = inv.map(item => {
    const st = _stkState[item.code] || { state: '未処理' };
    return [
      item.code, item.brand, item.model, item.ref, item.serial,
      item.purchasePrice || 0,
      item.status,
      st.state === '不明在庫' ? '無いはずなのにある（不明在庫）' : (st.state === '棚卸済' ? '実物確認済み' : 'あるはずなのに無い'),
      st.reason  || '',
      st.note    || '',
      st.checkedAt || '',
    ].map(v => `"${String(v).replace(/"/g,'""')}"`);
  });

  const bom  = '\uFEFF';
  const csv  = bom + [headers, ...rows].map(r => r.join(',')).join('\r\n');
  const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' });
  const url  = URL.createObjectURL(blob);
  const a    = document.createElement('a');
  a.href     = url;
  a.download = `棚卸_${_today()}.csv`;
  a.click();
  URL.revokeObjectURL(url);

  if (typeof showToast === 'function') {
    showToast('success', 'CSVダウンロード', `棚卸データ ${inv.length} 件を出力しました。`);
  }
}

/* ============================================================
   印刷
   ============================================================ */
function stkPrint() {
  const inv = _stkSession ? [..._stkSnapshotItems(), ..._stkUnknownItems()] : (APP_DATA.inventory || []);
  const fmt = v => typeof formatPrice === 'function'
    ? formatPrice(v) : `¥${v.toLocaleString('ja-JP')}`;

  const rows = inv.map(item => {
    const st = _stkState[item.code] || { state: '未処理' };
    const stateColor = {
      '未処理' : '#64748b',
      '棚卸済' : '#16a34a',
      '不一致' : '#c2410c',
      '承認待ち': '#1d4ed8',
    }[st.state] || '#333';
    return `<tr>
      <td>${_esc(item.code)}</td>
      <td>${_esc(item.brand || '')}</td>
      <td>${_esc(item.model || '')}</td>
      <td>${_esc(item.ref   || '')}</td>
      <td>${_esc(item.serial|| '')}</td>
      <td style="text-align:right;">${fmt(item.purchasePrice || 0)}</td>
      <td>${_esc(_stkNormalizeInventoryStatus(item.status) || '')}</td>
      <td style="color:${stateColor};font-weight:bold;">${st.state}${st.reason ? '（' + st.reason + '）' : ''}</td>
    </tr>`;
  }).join('');

  const html = `<!DOCTYPE html><html lang="ja"><head><meta charset="UTF-8">
    <title>棚卸表 ${_today()}</title>
    <style>
      body { font-family: sans-serif; font-size: 11px; margin: 20px; }
      h2 { font-size: 15px; margin-bottom: 8px; }
      table { border-collapse: collapse; width: 100%; }
      th, td { border: 1px solid #ccc; padding: 4px 6px; }
      th { background: #f0f4f8; font-size: 10px; }
      @media print { button { display: none; } }
    </style></head><body>
    <h2>棚卸表　${_today()}</h2>
    <table>
      <thead><tr><th>商品コード</th><th>ブランド</th><th>モデル名</th><th>型番</th><th>シリアル</th><th>仕入金額</th><th>ステータス</th><th>棚卸状態</th></tr></thead>
      <tbody>${rows}</tbody>
    </table>
    <script>window.onload = function(){ window.print(); }<\/script>
    </body></html>`;

  const w = window.open('', '_blank');
  if (w) { w.document.write(html); w.document.close(); }
}

/* ============================================================
   ユーティリティ
   ============================================================ */
function _stkUpdateBadge(type, label) {
  const el = document.getElementById('stkStatusBadge');
  if (!el) return;
  el.className = 'stk-session-badge stk-session-' + type;
  el.textContent = label;
}

/** 簡易トースト（showToastが呼べない場合の fallback） */
function _stkShowToast(html) {
  if (typeof showToast === 'function') return; // showToastがあれば呼び元で使う
  const div = document.createElement('div');
  div.innerHTML = html;
  div.style.cssText = `
    position:fixed;bottom:80px;right:24px;z-index:9999;
    background:#fff;border:1px solid #e5e7eb;border-radius:8px;
    padding:10px 16px;box-shadow:0 4px 16px rgba(0,0,0,.15);
    font-size:13px;max-width:320px;`;
  document.body.appendChild(div);
  setTimeout(() => div.remove(), 2600);
}

/** ポップアップ（alert代替） */
function _stkShowPopup(type, title, msg) {
  if (typeof showToast === 'function') {
    showToast(type === 'warning' ? 'warning' : type, title, msg);
  } else {
    alert(`${title}\n${msg}`);
  }
}

/** innerHTML用エスケープ */
function _esc(str) {
  return String(str)
    .replace(/&/g,'&amp;')
    .replace(/</g,'&lt;')
    .replace(/>/g,'&gt;')
    .replace(/"/g,'&quot;')
    .replace(/'/g,'&#39;');
}

/** 要素テキスト更新 */
function _el(id, text) {
  const el = document.getElementById(id);
  if (el) el.textContent = text;
}

/** YYYY-MM-DD */
function _today() {
  return new Date().toISOString().slice(0, 10);
}
