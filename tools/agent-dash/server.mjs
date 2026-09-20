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

const server = http.createServer((req, res) => {
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
    return send(res, 200, "application/json", JSON.stringify(report));
  }
  if (url.pathname === "/") return send(res, 200, "text/html; charset=utf-8", fs.readFileSync(path.join(HERE, "index.html")));
  if (url.pathname === "/index.html") {
    return send(res, 200, "text/html; charset=utf-8", fs.readFileSync(path.join(HERE, "index.html")));
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
