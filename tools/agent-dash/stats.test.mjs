// Проверка разбора данных на синтетическом хранилище: node stats.test.mjs

import { DatabaseSync } from "node:sqlite";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { createStore, dayKey, GUARD_MARK } from "./stats.mjs";

let ok = 0;
let fail = 0;
const check = (name, cond, extra = "") => {
  if (cond) {
    ok++;
    console.log("PASS  " + name);
  } else {
    fail++;
    console.log("FAIL  " + name + "  " + extra);
  }
};

const now = Date.now();
const todayNoon = new Date(new Date().setHours(12, 0, 0, 0)).getTime();
const yesterdayNoon = todayNoon - 24 * 3600 * 1000;
const todayMidnight = new Date(new Date().setHours(0, 0, 0, 0)).getTime();

// Курс go/zen в рублях — совпадает со значением по умолчанию DASH_USD_RUB.
// В тестах тестовые сообщения — это go/zen: их цена = cost × R.
const R = 129;

const dir = fs.mkdtempSync(path.join(os.tmpdir(), "agent-dash-"));
const dbPath = path.join(dir, "fake.db");
const db = new DatabaseSync(dbPath);
db.exec(`
  CREATE TABLE session (id text PRIMARY KEY, parent_id text, agent text, title text,
    cost real DEFAULT 0, tokens_input integer DEFAULT 0, tokens_output integer DEFAULT 0,
    tokens_cache_read integer DEFAULT 0, time_created integer, time_updated integer, directory text);
  CREATE TABLE message (id text PRIMARY KEY, session_id text, time_created integer, time_updated integer, data text);
  CREATE TABLE part (id text PRIMARY KEY, message_id text, session_id text, time_created integer, time_updated integer, data text);
`);

const session = (id, parent, agent, title, created, updated, tin, tout, tcache) =>
  db
    .prepare(
      "INSERT INTO session VALUES (?,?,?,?,0,?,?,?,?,?,?)"
    )
    .run(id, parent, agent, title, tin, tout, tcache, created, updated, "C:\\Zorion2");
const message = (id, sessionID, at, cost, input, cacheRead) =>
  db
    .prepare("INSERT INTO message VALUES (?,?,?,?,?)")
    .run(
      id,
      sessionID,
      at,
      at,
      JSON.stringify({
        role: "assistant",
        cost,
        providerID: "opencode-go",
        modelID: "deepseek-v4.1-flash",
        tokens: { input, output: 1, reasoning: 0, cache: { read: cacheRead, write: 0 } },
      })
    );
let partNo = 0;
const part = (sessionID, data) =>
  db
    .prepare("INSERT INTO part VALUES (?,?,?,?,?,?)")
    .run("p" + ++partNo, "m" + partNo, sessionID, todayNoon, todayNoon, JSON.stringify(data));
const toolPart = (sessionID, command) =>
  part(sessionID, { type: "tool", tool: "bash", callID: "c" + partNo, state: { input: { command }, title: command } });

session("s1", null, "manager", "Фича А", yesterdayNoon, todayNoon, 100, 10, 5000);
session("s2", "s1", "developer", "Подсессия А-разраб", todayNoon, todayNoon, 200, 20, 70000);
session("s3", "s1", "tester", "Подсессия А-тест", todayNoon, todayNoon, 300, 30, 9000);
session("s4", null, "manager", "Фича Б", yesterdayNoon, yesterdayNoon, 400, 40, 2000);
session("s5", "нет-такого", "manager", "Осиротевшая", yesterdayNoon, yesterdayNoon, 1, 1, 1);

message("m1", "s1", yesterdayNoon, 1.0, 10, 1000);
message("m2", "s1", todayNoon, 2.0, 20, 2000);
message("m3", "s2", Date.now(), 0.5, 5, 60000);
message("m4", "s3", todayNoon, 0.25, 5, 100);
message("m5", "s4", yesterdayNoon, 3.0, 30, 3000);

for (let i = 0; i < 6; i++) toolPart("s2", "git log -1");
toolPart("s2", "grep x");
part("s2", { type: "tool", tool: "bash", state: { input: { command: "y" }, output: GUARD_MARK + ": хватит" } });
part("s2", { type: "compaction", auto: true, overflow: true });
toolPart("s1", "go build");
toolPart("s1", "go test");
toolPart("s4", "ls");
db.close();

// Журнал сторожа: два отказа сегодня и один обрыв вчера.
const journalPath = path.join(dir, "loop-guard.log");
fs.writeFileSync(
  journalPath,
  [
    JSON.stringify({ at: todayNoon, session: "s2", tool: "bash", times: 5, action: "blocked" }),
    JSON.stringify({ at: todayNoon + 1000, session: "s2", tool: "bash", times: 6, action: "blocked" }),
    JSON.stringify({ at: yesterdayNoon, session: "s2", tool: "read", times: 41, action: "aborted" }),
    JSON.stringify({ at: todayNoon + 2000, session: "s3", tool: "bash", action: "remind", reason: "память 750k" }),
    "мусорная строка",
  ].join("\n"),
  "utf8"
);

const cachePath = path.join(dir, "cache.json");
const store = createStore({ dbPath, cachePath, journalPath, refreshGapMs: 0 });
const loaded = store.load();
check("загружены все сессии проекта", loaded.sessions === 5, JSON.stringify(loaded));

const scan = store.scanBatch(50);
check("тяжёлые счётчики досчитаны", scan.done === 5 && scan.scanned === 5, JSON.stringify(scan));

const all = store.report({});
const agent = (name) => all.agents.find((a) => a.label === name);
check("цена всего сходится", all.totals.cost === 6.75 * R, JSON.stringify(all.totals.cost));
check("цена менеджера", agent("manager").cost === 6 * R, JSON.stringify(agent("manager")?.cost));
check("цена разработчика", agent("developer").cost === 0.5 * R);
check("цена тестера", agent("tester").cost === 0.25 * R);
check("сессий по ролям: 3 у менеджера", agent("manager").sessions === 3, JSON.stringify(agent("manager")?.sessions));
check("пик памяти по роли — максимум сессии", agent("developer").peak === 60005, JSON.stringify(agent("developer")?.peak));
check("повторы считаются сверх трёх", agent("developer").repeats === 3, JSON.stringify(agent("developer")?.repeats));
check("вызовы инструментов без служебных частей", agent("developer").calls === 8, JSON.stringify(agent("developer")?.calls));
check("срабатывания сторожа — из журнала", agent("developer").guard === 3, JSON.stringify(agent("developer")?.guard));
check("сжатие памяти посчитано", agent("developer").compactions === 1);
check("журнал прочитан, битые строки отмечены", all.journal.lines === 4 && all.journal.broken === 1, JSON.stringify(all.journal));
check("напоминания про бюджет не считаются вмешательством", agent("tester").guard === 0, JSON.stringify(agent("tester")?.guard));
check("напоминания про бюджет видны отдельным счётчиком", all.totals.guardReminds === 1, JSON.stringify(all.totals));

check(
  "токены за всё время: вход/выход/кеш считаются раздельно",
  all.totals.tokensIn === 70 && all.totals.tokensOut === 5 && all.totals.tokensCache === 66100,
  JSON.stringify(all.totals)
);
check(
  "токены суммируются по роли",
  agent("manager").tokensIn === 60 && agent("manager").tokensOut === 3 && agent("manager").tokensCache === 6000,
  JSON.stringify(agent("manager"))
);
check(
  "кеш роли не смешивается со входом",
  agent("developer").tokensCache === 60000 && agent("developer").tokensIn === 5,
  JSON.stringify(agent("developer"))
);

check("фич в отчёте две", all.features.length === 2, JSON.stringify(all.features.map((f) => f.label)));
const featureA = all.features.find((f) => f.label === "Фича А");
const featureB = all.features.find((f) => f.label === "Фича Б");
check("цена фичи = дерево сессий", featureA.cost === 3.75 * R, JSON.stringify(featureA?.cost));
check("в фиче видны все роли", featureA.agents.join(",") === "developer,manager,tester", featureA.agents.join(","));
check(
  "токены суммируются по дереву фичи",
  featureA.tokensIn === 40 && featureA.tokensOut === 4 && featureA.tokensCache === 63100,
  JSON.stringify(featureA)
);
check("цена второй фичи", featureB.cost === 3 * R, JSON.stringify(featureB?.cost));
check("сессия без работы не засоряет список фич", !all.features.some((f) => f.label === "Осиротевшая"), JSON.stringify(all.features.map((f) => f.label)));

check("в отчёте видны все проекты", all.project === "все проекты", all.project);
check("в записи активной сессии есть проект", all.active.find((a) => a.id === "s1")?.project === "Zorion2", JSON.stringify(all.active.map((a) => [a.id, a.project])));
check("роль в отчёте несёт проект", agent("manager").project === "Zorion2", JSON.stringify(agent("manager")?.project));
check("фича в отчёте несёт проект", featureA.project === "Zorion2", JSON.stringify(featureA?.project));

const today = store.report({ since: todayMidnight });
const liveRow = today.active.find((a) => a.id === "s2");
check("активная сессия попадает в верхний блок", !!liveRow && liveRow.live === true, JSON.stringify(today.active.map((a) => [a.id, a.live])));
check("в записи активной сессии есть цена, память и роль", !!liveRow && liveRow.cost > 0 && liveRow.peak > 0 && liveRow.agent === "developer", JSON.stringify(liveRow));
check("в записи активной сессии есть заголовок фичи", liveRow?.parent === "Фича А", JSON.stringify(liveRow?.parent));

// Активное время сессии (поле workMs): пауза больше 10 минут в счёт не идёт.
const workRow = all.active.find((a) => a.id === "s1");
check("поле workMs есть в записи активной сессии", all.active.every((a) => typeof a.workMs === "number"), JSON.stringify(all.active.map((a) => [a.id, a.workMs])));
check("пауза больше 10 минут обрезается до 10 минут", workRow?.workMs === 600000, JSON.stringify(workRow?.workMs));
check("у сессии с одним сообщением активное время ноль", liveRow?.workMs === 0, JSON.stringify(liveRow?.workMs));
check("период «сегодня» режет по дням", Math.abs(today.totals.cost - 2.75 * R) < 1e-9, JSON.stringify(today.totals.cost));
check("в периоде «сегодня» три сессии", today.totals.sessions === 3, JSON.stringify(today.totals.sessions));
check("вчерашняя фича не попала в «сегодня»", !today.features.some((f) => f.label === "Фича Б"), JSON.stringify(today.features.map((f) => f.label)));
check("в «сегодня» только активные роли", today.agents.length === 3, JSON.stringify(today.agents.map((a) => a.label)));
const devToday = today.agents.find((a) => a.label === "developer");
check("срабатывания сторожа режутся по периоду", devToday.guard === 2, JSON.stringify(devToday?.guard));
check("вчерашний обрыв сессии не попал в «сегодня»", today.totals.guardAborted === 0, JSON.stringify(today.totals));
check("обрыв сессии виден во «всё время»", all.totals.guardAborted === 1 && all.totals.guardBlocked === 2, JSON.stringify(all.totals));
check(
  "токены режутся по периоду: сообщение старого дня не попало в «сегодня»",
  today.totals.tokensIn === 30 && all.totals.tokensIn === 70,
  JSON.stringify(today.totals)
);
check(
  "в «сегодня» вход/выход/кеш раздельны",
  today.totals.tokensOut === 3 && today.totals.tokensCache === 62100,
  JSON.stringify(today.totals)
);
check(
  "токены роли тоже режутся по периоду",
  devToday.tokensIn === 5 && devToday.tokensOut === 1 && devToday.tokensCache === 60000,
  JSON.stringify(devToday)
);

store.saveCache();

const store2 = createStore({ dbPath, cachePath, journalPath, refreshGapMs: 0 });
const loaded2 = store2.load();
check("кэш ускоряет второй запуск", loaded2.cached === 5, JSON.stringify(loaded2));
const all2 = store2.report({});
check("из кэша цифры те же", all2.totals.cost === 6.75 * R && all2.totals.repeats === 3, JSON.stringify(all2.totals));

check("день считается по локальной дате", dayKey(todayNoon) === dayKey(todayMidnight) && dayKey(todayNoon) !== dayKey(yesterdayNoon), dayKey(todayNoon) + " / " + dayKey(yesterdayNoon));

// Сторож дописывает журнал вживую — учёт должен подхватывать без перезапуска.
fs.appendFileSync(
  journalPath,
  JSON.stringify({ at: todayNoon + 5000, session: "s1", tool: "bash", action: "blocked" }) + "\n",
  "utf8"
);
const live = store2.report({});
check("новая строка журнала подхватывается на лету", live.totals.guardBlocked === 3, JSON.stringify(live.totals));

// Сессия, которая работает прямо сейчас, должна появляться без перезапуска.
const writer = new DatabaseSync(dbPath);
const nowMs = Date.now();
writer.prepare("INSERT INTO session VALUES (?,?,?,?,?,?,?,?,?,?,?)").run("s6", "s1", "developer", "Работающая сейчас", 0, 10, 1, 1, nowMs, nowMs, "C:\\Zorion2");
writer.prepare("INSERT INTO message VALUES (?,?,?,?,?)").run(
  "m9",
  "s6",
  nowMs,
  nowMs,
  JSON.stringify({ role: "assistant", cost: 4, providerID: "opencode-go", modelID: "deepseek-v4.1-flash", tokens: { input: 10, output: 1, cache: { read: 640000 } } })
);
writer.prepare("UPDATE session SET time_updated=?, cost=5 WHERE id='s4'").run(nowMs);
writer.prepare("INSERT INTO message VALUES (?,?,?,?,?)").run(
  "m10",
  "s4",
  nowMs,
  nowMs,
  JSON.stringify({ role: "assistant", cost: 2, providerID: "opencode-go", modelID: "deepseek-v4.1-flash", tokens: { input: 10, output: 1, cache: { read: 100000 } } })
);
// Пять одинаковых вызовов только что — это повторы «в рамках текущей задачи».
for (let i = 0; i < 5; i++) {
  writer.prepare("INSERT INTO part VALUES (?,?,?,?,?,?)").run(
    "rp" + i,
    "rm" + i,
    "s6",
    nowMs,
    nowMs,
    JSON.stringify({ type: "tool", tool: "bash", callID: "rc" + i, state: { input: { command: "git log -1" }, title: "git log -1" } })
  );
}
writer.close();

const after = store2.report({});
const fresh = (after.active || []).find((a) => a.id === "s6");
check("новая сессия появляется без перезапуска", !!fresh && fresh.live === true, JSON.stringify(after.active.map((a) => a.id)));
check("у новой сессии видна цена и память", !!fresh && fresh.cost === 4 * R && fresh.peak === 640010, JSON.stringify(fresh));
check("повторы считаются в рамках текущей задачи", fresh.recentRepeats === 2 && fresh.recentCalls === 5, JSON.stringify(fresh));
check("в записи указано, за что считали", fresh.recentLabel === "за 15 мин", JSON.stringify(fresh.recentLabel));

// Новый запрос пользователя = новая задача: счётчик повторов по задаче начинается заново.
const writer2 = new DatabaseSync(dbPath);
writer2.prepare("INSERT INTO message VALUES (?,?,?,?,?)").run(
  "m11",
  "s6",
  nowMs + 1000,
  nowMs + 1000,
  JSON.stringify({ role: "user", time: { created: nowMs + 1000 } })
);
writer2.prepare("UPDATE session SET time_updated=? WHERE id='s6'").run(nowMs + 1000);
writer2.close();
const afterTask = store2.report({});
const freshTask = afterTask.active.find((a) => a.id === "s6");
check(
  "с новым запросом задача новая — счётчик повторов обнуляется",
  freshTask.recentRepeats === 0 && freshTask.recentLabel === "в задаче",
  JSON.stringify(freshTask)
);
const s4 = after.features.find((f) => f.label === "Фича Б");
check("дописанная цена подхватывается", s4.cost === 5 * R, JSON.stringify(s4?.cost));
check("дописанная память подхватывается", s4.peak === 100010, JSON.stringify(s4?.peak));

// Часовой расход: отдельное хранилище — фильтра по проекту больше нет, поэтому
// свежие траты чужих сессий в общий отчёт не подмешиваются.
const rateDbPath = path.join(dir, "rate.db");
const rateDb = new DatabaseSync(rateDbPath);
rateDb.exec(`
  CREATE TABLE session (id text PRIMARY KEY, parent_id text, agent text, title text,
    cost real DEFAULT 0, tokens_input integer DEFAULT 0, tokens_output integer DEFAULT 0,
    tokens_cache_read integer DEFAULT 0, time_created integer, time_updated integer, directory text);
  CREATE TABLE message (id text PRIMARY KEY, session_id text, time_created integer, time_updated integer, data text);
  CREATE TABLE part (id text PRIMARY KEY, message_id text, session_id text, time_created integer, time_updated integer, data text);
`);
const insideHour = Date.now() - 20 * 60 * 1000;
const outsideHour = Date.now() - 90 * 60 * 1000;
const rateMessage = (id, at, cost) =>
  rateDb
    .prepare("INSERT INTO message VALUES (?,?,?,?,?)")
    .run(id, "r1", at, at, JSON.stringify({ role: "assistant", cost, providerID: "opencode-go", modelID: "deepseek-v4.1-flash", tokens: { input: 1, output: 1, cache: { read: 0 } } }));
rateDb
  .prepare("INSERT INTO session VALUES (?,?,?,?,0,?,?,?,?,?,?)")
  .run("r1", null, "developer", "Часовой расход", 0, 0, 0, outsideHour, insideHour, "C:\\Users\\admin\\Desktop\\backend");
rateMessage("rm1", insideHour, 3);
rateMessage("rm2", outsideHour, 10);
// Вторая сессия той же роли в другом проекте: ключ группировки — роль + проект.
rateDb
  .prepare("INSERT INTO session VALUES (?,?,?,?,0,?,?,?,?,?,?)")
  .run("r2", null, "developer", "Та же роль в Zorion", 0, 0, 0, outsideHour, insideHour, "C:\\Zorion2");
rateDb.close();

const rateStore = createStore({ dbPath: rateDbPath, refreshGapMs: 0 });
const rateReport = rateStore.report({});
check("часовой расход — только свежие траты", Math.abs(rateReport.rate.perHour - 3 * R) < 1e-9, JSON.stringify(rateReport.rate));
check("окно часового расхода — час", rateReport.rate.windowMin === 60, JSON.stringify(rateReport.rate));
check("часовой расход есть в обычном отчёте", typeof all.rate.perHour === "number", JSON.stringify(all.rate));
check(
  "сессия из чужой папки попадает в отчёт и несёт имя проекта",
  rateReport.active.find((a) => a.id === "r1")?.project === "backend",
  JSON.stringify(rateReport.active.map((a) => [a.id, a.project]))
);
check(
  "роли разных проектов не смешиваются в группе",
  rateReport.agents.filter((g) => g.label === "developer").map((g) => g.project).sort().join(",") === "Zorion2,backend",
  JSON.stringify(rateReport.agents.map((g) => [g.label, g.project]))
);
rateStore.close();

// Валютный учёт — гибрид: go/zen — из цены в БД × курс R; proxyapi — из токенов
// по рублёвой таблице (у старых сессий opencode пишет 0); gonka — без цены, остаётся 0.
const curDb = new DatabaseSync(dbPath);
const curAt = Date.now();
const mkS = (id, title) =>
  curDb
    .prepare("INSERT INTO session VALUES (?,?,?,?,0,?,?,?,?,?,?)")
    .run(id, null, "manager", title, 0, 1, 1, curAt, curAt, "C:\\Zorion2");
const mkM = (id, sid, data) =>
  curDb.prepare("INSERT INTO message VALUES (?,?,?,?,?)").run(id, sid, curAt, curAt, JSON.stringify(data));
mkS("sz", "Zen-история");
mkS("sp", "Proxy-история");
mkS("sn", "Gonka-история");
mkS("sr", "Proxy-с-рассуждением");
mkM("mz1", "sz", { role: "assistant", cost: 0.5, providerID: "opencode-go", modelID: "deepseek-v4.1-flash", tokens: { input: 1, output: 1, cache: { read: 0 } } });
mkM("mp1", "sp", { role: "assistant", cost: 0, providerID: "proxyapi", modelID: "qwen/qwen3.8-flash", tokens: { input: 1000, output: 0, cache: { read: 16000 } } });
// reasoning тарифицируется как выход: (1000*20 + (2000+3000)*65)/1M = 0.345 → 0.35
mkM("mr1", "sr", { role: "assistant", cost: 0, providerID: "proxyapi", modelID: "qwen/qwen3.8-flash", tokens: { input: 1000, output: 2000, reasoning: 3000, cache: { read: 0 } } });
mkM("mn1", "sn", { role: "assistant", cost: 0, providerID: "gonka", modelID: "deepseek-ai/DeepSeek-V4-Flash-0731", tokens: { input: 5000, output: 500, cache: { read: 0 } } });
curDb.close();
const curStore = createStore({ dbPath, refreshGapMs: 0 });
const curReport = curStore.report({});
const curS = (id) => curReport.active.find((a) => a.id === id);
check("go/zen: цена из БД × курс R", curS("sz")?.cost === 0.5 * R, JSON.stringify(curS("sz")?.cost));
// proxyapi: (1000*20 + 16000*2.5)/1M = 0.06 (округляется до копеек)
check("proxyapi: цена считается из токенов по рублёвой таблице", Math.abs(curS("sp")?.cost - 0.06) < 1e-9, JSON.stringify(curS("sp")?.cost));
check("proxyapi: токены рассуждения считаются как выходные", Math.abs(curS("sr")?.cost - 0.35) < 1e-9, JSON.stringify(curS("sr")?.cost));
check("gonka: без заданной цены остаётся нулём", curS("sn")?.cost === 0, JSON.stringify(curS("sn")?.cost));
curStore.close();

store.close();
store2.close();
fs.rmSync(dir, { recursive: true, force: true });

console.log("\nитог: PASS=" + ok + " FAIL=" + fail + " (" + Math.round((Date.now() - now) / 1000) + " c)");
process.exit(fail ? 1 : 0);
