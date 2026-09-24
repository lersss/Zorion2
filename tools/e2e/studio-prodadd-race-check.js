// tools/e2e/studio-prodadd-race-check.js
// Точечная проверка: кнопка «+ тип» ветки «Производители» подставляет расовость
// из фильтра шапки (идея 2026-09-21_студия-расовость-фильтра-при-создании):
//   уровень «Семейство» → race_family=семейство, race=NULL;
//   уровень «Раса»      → race_family=семейство расы, race=раса;
//   уровень «Универсальный» → без расовости.
// Плюс: созданный на уровне семейства тип виден на этом уровне и не виден
// на «Универсальном» (prodVisible).
//
// Драйвим тот же код, что и кнопка (#btnProdAdd → prodAdd()), через
// page.evaluate: клики мышью в смоуке блокирует открытый попап «Описания ИИ»,
// если в студии крутится чужой ИИ-джоб описаний.
//
// Run: $env:STUDIO_USER="skycomposer"; $env:STUDIO_PASS="..."; node studio-prodadd-race-check.js
// Requires: game server on 8080, Chrome/Edge.
import { chromium } from 'playwright-core';
import { existsSync } from 'node:fs';

const BASE_URL = (process.env.BASE_URL || 'http://127.0.0.1:8080').replace(/\/+$/, '');
const STUDIO_USER = process.env.STUDIO_USER || '';
const STUDIO_PASS = process.env.STUDIO_PASS || '';

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

let browser = null;
async function finish(code) {
  if (browser) await browser.close().catch(() => {});
  process.exit(code);
}

async function main() {
  if (!STUDIO_USER || !STUDIO_PASS) {
    report('setup creds', 'FAIL', 'STUDIO_USER/STUDIO_PASS env not set');
    return finish(1);
  }
  const login = await fetch(BASE_URL + '/login', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username: STUDIO_USER, password: STUDIO_PASS }),
  });
  if (login.status !== 200) { report('login', 'FAIL', 'status=' + login.status); return finish(1); }
  const token = (await login.json()).token;
  report('login', 'PASS', '');

  const exe = findExecutable();
  if (!exe) { report('setup browser', 'FAIL', 'no Chrome/Edge'); return finish(1); }
  browser = await chromium.launch({ executablePath: exe.path, headless: true });
  const context = await browser.newContext({ viewport: { width: 1400, height: 900 } });
  await context.addInitScript((t) => localStorage.setItem('adminToken', t), token);
  const page = await context.newPage();
  const pageErrors = [];
  page.on('pageerror', (e) => pageErrors.push(String(e && e.stack ? e.stack : e)));

  const created = [];
  try {
    await page.goto(BASE_URL + '/studio', { waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.waitForFunction(() => document.getElementById('loadingOverlay').style.display === 'none', null, { timeout: 15000 });
    const races = await page.evaluate(async () => (await studioFetch('/studio/api/races')).json());
    const famId = races.families[0].id;
    const raceId = (races.races.find(r => r.family === famId) || races.races[0]).id;
    report('1 studio loads + races', races.families.length > 0 ? 'PASS' : 'FAIL',
      `families=${races.families.length} pick=${famId}/${raceId}`);

    // студия стартует во вкладке «Товары» — переключаемся на «Производители»
    // реальной вкладкой, иначе кнопка/форма «+ тип» скрыты (ТЗ §7.2)
    await page.click('#branchProdColony'); // раздел построек (спека 2026-09-25): ветка «Производители» снята
    await page.waitForSelector('#btnProdNewOpen', { state: 'visible' });

    // make(name) — тот же путь, что кнопка «+ тип»: открыть попап формы
    // (ТЗ §7.6 — #prodNewName существует только пока окно открыто), выставить
    // поля и вызвать prodAdd(). Успешное создание закрывает окно, поэтому
    // открываем заново на каждом вызове; при ошибке оно уже открыто.
    const make = async (name) => {
      // closeModal() скрывает только overlay — разметка попапа остаётся в
      // #modalBody: проверяем видимость поля, а не его наличие в DOM
      const nameVisible = await page.locator('#prodNewName').isVisible().catch(() => false);
      if (!nameVisible) {
        await page.click('#btnProdNewOpen');
        await page.waitForSelector('#prodNewName');
      }
      await page.evaluate((n) => {
        document.getElementById('prodNewName').value = n;
        document.getElementById('prodNewKind').value = 'goods';
        prodAdd();
      }, name);
      await page.waitForFunction((n) => state.producer_types.some(p => p.name === n), name, { timeout: 8000 });
      return page.evaluate((n) => {
        const p = state.producer_types.find(x => x.name === n);
        return { id: p.id, race_family: p.race_family || null, race: p.race || null };
      }, name);
    };
    const tag = Date.now();
    const nameFam = 'QA_Тип_сем_' + tag, nameRace = 'QA_Тип_раса_' + tag, nameUni = 'QA_Тип_уни_' + tag;

    // 2 — уровень «Семейство»
    await page.evaluate((f) => { setProdRaceLevel('family'); setProdRaceFamily(f); }, famId);
    const famRec = await make(nameFam);
    created.push([nameFam, famRec.id]);
    report('2 family level -> race_family only',
      (famRec.race_family === famId && !famRec.race) ? 'PASS' : 'FAIL', JSON.stringify(famRec));

    // 3 — видимость: на своём уровне виден, на «Универсальном» — нет
    const vis = await page.evaluate((id) => {
      const p = state.producer_types.find(x => x.id === id);
      const onFamily = prodVisible(p);
      setProdRaceLevel('universal');
      return { onFamily, onUniversal: prodVisible(p) };
    }, famRec.id);
    report('3 visibility family vs universal',
      (vis.onFamily === true && vis.onUniversal === false) ? 'PASS' : 'FAIL', JSON.stringify(vis));

    // 4 — уровень «Раса»
    await page.evaluate((r) => { setProdRaceLevel('race'); setProdRace(r); }, raceId);
    const raceRec = await make(nameRace);
    created.push([nameRace, raceRec.id]);
    const expectFam = (races.races.find(x => x.id === raceId) || {}).family;
    report('4 race level -> family + race',
      (raceRec.race === raceId && raceRec.race_family === expectFam) ? 'PASS' : 'FAIL',
      JSON.stringify(raceRec) + ' expectFamily=' + expectFam);

    // 5 — уровень «Универсальный»
    await page.evaluate(() => setProdRaceLevel('universal'));
    const uniRec = await make(nameUni);
    created.push([nameUni, uniRec.id]);
    report('5 universal level -> no race',
      (!uniRec.race_family && !uniRec.race) ? 'PASS' : 'FAIL', JSON.stringify(uniRec));

    // 6 — сервер подтверждает записанное (не только клиентский state)
    const server = await page.evaluate(async (names) => {
      const st = await (await studioFetch('/studio/api/state')).json();
      return names.map(n => {
        const p = st.producer_types.find(x => x.name === n);
        return p ? { name: n, race_family: p.race_family || null, race: p.race || null } : { name: n, missing: true };
      });
    }, [nameFam, nameRace, nameUni]);
    const expect = [
      [famId, null], [expectFam, raceId], [null, null],
    ];
    const ok6 = server.every((r, i) => r.race_family === expect[i][0] && r.race === expect[i][1]);
    report('6 server state matches', ok6 ? 'PASS' : 'FAIL', JSON.stringify(server));

    report('7 no page JS errors', pageErrors.length === 0 ? 'PASS' : 'FAIL',
      pageErrors.length ? pageErrors.join(' | ').slice(0, 300) : 'clean');
  } catch (err) {
    report('UNCAUGHT', 'FAIL', String(err && err.message ? err.message : err));
  }

  // cleanup — созданные типы
  try {
    const del = await page.evaluate(async (list) => {
      const t = localStorage.getItem('adminToken');
      let n = 0;
      for (const [, id] of list) {
        const r = await fetch('/studio/api/producers/' + id, { method: 'DELETE', headers: { Authorization: 'Bearer ' + t } });
        if (r.ok) n++;
      }
      return n;
    }, created);
    report('cleanup', del === created.length ? 'PASS' : 'FAIL', `deleted ${del}/${created.length}`);
  } catch (e) {
    report('cleanup', 'FAIL', String(e && e.message ? e.message : e));
  }

  const fails = results.filter(r => r.status === 'FAIL');
  console.log(`\nTOTAL: ${results.length} steps, ${fails.length} FAIL`);
  return finish(fails.length ? 1 : 0);
}

main();
