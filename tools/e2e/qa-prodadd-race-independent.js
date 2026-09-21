// tools/e2e/qa-prodadd-race-independent.js
// Independent QA check (@tester) of the "+ тип" race-from-header-filter fix
// (idea 2026-09-21_студия-расовость-фильтра-при-создании, diff in prodAdd).
// NOT a re-run of the manager's script: drives the REAL header <select>s and
// the REAL #btnProdAdd button, then verifies the created record through the
// server (/studio/api/state) via Node fetch — NOT the page's `state` variable.
// Visibility is read from buildProdTree() (the actual tree builder), not from
// prodVisible() directly.
//
// Run: $env:STUDIO_USER="skycomposer"; $env:STUDIO_PASS="..."; node qa-prodadd-race-independent.js
import { chromium } from 'playwright-core';
import { existsSync } from 'node:fs';

const BASE_URL = (process.env.BASE_URL || 'http://127.0.0.1:8080').replace(/\/+$/, '');
const STUDIO_USER = process.env.STUDIO_USER || '';
const STUDIO_PASS = process.env.STUDIO_PASS || '';
const ARTIFACTS = 'artifacts';

const CHROME_PATHS = [process.env.CHROME_PATH, 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'].filter(Boolean);
const EDGE_PATHS = [process.env.EDGE_PATH, 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'].filter(Boolean);

const results = [];
function report(step, status, detail) {
  results.push({ step, status });
  console.log(`[${step}] ${status}${detail ? ' - ' + detail : ''}`);
}
function findExecutable() {
  for (const p of CHROME_PATHS) if (existsSync(p)) return { path: p, name: 'Chrome' };
  for (const p of EDGE_PATHS) if (existsSync(p)) return { path: p, name: 'Edge' };
  return null;
}

let token = '';
async function api(method, url, body) {
  const headers = { 'Content-Type': 'application/json' };
  if (token) headers['Authorization'] = 'Bearer ' + token;
  const r = await fetch(BASE_URL + url, { method, headers, body: body ? JSON.stringify(body) : undefined });
  let data = null;
  try { data = await r.json(); } catch (e) {}
  return { status: r.status, data };
}
async function serverRecord(name) {
  const st = await api('GET', '/studio/api/state');
  return (st.data.producer_types || []).find(p => p.name === name) || null;
}

let browser = null;
async function finish(code) {
  if (browser) await browser.close().catch(() => {});
  process.exit(code);
}

async function main() {
  if (!STUDIO_USER || !STUDIO_PASS) { report('setup creds', 'FAIL', 'STUDIO_USER/STUDIO_PASS not set'); return finish(1); }

  const login = await api('POST', '/login', { username: STUDIO_USER, password: STUDIO_PASS });
  if (login.status !== 200 || !login.data || !login.data.token) { report('login', 'FAIL', 'status=' + login.status); return finish(1); }
  token = login.data.token;
  report('login', 'PASS', 'role=' + (login.data.user && login.data.user.role));

  const racesResp = await api('GET', '/studio/api/races');
  if (racesResp.status !== 200) { report('races', 'FAIL', 'status=' + racesResp.status); return finish(1); }
  const families = racesResp.data.families || [];
  const races = racesResp.data.races || [];
  const famId = families.some(f => f.id === 'F2') ? 'F2' : (families[0] && families[0].id);
  const raceId = (races.find(r => r.family === famId) || races[0]).id;
  const expectFam = (races.find(r => r.id === raceId) || {}).family;
  report('setup races', (families.length && raceId) ? 'PASS' : 'FAIL',
    `families=${families.length} fam=${famId} race=${raceId} expectFamily=${expectFam}`);

  const exe = findExecutable();
  if (!exe) { report('setup browser', 'FAIL', 'no Chrome/Edge'); return finish(1); }
  console.log('browser: ' + exe.name + ' (' + exe.path + ')');
  browser = await chromium.launch({ executablePath: exe.path, headless: true });
  const context = await browser.newContext({ viewport: { width: 1500, height: 950 } });
  await context.addInitScript((t) => {
    localStorage.setItem('adminToken', t);
    localStorage.setItem('gs_branch', 'producers');
    localStorage.removeItem('gs_prodRaceLevel');
    localStorage.removeItem('gs_prodRaceFamily');
    localStorage.removeItem('gs_prodRace');
  }, token);
  const page = await context.newPage();
  const pageErrors = [];
  page.on('pageerror', (e) => pageErrors.push(String(e && e.stack ? e.stack : e)));

  const created = []; // [name, id]
  const tag = Date.now();
  const nameFam = 'QA_Ind_fam_' + tag, nameRace = 'QA_Ind_race_' + tag, nameUni = 'QA_Ind_uni_' + tag;
  try {
    await page.goto(BASE_URL + '/studio', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.waitForFunction(() => document.getElementById('loadingOverlay').style.display === 'none', null, { timeout: 15000 });
    await page.waitForFunction(() => document.getElementById('prodRaceLevelSel').options.length >= 3, null, { timeout: 10000 });
    const branchOK = await page.evaluate(() => {
      return document.getElementById('btnProdNewOpen').style.display !== 'none'
        && document.getElementById('segProdRace').style.display !== 'none';
    });
    report('1 producers branch + real controls', branchOK ? 'PASS' : 'FAIL',
      'btnProdNewOpen/segProdRace visible=' + branchOK);

    // выбрать уровень/семейство/расу РЕАЛЬНЫМИ селектами шапки (change → onchange)
    const setLevel = async (lvl) => { await page.selectOption('#prodRaceLevelSel', lvl); await page.waitForTimeout(150); };
    const setFamily = async (f) => { await page.selectOption('#prodRaceFamilySel', f); await page.waitForTimeout(150); };
    const setRace = async (r) => { await page.selectOption('#prodRaceSel', r); await page.waitForTimeout(150); };

    // «+ тип»: форма живёт в попапе (ТЗ §7.6) — открыть окно, заполнить имя,
    // нажать РЕАЛЬНУЮ кнопку (fallback — DOM click, если попап «Описания ИИ»
    // перехватывает мышь). Окно закрывается при успехе, поэтому открываем его
    // заново на каждом вызове; при ошибке оно уже открыто.
    const createViaUI = async (name) => {
      // closeModal() скрывает только overlay — разметка попапа остаётся в
      // #modalBody, поэтому «поле есть в DOM» ≠ «окно открыто»: проверяем
      // видимость, а не существование
      const nameVisible = await page.locator('#prodNewName').isVisible().catch(() => false);
      if (!nameVisible) {
        await page.click('#btnProdNewOpen');
        await page.waitForSelector('#prodNewName');
      }
      await page.fill('#prodNewName', name);
      try {
        await page.click('#btnProdAdd', { timeout: 4000 });
      } catch (e) {
        await page.evaluate(() => document.getElementById('btnProdAdd').click());
      }
      const deadline = Date.now() + 10000;
      while (Date.now() < deadline) {
        const rec = await serverRecord(name);
        if (rec) return rec;
        await page.waitForTimeout(250);
      }
      return null;
    };

    // ---------- 2: уровень «Семейство» ----------
    await setLevel('family');
    await setFamily(famId);
    const filterFam = await page.evaluate(() => ({ level: prodRaceLevel, family: prodRaceFamily, race: prodRace }));
    const recFam = await createViaUI(nameFam);
    if (recFam) created.push([nameFam, recFam.id]);
    const ok2 = recFam && recFam.race_family === famId && !recFam.race && !recFam.parent_id;
    report('2 family level -> race_family only (server)',
      ok2 ? 'PASS' : 'FAIL',
      `filter=${JSON.stringify(filterFam)} server=${JSON.stringify(recFam && { id: recFam.id, parent_id: recFam.parent_id, race_family: recFam.race_family || null, race: recFam.race || null })}`);
    if (!ok2) { report('STOP', 'FAIL', 'пункт 2 не прошёл — стоп-критерий'); }

    if (ok2) {
      // ---------- 3: уровень «Раса» ----------
      await setLevel('race');
      await setRace(raceId);
      const filterRace = await page.evaluate(() => ({ level: prodRaceLevel, family: prodRaceFamily, race: prodRace }));
      const recRace = await createViaUI(nameRace);
      if (recRace) created.push([nameRace, recRace.id]);
      const ok3 = recRace && recRace.race === raceId && recRace.race_family === expectFam && !recRace.parent_id;
      report('3 race level -> race + its family (server)',
        ok3 ? 'PASS' : 'FAIL',
        `filter=${JSON.stringify(filterRace)} server=${JSON.stringify(recRace && { id: recRace.id, race_family: recRace.race_family || null, race: recRace.race || null })} expectFamily=${expectFam}`);

      // ---------- 4: уровень «Универсальный» ----------
      await setLevel('universal');
      const recUni = await createViaUI(nameUni);
      if (recUni) created.push([nameUni, recUni.id]);
      const ok4 = recUni && !recUni.race_family && !recUni.race && !recUni.parent_id;
      report('4 universal level -> no race (server)',
        ok4 ? 'PASS' : 'FAIL',
        `server=${JSON.stringify(recUni && { id: recUni.id, race_family: recUni.race_family || null, race: recUni.race || null })}`);

      // ---------- 5a: видимость в дереве (buildProdTree) ----------
      await setLevel('family');
      await setFamily(famId);
      const treeAtFamily = await page.evaluate(() => {
        const ids = [];
        (function walk(n) { ids.push(n.id); n.children.forEach(walk); })(buildProdTree());
        return ids;
      });
      await setLevel('universal');
      const treeAtUni = await page.evaluate(() => {
        const ids = [];
        (function walk(n) { ids.push(n.id); n.children.forEach(walk); })(buildProdTree());
        return ids;
      });
      const nodeId = 'type:' + recFam.id;
      const visFam = treeAtFamily.includes(nodeId);
      const visUni = treeAtUni.includes(nodeId);
      const ok5a = visFam === true && visUni === false;
      report('5a created family type visible in tree @family, hidden @universal',
        ok5a ? 'PASS' : 'FAIL', `node=${nodeId} trFamily=${visFam} trUniversal=${visUni}`);

      // ---------- 5b: JS errors ----------
      report('5b no page JS errors', pageErrors.length === 0 ? 'PASS' : 'FAIL',
        pageErrors.length ? pageErrors.join(' | ').slice(0, 300) : 'clean');

      await page.screenshot({ path: ARTIFACTS + '/qa-prodadd-race-independent.png' }).catch(() => {});
    }
  } catch (err) {
    report('UNCAUGHT', 'FAIL', String(err && err.message ? err.message : err));
  }

  // ---------- cleanup: удалить созданные QA_ типы и проверить ----------
  try {
    let deleted = 0;
    for (const [, id] of created) {
      const r = await api('DELETE', '/studio/api/producers/' + id);
      if (r.status === 200) deleted++;
    }
    const st = await api('GET', '/studio/api/state');
    const leftover = (st.data.producer_types || []).filter(p => p.name.startsWith('QA_Ind_'));
    report('cleanup', (deleted === created.length && leftover.length === 0) ? 'PASS' : 'FAIL',
      `deleted ${deleted}/${created.length} leftover=${leftover.length}`);
  } catch (e) {
    report('cleanup', 'FAIL', String(e && e.message ? e.message : e));
  }

  const fails = results.filter(r => r.status === 'FAIL');
  console.log(`\nTOTAL: ${results.length} steps, ${fails.length} FAIL`);
  return finish(fails.length ? 1 : 0);
}

main();
