// tools/e2e/qa-fresh-galaxy-check.js
// A4: prepare a fresh galaxy through the admin UI.
//   - select preset "crossroads" (~10 000 worlds) and generate universe
//   - generate planets for all worlds
//   - generate factions
// Waits for each job to finish, records stats and UI result texts.
// ADMIN_TOKEN env required. Writes artifacts/fresh-galaxy-result.json.
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
function rec(s, st, d) { out.steps.push({ step: s, status: st, detail: d }); console.log(`[${s}] ${st}`); }
const sleep = (ms) => new Promise(r => setTimeout(r, ms));
function findExe() { for (const p of CHROME_PATHS) if (existsSync(p)) return p; for (const p of EDGE_PATHS) if (existsSync(p)) return p; return null; }
async function api(url) { const r = await fetch(BASE_URL + url, { headers: AUTH }); const t = await r.text(); let j = null; try { j = JSON.parse(t); } catch (e) {} return { status: r.status, json: j, text: t }; }
async function waitJob(job, timeoutMs) {
  const t0 = Date.now();
  while (Date.now() - t0 < timeoutMs) {
    const s = await api('/admin/generate-status?job=' + job);
    const st = s.json ? s.json.status : '?';
    if (st === 'done' || st === 'error' || st === 'canceled') return { status: st, json: s.json, ms: Date.now() - t0 };
    await sleep(2000);
  }
  return { status: 'timeout', json: null, ms: Date.now() - t0 };
}

let browser = null;
async function finish(code) {
  out.meta.exitCode = code;
  try { writeFileSync(path.join(ARTIFACTS_DIR, 'fresh-galaxy-result.json'), JSON.stringify(out, null, 2)); } catch (e) {}
  if (browser) await browser.close().catch(() => {});
  console.log('EXIT ' + code); process.exit(code);
}

async function main() {
  mkdirSync(ARTIFACTS_DIR, { recursive: true });
  const jobs = ['generate_universe','generate_planets','generate_factions','generate_resources','generate_race_settlements','regenerate_planets','generate_npc','hypothesis','pacman'];
  for (const j of jobs) { const s = await api('/admin/generate-status?job=' + j); if (s.json && s.json.status === 'running') { rec('PRE', 'FAIL', 'running: ' + j); return finish(1); } }
  out.meta.statsBefore = (await api('/admin/stats?refresh=1')).json;
  rec('PRE', 'PASS', 'jobs idle');

  const exe = findExe();
  if (!exe) { rec('BROWSER', 'FAIL', 'no browser'); return finish(1); }
  browser = await chromium.launch({ executablePath: exe, headless: true });
  const context = await browser.newContext({ viewport: { width: 1400, height: 900 } });
  await context.addInitScript((t) => {
    localStorage.setItem('adminToken', t);
    localStorage.setItem('adminActiveTab', 'tab-generation');
    localStorage.setItem('adminGenSubTab', 'stars');
  }, TOKEN);
  const page = await context.newPage();
  const pageErrors = [], consoleErrors = [];
  page.on('pageerror', e => pageErrors.push(String(e && e.message ? e.message : e)));
  page.on('console', m => { if (m.type() === 'error') consoleErrors.push(m.text().slice(0, 300)); });
  page.on('dialog', d => d.accept().catch(() => {}));
  const shot = n => page.screenshot({ path: path.join(ARTIFACTS_DIR, n) }).catch(() => {});

  try {
    await page.goto(BASE_URL + '/admin', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.waitForSelector('#genPreset', { state: 'attached', timeout: 20000 });
    await page.waitForTimeout(2000);

    // --- universe (skip if already generated in a previous partial run) ---
    const pre = out.meta.statsBefore || {};
    if (pre.worlds >= 10000) {
      out.meta.universe = { status: 'skipped-existing', worlds: pre.worlds };
      rec('A4-universe', 'SKIP', 'existing worlds=' + pre.worlds);
    } else {
      await page.selectOption('#genPreset', 'crossroads');
      await page.dispatchEvent('#genPreset', 'change');
      await page.waitForTimeout(200);
      const form = await page.evaluate(() => ({
        worlds: document.getElementById('genWorlds').value,
        clusters: document.getElementById('genClusters').value,
        mapSize: document.getElementById('genMapSize').value,
        minDist: document.getElementById('genMinDist').value,
        radius: document.getElementById('genClusterRadius').value,
        spacing: document.getElementById('genClusterSpacing').value,
        outlier: document.getElementById('genOutlierPercent').value,
        shape: document.getElementById('genShape').value,
      }));
      out.meta.form = form;
      await page.click('button[onclick="generateUniverse()"]');
      const u = await waitJob('generate_universe', 600000);
      const genText = await page.evaluate(() => document.getElementById('genResult')?.textContent || '');
      out.meta.universe = { status: u.status, ms: u.ms, json: u.json, uiText: genText };
      rec('A4-universe', u.status === 'done' ? 'PASS' : 'FAIL', u.status);
      await shot('fresh-a4-universe.png');
      if (u.status !== 'done') return finish(1);
    }

    // --- planets ---
    await page.evaluate(() => window.switchGenSubTab('planets'));
    await page.waitForTimeout(300);
    await page.click('button[onclick="generatePlanets()"]');
    const p = await waitJob('generate_planets', 900000);
    const planetText = await page.evaluate(() => document.getElementById('planetResult')?.textContent || '');
    out.meta.planets = { status: p.status, ms: p.ms, json: p.json, uiText: planetText };
    rec('A4-planets', p.status === 'done' ? 'PASS' : 'FAIL', p.status);
    await shot('fresh-a4-planets.png');
    if (p.status !== 'done') return finish(1);

    // --- factions ---
    await page.evaluate(() => window.switchGenSubTab('factions'));
    await page.waitForTimeout(300);
    await page.click('button[onclick="generateFactions()"]');
    const f = await waitJob('generate_factions', 600000);
    const factText = await page.evaluate(() => document.getElementById('factionResult')?.textContent || '');
    out.meta.factions = { status: f.status, ms: f.ms, json: f.json, uiText: factText };
    rec('A4-factions', f.status === 'done' ? 'PASS' : 'FAIL', f.status);

    out.meta.statsAfter = (await api('/admin/stats?refresh=1')).json;
    out.meta.consoleErrors = consoleErrors;
    out.meta.pageErrors = pageErrors;
    await shot('fresh-a4-after.png');
    const ok = out.meta.statsAfter && out.meta.statsAfter.worlds > 0 && out.meta.statsAfter.planets > 0;
    rec('A4-final', ok ? 'PASS' : 'FAIL', JSON.stringify(out.meta.statsAfter));
    return finish(ok ? 0 : 1);
  } catch (err) {
    out.meta.error = String(err && err.message ? err.message : err);
    rec('RUN', 'FAIL', out.meta.error);
    return finish(1);
  }
}
main();
