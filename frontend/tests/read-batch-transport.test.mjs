import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {JSDOM} from 'jsdom';
const source=await readFile(new URL('../public/admin-reference/js/api_bridge.js',import.meta.url),'utf8');
const response=(status,body)=>({status,ok:status>=200&&status<300,headers:{get:()=>null},text:async()=>JSON.stringify(body)});
const defer=()=>{let resolve;const promise=new Promise(r=>resolve=r);return{promise,resolve};};
async function fixture(run){
 const dom=new JSDOM('<!doctype html><html><body></body></html>',{runScripts:'outside-only',url:'https://inventory.example/app/admin-reference/app.html'});
 const w=dom.window;w.eval(source);w.ZaikoAPI.state.csrfToken='fresh-csrf';w.ZaikoAPI.state.readBatchEnabled=true;
 try{await run(w,w.ZaikoAPI);}finally{dom.window.close();}
}
await fixture(async(w,api)=>{
 let logical=0,peak=0,calls=0;const seen=[];
 w.fetch=async(url,options)=>{
  calls++;assert.equal(url,'/api/v1/read-batch');assert.equal(options.method,'POST');assert.equal(options.headers['X-CSRF-Token'],'fresh-csrf');
  const items=JSON.parse(options.body).requests;seen.push(...items.map(v=>v.path));logical+=items.length;peak=Math.max(peak,logical);
  await new Promise(r=>setTimeout(r,5));logical-=items.length;
  return response(200,{responses:items.reverse().map(item=>({id:item.id,status:200,body:{path:item.path}}))});
 };
 const paths=Array.from({length:33},(_,i)=>'/products?q=item'+i);
 const values=await Promise.all(paths.map(path=>api.request(path)));
 assert.deepEqual(values.map(v=>v.path),paths,'correlation IDs must preserve the original caller result');
 assert.equal(new Set(seen).size,33);assert.equal(seen.length,33,'no logical GET may be duplicated');
 assert.ok(peak<=8);assert.ok(calls<=5,`33 same-lane reads should use at most five exchanges, got ${calls}`);
});
await fixture(async(w,api)=>{
 const secondary=defer();const paths=[];
 w.fetch=async(url,options)=>{
  const items=JSON.parse(options.body).requests;paths.push(items.map(v=>v.path));
  if(items.some(v=>v.path==='/parts'))await secondary.promise;
  return response(200,{responses:items.map(item=>({id:item.id,status:200,body:{path:item.path}}))});
 };
 const core=api.request('/products');const later=api.request('/parts');
 assert.equal((await core).path,'/products','secondary reads must not delay core results');
 assert.equal(paths.length,2);secondary.resolve();await later;
});
for(const status of [404,405,413])await fixture(async(w,api)=>{
 let batch=0,direct=0;
 w.fetch=async(url,options)=>{
  if(url==='/api/v1/read-batch'){batch++;return response(status,{error:{code:status===413?'batch_response_too_large':'not_found',message:'fallback'}});}
  direct++;assert.equal(options.method,undefined);return response(200,{path:url});
 };
 const result=await Promise.all([api.request('/products'),api.request('/partners')]);
 assert.equal(result.length,2);assert.equal(batch,1);assert.equal(direct,2);
 assert.equal(api.state.readBatchEnabled,status===413,'only an unsupported endpoint disables capability');
});
for(const [status,body] of [[403,{error:{code:'csrf_rejected',message:'denied'}}],[400,{error:{code:'read_batch_not_supported',message:'unsupported'}}],[413,{error:{code:'different_limit',message:'limit'}}],[200,{}]])await fixture(async(w,api)=>{
 let calls=0;w.fetch=async(url)=>{calls++;assert.equal(url,'/api/v1/read-batch');return response(status,body);};
 await assert.rejects(api.request('/products',{retry:false}));assert.equal(calls,1,'denial/malformed responses must never silently fall back');
});
await fixture(async(w,api)=>{
 const occurrences=new Map();
 w.fetch=async(url,options)=>{
  assert.equal(url,'/api/v1/read-batch');const items=JSON.parse(options.body).requests;
  return response(200,{responses:items.map(item=>{
   const count=(occurrences.get(item.path)||0)+1;occurrences.set(item.path,count);
   const failed=item.path==='/partners'&&count===1;
   return{id:item.id,status:failed?503:200,body:failed?{error:{code:'temporary',message:'retry'}}:{path:item.path}};
  })});
 };
 const result=await Promise.all([api.request('/products'),api.request('/partners')]);
 assert.equal(result[1].path,'/partners');assert.equal(occurrences.get('/products'),1);assert.equal(occurrences.get('/partners'),2,'only the failed GET is retried');
});
await fixture(async(w,api)=>{
 let requests=[];w.fetch=async(url,options)=>{requests.push({url,options});return response(200,{ok:true});};
 w.__adminPageAccess={page:'inventory',code:'temporary-admin-code'};
 await api.request('/products');assert.equal(requests[0].url,'/api/v1/products');
 assert.equal(requests[0].options.headers['X-Admin-Page-Code'],'temporary-admin-code');
 delete w.__adminPageAccess;
 await api.request('/products',{method:'POST',body:'{}'});
 await api.request('/guest/catalog');await api.request('/products?unsupported=1');
 await api.request('/products?q=a&q=b');await api.request('/products?q='+encodeURIComponent('a'.repeat(2049)));
 await api.request('/products/'+'p'.repeat(129));await api.request('/products?q=a;b');await api.request('/products?q=%ZZ');
 api.state.readBatchEnabled=false;await api.request('/products');
 assert.equal(requests.some(v=>v.url==='/api/v1/read-batch'),false,'elevated, write, unknown and old-server requests retain their original route');
});
await fixture(async(w,api)=>{
 const started=defer(),finish=defer();let groupAborted=false;
 w.fetch=async(url,options)=>{
  options.signal.addEventListener('abort',()=>groupAborted=true);const items=JSON.parse(options.body).requests;started.resolve();await finish.promise;
  return response(200,{responses:items.map(item=>({id:item.id,status:200,body:{path:item.path}}))});
 };
 const abort=new w.AbortController();
 const cancelled=api.request('/products',{signal:abort.signal,retry:false});
 const cancellation=assert.rejects(cancelled,error=>error.name==='AbortError');
 const remaining=api.request('/partners');await started.promise;abort.abort();await cancellation;
 assert.equal(groupAborted,false,'one cancelled caller must not abort other batch subscribers');finish.resolve();
 assert.equal((await remaining).path,'/partners');
});
console.log('Read batching: grouping, concurrency, response mapping, lane isolation, capability fallback, 413 limit, errors, retry, elevation, mutations and abort passed.');
