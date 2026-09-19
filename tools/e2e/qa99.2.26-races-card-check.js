// tools/e2e/qa99.2.26-races-card-check.js
// Browser smoke test for the race card "player-facing" (spec 99.2.26):
// dashboard -> Encyclopedia -> Races. Checks the humans card (open by
// default): 6 attribute WORDS (no numbers/bars), "Вид · Ниша · Размер"
// block without "Рой" (humans are not collective), "Дом" in words, lore
// paragraphs. Then injects sulfur_swarms into the journal (client-side,
// presentation layer, spec 86a §7.3) and checks the collective card shows
// "Размер роя" (size_group). Catches pageerror / .toast-error / /login
// redirects (logout).
//
// Run: node qa99.2.26-races-card-check.js   (BASE_URL env overrides default)
// Exit code 0 = all PASS (SKIP allowed), 1 = any FAIL.
// Console output is ASCII on purpose (Windows PowerShell cp866 breaks Cyrillic).
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
    const username = 'e2e_race_' + Date.now() + '_' + attempt;
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

  // --- Step 1: register/login ---
  let token = null;
  try {
    const creds = await registerAndLogin();
    token = creds.token;
    report('1/5 register/login', 'PASS', 'user ' + creds.username);
  } catch (err) {
    report('1/5 register/login', 'FAIL', String(err && err.message ? err.message : err));
    return finish(1);
  }

  const exe = findExecutable();
  if (!exe) {
    report('setup browser', 'FAIL', 'no Chrome/Edge found');
    return finish(1);
  }
  console.log('browser: ' + exe.name + ' (' + exe.path + ')');

  try {
    browser = await chromium.launch({ executablePath: exe.path, headless: true });
  } catch (err) {
    report('setup browser', 'FAIL', 'launch: ' + String(err && err.message ? err.message : err));
    return finish(1);
  }

  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.addInitScript((t) => { localStorage.setItem('token', t); }, token);
  const page = await context.newPage();

  const pageErrors = [];
  page.on('pageerror', (err) => pageErrors.push(String(err && err.message ? err.message : err)));
  let encStatus = null;
  page.on('response', (res) => {
    if (res.url().includes('/api/encyclopedia/races')) encStatus = res.status();
  });

  try {
    // --- Step 2: dashboard loads, open Encyclopedia tab ---
    await page.goto(BASE_URL + '/', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.waitForSelector('.dashboard-tabs', { timeout: 15000 });
    await page.click('.tab-btn[data-tab="tab-encyclopedia"]');
    await page.waitForSelector('#enc-races', { timeout: 15000 });
    await page.waitForTimeout(800); // settle after fetch/render

    const dashState = await page.evaluate(() => ({
      path: location.pathname,
      hasToken: !!localStorage.getItem('token'),
      racesSection: !!document.getElementById('enc-races'),
      cards: document.querySelectorAll('#enc-races > div > div').length,
    }));
    const errs = pageErrors.slice();
    const ok2 = dashState.path === '/' && dashState.hasToken &&
      dashState.racesSection && encStatus === 200 && errs.length === 0;
    report('2/5 dashboard+encyclopedia', ok2 ? 'PASS' : 'FAIL',
      `path=${dashState.path} encHTTP=${encStatus} cards=${dashState.cards} pageErrors=${errs.length}` +
      (errs.length ? ' first: ' + errs[0] : ''));
    if (!ok2) return finish(1);

    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'races-encyclopedia.png') });

    // --- Step 3: humans card (open by default) ---
    // Find the humans card: it is the opened card (not silhouette) whose
    // header contains "Люди". Check: 6 attribute labels with words (no
    // numbers/bars), "Вид · Ниша · Размер" without "Рой", "Дом" in words,
    // lore paragraphs.
    const humans = await page.evaluate(() => {
      const cards = [...document.querySelectorAll('#enc-races > div > div')];
      const card = cards.find(c => c.textContent.includes('Люди') && !c.textContent.includes('???'));
      if (!card) return { ok: false, reason: 'humans opened card not found' };
      const text = card.textContent;
      const labels = ['Агрессия', 'Любопытство', 'Размножение', 'Интеллект', 'Дипломатия', 'Устойчивость'];
      const labelCount = labels.filter(l => text.includes(l)).length;
      const hasWords = ['агрессивные', 'любопытные', 'как человек', 'умные', 'дипломатичные', 'устойчивые']
        .filter(w => text.includes(w)).length;
      // "Вид · Ниша · Размер" block: exactly 3 parts (kind · niche ·
      // size_individual), NO size_group for non-collective humans (spec
      // 99.2.26 §2 п.5: size_group appended only if not null).
      const kindNicheEl = [...card.querySelectorAll('div')].find(d => d.textContent.includes('Вид · Ниша · Размер:'));
      const kindNiche = !!kindNicheEl && kindNicheEl.textContent.includes('гуманоид') &&
        kindNicheEl.textContent.includes('строитель') && kindNicheEl.textContent.includes('с человека') &&
        !kindNicheEl.textContent.includes('· рой') && !kindNicheEl.textContent.includes('· стая');
      const homeWords = text.includes('Дом:') && text.includes('умеренные миры');
      const loreParas = (card.querySelectorAll('div').length > 0) &&
        text.includes('Люди — двуногие существа');
      return {
        ok: labelCount === 6 && hasWords === 6 && kindNiche && homeWords && loreParas,
        labelCount, hasWords, kindNiche, homeWords, loreParas,
      };
    });

    if (!humans.ok) {
      report('3/5 humans card', 'FAIL', JSON.stringify(humans));
      return finish(1);
    }
    report('3/5 humans card', 'PASS',
      `labels=${humans.labelCount}/6 words=${humans.hasWords}/6 kindNiche=${humans.kindNiche} homeWords=${humans.homeWords} loreParas=${humans.loreParas}`);
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'races-humans-card.png') });

    // --- Step 4: collective race (sulfur_swarms) ---
    // Journal is client-side (spec 86a §7.3): inject metRaces via
    // localStorage, reload, re-open the tab, check the card shows
    // "Размер роя" (size_group "рой-облако").
    await page.evaluate(() => {
      const j = JSON.parse(localStorage.getItem('zorion.journal') || '{}');
      j.metRaces = j.metRaces || [];
      if (!j.metRaces.includes('sulfur_swarms')) j.metRaces.push('sulfur_swarms');
      localStorage.setItem('zorion.journal', JSON.stringify(j));
    });
    await page.reload({ waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.waitForSelector('.dashboard-tabs', { timeout: 15000 });
    await page.click('.tab-btn[data-tab="tab-encyclopedia"]');
    await page.waitForSelector('#enc-races', { timeout: 15000 });
    await page.waitForTimeout(800);

    const swarm = await page.evaluate(() => {
      const cards = [...document.querySelectorAll('#enc-races > div > div')];
      const card = cards.find(c => c.textContent.includes('Серные рои') && !c.textContent.includes('???'));
      if (!card) return { ok: false, reason: 'sulfur_swarms opened card not found' };
      const text = card.textContent;
      const kindNiche = text.includes('Вид · Ниша · Размер:') && text.includes('рой') &&
        text.includes('хищник') && text.includes('с ладонь');
      const sizeGroup = text.includes('рой-облако');
      const loreParas = text.includes('Серные рои не знают одиночества');
      return { ok: kindNiche && sizeGroup && loreParas, kindNiche, sizeGroup, loreParas };
    });

    if (!swarm.ok) {
      report('4/5 sulfur_swarms card', 'FAIL', JSON.stringify(swarm));
      return finish(1);
    }
    report('4/5 sulfur_swarms card', 'PASS',
      `kindNiche=${swarm.kindNiche} sizeGroup=${swarm.sizeGroup} loreParas=${swarm.loreParas}`);
    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'races-sulfur-swarms-card.png') });

    // --- Step 5: no JS errors / no toast-error / no logout redirect ---
    const finalState = await page.evaluate(() => ({
      path: location.pathname,
      hasToken: !!localStorage.getItem('token'),
      toastError: !!document.querySelector('.toast-error'),
    }));
    const errsFinal = pageErrors.slice();
    const ok5 = finalState.path === '/' && finalState.hasToken &&
      !finalState.toastError && errsFinal.length === 0;
    report('5/5 no errors', ok5 ? 'PASS' : 'FAIL',
      `path=${finalState.path} toastError=${finalState.toastError} pageErrors=${errsFinal.length}` +
      (errsFinal.length ? ' first: ' + errsFinal[0] : ''));
    if (!ok5) return finish(1);
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