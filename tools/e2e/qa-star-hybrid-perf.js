// tools/e2e/qa-star-hybrid-perf.js (QA temporary probe, NOT for commit)
// Point 9: at strong zoom (R>34) the sprite "hybrid" path must not be more
// expensive than the vector "eye" path. Times drawStar() only, both modes.
import { chromium } from 'playwright-core';
import { existsSync } from 'node:fs';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const exe = ['C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe', 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'].find(p => existsSync(p));
async function reg() {
  for (let i = 0; i < 3; i++) {
    const u = 'qahy_' + Date.now() + '_' + i;
    const r = await fetch(BASE_URL + '/register', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ username: u, password: 'x'.repeat(10) }) });
    if (r.status === 201) return (await r.json()).token;
    if (r.status !== 409) throw new Error('register ' + r.status);
  }
  throw new Error('collision');
}
const token = await reg();
const browser = await chromium.launch({ executablePath: exe, headless: true });
const ctx = await browser.newContext({ viewport: { width: 1280, height: 800 } });
await ctx.addInitScript((t) => { localStorage.setItem('token', t); localStorage.setItem('starVisualPreset', 'sprite'); }, token);
const page = await ctx.newPage();
const errs = [];
page.on('pageerror', (e) => errs.push(String(e.message || e)));
await page.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded', timeout: 30000 });
await page.waitForSelector('#mapCanvas', { timeout: 15000 });
await page.waitForFunction(() => { const el = document.getElementById('loading'); return !el || el.style.display === 'none'; }, { timeout: 30000 });
await page.waitForTimeout(800);

const res = await page.evaluate(async () => {
  const sr = await import('/static/js/map/star_render.js');
  sr.setStarVisualToggle('starVisualIgnite', false);
  sr.setStarVisualToggle('starVisualTwinkle', false);
  const cv = document.createElement('canvas');
  cv.width = 400; cv.height = 400;
  const g = cv.getContext('2d');
  const N = 3000;
  const REPS = 7;
  const R = 60; // > SPRITE_HYBRID_R=34
  const run = (mode) => {
    const c = { sid: 'qa-hy', sspec: 'G', stype: 'star', stemp: 5800 };
    const opts = { mode, crown: false, twinkle: false, ignite: false, additive: false, exotic: false, petals: false };
    for (let i = 0; i < 200; i++) sr.drawStar(g, c, 200, 200, R, opts); // warm-up
    const t0 = performance.now();
    for (let i = 0; i < N; i++) sr.drawStar(g, c, 200, 200, R, opts);
    return performance.now() - t0;
  };
  const med = (a) => a.slice().sort((x, y) => x - y)[Math.floor(a.length / 2)];
  const eyeRuns = [], spriteRuns = [];
  for (let r = 0; r < REPS; r++) { eyeRuns.push(run('eye')); spriteRuns.push(run('sprite')); }
  const eye = med(eyeRuns), sprite = med(spriteRuns);
  return { N, REPS, R, eyeMs: eye, spriteMs: sprite, perStarEyeUs: eye / N * 1000, perStarSpriteUs: sprite / N * 1000, ratio: sprite / eye };
});
console.log(JSON.stringify(res, null, 2));
console.log('hybrid<=vector: ' + (res.ratio <= 1.05 ? 'PASS' : 'FAIL') + ' ratio=' + res.ratio.toFixed(2));
console.log('pageErrors=' + errs.length);
await browser.close();
process.exit(res.ratio <= 1.05 && errs.length === 0 ? 0 : 1);
