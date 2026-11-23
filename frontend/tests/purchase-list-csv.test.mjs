import {installCsvRuntime} from './slip-csv-test-runtime.mjs';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import vm from 'node:vm';

const source = await readFile(process.argv[2] || new URL('../public/admin-reference/js/app.js', import.meta.url), 'utf8');
let downloaded, clicked = 0, toast;
const body = {purchaseCsvRows:null};
const context = vm.createContext({
  APP_DATA:{inventory:[],fxRateHistory:[{code:'USD',rate:150,observedAt:'2026-01-01'},{code:'HKD',rate:20,observedAt:'2026-01-01'}],fxRates:[{code:'USD',rate:200}]},
  PU_TAX_CATEGORY_CONSUMPTION:'consumption', PU_TAX_CATEGORY_EQUIVALENT:'equivalent', PU_TAX_CATEGORY_OUT_OF_SCOPE:'out_of_scope',
  puNormalizeTaxCategory:value=>value, normalizeInventoryStatusLabel:value=>value,
  getSupplierName:code=>code==='S1'?'仕入先A':'', getInventoryUsdRate:()=>200,
  currentSlipTab:'purchase', getFilteredSlipData:()=>{throw Error('Must export the rendered snapshot, not unapplied filters');},
  Blob, URL:{createObjectURL:blob=>{downloaded=blob;return 'blob:test';},revokeObjectURL:()=>{}},
  document:{getElementById:id=>id==='slipListBody'?body:null,body:{appendChild:()=>{}},createElement:()=>({click(){clicked++;},remove(){}})},
  setTimeout:fn=>fn(), getLocalDateISO:()=> '2026-10-11', showToast:(...args)=>{toast=args;},
});
function load(start,end) {
  const begin=source.indexOf(start), finish=source.indexOf(end,begin);
  assert.ok(begin>=0 && finish>begin,`Missing source ${start}`);
  vm.runInContext(source.slice(begin,finish),context);
}
load('function isPendingPurchaseArrivalStatus(', 'function renderPurchaseSlipStatusBadges(');
load('function getPurchaseSlipTaxSummary(', 'function canIssuePurchaseSlip(');
load('function getPurchasePaymentMethodLabel(', '// Resolve master-backed');
load('function buildPurchaseListCsvRows(', 'function getFilteredSlipData(');
installCsvRuntime(source,context,()=>(body.purchaseCsvRows || []).slice(1).map(row=>({id:row[0]})));
const domestic={id:'SD-DOMESTIC',date:'2026-09-17',supplier:'S1',staff:'浜田',paymentMethod:'cash',purchaseCurrency:'JPY',
  note:'修正理由: "確認",\n長い備考を省略せず出力する',issuedAt:'2026-09-16T16:30:00Z',
  lines:[{purchasePrice:100001,salePrice:1000,currentStatus:'仕入中'},{purchasePrice:200009,salePrice:2000,currentStatus:'在庫中'}]};
const overseas={id:'SD-OVERSEAS',date:'2026-09-18',supplierName:'海外業者',purchaseTaxMode:'overseas',purchaseCurrency:'USD',
  registrationPurchaseCurrency:'USD',registrationPurchaseJpyRate:140,paidAt:'2026-09-18T01:00:00Z',revisions:[{}],
  lines:[{purchaseCurrency:'USD',purchasePrice:100,salePrice:200,convertedPurchasePriceJpy:14000,currentStatus:'在庫中'}]};
const input=[domestic,overseas], before=JSON.stringify(input);
let rows=context.buildPurchaseListCsvRows(input);
const records=rows.slice(1).map(row=>Object.fromEntries(rows[0].map((key,i)=>[key,row[i]])));
assert.equal(rows.length,3,'one row per slip even when it contains multiple products');
assert.equal(rows[0].length,22);
assert.ok(rows.every(row=>row.length===22 && row.every(value=>value!==undefined && value!==null)));
assert.equal(records[0]['仕入伝票No'],'SD-OVERSEAS','same descending date order as list');
assert.equal(records[0]['原価（税抜・JPY）'],14000,'use registered FX, not current rate');
assert.equal(records[0]['仕入先'],'海外業者');
assert.equal(records[0]['伝票ステータス'],'処理済');
assert.equal(records[0]['支払確認'],'支払済');
assert.equal(records[1]['原価（税抜・JPY）'],300010);
assert.equal(records[1]['原価（税込・JPY）'],330010,'tax is truncated separately per line');
assert.equal(records[1]['売価合計（JPY）'],450000);
assert.equal(records[1]['明細点数'],2);
assert.equal(records[1]['備考'],domestic.note);
assert.equal(records[1]['伝票ステータス'],'処理中 / 未払い');
assert.equal(records[1]['入荷待ち点数'],1);
assert.equal(records[1]['発行日時'],'2026/09/17 01:30:00');
assert.equal(JSON.stringify(input),before,'export does not change saved records');
body.purchaseCsvRows=rows;
vm.runInContext("_purchaseListCurrency.cost='USD'; _purchaseListCurrency.sale='JPY';",context);
context.exportSlipSummaryCSV();
let text=await downloaded.text();
assert.ok(text.includes('原価（税抜・JPY）'),'export preserves displayed currency until rerender');
assert.ok(text.includes('修正理由: ""確認"",\n長い備考を省略せず出力する'));
assert.ok(!text.includes('undefined'));
assert.deepEqual([...new Uint8Array(await downloaded.arrayBuffer()).slice(0,3)],[239,187,191]);
rows=context.buildPurchaseListCsvRows([overseas]);
const converted=Object.fromEntries(rows[0].map((key,i)=>[key,rows[1][i]]));
assert.equal(converted['原価（税抜・JPY）'],14000);
assert.equal(converted['売価合計（JPY）'],28000);
rows=context.buildPurchaseListCsvRows([{...overseas,note:'=1+1'}]);
context.downloadPurchaseListCsv(rows);
assert.ok((await downloaded.text()).includes("\"'=1+1\""));
body.purchaseCsvRows=context.buildPurchaseListCsvRows([]);
const priorClicks=clicked;
context.exportSlipSummaryCSV();
assert.equal(clicked,priorClicks,'empty displayed results must not export other slips');
assert.equal(toast[0],'info');
console.log('PASS: purchase CSV rows, totals, tax rounding, historical FX, status, Tokyo dates, displayed snapshot, CSV escaping and empty results.');
