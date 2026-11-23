import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {JSDOM} from 'jsdom';
const root=new URL('../public/admin-reference/js/',import.meta.url);
const dom=new JSDOM('<!doctype html><html><body></body></html>',{url:'http://127.0.0.1/',runScripts:'outside-only'});
const w=dom.window;
let submitted;
const product={id:'product-1',productCode:'1609260001',brand:'ROLEX',brandId:'brand-1',supplierRoleId:'prl_random',purchaseStaffProfileId:'stf_random',purchaseDate:'2026-09-16',costAmountMinor:100,costCurrency:'JPY',baseSaleCurrency:'USD',inventoryStatus:'in_stock'};
const worker={id:'user-random',username:'0001',staffProfileId:'stf_random',staffCode:'0001',displayName:'バイヤー①',isActive:true,isPurchaseStaff:true,role:'worker'};
w.fetch=async(path,options={})=>{
 let payload={items:[],total:0,page:1,pageSize:100};
 if(path.endsWith('/auth/me'))payload={user:{id:'admin',role:'admin'},csrfToken:'test'};
 if(path.endsWith('/health'))payload={database:{driver:'postgres'}};
 if(path.endsWith('/company'))payload={bankAccounts:[]};
 if(path.includes('/products?'))payload={items:[product],total:1};
 if(path.includes('/partners?'))payload={items:[{id:'partner-1',partnerCode:'0006',legalName:'テスト仕入先',status:'active',roles:[{id:'prl_random',roleCode:'S002',roleType:'supplier',isActive:true}]}]};
 if(path.includes('/users?')||path.endsWith('/purchase-staff'))payload={items:[worker]};
 if(path.endsWith('/masters/brands'))payload={items:[{id:'brand-1',code:'0001',name:'ROLEX'}]};
 if(options.method==='PATCH'){
  submitted=JSON.parse(options.body);
  assert.equal(Object.hasOwn(submitted,'inventoryStatus'),false,'detail edits must not change workflow status');
  payload={...product,modelNumber:submitted.modelNumber};
 }
 return new Response(JSON.stringify(payload),{status:200});
};
w.eval((await readFile(new URL('data.js',root),'utf8'))+';window.APP_DATA=APP_DATA;');w.eval(await readFile(new URL('api_bridge.js',root),'utf8'));
w.allAccessoryMasterRecords=()=>[...(w.APP_DATA.accessoryRecords||[]),...(w.APP_DATA.legacyAccessoryRecords||[])];
await w.ZaikoAPI.hydrateAdmin();
const item=w.eval('APP_DATA.inventory[0]');
assert.equal(item.supplier,'S002');assert.equal(item.staff,'バイヤー①');
await w.ZaikoAPI.updateProduct(item,{...item,model:'モデル名の変更',accessories:[]});
assert.equal(submitted.modelNumber,'モデル名の変更');
assert.deepEqual(Object.keys(submitted).sort(),['duplicateSerialReason','modelNumber','reason']);
for (const status of ['purchasing','sold','shipped','broken_down','cost_adjusted','sales_return_pending']) {
 product.inventoryStatus=status;product.costCurrency='HKD';
 await w.ZaikoAPI.hydrateAdmin();
 const current=w.eval('APP_DATA.inventory[0]');
 await w.ZaikoAPI.updateProduct(current,{...current,model:'モデルだけ変更',status:'在庫中'});
 assert.deepEqual(Object.keys(submitted).sort(),['duplicateSerialReason','modelNumber','reason'],status);
}
await w.ZaikoAPI.updateProduct({...item,supplier:'S001',staff:'旧担当',brand:'旧ブランド'},item);
assert.equal(submitted.supplierCode,'S002');assert.equal(submitted.staffCode,'0001');assert.equal(submitted.brandCode,'0001');
await w.ZaikoAPI.updateProduct(item,{...item,code:'1609260009',accessories:['BRACELET PARTS'],braceletQty:3});
assert.equal(submitted.productCode,'1609260009');assert.equal(submitted.braceletQuantity,3);
await w.ZaikoAPI.updateProduct({...item,accessories:['BRACELET PARTS'],braceletQty:3},{...item,accessories:[]});
assert.equal(submitted.braceletQuantity,0);assert.deepEqual(submitted.accessoryCodes,[]);
// SKU and internal comments can be added from registration; other editors may omit them.
await w.ZaikoAPI.updateProduct(item,{...item,sku:'SKU-1-1',comment:'追加コメント'});
assert.equal(submitted.sku,'SKU-1-1');assert.equal(submitted.internalComment,'追加コメント');
assert.deepEqual(Object.keys(submitted).sort(),['duplicateSerialReason','internalComment','reason','sku']);
const populated={...item,sku:'KEEP-SKU',comment:'保存済みコメント'};
const withoutOptional={...populated,model:'モデルのみ変更'};
delete withoutOptional.sku;delete withoutOptional.comment;
await w.ZaikoAPI.updateProduct(populated,withoutOptional);
assert.deepEqual(Object.keys(submitted).sort(),['duplicateSerialReason','modelNumber','reason']);
// Legacy seeded IDs must still resolve through actual role records.
product.supplierRoleId='partner_role_S002';product.purchaseStaffProfileId='staff_0001';
await w.ZaikoAPI.hydrateAdmin();
assert.equal(w.eval('APP_DATA.inventory[0].supplier'),'S002');
assert.equal(w.eval('APP_DATA.inventory[0].staff'),'バイヤー①');
const oldMaster={id:'acc-links',code:'0040',name:'コマ（数字）'};
let liveMaster={...oldMaster};const renameCalls=[];
const originalFetch=w.fetch;
w.fetch=async(url,opts={})=>{
 if(url.includes('/masters/accessories')){
  if(opts.method==='PATCH'){liveMaster={...liveMaster,...JSON.parse(opts.body)};product.accessories=liveMaster.name;return new Response(JSON.stringify(liveMaster));}
  return new Response(JSON.stringify({items:[liveMaster]}));
 }
 return originalFetch(url,opts);
};
w._renameAccessoryReferences=(oldName,newName)=>renameCalls.push([oldName,newName]);
w.refreshAccessoryMasterConsumers=(oldName,newName)=>renameCalls.push(['refresh',oldName,newName]);
await w.ZaikoAPI.saveMasterRecord('accessory',oldMaster,{...oldMaster,name:'コマ'},'edit');
assert.deepEqual(renameCalls,[['コマ（数字）','コマ'],['refresh','コマ（数字）','コマ']]);
assert.equal(w.APP_DATA.inventory[0].accessories[0],'コマ');
assert.equal(w.APP_DATA.accessoryAliases['コマ（数字）'],'コマ');
const renamed=w.APP_DATA.inventory[0];
await w.ZaikoAPI.updateProduct(renamed,{...renamed,accessories:[]});
assert.deepEqual(submitted.accessoryCodes,[]);
console.log('Accessory rename API hydration and form refresh OK');
dom.window.close();console.log('Product edit master references: random IDs, legacy IDs, buyer preservation and PATCH payload passed.');
