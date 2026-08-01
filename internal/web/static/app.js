document.addEventListener("DOMContentLoaded", () => {
  const conversionStorageKey = "zaiko.showJPYConversion";
  const readConversionPreference = () => {
    try {
      return window.localStorage.getItem(conversionStorageKey) === "true";
    } catch (_error) {
      return false;
    }
  };
  const syncConversionToggles = (enabled) => {
    document.documentElement.classList.toggle("show-jpy-conversion", enabled);
    document.querySelectorAll("[data-jpy-conversion-toggle]").forEach((toggle) => {
      toggle.checked = enabled;
    });
  };
  let showJPYConversion = readConversionPreference();
  syncConversionToggles(showJPYConversion);
  document.addEventListener("change", (event) => {
    if (!event.target.matches("[data-jpy-conversion-toggle]")) return;
    showJPYConversion = event.target.checked;
    try {
      window.localStorage.setItem(conversionStorageKey, String(showJPYConversion));
    } catch (_error) {
      // 表示設定が保存できない環境でも、現在の画面では切替を維持する。
    }
    syncConversionToggles(showJPYConversion);
  });
  new MutationObserver(() => syncConversionToggles(showJPYConversion))
    .observe(document.body, {childList: true, subtree: true});

  const parseDisplayAmount = (value) => Number(String(value || "").replace(/[^\d.-]/g, "")) || 0;
  const formatJPY = new Intl.NumberFormat("ja-JP", {
    style: "currency", currency: "JPY", maximumFractionDigits: 0
  });
  const formatUSD = new Intl.NumberFormat("en-US", {
    style: "currency", currency: "USD", maximumFractionDigits: 0
  });
  const convertUSDToJPY = (amount, rateScaled, scale) => {
    if (!Number.isFinite(rateScaled) || rateScaled <= 0 || !Number.isFinite(scale) || scale <= 0) return null;
    return Math.trunc(amount * rateScaled / scale);
  };
  const braceletQuantityPattern = /(?:^|[\s　,、])コマ数[\s　]*[:：][\s　]*(\d+)/g;
  const readBraceletQuantity = (features) => {
    const match = String(features || "").match(/コマ数[\s　]*[:：][\s　]*(\d+)/);
    return match?.[1] || "";
  };
  const writeBraceletQuantity = (features, quantity) => {
    const cleaned = String(features || "").replace(braceletQuantityPattern, " ").trim();
    const value = String(quantity || "").trim();
    return value ? `${cleaned}${cleaned ? "　" : ""}コマ数：${value}` : cleaned;
  };

  const loginCard = document.querySelector("[data-login-card]");
  if (loginCard) {
    const username = loginCard.querySelector("[data-login-username]");
    const password = loginCard.querySelector("[data-login-password]");
    const submit = loginCard.querySelector("[data-login-submit]");
    loginCard.querySelectorAll("[data-login-role]").forEach((button) => {
      button.addEventListener("click", () => {
        const role = button.dataset.loginRole;
        loginCard.querySelectorAll("[data-login-role]").forEach((tab) => {
          const active = tab === button;
          tab.classList.toggle("active", active);
          tab.setAttribute("aria-selected", String(active));
        });
        username.value = role === "admin" ? "admin" : "worker";
        const configuredPassword = role === "admin" ? loginCard.dataset.adminPassword : loginCard.dataset.workerPassword;
        password.value = configuredPassword || "";
        submit.textContent = role === "admin" ? "↪ 管理者としてログイン" : "↪ 作業員としてログイン";
        username.focus();
      });
    });
  }

  const guestFilter = document.querySelector("[data-guest-filter]");
  if (guestFilter) {
    let filterTimer;
    const submitFilter = () => {
      window.clearTimeout(filterTimer);
      if (typeof guestFilter.requestSubmit === "function") {
        guestFilter.requestSubmit();
      } else {
        guestFilter.submit();
      }
    };
    guestFilter.querySelectorAll("select").forEach((select) => {
      select.addEventListener("change", submitFilter);
    });
    guestFilter.querySelector('input[name="q"]')?.addEventListener("input", () => {
      window.clearTimeout(filterTimer);
      filterTimer = window.setTimeout(submitFilter, 300);
    });
  }

  const guestDetailDialog = document.querySelector("[data-guest-detail-dialog]");
  const guestCartDialog = document.querySelector("[data-guest-cart-dialog]");
  const guestCartForm = document.querySelector("[data-guest-cart-form]");
  if (guestDetailDialog && guestCartDialog && guestCartForm) {
    let guestCart = [];
    const cartCount = document.querySelector("[data-guest-cart-count]");
    const cartEmpty = guestCartDialog.querySelector("[data-guest-cart-empty]");
    const cartFilled = guestCartDialog.querySelector("[data-guest-cart-filled]");
    const cartItems = guestCartDialog.querySelector("[data-guest-cart-items]");
    const requestSubmit = guestCartDialog.querySelector("[data-guest-request-submit]");
    const contactFields = guestCartDialog.querySelector("[data-guest-contact-fields]");
    const money = (amount, currency) => new Intl.NumberFormat("ja-JP", {
      style: "currency", currency: currency || "JPY", maximumFractionDigits: 0
    }).format(Number(amount || 0));
    const updateRequestButton = () => {
      if (guestCart.length === 0) {
        requestSubmit.disabled = true;
        return;
      }
      const checks = [...guestCartForm.querySelectorAll('input[type="checkbox"][required]')];
      const checksReady = checks.every((input) => input.checked);
      contactFields.hidden = !checksReady;
      const contacts = [...guestCartForm.querySelectorAll('.guest-contact-fields input[required]')];
      const contactsReady = contacts.every((input) => input.checkValidity());
      const ready = checksReady && contactsReady;
      requestSubmit.disabled = !ready;
      requestSubmit.textContent = ready ? "購入依頼を送信する" :
        checksReady ? "お名前とメールアドレスを入力してください" : "すべての項目にチェックを入れてください";
    };
    const renderCart = () => {
      cartCount.textContent = String(guestCart.length);
      cartCount.hidden = guestCart.length === 0;
      cartEmpty.hidden = guestCart.length > 0;
      cartFilled.hidden = guestCart.length === 0;
      document.querySelectorAll("[data-guest-card]").forEach((card) => {
        const selected = guestCart.some((item) => item.id === card.dataset.guestCard);
        card.classList.toggle("carted", selected);
        const button = card.querySelector("[data-guest-add-cart]");
        if (button) button.textContent = selected ? "✓ カート済" : "🛒 カートに追加";
      });
      if (guestCart.length === 0) {
        cartItems.replaceChildren();
        guestCartDialog.querySelector("[data-cart-total]").textContent = money(0, "JPY");
        updateRequestButton();
        return;
      }
      const rows = guestCart.map((item) => {
        const row = document.createElement("div");
        row.className = "guest-cart-item";
        const icon = document.createElement("span");
        icon.textContent = "◷";
        const copy = document.createElement("div");
        const name = document.createElement("strong");
        name.textContent = `${item.brand} ${item.model}`;
        const condition = document.createElement("small");
        condition.textContent = item.condition || "状態確認済み";
        copy.append(name, condition);
        const price = document.createElement("b");
        price.textContent = money(item.price, item.currency);
        const remove = document.createElement("button");
        remove.type = "button";
        remove.dataset.guestCartRemove = item.id;
        remove.setAttribute("aria-label", "カートから削除");
        remove.textContent = "×";
        const productID = document.createElement("input");
        productID.type = "hidden";
        productID.name = "product_id";
        productID.value = item.id;
        row.append(icon, copy, price, remove, productID);
        return row;
      });
      cartItems.replaceChildren(...rows);
      const currency = guestCart.every((item) => item.currency === guestCart[0].currency) ?
        guestCart[0].currency : "JPY";
      const total = guestCart.reduce((sum, item) => sum + Number(item.price || 0), 0);
      guestCartDialog.querySelector("[data-cart-total]").textContent = money(total, currency);
      updateRequestButton();
    };
    document.addEventListener("click", (event) => {
      const detailButton = event.target.closest("[data-guest-detail]");
      if (detailButton) {
        const template = document.querySelector(`[data-guest-detail-template="${CSS.escape(detailButton.dataset.guestDetail)}"]`);
        if (!template) return;
        guestDetailDialog.innerHTML = template.innerHTML;
        guestDetailDialog.showModal();
        guestDetailDialog.querySelector("[data-guest-detail-close]")?.focus();
        return;
      }
      const addButton = event.target.closest("[data-guest-add-cart]");
      if (addButton) {
        const item = {
          id: addButton.dataset.id, brand: addButton.dataset.brand, model: addButton.dataset.model,
          condition: addButton.dataset.condition, price: addButton.dataset.price, currency: addButton.dataset.currency
        };
        const existingIndex = guestCart.findIndex((cartItem) => cartItem.id === item.id);
        if (existingIndex >= 0) {
          guestCart.splice(existingIndex, 1);
        } else {
          guestCart.push(item);
        }
        renderCart();
        if (guestDetailDialog.open) guestDetailDialog.close();
        return;
      }
      if (event.target.closest("[data-guest-cart-open]")) {
        renderCart();
        guestCartDialog.showModal();
        return;
      }
      if (event.target.closest("[data-guest-detail-close]")) guestDetailDialog.close();
      if (event.target.closest("[data-guest-cart-close]")) guestCartDialog.close();
      const removeButton = event.target.closest("[data-guest-cart-remove]");
      if (removeButton) {
        guestCart = guestCart.filter((item) => item.id !== removeButton.dataset.guestCartRemove);
        renderCart();
      }
      const thumb = event.target.closest("[data-guest-detail-thumb]");
      if (thumb) {
        const main = guestDetailDialog.querySelector("[data-guest-detail-main]");
        if (main) main.src = thumb.dataset.guestDetailThumb;
      }
    });
    guestDetailDialog.addEventListener("click", (event) => {
      if (event.target === guestDetailDialog) guestDetailDialog.close();
    });
    guestCartDialog.addEventListener("click", (event) => {
      if (event.target === guestCartDialog) guestCartDialog.close();
    });
    guestCartForm.addEventListener("input", updateRequestButton);
    guestCartForm.addEventListener("change", updateRequestButton);
    renderCart();
  }

  const menuButton = document.querySelector("[data-menu-button]");
  if (menuButton) {
    menuButton.addEventListener("click", () => {
      const open = document.body.classList.toggle("menu-open");
      menuButton.setAttribute("aria-expanded", String(open));
      menuButton.setAttribute("aria-label", open ? "メニューを閉じる" : "メニューを開く");
    });
  }

  const productRegister = document.querySelector("[data-product-register]");
  if (productRegister) {
    const productCode = productRegister.querySelector("[data-product-code]");
    const purchaseDate = productRegister.querySelector('input[name="purchase_date"]');
    const braceletAccessory = productRegister.querySelector("[data-bracelet-accessory]");
    const braceletQuantityField = productRegister.querySelector("[data-bracelet-quantity-field]");
    const braceletQuantityInput = braceletQuantityField?.querySelector('input[name="bracelet_qty"]');
    const syncProductBraceletQuantity = () => {
      const selected = Boolean(braceletAccessory?.checked);
      if (braceletQuantityField) braceletQuantityField.hidden = !selected;
      if (braceletQuantityInput) {
        braceletQuantityInput.disabled = !selected;
        if (!selected) braceletQuantityInput.value = "";
      }
    };
    braceletAccessory?.addEventListener("change", syncProductBraceletQuantity);
    syncProductBraceletQuantity();
    productRegister.querySelector("[data-product-number]")?.addEventListener("click", async (event) => {
      const button = event.currentTarget;
      button.disabled = true;
      try {
        const params = new URLSearchParams({ purchase_date: purchaseDate?.value || "" });
        const response = await fetch(`${button.dataset.nextCodeUrl}?${params}`, {
          headers: { Accept: "application/json" },
        });
        if (!response.ok) throw new Error("product code request failed");
        const payload = await response.json();
        productCode.value = payload.product_code || "";
      } catch {
        productCode.value = button.dataset.nextCode || "";
      } finally {
        button.disabled = false;
        productCode.focus();
      }
    });
    const clearProductImage = (slot) => {
      const input = slot.querySelector("[data-product-image-input]");
      const preview = slot.querySelector("[data-product-image-preview]");
      if (preview.dataset.objectUrl) URL.revokeObjectURL(preview.dataset.objectUrl);
      input.value = "";
      preview.removeAttribute("src");
      preview.hidden = true;
      delete preview.dataset.objectUrl;
      slot.classList.remove("has-image");
      slot.querySelector("[data-product-image-remove]").hidden = true;
    };
    productRegister.querySelectorAll("[data-product-image-input]").forEach((input) => {
      input.addEventListener("change", () => {
        const slot = input.closest("[data-product-image-slot]");
        const preview = slot.querySelector("[data-product-image-preview]");
        const file = input.files?.[0];
        if (!file) {
          clearProductImage(slot);
          return;
        }
        if (preview.dataset.objectUrl) URL.revokeObjectURL(preview.dataset.objectUrl);
        const objectUrl = URL.createObjectURL(file);
        preview.src = objectUrl;
        preview.dataset.objectUrl = objectUrl;
        preview.hidden = false;
        slot.classList.add("has-image");
        slot.querySelector("[data-product-image-remove]").hidden = false;
      });
    });
    productRegister.querySelectorAll("[data-product-image-remove]").forEach((button) => {
      button.addEventListener("click", (event) => {
        event.preventDefault();
        event.stopPropagation();
        clearProductImage(button.closest("[data-product-image-slot]"));
      });
    });
    productRegister.addEventListener("reset", () => {
      setTimeout(() => {
        productRegister.querySelectorAll("[data-product-image-slot]").forEach(clearProductImage);
        syncProductBraceletQuantity();
      });
    });
    const productSaleInput = productRegister.querySelector("[data-usd-price-input]");
    const productSaleJPY = productRegister.querySelector("[data-product-sale-jpy]");
    const updateProductSaleConversion = () => {
      if (!productSaleJPY) return;
      const converted = productRegister.dataset.rateAvailable === "true"
        ? convertUSDToJPY(
          parseDisplayAmount(productSaleInput?.value),
          Number(productRegister.dataset.usdJpyRate),
          Number(productRegister.dataset.usdJpyScale)
        )
        : null;
      productSaleJPY.textContent = converted === null ? "換算レート未登録" : `約 ${formatJPY.format(converted)}`;
    };
    productSaleInput?.addEventListener("input", updateProductSaleConversion);
    updateProductSaleConversion();
  }

  const purchaseLines = document.querySelector("[data-purchase-lines]");
  const addLine = document.querySelector("[data-add-purchase-line]");
  const purchaseTemplate = document.querySelector("[data-purchase-line-template]");
  const purchaseForm = document.querySelector("[data-purchase-form]");
  if (purchaseLines && addLine && purchaseTemplate && purchaseForm) {
    const yen = formatJPY;
    const usd = formatUSD;
    const parseAmount = parseDisplayAmount;
    const convertedSaleAmount = (amount) => purchaseForm.dataset.rateAvailable === "true"
      ? convertUSDToJPY(amount, Number(purchaseForm.dataset.usdJpyRate), Number(purchaseForm.dataset.usdJpyScale))
      : null;
    const formatSaleJPY = (amount) => {
      const converted = convertedSaleAmount(amount);
      return converted === null ? "換算レート未登録" : `約 ${yen.format(converted)}`;
    };
    const purchaseDate = purchaseForm.querySelector("[data-purchase-date]");
    const purchaseSupplier = purchaseForm.querySelector("[data-purchase-supplier]");
    const lineHeader = purchaseLines.querySelector("[data-purchase-line-header]");
    const productDialog = purchaseForm.querySelector("[data-purchase-product-dialog]");
    const modalBraceletAccessory = productDialog?.querySelector("[data-purchase-modal-bracelet-accessory]");
    const modalBraceletQuantityField = productDialog?.querySelector("[data-purchase-modal-bracelet-quantity]");
    const modalBraceletQuantityInput = productDialog?.querySelector("[data-purchase-modal-bracelet-qty]");
    const syncPurchaseBraceletQuantity = () => {
      const selected = Boolean(modalBraceletAccessory?.checked);
      if (modalBraceletQuantityField) modalBraceletQuantityField.hidden = !selected;
      if (modalBraceletQuantityInput) {
        modalBraceletQuantityInput.disabled = !selected;
        if (!selected) modalBraceletQuantityInput.value = "";
      }
    };
    modalBraceletAccessory?.addEventListener("change", syncPurchaseBraceletQuantity);
    let activePurchaseRow = null;
    let nextPurchaseRowID = 1;
    let nextProductSequence = 1;
    let sequenceDate = "";
    let sequenceRequestID = 0;
    let sequencePromise = null;
    const todayJST = () => {
      const parts = new Intl.DateTimeFormat("en-US", {
        timeZone: "Asia/Tokyo", year: "numeric", month: "2-digit", day: "2-digit"
      }).formatToParts(new Date());
      const values = Object.fromEntries(parts.map((part) => [part.type, part.value]));
      return `${values.year}-${values.month}-${values.day}`;
    };
    const effectivePurchaseDate = () => purchaseDate?.value || todayJST();
    const datePrefix = () => {
      return effectivePurchaseDate().replaceAll("-", "");
    };
    const localNextProductSequence = (prefix) => {
      return [...purchaseLines.querySelectorAll("[data-purchase-row-code]")]
        .map((field) => String(field.value || ""))
        .filter((value) => value.startsWith(prefix))
        .reduce((maximum, value) => Math.max(maximum, Number(value.slice(8)) + 1 || 1), 1);
    };
    const syncPurchaseSequence = async (force = false) => {
      const targetDate = effectivePurchaseDate();
      if (!force && sequenceDate === targetDate && sequencePromise === null) return;
      if (!force && sequenceDate === targetDate && sequencePromise) {
        await sequencePromise;
        return;
      }
      const requestID = sequenceRequestID + 1;
      sequenceRequestID = requestID;
      sequenceDate = targetDate;
      sequencePromise = (async () => {
        try {
          const params = new URLSearchParams({purchase_date: targetDate});
          const response = await fetch(`/products/next-code?${params}`, {headers: {Accept: "application/json"}});
          if (!response.ok) throw new Error(`HTTP ${response.status}`);
          const payload = await response.json();
          if (requestID !== sequenceRequestID || sequenceDate !== targetDate) return;
          const code = String(payload.product_code || "");
          if (!code.startsWith(targetDate.replaceAll("-", ""))) throw new Error("invalid product code");
          const sequence = Number(code.slice(8));
          if (!Number.isInteger(sequence) || sequence < 1 || sequence > 999) throw new Error("invalid sequence");
          const prefix = targetDate.replaceAll("-", "");
          nextProductSequence = Math.max(sequence, localNextProductSequence(prefix));
        } catch (_error) {
          if (requestID === sequenceRequestID) {
            nextProductSequence = localNextProductSequence(targetDate.replaceAll("-", ""));
          }
        } finally {
          if (requestID === sequenceRequestID) sequencePromise = null;
        }
      })();
      await sequencePromise;
    };
    const generatedProductCode = () => {
      const prefix = datePrefix();
      if (!/^\d{8}$/.test(prefix)) return "";
      const code = `${prefix}${String(nextProductSequence).padStart(3, "0")}`;
      nextProductSequence += 1;
      return code;
    };
    const setRowProductCode = (row, code, automatic = false) => {
      const value = String(code || "");
      row.querySelector("[data-purchase-product-code]").textContent = value || "—";
      row.querySelector("[data-purchase-row-code]").value = value;
      row.dataset.codeAutomatic = automatic ? "true" : "false";
    };
    const renumberPurchaseLines = () => {
      [...purchaseLines.querySelectorAll("[data-purchase-line]")].forEach((row, index) => {
        row.querySelector("[data-purchase-line-number]").textContent = String(index + 1);
      });
    };
    const updatePurchaseSummary = () => {
      const rows = [...purchaseLines.querySelectorAll("[data-purchase-line]")];
      let costTotal = 0;
      let saleTotal = 0;
      rows.forEach((row) => {
        const quantity = Math.max(0, Number(row.querySelector('[name="quantity"]')?.value) || 0);
        costTotal += quantity * parseAmount(row.querySelector('[name="unit_cost"]')?.value);
        const saleAmount = parseAmount(row.querySelector('[name="base_sale_price"]')?.value);
        saleTotal += quantity * saleAmount;
        const rowConversion = row.querySelector("[data-purchase-row-sale-jpy]");
        if (rowConversion) rowConversion.textContent = formatSaleJPY(saleAmount);
      });
      document.querySelector("[data-purchase-count]").textContent = String(rows.length);
      document.querySelector("[data-purchase-cost-total]").textContent = yen.format(costTotal);
      document.querySelector("[data-purchase-sale-total]").textContent = usd.format(saleTotal);
      const saleTotalJPY = document.querySelector("[data-purchase-sale-total-jpy]");
      if (saleTotalJPY) saleTotalJPY.textContent = formatSaleJPY(saleTotal);
      document.querySelector("[data-purchase-submit]").disabled = rows.length === 0;
      purchaseLines.classList.toggle("purchase-lines-empty", rows.length === 0);
      purchaseLines.querySelector("[data-purchase-empty]")?.toggleAttribute("hidden", rows.length > 0);
      lineHeader?.toggleAttribute("hidden", rows.length === 0);
      renumberPurchaseLines();
    };
    const appendPurchaseLine = (values = {}) => {
      const clone = purchaseTemplate.content.firstElementChild.cloneNode(true);
      clone.dataset.rowId = `purchase-row-${nextPurchaseRowID}`;
      nextPurchaseRowID += 1;
      Object.entries(values).forEach(([name, value]) => {
        const field = clone.querySelector(`[name="${name}"]`);
        if (field && value !== undefined) field.value = value;
      });
      if (values.product_code && String(values.product_code).startsWith(datePrefix())) {
        const importedSequence = Number(String(values.product_code).slice(8));
        if (Number.isInteger(importedSequence)) nextProductSequence = Math.max(nextProductSequence, importedSequence + 1);
      }
      setRowProductCode(clone, values.product_code || generatedProductCode(), !values.product_code);
      purchaseLines.appendChild(clone);
      updatePurchaseSummary();
      return clone;
    };
    addLine.addEventListener("click", async () => {
      if (addLine.disabled) return;
      addLine.disabled = true;
      try {
        await syncPurchaseSequence(true);
        const row = appendPurchaseLine();
        row.querySelector("[data-purchase-row-sku]")?.focus();
      } finally {
        addLine.disabled = false;
      }
    });
    purchaseLines.addEventListener("click", (event) => {
      const removeButton = event.target.closest("[data-remove-purchase-line]");
      if (removeButton) {
        removeButton.closest("[data-purchase-line]").remove();
        updatePurchaseSummary();
        return;
      }
      const registerButton = event.target.closest("[data-register-purchase-product]");
      if (!registerButton || !productDialog) return;
      activePurchaseRow = registerButton.closest("[data-purchase-line]");
      productDialog.querySelector("[data-purchase-modal-code]").value =
        activePurchaseRow.querySelector("[data-purchase-row-code]").value;
      productDialog.querySelector("[data-purchase-modal-date]").value = purchaseDate?.value || "";
      productDialog.querySelector("[data-purchase-modal-supplier]").value =
        purchaseSupplier?.selectedOptions?.[0]?.dataset.supplierName || "";
      productDialog.querySelectorAll("[data-purchase-modal-field]").forEach((field) => {
        const name = field.dataset.purchaseModalField;
        const rowField = activePurchaseRow.querySelector(`[name="${name}"]`) ||
          activePurchaseRow.querySelector(`[data-purchase-row-detail="${name}"]`);
        field.value = rowField?.value || "";
      });
      const modalSale = productDialog.querySelector('[data-purchase-modal-field="base_sale_price"]');
      const modalSaleJPY = productDialog.querySelector("[data-purchase-modal-sale-jpy]");
      if (modalSaleJPY) modalSaleJPY.textContent = formatSaleJPY(parseAmount(modalSale?.value));
      const selectedAccessories = new Set(
        (activePurchaseRow.querySelector('[data-purchase-row-detail="accessories"]')?.value || "")
          .split(",").map((value) => value.trim()).filter(Boolean)
      );
      productDialog.querySelectorAll("[data-purchase-modal-accessory]").forEach((field) => {
        field.checked = selectedAccessories.has(field.value);
      });
      const modalFeatures = productDialog.querySelector('[data-purchase-modal-field="features"]');
      if (modalBraceletQuantityInput) modalBraceletQuantityInput.value = readBraceletQuantity(modalFeatures?.value);
      syncPurchaseBraceletQuantity();
      productDialog.showModal();
      productDialog.querySelector('[data-purchase-modal-field="sku"]')?.focus();
    });
    purchaseLines.addEventListener("input", updatePurchaseSummary);
    productDialog?.querySelector('[data-purchase-modal-field="base_sale_price"]')?.addEventListener("input", (event) => {
      const conversion = productDialog.querySelector("[data-purchase-modal-sale-jpy]");
      if (conversion) conversion.textContent = formatSaleJPY(parseAmount(event.target.value));
    });
    purchaseDate?.addEventListener("change", async () => {
      addLine.disabled = true;
      await syncPurchaseSequence(true);
      purchaseLines.querySelectorAll('[data-purchase-line][data-code-automatic="true"]').forEach((row) => {
        setRowProductCode(row, generatedProductCode(), true);
      });
      addLine.disabled = false;
    });
    purchaseSupplier?.addEventListener("change", () => {
      if (productDialog?.open) {
        productDialog.querySelector("[data-purchase-modal-supplier]").value =
          purchaseSupplier.selectedOptions?.[0]?.dataset.supplierName || "";
      }
    });
    productDialog?.addEventListener("click", (event) => {
      if (event.target === productDialog || event.target.closest("[data-purchase-product-cancel]")) {
        productDialog.close();
        activePurchaseRow = null;
        return;
      }
      if (!event.target.closest("[data-purchase-product-confirm]") || !activePurchaseRow) return;
      const skuField = productDialog.querySelector('[data-purchase-modal-field="sku"]');
      if (!skuField?.value.trim()) {
        skuField?.setCustomValidity("SKUを入力してください。");
        skuField?.reportValidity();
        return;
      }
      skuField.setCustomValidity("");
      const modalFeatures = productDialog.querySelector('[data-purchase-modal-field="features"]');
      if (modalFeatures) {
        modalFeatures.value = writeBraceletQuantity(
          modalFeatures.value,
          modalBraceletAccessory?.checked ? modalBraceletQuantityInput?.value : ""
        );
      }
      productDialog.querySelectorAll("[data-purchase-modal-field]").forEach((field) => {
        const name = field.dataset.purchaseModalField;
        const rowField = activePurchaseRow.querySelector(`[name="${name}"]`) ||
          activePurchaseRow.querySelector(`[data-purchase-row-detail="${name}"]`);
        if (rowField) rowField.value = field.value;
      });
      const accessories = [...productDialog.querySelectorAll("[data-purchase-modal-accessory]:checked")]
        .map((field) => field.value);
      activePurchaseRow.querySelector('[data-purchase-row-detail="accessories"]').value = accessories.join(", ");
      const registerButton = activePurchaseRow.querySelector("[data-register-purchase-product]");
      registerButton.textContent = "✎ 編集";
      registerButton.classList.add("registered");
      productDialog.close();
      activePurchaseRow = null;
      updatePurchaseSummary();
    });
    purchaseForm.addEventListener("reset", () => {
      setTimeout(() => {
        purchaseLines.querySelectorAll("[data-purchase-line]").forEach((row) => row.remove());
        nextPurchaseRowID = 1;
        nextProductSequence = 1;
        sequenceDate = "";
        void syncPurchaseSequence(true);
        updatePurchaseSummary();
      });
    });
    document.querySelector("[data-purchase-print]")?.addEventListener("click", () => window.print());

    const csvInput = document.querySelector("[data-purchase-csv]");
    csvInput?.addEventListener("change", async () => {
      const file = csvInput.files?.[0];
      if (!file) return;
      const parseCSVRow = (line) => {
        const cells = [];
        let value = "";
        let quoted = false;
        for (let index = 0; index < line.length; index += 1) {
          const char = line[index];
          if (char === '"' && quoted && line[index + 1] === '"') {
            value += '"';
            index += 1;
          } else if (char === '"') {
            quoted = !quoted;
          } else if (char === "," && !quoted) {
            cells.push(value.trim());
            value = "";
          } else {
            value += char;
          }
        }
        cells.push(value.trim());
        return cells;
      };
      const rows = (await file.text()).replace(/^\uFEFF/, "").split(/\r?\n/)
        .filter((line) => line.trim()).map(parseCSVRow);
      const normalizeHeader = (value) => String(value || "").trim().toLowerCase()
        .replace(/[\s_（）()・.]/g, "");
      const aliases = {
        product_code: ["商品コード", "productcode"], sku: ["sku"],
        brand: ["ブランド", "ブランド名", "brand"], product_type: ["商品名", "モデル名", "producttype", "model"],
        model_number: ["型番", "型番ref", "modelnumber", "ref"], serial_number: ["シリアル", "シリアルno", "serialnumber"],
        quantity: ["数量", "quantity"], unit_cost: ["仕入金額", "仕入金額円", "unitcost", "cost"],
        base_sale_price: ["売価", "売価円", "basesaleprice", "saleprice"],
        currency: ["通貨", "仕入通貨", "currency", "costcurrency"],
        sale_currency: ["売価通貨", "salecurrency", "basesalecurrency"],
        material: ["素材", "素材本体", "material"],
        movement: ["駆動方式", "movement"], condition: ["コンディション", "condition"],
        belt_material: ["ベルト素材", "beltmaterial"], dial: ["文字盤", "dial"], box: ["box", "box番号"],
        accessories: ["付属品", "accessories"], features: ["特徴備考", "features"]
      };
      const aliasLookup = new Map();
      Object.entries(aliases).forEach(([key, names]) => names.forEach((name) => aliasLookup.set(normalizeHeader(name), key)));
      const headerKeys = rows[0]?.map((cell) => aliasLookup.get(normalizeHeader(cell)) || "");
      const hasHeader = headerKeys?.some(Boolean);
      const importedRows = [];
      let currencyPolicyError = false;
      (hasHeader ? rows.slice(1) : rows).forEach((cells, index) => {
        if (!cells.some((cell) => cell.trim())) return;
        let values;
        if (hasHeader) {
          values = {};
          headerKeys.forEach((key, cellIndex) => {
            if (key) values[key] = cells[cellIndex] || "";
          });
          if (!values.sku && !values.brand && !values.product_code) return;
        } else {
          if (cells.length < 5) return;
          values = {
            brand: cells[0] || "その他", model_number: cells[1] || "",
            product_type: cells[2] || "腕時計", quantity: cells[3] || "1",
            unit_cost: cells[4] || "0", base_sale_price: cells[5] || "0",
            currency: cells[6] || "JPY", sale_currency: cells[7] || "USD"
          };
        }
        values.sku = values.sku || values.product_code || `CSV-${String(index + 1).padStart(3, "0")}`;
        values.brand = values.brand || "その他";
        values.product_type = values.product_type || "腕時計";
        values.currency = String(values.currency || "JPY").trim().toUpperCase();
        values.sale_currency = String(values.sale_currency || "USD").trim().toUpperCase();
        if (values.currency !== "JPY" || values.sale_currency !== "USD") {
          currencyPolicyError = true;
          return;
        }
        const quantity = Math.min(999, Math.max(1, Number.parseInt(values.quantity, 10) || 1));
        for (let quantityIndex = 0; quantityIndex < quantity; quantityIndex += 1) {
          importedRows.push({
            ...values,
            product_code: quantityIndex === 0 ? values.product_code : "",
            quantity: "1"
          });
        }
      });
      if (currencyPolicyError) {
        window.alert("仕入CSVは「仕入通貨=JPY・売価通貨=USD」の明細だけ取り込めます。既存の別通貨データは数値換算せずに再取込できません。");
        csvInput.value = "";
        return;
      }
      if (!importedRows.length) {
        window.alert("取込可能な明細がありません。CSVの見出しと内容を確認してください。");
        csvInput.value = "";
        return;
      }
      await syncPurchaseSequence(true);
      purchaseLines.querySelectorAll("[data-purchase-line]").forEach((row) => row.remove());
      importedRows.forEach((values) => appendPurchaseLine(values));
      updatePurchaseSummary();
      csvInput.value = "";
    });
    updatePurchaseSummary();
    void syncPurchaseSequence();
  }

  const liveForm = document.querySelector("[data-live-filter]");
  let filterTimer;
  const showPartialError = (message) => {
    const results = document.querySelector("#product-results");
    if (!results) return;
    results.setAttribute("aria-busy", "false");
    results.classList.remove("is-loading");
    let alert = document.querySelector("#partial-error");
    if (!alert) {
      alert = document.createElement("div");
      alert.id = "partial-error";
      alert.className = "alert error";
      alert.setAttribute("role", "alert");
      results.before(alert);
    }
    alert.textContent = message;
  };
  const loadPartial = async (url) => {
    const results = document.querySelector("#product-results");
    if (!results) return;
    results.setAttribute("aria-busy", "true");
    results.classList.add("is-loading");
    try {
      const response = await fetch(url, {
        headers: {"HX-Request": "true", "Accept": "text/html"},
        credentials: "same-origin"
      });
      const redirect = response.headers.get("HX-Redirect");
      if (redirect) {
        window.location.assign(redirect);
        return;
      }
      if (!response.ok) {
        showPartialError("一覧を更新できませんでした。画面を再読み込みしてください。");
        return;
      }
      const html = await response.text();
      const template = document.createElement("template");
      template.innerHTML = html.trim();
      const replacement = template.content.querySelector("#product-results");
      if (!replacement) throw new Error("partial response missing");
      results.replaceWith(replacement);
      document.querySelector("#partial-error")?.remove();
      history.replaceState(null, "", url);
    } catch {
      showPartialError("通信エラーが発生しました。ネットワークを確認して再試行してください。");
    }
  };
  if (liveForm) {
    liveForm.addEventListener("input", (event) => {
      if (!event.target.matches("input, select")) return;
      clearTimeout(filterTimer);
      filterTimer = setTimeout(() => {
        const params = new URLSearchParams(new FormData(liveForm));
        params.delete("page");
        loadPartial(`${liveForm.action}?${params.toString()}`);
      }, event.target.name === "q" ? 350 : 0);
    });
  }
  document.addEventListener("click", (event) => {
    const link = event.target.closest("[data-partial-link]");
    if (!link || !liveForm) return;
    event.preventDefault();
    loadPartial(link.href);
  });

  const productModal = document.querySelector("[data-product-modal]");
  let productModalTrigger = null;
  const openProductModal = async (productID, trigger) => {
    if (!productModal || !productID) return;
    if (trigger) productModalTrigger = trigger;
    productModal.innerHTML = '<div class="product-modal-loading">商品情報を読み込んでいます…</div>';
    if (!productModal.open) productModal.showModal();
    try {
      const response = await fetch(`/products/${encodeURIComponent(productID)}/modal`, {
        headers: {"HX-Request": "true", "Accept": "text/html"},
        credentials: "same-origin"
      });
      if (!response.ok) throw new Error("detail request failed");
      productModal.innerHTML = await response.text();
      productModal.querySelector("[data-product-modal-close]")?.focus();
    } catch {
      productModal.innerHTML = '<div class="product-modal-error" role="alert"><strong>商品情報を読み込めませんでした。</strong><button class="button secondary" type="button" data-product-modal-close>閉じる</button></div>';
    }
  };
  const openProductEditModal = async (productID, trigger) => {
    if (!productModal || !productID) return;
    if (trigger) productModalTrigger = trigger;
    productModal.innerHTML = '<div class="product-modal-loading">編集画面を読み込んでいます…</div>';
    if (!productModal.open) productModal.showModal();
    try {
      const response = await fetch(`/products/${encodeURIComponent(productID)}/edit`, {
        headers: {"HX-Request": "true", "Accept": "text/html"},
        credentials: "same-origin"
      });
      if (!response.ok) throw new Error("edit request failed");
      productModal.innerHTML = await response.text();
      productModal.querySelector("input, select, textarea")?.focus();
    } catch {
      productModal.innerHTML = '<div class="product-modal-error" role="alert"><strong>編集画面を読み込めませんでした。</strong><button class="button secondary" type="button" data-product-modal-close>閉じる</button></div>';
    }
  };
  document.addEventListener("click", (event) => {
    const editButton = event.target.closest("[data-product-edit]");
    if (editButton) {
      event.preventDefault();
      event.stopPropagation();
      openProductEditModal(editButton.dataset.productEdit, editButton);
      return;
    }
    const detailLink = event.target.closest("[data-product-detail]");
    if (detailLink) {
      event.preventDefault();
      openProductModal(detailLink.dataset.productDetail, detailLink);
      return;
    }
    const row = event.target.closest("[data-product-row]");
    if (!row || event.target.closest("[data-product-row-action], button, input, select, textarea")) return;
    openProductModal(row.dataset.productRow, row);
  });
  document.addEventListener("keydown", (event) => {
    const row = event.target.closest("[data-product-row]");
    if (!row || event.target !== row || (event.key !== "Enter" && event.key !== " ")) return;
    event.preventDefault();
    openProductModal(row.dataset.productRow, row);
  });
  productModal?.addEventListener("click", (event) => {
    const closeButton = event.target.closest("[data-product-modal-close]");
    if (closeButton) {
      productModal.close();
      return;
    }
    const editButton = event.target.closest("[data-product-edit]");
    if (editButton) {
      event.stopPropagation();
      openProductEditModal(editButton.dataset.productEdit);
      return;
    }
    const cancelButton = event.target.closest("[data-product-edit-cancel]");
    if (cancelButton) {
      openProductModal(cancelButton.dataset.productEditCancel);
      return;
    }
    const thumbnail = event.target.closest("[data-product-thumbnail]");
    if (thumbnail) {
      const mainImage = productModal.querySelector("[data-product-main-image]");
      if (mainImage) mainImage.src = thumbnail.dataset.productThumbnail;
      productModal.querySelectorAll("[data-product-thumbnail]").forEach((button) => button.classList.toggle("active", button === thumbnail));
      return;
    }
    if (event.target === productModal) productModal.close();
  });
  productModal?.addEventListener("close", () => {
    productModalTrigger?.focus();
    productModalTrigger = null;
  });
  productModal?.addEventListener("submit", async (event) => {
    const form = event.target.closest("[data-product-edit-form]");
    if (!form) return;
    event.preventDefault();
    const submitButton = form.querySelector('button[type="submit"]');
    const errorBox = form.querySelector("[data-product-edit-error]");
    if (submitButton) submitButton.disabled = true;
    if (errorBox) errorBox.hidden = true;
    try {
      const response = await fetch(form.action, {
        method: "POST",
        body: new FormData(form),
        headers: {"Accept": "application/json"},
        credentials: "same-origin"
      });
      if (!response.ok) {
        const payload = await response.json().catch(() => ({}));
        throw new Error(payload.error || "商品情報を更新できませんでした。");
      }
      await openProductModal(form.dataset.productId);
    } catch (error) {
      if (errorBox) {
        errorBox.textContent = error.message;
        errorBox.hidden = false;
        errorBox.scrollIntoView({behavior: "smooth", block: "center"});
      }
    } finally {
      if (submitButton?.isConnected) submitButton.disabled = false;
    }
  });

  const accessoryFilter = document.querySelector(".inventory-accessory-filter");
  if (accessoryFilter) {
    const boxes = [...accessoryFilter.querySelectorAll('input[name="accessory"]')];
    const summary = accessoryFilter.querySelector("[data-accessory-summary]");
    const refreshAccessorySummary = () => {
      const selected = boxes.filter((box) => box.checked).map((box) => box.value);
      summary.textContent = selected.length ? selected.join("・") : "すべて";
    };
    accessoryFilter.querySelector("[data-accessory-all]")?.addEventListener("click", () => {
      boxes.forEach((box) => { box.checked = true; });
      refreshAccessorySummary();
    });
    accessoryFilter.querySelector("[data-accessory-clear]")?.addEventListener("click", () => {
      boxes.forEach((box) => { box.checked = false; });
      refreshAccessorySummary();
    });
    boxes.forEach((box) => box.addEventListener("change", refreshAccessorySummary));
    refreshAccessorySummary();
  }

  const masterDialog = document.querySelector("[data-master-dialog]");
  const masterForm = document.querySelector("[data-master-form]");
  if (masterDialog && masterForm) {
    const title = masterDialog.querySelector("[data-master-dialog-title]");
    const code = masterDialog.querySelector("[data-master-code]");
    const name = masterDialog.querySelector("[data-master-name]");
    const pageCategory = masterForm.querySelector('input[name="category"]')?.value || "";
    const categoryName = document.querySelector("[data-master-category-name]")?.dataset.masterCategoryName || "マスタ";
    const fieldMap = {
      address: '[data-master-extra="address"]',
      contact: '[data-master-extra="contact"]',
      invoiceRegistrationNumber: '[data-master-extra="invoice_registration_number"]',
      representativeName: '[data-master-extra="representative_name"]',
      contactPerson: '[data-master-extra="contact_person"]',
      email: '[data-master-extra="email"]',
      phone: '[data-master-extra="phone"]',
      notes: '[data-master-extra="notes"]'
    };
    document.querySelector("[data-master-open]")?.addEventListener("click", () => {
      masterForm.action = "/masters";
      masterForm.reset();
      title.textContent = `${categoryName} — 新規追加`;
      masterDialog.showModal();
      (code?.type === "hidden" ? name : code)?.focus();
    });
    document.querySelectorAll("[data-master-edit]").forEach((button) => {
      button.addEventListener("click", () => {
        masterForm.action = `/masters/${encodeURIComponent(button.dataset.id)}/update`;
        code.value = button.dataset.code || "";
        name.value = button.dataset.name || "";
        Object.entries(fieldMap).forEach(([key, selector]) => {
          const field = masterForm.querySelector(selector);
          if (field) field.value = button.dataset[key] || "";
        });
        title.textContent = `${categoryName} — 編集`;
        masterDialog.showModal();
        name.focus();
      });
    });
    masterDialog.querySelectorAll("[data-master-close]").forEach((button) => {
      button.addEventListener("click", () => masterDialog.close());
    });
    masterDialog.addEventListener("click", (event) => {
      if (event.target === masterDialog) masterDialog.close();
    });
    if (pageCategory === "accessories") {
      name?.addEventListener("input", () => {
        const selectionStart = name.selectionStart;
        name.value = name.value.toUpperCase();
        name.setSelectionRange(selectionStart, selectionStart);
      });
    }
  }

  const masterDeleteDialog = document.querySelector("[data-master-delete-dialog]");
  if (masterDeleteDialog) {
    const deleteForm = masterDeleteDialog.querySelector("[data-master-delete-form]");
    const deleteName = masterDeleteDialog.querySelector("[data-master-delete-name]");
    document.querySelectorAll("[data-master-delete-open]").forEach((button) => {
      button.addEventListener("click", () => {
        deleteForm.action = `/masters/${encodeURIComponent(button.dataset.id)}/delete`;
        deleteName.textContent = button.dataset.name || "";
        masterDeleteDialog.showModal();
      });
    });
    masterDeleteDialog.querySelectorAll("[data-master-delete-close]").forEach((button) => {
      button.addEventListener("click", () => masterDeleteDialog.close());
    });
  }

  document.querySelectorAll("[data-password-tab]").forEach((button) => {
    button.addEventListener("click", () => {
      document.querySelectorAll("[data-password-tab]").forEach((tab) => tab.classList.toggle("active", tab === button));
      document.querySelectorAll("[data-password-panel]").forEach((panel) => {
        panel.hidden = panel.dataset.passwordPanel !== button.dataset.passwordTab;
      });
    });
  });

  const passwordDialog = document.querySelector("[data-password-dialog]");
  const passwordForm = passwordDialog?.querySelector("[data-password-form]");
  if (passwordDialog && passwordForm) {
    const title = passwordDialog.querySelector("[data-password-title]");
    const newFields = passwordDialog.querySelector("[data-password-new-fields]");
    const userID = passwordDialog.querySelector("[data-password-user-id]");
    const password = passwordDialog.querySelector("[data-password-value]");
    const generatePassword = () => {
      const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789!@#$%";
      const values = new Uint32Array(16);
      window.crypto.getRandomValues(values);
      return [...values].map((value) => alphabet[value % alphabet.length]).join("");
    };
    const openNew = (generated) => {
      passwordForm.reset();
      passwordForm.action = "/masters/passwords/users";
      title.textContent = "利用者 — 新規追加";
      newFields.hidden = false;
      newFields.querySelectorAll("input,select").forEach((field) => { field.disabled = false; });
      userID.value = "";
      password.value = generated ? generatePassword() : "";
      passwordDialog.showModal();
      (generated ? password : newFields.querySelector("input"))?.focus();
    };
    document.querySelector("[data-password-new]")?.addEventListener("click", () => openNew(false));
    document.querySelector("[data-password-generate]")?.addEventListener("click", () => openNew(true));
    document.querySelectorAll("[data-password-change]").forEach((button) => {
      button.addEventListener("click", () => {
        passwordForm.reset();
        passwordForm.action = button.dataset.passwordUrl ||
          `/masters/passwords/users/${encodeURIComponent(button.dataset.id)}/password`;
        title.textContent = `${button.dataset.name || "利用者"} — PW変更`;
        newFields.hidden = true;
        newFields.querySelectorAll("input,select").forEach((field) => { field.disabled = true; });
        userID.value = button.dataset.id || "";
        password.value = "";
        passwordDialog.showModal();
        password.focus();
      });
    });
    passwordDialog.querySelectorAll("[data-password-close]").forEach((button) => {
      button.addEventListener("click", () => passwordDialog.close());
    });
  }

  document.querySelectorAll("[data-confirm]").forEach((control) => {
    control.addEventListener("click", (event) => {
      if (!window.confirm(control.dataset.confirm || "実行しますか？")) event.preventDefault();
    });
  });

  document.querySelectorAll("[data-fx-card]").forEach((form) => {
    const rate = form.querySelector("[data-fx-rate]");
    const preview = form.querySelector("[data-fx-preview]");
    const example = form.querySelector("[data-fx-example]");
    const update = () => {
      if (preview) preview.textContent = rate?.value || "—";
      if (example) {
        const numericRate = Number(rate?.value);
        example.textContent = Number.isFinite(numericRate)
          ? Math.round(numericRate * 1000).toLocaleString("ja-JP")
          : "—";
      }
    };
    rate?.addEventListener("input", update);
    update();
  });

  const settingsForm = document.querySelector("[data-master-settings-form]");
  if (settingsForm) {
    const actions = settingsForm.querySelector("[data-settings-actions]");
    const setEditing = (editing) => {
      settingsForm.querySelectorAll("input:not([type=hidden])").forEach((input) => { input.disabled = !editing; });
      actions.hidden = !editing;
    };
    settingsForm.querySelector("[data-settings-edit]")?.addEventListener("click", () => setEditing(true));
    settingsForm.querySelector("[data-settings-cancel]")?.addEventListener("click", () => {
      settingsForm.reset();
      setEditing(false);
    });
  }

  const dashboardSettings = document.querySelector(".dashboard-settings");
  if (dashboardSettings) {
    const sales = dashboardSettings.querySelector('input[name="monthly_sales_target"]');
    const purchase = dashboardSettings.querySelector('input[name="monthly_purchase_budget"]');
    const format = (value) => `¥${Number(String(value || "0").replaceAll(",", "") || 0).toLocaleString("ja-JP")}`;
    const refresh = () => {
      dashboardSettings.querySelector("[data-dashboard-sales-preview]").textContent = format(sales?.value);
      dashboardSettings.querySelector("[data-dashboard-purchase-preview]").textContent = format(purchase?.value);
    };
    sales?.addEventListener("input", refresh);
    purchase?.addEventListener("input", refresh);
    dashboardSettings.addEventListener("reset", () => window.setTimeout(refresh));
    refresh();
  }

  const guestBoxDialog = document.querySelector("[data-guest-box-dialog]");
  const guestBoxEditorDialog = document.querySelector("[data-guest-box-editor-dialog]");
  if (guestBoxDialog && guestBoxEditorDialog) {
    const responseText = async (response) => {
      const html = await response.text();
      if (!response.ok) {
        const message = new DOMParser().parseFromString(html, "text/html").body.textContent.trim();
        throw new Error(message || "BOX情報を読み込めませんでした。");
      }
      return html;
    };
    const renderGuestBoxError = (dialog, message, retry) => {
      dialog.innerHTML = '<div class="purchase-workflow-loading workflow-load-error" role="alert"><strong></strong><div><button class="button secondary" type="button" data-guest-box-error-close>閉じる</button><button class="button primary" type="button" data-guest-box-error-retry>再試行</button></div></div>';
      dialog.querySelector("strong").textContent = message;
      dialog.querySelector("[data-guest-box-error-close]")?.addEventListener("click", () => dialog.close());
      dialog.querySelector("[data-guest-box-error-retry]")?.addEventListener("click", retry);
      if (!dialog.open) dialog.showModal();
    };
    const loadGuestBox = async (boxID) => {
      guestBoxDialog.innerHTML = '<div class="purchase-workflow-loading">BOX情報を読み込んでいます…</div>';
      if (!guestBoxDialog.open) guestBoxDialog.showModal();
      try {
        const response = await fetch(`/guest-management/boxes/${encodeURIComponent(boxID)}/modal`, {
          credentials: "same-origin", headers: {"HX-Request": "true"}
        });
        guestBoxDialog.innerHTML = await responseText(response);
        wireGuestBoxDialog();
      } catch (_error) {
        renderGuestBoxError(
          guestBoxDialog,
          "BOX情報を読み込めませんでした。通信状態を確認して再試行してください。",
          () => loadGuestBox(boxID)
        );
      }
    };
    const loadGuestBoxEditor = async (boxID, query = "") => {
      const suffix = query ? `?${query}` : "";
      guestBoxEditorDialog.innerHTML = '<div class="purchase-workflow-loading">BOX編集画面を読み込んでいます…</div>';
      if (!guestBoxEditorDialog.open) guestBoxEditorDialog.showModal();
      try {
        const response = await fetch(`/guest-management/boxes/${encodeURIComponent(boxID)}/edit-modal${suffix}`, {
          credentials: "same-origin", headers: {"HX-Request": "true"}
        });
        guestBoxEditorDialog.innerHTML = await responseText(response);
        wireGuestBoxEditor();
      } catch (_error) {
        renderGuestBoxError(
          guestBoxEditorDialog,
          "BOX編集画面を読み込めませんでした。通信状態を確認して再試行してください。",
          () => loadGuestBoxEditor(boxID, query)
        );
      }
    };
    const wireGuestBoxDialog = () => {
      guestBoxDialog.querySelectorAll("[data-guest-box-close]").forEach((button) => {
        button.addEventListener("click", () => guestBoxDialog.close());
      });
      guestBoxDialog.querySelector("[data-guest-box-editor-open]")?.addEventListener("click", (event) => {
        const boxID = event.currentTarget.dataset.guestBoxEditorOpen;
        guestBoxDialog.close();
        loadGuestBoxEditor(boxID);
      });
    };
    const wireGuestBoxEditor = () => {
      guestBoxEditorDialog.querySelectorAll("[data-guest-box-editor-close]").forEach((button) => {
        button.addEventListener("click", () => guestBoxEditorDialog.close());
      });
      const searchForm = guestBoxEditorDialog.querySelector("[data-guest-product-search-form]");
      searchForm?.addEventListener("submit", (event) => {
        event.preventDefault();
        const boxID = guestBoxEditorDialog.querySelector("[data-guest-box-editor-content]")?.dataset.boxId;
        loadGuestBoxEditor(boxID, new URLSearchParams(new FormData(searchForm)).toString());
      });
      guestBoxEditorDialog.querySelectorAll("[data-guest-editor-form]").forEach((form) => {
        form.addEventListener("submit", async (event) => {
          event.preventDefault();
          const response = await fetch(form.action, {
            method: "POST", body: new FormData(form), credentials: "same-origin",
            headers: {"HX-Request": "true"}
          });
          const html = await responseText(response);
          if (!html) return;
          guestBoxEditorDialog.innerHTML = html;
          wireGuestBoxEditor();
        });
      });
      const candidates = Array.from(guestBoxEditorDialog.querySelectorAll("[data-guest-candidate]"));
      const selectedCount = guestBoxEditorDialog.querySelector("[data-guest-selected-count]");
      const addButton = guestBoxEditorDialog.querySelector("[data-guest-bulk-add-button]");
      const updateSelection = () => {
        const count = candidates.filter((candidate) => candidate.checked).length;
        if (selectedCount) selectedCount.textContent = `${count}件選択`;
        if (addButton) addButton.disabled = count === 0;
      };
      candidates.forEach((candidate) => candidate.addEventListener("change", updateSelection));
      updateSelection();
    };
    document.querySelectorAll("[data-guest-box-open]").forEach((button) => {
      button.addEventListener("click", () => loadGuestBox(button.dataset.guestBoxOpen));
    });
  }

  const guestBoxEditDialog = document.querySelector("[data-guest-box-edit-dialog]");
  if (guestBoxEditDialog) {
    const form = guestBoxEditDialog.querySelector("[data-guest-box-edit-form]");
    const input = guestBoxEditDialog.querySelector("[data-guest-box-name]");
    document.querySelectorAll("[data-guest-box-edit]").forEach((button) => {
      button.addEventListener("click", () => {
        form.action = `/guest-management/boxes/${encodeURIComponent(button.dataset.guestBoxEdit)}`;
        input.value = button.dataset.boxName || "";
        guestBoxEditDialog.showModal();
      });
    });
    guestBoxEditDialog.querySelectorAll("[data-guest-box-edit-close]").forEach((button) => {
      button.addEventListener("click", () => guestBoxEditDialog.close());
    });
  }

  const stocktakeDialog = document.querySelector("[data-stocktake-dialog]");
  if (stocktakeDialog) {
    document.querySelectorAll("[data-stocktake-open]").forEach((button) => {
      button.addEventListener("click", () => stocktakeDialog.showModal());
    });
    stocktakeDialog.querySelectorAll("[data-stocktake-close]").forEach((button) => {
      button.addEventListener("click", () => stocktakeDialog.close());
    });
    stocktakeDialog.addEventListener("click", (event) => {
      if (event.target === stocktakeDialog) stocktakeDialog.close();
    });
  }

  const stocktakeScanForm = document.querySelector("[data-stocktake-scan-form]");
  if (stocktakeScanForm) {
    const codeInput = stocktakeScanForm.querySelector("[data-stocktake-code]");
    const scanMessage = document.querySelector("[data-stocktake-scan-message]");
    const stocktakeRows = Array.from(document.querySelectorAll("[data-stocktake-row]"));
    const toast = document.querySelector("[data-stocktake-toast]");
    const showMessage = (message, error = false) => {
      if (!scanMessage) return;
      scanMessage.textContent = message;
      scanMessage.style.color = error ? "#d94036" : "#2187bb";
      if (toast) {
        toast.textContent = message;
        toast.classList.toggle("error", error);
        toast.hidden = false;
        window.setTimeout(() => { toast.hidden = true; }, 3200);
      }
    };
    const confirmRow = (row) => {
      if (!row) return false;
      if (row.dataset.stocktakeCounted === "true") {
        showMessage(`商品コード「${row.dataset.productCode}」は棚卸済です。`, true);
        return false;
      }
      const form = row.querySelector("form");
      if (!form) return false;
      form.querySelector("[data-stocktake-result]").value = "present";
      form.requestSubmit();
      return true;
    };

    stocktakeScanForm.addEventListener("submit", (event) => {
      event.preventDefault();
      const value = (codeInput?.value || "").trim();
      if (!value) {
        showMessage("商品コードを入力してください。", true);
        codeInput?.focus();
        return;
      }
      const row = stocktakeRows.find((item) => item.dataset.productCode === value);
      if (!row) {
        showMessage(`商品コード「${value}」は棚卸リストにありません。`, true);
        codeInput?.select();
        return;
      }
      showMessage(`${value} を確認しました。`);
      confirmRow(row);
    });

    document.querySelectorAll("[data-stocktake-row-confirm]").forEach((button) => {
      button.addEventListener("click", () => confirmRow(button.closest("[data-stocktake-row]")));
    });

    const differenceDialog = document.querySelector("[data-stocktake-difference-dialog]");
    let differenceRow = null;
    document.querySelectorAll("[data-stocktake-row-difference]").forEach((button) => {
      button.addEventListener("click", () => {
        differenceRow = button.closest("[data-stocktake-row]");
        differenceDialog?.showModal();
      });
    });
    document.querySelector("[data-stocktake-difference-submit]")?.addEventListener("click", () => {
      const reason = differenceDialog?.querySelector('input[name="stocktake_reason_ui"]:checked')?.value || "";
      if (!reason) {
        showMessage("不一致理由を選択してください。", true);
        return;
      }
      const form = differenceRow?.querySelector("form");
      if (!form) return;
      form.querySelector("[data-stocktake-result]").value = "absent";
      form.querySelector("[data-stocktake-reason]").value = reason;
      form.querySelector("[data-stocktake-notes]").value =
        differenceDialog.querySelector("[data-stocktake-difference-note]")?.value || "";
      form.requestSubmit();
    });

    document.querySelectorAll("[data-stocktake-tab]").forEach((button) => {
      button.addEventListener("click", () => {
        const kind = button.dataset.stocktakeTab;
        document.querySelectorAll("[data-stocktake-tab]").forEach((tab) => {
          const active = tab === button;
          tab.classList.toggle("active", active);
          tab.setAttribute("aria-selected", String(active));
        });
        stocktakeRows.forEach((row) => {
          row.hidden = row.dataset.stocktakeKind !== kind;
        });
      });
    });

    document.querySelector("[data-stocktake-differences]")?.addEventListener("click", () => {
      document.querySelector("[data-stocktake-difference-list-dialog]")?.showModal();
    });

    const completeDialog = document.querySelector("[data-stocktake-complete-dialog]");
    document.querySelector("[data-stocktake-complete-open]")?.addEventListener("click", () => completeDialog?.showModal());
    document.querySelector("[data-stocktake-complete-submit]")?.addEventListener("click", () => {
      document.querySelector("[data-stocktake-complete-form]")?.requestSubmit();
    });
    document.querySelector("[data-stocktake-print]")?.addEventListener("click", () => window.print());
    document.querySelector("[data-stocktake-export]")?.addEventListener("click", () => {
      const rows = [["商品コード", "ブランド", "モデル", "状態"]];
      stocktakeRows.forEach((row) => rows.push([
        row.dataset.productCode,
        row.children[2]?.textContent.trim() || "",
        row.children[3]?.textContent.trim() || "",
        row.children[0]?.textContent.trim() || "",
      ]));
      const blob = new Blob(["\uFEFF" + rows.map((values) => values.map((value) => `"${String(value).replaceAll('"', '""')}"`).join(",")).join("\r\n")], {type: "text/csv;charset=utf-8"});
      const link = document.createElement("a");
      link.href = URL.createObjectURL(blob);
      link.download = "stocktake.csv";
      link.click();
      URL.revokeObjectURL(link.href);
    });
    document.querySelectorAll("[data-stocktake-dialog-close]").forEach((button) => {
      button.addEventListener("click", () => button.closest("dialog")?.close());
    });

    const reorderRows = () => {
      const body = stocktakeRows[0]?.parentElement;
      if (!body) return;
      stocktakeRows
        .slice()
        .sort((left, right) => Number(left.dataset.stocktakeCounted === "true") - Number(right.dataset.stocktakeCounted === "true"))
        .forEach((row) => body.appendChild(row));
    };
    reorderRows();
  }

  const performanceFrom = document.querySelector("[data-performance-from]");
  const performanceTo = document.querySelector("[data-performance-to]");
  if (performanceFrom && performanceTo) {
    const dateValue = (date) => {
      const year = date.getFullYear();
      const month = String(date.getMonth() + 1).padStart(2, "0");
      const day = String(date.getDate()).padStart(2, "0");
      return `${year}-${month}-${day}`;
    };
    document.querySelectorAll("[data-performance-months]").forEach((button) => {
      button.addEventListener("click", () => {
        const months = Number(button.dataset.performanceMonths);
        const now = new Date();
        const start = months === 0
          ? new Date(now.getFullYear(), now.getMonth(), 1)
          : new Date(now.getFullYear(), now.getMonth() - months + 1, 1);
        const end = new Date(now.getFullYear(), now.getMonth() + 1, 0);
        performanceFrom.value = dateValue(start);
        performanceTo.value = dateValue(end);
      });
    });
  }

  const buyerChart = document.querySelector("[data-buyer-bar-chart]");
  if (buyerChart) {
    const chartButtons = [...document.querySelectorAll("[data-buyer-chart-mode]")];
    const chartBars = [...buyerChart.querySelectorAll(".buyer-bar")];
    const showBuyerChartMode = (mode) => {
      chartButtons.forEach((button) => button.classList.toggle("active", button.dataset.buyerChartMode === mode));
      chartBars.forEach((bar) => {
        const height = Number(mode === "profit" ? bar.dataset.profit : bar.dataset.revenue) || 0;
        bar.style.height = `${Math.max(0, Math.min(100, height))}%`;
      });
    };
    chartButtons.forEach((button) => button.addEventListener("click", () => showBuyerChartMode(button.dataset.buyerChartMode)));
    showBuyerChartMode("revenue");
  }
  document.querySelectorAll("[data-consumption]").forEach((bar) => {
    const percentage = Math.max(0, Math.min(100, Number(bar.dataset.consumption) || 0));
    bar.style.flexBasis = `${percentage}%`;
  });
  document.querySelectorAll("[data-consumption-width]").forEach((bar) => {
    const percentage = Math.max(0, Math.min(100, Number(bar.dataset.consumptionWidth) || 0));
    bar.style.width = `${percentage}%`;
  });

  const destinationDonut = document.querySelector("[data-destination-donut]");
  if (destinationDonut) {
    const destinationButtons = [...document.querySelectorAll("[data-destination-chart-mode]")];
    const destinationSlices = [...destinationDonut.querySelectorAll(".donut-slice")];
    const destinationLegendValues = [...destinationDonut.querySelectorAll("[data-revenue-composition]")];
    const showDestinationChartMode = (mode) => {
      destinationButtons.forEach((button) => button.classList.toggle("active", button.dataset.destinationChartMode === mode));
      destinationSlices.forEach((slice) => {
        const dash = mode === "profit" ? slice.dataset.profitDash : slice.dataset.revenueDash;
        const offset = mode === "profit" ? slice.dataset.profitOffset : slice.dataset.revenueOffset;
        slice.style.strokeDasharray = `${Number(dash) || 0} 100`;
        slice.style.strokeDashoffset = String(Number(offset) || 0);
      });
      destinationLegendValues.forEach((value) => {
        value.textContent = mode === "profit" ? value.dataset.profitComposition : value.dataset.revenueComposition;
      });
    };
    destinationButtons.forEach((button) => button.addEventListener("click", () => showDestinationChartMode(button.dataset.destinationChartMode)));
    showDestinationChartMode("revenue");
  }
  document.querySelectorAll("[data-destination-comparison-chart] [data-bar-height]").forEach((bar) => {
    const height = Math.max(0, Math.min(100, Number(bar.dataset.barHeight) || 0));
    bar.style.height = `${height}%`;
  });

  const salesForm = document.querySelector("[data-sales-form]");
  const salesLines = document.querySelector("[data-sales-lines]");
  const salesTemplate = document.querySelector("[data-sales-line-template]");
  if (salesForm && salesLines && salesTemplate) {
    const yen = (value) => `￥${Math.max(0, value).toLocaleString("ja-JP")}`;
    const numeric = (value) => Number(String(value || "").replace(/[^\d-]/g, "")) || 0;
    const taxSwitch = salesForm.querySelector("[data-sales-tax-switch]");
    const taxMode = salesForm.querySelector("[data-sales-tax-mode]");
    const taxLabel = salesForm.querySelector("[data-sales-tax-label]");
    const slipNumberInput = salesForm.querySelector("[data-sales-slip-number]");
    const shipmentIDInput = salesForm.querySelector("[data-sales-shipment-id]");
    const autoNumberButton = salesForm.querySelector("[data-sales-auto-number]");
    const autoNumberState = salesForm.querySelector("[data-sales-auto-number-state]");
    const salesDateInput = salesForm.querySelector('[name="sales_date"]');
    const shipmentLookupStatus = salesForm.querySelector("[data-sales-shipment-lookup-status]");
    const productOptions = [...document.querySelectorAll("[data-sales-product-option]")];
    const productOptionsByCode = new Map(
      productOptions.map((option) => [option.dataset.code.trim(), option])
    );
    const selectedProductOption = (row) => {
      const code = row.querySelector("[data-sales-product-code]")?.value.trim();
      return productOptionsByCode.get(code);
    };
    const setSalesLinePosted = (row) => {
      const posted = row.querySelector("[data-sales-posted]").checked;
      row.classList.toggle("is-takehome", !posted);
      row.querySelectorAll("input[name], [data-sales-product-code]").forEach((input) => {
        input.disabled = !posted;
      });
    };
    const updateSalesTotals = () => {
      const postedRows = [...salesLines.querySelectorAll("[data-sales-line]")]
        .filter((row) => row.querySelector("[data-sales-posted]").checked);
      const subtotal = postedRows
        .reduce((sum, row) => {
          const quantity = numeric(row.querySelector('input[name="quantity"]')?.value) || 1;
          return sum + numeric(row.querySelector("[data-sales-price]").value) * quantity;
        }, 0);
    const exempt = taxSwitch?.checked || false;
    const tax = exempt ? 0 : Math.floor(subtotal * 0.1);
    salesForm.classList.toggle("is-tax-exempt", exempt);
    document.body.classList.toggle("sales-tax-exempt", exempt);
    if (taxMode) taxMode.value = exempt ? "exempt" : "normal";
      if (taxLabel) taxLabel.textContent = exempt ? "免税" : "通常";
      document.querySelector("[data-sales-subtotal]").textContent = yen(subtotal);
      document.querySelector("[data-sales-tax]").textContent = yen(tax);
      document.querySelector("[data-sales-total]").textContent = yen(subtotal + tax);
      document.querySelector("[data-sales-submit]").disabled =
        postedRows.length === 0 ||
        postedRows.some((row) => !row.querySelector("[data-sales-product]").value);
    };
    const fillSalesLine = (row) => {
      const option = selectedProductOption(row);
      row.querySelector("[data-sales-product]").value = option?.dataset.id || "";
      const values = {
        brand: option?.dataset.brand || "—",
        model: option?.dataset.model || "—",
        reference: option?.dataset.reference || "—",
        serial: option?.dataset.serial || "—",
        accessories: option?.dataset.accessories || "—"
      };
      Object.entries(values).forEach(([key, value]) => {
        row.querySelector(`[data-sales-${key}]`).textContent = value;
      });
      row.querySelector("[data-sales-price]").value = option ? option.dataset.price || "0" : "0";
      const codeInput = row.querySelector("[data-sales-product-code]");
      codeInput.setCustomValidity(option || !codeInput.value ? "" : "一覧にある管理番号を入力してください。");
      updateSalesTotals();
    };
    const addSalesLine = (prefill = null) => {
      const row = salesTemplate.content.firstElementChild.cloneNode(true);
      salesLines.appendChild(row);
      setSalesLinePosted(row);
      if (prefill) {
        const option = productOptions.find((candidate) => candidate.dataset.id === prefill.productId);
        if (option) {
          row.querySelector("[data-sales-product-code]").value = option.dataset.code;
          fillSalesLine(row);
          row.querySelector("[data-sales-price]").value = prefill.price || option.dataset.price || "0";
        } else {
          row.querySelector("[data-sales-product-code]").value = prefill.productCode || "";
          row.querySelector("[data-sales-product]").value = prefill.productId || "";
          row.querySelector("[data-sales-brand]").textContent = prefill.brand || "—";
          row.querySelector("[data-sales-model]").textContent = prefill.model || "—";
          row.querySelector("[data-sales-reference]").textContent = prefill.reference || "—";
          row.querySelector("[data-sales-serial]").textContent = prefill.serial || "—";
          row.querySelector("[data-sales-accessories]").textContent = prefill.accessories || "—";
          row.querySelector("[data-sales-price]").value = prefill.price || "0";
        }
        row.querySelector('input[name="quantity"]').value = prefill.quantity || "1";
      }
      updateSalesTotals();
      if (!prefill) row.querySelector("[data-sales-product-code]").focus();
    };
    document.querySelector("[data-add-sales-line]").addEventListener("click", addSalesLine);
    salesLines.addEventListener("change", (event) => {
      const row = event.target.closest("[data-sales-line]");
      if (event.target.matches("[data-sales-product-code]")) fillSalesLine(row);
      if (event.target.matches("[data-sales-posted]")) {
        setSalesLinePosted(row);
        updateSalesTotals();
      }
    });
    salesLines.addEventListener("input", (event) => {
      if (event.target.matches("[data-sales-product-code]")) fillSalesLine(event.target.closest("[data-sales-line]"));
      if (event.target.matches("[data-sales-price]")) updateSalesTotals();
    });
    salesLines.addEventListener("keydown", (event) => {
      if (event.key !== "Enter" || !event.target.matches("[data-sales-product-code]")) return;
      event.preventDefault();
      fillSalesLine(event.target.closest("[data-sales-line]"));
      event.target.reportValidity();
    });
    salesLines.addEventListener("click", (event) => {
      const remove = event.target.closest("[data-remove-sales-line]");
      if (!remove) return;
      remove.closest("[data-sales-line]").remove();
      if (!salesLines.children.length) addSalesLine();
      updateSalesTotals();
    });
    document.querySelector("[data-sales-print]").addEventListener("click", () => window.print());
    document.querySelector("[data-sales-csv]").addEventListener("click", () => {
      const rows = [["商品コード", "ブランド", "モデル名", "型番", "シリアル", "付属品", "販売金額"]];
      [...salesLines.querySelectorAll("[data-sales-line]")]
        .filter((row) => row.querySelector("[data-sales-posted]").checked)
        .forEach((row) => {
        const option = selectedProductOption(row);
        rows.push([
          option?.dataset.code || "", option?.dataset.brand || "", option?.dataset.model || "",
          option?.dataset.reference || "", option?.dataset.serial || "", option?.dataset.accessories || "",
          row.querySelector("[data-sales-price]").value
        ]);
      });
      const csv = rows.map((row) => row.map((cell) => `"${String(cell).replaceAll('"', '""')}"`).join(",")).join("\r\n");
      const link = document.createElement("a");
      link.href = URL.createObjectURL(new Blob(["\ufeff", csv], {type: "text/csv"}));
      link.download = "sales-slip.csv";
      link.click();
      URL.revokeObjectURL(link.href);
    });
    taxSwitch?.addEventListener("change", updateSalesTotals);
    let autoNumberController = null;
    const requestAutoNumber = async () => {
      const salesDate = salesDateInput?.value || "";
      if (!salesDate) {
        salesDateInput?.reportValidity();
        return;
      }
      autoNumberController?.abort();
      autoNumberController = new AbortController();
      autoNumberButton?.setAttribute("aria-busy", "true");
      slipNumberInput?.setAttribute("aria-busy", "true");
      if (shipmentLookupStatus) {
        shipmentLookupStatus.classList.remove("is-error", "is-success");
        shipmentLookupStatus.textContent = "売上伝票番号を確認しています…";
      }
      try {
        const response = await fetch(`/sales/next-number?date=${encodeURIComponent(salesDate)}`, {
          headers: {"Accept": "application/json"},
          signal: autoNumberController.signal
        });
        if (!response.ok) {
          throw new Error((await response.text()).trim() || "売上伝票番号を採番できませんでした");
        }
        const payload = await response.json();
        shipmentIDInput.value = "";
        autoNumberState.value = "1";
        slipNumberInput.value = payload.sales_slip_number;
        slipNumberInput.readOnly = true;
        delete slipNumberInput.dataset.importedSalesNumber;
        if (shipmentLookupStatus) {
          shipmentLookupStatus.classList.add("is-success");
          shipmentLookupStatus.textContent =
            `${payload.sales_slip_number} を候補として表示しています（保存時に重複を再確認します）`;
        }
      } catch (error) {
        if (error.name === "AbortError") return;
        autoNumberState.value = "0";
        slipNumberInput.readOnly = false;
        if (shipmentLookupStatus) {
          shipmentLookupStatus.classList.add("is-error");
          shipmentLookupStatus.textContent = error.message;
        }
      } finally {
        autoNumberButton?.removeAttribute("aria-busy");
        slipNumberInput?.removeAttribute("aria-busy");
      }
    };
    autoNumberButton?.addEventListener("click", requestAutoNumber);
    salesDateInput?.addEventListener("change", () => {
      if (autoNumberState?.value === "1") requestAutoNumber();
    });
    let shipmentLookupController = null;
    const lookupShipment = async () => {
      const number = slipNumberInput?.value.trim().toUpperCase() || "";
      if (!number.startsWith("SH-")) return;
      shipmentLookupController?.abort();
      shipmentLookupController = new AbortController();
      slipNumberInput.setAttribute("aria-busy", "true");
      if (shipmentLookupStatus) {
        shipmentLookupStatus.classList.remove("is-error", "is-success");
        shipmentLookupStatus.textContent = "出荷伝票を検索しています…";
      }
      try {
        const response = await fetch(`/sales/shipment-prefill?number=${encodeURIComponent(number)}`, {
          headers: {"Accept": "application/json"},
          signal: shipmentLookupController.signal
        });
        if (!response.ok) {
          throw new Error((await response.text()).trim() || "出荷伝票を取り込めませんでした");
        }
        const payload = await response.json();
        shipmentIDInput.value = payload.shipment_id;
        autoNumberState.value = "0";
        slipNumberInput.value = payload.sales_slip_number;
        slipNumberInput.dataset.importedSalesNumber = payload.sales_slip_number;
        slipNumberInput.readOnly = true;
        const customer = salesForm.querySelector('[name="customer_name"]');
        if (customer && ![...customer.options].some((option) => option.value === payload.recipient_name)) {
          customer.add(new Option(payload.recipient_name, payload.recipient_name));
        }
        if (customer) customer.value = payload.recipient_name;
        salesLines.replaceChildren();
        payload.lines.forEach((line) => addSalesLine({
          productId: line.product_id,
          productCode: line.product_code,
          brand: line.brand,
          model: line.model,
          reference: line.reference,
          serial: line.serial,
          accessories: line.accessories,
          quantity: line.quantity,
          price: line.price
        }));
        const notes = salesForm.querySelector('[name="notes"]');
        if (notes && !notes.value) notes.value = payload.notes || "";
        if (shipmentLookupStatus) {
          shipmentLookupStatus.classList.add("is-success");
          shipmentLookupStatus.textContent =
            `${payload.shipment_number} を取り込み、保存時に ${payload.sales_slip_number} を割り当てます`;
        }
        updateSalesTotals();
      } catch (error) {
        if (error.name === "AbortError") return;
        shipmentIDInput.value = "";
        autoNumberState.value = "0";
        if (shipmentLookupStatus) {
          shipmentLookupStatus.classList.add("is-error");
          shipmentLookupStatus.textContent = error.message;
        }
      } finally {
        slipNumberInput.removeAttribute("aria-busy");
      }
    };
    slipNumberInput?.addEventListener("keydown", (event) => {
      if (event.key !== "Enter") return;
      event.preventDefault();
      lookupShipment();
    });
    slipNumberInput?.addEventListener("change", lookupShipment);
    salesForm.addEventListener("reset", () => {
      window.setTimeout(() => {
        shipmentIDInput.value = "";
        autoNumberState.value = "0";
        slipNumberInput.readOnly = false;
        delete slipNumberInput.dataset.importedSalesNumber;
        if (shipmentLookupStatus) {
          shipmentLookupStatus.classList.remove("is-error", "is-success");
          shipmentLookupStatus.textContent = "";
        }
        salesLines.replaceChildren();
        addSalesLine();
        updateSalesTotals();
      });
    });
    salesForm.addEventListener("submit", () => {
      salesLines.querySelectorAll("[data-sales-line]").forEach(setSalesLinePosted);
      updateSalesTotals();
    });
    const prefill = document.querySelector("[data-sales-prefill]");
    const prefillLines = [...(prefill?.querySelectorAll("[data-product-id]") || [])];
    if (prefillLines.length) {
      prefillLines.forEach((line) => addSalesLine({
        productId: line.dataset.productId,
        price: line.dataset.price,
        quantity: line.dataset.quantity
      }));
      const recipient = prefill.dataset.recipient;
      const customer = salesForm.querySelector('[name="customer_name"]');
      if (recipient && customer && [...customer.options].some((option) => option.value === recipient)) {
        customer.value = recipient;
      }
    } else {
      addSalesLine();
    }
  }

  const shipmentForm = document.querySelector("[data-shipment-form]");
  if (shipmentForm) {
    const shipmentLines = shipmentForm.querySelector("[data-shipment-lines]");
    const shipmentTemplate = shipmentForm.querySelector("[data-shipment-line-template]");
    const shipmentOptions = [...shipmentForm.querySelectorAll("[data-shipment-product-option]")];
    const shipmentOptionsByCode = new Map(
      shipmentOptions.map((option) => [option.dataset.code.trim(), option])
    );
    const money = (value) => `¥${Math.max(0, Number(value) || 0).toLocaleString("ja-JP")}`;
    const selectedShipmentOption = (row) => {
      const code = row.querySelector("[data-shipment-product-code]").value.trim();
      return shipmentOptionsByCode.get(code);
    };
    const updateShipmentTotal = () => {
      const total = [...shipmentLines.querySelectorAll("[data-shipment-price]")]
        .reduce((sum, input) => sum + (Number(input.value) || 0), 0);
      shipmentForm.querySelector("[data-shipment-total]").textContent = money(total);
    };
    const fillShipmentLine = (row) => {
      const option = selectedShipmentOption(row);
      row.querySelector("[data-shipment-product]").value = option?.dataset.id || "";
      row.querySelector("[data-shipment-brand]").textContent = option?.dataset.brand || "―";
      row.querySelector("[data-shipment-model]").textContent = option?.dataset.model || "―";
      if (option) row.querySelector("[data-shipment-price]").value = option.dataset.price || "0";
      const codeInput = row.querySelector("[data-shipment-product-code]");
      codeInput.setCustomValidity(option || !codeInput.value.trim() ? "" : "登録済みの商品コードを入力してください。");
      updateShipmentTotal();
    };
    const addShipmentLine = (code = "", shouldFocus = true) => {
      const row = shipmentTemplate.content.firstElementChild.cloneNode(true);
      shipmentLines.appendChild(row);
      if (code) {
        row.querySelector("[data-shipment-product-code]").value = code;
        fillShipmentLine(row);
      } else {
        updateShipmentTotal();
      }
      if (shouldFocus) row.querySelector("[data-shipment-product-code]").focus();
    };
    shipmentForm.querySelector("[data-add-shipment-line]").addEventListener("click", addShipmentLine);
    shipmentLines.addEventListener("input", (event) => {
      const row = event.target.closest("[data-shipment-line]");
      if (event.target.matches("[data-shipment-product-code]")) fillShipmentLine(row);
      if (event.target.matches("[data-shipment-price]")) updateShipmentTotal();
    });
    shipmentLines.addEventListener("change", (event) => {
      if (event.target.matches("[data-shipment-product-code]")) fillShipmentLine(event.target.closest("[data-shipment-line]"));
    });
    shipmentLines.addEventListener("keydown", (event) => {
      if (event.key !== "Enter" || !event.target.matches("[data-shipment-product-code]")) return;
      event.preventDefault();
      fillShipmentLine(event.target.closest("[data-shipment-line]"));
      event.target.reportValidity();
    });
    shipmentLines.addEventListener("click", (event) => {
      const remove = event.target.closest("[data-remove-shipment-line]");
      if (!remove) return;
      remove.closest("[data-shipment-line]").remove();
      if (!shipmentLines.children.length) addShipmentLine();
      updateShipmentTotal();
    });
    shipmentForm.querySelector("[data-shipment-print]").addEventListener("click", () => window.print());
    shipmentForm.querySelector("[data-shipment-customs]").addEventListener("click", () => window.print());
    shipmentForm.querySelector("[data-shipment-csv]").addEventListener("click", () => {
      const rows = [["商品コード", "ブランド", "モデル名", "卸値（税抜）"]];
      shipmentLines.querySelectorAll("[data-shipment-line]").forEach((row) => {
        rows.push([
          row.querySelector("[data-shipment-product-code]").value,
          row.querySelector("[data-shipment-brand]").textContent,
          row.querySelector("[data-shipment-model]").textContent,
          row.querySelector("[data-shipment-price]").value
        ]);
      });
      const csv = rows.map((row) => row.map((cell) => `"${String(cell).replaceAll('"', '""')}"`).join(",")).join("\r\n");
      const link = document.createElement("a");
      link.href = URL.createObjectURL(new Blob(["\ufeff", csv], {type: "text/csv"}));
      link.download = "shipment-slip.csv";
      link.click();
      URL.revokeObjectURL(link.href);
    });
    shipmentForm.addEventListener("submit", (event) => {
      shipmentLines.querySelectorAll("[data-shipment-line]").forEach(fillShipmentLine);
      if (!shipmentForm.checkValidity()) {
        event.preventDefault();
        shipmentForm.reportValidity();
      }
    });
    shipmentForm.addEventListener("reset", () => {
      window.setTimeout(() => {
        shipmentLines.replaceChildren();
        addShipmentLine();
      });
    });
    const shipmentPrefills = [...shipmentForm.querySelectorAll("[data-shipment-prefills] [data-code]")]
      .map((item) => item.dataset.code)
      .filter(Boolean);
    if (shipmentPrefills.length) {
      shipmentPrefills.forEach((code) => addShipmentLine(code, false));
    } else {
      addShipmentLine();
    }
  }

  const slipsPage = document.querySelector("[data-slips-page]");
  if (slipsPage) {
    const checks = [...slipsPage.querySelectorAll("[data-slips-check]")];
    const checkAll = slipsPage.querySelector("[data-slips-check-all]");
    const issueButtons = [...slipsPage.querySelectorAll("[data-slips-issue]")];
    const updateSlipSelection = () => {
      const selected = checks.filter((check) => check.checked).length;
      issueButtons.forEach((issueButton) => { issueButton.disabled = selected === 0; });
      if (checkAll) {
        checkAll.checked = selected > 0 && selected === checks.length;
        checkAll.indeterminate = selected > 0 && selected < checks.length;
      }
    };
    checkAll?.addEventListener("change", () => {
      checks.forEach((check) => { check.checked = checkAll.checked; });
      updateSlipSelection();
    });
    checks.forEach((check) => check.addEventListener("change", updateSlipSelection));
    issueButtons
      .filter((issueButton) => !issueButton.matches("[data-sales-bulk-invoice], [data-purchase-return-bulk-invoice]"))
      .forEach((issueButton) => issueButton.addEventListener("click", () => window.print()));
  slipsPage.querySelectorAll(".slips-invoice").forEach((button) => {
    if (button.dataset.salesReturnOpen || button.dataset.purchaseReturnOpen) return;
    button.addEventListener("click", () => window.alert("請求書の発行準備ができました。"));
  });
  }

  const invoicePreviewDialog = document.querySelector("[data-invoice-preview-dialog]");
  const renderInvoicePreview = (html) => {
    if (!invoicePreviewDialog) return;
    invoicePreviewDialog.innerHTML = html;
    if (!invoicePreviewDialog.open) invoicePreviewDialog.showModal();
    invoicePreviewDialog.querySelector("[data-invoice-preview-close]")?.focus();
  };
  const invoicePrintWindow = () => {
    const sheets = [...(invoicePreviewDialog || document).querySelectorAll("[data-invoice-sheet]")];
    if (sheets.length === 0) return;
    const printWindow = window.open("", "_blank");
    if (!printWindow) {
      window.alert("印刷画面を開けませんでした。ポップアップを許可してください。");
      return;
    }
    printWindow.opener = null;
    printWindow.document.open();
    printWindow.document.write(`<!doctype html><html lang="ja"><head><meta charset="utf-8"><title>請求書</title><link rel="stylesheet" href="/static/app.css"></head><body class="invoice-print-document">${sheets.map((sheet) => sheet.outerHTML).join("")}<script>window.addEventListener("load",()=>window.print())<\/script></body></html>`);
    printWindow.document.close();
  };
  const requestInvoicePreview = async (url, formData, afterOpen) => {
    if (!invoicePreviewDialog) return;
    renderInvoicePreview('<div class="purchase-workflow-loading">請求書プレビューを読み込んでいます…</div>');
    try {
      const response = await fetch(url, {
        method: "POST",
        body: formData,
        headers: {"X-Invoice-Preview": "true"}
      });
      const html = await response.text();
      if (!response.ok) throw new Error(html || `HTTP ${response.status}`);
      renderInvoicePreview(html);
      afterOpen?.();
    } catch (_error) {
      renderInvoicePreview('<div class="purchase-workflow-loading workflow-load-error" role="alert"><strong>請求書プレビューを読み込めませんでした。</strong><button class="button secondary" type="button" data-invoice-preview-close>閉じる</button></div>');
    }
  };
  invoicePreviewDialog?.addEventListener("click", (event) => {
    if (event.target === invoicePreviewDialog || event.target.closest("[data-invoice-preview-close]")) {
      invoicePreviewDialog.close();
      return;
    }
    if (event.target.closest("[data-invoice-print]")) invoicePrintWindow();
  });
  document.querySelector("[data-invoice-print]")?.addEventListener("click", invoicePrintWindow);
  if (slipsPage) {
    const csrf = slipsPage.querySelector("[data-slips-csrf]")?.value || "";
    const selectedInvoiceIDs = () => [...slipsPage.querySelectorAll("[data-slips-check]:checked")].map((item) => item.value);
    const openBulkInvoice = (url) => {
      const data = new FormData();
      data.append("csrf_token", csrf);
      selectedInvoiceIDs().forEach((id) => data.append("id", id));
      requestInvoicePreview(url, data);
    };
    slipsPage.querySelector("[data-sales-bulk-invoice]")?.addEventListener("click", () => {
      openBulkInvoice("/slips/sales/invoices/preview");
    });
    slipsPage.querySelector("[data-purchase-return-bulk-invoice]")?.addEventListener("click", () => {
      openBulkInvoice("/slips/purchase-returns/invoices/preview");
    });
  }

  const purchaseWorkflowDialog = document.querySelector("[data-purchase-workflow-dialog]");
  if (purchaseWorkflowDialog) {
    let purchaseID = "";
    const updateReturnSelection = () => {
      const form = purchaseWorkflowDialog.querySelector("[data-purchase-return-form]");
      if (!form) return;
      const selected = [...form.querySelectorAll('[name="product_id"]:checked')];
      form.querySelector("[data-return-selection-count]").textContent = `${selected.length}点`;
      form.querySelector("[data-purchase-return-submit]").disabled = selected.length === 0;
      form.querySelectorAll("[data-return-product]").forEach((row) => {
        row.classList.toggle("selected", row.querySelector("input").checked);
      });
    };
    const loadPurchaseView = async (view = "detail") => {
      if (!purchaseID) return;
      const suffix = view === "edit" ? "edit" : view === "return" ? "return" : "modal";
      purchaseWorkflowDialog.innerHTML = '<div class="purchase-workflow-loading">仕入伝票を読み込んでいます…</div>';
      try {
        const response = await fetch(`/slips/purchases/${encodeURIComponent(purchaseID)}/${suffix}`, {
          headers: {"HX-Request": "true"}
        });
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        purchaseWorkflowDialog.innerHTML = await response.text();
        purchaseWorkflowDialog.querySelector("[data-purchase-workflow-close]")?.focus();
        updateReturnSelection();
      } catch (_error) {
        purchaseWorkflowDialog.innerHTML = '<div class="purchase-workflow-loading workflow-load-error" role="alert"><strong>仕入伝票を読み込めませんでした。</strong><button class="button secondary" type="button" data-purchase-workflow-close>閉じる</button></div>';
      }
    };
    const openPurchase = (row) => {
      purchaseID = row.dataset.purchaseSlipOpen;
      loadPurchaseView();
      purchaseWorkflowDialog.showModal();
    };
    const addReturnCode = () => {
      const input = purchaseWorkflowDialog.querySelector("[data-return-code-input]");
      const code = input?.value.trim().toLowerCase();
      if (!code) {
        input?.focus();
        return;
      }
      const product = [...purchaseWorkflowDialog.querySelectorAll("[data-return-product]")]
        .find((row) => row.dataset.productCode.toLowerCase() === code);
      if (!product) {
        input.setCustomValidity("この仕入伝票に該当する商品コードがありません。");
        input.reportValidity();
        return;
      }
      input.setCustomValidity("");
      product.querySelector("input").checked = true;
      product.scrollIntoView({block: "nearest"});
      input.value = "";
      updateReturnSelection();
      input.focus();
    };
    document.querySelectorAll("[data-purchase-slip-open]").forEach((row) => {
      row.addEventListener("click", (event) => {
        if (event.target.closest("a, button, input, select, textarea, form")) return;
        openPurchase(row);
      });
      row.addEventListener("keydown", (event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          openPurchase(row);
        }
      });
    });
    purchaseWorkflowDialog.addEventListener("click", (event) => {
      if (event.target === purchaseWorkflowDialog || event.target.closest("[data-purchase-workflow-close]")) {
        purchaseWorkflowDialog.close();
        return;
      }
      const viewButton = event.target.closest("[data-purchase-workflow-view]");
      if (viewButton) {
        loadPurchaseView(viewButton.dataset.purchaseWorkflowView);
        return;
      }
      if (event.target.closest("[data-return-code-add]")) addReturnCode();
      if (event.target.closest("[data-return-scan]")) {
        purchaseWorkflowDialog.querySelector("[data-return-code-input]")?.focus();
      }
    });
    purchaseWorkflowDialog.addEventListener("change", (event) => {
      if (event.target.matches('[name="product_id"]')) updateReturnSelection();
    });
    purchaseWorkflowDialog.addEventListener("keydown", (event) => {
      if (event.target.matches("[data-return-code-input]") && event.key === "Enter") {
        event.preventDefault();
        addReturnCode();
      }
    });
  }

  const shipmentWorkflowDialog = document.querySelector("[data-shipment-workflow-dialog]");
  if (shipmentWorkflowDialog) {
    let shipmentID = "";
    const loadShipmentView = async (view = "detail") => {
      if (!shipmentID) return;
      const suffix = view === "edit" ? "edit" : "modal";
      shipmentWorkflowDialog.innerHTML = '<div class="purchase-workflow-loading">出荷伝票を読み込んでいます…</div>';
      try {
        const response = await fetch(`/slips/shipments/${encodeURIComponent(shipmentID)}/${suffix}`, {
          headers: {"HX-Request": "true"}
        });
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        shipmentWorkflowDialog.innerHTML = await response.text();
        shipmentWorkflowDialog.querySelector("[data-shipment-workflow-close]")?.focus();
      } catch (_error) {
        shipmentWorkflowDialog.innerHTML = '<div class="purchase-workflow-loading workflow-load-error" role="alert"><strong>出荷伝票を読み込めませんでした。</strong><button class="button secondary" type="button" data-shipment-workflow-close>閉じる</button></div>';
      }
    };
    const openShipment = (row) => {
      shipmentID = row.dataset.shipmentSlipOpen;
      loadShipmentView();
      shipmentWorkflowDialog.showModal();
    };
    document.querySelectorAll("[data-shipment-slip-open]").forEach((row) => {
      row.addEventListener("click", (event) => {
        if (event.target.closest("a, button, input, select, textarea, form")) return;
        openShipment(row);
      });
      row.addEventListener("keydown", (event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          openShipment(row);
        }
      });
    });
    shipmentWorkflowDialog.addEventListener("click", (event) => {
      if (event.target === shipmentWorkflowDialog || event.target.closest("[data-shipment-workflow-close]")) {
        shipmentWorkflowDialog.close();
        return;
      }
      const viewButton = event.target.closest("[data-shipment-workflow-view]");
      if (viewButton) loadShipmentView(viewButton.dataset.shipmentWorkflowView);
    });
  }

  const salesWorkflowDialog = document.querySelector("[data-sales-workflow-dialog]");
  if (salesWorkflowDialog) {
    let salesID = "";
    const updateSalesReturnSelection = () => {
      const form = salesWorkflowDialog.querySelector("[data-sales-return-form]");
      if (!form) return;
      const selected = [...form.querySelectorAll('[name="sales_line_id"]:checked')];
      form.querySelector("[data-sales-return-selection-count]").textContent = `${selected.length}点`;
      form.querySelector("[data-sales-return-submit]").disabled = selected.length === 0;
      form.querySelectorAll("[data-sales-return-product]").forEach((row) => {
        row.classList.toggle("selected", row.querySelector("input").checked);
      });
    };
    const loadSalesView = async (view = "detail") => {
      if (!salesID) return;
      const suffix = view === "edit" ? "edit" : view === "return" ? "return" : "modal";
      salesWorkflowDialog.innerHTML = '<div class="purchase-workflow-loading">売上伝票を読み込んでいます…</div>';
      try {
        const response = await fetch(`/slips/sales/${encodeURIComponent(salesID)}/${suffix}`, {
          headers: {"HX-Request": "true"}
        });
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        salesWorkflowDialog.innerHTML = await response.text();
        salesWorkflowDialog.querySelector("[data-sales-workflow-close]")?.focus();
        updateSalesReturnSelection();
      } catch (_error) {
        salesWorkflowDialog.innerHTML = '<div class="purchase-workflow-loading workflow-load-error" role="alert"><strong>売上伝票を読み込めませんでした。</strong><button class="button secondary" type="button" data-sales-workflow-close>閉じる</button></div>';
      }
    };
    const openSales = (row) => {
      salesID = row.dataset.salesSlipOpen;
      loadSalesView();
      salesWorkflowDialog.showModal();
    };
    const addSalesReturnCode = () => {
      const input = salesWorkflowDialog.querySelector("[data-sales-return-code-input]");
      const code = input?.value.trim().toLowerCase();
      if (!code) {
        input?.focus();
        return;
      }
      const product = [...salesWorkflowDialog.querySelectorAll("[data-sales-return-product]")]
        .find((row) => row.dataset.productCode.toLowerCase() === code);
      if (!product) {
        input.setCustomValidity("この売上伝票に該当する商品コードがありません。");
        input.reportValidity();
        return;
      }
      input.setCustomValidity("");
      product.querySelector("input").checked = true;
      product.scrollIntoView({block: "nearest"});
      input.value = "";
      updateSalesReturnSelection();
      input.focus();
    };
    document.querySelectorAll("[data-sales-slip-open]").forEach((row) => {
      row.addEventListener("click", (event) => {
        if (event.target.closest("a, button, input, select, textarea, form")) return;
        openSales(row);
      });
      row.addEventListener("keydown", (event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          openSales(row);
        }
      });
    });
    salesWorkflowDialog.addEventListener("click", (event) => {
      if (event.target === salesWorkflowDialog || event.target.closest("[data-sales-workflow-close]")) {
        salesWorkflowDialog.close();
        return;
      }
      const viewButton = event.target.closest("[data-sales-workflow-view]");
      if (viewButton) {
        loadSalesView(viewButton.dataset.salesWorkflowView);
        return;
      }
      if (event.target.closest("[data-sales-return-code-add]")) addSalesReturnCode();
      if (event.target.closest("[data-sales-return-scan]")) {
        salesWorkflowDialog.querySelector("[data-sales-return-code-input]")?.focus();
      }
    });
    salesWorkflowDialog.addEventListener("change", (event) => {
      if (event.target.matches('[name="sales_line_id"]')) updateSalesReturnSelection();
    });
    salesWorkflowDialog.addEventListener("keydown", (event) => {
      if (event.target.matches("[data-sales-return-code-input]") && event.key === "Enter") {
        event.preventDefault();
        addSalesReturnCode();
      }
    });
  }

  const salesReturnWorkflowDialog = document.querySelector("[data-sales-return-workflow-dialog]");
  if (salesReturnWorkflowDialog) {
    let salesReturnID = "";
    const loadSalesReturnView = async () => {
      if (!salesReturnID) return;
      salesReturnWorkflowDialog.innerHTML = '<div class="purchase-workflow-loading">売上返品伝票を読み込んでいます…</div>';
      try {
        const response = await fetch(`/slips/sales-returns/${encodeURIComponent(salesReturnID)}/modal`, {
          headers: {"HX-Request": "true"}
        });
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        salesReturnWorkflowDialog.innerHTML = await response.text();
        salesReturnWorkflowDialog.querySelector("[data-sales-return-workflow-close]")?.focus();
      } catch (_error) {
        salesReturnWorkflowDialog.innerHTML = '<div class="purchase-workflow-loading workflow-load-error" role="alert"><strong>売上返品伝票を読み込めませんでした。</strong><button class="button secondary" type="button" data-sales-return-workflow-close>閉じる</button></div>';
      }
    };
    const openSalesReturn = (id) => {
      salesReturnID = id;
      loadSalesReturnView();
      salesReturnWorkflowDialog.showModal();
    };
    document.querySelectorAll("[data-sales-return-slip-open]").forEach((row) => {
      row.addEventListener("click", (event) => {
        if (event.target.closest("a, button, input, select, textarea, form")) return;
        openSalesReturn(row.dataset.salesReturnSlipOpen);
      });
      row.addEventListener("keydown", (event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          openSalesReturn(row.dataset.salesReturnSlipOpen);
        }
      });
    });
    document.querySelectorAll("[data-sales-return-open]").forEach((button) => {
      button.addEventListener("click", () => openSalesReturn(button.dataset.salesReturnOpen));
    });
    salesReturnWorkflowDialog.addEventListener("click", (event) => {
      if (event.target === salesReturnWorkflowDialog || event.target.closest("[data-sales-return-workflow-close]")) {
        salesReturnWorkflowDialog.close();
      }
    });
    salesReturnWorkflowDialog.addEventListener("submit", (event) => {
      if (!event.target.matches("[data-sales-return-invoice-form]")) return;
      event.preventDefault();
      const form = event.target;
      requestInvoicePreview(form.action, new FormData(form), loadSalesReturnView);
    });
  }

  document.querySelector("[data-sales-return-print]")?.addEventListener("click", invoicePrintWindow);

  const purchaseReturnWorkflowDialog = document.querySelector("[data-purchase-return-workflow-dialog]");
  if (purchaseReturnWorkflowDialog) {
    let purchaseReturnID = "";
    const loadPurchaseReturnView = async () => {
      if (!purchaseReturnID) return;
      purchaseReturnWorkflowDialog.innerHTML = '<div class="purchase-workflow-loading">仕入返品伝票を読み込んでいます…</div>';
      try {
        const response = await fetch(`/slips/purchase-returns/${encodeURIComponent(purchaseReturnID)}/modal`, {
          headers: {"HX-Request": "true"}
        });
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        purchaseReturnWorkflowDialog.innerHTML = await response.text();
        purchaseReturnWorkflowDialog.querySelector("[data-purchase-return-workflow-close]")?.focus();
      } catch (_error) {
        purchaseReturnWorkflowDialog.innerHTML = '<div class="purchase-workflow-loading workflow-load-error" role="alert"><strong>仕入返品伝票を読み込めませんでした。</strong><button class="button secondary" type="button" data-purchase-return-workflow-close>閉じる</button></div>';
      }
    };
    const openPurchaseReturn = (id) => {
      purchaseReturnID = id;
      loadPurchaseReturnView();
      purchaseReturnWorkflowDialog.showModal();
    };
    document.querySelectorAll("[data-purchase-return-slip-open]").forEach((row) => {
      row.addEventListener("click", (event) => {
        if (event.target.closest("a, button, input, select, textarea, form")) return;
        openPurchaseReturn(row.dataset.purchaseReturnSlipOpen);
      });
      row.addEventListener("keydown", (event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          openPurchaseReturn(row.dataset.purchaseReturnSlipOpen);
        }
      });
    });
    document.querySelectorAll("[data-purchase-return-open]").forEach((button) => {
      button.addEventListener("click", () => openPurchaseReturn(button.dataset.purchaseReturnOpen));
    });
    purchaseReturnWorkflowDialog.addEventListener("click", (event) => {
      if (event.target === purchaseReturnWorkflowDialog || event.target.closest("[data-purchase-return-workflow-close]")) {
        purchaseReturnWorkflowDialog.close();
      }
    });
    purchaseReturnWorkflowDialog.addEventListener("submit", (event) => {
      if (!event.target.matches("[data-purchase-return-invoice-form]")) return;
      event.preventDefault();
      const form = event.target;
      requestInvoicePreview(form.action, new FormData(form), loadPurchaseReturnView);
    });
  }

  document.querySelector("[data-purchase-return-print]")?.addEventListener("click", invoicePrintWindow);

  const returnsFilter = document.querySelector("[data-returns-filter]");
  if (returnsFilter) {
    let returnsFilterTimer;
    returnsFilter.querySelector('select[name="status"]')?.addEventListener("change", () => returnsFilter.requestSubmit());
    returnsFilter.querySelector('input[name="q"]')?.addEventListener("input", () => {
      clearTimeout(returnsFilterTimer);
      returnsFilterTimer = setTimeout(() => returnsFilter.requestSubmit(), 250);
    });
  }

  const returnRestoreDialog = document.querySelector("[data-return-restore-dialog]");
  if (returnRestoreDialog) {
    let returnSaleID = "";
    const updateReturnRestoreSelection = () => {
      const form = returnRestoreDialog.querySelector("[data-return-restore-form]");
      if (!form) return;
      const selected = [...form.querySelectorAll('[name="item_id"]:checked')];
      let ready = selected.length > 0;
      form.querySelectorAll("[data-return-restore-product]").forEach((product) => {
        const checked = product.querySelector('[name="item_id"]')?.checked;
        product.classList.toggle("selected", Boolean(checked));
        const condition = product.querySelector('select[name^="condition_"]');
        const quantity = product.querySelector('input[name^="quantity_"]');
        if (condition) condition.required = false;
        if (quantity) quantity.required = Boolean(checked);
        if (checked && (!quantity?.value || Number(quantity.value) < 1)) ready = false;
      });
      const submit = form.querySelector("[data-return-restore-submit]");
      if (submit) {
        submit.disabled = false;
        submit.setAttribute("aria-disabled", ready ? "false" : "true");
      }
    };
    const loadReturnRestore = async () => {
      if (!returnSaleID) return;
      returnRestoreDialog.innerHTML = '<div class="purchase-workflow-loading">返品/持ち帰り伝票を読み込んでいます…</div>';
      try {
        const response = await fetch(`/returns/${encodeURIComponent(returnSaleID)}/modal`, {
          headers: {"HX-Request": "true"}
        });
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        returnRestoreDialog.innerHTML = await response.text();
        updateReturnRestoreSelection();
        returnRestoreDialog.querySelector("[data-return-restore-close]")?.focus();
      } catch (_error) {
        returnRestoreDialog.innerHTML = '<div class="purchase-workflow-loading workflow-load-error" role="alert"><strong>返品/持ち帰り伝票を読み込めませんでした。</strong><button class="button secondary" type="button" data-return-restore-close>閉じる</button></div>';
      }
    };
    const openReturnRestore = (id) => {
      returnSaleID = id;
      loadReturnRestore();
      returnRestoreDialog.showModal();
    };
    document.querySelectorAll("[data-return-restore-open]").forEach((row) => {
      row.addEventListener("click", (event) => {
        if (event.target.closest("a, button, input, select, textarea, form")) return;
        openReturnRestore(row.dataset.returnRestoreOpen);
      });
      row.addEventListener("keydown", (event) => {
        if ((event.key === "Enter" || event.key === " ") && event.target === row) {
          event.preventDefault();
          openReturnRestore(row.dataset.returnRestoreOpen);
        }
      });
    });
    document.querySelectorAll("[data-return-restore-button]").forEach((button) => {
      button.addEventListener("click", () => openReturnRestore(button.dataset.returnRestoreButton));
    });
    returnRestoreDialog.addEventListener("click", (event) => {
      if (event.target === returnRestoreDialog || event.target.closest("[data-return-restore-close]")) {
        returnRestoreDialog.close();
      }
    });
    returnRestoreDialog.addEventListener("change", (event) => {
      if (event.target.matches('[name="item_id"], select[name^="condition_"], input[name^="quantity_"]')) {
        updateReturnRestoreSelection();
      }
    });
    returnRestoreDialog.addEventListener("input", (event) => {
      if (event.target.matches('input[name^="quantity_"]')) updateReturnRestoreSelection();
    });
    returnRestoreDialog.addEventListener("submit", (event) => {
      if (!event.target.matches("[data-return-restore-form]")) return;
      updateReturnRestoreSelection();
      const selected = [...event.target.querySelectorAll('[name="item_id"]:checked')];
      if (selected.length === 0) {
        event.preventDefault();
        event.stopImmediatePropagation();
        window.alert("在庫に戻す商品を選択してください");
        return;
      }
      if (!event.target.checkValidity()) {
        event.preventDefault();
        event.stopImmediatePropagation();
        event.target.reportValidity();
        return;
      }
      const isAdmin = event.target.dataset.returnRestoreAdmin === "true";
      const confirmed = event.target.querySelector("[data-return-restore-confirmed]");
      if (isAdmin && confirmed?.value !== "1") {
        event.preventDefault();
        event.stopImmediatePropagation();
        const confirmDialog = returnRestoreDialog.querySelector("[data-return-box-confirm-dialog]");
        const itemHost = confirmDialog?.querySelector("[data-return-box-confirm-items]");
        if (!confirmDialog || !itemHost) return;
        itemHost.replaceChildren();
        selected.forEach((checkbox) => {
          const product = checkbox.closest("[data-return-restore-product]");
          const row = document.createElement("label");
          row.className = "return-box-confirm-row";
          const copy = document.createElement("span");
          copy.textContent = product?.querySelector(".return-restore-product-title strong")?.textContent?.trim() || checkbox.value;
          const select = document.createElement("select");
          select.name = `box_${checkbox.value}`;
          select.setAttribute("form", "return-restore-form");
          [
            ["", "振り分けなし"],
            ["BOX1", "BOX1 — ロレックス特集"],
            ["BOX2", "BOX2 — 高額品セレクト"],
            ["BOX3", "BOX3 — 春の新入荷"],
            ["BOX4", "BOX4"], ["BOX5", "BOX5"], ["BOX6", "BOX6"], ["BOX7", "BOX7"],
            ["BOX8", "BOX8"], ["BOX9", "BOX9"], ["BOX10", "BOX10"]
          ].forEach(([value, label]) => select.add(new Option(label, value)));
          row.append(copy, select);
          itemHost.append(row);
        });
        confirmDialog.showModal();
        return;
      }
      event.target.querySelector("[data-return-restore-submit]")?.setAttribute("aria-busy", "true");
    });
    returnRestoreDialog.addEventListener("click", (event) => {
      const confirmDialog = returnRestoreDialog.querySelector("[data-return-box-confirm-dialog]");
      if (event.target.closest("[data-return-box-confirm-close]")) {
        confirmDialog?.close();
      }
      if (event.target.closest("[data-return-box-confirm-submit]")) {
        const form = returnRestoreDialog.querySelector("[data-return-restore-form]");
        const confirmed = form?.querySelector("[data-return-restore-confirmed]");
        if (confirmed) confirmed.value = "1";
        confirmDialog?.close();
        form?.requestSubmit();
      }
    });
  }

  document.querySelectorAll("[data-purchase-request-open]").forEach((card) => {
    const open = () => {
      document.getElementById(card.dataset.purchaseRequestOpen)?.showModal();
    };
    card.addEventListener("click", open);
    card.addEventListener("keydown", (event) => {
      if (event.key === "Enter" || event.key === " ") {
        event.preventDefault();
        open();
      }
    });
  });

  document.querySelectorAll("[data-purchase-request-dialog]").forEach((dialog) => {
    dialog.querySelectorAll("[data-purchase-request-close]").forEach((button) => {
      button.addEventListener("click", () => dialog.close());
    });
    dialog.addEventListener("click", (event) => {
      if (event.target === dialog) dialog.close();
    });
  });

  const marketDialog = document.querySelector("[data-market-dialog]");
  if (marketDialog) {
    const openMarketDetail = async (productID) => {
      marketDialog.innerHTML = '<div class="product-modal-loading">相場情報を読み込んでいます…</div>';
      marketDialog.showModal();
      try {
        const response = await fetch(`/market-prices/products/${encodeURIComponent(productID)}/modal`, {
          headers: {"HX-Request": "true"}
        });
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        marketDialog.innerHTML = await response.text();
        marketDialog.querySelector("[data-market-close]")?.focus();
      } catch (_error) {
        marketDialog.innerHTML = '<div class="product-modal-error" role="alert"><strong>相場情報を読み込めませんでした。</strong><button class="button secondary" type="button" data-market-close>閉じる</button></div>';
      }
    };
    document.querySelectorAll("[data-market-row]").forEach((row) => {
      row.addEventListener("click", (event) => {
        if (event.target.closest("a, button, input, select, textarea, form")) return;
        openMarketDetail(row.dataset.marketRow);
      });
      row.addEventListener("keydown", (event) => {
        if ((event.key === "Enter" || event.key === " ") && event.target === row) {
          event.preventDefault();
          openMarketDetail(row.dataset.marketRow);
        }
      });
    });
    marketDialog.addEventListener("click", (event) => {
      if (event.target === marketDialog || event.target.closest("[data-market-close]")) {
        marketDialog.close();
      }
    });
  }

  document.querySelector("[data-market-csv-file]")?.addEventListener("change", (event) => {
    if (event.target.files?.length) event.target.form?.requestSubmit();
  });

  document.querySelectorAll("[data-approval-open]").forEach((row) => {
    const open = () => document.getElementById(row.dataset.approvalOpen)?.showModal();
    row.addEventListener("click", (event) => {
      if (!event.target.closest("a,button,input,textarea,form")) open();
    });
    row.addEventListener("keydown", (event) => {
      if ((event.key === "Enter" || event.key === " ") && event.target === row) {
        event.preventDefault();
        open();
      }
    });
  });
  document.querySelectorAll("[data-approval-dialog],[data-approval-return-dialog]").forEach((dialog) => {
    dialog.querySelectorAll("[data-approval-close]").forEach((button) => {
      button.addEventListener("click", () => dialog.close());
    });
    dialog.addEventListener("click", (event) => {
      if (event.target === dialog) dialog.close();
    });
  });
  document.querySelectorAll("[data-approval-return-open]").forEach((button) => {
    button.addEventListener("click", () => {
      button.closest("[data-approval-dialog]")?.close();
      document.getElementById(button.dataset.approvalReturnOpen)?.showModal();
    });
  });

  document.querySelectorAll("form").forEach((form) => {
    form.addEventListener("submit", (event) => {
      if (form.dataset.confirm && !window.confirm(form.dataset.confirm)) {
        event.preventDefault();
        return;
      }
      if (!form.checkValidity()) return;
      form.querySelectorAll("button[type=submit]").forEach((button) => {
        button.disabled = true;
        button.setAttribute("aria-busy", "true");
      });
    });
  });
});
