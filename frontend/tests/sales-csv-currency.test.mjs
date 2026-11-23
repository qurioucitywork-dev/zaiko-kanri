import {installCsvRuntime} from './slip-csv-test-runtime.mjs';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFile} from 'node:fs/promises';
const source=await readFile(process.argv[2] || new URL('../public/admin-reference/js/app.js',import.meta.url),'utf8');
const bridge=await readFile(process.argv[3] || new URL('../public/admin-reference/js/api_bridge.js',import.meta.url),'utf8');
let blob,records=[],clicks=0;
const ctx=vm.createContext({
 APP_DATA:{},state:{latestRate:200},statusLabel:{confirmed:'確定'},allAccessoryMasterRecords:()=>[],
 currentSlipTab:'sales',_slSelectedIds:new Set(),getFilteredSlipData:()=>records,getBuyerName:()=> '販売先A',getSalesUsdRate:()=>200,
 Blob,URL:{createObjectURL:b=>{blob=b;return 'blob:test';}},document:{createElement:()=>({click(){clicks++;}})},showToast:()=>{},
});
const load=(text,start,end)=>{const begin=text.indexOf(start),finish=text.indexOf(end,begin);assert.ok(begin>=0 && finish>begin);vm.runInContext(text.slice(begin,finish),ctx);};
load(bridge,'  function saleView(', '  function shipmentView(');
load(source,'function getSalesRecordCurrency(', 'function formatSalesIssuedAt(');
load(source,'function getSalesCsvLinePrice(', 'function getFilteredSlipData(');
installCsvRuntime(source,ctx,()=>records,()=>ctx._slSelectedIds);
const apiSale=(id,currency,amounts)=>({id,slipNumber:id,saleDate:'2026-09-17',buyerCode:'B1',status:'confirmed',displayCurrency:currency,
 fxRateScaled:currency==='HKD'?20000000:155000000,fxScale:1000000,notes:'確認, "済"',
 lines:amounts.map((amount,i)=>({id:id+i,productCode:id+i,brand:'ROLEX',modelNumber:'モデル',saleCurrency:currency,unitPriceMinor:amount}))});
const jpy=ctx.saleView(apiSale('UD-JPY','JPY',[1234567,0]));
const usd=ctx.saleView(apiSale('UD-USD','USD',[12345]));
const hkd=ctx.saleView(apiSale('UD-HKD','HKD',[9800]));
const revision=ctx.saleView({...apiSale('UR-JPY','JPY',[]),documentType:'sales_revision',adjustmentDirection:'decrease',
 lines:[{productCode:'R1',brand:'ROLEX',modelNumber:'修正',direction:'decrease',adjustmentAmountMinor:555}]});
records=[jpy,usd,hkd,revision];
const before=JSON.stringify(records);
const parse=text=>text.replace(/^\uFEFF/,'').split(/\r?\n/).map(line=>line.slice(1,-1).split(/","/).map(value=>value.replace(/""/g,'"')));
ctx.exportSlipCSV();
const rows=parse(await blob.text());
assert.equal(rows.length,6,'retain one product per row');
assert.deepEqual(rows[0],['伝票番号','売上日','販売先','管理番号','ブランド','モデル','売価（JPY）','通貨','備考','修正回数']);
assert.ok(rows.every(row=>row.length===10));
assert.equal(jpy.items[0].salePrice,7965,'fixture uses actual API USD conversion');
assert.equal(rows[1][6],'1234567','preserve original JPY exactly, without a rounded USD round trip');
assert.equal(rows[1][7],'JPY');assert.equal(rows[2][6],'0');
assert.equal(rows[3][6],'1913475');assert.equal(rows[3][7],'JPY');
assert.equal(rows[4][6],'196000');assert.equal(rows[4][7],'JPY');
assert.equal(rows[5][6],'-555');assert.equal(rows[5][7],'JPY');
assert.equal(rows[1][8],'確認, "済"');
assert.equal(JSON.stringify(records),before,'no saved data changes');
assert.deepEqual([...new Uint8Array(await blob.arrayBuffer()).slice(0,3)],[239,187,191]);
ctx._slSelectedIds.add('UD-JPY');ctx.exportSlipCSV();assert.equal(parse(await blob.text()).length,3,'selection preserved');
ctx._slSelectedIds.clear();records=[usd];ctx.exportSlipCSV();assert.equal(parse(await blob.text()).length,2,'filtering preserved');
const oldJPY=ctx.getSalesCsvLinePrice({salePrice:100},{inputCurrency:'JPY',usdJpyRate:150});
assert.equal(oldJPY.amount,15000,'legacy JPY uses stored rate');assert.equal(oldJPY.currency,'JPY');
assert.equal(ctx.getSalesCsvLinePrice({salePrice:99,inputAmount:0,inputCurrency:'JPY'},{}).amount,0);
assert.equal(ctx.getSalesCsvLinePrice({salePrice:99,inputAmount:'',inputCurrency:'JPY'},{usdJpyRate:150}).amount,14850);
assert.equal(clicks,3);
console.log('PASS: sales CSV preserves original JPY/USD/HKD amounts, exact JPY and zero/negative values; labels currencies and preserves product rows, selection and filtering.');
