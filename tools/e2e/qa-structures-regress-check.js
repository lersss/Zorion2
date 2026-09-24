// tools/e2e/qa-structures-regress-check.js (temporary QA regress smoke for the
// "build structures" feature). Admin browser smoke: existing admin planet-card
// forms/tabs still render ("Залежи" form, settlement branch/effect forms,
// "Фракции" with capital, "Магазин"). Asserts 0 page errors. ASCII output.
// Creds from env QA_USER / QA_PASS.
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const ARTIFACTS_DIR = path.join(path.dirname(fileURLToPath(import.meta.url)), 'artifacts');
const WORLD_ID = process.env.QA_WORLD || '594d98ce-9158-4fad-97ef-2725976c13e7';
const PLANET_ID = process.env.QA_PLANET || '2d762191-2eb1-4564-bb18-db527a500492';
const USER = process.env.QA_USER;
const PASS = process.env.QA_PASS;

const CHROME_PATHS = [process.env.CHROME_PATH, 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'].filter(Boolean);
const EDGE_PATHS = [process.env.EDGE_PATH, 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'].filter(Boolean);
function findExecutable() {
  for (const p of CHROME_PATHS) if (existsSync(p)) return { path: p, name: 'Chrome' };
  for (const p of EDGE_PATHS) if (existsSync(p)) return { path: p, name: 'Edge' };
  return null;
}

let browser = null;
async function finish(code) { if (browser) await browser.close().catch(() => {}); process.exit(code); }

async function clickTab(page, key) {
  return await page.evaluate(async (k) => {
    const btn = document.querySelector(`button.tab-btn[data-tab="${k}"]`);
    if (!btn) return false;
    btn.click();
    await new Promise(r => setTimeout(r, 1000));
    return true;
  }, key);
}

async function main() {
  mkdirSync(ARTIFACTS_DIR, { recursive: true });
  const login = await fetch(BASE_URL + '/login', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: USER, password: PASS }),
  });
  if (!login.ok) { console.log('LOGIN FAIL ' + login.status); return finish(1); }
  const token = (await login.json()).token;

  const exe = findExecutable();
  if (!exe) { console.log('NO BROWSER'); return finish(1); }
  browser = await chromium.launch({ executablePath: exe.path, headless: true });
  const context = await browser.newContext({ viewport: { width: 1400, height: 900 } });
  await context.addInitScript((t) => { localStorage.setItem('token', t); }, token);
  const page = await context.newPage();
  const pageErrors = [];
  page.on('pageerror', (e) => pageErrors.push(String(e.message || e)));

  await page.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded', timeout: 30000 });
  await page.waitForSelector('#mapCanvas', { timeout: 15000 });
  await page.waitForTimeout(1500);

  const info = await page.evaluate(async ({ base, wid, pid }) => {
    const auth = { Authorization: 'Bearer ' + localStorage.getItem('token') };
    const wRes = await fetch(base + '/worlds/' + wid, { headers: auth });
    const w = await wRes.json(); const ww = w.world || w;
    const pRes = await fetch(base + '/api/worlds/' + wid + '/planets', { headers: auth });
    const p = await pRes.json();
    return { x: ww.coord_x, y: ww.coord_y, name: (ww.name || 'W'), idx: p.planets.findIndex(x => x.id === pid) };
  }, { base: BASE_URL, wid: WORLD_ID, pid: PLANET_ID });
  if (info.idx < 0) { console.log('PLANET NOT FOUND'); return finish(1); }

  await page.evaluate((s) => {
    window.openSystemModal(s.wid, s.name, '', null, null, { x: s.x, y: s.y });
  }, { wid: WORLD_ID, name: info.name, x: info.x, y: info.y });
  await page.waitForSelector('#system-modal-overlay', { timeout: 15000 });
  await page.waitForTimeout(1500);
  await page.evaluate((idx) => {
    const row = document.querySelector(`#right-panel tr[data-index="${idx}"]`);
    if (row) row.click();
  }, info.idx);
  await page.waitForTimeout(1200);

  const out = {};

  // deposits form
  out.depositsTab = await clickTab(page, 'deposits');
  out.deposits = await page.evaluate(() => {
    const p = document.getElementById('right-panel');
    const txt = p ? p.textContent : '';
    return { hasAdd: !!p.querySelector('#deposit-add-btn') && !!p.querySelector('#deposit-good-select'),
      label: txt.includes('Залеж') };
  });

  // settlements -> branch create form + effect load form (admin)
  out.settTab = await clickTab(page, 'settlements');
  out.settForms = await page.evaluate(() => {
    const p = document.getElementById('right-panel');
    return {
      branchCreate: !!p.querySelector('[data-branch-create]'),
      branchCreateForm: !!p.querySelector('[data-branch-create-form]'),
      effectLoad: !!p.querySelector('[data-effect-load-set]'),
      ownerLine: (p.textContent || '').includes('Владелец:'),
    };
  });
  await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'qa-structures-regress-sett.png') });

  // factions -> capital row
  out.factionsTab = await clickTab(page, 'factions');
  out.factions = await page.evaluate(() => {
    const p = document.getElementById('right-panel');
    const txt = p ? p.textContent : '';
    return { hasCapital: txt.includes('Столица'), text: txt.replace(/\s+/g, ' ').slice(0, 120) };
  });

  // market tab if visible
  out.marketClicked = await clickTab(page, 'market');
  out.market = await page.evaluate(() => {
    const p = document.getElementById('right-panel');
    return { present: !!document.querySelector('button.tab-btn[data-tab="market"]'),
      text: (p ? p.textContent : '').replace(/\s+/g, ' ').slice(0, 80) };
  });

  out.path = await page.evaluate(() => location.pathname);
  out.toastError = await page.evaluate(() => !!document.querySelector('.toast-error'));
  await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'qa-structures-regress.png') });

  const ok = out.depositsTab && out.deposits.hasAdd && out.deposits.label &&
    out.settTab && out.settForms.branchCreate && out.settForms.effectLoad && out.settForms.ownerLine &&
    out.factionsTab && out.factions.hasCapital && !out.toastError &&
    out.path !== '/login-page' && pageErrors.length === 0;

  console.log(JSON.stringify(out));
  console.log('pageErrors=' + pageErrors.length + ' RESULT: ' + (ok ? 'PASS' : 'FAIL'));
  return finish(ok ? 0 : 1);
}

main().catch((e) => { console.log('RUN FAIL ' + String(e)); finish(1); });
