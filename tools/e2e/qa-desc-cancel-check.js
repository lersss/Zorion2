// tools/e2e/qa-desc-cancel-check.js — независимый прогон @tester (2026-09-21):
// попап «Описания ИИ» — «Отмена» при простое = отказ от набора.
// Сценарий: API fill (good_ids:[1]) -> ждать предложения -> браузер /studio
// -> попап авто-открылся -> клик «Отмена» -> попап закрыт + отчёт
// «Набор предложений отброшен» + набор пуст -> F5 -> попап НЕ всплывает,
// набор пуст, pageerror=0. Скриншоты в tools/e2e/artifacts/qa-desc-cancel/.
// Output ASCII on purpose (Windows PowerShell cp866 breaks Cyrillic).
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://127.0.0.1:8080').replace(/\/+$/, '');
const USER = process.env.STUDIO_USER || '';
const PASS = process.env.STUDIO_PASS || '';
const GOOD_ID = process.env.QA_GOOD_ID || '1';
const ART = path.join(path.dirname(fileURLToPath(import.meta.url)), 'artifacts', 'qa-desc-cancel');

const CHROME_PATHS = [process.env.CHROME_PATH, 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'].filter(Boolean);
const EDGE_PATHS = [process.env.EDGE_PATH, 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'].filter(Boolean);
function findExe() {
  for (const p of CHROME_PATHS) if (existsSync(p)) return p;
  for (const p of EDGE_PATHS) if (existsSync(p)) return p;
  return null;
}

const results = [];
function report(step, status, detail) {
  results.push({ step, status });
  console.log(`[${step}] ${status}${detail ? ' - ' + detail : ''}`);
}

let token = '';
async function api(method, url, body) {
  const headers = { 'Content-Type': 'application/json' };
  if (token) headers['Authorization'] = 'Bearer ' + token;
  const r = await fetch(BASE_URL + url, { method, headers, body: body ? JSON.stringify(body) : undefined });
  let data = null;
  try { data = await r.json(); } catch (e) {}
  return { status: r.status, data };
}

const sleep = (ms) => new Promise(r => setTimeout(r, ms));

let browser = null;
async function finish(code) {
  if (browser) await browser.close().catch(() => {});
  const fails = results.filter(r => r.status === 'FAIL').length;
  console.log(`\nTOTAL: ${results.length} steps, ${fails} FAIL`);
  process.exit(code);
}

async function main() {
  mkdirSync(ART, { recursive: true });
  if (!USER || !PASS) { report('setup creds', 'FAIL', 'STUDIO_USER/STUDIO_PASS not set'); return finish(1); }
  const exe = findExe();
  if (!exe) { report('setup browser', 'FAIL', 'no Chrome/Edge'); return finish(1); }

  const login = await api('POST', '/login', { username: USER, password: PASS });
  if (login.status !== 200 || !login.data || !login.data.token) { report('login', 'FAIL', 'status=' + login.status); return finish(1); }
  token = login.data.token;
  report('login', 'PASS', 'role=' + (login.data.user && login.data.user.role));

  // --- server side: fill set (idle case) ---
  const st0 = await api('GET', '/studio/api/state');
  if (st0.data.desc_generating) {
    await api('POST', '/studio/api/descriptions/cancel', {});
    for (let i = 0; i < 30 && (await api('GET', '/studio/api/state')).data.desc_generating; i++) await sleep(1000);
  }
  const fill = await api('POST', '/studio/api/descriptions/fill', { good_ids: [GOOD_ID] });
  report('fill start', fill.status === 202 ? 'PASS' : 'FAIL', 'status=' + fill.status);
  if (fill.status !== 202) return finish(1);
  let s = null;
  for (let i = 0; i < 90; i++) {
    s = (await api('GET', '/studio/api/state')).data;
    if (!s.desc_generating && (s.desc_proposals || []).length > 0) break;
    await sleep(2000);
  }
  const propsBefore = (s && (s.desc_proposals || []).length) || 0;
  report('fill done', propsBefore > 0 ? 'PASS' : 'FAIL', 'proposals=' + propsBefore + ' generating=' + (s && s.desc_generating));
  if (propsBefore === 0) return finish(1);

  // --- browser ---
  browser = await chromium.launch({ executablePath: exe, headless: true });
  const ctx = await browser.newContext({ viewport: { width: 1400, height: 900 } });
  await ctx.addInitScript((t) => localStorage.setItem('adminToken', t), token);
  const page = await ctx.newPage();
  const errs = [];
  page.on('pageerror', (e) => errs.push(String(e && e.message ? e.message : e)));

  await page.goto(BASE_URL + '/studio', { waitUntil: 'domcontentloaded', timeout: 30000 });
  await page.waitForFunction(() => document.getElementById('loadingOverlay').style.display === 'none', null, { timeout: 15000 });
  await page.waitForFunction(() => document.getElementById('descPopup').style.display === 'flex', null, { timeout: 15000 }).catch(() => {});
  const openState = await page.evaluate(() => ({
    display: document.getElementById('descPopup').style.display,
    overlay: document.getElementById('descOverlay').style.display,
    props: (state.desc_proposals || []).length,
  }));
  report('popup auto-open', (openState.display === 'flex' && openState.props > 0) ? 'PASS' : 'FAIL', JSON.stringify(openState));
  await page.screenshot({ path: path.join(ART, '1-popup-open.png') });

  // click «Отмена» (idle -> discard)
  const clicked = await page.click('#descPopupBody button:has-text("Отмена")').then(() => true).catch(() => false);
  await page.waitForTimeout(1200);
  const afterCancel = await page.evaluate(() => ({
    display: document.getElementById('descPopup').style.display,
    overlay: document.getElementById('descOverlay').style.display,
    report: document.getElementById('report').textContent.replace(/\s+/g, ' ').trim(),
    props: (state.desc_proposals || []).length,
  }));
  const closedOK = clicked && afterCancel.display === 'none' && afterCancel.overlay === 'none';
  const reportOK = /отброшен/i.test(afterCancel.report);
  report('cancel -> popup closed + report', (closedOK && reportOK) ? 'PASS' : 'FAIL',
    `display=${afterCancel.display} report="${afterCancel.report.slice(0, 90)}"`);
  await page.screenshot({ path: path.join(ART, '2-after-cancel.png') });

  // state empty after cancel (poll up to 6s for fetchState)
  let props0 = -1;
  for (let i = 0; i < 6; i++) {
    props0 = await page.evaluate(() => (state.desc_proposals || []).length);
    if (props0 === 0) break;
    await page.waitForTimeout(1000);
  }
  report('set empty after cancel', props0 === 0 ? 'PASS' : 'FAIL', 'page-state proposals=' + props0);

  // F5 reload
  await page.reload({ waitUntil: 'domcontentloaded', timeout: 30000 });
  await page.waitForFunction(() => document.getElementById('loadingOverlay').style.display === 'none', null, { timeout: 15000 });
  await page.waitForTimeout(4000); // один цикл авто-опроса (3 с) — не всплывает ли заново
  const afterReload = await page.evaluate(() => ({
    display: document.getElementById('descPopup').style.display,
    props: (state.desc_proposals || []).length,
  }));
  report('after F5 popup not reopened + set empty',
    (afterReload.display !== 'flex' && afterReload.props === 0) ? 'PASS' : 'FAIL', JSON.stringify(afterReload));
  await page.screenshot({ path: path.join(ART, '3-after-reload.png') });

  report('no page JS errors', errs.length === 0 ? 'PASS' : 'FAIL', errs.length ? errs.join(' | ').slice(0, 200) : 'clean');
  report('screenshots', 'PASS', ART);

  // server state clean at the end
  const stEnd = (await api('GET', '/studio/api/state')).data;
  report('final set empty', (stEnd.desc_proposals || []).length === 0 ? 'PASS' : 'FAIL',
    'server proposals=' + (stEnd.desc_proposals || []).length);

  return finish(results.some(r => r.status === 'FAIL') ? 1 : 0);
}

main();
