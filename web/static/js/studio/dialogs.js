"use strict";
// ---------- модалки ----------
function openModal(html, buttons) {
  $("modalBody").innerHTML = html;
  $("modalBtns").innerHTML = buttons.map((b, i) =>
    '<button class="' + b.cls + '" data-idx="' + i + '"' + (b.primary ? ' data-primary="1"' : '') + (b.id ? ' id="' + b.id + '"' : '') + '>' + esc(b.label) + '</button>').join("");
  $("modalBtns").querySelectorAll("button").forEach(btn => {
    btn.addEventListener("click", () => buttons[+btn.dataset.idx].action());
  });
  $("modalOverlay").style.display = "flex";
  const primary = $("modalBtns").querySelector("button[data-primary]");
  if (primary) primary.focus();
}
function closeModal() { $("modalOverlay").style.display = "none"; }
document.addEventListener("keydown", ev => {
  if ($("modalOverlay").style.display !== "flex") return;
  if (ev.key === "Escape") closeModal();
  // Enter = primary, кроме textarea (там перенос строки — форма подгрузки, §7.3)
  if (ev.key === "Enter" && ev.target.tagName !== "TEXTAREA") {
    const primary = $("modalBtns").querySelector("button[data-primary]");
    if (primary) primary.click();
  }
});
async function askDeleteGood() {
  const g = state.goods.find(x => x.id === popupGoodId);
  if (!g) return;
  const n = state.goods.reduce((acc, p) => acc + (p.recipe || []).filter(s => s.good_id === g.id).length, 0);
  const dep = await depositsCountForDelete(g.id);
  openModal(
    '<h3>Удалить товар</h3>' +
    '<div>Удалить товар «' + esc(g.name) + '»?</div>' +
    depositWarningLine(dep),
    [
      {label: "Удалить", cls: "del", primary: true, action: () => confirmDeleteGood(g, n, dep)},
      {label: "Отмена", cls: "", action: closeModal}
    ]
  );
}
// второе подтверждение при N > 0 (спека iterB §4.7): слоты других товаров,
// ссылающиеся на удаляемого, очистятся — рецепты будут испорчены
function confirmDeleteGood(g, n, dep) {
  if (n > 0) {
    openModal(
      '<h3>Удалить товар</h3>' +
      '<div>Слоты, ссылающиеся на него (' + n + '), очистятся — рецепты этих товаров будут испорчены. Удалить всё равно?</div>' +
      depositWarningLine(dep),
      [
        {label: "Удалить и очистить " + n + " ссылок", cls: "del", primary: true, action: doDeleteGood},
        {label: "Отмена", cls: "", action: closeModal}
      ]
    );
    return;
  }
  doDeleteGood();
}
async function doDeleteGood() {
  closeModal();
  const r = await api("DELETE", "/studio/api/goods/" + popupGoodId);
  if (r) {
    // фактический счётчик — из ответа (источник правды; K может отличаться
    // от N при гонке/другой сессии между расчётом и ответом, §4.7)
    const out = await r.json();
    const k = out && out.cleared_links !== undefined ? out.cleared_links : 0;
    showReport([(k > 0 ? "⚠ " : "") + "Удалено. Очищено ссылок: " + k]);
  }
  if (selected === popupGoodId) selected = null; // удалённый был выбран — сброс (99a.3 §10.3, М3)
  closePopup();
  if (r) fetchState();
}

// ---------- «добавить родителя» (99a Пакет 2, п.6) ----------
// Обратное «Используется в:»: выбранный товар становится составляющей
// другого товара (новый слот у родителя). Два вызова: POST слот → PUT
// заполнить; при 409 (цикл) — откат: только что добавленный слот удаляется.
function askAddParent() {
  const g = state.goods.find(x => x.id === popupGoodId);
  if (!g) return;
  // родитель — товар с рецептом (не ресурс); сам себя исключаем (цикл).
  // Уже-родители не исключаются — добавится ещё один слот.
  const candidates = state.goods.filter(p => p.kind !== "resource" && p.id !== popupGoodId);
  const html = '<h3>Добавить родителя</h3>' +
    '<div style="font-size:12px;color:#999;margin-bottom:6px">«' + esc(g.name) + '» станет составляющей выбранного товара (новый слот).</div>' +
    '<div class="wlist">' + (candidates.length ? candidates.map(p =>
      '<div class="pline" onclick="addParent(\'' + p.id + '\')">' + esc(p.name) +
      ' <span style="color:#777">· ' + esc(catName(p.category_id)) + ' · тир ' + p.tier + '</span></div>'
    ).join("") : '<div class="empty-note">нет товаров-кандидатов</div>') + '</div>';
  openModal(html, [{label: "Отмена", cls: "", action: closeModal}]);
}
async function addParent(parentId) {
  closeModal();
  // Номер нового компонента — из текущего состояния, НЕ из ответа POST: контракт
  // §5 возвращает {"id": id} без рецепта; POST добавляет компонент в конец
  // (pos = MAX(pos)+1, репозиторий AddRecipeComponent).
  const parent = state.goods.find(x => x.id === parentId);
  if (!parent) return;
  const rid = parent.recipe_id;
  if (!rid) { showReport(["рецепт не создан"]); return; }
  const n = parent.recipe.length;
  const r = await api("POST", "/studio/api/recipes/" + rid + "/components");
  if (!r) return;
  const r2 = await api("PUT", "/studio/api/recipes/" + rid + "/components/" + n, {good_id: Number(popupGoodId)});
  if (!r2) {
    // откат: цикл 409 и т.п. — убрать только что добавленный пустой компонент
    await api("DELETE", "/studio/api/recipes/" + rid + "/components/" + n);
  }
  fetchState();
}

// ---------- справочник (99a.3 §8) ----------
// Единый список всех товаров + ресурсов из state.goods; поиск по имени,
// галки-фильтры (ортогональны, по умолчанию выключены), карточки draggable
// (источник drag&drop в слоты попапа), клик = выбор, двойной клик / ⓘ = попап.
function renderSprav() {
  if (branch === "producers" && spravMode !== "goods") { renderProdSprav(); return; }
  if (branch === "items") { renderItemSprav(); return; }
  renderGoodsSprav();
}
// renderGoodsSprav — тело товарного справочника (вкладка «Товары» и режим
// «Товары» на «Производителях», идея 2026-09-23 шаг 1): один общий код, чтобы
// списки не разъезжались.
function renderGoodsSprav() {
  const sc = $("spravList").scrollTop; // сохранить scroll списка при пересборке (поиск/пилюли/галки)
  const q = $("spravSearch").value.trim().toLowerCase();
  // фильтр по категории (пусто = все); при старте renderSprav идёт до renderCatSelects —
  // опции ещё не заполнены, берём сохранённое значение из localStorage
  const cat = $("spravCat").value || localStorage.getItem("gs_spravCat") || "";
  const used = {};
  state.goods.forEach(g => (g.recipe || []).forEach(s => { if (s.good_id) used[s.good_id] = true; }));
  const fUsed = $("fUsed").checked, fUnused = $("fUnused").checked;
  // Семантика фильтров (создатель 2026-09-19): показ включается ТОЛЬКО при «тир + галка»:
  // выбран хотя бы один тир И хотя бы одна галка. Галки — группы-РАСШИРЕНИЯ (OR):
  // каждая включённая добавляет свою группу («используемые»/«неиспользуемые»),
  // выключенная — её группа скрыта. Категория и поиск только сужают (AND), сами показ не включают.
  const hasTier = tierSel.size > 0;
  const hasFlag = fUsed || fUnused;
  const famAllowed = familyGoodsFilter(); // фильтр семейства (ТЗ §4), null = выключен
  // пилюли тиров: «все» первой + фиксированные 0..8 + тиры выше 8 из каталога (если есть)
  const tiers = spravTiers();
  // «все» активна, только когда выбраны ВСЕ тиры (показывается всё); при пустом
  // выборе фильтр тиров выключен — список пуст (создатель 2026-09-19)
  const allOn = tierSel.size === tiers.length;
  $("tierFilters").innerHTML =
    '<label class="pill' + (allOn ? ' on' : '') + '" title="все тиры">' +
    '<input type="checkbox" style="display:none"' + (allOn ? ' checked' : '') + ' onchange="toggleAllTiers()">все</label>' +
    tiers.map(t =>
      '<label class="pill' + (tierSel.has(t) ? ' on' : '') + '"' + (t === 0 ? ' title="тир 0"' : '') + '>' +
      '<input type="checkbox" style="display:none"' + (tierSel.has(t) ? ' checked' : '') + ' onchange="toggleTier(' + t + ', this.checked)">' + t + '</label>'
    ).join("");
  const list = state.goods.filter(g => {
    if (!hasTier || !hasFlag) return false;               // тир + галка обязательны (создатель 2026-09-19)
    if (!tierSel.has(g.tier)) return false;               // выбранные тиры — только они
    // галки — РАСШИРЕНИЕ (OR): каждая включает свою группу; выключенная — её группа
    // скрыта (создатель 2026-09-19). Группы «забаненные» больше нет (2026-09-21).
    const isUsed = !!used[g.id];
    if (isUsed) { if (!fUsed) return false; }
    else { if (!fUnused) return false; }
    if (cat && String(g.category_id) !== cat) return false;          // AND: фильтр по категории (сужает, не включает показ)
    if (g.kind !== "resource" && famAllowed && !famAllowed.has(g.id)) return false; // фильтр семейства (ТЗ §4 п.5)
    if (q && !g.name.toLowerCase().includes(q)) return false; // AND: поиск (сужает, не включает показ)
    return true;
  });
  $("spravTitle").textContent = "Справочник · " + list.length;
  const ft = factoryMode() ? factoryType() : null;
  $("spravList").innerHTML = list.map(g => {
    // бейджи рецепт-скопа (ТЗ §6.5): «не привязан» / «в наборе» выбранной фабрики
    const bound = g.bound_factories || [];
    let badge = '';
    if (g.kind === "good") {
      if (bound.length === 0) badge = ' <span class="badge b-unbound" title="рецепт не привязан ни к одной постройке">не привязан</span>';
      else if (ft && bound.includes(ft.id)) badge = ' <span class="badge b-inset" title="в наборе выбранной фабрики">в наборе</span>';
    }
    return '<div class="card" draggable="true" ondragstart="dragStart(event,\'' + g.id + '\')" onclick="spravClick(\'' + g.id + '\')" ondblclick="spravDblClick(\'' + g.id + '\')" oncontextmenu="showSpravCtxMenu(event,\'' + g.id + '\');return false">' +
    '<div class="nm">' + esc(g.name) + '<button class="info" onclick="event.stopPropagation();openPopup(\'' + g.id + '\')" title="деталь (без смены выбора)">ⓘ</button></div>' +
    '<div class="meta">' + esc(catName(g.category_id)) + ' · тир ' + g.tier + badge + '</div>' +
    '</div>';
  }).join("") || '<div class="empty-note">' + (hasTier && hasFlag ? 'ничего не найдено' : 'ничего не выбрано — выберите тир и хотя бы одну галку') + '</div>';
  $("spravList").scrollTop = sc;
}
