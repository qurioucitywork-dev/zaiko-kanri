import assert from 'node:assert/strict';import vm from 'node:vm';import {readFile} from 'node:fs/promises';
const app=await readFile(new URL('../public/admin-reference/js/app.js',import.meta.url),'utf8');
const bridge=await readFile(new URL('../public/admin-reference/js/api_bridge.js',import.meta.url),'utf8');
const ctx=vm.createContext({APP_DATA:{inventory:[{code:'A',status:'出荷中'}]},state:{latestRate:155},statusLabel:{confirmed:'確定'},normalizeInventoryStatusLabel:s=>s==='出荷中'?'出荷済':s});
vm.runInContext(bridge.slice(bridge.indexOf('  function shipmentView('),bridge.indexOf('  function consignmentView(')),ctx);
vm.runInContext(app.slice(app.indexOf('function getShipmentProcessingStatus('),app.indexOf('/** 委託伝票に含まれる商品の現在状態')),ctx);
const original={id:'old',slipNumber:'SY-OLD',status:'confirmed',processingStatus:'completed',lines:[{productCode:'A',shipmentPending:false}]};
const active={...original,id:'new',slipNumber:'SY-NEW',processingStatus:'processing',lines:[{productCode:'A',shipmentPending:true}]};
for(const status of ['出荷中','在庫中','売却済','出荷済']) {
 ctx.APP_DATA.inventory[0].status=status;
 assert.equal(ctx.getShipmentProcessingStatus(ctx.shipmentView(original)),'処理済');
 assert.equal(ctx.getShipmentProcessingStatus(ctx.shipmentView(active)),'処理中');
}
const reloaded=ctx.shipmentView(JSON.parse(JSON.stringify(original)));
assert.equal(reloaded.items[0].shipmentPending,false);
assert.equal(ctx.getShipmentProcessingStatus(reloaded),'処理済');
assert.equal(ctx.getShipmentProcessingStatus(ctx.shipmentView({...active,processingStatus:'completed'})),'処理済');
console.log('PASS: API mapping and list status use document history across inventory changes and reload.');
