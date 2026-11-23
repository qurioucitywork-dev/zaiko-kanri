import {installCsvRuntime} from './slip-csv-test-runtime.mjs';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFile} from 'node:fs/promises';
const source=await readFile(process.argv[2] || new URL('../public/admin-reference/js/app.js',import.meta.url),'utf8');
let blob,clicks=0;
const records=[
 {id:'SY-OLD',date:'2026-09-17',destination:'A',processingStatus:'completed',status:'確定',usdJpyRate:155,displayCurrency:'JPY',note:'確認, "済"',revisions:[{}],items:[{code:'P1',brand:'ROLEX',model:'モデル1',salePriceUsd:100,convertedSalePriceJpy:15500},{code:'P2',brand:'OMEGA',model:'モデル2',salePriceUsd:200,convertedSalePriceJpy:31000}]},
 {id:'SY-NEW',date:'2026-09-18',destination:'A',processingStatus:'processing',status:'確定',usdJpyRate:155,displayCurrency:'USD',items:[{code:'P1',brand:'ROLEX',model:'モデル1',salePriceUsd:300,convertedSalePriceJpy:46500}]},
 {id:'SY-LEGACY',date:'2026-09-16',destination:'A',usdJpyRate:155,items:[{code:'LEGACY',currentStatus:'出荷中',salePrice:400,convertedSalePriceJpy:62000,brand:'',model:''}]},
];
const context=vm.createContext({
 APP_DATA:{inventory:[{code:'P1',status:'出荷済'},{code:'P2',status:'売上済'}]},
 currentSlipTab:'shipping',_shSelectedIds:new Set(),getFilteredSlipData:()=>records,
 normalizeInventoryStatusLabel:s=>s==='出荷中'?'出荷済':s,getBuyerName:()=> '販売先A',getSalesUsdRate:()=>200,
 Blob,URL:{createObjectURL:b=>{blob=b;return 'blob:test';}},document:{createElement:()=>({click(){clicks++;}})},showToast:()=>{},
});
const load=(start,end)=>{const begin=source.indexOf(start),finish=source.indexOf(end,begin);assert.ok(begin>=0 && finish>begin);vm.runInContext(source.slice(begin,finish),context);};
load('function getShipmentProcessingStatus(', '/** 委託伝票に含まれる商品の現在状態');
load('function getShippingSalePriceUSD(', 'function getShippingSaleTotalUSD(');
load('function exportSlipCSV()', 'function getFilteredSlipData()');
installCsvRuntime(source,context,()=>context.getFilteredSlipData(),()=>context._shSelectedIds);
const parse=text=>text.replace(/^\uFEFF/,'').split(/\r?\n/).map(line=>line.slice(1,-1).split(/","/).map(value=>value.replace(/""/g,'"')));
context.exportSlipCSV();
const rows=parse(await blob.text());
assert.equal(rows.length,5,'existing product-level rows must remain');
assert.deepEqual(rows[0],['伝票番号','出荷日','出荷先','管理番号','ブランド','モデル','売価（JPY）','表示通貨','登録時USD/JPYレート','備考','修正回数','ステータス']);
assert.ok(rows.every(row=>row.length===12));
assert.deepEqual(rows.slice(1).map(row=>[row[0],row[3],row[11]]),[['SY-OLD','P1','処理済'],['SY-OLD','P2','処理済'],['SY-NEW','P1','処理中'],['SY-LEGACY','LEGACY','処理中']]);
assert.deepEqual(rows[1].slice(0,11),['SY-OLD','2026-09-17','販売先A','P1','ROLEX','モデル1','15500','JPY','155','確認, "済"','1'],'existing columns unchanged');
assert.deepEqual([...new Uint8Array(await blob.arrayBuffer()).slice(0,3)],[239,187,191]);
context._shSelectedIds.add('SY-OLD');
context.exportSlipCSV();
assert.equal(parse(await blob.text()).length,3,'selected slip still exports all its products');
context._shSelectedIds.clear();
context.getFilteredSlipData=()=>[records[1]];
context.exportSlipCSV();
assert.equal(parse(await blob.text())[1][11],'処理中');
assert.equal(parse(await blob.text()).length,2,'filtered export unchanged');
assert.equal(clicks,3);
console.log('PASS: shipment CSV preserves product rows, existing columns, prices, selection and filtering; exports document-specific processing/completed statuses.');
