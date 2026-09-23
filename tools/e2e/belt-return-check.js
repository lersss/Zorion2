// tools/e2e/belt-return-check.js
// e2e-проверка целевого поведения «выход из добычи возвращает на карту с уже
// открытым попапом системы пояса» (решение создателя 2026-09-23). ASCII-вывод.
//
// Механизм: страница добычи перед уходом ставит sessionStorage['beltReturn'] =
// world_id; карта при первой загрузке (map/data.js loadUserData) читает метку,
// совпавшую с current_world_id, и открывает openSystemModal. Проверяем:
//   A) метка beltReturn == current_world_id -> попап открыт (и метка потреблена);
//   B) обычный заход (без метки) -> попап НЕ открыт (ложных срабатываний нет);
//   C) путь «экран ошибки входа»: /belt.html?belt=<bogus> -> «Вернуться» ->
//      /map с открытым попапом (доказывает, что страница добычи ставит метку);
//   D) живой выход кнопкой (QA_TOKEN + QA_BELT_ID, игрок в поясе) -> /map + попап.
//
// Запуск: cd tools/e2e; npm.cmd i; node belt-return-check.js
// Переменные: BASE_URL (default http://localhost:8080), QA_TOKEN, QA_BELT_ID.
import { chromium } from 'playwright-core';
import { existsSync } from 'node:fs';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const TOKEN = process.env.QA_TOKEN || '';
const BELT_ID = process.env.QA_BELT_ID || '';
const BOGUS_BELT = '00000000-0000-0000-0000-000000000000';
const CHROME = [
  process.env.CHROME_PATH,
  'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe',
  'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe',
].filter(Boolean).find((p) => existsSync(p));

let browser = null;
async function finish(c) { if (browser) await browser.close().catch(() => {}); process.exit(c); }

const results = [];
function report(name, ok, detail) {
  results.push({ name, ok: !!ok });
  console.log(`[${name}] ${ok ? 'PASS' : 'FAIL'}${detail ? ' - ' + detail : ''}`);
}

async function register() {
  for (let i = 0; i < 3; i++) {
    const username = 'e2e_beltret_' + Date.now() + '_' + i;
    const res = await fetch(BASE_URL + '/register', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password: 'e2e-pass-' + Date.now() }),
    });
    if (res.status === 201) return res.json();
    if (res.status === 409) continue; // name taken - try another
    throw new Error('register HTTP ' + res.status);
  }
  throw new Error('register: name collision');
}

async function currentWorldId(token) {
  const res = await fetch(BASE_URL + '/me', { headers: { Authorization: 'Bearer ' + token } });
  if (!res.ok) return null;
  const me = await res.json();
  return me.current_world_id || null;
}

// waitMapLoaded - ждёт canvas карты и снятие прелоадера (#loading).
async function waitMapLoaded(page) {
  await page.waitForSelector('#mapCanvas', { timeout: 15000 }).catch(() => {});
  await page.waitForFunction(() => {
    const el = document.getElementById('loading');
    return !el || el.style.display === 'none';
  }, { timeout: 30000 }).catch(() => {});
}

// openMap - открывает /map с токеном и (опционально) меткой beltReturn.
// Метка/токен ставятся initScript'ом до скриптов страницы (как в реальном
// сценарии: метка уже в sessionStorage к моменту загрузки карты). Метка сеется
// ОДИН раз (флаг __seeded): иначе initScript вернул бы её на F5, и проверка E
// мерила бы не продукт, а сам тест (в жизни F5 метку не восстанавливает).
async function openMap(context, token, marker) {
  await context.addInitScript(({ t, m }) => {
    localStorage.setItem('token', t);
    if (m && !sessionStorage.getItem('__beltRetSeed')) {
      sessionStorage.setItem('beltReturn', m);
      sessionStorage.setItem('__beltRetSeed', '1');
    }
  }, { t: token, m: marker || '' });
  const page = await context.newPage();
  const errors = [];
  page.on('pageerror', (e) => errors.push(e.message));
  await page.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded', timeout: 30000 });
  await waitMapLoaded(page);
  // /me -> checkBeltReturn -> openSystemModal (планеты) - даём модалке успеть.
  await page.waitForTimeout(2500);
  const st = await page.evaluate(() => ({
    path: location.pathname,
    modal: !!document.getElementById('system-modal-overlay'),
    marker: sessionStorage.getItem('beltReturn'),
  }));
  st.errors = errors;
  return { page, st };
}

async function main() {
  if (!CHROME) { console.error('Chrome/Edge not found'); return finish(2); }

  // Шаг 1: свежий игрок (у него есть current_world_id - точка спавна).
  let creds = null, worldId = null;
  try {
    creds = await register();
    worldId = await currentWorldId(creds.token);
    report('1 setup register', !!worldId, 'world=' + (worldId || 'NULL'));
  } catch (err) {
    report('1 setup register', false, String(err && err.message ? err.message : err));
    return finish(1);
  }
  if (!worldId) {
    report('A marker-opens-modal', false, 'нет current_world_id у свежего игрока - проверка невозможна');
    return finish(1);
  }

  browser = await chromium.launch({ executablePath: CHROME, headless: true });

  // A. Метка beltReturn == current_world_id -> попап системы открыт, метка съедена.
  // E (в том же контексте). F5 после возврата: метка уже потреблена -> попап не
  // переоткрывается (ложное срабатывание на рефреше исключено).
  {
    const ctx = await browser.newContext({ viewport: { width: 1280, height: 800 } });
    const { page, st } = await openMap(ctx, creds.token, worldId);
    report('A marker-opens-modal',
      st.modal && st.marker === null && st.path !== '/login-page' && st.errors.length === 0,
      `modal=${st.modal} marker=${st.marker} path=${st.path} errors=${st.errors.length}`);
    await page.reload({ waitUntil: 'domcontentloaded' });
    await waitMapLoaded(page);
    await page.waitForTimeout(2000);
    const st2 = await page.evaluate(() => ({
      path: location.pathname,
      modal: !!document.getElementById('system-modal-overlay'),
      marker: sessionStorage.getItem('beltReturn'),
    }));
    report('E reload-no-reopen',
      !st2.modal && st2.marker === null && st2.path !== '/login-page',
      `modal=${st2.modal} marker=${st2.marker} path=${st2.path}`);
    await ctx.close();
  }

  // B. Обычный заход (без метки) -> попап НЕ открывается сам.
  {
    const ctx = await browser.newContext({ viewport: { width: 1280, height: 800 } });
    const { st } = await openMap(ctx, creds.token, '');
    report('B no-marker-no-modal',
      !st.modal && st.path !== '/login-page' && st.errors.length === 0,
      `modal=${st.modal} path=${st.path} errors=${st.errors.length}`);
    await ctx.close();
  }

  // C. Экран ошибки входа: bogus belt -> «Вернуться» -> /map + попап.
  {
    const ctx = await browser.newContext({ viewport: { width: 1280, height: 800 } });
    await ctx.addInitScript((t) => localStorage.setItem('token', t), creds.token);
    const page = await ctx.newPage();
    const errors = [];
    page.on('pageerror', (e) => errors.push(e.message));
    await page.goto(BASE_URL + '/belt.html?belt=' + BOGUS_BELT, { waitUntil: 'domcontentloaded' });
    await page.waitForSelector('#error', { state: 'visible', timeout: 15000 }).catch(() => {});
    const label = await page.evaluate(() => {
      const b = document.getElementById('error-map');
      return b ? b.textContent.trim() : null;
    });
    await page.click('#error-map').catch(() => {});
    await page.waitForURL('**/map', { timeout: 15000 }).catch(() => {});
    await waitMapLoaded(page);
    await page.waitForTimeout(2500);
    const st = await page.evaluate(() => ({
      path: location.pathname,
      modal: !!document.getElementById('system-modal-overlay'),
      marker: sessionStorage.getItem('beltReturn'),
    }));
    report('C error-screen-return',
      label === 'Вернуться' && st.path === '/map' && st.modal && st.marker === null && errors.length === 0,
      `label="${label}" path=${st.path} modal=${st.modal} marker=${st.marker} errors=${errors.length}`);
    await ctx.close();
  }

  // D. Живой выход кнопкой из сцены добычи (нужен игрок в поясе).
  if (TOKEN && BELT_ID) {
    const ctx = await browser.newContext({ viewport: { width: 1600, height: 900 } });
    await ctx.addInitScript((t) => localStorage.setItem('token', t), TOKEN);
    const page = await ctx.newPage();
    const errors = [];
    page.on('pageerror', (e) => errors.push(e.message));
    await page.goto(BASE_URL + '/belt.html?belt=' + BELT_ID, { waitUntil: 'domcontentloaded' });
    await page.waitForTimeout(3500);
    const label = await page.evaluate(() => {
      const b = document.getElementById('belt-leave-btn');
      return b ? b.textContent.trim() : null;
    });
    await page.click('#belt-leave-btn').catch(() => {});
    await page.waitForURL('**/map', { timeout: 15000 }).catch(() => {});
    await waitMapLoaded(page);
    await page.waitForTimeout(2500);
    const st = await page.evaluate(() => ({
      path: location.pathname,
      modal: !!document.getElementById('system-modal-overlay'),
      marker: sessionStorage.getItem('beltReturn'),
    }));
    report('D live-leave-button',
      label === 'Вернуться' && st.path === '/map' && st.modal && st.marker === null && errors.length === 0,
      `label="${label}" path=${st.path} modal=${st.modal} marker=${st.marker} errors=${errors.length}`);
    await ctx.close();
  } else {
    console.log('[D live-leave-button] SKIP - QA_TOKEN/QA_BELT_ID not set');
  }

  const failed = results.some((r) => !r.ok);
  console.log(`\n${results.filter((r) => r.ok).length}/${results.length} PASS`);
  return finish(failed ? 1 : 0);
}

main().catch((e) => { console.error('belt-return-check error:', e); finish(1); });
