// tools/e2e/qa-star-lite.js (QA temporary probe, NOT for commit)
// Point 7 regression: "Для слабых ПК" (starVisualLite=1) must force the cheap
// look and kill effects + the continuous frame, without touching saved keys.
import { chromium } from 'playwright-core';
import { existsSync } from 'node:fs';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const CHROME = ['C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'];
const EDGE = ['C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'];
function findExe() { for (const p of [...CHROME, ...EDGE]) if (existsSync(p)) return p; return null; }
function log(n, s, d) { console.log(`[${n}] ${s}${d ? ' - ' + d : ''}`); }

async function reg() {
  for (let i = 0; i < 3; i++) {
    const u = 'qalite_' + Date.now() + '_' + i;
    const r = await fetch(BASE_URL + '/register', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ username: u, password: 'x'.repeat(10) }) });
    if (r.status === 201) return (await r.json()).token;
    if (r.status !== 409) throw new Error('register ' + r.status);
  }
  throw new Error('collision');
}

const exe = findExe();
if (!exe) { console.log('no browser'); process.exit(1); }
const token = await reg();
const browser = await chromium.launch({ executablePath: exe, headless: true });
const ctx = await browser.newContext({ viewport: { width: 1280, height: 800 } });
await ctx.addInitScript((t) => { localStorage.setItem('token', t); }, token);
const page = await ctx.newPage();
const errs = [];
page.on('pageerror', (e) => errs.push(String(e.message || e)));
let code = 0;
try {
  await page.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded', timeout: 30000 });
  await page.waitForSelector('#mapCanvas', { timeout: 15000 });
  await page.waitForFunction(() => { const el = document.getElementById('loading'); return !el || el.style.display === 'none'; }, { timeout: 30000 });
  await page.waitForTimeout(800);

  const off = await page.evaluate(async () => {
    const sr = await import('/static/js/map/star_render.js');
    localStorage.setItem('starVisualPreset', 'crown');
    localStorage.setItem('starVisualTwinkle', '1');
    localStorage.setItem('starVisualLite', '0');
    return { opts: sr.starVisualOptions(), needsAnim: sr.starNeedsAnim() };
  });
  const on = await page.evaluate(async () => {
    const sr = await import('/static/js/map/star_render.js');
    localStorage.setItem('starVisualLite', '1');
    const opts = sr.starVisualOptions();
    const needsAnim = sr.starNeedsAnim();
    // saved keys must be untouched
    const saved = { preset: localStorage.getItem('starVisualPreset'), twinkle: localStorage.getItem('starVisualTwinkle') };
    localStorage.setItem('starVisualLite', '0');
    return { opts, needsAnim, saved };
  });
  const okOff = off.opts.mode === 'vector' && off.opts.crown === true && off.opts.twinkle === true;
  const okOn = on.opts.mode === 'sprite' && !on.opts.crown && !on.opts.twinkle && !on.opts.ignite && !on.opts.additive && !on.opts.exotic && !on.opts.petals && on.needsAnim === false;
  log('lite-off-baseline', okOff ? 'PASS' : 'FAIL', JSON.stringify(off));
  log('lite-on-kills-effects-frame', okOn ? 'PASS' : 'FAIL', JSON.stringify(on));
  log('lite-keeps-saved-keys', on.saved.preset === 'crown' && on.saved.twinkle === '1' ? 'PASS' : 'FAIL', JSON.stringify(on.saved));
  log('console', errs.length === 0 ? 'PASS' : 'FAIL', `pageErrors=${errs.length}${errs.length ? ' ' + errs[0] : ''}`);
  if (!okOff || !okOn || on.saved.preset !== 'crown' || errs.length) code = 1;
} catch (e) {
  log('run', 'FAIL', String(e.message || e)); code = 1;
}
await browser.close();
console.log('RESULT: ' + (code ? 'FAIL' : 'ALL PASS'));
process.exit(code);
