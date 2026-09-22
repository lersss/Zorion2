// tools/e2e/qa-i3-admin-deposits.js (temporary QA smoke, iteration 3)
// Admin browser smoke: open system W, planet card, tab "Залежи" -> renders
// deposits incl. "выработано" for a zero deposit. Asserts 0 page errors,
// 0 toast errors, no login redirect. ASCII console output.
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const ARTIFACTS_DIR = path.join(path.dirname(fileURLToPath(import.meta.url)), 'artifacts');
const WORLD_ID = process.env.QA_WORLD || '6f55ca4f-09e6-4d78-93f9-0f9b02c88ac6';
const PLANET_ID = process.env.QA_PLANET || '5fa237ea-ed36-4954-b2d5-c3afa90eed8d';

const CHROME_PATHS = [process.env.CHROME_PATH, 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'].filter(Boolean);
const EDGE_PATHS = [process.env.EDGE_PATH, 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'].filter(Boolean);

function findExecutable() {
  for (const p of CHROME_PATHS) if (existsSync(p)) return { path: p, name: 'Chrome' };
  for (const p of EDGE_PATHS) if (existsSync(p)) return { path: p, name: 'Edge' };
  return null;
}

let browser = null;
async function finish(code) { if (browser) await browser.close().catch(() => {}); process.exit(code); }

async function main() {
  mkdirSync(ARTIFACTS_DIR, { recursive: true });
  const login = await fetch(BASE_URL + '/login', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: 'qa_i3_admin', password: 'pass12345' }),
  });
  if (!login.ok) { console.log('LOGIN FAIL ' + login.status); return finish(1); }
  const token = (await login.json()).token;

  const exe = findExecutable();
  if (!exe) { console.log('NO BROWSER'); return finish(1); }
  browser = await chromium.launch({ executablePath: exe.path, headless: true });
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  await context.addInitScript((t) => { localStorage.setItem('token', t); }, token);
  const page = await context.newPage();
  const pageErrors = [];
  page.on('pageerror', (e) => pageErrors.push(String(e.message || e)));

  await page.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded', timeout: 30000 });
  await page.waitForSelector('#mapCanvas', { timeout: 15000 });
  await page.waitForTimeout(1500);

  // world coords + planet index (admin sees all planets)
  const info = await page.evaluate(async ({ base, wid, pid }) => {
    const auth = { Authorization: 'Bearer ' + localStorage.getItem('token') };
    const wRes = await fetch(base + '/worlds/' + wid, { headers: auth });
    const w = await wRes.json(); const ww = w.world || w;
    const pRes = await fetch(base + '/api/worlds/' + wid + '/planets', { headers: auth });
    const p = await pRes.json();
    const idx = p.planets.findIndex(x => x.id === pid);
    return { x: ww.coord_x, y: ww.coord_y, name: (ww.name || 'W'), idx, planets: p.planets.length };
  }, { base: BASE_URL, wid: WORLD_ID, pid: PLANET_ID });

  if (info.idx < 0) { console.log('PLANET NOT FOUND'); return finish(1); }

  await page.evaluate((s) => {
    window.openSystemModal(s.wid, s.name, '', null, null, { x: s.x, y: s.y });
  }, { wid: WORLD_ID, name: info.name, x: info.x, y: info.y });
  await page.waitForSelector('#system-modal-overlay', { timeout: 15000 });
  await page.waitForTimeout(1500);

  // click the planet row (data-index in the merged objects table)
  const rowClicked = await page.evaluate((idx) => {
    const row = document.querySelector(`#right-panel tr[data-index="${idx}"]`);
    if (!row) return false;
    row.click();
    return true;
  }, info.idx);
  if (!rowClicked) { console.log('ROW CLICK FAIL'); return finish(1); }
  await page.waitForTimeout(1000);

  // open the "Залежи" tab
  const tabClicked = await page.evaluate(() => {
    const btn = document.querySelector('button.tab-btn[data-tab="deposits"]');
    if (!btn) return false;
    btn.click();
    return true;
  });
  if (!tabClicked) { console.log('TAB NOT FOUND'); return finish(1); }
  await page.waitForTimeout(800);

  const state = await page.evaluate(() => {
    const panel = document.getElementById('right-panel');
    const txt = panel ? panel.textContent : '';
    return {
      hasDepositsTab: txt.includes('Залежи') || txt.includes('Залежей нет'),
      hasVyrabotano: /выработано\s*:\s*1/.test(txt),
      hasAddForm: !!panel.querySelector('[data-add-deposit], form'),
      toastError: !!document.querySelector('.toast-error'),
      path: location.pathname,
      snippet: txt.replace(/\s+/g, ' ').slice(0, 260),
    };
  });

  const ok = state.hasDepositsTab && state.hasVyrabotano && !state.toastError &&
    state.path !== '/login-page' && pageErrors.length === 0;
  console.log('depositsTab=' + state.hasDepositsTab + ' vyrabotano=' + state.hasVyrabotano +
    ' toastError=' + state.toastError + ' path=' + state.path + ' pageErrors=' + pageErrors.length);
  console.log('snippet: ' + state.snippet);
  await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'qa-i3-admin-deposits.png') });
  console.log('RESULT: ' + (ok ? 'PASS' : 'FAIL'));
  return finish(ok ? 0 : 1);
}

main().catch((e) => { console.log('RUN FAIL ' + String(e)); finish(1); });
