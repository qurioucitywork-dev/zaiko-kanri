import { useCallback, useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useOutletContext, useParams } from "react-router-dom";
import { api } from "./api";

const TODAY = new Date().toLocaleDateString("sv-SE");
const MASTER_KINDS = [
  ["brands", "ブランド"], ["materials", "素材"], ["movements", "駆動方式"],
  ["conditions", "コンディション"], ["accessories", "付属品"],
];

function useRemote(path) {
  const [data, setData] = useState(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const load = useCallback(async () => {
    if (!path) return;
    setLoading(true); setError("");
    try { setData(await api.get(path)); } catch (reason) { setError(reason.message); } finally { setLoading(false); }
  }, [path]);
  useEffect(() => { load(); }, [load]);
  return { data, setData, error, loading, reload: load };
}

function useReferences() {
  const [data, setData] = useState(null);
  const [error, setError] = useState("");
  useEffect(() => {
    Promise.all([
      api.get("/partners?includeInactive=false"), api.get("/purchase-staff"),
      ...MASTER_KINDS.map(([kind]) => api.get(`/masters/${kind}`)),
    ]).then(([partners, staff, ...masters]) => {
      const masterMap = Object.fromEntries(MASTER_KINDS.map(([kind], index) => [kind, masters[index]?.items || []]));
      setData({ partners: partners.items || [], staff: staff.items || [], masters: masterMap });
    }).catch((reason) => setError(reason.message));
  }, []);
  return { data, error };
}

function PageHeader({ title, description, actions }) {
  return <section className="page-heading"><div><h2>{title}</h2><p>{description}</p></div><div className="page-actions">{actions}</div></section>;
}

function Notice({ kind = "error", children }) { return <div className={`alert ${kind}`} role={kind === "error" ? "alert" : "status"}>{children}</div>; }
function Loading() { return <section className="card react-loading" aria-busy="true"><div /><div /><div /></section>; }
function Empty({ text = "対象データはありません。" }) { return <div className="empty-state"><strong>{text}</strong><p>検索条件や登録内容を確認してください。</p></div>; }
function Status({ value }) { return <span className={`status-badge ${value}`}>{statusLabel(value)}</span>; }
function InventoryStatus({ value }) {
  const label = value === "return_pending" ? "仕入返品中" : value === "cancelled" ? "仕入返品済" : statusLabel(value);
  return <span className={`status-badge ${value}`}>{label}</span>;
}
function CsvLink({ kind, children = "ダウンロード" }) { return <a className="button secondary" href={`/api/v1/exports/${kind}.csv`}>↓ {children}</a>; }
function TableCard({ title, description, count, columns, rows, empty = "対象データはありません。" }) {
  return <section className="card"><div className="card-header"><div><h3>{title}</h3><p>{description}</p></div><span className="badge">{count ?? rows.length}件</span></div>
    {rows.length ? <div className="table-wrap" tabIndex="0"><table><thead><tr>{columns.map((column) => <th key={column.key}>{column.label}</th>)}</tr></thead>
      <tbody>{rows.map((row, index) => <tr key={row.id || row.code || row.number || index}>{columns.map((column) => <td key={column.key} className={column.className || ""}>{column.render ? column.render(row) : row[column.key] || "—"}</td>)}</tr>)}</tbody></table></div> : <Empty text={empty} />}
  </section>;
}

function FormActions({ submitting, submitLabel = "保存", cancelTo }) {
  return <div className="form-actions">{cancelTo && <Link className="button ghost" to={cancelTo}>キャンセル</Link>}<button className="button primary" type="submit" disabled={submitting} aria-busy={submitting}>{submitting ? "保存中…" : submitLabel}</button></div>;
}
function Field({ label, children, helper }) { return <label className="field"><span>{label}</span>{children}{helper && <small>{helper}</small>}</label>; }

function partnerOptions(partners, roleType) {
  return (partners || []).flatMap((partner) => (partner.roles || []).filter((role) => role.roleType === roleType && role.isActive).map((role) => ({ code: role.roleCode, name: partner.legalName })));
}
function splitCodes(value) { return String(value || "").split(/[\s,、]+/).map((item) => item.trim()).filter(Boolean); }
function money(value, currency = "JPY") { return new Intl.NumberFormat("ja-JP", { style: "currency", currency, maximumFractionDigits: 0 }).format(Number(value) || 0); }
function dateTime(value) { return value ? new Intl.DateTimeFormat("ja-JP", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value)) : "—"; }
function statusLabel(value) { return ({ draft: "下書き", pending: "承認待ち", pending_approval: "承認待ち", approved: "承認済", returned: "差戻し", rejected: "却下", confirmed: "確定", cancelled: "取消", in_stock: "在庫中", reserved: "取置中", shipped: "出荷済", sold: "売上済", return_pending: "仕入返品中", active: "有効", inactive: "無効" })[value] || value || "—"; }

export function PurchasesPage() {
  const remote = useRemote("/purchases?limit=500");
  const { session } = useOutletContext();
  async function confirm(id) { if (!window.confirm("この仕入伝票を確定して在庫へ反映しますか？")) return; try { await api.post(`/purchases/${id}/confirm`, {}, session.csrfToken); remote.reload(); } catch (reason) { window.alert(reason.message); } }
  if (remote.loading) return <Loading />;
  return <><PageHeader title="仕入管理" description="仕入伝票を作成し、確定時に商品コードを固定採番して在庫へ反映します。" actions={<><CsvLink kind="purchases" /><Link className="button primary" to="/purchases/new">＋ 仕入登録</Link></>} />
    {remote.error && <Notice>{remote.error}</Notice>}<TableCard title="仕入伝票一覧" description="PostgreSQLへ保存された仕入伝票です。" rows={remote.data?.items || []} columns={[
      { key: "slipNumber", label: "伝票番号" }, { key: "purchaseDate", label: "仕入日" }, { key: "supplierName", label: "仕入先", render: (row) => <><strong>{row.supplierName}</strong><small className="table-sub">{row.supplierCode}</small></> },
      { key: "staffCode", label: "担当者" }, { key: "status", label: "状態", render: (row) => <Status value={row.status} /> },
      { key: "actions", label: "操作", render: (row) => row.status === "draft" ? <button className="button secondary small" onClick={() => confirm(row.id)}>確定</button> : "—" },
    ]} /></>;
}

const emptyPurchaseLine = () => ({ quantity: 1, sku: "", brandCode: "", modelNumber: "", referenceNumber: "", serialNumber: "", productType: "時計", materialCode: "", movementCode: "", conditionCode: "", accessoryCodes: [], unitCostMinor: 0, costCurrency: "JPY", baseSalePriceMinor: 0, baseSaleCurrency: "USD", notes: "" });

export function PurchaseForm() {
  const { session } = useOutletContext(); const navigate = useNavigate(); const refs = useReferences();
  const [header, setHeader] = useState({ supplierCode: "", staffCode: "", purchaseDate: TODAY, notes: "" });
  const [lines, setLines] = useState([emptyPurchaseLine()]); const [addCount, setAddCount] = useState(1); const [error, setError] = useState(""); const [submitting, setSubmitting] = useState(false);
  if (!refs.data) return refs.error ? <Notice>{refs.error}</Notice> : <Loading />;
  const suppliers = partnerOptions(refs.data.partners, "supplier");
  function updateLine(index, key, value) { setLines((current) => current.map((line, lineIndex) => lineIndex === index ? { ...line, [key]: value } : line)); }
  async function submit(event) { event.preventDefault(); setSubmitting(true); setError(""); try { const created = await api.post("/purchases", { ...header, lines: lines.map((line) => ({ ...line, quantity: Number(line.quantity), unitCostMinor: Number(line.unitCostMinor), baseSalePriceMinor: Number(line.baseSalePriceMinor) })) }, session.csrfToken); navigate("/purchases", { state: { created: created.slipNumber } }); } catch (reason) { setError(reason.message); } finally { setSubmitting(false); } }
  return <><PageHeader title="仕入登録" description="明細数をまとめて追加しても、確定時の商品コードは欠番なく固定採番されます。" /><form className="card business-form" onSubmit={submit}>{error && <Notice>{error}</Notice>}
    <fieldset><legend>伝票情報</legend><div className="form-grid three"><Field label="仕入先"><select required value={header.supplierCode} onChange={(event) => setHeader({ ...header, supplierCode: event.target.value })}><option value="">選択してください</option>{suppliers.map((item) => <option key={item.code} value={item.code}>{item.code} {item.name}</option>)}</select></Field>
      <Field label="仕入担当者"><select value={header.staffCode} onChange={(event) => setHeader({ ...header, staffCode: event.target.value })}><option value="">ログイン利用者</option>{refs.data.staff.map((item) => <option key={item.staffCode} value={item.staffCode}>{item.staffCode} {item.displayName}</option>)}</select></Field>
      <Field label="仕入日"><input type="date" required value={header.purchaseDate} onChange={(event) => setHeader({ ...header, purchaseDate: event.target.value })} /></Field></div></fieldset>
    <fieldset><legend>明細</legend><div className="line-toolbar"><Field label="追加行数"><input type="number" min="1" max="50" inputMode="numeric" value={addCount} onChange={(event) => setAddCount(Math.max(1, Math.min(50, Number(event.target.value) || 1)))} /></Field><button className="button secondary" type="button" onClick={() => setLines((current) => [...current, ...Array.from({ length: addCount }, emptyPurchaseLine)])}>＋ 明細追加</button></div>
      <div className="entry-lines">{lines.map((line, index) => <article className="entry-line" key={index}><header><strong>明細 {index + 1}</strong><button className="button ghost small" type="button" disabled={lines.length === 1} onClick={() => setLines((current) => current.filter((_, i) => i !== index))}>削除</button></header><div className="form-grid three">
        <Field label="数量"><input type="number" min="1" max="100" required value={line.quantity} onChange={(event) => updateLine(index, "quantity", event.target.value)} /></Field>
        <Field label="ブランド"><select required value={line.brandCode} onChange={(event) => updateLine(index, "brandCode", event.target.value)}><option value="">選択</option>{refs.data.masters.brands.map((item) => <option key={item.code} value={item.code}>{item.code} {item.name}</option>)}</select></Field>
        <Field label="モデル"><input value={line.modelNumber} onChange={(event) => updateLine(index, "modelNumber", event.target.value)} /></Field>
        <Field label="リファレンス"><input value={line.referenceNumber} onChange={(event) => updateLine(index, "referenceNumber", event.target.value)} /></Field>
        <Field label="シリアル"><input value={line.serialNumber} onChange={(event) => updateLine(index, "serialNumber", event.target.value)} /></Field>
        <Field label="コンディション"><select value={line.conditionCode} onChange={(event) => updateLine(index, "conditionCode", event.target.value)}><option value="">未設定</option>{refs.data.masters.conditions.map((item) => <option key={item.code} value={item.code}>{item.name}</option>)}</select></Field>
        <Field label="仕入価格（JPY）"><input type="number" min="0" required value={line.unitCostMinor} onChange={(event) => updateLine(index, "unitCostMinor", event.target.value)} /></Field>
        <Field label="売価（USD）"><input type="number" min="0" required value={line.baseSalePriceMinor} onChange={(event) => updateLine(index, "baseSalePriceMinor", event.target.value)} /></Field>
        <Field label="SKU"><input value={line.sku} onChange={(event) => updateLine(index, "sku", event.target.value)} /></Field>
      </div></article>)}</div></fieldset><Field label="備考"><textarea rows="3" value={header.notes} onChange={(event) => setHeader({ ...header, notes: event.target.value })} /></Field><FormActions submitting={submitting} submitLabel="仕入伝票を登録" cancelTo="/purchases" /></form></>;
}

export function ProductForm() {
  const { session } = useOutletContext(); const navigate = useNavigate(); const refs = useReferences(); const [error, setError] = useState(""); const [submitting, setSubmitting] = useState(false);
  const [form, setForm] = useState({ supplierCode: "", staffCode: "", purchaseDate: TODAY, sku: "", brandCode: "", modelNumber: "", referenceNumber: "", serialNumber: "", productType: "時計", materialCode: "", movementCode: "", conditionCode: "", accessoryCodes: [], costAmountMinor: 0, costCurrency: "JPY", baseSalePriceMinor: 0, baseSaleCurrency: "USD", notes: "" });
  if (!refs.data) return refs.error ? <Notice>{refs.error}</Notice> : <Loading />;
  async function submit(event) { event.preventDefault(); setSubmitting(true); setError(""); try { const created = await api.post("/products", { ...form, costAmountMinor: Number(form.costAmountMinor), baseSalePriceMinor: Number(form.baseSalePriceMinor) }, session.csrfToken); navigate(`/products/${created.product.id}`); } catch (reason) { setError(reason.message); } finally { setSubmitting(false); } }
  const set = (key) => (event) => setForm({ ...form, [key]: event.target.value });
  return <><PageHeader title="商品単品登録" description="登録時に仕入伝票を自動起票し、商品を在庫へ反映します。" /><form className="card business-form" onSubmit={submit}>{error && <Notice>{error}</Notice>}<div className="form-grid three">
    <Field label="仕入先"><select required value={form.supplierCode} onChange={set("supplierCode")}><option value="">選択</option>{partnerOptions(refs.data.partners, "supplier").map((item) => <option key={item.code} value={item.code}>{item.code} {item.name}</option>)}</select></Field>
    <Field label="仕入担当者"><select value={form.staffCode} onChange={set("staffCode")}><option value="">ログイン利用者</option>{refs.data.staff.map((item) => <option key={item.staffCode} value={item.staffCode}>{item.staffCode} {item.displayName}</option>)}</select></Field>
    <Field label="仕入日"><input type="date" required value={form.purchaseDate} onChange={set("purchaseDate")} /></Field>
    <Field label="ブランド"><select required value={form.brandCode} onChange={set("brandCode")}><option value="">選択</option>{refs.data.masters.brands.map((item) => <option key={item.code} value={item.code}>{item.code} {item.name}</option>)}</select></Field>
    <Field label="モデル"><input value={form.modelNumber} onChange={set("modelNumber")} /></Field><Field label="リファレンス"><input value={form.referenceNumber} onChange={set("referenceNumber")} /></Field>
    <Field label="シリアル"><input value={form.serialNumber} onChange={set("serialNumber")} /></Field><Field label="SKU"><input value={form.sku} onChange={set("sku")} /></Field>
    <Field label="素材"><select value={form.materialCode} onChange={set("materialCode")}><option value="">未設定</option>{refs.data.masters.materials.map((item) => <option key={item.code} value={item.code}>{item.name}</option>)}</select></Field>
    <Field label="駆動方式"><select value={form.movementCode} onChange={set("movementCode")}><option value="">未設定</option>{refs.data.masters.movements.map((item) => <option key={item.code} value={item.code}>{item.name}</option>)}</select></Field>
    <Field label="コンディション"><select value={form.conditionCode} onChange={set("conditionCode")}><option value="">未設定</option>{refs.data.masters.conditions.map((item) => <option key={item.code} value={item.code}>{item.name}</option>)}</select></Field>
    <Field label="仕入価格（JPY）"><input type="number" min="0" required value={form.costAmountMinor} onChange={set("costAmountMinor")} /></Field><Field label="売価（USD）"><input type="number" min="0" required value={form.baseSalePriceMinor} onChange={set("baseSalePriceMinor")} /></Field>
  </div><Field label="備考"><textarea rows="3" value={form.notes} onChange={set("notes")} /></Field><FormActions submitting={submitting} submitLabel="商品を登録" cancelTo="/products" /></form></>;
}

export function ProductDetail() {
  const { id } = useParams(); const { session } = useOutletContext(); const product = useRemote(`/products/${id}`); const files = useRemote(`/products/${id}/files`); const [error, setError] = useState(""); const [uploading, setUploading] = useState(false);
  async function upload(event) { const file = event.target.files?.[0]; if (!file) return; const body = new FormData(); body.append("file", file); setUploading(true); setError(""); try { await api.post(`/products/${id}/files`, body, session.csrfToken); files.reload(); } catch (reason) { setError(reason.message); } finally { setUploading(false); event.target.value = ""; } }
  if (product.loading) return <Loading />; if (product.error) return <Notice>{product.error}</Notice>; const item = product.data;
  return <><PageHeader title={item.productCode} description={`${item.brand} ${item.modelNumber || ""}`} actions={<Link className="button ghost" to="/products">← 在庫一覧</Link>} />{error && <Notice>{error}</Notice>}
    <section className="detail-grid"><article className="card detail-card"><h3>商品情報</h3><dl><dt>SKU</dt><dd>{item.sku || "—"}</dd><dt>リファレンス</dt><dd>{item.referenceNumber || "—"}</dd><dt>シリアル</dt><dd>{item.serialNumber || "—"}</dd><dt>仕入日</dt><dd>{item.purchaseDate}</dd><dt>原価</dt><dd>{money(item.costAmountMinor, item.costCurrency)}</dd><dt>売価</dt><dd>{money(item.baseSalePriceMinor, item.baseSaleCurrency)}</dd><dt>状態</dt><dd><InventoryStatus value={item.inventoryStatus} /></dd></dl></article>
      <article className="card detail-card"><h3>商品画像</h3><label className="button secondary upload-button">{uploading ? "アップロード中…" : "＋ 画像追加"}<input type="file" accept="image/jpeg,image/png,image/webp" onChange={upload} disabled={uploading} /></label><div className="image-grid">{(files.data?.items || []).map((file) => <img key={file.id} src={file.url} alt={`${item.productCode} 商品画像`} loading="lazy" />)}</div></article></section></>;
}

export function MarketPage() {
  const { session } = useOutletContext(); const remote = useRemote("/market-prices?limit=1000"); const refs = useReferences(); const [error, setError] = useState(""); const [busy, setBusy] = useState(false);
  const [form, setForm] = useState({ importDate: TODAY, brandCode: "", modelNumber: "", referenceNumber: "", conditionCode: "", purchasePriceMinor: 0, purchaseCurrency: "JPY", marketPriceMinor: 0, marketCurrency: "USD", source: "manual", notes: "" });
  async function create(event) { event.preventDefault(); setBusy(true); setError(""); try { await api.post("/market-prices", { ...form, purchasePriceMinor: Number(form.purchasePriceMinor), marketPriceMinor: Number(form.marketPriceMinor) }, session.csrfToken); setForm({ ...form, modelNumber: "", referenceNumber: "", purchasePriceMinor: 0, marketPriceMinor: 0 }); remote.reload(); } catch (reason) { setError(reason.message); } finally { setBusy(false); } }
  async function importCSV(event) { const file = event.target.files?.[0]; if (!file) return; const body = new FormData(); body.append("file", file); setBusy(true); setError(""); try { const preview = await api.post("/market-prices/imports/preview", body, session.csrfToken); if (preview.errorRows > 0) throw new Error(`CSVに${preview.errorRows}件のエラーがあります。`); await api.post(`/market-prices/imports/${preview.id}/commit`, {}, session.csrfToken); remote.reload(); } catch (reason) { setError(reason.message); } finally { setBusy(false); event.target.value = ""; } }
  if (remote.loading || !refs.data) return <Loading />;
  return <><PageHeader title="相場表" description="オークションデータを手入力またはCSVで取り込み、JPY/USDで管理します。" actions={<><CsvLink kind="market" /><label className="button primary upload-button">CSV取込<input type="file" accept=".csv,text/csv" onChange={importCSV} disabled={busy} /></label></>} />{(remote.error || refs.error || error) && <Notice>{remote.error || refs.error || error}</Notice>}
    <details className="card disclosure"><summary>相場データを手入力</summary><form className="business-form compact" onSubmit={create}><div className="form-grid three"><Field label="取込日"><input type="date" required value={form.importDate} onChange={(event) => setForm({ ...form, importDate: event.target.value })} /></Field><Field label="ブランド"><select required value={form.brandCode} onChange={(event) => setForm({ ...form, brandCode: event.target.value })}><option value="">選択</option>{refs.data.masters.brands.map((item) => <option key={item.code} value={item.code}>{item.name}</option>)}</select></Field><Field label="モデル"><input required value={form.modelNumber} onChange={(event) => setForm({ ...form, modelNumber: event.target.value })} /></Field><Field label="リファレンス"><input value={form.referenceNumber} onChange={(event) => setForm({ ...form, referenceNumber: event.target.value })} /></Field><Field label="コンディション"><select value={form.conditionCode} onChange={(event) => setForm({ ...form, conditionCode: event.target.value })}><option value="">未設定</option>{refs.data.masters.conditions.map((item) => <option key={item.code} value={item.code}>{item.name}</option>)}</select></Field><Field label="仕入価格（JPY）"><input type="number" min="0" value={form.purchasePriceMinor} onChange={(event) => setForm({ ...form, purchasePriceMinor: event.target.value })} /></Field><Field label="相場価格（USD）"><input type="number" min="0" value={form.marketPriceMinor} onChange={(event) => setForm({ ...form, marketPriceMinor: event.target.value })} /></Field></div><FormActions submitting={busy} /></form></details>
    <TableCard title="相場データ" description="固定ブランドコードと取込日で管理します。" rows={remote.data?.items || []} columns={[
      { key: "importDate", label: "取込日" }, { key: "brandName", label: "ブランド", render: (row) => <><strong>{row.brandName}</strong><small className="table-sub">{row.brandCode}</small></> }, { key: "modelNumber", label: "モデル" }, { key: "referenceNumber", label: "リファレンス" },
      { key: "purchasePriceMinor", label: "仕入価格", className: "money", render: (row) => money(row.purchasePriceMinor, row.purchaseCurrency) }, { key: "marketPriceMinor", label: "相場価格", className: "money", render: (row) => money(row.marketPriceMinor, row.marketCurrency) }, { key: "source", label: "取込元" },
    ]} /></>;
}

function TransactionList({ type }) {
  const config = {
    sales: { title: "売上管理", description: "売上伝票の登録・確定と請求金額を管理します。", path: "/sales?limit=500", create: "/sales/new", csv: "sales", date: "saleDate", partner: "buyerName", number: "slipNumber", amount: (row) => money(row.totalMinor, row.displayCurrency) },
    shipments: { title: "出荷管理", description: "出荷伝票、配送会社、追跡番号を管理します。", path: "/shipments?limit=500", create: "/shipments/new", csv: "shipments", date: "shipmentDate", partner: "buyerName", number: "slipNumber" },
    returns: { title: "返品・持ち帰り", description: "売上返品・持ち帰り・仕入返品を一つの伝票で管理します。", path: "/returns?limit=500", create: "/returns/new", csv: "returns", date: "transactionDate", partner: "buyerName", number: "slipNumber" },
  }[type];
  const remote = useRemote(config.path); const { session } = useOutletContext(); const [tracking, setTracking] = useState({});
  async function saveTracking(row) { try { await api.patch(`/${type}/${row.id}/tracking`, { carrier: tracking[row.id]?.carrier ?? row.carrier ?? "", trackingNumber: tracking[row.id]?.trackingNumber ?? row.trackingNumber ?? "" }, session.csrfToken); remote.reload(); } catch (reason) { window.alert(reason.message); } }
  if (remote.loading) return <Loading />;
  return <><PageHeader title={config.title} description={config.description} actions={<><CsvLink kind={config.csv} /><Link className="button primary" to={config.create}>＋ 新規登録</Link></>} />{remote.error && <Notice>{remote.error}</Notice>}
    <TableCard title={`${config.title}一覧`} rows={remote.data?.items || []} columns={[
      { key: config.number, label: "伝票番号" }, { key: config.date, label: "日付" }, { key: config.partner, label: "取引先", render: (row) => row[config.partner] || row.supplierName || "—" },
      ...(config.amount ? [{ key: "amount", label: "合計", className: "money", render: config.amount }] : []),
      ...(type !== "sales" ? [{ key: "tracking", label: "配送情報", render: (row) => <div className="tracking-edit"><input aria-label="配送会社" placeholder="配送会社" value={tracking[row.id]?.carrier ?? row.carrier ?? ""} onChange={(event) => setTracking({ ...tracking, [row.id]: { ...tracking[row.id], carrier: event.target.value } })} /><input aria-label="追跡番号" placeholder="追跡番号" value={tracking[row.id]?.trackingNumber ?? row.trackingNumber ?? ""} onChange={(event) => setTracking({ ...tracking, [row.id]: { ...tracking[row.id], trackingNumber: event.target.value } })} /><button className="button secondary small" onClick={() => saveTracking(row)}>保存</button></div> }] : []),
      { key: "status", label: "状態", render: (row) => <Status value={row.status} /> },
    ]} /></>;
}

export const SalesPage = () => <TransactionList type="sales" />;
export const ShipmentsPage = () => <TransactionList type="shipments" />;
export const ReturnsPage = () => <TransactionList type="returns" />;

function CodeLines({ value, onChange, label = "商品コード", price = false }) { return <Field label={label} helper={price ? "1行ごとに 商品コード,販売単価 を入力します。" : "空白・カンマ・改行で複数入力できます。"}><textarea required rows="5" value={value} onChange={onChange} placeholder={price ? "20260801001,1200" : "20260801001\n20260801002"} /></Field>; }

export function SaleForm() {
  const { session } = useOutletContext(); const navigate = useNavigate(); const refs = useReferences(); const [error, setError] = useState(""); const [submitting, setSubmitting] = useState(false);
  const [form, setForm] = useState({ buyerCode: "", saleDate: TODAY, displayCurrency: "USD", taxMode: "taxable", taxRateBasisPoints: 1000, notes: "", codeLines: "" });
  if (!refs.data) return <Loading />;
  async function submit(event) { event.preventDefault(); const lines = form.codeLines.split(/\r?\n/).map((line) => line.split(",")).filter(([code]) => code?.trim()).map(([productCode, price]) => ({ productCode: productCode.trim(), unitPriceMinor: Number(price) || 0 })); setSubmitting(true); setError(""); try { await api.post("/sales", { ...form, taxRateBasisPoints: Number(form.taxRateBasisPoints), lines, codeLines: undefined }, session.csrfToken); navigate("/sales"); } catch (reason) { setError(reason.message); } finally { setSubmitting(false); } }
  return <><PageHeader title="売上伝票登録" description="USDを基準に、販売金額・税額・合計をJPY/USDで固定保存します。" /><form className="card business-form" onSubmit={submit}>{error && <Notice>{error}</Notice>}<div className="form-grid three"><Field label="販売先"><select required value={form.buyerCode} onChange={(event) => setForm({ ...form, buyerCode: event.target.value })}><option value="">選択</option>{partnerOptions(refs.data.partners, "buyer").map((item) => <option key={item.code} value={item.code}>{item.code} {item.name}</option>)}</select></Field><Field label="売上日"><input type="date" required value={form.saleDate} onChange={(event) => setForm({ ...form, saleDate: event.target.value })} /></Field><Field label="表示通貨"><select value={form.displayCurrency} onChange={(event) => setForm({ ...form, displayCurrency: event.target.value })}><option value="USD">USD</option><option value="JPY">JPY</option></select></Field><Field label="税区分"><select value={form.taxMode} onChange={(event) => setForm({ ...form, taxMode: event.target.value })}><option value="taxable">課税</option><option value="exempt">免税</option></select></Field></div><CodeLines price value={form.codeLines} onChange={(event) => setForm({ ...form, codeLines: event.target.value })} /><Field label="備考"><textarea rows="3" value={form.notes} onChange={(event) => setForm({ ...form, notes: event.target.value })} /></Field><FormActions submitting={submitting} submitLabel="売上伝票を登録" cancelTo="/sales" /></form></>;
}

export function ShipmentForm() {
  const { session } = useOutletContext(); const navigate = useNavigate(); const refs = useReferences(); const [error, setError] = useState(""); const [submitting, setSubmitting] = useState(false);
  const [form, setForm] = useState({ buyerCode: "", salesSlipNumber: "", shipmentDate: TODAY, recipientName: "", recipientAddress: "", carrier: "", trackingNumber: "", notes: "", productCodesText: "" });
  if (!refs.data) return <Loading />;
  async function submit(event) { event.preventDefault(); setSubmitting(true); setError(""); try { await api.post("/shipments", { ...form, productCodes: splitCodes(form.productCodesText), productCodesText: undefined }, session.csrfToken); navigate("/shipments"); } catch (reason) { setError(reason.message); } finally { setSubmitting(false); } }
  const set = (key) => (event) => setForm({ ...form, [key]: event.target.value });
  return <><PageHeader title="出荷伝票登録" description="確定後は在庫を出荷済みにし、ゲスト公開から自動除外します。" /><form className="card business-form" onSubmit={submit}>{error && <Notice>{error}</Notice>}<div className="form-grid three"><Field label="販売先"><select required value={form.buyerCode} onChange={set("buyerCode")}><option value="">選択</option>{partnerOptions(refs.data.partners, "buyer").map((item) => <option key={item.code} value={item.code}>{item.code} {item.name}</option>)}</select></Field><Field label="出荷日"><input type="date" required value={form.shipmentDate} onChange={set("shipmentDate")} /></Field><Field label="売上伝票番号"><input value={form.salesSlipNumber} onChange={set("salesSlipNumber")} /></Field><Field label="受取人"><input value={form.recipientName} onChange={set("recipientName")} /></Field><Field label="配送会社"><input value={form.carrier} onChange={set("carrier")} /></Field><Field label="追跡番号"><input value={form.trackingNumber} onChange={set("trackingNumber")} /></Field></div><Field label="配送先住所"><input value={form.recipientAddress} onChange={set("recipientAddress")} /></Field><CodeLines value={form.productCodesText} onChange={set("productCodesText")} /><Field label="備考"><textarea rows="3" value={form.notes} onChange={set("notes")} /></Field><FormActions submitting={submitting} submitLabel="出荷伝票を登録" cancelTo="/shipments" /></form></>;
}

export function ReturnForm() {
  const { session } = useOutletContext(); const navigate = useNavigate(); const refs = useReferences(); const [error, setError] = useState(""); const [submitting, setSubmitting] = useState(false);
  const [form, setForm] = useState({ operationType: "return", transactionDate: TODAY, buyerCode: "", supplierCode: "", sourcePurchaseSlipNumber: "", reason: "", notes: "", carrier: "", trackingNumber: "", productCodesText: "" });
  if (!refs.data) return <Loading />;
  async function submit(event) { event.preventDefault(); setSubmitting(true); setError(""); try { await api.post("/returns", { ...form, productCodes: splitCodes(form.productCodesText), productCodesText: undefined }, session.csrfToken); navigate("/returns"); } catch (reason) { setError(reason.message); } finally { setSubmitting(false); } }
  const set = (key) => (event) => setForm({ ...form, [key]: event.target.value }); const purchaseReturn = form.operationType === "purchase_return";
  return <><PageHeader title="返品・持ち帰り登録" description="処理区分に応じて在庫状態と元伝票を連動します。" /><form className="card business-form" onSubmit={submit}>{error && <Notice>{error}</Notice>}<div className="form-grid three"><Field label="処理区分"><select value={form.operationType} onChange={set("operationType")}><option value="return">売上返品</option><option value="takeout">持ち帰り</option><option value="purchase_return">仕入返品</option></select></Field><Field label="処理日"><input type="date" required value={form.transactionDate} onChange={set("transactionDate")} /></Field>{purchaseReturn ? <><Field label="仕入先"><select required value={form.supplierCode} onChange={set("supplierCode")}><option value="">選択</option>{partnerOptions(refs.data.partners, "supplier").map((item) => <option key={item.code} value={item.code}>{item.code} {item.name}</option>)}</select></Field><Field label="元仕入伝票番号"><input required value={form.sourcePurchaseSlipNumber} onChange={set("sourcePurchaseSlipNumber")} /></Field></> : <Field label="販売先"><select value={form.buyerCode} onChange={set("buyerCode")}><option value="">未指定</option>{partnerOptions(refs.data.partners, "buyer").map((item) => <option key={item.code} value={item.code}>{item.code} {item.name}</option>)}</select></Field>}<Field label="配送会社"><input value={form.carrier} onChange={set("carrier")} /></Field><Field label="追跡番号"><input value={form.trackingNumber} onChange={set("trackingNumber")} /></Field></div><CodeLines value={form.productCodesText} onChange={set("productCodesText")} /><Field label="理由"><input required value={form.reason} onChange={set("reason")} /></Field><Field label="備考"><textarea rows="3" value={form.notes} onChange={set("notes")} /></Field><FormActions submitting={submitting} submitLabel="返品伝票を登録" cancelTo="/returns" /></form></>;
}

export function DocumentsPage() {
  const { session } = useOutletContext(); const documents = useRemote("/documents?limit=1000"); const history = useRemote("/document-events?limit=500"); const company = useRemote("/company"); const [printTarget, setPrintTarget] = useState(null); const [error, setError] = useState("");
  async function printDocument(row) { setPrintTarget(row); setError(""); try { await api.post("/document-events", { documentType: row.documentType, documentId: row.id, documentNumber: row.number, action: "print", outputFormat: "html", fileName: `${row.number}.html`, metadata: { status: row.status } }, session.csrfToken); history.reload(); setTimeout(() => window.print(), 80); } catch (reason) { setError(reason.message); } }
  if (documents.loading || history.loading || company.loading) return <Loading />;
  return <><PageHeader title="伝票一覧" description="仕入・出荷・売上・返品を横断し、印刷・ダウンロード履歴を保存します。" actions={<CsvLink kind="documents" />} />{(documents.error || history.error || company.error || error) && <Notice>{documents.error || history.error || company.error || error}</Notice>}
    <TableCard title="統合伝票" rows={documents.data?.items || []} columns={[{ key: "documentType", label: "種別", render: (row) => documentTypeLabel(row.documentType) }, { key: "number", label: "伝票番号" }, { key: "date", label: "日付" }, { key: "partnerName", label: "取引先", render: (row) => <>{row.partnerName}<small className="table-sub">{row.partnerCode}</small></> }, { key: "amount", label: "金額", className: "money", render: (row) => row.totalUsd ? money(row.totalUsd, "USD") : row.totalJpy ? money(row.totalJpy, "JPY") : "—" }, { key: "status", label: "状態", render: (row) => <Status value={row.status} /> }, { key: "print", label: "帳票", render: (row) => <button className="button secondary small" onClick={() => printDocument(row)}>印刷</button> }]} />
    <TableCard title="帳票発行履歴" description="印刷・CSVダウンロードの操作者と日時をDBへ保存しています。" rows={history.data?.items || []} columns={[{ key: "createdAt", label: "発行日時", render: (row) => dateTime(row.createdAt) }, { key: "documentType", label: "種別", render: (row) => documentTypeLabel(row.documentType) }, { key: "documentNumber", label: "伝票番号" }, { key: "action", label: "操作", render: (row) => row.action === "print" ? "印刷" : "ダウンロード" }, { key: "outputFormat", label: "形式", render: (row) => row.outputFormat.toUpperCase() }, { key: "createdByName", label: "操作者" }, { key: "fileName", label: "ファイル名" }]} />
    {printTarget && <article className="document-print" aria-hidden="true"><header><div><h1>{documentTypeLabel(printTarget.documentType)}</h1><p>{printTarget.number}</p></div><div><strong>{company.data?.companyName}</strong><p>〒{company.data?.postalCode} {company.data?.address}</p><p>{company.data?.phone}</p></div></header><dl><dt>日付</dt><dd>{printTarget.date}</dd><dt>取引先</dt><dd>{printTarget.partnerName}（{printTarget.partnerCode}）</dd><dt>状態</dt><dd>{statusLabel(printTarget.status)}</dd></dl><div className="document-total"><span>金額</span><strong>{printTarget.totalUsd ? money(printTarget.totalUsd, "USD") : money(printTarget.totalJpy, "JPY")}</strong></div></article>}
  </>;
}
function documentTypeLabel(value) { return ({ purchase: "仕入伝票", sale: "請求書", shipment: "出荷伝票", return: "返品伝票", purchase_return: "仕入返品伝票", inventory: "在庫一覧", market: "相場表", documents: "伝票一覧", stocktake: "棚卸" })[value] || value; }

export function PurchaseRequestsPage() {
  const { session } = useOutletContext(); const remote = useRemote("/purchase-requests"); async function decide(id, decision) { const note = window.prompt("処理メモ（任意）", "") ?? ""; try { await api.post(`/purchase-requests/${id}/${decision}`, { note }, session.csrfToken); remote.reload(); } catch (reason) { window.alert(reason.message); } }
  if (remote.loading) return <Loading />;
  return <><PageHeader title="購入依頼・取置" description="ゲストから届いた購入リクエストを承認すると在庫を排他取置します。" />{remote.error && <Notice>{remote.error}</Notice>}<TableCard title="購入リクエスト" rows={remote.data?.items || []} columns={[{ key: "requestNumber", label: "依頼番号" }, { key: "requestedAt", label: "依頼日時", render: (row) => dateTime(row.requestedAt) }, { key: "buyerName", label: "購入者", render: (row) => <>{row.buyerName}<small className="table-sub">{row.buyerCode}</small></> }, { key: "productCode", label: "商品", render: (row) => <>{row.productCode}<small className="table-sub">{row.brand} {row.modelNumber}</small></> }, { key: "status", label: "状態", render: (row) => <Status value={row.status} /> }, { key: "actions", label: "操作", render: (row) => row.status === "pending" ? <div className="inline-actions"><button className="button primary small" onClick={() => decide(row.id, "approve")}>承認</button><button className="button ghost small" onClick={() => decide(row.id, "reject")}>却下</button></div> : "—" }]} /></>;
}

export function ApprovalsPage() {
  const { session } = useOutletContext(); const remote = useRemote("/approvals"); async function decide(id, decision) { const note = window.prompt("承認コメント（任意）", "") ?? ""; try { await api.post(`/approvals/${id}/${decision}`, { note }, session.csrfToken); remote.reload(); } catch (reason) { window.alert(reason.message); } }
  if (remote.loading) return <Loading />;
  return <><PageHeader title="承認管理" description="作業者の申請を管理者が承認・差戻し・却下します。" />{remote.error && <Notice>{remote.error}</Notice>}<TableCard title="承認申請" rows={remote.data?.items || []} columns={[{ key: "approvalType", label: "申請種別" }, { key: "requesterName", label: "申請者" }, { key: "requestedAction", label: "申請操作" }, { key: "requestedAt", label: "申請日時", render: (row) => dateTime(row.requestedAt) }, { key: "status", label: "状態", render: (row) => <Status value={row.status} /> }, { key: "actions", label: "操作", render: (row) => row.status === "pending" && session.user.role === "admin" ? <div className="inline-actions"><button className="button primary small" onClick={() => decide(row.id, "approve")}>承認</button><button className="button secondary small" onClick={() => decide(row.id, "return")}>差戻し</button><button className="button ghost small" onClick={() => decide(row.id, "reject")}>却下</button></div> : "—" }]} /></>;
}

export function BoxesPage() {
  const { session } = useOutletContext(); const remote = useRemote("/boxes"); const [editing, setEditing] = useState(null); const [error, setError] = useState("");
  async function save(event) { event.preventDefault(); setError(""); try { await api.put(`/boxes/${editing.boxCode}`, { name: editing.name, isActive: editing.isActive, buyerCodes: splitCodes(editing.buyerCodesText), productCodes: splitCodes(editing.productCodesText) }, session.csrfToken); setEditing(null); remote.reload(); } catch (reason) { setError(reason.message); } }
  if (remote.loading) return <Loading />;
  return <><PageHeader title="ゲスト公開BOX" description="公開先企業コードと商品コードをBOX単位で管理します。商品は現在在庫を常時再照合します。" />{(remote.error || error) && <Notice>{remote.error || error}</Notice>}<TableCard title="BOX一覧" rows={remote.data?.items || []} columns={[{ key: "boxCode", label: "BOX番号" }, { key: "name", label: "BOX名" }, { key: "buyerCodes", label: "公開先", render: (row) => (row.buyerCodes || []).join(", ") || "非公開" }, { key: "productCodes", label: "商品数", render: (row) => `${(row.productCodes || []).length}点` }, { key: "isActive", label: "状態", render: (row) => <Status value={row.isActive ? "active" : "inactive"} /> }, { key: "action", label: "操作", render: (row) => <button className="button secondary small" onClick={() => setEditing({ ...row, buyerCodesText: (row.buyerCodes || []).join("\n"), productCodesText: (row.productCodes || []).join("\n") })}>編集</button> }]} />
    {editing && <form className="card business-form" onSubmit={save}><div className="card-header"><h3>{editing.boxCode}を編集</h3><button className="button ghost small" type="button" onClick={() => setEditing(null)}>閉じる</button></div><div className="form-grid two"><Field label="BOX名"><input required value={editing.name} onChange={(event) => setEditing({ ...editing, name: event.target.value })} /></Field><Field label="公開状態"><select value={String(editing.isActive)} onChange={(event) => setEditing({ ...editing, isActive: event.target.value === "true" })}><option value="true">有効</option><option value="false">無効</option></select></Field><Field label="公開先コード"><textarea rows="6" value={editing.buyerCodesText} onChange={(event) => setEditing({ ...editing, buyerCodesText: event.target.value })} /></Field><Field label="商品コード"><textarea rows="6" value={editing.productCodesText} onChange={(event) => setEditing({ ...editing, productCodesText: event.target.value })} /></Field></div><FormActions submitLabel="BOXを保存" /></form>}
  </>;
}

export function MastersPage() {
  const { session } = useOutletContext(); const [kind, setKind] = useState("brands"); const remote = useRemote(`/masters/${kind}?includeInactive=true`); const [form, setForm] = useState({ code: "", name: "" }); const [error, setError] = useState("");
  async function create(event) { event.preventDefault(); setError(""); try { await api.post(`/masters/${kind}`, { ...form, sortOrder: 0 }, session.csrfToken); setForm({ code: "", name: "" }); remote.reload(); } catch (reason) { setError(reason.message); } }
  async function toggle(item) { try { await api.patch(`/masters/${kind}/${item.id}`, { isActive: !item.isActive }, session.csrfToken); remote.reload(); } catch (reason) { setError(reason.message); } }
  return <><PageHeader title="マスタ登録" description="ブランド・素材・駆動方式・コンディション・付属品を固定コードで共通利用します。" />{error && <Notice>{error}</Notice>}<div className="tab-list" role="tablist">{MASTER_KINDS.map(([value, label]) => <button key={value} role="tab" aria-selected={kind === value} className={kind === value ? "active" : ""} onClick={() => setKind(value)}>{label}</button>)}</div>
    <form className="card inline-create" onSubmit={create}><Field label="固定コード"><input required value={form.code} onChange={(event) => setForm({ ...form, code: event.target.value.toUpperCase() })} /></Field><Field label="名称"><input required value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} /></Field><button className="button primary" type="submit">＋ 新規追加</button></form>
    {remote.loading ? <Loading /> : remote.error ? <Notice>{remote.error}</Notice> : <TableCard title={MASTER_KINDS.find(([value]) => value === kind)?.[1]} rows={remote.data?.items || []} columns={[{ key: "code", label: "固定コード" }, { key: "name", label: "名称" }, { key: "isActive", label: "状態", render: (row) => <Status value={row.isActive ? "active" : "inactive"} /> }, { key: "action", label: "操作", render: (row) => <button className="button secondary small" onClick={() => toggle(row)}>{row.isActive ? "無効化" : "有効化"}</button> }]} />}
  </>;
}

export function PartnersPage() {
  const { session } = useOutletContext(); const remote = useRemote("/partners?includeInactive=true"); const [error, setError] = useState(""); const [form, setForm] = useState({ legalName: "", representativeName: "", email: "", phone: "", address: "", invoiceNumber: "", buyer: true, supplier: false });
  async function create(event) { event.preventDefault(); const roles = [form.buyer && { roleType: "buyer", roleCode: "", isActive: true }, form.supplier && { roleType: "supplier", roleCode: "", isActive: true }].filter(Boolean); setError(""); try { await api.post("/partners", { legalName: form.legalName, representativeName: form.representativeName, email: form.email, phone: form.phone, address: form.address, invoiceNumber: form.invoiceNumber, roles }, session.csrfToken); setForm({ ...form, legalName: "", representativeName: "", email: "", phone: "", address: "", invoiceNumber: "" }); remote.reload(); } catch (reason) { setError(reason.message); } }
  if (remote.loading) return <Loading />;
  return <><PageHeader title="取引先会社" description="販売先・仕入先・ゲスト会社を共通会社コードで相互参照します。" />{(remote.error || error) && <Notice>{remote.error || error}</Notice>}<details className="card disclosure"><summary>取引先を新規登録</summary><form className="business-form compact" onSubmit={create}><div className="form-grid three"><Field label="会社名"><input required value={form.legalName} onChange={(event) => setForm({ ...form, legalName: event.target.value })} /></Field><Field label="代表者名"><input value={form.representativeName} onChange={(event) => setForm({ ...form, representativeName: event.target.value })} /></Field><Field label="メール"><input type="email" value={form.email} onChange={(event) => setForm({ ...form, email: event.target.value })} /></Field><Field label="電話番号"><input value={form.phone} onChange={(event) => setForm({ ...form, phone: event.target.value })} /></Field><Field label="インボイス番号"><input value={form.invoiceNumber} onChange={(event) => setForm({ ...form, invoiceNumber: event.target.value })} /></Field></div><div className="check-row"><label><input type="checkbox" checked={form.buyer} onChange={(event) => setForm({ ...form, buyer: event.target.checked })} />販売先取引</label><label><input type="checkbox" checked={form.supplier} onChange={(event) => setForm({ ...form, supplier: event.target.checked })} />仕入先取引</label></div><FormActions submitLabel="取引先を登録" /></form></details>
    <TableCard title="取引先一覧" rows={remote.data?.items || []} columns={[{ key: "partnerCode", label: "会社コード" }, { key: "legalName", label: "会社名" }, { key: "roles", label: "取引区分・コード", render: (row) => (row.roles || []).map((role) => `${role.roleType === "buyer" ? "販売" : "仕入"}:${role.roleCode}`).join(" / ") }, { key: "invoiceNumber", label: "インボイス番号" }, { key: "email", label: "メール" }, { key: "phone", label: "電話番号" }, { key: "status", label: "状態", render: (row) => <Status value={row.status} /> }]} /></>;
}

export function UsersPage() {
  const { session } = useOutletContext(); const users = useRemote("/users?includeInactive=true"); const partners = useRemote("/partners?includeInactive=false"); const [error, setError] = useState(""); const [form, setForm] = useState({ username: "", password: "", displayName: "", email: "", role: "worker", staffCode: "", isPurchaseStaff: true, guestCode: "", buyerCode: "" });
  async function create(event) { event.preventDefault(); setError(""); try { await api.post("/users", form, session.csrfToken); setForm({ ...form, username: "", password: "", displayName: "", email: "", staffCode: "", guestCode: "", buyerCode: "" }); users.reload(); } catch (reason) { setError(reason.message); } }
  async function toggle(row) { try { await api.patch(`/users/${row.id}`, { isActive: !row.isActive }, session.csrfToken); users.reload(); } catch (reason) { setError(reason.message); } }
  if (users.loading || partners.loading) return <Loading />;
  return <><PageHeader title="パスワード管理" description="管理者・作業者・ゲストのログイン情報を一元管理します。" />{(users.error || partners.error || error) && <Notice>{users.error || partners.error || error}</Notice>}<details className="card disclosure"><summary>アカウントを新規追加</summary><form className="business-form compact" onSubmit={create}><div className="form-grid three"><Field label="区分"><select value={form.role} onChange={(event) => setForm({ ...form, role: event.target.value })}><option value="admin">管理者</option><option value="worker">作業者</option><option value="guest">ゲスト</option></select></Field><Field label="ログインID"><input required value={form.username} onChange={(event) => setForm({ ...form, username: event.target.value })} /></Field><Field label="初期パスワード"><input type="password" minLength="8" required value={form.password} onChange={(event) => setForm({ ...form, password: event.target.value })} /></Field><Field label="表示名"><input required value={form.displayName} onChange={(event) => setForm({ ...form, displayName: event.target.value })} /></Field><Field label="メール"><input type="email" value={form.email} onChange={(event) => setForm({ ...form, email: event.target.value })} /></Field>{form.role === "guest" ? <><Field label="ゲストコード"><input value={form.guestCode} onChange={(event) => setForm({ ...form, guestCode: event.target.value })} /></Field><Field label="販売先コード"><select required value={form.buyerCode} onChange={(event) => setForm({ ...form, buyerCode: event.target.value })}><option value="">選択</option>{partnerOptions(partners.data?.items, "buyer").map((item) => <option key={item.code} value={item.code}>{item.code} {item.name}</option>)}</select></Field></> : <Field label="仕入担当者コード"><input value={form.staffCode} onChange={(event) => setForm({ ...form, staffCode: event.target.value })} /></Field>}</div><FormActions submitLabel="アカウントを追加" /></form></details>
    <TableCard title="ログインアカウント" rows={users.data?.items || []} columns={[{ key: "username", label: "ログインID" }, { key: "displayName", label: "表示名・会社", render: (row) => <>{row.displayName}<small className="table-sub">{row.companyName || ""}</small></> }, { key: "role", label: "区分", render: (row) => ({ admin: "管理者", worker: "作業者", guest: "ゲスト" })[row.role] }, { key: "code", label: "固定コード", render: (row) => row.staffCode || row.guestCode || "—" }, { key: "buyerCode", label: "販売先コード" }, { key: "email", label: "メール" }, { key: "isActive", label: "状態", render: (row) => <Status value={row.isActive ? "active" : "inactive"} /> }, { key: "action", label: "操作", render: (row) => <button className="button secondary small" onClick={() => toggle(row)}>{row.isActive ? "停止" : "再開"}</button> }]} /></>;
}

export function SettingsPage() {
  const { session } = useOutletContext(); const settings = useRemote("/settings"); const companyRemote = useRemote("/company"); const rates = useRemote("/exchange-rates?limit=100"); const [company, setCompany] = useState(null); const [error, setError] = useState("");
  useEffect(() => { if (companyRemote.data) setCompany(companyRemote.data); }, [companyRemote.data]);
  async function saveCompany(event) { event.preventDefault(); setError(""); try { const saved = await api.put("/company", company, session.csrfToken); setCompany(saved); } catch (reason) { setError(reason.message); } }
  async function saveSetting(item, value) { try { await api.put(`/settings/${encodeURIComponent(item.key)}`, { value }, session.csrfToken); settings.reload(); } catch (reason) { setError(reason.message); } }
  async function addRate(event) { event.preventDefault(); const form = new FormData(event.currentTarget); try { await api.post("/exchange-rates", { rate: form.get("rate"), provider: "manual", observedAt: new Date().toISOString() }, session.csrfToken); event.currentTarget.reset(); rates.reload(); } catch (reason) { setError(reason.message); } }
  if (settings.loading || companyRemote.loading || rates.loading || !company) return <Loading />;
  return <><PageHeader title="会社・運用設定" description="会社情報、振込先、為替、売上目標、仕入予算の正本を管理します。" />{(settings.error || companyRemote.error || rates.error || error) && <Notice>{settings.error || companyRemote.error || rates.error || error}</Notice>}<form className="card business-form" onSubmit={saveCompany}><div className="card-header"><h3>会社情報</h3></div><div className="form-grid three">{[["companyName", "会社名"], ["representativeName", "代表者名"], ["invoiceNumber", "インボイス番号"], ["postalCode", "郵便番号"], ["address", "住所"], ["phone", "電話番号"], ["email", "メール"]].map(([key, label]) => <Field key={key} label={label}><input required={key === "companyName"} value={company[key] || ""} onChange={(event) => setCompany({ ...company, [key]: event.target.value })} /></Field>)}</div><FormActions submitLabel="会社情報を保存" /></form>
    <section className="card"><div className="card-header"><h3>運用設定</h3></div><div className="settings-list">{(settings.data?.items || []).map((item) => <SettingRow key={item.key} item={item} onSave={saveSetting} />)}</div></section>
    <section className="card"><div className="card-header"><h3>USD/JPYレート履歴</h3><form className="inline-rate" onSubmit={addRate}><label>新規レート<input name="rate" type="number" min="0.01" step="0.01" required /></label><button className="button primary" type="submit">追加</button></form></div><div className="history-list">{(rates.data?.items || []).map((rate) => <div key={rate.id}><strong>1 USD = ¥{rate.rate}</strong><span>{dateTime(rate.observedAt)} / {rate.provider}</span></div>)}</div></section></>;
}
function SettingRow({ item, onSave }) { const [value, setValue] = useState(item.value); return <div className="setting-row"><div><strong>{item.key}</strong><small>{item.valueType}</small></div><input value={value} onChange={(event) => setValue(event.target.value)} /><button className="button secondary small" onClick={() => onSave(item, value)}>保存</button></div>; }

export function AuditPage() {
  const remote = useRemote("/audit-logs?limit=500"); if (remote.loading) return <Loading />;
  return <><PageHeader title="監査ログ" description="主要な認証・設定・業務操作を変更不能な履歴として記録します。" />{remote.error && <Notice>{remote.error}</Notice>}<TableCard title="操作履歴" rows={remote.data?.items || []} columns={[{ key: "createdAt", label: "日時", render: (row) => dateTime(row.createdAt) }, { key: "actorName", label: "操作者" }, { key: "action", label: "操作" }, { key: "targetType", label: "対象" }, { key: "targetId", label: "対象ID" }, { key: "result", label: "結果" }, { key: "requestId", label: "リクエストID" }]} /></>;
}

export function StocktakePage() {
  const remote = useRemote("/products?page=1&pageSize=100&sort=code_asc&includeCancelled=true"); if (remote.loading) return <Loading />;
  return <><PageHeader title="棚卸" description="バーコード・商品コード単位で在庫状態を確認し、棚卸CSVを保存履歴付きで出力します。" actions={<CsvLink kind="stocktake" />} />{remote.error && <Notice>{remote.error}</Notice>}<TableCard title="棚卸対象" rows={remote.data?.items || []} columns={[{ key: "productCode", label: "商品コード" }, { key: "brand", label: "ブランド" }, { key: "modelNumber", label: "モデル" }, { key: "serialNumber", label: "シリアル" }, { key: "inventoryStatus", label: "在庫状態", render: (row) => <InventoryStatus value={row.inventoryStatus} /> }, { key: "checked", label: "確認", render: (row) => <label className="stock-check"><input type="checkbox" />確認済み</label> }]} /></>;
}

export function GuestPortal({ session, onLogout }) {
  const catalog = useRemote("/guest/catalog"); const requests = useRemote("/guest/purchase-requests"); const [message, setMessage] = useState({}); const [error, setError] = useState("");
  async function requestProduct(productId) { setError(""); try { await api.post("/guest/purchase-requests", { productId, message: message[productId] || "" }, session.csrfToken); requests.reload(); catalog.reload(); } catch (reason) { setError(reason.message); } }
  return <main className="guest-react"><header className="guest-header"><div><span className="brand-mark">庫</span><div><strong>公開商品一覧</strong><small>{session.user.displayName} 様</small></div></div><button className="button secondary" onClick={onLogout}>ログアウト</button></header><section className="guest-content"><PageHeader title="ご案内商品" description="現在在庫中の商品だけを表示しています。出荷・売上・他社取置時は自動で非表示になります。" />{error && <Notice>{error}</Notice>}{catalog.loading ? <Loading /> : <div className="guest-grid">{(catalog.data?.items || []).map((item) => <article className="guest-product" key={item.productId}><div className="guest-product-image">{item.brand?.slice(0, 1) || "商"}</div><div><span className="badge">{item.boxCodes}</span><h3>{item.brand} {item.modelNumber}</h3><p>{item.referenceNumber || "リファレンス未設定"}</p><strong className="guest-price">{money(item.baseSalePriceMinor, item.baseSaleCurrency)}</strong><textarea aria-label={`${item.productCode}へのメッセージ`} rows="2" placeholder="ご質問・ご希望" value={message[item.productId] || ""} onChange={(event) => setMessage({ ...message, [item.productId]: event.target.value })} /><button className="button primary full" onClick={() => requestProduct(item.productId)}>購入リクエスト</button></div></article>)}</div>}</section></main>;
}

export function NotFoundPage() { return <section className="card empty-state"><h2>画面が見つかりません</h2><p>左側のメニューから操作を選択してください。</p><Link className="button primary" to="/">ダッシュボードへ</Link></section>; }
