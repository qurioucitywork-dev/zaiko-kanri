import assert from 'node:assert/strict';import vm from 'node:vm';import {readFile} from 'node:fs/promises';
const source=await readFile(new URL('../public/admin-reference/js/app.js',import.meta.url),'utf8');
let destination='domestic';let values=[749,1357];
const context=vm.createContext({
 APP_DATA:{buyers:[{code:'domestic',regionType:'domestic'},{code:'overseas',regionType:'overseas'}]},
 document:{getElementById:()=>({value:destination}),querySelectorAll:()=>values},
 _shippingEntryCurrency:'USD',getShippingRecordCurrency:r=>r.displayCurrency,getShippingSalePriceUSD:i=>i.salePriceUsd,
 getShippingLineJPY:i=>i.convertedSalePriceJpy,getShippingLinePriceUSD:i=>i,getShippingFormRate:()=>155,
 convertShippingUSDToJPY:n=>Math.ceil(n*155/1000)*1000,getShippingInputJPY:n=>Math.ceil(n*155/1000)*1000,
});
vm.runInContext(source.slice(source.indexOf('function getShippingTaxRate('),source.indexOf('function calcShippingTotal()')),context);
const amounts=r=>context.getShippingAmounts(r);
assert.equal(amounts().tax,209);assert.equal(amounts().total,2315);
destination='overseas';assert.equal(amounts().tax,0);assert.equal(amounts().total,2106);
destination='';assert.equal(amounts().tax,0);
destination='domestic';context._shippingEntryCurrency='JPY';values=[750];assert.equal(amounts().subtotal,117000);assert.equal(amounts().tax,11700);assert.equal(amounts().total,128700);
destination='overseas';const saved={taxRateBasisPoints:1000,displayCurrency:'JPY',items:[{salePriceUsd:750,convertedSalePriceJpy:117000}]};assert.equal(amounts(saved).total,128700);
assert.equal(amounts({...saved,taxRateBasisPoints:0}).total,117000);assert.equal(amounts({...saved,taxRateBasisPoints:undefined}).tax,0);
console.log('PASS: domestic-only tax, line rounding, currency conversion, saved tax snapshot and legacy totals.');
