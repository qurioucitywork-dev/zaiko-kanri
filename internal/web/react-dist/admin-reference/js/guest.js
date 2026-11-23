// =====================================================
// guest.js — ゲスト公開商品・カート・購入リクエスト
// =====================================================

const GUEST_SESSION_KEY = 'inv_guest_session';
const GUEST_CART_STORAGE_PREFIX = 'inv_guest_cart_v1:';

const GUEST_I18N = {
  ja: {
    condition: 'コンディション', accessories: '付属品',
    requestHistory: '購入リクエスト履歴', closeHistory: '閉じる', refreshHistory: '更新', historyLoading: '履歴を読み込んでいます…',
    historyEmpty: '送信済みの購入リクエストはありません', historyError: '履歴を取得できませんでした。「更新」から再試行してください。',
    requestedAt: '送信日時', historyPrice: '送信時の価格（税抜）', priceNotRecorded: '送信時の価格記録なし', historyNote: '送信した備考', historyUnknown: '確認中',
    historyPending: '未対応', historyApproved: '承認済み', historyRejected: '却下', historyCancelled: '取消済み', historyExpired: '期限切れ', historyConverted: '販売済み',
    cart: 'カート', logout: 'ログアウト', taxNotice: '表示価格は税抜き価格です。ご不明な点はお気軽にお問い合わせください。',
    items: '点', allBrands: 'すべてのブランド', allConditions: 'すべてのコンディション',
    emptyTitle: '公開中の商品がありません', emptyBody: '検索条件を変更するか、管理者へお問い合わせください。',
    cartAdded: 'カート済', addCart: 'カートに追加', removeCart: 'カートから削除', salePrice: '想定売価（税抜）',
    totalTaxExcluded: '合計（税抜）', checklist: '購入前確認チェックリスト',
    confirmDetails: '商品の詳細情報（コンディション・付属品）を確認しました', confirmPrice: '表示価格（税抜）に同意しました',
    confirmPolicy: '返品・交換ポリシーを確認しました', confirmContact: '注文後、担当者よりご連絡があることを了承しました',
    requestNote: '管理者への備考（任意）', closeCart: 'カートを閉じる', checkAll: 'すべての項目にチェックを入れてください',
    submitRequest: '購入リクエストを送信する', emptyCart: 'カートに商品がありません',
    requestSent: id => `購入リクエスト ${id} を管理者へ送信しました。担当者からの連絡をお待ちください。`,
  },
  en: {
    condition: 'Condition', accessories: 'Accessories',
    requestHistory: 'Request history', closeHistory: 'Close', refreshHistory: 'Refresh', historyLoading: 'Loading request history…',
    historyEmpty: 'No purchase requests sent yet', historyError: 'Could not load your history. Select Refresh to try again.',
    requestedAt: 'Sent at', historyPrice: 'Price when sent (excl. tax)', priceNotRecorded: 'Original price not recorded', historyNote: 'Your note', historyUnknown: 'Under review',
    historyPending: 'Pending', historyApproved: 'Approved', historyRejected: 'Rejected', historyCancelled: 'Cancelled', historyExpired: 'Expired', historyConverted: 'Sold',
    cart: 'Cart', logout: 'Log out', taxNotice: 'Prices shown are before tax. Please contact us if you have any questions.',
    items: ' items', allBrands: 'All brands', allConditions: 'All conditions',
    emptyTitle: 'No products are currently available', emptyBody: 'Change the filters or contact the administrator.',
    cartAdded: 'In cart', addCart: 'Add to cart', removeCart: 'Remove from cart', salePrice: 'Expected sale price (excl. tax)',
    totalTaxExcluded: 'Total (excl. tax)', checklist: 'Pre-purchase checklist',
    confirmDetails: 'I reviewed the product condition and accessories.', confirmPrice: 'I agree to the displayed price before tax.',
    confirmPolicy: 'I reviewed the return and exchange policy.', confirmContact: 'I understand that a representative will contact me.',
    requestNote: 'Note to administrator (optional)', closeCart: 'Close cart', checkAll: 'Check every item to continue',
    submitRequest: 'Send purchase request', emptyCart: 'Your cart is empty',
    requestSent: id => `Purchase request ${id} was sent to the administrator. Please wait for a representative to contact you.`,
  },
};

let guestAccount = null;
let guestLanguage = 'ja';
let guestCurrency = 'JPY';
let guestCartCodes = [];
let guestCatalogItems = [];
let guestAccessibleBoxes = [];
let guestRequestHistory = [];
let guestHistoryLoadVersion = 0;

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
      if (!isGuestVisibleProductStatus(item.status) || seen.has(item.code)) return;
      seen.add(item.code);
      guestCatalogItems.push({ ...item });
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

function guestDisplayPrice(item) {
  if (window.ZaikoAPI) return item.displayPrice == null ? null : Number(item.displayPrice);
  const amount = Number(item.salePrice) || 0;
  const from = item.saleCurrency || 'USD';
  if (from === guestCurrency) return guestCurrency === 'JPY' ? Math.ceil(amount / 1000) * 1000 : amount;
  const rate = getGuestFxRate('USD');
  return guestCurrency === 'JPY' ? Math.ceil(amount * rate / 1000) * 1000 : Math.round(amount / rate);
}

function formatGuestDisplayPrice(amount, currency = guestCurrency) {
  if (amount == null || !Number.isFinite(amount)) return '—';
  if (currency === 'JPY') return `¥${Math.round(amount).toLocaleString('ja-JP')}`;
  return new Intl.NumberFormat(guestLanguage === 'ja' ? 'ja-JP' : 'en-US', {
    style: 'currency', currency, maximumFractionDigits: 0,
  }).format(amount);
}

function formatGuestItemPrice(item) {
  return formatGuestDisplayPrice(guestDisplayPrice(item), item.displayCurrency || guestCurrency);
}

function initializeGuestFilters() {
  const brands = [...new Set(guestCatalogItems.map(item => item.brand).filter(Boolean))].sort((a, b) => a.localeCompare(b, 'ja'));
  const brandSelect = document.getElementById('guest-brand-filter');
  const conditionSelect = document.getElementById('guest-condition-filter');
  const currentBrand = brandSelect.value;
  const currentCondition = conditionSelect.value;

  brandSelect.innerHTML = `<option value="">${guestEscape(guestText('allBrands'))}</option>` + brands.map(brand => `<option value="${guestEscape(brand)}">${guestEscape(brand)}</option>`).join('');
  conditionSelect.innerHTML = `<option value="">${guestEscape(guestText('allConditions'))}</option>` + APP_DATA.conditions.map(item => `<option value="${guestEscape(item.code)}">${guestEscape(item.name)}</option>`).join('');
  if ([...brandSelect.options].some(option => option.value === currentBrand)) brandSelect.value = currentBrand;
  if ([...conditionSelect.options].some(option => option.value === currentCondition)) conditionSelect.value = currentCondition;
}

function renderGuestCatalog() {
  const query = document.getElementById('guest-search').value.trim().toLowerCase();
  const brand = document.getElementById('guest-brand-filter').value;
  const condition = document.getElementById('guest-condition-filter').value;
  const filtered = guestCatalogItems.filter(item => {
    const searchable = [item.brand, item.brandEn, item.model, item.modelEn, item.ref].join(' ').toLowerCase();
    return (!query || searchable.includes(query)) && (!brand || item.brand === brand) && (!condition || item.condition === condition);
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
        <div class="guest-product-price"><span>${guestEscape(guestText('salePrice'))}</span><strong>${guestEscape(formatGuestItemPrice(item))}</strong></div>
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
  showGuestCatalog();
  document.getElementById('guest-search').focus();
  document.getElementById('catalog').scrollIntoView({ behavior: 'smooth', block: 'start' });
}

function setGuestLanguage(language) {
  guestLanguage = language === 'en' ? 'en' : 'ja';
  document.documentElement.lang = guestLanguage;
  document.querySelectorAll('[data-i18n]').forEach(element => {
    const value = guestText(element.dataset.i18n);
    if (typeof value === 'string') element.textContent = value;
  });
  document.getElementById('guest-search').placeholder = guestLanguage === 'ja' ? 'ブランド / モデル / 型番で検索...' : 'Search by brand / model / reference...';
  if (!document.getElementById('guest-history-overlay').classList.contains('hidden')) renderGuestRequestHistory();
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
          <div class="guest-cart-line-price">${guestEscape(formatGuestItemPrice(item))}</div>
          <button class="guest-cart-remove" type="button" onclick="toggleGuestCart('${guestEscape(item.code)}')" title="${guestEscape(guestText('removeCart'))}" aria-label="${guestEscape(guestText('removeCart'))}"><i class="fa-solid fa-xmark" aria-hidden="true"></i></button>
        </div>`;
      }).join('');
  document.getElementById('guest-cart-total').textContent = formatGuestDisplayPrice(items.some(item => guestDisplayPrice(item) == null) ? null : items.reduce((sum, item) => sum + guestDisplayPrice(item), 0), items[0]?.displayCurrency || guestCurrency);
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
  if(window.ZaikoAPI) await refreshGuestCatalogFromServer();
  renderGuestCatalog();
  const success = document.getElementById('guest-success');
  success.innerHTML = `<i class="fa-solid fa-circle-check" aria-hidden="true"></i><span>${guestEscape(guestText('requestSent')(request.id))}</span>`;
  success.classList.remove('hidden');
  success.scrollIntoView({ behavior: 'smooth', block: 'center' });
}

async function openGuestRequestHistory() {
  const version = ++guestHistoryLoadVersion;
  closeGuestCart();
  closeGuestDetail();
  document.getElementById('guest-history-overlay').classList.remove('hidden');
  document.body.style.overflow = 'hidden';
  const body = document.getElementById('guest-history-body');
  guestRequestHistory = [];
  body.innerHTML = `<p class="guest-history-message">${guestEscape(guestText('historyLoading'))}</p>`;
  document.querySelector('#guest-history-overlay .guest-icon-button')?.focus();
  try {
    if (window.ZaikoAPI) {
      // Identity is taken from the authenticated server session, never the URL.
      const response = await window.ZaikoAPI.request('/guest/purchase-requests');
      if (version !== guestHistoryLoadVersion) return;
      guestRequestHistory = (response.items || []).map(record => ({
        id: record.requestNumber, date: record.requestedAt, status: record.status, note: record.message,
        items: (record.items?.length ? record.items : [record]).map(line=>({ name:[line.brand,line.modelNumber].filter(Boolean).join(' '),ref:line.referenceNumber,
          priceMinor:line.requestedPriceMinor,currency:line.requestedCurrency,status:line.status })),
      }));
    } else {
      hydrateGuestDomainState();
      guestRequestHistory = (APP_DATA.purchaseRequests || []).filter(record => record.guestId === guestAccount?.id).map(record => ({
        id: record.id, date: record.createdAt || record.date, status: record.status, note: record.note,
        items: (record.items || []).map(item => ({ name: item.itemName, ref: item.ref,
          priceMinor: Number(item.salePrice), currency: 'USD' })),
      }));
    }
    renderGuestRequestHistory();
  } catch {
    if (version !== guestHistoryLoadVersion) return;
    body.innerHTML = `<p class="guest-history-message" role="alert">${guestEscape(guestText('historyError'))}</p>`;
  }
}

function closeGuestRequestHistory() {
  ++guestHistoryLoadVersion;
  document.getElementById('guest-history-overlay').classList.add('hidden');
  document.body.style.overflow = '';
  document.getElementById('guest-history-button')?.focus();
}

function renderGuestRequestHistory() {
  const statusKeys = { pending: 'historyPending', approved: 'historyApproved', rejected: 'historyRejected',
    cancelled: 'historyCancelled', expired: 'historyExpired', sold: 'historyConverted',
    '未対応': 'historyPending', '承認済み': 'historyApproved', '却下': 'historyRejected', '取消': 'historyCancelled', '期限切れ': 'historyExpired', '販売済み': 'historyConverted' };
  document.getElementById('guest-history-body').innerHTML = guestRequestHistory.length
    ? guestRequestHistory.map(record => {
      const date = new Date(record.date);
      const sentAt = Number.isNaN(date.getTime()) ? (record.date || '—') : date.toLocaleString(guestLanguage === 'ja' ? 'ja-JP' : 'en-US');
      return `<article class="guest-history-card">
        <div class="guest-history-heading"><h3>${guestEscape(record.id)}</h3><span class="guest-history-status">${guestEscape(guestText(statusKeys[record.status] || 'historyUnknown'))}</span></div>
        <div class="guest-history-date">${guestEscape(guestText('requestedAt'))}: ${guestEscape(sentAt)}</div>
        ${record.items.map(item => {
          let price = guestText('priceNotRecorded');
          if (item.priceMinor != null && /^[A-Z]{3}$/.test(item.currency || '')) {
            price = new Intl.NumberFormat(guestLanguage === 'ja' ? 'ja-JP' : 'en-US', { style: 'currency', currency: item.currency }).format(Number(item.priceMinor));
          }
          return `<div class="guest-history-product"><div><strong>${guestEscape(item.name || '—')}</strong><small>Ref. ${guestEscape(item.ref || '—')}</small>${item.status ? `<small>${guestEscape(guestText(statusKeys[item.status] || 'historyUnknown'))}</small>` : ''}</div><div class="guest-history-price"><small>${guestEscape(guestText('historyPrice'))}</small><strong>${guestEscape(price)}</strong></div></div>`;
        }).join('')}
        ${record.note ? `<div class="guest-history-note"><strong>${guestEscape(guestText('historyNote'))}</strong><br>${guestEscape(record.note)}</div>` : ''}
      </article>`;
    }).join('')
    : `<p class="guest-history-message">${guestEscape(guestText('historyEmpty'))}</p>`;
}

function resetGuestDetailImageZoom(frame = document.querySelector('#guest-detail-body .guest-detail-image')) {
  if (!frame) return;
  frame.classList.remove('is-zoomed');
  frame.style.removeProperty('--guest-zoom-x');
  frame.style.removeProperty('--guest-zoom-y');
}

function initializeGuestDetailImageZoom() {
  const frame = document.querySelector('#guest-detail-body .guest-detail-image');
  const image = frame?.querySelector('img');
  if (!image) return;
  image.draggable = false;
  const updateReady = () => {
    resetGuestDetailImageZoom(frame);
    frame.classList.toggle('is-zoomable', image.complete && image.naturalWidth > 0);
  };
  const zoomAtPointer = event => {
    // Touch gestures keep their usual scrolling behaviour.
    if (event.pointerType !== 'mouse' || !frame.classList.contains('is-zoomable')) return;
    const bounds = frame.getBoundingClientRect();
    if (!bounds.width || !bounds.height) return;
    const percent = (position, start, size) => Math.max(0, Math.min(100, (position - start) / size * 100));
    frame.style.setProperty('--guest-zoom-x', `${percent(event.clientX, bounds.left, bounds.width)}%`);
    frame.style.setProperty('--guest-zoom-y', `${percent(event.clientY, bounds.top, bounds.height)}%`);
    frame.classList.add('is-zoomed');
  };
  frame.addEventListener('pointerenter', zoomAtPointer);
  frame.addEventListener('pointermove', zoomAtPointer);
  frame.addEventListener('pointerleave', () => resetGuestDetailImageZoom(frame));
  frame.addEventListener('pointercancel', () => resetGuestDetailImageZoom(frame));
  image.addEventListener('load', updateReady);
  image.addEventListener('error', () => {
    resetGuestDetailImageZoom(frame);
    frame.classList.remove('is-zoomable');
  });
  updateReady();
}

function openGuestDetail(code) {
  const item = guestCatalogItems.find(product => String(product.code) === String(code));
  if (!item) return;
  const image = item.images?.[0] ? `<img src="${guestEscape(item.images[0])}" alt="${guestEscape(`${item.brand} ${item.model}`)}">` : '<i class="fa-regular fa-clock" aria-hidden="true"></i>';
  document.getElementById('guest-detail-title').textContent = `${item.brand} ${item.model}`;
  document.getElementById('guest-detail-body').innerHTML = `<div class="guest-detail-layout">
    <div><div class="guest-detail-image">${image}</div>
      ${(item.images || []).length > 1 ? `<div class="guest-detail-thumbnails">${item.images.map((src,index)=>`<button type="button" data-image-index="${index}" aria-label="${guestLanguage==='en'?'Image':'商品画像'} ${index+1}" aria-pressed="${index===0}"><img src="${guestEscape(src)}" alt="" loading="lazy"></button>`).join('')}</div>` : ''}</div>
    <div><dl class="guest-detail-list">
      <dt>Ref.</dt><dd>${guestEscape(item.ref || '—')}</dd>
      <dt data-i18n="condition">${guestEscape(guestText('condition'))}</dt><dd>${guestEscape(getGuestConditionName(item.condition))}</dd>
      <dt data-i18n="accessories">${guestEscape(guestText('accessories'))}</dt><dd>${guestEscape((item.accessories || []).join('・') || '—')}</dd>
      <dt data-i18n="salePrice">${guestEscape(guestText('salePrice'))}</dt><dd>${guestEscape(formatGuestItemPrice(item))}</dd>
    </dl></div>
  </div>`;
  initializeGuestDetailImageZoom();
  document.querySelectorAll('#guest-detail-body [data-image-index]').forEach(button=>button.addEventListener('click',()=>{
    const frame=document.querySelector('#guest-detail-body .guest-detail-image');
    resetGuestDetailImageZoom(frame);
    frame?.classList.remove('is-zoomable');
    const main=document.querySelector('#guest-detail-body .guest-detail-image img');
    if(main)main.src=item.images[Number(button.dataset.imageIndex)];
    document.querySelectorAll('#guest-detail-body [data-image-index]').forEach(candidate=>candidate.setAttribute('aria-pressed',String(candidate===button)));
  }));
  document.getElementById('guest-detail-overlay').dataset.productCode=String(code);
  document.getElementById('guest-detail-overlay').classList.remove('hidden');
  document.body.style.overflow = 'hidden';
}

function closeGuestDetail() {
  resetGuestDetailImageZoom();
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
  const overseas = guestAccount.regionType === 'overseas';
  guestCurrency = overseas ? 'USD' : 'JPY';
  setGuestLanguage(overseas ? 'en' : 'ja');
  applyGuestPageRoute();
  renderGuestCatalog();

  document.querySelectorAll('.guest-modal-overlay').forEach(overlay => {
    overlay.addEventListener('click', event => {
      if (event.target !== overlay) return;
      if (overlay.id === 'guest-cart-overlay') closeGuestCart();
      if (overlay.id === 'guest-detail-overlay') closeGuestDetail();
      if (overlay.id === 'guest-history-overlay') closeGuestRequestHistory();
    });
  });
  document.addEventListener('keydown', event => {
    if (event.key !== 'Escape') return;
    closeGuestCart();
    closeGuestDetail();
    if (!document.getElementById('guest-history-overlay').classList.contains('hidden')) closeGuestRequestHistory();
  });
});

let guestCatalogRefreshing=false;
async function refreshGuestCatalogFromServer() {
 if (!window.ZaikoAPI || !guestAccount || guestCatalogRefreshing || document.hidden) return;
 guestCatalogRefreshing=true;
 try {
  const before=JSON.stringify((APP_DATA.publishedSnapshot || {}).boxes || []);
  await window.ZaikoAPI.hydrateGuest();
  const after=JSON.stringify((APP_DATA.publishedSnapshot || {}).boxes || []);
  if(before===after)return;
  refreshGuestPublishedItems();initializeGuestFilters();renderGuestCatalog();
  const detail=document.getElementById('guest-detail-overlay');
  if(detail && !detail.classList.contains('hidden') && !guestCatalogItems.some(item=>String(item.code)===detail.dataset.productCode))closeGuestDetail();
  if(!document.getElementById('guest-cart-overlay').classList.contains('hidden'))renderGuestCart();
 }catch(error){
  if(error.status===401){clearGuestSession();location.href='index.html';}
 }finally{guestCatalogRefreshing=false;}
}
document.addEventListener('DOMContentLoaded',()=>{
 if(window.ZaikoAPI)window.setInterval(refreshGuestCatalogFromServer,30000);
});
window.addEventListener('focus',refreshGuestCatalogFromServer);
document.addEventListener('visibilitychange',()=>{if(!document.hidden)refreshGuestCatalogFromServer();});

Object.assign(GUEST_I18N.ja, {
 inquiry:'お問い合わせ',backToProducts:'商品一覧に戻る',inquiryIntro:'ご質問やご相談をお送りください。管理者が内容を確認し、登録済みの連絡先へご連絡します。',
 inquirySender:'送信者',inquirySubject:'件名（必須）',inquiryProduct:'管理番号（任意）',inquiryMessage:'お問い合わせ内容（必須）',inquirySend:'お問い合わせを送信',
 inquirySent:'お問い合わせを送信しました。担当者からの連絡をお待ちください。',inquiryFailed:'送信できませんでした。入力内容は残っています。通信状態を確認し、もう一度お試しください。',inquiryRequired:'件名とお問い合わせ内容を入力してください。',inquirySending:'送信中…',inventorySearch:'在庫検索',
});
Object.assign(GUEST_I18N.en, {
 inquiry:'Contact us',backToProducts:'Back to products',inquiryIntro:'Send us your questions or requests. Our team will review your message and contact you using your registered contact details.',
 inquirySender:'From',inquirySubject:'Subject (required)',inquiryProduct:'Product number (optional)',inquiryMessage:'Message (required)',inquirySend:'Send inquiry',
 inquirySent:'Your inquiry has been sent. Please wait for our team to contact you.',inquiryFailed:'Your inquiry could not be sent. Your input has been kept. Check your connection and try again.',inquiryRequired:'Enter a subject and message.',inquirySending:'Sending…',inventorySearch:'Search products',
});
function applyGuestPageRoute(focus=false) {
 const inquiry=location.hash==='#inquiry';
 document.getElementById('catalog').classList.toggle('hidden',inquiry);
 document.getElementById('guest-inquiry-page').classList.toggle('hidden',!inquiry);
 document.getElementById('guest-inquiry-button').setAttribute('aria-current',inquiry?'page':'false');
 document.getElementById('guest-inquiry-sender').textContent=guestAccount?.name||'';
 if(focus&&inquiry)document.getElementById('guest-inquiry-title').focus();
}
function openGuestInquiry() {
 closeGuestCart();closeGuestDetail();closeGuestRequestHistory();
 location.hash='inquiry';applyGuestPageRoute(true);window.scrollTo(0,0);
}
function showGuestCatalog() {
 location.hash='catalog';applyGuestPageRoute();window.scrollTo(0,0);
}
window.addEventListener('hashchange',()=>applyGuestPageRoute());
let guestInquirySending=false,guestInquiryAttempt=null;
async function submitGuestInquiry(event) {
 event.preventDefault();
 const form=document.getElementById('guest-inquiry-form');
 if(guestInquirySending||!guestAccount||!form.reportValidity())return;
 const input={subject:document.getElementById('guest-inquiry-subject').value.trim(),productCode:document.getElementById('guest-inquiry-product').value.trim(),message:document.getElementById('guest-inquiry-message').value.trim()};
 const error=document.getElementById('guest-inquiry-error'),success=document.getElementById('guest-inquiry-success');error.textContent='';success.classList.add('hidden');
 if(!input.subject||!input.message){error.textContent=guestText('inquiryRequired');return;}
 const signature=JSON.stringify(input);
 if(!guestInquiryAttempt||guestInquiryAttempt.signature!==signature)guestInquiryAttempt={signature,id:crypto.randomUUID()};
 const button=document.getElementById('guest-inquiry-submit');guestInquirySending=true;button.disabled=true;button.querySelector('span').textContent=guestText('inquirySending');
 try {
  if(!window.ZaikoAPI)throw new Error('API unavailable');
  await window.ZaikoAPI.request('/guest/inquiries',{method:'POST',body:JSON.stringify({...input,clientRequestId:guestInquiryAttempt.id})});
  form.reset();guestInquiryAttempt=null;success.textContent=guestText('inquirySent');success.classList.remove('hidden');success.scrollIntoView({block:'nearest'});
 } catch {error.textContent=guestText('inquiryFailed');}
 finally {guestInquirySending=false;button.disabled=false;button.querySelector('span').textContent=guestText('inquirySend');}
}
