// tools/e2e/qa-structures-admin-check.js (temporary QA smoke for the
// "build structures on a planet" feature, spec 2026-09-24). Admin browser
// smoke: planet card shows the "Построить" button + panel + "Строения" tab +
// owner row in a settlement card. Asserts 0 page errors, no login redirect.
// ASCII console output. Creds from env QA_USER / QA_PASS.
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
    const idx = p.planets.findIndex(x => x.id === pid);
    return { x: ww.coord_x, y: ww.coord_y, name: (ww.name || 'W'), idx, planets: p.planets.length };
  }, { base: BASE_URL, wid: WORLD_ID, pid: PLANET_ID });

  if (info.idx < 0) { console.log('PLANET NOT FOUND'); return finish(1); }

  await page.evaluate((s) => {
    window.openSystemModal(s.wid, s.name, '', null, null, { x: s.x, y: s.y });
  }, { wid: WORLD_ID, name: info.name, x: info.x, y: info.y });
  await page.waitForSelector('#system-modal-overlay', { timeout: 15000 });
  await page.waitForTimeout(1500);

  const rowClicked = await page.evaluate((idx) => {
    const row = document.querySelector(`#right-panel tr[data-index="${idx}"]`);
    if (!row) return false;
    row.click();
    return true;
  }, info.idx);
  if (!rowClicked) { console.log('ROW CLICK FAIL'); return finish(1); }
  await page.waitForTimeout(1200);

  // 1) build button in the card header
  const hasBuildBtn = await page.evaluate(() => !!document.querySelector('#build-structure-btn'));
  // 2) click -> panel opens with type select + optgroups + live=false mark
  const panel = await page.evaluate(async () => {
    const btn = document.querySelector('#build-structure-btn');
    if (btn) btn.click();
    await new Promise(r => setTimeout(r, 2500));
    const panelEl = document.querySelector('[data-build-panel]');
    const sel = document.querySelector('#build-type');
    const txt = panelEl ? panelEl.textContent : '';
    return {
      visible: panelEl ? panelEl.style.display !== 'none' : false,
      hasSelect: !!sel,
      optgroups: sel ? sel.querySelectorAll('optgroup').length : 0,
      options: sel ? sel.options.length : 0,
      hasNotImplemented: sel ? Array.from(sel.options).some(o => o.textContent.includes('производство ещё не реализовано')) : false,
      ownerRows: document.querySelectorAll('#build-owner-faction [data-owner-pick]').length,
      hasRaceHint: txt.includes('Раса:'),
    };
  });
  await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'qa-structures-panel.png') });

  // 3) owner search min-2 hint when switching to player
  const ownerSearch = await page.evaluate(async () => {
    const sel = document.querySelector('#build-owner-type');
    if (sel) { sel.value = 'player'; sel.dispatchEvent(new Event('change')); }
    await new Promise(r => setTimeout(r, 400));
    const wrap = document.querySelector('#build-owner-search-wrap');
    const res = document.querySelector('#build-owner-results');
    return {
      searchVisible: wrap ? wrap.style.display !== 'none' : false,
      hint: res ? res.textContent.trim() : '',
    };
  });

  // 4) "Строения" tab present, renders capital with "не производит"
  const structTab = await page.evaluate(async () => {
    const btn = document.querySelector('button.tab-btn[data-tab="structures"]');
    if (!btn) return { present: false };
    btn.click();
    await new Promise(r => setTimeout(r, 1200));
    const panel = document.getElementById('right-panel');
    const txt = panel ? panel.textContent : '';
    return {
      present: true,
      hasHeader: txt.includes('Строения'),
      hasNotProducing: txt.includes('не производит'),
      hasOwnerLine: txt.includes('Владелец:'),
    };
  });
  await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'qa-structures-tab.png') });

  // 5) "Поселения" tab -> owner line incl. NPC (no owner) for legacy rows
  const settTab = await page.evaluate(async () => {
    const btn = document.querySelector('button.tab-btn[data-tab="settlements"]');
    if (!btn) return { present: false };
    btn.click();
    await new Promise(r => setTimeout(r, 1200));
    const panel = document.getElementById('right-panel');
    const txt = panel ? panel.textContent : '';
    return {
      present: true,
      hasOwnerLine: txt.includes('Владелец:'),
      hasNpc: txt.includes('NPC (без владельца)'),
    };
  });
  await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'qa-structures-settlements.png') });

  const pathOk = await page.evaluate(() => location.pathname);
  const toastError = await page.evaluate(() => !!document.querySelector('.toast-error'));

  const ok = hasBuildBtn && panel.visible && panel.hasSelect && panel.optgroups >= 1 &&
    panel.hasNotImplemented && panel.ownerRows > 0 && panel.hasRaceHint &&
    ownerSearch.searchVisible && ownerSearch.hint.includes('Введите минимум 2 символа') &&
    structTab.present && structTab.hasHeader && structTab.hasNotProducing && structTab.hasOwnerLine &&
    settTab.present && settTab.hasOwnerLine && settTab.hasNpc &&
    !toastError && pathOk !== '/login-page' && pageErrors.length === 0;

  console.log('buildBtn=' + hasBuildBtn + ' panelVisible=' + panel.visible + ' hasSelect=' + panel.hasSelect +
    ' optgroups=' + panel.optgroups + ' options=' + panel.options + ' notImplemented=' + panel.hasNotImplemented +
    ' ownerRows=' + panel.ownerRows + ' raceHint=' + panel.hasRaceHint);
  console.log('ownerSearch visible=' + ownerSearch.searchVisible + ' hint=' + JSON.stringify(ownerSearch.hint));
  console.log('structTab present=' + structTab.present + ' header=' + structTab.hasHeader +
    ' notProducing=' + structTab.hasNotProducing + ' ownerLine=' + structTab.hasOwnerLine);
  console.log('settTab present=' + settTab.present + ' ownerLine=' + settTab.hasOwnerLine + ' npc=' + settTab.hasNpc);
  console.log('path=' + pathOk + ' toastError=' + toastError + ' pageErrors=' + pageErrors.length);
  console.log('RESULT: ' + (ok ? 'PASS' : 'FAIL'));
  return finish(ok ? 0 : 1);
}

main().catch((e) => { console.log('RUN FAIL ' + String(e)); finish(1); });
