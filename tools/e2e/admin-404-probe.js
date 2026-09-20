// tools/e2e/admin-404-probe.js
// Quick probe: open /admin, wait 3s, capture all HTTP >= 400 responses (URL)
// and console errors. No pacman, no worlds eaten.
import { chromium } from 'playwright-core';
import { existsSync } from 'node:fs';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const ADMIN_TOKEN = process.env.ADMIN_TOKEN;
if (!ADMIN_TOKEN) {
  console.error('ADMIN_TOKEN env required: dev JWT-токен админа (см. tools/e2e/pacman-shot.js / README.md)');
  process.exit(1);
}
const CHROME_PATHS = [process.env.CHROME_PATH, 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'].filter(Boolean);
const EDGE_PATHS = [process.env.EDGE_PATH, 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'].filter(Boolean);

function findExecutable() {
  for (const p of CHROME_PATHS) if (existsSync(p)) return p;
  for (const p of EDGE_PATHS) if (existsSync(p)) return p;
  return null;
}

async function main() {
  const exe = findExecutable();
  if (!exe) { console.log('no browser'); process.exit(1); }
  const browser = await chromium.launch({ executablePath: exe, headless: true });
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  await context.addInitScript((t) => {
    localStorage.setItem('adminToken', t);
    localStorage.setItem('adminActiveTab', 'tab-generation');
  }, ADMIN_TOKEN);
  const page = await context.newPage();
  const bad = [];
  const consoleErrors = [];
  const allReqs = [];
  page.on('request', (req) => allReqs.push(req.method() + ' ' + req.url().replace(BASE_URL, '')));
  page.on('response', (res) => { if (res.status() >= 400) bad.push(res.status() + ' ' + res.url().replace(BASE_URL, '')); });
  page.on('console', (msg) => { if (msg.type() === 'error') consoleErrors.push(msg.text().slice(0, 200)); });
  await page.goto(BASE_URL + '/admin', { waitUntil: 'domcontentloaded', timeout: 30000 });
  await page.waitForTimeout(4000);
  console.log('BAD RESPONSES: ' + (bad.length ? bad.join(' | ') : 'none'));
  console.log('CONSOLE ERRORS: ' + (consoleErrors.length ? consoleErrors.join(' | ') : 'none'));
  console.log('ALL REQUESTS (' + allReqs.length + '):');
  allReqs.forEach((r, i) => console.log('  [' + i + '] ' + r));
  await browser.close();
  process.exit(0);
}
main();