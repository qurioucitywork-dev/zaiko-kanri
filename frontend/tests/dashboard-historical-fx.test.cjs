const assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {JSDOM}=require('jsdom');
const dom=new JSDOM('<main id="page-dashboard"></main>',{runScripts:'dangerously',url:'http://localhost/'}),w=dom.window;
w._escHtml=s=>String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const base={apiManaged:true,staffCode:'A',purchaseDate:'2026-09-01',saleDate:'2026-09-21',dashboardRawStatus:'sold'};
w.APP_DATA={fxRates:[{code:'USD',rate:999}],staffRecords:[{code:'A',name:'担当'}],suppliers:[{code:'P',name:'仕入先1'},{code:'Q',name:'仕入先2'}],buyers:[{code:'X',name:'販売先1'},{code:'Y',name:'販売先2'}],inventory:[
 {...base,code:'1',supplier:'P',saleBuyerCode:'X',purchaseSlipId:'P1',saleSlipId:'S1',dashboardPurchaseJpy:10000,dashboardCostJpy:12000,actualSalePriceJpy:30000,dashboardPurchaseUsdRate:100,dashboardSaleUsdRate:150},
 {...base,code:'2',supplier:'Q',saleBuyerCode:'Y',purchaseSlipId:'P2',saleSlipId:'S2',dashboardPurchaseJpy:10000,dashboardCostJpy:10000,actualSalePriceJpy:30000,dashboardPurchaseUsdRate:200,dashboardSaleUsdRate:200},
 {...base,code:'3',supplier:'Q',purchaseSlipId:'P3',saleDate:null,dashboardRawStatus:'in_stock',dashboardPurchaseJpy:20000,dashboardCostJpy:25000,dashboardPurchaseUsdRate:200}
]};
w.eval(fs.readFileSync(path.join(__dirname,'../public/admin-reference/js/dashboard.js'),'utf8')+';window.setState=s=>businessDashboardState=s;');
const now=new Date(2026,8,28),f={...w.businessDashboardDefaults(now),currency:'USD',expanded:[0]};
const root=w.document.getElementById('page-dashboard');
const plain=x=>JSON.parse(JSON.stringify(x));
let rows=w.businessDashboardRows(),m=w.businessDashboardAggregate(rows,f,now);
assert.deepEqual(plain(m),{salesCount:2,purchaseCount:3,inventoryCount:1,salesAmount:350,purchaseAmount:250,inventoryAmount:125,profit:180,margin:180/350*100});
const jpy=w.businessDashboardAggregate(rows,{...f,currency:'JPY'},now);
assert.equal(jpy.salesAmount,60000);assert.equal(jpy.purchaseAmount,40000);assert.equal(jpy.inventoryAmount,25000);assert.equal(jpy.profit,38000);
assert.equal(w.businessDashboardAggregate(rows,{...f,buyer:'X'},now).profit,80);
assert.equal(w.businessDashboardAggregate(rows,{...f,supplier:'Q'},now).purchaseAmount,150);
w.setState(f);w.renderBusinessDashboard();
assert.equal(root.querySelector('[data-business-metric=salesAmount]').textContent,'$350.00');
assert.equal(root.querySelector('[data-business-metric=profit]').textContent,'$180.00');
assert.equal(root.querySelector('[data-business-metric=margin]').textContent,'51.4%');
assert.equal(root.querySelectorAll('.business-child').length,2);
assert.ok(root.businessDashboardCsv.rows.some(r=>r[0]==='売上高'&&r[1]==='$350.00'));
assert.ok(root.businessDashboardCsv.rows.some(r=>r[0]==='販売先1'&&r[1]==='$200.00'&&r[2]==='57.1%'));
assert.ok(root.businessDashboardCsv.rows.some(r=>r[0]==='仕入先2'&&r[1]==='$150.00'&&r[2]==='60.0%'));
assert.ok(root.businessDashboardCsv.rows.some(r=>r[0]?.includes('商品ごとに')));
const before=JSON.stringify(root.businessDashboardCsv.rows);
w.APP_DATA.fxRates=[{code:'USD',rate:1}];w.renderBusinessDashboard();
assert.equal(JSON.stringify(root.businessDashboardCsv.rows),before,'current master rate must never affect historical USD totals or percentages');
w.APP_DATA.inventory[0].dashboardSaleUsdRate=null;w.renderBusinessDashboard();
assert.equal(root.querySelector('[data-business-metric=salesAmount]').textContent,'—');
assert.equal(root.querySelector('[data-business-metric=profit]').textContent,'—');
assert.equal(root.querySelector('[data-business-metric=margin]').textContent,'—');
assert.equal(root.querySelector('[data-business-metric=purchaseAmount]').textContent,'$250.00');
assert.equal(root.querySelectorAll('.business-donut').length,1,'missing sale rates must suppress the entire sales composition, not silently omit the affected buyer');
assert.ok(root.businessDashboardCsv.rows.some(r=>r[0]?.includes('構成比を計算できません')));
w.APP_DATA.inventory[0].dashboardSaleUsdRate=150;
w.APP_DATA.inventory[2].dashboardPurchaseUsdRate=null;
m=w.businessDashboardAggregate(w.businessDashboardRows(),f,now);
assert.equal(m.inventoryAmount,null);assert.equal(m.purchaseAmount,null);assert.equal(m.profit,180);
assert.equal(m.purchaseCount,3);assert.equal(m.inventoryCount,1);
const zero=w.businessDashboardAggregate([{purchaseSlip:'P',purchaseMonth:'2026-09',cost:0,purchaseCost:0}],f,now);
assert.equal(zero.inventoryAmount,0);assert.equal(zero.purchaseAmount,0);
const rates={fxRates:[{code:'USD',rate:999}],fxRateHistory:[
 {code:'USD',rate:100,observedAt:'2026-09-01T00:00:00+09:00'},
 {code:'HKD',rate:20,observedAt:'2026-09-01T23:00:00+09:00'},
 {code:'USD',rate:120,observedAt:'2026-09-01T23:59:59+09:00'},
 {code:'USD',rate:130,observedAt:'2026-09-02T00:00:00+09:00'}]};
assert.equal(w.businessDashboardHistoricalRate({purchaseDate:'2026-09-01'},'purchase',rates),120);
assert.equal(w.businessDashboardHistoricalRate({purchaseDate:'2026-08-31'},'purchase',rates),null);
assert.equal(w.businessDashboardHistoricalRate({purchaseCurrency:'USD',purchaseFxRateScaled:15500,purchaseFxScale:100},'purchase',rates),155);
assert.equal(w.businessDashboardHistoricalRate({apiManaged:true,purchaseDate:'2026-09-01'},'purchase',rates),null,'server-resolved missing rate must not fall back to truncated browser history');
dom.window.close();console.log('Historical USD: transaction rates, current-rate independence, adjusted costs, breakdown, composition, CSV, missing rates and Tokyo dates passed.');
