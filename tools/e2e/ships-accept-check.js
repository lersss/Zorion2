// tools/e2e/ships-accept-check.js
// Live + browser check of the manual ship-sprite acceptance mode
// (art studio, tab "Корабли рас", port 8798). Starts the studio itself,
// exercises HTTP actions (fit / rotate / preview / accept / reject / auto),
// opens the acceptance mode in a real browser (slider live preview,
// counter, accept), screenshots, then kills the studio and RESTORES
// ai_drafts/ships_pool + final_accepted/ships so the creator's pool is
// untouched. Demo before/after PNGs are kept in ai_drafts/ships_accept_demo/.
// ASCII console output on purpose (Windows PowerShell cp866 breaks Cyrillic).
// Run: node ships-accept-check.js
import { chromium } from 'playwright-core';
import { spawn, execSync } from 'node:child_process';
import {
  existsSync, mkdirSync, copyFileSync, rmSync, cpSync, readFileSync, writeFileSync, appendFileSync, openSync, closeSync,
} from 'node:fs';
import net from 'node:net';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..', '..');
const BASE = 'http://127.0.0.1:8798';
const ART = path.join(ROOT, 'tools', 'e2e', 'artifacts');
const LOG = path.join(ART, 'ships-accept-check.log');
const POOL = path.join(ROOT, 'ai_drafts', 'ships_pool');
const ACC = path.join(ROOT, 'ai_drafts', 'final_accepted', 'ships');
const DEMO = path.join(ROOT, 'ai_drafts', 'ships_accept_demo');
const EXE = path.join(ROOT, 'zorion-art-studio.exe');
const STUDIO_LOG = path.join(ART, 'ships-accept-studio.log');
const BAK = path.join(process.env.TEMP || ROOT, 'ship_accept_bak_' + Date.now());

// все строки — и в консоль, и в файл (консоль теряется, если процесс убьют)
const origLog = console.log.bind(console);
console.log = (...a) => { origLog(...a); try { appendFileSync(LOG, a.join(' ') + '\n'); } catch {} };

const CHROME = ['C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe', process.env.CHROME_PATH].filter(Boolean);
const EDGE = ['C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe', process.env.EDGE_PATH].filter(Boolean);
const findBrowser = () => [...CHROME, ...EDGE].find(existsSync) || null;

// portBusy/freePort — перед стартом освобождаем 8798: залипший процесс
// прошлого прогона держит порт, свежая студия печатает «порт занят» и
// выходит, а waitReady опрашивает ЧУЖОЙ слушатель — отсюда «timeout
// /ships/races» (соединение принято, ответа нет). Убиваем только владельца.
function portBusy(port) {
  return new Promise((resolve) => {
    const s = net.connect({ host: '127.0.0.1', port });
    const done = (v) => { s.destroy(); resolve(v); };
    s.once('connect', () => done(true));
    s.once('error', () => done(false));
    s.setTimeout(700, () => done(true));
  });
}
async function freePort(port) {
  if (!(await portBusy(port))) return true;
  console.log('[step] port ' + port + ' busy - killing stale listener');
  try {
    const out = execSync('netstat -ano -p tcp', { encoding: 'utf8' });
    const pids = new Set();
    for (const line of out.split(/\r?\n/)) {
      if (/LISTENING/i.test(line) && line.includes(':' + port)) {
        const cols = line.trim().split(/\s+/);
        pids.add(cols[cols.length - 1]);
      }
    }
    for (const pid of pids) { try { execSync('taskkill /PID ' + pid + ' /F', { stdio: 'ignore' }); } catch {} }
  } catch (e) { console.log('  freePort netstat: ' + e.message); }
  for (let i = 0; i < 25 && (await portBusy(port)); i++) await new Promise((r) => setTimeout(r, 100));
  return !(await portBusy(port));
}

let server = null;
let browser = null;
const results = [];
function report(name, ok, extra = '') {
  results.push(ok);
  console.log(`[${ok ? 'PASS' : 'FAIL'}] ${name}${extra ? ' - ' + extra : ''}`);
}
async function doFetch(url, signal) {
  const r = await fetch(BASE + url, { signal });
  if (!r.ok) throw new Error('HTTP ' + r.status);
  return Buffer.from(await r.arrayBuffer());
}
// Жёсткий таймаут через Promise.race: не полагаемся на AbortController
// (зависший сервер не должен подвешивать прогон).
async function get(url, ms = 15000) {
  const ac = new AbortController();
  let timer;
  const guard = new Promise((_, rej) => { timer = setTimeout(() => { ac.abort(); rej(new Error('timeout ' + url)); }, ms); });
  try { return await Promise.race([doFetch(url, ac.signal), guard]); }
  finally { clearTimeout(timer); }
}
const jget = async (u, ms) => JSON.parse((await get(u, ms)).toString('utf8'));
const save = (buf, name) => { writeFileSync(path.join(DEMO, name), buf); };
function pngSize(buf) {
  if (buf.length < 24 || buf.toString('ascii', 12, 16) !== 'IHDR') return null;
  return [buf.readUInt32BE(16), buf.readUInt32BE(20)];
}
async function waitReady() {
  for (let i = 0; i < 30; i++) {
    try { await get('/ships/races', 2000); return true; } catch (e) { console.log('  waitReady: ' + e.message); await new Promise((r) => setTimeout(r, 500)); }
  }
  return false;
}
function poolMetaAppend(entry) {
  const mp = path.join(POOL, 'meta.json');
  const arr = existsSync(mp) ? JSON.parse(readFileSync(mp, 'utf8')) : [];
  arr.push(entry);
  writeFileSync(mp, JSON.stringify(arr));
}
function restore() {
  try {
    if (existsSync(POOL)) rmSync(POOL, { recursive: true, force: true });
    cpSync(path.join(BAK, 'pool'), POOL, { recursive: true });
    if (existsSync(ACC)) rmSync(ACC, { recursive: true, force: true });
    if (existsSync(path.join(BAK, 'acc'))) cpSync(path.join(BAK, 'acc'), ACC, { recursive: true });
    console.log('[INFO] pool/accepted restored');
  } catch (e) { console.log('[INFO] restore error: ' + e.message); }
}
function stopServer() {
  if (server && !server.killed) { try { server.kill(); } catch {} }
  server = null;
}

async function main() {
  console.log('=== ships-accept-check ' + new Date().toISOString() + ' ===');
  mkdirSync(ART, { recursive: true });
  mkdirSync(DEMO, { recursive: true });
  mkdirSync(BAK, { recursive: true });
  cpSync(POOL, path.join(BAK, 'pool'), { recursive: true });
  if (existsSync(ACC)) cpSync(ACC, path.join(BAK, 'acc'), { recursive: true });

  if (!existsSync(EXE)) { report('studio binary present', false, EXE); return; }
  if (!(await freePort(8798))) { report('port 8798 free for studio', false); return; }
  console.log('[step] spawn studio');
  const outFd = openSync(STUDIO_LOG, 'w');
  server = spawn(EXE, [], { cwd: ROOT, stdio: ['ignore', outFd, outFd], windowsHide: true });
  try { closeSync(outFd); } catch {}
  if (!(await waitReady())) {
    let log = '';
    try { log = readFileSync(STUDIO_LOG, 'utf8').trim(); } catch {}
    report('studio starts and answers', false, log || 'no studio output');
    return;
  }
  report('studio starts and answers', true, BASE);

  // --- HTTP: pool + hint ---
  const list = await jget('/ships/list');
  report('pool candidates listed', list.length > 0, 'n=' + list.length);
  console.log('[step] auto hint');
  const auto = await jget('/ships/auto?file=s07.png');
  report('auto nose hint (python)', typeof auto.angle === 'number' && auto.error === undefined,
    JSON.stringify(auto));
  const autoBad = await jget('/ships/auto?file=nope.png');
  report('auto hint clear error for missing file', typeof autoBad.error === 'string', JSON.stringify(autoBad));
  const acc0 = await jget('/ships/accepted?race=humans');
  report('accepted counter endpoint', typeof acc0.accepted === 'number', 'accepted=' + acc0.accepted);

  // --- inject bad frame ---
  console.log('[step] inject bad frame');
  if (!existsSync(path.join(DEMO, 'bad_s01.png'))) { report('bad frame artifact present', false); return; }
  copyFileSync(path.join(DEMO, 'bad_s01.png'), path.join(POOL, 's99.png'));
  poolMetaAppend({ file: 's99.png', race: 'humans', race_name: 'Люди', seed: 1, prompt1: 'demo', prompt2: 'demo' });
  const list2 = await jget('/ships/list');
  report('injected candidate appears in pool', list2.some((x) => x.file === 's99.png'));
  save(await get('/ships/img/s99.png'), '01_before_fit.png');

  // --- fit ---
  console.log('[step] fit');
  const fitMsg = await jget('/ships/act?file=s99.png&what=fit');
  save(await get('/ships/img/s99.png'), '02_after_fit.png');
  report('action fit', /Вписано/.test(fitMsg.msg), fitMsg.msg);

  // --- arbitrary rotation ---
  console.log('[step] rotate');
  const rotMsg = await jget('/ships/act?file=s99.png&what=rotate&angle=-25');
  save(await get('/ships/img/s99.png'), '03_after_rotate25.png');
  report('action rotate (arbitrary angle)', /-25/.test(rotMsg.msg), rotMsg.msg);

  // --- preview is non-destructive ---
  console.log('[step] preview');
  const before = await get('/ships/img/s99.png');
  const prev = await get('/ships/preview?file=s99.png&angle=40');
  const after = await get('/ships/img/s99.png');
  const ps = pngSize(prev);
  report('preview renders 200x200', !!ps && ps[0] === 200 && ps[1] === 200, ps ? ps.join('x') : 'bad');
  report('preview does not modify pool file', Buffer.compare(before, after) === 0);
  save(prev, '04_preview_angle40.png');

  // --- accept writes registry with transform + date ---
  console.log('[step] accept');
  const accMsg = await jget('/ships/act?file=s99.png&what=accept');
  const meta = JSON.parse(readFileSync(path.join(ACC, 'ships_meta.json'), 'utf8'));
  const acceptedFile = meta.length ? meta[meta.length - 1].file : '';
  report('action accept', /Принято/.test(accMsg.msg) && existsSync(path.join(ACC, acceptedFile)), accMsg.msg);
  report('accepted meta has angle/flip/date',
    typeof meta[meta.length - 1].angle === 'number' && meta[meta.length - 1].date !== '',
    JSON.stringify(meta[meta.length - 1]));
  const accAfter = await jget('/ships/accepted?race=humans');
  report('accepted counter increments', accAfter.accepted === acc0.accepted + 1, 'accepted=' + accAfter.accepted);

  // --- reject removes candidate ---
  console.log('[step] reject');
  copyFileSync(path.join(DEMO, 'bad_s01.png'), path.join(POOL, 's98.png'));
  poolMetaAppend({ file: 's98.png', race: 'humans', race_name: 'Люди', seed: 2, prompt1: 'demo', prompt2: 'demo' });
  const rejMsg = await jget('/ships/act?file=s98.png&what=reject');
  const list3 = await jget('/ships/list');
  report('action reject', /Удалено/.test(rejMsg.msg) && !list3.some((x) => x.file === 's98.png'), rejMsg.msg);

  // --- browser: acceptance mode UI ---
  console.log('[step] browser');
  const exe = findBrowser();
  if (!exe) { report('browser available', false); return; }
  {
    browser = await chromium.launch({ executablePath: exe, headless: true });
    const page = await (await browser.newContext({ viewport: { width: 1400, height: 1000 } })).newPage();
    const pageErrors = [];
    page.on('pageerror', (e) => pageErrors.push(String(e && e.message ? e.message : e)));
    let previewHits = 0;
    page.on('request', (r) => { if (r.url().includes('/ships/preview')) previewHits++; });

    await page.goto(BASE + '/', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.click('#tabbtn-ships');
    // <option> внутри закрытого <select> Playwright не считает visible —
    // ждём именно наполнение списка, а не видимость опции.
    await page.waitForFunction(() => document.querySelectorAll('#shipRace option').length > 0, { timeout: 15000 });
    await page.selectOption('#shipRace', 'humans');
    await page.click('#btnAcceptMode');
    await page.waitForSelector('#acceptPanel', { state: 'visible', timeout: 15000 });
    await page.waitForFunction(() => {
      const im = document.getElementById('accImg');
      return im && im.complete && im.naturalWidth > 0;
    }, { timeout: 15000 }).catch(() => {});
    const thumbs = await page.$$eval('#accThumbs .accThumb', (n) => n.length);
    report('acceptance mode opens with candidate + thumbnails', thumbs > 0, 'thumbs=' + thumbs);

    // slider live preview (input) then commit (change)
    await page.$eval('#accSlider', (el) => { el.value = '-40'; el.dispatchEvent(new Event('input', { bubbles: true })); });
    await page.waitForTimeout(400);
    const sliderLabel = await page.$eval('#accAngle', (el) => el.textContent);
    report('slider live preview triggers /ships/preview', previewHits > 0, 'labels=' + sliderLabel);
    await page.$eval('#accSlider', (el) => el.dispatchEvent(new Event('change', { bubbles: true })));
    await page.waitForTimeout(900);

    const counter = await page.$eval('#accCounter', (el) => el.textContent);
    const hint = await page.$eval('#accHint', (el) => el.textContent);
    report('counter reflects accepted count', /принято\s+\d+\s+из\s+\d+/.test(counter), counter);
    report('auto hint shown as suggestion', /авто/.test(hint), hint);
    await page.screenshot({ path: path.join(ART, 'ships-accept-mode.png'), fullPage: true });

    // --- empty state: race selected in #shipRace but absent from the pool ---
    // (leading cause of "candidate won't load": the select offers all 60 races,
    // while the pool may hold only a few). The panel must explain, not go grey.
    console.log('[step] empty-pool race');
    const racesResp = await jget('/ships/races');
    const poolRaces = new Set(list.map((x) => x.race));
    const emptyRace = (racesResp.races || []).map((r) => r.slug).find((s) => !poolRaces.has(s));
    if (!emptyRace) {
      report('a race without pool candidates exists', false, 'every race has candidates');
    } else {
      await page.selectOption('#shipRace', emptyRace);
      await page.waitForTimeout(500);
      const emptyVisible = await page.$eval('#accEmpty', (el) => getComputedStyle(el).display !== 'none').catch(() => false);
      const emptyMsg = await page.$eval('#accEmptyMsg', (el) => el.textContent).catch(() => '');
      const emptyCounter = await page.$eval('#accCounter', (el) => el.textContent);
      const stageHidden = await page.$eval('.accStage', (el) => getComputedStyle(el).display === 'none').catch(() => false);
      report('empty race shows a message (not silent)',
        emptyVisible && emptyMsg.trim().length > 0,
        'race=' + emptyRace + ' visible=' + emptyVisible + ' stageHidden=' + stageHidden + ' msg=' + JSON.stringify(emptyMsg) + ' counter=' + emptyCounter);
      const jumpVisible = await page.$eval('#accJump', (el) => getComputedStyle(el).display !== 'none').catch(() => false);
      report('empty race offers a jump-to-race hint button', jumpVisible, 'jumpVisible=' + jumpVisible);
      await page.screenshot({ path: path.join(ART, 'ships-accept-empty.png'), fullPage: true });

      await page.click('#accJump');
      await page.waitForFunction((rc) => document.getElementById('shipRace').value !== rc, emptyRace, { timeout: 5000 }).catch(() => {});
      await page.waitForFunction(() => {
        const im = document.getElementById('accImg');
        return im && im.getAttribute('src');
      }, { timeout: 5000 }).catch(() => {});
      const backRace = await page.$eval('#shipRace', (el) => el.value);
      const stillEmpty = await page.$eval('#accEmpty', (el) => getComputedStyle(el).display !== 'none').catch(() => true);
      const backImg = await page.evaluate(() => {
        const im = document.getElementById('accImg');
        return !!(im && im.getAttribute('src'));
      });
      report('jump button selects a race with candidates and loads it',
        backRace !== emptyRace && !stillEmpty && backImg,
        'race=' + backRace + ' emptyVisible=' + stillEmpty + ' img=' + backImg);
      await page.screenshot({ path: path.join(ART, 'ships-accept-after-jump.png'), fullPage: true });

      await page.selectOption('#shipRace', 'humans');
      await page.waitForTimeout(400);
    }

    report('no page JS errors in acceptance mode', pageErrors.length === 0, pageErrors.join(' | '));
    await browser.close();
    browser = null;
  }
}

let finished = false;
let watchdogTimer = null;
async function cleanup(code) {
  if (finished) return;
  finished = true;
  if (watchdogTimer) clearTimeout(watchdogTimer);
  if (browser) await browser.close().catch(() => {});
  stopServer();
  await new Promise((r) => setTimeout(r, 600));
  restore();
  const bad = results.filter((x) => !x).length;
  console.log(`RESULT: ${bad === 0 ? 'PASS' : 'FAIL'} (${results.length - bad}/${results.length})`);
  process.exit(code !== undefined ? code : (bad === 0 ? 0 : 1));
}

watchdogTimer = setTimeout(() => { console.log('[FAIL] global watchdog fired (hang)'); cleanup(3); }, 150000);

main()
  .catch((e) => { report('unexpected error', false, e && e.message ? e.message : String(e)); })
  .finally(() => cleanup());
