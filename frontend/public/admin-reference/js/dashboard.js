// Dashboard values are derived from all hydrated products, including their
// latest confirmed sale and stable purchase/sale IDs (not the capped slip list).
let businessDashboardState = null;
const BUSINESS_DASH_COLORS = ['#5086bb','#dc8736','#67ab71','#8772b6','#4ca2a1','#c3737c','#7a8b9e','#b6a356'];
function businessMonth(value) { return String(value || '').slice(0,7); }
function businessMonthNow(now = new Date()) { return `${now.getFullYear()}-${String(now.getMonth()+1).padStart(2,'0')}`; }
function businessMonthShift(key, offset) { const [y,m]=key.split('-').map(Number); const d=new Date(y,m-1+offset,1);return businessMonthNow(d); }
function businessMonthLabel(key) { const [y,m]=String(key).split('-');return `${y}年${Number(m)}月`; }
function businessDashboardDefaults(now = new Date()) { const month=businessMonthNow(now);return {preset:'current',from:month,to:month,currency:'JPY',staff:'',supplier:'',buyer:'',excludeCurrent:false,group:'staff',secondary:'supplier',expanded:[]}; }
// Production rates are resolved server-side from transaction snapshots or full history.
function businessDashboardHistoricalRate(product, kind, data) {
  const field = kind === 'purchase' ? 'dashboardPurchaseUsdRate' : 'dashboardSaleUsdRate';
  const rate = Number(product[field]);
  if (product.apiManaged || Object.hasOwn(product, field)) return rate > 0 && Number.isFinite(rate) ? rate : null;
  const currency = kind === 'purchase' ? product.purchaseCurrency : product.actualSaleCurrency;
  const scaled = Number(product[kind + 'FxRateScaled']), scale = Number(product[kind + 'FxScale']);
  if (currency === 'USD' && scaled > 0 && scale > 0) return scaled / scale;
  const date = String(product[kind === 'purchase' ? 'purchaseDate' : 'saleDate'] || '').slice(0,10);
  const end = /^\d{4}-\d{2}-\d{2}$/.test(date) ? Date.parse(date + 'T00:00:00+09:00') + 86400000 : NaN;
  const history = (data.fxRateHistory || []).filter(r => r.code === 'USD' && Number(r.rate) > 0
    && Number.isFinite(Number(r.rate)) && Date.parse(r.observedAt) < end)
    .sort((a,b) => Date.parse(b.observedAt) - Date.parse(a.observedAt) || Date.parse(b.createdAt || b.observedAt) - Date.parse(a.createdAt || a.observedAt));
  return history.length ? Number(history[0].rate) : null;
}
function businessDashboardToUsd(jpy, rate) {
  if (jpy === 0) return 0;
  return Number.isFinite(jpy) && Number.isFinite(rate) && rate > 0 ? jpy / rate : null;
}
function businessDashboardRows(data = APP_DATA) {
  const saleByCode=new Map(),purchaseByCode=new Map();
  // Preview-only fixtures may not have the server's stable IDs.
  for(const slip of data.purchaseSlips||[]) for(const line of slip.lines||[]) if(line.code) purchaseByCode.set(line.code,slip);
  for(const sale of [...(data.sales||[])].sort((a,b)=>String(a.date).localeCompare(String(b.date)))) {
    if(!['confirmed','確定','処理済'].includes(sale.apiStatus||sale.status)) continue;
    for(const item of sale.items||[]) if(item.code) saleByCode.set(item.code,{sale,item});
  }
  return (data.inventory||[]).map(p=>{
    const source=saleByCode.get(p.code),purchase=purchaseByCode.get(p.code),raw=p.dashboardRawStatus||p.status;
    const excluded=Boolean(p.inventoryValuationExcluded)||['cancelled','cost_adjusted','broken_down','combined','仕入返品','仕入返品済','仕入返品処理済','原価調整済','結合済み'].includes(raw);
    const sold=['sold','売上済'].includes(raw);
    const live=p.apiManaged;
    const purchaseUsdRate=businessDashboardHistoricalRate(p,'purchase',data),saleUsdRate=businessDashboardHistoricalRate(p,'sale',data);
    const saleJpy=live?Number(p.actualSalePriceJpy)||0:Number(source?.item.salePrice||p.salePrice||0)*(source?.sale.currency==='JPY'?1:(saleUsdRate||0));
    return {code:p.code,staff:p.staffCode||p.staff||'',supplier:p.supplier||'',buyer:p.saleBuyerCode||(!live?source?.sale.buyer:'')||'',
      purchaseSlip:p.purchaseSlipId||(!live?purchase?._id||purchase?.id:'')||'',saleSlip:p.saleSlipId||(!live?source?.sale._id||source?.sale.id:'')||'',
      purchaseMonth:businessMonth(p.purchaseDate||purchase?.date),saleMonth:businessMonth(p.saleDate||(!live?source?.sale.date:'')),
      purchaseUsdRate,saleUsdRate,excluded,sold,cost:p.dashboardCostJpy??(Number(p.purchasePrice)||0),purchaseCost:p.dashboardPurchaseJpy??(Number(p.purchasePrice)||0),saleAmount:saleJpy};
  });
}
function businessDashboardAggregate(rows, filters, now = new Date()) {
  const sales=new Set(),purchases=new Set();let inventoryCount=0,salesAmount=0,purchaseAmount=0,inventoryAmount=0,salesCost=0;
  const range=month=>Boolean(month)&&month>=filters.from&&month<=filters.to;
  const amount=(value,rate)=>filters.currency==='USD'?businessDashboardToUsd(value,rate):value;
  // A missing historical rate invalidates only the affected total; never show a partial sum.
  const add=(total,value)=>total===null||value===null?null:total+value;
  for(const r of rows) {
    if(r.excluded || (filters.staff&&r.staff!==filters.staff) || (filters.supplier&&r.supplier!==filters.supplier)) continue;
    if(r.purchaseSlip&&range(r.purchaseMonth)){purchases.add(r.purchaseSlip);purchaseAmount=add(purchaseAmount,amount(r.purchaseCost,r.purchaseUsdRate));}
    if(!r.sold&&(filters.preset!=='custom'||range(r.purchaseMonth))&&(!filters.excludeCurrent||r.purchaseMonth!==businessMonthNow(now))){inventoryCount++;inventoryAmount=add(inventoryAmount,amount(r.cost,r.purchaseUsdRate));}
    if(r.sold&&r.saleSlip&&range(r.saleMonth)&&(!filters.buyer||r.buyer===filters.buyer)){
      sales.add(r.saleSlip);salesAmount=add(salesAmount,amount(r.saleAmount,r.saleUsdRate));
      salesCost=add(salesCost,amount(r.cost,r.purchaseUsdRate));
    }
  }
  const profit=salesAmount===null||salesCost===null?null:salesAmount-salesCost;
  return {salesCount:sales.size,purchaseCount:purchases.size,inventoryCount,salesAmount,purchaseAmount,inventoryAmount,profit,
    margin:profit===null||!salesAmount?null:profit/salesAmount*100};
}
const BUSINESS_DASH_PERIOD_METRICS = ['salesCount', 'purchaseCount', 'salesAmount', 'purchaseAmount', 'profit', 'margin'];

// Compare this calendar month with complete preceding months. Keep empty months
// in the divisor, and reuse the dashboard's transaction-rate and partner filters.
function businessDashboardComparisons(rows, filters, now = new Date()) {
  const month = businessMonthNow(now);
  const monthly = Array.from({ length: 7 }, (_, index) => {
    const key = businessMonthShift(month, -index);
    return businessDashboardAggregate(rows, { ...filters, from: key, to: key }, now);
  });
  const result = { month, current: monthly[0] };
  for (const [period, count] of [['quarter', 3], ['half', 6]]) {
    const average = {}, ratios = {}, reasons = {};
    for (const key of BUSINESS_DASH_PERIOD_METRICS) {
      const values = monthly.slice(1, count + 1).map(metrics => metrics[key]);
      average[key] = values.every(Number.isFinite) ? values.reduce((sum, value) => sum + value, 0) / count : null;
      const value = monthly[0][key], baseline = average[key];
      ratios[key] = Number.isFinite(value) && Number.isFinite(baseline) && baseline !== 0 ? value / baseline * 100 : null;
      if (!Number.isFinite(ratios[key])) ratios[key] = null;
      reasons[key] = ratios[key] !== null ? '' : !Number.isFinite(value) || baseline === null
        ? (key === 'margin' ? '当月または比較期間に粗利率を計算できない月があります（売上高0、または当時のレート未登録）。' : '当月または比較期間に当時のUSDレートが未登録の取引があります。')
        : baseline === 0 ? '比較する月平均が0のため、比率を計算できません。' : '比率を計算できません。';
    }
    result[period] = { from: businessMonthShift(month, -count), to: businessMonthShift(month, -1), average, ratios, reasons };
  }
  return result;
}

function businessDashboardComparisonValue(key, value, currency) {
  if (!Number.isFinite(value)) return '—';
  if (key === 'margin') return `${value.toFixed(1)}%`;
  if (key.endsWith('Count')) return `${value.toLocaleString('ja-JP', { maximumFractionDigits: 2 })}件`;
  return businessDashboardMoney(value, currency);
}

function businessDashboardComparisonHtml(key, comparison, filters) {
  if (!BUSINESS_DASH_PERIOD_METRICS.includes(key)) return '';
  const labels = { current: '当月', quarter: '直近3か月平均比', half: '半年平均比' };
  const periods = ['current', 'quarter', 'half'].filter(period => period !== filters.preset);
  return `<dl class="business-kpi-comparisons" aria-label="他の期間の実績・平均比">${periods.map(period => {
    let value, description;
    if (period === 'current') {
      value = businessDashboardComparisonValue(key, comparison.current[key], filters.currency);
      description = `${businessMonthLabel(comparison.month)}の実績：${value}`;
    } else {
      const entry = comparison[period], ratio = entry.ratios[key];
      value = ratio === null ? '—' : `${ratio.toLocaleString('ja-JP', { minimumFractionDigits: 1, maximumFractionDigits: 1 })}%`;
      const current = businessDashboardComparisonValue(key, comparison.current[key], filters.currency);
      const average = businessDashboardComparisonValue(key, entry.average[key], filters.currency);
      description = `${businessMonthLabel(comparison.month)}の実績 ${current} ÷ ${businessMonthLabel(entry.from)}～${businessMonthLabel(entry.to)}の月平均 ${average} × 100。${entry.reasons[key] || '100%で過去の月平均と同じです。'}`;
    }
    return `<div class="business-kpi-comparison" data-business-comparison="${period}" tabindex="0" title="${_escHtml(description)}" aria-label="${_escHtml(labels[period] + '：' + value + '。' + description)}"><dt>${labels[period]}</dt><dd>${_escHtml(value)}</dd></div>`;
  }).join('')}</dl>`;
}

function businessDashboardGroup(rows, filters, key, now = new Date()) {
  const groups=new Map();
  for(const row of rows){const value=row[key]||'';if(!groups.has(value))groups.set(value,[]);groups.get(value).push(row);}
  return [...groups].map(([value,items])=>({value,items,metrics:businessDashboardAggregate(items,filters,now)}))
    .filter(g=>key==='buyer'?g.metrics.salesCount:g.metrics.salesCount||g.metrics.purchaseCount||g.metrics.inventoryCount)
    .sort((a,b)=>b.metrics.salesAmount-a.metrics.salesAmount||b.metrics.purchaseAmount-a.metrics.purchaseAmount||a.value.localeCompare(b.value,'ja'));
}
function businessDashboardName(key,value) {
  if(!value)return '未設定';
  const records=key==='staff'?APP_DATA.staffRecords:key==='supplier'?APP_DATA.suppliers:APP_DATA.buyers;
  // Match the deduplicated filter options if a legacy directory contains aliases.
  return (records||[]).findLast(r=>r.code===value||r.name===value)?.name||value;
}
function businessDashboardMoney(amount, currency = businessDashboardState?.currency || 'JPY') {
  if (!Number.isFinite(amount)) return '—';
  return new Intl.NumberFormat('ja-JP',{style:'currency',currency,maximumFractionDigits:currency==='JPY'?0:2}).format(amount);
}
function businessDashboardMetric(key,value) {
  if(key==='margin') return value==null?'—':`${value.toFixed(1)}%`;
  if(key.endsWith('Count'))return `${Number(value).toLocaleString('ja-JP')}<small>${key==='inventoryCount'?'点':'件'}</small>`;
  return businessDashboardMoney(value);
}
function businessDashboardOptions(key,selected,rows) {
  const records=key==='staff'?APP_DATA.staffRecords:key==='supplier'?APP_DATA.suppliers:APP_DATA.buyers;
  const values=new Map((records||[]).map(r=>[r.code||r.name,r.name]));
  for(const row of rows)if(row[key]&&!values.has(row[key]))values.set(row[key],businessDashboardName(key,row[key]));
  return '<option value="">すべて</option>'+[...values].sort((a,b)=>String(a[1]).localeCompare(String(b[1]),'ja')).map(([value,name])=>`<option value="${_escHtml(value)}" ${value===selected?'selected':''}>${_escHtml(name)}</option>`).join('');
}
function businessDashboardPreset(preset) {
  const end=businessMonthNow(),start=businessMonthShift(end,preset==='half'?-5:preset==='quarter'?-2:0);
  businessDashboardState={...businessDashboardState,preset,from:preset==='custom'?businessDashboardState.from:start,to:preset==='custom'?businessDashboardState.to:end};
  // Keep un-applied partner filters when changing the period shortcut.
  for(const key of ['staff','supplier','buyer']) businessDashboardState[key]=document.getElementById('business-'+key)?.value||'';
  renderBusinessDashboard();
}
function businessDashboardApply() {
  const from=document.getElementById('business-from').value,to=document.getElementById('business-to').value;
  const error=document.getElementById('business-filter-error');
  if(!/^\d{4}-(0[1-9]|1[0-2])$/.test(from)||!/^\d{4}-(0[1-9]|1[0-2])$/.test(to)||from>to){error.textContent='開始月と終了月を正しく指定してください。';return;}
  const next={...businessDashboardState,from,to,expanded:[]};
  if(from!==businessDashboardState.from||to!==businessDashboardState.to)next.preset='custom';
  for(const key of ['staff','supplier','buyer'])next[key]=document.getElementById('business-'+key).value;
  businessDashboardState=next;renderBusinessDashboard();
}
function businessDashboardReset(){businessDashboardState=businessDashboardDefaults();renderBusinessDashboard();}
function businessDashboardCurrency(currency){businessDashboardState.currency=currency;renderBusinessDashboard();}
function businessDashboardGroupBy(key){businessDashboardState.group=key;businessDashboardState.expanded=[];renderBusinessDashboard();}
function businessDashboardSecondary(key){businessDashboardState.secondary=key;businessDashboardState.expanded=[];renderBusinessDashboard();}
function businessDashboardExpand(index){const a=businessDashboardState.expanded;businessDashboardState.expanded=a.includes(index)?a.filter(i=>i!==index):[...a,index];renderBusinessDashboard();}
function businessDashboardInventoryExclude(checked){businessDashboardState.excludeCurrent=checked;renderBusinessDashboard();}
function businessDashboardComposition(rows, filters, key, field, title) {
  const allGroups=businessDashboardGroup(rows,filters,key);
  if (allGroups.some(g=>g.metrics[field]===null)) return `<section class="business-card business-composition"><h3>${title}</h3><p class="business-caption">${key==='supplier'?'仕入高':'売上高'}ベース</p><div class="business-empty">当時のUSDレートが未登録のため、構成比を計算できません。</div></section>`;
  const groups=allGroups.filter(g=>g.metrics[field]>0).sort((a,b)=>b.metrics[field]-a.metrics[field]);
  const total=groups.reduce((sum,g)=>sum+g.metrics[field],0);let offset=0;
  const segments=groups.map((g,i)=>{const start=offset;offset+=g.metrics[field]/total*100;return `${BUSINESS_DASH_COLORS[i%BUSINESS_DASH_COLORS.length]} ${start}% ${offset}%`;});
  return `<section class="business-card business-composition"><h3><i class="fa-solid fa-chart-pie" aria-hidden="true"></i>${title}</h3><p class="business-caption">${key==='supplier'?'仕入高':'売上高'}ベース</p>${total?`<div class="business-chart-body"><div class="business-donut" role="img" aria-label="${title}。内訳は右の一覧に表示" style="background:conic-gradient(${segments.join(',')})"><div><small>合計</small><strong>${businessDashboardMoney(total)}</strong></div></div><ul class="business-chart-legend">${groups.map((g,i)=>`<li><span class="business-dot" style="background:${BUSINESS_DASH_COLORS[i%BUSINESS_DASH_COLORS.length]}"></span><span class="business-legend-name" data-preserve-terminology>${_escHtml(businessDashboardName(key,g.value))}<small>${businessDashboardMoney(g.metrics[field])}</small></span><strong>${(g.metrics[field]/total*100).toFixed(1)}%</strong></li>`).join('')}</ul></div>`:'<div class="business-empty">対象期間のデータがありません</div>'}</section>`;
}

// Capture the rendered report so pending filter edits and background data changes
// cannot make the downloaded CSV disagree with the dashboard on screen.
function businessDashboardCaptureCsv(root, filters) {
  const text = node => {
    if (!node) return '';
    const copy = node.cloneNode(true);
    copy.querySelectorAll('br').forEach(br => br.replaceWith('\n'));
    return copy.textContent.trim();
  };
  const labels = { staff: '仕入者', supplier: '仕入先', buyer: '販売先' };
  const rows = [['ダッシュボード'], ['集計開始月', filters.from], ['集計終了月', filters.to],
    ['表示通貨', filters.currency],
    ...Object.entries(labels).map(([key, label]) => [label, filters[key] ? businessDashboardName(key, filters[key]) : 'すべて']),
    ['在庫の仕入月', filters.preset === 'custom' ? filters.from + ' ～ ' + filters.to : '指定なし（現時点の全在庫）'],
    ['当月仕入分を除く', filters.excludeCurrent ? 'はい' : 'いいえ'],
    ['内訳の分類', labels[filters.group] + '別'],
    ['二次分類', filters.group === 'staff' ? labels[filters.secondary] + '別' : '—'],
    ['絞り込みの適用範囲', text(root.querySelector('.business-filter > .business-caption'))]];
  for (const [selector, title] of [['.business-kpi-grid', '期間集計'], ['.business-inventory-grid', '現在在庫']]) {
    rows.push([], [title], ['項目', '表示値']);
    root.querySelectorAll(selector + ' .business-kpi').forEach(card => {
      const label = text(card.querySelector('.business-kpi-label'));
      rows.push([label, text(card.querySelector('[data-business-metric]'))]);
      card.querySelectorAll('[data-business-comparison]').forEach(entry => rows.push([
        label + '（' + text(entry.querySelector('dt')) + '）', text(entry.querySelector('dd')), entry.title
      ]));
    });
    if (selector === '.business-kpi-grid') rows.push(['補足の計算方法', text(root.querySelector('.business-comparison-caption'))]);
  }
  rows.push([], ['内訳集計']);
  root.querySelectorAll('.business-table tr').forEach(row => rows.push([...row.cells].map(text)));
  root.querySelectorAll('.business-composition').forEach(chart => {
    rows.push([], [text(chart.querySelector('h3'))], ['集計基準', text(chart.querySelector('.business-caption'))], ['名称', '金額', '構成比']);
    chart.querySelectorAll('.business-chart-legend li').forEach(item => {
      const name = item.querySelector('.business-legend-name').cloneNode(true);
      name.querySelector('small')?.remove();
      rows.push([text(name), text(item.querySelector('.business-legend-name small')), text(item.querySelector('strong'))]);
    });
    const total = chart.querySelector('.business-donut strong');
    rows.push(total ? ['合計', text(total), '100.0%'] : [text(chart.querySelector('.business-empty'))]);
  });
  rows.push([], ['集計上の注意'], [text(root.querySelector('.business-footnote'))]);
  return { rows, filename: 'ダッシュボード_' + filters.from + '_' + filters.to + '_' + filters.currency + '.csv' };
}
function businessDashboardSerializeCsv(rows) {
  return '\uFEFF' + rows.map(row => row.map(value => {
    let cell = String(value ?? '');
    // Names and labels are user-editable. Keep spreadsheet formulas as text.
    if (/^[\s\uFEFF]*[=+@-]/.test(cell)) cell = "'" + cell;
    return '"' + cell.replace(/"/g, '""') + '"';
  }).join(',')).join('\r\n') + '\r\n';
}
function businessDashboardExportCsv() {
  const snapshot = document.getElementById('page-dashboard')?.businessDashboardCsv;
  if (!snapshot) return;
  const url = URL.createObjectURL(new Blob([businessDashboardSerializeCsv(snapshot.rows)], { type: 'text/csv;charset=utf-8;' }));
  const anchor = document.createElement('a');
  anchor.href = url; anchor.download = snapshot.filename; anchor.hidden = true;
  document.body.appendChild(anchor);
  try { anchor.click(); } finally { anchor.remove(); setTimeout(() => URL.revokeObjectURL(url), 1000); }
}

function renderBusinessDashboard(now = new Date()) {
  const root=document.getElementById('page-dashboard');if(!root)return;
  const apiState=window.ZaikoAPI?.state;
  if(apiState && !apiState.hydrated && !apiState.dashboardReady) {
    root.businessDashboardCsv=null;
    root.innerHTML=`<section class="business-panel" role="status" aria-live="polite"><h2><i class="fa-solid ${apiState.lastError?'fa-triangle-exclamation':'fa-spinner fa-spin'}" aria-hidden="true"></i> ${apiState.lastError?'データを再取得しています':'ダッシュボードを読み込んでいます'}</h2><p>在庫・取引情報を確認しています。取得したデータから集計結果を表示します。</p></section>`;
    return;
  }
  businessDashboardState ||= businessDashboardDefaults(now);const f=businessDashboardState,rows=businessDashboardRows(),m=businessDashboardAggregate(rows,f,now),comparison=businessDashboardComparisons(rows,f,now),period=f.from===f.to?businessMonthLabel(f.from):`${businessMonthLabel(f.from)} ～ ${businessMonthLabel(f.to)}`;
  const columns=[['salesCount','売上件数'],['purchaseCount','仕入件数'],['inventoryCount','在庫件数'],['salesAmount','売上高'],['purchaseAmount','仕入高'],['inventoryAmount','在庫高'],['profit','粗利高'],['margin','粗利率']];
  const kpi=(key,label,icon,color)=>`<div class="business-kpi"><span class="business-kpi-icon ${color}"><i class="fa-solid ${icon}" aria-hidden="true"></i></span><div><span class="business-kpi-label">${label}</span><strong data-business-metric="${key}" class="${m[key]<0?'business-negative':''}">${businessDashboardMetric(key,m[key])}</strong>${businessDashboardComparisonHtml(key,comparison,f)}</div></div>`;
  const groups=businessDashboardGroup(rows,f,f.group);
  const rowHtml=(g,index,child=false,key=f.group)=>`<tr class="${child?'business-child':''}"><th scope="row" data-preserve-terminology>${!child&&key==='staff'?`<button type="button" class="business-expand" aria-expanded="${f.expanded.includes(index)}" aria-label="${_escHtml(businessDashboardName(key,g.value))}の内訳" onclick="businessDashboardExpand(${index})"><i class="fa-solid fa-chevron-${f.expanded.includes(index)?'down':'right'}" aria-hidden="true"></i>${_escHtml(businessDashboardName(key,g.value))}</button>`:`${child?'<span class="business-child-mark">↳</span>':''}${_escHtml(businessDashboardName(key,g.value))}`}</th>${columns.map(([field])=>`<td class="${g.metrics[field]<0?'business-negative':''}">${key==='buyer'&&['purchaseCount','purchaseAmount','inventoryCount','inventoryAmount'].includes(field)?'<span class="business-not-applicable" title="販売先別では集計しません">—</span>':businessDashboardMetric(field,g.metrics[field])}</td>`).join('')}</tr>`;
  root.innerHTML=`<div class="business-dashboard">
  <section class="business-card business-filter"><div class="business-section-head business-filter-heading"><h3><i class="fa-solid fa-filter" aria-hidden="true"></i>集計条件</h3><button type="button" class="business-export" onclick="businessDashboardExportCsv()" title="現在表示している集計・内訳・構成比をCSV出力"><i class="fa-solid fa-file-csv" aria-hidden="true"></i> CSV出力</button></div>
   <div class="business-filter-top"><div class="business-segments" role="group" aria-label="集計期間">${[['current','当月'],['quarter','直近3か月'],['half','半年'],['custom','範囲指定（月）']].map(([key,label])=>`<button type="button" aria-pressed="${f.preset===key}" onclick="businessDashboardPreset('${key}')">${label}</button>`).join('')}</div>
   <div class="business-month-range"><label><span class="sr-only">開始月</span><input id="business-from" type="month" value="${f.from}"></label><span>～</span><label><span class="sr-only">終了月</span><input id="business-to" type="month" value="${f.to}"></label></div>
   <div class="business-segments business-currency" role="group" aria-label="表示通貨">${['JPY','USD'].map(c=>`<button type="button" aria-pressed="${f.currency===c}" onclick="businessDashboardCurrency('${c}')">${c==='JPY'?'¥':'$'}　${c}</button>`).join('')}</div></div>
   <div class="business-filter-bottom">${[['staff','仕入者'],['supplier','仕入先'],['buyer','販売先']].map(([key,label])=>`<label for="business-${key}">${label}<select id="business-${key}">${businessDashboardOptions(key,f[key],rows)}</select></label>`).join('')}<div class="business-filter-actions"><button type="button" class="business-apply" onclick="businessDashboardApply()">適用</button><button type="button" class="business-reset" onclick="businessDashboardReset()">リセット</button></div></div>
   <p class="business-caption">販売先の絞り込みは、売上件数・売上高・粗利高・粗利率に適用します。</p><p id="business-filter-error" role="alert"></p>
  </section>
  <section class="business-card"><div class="business-section-head"><h3><i class="fa-solid fa-chart-column" aria-hidden="true"></i>期間集計</h3><span class="business-caption">${_escHtml(period)}</span><span class="business-unit">${f.currency}・売上は税抜</span></div><div class="business-kpi-grid" aria-live="polite">${kpi('salesCount','売上件数','fa-cart-shopping','blue')}${kpi('purchaseCount','仕入件数','fa-box','amber')}${kpi('salesAmount','売上高','fa-chart-simple','green')}${kpi('purchaseAmount','仕入高','fa-wallet','red')}${kpi('profit','粗利高','fa-coins','purple')}${kpi('margin','粗利率','fa-percent','teal')}</div><p class="business-comparison-caption">補足の当月は${businessMonthLabel(comparison.month)}。平均比＝当月 ÷ 前月までの3か月・6か月の月平均 × 100（100%で平均と同じ）。計算できない値は「—」です。</p></section>
  <section class="business-card"><div class="business-section-head"><h3><i class="fa-solid fa-cube" aria-hidden="true"></i>現在在庫</h3><span class="business-caption">${f.preset==='custom'?`仕入月：${_escHtml(period)}`:'現時点の全在庫'}</span><label class="business-exclude"><input type="checkbox" ${f.excludeCurrent?'checked':''} onchange="businessDashboardInventoryExclude(this.checked)">当月仕入分を除く</label></div><div class="business-inventory-grid" aria-live="polite">${kpi('inventoryCount','在庫件数','fa-box-archive','blue')}${kpi('inventoryAmount','在庫高','fa-database','green')}<p class="business-caption">売上済・仕入返品・原価調整済を除きます。<br>「範囲指定（月）」では、仕入月で在庫を絞り込めます。</p></div></section>
  <section class="business-card"><h3><i class="fa-solid fa-table-list" aria-hidden="true"></i>内訳集計</h3><div class="business-breakdown-toolbar"><div class="business-segments" role="group" aria-label="内訳の分類">${[['staff','仕入者別'],['supplier','仕入先別'],['buyer','販売先別']].map(([key,label])=>`<button type="button" aria-pressed="${f.group===key}" onclick="businessDashboardGroupBy('${key}')">${label}</button>`).join('')}</div>${f.group==='staff'?`<label>二次分類<select aria-label="二次分類" onchange="businessDashboardSecondary(this.value)"><option value="supplier" ${f.secondary==='supplier'?'selected':''}>仕入先別</option><option value="buyer" ${f.secondary==='buyer'?'selected':''}>販売先別</option></select></label>`:''}</div>
  <div class="business-table-scroll"><table class="business-table"><thead><tr><th>${{staff:'仕入者',supplier:'仕入先',buyer:'販売先'}[f.group]}</th>${columns.map(([,label])=>`<th>${label}</th>`).join('')}</tr></thead><tbody>${groups.map((g,i)=>rowHtml(g,i)+(f.group==='staff'&&f.expanded.includes(i)?businessDashboardGroup(g.items,f,f.secondary).map(child=>rowHtml(child,i,true,f.secondary)).join(''):'')).join('')||'<tr><td colspan="9" class="business-empty">条件に一致するデータがありません</td></tr>'}</tbody></table></div></section>
  <div class="business-composition-grid">${businessDashboardComposition(rows,f,'supplier','purchaseAmount','仕入先別 構成比')}${businessDashboardComposition(rows,f,'buyer','salesAmount','販売先別 構成比')}</div>
  <footer class="business-footnote">売上・仕入件数は対象商品を含む伝票を重複なく集計。在庫件数は商品点数です。伝票が複数の分類にまたがる場合、内訳件数の合計は全体件数と一致しません。<br>仕入返品・原価調整済は集計対象外。仕入高は仕入時原価、在庫高は現在原価。粗利高＝売上高－売上対象商品の現在原価。粗利率は売上高が0の場合「—」。${f.currency==='USD'?'<br>USD表示は商品ごとに、売上高は売上時、仕入高・在庫高・売上原価は仕入時のレートで換算してから合算します。保存済みレートを優先し、それ以外は取引日までの最新履歴を使用します。当時のレートが未登録の集計額は「—」と表示します。':''}</footer></div>`;
  root.businessDashboardCsv = businessDashboardCaptureCsv(root, f);
}
document.addEventListener('zaiko:data-hydrated',()=>{if(document.getElementById('page-dashboard')?.classList.contains('hidden')===false)renderBusinessDashboard();});

document.addEventListener('zaiko:dashboard-ready',()=>{if(typeof applyRoleUI==='function')applyRoleUI();if(document.getElementById('page-dashboard')?.classList.contains('hidden')===false)renderBusinessDashboard();});
document.addEventListener('zaiko:data-hydrate-failed',()=>{if(document.getElementById('page-dashboard')?.classList.contains('hidden')===false)renderBusinessDashboard();});
