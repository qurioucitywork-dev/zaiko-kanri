/** Structured product data for the common statement; never parse display strings. */
function getSlipStatementProduct(line = {}, purpose = 'domestic', defaults = {}, options = {}) {
  if (line.kind === 'adjustment' || line.code === '99999') return {kind:'adjustment',code:'99999',model:line.itemName || line.model || ''};
  const saved = (APP_DATA.inventory || []).find(p => p.code === line.code) || {};
  const product = { ...defaults, ...saved };
  for (const source of [line, line.product || {}, line.productDetail || {}]) {
    for (const [key, value] of Object.entries(source)) {
      if (value !== undefined && (source !== line || (value !== null && value !== ''))) product[key] = value;
    }
  }
  const staff = product.staffCode || product.staff || '';
  const record = masterDisplayRecord('staff', staff);
  return { ...getDocumentProductDetails(product, purpose, options),
    statementCostJpy: getStatementCostJpy(line, saved),
    buyerMasterCode: record?.code || product.staffCode || '',
    boxNo: product.boxNo ?? product.boxCode ?? '',
  };
}

// Width is measured in full-width characters, with conservative Latin widths.
// Explicit lines let pagination preserve long descriptions without clipping them.
function slipStatementWrap(value, width) {
  const result = [];
  for (const paragraph of String(value ?? '').split(/\r?\n/)) {
    let line = '', units = 0;
    for (const char of Array.from(paragraph)) {
      const size = /[\x20-\x7e]/.test(char) ? .65 : 1;
      if (line && units + size > width) { result.push(line); line = ''; units = 0; }
      line += char; units += size;
    }
    result.push(line);
  }
  return result;
}

/** Keep each product field on one line, shrinking long values to its column. */
function slipStatementProductLine(value, columnIndex) {
  const text = String(value ?? '').replace(/\s+/g, ' ').trim();
  const width = [11, 17, 21][columnIndex];
  const units = Array.from(text).reduce((total, char) => total +
    (/[MW@%]/.test(char) ? 1 : /[\x20-\x7e]/.test(char) ? .75 : 1), 0);
  const percent = units > width ? Math.floor(width / units * 96) : 100;
  return `<span class="tpl-statement-line tpl-product-line" style="font-size:${percent}%">${_escHtml(text) || '&nbsp;'}</span>`;
}

function slipStatementRow(item, purpose, includeInternalNotes = false) {
  if (item.kind === 'adjustment' || item.code === '99999') {
    const name=slipStatementWrap(item.itemName || item.detail || item.product?.model || '',32);
    return {item,columns:[[],name,[],[],['99999']],height:Math.max(3,name.length)};
  }
  const p = item.product || getSlipStatementProduct({ code: item.code }, purpose);
  const columns = [
    [p.material, p.brand, p.movement],
    [p.model, p.ref, p.serial],
    [p.dial, p.belt, p.weightGrams !== '' && p.weightGrams != null ? `${p.weightGrams} g` : ''],
  ];
  const productLines = columns.map(column => column.map(value => String(value ?? '').replace(/\s+/g, ' ').trim()));
  const accessoryLines = slipStatementWrap((p.accessories || []).join(purpose === 'overseas' ? ' / ' : '・'), 22);
  const remarks = [...accessoryLines, ...Array(Math.max(0, 2 - accessoryLines.length)).fill(''),
    ...slipStatementWrap([[includeInternalNotes ? p.comment : '', p.note].filter(Boolean).join(' / '), includeInternalNotes ? item.statementNote : ''].filter(Boolean).join('\n'), 22)];
  const internal = [String(item.code || p.code || ''),
    formatPriceCode(p.statementCostJpy !== undefined ? p.statementCostJpy : p.purchasePrice, 'JPY', `cost:${item.code || p.code || ''}`),
    [p.buyerMasterCode, p.boxNo].filter(v => v !== '' && v != null).join('・')];
  const management = internal.flatMap(value => slipStatementWrap(value, 10));
  return { item, columns: [...productLines, remarks, management],
    height: Math.max(3, ...productLines.map(lines => lines.length), remarks.length, management.length) };
}

function slipStatementPages(items, purpose, includeInternalNotes = false) {
  const pages = [[]];
  let used = 0;
  items.forEach(item => {
    const row = slipStatementRow(item, purpose, includeInternalNotes);
    let offset = 0;
    while (offset < row.height) {
      if (used === 120 || (offset === 0 && used && used + row.height > 120)) {
        pages.push([]); used = 0;
      }
      const height = Math.min(row.height - offset, 120 - used);
      pages[pages.length - 1].push({ ...row, height, continuation: offset > 0,
        columns: row.columns.map(lines => lines.slice(offset, offset + height)) });
      offset += height; used += height;
    }
  });
  return pages;
}

function slipStatementStyles() {
  return `
    .tpl-detail-page{padding:28px 32px;min-height:1122px}
    .tpl-detail-heading h2{font-size:14px}.tpl-detail-heading div{font-size:9px}
    .tpl-detail-table{font-size:7.333px;line-height:1.05}
    .tpl-detail-table th{background:#fff;text-align:left;padding:2px 2px;font-weight:500}
    .tpl-detail-table td{padding:0 2px;height:auto;vertical-align:middle}
    .tpl-detail-table .tpl-col-no{width:3.5%}.tpl-detail-table .tpl-col-detail{width:54%}
    .tpl-detail-table .tpl-col-remarks{width:23%}.tpl-detail-table .tpl-col-amount{width:9.5%}.tpl-detail-table .tpl-col-code{width:10%}
    .tpl-product-grid{display:grid;grid-template-columns:22% 34% 44%}
    .tpl-product-grid>div{min-width:0}.tpl-product-line{white-space:nowrap;overflow-wrap:normal}
    .tpl-statement-line{display:block;min-height:7.7px;line-height:7.7px;white-space:pre}
    .tpl-cell-number,.tpl-cell-code{text-align:center;font-size:inherit}
    .tpl-cell-code{font-family:Consolas,monospace}
    .tpl-detail-table .tpl-cell-amount{text-align:center;white-space:nowrap;font-size:7px}
    .tpl-statement-summary{display:flex;justify-content:flex-end;gap:16px;border:1px solid #666;border-top:0;padding:6px 4px;font-size:8px}
    .tpl-note-block{border:1px solid #666;margin-top:24px;font-size:11px;overflow-wrap:anywhere}
    .tpl-bank-block{position:static;margin-top:28px}
    @media print{.tpl-detail-page{padding:8mm 10mm;min-height:297mm}.tpl-statement-line{min-height:2mm;line-height:2mm}.tpl-detail-table{font-size:5.5pt}.tpl-detail-table .tpl-cell-amount{font-size:5.25pt}.tpl-detail-table tr{break-inside:avoid;page-break-inside:avoid}.tpl-statement-summary{break-inside:avoid}.tpl-note-block,.tpl-bank-block{break-inside:avoid}.tpl-bank-block{position:static}}
  `;
}

function overseasSlipAmountLabel(label, title) {
  if (title === '出荷伝票' || title === '委託伝票') return 'Estimated price';
  return ({'仕入金額（税抜）':'Purchase amount (excl. tax)',
    '仕入返品金額（税抜）':'Purchase return amount (excl. tax)',
    '売上返品金額（税抜）':'Sales return amount (excl. tax)',
    '修正金額（税抜）':'Adjusted amount (excl. tax)',
    '調整後・配分原価':'Adjusted / allocated cost',
    '販売価格（税抜）':'Selling price (excl. tax)'})[label] || 'Amount';
}

// Translate application-owned captions only; registered names and free text are preserved.
function overseasSlipSystemNote(value) {
  return String(value || '').replace(/売上登録時固定レート：|登録時固定レート：|仕入登録時固定レート：/g, 'Registered exchange rate: ')
    .replace(/元仕入伝票：/g,'Original purchase invoice: ')
    .replace(/元売上伝票：/g,'Original sales invoice: ')
    .replace(/基準通貨：/g,'Base currency: ')
    .replace(/販売時レート：/g,'Sale exchange rate: ')
    .replace(/^(?:仕入担当者|バイヤー)：/gm,'Buyer: ')
    .replace(/^支払い方法：(現金|カード|銀行振込)$/gm,(_,method)=>'Payment method: '+({'現金':'Cash','カード':'Card','銀行振込':'Bank transfer'})[method])
    .replace(/^仕入通貨：(日本円（JPY）|米ドル（USD）|香港ドル（HKD）)$/gm,(_,unit)=>'Purchase currency: '+({'日本円（JPY）':'JPY','米ドル（USD）':'USD','香港ドル（HKD）':'HKD'})[unit])
    .replace(/^仕入レート（登録時固定）：/gm,'Registered purchase exchange rate: ')
    .replace(/^消費税相当額（社内参考・合計外）：/gm,'Tax equivalent (internal reference, excluded from total): ');
}

function overseasSlipCover(options, ci, subtotal, taxAmount, grandTotal, formatAmount, currency) {
  const esc = _escHtml;
  const subtype = ({'仕入伝票':'PURCHASE','仕入返品伝票':'PURCHASE RETURN','売上返品伝票':'SALES RETURN',
    '出荷伝票':'SHIPMENT','委託伝票':'CONSIGNMENT','売上修正伝票':'SALES ADJUSTMENT','売上変更伝票':'SALES ADJUSTMENT',
    '請求書':'SALES','売上伝票':'SALES'})[options.title] || '';
  const incoming = ['仕入伝票','売上返品伝票'].includes(options.title);
  const company = {name:ci.companyName,address:[ci.zip,ci.address].filter(Boolean).join('\n'),contact:ci.tel,email:ci.email};
  const party = (label,record={}) => `<section class="oi-party"><h2>${label}</h2><strong>${esc(record.name || 'Not registered')}</strong><div class="oi-address">${esc(record.address || '')}</div><div><b>TEL:</b> ${esc(record.contact || record.tel || '-')}</div><div><b>e-mail:</b> ${esc(record.email || '-')}</div></section>`;
  const items = options.items || [];
  let weight = 0, missing = 0;
  for (const item of items) {
    const p = item.product || getSlipStatementProduct(item,'overseas');
    const raw = p.weightGrams;
    const n = Number(raw);
    if (raw == null || raw === '' || !Number.isFinite(n) || n < 0) missing++;
    else weight += n;
  }
  const issued = options.issuedAt ? new Date(options.issuedAt) : new Date();
  const issueDate = Number.isFinite(issued.getTime()) ? issued.toLocaleDateString('en-GB',{day:'2-digit',month:'short',year:'numeric',timeZone:'Asia/Tokyo'}).toUpperCase() : '-';
  const note = options.showNote === false ? '' : `<div class="oi-notes"><h2>Remarks</h2><div>${esc(overseasSlipSystemNote(options.note)) || '-'}</div></div>`;
  const bank = options.includeBank ? `<section class="oi-bank"><h2>Bank details</h2><div><b>Bank:</b> ${esc(ci.bankName || '-')} &nbsp; <b>Branch:</b> ${esc(ci.branchName || '-')}</div><div><b>Account no.:</b> ${esc(ci.accountNumber || '-')}</div><div><b>Account holder:</b> ${esc(ci.accountHolder || '-')}</div></section>` : '';
  return `<section class="tpl-doc-page tpl-cover-page oi-cover">
    <h1 class="oi-title">INVOICE</h1>${subtype ? `<div class="oi-subtitle">${subtype}</div>` : ''}
    <div class="oi-reference"><div><b>DATE:</b><span>${esc(issueDate)}</span></div><div><b>INV NO:</b><strong>${esc(options.slipId || 'Not issued')}</strong></div>${options.transactionDate && options.showTransactionDate !== false ? `<div><b>Transaction date:</b><span>${esc(options.transactionDate)}</span></div>` : ''}</div>
    <div class="oi-parties">${party('Shipper:',incoming ? options.counterparty : company)}${party('Consignee:',incoming ? company : options.counterparty)}</div>
    <div class="oi-totals"><div><b>Qty.</b><span>${items.length.toLocaleString('en-US')} pcs</span></div><div><b>Wt (g)</b><span>${missing ? '-' : weight.toLocaleString('en-US',{maximumFractionDigits:3})+' g'}</span></div><div class="oi-grand"><b>Total (${esc(currency)})</b><strong>${formatAmount(grandTotal)}</strong></div>
    ${taxAmount ? `<small>Subtotal: ${formatAmount(subtotal)} / Tax: ${formatAmount(taxAmount)}</small>` : ''}
    ${options.title === '出荷伝票' || options.title === '委託伝票' ? '<small>Total estimated selling price</small>' : ''}
    ${missing ? `<small>Weight unavailable for ${missing} item(s).</small>` : ''}</div>
    ${note}${bank}
  </section>`;
}

function overseasSlipStyles() {
  return `.tpl-overseas{font-family:Arial,"Yu Gothic",sans-serif}
  .tpl-overseas .oi-cover{padding:45px 68px;min-height:1122px}
  .oi-title{text-align:center;font-size:30px;line-height:1.2;margin:0 auto;text-decoration:underline;text-underline-offset:7px;letter-spacing:0}
  .oi-subtitle{text-align:center;color:#68717b;font-size:10px;margin:13px 0 0}
  .oi-reference{margin:20px 0 0 auto;width:235px;font-size:11px}.oi-reference>div{display:flex;justify-content:space-between;gap:12px;padding:4px 0;border-bottom:1px solid #aab0b6}.oi-reference span,.oi-reference strong{overflow-wrap:anywhere;text-align:right}
  .oi-parties{margin-top:-28px;width:59%;padding-right:20px;box-sizing:border-box}.oi-party{font-size:12px;line-height:1.75;margin-bottom:44px;overflow-wrap:anywhere}.oi-party h2{font-size:12px;text-decoration:underline;text-underline-offset:4px;margin:0 0 10px}.oi-party>strong{font-size:13px;display:block;margin-bottom:7px}.oi-address{white-space:pre-line;min-height:68px;margin-bottom:8px}.oi-party b{display:inline-block;min-width:45px}
  .oi-totals{margin:0 0 0 auto;width:48%;font-size:12px;break-inside:avoid}.oi-totals>div{display:flex;justify-content:space-between;gap:15px;border-bottom:1px solid #68717b;padding:9px 0}.oi-grand strong{font-size:15px;white-space:nowrap}.oi-totals small{display:block;text-align:right;color:#68717b;font-size:10px;margin-top:7px}
  .oi-notes{margin-top:35px;break-inside:avoid}.oi-notes h2,.oi-bank h2{font-size:12px;margin:0 0 6px}.oi-notes>div{border:1px solid #68717b;min-height:50px;padding:12px;white-space:pre-line;overflow-wrap:anywhere;font-size:11px}.oi-bank{margin-top:22px;border-top:1px solid #68717b;padding-top:10px;font-size:10px;overflow-wrap:anywhere;break-inside:avoid}
  .tpl-overseas .tpl-detail-heading{display:block;text-align:center;margin-bottom:8px}.tpl-overseas .tpl-detail-heading h2{font-weight:700;letter-spacing:0;font-size:18px}.tpl-overseas .tpl-detail-heading>div{position:static;justify-content:flex-end;border:0;font-size:9px;padding-top:8px}
  .tpl-overseas .tpl-detail-table th{background:#f1f2f3;font-weight:600;overflow-wrap:anywhere}.tpl-overseas .tpl-statement-summary{flex-wrap:wrap;gap:5px 12px}.tpl-overseas .oi-blank td{height:23.1px;padding:0}
  @media(max-width:700px){.tpl-overseas .oi-cover{padding:28px 24px}.oi-parties{margin-top:20px;width:100%}.oi-reference{width:100%}.oi-totals{width:100%}}
  @media print{.tpl-overseas .oi-cover{padding:13mm 18mm;min-height:297mm}.tpl-overseas .oi-parties{width:59%;margin-top:-28px}.tpl-overseas .oi-reference{width:62mm}.tpl-overseas .oi-totals{width:48%}.tpl-overseas .oi-blank td{height:5.7mm}.tpl-overseas .tpl-statement-line{min-height:1.9mm;line-height:1.9mm}.tpl-overseas .tpl-detail-table{font-size:5.2pt}.tpl-overseas .tpl-detail-heading h2{font-size:14px}.tpl-overseas .tpl-detail-page{padding-top:8mm;padding-bottom:6mm}}
  `;
}
