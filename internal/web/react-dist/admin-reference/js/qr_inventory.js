/**
 * 棚卸用QRコード
 *
 * QRには価格や顧客情報を含めず、固定の管理番号だけを格納する。
 * 画像をDBへ保存せず、管理番号から必要な時に同じQRを再生成する。
 */

'use strict';

let _inventoryQrCurrentCode = '';

function _inventoryQrEscape(value) {
  return String(value ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}

/** 棚卸QRへ格納する値。読み取り後、そのまま管理番号入力欄へ入る。 */
function getInventoryQrPayload(itemOrCode) {
  const code = typeof itemOrCode === 'object' ? itemOrCode?.code : itemOrCode;
  return String(code || '').trim();
}

function createInventoryQrSvg(itemOrCode, options = {}) {
  const payload = getInventoryQrPayload(itemOrCode);
  if (!payload) throw new Error('管理番号がありません');
  if (typeof qrcode !== 'function') throw new Error('QR生成機能を読み込めませんでした');

  const qr = qrcode(0, options.errorCorrectionLevel || 'M');
  qr.addData(payload, 'Byte');
  qr.make();

  const safeId = payload.replace(/[^A-Za-z0-9_-]/g, '-');
  return qr.createSvgTag({
    cellSize: options.cellSize || 6,
    margin: options.margin ?? 18,
    scalable: true,
    title: {
      text: `棚卸用QR 管理番号 ${payload}`,
      id: `inventory-qr-title-${safeId}`,
    },
  });
}

function _ensureInventoryQrModal() {
  let modal = document.getElementById('inventoryQrModal');
  if (modal) return modal;

  document.body.insertAdjacentHTML('beforeend', `
    <div class="modal-overlay hidden inventory-qr-overlay" id="inventoryQrModal" style="z-index:1250;"
         role="dialog" aria-modal="true" aria-labelledby="inventoryQrModalTitle">
      <div class="modal-box inventory-qr-modal-box">
        <div class="modal-header">
          <h3 id="inventoryQrModalTitle"><i class="fa-solid fa-qrcode"></i> 棚卸用QRコード</h3>
          <button type="button" class="modal-close" data-qr-close onclick="closeInventoryQr()" aria-label="閉じる">
            <i class="fa-solid fa-xmark"></i>
          </button>
        </div>
        <div class="modal-body inventory-qr-modal-body">
          <div class="inventory-qr-preview" id="inventoryQrPreview" role="img" aria-label="棚卸用QRコード"></div>
          <div class="inventory-qr-product">
            <span class="inventory-qr-caption">管理番号</span>
            <strong id="inventoryQrCode"></strong>
            <span id="inventoryQrProductName"></span>
            <small id="inventoryQrProductMeta"></small>
          </div>
          <div class="inventory-qr-note">
            <i class="fa-solid fa-circle-info"></i>
            QRには管理番号だけが入っています。棚卸画面のQRボタンから読み取ってください。
          </div>
        </div>
        <div class="modal-footer inventory-qr-modal-footer">
          <button type="button" class="btn btn-outline" onclick="downloadInventoryQr()">
            <i class="fa-solid fa-download"></i> QRをダウンロード
          </button>
          <button type="button" class="btn btn-primary" onclick="printInventoryQrLabel()">
            <i class="fa-solid fa-print"></i> ラベル印刷
          </button>
        </div>
      </div>
    </div>`);

  modal = document.getElementById('inventoryQrModal');
  modal.addEventListener('click', event => {
    if (event.target === modal) closeInventoryQr();
  });
  return modal;
}

function openInventoryQr(code) {
  const item = (APP_DATA.inventory || []).find(candidate => candidate.code === code);
  if (!item) {
    if (typeof showToast === 'function') showToast('error', 'QRコード', '対象商品が在庫一覧に見つかりません');
    return;
  }

  const modal = _ensureInventoryQrModal();
  _inventoryQrCurrentCode = item.code;
  try {
    document.getElementById('inventoryQrPreview').innerHTML = createInventoryQrSvg(item);
  } catch (error) {
    if (typeof showToast === 'function') showToast('error', 'QRコードを生成できません', error.message);
    return;
  }

  document.getElementById('inventoryQrCode').textContent = item.code;
  document.getElementById('inventoryQrProductName').textContent = [item.brand, item.model].filter(Boolean).join(' / ') || '商品名未設定';
  document.getElementById('inventoryQrProductMeta').textContent = `型番: ${item.ref || '—'} / シリアル: ${item.serial || '—'}`;
  document.getElementById('inventoryQrPreview').setAttribute('aria-label', `管理番号 ${item.code} の棚卸用QRコード`);
  modal.classList.remove('hidden');
  modal.querySelector('[data-qr-close]')?.focus();
}

function closeInventoryQr() {
  document.getElementById('inventoryQrModal')?.remove();
  _inventoryQrCurrentCode = '';
}

function downloadInventoryQr() {
  const item = (APP_DATA.inventory || []).find(candidate => candidate.code === _inventoryQrCurrentCode);
  if (!item) return;

  const svg = createInventoryQrSvg(item, { cellSize: 10, margin: 30 });
  const blob = new Blob([svg], { type: 'image/svg+xml;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = `棚卸QR_${item.code}.svg`;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  URL.revokeObjectURL(url);
}

function _inventoryQrLabelHtml(item) {
  return `<article class="qr-label">
    <div class="qr-image" aria-label="管理番号 ${_inventoryQrEscape(item.code)} のQRコード">
      ${createInventoryQrSvg(item, { cellSize: 5, margin: 12 })}
    </div>
    <div class="qr-label-copy">
      <span>棚卸用・管理番号</span>
      <strong>${_inventoryQrEscape(item.code)}</strong>
      <b>${_inventoryQrEscape([item.brand, item.model].filter(Boolean).join(' / ') || '商品名未設定')}</b>
      <small>型番: ${_inventoryQrEscape(item.ref || '—')}</small>
      <small>シリアル: ${_inventoryQrEscape(item.serial || '—')}</small>
    </div>
  </article>`;
}

function _printInventoryQrItems(items, title) {
  if (!items.length) {
    if (typeof showToast === 'function') showToast('info', '棚卸用QR', '印刷できる商品がありません');
    return;
  }

  const frame = document.createElement('iframe');
  frame.className = 'inventory-qr-print-frame';
  frame.setAttribute('title', '棚卸用QR印刷');
  document.body.appendChild(frame);
  const printDocument = frame.contentDocument || frame.contentWindow?.document;
  if (!printDocument) {
    frame.remove();
    return;
  }

  printDocument.open();
  printDocument.write(`<!doctype html><html lang="ja"><head><meta charset="utf-8"><title>${_inventoryQrEscape(title)}</title>
    <style>
      @page { size: A4; margin: 8mm; }
      * { box-sizing: border-box; }
      body { margin: 0; color: #163858; font-family: "Yu Gothic", "Noto Sans JP", sans-serif; }
      .qr-sheet { display: grid; grid-template-columns: repeat(3, 1fr); gap: 4mm; }
      .qr-label { min-height: 42mm; border: .35mm solid #9fb3c8; border-radius: 2mm; padding: 3mm; display: flex; align-items: center; gap: 3mm; break-inside: avoid; }
      .qr-image { width: 30mm; height: 30mm; flex: 0 0 30mm; }
      .qr-image svg { width: 100%; height: 100%; display: block; }
      .qr-label-copy { min-width: 0; display: flex; flex-direction: column; gap: 1mm; }
      .qr-label-copy span { color: #66798d; font-size: 7pt; }
      .qr-label-copy strong { font-family: Consolas, monospace; font-size: 11pt; overflow-wrap: anywhere; }
      .qr-label-copy b { font-size: 8pt; line-height: 1.35; }
      .qr-label-copy small { color: #43586e; font-size: 7pt; line-height: 1.3; overflow-wrap: anywhere; }
      @media print { .qr-label { box-shadow: none; } }
    </style></head><body><main class="qr-sheet">${items.map(_inventoryQrLabelHtml).join('')}</main></body></html>`);
  printDocument.close();

  setTimeout(() => {
    try {
      frame.contentWindow?.focus();
      frame.contentWindow?.print();
    } finally {
      setTimeout(() => frame.remove(), 1000);
    }
  }, 150);
}

function printInventoryQrLabel() {
  const item = (APP_DATA.inventory || []).find(candidate => candidate.code === _inventoryQrCurrentCode);
  if (item) _printInventoryQrItems([item], `棚卸QR_${item.code}`);
}

function printAllInventoryQrLabels() {
  const items = [...(APP_DATA.inventory || [])].sort((a, b) => String(a.code).localeCompare(String(b.code), 'ja'));
  _printInventoryQrItems(items, `棚卸QR一覧_${new Date().toISOString().slice(0, 10)}`);
}

// =====================================================
// 商品タグ（商品詳細ポップアップ用）
// 表面のみ。売価（USD）と原価（JPY）は符丁で印字する。
// =====================================================

function _inventoryTagCurrentItem() {
  const code = String(window._itemDetailCurrentCode || '').trim();
  return (APP_DATA.inventory || []).find(item => item.code === code) || null;
}

function _inventoryTagAccessories(item) {
  return _inventoryTagValue(accessoryDisplayNames(item?.accessories, item?.accessorySupplements, 'tag').join('、'));
}

function _inventoryTagValue(value, fallback = 'ー') {
  const text = String(value ?? '').trim();
  return text || fallback;
}

function renderInventoryProductTagPanel(item) {
  return `<div class="inventory-product-tag-panel">
    <div class="inventory-product-tag-guide"><i class="fa-solid fa-circle-info"></i>
      表面のみのタグです。売価はUSD、原価は円の金額を符丁で表記します。
    </div>
    <div class="inventory-product-tag-preview" aria-label="商品タグの表面">
      <article class="inventory-product-tag inventory-product-tag-front" aria-label="商品タグ 表面">
        ${createInventoryProductTagSvg(item, { preview: true })}
      </article>
    </div>
  </div>`;
}

// All three output paths use the same drawing, so print and download match the preview.
function _inventoryTagTextLines(value, width, fontSize) {
  const lines = [];
  for (const paragraph of String(value ?? '').split(/\r?\n/)) {
    let line = '', used = 0;
    for (const char of Array.from(paragraph)) {
      const size = /[\x20-\x7e]/.test(char) ? .65 : 1;
      if (line && (used + size) * fontSize > width) { lines.push(line); line = ''; used = 0; }
      line += char; used += size;
    }
    lines.push(line);
  }
  return lines;
}

function _inventoryTagTextBlock(value, options) {
  const { x, y, width, height, field, align = 'middle', weight = 400, title = '', price = '' } = options;
  const safe = _inventoryQrEscape;
  let size = options.fontSize || 22;
  let lines = _inventoryTagTextLines(value, width, size);
  const minimum = options.minFontSize || 12;
  while (size > minimum && lines.length * size * 1.25 > height) {
    size -= 1;
    lines = _inventoryTagTextLines(value, width, size);
  }
  const limit = Math.max(1, Math.floor(height / (size * 1.25)));
  if (lines.length > limit) {
    lines = lines.slice(0, limit);
    lines[limit - 1] = Array.from(lines[limit - 1]).slice(0, -1).join('') + '…';
  }
  const anchor = align === 'start' ? x : align === 'end' ? x + width : x + width / 2;
  const firstY = y + (options.verticalAlign === 'top' ? 0 : (height - lines.length * size * 1.25) / 2) + size;
  return `<g data-tag-field="${safe(field)}"${price ? ` data-tag-price="${price}"` : ''}${title ? ` title="${safe(title)}"` : ''}>
    ${title ? `<title>${safe(title)}</title><rect x="${x}" y="${y}" width="${width}" height="${height}" fill="transparent"/>` : ''}
    <text x="${anchor}" y="${firstY}" text-anchor="${align}" font-size="${size}" font-weight="${weight}"${price ? ' class="tag-price-code"' : ''}>${lines.map((line, index) => `<tspan x="${anchor}" dy="${index ? size * 1.25 : 0}">${safe(line)}</tspan>`).join('')}</text>
  </g>`;
}

function createInventoryProductTagSvg(item, options = {}) {
  if (!item) throw new Error('対象商品が見つかりません');
  const safe = _inventoryQrEscape;
  const label = (type, value) => _inventoryTagValue(masterDisplayName(type, value, 'tag'));
  const detail = (type, value) => _inventoryTagValue(masterDisplayName(type, value, 'detail'));
  const brand = label('brand', item.brand), material = label('material', item.material);
  const belt = label('belt', item.belt), movement = label('movement', item.movement);
  const marking = label('marking', item.marking);
  const buyer = label('staff', item.staffCode || item.staff);
  const condition = item.condition ? masterDisplayName('condition', item.condition, 'tag') : '';
  const remarks = [condition ? `コンディション：${condition}` : '', item.note || ''].filter(Boolean).join('\n');
  const remarksDetail = [item.condition ? `コンディション：${detail('condition', item.condition)}` : '', item.note || ''].filter(Boolean).join('\n');
  const weight = item.weightGrams != null && item.weightGrams !== '' ? `${item.weightGrams} g` : 'ー';
  const accessories = _inventoryTagAccessories(item);
  const accessoryDetails = _inventoryTagValue(accessoryDisplayNames(item.accessories, item.accessorySupplements, 'detail').join('、'));
  const code = _inventoryTagValue(item.code);
  const amountTitle = (value, currency) => value == null || value === '' || !Number.isFinite(Number(value))
    ? '未設定' : new Intl.NumberFormat('ja-JP', { style: 'currency', currency, currencyDisplay: 'code' }).format(Number(value));
  const saleCode = formatPriceCode(item.salePrice, 'USD', 'sale:' + item.code) || 'ー';
  const costCode = formatPriceCode(item.purchasePrice, 'JPY', 'cost:' + item.code) || 'ー';
  const block = (value, field, y, height, extra = {}) => _inventoryTagTextBlock(value, {
    x: 16, y, width: 528, height, field, ...extra,
  });
  const hint = value => options.preview ? value : '';
  return `<svg xmlns="http://www.w3.org/2000/svg" class="inventory-product-tag-svg" width="560" height="740" viewBox="0 0 560 740" role="img" aria-label="管理番号 ${safe(code)} の商品タグ 表面">
    <style>.inventory-product-tag-svg text{font-family:'Yu Gothic','Noto Sans JP',sans-serif;fill:#15191d}.inventory-product-tag-svg .tag-price-code{font-family:Consolas,monospace}</style>
    <rect x="1" y="1" width="558" height="738" fill="#fff" stroke="#15191d" stroke-width="2"/>
    <circle class="inventory-product-tag-hole" cx="280" cy="36" r="10" fill="#f1f3f5" stroke="#9ba3ab"/>
    ${block(`${brand}（${material}）`, 'brand-material', 78, 64, {fontSize:28, weight:700, title:hint(`ブランド名：${detail('brand', item.brand)}\n素材：${detail('material', item.material)}`)})}
    ${block(_inventoryTagValue(item.model), 'model', 150, 64, {fontSize:25, title:hint('モデル名：' + _inventoryTagValue(item.model))})}
    ${block(`${_inventoryTagValue(item.ref)}　／　${_inventoryTagValue(item.serial)}`, 'reference-serial', 220, 37, {fontSize:22, title:hint(`型番：${_inventoryTagValue(item.ref)}\nシリアル：${_inventoryTagValue(item.serial)}`)})}
    ${block(`${_inventoryTagValue(item.dial)}（${weight}）`, 'dial-weight', 261, 37, {fontSize:22, title:hint(`文字盤：${_inventoryTagValue(item.dial)}\n重量：${weight}`)})}
    ${block(`${belt}（${movement}）`, 'belt-movement', 302, 37, {fontSize:22, title:hint(`ベルト種類：${detail('belt', item.belt)}\n駆動：${detail('movement', item.movement)}`)})}
    ${block(accessories, 'accessories', 347, 62, {fontSize:22, title:hint('付属品：' + accessoryDetails)})}
    <g${options.preview ? ` title="${safe('備考：' + _inventoryTagValue(remarksDetail))}"` : ''}>${options.preview ? `<title>${safe('備考：' + _inventoryTagValue(remarksDetail))}</title>` : ''}<text x="16" y="441" font-size="18">備考</text></g>
    ${block(remarks, 'remarks', 448, 132, {fontSize:20, minFontSize:11, align:'start', verticalAlign:'top', title:hint('備考：' + _inventoryTagValue(remarksDetail))})}

    <g class="inventory-product-tag-qr" role="img" aria-label="管理番号 ${safe(code)} のQRコード">${options.preview ? `<title>QRコード：管理番号 ${safe(code)}</title>` : ''}${_inventoryTagQrRects(item.code, 40, 593, 144)}</g>
    ${block(marking, 'marking', 601, 57, {x:236, width:74, fontSize:26, title:hint('マーキング：' + detail('marking', item.marking))})}
    ${block(saleCode, 'sale', 601, 57, {x:330, width:210, fontSize:22, minFontSize:10, weight:700, price:'sale', title:hint('売価（ドル）：' + amountTitle(item.salePrice, 'USD'))})}
    ${block(buyer, 'buyer', 664, 44, {x:234, width:88, fontSize:20, title:hint('仕入バイヤー：' + detail('staff', item.staffCode || item.staff))})}
    ${block(costCode, 'cost', 664, 44, {x:330, width:210, fontSize:22, minFontSize:10, weight:700, price:'cost', title:hint('原価（円）：' + amountTitle(item.purchasePrice, 'JPY'))})}
    ${block(code, 'management-code', 710, 27, {x:235, width:306, fontSize:18, weight:700, align:'end', title:hint('管理番号：' + code)})}
  </svg>`;
}

function _inventoryTagQrRects(code, x, y, size) {
  const qr = qrcode(0, 'M');
  qr.addData(getInventoryQrPayload(code), 'Byte');
  qr.make();
  const modules = qr.getModuleCount();
  const quiet = 4;
  const cell = size / (modules + quiet * 2);
  let rects = `<rect x="${x}" y="${y}" width="${size}" height="${size}" fill="#fff"/>`;
  for (let row = 0; row < modules; row++) {
    for (let column = 0; column < modules; column++) {
      if (!qr.isDark(row, column)) continue;
      const rx = (x + (column + quiet) * cell).toFixed(3);
      const ry = (y + (row + quiet) * cell).toFixed(3);
      rects += `<rect x="${rx}" y="${ry}" width="${cell.toFixed(3)}" height="${cell.toFixed(3)}" fill="#111827"/>`;
    }
  }
  return rects;
}

function downloadInventoryProductTag() {
  const item = _inventoryTagCurrentItem();
  if (!item) return;
  const svg = createInventoryProductTagSvg(item);
  const url = URL.createObjectURL(new Blob([svg], { type: 'image/svg+xml;charset=utf-8' }));
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = `商品タグ_${item.code}.svg`;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  URL.revokeObjectURL(url);
  if (typeof showToast === 'function') showToast('success', '商品タグ', `${item.code} のタグをダウンロードしました`);
}

function printInventoryProductTag() {
  const item = _inventoryTagCurrentItem();
  if (!item) return;
  const frame = document.createElement('iframe');
  frame.className = 'inventory-qr-print-frame';
  frame.setAttribute('title', '商品タグ印刷');
  document.body.appendChild(frame);
  const printDocument = frame.contentDocument || frame.contentWindow?.document;
  if (!printDocument) { frame.remove(); return; }
  printDocument.open();
  printDocument.write(`<!doctype html><html lang="ja"><head><meta charset="utf-8"><title>商品タグ_${_inventoryQrEscape(item.code)}</title>
    <style>@page{size:A4 landscape;margin:8mm}*{box-sizing:border-box}body{margin:0;display:flex;align-items:flex-start;justify-content:center;font-family:"Yu Gothic","Noto Sans JP",sans-serif}.tag-svg{height:185mm;max-width:100%}.tag-svg svg{height:100%;width:auto;max-width:100%;display:block}</style>
    </head><body><div class="tag-svg">${createInventoryProductTagSvg(item)}</div></body></html>`);
  printDocument.close();
  setTimeout(() => {
    try { frame.contentWindow?.focus(); frame.contentWindow?.print(); }
    finally { setTimeout(() => frame.remove(), 1000); }
  }, 150);
}

/** Resolve every product on the purchase, including multiple products from one line. */
function getPurchaseSlipTagProducts(slip) {
  const inventory = APP_DATA.inventory || [];
  const products = [];
  const seen = new Set();
  let missingLines = 0;
  for (const line of slip.lines || []) {
    if (line.lineItemKind === 'part') continue;
    let matches = line._id ? inventory.filter(item => item.purchaseSlipLineId === line._id) : [];
    if (!matches.length) matches = inventory.filter(item =>
      (line.productId && item._id === line.productId) || (line.code && item.code === line.code));
    matches = matches.filter(item => String(item.code || '').trim());
    if (matches.length < Math.max(1, Number(line.quantity) || 1)) missingLines++;
    for (const item of matches) {
      if (seen.has(item.code)) continue;
      seen.add(item.code);
      products.push(item);
    }
  }
  return {products, missingLines};
}

function printPurchaseSlipProductTags(slipId) {
  if (typeof isGuest === 'function' && isGuest()) return;
  const slip = (APP_DATA.purchaseSlips || []).find(record => record.id === slipId);
  if (!slip) { showToast('error', '商品タグ印刷', '対象の仕入伝票が見つかりません'); return; }
  const {products, missingLines} = getPurchaseSlipTagProducts(slip);
  if (missingLines) {
    showToast('warning', '商品タグを印刷できません', `商品を確認できない明細が${missingLines}件あります。在庫への反映を確認してから印刷してください。`);
    return;
  }
  if (!products.length) { showToast('info', '商品タグ印刷', 'この伝票には印刷対象の商品がありません'); return; }
  return _printSlipProductTags(slip.id, products, 'purchase-product-tags-print-frame');
}

/** Original rows are inserted by the cost-slip API with the reserved 元商品 note. */
function getCostAdjustmentSlipTagProducts(slip) {
  const inventory = APP_DATA.inventory || [];
  const products = [];
  const seen = new Set();
  const hasOriginals = ['breakdown', 'combine'].includes(slip.adjustmentType);
  let missingLines = 0;
  for (const line of slip.lines || []) {
    if (line.kind !== 'product' || (hasOriginals && line.note === '元商品')) continue;
    const code = String(line.code || '').trim();
    if (seen.has(code)) continue;
    seen.add(code);
    const product = code && inventory.find(item => item.code === code);
    if (!product) { missingLines++; continue; }
    products.push(product);
  }
  return {products, missingLines};
}

function printCostAdjustmentSlipProductTags(slipId) {
  if (typeof isGuest === 'function' && isGuest()) return;
  const slip = (APP_DATA.costAdjustments || []).find(record => record.id === slipId);
  if (!slip) { showToast('error', '商品タグ発行', '対象の原価調整伝票が見つかりません'); return; }
  const {products, missingLines} = getCostAdjustmentSlipTagProducts(slip);
  if (missingLines) {
    showToast('warning', '商品タグを発行できません', '商品を確認できない明細が' + missingLines + '件あります。在庫への反映を確認してから発行してください。');
    return;
  }
  if (!products.length) { showToast('info', '商品タグ発行', 'この伝票には発行対象の商品がありません（元商品・パーツは対象外）'); return; }
  return _printSlipProductTags(slip.id, products, 'cost-adjustment-product-tags-print-frame');
}

function _printSlipProductTags(slipId, products, frameClass) {
  const pages = products.map(item => `<section class="product-tag-print-page" data-product-code="${_inventoryQrEscape(item.code)}">${createInventoryProductTagSvg(item)}</section>`).join('');
  const frame = document.createElement('iframe');
  frame.className = 'inventory-qr-print-frame ' + frameClass;
  frame.title = `商品タグ一括印刷 ${slipId}（${products.length}点）`;
  document.body.appendChild(frame);
  const printDocument = frame.contentDocument || frame.contentWindow?.document;
  if (!printDocument) { frame.remove(); return; }
  printDocument.open();
  printDocument.write(`<!doctype html><html lang="ja"><head><meta charset="utf-8"><title>商品タグ_${_inventoryQrEscape(slipId)}</title>
    <style>
      @page{size:A4 landscape;margin:8mm}
      *{box-sizing:border-box}body{margin:0;font-family:"Yu Gothic","Noto Sans JP",sans-serif}
      .product-tag-print-page{height:190mm;display:flex;justify-content:center;align-items:flex-start;break-inside:avoid;page-break-inside:avoid;break-after:page;page-break-after:always}
      .product-tag-print-page:last-child{break-after:auto;page-break-after:auto}
      .product-tag-print-page>svg{display:block;height:185mm;width:auto;max-width:100%}
    </style></head><body>${pages}</body></html>`);
  printDocument.close();
  setTimeout(async () => {
    try {
      if (printDocument.fonts?.ready) await printDocument.fonts.ready;
      frame.contentWindow?.focus();
      frame.contentWindow?.print();
    } catch (error) {
      showToast('error', '商品タグ印刷エラー', error.message || '印刷画面を開けませんでした');
    } finally { setTimeout(() => frame.remove(), 1000); }
  }, 150);
  return frame;
}
