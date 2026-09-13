// =====================================================
// guest.js — ゲスト公開商品・カート・購入リクエスト
// =====================================================

const GUEST_SESSION_KEY = 'inv_guest_session';
const GUEST_CART_STORAGE_PREFIX = 'inv_guest_cart_v1:';

const GUEST_I18N = {
  ja: {
    cart: 'カート', logout: 'ログアウト', taxNotice: '表示価格は税抜き価格です。ご不明な点はお気軽にお問い合わせください。',
    items: '点', allBrands: 'すべてのブランド', allConditions: 'すべてのコンディション', allBoxes: 'すべての公開BOX',
    emptyTitle: '公開中の商品がありません', emptyBody: '検索条件を変更するか、管理者へお問い合わせください。',
    cartAdded: 'カート済', addCart: 'カートに追加', removeCart: 'カートから削除', salePrice: '販売価格（税抜）',
    totalTaxExcluded: '合計（税抜）', checklist: '購入前確認チェックリスト',
    confirmDetails: '商品の詳細情報（コンディション・付属品）を確認しました', confirmPrice: '表示価格（税抜）に同意しました',
    confirmPolicy: '返品・交換ポリシーを確認しました', confirmContact: '注文後、担当者よりご連絡があることを了承しました',
    requestNote: '管理者への備考（任意）', closeCart: 'カートを閉じる', checkAll: 'すべての項目にチェックを入れてください',
    submitRequest: '購入リクエストを送信する', emptyCart: 'カートに商品がありません', published: '公開BOX', updated: '最終更新',
    requestSent: id => `購入リクエスト ${id} を管理者へ送信しました。担当者からの連絡をお待ちください。`,
  },
  en: {
    cart: 'Cart', logout: 'Log out', taxNotice: 'Prices shown are before tax. Please contact us if you have any questions.',
    items: ' items', allBrands: 'All brands', allConditions: 'All conditions', allBoxes: 'All published boxes',
    emptyTitle: 'No products are currently available', emptyBody: 'Change the filters or contact the administrator.',
    cartAdded: 'In cart', addCart: 'Add to cart', removeCart: 'Remove from cart', salePrice: 'Sale price (excl. tax)',
    totalTaxExcluded: 'Total (excl. tax)', checklist: 'Pre-purchase checklist',
    confirmDetails: 'I reviewed the product condition and accessories.', confirmPrice: 'I agree to the displayed price before tax.',
    confirmPolicy: 'I reviewed the return and exchange policy.', confirmContact: 'I understand that a representative will contact me.',
    requestNote: 'Note to administrator (optional)', closeCart: 'Close cart', checkAll: 'Check every item to continue',
    submitRequest: 'Send purchase request', emptyCart: 'Your cart is empty', published: 'Published boxes', updated: 'Last updated',
    requestSent: id => `Purchase request ${id} was sent to the administrator. Please wait for a representative to contact you.`,
  },
};

let guestAccount = null;
let guestLanguage = 'ja';
let guestCurrency = 'JPY';
let guestCartCodes = [];
let guestCatalogItems = [];
let guestAccessibleBoxes = [];

function guestText(key) {
  return GUEST_I18N[guestLanguage]?.[key] ?? GUEST_I18N.ja[key] ?? key;
}

function guestEscape(value) {
  return String(value ?? '').replace(/[&<>'"]/g, char => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;',
  })[char]);
}

function guestQueryId() {
  return new URLSearchParams(location.search).get('id') || '';
}

function clearGuestSession() {
  sessionStorage.removeItem(GUEST_SESSION_KEY);
}

function guestCartStorageKey() {
  return `${GUEST_CART_STORAGE_PREFIX}${guestAccount?.id || 'anonymous'}`;
}

function loadGuestCart() {
  try {
    const stored = JSON.parse(localStorage.getItem(guestCartStorageKey()) || '[]');
    guestCartCodes = Array.isArray(stored) ? stored.map(String) : [];
  } catch {
    guestCartCodes = [];
  }
}

function saveGuestCart() {
  localStorage.setItem(guestCartStorageKey(), JSON.stringify(guestCartCodes));
}

function refreshGuestPublishedItems() {
  hydrateGuestDomainState();
  const snapshot = APP_DATA.publishedSnapshot || { boxes: [] };
  const buyerCode = guestAccount?.buyerCode;
  guestAccessibleBoxes = (snapshot.boxes || []).filter(box => (box.publicTo || []).includes(buyerCode));

  const seen = new Set();
  guestCatalogItems = [];
  guestAccessibleBoxes.forEach(box => {
    (box.items || []).forEach(item => {
      if (item.status !== '在庫中' || seen.has(item.code)) return;
      seen.add(item.code);
      guestCatalogItems.push({ ...item, publishedBoxNo: box.no, publishedBoxName: box.name || '' });
    });
  });
  const allowedCodes = new Set(guestCatalogItems.map(item => String(item.code)));
  guestCartCodes = guestCartCodes.filter(code => allowedCodes.has(code));
  saveGuestCart();
}

function getGuestConditionName(code) {
  const condition = APP_DATA.conditions.find(item => item.code === code);
  return condition?.name || code || '—';
}

function getGuestFxRate(code) {
  if (code === 'JPY') return 1;
  if (code === 'USD') return APP_DATA.fxRates.find(rate => rate.code === 'USD')?.rate || SALE_PRICE_JPY_PER_USD;
  return APP_DATA.fxRates.find(rate => rate.code === code)?.rate || 1;
}

function formatGuestPrice(usdValue) {
  const usd = Number(usdValue) || 0;
  const usdJpyRate = getGuestFxRate('USD');
  let amount = guestCurrency === 'USD' ? usd : (usd * usdJpyRate) / getGuestFxRate(guestCurrency);
  if (guestCurrency === 'JPY') amount = Math.round(amount / 1000) * 1000;
  if (guestCurrency === 'JPY') return `¥${Math.round(amount).toLocaleString('ja-JP')}`;
  return new Intl.NumberFormat(guestLanguage === 'ja' ? 'ja-JP' : 'en-US', {
    style: 'currency',
    currency: guestCurrency,
    maximumFractionDigits: 0,
  }).format(Math.round(amount));
}

function initializeGuestFilters() {
  const brands = [...new Set(guestCatalogItems.map(item => item.brand).filter(Boolean))].sort((a, b) => a.localeCompare(b, 'ja'));
  const brandSelect = document.getElementById('guest-brand-filter');
  const conditionSelect = document.getElementById('guest-condition-filter');
  const boxSelect = document.getElementById('guest-box-filter');
  const currentBrand = brandSelect.value;
  const currentCondition = conditionSelect.value;
  const currentBox = boxSelect.value;

  brandSelect.innerHTML = `<option value="">${guestEscape(guestText('allBrands'))}</option>` + brands.map(brand => `<option value="${guestEscape(brand)}">${guestEscape(brand)}</option>`).join('');
  conditionSelect.innerHTML = `<option value="">${guestEscape(guestText('allConditions'))}</option>` + APP_DATA.conditions.map(item => `<option value="${guestEscape(item.code)}">${guestEscape(item.name)}</option>`).join('');
  boxSelect.innerHTML = `<option value="">${guestEscape(guestText('allBoxes'))}</option>` + guestAccessibleBoxes.map(box => `<option value="${box.no}">BOX ${box.no}${box.name ? ` — ${guestEscape(box.name)}` : ''}</option>`).join('');
  if ([...brandSelect.options].some(option => option.value === currentBrand)) brandSelect.value = currentBrand;
  if ([...conditionSelect.options].some(option => option.value === currentCondition)) conditionSelect.value = currentCondition;
  if ([...boxSelect.options].some(option => option.value === currentBox)) boxSelect.value = currentBox;
}

function renderGuestCatalog() {
  const query = document.getElementById('guest-search').value.trim().toLowerCase();
  const brand = document.getElementById('guest-brand-filter').value;
  const condition = document.getElementById('guest-condition-filter').value;
  const boxNo = document.getElementById('guest-box-filter').value;
  const filtered = guestCatalogItems.filter(item => {
    const searchable = [item.brand, item.brandEn, item.model, item.modelEn, item.ref].join(' ').toLowerCase();
    return (!query || searchable.includes(query)) && (!brand || item.brand === brand) && (!condition || item.condition === condition) && (!boxNo || String(item.publishedBoxNo) === boxNo);
  });

  document.getElementById('guest-result-count').textContent = filtered.length;
  document.getElementById('guest-cart-count').textContent = guestCartCodes.length;
  document.getElementById('guest-empty').classList.toggle('hidden', filtered.length > 0);
  const grid = document.getElementById('guest-product-grid');
  grid.classList.toggle('hidden', filtered.length === 0);
  grid.innerHTML = filtered.map(item => renderGuestProductCard(item)).join('');
  grid.querySelectorAll('img[data-guest-product-image]').forEach(image => {
    image.addEventListener('error', () => {
      image.parentElement.innerHTML = '<span class="guest-product-placeholder"><i class="fa-regular fa-clock" aria-hidden="true"></i></span>';
    }, { once: true });
  });

  const snapshot = APP_DATA.publishedSnapshot || {};
  document.getElementById('guest-publish-summary').innerHTML = guestAccessibleBoxes.length
    ? `<i class="fa-solid fa-layer-group" aria-hidden="true"></i> ${guestEscape(guestText('published'))}: <strong>${guestAccessibleBoxes.length}</strong> / ${guestEscape(guestText('updated'))}: <strong>${guestEscape(snapshot.updatedAt || '—')}</strong>`
    : '';
}

function renderGuestProductCard(item) {
  const inCart = guestCartCodes.includes(String(item.code));
  const image = item.images?.[0]
    ? `<img data-guest-product-image src="${guestEscape(item.images[0])}" alt="${guestEscape(`${item.brand} ${item.model}`)}" loading="lazy">`
    : '<span class="guest-product-placeholder"><i class="fa-regular fa-clock" aria-hidden="true"></i></span>';
  const displayBrand = guestLanguage === 'en' && item.brandEn ? item.brandEn : item.brand;
  const displayModel = guestLanguage === 'en' && item.modelEn ? item.modelEn : item.model;
  return `
    <article class="guest-product-card${inCart ? ' is-in-cart' : ''}">
      <button class="guest-product-image-button" type="button" onclick="openGuestDetail('${guestEscape(item.code)}')" aria-label="${guestEscape(`${displayBrand} ${displayModel}`)}">
        ${inCart ? `<span class="guest-cart-chip"><i class="fa-solid fa-check" aria-hidden="true"></i> ${guestEscape(guestText('cartAdded'))}</span>` : ''}
        ${image}
      </button>
      <div class="guest-product-body">
        <div class="guest-product-brand">${guestEscape(displayBrand)}</div>
        <h2 class="guest-product-name">${guestEscape(displayModel)}</h2>
        <div class="guest-product-ref">Ref. ${guestEscape(item.ref || '—')}</div>
        <span class="guest-condition-badge">${guestEscape(getGuestConditionName(item.condition))}</span>
        <div class="guest-product-box"><i class="fa-solid fa-box" aria-hidden="true"></i> BOX ${item.publishedBoxNo}${item.publishedBoxName ? ` — ${guestEscape(item.publishedBoxName)}` : ''}</div>
        <div class="guest-product-price"><span>${guestEscape(guestText('salePrice'))}</span><strong>${guestEscape(formatGuestPrice(item.salePrice))}</strong></div>
        <button class="guest-cart-toggle${inCart ? ' is-remove' : ''}" type="button" onclick="toggleGuestCart('${guestEscape(item.code)}')">
          <i class="fa-solid ${inCart ? 'fa-trash-can' : 'fa-cart-plus'}" aria-hidden="true"></i> ${guestEscape(inCart ? guestText('removeCart') : guestText('addCart'))}
        </button>
      </div>
    </article>`;
}

function toggleGuestCart(code) {
  const normalized = String(code);
  guestCartCodes = guestCartCodes.includes(normalized)
    ? guestCartCodes.filter(itemCode => itemCode !== normalized)
    : [...guestCartCodes, normalized];
  saveGuestCart();
  renderGuestCatalog();
  if (!document.getElementById('guest-cart-overlay').classList.contains('hidden')) renderGuestCart();
}

function focusGuestSearch() {
  document.getElementById('guest-search').focus();
  document.getElementById('catalog').scrollIntoView({ behavior: 'smooth', block: 'start' });
}

function setGuestLanguage(language) {
  guestLanguage = language === 'en' ? 'en' : 'ja';
  document.documentElement.lang = guestLanguage;
  document.getElementById('guest-lang-ja').classList.toggle('is-active', guestLanguage === 'ja');
  document.getElementById('guest-lang-en').classList.toggle('is-active', guestLanguage === 'en');
  document.getElementById('guest-lang-ja').setAttribute('aria-pressed', String(guestLanguage === 'ja'));
  document.getElementById('guest-lang-en').setAttribute('aria-pressed', String(guestLanguage === 'en'));
  document.querySelectorAll('[data-i18n]').forEach(element => {
    const value = guestText(element.dataset.i18n);
    if (typeof value === 'string') element.textContent = value;
  });
  document.getElementById('guest-search').placeholder = guestLanguage === 'ja' ? 'ブランド / モデル / 型番で検索...' : 'Search by brand / model / reference...';
  document.getElementById('guest-request-note').placeholder = guestLanguage === 'ja' ? '希望納期や確認事項をご記入ください' : 'Enter requested delivery date or questions';
  initializeGuestFilters();
  renderGuestCatalog();
  if (!document.getElementById('guest-cart-overlay').classList.contains('hidden')) renderGuestCart();
}

function setGuestCurrency(currency) {
  guestCurrency = ['JPY', 'USD', 'EUR', 'HKD', 'CHF'].includes(currency) ? currency : 'JPY';
  renderGuestCatalog();
  if (!document.getElementById('guest-cart-overlay').classList.contains('hidden')) renderGuestCart();
}

function getGuestCartItems() {
  return guestCartCodes.map(code => guestCatalogItems.find(item => String(item.code) === code)).filter(Boolean);
}

function openGuestCart() {
  document.getElementById('guest-cart-overlay').classList.remove('hidden');
  document.body.style.overflow = 'hidden';
  renderGuestCart();
  document.querySelector('#guest-cart-overlay .guest-icon-button')?.focus();
}

function closeGuestCart() {
  document.getElementById('guest-cart-overlay').classList.add('hidden');
  document.body.style.overflow = '';
}

function renderGuestCart() {
  const items = getGuestCartItems();
  const list = document.getElementById('guest-cart-items');
  list.innerHTML = items.length === 0
    ? `<div class="guest-empty" style="padding:30px 12px;"><i class="fa-solid fa-cart-shopping" aria-hidden="true"></i><h2>${guestEscape(guestText('emptyCart'))}</h2></div>`
    : items.map(item => {
        const image = item.images?.[0] ? `<img src="${guestEscape(item.images[0])}" alt="">` : '<i class="fa-regular fa-clock" aria-hidden="true"></i>';
        return `<div class="guest-cart-line">
          <div class="guest-cart-thumb">${image}</div>
          <div><div class="guest-cart-line-name">${guestEscape(item.brand)} ${guestEscape(item.model)}</div><div class="guest-cart-line-meta">Ref. ${guestEscape(item.ref || '—')} / ${guestEscape(getGuestConditionName(item.condition))}</div></div>
          <div class="guest-cart-line-price">${guestEscape(formatGuestPrice(item.salePrice))}</div>
          <button class="guest-cart-remove" type="button" onclick="toggleGuestCart('${guestEscape(item.code)}')" title="${guestEscape(guestText('removeCart'))}" aria-label="${guestEscape(guestText('removeCart'))}"><i class="fa-solid fa-xmark" aria-hidden="true"></i></button>
        </div>`;
      }).join('');
  document.getElementById('guest-cart-total').textContent = formatGuestPrice(items.reduce((sum, item) => sum + (Number(item.salePrice) || 0), 0));
  document.querySelectorAll('.guest-confirm-check').forEach(check => { check.checked = false; check.disabled = items.length === 0; });
  updateGuestSubmitState();
}

function updateGuestSubmitState() {
  const checks = [...document.querySelectorAll('.guest-confirm-check')];
  const ready = getGuestCartItems().length > 0 && checks.length > 0 && checks.every(check => check.checked);
  const button = document.getElementById('guest-submit-request');
  button.disabled = !ready;
  button.querySelector('span').textContent = guestText(ready ? 'submitRequest' : 'checkAll');
}

async function submitGuestPurchaseRequest() {
  const button = document.getElementById('guest-submit-request');
  if (button.disabled) return;
  button.disabled = true;
  const items = getGuestCartItems();
  const note = document.getElementById('guest-request-note').value;
  let request;
  try {
    if (window.ZaikoAPI?.state?.hydrated) {
      const created = await window.ZaikoAPI.createGuestRequests(items, note);
      request = { id: created.map(item => item.requestNumber).join(', ') };
    } else {
      request = createGuestPurchaseRequest({ guest: guestAccount, items, note });
    }
  } catch (error) {
    button.disabled = false;
    alert(error.message || '購入リクエストを送信できませんでした。');
    return;
  }
  if (!request) return;
  guestCartCodes = [];
  saveGuestCart();
  document.getElementById('guest-request-note').value = '';
  closeGuestCart();
  renderGuestCatalog();
  const success = document.getElementById('guest-success');
  success.innerHTML = `<i class="fa-solid fa-circle-check" aria-hidden="true"></i><span>${guestEscape(guestText('requestSent')(request.id))}</span>`;
  success.classList.remove('hidden');
  success.scrollIntoView({ behavior: 'smooth', block: 'center' });
}

function openGuestDetail(code) {
  const item = guestCatalogItems.find(product => String(product.code) === String(code));
  if (!item) return;
  const image = item.images?.[0] ? `<img src="${guestEscape(item.images[0])}" alt="${guestEscape(`${item.brand} ${item.model}`)}">` : '<i class="fa-regular fa-clock" aria-hidden="true"></i>';
  document.getElementById('guest-detail-title').textContent = `${item.brand} ${item.model}`;
  document.getElementById('guest-detail-body').innerHTML = `<div class="guest-detail-layout">
    <div class="guest-detail-image">${image}</div>
    <div><dl class="guest-detail-list">
      <dt>Ref.</dt><dd>${guestEscape(item.ref || '—')}</dd>
      <dt>コンディション</dt><dd>${guestEscape(getGuestConditionName(item.condition))}</dd>
      <dt>付属品</dt><dd>${guestEscape((item.accessories || []).join('・') || '—')}</dd>
      <dt>公開BOX</dt><dd>BOX ${item.publishedBoxNo}${item.publishedBoxName ? ` — ${guestEscape(item.publishedBoxName)}` : ''}</dd>
      <dt>${guestEscape(guestText('salePrice'))}</dt><dd>${guestEscape(formatGuestPrice(item.salePrice))}</dd>
    </dl></div>
  </div>`;
  document.getElementById('guest-detail-overlay').classList.remove('hidden');
  document.body.style.overflow = 'hidden';
}

function closeGuestDetail() {
  document.getElementById('guest-detail-overlay').classList.add('hidden');
  document.body.style.overflow = '';
}

document.addEventListener('guest-domain-updated', event => {
  if (event.detail?.key !== GUEST_SNAPSHOT_STORAGE_KEY) return;
  refreshGuestPublishedItems();
  initializeGuestFilters();
  renderGuestCatalog();
});

document.addEventListener('DOMContentLoaded', async () => {
  if (window.ZaikoAPI) {
    try {
      await window.ZaikoAPI.hydrateGuest();
    } catch (error) {
      console.error('Guest REST API hydration failed', error);
      clearGuestSession();
      location.href = 'index.html';
      return;
    }
  }
  guestAccount = APP_DATA.guestAccounts.find(account => account.id === guestQueryId()) || APP_DATA.guestAccounts[0] || null;
  if (!guestAccount) {
    location.href = 'index.html';
    return;
  }
  sessionStorage.setItem(GUEST_SESSION_KEY, JSON.stringify({ id: guestAccount.id, name: guestAccount.name }));
  loadGuestCart();
  refreshGuestPublishedItems();
  initializeGuestFilters();
  setGuestLanguage('ja');
  document.getElementById('guest-currency').value = 'JPY';
  renderGuestCatalog();

  document.querySelectorAll('.guest-modal-overlay').forEach(overlay => {
    overlay.addEventListener('click', event => {
      if (event.target !== overlay) return;
      if (overlay.id === 'guest-cart-overlay') closeGuestCart();
      if (overlay.id === 'guest-detail-overlay') closeGuestDetail();
    });
  });
  document.addEventListener('keydown', event => {
    if (event.key !== 'Escape') return;
    closeGuestCart();
    closeGuestDetail();
  });
});
