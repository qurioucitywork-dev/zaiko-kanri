const fs = require('fs');
const path = require('path');
const crypto = require('crypto');
const vm = require('vm');
const sharp = require('sharp');

const qrSource = fs.readFileSync(path.resolve(__dirname, '..', 'frontend', 'public', 'admin-reference', 'js', 'qrcode-generator.js'), 'utf8');
const qrSandbox = {module: {exports: {}}, exports: {}};
vm.runInNewContext(qrSource, qrSandbox);
const qrcode = qrSandbox.module.exports;

const workspace = path.resolve(__dirname, '..');
const generated = 'C:\\Users\\ftmak\\.codex\\generated_images\\01a00c6e-f8f9-7570-986c-6bf0ad1c05f8';
const referenceRolex = 'C:\\Users\\ftmak\\AppData\\Local\\Temp\\codex-clipboard-f17ca0ac-699d-4664-aeb3-feffbaa2f645.png';
const uploadRoot = path.join(workspace, '.data', 'uploads');
const stagingRoot = path.join(workspace, '.data', 'image-staging', 'PI-2026-0032');
const backupRoot = path.join(workspace, '.data', 'image-backups', '20260818-PI-2026-0032');

const products = [
  {
    code: '20260818001', source: referenceRolex,
    target: 'organizations/org_preview/products/prd_c5ab304a57e4a227167c01e6d98f0916/fil_dba435a2a6094ac6b6309f20134b991d.png',
    fileID: 'fil_dba435a2a6094ac6b6309f20134b991d', exactReference: true,
  },
  {
    code: '20260818002', source: path.join(generated, 'exec-52c360a3-1dc5-4ff2-b886-5f51b444fa50.png'),
    target: 'organizations/org_preview/products/prd_a261cd5d4c272933cf317b3671d1037e/fil_02c216bee61df4dcd3690939cd78ecd0.png', fileID: 'fil_02c216bee61df4dcd3690939cd78ecd0',
    brand: 'オメガ', model: 'Speedmaster', reference: '310.30.42.50.01.001', serial: 'CUSTOMS-SERIAL-002', material: 'イエローゴールドYG', belt: 'レザー', movement: '手巻き', marking: '☆', note: 'Customs sample 2; full tag fields; Origin Japan',
  },
  {
    code: '20260818003', source: path.join(generated, 'exec-ed06aefc-04e3-409c-9cb4-b8c7f4e46c9a.png'),
    target: 'organizations/org_preview/products/prd_b99203eb0d3d81e6a455c1f778df9bd8/fil_7699453df9bba331060e1a525663f355.png', fileID: 'fil_7699453df9bba331060e1a525663f355',
    brand: 'パテック・フィリップ', model: 'Aquanaut', reference: '5167A-001', serial: 'CUSTOMS-SERIAL-003', material: 'ホワイトゴールドWG', belt: 'ラバー', movement: 'クオーツ', marking: '♧', note: 'Customs sample 3; full tag fields; Origin Japan',
  },
  {
    code: '20260818004', source: path.join(generated, 'exec-4c7a230a-3628-4f44-be47-318a333ed769.png'),
    target: 'organizations/org_preview/products/prd_79c822a63c5f44c29d6ced79801e73b7/fil_7538b6547a2fae2b8e6e7a87745918f5.png', fileID: 'fil_7538b6547a2fae2b8e6e7a87745918f5',
    brand: 'カルティエ', model: 'Santos', reference: 'WSSA0018', serial: 'CUSTOMS-SERIAL-004', material: 'ピンクゴールドPG', belt: 'チタン', movement: '電波', marking: '♤', note: 'Customs sample 4; full tag fields; Origin Japan',
  },
  {
    code: '20260818005', source: path.join(generated, 'exec-237c7f5e-d91c-4d64-bdef-e29b178324e7.png'),
    target: 'organizations/org_preview/products/prd_3f26e2745359762e9e0171bb8e14ecc0/fil_42b20b6ddfcf404edb0ef51e9190dd5f.png', fileID: 'fil_42b20b6ddfcf404edb0ef51e9190dd5f',
    brand: 'IWC', model: 'Portugieser', reference: 'IW371604', serial: 'CUSTOMS-SERIAL-005', material: 'プラチナPT', belt: 'ナイロン', movement: 'スマート', marking: '♡', note: 'Customs sample 5; full tag fields; Origin Japan',
  },
  {
    code: '20260818006', source: path.join(generated, 'exec-a7b6d6ba-6a1a-4636-970f-0db46b4f5497.png'),
    target: 'organizations/org_preview/products/prd_3f1c9b28ad33c195450b8b8e5de20e9f/fil_385ab9f40e4af30c0db4ede46dc00fe0.png', fileID: 'fil_385ab9f40e4af30c0db4ede46dc00fe0',
    brand: 'ブライトリング', model: 'Navitimer', reference: 'AB0138211B1P1', serial: 'CUSTOMS-SERIAL-006', material: 'チタンTi', belt: 'ステンレス', movement: '自動巻き', marking: '☆', note: 'Customs sample 6; full tag fields; Origin Japan',
  },
  {
    code: '20260818007', source: path.join(generated, 'exec-fcc3c82d-4748-4559-aeaa-7dc587992e5c.png'),
    target: 'organizations/org_preview/products/prd_c32e53f37fc2f30077f6cbc15296731d/fil_a0b842419a19f6031603db7ad2b8cded.png', fileID: 'fil_a0b842419a19f6031603db7ad2b8cded',
    brand: 'タグ・ホイヤー', model: 'Carrera', reference: 'CBS2210.FC6534', serial: 'CUSTOMS-SERIAL-007', material: 'ステンレスSS', belt: 'レザー', movement: '手巻き', marking: '♧', note: 'Customs sample 7; full tag fields; Origin Japan',
  },
  {
    code: '20260818008', source: path.join(generated, 'exec-cbfd1716-543e-4c01-927b-4a6ea4df80f0.png'),
    target: 'organizations/org_preview/products/prd_36b0022db3ec72fc8695088b164d0dff/fil_66368eab63c38c0c8e6dce3a4c489b7f.png', fileID: 'fil_66368eab63c38c0c8e6dce3a4c489b7f',
    brand: 'セイコー', model: 'Prospex', reference: 'SBDC101', serial: 'CUSTOMS-SERIAL-008', material: 'イエローゴールドYG', belt: 'ラバー', movement: 'クオーツ', marking: '♤', note: 'Customs sample 8; full tag fields; Origin Japan',
  },
  {
    code: '20260818009', source: path.join(generated, 'exec-766bd72d-2341-4382-81b8-c027c002fe23.png'),
    target: 'organizations/org_preview/products/prd_408bcf8be0c2ba0a9e276078ef137884/fil_e95dce9396c082c7eef877f873903f9f.png', fileID: 'fil_e95dce9396c082c7eef877f873903f9f',
    brand: 'グランドセイコー', model: 'Heritage Collection', reference: 'SBGA211', serial: 'CUSTOMS-SERIAL-009', material: 'ホワイトゴールドWG', belt: 'チタン', movement: '電波', marking: '♡', note: 'Customs sample 9; full tag fields; Origin Japan',
  },
  {
    code: '20260818010', source: path.join(generated, 'exec-8fb50ff0-ab95-4837-8c0c-a0353c2fc97b.png'),
    target: 'organizations/org_preview/products/prd_75fd3799e3bf4191e1c1ef4231a61101/fil_6c978e73af9d50f789cf2c5d71f4e0a2.png', fileID: 'fil_6c978e73af9d50f789cf2c5d71f4e0a2',
    brand: 'その他', model: 'Classic Watch', reference: 'OTHER-001', serial: 'CUSTOMS-SERIAL-010', material: 'ピンクゴールドPG', belt: 'ナイロン', movement: 'スマート', marking: '☆', note: 'Customs sample 10; full tag fields; Origin Japan',
  },
];

const xml = (value) => String(value).replace(/[&<>"']/g, (c) => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&apos;'}[c]));

function qrMarkup(value, x, y, size) {
  const qr = qrcode(0, 'M');
  qr.addData(value);
  qr.make();
  const count = qr.getModuleCount();
  const quiet = 4;
  const cell = size / (count + quiet * 2);
  let modules = `<rect x="${x}" y="${y}" width="${size}" height="${size}" rx="8" fill="#fff" stroke="#6b7280" stroke-dasharray="4 3"/>`;
  for (let row = 0; row < count; row += 1) {
    for (let col = 0; col < count; col += 1) {
      if (!qr.isDark(row, col)) continue;
      modules += `<rect x="${x + (col + quiet) * cell}" y="${y + (row + quiet) * cell}" width="${cell + 0.2}" height="${cell + 0.2}" fill="#050505"/>`;
    }
  }
  return modules;
}

function splitNote(note, limit = 31) {
  if (note.length <= limit) return [note];
  const cut = note.lastIndexOf(' ', limit);
  return [note.slice(0, cut > 0 ? cut : limit), note.slice((cut > 0 ? cut : limit) + 1)];
}

function tagSvg(p) {
  const tx = 1052, ty = 150, tw = 510, th = 850;
  const left = tx + 44, right = tx + tw - 44;
  const notes = splitNote(p.note);
  const accessories = 'BOX、GUARANTEE、BRACELET PARTS';
  const headlineLength = `${p.brand}（${p.material}／${p.belt}）`.length;
  const headlineSize = headlineLength > 30 ? 14 : headlineLength > 24 ? 16 : 27;
  const accessorySize = accessories.length > 24 ? 19 : 25;
  const rows = [
    {y: 355, value: p.model},
    {y: 420, value: p.reference},
    {y: 485, value: p.serial},
    {y: 550, value: accessories, fontSize: accessorySize},
  ];
  const rowSvg = rows.map(({y, value, fontSize}) => `<text x="${left}" y="${y}" class="body"${fontSize ? ` style="font-size:${fontSize}px"` : ''}>${xml(value)}</text><line x1="${left}" y1="${y + 18}" x2="${right}" y2="${y + 18}" class="line"/>`).join('');
  return Buffer.from(`<svg width="1728" height="1152" viewBox="0 0 1728 1152" xmlns="http://www.w3.org/2000/svg">
    <defs><filter id="shadow" x="-20%" y="-20%" width="140%" height="150%"><feDropShadow dx="0" dy="12" stdDeviation="18" flood-color="#64748b" flood-opacity=".20"/></filter></defs>
    <style>
      text { font-family: Meiryo, 'Yu Gothic', sans-serif; fill:#111827; }
      .headline { font-size:27px; font-weight:700; }
      .body { font-size:25px; font-weight:500; }
      .small { font-size:20px; font-weight:600; }
      .code { font-size:23px; font-weight:700; }
      .line { stroke:#4b5563; stroke-width:1.5; }
    </style>
    <rect x="${tx}" y="${ty}" width="${tw}" height="${th}" rx="38" fill="#fbfbfb" stroke="#e5e7eb" filter="url(#shadow)"/>
    <circle cx="${tx + tw/2}" cy="${ty + 48}" r="24" fill="#f8fafc" stroke="#9ca3af" stroke-width="8"/>
    <circle cx="${tx + tw/2}" cy="${ty + 48}" r="11" fill="#fff"/>
    <text x="${left}" y="292" class="headline" style="font-size:${headlineSize}px">${xml(p.brand)}（${xml(p.material)}／${xml(p.belt)}）</text>
    <line x1="${left}" y1="314" x2="${right}" y2="314" class="line"/>
    ${rowSvg}
    <rect x="${left}" y="590" width="${right-left}" height="134" rx="12" fill="#fff" stroke="#4b5563" stroke-width="1.5"/>
    <text x="${left + 18}" y="628" class="small">${xml(notes[0])}</text>
    ${notes[1] ? `<text x="${left + 18}" y="662" class="small">${xml(notes[1])}</text>` : ''}
    ${qrMarkup(p.code, left, 758, 168)}
    <text x="${left + 202}" y="817" class="small">${xml(p.movement)}</text>
    <line x1="${left + 202}" y1="842" x2="${left + 325}" y2="842" class="line"/>
    <text x="${left + 352}" y="817" font-size="34">${xml(p.marking)}</text>
    <line x1="${left + 345}" y1="842" x2="${right}" y2="842" class="line"/>
    <text x="${left + 202}" y="920" class="code">${xml(p.code)}</text>
    <line x1="${left + 202}" y1="944" x2="${right}" y2="944" class="line"/>
  </svg>`);
}

async function main() {
  fs.mkdirSync(stagingRoot, {recursive: true});
  fs.mkdirSync(backupRoot, {recursive: true});
  const results = [];
  for (const p of products) {
    if (!fs.existsSync(p.source)) throw new Error(`Missing source: ${p.source}`);
    const target = path.join(uploadRoot, ...p.target.split('/'));
    if (!fs.existsSync(target)) throw new Error(`Missing current target: ${target}`);
    const backup = path.join(backupRoot, path.basename(target).replace('.png', `-${p.code}.png`));
    if (!fs.existsSync(backup)) fs.copyFileSync(target, backup);
    const staged = path.join(stagingRoot, `${p.code}.png`);
    if (p.exactReference) {
      await sharp(p.source).resize(1728, 1152, {fit: 'fill'}).png().toFile(staged);
    } else {
      const base = await sharp(p.source).resize(1728, 1152, {fit: 'fill'}).png().toBuffer();
      await sharp(base).composite([{input: tagSvg(p), left: 0, top: 0}]).png().toFile(staged);
    }
    fs.copyFileSync(staged, target);
    const bytes = fs.readFileSync(target);
    results.push({code: p.code, fileID: p.fileID, target, backup, staged, size: bytes.length, sha256: crypto.createHash('sha256').update(bytes).digest('hex')});
  }
  fs.writeFileSync(path.join(stagingRoot, 'manifest.json'), JSON.stringify(results, null, 2));
  console.log(JSON.stringify(results, null, 2));
}

main().catch((error) => { console.error(error); process.exit(1); });
