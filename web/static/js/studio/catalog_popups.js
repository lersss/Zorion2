"use strict";
// ---------- категории и товары ----------
// Селекты разделены по kind (спека iterB §4.5): товарные — только
// kind=good, ресурсные — только kind=resource (иначе POST → 400).
function renderCatSelects() {
  const goodOpts = state.categories.filter(c => c.kind === "good").map(c => '<option value="' + c.id + '">' + esc(c.name) + '</option>').join("");
  const resOpts = state.categories.filter(c => c.kind === "resource").map(c => '<option value="' + c.id + '">' + esc(c.name) + '</option>').join("");
  const cur = pendingCatFilter || $("catFilter").value;
  $("catFilter").innerHTML = '<option value="">все категории</option>' + goodOpts;
  if (state.categories.some(c => String(c.id) === cur)) $("catFilter").value = cur;
  pendingCatFilter = "";
  // «+ товар»: при выбранной фабрике категория определяется фабрикой и
  // селект заблокирован (ТЗ §7.2); #catFilter/#spravCat остаются общими.
  // #spravNewCat/#spravNewResCat/#btnSpravAddRes существуют только при
  // открытом попапе формы (ТЗ §7.6 п.1) — все обращения под null-защитой.
  const ft = factoryMode() ? factoryType() : null;
  const newCat = $("spravNewCat");
  if (newCat) {
    if (ft && ft.category_id != null) {
      newCat.innerHTML = '<option value="' + ft.category_id + '">' + esc(catName(ft.category_id)) + '</option>';
      newCat.disabled = true;
      newCat.title = "категория определяется выбранной фабрикой";
    } else {
      newCat.innerHTML = goodOpts;
      newCat.disabled = false;
      newCat.title = "";
    }
  }
  const newResCat = $("spravNewResCat");
  if (newResCat) newResCat.innerHTML = resOpts;
  // ресурсных категорий нет — «+ ресурс» недоступен: кнопка в шапке + кнопка
  // попапа (если открыт) — перенос #btnSpravAddRes.disabled (ТЗ §7.2)
  const noResCats = !resOpts;
  const resAddBtn = $("btnSpravAddRes");
  if (resAddBtn) resAddBtn.disabled = noResCats;
  const resOpenBtn = $("btnNewResOpen");
  if (resOpenBtn) {
    resOpenBtn.disabled = noResCats;
    resOpenBtn.title = noResCats ? "нет ресурсных категорий — заведите их в „Категории“" : "создать ресурс";
  }
  // фильтр справочника по категории: сохранить текущий выбор при пересборке;
  // при старте опции ещё не заполнены — берём сохранённое из localStorage
  const sc = $("spravCat").value || localStorage.getItem("gs_spravCat") || "";
  $("spravCat").innerHTML = '<option value="">все категории</option>' + goodOpts;
  if (state.categories.some(c => String(c.id) === sc)) $("spravCat").value = sc;
}
// ---------- попап категорий (99a.3-ui §14) ----------
function openCatPopup() {
  $("catPopup").style.display = "flex";
  $("catOverlay").style.display = "block";
  renderCatPopup();
}
function closeCatPopup() {
  $("catPopup").style.display = "none";
  $("catOverlay").style.display = "none";
}
// список категорий: имя (dblclick/✏ = inline-rename) + счётчик по kind + корзина;
// системные (is_system) — метка «системная», ✏/🗑 disabled (сервер 403, §4.5)
function renderCatPopup() {
  const list = $("catList");
  if (!list) return;
  const counts = {};
  state.goods.forEach(g => { counts[g.category_id] = (counts[g.category_id] || 0) + 1; });
  list.innerHTML = state.categories.map(c => {
    const n = counts[c.id] || 0;
    const sys = c.is_system ? ' <span class="hint" title="системная категория — переименование/удаление запрещено">системная</span>' : '';
    return '<div class="cat-row">' +
      '<button onclick="startCatRename(\'' + c.id + '\')" title="переименовать"' + (c.is_system ? ' disabled' : '') + '>✏</button>' +
      '<span class="cname" ondblclick="startCatRename(\'' + c.id + '\')">' + esc(c.name) + sys + '</span>' +
      '<span class="ccount">' + n + '</span>' +
      '<button class="cat-del" onclick="askDeleteCategory(\'' + c.id + '\')"' + (c.is_system || n > 0 ? ' disabled title="' + (c.is_system ? "системная категория" : "сначала перекатегоризуйте товары (" + n + ")") + '"' : ' title="удалить категорию"') + '>🗑</button>' +
      '</div>';
  }).join("") || '<div class="empty-note">нет категорий</div>';
}
// создание категории (Enter в поле = клик по «+ категория»; дубликат — тост 409)
async function addCategory() {
  const name = $("catNewName").value.trim();
  if (!name) return;
  const r = await api("POST", "/studio/api/categories", {name});
  if (r) { $("catNewName").value = ""; fetchState(); }
}
// inline-переименование: двойной клик по имени / ✏ → input; Enter/Blur — PUT; Esc — отмена
function startCatRename(id) {
  const c = state.categories.find(x => x.id === id);
  if (!c || c.is_system) return; // системная — сервер 403, не предлагаем
  const row = [...document.querySelectorAll('#catList .cat-row')].find(r => r.querySelector('.cname') && r.querySelector('.cname').textContent === c.name);
  if (!row) return;
  const nameEl = row.querySelector('.cname');
  nameEl.innerHTML = '<input id="catRenameInput" type="text" value="' + esc(c.name) + '">';
  const inp = $("catRenameInput");
  inp.focus(); inp.select();
  inp.addEventListener("keydown", ev => {
    if (ev.key === "Enter") commitCatRename(id);
    if (ev.key === "Escape") renderCatPopup();
  });
  inp.addEventListener("blur", () => commitCatRename(id));
}
async function commitCatRename(id) {
  const inp = $("catRenameInput");
  if (!inp) return;
  const name = inp.value.trim();
  if (!name) { renderCatPopup(); return; }
  const r = await api("PUT", "/studio/api/categories/" + id, {name});
  if (r) fetchState(); else renderCatPopup(); // ошибка (409/400) — тост, имя возвращается
}
// удаление: пустая — модалка подтверждения; непустая/системная — корзина disabled (счётчик)
function askDeleteCategory(id) {
  const c = state.categories.find(x => x.id === id);
  if (!c || c.is_system) return; // системная — сервер 403, не предлагаем
  const n = state.goods.filter(g => g.category_id === id).length;
  if (n > 0) return; // корзина disabled — сюда не попадём
  openModal(
    '<h3>Удалить категорию</h3>' +
    '<div>Удалить категорию «' + esc(c.name) + '»?</div>',
    [
      {label: "Удалить", cls: "del", primary: true, action: () => deleteCategory(id)},
      {label: "Отмена", cls: "", action: closeModal}
    ]
  );
}
async function deleteCategory(id) {
  closeModal();
  const r = await api("DELETE", "/studio/api/categories/" + id);
  if (r) fetchState();
}
// ---------- попап «Эффекты» (спека эффектов §7.4) ----------
// Тип эффекта: имя, impact (открытый набор, §5.4), curve — ссылка на компоненту
// «Балансировки»; recovery в типе НЕ хранится (скаляр Балансировки). Каталог
// тянется отдельным эндпоинтом /studio/api/effects (не в общем state).
function openEffectPopup() {
  $("effPopup").style.display = "flex";
  $("effOverlay").style.display = "block";
  renderEffectPopup();
}
function closeEffectPopup() {
  $("effPopup").style.display = "none";
  $("effOverlay").style.display = "none";
}
async function renderEffectPopup() {
  const list = $("effList");
  if (!list) return;
  const r = await api("GET", "/studio/api/effects");
  const data = r ? await r.json() : {effects: []};
  const arr = (data && data.effects) || [];
  list.innerHTML = arr.map(e =>
    '<div class="cat-row">' +
      '<span class="cname" title="нормализованное имя: ' + esc(e.name_norm) + '">' + esc(e.name) + '</span>' +
      '<span class="ccount" title="impact / curve">' + esc(e.impact) + (e.curve ? ' · ' + esc(e.curve) : ' · без кривой') + '</span>' +
      (e.code ? '<span class="ccount" title="метка переноса">' + esc(e.code) + '</span>' : '') +
      '<button class="cat-del" onclick="askDeleteEffectType(' + e.id + ')" title="удалить тип эффекта">🗑</button>' +
    '</div>'
  ).join("") || '<div class="empty-note">нет типов эффектов</div>';
}
async function addEffectType() {
  const name = $("effNewName").value.trim();
  const impact = $("effNewImpact").value.trim() || "population_rate";
  const curve = $("effNewCurve").value.trim();
  if (!name) return;
  const r = await api("POST", "/studio/api/effects", {name, impact, curve});
  if (r) { effectTypes = null; effectTypesFailed = false; $("effNewName").value = ""; $("effNewCurve").value = ""; renderEffectPopup(); } // кэш селекта «Потребляет»
}
// удаление: тип с действующими эффектами → 409 (сервер); сначала счётчик
async function askDeleteEffectType(id) {
  const c = await api("GET", "/studio/api/effects/" + id + "/counts");
  const n = c ? await c.json() : {active_effects: 0};
  if (n && n.active_effects > 0) {
    showReport(["тип используется действующими эффектами (" + n.active_effects + ") — удаление запрещено"]);
    return;
  }
  openModal(
    '<h3>Удалить тип эффекта</h3><div>Удалить тип эффекта #' + id + '?</div>',
    [
      {label: "Удалить", cls: "del", primary: true, action: () => deleteEffectType(id)},
      {label: "Отмена", cls: "", action: closeModal}
    ]
  );
}
async function deleteEffectType(id) {
  closeModal();
  const r = await api("DELETE", "/studio/api/effects/" + id);
  if (r) { effectTypes = null; effectTypesFailed = false; renderEffectPopup(); } // кэш селекта «Потребляет»
}
// «+ товар» — единственный путь создания товара (addbar удалён по решению
// создателя 2026-09-19; 99a.3-ui §13). Форма — в попапе (ТЗ §7.3): поля
// существуют только пока окно открыто; при ошибке окно остаётся открытым.
async function spravAddGood() {
  const nameEl = $("spravNewName"), catEl = $("spravNewCat");
  if (!nameEl || !catEl) return; // форма доступна только из открытого попапа
  const name = nameEl.value.trim();
  const cat = catEl.value;
  if (!name || !cat) return;
  // category_id — число (строка → 400, спека iterB §4.3)
  const r = await api("POST", "/studio/api/goods", {name, category_id: Number(cat)});
  if (!r) return; // ошибка (409 и т.п.) — окно открыто, имя сохранено
  const g = await r.json();
  nameEl.value = "";
  selected = g.id;
  // при выбранной фабрике — сразу привязать рецепт нового товара (ТЗ §7.2)
  if (factoryMode()) {
    const ft = factoryType();
    if (!g.recipe_id) {
      showReport(["товар создан без рецепта — привяжите вручную"]);
    } else if (g.category_id === ft.category_id) {
      await api("POST", "/studio/api/producers/" + ft.id + "/recipes", {recipe_id: g.recipe_id});
    }
  }
  // галка «описание ИИ» (§9.3): после создания — одиночный прогон по записи
  const aiChk = $("newGoodDescAI");
  if (aiChk && aiChk.checked) {
    await api("POST", "/studio/api/descriptions/fill", {good_ids: [String(g.id)]});
  }
  closeModal();
  fetchState();
}
// «+ ресурс» (спека iterB §4.5): ресурс без привилегий — создание через
// справочник; категория — только ресурсная (иначе 400); слотов нет (лист)
async function spravAddResource() {
  const nameEl = $("spravNewResName"), catEl = $("spravNewResCat");
  if (!nameEl || !catEl) return; // форма доступна только из открытого попапа
  const name = nameEl.value.trim();
  const cat = catEl.value;
  if (!name || !cat) return;
  const r = await api("POST", "/studio/api/goods", {name, category_id: Number(cat), kind: "resource"});
  if (!r) return; // ошибка — окно открыто, имя сохранено
  const g = await r.json();
  nameEl.value = "";
  selected = g.id;
  // галка «описание ИИ» (§9.3): после создания — одиночный прогон по записи
  const aiChk = $("newResDescAI");
  if (aiChk && aiChk.checked) {
    await api("POST", "/studio/api/descriptions/fill", {good_ids: [String(g.id)]});
  }
  closeModal();
  fetchState();
}
// подгрузка списка (99a.3 §8.5): POST /api/goods/bulk → тост-отчёт по строкам
async function bulkCreate() {
  const ta = $("bulkText");
  if (!ta) return; // форма доступна только из открытого попапа
  const lines = ta.value.split("\n");
  const r = await api("POST", "/studio/api/goods/bulk", {lines});
  if (!r) return; // ошибка — окно открыто, список сохранён
  const out = await r.json();
  const lines2 = ["Создано " + (out.created || []).length + ", пропущено " + (out.skipped || []).length + ", ошибок " + (out.errors || []).length];
  (out.skipped || []).forEach(s => lines2.push("строка " + s.line + ": " + s.reason + (s.name ? " («" + s.name + "»)" : "")));
  (out.errors || []).forEach(e => lines2.push("строка " + e.line + ": " + e.reason));
  showReport(lines2);
  ta.value = "";
  closeModal();
  fetchState();
}
// ---------- попапы форм добавления (ТЗ §7.3): форма живёт в #modal ----------
// Поля (id) создаются вместе с разметкой попапа и существуют только пока он открыт.
// Enter = primary, Esc закрывает — общий обработчик #modalOverlay (для textarea
// Enter — перенос строки). Кнопки «Создать»/«Отмена» — штатные кнопки openModal.
function openNewGoodPopup() {
  const ft = factoryMode() ? factoryType() : null;
  const catHint = (ft && ft.category_id != null) ? '<div class="hint">категория определяется выбранной фабрикой</div>' : '';
  openModal(
    '<h3>Новый товар</h3>' +
    '<form autocomplete="off" onsubmit="return false">' +
    '<input id="spravNewName" type="text" name="studio_new_good_name" placeholder="имя товара" autocomplete="off">' +
    '<select id="spravNewCat"></select>' + catHint +
    '<label class="descai" title="после создания ИИ предложит описание"><input type="checkbox" id="newGoodDescAI" checked> описание ИИ</label>' +
    '</form>',
    [
      {label: "Создать", cls: "gen", primary: true, id: "btnSpravAdd", action: spravAddGood},
      {label: "Отмена", cls: "", action: closeModal}
    ]
  );
  renderCatSelects(); // наполнить селект + disabled при выбранной фабрике (ТЗ §7.6 п.1)
  $("spravNewName").focus();
}
function openNewResPopup() {
  openModal(
    '<h3>Новый ресурс</h3>' +
    '<form autocomplete="off" onsubmit="return false">' +
    '<input id="spravNewResName" type="text" name="studio_new_res_name" placeholder="имя ресурса" autocomplete="off">' +
    '<select id="spravNewResCat"></select>' +
    '<label class="descai" title="после создания ИИ предложит описание"><input type="checkbox" id="newResDescAI" checked> описание ИИ</label>' +
    '</form>',
    [
      {label: "Создать", cls: "gen", primary: true, id: "btnSpravAddRes", action: spravAddResource},
      {label: "Отмена", cls: "", action: closeModal}
    ]
  );
  renderCatSelects(); // селект ресурсных категорий + disabled «Создать», если их нет
  $("spravNewResName").focus();
}
function openBulkPopup() {
  openModal(
    '<h3>Массовая подгрузка</h3>' +
    '<form autocomplete="off" onsubmit="return false">' +
    '<textarea id="bulkText" rows="8" placeholder="название | категория | описание (необязательно)"></textarea>' +
    '<div class="hint">2 колонки — без описания; 3-я колонка — описание (пустая = без описания; символ «|» в описании допустим)</div>' +
    '</form>',
    [
      {label: "Создать из списка", cls: "gen", primary: true, id: "btnBulk", action: bulkCreate},
      {label: "Отмена", cls: "", action: closeModal}
    ]
  );
  $("bulkText").focus(); // Enter в textarea — перенос строки (исключение в общем keydown)
}
// подсказка об уровне расовости, на котором будет создан тип (ТЗ §7.3 форма 3):
// читает текущие значения фильтра шапки в момент открытия попапа
function prodRaceLevelHint() {
  if (prodRaceLevel === "family") {
    return prodRaceFamily
      ? "тип будет создан на уровне семейства: " + familyLabelById(prodRaceFamily)
      : "семейство не выбрано — выберите его в шапке, иначе тип уйдёт на универсальный уровень";
  }
  if (prodRaceLevel === "race") {
    return "тип будет создан на уровне расы: " + raceName(prodRace) + " (семейство " + familyLabelById(raceFamilyOf(prodRace)) + ")";
  }
  return "тип будет создан на универсальном уровне (без привязки к расе)";
}
// «+ тип» создаёт тип (parent_id NULL): категории у типов нет (спека 2026-09-21
// §1.2 п.4 — категории живут в подтипах, их заводят через узлы-приглашения)
function openProdNewPopup() {
  openModal(
    '<h3>Новый тип постройки</h3>' +
    '<form autocomplete="off" onsubmit="return false">' +
    '<input id="prodNewName" type="text" name="studio_new_prod_name" placeholder="имя типа" autocomplete="off">' +
    '<select id="prodNewKind" title="класс выхода"><option value="goods">товары</option><option value="items">предметы</option><option value="energy">энергия</option></select>' +
    '<select id="prodNewSection" title="раздел постройки — обязателен, «Прочее» не создаётся руками">' +
      PROD_SECTIONS.map(s => '<option value="' + s.key + '"' + (s.key === (PROD_SECTION_KEYS[prodSection] ? prodSection : "colony") ? ' selected' : '') + '>' + esc(s.title) + '</option>').join("") +
    '</select>' +
    '<div class="hint">' + esc(prodRaceLevelHint()) + '</div>' +
    '</form>',
    [
      {label: "Создать", cls: "gen", primary: true, id: "btnProdAdd", action: prodAdd},
      {label: "Отмена", cls: "", action: closeModal}
    ]
  );
  $("prodNewName").focus();
}
// ветка «Предметы» (спека 2026-09-20-фабрики §4.2)
function openItemNewPopup() {
  openModal(
    '<h3>Новый предмет</h3>' +
    '<form autocomplete="off" onsubmit="return false">' +
    '<input id="itemNewName" type="text" name="studio_new_item_name" placeholder="имя предмета" autocomplete="off">' +
    '<select id="itemNewSlot" title="тип слота"><option value="чертёж">чертёж</option><option value="модуль">модуль</option><option value="инструмент">инструмент</option><option value="сертификат">сертификат</option></select>' +
    '</form>',
    [
      {label: "Создать", cls: "gen", primary: true, id: "btnItemAdd", action: itemAdd},
      {label: "Отмена", cls: "", action: closeModal}
    ]
  );
  $("itemNewName").focus();
}
