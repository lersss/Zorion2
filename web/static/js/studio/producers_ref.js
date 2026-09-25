"use strict";
// ---------- справочник производителей (спека 2026-09-20-фабрики §4.1) ----------
// Список — вспомогательный (спека 2026-09-21 §2.5): клик по карточке выбирает
// узел в дереве на канвасе.
function renderProdSprav() {
  const sc = $("spravList").scrollTop;
  const q = $("spravSearch").value.trim().toLowerCase();
  // Фильтр по правилу раздела+видимости+скрытости дерева (спека скрытых §3.5 +
  // разделы построек 2026-09-25 §6.3): раздел (тип и его подтипы по prodSectionOf)
  // + расовость (prodVisible) + скрытость КАТЕГОРИИ-СЛОТА (галка, catHidden) +
  // узел в дереве («клик не повисает», prodNodeVisible) + поиск (AND).
  const list = state.producer_types.filter(p => {
    if (prodSectionOf(p) !== prodSection) return false;          // раздел (п.0)
    if (!prodVisible(p)) return false;                          // расовость (п.1)
    const t = p.parent_id ? state.producer_types.find(x => x.id === p.parent_id) : null;
    const catHidden = !!(t && t.kind === "goods" && prodCatHidden(t, p.category_id));
    if (catHidden && !prodShowHidden) return false;             // скрытость категории (п.2)
    if (!prodNodeVisible(p)) return false;                      // «клик не повисает» (п.3)
    if (q && !p.name.toLowerCase().includes(q)) return false;   // поиск (п.4)
    return true;
  });
  $("spravTitle").textContent = "Постройки · " + list.length;
  $("spravList").innerHTML = list.map(p => {
    const t = p.parent_id ? state.producer_types.find(x => x.id === p.parent_id) : null;
    const catHidden = !!(t && t.kind === "goods" && prodCatHidden(t, p.category_id));
    const produces = p.parent_id ? prodProducesText(p) : ""; // выводы набора (шаг 2 §3.2)
    return '<div class="card' + ((p.hidden || catHidden) ? ' hidden' : '') + '" onclick="prodClick(' + p.id + ')" ondblclick="prodDblClick(' + p.id + ')">' +
    '<div class="nm">' + esc(p.name) + '<button class="info" onclick="event.stopPropagation();openProdPopup(' + p.id + ')" title="деталь (без смены выбора)">ⓘ</button></div>' +
    '<div class="meta">' + esc(kindLabel(p.kind)) + (p.category_name ? ' · ' + esc(p.category_name) : '') +
    (p.parent_name ? ' · ← ' + esc(p.parent_name) : '') +
    (produces ? ' · производит: ' + esc(produces) : '') +
    (p.hidden ? ' <span class="badge b-hidden" title="запись-фабрика скрыта">скрыта</span>' : '') +
    (catHidden ? ' <span class="badge b-hidden" title="категория скрыта на этом уровне расовости">скрыта</span>' : '') + '</div>' +
    '</div>';
  }).join("") || '<div class="empty-note">нет типов построек</div>';
  $("spravList").scrollTop = sc;
}
function prodClick(id) {
  const p = state.producer_types.find(x => x.id === id);
  if (!p) return;
  selected = prodNodeId(p); // выбор узла в дереве (спека §2.5)
  renderAll();
  centerOn(selected);
}
function prodDblClick(id) {
  const p = state.producer_types.find(x => x.id === id);
  if (!p) return;
  selected = prodNodeId(p);
  openProdPopup(id);
  renderAll();
}
function openProdPopup(id) {
  popupGoodId = id;
  // модель «Потребляет» пересобирается под новый тип (спека 2026-09-23 §11.1)
  prodEatRows = null;
  prodEatRowsFor = null;
  prodStageEdit = null;
  prodStageEditFor = null;
  effectTypesFailed = false; // повторная попытка загрузить справочник эффектов
  resetPopupPos();
  $("detailPopup").style.display = "flex";
  $("detailOverlay").style.display = "block";
  renderProdPopup();
}
// prodSpecRows — источники строк карточки постройки, разложенные по блокам
// (спека 2026-09-23 §4): {conditions, people}. Каждая строка — [подпись,
// значение] (пустая подпись → значение без подписи). Пусто → блок не
// показываем. Блок «Потребляет» с итерации И1 — поля (eatBlock, §11.1 п.1),
// поэтому в consumes его больше нет. Источник — JSONB-поля input/params.
function prodSpecRows(p) {
  const inp = p.input || {}, par = p.params || {};
  // Условия работы — нужно (energy/fuel/robots), КПД, цена робота
  const conditions = [];
  const need = [];
  if (inp.energy) need.push("электричество");
  if (inp.fuel) need.push("топливо");
  if (inp.robots) need.push("роботы");
  if (need.length) conditions.push(["Нужно", need.join(", ")]);
  if (par.efficiency != null) conditions.push(["КПД", String(par.efficiency)]);
  if (par.robot_cost != null) conditions.push(["цена робота", String(par.robot_cost)]);
  // Люди — вместимость
  const people = [];
  if (inp.people && inp.people.capacity != null) people.push(["Вмещает людей", String(inp.people.capacity)]);
  return {conditions, people};
}
// specRow — строка-характеристика блока (подпись + значение).
function specRow(r) {
  return r[0]
    ? '<div class="prow"><span class="plabel">' + esc(r[0]) + ':</span> <span class="pval">' + esc(r[1]) + '</span></div>'
    : '<div class="prow"><span class="pval">' + esc(r[1]) + '</span></div>';
}
// pblockHtml — блок-карточка с заголовком (пустой rowsHtml → "").
function pblockHtml(title, rows) {
  if (!rows.length) return "";
  return '<div class="pblock"><div class="pblock-h">' + esc(title) + '</div>' + rows.map(specRow).join("") + '</div>';
}
function renderProdPopup() {
  const el = $("popupBody");
  if (!popupGoodId) return;
  const p = state.producer_types.find(x => x.id === popupGoodId);
  if (!p) { closePopup(); return; }
  fetchEffectTypes(); // селект «эффект при нехватке» (кэш, §11.1 п.1)
  $("popupTitle").textContent = p.name;
  let html = '';
  html += '<div class="dname"><span class="rename" onclick="startProdRename()" title="переименовать">' + esc(p.name) + '</span></div>';
  html += unitScaleSegmentHtml(); // переключатель масштаба единицы темпа (§2.1)
  // Шапка-блок (спека §4.1): вид/скрытая, родитель, категория, семейство/раса.
  // Подпись вида («товары») — только у записи с товарной категорией.
  html += '<div class="pblock">';
  html += '<div class="dmeta" style="margin-bottom:0">' + (p.category_id != null ? esc(kindLabel(p.kind)) + ' · ' : '');
  // Чекбокс «скрытая» — только у конкретной записи-фабрики (hidden — носитель
  // скрытия записи kind=goods с товарной категорией; у типов/items оси скрытия
  // нет). id — контракт e2e (studio-hidden-categories-check).
  if (p.parent_id && p.kind === "goods" && p.category_id != null) {
    html += '<label style="font-size:13px" title="Не показывать запись-фабрику по умолчанию и не предлагать в лестнице"><input type="checkbox" id="prodHiddenChk" onchange="saveProdHidden(this.checked)"' + (p.hidden ? ' checked' : '') + '> скрытая</label>';
  }
  html += '</div>';
  // родитель (дерево построек 2026-09-21 §1.2): подтип → тип-родитель
  if (p.parent_id) {
    const parent = state.producer_types.find(x => x.id === p.parent_id);
    html += '<div class="dmeta">родитель: <b>' + esc(parent ? parent.name : p.parent_id) + '</b></div>';
  }
  // категория — только у подтипов kind=goods (спека §1.2 п.4): тип абстрактен;
  // select — категории с применяемым слотом родителя на уровне записи
  // (инвариант С4, спека скрытых §3.4/§1.4 п.1)
  if (p.kind === "goods" && p.parent_id) {
    const parent = state.producer_types.find(x => x.id === p.parent_id);
    if (!parent || prodSlots(parent).length === 0) {
      // класс строения без товарной категории (тип-родитель слотов не имеет,
      // напр. «Поселение»): выбирать категорию товаров не из чего (§5.3 п.4)
      html += '<div class="dmeta">класс строения — без товарной категории (у типа-родителя нет слотов)</div>';
    } else {
      html += '<div class="dmeta">категория товаров: <select class="dcat" onchange="changeProdCategory(this.value)">' +
        prodCategoryOptions(p) +
        '</select></div>';
    }
  }
  // Раздел вида постройки (спека 2026-09-25 §6.4): селект у типа-корня,
  // read-only у подтипа (наследуется от корня).
  html += prodSectionBlock(p);
  // семейство + раса (спека §5): каскад семейство → расы семейства; запись —
  // через PUT {race_family, race} (соответствие каталогу проверяет сервер)
  html += '<div class="dmeta" style="margin-bottom:0">семейство рас: <select id="popupRaceFamilySel" onchange="popupRaceFamilyChanged()">' +
    '<option value="">универсальный</option>' + (racesData ? racesData.families.map(f =>
      '<option value="' + f.id + '"' + (f.id === p.race_family ? ' selected' : '') + '>' + esc(familyLabelById(f.id)) + '</option>').join("") : '') +
    '</select> · раса: <select id="popupRaceSel" onchange="popupRaceChanged()">' + prodRaceOptions(p.race) + '</select></div>';
  html += codeNote(p.code); // метка переноса — справочно, только чтение (§3.3)
  html += '</div>';
  // Производит — у ЛЮБОГО подтипа (спека §4.2): выводы назначенных рецептов
  // (+ «− отвязать») и выходы output; пусто у подтипа — подсказка про
  // перетаскивание. У типа без выходов блока нет (рецепт туда не назначить).
  if (p.parent_id || prodHasOutputs(p)) html += recipesBlock(p);
  // Блоки характеристик (спека §4.3–§4.5): Условия работы → Люди.
  // Пустые блоки не показываем (pblockHtml вернёт "").
  const spec = prodSpecRows(p);
  // «Потребляет» — поля (спека 2026-09-23 §11.1 п.1) и проекция арифметики
  // «на млрд» (§8.1) — только у записи-подтипа (у типа конфигурации нет).
  if (p.parent_id) html += eatBlock(p);
  html += stageBlock(p); // блок «Стадия» — только у ступени класса без слотов (§11.1 п.3)
  html += prodArithmeticBlock(p);
  html += pblockHtml("Условия работы", spec.conditions);
  html += pblockHtml("Люди", spec.people);
  // Слоты категорий — редактор родителя (спека §4.6): только тип kind=goods
  // (items/energy — без секции, §1.4 п.3); «+ подтип» — для класса без слотов
  if (p.kind === "goods" && !p.parent_id) {
    html += prodSlotEditor(p);
    if (prodSlots(p).length === 0) {
      html += '<div class="btns"><button class="gen" onclick="openProdSubtypeCreatePopup(' + p.id + ')">+ подтип</button></div>';
    }
  }
  // Производит предметы — только kind=items (спека §4.7)
  if (p.kind === "items") {
    html += '<div class="pblock"><div class="pblock-h">Производит предметы</div>';
    (p.items || []).forEach(it => {
      html += '<div class="usedin-item" style="cursor:default">' + esc(it.name) + ' <span style="color:#777">· ' + esc(it.slot_type) + '</span>' +
        '<button class="del" style="float:right" onclick="unlinkItem(' + it.id + ')">отвязать</button></div>';
    });
    const free = state.items.filter(it => !(p.items || []).some(x => x.id === it.id));
    html += '<div class="addrow" style="margin-top:6px"><select id="prodLinkItemSel">' +
      (free.length ? free.map(it => '<option value="' + it.id + '">' + esc(it.name) + '</option>').join("") : '<option value="">нет свободных</option>') +
      '</select><button class="gen" onclick="linkItem()"' + (free.length ? '' : ' disabled') + '>+ предмет</button></div>';
    html += '</div>';
  }
  // Служебные параметры (JSON) — свёрнуто (спека §4.8); id-шники и saveProdJSON
  // сохранены для итерации 2.
  html += '<details class="prod-json"><summary>служебные параметры (JSON)</summary>' +
    '<div class="dmeta">вход (input): <textarea id="prodInput" rows="3">' + esc(jsonText(p.input)) + '</textarea></div>' +
    '<div class="dmeta">выход (output): <textarea id="prodOutput" rows="2">' + esc(jsonText(p.output)) + '</textarea></div>' +
    '<div class="dmeta">параметры (params): <textarea id="prodParams" rows="2">' + esc(jsonText(p.params)) + '</textarea></div>' +
    '<div class="btns"><button class="gen" onclick="saveProdJSON()">сохранить JSON</button></div>' +
    '</details>';
  // Кнопки (спека §4.9)
  html += '<div class="btns">';
  html += '<button class="del" onclick="askDeleteProducer()">удалить</button>';
  html += '</div>';
  el.innerHTML = html;
}

// openProdSubtypeCreatePopup — попап создания подтипа у типа БЕЗ слотов
// (класс строения: «Поселение» → «Обычное поселение», итерация 4 §5.3 п.4):
// POST без товарной категории (у такого подтипа её нет по определению).
// Нормы еды задаются JSON-редактором «параметры (params)» в карточке.
function openProdSubtypeCreatePopup(typeId) {
  const type = state.producer_types.find(p => p.id === typeId);
  if (!type) return;
  const raceInfo = prodRaceLevel === "universal" ? ""
    : prodRaceLevel === "family" ? familyLabelById(prodRaceFamily) : raceName(prodRace);
  openModal(
    '<h3>Создать подтип</h3>' +
    '<div>Класс строения: <b>' + esc(type.name) + '</b>' +
    (raceInfo ? ' · расовость: <b>' + esc(raceInfo) + '</b>' : '') + '</div>' +
    '<div style="margin-top:8px">Имя: <input id="prodCreateName" type="text" name="studio_new_subtype_name" value="' + esc(type.name + " (подтип)") + '" style="width:100%;box-sizing:border-box" autocomplete="off"></div>' +
    '<div class="hint">У типа нет слотов — подтип создаётся без товарной категории. Нормы задаются в «параметрах (params)».</div>',
    [
      {label: "Создать", cls: "gen", primary: true, action: () => doProdSubtypeCreate(type)},
      {label: "Отмена", cls: "", action: closeModal}
    ]
  );
  const inp = $("prodCreateName");
  inp.focus(); inp.select();
}
async function doProdSubtypeCreate(type) {
  const name = $("prodCreateName").value.trim();
  if (!name) return;
  closeModal();
  const body = {name, kind: type.kind, parent_id: type.id};
  if (prodRaceLevel === "family") body.race_family = prodRaceFamily;
  if (prodRaceLevel === "race") { body.race_family = raceFamilyOf(prodRace); body.race = prodRace; }
  const r = await api("POST", "/studio/api/producers", body);
  if (!r) return;
  const p = await r.json();
  selected = prodNodeId(p);
  fetchState();
}
function startProdRename() {
  const p = state.producer_types.find(x => x.id === popupGoodId);
  if (!p) return;
  const el = $("popupBody").querySelector(".dname");
  el.innerHTML = '<input id="renameInput" type="text" value="' + esc(p.name) + '" style="width:100%;box-sizing:border-box">';
  const inp = $("renameInput");
  inp.focus(); inp.select();
  inp.addEventListener("keydown", ev => {
    if (ev.key === "Enter") commitProdRename();
    if (ev.key === "Escape") renderProdPopup();
  });
  inp.addEventListener("blur", commitProdRename);
}
async function commitProdRename() {
  const inp = $("renameInput");
  if (!inp) return;
  const name = inp.value.trim();
  if (!name) { renderProdPopup(); return; }
  const r = await api("PUT", "/studio/api/producers/" + popupGoodId, {name});
  if (r) fetchState(); else renderProdPopup();
}
async function changeProdCategory(catId) {
  const r = await api("PUT", "/studio/api/producers/" + popupGoodId, {category_id: Number(catId)});
  if (r) fetchState();
}
// --- копирование набора рецептов универсальной фабрики (ТЗ §13) ---
// copyRecipesCounts — клиентский расчёт K/M из state.producer_recipes (§13.2):
// источник — универсальные конкретные фабрики категории, кроме целевой.
function copyRecipesCounts(p) {
  const srcs = universalSources(p.category_id, p.id);
  const srcIds = new Set();
  srcs.forEach(s => factoryRecipeIds(s.id).forEach(rid => srcIds.add(rid)));
  const own = new Set(factoryRecipeIds(p.id));
  let k = 0, m = 0;
  srcIds.forEach(rid => { if (own.has(rid)) m++; else k++; });
  return {srcs, srcIds, own, k, m};
}
// copyRecipesBlock — разметка кнопки копирования набора рецептов в попапе
// постройки (§13.2). Счётчик набора рисует recipesBlock (шаг 2 §3.2).
function copyRecipesBlock(p) {
  const {srcs, srcIds, k, m} = copyRecipesCounts(p);
  const names = srcs.map(s => s.name).join('», «');
  let disabled, hint;
  if (!srcs.length) {
    disabled = true;
    hint = 'нет универсальной фабрики этой категории — копировать не из чего';
  } else if (k === 0) {
    disabled = true;
    hint = srcIds.size > 0 ? 'копировать нечего: все рецепты уже в наборе' : 'копировать нечего: у источников нет рецептов';
  } else {
    disabled = false;
    hint = 'источник: «' + names + '» · будет добавлено ' + k + ' (уже есть ' + m + ')';
  }
  return '<div class="dmeta">' +
    '<button class="gen" onclick="copyRecipes(' + p.id + ')"' +
    (disabled ? ' disabled title="' + esc(hint) + '"' : ' title="скопировать привязки универсальных фабрик категории"') +
    '>скопировать набор универсальной фабрики</button></div>' +
    '<div class="hint">' + esc(hint) + '</div>';
}
// prodHasOutputs — есть ли у постройки выходы output (энергия/предметы): у типа
// без выходов блок «Производит» не показываем (рецепт туда не назначить).
function prodHasOutputs(p) {
  const out = p.output || {};
  return out.energy != null || (Array.isArray(out.items) && out.items.length > 0);
}
// prodOutputs — выводы назначенных постройке рецептов (идея 2026-09-23, шаг 2
// §4): товары, чей recipe_id привязан к p. Порядок устойчивый — по тиру, затем
// по имени (иначе текст «плавает» между перерисовками).
function prodOutputs(p) {
  if (!p) return [];
  const bound = new Set(factoryRecipeIds(p.id));
  return state.goods
    .filter(g => g.recipe_id && bound.has(g.recipe_id))
    .sort((a, b) => (a.tier - b.tier) || a.name.localeCompare(b.name, "ru"));
}
// prodProducesText — компактная надпись «производит»: «Пища · т1, Вода · т1»;
// пустой набор — пустая строка (строку не показываем, §4).
function prodProducesText(p) {
  return prodOutputs(p).map(g => g.name + ' · т' + g.tier).join(', ');
}
// recipesBlock — блок «Производит» карточки постройки (спека 2026-09-23 §4.2 и
// §11.1 п.2): список «имя · тN [число ед/сутки/млрд] [− отвязать]» у ЛЮБОГО
// подтипа + выходы output одной строкой; строка без числа — предупреждение
// «не производит»; ниже — «забирает» по составу рецепта (§8.2). Пусто у
// подтипа — подсказка про перетаскивание. Кнопка копирования набора
// универсальной фабрики — только у категорийного подтипа kind=goods.
function recipesBlock(p) {
  const outs = prodOutputs(p);
  const out = p.output || {};
  const hasOutputs = Array.isArray(out.items) && out.items.length > 0;
  let inner = '';
  if (!outs.length && out.energy == null && !hasOutputs) {
    // подсказка — только у записи-подтипа (только ей можно назначить рецепт)
    if (p.parent_id) inner = '<div class="empty-note">перетащите рецепт из справочника (режим «Товары»)</div>';
  } else {
    outs.forEach(g => {
      const rate = recipeRate(p, g.recipe_id);
      inner += '<div class="usedin-item" style="display:flex;align-items:center;gap:6px;cursor:default">' +
        '<span>' + esc(g.name) + ' <span style="color:#777">· т' + g.tier + '</span></span>' +
        '<input type="number" min="0" step="any" style="width:96px;margin-left:auto" ' +
          'value="' + (rate == null ? '' : esc(String(scaleToDisplay(rate)))) + '" placeholder="не производит" ' +
          'title="число скорости, ' + esc(scaleUnitText()) + ' (пусто = не производит, 0 = объявленный ноль)" ' +
          'onchange="saveRecipeRate(' + p.id + ',' + g.recipe_id + ',this.value)">' +
        '<button class="bind-del" onclick="unbindFactoryRecipe(' + p.id + ',' + g.recipe_id + ')" title="отвязать рецепт от постройки">− отвязать</button>' +
        '</div>';
      // предупреждение (не блок): рецепт в наборе без числа — не производит (§7.1)
      if (rate == null) inner += '<div class="hint" style="color:#d9a">рецепт «' + esc(g.name) + '» в наборе без числа — не производит</div>';
      // проекция «забирает» по составу рецепта (§8.2)
      const take = recipeTakeText(g, rate);
      if (take) inner += '<div class="prow"><span class="pval">' + esc(take) + '</span></div>';
    });
    // выходы output (спека §4.2): «энергия — N» + предметы — одной строкой
    const emits = [];
    if (out.energy != null) emits.push("энергия — " + out.energy);
    if (hasOutputs) emits.push(...out.items);
    if (emits.length) inner += specRow(["Выпускает", emits.join(", ")]);
  }
  // единица числа и различие «пусто vs 0» (спека 2026-09-23 §11.2)
  if (outs.length) inner = '<div class="hint">число — ' + esc(scaleUnitText()) + ' (пусто = не производит, 0 = объявленный ноль)</div>' + inner;
  let html = '<div class="pblock"><div class="pblock-h">Производит</div>' + inner;
  // кнопка копирования набора — только у категорийного подтипа kind=goods (§13.2)
  if (p.kind === "goods" && p.parent_id) {
    const parent = state.producer_types.find(x => x.id === p.parent_id);
    if (parent && prodSlots(parent).length > 0) html += copyRecipesBlock(p);
  }
  html += '</div>';
  return html;
}
// recipeRate — число скорости пары «постройка × рецепт» (producer_recipes.rate,
// ед/сутки/млрд; null = «не объявлено» → не производит, спека 2026-09-23 §3.1).
function recipeRate(p, recipeId) {
  const b = (state.producer_recipes || []).find(pr =>
    String(pr.producer_type_id) === String(p.id) && String(pr.recipe_id) === String(recipeId));
  return b ? b.rate : null;
}
// saveRecipeRate — сохранение числа пары через PUT
// /studio/api/producers/{id}/recipes/{recipe_id} (спека §10): пусто → null
// («не производит»), 0 → объявленный ноль, < 0 → отказ.
async function saveRecipeRate(producerId, recipeId, val) {
  const v = String(val).trim();
  const display = v === "" ? null : Number(v); // ввод — в выбранном масштабе
  if (display != null && (!isFinite(display) || display < 0)) { showReport(["число скорости — неотрицательное"]); renderProdPopup(); return; }
  const rate = display == null ? null : scaleToStored(display); // на сервер — хранимая единица
  const r = await api("PUT", "/studio/api/producers/" + producerId + "/recipes/" + recipeId, {rate});
  if (r) fetchState(); else renderProdPopup();
}
