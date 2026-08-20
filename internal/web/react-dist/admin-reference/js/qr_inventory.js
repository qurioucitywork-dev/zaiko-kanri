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
// 原価・売価は暗号化仕様確定まで一切出力しない。
// =====================================================

function _inventoryTagCurrentItem() {
  const code = String(window._itemDetailCurrentCode || '').trim();
  return (APP_DATA.inventory || []).find(item => item.code === code) || null;
}

function _inventoryTagAccessories(item) {
  return Array.isArray(item?.accessories) && item.accessories.length
    ? item.accessories.map(value => _inventoryTagMasterName(value, APP_DATA.accessoryRecords || [])).join('、')
    : 'なし';
}

function _inventoryTagMasterName(value, records) {
  const text = String(value ?? '').trim();
  if (!text) return '';
  const record = (records || []).find(candidate =>
    String(candidate?.code || '').trim() === text
    || String(candidate?.id || '').trim() === text
    || String(candidate?.name || '').trim() === text);
  return String(record?.name || text).trim();
}

function _inventoryTagValue(value, fallback = '—') {
  const text = String(value ?? '').trim();
  return text || fallback;
}

function renderInventoryProductTagPanel(item) {
  const qr = createInventoryQrSvg(item, { cellSize: 5, margin: 10 });
  const code = _inventoryQrEscape(item.code || '—');
  const model = _inventoryQrEscape(item.model || '—');
  const brand = _inventoryQrEscape(_inventoryTagMasterName(item.brand, APP_DATA.brandRecords || []) || '—');
  const reference = _inventoryQrEscape(item.ref || '—');
  const serial = _inventoryQrEscape(item.serial || '—');
  const accessories = _inventoryQrEscape(_inventoryTagAccessories(item));
  const note = _inventoryQrEscape(item.note || '');
  const material = _inventoryQrEscape(_inventoryTagValue(_inventoryTagMasterName(item.material, APP_DATA.materials || [])));
  const belt = _inventoryQrEscape(_inventoryTagValue(_inventoryTagMasterName(item.belt, APP_DATA.beltMaterialRecords || [])));
  const movement = _inventoryQrEscape(_inventoryTagValue(_inventoryTagMasterName(item.movement, APP_DATA.movements || [])));
  const marking = _inventoryQrEscape(_inventoryTagValue(_inventoryTagMasterName(item.marking, APP_DATA.markingRecords || [])));

  const qrBlock = `<div class="inventory-product-tag-qr" role="img" aria-label="管理番号 ${code} のQRコード">${qr}</div>`;
  const managementBlock = `<div class="inventory-product-tag-code"><span class="sr-only">管理番号</span><strong>${code}</strong></div>`;

  return `<div class="inventory-product-tag-panel">
    <div class="inventory-product-tag-guide">
      <i class="fa-solid fa-shield-halved"></i>
      QRには管理番号だけが入っています。価格情報はタグへ出力していません。
    </div>
    <div class="inventory-product-tag-preview" aria-label="商品タグの表面と裏面">
      <div class="inventory-product-tag-column">
        <article class="inventory-product-tag inventory-product-tag-front" aria-label="商品タグ 表面">
          <div class="inventory-product-tag-hole" aria-hidden="true"></div>
          <div class="inventory-product-tag-content">
            <div class="inventory-product-tag-model"><span class="sr-only">ブランド・素材・ベルト素材</span><strong>${brand}（${material}／${belt}）</strong></div>
            <dl class="inventory-product-tag-fields">
              <div><dt class="sr-only">モデル名</dt><dd>${model}</dd></div>
              <div><dt class="sr-only">型番</dt><dd>${reference}</dd></div>
              <div><dt class="sr-only">シリアル</dt><dd>${serial}</dd></div>
              <div><dt class="sr-only">付属品</dt><dd>${accessories}</dd></div>
            </dl>
            <div class="inventory-product-tag-note"><span class="sr-only">商品の備考</span><p>${note}</p></div>
            <div class="inventory-product-tag-bottom">
              ${qrBlock}
              <div class="inventory-product-tag-bottom-fields">
                <div class="inventory-product-tag-mini-values">
                  <span><span class="sr-only">駆動方式</span>${movement}</span>
                  <span><span class="sr-only">マーキング</span>${marking}</span>
                </div>
                ${managementBlock}
              </div>
            </div>
          </div>
        </article>
        <span class="inventory-product-tag-side-label">表面</span>
      </div>
      <div class="inventory-product-tag-column">
        <article class="inventory-product-tag inventory-product-tag-back" aria-label="商品タグ 裏面">
          <div class="inventory-product-tag-hole" aria-hidden="true"></div>
          <div class="inventory-product-tag-content inventory-product-tag-back-content">
            <div class="inventory-product-tag-bottom">
              ${qrBlock}
              <div class="inventory-product-tag-bottom-fields inventory-product-tag-back-fields">
                <div class="inventory-product-tag-marking"><span class="sr-only">マーキング</span>${marking}</div>
                ${managementBlock}
              </div>
            </div>
          </div>
        </article>
        <span class="inventory-product-tag-side-label">裏面</span>
      </div>
    </div>
  </div>`;
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

function _inventoryTagWrap(value, maxChars = 24, maxLines = 4) {
  const text = String(value || '').replace(/\s+/g, ' ').trim();
  const lines = [];
  for (let index = 0; index < text.length && lines.length < maxLines; index += maxChars) {
    lines.push(text.slice(index, index + maxChars));
  }
  if (text.length > maxChars * maxLines && lines.length) {
    lines[lines.length - 1] = `${lines[lines.length - 1].slice(0, Math.max(0, maxChars - 1))}…`;
  }
  return lines;
}

function createInventoryProductTagSvg(item) {
  if (!item) throw new Error('対象商品が見つかりません');
  const safe = _inventoryQrEscape;
  const code = safe(item.code || '—');
  const noteLines = _inventoryTagWrap(item.note, 24, 4)
    .map((line, index) => `<tspan x="76" dy="${index === 0 ? 0 : 23}">${safe(line)}</tspan>`)
    .join('');
  const frontQr = _inventoryTagQrRects(item.code, 70, 650, 190);
  const backQr = _inventoryTagQrRects(item.code, 680, 650, 190);
  const valueField = (value, y) => `<text x="76" y="${y}" class="value">${safe(value || '—')}</text><line x1="76" y1="${y + 13}" x2="504" y2="${y + 13}" class="rule"/>`;
  const brand = safe(_inventoryTagMasterName(item.brand, APP_DATA.brandRecords || []) || '—');
  const material = safe(_inventoryTagValue(_inventoryTagMasterName(item.material, APP_DATA.materials || [])));
  const belt = safe(_inventoryTagValue(_inventoryTagMasterName(item.belt, APP_DATA.beltMaterialRecords || [])));
  const movement = safe(_inventoryTagValue(_inventoryTagMasterName(item.movement, APP_DATA.movements || [])));
  const marking = safe(_inventoryTagValue(_inventoryTagMasterName(item.marking, APP_DATA.markingRecords || [])));

  return `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="1160" height="930" viewBox="0 0 1160 930" role="img" aria-label="管理番号 ${code} の商品タグ 表面と裏面">
  <style>
    text{font-family:'Yu Gothic','Noto Sans JP',sans-serif;fill:#111827}.card{fill:#fff;stroke:#d1d5db;stroke-width:2}.rule{stroke:#4b5563;stroke-width:1.5}.value{font-size:20px}.model-value{font-size:25px;font-weight:700}.note-text{font-size:17px}.code-value{font-size:19px;font-weight:700;font-family:Consolas,monospace}.side{font-size:21px;font-weight:700}
  </style>
  <rect width="1160" height="930" fill="#f4f4f5"/>
  <rect x="35" y="25" width="520" height="850" rx="36" class="card"/>
  <circle cx="295" cy="75" r="23" fill="#d1d5db" stroke="#9ca3af" stroke-width="2"/>
  <text x="76" y="165" class="model-value">${brand}（${material}／${belt}）</text><line x1="76" y1="178" x2="504" y2="178" class="rule"/>
  ${valueField(item.model, 245)}
  ${valueField(item.ref, 300)}
  ${valueField(item.serial, 355)}
  ${valueField(_inventoryTagAccessories(item), 410)}
  <rect x="66" y="448" width="438" height="160" rx="10" fill="#fff" stroke="#4b5563" stroke-width="1.5"/>
  <text x="76" y="480" class="note-text">${noteLines}</text>
  ${frontQr}
  <text x="300" y="678" class="value">${movement}</text><line x1="300" y1="693" x2="390" y2="693" class="rule"/>
  <text x="414" y="678" class="value">${marking}</text><line x1="414" y1="693" x2="504" y2="693" class="rule"/>
  <text x="300" y="738" class="code-value">${code}</text><line x1="300" y1="753" x2="504" y2="753" class="rule"/>
  <text x="295" y="915" text-anchor="middle" class="side">表面</text>

  <rect x="645" y="25" width="480" height="850" rx="36" class="card"/>
  <circle cx="885" cy="75" r="23" fill="#d1d5db" stroke="#9ca3af" stroke-width="2"/>
  ${backQr}
  <text x="900" y="678" class="value">${marking}</text><line x1="900" y1="693" x2="1085" y2="693" class="rule"/>
  <text x="900" y="738" class="code-value">${code}</text><line x1="900" y1="753" x2="1085" y2="753" class="rule"/>
  <text x="885" y="915" text-anchor="middle" class="side">裏面</text>
</svg>`;
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
    <style>@page{size:A4 landscape;margin:8mm}*{box-sizing:border-box}body{margin:0;display:flex;align-items:flex-start;justify-content:center;font-family:"Yu Gothic","Noto Sans JP",sans-serif}.tag-svg{width:270mm;max-height:190mm}.tag-svg svg{width:100%;height:auto;display:block}</style>
    </head><body><div class="tag-svg">${createInventoryProductTagSvg(item)}</div></body></html>`);
  printDocument.close();
  setTimeout(() => {
    try { frame.contentWindow?.focus(); frame.contentWindow?.print(); }
    finally { setTimeout(() => frame.remove(), 1000); }
  }, 150);
}
