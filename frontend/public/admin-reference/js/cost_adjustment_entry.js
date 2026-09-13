// The direct cost-adjustment entry has its own draft, separate from split/merge.
(function () {
  const draft = { direction: '増額', items: [], reason: '', amount: 0, lastSlipNumber: '' };
  const oldMode = costAdjustmentSetMode;
  const oldRender = costAdjustmentRenderBreakdownWorkspace;
  const active = () => _costAdjustmentState.mode === 'adjustment';
  const sign = () => draft.direction === '増額' ? '+' : '−';
  const yen = value => `¥${Math.max(0, Number(value || 0)).toLocaleString('ja-JP')}`;
  const signedAmount = () => `${sign()}${yen(draft.amount)}`;
  const adjustedCost = item => Math.max(0, Number(item.purchasePrice || 0) + (draft.direction === '増額' ? 1 : -1) * Number(draft.amount || 0));
  const lineNote = () => draft.reason.trim()
    ? `${draft.direction}｜理由：${draft.reason.trim()}｜金額：${signedAmount()}`
    : '理由を入力してください';

  function mount() {
    if (document.getElementById('ca-direct-adjustment')) return;
    const workbench = document.querySelector('#page-cost-adjustment .ca-workbench');
    if (!workbench) return;
    const panel = document.createElement('section');
    panel.id = 'ca-direct-adjustment';
    panel.hidden = true;
    panel.className = 'ca-panel';
    panel.innerHTML = `
      <div class="ca-panel-heading"><div><h3>原価調整</h3><small>増額・減額を選択して商品を明細に追加してください</small></div></div>
      <div class="ca-code-entry ca-direct-config">
        <div class="ca-direct-mode-field">
          <span class="ca-direct-field-label">調整区分 <b class="ca-required">必須</b></span>
          <div class="ca-mode-switch" role="radiogroup" aria-label="原価の増額・減額">
            <button type="button" class="ca-mode-button ca-direct-increase" data-ca-direction="増額" role="radio" onclick="costDirectDirection('増額')"><strong>＋</strong> 増額</button>
            <button type="button" class="ca-mode-button ca-direct-decrease" data-ca-direction="減額" role="radio" onclick="costDirectDirection('減額')"><strong>−</strong> 減額</button>
          </div>
        </div>
        <div class="ca-direct-input-grid">
          <label class="ca-direct-field" for="ca-direct-reason">
            <span class="ca-direct-field-label">理由 <b class="ca-required">必須</b></span>
            <textarea id="ca-direct-reason" class="form-control" rows="2" placeholder="原価を調整する理由を入力してください" required oninput="costDirectReason(this.value)"></textarea>
          </label>
          <label class="ca-direct-field" for="ca-direct-global-amount">
            <span class="ca-direct-field-label">調整金額（税抜き） <b class="ca-required">必須</b></span>
            <span class="ca-direct-amount-wrap"><strong id="ca-direct-amount-sign">＋</strong><span>¥</span><input id="ca-direct-global-amount" class="form-control" type="text" inputmode="numeric" pattern="[0-9,]*" autocomplete="off" placeholder="0" required oninput="costDirectGlobalAmount(this.value)"></span>
            <small>増額はプラス、減額はマイナスで計算します</small>
          </label>
        </div>
        <div class="ca-direct-search">
        <label for="ca-direct-code">管理番号検索</label>
        <div class="ca-code-input-row">
          <input id="ca-direct-code" class="form-control" placeholder="商品の管理番号を入力" oninput="costDirectCodeChanged()" onkeydown="if(event.key==='Enter'){event.preventDefault();costDirectStart();}">
          <button type="button" class="btn btn-primary ca-direct-start" id="ca-direct-start" disabled onclick="costDirectStart()"><i class="fa-solid fa-play" aria-hidden="true"></i> 原価調整開始</button>
        </div>
        <p id="ca-direct-feedback" class="ca-input-feedback" aria-live="polite">管理番号を入力すると原価調整を開始できます（1商品専用）</p>
        </div>
      </div>
      <section class="ca-split-slip ca-direct-slip" aria-labelledby="ca-direct-title">
        <div class="slip-detail-header-sub ca-direct-slip-header">
          <span class="sdhs-id" id="ca-direct-title"><i class="fa-solid fa-file-invoice"></i> 原価調整伝票</span>
          <span class="sdhs-divider"></span><span class="sdhs-date" id="ca-direct-slip-direction">増額</span>
          <span class="sdhs-divider"></span><span class="sdhs-party" id="ca-direct-slip-total">調整額 ¥0</span>
        </div>
        <div class="slip-detail-items">
          <div class="sdi-heading"><i class="fa-solid fa-list"></i> 明細一覧 <span id="ca-direct-count">0件</span></div>
        <div class="slip-detail-table-scroll"><table class="data-table purchase-slip-detail-table ca-direct-slip-table">
          <colgroup><col class="ca-direct-col-no"><col class="ca-direct-col-code"><col class="ca-direct-col-sku"><col class="ca-direct-col-brand"><col class="ca-direct-col-model"><col class="ca-direct-col-status"><col class="ca-direct-col-cost"><col class="ca-direct-col-formula"><col class="ca-direct-col-result"><col class="ca-direct-col-note"><col class="ca-direct-col-action"></colgroup>
          <thead><tr><th>No.</th><th>商品コード</th><th>SKU</th><th>ブランド</th><th>型番／モデル</th><th>ステータス</th><th>現在原価</th><th>今回調整</th><th>調整後原価</th><th>備考</th><th>操作</th></tr></thead>
          <tbody id="ca-direct-lines"></tbody>
          <tfoot id="ca-direct-totals"></tfoot>
        </table></div></div>
        <div class="ca-finalize-bar"><p id="ca-direct-register-status">仕入伝票は変更せず、新しい原価調整伝票を発行します</p>
          <button type="button" class="btn btn-danger" id="ca-direct-reset" onclick="costDirectReset()" hidden><i class="fa-solid fa-rotate-left"></i> 開始した作業をリセット</button>
          <button type="button" class="btn btn-primary" id="ca-direct-register" onclick="costDirectRegister()"><i class="fa-solid fa-file-circle-check"></i> 原価調整伝票を発行</button>
        </div>
      </section>`;
    workbench.after(panel);
  }
  function render() {
    mount();
    const panel = document.getElementById('ca-direct-adjustment');
    if (!panel) return;
    panel.hidden = !active();
    for (const selector of ['.ca-workbench', '#ca-start-button', '#ca-finalize-bar']) {
      document.querySelector(`#page-cost-adjustment ${selector}`)?.classList.toggle('ca-direct-hidden', active());
    }
    if (!active()) return;
    panel.querySelectorAll('[data-ca-direction]').forEach(button => {
      const selected = button.dataset.caDirection === draft.direction;
      button.classList.toggle('active', selected);
      button.setAttribute('aria-checked', String(selected));
    });
    const reasonInput = document.getElementById('ca-direct-reason');
    if (reasonInput && reasonInput.value !== draft.reason) reasonInput.value = draft.reason;
    const amountInput = document.getElementById('ca-direct-global-amount');
    const amountValue = draft.amount > 0 ? draft.amount.toLocaleString('ja-JP') : '';
    if (amountInput && amountInput.value !== amountValue) amountInput.value = amountValue;
    const codeInput = document.getElementById('ca-direct-code');
    const startButton = document.getElementById('ca-direct-start');
    if (codeInput) codeInput.disabled = draft.items.length > 0;
    if (startButton) startButton.disabled = draft.items.length > 0 || !String(codeInput?.value || '').trim();
    document.getElementById('ca-direct-amount-sign').textContent = draft.direction === '増額' ? '＋' : '−';
    const beforeTotal = draft.items.reduce((sum, item) => sum + Number(item.purchasePrice || 0), 0);
    const adjustmentTotal = draft.amount * draft.items.length;
    const afterTotal = draft.items.reduce((sum, item) => sum + adjustedCost(item), 0);
    document.getElementById('ca-direct-slip-direction').innerHTML = `<i class="fa-solid ${draft.direction === '増額' ? 'fa-arrow-trend-up' : 'fa-arrow-trend-down'}"></i> ${draft.direction}`;
    document.getElementById('ca-direct-slip-total').textContent = `調整額 ${sign()}${yen(adjustmentTotal)}`;
    document.getElementById('ca-direct-count').textContent = draft.lastSlipNumber || `${draft.items.length}件`;
    document.getElementById('ca-direct-lines').innerHTML = draft.items.length ? draft.items.map((item, index) => `
      <tr><td>${index + 1}</td><td><strong>${_escHtml(item.code)}</strong></td>
      <td>${_escHtml(item.sku || '—')}</td><td>${_escHtml(item.brand || '—')}</td>
      <td>${_escHtml([item.ref, item.model].filter(Boolean).join(' / ') || '—')}</td>
      <td>${getStatusBadge(normalizeInventoryStatusLabel(item.status) || '—')}</td>
      <td>${_escHtml(_costAdjustmentCost(item))}</td>
      <td class="ca-direct-calculation"><span>${yen(item.purchasePrice)}</span><b class="${draft.direction === '増額' ? 'is-plus' : 'is-minus'}">${sign()} ${yen(draft.amount)}</b></td>
      <td><strong class="ca-direct-result">${yen(adjustedCost(item))}</strong></td>
      <td class="ca-direct-line-notes">${_escHtml(lineNote())}</td>
      <td>—</td></tr>`).join('')
      : '<tr><td colspan="11" class="ca-direct-empty">理由と金額を入力し、管理番号から商品を追加してください</td></tr>';
    document.getElementById('ca-direct-totals').innerHTML = draft.items.length ? `
      <tr><th colspan="6">合計</th><td>${yen(beforeTotal)}</td><td class="${draft.direction === '増額' ? 'is-plus' : 'is-minus'}">${sign()}${yen(adjustmentTotal)}</td><td><strong>${yen(afterTotal)}</strong></td><td colspan="2">${_escHtml(draft.reason.trim() || '理由未入力')}</td></tr>` : '';
    const register = document.getElementById('ca-direct-register');
    if (register) register.disabled = !draft.items.length || !draft.reason.trim() || draft.amount <= 0 || (draft.direction === '減額' && draft.items.some(item => draft.amount > Number(item.purchasePrice || 0)));
    const reset = document.getElementById('ca-direct-reset');
    if (reset) reset.hidden = draft.items.length === 0;
  }
  window.costDirectDirection = function (value) {
    if (!['増額', '減額'].includes(value)) return;
    draft.direction = value; render();
  };
  window.costDirectReason = function (value) { draft.reason = String(value || ''); render(); };
  window.costDirectGlobalAmount = function (value) {
    const normalized = String(value || '').replace(/[０-９]/g, digit => String(digit.charCodeAt(0) - 0xFEE0));
    const digits = normalized.replace(/[^0-9]/g, '');
    const amount = Number(digits || 0);
    draft.amount = Number.isSafeInteger(amount) ? amount : 0;
    render();
  };
  window.costDirectCodeChanged = function () {
    const input = document.getElementById('ca-direct-code');
    const button = document.getElementById('ca-direct-start');
    if (button) button.disabled = draft.items.length > 0 || !String(input?.value || '').trim();
  };
  window.costDirectStart = async function () {
    if (!active()) return false;
    const input = document.getElementById('ca-direct-code');
    const code = String(input.value || '').trim().toUpperCase();
    const feedback = document.getElementById('ca-direct-feedback');
    if (draft.items.length > 0) {
      feedback.textContent = '原価調整で指定できる商品は1点だけです';
      feedback.className = 'ca-input-feedback error'; return false;
    }
    const item = (APP_DATA.inventory || []).find(product => String(product.code || '').trim().toUpperCase() === code);
    if (!code || !item) {
      feedback.textContent = code ? `「${code}」の商品が見つかりません` : '管理番号を入力してください';
      feedback.className = 'ca-input-feedback error'; return false;
    }
    if (normalizeInventoryStatusLabel(item.status) !== '在庫中' || item.inventoryValuationExcluded) {
      feedback.textContent = `${item.code} は原価管理の対象外です`;
      feedback.className = 'ca-input-feedback error'; return false;
    }
    const startButton = document.getElementById('ca-direct-start');
    if (startButton) startButton.disabled = true;
    try {
      if (!window.ZaikoAPI?.startProductCostAdjustment || !item._id) throw new Error('DB接続を確認してください');
      await window.ZaikoAPI.startProductCostAdjustment(item, { mode: 'adjustment' });
      const started = (APP_DATA.inventory || []).find(product => product._id === item._id) || { ...item };
      started.status = '原価調整中';
      if (Number(started.salePrice || item.salePrice) > 0 && !String(started.comment || '').includes('もともとの想定売価:')) {
        const line = `もともとの想定売価: ${formatSalePrice(Number(started.salePrice || item.salePrice))}`;
        started.comment = [started.comment, line].filter(Boolean).join('\n');
      }
      started.salePrice = 0;
      draft.items.push({ ...started });
      input.value = '';
      feedback.textContent = `${item.code} の原価調整を開始しました`;
      feedback.className = 'ca-input-feedback success';
      render(); input.focus(); return true;
    } catch (error) {
      feedback.textContent = error?.message || '原価調整を開始できませんでした';
      feedback.className = 'ca-input-feedback error';
      render(); return false;
    }
  };
  window.costDirectAdd = window.costDirectStart;
  // 開始後の商品を画面だけから外すとDBの「原価調整中」が残るため、
  // 取り消しは必ず下部の作業リセットからDBと画面を一緒に戻します。
  window.costDirectRemove = function () { return false; };
  window.costDirectReset = async function () {
    if (draft.items.length !== 1 || !window.confirm('原価調整の開始後に行った今回の作業だけをリセットします。よろしいですか？')) return false;
    const button = document.getElementById('ca-direct-reset');
    if (button) button.disabled = true;
    try {
      if (!window.ZaikoAPI?.resetCostAdjustmentStart) throw new Error('DB接続を確認してください');
      await window.ZaikoAPI.resetCostAdjustmentStart(draft.items);
      draft.items = [];
      draft.reason = '';
      draft.amount = 0;
      const feedback = document.getElementById('ca-direct-feedback');
      if (feedback) {
        feedback.textContent = '開始前の状態へ戻しました。正しい管理番号を入力してください';
        feedback.className = 'ca-input-feedback success';
      }
      if (typeof showToast === 'function') showToast('success', '原価調整作業をリセットしました', 'ステータス・売価・社内コメントを開始前へ戻しました');
      render();
      return true;
    } catch (error) {
      if (typeof showToast === 'function') showToast('error', '作業をリセットできませんでした', error?.message || '確定済みの作業は戻せません');
      render();
      return false;
    }
  };
  window.costDirectRegister = async function () {
    const button = document.getElementById('ca-direct-register');
    if (!button || button.disabled || !window.ZaikoAPI?.request) return false;
    button.disabled = true;
    try {
      const result = await window.ZaikoAPI.request('/cost-adjustments/direct', { method: 'POST', body: JSON.stringify({
        direction: draft.direction === '増額' ? 'increase' : 'decrease', notes: draft.reason.trim(),
        items: draft.items.map(item => ({ productId: item._id, adjustmentAmountJpyMinor: draft.amount }))
      }) });
      for (const line of result.items || []) {
        const product = (APP_DATA.inventory || []).find(item => item._id === line.productId);
        if (product) { product.purchasePrice = line.adjustedCostJpyMinor; product.status = '原価調整中'; }
      }
      draft.lastSlipNumber = result.slipNumber || result.id || '';
      draft.items = [];
      draft.reason = '';
      draft.amount = 0;
      if (typeof showToast === 'function') showToast('success', '原価調整伝票を発行しました', draft.lastSlipNumber);
      try { await window.ZaikoAPI.hydrateAdmin?.(); } catch (_) {}
      render();
      const status = document.getElementById('ca-direct-register-status');
      if (status) status.textContent = `${draft.lastSlipNumber} を発行しました（仕入伝票は変更されていません）`;
      return true;
    } catch (error) {
      if (typeof showToast === 'function') showToast('error', '原価調整伝票を発行できません', error?.message || '入力内容を確認してください');
      render(); return false;
    }
  };
  costAdjustmentSetMode = function (mode) { const result = oldMode(mode); render(); return result; };
  costAdjustmentRenderBreakdownWorkspace = function () { const result = oldRender(); render(); return result; };
})();
