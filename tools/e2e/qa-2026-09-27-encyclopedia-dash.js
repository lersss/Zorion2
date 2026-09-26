// tools/e2e/qa-2026-09-27-encyclopedia-dash.js
// Independent smoke of two fresh edits (read-only, nothing written to DB):
//   1) dashboard "Энциклопедия": planet-type registry 9 -> 13 (mini-neptune /
//      glass / metallic / dead) + counter "X из Y" counted by the registry;
//   2) agent-dash "Идеи" + "Обзор": money printed in "₽" with 2 decimals,
//      header file counter labelled "затронуто за всё время".
//
// Console output is ASCII (PowerShell cp866 breaks Cyrillic); every Cyrillic
// fact is dumped to artifacts/qa-0927-report.json (UTF-8) for the report.
//
// Run: node qa-2026-09-27-encyclopedia-dash.js
// Env: BASE_URL (game, default http://127.0.0.1:8080), DASH_URL (default
// http://127.0.0.1:8790), QA_TOKEN (optional, else /register).
// Exit: 0 = all 10 items PASS, 1 = any FAIL.
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://127.0.0.1:8080').replace(/\/+$/, '');
const DASH_URL = (process.env.DASH_URL || 'http://127.0.0.1:8790').replace(/\/+$/, '');
const HERE = path.dirname(fileURLToPath(import.meta.url));
const ARTIFACTS_DIR = path.join(HERE, 'artifacts');
const REPORT_FILE = path.join(ARTIFACTS_DIR, 'qa-0927-report.json');

const CHROME_PATHS = [
  process.env.CHROME_PATH,
  'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe',
].filter(Boolean);
const EDGE_PATHS = [
  process.env.EDGE_PATH,
  'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe',
].filter(Boolean);

const out = {};          // Cyrillic facts -> JSON
const results = [];      // {id, status, detail}
function verdict(id, pass, detail) {
  results.push({ id, status: pass ? 'PASS' : 'FAIL', detail: String(detail ?? '') });
  console.log(`${id} ${pass ? 'PASS' : 'FAIL'} - ${detail ?? ''}`);
}
function findExecutable() {
  for (const p of CHROME_PATHS) if (existsSync(p)) return { path: p, name: 'Chrome' };
  for (const p of EDGE_PATHS) if (existsSync(p)) return { path: p, name: 'Edge' };
  return null;
}
async function getToken() {
  if (process.env.QA_TOKEN) {
    const r = await fetch(BASE_URL + '/me', { headers: { Authorization: 'Bearer ' + process.env.QA_TOKEN } });
    if (r.ok) return { token: process.env.QA_TOKEN, how: 'QA_TOKEN env' };
  }
  for (let i = 0; i < 3; i++) {
    const username = 'qa0927_' + Date.now() + '_' + i;
    const password = 'qa-pass-' + Date.now();
    const res = await fetch(BASE_URL + '/register', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password }),
    });
    if (res.status === 201) {
      const data = await res.json();
      return { token: data.token, how: 'POST /register user ' + username };
    }
    if (res.status === 409) continue;
    throw new Error('register HTTP ' + res.status + ': ' + (await res.text()));
  }
  throw new Error('register: name collision after 3 attempts');
}

let browser = null;
async function finish(code) {
  if (browser) await browser.close().catch(() => {});
  writeFileSync(REPORT_FILE, JSON.stringify({ out, results }, null, 2), 'utf8');
  const failed = results.filter((r) => r.status === 'FAIL');
  console.log('');
  console.log(`SUMMARY pass=${results.length - failed.length} fail=${failed.length}`);
  console.log('REPORT ' + REPORT_FILE);
  process.exit(code);
}

async function main() {
  mkdirSync(ARTIFACTS_DIR, { recursive: true });
  const authInfo = await getToken();
  out.auth = authInfo.how;
  console.log('auth: ' + authInfo.how);

  const exe = findExecutable();
  if (!exe) return finish(1);
  console.log('browser: ' + exe.name);
  browser = await chromium.launch({ executablePath: exe.path, headless: true });

  // ================= Scenario 1: dashboard encyclopedia =================
  const ctx1 = await browser.newContext({ viewport: { width: 1280, height: 1000 } });
  await ctx1.addInitScript((t) => { localStorage.setItem('token', t); }, authInfo.token);
  const game = await ctx1.newPage();
  const gameErrors = [];
  game.on('pageerror', (e) => gameErrors.push('pageerror: ' + (e && e.message)));
  game.on('console', (m) => {
    if (m.type() === 'error' && !m.text().includes('favicon')) gameErrors.push('console: ' + m.text());
  });

  const probe = () => game.evaluate(() => {
    const parse = (s) => {
      const m = String(s || '').match(/(\d+)\s*из\s*(\d+)/);
      return m ? { num: Number(m[1]), den: Number(m[2]) } : null;
    };
    const sections = Array.from(document.querySelectorAll('.enc-section'));
    const planetSection = sections.find((s) => (s.querySelector('h3') || {}).textContent?.includes('Планеты'));
    const header = planetSection ? planetSection.querySelector('h3').textContent : '';
    const grid = document.querySelector('#enc-planets > div:nth-child(2)');
    const cards = grid
      ? Array.from(grid.children).map((c) => ({
          title: (c.querySelector('strong') || {}).textContent || '',
          icon: (c.querySelector('span') || {}).textContent || '',
          desc: c.children[1] ? c.children[1].textContent.trim() : '',
          opened: c.textContent.includes('открыт'),
        }))
      : [];
    let statsRaw = null;
    document.querySelectorAll('#stats > div > div').forEach((cell) => {
      const label = cell.children[1] ? cell.children[1].textContent.trim() : '';
      if (label === 'Типы планет') statsRaw = cell.children[0].textContent.trim();
    });
    return { header, headerParsed: parse(header), statsRaw, statsParsed: parse(statsRaw), cards };
  });

  await game.goto(BASE_URL + '/', { waitUntil: 'domcontentloaded', timeout: 30000 });
  await game.waitForSelector('.tab-btn[data-tab="tab-encyclopedia"]', { timeout: 15000 });
  await game.click('.tab-btn[data-tab="tab-encyclopedia"]');
  await game.waitForSelector('#enc-planets > div:nth-child(2)', { timeout: 20000 });
  await game.waitForTimeout(500);
  const fresh = await probe();
  out.encFresh = fresh;
  await game.screenshot({ path: path.join(ARTIFACTS_DIR, 'qa-0927-encyclopedia-fresh.png'), fullPage: true });

  // Seeded journal: 2 registry types + 1 type outside the registry (the хвост №9
  // case) -> the counter must follow the registry, not the journal length.
  await game.evaluate(() => {
    const j = {
      visitedWorlds: ['seed-world'],
      flights: 7,
      metRaces: ['humans'],
      seenStarTypes: [],
      seenPlanetTypes: ['землеподобная', 'газовый гигант', 'не-из-реестра'],
    };
    localStorage.setItem('zorion.journal', JSON.stringify(j));
  });
  await game.reload({ waitUntil: 'domcontentloaded' });
  await game.waitForSelector('.tab-btn[data-tab="tab-encyclopedia"]', { timeout: 15000 });
  await game.click('.tab-btn[data-tab="tab-encyclopedia"]');
  await game.waitForSelector('#enc-planets > div:nth-child(2)', { timeout: 20000 });
  await game.waitForTimeout(500);
  const seeded = await probe();
  out.encSeeded = seeded;
  await game.screenshot({ path: path.join(ARTIFACTS_DIR, 'qa-0927-encyclopedia-seeded.png'), fullPage: true });

  // 1.1
  const n = fresh.cards.length;
  verdict('1.1', n === 13, `cards in #enc-planets grid = ${n} (want 13)`);
  // 1.2
  const wanted = ['мини-нептун', 'стеклянная', 'металлическая', 'мёртвая'];
  const newCards = wanted.map((t) => fresh.cards.find((c) => c.title === t) || null);
  const ok12 = newCards.every((c) => c && c.icon.trim() && c.desc.trim());
  out.newCards = newCards;
  verdict('1.2', ok12,
    newCards.map((c) => (c ? `${c.title} icon=${JSON.stringify(c.icon)} desc=${JSON.stringify(c.desc.slice(0, 45))}…` : 'MISSING')).join(' | '));
  // 1.3
  const ok13 = !!seeded.headerParsed && !!seeded.statsParsed &&
    seeded.headerParsed.num === seeded.statsParsed.num &&
    seeded.headerParsed.den === seeded.statsParsed.den &&
    seeded.headerParsed.den === 13;
  out.counterSource = { header: seeded.header, stats: seeded.statsRaw };
  verdict('1.3', ok13, `header="${seeded.header.trim()}" stats="${seeded.statsRaw}" (seeded journal, 2 registry + 1 foreign type)`);
  // 1.4
  const openedSeeded = seeded.cards.filter((c) => c.opened).length;
  const openedFresh = fresh.cards.filter((c) => c.opened).length;
  const ok14 = openedSeeded === seeded.headerParsed.num && seeded.headerParsed.num <= 13 &&
    openedFresh === fresh.headerParsed.num;
  out.opened = { fresh: openedFresh, seeded: openedSeeded, num: seeded.headerParsed.num };
  verdict('1.4', ok14, `opened cards: fresh=${openedFresh}, seeded=${openedSeeded}, numerator=${seeded.headerParsed.num}`);
  // 1.5
  verdict('1.5', gameErrors.length === 0, `JS errors on game page = ${gameErrors.length}${gameErrors.length ? ' first: ' + gameErrors[0] : ''}`);
  out.gameErrors = gameErrors;
  await ctx1.close();

  // ================= Scenario 2: agent-dash =================
  const ctx2 = await browser.newContext({ viewport: { width: 1400, height: 1000 } });
  const dash = await ctx2.newPage();
  const dashErrors = [];
  dash.on('pageerror', (e) => dashErrors.push('pageerror: ' + (e && e.message)));
  dash.on('console', (m) => {
    if (m.type() === 'error' && !m.text().includes('favicon')) dashErrors.push('console: ' + m.text());
  });

  const MONEY_RE = /-?\d[\d\u00a0 ]*[.,]?\d*\s*(?:₽|\$|руб(?:\\.|\\s)?)/g;
  const STRICT = /^-?\d+\.\d{2} ₽$/;

  // 2.4 first (overview is the default tab), then the Идеи tab.
  await dash.goto(DASH_URL + '/', { waitUntil: 'domcontentloaded', timeout: 30000 });
  await dash.waitForSelector('#cards .card', { timeout: 30000 });
  await dash.waitForTimeout(1200);
  const overview = await dash.evaluate((reSrc) => {
    const re = new RegExp(reSrc, 'g');
    const moneyOf = (s) => String(s || '').trim();
    const scan = (text) => (text.match(re) || []).map(moneyOf);
    const cards = Array.from(document.querySelectorAll('#cards .card')).map((c) => ({
      label: (c.querySelector('.dim') || {}).textContent || '',
      value: (c.querySelector('b') || {}).textContent || '',
    }));
    const agentMoney = Array.from(document.querySelectorAll('.lprice .money')).map((e) => e.textContent.trim());
    const tableMoney = Array.from(document.querySelectorAll('td[data-col="cost"], th[data-col="cost"] strong'))
      .map((e) => e.textContent.trim());
    return {
      rate: (document.getElementById('rate') || {}).textContent || '',
      cards,
      agentMoneyCount: agentMoney.length,
      agentMoneySample: agentMoney.slice(0, 3),
      tableMoneyCount: tableMoney.length,
      tableMoneySample: tableMoney.slice(0, 3),
      allMoney: scan(document.body.innerText),
      tableCount: document.querySelectorAll('#overview table').length,
      rowCount: document.querySelectorAll('#overview table tbody tr').length,
    };
  }, MONEY_RE.source);
  out.overview = { ...overview, allMoney: undefined, allMoneyCount: overview.allMoney.length };
  const ovBad = overview.allMoney.filter((m) => !STRICT.test(m));
  const ok24 = ovBad.length === 0 && overview.allMoney.length > 0 && overview.cards.length > 0 &&
    overview.tableCount > 0 && overview.rowCount > 0 && overview.rate.includes('₽');
  out.overview.rate = overview.rate;
  out.overview.bad = ovBad.slice(0, 5);
  verdict('2.4', ok24,
    `overview: money values=${overview.allMoney.length}, non-conforming=${ovBad.length} (${ovBad.slice(0, 3).join(' / ')}), rate="${overview.rate}", cards=${overview.cards.length}, tables=${overview.tableCount}, rows=${overview.rowCount}`);
  await dash.screenshot({ path: path.join(ARTIFACTS_DIR, 'qa-0927-dash-overview.png'), fullPage: false });

  await dash.click('#tab-ideas');
  await dash.waitForSelector('#ideas .cards .card', { timeout: 40000 });
  await dash.waitForSelector('#ideas tr[data-ideas-line]', { timeout: 40000 });
  await dash.waitForTimeout(500);
  // expand the first line -> roles pills + session price table
  await dash.click('#ideas tr[data-ideas-line]');
  await dash.waitForTimeout(400);

  const ideas = await dash.evaluate((reSrc) => {
    const re = new RegExp(reSrc, 'g');
    const scan = (text) => (text.match(re) || []).map((s) => s.trim());
    const num = (s) => parseFloat(String(s).replace(/[^\d.-]/g, ''));
    const box = document.getElementById('ideas');
    const cards = Array.from(box.querySelectorAll('.cards .card')).map((c) => ({
      label: (c.querySelector('.dim') || {}).textContent || '',
      value: (c.querySelector('b') || {}).textContent || '',
    }));
    const lineRows = Array.from(box.querySelectorAll('tr[data-ideas-line]')).map((tr) => {
      const td = tr.querySelectorAll('td');
      return {
        label: td[0] ? td[0].textContent.trim() : '',
        cost: td[4] ? td[4].textContent.trim() : '',
        costNum: td[4] ? num(td[4].textContent) : NaN,
      };
    });
    const rolePills = Array.from(box.querySelectorAll('span.pill'))
      .map((s) => s.textContent.trim()).filter((t) => t.includes('₽'));
    const sessPrices = Array.from(box.querySelectorAll('table td.num'))
      .map((td) => td.textContent.trim()).filter((t) => t.includes('₽'));
    const unattachedHead = Array.from(box.querySelectorAll('h2')).map((h) => h.textContent.trim());
    const header = box.querySelector('.livehead') ? box.querySelector('.livehead').innerText.replace(/\n/g, ' | ') : '';
    return {
      header,
      cards,
      lineCount: lineRows.length,
      sumLines: lineRows.reduce((a, r) => a + (isFinite(r.costNum) ? r.costNum : 0), 0),
      lineSample: lineRows.slice(0, 3),
      rolePillCount: rolePills.length,
      rolePillSample: rolePills.slice(0, 3),
      sessPriceCount: sessPrices.length,
      sessPriceSample: sessPrices.slice(0, 3),
      unattachedHead,
      unattachedRowCount: box.querySelectorAll('h2 + .scroll table tbody tr').length,
      allMoney: scan(box.innerText),
    };
  }, MONEY_RE.source);
  out.ideas = { ...ideas, allMoney: undefined, allMoneyCount: ideas.allMoney.length, allMoneySample: ideas.allMoney.slice(0, 6) };
  await dash.screenshot({ path: path.join(ARTIFACTS_DIR, 'qa-0927-dash-ideas.png'), fullPage: false });

  // 2.1
  const bad = ideas.allMoney.filter((m) => !STRICT.test(m));
  out.ideas.bad = bad.slice(0, 8);
  const ok21 = ideas.allMoney.length > 0 && bad.length === 0 &&
    ideas.rolePillCount > 0 && ideas.sessPriceCount > 0 &&
    ideas.unattachedRowCount > 0;
  verdict('2.1', ok21,
    `ideas: money values=${ideas.allMoney.length}, non-conforming=${bad.length} (${bad.slice(0, 3).join(' / ')}); role pills=${ideas.rolePillCount}, session prices=${ideas.sessPriceCount}, unattached rows=${ideas.unattachedRowCount}`);
  // 2.2
  const priceCard = ideas.cards.find((c) => c.label.includes('Цена идей'));
  const cardNum = priceCard ? parseFloat(priceCard.value.replace(/[^\d.-]/g, '')) : NaN;
  const diff = Math.abs(ideas.sumLines - cardNum);
  out.sum = { sumLines: ideas.sumLines, card: priceCard, cardNum, diff };
  const ok22 = !!priceCard && ideas.lineCount > 0 && diff <= 0.01;
  verdict('2.2', ok22,
    `sum of ${ideas.lineCount} line costs = ${ideas.sumLines.toFixed(2)}, card "Цена идей" = ${priceCard ? priceCard.value : 'MISSING'}, diff = ${diff.toFixed(4)}`);
  // 2.3
  const ok23 = ideas.header.includes('затронуто за всё время');
  out.ideasHeader = ideas.header;
  verdict('2.3', ok23, `header counter label: ${ok23 ? 'contains "затронуто за всё время"' : 'MISSING the phrase'}`);
  // 2.5
  verdict('2.5', dashErrors.length === 0, `JS errors on dash page = ${dashErrors.length}${dashErrors.length ? ' first: ' + dashErrors[0] : ''}`);
  out.dashErrors = dashErrors;
  await ctx2.close();

  const failed = results.filter((r) => r.status === 'FAIL');
  return finish(failed.length ? 1 : 0);
}

main().catch((e) => {
  console.log('RUN FAIL - ' + String((e && e.message) || e));
  return finish(1);
});
