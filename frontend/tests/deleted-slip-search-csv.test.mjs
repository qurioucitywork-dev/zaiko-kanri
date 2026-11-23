import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {createRequire} from 'node:module';
const {JSDOM}=createRequire(process.argv[4] || import.meta.url)('jsdom');
process.env.TZ='Asia/Tokyo';
const source=await readFile(process.argv[2] || new URL('../public/admin-reference/js/app.js',import.meta.url),'utf8');
const html=await readFile(process.argv[3] || new URL('../public/admin-reference/app.html',import.meta.url),'utf8');
const dom=new JSDOM(html,{url:'https://example.invalid/app.html',runScripts:'outside-only'});
const w=dom.window,d=w.document;let blob,download,clicks=0,toast,opened;
w.APP_DATA={deletedPurchaseSlips:[
 {_id:'db-a',id:'SD-A',date:'2026-09-01',supplier:'A',supplierName:'仕入先A',staff:'田中',deletedAt:'2026-09-30T15:00:00Z',reason:'重複, "確認"\n全行保持'},
 {_id:'db-b',id:'SD-B',date:'2026-09-02',supplier:'B',staff:'佐藤',deletedAt:'2026-09-30T14:59:00Z',reason:'=1+1'},
 {_id:'db-c',id:'SD-C',date:'2026-09-01',supplier:'A',supplierName:'仕入先A',staff:'佐藤',deletedAt:'2026-10-01T04:30:00Z',reason:'<img src=x onerror=alert(1)>誤登録'},
 {id:'SD-OLD',date:'2026-08-01',supplierName:'個人仕入先',staff:'退職者',reason:'旧データ'},
]};
w.getSupplierName=code=>code==='B'?'仕入先B':code;w.showToast=(...args)=>{toast=args;};w.openDeletedSlipDetail=id=>{opened=id;};
w.Blob=Blob;w.URL.createObjectURL=b=>{blob=b;return 'blob:test';};w.URL.revokeObjectURL=()=>{};
w.HTMLAnchorElement.prototype.click=function(){clicks++;download=this.download;};
const load=(start,end)=>{const a=source.indexOf(start),b=source.indexOf(end,a);assert.ok(a>=0&&b>a,start);w.eval(source.slice(a,b));};
load('function getLocalDateISO(', '// =====================================================');
load('function _escStrHtml(', '/** 商品コードエラー');
load('function formatArchiveDate(', 'async function markSalePaidFromList(');
load('let _deletedSlipArchiveFilters', 'let _deletedSlipDetailRequest');
const before=JSON.stringify(w.APP_DATA),body=d.getElementById('deletedSlipListBody');
const set=(name,value)=>{d.getElementById('deleted-slip-'+name).value=value;};
const ids=()=>[...body.querySelectorAll('tr td:nth-child(2)')].map(td=>td.textContent);
const search=()=>w.applyDeletedSlipFilter();
w.renderDeletedSlipArchive();
assert.deepEqual(ids(),['SD-C','SD-A','SD-B','SD-OLD']);
assert.equal(d.getElementById('deletedSlipCount').textContent,'4件 / 全4件');
assert.equal(body.querySelector('img'),null,'archived reasons are escaped');
assert.ok(d.getElementById('deleted-slip-supplier').textContent.includes('個人仕入先'),'historical counterparties without master codes remain searchable');
body.querySelector('[data-deleted-slip-id="db-a"]').click();assert.equal(opened,'db-a','details retain persistent ID');
set('from','2026-10-01');set('to','2026-10-01');search();assert.deepEqual(ids(),['SD-C','SD-A'],'deletion date uses JST, inclusive boundaries');
set('staff','田中');search();assert.deepEqual(ids(),['SD-A']);
w.exportDeletedSlipCSV();let csv=await blob.text();
assert.ok(csv.includes('"伝票種別","伝票番号","仕入日","仕入先","バイヤー","削除日時","削除理由","保存状態"'));
assert.ok(csv.includes('重複, ""確認""\n全行保持'));assert.ok(csv.includes('SD-A'));assert.ok(!csv.includes('SD-B'));
assert.deepEqual([...new Uint8Array(await blob.arrayBuffer()).slice(0,3)],[239,187,191]);
assert.match(download,/^slip_deleted_\d{4}-\d{2}-\d{2}\.csv$/u);
assert.equal(d.querySelectorAll('a[href="blob:test"]').length,0,'temporary download link removed');
set('keyword','no-match');w.exportDeletedSlipCSV();assert.ok((await blob.text()).includes('SD-A'),'export remains the displayed search result');
search();assert.equal(ids().length,0);assert.equal(d.getElementById('deletedSlipCsvButton').disabled,true);
assert.equal(d.getElementById('deletedSlipEmptyText').textContent,'検索条件に一致する削除伝票はありません');
const previous=clicks;w.exportDeletedSlipCSV();assert.equal(clicks,previous);assert.equal(toast[0],'info');
w.resetDeletedSlipFilter();assert.equal(ids().length,4);assert.equal(d.getElementById('deletedSlipCsvButton').disabled,false);
set('date-type','date');set('from','2026-09-01');set('to','2026-09-01');set('supplier','A');search();assert.deepEqual(ids(),['SD-C','SD-A'],'purchase date and supplier combine');
set('from','2026-09-02');search();assert.equal(toast[0],'warning');assert.deepEqual(ids(),['SD-C','SD-A'],'invalid date range preserves displayed result');
w.resetDeletedSlipFilter();set('keyword','ｓｄ－ａ　重複');search();assert.deepEqual(ids(),['SD-A'],'fullwidth, case and multiple terms');
w.resetDeletedSlipFilter();set('keyword','仕入先B');search();assert.deepEqual(ids(),['SD-B']);
w.exportDeletedSlipCSV();assert.ok((await blob.text()).includes("\"'=1+1\""),'spreadsheet formula text is neutralized');
w.resetDeletedSlipFilter();set('supplier','個人仕入先');search();assert.deepEqual(ids(),['SD-OLD'],'individual supplier without code can be selected');
w.resetDeletedSlipFilter();assert.equal(JSON.stringify(w.APP_DATA),before,'read-only feature never changes archives');
w.APP_DATA.deletedPurchaseSlips=[];w.renderDeletedSlipArchive();assert.equal(d.getElementById('deletedSlipEmptyText').textContent,'削除済み伝票はありません');
assert.equal(d.getElementById('deletedSlipCsvButton').disabled,true);
assert.match(d.querySelector('form[aria-label="削除伝票の検索"]').getAttribute('onsubmit'),/applyDeletedSlipFilter\(\)/u);
dom.window.close();
console.log('PASS: archive date/supplier/buyer/keyword search, historical options, inclusive local dates, sorting, detail IDs, CSV snapshot/escaping/BOM, reset, invalid ranges and empty states.');
