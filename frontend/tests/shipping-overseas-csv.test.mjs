import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import vm from 'node:vm';

const root = new URL('../public/admin-reference/js/', import.meta.url);
const source = await readFile(new URL('app.js', root), 'utf8');
const master = (code, name, overseasDetail) => ({code, name, overseasDetail,
  domesticDetail:'DOMESTIC_ONLY', tagDetail:'TAG_ONLY', inventoryDetail:'INVENTORY_ONLY'});
const data = {
  brandRecords:[master('0001','ブランド','Brand EN')], materials:[master('0001','素材','Material EN')],
  movements:[master('0001','駆動','Movement EN')], conditions:[master('0001','状態','Condition EN')],
  beltMaterialRecords:[master('0002','ジュビリー','Jubilee')],
  accessoryRecords:[master('0002','箱','Box'),master('0024','ギャラ2c年月','Guarantee(2c)'),
    master('0040','コマ','Bracelet Parts'),master('0001','RX箱','（表記無し）')],
  accessoryAliases:{'コマ（数字）':'コマ'},
  inventory:[{code:'00001',brand:'0001',model:'登録したモデル',ref:'00123',serial:'0009',
    material:'0001',dial:'Silver',belt:'0002',movement:'0001',weightGrams:0,condition:'0001',
    accessories:['箱','ギャラ2c年月','コマ（数字）','RX箱'],accessorySupplements:{'0024':'26/9','0040':'3'},
    note:'特徴, "確認"\n次の行',comment:'PRIVATE_COMMENT',purchasePrice:999999,salePrice:9999}],
};
let blob, fileName, toast, clicks=0;
const context = vm.createContext({APP_DATA:data,Blob,
  allAccessoryMasterRecords:()=>data.accessoryRecords,
  getBuyerName:()=> '登録取引先', getShipmentProcessingStatus:r=>r.status || '処理中',
  getSalesUsdRate:()=>200,getLocalDateISO:()=> '2026-10-12',showToast:(...args)=>{toast=args;},setTimeout:fn=>fn(),
  URL:{createObjectURL:b=>{blob=b;return 'blob:csv';},revokeObjectURL(){}},
  document:{body:{appendChild(){}},createElement:()=>({click(){fileName=this.download;clicks++;},remove(){}})},
});
for (const file of ['master_display.js','price_code.js','slip_statement.js']) {
  vm.runInContext(await readFile(new URL(file,root),'utf8'),context);
}
const load=(start,end)=>{
  const a=source.indexOf(start),b=source.indexOf(end,a);
  assert.ok(a>=0 && b>a,start);vm.runInContext(source.slice(a,b),context);
};
load('function slipCsvNumber(', '/** One row per displayed shipment');
load('function getDocumentMasterName(', 'function buildPurchaseRecordTemplateHTML(');
load('function getShippingSalePriceUSD(', 'function getShippingSaleTotalUSD(');
load('function buildShippingOverseasProductCsvRows(', 'function buildSlipSummaryCsvRows(');
load('function downloadSlipCsvRows(', 'function exportSlipSummaryCSV(');
const record={id:'SY-OVERSEAS',date:'2026-10-12',destination:'B1',displayCurrency:'JPY',usdJpyRate:155,
  note:'PRIVATE_SLIP_NOTE',items:[{code:'00001',salePriceUsd:100,convertedSalePriceJpy:16000},
    {code:'NO-INVENTORY',productDetail:{brand:'Legacy Brand',model:'未翻訳モデル',belt:'自由入力ベルト',
      accessories:[],note:'=FORMULA'},salePriceUsd:200}]};
const before=JSON.stringify({data,record});
const rows=context.buildShippingOverseasProductCsvRows(record);
const japaneseHeaders=['インボイス番号','出荷登録日','通し番号','ブランド','モデル名','型番','シリアル',
  '金性','文字盤','ベルト','コンディション','ムーブ','重量','付属品','コメント備考','売価','管理番号'];
const englishHeaders=['Invoice No.','Shipping date','No','Brand','Model','Reference No.','Serial No.',
  'Material','Dial','Belt','Condition','Movement','Weight (g)','Accessories','comment','Selling price (USD)','Management No.'];
assert.deepEqual([...rows[0]],japaneseHeaders,'Japanese template header');
assert.deepEqual([...rows[1]],englishHeaders,'English template header and exact order');
assert.equal(rows.length,4,'two headers followed by one row per shipment product');
assert.ok(rows.every(row=>row.length===rows[0].length));
const cells=rows.slice(2).map(row=>Object.fromEntries(rows[1].map((key,i)=>[key,row[i]])));
assert.deepEqual({...cells[0]}, {
  'Invoice No.':'SY-OVERSEAS','Shipping date':'2026/10/12',No:1,'Management No.':'00001',
  Brand:'Brand EN',Model:'登録したモデル','Reference No.':'00123','Serial No.':'0009',Material:'Material EN',
  Dial:'Silver',Belt:'Jubilee',Movement:'Movement EN','Weight (g)':0,
  Accessories:'Box / Guarantee(2c) 26/9 / Bracelet Parts 3 / （表記無し）',Condition:'Condition EN',
  'Selling price (USD)':100,comment:'特徴, "確認"\n次の行',
});
assert.equal(cells[1].No,2);
assert.equal(cells[1].Brand,'Legacy Brand');assert.equal(cells[1].Model,'未翻訳モデル');
assert.equal(cells[1].Belt,'自由入力ベルト');assert.equal(cells[1]['Selling price (USD)'],200);
assert.ok(!JSON.stringify(rows).includes('PRIVATE_'),'never include internal comments or slip revision notes');
assert.ok(!JSON.stringify(rows).includes('_ONLY'),'do not use tag, domestic or inventory labels');
assert.equal(JSON.stringify({data,record}),before,'no changes to product/master/shipment records');
// Changes to overseas master labels apply to the next export without touching saved items.
data.beltMaterialRecords[0].overseasDetail='Updated Jubilee';
data.accessoryRecords[0].overseasDetail='Updated Box';
data.conditions[0].overseasDetail='Updated Condition';
const updated=context.buildShippingOverseasProductCsvRows({...record,status:'処理済',displayCurrency:'USD'});
assert.equal(updated[2][9],'Updated Jubilee');assert.match(updated[2][13],/^Updated Box/);
assert.equal(updated[2][10],'Updated Condition');assert.equal(updated[2][15],100);
context.downloadSlipCsvRows(rows,'overseas_products','shipping');
assert.equal(fileName,'slip_shipping_overseas_products_2026-10-12.csv');
assert.equal(clicks,1);
assert.match(toast[2],/^2行を出力/,'header rows do not count as products');
const csv=await blob.text();
assert.ok(csv.startsWith(japaneseHeaders.map(value=>'"'+value+'"').join(',')+'\r\n'+englishHeaders.map(value=>'"'+value+'"').join(',')));
assert.ok(csv.includes('"特徴, ""確認""\n次の行"'));assert.ok(csv.includes('"\'=FORMULA"'));
assert.deepEqual([...new Uint8Array(await blob.arrayBuffer()).slice(0,3)],[239,187,191]);
const empty=context.buildShippingOverseasProductCsvRows({items:[]});
assert.equal(empty.length,2);
context.downloadSlipCsvRows(empty,'overseas_products','shipping');
assert.equal(clicks,1,'no download for a header-only shipment');
const missing=context.buildShippingOverseasProductCsvRows({id:'NO-RATE',apiManaged:true,date:'2026-09-09',
  displayCurrency:'JPY',items:[{code:'00001',convertedSalePriceJpy:15500}]});
assert.equal(missing[2][1],'2026/9/9');assert.equal(missing[2][2],1,'numbering restarts for each shipment');
assert.equal(missing[2][15],null,'missing historical rate cannot use the current rate');
context.downloadSlipCsvRows(missing,'overseas_products','shipping');
assert.equal(toast[0],'warn');assert.match(toast[2],/NO-RATE/);
const missingCsv=await blob.text();
assert.ok(!missingCsv.includes('Conversion note'),'missing FX cannot add columns to the supplied template');
assert.ok(missingCsv.includes('"","00001"'),'missing USD amount remains blank');
console.log('PASS: bilingual 17-column overseas template, numbering, dates, live master labels, USD amounts, privacy, empty/missing-rate guards and UTF-8 CSV escaping.');
