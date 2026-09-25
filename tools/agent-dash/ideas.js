// Вкладка «Идеи» сервиса учёта агентов (идея 100e): цена идеи целиком по файлам
// docs/gamedesign/ideas/*.md. Клиентская логика вынесена из index.html отдельно,
// данные берёт из /api/ideas (лёгкий JSON: привязку считает сервер).

const $ = (id) => document.getElementById(id);
const esc = (s) => String(s ?? "").replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" })[c]);
const money = (v) => "$" + (Number(v) < 10 ? Number(v).toFixed(2) : Number(v).toFixed(1));
const tok = (v) => (v >= 1e6 ? (v / 1e6).toFixed(1) + "M" : v >= 1000 ? Math.round(v / 1000) + "k" : String(v || 0));
const when = (ms) => {
  if (!ms) return "—";
  const d = new Date(ms);
  const today = new Date().toDateString() === d.toDateString();
  const time = d.toTimeString().slice(0, 5);
  return today ? time : d.toLocaleDateString("ru-RU", { day: "2-digit", month: "2-digit" }) + " " + time;
};

const PERIODS = [
  ["today", "Сегодня"],
  ["7", "7 дней"],
  ["all", "Всё"],
];

const state = { tab: "overview", period: "all", report: null, open: new Set(), loading: false };

function lineRow(l) {
  const open = state.open.has(l.key);
  const main = `<tr data-ideas-line="${esc(l.key)}" style="cursor:pointer">
    <td class="name"><strong>${open ? "▾" : "▸"} ${esc(l.label)}</strong></td>
    <td class="num">${l.files.length}</td>
    <td class="num">${l.sessions}</td>
    <td>${(l.agents || []).map((a) => `<span class="role">${esc(a)}</span>`).join(" ")}</td>
    <td class="num"><strong>${money(l.cost)}</strong></td>
    <td class="num">${tok(l.tokensIn)} / ${tok(l.tokensOut)}</td>
    <td>${when(l.last)}</td>
  </tr>`;
  if (!open) return main;

  const files = l.files.length ? l.files.map((f) => `<span class="pill">${esc(f)}</span>`).join(" ") : "—";
  const also = l.also.length ? `<div class="delta" style="margin-top:6px">Также затронуто (цена целиком в этой линии): ${l.also.map(esc).join(", ")}</div>` : "";
  const roles = l.byRole.length
    ? `<div style="margin-top:8px"><span class="dim">По ролям:</span> ${l.byRole
        .map((r) => `<span class="pill">${esc(r.agent)} · ${money(r.cost)} · ${r.sessions} сес.</span>`)
        .join(" ")}</div>`
    : "";
  const sess = `<table style="margin-top:8px"><thead><tr><th>Роль</th><th>Сессия</th><th class="num">Цена</th><th class="num">Ответов</th><th>Создал файл</th><th>Активность</th></tr></thead><tbody>${l.sessionRows
    .map(
      (s) => `<tr>
        <td><span class="role">${esc(s.agent)}</span></td>
        <td class="name">${esc(s.title || s.id)}</td>
        <td class="num">${money(s.cost)}</td>
        <td class="num">${s.turns}</td>
        <td>${s.created ? '<span class="pill ok">write</span>' : ""}</td>
        <td>${when(s.last)}</td>
      </tr>`
    )
    .join("")}</tbody></table>`;
  return main + `<tr><td colspan="7" style="background:#191c22">${files}${also}${roles}${sess}</td></tr>`;
}

function unattachedBlock(r) {
  if (!r.unattached.length) return "";
  const rows = r.unattached
    .map(
      (u) => `<tr>
        <td class="name">${esc(u.label)}</td>
        <td>${(u.agents || []).map((a) => `<span class="role">${esc(a)}</span>`).join(" ")}</td>
        <td class="num">${u.sessions}</td>
        <td class="num">${u.turns}</td>
        <td class="num"><strong>${money(u.cost)}</strong></td>
        <td>${when(u.last)}</td>
      </tr>`
    )
    .join("");
  return `<h2>Непривязанные деревья (без файла идеи) — ${money(r.totals.unattached)}</h2>
    <div class="scroll"><table><thead><tr><th>Сессия (корень дерева)</th><th>Роли</th><th class="num">Сессий</th><th class="num">Ответов</th><th class="num">Цена</th><th>Активность</th></tr></thead><tbody>${rows}</tbody></table></div>`;
}

function render() {
  const box = $("ideas");
  if (!box) return;
  const r = state.report;
  const periodBtns = PERIODS.map(
    ([p, l]) => `<button class="mini ${state.period === p ? "on" : ""}" data-ideas-period="${p}">${l}</button>`
  ).join(" ");
  let html = `<div class="livehead">
      <h2>Идеи — цена по файлам идей</h2>
      <span class="dim">${r ? "сессий: " + r.totals.sessions + " · файлов идей: " + r.totals.files : ""}</span>
      <span style="flex:1"></span>
      <span>${periodBtns}</span>
    </div>`;

  if (!r) {
    html += `<div class="idle">${state.loading ? "Читаю хранилище и связываю сессии с идеями…" : "Нет данных."}</div>`;
    box.innerHTML = html;
    return;
  }

  html += `<div class="cards">
      <div class="card"><span class="dim">Цена идей</span><b>${money(r.totals.attributed)}</b></div>
      <div class="card"><span class="dim">Непривязано</span><b>${money(r.totals.unattached)}</b></div>
      <div class="card"><span class="dim">Всего за период</span><b>${money(r.totals.cost)}</b></div>
      <div class="card"><span class="dim">Линий идей</span><b>${r.totals.lines}</b></div>
    </div>`;

  const lines = r.lines.length
    ? r.lines.map(lineRow).join("")
    : `<tr><td colspan="7" class="idle">За период идей не затронуто.</td></tr>`;
  html += `<table><thead><tr>
      <th>Идея</th><th class="num">Файлов</th><th class="num">Сессий</th><th>Роли</th>
      <th class="num">Цена</th><th class="num">Токены (вх / вых)</th><th>Активность</th>
    </tr></thead><tbody>${lines}</tbody></table>`;

  html += unattachedBlock(r);
  html += `<p class="note">Линия — файлы идей с одним ведущим номером («96a…96j» → №96);
    дата-названные файлы не склеиваются. Дерево сессий относится к семье, где затронуто
    больше файлов; при равенстве — где файл создан (write). Каждая сессия считается один
    раз, прочие затронутые семьи — пометкой, без деления цены.</p>`;
  box.innerHTML = html;
}

async function load() {
  state.loading = true;
  render();
  try {
    const res = await fetch(`/api/ideas?period=${state.period}`);
    const data = await res.json();
    if (!data || data.error) throw new Error(data?.error || "пустой ответ");
    state.report = data;
  } catch (e) {
    state.report = null;
    $("ideas").innerHTML = `<div class="idle">Не удалось загрузить идеи: ${esc(e.message)}</div>`;
    state.loading = false;
    return;
  }
  state.loading = false;
  render();
}

function showTab(tab) {
  const overview = $("overview");
  const ideas = $("ideas");
  if (!overview || !ideas) return;
  state.tab = tab;
  overview.style.display = tab === "overview" ? "" : "none";
  ideas.style.display = tab === "ideas" ? "" : "none";
  $("tab-overview")?.classList.toggle("on", tab === "overview");
  $("tab-ideas")?.classList.toggle("on", tab === "ideas");
  if (tab === "ideas" && !state.report && !state.loading) load();
}

$("tab-overview")?.addEventListener("click", () => showTab("overview"));
$("tab-ideas")?.addEventListener("click", () => showTab("ideas"));

document.addEventListener("click", (e) => {
  const period = e.target.closest("button[data-ideas-period]");
  if (period) {
    state.period = period.dataset.ideasPeriod;
    state.open = new Set();
    load();
    return;
  }
  const row = e.target.closest("tr[data-ideas-line]");
  if (!row) return;
  const key = row.dataset.ideasLine;
  if (state.open.has(key)) state.open.delete(key);
  else state.open.add(key);
  render();
});

showTab("overview");
