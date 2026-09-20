// Диагностика автооткрытия модалки по прибытии (99.2.30 п.4).
// Воспроизводит сценарий 1 composite-route-check.js с полным логированием.
import { chromium } from 'playwright-core';
import { existsSync } from 'node:fs';

const BASE_URL = process.env.BASE_URL || 'http://localhost:8080';
const TOKEN = process.env.QA_TOKEN || '';

const CHROME_PATHS = [
  process.env.CHROME_PATH,
  'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe',
].filter(Boolean);
const EDGE_PATHS = [
  process.env.EDGE_PATH,
  'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe',
].filter(Boolean);

function findExecutable() {
  for (const p of CHROME_PATHS) if (existsSync(p)) return p;
  for (const p of EDGE_PATHS) if (existsSync(p)) return p;
  return null;
}

const B = { id: 'f13452d8-6e9e-49e3-86ae-b0ef0daac9a3', name: 'Zinelchal', spec: 'M', stemp: 3665, x: -499.02, y: -313.44 };

async function main() {
  const exe = findExecutable();
  const browser = await chromium.launch({ executablePath: exe, headless: true });
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  await context.addInitScript((t) => { localStorage.setItem('token', t); }, TOKEN);
  const page = await context.newPage();

  page.on('pageerror', (err) => console.log('PAGEERROR: ' + String(err && err.message ? err.message : err)));
  page.on('console', (msg) => {
    const t = msg.type();
    if (t === 'error' || t === 'warning') console.log('CONSOLE[' + t + ']: ' + msg.text());
  });
  page.on('response', (res) => {
    if (res.url().includes('/me')) {
      res.json().then(j => console.log('ME: flight=' + (j.flight ? j.flight.to : 'null') + ' pos=' + (j.current_position ? j.current_position.status : 'null') + ' pending=' + (j.pending_destination ? JSON.stringify(j.pending_destination) : 'null') + ' world=' + j.current_world_name)).catch(() => {});
    }
  });

  await page.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded', timeout: 30000 });
  await page.waitForSelector('#mapCanvas', { timeout: 15000 });
  await page.waitForFunction(() => {
    const el = document.getElementById('loading');
    return !el || el.style.display === 'none';
  }, { timeout: 30000 });
  await page.waitForTimeout(1200);

  // Открыть модалку B программно.
  await page.evaluate((w) => {
    window.openSystemModal(w.id, w.name, w.spec, null, null, {
      stype: 'star', stemp: w.stemp, systype: 'single', smods: { metallicity: -0.8 }, x: w.x, y: w.y, hasEngine: true,
    });
  }, B);
  await page.waitForSelector('#system-modal-overlay', { timeout: 15000 });
  await page.waitForFunction(() => {
    const panel = document.getElementById('right-panel');
    return panel && panel.querySelectorAll('tr[data-index]').length > 0;
  }, { timeout: 15000 });

  // Клик по первой планете → композитная кнопка.
  await page.evaluate(() => {
    const panel = document.getElementById('right-panel');
    const tr = panel.querySelectorAll('tr[data-index]')[0];
    if (tr) tr.click();
  });
  await page.waitForSelector('#composite-fly-btn', { timeout: 10000 });
  await page.waitForTimeout(600);
  console.log('BTN: ' + JSON.stringify(await page.evaluate(() => {
    const b = document.getElementById('composite-fly-btn');
    return b ? { text: b.textContent.trim(), title: b.title } : null;
  })));

  // Старт.
  await page.evaluate(() => { document.getElementById('composite-fly-btn').click(); });
  console.log('CLICKED composite fly');
  await page.waitForTimeout(2500);
  console.log('AFTER START: ' + JSON.stringify(await page.evaluate(() => ({
    modalClosed: !document.getElementById('system-modal-overlay'),
    marker: sessionStorage.getItem('compositeRoute'),
    followShip: sessionStorage.getItem('followShip'),
  }))));

  // Ждём прибытие (полёт ~12 сек) + 10 сек запаса.
  await page.waitForTimeout(25000);
  console.log('AFTER 25s: ' + JSON.stringify(await page.evaluate(() => ({
    modalOpen: !!document.getElementById('system-modal-overlay'),
    marker: sessionStorage.getItem('compositeRoute'),
    strip: (() => { const s = document.getElementById('intra-flight-strip'); return s ? s.style.display : 'no-el'; })(),
  }))));

  await browser.close();
}

main().catch(e => { console.log('FATAL: ' + String(e && e.message ? e.message : e)); process.exit(1); });