// tools/e2e/dashboard-graphics-check.js
// Browser smoke test for the dashboard "Graphics" tab (idea 2026-09-22,
// @uidesigner UI). Verifies the tab opens, controls exist, and choices are
// read/written in localStorage (map reads the same keys). Console output is
// ASCII on purpose (Windows PowerShell cp866 breaks Cyrillic).
//
// Run: node dashboard-graphics-check.js   (BASE_URL env overrides)
// Exit code 0 = ALL PASS, 1 = any FAIL.
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
    const username = 'gfx_' + Date.now() + '_' + attempt;
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
    report('1/4 register/login', 'PASS', 'user ' + creds.username);
  } catch (err) {
    report('1/4 register/login', 'FAIL', String(err && err.message ? err.message : err));
    return finish(1);
  }

  const exe = findExecutable();
  if (!exe) {
    report('setup browser', 'FAIL', 'no Chrome/Edge found');
    return finish(1);
  }
  console.log('browser: ' + exe.name);

  try {
    browser = await chromium.launch({ executablePath: exe.path, headless: true });
  } catch (err) {
    report('setup browser', 'FAIL', 'launch: ' + String(err && err.message ? err.message : err));
    return finish(1);
  }

  const context = await browser.newContext({ viewport: { width: 1100, height: 900 } });
  await context.addInitScript((t) => { localStorage.setItem('token', t); }, token);
  const page = await context.newPage();

  const pageErrors = [];
  const consoleErrors = [];
  const http404 = [];
  page.on('pageerror', (err) => pageErrors.push(String(err && err.message ? err.message : err)));
  page.on('console', (m) => {
    if (m.type() === 'error') {
      const loc = m.location();
      consoleErrors.push(m.text() + (loc && loc.url ? ' @' + loc.url : ''));
    }
  });
  page.on('response', (res) => { if (res.status() === 404) http404.push(res.url()); });

  try {
    // --- Step 2: tab opens, controls present ---
    await page.goto(BASE_URL + '/', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.waitForSelector('.tab-btn[data-tab="tab-graphics"]', { timeout: 15000 });
    await page.click('.tab-btn[data-tab="tab-graphics"]');
    await page.waitForTimeout(400);

    const ui = await page.evaluate(() => {
      const root = document.getElementById('tab-graphics');
      const radios = (name) => [...root.querySelectorAll(`input[name="${name}"]`)];
      const presets = radios('graphicsPreset');
      const crown = presets.find(r => r.value === 'crown');
      return {
        active: !!root && root.classList.contains('active'),
        liteOff: !!document.getElementById('graphicsLiteOff'),
        liteOn: !!document.getElementById('graphicsLiteOn'),
        presets: presets.map(r => r.value),
        crownSelectable: !!(crown && !crown.disabled),
        effects: ['graphicsTwinkle', 'graphicsIgnite', 'graphicsAdditive', 'graphicsExotic', 'graphicsPetals']
          .every(id => !!document.getElementById(id)),
        radar: radios('graphicsRadar').map(r => r.value),
        defaults: {
          sprite: !!(presets.find(r => r.value === 'sprite') || {}).checked,
          twinkle: !!document.getElementById('graphicsTwinkle').checked,
          ignite: !!document.getElementById('graphicsIgnite').checked,
          additive: !!document.getElementById('graphicsAdditive').checked,
          exotic: !!document.getElementById('graphicsExotic').checked,
          petals: !!document.getElementById('graphicsPetals').checked,
          radar1: !!radios('graphicsRadar').find(r => r.value === '1').checked,
        },
        savedKeys: {
          preset: localStorage.getItem('starVisualPreset'),
          lite: localStorage.getItem('starVisualLite'),
          radar: localStorage.getItem('radarBoundaryVariant'),
        },
      };
    });

    const ok2 = ui.active && ui.liteOff && ui.liteOn &&
      ui.presets.includes('sprite') && ui.presets.includes('eye') &&
      ui.presets.includes('photo') && ui.presets.includes('crown') && ui.crownSelectable &&
      ui.effects && ui.radar.length === 4 &&
      ui.defaults.sprite && ui.defaults.twinkle && ui.defaults.ignite &&
      ui.defaults.additive && ui.defaults.exotic && !ui.defaults.petals && ui.defaults.radar1;
    report('2/4 tab + controls + defaults', ok2 ? 'PASS' : 'FAIL', JSON.stringify(ui));
    if (!ok2) return finish(1);

    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'dashboard-graphics.png') });

    // --- Step 3: choices persist into localStorage ---
    const readKeys = () => page.evaluate(() => ({
      preset: localStorage.getItem('starVisualPreset'),
      lite: localStorage.getItem('starVisualLite'),
      twinkle: localStorage.getItem('starVisualTwinkle'),
      petals: localStorage.getItem('starVisualPetals'),
      radar: localStorage.getItem('radarBoundaryVariant'),
    }));

    // Lite ON: blocks dim, note appears, other keys untouched.
    await page.check('#graphicsLiteOn');
    await page.waitForTimeout(200);
    const liteState = await page.evaluate(() => ({
      key: localStorage.getItem('starVisualLite'),
      viewDim: document.getElementById('graphics-view-block').classList.contains('graphics-block-off'),
      presetDisabled: [...document.querySelectorAll('input[name="graphicsPreset"]')].every(r => r.disabled),
      note: !document.getElementById('graphics-lite-note').hidden,
    }));

    // Lite OFF -> back to normal.
    await page.check('#graphicsLiteOff');
    await page.waitForTimeout(150);
    const liteOff = await page.evaluate(() => localStorage.getItem('starVisualLite'));

    // Crown (D) is selectable (no longer disabled "soon"): pick it, key persists.
    await page.check('input[name="graphicsPreset"][value="crown"]');
    await page.waitForTimeout(150);
    const crown = await page.evaluate(() => ({
      key: localStorage.getItem('starVisualPreset'),
      checked: document.querySelector('input[name="graphicsPreset"][value="crown"]').checked,
    }));

    // Photo preset -> petals auto-enabled; Sprite -> petals off.
    await page.check('input[name="graphicsPreset"][value="photo"]');
    await page.waitForTimeout(150);
    const photo = await page.evaluate(() => ({
      key: localStorage.getItem('starVisualPreset'),
      petals: localStorage.getItem('starVisualPetals'),
      petalsChecked: document.getElementById('graphicsPetals').checked,
    }));
    await page.check('input[name="graphicsPreset"][value="sprite"]');
    await page.waitForTimeout(150);
    const sprite = await page.evaluate(() => ({
      key: localStorage.getItem('starVisualPreset'),
      petals: localStorage.getItem('starVisualPetals'),
      petalsChecked: document.getElementById('graphicsPetals').checked,
    }));

    // Twinkle off + radar "контур". Effects are hidden under <details> by
    // default (idea §2) - open it first.
    await page.click('#graphics-effects-block > summary');
    await page.waitForTimeout(150);
    await page.uncheck('#graphicsTwinkle');
    await page.check('input[name="graphicsRadar"][value="2"]');
    await page.waitForTimeout(200);
    const manual = await readKeys();

    const ok3 = liteState.key === '1' && liteState.viewDim && liteState.presetDisabled && liteState.note &&
      liteOff === '0' &&
      crown.key === 'crown' && crown.checked &&
      photo.key === 'photo' && photo.petals === '1' && photo.petalsChecked &&
      sprite.key === 'sprite' && sprite.petals === '0' && !sprite.petalsChecked &&
      manual.twinkle === '0' && manual.radar === '2';
    report('3/4 choices -> localStorage', ok3 ? 'PASS' : 'FAIL',
      `lite=${JSON.stringify(liteState)} crown=${JSON.stringify(crown)} photo=${JSON.stringify(photo)} sprite=${JSON.stringify(sprite)} manual=${JSON.stringify(manual)}`);
    if (!ok3) return finish(1);

    // --- Step 4: reload keeps values, console clean ---
    await page.reload({ waitUntil: 'domcontentloaded' });
    await page.waitForSelector('.tab-btn[data-tab="tab-graphics"]', { timeout: 15000 });
    await page.click('.tab-btn[data-tab="tab-graphics"]');
    await page.waitForTimeout(300);
    const afterReload = await page.evaluate(() => ({
      preset: localStorage.getItem('starVisualPreset'),
      twinkle: localStorage.getItem('starVisualTwinkle'),
      radar: localStorage.getItem('radarBoundaryVariant'),
      spriteChecked: document.querySelector('input[name="graphicsPreset"][value="sprite"]').checked,
      twinkleChecked: document.getElementById('graphicsTwinkle').checked,
      radar2: document.querySelector('input[name="graphicsRadar"][value="2"]').checked,
      activeTab: localStorage.getItem('dashboardActiveTab'),
      errors: 0,
    }));
    // favicon.ico is requested by the browser itself (no <link rel=icon> on the
    // page) and 404s - it is not an application error.
    const errs = pageErrors.concat(consoleErrors.filter(t => !t.includes('favicon.ico')));
    const ok4 = afterReload.preset === 'sprite' && afterReload.twinkle === '0' &&
      afterReload.radar === '2' && afterReload.spriteChecked && !afterReload.twinkleChecked &&
      afterReload.radar2 && afterReload.activeTab === 'tab-graphics' && errs.length === 0;
    report('4/4 reload + console', ok4 ? 'PASS' : 'FAIL',
      `afterReload=${JSON.stringify(afterReload)} errors=${errs.length} http404=${JSON.stringify(http404)}` + (errs.length ? ' first: ' + errs[0] : ''));
    if (!ok4) return finish(1);
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
