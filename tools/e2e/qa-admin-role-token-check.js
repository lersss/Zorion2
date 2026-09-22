// tools/e2e/qa-admin-role-token-check.js
// Independent QA run: admin UI gate must use the role FROM THE TOKEN (/me),
// not from the DB. Four scenarios:
//   1. fresh admin (regression)  -> all tabs, no 4xx /admin/*, no toast-error,
//      "Users" tab hidden.
//   2. stale token (role raised in DB after token was issued) -> login overlay,
//      zero /admin/* requests, zero 403, zero toasts.
//   3. re-login on the same account -> fresh token, all tabs, no 403.
//   4. skycomposer (regression) -> "Users" tab visible, GET /admin/users 200,
//      other tabs no 403.
// Tokens come from a JSON file (QA_TOKENS env or %TEMP%\opencode\qa-role-tokens.json):
// { u1, u2, u4, pw, stale_token, t1, t4 }.
// ASCII console output (PowerShell cp866 breaks Cyrillic).
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync, readFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://127.0.0.1:8080').replace(/\/+$/, '');
const HERE = path.dirname(fileURLToPath(import.meta.url));
const ARTIFACTS_DIR = path.join(HERE, 'artifacts');
const TOKENS_FILE = process.env.QA_TOKENS ||
  path.join(process.env.TEMP || process.env.TMP || '.', 'opencode', 'qa-role-tokens.json');

const CHROME_PATHS = [process.env.CHROME_PATH, 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'].filter(Boolean);
const EDGE_PATHS = [process.env.EDGE_PATH, 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'].filter(Boolean);

function findExecutable() {
  for (const p of CHROME_PATHS) if (existsSync(p)) return { path: p, name: 'Chrome' };
  for (const p of EDGE_PATHS) if (existsSync(p)) return { path: p, name: 'Edge' };
  return null;
}

const T = JSON.parse(readFileSync(TOKENS_FILE, 'utf8').replace(/^\uFEFF/, ''));

let browser = null;
async function finish(code) { if (browser) await browser.close().catch(() => {}); process.exit(code); }

const INIT = (token) => {
  window.__toastErrors = [];
  document.addEventListener('DOMContentLoaded', () => {
    const obs = new MutationObserver((muts) => {
      for (const m of muts) for (const n of m.addedNodes) {
        if (n && n.nodeType === 1 && n.classList && n.classList.contains('toast-error')) {
          window.__toastErrors.push((n.textContent || '').trim().slice(0, 200));
        }
      }
    });
    obs.observe(document.body, { childList: true, subtree: true });
  });
  if (token) localStorage.setItem('adminToken', token);
  else localStorage.removeItem('adminToken');
  localStorage.removeItem('adminActiveTab');
};

async function openAdmin(token) {
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
  await ctx.addInitScript(INIT, token || '');
  const page = await ctx.newPage();
  const state = { subReqs: [], bad: [], pageErrors: [], consoleErrors: [] };
  page.on('response', (res) => {
    let u; try { u = new URL(res.url()); } catch { return; }
    if (u.pathname === '/admin' || u.pathname.startsWith('/admin/')) {
      if (res.request().resourceType() === 'document') return; // the /admin page itself
      const label = res.status() + ' ' + u.pathname + (u.search || '');
      state.subReqs.push(label);
      if (res.status() >= 400) state.bad.push(label);
    }
  });
  page.on('pageerror', (e) => state.pageErrors.push(String((e && e.message) || e).slice(0, 200)));
  page.on('console', (m) => { if (m.type() === 'error') state.consoleErrors.push(m.text().slice(0, 200)); });
  await page.goto(BASE_URL + '/admin', { waitUntil: 'domcontentloaded', timeout: 30000 });
  return { ctx, page, state };
}

async function overlayDisplay(page) {
  return page.evaluate(() => {
    const o = document.getElementById('adminLoginOverlay');
    return o ? getComputedStyle(o).display : 'missing';
  });
}

async function waitOverlay(page, mode, timeout = 15000) {
  await page.waitForFunction((m) => {
    const o = document.getElementById('adminLoginOverlay');
    if (!o) return false;
    const d = getComputedStyle(o).display;
    return m === 'hidden' ? d === 'none' : d !== 'none';
  }, mode, { timeout });
}

async function clickAllTabs(page) {
  const tabs = await page.evaluate(() => Array.from(document.querySelectorAll('.tab-btn'))
    .filter((b) => getComputedStyle(b).display !== 'none')
    .map((b) => b.dataset.tab));
  for (const id of tabs) {
    await page.evaluate((t) => {
      const b = document.querySelector('.tab-btn[data-tab="' + t + '"]');
      if (b) b.click();
    }, id);
    await page.waitForTimeout(900);
  }
  await page.waitForTimeout(1200);
  return tabs;
}

function line(step, ok, details) {
  console.log((ok ? 'PASS' : 'FAIL') + ' | ' + step + ' | ' + details);
  if (!ok) console.log('DETAIL: ' + details);
}

async function main() {
  mkdirSync(ARTIFACTS_DIR, { recursive: true });
  const exe = findExecutable();
  if (!exe) { console.log('NO BROWSER'); return finish(1); }
  console.log('browser: ' + exe.name + ' | base: ' + BASE_URL);
  browser = await chromium.launch({ executablePath: exe.path, headless: true });

  // ---------------- Step 1: fresh admin (regression) ----------------
  {
    const { ctx, page, state } = await openAdmin(T.t1);
    await waitOverlay(page, 'hidden');
    await page.waitForTimeout(2000);
    const usersHidden = await page.evaluate(() => {
      const b = document.getElementById('tab-users-btn');
      return !!b && getComputedStyle(b).display === 'none';
    });
    const tabs = await clickAllTabs(page);
    const toasts = await page.evaluate(() => window.__toastErrors.slice());
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'qa-admin-role-1-fresh-admin.png'), fullPage: false });
    const ok = state.bad.length === 0 && toasts.length === 0 && usersHidden;
    line('1 fresh admin (regression)', ok,
      'tabs=' + tabs.length + ' bad4xx=' + JSON.stringify(state.bad) + ' toastErrors=' + JSON.stringify(toasts) +
      ' usersTabHidden=' + usersHidden + ' adminReqs=' + state.subReqs.length +
      ' pageErrors=' + JSON.stringify(state.pageErrors));
    await ctx.close();
    if (!ok) return finish(1);
  }

  // ---------------- Step 2: stale token ----------------
  {
    const { ctx, page, state } = await openAdmin(T.stale_token);
    await page.waitForTimeout(3000);
    const disp = await overlayDisplay(page);
    const overlayShown = disp !== 'none' && disp !== 'missing';
    const toasts = await page.evaluate(() => window.__toastErrors.slice());
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'qa-admin-role-2-stale-token.png') });
    const ok = overlayShown && state.subReqs.length === 0 && state.bad.length === 0 && toasts.length === 0;
    line('2 stale token', ok,
      'overlayDisplay=' + disp + ' adminReqs=' + JSON.stringify(state.subReqs) +
      ' bad4xx=' + JSON.stringify(state.bad) + ' toastErrors=' + JSON.stringify(toasts) +
      ' pageErrors=' + JSON.stringify(state.pageErrors));
    if (!ok) { await ctx.close(); return finish(1); }

    // ---------------- Step 3: re-login (continues on the same page) ----------------
    await page.fill('#adminLoginUsername', T.u2);
    await page.fill('#adminLoginPassword', T.pw);
    await page.click('#adminLoginOverlay button.btn');
    await waitOverlay(page, 'hidden');
    await page.waitForTimeout(1500);
    state.subReqs.length = 0; state.bad.length = 0;
    const tabs = await clickAllTabs(page);
    const toasts3 = await page.evaluate(() => window.__toastErrors.slice());
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'qa-admin-role-3-relogin.png') });
    const ok3 = state.bad.length === 0 && toasts3.length === 0;
    line('3 re-login fresh token', ok3,
      'tabs=' + tabs.length + ' bad4xx=' + JSON.stringify(state.bad) + ' toastErrors=' + JSON.stringify(toasts3) +
      ' adminReqs=' + state.subReqs.length + ' pageErrors=' + JSON.stringify(state.pageErrors));
    await ctx.close();
    if (!ok3) return finish(1);
  }

  // ---------------- Step 4: skycomposer (regression) ----------------
  {
    const { ctx, page, state } = await openAdmin(T.t4);
    await waitOverlay(page, 'hidden');
    await page.waitForTimeout(2000);
    const usersVisible = await page.evaluate(() => {
      const b = document.getElementById('tab-users-btn');
      return !!b && getComputedStyle(b).display !== 'none';
    });
    // click "Users" first, wait for its request
    await page.evaluate(() => {
      const b = document.querySelector('.tab-btn[data-tab="tab-users"]');
      if (b) b.click();
    });
    await page.waitForTimeout(1500);
    const usersReq = state.subReqs.filter((s) => s.includes('/admin/users'));
    const usersOk = usersReq.some((s) => s.startsWith('200 '));
    const tabs = await clickAllTabs(page);
    const toasts = await page.evaluate(() => window.__toastErrors.slice());
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'qa-admin-role-4-skycomposer.png') });
    const ok = usersVisible && usersOk && state.bad.length === 0 && toasts.length === 0;
    line('4 skycomposer (regression)', ok,
      'usersTabVisible=' + usersVisible + ' usersReq=' + JSON.stringify(usersReq) +
      ' tabs=' + tabs.length + ' bad4xx=' + JSON.stringify(state.bad) + ' toastErrors=' + JSON.stringify(toasts) +
      ' pageErrors=' + JSON.stringify(state.pageErrors));
    await ctx.close();
    if (!ok) return finish(1);
  }

  console.log('ALL STEPS PASS');
  return finish(0);
}

main().catch((e) => { console.log('RUN FAIL ' + String(e && e.stack || e)); finish(1); });
