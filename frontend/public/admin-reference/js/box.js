// =====================================================
// box.js — BOX管理ロジック（番号 1〜10 固定制）
// =====================================================
// データ構造:
//   APP_DATA.boxes[n-1] = {
//     no: Number,          // 1〜10
//     name: String,        // BOX名（任意）
//     publicTo: [buyerCode, ...],  // 公開先企業コードの配列
//     createdAt: String
//   }
//   APP_DATA.inventory[i].boxNo = Number | null  // BOX番号

const BOX_COUNT = 10;

// 現在フォーカス中のBOX番号（名前編集エリア・編集モーダル共用）
let _activeBoxNo = null;

// =====================================================
// ゲスト公開スナップショット
// =====================================================

/**
 * 現在の boxes / inventory の状態を publishedSnapshot に焼き付ける。
 * ゲスト画面はこのスナップショットだけを参照する。追加・編集は公開更新時に反映し、
 * 出荷済・売上済の商品は事故防止のため確定時に自動で除外する。
 */
async function publishGuestSnapshot() {
  if (window.ZaikoAPI) {
    try {
      await window.ZaikoAPI.saveBoxes(APP_DATA.boxes || []);
      const liveBoxes = (APP_DATA.boxes || []).filter(box => (box.publicTo || []).length > 0);
      const totalItems = liveBoxes.reduce((count, box) => count + (box.productCodes || []).length, 0);
      _refreshPublishStatus();
      renderBoxMatrix();
      showToast('success', 'ゲスト公開情報を更新しました', `${liveBoxes.length}件のBOX / ${totalItems}点の商品をDBへ反映`);
    } catch (error) {
      showToast('error', 'ゲスト公開情報を更新できませんでした', error.message || '入力内容を確認してください');
    }
    return;
  }
  const now = new Date();
  const ts  = `${now.getFullYear()}/${String(now.getMonth()+1).padStart(2,'0')}/${String(now.getDate()).padStart(2,'0')} ${String(now.getHours()).padStart(2,'0')}:${String(now.getMinutes()).padStart(2,'0')}`;

  // 公開先が1社以上あるBOXのみスナップショットに含める
  const snapshotBoxes = APP_DATA.boxes
    .filter(box => (box.publicTo || []).length > 0)
    .map(box => {
      const items = APP_DATA.inventory
        .filter(item => item.boxNo === box.no && item.status === '在庫中')
        .map(item => ({
          code:       item.code,
          brand:      item.brand,
          brandEn:    item.brandEn    || '',
          model:      item.model,
          modelEn:    item.modelEn    || '',
          ref:        item.ref        || '',
          salePrice:  item.salePrice  || 0,
          condition:  item.condition  || '',
          status:     item.status,
          images:     item.images     || [],
          boxNo:      item.boxNo,
        }));
      return {
        no:       box.no,
        name:     box.name    || '',
        publicTo: [...(box.publicTo || [])],
        items,
      };
    });

  APP_DATA.publishedSnapshot = {
    updatedAt: ts,
    boxes: snapshotBoxes,
  };
  if (typeof persistGuestSnapshot === 'function') persistGuestSnapshot();
  if (typeof persistGuestBoxState === 'function') persistGuestBoxState();

  // ページ上部の「最終更新日時」表示を更新
  _refreshPublishStatus();

  const totalItems = snapshotBoxes.reduce((s, b) => s + b.items.length, 0);
  showToast('success',
    'ゲスト公開情報を更新しました',
    `${snapshotBoxes.length}件のBOX / ${totalItems}点の商品を公開`
  );
}

/** ページ上部のスナップショット状態表示を更新 */
function _refreshPublishStatus() {
  const liveBoxes = window.ZaikoAPI
    ? (APP_DATA.boxes || []).filter(box => box.active !== false && (box.publicTo || []).length > 0)
    : [];
  const snap = window.ZaikoAPI
    ? {
        updatedAt: liveBoxes.map(box => box.createdAt).filter(Boolean).sort().at(-1) || 'DB連動中',
        boxes: liveBoxes.map(box => ({ ...box, items: (box.productCodes || []).map(code => ({ code })) })),
      }
    : APP_DATA.publishedSnapshot;
  const elements = ['guestPublishStatus', 'masterGuestPublishStatus']
    .map(id => document.getElementById(id))
    .filter(Boolean);
  if (elements.length === 0) return;

  elements.forEach(el => {
    if (!snap?.updatedAt) {
      el.innerHTML = `<span style="color:var(--danger);"><i class="fa-solid fa-circle-exclamation"></i> 未公開（まだ一度も更新されていません）</span>`;
      return;
    }
    const boxCount  = snap.boxes.length;
    const itemCount = snap.boxes.reduce((s, b) => s + b.items.length, 0);
    el.innerHTML = `
      <span style="color:#059669;">
        <i class="fa-solid fa-circle-check"></i>
        最終更新: <strong>${snap.updatedAt}</strong>
      </span>
      <span style="color:var(--text-muted);font-size:11px;margin-left:10px;">
        公開中: BOX ${boxCount}件 / 商品 ${itemCount}点
      </span>`;
  });
}

// =====================================================
// メイン描画: BOX管理マトリクス
// =====================================================

function renderBoxMatrix() {
  renderBoxMatrixNumbers('boxMatrixNumbers');
  renderBoxMatrixBody('boxMatrixBody', 'box');
  renderBoxMatrixNumbers('masterBoxMatrixNumbers');
  renderBoxMatrixBody('masterBoxMatrixBody', 'master-box');
  _refreshPublishStatus();   // ページ上部のスナップショット状態を同期
}

// ── 上部のBOX番号行を描画 ──
function renderBoxMatrixNumbers(targetId = 'boxMatrixNumbers') {
  const row = document.getElementById(targetId);
  if (!row) return;

  row.innerHTML = Array.from({ length: BOX_COUNT }, (_, i) => {
    const no    = i + 1;
    const box   = APP_DATA.boxes[i];
    const count = APP_DATA.inventory.filter(item => item.boxNo === no).length;
    const hasName = box?.name?.trim();
    const tip   = hasName ? `BOX${no}：${box.name}（${count}点）` : `BOX${no}（${count}点）`;

    return `
      <div class="box-num-cell" title="${tip}">
        <button class="box-num-btn${count > 0 ? ' has-items' : ''}"
          onclick="openBoxLineupModal(${no})"
          title="${tip}">
          ${no}
        </button>
        ${count > 0 ? `<span class="box-num-badge">${count}</span>` : ''}
      </div>`;
  }).join('');
}

// ── 企業行を描画 ──
function renderBoxMatrixBody(targetId = 'boxMatrixBody', rowPrefix = 'box') {
  const body = document.getElementById(targetId);
  if (!body) return;

  const buyers = typeof getGuestManagedBuyers === 'function'
    ? getGuestManagedBuyers()
    : (APP_DATA.buyers || []).filter(buyer =>
        (APP_DATA.guestAccounts || []).some(guest => guest.buyerCode === buyer.code));
  if (buyers.length === 0) {
    body.innerHTML = `
      <div style="padding:32px;text-align:center;color:var(--text-muted);">
        <i class="fa-solid fa-building" style="font-size:28px;opacity:0.25;display:block;margin-bottom:8px;"></i>
        ゲストアカウントが登録されていません
      </div>`;
    return;
  }

  body.innerHTML = buyers.map((buyer, bi) => {
    const checkboxes = Array.from({ length: BOX_COUNT }, (_, i) => {
      const no  = i + 1;
      const box = APP_DATA.boxes[i];
      const checked = (box?.publicTo || []).includes(buyer.code);
      const hasItems = APP_DATA.inventory.some(item => item.boxNo === no);

      return `
        <div class="box-matrix-cell">
          <label class="box-check-label${!hasItems ? ' no-items' : ''}"
            title="${hasItems ? '' : 'このBOXに商品がありません'}">
            <input type="checkbox"
              class="box-check-input"
              data-box-no="${no}"
              data-buyer="${buyer.code}"
              ${checked ? 'checked' : ''}
              ${!hasItems ? 'disabled' : ''}
              onchange="onBoxPublicChange(this)">
            <span class="box-check-mark${checked ? ' checked' : ''}${!hasItems ? ' disabled' : ''}"></span>
          </label>
        </div>`;
    }).join('');

    return `
      <div class="box-matrix-row${bi % 2 === 1 ? ' alt' : ''}" id="${rowPrefix}-row-${buyer.code}">
        <div class="box-matrix-company-cell">
          <div class="box-company-name">
            <i class="fa-solid fa-building" style="color:var(--primary-light);font-size:12px;"></i>
            <span>${buyer.name}</span>
          </div>
          <div class="box-company-code">${buyer.code}</div>
        </div>
        <div class="box-matrix-numbers">
          <div class="box-matrix-check-row">${checkboxes}</div>
        </div>
      </div>`;
  }).join('');
}

// ── チェック変更時 ──
async function onBoxPublicChange(checkbox) {
  const no        = parseInt(checkbox.dataset.boxNo);
  const buyerCode = checkbox.dataset.buyer;
  const box       = APP_DATA.boxes[no - 1];
  if (!box) return;

  if (checkbox.checked) {
    if (!box.publicTo.includes(buyerCode)) box.publicTo.push(buyerCode);
  } else {
    box.publicTo = box.publicTo.filter(c => c !== buyerCode);
  }
  if (typeof persistGuestBoxState === 'function') persistGuestBoxState();

  if (window.ZaikoAPI) {
    try {
      await window.ZaikoAPI.saveBoxes([box]);
    } catch (error) {
      await window.ZaikoAPI.hydrateAdmin().catch(() => {});
      renderBoxMatrix();
      showToast('error', 'BOX公開設定を保存できませんでした', error.message || '入力内容を確認してください');
      return;
    }
  }

  // チェックマークのスタイルを更新
  const mark = checkbox.nextElementSibling;
  if (mark) mark.classList.toggle('checked', checkbox.checked);
  renderBoxMatrix();

  const buyer = APP_DATA.buyers.find(b => b.code === buyerCode);
  showToast(
    'success',
    checkbox.checked ? 'BOXを公開しました' : 'BOXを非公開にしました',
    `BOX${no} → ${buyer?.name || buyerCode}`
  );
}

// =====================================================
// BOX番号クリック：商品ラインナップモーダル
// =====================================================

function openBoxLineupModal(no) {
  const modal = document.getElementById('boxLineupModalOverlay');
  if (!modal) return;

  const box   = APP_DATA.boxes[no - 1];
  const items = APP_DATA.inventory.filter(item => item.boxNo === no);

  const nameStr = box?.name?.trim() ? `「${box.name}」` : '';
  document.getElementById('boxLineupTitle').textContent =
    `BOX ${no} ${nameStr} — 商品ラインナップ`;

  const buyers   = APP_DATA.buyers || [];
  const publicTo = box?.publicTo || [];
  const badgeHtml = publicTo.length > 0
    ? publicTo.map(code => {
        const b = buyers.find(x => x.code === code);
        return `<span class="box-public-badge">${b?.name || code}</span>`;
      }).join('')
    : `<span style="font-size:12px;color:var(--text-muted);">（全社非公開）</span>`;

  const editBtn = `
    <button class="btn btn-warning btn-sm" onclick="openBoxEditModal(${no});closeBoxLineupModal();"
      style="margin-left:auto;">
      <i class="fa-solid fa-pen"></i> 商品を編集
    </button>`;

  const headerHtml = `
    <div style="padding:12px 20px 10px;display:flex;align-items:center;gap:8px;flex-wrap:wrap;border-bottom:1px solid var(--border);">
      <span style="font-size:12px;font-weight:bold;color:var(--text-muted);">公開先：</span>
      ${badgeHtml}
      ${editBtn}
    </div>`;

  if (items.length === 0) {
    document.getElementById('boxLineupBody').innerHTML = `
      ${headerHtml}
      <div style="padding:48px 20px;text-align:center;color:var(--text-muted);">
        <i class="fa-solid fa-box-open" style="font-size:36px;opacity:0.2;display:block;margin-bottom:12px;"></i>
        このBOXに商品はまだありません
      </div>`;
  } else {
    const rows = items.map(item => `
      <tr>
        <td><code style="font-size:11px;">${item.code}</code></td>
        <td><b>${item.brand}</b></td>
        <td>${item.model}</td>
        <td style="font-size:11px;color:var(--text-muted);">${item.ref || '—'}</td>
        <td style="text-align:right;">${formatSalePrice(item.salePrice)}</td>
        <td>${getStatusBadge(typeof normalizeInventoryStatusLabel === 'function' ? normalizeInventoryStatusLabel(item.status) : item.status)}</td>
      </tr>`).join('');

    document.getElementById('boxLineupBody').innerHTML = `
      ${headerHtml}
      <div class="data-table-wrapper" style="max-height:50vh;overflow-y:auto;">
        <table class="data-table">
          <thead><tr>
            <th>商品コード</th><th>ブランド</th><th>モデル</th>
            <th>Ref</th><th style="text-align:right;">販売価格（USD）</th><th>ステータス</th>
          </tr></thead>
          <tbody>${rows}</tbody>
        </table>
      </div>
      <div style="padding:10px 20px;text-align:right;font-size:12px;color:var(--text-muted);">
        計 <b>${items.length}</b> 点
      </div>`;
  }

  modal.classList.remove('hidden');
}

function closeBoxLineupModal() {
  document.getElementById('boxLineupModalOverlay')?.classList.add('hidden');
}

// =====================================================
// BOX編集モーダル（商品追加・削除）
// =====================================================

let _boxEditFilteredItems = [];  // フィルタ後の候補リスト

function openBoxEditModal(no) {
  _activeBoxNo = no;
  const box    = APP_DATA.boxes[no - 1];
  const modal  = document.getElementById('boxEditModalOverlay');
  if (!modal) return;

  // ヘッダー
  document.getElementById('boxEditNo').textContent   = no;
  document.getElementById('boxEditName').textContent = box?.name?.trim()
    ? `「${box.name}」` : '（BOX名未設定）';

  // ブランドフィルタを初期化
  const brands = [...new Set(APP_DATA.inventory.map(i => i.brand))].sort();
  const brandSel = document.getElementById('beFilterBrand');
  brandSel.innerHTML = '<option value="">すべて</option>' +
    brands.map(b => `<option value="${b}">${b}</option>`).join('');

  // フィルタをクリア
  document.getElementById('beFilterDateFrom').value = '';
  document.getElementById('beFilterDateTo').value   = '';
  document.getElementById('beFilterKeyword').value  = '';
  brandSel.value = '';

  // 登録済み商品を描画
  renderBoxEditCurrent(no);

  // 追加候補を初期描画（全件）
  _boxEditFilteredItems = [];
  document.getElementById('boxEditAddBody').innerHTML =
    '<tr><td colspan="8" class="box-edit-empty">絞り込みで商品を表示してください</td></tr>';
  _updateBoxEditSelectedCount();

  modal.classList.remove('hidden');
}

function closeBoxEditModal() {
  document.getElementById('boxEditModalOverlay')?.classList.add('hidden');
  _activeBoxNo = null;
  _boxEditFilteredItems = [];
}

// ── 登録済み商品テーブルを描画 ──
function renderBoxEditCurrent(no) {
  const items = APP_DATA.inventory.filter(item => item.boxNo === no);
  const tbody = document.getElementById('boxEditCurrentBody');
  const badge = document.getElementById('boxEditCurrentCount');

  badge.textContent = `${items.length} 点`;

  if (items.length === 0) {
    tbody.innerHTML = '<tr><td colspan="7" class="box-edit-empty">商品がありません</td></tr>';
    return;
  }

  tbody.innerHTML = items.map(item => `
    <tr id="box-curr-row-${item.code.replace(/[^a-zA-Z0-9]/g, '_')}">
      <td><code style="font-size:11px;color:var(--primary-light);">${item.code}</code></td>
      <td>${item.brand}</td>
      <td>${item.model}</td>
      <td style="font-size:12px;">${item.purchaseDate || '—'}</td>
      <td style="text-align:right;">${formatSalePrice(item.salePrice)}</td>
      <td>${getStatusBadge(typeof normalizeInventoryStatusLabel === 'function' ? normalizeInventoryStatusLabel(item.status) : item.status)}</td>
      <td>
        <button class="btn btn-danger btn-sm"
          onclick="removeItemFromBox('${item.code}')"
          title="このBOXから外す">
          <i class="fa-solid fa-minus"></i> 外す
        </button>
      </td>
    </tr>`).join('');
}

// ── 商品をBOXから外す ──
async function removeItemFromBox(code) {
  const item = APP_DATA.inventory.find(i => i.code === code);
  if (!item) return;
  item.boxNo = null;
  if (typeof persistGuestBoxState === 'function') persistGuestBoxState();
  if (window.ZaikoAPI) {
    try {
      await window.ZaikoAPI.saveBoxes(APP_DATA.boxes || []);
    } catch (error) {
      await window.ZaikoAPI.hydrateAdmin().catch(() => {});
      showToast('error', 'BOXから外せませんでした', error.message || '入力内容を確認してください');
      return;
    }
  }

  renderBoxEditCurrent(_activeBoxNo);
  _syncBoxEditAddRow(code);  // 追加候補行を更新
  _refreshAfterBoxEdit();
  showToast('success', 'BOXから外しました', `${item.brand} ${item.model}`);
}

// ── フィルタを適用して追加候補を描画 ──
function applyBoxEditFilter() {
  const dateFrom = document.getElementById('beFilterDateFrom').value;
  const dateTo   = document.getElementById('beFilterDateTo').value;
  const brand    = document.getElementById('beFilterBrand').value;
  const keyword  = document.getElementById('beFilterKeyword').value.trim().toLowerCase();

  _boxEditFilteredItems = APP_DATA.inventory.filter(item => {
    // 入荷日フィルタ
    if (dateFrom && item.purchaseDate < dateFrom) return false;
    if (dateTo   && item.purchaseDate > dateTo)   return false;
    // ブランドフィルタ
    if (brand && item.brand !== brand) return false;
    // キーワード（商品コード・モデル）
    if (keyword) {
      const hit = item.code.toLowerCase().includes(keyword) ||
                  item.model.toLowerCase().includes(keyword) ||
                  item.brand.toLowerCase().includes(keyword);
      if (!hit) return false;
    }
    return true;
  });

  _renderBoxEditAddList();
}

function clearBoxEditFilter() {
  document.getElementById('beFilterDateFrom').value = '';
  document.getElementById('beFilterDateTo').value   = '';
  document.getElementById('beFilterBrand').value    = '';
  document.getElementById('beFilterKeyword').value  = '';
  _boxEditFilteredItems = [];
  document.getElementById('boxEditAddBody').innerHTML =
    '<tr><td colspan="8" class="box-edit-empty">絞り込みで商品を表示してください</td></tr>';
  _updateBoxEditSelectedCount();
  // 全選択チェックを外す
  const all = document.getElementById('beSelectAll');
  if (all) { all.checked = false; all.indeterminate = false; }
  const allMark = document.getElementById('beSelectAllMark');
  if (allMark) allMark.classList.remove('checked');
}

// ── 追加候補リストを描画 ──
function _renderBoxEditAddList() {
  const tbody = document.getElementById('boxEditAddBody');
  if (_boxEditFilteredItems.length === 0) {
    tbody.innerHTML = '<tr><td colspan="8" class="box-edit-empty">条件に一致する商品がありません</td></tr>';
    _updateBoxEditSelectedCount();
    return;
  }

  tbody.innerHTML = _boxEditFilteredItems.map(item => {
    const inThisBox  = item.boxNo === _activeBoxNo;
    const inOtherBox = item.boxNo != null && !inThisBox;
    const boxLabel   = item.boxNo != null
      ? `<span class="badge ${inThisBox ? 'badge-approval-approved' : 'badge-approval-pending'}">BOX${item.boxNo}</span>`
      : '<span style="font-size:11px;color:var(--text-muted);">未割当</span>';

    return `
      <tr class="box-edit-add-row${inThisBox ? ' already-in' : ''}"
          id="be-add-row-${item.code.replace(/[^a-zA-Z0-9]/g, '_')}">
        <td style="text-align:center;">
          ${inThisBox
            ? `<span style="font-size:12px;color:#059669;"><i class="fa-solid fa-check-circle"></i></span>`
            : `<label class="box-check-label">
                 <input type="checkbox" class="be-item-check box-check-input"
                   data-code="${item.code}"
                   onchange="_onBeItemCheckChange()">
                 <span class="box-check-mark"></span>
               </label>`
          }
        </td>
        <td><code style="font-size:11px;color:var(--primary-light);">${item.code}</code></td>
        <td>${item.brand}</td>
        <td>${item.model}</td>
        <td style="font-size:12px;">${item.purchaseDate || '—'}</td>
        <td style="text-align:right;">${formatSalePrice(item.salePrice)}</td>
        <td>${getStatusBadge(typeof normalizeInventoryStatusLabel === 'function' ? normalizeInventoryStatusLabel(item.status) : item.status)}</td>
        <td>${boxLabel}</td>
      </tr>`;
  }).join('');

  _updateBoxEditSelectedCount();
}

// ── 全選択トグル ──
function toggleBoxEditSelectAll(cb) {
  document.querySelectorAll('.be-item-check').forEach(c => {
    c.checked = cb.checked;
    const mark = c.nextElementSibling;
    if (mark) mark.classList.toggle('checked', cb.checked);
  });
  const allMark = document.getElementById('beSelectAllMark');
  if (allMark) allMark.classList.toggle('checked', cb.checked);
  _updateBoxEditSelectedCount();
}

// ── 個別チェック変更 ──
function _onBeItemCheckChange() {
  const checks  = document.querySelectorAll('.be-item-check');
  const checked = document.querySelectorAll('.be-item-check:checked');
  const allCb   = document.getElementById('beSelectAll');
  const allMark = document.getElementById('beSelectAllMark');
  if (allCb) {
    allCb.indeterminate = checked.length > 0 && checked.length < checks.length;
    allCb.checked       = checked.length === checks.length && checks.length > 0;
  }
  if (allMark) allMark.classList.toggle('checked', allCb?.checked);

  // チェックマーク同期
  document.querySelectorAll('.be-item-check').forEach(c => {
    const mark = c.nextElementSibling;
    if (mark) mark.classList.toggle('checked', c.checked);
  });
  _updateBoxEditSelectedCount();
}

function _updateBoxEditSelectedCount() {
  const n    = document.querySelectorAll('.be-item-check:checked').length;
  const span = document.getElementById('boxEditSelectedCount');
  const btn  = document.getElementById('boxEditAddBtn');
  if (span) span.textContent = `${n}件選択中`;
  if (btn)  btn.disabled = n === 0;
}

// ── 選択した商品を一括追加 ──
async function addCheckedItemsToBox() {
  const no = _activeBoxNo;
  if (no == null) return;

  const codes = [...document.querySelectorAll('.be-item-check:checked')]
    .map(c => c.dataset.code);
  if (codes.length === 0) return;

  let added = 0;
  codes.forEach(code => {
    const item = APP_DATA.inventory.find(i => i.code === code);
    if (item) { item.boxNo = no; added++; }
  });
  if (typeof persistGuestBoxState === 'function') persistGuestBoxState();
  if (window.ZaikoAPI) {
    try {
      await window.ZaikoAPI.saveBoxes(APP_DATA.boxes || []);
    } catch (error) {
      await window.ZaikoAPI.hydrateAdmin().catch(() => {});
      showToast('error', 'BOXへ追加できませんでした', error.message || '入力内容を確認してください');
      return;
    }
  }

  // 全選択チェックをリセット
  const allCb = document.getElementById('beSelectAll');
  if (allCb) { allCb.checked = false; allCb.indeterminate = false; }
  const allMark = document.getElementById('beSelectAllMark');
  if (allMark) allMark.classList.remove('checked');

  // 登録済みを再描画
  renderBoxEditCurrent(no);
  // 追加候補リストを再描画（追加済みマーク反映）
  _renderBoxEditAddList();
  _refreshAfterBoxEdit();

  showToast('success', `BOX${no}に追加しました`, `${added}点の商品を追加`);
}

// ── BOX編集後に関連UIを更新 ──
function _refreshAfterBoxEdit() {
  // BOX管理マトリクスを更新
  if (document.getElementById('boxMatrixNumbers')) renderBoxMatrix();
  // 在庫テーブルが表示中なら再描画
  if (typeof renderInventoryTable === 'function' &&
      document.getElementById('inventoryTableBody')) {
    renderInventoryTable();
  }
  // マスタページのBOXリストが表示中なら更新
  if (document.getElementById('boxListBody')) renderBoxList();
}

// 追加候補リスト内の特定行のBOXラベルを更新（外したとき）
function _syncBoxEditAddRow(code) {
  const safeCode = code.replace(/[^a-zA-Z0-9]/g, '_');
  const row = document.getElementById(`be-add-row-${safeCode}`);
  if (!row) return;
  // 行全体を再描画する代わりにリスト全体を再描画（件数が少ないため許容）
  _renderBoxEditAddList();
}

// =====================================================
// BOX名編集（BOX管理ページ下部エリア）
// =====================================================

function openBoxNameEdit(no) {
  _activeBoxNo = no;
  const box  = APP_DATA.boxes[no - 1];
  const area = document.getElementById('boxNameEditArea');
  document.getElementById('boxNameEditNo').textContent = no;
  document.getElementById('boxNameEditInput').value    = box?.name || '';
  area?.classList.remove('hidden');
  closeBoxLineupModal();
  area?.scrollIntoView({ behavior: 'smooth', block: 'center' });
}

function closeBoxNameEdit() {
  _activeBoxNo = null;
  document.getElementById('boxNameEditArea')?.classList.add('hidden');
}

async function saveBoxName() {
  if (_activeBoxNo == null) return;
  const box  = APP_DATA.boxes[_activeBoxNo - 1];
  if (!box) return;
  const name = document.getElementById('boxNameEditInput').value.trim();
  box.name   = name;
  if (!box.createdAt) box.createdAt = new Date().toISOString().slice(0, 10);
  if (typeof persistGuestBoxState === 'function') persistGuestBoxState();
  if (window.ZaikoAPI) {
    try {
      await window.ZaikoAPI.saveBoxes([box]);
    } catch (error) {
      await window.ZaikoAPI.hydrateAdmin().catch(() => {});
      showToast('error', 'BOX名を保存できませんでした', error.message || '入力内容を確認してください');
      return;
    }
  }

  showToast('success', `BOX${_activeBoxNo} の名前を保存しました`, name || '（名前なし）');
  closeBoxNameEdit();
  renderBoxMatrix();
}

function renderBoxMasterTab(area) {
  if (!area) return;
  area.innerHTML = `
    <div class="master-content">
      <div style="display:flex;align-items:flex-start;justify-content:space-between;gap:14px;flex-wrap:wrap;margin-bottom:14px;">
        <div>
          <h3 style="font-size:15px;font-weight:bold;color:var(--primary);margin-bottom:4px;"><i class="fa-solid fa-users-gear"></i> ゲスト管理</h3>
          <p style="font-size:12px;color:var(--text-muted);margin:0;">サイドメニューのゲスト管理と同じ企業・BOX公開設定を表示しています。</p>
        </div>
        <div style="display:flex;align-items:center;gap:10px;flex-wrap:wrap;">
          <div id="masterGuestPublishStatus" style="font-size:12px;display:flex;align-items:center;gap:6px;"></div>
          <button class="btn btn-accent btn-sm" onclick="publishGuestSnapshot()"><i class="fa-solid fa-satellite-dish"></i> ゲスト公開情報を一括更新</button>
        </div>
      </div>
      <div style="padding:10px 12px;margin-bottom:14px;background:#eff6ff;border:1px solid #bfdbfe;border-radius:8px;font-size:12px;color:#1e3a5f;">
        <i class="fa-solid fa-link"></i> ここでのチェック変更・BOX商品編集・公開更新は、サイドメニューの「ゲスト管理」とゲスト商品画面へ同じ内容で反映されます。
      </div>
      <div class="card" style="box-shadow:none;border:1px solid var(--border);">
        <div class="card-body" style="padding:0;">
          <div class="box-matrix-scroll" aria-label="企業別BOX公開設定。横にスクロールできます">
            <div class="box-matrix-header">
              <div class="box-matrix-company-cell box-matrix-label">企業名</div>
              <div class="box-matrix-numbers"><div id="masterBoxMatrixNumbers" class="box-matrix-num-row"></div></div>
            </div>
            <div id="masterBoxMatrixBody"></div>
          </div>
        </div>
      </div>
    </div>`;
  renderBoxMatrix();
}

// =====================================================
// 互換レイヤー（他ファイルから呼ばれる旧関数名）
// =====================================================

function renderBoxCards() { renderBoxMatrix(); }

function renderBoxList() {
  const tbody = document.getElementById('boxListBody');
  if (tbody) {
    tbody.innerHTML = APP_DATA.boxes.map(box => {
      const count    = APP_DATA.inventory.filter(item => item.boxNo === box.no).length;
      const pubCount = (box.publicTo || []).length;
      const nameStr  = box.name?.trim()
        || `<span style="color:var(--text-muted);font-style:italic;">（未設定）</span>`;
      return `<tr>
        <td><b>BOX ${box.no}</b></td>
        <td>${nameStr}</td>
        <td><span class="badge ${pubCount > 0 ? 'badge-approval-approved' : 'badge-approval-pending'}">
          ${pubCount > 0 ? `${pubCount}社に公開` : '非公開'}
        </span></td>
        <td><span class="badge badge-stock">${count} 点</span></td>
        <td>
          <button class="btn btn-sm btn-primary" onclick="openBoxLineupModal(${box.no})">
            <i class="fa-solid fa-eye"></i> 明細
          </button>
          <button class="btn btn-sm btn-warning" style="margin-left:4px;"
            onclick="openBoxEditModal(${box.no})">
            <i class="fa-solid fa-pen"></i> 編集
          </button>
        </td>
      </tr>`;
    }).join('');
  }
  renderBoxMatrix();
}

function toggleBoxPublic(boxId) {
  const no = parseInt(String(boxId).replace('BOX-', ''));
  if (!no) return;
  openBoxLineupModal(no);
}

function openAddBoxModal() {
  showToast('info', 'BOX管理', 'BOX番号をクリックして商品の編集ができます');
}

function openEditBoxModal(boxId) {
  const no = parseInt(String(boxId).replace('BOX-', ''));
  if (no) openBoxEditModal(no);
}

function closeBoxModal()    { closeBoxEditModal(); }
function saveBox()          { saveBoxName(); }

function deleteBox(boxId) {
  const no = parseInt(String(boxId).replace('BOX-', ''));
  if (!no) return;
  if (!confirm(`BOX${no}の商品割り当てをすべて解除しますか？`)) return;
  APP_DATA.inventory.forEach(item => { if (item.boxNo === no) item.boxNo = null; });
  if (typeof persistGuestBoxState === 'function') persistGuestBoxState();
  renderBoxMatrix();
  showToast('success', `BOX${no} の商品割り当てを解除しました`, '');
}

function openBoxDetailModal(boxId) {
  const no = parseInt(String(boxId).replace('BOX-', ''));
  if (no) openBoxLineupModal(no);
}
function closeBoxDetailModal() { closeBoxLineupModal(); }

function initBoxSelect(selectId, currentBoxNo) {
  const sel = document.getElementById(selectId);
  if (!sel) return;
  sel.innerHTML = '<option value="">— BOX未割当 —</option>' +
    APP_DATA.boxes.map(b => {
      const label = b.name?.trim() ? `BOX${b.no}：${b.name}` : `BOX${b.no}`;
      return `<option value="${b.no}" ${currentBoxNo == b.no ? 'selected' : ''}>${label}</option>`;
    }).join('');
}

function initBoxFilterSelect(selectId) {
  const sel = document.getElementById(selectId);
  if (!sel) return;
  sel.innerHTML = '<option value="">全BOX</option>' +
    APP_DATA.boxes
      .filter(b => b.name?.trim() || APP_DATA.inventory.some(i => i.boxNo === b.no))
      .map(b => {
        const label = b.name?.trim() ? `BOX${b.no}：${b.name}` : `BOX${b.no}`;
        return `<option value="${b.no}">${label}</option>`;
      }).join('');
}
