// =====================================================
// guest_shared.js — ゲスト公開・購入リクエストの共有状態
// =====================================================

const GUEST_SNAPSHOT_STORAGE_KEY = 'inv_guest_snapshot_v1';
const GUEST_BOX_DRAFT_STORAGE_KEY = 'inv_guest_box_draft_v1';
const PURCHASE_REQUESTS_STORAGE_KEY = 'inv_purchase_requests_v1';
const PURCHASE_REQUEST_AVAILABLE_STATUS = '在庫中';
const PURCHASE_REQUEST_RESERVED_STATUS = '取置中';
const PURCHASE_REQUEST_CLOSED_STATUSES = new Set(['却下', '取消', '取消済']);

function _guestReadStorage(key) {
  try {
    const raw = localStorage.getItem(key);
    return raw ? JSON.parse(raw) : null;
  } catch {
    return null;
  }
}

function _guestWriteStorage(key, value) {
  try {
    localStorage.setItem(key, JSON.stringify(value));
    return true;
  } catch {
    return false;
  }
}

function hydrateGuestDomainState() {
  const storedSnapshot = _guestReadStorage(GUEST_SNAPSHOT_STORAGE_KEY);
  if (storedSnapshot && Array.isArray(storedSnapshot.boxes)) {
    APP_DATA.publishedSnapshot = storedSnapshot;
  }

  const storedRequests = _guestReadStorage(PURCHASE_REQUESTS_STORAGE_KEY);
  if (Array.isArray(storedRequests)) {
    APP_DATA.purchaseRequests = storedRequests;
  }

  const storedBoxDraft = _guestReadStorage(GUEST_BOX_DRAFT_STORAGE_KEY);
  if (storedBoxDraft && Array.isArray(storedBoxDraft.boxes)) {
    APP_DATA.boxes = storedBoxDraft.boxes;
    const assignments = storedBoxDraft.assignments || {};
    APP_DATA.inventory.forEach(item => {
      if (Object.prototype.hasOwnProperty.call(assignments, item.code)) {
        item.boxNo = assignments[item.code];
      }
    });
  }

  // 購入リクエストは永続化されるため、再読込時にも承認済み商品の取置を復元する。
  syncPurchaseRequestReservations();
  const migratedRequestCount = syncPurchaseRequestPartyCodes();
  if (migratedRequestCount > 0) persistPurchaseRequests();
  syncPurchaseRequestReservations();
}

function _purchaseRequestCode(value) {
  return String(value || '').trim().toUpperCase();
}

/** Resolve fixed party codes without using a company-name comparison. */
function resolvePurchaseRequestPartyCodes(request = {}, guestOverride = null) {
  const guestId = String(request.guestId || guestOverride?.id || '').trim();
  const guest = guestOverride
    || (APP_DATA.guestAccounts || []).find(account => String(account.id) === guestId)
    || null;
  const buyerCode = _purchaseRequestCode(request.buyerCode || guest?.buyerCode);
  const requestedClientCode = _purchaseRequestCode(request.clientCompanyCode);
  const clientCompany = (APP_DATA.clientCompanies || []).find(company =>
    (requestedClientCode && _purchaseRequestCode(company.id) === requestedClientCode)
    || (buyerCode && _purchaseRequestCode(company.buyerCode) === buyerCode)
    || (guestId && String(company.guestId || '').trim() === guestId)) || null;

  return {
    guest,
    guestId,
    buyerCode,
    clientCompanyCode: requestedClientCode || _purchaseRequestCode(clientCompany?.id),
    clientCompany,
  };
}

/** Migrate legacy requests from guest-id linkage to fixed buyer/client codes. */
function syncPurchaseRequestPartyCodes() {
  let changedCount = 0;
  (APP_DATA.purchaseRequests || []).forEach(request => {
    const resolved = resolvePurchaseRequestPartyCodes(request);
    let changed = false;
    if (!request.guestId && resolved.guestId) {
      request.guestId = resolved.guestId;
      changed = true;
    }
    if (!request.buyerCode && resolved.buyerCode) {
      request.buyerCode = resolved.buyerCode;
      changed = true;
    }
    if (!request.clientCompanyCode && resolved.clientCompanyCode) {
      request.clientCompanyCode = resolved.clientCompanyCode;
      changed = true;
    }
    if (changed) changedCount += 1;
  });
  return changedCount;
}

function findPurchaseRequestInventoryItem(requestItem) {
  const code = String(requestItem?.itemCode || requestItem?.code || '').trim();
  return (APP_DATA.inventory || []).find(item => String(item.code) === code) || null;
}

function _isActivePurchaseRequestReservation(request, requestItem) {
  return requestItem?.itemStatus === 'approved'
    && !PURCHASE_REQUEST_CLOSED_STATUSES.has(request?.status)
    && request?.fulfillmentStatus !== '出荷済'
    && !request?.shipmentId;
}

/** 承認済み商品を当該購入リクエスト専用に取り置く。 */
function reservePurchaseRequestItem(request, requestItem, { unpublish = true } = {}) {
  const inventoryItem = findPurchaseRequestInventoryItem(requestItem);
  if (!inventoryItem) {
    return { ok: false, reason: `商品 ${requestItem?.itemCode || ''} が在庫一覧に見つかりません` };
  }

  if (inventoryItem.status === PURCHASE_REQUEST_RESERVED_STATUS
      && inventoryItem.reservationRequestId === request.id) {
    return { ok: true, item: inventoryItem, alreadyReserved: true };
  }

  if (inventoryItem.status !== PURCHASE_REQUEST_AVAILABLE_STATUS) {
    const owner = inventoryItem.reservationRequestId
      ? `（${inventoryItem.reservationRequestId} で取置中）`
      : '';
    return { ok: false, reason: `商品 ${inventoryItem.code} は「${inventoryItem.status}」です${owner}` };
  }

  inventoryItem.status = PURCHASE_REQUEST_RESERVED_STATUS;
  inventoryItem.reservationRequestId = request.id;
  inventoryItem.reservedForGuestId = request.guestId || '';
  inventoryItem.reservedForGuestName = request.guestName || '';
  inventoryItem.reservedForBuyerCode = request.buyerCode || '';
  inventoryItem.reservedForClientCompanyCode = request.clientCompanyCode || '';
  inventoryItem.reservedAt = new Date().toISOString();

  // 公開済みスナップショットからも即時除外し、別ゲストからの再購入を防ぐ。
  if (unpublish && typeof unpublishGuestProducts === 'function') {
    unpublishGuestProducts([inventoryItem.code]);
  }
  return { ok: true, item: inventoryItem, alreadyReserved: false };
}

/** 当該購入リクエストが所有している取置だけを安全に解除する。 */
function releasePurchaseRequestItem(request, requestItem) {
  const inventoryItem = findPurchaseRequestInventoryItem(requestItem);
  if (!inventoryItem
      || inventoryItem.status !== PURCHASE_REQUEST_RESERVED_STATUS
      || inventoryItem.reservationRequestId !== request.id) return false;

  inventoryItem.status = PURCHASE_REQUEST_AVAILABLE_STATUS;
  delete inventoryItem.reservationRequestId;
  delete inventoryItem.reservedForGuestId;
  delete inventoryItem.reservedForGuestName;
  delete inventoryItem.reservedForBuyerCode;
  delete inventoryItem.reservedForClientCompanyCode;
  delete inventoryItem.reservedAt;
  return true;
}

function releasePurchaseRequestReservations(request) {
  return (request?.items || []).reduce(
    (count, item) => count + (releasePurchaseRequestItem(request, item) ? 1 : 0),
    0,
  );
}

/** 保存済みリクエストと在庫の取置状態を整合させる。 */
function syncPurchaseRequestReservations() {
  const activeKeys = new Set();
  (APP_DATA.purchaseRequests || []).forEach(request => {
    (request.items || []).forEach(requestItem => {
      if (!_isActivePurchaseRequestReservation(request, requestItem)) return;
      activeKeys.add(`${request.id}:${requestItem.itemCode}`);
      reservePurchaseRequestItem(request, requestItem);
    });
  });

  // 承認解除・却下・取消済みなのに残った取置だけを解除する。
  (APP_DATA.inventory || []).forEach(inventoryItem => {
    if (inventoryItem.status !== PURCHASE_REQUEST_RESERVED_STATUS
        || !inventoryItem.reservationRequestId) return;
    const key = `${inventoryItem.reservationRequestId}:${inventoryItem.code}`;
    if (activeKeys.has(key)) return;
    inventoryItem.status = PURCHASE_REQUEST_AVAILABLE_STATUS;
    delete inventoryItem.reservationRequestId;
    delete inventoryItem.reservedForGuestId;
    delete inventoryItem.reservedForGuestName;
    delete inventoryItem.reservedForBuyerCode;
    delete inventoryItem.reservedForClientCompanyCode;
    delete inventoryItem.reservedAt;
  });
}

function persistGuestSnapshot() {
  return _guestWriteStorage(GUEST_SNAPSHOT_STORAGE_KEY, APP_DATA.publishedSnapshot);
}

/**
 * 出荷・売上が確定した商品を、公開済みのゲストスナップショットから即時除外する。
 * 在庫へ戻った場合は自動再公開せず、管理者が公開更新した時だけ再掲載する。
 */
function unpublishGuestProducts(productCodes) {
  const codes = new Set((Array.isArray(productCodes) ? productCodes : [productCodes])
    .map(code => String(code || '').trim())
    .filter(Boolean));
  const snapshot = APP_DATA.publishedSnapshot;
  if (codes.size === 0 || !snapshot || !Array.isArray(snapshot.boxes)) return 0;

  let removedCount = 0;
  const boxes = snapshot.boxes.map(box => ({
    ...box,
    items: (box.items || []).filter(item => {
      const remove = codes.has(String(item.code || ''));
      if (remove) removedCount += 1;
      return !remove;
    }),
  }));
  if (removedCount === 0) return 0;

  const now = new Date();
  APP_DATA.publishedSnapshot = {
    ...snapshot,
    updatedAt: `${now.getFullYear()}/${String(now.getMonth() + 1).padStart(2, '0')}/${String(now.getDate()).padStart(2, '0')} ${String(now.getHours()).padStart(2, '0')}:${String(now.getMinutes()).padStart(2, '0')}`,
    boxes,
  };
  persistGuestSnapshot();
  return removedCount;
}

function persistGuestBoxState() {
  const assignments = {};
  APP_DATA.inventory.forEach(item => { assignments[item.code] = item.boxNo ?? null; });
  return _guestWriteStorage(GUEST_BOX_DRAFT_STORAGE_KEY, {
    boxes: APP_DATA.boxes,
    assignments,
    updatedAt: new Date().toISOString(),
  });
}

function persistPurchaseRequests() {
  return _guestWriteStorage(PURCHASE_REQUESTS_STORAGE_KEY, APP_DATA.purchaseRequests);
}

function nextPurchaseRequestId() {
  const max = APP_DATA.purchaseRequests.reduce((current, request) => {
    const match = String(request.id || '').match(/^PR-(\d+)$/);
    return match ? Math.max(current, Number(match[1])) : current;
  }, 0);
  return `PR-${String(max + 1).padStart(3, '0')}`;
}

function createGuestPurchaseRequest({ guest, items, note = '' }) {
  if (!guest || !Array.isArray(items) || items.length === 0) return null;
  const party = resolvePurchaseRequestPartyCodes({}, guest);
  if (!party.buyerCode) return null;
  const now = new Date();
  const request = {
    id: nextPurchaseRequestId(),
    guestId: guest.id,
    guestName: guest.name,
    buyerCode: party.buyerCode,
    clientCompanyCode: party.clientCompanyCode,
    date: now.toLocaleString('ja-JP', {
      year: 'numeric', month: '2-digit', day: '2-digit',
      hour: '2-digit', minute: '2-digit', hour12: false,
    }),
    status: '未対応',
    note: String(note || '').trim(),
    items: items.map(item => ({
      itemCode: item.code,
      itemName: `${item.brand} ${item.model}`.trim(),
      salePrice: Number(item.salePrice) || 0,
      itemStatus: 'pending',
      boxNo: item.boxNo ?? null,
    })),
  };
  APP_DATA.purchaseRequests.unshift(request);
  persistPurchaseRequests();
  return request;
}

hydrateGuestDomainState();

document.addEventListener('DOMContentLoaded', () => {
  if (syncPurchaseRequestPartyCodes() > 0) persistPurchaseRequests();
});

window.addEventListener('storage', event => {
  if (![GUEST_SNAPSHOT_STORAGE_KEY, GUEST_BOX_DRAFT_STORAGE_KEY, PURCHASE_REQUESTS_STORAGE_KEY].includes(event.key)) return;
  hydrateGuestDomainState();
  document.dispatchEvent(new CustomEvent('guest-domain-updated', { detail: { key: event.key } }));
});
