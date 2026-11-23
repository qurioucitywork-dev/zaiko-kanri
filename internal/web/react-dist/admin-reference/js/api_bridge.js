// REST API bridge for the reference UI. The visual layer stays intact while
// authentication and durable business data are sourced from Go/PostgreSQL.
(function () {
  // A connected screen starts empty; preview fixtures are only for offline design tests.
  // This runs before app.js can render, including when the first API request fails.
  if (typeof APP_DATA !== 'undefined') {
    for (const [key, value] of Object.entries(APP_DATA)) {
      if (Array.isArray(value)) APP_DATA[key] = [];
    }
    APP_DATA.itemNumberByDate = {};
    APP_DATA.publishedSnapshot = { boxes: [] };
    APP_DATA.companyInfo = {};
    APP_DATA.dashboardSettings = { salesTarget: 0, purchaseBudget: 0 };
    APP_DATA.fxRates = ['USD', 'HKD', 'EUR'].map(code => ({ code, rate: null }));
  }
  function currentRate(code) {
    if (String(code).toUpperCase() === 'JPY') return 1;
    const value = Number((APP_DATA.fxRates || []).find(item => item.code === String(code).toUpperCase())?.rate);
    return Number.isFinite(value) && value > 0 ? value : NaN;
  }
  const state = {
    csrfToken: '', user: null, hydrated: false, dashboardReady: false, loading: false, company: null, latestRate: null,
    lastError: null, lastLoadedAt: '', retryCount: 0, databaseDriver: '',
  };
  const transientStatuses = new Set([408, 425, 429, 500, 502, 503, 504]);
  const retryDelays = [1500, 5000, 15000, 30000, 60000];
  let hydrateAdminPromise = null;
  let hydrateRetryTimer = null;
  let hydrateRetryStep = 0;
  let syncSuccessTimer = null;
  const statusLabel = {
    purchasing: '仕入中', in_stock: '在庫中', cost_adjustment: '原価調整中', cost_adjusted: '原価調整済', broken_down: '原価調整済', combined: '結合済み', reserved: '取置中', return_pending: '仕入返品処理中', consigned: '委託中', shipped: '出荷済', sold: '売上済', cancelled: '仕入返品処理済',
    draft: '未処理', confirmed: '処理済', pending: '未対応', approved: '承認済', rejected: '却下', returned: '差戻し',
  };
  const approvalActionLabel = value => ({
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
  })[String(value || '')] || String(value || '承認申請');

  const wait = milliseconds => new Promise(resolve => window.setTimeout(resolve, milliseconds));
  // Limit database read pressure. Queue time must not consume the network timeout.
  const readQueue = [];
  let activeReads = 0;
  const MAX_CONCURRENT_READS = 8;
  async function acquireReadSlot() {
    if (activeReads >= MAX_CONCURRENT_READS) await new Promise(resolve => readQueue.push(resolve));
    else activeReads += 1;
    let released = false;
    return () => {
      if (released) return;
      released = true;
      const next = readQueue.shift();
      if (next) next();
      else activeReads -= 1;
    };
  }

  function retryAfterMilliseconds(response) {
    const value = response.headers.get('Retry-After');
    if (!value) return 0;
    const seconds = Number(value);
    if (Number.isFinite(seconds)) return Math.max(0, seconds * 1000);
    const timestamp = Date.parse(value);
    return Number.isFinite(timestamp) ? Math.max(0, timestamp - Date.now()) : 0;
  }

  function shouldRetryRequest(error, method, attempt, maxAttempts, externalSignal) {
    if (attempt >= maxAttempts || externalSignal?.aborted || !['GET', 'HEAD'].includes(method)) return false;
    if (error?.code === 'postgres_required') return false;
    if (error?.code === 'invalid_json_response') return true;
    if (transientStatuses.has(Number(error?.status))) return true;
    return error?.name === 'AbortError' || error?.code === 'request_timeout' || error instanceof TypeError;
  }

  async function request(path, options = {}) {
    if (state.user && !state.hydrated && !['GET', 'HEAD'].includes(String(options.method || 'GET').toUpperCase())
        && !['/auth/login', '/auth/logout'].includes(path)) {
      throw Object.assign(new Error('初回データの読み込み完了後に操作してください。'), { code: 'initial_data_loading' });
    }
    const operation = workerDocumentOperation(path, options.method || 'GET');
    if (typeof isWorker === 'function' && isWorker() && operation) {
      await request('/document-change-requests', {method:'POST',body:JSON.stringify({...operation,payload:JSON.parse(options.body || '{}')})});
      await hydrateAdmin();
      if (typeof closeSlipRevise === 'function') closeSlipRevise();
      if (typeof closeSalesChangeModal === 'function') closeSalesChangeModal();
      throw Object.assign(new Error('承認申請を送信しました。管理者承認後に反映されます。'), {approvalRequested:true});
    }
    const { retry = true, timeoutMs = 12000, ...fetchOptions } = options;
    const method = String(fetchOptions.method || 'GET').toUpperCase();
    const maxAttempts = retry && ['GET', 'HEAD'].includes(method) ? 4 : 1;
    const headers = { Accept: 'application/json', ...(fetchOptions.headers || {}) };
    const access=window.__adminPageAccess;
    if (access) { headers['X-Admin-Page']=access.page; headers['X-Admin-Page-Code']=access.code; }
    if (fetchOptions.body && !(fetchOptions.body instanceof FormData)) headers['Content-Type'] = 'application/json; charset=utf-8';
    if (method !== 'GET' && method !== 'HEAD' && state.csrfToken) headers['X-CSRF-Token'] = state.csrfToken;

    for (let attempt = 1; attempt <= maxAttempts; attempt += 1) {
      const releaseRead = ['GET', 'HEAD'].includes(method) ? await acquireReadSlot() : () => {};
      const controller = new AbortController();
      const externalSignal = fetchOptions.signal;
      const abortFromExternal = () => controller.abort(externalSignal.reason);
      if (externalSignal) externalSignal.addEventListener('abort', abortFromExternal, { once: true });
      const timeout = window.setTimeout(() => controller.abort('request-timeout'), timeoutMs);
      try {
        const response = await fetch(`/api/v1${path}`, {
          credentials: 'same-origin',
          cache: method === 'GET' || method === 'HEAD' ? 'no-store' : 'default',
          ...fetchOptions,
          headers,
          signal: controller.signal,
        });
        if (response.status === 204) return null;
        const raw = await response.text();
        let payload = {};
        if (raw) {
          try {
            payload = JSON.parse(raw);
          } catch (_) {
            const invalid = new Error(`API応答を読み取れませんでした (${response.status})`);
            invalid.status = response.status;
            invalid.code = 'invalid_json_response';
            throw invalid;
          }
        }
        if (!response.ok) {
          const error = new Error(payload.error?.message || `API error (${response.status})`);
          error.status = response.status;
          error.code = payload.error?.code;
          error.retryAfterMs = retryAfterMilliseconds(response);
          throw error;
        }
        return payload;
      } catch (caught) {
        const error = caught?.name === 'AbortError' && !externalSignal?.aborted
          ? Object.assign(new Error('DBからの応答がタイムアウトしました。'), { code: 'request_timeout', name: 'TimeoutError' })
          : caught;
        if (!shouldRetryRequest(error, method, attempt, maxAttempts, externalSignal)) throw error;
        const backoff = Number(error?.retryAfterMs) || Math.min(4000, 350 * (2 ** (attempt - 1))) + Math.floor(Math.random() * 180);
        releaseRead();
        await wait(backoff);
      } finally {
        releaseRead();
        window.clearTimeout(timeout);
        if (externalSignal) externalSignal.removeEventListener('abort', abortFromExternal);
      }
    }
    throw new Error('DBデータを取得できませんでした。');
  }

  function workerDocumentOperation(path,method) {
    const match=path.match(/^\/(purchases|sales|shipments|consignments|returns)\/([^/]+)(?:\/(revision|paid|issue|tracking|withdraw|adjustments|return-scan))?$/);
    if(!match)return null;
    const type=({purchases:'purchase',sales:'sale',shipments:'shipment',consignments:'consignment',returns:'return'})[match[1]];
    const action=match[3] || (method.toUpperCase()==='DELETE'?'delete':'');
    const allowed=['purchase.revision','sale.revision','shipment.revision','consignment.revision','return.revision','purchase.paid','sale.paid','shipment.issue','return.issue','purchase.issue','sale.issue','consignment.issue','purchase.delete','return.withdraw','shipment.tracking','return.tracking','sale.adjustments','consignment.return-scan'];
    if(!['POST','PATCH','DELETE'].includes(method.toUpperCase()) || !allowed.includes(type+'.'+action))return null;
    return {kind:type+'.'+action,targetId:decodeURIComponent(match[2])};
  }

  function sessionPayload(result) {
    const user = result.user;
    return {
      id: user.id,
      role: user.role,
      name: user.displayName,
      avatar: (user.displayName || user.username || '?').slice(0, 1),
      loginId: user.username,
      csrfToken: result.csrfToken,
      api: true,
    };
  }

  async function login(username, password, expectedRole) {
    const result = await request('/auth/login', { method: 'POST', body: JSON.stringify({ username, password }) });
    if (expectedRole && result.user.role !== expectedRole && !(expectedRole === 'worker' && result.user.role === 'worker')) {
      await request('/auth/logout', { method: 'POST', headers: { 'X-CSRF-Token': result.csrfToken } }).catch(() => {});
      throw new Error('選択した利用区分とアカウント権限が一致しません。');
    }
    state.csrfToken = result.csrfToken;
    state.user = result.user;
    const session = sessionPayload(result);
    if (typeof setSession === 'function') setSession(session);
    return session;
  }

  async function logout() {
    const stored = typeof getSession === 'function' ? getSession() : null;
    state.csrfToken = state.csrfToken || stored?.csrfToken || '';
    await request('/auth/logout', { method: 'POST' }).catch(() => {});
    if (typeof clearSession === 'function') clearSession();
    sessionStorage.removeItem('inv_guest_session');
  }

  async function optional(path, { sqliteFallback = false } = {}) {
    try { return await request(path, { retry: !(sqliteFallback && state.databaseDriver === 'sqlite') }); } catch (error) {
      if (error.status === 403 && state.user?.role !== 'admin') return null;
      if (state.databaseDriver === 'sqlite' && (error.status === 404 || (error.status === 503 && error.code === 'postgres_required'))) return null;
      // The bundled SQLite fallback intentionally omits some PostgreSQL-only
      // accounting catalogs. Those auxiliary screens must not block core
      // inventory/purchase hydration; the same error remains fatal in PostgreSQL.
      if (sqliteFallback && state.databaseDriver === 'sqlite' && Number(error.status) >= 500) return null;
      throw error;
    }
  }

  function ensureSyncStatus() {
    let element = document.getElementById('dbSyncStatus');
    if (element || !document.body) return element;
    element = document.createElement('aside');
    element.id = 'dbSyncStatus';
    element.className = 'db-sync-status hidden';
    element.setAttribute('role', 'status');
    element.setAttribute('aria-live', 'polite');
    element.innerHTML = `<i class="fa-solid fa-database" aria-hidden="true"></i>
      <span class="db-sync-status-message"></span>
      <button type="button" class="db-sync-status-retry"><i class="fa-solid fa-rotate-right" aria-hidden="true"></i> 今すぐ再取得</button>`;
    element.querySelector('.db-sync-status-retry')?.addEventListener('click', () => {
      if (hydrateRetryTimer) window.clearTimeout(hydrateRetryTimer);
      hydrateRetryTimer = null;
      hydrateAdmin().catch(() => {});
    });
    document.body.appendChild(element);
    return element;
  }

  function showSyncStatus(kind, message) {
    const element = ensureSyncStatus();
    if (!element) return;
    if (syncSuccessTimer) window.clearTimeout(syncSuccessTimer);
    element.className = `db-sync-status db-sync-status-${kind}`;
    const text = element.querySelector('.db-sync-status-message');
    if (text) text.textContent = message;
    const retryButton = element.querySelector('.db-sync-status-retry');
    if (retryButton) retryButton.hidden = kind !== 'error';
    if (kind === 'success') {
      syncSuccessTimer = window.setTimeout(() => element.classList.add('hidden'), 5000);
    }
  }

  function snapshotAppData() {
    if (!state.hydrated) return null;
    try { return structuredClone(APP_DATA); } catch (_) {
      try { return JSON.parse(JSON.stringify(APP_DATA)); } catch (_) { return null; }
    }
  }

  function restoreAppData(snapshot) {
    if (!snapshot) return;
    Object.keys(APP_DATA).forEach(key => { delete APP_DATA[key]; });
    Object.assign(APP_DATA, snapshot);
  }

  function scheduleHydrationRetry(error) {
    if (hydrateRetryTimer || error?.status === 401 || error?.status === 403) return;
    const delay = retryDelays[Math.min(hydrateRetryStep, retryDelays.length - 1)];
    hydrateRetryStep += 1;
    hydrateRetryTimer = window.setTimeout(() => {
      hydrateRetryTimer = null;
      hydrateAdmin().catch(() => {});
    }, delay);
  }

  /**
   * ページングされた一覧APIを先頭ページから順番に取得し、itemsを1つに結合する。
   * 最初のページが権限などで取得できない場合は optional() と同じく null を返すが、
   * 途中ページの失敗は部分データを在庫全件として扱わないよう、そのままエラーにする。
   */
  async function fetchAllPages(path, pageSize = 100) {
    const separator = path.includes('?') ? '&' : '?';
    const first = await optional(`${path}${separator}page=1&pageSize=${pageSize}`);
    if (!first || !Array.isArray(first.items)) return first;

    const totalPages = Math.max(1, Number(first.totalPages) || 1);
    const items = [...first.items];
    for (let page = 2; page <= totalPages; page += 1) {
      const next = await request(`${path}${separator}page=${page}&pageSize=${pageSize}`);
      if (Array.isArray(next?.items)) items.push(...next.items);
    }

    return {
      ...first,
      items,
      page: 1,
      pageSize,
      total: Number(first.total) || items.length,
    };
  }

  async function fetchAllPagesWithFallback(path, pageSize = 100) {
    try {
      return await fetchAllPages(path, pageSize);
    } catch (error) {
      if (state.databaseDriver === 'sqlite' && Number(error.status) >= 500) return null;
      throw error;
    }
  }

  function roleItem(partner, role) {
    return {
      _partnerId: partner.id,
      auctionFees: partner.auctionFees || {},
      _partnerCode: partner.partnerCode,
      _roleId: role.id,
      code: role.roleCode,
      name: partner.legalName,
      address: [partner.postalCode, partner.address].filter(Boolean).join(' '),
      contact: partner.phone,
      invoice: partner.invoiceNumber,
      regionType: partner.regionType || 'domestic',
      closingDay: partner.closingDay || null,
      isOther: Boolean(partner.isOther),
      email: partner.email,
      partnerCode: partner.partnerCode,
      guestManaged: Boolean(role.guestCode),
      guestId: role.guestCode || '',
      active: role.isActive && partner.status === 'active',
    };
  }

  function applyMasters(results) {
    const assign = (key, target, namesOnly = false) => {
      const items = results[key]?.items;
      if (!Array.isArray(items)) return;
      APP_DATA[target] = namesOnly ? items.map(item => item.name) : items.map(item => ({ ...item }));
    };
    assign('brands', 'brandRecords');
    if (results.brands?.items) APP_DATA.brands = results.brands.items.map(item => item.name);
    assign('materials', 'materials');
    assign('movements', 'movements');
    assign('conditions', 'conditions');
    if (results.accessories?.items) {
      APP_DATA.legacyAccessoryRecords = results.accessories.referenceItems || [];
      APP_DATA.accessoryRecords = results.accessories.items.filter(item => !item.isLegacy);
      APP_DATA.accessories = APP_DATA.accessoryRecords.map(item => item.name);
    }
    assign('auctions', 'auctionRecords');
    assign('belt-materials', 'beltMaterialRecords');
    assign('dials', 'dialRecords');
	assign('product-shapes', 'shapeRecords');
	assign('markings', 'markingRecords');
	assign('part-names', 'partNameRecords');
  }

  function applyPartners(partners) {
    if (!Array.isArray(partners)) return;
    APP_DATA.suppliers = [];
    APP_DATA.buyers = [];
    partners.forEach(partner => (partner.roles || []).forEach(role => {
      if (!role.isActive || partner.status !== 'active') return;
      if (role.roleType === 'supplier') APP_DATA.suppliers.push(roleItem(partner, role));
      if (role.roleType === 'buyer') APP_DATA.buyers.push(roleItem(partner, role));
    }));
    APP_DATA.clientCompanies = partners.map(partner => ({
      _id: partner.id,
      auctionFees: partner.auctionFees || {},
      id: partner.partnerCode,
      companyName: partner.legalName,
      representative: partner.representativeName,
      contactPerson: '',
      email: partner.email,
      tel: partner.phone,
      contactPhone: partner.contactPhone || '',
      postalCode: partner.postalCode,
      address: partner.address,
      invoice: partner.invoiceNumber,
      antiqueLicenseNumber: partner.antiqueLicenseNumber || '',
      regionType: partner.regionType || 'domestic',
      closingDay: partner.closingDay || null,
      isOther: Boolean(partner.isOther),
      note: partner.notes,
      tradeTypes: (partner.roles || []).filter(role => role.isActive).map(role => role.roleType),
      buyerCode: partner.roles?.find(role => role.roleType === 'buyer' && role.isActive)?.roleCode || '',
      supplierCode: partner.roles?.find(role => role.roleType === 'supplier' && role.isActive)?.roleCode || '',
      guestId: partner.roles?.find(role => role.roleType === 'buyer' && role.isActive)?.guestCode || '',
    }));
  }

  function applyUsers(users) {
    if (!Array.isArray(users)) return;
    APP_DATA.users = users.filter(user => user.role !== 'guest').map(user => ({
      id: user.id,
      role: user.role,
      name: user.displayName,
      loginId: user.username,
      email: user.email,
      active: user.isActive,
      avatar: (user.displayName || user.username).slice(0, 1),
      password: '',
      apiManaged: true,
    }));
    APP_DATA.guestAccounts = users.filter(user => user.role === 'guest').map(user => ({
      id: user.guestCode,
      userId: user.id,
      name: user.displayName,
      company: user.companyName,
      buyerCode: user.buyerCode,
      partnerCode: user.partnerCode,
      email: user.email,
      active: user.isActive,
      password: '',
      apiManaged: true,
    }));

  }

  function applyPurchaseStaff(staff) {
    if (!Array.isArray(staff)) return;
    APP_DATA.staff = staff.map(record => record.displayName);
    APP_DATA.staffRecords = staff.map(record => ({
      _id: record.id, code: record.staffCode, name: record.displayName, active: true,
      inventoryDetail: record.inventoryDetail || '', overseasDetail: record.overseasDetail || '', tagDetail: record.tagDetail || '', domesticDetail: record.domesticDetail || '',
      isPurchaseStaff: true, apiManaged: true,
    }));
  }

  function masterCode(items, id) {
    return items?.find(item => item.id === id)?.code || '';
  }

  function applyProducts(products, masters, users, rate) {
    if (!Array.isArray(products)) return;
    const staffName = id => users?.find(user => user.staffCode && (user.staffProfileId === id || user.id === id || `staff_${user.username}` === id))?.displayName || '';
    APP_DATA.inventory = products.map(product => ({
      _id: product.id,
      code: product.productCode,
      sku: product.sku,
      brand: product.brand,
      brandCode: masterCode(masters.brands?.items, product.brandId),
      model: product.modelNumber,
      ref: product.referenceNumber || '',
      serial: product.serialNumber,
      supplier: APP_DATA.suppliers?.find(record => record._roleId === product.supplierRoleId)?.code
        || (product.supplierRoleId || '').replace('partner_role_', ''),
      supplierName: product.supplierName || '',
      staff: staffName(product.purchaseStaffProfileId),
      staffCode: (APP_DATA.staffRecords || []).find(record => record._id === product.purchaseStaffProfileId)?.code
        || users?.find(user => user.staffProfileId === product.purchaseStaffProfileId)?.staffCode || '',
      // Use the current inventory cost in JPY. Direct adjustments are added to
      // this value while the original purchase slip and its fixed FX rate stay immutable.
      purchasePrice: Object.prototype.hasOwnProperty.call(product, 'inventoryCostJpyMinor')
        ? Number(product.inventoryCostJpyMinor) || 0
        : (Number(product.fixedPurchaseCostJpyMinor)
          || (product.costCurrency === 'JPY' ? Number(product.costAmountMinor) || 0 : Math.round((Number(product.costAmountMinor) || 0) * rate))),
      purchaseCurrency: product.purchaseSourceCurrency || product.costCurrency,
      purchaseType: ['personal', 'overseas'].includes(product.purchaseTaxMode) ? product.purchaseTaxMode : 'domestic',
      purchaseOriginalPrice: Number(product.purchaseSourceAmountMinor) || Number(product.costAmountMinor) || 0,
      purchaseFxRateScaled: Number(product.purchaseFxRateScaled) || 0,
      purchaseFxScale: Number(product.purchaseFxScale) || 0,
      purchaseFxRateObservedAt: product.purchaseFxRateObservedAt || null,
      salePrice: product.baseSaleCurrency === 'JPY' ? Math.round(product.baseSalePriceMinor / rate) : product.baseSalePriceMinor,
      saleCurrency: product.baseSaleCurrency,
      saleOriginalPrice: Number(product.baseSalePriceMinor) || 0,
      saleDate: product.saleDate || null,
      saleSlipId: product.saleSlipId || '', saleBuyerCode: product.saleBuyerCode || '',
      purchaseSlipId: product.purchaseSlipId || '', dashboardRawStatus: product.inventoryStatus,
      dashboardPurchaseJpy: Number(product.dashboardPurchaseJpy) || 0,
      dashboardCostJpy: Number(product.dashboardCostJpy) || 0,
      dashboardPurchaseUsdRate: Number(product.dashboardPurchaseUsdRate) || null,
      dashboardSaleUsdRate: Number(product.dashboardSaleUsdRate) || null,
      actualSaleCurrency: product.saleCurrency || '',
      actualSalePrice: Number(product.saleAdjustedPriceMinor) || 0,
      actualSalePriceJpy: Number(product.salePriceJpyMinor) || 0,
      saleFxRateScaled: Number(product.saleFxRateScaled) || 0,
      saleFxScale: Number(product.saleFxScale) || 0,
      saleFxRateObservedAt: product.saleFxRateObservedAt || null,
      purchaseDate: product.purchaseDate,
      purchaseSlipLineId: product.purchaseSlipLineId || '',
      status: statusLabel[product.inventoryStatus] || product.inventoryStatus,
      material: masterCode(masters.materials?.items, product.materialId),
      movement: masterCode(masters.movements?.items, product.movementId),
      condition: masterCode(masters.conditions?.items, product.conditionId) || product.condition,
	  shape: masterCode(masters['product-shapes']?.items, product.shapeId),
	  marking: masterCode(masters.markings?.items, product.markingId),
      weightGrams: product.weightGrams ?? '',
      accessories: String(product.accessories || '').split(',').map(value => value.trim()).filter(Boolean).map(code =>
        masters.accessories?.items?.find(item => item.code === code)?.name || code),
      accessorySupplements: {...(product.accessorySupplements || {})},
      belt: product.beltText || '', dial: product.dialText || '', braceletQty: product.braceletQuantity ?? null,
      images: [],
      imageCount: product.imageCount || 0,
      note: product.notes || '',
	  comment: product.internalComment || '',
	  costAdjustmentId: product.costAdjustmentId || '',
	  inventoryValuationExcluded: Boolean(product.inventoryValuationExcluded),
	  inventoryValuationExclusionReason: product.inventoryValuationExclusionReason || '',
	  inventoryValuationExcludedAt: product.inventoryValuationExcludedAt || null,
      revisions: [],
      apiManaged: true,
    }));
  }

  function purchaseView(record, productByLine, rate, staffDirectory) {
    const staffName = (staffDirectory || []).find(user => user.staffCode === record.staffCode)?.displayName
      || record.staffCode;
    const accessoryNames = values => (Array.isArray(values) ? values : String(values || '').split(','))
      .map(value => String(value || '').trim()).filter(Boolean).map(code =>
        allAccessoryMasterRecords().find(item => item.code === code)?.name || code);
    const productNotes = value => {
      const parts = String(value || '').split(' / ').map(part => part.trim()).filter(Boolean);
      const boxPart = parts.find(part => part.startsWith('BOX:')) || '';
      return {
        boxNo: boxPart.slice(4).trim(),
        note: parts.filter(part => !/^(?:BOX|ベルト|文字盤):/u.test(part)).join(' / '),
      };
    };
    return {
      _id: record.id, id: record.slipNumber, date: record.purchaseDate, supplier: record.supplierCode,
      supplierName: record.supplierName || '',
      staff: staffName, staffCode: record.staffCode, note: record.notes,
      rawStatus: record.status, status: statusLabel[record.status] || record.status,
      purchaseTaxMode: ['personal', 'overseas'].includes(record.purchaseTaxMode) ? record.purchaseTaxMode : 'domestic',
      taxCategory: ['tax_equivalent', 'out_of_scope'].includes(record.taxCategory)
        ? record.taxCategory : (record.purchaseTaxMode === 'domestic' ? 'consumption_tax' : 'out_of_scope'),
      paymentMethod: ['cash', 'card'].includes(record.paymentMethod) ? record.paymentMethod : 'bank_transfer',
      purchaseCurrency: ['JPY', 'USD', 'HKD'].includes(String(record.lines?.[0]?.costCurrency || '').toUpperCase())
        ? String(record.lines[0].costCurrency).toUpperCase() : 'JPY',
      createdAt: record.createdAt,
      csvRegistrationUsdJpyRate: Number(record.registrationUsdJpyRate) || null,
      registrationPurchaseCurrency: record.lines?.[0]?.costCurrency || 'JPY',
      registrationPurchaseJpyRate: Number(record.lines?.[0]?.fxRateScaled) > 0 && Number(record.lines?.[0]?.fxScale) > 0
        ? Number(record.lines[0].fxRateScaled) / Number(record.lines[0].fxScale) : 1,
      taxRateBasisPoints: Number(record.taxRateBasisPoints) || 0,
      registeredAt: record.createdAt, issuedAt: record.issuedAt || null, issuedBy: record.issuedBy || null,
      paidAt: record.paidAt || null, paidBy: record.paidBy || null,
	  arrivalStatus: record.arrivalStatus || (Number(record.pendingArrivalCount) > 0 ? 'processing' : 'completed'),
	  pendingArrivalCount: Number(record.pendingArrivalCount) || 0,
	  arrivedCount: Number(record.arrivedCount) || 0,
      revisions: [], apiManaged: true,
      lines: (record.lines || []).map(line => {
        // purchaseSlipLineId is the stable DB link. Keep the original line snapshot for audit,
        // while presenting the latest editable product details after the product has been created.
        const currentProduct = line.productCode
          ? APP_DATA.inventory.find(item => item.code === line.productCode)
          : productByLine.get(line.id);
        const isPart = line.lineItemKind === 'part' || Boolean(line.partCode);
        const annotations = productNotes(line.notes);
        return {
          _id: line.id,
          lineNo: line.lineNumber,
          code: line.productCode || currentProduct?.code || line.partCode || '',
          sku: currentProduct ? currentProduct.sku : line.sku,
          quantity: line.quantity,
          generatedProductCount: Number(line.generatedProductCount) || 0,
		  generatedPartCount: Number(line.generatedPartCount) || 0,
		  lineItemKind: isPart ? 'part' : 'product',
		  productId: line.productId || currentProduct?._id || '',
		  partId: line.partId || '',
		  currentStatus: isPart
		    ? (statusLabel[line.partStatus] || line.partStatus || '')
		    : (statusLabel[line.inventoryStatus] || currentProduct?.status || line.inventoryStatus || ''),
          // 仕入伝票の金額は起票時通貨の原額を保持する。商品詳細は最新値へ追随するが、
          // 海外仕入を円換算してしまわないよう伝票明細の通貨スナップショットを優先する。
          purchasePrice: line.unitCostMinor,
          auctionFee: line.auctionFee || {},
          purchaseCurrency: line.costCurrency,
          convertedPurchasePriceJpy: Number(line.convertedTotalJpy) || 0,
          purchaseFxRateScaled: Number(line.fxRateScaled) || 0,
          purchaseFxScale: Number(line.fxScale) || 0,
          purchaseFxRateObservedAt: line.fxRateObservedAt || null,
          salePrice: currentProduct
            ? currentProduct.salePrice
            : (line.baseSaleCurrency === 'JPY' ? Math.round(line.baseSalePriceMinor / rate) : line.baseSalePriceMinor),
          saleCurrency: currentProduct?.saleCurrency || line.baseSaleCurrency,
          saleOriginalPrice: currentProduct?.saleOriginalPrice ?? line.baseSalePriceMinor,
          productDetail: {
            brand: currentProduct ? currentProduct.brand : line.brandName,
            brandCode: currentProduct ? (currentProduct.brandCode || '') : (line.brandCode || ''),
            model: currentProduct ? currentProduct.model : line.modelNumber,
            ref: currentProduct ? currentProduct.ref : line.referenceNumber,
            serial: currentProduct ? currentProduct.serial : line.serialNumber,
            material: currentProduct ? currentProduct.material : (line.materialCode || ''),
            movement: currentProduct ? currentProduct.movement : (line.movementCode || ''),
            condition: currentProduct ? currentProduct.condition : line.conditionCode,
            accessories: currentProduct ? [...(currentProduct.accessories || [])] : accessoryNames(line.accessoryCodes),
            accessorySupplements: {...(currentProduct?.accessorySupplements || line.accessorySupplements || {})},
            weightGrams: currentProduct?.weightGrams ?? line.weightGrams ?? '',
            belt: currentProduct ? currentProduct.belt : (line.beltText || ''),
            dial: currentProduct ? currentProduct.dial : (line.dialText || ''),
            braceletQty: currentProduct ? currentProduct.braceletQty : (line.braceletQuantity || null),
            boxNo: currentProduct ? (currentProduct.boxNo ?? null) : (line.boxCode ? Number(line.boxCode.replace('BOX-', '')) : annotations.boxNo),
            note: currentProduct ? currentProduct.note : annotations.note,
          },
        };
      }),
    };
  }

  /** Match the sale that preceded this return, not a later resale of the same product. */
  function findSourceSaleForReturn(record, sales) {
    if (record.operationType === 'purchase_return') return null;
    if (record.adjustmentDetails?.sourceSaleSlipNumber) return sales.find(sale=>sale.slipNumber===record.adjustmentDetails.sourceSaleSlipNumber) || null;
    const codes = (record.lines || []).map(line => line.productCode).filter(Boolean);
    if (!codes.length) return null;
    const returnedAt = Date.parse(record.createdAt || '');
    const saleTime = sale => Date.parse(sale.confirmedAt || sale.createdAt || '');
    const candidates = sales.filter(sale => {
      if (sale.documentType === 'sales_revision' || (sale.status && sale.status !== 'confirmed')) return false;
      if (record.buyerCode && sale.buyerCode !== record.buyerCode) return false;
      if (!codes.every(code => (sale.lines || []).some(line => line.productCode === code))) return false;
      const soldAt = saleTime(sale);
      if (Number.isFinite(returnedAt) && Number.isFinite(soldAt)) return soldAt <= returnedAt;
      return !record.transactionDate || !sale.saleDate || sale.saleDate <= record.transactionDate;
    });
    const sourceNumber = String(record.notes || '').match(/元売上伝票[:：]\s*(\S+)/u)?.[1];
    const explicit = candidates.find(sale => sale.slipNumber === sourceNumber);
    if (explicit) return explicit;
    return candidates.sort((a,b) => {
      const aTime = saleTime(a), bTime = saleTime(b);
      if (Number.isFinite(aTime) && Number.isFinite(bTime) && aTime !== bTime) return bTime - aTime;
      return String(b.saleDate || '').localeCompare(String(a.saleDate || ''))
        || String(b.slipNumber || '').localeCompare(String(a.slipNumber || ''));
    })[0] || null;
  }

  function saleView(record) {
    if (record.documentType === 'sales_revision') {
      const direction = ['decrease', 'mixed'].includes(record.adjustmentDirection) ? record.adjustmentDirection : 'increase';
      const sign = direction === 'decrease' ? -1 : 1;
      return {
        _id: record.id, id: record.slipNumber, date: record.saleDate,
        buyer: record.buyerCode, status: statusLabel[record.status] || record.status,
        documentType: 'sales_revision', sourceSaleId: record.sourceSaleId,
        sourceSlipNumber: record.sourceSlipNumber, adjustmentDirection: direction,
        currency: record.displayCurrency, inputCurrency: record.displayCurrency,
        taxMode: record.taxMode || 'out_of_scope', taxRateBasisPoints: Number(record.taxRateBasisPoints) || 0,
        fxRateScaled: Number(record.fxRateScaled) || 0, fxScale: Number(record.fxScale) || 0,
        displaySubtotalMinor: sign * (Number(record.subtotalMinor ?? record.totalMinor) || 0),
        displayTaxAmountMinor: sign * (Number(record.taxAmountMinor) || 0),
        displayTotalMinor: sign * (Number(record.totalMinor) || 0),
        totalJpy: sign * (Number(record.convertedTotalJpy) || 0),
        note: record.notes || '', reason: record.notes || '', apiManaged: true,
        revisions: [],
        items: (record.lines || []).map(line => {
          const lineDirection = line.direction || direction;
          const lineSign = lineDirection === 'decrease' ? -1 : 1;
          return {
          direction: lineDirection,
          _id: line.id, sourceSaleLineId: line.sourceSaleLineId, productId: line.productId,
          code: line.productCode, brand: line.brand, model: line.modelNumber,
          adjustmentAmount: Number(line.adjustmentAmountMinor) || 0,
          beforeSalePrice: line.beforeSalePriceMinor == null ? null : Number(line.beforeSalePriceMinor),
          afterSalePrice: line.afterSalePriceMinor == null ? null : Number(line.afterSalePriceMinor),
          reason: line.reason || record.notes || '',
          convertedAdjustmentJpy: Number(line.convertedAdjustmentJpy) || 0,
          inputAmount: lineSign * (Number(line.adjustmentAmountMinor) || 0),
          inputCurrency: record.displayCurrency, currency: record.displayCurrency,
          salePrice: lineSign * (Number(line.adjustmentAmountMinor) || 0),
        }; }),
      };
    }
    const rate = Number(record.fxRateScaled) > 0 && Number(record.fxScale) > 0
      ? Number(record.fxRateScaled) / Number(record.fxScale)
      : Number(state.latestRate || 155);
    const inputCurrency = ['JPY', 'USD', 'HKD', 'EUR'].includes(record.displayCurrency) ? record.displayCurrency : 'USD';
    const toUSD = amount => {
      const value = Number(amount) || 0;
      if (inputCurrency === 'USD') return value;
      if (inputCurrency === 'JPY') return Math.round(value / rate);
      return Math.round(value * rate / Number(state.latestRate || 155));
    };
    const sourceMatch = String(record.notes || '').match(/^(出荷|委託)伝票:\s*(\S+)/mu);
    const sourceDocumentType = sourceMatch?.[1] === '出荷'
      ? 'shipment'
      : (sourceMatch?.[1] === '委託' ? 'consignment' : '');
    const sourceDocumentId = sourceMatch?.[2] || '';
    return {
      _id: record.id, id: record.slipNumber, date: record.saleDate, buyer: record.buyerCode,
      status: statusLabel[record.status] || record.status, currency: record.displayCurrency, taxMode: record.taxMode,
      taxRateBasisPoints: Number(record.taxRateBasisPoints) || 0,
      inputCurrency, usdJpyRate: rate, createdAt: record.createdAt,
      fxRateSnapshotId: record.fxRateSnapshotId || '',
      fxRateScaled: Number(record.fxRateScaled) || 0, fxScale: Number(record.fxScale) || 0,
      issuedAt: record.issuedAt || null, issuedBy: record.issuedBy || null,
      paidAt: record.paidAt || null, paidBy: record.paidBy || null,
      taxFree: record.taxMode === 'tax_exempt' && inputCurrency !== 'USD',
      total: toUSD(record.subtotalMinor), taxAmount: toUSD(record.taxAmountMinor),
      grandTotal: toUSD(record.totalMinor), totalJpy: record.convertedTotalJpy,
      displaySubtotalMinor: Number(record.subtotalMinor) || 0,
      displayTaxAmountMinor: Number(record.taxAmountMinor) || 0,
      displayTotalMinor: Number(record.totalMinor) || 0,
      note: record.notes, apiManaged: true,
      sourceDocumentType, sourceDocumentId,
      sourceShipmentId: sourceDocumentType === 'shipment' ? sourceDocumentId : '',
      sourceConsignmentId: sourceDocumentType === 'consignment' ? sourceDocumentId : '',
      items: (record.lines || []).map(line => ({
        _id: line.id, productId: line.productId,
        code: line.productCode, brand: line.brand, model: line.modelNumber,
        ref: line.referenceNumber || '', serial: line.serialNumber || '',
        accessories: String(line.accessories || '').split(',').map(value => value.trim()).filter(Boolean).map(code =>
          allAccessoryMasterRecords().find(item => item.code === code)?.name || code),
        salePrice: toUSD(line.unitPriceMinor),
        auctionFee: line.auctionFee || {},
        inputAmount: Number(line.unitPriceMinor) || 0,
        inputCurrency: line.saleCurrency,
        currency: line.saleCurrency, subtotal: line.subtotalMinor,
        taxAmount: line.taxAmountMinor, total: line.totalMinor,
      })),
    };
  }

  function shipmentView(record) {
    const rate = Number(record.fxRateScaled) > 0 && Number(record.fxScale) > 0
      ? Number(record.fxRateScaled) / Number(record.fxScale)
      : Number(state.latestRate || 155);
    const items = (record.lines || []).map(line => ({
      code: line.productCode,
      _id: line.id, shipmentPending: line.shipmentPending,
      brand: line.brand,
      model: line.modelNumber,
      salePrice: Number(line.salePriceUsdMinor) || 0,
      salePriceUsd: Number(line.salePriceUsdMinor) || 0,
      convertedSalePriceJpy: Number(line.convertedSalePriceJpy) || 0,
    }));
    return {
      _id: record.id, id: record.slipNumber, date: record.shipmentDate, destination: record.buyerCode,
      buyer: record.buyerCode, status: statusLabel[record.status] || record.status, apiStatus: record.status, carrier: record.carrier,
      trackingNo: record.trackingNumber, address: record.recipientAddress, recipient: record.recipientName,
      note: record.notes, apiManaged: true,
      processingStatus: record.processingStatus,
      updatedAt: record.updatedAt,
      revisions: (record.revisions || []).map(item => ({revisedAt:new Date(item.revisedAt).toLocaleString('ja-JP'),buyerName:_escHtml(item.actorName || ''),approverName:_escHtml(item.actorName || ''),note:_escHtml(item.reason || '')})),
      displayCurrency: record.displayCurrency === 'JPY' ? 'JPY' : 'USD',
      inputCurrency: record.displayCurrency === 'JPY' ? 'JPY' : 'USD',
      usdJpyRate: rate, createdAt: record.createdAt,
      fxRateScaled: Number(record.fxRateScaled) || 0, fxScale: Number(record.fxScale) || 0,
      taxRateBasisPoints: Number(record.taxRateBasisPoints) || 0,
      total: items.reduce((sum, item) => sum + item.salePriceUsd, 0),
      totalJpy: items.reduce((sum, item) => sum + item.convertedSalePriceJpy, 0),
      items,
    };
  }

  function consignmentView(record) {
    const rate = Number(record.fxRateScaled) > 0 && Number(record.fxScale) > 0
      ? Number(record.fxRateScaled) / Number(record.fxScale) : 0;
    const items = (record.lines || []).map(line => ({
      _id: line.id,
      code: line.productCode,
      brand: line.brand || '',
      model: line.modelNumber || '',
      salePrice: Number(line.salePriceUsdMinor) || 0,
      salePriceUsd: Number(line.salePriceUsdMinor) || 0,
      convertedSalePriceJpy: Number(line.convertedSalePriceJpy) || 0,
    }));
    return {
      _id: record.id,
      id: record.slipNumber,
      date: record.consignmentDate,
      destination: record.consigneeCode,
      consignee: record.consigneeCode,
      consigneeName: record.consigneeName, apiStatus: record.status,
      processingStatus: record.processingStatus,
      status: statusLabel[record.status] || record.status,
      note: record.notes || '',
      registeredAt: record.createdAt,
      confirmedAt: record.confirmedAt,
      issuedAt: record.issuedAt || null,
      issuedBy: record.issuedBy || '',
      displayCurrency: 'JPY',
      inputCurrency: 'JPY',
      fxRateScaled: Number(record.fxRateScaled) || 0,
      fxScale: Number(record.fxScale) || 0,
      usdJpyRate: rate,
      totalJpy: items.reduce((sum, item) => sum + item.convertedSalePriceJpy, 0),
      updatedAt: record.updatedAt,
      revisions: (record.revisions || []).map(item => ({revisedAt:new Date(item.revisedAt).toLocaleString('ja-JP'),buyerName:_escHtml(item.actorName || ''),approverName:_escHtml(item.actorName || ''),note:_escHtml(item.reason || '')})),
      apiManaged: true,
      items,
    };
  }

  async function withDetails(path, payload) {
    const items = payload?.items || [];
    return Promise.all(items.map(item => request(`${path}/${encodeURIComponent(item.id)}`).catch(() => item)));
  }

  function enforceWorkspaceRole(user) {
    if (['admin', 'worker'].includes(user?.role)) {
      document.documentElement.dataset.adminAuthenticated = 'true';
      return;
    }
    delete document.documentElement.dataset.adminAuthenticated;
    window.__adminPageAccess = null;
    state.hydrated = false;
    state.dashboardReady = false;
    // Never retain another account's workspace snapshot after a role change.
    if (typeof APP_DATA !== 'undefined') {
      for (const [key, value] of Object.entries(APP_DATA)) {
        if (Array.isArray(value)) APP_DATA[key] = [];
      }
    }
    window.location.replace(user?.role === 'guest'
      ? '/app/admin-reference/guest.html'
      : '/app/admin-reference/index.html');
    throw Object.assign(new Error('ログインしたアカウントの画面へ移動します。'), { code: 'workspace_redirect' });
  }

  async function hydrateAdminOnce() {
    const [me, health] = await Promise.all([request('/auth/me'), request('/health')]);
    state.csrfToken = me.csrfToken;
    state.user = me.user;
    if (typeof setSession === 'function') setSession(sessionPayload(me));
    enforceWorkspaceRole(me.user);
    state.databaseDriver = String(health?.database?.driver || '');

    const masterKeys = ['brands', 'materials', 'movements', 'conditions', 'accessories', 'auctions', 'belt-materials', 'dials', 'product-shapes', 'markings', 'part-names'];
    const masterResults = {};
    const mastersPromise = Promise.all(masterKeys.map(async key => { masterResults[key] = await optional(`/masters/${key}`); }));
    // Dashboard aggregates come from every product and its transaction snapshots;
    // document lines and image metadata are not needed for these numbers.
    const corePromise = Promise.all([
      mastersPromise, fetchAllPages('/products?includeCancelled=true'),
      optional('/partners?includeInactive=true', { sqliteFallback: true }), optional('/purchase-staff'),
      optional('/exchange-rates?limit=100', { sqliteFallback: true }),
    ]);
    const secondaryPromise = Promise.all([
      optional('/parts'), optional('/users?includeInactive=true'), fetchAllPages('/purchases'),
      fetchAllPagesWithFallback('/purchases-deleted'), optional('/market-prices?limit=1000'),
      optional('/boxes', { sqliteFallback: true }), optional('/purchase-requests', { sqliteFallback: true }),
      optional('/notifications?limit=200&unread=true', { sqliteFallback: true }), optional('/approvals', { sqliteFallback: true }),
      optional('/sales?limit=500', { sqliteFallback: true }), optional('/shipments?limit=500', { sqliteFallback: true }),
      optional('/consignments?limit=500', { sqliteFallback: true }), optional('/returns?limit=500', { sqliteFallback: true }),
      optional('/cost-adjustments?limit=500'), optional('/settings', { sqliteFallback: true }),
      optional('/company', { sqliteFallback: true }), optional('/dashboard?months=24', { sqliteFallback: true }),
    ]);
    // The core is awaited first; still observe an early secondary rejection.
    secondaryPromise.catch(() => {});
    const [, products, partners, staff, rates] = await corePromise;
    // The endpoint mixes currencies in timestamp order. Selecting items[0] as
    // USD made a newly updated HKD rate (19.80) appear as USD/JPY.
    const latestRatesByCurrency = {};
    for (const record of rates?.items || []) {
      const code = String(record.baseCurrency || '').toUpperCase();
      if (!['USD', 'HKD', 'EUR'].includes(code) || latestRatesByCurrency[code]) continue;
      const rate = Number(record.rate);
      if (Number.isFinite(rate) && rate > 0) latestRatesByCurrency[code] = rate;
    }
    const latestRate = Number(latestRatesByCurrency.USD) || null;
    state.latestRate = latestRate;
    applyMasters(masterResults);
    applyPartners(partners?.items);
    applyPurchaseStaff(staff?.items);
    applyProducts(products?.items, masterResults, staff?.items, latestRate);
    applyExchangeRates(rates);
    if (!state.hydrated) {
      state.dashboardReady = true;
      showSyncStatus('loading', 'ダッシュボードを表示しました。伝票・画像の詳細を読み込んでいます…');
      document.dispatchEvent(new CustomEvent('zaiko:dashboard-ready'));
    }

    const imagesPromise = Promise.all((products?.items || []).filter(product => Number(product.imageCount) > 0).map(async product => {
      const files = await optional(`/products/${encodeURIComponent(product.id)}/files`);
      return { productId: product.id, files: (files?.items || []).map(file => ({ ...file })) };
    }));

    imagesPromise.catch(() => {});
    const [parts, users, purchases, deletedPurchases, market, boxes, requests, notifications, approvals,
      sales, shipments, consignments, returns, costAdjustments, settings, company, dashboard] = await secondaryPromise;
    applyUsers(users?.items);
    APP_DATA.parts = (parts?.items || []).map(item => ({ ...item, apiManaged: true }));
    const productByLine = new Map(APP_DATA.inventory.map(item => {
      const source = products?.items?.find(product => product.productCode === item.code);
      return [source?.purchaseSlipLineId, item];
    }));
    const [purchaseDetails, saleDetails, shipmentDetails, consignmentDetails, returnDetails, imageResults] = await Promise.all([
      withDetails('/purchases', purchases), withDetails('/sales', sales), withDetails('/shipments', shipments),
      withDetails('/consignments', consignments), withDetails('/returns', returns), imagesPromise,
    ]);
    // Apply only after all reads succeed. A late image request from a failed
    // hydration must never overwrite a restored, previously complete snapshot.
    for (const result of imageResults) {
      const item = APP_DATA.inventory.find(candidate => candidate._id === result.productId);
      if (item) {
        item.imageFiles = result.files;
        item.images = result.files.map(file => file.url);
      }
    }
    const purchaseViews = purchaseDetails.map(record => purchaseView(record, productByLine, latestRate, staff?.items || []));
    const reflectedPurchaseProducts = purchaseViews.reduce((total, slip) => total + (slip.lines || []).reduce(
      (lineTotal, line) => lineTotal + (Number(line.generatedProductCount) || 0), 0), 0);
    const linkedPurchaseProducts = APP_DATA.inventory.filter(item => Boolean(item.purchaseSlipLineId)).length;
    if (reflectedPurchaseProducts !== linkedPurchaseProducts) {
      const error = new Error(`仕入反映点数（${reflectedPurchaseProducts}点）と仕入紐づけ商品（${linkedPurchaseProducts}点）が一致しません。`);
      error.code = 'purchase_inventory_mismatch';
      throw error;
    }
    APP_DATA.purchaseSlips = purchaseViews;
    APP_DATA.deletedPurchaseSlips = (deletedPurchases?.items || []).map(record => ({
      _id: record.id,
      id: record.slipNumber,
      type: '仕入伝票',
      date: record.purchaseDate,
      supplier: record.supplierCode,
      supplierName: record.supplierName,
      staff: (staff?.items || []).find(user => user.staffCode === record.staffCode)?.displayName || record.staffCode,
      deletedAt: record.cancelledAt || record.updatedAt,
      reason: record.cancelReason || '伝票一覧から削除',
      archived: true,
    }));
    APP_DATA.sales = saleDetails.map(saleView);
    APP_DATA.shipments = shipmentDetails.map(shipmentView);
    APP_DATA.consignments = consignmentDetails.map(consignmentView);
    const pendingApprovalTargetIds = new Set((approvals?.items || []).filter(item => item.status === 'pending').map(item => item.targetId));
    const inventoryByCode = new Map(APP_DATA.inventory.map(item => [item.code, item]));
    const returnViews = returnDetails.map(record => {
      const withdrawn = record.operationType === 'purchase_return' && record.status === 'cancelled';
      const sourceSale = findSourceSaleForReturn(record, saleDetails);
      const sourceSaleLines = new Map((sourceSale?.lines || []).map(line => [line.productCode, line]));
      const items = (record.lines || []).map(line => {
        const inventory = inventoryByCode.get(line.productCode) || {};
        const saleLine = sourceSaleLines.get(line.productCode) || {};
        const trackingConfirmed = Boolean(record.trackingConfirmedAt);
        const purchaseReturnStatus = withdrawn ? '取り下げ済み' : trackingConfirmed ? '処理済' : '処理中';
        return { code: line.productCode, brand: line.brand, model: line.modelNumber,
          ref: inventory.ref || '', serial: inventory.serial || '',
          salePrice: Number(saleLine.unitPriceMinor || inventory.salePrice || 0),
          purchaseOriginalPrice: Number(line.costAmountMinor) || 0, purchaseCurrency: line.costCurrency,
          purchasePrice: line.costCurrency === 'USD' ? Math.round(line.costAmountMinor * latestRate) : line.costAmountMinor,
          trackingNo: record.trackingNumber || '',
          trackingConfirmed,
          trackingConfirmedAt: record.trackingConfirmedAt || '',
          status: record.operationType === 'purchase_return'
            ? purchaseReturnStatus
            : (record.status === 'confirmed' ? '処理済' : (pendingApprovalTargetIds.has(record.id) ? '承認待ち' : '未処理')) };
      });
      if (record.adjustmentDetails?.lines?.length) {
        items.push(...record.adjustmentDetails.lines.map((line,index)=>returnAdjustmentView(record.adjustmentDetails,line,index)));
      }
      return {
        _id: record.id, id: record.slipNumber, date: record.transactionDate, buyer: record.buyerCode,
        adjustmentDetails: record.adjustmentDetails || null,
        supplier: record.supplierCode, slipId: record.sourcePurchaseSlipNumber || sourceSale?.slipNumber || '',
        carrier: record.carrier || '', trackingNo: record.trackingNumber || '',
        trackingConfirmed: Boolean(record.trackingConfirmedAt),
        trackingConfirmedAt: record.trackingConfirmedAt || '',
        withdrawn,
        status: withdrawn ? '取り下げ済み' : pendingApprovalTargetIds.has(record.id) ? '承認待ち' : (statusLabel[record.status] || record.status),
        processingStatus: record.operationType === 'purchase_return'
          ? (withdrawn ? '取り下げ済み' : record.trackingConfirmedAt ? '処理済' : '処理中')
          : '',
        reason: record.reason, note: record.notes, createdBy: 'DB連動', createdAt: record.createdAt,
        returnType: record.operationType, total: items.reduce((sum, item) => sum + item.salePrice, 0),
        apiManaged: true, items,
      };
    });
    APP_DATA.purchaseReturns = returnViews.filter(record => record.returnType === 'purchase_return');
    APP_DATA.salesReturns = returnViews.filter(record => record.returnType !== 'purchase_return');
    APP_DATA.costAdjustments = (costAdjustments?.items || []).map(record => ({
      _id: record.id,
      id: record.slipNumber,
      date: record.adjustmentDate,
      adjustmentType: record.adjustmentType,
      sourceProductCode: record.sourceProductCode,
      sourceBrand: record.sourceBrand || '',
      sourceModel: record.sourceModel || '',
      sourceCostJpyMinor: Number(record.sourceCostJpyMinor) || 0,
      adjustedCostJpyMinor: Number(record.adjustedCostJpyMinor) || 0,
      status: record.processingStatus === 'processing' ? '処理中' : '処理済',
      processingStatus: record.processingStatus || 'completed',
      pendingProductCount: Number(record.pendingProductCount) || 0,
      note: record.notes || '',
      confirmedAt: record.confirmedAt || null,
      lines: (record.lines || []).map(line => ({
        position: Number(line.position) || 0,
        kind: line.kind || 'product',
        code: line.managementCode || '',
        previousCostJpyMinor: Number(line.previousCostJpyMinor) || 0,
        adjustmentAmountJpyMinor: Number(line.adjustmentAmountJpyMinor) || 0,
        adjustedCostJpyMinor: Number(line.adjustedCostJpyMinor) || 0,
        allocatedCostJpyMinor: Number(line.allocatedCostJpyMinor) || 0,
        direction: line.direction || '',
        note: line.notes || '',
      })),
      apiManaged: true,
    }));
    APP_DATA.marketPrices = (market?.items || []).map(record => {
      const marketCurrency = String(record.marketCurrency || 'JPY').toUpperCase();
      const storedRate = Number(record.marketFxRateScaled) > 0 && Number(record.marketFxScale) > 0
        ? Number(record.marketFxRateScaled) / Number(record.marketFxScale)
        : (marketCurrency === 'JPY' ? 1 : Number((APP_DATA.fxRates || []).find(rate => rate.code === marketCurrency)?.rate) || latestRate);
      return ({ id: record.id, importDate: record.importDate,
      brand: record.brandName, brandCode: record.brandCode, model: record.modelNumber, ref: record.referenceNumber,
      sku: record.sku || '', supplier: record.supplierCode || '',
      staff: record.staffName || '', material: record.materialCode || '', movement: record.movementCode || '',
      condition: record.conditionCode, warrantyYearMonth: record.warrantyYearMonth || '', purchasePrice: record.purchasePriceMinor, purchaseCurrency: record.purchaseCurrency,
      marketPrice: record.marketPriceMinor,
      marketPriceUsd: marketCurrency === 'USD' ? record.marketPriceMinor : Math.round(record.marketPriceMinor / storedRate),
      marketPriceJpy: marketCurrency === 'JPY' ? record.marketPriceMinor : Math.round(record.marketPriceMinor * storedRate),
      marketCurrency,
      marketFxRate: storedRate,
      marketFxRateScaled: Number(record.marketFxRateScaled) || Math.round(storedRate * 100000000),
      marketFxScale: Number(record.marketFxScale) || 100000000,
      accessories: String(record.accessoryCodes || '').split(',').map(value => value.trim()).filter(Boolean).map(code =>
        masterResults.accessories?.items?.find(item => item.code === code)?.name || code),
      braceletQty: record.braceletQuantity ?? null,
      source: record.source,
      marketCategory: /^(domestic-auction|overseas|domestic-retail)$/i.test(record.source || '')
        ? String(record.source).toLowerCase()
        : 'domestic-auction',
      auctionCode: record.auctionCode || '',
      auctionName: record.auctionName || (!record.source || /^(manual|csv|preview-seed|domestic-auction|overseas|domestic-retail)$/i.test(record.source)
        ? ((APP_DATA.suppliers || []).find(item => item.code === record.supplierCode)?.name || '')
        : record.source),
      note: record.notes, apiManaged: true });
    });
    // The fixed slots exist in the UI even before their first database save.
    APP_DATA.boxes = Array.from({ length: 10 }, (_, index) => {
      const no = index + 1;
      const record = (boxes?.items || []).find(box => Number(String(box.boxCode).replace(/\D/g, '')) === no);
      return record ? { _id: record.id, no, code: record.boxCode, name: record.name,
        publicTo: record.buyerCodes || [], productCodes: record.productCodes || [], productVisibility: record.productVisibility || [],
        createdAt: record.updatedAt, active: record.isActive, apiManaged: true }
        : { no, code: 'BOX-' + String(no).padStart(2, '0'), name: '', publicTo: [], productCodes: [], active: true, apiManaged: true };
    });
    APP_DATA.boxes.forEach(box => (box.productCodes || []).forEach(code => {
      const item = APP_DATA.inventory.find(candidate => candidate.code === code);
      if (item) item.boxNo = box.no;
    }));
    APP_DATA.purchaseRequests = (requests?.items || []).map(mapNotificationPurchaseRequest);
    APP_DATA.approvalRequests = (approvals?.items || []).map(mapNotificationApproval);
    APP_DATA.notifications = (notifications?.items || []).map(mapNotificationRecord);
    if (settings?.items) {
      const value = key => settings.items.find(item => item.key === key)?.value;
      APP_DATA.dashboardSettings = { salesTarget: latestRate ? Number(value('dashboard.sales_target_jpy') || 0) / latestRate : 0,
        purchaseBudget: Number(value('dashboard.purchase_budget_jpy') || 0) };
    }
    if (company) {
      state.company = company;
      const bank = company.bankAccounts?.find(item => item.isPrimary && item.currency === 'JPY') || company.bankAccounts?.[0] || {};
      APP_DATA.companyInfo = { companyName: company.companyName, zip: company.postalCode, address: company.address,
        tel: company.phone, fax: company.fax, email: company.email, invoice: company.invoiceNumber,
        representative: company.representativeName, bankName: bank.bankName || '', branchName: bank.branchName || '',
        accountType: bank.accountType || '', accountNumber: bank.accountNumber || '', accountHolder: bank.accountHolder || '' };
    }
    applyExchangeRates(rates);
    APP_DATA.apiDashboard = dashboard?.dashboard || null;
    state.hydrated = true;
    document.documentElement.dataset.apiConnected = 'true';
    return true;
  }

  function hydrateAdmin() {
    if (hydrateAdminPromise) return hydrateAdminPromise;
    const previousSnapshot = snapshotAppData();
    const recovered = Boolean(state.lastError);
    state.loading = true;
    document.documentElement.dataset.apiLoading = 'true';
    showSyncStatus('loading', state.hydrated ? 'DBの更新情報を確認しています…' : 'DB情報を読み込んでいます…');

    hydrateAdminPromise = hydrateAdminOnce()
      .then(result => {
        if (hydrateRetryTimer) window.clearTimeout(hydrateRetryTimer);
        hydrateRetryTimer = null;
        hydrateRetryStep = 0;
        state.lastError = null;
        state.lastLoadedAt = new Date().toISOString();
        state.retryCount = 0;
        document.documentElement.dataset.apiConnected = 'true';
        if (recovered) showSyncStatus('success', 'DB接続が復旧し、最新情報へ更新しました。');
        else ensureSyncStatus()?.classList.add('hidden');
        document.dispatchEvent(new CustomEvent('zaiko:data-hydrated', {
          detail: { recovered, loadedAt: state.lastLoadedAt },
        }));
        return result;
      })
      .catch(error => {
        if (error?.code === 'workspace_redirect') {
          if (hydrateRetryTimer) window.clearTimeout(hydrateRetryTimer);
          hydrateRetryTimer = null;
          throw error;
        }
        restoreAppData(previousSnapshot);
        state.lastError = { status: error?.status || 0, code: error?.code || '', message: error?.message || String(error) };
        state.retryCount += 1;
        document.documentElement.dataset.apiConnected = 'false';
        showSyncStatus('error', previousSnapshot
          ? 'DBの最新情報を取得できません。直前の正常データを表示し、自動で再取得します。'
          : state.dashboardReady ? 'ダッシュボードは取得済みです。伝票・画像の詳細を再取得しています。'
          : 'DB情報を取得できません。自動で再取得しています。');
        document.dispatchEvent(new CustomEvent('zaiko:data-hydrate-failed', { detail: { error: state.lastError } }));
        scheduleHydrationRetry(error);
        throw error;
      })
      .finally(() => {
        state.loading = false;
        delete document.documentElement.dataset.apiLoading;
        hydrateAdminPromise = null;
      });
    return hydrateAdminPromise;
  }

  window.addEventListener('online', () => {
    if (!state.lastError) return;
    if (hydrateRetryTimer) window.clearTimeout(hydrateRetryTimer);
    hydrateRetryTimer = null;
    hydrateAdmin().catch(() => {});
  });

  async function refreshPurchaseSlip(id) {
    if (hydrateAdminPromise) await hydrateAdminPromise;
    const existing = (APP_DATA.purchaseSlips || []).find(slip => slip.id === id || slip._id === id);
    const record = await request(`/purchases/${encodeURIComponent(existing?._id || id)}`);
    const byLine = new Map((APP_DATA.inventory || []).map(item => [item.purchaseSlipLineId, item]));
    const view = purchaseView(record, byLine, currentRate('USD'),
      (APP_DATA.staffRecords || []).map(staff => ({staffCode:staff.code,displayName:staff.name})));
    const index = (APP_DATA.purchaseSlips || []).findIndex(slip => slip._id === record.id);
    if (index >= 0) APP_DATA.purchaseSlips[index] = view;
    else APP_DATA.purchaseSlips.push(view);
    return view;
  }

  async function hydrateGuest() {
    const me = await request('/auth/me');
    if (me.user.role !== 'guest') throw new Error('ゲストアカウントでログインしてください。');
    state.csrfToken = me.csrfToken;
    state.user = me.user;
    const catalog = await request('/guest/catalog');
    const conditions = new Map();
    const visibleItems = [];
    (catalog.items || []).forEach(item => {
      conditions.set(item.condition, item.condition);
      visibleItems.push({
          _productId: item.productId, code: item.productCode, brand: item.brand, model: item.modelNumber,
          ref: item.referenceNumber, serial: item.serialNumber, salePrice: item.baseSalePriceMinor,
          displayPrice: item.displayPriceMinor, displayCurrency: item.displayCurrency,
          saleCurrency: item.baseSaleCurrency, condition: item.condition, accessories: String(item.accessories || '').split(',').map(v => v.trim()).filter(Boolean),
          status: item.inventoryStatus === 'shipped' ? '出荷中' : statusLabel[item.inventoryStatus], images: Array.isArray(item.images) ? item.images : [],
        });
    });
    const guestCode = me.user.username;
    APP_DATA.guestAccounts = [{ id: guestCode, userId: me.user.id, name: me.user.displayName,
      company: me.user.displayName, buyerCode: 'SELF', active: true, apiManaged: true,
      regionType: catalog.regionType === 'overseas' ? 'overseas' : 'domestic' }];
    APP_DATA.conditions = [...conditions.keys()].map(value => ({ code: value, name: value }));
    APP_DATA.publishedSnapshot = { updatedAt: new Date().toLocaleString('ja-JP'), boxes: [{ publicTo: ['SELF'], items: visibleItems }] };
    state.hydrated = true;
    document.documentElement.dataset.apiConnected = 'true';
    return true;
  }

  async function createGuestRequests(items, note) {
    const created = await request('/guest/purchase-requests', { method: 'POST', body: JSON.stringify({ productIds: items.map(item=>item._productId), message: note || '' }) });
    return [created];
  }

  async function reviewPurchaseRequest(id, decision, note = '', options = {}) {
    const result = await request(`/purchase-requests/${encodeURIComponent(id)}/${encodeURIComponent(decision)}`, {
      method: 'POST', body: JSON.stringify({ note, ...options }),
    });
    await hydrateAdmin();
    return result;
  }

  function mapNotificationPurchaseRequest(record) {
    const company = (APP_DATA.clientCompanies || []).find(candidate => candidate.buyerCode === record.buyerCode);
    return {
      _id:record.id,id:record.requestNumber,batchId:record.batchId,guestId:record.guestCode,guestName:record.buyerName,
      buyerCode:record.buyerCode,clientCompanyCode:company?.id||'',status:statusLabel[record.status]||record.status,
      note:record.message,date:record.requestedAt?new Date(record.requestedAt).toLocaleString('ja-JP'):'',
      createdAt:record.requestedAt,reviewNote:record.reviewNote,shipmentSlipNumber:record.shipmentSlipNumber,
      items:(record.items?.length?record.items:[record]).map(line=>{
        const item=APP_DATA.inventory.find(candidate=>candidate._id===line.productId||candidate.code===line.productCode);
        return {_requestId:line.id,_productId:line.productId,itemCode:line.productCode,
          itemName:[line.brand,line.modelNumber].filter(Boolean).join(' '),brand:line.brand,model:line.modelNumber,
          salePrice:item?.salePrice||0,requestedPriceMinor:line.requestedPriceMinor,requestedCurrency:line.requestedCurrency,itemStatus:line.status};
      }),apiManaged:true,
    };
  }
  function mapNotificationApproval(record) { return ({ id: record.id, buyerId: record.requestedBy,
      buyerName: record.requesterName, type: record.approvalType, typeLabel: record.targetType === 'document_change' ? record.approvalType : approvalActionLabel(record.requestedAction || record.approvalType),
      detail: { targetType: record.targetType, targetId: record.targetId,
        ...(()=>{try{const history=JSON.parse(record.resubmissionJson||'{}');return {previousDocument:history.previousDocument||null,resubmissionReason:history.reason||''};}catch{return {};}})(),
        document: (()=>{try{const value=JSON.parse(record.snapshotJson||'{}');return value&&typeof value==='object'&&!Array.isArray(value)?value:null;}catch{return null;}})(),
        ...(record.targetType === 'stocktake_line' ? (()=>{try{return JSON.parse(record.snapshotJson||'{}');}catch{return {};}})() : {}) }, status: record.status,
      note: record.decisionNote, decisionNote: record.decisionNote, decidedAt: record.decidedAt, deciderName: record.deciderName, createdAt: record.requestedAt, updatedAt:record.updatedAt, apiManaged: true }); }
  function mapNotificationRecord(record) { return ({ id: record.id, toUserId: state.user.id,
      type: record.eventKey, title: record.title, message: record.body, targetType: record.targetType,
      targetId: record.targetId, body: record.body, relatedId: record.targetId,
      read: Boolean(record.readAt), createdAt: record.createdAt, apiManaged: true }); }

  async function refreshNotifications() {
    const result = await request('/notifications?limit=200&unread=true');
    APP_DATA.notifications = (result.items || []).map(mapNotificationRecord);
    return APP_DATA.notifications;
  }

  async function refreshNotificationTarget(kind) {
    if (kind === 'purchase') {
      const result = await request('/purchase-requests');
      APP_DATA.purchaseRequests = (result.items || []).map(mapNotificationPurchaseRequest);
    } else if (kind === 'approval') {
      const result = await request('/approvals');
      APP_DATA.approvalRequests = (result.items || []).map(mapNotificationApproval);
    }
  }

  async function markNotificationRead(id) {
    await request(`/notifications/${encodeURIComponent(id)}/read`, { method: 'POST', body: '{}' });
    const notification = APP_DATA.notifications.find(item => item.id === id);
    if (notification) notification.read = true;
  }

  function resolveUserID(userId, isGuest) {
    if (!isGuest) return userId;
    return APP_DATA.guestAccounts.find(item => item.id === userId)?.userId || '';
  }

  async function changePassword(userId, isGuest, password) {
    const resolved = resolveUserID(userId, isGuest);
    if (!resolved) throw new Error('利用者を特定できませんでした。');
    return request(`/users/${encodeURIComponent(resolved)}/password`, {
      method: 'POST', body: JSON.stringify({ password }),
    });
  }

  async function setUserActive(userId, isGuest, isActive) {
    const resolved = resolveUserID(userId, isGuest);
    if (!resolved) throw new Error('利用者を特定できませんでした。');
    return request(`/users/${encodeURIComponent(resolved)}`, {
      method: 'PATCH', body: JSON.stringify({ isActive }),
    });
  }

  async function queuePasswordReset(userId, isGuest) {
    const resolved = resolveUserID(userId, isGuest);
    if (!resolved) throw new Error('利用者を特定できませんでした。');
    return request(`/users/${encodeURIComponent(resolved)}/password-reset`, { method: 'POST', body: '{}' });
  }

  async function saveCompany() {
    const ci = APP_DATA.companyInfo || {};
    const current = state.company || await request('/company');
    const existingBank = current.bankAccounts?.find(item => item.isPrimary && item.currency === 'JPY') || current.bankAccounts?.[0];
    const hasBankInput = [ci.bankName, ci.branchName, ci.accountNumber, ci.accountHolder].some(Boolean);
    const bankAccounts = hasBankInput ? [{
      id: existingBank?.id || '', bankName: ci.bankName || '', branchName: ci.branchName || '',
      accountType: ci.accountType || '普通', accountNumber: ci.accountNumber || '', accountHolder: ci.accountHolder || '',
      currency: 'JPY', isPrimary: true,
    }] : [];
    const saved = await request('/company', { method: 'PUT', body: JSON.stringify({
      companyName: ci.companyName || '', postalCode: ci.zip || '', address: ci.address || '', phone: ci.tel || '',
      fax: ci.fax || '', email: ci.email || '', invoiceNumber: ci.invoice || '',
      representativeName: ci.representative || '', bankAccounts,
    }) });
    state.company = saved;
    return saved;
  }

  async function saveDashboardSettings(salesTargetUSD, purchaseBudgetJPY) {
    const salesTargetJPY = Math.round(Number(salesTargetUSD || 0) * Number(state.latestRate || 155));
    await Promise.all([
      request('/settings/dashboard.sales_target_jpy', { method: 'PUT', body: JSON.stringify({ value: String(salesTargetJPY) }) }),
      request('/settings/dashboard.purchase_budget_jpy', { method: 'PUT', body: JSON.stringify({ value: String(Math.round(Number(purchaseBudgetJPY || 0))) }) }),
    ]);
  }

  async function getAdminAccessCode() {
    return request('/admin-access-code');
  }

  async function rotateAdminAccessCode() {
    return request('/admin-access-code/rotate', { method: 'POST', body: '{}' });
  }

  async function verifyAdminAccessCode(code) {
    return request('/admin-access-code/verify', {
      method: 'POST', body: JSON.stringify({ code }),
    });
  }

  function applyExchangeRates(rates) {
    APP_DATA.fxRateSync = rates?.sync || { enabled: false };
    APP_DATA.fxRateHistory = (rates?.items || []).map(record => ({
      id: record.id, code: record.baseCurrency, rate: Number(record.rate),
      observedAt: record.observedAt, createdAt: record.createdAt, provider: record.provider || '手動入力',
      publishedDate: record.publishedDate || '', sourceUrl: record.sourceUrl || '',
    }));
    APP_DATA.fxRates = ['USD', 'HKD', 'EUR'].map(code => {
      const latest = APP_DATA.fxRateHistory.find(item => item.code === code);
      return { code, symbol: ({USD:'$',HKD:'HK$',EUR:'€'})[code],
        flag: ({USD:'🇺🇸',HKD:'🇭🇰',EUR:'🇪🇺'})[code], name: ({USD:'USドル',HKD:'香港ドル',EUR:'ユーロ'})[code],
        rate: latest?.rate || null, updatedAt: latest ? new Date(latest.createdAt).toLocaleString('ja-JP') : '',
        updatedBy: latest?.provider || '', publishedDate: latest?.publishedDate || '' };
    });
    state.latestRate = APP_DATA.fxRates.find(item => item.code === 'USD')?.rate || null;
  }

  async function refreshExchangeRates() {
    const rates = await request('/exchange-rates?limit=100');
    applyExchangeRates(rates);
    return rates;
  }

  async function syncExchangeRates() {
    try { return await request('/exchange-rates/sync', {method:'POST',body:'{}'}); }
    finally { await refreshExchangeRates(); await refreshNotifications(); }
  }

  // Refresh rates and the unread badge without overwriting forms or draft entries.
  let ratePollBusy = false;
  async function pollExchangeRates() {
    if (ratePollBusy || !state.hydrated || !APP_DATA.fxRateSync?.enabled || document.hidden) return;
    ratePollBusy = true;
    try {
      await Promise.all([refreshExchangeRates(), refreshNotifications()]);
      if (typeof updateNotifyBadge === 'function') updateNotifyBadge();
      const area = document.getElementById('masterContentArea');
      if (area?.querySelector('#smbc-rate-status') && typeof renderFxRateTab === 'function') renderFxRateTab(area);
    } catch (_) { /* A transient failure leaves the last successfully loaded values intact. */ }
    finally { ratePollBusy = false; }
  }
  window.setInterval(pollExchangeRates, 60000);
  document.addEventListener('visibilitychange', () => { if (!document.hidden) pollExchangeRates(); });

  async function saveExchangeRate(code, rate) {
    const result = await request('/exchange-rates', { method: 'POST', body: JSON.stringify({
      baseCurrency: code, rate: String(rate), provider: '管理画面手入力', observedAt: new Date().toISOString(),
    }) });
    state.latestRate = Number(result.rate || rate);
    await hydrateAdmin();
    return result;
  }

  async function createUser(input) {
    return request('/users', { method: 'POST', body: JSON.stringify(input) });
  }

  async function updateUser(id, input) {
    return request(`/users/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(input) });
  }

  async function saveGuestLogin(input) {
    return request('/guest-logins', { method: 'POST', body: JSON.stringify(input) });
  }

  async function createGuestWithPartner({ company, name, email, buyerCode, password }) {
    return saveGuestLogin({ company, displayName: name || company, email, buyerCode, password, isActive: true });
  }

  function temporaryPassword() {
    const values = new Uint32Array(3);
    crypto.getRandomValues(values);
    return `Zaiko-${[...values].map(value => value.toString(36)).join('').slice(0, 14)}!`;
  }

  async function saveMasterRecord(key, current, values, mode) {
    const masterKinds = { brand: 'brands', material: 'materials', movement: 'movements',
      condition: 'conditions', accessory: 'accessories', auction: 'auctions', belt: 'belt-materials', dial: 'dials', shape: 'product-shapes', marking: 'markings', partName: 'part-names' };
    const kind = masterKinds[key];
    if (kind) {
      const result = mode === 'add'
        ? await request(`/masters/${kind}`, { method: 'POST', body: JSON.stringify({ code: values.code, name: values.name, meaning: values.meaning || '', ...((key === 'belt' || key === 'accessory') ? { inventoryDetail: values.inventoryDetail || '', overseasDetail: values.overseasDetail || '', tagDetail: values.tagDetail || '', domesticDetail: values.domesticDetail || '' } : {}), ...(key === 'brand' ? { reading: values.reading || '' } : {}), ...(key === 'accessory' ? { symbol: values.symbol || '', supplement: values.supplement || '' } : {}) }) })
        : await request(`/masters/${kind}/${encodeURIComponent(current.id)}`, { method: 'PATCH', body: JSON.stringify({ name: values.name, meaning: values.meaning || '', ...((key === 'belt' || key === 'accessory') ? { inventoryDetail: values.inventoryDetail || '', overseasDetail: values.overseasDetail || '', tagDetail: values.tagDetail || '', domesticDetail: values.domesticDetail || '' } : {}), ...(key === 'brand' ? { reading: values.reading || '' } : {}), ...(key === 'accessory' ? { symbol: values.symbol || '', supplement: values.supplement || '' } : {}) }) });
      await hydrateAdmin();
      if (key === 'accessory') {
        const previousName = current?.name || '';
        if (previousName && previousName !== result.name) {
          _renameAccessoryReferences(previousName, result.name);
          APP_DATA.accessoryAliases ||= {};
          Object.keys(APP_DATA.accessoryAliases).forEach(name => {
            if (APP_DATA.accessoryAliases[name] === previousName) APP_DATA.accessoryAliases[name] = result.name;
          });
          APP_DATA.accessoryAliases[previousName] = result.name;
        }
        refreshAccessoryMasterConsumers(previousName, result.name);
      }
      return { record: result };
    }

    if (key === 'supplier' || key === 'buyer') {
      const roleType = key === 'supplier' ? 'supplier' : 'buyer';
      const role = { roleType, roleCode: values.code, isActive: true };
      const payload = {
        legalName: values.name,
        phone: values.contact || '',
        address: values.address || '',
        invoiceNumber: values.invoice || '',
        roles: [role],
      };
      const result = mode === 'add'
        ? await request('/partners', { method: 'POST', body: JSON.stringify(payload) })
        : await request(`/partners/${encodeURIComponent(current._partnerId)}`, { method: 'PATCH', body: JSON.stringify(payload) });
      await hydrateAdmin();
      return { record: result };
    }

    if (key === 'staff') {
      const result = await request(mode === 'add' ? '/purchase-staff' : `/purchase-staff/${encodeURIComponent(current._id)}`, {
        method: mode === 'add' ? 'POST' : 'PATCH', body: JSON.stringify({ staffCode: values.code, displayName: values.name, inventoryDetail: values.inventoryDetail, overseasDetail: values.overseasDetail, tagDetail: values.tagDetail, domesticDetail: values.domesticDetail }),
      });
      await hydrateAdmin();
      return { record: result };
    }

    throw new Error('このマスタ種別はAPI保存に対応していません。');
  }

  async function deactivateMasterRecord(key, current) {
    const masterKinds = { brand: 'brands', material: 'materials', movement: 'movements',
      condition: 'conditions', accessory: 'accessories', auction: 'auctions', belt: 'belt-materials', dial: 'dials', shape: 'product-shapes', marking: 'markings', partName: 'part-names' };
    const kind = masterKinds[key];
    if (kind) {
      await request(`/masters/${kind}/${encodeURIComponent(current.id)}`, {
        method: 'PATCH', body: JSON.stringify({ isActive: false }),
      });
    } else if (key === 'supplier' || key === 'buyer') {
      await request(`/partners/${encodeURIComponent(current._partnerId)}`, {
        method: 'PATCH', body: JSON.stringify({ roles: [{ roleType: key, roleCode: current.code, isActive: false }] }),
      });
    } else if (key === 'staff') {
      await request(`/purchase-staff/${encodeURIComponent(current._id)}`, {
        method: 'PATCH', body: JSON.stringify({ isActive: false }),
      });
    } else {
      throw new Error('このマスタ種別はAPI削除に対応していません。');
    }
    await hydrateAdmin();
  }

  async function createSingleProduct(payload, files = []) {
    const result = await request('/products', { method: 'POST', body: JSON.stringify(payload) });
    for (const file of files.filter(Boolean)) {
      const form = new FormData();
      form.append('file', file, file.name);
      await request(`/products/${encodeURIComponent(result.product.id)}/files`, { method: 'POST', body: form });
    }
    await hydrateAdmin();
    return result;
  }

  /** Existing products keep their current files and receive the newly selected images. */
  async function appendProductImages(item, files = []) {
    if (!item?._id) throw new Error('画像を保存する商品が見つかりません。');
    const additions = files.filter(Boolean);
    if (additions.length === 0) return item;

    const productPath = `/products/${encodeURIComponent(item._id)}/files`;
    const current = await request(productPath);
    if ((current.items || []).length + additions.length > 10) {
      throw new Error(`商品画像は最大10枚です。追加できる画像は${Math.max(0, 10 - (current.items || []).length)}枚です。`);
    }

    for (const file of additions) {
      const form = new FormData();
      form.append('file', file, file.name || 'product-image');
      await request(productPath, { method: 'POST', body: form });
    }
    await hydrateAdmin();
    return APP_DATA.inventory.find(candidate => candidate._id === item._id) || item;
  }

  async function createApproval(input) {
    return request('/approvals', { method: 'POST', body: JSON.stringify(input) });
  }

  async function decideApproval(id, decision, note = '') {
    const result = await request(`/approvals/${encodeURIComponent(id)}/${encodeURIComponent(decision)}`, {
      method: 'POST', body: JSON.stringify({ note }),
    });
    await hydrateAdmin();
    return result;
  }

  async function saveSale(payload, requireApproval) {
    const created = await request('/sales', { method: 'POST', body: JSON.stringify(payload) });
    let record = created;
    let approval = null;
    if (requireApproval) {
      approval = await createApproval({ approvalType: '売上登録', targetType: 'sales_slip',
        targetId: created.id, requestedAction: 'sale.confirm' });
    } else {
      record = await request(`/sales/${encodeURIComponent(created.id)}/confirm`, { method: 'POST', body: '{}' });
    }
    await hydrateAdmin();
    return { record, approval };
  }

  async function saveShipment(payload, requireApproval) {
    const created = await request('/shipments', { method: 'POST', body: JSON.stringify(payload) });
    let record = created;
    let approval = null;
    if (requireApproval) {
      approval = await createApproval({ approvalType: '出荷登録', targetType: 'shipment_slip',
        targetId: created.id, requestedAction: 'shipment.confirm' });
    } else {
      record = await request(`/shipments/${encodeURIComponent(created.id)}/confirm`, { method: 'POST', body: '{}' });
    }
    await hydrateAdmin();
    return { record, approval };
  }

  async function createPart(payload) {
    const result = await request('/parts', { method: 'POST', body: JSON.stringify(payload) });
    await hydrateAdmin();
    return result;
  }

  async function updatePart(part, payload) {
    const partID = part?.id || part?._id;
    if (!partID) throw new Error('編集対象のパーツが見つかりません。');
    const result = await request(`/parts/${encodeURIComponent(partID)}`, {
      method: 'PATCH', body: JSON.stringify(payload),
    });
    await hydrateAdmin();
    return result;
  }

  async function returnShipmentProduct(shipment, code) {
    const shipmentID = shipment?._id || shipment?.id;
    if (!shipmentID) throw new Error('返却対象の出荷伝票が見つかりません。');
    const result = await request(`/shipments/${encodeURIComponent(shipmentID)}/return-scan`, {
      method: 'POST', body: JSON.stringify({ code }),
    });
    await hydrateAdmin();
    return result;
  }

  async function returnConsignmentProduct(consignment, code) {
    const consignmentID = consignment?._id || consignment?.id;
    if (!consignmentID) throw new Error('返却対象の委託伝票が見つかりません。');
    const result = await request(`/consignments/${encodeURIComponent(consignmentID)}/return-scan`, {
      method: 'POST', body: JSON.stringify({ code }),
    });
    await hydrateAdmin();
    return result;
  }

  async function saveConsignment(payload) {
    const record = await request('/consignments', { method: 'POST', body: JSON.stringify(payload) });
    await hydrateAdmin();
    return record;
  }

  async function issueConsignmentSlip(slip) {
    const consignmentID = slip?._id || slip?.id;
    if (!consignmentID) throw new Error('発行対象の委託伝票が見つかりません。');
    const record = await request(`/consignments/${encodeURIComponent(consignmentID)}/issue`, {
      method: 'POST', body: '{}',
    });
    await hydrateAdmin();
    return record;
  }

  async function savePurchaseSlip(slip, requireApproval) {
    const staffCode = APP_DATA.staffRecords?.find(item => item.name === slip.staff)?.code || slip.staff || '';
    const requestedPurchaseCurrency = String(slip.purchaseCurrency || '').toUpperCase();
    const purchaseCurrency = ['JPY', 'USD', 'HKD'].includes(requestedPurchaseCurrency)
      ? requestedPurchaseCurrency : 'JPY';
    const lines = (slip.lines || []).map(line => {
      const detail = line.productDetail || {};
      const resolvedBrandCode = typeof getBrandCodeByName === 'function'
        ? (getBrandCodeByName(detail.brand) || detail.brandCode || '')
        : (detail.brandCode || '');
      const accessoryCodes = (detail.accessories || []).map(name =>
        allAccessoryMasterRecords().find(item => item.name === name)?.code || name).filter(Boolean);
      return {
        quantity: 1,
        sku: line.sku || '',
        brandCode: resolvedBrandCode,
        modelNumber: detail.model || '', referenceNumber: detail.ref || '', serialNumber: detail.serial || '',
        productType: '', shapeCode: detail.shape || '', markingCode: detail.marking || '', materialCode: detail.material || '', movementCode: detail.movement || '',
        conditionCode: detail.condition || '', accessoryCodes, accessorySupplements: detail.accessorySupplements || {},
        weightGrams: detail.weightGrams ?? '',
        beltText: detail.belt || '', dialText: detail.dial || '', braceletQuantity: detail.braceletQty || null,
        unitCostMinor: Math.round(Number(line.purchasePrice) || 0), costCurrency: purchaseCurrency,
        baseSalePriceMinor: Math.round(Number(line.salePrice) || 0), baseSaleCurrency: 'USD',
        notes: detail.note || '',
        boxCode: detail.boxNo ? `BOX-${String(Number(detail.boxNo)).padStart(2, '0')}` : '',
      };
    });
    const created = await request('/purchases', { method: 'POST', body: JSON.stringify({
      auctionFeeRule: slip.auctionFeeRule || null,
      supplierCode: slip.supplier, supplierName: slip.supplierName || '', staffCode, purchaseDate: slip.date,
      purchaseTaxMode: ['personal', 'overseas'].includes(slip.purchaseTaxMode) ? slip.purchaseTaxMode : 'domestic',
      taxCategory: ['tax_equivalent', 'out_of_scope'].includes(slip.taxCategory)
        ? slip.taxCategory : 'consumption_tax',
      paymentMethod: ['cash', 'card'].includes(slip.paymentMethod) ? slip.paymentMethod : 'bank_transfer',
      notes: slip.note || '', lines,
    }) });
    let record = created;
    let approval = null;
    if (requireApproval) {
      approval = await createApproval({ approvalType: '仕入登録', targetType: 'purchase_slip',
        targetId: created.id, requestedAction: 'purchase.confirm' });
    } else {
      record = await request(`/purchases/${encodeURIComponent(created.id)}/confirm`, { method: 'POST', body: '{}' });
    }
    await hydrateAdmin();
    return { record, approval };
  }

  async function issuePurchaseSlip(slip) {
    const purchaseID = slip?._id || slip?.id;
    if (!purchaseID) throw new Error('発行対象の仕入伝票が見つかりません。');
    const record = await request(`/purchases/${encodeURIComponent(purchaseID)}/issue`, {
      method: 'POST', body: '{}',
    });
    await hydrateAdmin();
    return record;
  }

  async function markPurchasePaid(slip) {
    const purchaseID = slip?._id || slip?.id;
    if (!purchaseID) throw new Error('支払確認対象の仕入伝票が見つかりません。');
    const record = await request(`/purchases/${encodeURIComponent(purchaseID)}/paid`, { method:'POST', body:'{}' });
    await hydrateAdmin();
    return record;
  }

  async function receivePurchaseProduct(slip, productCode) {
	const purchaseID = slip?._id || slip?.id;
	if (!purchaseID) throw new Error('入荷対象の仕入伝票が見つかりません。');
	const result = await request(`/purchases/${encodeURIComponent(purchaseID)}/arrival-scan`, {
	  method: 'POST', body: JSON.stringify({ productCode: String(productCode || '').trim() }),
	});
	await hydrateAdmin();
	return result;
  }

  async function scanCostAdjustmentProduct(slip, productCode) {
    const adjustmentID = slip?._id;
    if (!adjustmentID) throw new Error('原価調整伝票が見つかりません。');
    const result = await request(`/cost-adjustments/${encodeURIComponent(adjustmentID)}/scan`, {
      method: 'POST', body: JSON.stringify({ productCode: String(productCode || '').trim() }),
    });
    await hydrateAdmin();
    return result;
  }

  async function deletePurchaseSlip(slip) {
    const purchaseID = slip?._id || slip?.id;
    if (!purchaseID) throw new Error('削除対象の仕入伝票が見つかりません。');
    await request(`/purchases/${encodeURIComponent(purchaseID)}`, { method:'DELETE' });
    await hydrateAdmin();
  }

  async function issueSaleSlip(slip) {
    const saleID = slip?._id || slip?.id;
    if (!saleID) throw new Error('発行対象の売上伝票が見つかりません。');
    const record = await request(`/sales/${encodeURIComponent(saleID)}/issue`, {
      method: 'POST', body: '{}',
    });
    await hydrateAdmin();
    return record;
  }

  async function markSalePaid(slip) {
    const saleID = slip?._id || slip?.id;
    if (!saleID) throw new Error('入金確認対象の売上伝票が見つかりません。');
    const record = await request(`/sales/${encodeURIComponent(saleID)}/paid`, { method:'POST', body:'{}' });
    await hydrateAdmin();
    return record;
  }

  async function setBoxProductVisibility(boxCode,productCode,visible) {
    await request(`/boxes/${encodeURIComponent(boxCode)}/products/${encodeURIComponent(productCode)}/visibility`,{method:'PATCH',body:JSON.stringify({visible})});
    if (hydrateAdminPromise) await hydrateAdminPromise;
    await hydrateAdmin();
  }

  async function saveBoxes(boxes = APP_DATA.boxes || []) {
    if (!boxes.length) throw new Error('保存するBOXがありません。画面を再読み込みしてください。');
    // Check all new assignments before sending any BOX writes.
    boxes.forEach(box => assertBoxAssignableProducts((APP_DATA.inventory || []).filter(item =>
      Number(item.boxNo) === Number(box.no) && !(box.productCodes || []).includes(item.code))));
    // Freeze every request before awaiting: hydration replaces inventory objects.
    const pending = boxes.map(box => ({
      code: box.code || 'BOX-' + String(box.no).padStart(2, '0'),
      body: { name: box.name || 'BOX ' + box.no, isActive: box.active !== false,
        buyerCodes: [...(box.publicTo || [])],
        productCodes: (APP_DATA.inventory || []).filter(item => Number(item.boxNo) === Number(box.no)).map(item => item.code) },
    }));
    for (const { code, body } of pending) {
      await request('/boxes/' + encodeURIComponent(code), { method: 'PUT', body: JSON.stringify(body) });
    }
    // Wait out an earlier refresh so the verification always reads after the writes.
    if (hydrateAdminPromise) await hydrateAdminPromise;
    await hydrateAdmin();
    const sameCodes = (a, b) => JSON.stringify([...(a || [])].sort()) === JSON.stringify([...(b || [])].sort());
    for (const { code, body } of pending) {
      const saved = APP_DATA.boxes.find(box => box.code === code && box._id);
      if (!saved || !sameCodes(saved.productCodes, body.productCodes) || !sameCodes(saved.publicTo, body.buyerCodes)
          || saved.name !== body.name || saved.active !== body.isActive) {
        throw new Error('BOXの保存内容を確認できませんでした。表示内容を確認して、再度操作してください。');
      }
    }
  }

  async function importMarketCSV(file) {
    const form = new FormData();
    const requestedName = String(file?.name || 'market.csv').trim() || 'market.csv';
    const uploadName = /\.csv$/i.test(requestedName) ? requestedName : `${requestedName}.csv`;
    form.append('file', file, uploadName);
    const preview = await request('/market-prices/imports/preview', { method: 'POST', body: form });
    if (Number(preview.errorRows) > 0) {
      const detail = (preview.rows || []).filter(row => !row.valid).slice(0, 3)
        .map(row => `${row.rowNumber}行目: ${row.errorMessage}`).join(' / ');
      throw new Error(`CSVに${preview.errorRows}件のエラーがあります。${detail}`);
    }
    const committed = await request(`/market-prices/imports/${encodeURIComponent(preview.id)}/commit`, {
      method: 'POST', body: '{}',
    });
    await hydrateAdmin();
    return committed;
  }

  async function updateMarketPrice(row, values) {
    const accessoryCodes = (values.accessories || []).map(name =>
      allAccessoryMasterRecords().find(record => record.name === name)?.code || name).filter(Boolean);
    const result = await request(`/market-prices/${encodeURIComponent(row.id)}`, { method: 'PATCH', body: JSON.stringify({
      importDate: values.importDate, brandCode: values.brandCode, modelNumber: values.model,
      referenceNumber: values.ref || '', sku: values.sku || '',
      conditionCode: values.condition || '', warrantyYearMonth: values.warrantyYearMonth || '', purchasePriceMinor: 0,
      purchaseCurrency: 'JPY', marketPriceMinor: Math.round(Number(values.marketPrice) || 0), marketCurrency: values.marketCurrency || 'JPY',
      marketFxRateScaled: Number(values.marketFxRateScaled) || 100000000, marketFxScale: Number(values.marketFxScale) || 100000000,
      supplierCode: '', staffCode: '', materialCode: values.material || '', movementCode: values.movement || '',
      accessoryCodes,
      braceletQuantity: (values.accessories || []).includes('BRACELET PARTS') ? (values.braceletQty ?? null) : null,
      auctionCode: values.auctionCode || '', source: values.marketCategory || row.marketCategory || 'domestic-auction', notes: values.note || '',
    }) });
    await hydrateAdmin();
    return result;
  }

  async function saveReturn(payload, requireApproval) {
    const created = await request('/returns', { method: 'POST', body: JSON.stringify(payload) });
    let record = created;
    let approval = null;
    if (requireApproval) {
      approval = await createApproval({ approvalType: payload.operationType === 'purchase_return' ? '仕入返品' : '返品・持ち帰り', targetType: 'return_slip',
        targetId: created.id, requestedAction: 'return.confirm' });
    } else {
      record = await request(`/returns/${encodeURIComponent(created.id)}/confirm`, { method: 'POST', body: '{}' });
    }
    await hydrateAdmin();
    return { record, approval };
  }

  async function updateShipmentTracking(id, carrier, trackingNumber) {
    const result = await request(`/shipments/${encodeURIComponent(id)}/tracking`, {
      method: 'PATCH', body: JSON.stringify({ carrier: carrier || '', trackingNumber: trackingNumber || '' }),
    });
    await hydrateAdmin();
    return result;
  }

  async function withdrawPurchaseReturn(record) {
    const result = await request('/returns/' + encodeURIComponent(record._id) + '/withdraw', {method:'POST', body:'{}'});
    await hydrateAdmin();
    return result;
  }

  async function revisePurchaseReturn(record, payload) {
    const result = await request(`/returns/${encodeURIComponent(record._id)}/revision`, {
      method: 'PATCH', body: JSON.stringify(payload),
    });
    await hydrateAdmin();
    return result;
  }

  async function updateReturnTracking(id, carrier, trackingNumber, confirmed = false) {
    const result = await request(`/returns/${encodeURIComponent(id)}/tracking`, {
      method: 'PATCH', body: JSON.stringify({ carrier: carrier || '', trackingNumber: trackingNumber || '', confirmed: Boolean(confirmed) }),
    });
    await hydrateAdmin();
    return result;
  }

  async function recordDocumentEvent(event) {
    return request('/document-events', { method: 'POST', body: JSON.stringify(event) });
  }

  async function updateProduct(item, values, reason = '') {
    if (values.boxNo && Number(values.boxNo) !== Number(item.boxNo || 0)) assertBoxAssignableProducts([item]);
    const fields = source => ({
      productCode: source.code || item.code,
      sku: source.sku ?? item.sku ?? '', internalComment: source.comment ?? item.comment ?? '',
      brandCode: APP_DATA.brandRecords?.find(record => record.name === source.brand)?.code || source.brand || '',
      staffCode: APP_DATA.staffRecords?.find(record => record.name === source.staff)?.code || source.staff || '',
      supplierCode: source.supplier || '', purchaseDate: source.purchaseDate || '',
      modelNumber: source.model || '', referenceNumber: source.ref || '', serialNumber: source.serial || '',
      materialCode: source.material || '', movementCode: source.movement || '', conditionCode: source.condition || '',
      shapeCode: source.shape || '', markingCode: source.marking || '',
      weightGrams: String(source.weightGrams ?? item.weightGrams ?? ''),
      costAmountMinor: Math.round(Number(source.purchasePrice) || 0),
      baseSalePriceMinor: Math.round(Number(source.salePrice) || 0),
      accessorySupplements: source.accessorySupplements ?? item.accessorySupplements ?? {},
      accessoryCodes: (source.accessories || []).map(name =>
        allAccessoryMasterRecords().find(record => record.name === name)?.code || name).filter(Boolean).sort(),
      beltText: source.belt || '', dialText: source.dial || '', notes: source.note || '',
      braceletQuantity: (source.accessories || []).includes('BRACELET PARTS') ? Number(source.braceletQty) || 0 : 0,
    });
    const before = fields(item);
    const after = fields(values);
    const changes = Object.fromEntries(Object.entries(after).filter(([key, value]) =>
      JSON.stringify(value) !== JSON.stringify(before[key])));
    // Status belongs to document workflows. Never infer it from a display label.
    // Leave unchanged prices/currencies and master references untouched as well.
    if ('costAmountMinor' in changes) changes.costCurrency = 'JPY';
    if ('baseSalePriceMinor' in changes) changes.baseSaleCurrency = 'USD';
    const result = await request('/products/' + encodeURIComponent(item._id), {
      method: 'PATCH', body: JSON.stringify({ ...changes, duplicateSerialReason: reason, reason }),
    });
    if (Number(values.boxNo || 0) !== Number(item.boxNo || 0)) {
      const previousBox = item.boxNo;
      item.boxNo = values.boxNo || null;
      try {
        await saveBoxes(APP_DATA.boxes || []);
      } catch (error) {
        item.boxNo = previousBox;
        throw error;
      }
    } else {
      await hydrateAdmin();
    }
    return result;
  }

  async function revisePurchaseSlip(record, changes) {
    if (!record?._id) throw new Error('仕入伝票のDB番号が見つかりません。');
    const result = await request(`/purchases/${encodeURIComponent(record._id)}/revision`, {
      method: 'PATCH', body: JSON.stringify(changes),
    });
    await hydrateAdmin();
    return result;
  }

  async function reviseConsignment(record, changes) {
    if (!record?._id) throw new Error('委託伝票のDB番号が見つかりません。');
    const result = await request('/consignments/'+encodeURIComponent(record._id)+'/revision', {method:'PATCH',body:JSON.stringify(changes)});
    await hydrateAdmin(); return result;
  }
  async function reviseShipment(record, changes) {
    if (!record?._id) throw new Error('出荷伝票のDB番号が見つかりません。');
    const result = await request(`/shipments/${encodeURIComponent(record._id)}/revision`, {method:'PATCH',body:JSON.stringify(changes)});
    await hydrateAdmin(); return result;
  }
  async function reviseSaleSlip(record, changes) {
    if (!record?._id) throw new Error('売上伝票のDB番号が見つかりません。');
    const result = await request(`/sales/${encodeURIComponent(record._id)}/revision`, {
      method: 'PATCH', body: JSON.stringify(changes),
    });
    await hydrateAdmin();
    return result;
  }

  async function createSalesAdjustment(record, changes) {
    if (!record?._id) throw new Error('元の売上伝票のDB番号が見つかりません。');
    const result = await request(`/sales/${encodeURIComponent(record._id)}/adjustments`, {
      method: 'POST', body: JSON.stringify(changes),
    });
    await hydrateAdmin();
    return result;
  }

  async function startProductCostAdjustment(item, options = {}) {
    if (!item?._id) throw new Error('原価調整を開始する商品が見つかりません。');
    const mode = ['combine', 'adjustment'].includes(options.mode) ? options.mode : 'breakdown';
    const result = await request(`/products/${encodeURIComponent(item._id)}/cost-adjustments/start`, {
      method: 'POST',
      body: JSON.stringify({ mode, partIds: mode === 'combine' ? (options.partIds || []) : [] }),
    });
    await hydrateAdmin();
    return result;
  }

  async function confirmProductCostAdjustment(item, payload) {
    if (!item?._id) throw new Error('原価調整を確定する対象商品が見つかりません。');
    const result = await request(`/products/${encodeURIComponent(item._id)}/cost-adjustments/confirm`, {
      method: 'POST',
      body: JSON.stringify(payload),
    });
    await hydrateAdmin();
    return result;
  }

  async function resetCostAdjustmentStart(items) {
    const productIds = (items || []).map(item => item?._id).filter(Boolean);
    if (productIds.length === 0) throw new Error('リセットする開始済み商品が見つかりません。');
    const result = await request('/cost-adjustments/reset', {
      method: 'POST', body: JSON.stringify({ productIds }),
    });
    await hydrateAdmin();
    return result;
  }

  async function updateProductImages(item, desiredImages = []) {
    if (!item?._id) throw new Error('画像を保存する商品が見つかりません。');
    const productPath = `/products/${encodeURIComponent(item._id)}/files`;
    const current = await request(productPath);
    const currentIDs = new Set((current.items || []).map(file => file.id));
    const desiredExistingIDs = new Set(desiredImages.filter(image => image.id).map(image => image.id));

    // Full 10-image sets must release removed slots before replacements upload.
    for (const file of current.items || []) {
      if (!desiredExistingIDs.has(file.id)) {
        await request(`/product-files/${encodeURIComponent(file.id)}`, { method: 'DELETE' });
      }
    }

    const orderedIDs = [];
    for (const image of desiredImages) {
      if (image.id && currentIDs.has(image.id)) {
        orderedIDs.push(image.id);
        continue;
      }
      if (!image.file) continue;
      const form = new FormData();
      form.append('file', image.file, image.file.name || image.originalName || 'product-image');
      const uploaded = await request(productPath, { method: 'POST', body: form });
      image.id = uploaded.id;
      orderedIDs.push(uploaded.id);
    }

    await request(`${productPath}/order`, { method: 'PUT', body: JSON.stringify({ fileIds: orderedIDs }) });
    await hydrateAdmin();
    return APP_DATA.inventory.find(candidate => candidate._id === item._id) || null;
  }

  async function savePartner(entry) {
    const roles = (entry.tradeTypes || []).map(roleType => ({
      roleType,
      roleCode: roleType === 'buyer' ? entry.buyerCode : entry.supplierCode,
      isActive: true,
    }));
    const payload = {
      auctionFees: entry.auctionFees || {isAuction:false,purchase:{mode:"none"},sales:{mode:"none"}},
      partnerCode: entry.id, legalName: entry.companyName, representativeName: entry.representative || '',
      email: entry.email || '', phone: entry.tel || '', contactPhone: entry.contactPhone || '', postalCode: entry.postalCode || '', address: entry.address || '',
      invoiceNumber: entry.invoice || '', antiqueLicenseNumber: entry.antiqueLicenseNumber || '', regionType: entry.regionType || 'domestic',
      closingDay: entry.closingDay || null,
      isOther: Boolean(entry.isOther), notes: entry.note || '', roles,
    };
    if (entry._id) {
      delete payload.partnerCode;
      payload.clearClosingDay = !entry.closingDay;
      return request(`/partners/${encodeURIComponent(entry._id)}`, { method: 'PATCH', body: JSON.stringify(payload) });
    }
    return request('/partners', { method: 'POST', body: JSON.stringify(payload) });
  }

  window.ZaikoAPI = { refreshPurchaseSlip, state, currentRate, request, login, logout, hydrateAdmin, hydrateGuest, createGuestRequests, reviewPurchaseRequest,
    markNotificationRead, refreshNotifications, refreshNotificationTarget, saveExchangeRate, refreshExchangeRates, syncExchangeRates,
    changePassword, setUserActive, queuePasswordReset, saveCompany, saveDashboardSettings,
    getAdminAccessCode, rotateAdminAccessCode, verifyAdminAccessCode, createUser, updateUser,
    createGuestWithPartner, saveGuestLogin, savePartner, saveMasterRecord, deactivateMasterRecord, createSingleProduct, createPart, updatePart, appendProductImages,
    createApproval, decideApproval, saveSale, saveShipment, returnShipmentProduct, saveConsignment, returnConsignmentProduct, savePurchaseSlip, issuePurchaseSlip, markPurchasePaid, receivePurchaseProduct, scanCostAdjustmentProduct, deletePurchaseSlip, issueSaleSlip, markSalePaid, issueConsignmentSlip, saveBoxes, setBoxProductVisibility, importMarketCSV, saveReturn,
    updateProduct, startProductCostAdjustment, confirmProductCostAdjustment, resetCostAdjustmentStart, updateProductImages, updateMarketPrice, updateShipmentTracking, updateReturnTracking, withdrawPurchaseReturn, revisePurchaseReturn, revisePurchaseSlip, reviseSaleSlip, reviseShipment, reviseConsignment, createSalesAdjustment, recordDocumentEvent };
})();
