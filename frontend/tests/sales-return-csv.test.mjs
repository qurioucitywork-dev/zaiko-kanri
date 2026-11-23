import {installCsvRuntime} from './slip-csv-test-runtime.mjs';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFile} from 'node:fs/promises';
const source=await readFile(process.argv[2] || new URL('../public/admin-reference/js/app.js',import.meta.url),'utf8');
const bridge=await readFile(process.argv[3] || new URL('../public/admin-reference/js/api_bridge.js',import.meta.url),'utf8');
const body={},filters={};let blob,clicks=0,toast;
const ctx=vm.createContext({
 APP_DATA:{sales:[],salesReturns:[],inventory:[]},state:{latestRate:200},statusLabel:{confirmed:'確定'},allAccessoryMasterRecords:()=>[],
 currentSlipTab:'salesreturn',_slSelectedIds:new Set(['UNRELATED-SALE']),_slipFilterState:{executed:true,showAll:false},
 getBuyerName:id=>id==='A'?'販売先A':'販売先B',getSalesUsdRate:()=>200,
 formatPrice:v=>`JPY ${v}`,formatSalePrice:v=>`USD ${v}`,updateSlipCsvControls:()=>{},
 matchesSlipStatusFilter:(r,s,type)=>{assert.equal(type,'salesreturn');return !s || (r.trackingConfirmed?'completed':'processing')===s;},
 _isReturnTrackingConfirmed:r=>Boolean(r.trackingConfirmed),
 Blob,URL:{createObjectURL:b=>{blob=b;return 'blob:test';}},
 document:{getElementById:id=>id==='slipListBody'?body:filters[id],createElement:()=>({click(){clicks++;}})},
 showToast:(...args)=>{toast=args;},
});
const load=(text,start,end)=>{const begin=text.indexOf(start),finish=text.indexOf(end,begin);assert.ok(begin>=0 && finish>begin);vm.runInContext(text.slice(begin,finish),ctx);};
load(bridge,'  function saleView(', '  function shipmentView(');
load(source,'function getSalesRecordCurrency(', 'function formatSalesIssuedAt(');
load(source,'function getSalesCsvLinePrice(', 'function openCostAdjustmentSlipDetail(');
load(source,'function getSalesReturnOriginalAmountInfo(', '// ── 売上返品詳細モーダル');
load(source,'function getSalesReturnProcessingStatus(', 'function _isPurchaseReturnWithdrawn(');
load(source,'function filterSlipList(', '// ── フィルタリセット');
// Execute the real snapshot assignment from the renderer; no layout dependencies required.
installCsvRuntime(source,ctx,()=>ctx.getFilteredSlipData());
const snapshot=source.match(/  tbody\.salesReturnCsvRecords =[^;]+;/)?.[0];assert.ok(snapshot);
ctx.renderSlipList=data=>{ctx.data=data;ctx.tbody=body;vm.runInContext(snapshot,ctx);};
const sale=(id,currency,amounts)=>ctx.saleView({id,slipNumber:id,saleDate:'2026-09-17',buyerCode:'A',status:'confirmed',displayCurrency:currency,
 fxRateScaled:currency==='HKD'?20000000:155000000,fxScale:1000000,lines:amounts.map((amount,i)=>({id:id+i,productCode:id+i,brand:'ROLEX',modelNumber:'モデル',saleCurrency:currency,unitPriceMinor:amount}))});
const jpy=sale('UD-JPY','JPY',[1234567,0,800]),usd=sale('UD-USD','USD',[12345]),hkd=sale('UD-HKD','HKD',[9800]);
ctx.APP_DATA.sales=[jpy,usd,hkd,sale('UD-NOT-RETURNED','JPY',[999999])];
const ret=(id,date,sale,indices,buyer='A')=>({id,date,slipId:sale.id,buyer,reason:'返品理由',note:'確認, "済"',trackingConfirmed:true,
 items:indices.map(i=>({code:sale.items[i].code,salePrice:99999}))});
const jpyRet=ret('UH-JPY','2026-09-18',jpy,[0,1]);
const usdRet={...ret('UH-USD','2026-09-19',usd,[0],'B'),trackingConfirmed:false};
const hkdRet=ret('UH-HKD','2026-09-20',hkd,[0]);
ctx.APP_DATA.salesReturns=[jpyRet,usdRet,hkdRet];
const original=JSON.stringify(ctx.APP_DATA);
const parse=text=>text.replace(/^\uFEFF/,'').split(/\r?\n/).map(line=>line.slice(1,-1).split(/","/).map(value=>value.replace(/""/g,'"')));
const exportRows=async()=>{ctx.filterSlipList();ctx.exportSlipCSV();return parse(await blob.text());};
let rows=await exportRows();
assert.equal(rows.length,5,'only returned products, one product per row');
assert.deepEqual(rows[0],['売上返品伝票No','返品日','元売上伝票No','販売先','管理番号','ブランド','モデル','売価（JPY）','通貨','返品理由','備考','ステータス']);
assert.ok(rows.every(r=>r.length===12));
assert.ok(rows.slice(1).every(r=>r[0].startsWith('UH-')),'must not export ordinary sales');
const jpyRow=rows.find(r=>r[4]==='UD-JPY0');
assert.equal(jpy.items[0].salePrice,7965,'fixture uses actual API rounded USD conversion');
assert.equal(jpyRow[0],'UH-JPY');assert.equal(jpyRow[2],'UD-JPY');
assert.equal(jpyRow[1],'2026-09-18');assert.equal(jpyRow[7],'1234567','JPY remains exact');assert.equal(jpyRow[8],'JPY');
assert.equal(jpyRow[10],'確認, "済"');assert.equal(jpyRow[11],'処理済');
assert.equal(rows.find(r=>r[4]==='UD-JPY1')[7],'0');
assert.equal(rows.find(r=>r[0]==='UH-USD')[7],'1913475');assert.equal(rows.find(r=>r[0]==='UH-USD')[8],'JPY');
assert.equal(rows.find(r=>r[0]==='UH-USD')[11],'処理中');
assert.equal(rows[1][0],'UH-HKD','descending date matches the displayed list');assert.equal(rows[1][7],'196000');assert.equal(rows[1][8],'JPY');
assert.deepEqual([...new Uint8Array(await blob.arrayBuffer()).slice(0,3)],[239,187,191]);
filters['slip-filter-keyword']={value:'UD-JPY'};rows=await exportRows();assert.equal(rows.length,3,'source slip keyword');
filters['slip-filter-keyword'].value='';filters['slip-filter-party']={value:'B'};rows=await exportRows();assert.equal(rows.length,2);assert.equal(rows[1][0],'UH-USD');
filters['slip-filter-party'].value='';filters['slip-filter-status']={value:'processing'};rows=await exportRows();assert.equal(rows.length,2);assert.equal(rows[1][0],'UH-USD');
filters['slip-filter-status'].value='';filters['slip-filter-from']={value:'2026-09-18'};filters['slip-filter-to']={value:'2026-09-18'};
rows=await exportRows();assert.equal(rows.length,3);assert.equal(rows[1][0],'UH-JPY');
// Editing filter controls without searching must still export the displayed returns.
filters['slip-filter-party'].value='B';ctx.exportSlipCSV();assert.equal(parse(await blob.text()).length,3);
ctx.filterSlipList();const prior=clicks;ctx.exportSlipCSV();assert.equal(clicks,prior,'zero results cannot fall back to sales');assert.equal(toast[0],'info');
ctx._slipFilterState.executed=false;ctx.filterSlipList();ctx.exportSlipCSV();assert.equal(clicks,prior,'unsearched list cannot export hidden records');
ctx.currentSlipTab='sales';ctx.renderSlipList(ctx.APP_DATA.sales);assert.equal(body.salesReturnCsvRecords,null,'tab switch clears snapshot');
assert.equal(JSON.stringify(ctx.APP_DATA),original,'no changes to business data');
console.log('PASS: return CSV uses displayed return IDs and only returned products; exact original JPY/USD/HKD amounts, zero, original sale reference, filters, statuses, BOM and empty-list guard.');
