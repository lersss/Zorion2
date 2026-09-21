// Живой учёт сессий агентов: отдаёт страницу и JSON с агрегатами.
// Запуск: node server.mjs   (порт и путь к хранилищу — через переменные окружения)

import http from "node:http";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { createStore, defaultDbPath } from "./stats.mjs";

const HERE = path.dirname(fileURLToPath(import.meta.url));
const PORT = Number(process.env.PORT || 8790);
const HOST = process.env.HOST || "127.0.0.1";
const PROJECT = process.env.PROJECT || "Zorion";
const DB_PATH = defaultDbPath();
const CACHE_PATH = process.env.DASH_CACHE || path.join(HERE, ".cache.json");
const JOURNAL_PATH = process.env.GUARD_LOG || path.join(HERE, "..", "..", ".opencode", "loop-guard.log");
const OVERRIDE_PATH = process.env.GUARD_OVERRIDE || path.join(HERE, "..", "..", ".opencode", "loop-guard.override.json");
const BATCH = Number(process.env.DASH_BATCH || 10);

const store = createStore({
  dbPath: DB_PATH,
  project: PROJECT,
  cachePath: CACHE_PATH,
  journalPath: JOURNAL_PATH,
});
const started = Date.now();
const { sessions, cached } = store.load();

console.log(`хранилище: ${DB_PATH}`);
console.log(`журнал сторожа: ${JOURNAL_PATH}`);
console.log(`проект: ${PROJECT} · сессий: ${sessions} · из кэша: ${cached}`);

// Тяжёлые счётчики досчитываются в фоне порциями, чтобы сервис отвечал сразу.
// Проход не выключается: сессии, которые работают прямо сейчас, попадают в
// пересчёт по мере изменений.
let scanning = true;
let idle = false;
function scanStep() {
  if (!scanning) return;
  let batch;
  try {
    batch = store.scanBatch(BATCH);
  } catch (e) {
    console.error("сбой разбора сессий:", e.message);
    scanning = false;
    return;
  }
  if (batch.scanned === 0) {
    if (!idle) {
      idle = true;
      store.saveCache();
      console.log(`разбор завершён за ${Math.round((Date.now() - started) / 1000)} c`);
    }
    setTimeout(scanStep, 3000);
    return;
  }
  idle = false;
  setTimeout(scanStep, 5);
}
setTimeout(scanStep, 50);

const periodSince = (period) => {
  const midnight = new Date(new Date().setHours(0, 0, 0, 0)).getTime();
  if (period === "today") return midnight;
  if (period === "7") return midnight - 6 * 24 * 3600 * 1000;
  return 0;
};

function send(res, code, type, body) {
  res.writeHead(code, { "content-type": type, "cache-control": "no-store" });
  res.end(body);
}

// HTML отдаём с явным запретом кэша и без ETag/Last-Modified: правишь интерфейс —
// браузер обязан показать новую версию, а не старую из кэша.
function sendHtml(res, body) {
  res.writeHead(200, {
    "content-type": "text/html; charset=utf-8",
    "cache-control": "no-store, no-cache, must-revalidate, max-age=0",
    pragma: "no-cache",
    expires: "0",
  });
  res.end(body);
}

// Продление лимитов сторожа на лету (идея 100d): сторож читает этот файл на каждом
// действии. Продление — только на срок, «навсегда» не пишем: заявка истекает сама.
const EXTEND_MAX_MINUTES = 240;
const EXTEND_MAX_ACTIONS = 200;

function readOverrides() {
  try {
    return JSON.parse(fs.readFileSync(OVERRIDE_PATH, "utf8")) || {};
  } catch {
    return {};
  }
}

function writeOverrides(list) {
  fs.mkdirSync(path.dirname(OVERRIDE_PATH), { recursive: true });
  const tmp = OVERRIDE_PATH + ".tmp";
  fs.writeFileSync(tmp, JSON.stringify(list, null, 2));
  fs.renameSync(tmp, OVERRIDE_PATH);
}

function activeExtensions() {
  const now = Date.now();
  return Object.fromEntries(Object.entries(readOverrides()).filter(([, e]) => Number(e?.until) > now));
}

function readBody(req, limit = 4096) {
  return new Promise((resolve) => {
    let body = "";
    let tooBig = false;
    req.on("data", (chunk) => {
      if (tooBig) return;
      body += chunk;
      if (body.length > limit) {
        tooBig = true;
        body = "";
      }
    });
    req.on("end", () => resolve(tooBig ? null : body));
  });
}

const server = http.createServer(async (req, res) => {
  const url = new URL(req.url, `http://${req.headers.host}`);
  if (url.pathname === "/api/health") return send(res, 200, "application/json", '{"ok":true}');
  if (url.pathname === "/api/stats") {
    const period = url.searchParams.get("period") || "today";
    const since = periodSince(period);
    let report;
    try {
      report = store.report({ since });
    } catch (e) {
      return send(res, 500, "application/json", JSON.stringify({ error: e.message }));
    }
    report.period = period;
    report.server = { port: PORT, uptimeSec: Math.round((Date.now() - started) / 1000), scanning };
    report.limits = {
      memWarn: Number(process.env.GUARD_MEM_WARN || 700000),
      memStop: Number(process.env.GUARD_MEM_STOP || 900000),
    };
    report.liveWindowMs = 5 * 60 * 1000;
    report.extensions = activeExtensions();
    return send(res, 200, "application/json", JSON.stringify(report));
  }
  if (url.pathname === "/api/extend" && req.method === "POST") {
    const raw = await readBody(req);
    if (raw === null) return send(res, 413, "application/json", JSON.stringify({ error: "тело запроса слишком большое" }));
    let payload;
    try {
      payload = JSON.parse(raw || "{}");
    } catch {
      return send(res, 400, "application/json", JSON.stringify({ error: "тело запроса — не JSON" }));
    }
    const session = String(payload.session ?? "").trim();
    if (!session) return send(res, 400, "application/json", JSON.stringify({ error: "не указана сессия" }));
    const minutes = Math.min(EXTEND_MAX_MINUTES, Math.max(1, Number(payload.minutes) || 30));
    const extra = Math.min(EXTEND_MAX_ACTIONS, Math.max(0, Number(payload.actions) || 15));
    const until = Date.now() + minutes * 60000;
    const list = readOverrides();
    const prev = list[session] || {};

    // «снять индульгенцию» — просто убираем заявку вкладки целиком.
    if (payload.revoke) {
      delete list[session];
      try {
        writeOverrides(list);
      } catch (e) {
        return send(res, 500, "application/json", JSON.stringify({ error: e.message }));
      }
      console.log(`индульгенция снята: ${session}`);
      return send(res, 200, "application/json", JSON.stringify({ ok: true, session, revoked: true }));
    }

    // pardon=true — индульгенция: сторож не трогает эту вкладку (повторы, память,
    // счётчики файлов/команд, гашение) до истечения срока. Обычная заявка продления
    // сохраняет уже выданную индульгенцию, и наоборот.
    const pardon = payload.pardon === true || prev.pardon === true;
    list[session] = {
      until,
      extra: pardon ? extra : Number(prev.extra) || extra,
      pardon,
      by: "создатель",
      reason: String(payload.reason ?? "").slice(0, 200),
      at: Date.now(),
    };
    try {
      writeOverrides(list);
    } catch (e) {
      return send(res, 500, "application/json", JSON.stringify({ error: e.message }));
    }
    console.log(
      pardon
        ? `индульгенция: ${session} — сторож не трогает до ${new Date(until).toLocaleTimeString()}`
        : `продление: ${session} — +${extra} действий до ${new Date(until).toLocaleTimeString()}`
    );
    return send(res, 200, "application/json", JSON.stringify({ ok: true, session, until, extra, minutes, pardon }));
  }
  // Страницу всегда отдаём свежей: при правке интерфейса браузер не должен
  // показывать закэшированную старую версию (иначе «новой кнопки нет»).
  if (url.pathname === "/") return sendHtml(res, fs.readFileSync(path.join(HERE, "index.html")));
  if (url.pathname === "/index.html") {
    return sendHtml(res, fs.readFileSync(path.join(HERE, "index.html")));
  }
  if (url.pathname === "/favicon.ico") return send(res, 204, "image/x-icon", "");
  send(res, 404, "text/plain", "нет такой страницы");
});

server.on("error", (e) => {
  if (e.code === "EADDRINUSE") {
    console.error(`порт ${PORT} занят — задай другой: $env:PORT=...`);
  } else {
    console.error("ошибка сервера:", e.message);
  }
  process.exit(1);
});

server.listen(PORT, HOST, () => {
  console.log(`открывай: http://${HOST}:${PORT}  (период по умолчанию: сегодня)`);
});

const shutdown = () => {
  scanning = false;
  store.saveCache();
  store.close();
  server.close(() => process.exit(0));
  setTimeout(() => process.exit(0), 500).unref();
};
process.on("SIGINT", shutdown);
process.on("SIGTERM", shutdown);
