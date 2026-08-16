// =====================================================
// approval.js — 管理者／作業者の承認フロー
// =====================================================

const APPROVAL_WORKFLOW_STORAGE_KEY = 'inv_approval_workflow_v2';

function _approvalLoadWorkflowState() {
  try {
    const stored = JSON.parse(localStorage.getItem(APPROVAL_WORKFLOW_STORAGE_KEY) || 'null');
    const requests = Array.isArray(stored) ? stored : stored?.requests;
    if (Array.isArray(requests)) APP_DATA.approvalRequests = requests;
  } catch {
    // 保存データが壊れている場合はデモ初期値を利用する
  }
}

function _approvalPersistWorkflowState() {
  try {
    localStorage.setItem(APPROVAL_WORKFLOW_STORAGE_KEY, JSON.stringify({
      version: 2,
      savedAt: new Date().toISOString(),
      requests: APP_DATA.approvalRequests || [],
    }));
  } catch {
    // プライベートブラウズ等で保存できない場合も画面内の操作は継続する
  }
}

function _approvalRequesterId(req) { return req.requesterId || req.buyerId || ''; }
function _approvalRequesterName(req) { return req.requesterName || req.buyerName || '—'; }
function _approvalClone(value) { return JSON.parse(JSON.stringify(value)); }

function _approvalNextId() {
  const max = (APP_DATA.approvalRequests || []).reduce((current, request) => {
    const match = String(request.id || '').match(/^APR-(\d+)$/);
    return match ? Math.max(current, Number(match[1])) : current;
  }, 0);
  return `APR-${String(max + 1).padStart(3, '0')}`;
}

function _approvalRefreshUI() {
  updateApprovalBadge();
  const pendingEl = document.getElementById('pendingApprovalCount');
  if (pendingEl) pendingEl.textContent = APP_DATA.approvalRequests.filter(r => r.status === 'pending').length;
}

/** 承認済み申請の対象操作を、画面データへ冪等に反映する */
function _approvalApplyApprovedOperation(req, { replay = false } = {}) {
  const detail = req.detail || {};

  if (req.type === 'item_edit' && detail.code && detail.after) {
    const item = (APP_DATA.inventory || []).find(record => record.code === detail.code);
    if (item) {
      Object.assign(item, detail.after);
      if (!Array.isArray(item.editHistory)) item.editHistory = [];
      if (!item.editHistory.some(history => history.approvalId === req.id)) {
        item.editHistory.unshift({
          approvalId: req.id,
          editedAt: req.approvedAt || new Date().toLocaleString('ja-JP'),
          editorName: _approvalRequesterName(req),
          approverName: req.approvedByName || '管理者',
          changes: detail.changes || [],
          note: req.note || '',
        });
      }
      if (typeof syncInventoryItemToDocuments === 'function') syncInventoryItemToDocuments(item);
    }
  }

  if (['_revision'].some(suffix => req.type?.endsWith(suffix)) && detail.id && detail.after) {
    const baseType = req.type.replace(/_revision$/, '');
    const collections = {
      purchase: APP_DATA.purchaseSlips,
      shipping: APP_DATA.shipments,
      sales: APP_DATA.sales,
    };
    const record = (collections[baseType] || []).find(item => item.id === detail.id);
    if (record) {
      Object.keys(record).forEach(key => { delete record[key]; });
      Object.assign(record, _approvalClone(detail.after));
      const latestRevision = record.revisions?.[record.revisions.length - 1];
      if (latestRevision) latestRevision.approverName = req.approvedByName || '管理者';
      if (typeof applyBusinessRecordState === 'function') applyBusinessRecordState(baseType, record);
    }
  }

  if (req.type === 'sales' && detail.slipId) {
    let sale = (APP_DATA.sales || []).find(record => record.id === detail.slipId);
    if (!sale) {
      sale = {
        id: detail.slipId,
        date: detail.date || '',
        items: detail.items || [],
        total: Number(detail.total) || 0,
        currency: detail.currency || 'USD',
        inputCurrency: detail.inputCurrency || 'USD',
        sourceShipmentId: detail.sourceShipmentId || '',
        usdJpyRate: Number(detail.usdJpyRate) || 155,
        taxFree: Boolean(detail.taxFree),
        taxAmount: Number(detail.taxAmount) || 0,
        grandTotal: Number(detail.grandTotal) || Number(detail.total) || 0,
        buyer: detail.buyer || '',
        note: detail.note || '',
        revisions: [],
      };
      APP_DATA.sales.push(sale);
    }
    sale.status = '確定';
    sale.approvalId = req.id;
    const soldProductCodes = [];
    (detail.items || []).forEach(saleItem => {
      if (saleItem.returnType) return;
      const item = (APP_DATA.inventory || []).find(record => record.code === saleItem.code);
      if (item) {
        item.status = '売上済';
        if (typeof clearInventoryReservationMetadata === 'function') clearInventoryReservationMetadata(item);
        soldProductCodes.push(item.code);
      }
    });
    if (typeof unpublishGuestProducts === 'function') unpublishGuestProducts(soldProductCodes);
    if (typeof applyBusinessRecordState === 'function') applyBusinessRecordState('sales', sale);
  }

  if (req.type === 'purchase_return' && detail.record) {
    let record = (APP_DATA.purchaseReturns || []).find(item => item.id === detail.record.id);
    if (!record) {
      record = _approvalClone(detail.record);
      APP_DATA.purchaseReturns.push(record);
    }
    record.status = '承認済';
    (record.items || []).forEach(item => { item.status = '承認済'; });
    if (typeof applyBusinessRecordState === 'function') applyBusinessRecordState('purchasereturn', record);
  }

  if (req.type === 'sales_return' && detail.record) {
    let record = (APP_DATA.salesReturns || []).find(item => item.id === detail.record.id);
    if (!record) {
      record = _approvalClone(detail.record);
      APP_DATA.salesReturns.push(record);
    }
    record.status = '承認済';
    const sale = (APP_DATA.sales || []).find(item => item.id === detail.record.slipId);
    (record.items || []).forEach(returnItem => {
      const saleItem = (sale?.items || []).find(item => item.code === returnItem.code);
      if (saleItem) {
        saleItem.returnType = 'return';
        saleItem.returnStatus = 'pending';
      }
      const inventoryItem = (APP_DATA.inventory || []).find(item => item.code === returnItem.code);
      if (inventoryItem) inventoryItem.status = '在庫中';
    });
    if (typeof applyBusinessRecordState === 'function') applyBusinessRecordState('salesreturn', record);
  }

  req.executedAt = req.executedAt || req.approvedAt || new Date().toLocaleString('ja-JP');
  if (!replay) _approvalPersistWorkflowState();
  if (typeof refreshLinkedBusinessViews === 'function') {
    refreshLinkedBusinessViews({ persist: !replay, source: `approval-${req.type}` });
  }
}

function replayApprovedApprovalOperations() {
  (APP_DATA.approvalRequests || [])
    .filter(request => request.status === 'approved')
    .forEach(request => _approvalApplyApprovedOperation(request, { replay: true }));
}

function getLatestPerformanceApproval(userId = currentUserId()) {
  return (APP_DATA.approvalRequests || [])
    .filter(request => request.type === 'performance'
      && request.detail?.targetField === '集計実行'
      && _approvalRequesterId(request) === userId)
    .slice()
    .sort((a, b) => String(b.createdAt || '').localeCompare(String(a.createdAt || '')))[0] || null;
}

_approvalLoadWorkflowState();

// ── OTP生成（6桁数字） ──
function generateOTP() {
  return String(Math.floor(100000 + Math.random() * 900000));
}

// ── OTP有効期限（30分後） ──
function otpExpiry() {
  const d = new Date();
  d.setMinutes(d.getMinutes() + 30);
  return d.toISOString();
}

// ── OTP有効チェック ──
function isOTPValid(req) {
  if (!req.otp || req.otpUsed) return false;
  if (!req.otpExpiry) return true;
  return new Date() < new Date(req.otpExpiry);
}

// ── 承認リクエスト作成 ──
function createApprovalRequest(type, typeLabel, detail, note = '', existingId = null) {
  const user = currentUser();
  if (!user) return null;

  // ⑤ 差戻し後の再申請: existingId が渡された場合はそのIDを再利用する
  const id = existingId || _approvalNextId();

  // existingId がある場合は既存レコードを上書き（同一IDで管理）
  if (existingId) {
    const existing = APP_DATA.approvalRequests.find(r => r.id === existingId);
    if (existing) {
      const previousRevisionComment = existing.revisionComment || '';
      existing.status       = 'pending';
      existing.revisionComment = '';
      existing.note         = note || existing.note;
      existing.detail       = detail;
      existing.createdAt    = new Date().toLocaleString('ja-JP');
      // ⑥ 差戻し履歴を積む
      if (!Array.isArray(existing.revisionHistory)) existing.revisionHistory = [];
      existing.revisionHistory.push({
        comment:    previousRevisionComment,
        revisedAt:  new Date().toLocaleString('ja-JP'),
        reappliedAt: new Date().toLocaleString('ja-JP'),
      });
      const adminUsers = APP_DATA.users.filter(u => u.role === 'admin');
      adminUsers.forEach(admin => {
        createNotification({
          toUserId:   admin.id,
          fromUserId: user.id,
          fromName:   user.name,
          type:       'approval_request',
          title:      `再申請：${typeLabel}（${existingId}）`,
          body:       `${user.name} さんが ${typeLabel} を再申請しました（ID: ${existingId}）。`,
          relatedId:  existingId,
        });
      });
      updateApprovalBadge();
      _approvalPersistWorkflowState();
      return existing;
    }
  }

  const req = {
    id,
    requesterId: user.id,
    requesterName: user.name,
    buyerId: user.id,       // 旧データ互換
    buyerName: user.name,   // 旧データ互換
    type,
    typeLabel,
    detail,
    status: 'pending',
    note,
    revisionComment: '',
    revisionHistory: [],   // ⑥ 差戻し履歴（拡張性確保）
    otp: null,
    otpExpiry: null,
    otpUsed: false,
    otpAttempts: 0,
    createdAt: new Date().toLocaleString('ja-JP'),
  };
  APP_DATA.approvalRequests.push(req);

  const adminUsers = APP_DATA.users.filter(u => u.role === 'admin');
  adminUsers.forEach(admin => {
    createNotification({
      toUserId: admin.id,
      fromUserId: user.id,
      fromName: user.name,
      type: 'approval_request',
      title: `承認リクエスト：${typeLabel}`,
      body: `${user.name} さんが${typeLabel}の承認を申請しました。`,
      relatedId: id,
    });
  });

  updateApprovalBadge();
  _approvalPersistWorkflowState();
  return req;
}

// ── 承認処理（管理者） ──
function approveRequest(reqId) {
  const req = APP_DATA.approvalRequests.find(r => r.id === reqId);
  if (!req) return false;
  if (!isAdmin()) {
    showToast('error', 'アクセス拒否', '承認は管理者のみ実行できます');
    return false;
  }
  if (_approvalRequesterId(req) === currentUserId()) {
    showToast('error', '自己承認はできません', '別の管理者が承認してください');
    return false;
  }

  if (window.ZaikoAPI && req.apiManaged) {
    (async () => {
      try {
        await window.ZaikoAPI.decideApproval(reqId, 'approved', '');
        updateNotifyBadge(); _approvalRefreshUI(); renderApprovalList(); closeApprovalDetail();
        showToast('success', '承認しました', `${req.typeLabel} をDBで承認・確定しました`);
      } catch (error) {
        showToast('error', '承認エラー', error.message);
      }
    })();
    return true;
  }

  if (req.type === 'sales') {
    const unavailable = (req.detail?.items || [])
      .filter(saleItem => !saleItem.returnType)
      .map(saleItem => (APP_DATA.inventory || []).find(item => item.code === saleItem.code))
      .filter(item => {
        if (item?.status === '保留' && item.reservationApprovalId === req.id) return false;
        if (typeof canUseInventoryItemForSales === 'function') {
          return !canUseInventoryItemForSales(item, req.detail?.sourceShipmentId || '');
        }
        return !item || item.status === '取置中' || item.status === '売上済';
      });
    if (unavailable.length > 0) {
      showToast('error', '売上を承認できません', `他の購入リクエストで取置中、または使用済みの商品があります: ${unavailable.map(item => item?.code || '未登録').join('、')}`);
      return false;
    }
  }

  req.status = 'approved';
  req.approvedAt = new Date().toLocaleString('ja-JP');
  req.approvedById = currentUserId();
  req.approvedByName = currentUser()?.name || '管理者';
  _approvalApplyApprovedOperation(req);

  createNotification({
    toUserId: _approvalRequesterId(req),
    fromUserId: currentUserId(),
    fromName: currentUser()?.name || '管理者',
    type: 'approved',
    title: `承認完了：${req.typeLabel}`,
    body: `${req.typeLabel}の申請が承認されました。`,
    relatedId: reqId,
  });

  updateNotifyBadge();
  _approvalRefreshUI();
  renderApprovalList();
  closeApprovalDetail();
  showToast('success', '承認しました', `${req.typeLabel} を承認しました`);
  return true;
}

// ── 差戻し処理（管理者） ──
function reviseRequest(reqId, comment) {
  const req = APP_DATA.approvalRequests.find(r => r.id === reqId);
  if (!req) return false;
  if (!isAdmin()) {
    showToast('error', 'アクセス拒否', '差戻しは管理者のみ実行できます');
    return false;
  }
  if (_approvalRequesterId(req) === currentUserId()) {
    showToast('error', '自己判断はできません', '別の管理者が判断してください');
    return false;
  }


  if (window.ZaikoAPI && req.apiManaged) {
    (async () => {
      try {
        await window.ZaikoAPI.decideApproval(reqId, 'returned', comment || '');
        updateNotifyBadge(); _approvalRefreshUI(); renderApprovalList(); closeApprovalDetail();
        showToast('warning', '差戻しました', `${_approvalRequesterName(req)} さんへDB通知を送信しました`);
      } catch (error) {
        showToast('error', '差戻しエラー', error.message);
      }
    })();
    return true;
  }

  if (req.type === 'sales') {
    (req.detail?.items || []).forEach(saleItem => {
      const inventoryItem = (APP_DATA.inventory || []).find(item => item.code === saleItem.code);
      if (!inventoryItem || inventoryItem.reservationApprovalId !== req.id) return;
      inventoryItem.status = inventoryItem.reservationPreviousStatus || '在庫中';
      if (typeof clearInventoryReservationMetadata === 'function') clearInventoryReservationMetadata(inventoryItem);
    });
  }

  req.status = 'revision';
  req.revisionComment = comment;

  // ⑥ 差戻し履歴を記録
  if (!Array.isArray(req.revisionHistory)) req.revisionHistory = [];
  req.revisionHistory.push({
    comment,
    revisedAt: new Date().toLocaleString('ja-JP'),
    revisedBy: currentUser()?.name || '管理者',
  });

  // ④ 差戻し通知（申請者へ）
  createNotification({
    toUserId:   _approvalRequesterId(req),
    fromUserId: currentUserId(),
    fromName:   currentUser()?.name || '管理者',
    type:       'revision',
    title:      `【差戻し】${req.typeLabel}（${reqId}）`,
    body:       `申請「${req.typeLabel}」（ID: ${reqId}）が差し戻されました。\n` +
                `理由: ${comment}\n再申請する際は同じIDで申請してください。`,
    relatedId:  reqId,
  });

  updateNotifyBadge();
  _approvalPersistWorkflowState();
  _approvalRefreshUI();
  if (typeof refreshLinkedBusinessViews === 'function') {
    refreshLinkedBusinessViews({ source: `approval-revision-${req.type}` });
  }
  renderApprovalList();
  closeApprovalDetail();
  showToast('warning', '差戻しました', `${_approvalRequesterName(req)} さんへ通知を送信しました`);
  return true;
}

// ── OTP検証 ──
function verifyOTP(reqId, inputOtp) {
  const req = APP_DATA.approvalRequests.find(r => r.id === reqId);
  if (!req) return { ok: false, msg: '承認リクエストが見つかりません' };
  if (req.status !== 'approved') return { ok: false, msg: '承認済みではありません' };
  if (req.otpUsed) return { ok: false, msg: '承認コードは既に使用済みです' };
  if (req.otpAttempts >= 5) return { ok: false, msg: '試行回数超過。管理者に再申請してください' };
  if (!isOTPValid(req)) return { ok: false, msg: '承認コードの有効期限が切れています' };

  req.otpAttempts++;
  if (req.otp !== inputOtp) {
    return { ok: false, msg: `承認コードが違います（残り${5 - req.otpAttempts}回）` };
  }
  req.otpUsed = true;
  return { ok: true, msg: 'OK' };
}

// =====================================================
// ① ソート状態管理
// =====================================================
let _approvalSortKey = 'createdAt';
let _approvalSortDir = 'desc'; // 初期: 新しい順

/** ソート実行（ヘッダークリックから呼ばれる） */
function sortApprovalList(key) {
  if (_approvalSortKey === key) {
    _approvalSortDir = _approvalSortDir === 'asc' ? 'desc' : 'asc';
  } else {
    _approvalSortKey = key;
    _approvalSortDir = 'asc';
  }
  renderApprovalList();
}

/** ソートアイコンの更新 */
function _updateSortIcons() {
  const keys = ['createdAt', 'buyerName', 'typeLabel', 'status'];
  keys.forEach(k => {
    const iconEl = document.getElementById(`sort-icon-${k}`);
    const thEl   = iconEl?.closest('th');
    if (!iconEl || !thEl) return;
    thEl.classList.remove('sort-asc', 'sort-desc');
    if (k === _approvalSortKey) {
      iconEl.textContent = _approvalSortDir === 'asc' ? '▲' : '▼';
      thEl.classList.add(_approvalSortDir === 'asc' ? 'sort-asc' : 'sort-desc');
    } else {
      iconEl.textContent = '⇅';
    }
  });
}

/** ステータスのソート順（pending → revision → approved） */
const _statusOrder = { pending: 0, revision: 1, approved: 2 };

// =====================================================
// 承認リクエスト一覧レンダリング
// =====================================================
function renderApprovalList() {
  const tbody = document.getElementById('approvalListBody');
  if (!tbody) return;

  // ③ 承認済を除外
  let list = APP_DATA.approvalRequests.filter(r => r.status !== 'approved');

  // ① ソート
  list = list.slice().sort((a, b) => {
    let aVal, bVal;
    if (_approvalSortKey === 'status') {
      aVal = _statusOrder[a.status] ?? 99;
      bVal = _statusOrder[b.status] ?? 99;
    } else if (_approvalSortKey === 'createdAt') {
      aVal = a.createdAt || '';
      bVal = b.createdAt || '';
    } else {
      aVal = (a[_approvalSortKey] || '').toString().toLowerCase();
      bVal = (b[_approvalSortKey] || '').toString().toLowerCase();
    }
    if (aVal < bVal) return _approvalSortDir === 'asc' ? -1 : 1;
    if (aVal > bVal) return _approvalSortDir === 'asc' ?  1 : -1;
    return 0;
  });

  _updateSortIcons();

  if (list.length === 0) {
    tbody.innerHTML = '<tr><td colspan="6" style="text-align:center;color:#aaa;padding:32px;">承認リクエストはありません</td></tr>';
    return;
  }

  tbody.innerHTML = list.map(req => {
    const statusBadge = {
      pending:  '<span class="badge-approval pending"><i class="fa-solid fa-clock"></i> 保留中</span>',
      approved: '<span class="badge-approval approved"><i class="fa-solid fa-circle-check"></i> 承認済</span>',
      revision: '<span class="badge-approval revision"><i class="fa-solid fa-rotate-left"></i> 差戻し</span>',
    }[req.status] || req.status;

    // 内容サマリー（クリック可）
    const summaryHtml = buildDetailSummary(req);

    return `<tr class="appr-row" onclick="openApprovalDetail('${req.id}')" title="クリックで詳細表示">
      <td><code style="font-size:11px;">${req.id}</code></td>
      <td style="white-space:nowrap;font-size:12px;">
        <i class="fa-solid fa-calendar-days" style="color:var(--text-muted);margin-right:4px;"></i>${req.createdAt}
      </td>
      <td>
        <div style="font-weight:bold;font-size:13px;">${req.buyerName}</div>
      </td>
      <td>
        <span class="appr-type-badge ${req.type}">${typeIcon(req.type)} ${req.typeLabel}</span>
      </td>
      <td class="appr-summary-cell">${summaryHtml}</td>
      <td>${statusBadge}</td>
    </tr>`;
  }).join('');
}

// ── 種別アイコン ──
function typeIcon(type) {
  const icons = {
    purchase:         '<i class="fa-solid fa-file-import"></i>',
    purchase_edit:    '<i class="fa-solid fa-file-pen"></i>',
    purchase_return:  '<i class="fa-solid fa-boxes-packing"></i>',
    shipping:         '<i class="fa-solid fa-truck"></i>',
    shipping_edit:    '<i class="fa-solid fa-truck-ramp-box"></i>',
    sales:            '<i class="fa-solid fa-yen-sign"></i>',
    sales_edit:       '<i class="fa-solid fa-file-invoice-dollar"></i>',
    sales_return:     '<i class="fa-solid fa-rotate-left"></i>',
    master:           '<i class="fa-solid fa-database"></i>',
    performance:      '<i class="fa-solid fa-chart-bar"></i>',
    return_to_stock:      '<i class="fa-solid fa-rotate-left"></i>',
    setting:              '<i class="fa-solid fa-gear"></i>',
    stocktake_mismatch:   '<i class="fa-solid fa-clipboard-list"></i>',
  };
  return icons[type] || '<i class="fa-solid fa-file"></i>';
}

// ── 内容サマリー（一覧用・1行） ──
function buildDetailSummary(req) {
  const d = req.detail || {};

  if (req.type === 'purchase') {
    return `<span class="appr-summary">
      ${d.brand || '—'} ${d.model || ''}
      <span class="appr-summary-price">${formatPrice(d.price)}</span>
    </span>`;
  }
  if (req.type === 'purchase_edit') {
    const diffPrice = (d.after?.purchasePrice || 0) - (d.before?.purchasePrice || 0);
    const diffStr = diffPrice !== 0
      ? `<span class="appr-summary-price" style="color:${diffPrice > 0 ? '#e07b39' : '#2980b9'};">${diffPrice > 0 ? '+' : ''}${formatPrice(diffPrice)}</span>`
      : '';
    return `<span class="appr-summary">
      ${d.brand || '—'} ${d.model || ''} / 伝票: <code style="font-size:11px;">${d.slipId || '—'}</code> ${diffStr}
    </span>`;
  }
  if (req.type === 'shipping' || req.type === 'shipping_edit') {
    const destName = getBuyerName(d.destination);
    const total = d.after?.total || d.total || 0;
    const slipId = d.slipId || d.shipId || '—';
    return `<span class="appr-summary">
      ${destName} 宛 / 伝票: <code style="font-size:11px;">${slipId}</code>
      <span class="appr-summary-price">${formatSalePrice(total)}</span>
    </span>`;
  }
  if (req.type === 'sales' || req.type === 'sales_edit') {
    const buyerName = getBuyerName(d.buyer);
    const total = d.after?.total || d.total || 0;
    const count = Array.isArray(d.items) ? d.items.length : '—';
    const slipId = d.slipId || '—';
    return `<span class="appr-summary">
      ${buyerName} / 伝票: <code style="font-size:11px;">${slipId}</code> ${count !== '—' ? `/ ${count}点` : ''}
      <span class="appr-summary-price">${formatSalePrice(total)}</span>
    </span>`;
  }
  if (req.type === 'purchase_return') {
    const supplierName = getSupplierName(d.supplier);
    return `<span class="appr-summary">
      ${d.brand||'—'} ${d.model||''} / 仕入先: ${supplierName}
      <span class="appr-summary-price">${formatPrice(d.price||0)}</span>
    </span>`;
  }
  if (req.type === 'sales_return') {
    const buyerName = getBuyerName(d.buyer);
    const count = Array.isArray(d.items) ? d.items.length : '—';
    return `<span class="appr-summary">
      ${buyerName} / 元伝票: <code style="font-size:11px;">${d.slipId||'—'}</code> ${count !== '—' ? `/ ${count}点` : ''}
      <span class="appr-summary-price">${formatSalePrice(d.total||0)}</span>
    </span>`;
  }
  if (req.type === 'master') {
    return `<span class="appr-summary">
      ${d.masterLabel || 'マスタ'} — ${d.action || ''}：<b>${d.data?.name || d.data?.code || '—'}</b>
    </span>`;
  }
  if (req.type === 'performance') {
    const diffAmt = (d.after?.amount || 0) - (d.before?.amount || 0);
    const diffStr = diffAmt !== 0
      ? `<span class="appr-summary-price" style="color:${diffAmt > 0 ? '#2980b9' : '#dc2626'};">${diffAmt > 0 ? '+' : ''}${formatPrice(diffAmt)}</span>`
      : '';
    return `<span class="appr-summary">
      ${d.targetMonth || '—'} / ${d.targetField || '実績'} ${diffStr}
    </span>`;
  }
  if (req.type === 'return_to_stock') {
    const count = Array.isArray(d.items) ? d.items.length : '—';
    return `<span class="appr-summary">
      元伝票: <code style="font-size:11px;">${d.saleId || '—'}</code> / ${count}点を在庫戻し
    </span>`;
  }
  if (req.type === 'stocktake_mismatch') {
    const fmt = typeof formatPrice === 'function' ? formatPrice : v => `¥${Number(v).toLocaleString('ja-JP')}`;
    return `<span class="appr-summary">
      棚卸不一致 / ${d.brand||'—'} ${d.model||''} / 理由: <b>${d.reason||'—'}</b>
      <span class="appr-summary-price">${fmt(d.price||0)}</span>
    </span>`;
  }
  return `<span class="appr-summary">${d.reason || JSON.stringify(d)}</span>`;
}

// =====================================================
// 承認詳細モーダル
// =====================================================
function openApprovalDetail(reqId) {
  const req = APP_DATA.approvalRequests.find(r => r.id === reqId);
  if (!req) return;

  const modal = document.getElementById('approvalDetailOverlay');
  if (!modal) return;

  // タイトル
  document.getElementById('apprDetailTitle').textContent = `${req.typeLabel} — 承認リクエスト詳細`;
  document.getElementById('apprDetailIcon').innerHTML = typeIcon(req.type);

  // ボディ
  const body = document.getElementById('apprDetailBody');
  body.innerHTML = buildDetailModalBody(req);

  // フッター（ステータスに応じたボタン）
  const footer = document.getElementById('apprDetailFooter');
  footer.innerHTML = buildDetailModalFooter(req);

  modal.classList.remove('hidden');
}

function closeApprovalDetail() {
  const modal = document.getElementById('approvalDetailOverlay');
  if (modal) modal.classList.add('hidden');
}

// ── 詳細モーダル ボディ ──
function buildDetailModalBody(req) {
  const statusBadge = {
    pending:  '<span class="badge-approval pending"><i class="fa-solid fa-clock"></i> 保留中</span>',
    approved: '<span class="badge-approval approved"><i class="fa-solid fa-circle-check"></i> 承認済</span>',
    revision: '<span class="badge-approval revision"><i class="fa-solid fa-rotate-left"></i> 差戻し</span>',
  }[req.status] || req.status;

  // 共通ヘッダー情報
  let html = `
    <div class="appr-detail-meta">
      <div class="appr-meta-row">
        <span class="appr-meta-label"><i class="fa-solid fa-hashtag"></i> リクエストID</span>
        <span class="appr-meta-val"><code>${req.id}</code></span>
      </div>
      <div class="appr-meta-row">
        <span class="appr-meta-label"><i class="fa-solid fa-calendar-days"></i> 申請日時</span>
        <span class="appr-meta-val">${req.createdAt}</span>
      </div>
      <div class="appr-meta-row">
        <span class="appr-meta-label"><i class="fa-solid fa-user"></i> 申請者</span>
        <span class="appr-meta-val"><b>${req.buyerName}</b></span>
      </div>
      <div class="appr-meta-row">
        <span class="appr-meta-label"><i class="fa-solid fa-tag"></i> 種別</span>
        <span class="appr-meta-val">${typeIcon(req.type)} ${req.typeLabel}</span>
      </div>
      <div class="appr-meta-row">
        <span class="appr-meta-label"><i class="fa-solid fa-circle-dot"></i> ステータス</span>
        <span class="appr-meta-val">${statusBadge}</span>
      </div>
      ${req.note ? `
      <div class="appr-meta-row">
        <span class="appr-meta-label"><i class="fa-solid fa-comment"></i> 備考</span>
        <span class="appr-meta-val">${req.note}</span>
      </div>` : ''}
      ${req.status === 'revision' && req.revisionComment ? `
      <div class="appr-meta-row" style="background:#fff8f0;border-radius:6px;padding:8px 12px;">
        <span class="appr-meta-label" style="color:#e07b39;"><i class="fa-solid fa-rotate-left"></i> 差戻しコメント</span>
        <span class="appr-meta-val" style="color:#e07b39;">${req.revisionComment}</span>
      </div>` : ''}
      ${req.status === 'revision' && Array.isArray(req.revisionHistory) && req.revisionHistory.length > 1 ? `
      <div class="appr-meta-row" style="background:#fff8f0;border-radius:6px;padding:8px 12px;">
        <span class="appr-meta-label" style="color:#e07b39;"><i class="fa-solid fa-clock-rotate-left"></i> 差戻し回数</span>
        <span class="appr-meta-val" style="color:#e07b39;font-weight:bold;">${req.revisionHistory.length}回</span>
      </div>` : ''}
    </div>
    <hr class="appr-detail-divider">
    <div class="appr-detail-content-title">
      <i class="fa-solid fa-list-check"></i> 申請内容
    </div>
  `;

  // 種別ごとの詳細
  if (req.type === 'purchase') {
    html += buildPurchaseDetail(req.detail);
  } else if (req.type === 'purchase_edit') {
    html += buildSlipEditDetail(req, 'purchase');
  } else if (req.type === 'shipping') {
    html += buildShippingDetail(req.detail);
  } else if (req.type === 'shipping_edit') {
    html += buildSlipEditDetail(req, 'shipping');
  } else if (req.type === 'sales') {
    html += buildSalesDetail(req.detail);
  } else if (req.type === 'sales_edit') {
    html += buildSlipEditDetail(req, 'sales');
  } else if (req.type === 'purchase_return') {
    html += buildPurchaseReturnApprDetail(req.detail);
  } else if (req.type === 'sales_return') {
    html += buildSalesReturnApprDetail(req.detail);
  } else if (req.type === 'master') {
    html += buildMasterDetail(req.detail);
  } else if (req.type === 'performance') {
    html += buildPerformanceDetail(req.detail);
  } else if (req.type === 'return_to_stock') {
    html += buildReturnToStockDetail(req);
  } else if (req.type === 'stocktake_mismatch') {
    html += buildStocktakeMismatchDetail(req.detail);
  } else {
    html += `<pre style="font-size:12px;background:#f5f5f5;padding:12px;border-radius:6px;">${JSON.stringify(req.detail, null, 2)}</pre>`;
  }

  return html;
}

// ── 仕入登録 詳細 ──
function buildPurchaseDetail(detail) {
  const inv = APP_DATA.inventory.find(i => i.code === detail.code);

  return `
    <div class="appr-content-card">
      <div class="appr-content-grid">
        <div class="appr-content-item">
          <span class="appr-content-label">商品コード</span>
          <span class="appr-content-val"><code>${detail.code || '—'}</code></span>
        </div>
        <div class="appr-content-item">
          <span class="appr-content-label">ブランド</span>
          <span class="appr-content-val"><b>${detail.brand || '—'}</b></span>
        </div>
        <div class="appr-content-item">
          <span class="appr-content-label">モデル名</span>
          <span class="appr-content-val">${detail.model || '—'}</span>
        </div>
        <div class="appr-content-item">
          <span class="appr-content-label">仕入金額（税抜）</span>
          <span class="appr-content-val appr-price">${formatPrice(detail.price)}</span>
        </div>
        ${detail.ref ? `<div class="appr-content-item">
          <span class="appr-content-label">Ref番号</span>
          <span class="appr-content-val">${detail.ref}</span>
        </div>` : ''}
        ${detail.serial ? `<div class="appr-content-item">
          <span class="appr-content-label">シリアル番号</span>
          <span class="appr-content-val">${detail.serial}</span>
        </div>` : ''}
        ${detail.supplier ? `<div class="appr-content-item">
          <span class="appr-content-label">仕入先</span>
          <span class="appr-content-val">${getSupplierName(detail.supplier)}</span>
        </div>` : ''}
        ${detail.purchaseDate ? `<div class="appr-content-item">
          <span class="appr-content-label">仕入日</span>
          <span class="appr-content-val">${detail.purchaseDate}</span>
        </div>` : ''}
        ${detail.condition ? `<div class="appr-content-item">
          <span class="appr-content-label">コンディション</span>
          <span class="appr-content-val">${APP_DATA.conditions.find(c=>c.code===detail.condition)?.name || detail.condition}</span>
        </div>` : ''}
      </div>
      ${detail.note ? `<div style="margin-top:12px;padding:10px;background:#f8fafc;border-radius:6px;font-size:12px;color:var(--text-muted);">
        <i class="fa-solid fa-note-sticky"></i> ${detail.note}
      </div>` : ''}
    </div>
  `;
}

// ── 出荷登録 詳細 ──
function buildShippingDetail(detail) {
  const destName = getBuyerName(detail.destination);
  const buyer = APP_DATA.buyers.find(b => b.code === detail.destination);
  const items = Array.isArray(detail.items) ? detail.items : [];
  const displayCurrency = detail.displayCurrency === 'JPY' || detail.inputCurrency === 'JPY' ? 'JPY' : 'USD';
  const rate = Number(detail.usdJpyRate) > 0 ? Number(detail.usdJpyRate)
    : (typeof getSalesUsdRate === 'function' ? getSalesUsdRate() : 155);

  // itemsが文字列コード配列の場合と詳細オブジェクト配列の両対応
  const itemRows = items.map(it => {
    if (typeof it === 'string') {
      const inv = APP_DATA.inventory.find(i => i.code === it);
      const salePriceUSD = Number(inv?.salePrice) || 0;
      return { code: it, brand: inv?.brand || '—', model: inv?.model || '—', salePriceUSD,
        displayPrice: displayCurrency === 'JPY' && typeof convertShippingUSDToJPY === 'function'
          ? convertShippingUSDToJPY(salePriceUSD, rate) : salePriceUSD };
    }
    return {
      code: it.code || '—',
      brand: it.brand || '—',
      model: it.model || '—',
      salePriceUSD: typeof getShippingSalePriceUSD === 'function'
        ? getShippingSalePriceUSD(it)
        : (Number(it.salePriceUsd) || Number(it.salePrice) || 0),
      displayPrice: displayCurrency === 'JPY'
        ? (Number(it.convertedSalePriceJpy) || (typeof convertShippingUSDToJPY === 'function'
          ? convertShippingUSDToJPY(Number(it.salePriceUsd) || Number(it.salePrice) || 0, rate) : 0))
        : (Number(it.salePriceUsd) || Number(it.salePrice) || 0),
    };
  });

  const totalPrice = itemRows.reduce((s, i) => s + (i.displayPrice || 0), 0);
  const formatAmount = amount => displayCurrency === 'JPY' ? formatPrice(amount) : formatSalePrice(amount);

  return `
    <div class="appr-content-card">
      <div class="appr-content-grid">
        <div class="appr-content-item">
          <span class="appr-content-label">出荷先</span>
          <span class="appr-content-val"><b>${destName}</b></span>
        </div>
        ${buyer?.address ? `<div class="appr-content-item">
          <span class="appr-content-label">住所</span>
          <span class="appr-content-val">${buyer.address}</span>
        </div>` : ''}
        ${buyer?.contact ? `<div class="appr-content-item">
          <span class="appr-content-label">連絡先</span>
          <span class="appr-content-val">${buyer.contact}</span>
        </div>` : ''}
        <div class="appr-content-item">
          <span class="appr-content-label">出荷伝票番号</span>
          <span class="appr-content-val"><code>${detail.shipId || '—'}</code></span>
        </div>
        <div class="appr-content-item">
          <span class="appr-content-label">合計金額（${displayCurrency}）</span>
          <span class="appr-content-val appr-price">${formatAmount(totalPrice)}</span>
        </div>
      </div>

      ${itemRows.length > 0 ? `
      <div class="appr-items-section">
        <div class="appr-items-title"><i class="fa-solid fa-boxes-stacked"></i> 出荷商品明細（${itemRows.length}点）</div>
        <table class="appr-items-table">
          <thead>
            <tr>
              <th>商品コード</th>
              <th>ブランド</th>
              <th>モデル</th>
              <th>売価（${displayCurrency}）</th>
            </tr>
          </thead>
          <tbody>
            ${itemRows.map(it => `
              <tr>
                <td><code style="font-size:11px;">${it.code}</code></td>
                <td>${it.brand}</td>
                <td>${it.model}</td>
                <td style="text-align:right;font-weight:bold;">${formatAmount(it.displayPrice)}</td>
              </tr>
            `).join('')}
            <tr class="appr-items-total">
              <td colspan="3" style="text-align:right;">合計</td>
              <td style="text-align:right;">${formatAmount(totalPrice)}</td>
            </tr>
          </tbody>
        </table>
      </div>
      ` : ''}
    </div>
  `;
}

// ── 売上伝票確定 詳細 ──
function buildSalesDetail(detail) {
  const buyerName = getBuyerName(detail.buyer);
  const items = Array.isArray(detail.items) ? detail.items : [];
  const total = detail.total || items.reduce((s, it) => s + (it.salePrice || 0), 0);

  return `
    <div class="appr-content-card">
      <div class="appr-content-grid">
        <div class="appr-content-item">
          <span class="appr-content-label">伝票番号</span>
          <span class="appr-content-val"><code>${detail.slipId || '—'}</code></span>
        </div>
        <div class="appr-content-item">
          <span class="appr-content-label">販売先</span>
          <span class="appr-content-val"><b>${buyerName}</b></span>
        </div>
        <div class="appr-content-item">
          <span class="appr-content-label">売上日</span>
          <span class="appr-content-val">${detail.date || detail.saleDate || '—'}</span>
        </div>
        <div class="appr-content-item">
          <span class="appr-content-label">合計金額（USD）</span>
          <span class="appr-content-val appr-price">${formatSalePrice(total)}</span>
        </div>
        ${detail.requestedBy ? `
        <div class="appr-content-item">
          <span class="appr-content-label">申請者</span>
          <span class="appr-content-val"><i class="fa-solid fa-user" style="font-size:10px;margin-right:3px;color:var(--text-muted);"></i>${detail.requestedBy}</span>
        </div>` : ''}
      </div>
      ${items.length > 0 ? `
      <div class="appr-items-section">
        <div class="appr-items-title"><i class="fa-solid fa-list"></i> 売上明細（${items.length}点）</div>
        <table class="appr-items-table">
          <thead><tr><th>商品コード</th><th>ブランド</th><th>モデル</th><th>販売金額（USD）</th></tr></thead>
          <tbody>
            ${items.map(it => `
              <tr>
                <td><code style="font-size:11px;">${it.code || '—'}</code></td>
                <td>${it.brand || '—'}</td>
                <td>${it.model || '—'}</td>
                <td style="text-align:right;font-weight:bold;">${formatSalePrice(it.salePrice || 0)}</td>
              </tr>
            `).join('')}
            <tr class="appr-items-total">
              <td colspan="3" style="text-align:right;">合計</td>
              <td style="text-align:right;">${formatSalePrice(total)}</td>
            </tr>
          </tbody>
        </table>
      </div>` : ''}
      ${detail.note ? `<div style="margin-top:10px;padding:10px;background:#f8fafc;border-radius:6px;font-size:12px;color:var(--text-muted);"><i class="fa-solid fa-note-sticky"></i> ${detail.note}</div>` : ''}
    </div>
  `;
}

// ── 伝票修正（仕入 / 出荷 / 売上） 詳細 ──
function buildSlipEditDetail(req, category) {
  const d = req.detail || {};
  const before = d.before || {};
  const after  = d.after  || {};

  // 変更項目リスト
  const allKeys = Array.from(new Set([...Object.keys(before), ...Object.keys(after)]));
  const labelMap = {
    purchasePrice: '仕入金額', salePrice: '販売金額（USD）', total: '合計金額',
    condition: 'コンディション', serial: 'シリアル番号', ref: '型番',
    purchaseDate: '仕入日', shipDate: '出荷日', saleDate: '売上日',
    note: '備考', items: '明細', supplier: '仕入先', destination: '出荷先', buyer: '販売先',
  };

  const rows = allKeys.map(key => {
    let bVal = before[key];
    let aVal = after[key];
    if (key === 'purchasePrice') {
      bVal = formatPrice(bVal || 0);
      aVal = formatPrice(aVal || 0);
    } else if (key === 'salePrice' || (key === 'total' && category !== 'purchase')) {
      bVal = formatSalePrice(bVal || 0);
      aVal = formatSalePrice(aVal || 0);
    } else if (key === 'total') {
      bVal = formatPrice(bVal || 0);
      aVal = formatPrice(aVal || 0);
    }
    if (key === 'condition') {
      bVal = APP_DATA.conditions?.find(c => c.code === bVal)?.name || bVal || '—';
      aVal = APP_DATA.conditions?.find(c => c.code === aVal)?.name || aVal || '—';
    }
    if (Array.isArray(bVal)) bVal = `${bVal.length}点`;
    if (Array.isArray(aVal)) aVal = `${aVal.length}点`;
    const changed = JSON.stringify(before[key]) !== JSON.stringify(after[key]);
    return `
      <tr${changed ? ' style="background:#fffbeb;"' : ''}>
        <td style="font-size:12px;color:var(--text-muted);white-space:nowrap;">${labelMap[key] || key}</td>
        <td style="font-size:12px;text-decoration:${changed ? 'line-through' : 'none'};color:${changed ? '#9ca3af' : 'inherit'};">${bVal ?? '—'}</td>
        <td style="font-size:12px;font-weight:${changed ? 'bold' : 'normal'};color:${changed ? 'var(--primary)' : 'inherit'};">${aVal ?? '—'}</td>
      </tr>`;
  }).join('');

  const headerLabel = { purchase: '仕入伝票', shipping: '出荷伝票', sales: '売上伝票' }[category] || '伝票';
  const slipId = d.slipId || d.shipId || '—';

  return `
    <div class="appr-content-card">
      <div class="appr-content-grid" style="margin-bottom:12px;">
        <div class="appr-content-item">
          <span class="appr-content-label">${headerLabel}番号</span>
          <span class="appr-content-val"><code>${slipId}</code></span>
        </div>
        ${d.brand ? `<div class="appr-content-item">
          <span class="appr-content-label">商品</span>
          <span class="appr-content-val"><b>${d.brand}</b> ${d.model || ''}</span>
        </div>` : ''}
        ${d.supplier ? `<div class="appr-content-item">
          <span class="appr-content-label">仕入先</span>
          <span class="appr-content-val">${getSupplierName(d.supplier)}</span>
        </div>` : ''}
        ${d.destination ? `<div class="appr-content-item">
          <span class="appr-content-label">出荷先</span>
          <span class="appr-content-val">${getBuyerName(d.destination)}</span>
        </div>` : ''}
      </div>
      <div class="appr-items-section">
        <div class="appr-items-title"><i class="fa-solid fa-arrows-left-right"></i> 変更内容（黄色行が修正箇所）</div>
        <table class="appr-items-table">
          <thead><tr><th>項目</th><th>修正前</th><th>修正後</th></tr></thead>
          <tbody>${rows}</tbody>
        </table>
      </div>
      ${d.reason ? `<div style="margin-top:10px;padding:10px;background:#f0f9ff;border-radius:6px;font-size:12px;">
        <i class="fa-solid fa-circle-info" style="color:#2980b9;"></i> <b>修正理由：</b>${d.reason}
      </div>` : ''}
    </div>
  `;
}

// ── マスタ登録 詳細 ──
function buildMasterDetail(detail) {
  const data = detail.data || {};
  const fieldMap = {
    code: 'コード', name: '名称', contact: '連絡先', address: '住所',
    note: '備考', price: '単価', category: 'カテゴリ',
  };
  const rows = Object.entries(data).map(([k, v]) =>
    `<tr>
      <td style="font-size:12px;color:var(--text-muted);white-space:nowrap;">${fieldMap[k] || k}</td>
      <td style="font-size:12px;font-weight:bold;">${v || '—'}</td>
    </tr>`
  ).join('');

  return `
    <div class="appr-content-card">
      <div class="appr-content-grid" style="margin-bottom:12px;">
        <div class="appr-content-item">
          <span class="appr-content-label">マスタ種別</span>
          <span class="appr-content-val"><b>${detail.masterLabel || detail.masterType || '—'}</b></span>
        </div>
        <div class="appr-content-item">
          <span class="appr-content-label">操作</span>
          <span class="appr-content-val">${detail.action || '—'}</span>
        </div>
      </div>
      <div class="appr-items-section">
        <div class="appr-items-title"><i class="fa-solid fa-table-cells"></i> 登録内容</div>
        <table class="appr-items-table">
          <thead><tr><th>項目</th><th>値</th></tr></thead>
          <tbody>${rows}</tbody>
        </table>
      </div>
    </div>
  `;
}

// ── 実績管理 詳細 ──
function buildPerformanceDetail(detail) {
  const before = detail.before || {};
  const after  = detail.after  || {};
  const diffAmt   = (after.amount || 0) - (before.amount || 0);
  const diffCount = (after.count  || 0) - (before.count  || 0);

  return `
    <div class="appr-content-card">
      <div class="appr-content-grid" style="margin-bottom:12px;">
        <div class="appr-content-item">
          <span class="appr-content-label">対象月</span>
          <span class="appr-content-val"><b>${detail.targetMonth || '—'}</b></span>
        </div>
        <div class="appr-content-item">
          <span class="appr-content-label">修正対象</span>
          <span class="appr-content-val">${detail.targetField || '—'}</span>
        </div>
      </div>
      <div class="appr-items-section">
        <div class="appr-items-title"><i class="fa-solid fa-arrows-left-right"></i> 変更内容</div>
        <table class="appr-items-table">
          <thead><tr><th>項目</th><th>修正前</th><th>修正後</th><th>差分</th></tr></thead>
          <tbody>
            <tr style="background:#fffbeb;">
              <td style="font-size:12px;color:var(--text-muted);">売上金額</td>
              <td style="font-size:12px;text-decoration:line-through;color:#9ca3af;">${formatPrice(before.amount || 0)}</td>
              <td style="font-size:12px;font-weight:bold;color:var(--primary);">${formatPrice(after.amount || 0)}</td>
              <td style="font-size:12px;font-weight:bold;color:${diffAmt >= 0 ? '#2980b9' : '#dc2626'};">${diffAmt >= 0 ? '+' : ''}${formatPrice(diffAmt)}</td>
            </tr>
            <tr style="background:#fffbeb;">
              <td style="font-size:12px;color:var(--text-muted);">件数</td>
              <td style="font-size:12px;text-decoration:line-through;color:#9ca3af;">${before.count || 0}件</td>
              <td style="font-size:12px;font-weight:bold;color:var(--primary);">${after.count || 0}件</td>
              <td style="font-size:12px;font-weight:bold;color:${diffCount >= 0 ? '#2980b9' : '#dc2626'};">${diffCount >= 0 ? '+' : ''}${diffCount}件</td>
            </tr>
          </tbody>
        </table>
      </div>
      ${detail.reason ? `<div style="margin-top:10px;padding:10px;background:#f0f9ff;border-radius:6px;font-size:12px;">
        <i class="fa-solid fa-circle-info" style="color:#2980b9;"></i> <b>修正理由：</b>${detail.reason}
      </div>` : ''}
    </div>
  `;
}

// ── 詳細モーダル フッター（ボタン） ──
function buildDetailModalFooter(req) {
  const closeBtn = `<button class="btn btn-outline" onclick="closeApprovalDetail()">閉じる</button>`;

  if (req.status === 'pending' && isAdmin()) {
    // return_to_stock は承認と同時にBOX振り分けモーダルを開く
    if (req.type === 'return_to_stock') {
      return `
        ${closeBtn}
        <button class="btn btn-warning" onclick="closeApprovalDetail();openReviseModal('${req.id}')">
          <i class="fa-solid fa-rotate-left"></i> 差戻し
        </button>
        <button class="btn btn-success" onclick="approveReturnToStock('${req.id}')">
          <i class="fa-solid fa-boxes-stacked"></i> 承認してBOX振り分け
        </button>
      `;
    }
    return `
      ${closeBtn}
      <button class="btn btn-warning" onclick="closeApprovalDetail();openReviseModal('${req.id}')">
        <i class="fa-solid fa-rotate-left"></i> 差戻し
      </button>
      <button class="btn btn-success" onclick="approveRequest('${req.id}')">
        <i class="fa-solid fa-check"></i> 承認する
      </button>
    `;
  }
  return closeBtn;
}

// =====================================================
// 差戻しモーダル
// =====================================================
function openReviseModal(reqId) {
  const req = APP_DATA.approvalRequests.find(r => r.id === reqId);
  if (!req) return;
  const modal = document.getElementById('reviseModalOverlay');
  if (!modal) return;
  document.getElementById('reviseReqId').value = reqId;
  document.getElementById('reviseReqInfo').textContent = `${req.typeLabel} / ${req.buyerName} / ${req.createdAt}`;
  document.getElementById('reviseComment').value = '';
  modal.classList.remove('hidden');
}

function closeReviseModal() {
  const modal = document.getElementById('reviseModalOverlay');
  if (modal) modal.classList.add('hidden');
}

function submitRevise() {
  const reqId = document.getElementById('reviseReqId').value;
  const comment = document.getElementById('reviseComment').value.trim();
  if (!comment) { showToast('error', 'コメントを入力してください', ''); return; }
  reviseRequest(reqId, comment);
  closeReviseModal();
}

// ── 承認バッジ更新 ──
function updateApprovalBadge() {
  const pending = APP_DATA.approvalRequests.filter(r => r.status === 'pending').length;
  const badge = document.getElementById('approvalBadge');
  if (!badge) return;
  badge.textContent = pending;
  badge.style.display = pending > 0 ? 'inline-flex' : 'none';
}

// ── OTP入力モーダル ──
function openOTPModal(reqId, onSuccess) {
  const modal = document.getElementById('otpModalOverlay');
  if (!modal) return;
  document.getElementById('otpReqId').value = reqId;
  document.getElementById('otpInput').value = '';
  document.getElementById('otpError').classList.add('hidden');
  modal.classList.remove('hidden');

  document.getElementById('otpConfirmBtn').onclick = function() {
    const input = document.getElementById('otpInput').value.trim();
    const result = verifyOTP(reqId, input);
    if (result.ok) {
      modal.classList.add('hidden');
      if (onSuccess) onSuccess();
    } else {
      const errEl = document.getElementById('otpError');
      errEl.textContent = result.msg;
      errEl.classList.remove('hidden');
    }
  };

  document.getElementById('otpCancelBtn').onclick = function() {
    modal.classList.add('hidden');
  };
}

// ── 作業者承認申請 ──
// existingId: 差戻し後の再申請時に既存IDを維持するために渡す（⑤）
function requestApproval(type, typeLabel, detail, note, onApproved, existingId = null) {
  if (isAdmin()) {
    if (onApproved) onApproved();
    return null;
  }
  const req = createApprovalRequest(type, typeLabel, detail, note, existingId);
  if (!req) return null;
  if (existingId) {
    showToast('info', '再申請を送信しました', `申請ID ${existingId} で再申請しました`);
  } else {
    showToast('info', '承認申請を送信しました', '管理者が確認後に処理されます');
  }
  return req;
}

// ── 仕入返品起票 申請内容詳細 ──
function buildPurchaseReturnApprDetail(d) {
  if (!d) return '<div class="appr-content-card"><p style="color:var(--text-muted);">詳細情報なし</p></div>';
  return `
    <div class="appr-content-card">
      <div class="appr-content-grid">
        <div class="appr-content-item">
          <span class="appr-content-label">返品伝票番号</span>
          <span class="appr-content-val"><code>${d.retId||'—'}</code></span>
        </div>
        <div class="appr-content-item">
          <span class="appr-content-label">商品コード</span>
          <span class="appr-content-val"><code>${d.code||'—'}</code></span>
        </div>
        <div class="appr-content-item">
          <span class="appr-content-label">ブランド</span>
          <span class="appr-content-val"><b>${d.brand||'—'}</b></span>
        </div>
        <div class="appr-content-item">
          <span class="appr-content-label">モデル</span>
          <span class="appr-content-val">${d.model||'—'}</span>
        </div>
        <div class="appr-content-item">
          <span class="appr-content-label">仕入先</span>
          <span class="appr-content-val">${getSupplierName(d.supplier)}</span>
        </div>
        <div class="appr-content-item">
          <span class="appr-content-label">仕入金額</span>
          <span class="appr-content-val" style="font-weight:bold;color:var(--primary);">${formatPrice(d.price||0)}</span>
        </div>
        <div class="appr-content-item">
          <span class="appr-content-label">返品日</span>
          <span class="appr-content-val">${d.date||'—'}</span>
        </div>
        <div class="appr-content-item">
          <span class="appr-content-label">備考</span>
          <span class="appr-content-val">${d.note||d.reason||'—'}</span>
        </div>
      </div>
    </div>`;
}

// ── 売上返品起票 申請内容詳細 ──
function buildSalesReturnApprDetail(d) {
  if (!d) return '<div class="appr-content-card"><p style="color:var(--text-muted);">詳細情報なし</p></div>';
  const itemRows = (d.items||[]).map(it => `
    <tr>
      <td><code style="font-size:11px;">${it.code||'—'}</code></td>
      <td>${it.brand||'—'}</td>
      <td>${it.model||'—'}</td>
      <td style="font-size:11px;color:var(--text-muted);">${it.ref||'—'}</td>
      <td style="font-size:11px;color:var(--text-muted);">${it.serial||'—'}</td>
      <td style="text-align:right;font-weight:bold;">${formatSalePrice(it.salePrice||0)}</td>
    </tr>`).join('');
  return `
    <div class="appr-content-card">
      <div class="appr-content-grid">
        <div class="appr-content-item">
          <span class="appr-content-label">返品伝票番号</span>
          <span class="appr-content-val"><code>${d.retId||'—'}</code></span>
        </div>
        <div class="appr-content-item">
          <span class="appr-content-label">元売上伝票</span>
          <span class="appr-content-val"><code>${d.slipId||'—'}</code></span>
        </div>
        <div class="appr-content-item">
          <span class="appr-content-label">販売先</span>
          <span class="appr-content-val"><b>${getBuyerName(d.buyer)}</b></span>
        </div>
        <div class="appr-content-item">
          <span class="appr-content-label">返品合計（USD）</span>
          <span class="appr-content-val" style="font-weight:bold;color:#7c3aed;">${formatSalePrice(d.total||0)}</span>
        </div>
        <div class="appr-content-item">
          <span class="appr-content-label">返品日</span>
          <span class="appr-content-val">${d.date||'—'}</span>
        </div>
        <div class="appr-content-item">
          <span class="appr-content-label">返品理由</span>
          <span class="appr-content-val">${d.reason||'—'}</span>
        </div>
      </div>
      ${d.note ? `<div class="appr-content-note"><i class="fa-solid fa-note-sticky"></i> ${d.note}</div>` : ''}
      ${itemRows ? `
        <hr style="margin:12px 0;border:none;border-top:1px solid var(--border);">
        <div style="font-size:12px;font-weight:700;color:var(--text-muted);margin-bottom:8px;">
          <i class="fa-solid fa-list-check"></i> 返品商品（${(d.items||[]).length}点）
        </div>
        <table class="appr-items-table">
          <thead><tr><th>商品コード</th><th>ブランド</th><th>モデル</th><th>型番</th><th>シリアル</th><th style="text-align:right;">売上金額</th></tr></thead>
          <tbody>${itemRows}</tbody>
          <tr class="appr-items-total">
            <td colspan="5" style="text-align:right;">返品合計</td>
            <td style="text-align:right;color:#7c3aed;">${formatPrice(d.total||0)}</td>
          </tr>
        </table>` : ''}
    </div>`;
}

// ── 棚卸不一致 申請内容詳細 ──
function buildStocktakeMismatchDetail(d) {
  if (!d) return '<div class="appr-content-card"><p style="color:var(--text-muted);">詳細情報なし</p></div>';
  const fmt = typeof formatPrice === 'function' ? formatPrice : v => `¥${Number(v).toLocaleString('ja-JP')}`;
  const reasonColor = {
    '紛失': '#dc2626', '返品忘れ': '#d97706', '破損': '#9333ea', '理由不明': '#64748b',
  }[d.reason] || '#333';
  return `
    <div class="appr-content-card">
      <div class="appr-content-grid">
        <div class="appr-content-item">
          <span class="appr-content-label">商品コード</span>
          <span class="appr-content-val"><code>${d.code||'—'}</code></span>
        </div>
        <div class="appr-content-item">
          <span class="appr-content-label">ブランド</span>
          <span class="appr-content-val"><b>${d.brand||'—'}</b></span>
        </div>
        <div class="appr-content-item">
          <span class="appr-content-label">モデル名</span>
          <span class="appr-content-val">${d.model||'—'}</span>
        </div>
        <div class="appr-content-item">
          <span class="appr-content-label">仕入金額</span>
          <span class="appr-content-val" style="font-weight:bold;color:var(--primary);">${fmt(d.price||0)}</span>
        </div>
        <div class="appr-content-item">
          <span class="appr-content-label">ステータス</span>
          <span class="appr-content-val">${d.status||'—'}</span>
        </div>
        <div class="appr-content-item">
          <span class="appr-content-label">不一致理由</span>
          <span class="appr-content-val" style="font-weight:bold;color:${reasonColor};">
            <i class="fa-solid fa-triangle-exclamation"></i> ${d.reason||'—'}
          </span>
        </div>
        <div class="appr-content-item">
          <span class="appr-content-label">申請者</span>
          <span class="appr-content-val">${d.requestedBy||'—'}</span>
        </div>
      </div>
      ${d.note ? `<div class="appr-content-note"><i class="fa-solid fa-note-sticky"></i> ${d.note}</div>` : ''}
    </div>`;
}

// ── 返品/持ち帰り 在庫戻し 申請内容詳細 ──
function buildReturnToStockDetail(req) {
  const items = req.detail?.items || [];
  const saleId = req.detail?.saleId || '—';
  const boxOpts = `<option value="">振り分けなし</option>` +
    [1,2,3,4,5,6,7,8,9,10].map(n => {
      const box = (APP_DATA.boxes || []).find(b => b.no === n);
      return `<option value="${n}">BOX ${n}${box ? ` — ${box.name}` : ''}</option>`;
    }).join('');

  const rows = items.map((it, i) => {
    const inv = APP_DATA.inventory.find(inv => inv.code === it.code);
    const condName = inv ? (APP_DATA.conditions.find(c => c.code === (it.condition || inv.condition))?.name || it.condition || inv.condition || '—') : (it.condition || '—');
    return `
      <div style="display:flex;align-items:center;gap:10px;padding:10px 0;border-bottom:1px solid var(--border);flex-wrap:wrap;">
        <div style="flex:1;min-width:0;">
          <div style="display:flex;gap:6px;align-items:center;flex-wrap:wrap;">
            <code style="font-size:12px;">${it.code}</code>
            <strong style="font-size:13px;">${it.brand || ''} ${it.model || ''}</strong>
          </div>
          <div style="font-size:11px;color:var(--text-muted);margin-top:3px;">
            コンディション: ${condName}　数量: ${it.qty || 1}
          </div>
        </div>
        ${isAdmin() && req.status === 'pending' ? `
        <div style="display:flex;gap:8px;align-items:center;flex-shrink:0;">
          <label style="font-size:12px;color:var(--text-muted);white-space:nowrap;">BOX振り分け:</label>
          <select class="form-control appr-ret-box-select" data-code="${it.code}" data-condition="${it.condition || ''}"
            style="width:180px;font-size:12px;height:30px;padding:2px 8px;">
            ${boxOpts}
          </select>
        </div>` : ''}
      </div>`;
  }).join('');

  return `
    <div class="appr-content-card">
      <div style="font-size:12px;color:var(--text-muted);margin-bottom:10px;">
        <i class="fa-solid fa-receipt"></i> 元売上伝票: <code>${saleId}</code>
      </div>
      ${rows || '<div style="color:var(--text-muted);font-size:12px;padding:10px;">商品情報なし</div>'}
      ${isAdmin() && req.status === 'pending' ? `
      <div class="form-group" style="margin-top:14px;">
        <label class="form-label">管理者コメント（任意）</label>
        <textarea class="form-control" id="appr-ret-note" rows="2" placeholder="処理内容のメモなど"></textarea>
      </div>` : ''}
    </div>`;
}

// ── 管理者：在庫戻し申請を承認 + BOX振り分け実行 ──
function approveReturnToStock(reqId) {
  const req = APP_DATA.approvalRequests.find(r => r.id === reqId);
  if (!req) return;
  if (!isAdmin()) {
    showToast('error', 'アクセス拒否', '承認は管理者のみ実行できます');
    return;
  }

  const items = req.detail?.items || [];
  const saleId = req.detail?.saleId;
  const note   = document.getElementById('appr-ret-note')?.value || '';
  const sale   = APP_DATA.sales.find(s => s.id === saleId);
  if (!sale) { showToast('error', 'エラー', '元売上伝票が見つかりません'); return; }

  // BOX選択値を収集
  const targets = items.map(it => {
    const boxEl = document.querySelector(`.appr-ret-box-select[data-code="${it.code}"]`);
    const boxNo = parseInt(boxEl?.value) || null;
    const saleItem = sale.items.find(si => si.code === it.code);
    const invItem  = APP_DATA.inventory.find(i => i.code === it.code);
    return { code: it.code, condition: it.condition || '', qty: it.qty || 1, boxNo, saleItem, invItem };
  }).filter(t => t.saleItem && t.invItem);

  if (targets.length === 0) {
    showToast('error', 'エラー', '対象商品が見つかりません');
    return;
  }

  // リクエストを承認状態に更新
  req.status = 'approved';
  req.otpUsed = true; // 管理者直接実行のためOTP不要
  req.approvedAt = new Date().toLocaleString('ja-JP');
  req.approvedById = currentUserId();
  req.approvedByName = currentUser()?.name || '管理者';

  // 在庫戻し実行（app.jsの共通関数を呼び出す）
  if (typeof _execReturnToStock === 'function') {
    _execReturnToStock(saleId, targets, currentUser()?.name || '管理者', note);
  }

  // 作業者に通知
  createNotification({
    toUserId:   _approvalRequesterId(req),
    fromUserId: currentUserId(),
    fromName:   currentUser()?.name || '管理者',
    type:       'approved',
    title:      `承認完了：${req.typeLabel}`,
    body:       `${req.typeLabel}の申請が承認され、在庫に戻されました。`,
    relatedId:  reqId,
  });

  updateApprovalBadge();
  _approvalPersistWorkflowState();
  _approvalRefreshUI();
  renderApprovalList();
  closeApprovalDetail();
  showToast('success', '承認・在庫戻しを実行しました', `${targets.length}件を在庫に戻しました`);
}
