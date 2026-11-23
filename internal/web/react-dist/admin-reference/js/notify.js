// =====================================================
// notify.js — 通知・インボックスロジック
// =====================================================

let _notifyCounter = APP_DATA.notifications.length + 1;

// ── 通知作成 ──
function createNotification({ toUserId, fromUserId, fromName, type, title, body, relatedId = null }) {
  const id = 'NTF-' + String(_notifyCounter++).padStart(3, '0');
  const ntf = {
    id,
    toUserId,
    fromUserId,
    fromName,
    type,     // approval_request / approved / revision / otp / system
    title,
    body,
    relatedId,
    read: false,
    createdAt: new Date().toLocaleString('ja-JP'),
  };
  APP_DATA.notifications.push(ntf);
  updateNotifyBadge();
  return ntf;
}

// ── 現在ユーザー宛の未読通知数 ──
function getUnreadCount() {
  const uid = currentUserId();
  if (!uid) return 0;
  return APP_DATA.notifications.filter(n => n.toUserId === uid && !n.read).length;
}

// ── 通知バッジ更新 ──
function updateNotifyBadge() {
  const badge = document.getElementById('notifyBadge');
  if (!badge) return;
  const count = getUnreadCount();
  badge.textContent = count;
  badge.style.display = count > 0 ? 'inline-flex' : 'none';
}

// ── 通知を既読にする ──
async function markRead(ntfId) {
  const ntf = APP_DATA.notifications.find(n => n.id === ntfId);
  if (!ntf) return false;
  if (window.ZaikoAPI && ntf.apiManaged) {
    try {
      await window.ZaikoAPI.markNotificationRead(ntfId);
    } catch (error) {
      showToast('error', '通知を既読にできませんでした', error.message || '通信状態を確認してください');
      return false;
    }
  }
  ntf.read = true;
  updateNotifyBadge();
  renderWorkerNotificationList();
  return true;
}

// ── 通知項目を開く ──
let notificationOpening = false;
async function openNotification(ntfId) {
  if (notificationOpening) return;
  const ntf = APP_DATA.notifications.find(n => n.id === ntfId && n.toUserId === currentUserId());
  if (!ntf) return;
  const kind = ntf.targetType === 'purchase_request' || ntf.type === 'purchase_request.created' ? 'purchase'
    : ntf.targetType === 'approval_request' || ['approval_request', 'approval.requested', 'approval.approved', 'approval.returned', 'approval.rejected', 'approved', 'revision'].includes(ntf.type) ? 'approval' : '';
  notificationOpening = true;
  try {
    if (ntf.targetType === 'exchange_rate' && isAdmin()) {
      await window.ZaikoAPI?.refreshExchangeRates();
      navigateTo('master');
      switchMasterTab('fxrate');
      document.getElementById('notifyPanel')?.classList.add('hidden');
      await markRead(ntfId); renderNotifyList(); return;
    }
    if (ntf.targetType === 'guest_inquiry' && isAdmin()) {
      if (!await openGuestInquiryDetail(ntf.targetId || ntf.relatedId)) return;
      document.getElementById('notifyPanel')?.classList.add('hidden');
      await markRead(ntfId);renderNotifyList();return;
    }
    if (kind && (isAdmin() || (kind === 'approval' && isWorker()))) {
      if (ntf.apiManaged && window.ZaikoAPI?.refreshNotificationTarget) await window.ZaikoAPI.refreshNotificationTarget(kind);
      const targetId = ntf.targetId || ntf.relatedId;
      const records = kind === 'purchase' ? APP_DATA.purchaseRequests : APP_DATA.approvalRequests;
      const target = targetId && records.find(record => record._id === targetId || record.id === targetId);
      if (!target || (isWorker() && _approvalRequesterId(target) !== currentUserId())) {
        showToast('error', '該当リクエストが見つかりません', '通知は未読のままです。更新してから再度確認してください。');
        return;
      }
      if (isWorker()) {
        navigateTo('notifications');
        if (!showWorkerApprovalResult(target.id)) return;
      } else {
        navigateTo(kind === 'purchase' ? 'purchase-list' : 'approval');
        if (kind === 'purchase') openPrDetail(target.id); else openApprovalDetail(target.id);
      }
      document.getElementById('notifyPanel')?.classList.add('hidden');
    }
    await markRead(ntfId);
    renderNotifyList();
  } catch (error) {
    showToast('error', '通知の詳細を開けませんでした', error.message || '通信状態を確認してください');
  } finally { notificationOpening = false; }
}

// ── すべて既読 ──
async function markAllRead() {
  const uid = currentUserId();
  const targets = APP_DATA.notifications.filter(n => n.toUserId === uid && !n.read);
  if (window.ZaikoAPI) {
    try {
      await Promise.all(targets.filter(n => n.apiManaged).map(n => window.ZaikoAPI.markNotificationRead(n.id)));
    } catch (error) {
      showToast('error', '通知を既読にできませんでした', error.message || '通信状態を確認してください');
      return;
    }
  }
  targets.forEach(n => { n.read = true; });
  renderWorkerNotificationList();
  updateNotifyBadge();
  renderNotifyList();
}

// ── 通知パネル開閉 ──
async function toggleNotifyPanel() {
  const panel = document.getElementById('notifyPanel');
  if (!panel) return;
  if (!panel.classList.contains('hidden')) { panel.classList.add('hidden'); return; }
  panel.classList.remove('hidden');
  if (window.ZaikoAPI?.refreshNotifications) {
    document.getElementById('notifyList').innerHTML = '<div class="notify-empty">通知を読み込んでいます…</div>';
    try {
      await window.ZaikoAPI.refreshNotifications();
      updateNotifyBadge();
    } catch {
      document.getElementById('notifyList').innerHTML = '<div class="notify-empty" role="alert">通知を取得できませんでした。ベルを開き直して再試行してください。</div>';
      return;
    }
  }
  renderNotifyList();
}

function notifyEscape(value) {
  return String(value ?? '').replace(/[&<>"']/g, char => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[char]));
}

// ── 通知リストレンダリング ──
function renderNotifyList() {
  const container = document.getElementById('notifyList');
  if (!container) return;

  const uid = currentUserId();
  const myNotifs = APP_DATA.notifications
    .filter(n => n.toUserId === uid && !n.read)
    .slice()
    .sort((a,b) => (Date.parse(b.createdAt) || 0) - (Date.parse(a.createdAt) || 0));

  if (myNotifs.length === 0) {
    container.innerHTML = '<div class="notify-empty">未確認の通知はありません</div>';
    return;
  }

  container.innerHTML = myNotifs.map(n => {
    const typeIcon = {
      'purchase_request.created': '<i class="fa-solid fa-cart-shopping" style="color:#287fb4"></i>',
      'approval.approved': '<i class="fa-solid fa-circle-check" style="color:#2ecc71"></i>',
      'approval.returned': '<i class="fa-solid fa-rotate-left" style="color:#e07b39"></i>',
      'approval.rejected': '<i class="fa-solid fa-circle-xmark" style="color:#e74c3c"></i>',
      'approval.requested': '<i class="fa-solid fa-clipboard-check" style="color:#4f8ef7"></i>',
      approval_request: '<i class="fa-solid fa-clipboard-check" style="color:#4f8ef7"></i>',
      approved: '<i class="fa-solid fa-circle-check" style="color:#2ecc71"></i>',
      revision: '<i class="fa-solid fa-rotate-left" style="color:#e07b39"></i>',
      otp: '<i class="fa-solid fa-key" style="color:#9b59b6"></i>',
      system: '<i class="fa-solid fa-bell" style="color:#888"></i>',
    }[n.type] || '<i class="fa-solid fa-bell"></i>';

    const otpBtn = n.type === 'approved' && n.relatedId
      ? `<button class="btn btn-sm btn-primary mt-6" data-otp-request="${notifyEscape(n.relatedId)}">
           <i class="fa-solid fa-key"></i> OTP入力
         </button>`
      : '';

    return `<div class="notify-item ${n.read ? '' : 'unread'}" role="button" tabindex="0" data-notification-id="${notifyEscape(n.id)}">
      <div class="notify-icon">${typeIcon}</div>
      <div class="notify-content">
        <div class="notify-title">${notifyEscape(n.title)}</div>
        <div class="notify-body">${notifyEscape(n.body)}</div>
        <div class="notify-time">${notifyEscape(Number.isNaN(Date.parse(n.createdAt)) ? n.createdAt : new Date(n.createdAt).toLocaleString('ja-JP'))}</div>
        ${otpBtn}
      </div>
      ${!n.read ? '<span class="notify-dot"></span>' : ''}
    </div>`;
  }).join('');
  container.querySelectorAll('[data-notification-id]').forEach(item => {
    item.addEventListener('click', () => openNotification(item.dataset.notificationId));
    item.addEventListener('keydown', event => {
      if (event.target !== item || !['Enter', ' '].includes(event.key)) return;
      event.preventDefault(); openNotification(item.dataset.notificationId);
    });
    item.querySelector('[data-otp-request]')?.addEventListener('click', async event => {
      event.stopPropagation(); openOTPFromNotify(event.currentTarget.dataset.otpRequest);
      await markRead(item.dataset.notificationId); renderNotifyList();
    });
  });
}

// ── 通知からOTPモーダルを開く ──
function openOTPFromNotify(reqId) {
  const req = APP_DATA.approvalRequests.find(r => r.id === reqId);
  if (!req) { showToast('error', '承認リクエストが見つかりません', ''); return; }

  // OTPモーダルを開く（onSuccessはrequestTypeに応じて実行）
  openOTPModal(reqId, function() {
    showToast('success', '認証完了', `${req.typeLabel}の操作が承認されました`);
    // 必要に応じてページ遷移や処理を追加
  });

  // 通知パネルを閉じる
  const panel = document.getElementById('notifyPanel');
  if (panel) panel.classList.add('hidden');
}

// ── ページ外クリックで通知パネルを閉じる ──
document.addEventListener('click', function(e) {
  const panel = document.getElementById('notifyPanel');
  const btn = document.getElementById('notifyBtn');
  if (!panel || panel.classList.contains('hidden')) return;
  if (!panel.contains(e.target) && btn && !btn.contains(e.target)) {
    panel.classList.add('hidden');
  }
});


// Worker history is based on the persistent approval records, so reading a bell
// notification never removes an approval result from this page.
function workerApprovalStatus(status) {
  return ({approved:'承認済', returned:'差戻し', revision:'差戻し', rejected:'却下', cancelled:'取消済', pending:'承認待ち'})[status] || status;
}
function workerNotificationTime(value) {
  return value ? (Number.isNaN(Date.parse(value)) ? String(value) : new Date(value).toLocaleString('ja-JP')) : '—';
}
function workerApprovalNotifications(id) {
  return (APP_DATA.notifications || []).filter(n => n.toUserId === currentUserId() && (n.targetId || n.relatedId) === id);
}
function renderWorkerNotificationList() {
  const container = document.getElementById('workerNotificationList');
  if (!container || !isWorker()) return;
  const results = (APP_DATA.approvalRequests || []).filter(req => _approvalRequesterId(req) === currentUserId() && ['approved','returned','revision','rejected','cancelled'].includes(req.status))
    .sort((a,b) => (Date.parse(b.decidedAt || b.createdAt)||0)-(Date.parse(a.decidedAt || a.createdAt)||0));
  if (!results.length) { container.innerHTML = '<div class="notify-empty">承認結果の通知はありません</div>'; return; }
  container.innerHTML = '<div class="worker-results-scroll"><table class="data-table worker-results-table" aria-label="自分の承認結果"><thead><tr><th scope="col">申請日時</th><th scope="col">承認者</th><th scope="col">種別</th><th scope="col">内容（クリックで詳細）</th><th scope="col">ステータス</th></tr></thead><tbody>' + results.map(req => {
    const unread=workerApprovalNotifications(req.id).some(n=>!n.read);
    const [badge,icon]=({approved:['approved','circle-check'],returned:['revision','rotate-left'],revision:['revision','rotate-left'],rejected:['revision','circle-xmark'],cancelled:['cancelled','ban']})[req.status]||['pending','clock'];
    return `<tr class="appr-row worker-result-row ${unread?'unread':''}" tabindex="0" data-approval-result="${notifyEscape(req.id)}" title="クリックで詳細表示" aria-label="${notifyEscape(approvalDisplayType(req))}：${notifyEscape(workerApprovalStatus(req.status))}、詳細を表示">
      <td class="worker-result-time"><i class="fa-solid fa-calendar-days" aria-hidden="true"></i>${notifyEscape(formatApprovalRequestTime(req.createdAt))}</td>
      <td class="worker-result-requester">${notifyEscape(req.deciderName||req.approvedByName||'—')}</td>
      <td><span class="appr-type-badge">${typeIcon(req.type)} ${notifyEscape(approvalDisplayType(req))}</span></td>
      <td class="appr-summary-cell">${buildDetailSummary(req)}</td>
      <td class="worker-result-status"><span class="badge-approval ${badge}"><i class="fa-solid fa-${icon}" aria-hidden="true"></i> ${notifyEscape(workerApprovalStatus(req.status))}</span>${unread?'<span class="worker-result-unread" aria-label="未確認" title="未確認"></span>':''}</td>
    </tr>`;
  }).join('') + '</tbody></table></div>';
  container.querySelectorAll('[data-approval-result]').forEach(row => {
    row.addEventListener('click',()=>openWorkerApprovalResult(row.dataset.approvalResult));
    row.addEventListener('keydown',event=>{if(event.key==='Enter'||event.key===' '){event.preventDefault();openWorkerApprovalResult(row.dataset.approvalResult);}});
  });
}
let workerNotificationRefresh = null;
async function init_notifications() {
  if (!isWorker()) return;
  if (workerNotificationRefresh) return workerNotificationRefresh;
  workerNotificationRefresh = (async()=>{
    try {
      if (window.ZaikoAPI?.refreshNotifications) await Promise.all([window.ZaikoAPI.refreshNotifications(),window.ZaikoAPI.refreshNotificationTarget('approval')]);
      renderWorkerNotificationList(); updateNotifyBadge(); renderNotifyList();
    } catch (error) {
      const container=document.getElementById('workerNotificationList');
      if(container) container.innerHTML='<div class="notify-empty" role="alert">通知を取得できませんでした。「更新」で再試行してください。</div>';
    }
  })();
  try { await workerNotificationRefresh; } finally {workerNotificationRefresh=null;}
}
function showWorkerApprovalResult(id) {
  const req=(APP_DATA.approvalRequests||[]).find(r=>r.id===id && _approvalRequesterId(r)===currentUserId());
  if (!isWorker() || !req) return false;
  const fields=[['申請ID',req.id],['申請内容',approvalDisplayType(req)],['申請日時',workerNotificationTime(req.createdAt)],['承認結果',workerApprovalStatus(req.status)],['確認した管理者',req.deciderName || req.approvedByName || '—'],['確認日時',workerNotificationTime(req.decidedAt || req.approvedAt)],['管理者コメント',req.decisionNote || req.revisionComment || req.note || '—']];
  if(req.detail?.targetType)fields.push(['対象',approvalTargetTypeLabel(req.detail.targetType)]);
  const approvalDocument=approvalDocumentSnapshot(req.detail||{});
  if(approvalDocument)fields.push(['伝票No',approvalDocument.slipNumber]);
  else if(req.detail?.targetId) {
    const target=[...(APP_DATA.purchaseSlips||[]),...(APP_DATA.sales||[]),...(APP_DATA.shipments||[]),...(APP_DATA.consignments||[])].find(r=>r._id===req.detail.targetId || r.id===req.detail.targetId);
    fields.push(['対象伝票・申請',target?.id || req.detail.targetId]);
  }
  document.getElementById('apprDetailTitle').textContent=approvalDisplayType(req)+' — 承認結果';
  document.getElementById('apprDetailIcon').innerHTML='<i class="fa-solid fa-clipboard-check"></i>';
  document.getElementById('apprDetailBody').innerHTML='<div class="appr-detail-meta">'+fields.map(([label,value])=>`<div class="appr-meta-row"><span class="appr-meta-label">${notifyEscape(label)}</span><span class="appr-meta-val" style="white-space:pre-wrap;">${notifyEscape(value)}</span></div>`).join('')+'</div>';
  document.getElementById('apprDetailBody').innerHTML+=buildApprovalDocumentDetail(req.detail||{})||'';
  document.getElementById('apprDetailBody').closest('.modal').style.maxWidth=approvalDocument?'1000px':'640px';
  document.getElementById('apprDetailFooter').innerHTML='<button type="button" class="btn btn-outline" onclick="closeApprovalDetail()">閉じる</button>'+returnedApprovalButton(req)+(req.status==='approved' && req.detail?.targetType==='document_change' && req.detail.document?.kind?.endsWith('.issue') ? '<button class="btn btn-primary" onclick="downloadApprovedDocument(\''+notifyEscape(req.id)+'\')">承認済み伝票PDF</button>':'');
  document.getElementById('approvalDetailOverlay').classList.remove('hidden');
  return true;
}
async function openWorkerApprovalResult(id) {
  if(!isWorker())return;
  try {
    if(window.ZaikoAPI?.refreshNotificationTarget)await window.ZaikoAPI.refreshNotificationTarget('approval');
    if(!showWorkerApprovalResult(id)){showToast('error','承認結果が見つかりません','更新してから再度確認してください。');return;}
    for(const n of workerApprovalNotifications(id).filter(n=>!n.read))await markRead(n.id);
    renderNotifyList();renderWorkerNotificationList();
  } catch(error){showToast('error','承認結果を開けませんでした',error.message || '通信状態を確認してください。');}
}
async function refreshWorkerNotificationsWhenActive() {
  if (document.visibilityState === 'hidden' || !isWorker() || !window.ZaikoAPI?.state?.hydrated) return;
  await init_notifications();
}
window.setInterval(refreshWorkerNotificationsWhenActive,30000);
window.addEventListener('focus',refreshWorkerNotificationsWhenActive);
document.addEventListener('visibilitychange',refreshWorkerNotificationsWhenActive);

async function openGuestInquiryDetail(id) {
 if(!isAdmin()||!window.ZaikoAPI)return false;
 const item=await window.ZaikoAPI.request(`/inquiries/${encodeURIComponent(id)}`);
 const fields=[['送信日時',new Date(item.createdAt).toLocaleString('ja-JP')],['ゲスト',`${item.guestName} (${item.guestCode})`],['取引先',item.companyName],['登録メールアドレス',item.email||'—'],['件名',item.subject],['管理番号',item.productCode||'—'],['お問い合わせ内容',item.message]];
 document.getElementById('apprDetailTitle').textContent='ゲストからのお問い合わせ';
 document.getElementById('apprDetailIcon').innerHTML='<i class="fa-solid fa-envelope"></i>';
 document.getElementById('apprDetailBody').innerHTML=fields.map(([label,value])=>`<div class="appr-meta-row"><span class="appr-meta-label">${notifyEscape(label)}</span><span class="appr-meta-val" style="white-space:pre-wrap;overflow-wrap:anywhere;">${notifyEscape(value)}</span></div>`).join('');
 document.getElementById('apprDetailBody').closest('.modal').style.maxWidth='850px';
 document.getElementById('apprDetailFooter').innerHTML='<button type="button" class="btn btn-outline" onclick="closeApprovalDetail()">閉じる</button>';
 document.getElementById('approvalDetailOverlay').classList.remove('hidden');return true;
}
