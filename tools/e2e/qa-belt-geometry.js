// tools/e2e/qa-belt-geometry.js
// QA-проба (свой прогон @tester, задача «пояс на схеме»): у пояса есть
// канвас-точка (beltPoint) на своей орбите, НЕ центр звезды (layout.mainX/Y);
// у строки пояса нет кнопок [data-belt-fly]/[data-belt-composite-fly]/[data-belt-mine],
// но есть [data-belt-row]. ASCII-вывод.
import { chromium } from 'playwright-core';
import { existsSync } from 'node:fs';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const TOKEN = process.env.QA_TOKEN || '';
const WORLD = {
  id: process.env.QA_WORLD_ID || '',
  name: process.env.QA_WORLD_NAME || 'QA',
  spec: process.env.QA_WORLD_SPEC || 'K',
};
const CHROME = ['C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe',
  'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'].find(p => existsSync(p));

let browser = null;
async function finish(c) { if (browser) await browser.close().catch(() => {}); process.exit(c); }

async function main() {
  if (!TOKEN || !WORLD.id) { console.error('QA_TOKEN/QA_WORLD_ID не заданы'); return finish(2); }
  browser = await chromium.launch({ executablePath: CHROME, headless: true });
  const page = await browser.newPage({ viewport: { width: 1600, height: 900 } });
  page.on('pageerror', e => console.log('PAGEERROR:', e.message));
  await page.addInitScript((t) => localStorage.setItem('token', t), TOKEN);
  await page.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(1500);
  await page.evaluate((w) => window.openSystemModal(w.id, w.name, w.spec, null, null, { hasEngine: true }), WORLD);
  await page.waitForSelector('#system-modal-overlay', { timeout: 15000 });
  await page.waitForTimeout(2500);

  const res = await page.evaluate(async () => {
    const layoutMod = await import('/static/js/modal/layout.js');
    const stateMod = await import('/static/js/modal/state.js');
    const ms = stateMod.modalState;
    const layout = layoutMod.computeLayout(ms.planets, ms.starRadius, ms.canvasWidth || 900, ms.canvasHeight || 700);
    const out = [];
    for (const b of (ms.belts || [])) {
      const ring = layoutMod.beltRing(layout, b);
      const pt = layoutMod.beltPoint(layout, b);
      const dMain = Math.hypot(pt.x - layout.mainX, pt.y - layout.mainY);
      out.push({
        id: b.id, kind: b.kind, orbit_index: (typeof b.orbit_index === 'number' ? b.orbit_index : null),
        pt: { x: Math.round(pt.x), y: Math.round(pt.y) },
        main: { x: Math.round(layout.mainX), y: Math.round(layout.mainY) },
        ringRadius: Math.round(ring.radius), half: +ring.half.toFixed(2),
        distToStarCenter: Math.round(dMain),
        isStarCenter: (Math.abs(pt.x - layout.mainX) < 0.5 && Math.abs(pt.y - layout.mainY) < 0.5),
      });
    }
    const panel = document.getElementById('right-panel');
    return {
      beltsCount: (ms.belts || []).length,
      planetsCount: (ms.planets || []).length,
      belts: out,
      countBeltRow: (panel ? panel.querySelectorAll('[data-belt-row]').length : -1),
      countBeltFly: (panel ? panel.querySelectorAll('[data-belt-fly]').length : -1),
      countBeltCompositeFly: (panel ? panel.querySelectorAll('[data-belt-composite-fly]').length : -1),
      countBeltMine: (panel ? panel.querySelectorAll('[data-belt-mine]').length : -1),
    };
  });
  console.log(JSON.stringify(res, null, 2));
  const allOffCenter = res.belts.length > 0 && res.belts.every(b => !b.isStarCenter && b.distToStarCenter > 10);
  const noButtons = res.countBeltFly === 0 && res.countBeltCompositeFly === 0 && res.countBeltMine === 0;
  const hasRow = res.countBeltRow > 0;
  console.log(`[belt-canvas-point] ${allOffCenter ? 'PASS' : 'FAIL'} belts=${res.belts.length}`);
  console.log(`[belt-row-no-buttons] ${(noButtons && hasRow) ? 'PASS' : 'FAIL'} row=${res.countBeltRow} fly=${res.countBeltFly} composite=${res.countBeltCompositeFly} mine=${res.countBeltMine}`);
  return finish((allOffCenter && noButtons && hasRow) ? 0 : 1);
}
main().catch(e => { console.error('error:', e); finish(1); });
