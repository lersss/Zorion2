"use strict";
// ---------- попап-деталь (99a.3 §10.3) ----------
// Деталь товара — модальное окно поверх канваса; правая панель = только
// справочник. Попап привязан к popupGoodId (отдельно от selected): все
// действия попапа идут в /api/goods/{popupGoodId} (вердикт критика С1).
function openPopup(id) {
  // Ветка «Производители»: узел дерева → попап детали/создания/справка (§2.5).
  // Исключение — режим «Товары» справочника производителей (идея 2026-09-23,
  // шаг 1): карточка каталога открывает товарный попап.
  if (branch === "producers" && spravMode !== "goods") { openProdNodePopup(id); return; }
  popupGoodId = id;
  resetPopupPos();
  $("detailPopup").style.display = "flex";
  $("detailOverlay").style.display = "block";
  renderPopup();
}
// сброс позиции перетаскивания: попап всегда открывается по центру — иначе
// оставленные drag-ом inline left/top уводят его за экран (идея 2026-09-24)
function resetPopupPos() {
  const p = $("detailPopup");
  p.style.left = ""; p.style.top = ""; p.style.transform = "";
}
function closePopup() {
  popupGoodId = null;
  $("detailPopup").style.display = "none";
  $("detailOverlay").style.display = "none";
}
function renderPopup() {
  const el = $("popupBody");
  if (!popupGoodId) return;
  const g = state.goods.find(x => x.id === popupGoodId);
  if (!g) { // товар попапа удалён (в попапе или извне) — закрыть; selected сбросить, если был он (99a.3 §10.3, М3)
    if (selected === popupGoodId) selected = null;
    closePopup();
    return;
  }
  $("popupTitle").textContent = g.name;
  const isRes = g.kind === "resource";
  let html = '';
  // имя: клик — inline-переименование (ресурсы без привилегий, спека iterB §4.5)
  html += '<div class="dname"><span class="rename" onclick="startRename()" title="переименовать">' + esc(g.name) + '</span></div>';
  // шапка-блок (спека 2026-09-23 §5.1): категория, сложность рецепта, источник
  html += '<div class="pblock"><div class="dmeta" style="margin-bottom:0">';
  html += '<select class="dcat" onchange="changeCategory(this.value)">' +
    state.categories.filter(c => c.kind === (isRes ? "resource" : "good")).map(c => '<option value="' + c.id + '"' + (c.id === g.category_id ? ' selected' : '') + '>' + esc(c.name) + '</option>').join("") +
    '</select>';
  // сложность рецепта (recipes.complexity) вместо тир-оверрайда (ТЗ §6.2):
  // пусто = вычисляется; у ресурса рецепта нет — поля нет вовсе
  if (!isRes) {
    if (g.recipe_id) {
      const cx = (g.complexity !== null && g.complexity !== undefined) ? g.complexity : "";
      html += ' · сложность рецепта: <input id="complexityInput" type="number" min="0" style="width:56px" value="' + cx + '" onchange="setComplexity(this.value)" title="сложность рецепта; пусто = вычисляется по графу"> (вычислен ' + g.tier_computed + ')';
      if (cx !== "" && Number(cx) !== g.tier_computed) {
        html += '<div class="tier-warn">⚠ отличается от вычисленного (' + g.tier_computed + ')</div>';
      }
    } else {
      html += ' · <span class="hint">рецепт не создан</span>';
    }
  }
  html += (g.source === "ai" ? ' · ИИ' : g.source === "import" ? ' · импорт' : '');
  html += '</div>';
  html += codeNote(g.code); // метка переноса — справочно, только чтение (§3.3)
  html += '</div>';
  // Состав рецепта (спека §5.2) — только у товаров (ресурс — лист, слотов нет):
  // заполненный и пустой — оба drop-target; обработчики не меняются
  if (!isRes) {
    html += '<div class="pblock"><div class="pblock-h">Состав рецепта</div>';
    (g.recipe || []).forEach((s, i) => {
      if (s.good_id) {
        html += '<div class="slot" ondragover="slotDragOver(event,this)" ondragleave="slotDragLeave(this)" ondrop="dropOnSlot(event,' + i + ')">' +
                '<button onclick="clearSlot(' + i + ')" title="исключить из рецепта" aria-label="исключить из рецепта">×</button>' +
                '<button class="delslot" onclick="delSlot(' + i + ')" title="удалить слот целиком" aria-label="удалить слот">🗑</button>' +
                '<span class="sname">' + esc(s.name || s.good_id) + '</span>' +
                ' <span style="color:#777">тир ' + s.tier + '</span>' +
                ' <span style="color:#999">×</span> <input type="number" min="1" style="width:56px" value="' + (s.quantity || 1) + '" onchange="setQuantity(' + i + ', this.value)" title="количество единиц составляющей">' +
                (s.reason ? '<div class="sreason">' + esc(s.reason) + '</div>' : '') + '</div>';
      } else {
        html += '<div class="slot empty" ondragover="slotDragOver(event,this)" ondragleave="slotDragLeave(this)" ondrop="dropOnSlot(event,' + i + ')">' +
                '<button class="delslot" onclick="delSlot(' + i + ')" title="удалить слот целиком" aria-label="удалить слот">🗑</button>' +
                'пусто — перетащите сюда карточку' +
                '<label class="allowres" title="разрешить ресурс в этом компоненте (для ИИ): с галкой ИИ может предложить ресурс; без галки — только товары"><input type="checkbox" onchange="setAllowResource(' + i + ', this.checked)"' + (s.allow_resource ? ' checked' : '') + '> разрешить ресурс в этом компоненте (для ИИ)</label></div>';
      }
    });
    html += '</div>';
  }
  // Объём и вес (спека §5.3) — данные каталога; значение есть всегда (Р2),
  // дефолт 1, пустой ввод = 1
  html += '<div class="pblock"><div class="pblock-h">Объём и вес</div>' +
    '<div class="dmeta" style="margin-bottom:0">объём: <input id="volInput" type="number" min="0" step="any" style="width:80px" value="' + (g.volume !== null && g.volume !== undefined ? g.volume : 1) + '" onchange="setVolumeWeight(this.value, null)" title="объём единицы товара">' +
    ' · вес: <input id="wgtInput" type="number" min="0" step="any" style="width:80px" value="' + (g.weight !== null && g.weight !== undefined ? g.weight : 1) + '" onchange="setVolumeWeight(null, this.value)" title="вес единицы товара"></div></div>';
  // Описание каталога (спека §5.4): многострочное, пусто = нет; автосохранение
  // по потере фокуса; защита несохранённого — как есть
  html += '<div class="pblock"><div class="pblock-h">Описание</div>' +
    '<textarea id="descInput" rows="4" style="width:100%;box-sizing:border-box;min-height:90px" placeholder="описание — увидят игроки; пусто = описания нет" onchange="setDescription(this.value)">' + esc(g.description || "") + '</textarea></div>';
  // кнопки (ресурсы — без слотов/fill; скрытия у товара нет — только «удалить»)
  html += '<div class="btns">';
  html += '<button onclick="askAddParent()" title="Добавить этот товар составляющей в рецепт другого товара (новый слот)">добавить родителя</button>';
  if (!isRes) {
    const hasEmpty = (g.recipe || []).some(s => !s.good_id);
    const fillDisabled = state.generating || !hasEmpty || !aiRunning();
    const fillTitle = state.generating ? "идёт генерация" : (!hasEmpty ? "Сначала добавьте слот («+ слот»)" : "Сначала запустите помощника");
    html += '<button class="gen" onclick="addSlot()">+ слот</button>';
    html += '<button class="gen" onclick="fillGood()"' + (fillDisabled ? ' disabled title="' + fillTitle + '"' : '') + '>заполнить комплектующие</button>';
  }
  const descDisabled = state.generating || !aiRunning();
  html += '<button class="gen" onclick="describeGood()"' + (descDisabled ? ' disabled title="' + (state.generating ? "идёт генерация" : "Сначала запустите помощника") + '"' : ' title="ИИ предложит описание (перезапишет существующее)"') + '>предложить описание</button>';
  html += '<button class="del" onclick="askDeleteGood()">удалить</button>';
  html += '</div>';
  // «Доступен постройкам» и «Используется в» — после кнопок (спека §5, прим.)
  if (!isRes) html += renderBindSection(g);
  // «Используется в:» — клик по родителю = выбор родителя + попап на нём (99a.3 §10.3)
  const parents = state.goods.filter(p => (p.recipe || []).some(s => s.good_id === g.id));
  html += '<div class="pblock"><div class="pblock-h">Используется в</div>';
  if (parents.length) {
    html += parents.map(p => '<div class="usedin-item" onclick="openParent(\'' + p.id + '\')">' + esc(p.name) + ' <span style="color:#777">· тир ' + p.tier + '</span></div>').join("");
  } else {
    html += '<div class="empty-note">нигде не используется</div>';
  }
  html += '</div>';
  el.innerHTML = html;
}
// renderBindSection — секция «Доступен постройкам:» попапа товара (ТЗ §6.4):
// список привязок (bound_factories) с «− отвязать» + селект любых записей-
// подтипов (исключая уже привязанные) с «+ привязать» (решение создателя
// 2026-09-23: категория и kind постройки список не сужают).
function renderBindSection(g) {
  const ft = factoryMode() ? factoryType() : null;
  const bound = g.bound_factories || [];
  let html = '<div class="pblock"><div class="pblock-h">Доступен постройкам</div>';
  if (bound.length) {
    bound.forEach(fid => {
      const p = state.producer_types.find(x => x.id === fid);
      if (!p) return;
      const inset = ft && ft.id === fid;
      // имя постройки первым — строки-тёзки под одним родителем различимы;
      // категория — фрагмент только если она есть (подтип без товарной
      // категории, напр. «Городок · Поселение · универсальная», без «null»)
      const catPart = p.category_id != null ? esc(p.category_name || catName(p.category_id)) + ' · ' : '';
      html += '<div class="bind-row">' +
        esc(p.name) + ' · ' + (p.parent_name ? esc(p.parent_name) + ' · ' : '') + catPart + esc(factoryLevelLabel(p)) +
        (inset ? ' <span class="badge b-inset" title="в наборе выбранной фабрики">в наборе</span>' : '') +
        '<button class="bind-del" onclick="unbindFactory(' + fid + ')" title="отвязать рецепт от постройки">− отвязать</button>' +
        '</div>';
    });
  } else {
    html += '<div class="empty-note">не привязан ни к одной постройке</div>';
  }
  html += '</div>';
  const avail = state.producer_types.filter(p =>
    p.parent_id && !bound.includes(p.id));
  html += '<div class="bind-add">';
  if (avail.length) {
    const groups = {}, order = [];
    avail.forEach(p => {
      const key = p.parent_name || "—";
      if (!groups[key]) { groups[key] = []; order.push(key); }
      groups[key].push(p);
    });
    html += '<select id="bindFactorySel">' + order.map(k =>
      '<optgroup label="' + esc(k) + '">' + groups[k].map(p =>
        '<option value="' + p.id + '">' + esc(p.name) + ' — ' + esc(factoryLevelLabel(p)) + '</option>').join("") + '</optgroup>').join("") + '</select>';
    html += '<button class="gen" onclick="bindFactory()">+ привязать</button>';
  } else {
    const anyBuilding = state.producer_types.some(p => p.parent_id);
    html += '<span class="hint">' + (anyBuilding
      ? 'все постройки уже привязаны'
      : 'Построек нет — раздел «Постройки»') + '</span>';
  }
  html += '</div>';
  return html;
}
// bindFactory — «+ привязать» из секции попапа (ТЗ §6.4).
async function bindFactory() {
  const sel = $("bindFactorySel");
  const g = state.goods.find(x => x.id === popupGoodId);
  if (!sel || !sel.value || !g) return;
  await bindRecipeToFactory(g, Number(sel.value));
}
// unbindFactory — «− отвязать» (DELETE /producers/{id}/recipes/{recipe_id}).
async function unbindFactory(fid) {
  const g = state.goods.find(x => x.id === popupGoodId);
  if (!g || !g.recipe_id) return;
  const r = await api("DELETE", "/studio/api/producers/" + fid + "/recipes/" + g.recipe_id);
  if (r) fetchState();
}
function openParent(id) {
  selected = id;
  openPopup(id);
  renderAll();
  if (!fullTreeMode()) centerOn(id);
}
function selectGood(id) { selected = id; renderAll(); }
function startRename() {
  const g = state.goods.find(x => x.id === popupGoodId);
  if (!g) return;
  const el = $("popupBody").querySelector(".dname");
  el.innerHTML = '<input id="renameInput" type="text" value="' + esc(g.name) + '" style="width:100%;box-sizing:border-box">';
  const inp = $("renameInput");
  inp.focus(); inp.select();
  inp.addEventListener("keydown", ev => {
    if (ev.key === "Enter") commitRename();
    if (ev.key === "Escape") renderPopup();
  });
  inp.addEventListener("blur", commitRename);
}
async function commitRename() {
  const inp = $("renameInput");
  if (!inp) return;
  const name = inp.value.trim();
  if (!name) { renderPopup(); return; }
  const r = await api("PUT", "/studio/api/goods/" + popupGoodId, {name});
  if (r) fetchState(); else renderPopup();
}
async function changeCategory(catId) {
  const r = await api("PUT", "/studio/api/goods/" + popupGoodId, {category_id: Number(catId)});
  if (r) fetchState();
}
async function api(method, url, body) {
  let r;
  try {
    r = await studioFetch(url, {method, headers: {"Content-Type": "application/json"}, body: body ? JSON.stringify(body) : undefined});
  } catch (e) {
    return null; // 401 — оверлей уже показан (studioFetch)
  }
  if (!r.ok) {
    let msg = "ошибка " + r.status;
    try { const e = await r.json(); if (e.error) msg = e.error; } catch (e) {}
    showReport([msg], true);
    return null;
  }
  return r;
}
// popupRecipeId — recipe_id товара попапа (адресация состава рецепта, ТЗ §6.3).
function popupRecipeId() {
  const g = state.goods.find(x => x.id === popupGoodId);
  return g ? g.recipe_id : 0;
}
// Состав рецепта адресуется по recipe_id товара, не по good_id (спека рецептов §5)
async function addSlot() { const rid = popupRecipeId(); if (!rid) return; await api("POST", "/studio/api/recipes/" + rid + "/components"); fetchState(); }
async function clearSlot(i) { const rid = popupRecipeId(); if (!rid) return; await api("DELETE", "/studio/api/recipes/" + rid + "/components/" + i + "/component"); fetchState(); }
async function delSlot(i) { const rid = popupRecipeId(); if (!rid) return; await api("DELETE", "/studio/api/recipes/" + rid + "/components/" + i); fetchState(); }
async function setAllowResource(i, checked) {
  const rid = popupRecipeId(); if (!rid) return;
  const r = await api("PUT", "/studio/api/recipes/" + rid + "/components/" + i + "/allow_resource", {allow_resource: checked});
  if (r) fetchState();
}
// количество единиц составляющей в компоненте (99a.2 §6.5): change → PUT {quantity}
async function setQuantity(i, val) {
  const q = parseInt(val, 10);
  if (!q || q < 1) { renderPopup(); return; }
  const rid = popupRecipeId(); if (!rid) return;
  const r = await api("PUT", "/studio/api/recipes/" + rid + "/components/" + i, {quantity: q});
  if (r) fetchState(); else renderPopup();
}
// сложность рецепта (ТЗ §6.2): ввод числа → PUT /recipes/{id} {complexity};
// очистка → {complexity: null} (вычисляемая по графу)
async function setComplexity(val) {
  const g = state.goods.find(x => x.id === popupGoodId);
  if (!g || !g.recipe_id) return;
  const v = String(val).trim();
  const body = v === "" ? {complexity: null} : {complexity: parseInt(v, 10)};
  const r = await api("PUT", "/studio/api/recipes/" + g.recipe_id, body);
  if (r) fetchState(); else renderPopup();
}
// объём/вес (спека 2026-09-20-фабрики §3.1, 3b.6.4; Р2 2026-09-21): ввод
// числа → PUT; пустой ввод трактуется как 1 («значение есть всегда»)
async function setVolumeWeight(vol, wgt) {
  const body = {};
  if (vol !== null) {
    const v = String(vol).trim();
    body.volume = v === "" ? 1 : parseFloat(v);
  }
  if (wgt !== null) {
    const v = String(wgt).trim();
    body.weight = v === "" ? 1 : parseFloat(v);
  }
  const r = await api("PUT", "/studio/api/goods/" + popupGoodId, body);
  if (r) fetchState(); else renderPopup();
}
function slotDragOver(ev, el) { ev.preventDefault(); el.classList.add("drop-target"); }
function slotDragLeave(el) { el.classList.remove("drop-target"); }
async function dropOnSlot(ev, i) {
  ev.preventDefault();
  ev.currentTarget.classList.remove("drop-target");
  const goodId = ev.dataTransfer.getData("text/plain");
  if (!goodId) return;
  const rid = popupRecipeId(); if (!rid) return;
  // good_id — число (сервер парсит *int64; строка → 400, вердикт критика С2)
  const r = await api("PUT", "/studio/api/recipes/" + rid + "/components/" + i, {good_id: Number(goodId)});
  if (r) fetchState();
}
