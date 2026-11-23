import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {JSDOM} from 'jsdom';
const dom=new JSDOM('<main id="page-dashboard"></main>',{runScripts:'dangerously',url:'http://localhost/'});
const w=dom.window;
w.APP_DATA={inventory:[],staffRecords:[{code:'A',name:'田中'},{code:'B',name:'佐藤'}],suppliers:[{code:'S',name:'国内仕入'},{code:'T',name:'海外仕入'}],buyers:[{code:'X',name:'国内販売'},{code:'Y',name:'海外販売'}],fxRates:[{code:'USD',rate:150}]};
w._escHtml=s=>String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
w.eval((await readFile(new URL('../public/admin-reference/js/dashboard.js',import.meta.url),'utf8'))+';window.getBusinessState=()=>businessDashboardState;window.setBusinessState=s=>{businessDashboardState=s};');
const now=new Date(2026,8,23),f=w.businessDashboardDefaults(now);
const base={staff:'A',supplier:'S',buyer:'X',purchaseMonth:'2026-09',saleMonth:'2026-09',excluded:false,sold:false,cost:100,purchaseCost:80,saleAmount:150};
const rows=[
 {...base,code:'1',sold:true,purchaseSlip:'P1',saleSlip:'S1'},
 {...base,code:'2',sold:true,purchaseSlip:'P1',saleSlip:'S1'},
 {...base,code:'3',staff:'B',supplier:'T',buyer:'Y',sold:true,purchaseSlip:'P2',saleSlip:'S2',cost:200,saleAmount:100},
 {...base,code:'4',purchaseSlip:'P3'},
 {...base,code:'5',purchaseSlip:'P4',purchaseMonth:'2026-08'},
 {...base,code:'6',purchaseSlip:'P5',excluded:true,cost:99999,saleAmount:99999},
];
const plain=x=>JSON.parse(JSON.stringify(x));
const metrics=w.businessDashboardAggregate(rows,f,now);
assert.deepEqual(plain(metrics),{salesCount:2,purchaseCount:3,inventoryCount:2,salesAmount:400,purchaseAmount:320,inventoryAmount:200,profit:0,margin:0});
const buyer=w.businessDashboardAggregate(rows,{...f,buyer:'X'},now);
assert.equal(buyer.salesCount,1);assert.equal(buyer.salesAmount,300);assert.equal(buyer.profit,100);assert.equal(buyer.purchaseCount,3);assert.equal(buyer.inventoryCount,2);
assert.equal(w.businessDashboardAggregate(rows,{...f,staff:'A'},now).purchaseCount,2);
assert.equal(w.businessDashboardAggregate(rows,{...f,supplier:'T'},now).profit,-100);
assert.equal(w.businessDashboardAggregate(rows,{...f,excludeCurrent:true},now).inventoryCount,1);
assert.equal(w.businessDashboardAggregate(rows,{...f,preset:'custom'},now).inventoryCount,1);
assert.equal(w.businessDashboardAggregate(rows,{...f,preset:'custom',from:'2026-08',to:'2026-08'},now).inventoryAmount,100);
assert.equal(w.businessDashboardAggregate(rows,{...f,buyer:'missing'},now).margin,null);
assert.equal(w.businessMonthShift('2026-01',-2),'2025-11');
assert.equal(w.businessDashboardAggregate([{...base,purchaseSlip:'P',purchaseMonth:'2025-12'}],{...f,from:'2025-11',to:'2026-01'},now).purchaseCount,1);
const shared=w.businessDashboardGroup(rows.map(r=>r.code==='2'?{...r,staff:'B'}:r),f,'staff',now);
assert.equal(shared.reduce((s,g)=>s+g.metrics.salesCount,0),3,'group counts can overlap the same voucher');
const normalized=w.businessDashboardRows({inventory:[
 {apiManaged:true,code:'a',status:'売上済',dashboardRawStatus:'sold',purchasePrice:99999,dashboardCostJpy:120,dashboardPurchaseJpy:100,purchaseSlipId:'P',saleSlipId:'S',saleBuyerCode:'X',actualSalePriceJpy:180},
 ...['cancelled','cost_adjusted','broken_down','combined'].map(status=>({dashboardRawStatus:status})),
 {inventoryValuationExcluded:true},
]});
assert.equal(normalized[0].cost,120);assert.equal(normalized[0].purchaseCost,100);assert.equal(normalized[0].saleAmount,180);assert.equal(normalized[0].saleSlip,'S');
assert.ok(normalized.slice(1).every(r=>r.excluded));
assert.equal(w.businessDashboardMoney(100,'USD'),'$100.00');
w.APP_DATA.suppliers.push({code:'S',name:'国内仕入（更新後）'});
assert.equal(w.businessDashboardName('supplier','S'),'国内仕入（更新後）');
assert.match(w.businessDashboardOptions('supplier','S',[]),/国内仕入（更新後）/);
w.APP_DATA.fxRates=[];assert.equal(w.businessDashboardMoney(100,'USD'),'$100.00');w.APP_DATA.fxRates=[{code:'USD',rate:150}];
// UI wiring: all 8 metrics, 2 charts, drilldown, filters, empty and invalid states.
w.APP_DATA.inventory=rows.map(r=>({apiManaged:true,code:r.code,staffCode:r.staff,supplier:r.supplier,saleBuyerCode:r.buyer,purchaseSlipId:r.purchaseSlip,saleSlipId:r.saleSlip,purchaseDate:r.purchaseMonth+'-01',saleDate:r.saleMonth+'-01',dashboardRawStatus:r.sold?'sold':'in_stock',inventoryValuationExcluded:r.excluded,dashboardCostJpy:r.cost,dashboardPurchaseJpy:r.purchaseCost,actualSalePriceJpy:r.saleAmount}));
w.setBusinessState(w.businessDashboardDefaults(now));
w.renderBusinessDashboard();
assert.equal(w.document.querySelectorAll('.business-kpi').length,8);assert.equal(w.document.querySelectorAll('.business-composition').length,2);
w.document.querySelector('.business-expand').click();assert.ok(w.document.querySelector('.business-child'));
w.businessDashboardSecondary('buyer');w.businessDashboardExpand(0);assert.ok(w.document.querySelector('.business-child .business-not-applicable'));
w.document.getElementById('business-buyer').value='X';w.businessDashboardApply();
assert.match(w.document.querySelector('[data-business-metric=salesCount]').textContent,/1件/);
assert.match(w.document.querySelector('[data-business-metric=purchaseCount]').textContent,/3件/);
w.document.getElementById('business-from').value='2027-01';w.businessDashboardApply();assert.ok(w.document.getElementById('business-filter-error').textContent);
w.businessDashboardPreset('half');assert.equal(w.getBusinessState().from,w.businessMonthShift(w.businessMonthNow(),-5));
w.businessDashboardReset();assert.equal(w.document.getElementById('business-buyer').value,'');
w.APP_DATA.staffRecords[0].name='<img src=x onerror="alert(1)">';w.renderBusinessDashboard();assert.equal(w.document.querySelector('img'),null);
w.APP_DATA.inventory=[];w.renderBusinessDashboard();assert.equal(w.document.querySelectorAll('.business-donut').length,0);
dom.window.close();console.log('Dashboard aggregation and interaction tests passed.');
