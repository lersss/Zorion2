// tools/e2e/qa99a3-fix-smoke.js — смоук баг-фикса: клик по дочке в фокус-подграфе
// не сбрасывает выбор (регрессия 99a.3); клик по пустому месту — сбрасывает.
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

// 1. выбрать товар с дочками (h_0001 — корень дерева, 3 детей)
const sel = await p.evaluate(() => {
  const g = state.goods.find(x => x.id === 'h_0001');
  if (!g) return { error: 'h_0001 not found' };
  selected = g.id; renderAll();
  const sub = focusSubset();
  const child = sub.find(x => x.id !== g.id && x.kind === 'good');
  return { selected: selected, subset: sub.map(x => x.id), child: child ? child.id : null };
});
report('1 select h_0001', sel.selected === 'h_0001' && sel.subset.length > 1, JSON.stringify(sel));

// 2. кликнуть по дочке на канвасе (mousedown+mouseup без движения)
const clickRes = await p.evaluate((childId) => {
  const L = computeLayout(focusSubset(), selected);
  const pos = L.pos[childId];
  const rect = document.getElementById('graph').getBoundingClientRect();
  if (!pos) return { error: 'child not in layout' };
  return { x: rect.left + (pos.x + CARD_W / 2) * zoom + panX, y: rect.top + (pos.y + CARD_H / 2) * zoom + panY };
}, sel.child);
if (clickRes.error) { report('2 click child', false, clickRes.error); }
else {
  await p.mouse.click(clickRes.x, clickRes.y);
  await p.waitForTimeout(400);
  const after = await p.evaluate(() => ({
    selected: selected,
    overlayGone: document.getElementById('focusEmptyOverlay').style.display === 'none',
    subgraph: (() => { const sub = focusSubset(); return sub.map(x => x.id); })(),
  }));
  report('2 click child rebuilds subgraph', after.selected === sel.child && after.overlayGone && after.subgraph.includes(sel.child), JSON.stringify(after));
}

// 2b. клик по УЖЕ выбранной карточке → только выбор, попап НЕ открывается
// (создатель 2026-09-19: «по двойному клику было норм» — попап по dblclick/ⓘ/меню)
const selPt = await p.evaluate(() => {
  const L = computeLayout(focusSubset(), selected);
  const pos = L.pos[selected];
  const rect = document.getElementById('graph').getBoundingClientRect();
  if (!pos) return { error: 'selected not in layout' };
  return { x: rect.left + (pos.x + CARD_W / 2) * zoom + panX, y: rect.top + (pos.y + CARD_H / 2) * zoom + panY };
});
if (selPt.error) { report('2b click selected no popup', false, selPt.error); }
else {
  await p.mouse.click(selPt.x, selPt.y);
  await p.waitForTimeout(300);
  const afterSel = await p.evaluate(() => ({
    selected: selected,
    popupOpen: document.getElementById('detailPopup').style.display === 'flex',
  }));
  report('2b click selected no popup', afterSel.selected === sel.child && !afterSel.popupOpen, JSON.stringify(afterSel));
}

// 3. клик по пустому месту → выбор СОХРАНЯЕТСЯ (создатель 2026-09-19: «по клику
//    вне графа на канвасе не очищать его, мешает»); оверлей не появляется
const emptyPt = await p.evaluate(() => {
  const L = computeLayout(focusSubset(), selected);
  const rect = document.getElementById('graph').getBoundingClientRect();
  const occ = [];
  L.goods.forEach(g => { const q = L.pos[g.id]; if (q) occ.push({x0: q.x, x1: q.x + CARD_W, y0: q.y, y1: q.y + CARD_H}); });
  L.slots.forEach(s => { const q = L.pos[s.id]; if (q) occ.push({x0: q.x, x1: q.x + CARD_W, y0: q.y, y1: q.y + CARD_H}); });
  for (let wy = 10; wy < 3000; wy += 40) {
    for (let wx = 10; wx < 3000; wx += 40) {
      if (!occ.some(r => wx >= r.x0 && wx <= r.x1 && wy >= r.y0 && wy <= r.y1)) {
        return { x: rect.left + wx * zoom + panX, y: rect.top + wy * zoom + panY };
      }
    }
  }
  return null;
});
if (!emptyPt) { report('3 empty click keeps selection', false, 'no empty spot'); }
else {
  await p.mouse.click(emptyPt.x, emptyPt.y);
  await p.waitForTimeout(400);
  const afterEmpty = await p.evaluate(() => ({
    selected: selected,
    overlayShown: document.getElementById('focusEmptyOverlay').style.display === 'flex',
  }));
  report('3 empty click keeps selection', afterEmpty.selected === sel.child && !afterEmpty.overlayShown, JSON.stringify(afterEmpty));
}

// 4. двойной клик по дочке → выбор + попап (регрессия dblclick)
const dblRes = await p.evaluate(() => {
  const g = state.goods.find(x => x.id === 'h_0001');
  selected = g.id; renderAll();
  const sub = focusSubset();
  const child = sub.find(x => x.id !== g.id && x.kind === 'good');
  const L = computeLayout(focusSubset(), selected);
  const pos = L.pos[child.id];
  const rect = document.getElementById('graph').getBoundingClientRect();
  return { child: child.id, x: rect.left + (pos.x + CARD_W / 2) * zoom + panX, y: rect.top + (pos.y + CARD_H / 2) * zoom + panY };
});
await p.mouse.dblclick(dblRes.x, dblRes.y);
await p.waitForTimeout(400);
const afterDbl = await p.evaluate(() => ({
  selected: selected,
  popupOpen: document.getElementById('detailPopup').style.display === 'flex',
  popupGoodId: popupGoodId,
}));
report('4 dblclick child = select + popup', afterDbl.selected === dblRes.child && afterDbl.popupOpen && afterDbl.popupGoodId === dblRes.child, JSON.stringify(afterDbl));
await p.keyboard.press('Escape');

// 5. нет JS-ошибок
report('5 no page JS errors', errs.length === 0, errs.length ? errs.join(' | ').slice(0, 300) : 'clean');

await b.close();
const fails = results.filter(r => r === 'FAIL').length;
console.log(`\nTOTAL: ${results.length} steps, ${fails} FAIL`);
process.exit(fails ? 1 : 0);