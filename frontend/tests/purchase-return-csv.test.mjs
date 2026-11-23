import {installCsvRuntime} from './slip-csv-test-runtime.mjs';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFile} from 'node:fs/promises';
const source=await readFile(process.argv[2] || new URL('../public/admin-reference/js/app.js',import.meta.url),'utf8');
const bridge=await readFile(process.argv[3] || new URL('../public/admin-reference/js/api_bridge.js',import.meta.url),'utf8');
const body={},filters={};let blob,clicks=0,toast;
const ctx=vm.createContext({
 APP_DATA:{sales:[{id:'UD-WRONG',items:[{code:'UNRELATED',salePrice:6400}]}],purchaseSlips:[],purchaseReturns:[],inventory:[]},
 statusLabel:{confirmed:'確定'},allAccessoryMasterRecords:()=>[],
 currentSlipTab:'purchasereturn',_prSortDir:0,_slipFilterState:{executed:true,showAll:false},
 getSupplierName:id=>id==='A'?'仕入先A':'仕入先B',formatPrice:v=>`JPY ${v}`,updateSlipCsvControls:()=>{},
 matchesSlipStatusFilter:(r,s,type)=>{assert.equal(type,'purchasereturn');return !s || (r.withdrawn?'withdrawn':r.trackingConfirmed?'completed':'processing')===s;},
 Blob,URL:{createObjectURL:b=>{blob=b;return 'blob:test';}},
 document:{getElementById:id=>id==='slipListBody'?body:filters[id],createElement:()=>({click(){clicks++;}})},showToast:(...args)=>{toast=args;},
});
const load=(text,start,end)=>{const a=text.indexOf(start),b=text.indexOf(end,a);assert.ok(a>=0&&b>a,start);vm.runInContext(text.slice(a,b),ctx);};
load(bridge,'  function purchaseView(', '  function findSourceSaleForReturn(');
load(source,'function getSalesCsvLinePrice(', 'function openCostAdjustmentSlipDetail(');
load(source,'function getPurchaseReturnOriginalAmountInfo(', 'let _purchaseReturnWithdrawId');
load(source,'function getPurchaseSlipCurrency(', 'function formatPurchaseSlipAmount(');
load(source,'function _prSortedData(', '// ── 仕入返品伝票発行確認モーダル');
load(source,'function filterSlipList(', '// ── フィルタリセット');
installCsvRuntime(source,ctx,()=>ctx.getFilteredSlipData());
const snapshot=source.match(/  tbody\.purchaseReturnCsvRecords =[^;]+;/)?.[0];assert.ok(snapshot);
ctx.renderSlipList=data=>{ctx.data=data;ctx.tbody=body;vm.runInContext(snapshot,ctx);};
const purchase=(id,currency,amounts)=>ctx.purchaseView({id,slipNumber:id,purchaseDate:'2026-09-17',supplierCode:'A',status:'confirmed',purchaseTaxMode:currency==='JPY'?'domestic':'overseas',
 lines:amounts.map((amount,i)=>({id:id+i,productCode:id+i,brandName:'ROLEX',modelNumber:'モデル',quantity:1,unitCostMinor:amount,costCurrency:currency,fxRateScaled:currency==='HKD'?20000000:155000000,fxScale:1000000,baseSalePriceMinor:999999,baseSaleCurrency:'USD'}))},new Map(),200,[]);
const jpy=purchase('SD-JPY','JPY',[1234567,0,800]),usd=purchase('SK-USD','USD',[12345]),hkd=purchase('SK-HKD','HKD',[9800]);
ctx.APP_DATA.purchaseSlips=[jpy,usd,hkd];
ctx.APP_DATA.inventory=[{code:'SD-JPY0',purchasePrice:7777777}];
const ret=(id,src,indices,supplier='A')=>({id,date:'2026-09-18',slipId:src.id,supplier,reason:'返品理由',note:'確認, "済"',trackingConfirmed:true,trackingNo:'TRACK-123',
 items:indices.map(i=>({code:src.lines[i].code,purchasePrice:99999,salePrice:6400}))});
const jpyRet=ret('SH-JPY',jpy,[0,1]);
const usdRet={...ret('SH-USD',usd,[0],'B'),trackingConfirmed:false};
const hkdRet={...ret('SH-HKD',hkd,[0]),withdrawn:true};
ctx.APP_DATA.purchaseReturns=[jpyRet,usdRet,hkdRet];
const before=JSON.stringify(ctx.APP_DATA);
const parse=text=>text.replace(/^\uFEFF/,'').split(/\r?\n/).map(line=>line.slice(1,-1).split(/","/).map(v=>v.replace(/""/g,'"')));
const exportRows=async()=>{ctx.filterSlipList();ctx.exportSlipCSV();return parse(await blob.text());};
let rows=await exportRows();
assert.deepEqual(rows[0],['仕入返品伝票No','返品日','元仕入伝票No','仕入先','管理番号','ブランド','モデル','仕入金額（JPY）','通貨','返品理由','備考','ステータス','配送番号']);
assert.equal(rows.length,5,'export only returned products, one product per row');assert.ok(rows.every(r=>r.length===13));
assert.ok(rows.slice(1).every(r=>r[0].startsWith('SH-')),'ordinary sales must not be exported');
assert.equal(rows[1][2],'SD-JPY');assert.equal(rows[1][3],'仕入先A');assert.equal(rows[1][7],'1234567','source purchase amount wins over current inventory cost and sale price');
assert.equal(rows[1][8],'JPY');assert.equal(rows[2][7],'0');assert.equal(rows[1][10],'確認, "済"');assert.equal(rows[1][11],'処理済');assert.equal(rows[1][12],'TRACK-123');
assert.equal(rows[3][7],'1913475');assert.equal(rows[3][8],'JPY');assert.equal(rows[3][11],'処理中');
assert.equal(rows[4][7],'196000');assert.equal(rows[4][8],'JPY');assert.equal(rows[4][11],'取り下げ済み');
assert.deepEqual([...new Uint8Array(await blob.arrayBuffer()).slice(0,3)],[239,187,191]);
filters['slip-filter-keyword']={value:'SH-JPY'};rows=await exportRows();assert.equal(rows.length,3);
filters['slip-filter-keyword'].value='';filters['slip-filter-party']={value:'B'};rows=await exportRows();assert.equal(rows.length,2);assert.equal(rows[1][0],'SH-USD');
filters['slip-filter-party'].value='';filters['slip-filter-status']={value:'withdrawn'};rows=await exportRows();assert.equal(rows.length,2);assert.equal(rows[1][0],'SH-HKD');
filters['slip-filter-status'].value='';ctx._prSortDir=-1;rows=await exportRows();assert.equal(rows[1][3],'仕入先B','supplier order matches displayed list');
filters['slip-filter-from']={value:'2026-09-19'};ctx.exportSlipCSV();assert.equal(parse(await blob.text()).length,5,'unedited displayed results still exported until next search');
ctx.filterSlipList();const prior=clicks;ctx.exportSlipCSV();assert.equal(clicks,prior,'empty result must not fall back to sales');assert.equal(toast[0],'info');
ctx._slipFilterState.executed=false;ctx.filterSlipList();ctx.exportSlipCSV();assert.equal(clicks,prior);
ctx.currentSlipTab='sales';ctx.renderSlipList(ctx.APP_DATA.sales);assert.equal(body.purchaseReturnCsvRecords,null,'tab change clears snapshot');
assert.equal(JSON.stringify(ctx.APP_DATA),before,'business records unchanged');
console.log('PASS: purchase return CSV references displayed returns and source purchases; exact JPY/USD/HKD costs, zero, returned items, filters, sorting, status, tracking and blank-list guard.');
