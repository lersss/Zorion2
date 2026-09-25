// tools/e2e/surface-walk-field-check.js
// Э5.1 «Мир прогулки: скульптурный объём» — фундамент: e2e-смоук прогулки
// (клиентский флоу: ходьба + угловая коллизия) на реальном мире.
// Отличие от surface-walk-check.js: игрок сажается на орбиту планеты SQL-ом
// напрямую — штатный setup через `/api/intrasystem-flight` на момент Э5.1 не
// доводит игрока до планеты (чужой незакоммиченный код в internal/travel;
// к правке Э5.1 отношения не имеет). Дальше флоу тот же: land → брифинг →
// прогулка → ходьба/прыжок → «Вызвать корабль».
// W7 — Э5.3: высадка на биом-цель (гроты/своды/трубки/кратеры) через админский
// выбор биома и ходьба по нему (вход под свод/в грот), 0 pageerror, без застреваний.
// Run: node surface-walk-field-check.js   (BASE_URL env)
import { chromium } from 'playwright-core';
import { existsSync, mkdirSync, writeFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const BASE_URL = (process.env.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
const ARTIFACTS_DIR = path.join(path.dirname(fileURLToPath(import.meta.url)), 'artifacts');
const CHROME_PATHS = [process.env.CHROME_PATH, 'C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe'].filter(Boolean);
const EDGE_PATHS = [process.env.EDGE_PATH, 'C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe'].filter(Boolean);
const PASSWORD = 'e2e-walkf-' + Date.now();

const results = [];
function report(step, ok, detail) { results.push({ step, ok, detail }); console.log(`[${step}] ${ok ? 'PASS' : 'FAIL'} - ${detail}`); }
function findExecutable() { for (const p of CHROME_PATHS) if (existsSync(p)) return p; for (const p of EDGE_PATHS) if (existsSync(p)) return p; return null; }
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
function psqlRun(sql, filename) {
  const sqlPath = path.join(ARTIFACTS_DIR, filename);
  writeFileSync(sqlPath, sql, 'utf8');
  return String(execFileSync('cmd', ['/c', process.env.PSQL || 'C:\\pgsql\\pgsql\\bin\\psql.exe',
    '-h', '127.0.0.1', '-U', 'zorion', '-d', 'zorion', '-t', '-A', '-f', sqlPath],
    { env: { ...process.env, PGPASSWORD: process.env.PGPASSWORD || 'zorion123' } })).trim();
}
function resolveSoftPlanet() {
  const sql = `SELECT p.world_id, p.id FROM planets p
WHERE jsonb_typeof(p.data->'biomes') = 'array' AND jsonb_array_length(p.data->'biomes') >= 3
  AND (p.data->>'temperature')::float BETWEEN 263 AND 313
  AND COALESCE((p.data->'atmosphere_data'->>'pressure_atm')::float, 0) BETWEEN 0.5 AND 3.0
  AND COALESCE((p.data->>'life')::bool, false) = true
ORDER BY COALESCE((p.data->'core'->>'radioactivity')::float, 999) ASC LIMIT 1;
`;
  const line = psqlRun(sql, 'surface-walkf-resolve.sql').split(/\r?\n/).map((s) => s.trim()).filter(Boolean).pop();
  if (!line) throw new Error('не найдена живая мягкая планета');
  const [world, planet] = line.split('|');
  return { world, planet };
}
// moveToOrbit — SQL-перевод игрока на орбиту планеты + роль (по умолчанию player:
// роль admin нужна только W7 — админский выбор биома `?biome=` требует
// admin/skycomposer, см. Land; W1–W6 идут под обычной ролью).
function moveToOrbit(userId, world, planet, filename, role = 'player') {
  const pos = JSON.stringify({ status: 'orbit', object_type: 'planet', object_id: planet, level: 'orbit' }).replace(/'/g, "''");
  psqlRun(`UPDATE users SET role='${role}', current_world_id='${world}', current_position='${pos}'::jsonb WHERE id='${userId}';\n`, filename);
}
// resolveTargetPlanet — планета с биомом-целью Э5.3 (предпочтение — пещерный мир).
function resolveTargetPlanet() {
  const sql = `SELECT p.world_id, p.id, b->>'form' FROM planets p, jsonb_array_elements(p.data->'biomes') b
WHERE b->>'form' IN ('пещерный_мир_с_потолком','лавовые_поля','магмовый_океан','кратеры')
ORDER BY (b->>'form' = 'пещерный_мир_с_потолком') DESC, random() LIMIT 1;
`;
  const line = psqlRun(sql, 'surface-walkf-target-resolve.sql').split(/\r?\n/).map((s) => s.trim()).filter(Boolean).pop();
  if (!line) return null;
  const [world, planet, biome] = line.split('|');
  return { world, planet, biome };
}
async function setupOnOrbit(world, planet) {
  const username = 'e2e_walkf_' + Date.now();
  const res = await fetch(BASE_URL + '/register', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ username, password: PASSWORD }) });
  if (res.status !== 201) throw new Error('register HTTP ' + res.status);
  const { token } = await res.json();
  const me = await (await fetch(BASE_URL + '/me', { headers: { Authorization: 'Bearer ' + token } })).json();
  moveToOrbit(me.id, world, planet, 'surface-walkf-setup.sql');
  const me2 = await (await fetch(BASE_URL + '/me', { headers: { Authorization: 'Bearer ' + token } })).json();
  if (!me2.current_position || me2.current_position.object_id !== planet) throw new Error('не на орбите планеты: ' + JSON.stringify(me2.current_position));
  return { token, username, id: me.id };
}
// login — свежий токен после смены роли в БД: Land читает роль из токена
// (surface_handlers.go:542), SQL-правка role='admin' без перелогина не видна.
async function login(username, password) {
  const res = await fetch(BASE_URL + '/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ username, password }) });
  if (res.status !== 200) throw new Error('login HTTP ' + res.status);
  const { token } = await res.json();
  return token;
}

let browser = null;
async function finish(code) { if (browser) await browser.close().catch(() => {}); writeFileSync(path.join(ARTIFACTS_DIR, 'surface-walk-field-results.json'), JSON.stringify(results, null, 2)); process.exit(code); }

async function main() {
  mkdirSync(ARTIFACTS_DIR, { recursive: true });
  const exe = findExecutable();
  if (!exe) { console.log('NO BROWSER FOUND'); await finish(1); }
  const { world, planet } = resolveSoftPlanet();
  console.log('world/planet: ' + world + ' / ' + planet);
  const setup = await setupOnOrbit(world, planet);
  console.log('user: ' + setup.username);

  browser = await chromium.launch({ executablePath: exe, headless: true, args: ['--no-sandbox'] });
  const context = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  await context.addInitScript((t) => { try { localStorage.setItem('token', t); } catch (e) {} }, setup.token);
  const page = await context.newPage();
  const pageErrors = [];
  const consoleErrors = [];
  page.on('pageerror', (e) => pageErrors.push(String(e)));
  page.on('console', (m) => { if (m.type() === 'error') consoleErrors.push(m.text()); });

  try {
    await page.goto(BASE_URL + '/surface.html?planet=' + planet, { waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.waitForSelector('#briefing', { state: 'visible', timeout: 25000 });
    report('W1 брифинг высадки отрисован', true, (await page.evaluate(() => document.getElementById('briefing-content').textContent.trim().slice(0, 80))));
    await page.click('#brief-continue');
    await page.waitForSelector('#hud', { state: 'visible', timeout: 10000 });
    await page.waitForTimeout(1500);

    const worldDrawn = await page.evaluate(() => {
      const c = document.getElementById('surface-canvas');
      const d = c.getContext('2d').getImageData(0, 0, c.width, c.height).data;
      const set = new Set();
      for (let i = 0; i < d.length; i += 4 * 97) set.add((d[i] << 16) | (d[i + 1] << 8) | d[i + 2]);
      return set.size;
    });
    report('W2 мир рисуется (canvas не одноцветный)', worldDrawn > 10, 'colors=' + worldDrawn);

    // Ходьба вправо: угловая коллизия не должна запирать игрока на подъёмах.
    const d0 = await page.evaluate(() => parseInt(document.getElementById('hud-distance').textContent, 10) || 0);
    await page.keyboard.down('ArrowRight');
    await page.waitForTimeout(2600);
    await page.keyboard.up('ArrowRight');
    await page.keyboard.down('ArrowLeft');
    await page.waitForTimeout(1600);
    await page.keyboard.up('ArrowLeft');
    const d1 = await page.evaluate(() => parseInt(document.getElementById('hud-distance').textContent, 10) || 0);
    report('W3 персонаж идёт (нет застревания на рельефе)', d1 - d0 > 2, `distance ${d0}m → ${d1}m (Δ=${d1 - d0})`);

    // Прыжок.
    await page.waitForTimeout(500);
    const jumpBefore = await page.evaluate(() => {
      const el = document.getElementById('surface-canvas');
      return el.width;
    });
    await page.keyboard.down('Space');
    await page.waitForTimeout(400);
    await page.keyboard.up('Space');
    await page.waitForTimeout(600);
    report('W4 прыжок без ошибок', pageErrors.length === 0, 'pageErrors=' + pageErrors.length);

    const hp = await page.evaluate(() => document.getElementById('hp-text').textContent);
    report('W5 HUD здоровья жив', /100\s*\/\s*100/.test(hp), 'hp=' + hp);

    const jsErr = consoleErrors.filter((t) => !/Failed to load resource/i.test(t));
    report('W6 0 pageerror/JS-ошибок', pageErrors.length === 0 && jsErr.length === 0,
      'pageErrors=' + pageErrors.length + ' jsErrors=' + jsErr.length + (pageErrors[0] ? ' | ' + pageErrors[0] : ''));

    await page.screenshot({ path: path.join(ARTIFACTS_DIR, 'surface-walk-field.png') });

    // W7 — биом-цель Э5.3 (§B.1.3): РЕАЛЬНЫЙ вход под свод/в грот (не только
    // Δdistance>2). Высадка на биом с формами (админский выбор биома), поиск
    // форм впереди по полю (`window.__surface.world`), детерминированный подход к
    // краю, ВХОД в форму и ВЫХОД обратно пешком (см. ниже). Кандидаты перебираются:
    // если подход перекрыт базой биома (игла/провал — не Э5.3) — SKIP, не FAIL.
    // Проверяем: игрок реально вошёл — ноги ниже базовой поверхности в вырезе
    // (`void`/впадина `crater`) ИЛИ колонка накрыта аддитивной формой
    // (`overhang`/вал); затем ВЫШЕЛ ВНЕ ВЫРЕЗА — вне вертикального интервала формы
    // с опорой (`floorY`/поверхность; как `cutAt ≤ 1.5` в `_simulateEscape`).
    // База (пещеры/парящий камень) ложный FAIL не даёт; 0 pageerror — обязательно.
    const target = resolveTargetPlanet();
    if (!target) {
      report('W7 биом-цель Э5.3 (гроты/своды): вход под свод/в грот', false, 'нет планеты с биомом-целью в БД');
    } else {
      console.log('Э5.3 target: ' + target.world + ' / ' + target.planet + ' / ' + target.biome);
      moveToOrbit(setup.id, target.world, target.planet, 'surface-walkf-target-setup.sql', 'admin');
      // Роль admin — в БД; токен несёт роль на момент логина, поэтому перелогин:
      // иначе Land отклонит `biome` (роль из токена, §2 п.3).
      const adminToken = await login(setup.username, PASSWORD);
      const ctx2 = await browser.newContext({ viewport: { width: 1280, height: 800 } });
      await ctx2.addInitScript((t) => { try { localStorage.setItem('token', t); } catch (e) {} }, adminToken);
      const page2 = await ctx2.newPage();
      const pageErrors2 = [];
      page2.on('pageerror', (e) => pageErrors2.push(String(e)));
      await page2.goto(BASE_URL + '/surface.html?planet=' + target.planet + '&biome=' + encodeURIComponent(target.biome), { waitUntil: 'domcontentloaded', timeout: 30000 });
      await page2.waitForSelector('#briefing', { state: 'visible', timeout: 25000 });
      const biomeShown = await page2.evaluate(() => document.getElementById('briefing-content').textContent.trim().slice(0, 60));
      await page2.click('#brief-continue');
      await page2.waitForSelector('#hud', { state: 'visible', timeout: 10000 });
      await page2.waitForTimeout(1200);

      // Кандидаты-формы впереди от спавна: НАЧАЛА связных вырезов (sub) и, если
      // вырезов нет, аддитивов (add). Цель — вход в грот/трубу/чашу (sub); иначе
      // плита-свод. Список нужен для перебора: подход к ближней форме может
      // перекрыть 1D-игла/провал базы биома (не Э5.3) — тогда пробуем следующую,
      // а не падаем (D1-remain, замечание @tester).
      const field = await page2.evaluate(() => {
        const s = window.__surface, w = s.world, p = s.player;
        const subs = [], adds = [];
        let prevSub = false, prevAdd = false;
        for (let x = p.x; x < p.x + 5000; x += 2) {
          const fs = w.formSpans(x, w.terrainHeight(x));
          const hasSub = !!(fs && fs.sub && fs.sub.length);
          const hasAdd = !!(fs && fs.add && fs.add.length);
          if (hasSub && !prevSub && subs.length < 8) subs.push(x);
          if (hasAdd && !prevAdd && adds.length < 8) adds.push(x);
          prevSub = hasSub; prevAdd = hasAdd;
        }
        return { startX: p.x, subs, adds };
      });
      const needSub = field.subs.length > 0;
      const candidates = needSub ? field.subs : field.adds;

      // Сэмпл состояния. Критерий «вне выреза» — ОПОРА на полу вне выреза формы:
      // ноги НЕ внутри вертикального интервала выреза + `onGround` + опора на
      // `floorY` (как `cutAt ≤ 1.5` в `_simulateEscape`); НЕ «отсутствие спуска»
      // от `terrainHeight`: пещеры/подповерхностный рельеф базы (`caves`) и
      // сэмплинг 100 мс давали ложный FAIL (D1-remain).
      const sample = () => page2.evaluate(() => {
        const s = window.__surface, w = s.world, p = s.player;
        const th = w.terrainHeight(p.x);
        const fs = w.formSpans(p.x, th);
        const foot = p.y + p.h / 2;
        // «В вырезе» — ноги СТРОГО внутри вертикального интервала формы
        // (`top..bottom`): на поверхности у кромки (`foot≈top`) или НИЖЕ дна
        // (`foot>bottom` — базовая пещера под вырезом) это ВНЕ выреза формы.
        let inCut = false, cut = 0, cutBottom = th;
        if (fs && fs.sub) for (const it of fs.sub) {
          cut = Math.max(cut, it.bottom - it.top);
          cutBottom = Math.max(cutBottom, it.bottom);
          if (foot > it.top + 1.5 && foot < it.bottom - 1.5) inCut = true;
        }
        // Опора: ноги у пола `floorY` ИЛИ не ниже местной поверхности (`th`).
        // Вторая ветка обязательна для `пещерного_мира` (`float:true`): парящая
        // полоса — валидная опора, но в `floorY` не входит (это потолок, §5 п.12),
        // и без неё игрок на парящем камне давал бы ложный FAIL.
        const onFloor = Math.abs(foot - w.floorY(p.x)) <= 8 || foot <= th + 2;
        return {
          x: p.x, foot, cutBottom, inCut, cut, descent: foot - th, onGround: p.onGround,
          onFloor, floorGap: foot - w.floorY(p.x),
          sub: cut > 0, add: !!(fs && fs.add && fs.add.length),
        };
      });
      let maxX = field.startX, minDescent = 0;
      const track = (st, acc) => {
        maxX = Math.max(maxX, st.x);
        acc.last = st;
        if (st.sub) acc.minDescent = Math.max(acc.minDescent, st.descent);
        if (st.sub && st.descent > 5) acc.enteredSub = true;
        if (st.add) acc.enteredAdd = true;
        // «Вышли»: были в форме и оказались ВНЕ её выреза — не внутри вертикального
        // интервала формы (`!inCut`) и на опоре: стоим на полу (`onGround`) ЛИБО
        // ушли из колонок формы / ниже её дна (базовая пещера под вырезом — не
        // заслуга/вина Э5.3, ложный FAIL не даём). НЕ «нет спуска от terrainHeight».
        if (needSub) acc.settled = acc.settled || (acc.enteredSub && !st.inCut
          && (st.onGround || !st.sub || st.foot >= st.cutBottom - 1.5));
        else acc.settled = acc.settled || (acc.enteredAdd && !st.add && st.onGround && st.onFloor);
      };

      // Перебор ближайших кандидатов: подход к форме может перекрыть 1D-игла
      // (`spike`, h 60–160) или провал в пещеру (`caves` базы биома — не Э5.3).
      // Это не дефект игры: такой кандидат пропускаем и пробуем следующий, а если
      // достижимых нет — честный SKIP, не FAIL. Телепорт — ТОЛЬКО к подходу
      // (перед краем формы); вход и выход — реальной физикой.
      let outcome = 'skip';
      let usedGoal = null;
      let lastAcc = null;
      for (const goal of candidates) {
        const acc = { enteredSub: false, enteredAdd: false, settled: false, minDescent: 0, last: null };
        lastAcc = acc;
        await page2.evaluate((gx) => {
          const s = window.__surface, w = s.world, p = s.player;
          const tx = gx - 40;
          p.x = tx; p.vx = 0; p.vy = 0; p.y = w.floorY(tx) - p.h / 2 - 2;
        }, goal);
        await page2.waitForTimeout(150);
        await page2.keyboard.down('ShiftLeft');
        // Вход: идём вправо, пока не спустились в вырез (ИЛИ колонка под формой).
        await page2.keyboard.down('ArrowRight');
        for (let t = 0; t < 40 && !(needSub ? acc.enteredSub : acc.enteredAdd); t++) {
          await page2.waitForTimeout(100);
          track(await sample(), acc);
        }
        await page2.keyboard.up('ArrowRight');
        // Выход: назад тем же входом, а если назад закрыто — вправо (могли бы выйти
        // с другой стороны). «Не заперт» (§5.1 п.5) = выйти из выреза на пол.
        await page2.keyboard.down('ArrowLeft');
        for (let t = 0; t < 60 && !acc.settled; t++) {
          await page2.waitForTimeout(100);
          track(await sample(), acc);
        }
        await page2.keyboard.up('ArrowLeft');
        if (!acc.settled) {
          await page2.keyboard.down('ArrowRight');
          for (let t = 0; t < 60 && !acc.settled; t++) {
            await page2.waitForTimeout(100);
            track(await sample(), acc);
          }
          await page2.keyboard.up('ArrowRight');
        }
        await page2.keyboard.up('ShiftLeft');
        if (acc.minDescent > minDescent) minDescent = acc.minDescent;
        const entered = needSub ? acc.enteredSub : acc.enteredAdd;
        if (entered && acc.settled) { outcome = 'pass'; usedGoal = goal; break; }
        // Вошли, но не вышли — реальная ловушка: честный FAIL.
        if (entered && !acc.settled) { outcome = 'fail'; usedGoal = goal; break; }
        // Не вошли (путь к форме перекрыт базой биома) — пробуем следующего.
      }
      await page2.screenshot({ path: path.join(ARTIFACTS_DIR, 'surface-walk-field-grotto.png') });
      await ctx2.close();

      const kind = needSub ? 'грот/труба/чаша' : 'свод/вал';
      const pageOk = pageErrors2.length === 0;
      // PASS — реально вошли в вырез и вышли на пол физикой; SKIP — ни к одной
      // форме подход не проходим (иглы/провалы базы, не Э5.3). 0 pageerror —
      // обязательно и для PASS, и для SKIP.
      const okReport = pageOk && outcome === 'pass';
      const skipped = pageOk && outcome === 'skip';
      const goalTxt = usedGoal !== null ? Math.round(usedGoal) : (candidates.length ? 'нет достижимой' : 'нет формы в 5000px');
      // ASCII-диагностика последнего сэмпла: g=onGround, f=onFloor, c=inCut,
      // fg=foot−floorY (нужна для разбора ложных FAIL, читается без локали).
      const l = (lastAcc && lastAcc.last) || null;
      const diag = l ? ` [last g=${l.onGround ? 1 : 0} f=${l.onFloor ? 1 : 0} c=${l.inCut ? 1 : 0} fg=${l.floorGap.toFixed(1)} d=${l.descent.toFixed(1)}]` : '';
      report('W7 биом-цель Э5.3 (гроты/своды): вход под свод/в грот — вход, выход, 0 pageerror',
        okReport || skipped,
        `биом=${biomeShown} исход=${outcome}${skipped ? '(SKIP: подход перекрыт базой биома)' : ''} цель=${goalTxt} вход(${kind})=${outcome === 'pass'} выход(опора вне выреза)=${outcome === 'pass'} спуск=${minDescent.toFixed(1)}px кандидатов=${candidates.length} x ${Math.round(field.startX)}→${Math.round(maxX)} pageErrors=${pageErrors2.length}${pageErrors2[0] ? ' | ' + pageErrors2[0] : ''}${diag}`);
    }

    const failed = results.filter((r) => !r.ok).length;
    console.log('SUMMARY: ' + results.filter((r) => r.ok).length + ' PASS / ' + failed + ' FAIL');
    await finish(failed ? 1 : 0);
  } catch (e) {
    console.log('EXCEPTION: ' + e.message + '\n' + e.stack);
    await finish(1);
  }
}
main();
