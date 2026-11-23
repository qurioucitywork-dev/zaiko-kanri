import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {JSDOM} from 'jsdom';
const source=await readFile(new URL('../public/admin-reference/js/app.js',import.meta.url),'utf8');
const html=await readFile(new URL('../public/admin-reference/app.html',import.meta.url),'utf8');
const dom=new JSDOM(html,{runScripts:'outside-only',url:'http://localhost/'});
const w=dom.window,d=w.document;
let blob,downloads=[],opened=0;
Object.assign(w,{
 currentSlipTab:'purchase',APP_DATA:{},Blob,getLocalDateISO:()=> '2026-10-11',showToast(){},
 _purchaseListCurrency:{cost:'JPY',sale:'USD'},_prSortedData:rows=>[...rows],
 getBuyerName:id=>id,getSupplierName:id=>id,getCurrentPurchaseLineDetail:line=>line.productDetail || line,
 getSlipStatementProduct:item=>({...item,accessories:[]}),getDocumentMasterName:(_type,value)=>value || '',
 getPurchaseSlipCurrency:r=>r.purchaseCurrency || 'JPY',getPurchaseSlipRate:()=>155,
 getPurchaseSlipTaxSummary:()=>({modeLabel:'国内',taxLabel:'税抜',rateBasisPoints:0}),
 getPurchaseListAmounts:r=>({costSubtotal:r.total,costGrandTotal:r.total,saleTotal:100,totals:{modeLabel:'国内',taxLabel:'税抜'}}),
 getPurchaseSlipStatusKeys:()=>['completed'],isPendingPurchaseArrivalStatus:()=>false,getPurchasePaymentMethodLabel:()=> '現金',
 getShippingAmounts:r=>({subtotal:r.total,tax:0,total:r.total,currency:r.displayCurrency,rate:0}),
 getShippingRecordRate:()=>155,getShippingRecordCurrency:r=>r.displayCurrency,
 getShippingSalePriceUSD:i=>i.salePrice,getShippingLineJPY:i=>i.salePrice*155,
 getShippingSaleTotalJPY:items=>items.reduce((sum,i)=>sum+i.salePrice*155,0),
 getShipmentProcessingStatus:()=> '処理中',getConsignmentProcessingStatus:()=> '処理済',
 getPurchaseReturnProcessingStatus:()=> '処理中',getSalesReturnProcessingStatus:()=> '処理済',_getReturnTrackingNumber:r=>r.trackingNo || '',
 getPurchaseReturnOriginalAmountInfo:r=>({items:r.items,sourcePurchase:{id:r.slipId,purchaseCurrency:'JPY',lines:r.items}}),
 getSalesReturnOriginalAmountInfo:r=>({items:r.items,sourceSale:{id:r.slipId,inputCurrency:'JPY'}}),
 getSalesRegistrationFXRate:()=>155,
 isCostAdjustmentRecord:r=>r._adjustmentType==='cost',isPurchaseAdjustmentRecord:r=>r._adjustmentType==='purchase',
 getAdjustmentRecordLabel:()=> '原価調整',getAdjustmentRecordTarget:r=>r.targetCode,getAdjustmentRecordAmount:r=>r.total,
 _slipOpenDetail:()=>opened++,
});
w.URL.createObjectURL=b=>{blob=b;return 'blob:test';};w.URL.revokeObjectURL=()=>{};
w.HTMLAnchorElement.prototype.click=function(){downloads.push(this.download);};
function load(start,end){const a=source.indexOf(start),b=source.indexOf(end,a);assert.ok(a>=0&&b>a,start);w.eval(source.slice(a,b));}
load('function _slipRecordId(', 'function _slipResetAllSelections(');
load('function _slipDecorateRenderedTable(', 'function buildSlipRow(');
load('function buildShippingSlipCsvRows(', 'function getFilteredSlipData(');
const body=d.getElementById('slipListBody');
function render(type,data){
 w.currentSlipTab=type;
 body.purchaseCsvRows=type==='purchase'&&data ? w.buildPurchaseListCsvRows(data):null;
 w.captureSlipCsvRecords(data);
 d.getElementById('slipTableHead').innerHTML='<tr><th>伝票番号</th></tr>';
 body.innerHTML=(data||[]).map(()=>'<tr><td>伝票</td></tr>').join('');
 [...body.rows].forEach(row=>row.addEventListener('click',()=>opened++));
 w._slipDecorateRenderedTable(data||[]);w.updateSlipCsvControls();
}
load('function slipCsvNumber(', '/** One row per displayed shipment');
const record=(id)=>({id,date:'2026-10-11',supplier:'仕入先',buyer:'販売先',destination:'委託先',staff:'担当者',purchaseCurrency:'JPY',
 inputCurrency:'JPY',displayCurrency:'JPY',usdJpyRate:155,_adjustmentType:'cost',total:300,slipId:'SOURCE-'+id,targetCode:'P-'+id,
 displaySubtotalMinor:300,displayTaxAmountMinor:30,displayTotalMinor:330,note:'備考, "全文"\n次の行',
 items:[1,2].map(n=>({code:id+'-'+n,brand:'ROLEX',model:'モデル',purchasePrice:n*100,purchaseCurrency:'JPY',inputAmount:n*100,inputCurrency:'JPY',salePrice:n*10,
  previousCostJpyMinor:100,adjustmentAmountJpyMinor:10,adjustedCostJpyMinor:110,allocatedCostJpyMinor:110})),
 lines:[1,2].map(n=>({code:id+'-'+n,brand:'ROLEX',model:'モデル',purchasePrice:n*100,purchaseCurrency:'JPY'})),
});
for(const type of ['purchase','shipping','consignment','sales','salesreturn','purchasereturn','adjustment']){
 const data=[record('SELECTED'),record('EXCLUDED')],before=JSON.stringify(data);
 render(type,data);
 assert.equal(d.getElementById('slipSummaryCsvButton').disabled,false,type);
 assert.equal(d.getElementById('slipProductCsvButton').disabled,false,type);
 assert.equal(d.querySelectorAll('.slip-csv-row-check').length,2,type);
 const check=d.querySelector('.slip-csv-row-check');check.click();
 assert.equal(opened,0,'checkbox must not open a detail modal');
 assert.equal(d.getElementById('slipCsvSelectAll').indeterminate,true);
 assert.equal(d.getElementById('slipCsvScope').textContent,'出力対象：選択した 1 件');
 assert.equal(body.rows[0].classList.contains('slip-csv-selected'),true);
 for(const unit of ['summary','products']){
  unit==='summary'?w.exportSlipSummaryCSV():w.exportSlipCSV();
  assert.equal(downloads.at(-1),`slip_${type}_${unit}_2026-10-11.csv`);
  const csv=await blob.text();
  assert.ok(csv.includes('SELECTED'),type+unit);
  assert.ok(!csv.includes('EXCLUDED'),type+unit+' excludes unselected records');
  assert.ok(csv.includes('備考, ""全文""\n次の行'),'notes survive comma, quotes and newline');
  assert.deepEqual([...new Uint8Array(await blob.arrayBuffer()).slice(0,3)],[239,187,191]);
  const rows=unit==='summary'?w.buildSlipSummaryCsvRows(type,w.getSlipCsvRecords()):w.buildSlipProductCsvRows(type,w.getSlipCsvRecords());
  assert.equal(rows.length,unit==='summary'?2:3,type+' unit row count');
  assert.ok(rows.every(row=>row.length===rows[0].length),'consistent columns');
 }
 // Redraw keeps selected IDs, then select-all and deselect-all synchronize all controls.
 render(type,data);assert.equal(d.querySelector('.slip-csv-row-check').checked,true);
 d.getElementById('slipCsvSelectAll').click();assert.equal(w.getSlipCsvRecords().length,2);
 assert.ok([...d.querySelectorAll('.slip-csv-row-check')].every(c=>c.checked));
 w.toggleSlipCsvSelection(null,false);assert.equal(d.getElementById('slipCsvSelectAll').checked,false);
 assert.equal(d.getElementById('slipCsvScope').textContent,'出力対象：表示中の全 2 件');
 w.exportSlipCSV();assert.ok((await blob.text()).includes('EXCLUDED'));
 w.toggleSlipCsvSelection('EXCLUDED',true);render(type,[data[0]]);
 assert.equal(body.slipCsvSelectedIds.size,0,'filter removes hidden selections');
 assert.equal(w.getSlipCsvRecords().length,1);
 // Editing a filter without searching cannot change the rendered snapshot.
 d.getElementById('slip-filter-keyword').value='not-yet-applied';
 w.exportSlipCSV();assert.ok((await blob.text()).includes('SELECTED'));
 assert.equal(JSON.stringify(data),before,'CSV never changes business records');
 render(type,[]);const count=downloads.length;w.exportSlipSummaryCSV();w.exportSlipCSV();
 assert.equal(downloads.length,count,'empty list cannot export hidden rows');
 assert.equal(d.getElementById('slipSummaryCsvButton').disabled,true);
 render(type,null);w.exportSlipCSV();assert.equal(downloads.length,count,'unsearched guard');
}
render('shipping',[record('SAME'),record('OTHER')]);w.toggleSlipCsvSelection('SAME',true);
render('sales',[record('SAME'),record('OTHER')]);assert.equal(body.slipCsvSelectedIds.size,0,'same ID never carries selection across tabs');
const untrusted=record('=FORMULA');untrusted.note='@SUM(1,2)';untrusted.items[0].inputAmount=-555;
render('sales',[untrusted]);w.exportSlipCSV();const safe=await blob.text();
assert.ok(safe.includes('"\'=FORMULA"'));assert.ok(safe.includes('"\'@SUM(1,2)"'));assert.ok(safe.includes('"-555"'),'negative numbers remain numeric');
const sales=w.buildSlipSummaryCsvRows('sales',[record('JPY')]);assert.equal(sales[1][4],300);assert.equal(sales[1][5],30);assert.equal(sales[1][6],330);assert.equal(sales[1][7],'JPY');
for(const direction of ['increase','decrease']) {
 const direct={...record('DIRECT'),adjustmentType:'direct_'+direction,items:[{code:'P1',direction,previousCostJpyMinor:100,adjustmentAmountJpyMinor:10,adjustedCostJpyMinor:direction==='increase'?110:90}]};
 const rows=w.buildSlipProductCsvRows('adjustment',[direct]);
 assert.equal(rows[1][7],direction==='increase'?10:-10,'cost adjustment sign');assert.equal(rows[1][8],'JPY');
 assert.equal(rows[1][9],100);assert.equal(rows[1][10],direction==='increase'?110:90);
}
const discount={...record('DISCOUNT'),_adjustmentType:'sales',items:[{code:'P1',salePrice:300,adjustedSalePrice:275}]};
assert.equal(w.buildSlipProductCsvRows('adjustment',[discount])[1][7],-25,'sale adjustment uses the per-product difference');
const originalPurchase={...record('USD-PURCHASE'),lines:[{code:'P1',purchasePrice:12345,purchaseCurrency:'USD',quantity:1}]};
const purchaseProduct=w.buildSlipProductCsvRows('purchase',[originalPurchase])[1];
assert.equal(purchaseProduct[11],1913475);assert.equal(purchaseProduct[12],'JPY','purchase products keep the entered currency');
// Detail CSV exports only the open shipment, independently of list selection/tab.
const modal=d.getElementById('slipDetailOverlay');
const detailShipment=record('OPEN-SHIPMENT'), otherShipment=record('OTHER-SHIPMENT');
w.APP_DATA.shipments=[detailShipment,otherShipment];
render('sales',[record('LIST-SELECTION')]);w.toggleSlipCsvSelection('LIST-SELECTION',true);
modal.dataset.slipType='shipping';modal.dataset.slipId=detailShipment.id;modal.classList.remove('hidden');
const shipmentBefore=JSON.stringify(w.APP_DATA.shipments);
{
 w.exportShippingSlipDetailCSV();
 assert.equal(downloads.at(-1),'slip_shipping_overseas_products_2026-10-11.csv');
 const csv=await blob.text();
 assert.ok(csv.includes('OPEN-SHIPMENT'));assert.ok(!csv.includes('OTHER-SHIPMENT'));assert.ok(!csv.includes('LIST-SELECTION'));
 assert.equal(w.currentSlipTab,'sales');assert.equal(w.getSlipCsvRecords()[0].id,'LIST-SELECTION');
}
assert.equal(JSON.stringify(w.APP_DATA.shipments),shipmentBefore);
let detailDownloads=downloads.length;
modal.classList.add('hidden');w.exportShippingSlipDetailCSV();assert.equal(downloads.length,detailDownloads);
modal.classList.remove('hidden');modal.dataset.slipType='sales';w.exportShippingSlipDetailCSV();assert.equal(downloads.length,detailDownloads);
modal.dataset.slipType='shipping';modal.dataset.slipId='MISSING';w.exportShippingSlipDetailCSV();assert.equal(downloads.length,detailDownloads);
modal.dataset.slipId=detailShipment.id;w.isGuest=()=>true;w.exportShippingSlipDetailCSV();assert.equal(downloads.length,detailDownloads);
dom.window.close();
console.log('PASS: all 7 slip types × 2 CSV units; selected-only/all/filter/tab/empty scopes, checkbox events, source currencies, escaping and no data mutations.');
