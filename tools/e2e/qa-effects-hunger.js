// tools/e2e/qa-effects-hunger.js
//
// E2E админ-витрины эффектов снабжения (спека
// 2026-09-22-эффекты-снабжения-задержка-голод §6/§7.3, закрывает T22):
//  1. Карточка планеты с поселением → вкладка «Поселения»: в блоке «Эффекты»
//     видно нагрузку/порог/состояние/силу и форму «задать нагрузку вручную».
//  2. «Балансировка»: компонента «Голод» → поле «Восстановление, сило-ч/ч»
//     (заводское 0.25), подпись оси «нагрузка»; изменили → «Сохранить» →
//     после перезагрузки значение сохранилось.
//  3. При выбранной расе «Голод» недоступен (не уходит в расовый эндпоинт).
//
// Живую фикстуру (товар+рецепт+ветка+поселение+нагрузка) скрипт кладёт сам из
// tools/e2e/artifacts/effects-hunger-setup.sql (уборка — effects-hunger-resolve.sql).
// Chrome/Edge — системные (playwright-core, executablePath). Вывод ASCII.
// Run: node qa-effects-hunger.js   (BASE_URL env)
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const ARTIFACTS_DIR = path.join(path.dirname(fileURLToPath(import.meta.url)), 'artifacts');
const SETTLEMENT_ID = 'e0000000-0000-4000-8000-000000000001';
const SETUP_SQL = path.join(ARTIFACTS_DIR, 'effects-hunger-setup.sql');
const RESOLVE_SQL = path.join(ARTIFACTS_DIR, 'effects-hunger-resolve.sql');
const USER_PREFIX = 'e2e_eff_';
const PASSWORD = 'e2e-effects-' + Date.now();
const PSQL = process.env.PSQL || 'C:\\pgsql\\pgsql\\bin\\psql.exe';

const CHROME_PATHS = [process.env.CHROME_PATH, 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'].filter(Boolean);
const EDGE_PATHS = [process.env.EDGE_PATH, 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'].filter(Boolean);

const results = [];
function report(step, ok, detail) {
  results.push({ step, ok, detail });
  console.log(`[${step}] ${ok ? 'PASS' : 'FAIL'} - ${detail}`);
}
function findExecutable() {
  for (const p of CHROME_PATHS) if (existsSync(p)) return p;
  for (const p of EDGE_PATHS) if (existsSync(p)) return p;
  return null;
}
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// psqlRun — SQL во временный файл + psql (env PGPASSWORD); tuples — -t -A.
function psqlRun(sql, filename, { tuples = false } = {}) {
  const sqlPath = path.join(os.tmpdir(), filename);
  writeFileSync(sqlPath, sql, 'utf8');
  const args = ['/c', PSQL, '-h', '127.0.0.1', '-U', 'zorion', '-d', 'zorion', '-v', 'ON_ERROR_STOP=1'];
  if (tuples) args.push('-t', '-A');
  args.push('-f', sqlPath);
  return String(execFileSync('cmd', args, {
    env: { ...process.env, PGPASSWORD: process.env.PGPASSWORD || 'zorion123' },
  })).trim();
}

let browser = null;
// cleanupFns — уборка за собой (фикстура + тест-пользователь): выполняется при
// ЛЮБОМ выходе (PASS, FAIL, ранний return), зарегистрированные — в обратном
// порядке. Иначе каждый прогон мутирует live-БД (дефект D1 ревью этапа 4).
const cleanupFns = [];
async function finish(code) {
  if (browser) await browser.close().catch(() => {});
  for (const fn of cleanupFns.reverse()) {
    try { fn(); } catch (e) { console.log('cleanup: ' + String(e)); }
  }
  process.exit(code);
}

async function main() {
  mkdirSync(ARTIFACTS_DIR, { recursive: true });

  // --- Фикстура: товар+рецепт+ветка+поселение+нагрузка (идемпотентно) ---
  // Уборку регистрируем ДО setup: если setup упадёт на середине, resolve
  // всё равно выполнится при выходе.
  cleanupFns.push(() => psqlRun(readFileSync(RESOLVE_SQL, 'utf8'), 'e2e_effects_hunger_resolve_run.sql'));
  psqlRun(readFileSync(SETUP_SQL, 'utf8'), 'e2e_effects_hunger_setup.sql');
  const line = psqlRun(
    `SELECT s.planet_id, p.world_id FROM settlements s JOIN planets p ON p.id = s.planet_id WHERE s.id = '${SETTLEMENT_ID}';`,
    'e2e_effects_hunger_resolve.sql', { tuples: true },
  ).split(/\r?\n/).map((s) => s.trim()).filter(Boolean).pop();
  if (!line) { console.log('FIXTURE NOT FOUND'); return finish(1); }
  const [planetID, worldID] = line.split('|');
  console.log('fixture planet=' + planetID + ' world=' + worldID);

  // --- Админ-аккаунт: регистрация + роль admin + перелогин за свежим JWT ---
  // Тест-юзеров этого префикса убираем при выходе (в т.ч. оставшихся от прошлых
  // прерванных прогонов).
  cleanupFns.push(() => psqlRun(`DELETE FROM users WHERE username LIKE '${USER_PREFIX}%';\n`, 'e2e_effects_hunger_cleanup.sql'));
  let token = null, username = null;
  for (let i = 0; i < 3; i++) {
    username = USER_PREFIX + Date.now() + '_' + i;
    const res = await fetch(BASE_URL + '/register', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password: PASSWORD }),
    });
    if (res.status === 201) { token = (await res.json()).token; break; }
    if (res.status !== 409) throw new Error('register HTTP ' + res.status);
  }
  if (!token) throw new Error('register failed');
  const me = await (await fetch(BASE_URL + '/me', { headers: { Authorization: 'Bearer ' + token } })).json();
  psqlRun(`UPDATE users SET role='admin' WHERE id='${me.id}';\n`, 'e2e_effects_hunger_admin.sql');
  const lr = await fetch(BASE_URL + '/login', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password: PASSWORD }),
  });
  if (lr.ok) token = (await lr.json()).token;

  const exe = findExecutable();
  if (!exe) { console.log('NO BROWSER'); return finish(1); }
  browser = await chromium.launch({ executablePath: exe, headless: true, args: ['--no-sandbox'] });
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  // Игровая карта читает localStorage.token, админка — localStorage.adminToken.
  await context.addInitScript((t) => {
    try { localStorage.setItem('token', t); localStorage.setItem('adminToken', t); } catch (e) {}
  }, token);
  const page = await context.newPage();
  const pageErrors = [];
  page.on('pageerror', (e) => pageErrors.push(String(e.message || e)));

  // =================== 1) Карточка планеты → «Поселения» → «Эффекты» ===================
  await page.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded', timeout: 30000 });
  await page.waitForSelector('#mapCanvas', { timeout: 15000 });
  await page.waitForFunction(() => typeof window.openSystemModal === 'function', { timeout: 20000 })
    .catch(() => {});
  await page.waitForTimeout(1500);

  const info = await page.evaluate(async ({ base, wid, pid }) => {
    const auth = { Authorization: 'Bearer ' + localStorage.getItem('token') };
    const wRes = await fetch(base + '/worlds/' + wid, { headers: auth });
    const w = await wRes.json(); const ww = w.world || w;
    const pRes = await fetch(base + '/api/worlds/' + wid + '/planets', { headers: auth });
    const p = await pRes.json();
    const idx = p.planets.findIndex((x) => x.id === pid);
    return { x: ww.coord_x, y: ww.coord_y, name: ww.name || 'W', idx };
  }, { base: BASE_URL, wid: worldID, pid: planetID });
  if (info.idx < 0) { console.log('PLANET NOT FOUND'); return finish(1); }

  const hasModalFn = await page.evaluate(() => typeof window.openSystemModal === 'function');
  if (!hasModalFn) {
    console.log('OPEN MODAL FN MISSING pageErrors=' + pageErrors.length + ' :: ' + pageErrors.slice(0, 3).join(' | '));
    return finish(1);
  }

  await page.evaluate((s) => {
    window.openSystemModal(s.wid, s.name, '', null, null, { x: s.x, y: s.y });
  }, { wid: worldID, name: info.name, x: info.x, y: info.y });
  await page.waitForSelector('#system-modal-overlay', { timeout: 15000 });
  await page.waitForTimeout(1200);

  const rowClicked = await page.evaluate((idx) => {
    const row = document.querySelector(`#right-panel tr[data-index="${idx}"]`);
    if (!row) return false;
    row.click();
    return true;
  }, info.idx);
  if (!rowClicked) { console.log('ROW CLICK FAIL'); return finish(1); }
  await page.waitForTimeout(1000);

  const tabClicked = await page.evaluate(() => {
    const btn = document.querySelector('button.tab-btn[data-tab="settlements"]');
    if (!btn) return false;
    btn.click();
    return true;
  });
  if (!tabClicked) { console.log('SETTLEMENTS TAB NOT FOUND'); return finish(1); }
  await page.waitForTimeout(1000);

  const effects = await page.evaluate(() => {
    const panel = document.getElementById('right-panel');
    const txt = panel ? panel.textContent : '';
    const m = txt.match(/Эффекты \((\d+)\)/);
    const form = panel ? panel.querySelector('[data-effect-load-form]') : null;
    return {
      hasBlock: !!m,
      count: m ? Number(m[1]) : 0,
      hasLoad: /нагрузка:/i.test(txt),
      hasThreshold: /порог:/i.test(txt),
      hasForm: !!form,
      hasSetBtn: !!(form && form.querySelector('[data-effect-load-set]')),
      toastError: !!document.querySelector('.toast-error'),
      path: location.pathname,
    };
  });
  const effectsOK = effects.hasBlock && effects.count >= 1 && effects.hasLoad &&
    effects.hasThreshold && effects.hasForm && effects.hasSetBtn &&
    !effects.toastError && effects.path !== '/login-page' && pageErrors.length === 0;
  report('settlement-effects-block', effectsOK,
    `block=${effects.hasBlock} count=${effects.count} load=${effects.hasLoad} ` +
    `threshold=${effects.hasThreshold} form=${effects.hasForm} set=${effects.hasSetBtn} ` +
    `toast=${effects.toastError} path=${effects.path} pageErrors=${pageErrors.length}`);
  await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'qa-effects-hunger-settlement.png') });
  if (!effectsOK) return finish(1);

  // =================== 2) Балансировка: компонента «Голод» + recovery ===================
  await page.goto(BASE_URL + '/admin', { waitUntil: 'domcontentloaded', timeout: 30000 });
  await page.waitForSelector('#balancerComponent', { state: 'attached', timeout: 15000 });
  await page.evaluate(() => {
    const btn = document.querySelector('.tab-btn[data-tab="tab-balancer"]');
    if (btn) btn.click();
  });
  await page.waitForTimeout(1200);
  // селектор расы наполняется асинхронно
  await page.waitForFunction(() => document.querySelectorAll('#balancerRace option').length > 1, { timeout: 10000 }).catch(() => {});

  await page.evaluate(() => {
    const s = document.getElementById('balancerComponent');
    s.value = 'hunger';
    s.dispatchEvent(new Event('change'));
  });
  await page.waitForTimeout(1000);
  const hunger = await page.evaluate(() => {
    const row = document.getElementById('balancerRecoveryRow');
    return {
      rowVisible: getComputedStyle(row).display !== 'none',
      value: document.getElementById('balancerRecovery').value,
      selectValue: document.getElementById('balancerComponent').value,
      optText: document.querySelector('#balancerComponent option[value="hunger"]').textContent,
    };
  });
  const hungerOK = hunger.rowVisible && hunger.selectValue === 'hunger' &&
    Math.abs(Number(hunger.value) - 0.25) < 1e-9 &&
    /нагрузк/i.test(hunger.optText);
  report('balancer-hunger-ui', hungerOK,
    `rowVisible=${hunger.rowVisible} value=${hunger.value} select=${hunger.selectValue} ` +
    `optLoad=${/нагрузк/i.test(hunger.optText)}`);
  await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'qa-effects-hunger-balancer.png') });
  if (!hungerOK) return finish(1);

  // Изменить recovery → Сохранить → перезагрузка → значение сохранилось.
  await page.evaluate(() => {
    document.getElementById('balancerRecovery').value = '0.33';
    document.getElementById('balancerSaveBtn').click();
  });
  await page.waitForTimeout(1500);
  await page.reload({ waitUntil: 'domcontentloaded', timeout: 30000 });
  await page.waitForSelector('#balancerComponent', { state: 'attached', timeout: 15000 });
  await page.waitForTimeout(1200);
  await page.evaluate(() => {
    const s = document.getElementById('balancerComponent');
    if (s.value !== 'hunger') { s.value = 'hunger'; s.dispatchEvent(new Event('change')); }
  });
  await page.waitForTimeout(1000);
  const persisted = await page.evaluate(() => document.getElementById('balancerRecovery').value);
  const persistOK = Math.abs(Number(persisted) - 0.33) < 1e-9;
  report('balancer-recovery-persist', persistOK, `after reload value=${persisted}`);
  if (!persistOK) return finish(1);

  // =================== 3) Раса → «Голод» недоступен ===================
  const racePicked = await page.evaluate(() => {
    const r = document.getElementById('balancerRace');
    const opt = [...r.options].find((o) => o.value);
    if (!opt) return null;
    r.value = opt.value;
    r.dispatchEvent(new Event('change'));
    return opt.value;
  });
  await page.waitForTimeout(1200);
  const raceState = await page.evaluate(() => ({
    hungerDisabled: document.querySelector('#balancerComponent option[value="hunger"]').disabled,
    component: document.getElementById('balancerComponent').value,
    rowVisible: getComputedStyle(document.getElementById('balancerRecoveryRow')).display !== 'none',
  }));
  const raceOK = !!racePicked && raceState.hungerDisabled &&
    raceState.component !== 'hunger' && !raceState.rowVisible;
  report('balancer-hunger-race-excluded', raceOK,
    `race=${racePicked ? 'yes' : 'none'} hungerDisabled=${raceState.hungerDisabled} ` +
    `component=${raceState.component} recoveryRow=${raceState.rowVisible}`);

  // Возврат в глобальный режим и восстановление заводского 0.25 (не мутируем store).
  await page.evaluate(() => {
    const r = document.getElementById('balancerRace');
    r.value = '';
    r.dispatchEvent(new Event('change'));
  });
  await page.waitForTimeout(1000);
  await page.evaluate(() => {
    const s = document.getElementById('balancerComponent');
    s.value = 'hunger';
    s.dispatchEvent(new Event('change'));
  });
  await page.waitForTimeout(1000);
  await page.evaluate(() => {
    document.getElementById('balancerRecovery').value = '0.25';
    document.getElementById('balancerSaveBtn').click();
  });
  await page.waitForTimeout(1200);

  const failed = results.filter((r) => !r.ok).length;
  console.log('RESULT: ' + (failed ? 'FAIL' : 'PASS') + ' (' + results.length + ' checks, ' + failed + ' failed)');
  return finish(failed ? 1 : 0);
}

main().catch((e) => { console.log('RUN FAIL ' + String(e)); finish(1); });
