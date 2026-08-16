// REST API bridge for the reference UI. The visual layer stays intact while
// authentication and durable business data are sourced from Go/PostgreSQL.
(function () {
  const state = { csrfToken: '', user: null, hydrated: false, company: null, latestRate: 155 };
  const statusLabel = {
    purchasing: '仕入中', in_stock: '在庫中', reserved: '取置中', return_pending: '仕入返品中', consigned: '委託中', shipped: '出荷済', sold: '売上済', cancelled: '仕入返品済',
    draft: '未処理', confirmed: '処理済', pending: '未対応', approved: '承認済', rejected: '却下', returned: '差戻し',
  };

  async function request(path, options = {}) {
    const headers = { Accept: 'application/json', ...(options.headers || {}) };
    if (options.body && !(options.body instanceof FormData)) headers['Content-Type'] = 'application/json; charset=utf-8';
    if (options.method && options.method !== 'GET' && state.csrfToken) headers['X-CSRF-Token'] = state.csrfToken;
    const response = await fetch(`/api/v1${path}`, { credentials: 'same-origin', ...options, headers });
    if (response.status === 204) return null;
    const payload = await response.json().catch(() => ({}));
    if (!response.ok) {
      const error = new Error(payload.error?.message || `API error (${response.status})`);
      error.status = response.status;
      error.code = payload.error?.code;
      throw error;
    }
    return payload;
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

  async function optional(path) {
    try { return await request(path); } catch (error) {
      if (error.status === 403 || error.status === 404) return null;
      throw error;
    }
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

  function roleItem(partner, role) {
    return {
      _partnerId: partner.id,
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
    assign('accessories', 'accessoryRecords');
    if (results.accessories?.items) APP_DATA.accessories = results.accessories.items.map(item => item.name);
    assign('auctions', 'auctionRecords');
    assign('belt-materials', 'beltMaterialRecords');
    assign('dials', 'dialRecords');
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
      staffCode: user.staffCode,
      isPurchaseStaff: user.isPurchaseStaff,
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
    APP_DATA.staff = users.filter(user => user.staffCode && user.isPurchaseStaff && user.isActive).map(user => user.displayName);
    APP_DATA.staffRecords = users.filter(user => user.staffCode && user.isActive && user.isPurchaseStaff).map(user => ({
      _id: user.id, code: user.staffCode, name: user.displayName, active: user.isActive,
      isPurchaseStaff: user.isPurchaseStaff, apiManaged: true,
    }));
  }

  function applyPurchaseStaff(staff) {
    if (!Array.isArray(staff)) return;
    APP_DATA.staff = staff.map(record => record.displayName);
    APP_DATA.staffRecords = staff.map(record => ({
      _id: record.id, code: record.staffCode, name: record.displayName, active: true,
      isPurchaseStaff: true, apiManaged: true,
    }));
  }

  function masterCode(items, id) {
    return items?.find(item => item.id === id)?.code || '';
  }

  function applyProducts(products, masters, users, rate) {
    if (!Array.isArray(products)) return;
    const staffName = id => users?.find(user => user.staffCode && (user.id === id || `staff_${user.username}` === id))?.displayName || '';
    APP_DATA.inventory = products.map(product => ({
      _id: product.id,
      code: product.productCode,
      sku: product.sku,
      brand: product.brand,
      brandCode: masterCode(masters.brands?.items, product.brandId),
      model: product.modelNumber,
      ref: product.referenceNumber || '',
      serial: product.serialNumber,
      supplier: (product.supplierRoleId || '').replace('partner_role_', ''),
      staff: staffName(product.purchaseStaffProfileId),
      // The JPY acquisition value is the immutable purchase-line snapshot.
      // Do not recalculate it with the latest master rate when hydrating stock.
      purchasePrice: Number(product.fixedPurchaseCostJpyMinor)
        || (product.costCurrency === 'JPY' ? Number(product.costAmountMinor) || 0 : Math.round((Number(product.costAmountMinor) || 0) * rate)),
      purchaseCurrency: product.purchaseSourceCurrency || product.costCurrency,
      purchaseOriginalPrice: Number(product.purchaseSourceAmountMinor) || Number(product.costAmountMinor) || 0,
      purchaseFxRateScaled: Number(product.purchaseFxRateScaled) || 0,
      purchaseFxScale: Number(product.purchaseFxScale) || 0,
      purchaseFxRateObservedAt: product.purchaseFxRateObservedAt || null,
      salePrice: product.baseSaleCurrency === 'JPY' ? Math.round(product.baseSalePriceMinor / rate) : product.baseSalePriceMinor,
      saleCurrency: product.baseSaleCurrency,
      purchaseDate: product.purchaseDate,
      status: statusLabel[product.inventoryStatus] || product.inventoryStatus,
      material: masterCode(masters.materials?.items, product.materialId),
      movement: masterCode(masters.movements?.items, product.movementId),
      condition: masterCode(masters.conditions?.items, product.conditionId) || product.condition,
      accessories: String(product.accessories || '').split(',').map(value => value.trim()).filter(Boolean).map(code =>
        masters.accessories?.items?.find(item => item.code === code)?.name || code),
      belt: product.beltText || '', dial: product.dialText || '', braceletQty: product.braceletQuantity || null,
      images: [],
      imageCount: product.imageCount || 0,
      note: product.notes || '',
      revisions: [],
      apiManaged: true,
    }));
  }

  function purchaseView(record, productByLine, rate, staffDirectory) {
    const staffName = (staffDirectory || []).find(user => user.staffCode === record.staffCode)?.displayName
      || record.staffCode;
    const accessoryNames = values => (Array.isArray(values) ? values : String(values || '').split(','))
      .map(value => String(value || '').trim()).filter(Boolean).map(code =>
        APP_DATA.accessoryRecords?.find(item => item.code === code)?.name || code);
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
      staff: staffName, staffCode: record.staffCode, note: record.notes,
      rawStatus: record.status, status: statusLabel[record.status] || record.status,
      purchaseTaxMode: record.purchaseTaxMode === 'overseas' ? 'overseas' : 'domestic',
      taxRateBasisPoints: record.purchaseTaxMode === 'overseas' ? 0 : 1000,
      registeredAt: record.createdAt, issuedAt: record.issuedAt || null, issuedBy: record.issuedBy || null,
      revisions: [], apiManaged: true,
      lines: (record.lines || []).map(line => {
        // purchaseSlipLineId is the stable DB link. Keep the original line snapshot for audit,
        // while presenting the latest editable product details after the product has been created.
        const currentProduct = productByLine.get(line.id);
        const annotations = productNotes(line.notes);
        return {
          lineNo: line.lineNumber,
          code: currentProduct?.code || '',
          sku: currentProduct ? currentProduct.sku : line.sku,
          quantity: line.quantity,
          generatedProductCount: Number(line.generatedProductCount) || 0,
          // 仕入伝票の金額は起票時通貨の原額を保持する。商品詳細は最新値へ追随するが、
          // 海外仕入を円換算してしまわないよう伝票明細の通貨スナップショットを優先する。
          purchasePrice: line.unitCostMinor,
          purchaseCurrency: line.costCurrency,
          convertedPurchasePriceJpy: Number(line.convertedTotalJpy) || 0,
          purchaseFxRateScaled: Number(line.fxRateScaled) || 0,
          purchaseFxScale: Number(line.fxScale) || 0,
          purchaseFxRateObservedAt: line.fxRateObservedAt || null,
          salePrice: currentProduct
            ? currentProduct.salePrice
            : (line.baseSaleCurrency === 'JPY' ? Math.round(line.baseSalePriceMinor / rate) : line.baseSalePriceMinor),
          saleCurrency: currentProduct?.saleCurrency || line.baseSaleCurrency,
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
            belt: currentProduct ? currentProduct.belt : (line.beltText || ''),
            dial: currentProduct ? currentProduct.dial : (line.dialText || ''),
            braceletQty: currentProduct ? currentProduct.braceletQty : (line.braceletQuantity || null),
            boxNo: currentProduct ? (currentProduct.boxNo ?? null) : annotations.boxNo,
            note: currentProduct ? currentProduct.note : annotations.note,
          },
        };
      }),
    };
  }

  function saleView(record) {
    const rate = Number(record.fxRateScaled) > 0 && Number(record.fxScale) > 0
      ? Number(record.fxRateScaled) / Number(record.fxScale)
      : Number(state.latestRate || 155);
    const inputCurrency = record.displayCurrency === 'JPY' ? 'JPY' : 'USD';
    const toUSD = amount => inputCurrency === 'JPY'
      ? Math.round((Number(amount) || 0) / rate)
      : Number(amount) || 0;
    const sourceMatch = String(record.notes || '').match(/^(出荷|委託)伝票:\s*(\S+)/mu);
    const sourceDocumentType = sourceMatch?.[1] === '出荷'
      ? 'shipment'
      : (sourceMatch?.[1] === '委託' ? 'consignment' : '');
    const sourceDocumentId = sourceMatch?.[2] || '';
    return {
      _id: record.id, id: record.slipNumber, date: record.saleDate, buyer: record.buyerCode,
      status: statusLabel[record.status] || record.status, currency: record.displayCurrency, taxMode: record.taxMode,
      inputCurrency, usdJpyRate: rate,
      fxRateSnapshotId: record.fxRateSnapshotId || '',
      fxRateScaled: Number(record.fxRateScaled) || 0, fxScale: Number(record.fxScale) || 0,
      issuedAt: record.issuedAt || null, issuedBy: record.issuedBy || null,
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
        code: line.productCode, brand: line.brand, model: line.modelNumber,
        ref: line.referenceNumber || '', serial: line.serialNumber || '',
        accessories: String(line.accessories || '').split(',').map(value => value.trim()).filter(Boolean).map(code =>
          APP_DATA.accessoryRecords?.find(item => item.code === code)?.name || code),
        salePrice: line.saleCurrency === 'JPY'
          ? Math.round((Number(line.unitPriceMinor) || 0) / rate)
          : Number(line.unitPriceMinor) || 0,
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
      brand: line.brand,
      model: line.modelNumber,
      salePrice: Number(line.salePriceUsdMinor) || 0,
      salePriceUsd: Number(line.salePriceUsdMinor) || 0,
      convertedSalePriceJpy: Number(line.convertedSalePriceJpy) || 0,
    }));
    return {
      _id: record.id, id: record.slipNumber, date: record.shipmentDate, destination: record.buyerCode,
      buyer: record.buyerCode, status: statusLabel[record.status] || record.status, carrier: record.carrier,
      trackingNo: record.trackingNumber, address: record.recipientAddress, recipient: record.recipientName,
      note: record.notes, apiManaged: true,
      displayCurrency: record.displayCurrency === 'JPY' ? 'JPY' : 'USD',
      inputCurrency: record.displayCurrency === 'JPY' ? 'JPY' : 'USD',
      usdJpyRate: rate,
      total: items.reduce((sum, item) => sum + item.salePriceUsd, 0),
      totalJpy: items.reduce((sum, item) => sum + item.convertedSalePriceJpy, 0),
      items,
    };
  }

  function consignmentView(record) {
    const rate = Number(record.fxRateScaled) > 0 && Number(record.fxScale) > 0
      ? Number(record.fxRateScaled) / Number(record.fxScale) : 0;
    const items = (record.lines || []).map(line => ({
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
      consigneeName: record.consigneeName,
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
      revisions: [],
      apiManaged: true,
      items,
    };
  }

  async function withDetails(path, payload) {
    const items = payload?.items || [];
    return Promise.all(items.map(item => request(`${path}/${encodeURIComponent(item.id)}`).catch(() => item)));
  }

  async function hydrateAdmin() {
    const me = await request('/auth/me');
    state.csrfToken = me.csrfToken;
    state.user = me.user;
    if (typeof setSession === 'function') setSession(sessionPayload(me));

    const masterKeys = ['brands', 'materials', 'movements', 'conditions', 'accessories', 'auctions', 'belt-materials', 'dials'];
    const masterResults = {};
    await Promise.all(masterKeys.map(async key => { masterResults[key] = await optional(`/masters/${key}`); }));
    const [products, partners, users, staff, purchases, market, boxes, requests, notifications, approvals, sales, shipments, consignments, returns, settings, company, rates, dashboard] = await Promise.all([
      fetchAllPages('/products?includeCancelled=true'), optional('/partners?includeInactive=true'),
      optional('/users?includeInactive=true'), optional('/purchase-staff'), fetchAllPages('/purchases'), optional('/market-prices?limit=1000'),
      optional('/boxes'), optional('/purchase-requests'), optional('/notifications?limit=500'), optional('/approvals'),
      optional('/sales?limit=500'), optional('/shipments?limit=500'), optional('/consignments?limit=500'), optional('/returns?limit=500'),
      optional('/settings'), optional('/company'), optional('/exchange-rates?limit=100'), optional('/dashboard?months=24'),
    ]);
    const latestRate = Number(rates?.items?.[0]?.rate || 155);
    state.latestRate = latestRate;
    applyMasters(masterResults);
    applyPartners(partners?.items);
    applyUsers(users?.items);
    applyPurchaseStaff(staff?.items);
    applyProducts(products?.items, masterResults, users?.items || staff?.items, latestRate);

    await Promise.all((products?.items || []).filter(product => Number(product.imageCount) > 0).map(async product => {
      const files = await optional(`/products/${encodeURIComponent(product.id)}/files`);
      const item = APP_DATA.inventory.find(candidate => candidate._id === product.id);
      if (item) {
        item.imageFiles = (files?.items || []).map(file => ({ ...file }));
        item.images = item.imageFiles.map(file => file.url);
      }
    }));

    const productByLine = new Map(APP_DATA.inventory.map(item => {
      const source = products?.items?.find(product => product.productCode === item.code);
      return [source?.purchaseSlipLineId, item];
    }));
    const purchaseDetails = await withDetails('/purchases', purchases);
    const saleDetails = await withDetails('/sales', sales);
    const shipmentDetails = await withDetails('/shipments', shipments);
    const consignmentDetails = await withDetails('/consignments', consignments);
    const returnDetails = await withDetails('/returns', returns);
    const purchaseViews = purchaseDetails.map(record => purchaseView(record, productByLine, latestRate, users?.items || staff?.items || []));
    const reflectedPurchaseProducts = purchaseViews.reduce((total, slip) => total + (slip.lines || []).reduce(
      (lineTotal, line) => lineTotal + (Number(line.generatedProductCount) || 0), 0), 0);
    if (reflectedPurchaseProducts !== APP_DATA.inventory.length) {
      const error = new Error(`仕入反映点数（${reflectedPurchaseProducts}点）と在庫全件（${APP_DATA.inventory.length}点）が一致しません。`);
      error.code = 'purchase_inventory_mismatch';
      throw error;
    }
    APP_DATA.purchaseSlips = purchaseViews;
    APP_DATA.sales = saleDetails.map(saleView);
    APP_DATA.shipments = shipmentDetails.map(shipmentView);
    APP_DATA.consignments = consignmentDetails.map(consignmentView);
    const pendingApprovalTargetIds = new Set((approvals?.items || []).filter(item => item.status === 'pending').map(item => item.targetId));
    const inventoryByCode = new Map(APP_DATA.inventory.map(item => [item.code, item]));
    const returnViews = returnDetails.map(record => {
      const productCodes = new Set((record.lines || []).map(line => line.productCode));
      const sourceSale = saleDetails.find(sale => productCodes.size > 0
        && [...productCodes].every(code => (sale.lines || []).some(line => line.productCode === code)));
      const sourceSaleLines = new Map((sourceSale?.lines || []).map(line => [line.productCode, line]));
      const items = (record.lines || []).map(line => {
        const inventory = inventoryByCode.get(line.productCode) || {};
        const saleLine = sourceSaleLines.get(line.productCode) || {};
        const purchaseReturnStatus = record.trackingNumber ? '処理済' : '処理中';
        return { code: line.productCode, brand: line.brand, model: line.modelNumber,
          ref: inventory.ref || '', serial: inventory.serial || '',
          salePrice: Number(saleLine.unitPriceMinor || inventory.salePrice || 0),
          purchasePrice: line.costCurrency === 'USD' ? Math.round(line.costAmountMinor * latestRate) : line.costAmountMinor,
          trackingNo: record.trackingNumber || '',
          status: record.operationType === 'purchase_return'
            ? purchaseReturnStatus
            : (record.status === 'confirmed' ? '処理済' : (pendingApprovalTargetIds.has(record.id) ? '承認待ち' : '未処理')) };
      });
      return {
        _id: record.id, id: record.slipNumber, date: record.transactionDate, buyer: record.buyerCode,
        supplier: record.supplierCode, slipId: record.sourcePurchaseSlipNumber || sourceSale?.slipNumber || '',
        carrier: record.carrier || '', trackingNo: record.trackingNumber || '',
        status: pendingApprovalTargetIds.has(record.id) ? '承認待ち' : (statusLabel[record.status] || record.status),
        processingStatus: record.operationType === 'purchase_return'
          ? (record.trackingNumber ? '処理済' : '処理中')
          : '',
        reason: record.reason, note: record.notes, createdBy: 'DB連動', createdAt: record.createdAt,
        returnType: record.operationType, total: items.reduce((sum, item) => sum + item.salePrice, 0),
        apiManaged: true, items,
      };
    });
    APP_DATA.purchaseReturns = returnViews.filter(record => record.returnType === 'purchase_return');
    APP_DATA.salesReturns = returnViews.filter(record => record.returnType !== 'purchase_return');
    APP_DATA.marketPrices = (market?.items || []).map(record => ({ id: record.id, importDate: record.importDate,
      brand: record.brandName, brandCode: record.brandCode, model: record.modelNumber, ref: record.referenceNumber,
      sku: record.sku || '', supplier: record.supplierCode || '',
      staff: record.staffName || '', material: record.materialCode || '', movement: record.movementCode || '',
      condition: record.conditionCode, purchasePrice: record.purchasePriceMinor, purchaseCurrency: record.purchaseCurrency,
      marketPrice: record.marketPriceMinor,
      marketPriceUsd: record.marketCurrency === 'JPY' ? Math.round(record.marketPriceMinor / latestRate) : record.marketPriceMinor,
      marketPriceJpy: record.marketCurrency === 'USD' ? Math.round(record.marketPriceMinor * latestRate) : record.marketPriceMinor,
      marketCurrency: record.marketCurrency,
      accessories: String(record.accessoryCodes || '').split(',').map(value => value.trim()).filter(Boolean).map(code =>
        masterResults.accessories?.items?.find(item => item.code === code)?.name || code),
      source: record.source,
      auctionCode: record.auctionCode || '',
      auctionName: record.auctionName || (!record.source || /^(manual|csv|preview-seed)$/i.test(record.source)
        ? ((APP_DATA.suppliers || []).find(item => item.code === record.supplierCode)?.name || '')
        : record.source),
      note: record.notes, apiManaged: true }));
    APP_DATA.boxes = (boxes?.items || []).map(record => ({ _id: record.id, no: Number(String(record.boxCode).replace(/\D/g, '')),
      code: record.boxCode, name: record.name, publicTo: record.buyerCodes || [], productCodes: record.productCodes || [],
      createdAt: record.updatedAt, active: record.isActive, apiManaged: true }));
    APP_DATA.boxes.forEach(box => (box.productCodes || []).forEach(code => {
      const item = APP_DATA.inventory.find(candidate => candidate.code === code);
      if (item) item.boxNo = box.no;
    }));
    APP_DATA.purchaseRequests = (requests?.items || []).map(record => {
      const item = APP_DATA.inventory.find(candidate => candidate._id === record.productId || candidate.code === record.productCode);
      const company = (APP_DATA.clientCompanies || []).find(candidate => candidate.buyerCode === record.buyerCode);
      return {
        _id: record.id, id: record.requestNumber, guestId: record.guestCode, guestName: record.buyerName,
        buyerCode: record.buyerCode, clientCompanyCode: company?.id || '', status: statusLabel[record.status] || record.status,
        note: record.message, date: record.requestedAt ? new Date(record.requestedAt).toLocaleString('ja-JP') : '',
        createdAt: record.requestedAt, reviewNote: record.reviewNote,
        items: [{ _productId: record.productId, itemCode: record.productCode,
          itemName: [record.brand, record.modelNumber].filter(Boolean).join(' '), brand: record.brand,
          model: record.modelNumber, salePrice: item?.salePrice || 0, itemStatus: record.status }], apiManaged: true,
      };
    });
    APP_DATA.approvalRequests = (approvals?.items || []).map(record => ({ id: record.id, buyerId: record.requestedBy,
      buyerName: record.requesterName, type: record.approvalType, typeLabel: record.requestedAction,
      detail: { targetType: record.targetType, targetId: record.targetId }, status: record.status,
      note: record.decisionNote, createdAt: record.requestedAt, apiManaged: true }));
    APP_DATA.notifications = (notifications?.items || []).map(record => ({ id: record.id, toUserId: state.user.id,
      type: record.eventKey, title: record.title, message: record.body, targetType: record.targetType,
      targetId: record.targetId, body: record.body, relatedId: record.targetId,
      read: Boolean(record.readAt), createdAt: record.createdAt, apiManaged: true }));
    if (settings?.items) {
      const value = key => settings.items.find(item => item.key === key)?.value;
      APP_DATA.dashboardSettings = { salesTarget: Number(value('dashboard.sales_target_jpy') || 0) / latestRate,
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
    APP_DATA.fxRates = [{ code: 'USD', name: 'USドル', rate: latestRate }, ...(APP_DATA.fxRates || []).filter(item => item.code !== 'USD')];
    APP_DATA.apiDashboard = dashboard?.dashboard || null;
    state.hydrated = true;
    document.documentElement.dataset.apiConnected = 'true';
    return true;
  }

  async function hydrateGuest() {
    const me = await request('/auth/me');
    if (me.user.role !== 'guest') throw new Error('ゲストアカウントでログインしてください。');
    state.csrfToken = me.csrfToken;
    state.user = me.user;
    const catalog = await request('/guest/catalog');
    const conditions = new Map();
    const boxes = new Map();
    (catalog.items || []).forEach(item => {
      conditions.set(item.condition, item.condition);
      String(item.boxCodes || '').split(',').map(value => value.trim()).filter(Boolean).forEach(code => {
        if (!boxes.has(code)) boxes.set(code, []);
        boxes.get(code).push({
          _productId: item.productId, code: item.productCode, brand: item.brand, model: item.modelNumber,
          ref: item.referenceNumber, serial: item.serialNumber, salePrice: item.baseSalePriceMinor,
          saleCurrency: item.baseSaleCurrency, condition: item.condition, accessories: String(item.accessories || '').split(',').map(v => v.trim()).filter(Boolean),
          status: (item.inventoryStatus === 'in_stock' || item.reservedByMe) ? '在庫中' : statusLabel[item.inventoryStatus], images: [],
        });
      });
    });
    const guestCode = me.user.username;
    APP_DATA.guestAccounts = [{ id: guestCode, userId: me.user.id, name: me.user.displayName,
      company: me.user.displayName, buyerCode: 'SELF', active: true, apiManaged: true }];
    APP_DATA.conditions = [...conditions.keys()].map(value => ({ code: value, name: value }));
    APP_DATA.publishedSnapshot = { updatedAt: new Date().toLocaleString('ja-JP'), boxes: [...boxes.entries()].map(([code, items]) => ({
      no: Number(code.replace(/\D/g, '')), name: code, publicTo: ['SELF'], items,
    })) };
    state.hydrated = true;
    document.documentElement.dataset.apiConnected = 'true';
    return true;
  }

  async function createGuestRequests(items, note) {
    const created = [];
    for (const item of items) {
      created.push(await request('/guest/purchase-requests', { method: 'POST', body: JSON.stringify({ productId: item._productId, message: note || '' }) }));
    }
    return created;
  }

  async function reviewPurchaseRequest(id, decision, note = '') {
    const result = await request(`/purchase-requests/${encodeURIComponent(id)}/${encodeURIComponent(decision)}`, {
      method: 'POST', body: JSON.stringify({ note }),
    });
    await hydrateAdmin();
    return result;
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

  async function saveExchangeRate(code, rate) {
    if (code !== 'USD') throw new Error('現在DBで使用する基準通貨はUSD/JPYです。');
    const result = await request('/exchange-rates', { method: 'POST', body: JSON.stringify({
      rate: String(rate), provider: '管理画面手入力', observedAt: new Date().toISOString(),
    }) });
    state.latestRate = Number(result.rate || rate);
    await hydrateAdmin();
    return result;
  }

  async function createUser(input) {
    return request('/users', { method: 'POST', body: JSON.stringify(input) });
  }

  async function createGuestWithPartner({ company, name, email, buyerCode, password }) {
    let resolvedBuyerCode = buyerCode;
    if (!resolvedBuyerCode) {
      const partner = await request('/partners', { method: 'POST', body: JSON.stringify({
        legalName: company, email, roles: [{ roleType: 'buyer', roleCode: '', isActive: true }],
      }) });
      resolvedBuyerCode = partner.roles?.find(role => role.roleType === 'buyer')?.roleCode || '';
    }
    return createUser({ username: `guest-${Date.now()}`, password, displayName: name || company,
      email, role: 'guest', buyerCode: resolvedBuyerCode });
  }

  function temporaryPassword() {
    const values = new Uint32Array(3);
    crypto.getRandomValues(values);
    return `Zaiko-${[...values].map(value => value.toString(36)).join('').slice(0, 14)}!`;
  }

  async function saveMasterRecord(key, current, values, mode) {
    const masterKinds = { brand: 'brands', material: 'materials', movement: 'movements',
      condition: 'conditions', accessory: 'accessories', auction: 'auctions', belt: 'belt-materials', dial: 'dials' };
    const kind = masterKinds[key];
    if (kind) {
      const result = mode === 'add'
        ? await request(`/masters/${kind}`, { method: 'POST', body: JSON.stringify({ code: values.code, name: values.name }) })
        : await request(`/masters/${kind}/${encodeURIComponent(current.id)}`, { method: 'PATCH', body: JSON.stringify({ name: values.name }) });
      await hydrateAdmin();
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
      if (mode === 'edit') {
        const result = await request(`/users/${encodeURIComponent(current._id)}`, {
          method: 'PATCH', body: JSON.stringify({ displayName: values.name, isPurchaseStaff: true, isActive: true }),
        });
        await hydrateAdmin();
        return { record: result };
      }
      const password = temporaryPassword();
      const username = values.code.toLowerCase().replace(/[^a-z0-9]+/g, '-');
      const result = await createUser({ username, password, displayName: values.name, email: '', role: 'worker',
        staffCode: values.code, isPurchaseStaff: true });
      await hydrateAdmin();
      return { record: result, temporaryPassword: password, username };
    }

    throw new Error('このマスタ種別はAPI保存に対応していません。');
  }

  async function deactivateMasterRecord(key, current) {
    const masterKinds = { brand: 'brands', material: 'materials', movement: 'movements',
      condition: 'conditions', accessory: 'accessories', auction: 'auctions', belt: 'belt-materials', dial: 'dials' };
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
      await request(`/users/${encodeURIComponent(current._id)}`, {
        method: 'PATCH', body: JSON.stringify({ isPurchaseStaff: false, isActive: false }),
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
    const purchaseCurrency = slip.purchaseTaxMode === 'overseas' ? 'USD' : 'JPY';
    const lines = (slip.lines || []).map(line => {
      const detail = line.productDetail || {};
      const resolvedBrandCode = typeof getBrandCodeByName === 'function'
        ? (getBrandCodeByName(detail.brand) || detail.brandCode || '')
        : (detail.brandCode || '');
      const accessoryCodes = (detail.accessories || []).map(name =>
        APP_DATA.accessoryRecords?.find(item => item.name === name)?.code || name).filter(Boolean);
      return {
        quantity: 1,
        sku: line.sku || '',
        brandCode: resolvedBrandCode,
        modelNumber: detail.model || '', referenceNumber: detail.ref || '', serialNumber: detail.serial || '',
        productType: 'watch', materialCode: detail.material || '', movementCode: detail.movement || '',
        conditionCode: detail.condition || '', accessoryCodes,
        beltText: detail.belt || '', dialText: detail.dial || '', braceletQuantity: detail.braceletQty || null,
        unitCostMinor: Math.round(Number(line.purchasePrice) || 0), costCurrency: purchaseCurrency,
        baseSalePriceMinor: Math.round(Number(line.salePrice) || 0), baseSaleCurrency: 'USD',
        notes: [detail.note || '', detail.boxNo ? `BOX:${detail.boxNo}` : ''].filter(Boolean).join(' / '),
      };
    });
    const created = await request('/purchases', { method: 'POST', body: JSON.stringify({
      supplierCode: slip.supplier, staffCode, purchaseDate: slip.date,
      purchaseTaxMode: slip.purchaseTaxMode === 'overseas' ? 'overseas' : 'domestic',
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

  async function issueSaleSlip(slip) {
    const saleID = slip?._id || slip?.id;
    if (!saleID) throw new Error('発行対象の売上伝票が見つかりません。');
    const record = await request(`/sales/${encodeURIComponent(saleID)}/issue`, {
      method: 'POST', body: '{}',
    });
    await hydrateAdmin();
    return record;
  }

  async function saveBoxes(boxes = APP_DATA.boxes || []) {
    for (const box of boxes) {
      const code = box.code || `BOX${String(box.no).padStart(2, '0')}`;
      const productCodes = (APP_DATA.inventory || []).filter(item => Number(item.boxNo) === Number(box.no))
        .map(item => item.code);
      await request(`/boxes/${encodeURIComponent(code)}`, { method: 'PUT', body: JSON.stringify({
        name: box.name || code, isActive: box.active !== false, buyerCodes: box.publicTo || [], productCodes,
      }) });
    }
    await hydrateAdmin();
  }

  async function importMarketCSV(file) {
    const form = new FormData();
    form.append('file', file, file.name || 'market.csv');
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
      APP_DATA.accessoryRecords?.find(record => record.name === name)?.code || name).filter(Boolean);
    const result = await request(`/market-prices/${encodeURIComponent(row.id)}`, { method: 'PATCH', body: JSON.stringify({
      importDate: values.importDate, brandCode: values.brandCode, modelNumber: values.model,
      referenceNumber: values.ref || '', sku: values.sku || '',
      conditionCode: values.condition || '', purchasePriceMinor: 0,
      purchaseCurrency: 'JPY', marketPriceMinor: Math.round(Number(values.marketPriceJpy) || 0), marketCurrency: 'JPY',
      supplierCode: '', staffCode: '', materialCode: values.material || '', movementCode: values.movement || '',
      accessoryCodes,
      auctionCode: values.auctionCode || '', source: values.auctionCode || 'manual', notes: values.note || '',
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

  async function updateReturnTracking(id, carrier, trackingNumber) {
    const result = await request(`/returns/${encodeURIComponent(id)}/tracking`, {
      method: 'PATCH', body: JSON.stringify({ carrier: carrier || '', trackingNumber: trackingNumber || '' }),
    });
    await hydrateAdmin();
    return result;
  }

  async function recordDocumentEvent(event) {
    return request('/document-events', { method: 'POST', body: JSON.stringify(event) });
  }

  async function updateProduct(item, values, reason = '') {
    const brandCode = APP_DATA.brandRecords?.find(record => record.name === values.brand)?.code || values.brand || '';
    const staffCode = APP_DATA.staffRecords?.find(record => record.name === values.staff)?.code || values.staff || '';
    const accessoryCodes = (values.accessories || []).map(name =>
      APP_DATA.accessoryRecords?.find(record => record.name === name)?.code || name).filter(Boolean);
    const statusCode = Object.entries(statusLabel).find(([, label]) => label === values.status)?.[0] || values.status;
    const result = await request(`/products/${encodeURIComponent(item._id)}`, { method: 'PATCH', body: JSON.stringify({
      brandCode, modelNumber: values.model || '', referenceNumber: values.ref || '', serialNumber: values.serial || '',
      materialCode: values.material || '', movementCode: values.movement || '', conditionCode: values.condition || '',
      supplierCode: values.supplier || '', staffCode, purchaseDate: values.purchaseDate,
      costAmountMinor: Math.round(Number(values.purchasePrice) || 0), costCurrency: 'JPY',
      baseSalePriceMinor: Math.round(Number(values.salePrice) || 0), baseSaleCurrency: 'USD',
      accessoryCodes, beltText: values.belt || '', dialText: values.dial || '', notes: values.note || '',
      inventoryStatus: statusCode, duplicateSerialReason: reason, reason,
    }) });
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

  window.ZaikoAPI = { state, request, login, logout, hydrateAdmin, hydrateGuest, createGuestRequests, reviewPurchaseRequest,
    markNotificationRead, saveExchangeRate,
    changePassword, setUserActive, queuePasswordReset, saveCompany, saveDashboardSettings,
    getAdminAccessCode, rotateAdminAccessCode, verifyAdminAccessCode, createUser,
    createGuestWithPartner, savePartner, saveMasterRecord, deactivateMasterRecord, createSingleProduct, appendProductImages,
    createApproval, decideApproval, saveSale, saveShipment, saveConsignment, savePurchaseSlip, issuePurchaseSlip, issueSaleSlip, issueConsignmentSlip, saveBoxes, importMarketCSV, saveReturn,
    updateProduct, updateProductImages, updateMarketPrice, updateShipmentTracking, updateReturnTracking, recordDocumentEvent };
})();
