// Замер ореола: у камня-спрайта берём кольцо пикселей сразу за силуэтом и
// сравниваем со средним фоном сцены. Если rim-light обрезан по альфе, кольцо
// не ярче фона (ореола нет).
import { chromium } from 'playwright-core';
import { existsSync } from 'node:fs';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const TOKEN = process.env.QA_TOKEN || '';
const BELT_ID = process.env.QA_BELT_ID || '';
const CHROME = ['C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe',
  'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'].find(p => existsSync(p));

let browser = null;
async function finish(c) { if (browser) await browser.close().catch(() => {}); process.exit(c); }

async function main() {
  if (!TOKEN || !BELT_ID) { console.error('QA_TOKEN/QA_BELT_ID не заданы'); return finish(2); }
  browser = await chromium.launch({ executablePath: CHROME, headless: true });
  const page = await browser.newPage({ viewport: { width: 1600, height: 900 } });
  await page.addInitScript((t) => localStorage.setItem('token', t), TOKEN);
  await page.goto(BASE_URL + '/belt.html?belt=' + BELT_ID, { waitUntil: 'domcontentloaded' });
  await page.waitForTimeout(4000);

  const res = await page.evaluate(() => {
    const canvas = document.getElementById('belt-canvas');
    const ctx = canvas.getContext('2d');
    const dpr = canvas.width / window.innerWidth;
    const w = window.__beltWorld;
    if (!w) return { error: 'no world' };
    // Берём жилу, ближайшую к центру экрана.
    const cam = { x: w.ship.x, y: w.ship.y };
    const vw = window.innerWidth, vh = window.innerHeight;
    let best = null, bd = Infinity;
    for (const a of w.asteroids) {
      const sx = (a.x - cam.x) + vw / 2;
      const sy = (a.y - cam.y) + vh / 2;
      const d = Math.hypot(sx - vw / 2, sy - vh / 2);
      if (d < bd) { bd = d; best = a; }
    }
    if (!best) return { error: 'no asteroid' };
    const sx = Math.round(((best.x - cam.x) + vw / 2) * dpr);
    const sy = Math.round(((best.y - cam.y) + vh / 2) * dpr);
    const rad = Math.round(best.r * dpr);
    const img = ctx.getImageData(0, 0, canvas.width, canvas.height).data;
    const lum = (x, y) => {
      if (x < 0 || y < 0 || x >= canvas.width || y >= canvas.height) return null;
      const i = (y * canvas.width + x) * 4;
      return 0.299 * img[i] + 0.587 * img[i + 1] + 0.114 * img[i + 2];
    };
    // Кольцо сразу за силуэтом (1.15r..1.35r) — там был бы ореол.
    let ringSum = 0, ringN = 0;
    for (let k = 0; k < 360; k += 3) {
      const ang = k * Math.PI / 180;
      for (const f of [1.15, 1.25, 1.35]) {
        const v = lum(Math.round(sx + Math.cos(ang) * rad * f), Math.round(sy + Math.sin(ang) * rad * f));
        if (v !== null) { ringSum += v; ringN++; }
      }
    }
    // Базовый фон — далёкие углы канваса.
    const corners = [lum(20, 20), lum(canvas.width - 20, 20), lum(20, canvas.height - 20), lum(canvas.width - 20, canvas.height - 20)].filter(v => v !== null);
    const bg = corners.reduce((a, b) => a + b, 0) / corners.length;
    return { ring: ringSum / ringN, bg, rad, sx, sy };
  });

  if (res.error) { console.log('[halo] FAIL - ' + res.error); return finish(1); }
  const delta = res.ring - res.bg;
  // Ореол = кольцо заметно ярче фона. Порог 6 (из 255) — мягкий.
  const ok = delta < 6;
  console.log(`[halo] ${ok ? 'PASS' : 'FAIL'} - ring=${res.ring.toFixed(2)} bg=${res.bg.toFixed(2)} delta=${delta.toFixed(2)} (rad=${res.rad}px)`);
  return finish(ok ? 0 : 1);
}
main().catch((e) => { console.error('halo-check error:', e); finish(1); });
