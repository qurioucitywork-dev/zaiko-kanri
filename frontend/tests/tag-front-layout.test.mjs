import assert from 'node:assert/strict';
import { readFile, writeFile } from 'node:fs/promises';
import { createRequire } from 'node:module';
import path from 'node:path';
const repo = process.argv[2] || path.resolve(import.meta.dirname, '..');
const require = createRequire(path.join(repo, 'package.json'));
const {JSDOM} = require('jsdom');
const dom = new JSDOM('<!doctype html><body></body>',{runScripts:'dangerously',url:'https://example.invalid'});
const w=dom.window,d=w.document;
const sourceRoot=path.join(repo,'public/admin-reference');
for(const script of ['qrcode-generator','price_code','master_display','qr_inventory']) {
  const source=script==='qr_inventory' && process.argv[3] ? process.argv[3] : path.join(sourceRoot,'js',script+'.js');
  const scriptElement=d.createElement('script');scriptElement.textContent=await readFile(source,'utf8');d.head.appendChild(scriptElement);
}
w.APP_DATA={inventory:[],brandRecords:[{code:'BR1',name:'Brand',tagDetail:'TAG BRAND'}],materials:[{code:'MAT',name:'素材',tagDetail:'YG/SS'}],
 beltMaterialRecords:[{code:'0003',name:'巻きジュビリー',tagDetail:'Old Jubilee'}],movements:[{code:'AT',name:'自動巻',tagDetail:'AT'}],
 markingRecords:[{code:'M1',name:'ハート',tagDetail:'♡'}],staffRecords:[{code:'0002',name:'仕入担当',tagDetail:'2'}],
 conditions:[{code:'C1',name:'A'}],accessoryRecords:[{code:'A1',name:'RX箱',tagDetail:'・'},{code:'A2',name:'ギャラ2c年月',tagDetail:'2c'}]};
w.allAccessoryMasterRecords=()=>w.APP_DATA.accessoryRecords;
const item={code:'1609260006',brand:'BR1',material:'MAT',model:'Oyster Perpetual Milgauss',ref:'16233',serial:'E12345',dial:'シルバー',weightGrams:125.5,belt:'3',movement:'AT',marking:'M1',staffCode:'0002',condition:'C1',note:'ケースに小傷あり\n動作確認済み',comment:'INTERNAL NEVER PRINT',accessories:['A1','A2'],accessorySupplements:{A2:'26/9'},salePrice:66500,purchasePrice:1355000};
const saved=JSON.stringify(item);
d.body.innerHTML=w.renderInventoryProductTagPanel(item);
const front=d.querySelector('.inventory-product-tag-front');
assert.equal(d.querySelectorAll('.inventory-product-tag').length,1);
assert.equal(d.querySelectorAll('.inventory-product-tag-back').length,0);
assert.equal(front.querySelectorAll('.inventory-product-tag-qr').length,1);
const text=field=>front.querySelector(`[data-tag-field="${field}"] text`).textContent;
assert.equal(text('brand-material'),'TAG BRAND（YG/SS）');
assert.equal(text('reference-serial'),'16233　／　E12345');
assert.equal(text('dial-weight'),'シルバー（125.5 g）');
assert.equal(text('belt-movement'),'Old Jubilee（AT）');
assert.equal(text('accessories'),'・、2c 26/9');
assert.equal(front.querySelector('[data-tag-field="accessories"]').getAttribute('title'),'付属品：RX箱、ギャラ2c年月 26/9');
assert.equal(text('buyer'),'2');assert.equal(text('marking'),'♡');assert.equal(text('management-code'),item.code);
assert.match(text('remarks'),/^コンディション：Aケースに小傷あり動作確認済み$/u);
assert.ok(!front.textContent.includes(item.comment));
assert.equal(text('sale').slice(3,-3),'REY');assert.equal(text('cost').slice(3,-3),'TSYE');
assert.equal(front.querySelector('[data-tag-price="sale"]').getAttribute('title'),'売価（ドル）：USD 66,500.00');
assert.equal(front.querySelector('[data-tag-price="cost"]').getAttribute('title'),'原価（円）：JPY 1,355,000');
const expectedHints={
 'brand-material':'ブランド名：Brand\n素材：素材',model:'モデル名：Oyster Perpetual Milgauss',
 'reference-serial':'型番：16233\nシリアル：E12345','dial-weight':'文字盤：シルバー\n重量：125.5 g',
 'belt-movement':'ベルト種類：巻きジュビリー\n駆動：自動巻',marking:'マーキング：ハート',buyer:'仕入バイヤー：仕入担当',
 remarks:'備考：コンディション：A\nケースに小傷あり\n動作確認済み','management-code':'管理番号：1609260006',
};
for(const [field,expected] of Object.entries(expectedHints)) assert.equal(front.querySelector(`[data-tag-field="${field}"] title`).textContent,expected);
for(const field of front.querySelectorAll('[data-tag-field]')){
 assert.ok(field.getAttribute('title'),'every field has a Japanese hint');
 assert.ok(field.querySelector('rect[fill="transparent"]'),'the entire field receives hover, including empty remarks');
}
assert.equal(front.querySelector('.inventory-product-tag-qr title').textContent,'QRコード：管理番号 1609260006');
assert.equal(front.querySelectorAll('.tag-rule').length,0,'no horizontal dividers');
const visibleText=[...front.querySelectorAll('text')].map(node=>node.textContent).join('\n');
for(const label of ['売価（ドル）','仕入バイヤー','原価（¥）']) assert.ok(!visibleText.includes(label),'visible caption removed: '+label);
const svg=w.createInventoryProductTagSvg(item), parsed=new w.DOMParser().parseFromString(svg,'image/svg+xml');
assert.equal(parsed.querySelectorAll('parsererror').length,0);
assert.ok(!svg.includes('裏面'));assert.ok(!svg.includes('66,500'));assert.ok(!svg.includes('1,355,000'));assert.ok(!svg.includes(item.comment));
assert.equal(parsed.querySelectorAll('[data-tag-price]').length,2);
assert.equal(parsed.querySelectorAll('title, [title]').length,0,'hover-only information is excluded from downloadable tags');
assert.equal(parsed.querySelectorAll('path').length,0,'no internal tag dividers');
for(const field of ['brand-material','model','reference-serial','dial-weight','belt-movement','accessories','remarks','marking','buyer','sale','cost','management-code']) {
 assert.equal(parsed.querySelector(`[data-tag-field="${field}"] text`).textContent,text(field),`same preview/export field: ${field}`);
}
assert.equal(JSON.stringify(item),saved,'rendering leaves stored data unchanged');
assert.equal(w.getInventoryQrPayload(item),item.code);
// Decode the vector QR to verify it still carries only the management number.
const qrSvg=parsed.querySelector('.inventory-product-tag-qr');
const pixels=new Uint8ClampedArray(400*400*4).fill(255);
for(const rect of qrSvg.querySelectorAll('rect[fill="#111827"]')) {
 const x=(Number(rect.getAttribute('x'))-40)/144*400,y=(Number(rect.getAttribute('y'))-593)/144*400;
 const width=Number(rect.getAttribute('width'))/144*400,height=Number(rect.getAttribute('height'))/144*400;
 for(let py=Math.round(y);py<Math.round(y+height);py++)for(let px=Math.round(x);px<Math.round(x+width);px++){const p=(py*400+px)*4;pixels[p]=pixels[p+1]=pixels[p+2]=0;}
}
assert.equal(require('jsqr')(pixels,400,400).data,item.code);
const unsafe=w.createInventoryProductTagSvg({...item,note:'<script>alert(1)</script>',brand:'<img onerror=x>',dial:'<&>'});
assert.ok(!unsafe.includes('<script>'));assert.ok(unsafe.includes('&lt;script&gt;'));assert.ok(unsafe.includes('&lt;&amp;&gt;'));
const zero=w.createInventoryProductTagSvg({...item,weightGrams:0,purchasePrice:0,salePrice:0});
assert.ok(zero.includes('0 g'));
const missing=w.createInventoryProductTagSvg({code:'EMPTY',accessories:[]});
assert.ok(!missing.includes('undefined'));assert.ok(!missing.includes('NaN'));
const missingDoc=new w.DOMParser().parseFromString(missing,'image/svg+xml');
assert.equal(missingDoc.querySelector('[data-tag-field="accessories"] text').textContent,'ー');
assert.equal(missingDoc.querySelector('[data-tag-field="model"] text').textContent,'ー');
w.APP_DATA.accessoryRecords.push({code:'HIDDEN',name:'非表示付属品',tagDetail:''});
assert.equal(w._inventoryTagAccessories({...item,accessories:['HIDDEN']}),'ー','blank tag master labels use the same placeholder');
const emptyHint=new w.DOMParser().parseFromString(w.createInventoryProductTagSvg({code:'EMPTY',accessories:[]},{preview:true}),'image/svg+xml');
assert.equal(emptyHint.querySelector('[data-tag-price="sale"] title').textContent,'売価（ドル）：未設定');
assert.equal(emptyHint.querySelector('[data-tag-field="accessories"] title').textContent,'付属品：ー');
const zeroHint=new w.DOMParser().parseFromString(w.createInventoryProductTagSvg({...item,purchasePrice:0,salePrice:0},{preview:true}),'image/svg+xml');
assert.equal(zeroHint.querySelector('[data-tag-price="cost"] title').textContent,'原価（円）：JPY 0');
const unsafeHint=new w.DOMParser().parseFromString(w.createInventoryProductTagSvg({...item,model:'<img onerror=x>',note:'<&>"'},{preview:true}),'image/svg+xml');
assert.equal(unsafeHint.querySelectorAll('parsererror, img').length,0);
assert.equal(unsafeHint.querySelector('[data-tag-field="model"] title').textContent,'モデル名：<img onerror=x>');
const longSvg=w.createInventoryProductTagSvg({...item,model:'A very long model name '.repeat(4),note:'長い備考内容'.repeat(60),accessories:['A1','A2']});
assert.equal(new w.DOMParser().parseFromString(longSvg,'image/svg+xml').querySelectorAll('parsererror').length,0);
// Individual print and both bulk callers route through this same front-only SVG.
w.APP_DATA.inventory=[item];w._itemDetailCurrentCode=item.code;w.setTimeout=()=>0;w.showToast=()=>{};
w.printInventoryProductTag();
const individual=d.querySelector('iframe').contentDocument;
assert.equal(individual.querySelectorAll('.inventory-product-tag-svg').length,1);
assert.equal(individual.querySelectorAll('[data-tag-price]').length,2);
assert.equal(individual.querySelectorAll('svg title, svg [title]').length,0);
for(const frameClass of ['purchase-product-tags-print-frame','cost-adjustment-product-tags-print-frame']) {
 const frame=w._printSlipProductTags('TEST',[item,{...item,code:'SECOND'}],frameClass);
 assert.equal(frame.contentDocument.querySelectorAll('.product-tag-print-page').length,2);
 assert.equal(frame.contentDocument.querySelectorAll('.inventory-product-tag-qr').length,2);
 assert.equal(frame.contentDocument.querySelectorAll('[data-tag-price]').length,4);
}
if(process.argv[4]) await writeFile(path.join(process.argv[4],'sample-tag.svg'),svg);
console.log('Front-only tags: field order, purpose-specific masters, fixed currency codes, hover amounts, QR decoding, safe notes, preview/export parity and individual/bulk printing passed.');
dom.window.close();
