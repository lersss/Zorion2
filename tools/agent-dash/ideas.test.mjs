// Проверка привязки затрат к идеям на синтетическом хранилище: node ideas.test.mjs

import { DatabaseSync } from "node:sqlite";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { createIdeas, familyKey } from "./ideas.mjs";

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

const todayNoon = new Date(new Date().setHours(12, 0, 0, 0)).getTime();
const yesterdayNoon = todayNoon - 24 * 3600 * 1000;
const todayMidnight = new Date(new Date().setHours(0, 0, 0, 0)).getTime();

const dir = fs.mkdtempSync(path.join(os.tmpdir(), "agent-dash-ideas-"));
const dbPath = path.join(dir, "fake.db");
const db = new DatabaseSync(dbPath);
db.exec(`
  CREATE TABLE session (id text PRIMARY KEY, parent_id text, agent text, title text,
    cost real DEFAULT 0, time_created integer, time_updated integer, directory text);
  CREATE TABLE message (id text PRIMARY KEY, session_id text, time_created integer, time_updated integer, data text);
  CREATE TABLE part (id text PRIMARY KEY, message_id text, session_id text, time_created integer, time_updated integer, data text);
`);

const session = (id, parent, agent, title, created, updated) =>
  db
    .prepare("INSERT INTO session VALUES (?,?,?,?,0,?,?,?)")
    .run(id, parent, agent, title, created, updated, "C:\\Zorion2");
const message = (id, sessionID, at, cost) =>
  db
    .prepare("INSERT INTO message VALUES (?,?,?,?,?)")
    .run(
      id,
      sessionID,
      at,
      at,
      JSON.stringify({ role: "assistant", cost, tokens: { input: 1, output: 1, cache: { read: 0 } } })
    );
let partNo = 0;
const filePart = (sessionID, tool, filePath) =>
  db
    .prepare("INSERT INTO part VALUES (?,?,?,?,?,?)")
    .run(
      "p" + ++partNo,
      "m" + partNo,
      sessionID,
      todayNoon,
      todayNoon,
      JSON.stringify({ type: "tool", tool, callID: "c" + partNo, state: { status: "completed", input: { filePath } } })
    );

const IDEAS = "C:\\Zorion2\\docs\\gamedesign\\ideas\\";

// Дерево «Линия 96»: менеджер + разработчик.
session("s96", null, "manager", "Линия 96", yesterdayNoon, todayNoon);
session("s96d", "s96", "developer", "Подсессия 96", todayNoon, todayNoon);
message("m96", "s96", todayNoon, 2);
message("m96d", "s96d", todayNoon, 1);

// Дерево «Линия 100»: вчера и сегодня.
session("s100", null, "manager", "Линия 100", yesterdayNoon, todayNoon);
message("m100a", "s100", yesterdayNoon, 3);
message("m100b", "s100", todayNoon, 0.5);

// Дерево «Разговор без идеи» — непривязанное.
session("sun", null, "manager", "Разговор без идеи", todayNoon, todayNoon);
message("mun", "sun", todayNoon, 1.5);

// Дерево «Смесь»: по одному файлу в семьях 96 и 100, но в 100 файл создан (write).
session("smix", null, "manager", "Смесь", todayNoon, todayNoon);
session("smixa", "smix", "tester", "Смесь-тест", todayNoon, todayNoon);
message("mmix", "smix", todayNoon, 1);
message("mmixa", "smixa", todayNoon, 0.5);

// Дерево «Даты»: два дата-названных файла — склеивать нельзя.
session("sdate", null, "manager", "Даты", todayNoon, todayNoon);
message("mdate", "sdate", todayNoon, 0.25);

// Файлы идей по деревьям.
filePart("s96", "write", IDEAS + "96a. Генератор событий.md"); // 96, создан
filePart("s96", "edit", IDEAS + "96c. Новый слой — технологии.md"); // 96
filePart("s100", "write", IDEAS + "100a. Роль UI-дизайнера.md"); // 100, создан
filePart("s100", "edit", IDEAS + "100c. Живой учёт агентов.md"); // 100
filePart("smix", "edit", IDEAS + "96b. Система первого контакта.md"); // 96 (не создан)
filePart("smix", "write", IDEAS + "100b. Сторож зацикливания.md"); // 100, создан
filePart("sdate", "write", IDEAS + "2026-09-25_идея-один.md");
filePart("sdate", "edit", IDEAS + "2026-09-25_идея-два.md");
// Упоминание файла идей в тексте/контенте не должно привязывать (шум).
db.prepare("INSERT INTO part VALUES (?,?,?,?,?,?)").run(
  "ptxt",
  "mptxt",
  "sun",
  todayNoon,
  todayNoon,
  JSON.stringify({ type: "text", text: "смотри " + IDEAS + "100e. Учёт затрат.md" })
);
db.close();

check("склейка семей: 96a и 96c дают ключ 96", familyKey(IDEAS + "96a. X.md") === "96" && familyKey(IDEAS + "96c. Y.md") === "96");
check("склейка семей: 99a.3-… даёт ключ 99", familyKey(IDEAS + "99a.3-студия.md") === "99");
check("дата-названные не склеиваются", familyKey(IDEAS + "2026-09-25_идея-один.md") === "2026-09-25_идея-один");
check("файл без номера — ключ целиком", familyKey(IDEAS + "qa-checklist.md") === "qa-checklist");
check("один и тот же файл из edit и write даёт одну семью", familyKey(IDEAS + "14d. Эталон.md") === familyKey(IDEAS + "14c. Тестировщик.md"));

const cachePath = path.join(dir, ".ideas-cache.json");
const store = createIdeas({ dbPath, project: "Zorion", cachePath, refreshGapMs: 0 });
const all = store.report({});

const line = (k) => all.lines.find((l) => l.key === k);
check("линия 96 есть", !!line("96"), JSON.stringify(all.lines.map((l) => l.key)));
check("линия 100 есть", !!line("100"));
check("дата-названный файл — своя линия", !!line("2026-09-25_идея-один"), JSON.stringify(all.lines.map((l) => l.key)));
check("дата-названные не схлопнулись в «2026»", !all.lines.some((l) => l.key === "2026"), JSON.stringify(all.lines.map((l) => l.key)));

check("склейка семей: у линии 96 несколько файлов", line("96").files.length === 3, JSON.stringify(line("96").files));
check("название линии — номер и число файлов", /^№96 · 3 файла$/.test(line("96").label), line("96").label);
check("цена линии 96 = дерево целиком", line("96").cost === 3, JSON.stringify(line("96").cost));
check("в линии 96 видны роли", line("96").agents.join(",") === "developer,manager", line("96").agents.join(","));
check("цена линии 100 = свои сессии + смесь", line("100").cost === 5, JSON.stringify(line("100").cost));
check("цена дата-линии", line("2026-09-25_идея-один").cost === 0.25, JSON.stringify(line("2026-09-25_идея-один").cost));

// Приоритет созданного файла: у «Смеси» по одному файлу в 96 и 100, создан в 100.
const mix = line("100").sessionRows.find((s) => s.id === "smix");
check("при равенстве файлов дерево уходит в семью, где файл создан (write)", !!mix, JSON.stringify(line("100").sessionRows.map((s) => s.id)));
check("цена смешанного дерева не поделена, а целиком в линии 100", line("100").sessionRows.filter((s) => s.id === "smix" || s.id === "smixa").length === 2, JSON.stringify(line("100").sessionRows.map((s) => s.id)));
check("число сессий линии — это счётчик, а разрез в sessionRows", typeof line("100").sessions === "number" && Array.isArray(line("100").sessionRows), JSON.stringify({ sessions: line("100").sessions, row: typeof line("100").sessionRows }));
check("в линии 100 указана ещё затронутая семья пометкой", line("100").also.some((x) => x.startsWith("№96")), JSON.stringify(line("100").also));
check("разрез по ролям внутри линии 100", line("100").byRole.map((r) => r.agent).sort().join(",") === "manager,tester", JSON.stringify(line("100").byRole));

check("непривязанное дерево видно", all.unattached.some((u) => u.label === "Разговор без идеи"), JSON.stringify(all.unattached.map((u) => u.label)));
const un = all.unattached.find((u) => u.label === "Разговор без идеи");
check("в непривязанном цена сохранена", un.cost === 1.5, JSON.stringify(un?.cost));
check("текст с упоминанием файла идеи не привязывает сессию", !all.lines.some((l) => l.sessionRows.some((s) => s.id === "sun")), JSON.stringify(all.lines.map((l) => l.sessionRows.map((s) => s.id))));

check("сумма линий и непривязанных = общая цена", Math.abs(all.totals.attributed + all.totals.unattached - all.totals.cost) < 1e-9, JSON.stringify(all.totals));
check("общая цена сходится с суммой сообщений", all.totals.cost === 9.75, JSON.stringify(all.totals));

// Нарезка цены по периоду: вчерашние траты не попадают в «сегодня».
const today = store.report({ since: todayMidnight });
check("нарезка по периоду: линия 100 сегодня без вчерашних денег", today.lines.find((l) => l.key === "100").cost === 2, JSON.stringify(today.lines.find((l) => l.key === "100")?.cost));
check("нарезка по периоду: линия 96 не изменилась", today.lines.find((l) => l.key === "96").cost === 3);
check("нарезка по периоду: общая цена только за сегодня", today.totals.cost === 6.75, JSON.stringify(today.totals.cost));
check("нарезка по периоду: непривязанные тоже режутся", today.totals.unattached === 1.5, JSON.stringify(today.totals.unattached));

// Кэш: повторный проход берёт привязку из файла, цифры те же.
check("кэш привязки записан", fs.existsSync(cachePath));
const store2 = createIdeas({ dbPath, project: "Zorion", cachePath, refreshGapMs: 0 });
const again = store2.report({});
check("из кэша линии те же", again.lines.map((l) => l.key).sort().join(",") === all.lines.map((l) => l.key).sort().join(","), JSON.stringify(again.lines.map((l) => l.key)));
check("из кэша цена та же", again.totals.cost === 9.75, JSON.stringify(again.totals));

// Догрузка изменившейся сессии: новая правка идеи подхватывается без полного прохода.
const w = new DatabaseSync(dbPath);
w.prepare("INSERT INTO part VALUES (?,?,?,?,?,?)").run(
  "pnew",
  "mnew",
  "sdate",
  todayNoon,
  todayNoon,
  JSON.stringify({ type: "tool", tool: "write", callID: "cnew", state: { input: { filePath: IDEAS + "77a. Новая идея.md" } } })
);
w.prepare("UPDATE session SET time_updated=? WHERE id='sdate'").run(todayNoon + 1000);
w.close();
const after = store2.report({});
check("изменившаяся сессия перечитана и получила новую семью", after.lines.some((l) => l.key === "2026-09-25_идея-один" && l.also.some((x) => x.startsWith("77a"))), JSON.stringify(after.lines.find((l) => l.key === "2026-09-25_идея-один")?.also));

store.close();
store2.close();
fs.rmSync(dir, { recursive: true, force: true });

console.log("\nитог: PASS=" + ok + " FAIL=" + fail);
process.exit(fail ? 1 : 0);
