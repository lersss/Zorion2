// tools/e2e/qa99a3-center-check.js — проверка центрирования при клике по карточке на канвасе.
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

// helper: экранный центр карточки id
const cardCenter = (id) => p.evaluate((cid) => {
  const L = computeLayout(focusSubset(), selected);
  const pos = L.pos[cid];
  const rect = document.getElementById('graph').getBoundingClientRect();
  if (!pos) return null;
  return { x: rect.left + (pos.x + CARD_W / 2) * zoom + panX, y: rect.top + (pos.y + CARD_H / 2) * zoom + panY };
}, id);

// 1. зум 1.5, выбрать h_0001, кликнуть по дочке h_0002 → карточка в центре, зум сохранён
await p.evaluate(() => { zoom = 1.5; panX = 0; panY = 0; saveView(); renderGraph(); });
await p.evaluate(() => { const g = state.goods.find(x => x.id === 'h_0001'); selected = g.id; renderAll(); });
const childPt = await cardCenter('h_0002');
await p.mouse.click(childPt.x, childPt.y);
await p.waitForTimeout(400);
const afterClick = await p.evaluate(() => {
  const L = computeLayout(focusSubset(), selected);
  const pos = L.pos[selected];
  const rect = document.getElementById('graph').getBoundingClientRect();
  const cx = rect.left + (pos.x + CARD_W / 2) * zoom + panX;
  const cy = rect.top + (pos.y + CARD_H / 2) * zoom + panY;
  return { selected: selected, zoom: zoom, centerX: cx, centerY: cy, viewCX: rect.left + rect.width / 2, viewCY: rect.top + rect.height / 2 };
});
const centered = Math.abs(afterClick.centerX - afterClick.viewCX) < 3 && Math.abs(afterClick.centerY - afterClick.viewCY) < 3;
report('1 click child centers view', afterClick.selected === 'h_0002' && centered && Math.abs(afterClick.zoom - 1.5) < 0.01, JSON.stringify(afterClick));

// 2. повторный клик по родителю h_0001 → снова в центре, зум сохранён
const parentPt = await cardCenter('h_0001');
await p.mouse.click(parentPt.x, parentPt.y);
await p.waitForTimeout(400);
const afterParent = await p.evaluate(() => {
  const L = computeLayout(focusSubset(), selected);
  const pos = L.pos[selected];
  const rect = document.getElementById('graph').getBoundingClientRect();
  const cx = rect.left + (pos.x + CARD_W / 2) * zoom + panX;
  const cy = rect.top + (pos.y + CARD_H / 2) * zoom + panY;
  return { selected: selected, zoom: zoom, centerX: cx, centerY: cy, viewCX: rect.left + rect.width / 2, viewCY: rect.top + rect.height / 2 };
});
const centered2 = Math.abs(afterParent.centerX - afterParent.viewCX) < 3 && Math.abs(afterParent.centerY - afterParent.viewCY) < 3;
report('2 click parent centers again', afterParent.selected === 'h_0001' && centered2 && Math.abs(afterParent.zoom - 1.5) < 0.01, JSON.stringify(afterParent));

// 3. pan-драг: mousedown на пустом месте + движение → pan меняется, центрирование не срабатывает
const dragRes = await p.evaluate(() => {
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
const panBefore = await p.evaluate(() => ({ panX, panY }));
await p.mouse.move(dragRes.x, dragRes.y);
await p.mouse.down();
await p.mouse.move(dragRes.x + 120, dragRes.y + 80, { steps: 5 });
await p.mouse.up();
await p.waitForTimeout(300);
const panAfter = await p.evaluate(() => ({ panX, panY, selected }));
report('3 pan drag works, no recenter', panAfter.panX !== panBefore.panX && panAfter.panY !== panBefore.panY && panAfter.selected === 'h_0001', `before=${JSON.stringify(panBefore)} after=${JSON.stringify(panAfter)}`);

// 4. выбор из справочника центрирует (зум сохранён)
await p.evaluate(() => { zoom = 1.2; saveView(); });
await p.locator('#spravList .card').nth(0).click();
await p.waitForTimeout(400);
const spravSel = await p.evaluate(() => {
  const L = computeLayout(focusSubset(), selected);
  const pos = L.pos[selected];
  const rect = document.getElementById('graph').getBoundingClientRect();
  const cx = rect.left + (pos.x + CARD_W / 2) * zoom + panX;
  const cy = rect.top + (pos.y + CARD_H / 2) * zoom + panY;
  return { selected: selected, zoom: zoom, centerX: cx, centerY: cy, viewCX: rect.left + rect.width / 2, viewCY: rect.top + rect.height / 2 };
});
const centered3 = Math.abs(spravSel.centerX - spravSel.viewCX) < 3 && Math.abs(spravSel.centerY - spravSel.viewCY) < 3;
report('4 sprav click centers', centered3 && Math.abs(spravSel.zoom - 1.2) < 0.01, JSON.stringify(spravSel));

// 5. нет JS-ошибок
report('5 no page JS errors', errs.length === 0, errs.length ? errs.join(' | ').slice(0, 300) : 'clean');

await b.close();
const fails = results.filter(r => r === 'FAIL').length;
console.log(`\nTOTAL: ${results.length} steps, ${fails} FAIL`);
process.exit(fails ? 1 : 0);