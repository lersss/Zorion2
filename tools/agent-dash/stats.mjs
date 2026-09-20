// Данные для живого учёта сессий агентов (см. идею 100c).
// Читает хранилище сессий opencode ТОЛЬКО на чтение и отдаёт агрегаты
// по ролям (субагентам) и по фичам (дерево сессий: вкладка менеджера + подсессии).

import { DatabaseSync } from "node:sqlite";
import path from "node:path";
import fs from "node:fs";

export const GUARD_MARK = "Сторож зацикливания";

const AS_ASSISTANT = "data LIKE '%\"role\":\"assistant\"%'";
const AS_TOOL = "data LIKE '%\"type\":\"tool\"%'";
const CTX = "(json_extract(data,'$.tokens.input') + json_extract(data,'$.tokens.cache.read'))";

export function defaultDbPath() {
  if (process.env.OPENCODE_DB) return process.env.OPENCODE_DB;
  const home = process.env.USERPROFILE || process.env.HOME || "";
  return path.join(home, ".local", "share", "opencode", "opencode.db");
}

// Локальная дата «ГГГГ-ММ-ДД» — ей сравниваем периоды (сегодня / 7 дней).
export function dayKey(ms) {
  const d = new Date(ms);
  const p = (n) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

const EMPTY_STAT = { calls: 0, worst: 0, repeats: 0, compactions: 0, overflow: 0 };

const round = (v) => Math.round(v * 100) / 100;

// Срабатывания сторожа берём из его журнала: так надёжнее, чем искать текст
// отказа в частях сессии (текст встречается и в самих файлах проекта).
function loadJournal(journalPath) {
  const byId = new Map();
  let lines = 0;
  let bad = 0;
  if (journalPath) {
    try {
      for (const line of fs.readFileSync(journalPath, "utf8").split("\n")) {
        if (!line.trim()) continue;
        let e;
        try {
          e = JSON.parse(line);
        } catch {
          // Строка могла склеиться с предыдущей (журнал оборвался без перевода
          // строки) — пробуем вытащить последний объект из строки.
          try {
            e = JSON.parse(line.slice(line.lastIndexOf("{")));
          } catch {
            bad++;
            continue;
          }
        }
        if (!e.session || !e.at) {
          bad++;
          continue;
        }
        lines++;
        let days = byId.get(e.session);
        if (!days) byId.set(e.session, (days = new Map()));
        const day = dayKey(e.at);
        const cur = days.get(day) || { blocked: 0, aborted: 0, remind: 0 };
        if (e.action === "aborted") cur.aborted++;
        else if (e.action === "blocked") cur.blocked++;
        else if (e.action === "remind") cur.remind++;
        days.set(day, cur);
      }
    } catch {
      // журнала может ещё не быть — это нормально
    }
  }
  return { byId, lines, bad };
}

function newGroup(key, label, agent) {
  return {
    key,
    label,
    agent,
    sessions: 0,
    cost: 0,
    tokensIn: 0,
    tokensOut: 0,
    turns: 0,
    peak: 0,
    calls: 0,
    repeats: 0,
    guard: 0,
    compactions: 0,
    last: 0,
    since: 0,
    agents: new Set(),
  };
}

function addTo(g, session, activity, stat, guard) {
  g.sessions += 1;
  g.cost += activity.cost;
  g.tokensIn += session.tokensIn;
  g.tokensOut += session.tokensOut;
  g.turns += activity.turns;
  g.peak = Math.max(g.peak, activity.peak);
  g.calls += stat.calls;
  g.repeats += stat.repeats;
  g.guard += guard;
  g.compactions += stat.compactions;
  g.last = Math.max(g.last, activity.last);
  if (!g.since || session.created < g.since) g.since = session.created;
  g.agents.add(session.agent);
}

function shrinkGroup(g) {
  return {
    key: g.key,
    label: g.label,
    agent: g.agent,
    sessions: g.sessions,
    cost: round(g.cost),
    tokensIn: g.tokensIn,
    tokensOut: g.tokensOut,
    turns: g.turns,
    peak: g.peak,
    calls: g.calls,
    repeats: g.repeats,
    guard: g.guard,
    compactions: g.compactions,
    last: g.last,
    since: g.since,
    agents: [...g.agents].sort(),
  };
}

export function createStore({
  dbPath,
  project = "Zorion",
  cachePath = null,
  journalPath = null,
  refreshGapMs = 8000,
  recentWindowMs = Number(process.env.DASH_RECENT_MIN || 15) * 60 * 1000,
} = {}) {
  const db = new DatabaseSync(dbPath, { readOnly: true });
  const cache = readCache(cachePath);

  let sessions = [];
  let byId = new Map();
  let children = new Map();
  let journal = { byId: new Map(), lines: 0, bad: 0 };
  let lastRefresh = 0;
  const stats = new Map();
  const scanned = new Set();

  const sessionRows = () =>
    db
      .prepare(
        "SELECT id,parent_id,agent,title,cost,tokens_input,tokens_output,tokens_cache_read," +
          "time_created,time_updated FROM session WHERE lower(directory) LIKE ?"
      )
      .all("%" + String(project).toLowerCase() + "%");

  const dayAgg = db.prepare(
    `SELECT strftime('%Y-%m-%d', time_created/1000, 'unixepoch', 'localtime') day,
            COUNT(*) turns, SUM(json_extract(data,'$.cost')) cost,
            MAX(${CTX}) peak, MAX(time_created) last
     FROM message WHERE session_id=? AND ${AS_ASSISTANT} GROUP BY 1`
  );

  const toolGroups = db.prepare(
    `SELECT COUNT(*) c FROM part WHERE session_id=? AND ${AS_TOOL}
     GROUP BY json_extract(data,'$.tool'), json_extract(data,'$.state.input')`
  );
  // То же, но только по текущей задаче — «в рамках того, что агент делает сейчас».
  const recentGroups = db.prepare(
    `SELECT COUNT(*) c FROM part WHERE session_id=? AND ${AS_TOOL} AND time_created >= ?
     GROUP BY json_extract(data,'$.tool'), json_extract(data,'$.state.input')`
  );
  const lastUserMessage = db.prepare(
    `SELECT MAX(time_created) m FROM message WHERE session_id=? AND data LIKE '%"role":"user"%'`
  );
  const miscOf = db.prepare(
    `SELECT SUM(CASE WHEN ${AS_TOOL} THEN 1 ELSE 0 END) calls,
            SUM(CASE WHEN data LIKE '%"type":"compaction"%' THEN 1 ELSE 0 END) compactions,
            SUM(CASE WHEN data LIKE '%"overflow":true%' THEN 1 ELSE 0 END) overflow
     FROM part WHERE session_id=?`
  );

  function newSession(r) {
    return {
      id: r.id,
      parentID: r.parent_id || null,
      agent: r.agent || "—",
      title: r.title || "",
      cost: r.cost || 0,
      tokensIn: r.tokens_input || 0,
      tokensOut: r.tokens_output || 0,
      created: r.time_created || 0,
      updated: r.time_updated || 0,
      turns: 0,
      peak: 0,
      last: r.time_updated || 0,
      days: new Map(),
    };
  }

  // Активность сессии по дням — по отметкам сообщений.
  function fillDays(s) {
    s.days = new Map();
    s.turns = 0;
    s.peak = 0;
    s.last = s.updated || 0;
    for (const r of dayAgg.all(s.id)) {
      if (!r.day) continue;
      s.days.set(r.day, {
        cost: r.cost || 0,
        turns: r.turns || 0,
        peak: r.peak || 0,
        last: r.last || 0,
      });
      s.turns += r.turns || 0;
      s.peak = Math.max(s.peak, r.peak || 0);
      if (r.last > s.last) s.last = r.last;
    }
  }

  function linkParents() {
    const ids = new Set(sessions.map((s) => s.id));
    for (const s of sessions) s.parentID = ids.has(s.parentID) ? s.parentID : null;
    children = new Map();
    for (const s of sessions) {
      if (!s.parentID) continue;
      if (!children.has(s.parentID)) children.set(s.parentID, []);
      children.get(s.parentID).push(s);
    }
  }

  // Полная загрузка: сессии проекта и разрез их активности по дням.
  function load() {
    const rows = sessionRows();
    sessions = rows.map(newSession);
    byId = new Map(sessions.map((s) => [s.id, s]));
    linkParents();

    const days = db
      .prepare(
        `SELECT session_id id,
                strftime('%Y-%m-%d', time_created/1000, 'unixepoch', 'localtime') day,
                COUNT(*) turns, SUM(json_extract(data,'$.cost')) cost,
                MAX(${CTX}) peak, MAX(time_created) last
         FROM message WHERE ${AS_ASSISTANT} GROUP BY 1, 2`
      )
      .all();
    for (const r of days) {
      const s = byId.get(r.id);
      if (!s || !r.day) continue;
      s.days.set(r.day, {
        cost: r.cost || 0,
        turns: r.turns || 0,
        peak: r.peak || 0,
        last: r.last || 0,
      });
      s.turns += r.turns || 0;
      s.peak = Math.max(s.peak, r.peak || 0);
      if (r.last > s.last) s.last = r.last;
    }

    // Кэш прошлого запуска принимаем только для не изменившихся сессий.
    for (const s of sessions) {
      const st = cache[s.id];
      if (st && st.updated === s.updated) {
        stats.set(s.id, st);
        scanned.add(s.id);
      }
    }

    journal = loadJournal(journalPath);
    lastRefresh = Date.now();
    return { sessions: sessions.length, cached: stats.size, journalLines: journal.lines };
  }

  // Догрузка изменений: сессии, которые работают прямо сейчас, появляются на
  // странице без перезапуска сервиса (новые сессии, новая цена, память, время).
  function refresh(gapMs = refreshGapMs) {
    const now = Date.now();
    if (now - lastRefresh < gapMs) return 0;
    lastRefresh = now;
    const rows = sessionRows();
    const seen = new Set();
    let changed = 0;
    const next = [];
    for (const r of rows) {
      seen.add(r.id);
      const prev = byId.get(r.id);
      if (!prev) {
        const s = newSession(r);
        fillDays(s);
        next.push(s);
        changed++;
        continue;
      }
      if (prev.updated !== r.time_updated) {
        prev.updated = r.time_updated;
        prev.cost = r.cost || 0;
        prev.tokensIn = r.tokens_input || 0;
        prev.tokensOut = r.tokens_output || 0;
        prev.agent = r.agent || "—";
        prev.title = r.title || "";
        fillDays(prev);
        scanned.delete(prev.id); // тяжёлые счётчики пересчитает фоновый проход
        changed++;
      }
      next.push(prev);
    }
    for (const id of [...byId.keys()]) {
      if (seen.has(id)) continue;
      byId.delete(id);
      stats.delete(id);
      scanned.delete(id);
    }
    sessions = next;
    byId = new Map(sessions.map((s) => [s.id, s]));
    linkParents();
    return changed;
  }

  const statOf = (id) => stats.get(id) || EMPTY_STAT;

  // Тяжёлые счётчики (вызовы, повторы, сторож, сжатия памяти) досчитываются
  // порциями: сначала самые дорогие сессии, чтобы цифры появлялись сверху вниз.
  function scanBatch(limit = 20) {
    const todo = sessions
      .filter((s) => !scanned.has(s.id))
      .sort((a, b) => b.cost - a.cost)
      .slice(0, limit);
    for (const s of todo) {
      let worst = 0;
      let repeats = 0;
      for (const g of toolGroups.all(s.id)) {
        const c = g.c || 0;
        if (c > worst) worst = c;
        if (c > 3) repeats += c - 3;
      }
      const m = miscOf.get(s.id) || {};
      stats.set(s.id, {
        updated: s.updated,
        calls: m.calls || 0,
        worst,
        repeats,
        compactions: m.compactions || 0,
        overflow: m.overflow || 0,
      });
      scanned.add(s.id);
    }
    return { done: scanned.size, total: sessions.length, scanned: todo.length };
  }

  // Срабатывания сторожа по сессии за период (из его журнала).
  function guardOf(id, sinceDay) {
    const days = journal.byId.get(id);
    if (!days) return { blocked: 0, aborted: 0, remind: 0 };
    let blocked = 0;
    let aborted = 0;
    let remind = 0;
    for (const [day, c] of days) {
      if (sinceDay && day < sinceDay) continue;
      blocked += c.blocked;
      aborted += c.aborted;
      remind += c.remind;
    }
    return { blocked, aborted, remind };
  }

  // Срабатывания сторожа берём из его журнала: так надёжнее, чем искать текст
  // отказа в частях сессии (текст встречается и в самих файлах проекта).
  // Журнал перечитывается на каждый отчёт — он дописывается сторожем вживую.
  let journalStamp = "";
  function refreshJournal() {
    if (!journalPath) return;
    let size;
    let mtime;
    try {
      const st = fs.statSync(journalPath);
      size = st.size;
      mtime = st.mtimeMs;
    } catch {
      return; // журнала ещё нет — появится, поймаем в следующий раз
    }
    const stamp = size + ":" + mtime;
    if (stamp === journalStamp) return;
    journalStamp = stamp;
    journal = loadJournal(journalPath);
  }

  // Текущая задача = с последнего запроса пользователя в этой сессии; если
  // запроса нет — берём последние минуты (recentWindowMs).
  function taskWindow(id) {
    const last = lastUserMessage.get(id)?.m;
    if (last) return { from: last, label: "в задаче" };
    const minutes = Math.round(recentWindowMs / 60000);
    return { from: Date.now() - recentWindowMs, label: `за ${minutes} мин` };
  }

  // Повторы в рамках текущей задачи: видно, крутится ли агент прямо сейчас.
  function recentOf(id) {
    const win = taskWindow(id);
    let calls = 0;
    let worst = 0;
    let repeats = 0;
    for (const g of recentGroups.all(id, win.from)) {
      const c = g.c || 0;
      calls += c;
      if (c > worst) worst = c;
      if (c > 3) repeats += c - 3;
    }
    return { calls, worst, repeats, label: win.label };
  }

  // Активность сессии внутри периода; null — в этом периоде сессия не работала.
  function activityOf(s, sinceDay) {
    if (!sinceDay) {
      let cost = 0;
      for (const d of s.days.values()) cost += d.cost;
      return { cost: s.days.size ? cost : s.cost, turns: s.turns, peak: s.peak, last: s.last };
    }
    let cost = 0;
    let turns = 0;
    let peak = 0;
    let last = 0;
    let any = false;
    for (const [day, d] of s.days) {
      if (day < sinceDay) continue;
      any = true;
      cost += d.cost;
      turns += d.turns;
      if (d.peak > peak) peak = d.peak;
      if (d.last > last) last = d.last;
    }
    return any ? { cost, turns, peak, last } : null;
  }

  function report({ since = 0 } = {}) {
    refresh();
    refreshJournal();
    const sinceDay = since ? dayKey(since) : null;
    const rows = [];
    for (const s of sessions) {
      const activity = activityOf(s, sinceDay);
      if (!activity) continue;
      rows.push({ s, activity, stat: statOf(s.id), guard: guardOf(s.id, sinceDay) });
    }

    const agentMap = new Map();
    for (const { s, activity, stat, guard } of rows) {
      let g = agentMap.get(s.agent);
      if (!g) agentMap.set(s.agent, (g = newGroup(s.agent, s.agent, s.agent)));
      addTo(g, s, activity, stat, guard.blocked + guard.aborted);
    }

    const featureList = [];
    for (const root of sessions) {
      if (root.parentID) continue;
      const tree = [];
      const walk = (x) => {
        tree.push(x);
        for (const k of children.get(x.id) || []) walk(k);
      };
      walk(root);
      const g = newGroup(root.id, root.title, root.agent);
      for (const x of tree) {
        const activity = activityOf(x, sinceDay);
        if (!activity) continue;
        const guard = guardOf(x.id, sinceDay);
        addTo(g, x, activity, statOf(x.id), guard.blocked + guard.aborted);
      }
      if (g.sessions && (g.turns || g.cost)) featureList.push(shrinkGroup(g));
    }

    const now = Date.now();
    const agentList = [...agentMap.values()]
      .map(shrinkGroup)
      .sort((x, y) => y.cost - x.cost);
    featureList.sort((x, y) => y.cost - x.cost);

    return {
      generatedAt: now,
      project,
      since,
      sinceDay,
      scanning: {
        done: scanned.size,
        total: sessions.length,
        finished: scanned.size >= sessions.length,
      },
      totals: {
        sessions: rows.length,
        cost: round(rows.reduce((a, r) => a + r.activity.cost, 0)),
        calls: rows.reduce((a, r) => a + r.stat.calls, 0),
        repeats: rows.reduce((a, r) => a + r.stat.repeats, 0),
        guard: rows.reduce((a, r) => a + r.guard.blocked + r.guard.aborted, 0),
        guardBlocked: rows.reduce((a, r) => a + r.guard.blocked, 0),
        guardAborted: rows.reduce((a, r) => a + r.guard.aborted, 0),
        guardReminds: rows.reduce((a, r) => a + r.guard.remind, 0),
        features: featureList.length,
      },
      journal: { lines: journal.lines, broken: journal.bad },
      agents: agentList,
      features: featureList,
      active: rows
        .slice()
        .sort((x, y) => y.activity.last - x.activity.last)
        .slice(0, 40)
        .map(({ s, activity, stat, guard }) => {
          const recent = recentOf(s.id);
          return {
            id: s.id,
            agent: s.agent,
            title: s.title,
            parent: s.parentID ? byId.get(s.parentID)?.title || "" : "",
            cost: round(activity.cost),
            peak: activity.peak,
            turns: activity.turns,
            last: activity.last,
            live: now - activity.last < 5 * 60 * 1000,
            guard: guard.blocked + guard.aborted,
            repeats: stat.repeats,
            recentCalls: recent.calls,
            recentRepeats: recent.repeats,
            recentWorst: recent.worst,
            recentLabel: recent.label,
          };
        }),
    };
  }

  function saveCache() {
    if (!cachePath) return;
    try {
      const out = {};
      for (const [id, st] of stats) out[id] = st;
      fs.writeFileSync(cachePath, JSON.stringify(out), "utf8");
    } catch {
      // кэш — необязательное удобство
    }
  }

  return { load, refresh, scanBatch, saveCache, report, close: () => db.close() };
}

function readCache(cachePath) {
  if (!cachePath) return {};
  try {
    return JSON.parse(fs.readFileSync(cachePath, "utf8"));
  } catch {
    return {};
  }
}
