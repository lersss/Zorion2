"use strict";
const $ = id => document.getElementById(id);
let state = {categories:[], goods:[], unused:[], warnings:[], model:"", generating:false,
  // до первого fetchState читатели (concreteGoodsFactories и пр.) не должны
  // падать на undefined — держим пустые массивы, а не отсутствующие поля
  items:[], producer_types:[], producer_slots:[], producer_recipes:[]};
let selected = null;
let popupGoodId = null; // товар попапа-детали, отдельно от selected (99a.3 §10.3, вердикт критика С1)
let tierSel = new Set(); // выбранные тиры-пилюли справочника; пусто = фильтр выключен = все тиры (99a.3-ui §5.3)
// Шкала тиров каталога 0..8 (решение создателя 2026-09-19: верхний тир = Т8).
// Пилюли 0..8 показываются ВСЕГДА (создатель 2026-09-20), даже если товаров
// такого тира в базе нет — пусто видно сразу; тиры выше 8 (оверрайд) — динамически.
const TIER_FILTER_MAX = 8;
function spravTiers() {
  const base = Array.from({ length: TIER_FILTER_MAX + 1 }, (_, i) => i);
  const extra = [...new Set(state.goods.map(g => g.tier))].filter(t => t > TIER_FILTER_MAX).sort((a, b) => a - b);
  return [...base, ...extra];
}
let zoom = parseFloat(localStorage.getItem("gs_zoom") || "1");
let panX = parseFloat(localStorage.getItem("gs_panX") || "0");
let panY = parseFloat(localStorage.getItem("gs_panY") || "0");
let lastGoodsJSON = "";
let lastCatsJSON = ""; // категории тоже перерисовывают UI (попап категорий, селекты)
let lastProdSlotsJSON = "";
let lastProdTypesJSON = ""; // ветка «Производители»: слоты/типы тоже перерисовывают UI (баг B3)
let lastProdRecipesJSON = ""; // привязки рецептов — источник бейджей/графа фабрики (ТЗ §7.4)
let lastShownReport = ""; // отчёт fill — один раз, по изменению содержимого (спека iterC §6.4)
let lastProposalsKey = ""; // ключ предложений ИИ — авто-открытие попапа один раз (iterC §6.5, М1)
let proposalsAccepted = {}; // {index: {accepted, category_id}} — выбор в попапе «Предложения ИИ»
let descAccepted = {}; // {index: {accepted, text}} — выбор в попапе «Описания ИИ» (спека §9.4)
let lastDescKey = "";  // ключ набора предложений описаний — авто-открытие один раз (§9.4)
let lastShownDescReport = ""; // отчёт описаний — один раз по изменению
let dragging = null; // {goodId, name}
let loading = true;  // первичный /studio/api/state ещё не пришёл
let offline = false; // сеть потеряна (повтор идёт)
let pendingCatFilter = localStorage.getItem("gs_catFilter") || "";
// Раздел студии (спека 2026-09-20-фабрики §4): goods (граф рецептов) /
// producers (типы производителей) / items (предметы). Сохраняется.
let branch = localStorage.getItem("gs_branch") || "goods";
// Активный раздел построек (спека 2026-09-25 §6.6): colony/factory/lab/mining/
// energy; значение вне списка/отсутствует → colony. Сохраняется; ветка построек
// технически остаётся branch="producers" (дерево/скоп/M-mode не переписаны).
let prodSection = localStorage.getItem("gs_prodSection") || "colony";
// Уровень расовости дерева построек (спека 2026-09-21 §2.4/§5):
// universal → family (F0–F10) → race (60). Состояние — в localStorage.
let prodRaceLevel = localStorage.getItem("gs_prodRaceLevel") || "universal";
let prodRaceFamily = localStorage.getItem("gs_prodRaceFamily") || "";
let prodRace = localStorage.getItem("gs_prodRace") || "";
let prodShowHidden = localStorage.getItem("gs_prodShowHidden") === "1"; // галка «показать скрытые» (спека скрытых §3.1)
let racesData = null; // {families, races} из /studio/api/races (спека §3)
let racesLoaded = false;
// Скоп ветки «Товары» (спека 2026-09-21-рецепт-сущность §4.1/§4.3, ТЗ §3–§4):
// выбранная конкретная фабрика ("" = «Все товары (обзор)») и фильтр семейства
// рас ("" = «Все семейства»). Сохраняются в localStorage (§8 ТЗ).
let goodsFactory = localStorage.getItem("gs_goodsFactory") || "";
let goodsFamily = localStorage.getItem("gs_goodsFamily") || "";
// Режим справочника вкладки «Производители» (идея 2026-09-23, шаг 1):
// "buildings" — список типов построек (текущее поведение), "goods" — справочник
// каталога товаров. Влияет ТОЛЬКО на правую панель; канвас-дерево не трогает.
let spravMode = localStorage.getItem("gs_spravMode") || "buildings";
// Масштаб отображения/ввода единицы темпа (задача «переключатель масштаба
// единицы», спека 2026-09-23 §2.1). Хранимая единица одна — «ед/сутки/млрд»;
// масштаб влияет ТОЛЬКО на показ/ввод, на сервер значение уходит в хранимой
// единице. Ключ общий с витриной поселения; дефолт — хранимая единица («на
// млрд», как было до переключателя). Множители — в web/static/js/unit_scale.js.
let unitScale = localStorage.getItem("gs_unitScale") || "billion";
const bindingInFlight = new Set(); // recipe_id в полёте — защита от двойного POST привязки (ТЗ §7.3.2)
let dndActive = false; // перетаскивание карточки на канвас — глушит зум/пан (ТЗ §7.3.3)
let dndDepth = 0;      // счётчик dragenter/dragleave для подсветки канваса (ТЗ §7.3.3)
let prodDropHighlight = null; // узел дерева построек под курсором при drag (идея 2026-09-23, шаг 2 §3.1)
// Редактор «Потребляет» (спека 2026-09-23 §11.1 п.1): строки позиция → эффект
// при нехватке → норма. Модель живёт, пока открыт попап этого типа
// (prodEatRowsFor = id), иначе пересобирается из params.eat/effects.
let prodEatRows = null;
let prodEatRowsFor = null;
// Редактор «Стадия» (спека 2026-09-23 §11.1 п.3): значения полей порогов живут,
// пока открыт попап этого типа (prodStageEditFor = id), иначе берутся из params.
let prodStageEdit = null;
let prodStageEditFor = null;
// Типы эффектов для селекта «эффект при нехватке»: отдельный эндпоинт
// /studio/api/effects (в общем state их нет), кэш на сессию.
let effectTypes = null;
let effectTypesLoading = false;
let effectTypesFailed = false;

const canvas = $("graph");
const ctx = canvas.getContext("2d");
const CARD_W = 190, CARD_H = 86, GAP_X = 26, SLOT_H = 26, ROW_GAP = 64, PAD = 30, LINE_H = 12;
// нижняя граница зума — общая для колеса и авто-вписывания (авто не должно
// уходить ниже доступного вручную): дерево построек ~35 колонок требует ~0.134.
const MIN_ZOOM = 0.1;
let dropTarget = null; // {goodId, slotIndex} — подсветка слота под курсором при drag

// ---------- состояние ----------
async function fetchState() {
  try {
    const r = await studioFetch("/studio/api/state");
    if (!r.ok) return;
    state = await r.json();
    if (!racesLoaded) fetchRaces(); // уровни расовости дерева построек (спека 2026-09-21 §3)
    loading = false;
    offline = false;
    $("modelInfo").textContent = "Модель ИИ: " + (state.model || "—");
    $("genIndicator").textContent = state.generating ? "⚙ генерация… (" + (state.model || "модель не задана") + ")" : "";
    updateAIStatus(); // индикатор/кнопки локального ИИ-помощника (спека 2026-09-24 §4)
    updateDescBatchBtn(); // кнопка пакетного прогона: disabled при generating (спека §9.6)
    // отчёт fill — один раз, по изменению содержимого (не мигает при опросах;
    // спека iterC §6.4, эталон index.html:269–273)
    if (state.report && state.report.length) {
      const rj = JSON.stringify(state.report);
      if (rj !== lastShownReport) { lastShownReport = rj; showReport(state.report, true); }
    } else {
      lastShownReport = "";
    }
    // попап «Предложения ИИ» — авто-открытие по ключу (спека iterC §6.5):
    // повторный опрос тот же набор не переоткрывает; else-ветка сбрасывает
    // ключ (М1: «Отмена» → повторный fill → тот же набор ИИ → попап снова
    // открывается) и закрывает попап при старте нового fill (М6)
    const props = state.proposals || [];
    if (!state.generating && props.length > 0) {
      const key = JSON.stringify(props) + state.proposals_good_id;
      if (key !== lastProposalsKey) {
        lastProposalsKey = key;
        openProposalsPopup();
      }
    } else {
      lastProposalsKey = "";
      closeProposalsPopup();
    }
    // отчёт описаний — тост один раз по изменению (спека §9.6)
    if (state.desc_report && state.desc_report.length) {
      const drj = JSON.stringify(state.desc_report);
      if (drj !== lastShownDescReport) { lastShownDescReport = drj; showReport(state.desc_report, true); }
    } else {
      lastShownDescReport = "";
    }
    // ожидание прогона (fill/описания): счётчик секунд в отчёте
    if (state.generating || state.desc_generating) aiWaitStart(); else aiWaitStop();
    // попап «Описания ИИ» — авто-открытие на новый набор; во время прогона
    // не закрывается, строки/прогресс обновляются опросом (спека §9.4)
    const dprops = state.desc_proposals || [];
    if (dprops.length > 0) {
      const dkey = JSON.stringify(dprops) + state.desc_total;
      if ($("descPopup").style.display !== "flex") {
        if (dkey !== lastDescKey) { lastDescKey = dkey; openDescPopup(); }
      } else {
        lastDescKey = dkey;
        refreshDescPopup();
      }
    } else {
      lastDescKey = "";
    }
    // авто-обновление: сервер шлёт auto_refresh_ms=0 — клиентский дефолт 3000 (С1)
    if (!window.__refreshStarted) {
      window.__refreshStarted = true;
      setInterval(fetchState, state.auto_refresh_ms || 3000);
    }
    const gj = JSON.stringify(state.goods);
    const cj = JSON.stringify(state.categories);
    // Ветка «Производители»: слоты родителя (producer_slots) и типы тоже
    // перерисовывают UI (редактор слотов в попапе, дерево, список справа) —
    // иначе мутация /slots или подтипа не перерисовывала UI, пока не менялись
    // goods/categories (баг B3, 2026-09-21).
    const pj = JSON.stringify(state.producer_slots);
    const tj = JSON.stringify(state.producer_types);
    const prj = JSON.stringify(state.producer_recipes); // привязки рецептов (ТЗ §7.4)
    if (gj !== lastGoodsJSON || cj !== lastCatsJSON || pj !== lastProdSlotsJSON || tj !== lastProdTypesJSON || prj !== lastProdRecipesJSON) {
      lastGoodsJSON = gj; lastCatsJSON = cj; lastProdSlotsJSON = pj; lastProdTypesJSON = tj; lastProdRecipesJSON = prj; renderAll();
    }
    updateOverlays();
  } catch (e) {
    if (e && e.message === "Unauthorized") return; // 401 — оверлей показан, ретрай не нужен
    offline = true;
    if (!window.__refreshStarted) { // повтор и при сбое первого /studio/api/state (BUG-3)
      window.__refreshStarted = true;
      setInterval(fetchState, (state && state.auto_refresh_ms) || 3000);
    }
    updateOverlays();
  }
}
async function fetchResources() {
  // /api/resources остаётся на сервере (обратная совместимость, 99a.3 §8.5);
  // справочник строит список из state.goods — этот вызов не используется.
}
function showReport(lines, persist) {
  const el = $("report");
  aiWaitShown = false;
  el.innerHTML = lines.map(l => '<div class="rline">' + esc(l) + '</div>').join("") +
    (persist ? '<button class="rclose" onclick="hideReport()" title="скрыть отчёт">×</button>' : "");
  el.style.display = "block";
  clearTimeout(showReport._t);
  if (!persist) showReport._t = setTimeout(() => el.style.display = "none", 8000);
}
function hideReport() { clearTimeout(showReport._t); $("report").style.display = "none"; }
// Отсчёт ожидания ИИ-прогона (решение создателя 2026-09-24): пока идёт прогон
// (fill или описания), в отчёте видно «ИИ думает… N с» — долгое ожидание и
// провал не выглядят как «ничего не произошло». Отчёт прогона не гаснет сам
// (persist), его снимает следующий отчёт или крестик.
let aiWaitSince = 0, aiWaitTimer = 0, aiWaitShown = false;
function aiWaitStart() {
  if (aiWaitSince) return;
  aiWaitSince = Date.now();
  const tick = () => {
    if (!aiWaitSince) return;
    showReport(["ИИ думает… " + Math.round((Date.now() - aiWaitSince) / 1000) + " с"], true);
    aiWaitShown = true;
  };
  tick();
  aiWaitTimer = setInterval(tick, 1000);
}
function aiWaitStop() {
  if (!aiWaitSince) return;
  clearInterval(aiWaitTimer);
  aiWaitSince = 0;
  if (aiWaitShown) hideReport(); // прогон кончился, отчёта нет — снимаем счётчик
  aiWaitShown = false;
}
function esc(s) { return String(s).replace(/[&<>"]/g, c => ({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;"}[c])); }
// codeNote — справочная метка переноса записи каталога (code, спека
// 2026-09-24-каталог-экспорт… §3.3): только чтение, служит сверке dev↔прод.
// Пусто — строки нет.
function codeNote(code) {
  return code ? '<div class="hint code-note">метка переноса: <code>' + esc(code) + '</code></div>' : '';
}

// ---------- граф ----------
// Режимы канваса (99a.3 §5/§6): фокус-подграф по умолчанию, «всё дерево» —
// галкой. Тиры для отображения берутся из серверных полей GoodView.tier /
// SlotView.Tier (эффективный, 99a.3 §9.3) — клиент тиры не считает (М5).
function fullTreeMode() { return factoryMode() ? true : $("fullTree").checked; } // режим фабрики — всегда «всё дерево» (ТЗ §5.2)
// focusSubset — подграф «вокруг выбранного» (99a.3 §5.1): выбранный + прямые
// родители (в чьих рецептах есть выбранный) + прямые дети (составляющие
// рецепта выбранного). Ресурсы, попавшие в подграф, видны всегда.
function focusSubset() {
  if (!selected) return [];
  const byId = {};
  state.goods.forEach(g => byId[g.id] = g);
  const sel = byId[selected];
  if (!sel) return [];
  const ids = new Set([selected]);
  state.goods.forEach(p => {
    if ((p.recipe || []).some(s => s.good_id === selected)) ids.add(p.id);
  });
  (sel.recipe || []).forEach(s => { if (s.good_id) ids.add(s.good_id); });
  return state.goods.filter(g => ids.has(g.id));
}
