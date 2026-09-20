// Диагностика тултипа «Маршрут развернётся» (сценарий 4).
import { chromium } from 'playwright-core';
import { existsSync } from 'node:fs';

const BASE_URL = process.env.BASE_URL || 'http://localhost:8080';
const TOKEN = process.env.QA_TOKEN || '';

const CHROME_PATHS = ['C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'].filter(existsSync);
const EDGE_PATHS = ['C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'].filter(existsSync);
const exe = CHROME_PATHS[0] || EDGE_PATHS[0];

const B = { id: 'f13452d8-6e9e-49e3-86ae-b0ef0daac9a3', name: 'Zinelchal', spec: 'M', stemp: 3665, x: -472.02, y: -282.44 };
const C = { id: '5a867045-11c2-48cd-ac4e-e1362b2dcc4f', name: 'Niubek', spec: 'M', stemp: 3600, x: -586.02, y: -282.44 };

async function main() {
  const browser = await chromium.launch({ executablePath: exe, headless: true });
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  await context.addInitScript((t) => { localStorage.setItem('token', t); }, TOKEN);
  const page = await context.newPage();

  page.on('pageerror', (err) => console.log('PAGEERROR: ' + String(err && err.message ? err.message : err)));
  page.on('request', (req) => {
    if (req.url().includes('/me')) console.log('REQ /me: ' + req.method());
  });
  page.on('response', (res) => {
    const u = res.url();
    if (u.includes('/me')) {
      res.json().then(j => console.log('ME[' + res.status() + ']: flight=' + (j.flight ? j.flight.to : 'null'))).catch(() => console.log('ME[' + res.status() + ']: (no json)'));
    } else if (u.includes('/api/worlds/') && u.includes('/planets')) {
      console.log('PLANETS[' + res.status() + ']: ' + u.replace(BASE_URL, ''));
    }
  });

  await page.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded', timeout: 30000 });
  await page.waitForSelector('#mapCanvas', { timeout: 15000 });
  await page.waitForFunction(() => {
    const el = document.getElementById('loading');
    return !el || el.style.display === 'none';
  }, { timeout: 30000 });
  await page.waitForTimeout(1200);

  // Старт композитного к B.
  await page.evaluate((w) => {
    window.openSystemModal(w.id, w.name, w.spec, null, null, { stype: 'star', stemp: w.stemp, systype: 'single', smods: {}, x: w.x, y: w.y, hasEngine: true });
  }, B);
  await page.waitForSelector('#system-modal-overlay', { timeout: 15000 });
  await page.waitForFunction(() => {
    const panel = document.getElementById('right-panel');
    return panel && panel.querySelectorAll('tr[data-index]').length > 0;
  }, { timeout: 15000 });
  await page.evaluate(() => { document.querySelectorAll('tr[data-index]')[0].click(); });
  await page.waitForSelector('#composite-fly-btn', { timeout: 10000 });
  await page.waitForTimeout(600);
  await page.evaluate(() => { document.getElementById('composite-fly-btn').click(); });
  console.log('STARTED to B');
  await page.waitForTimeout(3000);

  // Открыть модалку C.
  await page.evaluate((w) => {
    window.openSystemModal(w.id, w.name, w.spec, null, null, { stype: 'star', stemp: w.stemp, systype: 'single', smods: {}, x: w.x, y: w.y, hasEngine: true });
  }, C);
  await page.waitForSelector('#system-modal-overlay', { timeout: 15000 });
  await page.waitForFunction(() => {
    const panel = document.getElementById('right-panel');
    return panel && panel.querySelectorAll('tr[data-index]').length > 0;
  }, { timeout: 15000 });
  await page.evaluate(() => { document.querySelectorAll('tr[data-index]')[0].click(); });
  await page.waitForSelector('#composite-fly-btn', { timeout: 10000 });

  // Логируем тултип каждые 2 сек в течение 14 сек.
  for (let i = 0; i < 7; i++) {
    await page.waitForTimeout(2000);
    const t = await page.evaluate(() => {
      const b = document.getElementById('composite-fly-btn');
      const ov = document.getElementById('system-modal-overlay');
      return { title: b ? b.title : 'NO-BTN', modal: !!(ov && ov.style.display !== 'none'), worldName: (document.getElementById('currentWorldName') || {}).textContent };
    });
    console.log('T+' + ((i + 1) * 2) + 's: ' + JSON.stringify(t));
  }

  await browser.close();
}

main().catch(e => { console.log('FATAL: ' + String(e && e.message ? e.message : e)); process.exit(1); });