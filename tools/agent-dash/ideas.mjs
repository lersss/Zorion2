// Учёт затрат по идеям — линии по файлам `docs/gamedesign/ideas/*.md`.
// Идея 100e. Хранилище сессий opencode читается ТОЛЬКО на чтение.
//
// Привязка — к файлу идеи, а не к названию окна сессии (одну идею обычно
// разрабатывают в нескольких вкладках). Тяжёлый разбор частей (кто какие файлы
// идей писал/правил) кэшируется отдельным файлом: ключ — time_updated сессии.

import { DatabaseSync } from "node:sqlite";
import fs from "node:fs";
import { dayKey } from "./stats.mjs";

const round = (v) => Math.round(v * 100) / 100;

// Файл идеи — это `…/docs/gamedesign/ideas/<имя>.md`.
const IDEA_RE = /[\\/]docs[\\/]gamedesign[\\/]ideas[\\/][^\\/]+\.md$/i;
// Дата-названные файлы (2026-09-25_…) не склеиваются — ключ = полное имя.
const DATE_NAME_RE = /^\d{4}-\d{2}-\d{2}[_-]/;
// Ведущее число + необязательная одна буква + точка: `96a.` → 96, `99a.3-…` → 99.
const FAMILY_RE = /^(\d+)[A-Za-zА-Яа-яЁё]?\./;

const baseName = (p) => String(p).split(/[\\/]/).pop() || "";
const fileStem = (p) => baseName(p).replace(/\.md$/i, "");

// Семья файла идеи: ключ = число для «номерных», полное имя — для прочих.
export function familyKey(filePath) {
  const name = fileStem(filePath);
  if (DATE_NAME_RE.test(name)) return name;
  const m = name.match(FAMILY_RE);
  return m ? m[1] : name;
}

export function fileName(filePath) {
  return fileStem(filePath);
}

function filesWord(n) {
  const m10 = n % 10;
  const m100 = n % 100;
  if (m10 === 1 && m100 !== 11) return "файл";
  if (m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14)) return "файла";
  return "файлов";
}

// Человекочитаемое название линии: семья из нескольких файлов — «№96 · 10 файлов»,
// одиночный файл — его имя без `.md`.
export function labelFor(key, names) {
  const list = [...names];
  if (/^\d+$/.test(key) && list.length > 1) return "№" + key + " · " + list.length + " " + filesWord(list.length);
  return list[0] || key;
}

function readCache(cachePath) {
  if (!cachePath) return {};
  try {
    const raw = JSON.parse(fs.readFileSync(cachePath, "utf8"));
    return raw && raw.sessions ? raw : {};
  } catch {
    return {};
  }
}

export function createIdeas({ dbPath, project = "Zorion", cachePath = null, refreshGapMs = 15000 } = {}) {
  const db = new DatabaseSync(dbPath, { readOnly: true });
  const projectLike = "%" + String(project).toLowerCase() + "%";
  const cached = readCache(cachePath);

  const sessionRows = db.prepare(
    "SELECT id,parent_id,agent,title,cost,time_created,time_updated FROM session" +
      " WHERE lower(directory) LIKE ?"
  );
  const dayAgg = db.prepare(
    `SELECT session_id id,
            strftime('%Y-%m-%d', time_created/1000, 'unixepoch', 'localtime') day,
            COUNT(*) turns, SUM(json_extract(data,'$.cost')) cost,
            SUM(json_extract(data,'$.tokens.input')) tok_in,
            SUM(json_extract(data,'$.tokens.output')) tok_out,
            SUM(json_extract(data,'$.tokens.cache.read')) tok_cache,
            MAX(time_created) last
     FROM message WHERE data LIKE '%"role":"assistant"%' GROUP BY 1, 2`
  );
  // ВНИМАНИЕ: привязка — ТОЛЬКО по вызовам, которые пишут файл (`tool` = write/edit
  // и `filePath` из входа). Упоминания файла идеи в тексте/чтении НЕ считаем — менеджер
  // читает много чужих идей, по ним атрибуция ломается (проверено: ~19.5 тыс. частей-шум).
  const partOfSession = db.prepare(
    `SELECT json_extract(data,'$.state.input.filePath') fp, json_extract(data,'$.tool') tool
     FROM part WHERE session_id=?
       AND json_extract(data,'$.tool') IN ('write','edit')
       AND json_extract(data,'$.state.input.filePath') IS NOT NULL`
  );
  const dayAggOfSession = db.prepare(
    `SELECT strftime('%Y-%m-%d', time_created/1000, 'unixepoch', 'localtime') day,
            COUNT(*) turns, SUM(json_extract(data,'$.cost')) cost,
            SUM(json_extract(data,'$.tokens.input')) tok_in,
            SUM(json_extract(data,'$.tokens.output')) tok_out,
            SUM(json_extract(data,'$.tokens.cache.read')) tok_cache,
            MAX(time_created) last
     FROM message WHERE session_id=? AND data LIKE '%"role":"assistant"%' GROUP BY 1`
  );
  // Первый проход — тем же фильтром по всем сессиям сразу (см. предупреждение выше).
  const allIdeaParts = db.prepare(
    `SELECT session_id, json_extract(data,'$.state.input.filePath') fp, json_extract(data,'$.tool') tool
     FROM part
     WHERE json_extract(data,'$.tool') IN ('write','edit')
       AND json_extract(data,'$.state.input.filePath') IS NOT NULL`
  );

  let sessions = [];
  let byId = new Map();
  let children = new Map();
  let loaded = false;
  let lastRefresh = 0;
  let cacheSessions = cached.sessions || {};
  let firstScan = Object.keys(cacheSessions).length === 0;
  let cacheDirty = false;

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

  function newSession(r) {
    return {
      id: r.id,
      parentID: r.parent_id || null,
      agent: r.agent || "—",
      title: r.title || "",
      cost: r.cost || 0,
      created: r.time_created || 0,
      updated: r.time_updated || 0,
      turns: 0,
      last: r.time_updated || 0,
      days: new Map(),
    };
  }

  // Разрез активности сессии по дням — как в stats.mjs, чтобы цена сходилась.
  function load() {
    sessions = sessionRows.all(projectLike).map(newSession);
    byId = new Map(sessions.map((s) => [s.id, s]));
    linkParents();

    for (const r of dayAgg.all()) {
      const s = byId.get(r.id);
      if (!s || !r.day) continue;
      s.days.set(r.day, {
        cost: r.cost || 0,
        turns: r.turns || 0,
        tokIn: r.tok_in || 0,
        tokOut: r.tok_out || 0,
        tokCache: r.tok_cache || 0,
        last: r.last || 0,
      });
      s.turns += r.turns || 0;
      if (r.last > s.last) s.last = r.last;
    }
    lastRefresh = Date.now();
    loaded = true;
    return { sessions: sessions.length };
  }

  // Точный разрез по дням одной сессии (для изменившихся сессий).
  function fillDays(s) {
    s.days = new Map();
    s.turns = 0;
    s.last = s.updated || 0;
    for (const r of dayAggOfSession.all(s.id)) {
      if (!r.day) continue;
      s.days.set(r.day, {
        cost: r.cost || 0,
        turns: r.turns || 0,
        tokIn: r.tok_in || 0,
        tokOut: r.tok_out || 0,
        tokCache: r.tok_cache || 0,
        last: r.last || 0,
      });
      s.turns += r.turns || 0;
      if (r.last > s.last) s.last = r.last;
    }
  }

  // Догрузка: новые и изменившиеся сессии попадают в отчёт без перезапуска.
  function refresh(gapMs = refreshGapMs) {
    const now = Date.now();
    if (now - lastRefresh < gapMs) return 0;
    lastRefresh = now;
    const rows = sessionRows.all(projectLike);
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
        prev.agent = r.agent || "—";
        prev.title = r.title || "";
        fillDays(prev);
        changed++;
      }
      next.push(prev);
    }
    for (const id of [...byId.keys()]) if (!seen.has(id)) byId.delete(id);
    sessions = next;
    byId = new Map(sessions.map((s) => [s.id, s]));
    linkParents();
    return changed;
  }

  const ensureLoaded = () => {
    if (!loaded) load();
  };

  // Записать в семью файл, который сессия писала (write) или правила (edit).
  function applyPart(fams, fp, tool) {
    if (!fp || !IDEA_RE.test(fp)) return;
    const key = familyKey(fp);
    let f = fams[key];
    if (!f) fams[key] = f = { files: [], created: false };
    const name = fileName(fp);
    if (!f.files.includes(name)) f.files.push(name);
    if (tool === "write") f.created = true;
  }

  // Разбор частей: первый проход — одним запросом, дальше — только изменившиеся
  // сессии (ключ кэша — time_updated сессии).
  function attribute() {
    if (firstScan) {
      const next = {};
      for (const r of allIdeaParts.all()) {
        if (!byId.has(r.session_id)) continue;
        const fams = next[r.session_id] || (next[r.session_id] = {});
        applyPart(fams, r.fp, r.tool);
      }
      for (const s of sessions) {
        if (!next[s.id]) next[s.id] = {};
        cacheSessions[s.id] = { updated: s.updated, fams: next[s.id] };
      }
      firstScan = false;
      cacheDirty = true;
      return;
    }
    for (const s of sessions) {
      const st = cacheSessions[s.id];
      if (st && st.updated === s.updated) continue;
      const fams = {};
      for (const r of partOfSession.all(s.id)) applyPart(fams, r.fp, r.tool);
      cacheSessions[s.id] = { updated: s.updated, fams };
      cacheDirty = true;
    }
    for (const id of Object.keys(cacheSessions)) {
      if (!byId.has(id)) {
        delete cacheSessions[id];
        cacheDirty = true;
      }
    }
  }

  // Активность сессии внутри периода; null — в этом периоде сессия не работала.
  function activityOf(s, sinceDay) {
    if (!sinceDay) {
      let cost = 0;
      let tokensIn = 0;
      let tokensOut = 0;
      let tokensCache = 0;
      for (const d of s.days.values()) {
        cost += d.cost;
        tokensIn += d.tokIn;
        tokensOut += d.tokOut;
        tokensCache += d.tokCache;
      }
      return {
        cost: s.days.size ? cost : s.cost,
        tokensIn,
        tokensOut,
        tokensCache,
        turns: s.turns,
        last: s.last,
      };
    }
    let cost = 0;
    let tokensIn = 0;
    let tokensOut = 0;
    let tokensCache = 0;
    let turns = 0;
    let last = 0;
    let any = false;
    for (const [day, d] of s.days) {
      if (day < sinceDay) continue;
      any = true;
      cost += d.cost;
      tokensIn += d.tokIn;
      tokensOut += d.tokOut;
      tokensCache += d.tokCache;
      turns += d.turns;
      if (d.last > last) last = d.last;
    }
    return any ? { cost, tokensIn, tokensOut, tokensCache, turns, last } : null;
  }

  function newLine(key) {
    return {
      key,
      cost: 0,
      sessions: 0,
      turns: 0,
      tokensIn: 0,
      tokensOut: 0,
      tokensCache: 0,
      last: 0,
      created: false,
      agents: new Set(),
      also: new Set(),
      byRole: new Map(),
      rows: [],
    };
  }

  function report({ since = 0 } = {}) {
    ensureLoaded();
    refresh();
    attribute();
    const sinceDay = since ? dayKey(since) : null;

    const act = new Map();
    for (const s of sessions) {
      const a = activityOf(s, sinceDay);
      if (a) act.set(s.id, a);
    }

    // Все файлы каждой семьи (для названия линии и списка в детали).
    const familyFiles = new Map();
    for (const s of sessions) {
      const fams = cacheSessions[s.id]?.fams || {};
      for (const [key, f] of Object.entries(fams)) {
        let set = familyFiles.get(key);
        if (!set) familyFiles.set(key, (set = new Set()));
        for (const n of f.files) set.add(n);
      }
    }

    const lines = new Map();
    const unattached = [];
    let attrCost = 0;
    let unattachedCost = 0;
    let totalSessions = 0;

    for (const root of sessions) {
      if (root.parentID) continue;
      const tree = [];
      const walk = (x) => {
        tree.push(x);
        for (const k of children.get(x.id) || []) walk(k);
      };
      walk(root);

      // Семьи, затронутые деревом (любой сессией).
      const fams = new Map(); // key -> {files:Set, created}
      for (const x of tree) {
        const c = cacheSessions[x.id]?.fams || {};
        for (const [key, f] of Object.entries(c)) {
          let g = fams.get(key);
          if (!g) fams.set(key, (g = { files: new Set(), created: false }));
          for (const n of f.files) g.files.add(n);
          if (f.created) g.created = true;
        }
      }

      const treeAct = [];
      let treeCost = 0;
      let treeTurns = 0;
      for (const x of tree) {
        const a = act.get(x.id);
        if (!a) continue;
        treeAct.push({ s: x, a });
        treeCost += a.cost;
        treeTurns += a.turns;
      }
      if (!treeAct.length || (!treeCost && !treeTurns)) continue;
      totalSessions += treeAct.length;

      // Дерево без затронутых файлов идей → «Непривязанные» (деньги не теряем).
      if (!fams.size) {
        unattachedCost += treeCost;
        unattached.push(makeUnattached(root, treeAct, treeCost));
        continue;
      }
      attrCost += treeCost;

      // Семья с максимумом затронутых файлов; при равенстве — где файл создан.
      let best = null;
      for (const [key, g] of fams) {
        const cand = { key, files: g.files.size, created: g.created };
        if (!best) best = cand;
        else if (cand.files > best.files) best = cand;
        else if (cand.files === best.files && cand.created && !best.created) best = cand;
        else if (cand.files === best.files && cand.created === best.created && cand.key < best.key) best = cand;
      }

      let line = lines.get(best.key);
      if (!line) lines.set(best.key, (line = newLine(best.key)));
      addTree(line, best.key, treeAct, fams);
    }

    const lineList = [...lines.values()]
      .map((l) => {
        const names = [...(familyFiles.get(l.key) || [])].sort();
        return {
          key: l.key,
          label: labelFor(l.key, names.length ? names : [l.key]),
          files: names,
          cost: round(l.cost),
          sessions: l.sessions,
          turns: l.turns,
          tokensIn: l.tokensIn,
          tokensOut: l.tokensOut,
          tokensCache: l.tokensCache,
          last: l.last,
          created: l.created,
          agents: [...l.agents].sort(),
          also: [...l.also].sort().map((k) => {
            const fn = [...(familyFiles.get(k) || [])].sort();
            return fn.length ? labelFor(k, fn) : k;
          }),
          byRole: [...l.byRole.entries()]
            .map(([agent, r]) => ({ agent, cost: round(r.cost), sessions: r.sessions }))
            .sort((a, b) => b.cost - a.cost),
          sessionRows: l.rows.sort((a, b) => b.cost - a.cost),
        };
      })
      .sort((a, b) => b.cost - a.cost);

    unattached.sort((a, b) => b.cost - a.cost);

    if (cacheDirty) saveCache();

    return {
      generatedAt: Date.now(),
      project,
      since,
      sinceDay,
      totals: {
        cost: round(attrCost + unattachedCost),
        attributed: round(attrCost),
        unattached: round(unattachedCost),
        lines: lineList.length,
        unattachedTrees: unattached.length,
        sessions: totalSessions,
        files: [...familyFiles.values()].reduce((a, s) => a + s.size, 0),
      },
      lines: lineList,
      unattached,
    };
  }

  function addTree(line, bestKey, treeAct, fams) {
    const best = fams.get(bestKey);
    if (best.created) line.created = true;
    for (const key of fams.keys()) if (key !== bestKey) line.also.add(key);

    for (const { s, a } of treeAct) {
      line.cost += a.cost;
      line.sessions += 1;
      line.turns += a.turns;
      line.tokensIn += a.tokensIn;
      line.tokensOut += a.tokensOut;
      line.tokensCache += a.tokensCache;
      if (a.last > line.last) line.last = a.last;
      line.agents.add(s.agent);
      let r = line.byRole.get(s.agent);
      if (!r) line.byRole.set(s.agent, (r = { cost: 0, sessions: 0 }));
      r.cost += a.cost;
      r.sessions += 1;
      const own = cacheSessions[s.id]?.fams?.[bestKey];
      line.rows.push({
        id: s.id,
        title: s.title,
        agent: s.agent,
        cost: round(a.cost),
        turns: a.turns,
        last: a.last,
        created: !!(own && own.created),
      });
    }
  }

  function makeUnattached(root, treeAct, treeCost) {
    const agents = new Set();
    let turns = 0;
    let last = 0;
    for (const { s, a } of treeAct) {
      agents.add(s.agent);
      turns += a.turns;
      if (a.last > last) last = a.last;
    }
    return {
      key: root.id,
      label: root.title || "(без названия)",
      agent: root.agent,
      cost: round(treeCost),
      sessions: treeAct.length,
      turns,
      last,
      agents: [...agents].sort(),
    };
  }

  function saveCache() {
    if (!cachePath || !loaded) return;
    try {
      const out = { version: 1, sessions: {} };
      for (const s of sessions) if (cacheSessions[s.id]) out.sessions[s.id] = cacheSessions[s.id];
      fs.writeFileSync(cachePath, JSON.stringify(out), "utf8");
      cacheDirty = false;
    } catch {
      // кэш — необязательное удобство
    }
  }

  return { load, report, saveCache, close: () => db.close() };
}
