import {installCsvRuntime} from './slip-csv-test-runtime.mjs';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFile} from 'node:fs/promises';
const source=await readFile(process.argv[2] || new URL('../public/admin-reference/js/app.js',import.meta.url),'utf8');
let blob,clicks=0,fileName,toast,buttonHidden;
const body={shippingCsvRows:null},label={textContent:''};
const ctx=vm.createContext({
 APP_DATA:{inventory:[]},currentSlipTab:'shipping',_shSelectedIds:new Set(),
 normalizeInventoryStatusLabel:s=>s,getBuyerName:()=> '販売先A',getSalesUsdRate:()=>200,
 Blob,URL:{createObjectURL:b=>{blob=b;return 'blob:test';},revokeObjectURL:()=>{}},setTimeout:fn=>fn(),getLocalDateISO:()=> '2026-10-11',
 document:{getElementById:id=>({slipListBody:body,slipCsvButtonLabel:label,shippingSlipCsvButton:{classList:{toggle:(name,value)=>{buttonHidden=value;}}}}[id]),
  body:{appendChild:()=>{}},createElement:()=>({click(){clicks++;fileName=this.download;},remove(){}})},showToast:(...args)=>{toast=args;},
});
const load=(start,end)=>{const begin=source.indexOf(start),finish=source.indexOf(end,begin);assert.ok(begin>=0 && finish>begin);vm.runInContext(source.slice(begin,finish),ctx);};
load('function getShipmentProcessingStatus(', '/** 委託伝票に含まれる商品の現在状態');
load('function getShippingSalePriceUSD(', 'function getShippingSaleTotalUSD(');
load('function getShippingTaxRate(', 'function formatShippingTotal(');
load('function buildShippingSlipCsvRows(', 'function captureSlipCsvRecords(');
installCsvRuntime(source,ctx,()=>input.filter(r=>(body.shippingCsvRows || []).slice(1).some(row=>row[0]===r.id)),()=>ctx._shSelectedIds);
const domestic={id:'SY-JPY',date:'2026-09-17',destination:'A',processingStatus:'completed',usdJpyRate:155,displayCurrency:'JPY',taxRateBasisPoints:1000,
 note:'備考全文, "確認"\n改行も保持',revisions:[{}],items:[{salePriceUsd:100,convertedSalePriceJpy:15501},{salePriceUsd:200,convertedSalePriceJpy:31009}]};
const overseas={id:'SY-USD',date:'2026-09-18',destination:'B',processingStatus:'processing',usdJpyRate:140,displayCurrency:'USD',taxRateBasisPoints:0,
 items:[{salePriceUsd:749,convertedSalePriceJpy:105000},{salePriceUsd:1357,convertedSalePriceJpy:190000}]};
const input=[domestic,overseas],before=JSON.stringify(input);
const rows=ctx.buildShippingSlipCsvRows(input);
assert.equal(rows.length,3,'one slip with multiple products must produce one row');
assert.equal(rows[0].length,13);
assert.ok(rows.every(row=>row.length===13 && row.every(value=>value!=null)));
const object=row=>Object.fromEntries(rows[0].map((key,index)=>[key,row[index]]));
const usd=object(rows[1]),jpy=object(rows[2]);
assert.equal(usd['出荷伝票No'],'SY-USD','same descending date order as list');
assert.equal(usd['点数'],2);assert.equal(usd['合計金額（JPY）'],295000);assert.equal(usd['表示通貨'],'JPY');
assert.equal(usd['消費税（JPY）'],0);assert.equal(usd['登録時USD/JPYレート'],140);assert.equal(usd['ステータス'],'処理中');
assert.equal(jpy['小計（税抜・JPY）'],46510);assert.equal(jpy['消費税（JPY）'],4650,'per-line rounding, not tax on subtotal');
assert.equal(jpy['合計金額（JPY）'],51160);assert.equal(jpy['税率（%）'],10);assert.equal(jpy['表示通貨'],'JPY');
assert.equal(jpy['備考'],domestic.note);assert.equal(jpy['ステータス'],'処理済');
assert.equal(JSON.stringify(input),before);
ctx.currentSlipTab='purchase';ctx.exportShippingSlipCSV();assert.equal(clicks,0);
ctx.currentSlipTab='shipping';body.shippingCsvRows=rows;
ctx.exportShippingSlipCSV();
assert.equal(fileName,'slip_shipping_summary_2026-10-11.csv');
assert.ok((await blob.text()).includes('備考全文, ""確認""\n改行も保持'));
assert.deepEqual([...new Uint8Array(await blob.arrayBuffer()).slice(0,3)],[239,187,191]);
ctx._shSelectedIds.add('SY-USD');ctx.exportShippingSlipCSV();
const selected=await blob.text();assert.ok(selected.includes('SY-USD'));assert.ok(!selected.includes('SY-JPY'));
ctx._shSelectedIds.clear();overseas.note='=1+1';body.shippingCsvRows=ctx.buildShippingSlipCsvRows([overseas]);
ctx.exportShippingSlipCSV();assert.ok((await blob.text()).includes("\"'=1+1\""));
body.shippingCsvRows=ctx.buildShippingSlipCsvRows([]);const prior=clicks;
ctx.exportShippingSlipCSV();assert.equal(clicks,prior,'empty filtered list must not fall back to all slips');assert.equal(toast[0],'info');
body.shippingCsvRows=null;ctx.exportShippingSlipCSV();assert.equal(clicks,prior);
console.log('PASS: shipment summary rows, amounts, tax rounding, currencies, status, snapshot selection, empty results and tab-specific buttons.');
