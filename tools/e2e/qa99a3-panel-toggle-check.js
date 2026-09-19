// tools/e2e/qa99a3-panel-toggle-check.js — сворачивание панели вытягивает канвас.
import { chromium } from 'playwright-core';
import { existsSync } from 'node:fs';
const exe = ['C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe', 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'].find(p => existsSync(p));
const b = await chromium.launch({ executablePath: exe, headless: true });
const p = await b.newPage({ viewport: { width: 1400, height: 900 } });
await p.addInitScript(() => { if (!sessionStorage.getItem('qa_ls_cleared')) { localStorage.clear(); sessionStorage.setItem('qa_ls_cleared', '1'); } });
const errs = [];
p.on('pageerror', e => errs.push(String(e && e.stack ? e.stack : e)));
const results = [];
const report = (n, ok, d) => { results.push(ok ? 'PASS' : 'FAIL'); console.log(`[${n}] ${ok ? 'PASS' : 'FAIL'}${d ? ' - ' + d : ''}`); };

await p.goto('http://127.0.0.1:8799/', { waitUntil: 'domcontentloaded' });
await p.waitForFunction(() => document.getElementById('loadingOverlay').style.display === 'none', null, { timeout: 15000 });
await p.waitForTimeout(500);

// 1. развёрнуто: канвас right:320px, панель видна
const expanded = await p.evaluate(() => {
  const c = document.getElementById('canvasWrap').getBoundingClientRect();
  const r = document.getElementById('right').getBoundingClientRect();
  return { canvasRight: c.right, rightLeft: r.left, canvasExpanded: document.getElementById('canvasWrap').classList.contains('expanded') };
});
report('1 panel open, canvas right:320', !expanded.canvasExpanded && expanded.canvasRight <= expanded.rightLeft + 2, JSON.stringify(expanded));

// 2. клик «◀» → панель уезжает, канвас на всю ширину (right:0)
await p.click('#panelToggle');
await p.waitForTimeout(400);
const collapsed = await p.evaluate(() => {
  const c = document.getElementById('canvasWrap').getBoundingClientRect();
  const r = document.getElementById('right').getBoundingClientRect();
  const o = document.getElementById('detailOverlay');
  return {
    canvasRight: c.right,
    rightLeft: r.left,
    canvasExpanded: document.getElementById('canvasWrap').classList.contains('expanded'),
    overlayExpanded: o.classList.contains('expanded'),
    canvasW: c.width,
    viewW: window.innerWidth,
  };
});
report('2 collapse expands canvas', collapsed.canvasExpanded && collapsed.overlayExpanded && collapsed.canvasRight >= collapsed.viewW - 30 && collapsed.canvasW > 1300, JSON.stringify(collapsed));

// 3. канвас перерисован (canvas.width пересчитан под новую ширину)
const canvasSize = await p.evaluate(() => {
  const cv = document.getElementById('graph');
  return { cssW: document.getElementById('canvasWrap').clientWidth, canvasW: cv.width, dpr: window.devicePixelRatio || 1 };
});
report('3 canvas redrawn to full width', Math.abs(canvasSize.canvasW - canvasSize.cssW * canvasSize.dpr) < 4, JSON.stringify(canvasSize));

// 4. клик «▶» → панель возвращается, канвас сжимается
await p.click('#panelToggle');
await p.waitForTimeout(400);
const restored = await p.evaluate(() => {
  const c = document.getElementById('canvasWrap').getBoundingClientRect();
  const r = document.getElementById('right').getBoundingClientRect();
  return { canvasRight: c.right, rightLeft: r.left, canvasExpanded: document.getElementById('canvasWrap').classList.contains('expanded'), overlayExpanded: document.getElementById('detailOverlay').classList.contains('expanded') };
});
report('4 expand restores canvas', !restored.canvasExpanded && !restored.overlayExpanded && restored.canvasRight <= restored.rightLeft + 2, JSON.stringify(restored));

// 5. состояние сохраняется (localStorage gs_panelCollapsed)
await p.click('#panelToggle'); // свернуть
await p.waitForTimeout(200);
await p.reload({ waitUntil: 'domcontentloaded' });
await p.waitForFunction(() => document.getElementById('loadingOverlay').style.display === 'none', null, { timeout: 15000 });
await p.waitForTimeout(400);
const persisted = await p.evaluate(() => ({
  collapsed: document.getElementById('right').classList.contains('collapsed'),
  canvasExpanded: document.getElementById('canvasWrap').classList.contains('expanded'),
  overlayExpanded: document.getElementById('detailOverlay').classList.contains('expanded'),
  btnText: document.getElementById('panelToggle').textContent,
}));
report('5 state persisted', persisted.collapsed && persisted.canvasExpanded && persisted.overlayExpanded && persisted.btnText === '▶', JSON.stringify(persisted));
// вернуть панель (чистота)
await p.click('#panelToggle');
await p.waitForTimeout(200);

// 6. нет JS-ошибок
report('6 no page JS errors', errs.length === 0, errs.length ? errs.join(' | ').slice(0, 300) : 'clean');

await b.close();
const fails = results.filter(r => r === 'FAIL').length;
console.log(`\nTOTAL: ${results.length} steps, ${fails} FAIL`);
process.exit(fails ? 1 : 0);