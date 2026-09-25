"use strict";
// ---------- экспорт снимка контента (спека 2026-09-24 §7, И2) ----------
// GET /studio/api/content/export: сервер пишет content/catalog.json и отдаёт
// тот же JSON на скачивание. Браузерное скачивание тела не отдаёт JS, поэтому
// тянем ответ studioFetch'ем, читаем counts и имя файла из Content-Disposition,
// скачиваем Blob'ом и показываем отчёт.
async function exportContent() {
  const btn = $("btnExportContent");
  if (btn) btn.disabled = true;
  const r = await api("GET", "/studio/api/content/export");
  if (!r) { if (btn) btn.disabled = false; return; } // guard: api вернул null (401/ошибка)
  const text = await r.text();
  const cd = r.headers.get("Content-Disposition") || "";
  const m = /filename="?([^";]+)"?/.exec(cd);
  const fname = m ? m[1] : "catalog.json";
  let total = 0;
  try { const j = JSON.parse(text); total = Object.values(j.counts || {}).reduce((s, n) => s + n, 0); } catch (e) {}
  const blob = new Blob([text], {type: "application/json;charset=utf-8"});
  const a = document.createElement("a");
  a.href = URL.createObjectURL(blob);
  a.download = fname;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(a.href);
  showReport(["Экспорт: сохранено " + total + " записей, файл " + fname], true);
  refreshImportStatus(); // файл появился → кнопка импорта оживает (баг D1)
  if (btn) btn.disabled = false;
}
// ---------- импорт снимка контента (спека 2026-09-24 §5/§7, И3) ----------
// Два шага: (1) «Проверить» — POST ?dry_run=true, попап-дифф; (2) «Применить» —
// второе подтверждение, POST без dry_run, попап-отчёт. blocked/unmatched
// непусты → «Применить» неактивна (удаление живого невозможно в один клик).
let importDiff = null;
// sumCounts — сумма чисел секционного map (create/update/created/…).
function sumCounts(m) {
  return Object.values(m || {}).reduce((s, n) => s + (typeof n === "number" ? n : 0), 0);
}
// refreshImportStatus — наличие файла снимка в репозитории: нет файла → кнопка
// импорта неактивна с подсказкой (§7).
async function refreshImportStatus() {
  const btn = $("btnImportContent");
  if (!btn) return;
  const r = await api("GET", "/studio/api/content/status");
  if (!r) { btn.disabled = true; btn.title = "статус снимка контента недоступен"; return; }
  let s = null;
  try { s = await r.json(); } catch (e) {}
  const hasFile = !!(s && s.file);
  btn.disabled = !hasFile;
  btn.title = hasFile
    ? "Импорт контента (полная замена): сначала проверка (сухой прогон)"
    : "нет файла снимка (content/catalog.json) — сначала экспорт контента в dev";
}
// importContent — шаг 1: сухой прогон и попап-дифф.
async function importContent() {
  const btn = $("btnImportContent");
  if (btn) btn.disabled = true;
  showReport(["Проверка снимка контента (сухой прогон)…"], true);
  const r = await api("POST", "/studio/api/content/import?dry_run=true", {});
  if (!r) { refreshImportStatus(); return; } // guard: api → null (401/ошибка)
  let diff = null;
  try { diff = await r.json(); } catch (e) {}
  if (!diff) { refreshImportStatus(); return; }
  openImportPopup(diff);
  refreshImportStatus();
}
// openImportPopup — рендер диффа: режим, числа, перечень удаляемых, blocked.
function openImportPopup(diff) {
  importDiff = diff;
  const create = sumCounts(diff.create);
  const update = sumCounts(diff.update);
  const del = diff.delete || [];
  const blocked = diff.blocked || [];
  const unmatched = diff.unmatched || [];
  const remap = diff.remap || [];
  const modeLabel = diff.mode === "bootstrap"
    ? "bootstrap (накатка по имени, метки переназначаются по файлу)"
    : (diff.mode || "—");
  let html = '<div class="imp-sec"><div class="imp-h">Режим сопоставления: ' + esc(modeLabel) + '</div>';
  html += '<div class="imp-row">создать: ' + create + ' · обновить: ' + update + ' · удалить: ' + del.length + '</div></div>';
  if (remap.length) {
    html += '<div class="imp-h">Метки будут переназначены по файлу (' + remap.length + ')</div>';
    html += remap.slice(0, 100).map(r =>
      '<div class="imp-row imp-remap">' + esc((r.section || "") + " · " + (r.old_code || "—") + " → " + r.code + " · " + (r.name || "")) + '</div>').join("");
    if (remap.length > 100) html += '<div class="imp-row">…ещё ' + (remap.length - 100) + '</div>';
  }
  if (del.length) {
    html += '<div class="imp-h">Удалить (нет в файле)</div>';
    html += del.slice(0, 200).map(d =>
      '<div class="imp-row imp-del">' + esc((d.section || "") + " · " + (d.code || "—") + " · " + (d.name || "")) + '</div>').join("");
    if (del.length > 200) html += '<div class="imp-row">…ещё ' + (del.length - 200) + '</div>';
  }
  if (blocked.length) {
    html += '<div class="imp-h">Заблокировано мировыми ссылками</div>';
    html += blocked.map(b =>
      '<div class="imp-row imp-block">' + esc((b.section || "") + " · " + (b.code || "—") + " · " + (b.name || "") +
      " → " + esc(b.ref_table || "") + " × " + b.ref_count) + '</div>').join("");
  }
  if (unmatched.length) {
    html += '<div class="imp-h">Расхождения тождества (unmatched)</div>';
    html += unmatched.map(u =>
      '<div class="imp-row imp-block">' + esc((u.section || "") + " · " + (u.name || "")) + '</div>').join("");
  }
  const canApply = blocked.length === 0 && unmatched.length === 0;
  html += '<div class="imp-btns"><button id="importCancelBtn">Отмена</button>' +
    '<button id="importApplyBtn"' + (canApply ? "" : ' disabled title="есть blocked/unmatched — применение запрещено"') + '>Применить</button></div>';
  $("importBody").innerHTML = html;
  $("importCancelBtn").addEventListener("click", closeImportPopup);
  const applyBtn = $("importApplyBtn");
  if (applyBtn && canApply) applyBtn.addEventListener("click", applyImportContent);
  $("importOverlay").style.display = "block";
  $("importPopup").style.display = "flex";
}
function closeImportPopup() {
  importDiff = null;
  $("importPopup").style.display = "none";
  $("importOverlay").style.display = "none";
}
// applyImportContent — шаг 2: второе подтверждение (операция пишет в БД, включая
// удаления) и попап-отчёт.
async function applyImportContent() {
  const del = importDiff && importDiff.delete ? importDiff.delete.length : 0;
  const msg = "Применить импорт контента?" + (del ? "\nУдалений: " + del + " (необратимо без бэкапа)." : "");
  if (!window.confirm(msg)) return;
  const applyBtn = $("importApplyBtn");
  if (applyBtn) applyBtn.disabled = true;
  showReport(["Применение импорта контента…"], true);
  const r = await api("POST", "/studio/api/content/import", {});
  if (!r) { closeImportPopup(); return; } // guard: api → null (ошибка/блок)
  let rep = null;
  try { rep = await r.json(); } catch (e) {}
  closeImportPopup();
  if (!rep) return;
  showReport(["Импорт контента: создано " + sumCounts(rep.created) + ", обновлено " +
    sumCounts(rep.updated) + ", удалено " + sumCounts(rep.deleted)], true);
  fetchState();
}