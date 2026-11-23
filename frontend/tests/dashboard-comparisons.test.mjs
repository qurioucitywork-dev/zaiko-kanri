import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {createRequire} from 'node:module';
import path from 'node:path';
const frontend=process.argv[2] || path.resolve(import.meta.dirname,'..');
const {JSDOM}=createRequire(path.join(frontend,'package.json'))('jsdom');
const source=await readFile(process.argv[3] || path.join(frontend,'public/admin-reference/js/dashboard.js'),'utf8');
const dom=new JSDOM('<main id="page-dashboard"></main>',{runScripts:'dangerously',url:'http://localhost/'}),w=dom.window;
w._escHtml=s=>String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
w.APP_DATA={inventory:[],staffRecords:[{code:'A',name:'担当A'},{code:'B',name:'担当B'}],suppliers:[{code:'P',name:'仕入先'}],buyers:[{code:'X',name:'販売先X'},{code:'Y',name:'販売先Y'}],fxRates:[{code:'USD',rate:999}]};
w.eval(source+';window.setState=s=>businessDashboardState=s;window.getState=()=>businessDashboardState;');
const now=new Date(2026,0,15),filter=w.businessDashboardDefaults(now);
const base={staff:'A',supplier:'P',buyer:'X',excluded:false,sold:true,purchaseUsdRate:100,saleUsdRate:200};
const row=(month,saleAmount,cost,purchaseCost,ids)=>({...base,code:month+'-'+ids,purchaseMonth:month,saleMonth:month,saleAmount,cost,purchaseCost,purchaseSlip:'P'+ids,saleSlip:'S'+ids});
const rows=[
 row('2026-01',600,300,150,'NOW1'),row('2026-01',600,300,150,'NOW2'),
 // Two products on the same previous-month voucher count as one voucher.
 row('2025-12',450,225,100,'DEC'),row('2025-12',450,225,100,'DEC'),
 row('2025-11',600,360,200,'NOV'),row('2025-10',1500,300,200,'OCT'),
 row('2025-09',600,300,200,'SEP'),row('2025-07',1200,600,200,'JUL'),
 row('2025-06',999999,100,999999,'TOO-OLD'),row('2026-02',999999,100,999999,'FUTURE'),
 {...row('2025-12',999999,0,999999,'EXCLUDED'),excluded:true}
];
const original=JSON.stringify(rows),c=w.businessDashboardComparisons(rows,filter,now);
assert.equal(c.month,'2026-01');assert.equal(c.quarter.from,'2025-10');assert.equal(c.quarter.to,'2025-12');
assert.equal(c.half.from,'2025-07');assert.equal(c.half.to,'2025-12');
assert.equal(c.current.salesAmount,1200);assert.equal(c.current.salesCount,2);
assert.equal(c.quarter.average.salesAmount,1000);assert.equal(c.quarter.ratios.salesAmount,120);
assert.equal(c.half.average.salesAmount,800);assert.equal(c.half.ratios.salesAmount,150);
assert.equal(c.quarter.average.salesCount,1);assert.equal(c.quarter.ratios.salesCount,200);
assert.equal(c.half.average.salesCount,5/6);assert.equal(c.half.ratios.salesCount,240,'empty months remain in six-month average');
assert.equal(c.quarter.ratios.purchaseCount,200);assert.equal(c.quarter.ratios.purchaseAmount,150);
assert.equal(c.quarter.average.profit,630);assert.equal(c.quarter.ratios.profit,600/630*100);
assert.equal(c.quarter.average.margin,(50+40+80)/3);assert.equal(c.quarter.ratios.margin,50/((50+40+80)/3)*100,'margin uses the arithmetic mean of monthly margins');
assert.equal(c.half.ratios.margin,null,'undefined monthly margin is not silently treated as zero');
assert.match(c.half.reasons.margin,/粗利率を計算できない月/);
assert.equal(JSON.stringify(rows),original);
const filteredRows=[...rows,{...row('2026-01',1000,100,800,'OTHER'),staff:'B',supplier:'Q',buyer:'Y'}];
assert.equal(w.businessDashboardComparisons(filteredRows,{...filter,staff:'A'},now).quarter.ratios.salesAmount,120);
assert.equal(w.businessDashboardComparisons(filteredRows,{...filter,supplier:'P'},now).quarter.ratios.salesAmount,120);
const byBuyer=w.businessDashboardComparisons(filteredRows,{...filter,buyer:'X'},now);
assert.equal(byBuyer.quarter.ratios.salesAmount,120);assert.equal(byBuyer.current.purchaseAmount,1100,'buyer filters only sales metrics');
// Historical USD conversion happens for each transaction, before averaging.
const usdRows=rows.map(r=>({...r,saleUsdRate:r.saleMonth==='2026-01'?100:200,purchaseUsdRate:r.purchaseMonth==='2026-01'?100:50}));
const usd=w.businessDashboardComparisons(usdRows,{...filter,currency:'USD'},now);
assert.equal(usd.current.salesAmount,12);assert.equal(usd.quarter.average.salesAmount,5);assert.equal(usd.quarter.ratios.salesAmount,240);
assert.equal(usd.quarter.ratios.purchaseAmount,75);
const missing=w.businessDashboardComparisons(usdRows.map(r=>r.saleSlip==='SDEC'?{...r,saleUsdRate:null}:r),{...filter,currency:'USD'},now);
assert.equal(missing.quarter.ratios.salesAmount,null);assert.equal(missing.quarter.ratios.purchaseAmount,75);assert.equal(missing.quarter.ratios.salesCount,200);
assert.match(missing.quarter.reasons.salesAmount,/レートが未登録/);
const empty=w.businessDashboardComparisons([],filter,now);assert.equal(empty.quarter.ratios.salesCount,null);assert.match(empty.quarter.reasons.salesCount,/月平均が0/);
assert.equal(w.businessDashboardComparisons(rows.filter(r=>r.saleMonth!=='2026-01'),filter,now).quarter.ratios.salesAmount,0,'zero current month vs positive past average is 0%');
const root=w.document.getElementById('page-dashboard');
w.APP_DATA.inventory=rows.map(r=>({apiManaged:true,code:r.code,staffCode:r.staff,supplier:r.supplier,saleBuyerCode:r.buyer,purchaseSlipId:r.purchaseSlip,saleSlipId:r.saleSlip,purchaseDate:r.purchaseMonth+'-01',saleDate:r.saleMonth+'-01',dashboardRawStatus:r.sold?'sold':'in_stock',inventoryValuationExcluded:r.excluded,dashboardCostJpy:r.cost,dashboardPurchaseJpy:r.purchaseCost,actualSalePriceJpy:r.saleAmount,dashboardPurchaseUsdRate:r.purchaseUsdRate,dashboardSaleUsdRate:r.saleUsdRate}));
const card=()=>root.querySelector('[data-business-metric="salesAmount"]').closest('.business-kpi');
for(const [preset,offset,expected,main] of [['current',0,['quarter','half'],'￥1,200'],['quarter',-2,['current','half'],'￥2,700'],['half',-5,['current','quarter'],'￥4,800'],['custom',-1,['current','quarter','half'],'￥2,100']]) {
 w.setState({...filter,preset,from:w.businessMonthShift(filter.to,offset)});w.renderBusinessDashboard(now);
 assert.deepEqual([...card().querySelectorAll('[data-business-comparison]')].map(n=>n.dataset.businessComparison),expected);
 assert.equal(card().querySelector('[data-business-metric]').textContent,main,'primary metric keeps selected period total');
 assert.equal(root.querySelectorAll('.business-inventory-grid [data-business-comparison]').length,0,'current inventory is not substituted for historical snapshots');
 assert.equal(root.querySelectorAll('.business-kpi-grid [data-business-comparison]').length,6*expected.length);
 assert.ok(root.businessDashboardCsv.rows.some(r=>r[0]==='売上高'&&r[1]===main));
 if(expected.includes('quarter'))assert.ok(root.businessDashboardCsv.rows.some(r=>r[0]==='売上高（直近3か月平均比）'&&r[1]==='120.0%'&&r[2].includes('2025年10月～2025年12月')));
 if(expected.includes('current'))assert.equal(card().querySelector('[data-business-comparison="current"] dd').textContent,'￥1,200');
}
assert.equal(card().querySelector('[data-business-comparison="half"] dd').textContent,'150.0%');
const before=w.businessDashboardSerializeCsv(root.businessDashboardCsv.rows);
w.document.getElementById('business-staff').value='B';w.document.getElementById('business-from').value='2020-01';
assert.equal(w.businessDashboardSerializeCsv(root.businessDashboardCsv.rows),before,'CSV remains the rendered snapshot until applying edits');
w.setState({...filter,currency:'USD'});w.renderBusinessDashboard(now);
assert.equal(card().querySelector('[data-business-comparison="quarter"] dd').textContent,'120.0%');
w.APP_DATA.fxRates=[{code:'USD',rate:1}];w.renderBusinessDashboard(now);
assert.equal(card().querySelector('[data-business-comparison="quarter"] dd').textContent,'120.0%','current FX never changes the comparisons');
w.APP_DATA.inventory=[];w.renderBusinessDashboard(now);
assert.equal(card().querySelector('[data-business-comparison="quarter"] dd').textContent,'—');
assert.ok(!root.textContent.includes('Infinity'));assert.ok(!root.textContent.includes('NaN'));
dom.window.close();
console.log('Dashboard comparisons passed: previous 3/6 months, year boundary, empty months, voucher counts, margins, filtered historical FX, preset visibility, primary totals and CSV snapshot.');
