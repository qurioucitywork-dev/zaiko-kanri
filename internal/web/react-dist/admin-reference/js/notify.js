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
  if (!ntf) return;
  if (window.ZaikoAPI && ntf.apiManaged) {
    try {
      await window.ZaikoAPI.markNotificationRead(ntfId);
    } catch (error) {
      showToast('error', '通知を既読にできませんでした', error.message || '通信状態を確認してください');
      return;
    }
  }
  ntf.read = true;
  updateNotifyBadge();
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
  updateNotifyBadge();
  renderNotifyList();
}

// ── 通知パネル開閉 ──
function toggleNotifyPanel() {
  const panel = document.getElementById('notifyPanel');
  if (!panel) return;
  const isOpen = !panel.classList.contains('hidden');
  if (isOpen) {
    panel.classList.add('hidden');
  } else {
    renderNotifyList();
    panel.classList.remove('hidden');
  }
}

// ── 通知リストレンダリング ──
function renderNotifyList() {
  const container = document.getElementById('notifyList');
  if (!container) return;

  const uid = currentUserId();
  const myNotifs = APP_DATA.notifications
    .filter(n => n.toUserId === uid)
    .slice()
    .reverse();

  if (myNotifs.length === 0) {
    container.innerHTML = '<div class="notify-empty">通知はありません</div>';
    return;
  }

  container.innerHTML = myNotifs.map(n => {
    const typeIcon = {
      approval_request: '<i class="fa-solid fa-clipboard-check" style="color:#4f8ef7"></i>',
      approved: '<i class="fa-solid fa-circle-check" style="color:#2ecc71"></i>',
      revision: '<i class="fa-solid fa-rotate-left" style="color:#e07b39"></i>',
      otp: '<i class="fa-solid fa-key" style="color:#9b59b6"></i>',
      system: '<i class="fa-solid fa-bell" style="color:#888"></i>',
    }[n.type] || '<i class="fa-solid fa-bell"></i>';

    const otpBtn = n.type === 'approved' && n.relatedId
      ? `<button class="btn btn-sm btn-primary mt-6" onclick="openOTPFromNotify('${n.relatedId}');markRead('${n.id}');renderNotifyList();">
           <i class="fa-solid fa-key"></i> OTP入力
         </button>`
      : '';

    return `<div class="notify-item ${n.read ? '' : 'unread'}" onclick="markRead('${n.id}');renderNotifyList();">
      <div class="notify-icon">${typeIcon}</div>
      <div class="notify-content">
        <div class="notify-title">${n.title}</div>
        <div class="notify-body">${n.body}</div>
        <div class="notify-time">${n.createdAt}</div>
        ${otpBtn}
      </div>
      ${!n.read ? '<span class="notify-dot"></span>' : ''}
    </div>`;
  }).join('');
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
