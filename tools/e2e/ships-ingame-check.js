// tools/e2e/ships-ingame-check.js
// Browser check of the art-studio tab "В игре" (game ship registry showcase,
// port 8798). Does NOT start/kill the studio and does NOT touch the ships pool /
// accepted set — it only opens the already-running studio. Checks the Delete
// button + confirm, but cancels the dialog: a live ship is never deleted.
// Expectations are data-agnostic (numbers read from the live endpoints).
// Run: node ships-ingame-check.js  (from tools/e2e)
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync, writeFileSync, appendFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', '..');
const BASE = 'http://127.0.0.1:8798';
const ART = path.join(ROOT, 'tools', 'e2e', 'artifacts');
const LOG = path.join(ART, 'ships-ingame-check.log');
const origLog = console.log.bind(console);
console.log = (...a) => { origLog(...a); try { appendFileSync(LOG, a.join(' ') + '\n'); } catch {} };

const CHROME = ['C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe', process.env.CHROME_PATH].filter(Boolean);
const EDGE = ['C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe', process.env.EDGE_PATH].filter(Boolean);
const findBrowser = () => [...CHROME, ...EDGE].find(existsSync) || null;

const results = [];
function report(name, ok, extra = '') {
  results.push(ok);
  console.log(`[${ok ? 'PASS' : 'FAIL'}] ${name}${extra ? ' - ' + extra : ''}`);
}

async function main() {
  console.log('=== ships-ingame-check ' + new Date().toISOString() + ' ===');
  mkdirSync(ART, { recursive: true });
  const exe = findBrowser();
  if (!exe) { report('browser available', false); return; }
  const browser = await chromium.launch({ executablePath: exe, headless: true });
  try {
    const page = await (await browser.newContext({ viewport: { width: 1500, height: 1000 } })).newPage();
    const pageErrors = [];
    const consoleErrors = [];
    const badResponses = [];
    page.on('pageerror', (e) => pageErrors.push(String(e && e.message ? e.message : e)));
    // favicon.ico — шум вне фичи: у студии нет маршрута (404), браузер просит его сам. Не считаем ошибкой.
    page.on('console', (m) => { if (m.type() === 'error' && !/favicon\.ico/.test(m.text())) consoleErrors.push(m.text() + ' @' + JSON.stringify(m.location())); });
    page.on('response', (r) => { if (r.status() >= 400 && !/favicon\.ico/.test(r.url())) badResponses.push(r.status() + ' ' + r.url()); });
    page.on('requestfailed', (r) => badResponses.push('FAILED ' + r.url() + ' ' + (r.failure() ? r.failure().errorText : '')));

    await page.goto(BASE + '/', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.click('#tabbtn-ingame');
    await page.waitForFunction(() => document.querySelectorAll('#gridIngame .cell img.orient').length > 0, { timeout: 20000 });
    // wait for images to actually load
    await page.waitForFunction(() => {
      const imgs = [...document.querySelectorAll('#gridIngame img.orient')];
      return imgs.filter((i) => i.complete && i.naturalWidth > 0).length >= 3;
    }, { timeout: 20000 }).catch(() => {});
    await page.waitForTimeout(500);

    // (a) groups + counters, (b) first group
    const grid = await page.evaluate(() => {
      const g = document.getElementById('gridIngame');
      const out = { groups: [], info: '' };
      let cur = null;
      for (const k of [...g.children]) {
        if (k.classList.contains('cell')) { if (cur) cur.count++; continue; }
        const st = k.getAttribute('style') || '';
        if (/#e8a33d/i.test(st)) { cur = { header: k.textContent.trim(), count: 0 }; out.groups.push(cur); }
        else if (/#888/i.test(st)) out.info = k.textContent.trim();
      }
      return out;
    });
    const imgsLoaded = await page.evaluate(() =>
      [...document.querySelectorAll('#gridIngame img.orient')].filter((i) => i.complete && i.naturalWidth > 0).length);
    const orientApplied = await page.evaluate(() => {
      const im = document.querySelector('#gridIngame img.orient');
      return { cls: im.className, transform: getComputedStyle(im).transform };
    });

    // data-agnostic expectations: numbers come from the live endpoints, not hardcoded
    const live = await page.evaluate(async () => {
      const ships = await (await fetch('/ships/ingame')).json();
      const races = ((await (await fetch('/ships/races')).json()).races) || [];
      const have = new Set((Array.isArray(ships) ? ships : []).map((s) => s.race).filter(Boolean));
      const missing = races.filter((r) => !have.has(r.slug));
      return { ships: (Array.isArray(ships) ? ships : []).length, missing: missing.length, missingNames: missing.map((r) => r.name) };
    });

    report('(a) groups drawn, first is humans',
      grid.groups.length > 0 && /Люди/.test(grid.groups[0].header) && grid.groups[0].count > 0,
      'groups=' + grid.groups.length + ' first="' + grid.groups[0].header + '" firstCount=' + (grid.groups[0] || {}).count);
    report('(b) groups have counters (header "· N")',
      grid.groups.length > 0 && grid.groups.every((x) => /·\s*\d+\s*$/.test(x.header)),
      JSON.stringify(grid.groups.map((x) => x.header)));
    report('(c) at least 3 images loaded (naturalWidth>0)', imgsLoaded >= 3, 'loaded=' + imgsLoaded);
    report('(orientation) transform applied on first card', orientApplied.transform !== 'none',
      JSON.stringify(orientApplied));

    // (d) missing block — compare against the live catalog, not a hardcoded "3"
    const missing = await page.evaluate(() => {
      const md = document.getElementById('ingameMissing');
      const title = md.querySelector('div') ? md.querySelector('div').textContent.trim() : '';
      const btns = [...md.querySelectorAll('button')].map((b) => ({
        name: b.textContent.trim(), slug: (b.getAttribute('onclick') || '').match(/goToRaceShips\('([^']*)'\)/)?.[1] || '',
      }));
      return { title, btns };
    });
    const titleN = (missing.title.match(/Без корабля:\s*(\d+)/) || [])[1];
    const missingOk = live.missing === 0
      ? (/Все расы каталога имеют корабль/.test(missing.title) && missing.btns.length === 0)
      : (Number(titleN) === live.missing && missing.btns.length === live.missing);
    report('(d) "Без корабля" block matches live catalog (N=' + live.missing + ')',
      missingOk, 'title="' + missing.title + '" buttons=' + missing.btns.length);

    // (delete) every card has an "Удалить" button; the confirm is shown and
    // cancelling it changes nothing (a live ship is never actually deleted).
    const beforeDel = await page.evaluate(() => {
      const cell = document.querySelector('#gridIngame .cell');
      const fileDiv = cell ? cell.querySelector('div[style*="color:#888"]') : null;
      const nameDiv = cell ? cell.querySelector('.name') : null;
      const img = cell ? cell.querySelector('img.orient') : null;
      return {
        cells: document.querySelectorAll('#gridIngame .cell').length,
        withDel: document.querySelectorAll('#gridIngame .cell button.del').length,
        file: fileDiv ? fileDiv.textContent.trim() : '',
        name: nameDiv ? nameDiv.textContent.trim() : '',
        src: img ? img.getAttribute('src') : '',
      };
    });
    const deleteReqs = [];
    page.on('request', (r) => { if (r.url().includes('/ships/ingame/delete')) deleteReqs.push(r.method() + ' ' + r.url()); });
    let confirmMsg = null;
    const onDialog = async (d) => { confirmMsg = d.message(); await d.dismiss(); };
    page.on('dialog', onDialog);
    await page.click('#gridIngame .cell button.del', { force: true }).catch(() => {});
    await page.waitForTimeout(600);
    page.off('dialog', onDialog);
    const afterDel = await page.evaluate(() => ({
      cells: document.querySelectorAll('#gridIngame .cell').length,
      src: (document.querySelector('#gridIngame .cell img.orient') || {}).getAttribute
        ? document.querySelector('#gridIngame .cell img.orient').getAttribute('src') : '',
    }));
    report('(delete) every card has a "Удалить" button',
      beforeDel.cells > 0 && beforeDel.withDel === beforeDel.cells,
      'cells=' + beforeDel.cells + ' withDel=' + beforeDel.withDel);
    report('(delete) confirm names ship + file, warns about restart; cancel is a no-op',
      !!confirmMsg && confirmMsg.includes(beforeDel.file) && confirmMsg.includes(beforeDel.name)
        && /из игры/.test(confirmMsg) && /перезапуска/.test(confirmMsg)
        && deleteReqs.length === 0 && afterDel.cells === beforeDel.cells && afterDel.src === beforeDel.src,
      'confirm="' + confirmMsg + '" deleteReqs=' + JSON.stringify(deleteReqs) + ' cells ' + beforeDel.cells + '->' + afterDel.cells);

    // item 5: click first missing race -> ships tab + select
    const first = missing.btns[0];
    let selOk = false, selValue = '', shipsVisible = false;
    if (first) {
      await page.click('#ingameMissing button');
      await page.waitForFunction(() => document.getElementById('shipRace').value !== '', { timeout: 15000 }).catch(() => {});
      await page.waitForTimeout(500);
      shipsVisible = await page.evaluate(() => getComputedStyle(document.getElementById('tab-ships')).display !== 'none');
      selValue = await page.$eval('#shipRace', (el) => el.value);
      selOk = shipsVisible && selValue === first.slug && selValue !== '';
    }
    report('(5) click missing race opens races tab + selects that race',
      first ? selOk : live.missing === 0,
      'race="' + (first ? first.name : '') + '" expected=' + (first ? first.slug : '') + ' got=' + selValue + ' shipsTabVisible=' + shipsVisible + ' missing=' + live.missing);
    report('(5) no JS errors',
      pageErrors.length === 0 && consoleErrors.length === 0,
      'pageErrors=' + JSON.stringify(pageErrors) + ' consoleErrors=' + JSON.stringify(consoleErrors) + ' badResponses=' + JSON.stringify(badResponses));

    // screenshot of the ingame tab (go back)
    await page.click('#tabbtn-ingame');
    await page.waitForTimeout(800);
    const shot = path.join(ART, 'ingame-tab.png');
    await page.screenshot({ path: shot, fullPage: false });
    console.log('[INFO] screenshot: ' + shot);
  } finally {
    await browser.close();
  }
  const bad = results.filter((x) => !x).length;
  console.log(`RESULT: ${bad === 0 ? 'PASS' : 'FAIL'} (${results.length - bad}/${results.length})`);
  process.exit(bad === 0 ? 0 : 1);
}

main().catch((e) => { report('unexpected error', false, e && e.message ? e.message : String(e)); process.exit(2); });
