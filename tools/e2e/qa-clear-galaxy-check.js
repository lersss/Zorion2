// tools/e2e/qa-clear-galaxy-check.js
// Tester run: wave ЧК7 хвост #40 — "Очистка галактики" (clearUniverse).
//   A1 before click: button present, label, enabled
//   A2 while running: button disabled + label "Очищаем...", second click = no 2nd job
//   A3 after: worlds=0 (/admin/stats), no false pacman error, 0 console errors
// ADMIN_TOKEN env required. Writes artifacts/clear-galaxy-result.json + screenshots.
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const ARTIFACTS_DIR = path.join(path.dirname(fileURLToPath(import.meta.url)), 'artifacts');
const CHROME_PATHS = [process.env.CHROME_PATH, 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'].filter(Boolean);
const EDGE_PATHS = [process.env.EDGE_PATH, 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'].filter(Boolean);

const TOKEN = process.env.ADMIN_TOKEN;
if (!TOKEN) { console.error('ADMIN_TOKEN env required'); process.exit(1); }
const AUTH = { Authorization: 'Bearer ' + TOKEN };
const out = { steps: [], meta: {} };
function rec(step, status, detail) { out.steps.push({ step, status, detail }); console.log(`[${step}] ${status}`); }

const sleep = (ms) => new Promise(r => setTimeout(r, ms));
function findExe() {
  for (const p of CHROME_PATHS) if (existsSync(p)) return p;
  for (const p of EDGE_PATHS) if (existsSync(p)) return p;
  return null;
}
async function api(url) {
  const res = await fetch(BASE_URL + url, { headers: AUTH });
  const text = await res.text();
  let json = null; try { json = JSON.parse(text); } catch (e) {}
  return { status: res.status, json, text };
}

let browser = null;
async function finish(code) {
  out.meta.exitCode = code;
  try { writeFileSync(path.join(ARTIFACTS_DIR, 'clear-galaxy-result.json'), JSON.stringify(out, null, 2)); } catch (e) {}
  if (browser) await browser.close().catch(() => {});
  console.log('EXIT ' + code);
  process.exit(code);
}

async function main() {
  mkdirSync(ARTIFACTS_DIR, { recursive: true });

  const jobs = ['generate_universe','generate_planets','generate_factions','generate_resources',
    'generate_race_settlements','regenerate_planets','generate_npc','hypothesis','pacman'];
  const running = [];
  for (const j of jobs) { const s = await api('/admin/generate-status?job=' + j); if (s.json && s.json.status === 'running') running.push(j); }
  if (running.length) { rec('PRE', 'FAIL', 'running jobs: ' + running.join(',')); return finish(1); }
  const stats0 = await api('/admin/stats?refresh=1');
  out.meta.statsBefore = stats0.json;
  rec('PRE', 'PASS', 'jobs idle');

  const exe = findExe();
  if (!exe) { rec('BROWSER', 'FAIL', 'no Chrome/Edge'); return finish(1); }
  browser = await chromium.launch({ executablePath: exe, headless: true });
  const context = await browser.newContext({ viewport: { width: 1400, height: 900 } });
  await context.addInitScript((t) => {
    localStorage.setItem('adminToken', t);
    localStorage.setItem('adminActiveTab', 'tab-generation');
    localStorage.setItem('adminGenSubTab', 'stars');
  }, TOKEN);
  const page = await context.newPage();

  const pageErrors = [], consoleErrors = [], badResponses = [], clearReqs = [], clearResps = [], dialogs = [];
  page.on('pageerror', (e) => pageErrors.push(String(e && e.message ? e.message : e)));
  page.on('console', (m) => { if (m.type() === 'error') consoleErrors.push(m.text().slice(0, 300)); });
  page.on('response', (r) => {
    const u = r.url().replace(BASE_URL, '');
    if (r.status() >= 400) badResponses.push({ status: r.status(), url: u });
    if (u.includes('/admin/clear')) clearResps.push({ status: r.status(), url: u });
  });
  page.on('request', (r) => { if (r.method() === 'POST' && r.url().includes('/admin/clear')) clearReqs.push({ t: Date.now(), url: r.url().replace(BASE_URL, '') }); });
  page.on('dialog', (d) => { dialogs.push(d.message()); d.accept().catch(() => {}); });
  const shot = (n) => page.screenshot({ path: path.join(ARTIFACTS_DIR, n) }).catch(() => {});

  try {
    // ============ A1 ============
    await page.goto(BASE_URL + '/admin', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.waitForSelector('#clearUniverseBtn', { state: 'attached', timeout: 20000 });
    await page.waitForTimeout(2500);
    const a1 = await page.evaluate(() => {
      const b = document.getElementById('clearUniverseBtn');
      const sec = b ? b.closest('div') : null;
      return {
        exists: !!b,
        label: b ? b.textContent.trim() : null,
        disabled: b ? b.disabled : null,
        visible: b ? (getComputedStyle(b).display !== 'none') : false,
        nearHeading: sec ? (sec.parentElement ? sec.parentElement.querySelector('h2,h3')?.textContent.trim() : null) : null,
      };
    });
    out.meta.a1 = a1;
    const a1ok = a1.exists && a1.visible && a1.disabled === false;
    rec('A1', a1ok ? 'PASS' : 'FAIL', null);
    await shot('clear-a1-before.png');
    if (!a1ok) return finish(1);

    // ============ A2 ============
    const reqBefore = clearReqs.length;
    await page.click('#clearUniverseBtn');
    // tight sampling right after the click
    let seenDisabled = false, seenLabel = false, a2state = null;
    for (let i = 0; i < 60; i++) { // up to ~18s
      a2state = await page.evaluate(() => {
        const b = document.getElementById('clearUniverseBtn');
        return b ? { label: b.textContent.trim(), disabled: b.disabled, resultText: document.getElementById('clearResult')?.textContent || '' } : null;
      });
      if (a2state && a2state.disabled && /Очищаем/.test(a2state.label)) { seenDisabled = true; seenLabel = true; break; }
      if (i === 0) { out.meta.a2first = a2state; }
      await sleep(300);
    }
    await shot('clear-a2-during.png');
    // attempt second click (disabled => no-op)
    await page.evaluate(() => { const b = document.getElementById('clearUniverseBtn'); if (b) b.click(); });
    await sleep(400);
    const reqAfterSecondClick = clearReqs.length;
    out.meta.a2 = { seenDisabled, seenLabel, firstSample: out.meta.a2first, reqBefore, reqAfterSecondClick };
    const a2ok = seenDisabled && seenLabel;
    rec('A2', a2ok ? 'PASS' : 'FAIL', null);

    // ============ A3 ============
    // wait for clear to finish: result text shows success OR button restored+enabled
    let a3state = null, done = false;
    for (let i = 0; i < 300; i++) { // up to ~90s
      a3state = await page.evaluate(() => {
        const b = document.getElementById('clearUniverseBtn');
        return {
          label: b ? b.textContent.trim() : null,
          disabled: b ? b.disabled : null,
          resultText: document.getElementById('clearResult')?.textContent || '',
          toastErrors: [...document.querySelectorAll('.toast-error')].map(t => t.textContent),
        };
      });
      if (/✅|❌/.test(a3state.resultText) && a3state.disabled === false) { done = true; break; }
      await sleep(300);
    }
    await sleep(1000);
    const statsAfter = await api('/admin/stats?refresh=1');
    const finalState = await page.evaluate(() => ({
      label: document.getElementById('clearUniverseBtn')?.textContent.trim(),
      disabled: document.getElementById('clearUniverseBtn')?.disabled,
      resultText: document.getElementById('clearResult')?.textContent || '',
      toastErrors: [...document.querySelectorAll('.toast-error')].map(t => t.textContent),
    }));
    await shot('clear-a3-after.png');

    const worldsAfter = statsAfter.json ? statsAfter.json.worlds : null;
    const noFalsePacman = !/pacman|пакман/i.test(finalState.resultText) && !/Pacman is eating/i.test(JSON.stringify(badResponses));
    const a3ok = done && worldsAfter === 0 && /✅/.test(finalState.resultText) && noFalsePacman &&
      consoleErrors.length === 0 && pageErrors.length === 0 &&
      clearReqs.length - reqBefore === 1;
    out.meta.a3 = {
      done, finalState, worldsAfter, statsAfter: statsAfter.json,
      noFalsePacman,
      clearReqs: clearReqs.length, clearReqsFromClick: clearReqs.length - reqBefore,
      clearResps, badResponses, consoleErrors, pageErrors, dialogs,
    };
    rec('A3', a3ok ? 'PASS' : 'FAIL', null);
    if (!a3ok) return finish(1);
    return finish(0);
  } catch (err) {
    out.meta.error = String(err && err.message ? err.message : err);
    rec('RUN', 'FAIL', out.meta.error);
    return finish(1);
  }
}
main();
