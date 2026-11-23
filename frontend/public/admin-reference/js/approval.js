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
const _statusOrder = { pending: 0, revision: 1, returned: 1, approved: 2 };

// =====================================================
// 承認リクエスト一覧レンダリング
// =====================================================
function formatApprovalRequestTime(value) {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '—';
  const parts = Object.fromEntries(new Intl.DateTimeFormat('ja-JP', {
    timeZone: 'Asia/Tokyo', year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', hourCycle: 'h23',
  }).formatToParts(date).map(part => [part.type, part.value]));
  return `${parts.year}/${parts.month}/${parts.day} ${parts.hour}:${parts.minute}`;
}

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
    tbody.innerHTML = '<tr><td colspan="5" style="text-align:center;color:#aaa;padding:32px;">承認リクエストはありません</td></tr>';
    return;
  }

  tbody.innerHTML = list.map(req => {
    const statusBadge = {
      pending:  '<span class="badge-approval pending"><i class="fa-solid fa-clock"></i> 保留中</span>',
      approved: '<span class="badge-approval approved"><i class="fa-solid fa-circle-check"></i> 承認済</span>',
      revision: '<span class="badge-approval revision"><i class="fa-solid fa-rotate-left"></i> 差戻し</span>',
    returned: '<span class="badge-approval revision"><i class="fa-solid fa-rotate-left"></i> 差戻し</span>',
    }[req.status] || req.status;

    // 内容サマリー（クリック可）
    const summaryHtml = buildDetailSummary(req);

    return `<tr class="appr-row" onclick="openApprovalDetail('${req.id}')" title="クリックで詳細表示">
      <td style="white-space:nowrap;font-size:12px;">
        <i class="fa-solid fa-calendar-days" style="color:var(--text-muted);margin-right:4px;"></i>${formatApprovalRequestTime(req.createdAt)}
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
  const documentSummary=buildApprovalDocumentSummary(d);
  if(documentSummary)return documentSummary;

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
  const targetLabel = approvalTargetTypeLabel(d.targetType);
  const purchaseSlip = d.targetType === 'purchase_slip' ? findApprovalPurchaseSlip(d.targetId) : null;
  const displayTargetId = purchaseSlip?.id || d.targetId || '—';
  return `<span class="appr-summary">${d.reason || `${targetLabel}：${displayTargetId}`}</span>`;
}

// =====================================================
// 承認詳細モーダル
// =====================================================
function approvalDisplayType(req) {
  const key = String(req?.typeLabel || req?.type || '').trim();
  const labels = {
    'purchase.confirm': '仕入伝票の確定',
    'purchase.confirmed': '仕入伝票の確定',
    'sale.confirm': '売上伝票の確定',
    'return.confirm': '返品伝票の確定',
    'sales.confirm': '売上伝票の確定',
    'sales.confirmed': '売上伝票の確定',
    'consignment.confirm': '委託伝票の確定',
    'shipment.confirm': '出荷伝票の確定',
    'shipment.confirmed': '出荷伝票の確定',
    'consignment.confirm': '委託伝票の確定',
    'purchase.update': '仕入伝票の修正',
    'sales.update': '売上伝票の修正',
    'shipment.update': '出荷伝票の修正',
    'stocktake.mismatch': '棚卸不一致の確定',
  };
  return labels[key] || labels[String(req?.type || '').trim()] || key || '承認申請';
}

function approvalTargetTypeLabel(value) {
  return ({
    document_change: '伝票操作',
    purchase_slip: '仕入伝票',
    sales_slip: '売上伝票',
    shipment_slip: '出荷伝票',
    consignment_slip: '委託伝票',
    consignment_slip: '委託伝票',
    return_slip: '返品伝票',
    product: '商品',
    stocktake: '棚卸',
    stocktake_line: '棚卸明細',
  })[String(value || '')] || '対象データ';
}

function findApprovalPurchaseSlip(targetId) {
  const id = String(targetId || '');
  return (APP_DATA.purchaseSlips || []).find(record => String(record._id || '') === id || String(record.id || '') === id) || null;
}

function openApprovalPurchaseSlip(targetId) {
  const slip = findApprovalPurchaseSlip(targetId);
  if (!slip) { showToast('warning', '伝票を開けません', '対象の仕入伝票が見つかりません'); return; }
  closeApprovalDetail();
  navigateTo('sales-list');
  switchSlipTab('purchase');
  openSlipDetail('purchase', slip.id);
}

function openApprovalProductDetail(code) {
  const product = (APP_DATA.inventory || []).find(item => String(item.code) === String(code));
  if (!product) { showToast('warning', '商品を開けません', `管理番号 ${code} の商品が見つかりません`); return; }
  closeApprovalDetail();
  navigateTo('inventory');
  execInventorySearch();
  showItemDetail(product.code);
}

function buildPurchaseApprovalTargetDetail(detail) {
  const slip = findApprovalPurchaseSlip(detail.targetId);
  if (!slip) return null;
  const lines = slip.lines || [];
  return `<div class="appr-content-card">
    <div class="appr-content-grid">
      <div class="appr-content-item"><span class="appr-content-label">申請対象</span><span class="appr-content-val">仕入伝票</span></div>
      <div class="appr-content-item"><span class="appr-content-label">仕入伝票番号</span><span class="appr-content-val"><b>${slip.id || '—'}</b></span></div>
      <div class="appr-content-item"><span class="appr-content-label">仕入日</span><span class="appr-content-val">${slip.date || '—'}</span></div>
      <div class="appr-content-item"><span class="appr-content-label">仕入先</span><span class="appr-content-val">${getSupplierName(slip.supplier)}</span></div>
    </div>
    <div style="margin-top:12px;"><button type="button" class="btn btn-primary btn-sm" onclick="openApprovalPurchaseSlip('${detail.targetId}')"><i class="fa-solid fa-file-invoice"></i> 仕入伝票を開く</button></div>
    <div class="appr-items-section">
      <div class="appr-items-title"><i class="fa-solid fa-boxes-stacked"></i> 商品明細（${lines.length}点）</div>
      <div style="overflow-x:auto;"><table class="appr-items-table"><thead><tr><th>管理番号</th><th>ブランド</th><th>モデル名</th><th>型番</th><th>シリアル</th><th>詳細</th></tr></thead><tbody>
      ${lines.map(line => {
        const product = (APP_DATA.inventory || []).find(item => item.code === line.code);
        const source = product || line.productDetail || line;
        return `<tr><td>${line.code || '未発番'}</td><td>${source.brand || '—'}</td><td>${source.model || '—'}</td><td>${source.ref || '—'}</td><td>${source.serial || '—'}</td><td>${line.code ? `<button type="button" class="btn btn-outline btn-sm" onclick="openApprovalProductDetail('${line.code}')">商品詳細</button>` : '—'}</td></tr>`;
      }).join('') || '<tr><td colspan="6">商品明細がありません</td></tr>'}
      </tbody></table></div>
    </div>
  </div>`;
}

function buildReadableApprovalDetail(detail = {}) {
  if (detail.targetType === 'purchase_slip') {
    const purchaseDetail = buildPurchaseApprovalTargetDetail(detail);
    if (purchaseDetail) return purchaseDetail;
  }
  if(detail.targetType === 'stocktake_line') {
    const escape = value => String(value ?? '').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
    return '<div class="appr-content-card">'+[['管理番号',detail.productCode],['ブランド',detail.brand],['モデル',detail.modelNumber],['不一致理由',detail.reason],['備考',detail.note]].map(([label,value])=>'<p>'+label+'：'+escape(value)+'</p>').join('')+'</div>';
  }
  const targetLabel = approvalTargetTypeLabel(detail.targetType);
  const rows = [
    ['申請対象', targetLabel],
    [`${targetLabel}ID`, detail.targetId || '—'],
  ];
  if (detail.reason) rows.push(['申請理由', detail.reason]);
  return `<div class="appr-content-card"><div class="appr-content-grid">
    ${rows.map(([label, value]) => `<div class="appr-content-item">
      <span class="appr-content-label">${label}</span>
      <span class="appr-content-val">${value}</span>
    </div>`).join('')}
  </div></div>`;
}

function openApprovalDetail(reqId) {
  const req = APP_DATA.approvalRequests.find(r => r.id === reqId);
  if (!req) return;

  const modal = document.getElementById('approvalDetailOverlay');
  if (!modal) return;

  // タイトル
  document.getElementById('apprDetailTitle').textContent = `${approvalDisplayType(req)} — 承認申請の詳細`;
  document.getElementById('apprDetailIcon').innerHTML = typeIcon(req.type);

  // ボディ
  const body = document.getElementById('apprDetailBody');
  body.innerHTML = buildDetailModalBody(req);
  body.closest('.modal').style.maxWidth=approvalDocumentSnapshot(req.detail||{})?'1000px':'640px';

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
    returned: '<span class="badge-approval revision"><i class="fa-solid fa-rotate-left"></i> 差戻し</span>',
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
        <span class="appr-meta-val">${formatApprovalRequestTime(req.createdAt)}</span>
      </div>
      <div class="appr-meta-row">
        <span class="appr-meta-label"><i class="fa-solid fa-user"></i> 申請者</span>
        <span class="appr-meta-val"><b>${req.buyerName}</b></span>
      </div>
      <div class="appr-meta-row">
        <span class="appr-meta-label"><i class="fa-solid fa-tag"></i> 種別</span>
        <span class="appr-meta-val">${typeIcon(req.type)} ${approvalDisplayType(req)}</span>
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
  const documentDetail=buildApprovalDocumentDetail(req.detail||{});
  if(documentDetail){html+=documentDetail;}
  else if (req.type === 'purchase') {
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
    html += buildReadableApprovalDetail(req.detail);
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
  return closeBtn + returnedApprovalButton(req);
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
  document.getElementById('reviseReqInfo').textContent = `${req.typeLabel} / ${req.buyerName} / ${formatApprovalRequestTime(req.createdAt)}`;
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

// Approval documents are displayed from the saved application snapshot, not mutable inventory prices.
function approvalDocumentSnapshot(detail = {}) {
 const doc=detail.document;
 return ['purchase_slip','sales_slip','shipment_slip','return_slip','consignment_slip'].includes(detail.targetType)
   && doc && typeof doc==='object' && !Array.isArray(doc) && doc.slipNumber ? doc : null;
}
function approvalDocumentEscape(value) {return String(value ?? '').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));}
function approvalDocumentMoney(value,currency) {
 if(value==null || !Number.isFinite(Number(value)))return '—';
 const code=/^[A-Z]{3}$/.test(currency||'')?currency:'JPY';
 return new Intl.NumberFormat('ja-JP',{style:'currency',currency:code,maximumFractionDigits:code==='JPY'?0:2}).format(Number(value));
}
function approvalDocumentLines(detail) {
 const doc=approvalDocumentSnapshot(detail);if(!doc)return [];
 return [...(Array.isArray(doc.lines)?doc.lines:[]),...(doc.adjustmentDetails?.lines || []).map(line=>({productCode:'99999',modelNumber:line.name,quantity:0,costAmountMinor:line.amountMinor,costCurrency:doc.adjustmentDetails.currency}))].map(line=>{
  let currency,unit,amount;
  const quantity=line.quantity ?? 1;
  if(detail.targetType==='purchase_slip'){currency=line.costCurrency;unit=line.unitCostMinor;amount=unit==null?null:Number(unit)*quantity;}
  else if(['shipment_slip','consignment_slip'].includes(detail.targetType)){currency=doc.displayCurrency||'USD';unit=currency==='JPY'?line.convertedSalePriceJpy:line.salePriceUsdMinor;amount=unit;}
  else if(detail.targetType==='sales_slip'){currency=line.saleCurrency||doc.displayCurrency;unit=line.unitPriceMinor;amount=line.subtotalMinor ?? unit;}
  else {currency=line.costCurrency;unit=line.costAmountMinor;amount=unit;}
  return {line,quantity,currency,unit,amount};
 });
}
function buildApprovalDocumentSummary(detail) {
 if(detail.targetType==='document_change')return buildApprovalDocumentSummary(documentChangeDetail(detail,'before')); 
 const doc=approvalDocumentSnapshot(detail);if(!doc)return null;
 const esc=approvalDocumentEscape,lines=approvalDocumentLines(detail),sums=new Map();
 for(const row of lines){if(row.amount!=null)sums.set(row.currency||'JPY',(sums.get(row.currency||'JPY')||0)+Number(row.amount));}
 return `<span class="appr-summary"><b>伝票No：${esc(doc.slipNumber)}</b><br>${esc(doc.supplierName||doc.buyerName||doc.consigneeName||doc.recipientName||'—')} / ${lines.length}明細<br>${[...sums].map(([currency,amount])=>esc(approvalDocumentMoney(amount,currency))).join(' / ')}</span>`;
}
function buildApprovalDocumentDetail(detail) {
 const revised=buildReturnedApprovalReview(detail);if(revised)return revised;
 if(detail.targetType==='document_change')return '<h4>現在の伝票（申請時点）</h4>'+ (buildApprovalDocumentDetail(documentChangeDetail(detail,'before'))||'')+'<h4>申請内容（承認後に反映）</h4>'+documentChangeFields(detail.document?.payload||{},false)+(buildApprovalDocumentDetail(documentChangeDetail(detail,'after'))||'');
 const doc=approvalDocumentSnapshot(detail);if(!doc)return null;
 const esc=approvalDocumentEscape,lines=approvalDocumentLines(detail),purchase=detail.targetType==='purchase_slip',sale=detail.targetType==='sales_slip',shipment=detail.targetType==='shipment_slip';
 const fields=[['伝票No',doc.slipNumber],['伝票種類',approvalTargetTypeLabel(detail.targetType)],['伝票日付',doc.purchaseDate||doc.saleDate||doc.shipmentDate||doc.transactionDate||doc.consignmentDate||'—'],[purchase?'仕入先':'取引先',doc.supplierName||doc.buyerName||doc.consigneeName||doc.recipientName||'—']];
 if(doc.staffName)fields.push(['担当者',doc.staffName]);
 if(doc.paymentMethod)fields.push(['支払い方法',({cash:'現金',bank_transfer:'銀行振込',credit_card:'クレジットカード',credit:'掛け',other:'その他'})[doc.paymentMethod]||doc.paymentMethod]);
 if(doc.recipientAddress)fields.push(['送付先住所',doc.recipientAddress]);
 if(doc.sourcePurchaseSlipNumber)fields.push(['元仕入伝票No',doc.sourcePurchaseSlipNumber]);
 if(doc.reason)fields.push(['理由',doc.reason]);
 fields.push(['備考',doc.notes||'—']);
 const totals=[];
 if(sale){totals.push(['小計（税抜）',approvalDocumentMoney(doc.subtotalMinor,doc.displayCurrency)],['消費税',approvalDocumentMoney(doc.taxAmountMinor,doc.displayCurrency)],['合計（税込）',approvalDocumentMoney(doc.totalMinor,doc.displayCurrency)]);}
 else {
  const sums=new Map();for(const row of lines){if(row.amount!=null)sums.set(row.currency||'JPY',(sums.get(row.currency||'JPY')||0)+Number(row.amount));}
  for(const [currency,amount] of sums)totals.push([purchase?'仕入小計（税抜）':shipment?'売価小計（税抜）':'明細金額合計',approvalDocumentMoney(amount,currency)]);
  if(purchase && lines.every(row=>row.line.convertedTotalJpy!=null))totals.push(['仕入小計（円換算）',approvalDocumentMoney(lines.reduce((sum,row)=>sum+Number(row.line.convertedTotalJpy),0),'JPY')]);
 }
 return `<div class="appr-content-card approval-document-detail"><p class="text-muted">申請時の伝票内容</p><div class="appr-content-grid">${[...fields,...totals].map(([label,value])=>`<div class="appr-content-item"><span class="appr-content-label">${esc(label)}</span><span class="appr-content-val" style="white-space:pre-wrap">${esc(value)}</span></div>`).join('')}</div>
 <div class="appr-items-section"><div class="appr-items-title">商品明細（${lines.length}明細）</div><div style="overflow-x:auto"><table class="appr-items-table" style="min-width:860px"><thead><tr><th>No.</th><th>管理番号</th><th>ブランド</th><th>モデル</th><th>型番</th><th>シリアル</th><th>数量</th><th>${purchase?'原価単価':'単価'}</th><th>金額</th></tr></thead><tbody>
 ${lines.map(({line,quantity,currency,unit,amount},index)=>`<tr><td>${esc(line.lineNumber??index+1)}</td><td>${esc(line.productCode||line.partCode||'未発番')}</td><td>${esc(line.brandName||line.brand||'—')}</td><td>${esc(line.modelNumber||'—')}</td><td>${esc(line.referenceNumber||'—')}</td><td>${esc(line.serialNumber||'—')}</td><td>${esc(quantity)}</td><td style="white-space:nowrap">${esc(approvalDocumentMoney(unit,currency))}</td><td style="white-space:nowrap">${esc(approvalDocumentMoney(amount,currency))}</td></tr>${line.notes?`<tr><td></td><td colspan="8">備考：${esc(line.notes)}</td></tr>`:''}`).join('')||'<tr><td colspan="9">商品明細がありません</td></tr>'}
 </tbody></table></div></div></div>`;
}

// Returned documents are edited through an owner- and state-checked API.
function canReviseReturnedApproval(req) {
  return Boolean(window.ZaikoAPI && req?.apiManaged && isWorker() && _approvalRequesterId(req) === currentUserId()
    && ['returned','rejected'].includes(req.status) && ['purchase_slip','sales_slip','shipment_slip','return_slip','consignment_slip','document_change'].includes(req.detail?.targetType));
}
function returnedApprovalButton(req) {
  return canReviseReturnedApproval(req) ? `<button type="button" class="btn btn-warning" onclick="openReturnedApprovalEditor('${approvalDocumentEscape(req.id)}')"><i class="fa-solid fa-pen-to-square"></i> 棄却・差戻し修正</button>` : '';
}
function returnedPurchaseProductFields(line, index) {
  if(line.lineItemKind==='part' || line.generatedProductCount>0 || line.generatedPartCount>0)return '';
  const esc = approvalDocumentEscape;
  const field = (key,label,type='text') => `<label>${label}<input class="form-control" style="width:100%" data-returned-product="${key}" type="${type}" ${type==='number'?'min="0" step="any"':''} ${['referenceNumber','serialNumber'].includes(key)?'data-fmt="half"':''} value="${esc(line[key]??'')}"></label>`;
  const select = (key,label,records) => {
    const value=String(line[key]||'');
    const options=[...(records||[])];
    if(value&&!options.some(r=>String(r.code)===value))options.push({code:value,name:value});
    return `<label>${label}<select class="form-control" style="width:100%" data-returned-product="${key}"><option value="">—</option>${options.map(r=>`<option value="${esc(r.code)}" ${String(r.code)===value?'selected':''}>${esc(r.name)}</option>`).join('')}</select></label>`;
  };
  const records=(name)=>getProductSpecMasterRecords(name);
  const accessories=[...(APP_DATA.accessoryRecords||[])];
  for(const code of line.accessoryCodes||[])if(!accessories.some(r=>r.code===code))accessories.push(allAccessoryMasterRecords().find(r=>r.code===code)||{code,name:code});
  return `<tr><td colspan="7"><details class="returned-product-editor" data-returned-product-index="${index}"><summary style="cursor:pointer;color:var(--primary);padding:10px 0;font-weight:600;">商品登録内容を修正（明細 ${index+1}）</summary>
    <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:16px;padding:16px 0;">
    ${select('brandCode','ブランド',APP_DATA.brandRecords)}${field('modelNumber','モデル')}${field('referenceNumber','型番')}${field('serialNumber','シリアル')}
    ${select('shapeCode','形状',records('shape'))}${select('markingCode','マーキング',records('marking'))}
    ${select('materialCode','素材',records('material'))}${select('movementCode','駆動方式',records('movement'))}${select('conditionCode','コンディション',APP_DATA.conditions)}
    ${select('beltText','ベルト素材',(APP_DATA.beltMaterialRecords||[]).map(r=>({code:r.name,name:r.name})))}${field('dialText','文字盤')}${field('weightGrams','グラム','number')}
    ${select('boxCode','BOX番号',Array.from({length:10},(_,i)=>({code:'BOX-'+String(i+1).padStart(2,'0'),name:'BOX '+(i+1)})))}
    </div>
    <label>付属品</label><div class="checkbox-group" id="returned-accessories-${index}">${accessories.map(r=>`<label class="checkbox-label ${(line.accessoryCodes||[]).includes(r.code)?'checked':''}"><input type="checkbox" value="${esc(r.code)}" ${(line.accessoryCodes||[]).includes(r.code)?'checked':''} onchange="this.closest('.checkbox-label').classList.toggle('checked',this.checked);refreshAccessorySupplementInputs(this.closest('.checkbox-group'))">${esc(r.name)}</label>`).join('')}</div>
    ${field('braceletQuantity','コマ数','number')}${field('productType','商品種別')}${field('duplicateSerialReason','シリアル重複登録理由')}
    <label>商品備考<textarea class="form-control" style="width:100%;margin:10px 0;" rows="3" data-returned-product="notes">${esc(line.notes||'')}</textarea></label>
  </details></td></tr>`;
}
function readReturnedPurchaseProducts(form, doc) {
  const products={};
  for(const editor of form.querySelectorAll('[data-returned-product-index]')) {
    const index=Number(editor.dataset.returnedProductIndex),line=doc.lines[index];
    const detail={};
    for(const input of editor.querySelectorAll('[data-returned-product]'))detail[input.dataset.returnedProduct]=input.value;
    detail.braceletQuantity=detail.braceletQuantity?Number(detail.braceletQuantity):null;
    const group='returned-accessories-'+index;
    detail.accessoryCodes=[...document.querySelectorAll('#'+group+' input:checked')].map(input=>input.value);
    detail.accessorySupplements=readAccessorySupplementInputs(group,true);
    if(detail.accessorySupplements===null)return null;
    products[line.id]=detail;
  }
  return products;
}

function returnedShipmentProductSelect(line,index) {
  const esc=approvalDocumentEscape;
  const items=(APP_DATA.inventory||[]).filter(item=>item.status==='在庫中'&&item.code!==line.productCode);
  return `<select class="form-control" style="min-width:220px;width:100%" aria-label="明細${index+1}の商品" data-returned-field="product-${index}" onchange="returnedShipmentProductChanged(${index})"><option value="${esc(line.productCode)}">${esc(line.productCode)}（現在の商品）</option>${items.map(item=>`<option value="${esc(item.code)}">${esc(item.code)} / ${esc(item.brand)} ${esc(item.model)} / ${esc(item.serial||'')}</option>`).join('')}</select>`;
}
function returnedShipmentProductChanged(index) {
  const doc=returnedApprovalEditor?.document,line=doc?.lines[index];if(!line)return;
  const form=document.getElementById('returned-approval-form');
  const code=form.querySelector(`[data-returned-field="product-${index}"]`).value;
  const cell=form.querySelector(`[data-returned-product-summary="${index}"]`);
  const price=form.querySelector(`[data-returned-field="price-${index}"]`);
  if(code===line.productCode){cell.textContent=[line.brandName||line.brand,line.modelNumber,line.referenceNumber].filter(Boolean).join(' / ');price.value=doc.displayCurrency==='JPY'?line.convertedSalePriceJpy:line.salePriceUsdMinor;return;}
  const item=(APP_DATA.inventory||[]).find(item=>item.code===code);
  if(!item)return;
  cell.textContent=[item.brand,item.model,item.ref,item.serial].filter(Boolean).join(' / ');
  const usd=Number(item.salePrice)||0;
  price.value=doc.displayCurrency==='JPY'&&doc.fxRateScaled>0&&doc.fxScale>0?Math.ceil(usd*doc.fxRateScaled/doc.fxScale/1000)*1000:usd;
}

let returnedApprovalEditor = null;
const returnedHeaderFields={
 purchase_slip:['purchaseDate','purchaseTaxMode','taxCategory','paymentMethod','supplierCode','supplierName','staffCode'],
 sales_slip:['saleDate','buyerCode','displayCurrency','taxMode','taxRateBasisPoints'],
 shipment_slip:['shipmentDate','buyerCode','displayCurrency','salesSlipNumber','recipientName','recipientAddress','carrier','trackingNumber'],
 consignment_slip:['consignmentDate','consigneeCode'],
 return_slip:['transactionDate','buyerCode','supplierCode','sourcePurchaseSlipNumber','carrier','trackingNumber','reason']
};
const returnedLabels={purchaseDate:'仕入日',purchaseTaxMode:'仕入区分',taxCategory:'税区分',paymentMethod:'支払方法',supplierCode:'仕入先',supplierName:'仕入先名（個人買取）',staffCode:'バイヤー',saleDate:'売上日',buyerCode:'販売先',displayCurrency:'通貨',taxMode:'税区分',taxRateBasisPoints:'税率（%）',shipmentDate:'出荷日',salesSlipNumber:'参照売上伝票番号',recipientName:'宛先名',recipientAddress:'宛先住所',carrier:'配送会社',trackingNumber:'追跡番号',consignmentDate:'委託日',consigneeCode:'委託先',transactionDate:'返品日',sourcePurchaseSlipNumber:'元仕入伝票番号',reason:'返品理由',notes:'備考',productCode:'管理番号',brandCode:'ブランド',brandName:'ブランド',brand:'ブランド',modelNumber:'モデル',referenceNumber:'型番',serialNumber:'シリアル',sku:'SKU',quantity:'数量',unitCostMinor:'原価',costCurrency:'原価通貨',baseSalePriceMinor:'想定売価',baseSaleCurrency:'想定売価通貨',unitPriceMinor:'売価',salePriceUsdMinor:'売価（USD）',convertedSalePriceJpy:'売価（円換算）',materialCode:'素材',movementCode:'駆動方式',conditionCode:'コンディション',shapeCode:'形状',markingCode:'マーキング',weightGrams:'グラム',beltText:'ベルト素材',dialText:'文字盤',boxCode:'BOX番号',accessoryCodes:'付属品',accessorySupplements:'付属品補足',braceletQuantity:'コマ数',productType:'商品種別',duplicateSerialReason:'シリアル重複登録理由',subtotalMinor:'小計（税抜）',taxAmountMinor:'消費税',totalMinor:'合計（税込）'};
const returnedChoices={purchaseTaxMode:[['domestic','国内業者仕入'],['personal','個人買取'],['overseas','海外仕入']],taxCategory:[['consumption_tax','消費税'],['tax_equivalent','税相当額'],['out_of_scope','対象外']],paymentMethod:[['cash','現金'],['bank_transfer','銀行振込'],['card','カード']],taxMode:[['taxable','課税'],['out_of_scope','対象外'],['exempt','非課税']],displayCurrency:[['JPY','円（JPY）'],['USD','USドル（USD）'],['HKD','香港ドル（HKD）']]};
function returnedMasterOptions(key){
 if(key==='supplierCode')return APP_DATA.suppliers||[];
 if(key==='buyerCode'||key==='consigneeCode')return APP_DATA.buyers||[];
 if(key==='staffCode')return APP_DATA.staffRecords||[];
 const masters={brandCode:'brandRecords',conditionCode:'conditions',accessoryCodes:'accessoryRecords'};
 if(masters[key])return APP_DATA[masters[key]]||[];
 const spec={shapeCode:'shape',markingCode:'marking',materialCode:'material',movementCode:'movement'};
 return spec[key]?getProductSpecMasterRecords(spec[key]):null;
}
function returnedControl(key,value,attr='',choices=null){
 if(key==='taxRateBasisPoints' && value!=null)value=Number(value)/100;
 const esc=approvalDocumentEscape, options=choices||returnedChoices[key]||returnedMasterOptions(key)?.map(r=>[r.code,r.name]);
 if(options){const all=[...options];if(value!==''&&value!=null&&!all.some(([v])=>String(v)===String(value)))all.push([value,value]);return `<select class="form-control" style="width:100%" ${attr}><option value="">—</option>${all.map(([v,label])=>`<option value="${esc(v)}" ${String(v)===String(value)?'selected':''}>${esc(label||v)}</option>`).join('')}</select>`;}
 const numeric=['taxRateBasisPoints','unitPriceMinor','unitCostMinor','baseSalePriceMinor','weightGrams','braceletQuantity'].includes(key);
 return `<input class="form-control" style="width:100%;min-width:100px" ${attr} type="${key.endsWith('Date')?'date':numeric?'number':'text'}" ${numeric?'min="0" step="any" required':''} ${['sku','referenceNumber','serialNumber'].includes(key)?'data-fmt="half"':''} value="${esc(value??'')}">`;
}
function returnedVisibleHeaders(type,doc){
 return returnedHeaderFields[type].filter(key=>{
  if(type==='return_slip')return doc.operationType==='purchase_return'?key!=='buyerCode':!['supplierCode','sourcePurchaseSlipNumber'].includes(key);
  return true;
 });
}
async function openReturnedApprovalEditor(id){
 const req=(APP_DATA.approvalRequests||[]).find(r=>r.id===id);if(!canReviseReturnedApproval(req))return;
 if(req.detail?.targetType==='document_change')return openDocumentChangeEditor(req);
 try{
  const loaded=await ZaikoAPI.request(`/approvals/${encodeURIComponent(id)}/returned-document`),doc=loaded.document,type=loaded.approval.targetType,esc=approvalDocumentEscape;
  returnedApprovalEditor={id,...loaded,rows:JSON.parse(JSON.stringify(doc.lines||[])).map(line=>({...line,unitPriceMinor:type==='shipment_slip'?(doc.displayCurrency==='JPY'?line.convertedSalePriceJpy:line.salePriceUsdMinor):type==='consignment_slip'?line.salePriceUsdMinor:line.unitPriceMinor}))};
  document.getElementById('apprDetailTitle').textContent=`棄却・差戻し修正 — ${doc.slipNumber}`;
  document.getElementById('apprDetailBody').innerHTML=`<form id="returned-approval-form">${returnedRegistrationLayout(loaded)}${returnApprovalAdjustmentEditor(doc)}</form>`;
  returnedRenderRows();
  if ((doc.lines || []).some(line=>line.auctionFee?.rule)) showToast('info','手数料適用済みの伝票です','金額欄は手数料反映後の金額です。差戻し修正では手数料を再計算しません。');returnedRegistrationInit();
  document.getElementById('apprDetailBody').closest('.modal').style.maxWidth='1440px';
  document.getElementById('apprDetailFooter').innerHTML=`<button class="btn btn-outline" onclick="openWorkerApprovalResult('${esc(id)}')">戻る</button><button id="returned-approval-submit" class="btn btn-primary" onclick="submitReturnedApproval()">修正を保存して再申請</button>`;
  document.getElementById('approvalDetailOverlay').classList.remove('hidden');
 }catch(error){showToast('error','差戻し伝票を開けませんでした',error.message);}
}
function returnedCaptureRows(validate=true){
 const state=returnedApprovalEditor,form=document.getElementById('returned-approval-form');if(!state||!form)return false;
 for(let i=0;i<state.rows.length;i++){
  const row=state.rows[i];for(const el of form.querySelectorAll(`[data-returned-row="${i}"]`))row[el.dataset.rowKey]=el.type==='checkbox'?el.checked:el.type==='number'?Number(el.value):el.value;
  const editor=form.querySelector(`[data-returned-product-index="${i}"]`);
  if(editor){for(const el of editor.querySelectorAll('[data-returned-product]'))row[el.dataset.returnedProduct]=el.value;
   row.braceletQuantity=row.braceletQuantity===''||row.braceletQuantity==null?null:Number(row.braceletQuantity);
   row.accessoryCodes=[...editor.querySelectorAll('input[type="checkbox"]:checked')].map(el=>el.value);
   row.accessorySupplements=readAccessorySupplementInputs('returned-accessories-'+i,validate);if(row.accessorySupplements===null)return false;
  }
 }
 return true;
}
function returnedRenderRows(){returnedRegistrationRenderRows();}
function returnedAddLine(){if(!returnedCaptureRows(false))return;const state=returnedApprovalEditor;if(state.rows.length>=100){showToast('warning','明細は100件までです','');return;}const first=state.rows[0]||{};state.rows.push({id:'',quantity:1,sku:'',brandCode:'',productCode:'',modelNumber:'',unitCostMinor:0,unitPriceMinor:0,baseSalePriceMinor:0,costCurrency:first.costCurrency||'JPY',baseSaleCurrency:first.baseSaleCurrency||'USD',accessoryCodes:[],accessorySupplements:{}});returnedRenderRows();}
function returnedRemoveLine(index){if(!returnedCaptureRows(false))return;returnedApprovalEditor.rows.splice(index,1);returnedRenderRows();}
function returnedFullProductChanged(index){
 const state=returnedApprovalEditor,el=document.querySelector(`[data-returned-row="${index}"][data-row-key="productCode"]`),price=document.querySelector(`[data-returned-row="${index}"][data-row-key="unitPriceMinor"]`);if(!price)return;
 const original=state.document.lines.find(l=>l.productCode===el.value),type=state.approval.targetType,currency=document.querySelector('[data-returned-header="displayCurrency"]')?.value||'USD';
 if(original){price.value=type==='sales_slip'?original.unitPriceMinor:type==='consignment_slip'||currency!=='JPY'?original.salePriceUsdMinor:original.convertedSalePriceJpy;return;}
 const item=(APP_DATA.inventory||[]).find(p=>p.code===el.value);if(!item)return;
 const usd=Number(item.salePrice)||0,doc=state.document;price.value=type!=='consignment_slip'&&currency==='JPY'&&doc.fxScale>0?Math.ceil(usd*doc.fxRateScaled/doc.fxScale/1000)*1000:usd;
}
async function submitReturnedApproval(){
 const state=returnedApprovalEditor,form=document.getElementById('returned-approval-form'),button=document.getElementById('returned-approval-submit');if(!state||!form||button.disabled||!form.reportValidity()||!returnedCaptureRows())return;
 const errorEl=document.getElementById('returned-approval-error');if(!state.rows.length && !state.document.adjustmentDetails?.lines?.length){errorEl.textContent='商品明細または金額調整を1件以上追加してください。';return;}
 const header={};form.querySelectorAll('[data-returned-header]').forEach(el=>header[el.dataset.returnedHeader]=el.type==='number'?Number(el.value)*(el.dataset.returnedHeader==='taxRateBasisPoints'?100:1):el.value);
 const reason=form.querySelector('[data-returned-field="reason"]').value.trim();if(!reason){errorEl.textContent='再申請理由を入力してください。';return;}
 const doc=state.document,type=state.approval.targetType,rows=state.rows.filter(row=>type!=='sales_slip'||row._included!==false),rebuild={reason,originalLineIds:rows.map(r=>r.id||'')};
 if(!rows.length && !doc.adjustmentDetails?.lines?.length){errorEl.textContent='計上する商品明細を1件以上選択してください。';return;}
 if(type==='purchase_slip'){
  const keys=['quantity','sku','brandCode','modelNumber','referenceNumber','serialNumber','productType','shapeCode','markingCode','materialCode','movementCode','conditionCode','accessoryCodes','accessorySupplements','beltText','dialText','braceletQuantity','weightGrams','boxCode','unitCostMinor','costCurrency','baseSalePriceMinor','baseSaleCurrency','notes','duplicateSerialReason'];
  rebuild.purchase={...header,lines:rows.map(row=>Object.fromEntries(keys.filter(key=>row[key]!=null).map(key=>[key,key==='quantity'?1:row[key]])))};
 }else{
  const codes=rows.map(r=>r.productCode);if(codes.some(c=>!c)||new Set(codes).size!==codes.length){errorEl.textContent='商品を選択してください。同じ商品を複数の明細には指定できません。';return;}
  if(type==='sales_slip')rebuild.sale={...header,lines:rows.map(r=>({productCode:r.productCode,unitPriceMinor:Number(r.unitPriceMinor)}))};
  if(type==='shipment_slip')rebuild.shipment={...header,productCodes:codes,items:rows.map(r=>({productCode:r.productCode,[header.displayCurrency==='JPY'?'salePriceJpyMinor':'salePriceUsdMinor']:Number(r.unitPriceMinor)}))};
  if(type==='consignment_slip')rebuild.consignment={...header,productCodes:codes,items:rows.map(r=>({productCode:r.productCode,salePriceUsdMinor:Number(r.unitPriceMinor)}))};
  if(type==='return_slip')rebuild.return={...header,operationType:doc.operationType,productCodes:codes,sourceSaleSlipNumber:doc.adjustmentDetails?.sourceSaleSlipNumber || '',adjustmentLines:captureReturnApprovalAdjustments(doc)};
 }
 button.disabled=true;try{
  await ZaikoAPI.request(`/approvals/${encodeURIComponent(state.id)}/resubmit`,{method:'POST',body:JSON.stringify({expectedApprovalUpdatedAt:state.approval.updatedAt,expectedDocumentUpdatedAt:doc.updatedAt,rebuild})});
  returnedApprovalEditor=null;closeApprovalDetail();showToast('success','修正して再申請しました',`${doc.slipNumber} は管理者の承認待ちです。`);await ZaikoAPI.hydrateAdmin();renderWorkerNotificationList();renderNotifyList();updateNotifyBadge();
 }catch(error){errorEl.textContent=error.message;}finally{button.disabled=false;}
}

function returnedReviewValue(key,value){
 if(value==null||value==='')return '—';
 if(key==='taxRateBasisPoints')return String(Number(value)/100)+'%';
 if(key==='accessorySupplements')return Object.entries(value||{}).map(([code,v])=>`${allAccessoryMasterRecords().find(r=>r.code===code)?.name||code}：${v}`).join(' / ')||'—';
 if(Array.isArray(value))return value.map(v=>returnedReviewValue(key,v)).join(' / ')||'—';
 const choices=returnedChoices[key];if(choices)return choices.find(([code])=>String(code)===String(value))?.[1]||String(value);
 const masters=returnedMasterOptions(key);if(masters)return masters.find(r=>String(r.code)===String(value))?.name||String(value);
 if(typeof value==='number')return value.toLocaleString('ja-JP');
 return String(value);
}
function returnedReviewEqual(a,b){
 const normalize=v=>v==null||v===''?'':Array.isArray(v)?v.map(normalize):typeof v==='object'?Object.fromEntries(Object.keys(v).sort().map(k=>[k,normalize(v[k])])):String(v);
 return JSON.stringify(normalize(a))===JSON.stringify(normalize(b));
}
function buildReturnedApprovalReview(detail){
 const doc=detail.document,previous=detail.previousDocument,type=detail.targetType,esc=approvalDocumentEscape;if(!previous||!doc||!returnedHeaderFields[type])return null;
 const changedStyle='background:#fff1a8;border:1px solid #e2c147;border-radius:6px;padding:10px;';
 const cell=(key,old,value,force=false)=>{const changed=force||!returnedReviewEqual(old,value);return `<div class="returned-review-field${changed?' returned-review-changed':''}" style="${changed?changedStyle:'padding:10px;'}"><b>${esc(returnedLabels[key]||key)}</b><div style="white-space:pre-wrap">${esc(returnedReviewValue(key,value))}</div>${changed?`<small style="color:#725719">変更前：${esc(returnedReviewValue(key,old))}</small>`:''}</div>`;};
 const headers=[...returnedVisibleHeaders(type,doc),'notes'];if(type==='sales_slip')headers.push('subtotalMinor','taxAmountMinor','totalMinor');
 const keys=type==='purchase_slip'?['sku','brandCode','modelNumber','referenceNumber','serialNumber','quantity','unitCostMinor','costCurrency','baseSalePriceMinor','baseSaleCurrency','shapeCode','markingCode','materialCode','movementCode','conditionCode','beltText','dialText','weightGrams','boxCode','accessoryCodes','accessorySupplements','braceletQuantity','productType','duplicateSerialReason','notes']:type==='sales_slip'?['productCode','brand','modelNumber','referenceNumber','serialNumber','unitPriceMinor']:type==='return_slip'?['productCode','brand','modelNumber']:['productCode','brand','modelNumber','salePriceUsdMinor','convertedSalePriceJpy'];
 const oldByID=new Map((previous.lines||[]).map(l=>[l.id,l])),newIDs=new Set((doc.lines||[]).map(l=>l.id));
 const lines=(doc.lines||[]).map((line,i)=>{const old=oldByID.get(line.id);return `<section style="margin:18px 0;border:1px solid #dce1e8;padding:12px;border-radius:8px"><h4 ${old?'':`class="returned-review-changed" style="${changedStyle}"`}>明細 ${i+1}${old?'':'（追加）'}</h4><div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(190px,1fr));gap:8px">${keys.map(key=>cell(key,old?.[key],line[key],!old)).join('')}</div></section>`;}).join('');
 const removed=(previous.lines||[]).filter(l=>!newIDs.has(l.id)).map(line=>`<section class="returned-review-changed" style="${changedStyle}margin:12px 0"><b>削除した明細 ${esc(line.lineNumber)}：${esc([line.productCode,line.brandName||line.brand,line.modelNumber,line.referenceNumber].filter(Boolean).join(' / '))}</b><div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(190px,1fr));gap:8px">${keys.map(key=>`<div><b>${esc(returnedLabels[key])}</b>：${esc(returnedReviewValue(key,line[key]))}</div>`).join('')}</div></section>`).join('');
 return `<div class="approval-document-detail"><p><b>伝票No：${esc(doc.slipNumber)}</b></p><p style="${changedStyle}">黄色の項目が、前回の申請から変更された箇所です。追加・削除した明細も黄色で表示します。</p>${detail.resubmissionReason?`<p>再申請理由：${esc(detail.resubmissionReason)}</p>`:''}<div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(230px,1fr));gap:10px">${headers.map(key=>cell(key,previous[key],doc[key])).join('')}</div><h4>商品明細（${doc.lines?.length||0}明細）</h4>${lines}${removed}</div>`;
}

function documentChangeDetail(detail,which){const snap=detail.document||{};return {targetType:({purchase:'purchase_slip',sale:'sales_slip',shipment:'shipment_slip',consignment:'consignment_slip',return:'return_slip'})[String(snap.kind||'').split('.')[0]],document:snap[which],previousDocument:which==='after'?detail.previousDocument?.after:null,resubmissionReason:detail.resubmissionReason};}
const documentChangeFieldLabels={purchaseDate:'仕入日',saleDate:'売上日',shipmentDate:'出荷日',consignmentDate:'委託日',transactionDate:'返品日',adjustmentDate:'変更日',supplierCode:'仕入先コード',staffCode:'バイヤーコード',buyerCode:'販売先コード',consigneeCode:'委託先コード',notes:'備考',reason:'変更理由',returnReason:'返品理由',carrier:'配送会社',trackingNumber:'追跡番号',confirmed:'配送番号を確定',code:'管理番号',sku:'SKU',unitCostMinor:'原価',baseSalePriceMinor:'想定売価',unitPriceMinor:'金額',adjustmentAmountMinor:'調整金額',direction:'増減区分',lines:'明細',recipientName:'宛先名',recipientAddress:'宛先住所',expectedUpdatedAt:'更新日時'};
function documentChangeFields(payload,editable,prefix=[]){
 return Object.entries(payload).map(([key,value])=>{
  if(['id','saleLineId','expectedUpdatedAt','organizationId','actorUserId'].includes(key))return '';
  const path=[...prefix,key],label=documentChangeFieldLabels[key]||key,esc=approvalDocumentEscape;
  if(Array.isArray(value))return `<div><b>${esc(label)}</b>${value.map((row,i)=>`<fieldset><legend>明細 ${i+1}</legend>${documentChangeFields(row,editable,[...path,i])}</fieldset>`).join('')}</div>`;
  if(value&&typeof value==='object')return documentChangeFields(value,editable,path);
  if(!editable)return `<p><b>${esc(label)}</b>：${esc(typeof value==='boolean'?(value?'はい':'いいえ'):value)}</p>`;
  const data=esc(JSON.stringify(path));
  if(typeof value==='boolean')return `<label>${esc(label)} <input type="checkbox" data-change-path="${data}" ${value?'checked':''}></label>`;
  return `<label style="display:block;margin:12px 0">${esc(label)}<input class="form-control" style="width:100%" data-change-path="${data}" type="${typeof value==='number'?'number':key.endsWith('Date')?'date':'text'}" value="${esc(value)}"></label>`;
 }).join('');
}
let documentChangeEditor=null;
function openDocumentChangeEditor(req){
 if(/^(purchase|sale|shipment|consignment)\.revision$/.test(req.detail.document.kind))return openReturnedRevisionEditor(req);
 const snap=req.detail.document;documentChangeEditor={req,payload:JSON.parse(JSON.stringify(snap.payload||{}))};
 document.getElementById('apprDetailTitle').textContent='申請内容を修正して再申請';
 document.getElementById('apprDetailBody').innerHTML=`<p>伝票No：${approvalDocumentEscape(snap.before?.slipNumber||'')}</p><p>棄却・差戻し理由：${approvalDocumentEscape(req.decisionNote||req.revisionComment||req.note||'—')}</p><form id="document-change-form">${documentChangeFields(documentChangeEditor.payload,true)}<p id="document-change-error" style="color:#c0392b" role="alert"></p></form>`;
 document.getElementById('apprDetailFooter').innerHTML='<button class="btn btn-outline" onclick="closeApprovalDetail()">閉じる</button><button id="document-change-submit" class="btn btn-primary" onclick="resubmitDocumentChange()">修正して再申請</button>';
 document.getElementById('approvalDetailOverlay').classList.remove('hidden');
}
async function resubmitDocumentChange(){
 const state=documentChangeEditor,form=document.getElementById(state?.registration?'returned-approval-form':'document-change-form'),button=document.getElementById('document-change-submit');if(!state||button.disabled||!form.reportValidity())return;
 const payload=state.registration?returnedRevisionPayload():JSON.parse(JSON.stringify(state.payload));
 form.querySelectorAll('[data-change-path]').forEach(input=>{const path=JSON.parse(input.dataset.changePath);let obj=payload;path.slice(0,-1).forEach(k=>obj=obj[k]);obj[path.at(-1)]=input.type==='checkbox'?input.checked:input.type==='number'?Number(input.value):input.value;});
 button.disabled=true;try{
 const snap=state.req.detail.document;
 await ZaikoAPI.request('/document-change-requests',{method:'POST',body:JSON.stringify({kind:snap.kind,targetId:snap.targetId,payload,approvalId:state.req.id,expectedUpdatedAt:state.req.updatedAt})});
 closeApprovalDetail();await ZaikoAPI.hydrateAdmin();showToast('success','修正して再申請しました','管理者承認後に反映されます。');
 }catch(error){document.getElementById('document-change-error').textContent=error.message;}finally{button.disabled=false;}
}

async function downloadApprovedDocument(id){try { const ref=await ZaikoAPI.request('/approvals/'+encodeURIComponent(id)+'/document');const link=document.createElement('a');link.href=ref.downloadUrl;link.download=ref.fileName;link.click();}catch(error){showToast('error','PDFを取得できません',error.message);}}

// Use the registration screens' cards, form rows, switches and line tables.
function returnedRegistrationField(key,doc,label=returnedLabels[key]){
 return `<div class="form-group" data-returned-header-group="${key}"><label class="form-label">${approvalDocumentEscape(label)}</label>${returnedControl(key,doc[key],`data-returned-header="${key}" aria-label="${approvalDocumentEscape(label)}" ${returnedMasterOptions(key)?'data-partner-search':''}`)}</div>`;
}
function returnedRegistrationSaleBuyer(doc){
 const esc=approvalDocumentEscape,name=doc.buyerName||returnedMasterOptions('buyerCode').find(b=>b.code===doc.buyerCode)?.name||doc.buyerCode||'—';
 return `<div class="form-group" data-returned-header-group="buyerCode"><label class="form-label">販売先 <span class="auto-tag">変更不可</span></label><input type="hidden" data-returned-header="buyerCode" value="${esc(doc.buyerCode||'')}"><input class="form-control auto-filled" aria-label="販売先" readonly value="${esc(name)}"></div>`;
}
function returnedRegistrationSwitch(key,value,style,options=returnedChoices[key]){
 const esc=approvalDocumentEscape;
 return `<input type="hidden" data-returned-header="${key}" value="${esc(value||options[0][0])}"><div class="${style}-switch" role="group" aria-label="${esc(returnedLabels[key]||key)}">${options.map(([v,name])=>`<button type="button" class="${style}-btn ${v===(value||options[0][0])?'active':''}" data-returned-switch="${key}" data-switch-value="${esc(v)}" aria-pressed="${v===(value||options[0][0])}" onclick="returnedRegistrationSwitchChanged(this)">${esc(name)}</button>`).join('')}</div>`;
}
function returnedRegistrationLayout(loaded){
 const doc=loaded.document,type=loaded.approval.targetType,esc=approvalDocumentEscape;
 const title={purchase_slip:['file-invoice','仕入伝票'],sales_slip:['receipt','売上伝票'],shipment_slip:['truck','出荷伝票'],consignment_slip:['handshake','委託伝票'],return_slip:['rotate-left',doc.operationType==='purchase_return'?'仕入返品伝票':'売上返品伝票']}[type];
 const number=`<div class="form-group"><label class="form-label">伝票番号 <span class="auto-tag">変更なし</span></label><input class="form-control auto-filled" aria-label="伝票番号" readonly value="${esc(doc.slipNumber)}"></div>`;
 let headers='',extra='';
 if(type==='purchase_slip'){
  headers=`<div class="form-row cols-4">${number}${returnedRegistrationField('purchaseDate',doc)}<div class="form-group">${returnedRegistrationField('supplierCode',doc)}${returnedRegistrationField('supplierName',doc,'仕入先名')}</div>${returnedRegistrationField('staffCode',doc)}</div>`;
  for(const [key,style,icon,label] of [['paymentMethod','pe-payment-method','wallet','支払い方法'],['purchaseTaxMode','pe-tax-mode','receipt','仕入区分'],['taxCategory','pe-tax-category','percent','税区分']])headers+=`<div class="${style}-panel"><div class="${style}-copy"><strong><i class="fa-solid fa-${icon}"></i> ${label}</strong></div>${returnedRegistrationSwitch(key,doc[key],style)}${key==='purchaseTaxMode'?`<div class="pe-purchase-currency-panel"><strong>仕入通貨</strong><div class="pe-purchase-currency-switch" role="group" aria-label="仕入通貨">${returnedChoices.displayCurrency.map(([code,name])=>`<button class="pe-purchase-currency-btn ${(doc.lines?.[0]?.costCurrency||'JPY')===code?'active':''}" type="button" data-returned-cost-currency="${code}" onclick="returnedSetCostCurrency('${code}')">${esc(name)}</button>`).join('')}</div></div>`:''}</div>`;
 }else if(type==='shipment_slip'){
  headers=`<div class="form-row shipping-entry-header-fields">${number}${returnedRegistrationField('shipmentDate',doc)}${returnedRegistrationField('buyerCode',doc,'出荷先')}<div class="form-group shipping-entry-currency"><label class="form-label">売価</label>${returnedRegistrationSwitch('displayCurrency',doc.displayCurrency,'sl-currency',[['USD','$ USD'],['JPY','¥ 円']])}</div>${returnedRegistrationField('notes',doc)}</div>`;
  extra=`<details class="returned-delivery"><summary>配送情報・参照伝票</summary><div class="form-row cols-3">${['salesSlipNumber','recipientName','recipientAddress','carrier','trackingNumber'].map(key=>returnedRegistrationField(key,doc)).join('')}</div></details>`;
 }else if(type==='sales_slip'){
  headers=`<div class="form-row cols-3 sales-entry-header-fields">${number}${returnedRegistrationField('saleDate',doc)}${returnedRegistrationSaleBuyer(doc)}</div><div class="returned-sales-options"><div><label class="form-label">販売金額の入力通貨</label>${returnedRegistrationSwitch('displayCurrency',doc.displayCurrency,'sl-currency')}</div><div><label class="form-label">税区分</label>${returnedRegistrationSwitch('taxMode',doc.taxMode||'out_of_scope','pe-tax-category')}</div>${returnedRegistrationField('taxRateBasisPoints',doc)}</div>`;
 }else if(type==='consignment_slip')headers=`<div class="form-row cols-4 consignment-meta-grid">${number}${returnedRegistrationField('consignmentDate',doc)}${returnedRegistrationField('consigneeCode',doc)}${returnedRegistrationField('notes',doc)}</div>`;
 else headers=`<div class="form-row cols-3">${number}${returnedVisibleHeaders(type,doc).map(key=>returnedRegistrationField(key,doc)).join('')}</div>`;
 const add=type==='purchase_slip'?`<div class="pe-line-add-controls"><label for="returned-add-count">追加行数</label><div class="pe-line-add-count-field"><input id="returned-add-count" class="form-control" type="number" min="1" max="100" value="1"><span>行</span></div><button type="button" class="btn btn-primary btn-sm" onclick="returnedRegistrationAddLines()"><i class="fa-solid fa-plus"></i> 明細追加</button></div>`:'';
 const notes=!['shipment_slip','consignment_slip'].includes(type)?`<div class="form-group returned-notes"><label class="form-label">備考</label><textarea class="form-control" rows="2" data-returned-header="notes">${esc(doc.notes||'')}</textarea></div>`:'';
 return `<div class="returned-registration returned-registration--${type}"><div class="returned-reason"><i class="fa-solid fa-comment-dots"></i><div><b>棄却・差戻し理由</b><p>${esc(loaded.approval.decisionNote||'—')}</p></div></div><section class="card mb-16"><div class="card-header"><h3><i class="fa-solid fa-${title[0]}"></i> ${title[1]}</h3></div><div class="card-body">${headers}${extra}</div></section><section class="card mb-16"><div class="card-header"><h3><i class="fa-solid fa-list"></i> 明細</h3>${add}</div><div class="card-body returned-lines-body"><div id="returned-registration-lines"></div>${type==='purchase_slip'?'':`<button type="button" class="add-row-btn" onclick="returnedAddLine()">＋ 明細行を追加</button>`}<div id="returned-registration-totals" class="slip-total-area" aria-live="polite"></div>${notes}</div></section><section class="card"><div class="card-header"><h3><i class="fa-solid fa-comment"></i> 再申請</h3></div><div class="card-body"><label class="form-label" for="returned-resubmit-reason">修正内容・再申請理由 <span class="required">*</span></label><textarea id="returned-resubmit-reason" class="form-control" rows="3" required maxlength="1000" data-returned-field="reason"></textarea><p class="text-muted">伝票番号を維持して再申請します。管理者の承認後に確定します。</p><p id="returned-approval-error" role="alert" class="returned-error"></p></div></section></div>`;
}
function returnedRegistrationInit(){
 const form=document.getElementById('returned-approval-form');
 form.addEventListener('input',e=>{if(!e.isComposing && !e.target.closest('#returned-product-overlay'))returnedRegistrationTotals();});
 form.addEventListener('change',e=>{if(!e.target.closest('#returned-product-overlay'))returnedRegistrationTotals();});
 returnedRegistrationSyncHeaders();enhanceSearchablePartnerSelects();
 const body=document.getElementById('apprDetailBody');
 body._returnedSearchController?.abort();body._returnedSearchController=new AbortController();
 const options={capture:true,signal:body._returnedSearchController.signal};
 body.addEventListener('scroll',returnedPositionSearchPanels,options);
 form.addEventListener('focusin',returnedPositionSearchPanels);
 form.addEventListener('click',returnedPositionSearchPanels);form.addEventListener('input',returnedPositionSearchPanels);
 window.addEventListener('resize',returnedPositionSearchPanels,{signal:body._returnedSearchController.signal});
}
function returnedPositionSearchPanels(){
 document.querySelectorAll('#returned-approval-form .partner-search-select.open').forEach(wrapper=>{
  const rect=wrapper.getBoundingClientRect(),panel=wrapper.querySelector('.partner-search-panel'),height=Math.min(238,panel.scrollHeight||238),width=Math.min(Math.max(rect.width,230),innerWidth-24);
  Object.assign(panel.style,{position:'fixed',width:width+'px',left:Math.max(12,Math.min(rect.left,innerWidth-width-12))+'px',top:(rect.bottom+height+8>innerHeight?Math.max(12,rect.top-height-5):rect.bottom+5)+'px'});
 });
}
function returnedRegistrationSwitchChanged(button){
 const form=document.getElementById('returned-approval-form'),key=button.dataset.returnedSwitch;
 form.querySelector(`[data-returned-header="${key}"]`).value=button.dataset.switchValue;
 form.querySelectorAll(`[data-returned-switch="${key}"]`).forEach(b=>{b.classList.toggle('active',b===button);b.setAttribute('aria-pressed',String(b===button));});
 returnedRegistrationSyncHeaders();returnedRegistrationTotals();
}
function returnedRegistrationSyncHeaders(){
 const form=document.getElementById('returned-approval-form'),personal=form.querySelector('[data-returned-header="purchaseTaxMode"]')?.value==='personal';
 for(const key of ['supplierCode','supplierName']){const group=form.querySelector(`[data-returned-header-group="${key}"]`);if(group&&returnedApprovalEditor.approval.targetType==='purchase_slip')group.hidden=(key==='supplierName')!==personal;}
 const tax=form.querySelector('[data-returned-header-group="taxRateBasisPoints"]');if(tax){tax.hidden=form.querySelector('[data-returned-header="taxMode"]')?.value!=='taxable';const input=tax.querySelector('input');if(input.value==='')input.value='0';}
 const currency=form.querySelector('[data-returned-header="displayCurrency"]')?.value||'USD';form.querySelectorAll('[data-returned-price-heading]').forEach(el=>el.textContent=`販売金額（${currency}・税抜）`);
}
function returnedRegistrationAddLines(){
 const count=Number(document.getElementById('returned-add-count').value);if(!Number.isInteger(count)||count<1||count+returnedApprovalEditor.rows.length>100){showToast('warning','追加行数を確認してください','明細は合計100件までです。');return;}
 for(let i=0;i<count;i++)returnedAddLine();
}
function returnedSetCostCurrency(currency){
 if(!returnedCaptureRows(false))return;returnedApprovalEditor.rows.forEach(row=>row.costCurrency=currency);document.querySelectorAll('[data-returned-cost-currency]').forEach(b=>b.classList.toggle('active',b.dataset.returnedCostCurrency===currency));returnedRenderRows();
}
function returnedRegistrationProduct(row){
 const item=(APP_DATA.inventory||[]).find(p=>p.code===row.productCode),original=returnedApprovalEditor.document.lines.find(l=>l.productCode===row.productCode)||{};
 return {brand:item?.brand||original.brand||row.brandName||row.brand||'—',model:item?.model||original.modelNumber||row.modelNumber||'—',ref:item?.ref||original.referenceNumber||'—',serial:item?.serial||original.serialNumber||'—',accessories:item?.accessories||original.accessories||[],cost:item?.purchasePrice??original.costAmountMinor??null};
}
function returnedRegistrationProductSelect(row,index){
 const state=returnedApprovalEditor,type=state.approval.targetType,doc=state.document,allowed=type==='return_slip'?(doc.operationType==='purchase_return'?['在庫中','仕入中']:['売上済','出荷済','取置中']):type==='sales_slip'?['在庫中','出荷済','取置中','委託中']:['在庫中'];
 const choices=(APP_DATA.inventory||[]).filter(p=>allowed.includes(p.status)||doc.lines.some(l=>l.productCode===p.code)).map(p=>[p.code,`${p.code} / ${p.brand} ${p.model}`]);
 for(const line of doc.lines)if(!choices.some(([code])=>code===line.productCode))choices.push([line.productCode,`${line.productCode} / ${line.brand||''} ${line.modelNumber||''}`]);
 return returnedControl('productCode',row.productCode,`data-partner-search aria-label="明細${index+1}の管理番号" data-returned-row="${index}" data-row-key="productCode" onchange="returnedRegistrationProductChanged(${index})"`,choices);
}
function returnedRegistrationRenderRows(){
 const {rows,approval}=returnedApprovalEditor,type=approval.targetType,esc=approvalDocumentEscape,purchase=type==='purchase_slip',sale=type==='sales_slip',consignment=type==='consignment_slip',returned=type==='return_slip';
 const remove=i=>`<button type="button" class="btn btn-sm returned-remove" title="この明細を削除" aria-label="明細${i+1}を削除" onclick="returnedRemoveLine(${i})"><i class="fa-solid fa-trash-can"></i></button>`;
 const field=(row,i,key,choices)=>returnedControl(key,row[key],`aria-label="明細${i+1}の${returnedLabels[key]}" data-returned-row="${i}" data-row-key="${key}"`,choices);
 let headings=purchase?['No.','商品コード','SKU（任意）','仕入金額','税区分 / 税額','想定売価','商品登録','削除']:consignment?['商品管理番号','ブランド','モデル名','原価（円）','販売価格（USD）','']:returned?['管理番号','ブランド','モデル名','型番','シリアル','原価','']:['商品コード','ブランド','モデル名','型番','シリアル','付属品','原価','販売金額',''];if(sale)headings.unshift('計上');
 let html=`<div class="returned-table-wrap"><table class="${sale?'sl-table':purchase?'data-table pe-detail-table':'data-table'} returned-entry-table"><thead><tr>${headings.map(h=>`<th ${h==='販売金額'?'data-returned-price-heading':''}>${esc(h)}</th>`).join('')}</tr></thead><tbody>`;
 html+=rows.map((row,i)=>{
  if(purchase)return `<tr><td>${i+1}</td><td><code>${esc(row.productCode||'未発番')}</code><small class="returned-product-summary">${esc([row.brandName||returnedMasterOptions('brandCode').find(b=>b.code===row.brandCode)?.name,row.modelNumber].filter(Boolean).join(' '))}</small></td><td>${field(row,i,'sku')}</td><td>${field(row,i,'unitCostMinor')}${field(row,i,'costCurrency',returnedChoices.displayCurrency)}</td><td data-returned-line-tax="${i}"></td><td>${field(row,i,'baseSalePriceMinor')}${field(row,i,'baseSaleCurrency',returnedChoices.displayCurrency)}</td><td><button type="button" class="btn btn-outline btn-sm pe-regbtn" onclick="returnedOpenProduct(${i})"><i class="fa-solid fa-box-open"></i> ${row.modelNumber?'確認・編集':'登録'}</button></td><td>${remove(i)}</td></tr>`;
  const p=returnedRegistrationProduct(row),cells=[returnedRegistrationProductSelect(row,i),esc(p.brand),esc(p.model)];
  if(!consignment)cells.push(esc(p.ref),esc(p.serial));if(!consignment&&!returned)cells.push(esc(Array.isArray(p.accessories)?p.accessories.join(' / '):p.accessories));cells.push(p.cost==null?'—':esc(approvalDocumentMoney(p.cost,'JPY')));if(!returned)cells.push(field(row,i,'unitPriceMinor'));cells.push(remove(i));if(sale)cells.unshift(`<input type="checkbox" aria-label="明細${i+1}を売上計上" data-returned-row="${i}" data-row-key="_included" ${row._included===false?'':'checked'}>`);
  return `<tr data-returned-line="${i}">${cells.map(c=>`<td>${c}</td>`).join('')}</tr>`;
 }).join('');html+='</tbody></table></div>';
 document.getElementById('returned-registration-lines').innerHTML=html;
 returnedRevisionConfigureRows();enhanceSearchablePartnerSelects();initInputFormatListeners();returnedRegistrationSyncHeaders();returnedRegistrationTotals();
}
function returnedRegistrationProductChanged(index){if(!document.querySelector(`[data-returned-row="${index}"][data-row-key="productCode"]`)?.value)return;returnedFullProductChanged(index);if(!returnedCaptureRows(false))return;returnedRenderRows();}
function returnedRegistrationTotals(){
 const form=document.getElementById('returned-approval-form'),target=document.getElementById('returned-registration-totals');if(!form||!target)return;
 const value=key=>form.querySelector(`[data-returned-header="${key}"]`)?.value||'',type=returnedApprovalEditor.approval.targetType,sums=new Map(),taxes=new Map();let count=0;
 returnedApprovalEditor.rows.forEach((row,i)=>{
  const get=key=>form.querySelector(`[data-returned-row="${i}"][data-row-key="${key}"]`)?.value??row[key];if(form.querySelector(`[data-returned-row="${i}"][data-row-key="_included"]`)?.checked===false)return;count++;
  const currency=type==='purchase_slip'?get('costCurrency'):type==='consignment_slip'?(returnedApprovalEditor.revisionKind?returnedApprovalEditor.document.displayCurrency||'USD':'USD'):value('displayCurrency')||'JPY';let amount=type==='purchase_slip'?Number(get('unitCostMinor')):type==='return_slip'?Number(returnedRegistrationProduct(row).cost||0):Number(get('unitPriceMinor'));
  if(!Number.isFinite(amount))amount=0;sums.set(currency,(sums.get(currency)||0)+amount);const rate=type==='purchase_slip'?(value('taxCategory')==='consumption_tax'?10:0):type==='sales_slip'&&currency==='JPY'&&value('taxMode')==='taxable'?Number(value('taxRateBasisPoints')):0;taxes.set(currency,(taxes.get(currency)||0)+(type==='purchase_slip'?Math.floor(amount*rate/100):Math.round(amount*rate/100)));
  const taxCell=form.querySelector(`[data-returned-line-tax="${i}"]`);if(taxCell){const taxed=value('taxCategory')==='consumption_tax';taxCell.textContent=taxed?approvalDocumentMoney(Math.floor(amount*.1),currency):returnedReviewValue('taxCategory',value('taxCategory'));}
 });
 target.innerHTML=`<div class="slip-total-row"><span class="slip-total-label">明細 ${count} 点</span></div>`+[...sums].map(([currency,amount])=>{const rate=type==='purchase_slip'?(value('taxCategory')==='consumption_tax'?10:0):type==='sales_slip'&&currency==='JPY'&&value('taxMode')==='taxable'?Number(value('taxRateBasisPoints')):0;const tax=taxes.get(currency)||0;return `<div class="slip-total-row"><span class="slip-total-label">合計金額（${currency}・税抜）</span><span class="slip-total-val">${approvalDocumentMoney(amount,currency)}</span></div>${rate?`<div class="slip-total-row"><span>消費税（${rate}%）</span><span>${approvalDocumentMoney(tax,currency)}</span></div><div class="slip-total-row slip-total-grand"><span>税込合計</span><strong>${approvalDocumentMoney(amount+tax,currency)}</strong></div>`:''}`;}).join('');
}
function returnedOpenProduct(index){
 if(!returnedCaptureRows(false))return;const row=returnedApprovalEditor.rows[index],doc=returnedApprovalEditor.document,esc=approvalDocumentEscape;
 const template=document.createElement('template');template.innerHTML=`<table><tbody>${returnedPurchaseProductFields(row,index)}</tbody></table>`;const editor=template.content.querySelector('[data-returned-product-index]');if(!editor)return;
 const field=key=>{const el=editor.querySelector(`[data-returned-product="${key}"]`);if(!el)return '';const label=el.closest('label');label.className='form-group';label.style='';const text=label.firstChild;if(text?.nodeType===3){const span=document.createElement('span');span.className='form-label';span.textContent=text.textContent;label.replaceChild(span,text);}if(el.tagName==='SELECT')el.setAttribute('data-partner-search','');return label.outerHTML;};
 const ro=(name,val)=>`<div class="form-group"><label class="form-label">${name}</label><input class="form-control auto-filled" readonly value="${esc(val||'—')}"></div>`;
 const headerValue=k=>document.querySelector(`[data-returned-header="${k}"]`)?.value||'';
 let overlay=document.createElement('div');overlay.id='returned-product-overlay';overlay.className='modal-overlay returned-product-overlay';overlay.innerHTML=`<div class="modal" role="dialog" aria-modal="true" aria-labelledby="returned-product-title"><div class="modal-header"><h3 id="returned-product-title"><i class="fa-solid fa-box-open"></i> 商品登録 — 明細 ${index+1}</h3><button type="button" class="modal-close" aria-label="閉じる" onclick="returnedCloseProduct(false)">×</button></div><div class="modal-body" data-returned-product-index="${index}"><div class="form-row cols-3">${field('shapeCode')}${field('markingCode')}${ro('商品コード',row.productCode||'承認後に発番')}${ro('仕入日',headerValue('purchaseDate'))}<label class="form-group"><span class="form-label">SKU</span><input class="form-control" data-fmt="half" data-returned-product="sku" value="${esc(row.sku||'')}"></label></div><div class="form-row cols-3">${ro('仕入先',returnedReviewValue('supplierCode',headerValue('supplierCode')))}${ro('仕入金額',approvalDocumentMoney(row.unitCostMinor,row.costCurrency))}${ro('想定売価',approvalDocumentMoney(row.baseSalePriceMinor,row.baseSaleCurrency))}</div><hr><div class="form-row cols-3">${['brandCode','modelNumber','referenceNumber','serialNumber','materialCode','movementCode','conditionCode','beltText','dialText'].map(field).join('')}</div><div class="form-row cols-2">${field('boxCode')}${field('weightGrams')}</div><div class="form-group"><label class="form-label">付属品</label>${editor.querySelector('.checkbox-group').outerHTML}</div><div class="form-row cols-2">${field('braceletQuantity')}${field('productType')}</div>${field('duplicateSerialReason')}${field('notes')}<p class="returned-error" id="returned-product-error"></p></div><div class="modal-footer"><button type="button" class="btn btn-outline" onclick="returnedCloseProduct(false)">キャンセル</button><button type="button" class="btn btn-primary" onclick="returnedCloseProduct(true)"><i class="fa-solid fa-check"></i> 確定</button></div></div>`;
 document.getElementById('returned-approval-form').appendChild(overlay);setAccessorySupplementInputs('returned-accessories-'+index,row.accessorySupplements||{});enhanceSearchablePartnerSelects();initInputFormatListeners();overlay.querySelector('.modal-close').focus();
}
function returnedCloseProduct(save){
 const overlay=document.getElementById('returned-product-overlay');if(!overlay)return;
 if(save){for(const input of overlay.querySelectorAll('input,select,textarea'))if(!input.reportValidity())return;if(!returnedCaptureRows())return;}
 overlay.remove();if(save)returnedRenderRows();
}

// Confirmed-document revision requests share the registration layout with draft resubmissions.
// Only fields supported by the existing revision API are editable; immutable IDs stay in the payload.
const returnedRevisionHeaders={
 purchase:['purchaseDate','supplierCode','staffCode','notes'],
 sale:['saleDate','buyerCode','notes'],
 shipment:['shipmentDate','buyerCode','notes'],
 consignment:['consignmentDate','consigneeCode','notes'],
};
function openReturnedRevisionEditor(req){
 const snap=req.detail.document,kind=snap.kind.split('.')[0],type={purchase:'purchase_slip',sale:'sales_slip',shipment:'shipment_slip',consignment:'consignment_slip'}[kind];
 const payload=JSON.parse(JSON.stringify(snap.payload||{})),doc=JSON.parse(JSON.stringify(snap.before||snap.after||{}));
 for(const key of returnedRevisionHeaders[kind])if(Object.hasOwn(payload,key)&&!(kind==='sale'&&key==='buyerCode'))doc[key]=payload[key];
 if(kind==='sale')payload.buyerCode=doc.buyerCode;
 doc.lines=(doc.lines||[]).map(line=>({...line,...(payload.lines||[]).find(change=>change.id===line.id)}));
 const loaded={document:doc,approval:{targetType:type,decisionNote:req.decisionNote||req.revisionComment||req.note||'—'}};
 returnedApprovalEditor={...loaded,id:req.id,revisionKind:kind,rows:doc.lines.map(line=>({...line,unitPriceMinor:Object.hasOwn(line,'unitPriceMinor')?line.unitPriceMinor:doc.displayCurrency==='JPY'?line.convertedSalePriceJpy:line.salePriceUsdMinor}))};
 documentChangeEditor={req,payload,registration:true};
 document.getElementById('apprDetailTitle').textContent=`棄却・差戻し修正 — ${doc.slipNumber}`;
 document.getElementById('apprDetailBody').innerHTML=`<form id="returned-approval-form">${returnedRegistrationLayout(loaded)}</form>`;
 returnedRenderRows();
 const form=document.getElementById('returned-approval-form');
 form.querySelector('[data-returned-field="reason"]').value=payload.reason||'';
 form.querySelector('#returned-approval-error').id='document-change-error';
 form.querySelector('.pe-line-add-controls')?.remove();form.querySelector('.add-row-btn')?.remove();
 form.querySelector('.returned-delivery')?.remove();
 for(const input of form.querySelectorAll('[data-returned-header]')){
  const key=input.dataset.returnedHeader;
  if(!returnedRevisionHeaders[kind].includes(key)||(kind==='purchase'&&key==='purchaseDate'))input.disabled=true;
 }
 form.querySelectorAll('[data-returned-switch],[data-returned-cost-currency]').forEach(button=>button.disabled=true);
 returnedRegistrationInit();
 document.getElementById('apprDetailBody').closest('.modal').style.maxWidth='1440px';
 document.getElementById('apprDetailFooter').innerHTML='<button class="btn btn-outline" onclick="closeApprovalDetail()">閉じる</button><button id="document-change-submit" class="btn btn-primary" onclick="resubmitDocumentChange()">修正して再申請</button>';
 document.getElementById('approvalDetailOverlay').classList.remove('hidden');
}
function returnedRevisionConfigureRows(){
 const state=returnedApprovalEditor;if(!state?.revisionKind)return;
 const form=document.getElementById('returned-approval-form'),kind=state.revisionKind;
 const allowed=kind==='purchase'?['sku','unitCostMinor','baseSalePriceMinor']:['unitPriceMinor'];
 const editableIDs=new Set((documentChangeEditor?.payload.lines||[]).map(line=>line.id));
 for(const input of form.querySelectorAll('[data-returned-row]')){
  if(!allowed.includes(input.dataset.rowKey)||!editableIDs.has(state.rows[Number(input.dataset.returnedRow)]?.id))input.disabled=true;
 }
 form.querySelectorAll('.returned-remove').forEach(button=>button.closest('td').hidden=true);
 const headings=form.querySelectorAll('.returned-entry-table thead th');if(headings.length)headings[headings.length-1].hidden=true;
 if(kind==='purchase')form.querySelectorAll('.pe-regbtn').forEach((button,i)=>{
  const row=state.rows[i],esc=approvalDocumentEscape;
  button.outerHTML=`<details class="returned-product-summary"><summary>商品情報</summary>${['brandName','modelNumber','referenceNumber','serialNumber','weightGrams'].map(key=>`<div><b>${esc(returnedLabels[key])}</b>：${esc(row[key]||'—')}</div>`).join('')}</details>`;
 });
 if(kind==='consignment'){
  const currency=state.document.displayCurrency||'USD';
  form.querySelectorAll('.returned-entry-table thead th')[4].textContent=`販売価格（${currency}）`;
  form.querySelectorAll('[data-row-key="unitPriceMinor"]').forEach(input=>{
   const shell=document.createElement('div'),unit=document.createElement('span');
   shell.className='pu-price-input-shell';unit.textContent=currency==='JPY'?'¥':currency==='HKD'?'HK$':'$';
   input.replaceWith(shell);shell.append(unit,input);
  });
 }
}
function returnedRevisionPayload(){
 const state=documentChangeEditor,form=document.getElementById('returned-approval-form');
 const payload=JSON.parse(JSON.stringify(state.payload)),kind=returnedApprovalEditor.revisionKind;
 for(const key of returnedRevisionHeaders[kind]){
  const input=form.querySelector(`[data-returned-header="${key}"]`);
  if(input&&!input.disabled)payload[key]=input.value;
 }
 const fields=kind==='purchase'?['sku','unitCostMinor','baseSalePriceMinor']:['unitPriceMinor'];
 payload.lines=(payload.lines||[]).map(line=>{
  const index=returnedApprovalEditor.rows.findIndex(row=>row.id===line.id),result={...line};
  for(const key of fields){const input=form.querySelector(`[data-returned-row="${index}"][data-row-key="${key}"]`);if(input&&!input.disabled)result[key]=input.type==='number'?Number(input.value):input.value;}
  return result;
 });
 payload.reason=form.querySelector('[data-returned-field="reason"]').value.trim();
 return payload;
}
