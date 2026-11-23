import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { JSDOM, VirtualConsole } from "jsdom";

const frontendRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const referenceRoot = path.join(frontendRoot, "public", "admin-reference");
const scriptNames = [
  "qrcode-generator.js",
  "jsQR.js",
  "data.js",
  "guest_shared.js",
  "login_info.js",
  "auth.js",
  "approval.js",
  "notify.js",
  "box.js",
  "market_table.js",
  "slip_pdf.js",
  "price_code.js",
  "qr_inventory.js",
  "slip_statement.js", "master_display.js", "accessory_layout.js", "accessory_values.js",
  "dashboard.js",
  "auction_fees.js",
  "return_adjustments.js",
  "app.js",
  "consignment.js",
  "stocktake.js",
  "purchase_entry.js",
];
const scriptSources = new Map();
const loginHtml = await readFile(path.join(referenceRoot, "index.html"), "utf8");
const guestHtmlSource = await readFile(path.join(referenceRoot, "guest.html"), "utf8");
const guestCssSource = await readFile(path.join(referenceRoot, "css", "guest.css"), "utf8");
const marketCssSource = await readFile(path.join(referenceRoot, "css", "market-table.css"), "utf8");
const apiBridgeSource = await readFile(path.join(referenceRoot, "js", "api_bridge.js"), "utf8");
const authSource = await readFile(path.join(referenceRoot, "js", "auth.js"), "utf8");
const staticAppSource = await readFile(path.join(referenceRoot, "js", "app.js"), "utf8");
const consignmentSource = await readFile(path.join(referenceRoot, "js", "consignment.js"), "utf8");
const purchaseEntrySource = await readFile(path.join(referenceRoot, "js", "purchase_entry.js"), "utf8");
assert.match(marketCssSource, /\.market-currency-prefix\s*\{[\s\S]*?width:\s*54px/u,
  "market edit currency prefixes must reserve enough width for HK$");
assert.match(marketCssSource, /\.market-currency-field input\s*\{\s*padding-left:\s*62px;\s*\}/u,
  "market edit values must start after the reserved currency-prefix area");
assert.match(authSource, /function requireAdminForSensitiveOperation\(operationLabel\)/u,
  "money, price, and document mutations must share the administrator guard");
for (const operationLabel of ["商品情報・原価・売価の変更", "登録済み伝票の内容変更", "外貨レートの変更"]) {
  assert.ok(staticAppSource.includes(operationLabel), `${operationLabel} must be protected in the UI`);
}
assert.match(consignmentSource, /requireAdminForSensitiveOperation\('委託伝票の登録'\)/u,
  "consignment registration must require an administrator");
assert.match(purchaseEntrySource, /requireAdminForSensitiveOperation\('仕入伝票の確定'\)/u,
  "the offline purchase fallback must not bypass administrator approval");
assert.equal(purchaseEntrySource.includes('SKU必須'), false,
  "purchase registration must present SKU as optional");
assert.equal(purchaseEntrySource.includes('SKUを入力してください'), false,
  "purchase registration must accept blank SKU values");
assert.equal(purchaseEntrySource.includes("_peCSVHasHeader(headers, 'SKU')"), false,
  "purchase CSV import must not require an SKU column");
assert.match(purchaseEntrySource, /if \(!_peConfirmBlankSKURegistration\(\)\) return;/u,
  "purchase registration must confirm immediately before saving when an SKU is blank");
assert.ok(purchaseEntrySource.includes('SKUが空欄の商品があります。このまま進めますか'),
  "the blank-SKU confirmation must explain that registration can continue");
assert.match(staticAppSource, /modalBody\.scrollTo\(\{ top: 0/u,
  "inventory detail modal must always open at the top");
assert.match(purchaseEntrySource, /_peClearCSVImportErrors/u,
  "purchase CSV import errors must provide an explicit close action");
assert.match(purchaseEntrySource, /window\.__purchaseEntryLastCSVErrors/u,
  "purchase CSV import errors must remain visible until replaced or closed");
assert.equal(staticAppSource.includes("_prInjectInvoiceBtn"), false,
  "purchase-return bulk issue controls must not be injected into the slip summary");
assert.equal(staticAppSource.includes("slip-generic-select"), false,
  "slip rows must not restore the undefined bulk-selection UI");
assert.equal(staticAppSource.includes("slipBulkPreviewBtn"), false,
  "the slip list must not restore the removed statement-issue button");
assert.match(staticAppSource, /originalCode:\s*String\(item\.code/u,
  "product editing must retain the original code for self-excluding duplicate validation");
// Product code and bracelet payloads are exercised in product-master-reference.test.mjs.
assert.match(apiBridgeSource, /async function fetchAllPages\(path, pageSize = 100\)/u,
  "inventory API hydration must support fetching every product page");
assert.match(apiBridgeSource, /for \(let page = 2; page <= totalPages; page \+= 1\)/u,
  "product pages must be fetched sequentially through the final page");
assert.match(apiBridgeSource, /fetchAllPages\('\/products\?includeCancelled=true'\)/u,
  "admin hydration must merge all inventory pages instead of stopping at 100 products");
assert.match(apiBridgeSource, /fetchAllPages\('\/purchases'\)/u,
  "admin hydration must merge every purchase page instead of stopping at 500 slips");
assert.match(apiBridgeSource, /const linkedPurchaseProducts = APP_DATA\.inventory\.filter\(item => Boolean\(item\.purchaseSlipLineId\)\)\.length/u,
  "admin hydration must compare purchase counts only with products linked to purchase lines");
assert.match(apiBridgeSource, /reflectedPurchaseProducts !== linkedPurchaseProducts/u,
  "admin hydration must reject genuinely divergent purchase-linked datasets");
assert.match(apiBridgeSource, /const maxAttempts = retry && \['GET', 'HEAD'\]\.includes\(method\) \? 4 : 1/u,
  "safe DB reads must retry temporary network and server failures");
assert.match(apiBridgeSource, /cache: method === 'GET' \|\| method === 'HEAD' \? 'no-store' : 'default'/u,
  "DB reads must bypass browser and intermediary caches");
assert.match(apiBridgeSource, /error\.status === 503 && error\.code === 'postgres_required'/u,
  "PostgreSQL-only optional catalogs must not abort inventory hydration in the SQLite fallback");
assert.match(apiBridgeSource, /if \(hydrateAdminPromise\) return hydrateAdminPromise/u,
  "concurrent hydration requests must share one in-flight read");
assert.match(apiBridgeSource, /restoreAppData\(previousSnapshot\)/u,
  "a failed refresh must preserve the last complete DB snapshot");
assert.match(apiBridgeSource, /scheduleHydrationRetry\(error\)/u,
  "a failed DB hydration must automatically retry in the background");
assert.match(staticAppSource, /document\.addEventListener\('zaiko:data-hydrated'/u,
  "the active page must listen for DB recovery and rerender without a manual refresh");

// Exercise the transport independently from the large UI hydration graph.
// Two temporary network failures must recover without involving a page reload,
// while a mutation must never be submitted twice automatically.
const bridgeDOM = new JSDOM("<!doctype html><html><body></body></html>", {
  runScripts: "outside-only",
  url: "http://localhost/app/admin-reference/app.html",
});
const bridgeWindow = bridgeDOM.window;
const bridgeNativeTimeout = bridgeWindow.setTimeout.bind(bridgeWindow);
bridgeWindow.setTimeout = (callback, milliseconds) => milliseconds >= 10000
  ? 1
  : bridgeNativeTimeout(callback, 0);
bridgeWindow.clearTimeout = () => {};
let bridgeFetchCalls = 0;
const bridgeFetchOptions = [];
bridgeWindow.fetch = async (_url, options) => {
  bridgeFetchCalls += 1;
  bridgeFetchOptions.push(options);
  if (bridgeFetchCalls < 3) throw new bridgeWindow.TypeError("temporary network failure");
  return {
    status: 200,
    ok: true,
    headers: { get: () => null },
    text: async () => JSON.stringify({ status: "ok" }),
  };
};
bridgeWindow.eval(apiBridgeSource);
const recoveredBridgePayload = await bridgeWindow.ZaikoAPI.request("/retry-probe");
assert.equal(recoveredBridgePayload.status, "ok");
assert.equal(bridgeFetchCalls, 3, "temporary GET failures must be retried until they recover");
assert.equal(bridgeFetchOptions.at(-1).cache, "no-store", "retried GETs must bypass HTTP caches");

bridgeFetchCalls = 0;
bridgeWindow.fetch = async () => {
  bridgeFetchCalls += 1;
  throw new bridgeWindow.TypeError("write connection failed");
};
await assert.rejects(
  bridgeWindow.ZaikoAPI.request("/retry-probe", { method: "POST", body: "{}" }),
  /write connection failed/u,
);
assert.equal(bridgeFetchCalls, 1, "unsafe writes must not be retried automatically");
bridgeDOM.window.close();
assert.match(apiBridgeSource, /productByLine\.get\(line\.id\)/u,
  "API purchase slips must resolve current product details through the stable purchase-line link");
assert.match(apiBridgeSource, /brandCode: resolvedBrandCode/u,
  "purchase API payloads must support an empty brand while preserving a supplied stable brand code");
assert.match(apiBridgeSource, /product\.fixedPurchaseCostJpyMinor/u,
  "inventory hydration must use the persisted purchase-date JPY snapshot");
assert.match(apiBridgeSource, /purchaseType: \['personal', 'overseas'\]\.includes\(product\.purchaseTaxMode\)/u,
  "inventory hydration must retain the purchase category stored on the source purchase slip");
assert.match(apiBridgeSource, /supplierName: slip\.supplierName \|\| ''/u,
  "personal purchase registration must send its transaction-only supplier name to the API");
assert.match(apiBridgeSource, /supplierName: record\.supplierName \|\| ''/u,
  "purchase hydration must restore the transaction-only supplier name");
assert.match(apiBridgeSource, /supplierName: product\.supplierName \|\| ''/u,
  "inventory hydration must retain the supplier name from a personal purchase slip");
assert.equal(
  /product\.costCurrency === 'USD'\s*\?\s*Math\.round\(product\.costAmountMinor \* rate\)/u.test(apiBridgeSource),
  false,
  "inventory hydration must not recalculate overseas costs with the latest master rate",
);
assert.match(apiBridgeSource, /request\(`\/sales\/\$\{encodeURIComponent\(saleID\)\}\/issue`/u,
  "sales issue must persist its timestamp through the REST API");
assert.match(apiBridgeSource, /reason:\s*line\.reason\s*\|\|\s*record\.notes/u,
  "sales revision hydration must retain each product's reason with a legacy slip-note fallback");
assert.match(staticAppSource, /function issueSaleSlipDocument\(slipId, event\)/u,
  "sales invoices must expose the administrator issue workflow");
assert.match(staticAppSource, /売上登録時固定レート/u,
  "saved sales invoices must display the registration-time exchange rate");
assert.match(staticAppSource, /inputCurrency === 'USD' \? 'out_of_scope'/u,
  "USD sales invoices must be rendered as tax out of scope");
assert.match(staticAppSource, /accessories\.length \? `付属品:/u,
  "sales invoice descriptions must include product accessories");
assert.match(apiBridgeSource, /async function appendProductImages\(item, files = \[\]\)/u,
  "product registration must append selected images to an existing managed product");
assert.match(staticAppSource, /async function _puSaveMatchedProduct\(existingItem\)/u,
  "the product registration save action must persist staged images for an existing management number");
assert.equal(
  /if \(!filterStatus && itemStatus === '仕入返品処理済'\)/u.test(staticAppSource),
  false,
  "the all-status inventory view must include completed purchase returns for purchase reconciliation",
);
const reactEntrySource = await readFile(path.join(frontendRoot, "src", "App.jsx"), "utf8");
const reactMainSource = await readFile(path.join(frontendRoot, "src", "main.jsx"), "utf8");
assert.equal(reactEntrySource.includes("<iframe"), false, "the canonical React entry must not frame the reference UI");
assert.match(reactEntrySource, /const REFERENCE_ROOT = "\/app\/admin-reference\/"/u);
assert.match(reactEntrySource, /mountRef\.current\.innerHTML = source\.body\.innerHTML/u, "the reference design must mount into the same DOM");
assert.match(reactMainSource, /import Chart from "chart\.js\/auto"/u, "the canonical UI must bundle Chart.js instead of depending on a CDN");
assert.match(reactMainSource, /window\.Chart = Chart/u);
assert.match(loginHtml, /作業者としてログイン/u);
assert.equal(loginHtml.includes("作業員"), false, "login role name must be 作業者");
assert.match(loginHtml, /guest\.html\?id=/u, "guest login must lead to the guest catalog");
assert.match(loginHtml, /js\/login_info\.js/u, "login page must hydrate administrator-managed credentials");
assert.match(guestHtmlSource, /submitGuestPurchaseRequest\(\)/u);
assert.doesNotMatch(guestHtmlSource, /guest-box-filter/u, "guest catalog must not expose BOX selection");
assert.match(guestCssSource, /@media \(max-width: 520px\)/u);
assert.match(guestCssSource, /overflow-x: hidden/u);
assert.match(marketCssSource, /\.market-date-range\s*\{[\s\S]*?grid-template-columns:\s*minmax\(0,\s*1fr\)\s+auto\s+minmax\(0,\s*1fr\)/u,
  "market date range must allow both date inputs to shrink inside their grid cell");
assert.match(marketCssSource, /\.market-date-range \.inv-filter-input\s*\{[\s\S]*?min-width:\s*0/u,
  "market date inputs must not overlap the adjacent brand filter");
assert.match(marketCssSource, /\.market-table th,[\s\S]*?overflow-wrap:\s*anywhere[\s\S]*?text-overflow:\s*clip[\s\S]*?white-space:\s*normal/u,
  "market table text columns must show complete values instead of ellipsis truncation");
assert.match(staticAppSource, /record\.meaning \? `\$\{record\.name\}（\$\{record\.meaning\}）` : record\.name/u,
  "inventory marking search must display both the symbol and its meaning");
assert.match(staticAppSource, /index === 0 \? ' customs-document-image' : ''/u,
  "the first product image must be marked as the customs document image");
assert.match(staticAppSource, /customsBadge\.textContent = '通関画像に使用予定'/u,
  "the customs image must have a clear Japanese label");
assert.match(staticAppSource, /function openProductImageLightbox\(/u,
  "registered product images must support an enlarged preview");
assert.match(staticAppSource, /i === 0 \? ' customs-document-image' : ''/u,
  "product registration must mark image slot one as the customs document image");
assert.match(staticAppSource, /customsBadge\.className = 'image-slot-customs-badge'/u,
  "product registration must label image slot one for customs use");
assert.match(staticAppSource, /function _findCustomsInventoryItem\(line = \{\}\)/u,
  "customs PDF download must resolve saved shipment items back to inventory records");
assert.match(staticAppSource, /fetch\(`\/api\/v1\/products\/\$\{encodeURIComponent\(item\.productId\)\}\/files`/u,
  "customs image PDF download must reload product files when the initial image URL is missing");
assert.match(staticAppSource, /credentials: 'same-origin', cache: 'no-store'/u,
  "customs image requests must include the authenticated same-origin session");
assert.match(staticAppSource, /const tableWidth = 794;[\s\S]*?const columns = \[24, 62, 48, 70, 62, 62, 70, 76, 58, 60, 126, 76\]/u,
  "customs document columns must fit exactly inside the A4 landscape printable width");
assert.match(staticAppSource, /_pdfStrokeRect\(tableX, tableBottom, tableWidth, headerTop - tableBottom/u,
  "customs document details must have a visible outer border");
assert.match(staticAppSource, /columns\.slice\(0, -1\)\.forEach\(width => \{[\s\S]*?_pdfLine\(gridX, tableBottom, gridX, headerTop/u,
  "customs document fields must be separated by vertical grid lines");
assert.match(staticAppSource, /const _pdfWrap = \(value, maxChars, maxLines = 2\)/u,
  "customs document values must wrap inside their assigned cells");
assert.match(staticAppSource, /const SEARCHABLE_PARTNER_SELECT_IDS = Object\.freeze/u,
  "growing business-partner selects must share one searchable control");
for (const partnerSelectId of ["pe-supplier", "pu-supplier", "ie-supplier", "sl-buyer", "sh-dest", "co-dest", "slip-filter-party"]) {
  assert.match(staticAppSource, new RegExp(`'${partnerSelectId}'`),
    `${partnerSelectId} must be registered as a searchable business-partner select`);
}
assert.match(staticAppSource, /button\.textContent = record\.label/u,
  "partner candidates must display the business name without exposing the internal code");

let html = await readFile(path.join(referenceRoot, "app.html"), "utf8");
assert.doesNotMatch(html, /id="slipBulkPreviewBtn"/u,
  "the slip list must omit the statement-issue button from its toolbar");
assert.doesNotMatch(html, /id="slipBulkControls"/u,
  "the slip summary must omit bulk download and print controls");
assert.match(html, /\.cd-items-table tbody td\s*\{[\s\S]*?border: 1px solid #9eabb8;[\s\S]*?overflow-wrap: anywhere;/u,
  "customs preview rows must visibly separate and wrap every field");
html = html.replace(/<link\b[^>]*>/gi, "");
html = html.replace(/<script\b[^>]*src="https?:\/\/[^\"]+"[^>]*><\/script>/gi, "");

for (const name of scriptNames) {
  const source = await readFile(path.join(referenceRoot, "js", name), "utf8");
  scriptSources.set(name, source);
  const tag = `<script src="js/${name}"></script>`;
  const escapedName = name.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const tagPattern = new RegExp(`<script src="js/${escapedName}(?:\\?[^\"]*)?"></script>`);
  assert.ok(tagPattern.test(html), `app.html is missing ${tag}`);
  html = html.replace(tagPattern, () => `<script>\n${source}\n</script>`);
}

const runtimeErrors = [];
const virtualConsole = new VirtualConsole();
virtualConsole.on("jsdomError", (error) => runtimeErrors.push(error));
virtualConsole.on("error", (...args) => runtimeErrors.push(new Error(args.join(" "))));

const dom = new JSDOM(html, {
  url: "http://127.0.0.1:8080/app/admin-reference/app.html",
  runScripts: "dangerously",
  pretendToBeVisual: true,
  virtualConsole,
  beforeParse(window) {
    window.sessionStorage.setItem("inv_session", JSON.stringify({
      id: "U001",
      role: "admin",
      name: "管理者",
      avatar: "管",
      loginId: "admin",
    }));
    class ChartStub {
      constructor(_context, config) {
        this.destroyed = false;
        this.config = config;
      }
      destroy() {
        this.destroyed = true;
      }
    }

    window.Chart = ChartStub;
    window.alert = () => {};
    window.confirm = () => true;
    window.prompt = () => "";
    window.print = () => {};
    window.open = () => null;
    window.scrollTo = () => {};
    window.matchMedia = () => ({
      matches: false,
      addEventListener() {},
      removeEventListener() {},
      addListener() {},
      removeListener() {},
    });
    window.ResizeObserver = class {
      observe() {}
      unobserve() {}
      disconnect() {}
    };
    window.URL.createObjectURL = () => "blob:reference-test";
    window.URL.revokeObjectURL = () => {};
    window.HTMLElement.prototype.scrollIntoView = () => {};
    Object.defineProperty(window.navigator, "clipboard", {
      configurable: true,
      value: {
        async writeText(value) {
          window.__lastClipboardText = value;
        },
      },
    });
    Object.defineProperty(window.HTMLCanvasElement.prototype, "getContext", {
      configurable: true,
      value() {
        return { canvas: this };
      },
    });
  },
});

await new Promise((resolve) => {
  if (dom.window.document.readyState === "complete") {
    resolve();
    return;
  }
  dom.window.addEventListener("load", resolve, { once: true });
});
await new Promise((resolve) => setTimeout(resolve, 25));

const { window } = dom;
// Canvas rendering is exercised separately in the real-browser PDF artifact test.
window.buildSlipPdfBlob = async () => new window.Blob(['%PDF-1.4\nPDF fixture'], {type:'application/pdf'});
const terminologyFixture=window.document.createElement('div');
terminologyFixture.innerHTML='<label>仕入れ先</label><table><tbody><tr><td>海外仕入れ株式会社</td></tr></tbody></table><select><option>国内仕入れ株式会社</option></select><button role="option">海外仕入れ株式会社</button>';
window.applyUnifiedTerminology(terminologyFixture);
assert.equal(terminologyFixture.querySelector('label').textContent,'仕入先');
assert.equal(terminologyFixture.querySelector('td').textContent,'海外仕入れ株式会社');
assert.equal(terminologyFixture.querySelector('option').textContent,'国内仕入れ株式会社');
assert.equal(terminologyFixture.querySelector('[role="option"]').textContent,'海外仕入れ株式会社');

const { document } = window;

assert.deepEqual(runtimeErrors, [], `reference boot emitted errors: ${runtimeErrors.map(String).join("\n")}`);
assert.equal(typeof window.navigateTo, "function", "navigateTo must be globally callable");
assert.equal(typeof window.openNotification, "function", "notification items must be globally actionable");
assert.equal(typeof window.showItemDetail, "function", "showItemDetail must be globally callable");
for (const partnerSelectId of ["pe-supplier", "pu-supplier", "ie-supplier", "inv-f-supplier", "sl-buyer", "sh-dest", "co-dest", "slip-filter-party"]) {
  const select = document.getElementById(partnerSelectId);
  assert.equal(select?.dataset.partnerSearchEnhanced, "true", `${partnerSelectId} must be enhanced for free-word search`);
  assert.ok(document.querySelector(`input[data-partner-search-for="${partnerSelectId}"]`),
    `${partnerSelectId} must expose a searchable combobox input`);
}
// Master codes must resolve to the original select value, including unpadded input.
{
  const data = window.eval("APP_DATA");
  const originalMovements = data.movements;
  const select = document.createElement("select");
  select.id = "movement-code-test";
  document.body.appendChild(select);
  try {
    data.movements = [{ code: "0001", name: "AT" }, { code: "0010", name: "QZ" }, { code: "0003", name: "HW" }];
    window.populateProductSpecMasterSelect(select.id, "movement");
    const control = select._partnerSearchController;
    let changes = 0;
    select.addEventListener("change", () => changes++);
    const type = value => {
      control.input.focus();
      control.input.value = value;
      control.input.dispatchEvent(new window.Event("input", { bubbles: true }));
    };
    const key = value => control.input.dispatchEvent(new window.KeyboardEvent("keydown", { key: value, bubbles: true }));
    for (const value of ["1", "0001", "１"]) {
      type(value);
      assert.deepEqual([...control.panel.querySelectorAll("button")].map(button => button.dataset.value), ["0001"], "numeric code must not select 0010 by substring");
      key("Enter");
      assert.equal(select.value, "0001");
      assert.equal(control.input.value, "AT");
    }
    type("3"); key("Tab");
    assert.equal(select.value, "0003", "Tab must commit an exact master code");
    type("10");
    document.body.dispatchEvent(new window.Event("pointerdown", { bubbles: true }));
    assert.equal(select.value, "0010", "leaving the field must commit an exact code");
    type("9999"); key("Enter");
    assert.equal(select.value, "", "unknown master code must not select another record");
    assert.equal(control.panel.querySelectorAll("button").length, 0);
    type("HW"); key("Enter");
    assert.equal(select.value, "0003", "name input must remain usable");
    assert.ok(changes > 0, "selection must notify existing save handlers");
    window.populateProductSpecMasterSelect(select.id, "movement", { selected: "0001" });
    assert.equal(select._partnerSearchController, control, "refresh must reuse the control");
    assert.equal(control.input.value, "AT", "programmatic selection must update visible value");
    select.disabled = true;
    control.sync(true);
    assert.equal(control.input.disabled, true);
    type("3"); key("Enter");
    assert.equal(select.value, "0001", "disabled selectors must not change");
  } finally {
    data.movements = originalMovements;
    select._partnerSearchController?.wrapper.remove();
  }
}

// Code lookup must preserve name-valued fields and company role identifiers.
{
  const data = window.eval("APP_DATA");
  const original = Object.fromEntries(["brandRecords", "staffRecords", "beltMaterialRecords", "materials", "conditions", "shapeRecords", "markingRecords", "suppliers", "accessoryRecords"].map(key => [key, data[key]]));
  const host = document.createElement("div");
  document.body.appendChild(host);
  const chooseCode = (select, query = "1") => {
    const control = select._partnerSearchController;
    assert.ok(control, select.id + " must have a code entry control");
    control.input.focus();
    control.input.value = query;
    control.input.dispatchEvent(new window.Event("input", { bubbles: true }));
    control.input.dispatchEvent(new window.KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
  };
  try {
    data.brandRecords = [{code:"0001",name:"ROLEX",reading:"ロレックス"}];
    data.staffRecords = [{code:"0001",name:"バイヤー①"}];
    data.beltMaterialRecords = [{code:"0001",name:"SS"}];
    data.materials = [{code:"0001",name:"SS"}];
    data.conditions = [{code:"0001",name:"A"}];
    data.shapeRecords = [{code:"0001",name:"腕時計"}];
    data.markingRecords = [{code:"0001",name:"♡"}];
    data.suppliers = [{code:"S901",partnerCode:"0001",name:"コード検索テスト会社"}];
    data.accessoryRecords = [{code:"0001",name:"Guarantee(P)"},{code:"0002",name:"Guarantee(C)"}];
    const cases = [
      ["brand", "ROLEX", select => window.populateBrandMasterSelect(select.id)],
      ["staff", "バイヤー①", select => window.populateStaffMasterSelect(select.id)],
      ["belt", "SS", select => window.populateBeltMaterialMasterSelect(select.id)],
      ["condition", "0001", select => window.populateConditionMasterSelect(select.id)],
      ["partner", "S901", select => window.populateSupplierMasterSelect(select.id)],
      ...["material","shape","marking"].map(type => [type,"0001",select => window.populateProductSpecMasterSelect(select.id,type)]),
    ];
    for (const [type, expected, populate] of cases) {
      const select = document.createElement("select");
      select.id = "master-lookup-test-" + type;
      host.appendChild(select);
      populate(select);
      chooseCode(select);
      assert.equal(select.value, expected, type + " must retain the original storage value");
    }
    const dynamic = document.createElement("select");
    dynamic.id = "market-draft-brand-999";
    dynamic.innerHTML = '<option value=""></option><option value="ROLEX">ROLEX</option>';
    host.appendChild(dynamic);
    window.enhanceSearchablePartnerSelects();
    chooseCode(dynamic, "0001");
    assert.equal(dynamic.value, "ROLEX", "dynamic draft row must support master code lookup");
    const group = document.createElement("div");
    group.id = "market-draft-accessory-menu-999";
    group.innerHTML = '<input type="checkbox" value="Guarantee(P)" checked><input type="checkbox" value="Guarantee(C)">';
    host.appendChild(group);
    let accessoryChanges = 0;
    group.addEventListener("change", () => accessoryChanges++);
    window.enhanceSearchablePartnerSelects();
    chooseCode(group._masterAccessoryPicker, "2");
    assert.deepEqual([...group.querySelectorAll('input[type="checkbox"]:checked')].map(input => input.value), ["Guarantee(P)","Guarantee(C)"], "adding an accessory by code must retain earlier selections");
    assert.ok(accessoryChanges > 0);
    window.enhanceSearchablePartnerSelects();
    assert.equal(group._masterAccessoryPicker.disabled, true, "already selected accessories must not be offered again");
  } finally {
    Object.assign(data, original);
    host.remove();
  }
}

const supplierSearch = document.querySelector('input[data-partner-search-for="pe-supplier"]');
window.init_purchase_entry();
supplierSearch.focus();
supplierSearch.value = "ゴールデン";
supplierSearch.dispatchEvent(new window.Event("input", { bubbles: true }));
const supplierCandidates = [...document.querySelectorAll('#partner-search-panel-pe-supplier .partner-search-option')];
assert.deepEqual(supplierCandidates.map((button) => button.textContent.trim()), ["ゴールデンウォッチ"],
  "supplier free-word search must narrow the visible candidates by name");
assert.equal(supplierCandidates[0].textContent.includes("S003"), false,
  "supplier search candidates must not display internal codes");
supplierCandidates[0].click();
assert.equal(document.getElementById("pe-supplier").value, "S003",
  "choosing a searched supplier must update the original select value");

document.getElementById("slipReviseFields").innerHTML = window.buildReviseFields(
  "shipping",
  window.eval("APP_DATA.shipments.find((record) => record.id === 'SH-2026-0004')"),
);
await new Promise((resolve) => setTimeout(resolve, 0));
assert.equal(document.getElementById("rv-destination")?.dataset.partnerSearchEnhanced, "true",
  "dynamically rendered slip-revision partner selects must also support free-word search");
assert.ok(window.doAppLogin("admin", "admin123"), "reference admin login must succeed");
assert.equal(window.currentRole(), "admin");
assert.equal(window.currentUser()?.name, "管理者");
window.applyRoleUI();

const approvalNotification = window.eval("APP_DATA.notifications.find((item) => item.type === 'approval_request')");
assert.ok(approvalNotification, "admin preview data must include an approval notification");
document.getElementById("notifyPanel").classList.remove("hidden");
await window.openNotification(approvalNotification.id);
assert.equal(document.getElementById("page-approval").classList.contains("hidden"), false,
  "an approval notification must navigate administrators to approval management");
assert.equal(document.getElementById("notifyPanel").classList.contains("hidden"), true,
  "opening an approval notification must close the notification panel");
window.navigateTo("dashboard");

const pages = [
  "dashboard",
  "market",
  "market-entry",
  "inventory",
  "purchase-entry",
  "purchase",
  "sales",
  "sales-list",
  "shipping",
  "master",
  "box",
  "performance",
  "stocktake",
  "approval",
  "purchase-list",
  "client",
  "company",
  "password",
];
const masterSubPages = new Set(["client", "company", "password"]);

for (const page of pages) {
  window.navigateTo(page);
  const target = document.getElementById(`page-${page}`);
  assert.ok(target, `page-${page} must exist`);
  const visibleTarget = masterSubPages.has(page) ? document.getElementById("page-master") : target;
  assert.equal(visibleTarget.classList.contains("hidden"), false, `${visibleTarget.id} must be visible after ${page} navigation`);

  const activePanels = [...document.querySelectorAll(".page-panel:not(.hidden)")];
  assert.deepEqual(activePanels, [visibleTarget], `only ${visibleTarget.id} may be visible after ${page} navigation`);

  const activeNavPage = masterSubPages.has(page) ? "master" : page;
  const nav = document.querySelector(`.nav-item[data-page="${activeNavPage}"]`);
  if (nav) assert.equal(nav.classList.contains("active"), true, `${activeNavPage} navigation must be active`);
}

window.navigateTo("purchase");
assert.equal(document.getElementById("registration-mode-product").getAttribute("aria-checked"), "true");
window.switchRegistrationMode("part");
assert.equal(document.getElementById("product-registration-form").classList.contains("hidden"), true,
  "part registration mode must hide the product form");
assert.equal(document.getElementById("part-registration-form").classList.contains("hidden"), false,
  "part registration mode must show the dedicated part form");
document.getElementById("part-date").value = "2026-08-29";
assert.equal(window.partAssignCode(), "P2908260001", "part codes must use P + DDMMYY + four digits");
document.getElementById("part-name").value = "PRT-001";
window.partNameChanged();
assert.equal(document.getElementById("part-detail-master-group").classList.contains("hidden"), false,
  "material parts must replace free text with the material master selector");
assert.equal(document.getElementById("part-detail-text-group").classList.contains("hidden"), true);
assert.match(document.getElementById("part-detail-master-label").textContent, /素材/u);
assert.equal(document.querySelectorAll("#part-detail-master option").length,
  window.eval("APP_DATA.materials.filter(record => record?.isActive !== false).length") + 1,
  "the detail selector must be populated from the selected part's own master");
document.getElementById("part-detail-master").value = "MAT-001";
document.getElementById("part-name").value = "PRT-006";
window.partNameChanged();
assert.equal(document.getElementById("part-bracelet-quantity-group").classList.contains("hidden"), false,
  "BRACELET PARTS must expose the bracelet quantity field");
assert.equal(document.getElementById("part-detail-text-group").classList.contains("hidden"), true,
  "BRACELET PARTS must replace free text details with bracelet quantity");
assert.equal(document.getElementById("part-detail-master-group").classList.contains("hidden"), true,
  "BRACELET PARTS must not leave a stale detail master selector visible");
window.partSetPurchaseType("personal");
assert.equal(document.getElementById("part-supplier").required, false,
  "personal part purchases may use an empty supplier");
window.partSetCurrency("USD");
assert.match(document.getElementById("part-rate-text").textContent, /1 USD/u);
document.getElementById("part-cost").value = "12,500";
document.getElementById("part-sale-price-usd").value = "4,100";
document.getElementById("part-bracelet-quantity").value = "8";
const partCountBefore = window.eval("APP_DATA.parts.length");
await window.savePartRegistration();
assert.equal(window.eval("APP_DATA.parts.length"), partCountBefore + 1,
  "part registration must persist a part in the reference runtime");
assert.equal(window.eval("APP_DATA.parts.at(-1).partCode"), "P2908260001");
assert.equal(window.eval("APP_DATA.parts.at(-1).braceletQuantity"), 8);
assert.equal(window.eval("APP_DATA.parts.at(-1).salePriceUsdMinor"), 4100,
  "part sale prices must be stored with USD as the fixed base currency");
window.switchRegistrationMode("product");
assert.equal(document.getElementById("product-registration-form").classList.contains("hidden"), false,
  "switching back must restore the product form");

assert.equal(document.querySelector('.nav-item[data-page="login-info"]'), null, "the duplicate login information navigation must be removed");
assert.equal(document.getElementById("page-login-info"), null, "the duplicate login information page must be removed");
window.navigateTo("master");
assert.doesNotMatch(document.getElementById("masterTabList").textContent, /パーツ名/u,
  "the unused part-name master must not appear in master registration");
assert.equal(window.eval("APP_DATA.clientCompanies.length"), 9, "all four buyers and five suppliers must be represented in the shared client-company directory");
assert.equal(window.eval("APP_DATA.buyers.every(buyer => APP_DATA.clientCompanies.some(company => company.buyerCode === buyer.code))"), true, "every buyer must link to a client company");
assert.equal(window.eval("APP_DATA.suppliers.every(supplier => APP_DATA.clientCompanies.some(company => company.supplierCode === supplier.code))"), true, "every supplier must link to a client company");
assert.equal(window.eval("APP_DATA.clientCompanies.find(company => company.supplierCode === 'S001').invoice"), "T0001234567890", "supplier invoice data must flow into the client-company directory");
assert.equal(window.eval("APP_DATA.clientCompanies.find(company => company.buyerCode === 'B004').guestId"), "G001", "guest-issued buyers must link to the same client company");
assert.equal(window.eval("APP_DATA.purchaseRequests.every(request => /^B\\d+$/u.test(request.buyerCode))"), true, "purchase requests must persist a fixed buyer code");
assert.equal(window.eval("APP_DATA.purchaseRequests.every(request => /^(?:CLI-\\d+|\\d{4})$/u.test(request.clientCompanyCode))"), true, "purchase requests must persist a fixed client-company code");
const legacyPartyRequest = {
  id: "PR-LEGACY-CODE",
  guestId: "G001",
  guestName: "ウォッチマート",
  date: "2026-01-01 00:00",
  status: "未対応",
  items: [],
};
window.eval("APP_DATA.purchaseRequests").push(legacyPartyRequest);
assert.equal(window.syncPurchaseRequestPartyCodes(), 1, "legacy requests must migrate by guest id");
assert.equal(legacyPartyRequest.buyerCode, "B004", "a same-name company must not override the guest's buyer code");
assert.equal(legacyPartyRequest.clientCompanyCode, "CLI-001");
window.eval("APP_DATA.purchaseRequests").pop();
window.switchMasterTab("client");
assert.match(document.getElementById("masterContentArea").textContent, /取引先コード/u);
assert.match(document.getElementById("masterContentArea").textContent, /取引区分/u);
assert.match(document.getElementById("masterContentArea").textContent, /インボイス番号/u);
assert.equal(document.querySelectorAll('#mClient-tbody tr[data-trade-types]').length, 9);
assert.match(document.getElementById("mClientTradeFilter").textContent, /取引区分/u);
assert.equal(document.getElementById("mClientTradeFilterCount").textContent.trim(), "表示 9 / 全 9 件");
window.setClientCompanyTradeFilter("supplier", false);
window.setClientCompanyTradeFilter("other", false);
assert.equal(document.querySelectorAll('#mClient-tbody tr[data-trade-types]').length, 4, "the buyer filter must show buyer-linked companies only");
assert.equal([...document.querySelectorAll('#mClient-tbody tr[data-trade-types]')].every(row => row.dataset.tradeTypes.split(',').includes('buyer')), true);
assert.equal(document.getElementById("mClientTradeFilterCount").textContent.trim(), "表示 4 / 全 9 件");
window.setClientCompanyTradeFilter("buyer", false);
window.setClientCompanyTradeFilter("supplier", true);
assert.equal(document.querySelectorAll('#mClient-tbody tr[data-trade-types]').length, 6, "the supplier filter must show supplier-linked companies only");
assert.equal([...document.querySelectorAll('#mClient-tbody tr[data-trade-types]')].every(row => row.dataset.tradeTypes.split(',').includes('supplier')), true);
assert.equal(document.getElementById("mClientTradeFilterCount").textContent.trim(), "表示 6 / 全 9 件");
window.setClientCompanyTradeFilter("buyer", true);
window.setClientCompanyTradeFilter("other", true);
assert.equal(document.querySelectorAll('#mClient-tbody tr[data-trade-types]').length, 9, "the all filter must restore the full directory");
const initialChronosClientId = window.eval("APP_DATA.clientCompanies.find(company => company.buyerCode === 'B004').id");
window.showClientModal(initialChronosClientId);
assert.equal(document.getElementById("clientModal-code").readOnly, true, "client company codes must be fixed");
assert.equal(document.getElementById("clientModal-roleBuyer").checked, true);
assert.equal(document.getElementById("clientModal-roleSupplier").checked, true, "the sample partner must demonstrate multiple trade categories");
assert.equal(document.getElementById("clientModal-regionDomestic").checked, true);
assert.equal(document.getElementById("clientModal-invoice").value, "T0007777888899");
window.closeClientModal();
assert.equal([...document.querySelectorAll("#masterTabList .master-nav-item")].some(item => item.textContent.trim() === "仕入先"), false, "the legacy supplier edit master must be removed");
assert.equal([...document.querySelectorAll("#masterTabList .master-nav-item")].some(item => item.textContent.trim() === "販売先"), false, "the legacy buyer edit master must be removed");
window.switchMasterTab("brand");
const initialBrandCount = window.eval("APP_DATA.brands.length");
assert.equal(window.eval("APP_DATA.brandRecords[0].code"), "BRD-001", "existing brands must migrate to fixed codes");
assert.equal(window.eval("APP_DATA.inventory.find(item => item.brand === 'ロレックス')?.brandCode"), "BRD-001", "inventory must carry the fixed brand code");
window.showAddMasterModal("brand");
assert.equal(document.getElementById("medit-code").readOnly, false);
document.getElementById("medit-name").value = "Codex Test Brand";
for (const invalidCode of ["10000", "-1", "1.5", "１", "ABCD", "BRD-100"]) {
 document.getElementById("medit-code").value = invalidCode;
 await window.saveMasterEdit();
 assert.equal(window.eval("APP_DATA.brands.length"), initialBrandCount, invalidCode + " must be rejected");
}

document.getElementById("medit-code").value = "42";
document.getElementById("medit-reading").value = "ブランド<読み>";
window.saveMasterEdit();
assert.equal(window.eval("APP_DATA.brands.includes('Codex Test Brand')"), true, "brand master additions must update APP_DATA");
const fixedAddedBrandCode = window.eval("APP_DATA.brandRecords.find(brand => brand.name === 'Codex Test Brand').code");
assert.equal(fixedAddedBrandCode, "0042", "short numeric brand codes must be padded on save");
assert.equal(window.eval("APP_DATA.brandRecords.find(brand => brand.code === '0042').reading"), "ブランド<読み>");
assert.match(document.getElementById("masterContentArea").textContent, /ブランド<読み>/u);
assert.ok(document.getElementById("masterContentArea").innerHTML.includes("ブランド&lt;読み&gt;"));
for (const selectId of ["market-f-brand", "inv-f-brand", "pu-brand", "pep-brand"]) {
  assert.ok([...document.getElementById(selectId).options].some(option => option.value === "Codex Test Brand"), `${selectId} must use the shared brand master`);
}
assert.match(window.localStorage.getItem("inv_brand_master_v1"), /Codex Test Brand/u, "brand master changes must persist locally");

window.showAddMasterModal("brand");
document.getElementById("medit-code").value = "42";
document.getElementById("medit-name").value = "Duplicate numeric test";
await window.saveMasterEdit();
assert.equal(window.eval("APP_DATA.brands.length"), initialBrandCount + 1, "42 must conflict with existing 0042 after normalization");
window.closeMasterEditModal();
window.showEditMasterModal("brand", initialBrandCount);
assert.equal(document.getElementById("medit-reading").value, "ブランド<読み>");
document.getElementById("medit-reading").value = "ブランド読み更新";
document.getElementById("medit-name").value = "Codex Renamed Brand";
window.saveMasterEdit();
assert.equal(window.eval("APP_DATA.brands.includes('Codex Test Brand')"), false);
assert.equal(window.eval("APP_DATA.brands.includes('Codex Renamed Brand')"), true);
assert.equal(window.eval("APP_DATA.brandRecords.find(brand => brand.name === 'Codex Renamed Brand').code"), fixedAddedBrandCode, "renaming must preserve the brand code");
assert.ok([...document.getElementById("pu-brand").options].some(option => option.value === "Codex Renamed Brand"), "brand edits must immediately refresh product registration");

const rolexBrandIndex = window.eval("APP_DATA.brands.indexOf('ロレックス')");
window.showEditMasterModal("brand", rolexBrandIndex);
document.getElementById("medit-name").value = "Rolex Sync Test";
window.saveMasterEdit();
assert.equal(window.eval("APP_DATA.inventory.some(item => item.brand === 'ロレックス')"), false, "brand edits must update inventory references");
assert.equal(window.eval("APP_DATA.inventory.some(item => item.brand === 'Rolex Sync Test')"), true);
assert.equal(window.eval("APP_DATA.inventory.find(item => item.brand === 'Rolex Sync Test')?.brandCode"), "BRD-001", "renaming a brand must preserve inventory code references");
assert.equal(window.eval("APP_DATA.marketPrices.some(item => item.brand === 'Rolex Sync Test')"), true, "brand edits must update market table references");
const brandCountBeforeBlockedDelete = window.eval("APP_DATA.brands.length");
window.showDeleteMasterModal("brand", rolexBrandIndex);
window.confirmMasterDelete();
assert.equal(window.eval("APP_DATA.brands.length"), brandCountBeforeBlockedDelete, "brands referenced by inventory or market data must not be deleted");
window.showEditMasterModal("brand", rolexBrandIndex);
document.getElementById("medit-name").value = "ロレックス";
window.saveMasterEdit();

window.loadBrandMasterDirectory();
assert.equal(window.eval("APP_DATA.brandRecords.find(brand => brand.code === '0042').reading"), "ブランド読み更新", "reading must survive storage reload");
const temporaryBrandIndex = window.eval("APP_DATA.brands.indexOf('Codex Renamed Brand')");
window.showDeleteMasterModal("brand", temporaryBrandIndex);
window.confirmMasterDelete();
assert.equal(window.eval("APP_DATA.brands.length"), initialBrandCount, "unused brands must be removable from the shared master");
assert.equal([...document.getElementById("market-f-brand").options].some(option => option.value === "Codex Renamed Brand"), false, "deleted brands must disappear from search options");
window.showAddMasterModal("brand");
document.getElementById("medit-name").value = "Codex Next Brand";
window.saveMasterEdit();
const nextFixedBrandCode = window.eval("APP_DATA.brandRecords.find(brand => brand.name === 'Codex Next Brand').code");
assert.notEqual(nextFixedBrandCode, fixedAddedBrandCode, "deleted brand codes must never be reused");
assert.ok(Number(nextFixedBrandCode.replace("BRD-", "")) > Number(fixedAddedBrandCode.replace("BRD-", "")), "brand codes must be monotonically allocated");
const persistedBrandDirectory = JSON.parse(window.localStorage.getItem("inv_brand_master_v1"));
assert.equal(persistedBrandDirectory.brandRecords.find(brand => brand.name === "Codex Next Brand").code, nextFixedBrandCode, "fixed codes must persist with the brand directory");
window.showDeleteMasterModal("brand", window.eval("APP_DATA.brands.indexOf('Codex Next Brand')"));
window.confirmMasterDelete();
assert.equal(window.eval("APP_DATA.brands.length"), initialBrandCount);
window.localStorage.removeItem("inv_brand_master_v1");
window.eval("APP_DATA.brandAliases = {};");

window.switchMasterTab("supplier");
const initialSupplierCount = window.eval("APP_DATA.suppliers.length");
window.showAddMasterModal("supplier");
document.getElementById("medit-code").value = "S999";
document.getElementById("medit-name").value = "Codex Test Supplier";
document.getElementById("medit-address").value = "Tokyo";
document.getElementById("medit-contact").value = "03-9999-9999";
document.getElementById("medit-invoice").value = "T9999999999";
window.saveMasterEdit();
assert.equal(window.eval("APP_DATA.suppliers.some(supplier => supplier.code === 'S999')"), true, "supplier master additions must update APP_DATA");
assert.equal(window.eval("APP_DATA.clientCompanies.some(company => company.supplierCode === 'S999' && company.invoice === 'T9999999999')"), true, "supplier additions must create the linked client company with invoice data");
for (const selectId of ["inv-f-supplier", "pu-supplier", "pe-supplier"]) {
  assert.ok([...document.getElementById(selectId).options].some(option => option.value === "S999"), `${selectId} must use the shared supplier master`);
}
assert.match(window.localStorage.getItem("inv_supplier_master_v1"), /Codex Test Supplier/u, "supplier master changes must persist locally");

window.showEditMasterModal("supplier", initialSupplierCount);
document.getElementById("medit-code").value = "S998";
document.getElementById("medit-name").value = "Codex Renamed Supplier";
window.saveMasterEdit();
assert.equal(window.eval("APP_DATA.suppliers.some(supplier => supplier.code === 'S999')"), false);
assert.equal(window.eval("APP_DATA.suppliers.some(supplier => supplier.code === 'S998')"), true);
assert.equal(window.eval("APP_DATA.clientCompanies.some(company => company.supplierCode === 'S999')"), false);
assert.equal(window.eval("APP_DATA.clientCompanies.some(company => company.supplierCode === 'S998' && company.companyName === 'Codex Renamed Supplier')"), true, "supplier edits must update the linked client company");
assert.ok([...document.getElementById("pu-supplier").options].some(option => option.value === "S998"), "supplier edits must immediately refresh product registration");

const supplierS001Index = window.eval("APP_DATA.suppliers.findIndex(supplier => supplier.code === 'S001')");
window.showEditMasterModal("supplier", supplierS001Index);
document.getElementById("medit-code").value = "S101";
window.saveMasterEdit();
assert.equal(window.eval("APP_DATA.inventory.some(item => item.supplier === 'S001')"), false, "supplier code edits must update inventory references");
assert.equal(window.eval("APP_DATA.inventory.some(item => item.supplier === 'S101')"), true);
const supplierCountBeforeBlockedDelete = window.eval("APP_DATA.suppliers.length");
window.showDeleteMasterModal("supplier", supplierS001Index);
window.confirmMasterDelete();
assert.equal(window.eval("APP_DATA.suppliers.length"), supplierCountBeforeBlockedDelete, "suppliers referenced by inventory or slips must not be deleted");
window.showEditMasterModal("supplier", supplierS001Index);
document.getElementById("medit-code").value = "S001";
window.saveMasterEdit();

const temporarySupplierIndex = window.eval("APP_DATA.suppliers.findIndex(supplier => supplier.code === 'S998')");
window.showDeleteMasterModal("supplier", temporarySupplierIndex);
window.confirmMasterDelete();
assert.equal(window.eval("APP_DATA.suppliers.length"), initialSupplierCount, "unused suppliers must be removable from the shared master");
assert.equal(window.eval("APP_DATA.clientCompanies.some(company => company.supplierCode === 'S998')"), false, "removing an unused supplier must remove its automatic client-company link");
window.localStorage.removeItem("inv_supplier_master_v1");
window.eval("APP_DATA.supplierAliases = {};");

const initialStaffCount = window.eval("APP_DATA.staffRecords.length");
assert.equal(initialStaffCount, 5, "the five purchase staff members must be loaded as stable master records");
assert.deepEqual(
  [...window.eval("APP_DATA.staffRecords.map(record => record.code)")],
  ["BUY-001", "BUY-002", "BUY-003", "BUY-004", "BUY-005"],
  "purchase staff codes must remain stable instead of being derived from the current row index",
);
const independentUsersBefore = JSON.stringify(window.eval("APP_DATA.users"));
window.switchMasterTab("staff");
assert.match(document.getElementById("masterContentArea").textContent, /バイヤーマスタ/u);
assert.equal(document.querySelectorAll("#masterContentArea tbody tr").length, 5);
window.showAddMasterModal("staff");
assert.equal(document.getElementById("medit-code").value, "0006", "the next stable staff code must be proposed automatically");
document.getElementById("medit-code").value = "6";
document.getElementById("medit-name").value = "共通担当者テスト";
window.saveMasterEdit();
assert.equal(JSON.stringify(window.eval("APP_DATA.users")), independentUsersBefore, "buyer changes must leave accounts unchanged");
for (const selectId of ["inv-f-staff", "pu-staff", "pe-staff"]) {
  assert.equal([...document.getElementById(selectId).options].some(option => option.value === "共通担当者テスト"), true, `${selectId} must use the shared purchase staff master`);
}
const temporaryStaffIndex = window.eval("APP_DATA.staffRecords.findIndex(record => record.code === '0006')");
window.showEditMasterModal("staff", temporaryStaffIndex);
document.getElementById("medit-name").value = "共通担当者テスト更新";
window.saveMasterEdit();
assert.equal(JSON.stringify(window.eval("APP_DATA.users")), independentUsersBefore, "buyer edits must leave accounts unchanged");
const staffCountBeforeBlockedDelete = window.eval("APP_DATA.staffRecords.length");
const usedStaffIndex = window.eval("APP_DATA.staffRecords.findIndex(record => record.code === 'BUY-001')");
window.showDeleteMasterModal("staff", usedStaffIndex);
window.confirmMasterDelete();
assert.equal(window.eval("APP_DATA.staffRecords.length"), staffCountBeforeBlockedDelete, "staff referenced by inventory or slips must not be deleted");
const renamedTemporaryStaffIndex = window.eval("APP_DATA.staffRecords.findIndex(record => record.code === '0006')");
window.showDeleteMasterModal("staff", renamedTemporaryStaffIndex);
window.confirmMasterDelete();
assert.equal(window.eval("APP_DATA.staffRecords.length"), initialStaffCount, "unused staff can be removed from the shared master");
assert.equal(JSON.stringify(window.eval("APP_DATA.users")), independentUsersBefore, "buyer changes must leave accounts unchanged");

const initialMaterialCount = window.eval("APP_DATA.materials.length");
const initialMovementCount = window.eval("APP_DATA.movements.length");
const initialBeltMaterialCount = window.eval("APP_DATA.beltMaterialRecords.length");
const initialDialCount = window.eval("APP_DATA.dialRecords.length");
window.switchMasterTab("material");
assert.match(document.getElementById("masterContentArea").textContent, /共通素材マスタ/u);
window.showAddMasterModal("material");
document.getElementById("medit-code").value = "MAT-099";
document.getElementById("medit-name").value = "共通素材テスト";
window.saveMasterEdit();
window.switchMasterTab("movement");
assert.match(document.getElementById("masterContentArea").textContent, /共通駆動方式マスタ/u);
window.showAddMasterModal("movement");
document.getElementById("medit-code").value = "MOV-099";
document.getElementById("medit-name").value = "共通駆動テスト";
window.saveMasterEdit();
for (const selectId of ["market-f-material", "inv-f-material", "pu-material", "pep-material", "ie-material", "me-material"]) {
  assert.equal([...document.getElementById(selectId).options].some(option => option.value === "MAT-099"), true, `${selectId} must use the shared material master`);
}
for (const selectId of ["market-f-movement", "inv-f-movement", "pu-movement", "pep-movement", "ie-movement", "me-movement"]) {
  assert.equal([...document.getElementById(selectId).options].some(option => option.value === "MOV-099"), true, `${selectId} must use the shared movement master`);
}
assert.match(window.localStorage.getItem("inv_product_spec_master_v1"), /共通素材テスト/u, "material master changes must persist locally");
assert.match(window.localStorage.getItem("inv_product_spec_master_v1"), /共通駆動テスト/u, "movement master changes must persist locally");
window.switchMasterTab("belt");
assert.match(document.getElementById("masterContentArea").textContent, /共通ベルト素材マスタ/u);
window.showAddMasterModal("belt");
assert.equal(document.getElementById("medit-code").value, "0001");
document.getElementById("medit-name").value = "セラミック";
window.saveMasterEdit();
assert.equal(window.eval("APP_DATA.beltMaterialRecords.at(-1).code"), "0001");
assert.equal(window.eval("MASTER_TABS.some(tab => tab.key === 'dial')"), false, "dial master must be hidden");
window.eval(`APP_DATA.beltMaterialRecords.splice(${initialBeltMaterialCount}); APP_DATA.dialRecords.splice(${initialDialCount});`);

// Numeric material codes and details must survive form save, edit and shared selectors.
for (const code of ["1", "2", "6", "7"]) {
  const paddedCode = code.padStart(4, "0");
  window.switchMasterTab("material");
  window.showAddMasterModal("material");
  assert.match(document.getElementById("medit-code").value, /^[0-9]{4}$/u);
  document.getElementById("medit-code").value = code;
  document.getElementById("medit-name").value = "素材コード" + code + "テスト";
  document.getElementById("medit-meaning").value = "詳細 <テスト> & 保持";
  await window.saveMasterEdit();
  const ix = window.eval("APP_DATA.materials.findIndex(r => r.code === '" + paddedCode + "')");
  assert.ok(ix >= 0, "numeric material code should be saved with four digits");
  assert.ok(document.getElementById("masterContentArea").textContent.includes("詳細 <テスト> & 保持"));
  for (const id of ["inv-f-material", "pu-material", "pep-material"]) {
    assert.ok([...document.getElementById(id).options].some(o => o.value === paddedCode), "numeric material should propagate: " + id);
  }
  const beforeDuplicate = window.eval("APP_DATA.materials.length");
  for (const input of [code, paddedCode, "10000"]) {
    window.showAddMasterModal("material");
    document.getElementById("medit-code").value = input;
    document.getElementById("medit-name").value = "重複防止素材" + input;
    await window.saveMasterEdit();
    assert.equal(window.eval("APP_DATA.materials.length"), beforeDuplicate, "normalized duplicates and five-digit codes must be rejected");
  }
  window.showEditMasterModal("material", ix);
  assert.equal(document.getElementById("medit-meaning").value, "詳細 <テスト> & 保持");
  document.getElementById("medit-meaning").value = "更新した詳細";
  await window.saveMasterEdit();
  assert.equal(window.eval("APP_DATA.materials[" + ix + "].meaning"), "更新した詳細");
  window.showDeleteMasterModal("material", ix);
  window.confirmMasterDelete();
}

// Movement codes, details and shared selectors use the same four-digit rule.
window.switchMasterTab("movement");
window.showAddMasterModal("movement");
assert.match(document.getElementById("medit-code").value, /^[0-9]{4}$/u);
document.getElementById("medit-code").value = "1";
document.getElementById("medit-name").value = "AT詳細テスト";
document.getElementById("medit-meaning").value = "自動巻き";
await window.saveMasterEdit();
const movementFourIndex = window.eval("APP_DATA.movements.findIndex(r => r.code === '0001')");
assert.ok(movementFourIndex >= 0);
assert.match(document.getElementById("masterContentArea").textContent, /自動巻き/u);
for (const id of ["inv-f-movement", "pu-movement", "pep-movement"]) {
 assert.ok([...document.getElementById(id).options].some(o => o.value === "0001"));
}
window.showEditMasterModal("movement", movementFourIndex);
assert.equal(document.getElementById("medit-meaning").value,"自動巻き");
document.getElementById("medit-meaning").value="詳細更新";
await window.saveMasterEdit();
assert.equal(window.eval("APP_DATA.movements["+movementFourIndex+"].meaning"),"詳細更新");
window.showDeleteMasterModal("movement",movementFourIndex);
window.confirmMasterDelete();

// Shape numeric input is stored as four digits and rejects normalized duplicates.
const originalShapeRecords = window.eval("APP_DATA.shapeRecords");
window.eval("APP_DATA.shapeRecords = []");
window.switchMasterTab("shape");
window.showAddMasterModal("shape");
assert.match(document.getElementById("medit-code").value, /^[0-9]{4}$/u);
document.getElementById("medit-code").value="1";
document.getElementById("medit-name").value="形状四桁テスト";
await window.saveMasterEdit();
const shapeFourIndex=window.eval("APP_DATA.shapeRecords.findIndex(r => r.code === '0001')");
assert.ok(shapeFourIndex>=0);
const shapeFourCount=window.eval("APP_DATA.shapeRecords.length");
for (const input of ["1","0001","10000"]) {
 window.showAddMasterModal("shape");
 document.getElementById("medit-code").value=input;
 document.getElementById("medit-name").value="重複形状"+input;
 await window.saveMasterEdit();
 assert.equal(window.eval("APP_DATA.shapeRecords.length"),shapeFourCount);
}
window.showEditMasterModal("shape",shapeFourIndex);
document.getElementById("medit-name").value="形状更新テスト";
await window.saveMasterEdit();
assert.equal(window.eval("APP_DATA.shapeRecords["+shapeFourIndex+"].name"),"形状更新テスト");
window.showDeleteMasterModal("shape",shapeFourIndex);
window.confirmMasterDelete();

window.eval("APP_DATA.shapeRecords = " + (originalShapeRecords === undefined ? "undefined" : JSON.stringify(originalShapeRecords)));

// Marking numeric input is stored as four digits and rejects normalized duplicates.
const originalMarkingRecords = window.eval("APP_DATA.markingRecords");
window.eval("APP_DATA.markingRecords = []");
window.switchMasterTab("marking");
window.showAddMasterModal("marking");
assert.match(document.getElementById("medit-code").value, /^[0-9]{4}$/u);
document.getElementById("medit-code").value="1";
document.getElementById("medit-name").value="マーキング四桁テスト";
await window.saveMasterEdit();
const markingFourIndex=window.eval("APP_DATA.markingRecords.findIndex(r => r.code === '0001')");
assert.ok(markingFourIndex>=0);
const markingFourCount=window.eval("APP_DATA.markingRecords.length");
for (const input of ["1","0001","10000"]) {
 window.showAddMasterModal("marking");
 document.getElementById("medit-code").value=input;
 document.getElementById("medit-name").value="重複マーキング"+input;
 await window.saveMasterEdit();
 assert.equal(window.eval("APP_DATA.markingRecords.length"),markingFourCount);
}
window.showEditMasterModal("marking",markingFourIndex);
document.getElementById("medit-name").value="マーキング更新テスト";
await window.saveMasterEdit();
assert.equal(window.eval("APP_DATA.markingRecords["+markingFourIndex+"].name"),"マーキング更新テスト");
window.showDeleteMasterModal("marking",markingFourIndex);
window.confirmMasterDelete();

window.eval("APP_DATA.markingRecords = " + (originalMarkingRecords === undefined ? "undefined" : JSON.stringify(originalMarkingRecords)));

// Belt numeric input is stored as four digits and rejects normalized duplicates.
const originalBeltRecords = window.eval("APP_DATA.beltMaterialRecords");
window.eval("APP_DATA.beltMaterialRecords = []");
window.switchMasterTab("belt");
window.showAddMasterModal("belt");
assert.match(document.getElementById("medit-code").value, /^[0-9]{4}$/u);
document.getElementById("medit-code").value="1";
document.getElementById("medit-name").value="ベルト素材四桁テスト";
document.getElementById("medit-inventoryDetail").value="ステンレススチール";
await window.saveMasterEdit();
const beltFourIndex=window.eval("APP_DATA.beltMaterialRecords.findIndex(r => r.code === '0001')");
assert.ok(beltFourIndex>=0);
assert.equal(window.eval("APP_DATA.beltMaterialRecords["+beltFourIndex+"].inventoryDetail"),"ステンレススチール");
const beltFourCount=window.eval("APP_DATA.beltMaterialRecords.length");
for (const input of ["1","0001","10000"]) {
 window.showAddMasterModal("belt");
 document.getElementById("medit-code").value=input;
 document.getElementById("medit-name").value="重複ベルト素材"+input;
 await window.saveMasterEdit();
 assert.equal(window.eval("APP_DATA.beltMaterialRecords.length"),beltFourCount);
}
window.showEditMasterModal("belt",beltFourIndex);
assert.equal(document.getElementById("medit-inventoryDetail").value,"ステンレススチール");
document.getElementById("medit-inventoryDetail").value="詳細更新";
document.getElementById("medit-name").value="ベルト素材更新テスト";
await window.saveMasterEdit();
assert.equal(window.eval("APP_DATA.beltMaterialRecords["+beltFourIndex+"].name"),"ベルト素材更新テスト");
assert.equal(window.eval("APP_DATA.beltMaterialRecords["+beltFourIndex+"].inventoryDetail"),"詳細更新");
window.showDeleteMasterModal("belt",beltFourIndex);
window.confirmMasterDelete();

window.eval("APP_DATA.beltMaterialRecords = " + (originalBeltRecords === undefined ? "undefined" : JSON.stringify(originalBeltRecords)));

// Accessory numeric input is stored as four digits and rejects normalized duplicates.
const originalAccessoryRecords = window.eval("APP_DATA.accessoryRecords");
window.eval("APP_DATA.accessoryRecords = []");
window.switchMasterTab("accessory");
window.showAddMasterModal("accessory");
assert.match(document.getElementById("medit-code").value, /^[0-9]{4}$/u);
document.getElementById("medit-code").value="1";
document.getElementById("medit-name").value="Guarantee(P)";
document.getElementById("medit-meaning").value="紙ギャラ";
document.getElementById("medit-inventoryDetail").value="B（仕入返品伝票にも表示）";
document.getElementById("medit-overseasDetail").value="（表記無し）";
document.getElementById("medit-tagDetail").value="・";
document.getElementById("medit-domesticDetail").value="（表記無し）";
document.getElementById("medit-symbol").value="P";
document.getElementById("medit-supplement").value="初期補足";
await window.saveMasterEdit();
const accessoryFourIndex=window.eval("APP_DATA.accessoryRecords.findIndex(r => r.code === '0001')");
assert.ok(accessoryFourIndex>=0);
assert.equal(window.eval("APP_DATA.accessoryRecords["+accessoryFourIndex+"].name"),"Guarantee(P)");
window.loadAccessoryMasterDirectory();
assert.equal(window.eval("APP_DATA.accessoryRecords["+accessoryFourIndex+"].name"),"Guarantee(P)");
assert.equal(window.eval("APP_DATA.accessoryRecords["+accessoryFourIndex+"].meaning"),"紙ギャラ");
assert.equal(window.eval("APP_DATA.accessoryRecords["+accessoryFourIndex+"].symbol"),"P");
window.switchMasterTab("accessory");
assert.match(document.getElementById("masterContentArea").textContent,/詳細（札用）/u);
assert.deepEqual([...document.querySelectorAll("#masterContentArea th")].map(el=>el.textContent),["コード","詳細（商品詳細用）","詳細（在庫一覧用）","詳細（海外用）","詳細（札用）","詳細（国内用）","操作"]);
for (const [key,value] of Object.entries({inventoryDetail:'B（仕入返品伝票にも表示）',overseasDetail:'（表記無し）',tagDetail:'・',domesticDetail:'（表記無し）'})) {
 assert.equal(window.eval('APP_DATA.accessoryRecords['+accessoryFourIndex+'].'+key),value);
}
const accessoryFourCount=window.eval("APP_DATA.accessoryRecords.length");
for (const input of ["1","0001","10000"]) {
 window.showAddMasterModal("accessory");
 document.getElementById("medit-code").value=input;
 document.getElementById("medit-name").value="重複付属品"+input;
 await window.saveMasterEdit();
 assert.equal(window.eval("APP_DATA.accessoryRecords.length"),accessoryFourCount);
}
window.showEditMasterModal("accessory",accessoryFourIndex);
assert.equal(document.getElementById("medit-symbol").value,"P");
assert.equal(document.getElementById("medit-supplement").value,"初期補足");
document.getElementById("medit-supplement").value="更新補足";
document.getElementById("medit-symbol").value="○";
assert.equal(document.getElementById("medit-meaning").value,"紙ギャラ");
document.getElementById("medit-meaning").value="詳細更新";
document.getElementById("medit-name").value="Guarantee(C)";
await window.saveMasterEdit();
assert.equal(window.eval("APP_DATA.accessoryRecords["+accessoryFourIndex+"].name"),"Guarantee(C)");
assert.equal(window.eval("APP_DATA.accessoryRecords["+accessoryFourIndex+"].meaning"),"詳細更新");
window.loadAccessoryMasterDirectory();
assert.equal(window.eval("APP_DATA.accessoryRecords["+accessoryFourIndex+"].symbol"),"○");
assert.equal(window.eval("APP_DATA.accessoryRecords["+accessoryFourIndex+"].supplement"),"更新補足");
window.showDeleteMasterModal("accessory",accessoryFourIndex);
window.confirmMasterDelete();

window.eval("APP_DATA.accessoryRecords = " + (originalAccessoryRecords === undefined ? "undefined" : JSON.stringify(originalAccessoryRecords)));
window.persistAccessoryMasterDirectory();
window.refreshAccessoryMasterConsumers();

// Condition numeric input is stored as four digits and rejects normalized duplicates.
const originalConditionRecords = window.eval("APP_DATA.conditions");
window.eval("APP_DATA.conditions = []");
window.switchMasterTab("condition");
window.showAddMasterModal("condition");
assert.match(document.getElementById("medit-code").value, /^[0-9]{4}$/u);
document.getElementById("medit-code").value="1";
document.getElementById("medit-name").value="コンディション四桁テスト";
await window.saveMasterEdit();
const conditionFourIndex=window.eval("APP_DATA.conditions.findIndex(r => r.code === '0001')");
assert.ok(conditionFourIndex>=0);
window.loadConditionMasterDirectory();
assert.equal(window.eval("APP_DATA.conditions["+conditionFourIndex+"].code"),"0001");
const conditionFourCount=window.eval("APP_DATA.conditions.length");
for (const input of ["1","0001","10000"]) {
 window.showAddMasterModal("condition");
 document.getElementById("medit-code").value=input;
 document.getElementById("medit-name").value="重複コンディション"+input;
 await window.saveMasterEdit();
 assert.equal(window.eval("APP_DATA.conditions.length"),conditionFourCount);
}
window.showEditMasterModal("condition",conditionFourIndex);
document.getElementById("medit-name").value="コンディション更新テスト";
await window.saveMasterEdit();
assert.equal(window.eval("APP_DATA.conditions["+conditionFourIndex+"].name"),"コンディション更新テスト");
window.showDeleteMasterModal("condition",conditionFourIndex);
window.confirmMasterDelete();

window.eval("APP_DATA.conditions = " + (originalConditionRecords === undefined ? "undefined" : JSON.stringify(originalConditionRecords)));
window.persistConditionMasterDirectory();
window.refreshConditionMasterConsumers();

const specInventory = window.eval("APP_DATA.inventory");
const originalMaterial = specInventory[0].material;
const originalMovement = specInventory[0].movement;
const originalMarketMaterial = window.eval("APP_DATA.marketPrices[0].material");
const originalMarketMovement = window.eval("APP_DATA.marketPrices[0].movement");
specInventory[0].material = "MAT-099";
specInventory[0].movement = "MOV-099";
window.eval("APP_DATA.marketPrices[0].material = 'MAT-099'; APP_DATA.marketPrices[0].movement = 'MOV-099';");
window.switchMasterTab("material");
const temporaryMaterialIndex = window.eval("APP_DATA.materials.findIndex(record => record.code === 'MAT-099')");
window.showEditMasterModal("material", temporaryMaterialIndex);
document.getElementById("medit-code").value = "MAT-098";
document.getElementById("medit-name").value = "共通素材テスト更新";
window.saveMasterEdit();
assert.equal(specInventory[0].material, "MAT-098", "material code edits must update inventory references");
assert.equal(window.eval("APP_DATA.marketPrices[0].material"), "MAT-098", "material code edits must update market references");
window.switchMasterTab("movement");
const temporaryMovementIndex = window.eval("APP_DATA.movements.findIndex(record => record.code === 'MOV-099')");
window.showEditMasterModal("movement", temporaryMovementIndex);
document.getElementById("medit-code").value = "MOV-098";
document.getElementById("medit-name").value = "共通駆動テスト更新";
window.saveMasterEdit();
assert.equal(specInventory[0].movement, "MOV-098", "movement code edits must update inventory references");
assert.equal(window.eval("APP_DATA.marketPrices[0].movement"), "MOV-098", "movement code edits must update market references");

document.getElementById("inv-f-material").value = "MAT-098";
document.getElementById("inv-f-movement").value = "MOV-098";
window.execInventorySearch();
assert.equal(document.querySelectorAll("#inventoryTableBody tr").length, 1, "inventory search must filter by material and movement master codes");
document.getElementById("market-f-material").value = "MAT-098";
document.getElementById("market-f-movement").value = "MOV-098";
window.marketApplyFilters();
assert.equal(document.querySelectorAll("#marketTableBody tr").length, 1, "market search must filter by material and movement master codes");

window.switchMasterTab("material");
const materialCountBeforeBlockedDelete = window.eval("APP_DATA.materials.length");
window.showDeleteMasterModal("material", temporaryMaterialIndex);
window.confirmMasterDelete();
assert.equal(window.eval("APP_DATA.materials.length"), materialCountBeforeBlockedDelete, "materials referenced by inventory or market data must not be deleted");
window.switchMasterTab("movement");
const movementCountBeforeBlockedDelete = window.eval("APP_DATA.movements.length");
window.showDeleteMasterModal("movement", temporaryMovementIndex);
window.confirmMasterDelete();
assert.equal(window.eval("APP_DATA.movements.length"), movementCountBeforeBlockedDelete, "movements referenced by inventory or market data must not be deleted");

specInventory[0].material = originalMaterial;
specInventory[0].movement = originalMovement;
window.eval(`APP_DATA.marketPrices[0].material = ${JSON.stringify(originalMarketMaterial)}; APP_DATA.marketPrices[0].movement = ${JSON.stringify(originalMarketMovement)};`);
window.switchMasterTab("material");
window.showDeleteMasterModal("material", temporaryMaterialIndex);
window.confirmMasterDelete();
window.switchMasterTab("movement");
window.showDeleteMasterModal("movement", temporaryMovementIndex);
window.confirmMasterDelete();
assert.equal(window.eval("APP_DATA.materials.length"), initialMaterialCount, "unused material records must be removable");
assert.equal(window.eval("APP_DATA.movements.length"), initialMovementCount, "unused movement records must be removable");
window.resetInventorySearch();
window.marketResetFilters();
window.localStorage.removeItem("inv_product_spec_master_v1");
window.eval("APP_DATA.productSpecAliases = { material: {}, movement: {} };");

const initialAccessoryCount = window.eval("APP_DATA.accessoryRecords.length");
assert.equal(
  window.eval("APP_DATA.accessoryRecords.map(record => record.code).join(',')"),
  "ACC-001,ACC-002,ACC-003,ACC-004,ACC-005,ACC-006",
  "accessories must have stable master codes",
);
window.switchMasterTab("accessory");
assert.match(document.getElementById("masterContentArea").textContent, /共通付属品マスタ/u);
window.showAddMasterModal("accessory");
assert.equal(document.getElementById("medit-code").value, "0001", "the next accessory code must be proposed automatically");
assert.equal(document.getElementById("medit-code").readOnly, false, "new accessory codes can be entered as numbers");
document.getElementById("medit-name").value = "TEST ACCESSORY";
window.saveMasterEdit();
assert.equal(window.eval("APP_DATA.accessoryRecords.some(record => record.code === '0001' && record.name === 'TEST ACCESSORY')"), true);
assert.match(window.localStorage.getItem("inv_accessory_master_v1"), /TEST ACCESSORY/u, "accessory master changes must persist locally");
assert.equal([...document.getElementById("market-f-accessory").options].some(option => option.value === "TEST ACCESSORY"), true, "market search must use the shared accessory master");
for (const containerId of ["inv-acc-list", "pu-accessories", "pep-accessories", "ie-accessories", "me-accessories"]) {
  assert.ok(document.querySelector(`#${containerId} input[value="TEST ACCESSORY"]`), `${containerId} must use the shared accessory master`);
}

const originalInventoryAccessories = [...(specInventory[0].accessories || [])];
const originalMarketAccessories = [...window.eval("APP_DATA.marketPrices[0].accessories || []")];
specInventory[0].accessories = [...originalInventoryAccessories, "TEST ACCESSORY"];
window.eval("APP_DATA.marketPrices[0].accessories = [...APP_DATA.marketPrices[0].accessories, 'TEST ACCESSORY'];");
window.switchMasterTab("accessory");
const temporaryAccessoryIndex = window.eval("APP_DATA.accessoryRecords.findIndex(record => record.code === '0001')");
window.showEditMasterModal("accessory", temporaryAccessoryIndex);
document.getElementById("medit-name").value = "TEST ACCESSORY UPDATED";
document.getElementById("medit-symbol").value = "P";
window.saveMasterEdit();
assert.equal(window.eval("APP_DATA.accessoryRecords.find(record => record.code === '0001').name"), "TEST ACCESSORY UPDATED", "accessory edits must preserve their stable code");
assert.equal(specInventory[0].accessories.includes("TEST ACCESSORY UPDATED"), true, "accessory edits must update inventory references");
assert.equal(window.eval("APP_DATA.marketPrices[0].accessories.includes('TEST ACCESSORY UPDATED')"), true, "accessory edits must update market references");
assert.equal(document.querySelector('#pu-accessories input[value="TEST ACCESSORY UPDATED"]') !== null, true, "renamed accessories must refresh registration forms");

window.eval("_invAccFilterState = ['TEST ACCESSORY UPDATED'];");
window.execInventorySearch();
assert.equal(document.querySelectorAll("#inventoryTableBody tr").length, 1, "inventory search must filter by shared accessory names");
const inventoryAccessoryCell = document.querySelector('#inventoryTableBody [data-inv-col="accessories"]');
assert.ok(inventoryAccessoryCell.querySelector('svg[aria-label="紙ギャラ（P）"]'), "inventory must display the icon configured by its accessory master");
assert.ok(inventoryAccessoryCell.querySelector('[title="TEST ACCESSORY UPDATED"]'), "the original accessory name remains available as a tooltip");
assert.match(window.renderInventoryAccessorySymbols(['0001']), /紙ギャラ（P）/u, "code-based accessories resolve through the master too");
assert.equal(window.renderInventoryAccessorySymbols([]), '—');
assert.ok(!window.renderInventoryAccessorySymbols(['<img src=x onerror=alert(1)>']).includes('<img'), "unregistered accessory text is escaped");
window.eval("APP_DATA.accessoryRecords.find(record => record.code === '0001').symbol = 'NC'");
window.renderInventoryTable();
assert.ok(document.querySelector('#inventoryTableBody [data-inv-col="accessories"] svg[aria-label="中期カードギャラ（NC）"]'), "changing the master symbol updates inventory display");

document.getElementById("market-f-accessory").value = "TEST ACCESSORY UPDATED";
window.marketApplyFilters();
assert.equal(document.querySelectorAll("#marketTableBody tr").length, 1, "market search must filter by shared accessory names");

window.switchMasterTab("accessory");
const accessoryCountBeforeBlockedDelete = window.eval("APP_DATA.accessoryRecords.length");
window.showDeleteMasterModal("accessory", temporaryAccessoryIndex);
window.confirmMasterDelete();
assert.equal(window.eval("APP_DATA.accessoryRecords.length"), accessoryCountBeforeBlockedDelete, "accessories referenced by inventory or market data must not be deleted");

specInventory[0].accessories = originalInventoryAccessories;
window.eval(`APP_DATA.marketPrices[0].accessories = ${JSON.stringify(originalMarketAccessories)};`);
window.resetInventorySearch();
window.marketResetFilters();
window.switchMasterTab("accessory");
const renamedAccessoryIndex = window.eval("APP_DATA.accessoryRecords.findIndex(record => record.code === '0001')");
window.showDeleteMasterModal("accessory", renamedAccessoryIndex);
window.confirmMasterDelete();
assert.equal(window.eval("APP_DATA.accessoryRecords.length"), initialAccessoryCount, "unused accessories can be removed from the shared master");
assert.equal([...document.getElementById("market-f-accessory").options].some(option => option.value === "TEST ACCESSORY UPDATED"), false, "removed accessories must disappear from search choices");
window.localStorage.removeItem("inv_accessory_master_v1");
window.eval("APP_DATA.accessoryAliases = {};");

const initialConditionCount = window.eval("APP_DATA.conditions.length");
assert.equal(
  window.eval("APP_DATA.conditions.map(record => record.code).join(',')"),
  "CON-001,CON-002,CON-003,CON-004,CON-005,CON-006,CON-007",
  "conditions must have stable master codes",
);
window.switchMasterTab("condition");
assert.match(document.getElementById("masterContentArea").textContent, /共通コンディションマスタ/u);
window.showAddMasterModal("condition");
assert.equal(document.getElementById("medit-code").value, "0001", "the next condition code must be proposed automatically");
assert.equal(document.getElementById("medit-code").readOnly, false, "new condition codes accept numeric input");
document.getElementById("medit-name").value = "TEST CONDITION";
document.getElementById("medit-meaning").value = "コンディション詳細テスト";
window.saveMasterEdit();
assert.equal(window.eval("APP_DATA.conditions.some(record => record.code === '0001' && record.name === 'TEST CONDITION')"), true);
assert.match(window.localStorage.getItem("inv_condition_master_v1"), /TEST CONDITION/u, "condition master changes must persist locally");
for (const selectId of ["inv-f-condition", "market-f-condition", "pu-condition", "pep-condition", "ie-condition", "me-condition"]) {
  assert.equal([...document.getElementById(selectId).options].some(option => option.value === "0001"), true, `${selectId} must use the shared condition master`);
}

assert.deepEqual([...document.querySelectorAll("#masterContentArea th")].map(el=>el.textContent),["コード","名称","詳細","操作"]);
assert.equal(window.eval("APP_DATA.conditions.find(r=>r.code===\u00270001\u0027).meaning"),"コンディション詳細テスト");
const originalInventoryCondition = specInventory[0].condition;
const originalMarketCondition = window.eval("APP_DATA.marketPrices[0].condition");
specInventory[0].condition = "0001";
window.eval("APP_DATA.marketPrices[0].condition = '0001';");
window.switchMasterTab("condition");
const temporaryConditionIndex = window.eval("APP_DATA.conditions.findIndex(record => record.code === '0001')");
window.showEditMasterModal("condition", temporaryConditionIndex);
document.getElementById("medit-name").value = "TEST CONDITION UPDATED";
window.saveMasterEdit();
assert.equal(window.eval("APP_DATA.conditions.find(record => record.code === '0001').name"), "TEST CONDITION UPDATED", "condition names must be editable without changing their code");
assert.match(document.querySelector('#pu-condition option[value="0001"]').textContent, /TEST CONDITION UPDATED/u, "renamed condition labels must refresh registration forms");

document.getElementById("inv-f-condition").value = "0001";
window.execInventorySearch();
assert.equal(document.querySelectorAll("#inventoryTableBody tr").length, 1, "inventory search must filter by shared condition codes");
document.getElementById("market-f-condition").value = "0001";
window.marketApplyFilters();
assert.equal(document.querySelectorAll("#marketTableBody tr").length, 1, "market search must filter by shared condition codes");

window.switchMasterTab("condition");
const conditionCountBeforeBlockedDelete = window.eval("APP_DATA.conditions.length");
window.showDeleteMasterModal("condition", temporaryConditionIndex);
window.confirmMasterDelete();
assert.equal(window.eval("APP_DATA.conditions.length"), conditionCountBeforeBlockedDelete, "conditions referenced by inventory or market data must not be deleted");

specInventory[0].condition = originalInventoryCondition;
window.eval(`APP_DATA.marketPrices[0].condition = ${JSON.stringify(originalMarketCondition)};`);
window.resetInventorySearch();
window.marketResetFilters();
window.switchMasterTab("condition");
const removableConditionIndex = window.eval("APP_DATA.conditions.findIndex(record => record.code === '0001')");
window.showDeleteMasterModal("condition", removableConditionIndex);
window.confirmMasterDelete();
assert.equal(window.eval("APP_DATA.conditions.length"), initialConditionCount, "unused conditions can be removed from the shared master");
assert.equal([...document.getElementById("market-f-condition").options].some(option => option.value === "0001"), false, "removed conditions must disappear from search choices");
window.localStorage.removeItem("inv_condition_master_v1");

const initialBuyerCount = window.eval("APP_DATA.buyers.length");
const unifiedPartnerInitialSupplierCount = window.eval("APP_DATA.suppliers.length");
const initialGuestCount = window.eval("APP_DATA.guestAccounts.length");
window.switchMasterTab("client");
assert.match(document.getElementById("masterContentArea").textContent, /取引先/u);
assert.ok(document.getElementById("mClientNameFilter"), "the unified partner master must provide a name filter");
assert.ok(document.getElementById("mClientRegionFilter"), "the unified partner master must provide a domestic/overseas filter");
assert.ok(document.getElementById("mClientTradeFilter"), "the unified partner master must provide a buyer/supplier/other filter");
assert.equal(window.eval("MASTER_TABS.some(tab => tab.key === 'buyer')"), false, "the old buyer edit master must not remain");
assert.equal(window.eval("MASTER_TABS.some(tab => tab.key === 'supplier')"), false, "the old supplier edit master must not remain");

window.showClientModal();
document.getElementById("clientModal-roleBuyer").checked = true;
document.getElementById("clientModal-roleSupplier").checked = true;
document.getElementById("clientModal-roleOther").checked = false;
document.getElementById("clientModal-regionDomestic").checked = false;
document.getElementById("clientModal-regionOverseas").checked = true;
window.clientTradeTypeChanged();
document.getElementById("clientModal-companyName").value = "Codex Shared Trade Company";
document.getElementById("clientModal-address").value = "香港テスト地区";
document.getElementById("clientModal-tel").value = "+852-1234-9999";
document.getElementById("clientModal-email").value = "shared-trade@example.com";
document.getElementById("clientModal-invoice").value = "T1234000099999";
document.getElementById("clientModal-contactPhone").value = "+852-9876-5432";
document.getElementById("clientModal-antiqueLicense").value = "香港古物登録 TEST-001";
assert.equal(document.getElementById("clientModal-code").readOnly, false);
const partnerCountBefore=window.eval("APP_DATA.clientCompanies.length");
for(const code of ["10000","00001","-1","1.5","１","CLI-999"]) {
 document.getElementById("clientModal-code").value=code;
 await window.saveClientModal();
 assert.equal(window.eval("APP_DATA.clientCompanies.length"),partnerCountBefore,"invalid partner code must not save: "+code);
}
document.getElementById("clientModal-code").value="42";
const sharedClientCode = "0042";
const sharedBuyerCode = document.getElementById("clientModal-buyerCode").value;
const sharedSupplierCode = document.getElementById("clientModal-supplierCode").value;
await window.saveClientModal();
const sharedClient = window.eval("APP_DATA.clientCompanies.find(company => company.companyName === 'Codex Shared Trade Company')");
assert.equal(sharedClient.id, sharedClientCode);
assert.deepEqual([...sharedClient.tradeTypes], ["buyer", "supplier"], "one canonical partner must support both buyer and supplier roles");
assert.equal(sharedClient.regionType, "overseas");
assert.equal(sharedClient.contactPhone, "+852-9876-5432");
assert.equal(sharedClient.antiqueLicenseNumber, "香港古物登録 TEST-001");
assert.equal(sharedClient.buyerCode, sharedBuyerCode);
assert.equal(sharedClient.supplierCode, sharedSupplierCode);
assert.equal(window.eval(`APP_DATA.buyers.find(record => record.code === ${JSON.stringify(sharedBuyerCode)}).invoice`), "T1234000099999", "partner edits must flow to buyer selectors and transactions");
assert.equal(window.eval(`APP_DATA.suppliers.find(record => record.code === ${JSON.stringify(sharedSupplierCode)}).invoice`), "T1234000099999", "partner edits must flow to supplier selectors and transactions");
for (const selectId of ["sl-buyer", "sh-dest"]) {
  assert.equal([...document.getElementById(selectId).options].some(option => option.value === sharedBuyerCode), true, `${selectId} must use the unified partner master`);
}

window.showClientModal(sharedClientCode);
assert.equal(document.getElementById("clientModal-code").value,"0042");
assert.equal(document.getElementById("clientModal-code").readOnly,true);
assert.equal(document.getElementById("clientModal-regionOverseas").checked,true);
assert.equal(document.getElementById("clientModal-roleBuyer").checked,true);
assert.equal(document.getElementById("clientModal-roleSupplier").checked,true);
window.closeClientModal();
window.showClientModal();
document.getElementById("clientModal-code").value="42";
document.getElementById("clientModal-companyName").value="Duplicate partner test";
document.getElementById("clientModal-address").value="テスト住所";
await window.saveClientModal();
assert.equal(window.eval("APP_DATA.clientCompanies.length"),partnerCountBefore+1,"42 must conflict with 0042");
window.closeClientModal();
window.setClientCompanyNameFilter("Codex Shared");
window.setClientCompanyRegionFilter("overseas");
window.setClientCompanyTradeFilter("buyer", false);
window.setClientCompanyTradeFilter("other", false);
assert.equal(document.querySelectorAll('#mClient-tbody tr[data-client-id]').length, 1, "combined partner filters must be applied together");
window.setClientCompanyNameFilter("");
window.setClientCompanyRegionFilter("all");
window.setClientCompanyTradeFilter("buyer", true);
window.setClientCompanyTradeFilter("other", true);

window.openGuestLoginForPartner(sharedClientCode);
assert.equal(document.getElementById("addGuest-company").value, "Codex Shared Trade Company");
assert.equal(document.getElementById("addGuest-buyer").value, sharedBuyerCode, "guest issuance must reuse the canonical partner's buyer role code");
document.getElementById("addGuest-name").value = "Codex Guest";
document.getElementById("addGuest-email").value = "shared-trade@example.com";
document.getElementById("addGuest-pw").value = "guestpass99";
await window.saveAddGuest();
assert.equal(window.eval(`APP_DATA.guestAccounts.filter(guest => guest.buyerCode === ${JSON.stringify(sharedBuyerCode)}).length`), 1, "guest issuance must create exactly one linked guest account");
window.switchMasterTab("client");
const issuedButton = document.querySelector(`#mClient-tbody tr[data-client-id="${sharedClientCode}"] .partner-guest-issued`);
assert.ok(issuedButton?.disabled, "an already issued guest button must be disabled and shown as issued");
window.openGuestLoginForPartner(sharedClientCode);
assert.equal(window.eval(`APP_DATA.guestAccounts.filter(guest => guest.buyerCode === ${JSON.stringify(sharedBuyerCode)}).length`), 1, "a second guest account must not be issued for the same partner");

window.showClientModal();
document.getElementById("clientModal-roleBuyer").checked = false;
document.getElementById("clientModal-roleSupplier").checked = false;
document.getElementById("clientModal-roleOther").checked = true;
document.getElementById("clientModal-companyName").value = "Codex Other Partner";
document.getElementById("clientModal-address").value = "東京都テスト区";
await window.saveClientModal();
assert.equal(window.eval("APP_DATA.clientCompanies.some(company => company.companyName === 'Codex Other Partner' && company.isOther)"), true, "other-only partners must remain in the canonical directory");
window.setClientCompanyTradeFilter("buyer", false);
window.setClientCompanyTradeFilter("supplier", false);
assert.match(document.getElementById("mClient-tbody").textContent, /Codex Other Partner/u);
window.setClientCompanyTradeFilter("buyer", true);
window.setClientCompanyTradeFilter("supplier", true);

window.eval(`
  APP_DATA.guestAccounts = APP_DATA.guestAccounts.filter(guest => guest.buyerCode !== ${JSON.stringify(sharedBuyerCode)});
  APP_DATA.buyers = APP_DATA.buyers.filter(record => record.code !== ${JSON.stringify(sharedBuyerCode)});
  APP_DATA.suppliers = APP_DATA.suppliers.filter(record => record.code !== ${JSON.stringify(sharedSupplierCode)});
  APP_DATA.clientCompanies = APP_DATA.clientCompanies.filter(company => !['Codex Shared Trade Company', 'Codex Other Partner'].includes(company.companyName));
`);
assert.equal(window.eval("APP_DATA.buyers.length"), initialBuyerCount);
assert.equal(window.eval("APP_DATA.suppliers.length"), unifiedPartnerInitialSupplierCount);
assert.equal(window.eval("APP_DATA.guestAccounts.length"), initialGuestCount);
window.localStorage.removeItem("inv_login_directory_v1");

window.switchMasterTab("box");
assert.equal(document.querySelectorAll("#masterBoxMatrixNumbers .box-num-cell").length, 10, "master guest management must show all BOX columns");
assert.equal(document.querySelectorAll("#masterBoxMatrixBody .box-matrix-row").length, window.eval("APP_DATA.guestAccounts.length"), "guest management must show exactly the accounts in password management");
assert.equal(document.querySelectorAll("#boxMatrixBody .box-matrix-row").length, window.eval("APP_DATA.guestAccounts.length"), "sidebar guest management must use the same synchronized account directory");
const masterBoxCheckbox = document.querySelector('#masterBoxMatrixBody input[data-box-no="1"][data-buyer="B001"]');
assert.ok(masterBoxCheckbox, "master guest management must provide the shared BOX publication controls");
const masterBoxOriginalChecked = masterBoxCheckbox.checked;
masterBoxCheckbox.checked = !masterBoxOriginalChecked;
window.onBoxPublicChange(masterBoxCheckbox);
assert.equal(window.eval("APP_DATA.boxes[0].publicTo.includes('B001')"), !masterBoxOriginalChecked, "master BOX changes must update the shared BOX data");
assert.equal(document.querySelector('#boxMatrixBody input[data-box-no="1"][data-buyer="B001"]').checked, !masterBoxOriginalChecked, "sidebar guest management must immediately reflect master BOX changes");
const masterBoxRestoreCheckbox = document.querySelector('#masterBoxMatrixBody input[data-box-no="1"][data-buyer="B001"]');
masterBoxRestoreCheckbox.checked = masterBoxOriginalChecked;
window.onBoxPublicChange(masterBoxRestoreCheckbox);

window.switchMasterTab("password");
assert.match(document.getElementById("masterContentArea").textContent, /パスワード管理/u);
assert.match(document.getElementById("masterContentArea").textContent, /ログイン認証と連動/u);
assert.equal(document.querySelectorAll("#mpw-admin-tbody tr").length, 1);
assert.equal(document.querySelector("#mpw-admin-tbody").textContent.includes("承認コード"), false, "the shared access code must not be stored per administrator account");
window.switchMpwTab("buyer");
assert.equal(document.querySelectorAll("#mpw-buyer-tbody tr").length, window.eval("APP_DATA.users.filter(u => u.role === 'buyer' || u.role === 'worker').length"));
assert.equal(document.querySelectorAll("#mpw-buyer-tbody tr[data-staff-code]").length, 0, "worker rows must not expose buyer codes");
assert.match(document.getElementById("mpw-buyer-tbody").textContent, /当社/u, "purchase staff must be shown as own-company workers");
window.switchMpwTab("guest");
assert.equal(document.querySelectorAll("#mpw-guest-tbody tr").length, window.eval("APP_DATA.guestAccounts.length"), "password management must show exactly the guest-management accounts");
assert.match(document.getElementById("mpw-guest-tbody").textContent, /B00/u, "guest rows must show their linked customer code");
assert.ok(document.querySelector('#mpw-guest-tbody tr[data-buyer-code="B001"] button[onclick*="openLoginInfoModal"]'), "migrated guest-management customers must have editable login information");
assert.equal(window.eval("APP_DATA.guestAccounts.find(guest => guest.buyerCode === 'B001').id"), "G003");
window.openGuestLoginForBuyer("B001");
assert.equal(document.getElementById("loginInfoCompany").value, window.eval("APP_DATA.buyers.find(buyer => buyer.code === 'B001').name"));
assert.equal(document.getElementById("loginInfoBuyerCode").value, "B001");
assert.equal(document.getElementById("loginInfoAddress").value, window.eval("APP_DATA.buyers.find(buyer => buyer.code === 'B001').address"));
window.closeLoginInfoModal();
window.openLoginInfoModal("guest");
document.getElementById("loginInfoName").value = "Duplicate customer guest";
document.getElementById("loginInfoEmail").value = "duplicate@example.com";
document.getElementById("loginInfoPassword").value = "duplicate123";
document.getElementById("loginInfoCompany").value = "Duplicate customer";
document.getElementById("loginInfoBuyerCode").value = "B002";
assert.equal(await window.saveLoginInfo(), false, "one customer must not receive multiple guest login accounts");
window.closeLoginInfoModal();
window.switchMpwTab("authcode");
await new Promise((resolve) => setTimeout(resolve, 0));
const firstAdminAccessCode = document.getElementById("mpw-authcode-value").textContent.trim();
assert.match(firstAdminAccessCode, /^[A-Z0-9]{6}$/, "the administrator access tab must show a six-character alphanumeric code");
await window.rotateAdminAccessCodeNow();
const rotatedAdminAccessCode = document.getElementById("mpw-authcode-value").textContent.trim();
assert.match(rotatedAdminAccessCode, /^[A-Z0-9]{6}$/);
assert.notEqual(rotatedAdminAccessCode, firstAdminAccessCode, "manual rotation must invalidate the previous administrator access code");
window.switchMpwTab("admin");

window.openLoginInfoModal("admin", "U001");
document.getElementById("loginInfoActive").checked = false;
assert.equal(await window.saveLoginInfo(), false, "the signed-in administrator must not be able to disable itself");
assert.equal(window.eval("APP_DATA.users.find(user => user.id === 'U001').active"), undefined);
window.closeLoginInfoModal();

const usersBeforeLoginInfoTest = window.eval("APP_DATA.users.length");
const guestsBeforeLoginInfoTest = window.eval("APP_DATA.guestAccounts.length");
const buyersBeforeLoginInfoTest = window.eval("APP_DATA.buyers.length");
const staffBeforeLoginInfoTest = window.eval("APP_DATA.staffRecords.length");
window.openLoginInfoModal("buyer");
assert.equal(document.getElementById("loginInfoStaffCode"), null, "worker form must not contain buyer code");
document.getElementById("loginInfoName").value = "ログイン管理テスト作業者";
document.getElementById("loginInfoLoginId").value = "login-test-worker";
document.getElementById("loginInfoEmail").value = "login-test-worker@example.com";
document.getElementById("loginInfoPassword").value = "testpass123";
assert.equal(await window.saveLoginInfo(), true, "administrator must be able to create a worker account");
assert.equal(window.eval("APP_DATA.users.find(user => user.loginId === 'login-test-worker').staffCode"), undefined);
assert.equal(window.eval("APP_DATA.staffRecords.some(record => record.code === '0006' && record.name === 'ログイン管理テスト作業者')"), false);
assert.ok(window.doAppLogin("login-test-worker", "testpass123"), "created worker credentials must work immediately");
assert.equal(window.currentRole(), "buyer");
assert.match(window.localStorage.getItem("inv_login_directory_v1"), /login-test-worker/u);

assert.ok(window.doAppLogin("admin", "admin123"));
window.switchMasterTab("password");
window.switchMpwTab("buyer");
assert.match(document.getElementById("mpw-buyer-tbody").textContent, /login-test-worker/u, "new worker accounts must appear in master password management");
const linkedWorker = window.eval("APP_DATA.users").find(user => user.loginId === "login-test-worker");
window.showMpwChangeModal(linkedWorker.id, false);
document.getElementById("mpwChange-new").value = "linkedpass123";
window.saveMpwChange();
assert.ok(window.doAppLogin("login-test-worker", "linkedpass123"), "password changes in master management must affect login immediately");
assert.ok(window.doAppLogin("admin", "admin123"));
window.openLoginInfoModal("guest");
document.getElementById("loginInfoName").value = "ログイン管理テスト顧客";
document.getElementById("loginInfoEmail").value = "login-test-guest@example.com";
document.getElementById("loginInfoPassword").value = "guestpass123";
document.getElementById("loginInfoCompany").value = "ログイン管理テスト株式会社";
document.getElementById("loginInfoAddress").value = "東京都テスト区1-2-3";
document.getElementById("loginInfoContact").value = "03-0000-0000";
document.getElementById("loginInfoInvoice").value = "T0000000000001";
const createdGuestId = document.getElementById("loginInfoLoginId").value;
const createdBuyerCode = document.getElementById("loginInfoBuyerCode").value;
assert.equal(await window.saveLoginInfo(), true, "administrator must be able to create a guest with customer data");
const createdGuest = window.eval("APP_DATA.guestAccounts").find(guest => guest.id === createdGuestId);
const createdCustomer = window.eval("APP_DATA.buyers").find(buyer => buyer.code === createdBuyerCode);
assert.equal(createdGuest.company, "ログイン管理テスト株式会社");
assert.equal(createdCustomer.address, "東京都テスト区1-2-3");
assert.equal(createdCustomer.invoice, "T0000000000001");
window.switchMasterTab("password");
window.switchMpwTab("guest");
assert.match(document.getElementById("mpw-guest-tbody").textContent, /ログイン管理テスト株式会社/u, "new guest and customer data must appear in master password management");
assert.match(document.getElementById("mpw-guest-tbody").textContent, /東京都テスト区1-2-3/u, "linked customer details must be visible in master password management");

window.eval(`APP_DATA.users.splice(${usersBeforeLoginInfoTest})`);
window.eval(`APP_DATA.guestAccounts.splice(${guestsBeforeLoginInfoTest})`);
window.eval(`APP_DATA.buyers.splice(${buyersBeforeLoginInfoTest})`);
window.eval(`APP_DATA.staffRecords.splice(${staffBeforeLoginInfoTest}); _syncLegacyStaffNames()`);
window.localStorage.removeItem("inv_login_directory_v1");
window.localStorage.removeItem("inv_staff_master_v1");
assert.ok(window.doAppLogin("admin", "admin123"));
window.applyRoleUI();

const inventory = window.eval("APP_DATA.inventory");
assert.ok(Array.isArray(inventory) && inventory.length > 0, "preview inventory must be available");
assert.equal(inventory[0].purchasePrice, 850000, "purchase price must stay in JPY");
assert.equal(inventory[0].salePrice, 7613, "legacy sale price must be converted to USD at 155 JPY/USD");
assert.equal(window.formatPrice(inventory[0].purchasePrice), "¥850,000");
assert.equal(window.formatSalePrice(inventory[0].salePrice), "$7,613");
const seededReservedItem = inventory.find(item => item.code === "0303260001");
assert.equal(seededReservedItem.status, "取置中", "an already-approved purchase request must restore its reservation on load");
assert.equal(seededReservedItem.reservationRequestId, "PR-003");
assert.equal(document.body.textContent.includes("売価（円）"), false, "sale price labels must not claim JPY");
window.init_dashboard();
assert.equal(document.querySelectorAll('#page-dashboard .business-kpi').length,8);
assert.equal(document.querySelectorAll('#page-dashboard .business-composition').length,2);
assert.ok(document.getElementById('business-from'));
assert.ok(document.getElementById('business-to'));
window.businessDashboardReset();
assert.equal(document.getElementById("salesTotalDisplay").textContent.trim(), "¥0");
assert.equal(document.getElementById("shippingTotalDisplay").textContent.trim(), "$0");

window.navigateTo("shipping");
window.resetShippingForm();
const shippingCodeInput = document.querySelector('#shippingLines [id^="sh-code-"]');
const shippingLineId = shippingCodeInput.id.replace("sh-code-", "");
const shippingPriceInput = document.getElementById(`sh-price-${shippingLineId}`);
const shippingInventoryItem = inventory.find(item => item.status === "在庫中" && Number(item.salePrice) > 0);
assert.ok(shippingInventoryItem, "shipping autofill test requires an in-stock item with a sale price");
shippingCodeInput.value = shippingInventoryItem.code;
window.autoFillItem(shippingCodeInput, Number(shippingLineId), "shipping");
const expectedSalePriceUsd = shippingInventoryItem.salePrice;
assert.equal(window.getShippingLinePriceUSD(shippingPriceInput), expectedSalePriceUsd,
  "shipping amount must be filled from the inventory USD sale price");
assert.equal(document.getElementById("shippingTotalDisplay").textContent.trim(), window.formatSalePrice(expectedSalePriceUsd),
  "shipping total must update after sale-price autofill");
const shippingRate = window.getShippingFormRate();
window.switchShippingEntryCurrency("JPY");
const expectedSalePriceJpy = window.roundShippingJPYToThousand(expectedSalePriceUsd * shippingRate);
assert.equal(window.getPriceValue(shippingPriceInput), expectedSalePriceJpy,
  "each shipping line converted to JPY must be rounded up to the nearest 1,000 yen");
assert.equal(window.roundShippingJPYToThousand(1552500), 1553000,
  "a converted amount with a 1-999 yen remainder must round up to the next 1,000 yen");
assert.equal(document.getElementById("shippingTotalDisplay").textContent.trim(), window.formatPrice(expectedSalePriceJpy));
window.switchShippingEntryCurrency("USD");
shippingCodeInput.value = "";
window.autoFillItem(shippingCodeInput, Number(shippingLineId), "shipping");
assert.equal(window.getPriceValue(shippingPriceInput), 0,
  "clearing the inventory code must also clear the linked sale price");
assert.equal(document.getElementById("shippingTotalDisplay").textContent.trim(), "$0");

assert.equal(typeof window.persistGuestSnapshot, "function");
assert.equal(typeof window.unpublishGuestProducts, "function");
assert.equal(typeof window.createGuestPurchaseRequest, "function");
assert.equal(typeof window.reservePurchaseRequestItem, "function");
assert.equal(typeof window.releasePurchaseRequestReservations, "function");
window.publishGuestSnapshot();
let storedPublishedSnapshot = JSON.parse(window.localStorage.getItem("inv_guest_snapshot_v1"));
assert.ok(storedPublishedSnapshot.boxes.length > 0, "publishing boxes must persist the guest snapshot");
const publishedProductCode = storedPublishedSnapshot.boxes.flatMap(box => box.items || [])[0]?.code;
assert.ok(publishedProductCode, "the published snapshot must include an in-stock product");
const publishedInventoryItem = inventory.find(item => item.code === publishedProductCode);
const previousPublishedStatus = publishedInventoryItem.status;
publishedInventoryItem.status = '出荷済';
assert.equal(window.unpublishGuestProducts([publishedProductCode]), 1, 'shipping a product added in stock must hide it');
assert.ok(!JSON.parse(window.localStorage.getItem('inv_guest_snapshot_v1')).boxes.some(box => box.items.some(item => item.code === publishedProductCode)));
publishedInventoryItem.status = '売上済';
assert.equal(window.unpublishGuestProducts([publishedProductCode]), 0, 'selling an already hidden product must keep it hidden');
storedPublishedSnapshot = JSON.parse(window.localStorage.getItem("inv_guest_snapshot_v1"));
assert.equal(
  storedPublishedSnapshot.boxes.some(box => (box.items || []).some(item => item.code === publishedProductCode)),
  false,
  "the automatically unpublished product must also be removed from persisted guest data",
);
assert.equal(window.unpublishGuestProducts([publishedProductCode]), 0, "automatic unpublishing must be idempotent");
publishedInventoryItem.status = previousPublishedStatus;
window.publishGuestSnapshot();
const adminGuest = window.eval("APP_DATA.guestAccounts[0]");
const requestCountBeforeGuestSync = window.eval("APP_DATA.purchaseRequests.length");
const incomingGuestRequest = window.createGuestPurchaseRequest({
  guest: adminGuest,
  items: [inventory[0]],
  note: "管理者通知テスト",
});
window.navigateTo("dashboard");
window.refreshGuestRequestAdminUI(false);
assert.equal(incomingGuestRequest.status, "未対応");
assert.equal(incomingGuestRequest.buyerCode, adminGuest.buyerCode, "new guest requests must store the buyer code at submission time");
assert.equal(incomingGuestRequest.clientCompanyCode, "CLI-001", "new guest requests must store the common client-company code");
assert.ok(window.eval("APP_DATA.purchaseRequests").some(request => request.note === "管理者通知テスト"));
assert.equal(Number(document.getElementById("requestBadge").textContent), 3);
assert.equal(document.querySelectorAll("#page-dashboard .business-kpi").length, 8);

const incomingRequestId = incomingGuestRequest.id;
window.setPrItemStatus(incomingRequestId, 0, "approved");
assert.equal(inventory[0].status, "取置中", "approving a guest request must reserve its inventory item immediately");
assert.equal(inventory[0].reservationRequestId, incomingRequestId);
assert.equal(inventory[0].reservedForBuyerCode, incomingGuestRequest.buyerCode);
assert.equal(inventory[0].reservedForClientCompanyCode, incomingGuestRequest.clientCompanyCode);
assert.equal(window.canUseInventoryItemForShipping(inventory[0]), false, "a reserved item must be blocked from ordinary shipping");
assert.equal(window.canUseInventoryItemForShipping(inventory[0], incomingRequestId), true, "the owning request may ship its reserved item");
assert.equal(window.canUseInventoryItemForSales(inventory[0]), false, "a reserved item must be blocked from direct sales");
storedPublishedSnapshot = JSON.parse(window.localStorage.getItem("inv_guest_snapshot_v1"));
assert.equal(
  storedPublishedSnapshot.boxes.some(box => (box.items || []).some(item => item.code === inventory[0].code)),
  false,
  "reservation must immediately remove the item from every guest snapshot",
);

// Inventory itself is not stored in this prototype, so hydration must reconstruct reservations from persisted requests.
inventory[0].status = "在庫中";
window.clearInventoryReservationMetadata(inventory[0]);
window.hydrateGuestDomainState();
assert.equal(inventory[0].status, "取置中", "persisted purchase requests must restore reservations after reload");
assert.equal(inventory[0].reservationRequestId, incomingRequestId);

const competingRequest = window.createGuestPurchaseRequest({
  guest: adminGuest,
  items: [inventory[0]],
  note: "二重確保防止テスト",
});
window.setPrItemStatus(competingRequest.id, 0, "approved");
const storedCompetingRequest = window.eval("APP_DATA.purchaseRequests").find(request => request.id === competingRequest.id);
assert.equal(storedCompetingRequest.items[0].itemStatus, "pending", "another request must not reserve the same item");
assert.equal(inventory[0].reservationRequestId, incomingRequestId);

window.setPrRequestStatus(incomingRequestId, "却下");
assert.equal(inventory[0].status, "在庫中", "rejecting the owning request must release the reservation");
assert.equal(inventory[0].reservationRequestId, undefined);
window.setPrItemStatus(competingRequest.id, 0, "approved");
assert.equal(inventory[0].status, "取置中", "the item may be reserved by another request after release");
assert.equal(inventory[0].reservationRequestId, competingRequest.id);
window.setPrRequestStatus(competingRequest.id, "取消済");
assert.equal(inventory[0].status, "在庫中", "cancelling a request must release its reservation");

window.eval(`APP_DATA.purchaseRequests = APP_DATA.purchaseRequests.filter(request => !${JSON.stringify([incomingRequestId, competingRequest.id])}.includes(request.id))`);
assert.equal(window.eval("APP_DATA.purchaseRequests.length"), requestCountBeforeGuestSync);
window.localStorage.removeItem("inv_guest_snapshot_v1");
window.localStorage.removeItem("inv_guest_box_draft_v1");
window.localStorage.removeItem("inv_purchase_requests_v1");

const appSource = scriptSources.get("app.js");
const dataSource = scriptSources.get("data.js");
const approvalSource = scriptSources.get("approval.js");
const qrInventorySource = scriptSources.get("qr_inventory.js");
assert.equal(dataSource.includes("OWN_COMPANY"), false, "company information must have one APP_DATA source");
assert.equal(appSource.includes("OWN_COMPANY"), false, "every document must read the editable company master");
assert.match(appSource, /function shouldUseBrowserMasterStorage\(\)/u, "API mode must have a single guard against stale browser master data");
assert.ok((appSource.match(/if \(!shouldUseBrowserMasterStorage\(\)\)/gu) || []).length >= 10,
  "DB-hydrated masters must not be overwritten or persisted by stale localStorage directories");
assert.match(appSource, /\(APP_DATA\.salesReturns \|\| \[\]\)\.forEach/u,
  "DB-backed return documents must remain available to related document workflows");
assert.match(appSource, /sourceType === 'return-document'/u,
  "DB-backed returns must open the persisted return detail instead of the legacy takeback modal");
assert.match(apiBridgeSource, /const sourceSale = findSourceSaleForReturn\(record, saleDetails\)/u,
  "return documents must resolve the original sale before restoring monetary detail");
assert.match(appSource, /unpublishGuestProducts\(soldProductCodes\)/u, "confirmed sales must automatically unpublish sold products");
assert.match(appSource, /unpublishGuestProducts\(shippedProductCodes\)/u, "confirmed shipments must automatically unpublish shipped products");
assert.match(approvalSource, /unpublishGuestProducts\(soldProductCodes\)/u, "approved worker sales must automatically unpublish sold products");
assert.match(appSource, /getShippingSaleTotalUSD/u, "shipping totals must use the saved sale-price snapshot");
assert.match(appSource, /roundShippingJPYToThousand/u, "shipping JPY conversion must round each product up to 1,000 yen");
assert.match(appSource, /formatSalePrice\(total\)/u, "sales invoice total must use USD");
assert.equal(appSource.includes("formatPrice(rec.total)"), false, "sales and shipping detail totals must not use JPY formatting");
assert.equal(appSource.includes("r.guestName.includes"), false, "shipping must never locate a buyer by guest/company name");
assert.match(appSource, /destSel\.value = r\.buyerCode/u, "shipping must use the buyer code stored on the purchase request");
assert.match(appSource, /clientCompanyCode,/u, "shipping slips must retain the common client-company code");

window.navigateTo("inventory");
window.execInventorySearch();
// Menu navigation retains search results, page, sorting, BOX and accessories.
{
  window.resetInventorySearch();
  document.getElementById('inv-f-status').value = '';
  window.execInventorySearch();
  window.toggleInventoryStatusSort();
  window.eval('inventoryPage = 2');
  window.renderInventoryTable();
  const rows = document.getElementById('inventoryTableBody').innerHTML;
  window.navigateTo('dashboard');
  window.navigateTo('inventory');
  assert.equal(window.eval('_invSearched'),true);
  assert.equal(window.eval('inventoryPage'),2);
  assert.equal(window.eval('_invStatusSort'),'ascending');
  assert.equal(document.getElementById('inventoryTableBody').innerHTML,rows);
  assert.notEqual(document.getElementById('inv-result-area').style.display,'none');
  const boxFilter = document.getElementById('inv-f-box');
  boxFilter.selectedIndex = 1;
  const selectedBox = boxFilter.value;
  window.eval('_invAccFilterState = APP_DATA.accessories.slice(0,1)');
  const accessories = JSON.stringify(window.eval('_invAccFilterState'));
  document.getElementById('inv-f-model').value = 'retained model query';
  window.execInventorySearch();
  window.navigateTo('dashboard');window.navigateTo('inventory');
  assert.equal(boxFilter.value,selectedBox);
  assert.equal(JSON.stringify(window.eval('_invAccFilterState')),accessories);
  assert.equal(document.getElementById('inv-f-model').value,'retained model query');
  assert.notEqual(document.getElementById('inv-result-area').style.display,'none','zero-result searches remain visible');
  window.resetInventorySearch();
  window.navigateTo('dashboard');window.navigateTo('inventory');
  assert.equal(window.eval('_invSearched'),false);
  assert.equal(document.getElementById('inv-result-area').style.display,'none');
  assert.equal(boxFilter.value,'');
  window.eval("_invStatusSort = 'none'");window._invSyncStatusSortUI();
  window.execInventorySearch();
}

assert.match(document.querySelector('[data-page="inventory"]').textContent, /在庫管理/u,
  "inventory management must be a direct sidebar item");
assert.equal(document.getElementById("inventoryNavGroup"), null,
  "inventory management must not have a nested sidebar hierarchy");
assert.equal(document.getElementById("pageTitle").textContent, "在庫管理",
  "the existing inventory page must be presented as inventory management");
assert.equal(document.querySelector("#page-inventory #partInventoryTable"), null,
  "the product management page must not contain the parts list");
window.navigateTo("parts-management");
assert.equal(document.getElementById("pageTitle").textContent, "パーツ管理");
assert.equal(document.getElementById("page-parts-management").classList.contains("hidden"), false);
document.getElementById("part-f-code").value = "P2908260001";
window.execPartInventorySearch();
assert.equal(document.querySelectorAll("#partInventoryTableBody .part-inventory-row").length, 1,
  "part management search must filter by part management number");
window.resetPartInventorySearch();
assert.deepEqual(
  [...document.querySelectorAll("#partInventoryTable thead th")].map(cell => cell.textContent.trim()),
  ["パーツ管理番号", "仕入日", "パーツ名", "詳細", "ブランド", "型番／モデル", "SKU", "仕入先", "バイヤー", "原価（JPY）", "想定売価", "ステータス"],
  "the parts list must expose the selected management fields in a stable order",
);
assert.equal(document.querySelectorAll("#partInventoryTableBody tr").length,
  Math.max(1, Math.min(10, window.eval("APP_DATA.parts.length"))),
  "the inventory page must render the registered parts below the product inventory list");
assert.match(document.getElementById("partInventoryTableBody").textContent, /P2908260001/u,
  "the part management number must be visible in the parts list");
assert.match(document.getElementById("partInventoryTableBody").textContent, /\$4,100/u,
  "the parts list must show sale price with USD as its base currency");
const editablePartRow = document.querySelector("#partInventoryTableBody .part-inventory-row");
editablePartRow.click();
assert.equal(document.getElementById("partDetailEditModal").classList.contains("hidden"), false,
  "clicking a parts-list row must open the part detail editor");
assert.equal(document.getElementById("part-edit-code").value, "P2908260001");
assert.equal(document.getElementById("part-edit-code").readOnly, true,
  "the management number must remain immutable in the part editor");
assert.equal(document.getElementById("part-edit-date").readOnly, true,
  "the numbered purchase date must remain immutable in the part editor");
assert.equal(document.getElementById("part-edit-quantity-group").classList.contains("hidden"), false,
  "BRACELET PARTS editing must retain its bracelet quantity field");
document.getElementById("part-edit-model").value = "編集モデル";
document.getElementById("part-edit-sale").value = "4,200";
assert.equal(await window.savePartDetailEditor(), true);
assert.equal(document.getElementById("partDetailEditModal").classList.contains("hidden"), true);
assert.equal(window.eval("APP_DATA.parts.find(part => part.partCode === 'P2908260001').modelName"), "編集モデル",
  "saving the modal must update the part record");
assert.match(document.getElementById("partInventoryTableBody").textContent, /\$4,200/u,
  "the parts list must redraw the saved USD sale price");
assert.equal(document.querySelector('#inventoryTableBody tr:first-child td[data-inv-col="purchaseType"]').textContent.trim(), "国内業者仕入／オークション", "legacy inventory must be assigned the domestic purchase category");
assert.equal(document.querySelector('#inventoryTableBody tr:first-child td[data-inv-col="purchaseRate"]').textContent.trim(), "￥1", "inventory must show the registered purchase rate");
assert.equal(document.querySelector('#inventoryTableBody tr:first-child td[data-inv-col="purchasePriceAtPurchaseRate"]').textContent.trim(), "¥850,000", "inventory must show cost converted at the registered rate");
assert.equal(document.querySelector('#inventoryTableBody tr:first-child td[data-inv-col="purchasePriceAtPurchaseRate"]').classList.contains("inv-purchase-currency-match"), true,
  "purchase-time cost must be blue-highlighted when the linked display currency matches the actual purchase currency");
assert.equal(document.querySelector('#inventoryTableBody tr:first-child td[data-inv-col="purchasePrice"]').textContent.trim(), "¥850,000", "inventory must show cost converted at the current rate");
assert.equal(document.querySelector('#inventoryTableBody tr:first-child td[data-inv-col="salePrice"]').textContent.trim(), "¥1,180,015", "inventory sale price must initially share the linked JPY display");
assert.equal(document.querySelector('#inventoryTableBody tr:first-child td[data-inv-col="grossMargin"]').textContent.trim(), "28.0%", "inventory must show gross margin after sale price");
const inventoryCodeCopyButton = document.querySelector('#inventoryTableBody tr:first-child .inv-management-number-copy');
assert.ok(inventoryCodeCopyButton, "each management number must have a one-tap copy button");
assert.equal(inventoryCodeCopyButton.getAttribute("aria-label"), `管理番号 ${inventory[0].code} をコピー`);
assert.equal(typeof window.copyInventoryManagementNumber, "function");
assert.equal(document.getElementById("itemDetailModal").classList.contains("hidden"), true);
inventoryCodeCopyButton.click();
await new Promise(resolve => setTimeout(resolve, 0));
assert.equal(window.__lastClipboardText, inventory[0].code, "copy button must write the management number to the clipboard");
assert.equal(inventoryCodeCopyButton.classList.contains("copied"), true, "copy success must be visible on the button");
assert.match(inventoryCodeCopyButton.getAttribute("aria-label"), /コピーしました/u);
assert.equal(document.getElementById("itemDetailModal").classList.contains("hidden"), true,
  "copying a management number must not open the row detail modal");
assert.match(document.querySelector("#page-inventory thead").textContent, /売価/u);
assert.match(document.querySelector("#page-inventory thead").textContent, /粗利率/u);
const inventoryColumnOrder = [...document.querySelectorAll('#inventoryTable thead th')].map(th => th.dataset.invCol);
assert.equal(inventoryColumnOrder.indexOf('purchaseType') + 1, inventoryColumnOrder.indexOf('purchaseRate'), "purchase category must appear immediately before purchase rate");
assert.equal(document.querySelectorAll('#page-inventory th[data-inv-col="purchasePriceAtPurchaseRate"] .inv-price-currency-btn').length, 3,
  "purchase-time cost must support JPY, USD, and HKD display");
assert.equal(document.querySelectorAll('#page-inventory th[data-inv-col="purchasePrice"] .inv-price-currency-btn').length, 3,
  "current-rate cost must support JPY, USD, and HKD display");
assert.equal(document.querySelectorAll('#page-inventory th[data-inv-col="salePrice"] .inv-price-currency-btn').length, 3,
  "sale price must support the same JPY, USD, and HKD display currencies");
assert.equal(typeof window.switchInventoryCostCurrency, "function");
window.switchInventoryCostCurrency("purchase-time", "USD");
assert.match(document.querySelector('#inventoryTableBody tr:first-child td[data-inv-col="purchasePriceAtPurchaseRate"]').textContent.trim(), /^\$/u,
  "purchase-time cost must convert to USD");
assert.match(document.querySelector('#inventoryTableBody tr:first-child td[data-inv-col="purchasePrice"]').textContent.trim(), /^\$/u,
  "changing purchase-time cost currency must also switch the current-rate column");
assert.equal(document.querySelector('#inventoryTableBody tr:first-child td[data-inv-col="salePrice"]').textContent.trim(), "$7,613",
  "changing purchase-time cost currency must also switch the sale-price column");
assert.match(document.querySelector('#inventoryTableBody tr:first-child td[data-inv-col="purchaseRate"]').textContent.trim(), /^￥/u,
  "purchase rate must follow the linked currency using the purchase-time rate");
assert.equal(document.querySelector('#inventoryTableBody tr:first-child td[data-inv-col="purchasePriceAtPurchaseRate"]').classList.contains("inv-purchase-currency-match"), false,
  "the purchase-time cost highlight must clear when the display currency differs from the actual purchase currency");
assert.equal(document.getElementById("inv-purchase-time-usd").getAttribute("aria-pressed"), "true");
assert.equal(document.getElementById("inv-purchase-current-usd").getAttribute("aria-pressed"), "true");
assert.equal(document.getElementById("inv-sale-usd").getAttribute("aria-pressed"), "true");
window.switchInventoryCostCurrency("current", "HKD");
assert.match(document.querySelector('#inventoryTableBody tr:first-child td[data-inv-col="purchasePriceAtPurchaseRate"]').textContent.trim(), /^HK\$/u,
  "changing current-rate cost currency must also switch the purchase-time column");
assert.match(document.querySelector('#inventoryTableBody tr:first-child td[data-inv-col="purchasePrice"]').textContent.trim(), /^HK\$/u,
  "current-rate cost must convert to HKD");
assert.match(document.querySelector('#inventoryTableBody tr:first-child td[data-inv-col="salePrice"]').textContent.trim(), /^HK\$/u,
  "current-rate switch must also convert sale price to HKD");
assert.match(document.querySelector('#inventoryTableBody tr:first-child td[data-inv-col="purchaseRate"]').textContent.trim(), /^￥/u,
  "purchase rate must use the item's purchase-time HKD rate when HKD is selected");
assert.equal(document.getElementById("inv-purchase-time-hkd").getAttribute("aria-pressed"), "true");
assert.equal(document.getElementById("inv-purchase-current-hkd").getAttribute("aria-pressed"), "true");
assert.equal(document.getElementById("inv-sale-hkd").getAttribute("aria-pressed"), "true");
assert.equal(typeof window.switchInventoryPriceCurrency, "function");
assert.equal(typeof window._invColumnVisibilityChanged, "function");
window._invToggleColumnMenu({ stopPropagation() {} });
assert.equal(document.getElementById("inv-column-panel").classList.contains("open"), true);
assert.equal(document.getElementById("inv-column-trigger").getAttribute("aria-expanded"), "true");
window._invCloseColumnMenu();
assert.equal(document.getElementById("inv-column-trigger").getAttribute("aria-expanded"), "false");

window.switchInventoryPriceCurrency("sale", "JPY");
assert.equal(document.querySelector('#inventoryTableBody tr:first-child td[data-inv-col="salePrice"]').textContent.trim(), "¥1,180,015", "sale price must convert from USD to JPY using the master rate");
assert.equal(document.getElementById("inv-sale-jpy").getAttribute("aria-pressed"), "true");
assert.equal(document.getElementById("inv-purchase-time-jpy").getAttribute("aria-pressed"), "true");
assert.equal(document.getElementById("inv-purchase-current-jpy").getAttribute("aria-pressed"), "true");
assert.equal(document.querySelector('#inventoryTableBody tr:first-child td[data-inv-col="purchasePriceAtPurchaseRate"]').classList.contains("inv-purchase-currency-match"), true);
assert.match(document.getElementById("inv-sale-heading").textContent, /JPY/u);

const actualSaleTestData = window.eval("APP_DATA");
const actualSaleHistoryBackup = actualSaleTestData.fxRateHistory;
const actualSaleItemBackup = { ...inventory[0] };
try {
  actualSaleTestData.fxRateHistory = [
    { code: "HKD", rate: 18, observedAt: "2026-08-01T09:00:00+09:00" },
    { code: "HKD", rate: 20, observedAt: "2026-08-10T09:00:00+09:00" },
    { code: "HKD", rate: 25, observedAt: "2026-08-20T09:00:00+09:00" },
    { code: "USD", rate: 145, observedAt: "2026-08-10T09:00:00+09:00" },
    { code: "USD", rate: 165, observedAt: "2026-08-20T09:00:00+09:00" },
  ];
  const usdSale = {
    saleDate: "2026-08-15", actualSaleCurrency: "USD", actualSalePrice: 1001,
    actualSalePriceJpy: 150151, saleFxRateScaled: 15000000000, saleFxScale: 100000000,
  };
  const jpySale = {
    saleDate: "2026-08-15", actualSaleCurrency: "JPY", actualSalePrice: 300000,
    actualSalePriceJpy: 300000, saleFxRateScaled: 15000000000, saleFxScale: 100000000,
  };
  const hkdSale = {
    saleDate: "2026-08-15", actualSaleCurrency: "HKD", actualSalePrice: 10000,
    actualSalePriceJpy: 195000, saleFxRateScaled: 1950000000, saleFxScale: 100000000,
  };
  assert.equal(window.formatInventoryActualSalePrice(usdSale), "$1,001",
    "actual sale detail must retain its transaction currency by default");
  assert.equal(window.formatInventoryActualSalePrice(usdSale, "JPY"), "¥150,151",
    "converted actual sales must preserve the backend's adjusted JPY value, including rounding");
  assert.equal(window.formatInventoryActualSalePrice(usdSale, "HKD"), "HK$7,508",
    "actual sales must use the latest HKD history before the sale, not a later or current rate");
  assert.equal(window.formatInventoryActualSalePrice({ ...usdSale, saleFxRateObservedAt: "2026-08-21T09:00:00+09:00" }, "HKD"), "HK$6,006",
    "cross-currency sales must prefer the frozen snapshot timestamp when it differs from the sale date");
  assert.equal(window.formatInventoryActualSaleRate(usdSale, "USD"), "1 USD = ¥150.00",
    "the transaction's frozen sale rate must take priority over USD history");
  assert.equal(window.formatInventoryActualSalePrice(jpySale, "USD"), "$2,000",
    "JPY sales must use the stored USD snapshot when displayed in USD");
  assert.equal(window.formatInventoryActualSalePrice(jpySale, "JPY"), "¥300,000",
    "JPY transactions must retain their exact native amount");
  assert.equal(window.formatInventoryActualSaleRate(jpySale, "JPY"), "1 JPY = ¥1.00");
  assert.equal(window.formatInventoryActualSalePrice(hkdSale, "HKD"), "HK$10,000",
    "HKD transactions must retain their exact native amount");
  assert.equal(window.formatInventoryActualSalePrice(hkdSale, "JPY"), "¥195,000");
  assert.equal(window.formatInventoryActualSalePrice(hkdSale, "USD"), "$1,345",
    "HKD sales must convert through saved JPY value using sale-date USD history");
  assert.equal(window.formatInventoryActualSaleRate(hkdSale), "1 HKD = ¥19.50",
    "an HKD sale snapshot must be labeled as HKD instead of USD");
  assert.equal(window.isInventoryOriginalSaleCurrency(usdSale, "USD"), true);
  assert.equal(window.isInventoryOriginalSaleCurrency(usdSale, "JPY"), false);
  assert.equal(window.isInventoryOriginalSaleCurrency(jpySale, "JPY"), true);
  assert.equal(window.isInventoryOriginalSaleCurrency(hkdSale, "HKD"), true);
  assert.equal(window.formatInventoryActualSalePrice({ ...usdSale, saleDate: "" }, "JPY"), "—",
    "unsold inventory must stay empty even if stale sale fields exist");
  assert.equal(window.isInventoryOriginalSaleCurrency({ ...jpySale, saleDate: "" }, "JPY"), false,
    "unsold inventory must never receive the original-sale-currency highlight");

  Object.assign(inventory[0], usdSale);
  window.switchInventoryPriceCurrency("sale", "JPY");
  const actualSaleCell = () => document.querySelector('#inventoryTableBody tr:first-child td[data-inv-col="actualSalePrice"]');
  assert.equal(document.querySelectorAll('#page-inventory th[data-inv-col="actualSalePrice"] .inv-price-currency-btn').length, 3,
    "the actual sale column must expose JPY, USD, and HKD controls");
  assert.equal(actualSaleCell().textContent.trim(), "¥150,151");
  assert.equal(actualSaleCell().classList.contains("inv-sale-currency-match"), false,
    "converted actual sales must not be highlighted as native-currency amounts");
  window.switchInventoryCostCurrency("purchase-time", "USD");
  assert.equal(actualSaleCell().textContent.trim(), "$1,001",
    "purchase-cost currency controls must also change actual sale currency");
  assert.equal(actualSaleCell().classList.contains("inv-sale-currency-match"), true,
    "actual sales must receive the blue highlight when the native transaction currency is selected");
  assert.equal(document.getElementById("inv-actual-sale-usd").getAttribute("aria-pressed"), "true");
  document.getElementById("inv-actual-sale-hkd").click();
  assert.equal(actualSaleCell().textContent.trim(), "HK$7,508");
  assert.equal(actualSaleCell().classList.contains("inv-sale-currency-match"), false);
  for (const id of ["inv-purchase-time-hkd", "inv-purchase-current-hkd", "inv-sale-hkd", "inv-actual-sale-hkd"]) {
    assert.equal(document.getElementById(id).getAttribute("aria-pressed"), "true",
      "the actual-sale switch must update all four linked currency controls");
  }
  assert.equal(document.querySelector('#inventoryTableBody tr:first-child td[data-inv-col="saleRate"]').textContent.trim(), "1 HKD = ¥20.00",
    "the sale-rate column must show the rate used for the selected sale display currency");
  document.getElementById("inv-actual-sale-usd").click();
  assert.equal(actualSaleCell().textContent.trim(), "$1,001",
    "switching through other currencies must return to the exact original transaction amount");
  assert.equal(inventory[0].actualSalePrice, 1001,
    "display currency switching must not mutate the saved transaction amount");
  for (const [sale, currency, expected] of [[jpySale, "JPY", "¥300,000"], [hkdSale, "HKD", "HK$10,000"]]) {
    Object.assign(inventory[0], sale);
    document.getElementById(`inv-actual-sale-${currency.toLowerCase()}`).click();
    assert.equal(actualSaleCell().textContent.trim(), expected);
    assert.equal(actualSaleCell().classList.contains("inv-sale-currency-match"), true,
      `${currency} sales must highlight their original currency in the table`);
  }
  inventory[0].saleDate = "";
  window.renderInventoryTable();
  assert.equal(actualSaleCell().textContent.trim(), "—");
  assert.equal(actualSaleCell().classList.contains("inv-sale-currency-match"), false,
    "the table must not highlight an unsold item with stale transaction fields");
} finally {
  actualSaleTestData.fxRateHistory = actualSaleHistoryBackup;
  for (const key of Object.keys(inventory[0])) {
    if (!Object.hasOwn(actualSaleItemBackup, key)) delete inventory[0][key];
  }
  Object.assign(inventory[0], actualSaleItemBackup);
  window.switchInventoryPriceCurrency("sale", "JPY");
}

const foreignInventoryItem = {
  purchaseCurrency: "USD", purchaseOriginalPrice: 1000, purchasePrice: 150000,
  purchaseFxRateScaled: 15000000000, purchaseFxScale: 100000000,
  salePrice: 2000,
};
assert.equal(window.formatInventoryRegisteredPurchaseRate(foreignInventoryItem), "1 USD = ¥150.00");
assert.equal(window.formatInventoryPurchaseTimeRate(foreignInventoryItem, "USD"), "￥150");
assert.equal(window.isInventoryOriginalPurchaseCurrency(foreignInventoryItem, "USD"), true);
assert.equal(window.formatInventoryPurchaseCostAtRegisteredRate(foreignInventoryItem), "¥150,000");
assert.equal(window.formatInventoryPurchaseCostAtCurrentRate(foreignInventoryItem), "¥155,000");
assert.equal(window.formatItemDetailPurchasePrice(foreignInventoryItem, "JPY"), "¥150,000",
  "item detail cost must retain its fixed purchase-time JPY value");
assert.equal(window.formatItemDetailPurchasePrice(foreignInventoryItem, "USD"), "$1,000",
  "item detail cost must convert with the saved purchase rate");
assert.match(window.getItemDetailPurchaseRateNote(foreignInventoryItem), /仕入レート：1 USD = ¥150\.00/u,
  "item detail must identify the saved purchase rate instead of the current rate");
assert.equal(window.formatInventoryGrossMargin(foreignInventoryItem), "50.0%", "gross margin must compare the current-rate JPY cost with the current-rate JPY sale price");
assert.equal(window.formatInventoryGrossMargin({ salePrice: 0, purchasePrice: 1000 }), "—", "zero sale price must not divide by zero");

window.showItemDetail(inventory[0].code);
assert.equal(document.getElementById("itemDetailModal").classList.contains("hidden"), false, "item detail modal must open");
assert.match(document.getElementById("itemDetailTitle").textContent, new RegExp(inventory[0].code));
assert.match(document.getElementById("itemDetailBody").textContent, /¥850,000/u, "item detail purchase price must remain available in JPY");
assert.match(document.getElementById("itemDetailBody").textContent, /¥1,180,015/u, "item detail sale price must follow the selected JPY display");
const itemDetailInfoPanel = document.getElementById("itemDetailInfoPanel");
const itemDetailLabels = [...itemDetailInfoPanel.querySelectorAll(".detail-label")].map(label => label.textContent.trim());
assert.ok(itemDetailLabels.includes("原価（仕入レート）"), "product details must label purchase-rate cost explicitly");
assert.deepEqual(
  [...itemDetailInfoPanel.querySelectorAll(".item-detail-summary-field > .detail-label")].map(label => label.textContent.trim()),
  ["原価（仕入レート）", "想定売価", "想定粗利率", "ステータス"],
  "cost, sale price, gross margin, and status must share one four-column summary row",
);
assert.equal(document.getElementById("item-detail-gross-margin-value").textContent, window.formatInventoryGrossMargin(inventory[0]));
assert.equal(itemDetailInfoPanel.querySelectorAll('[data-item-detail-price="purchase"] .item-detail-currency-btn').length, 3,
  "purchase-rate cost must support JPY, USD, and HKD display");
assert.match(itemDetailInfoPanel.querySelector('[data-item-detail-price="purchase"] .item-detail-price-rate').textContent, /^仕入レート：/u);
assert.match(itemDetailInfoPanel.querySelector('[data-item-detail-price="sale"] .item-detail-price-rate').textContent, /^現在レート：/u);
assert.equal(itemDetailInfoPanel.querySelectorAll('[data-item-detail-price="sale"] .item-detail-currency-btn').length, 3,
  "sale price must support JPY, USD, and HKD display");
assert.equal(typeof window.switchItemDetailPriceCurrency, "function");
window.switchItemDetailPriceCurrency("purchase", "USD");
assert.equal(document.getElementById("item-detail-purchase-value").textContent,
  window.formatItemDetailPurchasePrice(inventory[0], "USD"));
assert.equal(document.getElementById("item-detail-sale-value").textContent,
  window.formatItemDetailSalePrice(inventory[0], "USD"), "switching the cost currency must also update the sale price");
assert.equal(itemDetailInfoPanel.querySelector('[data-item-detail-price="purchase"] [data-currency="USD"]').getAttribute("aria-pressed"), "true");
assert.equal(itemDetailInfoPanel.querySelector('[data-item-detail-price="sale"] [data-currency="USD"]').getAttribute("aria-pressed"), "true",
  "cost and sale price currency buttons must stay linked");
window.switchItemDetailPriceCurrency("purchase", "HKD");
assert.match(document.getElementById("item-detail-purchase-value").textContent, /^HK\$/u);
assert.match(document.getElementById("item-detail-sale-value").textContent, /^HK\$/u,
  "switching the cost to HKD must also update the sale price");
window.switchItemDetailPriceCurrency("sale", "USD");
assert.equal(document.getElementById("item-detail-sale-value").textContent, "$7,613");
assert.equal(document.getElementById("item-detail-purchase-value").textContent,
  window.formatItemDetailPurchasePrice(inventory[0], "USD"), "switching the sale price currency must also update the cost");
assert.equal(itemDetailInfoPanel.querySelector('[data-item-detail-price="purchase"] [data-currency="USD"]').getAttribute("aria-pressed"), "true");
assert.equal(itemDetailInfoPanel.querySelector('[data-item-detail-price="sale"] [data-currency="USD"]').getAttribute("aria-pressed"), "true");
window.switchItemDetailPriceCurrency("sale", "HKD");
assert.match(document.getElementById("item-detail-sale-value").textContent, /^HK\$/u);
assert.match(document.getElementById("item-detail-purchase-value").textContent, /^HK\$/u,
  "switching the sale price to HKD must also update the cost");
window.switchItemDetailPriceCurrency("purchase", "JPY");
assert.equal(itemDetailInfoPanel.querySelector('[data-item-detail-price="purchase"] [data-currency="JPY"]').getAttribute("aria-pressed"), "true");
assert.equal(itemDetailInfoPanel.querySelector('[data-item-detail-price="sale"] [data-currency="JPY"]').getAttribute("aria-pressed"), "true");
assert.ok(itemDetailLabels.includes("型番"), "product details must use the unified 型番 label");
assert.ok(itemDetailLabels.includes("素材"), "product details must use the unified 素材 label");
assert.equal(itemDetailLabels.some(label => /Ref\.|本体/u.test(label)), false,
  "product details must not retain legacy field-name annotations");
assert.equal(window.eval("_unifyTerminologyText('商品管理番号 / 商品コード / モデル名 / 仕入担当者 / 仕入金額（税抜） / 売価（JPY）')"),
  "管理番号 / 管理番号 / モデル / バイヤー / 原価 / 売価");
const itemGallery = itemDetailInfoPanel.querySelector(".item-gallery");
assert.equal(itemDetailInfoPanel.lastElementChild, itemGallery, "product image gallery must be the final section of item information");
assert.equal(itemGallery.querySelectorAll(".gallery-thumb-button").length, inventory[0].images.length,
  "all product images must be rendered as accessible thumbnails");
assert.equal(itemGallery.querySelector(".gallery-thumb-button.active")?.getAttribute("aria-pressed"), "true",
  "the selected thumbnail must expose its active state");
const secondGalleryButton = itemGallery.querySelectorAll(".gallery-thumb-button")[1];
window.switchGalleryMain(secondGalleryButton.querySelector("img").src, secondGalleryButton);
assert.equal(secondGalleryButton.classList.contains("active"), true, "thumbnail selection must update the active image");
assert.equal(secondGalleryButton.getAttribute("aria-pressed"), "true", "thumbnail selection must update aria-pressed");
assert.equal(typeof window.switchItemDetailTab, "function", "item detail must expose a tab switcher");
assert.equal(typeof window.downloadInventoryProductTag, "function", "product tag must be downloadable");
assert.equal(typeof window.printInventoryProductTag, "function", "product tag must be printable");
window.switchItemDetailTab("tag");
const productTagPanel = document.getElementById("itemDetailTagPanel");
assert.equal(productTagPanel.hidden, false, "product tag tab must become visible");
assert.equal(productTagPanel.querySelectorAll(".inventory-product-tag").length, 1, "product tag must show the front only");
assert.equal(productTagPanel.querySelectorAll(".inventory-product-tag-qr").length, 1, "one QR per product tag");
const tagMaterialName = window.masterDisplayName('material', inventory[0].material, 'tag');
const tagMovementName = window.masterDisplayName('movement', inventory[0].movement, 'tag');
assert.equal(productTagPanel.querySelector('[data-tag-field="brand-material"] text').textContent, window.masterDisplayName('brand',inventory[0].brand,'tag')+'（'+tagMaterialName+'）');
assert.equal(productTagPanel.querySelector('[data-tag-field="model"] text').textContent, inventory[0].model);
assert.ok(productTagPanel.querySelector('[data-tag-field="belt-movement"] text').textContent.includes(tagMovementName));
assert.equal(productTagPanel.querySelector('[data-tag-field="management-code"] text').textContent, inventory[0].code);
assert.equal(productTagPanel.querySelectorAll('.inventory-product-tag-front [data-tag-price="sale"]').length, 1);
assert.equal(productTagPanel.querySelectorAll('.inventory-product-tag-front [data-tag-price="cost"]').length, 1);
const downloadedTagSvg = window.createInventoryProductTagSvg(inventory[0]);
assert.ok(downloadedTagSvg.includes(inventory[0].code));
assert.ok(!downloadedTagSvg.includes('裏面'));
assert.equal(new window.DOMParser().parseFromString(downloadedTagSvg,'image/svg+xml').querySelectorAll('parsererror').length,0);
// Purchase tag printing follows stable line links, not inventory filters or the open product.
{
  const first={...inventory[0],code:'TAG-ONE',_id:'tag-1',purchaseSlipLineId:'tag-line'};
  const second={...inventory[0],code:'TAG-TWO',_id:'tag-2',purchaseSlipLineId:'tag-line'};
  const unrelated={...inventory[0],code:'TAG-OTHER',_id:'tag-other',purchaseSlipLineId:'other-line'};
  inventory.push(first,second,unrelated);
  const slip={id:'TAG-SLIP',lines:[{_id:'tag-line',quantity:2},{code:'TAG-ONE'},{lineItemKind:'part',code:'PART-ONE'}]};
  const result=window.getPurchaseSlipTagProducts(slip);
  assert.deepEqual(Array.from(result.products,item=>item.code),['TAG-ONE','TAG-TWO']);
  assert.equal(result.missingLines,0);
  assert.equal(window.getPurchaseSlipTagProducts({lines:[{code:'NO-SUCH-PRODUCT'}]}).missingLines,1);
  assert.equal(window.getPurchaseSlipTagProducts({lines:[]}).products.length,0);
  assert.equal(window.getPurchaseSlipTagProducts({lines:[{_id:'tag-line',quantity:3}]}).missingLines,1,'partially reflected quantities must not silently print an incomplete set');
  const footer=window.buildSlipDetailFooter('purchase',slip);
  assert.ok(footer.indexOf('purchase-product-tag-print-button')<footer.indexOf('purchase-arrival-scan-button'));
  assert.ok(!window.buildSlipDetailFooter('sales',slip).includes('purchase-product-tag-print-button'));
  inventory.splice(inventory.indexOf(first),3);
}
const blankNoteTagHtml = window.renderInventoryProductTagPanel({ ...inventory[0], note: "" });
assert.doesNotMatch(blankNoteTagHtml, /Preview inventory: monthly purchase-date distribution|記載なし/u,
  "blank product notes must stay empty on the product tag");
const blankNoteTagSvg = window.createInventoryProductTagSvg({ ...inventory[0], note: "" });
assert.doesNotMatch(blankNoteTagSvg, /Preview inventory: monthly purchase-date distribution|記載なし/u,
  "blank product notes must stay empty in downloaded and printed tags");
assert.equal(document.getElementById("itemDetailTagDownloadBtn").classList.contains("hidden"), false);
assert.equal(document.getElementById("itemDetailTagPrintBtn").classList.contains("hidden"), false);
assert.equal(document.getElementById("itemDetailEditBtn").classList.contains("hidden"), true);
assert.match(window.prBulkDownloadCSV.toString(), /'管理番号'.*'ブランド'.*'モデル'.*'型番'.*'シリアル'.*'原価（JPY）'/u,
  "purchase-return CSV headers must use the unified inventory terminology");
assert.match(window.buildSlipProductCsvRows.toString(), /'委託先','管理番号','ブランド','モデル'/u,
  "consignment CSV headers must use 管理番号 and モデル");
window.switchItemDetailTab("info");

const serialVisibility = document.querySelector('#inv-column-panel input[value="serial"]');
serialVisibility.checked = false;
window._invColumnVisibilityChanged(serialVisibility);
assert.equal(document.querySelector('th[data-inv-col="serial"]').classList.contains("inv-col-hidden"), true);
assert.equal(document.querySelector('#inventoryTableBody td[data-inv-col="serial"]').classList.contains("inv-col-hidden"), true);
assert.equal(document.getElementById("inv-column-count").textContent, "21/24");

const purchaseVisibility = document.querySelector('#inv-column-panel input[value="purchasePrice"]');
const saleVisibility = document.querySelector('#inv-column-panel input[value="salePrice"]');
purchaseVisibility.checked = false;
window._invColumnVisibilityChanged(purchaseVisibility);
saleVisibility.checked = false;
window._invColumnVisibilityChanged(saleVisibility);
assert.equal(document.querySelector('th[data-inv-col="purchasePrice"]').classList.contains("inv-col-hidden"), true);
assert.equal(document.querySelector('th[data-inv-col="salePrice"]').classList.contains("inv-col-hidden"), true);
window._invShowAllColumns();
assert.equal(document.getElementById("inv-column-count").textContent, "24/24");
assert.equal(document.querySelectorAll("#inventoryTable .inv-col-hidden").length, 0);

assert.equal(typeof window.openInventoryQr, "function", "inventory QR display must be globally callable");
assert.equal(typeof window.printAllInventoryQrLabels, "function", "bulk inventory QR printing must be globally callable");
assert.equal(document.querySelectorAll('#inventoryTableBody button[onclick*="openInventoryQr"]').length, 0,
  "the inventory list must not expose stocktake QR controls after product tags replace them");
assert.equal(document.querySelector('button[onclick="printAllInventoryQrLabels()"]'), null,
  "the inventory list must not expose bulk stocktake QR printing");
assert.equal(document.getElementById("inv-f-shape-query"), null,
  "inventory search must not expose the shape filter");
assert.equal(document.querySelector('th[data-inv-col="marking"]').textContent.trim(), "",
  "the inventory marking column header must be visually blank");
window.openInventoryQr(inventory[0].code);
assert.equal(document.getElementById("inventoryQrModal").classList.contains("hidden"), false, "inventory QR modal must open");
assert.equal(document.getElementById("inventoryQrCode").textContent, inventory[0].code);
assert.ok(document.querySelector("#inventoryQrPreview svg"), "inventory QR must render as a local SVG");
assert.equal(window.getInventoryQrPayload(inventory[0]), inventory[0].code, "QR payload must be the stable management number only");
window.closeInventoryQr();
assert.equal(document.getElementById("inventoryQrModal"), null, "closing the generated QR modal must clean up its DOM");

window.eval("_barcodeMode = 'stocktake'; _barcodeCooldown = false; _barcodeLastCode = '';");
window._onBarcodeDetected(inventory[0].code);
assert.equal(document.getElementById("stkCodeInput").value, inventory[0].code,
  "camera QR detection must place the management number in the stocktake input");

assert.ok(document.querySelector('button[onclick="openBarcodeScanner(\'inventory-search\')"]'),
  "inventory management-number search must expose a QR camera button");
window.eval("_barcodeMode = 'inventory-search'; _barcodeCooldown = false; _barcodeLastCode = '';");
window._onBarcodeDetected(inventory[1].code);
assert.equal(document.getElementById("inv-f-code").value, inventory[1].code,
  "inventory QR detection must place the management number in the search input");
assert.equal(document.getElementById("inv-result-area").style.display, "",
  "inventory QR detection must run the inventory search");
document.getElementById("inv-f-code").value = "";
window.execInventorySearch();

window.switchInventoryPriceCurrency("purchase", "JPY");
window.switchInventoryPriceCurrency("sale", "USD");

assert.equal(typeof window.toggleInventoryStatusSort, "function", "inventory status sorting must be globally callable");
document.getElementById("inv-f-status").value = "";
window.execInventorySearch();
assert.equal(document.getElementById("inv-status-sort-th").getAttribute("aria-sort"), "none");
const normalizeInventoryStatus = (status) => {
  if (["仕入返品", "仕入返品中", "仕入返品処理中", "return_pending"].includes(status)) return "仕入返品処理中";
  if (["取消", "取消済", "取り消し", "仕入返品済", "仕入返品処理済", "cancelled"].includes(status)) return "仕入返品処理済";
  return status;
};
const statusOrder = ["在庫中", "取置中", "委託中", "仕入返品処理中", "出荷済", "売上済", "仕入返品処理済", "保留"];
const expectedAscendingStatuses = [...inventory]
  .sort((a, b) => {
    const aStatus = normalizeInventoryStatus(a.status);
    const bStatus = normalizeInventoryStatus(b.status);
    const aRank = statusOrder.includes(aStatus) ? statusOrder.indexOf(aStatus) : statusOrder.length;
    const bRank = statusOrder.includes(bStatus) ? statusOrder.indexOf(bStatus) : statusOrder.length;
    return aRank - bRank;
  })
  .slice(0, 10)
  .map((item) => normalizeInventoryStatus(item.status));
window.toggleInventoryStatusSort();
assert.equal(document.getElementById("inv-status-sort-th").getAttribute("aria-sort"), "ascending");
assert.deepEqual(
  [...document.querySelectorAll('#inventoryTableBody td[data-inv-col="status"]')].map((cell) => cell.textContent.replace('●', '').trim()),
  expectedAscendingStatuses,
  "ascending status sort must follow the inventory workflow order",
);
const expectedDescendingStatuses = [...inventory]
  .sort((a, b) => {
    const aStatus = normalizeInventoryStatus(a.status);
    const bStatus = normalizeInventoryStatus(b.status);
    const aRank = statusOrder.includes(aStatus) ? statusOrder.indexOf(aStatus) : statusOrder.length;
    const bRank = statusOrder.includes(bStatus) ? statusOrder.indexOf(bStatus) : statusOrder.length;
    return bRank - aRank;
  })
  .slice(0, 10)
  .map((item) => normalizeInventoryStatus(item.status));
window.toggleInventoryStatusSort();
assert.equal(document.getElementById("inv-status-sort-th").getAttribute("aria-sort"), "descending");
assert.deepEqual(
  [...document.querySelectorAll('#inventoryTableBody td[data-inv-col="status"]')].map((cell) => cell.textContent.replace('●', '').trim()),
  expectedDescendingStatuses,
  "descending status sort must reverse the workflow order",
);
window.toggleInventoryStatusSort();
assert.equal(document.getElementById("inv-status-sort-th").getAttribute("aria-sort"), "none");

const marginSortItems = [
  { code: 'missing', purchasePrice: 100, actualSalePriceJpy: 100 },
  { code: 'high', purchasePrice: 10, actualSalePriceJpy: 100, saleDate: '2026-09-14' },
  { code: 'negative', purchasePrice: 150, actualSalePriceJpy: 100, saleDate: '2026-09-14' },
  { code: 'zero', purchasePrice: 100, actualSalePriceJpy: 100, saleDate: '2026-09-14' },
  { code: 'equal', purchasePrice: 10, actualSalePriceJpy: 100, saleDate: '2026-09-14' },
  { code: 'no-price', purchasePrice: 100, actualSalePriceJpy: 0, saleDate: '2026-09-14' },
];
const marginOriginalOrder = marginSortItems.map(item => item.code);
assert.deepEqual(Array.from(window.sortInventoryByGrossMargin(marginSortItems, 'ascending'), item => item.code),
  ['negative', 'zero', 'high', 'equal', 'missing', 'no-price']);
assert.deepEqual(Array.from(window.sortInventoryByGrossMargin(marginSortItems, 'descending'), item => item.code),
  ['high', 'equal', 'zero', 'negative', 'missing', 'no-price']);
assert.deepEqual(marginSortItems.map(item => item.code), marginOriginalOrder, 'sorting must not mutate source inventory');
const marginSortButton = document.getElementById('inv-gross-margin-sort-btn');
marginSortButton.click();
assert.equal(document.getElementById('inv-gross-margin-sort-th').getAttribute('aria-sort'), 'ascending');
assert.equal(window.eval('inventoryPage'), 1);
assert.deepEqual(
  [...document.querySelectorAll('#inventoryTableBody td[data-inv-col="actualGrossMargin"]')].map(cell => cell.textContent.trim()),
  Array.from(window.sortInventoryByGrossMargin(inventory, 'ascending').slice(0, 10), item => window.formatInventoryActualGrossMargin(item)),
  'margin sorting applies to the whole filtered list before pagination');
marginSortButton.click();
assert.equal(document.getElementById('inv-gross-margin-sort-th').getAttribute('aria-sort'), 'descending');
marginSortButton.click();
assert.equal(document.getElementById('inv-gross-margin-sort-th').getAttribute('aria-sort'), 'none');
window.toggleInventoryStatusSort();
marginSortButton.click();
assert.equal(document.getElementById('inv-status-sort-th').getAttribute('aria-sort'), 'none');
window.toggleInventoryStatusSort();
assert.equal(document.getElementById('inv-gross-margin-sort-th').getAttribute('aria-sort'), 'none');
window.toggleInventoryStatusSort();
window.toggleInventoryStatusSort();

// Inventory amount/date/rate sorting follows the displayed currency across all pages.
{
  const rows = [
    { code: 'large', purchaseDate: '2026-09-20', purchasePrice: 1000, purchaseCurrency: 'USD', purchaseOriginalPrice: 10, purchaseRate: 100, saleDate: '2026-09-20', actualSaleCurrency: 'USD', actualSalePrice: 20, actualSalePriceJpy: 2000, saleFxRateScaled: 10000, saleFxScale: 100 },
    { code: 'small', purchaseDate: '2026-08-01', purchasePrice: 200, purchaseCurrency: 'USD', purchaseOriginalPrice: 1, purchaseRate: 200, saleDate: '2026-08-01', actualSaleCurrency: 'USD', actualSalePrice: 100, actualSalePriceJpy: 20000, saleFxRateScaled: 20000, saleFxScale: 100 },
    { code: 'empty', purchaseDate: '', purchasePrice: 0 },
    { code: 'equal', purchaseDate: '2026-08-01', purchasePrice: 200, purchaseCurrency: 'USD', purchaseOriginalPrice: 1, purchaseRate: 200 },
  ];
  const codes = (key, direction) => Array.from(window.sortInventoryByColumn(rows, key, direction), item => item.code);
  window.switchInventoryLinkedCurrency('USD');
  assert.deepEqual(codes('purchaseDate','ascending'), ['small','equal','large','empty']);
  assert.deepEqual(codes('purchaseDate','descending'), ['large','small','equal','empty']);
  for (const key of ['purchasePriceAtPurchaseRate','purchasePrice']) {
    assert.deepEqual(codes(key,'ascending'), ['empty','small','equal','large']);
    assert.deepEqual(codes(key,'descending'), ['large','small','equal','empty']);
  }
  assert.equal(window.getInventoryColumnSortValue(rows[0], 'purchaseRate'), 100);
  assert.equal(window.getInventoryColumnSortValue(rows[1], 'purchaseRate'), 200);
  assert.deepEqual(codes('saleRate','ascending'), ['large','small','empty','equal']);
  assert.deepEqual(codes('saleRate','descending'), ['small','large','empty','equal']);
  assert.deepEqual(codes('actualSalePrice','ascending'), ['large','small','empty','equal']);
  assert.deepEqual(codes('actualSalePrice','descending'), ['small','large','empty','equal']);
  assert.deepEqual(rows.map(item => item.code), ['large','small','empty','equal']);
  window.switchInventoryLinkedCurrency('JPY');
  assert.equal(window.getInventoryColumnSortValue(rows[0], 'actualSalePrice'), 2000);
  assert.equal(window.getInventoryColumnSortValue(rows[0], 'purchasePriceAtPurchaseRate'), 1000);
  const keys = ['purchaseDate','purchaseRate','saleRate','purchasePriceAtPurchaseRate','purchasePrice','actualSalePrice'];
  for (const key of keys) {
    const button = document.getElementById('inv-sort-' + key + '-btn');
    const heading = document.getElementById('inv-sort-' + key + '-th');
    window.eval('inventoryPage = 2');
    button.click();
    assert.equal(heading.getAttribute('aria-sort'), 'ascending');
    assert.equal(window.eval('inventoryPage'), 1);
    assert.deepEqual(
      [...document.querySelectorAll('#inventoryTableBody td[data-inv-col="code"] code')].map(cell => cell.textContent),
      Array.from(window.sortInventoryByColumn(inventory, key, 'ascending').slice(0,10), item => item.code),
      key + ': sort the complete filtered inventory before pagination');
    button.click();
    assert.equal(heading.getAttribute('aria-sort'), 'descending');
    button.click();
    assert.equal(heading.getAttribute('aria-sort'), 'none');
  }
  window.toggleInventoryStatusSort();
  window.toggleInventoryColumnSort('purchaseDate');
  assert.equal(document.getElementById('inv-status-sort-th').getAttribute('aria-sort'),'none');
  window.toggleInventoryColumnSort('actualSalePrice');
  assert.equal(document.getElementById('inv-sort-purchaseDate-th').getAttribute('aria-sort'),'none');
  window.toggleInventoryGrossMarginSort();
  assert.equal(document.getElementById('inv-sort-actualSalePrice-th').getAttribute('aria-sort'),'none');
  window.toggleInventoryColumnSort('purchaseRate');
  assert.equal(document.getElementById('inv-gross-margin-sort-th').getAttribute('aria-sort'),'none');
  window.toggleInventoryStatusSort();
  assert.equal(document.getElementById('inv-sort-purchaseRate-th').getAttribute('aria-sort'),'none');
  window.toggleInventoryStatusSort();
  window.toggleInventoryStatusSort();
}

// Locked purchasing values must survive metadata-only saves; pencils unlock one field at a time.
{
  const target = inventory[0];
  const originalAPI = window.ZaikoAPI;
  const originalManaged = target.apiManaged;
  const originalAccessories = target.accessories;
  let submitted;
  target.apiManaged = true;
  target.accessories = [];
  window.ZaikoAPI = { updateProduct: async (_item, values) => {
    submitted = values;
    throw new Error('Test capture: no persistence');
  }};
  window.openItemEdit(target.code);
  const ids = ['purchasePrice','salePrice','purchaseDate','staff','supplier','box'];
  for (const id of ids) {
    const input = document.getElementById('ie-' + id);
    assert.equal(input.disabled, true, id + ' starts locked');
    if (input._partnerSearchController) assert.equal(input._partnerSearchController.input.disabled, true);
  }
  document.getElementById('ie-model').value = target.model + ' lock test';
  document.getElementById('ie-purchasePrice').value = '123';
  document.getElementById('ie-purchaseDate').value = '2000-01-01';
  for (const id of ['staff','supplier','box']) document.getElementById('ie-' + id).value = '';
  document.getElementById('ie-salePrice').value = '456';
  await window.saveItemEdit();
  assert.ok(submitted, 'save reaches the API capture');
  for (const field of ['purchasePrice','purchaseDate','supplier','salePrice','staff','boxNo']) {
    assert.equal(submitted[field], target[field], field + ' preserves persisted value while locked');
  }
  for (const id of ['salePrice','staff','box']) {
    document.getElementById('ie-' + id + '-unlock').click();
    assert.equal(document.getElementById('ie-' + id).disabled, false);
    const controller = document.getElementById('ie-' + id)._partnerSearchController;
    if (controller) assert.equal(controller.input.disabled, false);
    for (const fixed of ['purchasePrice','purchaseDate','supplier']) assert.equal(document.getElementById('ie-' + fixed).disabled, true);
  }
  document.getElementById('ie-salePrice').value = '456';
  document.getElementById('ie-staff').value = '';
  document.getElementById('ie-box').value = '';
  await window.saveItemEdit();
  assert.equal(submitted.salePrice,456);
  assert.equal(submitted.staff,'');
  assert.equal(submitted.boxNo,null);
  for (const field of ['purchasePrice','purchaseDate','supplier']) assert.equal(submitted[field],target[field]);
  window.closeItemEdit();
  window.openItemEdit(target.code);
  for (const id of ids) assert.equal(document.getElementById('ie-' + id).disabled,true,'reopening resets ' + id);
  for (const id of ['salePrice','staff','box']) assert.equal(document.getElementById('ie-' + id + '-unlock').disabled,false);
  window.closeItemEdit();
  window.ZaikoAPI = originalAPI;
  target.apiManaged = originalManaged;
  target.accessories = originalAccessories;
}

const originalEditImages = [...(inventory[0].images || [])];
const originalEditImageFiles = inventory[0].imageFiles;
const originalEditAccessories = [...(inventory[0].accessories || [])];
const originalEditBraceletQty = inventory[0].braceletQty;
inventory[0].images = ["/api/v1/product-files/fil_edit_1", "/api/v1/product-files/fil_edit_2"];
inventory[0].imageFiles = [
  { id: "fil_edit_1", url: inventory[0].images[0], originalName: "front.jpg", sortOrder: 0 },
  { id: "fil_edit_2", url: inventory[0].images[1], originalName: "back.jpg", sortOrder: 1 },
];
// Product details retain non-selectable workflow statuses during metadata edits.
const editStatusOriginalAPI = window.ZaikoAPI;
const editStatusOriginalValue = inventory[0].status;
const editStatusOriginalManaged = inventory[0].apiManaged;
window.ZaikoAPI = {};
inventory[0].apiManaged = true;
for (const currentStatus of ['仕入中','原価調整中','原価調整済','売上返品処理中','売上済']) {
 inventory[0].status = currentStatus;
 window.openItemEdit(inventory[0].code);
 assert.equal(document.getElementById('ie-status').value,currentStatus);
 assert.equal(document.getElementById('ie-status').disabled,true);
 window.closeItemEdit();
}
inventory[0].status = editStatusOriginalValue;
inventory[0].apiManaged = editStatusOriginalManaged;
window.ZaikoAPI = editStatusOriginalAPI;
window.openItemEdit(inventory[0].code);
assert.equal(document.getElementById("ie-salePrice").value, "7,613", "inventory edit must show the USD value with thousands separators");
assert.equal((document.querySelector('label[for="ie-salePrice"]')?.textContent || document.getElementById("ie-salePrice").previousElementSibling?.textContent || "").trim(), "想定売価");
assert.equal(document.getElementById("ie-image-count").textContent, "2 / 10枚", "inventory edit must load the product's existing images");
assert.equal(document.querySelectorAll("#ie-image-grid .item-edit-image-card").length, 2);
window.moveItemEditImage(1, -1);
assert.match(document.querySelector("#ie-image-grid .item-edit-image-preview").src, /fil_edit_2$/u, "image order controls must update the preview order");
window.removeItemEditImage(1);
assert.equal(document.getElementById("ie-image-count").textContent, "1 / 10枚", "image deletion must remain staged in the edit modal");
window.handleItemEditImageFiles([new window.File(["image"], "replacement.webp", { type: "image/webp" })]);
assert.equal(document.getElementById("ie-image-count").textContent, "2 / 10枚", "valid images must be addable from inventory edit");
assert.match(document.getElementById("ie-image-grid").textContent, /replacement\.webp/u);
const braceletPartsCheckbox = document.querySelector('#ie-accessories input[value="BRACELET PARTS"]');
braceletPartsCheckbox.checked = false;
window.itemEditAccessoryChanged(braceletPartsCheckbox);
assert.equal(document.getElementById("ie-bracelet-qty-row").hidden, true,
  "inventory edit must hide the bracelet quantity when BRACELET PARTS is not selected");
braceletPartsCheckbox.checked = true;
window.itemEditAccessoryChanged(braceletPartsCheckbox);
assert.equal(document.getElementById("ie-bracelet-qty-row").hidden, false,
  "selecting BRACELET PARTS in inventory edit must reveal the quantity input");
assert.equal(document.getElementById("ie-bracelet-qty").required, true,
  "a selected BRACELET PARTS accessory must require a link count");
window.closeItemEdit();
inventory[0].accessories = [...new Set([...originalEditAccessories, "BRACELET PARTS"])];
inventory[0].braceletQty = 8;
window.openItemEdit(inventory[0].code);
assert.equal(document.getElementById("ie-bracelet-qty-row").hidden, false,
  "inventory edit must show the bracelet quantity for an already-selected BRACELET PARTS accessory");
assert.equal(document.getElementById("ie-bracelet-qty").value, "8",
  "inventory edit must restore the persisted bracelet link count");
const inventoryPurchasePriceInput = document.getElementById("ie-purchasePrice");
inventoryPurchasePriceInput.value = "１２３４５６７";
window.priceFormatHandler(inventoryPurchasePriceInput);
assert.equal(inventoryPurchasePriceInput.value, "1,234,567", "integer money input must insert separators while typing");
assert.equal(window.getPriceValue(inventoryPurchasePriceInput), 1234567, "formatted integer money must retain its numeric value");
window.closeItemEdit();
inventory[0].images = originalEditImages;
inventory[0].imageFiles = originalEditImageFiles;
inventory[0].accessories = originalEditAccessories;
inventory[0].braceletQty = originalEditBraceletQty;
// Dynamically created monetary fields must wait for IME commit, including when
// compositionend is the last native event (no subsequent input event).
const imePriceProbe = document.createElement('input');
imePriceProbe.type = 'text';
imePriceProbe.addEventListener('input', () => window.priceFormatHandler(imePriceProbe));
document.body.appendChild(imePriceProbe);
for (const text of ['1234', '１２３４']) {
  imePriceProbe.value = '';
  imePriceProbe.dispatchEvent(new window.CompositionEvent('compositionstart', { bubbles: true }));
  for (let length = 1; length <= text.length; length++) {
    imePriceProbe.value = text.slice(0, length);
    imePriceProbe.dispatchEvent(new window.InputEvent('input', { bubbles: true, isComposing: true, inputType: 'insertCompositionText', data: text.slice(0, length) }));
    assert.equal(imePriceProbe.value, text.slice(0, length), 'formatting must not rewrite an active IME range');
  }
  imePriceProbe.dispatchEvent(new window.CompositionEvent('compositionend', { bubbles: true, data: text }));
  assert.equal(imePriceProbe.value, '1,234', 'committed IME numbers must format once');
  assert.equal(window.getPriceValue(imePriceProbe), 1234);
  imePriceProbe.dispatchEvent(new window.InputEvent('input', { bubbles: true, inputType: 'insertText', data: text }));
  assert.equal(imePriceProbe.value, '1,234', 'a trailing input event must not duplicate committed numbers');
}
imePriceProbe.dispatchEvent(new window.CompositionEvent('compositionstart', { bubbles: true }));
imePriceProbe.value = '';
imePriceProbe.dispatchEvent(new window.InputEvent('input', { bubbles: true, isComposing: true }));
imePriceProbe.dispatchEvent(new window.CompositionEvent('compositionend', { bubbles: true, data: '' }));
assert.equal(imePriceProbe.value, '', 'cancelling composition must not restore stale digits');
imePriceProbe.value = '5678';
imePriceProbe.dispatchEvent(new window.InputEvent('input', { bubbles: true }));
assert.equal(imePriceProbe.value, '5,678', 'ordinary typing resumes after IME cancellation');
imePriceProbe.remove();

const marketWinningBidInput = document.getElementById("me-marketPriceJpy");
marketWinningBidInput.value = "１２３４５６７";
window.priceFormatHandler(marketWinningBidInput);
assert.equal(marketWinningBidInput.value, "1,234,567", "JPY winning bid input must insert separators while typing");
assert.equal(window.getPriceValue(marketWinningBidInput), 1234567, "formatted JPY winning bid must retain its numeric value");
assert.equal(document.getElementById("pu-bracelet-qty").type, "number", "quantities must remain unformatted numeric inputs");

const sales = window.eval("APP_DATA.sales");
assert.equal(sales[0].total, sales[0].items[0].salePrice, "sales totals must be recalculated in USD");
assert.equal(window.eval("APP_DATA.purchaseSlips[0].lines[0].salePrice"), 7613, "purchase slip sale price must be converted to USD");

window.navigateTo("purchase");
assert.match(document.querySelector('label[for="pu-code"]').textContent, /管理番号/u);
assert.equal(document.getElementById("pu-currency-jpy").getAttribute("aria-checked"), "true",
  "product registration must default the original-cost currency to JPY");
assert.equal(document.getElementById("pu-purchase-type-domestic").getAttribute("aria-checked"), "true",
  "product registration must default the purchase category to domestic dealer / auction");
assert.equal(document.getElementById("pu-tax-category").value, "consumption_tax",
  "product registration must default the tax category to consumption tax");
window.puSetPurchaseCurrency("USD");
assert.equal(document.getElementById("pu-currency-usd").getAttribute("aria-checked"), "true");
assert.match(document.getElementById("pu-price-label").textContent, /USD/u);
assert.match(document.getElementById("pu-purchase-rate").textContent, /1 USD = ¥/u,
  "selecting an original-cost currency must show its current purchase rate");
window.puSetPurchaseType("personal");
assert.equal(document.getElementById("pu-purchase-type-personal").getAttribute("aria-checked"), "true");
window.puSetTaxCategory("tax_equivalent");
assert.match(document.getElementById("pu-tax-category-description").textContent, /社内参考値/u);
window.puSetPurchaseCurrency("JPY");
window.puSetPurchaseType("domestic");
window.puSetTaxCategory("consumption_tax");
assert.equal(document.getElementById("pu-code").getAttribute("oninput"), "puManagementNumberInput(this)");
assert.ok(document.querySelector('button[onclick="openBarcodeScanner(\'product-registration\')"]'),
  "product registration management number must expose a QR camera button");
const productDetailControlOrder = [...document.querySelectorAll("#page-purchase .card")]
  .find((card) => /商品詳細/u.test(card.querySelector(".card-header")?.textContent || ""))
  .querySelectorAll("input, select");
const productDetailControlIds = [...productDetailControlOrder].map((control) => control.id).filter(Boolean);
assert.ok(productDetailControlIds.indexOf("pu-shape") > productDetailControlIds.indexOf("pu-dial"),
  "shape must appear below the dial field in product details");
assert.ok(productDetailControlIds.indexOf("pu-marking") > productDetailControlIds.indexOf("pu-sale-price"),
  "marking must appear below the sale-price field in product details");
const purchaseSlips = window.eval("APP_DATA.purchaseSlips");
const existingManagementNumber = inventory[0].code;
const inventoryCountBeforeExistingLookup = inventory.length;
const purchaseSlipCountBeforeExistingLookup = purchaseSlips.length;
window.eval("_barcodeMode = 'product-registration'; _barcodeCooldown = false; _barcodeLastCode = '';");
window._onBarcodeDetected(existingManagementNumber);
assert.equal(document.getElementById("pu-code").value, existingManagementNumber);
assert.equal(document.getElementById("pu-code").disabled, true, "an existing management number must be locked against duplicate registration");
assert.match(document.getElementById("pu-existing-banner").textContent, /管理番号/u);
assert.match(document.getElementById("pu-existing-banner").textContent, /既に在庫登録済み/u);
assert.match(document.getElementById("pu-existing-banner").textContent, new RegExp(inventory[0].status));
assert.equal(document.querySelectorAll("#pu-existing-banner .pu-banner-actions button").length, 2);
const existingImagesBeforeProductRegistration = [...(inventory[0].images || [])];
window.eval("uploadedImages[0] = 'blob:product-registration-image'; uploadedImageFiles[0] = new File(['image'], 'registered.webp', { type: 'image/webp' }); renderImageGrid();");
assert.match(document.querySelector('#page-purchase .btn-primary.btn-lg').textContent, /画像1枚を保存する/u,
  "selecting an image for an existing management number must expose an explicit image save action");
await window.savePurchase();
assert.equal(inventory.length, inventoryCountBeforeExistingLookup, "an existing management number must not create another inventory product");
assert.equal(purchaseSlips.length, purchaseSlipCountBeforeExistingLookup, "an existing management number must not issue another purchase slip");
assert.equal(inventory[0].images.length, existingImagesBeforeProductRegistration.length + 1,
  "the selected image must be appended to the existing product");
assert.equal(document.getElementById("pu-code").value, "", "successful image save must clear the management number to prevent duplicate submission");
inventory[0].images = existingImagesBeforeProductRegistration;

// Stored images are visible but never staged again as uploads.
{
  const existing={...inventory[0],code:'STORED-IMAGE-TEST',images:['/saved-one.png','/saved-two.png'],
    imageFiles:[{id:'saved-1',url:'/saved-one.png'},{id:'saved-2',url:'/saved-two.png'}],imageCount:2};
  inventory.push(existing);
  window.puCodeSearch(existing.code);
  const grid=document.getElementById('imageGrid');
  assert.equal(grid.querySelectorAll('[data-saved="true"]').length,2);
  assert.equal(grid.querySelectorAll('[data-saved="true"] .del-btn').length,0,'stored images are not removable pending uploads');
  assert.deepEqual([...grid.querySelectorAll('img')].map(img=>img.getAttribute('src')),existing.images);
  assert.equal(window.eval('uploadedImageFiles.filter(Boolean).length'),0,'lookup must not reupload saved images');
  window.eval("uploadedImages[0]='new-preview';uploadedImageFiles[0]=new File(['new'],'new.png');renderImageGrid();");
  assert.deepEqual([...grid.querySelectorAll('img')].map(img=>img.getAttribute('src')),[...existing.images,'new-preview']);
  assert.equal(grid.querySelectorAll('.del-btn').length,1);
  grid.querySelector('.del-btn').click();
  assert.equal(grid.querySelectorAll('img').length,2);
  assert.equal(window.eval('uploadedImageFiles.filter(Boolean).length'),0);
  grid.querySelector('img').click();
  assert.equal(document.querySelector('#product-image-lightbox img').getAttribute('src'),existing.images[0]);
  document.querySelector('.product-image-lightbox-close').click();
  existing.imageFiles=Array.from({length:10},(_,index)=>({id:String(index),url:'/saved-'+index+'.png'}));
  existing.images=existing.imageFiles.map(file=>file.url);existing.imageCount=10;
  window.puCodeSearch(existing.code);
  assert.equal(grid.querySelectorAll('[data-saved="true"]').length,10);
  assert.equal(grid.querySelectorAll('.image-slot:not(.filled)').length,0,'ten stored images leave no upload slot');
  window.triggerImageUpload(0);
  assert.equal(window.eval('uploadedImageFiles.filter(Boolean).length'),0);
  window.resetPurchaseForm({silent:true});
  assert.equal(grid.querySelectorAll('img').length,0,'reset clears saved previews too');
  inventory.splice(inventory.indexOf(existing),1);
}

// Existing product fields stay locked, and the inventory edit route clears registration.
{
  const existing = {...inventory[0], code:"LOCK-EXISTING-TEST", sku:"", comment:"保存済みコメント",
    purchasePrice:0, purchaseOriginalPrice:0, salePrice:0, purchaseType:"overseas", purchaseCurrency:"USD"};
  for (const [id,prop] of [["pu-shape","shape"],["pu-marking","marking"],["pu-belt","belt"],["pu-brand","brand"]]) {
    existing[prop] = [...document.getElementById(id).options].find(option=>option.value)?.value || "";
  }
  inventory.push(existing);
  window.puCodeSearch(existing.code);
  for (const id of ["pu-code","pu-price","pu-sale-price","pu-comment","pu-tax-category","pu-currency-usd","pu-purchase-type-overseas"]) {
    assert.equal(document.getElementById(id).disabled,true,id+" must be locked for an existing product");
  }
  for (const [id,prop] of [["pu-shape","shape"],["pu-marking","marking"],["pu-belt","belt"],["pu-brand","brand"]]) {
    if (!existing[prop]) continue;
    const select=document.getElementById(id);
    assert.equal(select.value,existing[prop]);
    assert.equal(select.disabled,true);
    assert.equal(select._partnerSearchController.input.disabled,true);
    assert.equal(select._partnerSearchController.wrapper.classList.contains("pu-locked"),true);
  }
  assert.equal(document.getElementById("pu-sku").disabled,false,"an empty field remains available");
  document.getElementById("pu-sku").value="DRAFT-SKU";
  window._pu3KeySearchTrigger("sku");
  assert.equal(document.getElementById("pu-code").disabled,true,"editing an empty SKU must not unlock the matched product");
  assert.equal(document.getElementById("pu-comment").value,existing.comment);
  window.savePurchaseCarryover();
  window.openMatchedInventoryFromProductRegistration();
  await new Promise(resolve=>window.setTimeout(resolve,10));
  window.closeItemEdit();
  window.navigateTo("purchase");
  for (const id of ["pu-code","pu-supplier","pu-sku","pu-model","pu-shape","pu-marking","pu-comment","pu-belt","pu-price","pu-sale-price"]) {
    const field=document.getElementById(id);
    assert.equal(field.value,"",id+" must be reset after leaving for inventory edit");
    assert.equal(field.disabled,false,id+" must be available for new registration");
    if(field._partnerSearchController){
      assert.equal(field._partnerSearchController.input.value,"");
      assert.equal(field._partnerSearchController.input.disabled,false);
      assert.equal(field._partnerSearchController.wrapper.classList.contains("pu-locked"),false);
    }
  }
  assert.equal(document.getElementById("pu-date").value,window.getLocalDateISO());
  assert.equal(window.sessionStorage.getItem("pu_carryover_supplier"),null);
  assert.equal(window.eval("_puMatchedItem"),null);
  assert.equal(document.getElementById("pu-existing-banner").classList.contains("visible"),false);
  inventory.splice(inventory.indexOf(existing),1);
}

// Existing products accept missing details without overwriting populated fields or creating slips.
{
  const originalAPI = window.ZaikoAPI;
  const existing = {...inventory[0], code:'MISSING-DETAILS-TEST', sku:'', comment:'', shape:'', marking:'',
    belt:'', note:'', purchasePrice:0, purchaseOriginalPrice:0, salePrice:0, accessories:[], images:[], imageFiles:[], imageCount:0};
  inventory.push(existing);
  const count = inventory.length, slipCount = purchaseSlips.length;
  window.puCodeSearch(existing.code);
  document.getElementById('pu-sku').value = 'SKU-1-1';
  document.getElementById('pu-comment').value = '追加コメント';
  document.getElementById('pu-note').value = '追加備考';
  for (const id of ['pu-shape','pu-marking','pu-belt']) {
    const select = document.getElementById(id);
    select.value = [...select.options].find(option=>option.value)?.value || '';
  }
  // Even a programmatic change to a disabled control must not replace stored information.
  document.getElementById('pu-price').value = '9999';
  document.getElementById('pu-sale-price').value = '9999';
  const originalModel = existing.model;
  document.getElementById('pu-model').value = 'DO-NOT-OVERWRITE';
  document.getElementById('pu-sku').dispatchEvent(new window.Event('input',{bubbles:true}));
  assert.match(document.querySelector('#page-purchase button[onclick="savePurchase()"]').textContent,/不足情報を保存する/u);
  const additions = window._puCollectMissingProductValues(existing);
  assert.equal(additions.sku,'SKU-1-1');
  assert.equal(additions.comment,'追加コメント');
  assert.equal(Object.hasOwn(additions,'purchasePrice'),false);
  assert.equal(Object.hasOwn(additions,'salePrice'),false);
  assert.equal(Object.hasOwn(additions,'model'),false);
  let submitted, calls = 0;
  window.ZaikoAPI = {updateProduct:async()=>{calls++;throw new Error('保存失敗テスト');}};
  await window.savePurchase();
  assert.equal(existing.sku,'');
  assert.equal(document.getElementById('pu-sku').value,'SKU-1-1','failed saves keep additions');
  assert.equal(document.getElementById('pu-code').value,existing.code);
  window.ZaikoAPI.updateProduct = async(item,values)=>{calls++;submitted=values;Object.assign(item,values);};
  await window.savePurchase();
  assert.equal(calls,2);
  assert.equal(submitted.sku,'SKU-1-1');
  assert.equal(submitted.comment,'追加コメント');
  assert.equal(submitted.note,'追加備考');
  assert.equal(submitted.model,originalModel);
  assert.equal(submitted.purchasePrice,0);
  assert.equal(submitted.salePrice,0);
  for (const field of ['shape','marking','belt']) assert.equal(submitted[field],additions[field]||'');
  assert.equal(document.getElementById('pu-code').value,'');
  window.puCodeSearch(existing.code);
  assert.equal(document.getElementById('pu-sku').value,'SKU-1-1');
  assert.equal(document.getElementById('pu-sku').disabled,true,'saved additions become read-only on lookup');
  assert.equal(document.getElementById('pu-comment').disabled,true);
  window.resetPurchaseForm({silent:true});
  existing.sku='';existing.comment='';
  window.puCodeSearch(existing.code);
  document.getElementById('pu-sku').value='SKU-WITH-IMAGES';
  window.eval("uploadedImages[0]='image-one';uploadedImages[1]='image-two';uploadedImageFiles[0]=new File(['one'],'one.png');uploadedImageFiles[1]=new File(['two'],'two.png');");
  let uploads=0;
  window.ZaikoAPI.appendProductImages=async(item,files)=>{
    uploads++;
    if(uploads===2)throw new Error('画像保存失敗テスト');
    item.images.push(files[0].name);item.imageCount=item.images.length;
  };
  await window.savePurchase();
  assert.equal(existing.sku,'SKU-WITH-IMAGES');
  assert.equal(document.getElementById('pu-sku').disabled,true);
  assert.equal(window.eval('uploadedImageFiles.filter(Boolean).length'),1,'only failed images remain pending');
  await window.savePurchase();
  assert.equal(uploads,3,'retry must not resend a confirmed image');
  assert.deepEqual(existing.images,['one.png','two.png']);
  assert.equal(inventory.length,count);
  assert.equal(purchaseSlips.length,slipCount);
  assert.equal(document.getElementById('pu-code').value,'');
  inventory.splice(inventory.indexOf(existing),1);
  window.ZaikoAPI=originalAPI;
}


// Registration receives purchasing products even with no added data; QR uses the same path.
{
  const originalAPI=window.ZaikoAPI;
  window.ZaikoAPI=undefined;
  const first={...inventory[0],code:'ARRIVAL-REG-ONE',_id:'arrival-product-1',purchaseSlipLineId:'arrival-line-1',status:'仕入中',sku:'',comment:'',images:[],imageFiles:[],imageCount:0};
  const second={...first,code:'ARRIVAL-REG-TWO',_id:'arrival-product-2',purchaseSlipLineId:'arrival-line-2'};
  const slip={...purchaseSlips[0],id:'ARRIVAL-REG-SLIP',_id:'arrival-purchase',pendingArrivalCount:2,arrivedCount:0,paidAt:'KEEP-PAYMENT',
    lines:[{code:first.code,_id:'arrival-line-1',productId:first._id,currentStatus:'仕入中'},{code:second.code,_id:'arrival-line-2',productId:second._id,currentStatus:'仕入中'}]};
  inventory.push(first,second);purchaseSlips.push(slip);
  window.resetPurchaseForm({silent:true});window.puCodeSearch(first.code);
  assert.match(document.querySelector('#page-purchase button[onclick="savePurchase()"]').textContent,/登録する/u);
  await window.savePurchase();
  assert.equal(first.status,'在庫中');assert.equal(second.status,'仕入中');
  assert.equal(slip.lines[0].currentStatus,'在庫中');assert.equal(slip.pendingArrivalCount,1);
  assert.equal(window.getPurchaseArrivalStatus(slip),'処理中');
  let receiveCalls=0, metadataCalls=0;
  window.ZaikoAPI={
    updateProduct:async(item,values)=>{metadataCalls++;Object.assign(item,values);},
    receivePurchaseProduct:async()=>{receiveCalls++;throw new Error('入荷登録失敗テスト');},
  };
  window.eval("_barcodeMode='product-registration';_barcodeCooldown=false;_barcodeLastCode='';");
  window._onBarcodeDetected(second.code);
  document.getElementById('pu-sku').value='ADDED-ON-ARRIVAL';
  await window.savePurchase();
  assert.equal(metadataCalls,1);assert.equal(receiveCalls,1);
  assert.equal(second.status,'仕入中');assert.equal(document.getElementById('pu-code').value,second.code);
  assert.equal(second.sku,'ADDED-ON-ARRIVAL','saved metadata survives arrival failure');
  window.ZaikoAPI.receivePurchaseProduct=async(purchase,code)=>{
    receiveCalls++;assert.equal(purchase,slip);assert.equal(code,second.code);
    second.status='在庫中';slip.lines[1].currentStatus='在庫中';slip.pendingArrivalCount=0;slip.arrivedCount=2;
    return {result:'received',inventoryStatus:'in_stock'};
  };
  await window.savePurchase();
  assert.equal(metadataCalls,1,'retry does not resend saved metadata');assert.equal(receiveCalls,2);
  assert.equal(second.status,'在庫中');assert.equal(window.getPurchaseArrivalStatus(slip),'処理済');
  assert.equal(slip.paidAt,'KEEP-PAYMENT');assert.equal(document.getElementById('pu-code').value,'');
  assert.ok(window.buildSlipDetailBody('purchase',slip).includes('在庫中'));
  assert.equal(inventory.filter(item=>item.code.startsWith('ARRIVAL-REG-')).length,2);
  inventory.splice(inventory.indexOf(first),2);purchaseSlips.splice(purchaseSlips.indexOf(slip),1);
  window.ZaikoAPI=originalAPI;
}

// Belt selection must survive the product form's IME listeners and final save formatting.
const purchaseBeltData = window.eval("APP_DATA");
const originalPurchaseBelts = purchaseBeltData.beltMaterialRecords;
purchaseBeltData.beltMaterialRecords = [{code:"0001",name:"SS"},{code:"0005",name:"Calf"}];
window.populateBeltMaterialMasterSelect("pu-belt");
window.initInputFormatListeners();
const purchaseBelt = document.getElementById("pu-belt");
const purchaseBeltControl = purchaseBelt._partnerSearchController;
purchaseBeltControl.input.focus();
purchaseBeltControl.input.value = "1";
purchaseBeltControl.input.dispatchEvent(new window.Event("input", {bubbles:true}));
purchaseBeltControl.input.dispatchEvent(new window.KeyboardEvent("keydown", {key:"Enter",bubbles:true}));
assert.equal(purchaseBelt.value, "SS", "code lookup must not full-width-convert the master value");
assert.equal(purchaseBeltControl.input.value, "SS");
purchaseBeltControl.input.value = "";
purchaseBeltControl.input.dispatchEvent(new window.Event("input", {bubbles:true}));
const calfOption = [...purchaseBeltControl.panel.querySelectorAll(".partner-search-option")]
  .find(option => option.textContent.includes("Calf"));
assert.ok(calfOption);
calfOption.click();
purchaseBeltControl.input.blur();
purchaseBelt.dispatchEvent(new window.Event("blur"));
assert.equal(purchaseBelt.value, "Calf", "dropdown selection must survive blur");
assert.equal(purchaseBeltControl.input.value, "Calf");

// Composition confirmation is not an inventory-search Enter key.
{
  const originalCodeSearch = window.puCodeSearch;
  const originalSkuSearch = window._pu3KeySearchTrigger;
  const calls = [];
  window.puCodeSearch = value => calls.push(value);
  window._pu3KeySearchTrigger = value => calls.push(value);
  window.initInputFormatListeners();
  window.initInputFormatListeners();
  for (const id of ['pu-code', 'pu-sku']) {
    const field = document.getElementById(id);
    const original = field.value;
    field.dispatchEvent(new window.CompositionEvent('compositionstart', {bubbles:true}));
    field.value = '１２３';
    field.dispatchEvent(new window.InputEvent('input', {bubbles:true,isComposing:true}));
    await new Promise(resolve => window.setTimeout(resolve, 350));
    assert.deepEqual(calls, [], id + ' must not search a composing value');
    const enter = new window.KeyboardEvent('keydown', {key:'Enter',isComposing:true,keyCode:229,bubbles:true,cancelable:true});
    field.dispatchEvent(enter);
    assert.equal(enter.defaultPrevented, false, id + ' must let IME handle confirmation');
    assert.deepEqual(calls, []);
    field.dispatchEvent(new window.CompositionEvent('compositionend', {bubbles:true,data:'１２３'}));
    assert.equal(field.value, '123');
    await new Promise(resolve => window.setTimeout(resolve, 350));
    assert.deepEqual(calls, id === 'pu-code' ? ['123'] : [], 'committed management code is looked up once');
    calls.length = 0;
    field.dispatchEvent(new window.KeyboardEvent('keydown', {key:'Enter',bubbles:true,cancelable:true}));
    assert.deepEqual(calls, [id === 'pu-code' ? '123' : 'sku'], 'ordinary Enter still searches');
    calls.length = 0;
    field.value = original;
  }
  window.puCodeSearch = originalCodeSearch;
  window._pu3KeySearchTrigger = originalSkuSearch;
}

const singlePurchaseCode = "3012990001";
const inventoryCountBeforeSinglePurchase = inventory.length;
const purchaseSlipCountBeforeSinglePurchase = purchaseSlips.length;
document.getElementById("pu-date").value = "2099-12-30";
window.puSetPurchaseType("personal");
window.puSetTaxCategory("out_of_scope");
document.getElementById("pu-supplier").value = "";
assert.equal(document.getElementById("pu-supplier").required, false,
  "personal product registration must not require a supplier");
assert.equal(document.getElementById("pu-supplier-required").style.display, "none",
  "personal product registration must hide the supplier required mark");
document.getElementById("pu-code").disabled = false;
document.getElementById("pu-code").value = singlePurchaseCode;
document.getElementById("pu-price").value = "123000";
document.getElementById("pu-sale-price").value = "2500";
document.getElementById("pu-sku").value = "SINGLE-SLIP-TEST";
document.getElementById("pu-model").value = "単品伝票テスト";
window.savePurchase();
assert.equal(inventory.length, inventoryCountBeforeSinglePurchase + 1, "single product registration must add inventory");
assert.equal(purchaseSlips.length, purchaseSlipCountBeforeSinglePurchase + 1, "single product registration must issue a purchase slip");
const savedSinglePurchaseSlip = purchaseSlips.at(-1);
const savedSingleInventory = inventory.find((item) => item.code === singlePurchaseCode);
assert.equal(savedSinglePurchaseSlip.source, "single-product");
assert.equal(savedSinglePurchaseSlip.lines.length, 1);
assert.equal(savedSinglePurchaseSlip.lines[0].code, singlePurchaseCode);
assert.equal(savedSinglePurchaseSlip.lines[0].purchasePrice, 123000);
assert.equal(savedSinglePurchaseSlip.lines[0].salePrice, 2500);
assert.equal(savedSinglePurchaseSlip.purchaseCurrency, "JPY");
assert.equal(savedSinglePurchaseSlip.purchaseTaxMode, "personal");
assert.equal(savedSinglePurchaseSlip.taxCategory, "out_of_scope");
assert.equal(savedSinglePurchaseSlip.supplier, "");
assert.equal(savedSingleInventory.status, "仕入中");
assert.equal(savedSingleInventory.belt, "Calf", "registration must retain the selected belt master name");
assert.equal(savedSinglePurchaseSlip.lines[0].productDetail.belt, "Calf", "purchase slip must retain the selected belt master name");
purchaseBeltData.beltMaterialRecords = originalPurchaseBelts;
window.populateBeltMaterialMasterSelect("pu-belt");
assert.equal(savedSingleInventory.purchaseSlipId, savedSinglePurchaseSlip.id, "inventory must link back to its auto-issued purchase slip");
const purchaseApprovalDetailHtml = window.buildReadableApprovalDetail({
  targetType: "purchase_slip",
  targetId: savedSinglePurchaseSlip._id || savedSinglePurchaseSlip.id,
});
assert.match(purchaseApprovalDetailHtml, new RegExp(savedSinglePurchaseSlip.id),
  "purchase approvals must display the business slip number instead of only the internal DB id");
assert.match(purchaseApprovalDetailHtml, /仕入伝票を開く/u);
assert.match(purchaseApprovalDetailHtml, /商品詳細/u,
  "purchase approvals must provide links to their product details");

assert.equal(typeof window.applyBusinessRecordState, "function");
assert.equal(typeof window.refreshLinkedBusinessViews, "function");
assert.equal(typeof window.persistBusinessWorkflowState, "function");
window.navigateTo("inventory");
window.resetInventorySearch();
await new Promise(resolve => window.setTimeout(resolve, 0));
assert.equal(document.activeElement, document.getElementById("inv-f-code"),
  "opening inventory must focus a search field so Enter immediately executes the current filters");
document.activeElement.dispatchEvent(new window.KeyboardEvent("keydown", {
  key: "Enter", bubbles: true, cancelable: true,
}));
assert.notEqual(document.getElementById("inv-result-area").style.display, "none",
  "pressing Enter immediately after opening inventory must execute the current search filters");
window.resetInventorySearch();
document.getElementById("inv-f-code").value = singlePurchaseCode;
document.getElementById("inv-f-status").value = "仕入中";
document.getElementById("inv-f-code").dispatchEvent(new window.KeyboardEvent("keydown", {
  key: "Enter", bubbles: true, cancelable: true,
}));
assert.notEqual(document.getElementById("inv-result-area").style.display, "none",
  "pressing Enter in an inventory search field must execute the search");
assert.match(document.getElementById("inventoryTableBody").textContent, new RegExp(singlePurchaseCode),
  "inventory Enter search must render the matching inventory record");
window.navigateTo("sales-list");
assert.equal(document.getElementById("slipBulkControls"), null,
  "the shared bulk download and print controls must not exist");
for (const slipType of ["purchase", "shipping", "consignment", "sales", "salesreturn", "purchasereturn"]) {
  window.switchSlipTab(slipType);
  window.showAllSlipList();
  assert.equal(document.querySelectorAll("#slipListBody .slip-csv-row-check").length, document.querySelectorAll("#slipListBody tr").length,
    `${slipType} slips must expose CSV selection for each displayed row`);
  assert.equal(document.getElementById("prBulkInvoiceBtn"), null,
    `${slipType} slips must not retain the legacy purchase-return issue button`);
}
const slipTabOrder = [...document.querySelectorAll(".slip-type-tabs > .slip-type-tab")].map(tab => tab.id);
assert.equal(document.getElementById("sltab-purchaseadjustment"), null,
  "the separate purchase adjustment tab must be removed");
assert.equal(document.getElementById("sltab-salesadjustment"), null,
  "the separate sales adjustment tab must be removed");
assert.equal(document.getElementById("sltab-adjustment"), null,
  "adjustment slips must not appear as a tab in the accounting document list");
assert.deepEqual(slipTabOrder,
  ["sltab-purchase", "sltab-shipping", "sltab-consignment", "sltab-sales", "sltab-salesreturn", "sltab-purchasereturn"]);
const retainedAccountingStatus = document.getElementById("slip-filter-status").value;
window.navigateTo("cost-adjustment-slips");
window.showAllSlipList();
assert.equal(document.getElementById("page-cost-adjustment-slips").classList.contains("hidden"), false);
assert.equal(document.getElementById("sharedSlipListCard").parentElement.id, "page-cost-adjustment-slips");
assert.match(document.getElementById("sharedSlipListTitle").textContent, /調整伝票/u);
assert.match(document.getElementById("slipFilterPartyLabel").textContent, /取引先/u);
assert.equal(window.getFilteredSlipData().length, 0,
  "the unified adjustment tab must begin with independent adjustment data sources");
window.switchSlipTab("purchase");
assert.equal(document.getElementById("sharedSlipListCard").parentElement.id, "page-sales-list",
  "leaving the dedicated adjustment page must restore the shared list to the accounting page");
assert.equal(document.getElementById("slip-filter-status").value, retainedAccountingStatus, "returning from adjustment slips must retain accounting search status");
window.switchSlipTab("purchase");
assert.equal(document.getElementById("slip-filter-status").value, "processing", "explicit tab selection must default to processing");
assert.deepEqual([...document.querySelectorAll("#slip-filter-status option")].map(option => option.textContent),
  ["処理中", "未払い", "処理済", "すべて"],
  "purchase slips must expose processing, unpaid and completed status filters");
savedSinglePurchaseSlip.rawStatus = "confirmed";
savedSinglePurchaseSlip.arrivalStatus = "processing";
savedSinglePurchaseSlip.pendingArrivalCount = 1;
savedSinglePurchaseSlip.paidAt = null;
savedSinglePurchaseSlip.lines[0].currentStatus = "仕入中";
savedSingleInventory.status = "仕入中";
window.execSlipFilter();
assert.match(document.getElementById("slipListBody").textContent, new RegExp(savedSinglePurchaseSlip.id),
  "purchase-slip processing search must include a slip that still contains a purchasing product");
assert.deepEqual(Array.from(window.getPurchaseSlipStatusKeys(savedSinglePurchaseSlip)), ["processing", "unpaid"],
  "a purchasing unpaid slip must expose both statuses");
assert.match(document.getElementById("slipListBody").textContent, /処理中/u);
assert.match(document.getElementById("slipListBody").textContent, /未払い/u);
document.getElementById("slip-filter-status").value = "unpaid";
window.execSlipFilter();
assert.match(document.getElementById("slipListBody").textContent, new RegExp(savedSinglePurchaseSlip.id),
  "unpaid search must include a slip without a payment date even while it is processing");
window.openSlipDetail("purchase", savedSinglePurchaseSlip.id);
assert.match(document.getElementById("slipDetailFooter").textContent, /入荷スキャン/u,
  "purchase-slip detail must expose the physical-arrival scanner in its footer");
assert.match(document.getElementById("slipDetailBody").textContent, /仕入中/u,
  "purchase-slip detail must show the pending product status");
const purchaseDetailHeaders = [...document.querySelectorAll("#slipDetailBody .purchase-slip-detail-table thead th")]
  .map((header) => header.textContent.trim());
assert.equal(purchaseDetailHeaders.indexOf("ステータス"), purchaseDetailHeaders.indexOf("モデル") + 1,
  "the product status column must appear immediately after the model column");
assert.match(purchaseDetailHeaders[purchaseDetailHeaders.indexOf("ステータス") + 1], /^仕入金額/u,
  "the product status column must appear immediately before purchase cost");
const purchaseProductRow = document.querySelector("#slipDetailBody .purchase-slip-product-row");
assert.equal(purchaseProductRow?.getAttribute("tabindex"), "0",
  "purchase product rows must be keyboard focusable");
purchaseProductRow.click();
assert.equal(document.getElementById("itemDetailModal").classList.contains("hidden"), false,
  "clicking a purchase product row must open its product detail modal");
assert.match(document.getElementById("itemDetailTitle").textContent, new RegExp(singlePurchaseCode),
  "the product detail modal must open the clicked purchase product");
assert.equal(document.getElementById("slipDetailOverlay").classList.contains("hidden"), false,
  "the purchase slip must remain behind product detail so closing returns to the slip");
document.getElementById("itemDetailModal").classList.add("hidden");
window.closeSlipDetail();
const originalRevisePurchaseSlip = window.ZaikoAPI?.revisePurchaseSlip;
savedSinglePurchaseSlip.apiManaged = true;
savedSinglePurchaseSlip._id = "pur-revision-test";
savedSinglePurchaseSlip.staffCode = window.eval("APP_DATA.staffRecords[0]?.code || ''");
savedSinglePurchaseSlip.lines[0]._id = "pul-revision-test";
let purchaseRevisionPayload = null;
window.ZaikoAPI = window.ZaikoAPI || {};
window.ZaikoAPI.revisePurchaseSlip = async (_record, payload) => { purchaseRevisionPayload = payload; return {}; };
window.openSlipRevise("purchase", savedSinglePurchaseSlip.id);
assert.equal(document.getElementById("slipReviseSaveButton").disabled, true,
  "purchase revision save must remain disabled until a required reason is entered");
document.getElementById("slipReviseNote").value = "仕入金額の入力訂正";
window.updateSlipReviseSaveState();
assert.equal(document.getElementById("slipReviseSaveButton").disabled,true,'reason alone must not enable saving');
assert.equal(await window.submitSlipRevise(),false);
assert.equal(purchaseRevisionPayload,null,'reason-only submission must not call the API');
const revisionSku=document.getElementById('rv-pu-sku-0');
const originalRevisionSku=revisionSku.value;
revisionSku.value=originalRevisionSku+'-CHANGED';revisionSku.dispatchEvent(new window.Event('input',{bubbles:true}));
assert.equal(document.getElementById('slipReviseSaveButton').disabled,false);
revisionSku.value=originalRevisionSku;revisionSku.dispatchEvent(new window.Event('input',{bubbles:true}));
assert.equal(document.getElementById('slipReviseSaveButton').disabled,true,'reverting the value must disable saving again');
document.getElementById('rv-pu-pp-0').value=String(savedSinglePurchaseSlip.lines[0].purchasePrice+1);
window.updateSlipReviseSaveState();
assert.equal(document.getElementById('slipReviseSaveButton').disabled,false,'a real value change with a reason enables saving');
assert.equal(await window.submitSlipRevise(), true);
assert.equal(purchaseRevisionPayload.reason, "仕入金額の入力訂正");
assert.equal(purchaseRevisionPayload.lines[0].id, "pul-revision-test");
assert.equal(purchaseRevisionPayload.notes, savedSinglePurchaseSlip.note || "",
  "the DB revision must send the slip remarks together with the required save reason");
if (originalRevisePurchaseSlip) window.ZaikoAPI.revisePurchaseSlip = originalRevisePurchaseSlip;
else delete window.ZaikoAPI.revisePurchaseSlip;

const revisedYenSale = window.eval("APP_DATA.sales.find((record) => record.id === 'SL-2026-0001')");
const originalReviseSaleSlip = window.ZaikoAPI?.reviseSaleSlip;
revisedYenSale.apiManaged = true;
revisedYenSale._id = "sal-revision-yen-test";
revisedYenSale.currency = "JPY";
revisedYenSale.inputCurrency = "JPY";
revisedYenSale.displaySubtotalMinor = 1200000;
revisedYenSale.items[0]._id = "sln-revision-yen-test";
revisedYenSale.items[0].inputCurrency = "JPY";
revisedYenSale.items[0].inputAmount = 1200000;
let saleRevisionPayload = null;
window.ZaikoAPI.reviseSaleSlip = async (_record, payload) => { saleRevisionPayload = payload; return {}; };
window.openSlipRevise("sales", revisedYenSale.id);
assert.match(document.getElementById("slipReviseFields").textContent, /売価（JPY）/u,
  "a JPY sale revision must label its editable sales price as JPY");
assert.equal(document.getElementById("rv-sale-price-0").value, "1,200,000",
  "a JPY sale revision must start from the original JPY transaction amount rather than its USD conversion");
document.getElementById("rv-sale-price-0").value = "1,250,000";
document.getElementById("slipReviseNote").value = "円建て売価の入力訂正";
window.updateSlipReviseSaveState();
assert.equal(await window.submitSlipRevise(), true);
assert.equal(saleRevisionPayload.lines[0].id, "sln-revision-yen-test");
assert.equal(saleRevisionPayload.lines[0].unitPriceMinor, 1250000,
  "the DB revision must submit a JPY amount without USD conversion");
assert.equal(saleRevisionPayload.reason, "円建て売価の入力訂正",
  "the required save reason must be sent for persistence in the sales-slip remarks");
if (originalReviseSaleSlip) window.ZaikoAPI.reviseSaleSlip = originalReviseSaleSlip;
else delete window.ZaikoAPI.reviseSaleSlip;

// Sales-change edits are line-specific deltas, not replacement sales prices.
// Use only an in-memory API stub so this contract never creates a real slip.
const salesChangeData = window.eval("APP_DATA");
const salesChangeOriginalSales = [...salesChangeData.sales];
const salesChangeOriginalAPI = window.ZaikoAPI;
const salesChangeOriginalCreate = window.ZaikoAPI?.createSalesAdjustment;
const salesChangeSource = {
  _id: "sal-change-test", id: "SI-CHANGE-TEST", date: "2026-09-14", buyer: "B001",
  currency: "USD", inputCurrency: "USD", status: "処理済", apiManaged: true,
  items: [
    { _id: "sln-change-a", code: "CHANGE-A", brand: "<img src=x onerror=alert(1)>", model: "A & B",
      inputCurrency: "USD", inputAmount: 9200, salePrice: 9200 },
    { _id: "sln-change-b", code: "CHANGE-B", brand: "オメガ", model: "Speedmaster",
      inputCurrency: "USD", inputAmount: 9850, salePrice: 9850 },
  ],
};
const makeSalesChangeRevision = (id, direction, amount, overrides = {}) => ({
  _id: `sal-${id}`, id, documentType: "sales_revision", sourceSaleId: salesChangeSource._id,
  currency: "USD", inputCurrency: "USD", status: "処理済", rawStatus: "confirmed",
  adjustmentDirection: direction,
  items: [{ _id: `${id}-line`, sourceSaleLineId: "sln-change-a", adjustmentAmount: amount,
    inputCurrency: "USD", inputAmount: (direction === "decrease" ? -1 : 1) * amount }],
  ...overrides,
});
const salesChangeSubmissions = [];
try {
  salesChangeData.sales.push(
    salesChangeSource,
    makeSalesChangeRevision("SC-INCREASE", "increase", 300),
    makeSalesChangeRevision("SC-DECREASE", "decrease", 150),
    makeSalesChangeRevision("SC-CANCELLED", "increase", 9000, { status: "取消済", rawStatus: "cancelled" }),
    makeSalesChangeRevision("SC-DRAFT", "increase", 9000, { status: "未処理", rawStatus: "draft" }),
    makeSalesChangeRevision("SC-OTHER-SALE", "increase", 9000, { sourceSaleId: "sal-unrelated" }),
    makeSalesChangeRevision("SC-OTHER-LINE", "increase", 9000, {
      items: [{ sourceSaleLineId: "sln-unrelated", adjustmentAmount: 9000, inputAmount: 9000 }],
    }),
  );
  window.ZaikoAPI = window.ZaikoAPI || {};
  window.ZaikoAPI.createSalesAdjustment = async (record, payload) => {
    salesChangeSubmissions.push({ recordID: record._id, payload: JSON.parse(JSON.stringify(payload)) });
    return { slipNumber: "SC-MOCK-ONLY" };
  };
  assert.equal(window._salesChangeAmount(salesChangeSource, salesChangeSource.items[0]), 9350,
    "current sales price must accumulate only confirmed adjustments for the same original sale and line");
  assert.equal(window._salesChangeAmount(salesChangeSource, salesChangeSource.items[1]), 9850,
    "adjustments for another source line must not leak into the second product");
  const unpaidFooter = JSDOM.fragment(window.buildSlipDetailFooter('sales',salesChangeSource));
  assert.equal(unpaidFooter.querySelector('[onclick^="openSalesChangeModal"]').disabled,true);
  assert.equal(unpaidFooter.querySelector('[onclick^="openSlipRevise"]').disabled,false);
  window.openSalesChangeModal(salesChangeSource.id);
  assert.equal(document.getElementById('salesChangeOverlay'),null,'unpaid sale cannot open adjustment');
  window.openSlipRevise('sales',salesChangeSource.id);
  salesChangeSource.paidAt = '2026-09-17T00:00:00Z';
  assert.equal(await window.submitSlipRevise(),false,'payment after opening editor blocks save');
  assert.equal(document.getElementById('slipReviseSaveButton').disabled,true);
  window.closeSlipRevise();
  window.openSlipRevise('sales',salesChangeSource.id);
  assert.equal(document.getElementById('slipReviseOverlay').classList.contains('hidden'),true);
  const paidFooter = JSDOM.fragment(window.buildSlipDetailFooter('sales',salesChangeSource));
  assert.equal(paidFooter.querySelector('[onclick^="openSalesChangeModal"]').disabled,false);
  assert.equal(paidFooter.querySelector('[onclick^="openSlipRevise"]').disabled,true);
  window.openSalesChangeModal(salesChangeSource.id);
  const salesChangeOverlay = document.getElementById("salesChangeOverlay");
  const salesChangeClickDate = window.getLocalDateISO();
  assert.equal(salesChangeOverlay.dataset.adjustmentDate, salesChangeClickDate);
  assert.equal(document.getElementById('sales-change-date').value, salesChangeClickDate);
  assert.equal(document.getElementById('sales-change-date').readOnly, true);
  const salesChangeDirectionGroup = document.getElementById("sales-change-direction-group");
  const salesChangeTable = salesChangeOverlay.querySelector("table");
  assert.ok(salesChangeDirectionGroup.compareDocumentPosition(salesChangeTable) & window.Node.DOCUMENT_POSITION_FOLLOWING,
    "increase/decrease controls must appear above the product table");
  const salesChangeHeadings = [...salesChangeTable.querySelectorAll("th")].map(cell => cell.textContent);
  assert.ok(salesChangeHeadings.some(label => /現在売価/u.test(label)));
  assert.ok(salesChangeHeadings.some(label => /調整金額.*USD/u.test(label)),
    "the editable amount must be labelled as the adjustment delta in the sale currency");
  assert.ok(salesChangeHeadings.some(label => /理由・説明/u.test(label)));
  assert.ok(salesChangeHeadings.some(label => /変更金額.*USD/u.test(label)),
    "the calculated final sales price must be labelled 変更金額");
  assert.equal(salesChangeOverlay.querySelector("img"), null,
    "product labels must be escaped instead of interpreted as HTML in the change modal");
  assert.match(salesChangeTable.textContent, /<img src=x onerror=alert\(1\)>/u);
  assert.equal(document.getElementById("sales-change-reason"), null,
    "a single global reason must not replace the reason for each selected product");
  const salesChangeCheckA = document.getElementById("sales-change-check-0");
  const salesChangeCheckB = document.getElementById("sales-change-check-1");
  const salesChangeAmountA = document.getElementById("sales-change-amount-0");
  const salesChangeAmountB = document.getElementById("sales-change-amount-1");
  const salesChangeReasonA = document.getElementById("sales-change-reason-0");
  const salesChangeReasonB = document.getElementById("sales-change-reason-1");
  const salesChangeResultA = document.getElementById("sales-change-result-0");
  const salesChangeSave = document.getElementById("sales-change-save");
  const salesChangeIncrease = salesChangeOverlay.querySelector('input[name="sales-change-direction"][value="increase"]');
  const salesChangeDecrease = salesChangeOverlay.querySelector('input[name="sales-change-direction"][value="decrease"]');
  assert.equal(salesChangeReasonA.maxLength, 1000);
  assert.equal(salesChangeAmountA.disabled, true);
  assert.equal(salesChangeReasonA.disabled, true);
  assert.notEqual(salesChangeResultA.tagName, "INPUT", "the calculated final price must not be editable");
  assert.equal(salesChangeResultA.isContentEditable === true, false);
  assert.equal(salesChangeSave.disabled, true);
  assert.equal(window.updateSalesChangeState(), false);
  await window.submitSalesChange(salesChangeSource.id);
  assert.equal(salesChangeSubmissions.length, 0, "direct submission must revalidate rather than bypass disabled save");

  salesChangeCheckA.checked = true;
  salesChangeCheckA.dispatchEvent(new window.Event("change", { bubbles: true }));
  assert.equal(salesChangeAmountA.disabled, false);
  assert.equal(salesChangeReasonA.disabled, false);
  assert.equal(salesChangeAmountB.disabled, true);
  assert.equal(salesChangeReasonB.disabled, true);
  salesChangeIncrease.checked = true;
  salesChangeIncrease.dispatchEvent(new window.Event("change", { bubbles: true }));
  salesChangeAmountA.value = "200";
  salesChangeAmountA.dispatchEvent(new window.Event("input", { bubbles: true }));
  assert.match(salesChangeResultA.textContent, /\$9,550/u,
    "typing an adjustment must show the current cumulative price plus the adjustment");
  assert.equal(salesChangeSave.disabled, true, "each selected product requires its own reason");
  salesChangeReasonA.value = "  値引き合意  ";
  salesChangeReasonA.dispatchEvent(new window.Event("input", { bubbles: true }));
  assert.equal(salesChangeSave.disabled, false);
  salesChangeDecrease.checked = true;
  salesChangeDecrease.dispatchEvent(new window.Event("change", { bubbles: true }));
  assert.match(salesChangeResultA.textContent, /\$9,150/u,
    "changing direction must immediately recalculate the final sales price");

  for (const invalidAmount of ["0", "-1", "1.5", "9351"]) {
    salesChangeAmountA.value = invalidAmount;
    assert.equal(window.updateSalesChangeState(), false,
      `adjustment ${invalidAmount} must not allow an invalid amount or a negative final price`);
    assert.equal(salesChangeSave.disabled, true);
    await window.submitSalesChange(salesChangeSource.id);
  }
  assert.equal(salesChangeSubmissions.length, 0, "invalid edits must not call the mutation API");
  salesChangeAmountA.value = "9,350";
  assert.equal(window.updateSalesChangeState(), true, "a final sales price of zero remains valid");
  assert.match(salesChangeResultA.textContent, /\$0(?:\D|$)/u);
  salesChangeAmountA.value = "200";
  salesChangeReasonA.value = " ";
  assert.equal(window.updateSalesChangeState(), false, "whitespace-only reasons are invalid");
  salesChangeReasonA.value = "あ".repeat(1001);
  assert.equal(window.updateSalesChangeState(), false, "a reason beyond the API length limit must not be submitted");
  salesChangeReasonA.value = "  値引き合意  ";
  salesChangeCheckB.checked = true;
  salesChangeCheckB.dispatchEvent(new window.Event("change", { bubbles: true }));
  salesChangeAmountB.value = "350";
  assert.equal(window.updateSalesChangeState(), false, "the second selected product also requires a reason");
  salesChangeReasonB.value = "付属品不足への対応";
  salesChangeReasonB.dispatchEvent(new window.Event("input", { bubbles: true }));
  assert.equal(salesChangeSave.disabled, false);
  assert.match(document.getElementById("sales-change-result-1").textContent, /\$9,500/u);
  salesChangeCheckB.checked = false;
  salesChangeCheckB.dispatchEvent(new window.Event("change", { bubbles: true }));
  assert.equal(salesChangeAmountB.disabled, true);
  assert.equal(salesChangeReasonB.disabled, true);
  assert.equal(salesChangeSave.disabled, false, "deselecting a row must exclude its fields from validation");
  salesChangeCheckB.checked = true;
  salesChangeCheckB.dispatchEvent(new window.Event("change", { bubbles: true }));
  const originalSalesChangeDate = window.getLocalDateISO;
  window.getLocalDateISO = () => '2099-01-01';
  try {
    await window.submitSalesChange(salesChangeSource.id);
  } finally {
    window.getLocalDateISO = originalSalesChangeDate;
  }
  assert.equal(salesChangeSubmissions.length, 1);
  assert.deepEqual(salesChangeSubmissions[0], {
    recordID: "sal-change-test",
    payload: { adjustmentDate: salesChangeClickDate, direction: "decrease", lines: [
      { saleLineId: "sln-change-a", adjustmentAmountMinor: 200, direction: "decrease", reason: "値引き合意" },
      { saleLineId: "sln-change-b", adjustmentAmountMinor: 350, direction: "decrease", reason: "付属品不足への対応" },
    ] },
  }, "the API must receive deltas and distinct trimmed reasons, not calculated final prices");
  assert.equal(document.getElementById("salesChangeOverlay"), null);

  window.openSalesChangeModal(salesChangeSource.id);
  window.applySalesChangeDirection('decrease');
  const mixedDirectionA = document.getElementById('sales-change-line-direction-0');
  const mixedDirectionB = document.getElementById('sales-change-line-direction-1');
  assert.equal(mixedDirectionA.value, 'decrease');
  assert.equal(mixedDirectionB.value, 'decrease', 'bulk selection includes products not selected yet');
  assert.equal(document.querySelector('.sales-change-table th:nth-child(5)').textContent.trim(), '増減 必須');
  for (const index of [0, 1]) {
    document.getElementById(`sales-change-check-${index}`).checked = true;
    document.getElementById(`sales-change-amount-${index}`).value = '200';
    document.getElementById(`sales-change-reason-${index}`).value = '個別変更';
  }
  mixedDirectionB.value = 'increase';
  mixedDirectionB.dispatchEvent(new window.Event('change', { bubbles: true }));
  assert.equal(mixedDirectionA.value, 'decrease', 'individual edits must not change other rows');
  assert.match(document.getElementById('sales-change-result-0').textContent, /\$9,150/u);
  assert.match(document.getElementById('sales-change-result-1').textContent, /\$10,050/u);
  assert.equal(document.querySelector('#salesChangeOverlay input[name="sales-change-direction"]:checked'), null,
    'mixed selections must not show a misleading bulk direction');
  assert.equal(window.updateSalesChangeState(), true);
  window.applySalesChangeDirection('increase');
  assert.equal(mixedDirectionA.value, 'increase', 'bulk can overwrite individual selections');
  mixedDirectionA.value = 'decrease';
  await window.submitSalesChange(salesChangeSource.id);
  assert.equal(salesChangeSubmissions[1].payload.direction, 'mixed');
  assert.deepEqual(salesChangeSubmissions[1].payload.lines.map(line => line.direction), ['decrease', 'increase']);
  const mixedRevision = makeSalesChangeRevision('SC-MIXED', 'mixed', 0, {
    displayTotalMinor: -150,
    items: [
      { sourceSaleLineId: 'sln-change-a', direction: 'increase', adjustmentAmount: 200, reason: '増額理由' },
      { sourceSaleLineId: 'sln-change-b', direction: 'decrease', adjustmentAmount: 350, reason: '減額理由' },
    ],
  });
  const mixedPreview = JSDOM.fragment(window.buildSalesAdjustmentRecordTemplateHTML(mixedRevision));
  assert.match(mixedPreview.textContent, /\+\$200/u);
  assert.match(mixedPreview.textContent, /−\$350/u);
  assert.match(mixedPreview.textContent, /−\$150/u, 'mixed voucher total is the signed net adjustment');
  assert.doesNotMatch(mixedPreview.textContent, /増額理由：増額理由/u, 'Only public product notes belong in non-cost statement remarks');
  assert.doesNotMatch(mixedPreview.textContent, /減額理由：減額理由/u, 'Only public product notes belong in non-cost statement remarks');
  const mixedDetail = JSDOM.fragment(window.buildSlipDetailBody('sales', mixedRevision));
  assert.match(mixedDetail.textContent, /増額・減額/u);
  assert.match(mixedDetail.textContent, /−\$150/u);
  salesChangeData.sales.push(mixedRevision);
  assert.equal(window._salesChangeAmount(salesChangeSource, salesChangeSource.items[0]), 9550);
  assert.equal(window._salesChangeAmount(salesChangeSource, salesChangeSource.items[1]), 9500);
  salesChangeData.sales.pop();

  const savedSalesChangeRevision = makeSalesChangeRevision("SC-REASONS-PREVIEW", "decrease", 200, {
    date: "2026-09-14", buyer: "B001", sourceSlipNumber: salesChangeSource.id,
    displayTotalMinor: -550, reason: "商品ごとの調整理由", note: "商品ごとの調整理由",
    items: [
      { code: "CHANGE-A", brand: "ロレックス", model: "Submariner", adjustmentAmount: 200, beforeSalePrice: 9200, afterSalePrice: 9000, reason: "値引き合意 <説明>" },
      { code: "CHANGE-B", brand: "オメガ", model: "Speedmaster", adjustmentAmount: 350, beforeSalePrice: 350, afterSalePrice: 0, reason: "付属品不足への対応" },
    ],
  });
  const salesChangePreview = JSDOM.fragment(window.buildSalesAdjustmentRecordTemplateHTML(savedSalesChangeRevision));
  assert.ok(salesChangePreview.textContent.includes(`売上伝票:${salesChangeSource.id}における金額修正`),
    "revision remarks must identify the sales slip and state that its amount is being corrected");
  assert.doesNotMatch(salesChangePreview.textContent, /元売上伝票：/u);
  assert.doesNotMatch(salesChangePreview.textContent, /理由：商品ごとの調整理由/u, 'remarks must only identify the source sale');
  assert.doesNotMatch(salesChangePreview.textContent, /初期売価：\$9,200 → 変更売価：\$9,000/u, 'Only public product notes belong in non-cost statement remarks');
  assert.doesNotMatch(salesChangePreview.textContent, /元値：|変更後売価：/u);
  assert.match(salesChangePreview.textContent, /ロレックス[\s\S]*Submariner/u);
  assert.doesNotMatch(salesChangePreview.textContent, /Submariner \/ 減額|Speedmaster \/ 減額/u);
  assert.doesNotMatch(salesChangePreview.textContent, /変更売価：\$0/u, 'Only public product notes belong in non-cost statement remarks');
  assert.match(salesChangePreview.textContent, /−\$550/u, "total must remain the adjustment delta only");
  const yenRevisionPreview = JSDOM.fragment(window.buildSalesAdjustmentRecordTemplateHTML({
    ...savedSalesChangeRevision, currency: 'JPY', inputCurrency: 'JPY', adjustmentDirection: 'increase',
    items: [{ brand: 'ロレックス', model: 'Submariner', adjustmentAmount: 1000, beforeSalePrice: 9000, afterSalePrice: 10000, reason: '追加の合意' }],
  }));
  assert.doesNotMatch(yenRevisionPreview.textContent, /初期売価：¥9,000 → 変更売価：¥10,000/u, 'Only public product notes belong in non-cost statement remarks');
  assert.match(yenRevisionPreview.textContent, /\+¥1,000/u);
  assert.doesNotMatch(yenRevisionPreview.textContent, /増額理由：追加の合意/u, 'Only public product notes belong in non-cost statement remarks');
  assert.doesNotMatch(yenRevisionPreview.textContent, /Submariner \/ 増額/u);
  assert.doesNotMatch(salesChangePreview.textContent, /減額理由：値引き合意 <説明>/u, 'Only public product notes belong in non-cost statement remarks');
  assert.doesNotMatch(salesChangePreview.textContent, /減額理由：付属品不足への対応/u, 'Only public product notes belong in non-cost statement remarks');
  assert.equal(salesChangePreview.querySelector("説明"), null,
    "saved reasons must be escaped in the printable document");
  assert.match(salesChangePreview.textContent, /調整金額合計/u);
  const taxableRevision = { ...savedSalesChangeRevision, currency: 'JPY', inputCurrency: 'JPY', taxMode: 'taxable', taxRateBasisPoints: 1000,
    displaySubtotalMinor: -50000, displayTaxAmountMinor: -5000, displayTotalMinor: -55000,
    items: [{ adjustmentAmount: 50000, beforeSalePrice: 650000, afterSalePrice: 600000, direction: 'decrease', reason: '保証書なし' }] };
  const taxablePreview = JSDOM.fragment(window.buildSalesAdjustmentRecordTemplateHTML(taxableRevision));
  assert.match(taxablePreview.textContent, /消費税（10%）/u);
  assert.match(taxablePreview.textContent, /−¥5,000/u);
  assert.match(taxablePreview.textContent, /−¥55,000/u);
  assert.doesNotMatch(taxablePreview.textContent, /減額理由：保証書なし/u, 'Only public product notes belong in non-cost statement remarks');
  const taxableDetail = JSDOM.fragment(window.buildSlipDetailBody('sales', taxableRevision));
  assert.match(taxableDetail.textContent, /調整小計（税抜）/u);
  assert.match(taxableDetail.textContent, /−¥5,000/u);
  assert.match(taxableDetail.textContent, /−¥55,000/u);
  assert.equal(window._salesAdjustmentLineTax(-15, taxableRevision), -2, 'round absolute tax before applying the direction');
  assert.equal(window._salesAdjustmentLineTax(15, taxableRevision), 2);
  assert.equal(window._salesAdjustmentLineTax(-50000, {taxMode:'tax_exempt',taxRateBasisPoints:1000}), 0);
  assert.equal(window._salesAdjustmentLineTax(-50000, {taxMode:'taxable',taxRateBasisPoints:800}), -4000);
  const salesChangeDetails = JSDOM.fragment(window.buildSlipDetailBody("sales", savedSalesChangeRevision));
  const salesChangeDetailRows = [...salesChangeDetails.querySelectorAll(".appr-items-table tbody tr")];
  assert.match(salesChangeDetailRows[0].textContent, /CHANGE-A[\s\S]*値引き合意 <説明>/u,
    "saved slip details must associate the first reason with its own product row");
  assert.match(salesChangeDetailRows[1].textContent, /CHANGE-B[\s\S]*付属品不足への対応/u,
    "saved slip details must associate the second reason with its own product row");
  assert.equal(salesChangeDetails.querySelector("説明"), null);

  for (const [currency, expectedCurrencyText] of [["JPY", /¥9,250/u], ["HKD", /HK\$9,250/u]]) {
    const currencySale = { ...salesChangeSource, _id: `sal-change-${currency}`, id: `SI-CHANGE-${currency}`,
      currency, inputCurrency: currency,
      items: [{ ...salesChangeSource.items[0], _id: `sln-change-${currency}`, inputCurrency: currency }] };
    salesChangeData.sales.push(currencySale);
    window.openSalesChangeModal(currencySale.id);
    document.getElementById("sales-change-check-0").checked = true;
    window.applySalesChangeDirection('increase');
    document.getElementById("sales-change-amount-0").value = "50";
    document.getElementById("sales-change-reason-0").value = "通貨表示の確認";
    assert.equal(window.updateSalesChangeState(), true);
    assert.match(document.getElementById("sales-change-result-0").textContent, expectedCurrencyText,
      `${currency} sales must calculate and format in their native transaction currency`);
    window.closeSalesChangeModal();
  }
} finally {
  window.closeSalesChangeModal();
  salesChangeData.sales.splice(0, salesChangeData.sales.length, ...salesChangeOriginalSales);
  if (salesChangeOriginalAPI) {
    window.ZaikoAPI = salesChangeOriginalAPI;
    if (salesChangeOriginalCreate) window.ZaikoAPI.createSalesAdjustment = salesChangeOriginalCreate;
    else delete window.ZaikoAPI.createSalesAdjustment;
  } else delete window.ZaikoAPI;
}
savedSinglePurchaseSlip.arrivalStatus = "processing";
savedSinglePurchaseSlip.pendingArrivalCount = 0;
savedSinglePurchaseSlip.lines[0].currentStatus = "原価調整中";
savedSingleInventory.status = "原価調整中";
assert.equal(window.getPurchaseArrivalStatus(savedSinglePurchaseSlip), "処理中",
  "a cost-adjusting product must keep its purchase slip processing until it is scanned");
const costAdjustmentPartLine = {
  lineNo: 99, code: "P3108269999", lineItemKind: "part", currentStatus: "原価調整中",
  purchasePrice: 0, purchaseCurrency: "JPY", convertedPurchasePriceJpy: 0,
};
savedSinglePurchaseSlip.lines[0].currentStatus = "在庫中";
savedSingleInventory.status = "在庫中";
savedSinglePurchaseSlip.lines.push(costAdjustmentPartLine);
assert.equal(window.getPurchaseArrivalStatus(savedSinglePurchaseSlip), "処理中",
  "a cost-adjusting part must keep its purchase slip processing");
assert.deepEqual(Array.from(window.getPurchaseSlipStatusKeys(savedSinglePurchaseSlip)), ["processing", "unpaid"],
  "a cost-adjusting part on an unpaid slip must expose both processing and unpaid statuses");
savedSinglePurchaseSlip.lines.pop();
savedSinglePurchaseSlip.arrivalStatus = "completed";
savedSinglePurchaseSlip.pendingArrivalCount = 0;
savedSinglePurchaseSlip.lines[0].currentStatus = "在庫中";
savedSingleInventory.status = "在庫中";
assert.deepEqual(Array.from(window.getPurchaseSlipStatusKeys(savedSinglePurchaseSlip)), ["unpaid"],
  "an arrived slip without a payment date must remain unpaid rather than completed");
document.getElementById("slip-filter-status").value = "unpaid";
window.execSlipFilter();
assert.match(document.getElementById("slipListBody").textContent, new RegExp(savedSinglePurchaseSlip.id));
savedSinglePurchaseSlip.paidAt = "2099-12-31T00:00:00+09:00";
document.getElementById("slip-filter-status").value = "completed";
window.execSlipFilter();
assert.match(document.getElementById("slipListBody").textContent, new RegExp(savedSinglePurchaseSlip.id),
  "purchase-slip completed search must include a fully arrived and paid slip");
assert.deepEqual(Array.from(window.getPurchaseSlipStatusKeys(savedSinglePurchaseSlip)), ["completed"]);
document.getElementById("slip-filter-status").value = "";
window.execSlipFilter();
assert.match(document.getElementById("slipListBody").textContent, new RegExp(savedSinglePurchaseSlip.id),
  "searching with the all-status option must render all matching slips");
window.showAllSlipList();
assert.match(document.getElementById("slipListBody").textContent, new RegExp(savedSinglePurchaseSlip.id), "purchase registration must appear in the purchase-slip tab without an extra search");

const linkedShipment = {
  id: "SH-LINK-TEST", date: "2099-12-30", destination: "B001", status: "処理済",
  items: [{ code: singlePurchaseCode, brand: savedSingleInventory.brand, model: savedSingleInventory.model, wholesale: 2500 }], total: 2500, note: "連動テスト",
};
window.eval("APP_DATA.shipments").push(linkedShipment);
window.applyBusinessRecordState("shipping", linkedShipment);
assert.equal(savedSingleInventory.status, "出荷済", "shipping slips must update inventory status");
window.switchSlipTab("shipping");
document.getElementById("slip-filter-status").value = "processing";
window.execSlipFilter();
assert.match(document.getElementById("slipListBody").textContent, /SH-LINK-TEST/u);
assert.match(document.getElementById("slipListBody").textContent, /処理中/u,
  "a shipment containing at least one shipping product must be processing");
savedSingleInventory.status = "在庫中";
document.getElementById("slip-filter-status").value = "completed";
window.execSlipFilter();
assert.match(document.getElementById("slipListBody").textContent, /SH-LINK-TEST/u,
  "a shipment with no shipping products must be completed");
assert.match(document.getElementById("slipListBody").textContent, /処理済/u);
savedSingleInventory.status = "出荷済";

const linkedSale = {
  id: "SL-LINK-TEST", date: "2099-12-30", buyer: "B001", status: "確定",
  items: [{ code: singlePurchaseCode, brand: savedSingleInventory.brand, model: savedSingleInventory.model, salePrice: 2500 }], total: 2500, note: "連動テスト",
};
window.eval("APP_DATA.sales").push(linkedSale);
window.applyBusinessRecordState("sales", linkedSale);
assert.equal(savedSingleInventory.status, "売上済", "sales slips must update inventory status");
window.switchSlipTab("sales");
window.showAllSlipList();
assert.match(document.getElementById("slipListBody").textContent, /SL-LINK-TEST/u);

const linkedSalesReturn = {
  id: "SR-LINK-TEST", slipId: linkedSale.id, date: "2099-12-31", buyer: "B001", status: "承認済",
  items: [{ code: singlePurchaseCode, brand: savedSingleInventory.brand, model: savedSingleInventory.model, salePrice: 2500 }], total: 2500,
};
window.eval("APP_DATA.salesReturns").push(linkedSalesReturn);
window.applyBusinessRecordState("salesreturn", linkedSalesReturn);
assert.equal(savedSingleInventory.status, "在庫中", "sales-return slips must return products to inventory");
window.switchSlipTab("salesreturn");
window.showAllSlipList();
assert.match(document.getElementById("slipListBody").textContent, /SR-LINK-TEST/u);
// Transaction confirmation is distinct from tracking confirmation. All views must agree.
const originalSalesReturnStatus = linkedSalesReturn.status;
linkedSalesReturn.status = "処理済";
for (const [trackingNumber, confirmed, expected] of [
  ["", false, "処理中"], ["TRACK-SALES-001", false, "処理中"],
  ["TRACK-SALES-001", true, "処理済"], ["TRACK-SALES-001", false, "処理中"],
]) {
  linkedSalesReturn.trackingNo = trackingNumber;
  linkedSalesReturn.trackingConfirmed = confirmed;
  linkedSalesReturn.trackingConfirmedAt = confirmed ? "2099-12-31T12:00:00Z" : "";
  window.renderSlipList([linkedSalesReturn]);
  const returnListRow = document.querySelector("#slipListBody tr");
  assert.equal(returnListRow.cells[6].textContent.trim(), expected, "list status must use tracking confirmation rather than raw transaction status");
  window.openSalesReturnDetail(linkedSalesReturn.id);
  const statusDetail = [...document.querySelectorAll("#salesReturnDetailBody .detail-row")].find(row => row.querySelector(".detail-label")?.textContent === "ステータス");
  assert.equal(statusDetail.querySelector(".detail-value").textContent.trim(), expected, "detail and list status must agree");
  assert.equal(window.matchesSlipStatusFilter(linkedSalesReturn, "processing", "salesreturn"), !confirmed);
  assert.equal(window.matchesSlipStatusFilter(linkedSalesReturn, "completed", "salesreturn"), confirmed);
}
document.getElementById("salesReturnDetailModal").classList.add("hidden");
linkedSalesReturn.status = originalSalesReturnStatus;
delete linkedSalesReturn.trackingNo;
delete linkedSalesReturn.trackingConfirmed;
delete linkedSalesReturn.trackingConfirmedAt;


const linkedPurchaseReturn = {
  id: "PR-RET-LINK-TEST", slipId: savedSinglePurchaseSlip.id, date: "2099-12-31", supplier: "S001", status: "承認済",
  items: [{ code: singlePurchaseCode, brand: savedSingleInventory.brand, model: savedSingleInventory.model, purchasePrice: 123000, status: "承認済" }],
};
window.eval("APP_DATA.purchaseReturns").push(linkedPurchaseReturn);
window.applyBusinessRecordState("purchasereturn", linkedPurchaseReturn);
assert.equal(savedSingleInventory.status, "仕入返品処理中", "purchase-return slips must reserve products from normal inventory use");
assert.equal(window.getPurchaseReturnProcessingStatus(linkedPurchaseReturn), "処理中", "purchase returns without tracking must remain processing");
window.switchSlipTab("purchasereturn");
assert.match(document.getElementById("slipListBody").textContent, /PR-RET-LINK-TEST/u);
assert.match(document.getElementById("slipListBody").textContent, /処理中/u, "purchase-return rows without tracking must display processing");
const purchaseReturnTrackingInput = document.getElementById("pr-list-tracking-PR-RET-LINK-TEST");
assert.ok(purchaseReturnTrackingInput, "purchase-return rows must expose a tracking-number confirmation field");
purchaseReturnTrackingInput.value = "TRACK-RETURN-001";
await window.prConfirmTrackingFromList("PR-RET-LINK-TEST");
assert.equal(savedSingleInventory.status, "仕入返品処理済", "confirming purchase-return tracking must complete the linked inventory return");
assert.equal(linkedPurchaseReturn.items[0].trackingNo, "TRACK-RETURN-001", "tracking is saved only by the explicit confirmation action");
assert.equal(window.getPurchaseReturnProcessingStatus(linkedPurchaseReturn), "処理済", "purchase returns with saved tracking must display processed");
assert.ok(document.querySelector('#inv-f-status option[value="仕入返品処理済"]'), "inventory status search must include completed purchase-return products");
window.resetInventorySearch();
assert.equal(document.getElementById("inv-f-status").value, "在庫中", "inventory search must default to in-stock products");
document.getElementById("inv-f-status").value = "";
window.execInventorySearch();
assert.equal(
  document.getElementById("inventoryCount").textContent.trim(),
  `${inventory.length} 件`,
  "the all-status inventory count must include every product, including completed purchase returns",
);
document.getElementById("inv-f-status").value = "仕入返品処理済";
window.execInventorySearch();
assert.equal(
  document.getElementById("inventoryCount").textContent.trim(),
  `${inventory.filter((item) => normalizeInventoryStatus(item.status) === "仕入返品処理済").length} 件`,
  "cancelled products remain searchable when the cancelled status is selected",
);
assert.match(document.getElementById("inventoryTableBody").textContent, new RegExp(singlePurchaseCode, "u"));
document.getElementById("inv-f-status").value = "";
window.execInventorySearch();

window.eval("APP_DATA.shipments").splice(window.eval("APP_DATA.shipments").indexOf(linkedShipment), 1);
window.eval("APP_DATA.sales").splice(window.eval("APP_DATA.sales").indexOf(linkedSale), 1);
window.eval("APP_DATA.salesReturns").splice(window.eval("APP_DATA.salesReturns").indexOf(linkedSalesReturn), 1);
window.eval("APP_DATA.purchaseReturns").splice(window.eval("APP_DATA.purchaseReturns").indexOf(linkedPurchaseReturn), 1);
inventory.splice(inventory.indexOf(savedSingleInventory), 1);
purchaseSlips.splice(purchaseSlips.indexOf(savedSinglePurchaseSlip), 1);

window.navigateTo("purchase-entry");
assert.equal(typeof window.peGetStaffDisplayName, "function", "purchase slips must expose the shared staff-name resolver");
assert.ok(document.getElementById("pe-tax-domestic"), "purchase entry must expose a domestic purchase switch");
assert.ok(document.getElementById("pe-tax-personal"), "purchase entry must expose a personal purchase switch");
assert.ok(document.getElementById("pe-tax-overseas"), "purchase entry must expose an overseas purchase switch");
assert.equal(document.getElementById("pe-tax-domestic").getAttribute("aria-checked"), "true");
assert.equal(document.getElementById("pe-supplier-master-container").classList.contains("hidden"), false);
assert.equal(document.getElementById("pe-personal-supplier-container").classList.contains("hidden"), true);
assert.deepEqual(["cash", "bank-transfer", "card"].map(method => Boolean(document.getElementById(`pe-payment-${method}`))), [true, true, true],
  "purchase entry must expose cash, bank transfer, and card payment methods");
assert.equal(document.getElementById("pe-payment-bank-transfer").getAttribute("aria-checked"), "true",
  "new purchase slips must default to bank transfer");
window.peSetPaymentMethod("card");
assert.equal(document.getElementById("pe-payment-card").getAttribute("aria-checked"), "true");
assert.equal(window.eval("_peSlipData.paymentMethod"), "card");
assert.deepEqual(["jpy", "usd", "hkd"].map(code => Boolean(document.getElementById(`pe-currency-${code}`))), [true, true, true],
  "purchase currency must be selectable from JPY, USD, and HKD");
assert.equal(document.getElementById("pe-tax-mode-label").textContent.replace(/\s+/gu, " ").trim(), "仕入区分");
assert.equal(document.getElementById("pe-tax-category-label").textContent.replace(/\s+/gu, " ").trim(), "税区分");
assert.deepEqual(
  ["consumption", "equivalent", "out-of-scope"].map(category => Boolean(document.getElementById(`pe-tax-category-${category}`))),
  [true, true, true],
  "purchase entry must expose three independent tax category choices",
);
assert.equal(document.getElementById("pe-tax-category-consumption").getAttribute("aria-checked"), "true");
window.peSetTaxCategory("tax_equivalent");
assert.equal(document.getElementById("pe-tax-category-equivalent").getAttribute("aria-checked"), "true");
assert.equal(window.eval("_peSlipData.taxCategory"), "tax_equivalent");
assert.match(document.getElementById("pe-tax-category-description").textContent, /支払合計には加算しません/u);
window.peSetTaxCategory("consumption_tax");
assert.ok(document.querySelector('#page-purchase-entry button[onclick="peDownloadCSVTemplate()"]'), "purchase entry must expose a CSV template download");
assert.ok(document.getElementById("pe-csv-import-button"), "purchase entry must expose CSV detail staging");
assert.match(document.getElementById("pe-csv-import-button").textContent, /CSV取込/u);
assert.equal(document.querySelector('#peProductModal button[onclick="peExportProductCSV()"]'), null,
  "CSV must be handled by the purchase template, not by the per-product modal");
assert.equal(typeof window.peDownloadCSVTemplate, "function");
assert.equal(typeof window.peExportProductCSV, "undefined");
assert.equal(typeof window.peImportCSVText, "function");
const purchaseStaffRecord = window.eval("APP_DATA.staffRecords[0]");
const staffDisplaySlip = window.eval("APP_DATA.purchaseSlips[0]");
const originalStaffValue = staffDisplaySlip.staff;
staffDisplaySlip.staff = purchaseStaffRecord.code;
window.peRenderList();
assert.equal(window.peGetStaffDisplayName(purchaseStaffRecord.code), purchaseStaffRecord.name, "stable staff codes must resolve to the current master name");
assert.equal(document.getElementById("pe-list-tbody").textContent.includes(purchaseStaffRecord.name), true, "registered purchase slips must display the staff name");
assert.equal(document.getElementById("pe-list-tbody").textContent.includes(purchaseStaffRecord.code), false, "registered purchase slips must not expose the internal staff code");
document.querySelector('#pe-list-tbody tr[data-slip-id="'+staffDisplaySlip.id+'"]').click();
assert.equal(window.__zaikoCurrentPage,'sales-list');
assert.equal(window.eval('currentSlipTab'),'purchase');
assert.equal(document.getElementById('slipDetailOverlay').classList.contains('hidden'),false);
assert.ok(document.getElementById('slipDetailHeaderSub').textContent.includes(staffDisplaySlip.id));
assert.equal(document.getElementById('slipDetailBody').textContent.includes(purchaseStaffRecord.name),true,'shared details resolve staff codes');
assert.equal(document.getElementById('slipDetailBody').textContent.includes(purchaseStaffRecord.code),false,'shared details hide internal staff codes');
assert.match(document.getElementById('slipDetailBody').textContent,/仕入区分・税区分/u);
assert.notEqual(document.getElementById('peViewModal').style.display,'flex','legacy entry modal must not open');
window.closeSlipDetail();
window.navigateTo('purchase-entry');
assert.equal(window.eval('_peSlipData.paymentMethod'),'bank_transfer','leaving purchase entry discards the prior draft');
window.peSetPaymentMethod('card');
const purchaseSlipDetailHtml = window.buildSlipDetailBody("purchase", staffDisplaySlip);
assert.equal(purchaseSlipDetailHtml.includes("${purchaseTax.taxLabel}"), false,
  "purchase-slip details must render the tax label instead of exposing the template expression");
assert.match(purchaseSlipDetailHtml, /消費税（10%）/u);
assert.match(purchaseSlipDetailHtml, /支払い方法/u);
assert.match(purchaseSlipDetailHtml, /銀行振込/u);
assert.match(purchaseSlipDetailHtml, /purchase-slip-tax-cell/u,
  "purchase-slip details must use a dedicated balanced tax column");
assert.match(purchaseSlipDetailHtml, /purchase-slip-grand-total-row/u,
  "purchase-slip details must separate subtotal, tax and grand total rows");
staffDisplaySlip.staff = originalStaffValue;
window.peRenderList();
const addLineCountInput = document.getElementById("pe-line-add-count");
assert.ok(addLineCountInput, "purchase entry must expose the bulk line-count input");
assert.equal(addLineCountInput.inputMode, "numeric");
assert.equal(addLineCountInput.pattern, "[0-9]*");
assert.equal(addLineCountInput.value, "1");
assert.equal(typeof window.peNormalizeAddCountInput, "function");
assert.equal(typeof window.peHandleAddCountKey, "function");
const purchaseSaveButton = document.getElementById("pe-save-button");
assert.ok(purchaseSaveButton, "purchase entry must expose a save button with a stable id");

const originalPurchaseConfirm = window.confirm;
let blankSkuConfirmMessage = "";
window.confirm = (message) => {
  blankSkuConfirmMessage = message;
  return false;
};
assert.equal(window._peConfirmBlankSKURegistration([{ sku: "" }]), false,
  "cancelling the blank-SKU confirmation must stop registration");
assert.equal(blankSkuConfirmMessage, "SKUが空欄の商品があります。このまま進めますか");
window.confirm = () => true;
assert.equal(window._peConfirmBlankSKURegistration([{ sku: "   " }]), true,
  "confirming must allow a blank SKU to remain blank and continue");
let nonBlankSkuConfirmCalls = 0;
window.confirm = () => {
  nonBlankSkuConfirmCalls += 1;
  return true;
};
assert.equal(window._peConfirmBlankSKURegistration([{ sku: "SKU-001" }]), true);
assert.equal(nonBlankSkuConfirmCalls, 0,
  "non-blank SKUs must not show the confirmation popup");
window.confirm = originalPurchaseConfirm;

document.getElementById("pe-date").value = "2099-12-27";
window.peOnDateChange();
document.getElementById("pe-staff").value = document.getElementById("pe-staff").options[1].value;
window.peSetPurchaseTaxMode("personal");
document.getElementById("pe-supplier").value = "";
const personalSupplierInput = document.getElementById("pe-personal-supplier-name");
assert.ok(personalSupplierInput, "personal purchase slips must expose a transaction-only supplier-name input");
assert.equal(document.getElementById("pe-supplier-master-container").classList.contains("hidden"), true,
  "personal purchases must hide the supplier master selector");
assert.equal(document.getElementById("pe-personal-supplier-container").classList.contains("hidden"), false,
  "personal purchases must show the transaction-only supplier input");
assert.equal(personalSupplierInput.disabled, false);
personalSupplierInput.value = "個人買取テスト 太郎";
window.peOnHeaderChange();
assert.equal(window.eval("_peSlipData.supplier"), "", "temporary personal suppliers must not create a master-code reference");
assert.equal(window.eval("_peSlipData.supplierName"), "個人買取テスト 太郎");
assert.equal(window.peGetSupplierDisplayName(window.eval("_peSlipData")), "個人買取テスト 太郎");
assert.equal(document.getElementById("pe-supplier").required, false,
  "personal purchase slips must not require a supplier");
assert.equal(document.getElementById("pe-supplier-required").style.display, "none",
  "personal purchase slips must hide the supplier required mark");
assert.equal(window.eval("_peHeaderReady()"), true,
  "personal purchase slips must be ready when date and staff are present without a supplier");
addLineCountInput.value = "1";
window.peAddLine();
const modalTestLineId = window.eval("_peSlipData.lines[0].lineId");
const modalTestShape = window.eval("getProductSpecMasterRecords('shape')[0]?.code || ''");
const modalTestMarking = window.eval("getProductSpecMasterRecords('marking')[0]?.code || ''");
window.eval(`_peSlipData.lines[0].productDetail = {
  brand: "ロレックス",
  model: "モーダル編集前",
  shape: ${JSON.stringify(modalTestShape)},
  marking: ${JSON.stringify(modalTestMarking)},
  accessories: []
};`);
assert.doesNotThrow(() => window.peOpenProductModal(modalTestLineId),
  "the purchase detail registration button must open its editable product modal");
assert.equal(document.getElementById("peProductModal").style.display, "flex");
assert.equal(document.getElementById("pep-model").value, "モーダル編集前");
assert.equal(document.getElementById("pep-shape").value, modalTestShape);
assert.equal(document.getElementById("pep-marking").value, modalTestMarking);
// Draft-row inputs need IME protection independently of product-modal initialization.
{
  const sku = document.querySelector('#pe-detail-tbody .pe-sku-input');
  const count = document.getElementById('pe-line-add-count');
  const quantity = document.getElementById('pep-bracelet-qty');
  const row = window.eval('_peSlipData.lines[0]');
  const originalSku = row.sku;
  for (const field of [sku, count, quantity]) {
    const original = field.value;
    field.value = '';
    field.dispatchEvent(new window.InputEvent('input', {bubbles:true}));
    assert.equal(field.value, '', 'clearing a quantity must not insert 1 while editing');
    field.dispatchEvent(new window.CompositionEvent('compositionstart', {bubbles:true}));
    field.value = '１２';
    field.dispatchEvent(new window.InputEvent('input', {bubbles:true,isComposing:true}));
    if (field.type !== 'number') assert.equal(field.value, '１２', 'do not rewrite composing digits');
    const event = new window.KeyboardEvent('keydown', {key:'Enter',isComposing:true,keyCode:229,bubbles:true,cancelable:true});
    field.dispatchEvent(event);
    assert.equal(event.defaultPrevented, false, 'IME confirmation must not navigate or add lines');
    // Native number inputs normalize committed full-width text before compositionend.
    if (field.type === 'number') field.value = '12';
    field.dispatchEvent(new window.CompositionEvent('compositionend', {bubbles:true,data:'１２'}));
    assert.equal(field.value, '12');
    if (field === sku) assert.equal(row.sku, '12', 'draft must receive committed SKU without trailing input');
    field.value = original;
  }
  row.sku = originalSku;
}
// IME must own its composition range until commit; rewriting it duplicates digits.
for (const id of ['pep-ref', 'pep-serial']) {
  const input = document.getElementById(id);
  const original = input.value;
  input.dispatchEvent(new window.CompositionEvent('compositionstart', { bubbles: true }));
  for (const text of ['１', '１２', '１２３']) {
    input.value = text;
    input.dispatchEvent(new window.InputEvent('input', { bubbles: true, isComposing: true, inputType: 'insertCompositionText', data: text }));
    assert.equal(input.value, text, id + ' must not rewrite active IME digits');
  }
  input.dispatchEvent(new window.CompositionEvent('compositionend', { bubbles: true, data: '１２３' }));
  assert.equal(input.value, '123', id + ' normalizes committed digits once');
  input.dispatchEvent(new window.InputEvent('input', { bubbles: true, inputType: 'insertText', data: '１２３' }));
  assert.equal(input.value, '123', id + ' remains unchanged on trailing input');
  input.value = '11112222';
  input.dispatchEvent(new window.InputEvent('input', { bubbles: true }));
  assert.equal(input.value, '11112222', id + ' preserves intentional repeated digits');
  input.value = original;
}
document.getElementById("pep-model").value = "モーダル編集後";
window.peSaveProduct();
assert.equal(document.getElementById("peProductModal").style.display, "none");
assert.equal(window.eval("_peSlipData.lines[0].productDetail.model"), "モーダル編集後",
  "confirming the product modal must write edits back to the purchase detail line");
window.eval(`
  _peSlipData.lines[0].sku = "SAVE-RESET-TEST";
  _peSlipData.lines[0].purchasePrice = 100000;
  _peSlipData.lines[0].salePrice = 1000;
  _peSlipData.lines[0].productDetail = { brand: "", model: "Reset Test" };
`);
document.querySelector(".pe-sku-input").value = "SAVE-RESET-TEST";

let purchaseSaveCalls = 0;
let savedPurchasePaymentMethod = "";
let savedPersonalSupplierName = "";
let completePurchaseSave;
const pendingPurchaseSave = new Promise((resolve) => { completePurchaseSave = resolve; });
window.ZaikoAPI = {
  savePurchaseSlip: async (slip) => {
    purchaseSaveCalls += 1;
    savedPurchasePaymentMethod = slip.paymentMethod;
    savedPersonalSupplierName = slip.supplierName;
    return pendingPurchaseSave;
  },
};
const firstPurchaseSave = window.peSave();
const duplicatePurchaseSave = window.peSave();
assert.equal(purchaseSaveCalls, 1, "a second click while saving must not submit another purchase slip");
assert.equal(savedPurchasePaymentMethod, "card", "the selected payment method must be included in purchase registration");
assert.equal(savedPersonalSupplierName, "個人買取テスト 太郎",
  "the transaction-only supplier name must be included in personal purchase registration");
assert.equal(window.eval("_peSlipData.lines[0].productDetail.brand"), "",
  "purchase registration must reach the API even when the brand is not known yet");
assert.equal(purchaseSaveButton.disabled, true, "the purchase save button must be disabled while saving");
assert.equal(purchaseSaveButton.getAttribute("aria-busy"), "true");
assert.match(purchaseSaveButton.textContent, /登録中/u);
completePurchaseSave({ record: { slipNumber: "PI-2099-RESET" }, approval: null });
await Promise.all([firstPurchaseSave, duplicatePurchaseSave]);
assert.equal(purchaseSaveButton.disabled, true,
  "after reset, purchase registration must remain disabled until the required slip header is selected again");
assert.equal(purchaseSaveButton.hasAttribute("aria-busy"), false);
assert.match(purchaseSaveButton.textContent, /仕入登録する/u);
assert.equal(document.getElementById("pe-date").value, "", "successful purchase registration must clear the purchase date");
assert.equal(document.getElementById("pe-supplier").value, "", "successful purchase registration must clear the supplier");
assert.equal(document.getElementById("pe-personal-supplier-name").value, "",
  "successful purchase registration must clear the transaction-only supplier name");
assert.equal(document.getElementById("pe-staff").value, "", "successful purchase registration must clear the staff member");
assert.equal(document.querySelectorAll("#pe-detail-tbody tr").length, 0, "successful purchase registration must clear every detail row");
assert.equal(document.getElementById("pe-line-count").textContent, "0");
assert.equal(document.getElementById("pe-subtotal-purchase").textContent, "¥0");
assert.equal(document.getElementById("pe-total-purchase").textContent, "¥0");
assert.equal(document.getElementById("pe-total-sale").textContent, "$0");
assert.equal(addLineCountInput.value, "1");
assert.equal(document.getElementById("pe-tax-domestic").getAttribute("aria-checked"), "true", "a new purchase form must restore domestic tax mode");
assert.equal(document.getElementById("pe-payment-bank-transfer").getAttribute("aria-checked"), "true", "a new purchase form must restore bank transfer");
delete window.ZaikoAPI;

addLineCountInput.value = "１２a";
window.peNormalizeAddCountInput(addLineCountInput);
assert.equal(addLineCountInput.value, "12", "bulk line count normalizes committed full-width digits and removes non-digits");

document.getElementById("pe-date").value = "2099-12-31";
window.peOnDateChange();
document.getElementById("pe-supplier").value = document.getElementById("pe-supplier").options[1].value;
document.getElementById("pe-staff").value = document.getElementById("pe-staff").options[1].value;
window.peOnHeaderChange();
addLineCountInput.value = "10";
window.peAddLine();
assert.equal(document.querySelectorAll("#pe-detail-tbody tr").length, 10, "one click must add the requested ten lines");
assert.equal(document.getElementById("pe-line-count").textContent, "10");
assert.deepEqual(
  [...document.querySelectorAll('[data-role="pe-line-no"]')].map((cell) => cell.textContent.trim()),
  Array.from({ length: 10 }, (_, index) => String(index + 1)),
  "bulk-added detail numbers must be sequential",
);
assert.deepEqual(
  [...document.querySelectorAll('[data-role="pe-item-code"]')].map((cell) => cell.textContent.trim()),
  Array.from({ length: 10 }, (_, index) => `311299${String(index + 1).padStart(4, "0")}`),
  "bulk-added product codes must be sequential",
);

const thirdLineId = window.eval("_peSlipData.lines[2].lineId");
window.peRemoveLine(thirdLineId);
assert.equal(document.querySelectorAll("#pe-detail-tbody tr").length, 9, "deleting the third line must leave nine lines");
assert.deepEqual(
  [...document.querySelectorAll('[data-role="pe-line-no"]')].map((cell) => cell.textContent.trim()),
  Array.from({ length: 9 }, (_, index) => String(index + 1)),
  "detail numbers must close the deleted gap",
);
assert.deepEqual(
  [...document.querySelectorAll('[data-role="pe-item-code"]')].map((cell) => cell.textContent.trim()),
  Array.from({ length: 9 }, (_, index) => `311299${String(index + 1).padStart(4, "0")}`),
  "product codes must be reissued sequentially after a middle row is deleted",
);

const csvDownloadNames = [];
const originalPurchaseCSVAnchorClick = window.HTMLAnchorElement.prototype.click;
window.HTMLAnchorElement.prototype.click = function capturePurchaseCSVDownload() {
  csvDownloadNames.push(this.download);
};
const purchaseCSVTemplate = window.peDownloadCSVTemplate();
const purchaseTemplateRows = purchaseCSVTemplate.rows;
assert.equal(csvDownloadNames.at(-1), "仕入伝票CSVテンプレート.csv");
assert.deepEqual(
  [...purchaseTemplateRows[0]],
  [
    "マーキングコード", "売価", "形状コード", "SKU", "原価",
    "ブランドコード", "モデル", "型番", "シリアル", "素材コード",
    "駆動方式コード", "ベルト素材コード", "特徴・備考",
  ],
  "the purchase template must contain only user-entered fields and stable master codes",
);
assert.equal(purchaseTemplateRows.length, 1, "the purchase template must contain only the header row");
for (const removedColumn of ["仕入伝票番号", "伝票備考", "明細No.", "管理番号", "ブランド名", "コンディションコード", "コンディション名（参照用）", "BOX番号", "付属品", "BRACELET PARTS数量"]) {
  assert.equal(purchaseTemplateRows[0].includes(removedColumn), false, `${removedColumn} must not remain in the purchase CSV template`);
}
const marketCSVTemplate = window.marketDownloadTemplate();
assert.equal(csvDownloadNames.at(-1), "相場表テンプレート.csv");
assert.equal(marketCSVTemplate.rows.length, 2, "the market template must include one editable sample row");
assert.equal(marketCSVTemplate.rows[1].length, marketCSVTemplate.rows[0].length);
assert.deepEqual([...marketCSVTemplate.rows[0]], [
  "ブランドコード", "型番", "モデル", "素材コード", "駆動方式コード", "コンディションコード",
  "保証年月", "取引価格", "SKU", "付属品コード", "コマ数", "備考",
], "the market template must contain detail fields only; shared market data belongs to the basic-information section");
assert.match(marketCSVTemplate.rows[1][6], /^\d{4}-\d{2}$/u, "the market sample must include a warranty year and month");
const marketTemplateCSVText = marketCSVTemplate.rows.map((row) => row.map((value) => `"${String(value ?? "").replaceAll('"', '""')}"`).join(",")).join("\r\n");
const marketCountBeforeTemplateImport = window.eval("APP_DATA.marketPrices.length");
const marketTemplateImportResult = window.marketImportCsvText(marketTemplateCSVText, "相場表テンプレート.csv");
assert.deepEqual({ ...marketTemplateImportResult }, { imported: 0, staged: 1, skipped: 0 }, "the downloaded market sample must stage without saving");
assert.equal(window.eval("APP_DATA.marketPrices.length"), marketCountBeforeTemplateImport, "market sample staging must not change persisted rows");
assert.equal(document.getElementById("marketDraftTableWrap").hidden, false, "market CSV staging must open the inline detail preview");
assert.equal(document.querySelectorAll("#marketDraftTableBody tr").length, 1, "staged CSV rows must appear in the editable detail table");
assert.deepEqual(
  [...document.querySelectorAll("#marketDraftTable thead th")].map(th => th.textContent.replace(/\s+/gu, " ").trim()),
  ["No.", "ブランド *", "型番", "モデル *", "素材", "駆動方式", "コンディション", "保証年月", "取引価格（JPY） *", "SKU", "付属品", "削除"],
  "market draft columns must follow the requested product-detail order",
);
const templateHeader = [...marketCSVTemplate.rows[0]];
const templateSample = [...marketCSVTemplate.rows[1]];
const templateMaterialCode = templateSample[templateHeader.indexOf("素材コード")];
const templateMovementCode = templateSample[templateHeader.indexOf("駆動方式コード")];
const templateConditionCode = templateSample[templateHeader.indexOf("コンディションコード")];
assert.equal(document.querySelector('#marketDraftTableBody select[aria-label="明細1 素材"]').value, templateMaterialCode,
  "CSV material codes must select the matching material master option");
assert.equal(document.querySelector('#marketDraftTableBody select[aria-label="明細1 駆動方式"]').value, templateMovementCode,
  "CSV movement codes must select the matching movement master option");
assert.equal(document.querySelector('#marketDraftTableBody select[aria-label="明細1 コンディション"]').value, templateConditionCode,
  "CSV condition codes must select the matching condition master option");
assert.equal(document.querySelector('#marketDraftTableBody input[aria-label="明細1 備考"]'), null,
  "notes must not remain as a draft-row column");
assert.equal(document.getElementById("marketCsvPreviewModal").classList.contains("hidden"), true, "market CSV staging must remain on the registration page instead of opening a modal");
window.marketCancelCSVImport();
assert.equal(document.getElementById("marketDraftTableWrap").hidden, true);
const purchaseCSVHeaders = [...purchaseTemplateRows[0]];
const supplierForCSV = window.eval("APP_DATA.suppliers[0]");
const staffForCSV = window.eval("APP_DATA.staffRecords[0]");
const materialForCSV = window.eval("APP_DATA.materials[0]");
const movementForCSV = window.eval("APP_DATA.movements[0]");
const beltMaterialForCSV = window.eval("APP_DATA.beltMaterialRecords[0]");
window.eval("APP_DATA.markingRecords = APP_DATA.markingRecords || [{ code: 'MRK-001', name: '♡', meaning: '確認済' }]");
window.eval("APP_DATA.shapeRecords = APP_DATA.shapeRecords || [{ code: 'TYP-001', name: '腕時計' }]");
const markingForCSV = window.eval("APP_DATA.markingRecords[0]");
const shapeForCSV = window.eval("APP_DATA.shapeRecords[0]");
const bulkProductNote = "CSV一括登録,改行付き\n備考";
const purchaseCSVValues = {
  "マーキングコード": markingForCSV.code,
  "形状コード": shapeForCSV.code,
  SKU: "CSV-BULK-001",
  "原価": "1,234,567",
  "売価": "9,876",
  "ブランドコード": "",
  "モデル": "CSV Bulk Model",
  "型番": "CSV-REF-001",
  "シリアル": "CSV-SERIAL-001",
  "素材コード": materialForCSV.code,
  "駆動方式コード": movementForCSV.code,
  "ベルト素材コード": beltMaterialForCSV.code,
  "特徴・備考": bulkProductNote,
};
const escapeCSVCell = (value) => `"${String(value ?? "").replaceAll('"', '""')}"`;
const secondPurchaseCSVValues = { ...purchaseCSVValues, SKU: "CSV-BULK-002", "シリアル": "CSV-SERIAL-002", "特徴・備考": "2行目" };
const purchaseCSVText = [purchaseCSVHeaders, purchaseCSVHeaders.map((header) => purchaseCSVValues[header] || ""), purchaseCSVHeaders.map((header) => secondPurchaseCSVValues[header] || "")]
  .map((row) => row.map(escapeCSVCell).join(","))
  .join("\r\n");
const purchaseCountBeforeBulkCSV = window.eval("APP_DATA.purchaseSlips.length");
const inventoryCountBeforeBulkCSV = window.eval("APP_DATA.inventory.length");
document.getElementById("pe-date").value = "2099-12-29";
document.getElementById("pe-supplier").value = supplierForCSV.code;
document.getElementById("pe-staff").value = staffForCSV.name;
const bulkImportSummary = await window.peImportCSVText(purchaseCSVText);
assert.deepEqual({ ...bulkImportSummary }, { staged: 2, slipCount: 0, productCount: 0, approvalCount: 0 });
assert.equal(window.eval("APP_DATA.purchaseSlips.length"), purchaseCountBeforeBulkCSV, "CSV staging must not create the purchase slip");
assert.equal(window.eval("APP_DATA.inventory.length"), inventoryCountBeforeBulkCSV, "CSV staging must not add products to inventory");
const bulkPurchase = window.eval("_peSlipData");
assert.ok(bulkPurchase, "the CSV purchase must be loaded into the editable draft");
assert.equal(document.querySelectorAll("#pe-detail-tbody tr").length, 2, "all CSV rows must be visible in the purchase detail table");
assert.equal(document.getElementById("pe-date").value, "2099-12-29");
assert.equal(document.getElementById("pe-supplier").value, supplierForCSV.code);
assert.match(bulkPurchase.id, /^SD-291299[0-9]{3}$/u, "the purchase slip number must be generated without a CSV column");
assert.deepEqual([...bulkPurchase.lines.map((line) => line.lineNo)], [1, 2], "detail numbers must follow CSV row order automatically");
assert.deepEqual([...bulkPurchase.lines.map((line) => line.code)], ["2912990001", "2912990002"], "management numbers must be generated sequentially without a CSV column");
assert.equal(bulkPurchase.lines[0].productDetail.brand, "", "CSV purchase registration must allow an unknown brand");
assert.equal(bulkPurchase.lines[0].productDetail.model, "CSV Bulk Model");
assert.equal(bulkPurchase.lines[0].productDetail.material, materialForCSV.code);
assert.equal(bulkPurchase.lines[0].productDetail.movement, movementForCSV.code);
assert.equal(bulkPurchase.lines[0].productDetail.marking, markingForCSV.code);
assert.equal(bulkPurchase.lines[0].productDetail.shape, shapeForCSV.code);
assert.equal(bulkPurchase.lines[0].productDetail.condition, "", "condition is intentionally outside the purchase CSV");
assert.equal(bulkPurchase.lines[0].productDetail.belt, beltMaterialForCSV.name, "belt material code must resolve through the master");
assert.equal(bulkPurchase.lines[0].productDetail.dial, "", "dial must not be imported from the purchase CSV");
assert.deepEqual([...bulkPurchase.lines[0].productDetail.accessories], [], "accessories are intentionally outside the purchase CSV");
assert.equal(bulkPurchase.lines[0].productDetail.note, bulkProductNote, "quoted multiline notes must survive CSV import");
assert.equal(bulkPurchase.lines[0].purchasePrice, 1234567);
assert.equal(bulkPurchase.lines[0].salePrice, 9876);
const invalidPurchaseCSVValues = { ...purchaseCSVValues, "マーキングコード": "MRK-999", "形状コード": "TYP-999" };
const invalidPurchaseCSVText = [purchaseCSVHeaders, purchaseCSVHeaders.map((header) => invalidPurchaseCSVValues[header] || "")]
  .map((row) => row.map(escapeCSVCell).join(","))
  .join("\r\n");
let purchaseCSVError;
try { await window.peImportCSVText(invalidPurchaseCSVText); } catch (error) { purchaseCSVError = error; }
assert.ok(purchaseCSVError, "unknown purchase master codes must reject the CSV");
assert.deepEqual([...purchaseCSVError.csvErrors.map((item) => item.cell)], ["A2", "C2"]);
window._peShowCSVImportErrors(purchaseCSVError);
assert.match(document.getElementById("pe-csv-error-toast").textContent, /CSV取込エラー 2件/u);
assert.match(document.getElementById("pe-csv-error-toast").textContent, /例 A2、C2/u);
window.peShowCSVErrorExample();
assert.match(document.getElementById("pe-csv-error-toast").textContent, /表示例（未取込）/u);
assert.match(document.getElementById("pe-csv-error-toast").textContent, /例 A2、C3/u);
window.eval("_peInitNewSlip()");
window.HTMLAnchorElement.prototype.click = originalPurchaseCSVAnchorClick;

window.navigateTo("master");
window.switchMasterTab("company");
window.toggleMCompanyEdit();
document.getElementById("mComp-name").value = "統一会社情報テスト株式会社";
document.getElementById("mComp-zip").value = "〒100-0001";
document.getElementById("mComp-address").value = "東京都統一区1-2-3";
document.getElementById("mComp-tel").value = "03-9999-1111";
document.getElementById("mComp-fax").value = "03-9999-2222";
document.getElementById("mComp-email").value = "unified@example.jp";
document.getElementById("mComp-invoice").value = "T9999999999999";
window.saveMCompanyInfo();
window.toggleMBankEdit();
document.getElementById("mBank-name").value = "統一銀行";
document.getElementById("mBank-branch").value = "統一支店";
document.getElementById("mBank-type").value = "当座";
document.getElementById("mBank-no").value = "7654321";
document.getElementById("mBank-holder").value = "カ）トウイツカイシャ";
window.saveMBankInfo();

const companyInfo = window.eval("APP_DATA.companyInfo");
assert.equal(companyInfo.companyName, "統一会社情報テスト株式会社");
assert.equal(companyInfo.accountNumber, "7654321");
assert.equal(companyInfo.accountHolder, "カ）トウイツカイシャ");
assert.equal(companyInfo.accountNo, undefined, "legacy accountNo must not remain in the master");
assert.equal(companyInfo.accountName, undefined, "legacy accountName must not remain in the master");
assert.match(document.getElementById("mCompanyViewArea").textContent, /T9999999999999/u);
assert.match(document.getElementById("mBankViewArea").textContent, /7654321/u);

const savedShippingTemplateHtml = window.buildShipmentRecordTemplateHTML(window.eval("APP_DATA.shipments[0]"));
assert.match(savedShippingTemplateHtml, /出荷伝票/u, "saved shipping slips must use the template layout");
assert.equal(savedShippingTemplateHtml.includes("お振込先"), false);
const savedConsignment = {
  id: "CO-TEMPLATE-0001",
  date: "2099-11-30",
  destination: window.eval("APP_DATA.shipments[0].destination"),
  status: "処理済",
  note: "委託帳票テスト",
  items: window.eval("APP_DATA.shipments[0].items.slice(0, 1)"),
};
window.eval("APP_DATA.consignments").push(savedConsignment);
const savedConsignmentTemplateHtml = window.buildConsignmentRecordTemplateHTML(savedConsignment);
assert.match(savedConsignmentTemplateHtml, /委託伝票/u, "saved consignment slips must use the shipping template layout");
assert.match(savedConsignmentTemplateHtml, /委託日/u);
assert.match(savedConsignmentTemplateHtml, /委託先/u);
assert.match(savedConsignmentTemplateHtml, /明細表/u);
assert.equal(savedConsignmentTemplateHtml.includes("お振込先"), false);
const savedPurchase = window.eval("APP_DATA.purchaseSlips[0]");
const savedPurchaseTemplateHtml = window.buildPurchaseRecordTemplateHTML(savedPurchase);
assert.match(savedPurchaseTemplateHtml, /仕入伝票/u, "saved purchase slips must use the template layout");
assert.match(savedPurchaseTemplateHtml, /明細表/u);
assert.equal(savedPurchaseTemplateHtml.includes("SKU:"), false, "saved purchase slips must not display SKU");
assert.match(savedPurchaseTemplateHtml, /BOX・GUARANTEE/u, "saved purchase slips must display accessories in the description");
assert.match(savedPurchaseTemplateHtml, /消費税（10%）/u, "saved domestic purchase slips must reproduce the 10% tax category");
assert.match(savedPurchaseTemplateHtml, /仕入通貨：日本円（JPY）/u,
  "domestic purchase slips must identify JPY in the remarks section");
assert.match(savedPurchaseTemplateHtml, /仕入レート（登録時固定）：1 JPY = ¥1\.00/u,
  "domestic purchase slips must display the fixed JPY rate in the remarks section");
const usdPurchaseTemplateHtml = window.buildPurchaseRecordTemplateHTML({
  ...savedPurchase,
  purchaseTaxMode: "overseas",
  purchaseCurrency: "USD",
  registrationPurchaseJpyRate: 155.25,
  lines: savedPurchase.lines.map((line) => ({ ...line, purchaseCurrency: "USD", purchaseFxRateScaled: 15525, purchaseFxScale: 100 })),
});
assert.match(usdPurchaseTemplateHtml, /仕入通貨：米ドル（USD）/u);
assert.match(usdPurchaseTemplateHtml, /仕入レート（登録時固定）：1 USD = ¥155\.25/u);
const hkdPurchaseTemplateHtml = window.buildPurchaseRecordTemplateHTML({
  ...savedPurchase,
  purchaseTaxMode: "overseas",
  purchaseCurrency: "HKD",
  registrationPurchaseJpyRate: 19.8,
  lines: savedPurchase.lines.map((line) => ({ ...line, purchaseCurrency: "HKD", purchaseFxRateScaled: 1980, purchaseFxScale: 100 })),
});
assert.match(hkdPurchaseTemplateHtml, /仕入通貨：香港ドル（HKD）/u);
assert.match(hkdPurchaseTemplateHtml, /仕入レート（登録時固定）：1 HKD = ¥19\.80/u);
assert.equal(savedPurchaseTemplateHtml.includes("お振込先"), false);
assert.match(savedPurchaseTemplateHtml, new RegExp(companyInfo.companyName));
const currentPurchaseLine = savedPurchase.lines.find((line) => inventory.some((item) => item.code === line.code));
assert.ok(currentPurchaseLine, "a saved purchase line must stay linked to its inventory product by management number");
const currentPurchaseProduct = inventory.find((item) => item.code === currentPurchaseLine.code);
const originalPurchaseProductDetails = {
  brand: currentPurchaseProduct.brand,
  model: currentPurchaseProduct.model,
  ref: currentPurchaseProduct.ref,
  serial: currentPurchaseProduct.serial,
  accessories: [...(currentPurchaseProduct.accessories || [])],
};
Object.assign(currentPurchaseProduct, {
  brand: "更新後ブランド",
  model: "更新後モデル",
  ref: "UPDATED-REF",
  serial: "UPDATED-SERIAL",
  accessories: ["CASE"],
});
const updatedPurchaseTemplateHtml = window.buildPurchaseRecordTemplateHTML(savedPurchase);
const updatedPurchaseDetailHtml = window.buildSlipDetailBody("purchase", savedPurchase);
assert.match(updatedPurchaseTemplateHtml, /更新後ブランド[\s\S]*更新後モデル/u,
  "purchase print previews must show the latest linked product brand and model");
assert.match(updatedPurchaseTemplateHtml, /UPDATED-REF/u);
assert.match(updatedPurchaseTemplateHtml, /CASE/u);
assert.match(updatedPurchaseDetailHtml, /更新後ブランド/u,
  "purchase detail popups must show later product edits without rewriting the historical snapshot");
Object.assign(currentPurchaseProduct, originalPurchaseProductDetails);
const savedSalesTemplateHtml = window.buildSalesRecordTemplateHTML(window.eval("APP_DATA.sales[0]"));
assert.match(savedSalesTemplateHtml, /請求書/u, "saved sales slips must use the invoice template layout");
assert.match(savedSalesTemplateHtml, /お振込先/u);
assert.match(savedSalesTemplateHtml, new RegExp(companyInfo.bankName));
assert.match(savedSalesTemplateHtml, new RegExp(companyInfo.accountNumber));
assert.match(savedSalesTemplateHtml, new RegExp(companyInfo.accountHolder));

assert.ok(document.querySelector('#savedSlipDocumentModal button[onclick="downloadSavedSlipDocument()"]'));
assert.ok(document.querySelector('#pePrintModal button[onclick="peDownloadDocument()"]'));
assert.ok(document.querySelector('#salesPrintModal button[onclick="downloadCurrentSalesDocument()"]'));
assert.ok(document.querySelector('#shipPrintModal button[onclick="downloadCurrentShipmentDocument()"]'));
const savedSlipDownloadNames = [];
const originalSavedSlipCreateObjectURL = window.URL.createObjectURL;
const originalSavedSlipRevokeObjectURL = window.URL.revokeObjectURL;
const originalSavedSlipAnchorClick = window.HTMLAnchorElement.prototype.click;
window.URL.createObjectURL = blob => { assert.equal(blob.type, 'application/pdf'); return 'blob:saved-slip-test'; };
window.URL.revokeObjectURL = () => {};
window.HTMLAnchorElement.prototype.click = function captureSavedSlipDownload() {
  savedSlipDownloadNames.push(this.download);
};
const savedSlipCases = [
  { type: "purchase", record: savedPurchase, title: /仕入伝票プレビュー/u, filename: `${savedPurchase.id}_仕入伝票.pdf` },
  { type: "shipping", record: window.eval("APP_DATA.shipments[0]"), title: /出荷伝票プレビュー/u, filename: `${window.eval("APP_DATA.shipments[0].id")}_出荷伝票.pdf` },
  { type: "consignment", record: savedConsignment, title: /委託伝票プレビュー/u, filename: `${savedConsignment.id}_委託伝票.pdf` },
  { type: "sales", record: window.eval("APP_DATA.sales[0]"), title: /請求書（売上伝票）プレビュー/u, filename: `${window.eval("APP_DATA.sales[0].id")}_請求書.pdf` },
];
for (const savedSlipCase of savedSlipCases) {
  assert.match(
    window.buildSlipDetailFooter(savedSlipCase.type, savedSlipCase.record),
    new RegExp(`openSavedSlipDocument\\('${savedSlipCase.type}'`),
    "each saved slip detail popup must expose its template preview",
  );
  window.openSavedSlipDocument(savedSlipCase.type, savedSlipCase.record.id);
  assert.match(document.getElementById("savedSlipDocumentTitle").textContent, savedSlipCase.title);
  assert.match(document.getElementById("savedSlipDocumentPrintArea").textContent, /明細表/u);
  await window.downloadSavedSlipDocument();
  assert.equal(savedSlipDownloadNames.at(-1), savedSlipCase.filename);
  window.closeSavedSlipDocument();
}
window.HTMLAnchorElement.prototype.click = originalSavedSlipAnchorClick;
window.URL.createObjectURL = originalSavedSlipCreateObjectURL;
window.URL.revokeObjectURL = originalSavedSlipRevokeObjectURL;

window.eval(`
  APP_DATA.purchaseSlips.push({
    id: "PI-TEMPLATE-RETURN-TEST",
    date: "2099-12-01",
    supplier: "S001",
    lines: [{
      code: "PR-TEMPLATE-CODE",
      sku: "PR-TEMPLATE-SKU",
      purchasePrice: 456789,
      productDetail: {
        brand: "仕入返品テストブランド",
        model: "仕入返品テストモデル",
        ref: "PR-REF-001",
        serial: "PR-SERIAL-001",
        accessories: ["BOX", "GUARANTEE"]
      }
    }]
  });
  APP_DATA.purchaseReturns.push({
    id: "PR-TEMPLATE-RETURN-TEST",
    slipId: "PI-TEMPLATE-RETURN-TEST",
    supplier: "S001",
    date: "2099-12-15",
    reason: "帳票テスト返品",
    note: "元仕入価格を使用",
    items: [{ code: "PR-TEMPLATE-CODE", purchasePrice: 1 }],
    total: 1,
    status: "未処理",
    createdBy: "帳票テスト担当者",
    createdAt: "2099-12-15 10:00",
    invoicePrinted: false
  });
`);
const purchaseReturn = window.eval('APP_DATA.purchaseReturns.find(record => record.id === "PR-TEMPLATE-RETURN-TEST")');
const originalPurchaseAmount = window.getPurchaseReturnOriginalAmountInfo(purchaseReturn);
assert.equal(originalPurchaseAmount.subtotal, 456789, "purchase return totals must use the original purchase price, not the return record copy");
window.eval(`_currentPrRetId = ${JSON.stringify(purchaseReturn.id)}`);
window.openPurchaseReturnInvoice();
const purchaseReturnInvoiceHtml = document.getElementById("prInvoicePrintArea").innerHTML;
assert.match(purchaseReturnInvoiceHtml, new RegExp(companyInfo.companyName));
assert.match(purchaseReturnInvoiceHtml, new RegExp(companyInfo.invoice));
assert.match(purchaseReturnInvoiceHtml, /仕入返品伝票/u);
assert.match(purchaseReturnInvoiceHtml, /明細表/u, "purchase return preview must include the template detail sheet");
assert.match(purchaseReturnInvoiceHtml, /仕入金額合計/u);
assert.match(purchaseReturnInvoiceHtml, /¥456,789/u, "purchase return preview must show the original purchase amount");
assert.equal(purchaseReturnInvoiceHtml.includes("返品日"), false, "purchase return documents must omit the return date");
assert.match(purchaseReturnInvoiceHtml, /消費税（10%）/u, "purchase return detail sheets must show the 10% consumption-tax column");
assert.match(purchaseReturnInvoiceHtml, /¥45,678/u, "purchase return detail sheets must calculate 10% consumption tax per item");
assert.match(purchaseReturnInvoiceHtml, /¥502,467/u, "purchase return cover totals must include consumption tax");
assert.equal(purchaseReturnInvoiceHtml.includes("お振込先"), false, "purchase return slips must not print bank details");
assert.equal(purchaseReturnInvoiceHtml.includes("SKU:"), false, "purchase return details must not display SKU");
assert.equal(purchaseReturnInvoiceHtml.includes("元仕入価格を使用"), false, "purchase return documents must not display line or slip notes");
assert.equal(purchaseReturnInvoiceHtml.includes("帳票テスト返品"), false, "purchase return documents must not display return reasons as notes");
assert.equal(purchaseReturnInvoiceHtml.includes("<strong>備考</strong>"), false, "purchase return documents must omit the note section");
assert.match(purchaseReturnInvoiceHtml, /BOX・GUARANTEE/u, "purchase return details must retain accessory information");
assert.match(document.getElementById("prInvoiceModal").textContent, /ダウンロード/u);
assert.ok(document.querySelector('#prInvoiceModal button[onclick="downloadPurchaseReturnDocument()"]'));
assert.match(document.getElementById("sltab-purchasereturn").textContent, /仕入返品伝票/u);
let downloadedPurchaseReturnName = "";
const originalPurchaseReturnCreateObjectURL = window.URL.createObjectURL;
const originalPurchaseReturnRevokeObjectURL = window.URL.revokeObjectURL;
const originalPurchaseReturnAnchorClick = window.HTMLAnchorElement.prototype.click;
window.URL.createObjectURL = () => "blob:purchase-return-test";
window.URL.revokeObjectURL = () => {};
window.HTMLAnchorElement.prototype.click = function capturePurchaseReturnDownload() {
  downloadedPurchaseReturnName = this.download;
};
await window.downloadPurchaseReturnDocument();
assert.equal(downloadedPurchaseReturnName, `${purchaseReturn.id}_仕入返品伝票.pdf`);
window.HTMLAnchorElement.prototype.click = originalPurchaseReturnAnchorClick;
window.URL.createObjectURL = originalPurchaseReturnCreateObjectURL;
window.URL.revokeObjectURL = originalPurchaseReturnRevokeObjectURL;
window.closePurchaseReturnInvoice();
window.eval(`
  APP_DATA.purchaseReturns = APP_DATA.purchaseReturns.filter(record => record.id !== "PR-TEMPLATE-RETURN-TEST");
  APP_DATA.purchaseSlips = APP_DATA.purchaseSlips.filter(record => record.id !== "PI-TEMPLATE-RETURN-TEST");
`);

const salesReturn = window.eval("APP_DATA.salesReturns[0]");
const sourceSale = window.eval("APP_DATA.sales").find((sale) => sale.id === salesReturn.slipId);
const sourceSaleLine = sourceSale?.items?.find((line) => line.code === salesReturn.items[0]?.code);
assert.ok(sourceSaleLine, "sales return must resolve its original sales line by product code");
const savedReturnTotal = salesReturn.total;
const savedReturnItemPrice = salesReturn.items[0].salePrice;
salesReturn.total = 1;
salesReturn.items[0].salePrice = 2;
const originalSaleAmount = window.getSalesReturnOriginalAmountInfo(salesReturn);
assert.equal(originalSaleAmount.subtotal, sourceSaleLine.salePrice, "sales return totals must use the original sale price, not the return record copy");
assert.equal(
  originalSaleAmount.grandTotal,
  sourceSaleLine.salePrice + Math.floor(sourceSaleLine.salePrice * 0.1),
  "taxable sales returns must reproduce the original tax-inclusive sales total",
);
window.openSalesReturnDetail(salesReturn.id);
assert.equal(document.getElementById("salesReturnDetailModal").classList.contains("hidden"), false,
  "sales return detail must open from the document list");
assert.match(document.getElementById("srDetailModalTitle").textContent, new RegExp(salesReturn.id));
window.closeSalesReturnDetailModal();
window.eval(`_currentSrRetId = ${JSON.stringify(salesReturn.id)}`);
window.openSalesReturnInvoice();
const salesReturnInvoiceHtml = document.getElementById("srInvoicePrintArea").innerHTML;
assert.match(salesReturnInvoiceHtml, new RegExp(companyInfo.companyName));
assert.match(salesReturnInvoiceHtml, new RegExp(companyInfo.invoice));
assert.match(salesReturnInvoiceHtml, /売上返品伝票/u);
assert.match(salesReturnInvoiceHtml, /明細表/u, "sales return preview must include the template detail sheet");
assert.match(salesReturnInvoiceHtml, /販売時合計金額/u);
assert.match(salesReturnInvoiceHtml, new RegExp(originalSaleAmount.formatAmount(originalSaleAmount.grandTotal).replace(/[.*+?^${}()|[\]\\]/g, "\\$&")));
assert.equal(salesReturnInvoiceHtml.includes("お振込先"), false);
assert.match(document.getElementById("srInvoiceModal").textContent, /ダウンロード/u);
assert.ok(document.querySelector('#srInvoiceModal button[onclick="downloadSalesReturnDocument()"]'));
assert.match(document.getElementById("sltab-salesreturn").textContent, /売上返品伝票/u);
let downloadedSalesReturnName = "";
const originalCreateObjectURL = window.URL.createObjectURL;
const originalRevokeObjectURL = window.URL.revokeObjectURL;
const originalSalesReturnAnchorClick = window.HTMLAnchorElement.prototype.click;
window.URL.createObjectURL = () => "blob:sales-return-test";
window.URL.revokeObjectURL = () => {};
window.HTMLAnchorElement.prototype.click = function captureSalesReturnDownload() {
  downloadedSalesReturnName = this.download;
};
await window.downloadSalesReturnDocument();
assert.equal(downloadedSalesReturnName, `${salesReturn.id}_売上返品伝票.pdf`);
window.HTMLAnchorElement.prototype.click = originalSalesReturnAnchorClick;
window.URL.createObjectURL = originalCreateObjectURL;
window.URL.revokeObjectURL = originalRevokeObjectURL;
window.closeSalesReturnInvoice();
salesReturn.total = savedReturnTotal;
salesReturn.items[0].salePrice = savedReturnItemPrice;
document.getElementById("pe-date").value = "2099-12-31";
document.getElementById("pe-supplier").value = document.getElementById("pe-supplier").options[1].value;
document.getElementById("pe-staff").value = document.getElementById("pe-staff").options[1].value;
window.peOnHeaderChange();
document.getElementById("pe-line-add-count").value = "1";
window.peAddLine();
window.eval(`
  _peSlipData.lines[0].sku = "TPL-PURCHASE";
  _peSlipData.lines[0].purchasePrice = 123456;
  _peSlipData.lines[0].productDetail = {
    brand: "帳票テストブランド",
    model: "帳票テストモデル",
    ref: "TPL-001",
    serial: "SERIAL-001",
    accessories: ["BOX", "GUARANTEE"]
  };
`);
document.getElementById("pe-supplier").value = "S001";
window.peSetPurchaseTaxMode("domestic");
window.eval("_peUpdateDetailUI()");
assert.match(document.querySelector('[data-role="pe-line-tax-label"]').textContent, /消費税（10%）/u);
assert.equal(document.querySelector('[data-role="pe-line-tax-amount"]').textContent.trim(), "¥12,345");
assert.equal(document.getElementById("pe-total-tax").textContent, "¥12,345");
assert.equal(document.getElementById("pe-total-purchase").textContent, "¥135,801");
window.peSetTaxCategory("tax_equivalent");
assert.equal(document.getElementById("pe-tax-category-equivalent").getAttribute("aria-checked"), "true");
assert.match(document.querySelector('[data-role="pe-line-tax-label"]').textContent, /消費税相当額/u);
assert.equal(document.querySelector('[data-role="pe-line-tax-amount"]').textContent.trim(), "¥12,345");
assert.equal(document.getElementById("pe-total-tax").textContent, "¥12,345（参考）");
assert.equal(document.getElementById("pe-total-purchase").textContent, "¥123,456",
  "tax-equivalent reference values must not be added to the payment total");
window.peSetTaxCategory("out_of_scope");
assert.equal(document.getElementById("pe-tax-category-out-of-scope").getAttribute("aria-checked"), "true");
assert.equal(document.querySelector('[data-role="pe-line-tax-label"]').textContent.trim(), "TAX FREE");
assert.equal(document.getElementById("pe-total-tax").textContent, "TAX FREE");
window.peSetTaxCategory("consumption_tax");
window.pePrintPreview();
const purchasePrintHtml = document.getElementById("pePrintPreviewContent").innerHTML;
assert.match(purchasePrintHtml, /仕入伝票/u);
assert.match(purchasePrintHtml, /明細表/u);
assert.match(purchasePrintHtml, /¥123,456/u, "purchase slips must print the purchase amount");
assert.match(purchasePrintHtml, /¥12,345/u, "domestic purchase slips must print each line's 10% consumption tax");
assert.match(purchasePrintHtml, /¥135,801/u, "domestic purchase slips must print the tax-inclusive total");
assert.match(purchasePrintHtml, /商品点数：1点/u, "purchase slips must display the number of products");
assert.match(purchasePrintHtml, /\.tpl-cover-total span\{[^}]*white-space:nowrap/u, "purchase total labels must remain on one line");
assert.equal(purchasePrintHtml.includes("SKU:"), false, "purchase slip previews must not display SKU");
assert.match(purchasePrintHtml, /BOX・GUARANTEE/u, "purchase slip previews must display accessories in the description");
assert.match(purchasePrintHtml, /支払い方法：銀行振込/u, "purchase slips must print the selected payment method");
assert.match(purchasePrintHtml, new RegExp(companyInfo.companyName));
assert.match(purchasePrintHtml, new RegExp(companyInfo.address));
assert.equal(purchasePrintHtml.includes("お振込先"), false, "purchase slips must not print bank details");
window.peclosePrintModal();
window.peSetPurchaseTaxMode("personal");
assert.equal(document.getElementById("pe-tax-personal").getAttribute("aria-checked"), "true");
assert.equal(document.querySelector('[data-role="pe-line-tax-label"]').textContent.trim(), "TAX FREE");
assert.equal(document.getElementById("pe-total-purchase").textContent, "¥123,456");
assert.match(document.getElementById("pe-tax-mode-description").textContent, /個人買取仕入/u);
window.peSetPurchaseCurrency("HKD");
assert.equal(document.getElementById("pe-currency-hkd").getAttribute("aria-checked"), "true");
assert.equal(document.getElementById("pe-total-purchase").textContent, "HK$123,456");
assert.equal(document.getElementById("pe-currency-hkd").getAttribute("aria-checked"), "true",
  "changing the purchase category must not reset the selected purchase currency");
window.peSetPurchaseTaxMode("overseas");
window.peSetPurchaseCurrency("USD");
assert.equal(document.getElementById("pe-tax-overseas").getAttribute("aria-checked"), "true");
assert.equal(document.querySelector('[data-role="pe-line-tax-label"]').textContent.trim(), "TAX FREE");
assert.equal(document.getElementById("pe-total-tax").textContent.trim(), "TAX FREE");
assert.equal(document.getElementById("pe-total-purchase").textContent, "$123,456");
assert.match(document.getElementById("pe-purchase-price-heading").textContent, /USD/u);
window.pePrintPreview();
const overseasPurchasePrintHtml = document.getElementById("pePrintPreviewContent").innerHTML;
assert.match(overseasPurchasePrintHtml, /TAX FREE/u, "overseas purchase slip tax cells must be marked out of scope");
assert.match(overseasPurchasePrintHtml, /\$123,456/u, "overseas purchase slips must print the original USD amount");
assert.match(overseasPurchasePrintHtml, /USD（米ドル）/u, "overseas purchase slips must identify USD as the display currency");
assert.match(overseasPurchasePrintHtml, /発行日時/u, "purchase slips must label the issuance timestamp explicitly");
assert.equal(overseasPurchasePrintHtml.includes("¥135,801"), false, "overseas purchases must not add domestic consumption tax");
window.peclosePrintModal();
window.peSetPurchaseTaxMode("domestic");
window.peSetPurchaseCurrency("JPY");

const domesticPurchaseSummarySlip = {
  id: "PI-SUMMARY-JPY",
  date: "2026-08-15",
  supplier: "S001",
  staff: "管理者",
  purchaseTaxMode: "domestic",
  taxRateBasisPoints: 1000,
  status: "処理済",
  lines: [{ purchasePrice: 1000 }],
};
const overseasPurchaseSummarySlip = {
  id: "PI-SUMMARY-USD",
  date: "2026-08-15",
  supplier: "S003",
  staff: "管理者",
  purchaseTaxMode: "overseas",
  issueFxRateScaled: 99900000000,
  issueFxScale: 100000000,
  issuedAt: "2026-08-15T10:00:00+09:00",
  status: "処理済",
  lines: [{
    purchasePrice: 100,
    convertedPurchasePriceJpy: 16000,
    purchaseFxRateScaled: 16000000000,
    purchaseFxScale: 100000000,
  }],
};
assert.equal(window.getPurchaseSlipGrandTotalJPY(domesticPurchaseSummarySlip), 1100);
assert.equal(window.getPurchaseSlipGrandTotalJPY(overseasPurchaseSummarySlip), 16000);
overseasPurchaseSummarySlip.issueFxRateScaled = 20000000000;
overseasPurchaseSummarySlip.issueFxScale = 100000000;
overseasPurchaseSummarySlip.issuedAt = "2026-08-15T12:00:00+09:00";
assert.equal(window.getPurchaseSlipGrandTotalJPY(overseasPurchaseSummarySlip), 16000,
  "issuing or reissuing must not change the registration-time JPY conversion");
window.eval("currentSlipTab = 'purchase'");
window.renderSlipList([domesticPurchaseSummarySlip, overseasPurchaseSummarySlip]);
assert.equal(document.getElementById("slipSummaryTotalLabel").textContent, "合計金額（仕入登録時レート換算・JPY）");
assert.equal(document.getElementById("slipSummaryTotal").textContent, "¥17,100");
assert.doesNotMatch(document.getElementById("slipSummaryTotal").textContent, /国内|海外|\$/u);
const purchaseListHeaders = [...document.querySelectorAll("#slipTableHead th")].map(cell => cell.textContent.trim());
assert.equal(purchaseListHeaders.includes("発行日時"), false, "document lists must label the issuance column as 発行日");
assert.equal(purchaseListHeaders[purchaseListHeaders.indexOf("発行") + 1], "発行日",
  "発行日 must be immediately to the right of 発行");
assert.equal(purchaseListHeaders[purchaseListHeaders.indexOf("支払確認") + 1], "支払日付");
assert.match(window.formatPurchasePaidAtStacked(null), /未払い/u);
assert.equal(window.formatIssuedAtStacked("2026-08-15T12:34:56+09:00").includes(":"), false,
  "issuance and payment timestamps must display the date only in document lists");
assert.match(window.formatIssuedAtStacked("2026-08-15T12:34:56+09:00"), /2026-08-15/u);

window.navigateTo("shipping");
const shippingAvailableItems = inventory.filter(item => item.status === "在庫中");
assert.ok(shippingAvailableItems.length >= 3, "shipping QR/auto-row tests require at least three available inventory items");
assert.match(document.querySelector("#page-shipping .shipping-line-header").textContent, /管理番号/u);
assert.equal(document.querySelector("#page-shipping .shipping-line-header").textContent.includes("商品管理番号"), false);
assert.equal(document.querySelector("#page-shipping .shipping-line-header").textContent.includes("商品コード"), false);
const shippingQrButton = document.querySelector('#page-shipping button[onclick="openBarcodeScanner(\'shipping\')"]');
assert.ok(shippingQrButton, "shipping entry must expose the continuous QR scanner");
assert.match(shippingQrButton.textContent, /QR連続読取/u);
assert.ok(shippingQrButton.parentElement.querySelector('button[onclick="addShippingLine()"]'),
  "shipping QR scanner must share the below-table action row with the add-line button");
const consignmentQrButton = document.querySelector('#page-consignment button[onclick="openBarcodeScanner(\'consignment\')"]');
assert.ok(consignmentQrButton?.parentElement.querySelector('button[onclick="addConsignmentLine()"]'),
  "consignment QR scanner must share the below-table action row with the add-line button");

window.resetShippingForm();
const firstShippingInput = document.getElementById("sh-code-1");
firstShippingInput.value = shippingAvailableItems[0].code;
assert.equal(window.onShippingManagementNumberInput(firstShippingInput, 1), true);
assert.equal(document.querySelectorAll("#shippingLines .slip-line").length, 2,
  "manual entry must append a trailing blank row after a valid management number");
assert.equal(document.getElementById("sh-code-2").value, "");
assert.equal(window.addShippingItemByCode(shippingAvailableItems[1].code, { notify: false, focusNext: false }), true);
assert.equal(document.getElementById("sh-code-2").value, shippingAvailableItems[1].code,
  "the next QR scan must append to the next blank row");
assert.equal(document.querySelectorAll("#shippingLines .slip-line").length, 3,
  "continuous QR scans must keep one trailing blank row");
assert.equal(window.addShippingItemByCode(shippingAvailableItems[1].code, { notify: false }), false,
  "the same management number must not be added twice");
let shippingCollected = window.collectShippingItemsForSave();
assert.deepEqual(shippingCollected.items.map(item => item.code), shippingAvailableItems.slice(0, 2).map(item => item.code));
assert.equal(shippingCollected.items.some(item => item.code === ""), false,
  "the trailing blank row must never be included in the saved shipment payload");

window.eval("_barcodeMode = 'shipping'; _barcodeScanCount = 0; _barcodeCooldown = false; _barcodeLastCode = '';");
document.getElementById("barcodeScanCount").textContent = "0";
window._onBarcodeDetected(shippingAvailableItems[2].code);
assert.equal(document.getElementById("sh-code-3").value, shippingAvailableItems[2].code,
  "camera QR detection must continuously append management numbers");
assert.equal(document.getElementById("sh-code-4").value, "");
assert.equal(document.getElementById("barcodeScanCount").textContent, "1");

window.resetShippingForm();
shippingCollected = window.collectShippingItemsForSave();
assert.equal(shippingCollected.items.length, 0, "a blank shipment row must not become a shipment item");

const shippingTemplateItem = shippingAvailableItems[0];
document.getElementById("sh-dest").value = "B001";
document.getElementById("sh-code-1").value = shippingTemplateItem.code;
window.onShippingManagementNumberInput(document.getElementById("sh-code-1"), 1);
document.getElementById("sh-price-1").value = "1001";
window.onShippingPriceInput(document.getElementById("sh-price-1"));
const downloadClicks = [];
const originalAnchorClick = window.HTMLAnchorElement.prototype.click;
window.HTMLAnchorElement.prototype.click = function clickDownloadAnchor() {
  downloadClicks.push({ filename: this.download, href: this.href });
};
const shippingDownloadButton = document.querySelector('#page-shipping button[onclick="downloadCurrentShipmentDocument(true)"]');
assert.match(shippingDownloadButton.textContent, /ダウンロード/u);
assert.equal(shippingDownloadButton.textContent.includes("CSV"), false, "shipping button must be labelled as download");
await window.downloadCurrentShipmentDocument(true);
assert.match(downloadClicks.at(-1).filename, /_出荷伝票\.pdf$/u);
const shippingDownload = window.exportCSV("shipping");
assert.match(shippingDownload.filename, /^shipping_slip_.*\.csv$/u);
assert.match(shippingDownload.csv, /売価/u);
assert.match(shippingDownload.csv, /円換算売価（JPY・1,000円単位切上げ）/u);
assert.match(shippingDownload.csv, /1001/u, "shipping download must include the entered USD sale amount");
const expectedShippingDownloadJPY = window.roundShippingJPYToThousand(1001 * window.getShippingFormRate());
assert.match(shippingDownload.csv, new RegExp(String(expectedShippingDownloadJPY), "u"),
  "shipping download must include per-item JPY rounding at the fixed rate");
assert.equal(downloadClicks.at(-1).filename, shippingDownload.filename);
const shippingPrintHtml = window.buildShipmentSlipHTML();
assert.match(shippingPrintHtml, /出荷伝票/u);
assert.match(shippingPrintHtml, /明細表/u);
assert.match(shippingPrintHtml, /\$1,001/u, "shipping slips must print the entered USD sale amount");
assert.match(shippingPrintHtml, /合計金額/u);
assert.match(shippingPrintHtml, /商品情報/u);
assert.match(shippingPrintHtml, /社内管理/u);
assert.match(shippingPrintHtml, /tpl-product-grid/u);
assert.equal(shippingPrintHtml.includes("文字盤:"), false, "shipping product detail must not include the dial field");
assert.equal(shippingPrintHtml.includes("発行日"), false, "shipping slips must not show an issue date");
window.switchShippingEntryCurrency("JPY");
const shippingPrintJpyHtml = window.buildShipmentSlipHTML();
assert.match(shippingPrintJpyHtml, /¥156,000/u, "shipping JPY print must use per-item 1,000-yen rounding");
window.switchShippingEntryCurrency("USD");
assert.match(shippingPrintHtml, new RegExp(companyInfo.companyName));
assert.match(shippingPrintHtml, new RegExp(companyInfo.address));
assert.equal(shippingPrintHtml.includes("お振込先"), false, "shipping slips must not print bank details");

window.navigateTo("sales");
assert.equal(document.querySelector('label[for="sl-id"]').textContent.trim(), "参照伝票番号");
assert.match(document.getElementById("sl-id").placeholder, /＋ボタンから出荷・委託伝票を選択/u);
assert.equal(document.getElementById("sl-id").readOnly, true, "sales references must be selected through the multi-document picker");
assert.ok(document.getElementById("salesReferenceModal"), "sales must provide a multi-document reference picker");
assert.ok(document.getElementById("sales-reference-number-search"), "sales reference picker must filter by slip number");
assert.ok(document.getElementById("sales-reference-destination-search"), "sales reference picker must filter by destination");
const salesReferenceTestItems = window.eval("APP_DATA.inventory.slice(0, 3)");
const salesReferenceOriginalStatuses = salesReferenceTestItems.map(item => item.status);
salesReferenceTestItems[0].status = "出荷済";
salesReferenceTestItems[1].status = "委託中";
salesReferenceTestItems[2].status = "在庫中";
const salesReferenceShipment = { id: "SH-REF-PROCESSING", date: "2099-01-01", destination: "B001", items: [{ code: salesReferenceTestItems[0].code }] };
const salesReferenceConsignment = { id: "CO-REF-PROCESSING", date: "2099-01-02", destination: "B002", items: [{ code: salesReferenceTestItems[1].code }] };
const salesReferenceCompleted = { id: "SH-REF-COMPLETED", date: "2099-01-03", destination: "B001", items: [{ code: salesReferenceTestItems[2].code }] };
window.eval("APP_DATA.shipments").push(salesReferenceShipment, salesReferenceCompleted);
window.eval("APP_DATA.consignments").push(salesReferenceConsignment);
// Unapproved worker shipments must stay hidden even when inventory makes them processing.
const unapprovedReference = {...salesReferenceShipment, id:'SY-UNAPPROVED', apiManaged:true, apiStatus:'draft', processingStatus:'processing'};
window.eval('APP_DATA.shipments').push(unapprovedReference);
for (const status of ['draft','pending_approval','returned','rejected','cancelled',undefined]) {
  unapprovedReference.apiStatus = status;
  window.openSalesReferenceModal();
  assert.equal(document.querySelector('[data-reference-id="sy-unapproved"]'),null);
  assert.equal(window.appendSalesReference('shipment',unapprovedReference),false);
  window.onSalesIdInput(unapprovedReference.id);
  assert.equal(window.eval('_salesSourceShipmentId'),null);
}
unapprovedReference.apiStatus='confirmed';
window.openSalesReferenceModal();
assert.ok(document.querySelector('[data-reference-id="sy-unapproved"]'),'approved shipment becomes selectable');
const staleReferenceChoice=document.querySelector('.sales-reference-check[value="SY-UNAPPROVED"]');
staleReferenceChoice.checked=true;
unapprovedReference.apiStatus='draft';
window.addSelectedSalesReferences();
assert.equal(window.eval('_salesSourceShipmentId'),null,'stale checked choice must not import an unapproved shipment');
window.eval('APP_DATA.shipments').pop();
window.resetSalesForm();
window.openSalesReferenceModal();
assert.ok(document.querySelector('[data-reference-id="sh-ref-processing"]'));
assert.ok(document.querySelector('[data-reference-id="co-ref-processing"]'));
assert.equal(document.querySelector('[data-reference-id="sh-ref-completed"]'), null, "completed documents must not be selectable");
document.getElementById("sales-reference-number-search").value = "CO-REF";
window.filterSalesReferenceList();
assert.equal(document.querySelector('[data-reference-id="sh-ref-processing"]').classList.contains("hidden"), true);
assert.equal(document.querySelector('[data-reference-id="co-ref-processing"]').classList.contains("hidden"), false);
document.getElementById("sales-reference-number-search").value = "";
document.getElementById("sales-reference-destination-search").value = window.getBuyerName("B001");
window.filterSalesReferenceList();
assert.equal(document.querySelector('[data-reference-id="co-ref-processing"]').classList.contains("hidden"), true);
const processingShipmentSummary = document.querySelector('[data-reference-id="sh-ref-processing"] .sales-reference-summary');
window.toggleSalesReferenceDetails(processingShipmentSummary);
assert.equal(document.querySelector('[data-reference-id="sh-ref-processing"] .sales-reference-details').classList.contains("hidden"), false);
assert.match(document.querySelector('[data-reference-id="sh-ref-processing"] .sales-reference-details').textContent, new RegExp(salesReferenceTestItems[0].code));
window.closeSalesReferenceModal();
// Imported details must follow reference deselection without rebuilding unrelated rows.
window.resetSalesForm();
salesReferenceConsignment.destination = salesReferenceShipment.destination;
window.addSalesItemByCode(salesReferenceTestItems[2].code, { notify: false });
document.getElementById("sl-note").value = "手入力の備考";
window.appendSalesReference("shipment", salesReferenceShipment);
window.appendSalesReference("consignment", salesReferenceConsignment);
const referenceSalesRows = () => [...document.querySelectorAll("#salesLines tr[data-line-id]")];
const referenceRowCode = row => document.getElementById("sl-code-" + row.dataset.lineId).value;
const retainedReferenceRow = referenceSalesRows().find(row => referenceRowCode(row) === salesReferenceTestItems[1].code);
document.getElementById("sl-price-" + retainedReferenceRow.dataset.lineId).value = "123456";
window.openSalesReferenceModal();
const referenceChoice = (type, id) => document.querySelector('.sales-reference-check[data-type="' + type + '"][value="' + id + '"]');
const shipmentReferenceChoice = referenceChoice("shipment", salesReferenceShipment.id);
assert.equal(shipmentReferenceChoice.checked, true);
assert.equal(shipmentReferenceChoice.disabled, false, "added references must be deselectable");
shipmentReferenceChoice.click();
assert.deepEqual(referenceSalesRows().map(referenceRowCode), [salesReferenceTestItems[2].code, salesReferenceTestItems[1].code]);
assert.equal(document.getElementById("sl-price-" + retainedReferenceRow.dataset.lineId).value, "123456", "deselecting another source must preserve edited prices");
assert.equal(window.eval("_salesSourceShipmentId"), null);
assert.equal(document.getElementById("sl-id").value, salesReferenceConsignment.id);
assert.equal(document.getElementById("sl-note").value.includes(salesReferenceShipment.id), false);
assert.equal(document.getElementById("sl-note").value.includes("手入力の備考"), true);
window.closeSalesReferenceModal();
window.openSalesReferenceModal();
assert.equal(referenceChoice("shipment", salesReferenceShipment.id).checked, false);
referenceChoice("consignment", salesReferenceConsignment.id).click();
assert.deepEqual(referenceSalesRows().map(referenceRowCode), [salesReferenceTestItems[2].code], "manual rows must survive all reference removals");
assert.equal(document.getElementById("sl-id").value, "");
assert.equal(document.getElementById("sl-note").value, "手入力の備考");
referenceChoice("shipment", salesReferenceShipment.id).click();
window.addSelectedSalesReferences();
assert.deepEqual(referenceSalesRows().map(referenceRowCode), [salesReferenceTestItems[2].code, salesReferenceTestItems[0].code], "removed references can be re-added");
window.resetSalesForm();
window.appendSalesReference("shipment", salesReferenceShipment);
window.openSalesReferenceModal();
referenceChoice("shipment", salesReferenceShipment.id).click();
assert.deepEqual(referenceSalesRows().map(referenceRowCode), [""], "removing the last source must leave one empty entry row");
assert.equal(document.getElementById("salesTotalDisplay").textContent, "¥0");
assert.equal(window.eval("_salesSourceDocuments.length"), 0);
window.closeSalesReferenceModal();
window.resetSalesForm();
window.eval("APP_DATA.shipments").splice(window.eval("APP_DATA.shipments").indexOf(salesReferenceShipment), 1);
window.eval("APP_DATA.shipments").splice(window.eval("APP_DATA.shipments").indexOf(salesReferenceCompleted), 1);
window.eval("APP_DATA.consignments").splice(window.eval("APP_DATA.consignments").indexOf(salesReferenceConsignment), 1);
salesReferenceTestItems.forEach((item, index) => { item.status = salesReferenceOriginalStatuses[index]; });
assert.match(document.querySelector('[onclick="openBarcodeScanner(\'sales\')"]').textContent, /QR連続読取/u);
assert.ok(document.getElementById("barcodeManualInput"), "the continuous scanner must accept USB/Bluetooth QR-reader input");
const salesQrInventoryItem = window.eval("APP_DATA.inventory.find(item => item.status === '在庫中' && Number(item.salePrice) > 0)");
assert.ok(salesQrInventoryItem, "sales QR tests require an available inventory item");
window.resetSalesForm();
window.eval("_barcodeMode = 'sales'; _barcodeScanCount = 0; _barcodeCooldown = false; _barcodeLastCode = '';");
document.getElementById("barcodeScanCount").textContent = "0";
document.getElementById("barcodeManualInput").value = salesQrInventoryItem.code.toLowerCase();
assert.equal(window.submitBarcodeManualInput(), true);
assert.equal(document.querySelector("#salesLines input[id^='sl-code-']").value, salesQrInventoryItem.code,
  "QR-reader Enter input must add the scanned product to sales details");
assert.equal(document.getElementById("barcodeScanCount").textContent, "1");
document.getElementById("barcodeManualInput").value = salesQrInventoryItem.code;
window.submitBarcodeManualInput();
assert.equal(document.querySelectorAll("#salesLines tr[data-line-id]").length, 1,
  "continuous sales scanning must not add a duplicate product");
window.resetSalesForm();
assert.equal(document.getElementById("slTakebackPanel"), null, "unchecked sales rows must no longer create a takeback preview");
assert.equal(window.getSalesUsdRate(), 155, "sales conversion must use the USD rate from master data");
assert.equal(document.getElementById("sl-currency-usd").getAttribute("aria-pressed"), "false");
assert.equal(document.getElementById("sl-currency-jpy").getAttribute("aria-pressed"), "true");
assert.deepEqual(
  [...document.querySelectorAll("#page-sales .sl-currency-switch .sl-currency-btn")].map(button => button.id),
  ["sl-currency-jpy", "sl-currency-usd", "sl-currency-hkd", "sl-currency-eur"],
  "sales currencies must be ordered JPY, USD, HKD, EUR",
);
assert.match(document.getElementById("sl-price-heading").textContent, /円/u);
assert.match(document.getElementById("sl-price-rate").textContent, /1 USD = ¥155\.00/u);

// Round each automatically populated JPY line upward, never the final aggregate.
assert.equal(window.formatSalesEntryAmount(1001,'JPY'),'156,000');
assert.equal(window.formatSalesEntryAmount(1000,'JPY'),'155,000');
assert.equal(window.formatSalesEntryAmount(0,'JPY'),'');
assert.equal(window.formatSalesEntryAmount(1001,'USD'),'1,001');
const roundingProduct={code:'ROUND-TEST',brand:'TEST',model:'Rounding',status:'出荷済',salePrice:1001,purchasePrice:10000};
window.eval('APP_DATA.inventory').push(roundingProduct);
const roundingShipment={id:'SY-170926998',date:'2026-09-17',destination:'',items:[{...roundingProduct,salePrice:1001}]};
window.applyShipmentToSales(roundingShipment);
assert.equal(document.getElementById('sl-price-1').value,'156,000','shipment number lookup must round JPY up');
assert.equal(document.getElementById('salesTotalDisplay').textContent,'¥156,000');
document.getElementById('sl-price-1').value='156123';window.onSalesPriceInput(document.getElementById('sl-price-1'));
window.switchSalesEntryCurrency('JPY');
assert.equal(document.getElementById('sl-price-1').value,'156,123','selected currency click must preserve manually entered yen');
window.eval('APP_DATA.shipments').push(roundingShipment);
window.resetSalesForm();window.appendSalesReference('shipment',roundingShipment);
assert.equal(document.querySelector('#salesLines .sl-input-price').value,'156,000','reference picker must use the same rounding');
window.resetSalesForm();window.switchSalesEntryCurrency('USD');window.applyShipmentToSales(roundingShipment);
assert.equal(document.getElementById('sl-price-1').value,'1,001');
window.switchSalesEntryCurrency('JPY');assert.equal(document.getElementById('sl-price-1').value,'156,000');
window.eval('APP_DATA.inventory').pop();window.eval('APP_DATA.shipments').pop();window.resetSalesForm();

const sourceShipmentForClear = window.eval("APP_DATA.shipments.find(shipment => Array.isArray(shipment.items) && shipment.items.length > 0)");
assert.ok(sourceShipmentForClear, "a shipment with items is required for the sales-link clear test");
const oldShipmentNumber = sourceShipmentForClear.id;
for (const number of [oldShipmentNumber, 'SY-2026-0998']) {
  sourceShipmentForClear.id = number;
  window.onSalesIdInput(number);
  assert.equal(window.eval('_salesSourceShipmentId'), number, 'both old and new shipment numbers must link sales details');
}
sourceShipmentForClear.id = oldShipmentNumber;
window.onSalesIdInput(oldShipmentNumber);
assert.equal(document.querySelectorAll("#salesLines tr[data-line-id]").length, sourceShipmentForClear.items.length);
assert.equal(document.getElementById("sl-code-1").value, sourceShipmentForClear.items[0].code);
document.getElementById("sl-id").value = "";
window.onSalesIdInput("");
assert.equal(window.eval("_salesSourceShipmentId"), null, "clearing the shipment number must remove the source link");
assert.equal(document.querySelectorAll("#salesLines tr[data-line-id]").length, 1, "clearing the shipment number must leave only one blank detail row");
assert.equal(document.getElementById("sl-code-1").value, "", "clearing the shipment number must clear the linked product detail");
assert.equal(document.getElementById("sl-price-1").value, "", "clearing the shipment number must clear the linked price");
assert.equal(document.getElementById("sl-note").value, "", "clearing the shipment number must clear the auto-filled shipment note");

const consignmentInventoryItem = window.eval("APP_DATA.inventory.find(item => item.status === '在庫中' && Number(item.salePrice) > 0)");
assert.ok(consignmentInventoryItem, "an in-stock item with a sale price is required for the consignment-to-sales test");
const originalConsignmentItemStatus = consignmentInventoryItem.status;
consignmentInventoryItem.status = "委託中";
const sourceConsignmentForSales = {
  id: "IT-2026-0999",
  date: "2026-08-01",
  destination: sourceShipmentForClear.destination || "B001",
  items: [{
    code: consignmentInventoryItem.code,
    brand: consignmentInventoryItem.brand,
    model: consignmentInventoryItem.model,
  }],
};
window.eval("APP_DATA.consignments").push(sourceConsignmentForSales);
const salesDateBeforeConsignmentLink = document.getElementById("sl-date").value;
document.getElementById("sl-id").value = sourceConsignmentForSales.id;
window.onSalesIdInput(sourceConsignmentForSales.id);
assert.equal(window.eval("_salesSourceShipmentId"), null, "a consignment source must not retain the shipment link");
assert.equal(window.eval("_salesSourceConsignmentId"), sourceConsignmentForSales.id);
assert.equal(document.getElementById("sl-id").value, sourceConsignmentForSales.id,
  "the entered consignment number must remain visible as the source reference");
assert.equal(document.getElementById("sl-buyer").value, sourceConsignmentForSales.destination,
  "the consignee must populate the sales destination");
assert.equal(document.getElementById("sl-code-1").value, consignmentInventoryItem.code);
assert.notEqual(document.getElementById("sl-price-1").value, "", "the inventory sale price must populate from a consignment slip");
assert.equal(document.getElementById("sl-date").value, salesDateBeforeConsignmentLink,
  "linking a consignment must keep the actual sales date instead of copying the consignment date");
assert.equal(window.canUseInventoryItemForSales(consignmentInventoryItem), true,
  "a consigned item may be sold with a reference");
assert.match(document.getElementById("sl-note").value, /委託伝票: IT-2026-0999/u);
document.getElementById("sl-id").value = "";
window.onSalesIdInput("");
assert.equal(window.eval("_salesSourceConsignmentId"), null);
assert.equal(window.canUseInventoryItemForSales(consignmentInventoryItem), true,
  "consigned items must also support direct sales without a reference");
// Direct entry, clearing the code, and reopening a reference must remain recoverable.
const directConsignmentCode = document.getElementById("sl-code-1");
directConsignmentCode.value = consignmentInventoryItem.code;
window.autoFillItem(directConsignmentCode, 1, "sales");
assert.equal(document.getElementById("sl-brand-1").textContent.includes("未登録"), false);
document.getElementById("sl-price-1").value = "123456";
window.appendSalesReference("consignment", sourceConsignmentForSales);
assert.equal(document.querySelectorAll("#salesLines tr[data-line-id]").length, 1, "reference must reuse a matching manual row");
assert.equal(document.getElementById("sl-price-1").value, "123456", "reference must preserve an entered price");
directConsignmentCode.value = "";
window.autoFillItem(directConsignmentCode, 1, "sales");
assert.equal(document.getElementById("sl-price-1").value, "");
assert.equal(document.getElementById("salesTotalDisplay").textContent, "¥0");
window.resetSalesForm();
window.appendSalesReference("consignment", sourceConsignmentForSales);
window.removeSalesLine(document.querySelector("#salesLines [onclick^='removeSalesLine']"));
assert.equal(document.querySelectorAll("#salesLines tr[data-line-id]").length, 1);
assert.equal(document.querySelector("#salesLines input[id^='sl-code-']").value, "", "the final row must be removable and replaced by an empty row");
assert.equal(document.getElementById("salesTotalDisplay").textContent, "¥0");
window.resetSalesForm();
window.eval("APP_DATA.consignments").pop();
consignmentInventoryItem.status = originalConsignmentItemStatus;

window.switchSalesEntryCurrency("USD");
// Manual management-code entry uses the same expected price and currency as reference entry.
const manualPriceFixtures = [
 {code:'TEST-MANUAL-PRICE-1',brand:'TEST',model:'One',status:'在庫中',salePrice:1000,purchasePrice:10000},
 {code:'TEST-MANUAL-PRICE-2',brand:'TEST',model:'Two',status:'委託中',salePrice:2000,purchasePrice:20000},
 {code:'TEST-MANUAL-PRICE-0',brand:'TEST',model:'Zero',status:'在庫中',salePrice:0,purchasePrice:10000},
 {code:'TEST-MANUAL-PRICE-SOLD',brand:'TEST',model:'Sold',status:'売上済',salePrice:3000,purchasePrice:10000},
];
window.eval("APP_DATA.inventory").push(...manualPriceFixtures);
for(const currency of ['JPY','USD','HKD','EUR']) {
 window.resetSalesForm();window.switchSalesEntryCurrency(currency);
 const code=document.getElementById('sl-code-1'),price=document.getElementById('sl-price-1');
 const enter=value=>{code.value=value;window.autoFillItem(code,1,'sales');};
 enter(manualPriceFixtures[0].code);
 assert.equal(price.value,window.formatSalesEntryAmount(1000,currency),'manual entry must populate '+currency);
 assert.equal(price.dataset.entryCurrency,currency);
 assert.equal(document.getElementById('salesTotalDisplay').textContent,window.formatSalesEntryDisplayValue(window.parseSalesPrice(price.value),currency));
 price.value='12345';window.onSalesPriceInput(price);enter(' '+manualPriceFixtures[0].code+' ');
 assert.equal(price.value,'12,345','same product lookup must preserve a manual override');
 enter(manualPriceFixtures[1].code);assert.equal(price.value,window.formatSalesEntryAmount(2000,currency),'changing product must refresh its price');
 enter('UNKNOWN');assert.equal(price.value,'');assert.equal(window.getSalesLinePriceUSD(price),0);
 enter(manualPriceFixtures[0].code);enter(manualPriceFixtures[3].code);assert.equal(price.value,'','unavailable product must not retain previous price');
 enter(manualPriceFixtures[0].code);enter(manualPriceFixtures[2].code);assert.equal(price.value,'','zero-price product must not retain previous price');
 enter(manualPriceFixtures[0].code);enter('');assert.equal(price.value,'');assert.equal(window.getSalesLinePriceUSD(price),0);
}
window.eval("APP_DATA.inventory").splice(-manualPriceFixtures.length);
window.resetSalesForm();window.switchSalesEntryCurrency('USD');
const salesPriceInput = document.getElementById("sl-price-1");
salesPriceInput.value = "1000";
window.onSalesPriceInput(salesPriceInput);
assert.equal(salesPriceInput.value, "1,000");
assert.equal(salesPriceInput.dataset.entryCurrency, "USD");
assert.equal(document.getElementById("salesTotalDisplay").textContent.trim(), "$1,000");
assert.equal(document.getElementById("salesTotalTax").textContent.trim(), "TAX FREE");
assert.equal(document.getElementById("salesTotalGrand").textContent.trim(), "$1,000");
assert.match(document.getElementById("salesSubtotalLabel").textContent, /USD/u);
assert.match(document.getElementById("salesTotalFxReference").textContent, /¥155,000/u);

window.switchSalesEntryCurrency("JPY");
assert.equal(document.getElementById("sl-currency-usd").getAttribute("aria-pressed"), "false");
assert.equal(document.getElementById("sl-currency-jpy").getAttribute("aria-pressed"), "true");
assert.equal(salesPriceInput.value, "155,000", "USD amount must convert to JPY using the master rate");
assert.equal(salesPriceInput.getAttribute("aria-label"), "売価");
assert.match(document.getElementById("sl-price-heading").textContent, /円入力/u);
assert.equal(document.getElementById("salesTotalDisplay").textContent.trim(), "¥155,000");
assert.equal(document.getElementById("salesTotalTax").textContent.trim(), "¥15,500");
assert.equal(document.getElementById("salesTotalGrand").textContent.trim(), "¥170,500");
assert.match(document.getElementById("salesSubtotalLabel").textContent, /円/u);
assert.match(document.getElementById("salesTaxLabel").textContent, /円/u);
assert.match(document.getElementById("salesGrandLabel").textContent, /円/u);
assert.match(document.getElementById("salesTotalFxReference").textContent, /参考USD換算: \$1,100/u);

salesPriceInput.value = "310000";
window.onSalesPriceInput(salesPriceInput);
assert.equal(salesPriceInput.value, "310,000");
assert.equal(window.getSalesLinePriceUSD(salesPriceInput), 2000);
assert.equal(document.getElementById("salesTotalDisplay").textContent.trim(), "¥310,000");
assert.equal(document.getElementById("salesTotalTax").textContent.trim(), "¥31,000");
assert.equal(document.getElementById("salesTotalGrand").textContent.trim(), "¥341,000");
assert.match(document.getElementById("salesTotalFxReference").textContent, /\$2,200/u);

const salesPrintHtml = window.buildSalesSlip2PageHTML();
assert.match(salesPrintHtml, /請求書/u);
assert.match(salesPrintHtml, /明細表/u);
assert.match(salesPrintHtml, /表示通貨：JPY（円）/u);
assert.match(salesPrintHtml, /売上登録時に固定予定のレート/u);
assert.match(salesPrintHtml, /¥310,000/u);
assert.match(salesPrintHtml, /¥341,000/u);
assert.match(salesPrintHtml, /消費税（10%）/u);
assert.match(salesPrintHtml, /お振込先/u);
assert.match(salesPrintHtml, new RegExp(companyInfo.companyName));
assert.match(salesPrintHtml, new RegExp(companyInfo.address));
assert.match(salesPrintHtml, new RegExp(companyInfo.bankName));
assert.match(salesPrintHtml, new RegExp(companyInfo.branchName));
assert.match(salesPrintHtml, new RegExp(companyInfo.accountNumber));
assert.equal(salesPrintHtml.includes("_fmtYen"), false);
assert.equal(appSource.includes("_fmtYen(subtotal)"), false, "sales print totals must not use JPY formatting");

document.getElementById("sl-code-1").value = shippingTemplateItem.code;
window.autoFillItem(document.getElementById("sl-code-1"), 1, "sales");
document.getElementById("sl-buyer").value = "B001";
salesPriceInput.value = "310000";
window.onSalesPriceInput(salesPriceInput);
const salesDownloadButton = document.querySelector('#page-sales button[onclick="downloadCurrentSalesDocument(true)"]');
assert.match(salesDownloadButton.textContent, /ダウンロード/u);
assert.equal(salesDownloadButton.textContent.includes("CSV"), false, "sales button must be labelled as download");
await window.downloadCurrentSalesDocument(true);
assert.match(downloadClicks.at(-1).filename, /_請求書\.pdf$/u);
const salesDownload = window.exportCSV("sales");
assert.match(salesDownload.filename, /^sales_slip_.*\.csv$/u);
assert.match(salesDownload.csv, /売価/u);
assert.match(salesDownload.csv, /310000/u, "sales download must use the selected JPY amount");
assert.match(salesDownload.csv, /341000/u, "sales download must include the taxed total");
assert.equal(downloadClicks.at(-1).filename, salesDownload.filename);
window.HTMLAnchorElement.prototype.click = originalAnchorClick;

window.onTaxFreeToggle(true);
const taxFreeSalesPrintHtml = window.buildSalesSlip2PageHTML();
assert.match(taxFreeSalesPrintHtml, /免税（0%）/u);
assert.match(taxFreeSalesPrintHtml, /合計金額（免税）/u);
assert.match(taxFreeSalesPrintHtml, /¥310,000/u);
assert.equal(taxFreeSalesPrintHtml.includes("¥341,000"), false, "tax-free invoices must not add consumption tax");
window.onTaxFreeToggle(false);

window.switchSalesEntryCurrency("USD");
assert.equal(salesPriceInput.value, "2,000", "JPY input must round-trip back to the USD base amount");
assert.equal(document.getElementById("salesTotalDisplay").textContent.trim(), "$2,000");
assert.equal(document.getElementById("salesTotalTax").textContent.trim(), "TAX FREE");
assert.equal(document.getElementById("salesTotalGrand").textContent.trim(), "$2,000");
assert.match(document.getElementById("salesTotalFxReference").textContent, /¥310,000/u);
const usdSalesPrintHtml = window.buildSalesSlip2PageHTML();
assert.match(usdSalesPrintHtml, /表示通貨：USD/u);
assert.match(usdSalesPrintHtml, /\$2,000/u);
assert.match(usdSalesPrintHtml, /税区分：TAX FREE/u);
assert.equal(usdSalesPrintHtml.includes("$2,200"), false, "USD invoices must not add consumption tax");
window.switchSalesEntryCurrency("JPY");

const saleInventoryItem = inventory.find((item) => item.status === "在庫中");
assert.ok(saleInventoryItem, "an in-stock item is required for the sales currency save contract");
document.getElementById("sl-code-1").value = saleInventoryItem.code;
window.autoFillItem(document.getElementById("sl-code-1"), 1, "sales");
document.getElementById("sl-buyer").value = "B001";
window.saveSales();
const savedCurrencySale = sales.at(-1);
assert.equal(savedCurrencySale.currency, "USD", "saved sales slips must use USD as the base currency");
assert.equal(savedCurrencySale.inputCurrency, "JPY");
assert.equal(savedCurrencySale.usdJpyRate, 155);
assert.equal(savedCurrencySale.total, 2000);
assert.equal(savedCurrencySale.taxAmount, 200);
assert.equal(savedCurrencySale.grandTotal, 2200);
assert.equal(savedCurrencySale.items[0].salePrice, 2000);
assert.equal(savedCurrencySale.items[0].inputCurrency, "JPY");
assert.equal(savedCurrencySale.items[0].inputAmount, 310000);
assert.equal(document.getElementById("sl-currency-jpy").getAttribute("aria-pressed"), "true", "sales reset must restore JPY input");

assert.equal(typeof window.marketImportCsvText, "function", "market CSV import must be globally callable");
assert.equal(typeof window.marketOpenEdit, "function", "market edit modal must be globally callable");
const marketRegistrationPage = document.getElementById("page-market-entry");
assert.ok(marketRegistrationPage, "market registration must have its own page");
assert.ok(marketRegistrationPage.querySelector("#market-template-download-button"), "market registration must expose the CSV template");
assert.ok(marketRegistrationPage.querySelector("#market-csv-import-button"), "market registration must expose CSV import");
assert.ok(marketRegistrationPage.querySelector("#market-csv-file-input"), "market registration must own the CSV file input");
assert.ok(marketRegistrationPage.querySelector("#marketEntryTable"), "market registration must show the registered market list");
assert.ok(marketRegistrationPage.querySelector("#marketEntryTableBody"), "market registration must own a registered market table body");
assert.ok(marketRegistrationPage.querySelector("#marketDraftTable"), "market registration must show an inline editable detail preview");
assert.ok(marketRegistrationPage.querySelector("#market-add-draft-button"), "market registration must allow handwritten detail rows");
assert.ok(marketRegistrationPage.querySelector("#marketDraftAddCount"), "market registration must accept a numeric bulk row count");
assert.ok(marketRegistrationPage.querySelector("#market-entry-basic-card"), "market registration must expose shared basic information");
assert.ok(marketRegistrationPage.querySelector("#market-basic-category"));
assert.ok(marketRegistrationPage.querySelector("#market-basic-auction"));
assert.ok(marketRegistrationPage.querySelector("#market-basic-research-date"));
assert.ok(marketRegistrationPage.querySelector("#market-basic-currency"));
assert.deepEqual(
  [...marketRegistrationPage.querySelectorAll(".market-basic-grid > .form-group")]
    .map(group => group.querySelector("select, input[id]")?.id),
  ["market-basic-research-date", "market-basic-auction", "market-basic-category", "market-basic-currency"],
  "market research date and market category must appear in the swapped positions",
);
assert.equal(document.getElementById("market-basic-currency-group").hidden, false,
  "transaction currency must remain visible so domestic JPY is explicit");
assert.deepEqual(
  [...document.getElementById("market-basic-currency").options].map(option => option.value),
  ["JPY", "USD", "HKD"],
  "transaction currency must support JPY, USD, and HKD",
);
assert.equal(document.getElementById("market-basic-currency").value, "JPY");
assert.equal(document.getElementById("market-basic-currency").disabled, true,
  "domestic market categories must lock transaction currency to JPY");
assert.equal(document.getElementById("market-basic-currency-lock-note").hidden, false);
assert.equal(document.getElementById("market-basic-rate").textContent, "1 JPY = ¥1.00");
document.getElementById("market-basic-category").value = "overseas";
window.marketApplyBasicInfoToDrafts();
assert.equal(document.getElementById("market-basic-currency").disabled, false,
  "overseas market research must enable transaction currency selection");
assert.equal(document.getElementById("market-basic-currency-lock-note").hidden, true);
document.getElementById("market-basic-currency").value = "HKD";
window.marketApplyBasicInfoToDrafts();
assert.equal(document.getElementById("market-basic-currency").value, "HKD");
document.getElementById("market-basic-category").value = "domestic-retail";
window.marketApplyBasicInfoToDrafts();
assert.equal(document.getElementById("market-basic-currency").value, "JPY",
  "switching from overseas to a domestic category must reset currency to JPY");
assert.equal(document.getElementById("market-basic-currency").disabled, true);
assert.equal(marketRegistrationPage.querySelectorAll(".market-entry-step").length, 2,
  "the redundant handwritten-entry guide card must be removed");
assert.equal(marketRegistrationPage.querySelector(".market-entry-step-manual"), null);
assert.ok(marketRegistrationPage.querySelector("#marketDraftRegisterButton"), "market registration must place a final registration button in the detail footer");
assert.equal(typeof window.marketAddDraftRow, "function");
assert.equal(typeof window.marketAddDraftRows, "function");
assert.equal(typeof window.marketConfirmDraftRegistration, "function");
assert.equal(document.querySelector("#page-market #market-csv-import-button"), null, "market table must not expose CSV import");
assert.equal(document.querySelector("#page-market #market-template-download-button"), null, "market table must not expose the registration template");
window.navigateTo("market-entry");
assert.equal(marketRegistrationPage.classList.contains("hidden"), false, "market registration navigation must open the registration page");
assert.equal(document.getElementById("pageTitle").textContent, "相場登録");
assert.match(marketRegistrationPage.querySelector("#market-entry-result-area h3").textContent, /登録済み相場一覧表/u);
assert.equal(document.querySelectorAll("#marketEntryTableBody tr").length, Math.min(inventory.length, 10), "market registration must render persisted rows below the import controls");
const marketEntryColumns = [...document.querySelectorAll("#marketEntryTable thead th")].map(th => th.dataset.marketCol);
const marketMainColumns = [...document.querySelectorAll("#marketTable thead th")].map(th => th.dataset.marketCol);
assert.deepEqual(marketEntryColumns, marketMainColumns, "market registration list must use the same columns as the market list");
assert.deepEqual(marketMainColumns.slice(0, 4), ["importDate", "brand", "ref", "model"], "market list columns must follow date, brand, reference number, model order");
assert.deepEqual(marketMainColumns.slice(4, 6), ["auctionName", "marketCategory"], "market category must be immediately to the right of auction name");
const marketAccessoryColumnIndex = marketMainColumns.indexOf("accessories");
assert.deepEqual(
  marketMainColumns.slice(marketAccessoryColumnIndex, marketAccessoryColumnIndex + 4),
  ["accessories", "material", "movement", "condition"],
  "material, movement, and condition must appear immediately to the right of accessories",
);
window.navigateTo("market");
assert.equal(document.getElementById("pageTitle").textContent, "相場表");
assert.match(document.querySelector("#market-result-area h3").textContent, /相場表/u);
const marketRows = window.eval("APP_DATA.marketPrices");
assert.equal(marketRows.length, inventory.length, "market preview must start from inventory rows");
assert.equal(document.querySelectorAll("#marketTableBody tr").length, inventory.length);
assert.deepEqual(
  [...document.querySelector("#marketTableBody tr").children].slice(0, 4).map(cell => cell.dataset.marketCol),
  ["importDate", "brand", "ref", "model"],
  "market table body must follow the same brand, reference number, model order as the header",
);
const marketAmountsBeforeCurrencySwitch = JSON.stringify(marketRows.map(row => ({
  id: row.id,
  marketPrice: row.marketPrice,
  marketPriceJpy: row.marketPriceJpy,
  marketCurrency: row.marketCurrency,
})));
assert.equal(typeof window.marketSwitchDisplayCurrency, "function");
assert.equal(typeof window.getMarketDisplayAmount, "function");
assert.equal(document.querySelectorAll("[data-market-display-currency]").length, 6, "both market lists must expose JPY, USD, and HKD switches");
window.marketSwitchDisplayCurrency("USD");
assert.equal(document.querySelectorAll('[data-market-display-currency="USD"][aria-pressed="true"]').length, 2);
assert.match(document.querySelector("#marketTableBody td[data-market-col=\"marketPrice\"] strong").textContent, /^\$/u);
assert.match(document.querySelector("#marketEntryTableBody td[data-market-col=\"marketPrice\"] strong").textContent, /^\$/u);
assert.match(document.querySelector("#marketTableBody td[data-market-col=\"marketPrice\"]").textContent, /円換算/u);
window.marketSwitchDisplayCurrency("HKD");
assert.equal(document.querySelectorAll('[data-market-display-currency="HKD"][aria-pressed="true"]').length, 2);
assert.match(document.querySelector("#marketTableBody td[data-market-col=\"marketPrice\"] strong").textContent, /^HK\$/u);
assert.match(document.querySelector("#marketEntryTableBody td[data-market-col=\"marketPrice\"] strong").textContent, /^HK\$/u);
window.marketSwitchDisplayCurrency("JPY");
assert.equal(document.querySelectorAll('[data-market-display-currency="JPY"][aria-pressed="true"]').length, 2);
assert.match(document.querySelector("#marketTableBody td[data-market-col=\"marketPrice\"] strong").textContent, /^¥/u);
assert.match(document.querySelector("#marketEntryTableBody td[data-market-col=\"marketPrice\"] strong").textContent, /^¥/u);
assert.equal(
  JSON.stringify(marketRows.map(row => ({ id: row.id, marketPrice: row.marketPrice, marketPriceJpy: row.marketPriceJpy, marketCurrency: row.marketCurrency }))),
  marketAmountsBeforeCurrencySwitch,
  "display currency switching must not mutate registered market prices",
);
assert.doesNotMatch(document.getElementById("marketTableBody").textContent, /\$/u, "domestic winning bid prices must be displayed in JPY");
assert.match(document.querySelector("#page-market thead").textContent, /市場調査日/u);
assert.match(document.querySelector("#page-market thead").textContent, /オークション名/u);
assert.match(document.querySelector("#page-market thead").textContent, /市場区分/u);
assert.match(document.querySelector("#page-market thead").textContent, /取引価格/u);
assert.match(document.querySelector("#page-market thead").textContent, /市場調査レート/u);
assert.match(document.querySelector("#page-market thead").textContent, /付属品\s*素材\s*駆動方式\s*コンディション/u);
assert.match(document.querySelector("#page-market thead").textContent, /備考/u);
assert.doesNotMatch(document.querySelector("#page-market thead").textContent, /シリアル|仕入日|仕入れ価格|担当者|ステータス|BOX/u);
assert.equal(document.getElementById("market-f-serial"), null, "market filters must not include serial number");
assert.equal(document.getElementById("market-f-staff"), null, "market filters must not include staff");
assert.equal(document.getElementById("market-f-supplier"), null, "market filters must not include supplier");
const marketAuctionFilter = document.getElementById("market-f-auction");
assert.notEqual(marketAuctionFilter, null, "market filters must include auction name");
const marketCategoryFilter = document.getElementById("market-f-category");
assert.notEqual(marketCategoryFilter, null, "market filters must include market category beside auction name");
assert.deepEqual(
  [...marketCategoryFilter.options].map(option => option.value),
  ["", "domestic-auction", "overseas", "domestic-retail"],
  "market category filter must expose all supported categories",
);
const auctionMasters = window.eval("APP_DATA.auctionRecords");
assert.deepEqual(
  [...marketAuctionFilter.options].map(option => option.value),
  ["", ...auctionMasters.map(record => record.code)],
  "auction filter values must use stable master codes",
);
assert.deepEqual(
  [...marketAuctionFilter.options].map(option => option.textContent),
  ["すべて", ...auctionMasters.map(record => record.name)],
  "auction filter labels must come from the auction master",
);
marketRows[0].auctionCode = auctionMasters[0].code;
marketRows[0].auctionName = "旧オークション名";
window.init_market();
assert.equal(marketRows[0].auctionName, auctionMasters[0].name, "market row name must follow the current master name");
marketAuctionFilter.value = auctionMasters[0].code;
window.marketApplyFilters();
const expectedAuctionRows = marketRows.filter(row => row.auctionCode === auctionMasters[0].code).length;
assert.equal(document.querySelectorAll("#marketTableBody tr").length, expectedAuctionRows, "auction filter must match by master code");
assert.match(document.getElementById("marketTableBody").textContent, new RegExp(auctionMasters[0].name, "u"));
window.marketResetFilters();
const firstMarketCategoryBeforeFilter = marketRows[0].marketCategory;
marketRows[0].marketCategory = "overseas";
marketCategoryFilter.value = "overseas";
window.marketApplyFilters();
assert.equal(document.querySelectorAll("#marketTableBody tr").length, 1, "market category filter must restrict the market list");
assert.match(document.querySelector("#marketTableBody").textContent, /海外/u);
marketRows[0].marketCategory = firstMarketCategoryBeforeFilter;
window.marketResetFilters();
assert.equal(document.getElementById("market-f-status"), null, "market filters must not include inventory status");
for (const removedKey of ["serial", "purchaseDate", "purchasePrice", "staff", "supplier", "status", "box"]) {
  assert.equal(document.querySelector(`#market-column-panel input[value="${removedKey}"]`), null, `${removedKey} must not be a market column option`);
  assert.equal(document.querySelector(`#marketTable [data-market-col="${removedKey}"]`), null, `${removedKey} must not be rendered in the market table`);
}
assert.equal(document.getElementById("market-purchase-jpy"), null);
assert.equal(document.getElementById("market-price-usd"), null);
assert.equal(document.getElementById("market-price-heading").textContent, "取引価格");
assert.equal(document.querySelector('#marketTableBody td[data-market-col="marketPrice"]').textContent.trim(), "¥1,224,500");
assert.equal(document.querySelector('#marketTableBody td[data-market-col="marketResearchRate"]').textContent.trim(), "1 JPY = ¥1.00");
assert.equal(
  document.querySelector('#marketTableBody td[data-market-col="material"]').textContent.trim(),
  window.getProductSpecName("material", marketRows[0].material) || "—",
  "market material cells must display the master name",
);
assert.equal(
  document.querySelector('#marketTableBody td[data-market-col="movement"]').textContent.trim(),
  window.getProductSpecName("movement", marketRows[0].movement) || "—",
  "market movement cells must display the master name",
);
assert.equal(
  document.querySelector('#marketTableBody td[data-market-col="condition"]').textContent.trim(),
  window.getConditionName(marketRows[0].condition) || "—",
  "market condition cells must display the master name",
);
assert.equal(marketRows[0].marketPriceJpy, 1224500, "legacy preview prices must normalize to JPY once");

window.marketToggleColumnMenu({ stopPropagation() {} });
assert.equal(document.getElementById("market-column-trigger").getAttribute("aria-expanded"), "true");
const marketHiddenKeys = ["marketPrice", "note"];
for (const key of marketHiddenKeys) {
  const checkbox = document.querySelector(`#market-column-panel input[value="${key}"]`);
  checkbox.checked = false;
  window.marketColumnVisibilityChanged(checkbox);
  assert.equal(document.querySelector(`th[data-market-col="${key}"]`).classList.contains("market-col-hidden"), true);
  assert.equal(document.querySelector(`#marketTableBody td[data-market-col="${key}"]`).classList.contains("market-col-hidden"), true);
}
assert.equal(document.getElementById("market-column-count").textContent, "14/16");
window.marketShowAllColumns();
assert.equal(document.getElementById("market-column-count").textContent, "16/16");
assert.equal(document.querySelectorAll("#marketTable .market-col-hidden").length, 0);
window.marketCloseColumnMenu();
const firstMarketId = marketRows[0].id;
window.marketOpenEdit(firstMarketId);
assert.equal(document.getElementById("marketEditModal").classList.contains("hidden"), false, "market edit modal must open");
assert.deepEqual(
  [...document.getElementById("me-marketCurrency").options].map(option => option.value),
  ["JPY", "USD", "HKD"],
  "market edit must allow all transaction currencies",
);
assert.equal(document.getElementById("me-marketPriceLabel").textContent, "取引価格（JPY）");
const marketEditPriceInput = document.getElementById("me-marketPriceJpy");
const marketEditCurrencySelect = document.getElementById("me-marketCurrency");
const marketEditDate = document.getElementById("me-importDate").value;
const marketEditJpyBasis = Number(marketEditPriceInput.dataset.jpyAmount);
marketEditCurrencySelect.value = "USD";
window.marketEditCurrencyChanged({ recalculatePrice: true });
const marketEditUsdRate = window.getMarketJpyRateAtDate("USD", marketEditDate);
assert.equal(window.getPriceValue(marketEditPriceInput), Math.round(marketEditJpyBasis / marketEditUsdRate),
  "changing transaction currency to USD must recalculate the price with the market-date rate");
assert.equal(Number(document.getElementById("me-marketFxRate").dataset.rate), marketEditUsdRate);
assert.match(document.getElementById("me-marketConversionNote").textContent, new RegExp(marketEditDate, "u"));
marketEditCurrencySelect.value = "HKD";
window.marketEditCurrencyChanged({ recalculatePrice: true });
const marketEditHkdRate = window.getMarketJpyRateAtDate("HKD", marketEditDate);
assert.equal(window.getPriceValue(marketEditPriceInput), Math.round(marketEditJpyBasis / marketEditHkdRate),
  "changing transaction currency to HKD must recalculate the price with the market-date rate");
marketEditCurrencySelect.value = "JPY";
window.marketEditCurrencyChanged({ recalculatePrice: true });
assert.equal(window.getPriceValue(marketEditPriceInput), marketEditJpyBasis,
  "round-tripping transaction currencies must preserve the registered JPY basis");
assert.equal(document.getElementById("me-purchaseDate"), null, "market edit must not include purchase date");
assert.equal(document.getElementById("me-status"), null, "market edit must not include inventory status");
assert.equal(document.getElementById("me-box"), null, "market edit must not include BOX assignment");
assert.equal(document.getElementById("me-serial"), null, "market edit must not include serial number");
assert.equal(document.getElementById("me-staff"), null, "market edit must not include staff");
assert.equal(document.getElementById("me-purchasePrice"), null, "market edit must not include purchase price");
marketEditPriceInput.value = "1,259,500";
document.getElementById("me-auctionCode").value = "AUC-001";
document.getElementById("me-note").value = "落札結果を確認済み";
window.marketSaveEdit();
assert.equal(marketRows[0].marketPriceJpy, 1259500, "market edit must save JPY winning bid price");
assert.equal(marketRows[0].auctionName, "東京オークション");
assert.equal(marketRows[0].auctionCode, "AUC-001");
assert.equal(marketRows[0].note, "落札結果を確認済み");
assert.equal(document.getElementById("marketEditModal").classList.contains("hidden"), true, "market edit modal must close after save");

document.getElementById("market-basic-category").value = "overseas";
document.getElementById("market-basic-auction").value = "AUC-001";
document.getElementById("market-basic-research-date").value = "2026-08-01";
window.marketApplyBasicInfoToDrafts();
assert.equal(document.getElementById("market-basic-currency-group").hidden, false,
  "overseas market research must expose the transaction currency selector");
assert.equal(document.getElementById("market-basic-currency").disabled, false,
  "overseas market research must allow transaction currency changes");
document.getElementById("market-basic-currency").value = "HKD";
window.marketApplyBasicInfoToDrafts();
assert.match(document.getElementById("market-basic-rate").textContent, /^1 HKD = ¥/u);
const csvResult = window.marketImportCsvText(
  "ブランドコード,モデル,型番,取引価格,SKU,付属品コード,備考\n" +
  "BRD-001,自由入力モデル,自由入力型番,565750,FREE-SKU-001,ACC-001,自由入力備考",
  "market-test.csv",
);
const marketCountBeforeCsvPreview = marketRows.length;
assert.equal(csvResult.imported, 0);
assert.equal(csvResult.staged, 1);
assert.equal(csvResult.skipped, 0);
assert.equal(marketRows.length, marketCountBeforeCsvPreview, "market CSV staging must not append a row before confirmation");
assert.equal(document.getElementById("marketDraftTableWrap").hidden, false);
assert.equal(document.querySelector('#marketDraftTableBody input[aria-label="明細1 モデル名"]').value, "自由入力モデル");
assert.equal(document.querySelector('#marketDraftTableBody input[type="text"][aria-label="明細1 付属品"]'), null,
  "market draft accessories must no longer use a free-text field");
const importedAccessoryButton = document.querySelector('[data-market-draft-accessories="0"] .market-draft-accessory-button');
assert.ok(importedAccessoryButton, "market draft accessories must expose a multi-select button");
assert.match(importedAccessoryButton.getAttribute("aria-label"), /複数選択/u);
assert.match(importedAccessoryButton.textContent, /BOX/u, "CSV accessory values must be reflected in the selected summary");
window.marketToggleDraftAccessoryMenu(0, { stopPropagation() {} });
const importedAccessoryMenu = document.getElementById("market-draft-accessory-menu-0");
assert.equal(importedAccessoryMenu.hidden, false);
assert.equal(importedAccessoryMenu.querySelector('input[value="BOX"]').checked, true,
  "CSV accessory values must be checked in the multi-select");
const importedGuarantee = importedAccessoryMenu.querySelector('input[value="GUARANTEE"]');
importedGuarantee.checked = true;
window.marketDraftAccessoryChanged(0);
assert.match(importedAccessoryButton.textContent, /2件/u, "multi-select must display the selected accessory count");
assert.deepEqual(Array.from(window.eval("_marketPendingImport.rows[0].accessories")), ["BOX", "GUARANTEE"],
  "multiple checked accessories must be stored in the draft row");
assert.equal(document.getElementById("marketDraftRegisterButton").disabled, false, "registration must become available after staging a valid row");
const marketConfirmResult = await window.marketConfirmCSVImport();
assert.equal(marketConfirmResult.imported, 1);
assert.equal(marketRows.length, marketCountBeforeCsvPreview + 1, "market CSV confirmation must append the staged row");
assert.equal(marketRows.at(-1).importDate, "2026-08-01");
assert.equal(marketRows.at(-1).marketCategory, "overseas");
assert.equal(marketRows.at(-1).auctionName, "東京オークション");
assert.equal(marketRows.at(-1).auctionCode, "AUC-001");
assert.equal(marketRows.at(-1).brand, "ロレックス");
assert.deepEqual([...marketRows.at(-1).accessories], ["BOX", "GUARANTEE"],
  "all accessories selected in the draft must be registered");
assert.equal(marketRows.at(-1).marketCurrency, "HKD");
assert.equal(marketRows.at(-1).marketPrice, 565750);
assert.equal(marketRows.at(-1).marketPriceJpy, Math.round(565750 * marketRows.at(-1).marketFxRate));
assert.equal(marketRows.at(-1).marketFxRate, window.getMarketJpyRate("HKD"));
assert.equal(marketRows.at(-1).note, "自由入力備考");
assert.equal(document.getElementById("marketImportSummary").classList.contains("show"), true);
assert.equal(document.getElementById("marketCsvPreviewModal").classList.contains("hidden"), true);
assert.equal(document.getElementById("marketDraftTableWrap").hidden, true, "confirmed rows must leave the draft preview");
assert.equal(document.getElementById("marketDraftRegisterButton").disabled, true);

const marketCountBeforeManualEntry = marketRows.length;
document.getElementById("marketDraftAddCount").value = "3";
assert.equal(window.marketAddDraftRows(), 3);
assert.match(window.eval("_marketPendingImport.fileName"), /\.csv$/iu,
  "handwritten market rows must use a CSV upload filename accepted by the API");
assert.equal(document.querySelectorAll("#marketDraftTableBody tr").length, 3, "manual entry must add the requested number of editable rows");
window.marketRemoveDraftRow(2);
window.marketRemoveDraftRow(1);
assert.equal(document.querySelectorAll("#marketDraftTableBody tr").length, 1);
const invalidManualResult = await window.marketConfirmDraftRegistration();
assert.equal(invalidManualResult.validationError, true, "blank handwritten rows must not be registered");
assert.equal(document.querySelector("#marketDraftTableBody tr").classList.contains("market-draft-row-invalid"), true);
document.getElementById("market-basic-category").value = "domestic-retail";
document.getElementById("market-basic-currency").value = "JPY";
document.getElementById("market-basic-auction").value = "";
document.getElementById("market-basic-research-date").value = "2026-08-30";
window.marketApplyBasicInfoToDrafts();
window.marketUpdateDraftRow(0, "brand", "ロレックス");
window.marketUpdateDraftRow(0, "model", "手入力モデル");
window.marketUpdateDraftRow(0, "ref", "MANUAL-001");
const manualMarketMaterial = window.eval("APP_DATA.materials[0].code");
const manualMarketMovement = window.eval("APP_DATA.movements[0].code");
const manualMarketCondition = window.eval("APP_DATA.conditions[0].code");
window.marketUpdateDraftRow(0, "material", manualMarketMaterial);
window.marketUpdateDraftRow(0, "movement", manualMarketMovement);
window.marketUpdateDraftRow(0, "condition", manualMarketCondition);
window.marketUpdateDraftRow(0, "marketPrice", "777,000");
window.marketUpdateDraftRow(0, "sku", "MANUAL-SKU-001");
assert.equal(document.querySelector('[data-market-draft-bracelet-qty="0"]').hidden, true,
  "bracelet quantity must stay hidden until BRACELET PARTS is selected");
window.marketUpdateDraftRow(0, "accessories", ["BOX", "BRACELET PARTS"]);
window.marketRenderDraftRows();
const manualBraceletQuantity = document.querySelector('[data-market-draft-bracelet-qty="0"]');
assert.equal(manualBraceletQuantity.hidden, false,
  "selecting BRACELET PARTS must expose the bracelet quantity input");
const manualBraceletQuantityInput = manualBraceletQuantity.querySelector("input");
manualBraceletQuantityInput.value = "8";
window.marketDraftBraceletQtyChanged(0, manualBraceletQuantityInput);
assert.match(document.querySelector('[data-market-draft-accessory-summary]').textContent, /BRACELET PARTS（8コマ）/u,
  "the selected accessory summary must include the recorded bracelet quantity");
window.marketUpdateDraftRow(0, "warrantyYearMonth", "2026-08");
window.marketUpdateDraftRow(0, "note", "画面から手入力");
const manualMarketResult = await window.marketConfirmDraftRegistration();
assert.equal(manualMarketResult.imported, 1);
assert.equal(marketRows.length, marketCountBeforeManualEntry + 1, "confirmed handwritten rows must be appended to registered market data");
assert.equal(marketRows.at(-1).model, "手入力モデル");
assert.equal(marketRows.at(-1).marketCategory, "domestic-retail");
assert.equal(marketRows.at(-1).auctionCode, "", "auction selection must be optional");
assert.equal(marketRows.at(-1).marketPriceJpy, 777000);
assert.equal(marketRows.at(-1).material, manualMarketMaterial);
assert.equal(marketRows.at(-1).movement, manualMarketMovement);
assert.equal(marketRows.at(-1).condition, manualMarketCondition);
assert.equal(marketRows.at(-1).accessories.join("・"), "BOX・BRACELET PARTS");
assert.equal(marketRows.at(-1).braceletQty, 8, "the bracelet quantity must persist with the registered market row");
window.eval("marketEntryPage = Math.ceil(APP_DATA.marketPrices.length / MARKET_ITEMS_PER_PAGE)");
window.marketRenderEntryTable();
assert.match(document.getElementById("marketEntryTableBody").textContent, /手入力モデル/u,
  "the confirmed handwritten row must appear in the registered list below");
window.eval("marketEntryPage = 1");
window.marketRenderEntryTable();

// 管理者／作業者の画面権限と、ログイン切替をまたぐ承認フロー
window.localStorage.removeItem("inv_approval_workflow_v2");
assert.ok(window.doAppLogin("buyer1", "buyer123"), "worker login must succeed");
assert.equal(window.isWorker(), true);
assert.equal(window.currentRoleLabel(), "作業者");
window.applyRoleUI();
assert.equal(document.querySelector(".sidebar-user div div:last-child").textContent, "作業者");
assert.equal(document.querySelector('.nav-item[data-page="market"]').style.display, "", "worker must see the market table");
assert.equal(document.querySelector('.nav-item[data-page="market-entry"]').style.display, "", "worker must see market registration");
assert.equal(document.querySelector('.nav-item[data-page="approval"]').style.display, "none", "worker must not see approval management");
assert.equal(document.querySelector('.nav-item[data-page="login-info"]'), null, "duplicate login information navigation must remain removed for workers");

window.navigateTo("approval");
assert.equal(document.getElementById("page-approval").classList.contains("hidden"), true, "worker direct navigation to approvals must be blocked");

window.navigateTo("master");
assert.equal(document.getElementById("adminAuthModal").classList.contains("hidden"), false, "worker master access must request admin authentication");
document.getElementById("adminAuthCode").value = window._getLocalAdminAccessCode(false).code;
await window.submitAdminAuth();
assert.equal(document.getElementById("page-master").classList.contains("hidden"), false, "admin authentication must unlock master access once");

window.navigateTo("performance");
assert.equal(document.getElementById("adminAuthModal").classList.contains("hidden"),false,"performance requires a password on access");
const approvalCountBeforePerformance=window.eval("APP_DATA.approvalRequests.length");
document.getElementById("adminAuthCode").value=window._getLocalAdminAccessCode(false).code;
await window.submitAdminAuth();
assert.equal(document.getElementById("page-performance").classList.contains("hidden"),false);
assert.equal(window.eval("APP_DATA.approvalRequests.length"),approvalCountBeforePerformance,"password access must not create an approval request");

window.navigateTo("inventory");
window.execInventorySearch();
const originalWorkerEditModel = inventory[0].model;
const approvedWorkerEditModel = `${originalWorkerEditModel} 作業者申請`;
window.openItemEdit(inventory[0].code);
document.getElementById("ie-model").value = approvedWorkerEditModel;
document.getElementById("ie-editNote").value = "管理者確認用の変更";
window.saveItemEdit();
const workerEditApproval = window.eval("APP_DATA.approvalRequests").at(-1);
assert.equal(workerEditApproval.type, "item_edit");
assert.equal(workerEditApproval.status, "pending");
assert.equal(inventory[0].model, originalWorkerEditModel, "worker edit must wait for admin approval");

assert.ok(window.doAppLogin("admin", "admin123"));
window.applyRoleUI();
assert.notEqual(document.querySelector('.nav-item[data-page="approval"]').style.display, "none", "admin must see approval management");
assert.notEqual(document.querySelector('.nav-item[data-page="master"]').style.display, "none", "admin must manage login credentials from master registration");
assert.equal(document.querySelector('.nav-item[data-page="login-info"]'), null, "admin must not see the removed duplicate navigation");
window.navigateTo("approval");
assert.equal(window.approveRequest(workerEditApproval.id), true);
assert.equal(inventory[0].model, approvedWorkerEditModel, "approval must apply the requested inventory edit");
inventory[0].model = originalWorkerEditModel;
window.replayApprovedApprovalOperations();
assert.equal(inventory[0].model, approvedWorkerEditModel, "approved operations must replay after a page reload");

assert.ok(window.doAppLogin("buyer1", "buyer123"));
window.applyRoleUI();
window.navigateTo("performance");
assert.equal(document.getElementById("adminAuthModal").classList.contains("hidden"),false,"reentry requires authentication again");
document.getElementById("adminAuthCode").value=window._getLocalAdminAccessCode(false).code;
await window.submitAdminAuth();
assert.match(document.getElementById("perf-supplier").textContent, /田中商事/u, "approved worker must be able to view performance data");

const returnRequest = window.createApprovalRequest(
  "setting",
  "表示設定変更",
  { reason: "差戻しテスト" },
  "内容確認をお願いします",
);
assert.ok(window.doAppLogin("admin", "admin123"));
window.applyRoleUI();
assert.equal(window.reviseRequest(returnRequest.id, "対象範囲を追記してください"), true);
assert.equal(returnRequest.status, "revision");
assert.equal(returnRequest.revisionComment, "対象範囲を追記してください");
assert.ok(window.doAppLogin("buyer1", "buyer123"));
window.applyRoleUI();
const reappliedRequest = window.createApprovalRequest(
  "setting",
  "表示設定変更",
  { reason: "対象範囲を追記済み" },
  "修正しました",
  returnRequest.id,
);
assert.equal(reappliedRequest.id, returnRequest.id, "returned requests must keep the same request id");
assert.equal(reappliedRequest.status, "pending");

window.localStorage.removeItem("inv_approval_workflow_v2");

window.clearSession();

const ignoredCallNames = new Set(["if", "for", "while", "switch", "function"]);
const missingHandlerFunctions = new Set();
for (const element of document.querySelectorAll("*")) {
  for (const attribute of element.attributes) {
    if (!attribute.name.startsWith("on")) continue;
    const expression = attribute.value;
    const callPattern = /(^|[^\w$.])([A-Za-z_$][\w$]*)\s*\(/g;
    for (const match of expression.matchAll(callPattern)) {
      const name = match[2];
      if (!ignoredCallNames.has(name) && typeof window[name] !== "function") {
        missingHandlerFunctions.add(name);
      }
    }
  }
}
assert.deepEqual(
  [...missingHandlerFunctions].sort(),
  [],
  `inline handlers reference missing functions: ${[...missingHandlerFunctions].sort().join(", ")}`,
);

assert.equal(document.querySelectorAll(".page-panel").length, 26);
assert.equal(document.querySelectorAll(".nav-item").length, 22);
assert.equal(document.querySelectorAll(".modal-overlay").length, 51);
assert.ok(document.getElementById("purchaseReturnWithdrawOverlay"), "purchase returns require a withdrawal confirmation dialog");
const desktopSidebar = document.getElementById("appSidebar");
const desktopSidebarToggle = document.getElementById("sidebarVisibilityToggle");
assert.ok(desktopSidebar, "desktop sidebar must have a stable controlled element");
assert.ok(desktopSidebarToggle, "desktop topbar must expose a sidebar visibility button");
assert.equal(typeof window.toggleDesktopSidebar, "function");
window.setDesktopSidebarHidden(false);
assert.equal(desktopSidebarToggle.getAttribute("aria-expanded"), "true");
window.toggleDesktopSidebar();
assert.equal(desktopSidebar.classList.contains("sidebar-hidden"), true, "sidebar toggle must completely hide the left navigation");
assert.equal(desktopSidebarToggle.getAttribute("aria-expanded"), "false");
assert.match(desktopSidebarToggle.textContent, /メニューを表示/u, "the remaining button must explain how to restore the sidebar");
window.toggleDesktopSidebar();
assert.equal(desktopSidebar.classList.contains("sidebar-hidden"), false, "the same button must restore the left navigation");
assert.equal(window.localStorage.getItem("inv_admin_sidebar_hidden_v1"), "0", "sidebar visibility must persist across reloads");
const sidebarGroups = [...document.querySelectorAll(".sidebar-nav > .nav-group")];
assert.equal(sidebarGroups[0].querySelector(".nav-group-label").textContent.trim(), "メイン");
const marketNavGroup = document.getElementById("marketNavGroup");
const marketNavToggle = document.getElementById("marketNavToggle");
const marketNavSubmenu = document.getElementById("marketNavSubmenu");
assert.ok(marketNavGroup, "market navigation links must be grouped under one parent");
assert.match(marketNavToggle.textContent, /相場関連/u);
window.navigateTo("dashboard");
assert.equal(marketNavToggle.getAttribute("aria-expanded"), "false");
assert.equal(marketNavSubmenu.hidden, true, "market child links must be collapsed outside market pages");
marketNavToggle.click();
await Promise.resolve();
assert.equal(marketNavToggle.getAttribute("aria-expanded"), "true");
assert.equal(marketNavSubmenu.hidden, false, "clicking the market parent must reveal both child links");
window.syncMarketNavGroup("market-entry");
assert.equal(marketNavToggle.getAttribute("aria-expanded"), "true");
assert.equal(marketNavGroup.classList.contains("has-active"), true,
  "the market parent must stay highlighted while a market child page is active");
const costAdjustmentNavButton = document.getElementById("costAdjustmentNavButton");
assert.ok(costAdjustmentNavButton, "cost adjustment must remain under cost management");
assert.match(costAdjustmentNavButton.textContent, /原価管理/u);
const costManagementNavGroup = document.getElementById("costManagementNavGroup");
const costManagementNavToggle = document.getElementById("costManagementNavToggle");
const costManagementNavSubmenu = document.getElementById("costManagementNavSubmenu");
assert.ok(costManagementNavGroup && costManagementNavToggle && costManagementNavSubmenu);
assert.deepEqual([...costManagementNavSubmenu.querySelectorAll('.nav-item[data-page]')].map(item=>item.dataset.page),
  ["cost-adjustment", "cost-adjustment-slips"]);
assert.doesNotMatch(sidebarGroups[0].textContent, /分解|結合|入れ替え/u,
  "the three removed cost adjustment actions must not remain in the sidebar");
assert.deepEqual(
  [...sidebarGroups[0].querySelectorAll(".nav-item[data-page]")].map((item) => item.dataset.page),
  ["dashboard", "market-entry", "market", "inventory", "purchase-entry", "purchase", "cost-adjustment", "cost-adjustment-slips"],
  "main navigation order must follow the inventory workflow",
);
assert.ok(document.getElementById("page-cost-adjustment"), "cost adjustment must have its own page");
assert.ok(document.getElementById("page-cost-adjustment-slips"), "adjustment slips must have their dedicated page");
assert.deepEqual(
  [...document.querySelectorAll("[data-ca-mode]")].map((button) => button.textContent.trim()),
  ["原価分け", "原価合わせ", "原価調整"],
  "the cost management toolbar must expose split, combine, and adjustment",
);
assert.equal(document.querySelector('[data-ca-mode="swap"]'), null,
  "the removed swap mode must not remain in the cost adjustment toolbar");
assert.equal(typeof window.init_cost_adjustment, "function");
assert.equal(typeof window.costAdjustmentLoadProduct, "function");
assert.equal(typeof window.costAdjustmentAddPart, "function");
assert.equal(typeof window.costAdjustmentDrop, "function");
window.navigateTo("cost-adjustment");
assert.equal(document.getElementById("page-cost-adjustment").classList.contains("hidden"), false);
assert.equal(document.querySelectorAll("#page-cost-adjustment .ca-workbench > .ca-panel").length, 2,
  "cost adjustment must use only the left product and right parts panels");
assert.equal(document.getElementById("ca-stage"), null,
  "the removed dress-up work area must not remain in the page");
assert.equal(document.querySelector("#page-cost-adjustment .ca-attribute-panel"), null,
  "the removed product attributes area must not remain in the page");
assert.equal(document.querySelectorAll("#ca-part-grid .ca-part-slot").length, 20,
  "the parts box must render as twenty positions (4 by 5)");
assert.equal(document.getElementById("ca-start-button").disabled, true,
  "cost adjustment must not start before a product is loaded");
window.eval("APP_DATA.inventory.push({_id:'cost-sold',code:'COST-SOLD',status:'売上済',purchasePrice:999999},{_id:'cost-progress',code:'COST-IN-PROGRESS',status:'原価調整中',purchasePrice:999999});");
document.getElementById("ca-product-code").value = "COST-SOLD";
assert.equal(window.costAdjustmentLoadProduct(), false,
  "sold products must never be loaded into cost management");
assert.match(document.getElementById("ca-product-feedback").textContent, /対象外/u);
document.getElementById("ca-product-code").value = "COST-IN-PROGRESS";
assert.equal(window.costAdjustmentLoadProduct(), false,
  "products already under cost adjustment must not be loaded into another cost-management operation");
window.eval("APP_DATA.inventory[0].status = '在庫中'; APP_DATA.inventory[0].inventoryValuationExcluded = false;");
const costAdjustmentProductCode = window.eval("APP_DATA.inventory[0].code");
document.getElementById("ca-product-code").value = costAdjustmentProductCode;
assert.equal(window.costAdjustmentLoadProduct(), true);
assert.notEqual(document.getElementById("ca-product-brand").textContent, "—",
  "loading a management code must populate the product summary");
assert.deepEqual(
  [...document.querySelectorAll("#ca-product-details .ca-product-detail-secondary dt")].map((label) => label.textContent.trim()),
  ["素材", "ベルト素材", "文字盤", "付属品", "特徴・備考"],
  "the product summary right column must show the requested attribute labels in order",
);
assert.notEqual(document.getElementById("ca-attribute-material").textContent, "—",
  "the product material must be resolved into the loaded product summary");
assert.notEqual(document.getElementById("ca-attribute-accessories").textContent, "—",
  "the loaded product accessories must be shown in the product summary");
assert.equal(document.getElementById("ca-attribute-note").textContent,
  window.eval("String(_costAdjustmentState.product.note || '').trim() || '—'"),
  "the loaded product note must be shown without inventing content");
assert.equal(document.getElementById("ca-start-button").disabled, false,
  "loading a product must enable the cost adjustment start action");
const combineSource = window.eval("_costAdjustmentState.product");
const combineOriginal = {
  status: combineSource.status,
  purchasePrice: combineSource.purchasePrice,
  accessories: [...(combineSource.accessories || [])],
  braceletQty: combineSource.braceletQty,
  note: combineSource.note,
  comment: combineSource.comment,
};
const originalCostAdjustmentParts = window.eval("APP_DATA.parts");
const combinePartCode = "P3108269999";
window.eval(`APP_DATA.parts = [{
  id: 'part_combine_test', partCode: '${combinePartCode}', status: 'in_stock', partName: 'BRACELET PARTS',
  braceletQuantity: 3, fixedCostJpyMinor: 10000, detailText: '', notes: ''
}];`);
window.costAdjustmentSetMode("combine");
assert.equal(document.getElementById("ca-start-button").disabled, true,
  "combine must wait for at least one registered part in the parts box");
assert.equal(window.costAdjustmentAddPart(combinePartCode), true);
assert.equal(document.getElementById("ca-start-button").disabled, false,
  "combine must become startable after both product and part have loaded");
assert.equal(await window.costAdjustmentStart(), true);
assert.equal(window.eval("_costAdjustmentState.combineActive"), true);
assert.equal(window.eval("_costAdjustmentState.product.status"), "原価調整中");
assert.equal(window.eval("_costAdjustmentState.combineDraft.inputParts[0].status"), "cost_adjustment");
const combineDropEvent = {
  dataTransfer: { getData: () => JSON.stringify({ source: "slot", index: 0 }) },
  preventDefault() {},
  currentTarget: document.getElementById("ca-product-summary"),
};
assert.equal(window.costAdjustmentDropPartOnProduct(combineDropEvent), true,
  "dragging a part from the parts box must apply it to the target product");
assert.equal(
  window.eval("_costAdjustmentState.combineDraft.preview.braceletQty"),
  Number(combineOriginal.braceletQty || 0) + 3,
  "BRACELET PARTS must add its link count to the product's existing count",
);
assert.equal(
  window.eval("_costAdjustmentState.combineDraft.preview.purchasePrice"),
  Number(combineOriginal.purchasePrice || 0) + 10000,
  "the combined inventory cost must add the part's fixed JPY cost",
);
assert.match(window.eval("_costAdjustmentState.combineDraft.preview.comment"), new RegExp(combinePartCode));
assert.equal(window.eval("_partInventoryStatusLabel('combined')"), "結合済み",
  "a consumed combine part must be presented as combined in the parts inventory");
assert.match(window.getStatusBadge("結合済み"), /結合済み/u,
  "the combined part status must render as a Japanese status badge");
assert.match(document.getElementById("ca-combine-diff").textContent, /以前の対象商品情報\s*→\s*変更後/u);
assert.equal(document.getElementById("ca-finalize-button").disabled, false,
  "combine confirmation must unlock after every loaded part is applied");
Object.assign(combineSource, combineOriginal);
window.eval("_costAdjustmentState.product = APP_DATA.inventory.find(item => item.code === " + JSON.stringify(costAdjustmentProductCode) + ");");
window.eval("_costAdjustmentState.partSlots = Array(20).fill(null); _costAdjustmentState.stageItems = [];");
window.costAdjustmentResetBreakdownWorkspace();
window.__originalCostAdjustmentParts = originalCostAdjustmentParts;
window.eval("APP_DATA.parts = window.__originalCostAdjustmentParts");
delete window.__originalCostAdjustmentParts;
window.costAdjustmentSetMode("breakdown");
window.costAdjustmentRenderProduct(window.eval("_costAdjustmentState.product"));
assert.equal(window.costAdjustmentSetMode("swap"), "breakdown",
  "a stale swap-mode request must safely fall back to breakdown");
window.costAdjustmentSetMode("breakdown");
assert.equal(await window.costAdjustmentStart(), true,
  "breakdown must start once the management number resolves to a product");
assert.equal(document.getElementById("ca-parts-title").textContent, "原価分け作業スペース");
assert.equal(document.getElementById("ca-product-summary").draggable, true,
  "the full product summary must become draggable after breakdown starts");
assert.equal(document.getElementById("ca-breakdown-drop-guide").getAttribute("role"), "button",
  "the drag target must also provide an accessible activation fallback");
assert.equal(document.getElementById("ca-product-status").textContent, "原価調整中");
assert.equal(window.eval("_costAdjustmentState.product.status"), "原価調整中",
  "starting breakdown must change the loaded product status");
document.getElementById("ca-part-code").value = "PART-TEST-001";
assert.equal(window.costAdjustmentAddPart(), true);
assert.equal(window.eval("_costAdjustmentMove({ source: 'slot', index: 0 }, 'slot', 5)"), true);
assert.match(document.querySelectorAll("#ca-part-grid .ca-part-slot")[5].textContent, /PART-TEST-001/u,
  "parts must remain draggable before the breakdown quantities are confirmed");
const costAdjustmentTransferValues = new Map();
const costAdjustmentDataTransfer = {
  effectAllowed: "",
  dropEffect: "",
  setData: (type, value) => costAdjustmentTransferValues.set(type, value),
  getData: (type) => costAdjustmentTransferValues.get(type) || "",
};
assert.equal(window.costAdjustmentDragProduct({ dataTransfer: costAdjustmentDataTransfer, preventDefault() {} }), true);
assert.equal(window.costAdjustmentDropProduct({
  dataTransfer: costAdjustmentDataTransfer,
  preventDefault() {},
  stopPropagation() {},
}), true, "dropping the full product summary into the breakdown workspace must open its modal");
assert.equal(document.getElementById("costAdjustmentBreakdownModal").classList.contains("hidden"), false,
  "dropping the product must be able to open the quantity modal");
document.getElementById("ca-breakdown-product-count").value = "21";
assert.equal(window.costAdjustmentConfirmBreakdown(), false,
  "breakdown quantities must not exceed the twenty available slots");
assert.match(document.getElementById("ca-breakdown-modal-error").textContent, /20個以内/u);
document.getElementById("ca-breakdown-product-count").value = "4";
window.eval("_costAdjustmentState.breakdownStartedDate = '2026-09-11'");
assert.equal(window.costAdjustmentConfirmBreakdown(), true);
assert.equal(window.eval("_costAdjustmentState.breakdownDraft.adjustmentDate"), '2026-09-11');
assert.equal(window.eval("_costAdjustmentState.breakdownDraft.items.every(item => item.managementCode.startsWith('110926'))"), true);
assert.equal(window.eval("_costAdjustmentState.breakdownDraft.items.every(item => item.sku === '')"), true,
  "split outputs must start with an empty SKU");
assert.equal(window.eval(`_costAdjustmentState.breakdownDraft.items.every(item => {
  const product = item.product;
  return product.brandCode === '' && product.modelName === '' && product.referenceNumber === ''
    && product.serialNumber === '' && product.materialCode === '' && product.movementCode === ''
    && product.conditionCode === '' && product.beltMaterial === '' && product.dial === ''
    && product.accessories.length === 0 && product.salePriceUsdMinor === 0 && product.notes === '';
})`), true, "split output details must not inherit the source product data");
assert.equal(window.eval("_costAdjustmentState.breakdownDraft.items.every(item => item.product.internalComment === `原価管理番号: ${_costAdjustmentState.product.code}`)"), true,
  "split outputs must keep only the source cost management number as an audit comment");
window.costAdjustmentOpenItemEditor(0);
assert.match(document.querySelector('label[for="ca-item-comment"]').textContent, /原価管理番号/u);
assert.match(document.querySelector('label[for="ca-item-comment"] + textarea + small').textContent, /調整伝票番号/u);
assert.equal(document.getElementById("ca-item-sku").value, "");
assert.equal(document.getElementById("ca-edit-product-brand").value, "");
assert.equal(document.getElementById("ca-edit-product-model").value, "");
assert.equal(document.getElementById("ca-edit-product-ref").value, "");
assert.equal(document.getElementById("ca-edit-product-condition").value, "");
assert.equal(document.querySelectorAll('#ca-edit-product-accessories input:checked').length, 0);
assert.equal(document.getElementById("ca-item-notes").value, "");
document.getElementById("ca-edit-product-brand").value = document.getElementById("ca-edit-product-brand").options[1].value;
document.getElementById("ca-item-cost").value = "123000";
const gramsField = document.getElementById('ca-edit-product-grams');
assert.equal(gramsField.closest('.form-row').classList.contains('cols-3'),true);
assert.equal(gramsField.closest('.form-group').previousElementSibling.querySelector('.checkbox-group').id,'ca-edit-product-accessories');
assert.equal(gramsField.closest('.form-group').nextElementSibling.querySelector('input').id,'ca-edit-product-sale');
gramsField.value='-1';assert.equal(window.costAdjustmentCompleteItemEdit(),false);
gramsField.value='123.45';
assert.equal(window.costAdjustmentCompleteItemEdit(), true);
window.costAdjustmentOpenItemEditor(0);
assert.equal(document.getElementById('ca-edit-product-grams').value,'123.45');
document.getElementById('ca-edit-product-grams').value='';assert.equal(window.costAdjustmentCompleteItemEdit(),true);
window.costAdjustmentOpenItemEditor(0);assert.equal(document.getElementById('ca-edit-product-grams').value,'');
document.getElementById('ca-edit-product-grams').value='123.45';assert.equal(window.costAdjustmentCompleteItemEdit(),true);
assert.equal(document.querySelector("#ca-split-slip-lines .ca-split-cost-input").value, "123000",
  "a cost entered in the item editor must immediately appear in the matching adjustment-slip line");
assert.match(document.getElementById("ca-breakdown-result").textContent, /分裂後の商品\s*4\s*個/u);
assert.equal(document.querySelectorAll("#ca-part-grid .ca-breakdown-product-slot").length, 4,
  "confirmed product quantities must occupy red breakdown slots");
assert.equal(document.querySelectorAll("#ca-part-grid .ca-breakdown-part-slot").length, 0,
  "cost splitting must not create part slots");
assert.equal(document.querySelectorAll("#ca-part-grid .ca-part-slot-disabled").length, 16,
  "all remaining breakdown slots must be greyed out");
assert.equal(document.querySelectorAll("#ca-part-grid .ca-part-slot-disabled[aria-disabled='true']").length, 16,
  "greyed out slots must be exposed as disabled to assistive technology");
assert.equal(document.getElementById("ca-split-slip").hidden, false,
  "the cost split slip must appear directly below the BOX after quantities are confirmed");
assert.match(document.getElementById("ca-split-slip-title").textContent, /原価調整伝票/u);
assert.match(document.getElementById("ca-split-slip").textContent, /備考：原価分け/u);
assert.equal(document.querySelectorAll("#ca-split-slip-lines .ca-split-cost-input").length, 4,
  "the cost split slip must provide one editable amount for every generated product");
assert.equal(document.querySelectorAll("#ca-split-slip-lines .ca-split-source-row").length, 1,
  "the adjustment slip preview must include the source product");
assert.match(document.querySelector("#ca-split-slip-lines .ca-split-source-row").textContent, /対象商品（原価分け前）/u);
assert.match(document.querySelector("#ca-split-slip-lines .ca-split-source-row").textContent, /¥0/u,
  "the source product must be shown at zero yen on the adjustment slip");
assert.equal(document.getElementById("ca-part-code").disabled, true,
  "part management number input must be disabled after breakdown confirmation");
assert.equal(document.querySelector("#ca-parts-panel .ca-add-part-button").disabled, true,
  "the part add action must be disabled after breakdown confirmation");
assert.equal(document.querySelector('label[for="ca-part-code"]').textContent.trim(), "管理番号検索");
assert.equal(window.costAdjustmentAddPart("PART-LOCKED-001"), false,
  "confirmed breakdown boxes must reject further part entry");
assert.equal(window.eval("_costAdjustmentMove({ source: 'slot', index: 0 }, 'slot', 5)"), false,
  "confirmed breakdown boxes must reject drag movement");
for (let index = 0; index < 4; index += 1) {
  window.costAdjustmentUpdateSplitLineAmount(index, "1");
}
assert.equal(document.getElementById("ca-finalize-button").disabled, true,
  "cost split confirmation must remain disabled while the allocated total differs from the source cost");
assert.match(document.getElementById("ca-finalize-button").title, /編集/u,
  "blank split outputs must explain that product editing is still required");
const costSplitTarget = window.eval("_costAdjustmentState.breakdownDraft.targetCostJpyMinor");
const sourceHistoricalCost = window.eval("_costAdjustmentState.product.purchasePrice");
window.costAdjustmentUpdateSplitLineAmount(0, String(costSplitTarget - 3));
assert.equal(document.getElementById("ca-finalize-button").disabled, true,
  "cost split confirmation must remain disabled until every blank output has been edited");
window.eval("_costAdjustmentState.breakdownDraft.items.forEach(item => { item.completed = true; })");
window.costAdjustmentRenderAllocationSummary();
window.costAdjustmentRenderSplitSlip();
assert.equal(document.getElementById("ca-finalize-button").disabled, false,
  "cost split confirmation must unlock only when every item is complete and the cost total matches exactly");
assert.equal(document.getElementById("ca-finalize-button").getAttribute("aria-disabled"), "false");
assert.match(document.getElementById("ca-allocation-balance").textContent, /完全一致/u);
assert.match(document.getElementById("ca-split-slip-status").textContent, /元商品の原価と完全一致/u);
const originalZaikoAPI = window.ZaikoAPI;
window.ZaikoAPI = window.ZaikoAPI || {};
const originalConfirmProductCostAdjustment = window.ZaikoAPI.confirmProductCostAdjustment;
window.ZaikoAPI.confirmProductCostAdjustment = async (_source, payload) => { assert.equal(payload.outputs[0].product.weightGrams, "123.45"); return ({
  id: "cad-test-1",
  slipNumber: "CA-2026-0001",
  adjustmentDate: "2026-08-31",
  sourceProductCode: costAdjustmentProductCode,
  sourceStatus: "cost_adjustment",
  outputs: window.eval("_costAdjustmentState.breakdownDraft.items.map(item => ({ position: item.position, managementCode: `3108269${String(item.position).padStart(3, '0')}` }))"),
}); };
const confirmedSplitDraft = window.eval("_costAdjustmentState.breakdownDraft");
const confirmedSplitSource = window.eval("_costAdjustmentState.product");
const successfulSplitConfirm = window.ZaikoAPI.confirmProductCostAdjustment;
window.ZaikoAPI.confirmProductCostAdjustment = async () => { throw new Error("保存失敗テスト"); };
assert.equal(await window.costAdjustmentFinalize(), false);
assert.equal(window.eval("_costAdjustmentState.breakdownDraft"), confirmedSplitDraft, "failed confirmation must preserve the draft");
assert.equal(window.eval("_costAdjustmentState.product"), confirmedSplitSource);
window.ZaikoAPI.confirmProductCostAdjustment = successfulSplitConfirm;
assert.equal(await window.costAdjustmentFinalize(), true, "an exactly allocated cost split must be confirmed");
assert.equal(confirmedSplitDraft.items.every(item => item.product.internalComment.includes('調整伝票番号: CA-2026-0001')), true);
assert.equal(confirmedSplitSource.purchasePrice, sourceHistoricalCost);
assert.equal(confirmedSplitSource.inventoryValuationExcluded, true);
assert.equal(confirmedSplitSource.status, "原価調整中", "clearing the UI must not undo confirmation");
assert.equal(window.eval("_costAdjustmentState.product"), null);
assert.equal(window.eval("_costAdjustmentState.breakdownDraft"), null);
assert.equal(window.eval("_costAdjustmentState.breakdownActive"), false);
assert.equal(window.eval("_costAdjustmentState.partSlots.every(slot => slot === null)"), true);
assert.equal(document.getElementById("ca-product-code").value, "");
assert.equal(document.getElementById("ca-product-details").hidden, true);
assert.equal(document.getElementById("ca-product-placeholder").hidden, false);
assert.equal(document.getElementById("ca-split-slip").hidden, true);
assert.equal(document.getElementById("ca-split-slip-lines").children.length, 0);
assert.equal(document.getElementById("ca-split-slip-total").textContent, "¥0");
assert.equal(document.getElementById("ca-finalize-button").disabled, true);
assert.equal(await window.costAdjustmentFinalize(), false, "the cleared workspace must not submit again");
if (originalZaikoAPI) window.ZaikoAPI.confirmProductCostAdjustment = originalConfirmProductCostAdjustment;
else delete window.ZaikoAPI;
assert.equal(sidebarGroups[1].querySelector(".nav-group-label").textContent.trim(), "経理・会計");
assert.deepEqual(
  [...sidebarGroups[1].querySelectorAll(".nav-item[data-page]")].map((item) => item.dataset.page),
  ["sales-list", "deleted-slips", "sales", "shipping", "consignment"],
  "accounting navigation must start with the document list",
);
assert.equal(document.getElementById("adjustmentEntryNavGroup"), null,
  "the retired adjustment registration hierarchy must not appear in the sidebar");
assert.equal(document.querySelector('.nav-item[data-page="sales-adjustment-entry"]'), null);
assert.equal(document.querySelector('.nav-item[data-page="purchase-adjustment-entry"]'), null);
window.navigateTo("sales-adjustment-entry");
assert.equal(document.getElementById("page-sales-adjustment-entry").classList.contains("hidden"), false);
assert.equal(document.getElementById("pageTitle").textContent, "売上調整登録");
for (const controlId of [
  "sae-part-code", "sae-product-code", "sae-load-button", "sae-target-result", "sae-linked-slip",
  "sae-reason", "sae-amount", "sae-amount-preview", "sae-register-button",
]) {
  assert.ok(document.getElementById(controlId), `sales adjustment entry must include ${controlId}`);
}
assert.equal(typeof window.loadSalesAdjustmentTarget, "function");
assert.equal(typeof window.applySalesAdjustmentScannedCode, "function");
assert.equal(typeof window.registerSalesAdjustment, "function");
const salesAdjustmentCountBefore = window.eval("APP_DATA.salesAdjustments.length");
document.getElementById("sae-product-code").value = "0503260001";
window.salesAdjustmentHandleCodeInput("product");
assert.equal(window.loadSalesAdjustmentTarget(), true,
  "a sold product must load into sales adjustment entry");
assert.match(document.getElementById("sae-target-result").textContent, /0503260001/u);
assert.match(document.getElementById("sae-target-result").textContent, /売上済/u);
assert.match(document.getElementById("sae-linked-slip").textContent, /売上伝票/u);
assert.match(document.getElementById("sae-linked-slip").textContent, /SL-2026-0001/u,
  "the loaded target must resolve its originating sales slip");
document.getElementById("sae-reason").value = "成約後の価格調整";
document.getElementById("sae-amount").value = "100";
window.updateSalesAdjustmentEntryState();
assert.equal(document.getElementById("sae-register-button").disabled, false,
  "a loaded target, required reason, adjustment type and amount must enable registration");
assert.match(document.getElementById("sae-amount-preview").textContent, /調整後売価/u);
const registeredSalesAdjustment = window.registerSalesAdjustment();
assert.ok(registeredSalesAdjustment, "registration must create a sales adjustment slip");
assert.match(registeredSalesAdjustment.id, /^SS-\d{9}$/u);
assert.equal(registeredSalesAdjustment.sourceSlipId, "SL-2026-0001");
assert.equal(registeredSalesAdjustment.targetCode, "0503260001");
assert.equal(registeredSalesAdjustment.adjustmentType, "discount");
assert.equal(registeredSalesAdjustment.adjustmentAmount, -100);
assert.equal(registeredSalesAdjustment.reason, "成約後の価格調整");
assert.equal(window.eval("APP_DATA.salesAdjustments.length"), salesAdjustmentCountBefore + 1);
const persistedBusinessState = JSON.parse(window.localStorage.getItem("inv_business_workflow_v1"));
assert.equal(persistedBusinessState.salesAdjustments.at(-1).id, registeredSalesAdjustment.id,
  "created adjustment slips must survive page reload through business workflow persistence");
window.navigateTo("sales-list");
window.navigateTo("cost-adjustment-slips");
window.showAllSlipList();
assert.match(document.getElementById("slipListBody").textContent, new RegExp(registeredSalesAdjustment.id),
  "the created sales adjustment slip must appear in the unified adjustment document tab");
assert.match(document.getElementById("slipListBody").textContent, /売上調整/u);

window.navigateTo("sales-adjustment-entry");
window.eval(`APP_DATA.inventory.push({
  code: "SAE-INELIGIBLE-001", brand: "テスト", model: "対象外", status: "在庫中", salePrice: 1000
})`);
document.getElementById("sae-product-code").value = "SAE-INELIGIBLE-001";
window.salesAdjustmentHandleCodeInput("product");
assert.equal(window.loadSalesAdjustmentTarget(), false,
  "an in-stock product must not be displayed in sales adjustment entry");
assert.match(document.getElementById("sae-target-result").textContent, /まだ読み込まれていません/u);
window.eval("APP_DATA.inventory.pop()");

window.eval(`APP_DATA.parts.push({
  partCode: "P-SAE-0001", partName: "BOX", brandName: "ロレックス", modelName: "Submariner",
  status: "consigned", salePriceUsdMinor: 250
}); APP_DATA.consignments.push({
  id: "CO-SAE-0001", date: "2026-08-31", destination: "B001", status: "処理済",
  items: [{ code: "P-SAE-0001", brand: "ロレックス", model: "Submariner", salePrice: 250 }]
})`);
const scannedSalesAdjustmentPart = window.applySalesAdjustmentScannedCode("P-SAE-0001");
assert.equal(scannedSalesAdjustmentPart.ok, true,
  "sales adjustment QR input must recognize an eligible part management number");
assert.equal(document.getElementById("sae-part-code").value, "P-SAE-0001");
assert.equal(document.getElementById("sae-product-code").value, "");
assert.equal(window.loadSalesAdjustmentTarget(), true,
  "an eligible consigned part must load from the part management number field");
assert.match(document.getElementById("sae-linked-slip").textContent, /委託伝票/u);
assert.match(document.getElementById("sae-linked-slip").textContent, /CO-SAE-0001/u);
window.eval("APP_DATA.parts.pop(); APP_DATA.consignments.pop()");
window.navigateTo("purchase-adjustment-entry");
assert.equal(document.getElementById("page-purchase-adjustment-entry").classList.contains("hidden"), false);
assert.equal(document.getElementById("pageTitle").textContent, "仕入調整登録");
assert.ok(document.getElementById("page-deleted-slips"), "deleted document archive page must be present");
assert.ok(document.getElementById("deletedSlipListBody"), "deleted document archive table must be present");
assert.ok(document.getElementById("page-consignment"), "consignment registration page must be present");
assert.equal(
  document.querySelector('.nav-item[data-page="consignment"]').textContent.trim(),
  "委託登録",
  "consignment navigation must use the shortened label",
);
assert.match(staticAppSource, /'consignment': '委託登録'/u,
  "consignment page title must use the shortened label");
assert.equal(staticAppSource.includes("委託伝票登録"), false,
  "the former consignment registration page label must not remain");
assert.ok(document.getElementById("sltab-consignment"), "document list must include the consignment tab");
assert.equal(document.getElementById("sltab-adjustment"), null, "document list must not include adjustment slips");
assert.equal(document.getElementById("sltab-purchaseadjustment"), null);
assert.equal(document.getElementById("sltab-salesadjustment"), null);
window.eval(`APP_DATA.costAdjustments = [{
  id: "CA-2026-0001", date: "2026-08-31", adjustmentType: "breakdown",
  sourceProductCode: "3108260010", sourceBrand: "ロレックス", sourceModel: "Classic Watch",
  sourceCostJpyMinor: 350000, adjustedCostJpyMinor: 350000, status: "処理済", note: "原価分け",
  lines: [{ position: 1, kind: "product", code: "3108260011", allocatedCostJpyMinor: 350000 }]
}]`);
window.navigateTo("cost-adjustment-slips");
window.showAllSlipList();
assert.match(document.getElementById("slipListBody").textContent, /原価調整（原価分け）/u,
  "unified adjustment list must render the cost-adjustment slip pattern");
assert.match(document.getElementById("slipListBody").textContent, /CA-2026-0001/u);
window.openCostAdjustmentSlipDetail("CA-2026-0001");
assert.match(document.getElementById("slipDetailTitle").textContent, /原価調整伝票/u);
assert.match(document.getElementById("slipDetailBody").textContent, /調整前原価/u);
assert.match(document.getElementById("slipDetailFooter").textContent, /伝票をプレビューで見る/u);
window.openSavedSlipDocument("costadjustment", "CA-2026-0001");
assert.equal(document.getElementById("savedSlipDocumentModal").classList.contains("hidden"), false);
assert.match(document.getElementById("savedSlipDocumentTitle").textContent, /原価調整伝票プレビュー/u);
assert.match(document.getElementById("savedSlipDocumentPrintArea").textContent, /3108260011/u);
window.closeSavedSlipDocument();
window.closeSlipDetail();
window.eval("APP_DATA.costAdjustments = []");
const shipmentDetailTable = window.buildItemsTable(
  [{ code: "0408260004", brand: "グランドセイコー", model: "Heritage Collection", salePrice: 5975 }],
  "shipping",
  { currency: "USD", rate: 155.25 },
);
assert.match(shipmentDetailTable, /現在ステータス/u, "shipment detail must show each product's current status");
assert.match(
  window.buildSlipDetailFooter("shipping", { _id: "shp_test", id: "SH-TEST", revisions: [] }),
  /返却QRスキャン/u,
  "shipment detail footer must expose the return QR scanner",
);
assert.equal(typeof window.openShipmentReturnScanner, "function");
assert.equal(
  window.eval("getLocalDateISO(new Date(2026, 7, 3, 1, 0, 0))"),
  "2026-08-03",
  "document defaults must use the browser's local calendar date instead of UTC",
);

assert.ok(document.getElementById("registered-sales-slip-list"), "sales entry must include the registered sales slip list");
assert.equal(typeof window.renderRegisteredSalesSlips, "function");
window.renderRegisteredSalesSlips();
assert.equal(
  document.querySelectorAll("#registered-sales-list-body tr").length,
  window.eval("APP_DATA.sales.length"),
  "registered sales list must render every hydrated sales slip",
);
assert.match(document.getElementById("registered-sales-list-count").textContent, /伝票/u);

assert.ok(document.getElementById("registered-shipping-slip-list"), "shipping entry must include the registered shipping slip list");
assert.equal(typeof window.renderRegisteredShippingSlips, "function");
window.renderRegisteredShippingSlips();
assert.equal(
  document.querySelectorAll("#registered-shipping-list-body tr").length,
  window.eval("APP_DATA.shipments.length"),
  "registered shipping list must render every hydrated shipping slip",
);
assert.match(document.getElementById("registered-shipping-list-count").textContent, /伝票/u);

assert.ok(document.getElementById("registered-consignment-slip-list"), "consignment entry must include the registered consignment slip list");
assert.equal(typeof window.renderRegisteredConsignmentSlips, "function");
window.renderRegisteredConsignmentSlips();
assert.equal(
  document.querySelectorAll("#registered-consignment-list-body tr").length,
  window.eval("APP_DATA.consignments.length"),
  "registered consignment list must render every hydrated consignment slip",
);
assert.match(document.getElementById("registered-consignment-list-count").textContent, /伝票/u);

assert.equal(typeof window._renderFxRateTrendCharts, "function");
const fxTrendMarkup = window._renderFxRateTrendCharts([
  { code: "USD", rate: 150.25, observedAt: "2026-08-16T09:00:00+09:00" },
  { code: "USD", rate: 155.25, observedAt: "2026-08-18T09:00:00+09:00" },
  { code: "HKD", rate: 19.8, observedAt: "2026-08-18T09:00:00+09:00" },
  { code: "EUR", rate: 181.5, observedAt: "2026-08-18T09:00:00+09:00" },
]);
assert.match(fxTrendMarkup, /USD \/ 米ドル/u);
assert.match(fxTrendMarkup, /HKD \/ 香港ドル/u);
assert.match(fxTrendMarkup, /EUR \/ ユーロ/u);
assert.match(fxTrendMarkup, /<polyline/u, "rate history must render as a line chart");
assert.match(fxTrendMarkup, /role="img"/u, "rate chart must expose an accessible image role");
const fxRateArea = document.createElement("div");
window.renderFxRateTab(fxRateArea);
assert.equal(fxRateArea.querySelectorAll(".fx-trend-card").length, 3, "foreign exchange tab must show three currency charts");
assert.ok(fxRateArea.querySelector(".fx-history-table"), "numeric rate history must remain available below the charts");
window.eval(`APP_DATA.fxRateSync = {enabled:true,state:{publishedDate:'2026-10-15',lastSuccessAt:'2026-10-15T03:00:00Z',lastError:'取得失敗'}}`);
window.renderFxRateTab(fxRateArea);
assert.ok(fxRateArea.querySelector('#smbc-rate-status'), 'automatic rates must expose source and fetch status');
assert.equal(fxRateArea.querySelectorAll('.fx-rate-input').length, 0, 'SMBC mode must not show manual rate inputs');
assert.match(fxRateArea.textContent, /SMBC仲値を自動取得/u);
assert.match(fxRateArea.textContent, /1％以上/u);
assert.match(fxRateArea.textContent, /2026-10-15/u);
assert.ok(fxRateArea.querySelector('[role="alert"]'), 'failed sync must be visible');
window.eval('delete APP_DATA.fxRateSync');


window.eval(await readFile(path.join(referenceRoot, 'js', 'cost_merge.js'), 'utf8'));
window.eval(`_costAdjustmentState.breakdownActive=false; _costAdjustmentState.combineActive=false;
  _costAdjustmentState.breakdownDraft=null; _costAdjustmentState.combineDraft=null;
  APP_DATA.inventory.push({_id:'merge-a',code:'MERGE-A',status:'在庫中',purchasePrice:100,salePrice:250,sku:'A',internalComment:'調整伝票番号: CA-2026-0001'},
    {_id:'merge-b',code:'MERGE-B',status:'原価調整中',purchasePrice:200,sku:'B',internalComment:'原価調整伝票番号: CA-2026-0001'});`);
window.costAdjustmentSetMode('combine');
assert.equal(window.costAdjustmentAddPart('COST-SOLD'), false,
  'sold products must not be added to the cost merge BOX');
assert.equal(window.costAdjustmentAddPart('MERGE-A'), true);
assert.equal(document.getElementById('ca-start-button').disabled, true);
assert.equal(window.costAdjustmentAddPart('MERGE-A'), false);
assert.equal(window.costAdjustmentAddPart('MERGE-B'), false,
  'products already under cost adjustment must not be added to a cost merge BOX');
assert.equal(document.getElementById('ca-start-button').disabled, true);
window.eval("APP_DATA.inventory.find(p=>p.code==='MERGE-B').status='在庫中'");
assert.equal(window.costAdjustmentAddPart('MERGE-B'), true);
assert.match(document.getElementById('ca-parts-subtitle').textContent,/在庫中の商品/u);
assert.equal(document.getElementById('ca-start-button').disabled, false);
assert.equal(document.querySelectorAll('#ca-split-slip-lines tr').length, 2);
assert.match(document.getElementById('ca-split-slip-lines').textContent,/履歴原価・現在在庫集計対象外/u);
let mergeConfirmCount=0;
let costStartResetCount=0;
window.ZaikoAPI={hydrateAdmin:async()=>{},request:async(path,options)=>{
 if(path==='/cost-merges') {
  assert.deepEqual(JSON.parse(options.body).productIds,['merge-a','merge-b']);
  return {id:'cmg-test',managementCode:'1209260001',businessDate:'2026-09-12'};
 }
 mergeConfirmCount++;
 const body=JSON.parse(options.body);
 assert.equal(body.output.allocatedCostJpyMinor,300);
 assert.equal(body.output.product.modelNumber,'Merged watch');
 return {slipNumber:'CA-2026-0099'};
},startProductCostAdjustment:async(item,options)=>{
 assert.equal(options.mode,'adjustment');
 const product=window.eval('APP_DATA.inventory').find(product=>product._id===item._id);
 product.status='原価調整中';
 product.comment=[product.comment,`もともとの想定売価: ${window.formatSalePrice(product.salePrice||0)}`].filter(Boolean).join('\n');
 product.salePrice=0;
 return product;
},resetCostAdjustmentStart:async(items)=>{
 costStartResetCount++;
 for (const item of items) {
  const product=window.eval('APP_DATA.inventory').find(product=>product._id===item._id);
  if (!product) continue;
  product.status='在庫中';
  product.salePrice=250;
  product.comment=String(product.comment||'').split('\n').filter(line=>!line.startsWith('もともとの想定売価:')).join('\n');
 }
 return {mode:items.length > 1 ? 'combine' : 'adjustment',products:items};
}};
assert.equal(await window.costAdjustmentStart(),true);
assert.equal(document.getElementById('ca-reset-button').hidden,false,
  'a started cost merge must expose the current-operation reset action');
assert.equal(window.eval("APP_DATA.inventory.find(p=>p.code==='MERGE-A').status"),'原価調整中');
assert.equal(window.eval("APP_DATA.inventory.find(p=>p.code==='MERGE-B').status"),'原価調整中');
assert.equal(window.eval("_costAdjustmentState.breakdownDraft.inputs.map(p=>p.status).join(',')"),'原価調整中,原価調整中',
  'starting cost merge must immediately mark every source product as cost adjustment in progress');
assert.match(document.getElementById('ca-product-feedback').textContent,/1209260001/u);
assert.equal(document.querySelectorAll('#ca-split-slip-lines tr').length,3);
assert.equal(document.querySelector('#ca-split-slip-lines .ca-split-cost-input'),null,
  'cost merge output must not expose an editable cost input');
assert.match(document.querySelector('#ca-split-slip-lines .ca-merge-auto-cost').textContent,/¥300/u,
  'cost merge output must show the sum of BOX source costs');
assert.equal(window.costMergeEdit(),true);
assert.equal(document.getElementById('ca-item-cost').readOnly,true,
  'cost merge editor must keep the automatically summed cost read-only');
assert.equal(document.getElementById('ca-item-comment-label').textContent.includes('原価管理番号'),false,
  'cost merge editor must not show a cost management number');
assert.match(document.getElementById('ca-item-comment').value,/調整伝票番号: CA-2026-0001/u,
  'cost merge editor must retain the source cost-adjustment slip number');
assert.match(document.getElementById('ca-item-comment').value,/原価合わせ元: MERGE-A, MERGE-B/u);
assert.equal(document.getElementById('ca-item-comment').value.includes('原価管理番号:'),false);
document.getElementById('ca-edit-product-brand').value=document.getElementById('ca-edit-product-brand').options[1].value;
document.getElementById('ca-edit-product-model').value='Merged watch';
assert.equal(window.costAdjustmentCompleteItemEdit(),true);
window.costMergeAmount('299');
assert.equal(window.eval('_costAdjustmentState.breakdownDraft.items[0].allocatedCostJpyMinor'),300,
  'cost merge amount must ignore attempted overrides and stay equal to the BOX total');
assert.equal(document.getElementById('ca-finalize-button').disabled,false);
assert.equal(await window.costAdjustmentFinalize(),true);
assert.equal(await window.costAdjustmentFinalize(),false);
assert.equal(mergeConfirmCount,1);
assert.match(document.getElementById('ca-split-slip-title').textContent,/原価調整済み/u);
assert.equal(window.eval("_costAdjustmentState.breakdownDraft.items[0].product.internalComment.includes('原価管理番号:')"),false);
assert.equal(window.eval("(_costAdjustmentState.breakdownDraft.items[0].product.internalComment.match(/CA-2026-0001/g)||[]).length"),1,
  'duplicate source adjustment slip numbers must be accumulated only once');
assert.match(window.eval("_costAdjustmentState.breakdownDraft.items[0].product.internalComment"),/調整伝票番号: CA-2026-0001[\s\S]*調整伝票番号: CA-2026-0099/u);
assert.match(window.eval("_costAdjustmentState.breakdownDraft.items[0].product.internalComment"),/調整伝票番号: CA-2026-0099/u);
window.eval(await readFile(path.join(referenceRoot, 'js', 'cost_adjustment_entry.js'), 'utf8'));
window.eval('_costAdjustmentState.combineActive=false; _costAdjustmentState.breakdownActive=false;');
window.costAdjustmentSetMode('adjustment');
assert.equal(document.getElementById('ca-direct-adjustment').hidden,false);
assert.equal(document.querySelector('#page-cost-adjustment .ca-workbench').classList.contains('ca-direct-hidden'),true);
assert.equal(document.querySelector('.ca-direct-slip-table').classList.contains('purchase-slip-detail-table'),true,
  'the cost adjustment slip must reuse the purchase/sales slip detail-table presentation');
assert.match(document.querySelector('.ca-direct-slip .sdi-heading').textContent,/明細一覧/u);
assert.deepEqual([...document.querySelectorAll('.ca-direct-slip-table thead th')].map(th=>th.textContent.trim()),
  ['No.','商品コード','SKU','ブランド','型番／モデル','ステータス','現在原価','今回調整','調整後原価','備考','操作']);
assert.equal(document.getElementById('ca-direct-start').disabled,true,
  'direct cost adjustment start must stay disabled until a management number is entered');
document.getElementById('ca-direct-code').value='COST-SOLD';
window.costDirectCodeChanged();
assert.equal(document.getElementById('ca-direct-start').disabled,false);
assert.equal(await window.costDirectStart(),false,
  'sold products must not be added to a direct cost adjustment slip');
document.getElementById('ca-direct-code').value='COST-IN-PROGRESS';
assert.equal(await window.costDirectStart(),false,
  'products already under cost adjustment must not be added to a direct cost adjustment slip');
document.getElementById('ca-direct-code').value='MERGE-A';
assert.equal(await window.costDirectStart(),false,
  'a product started by another cost-management operation must be rejected');
window.eval("const p=APP_DATA.inventory.find(p=>p.code==='MERGE-A'); p.status='在庫中'; p.note='既存の備考'");
document.getElementById('ca-direct-code').value='MERGE-A';
assert.equal(await window.costDirectStart(),true);
assert.equal(window.eval("APP_DATA.inventory.find(p=>p.code==='MERGE-A').salePrice"),0,
  'starting direct cost adjustment must clear the planned sale price');
assert.match(window.eval("APP_DATA.inventory.find(p=>p.code==='MERGE-A').comment"),/もともとの想定売価:/u,
  'starting direct cost adjustment must preserve the former sale price in the internal comment');
assert.equal(document.getElementById('ca-direct-start').disabled,true,
  'direct cost adjustment must not accept a second product after starting');
assert.equal(document.getElementById('ca-direct-code').disabled,true);
assert.equal(document.querySelectorAll('#ca-direct-lines tr').length,1,
  'direct cost adjustment must contain exactly one product line');
assert.equal(document.getElementById('ca-direct-reset').hidden,false,
  'a started direct adjustment must expose the current-operation reset action');
assert.equal(await window.costDirectReset(),true);
assert.equal(costStartResetCount,1);
assert.equal(window.eval("APP_DATA.inventory.find(p=>p.code==='MERGE-A').status"),'在庫中');
assert.equal(window.eval("APP_DATA.inventory.find(p=>p.code==='MERGE-A').salePrice"),250);
assert.equal(document.getElementById('ca-direct-reset').hidden,true);
document.getElementById('ca-direct-code').value='MERGE-A';
window.costDirectCodeChanged();
assert.equal(await window.costDirectStart(),true,
  'the restored product can be started again with the corrected operation');
assert.equal(document.getElementById('ca-direct-reason').required,true);
assert.equal(document.getElementById('ca-direct-global-amount').required,true);
assert.match(document.querySelector('label[for="ca-direct-global-amount"] .ca-direct-field-label').textContent,/調整金額（税抜き）/u);
assert.equal(document.getElementById('ca-direct-register').disabled,true,
  'reason and adjustment amount must be required before issuing the slip');
document.getElementById('ca-direct-global-amount').value='50000';
document.getElementById('ca-direct-global-amount').dispatchEvent(new window.Event('input',{bubbles:true}));
assert.equal(document.getElementById('ca-direct-global-amount').value,'50,000',
  'direct cost adjustment amount must be formatted with thousands separators while typing');
document.getElementById('ca-direct-code').value='MERGE-A';
assert.equal(await window.costDirectStart(),false);
window.costDirectReason('手入力メモ');
window.costDirectGlobalAmount('50');
window.costDirectDirection('減額');
assert.equal(document.getElementById('ca-direct-reason').value,'手入力メモ');
assert.equal(document.getElementById('ca-direct-amount-sign').textContent,'−');
assert.match(document.getElementById('ca-direct-lines').textContent,/¥100\s*−\s*¥50/u);
assert.match(document.getElementById('ca-direct-lines').textContent,/¥50/u);
assert.match(document.getElementById('ca-direct-lines').textContent,/減額｜理由：手入力メモ｜金額：−¥50/u);
assert.match(document.getElementById('ca-direct-totals').textContent,/合計/u);
assert.match(document.getElementById('ca-direct-totals').textContent,/−¥50/u);
assert.equal(document.getElementById('ca-direct-register').disabled, false);
const mergeRequest = window.ZaikoAPI.request;
window.ZaikoAPI.request = async (path, options) => {
  assert.equal(path, '/cost-adjustments/direct');
  const body = JSON.parse(options.body);
  assert.equal(body.direction, 'decrease');
  assert.equal(body.notes, '手入力メモ');
  assert.deepEqual(body.items, [{productId:'merge-a',adjustmentAmountJpyMinor:50}]);
  return {id:'cad-direct-test',slipNumber:'CA-2026-0100',items:[{productId:'merge-a',adjustedCostJpyMinor:50}]};
};
assert.equal(await window.costDirectRegister(), true);
assert.match(document.getElementById('ca-direct-register-status').textContent,/CA-2026-0100/u);
assert.match(document.getElementById('ca-direct-register-status').textContent,/仕入伝票は変更されていません/u);
window.ZaikoAPI.request = mergeRequest;
window.costAdjustmentSetMode('breakdown');
assert.equal(document.getElementById('ca-direct-adjustment').hidden,true);
assert.equal(document.querySelector('#page-cost-adjustment .ca-workbench').classList.contains('ca-direct-hidden'),false);
// Navigation discards only unsaved page inputs; accounting and inventory retain their views.
{
  delete window.ZaikoAPI;
  const beforeCounts = window.eval('[APP_DATA.inventory.length, APP_DATA.purchaseSlips.length, APP_DATA.sales.length, APP_DATA.shipments.length]');
  for (const [page, id] of [['purchase','pu-model'], ['purchase-entry','pe-date'], ['sales','sl-note'], ['shipping','sh-note'], ['consignment','co-note'], ['market','market-f-model']]) {
    window.navigateTo(page);
    const field = document.getElementById(id);
    const initial = field.value;
    field.value = field.type === 'date' ? '2099-12-31' : 'DISCARD-ON-LEAVE';
    document.getElementById('page-' + page).scrollTop = 250;
    window.navigateTo('inventory');
    assert.equal(field.value, initial, page + ' must clear on departure');
    window.navigateTo(page);
    assert.equal(field.value, initial, page + ' must not restore discarded input');
    assert.equal(document.getElementById('page-' + page).scrollTop, 0);
  }
  window.navigateTo('cost-adjustment');
  window.costAdjustmentSetMode('adjustment');
  window.costDirectReason('discard this draft');
  window.costDirectGlobalAmount('123');
  window.navigateTo('inventory');
  window.navigateTo('cost-adjustment');
  window.costAdjustmentSetMode('adjustment');
  assert.equal(document.getElementById('ca-direct-reason').value, '');
  window.navigateTo('sales-list');
  window.switchSlipTab('sales');
  document.getElementById('slip-filter-keyword').value = 'KEEP-FILTER';
  window.execSlipFilter();
  window.navigateTo('purchase');
  window.navigateTo('sales-list');
  assert.equal(document.getElementById('slip-filter-keyword').value, 'KEEP-FILTER');
  assert.equal(window.eval('currentSlipTab'), 'sales');
  window.navigateTo('cost-adjustment-slips');
  window.navigateTo('sales-list');
  assert.equal(document.getElementById('slip-filter-keyword').value, 'KEEP-FILTER');
  assert.equal(window.eval('currentSlipTab'), 'sales');
  assert.deepEqual([...window.eval('[APP_DATA.inventory.length, APP_DATA.purchaseSlips.length, APP_DATA.sales.length, APP_DATA.shipments.length]')], [...beforeCounts]);
}

// Use deliberately distinct labels so a wrong display context cannot pass unnoticed.
{
 const data=window.eval('APP_DATA');
 const accessory={code:'0098',name:'商品詳細付属品',inventoryDetail:'在庫一覧付属品',overseasDetail:'OVERSEAS-ACCESSORY',tagDetail:'TAG-ACCESSORY',domesticDetail:'国内付属品',supplement:'数字'};
 const belt={code:'0098',name:'商品詳細ベルト',inventoryDetail:'在庫ベルト',overseasDetail:'OVERSEAS-BELT',tagDetail:'TAG-BELT',domesticDetail:'国内ベルト'};
 const staff={code:'BUY-PURPOSE',name:'商品詳細バイヤー',inventoryDetail:'在庫バイヤー',overseasDetail:'OVERSEAS-STAFF',tagDetail:'TAG-STAFF',domesticDetail:'国内バイヤー'};
 data.accessoryRecords.push(accessory);data.beltMaterialRecords.push(belt);data.staffRecords.push(staff);
 const p={...data.inventory[0],code:'PURPOSE-TEST',brand:'Display Test',accessories:[accessory.name],accessorySupplements:{'0098':'3'},belt:belt.name,staff:staff.name,staffCode:staff.code};data.inventory.push(p);
 const original=JSON.stringify(p);
 for(const [purpose,label] of [['detail',accessory.name],['inventory',accessory.inventoryDetail],['overseas',accessory.overseasDetail],['tag',accessory.tagDetail],['domestic',accessory.domesticDetail]]){
  assert.equal(window.accessoryDisplayNames(['0098'],p.accessorySupplements,purpose).join(''),label+' 3');
  assert.equal(window.masterDisplayName('belt','98',purpose),purpose==='detail'?belt.name:belt[{inventory:'inventoryDetail',overseas:'overseasDetail',tag:'tagDetail',domestic:'domesticDetail'}[purpose]]);
  assert.equal(window.masterDisplayName('staff',staff.code,purpose),purpose==='detail'?staff.name:staff[{inventory:'inventoryDetail',overseas:'overseasDetail',tag:'tagDetail',domestic:'domesticDetail'}[purpose]]);
 }
 assert.match(window.renderInventoryAccessorySymbols(p.accessories,p.accessorySupplements),/在庫一覧付属品 3/);
 assert.match(window.renderAccessoryNameWithSupplement(accessory.code,p.accessorySupplements),/商品詳細付属品/);
 assert.match(window.renderInventoryProductTagPanel(p),/TAG-ACCESSORY 3/);assert.match(window.renderInventoryProductTagPanel(p),/TAG-BELT/);
 for(const regionType of ['domestic','overseas']){
  const code='PURPOSE-'+regionType,party={code,name:code,regionType};data.buyers.push(party);data.suppliers.push(party);
  const label=regionType==='overseas'?'OVERSEAS-ACCESSORY':'国内付属品';
  const expectedBelt=regionType==='overseas'?'OVERSEAS-BELT':'国内ベルト';
  const shipment={...data.shipments[0],id:code,destination:code,displayCurrency:regionType==='overseas'?'JPY':'USD',items:[{code:p.code,brand:p.brand,model:p.model,salePrice:100}]};
  const html=window.buildShipmentRecordTemplateHTML(shipment);assert.ok(html.includes(label+' 3'));assert.ok(html.includes(expectedBelt));
  assert.ok(window.buildConsignmentRecordTemplateHTML(shipment).includes(label+' 3'));
  const sale={...data.sales[0],id:code,buyer:code,items:[{...shipment.items[0],accessories:[accessory.code]}]};
  assert.ok(window.buildSalesRecordTemplateHTML(sale).includes(label+' 3'));
  const purchase={...data.purchaseSlips[0],id:code,supplier:code,staff:staff.code,lines:[{code:p.code,productDetail:{...p},quantity:1,purchasePrice:100}]};
  const purchaseHTML=window.buildPurchaseRecordTemplateHTML(purchase);assert.ok(purchaseHTML.includes(label+' 3'));assert.ok(purchaseHTML.includes(regionType==='overseas'?'OVERSEAS-STAFF':'国内バイヤー'));
  assert.ok(window.buildPurchaseReturnRecordTemplateHTML({id:'RETURN-'+code,supplier:code,items:[{code:p.code}]}).includes(label+' 3'));
  assert.ok(window.buildSalesReturnRecordTemplateHTML({id:'SALES-RETURN-'+code,buyer:code,items:[{code:p.code}]}).includes(label+' 3'));
 }
 assert.equal(window._buildCustomsItems({items:[{code:p.code} ]})[0].belt,'OVERSEAS-BELT');
 assert.equal(JSON.stringify(p),original,'Display must not rewrite stored values');
 accessory.tagDetail='<script>unsafe</script>';assert.ok(window.renderInventoryProductTagPanel(p).includes('&lt;script&gt;unsafe&lt;/script&gt;'));assert.ok(!window.renderInventoryProductTagPanel(p).includes('<script>unsafe</script>'));
 accessory.overseasDetail='（表記無し）';assert.equal(window.masterDisplayName('accessory',accessory.code,'overseas'),'（表記無し）');
 accessory.inventoryDetail='';assert.equal(window.masterDisplayName('accessory',accessory.code,'inventory'),accessory.name);
 console.log('Purpose-specific master labels: inventory, details, tags, purchase, sales, returns, shipping, consignment, customs and safe escaping OK');
}

{
 const data=window.eval('APP_DATA');
 data.staffRecords.push({code:'0042',name:'Buyer Name'});
 const product={code:'STATEMENT-001',material:'SS/YG',brand:'ROLEX',model:'デイトジャスト',ref:'126333',serial:'3XV85225',dial:'シャンパン',belt:'ジュビリー',movement:'AT',weightGrams:'200.0',accessories:['紙ギャラ'],staff:'Buyer Name',boxNo:3,purchasePrice:123456789,note:'シール付き',comment:'確認済み'};
 data.inventory.push(product);
 const original=JSON.stringify(product);
 const fixture=()=>({title:'サンプル請求書',slipId:'SAMPLE-001',counterparty:{name:'テスト取引先'},note:'表紙だけの備考\n改行を保持',taxMode:'standard',includeBank:true,items:Array.from({length:40},(_,index)=>({no:index+1,code:product.code,amount:2300000}))});
 const html=window.buildTemplateStyleSlipDocument(fixture());
 const root=window.document.createElement('div');root.innerHTML=html;
 assert.equal(root.querySelectorAll('.tpl-detail-page').length,1,'40 standard products fit one detail page');
 assert.equal(root.querySelectorAll('.tpl-detail-table tbody tr').length,40);
 assert.deepEqual([...root.querySelectorAll('.tpl-detail-table th')].map(th=>th.textContent),['No','商品情報','備考','販売価格（税抜）','社内管理']);
 const cells=root.querySelector('.tpl-detail-table tbody tr').children;
 assert.equal(cells[1].querySelectorAll('.tpl-product-grid > div').length,3);
 assert.deepEqual([...cells[1].querySelectorAll('.tpl-product-grid > div')].map(col=>[...col.children].map(el=>el.textContent.trim())),[['SS/YG','ROLEX','AT'],['デイトジャスト','126333','3XV85225'],['シャンパン','ジュビリー','200.0 g']]);
 assert.match(cells[4].textContent,/0042・3/);assert.ok(!cells[4].textContent.includes('Buyer Name'));
 assert.ok(!root.textContent.includes('123456789'),'Raw internal cost must not be exposed before cipher rules are supplied');
 assert.equal(root.querySelectorAll('.tpl-note-block').length,1);assert.ok(root.querySelector('.tpl-cover-page .tpl-note-block'));assert.equal(root.querySelectorAll('.tpl-detail-page .tpl-note-block').length,0);
 assert.match(root.querySelector('.tpl-statement-summary').textContent,/92,000,000/);assert.match(root.querySelector('.tpl-statement-summary').textContent,/9,200,000/);assert.match(root.querySelector('.tpl-statement-summary').textContent,/101,200,000/);
 const more=fixture();more.items.push({no:41,code:product.code,amount:0});root.innerHTML=window.buildTemplateStyleSlipDocument(more);
 assert.equal(root.querySelectorAll('.tpl-detail-page').length,2);assert.equal(root.querySelectorAll('.tpl-statement-summary').length,1);assert.equal(root.querySelectorAll('.tpl-detail-table tbody tr').length,41);assert.equal(root.querySelector('.tpl-detail-page:last-child .tpl-cell-number').textContent,'41');
 if (typeof writeStatementFixture==='function') await writeStatementFixture(root.innerHTML,'41');
 const long=fixture();long.items=[{no:1,code:product.code,amount:10,product:{...window.getSlipStatementProduct(product),note:'長い備考'.repeat(1200)+'終端<&>',comment:''}}];root.innerHTML=window.buildTemplateStyleSlipDocument(long);
 assert.ok(root.querySelectorAll('.tpl-detail-page').length>1);assert.ok(root.textContent.includes('終端<&>'));assert.equal(root.querySelectorAll('.tpl-cell-amount').length>1,true);assert.equal([...root.querySelectorAll('.tpl-cell-amount')].filter(el=>el.textContent).length,1,'Continuation rows must not duplicate amounts');
 if (typeof writeStatementFixture==='function') await writeStatementFixture(root.innerHTML,'long');
 assert.equal(JSON.stringify(product),original,'Printing must never mutate stored product values');
 const escaped=fixture();escaped.items=[{code:product.code,amount:0,product:{...window.getSlipStatementProduct(product),note:'<img src=x onerror=alert(1)>'}}];root.innerHTML=window.buildTemplateStyleSlipDocument(escaped);assert.equal(root.querySelectorAll('img').length,0);
 const oldPurchase=data.purchaseSlips[0],oldShipment=data.shipments[0],oldSale=data.sales[0];
 const cases=[
  ['purchase',window.buildPurchaseRecordTemplateHTML({...oldPurchase,lines:[{code:product.code,purchasePrice:1000,quantity:1}]}),'仕入金額（税抜）'],
  ['shipping',window.buildShipmentRecordTemplateHTML({...oldShipment,items:[{code:product.code,salePrice:1000}]}),'販売価格（税抜）'],
  ['consignment',window.buildConsignmentRecordTemplateHTML({...oldShipment,items:[{code:product.code,salePrice:1000}]}),'販売価格（税抜）'],
  ['sale',window.buildSalesRecordTemplateHTML({...oldSale,items:[{code:product.code,salePrice:1000}]}),'販売価格（税抜）'],
  ['purchase-return',window.buildPurchaseReturnRecordTemplateHTML({id:'PR-TEST',items:[{code:product.code}]}),'仕入返品金額（税抜）'],
  ['sales-return',window.buildSalesReturnRecordTemplateHTML({id:'SR-TEST',items:[{code:product.code}]}),'売上返品金額（税抜）'],
  ['cost',window.buildCostAdjustmentRecordTemplateHTML({id:'CA-TEST',lines:[{code:product.code,allocatedCostJpyMinor:1000}]}),'調整後・配分原価'],
  ['sale-adjustment',window.buildSalesAdjustmentRecordTemplateHTML({id:'SA-TEST',items:[{code:product.code,beforeSalePrice:100,afterSalePrice:200,adjustmentAmount:100}]}),'修正金額（税抜）'],
 ];
 for(const [name,html,label] of cases){root.innerHTML=html;
 const detailText=[...root.querySelectorAll('.tpl-detail-page')].map(page=>page.textContent).join('');
 assert.ok(detailText.includes('シール付き'), name + ': product features must remain');
 assert.equal(detailText.includes('確認済み'), name === 'cost', name + ': internal comments are only for cost adjustments');
 assert.ok(detailText.includes('紙ギャラ') || detailText.includes('Guarantee'), name + ': accessories must remain');
assert.equal(root.querySelectorAll('.tpl-detail-table th').length,5,name);assert.equal(root.querySelector('.tpl-detail-table th:nth-child(4)').textContent,label,name);assert.ok(root.querySelector('.tpl-product-grid').textContent.includes('200.0 g'),name);assert.ok(root.querySelector('.tpl-cell-code').textContent.includes('0042'),name);}
 if (typeof writeStatementFixture==='function') await writeStatementFixture(html);
 const noteOnly=fixture();noteOnly.items=[{code:product.code,amount:1,statementNote:'社内の変更理由'}];
 root.innerHTML=window.buildTemplateStyleSlipDocument(noteOnly);
 assert.ok(!root.querySelector('.tpl-detail-page').textContent.includes('社内の変更理由'));
 const noteFreeProduct={...window.getSlipStatementProduct(product),note:'',comment:'秘密の社内メモ'};
 const noteFree=window.slipStatementRow({product:noteFreeProduct},'domestic');
 assert.ok(!noteFree.columns[3].join('').includes('秘密の社内メモ'),'empty public notes must never fall back to internal comments');
 assert.equal(JSON.stringify(product),original,'Filtering notes must preserve stored data');
 console.log('Unified statement: 8 slip types, fields, buyer code, cover note, tax totals, 40/41 pagination, long text, escaping and immutability OK');
}


{
 for (const [amount,currency,expected] of [[1355000,'JPY','TSYE'],[555000,'JPY','YEY'],[1818,'JPY','T'],[350,'JPY','LS'],[66500,'USD','REY'],[800,'USD','P'],[50,'USD','LY'],[99,'JPY','LT'],[9,'USD','LT'],[100,'JPY','LT'],[10,'USD','LT'],[5555000,'JPY','YEYE'],[1000000,'JPY','TLEL'],[0,'JPY','L']]) {
  assert.equal(window.priceCodeBody(amount,currency),expected);
  const coded=window.formatPriceCode(amount,currency,'unit');
  assert.equal(coded.slice(3,-3),expected);
  assert.match(coded.slice(0,3)+coded.slice(-3),/^[TNSKYRUPOL]{6}$/);
  assert.equal(window.formatPriceCode(amount,currency,'unit'),coded);
 }
 for(const amount of [null,undefined,'',NaN,Infinity,-1]) assert.equal(window.formatPriceCode(amount,'JPY'),'');
 assert.equal(window.formatPriceCode(100,'HKD'),'');
 assert.equal(window.getStatementCostJpy({purchasePrice:1000,purchaseCurrency:'USD'},{purchasePrice:155000}),155000);
 assert.equal(window.getStatementCostJpy({purchasePrice:1000,purchaseCurrency:'USD',purchaseFxRateScaled:1550000,purchaseFxScale:10000}),155000);
 assert.equal(window.getStatementCostJpy({purchasePrice:1000,purchaseCurrency:'USD'}),null);
 assert.equal(window.getStatementCostJpy({convertedPurchasePriceJpy:310000,quantity:2}),155000);
 assert.equal(window.getStatementCostJpy({allocatedCostJpyMinor:0},{purchasePrice:155000}),0);
 const priceFixture={code:'CODE-TAG',brand:'テスト',model:'検証',purchasePrice:1355000,salePrice:66500,purchaseCurrency:'HKD',saleCurrency:'JPY',accessories:[]};
 const before=JSON.stringify(priceFixture),root=window.document.createElement('div');
 root.innerHTML=window.renderInventoryProductTagPanel(priceFixture);
 const sale=root.querySelector('.inventory-product-tag-front [data-tag-price="sale"] text').textContent;
 const cost=root.querySelector('.inventory-product-tag-front [data-tag-price="cost"] text').textContent;
 assert.equal(sale.slice(3,-3),'REY'); assert.equal(cost.slice(3,-3),'TSYE');
 assert.equal(root.querySelectorAll('.inventory-product-tag-back').length,0);
 const svg=window.createInventoryProductTagSvg(priceFixture);
 assert.ok(svg.includes(sale));assert.ok(svg.includes(cost));assert.ok(!svg.includes('1355000'));assert.ok(!svg.includes('66500'));
 assert.equal(window.getInventoryQrPayload(priceFixture),'CODE-TAG');
 assert.equal(JSON.stringify(priceFixture),before);
 if(typeof writeStatementFixture==='function') {
  await writeFile('price-code-20260919/tag.svg',svg);
  const css=await readFile(path.join(referenceRoot,'css/style.css'),'utf8');
  await writeFile('price-code-20260919/tag.html','<!doctype html><meta charset="utf-8"><style>'+css+'</style>'+root.innerHTML);
 }
 const foreign=window.getSlipStatementProduct({purchasePrice:66500,purchaseCurrency:'USD'});
 assert.equal(window.slipStatementRow({product:foreign},'domestic').columns[4].join(''),'');
 console.log('Price code: supplied examples, boundaries, repeats, padding, fixed currencies, tag sides, QR payload and cost provenance OK');
}


{
 const data=window.eval('APP_DATA');
 const previous=data.accessoryRecords;
 data.accessoryRecords=[{code:'0001',name:'RX箱',inventoryDetail:'B',tagDetail:'・',domesticDetail:'（表記無し）',overseasDetail:'（表記無し）'},{code:'0002',name:'箱',tagDetail:'Box',domesticDetail:'箱',overseasDetail:'Box'}];
 const p={code:'RX-RETURN-TEST',brand:'テスト',accessories:['RX箱','箱'],purchasePrice:1000,accessorySupplements:{'0001':'補足'}};
 data.inventory.push(p);
 const before=JSON.stringify(p);
 for(const regionType of ['domestic','overseas']) {
  const supplier={code:'RX-SUPPLIER-'+regionType,name:'確認仕入先',regionType}; data.suppliers.push(supplier);
  const ret={id:'RX-RETURN-'+regionType,supplier:supplier.code,items:[{code:p.code}]};
  const root=window.document.createElement('div');root.innerHTML=window.buildPurchaseReturnRecordTemplateHTML(ret);
  const remarks=root.querySelector('.tpl-cell-remarks').textContent;
  assert.ok(remarks.includes('RX箱 補足')); assert.ok(!remarks.includes('（表記無し）'));
  assert.ok(remarks.includes(regionType==='overseas'?'Box':'箱'));
  assert.ok(root.querySelector('.tpl-cell-amount').textContent.includes('1,000'));
  assert.ok(window.buildPurchaseReturnDownloadHTML(ret).includes('RX箱'));
  const normal=window.getSlipStatementProduct(p,regionType);
  assert.ok(normal.accessories[0].includes('（表記無し）'));
  data.suppliers.pop();
 }
 assert.equal(window.accessoryDisplayNames(['0001'],{},'tag')[0],'・');
 assert.equal(window.accessoryDisplayNames(['1'],{},'inventory')[0],'B');
 assert.equal(window.accessoryDisplayNames(['0001'],{},'overseas',{documentType:'purchaseReturn'})[0],'RX箱');
 assert.equal(JSON.stringify(p),before);
 data.inventory.pop();data.accessoryRecords=previous;
 console.log('RX box exception: domestic/overseas purchase returns and download; other accessories, tags, inventory, other documents, amounts and stored data unchanged OK');
}

{
 const data=window.eval('APP_DATA'), previous=data.accessoryRecords;
 data.accessoryRecords=[{code:'0001',name:'RX箱',tagDetail:'・'},{code:'0002',name:'箱',tagDetail:'Box'}, {code:'0024',name:'ギャラ2c年月',tagDetail:'2c'}, {code:'0040',name:'コマ（数字）',supplement:'コマ数'}];
 const product={accessories:['RX箱','箱','ギャラ2c年月','コマ（数字）'],accessorySupplements:{'0024':'26/9','0040':'3'}};
 assert.equal(window._inventoryTagAccessories(product),'・、Box、2c 26/9、3');
 assert.equal(window.accessoryDisplayNames(['0040'],{},'tag').length,0);
 assert.equal(window.accessoryDisplayNames(['0040'],{'0040':'0'},'tag')[0],'0');
 assert.equal(window.accessoryDisplayNames(['0040'],{'0040':'3'},'detail')[0],'コマ（数字） 3');
 data.accessoryRecords[3].tagDetail='LINKS';
 assert.equal(window.accessoryDisplayNames(['0040'],{'0040':'3'},'tag')[0],'LINKS 3');
 data.accessoryRecords[3].tagDetail='';
 assert.equal(window.accessoryDisplayNames(['0040'],{'0040':'3'},'tag')[0],'3');
 data.accessoryRecords=previous;
 console.log('Accessory tags: configured labels, empty labels, counts and detail display verified');
}
dom.window.close();

let guestHtml = guestHtmlSource.replace(/<link\b[^>]*>/gi, "");
guestHtml = guestHtml.replace(/<script\b[^>]*src="https?:\/\/[^\"]+"[^>]*><\/script>/gi, "");
for (const name of ["data.js", "guest_shared.js", "login_info.js", "guest.js"]) {
  const source = name === "guest.js"
    ? await readFile(path.join(referenceRoot, "js", name), "utf8")
    : scriptSources.get(name);
  const tag = new RegExp(`<script src="js/${name.replaceAll(".", "\\.")}(?:\\?[^" ]*)?"></script>`);
  assert.match(guestHtml, tag, `guest.html is missing script ${name}`);
  guestHtml = guestHtml.replace(tag, () => `<script>\n${source}\n</script>`);
}

const guestRuntimeErrors = [];
const guestVirtualConsole = new VirtualConsole();
guestVirtualConsole.on("jsdomError", (error) => guestRuntimeErrors.push(error));
guestVirtualConsole.on("error", (...args) => guestRuntimeErrors.push(new Error(args.join(" "))));
const guestDom = new JSDOM(guestHtml, {
  url: "http://127.0.0.1:8080/app/admin-reference/guest.html?id=G001",
  runScripts: "dangerously",
  pretendToBeVisual: true,
  virtualConsole: guestVirtualConsole,
  beforeParse(guestWindow) {
    guestWindow.HTMLElement.prototype.scrollIntoView = () => {};
  },
});
await new Promise((resolve) => {
  if (guestDom.window.document.readyState === "complete") return resolve();
  guestDom.window.addEventListener("load", resolve, { once: true });
});
await new Promise((resolve) => setTimeout(resolve, 20));

const guestWindow = guestDom.window;
const guestDocument = guestWindow.document;
assert.deepEqual(guestRuntimeErrors, [], `guest boot emitted errors: ${guestRuntimeErrors.map(String).join("\n")}`);
const publishedGuestItems = guestWindow.eval("guestCatalogItems");
assert.equal(publishedGuestItems.length, 7, "G001 must only see available products from boxes published to B004");
assert.equal(publishedGuestItems.some(item => item.code === "0303260001"), false, "reserved products must not appear in the guest catalog");
assert.equal(guestDocument.querySelectorAll(".guest-product-card").length, 7);
assert.equal(guestDocument.getElementById("guest-publish-summary"), null);
assert.equal(guestDocument.querySelectorAll(".guest-product-box").length, 0);
assert.match(guestDocument.querySelector(".guest-product-price").textContent, /¥1,181,000/u);
assert.equal(guestDocument.querySelectorAll("#guest-currency,#guest-lang-ja,#guest-lang-en").length, 0);

guestWindow.setGuestCurrency("USD");
assert.match(guestDocument.querySelector(".guest-product-price").textContent, /\$7,613/u);
const firstGuestItem = publishedGuestItems[0];
guestWindow.toggleGuestCart(firstGuestItem.code);
assert.equal(guestDocument.getElementById("guest-cart-count").textContent, "1");
guestWindow.openGuestCart();
for (const checkbox of guestDocument.querySelectorAll(".guest-confirm-check")) checkbox.checked = true;
guestWindow.updateGuestSubmitState();
assert.equal(guestDocument.getElementById("guest-submit-request").disabled, false);
guestDocument.getElementById("guest-request-note").value = "ゲスト画面テスト";
guestWindow.submitGuestPurchaseRequest();
const storedGuestRequests = JSON.parse(guestWindow.localStorage.getItem("inv_purchase_requests_v1"));
assert.equal(storedGuestRequests.length, 4);
assert.equal(storedGuestRequests[0].guestId, "G001");
assert.equal(storedGuestRequests[0].buyerCode, "B004");
assert.equal(storedGuestRequests[0].clientCompanyCode, "CLI-001");
assert.equal(storedGuestRequests[0].items[0].itemCode, firstGuestItem.code);
assert.equal(storedGuestRequests[0].items[0].boxNo, firstGuestItem.boxNo);
assert.match(guestDocument.getElementById("guest-success").textContent, /管理者へ送信しました/u);
assert.equal(guestDocument.getElementById("guest-cart-count").textContent, "0");

guestDom.window.close();
console.log("Reference admin and guest DOM contracts: OK (BOX publish, cart, purchase request, responsive guest catalog)");

const accessoryCodeFixture = window.eval("APP_DATA.accessoryRecords[0]");
assert.ok(accessoryCodeFixture?.code);
assert.deepEqual(Array.from(window._costAdjustmentAccessoryCodes([accessoryCodeFixture.name, accessoryCodeFixture.code])), [accessoryCodeFixture.code]);
assert.deepEqual(Array.from(window._costAdjustmentAccessoryCodes([])), []);
assert.throws(() => window._costAdjustmentAccessoryCodes(["MISSING-ACCESSORY-TEST"]), /マスタに見つかりません/u);
