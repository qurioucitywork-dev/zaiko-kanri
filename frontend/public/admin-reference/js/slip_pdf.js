// Generate PDFs locally from the same HTML used by the document preview.
let _slipPdfLibraries;
let _slipPdfDownloading = false;

async function loadSlipPdfLibraries() {
  if (window.jspdf?.jsPDF && window.html2canvas) return;
  if (!_slipPdfLibraries) {
    _slipPdfLibraries = Promise.all(['jspdf.umd.min.js', 'html2canvas.min.js'].map(file => new Promise((resolve, reject) => {
      const script = document.createElement('script');
      script.src = new URL(`js/vendor/${file}`, document.baseURI).href;
      script.onload = resolve;
      script.onerror = () => { script.remove(); reject(new Error('PDF作成機能を読み込めません。再読み込みしてお試しください。')); };
      document.head.appendChild(script);
    }))).catch(error => { _slipPdfLibraries = null; throw error; });
  }
  await _slipPdfLibraries;
}

async function buildSlipPdfBlob(title, documentHtml) {
  await loadSlipPdfLibraries();
  const frame = document.createElement('iframe');
  frame.title = 'PDF作成用';
  frame.setAttribute('aria-hidden', 'true');
  frame.tabIndex = -1;
  frame.style.cssText = 'position:fixed;left:-20000px;top:0;width:794px;height:1123px;border:0;pointer-events:none';
  document.body.appendChild(frame);
  try {
    const doc = frame.contentDocument;
    // The generated document contains escaped product data; never execute scripts in it.
    const source = new DOMParser().parseFromString(documentHtml, 'text/html');
    source.querySelectorAll('script,iframe,object,embed').forEach(el => el.remove());
    source.querySelectorAll('*').forEach(el => Array.from(el.attributes).forEach(attr => {
      if (/^on/i.test(attr.name)) el.removeAttribute(attr.name);
    }));
    doc.open();
    doc.write('<!doctype html><html lang="ja"><head><meta charset="utf-8"><style>html,body{margin:0;background:white}</style></head><body>' + source.body.innerHTML + '</body></html>');
    doc.close();
    // DOMParser moves leading styles into head for a full HTML document.
    source.head.querySelectorAll('style').forEach(style => doc.head.appendChild(doc.importNode(style, true)));
    const printRules = [];
    for (const sheet of Array.from(doc.styleSheets)) {
      for (const rule of Array.from(sheet.cssRules)) {
        if (rule.type === 4 && /\bprint\b/.test(rule.conditionText)) {
          for (const child of Array.from(rule.cssRules)) if (child.type !== 6) printRules.push(child.cssText);
        }
      }
    }
    const style = doc.createElement('style');
    style.textContent = printRules.join('\n') + '\n.tpl-doc-page{box-shadow:none!important;margin:0!important;overflow:visible!important}.tpl-page-divider{display:none!important}';
    doc.head.appendChild(style);
    await doc.fonts.ready;
    await Promise.all(Array.from(doc.images).map(img => img.decode().catch(() => { throw new Error('帳票画像を読み込めませんでした。'); })));
    const pages = Array.from(doc.querySelectorAll('.tpl-doc-page'));
    if (!pages.length) throw new Error('PDFに保存できる帳票がありません。');
    const pdf = new window.jspdf.jsPDF({orientation:'portrait', unit:'mm', format:'a4', compress:true});
    pdf.setProperties({title, creator:'在庫管理ツール'});
    let pageCount = 0;
    for (const page of pages) {
      // Render one sheet at a time so large documents do not exceed canvas limits.
      const bounds = page.getBoundingClientRect();
      const pageHeight = bounds.width * 297 / 210;
      const textRects = [];
      const walker = doc.createTreeWalker(page, 4);
      while (walker.nextNode()) {
        if (!walker.currentNode.textContent.trim()) continue;
        const range = doc.createRange(); range.selectNodeContents(walker.currentNode);
        for (const rect of range.getClientRects()) textRects.push({top:rect.top-bounds.top,bottom:rect.bottom-bounds.top});
      }
      let offset = 0;
      const height = Math.max(bounds.height, page.scrollHeight);
      while (offset < height - 1) {
        let end = Math.min(height, offset + pageHeight);
        if (end < height - 1) {
          const crossing = textRects.filter(rect => rect.top < end && rect.bottom > end && rect.top > offset + 1);
          if (crossing.length) end = Math.min(...crossing.map(rect => rect.top));
        }
        // Native SVG text layout preserves the compact three-line Japanese detail cells.
        // Cancel the document position and shift the cloned sheet for continuation pages.
        const canvas = await window.html2canvas(page, {
          scale:3, backgroundColor:'#ffffff', logging:false, useCORS:true, foreignObjectRendering:true,
          windowWidth:794, windowHeight:1123, scrollX:0, scrollY:0,
          x:-bounds.left, y:offset-bounds.top, width:bounds.width, height:end-offset,
          onclone: (_doc, clonedPage) => { clonedPage.style.transform = `translateY(-${offset}px)`; },
        });
        if (pageCount++) pdf.addPage('a4','portrait');
        pdf.addImage(canvas, 'PNG', 0, 0, 210, (end-offset) * 210 / bounds.width, undefined, 'FAST');
        canvas.width = canvas.height = 0;
        offset = end;
      }
    }
    return pdf.output('blob');
  } finally {
    frame.remove();
  }
}

async function downloadSlipPdf(title, filename, documentHtml) {
  if (_slipPdfDownloading) return false;
  _slipPdfDownloading = true;
  try {
    showToast('info', 'PDF作成中', '帳票をPDFにしています。しばらくお待ちください。');
    const blob = await buildSlipPdfBlob(title, documentHtml);
    const safeName = String(filename || title).replace(/\.(?:html?|pdf)$/i, '').replace(/[\\/:*?"<>|\x00-\x1f]/g, '_') + '.pdf';
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url; link.download = safeName;
    document.body.appendChild(link); link.click(); link.remove();
    setTimeout(() => URL.revokeObjectURL(url), 60000);
    showToast('success', 'PDFをダウンロードしました', safeName);
    return true;
  } catch (error) {
    showToast('error', 'PDFを作成できませんでした', error?.message || '再度お試しください。');
    return false;
  } finally {
    _slipPdfDownloading = false;
  }
}
