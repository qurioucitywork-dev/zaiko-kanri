import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFile} from 'node:fs/promises';
const source = (await readFile(new URL('../public/admin-reference/js/app.js', import.meta.url), 'utf8')).replace(/\r\n/g, '\n');
const inputs = Object.fromEntries(['from','to','party','status','keyword'].map(k => ['slip-filter-' + k, {value:''}]));
inputs.slipListBody = {};
let rendered, downloads = 0, errors = 0;
const slip = (id, date, supplier, pendingArrivalCount) => ({id,date,supplier,pendingArrivalCount,lines:[]});
const records = [
  slip('SD-TARGET','2026-09-17','A',1),
  slip('SD-UNPAID','2026-09-17','A',0),
  slip('SD-OLDER','2026-09-16','A',1),
  slip('SD-NEWER','2026-09-18','A',1),
  slip('SD-OTHER-PARTY','2026-09-17','B',1),
  slip('SD-OTHER-KEYWORD','2026-09-17','A',1),
];
const ctx = vm.createContext({
  APP_DATA:{purchaseSlips:records,inventory:[]}, currentSlipTab:'purchase',
  _slipFilterState:{executed:true,showAll:false},
  document:{getElementById:id => inputs[id]}, HTMLButtonElement:class {},
  normalizeInventoryStatusLabel:v=>v, getSupplierName:v=>v,
  renderSlipList:data=>{rendered=data;}, canIssuePurchaseSlip:()=>true,
  peRenderList:()=>{}, buildPurchaseRecordTemplateHTML:r=>r.id,
  _downloadTemplateDocument:()=>{downloads++;}, showToast:()=>{errors++;},
  window:{ZaikoAPI:{issuePurchaseSlip:async r=>{r.issuedAt='2026-09-17T00:00:00Z';}}},
});
const runRange = (start,end) => vm.runInContext(source.slice(source.indexOf(start), source.indexOf(end,source.indexOf(start))),ctx);
runRange('function refreshSlipList()', '/**\n * 伝票種別');
runRange('function isPendingPurchaseArrivalStatus(', 'function renderPurchaseSlipStatusBadges(');
runRange('function matchesSlipStatusFilter(', '/** 現在の伝票タブ');
runRange('function filterSlipList()', '// ── フィルタリセット');
runRange('async function issuePurchaseSlipDocument(', 'async function issueConsignmentSlipDocument(');
const set = values => Object.entries(values).forEach(([key,value])=>{inputs['slip-filter-'+key].value=value;});
const ids = () => rendered?.map(r=>r.id);
set({status:'processing'});
ctx.filterSlipList();
const processing = ids();
assert.ok(!processing.includes('SD-UNPAID'));
await ctx.issuePurchaseSlipDocument('SD-TARGET');
assert.deepEqual(ids(),processing,'issuance must preserve processing filter');
set({from:'2026-09-17',to:'2026-09-17',party:'A',keyword:'target'});
ctx.filterSlipList();
assert.equal(ids().join(','),'SD-TARGET');
await ctx.issuePurchaseSlipDocument('SD-TARGET');
assert.equal(ids().join(','),'SD-TARGET','reissuance must preserve all combined filters');
assert.equal(downloads,2);
ctx.window.ZaikoAPI.issuePurchaseSlip = async()=>{throw new Error('test failure');};
await ctx.issuePurchaseSlipDocument('SD-TARGET');
assert.equal(ids().join(','),'SD-TARGET');
assert.equal(downloads,2); assert.equal(errors,1);
ctx.window.ZaikoAPI = null;
await ctx.issuePurchaseSlipDocument('SD-TARGET');
assert.equal(ids().join(','),'SD-TARGET','preview issuance also preserves filters');
set({from:'',to:'',party:'',keyword:'',status:''});
ctx._slipFilterState.showAll=true;
await ctx.issuePurchaseSlipDocument('SD-TARGET');
assert.equal(rendered.length,records.length,'explicit show all remains show all');
ctx._slipFilterState={executed:false,showAll:false};
await ctx.issuePurchaseSlipDocument('SD-TARGET');
assert.equal(rendered,null,'unsearched list remains empty');
console.log('PASS: purchase issuance/reissuance preserves status/date/party/keyword, show-all, unsearched and failed states.');
