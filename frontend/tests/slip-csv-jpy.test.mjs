import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';
const source=fs.readFileSync(process.argv[2] || new URL('../public/admin-reference/js/app.js',import.meta.url),'utf8');
let downloads=0,toast,downloaded;
const ctx=vm.createContext({APP_DATA:{fxRateHistory:[
  {code:'USD',rate:140,observedAt:'2026-01-01'},
  {code:'USD',rate:180,observedAt:'2026-09-02'},
  {code:'USD',rate:999,observedAt:'2026-10-01'},
],fxRates:[{code:'USD',rate:999}]},
 getSalesUsdRate:()=>{throw Error('CSV must never use current FX');},
 getPurchaseSlipRate:()=>{throw Error('CSV must never use synthesized FX');},
 getBuyerName:x=>x,getSupplierName:x=>x,getPurchaseSlipCurrency:r=>r.purchaseCurrency || 'JPY',
 getPurchaseSlipTaxSummary:()=>({rateBasisPoints:1000,modeLabel:'国内',taxLabel:'課税'}),
 getPurchaseSlipStatusKeys:()=>[],isPendingPurchaseArrivalStatus:()=>false,getPurchasePaymentMethodLabel:()=>'',
 getCurrentPurchaseLineDetail:l=>l.productDetail || l,getShipmentProcessingStatus:()=> '処理中',
 getConsignmentProcessingStatus:()=> '処理中',getSalesReturnProcessingStatus:()=> '処理済',getPurchaseReturnProcessingStatus:()=> '処理済',
 _getReturnTrackingNumber:()=>'',getSlipStatementProduct:i=>({...i,accessories:[]}),getDocumentMasterName:(_,v)=>v || '',
 isCostAdjustmentRecord:r=>r._adjustmentType==='cost',isPurchaseAdjustmentRecord:r=>r._adjustmentType==='purchase',
 getAdjustmentRecordLabel:()=>'',getAdjustmentRecordTarget:()=>'',getAdjustmentRecordAmount:r=>r.adjustedCostJpyMinor ?? r.adjustmentAmount,
 getLocalDateISO:()=> '2026-10-13',currentSlipTab:'sales',Blob,setTimeout:fn=>fn(),
 URL:{createObjectURL:blob=>{downloads++;downloaded=blob;return 'blob:test';},revokeObjectURL(){}},
 document:{body:{appendChild(){}},createElement:()=>({click(){},remove(){}})},showToast:(...args)=>{toast=args;},
});
const load=(start,end)=>{let a=source.indexOf(start),b=source.indexOf(end,a);assert.ok(a>=0&&b>a,start);vm.runInContext(source.slice(a,b),ctx);};
load('function slipCsvNumber(', 'function captureSlipCsvRecords(');
load('function downloadSlipCsvRows(', 'function getFilteredSlipData(');
load('function getPurchaseReturnOriginalAmountInfo(', 'function _isReturnTrackingConfirmed(');
load('function getSalesReturnOriginalAmountInfo(', '// ── 売上返品詳細モーダル');
ctx.formatPrice=String;ctx.formatSalePrice=String;
const frozen={apiManaged:true,inputCurrency:'USD',usdJpyRate:999,fxRateScaled:155500000,fxScale:1000000,createdAt:'2026-09-01T10:00:00Z'};
const sale={...frozen,id:'UD-1',items:[{code:'P1',inputAmount:101,inputCurrency:'USD',salePrice:101,total:101}],totalJpy:15706};
assert.deepEqual([...ctx.buildSlipSummaryCsvRows('sales',[sale])[1].slice(4,8)],[15706,0,15706,'JPY']);
assert.equal(ctx.buildSlipProductCsvRows('sales',[sale])[1][6],15706);
// Preserve an exact entered JPY value despite the view's rounded internal USD value.
assert.equal(ctx.getSalesCsvLinePrice({inputAmount:1234567,inputCurrency:'JPY',salePrice:7939},frozen).amount,1234567);
assert.equal(ctx.getSalesCsvLinePrice({inputAmount:0,inputCurrency:'USD',salePrice:999},{}).amount,0);
assert.equal(ctx.getSlipCsvJPY(-1,'USD',frozen),-156,'half yen decrease rounds symmetrically');
const legacy={date:'2026-09-30',createdAt:'2026-09-01T10:00:00Z',apiManaged:true,inputCurrency:'USD',usdJpyRate:999};
assert.equal(ctx.getSlipCsvJPY(100,'USD',legacy),14000,'creation timestamp precedes later history and transaction date');
assert.equal(ctx.getSlipCsvJPY(100,'USD',{...legacy,registrationUsdJpyRate:999}),14000,'a view fallback must not become a CSV snapshot');
assert.equal(ctx.getSlipCsvJPY(100,'USD',{apiManaged:true,csvRegistrationUsdJpyRate:155,registrationUsdJpyRate:999}),15500,'server history is available outside the latest 100 UI rates');
const missing={id:'MISSING',apiManaged:true,inputCurrency:'USD',usdJpyRate:999,items:[{inputAmount:100,inputCurrency:'USD'}]};
assert.equal(ctx.getSalesCsvLinePrice(missing.items[0],missing).amount,null,'missing snapshot cannot use display fallback');
ctx.downloadSlipCsvRows(ctx.buildSlipProductCsvRows('sales',[missing]),'products');
assert.equal(downloads,1);assert.match(toast[2],/MISSING/);
assert.match(await downloaded.text(),/換算注記/);
assert.match(await downloaded.text(),/起票時の換算情報なし：売価（JPY）/);
assert.ok(!(await downloaded.text()).includes('99900'),'never silently substitute the current rate');
const purchase={id:'SD-1',date:'2026-09-01',registeredAt:'2026-09-01T00:00:00Z',purchaseCurrency:'USD',lines:[
  {code:'P2',quantity:3,purchasePrice:101,purchaseCurrency:'USD',convertedPurchasePriceJpy:47117,purchaseFxRateScaled:155500000,purchaseFxScale:1000000,
    saleOriginalPrice:1234567,saleCurrency:'JPY',salePrice:999},
]};
assert.equal(ctx.buildSlipProductCsvRows('purchase',[purchase])[1][11],15706);
assert.deepEqual([...ctx.buildPurchaseListCsvRows([purchase])[1].slice(11,14)],[47117,51828,3703701]);
const shipment={...frozen,id:'SY-1',displayCurrency:'USD',taxRateBasisPoints:1000,items:[
  {code:'P3',salePriceUsd:100,convertedSalePriceJpy:16000}, {code:'P4',salePriceUsd:0,convertedSalePriceJpy:0},
]};
assert.deepEqual([...ctx.buildShippingSlipCsvRows([shipment])[1].slice(4,8)],[16000,1600,17600,'JPY']);
assert.equal(ctx.getShippingCsvJPY({salePriceUsd:100},frozen),16000);
const overseas=ctx.buildShippingOverseasProductCsvRows({...shipment,displayCurrency:'JPY'});
assert.equal(overseas[2][15],100);assert.equal(overseas[3][15],0);
assert.equal(overseas[1][15],'Selling price (USD)');assert.equal(overseas[2][16],'P3');
assert.ok(!overseas[1].some(h=>h.startsWith('Selling price (JPY')));
assert.equal(ctx.buildSlipSummaryCsvRows('consignment',[shipment])[1][4],16000);
const adjustment={...frozen,id:'UR-1',_adjustmentType:'sales',adjustmentDirection:'mixed',items:[
  {code:'P1',direction:'decrease',adjustmentAmount:101,convertedAdjustmentJpy:15706},
  {code:'P2',direction:'increase',adjustmentAmount:20,convertedAdjustmentJpy:3110},
]};
assert.equal(ctx.buildSlipSummaryCsvRows('adjustment',[adjustment])[1][5],-12596);
assert.equal(ctx.buildSlipProductCsvRows('adjustment',[adjustment])[1][7],-15706);
// Returns use their linked original transaction, even after prices/rates change.
ctx.APP_DATA.sales=[sale,{...sale,id:'UD-RESALE',items:[{code:'P1',inputAmount:9999,inputCurrency:'JPY'}]}];
ctx.APP_DATA.purchaseSlips=[purchase];ctx.APP_DATA.inventory=[{code:'P2',purchasePrice:9999999}];
const sr={id:'UH-1',slipId:'UD-1',items:[{code:'P1',salePrice:9999}]};
const pr={id:'SH-1',slipId:'SD-1',items:[{code:'P2',purchasePrice:9999}]};
assert.equal(ctx.buildSlipProductCsvRows('salesreturn',[sr])[1][7],15706);
assert.equal(ctx.buildSlipSummaryCsvRows('salesreturn',[sr])[1][5],15706);
assert.equal(ctx.buildSlipProductCsvRows('purchasereturn',[pr])[1][7],15706);
assert.equal(ctx.buildSlipSummaryCsvRows('purchasereturn',[pr])[1][5],15706);
console.log('PASS: frozen JPY conversions, source returns, current-rate independence, missing-rate guard, quantity, tax, negative adjustments and USD-only overseas CSV.');
