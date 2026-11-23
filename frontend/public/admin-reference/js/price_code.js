// Printed price notation only. Stored amounts and accounting calculations are unchanged.
const PRICE_CODE_DIGITS = 'LTNSKYRUPO';
const PRICE_CODE_PADDING = 'TNSKYRUPOL';
const priceCodeCache = new Map();

function getStatementCostJpy(line = {}, saved = {}) {
  // Inventory cost is already JPY; purchase-line prices may still be USD or HKD.
  for (const value of [line.adjustedCostJpyMinor, line.allocatedCostJpyMinor,
    line.inventoryCostJpyMinor, saved.purchasePrice, line.fixedPurchaseCostJpyMinor]) {
    if (value != null && value !== '' && Number.isFinite(Number(value))) return Number(value);
  }
  const converted = line.convertedPurchasePriceJpy ?? line.convertedTotalJpy;
  if (converted != null && converted !== '' && Number.isFinite(Number(converted))) {
    return Number(converted) / (Number(line.quantity) > 0 ? Number(line.quantity) : 1);
  }
  if (line.purchasePrice == null || line.purchasePrice === '') return null;
  if (!line.purchaseCurrency || line.purchaseCurrency === 'JPY') return Number(line.purchasePrice);
  const scale = Number(line.purchaseFxScale ?? line.fxScale);
  const rate = Number(line.purchaseFxRateScaled ?? line.fxRateScaled);
  return scale > 0 && rate > 0 ? Math.round(Number(line.purchasePrice) * rate / scale) : null;
}

function priceCodeBody(amount, currency) {
  if (amount === '' || amount == null) return '';
  const value = Number(amount);
  const unit = currency === 'JPY' ? 1000 : currency === 'USD' ? 100 : 0;
  if (!unit || !Number.isFinite(value) || value < 0) return '';
  const small = value > 0 && value < unit;
  const digits = String(small ? Math.max(1, Math.floor(value / (unit / 10))) : Math.floor(value / unit));
  if (!/^\d+$/.test(digits)) return '';
  let previous = '', repeated = false;
  const encoded = Array.from(digits, digit => {
    repeated = digit === previous && !repeated;
    previous = digit;
    return repeated ? 'E' : PRICE_CODE_DIGITS[Number(digit)];
  }).join('');
  return (small ? 'L' : '') + encoded;
}

function formatPriceCode(amount, currency, identity = '') {
  const body = priceCodeBody(amount, currency);
  if (!body) return '';
  // Keep padding stable between preview, SVG download and printing in this session.
  const key = JSON.stringify([identity, currency, Number(amount)]);
  if (!priceCodeCache.has(key)) {
    const bytes = new Uint32Array(6);
    crypto.getRandomValues(bytes);
    const padding = Array.from(bytes, value => PRICE_CODE_PADDING[value % PRICE_CODE_PADDING.length]);
    priceCodeCache.set(key, padding.slice(0, 3).join('') + body + padding.slice(3).join(''));
  }
  return priceCodeCache.get(key);
}
