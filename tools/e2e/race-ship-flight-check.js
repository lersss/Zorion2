// tools/e2e/race-ship-flight-check.js
// Probe (2026-09-21): humans race ship sprite in the game map DURING flight.
// Registers a player, sets ship_icon to race_humans_starship.png, starts an
// interstellar flight via /travel, opens /map with followShip, screenshots the
// map mid-flight (full + 4x zoom around canvas center). Console output ASCII
// (Windows cp866 breaks Cyrillic).
// Run: node race-ship-flight-check.js   (BASE_URL overrides default)
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const ARTIFACTS_DIR = path.join(path.dirname(fileURLToPath(import.meta.url)), 'artifacts');
const SHIP_ICON = 'race_humans_starship.png';

const CHROME_PATHS = [
  process.env.CHROME_PATH,
  'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe',
].filter(Boolean);
const EDGE_PATHS = [
  process.env.EDGE_PATH,
  'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe',
].filter(Boolean);

function findExecutable() {
  for (const p of CHROME_PATHS) if (existsSync(p)) return { path: p, name: 'Chrome' };
  for (const p of EDGE_PATHS) if (existsSync(p)) return { path: p, name: 'Edge' };
  return null;
}

async function register() {
  const username = 'e2e_flight_' + Date.now();
  const password = 'e2e-pass-' + Date.now();
  const res = await fetch(BASE_URL + '/register', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  });
  if (res.status !== 201) throw new Error('register HTTP ' + res.status);
  return (await res.json()).token;
}

let browser = null;
async function finish(code) {
  if (browser) await browser.close().catch(() => {});
  process.exit(code);
}

async function main() {
  mkdirSync(ARTIFACTS_DIR, { recursive: true });
  let token;
  try {
    token = await register();
  } catch (e) {
    console.log('RESULT: FAIL - ' + e.message);
    return finish(1);
  }
  const auth = { Authorization: 'Bearer ' + token, 'Content-Type': 'application/json' };

  // set the race sprite as the player's ship look
  const setIcon = await fetch(BASE_URL + '/me/ship-icon', {
    method: 'PUT', headers: auth, body: JSON.stringify({ ship_icon: SHIP_ICON }),
  });
  console.log('ship-icon HTTP ' + setIcon.status);

  const me = await (await fetch(BASE_URL + '/me', { headers: auth })).json();
  const worlds = await (await fetch(BASE_URL + '/worlds', { headers: auth })).json();
  const list = Array.isArray(worlds) ? worlds : (worlds.worlds || []);
  const dest = list.find((w) => w.id && w.id !== me.current_world_id);
  if (!dest) {
    console.log('RESULT: FAIL - no destination world (worlds=' + list.length + ')');
    return finish(1);
  }
  const travel = await fetch(BASE_URL + '/travel', {
    method: 'POST', headers: auth, body: JSON.stringify({ world_id: dest.id }),
  });
  const travelBody = await travel.text();
  console.log('travel HTTP ' + travel.status + ' ' + travelBody.slice(0, 200));
  if (travel.status !== 200 && travel.status !== 202) {
    console.log('RESULT: FAIL - travel not started');
    return finish(1);
  }

  const exe = findExecutable();
  if (!exe) {
    console.log('RESULT: FAIL - no Chrome/Edge found');
    return finish(1);
  }
  browser = await chromium.launch({ executablePath: exe.path, headless: true });
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  await context.addInitScript((t) => {
    localStorage.setItem('token', t);
    sessionStorage.setItem('followShip', '1'); // camera follows the flying ship
  }, token);
  const page = await context.newPage();
  const pageErrors = [];
  page.on('pageerror', (e) => pageErrors.push(String(e && e.message ? e.message : e)));

  await page.goto(BASE_URL + '/map', { waitUntil: 'domcontentloaded', timeout: 30000 });
  await page.waitForSelector('#mapCanvas', { timeout: 15000 });
  await page.waitForFunction(() => {
    const el = document.getElementById('loading');
    return !el || el.style.display === 'none';
  }, { timeout: 30000 });
  await page.waitForTimeout(1200); // let the flight animation run a bit
  // "Найти меня" (centerBtn) = followShip + centerOnAgent → camera follows the ship
  const centerBtn = await page.$('#centerBtn');
  if (centerBtn) await centerBtn.click();
  await page.waitForTimeout(800);

  const flying = await page.evaluate(() => {
    const bar = document.getElementById('status-bar');
    return bar ? bar.textContent.trim() : '';
  });
  console.log('statusBar: ' + flying);

  const full = path.join(ARTIFACTS_DIR, 'race-ship-flight.png');
  await page.screenshot({ path: full });

  // crop a 220x220 region around the canvas center (= the ship, followShip) and upscale 4x
  const zoom = path.join(ARTIFACTS_DIR, 'race-ship-flight-zoom.png');
  const shot = await page.screenshot({ clip: { x: 640 - 110, y: 400 - 110, width: 220, height: 220 } });
  const b64 = shot.toString('base64');
  const png = await page.evaluate(async (data) => {
    const bmp = await createImageBitmap(await (await fetch('data:image/png;base64,' + data)).blob());
    const c = document.createElement('canvas');
    c.width = bmp.width * 4; c.height = bmp.height * 4;
    const ctx = c.getContext('2d');
    ctx.imageSmoothingEnabled = false;
    ctx.drawImage(bmp, 0, 0, c.width, c.height);
    return c.toDataURL('image/png');
  }, b64);
  const fs = await import('node:fs/promises');
  await fs.writeFile(zoom, Buffer.from(png.split(',')[1], 'base64'));

  const ok = flying.includes('полёт') && pageErrors.length === 0 && existsSync(zoom);
  console.log('screenshot: ' + full);
  console.log('zoom: ' + zoom);
  console.log('pageErrors: ' + pageErrors.length);
  console.log('RESULT: ' + (ok ? 'PASS' : 'FAIL'));
  return finish(ok ? 0 : 1);
}

main().catch((e) => {
  console.log('RESULT: FAIL - ' + (e && e.message ? e.message : e));
  finish(1);
});
