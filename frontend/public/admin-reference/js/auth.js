// =====================================================
// auth.js — ログインセッション・権限管理
// =====================================================

const SESSION_KEY = 'inv_session';

// ── セッション取得 ──
function getSession() {
  try {
    const s = sessionStorage.getItem(SESSION_KEY);
    return s ? JSON.parse(s) : null;
  } catch { return null; }
}

// ── セッション保存 ──
function setSession(user) {
  sessionStorage.setItem(SESSION_KEY, JSON.stringify(user));
}

// ── セッションクリア ──
function clearSession() {
  sessionStorage.removeItem(SESSION_KEY);
}

// ── 現在ユーザー ──
function currentUser() { return getSession(); }
function currentRole() { return getSession()?.role || null; }
function currentUserId() { return getSession()?.id || null; }

// ── 権限チェック ──
const ROLE_LEVEL = { admin: 3, buyer: 2, worker: 2, guest: 1 };
const ROLE_LABELS = { admin: '管理者', buyer: '作業者', worker: '作業者', guest: 'ゲスト' };
const ROLE_ADMIN_ONLY_PAGES = ['approval', 'password', 'company', 'login-info'];
const ROLE_ADMIN_AUTH_PAGES = ['master', 'box'];
const ROLE_WORKSPACE_PAGES = [
  'dashboard', 'market', 'market-entry', 'inventory', 'purchase-entry', 'purchase', 'sales',
  'sales-list', 'shipping', 'consignment', 'returns', 'master', 'box', 'performance',
  'stocktake', 'purchase-list', 'client',
];

function hasRole(requiredRole) {
  const level = ROLE_LEVEL[currentRole()] || 0;
  return level >= (ROLE_LEVEL[requiredRole] || 99);
}
function isAdmin()  { return currentRole() === 'admin';  }
function isWorker() { return currentRole() === 'buyer' || currentRole() === 'worker'; }
function isBuyer()  { return isWorker(); } // 旧プロトタイプ互換
function isGuest()  { return currentRole() === 'guest';  }
function currentRoleLabel() { return ROLE_LABELS[currentRole()] || '未ログイン'; }

/**
 * 金額・価格・伝票内容を直接確定する操作の共通ガード。
 * 未ログインの制作プレビューは従来どおり管理者相当とする。
 */
function requireAdminForSensitiveOperation(operationLabel) {
  if (!currentUser() || isAdmin()) return true;
  const message = `${operationLabel}には管理者権限または管理者承認が必要です。`;
  if (typeof showToast === 'function') {
    showToast('warning', '管理者承認が必要です', message);
  }
  return false;
}

/** 画面ごとのロール権限。未ログイン時は制作プレビューとして管理者相当で表示する。 */
function canAccessPage(page) {
  const role = currentRole();
  if (!role || role === 'admin') return true;
  if (isWorker()) return ROLE_WORKSPACE_PAGES.includes(page) && !ROLE_ADMIN_ONLY_PAGES.includes(page);
  return false;
}

function needsAdminAuthentication(page) {
  return isWorker() && ROLE_ADMIN_AUTH_PAGES.includes(page);
}

// ── ログイン処理（管理者・作業者） ──
function doAppLogin(loginId, password) {
  const user = APP_DATA.users.find(u => u.loginId === loginId && u.password === password && u.active !== false);
  if (!user) return null;
  setSession({ id: user.id, role: user.role, name: user.name, avatar: user.avatar, loginId: user.loginId });
  return user;
}

// ── ログアウト ──
async function doAppLogout() {
  if (window.ZaikoAPI) await window.ZaikoAPI.logout();
  else clearSession();
  window.location.href = 'index.html';
}

// ── ページロード時：ログイン必須チェック ──
function requireLogin() {
  if (!getSession()) { window.location.href = 'index.html'; return false; }
  return true;
}

// ── サイドバー・UIをロールに合わせて制御 ──
function applyRoleUI() {
  const user = currentUser();

  // セッションがない場合は data-admin-only を表示したまま（開発環境・プレビュー対応）
  if (!user) {
    document.querySelectorAll('[data-admin-only]').forEach(el => {
      el.style.display = '';
    });
    document.querySelectorAll('[data-buyer-only]').forEach(el => {
      el.style.display = '';
    });
    return;
  }

  // アバター・ユーザー名更新
  const avatarEl = document.querySelector('.sidebar-user .avatar');
  const nameEl   = document.querySelector('.sidebar-user div div:first-child');
  const roleEl   = document.querySelector('.sidebar-user div div:last-child');
  if (avatarEl) avatarEl.textContent = user.avatar || user.name.slice(0, 1);
  if (nameEl)   nameEl.textContent   = user.name;
  if (roleEl)   roleEl.textContent   = currentRoleLabel();

  // app-wrapper に role クラスを付与
  const wrapper = document.querySelector('.app-wrapper');
  if (wrapper) {
    wrapper.classList.toggle('role-admin', user.role === 'admin');
    wrapper.classList.toggle('role-worker', isWorker());
    wrapper.classList.toggle('role-buyer', isWorker()); // 旧CSS互換
  }

  // data-admin-only: 管理者のみ表示
  document.querySelectorAll('[data-admin-only]').forEach(el => {
    el.style.display = isAdmin() ? '' : 'none';
  });

  // data-buyer-only: 管理者または作業者に表示（旧属性名互換）
  document.querySelectorAll('[data-buyer-only]').forEach(el => {
    el.style.display = (isAdmin() || isBuyer()) ? '' : 'none';
  });

  document.querySelectorAll('[data-worker-only]').forEach(el => {
    el.style.display = isWorker() ? '' : 'none';
  });

  document.querySelectorAll('.nav-item[data-page]').forEach(el => {
    el.style.display = canAccessPage(el.dataset.page) ? '' : 'none';
  });

  // 作業者には承認が必要な操作に警告バッジ
  document.querySelectorAll('[data-buyer-restricted]').forEach(el => {
    const existing = el.querySelector('.badge-otp');
    if (isWorker() && !existing) {
      el.insertAdjacentHTML('beforeend', ' <span class="badge-otp"><i class="fa-solid fa-shield-halved"></i> 要承認</span>');
    } else if (!isWorker() && existing) {
      existing.remove();
    }
  });
}
