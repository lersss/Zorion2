"use strict";
// ---------- масштаб единицы темпа (спека 2026-09-23 §2.1) ----------
// Обёртки над единой точкой конверсии web/static/js/unit_scale.js (глобал
// UnitScale; классический скрипт студии не умеет ES-import). Множители не
// дублируем: если модуль не загрузился — показываем в хранимой единице (×1).
function unitScaleModule() { return globalThis.UnitScale || null; }
// scaleDef — описание выбранного масштаба (модуль недоступен → хранимая единица).
function scaleDef() {
  const U = unitScaleModule();
  if (!U) return {key: "billion", mul: 1, label: "на млрд", unit: "ед/сутки / 10⁹"};
  return U.unitScaleDef(unitScale);
}
// scaleToStored/scaleToDisplay — прямой и обратный перевод единицы (§2.1).
function scaleToStored(v) { const U = unitScaleModule(); return U ? U.displayToStored(v, unitScale) : v; }
function scaleToDisplay(v) { const U = unitScaleModule(); return U ? U.storedToDisplay(v, unitScale) : v; }
function scaleUnitText() { return scaleDef().unit; }
function scaleLabel() { return scaleDef().label; }
// unitScaleSegmentHtml — переключатель масштаба в карточке постройки.
function unitScaleSegmentHtml() {
  const U = unitScaleModule();
  const list = U ? U.UNIT_SCALE_LIST : [];
  let html = '<div class="tool-seg" style="margin:6px 0"><span class="seg-label">Единица темпа</span>';
  list.forEach(s => {
    html += '<button type="button" class="branch-btn' + (s.key === unitScale ? ' on' : '') +
      '" onclick="setUnitScale(\'' + s.key + '\')" title="показывать и вводить число как ' + esc(s.unit) + '">' + esc(s.label) + '</button>';
  });
  html += '<span class="seg-sub">на сервер — ед/сутки/млрд</span></div>';
  return html;
}
// setUnitScale — сохраняет масштаб (общий localStorage) и перерисовывает карточку.
function setUnitScale(key) {
  unitScale = key;
  localStorage.setItem("gs_unitScale", key);
  if (globalThis.UnitScale) unitScale = globalThis.UnitScale.readUnitScale(localStorage);
  renderProdPopup();
}
// fmtNum — число арифметики на млрд без хвостовых нулей (§8.1).
function fmtNum(n) {
  if (n == null || !isFinite(n)) return "—";
  return String(Math.round(n * 100) / 100);
}
// recipeTakeText — «забирает» по ветке (§8.2): состав рецепта (quantity_i × товар
// за 1 выход) и, при заданном числе, расчётный расход в сутки на млрд.
function recipeTakeText(g, rate) {
  const comps = (Array.isArray(g.recipe) ? g.recipe : []).filter(c => c.good_id);
  if (!comps.length) return "";
  let s = 'забирает: ' + comps.map(c => (c.quantity || 1) + ' × ' + c.name).join(' + ') + ' за 1 ' + g.name;
  if (rate != null) s += ' → ' + comps.map(c => fmtNum(scaleToDisplay((c.quantity || 1) * rate)) + ' ' + c.name).join(' + ') + ' за сутки (' + esc(scaleLabel()) + ')';
  return s;
}
// prodNorm — нормализация имени позиции (graph.NormalizeName: lower + trim +
// сжатие пробелов); ключи params.eat/effects — это goods.name_norm (спека
// 2026-09-24 §9.1: позиция потребления — ТОВАР, категория как позиция снята).
function prodNorm(s) { return String(s == null ? "" : s).toLowerCase().trim().replace(/\s+/g, " "); }
// goodByNorm/goodNameByNorm — товар по нормализованному имени (§11.2:
// отсутствующая в справочнике позиция — предупреждение).
function goodByNorm(norm) { return state.goods.find(g => prodNorm(g.name) === norm) || null; }
function goodNameByNorm(norm) { const g = goodByNorm(norm); return g ? g.name : norm; }
// prodPosOptions — селект позиции (ТОВАРЫ, goods.name_norm): value = name_norm.
function prodPosOptions(cur) {
  let html = '<option value=""' + (cur ? '' : ' selected') + '>— позиция —</option>';
  const known = state.goods.some(g => prodNorm(g.name) === cur);
  state.goods.forEach(g => {
    const n = prodNorm(g.name);
    html += '<option value="' + esc(n) + '"' + (n === cur ? ' selected' : '') + '>' + esc(g.name) + '</option>';
  });
  if (cur && !known) html += '<option value="' + esc(cur) + '" selected>' + esc(cur) + ' (нет в справочнике)</option>';
  return html;
}
// prodEffOptions — селект типа эффекта при нехватке: value = name_norm.
function prodEffOptions(cur) {
  // пока справочник не загружен (effectTypes === null) — не помечаем значение
  // «нет в справочнике»: это была бы ложная тревога (спека §11.2)
  const list = effectTypes || [];
  let html = '<option value=""' + (cur ? '' : ' selected') + '>— без эффекта —</option>';
  const known = list.some(e => e.name_norm === cur);
  list.forEach(e => {
    html += '<option value="' + esc(e.name_norm) + '"' + (e.name_norm === cur ? ' selected' : '') + '>' + esc(e.name) + '</option>';
  });
  if (cur && !known) html += '<option value="' + esc(cur) + '" selected>' + esc(cur) + (effectTypes !== null ? ' (нет в справочнике)' : '') + '</option>';
  return html;
}
// fetchEffectTypes — кэш типов эффектов (/studio/api/effects) для селекта
// «Потребляет» (в общем state их нет); после загрузки перерисовывает попап.
async function fetchEffectTypes() {
  if (effectTypes !== null || effectTypesLoading || effectTypesFailed) return;
  effectTypesLoading = true;
  const r = await api("GET", "/studio/api/effects");
  effectTypesLoading = false;
  if (!r) { effectTypesFailed = true; return; } // повтор — при следующем открытии попапа
  const data = await r.json();
  effectTypes = (data && data.effects) || [];
  // перерисовываем только открытую карточку постройки (не карточку товара)
  if (popupGoodId && state.producer_types.some(p => p.id === popupGoodId)) renderProdPopup();
}
// prodEatRowsFromParams — строки редактора из params.eat/effects (объединение
// ключей, порядок устойчивый). norm: "" = записи нет (мир применит 600, §8.2).
function prodEatRowsFromParams(eat, effects) {
  const keys = [...new Set([...Object.keys(effects), ...Object.keys(eat)])].sort();
  return keys.map(k => ({pos: k, eff: effects[k] || "", norm: eat[k] == null ? "" : String(eat[k])}));
}
// prodEatRowHtml — одна строка «Потребляет»: позиция → эффект → норма (§11.1 п.1).
function prodEatRowHtml(r, i) {
  // r.norm хранится в единице модели («ед/сутки/млрд»); показываем в масштабе.
  const shown = r.norm === "" ? "" : String(scaleToDisplay(Number(r.norm)));
  const defEat = String(scaleToDisplay(600)); // DefaultEatK — в выбранном масштабе
  return '<div class="eat-row" style="display:flex;gap:6px;align-items:center;margin-bottom:4px">' +
    '<select class="dcat" onchange="prodEatPosChange(' + i + ',this.value)">' + prodPosOptions(r.pos) + '</select>' +
    '<span style="color:#777">→</span>' +
    '<select class="dcat" onchange="prodEatEffChange(' + i + ',this.value)">' + prodEffOptions(r.eff) + '</select>' +
    '<input type="number" min="0" step="any" style="width:96px" value="' + esc(shown) + '" placeholder="по умолчанию ' + esc(defEat) + '" ' +
      'title="норма, ' + esc(scaleUnitText()) + ' (пусто = по умолчанию ' + esc(defEat) + ' при наличии эффекта)" ' +
      'onchange="prodEatNormChange(' + i + ',this.value)">' +
    '<button class="bind-del" style="margin-left:0" onclick="prodEatDelRow(' + i + ')" title="удалить строку">−</button>' +
    '</div>';
}
// eatBlock — блок «Потребляет» карточки постройки (спека 2026-09-23 §11.1 п.1):
// строки позиция → эффект при нехватке → норма, добавить/удалить. Семантика §8.2:
// потребляется только то, что попало в effects; позиция без эффекта —
// предупреждение «не потребляется»; пустой effects → «не потребляет».
function eatBlock(p) {
  const par = p.params || {};
  const eat = (par.eat && typeof par.eat === "object") ? par.eat : {};
  const effects = (par.effects && typeof par.effects === "object") ? par.effects : {};
  if (prodEatRowsFor !== p.id) { prodEatRows = prodEatRowsFromParams(eat, effects); prodEatRowsFor = p.id; }
  let html = '<div class="pblock"><div class="pblock-h">Потребляет</div>';
  html += '<div class="hint">потребляется только то, что попало в «эффект»; норма — ' + esc(scaleUnitText()) + ' (пусто при эффекте = по умолчанию ' + esc(String(scaleToDisplay(600))) + ')</div>';
  if (!prodEatRows.length) html += '<div class="empty-note">строк нет — добавьте позицию ниже</div>';
  prodEatRows.forEach((r, i) => {
    html += prodEatRowHtml(r, i);
    if (r.pos && !r.eff) html += '<div class="hint" style="color:#d9a">позиция «' + esc(goodNameByNorm(r.pos)) + '» без эффекта — не потребляется</div>';
    if (r.pos && !goodByNorm(r.pos)) html += '<div class="hint" style="color:#d9a">позиция «' + esc(r.pos) + '» отсутствует в справочнике товаров — не сохраняется</div>';
    if (r.eff && effectTypes !== null && !effectTypes.some(e => e.name_norm === r.eff)) html += '<div class="hint" style="color:#d9a">тип эффекта «' + esc(r.eff) + '» отсутствует в справочнике — не сохраняется</div>';
  });
  if (!prodEatRows.some(r => r.pos && r.eff)) html += '<div class="hint">не потребляет (ни одной позиции с эффектом)</div>';
  // расходники (input.consumables) — read-only, как в прежнем блоке «Потребляет»
  const cons = (p.input && Array.isArray(p.input.consumables)) ? p.input.consumables : [];
  if (cons.length) html += specRow(["Расходники", cons.join(", ")]);
  html += '<div class="addrow" style="margin-top:4px"><button class="gen" onclick="prodEatAddRow()">+ строка</button></div>';
  html += '</div>';
  return html;
}
// prodEatAddRow/prodEatDelRow — добавить/удалить строку редактора (§11.1).
function prodEatAddRow() {
  if (!prodEatRows) prodEatRows = [];
  prodEatRows.push({pos: "", eff: "", norm: ""});
  renderProdPopup();
}
function prodEatDelRow(i) {
  if (!prodEatRows) return;
  prodEatRows.splice(i, 1);
  saveEatBlock();
}
function prodEatPosChange(i, val) { if (prodEatRows && prodEatRows[i]) { prodEatRows[i].pos = val; saveEatBlock(); } }
function prodEatEffChange(i, val) { if (prodEatRows && prodEatRows[i]) { prodEatRows[i].eff = val; saveEatBlock(); } }
function prodEatNormChange(i, val) {
  if (!prodEatRows || !prodEatRows[i]) return;
  const v = String(val).trim();
  // модель хранит норму в единице модели: ввод в масштабе → хранимое; пусто — пусто.
  prodEatRows[i].norm = v === "" ? "" : String(scaleToStored(Number(v)));
  saveEatBlock();
}
// saveEatBlock — запись типизированных eat/effects (PUT /producers/{id}, §10):
// строка без позиции пропускается; норма пуста → в eat не пишем (мир возьмёт
// 600); позиция/тип эффекта вне справочника не шлются (сервер вернёт 422, §9.2),
// но норма такой позиции остаётся в eat (позиция без эффекта — «не потребляется»).
async function saveEatBlock() {
  if (!popupGoodId) return;
  const eat = {}, effects = {};
  const effKnown = e => effectTypes === null || effectTypes.some(x => x.name_norm === e);
  (prodEatRows || []).forEach(r => {
    if (!r.pos || !goodByNorm(r.pos)) return;
    if (r.eff && effKnown(r.eff)) effects[r.pos] = r.eff;
    if (r.norm !== "") eat[r.pos] = Number(r.norm);
  });
  const r = await api("PUT", "/studio/api/producers/" + popupGoodId, {eat, effects});
  if (r) fetchState(); else renderProdPopup();
}
// stageBlock — блок «Стадия» карточки постройки (спека 2026-09-23 §11.1 п.3):
// [порог входа] → [порог выхода] в людях; у низшей ступени (enter = 0) выход
// «не читается (пол)»; связка со следующей ступенью — по данным (§11.1);
// exit ≥ enter — ошибка, сохранение блокировано (§11.2). Только у ступени
// класса без слотов («Колония») — тот же предикат, что prodSubtypeNoCatRecords.
function stageBlock(p) {
  if (!p.parent_id || p.kind !== "goods" || p.category_id != null) return "";
  const parent = state.producer_types.find(x => x.id === p.parent_id);
  if (!parent || prodSlots(parent).length !== 0) return "";
  const st = (p.params && p.params.stage) || {};
  if (prodStageEditFor !== p.id) {
    prodStageEdit = {
      enter: st.enter != null ? String(st.enter) : "",
      exit: st.exit != null ? String(st.exit) : "",
    };
    prodStageEditFor = p.id;
  }
  const enter = prodStageEdit.enter, exit = prodStageEdit.exit;
  const enterNum = enter === "" ? null : Number(enter);
  const exitNum = exit === "" ? null : Number(exit);
  let html = '<div class="pblock"><div class="pblock-h">Стадия</div>';
  html += '<div class="hint">пороги в людях: вход — переход в эту ступень, выход — гистерезис (ниже входа)</div>';
  html += '<div class="bind-row">' +
    '<input type="number" min="0" step="any" style="width:120px" value="' + esc(enter) + '" placeholder="порог входа" title="порог входа, чел." onchange="prodStageEnterChange(this.value)">' +
    '<span style="color:#777">→</span>' +
    '<input type="number" min="0" step="any" style="width:120px" value="' + esc(exit) + '" placeholder="порог выхода" title="порог выхода, чел. (меньше порога входа)" onchange="prodStageExitChange(this.value)">' +
    '</div>';
  if (enterNum === 0) html += '<div class="hint">порог выхода не читается (пол)</div>';
  if (enterNum != null && exitNum != null && exitNum >= enterNum) {
    html += '<div class="hint" style="color:#f66">порог выхода должен быть меньше порога входа</div>';
  }
  if (enter === "" && exit === "") html += '<div class="hint">заполните хотя бы порог входа</div>';
  const next = prodNextStage(p);
  if (next) {
    html += '<div class="dmeta">следующая ступень: <b>' + esc(next.name) + '</b> (при ' +
      Number(prodStageEnter(next)).toLocaleString("ru-RU") + ' чел.)</div>';
  } else {
    html += '<div class="dmeta">дальше ступеней нет</div>';
  }
  html += '</div>';
  return html;
}
// prodStageEnterChange/prodStageExitChange — правка полей блока (§11.1 п.3):
// значение кладётся в модель попапа и сохраняется (saveStageBlock).
function prodStageEnterChange(val) {
  if (!prodStageEdit) prodStageEdit = {enter: "", exit: ""};
  prodStageEdit.enter = String(val).trim();
  saveStageBlock();
}
function prodStageExitChange(val) {
  if (!prodStageEdit) prodStageEdit = {enter: "", exit: ""};
  prodStageEdit.exit = String(val).trim();
  saveStageBlock();
}
// saveStageBlock — запись типизированных порогов stage (PUT /producers/{id},
// §10): оба поля пусты — не шлём (подсказка в блоке); exit ≥ enter — ошибка
// §11.2, сохранение блокировано; шлём только заполненные числа. Успех —
// fetchState (пересчёт порядка и связки), ошибка — перерисовка (как saveEatBlock).
async function saveStageBlock() {
  if (!popupGoodId) return;
  const enter = prodStageEdit ? prodStageEdit.enter : "";
  const exit = prodStageEdit ? prodStageEdit.exit : "";
  const enterNum = enter === "" ? null : Number(enter);
  const exitNum = exit === "" ? null : Number(exit);
  if (enterNum == null && exitNum == null) { renderProdPopup(); return; }
  if (enterNum != null && exitNum != null && exitNum >= enterNum) { renderProdPopup(); return; }
  const stage = {};
  if (enterNum != null) stage.enter = enterNum;
  if (exitNum != null) stage.exit = exitNum;
  const r = await api("PUT", "/studio/api/producers/" + popupGoodId, {stage});
  if (r) fetchState(); else renderProdPopup();
}
// prodArithmeticBlock — проекция арифметики «на млрд особей» (спека 2026-09-23
// §8.1/§8.2): по позиции производим − потребляем = сверх (отрицательное —
// «дефицит»). «Производим» — сумма чисел рецептов набора по позиции выхода;
// «потребляем» — только позиции из effects (норма eat[pos] или DefaultEatK 600).
function prodArithmeticBlock(p) {
  const par = p.params || {};
  const eat = (par.eat && typeof par.eat === "object") ? par.eat : {};
  const effects = (par.effects && typeof par.effects === "object") ? par.effects : {};
  const produced = {};
  prodOutputs(p).forEach(g => {
    const rate = recipeRate(p, g.recipe_id);
    if (rate == null) return;
    // позиция = ТОВАР-выход ветки (goods.name_norm, спека 2026-09-24 §7.3)
    const pos = prodNorm(g.name);
    produced[pos] = (produced[pos] || 0) + rate;
  });
  const positions = [...new Set([...Object.keys(produced), ...Object.keys(effects)])].sort();
  if (!positions.length) return "";
  const unit = scaleUnitText();
  let html = '<div class="pblock"><div class="pblock-h">Арифметика (' + esc(scaleLabel()) + ')</div>';
  positions.forEach(pos => {
    const prod = produced[pos] || 0;
    const cons = Object.prototype.hasOwnProperty.call(effects, pos) ? (eat[pos] != null ? Number(eat[pos]) : 600) : 0;
    const diff = prod - cons; // всё в хранимой единице; показ — в выбранном масштабе
    const tail = diff < 0 ? 'дефицит ' + fmtNum(scaleToDisplay(-diff)) : 'сверх потребления +' + fmtNum(scaleToDisplay(diff));
    html += specRow(["", goodNameByNorm(pos) + ': производим ' + fmtNum(scaleToDisplay(prod)) + ' ' + unit + ' · потребляем ' + fmtNum(scaleToDisplay(cons)) + ' ' + unit + ' · ' + tail + ' ' + unit]);
  });
  html += '</div>';
  return html;
}
// unbindFactoryRecipe — «− отвязать» из набора постройки (DELETE
// /studio/api/producers/{id}/recipes/{recipe_id}, шаг 2 §3.2).
async function unbindFactoryRecipe(producerId, recipeId) {
  const r = await api("DELETE", "/studio/api/producers/" + producerId + "/recipes/" + recipeId);
  if (r) fetchState();
}
// copyRecipes — подтверждение перед копированием (§13.3): объём K понятен заранее.
function copyRecipes(id) {
  const p = state.producer_types.find(x => x.id === id);
  if (!p) return;
  const {srcs, k, m} = copyRecipesCounts(p);
  const names = srcs.map(s => s.name).join('», «');
  openModal(
    '<h3>Скопировать набор рецептов</h3>' +
    '<div>Скопировать набор рецептов в «' + esc(p.name) + '»? Будет добавлено <b>' + k + '</b> рецептов (уже в наборе: ' + m + '). Источник: «' + esc(names) + '».</div>',
    [
      {label: "Скопировать", cls: "gen", primary: true, action: () => doCopyRecipes(id)},
      {label: "Отмена", cls: "", action: closeModal}
    ]
  );
}
// doCopyRecipes — POST copy-universal → тост «добавлено K, пропущено M» → fetchState (§13.3).
async function doCopyRecipes(id) {
  closeModal();
  const r = await api("POST", "/studio/api/producers/" + id + "/recipes/copy-universal");
  if (!r) return;
  const out = await r.json();
  const added = (out && out.added) || 0, skipped = (out && out.skipped) || 0;
  if (added === 0 && skipped === 0) showReport(["копировать нечего"]);
  else showReport(["добавлено " + added + ", пропущено " + skipped]);
  fetchState();
}
// семейство/раса в попапе (спека §5): каскад — выбор расы фиксирует семейство,
// выбор семейства сбрасывает расу; сохраняется через PUT {race_family, race}.
function popupRaceFamilyChanged() {
  const raceSel = $("popupRaceSel");
  if (raceSel) raceSel.value = "";
  saveProdRace();
}
function popupRaceChanged() {
  const raceSel = $("popupRaceSel");
  const famSel = $("popupRaceFamilySel");
  if (raceSel && famSel && raceSel.value) {
    const fam = raceFamilyOf(raceSel.value);
    if (fam) famSel.value = fam;
  }
  saveProdRace();
}
async function saveProdRace() {
  const famSel = $("popupRaceFamilySel");
  const raceSel = $("popupRaceSel");
  if (!famSel || !raceSel) return;
  const r = await api("PUT", "/studio/api/producers/" + popupGoodId, {race_family: famSel.value, race: raceSel.value});
  if (r) fetchState();
}
// --- редактор слотов родителя (спека 2026-09-21-скрытые §3.2) ---
// Скрытость живёт на СЛОТЕ родителя, не на записи (С6): галки «скрытая»
// расставляются из карточки типа kind=goods на текущем уровне расовости.

// prodSlotAppliesTo(s, p) — применим ли слот к уровню расовости ЗАПИСИ p
// (спека скрытых §2.1): универсальный слот (race_family IS NULL) — всем;
// семейный (race_family=Fk, race NULL) — записям с race_family=Fk (включая
// расовые записи семейства — у них race_family=Fk); расовый (race=R) —
// только записям с race=R.
function prodSlotAppliesTo(s, p) {
  if (!s.race_family) return true;
  if (s.race) return s.race === p.race;
  return s.race_family === p.race_family;
}

// prodCategoryOptions(p) — категории с применяемым слотом родителя на уровне
// расовости ЗАПИСИ p (её race_family/race, спека скрытых §3.4), не текущего
// уровня UI: универсальная запись при уровне UI «Семейство F4» видит
// универсальные/наследуемые слоты, семейная запись при «Универсальном» — свои
// (иначе 400 «категория не настроена у родителя на этом уровне», инвариант С4).
function prodCategoryOptions(p) {
  const t = state.producer_types.find(x => x.id === p.parent_id);
  if (!t) return '';
  const slots = prodSlots(t).filter(s => prodSlotAppliesTo(s, p));
  const catIDs = [...new Set(slots.map(s => s.category_id))];
  return catIDs.map(catID => {
    const c = state.categories.find(x => x.id === catID);
    if (!c) return '';
    return '<option value="' + c.id + '"' + (c.id === p.category_id ? ' selected' : '') + '>' + esc(c.name) + '</option>';
  }).join("");
}

// prodSlotEditor(t) — HTML секции «Слоты категорий» (редактор слотов):
// список категорий набора на текущем уровне с галками «скрытая», пометками
// «унаследован/переопределён», «− слот»; внизу «+ слот категории».
function prodSlotEditor(t) {
  const levelLabel = prodRaceLevel === "universal" ? "Универсальный (база)"
    : prodRaceLevel === "family" ? "Семейство " + familyLabelById(prodRaceFamily)
    : "Раса " + raceName(prodRace);
  const applied = prodAppliedSlots(t);
  const catIDs = [...new Set(applied.map(s => s.category_id))];
  let html = '<div class="pblock"><div class="pblock-h">Слоты категорий <span class="seg-sub">· ' + esc(levelLabel) + '</span></div>';
  if (!catIDs.length) {
    html += '<div class="empty-note">слотов нет — добавьте категории ниже</div>';
  } else {
    html += '<div class="prod-slots-list">';
    catIDs.forEach(catID => {
      const cat = state.categories.find(c => c.id === catID);
      if (!cat) return;
      const recs = applied.filter(s => s.category_id === catID);
      const most = prodMostSpecificSlot(recs);
      const own = recs.find(prodSlotOwned) || null; // слот текущего уровня (переопределение) / база на universal
      const base = recs.find(s => !s.race_family && !s.race) || null;
      const inherited = !own; // на universal own = база → переопределения нет
      // Обратный кейс (§3.2): скрытая база + видимое переопределение — хинт
      const baseHiddenHint = (base && own && own !== base && base.hidden && !own.hidden)
        ? '<span style="color:#777;font-size:12px"> база скрыта</span>' : '';
      const labelHtml = inherited
        ? '<span style="color:#777;font-size:12px">унаследован</span>'
        : '<span class="badge b-override">переопределён</span>';
      const removeBtn = own
        ? '<button onclick="removeProdSlot(' + t.id + ',' + catID + ')" title="' +
          (own.race_family ? 'снять переопределение' : 'убрать из набора родителя') + '">−</button>'
        : '';
      html += '<div class="pslot' + (most.hidden ? ' hidden' : '') + '">' +
        '<label style="font-size:13px"><input type="checkbox"' + (most.hidden ? ' checked' : '') +
        ' onchange="toggleProdSlot(' + t.id + ',' + catID + ',this.checked)" title="Не показывать категорию на этом уровне расовости"> скрытая</label>' +
        '<span class="cname" title="' + esc(cat.name) + '">' + esc(cat.name) + '</span>' + labelHtml + baseHiddenHint + removeBtn +
        '</div>';
    });
    html += '</div>';
  }
  // «+ слот категории»: категории ВНЕ набора на текущем уровне
  const free = state.categories.filter(c => !catIDs.includes(c.id));
  html += '<div class="addrow" style="margin-top:4px"><select id="prodNewSlotCat">' +
    (free.length ? free.map(c => '<option value="' + c.id + '">' + esc(c.name) + '</option>').join("") : '<option value="">нет свободных</option>') +
    '</select><button class="gen" onclick="addProdSlot(' + t.id + ')"' + (free.length ? '' : ' disabled') + '>+ слот</button></div>';
  html += '</div>';
  return html;
}

// toggleProdSlot — галка «скрытая» в редакторе (§3.2): собственный слот
// уровня → PUT {hidden}; унаследованный → создать переопределение уровня
// (POST) с новым значением (база не трогается).
async function toggleProdSlot(tid, catID, checked) {
  const t = state.producer_types.find(x => x.id === tid);
  if (!t) return;
  const applied = prodAppliedSlots(t);
  const recs = applied.filter(s => s.category_id === catID);
  const own = recs.find(prodSlotOwned) || null;
  if (own) {
    await api("PUT", "/studio/api/slots/" + own.id, {hidden: checked});
  } else {
    const body = {parent_id: tid, category_id: catID, hidden: checked};
    if (prodRaceLevel === "family") body.race_family = prodRaceFamily;
    if (prodRaceLevel === "race") { body.race_family = raceFamilyOf(prodRace); body.race = prodRace; }
    await api("POST", "/studio/api/slots", body);
  }
  fetchState();
}

// removeProdSlot — «− слот» (§3.2/§5.3): снятие переопределения уровня — без
// подтверждения (обратимо); удаление базового слота из набора родителя —
// подтверждение-модалка (RESTRICT 409 при заводах).
function removeProdSlot(tid, catID) {
  const t = state.producer_types.find(x => x.id === tid);
  if (!t) return;
  const recs = prodAppliedSlots(t).filter(s => s.category_id === catID);
  const own = recs.find(prodSlotOwned) || null;
  if (!own) return;
  if (!own.race_family && prodRaceLevel === "universal") {
    const cat = state.categories.find(c => c.id === catID);
    openModal(
      '<h3>Убрать категорию из набора</h3>' +
      '<div>Убрать «' + esc(cat ? cat.name : catID) + '» из набора родителя на универсальном уровне? Заводы категории останутся.</div>',
      [
        {label: "Убрать", cls: "del", primary: true, action: () => doRemoveProdSlot(own.id)},
        {label: "Отмена", cls: "", action: closeModal}
      ]
    );
    return;
  }
  doRemoveProdSlot(own.id); // снять переопределение уровня — обратимо
}
async function doRemoveProdSlot(slotId) {
  closeModal();
  const r = await api("DELETE", "/studio/api/slots/" + slotId);
  if (r) fetchState();
}

// addProdSlot — «+ слот категории»: POST слот уровня/базы (категория вне
// набора на текущем уровне).
async function addProdSlot(tid) {
  const sel = $("prodNewSlotCat");
  if (!sel || !sel.value) return;
  const body = {parent_id: tid, category_id: Number(sel.value)};
  if (prodRaceLevel === "family") body.race_family = prodRaceFamily;
  if (prodRaceLevel === "race") { body.race_family = raceFamilyOf(prodRace); body.race = prodRace; }
  const r = await api("POST", "/studio/api/slots", body);
  if (r) fetchState();
}
async function saveProdJSON() {
  const body = {};
  const fields = [["input", "prodInput"], ["output", "prodOutput"], ["params", "prodParams"]];
  for (const [key, elId] of fields) {
    const v = $(elId).value.trim();
    if (v === "") { body[key] = null; continue; }
    try { JSON.parse(v); } catch (e) { showReport(["ошибка: " + key + " — невалидный JSON"]); return; }
    body[key] = v;
  }
  const r = await api("PUT", "/studio/api/producers/" + popupGoodId, body);
  if (r) fetchState();
}
// saveProdHidden — скрыть/показать запись-фабрику из чекбокса (единственный
// носитель скрытия, спека 2026-09-21 §4.2/К1).
async function saveProdHidden(checked) {
  await api("POST", "/studio/api/producers/" + popupGoodId + "/hidden", {hidden: !!checked});
  fetchState();
}
function askDeleteProducer() {
  const p = state.producer_types.find(x => x.id === popupGoodId);
  if (!p) return;
  openModal(
    '<h3>Удалить тип постройки</h3>' +
    '<div>Удалить тип «' + esc(p.name) + '»? Связи с предметами удалятся.</div>',
    [
      {label: "Удалить", cls: "del", primary: true, action: doDeleteProducer},
      {label: "Отмена", cls: "", action: closeModal}
    ]
  );
}
async function doDeleteProducer() {
  closeModal();
  const r = await api("DELETE", "/studio/api/producers/" + popupGoodId);
  if (selected === popupGoodId) selected = null;
  closePopup();
  if (r) fetchState();
}
async function linkItem() {
  const sel = $("prodLinkItemSel");
  if (!sel || !sel.value) return;
  const r = await api("POST", "/studio/api/producers/" + popupGoodId + "/items", {item_id: Number(sel.value)});
  if (r) fetchState();
}
async function unlinkItem(itemId) {
  const r = await api("DELETE", "/studio/api/producers/" + popupGoodId + "/items/" + itemId);
  if (r) fetchState();
}
async function prodAdd() {
  const nameEl = $("prodNewName");
  if (!nameEl) return; // форма доступна только из открытого попапа
  const name = nameEl.value.trim();
  const kind = $("prodNewKind").value;
  if (!name) return;
  // «+ тип» создаёт тип (parent_id NULL): категории у типов нет (спека
  // 2026-09-21 §1.2 п.4 — категории живут в подтипах, их заводят через
  // узлы-приглашения дерева на канвасе). Раздел обязателен (спека 2026-09-25
  // §6.4): селект предзаполнен активным разделом, «Прочее» в нём нет.
  const body = {name, kind};
  const sectionEl = $("prodNewSection");
  if (sectionEl && sectionEl.value) body.section = sectionEl.value;
  // расовость из фильтра шапки — как в узле-приглашении (doProdCreate)
  if (prodRaceLevel === "family") body.race_family = prodRaceFamily;
  if (prodRaceLevel === "race") { body.race_family = raceFamilyOf(prodRace); body.race = prodRace; }
  const r = await api("POST", "/studio/api/producers", body);
  if (!r) return; // ошибка — попап остаётся открытым с введённым именем
  const p = await r.json();
  // переключиться на раздел созданного корня — запись видна сразу (§6.4)
  if (p.section) { prodSection = p.section; localStorage.setItem("gs_prodSection", p.section); }
  nameEl.value = "";
  selected = prodNodeId(p);
  closeModal();
  fetchState();
}
