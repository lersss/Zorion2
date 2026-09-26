"use strict";
// ---------- оверлеи состояний ----------
// factoryOverlayText — текст оверлея режима фабрики (ТЗ §7.1 п.2–3): нет рецептов
// у фабрики либо фабрика не входит в набор выбранного семейства; null = нет оверлея.
function factoryOverlayText() {
  if (!factoryMode()) return null;
  const ft = factoryType();
  if (goodsFamily) {
    const applied = familyAppliedFactories(ft.category_id);
    const most = applied.length ? mostSpecificFactories(applied) : [];
    if (!most.some(p => p.id === ft.id)) {
      return "Фабрика «" + ft.name + "» не входит в набор семейства «" + familyLabelById(goodsFamily) + "» — выберите другую или „Все товары (обзор)“";
    }
  }
  if (factoryRecipeIds(ft.id).length === 0) {
    return "У фабрики «" + ft.name + "» нет рецептов — перетащите товар из справочника или создайте («+ товар»)";
  }
  return null;
}
function updateOverlays() {
  $("loadingOverlay").style.display = loading ? "flex" : "none";
  $("right").classList.toggle("disabled", loading);
  const empty = !loading && state.goods.length === 0;
  // оверлеи канваса — только ветка «Товары» (производители/предметы — списки)
  $("emptyOverlay").style.display = (branch === "goods" && empty) ? "flex" : "none";
  // оверлей режима фабрики (ТЗ §7.1) — приоритет выше focus/noResults
  const fo = (!loading && !empty && branch === "goods") ? factoryOverlayText() : null;
  $("factoryEmptyOverlay").style.display = fo ? "flex" : "none";
  if (fo) $("factoryEmptyOverlay").textContent = fo;
  // фокус-режим без выбора: «Кликните товар в справочнике» (99a.3 §5.3)
  const focusEmpty = !loading && !empty && branch === "goods" && !fo && !fullTreeMode() && !selected;
  $("focusEmptyOverlay").style.display = focusEmpty ? "flex" : "none";
  const noRes = !loading && !empty && branch === "goods" && !fo && fullTreeMode() && visibleGoods().length === 0;
  $("noResultsOverlay").style.display = noRes ? "flex" : "none";
  $("offlinePill").style.display = offline ? "block" : "none";
}
function focusNewGood() { openNewGoodPopup(); } // «+ товар» в оверлее «Реестр пуст» → попап «Новый товар» (ТЗ §7.4)

// ---------- режимы канваса (99a.3 §10.1 / 99a.3-ui §3.4) ----------
// В фокус-режиме скрыты: поиск по графу, фильтр категорий, «показать ресурсы»,
// «обратные рёбра», «⟲ на верхний тир». В «всё дерево» — видны.
function applyModeUI() {
  const fm = factoryMode();
  const full = fullTreeMode();
  $("search").style.display = full ? "" : "none";
  $("catFilter").style.display = full ? "" : "none";
  $("showResources").parentElement.style.display = full ? "" : "none";
  $("reverseEdges").parentElement.style.display = full ? "" : "none";
  $("btnTop").style.display = full ? "" : "none";
  // галка «всё дерево» в режиме фабрики скрыта (ТЗ §5.2)
  $("fullTree").parentElement.style.display = fm ? "none" : "";
  if (fm) {
    const ft = factoryType();
    $("modeBadge").textContent = "фабрика: " + ft.name + " · всё дерево · в наборе " + factoryRecipeIds(ft.id).length;
  } else {
    $("modeBadge").textContent = full ? "всё дерево" : "фокус: вокруг выбранного";
  }
}

// ---------- сборка ----------
function renderAll() {
  // выбранный узел удалён (авто-обновление/импорт) → сброс к пустому состоянию
  if (selected) {
    if (branch === "goods" && !state.goods.some(g => g.id === selected)) selected = null;
    if (branch === "producers" && !prodNodeById(selected)) selected = null;
  }
  const sc = $("spravList").scrollTop; // сохранить scroll списка справочника при пересборке
  renderGoodsFactorySel(); // селектор фабрики: пересборка опций с сохранением выбора (ТЗ §3)
  renderGoodsFamilySel();  // фильтр семейства (ТЗ §4)
  applyBranchUI();
  updateDescBatchBtn();
  if (branch === "goods") {
    renderGraph();
    if (!descInputDirty()) renderPopup(); // §9.7: не стирать несохранённый текст
    if ($("descPopup").style.display === "flex") refreshDescPopup();
    renderCatPopup(); // авто-обновление попапа категорий (если открыт)
    renderSprav();
    renderCatSelects();
  } else if (branch === "producers") {
    renderGraph(); // дерево построек на канвасе (спека 2026-09-21 §2)
    // режим «Товары» справочника (идея 2026-09-23, шаг 1): фильтр категорий
    // каталога + товарный попап (renderProdPopup закрыл бы карточку товара)
    if (spravMode === "goods") renderCatSelects();
    if (spravMode === "goods" && popupGoodId && state.goods.some(g => g.id === popupGoodId)) renderPopup();
    else renderProdPopup(); // попап производителя (если открыт)
    renderSprav(); // список справочника — вспомогательный (§2.5)
  } else {
    renderSprav();
  }
  updateOverlays();
  applyModeUI();
  $("spravList").scrollTop = sc;
}

// ---------- разделы студии (спека 2026-09-20-фабрики §4) ----------
// Товары — граф рецептов + справочник; Производители/Предметы — списки
// справочников в правой панели (канвас скрыт).
function setBranch(b) {
  branch = b;
  localStorage.setItem("gs_branch", b);
  $("branchGoods").classList.toggle("on", b === "goods");
  $("branchItems").classList.toggle("on", b === "items");
  renderBuildingsTabsUI();
  selected = null;
  popupGoodId = null;
  closePopup();
  // вид канваса по ветке: дерево построек — свой zoom/pan (спека §2.5)
  if (b === "producers") {
    zoom = parseFloat(localStorage.getItem("gs_prodZoom") || "1");
    panX = parseFloat(localStorage.getItem("gs_prodPanX") || "0");
    panY = parseFloat(localStorage.getItem("gs_prodPanY") || "0");
  } else {
    zoom = parseFloat(localStorage.getItem("gs_zoom") || "1");
    panX = parseFloat(localStorage.getItem("gs_panX") || "0");
    panY = parseFloat(localStorage.getItem("gs_panY") || "0");
  }
  renderAll();
}
function applyBranchUI() {
  const isGoods = branch === "goods";
  const isProducers = branch === "producers";
  $("segGoodsScope").style.display = isGoods ? "" : "none"; // скоп ветки «Товары» (ТЗ §2)
  $("segGraph").style.display = isGoods ? "" : "none";
  // ряд действий: кнопки вызова форм по активному разделу (ТЗ §7.2)
  $("btnNewGoodOpen").style.display = isGoods ? "" : "none";
  $("btnNewResOpen").style.display = isGoods ? "" : "none";
  $("btnBulkOpen").style.display = isGoods ? "" : "none";
  $("btnCatOpen").style.display = isGoods ? "" : "none";
  $("btnDescBatch").style.display = isGoods ? "" : "none";
  $("btnProdNewOpen").style.display = isProducers ? "" : "none";
  $("btnItemNewOpen").style.display = branch === "items" ? "" : "none";
  // канвас показывается и в ветке «Производители» — дерево построек (спека §2.5)
  $("canvasWrap").style.display = (isGoods || isProducers) ? "" : "none";
  // переключатель уровня расовости — только ветка «Производители» (спека §5)
  $("segProdRace").style.display = isProducers ? "" : "none";
  // ряд разделов построек (спека 2026-09-25 §6.1): виден всегда — это точка
  // входа в ветку построек (клик по разделу ставит branch="producers");
  // активная кнопка и видимость «Прочее» — renderBuildingsTabsUI.
  $("segBuildings").style.display = "";
  renderBuildingsTabsUI();
  $("spravSearch").style.display = (isGoods || isProducers) ? "" : "none";
  // переключатель справочника «Постройки | Товары» — только ветка «Производители»
  // (идея 2026-09-23, шаг 1); фильтры каталога видны на «Товарах» и в режиме
  // «Товары» справочника производителей
  $("segSpravMode").style.display = isProducers ? "" : "none";
  $("spravModeBuildings").classList.toggle("on", spravMode !== "goods");
  $("spravModeGoods").classList.toggle("on", spravMode === "goods");
  const goodsCat = isGoods || (isProducers && spravMode === "goods");
  $("spravCat").style.display = goodsCat ? "" : "none";
  $("filters").style.display = goodsCat ? "" : "none";
  $("tierFilters").style.display = goodsCat ? "" : "none";
}
// setSpravMode — переключатель «Постройки | Товары» правой панели вкладки
// «Производители» (идея 2026-09-23, шаг 1): влияет только на справочник,
// канвас-дерево построек не перерисовывается.
function setSpravMode(m) {
  spravMode = m;
  localStorage.setItem("gs_spravMode", m);
  if (m === "goods") renderCatSelects(); // заполнить фильтр категорий (вне renderAll)
  applyBranchUI();
  renderSprav();
}
function kindLabel(k) {
  return {goods: "товары", items: "предметы", energy: "энергия"}[k] || k;
}
function jsonText(v) {
  if (v === null || v === undefined) return "";
  if (typeof v === "string") return v;
  return JSON.stringify(v, null, 1);
}
$("catFilter").addEventListener("change", () => { localStorage.setItem("gs_catFilter", $("catFilter").value); renderAll(); });
// селектор фабрики: клиент-only пересчёт графа/маркеров (ТЗ §3/§7.4)
$("goodsFactorySel").addEventListener("change", () => {
  goodsFactory = $("goodsFactorySel").value;
  localStorage.setItem("gs_goodsFactory", goodsFactory);
  selected = null; // смена фабрики — выбор сбрасывается (ТЗ §7.4)
  renderAll();
});
// фильтр семейства: клиент-only пересчёт графа + справочника (ТЗ §4/§7.4)
$("goodsFamilySel").addEventListener("change", () => {
  goodsFamily = $("goodsFamilySel").value;
  localStorage.setItem("gs_goodsFamily", goodsFamily);
  if (selected && !visibleGoods().some(g => g.id === selected)) selected = null; // скрыт фильтром
  renderAll();
});
$("spravSearch").addEventListener("input", () => {
  localStorage.setItem("gs_spravSearch", $("spravSearch").value);
  renderSprav();
});
$("spravCat").addEventListener("change", () => {
  localStorage.setItem("gs_spravCat", $("spravCat").value);
  renderSprav();
});
$("search").addEventListener("input", renderAll);
$("reverseEdges").addEventListener("change", () => {
  localStorage.setItem("gs_reverseEdges", $("reverseEdges").checked ? "1" : "0");
  renderGraph();
});
$("showResources").addEventListener("change", () => {
  localStorage.setItem("gs_showResources", $("showResources").checked ? "1" : "0");
  renderAll();
});
$("fullTree").addEventListener("change", () => {
  localStorage.setItem("gs_fullTree", $("fullTree").checked ? "1" : "0");
  applyModeUI();
  renderAll();
});
// галка «показать скрытые» (спека скрытых §3.1): клиентский фильтр —
// renderAll без сброса zoom/pan и без запроса к серверу
$("prodShowHidden").addEventListener("change", () => {
  prodShowHidden = $("prodShowHidden").checked;
  localStorage.setItem("gs_prodShowHidden", prodShowHidden ? "1" : "0");
  renderAll();
});
// галки-фильтры справочника (99a.3 §8.2): пересборка списка, выбор не сбрасывается;
// состояние сохраняется в localStorage (создатель 2026-09-19: «при перезагрузке
// страницы не снимать выбранные фильтры»)
$("fUsed").addEventListener("change", () => {
  localStorage.setItem("gs_spravUsed", $("fUsed").checked ? "1" : "0");
  renderSprav();
});
$("fUnused").addEventListener("change", () => {
  localStorage.setItem("gs_spravUnused", $("fUnused").checked ? "1" : "0");
  renderSprav();
});
$("btnCatOpen").addEventListener("click", openCatPopup);
$("catAddBtn").addEventListener("click", addCategory);
$("catNewName").addEventListener("keydown", ev => { if (ev.key === "Enter") addCategory(); });
$("catPopupClose").addEventListener("click", closeCatPopup);
$("catOverlay").addEventListener("click", closeCatPopup);
// попап «Эффекты» (спека эффектов §7.4): кнопка вызова, закрытие, создание
$("btnEffectsOpen").addEventListener("click", openEffectPopup);
$("effAddBtn").addEventListener("click", addEffectType);
$("effNewName").addEventListener("keydown", ev => { if (ev.key === "Enter") addEffectType(); });
$("effPopupClose").addEventListener("click", closeEffectPopup);
$("effOverlay").addEventListener("click", closeEffectPopup);
// попап «Предложения ИИ»: закрытие (крестик/подложка); Esc — в общем keydown
$("proposalsPopupClose").addEventListener("click", closeProposalsPopup);
$("proposalsOverlay").addEventListener("click", closeProposalsPopup);
// попап «Описания ИИ»: закрытие (крестик/подложка) + кнопка пакетного прогона
$("descPopupClose").addEventListener("click", closeDescPopup);
$("descOverlay").addEventListener("click", closeDescPopup);
$("btnDescBatch").addEventListener("click", runDescBatch);
// кнопки локального ИИ-помощника (спека 2026-09-24 §4)
$("btnAiStart").addEventListener("click", aiStart);
$("btnAiStop").addEventListener("click", aiStop);
// кнопка экспорта снимка контента (спека 2026-09-24 §7, И2)
$("btnExportContent").addEventListener("click", exportContent);
// кнопка импорта снимка контента (спека 2026-09-24 §7, И3) + закрытие попапа
$("btnImportContent").addEventListener("click", importContent);
$("importPopupClose").addEventListener("click", closeImportPopup);
$("importOverlay").addEventListener("click", closeImportPopup);
// кнопки вызова форм добавления (ТЗ §7.2): сами формы живут в попапах (#modal),
// обработчики полей/кнопок навешивает openModal — при загрузке их в DOM нет
$("btnNewGoodOpen").addEventListener("click", openNewGoodPopup);
$("btnNewResOpen").addEventListener("click", openNewResPopup);
$("btnBulkOpen").addEventListener("click", openBulkPopup);
$("btnProdNewOpen").addEventListener("click", openProdNewPopup);
$("btnItemNewOpen").addEventListener("click", openItemNewPopup);
$("btnTop").addEventListener("click", centerTop);
// разделы студии (спека 2026-09-20-фабрики §4): переключение веток
$("branchGoods").addEventListener("click", () => setBranch("goods"));
$("branchItems").addEventListener("click", () => setBranch("items"));
// разделы построек (спека 2026-09-25 §6.2): клик по разделу входит в ветку
// построек и ставит активный раздел; «Прочее» — служебный раздел корней без
// раздела (кнопка видна только при их наличии).
PROD_SECTIONS.forEach(s => $(s.id).addEventListener("click", () => setProdSection(s.key)));
$("branchProdOther").addEventListener("click", () => setProdSection("other"));
// справочник вкладки «Производители»: переключатель «Постройки | Товары»
// (идея 2026-09-23, шаг 1)
$("spravModeBuildings").addEventListener("click", () => setSpravMode("buildings"));
$("spravModeGoods").addEventListener("click", () => setSpravMode("goods"));
// Перетаскивание попапа за заголовок (деталь/категории/эффекты — общий приём).
// Старт берёт ВИЗУАЛЬНУЮ позицию через getBoundingClientRect: offsetLeft/Top —
// это раскладка ДО transform:translate(-50%,-50%), поэтому по ним попап скакал
// в момент снятия transform (идея 2026-09-26). Движение удерживает попап в
// области канваса #canvasWrap, не давая улететь за экран.
function startPopupDrag(ev, p, closeBtn) {
  if (ev.target === closeBtn) return null;
  const r = p.getBoundingClientRect();
  const pr = (p.offsetParent || document.body).getBoundingClientRect();
  const wrap = $("canvasWrap");
  p.style.transform = "none";
  p.style.left = (r.left - pr.left) + "px";
  p.style.top = (r.top - pr.top) + "px";
  return {
    dx: ev.clientX - r.left, dy: ev.clientY - r.top,
    pl: pr.left, pt: pr.top,
    area: {left: wrap.offsetLeft, top: wrap.offsetTop, right: wrap.offsetLeft + wrap.offsetWidth, bottom: wrap.offsetTop + wrap.offsetHeight}
  };
}
function movePopupDrag(ev, p, d) {
  if (!d) return;
  // попап влезает в канвас — границы канваса; шире области — границы окна
  // (иначе он «прилипал» бы к левому краю и терял дельту drag)
  const fitsX = p.offsetWidth <= d.area.right - d.area.left;
  const fitsY = p.offsetHeight <= d.area.bottom - d.area.top;
  const loX = fitsX ? d.area.left : -d.pl;
  const hiX = fitsX ? d.area.right - p.offsetWidth : window.innerWidth - p.offsetWidth - d.pl;
  const loY = fitsY ? d.area.top : -d.pt;
  const hiY = fitsY ? d.area.bottom - p.offsetHeight : window.innerHeight - p.offsetHeight - d.pt;
  const left = ev.clientX - d.dx - d.pl;
  const top = ev.clientY - d.dy - d.pt;
  p.style.left = Math.min(Math.max(left, Math.min(loX, hiX)), Math.max(loX, hiX)) + "px";
  p.style.top = Math.min(Math.max(top, Math.min(loY, hiY)), Math.max(loY, hiY)) + "px";
}
let popupDrag = null, catPopupDrag = null, effPopupDrag = null;
// попап-деталь: закрытие (крестик/подложка), перетаскивание за заголовок
$("popupClose").addEventListener("click", closePopup);
$("detailOverlay").addEventListener("click", closePopup);
$("popupHead").addEventListener("mousedown", ev => {
  popupDrag = startPopupDrag(ev, $("detailPopup"), $("popupClose"));
  if (popupDrag) ev.preventDefault();
});
window.addEventListener("mousemove", ev => movePopupDrag(ev, $("detailPopup"), popupDrag));
window.addEventListener("mouseup", () => { popupDrag = catPopupDrag = effPopupDrag = null; });
// попап категорий: закрытие (крестик/подложка), перетаскивание за заголовок
$("catPopupHead").addEventListener("mousedown", ev => {
  catPopupDrag = startPopupDrag(ev, $("catPopup"), $("catPopupClose"));
  if (catPopupDrag) ev.preventDefault();
});
window.addEventListener("mousemove", ev => movePopupDrag(ev, $("catPopup"), catPopupDrag));
// попап «Эффекты»: перетаскивание за заголовок (по образцу попапа категорий)
$("effPopupHead").addEventListener("mousedown", ev => {
  effPopupDrag = startPopupDrag(ev, $("effPopup"), $("effPopupClose"));
  if (effPopupDrag) ev.preventDefault();
});
window.addEventListener("mousemove", ev => movePopupDrag(ev, $("effPopup"), effPopupDrag));

// localStorage: переключатели применяются при старте
$("showResources").checked = localStorage.getItem("gs_showResources") === "1";
$("reverseEdges").checked = localStorage.getItem("gs_reverseEdges") === "1";
$("fullTree").checked = localStorage.getItem("gs_fullTree") === "1";
$("prodShowHidden").checked = prodShowHidden;
// фильтры справочника восстанавливаются (создатель 2026-09-19: «при перезагрузке
// страницы не снимать выбранные фильтры»); renderSprav соберёт пилюли с .on
$("fUsed").checked = localStorage.getItem("gs_spravUsed") === "1";
$("fUnused").checked = localStorage.getItem("gs_spravUnused") === "1";
$("spravSearch").value = localStorage.getItem("gs_spravSearch") || "";
$("spravCat").value = localStorage.getItem("gs_spravCat") || "";
try { tierSel = new Set(JSON.parse(localStorage.getItem("gs_tierSel") || "[]")); } catch (e) { tierSel = new Set(); }

// правая панель поверх канваса: сворачивание + сохранение
function togglePanel() {
  const right = $("right");
  const collapsed = right.classList.toggle("collapsed");
  $("panelToggle").textContent = collapsed ? "▶" : "◀";
  localStorage.setItem("gs_panelCollapsed", collapsed ? "1" : "0");
  // канвас и подложка попапа вытягиваются на освободившееся место
  // (создатель 2026-09-19: «кнопка должна вытягивать канвас, иначе зачем она нужна»)
  $("canvasWrap").classList.toggle("expanded", collapsed);
  $("detailOverlay").classList.toggle("expanded", collapsed);
  // попапы центрируются по канвасу: при свёрнутой панели центр — по всей ширине
  $("detailPopup").classList.toggle("expanded", collapsed);
  $("catPopup").classList.toggle("expanded", collapsed);
  $("effPopup").classList.toggle("expanded", collapsed);
  $("proposalsPopup").classList.toggle("expanded", collapsed);
  $("descPopup").classList.toggle("expanded", collapsed);
  $("importPopup").classList.toggle("expanded", collapsed);
  centerTop(); // пересчёт размеров канваса + перерисовка
}
$("panelToggle").addEventListener("click", togglePanel);
if (localStorage.getItem("gs_panelCollapsed") === "1") {
  $("right").classList.add("collapsed");
  $("canvasWrap").classList.add("expanded");
  $("detailOverlay").classList.add("expanded");
  $("detailPopup").classList.add("expanded");
  $("catPopup").classList.add("expanded");
  $("effPopup").classList.add("expanded");
  $("proposalsPopup").classList.add("expanded");
  $("descPopup").classList.add("expanded");
  $("importPopup").classList.add("expanded");
  $("panelToggle").textContent = "▶";
}
// при изменении размера окна — пересчитать граф (canvas теперь 100% ширины)
window.addEventListener("resize", () => { centerTop(); });

updateOverlays(); // спиннер при первичной загрузке (BUG-2)
applyModeUI();   // видимость топбар-элементов по режиму при старте
// разделы студии: активная ветка при старте (спека 2026-09-20-фабрики §4)
$("branchGoods").classList.toggle("on", branch === "goods");
$("branchItems").classList.toggle("on", branch === "items");
// активный раздел построек: «Прочее» — полноправный активный раздел, остальное
// неизвестное/пустое → «Колонии» (спека 2026-09-25 §6.6; легаси gs_branch=
// "producers" открывает «Колонии»)
if (prodSection !== "other" && !PROD_SECTION_KEYS[prodSection]) prodSection = "colony";
renderBuildingsTabsUI();
applyBranchUI();
// вид канваса по ветке при старте: дерево построек — свой zoom/pan (спека §2.5)
if (branch === "producers") {
  zoom = parseFloat(localStorage.getItem("gs_prodZoom") || "1");
  panX = parseFloat(localStorage.getItem("gs_prodPanX") || "0");
  panY = parseFloat(localStorage.getItem("gs_prodPanY") || "0");
}
renderProdRaceUI(); // переключатель расовости (данные придут с /studio/api/races)
// старт: авторизация (спека iterC §8.3) → start() = fetchState + интервал;
// bootstrap — в модуле web/static/js/studio/auth.js (после парсинга документа)
function start() { fetchState(); refreshImportStatus(); }
// первичное центрирование на верхнем тире (если pan уехал от старого широкого канваса)
window.addEventListener("load", () => setTimeout(() => {
  // если позиция уже сохранялась — оставить как была (решение создателя
  // 2026-09-19: «позицию экрана после обновления страницы нужно оставлять»);
  // иначе (первый заход) — центрировать на верхушках
  if (localStorage.getItem("gs_viewPos") !== "1") centerTop();
}, 300));
