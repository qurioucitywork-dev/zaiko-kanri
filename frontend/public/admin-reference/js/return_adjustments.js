// 99999 is a financial-only return line, never an inventory product.
function isReturnAdjustment(item) { return item?.kind === 'adjustment' || item?.code === '99999'; }
function returnProductCount(items) { return (items || []).filter(item => !isReturnAdjustment(item)).length; }
function returnAdjustmentMoney(amount, currency) {
  return new Intl.NumberFormat('ja-JP', {style:'currency',currency:currency || 'JPY',maximumFractionDigits:2}).format(Number(amount) || 0);
}
function returnAdjustmentSource(prefix) {
  return prefix === 'pr' ? (APP_DATA.purchaseSlips || []).find(s => s.id === _purchaseReturnTargetCode)
    : (APP_DATA.sales || []).find(s => s.id === _salesReturnTargetId);
}
function returnAdjustmentCurrency(prefix) {
  const source = returnAdjustmentSource(prefix);
  return prefix === 'pr' ? source?.purchaseCurrency || source?.lines?.[0]?.purchaseCurrency || 'JPY'
    : source?.inputCurrency || source?.currency || 'USD';
}
function returnAdjustmentItems(prefix) { return prefix === 'pr' ? _prAddedItems : _srAddedItems; }
function addReturnAdjustment(prefix) {
  const rows = returnAdjustmentItems(prefix);
  if (rows.length >= 100) { showToast('warning','明細は100件までです',''); return; }
  rows.push({kind:'adjustment',code:'99999',itemName:'',model:'',brand:'',amountMinor:0,amountInput:'',sign:'+',purchasePrice:0,salePrice:0});
  (prefix === 'pr' ? _prRenderAddedItems : _srRenderAddedItems)();
  document.getElementById(prefix + '-add-code-input').value = '';
  document.getElementById(`${prefix}-adjustment-name-${rows.length-1}`)?.focus();
}
function returnAdjustmentEditor(item, index, prefix) {
  const esc = _escHtml;
  return `<div style="padding:10px 14px;border-bottom:1px solid var(--border);background:#f8fafc;">
    <div style="display:flex;gap:8px;align-items:center;flex-wrap:wrap;">
      <code>99999</code>
      <input class="form-control" id="${prefix}-adjustment-name-${index}" aria-label="項目名" maxlength="200" placeholder="項目名を入力" value="${esc(item.itemName || '')}" style="flex:1;min-width:120px;" oninput="updateReturnAdjustment('${prefix}',${index},'name',this.value)">
      <select class="form-control" aria-label="金額の符号" style="width:64px;" onchange="updateReturnAdjustment('${prefix}',${index},'sign',this.value)"><option value="+" ${item.sign !== '-' ? 'selected' : ''}>＋</option><option value="-" ${item.sign === '-' ? 'selected' : ''}>－</option></select>
      <input class="form-control" aria-label="金額（税抜）" type="number" min="1" max="1000000000000" step="1" placeholder="金額" value="${esc(item.amountInput ?? '')}" style="width:140px;" oninput="updateReturnAdjustment('${prefix}',${index},'amount',this.value)">
      <span>${esc(returnAdjustmentCurrency(prefix))}</span>
      <button class="btn btn-sm btn-outline" aria-label="金額調整行を削除" onclick="${prefix}RemoveItem(${index})"><i class="fa-solid fa-trash-can"></i></button>
    </div><div style="margin-top:5px;font-size:11px;color:var(--text-muted);">金額調整（在庫に影響しません）・元伝票と同じ通貨／税区分</div>
  </div>`;
}
function updateReturnAdjustment(prefix, index, field, value) {
  const item = returnAdjustmentItems(prefix)[index];
  if (!isReturnAdjustment(item)) return;
  if (field === 'name') item.itemName = item.model = value;
  if (field === 'sign') item.sign = value;
  if (field === 'amount') item.amountInput = value;
  item.amountMinor = (item.sign === '-' ? -1 : 1) * Number(item.amountInput || 0);
  item.purchasePrice = item.salePrice = item.amountMinor;
  updateReturnEntryTotal(prefix);
}
function returnEntryItemAmount(item, prefix) {
  if (isReturnAdjustment(item)) return item.amountMinor;
  if (prefix === 'pr') return item.purchasePrice || 0;
  const line = returnAdjustmentSource(prefix)?.items?.find(line => line.code === item.code);
  return line?.inputAmount ?? item.salePrice ?? 0;
}
function updateReturnEntryTotal(prefix) {
  const rows=returnAdjustmentItems(prefix), adjustments=rows.filter(isReturnAdjustment).length;
  const count=document.getElementById(prefix+'-item-count-badge'), total=document.getElementById(prefix+'-total-badge');
  if(count) count.textContent=`${returnProductCount(rows)}点${adjustments ? ` ＋ 調整${adjustments}行` : ''}`;
  if(total) total.textContent=rows.length ? `税抜合計: ${returnAdjustmentMoney(rows.reduce((sum,item)=>sum+returnEntryItemAmount(item,prefix),0),returnAdjustmentCurrency(prefix))}` : '';
}
function getReturnAdjustmentPayload(prefix) {
  const lines=returnAdjustmentItems(prefix).filter(isReturnAdjustment);
  if (lines.some(item=>!item.itemName.trim() || !Number.isSafeInteger(item.amountMinor) || item.amountMinor === 0 || Number(item.amountInput) <= 0 || Math.abs(item.amountMinor)>1000000000000)) {
    showToast('error','金額調整の入力を確認してください','項目名、＋／－、1以上の整数金額を入力してください。');
    return null;
  }
  return lines.map(item=>({name:item.itemName.trim(),amountMinor:item.amountMinor}));
}
function returnAdjustmentView(details,line,index) {
  return {kind:'adjustment',code:'99999',adjustmentIndex:index,itemName:line.name,model:line.name,brand:'',
    amountMinor:line.amountMinor,inputAmount:line.amountMinor,inputCurrency:details.currency,currency:details.currency,
    purchaseOriginalPrice:line.amountMinor,purchaseCurrency:details.currency,purchasePrice:line.amountMinor,
    salePrice:line.amountMinor,status:'金額調整',fxRateScaled:details.fxRateScaled,fxScale:details.fxScale};
}
function returnAdjustmentJPY(item, ret) {
  const details=ret.adjustmentDetails;
  return details.currency === 'JPY' ? Number(item.amountMinor) : Math.round(Number(item.amountMinor)*details.fxRateScaled/details.fxScale);
}
// New returns with adjustment lines retain the original document's currency and tax basis.
function returnFinancialAmountInfo(ret, purchase) {
  const details=ret.adjustmentDetails;
  const source=((purchase ? APP_DATA.purchaseSlips : APP_DATA.sales) || []).find(s=>s.id===ret.slipId);
  const items=(ret.items || []).map((item,index)=>{
    if(isReturnAdjustment(item)) return {...item,no:index+1,purchasePrice:item.amountMinor,salePrice:item.amountMinor};
    const sourceLine=(purchase ? source?.lines : source?.items)?.find(line=>line.code===item.code);
    const inventory=(APP_DATA.inventory || []).find(product=>product.code===item.code) || {};
    const amount=purchase ? sourceLine?.purchasePrice ?? item.purchaseOriginalPrice ?? item.purchasePrice
      : sourceLine?.inputAmount ?? item.inputAmount ?? item.salePrice;
    return {...inventory,...item,...(sourceLine?.productDetail || {}),...sourceLine,no:index+1,purchasePrice:Number(amount)||0,salePrice:Number(amount)||0};
  });
  const subtotal=items.reduce((sum,item)=>sum+Number(purchase ? item.purchasePrice : item.salePrice),0);
  const taxAmount=details.taxMode==='standard' ? items.reduce((sum,item)=>sum+Math.floor(Number(purchase ? item.purchasePrice : item.salePrice)*details.taxRateBasisPoints/10000),0) : 0;
  return {sourcePurchase:purchase ? source : null,sourceSale:purchase ? null : source,items,subtotal,taxAmount,grandTotal:subtotal+taxAmount,
    inputCurrency:details.currency,nativeAmounts:true,rate:details.fxRateScaled/details.fxScale,
    taxMode:details.taxMode,taxRateBasisPoints:details.taxRateBasisPoints,
    formatAmount:amount=>returnAdjustmentMoney(amount,details.currency)};
}
function returnApprovalAdjustmentEditor(doc) {
  if (!doc.adjustmentDetails?.lines?.length) return '';
  return `<fieldset style="margin-top:16px;padding:12px;"><legend>金額調整（在庫対象外・${_escHtml(doc.adjustmentDetails.currency)}）</legend>${doc.adjustmentDetails.lines.map((line,index)=>`<div style="display:flex;gap:8px;margin:8px 0;"><code>99999</code><input class="form-control" aria-label="調整項目名" data-return-adjustment-name="${index}" value="${_escHtml(line.name)}" required maxlength="200"><input class="form-control" aria-label="調整金額（符号付き）" data-return-adjustment-amount="${index}" type="number" step="1" value="${line.amountMinor}" required></div>`).join('')}<small>マイナスは金額の先頭に「-」を入力してください。</small></fieldset>`;
}
function captureReturnApprovalAdjustments(doc) {
  return (doc.adjustmentDetails?.lines || []).map((line,index)=>({name:document.querySelector(`[data-return-adjustment-name="${index}"]`)?.value.trim() ?? line.name,amountMinor:Number(document.querySelector(`[data-return-adjustment-amount="${index}"]`)?.value ?? line.amountMinor)}));
}
function returnListSummary(records,purchase) {
  const sums=new Map();
  records.filter(ret=>!purchase || !_isPurchaseReturnWithdrawn(ret)).forEach(ret=>{
    const info=purchase ? getPurchaseReturnOriginalAmountInfo(ret) : getSalesReturnOriginalAmountInfo(ret);
    const currency=info.inputCurrency || 'JPY';
    let amount=purchase ? info.subtotal : info.grandTotal;
    if(!purchase && !info.nativeAmounts && currency==='JPY') amount=Math.round(amount*info.rate);
    sums.set(currency,(sums.get(currency)||0)+amount);
  });
  return [...sums].map(([currency,amount])=>returnAdjustmentMoney(amount,currency)).join(' / ') || 'ー';
}
