document.addEventListener("DOMContentLoaded", () => {
  const menuButton = document.querySelector("[data-menu-button]");
  if (menuButton) {
    menuButton.addEventListener("click", () => {
      const open = document.body.classList.toggle("menu-open");
      menuButton.setAttribute("aria-expanded", String(open));
      menuButton.setAttribute("aria-label", open ? "メニューを閉じる" : "メニューを開く");
    });
  }

  const purchaseLines = document.querySelector("[data-purchase-lines]");
  const addLine = document.querySelector("[data-add-purchase-line]");
  if (purchaseLines && addLine) {
    addLine.addEventListener("click", () => {
      const source = purchaseLines.querySelector("[data-purchase-line]");
      const clone = source.cloneNode(true);
      clone.querySelectorAll("input").forEach((input) => {
        if (input.name === "quantity") input.value = "1";
        else if (input.name === "unit_cost") input.value = "0";
        else if (input.name === "product_type") input.value = "腕時計";
        else input.value = "";
      });
      purchaseLines.appendChild(clone);
      clone.querySelector("input, select")?.focus();
    });
    purchaseLines.addEventListener("click", (event) => {
      const removeButton = event.target.closest("[data-remove-purchase-line]");
      if (!removeButton) return;
      const rows = purchaseLines.querySelectorAll("[data-purchase-line]");
      if (rows.length > 1) removeButton.closest("[data-purchase-line]").remove();
    });
  }

  const liveForm = document.querySelector("[data-live-filter]");
  let filterTimer;
  const showPartialError = (message) => {
    const results = document.querySelector("#product-results");
    if (!results) return;
    results.setAttribute("aria-busy", "false");
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

  document.querySelectorAll("form").forEach((form) => {
    form.addEventListener("submit", () => {
      if (!form.checkValidity()) return;
      form.querySelectorAll("button[type=submit]").forEach((button) => {
        button.disabled = true;
        button.setAttribute("aria-busy", "true");
      });
    });
  });
});
