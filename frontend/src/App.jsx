import { useEffect, useRef, useState } from "react";

const REFERENCE_ROOT = "/app/admin-reference/";

function referenceDocument(pathname) {
  const leaf = pathname.replace(/\/+$/u, "").split("/").at(-1) || "";
  if (leaf === "guest" || leaf === "guest.html") return "guest.html";
  if (leaf === "login" || leaf === "index.html") return "index.html";
  return "app.html";
}

function absoluteReferenceURL(value, documentName) {
  return new URL(value, `${window.location.origin}${REFERENCE_ROOT}${documentName}`).href;
}

function copyReferenceHead(source, documentName) {
  const copied = [];
  source.head.querySelectorAll('link[rel="stylesheet"], style').forEach((node) => {
    const clone = node.cloneNode(true);
    clone.dataset.referenceUi = "true";
    if (clone.tagName === "LINK") clone.href = absoluteReferenceURL(node.getAttribute("href"), documentName);
    document.head.appendChild(clone);
    copied.push(clone);
  });
  return copied;
}

function executeReferenceScript(source, documentName) {
  return new Promise((resolve) => {
    const script = document.createElement("script");
    script.dataset.referenceUi = "true";
    const src = source.getAttribute("src");
    if (src) {
      if (src.includes("genspark.ai/sandbox_inspect")) {
        resolve(null);
        return;
      }
      script.src = absoluteReferenceURL(src, documentName);
      script.onload = () => resolve(script);
      script.onerror = () => resolve(script);
    } else {
      script.textContent = source.textContent;
    }
    document.body.appendChild(script);
    if (!src) resolve(script);
  });
}

export default function App() {
  const mountRef = useRef(null);
  const [error, setError] = useState("");

  useEffect(() => {
    let active = true;
    const copiedHead = [];
    const copiedScripts = [];
    const documentName = referenceDocument(window.location.pathname);

    async function mountReferenceUI() {
      try {
        const response = await fetch(`${REFERENCE_ROOT}${documentName}`, { credentials: "same-origin" });
        if (!response.ok) throw new Error(`基準画面を読み込めませんでした (${response.status})`);
        const source = new DOMParser().parseFromString(await response.text(), "text/html");
        if (!active || !mountRef.current) return;

        document.title = source.title || "在庫管理ツール";
        copiedHead.push(...copyReferenceHead(source, documentName));
        const scripts = [...source.body.querySelectorAll("script")];
        scripts.forEach((script) => script.remove());
        mountRef.current.innerHTML = source.body.innerHTML;

        for (const sourceScript of scripts) {
          if (!active) return;
          const loaded = await executeReferenceScript(sourceScript, documentName);
          if (loaded) copiedScripts.push(loaded);
        }
        if (!active) return;
        document.dispatchEvent(new Event("DOMContentLoaded", { bubbles: true }));
      } catch (reason) {
        if (active) setError(reason instanceof Error ? reason.message : "画面の読み込みに失敗しました");
      }
    }

    mountReferenceUI();
    return () => {
      active = false;
      copiedHead.forEach((node) => node.remove());
      copiedScripts.forEach((node) => node.remove());
      if (mountRef.current) mountRef.current.replaceChildren();
      document.body.style.overflow = "";
    };
  }, []);

  if (error) {
    return (
      <main role="alert" style={{ maxWidth: 720, margin: "80px auto", padding: 24, fontFamily: "sans-serif" }}>
        <h1 style={{ fontSize: 24 }}>在庫管理ツールを表示できません</h1>
        <p>{error}</p>
        <button type="button" onClick={() => window.location.reload()}>再読み込み</button>
      </main>
    );
  }
  return <div ref={mountRef} />;
}
