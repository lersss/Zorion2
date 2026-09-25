"use strict";
// ---------- дерево построек на канвасе (спека 2026-09-21 §2) ----------
// Уровни: Строения (корень) → Классы (фиксированные) → Типы (parent_id NULL)
// → Подтипы + узлы-приглашения категорий (автоматика). Раскладка — свой
// лейут (4 уровня-ряда), рёбра — орто (orthoEdge), pan/zoom — общие.

function raceFamilyOf(raceId) {
  if (!racesData) return "";
  const r = racesData.races.find(x => x.id === raceId);
  return r ? r.family : "";
}
function raceName(raceId) {
  if (!racesData) return raceId;
  const r = racesData.races.find(x => x.id === raceId);
  return r ? r.name : raceId;
}
function familyLabelById(fid) {
  if (!racesData) return fid;
  const f = racesData.families.find(x => x.id === fid);
  return f ? (f.id === "robotic" ? "F10 " + f.name : f.id + " " + f.name) : fid;
}

// Видимость записи при выбранном уровне расовости (спека 2026-09-21 §5.1,
// Р5 — единая применяемость): универсальный — race_family IS NULL; семейство
// Fk — NULL или (Fk и race IS NULL) — расовых записей семейство НЕ видит;
// раса R — NULL или (family(R) и race IS NULL) или race = R.
function prodVisible(p) {
  if (prodRaceLevel === "universal") return !p.race_family;
  if (prodRaceLevel === "family") return !p.race_family || (p.race_family === prodRaceFamily && !p.race);
  const fam = raceFamilyOf(prodRace);
  return !p.race_family || (p.race_family === fam && !p.race) || p.race === prodRace;
}

// Применяемые записи узла (тип t, категория catID — ЧИСЛО id категории) на
// текущем уровне расовости (спека дерева §2.4): подтипы с parent_id=t.id и
// category_id=catID, видимые по prodVisible.
function prodAppliedRecords(t, catID) {
  return state.producer_types.filter(s => s.parent_id === t.id && s.category_id === catID && prodVisible(s));
}

// prodRecordSpecificity — ступень записи-фабрики: раса > семейство > универсал.
function prodRecordSpecificity(p) {
  if (p.race) return 2;
  if (p.race_family) return 1;
  return 0;
}
// prodCatWinner(t, catID) — победитель лестницы «самая конкретная побеждает»
// (спека 2026-09-21 §5.2/§5.6): применяемая запись категории самой конкретной
// ступени; null — записей нет. Единственный источник правила для дерева,
// списка и prodNodeVisible.
function prodCatWinner(t, catID) {
  const recs = prodAppliedRecords(t, catID);
  if (!recs.length) return null;
  return recs.reduce((a, b) => prodRecordSpecificity(b) > prodRecordSpecificity(a) ? b : a);
}
// prodCatDrawRecords(t, catID) — применяемые записи категории, рисуемые на
// уровне (спека скрытых §2.2 п.2, вариант б): скрыто — только скрытые записи +
// победитель лестницы (затемнённые; видимые менее конкретные НЕ показываем);
// видимо — победитель + скрытые записи; лосеры-видимые не рисуются.
function prodCatDrawRecords(t, catID) {
  const recs = prodAppliedRecords(t, catID);
  const winner = prodCatWinner(t, catID);
  const out = [];
  if (winner) out.push({ref: winner, hidden: prodCatHidden(t, catID)});
  recs.forEach(s => {
    if (s.hidden && !(winner && winner.id === s.id)) out.push({ref: s, hidden: true});
  });
  return out;
}

// --- слоты родителя (спека 2026-09-21-скрытые-категории-строений §2) ---
// Слот = «родитель предлагает категорию на уровне расовости»; конфигурация
// типа kind=goods. Дерево строит категории ИЗ СЛОТОВ (автоматика по имени
// типа удалена, §2.3); скрытость слота — скрытость КАТЕГОРИИ на уровне
// (вторая ось к producer_types.hidden, 2026-09-21 §5.3).

// prodSlots(t) — все слоты типа t (конфигурация, без фильтра расовости).
function prodSlots(t) {
  return state.producer_slots.filter(s => s.parent_id === t.id);
}
// prodSlotVisible(s) — применим ли слот на текущем уровне расовости (§5.1,
// та же логика, что prodVisible у записей): универсальный — race_family IS
// NULL; семейство Fk — NULL или (Fk и race IS NULL) — расовых слотов
// семейство НЕ видит; раса R — NULL или (family(R), race NULL) или race=R.
function prodSlotVisible(s) {
  if (prodRaceLevel === "universal") return !s.race_family;
  if (prodRaceLevel === "family") return !s.race_family || (s.race_family === prodRaceFamily && !s.race);
  const fam = raceFamilyOf(prodRace);
  return !s.race_family || (s.race_family === fam && !s.race) || s.race === prodRace;
}
function prodSlotSpecificity(s) {
  if (s.race) return 2;
  if (s.race_family) return 1;
  return 0;
}
// prodAppliedSlots(t) — применяемые слоты типа на текущем уровне (§2.1).
function prodAppliedSlots(t) {
  return prodSlots(t).filter(prodSlotVisible);
}
// prodMostSpecificSlot(applied) — самая конкретная применяемая запись
// (race > family > universal); UNIQUE NULLS NOT DISTINCT гарантирует
// однозначность на уровне.
function prodMostSpecificSlot(applied) {
  return applied.reduce((a, b) => prodSlotSpecificity(b) > prodSlotSpecificity(a) ? b : a);
}
// prodSlotOwned(s) — слот принадлежит ТЕКУЩЕМУ уровню расовости (не наследуется).
function prodSlotOwned(s) {
  if (prodRaceLevel === "universal") return !s.race_family;
  if (prodRaceLevel === "family") return s.race_family === prodRaceFamily && !s.race;
  return s.race === prodRace;
}
// prodCatInSet(t, catID) — категория входит в набор родителя на уровне
// (есть применяемый слот). Вне набора категория не рисуется вовсе (§2.1).
function prodCatInSet(t, catID) {
  return prodAppliedSlots(t).some(s => s.category_id === catID);
}
// prodCatHidden(t, catID) — категория скрыта на уровне (спека §5.4): скрытость
// = OR двух осей — самая конкретная применяемая ЗАПИСЬ-фабрика
// (producer_types.hidden, Р1/К1) ИЛИ самый конкретный применяемый СЛОТ
// (producer_slots.hidden). Вне набора — false (отсутствует, не «скрыта»).
function prodCatHidden(t, catID) {
  const applied = prodAppliedSlots(t).filter(s => s.category_id === catID);
  const slotHidden = applied.length ? prodMostSpecificSlot(applied).hidden : false;
  const winner = prodCatWinner(t, catID);
  return slotHidden || !!(winner && winner.hidden);
}

// prodNodeVisible(p) — рисуется ли узел записи p в дереве при текущем уровне
// расовости и галке (спека §5.4/§5.6, «клик не повисает»): категория записи
// входит в набор родителя (слот-управляемый); скрыта — видна только с галкой
// (затемнённо); видима — в дереве только победитель лестницы и скрытые записи
// (лосеры-видимые не рисуются). Типы items/energy без слотов — без оси.
function prodNodeVisible(p) {
  if (!p.parent_id) return true; // типы не скрываются (§1.4 п.1)
  const t = state.producer_types.find(x => x.id === p.parent_id);
  if (!t || t.kind !== "goods") return true; // items/energy — как раньше
  // класс строения без товарной категории (тип без слотов, напр. «Поселение»):
  // своя ось — слотов нет, набор категорий к нему неприменим (§5.3 п.4)
  if (prodSlots(t).length === 0 && p.category_id == null) return true;
  if (!prodCatInSet(t, p.category_id)) return false; // категория вне набора уровня
  const catHidden = prodCatHidden(t, p.category_id);
  const winner = prodCatWinner(t, p.category_id);
  // скрыта: без галки ничего; с галкой — только скрытые записи + победитель
  // лестницы (вариант б, спека скрытых §2.2 п.2; видимые менее конкретные
  // не показываем). Расовость фильтрует набор через prodAppliedRecords.
  if (catHidden && !prodShowHidden) return false;
  return !!(winner && winner.id === p.id) || !!p.hidden;
}

// prodNodeId — id узла дерева для записи producer_types ("type:N"/"sub:N").
function prodNodeId(p) { return (p.parent_id ? "sub:" : "type:") + p.id; }
// prodNodeById — запись по id узла дерева (null для фиксированных/приглашений).
function prodNodeById(id) {
  const m = /^(type|sub):(\d+)$/.exec(id || "");
  if (!m) return null;
  return state.producer_types.find(p => p.id === Number(m[2]));
}

// ---------- разделы построек (спека 2026-09-25 §6.2) ----------
// Раздел вида постройки — свойство каталога (producer_types.section), а не
// сравнение имён: переименование корня привязку не ломает. Список разделов —
// данные интерфейса (без CHECK в БД); «Прочее» — виртуальный раздел (NULL/
// неизвестное значение), в БД не пишется. Порядок кнопок задан здесь.
const PROD_SECTIONS = [
  {key: "colony", id: "branchProdColony", label: "Колонии", title: "Колонии"},
  {key: "factory", id: "branchProdFactory", label: "Фабрики", title: "Фабрики и автофабрики"},
  {key: "lab", id: "branchProdLab", label: "Лаборатории", title: "Лаборатории"},
  {key: "mining", id: "branchProdMining", label: "Добывающие", title: "Добывающие платформы"},
  {key: "energy", id: "branchProdEnergy", label: "Энергостанции", title: "Энергостанции"},
];
const PROD_SECTION_KEYS = {colony: 1, factory: 1, lab: 1, mining: 1, energy: 1};

// prodSectionLabel — подпись раздела по ключу («Прочее» — служебный/наследие).
function prodSectionLabel(sec) {
  const s = PROD_SECTIONS.find(x => x.key === sec);
  return s ? s.label : "Прочее";
}
// prodSectionOf — раздел записи p: у корня — p.section из пяти (иначе "other");
// у подтипа — раздел его корня по parent_id (наследование, в БД не дублируется).
function prodSectionOf(p) {
  if (!p.parent_id) return PROD_SECTION_KEYS[p.section] ? p.section : "other";
  const root = (state.producer_types || []).find(x => x.id === p.parent_id);
  return root ? (PROD_SECTION_KEYS[root.section] ? root.section : "other") : "other";
}
// prodSectionRoots — типы-корни активного раздела, видимые по расовости (порядок
// — по заведению, id: дерево не пересортировывает корни).
function prodSectionRoots() {
  return (state.producer_types || []).filter(p => !p.parent_id && prodVisible(p) && prodSectionOf(p) === prodSection);
}
// renderBuildingsTabsUI — .on у активной кнопки раздела; «Прочее» видно, пока
// есть корень без раздела. Активен «Прочее», а корней без раздела не осталось —
// откат на «Колонии» (спека §7): только когда состояние загружено (иначе на
// старте до fetchState корней ещё нет и сохранённый «Прочее» обнулялся бы).
// Зовётся из applyBranchUI (в т.ч. до загрузки состояния) — читает producer_types
// защищённо.
function renderBuildingsTabsUI() {
  const roots = state.producer_types || [];
  const loaded = Array.isArray(state.producer_types);
  const hasOther = roots.some(p => !p.parent_id && prodSectionOf(p) === "other");
  if (loaded && prodSection === "other" && !hasOther) {
    prodSection = "colony";
    localStorage.setItem("gs_prodSection", prodSection);
  }
  // Подсветка активного раздела — только в ветке построек (спека §6.1/§12.1):
  // на «Товарах»/«Предметах» ряд виден как точка входа, но ни одна кнопка
  // раздела не активна (ветка построек ещё не выбрана).
  const inProducers = branch === "producers";
  PROD_SECTIONS.forEach(s => {
    const el = $(s.id);
    if (el) el.classList.toggle("on", inProducers && s.key === prodSection);
  });
  const other = $("branchProdOther");
  if (other) {
    other.style.display = hasOther ? "" : "none";
    other.classList.toggle("on", inProducers && prodSection === "other");
  }
}
// setProdSection — переключение раздела (кнопки шапки): ставит техническую ветку
// branch="producers", сбрасывает выбор/попап, восстанавливает общий zoom/pan
// группы построек и центрирует полотно (спека §6.6). «Прочее» — полноправный
// активный раздел (не хранится в БД, но кликается как остальные).
function setProdSection(key) {
  if (key !== "other" && !PROD_SECTION_KEYS[key]) key = "colony";
  prodSection = key;
  localStorage.setItem("gs_prodSection", key);
  branch = "producers";
  localStorage.setItem("gs_branch", "producers");
  $("branchGoods").classList.toggle("on", false);
  $("branchItems").classList.toggle("on", false);
  selected = null;
  popupGoodId = null;
  closePopup();
  zoom = parseFloat(localStorage.getItem("gs_prodZoom") || "1");
  panX = parseFloat(localStorage.getItem("gs_prodPanX") || "0");
  panY = parseFloat(localStorage.getItem("gs_prodPanY") || "0");
  renderAll();
  centerProdTree();
}
// saveProdSection — сохранение раздела типа-корня (карточка, паттерн
// saveStageBlock): PUT /studio/api/producers/{id} телом {section}; пусто —
// «— не задан (Прочее)» (сервер пишет NULL). Незнакомое/непустое у подтипа
// отвергает сервер (400) — карточка перерисовывается.
async function saveProdSection(val) {
  if (!popupGoodId) return;
  const r = await api("PUT", "/studio/api/producers/" + popupGoodId, {section: val || ""});
  if (r) fetchState(); else renderProdPopup();
}
// prodSectionBlock — блок «Раздел» карточки (спека §6.4): у типа-корня — селект
// из пяти + «— не задан (Прочее)»; у подтипа — только чтение (наследуется).
function prodSectionBlock(p) {
  if (!p.parent_id) {
    const opts = PROD_SECTIONS.map(s =>
      '<option value="' + s.key + '"' + (p.section === s.key ? ' selected' : '') + '>' + esc(s.title) + '</option>').join("");
    return '<div class="dmeta">раздел: <select class="dsection" onchange="saveProdSection(this.value)">' +
      '<option value=""' + (!p.section ? ' selected' : '') + '>— не задан (Прочее)</option>' + opts + '</select></div>';
  }
  return '<div class="dmeta">раздел: <b>' + esc(prodSectionLabel(prodSectionOf(p))) + '</b> (наследуется от корня)</div>';
}
// prodEmptyStateText — текст пустого состояния раздела (спека §6.5): нет корней
// / корни есть, но скрыты уровнем расовости / служебное «Прочее».
function prodEmptyStateText() {
  const inSection = (state.producer_types || []).filter(p => !p.parent_id && prodSectionOf(p) === prodSection);
  if (!inSection.length) {
    if (prodSection === "other") return "Корни без раздела. Откройте карточку типа и выберите раздел.";
    return "В разделе «" + prodSectionLabel(prodSection) + "» пока нет типов построек. Кнопка «+ тип» создаст тип.";
  }
  const hidden = inSection.find(p => !prodVisible(p));
  return "Тип «" + (hidden ? hidden.name : inSection[0].name) + "» скрыт на выбранном уровне расовости.";
}

// prodStageEnter — нижний порог населения ступени ладдеры (params.stage.enter);
// нет порога — Infinity (записи без ладдеры уходят в конец, порядок стабилен).
function prodStageEnter(p) {
  const v = Number((p.params && p.params.stage || {}).enter);
  return isFinite(v) ? v : Infinity;
}
// prodNextStage — следующая ступень ладдеры: сосед по родителю с минимальным
// enter, большим своего; null — следующей ступени нет. Связка блока «Стадия»
// (§11.1 п.3) — по записи, не по имени.
function prodNextStage(p) {
  const own = prodStageEnter(p);
  let next = null;
  state.producer_types.forEach(s => {
    if (s.parent_id !== p.parent_id) return;
    const v = prodStageEnter(s);
    if (!isFinite(v) || v <= own) return;
    if (next == null || v < prodStageEnter(next)) next = s;
  });
  return next;
}
// prodNextStageEnter — нижний порог следующей ступени ладдеры (enter записи
// prodNextStage); null — следующей ступени нет.
function prodNextStageEnter(p) {
  const s = prodNextStage(p);
  return s ? prodStageEnter(s) : null;
}
// prodSubtypeNoCatRecords — подтипы типа БЕЗ товарной категории (класс
// строения: «Колония» → «Форпост», итерация 4 §5.3 п.4): у типа
// нет слотов, значит товарной категории у подтипа нет по определению; ось —
// только prodVisible (класс строения у постройки есть всегда). Порядок — по
// нижнему порогу населения ступени (идея 2026-09-24, «само по данным»).
function prodSubtypeNoCatRecords(t) {
  return state.producer_types.filter(s =>
    s.parent_id === t.id && s.category_id == null && prodVisible(s))
    .sort((a, b) => prodStageEnter(a) - prodStageEnter(b));
}

// buildProdTree — дерево активного РАЗДЕЛА построек (спека 2026-09-25 §6.3):
// виртуальный корень → типы-корни раздела (prodSectionRoots: parent_id NULL и
// prodVisible) → их подтипы. Узлы «Строения» и фиксированные классы сняты —
// первый ряд это карточки корней (у «Фабрик» — два). Ветки отрисовки категорий
// из слотов и подтипов без товарной категории — дословно из прежней версии.
function buildProdTree() {
  const root = {id: "sectionRoot", kind: "sectionRoot", label: "", children: []};
  const subtypeNode = (s, isHidden) => ({id: prodNodeId(s), label: s.name, kind: "subtype", ref: s, children: [], hidden: isHidden});
  prodSectionRoots().forEach(t => {
    const node = {id: prodNodeId(t), label: t.name, kind: "type", ref: t, children: []};
    root.children.push(node);
    if (t.kind === "goods") {
      // Слоты родителя (спека §5.2–§5.4): категории = применяемые слоты на
      // текущем уровне; вне набора — категория не рисуется вовсе.
      const applied = prodAppliedSlots(t);
      const catIDs = [...new Set(applied.map(s => s.category_id))];
      catIDs.forEach(catID => {
        const cat = state.categories.find(c => c.id === catID);
        if (!cat) return;
        const recs = prodAppliedRecords(t, catID);
        if (prodCatHidden(t, catID)) {
          // state 2 «скрыто» (OR осей): без галки ничего; с галкой — только
          // скрытые записи + победитель лестницы (вариант б, спека скрытых
          // §2.2 п.2; видимые менее конкретные не показываем);
          // записей нет — заглушка (§5.4)
          if (!prodShowHidden) return;
          if (!recs.length) {
            node.children.push({id: "hstub:" + t.id + ":" + catID, label: cat.name, kind: "hiddenStub", ref: {type: t, catID}, children: []});
          } else {
            prodCatDrawRecords(t, catID).forEach(d => node.children.push(subtypeNode(d.ref, true)));
          }
        } else {
          // видимо: лестница — победитель + затемнённые скрытые записи;
          // записей нет — узел-приглашение «создать» (§5.4)
          const draw = prodCatDrawRecords(t, catID);
          if (!draw.length) {
            node.children.push({id: "invite:" + t.id + ":" + catID, label: cat.name, kind: "invite", ref: {type: t, cat}, children: []});
          } else {
            draw.forEach(d => node.children.push(subtypeNode(d.ref, d.hidden)));
          }
        }
      });
      // тип без слотов: подтипы-классы строения без товарной категории (§5.3 п.4)
      if (prodSlots(t).length === 0) {
        prodSubtypeNoCatRecords(t).forEach(s => node.children.push(subtypeNode(s, false)));
      }
      // записи подтипов с категорией ВНЕ набора уровня не рисуются (§5.2)
    } else {
      // Типы items/energy: подтипы как раньше (по prodVisible), без лестницы
      // и оси скрытости (у producer_types.hidden — только подтипы kind=goods).
      state.producer_types.filter(s => s.parent_id === t.id && prodVisible(s)).forEach(s => {
        node.children.push(subtypeNode(s, false));
      });
    }
  });
  return root;
}

// prodCardHeight — высота карточки дерева построек от содержимого: базовая
// CARD_H, если строк «производит» нет; иначе растёт под число строк списка
// (шрифт 11px Arial, LINE_H). Ширина карточки фиксированная (CARD_W) — ряды
// дерева не разъезжаются, решение создателя 2026-09-23.
function prodCardHeight(n) {
  const produces = (n.ref && n.ref.parent_id) ? prodProducesText(n.ref) : "";
  if (!produces) return CARD_H;
  ctx.font = "11px Arial";
  const lines = wrapPx(produces, CARD_W - 16).length;
  return Math.max(CARD_H, 68 + (lines - 1) * LINE_H + 8);
}

// prodTreeLayout — раскладка дерева раздела (спека 2026-09-25 §6.3): первый ряд
// — карточки корней раздела (детей виртуального корня «sectionRoot»); дальше
// уровни как раньше. Ширина поддерева = сумма детей, родитель центрируется над
// детьми; высота ряда = максимум prodCardHeight карточек ряда.
function prodTreeLayout() {
  const root = buildProdTree();
  const tops = root.kind === "sectionRoot" ? root.children : [root];
  const cols = {};
  function colsOf(n) {
    if (cols[n.id] !== undefined) return cols[n.id];
    if (!n.children.length) { cols[n.id] = 1; return 1; }
    let s = 0;
    n.children.forEach(c => s += colsOf(c));
    cols[n.id] = s;
    return s;
  }
  // высота каждого ряда = максимум высот карточек этого ряда
  const rowH = [];
  function measure(n, depth) {
    rowH[depth] = Math.max(rowH[depth] || 0, prodCardHeight(n));
    n.children.forEach(c => measure(c, depth + 1));
  }
  tops.forEach(n => measure(n, 0));
  const rowY = [PAD];
  for (let d = 1; d < rowH.length; d++) rowY[d] = rowY[d - 1] + rowH[d - 1] + ROW_GAP;
  const pos = {}, h = {};
  let maxCols = 1;
  function place(n, x, depth) {
    const blockW = colsOf(n) * (CARD_W + GAP_X) - GAP_X;
    pos[n.id] = {x: x + (blockW - CARD_W) / 2, y: rowY[depth]};
    h[n.id] = prodCardHeight(n);
    maxCols = Math.max(maxCols, x + blockW + GAP_X);
    let cx = x;
    n.children.forEach(c => { place(c, cx, depth + 1); cx += colsOf(c) * (CARD_W + GAP_X); });
  }
  let x = PAD;
  tops.forEach(n => { place(n, x, 0); x += colsOf(n) * (CARD_W + GAP_X); });
  let treeH = 0;
  rowH.forEach((rh, d) => { treeH += rh + (d ? ROW_GAP : 0); });
  return {root, pos, maxCols, h, treeH};
}

// renderProdTree — дерево построек на канвасе (спека §2.5): карточки уровней,
// орто-рёбра, узлы-приглашения пунктиром, клик = выбор, двойной клик = попап.
function renderProdTree() {
  const dpr = window.devicePixelRatio || 1;
  const cssW = $("canvasWrap").clientWidth, cssH = $("canvasWrap").clientHeight;
  canvas.width = Math.max(1, Math.round(cssW * dpr));
  canvas.height = Math.max(1, Math.round(cssH * dpr));
  ctx.setTransform(dpr * zoom, 0, 0, dpr * zoom, dpr * panX, dpr * panY);
  ctx.clearRect(-panX/zoom, -panY/zoom, cssW/zoom, cssH/zoom);
  const L = prodTreeLayout();
  // пустой раздел: текстом по центру (спека §6.5) — нет корней / скрыты расовостью
  if (!Object.keys(L.pos).length) {
    ctx.fillStyle = "#aaa";
    ctx.font = "14px Arial";
    ctx.textAlign = "center";
    const cx = (cssW / 2 - panX) / zoom, cy = (cssH / 2 - panY) / zoom;
    const lines = wrapPx(prodEmptyStateText(), Math.max(140, (cssW - 80) / zoom));
    lines.forEach((ln, i) => ctx.fillText(ln, cx, cy + (i - (lines.length - 1) / 2) * LINE_H));
    ctx.textAlign = "start";
    return;
  }
  // рёбра (орто): у виртуального корня (sectionRoot) позиции нет — карточки
  // корней стоят первым рядом без входящих рёбер.
  ctx.strokeStyle = "#555";
  ctx.lineWidth = 1.5;
  (function edges(n) {
    n.children.forEach(c => {
      if (L.pos[n.id] && L.pos[c.id]) orthoEdge(L.pos[n.id], L.pos[c.id], L.h[n.id]);
      edges(c);
    });
  })(L.root);
  // карточки
  (function draw(n) {
    const p = L.pos[n.id];
    if (!p) { n.children.forEach(draw); return; }
    const x = p.x, y = p.y;
    const isInvite = n.kind === "invite";
    const isFixed = n.kind === "root" || n.kind === "class";
    const sel = selected === n.id;
    if (isInvite) {
      ctx.setLineDash([5, 4]);
      ctx.fillStyle = "#1a1a1a";
      ctx.strokeStyle = "#777";
      ctx.lineWidth = 1.5;
      roundRect(x, y, CARD_W, CARD_H, 8);
      ctx.fill(); ctx.stroke();
      ctx.setLineDash([]);
      ctx.fillStyle = "#999";
      ctx.font = "14px Arial";
      ctx.fillText("создать", x + 8, y + 18);
      ctx.font = "12px Arial";
      ctx.fillText(truncPx(n.label, CARD_W - 16, 24), x + 8, y + 32);
    } else if (n.kind === "hiddenStub") {
      // Карточка-заглушка скрытой пустой категории (спека скрытых §2.2 п.2):
      // сплошная затемнённая карточка, подпись «нет заводов», бейдж «скрыта»;
      // клик/двойной клик открывает карточку родителя (§3.2, путь «открыть»).
      ctx.fillStyle = "#3a3a3a";
      ctx.strokeStyle = sel ? "#8cf" : "#4a4a4a";
      ctx.lineWidth = sel ? 2 : 1;
      roundRect(x, y, CARD_W, CARD_H, 8);
      ctx.fill(); ctx.stroke();
      ctx.fillStyle = "#ddd";
      ctx.font = "14px Arial";
      ctx.fillText(truncPx(n.label, CARD_W - 16, 24), x + 8, y + 18);
      ctx.fillStyle = "#888";
      ctx.font = "12px Arial";
      ctx.fillText("нет заводов", x + 8, y + 32);
      ctx.font = "11px Arial";
      const b = "скрыта";
      const w = ctx.measureText(b).width + 10;
      ctx.fillStyle = "#333";
      roundRect(x + 8, y + 40, w, 18, 4); ctx.fill();
      ctx.fillStyle = "#aaa";
      ctx.fillText(b, x + 13, y + 53);
    } else {
      const isHidden = !!n.hidden;
      const isDropHere = prodDropHighlight === n.id; // карточка-подтип под курсором при drag (§3.1)
      // Скрытая запись (спека скрытых §3.4): затемнение ЦВЕТАМИ (fill/border/
      // имя/подпись), не globalAlpha — выделение #8cf остаётся ярким.
      ctx.fillStyle = isDropHere ? "#1e2a3a" : isFixed ? "#2a2a2a" : isHidden ? "#3a3a3a" : "#3d3d3d";
      ctx.strokeStyle = (isDropHere || sel) ? "#8cf" : (isFixed ? "#666" : isHidden ? "#4a4a4a" : "#5a5a5a");
      ctx.lineWidth = isDropHere ? 3 : sel ? 2 : 1;
      roundRect(x, y, CARD_W, L.h[n.id], 8);
      ctx.fill(); ctx.stroke();
      ctx.fillStyle = isHidden ? "#ddd" : "#fff";
      ctx.font = "14px Arial";
      ctx.fillText(truncPx(n.label, CARD_W - 16, 24), x + 8, y + 18);
      ctx.fillStyle = isHidden ? "#888" : "#bbb";
      ctx.font = "12px Arial";
      const sub = n.kind === "type" ? kindLabel(n.ref.kind)
        : n.kind === "subtype" ? (n.ref.category_name || kindLabel(n.ref.kind))
        : (n.id === "class-other" ? "задел" : "фиксированный класс");
      ctx.fillText(truncPx(sub, CARD_W - 16, 22), x + 8, y + 32);
      // бейдж «скрыта» — ряд на y+40 (высота 18, текст y+53)
      let bx = x + 8;
      if (isHidden) {
        ctx.font = "11px Arial";
        const b = "скрыта";
        const w = ctx.measureText(b).width + 10;
        ctx.fillStyle = "#333";
        roundRect(bx, y + 40, w, 18, 4); ctx.fill();
        ctx.fillStyle = "#aaa";
        ctx.fillText(b, bx + 5, y + 53);
      }
      // 3-я строка — выводы назначенных рецептов (идея 2026-09-23, шаг 2 §3.2):
      // только у подтипа; пустой набор — строку не рисуем (карточка не меняется)
      if (n.ref && n.ref.parent_id) {
        const produces = prodProducesText(n.ref);
        if (produces) {
          ctx.fillStyle = isHidden ? "#7a7" : "#9c9";
          ctx.font = "11px Arial";
          wrapPx(produces, CARD_W - 16).forEach((ln, i) => ctx.fillText(ln, x + 8, y + 68 + i * LINE_H));
        }
      }
    }
    n.children.forEach(draw);
  })(L.root);
}

// prodTreeHitTest — карточка дерева под точкой (мировые координаты).
function prodTreeHitTest(mx, my) {
  const L = prodTreeLayout();
  for (const id in L.pos) {
    const p = L.pos[id];
    if (mx >= p.x && mx <= p.x + CARD_W && my >= p.y && my <= p.y + (L.h[id] || CARD_H)) return {type: "card", id: id};
  }
  return null;
}

// openProdNodePopup — попап по узлу дерева (спека §2.5): тип/подтип — деталь
// (renderProdPopup); узел-приглашение — попап создания; заглушка скрытой
// категории — карточка родителя. Узлы «Строения»/классы сняты (спека 2026-09-25
// §6.3) — неизвестный id получает безопасный fallback.
function openProdNodePopup(id) {
  const n = prodNodeById(id);
  if (n) { openProdPopup(n.id); return; }
  if (id && id.indexOf("invite:") === 0) { openProdCreatePopup(id); return; }
  // Заглушка скрытой пустой категории (спека скрытых §2.2 п.2): клик →
  // карточка родителя (путь «открыть» скрытую категорию, §3.2).
  if (id && id.indexOf("hstub:") === 0) {
    const parts = id.split(":");
    const t = state.producer_types.find(x => x.id === Number(parts[1]));
    if (t) {
      selected = prodNodeId(t);
      openProdPopup(t.id);
      renderAll();
      centerOn(selected);
    }
    return;
  }
  // Служебный fallback: неизвестный узел (после снятия «Строения»/классов
  // недостижим) — безопасная справка, не редактируется.
  openModal(
    '<h3>Узел дерева</h3>' +
    '<div>Узел не редактируется.</div>',
    [{label: "Ок", cls: "", action: closeModal}]
  );
}

// openProdCreatePopup — попап создания подтипа из узла-приглашения (спека
// §2.3): name = «<Родитель> <категория>», kind/category_id/parent_id — из
// узла, race_family/race — текущий уровень расовости. POST → draft.
function openProdCreatePopup(id) {
  const m = /^invite:(\d+):(\d+)$/.exec(id || "");
  if (!m) return;
  const type = state.producer_types.find(p => p.id === Number(m[1]));
  const cat = state.categories.find(c => c.id === Number(m[2]));
  if (!type || !cat) return;
  const raceInfo = prodRaceLevel === "universal" ? ""
    : prodRaceLevel === "family" ? familyLabelById(prodRaceFamily) : raceName(prodRace);
  // чекбокс копирования набора рецептов универсальной фабрики категории
  // (ТЗ §13.5): показывается только если есть что копировать (K > 0)
  const srcs = universalSources(cat.id, null);
  const srcIds = new Set();
  srcs.forEach(s => factoryRecipeIds(s.id).forEach(rid => srcIds.add(rid)));
  const copyLine = srcIds.size > 0
    ? '<label style="display:block;margin-top:8px;font-size:12px"><input type="checkbox" id="prodCreateCopy"> скопировать набор рецептов универсальной фабрики категории «' + esc(cat.name) + '» (' + srcIds.size + ' рецептов)</label>'
    : '';
  openModal(
    '<h3>Создать подтип</h3>' +
    '<div>Родитель: <b>' + esc(type.name) + '</b> · категория: <b>' + esc(cat.name) + '</b>' +
    (raceInfo ? ' · расовость: <b>' + esc(raceInfo) + '</b>' : '') + '</div>' +
    '<div style="margin-top:8px">Имя: <input id="prodCreateName" type="text" name="studio_new_subtype_name" value="' + esc(type.name + " " + cat.name) + '" style="width:100%;box-sizing:border-box" autocomplete="off"></div>' +
    copyLine,
    [
      {label: "Создать", cls: "gen", primary: true, action: () => doProdCreate(type, cat)},
      {label: "Отмена", cls: "", action: closeModal}
    ]
  );
  const inp = $("prodCreateName");
  inp.focus(); inp.select();
  // Enter — глобальный обработчик модалки (клик по primary-кнопке «Создать»).
}
async function doProdCreate(type, cat) {
  const name = $("prodCreateName").value.trim();
  if (!name) return;
  // чекбокс читаем ДО closeModal (элемент остаётся в DOM, но читать яснее сразу)
  const copyChk = $("prodCreateCopy");
  const doCopy = !!(copyChk && copyChk.checked);
  closeModal();
  const body = {name, kind: type.kind, category_id: cat.id, parent_id: type.id};
  if (prodRaceLevel === "family") body.race_family = prodRaceFamily;
  if (prodRaceLevel === "race") { body.race_family = raceFamilyOf(prodRace); body.race = prodRace; }
  const r = await api("POST", "/studio/api/producers", body);
  if (!r) return;
  const p = await r.json();
  selected = prodNodeId(p);
  // копирование набора рецептов — только в СУЩЕСТВУЮЩУЮ фабрику (ТЗ §13.5)
  if (doCopy) {
    const rc = await api("POST", "/studio/api/producers/" + p.id + "/recipes/copy-universal");
    if (rc) {
      const out = await rc.json();
      const added = (out && out.added) || 0, skipped = (out && out.skipped) || 0;
      if (added === 0 && skipped === 0) showReport(["копировать нечего"]);
      else showReport(["добавлено " + added + ", пропущено " + skipped]);
    }
  }
  fetchState();
}

// --- переключатель уровня расовости (спека §5) ---
// Универсальный / Семейство (F0–F10) / Раса (60, сгруппированы по семействам).
// Выбор расы фиксирует её семейство (каскад), выбор семейства сбрасывает расу.
function setProdRaceLevel(v) {
  prodRaceLevel = v;
  localStorage.setItem("gs_prodRaceLevel", v);
  if (v === "universal") { prodRaceFamily = ""; prodRace = ""; }
  if (v === "family") prodRace = "";
  localStorage.setItem("gs_prodRaceFamily", prodRaceFamily);
  localStorage.setItem("gs_prodRace", prodRace);
  renderProdRaceUI();
  renderAll();
}
function setProdRaceFamily(v) {
  prodRaceFamily = v;
  prodRace = ""; // выбор семейства сбрасывает расу (спека §5)
  localStorage.setItem("gs_prodRaceFamily", v);
  localStorage.setItem("gs_prodRace", "");
  renderProdRaceUI();
  renderAll();
}
function setProdRace(v) {
  prodRace = v;
  if (v) {
    const fam = raceFamilyOf(v);
    if (fam) { prodRaceFamily = fam; localStorage.setItem("gs_prodRaceFamily", fam); }
  }
  localStorage.setItem("gs_prodRace", v);
  renderProdRaceUI();
  renderAll();
}
function renderProdRaceUI() {
  const levelSel = $("prodRaceLevelSel");
  const famSel = $("prodRaceFamilySel");
  const raceSel = $("prodRaceSel");
  if (!levelSel) return;
  levelSel.value = prodRaceLevel;
  famSel.innerHTML = (racesData ? racesData.families : []).map(f =>
    '<option value="' + f.id + '"' + (f.id === prodRaceFamily ? ' selected' : '') + '>' + esc(familyLabelById(f.id)) + '</option>').join("");
  raceSel.innerHTML = prodRaceOptions(prodRace);
  famSel.style.display = prodRaceLevel === "universal" ? "none" : "";
  raceSel.style.display = prodRaceLevel === "race" ? "" : "none";
}
function prodRaceOptions(sel) {
  if (!racesData) return '<option value="">—</option>';
  const byFam = {};
  racesData.races.forEach(r => { (byFam[r.family] = byFam[r.family] || []).push(r); });
  let html = '<option value="">—</option>';
  racesData.families.forEach(f => {
    const list = byFam[f.id] || [];
    if (!list.length) return;
    html += '<optgroup label="' + esc(familyLabelById(f.id)) + '">' +
      list.map(r => '<option value="' + r.id + '"' + (r.id === sel ? ' selected' : '') + '>' + esc(r.name) + '</option>').join("") +
      '</optgroup>';
  });
  return html;
}
async function fetchRaces() {
  try {
    const r = await studioFetch("/studio/api/races");
    if (!r.ok) return;
    racesData = await r.json();
    racesLoaded = true;
    renderProdRaceUI();
    renderAll();
  } catch (e) { /* 401 — оверлей уже показан (studioFetch) */ }
}

// showProdTip — тултип узла дерева построек.
function showProdTip(ev, id) {
  const n = prodNodeById(id);
  const tip = $("tip");
  if (n) {
    const t = n.parent_id ? state.producer_types.find(x => x.id === n.parent_id) : null;
    const catHidden = !!(t && t.kind === "goods" && prodCatHidden(t, n.category_id));
    tip.innerHTML = '<b>' + esc(n.name) + '</b><br>' + esc(kindLabel(n.kind)) +
      (n.category_name ? ' · ' + esc(n.category_name) : '') + (n.parent_name ? ' · ← ' + esc(n.parent_name) : '') +
      (catHidden ? '<br><span style="color:#aaa">скрыта</span>' : '');
  } else if (id === "root") {
    tip.innerHTML = '<b>Строения</b><br>корень дерева построек';
  } else if (id === "class-other") {
    tip.innerHTML = '<b>Другие</b><br>задел — типы появятся позже';
  } else if (id && id.indexOf("class-") === 0) {
    tip.innerHTML = '<b>' + esc(id === "class-goods" ? "Производители товаров" : id === "class-items" ? "Производители предметов" : "Производители энергии") + '</b><br>фиксированный класс';
  } else if (id && id.indexOf("invite:") === 0) {
    tip.innerHTML = '<b>создать</b><br>подтип не заведён — клик создаст';
  } else if (id && id.indexOf("hstub:") === 0) {
    const parts = id.split(":");
    const cat = state.categories.find(c => c.id === Number(parts[2]));
    tip.innerHTML = '<b>' + esc(cat ? cat.name : "категория") + '</b><br>скрытая категория · нет заводов — клик откроет карточку родителя';
  } else {
    hideTip(); return;
  }
  const rect = canvas.getBoundingClientRect();
  let x = ev.clientX - rect.left + 14, y = ev.clientY - rect.top + 14;
  if (x + 220 > rect.width) x = ev.clientX - rect.left - 230;
  if (y + 60 > rect.height) y = ev.clientY - rect.top - 60;
  tip.style.left = x + "px"; tip.style.top = y + "px";
  tip.style.display = "block";
}
