// tools/e2e/qa-belt-canvas-rc.js
// QA-проба (@tester): ПКМ прямо по кольцу пояса на канвасе -> меню showBeltMenu
// («Лететь»/«Добывать»). Реальная мышь в спроецированную точку beltPoint.
import { chromium } from 'playwright-core';
import { existsSync } from 'node:fs';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const TOKEN = process.env.QA_TOKEN || '';
const WORLD = { id: process.env.QA_WORLD_ID || '', name: process.env.QA_WORLD_NAME || 'QA', spec: process.env.QA_WORLD_SPEC || 'K' };
const CHROME = ['C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe',
  'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'].find(p => existsSync(p));

let browser = null;
async function finish(c) { if (browser) await browser.close().catch(() => {}); process.exit(c); }

async function main() {
  if (!TOKEN || !WORLD.id) { console.error('QA_TOKEN/QA_WORLD_ID не заданы'); return finish(2); }
  browser = await chromium.launch({ executablePath: CHROME, headless: true });
  const page = await browser.newPage({ viewport: { width: 1600, height: 900 } });
  const errors = [];
  page.on('pageerror', e => { errors.push(e.message); console.log('PAGEERROR:', e.message); });
  await page.addInitScript((t) => localStorage.setItem('token', t), TOKEN);
  await page.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(1500);
  await page.evaluate((w) => window.openSystemModal(w.id, w.name, w.spec, null, null, { hasEngine: true }), WORLD);
  await page.waitForSelector('#system-modal-overlay', { timeout: 15000 });
  await page.waitForTimeout(2500);

  const info = await page.evaluate(async () => {
    const layoutMod = await import('/static/js/modal/layout.js');
    const stateMod = await import('/static/js/modal/state.js');
    const ms = stateMod.modalState;
    if (!ms.belts || !ms.belts.length) return null;
    const layout = layoutMod.computeLayout(ms.planets, ms.starRadius, ms.canvasWidth || 900, ms.canvasHeight || 700);
    const b = ms.belts.find(x => typeof x.orbit_index === 'number') || ms.belts[0];
    const pt = layoutMod.beltPoint(layout, b);
    const canvas = ms.canvas || document.querySelector('#system-modal-overlay canvas') || document.querySelector('canvas');
    if (!canvas) return { error: 'no canvas' };
    const dpr = window.devicePixelRatio || 1;
    const rect = canvas.getBoundingClientRect();
    const mouseX = pt.x * ms.zoom + ms.offsetX + (ms.followOffsetX || 0);
    const mouseY = pt.y * ms.zoom + ms.offsetY + (ms.followOffsetY || 0);
    const clientX = rect.left + mouseX / ((canvas.width / rect.width) / dpr);
    const clientY = rect.top + mouseY / ((canvas.height / rect.height) / dpr);
    return { beltId: b.id, point: { x: Math.round(pt.x), y: Math.round(pt.y) }, clientX: Math.round(clientX), clientY: Math.round(clientY), rect: { l: Math.round(rect.left), t: Math.round(rect.top), w: Math.round(rect.width), h: Math.round(rect.height) } };
  });
  console.log('info:', JSON.stringify(info));
  if (!info || info.error) { console.log('[canvas-rc] FAIL - ' + (info ? info.error : 'no belts')); return finish(1); }

  const inside = info.clientX > info.rect.l && info.clientX < info.rect.l + info.rect.w && info.clientY > info.rect.t && info.clientY < info.rect.t + info.rect.h;
  console.log(`[point-inside-canvas] ${inside ? 'PASS' : 'FAIL'} - (${info.clientX},${info.clientY}) rect=${JSON.stringify(info.rect)}`);

  await page.mouse.click(info.clientX, info.clientY, { button: 'right' });
  await page.waitForTimeout(700);
  const menu = await page.evaluate(() => {
    const m = document.getElementById('star-context-menu');
    if (!m) return null;
    const txt = m.textContent.replace(/\s+/g, ' ').trim();
    return { txt, hasFly: txt.includes('Лететь'), hasMine: txt.includes('Добывать') };
  });
  const ok = !!menu && menu.hasFly;
  console.log(`[canvas-rc-menu] ${ok ? 'PASS' : 'FAIL'} - ${menu ? menu.txt : 'меню не открылось'}`);
  return finish((inside && ok && errors.length === 0) ? 0 : 1);
}
main().catch(e => { console.error('error:', e); finish(1); });
