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
  "qr_inventory.js",
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
const staticAppSource = await readFile(path.join(referenceRoot, "js", "app.js"), "utf8");
const purchaseEntrySource = await readFile(path.join(referenceRoot, "js", "purchase_entry.js"), "utf8");
assert.match(staticAppSource, /modalBody\.scrollTo\(\{ top: 0/u,
  "inventory detail modal must always open at the top");
assert.match(purchaseEntrySource, /_peClearCSVImportErrors/u,
  "purchase CSV import errors must provide an explicit close action");
assert.match(purchaseEntrySource, /window\.__purchaseEntryLastCSVErrors/u,
  "purchase CSV import errors must remain visible until replaced or closed");
assert.match(staticAppSource, /const _slipSelections = \{/u,
  "all slip tabs must share the multi-selection state manager");
for (const slipType of ["purchase", "shipping", "consignment", "sales", "salesreturn", "purchasereturn"]) {
  assert.match(staticAppSource, new RegExp(`${slipType}: new Set\\(\\)`),
    `${slipType} slips must support independent multi-selection`);
}
for (const label of ["明細書発行", "請求書発行", "仕入返品伝票発行", "売上返品伝票発行"]) {
  assert.ok(staticAppSource.includes(label), `${label} must be available only through the matching tab operation`);
}
assert.match(staticAppSource, /対象の伝票を1件以上選択してください/u,
  "bulk slip operations must reject an empty selection");
assert.match(staticAppSource, /originalCode:\s*String\(item\.code/u,
  "product editing must retain the original code for self-excluding duplicate validation");
assert.match(apiBridgeSource, /productCode: values\.code \|\| item\.code/u,
  "product code changes must be sent to the API");
assert.match(apiBridgeSource, /async function fetchAllPages\(path, pageSize = 100\)/u,
  "inventory API hydration must support fetching every product page");
assert.match(apiBridgeSource, /for \(let page = 2; page <= totalPages; page \+= 1\)/u,
  "product pages must be fetched sequentially through the final page");
assert.match(apiBridgeSource, /fetchAllPages\('\/products\?includeCancelled=true'\)/u,
  "admin hydration must merge all inventory pages instead of stopping at 100 products");
assert.match(apiBridgeSource, /fetchAllPages\('\/purchases'\)/u,
  "admin hydration must merge every purchase page instead of stopping at 500 slips");
assert.match(apiBridgeSource, /reflectedPurchaseProducts !== APP_DATA\.inventory\.length/u,
  "admin hydration must reject purchase and inventory datasets whose reflected counts diverge");
assert.match(apiBridgeSource, /const currentProduct = productByLine\.get\(line\.id\)/u,
  "API purchase slips must resolve current product details through the stable purchase-line link");
assert.match(apiBridgeSource, /brandCode: resolvedBrandCode/u,
  "purchase API payloads must support an empty brand while preserving a supplied stable brand code");
assert.match(apiBridgeSource, /product\.fixedPurchaseCostJpyMinor/u,
  "inventory hydration must use the persisted purchase-date JPY snapshot");
assert.equal(
  /product\.costCurrency === 'USD'\s*\?\s*Math\.round\(product\.costAmountMinor \* rate\)/u.test(apiBridgeSource),
  false,
  "inventory hydration must not recalculate overseas costs with the latest master rate",
);
assert.match(apiBridgeSource, /request\(`\/sales\/\$\{encodeURIComponent\(saleID\)\}\/issue`/u,
  "sales issue must persist its timestamp through the REST API");
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
assert.match(staticAppSource, /async function _puSaveMatchedProductImages\(existingItem\)/u,
  "the product registration save action must persist staged images for an existing management number");
assert.equal(
  /if \(!filterStatus && itemStatus === '仕入返品済'\)/u.test(staticAppSource),
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
assert.match(guestHtmlSource, /guest-box-filter/u, "guest catalog must be driven by published boxes");
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

let html = await readFile(path.join(referenceRoot, "app.html"), "utf8");
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
const { document } = window;

assert.deepEqual(runtimeErrors, [], `reference boot emitted errors: ${runtimeErrors.map(String).join("\n")}`);
assert.equal(typeof window.navigateTo, "function", "navigateTo must be globally callable");
assert.equal(typeof window.openNotification, "function", "notification items must be globally actionable");
assert.equal(typeof window.showItemDetail, "function", "showItemDetail must be globally callable");
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

assert.equal(document.querySelector('.nav-item[data-page="login-info"]'), null, "the duplicate login information navigation must be removed");
assert.equal(document.getElementById("page-login-info"), null, "the duplicate login information page must be removed");
window.navigateTo("master");
assert.equal(window.eval("APP_DATA.clientCompanies.length"), 9, "all four buyers and five suppliers must be represented in the shared client-company directory");
assert.equal(window.eval("APP_DATA.buyers.every(buyer => APP_DATA.clientCompanies.some(company => company.buyerCode === buyer.code))"), true, "every buyer must link to a client company");
assert.equal(window.eval("APP_DATA.suppliers.every(supplier => APP_DATA.clientCompanies.some(company => company.supplierCode === supplier.code))"), true, "every supplier must link to a client company");
assert.equal(window.eval("APP_DATA.clientCompanies.find(company => company.supplierCode === 'S001').invoice"), "T0001234567890", "supplier invoice data must flow into the client-company directory");
assert.equal(window.eval("APP_DATA.clientCompanies.find(company => company.buyerCode === 'B004').guestId"), "G001", "guest-issued buyers must link to the same client company");
assert.equal(window.eval("APP_DATA.purchaseRequests.every(request => /^B\\d+$/u.test(request.buyerCode))"), true, "purchase requests must persist a fixed buyer code");
assert.equal(window.eval("APP_DATA.purchaseRequests.every(request => /^CLI-\\d+$/u.test(request.clientCompanyCode))"), true, "purchase requests must persist a fixed client-company code");
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
document.getElementById("medit-name").value = "Codex Test Brand";
window.saveMasterEdit();
assert.equal(window.eval("APP_DATA.brands.includes('Codex Test Brand')"), true, "brand master additions must update APP_DATA");
const fixedAddedBrandCode = window.eval("APP_DATA.brandRecords.find(brand => brand.name === 'Codex Test Brand').code");
assert.match(fixedAddedBrandCode, /^BRD-\d+$/, "new brands must receive a fixed code");
for (const selectId of ["market-f-brand", "inv-f-brand", "pu-brand", "pep-brand"]) {
  assert.ok([...document.getElementById(selectId).options].some(option => option.value === "Codex Test Brand"), `${selectId} must use the shared brand master`);
}
assert.match(window.localStorage.getItem("inv_brand_master_v1"), /Codex Test Brand/u, "brand master changes must persist locally");

window.showEditMasterModal("brand", initialBrandCount);
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
assert.equal(
  window.eval("APP_DATA.staffRecords.every(record => APP_DATA.users.some(user => user.staffCode === record.code && (user.role === 'buyer' || user.role === 'worker')))"),
  true,
  "every purchase staff record must have a linked own-company worker account",
);
window.switchMasterTab("staff");
assert.match(document.getElementById("masterContentArea").textContent, /共通バイヤー・作業者マスタ/u);
assert.equal(document.querySelectorAll("#masterContentArea tbody tr").length, 5);
window.showAddMasterModal("staff");
assert.equal(document.getElementById("medit-code").value, "BUY-006", "the next stable staff code must be proposed automatically");
document.getElementById("medit-name").value = "共通担当者テスト";
window.saveMasterEdit();
assert.equal(window.eval("APP_DATA.users.some(user => user.staffCode === 'BUY-006' && user.name === '共通担当者テスト')"), true, "adding purchase staff must create the matching worker account");
for (const selectId of ["inv-f-staff", "pu-staff", "pe-staff"]) {
  assert.equal([...document.getElementById(selectId).options].some(option => option.value === "共通担当者テスト"), true, `${selectId} must use the shared purchase staff master`);
}
const temporaryStaffIndex = window.eval("APP_DATA.staffRecords.findIndex(record => record.code === 'BUY-006')");
window.showEditMasterModal("staff", temporaryStaffIndex);
document.getElementById("medit-name").value = "共通担当者テスト更新";
window.saveMasterEdit();
assert.equal(window.eval("APP_DATA.users.find(user => user.staffCode === 'BUY-006').name"), "共通担当者テスト更新", "staff edits must update the linked worker account");
const staffCountBeforeBlockedDelete = window.eval("APP_DATA.staffRecords.length");
const usedStaffIndex = window.eval("APP_DATA.staffRecords.findIndex(record => record.code === 'BUY-001')");
window.showDeleteMasterModal("staff", usedStaffIndex);
window.confirmMasterDelete();
assert.equal(window.eval("APP_DATA.staffRecords.length"), staffCountBeforeBlockedDelete, "staff referenced by inventory or slips must not be deleted");
const renamedTemporaryStaffIndex = window.eval("APP_DATA.staffRecords.findIndex(record => record.code === 'BUY-006')");
window.showDeleteMasterModal("staff", renamedTemporaryStaffIndex);
window.confirmMasterDelete();
assert.equal(window.eval("APP_DATA.staffRecords.length"), initialStaffCount, "unused staff can be removed from the shared master");
assert.equal(window.eval("APP_DATA.users.some(user => user.staffCode === 'BUY-006')"), false, "removing unused staff must remove its linked worker account");

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
assert.equal(document.getElementById("medit-code").value, "BLT-006");
document.getElementById("medit-name").value = "セラミック";
window.saveMasterEdit();
assert.equal(window.eval("APP_DATA.beltMaterialRecords.at(-1).code"), "BLT-006");
assert.equal(window.eval("MASTER_TABS.some(tab => tab.key === 'dial')"), false, "dial master must be hidden");
window.eval(`APP_DATA.beltMaterialRecords.splice(${initialBeltMaterialCount}); APP_DATA.dialRecords.splice(${initialDialCount});`);

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
assert.equal(document.getElementById("medit-code").value, "ACC-007", "the next accessory code must be proposed automatically");
assert.equal(document.getElementById("medit-code").readOnly, true, "accessory codes must remain stable after creation");
document.getElementById("medit-name").value = "TEST ACCESSORY";
window.saveMasterEdit();
assert.equal(window.eval("APP_DATA.accessoryRecords.some(record => record.code === 'ACC-007' && record.name === 'TEST ACCESSORY')"), true);
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
const temporaryAccessoryIndex = window.eval("APP_DATA.accessoryRecords.findIndex(record => record.code === 'ACC-007')");
window.showEditMasterModal("accessory", temporaryAccessoryIndex);
document.getElementById("medit-name").value = "TEST ACCESSORY UPDATED";
window.saveMasterEdit();
assert.equal(window.eval("APP_DATA.accessoryRecords.find(record => record.code === 'ACC-007').name"), "TEST ACCESSORY UPDATED", "accessory edits must preserve their stable code");
assert.equal(specInventory[0].accessories.includes("TEST ACCESSORY UPDATED"), true, "accessory edits must update inventory references");
assert.equal(window.eval("APP_DATA.marketPrices[0].accessories.includes('TEST ACCESSORY UPDATED')"), true, "accessory edits must update market references");
assert.equal(document.querySelector('#pu-accessories input[value="TEST ACCESSORY UPDATED"]') !== null, true, "renamed accessories must refresh registration forms");

window.eval("_invAccFilterState = ['TEST ACCESSORY UPDATED'];");
window.execInventorySearch();
assert.equal(document.querySelectorAll("#inventoryTableBody tr").length, 1, "inventory search must filter by shared accessory names");
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
const renamedAccessoryIndex = window.eval("APP_DATA.accessoryRecords.findIndex(record => record.code === 'ACC-007')");
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
assert.equal(document.getElementById("medit-code").value, "CON-008", "the next condition code must be proposed automatically");
assert.equal(document.getElementById("medit-code").readOnly, true, "condition codes must remain stable after creation");
document.getElementById("medit-name").value = "TEST CONDITION";
window.saveMasterEdit();
assert.equal(window.eval("APP_DATA.conditions.some(record => record.code === 'CON-008' && record.name === 'TEST CONDITION')"), true);
assert.match(window.localStorage.getItem("inv_condition_master_v1"), /TEST CONDITION/u, "condition master changes must persist locally");
for (const selectId of ["inv-f-condition", "market-f-condition", "pu-condition", "pep-condition", "ie-condition", "me-condition"]) {
  assert.equal([...document.getElementById(selectId).options].some(option => option.value === "CON-008"), true, `${selectId} must use the shared condition master`);
}

const originalInventoryCondition = specInventory[0].condition;
const originalMarketCondition = window.eval("APP_DATA.marketPrices[0].condition");
specInventory[0].condition = "CON-008";
window.eval("APP_DATA.marketPrices[0].condition = 'CON-008';");
window.switchMasterTab("condition");
const temporaryConditionIndex = window.eval("APP_DATA.conditions.findIndex(record => record.code === 'CON-008')");
window.showEditMasterModal("condition", temporaryConditionIndex);
document.getElementById("medit-name").value = "TEST CONDITION UPDATED";
window.saveMasterEdit();
assert.equal(window.eval("APP_DATA.conditions.find(record => record.code === 'CON-008').name"), "TEST CONDITION UPDATED", "condition names must be editable without changing their code");
assert.match(document.querySelector('#pu-condition option[value="CON-008"]').textContent, /TEST CONDITION UPDATED/u, "renamed condition labels must refresh registration forms");

document.getElementById("inv-f-condition").value = "CON-008";
window.execInventorySearch();
assert.equal(document.querySelectorAll("#inventoryTableBody tr").length, 1, "inventory search must filter by shared condition codes");
document.getElementById("market-f-condition").value = "CON-008";
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
const removableConditionIndex = window.eval("APP_DATA.conditions.findIndex(record => record.code === 'CON-008')");
window.showDeleteMasterModal("condition", removableConditionIndex);
window.confirmMasterDelete();
assert.equal(window.eval("APP_DATA.conditions.length"), initialConditionCount, "unused conditions can be removed from the shared master");
assert.equal([...document.getElementById("market-f-condition").options].some(option => option.value === "CON-008"), false, "removed conditions must disappear from search choices");
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
const sharedClientCode = document.getElementById("clientModal-code").value;
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
assert.equal(document.querySelectorAll("#mpw-buyer-tbody tr").length, 5);
assert.equal(document.querySelectorAll("#mpw-buyer-tbody tr[data-staff-code^='BUY-']").length, 5, "worker rows must display the matching purchase staff codes");
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
assert.equal(window.saveLoginInfo(), false, "one customer must not receive multiple guest login accounts");
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
assert.equal(window.saveLoginInfo(), false, "the signed-in administrator must not be able to disable itself");
assert.equal(window.eval("APP_DATA.users.find(user => user.id === 'U001').active"), undefined);
window.closeLoginInfoModal();

const usersBeforeLoginInfoTest = window.eval("APP_DATA.users.length");
const guestsBeforeLoginInfoTest = window.eval("APP_DATA.guestAccounts.length");
const buyersBeforeLoginInfoTest = window.eval("APP_DATA.buyers.length");
const staffBeforeLoginInfoTest = window.eval("APP_DATA.staffRecords.length");
window.openLoginInfoModal("buyer");
assert.equal(document.getElementById("loginInfoStaffCode").value, "BUY-006", "new worker accounts must receive the next purchase staff code");
document.getElementById("loginInfoName").value = "ログイン管理テスト作業者";
document.getElementById("loginInfoLoginId").value = "login-test-worker";
document.getElementById("loginInfoEmail").value = "login-test-worker@example.com";
document.getElementById("loginInfoPassword").value = "testpass123";
assert.equal(window.saveLoginInfo(), true, "administrator must be able to create a worker account");
assert.equal(window.eval("APP_DATA.users.find(user => user.loginId === 'login-test-worker').staffCode"), "BUY-006", "workers created from password management must link back to the purchase staff master");
assert.equal(window.eval("APP_DATA.staffRecords.some(record => record.code === 'BUY-006' && record.name === 'ログイン管理テスト作業者')"), true);
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
assert.equal(window.saveLoginInfo(), true, "administrator must be able to create a guest with customer data");
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
const seededReservedItem = inventory.find(item => item.code === "20260303001");
assert.equal(seededReservedItem.status, "取置中", "an already-approved purchase request must restore its reservation on load");
assert.equal(seededReservedItem.reservationRequestId, "PR-003");
assert.equal(document.body.textContent.includes("売価（円）"), false, "sale price labels must not claim JPY");
const initialDashboardSummary = window.getDashboardSummary();
assert.equal(document.getElementById("dashInventoryCount").textContent.trim(), String(initialDashboardSummary.inventoryCount));
assert.equal(document.getElementById("dashSalesAmount").textContent.trim(), window.formatDashboardSales(initialDashboardSummary.monthlySalesUsd, "USD"));
assert.equal(document.getElementById("dashPurchaseAmount").textContent.trim(), window.formatDashboardPurchase(initialDashboardSummary.monthlyPurchaseJpy, "JPY"));
assert.equal(document.getElementById("dashPurchaseItemCount").textContent.trim(), `${initialDashboardSummary.monthlyPurchaseCount}点仕入`);
window.switchDashboardCurrency("sales", "JPY");
assert.equal(document.getElementById("dashSalesLabel").textContent, "今月売上（JPY）");
assert.equal(document.getElementById("dashSalesAmount").textContent.trim(), window.formatDashboardSales(initialDashboardSummary.monthlySalesUsd, "JPY"));
assert.equal(document.getElementById("dash-sales-jpy").getAttribute("aria-pressed"), "true");
window.switchDashboardCurrency("purchase", "USD");
assert.equal(document.getElementById("dashPurchaseLabel").textContent, "今月原価（USD）");
assert.equal(document.getElementById("dashPurchaseAmount").textContent.trim(), window.formatDashboardPurchase(initialDashboardSummary.monthlyPurchaseJpy, "USD"));
assert.equal(document.getElementById("dash-purchase-usd").getAttribute("aria-pressed"), "true");
window.switchDashboardCurrency("sales", "USD");
window.switchDashboardCurrency("purchase", "JPY");
assert.match(document.getElementById("page-dashboard").textContent, /月別売上推移（USD）/u);
assert.equal(document.getElementById("dashSupplierMonthSelect").options.length, 24, "supplier chart must expose the complete 24-month DB window");
assert.equal(document.getElementById("dashSalesEndMonthSelect").options.length, 24, "sales chart must expose the complete 24-month DB window");
window.switchDashboardChartCurrency("JPY");
assert.equal(document.getElementById("dash-chart-jpy").getAttribute("aria-pressed"), "true");
assert.match(document.getElementById("dashSalesChartTitle").textContent, /月別売上推移（JPY）/u);
assert.deepEqual(
  window.eval("dashCharts.chart2.config.data.datasets[0].data"),
  initialDashboardSummary.months.slice(-6).map(month => month.salesJpy),
  "JPY chart must use actual monthly JPY totals",
);
window.setDashboardSalesWindow("12");
assert.equal(window.eval("dashCharts.chart2.config.data.datasets[0].data.length"), 12, "sales chart must switch to a 12-month window");
window.shiftDashboardSalesEndMonth(-1);
assert.equal(
  window.eval("dashCharts.chart2.config.data.datasets[0].data.at(-1)"),
  initialDashboardSummary.months.at(-2).salesJpy,
  "sales chart month navigation must update its ending month",
);
window.setDashboardSalesEndMonth(initialDashboardSummary.currentMonthKey);
window.setDashboardSalesWindow("6");
window.switchDashboardChartCurrency("USD");
assert.equal(document.getElementById("dash-chart-usd").getAttribute("aria-pressed"), "true");
const previousDashboardMonth = initialDashboardSummary.months.at(-2).month;
window.setDashboardSupplierMonth(previousDashboardMonth);
assert.match(document.getElementById("dashSupplierChartTitle").textContent, new RegExp(window._dashboardMonthLabel(previousDashboardMonth)));
assert.equal(
  window.eval("dashCharts.chart1.config.data.datasets[0].data.reduce((sum, value) => sum + value, 0)"),
  initialDashboardSummary.supplierMonthly.filter(row => row.month === previousDashboardMonth).reduce((sum, row) => sum + row.purchaseJpy, 0),
  "supplier composition must follow the selected month",
);
window.setDashboardSupplierMonth(initialDashboardSummary.currentMonthKey);

const dashboardToday = new window.Date();
const dashboardDate = `${dashboardToday.getFullYear()}-${String(dashboardToday.getMonth() + 1).padStart(2, "0")}-02`;
const dashboardInventoryLength = inventory.length;
const dashboardPurchaseLength = window.eval("APP_DATA.purchaseSlips.length");
const dashboardSalesLength = window.eval("APP_DATA.sales.length");
inventory.push({ code: "DASH-TEST-001", status: "在庫中", brand: "テスト", model: "集計", purchasePrice: 350000 });
window.eval("APP_DATA.purchaseSlips").push({
  id: "PI-DASH-TEST",
  date: dashboardDate,
  supplier: "S001",
  status: "未処理",
  lines: [
    { code: "DASH-TEST-001", purchasePrice: 120000 },
    { code: "DASH-TEST-002", purchasePrice: 230000 },
  ],
});
window.eval("APP_DATA.purchaseSlips").push({
  id: "PI-DASH-PENDING",
  date: dashboardDate,
  supplier: "S001",
  status: "承認待ち",
  lines: [{ code: "DASH-PENDING", purchasePrice: 999999 }],
});
window.eval("APP_DATA.sales").push(
  { id: "SL-DASH-TEST", date: dashboardDate, status: "確定", total: 1234, items: [] },
  { id: "SL-DASH-PENDING", date: dashboardDate, status: "承認待ち", total: 9999, items: [] },
);
window.init_dashboard();
const updatedDashboardSummary = window.getDashboardSummary();
assert.equal(updatedDashboardSummary.inventoryCount, initialDashboardSummary.inventoryCount + 1, "in-stock KPI must follow inventory status");
assert.equal(updatedDashboardSummary.monthlyPurchaseCount, initialDashboardSummary.monthlyPurchaseCount + 2, "purchase KPI must count recorded slip lines");
assert.equal(updatedDashboardSummary.monthlyPurchaseJpy, initialDashboardSummary.monthlyPurchaseJpy + 350000, "purchase KPI must sum recorded slip prices");
assert.equal(updatedDashboardSummary.monthlySalesUsd, initialDashboardSummary.monthlySalesUsd + 1234, "sales KPI must include confirmed slips and exclude pending slips");
assert.equal(document.getElementById("dashPurchaseItemCount").textContent.trim(), `${updatedDashboardSummary.monthlyPurchaseCount}点仕入`);
assert.equal(document.getElementById("dashSalesAmount").textContent.trim(), window.formatDashboardSales(updatedDashboardSummary.monthlySalesUsd, "USD"));
assert.equal(window.eval("dashCharts.chart2.config.data.datasets[0].data.at(-1)"), updatedDashboardSummary.monthlySalesUsd, "monthly chart must use actual confirmed sales slips");
assert.equal(window.eval("dashCharts.chart1.config.data.datasets[0].data.reduce((sum, value) => sum + value, 0)"), updatedDashboardSummary.monthlyPurchaseJpy, "supplier chart must use current-month purchase slips");
inventory.splice(dashboardInventoryLength);
window.eval(`APP_DATA.purchaseSlips.splice(${dashboardPurchaseLength})`);
window.eval(`APP_DATA.sales.splice(${dashboardSalesLength})`);
window.init_dashboard();
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
assert.equal(window.unpublishGuestProducts([publishedProductCode]), 1, "shipping or selling must remove the product from the guest snapshot");
storedPublishedSnapshot = JSON.parse(window.localStorage.getItem("inv_guest_snapshot_v1"));
assert.equal(
  storedPublishedSnapshot.boxes.some(box => (box.items || []).some(item => item.code === publishedProductCode)),
  false,
  "the automatically unpublished product must also be removed from persisted guest data",
);
assert.equal(window.unpublishGuestProducts([publishedProductCode]), 0, "automatic unpublishing must be idempotent");
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
assert.match(document.getElementById("dashRequests").textContent, /管理者通知テスト/u);
assert.equal(Number(document.getElementById("requestBadge").textContent), 3);
assert.equal(document.getElementById("dashRequestCount").textContent, "3");

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
assert.match(apiBridgeSource, /const sourceSale = saleDetails\.find/u,
  "return documents must recover their source sale and monetary detail by product code");
assert.match(qrInventorySource, /function _inventoryTagMasterName\(value, records\)/u,
  "product tags must resolve master codes to names or symbols");
assert.match(qrInventorySource, /APP_DATA\.markingRecords/u,
  "product tags must show the marking symbol instead of its code");
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
assert.equal(document.querySelector('#inventoryTableBody tr:first-child td[data-inv-col="purchasePrice"]').textContent.trim(), "¥850,000", "inventory purchase price must stay JPY");
assert.equal(document.querySelector('#inventoryTableBody tr:first-child td[data-inv-col="salePrice"]').textContent.trim(), "$7,613", "inventory sale price must display USD");
assert.match(document.querySelector("#page-inventory thead").textContent, /売価/u);
assert.equal(document.getElementById("inv-purchase-jpy").getAttribute("aria-pressed"), "true");
assert.equal(document.getElementById("inv-sale-usd").getAttribute("aria-pressed"), "true");
assert.equal(typeof window.switchInventoryPriceCurrency, "function");
assert.equal(typeof window._invColumnVisibilityChanged, "function");
window._invToggleColumnMenu({ stopPropagation() {} });
assert.equal(document.getElementById("inv-column-panel").classList.contains("open"), true);
assert.equal(document.getElementById("inv-column-trigger").getAttribute("aria-expanded"), "true");
window._invCloseColumnMenu();
assert.equal(document.getElementById("inv-column-trigger").getAttribute("aria-expanded"), "false");

window.switchInventoryPriceCurrency("purchase", "USD");
window.switchInventoryPriceCurrency("sale", "JPY");
assert.equal(document.querySelector('#inventoryTableBody tr:first-child td[data-inv-col="purchasePrice"]').textContent.trim(), "$5,484", "purchase price must convert from JPY to USD using the master rate");
assert.equal(document.querySelector('#inventoryTableBody tr:first-child td[data-inv-col="salePrice"]').textContent.trim(), "¥1,180,015", "sale price must convert from USD to JPY using the master rate");
assert.equal(document.getElementById("inv-purchase-usd").getAttribute("aria-pressed"), "true");
assert.equal(document.getElementById("inv-sale-jpy").getAttribute("aria-pressed"), "true");
assert.match(document.getElementById("inv-purchase-heading").textContent, /USD/u);
assert.match(document.getElementById("inv-sale-heading").textContent, /JPY/u);

window.showItemDetail(inventory[0].code);
assert.equal(document.getElementById("itemDetailModal").classList.contains("hidden"), false, "item detail modal must open");
assert.match(document.getElementById("itemDetailTitle").textContent, new RegExp(inventory[0].code));
assert.match(document.getElementById("itemDetailBody").textContent, /\$5,484/u, "item detail purchase price must follow the selected USD display");
assert.match(document.getElementById("itemDetailBody").textContent, /¥1,180,015/u, "item detail sale price must follow the selected JPY display");
const itemDetailInfoPanel = document.getElementById("itemDetailInfoPanel");
const itemDetailLabels = [...itemDetailInfoPanel.querySelectorAll(".detail-label")].map(label => label.textContent.trim());
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
assert.equal(productTagPanel.querySelectorAll(".inventory-product-tag").length, 2, "product tag must show front and back");
assert.equal(productTagPanel.querySelectorAll(".inventory-product-tag-qr svg").length, 2, "both tag sides must include the management-number QR");
assert.equal(productTagPanel.querySelector(".inventory-product-tag-model span")?.classList.contains("sr-only"), true,
  "the product tag must hide the brand label visually");
const tagMaterialName = window.eval(`APP_DATA.materials.find(record => record.code === ${JSON.stringify(inventory[0].material)})?.name || ${JSON.stringify(inventory[0].material || "—")}`);
const tagMovementName = window.eval(`APP_DATA.movements.find(record => record.code === ${JSON.stringify(inventory[0].movement)})?.name || ${JSON.stringify(inventory[0].movement || "—")}`);
assert.equal(productTagPanel.querySelector(".inventory-product-tag-model strong")?.textContent,
  `${inventory[0].brand}（${tagMaterialName}／${inventory[0].belt || "—"}）`,
  "the product tag heading must combine brand, material, and belt material without visible labels");
assert.equal(productTagPanel.querySelector(".inventory-product-tag-fields dt")?.classList.contains("sr-only"), true,
  "the product tag must hide item labels visually");
assert.equal(productTagPanel.querySelector(".inventory-product-tag-fields dd")?.textContent, inventory[0].model);
assert.equal(productTagPanel.querySelector(".inventory-product-tag-note span")?.classList.contains("sr-only"), true,
  "the product tag must hide the note label visually");
assert.ok([...productTagPanel.querySelectorAll(".inventory-product-tag-code span")]
  .every((label) => label.classList.contains("sr-only")), "management-number labels must be visually hidden on both sides");
assert.match(productTagPanel.textContent, new RegExp(inventory[0].code), "product tag must include the management number");
assert.match(productTagPanel.textContent, new RegExp(inventory[0].model), "product tag must include the model name");
assert.match(productTagPanel.textContent, new RegExp(tagMovementName), "product tag must include the movement name instead of its code");
assert.ok([...productTagPanel.querySelectorAll(".inventory-product-tag-mini-values .sr-only, .inventory-product-tag-marking .sr-only")]
  .every((label) => label.classList.contains("sr-only")), "movement and marking labels must be visually hidden");
assert.doesNotMatch(productTagPanel.textContent, /原価|売価|仕入金額|販売価格/u, "product tag must not expose purchase or sale prices");
const downloadedTagSvg = window.createInventoryProductTagSvg(inventory[0]);
assert.match(downloadedTagSvg, new RegExp(inventory[0].code), "downloaded tag SVG must include the management number");
assert.doesNotMatch(downloadedTagSvg, />ブランド名<|>モデル名<|>型番<|>シリアル<|>付属品<|>商品の備考<|>管理番号</u,
  "downloaded and printed tags must contain values without visible item labels");
assert.doesNotMatch(downloadedTagSvg, /原価|売価|purchasePrice|salePrice/u, "downloaded tag SVG must not contain price data");
const blankNoteTagHtml = window.renderInventoryProductTagPanel({ ...inventory[0], note: "" });
assert.doesNotMatch(blankNoteTagHtml, /Preview inventory: monthly purchase-date distribution|記載なし/u,
  "blank product notes must stay empty on the product tag");
const blankNoteTagSvg = window.createInventoryProductTagSvg({ ...inventory[0], note: "" });
assert.doesNotMatch(blankNoteTagSvg, /Preview inventory: monthly purchase-date distribution|記載なし/u,
  "blank product notes must stay empty in downloaded and printed tags");
assert.equal(document.getElementById("itemDetailTagDownloadBtn").classList.contains("hidden"), false);
assert.equal(document.getElementById("itemDetailTagPrintBtn").classList.contains("hidden"), false);
assert.equal(document.getElementById("itemDetailEditBtn").classList.contains("hidden"), true);
assert.match(window.prBulkDownloadCSV.toString(), /'管理番号'.*'ブランド'.*'モデル'.*'型番'.*'シリアル'.*'原価'/u,
  "purchase-return CSV headers must use the unified inventory terminology");
assert.match(window.exportSlipCSV.toString(), /'委託先','管理番号','ブランド','モデル'/u,
  "consignment CSV headers must use 管理番号 and モデル");
window.switchItemDetailTab("info");

const serialVisibility = document.querySelector('#inv-column-panel input[value="serial"]');
serialVisibility.checked = false;
window._invColumnVisibilityChanged(serialVisibility);
assert.equal(document.querySelector('th[data-inv-col="serial"]').classList.contains("inv-col-hidden"), true);
assert.equal(document.querySelector('#inventoryTableBody td[data-inv-col="serial"]').classList.contains("inv-col-hidden"), true);
assert.equal(document.getElementById("inv-column-count").textContent, "15/17");

const purchaseVisibility = document.querySelector('#inv-column-panel input[value="purchasePrice"]');
const saleVisibility = document.querySelector('#inv-column-panel input[value="salePrice"]');
purchaseVisibility.checked = false;
window._invColumnVisibilityChanged(purchaseVisibility);
saleVisibility.checked = false;
window._invColumnVisibilityChanged(saleVisibility);
assert.equal(document.querySelector('th[data-inv-col="purchasePrice"]').classList.contains("inv-col-hidden"), true);
assert.equal(document.querySelector('th[data-inv-col="salePrice"]').classList.contains("inv-col-hidden"), true);
window._invShowAllColumns();
assert.equal(document.getElementById("inv-column-count").textContent, "17/17");
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
  if (["仕入返品", "仕入返品中", "return_pending"].includes(status)) return "仕入返品中";
  if (["取消", "取消済", "取り消し", "仕入返品済", "cancelled"].includes(status)) return "仕入返品済";
  return status;
};
const statusOrder = ["在庫中", "取置中", "委託中", "仕入返品中", "出荷済", "売上済", "仕入返品済", "保留"];
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

const originalEditImages = [...(inventory[0].images || [])];
const originalEditImageFiles = inventory[0].imageFiles;
inventory[0].images = ["/api/v1/product-files/fil_edit_1", "/api/v1/product-files/fil_edit_2"];
inventory[0].imageFiles = [
  { id: "fil_edit_1", url: inventory[0].images[0], originalName: "front.jpg", sortOrder: 0 },
  { id: "fil_edit_2", url: inventory[0].images[1], originalName: "back.jpg", sortOrder: 1 },
];
window.openItemEdit(inventory[0].code);
assert.equal(document.getElementById("ie-salePrice").value, "7,613", "inventory edit must show the USD value with thousands separators");
assert.equal((document.querySelector('label[for="ie-salePrice"]')?.textContent || document.getElementById("ie-salePrice").previousElementSibling?.textContent || "").trim(), "売価");
assert.equal(document.getElementById("ie-image-count").textContent, "2 / 10枚", "inventory edit must load the product's existing images");
assert.equal(document.querySelectorAll("#ie-image-grid .item-edit-image-card").length, 2);
window.moveItemEditImage(1, -1);
assert.match(document.querySelector("#ie-image-grid .item-edit-image-preview").src, /fil_edit_2$/u, "image order controls must update the preview order");
window.removeItemEditImage(1);
assert.equal(document.getElementById("ie-image-count").textContent, "1 / 10枚", "image deletion must remain staged in the edit modal");
window.handleItemEditImageFiles([new window.File(["image"], "replacement.webp", { type: "image/webp" })]);
assert.equal(document.getElementById("ie-image-count").textContent, "2 / 10枚", "valid images must be addable from inventory edit");
assert.match(document.getElementById("ie-image-grid").textContent, /replacement\.webp/u);
const inventoryPurchasePriceInput = document.getElementById("ie-purchasePrice");
inventoryPurchasePriceInput.value = "１２３４５６７";
window.priceFormatHandler(inventoryPurchasePriceInput);
assert.equal(inventoryPurchasePriceInput.value, "1,234,567", "integer money input must insert separators while typing");
assert.equal(window.getPriceValue(inventoryPurchasePriceInput), 1234567, "formatted integer money must retain its numeric value");
window.closeItemEdit();
inventory[0].images = originalEditImages;
inventory[0].imageFiles = originalEditImageFiles;
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
assert.equal(document.getElementById("pu-code").getAttribute("oninput"), "puManagementNumberInput(this)");
assert.ok(document.querySelector('button[onclick="openBarcodeScanner(\'product-registration\')"]'),
  "product registration management number must expose a QR camera button");
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

const singlePurchaseCode = "20991230001";
const inventoryCountBeforeSinglePurchase = inventory.length;
const purchaseSlipCountBeforeSinglePurchase = purchaseSlips.length;
document.getElementById("pu-date").value = "2099-12-30";
document.getElementById("pu-supplier").value = "S001";
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
assert.equal(savedSingleInventory.status, "在庫中");
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
document.getElementById("inv-f-code").dispatchEvent(new window.KeyboardEvent("keydown", {
  key: "Enter", bubbles: true, cancelable: true,
}));
assert.notEqual(document.getElementById("inv-result-area").style.display, "none",
  "pressing Enter in an inventory search field must execute the search");
assert.match(document.getElementById("inventoryTableBody").textContent, new RegExp(singlePurchaseCode),
  "inventory Enter search must render the matching inventory record");
window.navigateTo("sales-list");
window.switchSlipTab("purchase");
assert.equal(document.getElementById("slip-filter-status").value, "processing", "document status search must default to processing");
savedSinglePurchaseSlip.status = "処理済";
savedSinglePurchaseSlip.paidAt = null;
window.execSlipFilter();
assert.match(document.getElementById("slipListBody").textContent, new RegExp(savedSinglePurchaseSlip.id),
  "purchase-slip processing search must use unpaid document status instead of the internal confirmed status");
savedSinglePurchaseSlip.paidAt = "2099-12-30T12:00:00+09:00";
document.getElementById("slip-filter-status").value = "completed";
window.execSlipFilter();
assert.match(document.getElementById("slipListBody").textContent, new RegExp(savedSinglePurchaseSlip.id),
  "purchase-slip completed search must include paid documents");
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

const linkedPurchaseReturn = {
  id: "PR-RET-LINK-TEST", slipId: savedSinglePurchaseSlip.id, date: "2099-12-31", supplier: "S001", status: "承認済",
  items: [{ code: singlePurchaseCode, brand: savedSingleInventory.brand, model: savedSingleInventory.model, purchasePrice: 123000, status: "承認済" }],
};
window.eval("APP_DATA.purchaseReturns").push(linkedPurchaseReturn);
window.applyBusinessRecordState("purchasereturn", linkedPurchaseReturn);
assert.equal(savedSingleInventory.status, "仕入返品中", "purchase-return slips must reserve products from normal inventory use");
assert.equal(window.getPurchaseReturnProcessingStatus(linkedPurchaseReturn), "処理中", "purchase returns without tracking must remain processing");
window.switchSlipTab("purchasereturn");
assert.match(document.getElementById("slipListBody").textContent, /PR-RET-LINK-TEST/u);
assert.match(document.getElementById("slipListBody").textContent, /処理中/u, "purchase-return rows without tracking must display processing");
const purchaseReturnTrackingInput = document.getElementById("pr-list-tracking-PR-RET-LINK-TEST");
assert.ok(purchaseReturnTrackingInput, "purchase-return rows must expose a tracking-number confirmation field");
purchaseReturnTrackingInput.value = "TRACK-RETURN-001";
await window.prConfirmTrackingFromList("PR-RET-LINK-TEST");
assert.equal(savedSingleInventory.status, "仕入返品済", "confirming purchase-return tracking must complete the linked inventory return");
assert.equal(linkedPurchaseReturn.items[0].trackingNo, "TRACK-RETURN-001", "tracking is saved only by the explicit confirmation action");
assert.equal(window.getPurchaseReturnProcessingStatus(linkedPurchaseReturn), "処理済", "purchase returns with saved tracking must display processed");
assert.ok(document.querySelector('#inv-f-status option[value="仕入返品済"]'), "inventory status search must include completed purchase-return products");
window.resetInventorySearch();
assert.equal(document.getElementById("inv-f-status").value, "在庫中", "inventory search must default to in-stock products");
document.getElementById("inv-f-status").value = "";
window.execInventorySearch();
assert.equal(
  document.getElementById("inventoryCount").textContent.trim(),
  `${inventory.length} 件`,
  "the all-status inventory count must include every product, including completed purchase returns",
);
document.getElementById("inv-f-status").value = "仕入返品済";
window.execInventorySearch();
assert.equal(
  document.getElementById("inventoryCount").textContent.trim(),
  `${inventory.filter((item) => normalizeInventoryStatus(item.status) === "仕入返品済").length} 件`,
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
assert.ok(document.getElementById("pe-tax-overseas"), "purchase entry must expose an overseas purchase switch");
assert.equal(document.getElementById("pe-tax-domestic").getAttribute("aria-checked"), "true");
assert.match(document.getElementById("pe-tax-mode-badge").textContent, /消費税（10%）/u);
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
window.peViewSlip(staffDisplaySlip.id);
assert.equal(document.getElementById("peViewModalBody").textContent.includes(purchaseStaffRecord.name), true, "purchase-slip details must display the staff name");
assert.equal(document.getElementById("peViewModalBody").textContent.includes(purchaseStaffRecord.code), false, "purchase-slip details must hide the internal staff code");
document.getElementById("peViewModal").style.display = "none";
const purchaseSlipDetailHtml = window.buildSlipDetailBody("purchase", staffDisplaySlip);
assert.equal(purchaseSlipDetailHtml.includes("${purchaseTax.taxLabel}"), false,
  "purchase-slip details must render the tax label instead of exposing the template expression");
assert.match(purchaseSlipDetailHtml, /消費税（10%）/u);
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

document.getElementById("pe-date").value = "2099-12-27";
window.peOnDateChange();
document.getElementById("pe-supplier").value = window.eval("APP_DATA.suppliers[0].code");
document.getElementById("pe-staff").value = document.getElementById("pe-staff").options[1].value;
addLineCountInput.value = "1";
window.peAddLine();
window.eval(`
  _peSlipData.lines[0].sku = "SAVE-RESET-TEST";
  _peSlipData.lines[0].purchasePrice = 100000;
  _peSlipData.lines[0].salePrice = 1000;
  _peSlipData.lines[0].productDetail = { brand: "", model: "Reset Test" };
`);
document.querySelector(".pe-sku-input").value = "SAVE-RESET-TEST";
window.peSetPurchaseTaxMode("overseas");

let purchaseSaveCalls = 0;
let completePurchaseSave;
const pendingPurchaseSave = new Promise((resolve) => { completePurchaseSave = resolve; });
window.ZaikoAPI = {
  savePurchaseSlip: async () => {
    purchaseSaveCalls += 1;
    return pendingPurchaseSave;
  },
};
const firstPurchaseSave = window.peSave();
const duplicatePurchaseSave = window.peSave();
assert.equal(purchaseSaveCalls, 1, "a second click while saving must not submit another purchase slip");
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
assert.equal(document.getElementById("pe-staff").value, "", "successful purchase registration must clear the staff member");
assert.equal(document.querySelectorAll("#pe-detail-tbody tr").length, 0, "successful purchase registration must clear every detail row");
assert.equal(document.getElementById("pe-line-count").textContent, "0");
assert.equal(document.getElementById("pe-subtotal-purchase").textContent, "¥0");
assert.equal(document.getElementById("pe-total-purchase").textContent, "¥0");
assert.equal(document.getElementById("pe-total-sale").textContent, "$0");
assert.equal(addLineCountInput.value, "1");
assert.equal(document.getElementById("pe-tax-domestic").getAttribute("aria-checked"), "true", "a new purchase form must restore domestic tax mode");
delete window.ZaikoAPI;

addLineCountInput.value = "１２a";
window.peNormalizeAddCountInput(addLineCountInput);
assert.equal(addLineCountInput.value, "", "bulk line count must accept half-width digits only");

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
  Array.from({ length: 10 }, (_, index) => `20991231${String(index + 1).padStart(3, "0")}`),
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
  Array.from({ length: 9 }, (_, index) => `20991231${String(index + 1).padStart(3, "0")}`),
  "product codes must be reissued sequentially after a middle row is deleted",
);

const csvDownloadNames = [];
const originalPurchaseCSVAnchorClick = window.HTMLAnchorElement.prototype.click;
window.HTMLAnchorElement.prototype.click = function capturePurchaseCSVDownload() {
  csvDownloadNames.push(this.download);
};
const purchaseCSVTemplate = window.peDownloadCSVTemplate();
assert.equal(csvDownloadNames.at(-1), "仕入伝票CSVテンプレート.csv");
assert.deepEqual(
  [...purchaseCSVTemplate.rows[0]],
  [
    "マーキングコード", "売価", "形状コード", "SKU", "原価",
    "ブランドコード", "モデル", "型番", "シリアル", "素材コード",
    "駆動方式コード", "ベルト素材コード", "特徴・備考",
  ],
  "the purchase template must contain only user-entered fields and stable master codes",
);
assert.equal(purchaseCSVTemplate.rows.length, 1, "the purchase template must contain only the header row");
for (const removedColumn of ["仕入伝票番号", "伝票備考", "明細No.", "管理番号", "ブランド名", "コンディションコード", "コンディション名（参照用）", "BOX番号", "付属品", "BRACELET PARTS数量"]) {
  assert.equal(purchaseCSVTemplate.rows[0].includes(removedColumn), false, `${removedColumn} must not remain in the purchase CSV template`);
}
const marketCSVTemplate = window.marketDownloadTemplate();
assert.equal(csvDownloadNames.at(-1), "相場表テンプレート.csv");
assert.equal(marketCSVTemplate.rows.length, 2, "the market template must include one editable sample row");
assert.equal(marketCSVTemplate.rows[1].length, marketCSVTemplate.rows[0].length);
assert.match(marketCSVTemplate.rows[1][0], /^\d{4}-\d{2}-\d{2}$/u);
assert.match(marketCSVTemplate.rows[1][7], /^AUC-/u, "the market sample must use an auction master code");
const marketTemplateCSVText = marketCSVTemplate.rows.map((row) => row.map((value) => `"${String(value ?? "").replaceAll('"', '""')}"`).join(",")).join("\r\n");
const marketCountBeforeTemplateImport = window.eval("APP_DATA.marketPrices.length");
const marketTemplateImportResult = window.marketImportCsvText(marketTemplateCSVText, "相場表テンプレート.csv");
assert.deepEqual({ ...marketTemplateImportResult }, { imported: 0, staged: 1, skipped: 0 }, "the downloaded market sample must stage without saving");
assert.equal(window.eval("APP_DATA.marketPrices.length"), marketCountBeforeTemplateImport, "market sample staging must not change persisted rows");
assert.equal(document.getElementById("marketCsvPreviewModal").classList.contains("hidden"), false, "market CSV staging must open the detail preview");
window.marketCancelCSVImport();
assert.equal(document.getElementById("marketCsvPreviewModal").classList.contains("hidden"), true);
const purchaseCSVHeaders = [...purchaseCSVTemplate.rows[0]];
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
assert.match(bulkPurchase.id, /^PI-2099-[0-9]{4}$/u, "the purchase slip number must be generated without a CSV column");
assert.deepEqual([...bulkPurchase.lines.map((line) => line.lineNo)], [1, 2], "detail numbers must follow CSV row order automatically");
assert.deepEqual([...bulkPurchase.lines.map((line) => line.code)], ["20991229001", "20991229002"], "management numbers must be generated sequentially without a CSV column");
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
assert.match(savedPurchaseTemplateHtml, /付属品: BOX・GUARANTEE/u, "saved purchase slips must display accessories in the description");
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
assert.match(updatedPurchaseTemplateHtml, /更新後ブランド \/ 更新後モデル/u,
  "purchase print previews must show the latest linked product brand and model");
assert.match(updatedPurchaseTemplateHtml, /UPDATED-REF/u);
assert.match(updatedPurchaseTemplateHtml, /付属品: CASE/u);
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
window.URL.createObjectURL = () => "blob:saved-slip-test";
window.URL.revokeObjectURL = () => {};
window.HTMLAnchorElement.prototype.click = function captureSavedSlipDownload() {
  savedSlipDownloadNames.push(this.download);
};
const savedSlipCases = [
  { type: "purchase", record: savedPurchase, title: /仕入伝票プレビュー/u, filename: `${savedPurchase.id}_仕入伝票.html` },
  { type: "shipping", record: window.eval("APP_DATA.shipments[0]"), title: /出荷伝票プレビュー/u, filename: `${window.eval("APP_DATA.shipments[0].id")}_出荷伝票.html` },
  { type: "consignment", record: savedConsignment, title: /委託伝票プレビュー/u, filename: `${savedConsignment.id}_委託伝票.html` },
  { type: "sales", record: window.eval("APP_DATA.sales[0]"), title: /請求書（売上伝票）プレビュー/u, filename: `${window.eval("APP_DATA.sales[0].id")}_請求書.html` },
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
  window.downloadSavedSlipDocument();
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
assert.match(purchaseReturnInvoiceHtml, /付属品: BOX・GUARANTEE/u, "purchase return details must retain accessory information");
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
window.downloadPurchaseReturnDocument();
assert.equal(downloadedPurchaseReturnName, `${purchaseReturn.id}_仕入返品伝票.html`);
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
window.downloadSalesReturnDocument();
assert.equal(downloadedSalesReturnName, `${salesReturn.id}_売上返品伝票.html`);
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
assert.match(purchasePrintHtml, /付属品: BOX・GUARANTEE/u, "purchase slip previews must display accessories in the description");
assert.match(purchasePrintHtml, new RegExp(companyInfo.companyName));
assert.match(purchasePrintHtml, new RegExp(companyInfo.address));
assert.equal(purchasePrintHtml.includes("お振込先"), false, "purchase slips must not print bank details");
window.peclosePrintModal();
window.peSetPurchaseTaxMode("overseas");
assert.equal(document.getElementById("pe-tax-overseas").getAttribute("aria-checked"), "true");
assert.equal(document.querySelector('[data-role="pe-line-tax-label"]').textContent.trim(), "対象外");
assert.equal(document.getElementById("pe-total-tax").textContent.trim(), "対象外");
assert.equal(document.getElementById("pe-total-purchase").textContent, "$123,456");
assert.match(document.getElementById("pe-purchase-price-heading").textContent, /USD/u);
window.pePrintPreview();
const overseasPurchasePrintHtml = document.getElementById("pePrintPreviewContent").innerHTML;
assert.match(overseasPurchasePrintHtml, /対象外/u, "overseas purchase slip tax cells must be marked out of scope");
assert.match(overseasPurchasePrintHtml, /\$123,456/u, "overseas purchase slips must print the original USD amount");
assert.match(overseasPurchasePrintHtml, /USD（米ドル）/u, "overseas purchase slips must identify USD as the display currency");
assert.match(overseasPurchasePrintHtml, /発行日時/u, "purchase slips must label the issuance timestamp explicitly");
assert.equal(overseasPurchasePrintHtml.includes("¥135,801"), false, "overseas purchases must not add domestic consumption tax");
window.peclosePrintModal();
window.peSetPurchaseTaxMode("domestic");

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
assert.equal(purchaseListHeaders[purchaseListHeaders.indexOf("入金確認") + 1], "入金日付");
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
const shippingDownloadButton = document.querySelector('#page-shipping button[onclick="exportCSV(\'shipping\')"]');
assert.match(shippingDownloadButton.textContent, /ダウンロード/u);
assert.equal(shippingDownloadButton.textContent.includes("CSV"), false, "shipping button must be labelled as download");
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
assert.match(shippingPrintHtml, /商品詳細/u);
assert.match(shippingPrintHtml, /付属品:/u);
assert.match(shippingPrintHtml, /素材（本体）:/u);
assert.match(shippingPrintHtml, /駆動方式:/u);
assert.match(shippingPrintHtml, /ベルト素材:/u);
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

const sourceShipmentForClear = window.eval("APP_DATA.shipments.find(shipment => Array.isArray(shipment.items) && shipment.items.length > 0)");
assert.ok(sourceShipmentForClear, "a shipment with items is required for the sales-link clear test");
window.applyShipmentToSales(sourceShipmentForClear);
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
  id: "CO-2026-0999",
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
  "a consigned item may be sold only while its source consignment is linked");
assert.match(document.getElementById("sl-note").value, /委託伝票: CO-2026-0999/u);
document.getElementById("sl-id").value = "";
window.onSalesIdInput("");
assert.equal(window.eval("_salesSourceConsignmentId"), null);
assert.equal(window.canUseInventoryItemForSales(consignmentInventoryItem), false,
  "clearing the consignment reference must stop a consigned item from being sold manually");
window.eval("APP_DATA.consignments").pop();
consignmentInventoryItem.status = originalConsignmentItemStatus;

window.switchSalesEntryCurrency("USD");
const salesPriceInput = document.getElementById("sl-price-1");
salesPriceInput.value = "1000";
window.onSalesPriceInput(salesPriceInput);
assert.equal(salesPriceInput.value, "1,000");
assert.equal(salesPriceInput.dataset.entryCurrency, "USD");
assert.equal(document.getElementById("salesTotalDisplay").textContent.trim(), "$1,000");
assert.equal(document.getElementById("salesTotalTax").textContent.trim(), "対象外");
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
const salesDownloadButton = document.querySelector('#page-sales button[onclick="exportCSV(\'sales\')"]');
assert.match(salesDownloadButton.textContent, /ダウンロード/u);
assert.equal(salesDownloadButton.textContent.includes("CSV"), false, "sales button must be labelled as download");
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
assert.equal(document.getElementById("salesTotalTax").textContent.trim(), "対象外");
assert.equal(document.getElementById("salesTotalGrand").textContent.trim(), "$2,000");
assert.match(document.getElementById("salesTotalFxReference").textContent, /¥310,000/u);
const usdSalesPrintHtml = window.buildSalesSlip2PageHTML();
assert.match(usdSalesPrintHtml, /表示通貨：USD/u);
assert.match(usdSalesPrintHtml, /\$2,000/u);
assert.match(usdSalesPrintHtml, /税区分：対象外/u);
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
window.navigateTo("market");
const marketRows = window.eval("APP_DATA.marketPrices");
assert.equal(marketRows.length, inventory.length, "market preview must start from inventory rows");
assert.equal(document.querySelectorAll("#marketTableBody tr").length, inventory.length);
assert.doesNotMatch(document.getElementById("marketTableBody").textContent, /\$/u, "winning bid prices must be displayed in JPY only");
assert.match(document.querySelector("#page-market thead").textContent, /オークション開催日/u);
assert.match(document.querySelector("#page-market thead").textContent, /オークション名/u);
assert.match(document.querySelector("#page-market thead").textContent, /落札価格（JPY）/u);
assert.match(document.querySelector("#page-market thead").textContent, /備考/u);
assert.doesNotMatch(document.querySelector("#page-market thead").textContent, /シリアル|仕入日|仕入れ価格|担当者|ステータス|BOX/u);
assert.equal(document.getElementById("market-f-serial"), null, "market filters must not include serial number");
assert.equal(document.getElementById("market-f-staff"), null, "market filters must not include staff");
assert.equal(document.getElementById("market-f-supplier"), null, "market filters must not include supplier");
const marketAuctionFilter = document.getElementById("market-f-auction");
assert.notEqual(marketAuctionFilter, null, "market filters must include auction name");
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
assert.equal(document.getElementById("market-f-status"), null, "market filters must not include inventory status");
for (const removedKey of ["serial", "purchaseDate", "purchasePrice", "staff", "supplier", "status", "box"]) {
  assert.equal(document.querySelector(`#market-column-panel input[value="${removedKey}"]`), null, `${removedKey} must not be a market column option`);
  assert.equal(document.querySelector(`#marketTable [data-market-col="${removedKey}"]`), null, `${removedKey} must not be rendered in the market table`);
}
assert.equal(document.getElementById("market-purchase-jpy"), null);
assert.equal(document.getElementById("market-price-usd"), null);
assert.equal(document.getElementById("market-price-heading").textContent, "落札価格（JPY）");
assert.equal(document.querySelector('#marketTableBody td[data-market-col="marketPrice"]').textContent.trim(), "¥1,224,500");
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
assert.equal(document.getElementById("market-column-count").textContent, "8/10");
window.marketShowAllColumns();
assert.equal(document.getElementById("market-column-count").textContent, "10/10");
assert.equal(document.querySelectorAll("#marketTable .market-col-hidden").length, 0);
window.marketCloseColumnMenu();
const firstMarketId = marketRows[0].id;
window.marketOpenEdit(firstMarketId);
assert.equal(document.getElementById("marketEditModal").classList.contains("hidden"), false, "market edit modal must open");
assert.equal(document.getElementById("me-purchaseDate"), null, "market edit must not include purchase date");
assert.equal(document.getElementById("me-status"), null, "market edit must not include inventory status");
assert.equal(document.getElementById("me-box"), null, "market edit must not include BOX assignment");
assert.equal(document.getElementById("me-serial"), null, "market edit must not include serial number");
assert.equal(document.getElementById("me-staff"), null, "market edit must not include staff");
assert.equal(document.getElementById("me-purchasePrice"), null, "market edit must not include purchase price");
document.getElementById("me-marketPriceJpy").value = "1,259,500";
document.getElementById("me-auctionCode").value = "AUC-001";
document.getElementById("me-note").value = "落札結果を確認済み";
window.marketSaveEdit();
assert.equal(marketRows[0].marketPriceJpy, 1259500, "market edit must save JPY winning bid price");
assert.equal(marketRows[0].auctionName, "東京オークション");
assert.equal(marketRows[0].auctionCode, "AUC-001");
assert.equal(marketRows[0].note, "落札結果を確認済み");
assert.equal(document.getElementById("marketEditModal").classList.contains("hidden"), true, "market edit modal must close after save");

const csvResult = window.marketImportCsvText(
  "オークション開催日,ブランドコード,モデル,型番,オークションコード,落札価格（JPY）,SKU,付属品コード,備考\n" +
  "2026-08-01,BRD-001,自由入力モデル,自由入力型番,AUC-001,565750,FREE-SKU-001,ACC-001,自由入力備考",
  "market-test.csv",
);
const marketCountBeforeCsvPreview = marketRows.length;
assert.equal(csvResult.imported, 0);
assert.equal(csvResult.staged, 1);
assert.equal(csvResult.skipped, 0);
assert.equal(marketRows.length, marketCountBeforeCsvPreview, "market CSV staging must not append a row before confirmation");
assert.equal(document.getElementById("marketCsvPreviewModal").classList.contains("hidden"), false);
assert.match(document.getElementById("marketCsvPreviewBody").textContent, /ロレックス/u);
const marketConfirmResult = await window.marketConfirmCSVImport();
assert.equal(marketConfirmResult.imported, 1);
assert.equal(marketRows.length, marketCountBeforeCsvPreview + 1, "market CSV confirmation must append the staged row");
assert.equal(marketRows.at(-1).importDate, "2026-08-01");
assert.equal(marketRows.at(-1).auctionName, "東京オークション");
assert.equal(marketRows.at(-1).auctionCode, "AUC-001");
assert.equal(marketRows.at(-1).brand, "ロレックス");
assert.equal(marketRows.at(-1).accessories[0], "BOX");
assert.equal(marketRows.at(-1).marketPriceJpy, 565750);
assert.equal(marketRows.at(-1).note, "自由入力備考");
assert.equal(document.getElementById("marketImportSummary").classList.contains("show"), true);
assert.equal(document.getElementById("marketCsvPreviewModal").classList.contains("hidden"), true);

// 管理者／作業者の画面権限と、ログイン切替をまたぐ承認フロー
window.localStorage.removeItem("inv_approval_workflow_v2");
assert.ok(window.doAppLogin("buyer1", "buyer123"), "worker login must succeed");
assert.equal(window.isWorker(), true);
assert.equal(window.currentRoleLabel(), "作業者");
window.applyRoleUI();
assert.equal(document.querySelector(".sidebar-user div div:last-child").textContent, "作業者");
assert.equal(document.querySelector('.nav-item[data-page="market"]').style.display, "", "worker must see the market table");
assert.equal(document.querySelector('.nav-item[data-page="approval"]').style.display, "none", "worker must not see approval management");
assert.equal(document.querySelector('.nav-item[data-page="login-info"]'), null, "duplicate login information navigation must remain removed for workers");

window.navigateTo("approval");
assert.equal(document.getElementById("page-approval").classList.contains("hidden"), true, "worker direct navigation to approvals must be blocked");

window.navigateTo("master");
assert.equal(document.getElementById("adminAuthModal").classList.contains("hidden"), false, "worker master access must request admin authentication");
document.getElementById("adminAuthCode").value = window._getLocalAdminAccessCode(false).code;
window.submitAdminAuth();
assert.equal(document.getElementById("page-master").classList.contains("hidden"), false, "admin authentication must unlock master access once");

window.navigateTo("performance");
assert.match(document.getElementById("perf-supplier").textContent, /管理者の承認が必要/u);
const approvalCountBeforePerformance = window.eval("APP_DATA.approvalRequests.length");
window.renderPerformance();
const performanceApproval = window.getLatestPerformanceApproval();
assert.ok(performanceApproval, "worker performance access must create an approval request");
assert.equal(performanceApproval.status, "pending");
assert.equal(performanceApproval.requesterName, "山本 太郎");
assert.match(document.getElementById("perf-approval-status").textContent, /管理者の承認待ち/u);
window.renderPerformance();
assert.equal(window.eval("APP_DATA.approvalRequests.length"), approvalCountBeforePerformance + 1, "pending access request must not be duplicated");
assert.match(window.localStorage.getItem("inv_approval_workflow_v2"), new RegExp(performanceApproval.id));

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
assert.equal(window.approveRequest(performanceApproval.id), true);
assert.equal(window.approveRequest(workerEditApproval.id), true);
assert.equal(performanceApproval.status, "approved");
assert.equal(inventory[0].model, approvedWorkerEditModel, "approval must apply the requested inventory edit");
inventory[0].model = originalWorkerEditModel;
window.replayApprovedApprovalOperations();
assert.equal(inventory[0].model, approvedWorkerEditModel, "approved operations must replay after a page reload");

assert.ok(window.doAppLogin("buyer1", "buyer123"));
window.applyRoleUI();
window.navigateTo("performance");
assert.match(document.getElementById("perf-approval-status").textContent, /閲覧が承認されています/u);
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

assert.equal(document.querySelectorAll(".page-panel").length, 19);
assert.equal(document.querySelectorAll(".nav-item").length, 16);
assert.equal(document.querySelectorAll(".modal-overlay").length, 47);
const sidebarGroups = [...document.querySelectorAll(".sidebar-nav > .nav-group")];
assert.equal(sidebarGroups[0].querySelector(".nav-group-label").textContent.trim(), "メイン");
assert.deepEqual(
  [...sidebarGroups[0].querySelectorAll(".nav-item")].map((item) => item.dataset.page),
  ["dashboard", "market", "inventory", "purchase-entry", "purchase"],
  "main navigation order must follow the inventory workflow",
);
assert.equal(sidebarGroups[1].querySelector(".nav-group-label").textContent.trim(), "経理・会計");
assert.deepEqual(
  [...sidebarGroups[1].querySelectorAll(".nav-item")].map((item) => item.dataset.page),
  ["sales-list", "deleted-slips", "sales", "shipping", "consignment"],
  "accounting navigation must start with the document list",
);
assert.ok(document.getElementById("page-deleted-slips"), "deleted document archive page must be present");
assert.ok(document.getElementById("deletedSlipListBody"), "deleted document archive table must be present");
assert.ok(document.getElementById("page-consignment"), "consignment registration page must be present");
assert.ok(document.getElementById("sltab-consignment"), "document list must include the consignment tab");
const shipmentDetailTable = window.buildItemsTable(
  [{ code: "20260804004", brand: "グランドセイコー", model: "Heritage Collection", salePrice: 5975 }],
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

dom.window.close();

let guestHtml = guestHtmlSource.replace(/<link\b[^>]*>/gi, "");
guestHtml = guestHtml.replace(/<script\b[^>]*src="https?:\/\/[^\"]+"[^>]*><\/script>/gi, "");
for (const name of ["data.js", "guest_shared.js", "login_info.js", "guest.js"]) {
  const source = name === "guest.js"
    ? await readFile(path.join(referenceRoot, "js", name), "utf8")
    : scriptSources.get(name);
  const tag = `<script src="js/${name}"></script>`;
  assert.ok(guestHtml.includes(tag), `guest.html is missing ${tag}`);
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
assert.equal(publishedGuestItems.some(item => item.code === "20260303001"), false, "reserved products must not appear in the guest catalog");
assert.equal(guestDocument.querySelectorAll(".guest-product-card").length, 7);
assert.match(guestDocument.getElementById("guest-publish-summary").textContent, /公開BOX: 2/u);
assert.match(guestDocument.querySelector(".guest-product-price").textContent, /¥1,180,000/u);

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
