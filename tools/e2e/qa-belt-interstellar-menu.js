// tools/e2e/qa-belt-interstellar-menu.js
// QA-проба (баг 2026-09-24): в активном межзвёздном полёте ПКМ по поясу в
// модалке системы должен показывать пункт «🚀 Лететь» (композитный маршрут,
// спека 99.2.30 §6.2: композитный пункт при межзвёздном НЕ блокируется).
// До фикса пункт скрывался безусловной проверкой !interstellarFlight.
//
// Межзвёздный полёт имитируется подменой ответа /me (flight != null) — это ровно
// то состояние, из которого /me заполняет modalState.interstellarFlight
// (modal/index.js). Требует: dev-сервер (BASE_URL), QA_TOKEN, мир с поясами.
// Переменные: BASE_URL, QA_TOKEN, QA_BELT_WORLD_ID, QA_BELT_WORLD_NAME,
// QA_BELT_WORLD_SPEC, QA_FLY_FROM, QA_FLY_TO. ASCII-вывод (PowerShell cp866).
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const ARTIFACTS_DIR = path.join(path.dirname(fileURLToPath(import.meta.url)), 'artifacts');
const TOKEN = process.env.QA_TOKEN || '';
const W = {
  id: process.env.QA_BELT_WORLD_ID || '',
  name: process.env.QA_BELT_WORLD_NAME || 'QA-мир',
  spec: process.env.QA_BELT_WORLD_SPEC || 'M',
};
const FLY_FROM = process.env.QA_FLY_FROM || W.id;
const FLY_TO = process.env.QA_FLY_TO || W.id;

const results = [];
function report(step, status, detail) {
  results.push({ step, status, detail });
  console.log(`[${step}] ${status}${detail ? ' - ' + detail : ''}`);
}

const CHROME_PATHS = [process.env.CHROME_PATH, 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'].filter(Boolean);
const EDGE_PATHS = [process.env.EDGE_PATH, 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'].filter(Boolean);
function findExecutable() {
  for (const p of CHROME_PATHS) if (existsSync(p)) return { path: p, name: 'Chrome' };
  for (const p of EDGE_PATHS) if (existsSync(p)) return { path: p, name: 'Edge' };
  return null;
}

let browser = null;
async function finish(code) {
  if (browser) await browser.close().catch(() => {});
  process.exit(code);
}

async function main() {
  if (!TOKEN) { console.error('QA_TOKEN не задан'); return finish(2); }
  if (!W.id) { console.error('QA_BELT_WORLD_ID не задан'); return finish(2); }
  const exe = findExecutable();
  if (!exe) { console.error('Chrome/Edge не найден'); return finish(2); }
  if (!existsSync(ARTIFACTS_DIR)) mkdirSync(ARTIFACTS_DIR, { recursive: true });

  browser = await chromium.launch({ executablePath: exe.path, headless: true });
  const page = await browser.newPage({ viewport: { width: 1600, height: 900 } });
  const pageErrors = [];
  page.on('pageerror', (e) => pageErrors.push(String(e && e.message ? e.message : e)));
  await page.addInitScript((t) => localStorage.setItem('token', t), TOKEN);

  // Подмена /me: добавляем flight (межзвёздный полёт) — так modalState
  // .interstellarFlight становится непустым, как при реальном полёте.
  let injectFlight = false;
  await page.route('**/me', async (route) => {
    try {
      const resp = await route.fetch();
      let body = null;
      try { body = await resp.json(); } catch { /* not json */ }
      if (injectFlight && body && typeof body === 'object') {
        body.flight = {
          from: FLY_FROM, to: FLY_TO,
          start_time: Date.now() - 1000, duration: 3600,
          start_x: 0, start_y: 0,
        };
      }
      const headers = { ...resp.headers() };
      headers['content-type'] = 'application/json';
      await route.fulfill({ status: resp.status(), headers, body: JSON.stringify(body) });
    } catch (e) {
      await route.continue().catch(() => {});
    }
  });

  await page.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(1500);

  // Включаем имитацию межзвёздного полёта и открываем модалку своей системы.
  injectFlight = true;
  await page.evaluate((w) => {
    window.openSystemModal(w.id, w.name, w.spec, null, null, { hasEngine: true });
  }, W);
  await page.waitForSelector('#system-modal-overlay', { timeout: 15000 });
  await page.waitForTimeout(1500);

  // Межзвёздный полёт подхвачен модалкой?
  const interstellar = await page.evaluate(() => {
    // косвенно: в шапке карты «В полёте: …» (state.isFlying) — признак, что /me
    // отдал flight; саму modalState.interstellarFlight из DOM не достать.
    const prefix = document.getElementById('currentWorldPrefix');
    return prefix ? prefix.textContent : '';
  });
  report('interstellar-injected', interstellar.includes('В полёте') ? 'PASS' : 'WARN', interstellar || '(нет)');

  // Строка пояса есть?
  const beltRow = await page.$('[data-belt-row]');
  if (!beltRow) { report('belt-row', 'FAIL', 'строка пояса не найдена (нет поясов/видимости?)'); return finish(1); }
  report('belt-row', 'PASS', 'строка пояса найдена');

  // ПКМ по строке пояса -> меню.
  await beltRow.click({ button: 'right' });
  const menuOpened = await page.waitForSelector('#star-context-menu', { timeout: 5000 }).then(() => true).catch(() => false);
  if (!menuOpened) { report('belt-menu', 'FAIL', 'ПКМ-меню пояса не открылось'); return finish(1); }

  const menuInfo = await page.evaluate(() => {
    const menu = document.getElementById('star-context-menu');
    if (!menu) return null;
    const items = [...menu.querySelectorAll('div')];
    const fly = items.find((d) => d.textContent.includes('Лететь'));
    const mine = items.find((d) => d.textContent.includes('Добывать'));
    return { fly: !!fly, mineTitle: mine ? (mine.title || '') : null };
  });

  // Положительный контроль: пункт «Добывать» получает title «Вы в полёте —
  // дождитесь прибытия» ТОЛЬКО при modalState.interstellarFlight (myPos пуст).
  // Значит, если title есть — имитация межзвёздного полёта дошла до меню.
  const controlOk = !!(menuInfo && menuInfo.mineTitle && menuInfo.mineTitle.includes('дождитесь прибытия'));
  report('control-interstellar-set', controlOk ? 'PASS' : 'FAIL',
    controlOk ? 'modalState.interstellarFlight установлен (доказано title пункта «Добывать»)'
              : `контроль не подтвердил межзвёздный полёт (mineTitle=${menuInfo ? JSON.stringify(menuInfo.mineTitle) : 'нет меню'})`);

  report('belt-menu-fly', menuInfo && menuInfo.fly ? 'PASS' : 'FAIL',
    menuInfo && menuInfo.fly ? 'пункт «Лететь» есть при межзвёздном полёте' : 'пункта «Лететь» НЕТ при межзвёздном полёте (баг)');

  await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'qa-belt-interstellar-menu.png') });
  if (pageErrors.length) report('pageerrors', 'WARN', pageErrors.slice(0, 3).join(' | '));
  const failed = results.some((r) => r.status === 'FAIL');
  return finish(failed ? 1 : 0);
}

main().catch((e) => { console.error('qa-belt-interstellar-menu error:', e); finish(1); });
