import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { JSDOM } from 'jsdom';
const source = await readFile(new URL('../public/admin-reference/js/api_bridge.js', import.meta.url), 'utf8');
const appHtml = await readFile(new URL('../public/admin-reference/app.html', import.meta.url), 'utf8');
const loadingCss=appHtml.match(/<style id="api-loading-visibility">([\s\S]*?)<\/style>/)[1];
const dashboard = await readFile(new URL('../public/admin-reference/js/dashboard.js', import.meta.url), 'utf8');
const dom = new JSDOM('<!doctype html><html><body><div id="page-dashboard" class="page-panel"></div><div id="other-page" class="page-panel"></div></body></html>', { runScripts:'outside-only', url:'https://inventory.example/app/admin-reference/app.html' });
const w=dom.window;
const batchMode=process.argv.includes('--batch');
const style=w.document.createElement('style');style.textContent=loadingCss;w.document.head.appendChild(style);
w.eval('var APP_DATA={inventory:[],purchaseSlips:[],sales:[],shipments:[],consignments:[],purchaseReturns:[],salesReturns:[],staff:[],suppliers:[],buyers:[]};');
w._escHtml=value=>String(value??'');
w.allAccessoryMasterRecords=()=>[];
const deferred=()=>{let resolve;return{promise:new Promise(r=>{resolve=r;}),resolve:(v)=>resolve(v)};};
const secondary=deferred(), images=deferred(), coreReady=deferred();
const detailsStarted=new Set();
let active=0,peak=0,mutationCalls=0;
const listNames=['purchases','sales','shipments','consignments','returns'];
w.fetch=async(url,options)=>{
 const u=new URL(url,'https://inventory.example');const path=u.pathname.replace('/api/v1','');
 if(options.method && options.method!=='GET')mutationCalls++;
 active++;peak=Math.max(peak,active);
 try{
  await new Promise(r=>setTimeout(r,5));
  let value={items:[]};
  if(path==='/auth/me')value={user:{id:'admin',role:'admin',displayName:'Admin'},csrfToken:'test-csrf'};
  else if(path==='/health')value={database:{driver:'postgres'},features:{readBatch:batchMode}};
  else if(path==='/products')value={items:[{id:'p1',productCode:'PRODUCT1',brand:'Test',inventoryStatus:'in_stock',imageCount:1,costCurrency:'JPY',baseSaleCurrency:'USD',dashboardCostJpy:125000,purchaseDate:'2026-11-01'}],totalPages:1,total:1};
  else if(path==='/parts')await secondary.promise;
  else if(path==='/products/p1/files'){await images.promise;value={items:[{id:'file1',url:'/api/v1/product-files/file1'}]};}
  else if(path==='/exchange-rates')value={items:[{baseCurrency:'USD',rate:150}]};
  else if(listNames.some(name=>path==='/'+name))value={items:[{id:path.slice(1)+'1'}],totalPages:1};
  else if(listNames.some(name=>path.startsWith('/'+name+'/'))){detailsStarted.add(path.split('/')[1]);value={id:path.split('/')[2],lines:[],items:[],revisions:[]};}
  return{status:200,ok:true,headers:{get:()=>null},text:async()=>JSON.stringify(value)};
 }finally{active--;}
};
const wrapBatchTransport=fetchOne=>async(url,options)=>{
 if(batchMode && String(url)==='/api/v1/read-batch'){
  const requests=JSON.parse(options.body).requests;
  const responses=await Promise.all(requests.map(async item=>{
   const response=await fetchOne('/api/v1'+item.path,{...options,method:'GET',body:undefined});
   return{id:item.id,status:response.status,body:JSON.parse(await response.text())};
  }));
  return{status:200,ok:true,headers:{get:()=>null},text:async()=>JSON.stringify({responses})};
 }
 return fetchOne(url,options);
};
w.fetch=wrapBatchTransport(w.fetch);
w.eval(source);
w.eval(dashboard);
w.document.addEventListener('zaiko:dashboard-ready',()=>coreReady.resolve());
const hydration=w.ZaikoAPI.hydrateAdmin();
w.renderBusinessDashboard();
assert.equal(w.getComputedStyle(w.document.getElementById('page-dashboard')).visibility,'visible','initial loading text must be visible while requests run');
assert.equal(w.getComputedStyle(w.document.getElementById('other-page')).visibility,'hidden','other screens remain gated until the full snapshot');
assert.match(w.document.getElementById('page-dashboard').textContent,/読み込んでいます/);
assert.equal(w.document.querySelector('[data-business-metric]'),null,'pending data must not appear as zero totals');
const deadline=setTimeout(()=>coreReady.resolve('timeout'),3000);
assert.notEqual(await coreReady.promise,'timeout','dashboard should not wait for the parts/image/detail requests');
clearTimeout(deadline);
assert.equal(w.ZaikoAPI.state.hydrated,false,'partial core data must not unlock mutations');
assert.equal(w.ZaikoAPI.state.dashboardReady,true);
assert.equal(w.getComputedStyle(w.document.getElementById('page-dashboard')).visibility,'visible','real core metrics must be visible before detail/image responses');
assert.equal(w.APP_DATA.inventory.length,1);
assert.equal(w.document.querySelector('[data-business-metric="inventoryCount"]').textContent,'1点');
await assert.rejects(w.ZaikoAPI.request('/products',{method:'POST',body:'{}'}),/読み込み完了後/);
assert.equal(mutationCalls,0,'initial mutations must be blocked before reaching the network');
secondary.resolve();
for(let attempt=0;attempt<100 && detailsStarted.size<5;attempt++)await new Promise(r=>setTimeout(r,10));
assert.equal(detailsStarted.size,5,'all detail groups should start before waiting for image metadata');
images.resolve();
await hydration;
assert.equal(w.ZaikoAPI.state.hydrated,true);
assert.equal(w.APP_DATA.inventory[0].images.length,1,'full initialization must still include image metadata');
assert.ok(peak<=8,`read concurrency must be bounded, observed ${peak}`);
assert.ok(peak>1,'independent reads should overlap');
// A failed refresh must keep the complete old snapshot even when an image read
// from that refresh resolves after the failure handler has restored it.
const previousSnapshot=JSON.stringify(w.APP_DATA);
const oldFetch=w.fetch;
const failParts=deferred(),lateImage=deferred(),imageStarted=deferred();
w.fetch=wrapBatchTransport(async(url,options)=>{
 const path=new URL(url,'https://inventory.example').pathname;
 if(path==='/api/v1/parts'){
  await failParts.promise;
  return{status:403,ok:false,headers:{get:()=>null},text:async()=>JSON.stringify({error:{code:'test_failure',message:'auxiliary read failed'}})};
 }
 if(path==='/api/v1/products/p1/files'){
  imageStarted.resolve();await lateImage.promise;
  return{status:200,ok:true,headers:{get:()=>null},text:async()=>JSON.stringify({items:[{id:'late-file',url:'/late-file.png'}]})};
 }
 return oldFetch(url,options);
});
const refresh=w.ZaikoAPI.hydrateAdmin();
assert.equal(w.getComputedStyle(w.document.getElementById('page-dashboard')).visibility,'hidden','normal full refresh retains the existing snapshot visibility gate');
const rejectedRefresh=assert.rejects(refresh,/auxiliary read failed/);
await imageStarted.promise;
failParts.resolve();
await rejectedRefresh;
assert.equal(JSON.stringify(w.APP_DATA),previousSnapshot,'refresh failure must restore the complete snapshot');
lateImage.resolve();
await new Promise(r=>setTimeout(r,25));
assert.equal(JSON.stringify(w.APP_DATA),previousSnapshot,'late images must not overwrite the restored snapshot');
dom.window.close();
console.log('Progressive hydration, complete data, mutation guard, bounded reads and rollback isolation passed.');
