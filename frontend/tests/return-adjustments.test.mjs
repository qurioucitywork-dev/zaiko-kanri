import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {createRequire} from 'node:module';
import path from 'node:path';
import vm from 'node:vm';
const repo=process.argv[2] || path.resolve(import.meta.dirname,'..');
const stage=process.argv[3] || path.join(repo,'public/admin-reference/js');
const require=createRequire(path.join(repo,'package.json'));
const {JSDOM}=require('jsdom');
const html=await readFile(path.join(repo,'public/admin-reference/app.html'),'utf8');
const dom=new JSDOM(html,{runScripts:'outside-only',url:'http://localhost'}),w=dom.window,d=w.document;
const evaluate=source=>vm.runInContext(source,dom.getInternalVMContext());
const app=await readFile(path.join(stage,'app.js'),'utf8');
const bridge=await readFile(path.join(stage,'api_bridge.js'),'utf8');
const load=(source,start,end)=>{const a=source.indexOf(start),b=source.indexOf(end,a);assert.ok(a>=0&&b>a,start);evaluate(source.slice(a,b));};
evaluate(await readFile(path.join(stage,'return_adjustments.js'),'utf8'));
load(app,'let _purchaseReturnTargetCode','function _isReturnTrackingConfirmed');
load(app,'let _salesReturnTargetId','// ── 売上返品詳細モーダル');
load(app,'function _setRecordInventoryStatus','function syncApprovalRequestForBusinessRecord');
load(app,'function getSalesCsvLinePrice','function openCostAdjustmentSlipDetail');
load(app,'function slipCsvNumber','/** One row per displayed shipment');
load(app,'function buildSlipProductCsvRows','function getFilteredSlipData');
load(app,'function buildSlipSummaryCsvRows','/** Snapshot the displayed purchase slips');
load(bridge,'  function findSourceSaleForReturn','  function saleView');
Object.assign(w,{_escHtml:value=>String(value??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c])),
 showToast:()=>{},getLocalDateISO:()=> '2026-10-13',closeSlipDetail:()=>{},formatPrice:n=>`JPY ${n}`,formatSalePrice:n=>`USD ${n}`,
 getSupplierName:()=> '仕入先',getBuyerName:()=> '販売先',isBuyer:()=>false,isWorker:()=>false,refreshLinkedBusinessViews:()=>{},
 getPurchaseReturnProcessingStatus:()=> '処理中',getSalesReturnProcessingStatus:()=> '処理中',_getReturnTrackingNumber:()=>'',normalizeInventoryStatusLabel:x=>x,
 getSalesUsdRate:()=>999,getSalesRegistrationFXRate:s=>s.usdJpyRate,getPurchaseSlipCurrency:s=>s.purchaseCurrency || 'JPY'});
const source={id:'SD-TEST',supplier:'S',date:'2026-10-01',purchaseCurrency:'JPY',lines:[{code:'P1',purchasePrice:10000,productDetail:{brand:'ROLEX',model:'時計'}}]};
const sale={id:'UD-TEST',buyer:'B',date:'2026-10-01',inputCurrency:'JPY',usdJpyRate:155,items:[{code:'S1',inputAmount:310000,inputCurrency:'JPY',salePrice:2000,brand:'ROLEX',model:'時計'}]};
w.APP_DATA={purchaseSlips:[source],sales:[sale],inventory:[{code:'P1',status:'在庫中',purchasePrice:10000},{code:'S1',status:'売上済'},{code:'99999',status:'売上済',purchasePrice:777}],purchaseReturns:[],salesReturns:[]};
let saved=[];w.ZaikoAPI={saveReturn:async payload=>{saved.push(payload);return {record:{slipNumber:'RETURN'}};}};
const stockBefore=JSON.stringify(w.APP_DATA.inventory);
for(const prefix of ['pr','sr']){
 w[prefix==='pr'?'openPurchaseReturnModal':'openSalesReturnModal'](prefix==='pr'?source.id:sale.id);
 const add=prefix==='pr'?'prAddItemByCode':'srAddItemByCode';
 w[add]('99999');w[add]('99999');
 const list=d.getElementById(prefix==='pr'?'pr-selected-items-list':'salesReturnItemList');
 assert.equal(list.querySelectorAll('input[aria-label="項目名"]').length,2,'multiple financial lines allowed');
 w.updateReturnAdjustment(prefix,0,'name','送料 <確認>');w.updateReturnAdjustment(prefix,0,'amount','1000');
 w.updateReturnAdjustment(prefix,1,'name','値引き');w.updateReturnAdjustment(prefix,1,'amount','300');w.updateReturnAdjustment(prefix,1,'sign','-');
 assert.equal(d.getElementById(prefix+'-item-count-badge').textContent,'0点 ＋ 調整2行');
 assert.match(d.getElementById(prefix+'-total-badge').textContent,/700/);
 d.getElementById(prefix+'-return-note').value='テスト';
 await w[prefix==='pr'?'submitPurchaseReturn':'submitSalesReturn']();
 assert.deepEqual(Array.from(saved.at(-1).productCodes),[]);
 assert.deepEqual(JSON.parse(JSON.stringify(saved.at(-1).adjustmentLines)),[{name:'送料 <確認>',amountMinor:1000},{name:'値引き',amountMinor:-300}]);
 assert.equal(saved.at(-1).operationType,prefix==='pr'?'purchase_return':'return');
}
assert.equal(JSON.stringify(w.APP_DATA.inventory),stockBefore,'adding and submitting 99999 never changes inventory');
const details={currency:'JPY',taxMode:'standard',taxRateBasisPoints:1000,fxRateScaled:1,fxScale:1,sourceSaleSlipNumber:'UD-TEST',lines:[{name:'送料',amountMinor:1000},{name:'値引き',amountMinor:-300}]};
const adjustments=details.lines.map((line,index)=>w.returnAdjustmentView(details,line,index));
const pr={id:'SH-TEST',slipId:'SD-TEST',supplier:'S',items:[{code:'P1'},...adjustments],adjustmentDetails:details};
const sr={id:'UH-TEST',slipId:'UD-TEST',buyer:'B',items:[{code:'S1'},...adjustments],adjustmentDetails:details};
assert.equal(w.getPurchaseReturnOriginalAmountInfo(pr).subtotal,10700);
assert.equal(w.getSalesReturnOriginalAmountInfo(sr).subtotal,310700);
assert.equal(w.getSalesReturnOriginalAmountInfo(sr).taxAmount,31070);
for(const [type,record] of [['purchasereturn',pr],['salesreturn',sr]]){
 const rows=w.buildSlipProductCsvRows(type,[record]);assert.equal(rows.length,4);assert.equal(rows[2][4],'99999');assert.equal(rows[2][6],'送料');assert.equal(rows[2][7],1000);assert.equal(rows[3][7],-300);
 const summary=w.buildSlipSummaryCsvRows(type,[record]);assert.equal(summary[1][4],1,'CSV product count excludes adjustments');assert.equal(summary[1][5],type==='purchasereturn'?10700:310700);
}
const usdDetails={...details,currency:'USD',taxMode:'out_of_scope',taxRateBasisPoints:0,fxRateScaled:155,fxScale:1};
const usd={...sr,items:usdDetails.lines.map((line,index)=>w.returnAdjustmentView(usdDetails,line,index)),adjustmentDetails:usdDetails};
assert.equal(w.getSalesReturnOriginalAmountInfo(usd).taxAmount,0);
assert.equal(w.buildSlipProductCsvRows('salesreturn',[usd])[1][7],155000);
assert.equal(w.buildSlipProductCsvRows('salesreturn',[usd])[2][7],-46500);
assert.equal(w.findSourceSaleForReturn({operationType:'return',lines:[],adjustmentDetails:details},[{slipNumber:'UD-TEST'}]).slipNumber,'UD-TEST');
w._setRecordInventoryStatus({items:adjustments},'在庫中');w.applyBusinessRecordState('salesreturn',{slipId:'UD-TEST',items:adjustments});assert.equal(JSON.stringify(w.APP_DATA.inventory),stockBefore);
// Statement uses the entered name, never resolves product 99999 or prints a cost code.
evaluate(await readFile(path.join(stage,'slip_statement.js'),'utf8'));
const statement=w.slipStatementRow({kind:'adjustment',code:'99999',itemName:'送料',amount:1000},'domestic');
assert.equal(statement.columns[1][0],'送料');assert.deepEqual(Array.from(statement.columns[4]),['99999']);
w.openPurchaseReturnModal('SD-TEST');w.prAddItemByCode('99999');assert.equal(w.getReturnAdjustmentPayload('pr'),null,'empty name and amount rejected');
w.updateReturnAdjustment('pr',0,'name','<img src=x>');w.updateReturnAdjustment('pr',0,'amount','-1');assert.equal(w.getReturnAdjustmentPayload('pr'),null,'sign must be selected separately');
w.updateReturnAdjustment('pr',0,'amount','10');w._prRenderAddedItems();assert.equal(d.querySelectorAll('#pr-selected-items-list img').length,0,'names escaped in editable rows');
console.log('PASS: 99999 entry for both returns, multiple signed lines, validation, non-inventory counts, original currency/tax, fixed FX CSV, statement and inventory isolation.');
dom.window.close();

const fractionalTax={...pr,items:[w.returnAdjustmentView(details,{name:'端数を含む減額',amountMinor:-1001},0)]};
assert.equal(w.getPurchaseReturnOriginalAmountInfo(fractionalTax).taxAmount,-101,'signed tax follows existing invoice rounding');
load(app,'function buildPurchaseReturnRecordTemplateHTML','function openPurchaseReturnInvoice');
load(app,'function buildSalesReturnRecordTemplateHTML','function openSalesReturnInvoice');
w.buildTemplateStyleSlipDocument=options=>options;
for(const [purchase,record] of [[true,{...pr,items:adjustments}],[false,usd]]){
 const doc=w[purchase?'buildPurchaseReturnRecordTemplateHTML':'buildSalesReturnRecordTemplateHTML'](record);
 assert.equal(doc.items.length,2);assert.equal(doc.items[0].code,'99999');assert.equal(doc.items[1].amount,-300);
 assert.equal(doc.currencyLabel,purchase?'JPY':'USD');assert.equal(doc.items[0].taxAmount,purchase?100:0);
}
console.log('PASS: return document currency, signed adjustment rows and consistent tax rounding.');
