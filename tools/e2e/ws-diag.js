// tools/e2e/ws-diag.js — диагностика браузерного WS на /map:
// 1) было ли WS-соединение из кода страницы (performance entries),
// 2) открывается ли WS из страницы вручную и приходят ли фреймы.
import { chromium } from 'playwright-core';
import { existsSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const CHROME_PATHS = [
  process.env.CHROME_PATH,
  'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe',
].filter(Boolean);
const EDGE_PATHS = [
  process.env.EDGE_PATH,
  'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe',
].filter(Boolean);
function findExe() {
  for (const p of CHROME_PATHS) if (existsSync(p)) return p;
  for (const p of EDGE_PATHS) if (existsSync(p)) return p;
  return null;
}

async function main() {
  let token;
  for (let a = 0; a < 3; a++) {
    const r = await fetch(BASE_URL + '/register', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username: 'diag_' + Date.now() + '_' + a, password: 'x' }),
    });
    if (r.status === 201) { token = (await r.json()).token; break; }
  }
  if (!token) { console.log('register FAIL'); process.exit(1); }
  const exe = findExe();
  const browser = await chromium.launch({ executablePath: exe, headless: true });
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  await context.addInitScript((t) => { localStorage.setItem('token', t); }, token);
  const page = await context.newPage();
  const pageErrors = [];
  page.on('pageerror', (e) => pageErrors.push(String(e && e.message ? e.message : e)));
  const consoleMsgs = [];
  page.on('console', (m) => consoleMsgs.push(m.type() + ': ' + m.text().slice(0, 150)));

  await page.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded', timeout: 30000 });
  await page.waitForSelector('#mapCanvas', { timeout: 15000 });
  await page.waitForTimeout(5000);

  const perf = await page.evaluate(() => {
    return performance.getEntriesByType('resource')
      .filter(e => e.name.includes('/ws'))
      .map(e => e.name.slice(0, 60) + ' dur=' + Math.round(e.duration) + 'ms');
  });

  const manual = await page.evaluate((base) => {
    return new Promise((resolve) => {
      const proto = location.protocol === 'https:' ? 'wss://' : 'ws://';
      const url = proto + location.host + '/ws?token=' + localStorage.getItem('token');
      let frames = 0;
      const types = [];
      const log = [];
      try {
        const ws = new WebSocket(url);
        ws.onopen = () => { log.push('MANUAL OPEN'); };
        ws.onerror = () => { log.push('MANUAL ERROR'); };
        ws.onclose = (e) => { log.push('MANUAL CLOSE ' + e.code); };
        ws.onmessage = (e) => {
          frames++;
          try { const d = JSON.parse(e.data); if (d.type) types.push(d.type); } catch (err) {}
        };
        setTimeout(() => {
          log.push('frames=' + frames + ' types=' + types.join(','));
          try { ws.close(); } catch (e) {}
          resolve({ log, frames });
        }, 12000);
      } catch (e) {
        resolve({ log: ['THROW ' + String(e && e.message ? e.message : e)], frames: 0 });
      }
    });
  }, BASE_URL);

  console.log('PERF_WS_ENTRIES: ' + JSON.stringify(perf));
  console.log('MANUAL_WS: ' + JSON.stringify(manual));
  console.log('PAGE_ERRORS: ' + JSON.stringify(pageErrors));
  console.log('CONSOLE: ' + JSON.stringify(consoleMsgs.slice(0, 10)));
  await browser.close();
  process.exit(0);
}
main();