"use strict";
// ---------- контекстное меню (правый клик по карточке товара/ресурса) ----------
let ctxGoodId = null;
function showCtxMenu(ev, goodId) {
  const g = state.goods.find(x => x.id === goodId);
  if (!g) return;
  ctxGoodId = goodId;
  const items = [];
  // «+ слот» и «заполнить комплектующие» — только товарам (ресурс — лист, §4.5)
  if (g.kind !== "resource") {
    items.push({label: "+ слот", fn: () => { selectGood(goodId); openPopup(goodId); addSlot(); }});
    items.push({label: "заполнить комплектующие", fn: () => { selectGood(goodId); openPopup(goodId); fillGood(); }});
  }
  items.push({label: "добавить родителя", fn: () => { selectGood(goodId); openPopup(goodId); askAddParent(); }});
  items.push({label: "переименовать", fn: () => { selectGood(goodId); openPopup(goodId); startRename(); }});
  items.push({label: "удалить", fn: () => { selectGood(goodId); openPopup(goodId); askDeleteGood(); }});
  items.push({label: "открыть деталь", fn: () => { selectGood(goodId); openPopup(goodId); }});
  const menu = $("ctxMenu");
  menu.innerHTML = items.map((it, i) => '<button data-i="' + i + '">' + esc(it.label) + '</button>').join("");
  menu.querySelectorAll("button").forEach(b => b.addEventListener("click", () => {
    const it = items[+b.dataset.i];
    hideCtxMenu();
    it.fn();
  }));
  const mw = 210, mh = items.length * 30 + 8;
  let x = ev.clientX, y = ev.clientY;
  if (x + mw > window.innerWidth) x = window.innerWidth - mw - 8;
  if (y + mh > window.innerHeight) y = window.innerHeight - mh - 8;
  menu.style.left = x + "px"; menu.style.top = y + "px";
  menu.style.display = "block";
}
function hideCtxMenu() { $("ctxMenu").style.display = "none"; ctxGoodId = null; }
// контекстное меню карточки в списке справочника (создатель 2026-09-19):
// переиспользует контейнер #ctxMenu, пункты свои; работает и для ресурсов.
// Скрытия у товара нет (2026-09-21) — только удаление.
function showSpravCtxMenu(ev, id) {
  const g = state.goods.find(x => x.id === id);
  if (!g) return;
  ctxGoodId = id;
  const items = [];
  // пункт «привязать к „{фабрика}“» (ТЗ §7.6): только в режиме фабрики и если
  // товар ещё не в наборе; у ресурса/чужой категории клик даёт тост пречека
  if (factoryMode()) {
    const ft = factoryType();
    if (!(g.bound_factories || []).includes(ft.id)) {
      items.push({label: "привязать к „" + ft.name + "“", fn: () => ctxBindToFactory(id)});
    }
  }
  items.push({label: "удалить", fn: () => askDeleteGoodFor(id)});
  const menu = $("ctxMenu");
  menu.innerHTML = items.map((it, i) => '<button data-i="' + i + '">' + esc(it.label) + '</button>').join("");
  menu.querySelectorAll("button").forEach(b => b.addEventListener("click", () => {
    const it = items[+b.dataset.i];
    hideCtxMenu();
    it.fn();
  }));
  const mw = 210, mh = items.length * 30 + 8;
  let x = ev.clientX, y = ev.clientY;
  if (x + mw > window.innerWidth) x = window.innerWidth - mw - 8;
  if (y + mh > window.innerHeight) y = window.innerHeight - mh - 8;
  menu.style.left = x + "px"; menu.style.top = y + "px";
  menu.style.display = "block";
}
// ctxBindToFactory — клик пункта «привязать к „{фабрика}“» (ТЗ §7.6): те же
// пречеки, что у drop (единый источник правды, §7.3.2).
function ctxBindToFactory(id) {
  const g = state.goods.find(x => x.id === id);
  const ft = factoryMode() ? factoryType() : null;
  if (!g || !ft) return;
  if (g.kind === "resource") { showReport(["ресурс нельзя привязать как рецепт"]); return; }
  // категория НЕ ограничивает набор (шаг 2 §3.2.1): кросс-категорийная привязка
  // разрешена и здесь, как на дереве «Производителей» (решение создателя 2026-09-23)
  bindRecipeToFactory(g, ft.id);
}
// depositsCountForDelete — предпроверка числа залежей ресурса перед удалением
// (§3.3/T14): GET /studio/api/goods/{id}/deposits-count → {count}. Ошибка или
// недоступность ручки → 0: удаление не блокируем, каскад FK сработает и так.
async function depositsCountForDelete(goodId) {
  try {
    const r = await studioFetch("/studio/api/goods/" + goodId + "/deposits-count", {method: "GET"});
    if (!r.ok) return 0;
    const out = await r.json();
    return out && typeof out.count === "number" ? out.count : 0;
  } catch (e) {
    return 0;
  }
}
// depositWarningLine — строка предупреждения о каскаде залежей в подтверждении
// удаления ресурса (пусто при count = 0).
function depositWarningLine(dep) {
  if (!dep || dep <= 0) return "";
  return '<div style="color:#f87171;margin-top:6px;">Будет удалено ' + dep + ' залежей (каскад).</div>';
}
// удаление по id (для контекстного меню списка): модалка 1 → (N>0) модалка 2
// → DELETE → тост с фактическим K из ответа (спека iterB §4.7)
async function askDeleteGoodFor(id) {
  const g = state.goods.find(x => x.id === id);
  if (!g) return;
  const n = state.goods.reduce((acc, p) => acc + (p.recipe || []).filter(s => s.good_id === g.id).length, 0);
  const dep = await depositsCountForDelete(g.id);
  openModal(
    '<h3>Удалить товар</h3>' +
    '<div>Удалить товар «' + esc(g.name) + '»?</div>' +
    depositWarningLine(dep),
    [
      {label: "Удалить", cls: "del", primary: true, action: () => confirmDeleteGoodFor(g, n, dep)},
      {label: "Отмена", cls: "", action: closeModal}
    ]
  );
}
function confirmDeleteGoodFor(g, n, dep) {
  if (n > 0) {
    openModal(
      '<h3>Удалить товар</h3>' +
      '<div>Слоты, ссылающиеся на него (' + n + '), очистятся — рецепты этих товаров будут испорчены. Удалить всё равно?</div>' +
      depositWarningLine(dep),
      [
        {label: "Удалить и очистить " + n + " ссылок", cls: "del", primary: true, action: () => doDeleteGoodFor(g.id)},
        {label: "Отмена", cls: "", action: closeModal}
      ]
    );
    return;
  }
  doDeleteGoodFor(g.id);
}
async function doDeleteGoodFor(id) {
  closeModal();
  const r = await api("DELETE", "/studio/api/goods/" + id);
  if (r) {
    const out = await r.json();
    const k = out && out.cleared_links !== undefined ? out.cleared_links : 0;
    showReport([(k > 0 ? "⚠ " : "") + "Удалено. Очищено ссылок: " + k]);
  }
  if (selected === id) selected = null;
  if (popupGoodId === id) closePopup();
  if (r) fetchState();
}
canvas.addEventListener("contextmenu", ev => {
  ev.preventDefault();
  if (branch !== "goods") { hideCtxMenu(); return; } // контекстное меню — только граф товаров
  const hit = hitTest(ev);
  if (hit && hit.type === "card") showCtxMenu(ev, hit.id);
  else hideCtxMenu();
});
document.addEventListener("click", ev => {
  if (!$("ctxMenu").contains(ev.target)) hideCtxMenu();
});
document.addEventListener("keydown", ev => {
  if (ev.key === "Escape") {
    hideCtxMenu();
    // Esc закрывает попапы, только если нет открытой модалки подтверждения (99a.3-ui §4.2/§14.7)
    if ($("modalOverlay").style.display !== "flex") {
      if (popupGoodId) closePopup();
      if ($("catPopup").style.display === "flex") closeCatPopup();
      if ($("effPopup").style.display === "flex") closeEffectPopup();
      if ($("proposalsPopup").style.display === "flex") closeProposalsPopup();
    }
  }
});
canvas.addEventListener("wheel", hideCtxMenu);

// ---------- drag&drop карточки справочника на канвас = привязка (ТЗ §7.3) ----------
// Ветка «Товары»: канвас принимает drop — рецепт перетащенного товара
// привязывается к ВЫБРАННОЙ фабрике. Бросок в любую точку канваса — одно
// действие (привязка — свойство фабрики, не слота); состав рецепта drop не
// меняет. Слоты попапа — отдельные цели (dropOnSlot), не конфликтуют.
// Ветка «Производители» в режиме справочника «Товары» (идея 2026-09-23,
// шаг 2 §3.1): цель — УЗЕЛ ПОД КУРСОРОМ в дереве построек (hitTest →
// prodNodeById), а не выбранный; это ДРУГОЙ резолвер цели при общем
// bindRecipeToFactory (ловушка §10).
function prodMode() { return branch === "producers" && spravMode === "goods"; }
// prodDropBlock — причина, по которой бросок недоступен (null = можно).
// Матрица §3.1: подтип — можно; тип/класс/заглушка — «не классу»; invite —
// «сначала создайте»; ресурс/без рецепта/повтор — свои тосты.
function prodDropBlock(g, nodeId, p) {
  if (!p || !p.parent_id) {
    if (nodeId.indexOf("invite:") === 0) return "Сначала создайте постройку";
    return "Рецепт назначается конкретной постройке (дочке), не классу";
  }
  if (!g) return "товар не найден";
  if (g.kind === "resource") return "ресурс нельзя назначить как рецепт";
  if (!g.recipe_id) return "рецепт не создан";
  if ((g.bound_factories || []).some(fid => String(fid) === String(p.id))) return "уже в наборе постройки";
  return null;
}
function dndReset() {
  dndDepth = 0;
  dndActive = false;
  const wasHighlighted = !!prodDropHighlight;
  prodDropHighlight = null;
  $("canvasWrap").classList.remove("dnd-over");
  if (wasHighlighted && branch === "producers") renderGraph();
}
$("canvasWrap").addEventListener("dragenter", ev => {
  if (branch === "goods") {
    dndActive = true;
    dndDepth++;
    if (factoryMode()) $("canvasWrap").classList.add("dnd-over"); // подсветка только с выбранной фабрикой
    return;
  }
  if (!prodMode()) return; // «Производители» без режима «Товары» — дерево drop не принимает
  dndActive = true;
  dndDepth++;
});
$("canvasWrap").addEventListener("dragleave", ev => {
  if (dndDepth > 0) dndDepth--;
  if (dndDepth === 0) dndReset2();
});
// dndReset2 — снять подсветку при уходе курсора с канваса, не сбрасывая dndActive
// (перетаскивание продолжается, ТЗ §7.3.3).
function dndReset2() {
  $("canvasWrap").classList.remove("dnd-over");
  if (prodDropHighlight) { prodDropHighlight = null; renderGraph(); }
}
$("canvasWrap").addEventListener("dragover", ev => {
  if (branch === "goods") {
    ev.preventDefault();
    ev.dataTransfer.dropEffect = factoryMode() ? "copy" : "none";
    return;
  }
  if (!prodMode()) return; // без preventDefault — drop инертен
  ev.preventDefault();
  const hit = hitTest(ev);
  const nodeId = hit && hit.type === "card" ? hit.id : null;
  const target = nodeId ? prodNodeById(nodeId) : null;
  const ok = !!(target && target.parent_id); // цель — только запись-подтип (шаг 2 §3.1)
  ev.dataTransfer.dropEffect = ok ? "copy" : "none";
  const next = ok ? nodeId : null;
  if (prodDropHighlight !== next) { prodDropHighlight = next; renderGraph(); } // подсветка карточки-подтипа
});
$("canvasWrap").addEventListener("drop", ev => {
  dndReset();
  const goodId = ev.dataTransfer.getData("text/plain");
  const g = state.goods.find(x => x.id === goodId);
  if (branch === "goods") {
    ev.preventDefault();
    if (!factoryMode()) {
      showReport(["Выберите фабрику в шапке — перетаскивание привяжет рецепт к ней"]);
      return;
    }
    if (!g) { showReport(["товар не найден"]); return; }
    const ft = factoryType();
    if (g.kind === "resource") { showReport(["ресурс нельзя привязать как рецепт"]); return; }
    // категория НЕ ограничивает набор (шаг 2 §3.2.1) — как на дереве «Производителей»
    if ((g.bound_factories || []).includes(ft.id)) { showReport(["уже в наборе постройки"]); return; }
    bindRecipeToFactory(g, ft.id);
    return;
  }
  if (!prodMode()) return;
  ev.preventDefault();
  const hit = hitTest(ev);
  const nodeId = hit && hit.type === "card" ? hit.id : null;
  if (!nodeId) return; // пустое место канваса — инертно, без тоста (случайный промах)
  const p = prodNodeById(nodeId);
  const block = prodDropBlock(g, nodeId, p);
  if (block) { showReport([block]); return; }
  bindRecipeToFactory(g, p.id);
});
// отмена переноса (Esc/drop вне цели/другая программа) — снять подсветку/счётчик
// (делегированно: карточки справочника перерисовываются, ТЗ §7.3.3)
document.addEventListener("dragend", dndReset);

// bindRecipeToFactory — единый путь привязки рецепта к постройке (drop §7.3.2,
// контекстное меню §7.6, попап §6.4). Успех — без тоста (визуальная обратная
// связь: узел в графе + бейдж «в наборе»); 4xx — тост api(). Защита от
// двойного POST — bindingInFlight по ключу producerId:recipeId (иначе быстрый
// бросок одного рецепта на две разные постройки молча терял второй, §3.1).
async function bindRecipeToFactory(g, factoryId) {
  if (!g) return false;
  if (!g.recipe_id) { showReport(["рецепт не создан"]); return false; }
  const key = factoryId + ":" + g.recipe_id;
  if (bindingInFlight.has(key)) return false;
  bindingInFlight.add(key);
  try {
    const r = await api("POST", "/studio/api/producers/" + factoryId + "/recipes", {recipe_id: g.recipe_id});
    if (r) { fetchState(); return true; }
    return false;
  } finally {
    bindingInFlight.delete(key);
  }
}
