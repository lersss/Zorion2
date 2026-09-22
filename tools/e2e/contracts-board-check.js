// tools/e2e/contracts-board-check.js
// Browser smoke for the planet-card "Задания/Контракты" tab (spec
// 2026-09-22-контракт-перелёт-и-доска §2-§3, stage B3): the tab opens without
// JS errors / toast errors / logout, the board request hits the server, the
// planet card and tab render, and the take button actually fires a 200
// (7 steps). Publish form visibility follows the player's position (planet
// orbit/surface only) - checked informationally.
//
// Run: node contracts-board-check.js   (BASE_URL env overrides the default)
// Exit code 0 = all PASS (SKIP allowed), 1 = any FAIL. ASCII output on purpose.
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const ARTIFACTS_DIR = path.join(path.dirname(fileURLToPath(import.meta.url)), 'artifacts');

const CHROME_PATHS = [
  process.env.CHROME_PATH,
  'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe',
].filter(Boolean);
const EDGE_PATHS = [
  process.env.EDGE_PATH,
  'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe',
].filter(Boolean);

const results = [];
function report(stepName, status, detail) {
  results.push({ stepName, status, detail });
  console.log(`[${stepName}] ${status}${detail ? ' - ' + detail : ''}`);
}

function findExecutable() {
  for (const p of CHROME_PATHS) if (existsSync(p)) return { path: p, name: 'Chrome' };
  for (const p of EDGE_PATHS) if (existsSync(p)) return { path: p, name: 'Edge' };
  return null;
}

async function registerAndLogin() {
  for (let attempt = 0; attempt < 3; attempt++) {
    const username = 'e2ec_' + Date.now() + '_' + attempt;
    const password = 'e2e-pass-' + Date.now();
    const res = await fetch(BASE_URL + '/register', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password }),
    });
    if (res.status === 201) {
      const data = await res.json();
      return { username, token: data.token };
    }
    if (res.status === 409) continue;
    throw new Error('register HTTP ' + res.status + ': ' + (await res.text()));
  }
  throw new Error('register: name collision after 3 attempts');
}

let browser = null;
async function finish(code) {
  if (browser) await browser.close().catch(() => {});
  process.exit(code);
}

async function main() {
  mkdirSync(ARTIFACTS_DIR, { recursive: true });
  console.log('BASE_URL: ' + BASE_URL);

  let token = null;
  try {
    const creds = await registerAndLogin();
    token = creds.token;
    report('1/7 register/login', 'PASS', 'user ' + creds.username);
  } catch (err) {
    report('1/7 register/login', 'FAIL', String(err && err.message ? err.message : err));
    return finish(1);
  }

  const exe = findExecutable();
  if (!exe) { report('setup browser', 'FAIL', 'no Chrome/Edge found'); return finish(1); }
  console.log('browser: ' + exe.name + ' (' + exe.path + ')');

  try {
    browser = await chromium.launch({ executablePath: exe.path, headless: true });
  } catch (err) {
    report('setup browser', 'FAIL', 'launch: ' + String(err && err.message ? err.message : err));
    return finish(1);
  }

  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  await context.addInitScript((t) => { localStorage.setItem('token', t); }, token);
  const page = await context.newPage();

  const pageErrors = [];
  page.on('pageerror', (err) => pageErrors.push(String(err && err.message ? err.message : err)));
  // Board request statuses: GET /api/planets/{id}/contracts (200 known, 403 unknown).
  let boardStatuses = [];
  page.on('response', (res) => {
    if (/\/api\/planets\/[^/]+\/contracts$/.test(res.url())) boardStatuses.push(res.status());
  });
  // Take requests: POST /api/contracts/take (regression for the dead "Взять"
  // button - handlers must be attached after the async board render).
  let takeStatuses = [];
  page.on('response', (res) => {
    if (/\/api\/contracts\/take$/.test(res.url())) takeStatuses.push(res.status());
  });

  try {
    // --- Step 2: open map, click own star -> modal ---
    await page.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.waitForSelector('#mapCanvas', { timeout: 15000 });
    await page.waitForFunction(() => {
      const el = document.getElementById('loading');
      return !el || el.style.display === 'none';
    }, { timeout: 30000 });
    await page.waitForTimeout(1000);

    const clickTarget = await page.evaluate(async (baseUrl) => {
      const canvas = document.getElementById('mapCanvas');
      const rect = canvas.getBoundingClientRect();
      const cw = canvas.width, ch = canvas.height;
      const token = localStorage.getItem('token');
      const auth = { Authorization: 'Bearer ' + token };

      let vp = null;
      try { vp = JSON.parse(sessionStorage.getItem('viewport') || 'null'); } catch (e) { vp = null; }

      let wx = 0, wy = 0;
      const meRes = await fetch(baseUrl + '/me', { headers: auth });
      if (meRes.ok) {
        const me = await meRes.json();
        if (me && me.current_world_id) {
          const wRes = await fetch(baseUrl + '/worlds/' + me.current_world_id, { headers: auth });
          if (wRes.ok) {
            const w = await wRes.json();
            const ww = w.world || w;
            if (typeof ww.coord_x === 'number') { wx = ww.coord_x; wy = ww.coord_y; }
          }
        }
      }

      const scale = vp && vp.scale ? vp.scale : 1.0;
      const offsetX = vp ? vp.offsetX : cw / 2 - wx * scale;
      const offsetY = vp ? vp.offsetY : ch / 2 - wy * scale;

      const inv = 1 / scale;
      const params = new URLSearchParams({
        x_min: (-offsetX * inv).toFixed(3),
        x_max: ((cw - offsetX) * inv).toFixed(3),
        y_min: (-offsetY * inv).toFixed(3),
        y_max: ((ch - offsetY) * inv).toFixed(3),
        cell: (40 / scale).toFixed(3),
      });
      const res = await fetch(baseUrl + '/api/worlds/filter?' + params.toString(), { headers: auth });
      if (!res.ok) return { ok: false, reason: 'filter HTTP ' + res.status };
      const clusters = await res.json();

      const cx = cw / 2, cy = ch / 2;
      let best = null, bestD = Infinity;
      for (const c of clusters) {
        const px = c.x * scale + offsetX;
        const py = c.y * scale + offsetY;
        const d = (px - cx) * (px - cx) + (py - cy) * (py - cy);
        if (d < bestD) { bestD = d; best = { x: px, y: py, sid: c.sid, cnt: c.cnt, name: c.sname }; }
      }
      if (!best) return { ok: false, reason: 'no clusters in viewport' };
      return { ok: true, clickX: rect.left + best.x * (rect.width / cw), clickY: rect.top + best.y * (rect.height / ch), star: best };
    }, BASE_URL);

    if (!clickTarget.ok) {
      report('2/7 open own modal', 'SKIP', clickTarget.reason);
    } else {
      await page.mouse.click(clickTarget.clickX, clickTarget.clickY);
      await page.waitForSelector('#system-modal-overlay', { timeout: 15000 });
      await page.waitForTimeout(2000);
      const modal = await page.evaluate(() => {
        const panel = document.getElementById('right-panel');
        const rows = panel ? panel.querySelectorAll('tr[data-index]').length : 0;
        return {
          overlay: !!document.getElementById('system-modal-overlay'),
          rows,
          toastError: !!document.querySelector('.toast-error'),
          path: location.pathname,
        };
      });
      const errs = pageErrors.slice();
      const ok2 = modal.overlay && modal.rows > 0 && !modal.toastError &&
        modal.path !== '/login-page' && errs.length === 0;
      report('2/7 open own modal', ok2 ? 'PASS' : 'FAIL',
        `rows=${modal.rows} toastError=${modal.toastError} pageErrors=${errs.length}` +
        (errs.length ? ' first: ' + errs[0] : ''));
      if (!ok2) return finish(1);
    }

    // --- Step 3: open a planet card ---
    const card = await page.evaluate(async () => {
      const panel = document.getElementById('right-panel');
      const tr = panel && panel.querySelector('tr[data-index]');
      if (!tr) return { ok: false, reason: 'no planet row' };
      tr.click();
      await new Promise(r => setTimeout(r, 1200));
      const tabs = [...panel.querySelectorAll('.tab-btn')].map(b => b.dataset.tab);
      return {
        ok: true,
        tabs,
        hasContractsTab: tabs.includes('contracts'),
        toastError: !!document.querySelector('.toast-error'),
      };
    });
    const ok3 = card.ok && card.hasContractsTab && !card.toastError;
    report('3/7 planet card tab', ok3 ? 'PASS' : 'FAIL',
      `tabs=[${(card.tabs || []).join(',')}] hasContracts=${card.hasContractsTab} toastError=${card.toastError}` +
      (card.ok ? '' : ' reason=' + card.reason));
    if (!ok3) return finish(1);

    // --- Step 4: open the "Задания/Контракты" tab ---
    const tab = await page.evaluate(async () => {
      const panel = document.getElementById('right-panel');
      const btn = panel && panel.querySelector('.tab-btn[data-tab="contracts"]');
      if (!btn) return { ok: false, reason: 'no contracts tab button' };
      btn.click();
      await new Promise(r => setTimeout(r, 1500)); // board fetch + render
      const content = panel.querySelector('#tab-content');
      const text = content ? content.textContent : '';
      return {
        ok: true,
        rendered: !!content && text.length > 0,
        hasBoardHeader: text.includes('Контракты') || text.includes('Контрактов нет') || text.includes('Нет данных'),
        hasPublish: !!content.querySelector('[data-contract-publish]'),
        hasPublishHint: text.includes('Опубликовать контракт можно только с планеты'),
        toastError: !!document.querySelector('.toast-error'),
        path: location.pathname,
        hasToken: !!localStorage.getItem('token'),
      };
    });
    const ok4 = tab.ok && tab.rendered && tab.hasBoardHeader && !tab.toastError &&
      tab.path !== '/login-page' && tab.hasToken;
    report('4/7 contracts tab renders', ok4 ? 'PASS' : 'FAIL',
      `rendered=${tab.rendered} boardHeader=${tab.hasBoardHeader} publish=${tab.hasPublish} ` +
      `publishHint=${tab.hasPublishHint} toastError=${tab.toastError} path=${tab.path}` +
      (tab.ok ? '' : ' reason=' + tab.reason));
    if (!ok4) return finish(1);
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'contracts-tab.png') });

    // --- Step 5: "Взять" button is wired after the async board render ---
    // Regression for the blocking defect: initContracts attached handlers before
    // loadContracts filled the board, so the button was dead. Publish a contract
    // via API (fly to the planet first), reopen the board, click "Взять", and
    // assert POST /api/contracts/take actually fired.
    const prep = await page.evaluate(async (baseUrl) => {
      const token = localStorage.getItem('token');
      const auth = { Authorization: 'Bearer ' + token };
      const meRes = await fetch(baseUrl + '/me', { headers: auth });
      const me = await meRes.json();
      const wid = me && me.current_world_id;
      if (!wid) return { ok: false, reason: 'no current world' };
      const pRes = await fetch(baseUrl + '/api/worlds/' + wid + '/planets', { headers: auth });
      if (!pRes.ok) return { ok: false, reason: 'planets HTTP ' + pRes.status };
      const data = await pRes.json();
      const planets = data.planets || data;
      if (!planets || !planets.length) return { ok: false, reason: 'no planets' };
      const pid = planets[0].id;
      // Fly to the planet so publication is allowed (spec §3: planet only).
      const fRes = await fetch(baseUrl + '/api/intrasystem-flight', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + token },
        body: JSON.stringify({ object_type: 'planet', object_id: pid }),
      });
      if (!fRes.ok) return { ok: false, reason: 'flight HTTP ' + fRes.status + ': ' + (await fRes.text()) };
      const f = await fRes.json();
      return { ok: true, pid, arrive_at: f.arrive_at };
    }, BASE_URL);

    if (!prep.ok) {
      report('5/7 take button', 'SKIP', prep.reason);
    } else {
      const waitMs = Math.max(0, new Date(prep.arrive_at).getTime() - Date.now()) + 2500;
      await page.waitForTimeout(waitMs);
      // Publish a contract on the planet via API (the board must show a row).
      const pub = await page.evaluate(async ({ baseUrl, pid }) => {
        const token = localStorage.getItem('token');
        const auth = { Authorization: 'Bearer ' + token };
        const meRes = await fetch(baseUrl + '/me', { headers: auth });
        const me = await meRes.json();
        const res = await fetch(baseUrl + '/api/contracts', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + token },
          body: JSON.stringify({
            planet_id: pid, type: 'travel', title: 'e2e take probe', reward: 100,
            payload: { from_world_id: me.current_world_id, dest_world_id: me.current_world_id, dest_planet_id: null },
          }),
        });
        if (!res.ok) return { ok: false, reason: 'publish HTTP ' + res.status + ': ' + (await res.text()) };
        const c = await res.json();
        return { ok: true, id: c.id };
      }, { baseUrl: BASE_URL, pid: prep.pid });

      if (!pub.ok) {
        report('5/7 take button', 'SKIP', pub.reason);
      } else {
        // Reopen the modal so my_position is fresh, then open the board tab.
        await page.keyboard.press('Escape');
        await page.waitForTimeout(400);
        await page.mouse.click(clickTarget.clickX, clickTarget.clickY);
        await page.waitForSelector('#system-modal-overlay', { timeout: 15000 });
        await page.waitForTimeout(1500);
        const clicked = await page.evaluate(async () => {
          const panel = document.getElementById('right-panel');
          const tr = panel && panel.querySelector('tr[data-index]');
          if (!tr) return { ok: false, reason: 'no planet row' };
          tr.click();
          await new Promise(r => setTimeout(r, 1000));
          const btn = panel.querySelector('.tab-btn[data-tab="contracts"]');
          if (!btn) return { ok: false, reason: 'no contracts tab' };
          btn.click();
          // Wait for the async board render to place the take button.
          let takeBtn = null;
          for (let i = 0; i < 30 && !takeBtn; i++) {
            await new Promise(r => setTimeout(r, 200));
            takeBtn = panel.querySelector('[data-contract-take]');
          }
          if (!takeBtn) return { ok: false, reason: 'no take button after board load' };
          takeBtn.click();
          await new Promise(r => setTimeout(r, 1500));
          return { ok: true };
        });
        const ok5 = clicked.ok && takeStatuses.length > 0 && takeStatuses.every(s => s === 200);
        report('5/7 take button', ok5 ? 'PASS' : 'FAIL',
          `clicked=${clicked.ok} takeStatuses=[${takeStatuses.join(',')}]` +
          (clicked.ok ? '' : ' reason=' + clicked.reason));
        if (!ok5) return finish(1);
      }
    }

    // --- Step 6: board request hit the server (200 known / 403 unknown) ---
    const ok6 = boardStatuses.length > 0 && boardStatuses.every(s => s === 200 || s === 403);
    report('6/7 board request', ok6 ? 'PASS' : 'FAIL',
      `statuses=[${boardStatuses.join(',')}]`);
    if (!ok6) return finish(1);

    // --- Step 7: no page errors overall ---
    const errs = pageErrors.slice();
    const ok7 = errs.length === 0;
    report('7/7 no page errors', ok7 ? 'PASS' : 'FAIL', errs.length ? errs.join(' | ') : '0 errors');
    if (!ok7) return finish(1);
  } catch (err) {
    report('run', 'FAIL', String(err && err.message ? err.message : err));
    return finish(1);
  }

  const failed = results.filter(r => r.status === 'FAIL');
  console.log('');
  console.log('RESULT: ' + (failed.length ? 'FAIL (' + failed.length + ')' : 'ALL PASS'));
  return finish(failed.length ? 1 : 0);
}

main();
