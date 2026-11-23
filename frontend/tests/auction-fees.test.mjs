import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {JSDOM,VirtualConsole} from 'jsdom';
const base=new URL('../public/admin-reference/',import.meta.url);
let html=await readFile(new URL('app.html',base),'utf8');
html=html.replace(/<link\b[^>]*>/gi,'');
for(const match of [...html.matchAll(/<script src="([^"]+)"[^>]*><\/script>/g)]){
 const src=match[1].split('?')[0];
 const code=src.startsWith('js/') && !src.includes('api_bridge') ? await readFile(new URL(src,base),'utf8') : '';
 html=html.replace(match[0],()=>`<script>${code.replaceAll('</script>','<\/script>')}</script>`);
}
const errors=[],vc=new VirtualConsole();vc.on('jsdomError',e=>errors.push(e));
const dom=new JSDOM(html,{url:'http://127.0.0.1/app/admin-reference/app.html',runScripts:'dangerously',pretendToBeVisual:true,virtualConsole:vc,beforeParse(w){
 w.sessionStorage.setItem('inv_session',JSON.stringify({id:'U001',role:'admin',name:'管理者',loginId:'admin'}));
 w.Chart=class{constructor(){}destroy(){}};w.alert=()=>{};w.confirm=()=>true;w.prompt=()=>'';w.print=()=>{};w.open=()=>null;w.scrollTo=()=>{};
 w.matchMedia=()=>({matches:false,addEventListener(){},removeEventListener(){},addListener(){},removeListener(){}});
 w.ResizeObserver=class{observe(){}unobserve(){}disconnect(){}};
 w.URL.createObjectURL=()=> 'blob:test';w.URL.revokeObjectURL=()=>{};w.HTMLElement.prototype.scrollIntoView=()=>{};
 w.HTMLCanvasElement.prototype.getContext=function(){return {canvas:this};};
}});
await new Promise(resolve=>dom.window.document.readyState==='complete'?resolve():dom.window.addEventListener('load',resolve,{once:true}));
const w=dom.window,d=w.document,run=s=>w.eval(s),plain=o=>JSON.parse(JSON.stringify(o));
const rate={mode:'rate',rateBasisPoints:333},tiers={mode:'tiers',tiers:[{from:0,to:100000,rateBasisPoints:1000},{from:100000,to:null,rateBasisPoints:500}]};
assert.equal(w.auctionCalculate(100019,rate,'purchase').amount,103349);
assert.equal(w.auctionCalculate(100019,rate,'sales').amount,96689);
assert.equal(w.auctionCalculate(99999,tiers,'purchase').fee,9999);
assert.equal(w.auctionCalculate(100000,tiers,'purchase').fee,5000);
assert.throws(()=>w.auctionCalculate(5,{mode:'fixed',fixedAmount:6},'sales'),/超え/);
assert.throws(()=>w.auctionCalculate(100,rate,'purchase','USD'),/円取引/);
for(const malformed of [{mode:'tiers',tiers:[{from:1,to:null,rateBasisPoints:100}]},{mode:'tiers',tiers:[{from:0,to:100,rateBasisPoints:100},{from:99,to:null,rateBasisPoints:100}]},{mode:'rate',rateBasisPoints:10001}])assert.throws(()=>w.auctionNormalizeRule(malformed));

w.showClientModal();
assert.equal(d.getElementById('client-auction-enabled').checked,false);
d.getElementById('client-auction-enabled').click();
assert.equal(d.getElementById('auction-purchase-mode'),null,'old calculation dropdown removed');
d.getElementById('auction-purchase-enabled').click();
d.getElementById('auction-purchase-scope').value='individual';w.auctionMasterScope('purchase','individual');
let tierRows=d.querySelectorAll('#auction-purchase-tiers .auction-tier-row');
tierRows[0].querySelector('[data-tier="value"]').value='10';w.auctionTierAdd('purchase');
tierRows=d.querySelectorAll('#auction-purchase-tiers .auction-tier-row');
tierRows[1].querySelector('[data-tier="value"]').value='5';
d.getElementById('auction-sales-enabled').click();
let saleTier=d.querySelector('#auction-sales-tiers .auction-tier-row');
saleTier.querySelector('[data-tier="unit"]').value='fixed';w.auctionTierUnit('sales',0,'fixed');
d.querySelector('#auction-sales-tiers [data-tier="value"]').value='550';
const settings=w.auctionReadMaster();assert.deepEqual(plain(settings.purchase),tiers);assert.equal(settings.sales.fixedAmount,550);
// Reopening an existing company hydrates both rules, and no other company inherits them.
w.auctionOpenMaster({auctionFees:settings});assert.equal(d.querySelector('#auction-sales-tiers [data-tier="value"]').value,'550');
assert.equal(d.getElementById('auction-purchase-scope').value,'individual');
assert.equal(d.getElementById('auction-sales-scope').value,'uniform');
assert.ok(d.querySelector('#auction-sales-tiers [data-tier="from"]').disabled);
// Mixed percentage/fixed tiers round-trip and use the same boundaries on both directions.
const mixed={mode:'tiers',tiers:[{from:0,to:100000,rateBasisPoints:333},{from:100000,to:null,rateBasisPoints:0,mode:'fixed',fixedAmount:550}]};
w.auctionOpenMaster({auctionFees:{isAuction:true,purchase:mixed,sales:rate}});
assert.deepEqual(plain(w.auctionReadMaster().purchase),mixed);
assert.equal(w.auctionCalculate(99999,mixed,'purchase').amount,103328);
assert.equal(w.auctionCalculate(100000,mixed,'purchase').amount,100550);
assert.equal(w.auctionCalculate(100000,mixed,'sales').amount,99450);
assert.match(w.auctionRuleLabel(mixed),/3.33%.*¥550/);
// Switching scope keeps unsaved bands and the other direction's inputs.
d.querySelector('#auction-sales-tiers [data-tier="value"]').value='7.25';
w.auctionMasterScope('purchase','uniform');
assert.equal(w.auctionReadMaster().purchase.mode,'rate');
assert.equal(w.auctionReadMaster().sales.rateBasisPoints,725);
w.auctionMasterScope('purchase','individual');assert.deepEqual(plain(w.auctionReadMaster().purchase),mixed);
// Disabling one side preserves its form values; % and JPY have different validation.
d.getElementById('auction-purchase-enabled').click();assert.equal(w.auctionReadMaster().purchase.mode,'none');
d.getElementById('auction-purchase-enabled').click();assert.deepEqual(plain(w.auctionReadMaster().purchase),mixed);
d.querySelectorAll('#auction-purchase-tiers [data-tier="value"]')[1].value='1.5';
assert.throws(()=>w.auctionReadMaster(),/整数/);
w.auctionTierUnit('purchase',1,'rate');assert.equal(w.auctionReadMaster().purchase.tiers[1].rateBasisPoints,150);
d.querySelectorAll('#auction-purchase-tiers [data-tier="value"]')[1].value='101';
assert.throws(()=>w.auctionReadMaster(),/0〜100/);
for (const legacy of [{mode:'none'},rate,{mode:'fixed',fixedAmount:550},tiers,mixed]) {
  w.auctionOpenMaster({auctionFees:{isAuction:true,purchase:legacy,sales:legacy}});
  assert.deepEqual(plain(w.auctionReadMaster().purchase),legacy,'preserve existing rule shape and values');
}
const malformedTiers=[{mode:'fixed',fixedAmount:-1},{mode:'fixed',fixedAmount:1,rateBasisPoints:1},
  {mode:'rate',fixedAmount:1},{mode:'bogus'},{mode:'fixed',fixedAmount:Number.MAX_SAFE_INTEGER+1}];
for (const fee of malformedTiers) assert.throws(()=>w.auctionNormalizeRule({mode:'tiers',tiers:[{from:0,to:null,...fee}]}));
w.auctionOpenMaster(null);assert.equal(d.getElementById('client-auction-enabled').checked,false);
const data=run('APP_DATA');data.clientCompanies.push({id:'9000',companyName:'手数料テスト',supplierCode:'S900',buyerCode:'B900',auctionFees:settings});
data.suppliers.push({code:'S900',name:'手数料テスト'});data.buyers.push({code:'B900',name:'手数料テスト'});
d.getElementById('pe-supplier').add(new w.Option('手数料テスト','S900'));
d.getElementById('sl-buyer').add(new w.Option('手数料テスト','B900'));
w._peInitNewSlip();d.getElementById('pe-supplier').value='S900';
run(`_peSlipData.purchaseCurrency='JPY';_peSlipData.supplier='S900';_peSlipData.lines=[{lineId:1,lineNo:1,code:'T1',purchasePrice:99999,salePrice:0},{lineId:2,lineNo:2,code:'T2',purchasePrice:100000,salePrice:0}];_peSlipData.lines.forEach(_peRenderLine);`);
w._peUpdateDetailUI();assert.equal(d.getElementById('pe-auction-fee').hidden,false);assert.equal(w.peGetPurchaseTotals().subtotal,199999);
w.auctionToggle('purchase');assert.equal(w.peGetPurchaseTotals().subtotal,214998);assert.equal(w.peGetPurchaseTotals().taxAmount,21499);
assert.equal(run('_peSlipData.lines[0].purchasePrice'),99999,'raw price must survive toggling');
assert.match(d.getElementById('pe-fee-1').textContent,/9,999.*109,998/);
w.auctionToggle('purchase');assert.equal(w.peGetPurchaseTotals().subtotal,199999);
w.auctionToggle('purchase');assert.equal(w.peGetPurchaseTotals().subtotal,214998,'ON twice never compounds');
const saved=w.auctionPurchaseSavedCopy(run('_peSlipData'));assert.equal(saved.lines[0].purchasePrice,109998);assert.equal(saved.lines[0].auctionFee.baseAmountMinor,99999);assert.ok(!saved.auctionFeeRule);
w.peSetPurchaseCurrency('USD');assert.equal(run('_peSlipData.auctionFeeRule'),null);assert.ok(d.querySelector('#pe-auction-fee button').disabled);
w.peSetPurchaseCurrency('JPY');assert.equal(run('_peSlipData.auctionFeeRule'),null,'return to JPY starts OFF');

w.resetSalesForm({silent:true});d.getElementById('sl-buyer').value='B900';
const input=d.querySelector('#salesLines .sl-input-price');input.value='100,019';
w.calcSalesTotal();w.auctionToggle('sales');
assert.equal(w.auctionSalesAmount(input),99469);assert.match(d.getElementById('salesTotalDisplay').textContent,/99,469/);
assert.match(d.getElementById('salesTotalTax').textContent,/9,947/,'tax computed on adjusted net');
assert.equal(input.value,'100,019');w.auctionToggle('sales');assert.equal(w.auctionSalesAmount(input),100019);w.auctionToggle('sales');
input.value='500';assert.equal(w.auctionValidateRegistration('sales'),false);input.value='550';assert.equal(w.auctionSalesResult(input).amount,0);
w.switchSalesEntryCurrency('USD');assert.equal(run('_auctionTransactions.sales.rule'),null);
w.switchSalesEntryCurrency('JPY');assert.equal(run('_auctionTransactions.sales.rule'),null);
// An unrelated counterparty must not carry the toggle across transactions.
d.getElementById('sl-buyer').value='';w.calcSalesTotal();assert.equal(d.getElementById('sl-auction-fee').hidden,true);
// Saving sends the original input to the authoritative server once, and ignores excluded rows.
w.resetSalesForm({silent:true});d.getElementById('sl-buyer').value='B900';w.addSalesLine();
const saleRows=[...d.querySelectorAll('#salesLines tr[data-line-id]')];
const includedID=saleRows[0].dataset.lineId,excludedID=saleRows[1].dataset.lineId;
data.inventory.push({code:'AUCTION-SALE-TEST',status:'在庫中',brand:'テスト'});
d.getElementById(`sl-code-${includedID}`).value='AUCTION-SALE-TEST';
d.getElementById(`sl-price-${includedID}`).value='100019';
d.getElementById(`sl-price-${excludedID}`).value='50';
d.getElementById(`sl-chk-${excludedID}`).checked=false;
w.calcSalesTotal();w.auctionToggle('sales');
let savedPayload;w.ZaikoAPI={saveSale:async payload=>{savedPayload=plain(payload);return {record:{slipNumber:'TEST-FEE'}};}};
await w.saveSales();
assert.ok(savedPayload,'excluded low-priced row must not block registration');
assert.deepEqual(savedPayload.lines,[{productCode:'AUCTION-SALE-TEST',unitPriceMinor:100019}]);
assert.equal(savedPayload.auctionFeeRule.fixedAmount,550);delete w.ZaikoAPI;
assert.equal(errors.length,0,errors.map(e=>e.message).join('\n'));
await new Promise(resolve=>setTimeout(resolve,20));
dom.window.close();console.log('Auction fee UI: master forms, tier boundaries, snapshots, taxes, ON/OFF and currency changes passed.');
