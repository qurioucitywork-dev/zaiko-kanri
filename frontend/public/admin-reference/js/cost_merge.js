// Product-to-product cost merging. Codes are reserved by the server at start.
(function () {
  const original = {
    canStart: _costAdjustmentCanStart, add: costAdjustmentAddPart,
    start: costAdjustmentStart, finalize: costAdjustmentFinalize,
    render: costAdjustmentRenderBreakdownWorkspace, allocation: costAdjustmentRenderAllocationSummary,
    edit: costAdjustmentCompleteItemEdit, mode: costAdjustmentSetMode,
  };
  let merge = null;
  let starting = false;
  const isMerge = () => _costAdjustmentState.mode === 'combine';
  const sources = () => _costAdjustmentState.partSlots.filter(slot => slot?.type === 'product').map(slot => slot.product);
  const money = value => `¥${Number(value || 0).toLocaleString('ja-JP')}`;
  window.costMergeAfterReset = function () {
    merge = null;
    starting = false;
    _costAdjustmentState.combineActive = false;
    _costAdjustmentState.breakdownDraft = null;
  };

  _costAdjustmentCanStart = function () {
    return isMerge() ? !starting && !merge && sources().length >= 2 : original.canStart();
  };
  costAdjustmentSetMode = function (mode) {
    const previous = _costAdjustmentState.mode;
    const result = original.mode(mode);
    if (result !== previous && !_costAdjustmentState.combineActive) {
      _costAdjustmentState.partSlots = Array(20).fill(null);
      merge = null;
      if (isMerge()) _costAdjustmentState.product = null;
      costAdjustmentRenderPartGrid();
    }
    if (isMerge()) render();
    else {
      const area = document.getElementById('ca-product-summary');
      area?.removeAttribute('role'); area?.removeAttribute('tabindex');
      if (area) { area.onclick = null; area.onkeydown = null; }
      document.getElementById('ca-product-code')?.closest('.ca-code-entry')?.removeAttribute('hidden');
    }
    return result;
  };
  costAdjustmentAddPart = function (value) {
    if (!isMerge()) return original.add(value);
    if (merge || starting) return false;
    const input = document.getElementById('ca-part-code');
    const code = String(value ?? input?.value ?? '').trim().toUpperCase();
    const product = (APP_DATA.inventory || []).find(item => String(item.code).toUpperCase() === code);
    if (!product || normalizeInventoryStatusLabel(product.status) !== '在庫中' || product.inventoryValuationExcluded) {
      showToast('error', '商品を追加できません', '原価管理に使用できるのは在庫中の商品だけです'); return false;
    }
    if (sources().some(item => item.code === product.code)) {
      showToast('info', '登録済みの商品です', product.code); return false;
    }
    const index = _costAdjustmentState.partSlots.findIndex(slot => !slot);
    if (index < 0) return false;
    _costAdjustmentState.partSlots[index] = { type: 'product', label: product.code, product: { ...product } };
    if (input) input.value = '';
    costAdjustmentRenderPartGrid(); render();
    return true;
  };
  window.costMergeRemove = function (index) {
    if (merge || starting) return;
    _costAdjustmentState.partSlots[index] = null;
    costAdjustmentRenderPartGrid(); render();
  };
  costAdjustmentStart = async function () {
    if (!isMerge()) return original.start();
    if (!_costAdjustmentCanStart()) return false;
    starting = true; _costAdjustmentUpdateStartButton();
    try {
      const inputs = sources().map(item => ({ ...item }));
      if (!window.ZaikoAPI?.request || inputs.some(item => !item._id)) throw new Error('DB接続を確認してください');
      const result = await window.ZaikoAPI.request('/cost-merges', {
        method: 'POST', body: JSON.stringify({ productIds: inputs.map(item => item._id) }),
      });
      inputs.forEach(item => {
        item.status = '原価調整中';
        const source = (APP_DATA.inventory || []).find(product => product._id === item._id);
        if (source) source.status = '原価調整中';
      });
      const total = inputs.reduce((sum, item) => sum + Number(item.purchasePrice || 0), 0);
      const priorSlipNumbers = [...new Set(inputs.flatMap(item => String(item.internalComment || '').split('\n').map(line => {
        const match = line.trim().match(/^(?:原価調整伝票番号|調整伝票番号):\s*(.+)$/u);
        return match ? match[1].trim() : '';
      }).filter(Boolean)))];
      const output = { position: 1, kind: 'product', managementCode: result.managementCode,
        sku: '', allocatedCostJpyMinor: total, completed: false,
        product: { accessories: [], notes: '', internalComment: [
          ...priorSlipNumbers.map(number => `調整伝票番号: ${number}`),
          `原価合わせ元: ${inputs.map(item => item.code).join(', ')}`,
        ].join('\n') } };
      merge = { id: result.id, adjustmentDate: result.businessDate, inputs, total, items: [output], finalized: false };
      _costAdjustmentState.combineActive = true;
      _costAdjustmentState.breakdownDraft = merge; // Reuse the shared new-product editor.
      _costAdjustmentState.product = { code: result.managementCode, purchasePrice: total, status: '原価調整中' };
      costAdjustmentRenderBreakdownWorkspace();
      try { await window.ZaikoAPI.hydrateAdmin(); } catch (_) { showToast('info', '原価合わせを開始しました', '一覧更新は画面の再読み込みで確認できます'); }
      render();
      return true;
    } catch (error) { showToast('error', '原価合わせを開始できません', error.message); return false; }
    finally { starting = false; _costAdjustmentUpdateStartButton(); }
  };
  window.costMergeEdit = function () {
    if (!merge || merge.finalized) return false;
    const opened = costAdjustmentOpenItemEditor(0);
    const sku = document.getElementById('ca-item-sku');
    if (sku) sku.readOnly = false;
    const cost = document.getElementById('ca-item-cost');
    if (cost) {
      _costAdjustmentSetPrice('ca-item-cost', merge.total);
      cost.readOnly = true;
      cost.setAttribute('aria-readonly', 'true');
      cost.title = 'BOX商品の原価合計から自動算出されます';
    }
    return opened;
  };
  costAdjustmentCompleteItemEdit = function () {
    if (isMerge() && merge) _costAdjustmentSetPrice('ca-item-cost', merge.total);
    const ok = original.edit();
    if (ok && isMerge() && merge) {
      merge.items[0].sku = String(document.getElementById('ca-item-sku')?.value || '').trim();
      merge.items[0].allocatedCostJpyMinor = merge.total;
      render();
    }
    return ok;
  };
  window.costMergeAmount = function () {
    if (!merge || merge.finalized || _costAdjustmentState.finalizing) return;
    const amount = merge.total;
    merge.items[0].allocatedCostJpyMinor = amount;
    costAdjustmentRenderAllocationSummary();
    document.getElementById('ca-product-cost').textContent = money(amount);
    document.getElementById('ca-split-slip-total').textContent = money(amount);
    return amount;
  };
  costAdjustmentRenderAllocationSummary = function () {
    if (!isMerge()) return original.allocation();
    const item = merge?.items[0];
    const ready = Boolean(merge && !merge.finalized && item.completed && item.allocatedCostJpyMinor === merge.total);
    document.getElementById('ca-source-cost-label').textContent = 'BOX商品の原価合計';
    document.getElementById('ca-allocated-cost-label').textContent = '新商品の原価';
    document.getElementById('ca-source-cost-total').textContent = money(merge?.total || sources().reduce((s,p)=>s+Number(p.purchasePrice||0),0));
    document.getElementById('ca-allocated-cost-total').textContent = money(item?.allocatedCostJpyMinor);
    document.getElementById('ca-allocation-balance').textContent = merge?.finalized ? '確定済み' : !item?.completed ? '新商品情報を登録してください' : ready ? '原価が完全一致しました' : 'BOX商品の原価合計と一致させてください';
    const button = document.getElementById('ca-finalize-button');
    button.disabled = !ready || _costAdjustmentState.finalizing;
    button.setAttribute('aria-disabled', String(button.disabled));
    return { ready };
  };
  costAdjustmentRenderBreakdownWorkspace = function () {
    original.render();
    if (isMerge()) render();
  };
  function render() {
    const inputs = merge?.inputs || sources();
    if (merge?.items?.[0]) merge.items[0].allocatedCostJpyMinor = merge.total;
    const summary = document.getElementById('ca-product-summary');
    document.getElementById('ca-parts-subtitle').textContent = '在庫中の商品を2点以上登録してください';
    const productInput = document.getElementById('ca-product-code');
    if (productInput) productInput.closest('.ca-code-entry').hidden = true;
    if (summary) {
      summary.draggable = false;
      summary.classList.remove('ca-combine-drop-target');
      summary.setAttribute('role', 'button'); summary.tabIndex = 0;
      summary.onclick = window.costMergeEdit;
      summary.onkeydown = event => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); window.costMergeEdit(); } };
    }
    if (merge) {
      const item = merge.items[0], p = item.product;
      costAdjustmentRenderProduct({ code: item.managementCode, sku: item.sku,
        brand: (APP_DATA.brandRecords || []).find(b=>b.code===p.brandCode)?.name || 'クリックして新商品を登録',
        ref: p.referenceNumber, model: p.modelName, purchasePrice: item.allocatedCostJpyMinor,
        material: p.materialCode, beltMaterial: p.beltMaterial, dial: p.dial, accessories:p.accessories, note:p.notes, status:'原価調整中' });
      document.getElementById('ca-product-feedback').textContent = `新商品 ${item.managementCode}（開始日 ${merge.adjustmentDate}）・商品情報をクリックして編集`;
    } else {
      document.getElementById('ca-product-details').hidden = true;
      document.getElementById('ca-product-placeholder').hidden = false;
      document.querySelector('#ca-product-placeholder span').textContent = 'BOXに2点以上登録して開始すると、新商品を登録できます';
    }
    const grid = document.getElementById('ca-part-grid');
    grid.innerHTML = _costAdjustmentState.partSlots.map((slot,index)=>`<div class="ca-part-slot" role="gridcell" data-slot-label="${index+1}">${slot?.type==='product' ? `<strong>${_escHtml(slot.label)}</strong><small>${_escHtml(slot.product.brand||'')} ${_escHtml(slot.product.model||'')}</small>${!merge ? `<button type="button" class="btn btn-outline" onclick="costMergeRemove(${index})" aria-label="${_escHtml(slot.label)}をBOXから外す">×</button>` : ''}` : ''}</div>`).join('');
    const section = document.getElementById('ca-split-slip'); section.hidden = inputs.length === 0;
    document.getElementById('ca-split-slip-title').textContent = merge?.finalized ? '原価調整済み伝票' : '原価調整伝票（原価合わせ）';
    section.querySelector('.ca-split-slip-heading small').textContent = 'BOX商品の元原価は履歴として保持し、現在在庫集計から除外します';
    section.querySelector('.ca-split-slip-badge').textContent = merge?.slipNumber || '明細';
    const item = merge?.items[0];
    document.getElementById('ca-split-slip-lines').innerHTML = inputs.map((p,i)=>`<tr><td>${i+1}</td><td>${_escHtml(p.code)}</td><td>${_escHtml(p.sku||'—')}</td>${_costAdjustmentProductInfoCells(p)}<td>原価合わせ元商品</td><td>${money(p.purchasePrice)}<small>（履歴原価・現在在庫集計対象外）</small></td></tr>`).join('') + (item ? `<tr><td>新</td><td>${_escHtml(item.managementCode)}</td><td>${_escHtml(item.sku||'—')}</td>${_costAdjustmentProductInfoCells(item.product || {})}<td>原価合わせした商品</td><td><strong class="ca-merge-auto-cost">${money(item.allocatedCostJpyMinor)}</strong><small>（BOX商品の原価合計から自動算出）</small></td></tr>` : '');
    document.getElementById('ca-split-slip-total').textContent = money(item?.allocatedCostJpyMinor);
    document.getElementById('ca-split-slip-status').textContent = merge?.finalized ? '確定済み' : '確定前の明細です。新商品の情報を入力してください。原価はBOX商品の合計から自動算出されます';
    _costAdjustmentUpdateStartButton(); costAdjustmentRenderAllocationSummary();
  }
  costAdjustmentFinalize = async function () {
    if (!isMerge()) return original.finalize();
    if (!merge || !costAdjustmentRenderAllocationSummary().ready || _costAdjustmentState.finalizing) return false;
    _costAdjustmentState.finalizing = true; costAdjustmentRenderAllocationSummary();
    try {
      const item = merge.items[0], p = item.product;
      const result = await window.ZaikoAPI.request(`/cost-merges/${encodeURIComponent(merge.id)}/confirm`, {
        method:'POST', body:JSON.stringify({ sku:item.sku, output:{ position:1,kind:'product',allocatedCostJpyMinor:item.allocatedCostJpyMinor,
          product:{ ...p,modelNumber:p.modelName,beltText:p.beltMaterial,dialText:p.dial,accessoryCodes:_costAdjustmentAccessoryCodes(p.accessories),
            modelName:undefined,beltMaterial:undefined,dial:undefined,accessories:undefined } } }),
      });
      merge.finalized = true; merge.slipNumber = result.slipNumber;
      item.product.internalComment = _costAdjustmentConfirmedComment(item.product.internalComment, result.slipNumber);
      render();
      try { await window.ZaikoAPI.hydrateAdmin(); } catch (_) { showToast('info','確定しました','一覧更新には画面を再読み込みしてください'); }
      showToast('success','原価合わせを確定しました',result.slipNumber);
      return true;
    } catch(error) { showToast('error','確定できません',error.message); return false; }
    finally { _costAdjustmentState.finalizing=false; costAdjustmentRenderAllocationSummary(); }
  };
})();
