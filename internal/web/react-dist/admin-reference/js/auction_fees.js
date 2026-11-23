// Auction fees are applied once to the entered, tax-exclusive JPY amount.
// Persisted slips contain their own snapshots and never consult today's master.
const AUCTION_MAX_AMOUNT = Number.MAX_SAFE_INTEGER;
function auctionEscape(value) { return String(value ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c])); }
function auctionInteger(value, label) {
  if (!/^\d+$/.test(String(value))) throw Error(`${label}は0以上の整数で入力してください。`);
  const number = Number(value);
  if (!Number.isSafeInteger(number)) throw Error(`${label}が大きすぎます。`);
  return number;
}
function auctionRate(value) {
  if (!/^\d+(?:\.\d{1,2})?$/.test(String(value)) || Number(value) > 100) throw Error('料率は0〜100%（小数点以下2桁まで）で入力してください。');
  return Math.round(Number(value) * 100);
}
function auctionNormalizeRule(rule = {}) {
  const mode = rule.mode || 'none';
  const rate = rule.rateBasisPoints || 0, fixed = rule.fixedAmount || 0, tiers = rule.tiers || [];
  if (!Number.isSafeInteger(rate) || rate < 0 || rate > 10000 || !Number.isSafeInteger(fixed) || fixed < 0) throw Error('手数料の設定が不正です。');
  if (mode === 'none' && !rate && !fixed && !tiers.length) return {mode};
  if (mode === 'rate' && !fixed && !tiers.length) return {mode,rateBasisPoints:rate};
  if (mode === 'fixed' && !rate && !tiers.length) return {mode,fixedAmount:fixed};
  if (mode === 'tiers' && !rate && !fixed && tiers.length > 0 && tiers.length <= 50) {
    let next = 0;
    for (const [index,tier] of tiers.entries()) {
      const tierMode = tier.mode || 'rate', tierRate = tier.rateBasisPoints ?? 0, tierFixed = tier.fixedAmount ?? 0;
      if (!['rate','fixed'].includes(tierMode) || !Number.isSafeInteger(tierFixed) || tierFixed < 0 ||
          !Number.isSafeInteger(tierRate) || tierRate < 0 || tierRate > 10000 ||
          (tierMode === 'fixed' ? tierRate !== 0 : tierFixed !== 0)) throw Error('価格帯の手数料の設定が不正です。');
      if (!Number.isSafeInteger(tier.from) || tier.from !== next) throw Error('価格帯は0円から重複や空白なく設定してください。');
      if (tier.to == null) { if (index !== tiers.length-1) throw Error('上限なしは最後の価格帯に設定してください。'); }
      else {
        if (!Number.isSafeInteger(tier.to) || tier.to <= tier.from || index === tiers.length-1) throw Error('上限は下限より大きくし、最後の価格帯は上限なしにしてください。');
        next = tier.to;
      }
    }
    return {mode,tiers:tiers.map(t => t.mode === 'fixed'
      ? {from:t.from,to:t.to ?? null,rateBasisPoints:0,mode:'fixed',fixedAmount:t.fixedAmount ?? 0}
      : {from:t.from,to:t.to ?? null,rateBasisPoints:t.rateBasisPoints ?? 0})};
  }
  throw Error('手数料の設定を確認してください。');
}
function auctionCalculate(base, rule, direction, currency = 'JPY') {
  if (!rule || rule.mode === 'none') return {amount:base,fee:0,snapshot:{}};
  if (currency !== 'JPY') throw Error('オークション手数料は円取引のみ対応しています。');
  if (!Number.isSafeInteger(base) || base < 0 || !['purchase','sales'].includes(direction)) throw Error('商品金額を確認してください。');
  const normalized = auctionNormalizeRule(rule);
  const applicable = normalized.mode === 'tiers'
    ? normalized.tiers.find(t => base >= t.from && (t.to == null || base < t.to)) : normalized;
  const fee = applicable.mode === 'fixed' ? applicable.fixedAmount : Number(BigInt(base) * BigInt(applicable.rateBasisPoints || 0) / 10000n);
  const amount = base + (direction === 'purchase' ? fee : -fee);
  if (amount < 0) throw Error('手数料が販売金額を超えています。');
  if (!Number.isSafeInteger(amount)) throw Error('手数料適用後の金額が大きすぎます。');
  return {amount,fee,snapshot:{rule:normalized,baseAmountMinor:base,feeMinor:fee,resultAmountMinor:amount}};
}
function auctionRuleLabel(rule) {
  if (!rule || !rule.mode || rule.mode === 'none') return '設定なし';
  if (rule.mode === 'rate') return `一律 ${rule.rateBasisPoints / 100}%`;
  if (rule.mode === 'fixed') return `1点あたり ¥${Number(rule.fixedAmount).toLocaleString('ja-JP')}`;
  return (rule.tiers || []).map(t => `¥${t.from.toLocaleString('ja-JP')}以上${t.to == null ? '' : `〜¥${t.to.toLocaleString('ja-JP')}未満`}：${t.mode === 'fixed' ? `¥${Number(t.fixedAmount || 0).toLocaleString('ja-JP')}` : `${t.rateBasisPoints / 100}%`}`).join(' / ');
}

// The editor uses one price-band layout for both uniform and individual rules.
function auctionRuleDraft(rule = {}) {
  const scope = rule.mode === 'tiers' ? 'individual' : 'uniform';
  const rows = scope === 'individual' ? (rule.tiers || []) : [{from:0,to:null,...rule}];
  return {enabled:Boolean(rule.mode && rule.mode !== 'none'),scope,rows:rows.map(t => ({
    from:String(t.from ?? 0),to:t.to == null ? '' : String(t.to),
    unit:t.mode === 'fixed' ? 'fixed' : 'rate',
    value:String(t.mode === 'fixed' ? t.fixedAmount || 0 : (t.rateBasisPoints || 0) / 100),
  }))};
}
let _auctionMasterDraft = {isAuction:false,purchase:auctionRuleDraft(),sales:auctionRuleDraft()};
function auctionOpenMaster(company) {
  const settings = company?.auctionFees || {};
  _auctionMasterDraft = {isAuction:Boolean(settings.isAuction),
    purchase:auctionRuleDraft(settings.purchase),sales:auctionRuleDraft(settings.sales)};
  auctionRenderMaster();
}
function auctionRenderMaster() {
  const host = document.getElementById('client-auction-fees');
  if (!host) return;
  host.innerHTML = `<label class="auction-company-check"><input type="checkbox" id="client-auction-enabled" ${_auctionMasterDraft.isAuction ? 'checked' : ''} onchange="auctionMasterEnabled(this.checked)"> オークション会社</label>
    <div id="auction-master-settings" ${_auctionMasterDraft.isAuction ? '' : 'hidden'}>
      <p class="auction-caption">税抜の商品金額を基準に、1点ずつ計算します。円取引のみ・円未満切り捨て。</p>
      <div class="auction-rule-grid">${['purchase','sales'].map(direction => {
        const draft = _auctionMasterDraft[direction];
        const name = direction === 'purchase' ? '仕入' : '売上';
        return `<fieldset class="auction-rule-editor"><legend>${name}手数料（${direction === 'purchase' ? '原価へ加算' : '販売金額から減算'}）</legend>
          <div class="auction-rule-heading"><strong>価格帯別の料率</strong>
            <label><input type="checkbox" id="auction-${direction}-enabled" ${draft.enabled ? 'checked' : ''} onchange="auctionRuleEnabled('${direction}',this.checked)"> 手数料を設定</label></div>
          <label class="auction-scope-label" for="auction-${direction}-scope">適用方法
            <select class="form-control" id="auction-${direction}-scope" aria-label="${name}手数料の適用方法" ${draft.enabled ? '' : 'disabled'} onchange="auctionMasterScope('${direction}',this.value)">
              <option value="uniform" ${draft.scope === 'uniform' ? 'selected' : ''}>一律</option>
              <option value="individual" ${draft.scope === 'individual' ? 'selected' : ''}>個別</option>
            </select></label>
          ${auctionRuleInputs(direction,draft)}
        </fieldset>`;
      }).join('')}</div>
    </div>`;
}
function auctionRuleInputs(direction,draft) {
  const name = direction === 'purchase' ? '仕入' : '売上';
  const individual = draft.scope === 'individual', disabled = draft.enabled ? '' : 'disabled';
  return `<p class="auction-caption">${individual ? '個別：下限以上・上限未満の商品に適用。最初は0円、最後の上限は空欄（上限なし）にします。' : '一律：価格帯にかかわらず、すべての商品に同じ手数料を適用します。'}</p>
    <div id="auction-${direction}-tiers">${draft.rows.map((tier,index) => `<div class="auction-tier-row">
      <label>下限（円・以上）<input class="form-control" type="number" min="0" step="1" data-tier="from" ${individual ? disabled : 'disabled'} value="${auctionEscape(individual ? tier.from : '0')}"></label>
      <label>上限（円・未満）<input class="form-control" type="number" min="0" step="1" data-tier="to" ${individual ? disabled : 'disabled'} placeholder="上限なし" value="${auctionEscape(individual ? tier.to : '')}"></label>
      <label>${tier.unit === 'fixed' ? '手数料（円／点）' : '料率（%）'}<input class="form-control" type="number" min="0" ${tier.unit === 'fixed' ? 'step="1"' : 'max="100" step="0.01"'} data-tier="value" ${disabled} value="${auctionEscape(tier.value)}"></label>
      <label>単位<select class="form-control" data-tier="unit" aria-label="${name}の価格帯${index+1}の単位" ${disabled} onchange="auctionTierUnit('${direction}',${index},this.value)">
        <option value="rate" ${tier.unit === 'rate' ? 'selected' : ''}>％</option><option value="fixed" ${tier.unit === 'fixed' ? 'selected' : ''}>円</option>
      </select></label>
      <button type="button" class="btn btn-outline btn-sm" ${individual ? disabled : 'disabled'} aria-label="${name}の価格帯${index+1}を削除" onclick="auctionTierRemove('${direction}',${index})">削除</button>
    </div>`).join('')}</div>
    <button type="button" class="btn btn-outline btn-sm" ${individual ? disabled : 'disabled'} onclick="auctionTierAdd('${direction}')">＋ 価格帯を追加</button>`;
}
function auctionCaptureDraft(direction) {
  const draft = _auctionMasterDraft[direction];
  return {...draft,enabled:Boolean(document.getElementById(`auction-${direction}-enabled`)?.checked),
    rows:[...document.querySelectorAll(`#auction-${direction}-tiers .auction-tier-row`)].map(row => ({
      from:row.querySelector('[data-tier="from"]').value,to:row.querySelector('[data-tier="to"]').value,
      unit:row.querySelector('[data-tier="unit"]').value,value:row.querySelector('[data-tier="value"]').value,
    }))};
}
function auctionCaptureRule(direction) {
  const draft = auctionCaptureDraft(direction);
  if (!draft.enabled) return {mode:'none'};
  const fee = row => {
    if (row.unit === 'rate') return {rateBasisPoints:auctionRate(row.value)};
    if (row.unit === 'fixed') return {mode:'fixed',rateBasisPoints:0,fixedAmount:auctionInteger(row.value,'手数料')};
    throw Error('手数料の単位を選択してください。');
  };
  if (draft.scope === 'uniform') {
    const row = draft.rows[0], value = fee(row);
    return auctionNormalizeRule(row.unit === 'fixed' ? {mode:'fixed',fixedAmount:value.fixedAmount} : {mode:'rate',rateBasisPoints:value.rateBasisPoints});
  }
  return auctionNormalizeRule({mode:'tiers',tiers:draft.rows.map(row => ({
    from:auctionInteger(row.from,'下限'),to:row.to === '' ? null : auctionInteger(row.to,'上限'),...fee(row),
  }))});
}
function auctionReadMaster() {
  const isAuction = Boolean(document.getElementById('client-auction-enabled')?.checked);
  return isAuction ? {isAuction,purchase:auctionCaptureRule('purchase'),sales:auctionCaptureRule('sales')} : {isAuction:false,purchase:{mode:'none'},sales:{mode:'none'}};
}
function auctionMasterEnabled(enabled) {
  _auctionMasterDraft.isAuction = enabled;
  document.getElementById('auction-master-settings').hidden = !enabled;
}
function auctionUpdateMaster(direction, change) {
  _auctionMasterDraft.purchase = auctionCaptureDraft('purchase');
  _auctionMasterDraft.sales = auctionCaptureDraft('sales');
  try { change(_auctionMasterDraft[direction]);auctionRenderMaster(); }
  catch(error) { showToast('error','手数料設定',error.message); }
}
function auctionRuleEnabled(direction, enabled) {
  auctionUpdateMaster(direction,draft => {draft.enabled=enabled;});
}
function auctionMasterScope(direction,scope) {
  auctionUpdateMaster(direction,draft => {
    if (!['uniform','individual'].includes(scope) || draft.scope === scope) return;
    // Keep unsaved rules while switching scopes; never discard a set of price bands.
    draft[draft.scope+'Rows'] = draft.rows;
    draft.rows = draft[scope+'Rows'] || [{...draft.rows[0],from:'0',to:''}];
    draft.scope = scope;
  });
}
function auctionTierUnit(direction,index,unit) {
  auctionUpdateMaster(direction,draft => {if (['rate','fixed'].includes(unit)) draft.rows[index].unit=unit;});
}
function auctionTierAdd(direction) {
  auctionUpdateMaster(direction,draft => {
    if (draft.scope !== 'individual') return;
    if (draft.rows.length>=50) throw Error('価格帯は50件まで設定できます。');
    const last=draft.rows[draft.rows.length-1];
    if (last.to === '') {
      const lower=auctionInteger(last.from,'下限');
      if (lower >= AUCTION_MAX_AMOUNT) throw Error('価格帯の上限が大きすぎます。');
      last.to=String(Math.min(AUCTION_MAX_AMOUNT,lower+100000));
    }
    draft.rows.push({from:last.to,to:'',unit:'rate',value:'0'});
  });
}
function auctionTierRemove(direction,index) {
  auctionUpdateMaster(direction,draft => {
    if (draft.scope !== 'individual') return;
    draft.rows.splice(index,1);
    if (!draft.rows.length) {draft.enabled=false;draft.rows=[{from:'0',to:'',unit:'rate',value:'0'}];}
  });
}


const _auctionTransactions = {purchase:{key:'',enabled:false,rule:null},sales:{key:'',enabled:false,rule:null}};
function auctionResetTransaction(direction) { Object.assign(_auctionTransactions[direction],{key:'',enabled:false,rule:null}); }
function auctionTransactionCompany(direction,code) {
  const role = (direction === 'purchase' ? APP_DATA.suppliers : APP_DATA.buyers)?.find(item => item.code === code);
  return (APP_DATA.clientCompanies || []).find(company => company[direction === 'purchase' ? 'supplierCode' : 'buyerCode'] === code && code)
    || role;
}
function auctionSyncTransaction(direction) {
  const purchase=direction==='purchase';
  const code=document.getElementById(purchase?'pe-supplier':'sl-buyer')?.value || '';
  const currency=purchase ? (_peSlipData?.purchaseCurrency || 'JPY') : _salesEntryCurrency;
  const company=auctionTransactionCompany(direction,code), settings=company?.auctionFees;
  const personal=purchase && _peSlipData?.purchaseTaxMode==='personal';
  const rule=settings?.isAuction && !personal ? settings[direction] : null;
  const configured=Boolean(rule?.mode && rule.mode!=='none');
  const key=JSON.stringify([code,currency,personal,rule]);
  const state=_auctionTransactions[direction];
  if(state.key!==key){state.key=key;state.enabled=false;}
  state.rule=configured && currency==='JPY' && state.enabled ? JSON.parse(JSON.stringify(rule)) : null;
  if(purchase && _peSlipData) _peSlipData.auctionFeeRule=state.rule;
  const panel=document.getElementById(purchase?'pe-auction-fee':'sl-auction-fee');
  if(panel){
    panel.hidden=!configured;
    const signature=key+String(state.enabled);
    if(panel.dataset.signature!==signature){
      panel.dataset.signature=signature;
      panel.innerHTML=configured ? `<div><strong>オークション手数料</strong><span class="auction-rule-summary">${auctionEscape(auctionRuleLabel(rule))}</span><small>${currency!=='JPY'?'円取引のみ対応しています。':`税抜金額に適用・円未満切り捨て。商品ごとに${purchase?'原価へ加算':'販売金額から減算'}します。`}</small></div>
      <button type="button" role="switch" aria-label="${purchase?'仕入':'売上'}手数料を適用" aria-checked="${Boolean(state.rule)}" class="auction-toggle ${state.rule?'on':''}" ${currency!=='JPY'?'disabled':''} onclick="auctionToggle('${direction}')">${state.rule?'ON':'OFF'}</button>` : '';
    }
  }
  return state.rule;
}
function auctionToggle(direction) {
  _auctionTransactions[direction].enabled=!_auctionTransactions[direction].enabled;
  if(direction==='purchase')_peUpdateDetailUI();else calcSalesTotal();
}
function auctionPurchaseResult(line,slip) { return auctionCalculate(Number(line.purchasePrice)||0,slip?.auctionFeeRule,'purchase',slip?.purchaseCurrency||'JPY'); }
function auctionPurchaseAmount(line,slip) { try{return auctionPurchaseResult(line,slip).amount;}catch{return Number(line.purchasePrice)||0;} }
function auctionSalesResult(input) { return auctionCalculate(parseSalesPrice(input?.value),_auctionTransactions.sales.rule,'sales',_salesEntryCurrency); }
function auctionSalesAmount(input) { try{return auctionSalesResult(input).amount;}catch{return parseSalesPrice(input?.value);} }
function auctionMoney(value) { return '¥'+Number(value).toLocaleString('ja-JP'); }
function auctionShowLine(host,result,direction,enabled) {
  if(!host)return;
  host.hidden=!enabled;
  host.classList.toggle('auction-error',Boolean(result.error));
  host.textContent=result.error || `手数料 ${direction==='purchase'?'+':'−'}${auctionMoney(result.fee)} → ${auctionMoney(result.amount)}`;
}
function auctionPurchasePreviews() {
  (_peSlipData?.lines || []).forEach(line => {
    let result;try{result=auctionPurchaseResult(line,_peSlipData);}catch(error){result={error:error.message};}
    auctionShowLine(document.getElementById(`pe-fee-${line.lineId}`),result,'purchase',Boolean(_peSlipData.auctionFeeRule));
  });
}
function auctionSalesPreviews() {
  document.querySelectorAll('#salesLines .sl-input-price').forEach(input => {
    let host=input.closest('td')?.querySelector('.auction-line-preview');
    if(!host && input.closest('td')){host=document.createElement('div');host.className='auction-line-preview';input.closest('td').appendChild(host);}
    let result;try{result=auctionSalesResult(input);}catch(error){result={error:error.message};}
    auctionShowLine(host,result,'sales',Boolean(_auctionTransactions.sales.rule));
  });
}
function auctionValidateRegistration(direction) {
  auctionSyncTransaction(direction);
  try {
    if(direction==='purchase')(_peSlipData?.lines || []).forEach(line=>auctionPurchaseResult(line,_peSlipData));
    else document.querySelectorAll('#salesLines tr[data-line-id]').forEach(row=>{
      const id=row.dataset.lineId;if(document.getElementById(`sl-chk-${id}`)?.checked!==false)auctionSalesResult(document.getElementById(`sl-price-${id}`));
    });
    return true;
  } catch(error) { showToast('error','手数料を計算できません',error.message);return false; }
}
function auctionPurchaseSavedCopy(slip) {
  const copy=JSON.parse(JSON.stringify(slip));
  copy.lines.forEach(line=>{const result=auctionPurchaseResult(line,copy);line.purchasePrice=result.amount;line.auctionFee=result.snapshot;});
  delete copy.auctionFeeRule;
  return copy;
}
