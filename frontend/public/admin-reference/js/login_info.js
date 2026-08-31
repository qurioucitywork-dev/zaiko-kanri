// =====================================================
// ログイン情報・顧客情報管理（ローカルプロトタイプ）
// =====================================================

const LOGIN_DIRECTORY_STORAGE_KEY = 'inv_login_directory_v1';

let loginInfoState = {
  role: 'admin',
  query: '',
  editingType: null,
  editingId: null,
};

function loginInfoEscapeHtml(value) {
  return String(value ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}

function hydrateLoginDirectory() {
  try {
    const stored = JSON.parse(localStorage.getItem(LOGIN_DIRECTORY_STORAGE_KEY) || 'null');
    if (!stored || typeof stored !== 'object') return false;
    if (Array.isArray(stored.users) && stored.users.length) APP_DATA.users = stored.users;
    if (Array.isArray(stored.guestAccounts)) APP_DATA.guestAccounts = stored.guestAccounts;
    if (Array.isArray(stored.buyers)) APP_DATA.buyers = stored.buyers;
    if (Array.isArray(stored.clientCompanies)) APP_DATA.clientCompanies = stored.clientCompanies;
    return true;
  } catch (error) {
    console.warn('ログイン情報の復元に失敗しました', error);
    return false;
  }
}

function persistLoginDirectory() {
  reconcileGuestBuyerDirectory();
  if (typeof reconcileClientCompanyDirectory === 'function') {
    reconcileClientCompanyDirectory({ persist: true });
  }
  const snapshot = {
    version: 1,
    updatedAt: new Date().toISOString(),
    users: APP_DATA.users,
    guestAccounts: APP_DATA.guestAccounts,
    buyers: APP_DATA.buyers,
    clientCompanies: APP_DATA.clientCompanies || [],
  };
  localStorage.setItem(LOGIN_DIRECTORY_STORAGE_KEY, JSON.stringify(snapshot));
  window.dispatchEvent(new CustomEvent('login-directory-updated', { detail: snapshot }));
  return snapshot;
}

function getBuyerMasterRecords(extraCodes = []) {
  const records = (APP_DATA.buyers || []).map(buyer => ({ ...buyer }));
  (extraCodes || []).map(code => String(code || '').trim().toUpperCase()).filter(Boolean).forEach(code => {
    if (!records.some(buyer => buyer.code === code)) records.push({ code, name: code, address: '', contact: '', invoice: '' });
  });
  return records;
}

function getBuyerChannel(buyerCode) {
  const guest = (APP_DATA.guestAccounts || []).find(account => account.buyerCode === buyerCode);
  return guest
    ? { type: 'guest', label: guest.active === false ? 'ゲスト停止中' : 'ゲスト発行済', guest }
    : { type: 'direct', label: '直取引（未発行）', guest: null };
}

function populateBuyerMasterSelect(id, options = {}) {
  const select = document.getElementById(id);
  if (!select) return;
  const emptyLabel = Object.prototype.hasOwnProperty.call(options, 'emptyLabel') ? options.emptyLabel : '-- 選択 --';
  const selected = Object.prototype.hasOwnProperty.call(options, 'selected') ? String(options.selected || '') : select.value;
  const records = getBuyerMasterRecords(options.extraCodes || []);
  const emptyOption = emptyLabel === null ? '' : `<option value="">${loginInfoEscapeHtml(emptyLabel)}</option>`;
  select.innerHTML = emptyOption + records.map(buyer => {
    return `<option value="${loginInfoEscapeHtml(buyer.code)}">${loginInfoEscapeHtml(buyer.name)}</option>`;
  }).join('');
  if (records.some(buyer => buyer.code === selected)) select.value = selected;
  else if (emptyLabel !== null) select.value = '';
}

function ensureBuyerForGuest(guest, details = {}) {
  if (!guest) return null;
  let buyerCode = String(guest.buyerCode || details.code || '').trim().toUpperCase();
  if (!buyerCode) buyerCode = loginInfoNextId('B', APP_DATA.buyers || [], 'code');
  guest.buyerCode = buyerCode;
  let buyer = (APP_DATA.buyers || []).find(record => record.code === buyerCode);
  if (!buyer) {
    buyer = { code: buyerCode, name: '', address: '', contact: '', invoice: '', email: '', guestManaged: true };
    APP_DATA.buyers.push(buyer);
  }
  const fallbackName = String(guest.company || guest.name || buyerCode).trim();
  const fields = {
    name: Object.prototype.hasOwnProperty.call(details, 'name') ? details.name : (buyer.name || fallbackName),
    address: Object.prototype.hasOwnProperty.call(details, 'address') ? details.address : (buyer.address || ''),
    contact: Object.prototype.hasOwnProperty.call(details, 'contact') ? details.contact : (buyer.contact || ''),
    invoice: Object.prototype.hasOwnProperty.call(details, 'invoice') ? details.invoice : (buyer.invoice || ''),
    email: Object.prototype.hasOwnProperty.call(details, 'email') ? details.email : (buyer.email || guest.email || ''),
  };
  Object.assign(buyer, { code: buyerCode, guestManaged: true }, fields);
  if (typeof syncClientCompanyFromBuyer === 'function') {
    syncClientCompanyFromBuyer(buyer, { preferTrade: true, guest });
  }
  return buyer;
}

function buildGuestAccountForBuyer(buyer) {
  if (!buyer) return null;
  const existing = (APP_DATA.guestAccounts || []).find(guest => guest.buyerCode === buyer.code);
  if (existing) {
    buyer.guestManaged = true;
    return existing;
  }
  const guestId = loginInfoNextId('G', APP_DATA.guestAccounts || []);
  const guest = {
    id: guestId,
    name: buyer.name || buyer.code,
    password: `Guest-${buyer.code}-01`,
    company: buyer.name || buyer.code,
    buyerCode: buyer.code,
    email: buyer.email || `guest.${String(buyer.code || guestId).toLowerCase()}@local.invalid`,
    active: true,
    autoProvisioned: true,
  };
  APP_DATA.guestAccounts.push(guest);
  buyer.guestManaged = true;
  return guest;
}

function setBuyerGuestManaged(buyerCode, managed, options = {}) {
  const buyer = (APP_DATA.buyers || []).find(record => record.code === buyerCode);
  if (buyer) buyer.guestManaged = Boolean(managed);
  if (!managed && options.revokeBoxes !== false) {
    (APP_DATA.boxes || []).forEach(box => {
      box.publicTo = (box.publicTo || []).filter(code => code !== buyerCode);
    });
    (APP_DATA.publishedSnapshot?.boxes || []).forEach(box => {
      box.publicTo = (box.publicTo || []).filter(code => code !== buyerCode);
    });
    if (typeof persistGuestBoxState === 'function') persistGuestBoxState();
    if (typeof persistGuestSnapshot === 'function') persistGuestSnapshot();
  }
  return buyer;
}

function getGuestManagedBuyers() {
  const guestBuyerCodes = new Set((APP_DATA.guestAccounts || []).map(guest => guest.buyerCode).filter(Boolean));
  return (APP_DATA.buyers || []).filter(buyer => guestBuyerCodes.has(buyer.code));
}

function reconcileGuestBuyerDirectory() {
  if (!Array.isArray(APP_DATA.buyers)) APP_DATA.buyers = [];
  if (!Array.isArray(APP_DATA.guestAccounts)) APP_DATA.guestAccounts = [];
  APP_DATA.guestAccounts.forEach(guest => ensureBuyerForGuest(guest));
  // 旧版のゲスト管理は販売先を全件表示していたため、管理区分がない既存行は
  // ゲスト管理済みとして一度だけログイン情報を補完する。新規の直取引先は false を保持する。
  APP_DATA.buyers.forEach(buyer => {
    if (buyer.guestManaged !== false) buildGuestAccountForBuyer(buyer);
  });
  return APP_DATA.buyers;
}

// data.js の直後に読み込むことで、ログイン画面と管理画面の双方へ保存値を反映する。
hydrateLoginDirectory();
reconcileGuestBuyerDirectory();
persistLoginDirectory();

function loginInfoRoleLabel(role) {
  return role === 'admin' ? '管理者' : role === 'guest' ? 'ゲスト' : '作業者';
}

function loginInfoNextId(prefix, records, field = 'id') {
  const values = records
    .map(record => String(record[field] || ''))
    .filter(value => value.toUpperCase().startsWith(prefix.toUpperCase()) && /^\d+$/.test(value.slice(prefix.length)))
    .map(value => Number(value.slice(prefix.length)))
    .filter(Number.isFinite);
  const next = values.length ? Math.max(...values) + 1 : 1;
  return `${prefix}${String(next).padStart(3, '0')}`;
}

function loginInfoGetCustomer(guest) {
  return APP_DATA.buyers.find(buyer => buyer.code === guest?.buyerCode) || null;
}

function init_login_info() {
  if (typeof isAdmin === 'function' && !isAdmin()) return;
  renderLoginInfoPage();
}

function setLoginInfoRole(role) {
  if (!['admin', 'buyer', 'guest'].includes(role)) return;
  loginInfoState.role = role;
  document.querySelectorAll('[data-login-role]').forEach(button => {
    const active = button.dataset.loginRole === role;
    button.classList.toggle('active', active);
    button.setAttribute('aria-selected', String(active));
  });
  renderLoginInfoTable();
}

function filterLoginInfo(value) {
  loginInfoState.query = String(value || '').trim().toLowerCase();
  renderLoginInfoTable();
}

function renderLoginInfoPage() {
  const adminCount = APP_DATA.users.filter(user => user.role === 'admin').length;
  const workerCount = APP_DATA.users.filter(user => user.role === 'buyer' || user.role === 'worker').length;
  const guestCount = APP_DATA.guestAccounts.length;
  const customerCount = new Set(APP_DATA.guestAccounts.map(guest => guest.buyerCode).filter(Boolean)).size;

  const counts = { admin: adminCount, buyer: workerCount, guest: guestCount };
  Object.entries(counts).forEach(([role, count]) => {
    const element = document.getElementById(`loginInfoCount-${role}`);
    if (element) element.textContent = count;
  });
  const customerCountElement = document.getElementById('loginInfoCustomerCount');
  if (customerCountElement) customerCountElement.textContent = customerCount;

  setLoginInfoRole(loginInfoState.role);
}

function loginInfoMatches(record) {
  if (!loginInfoState.query) return true;
  return Object.values(record).some(value => String(value ?? '').toLowerCase().includes(loginInfoState.query));
}

function renderLoginInfoTable() {
  const tbody = document.getElementById('loginInfoTableBody');
  const head = document.getElementById('loginInfoTableHead');
  const heading = document.getElementById('loginInfoListTitle');
  const addLabel = document.getElementById('loginInfoAddLabel');
  if (!tbody || !head) return;

  const role = loginInfoState.role;
  if (heading) heading.textContent = `${loginInfoRoleLabel(role)}のログイン情報`;
  if (addLabel) addLabel.textContent = `${loginInfoRoleLabel(role)}を新規作成`;

  if (role === 'guest') {
    head.innerHTML = '<tr><th>ゲスト／顧客</th><th>ログインID</th><th>連絡先</th><th>顧客情報</th><th>状態</th><th>操作</th></tr>';
    const guests = APP_DATA.guestAccounts
      .map(guest => ({ guest, customer: loginInfoGetCustomer(guest) }))
      .filter(({ guest, customer }) => loginInfoMatches({ ...guest, ...customer }));
    tbody.innerHTML = guests.length ? guests.map(({ guest, customer }) => `
      <tr>
        <td data-label="ゲスト／顧客"><strong>${loginInfoEscapeHtml(guest.name || guest.company)}</strong><span class="login-info-sub">${loginInfoEscapeHtml(guest.company || '会社名未登録')}</span></td>
        <td data-label="ログインID"><code class="login-info-code">${loginInfoEscapeHtml(guest.id)}</code><span class="login-info-password"><i class="fa-solid fa-lock" aria-hidden="true"></i> 設定済み</span></td>
        <td data-label="連絡先">${loginInfoEscapeHtml(guest.email || '—')}<span class="login-info-sub">${loginInfoEscapeHtml(customer?.contact || '連絡先未登録')}</span></td>
        <td data-label="顧客情報"><span class="login-info-customer-code">${loginInfoEscapeHtml(guest.buyerCode || '未連携')}</span><span class="login-info-sub">${loginInfoEscapeHtml(customer?.address || '住所未登録')}</span></td>
        <td data-label="状態"><span class="login-info-status ${guest.active === false ? 'inactive' : ''}"><i class="fa-solid fa-circle" aria-hidden="true"></i>${guest.active === false ? '停止中' : '有効'}</span></td>
        <td data-label="操作"><button type="button" class="btn btn-outline btn-sm" onclick="openLoginInfoModal('guest','${loginInfoEscapeHtml(guest.id)}')"><i class="fa-solid fa-pen"></i> 編集</button></td>
      </tr>`).join('') : loginInfoEmptyRow(6);
    return;
  }

  head.innerHTML = '<tr><th>氏名</th><th>ログインID</th><th>メールアドレス</th><th>パスワード</th><th>状態</th><th>操作</th></tr>';
  const users = APP_DATA.users
    .filter(user => role === 'admin' ? user.role === 'admin' : user.role === 'buyer' || user.role === 'worker')
    .filter(loginInfoMatches);
  tbody.innerHTML = users.length ? users.map(user => `
    <tr>
      <td data-label="氏名"><span class="login-info-person"><span class="login-info-avatar">${loginInfoEscapeHtml(user.avatar || user.name?.slice(0, 1) || '?')}</span><strong>${loginInfoEscapeHtml(user.name)}</strong></span></td>
      <td data-label="ログインID"><code class="login-info-code">${loginInfoEscapeHtml(user.loginId)}</code></td>
      <td data-label="メールアドレス">${loginInfoEscapeHtml(user.email || '—')}</td>
      <td data-label="パスワード"><span class="login-info-password"><i class="fa-solid fa-lock" aria-hidden="true"></i> 設定済み</span></td>
      <td data-label="状態"><span class="login-info-status ${user.active === false ? 'inactive' : ''}"><i class="fa-solid fa-circle" aria-hidden="true"></i>${user.active === false ? '停止中' : '有効'}</span></td>
      <td data-label="操作"><button type="button" class="btn btn-outline btn-sm" onclick="openLoginInfoModal('${role}','${loginInfoEscapeHtml(user.id)}')"><i class="fa-solid fa-pen"></i> 編集</button></td>
    </tr>`).join('') : loginInfoEmptyRow(6);
}

function loginInfoEmptyRow(colspan) {
  return `<tr class="login-info-empty"><td colspan="${colspan}"><i class="fa-solid fa-magnifying-glass"></i><strong>該当する登録がありません</strong><span>検索条件を変えるか、新規作成してください。</span></td></tr>`;
}

function openLoginInfoModal(role = loginInfoState.role, id = null) {
  if (!['admin', 'buyer', 'guest'].includes(role)) return;
  loginInfoState.editingType = role;
  loginInfoState.editingId = id || null;

  const isGuestRole = role === 'guest';
  const record = isGuestRole
    ? APP_DATA.guestAccounts.find(guest => guest.id === id)
    : APP_DATA.users.find(user => user.id === id);
  const customer = isGuestRole ? loginInfoGetCustomer(record) : null;
  const isEdit = Boolean(record);
  const suggestedLoginId = isGuestRole ? loginInfoNextId('G', APP_DATA.guestAccounts) : '';

  document.getElementById('loginInfoModalTitle').textContent = `${loginInfoRoleLabel(role)}を${isEdit ? '編集' : '新規作成'}`;
  const roleInput = document.getElementById('loginInfoRole');
  roleInput.value = role;
  roleInput.disabled = isEdit;
  document.getElementById('loginInfoName').value = record?.name || '';
  const isWorkerRole = role === 'buyer';
  const staffCode = record?.staffCode || (isWorkerRole && typeof getNextStaffCode === 'function' ? getNextStaffCode() : '');
  const staffCodeField = document.getElementById('loginInfoStaffCodeField');
  const staffCodeInput = document.getElementById('loginInfoStaffCode');
  if (staffCodeField) staffCodeField.classList.toggle('hidden', !isWorkerRole);
  if (staffCodeInput) staffCodeInput.value = staffCode;
  document.getElementById('loginInfoLoginId').value = isGuestRole ? (record?.id || suggestedLoginId) : (record?.loginId || '');
  document.getElementById('loginInfoLoginId').readOnly = isGuestRole && isEdit;
  document.getElementById('loginInfoEmail').value = record?.email || '';
  document.getElementById('loginInfoPassword').value = '';
  document.getElementById('loginInfoPassword').placeholder = isEdit ? '変更しない場合は空欄' : '8文字以上を推奨';
  document.getElementById('loginInfoActive').checked = record?.active !== false;
  document.getElementById('loginInfoCompany').value = record?.company || customer?.name || '';
  document.getElementById('loginInfoBuyerCode').value = record?.buyerCode || (isGuestRole ? loginInfoNextId('B', APP_DATA.buyers, 'code') : '');
  document.getElementById('loginInfoAddress').value = customer?.address || '';
  document.getElementById('loginInfoContact').value = customer?.contact || '';
  document.getElementById('loginInfoInvoice').value = customer?.invoice || '';
  document.getElementById('loginInfoCustomerFields').classList.toggle('hidden', !isGuestRole);
  document.getElementById('loginInfoPassword').type = 'password';
  document.getElementById('loginInfoPasswordToggleIcon').className = 'fa-regular fa-eye';
  document.getElementById('loginInfoModal').classList.remove('hidden');
  setTimeout(() => document.getElementById('loginInfoName')?.focus(), 0);
}

function onLoginInfoRoleChange(role) {
  if (loginInfoState.editingId) return;
  loginInfoState.editingType = role;
  const guest = role === 'guest';
  document.getElementById('loginInfoCustomerFields').classList.toggle('hidden', !guest);
  document.getElementById('loginInfoLoginId').value = guest ? loginInfoNextId('G', APP_DATA.guestAccounts) : '';
  document.getElementById('loginInfoBuyerCode').value = guest ? loginInfoNextId('B', APP_DATA.buyers, 'code') : '';
  const worker = role === 'buyer';
  document.getElementById('loginInfoStaffCodeField')?.classList.toggle('hidden', !worker);
  const staffCodeInput = document.getElementById('loginInfoStaffCode');
  if (staffCodeInput) staffCodeInput.value = worker && typeof getNextStaffCode === 'function' ? getNextStaffCode() : '';
}

function closeLoginInfoModal() {
  document.getElementById('loginInfoModal').classList.add('hidden');
  loginInfoState.editingType = null;
  loginInfoState.editingId = null;
}

function generateLoginInfoPassword() {
  const input = document.getElementById('loginInfoPassword');
  const chars = 'abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789';
  let value = '';
  for (let index = 0; index < 12; index += 1) value += chars[Math.floor(Math.random() * chars.length)];
  input.value = value;
  input.type = 'text';
  document.getElementById('loginInfoPasswordToggleIcon').className = 'fa-regular fa-eye-slash';
}

function toggleLoginInfoPassword() {
  const input = document.getElementById('loginInfoPassword');
  const show = input.type === 'password';
  input.type = show ? 'text' : 'password';
  document.getElementById('loginInfoPasswordToggleIcon').className = show ? 'fa-regular fa-eye-slash' : 'fa-regular fa-eye';
}

function saveLoginInfo() {
  const role = document.getElementById('loginInfoRole').value;
  const name = document.getElementById('loginInfoName').value.trim();
  const loginId = document.getElementById('loginInfoLoginId').value.trim();
  const email = document.getElementById('loginInfoEmail').value.trim();
  const password = document.getElementById('loginInfoPassword').value;
  const active = document.getElementById('loginInfoActive').checked;
  const isGuestRole = role === 'guest';
  const isEdit = Boolean(loginInfoState.editingId);

  if (!name || !loginId || !email) {
    showToast('error', '入力内容を確認してください', '氏名・ログインID・メールアドレスは必須です');
    return false;
  }
  if (!/^[A-Za-z0-9._@+-]+$/.test(loginId)) {
    showToast('error', 'ログインIDを確認してください', '半角英数字と . _ @ + - のみ使用できます');
    return false;
  }
  if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) {
    showToast('error', 'メールアドレスを確認してください', '有効なメールアドレスを入力してください');
    return false;
  }
  if (!isEdit && password.length < 8) {
    showToast('error', 'パスワードを確認してください', '新規作成時は8文字以上で入力してください');
    return false;
  }

  const duplicateUser = APP_DATA.users.some(user => user.loginId.toLowerCase() === loginId.toLowerCase() && user.id !== loginInfoState.editingId);
  const duplicateGuest = APP_DATA.guestAccounts.some(guest => guest.id.toLowerCase() === loginId.toLowerCase() && guest.id !== loginInfoState.editingId);
  if (duplicateUser || duplicateGuest) {
    showToast('error', 'ログインIDが重複しています', '別のログインIDを入力してください');
    return false;
  }

  if (isGuestRole) {
    const company = document.getElementById('loginInfoCompany').value.trim();
    const buyerCode = document.getElementById('loginInfoBuyerCode').value.trim().toUpperCase();
    if (!company || !buyerCode) {
      showToast('error', '顧客情報を確認してください', '会社名と顧客コードは必須です');
      return false;
    }
    const duplicateBuyerGuest = APP_DATA.guestAccounts.some(guest =>
      guest.buyerCode === buyerCode && guest.id !== loginInfoState.editingId
    );
    if (duplicateBuyerGuest) {
      showToast('error', 'この販売先にはログイン情報があります', '同じ顧客コードに複数のゲストログインは発行できません');
      return false;
    }
    let guest = APP_DATA.guestAccounts.find(item => item.id === loginInfoState.editingId);
    const previousBuyerCode = guest?.buyerCode || '';
    if (!guest) {
      guest = { id: loginId.toUpperCase(), password };
      APP_DATA.guestAccounts.push(guest);
    }
    Object.assign(guest, {
      id: loginId.toUpperCase(),
      name,
      company,
      buyerCode,
      email,
      active,
    });
    if (password) guest.password = password;

    ensureBuyerForGuest(guest, {
      code: buyerCode,
      name: company,
      address: document.getElementById('loginInfoAddress').value.trim(),
      contact: document.getElementById('loginInfoContact').value.trim(),
      invoice: document.getElementById('loginInfoInvoice').value.trim(),
      email,
    });
    if (previousBuyerCode && previousBuyerCode !== buyerCode && typeof setBuyerGuestManaged === 'function') {
      setBuyerGuestManaged(previousBuyerCode, false);
    }
  } else {
    let user = APP_DATA.users.find(item => item.id === loginInfoState.editingId);
    const previousWorkerName = user?.name || '';
    if (user?.role === 'admin' && !active) {
      if (typeof currentUserId === 'function' && currentUserId() === user.id) {
        showToast('error', '現在の管理者は停止できません', '別の管理者でログインしてから変更してください');
        return false;
      }
      const otherActiveAdmins = APP_DATA.users.filter(item => item.role === 'admin' && item.id !== user.id && item.active !== false);
      if (otherActiveAdmins.length === 0) {
        showToast('error', '最後の管理者は停止できません', '有効な管理者を1件以上残してください');
        return false;
      }
    }
    if (!user) {
      user = {
        id: loginInfoNextId('U', APP_DATA.users),
        role,
        password,
      };
      if (role === 'admin') {
        user.approvalCode = String(Math.floor(Math.random() * 1000000)).padStart(6, '0');
        user.approvalCodeUpdatedAt = new Date().toISOString().slice(0, 10);
      }
      APP_DATA.users.push(user);
    }
    Object.assign(user, {
      role,
      name,
      loginId,
      email,
      active,
      avatar: name.slice(0, 1),
    });
    if (password) user.password = password;
    if ((role === 'buyer' || role === 'worker') && typeof syncWorkerAccountToStaffMaster === 'function') {
      syncWorkerAccountToStaffMaster(user, previousWorkerName, document.getElementById('loginInfoStaffCode')?.value || '');
    }
    if (typeof currentUserId === 'function' && currentUserId() === user.id && typeof setSession === 'function') {
      setSession({ id: user.id, role: user.role, name: user.name, avatar: user.avatar, loginId: user.loginId });
      if (typeof applyRoleUI === 'function') applyRoleUI();
    }
  }

  persistLoginDirectory();
  closeLoginInfoModal();
  renderLoginInfoPage();
  if (isGuestRole && typeof refreshBuyerMasterConsumers === 'function') refreshBuyerMasterConsumers();
  else if (typeof refreshPasswordMasterDirectory === 'function') refreshPasswordMasterDirectory();
  showToast('success', isEdit ? '更新しました' : '作成しました', `${name} のログイン情報を保存しました`);
  return true;
}
