// tools/e2e/qa-belt-null-enter.js  (QA-прогон фикса 2026-09-23)
// Фикс: клиентский пред-чек больше НЕ блокирует заход в пояс при пустом
// remaining_level (iron_remaining IS NULL — запас инициализируется лениво на
// первом enter). Проверяем:
//   A) реальный игрок testtest в поясе Ozyal (iron_remaining NULL) ->
//      ПКМ по строке пояса -> «Добывать» -> заход открылся (/belt.html?belt=..),
//      сцена загрузилась, pageerror=0.
//   C) live-добыча в этой же сцене: корабль к жиле, ЛКМ -> «добыто» растёт.
//   B) регресс: remaining_level='выработан' (интерцепт) -> алерт «Пояс выработан»,
//      заход НЕ открывается (остаёмся на /map).
//
// Запуск: cd tools/e2e; node qa-belt-null-enter.js
// Переменные: BASE_URL (default http://localhost:8080), QA_TOKEN (testtest),
//             QA_DEPLETED_TOKEN (свежий игрок для сценария B).
// ASCII-вывод.
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const TT_TOKEN = process.env.QA_TOKEN || '';
const DEP_TOKEN = process.env.QA_DEPLETED_TOKEN || '';
const WORLD = { id: '46cdbffa-645f-4af2-8183-177c6975e5c9', name: 'Ozyal', spec: 'G' };
const BELT = '3c6e168a-04c7-4c5c-abda-70738d8232de';
const DEP_BELT = 'qa-belt-depleted';
const ARTIFACTS_DIR = path.join(path.dirname(fileURLToPath(import.meta.url)), 'artifacts');

const CHROME_PATHS = [process.env.CHROME_PATH, 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'].filter(Boolean);
const EDGE_PATHS = [process.env.EDGE_PATH, 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'].filter(Boolean);
function findExecutable() {
  for (const p of CHROME_PATHS) if (existsSync(p)) return { path: p, name: 'Chrome' };
  for (const p of EDGE_PATHS) if (existsSync(p)) return { path: p, name: 'Edge' };
  return null;
}

const results = [];
function report(name, ok, detail) {
  results.push({ name, ok: !!ok });
  console.log(`[${name}] ${ok ? 'PASS' : 'FAIL'}${detail ? ' - ' + detail : ''}`);
}

let browser = null;
async function finish(code) { if (browser) await browser.close().catch(() => {}); process.exit(code); }

async function main() {
  const exe = findExecutable();
  if (!exe) { console.error('Chrome/Edge not found'); return finish(2); }
  mkdirSync(ARTIFACTS_DIR, { recursive: true });
  browser = await chromium.launch({ executablePath: exe.path, headless: true });

  // ======================= A + C: testtest, реальный NULL-запас =======================
  if (TT_TOKEN) {
    const ctx = await browser.newContext({ viewport: { width: 1600, height: 900 } });
    await ctx.addInitScript((t) => localStorage.setItem('token', t), TT_TOKEN);
    const page = await ctx.newPage();
    const pageErrors = [];
    page.on('pageerror', (e) => { pageErrors.push(e.message); console.log('PAGEERROR:', e.message); });

    await page.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.waitForSelector('#mapCanvas', { timeout: 15000 });
    await page.waitForTimeout(1500);
    await page.evaluate((w) => window.openSystemModal(w.id, w.name, w.spec, null, null, { hasEngine: true }), WORLD);
    await page.waitForSelector('#system-modal-overlay', { timeout: 15000 });
    const rowSel = `[data-belt-row="${BELT}"]`;
    await page.waitForSelector(rowSel, { timeout: 10000 });
    await page.waitForTimeout(400);

    await page.click(rowSel, { button: 'right' });
    await page.waitForSelector('#star-context-menu', { timeout: 5000 });
    const info = await page.evaluate(() => {
      const items = [...document.querySelectorAll('#star-context-menu div')];
      const el = items.find((d) => /Добыва|добычу/.test(d.textContent));
      return { count: el ? 1 : 0, label: el ? el.textContent.trim() : '', disabled: el ? el.style.cursor === 'not-allowed' : null };
    });
    // Клик через dispatch: обработчик синхронно прячет меню и уходит на belt.html —
    // Playwright-клик по исчезающему элементу зависает.
    const clicked = await page.evaluate(() => {
      const items = [...document.querySelectorAll('#star-context-menu div')];
      const el = items.find((d) => /Добыва|добычу/.test(d.textContent));
      if (!el) return false;
      el.click();
      return true;
    });

    // Заход = навигация на /belt.html?belt=...
    const navigated = await page.waitForURL(/\/belt\.html\?belt=/, { timeout: 10000 }).then(() => true).catch(() => false);
    await page.waitForTimeout(300);
    const url = page.url();
    report('A1 mine-item-enabled', info.count > 0 && !info.disabled && clicked,
      `label="${info.label}" disabled=${info.disabled} clicked=${clicked}`);

    let scene = null;
    if (navigated) {
      await page.waitForSelector('#belt-canvas', { timeout: 10000 }).catch(() => {});
      await page.waitForTimeout(3000);
      scene = await page.evaluate(() => {
        const hud = document.getElementById('hud');
        const err = document.getElementById('error');
        const minedEl = document.getElementById('hud-mined');
        return {
          hud: !!hud && hud.style.display !== 'none',
          errorShown: !!err && getComputedStyle(err).display !== 'none',
          errorText: (document.getElementById('error-text') || {}).textContent || '',
          minedText: minedEl ? minedEl.textContent.trim() : null,
          reserve: (document.getElementById('hud-reserve') || {}).textContent || '',
          beltName: (document.getElementById('hud-belt') || {}).textContent || '',
          world: !!window.__beltWorld,
        };
      });
    }
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'qa-belt-null-enter.png') }).catch(() => {});

    report('A2 enter-opened-null-reserve',
      navigated && !!scene && scene.hud && !scene.errorShown && scene.world,
      `navigated=${navigated} url=${url} hud=${scene && scene.hud} error=${scene && scene.errorShown} "${scene && scene.errorText}" belt="${scene && scene.beltName}" reserve="${scene && scene.reserve}"`);
    report('A3 no-page-errors-on-enter', pageErrors.length === 0, `errors=${pageErrors.length}`);

    // ---- C: live-добыча: ставим корабль к жиле, держим ЛКМ ----
    let mine = null;
    if (navigated && scene && scene.hud) {
      const placed = await page.evaluate(() => {
        const w = window.__beltWorld;
        if (!w || !w.asteroids || !w.asteroids.length) return null;
        // Крупное тело, к которому встанем слева, носом (heading=0) вправо.
        let a = null;
        for (const b of w.asteroids) if (!a || b.r > a.r) a = b;
        w.ship.x = a.x - (a.r + 20);
        w.ship.y = a.y;
        w.ship.heading = 0;
        w.ship.vx = 0; w.ship.vy = 0; w.ship.angVel = 0;
        return { x: a.x, y: a.y, r: a.r };
      });
      await page.waitForTimeout(1200); // камера догоняет корабль
      await page.mouse.move(1150, 450); // цель носа — вправо
      await page.waitForTimeout(300);
      await page.mouse.down({ button: 'left' });
      await page.waitForTimeout(4200);
      const minedAfter = await page.evaluate(() => {
        const el = document.getElementById('hud-mined');
        return { text: el ? el.textContent.trim() : null, hint: (document.getElementById('hud-hint') || {}).textContent || '' };
      });
      await page.mouse.up({ button: 'left' });
      await page.waitForTimeout(300);
      const m = (minedAfter.text || '').replace(',', '.').match(/([0-9.]+)/);
      mine = { placed, text: minedAfter.text, hint: minedAfter.hint, tons: m ? Number(m[1]) : 0 };
    }
    report('C1 live-mining-grows',
      !!mine && mine.tons > 0,
      `placed=${JSON.stringify(mine && mine.placed)} mined="${mine && mine.text}" hint="${mine && mine.hint}"`);

    await ctx.close();
  } else {
    report('A2 enter-opened-null-reserve', false, 'QA_TOKEN не задан');
  }

  // ======================= B: «выработан» (интерцепт, свежий игрок) =======================
  if (DEP_TOKEN) {
    const ctx = await browser.newContext({ viewport: { width: 1280, height: 800 } });
    await ctx.addInitScript((t) => localStorage.setItem('token', t), DEP_TOKEN);
    const page = await ctx.newPage();
    await page.route('**/api/worlds/*/planets*', async (route) => {
      const res = await route.fetch();
      let json = {};
      try { json = await res.json(); } catch (e) { json = {}; }
      const belts = Array.isArray(json.belts) ? json.belts.slice() : [];
      if (!belts.some((b) => b && b.id === DEP_BELT)) {
        belts.push({
          id: DEP_BELT, name: 'QA-выработан', kind: 'asteroid',
          radius_au: 2.5, width_au: 0.5, body_size_km: 5, mass: 0.001,
          belt_class: 'бедный', remaining_level: 'выработан',
        });
      }
      json.belts = belts;
      json.my_position = { status: 'orbit', object_type: 'belt', object_id: DEP_BELT };
      await route.fulfill({ json });
    });
    await page.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.waitForSelector('#mapCanvas', { timeout: 15000 });
    await page.waitForTimeout(1500);
    await page.evaluate((w) => window.openSystemModal(w.id, w.name, w.spec, null, null, { hasEngine: true }), WORLD);
    await page.waitForSelector('#system-modal-overlay', { timeout: 15000 });
    await page.waitForSelector(`[data-belt-row="${DEP_BELT}"]`, { timeout: 10000 });
    await page.waitForTimeout(300);
    await page.click(`[data-belt-row="${DEP_BELT}"]`, { button: 'right' });
    await page.waitForSelector('#star-context-menu', { timeout: 5000 });
    await page.evaluate(() => {
      const items = [...document.querySelectorAll('#star-context-menu div')];
      const el = items.find((d) => /Добыва|добычу/.test(d.textContent));
      if (el) el.click();
    });
    await page.waitForSelector('.ui-alert-overlay', { timeout: 5000 }).catch(() => {});
    await page.waitForTimeout(300);
    const st = await page.evaluate(() => ({
      alert: !!document.querySelector('.ui-alert-overlay'),
      title: (document.querySelector('.ui-alert-title') || {}).textContent || '',
      path: location.pathname,
      beltPage: /\/belt\.html/.test(location.href),
    }));
    report('B1 depleted-blocked',
      st.path === '/map' && !st.beltPage,
      `blocked=${st.path === '/map' && !st.beltPage} alert=${st.alert} title="${st.title}" path=${st.path}`);
    await ctx.close();
  } else {
    report('B1 depleted-blocked', false, 'QA_DEPLETED_TOKEN не задан');
  }

  const failed = results.filter((r) => !r.ok).length;
  console.log(`\n${results.length - failed}/${results.length} PASS`);
  return finish(failed ? 1 : 0);
}

main().catch((e) => { console.error('qa-belt-null-enter error:', e); finish(1); });
