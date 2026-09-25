"use strict";
// ---------- скоп ветки «Товары»: фабрика + семейство (ТЗ §3–§5) ----------
// factoryType — выбранная конкретная фабрика (или null = обзор).
function factoryType() {
  if (!goodsFactory) return null;
  return state.producer_types.find(p => String(p.id) === String(goodsFactory)) || null;
}
// factoryMode — ветка «Товары» И выбрана конкретная фабрика (ТЗ §5.2).
function factoryMode() { return branch === "goods" && !!factoryType(); }
// concreteGoodsFactories — конкретные фабрики kind=goods с товарной категорией (ТЗ §3).
function concreteGoodsFactories() {
  return state.producer_types.filter(p =>
    p.parent_id && p.kind === "goods" && p.category_id != null &&
    (state.categories.find(c => c.id === p.category_id) || {}).kind === "good");
}
// factoryLevelLabel — подпись уровня расовости фабрики (ТЗ §3).
function factoryLevelLabel(p) {
  if (p.race) return "Раса: " + raceName(p.race) + (p.race_family ? " (" + familyLabelById(p.race_family) + ")" : "");
  if (p.race_family) return familyLabelById(p.race_family);
  return "универсальная";
}
function factoryStepRank(p) { return p.race ? 2 : p.race_family ? 1 : 0; }
// factoryRecipeIds — recipe_id, привязанные к фабрике (producer_recipes).
function factoryRecipeIds(ptId) {
  return (state.producer_recipes || []).filter(pr => String(pr.producer_type_id) === String(ptId)).map(pr => pr.recipe_id);
}
// factoryClosureGoodIds — замыкание вниз от выходов рецептов фабрики (ТЗ §5.2).
function factoryClosureGoodIds(ptId) {
  const byId = {};
  state.goods.forEach(g => byId[g.id] = g);
  const roots = factoryRecipeIds(ptId)
    .map(rid => state.goods.find(g => g.recipe_id === rid))
    .filter(Boolean).map(g => g.id);
  const out = new Set();
  const stack = [...roots];
  while (stack.length) {
    const id = stack.pop();
    if (out.has(id)) continue;
    out.add(id);
    const g = byId[id];
    if (!g || g.kind === "resource") continue;
    (g.recipe || []).forEach(s => { if (s.good_id) stack.push(s.good_id); });
  }
  return out;
}
// universalSources — универсальные конкретные фабрики категории, кроме exceptId (ТЗ §13.2).
function universalSources(categoryId, exceptId) {
  return state.producer_types.filter(p =>
    p.parent_id && p.kind === "goods" && p.category_id === categoryId &&
    !p.race_family && !p.race && String(p.id) !== String(exceptId));
}
// mostSpecificFactories — записи самой конкретной ступени: race > family > universal.
function mostSpecificFactories(list) {
  if (list.some(p => p.race)) return list.filter(p => p.race);
  if (list.some(p => p.race_family)) return list.filter(p => p.race_family && !p.race);
  return list.filter(p => !p.race_family && !p.race);
}
function mostSpecificSlot(list) {
  if (list.some(s => s.race)) return list.filter(s => s.race)[0];
  if (list.some(s => s.race_family)) return list.filter(s => s.race_family && !s.race)[0];
  return list.filter(s => !s.race_family && !s.race)[0];
}
// familyAppliedFactories — применяемые к выбранному семейству фабрики категории (ТЗ §4 п.1).
function familyAppliedFactories(catId) {
  return state.producer_types.filter(p =>
    p.parent_id && p.kind === "goods" && p.category_id === catId &&
    (!p.race_family || (p.race_family === goodsFamily && !p.race)));
}
// familyCatVisible — видима ли товарная категория выбранному семейству (обе оси, ТЗ §4 п.2–3).
function familyCatVisible(catId) {
  if (!goodsFamily) return true;
  const applied = familyAppliedFactories(catId);
  if (!applied.length) return false;
  const most = mostSpecificFactories(applied);
  if (most.some(p => p.hidden)) return false; // ось 1: скрытость записи-фабрики
  const parentIds = new Set(most.map(p => p.parent_id));
  const slots = (state.producer_slots || []).filter(s =>
    parentIds.has(s.parent_id) && s.category_id === catId &&
    (!s.race_family || (s.race_family === goodsFamily && !s.race)));
  if (slots.length) {
    const mostSlot = mostSpecificSlot(slots);
    if (mostSlot && mostSlot.hidden) return false; // ось 2: скрытость слота родителя
  }
  return true;
}
// familyGoodsFilter — множество товаров, чей рецепт привязан к фабрике, которую
// получает выбранное семейство (ТЗ §4 п.5); null = фильтр выключен. Ресурсы
// фильтр не сужает (они — общий пул и листья графа, ТЗ §4 «Показываются товары»).
function familyGoodsFilter() {
  if (!goodsFamily) return null;
  const allowed = new Set();
  state.categories.filter(c => c.kind === "good").forEach(c => {
    if (!familyCatVisible(c.id)) return;
    const mostIds = new Set(mostSpecificFactories(familyAppliedFactories(c.id)).map(p => p.id));
    state.goods.forEach(g => {
      if ((g.bound_factories || []).some(fid => mostIds.has(fid))) allowed.add(g.id);
    });
  });
  return allowed;
}
// isOtherFactoryComponent — компонент графа фабрики, произведённый ДРУГОЙ
// фабрикой (ТЗ §5.3): бейдж «др. фабрика» и строка тултипа.
function isOtherFactoryComponent(g) {
  if (!factoryMode()) return false;
  const ft = factoryType();
  if (factoryRecipeIds(ft.id).includes(g.recipe_id)) return false; // корень набора
  const bf = g.bound_factories || [];
  return bf.length > 0 && !bf.includes(ft.id);
}

// visibleGoods: ресурсы (нижний ряд) видны всегда — фильтр категории их не
// скрывает (спека 99a.1 §10.6), поиск фильтрует всех. Только режим «всё дерево».
// Мемоизация до смены состояния/фильтров: hitTest зовёт её на каждом mousemove,
// а внутри — замыкание фабрики и фильтр семейства (ТЗ §7, перф). Состояние —
// новые массивы на каждый опрос (state = await r.json()), поэтому ключ по
// ссылкам корректен; in-place мутаций state.* в файле нет.
let visibleGoodsCache = null;
function visibleGoods() {
  const cat = $("catFilter").value, q = $("search").value.trim().toLowerCase();
  const showRes = $("showResources").checked;
  const key = [state.goods, state.categories, state.producer_types, state.producer_slots,
    state.producer_recipes, cat, q, showRes, goodsFactory, goodsFamily, branch];
  if (visibleGoodsCache && visibleGoodsCache.key.length === key.length &&
      visibleGoodsCache.key.every((v, i) => v === key[i])) return visibleGoodsCache.val;
  const famAllowed = familyGoodsFilter(); // фильтр семейства (ТЗ §4), null = выключен
  const factoryIds = factoryMode() ? factoryClosureGoodIds(factoryType().id) : null; // замыкание фабрики (ТЗ §5.2)
  const result = state.goods.filter(g => {
    if (factoryIds && !factoryIds.has(g.id)) return false; // режим фабрики: только её замыкание вниз
    if (g.kind === "resource" && !showRes) return false; // ресурсы скрыты по умолчанию
    if (cat && String(g.category_id) !== cat && g.kind !== "resource") return false;
    if (g.kind !== "resource" && famAllowed && !famAllowed.has(g.id)) return false; // семейство — товары по привязке рецепта
    if (q && !g.name.toLowerCase().includes(q)) return false;
    return true;
  });
  visibleGoodsCache = {key, val: result};
  return result;
}
// depthOf — «глубина от верхушки» (сверху вниз): верхушка (никем не
// используется как составляющая) = 0, её дочки = 1, и т.д. Раскладка идёт
// по глубине, а не по тиру: слот (пустой/заполненный) всегда под своим
// родителем (решение создателя 2026-09-19). Ресурсы — как обычные дочки.
// scope — множество товаров, в пределах которого считается глубина
// (фокус-подграф: родители выбранного = корни 0, выбранный = 1, дети = 2,
// плейсхолдеры = 3; 99a.3 §5.2).
function depthOf(g, byId, memo, scope) {
  if (memo[g.id] !== undefined) return memo[g.id];
  let parentDepth = -1;
  (scope || state.goods).forEach(p => {
    if ((p.recipe || []).some(s => s.good_id === g.id)) {
      const d = depthOf(p, byId, memo, scope);
      if (d > parentDepth) parentDepth = d;
    }
  });
  memo[g.id] = parentDepth < 0 ? 0 : parentDepth + 1;
  return memo[g.id];
}
// computeLayout — древесная раскладка по глубине (верхушки сверху, вниз до
// ресурсов). Пустые слоты = виртуальные карточки-плейсхолдеры того же
// размера на уровень ниже родителя. Классика: ширина поддерева = сумма
// ширины детей, родитель центрируется над своими детьми, соседние поддеревья
// раздвигаются — слот приёмопередатчика не оказывается под вычислительным
// блоком (решение создателя 2026-09-19).
// Параметризация (99a.3 §5.2): subset — множество товаров раскладки
// (фокус-подграф или null = visibleGoods()); placeholderFor — id товара,
// для которого рисуются плейсхолдеры пустых слотов (фокус: только выбранный;
// null = все товары множества).
function computeLayout(subset, placeholderFor) {
  const byId = {};
  state.goods.forEach(g => byId[g.id] = g);
  const depthMemo = {}; // отдельный memo для depthOf (НЕ для tierOf!)
  const goods = subset || visibleGoods();
  const vis = {};
  goods.forEach(g => vis[g.id] = true);
  // виртуальные плейсхолдеры пустых слотов (глубина = глубина родителя + 1)
  const slots = [];
  goods.forEach(g => {
    if (g.kind === "resource") return;
    if (placeholderFor && g.id !== placeholderFor) return; // фокус: только пустые слоты выбранного
    const d = depthOf(g, byId, depthMemo, subset);
    (g.recipe || []).forEach((s, i) => {
      if (s.good_id) return;
      slots.push({id: "__slot:" + g.id + ":" + i, parentId: g.id, slotIndex: i, depth: d + 1, allowResource: !!s.allow_resource});
    });
  });
  const depthOfGood = {};
  goods.forEach(g => { depthOfGood[g.id] = depthOf(g, byId, depthMemo, subset); });
  const maxDepth = Math.max(0, ...Object.values(depthOfGood), ...slots.map(s => s.depth));

  // дети узла (товар или плейсхолдер): на глубине на 1 ниже
  const childMap = {}; // id -> [childId]
  const nodeDepth = {}; // id -> depth (для товаров и слотов)
  goods.forEach(g => nodeDepth[g.id] = depthOfGood[g.id]);
  slots.forEach(s => nodeDepth[s.id] = s.depth);
  function childrenOf(id) {
    if (childMap[id]) return childMap[id];
    const d = nodeDepth[id];
    const kids = [];
    // для товара: его составляющие из рецепта (дочки на глубине d+1)
    const g = byId[id];
    if (g && g.kind !== "resource") {
      (g.recipe || []).forEach(s => {
        if (!s.good_id) return;
        const cg = byId[s.good_id];
        if (cg && nodeDepth[cg.id] === d + 1) kids.push(cg.id);
      });
    }
    // плейсхолдеры пустых слотов этого родителя
    slots.forEach(s => { if (s.parentId === id && s.depth === d + 1) kids.push(s.id); });
    childMap[id] = kids;
    return kids;
  }
  // ячейки по рядам (товары + слоты), глубины 0..maxDepth
  const rowsAt = t => {
    const out = [];
    goods.forEach(g => { if (depthOfGood[g.id] === t) out.push({type: "good", id: g.id}); });
    slots.forEach(s => { if (s.depth === t) out.push({type: "slot", id: s.id, parentId: s.parentId}); });
    return out;
  };
  // ширина поддерева в «колонках»: 1, если детей нет; иначе сумма детей
  const colMemo = {};
  function colsOf(id) {
    if (colMemo[id] !== undefined) return colMemo[id];
    const kids = childrenOf(id);
    if (!kids.length) { colMemo[id] = 1; return 1; }
    let s = 0;
    kids.forEach(k => s += colsOf(k));
    colMemo[id] = s;
    return s;
  }
  // раскладка: place(id, x) ставит узел и рекурсивно детей; возвращает новую x
  const pos = {};
  let maxCols = 1;
  const rowH = CARD_H + ROW_GAP;
  function place(id, x, depth) {
    const blockW = colsOf(id) * (CARD_W + GAP_X) - GAP_X;
    pos[id] = {x: x + (blockW - CARD_W) / 2, y: PAD + depth * rowH};
    maxCols = Math.max(maxCols, x + blockW + GAP_X);
    const kids = childrenOf(id);
    let cx = x;
    kids.forEach(k => {
      place(k, cx, depth + 1);
      cx += colsOf(k) * (CARD_W + GAP_X);
    });
  }
  // корни: товары глубины 0 (верхушки); ресурсы без родителей — тоже корни
  const roots = goods.filter(g => depthOfGood[g.id] === 0);
  let x = PAD;
  const placedRoots = {};
  roots.forEach(r => {
    if (placedRoots[r.id]) return;
    place(r.id, x, 0);
    placedRoots[r.id] = true;
    x += colsOf(r.id) * (CARD_W + GAP_X);
  });
  // слоты-плейсхолдеры, не попавшие в дерево (родитель вне видимой части)
  slots.forEach(s => { if (!pos[s.id]) { pos[s.id] = {x, y: PAD + s.depth * rowH}; x += CARD_W + GAP_X; } });
  return {byId, goods, vis, maxDepth, maxCols, pos, slots};
}
// орто-ребро: вертикально вниз → горизонтально по зазору между рядами →
// вертикально к дочке. Не заезжает на чужие карточки (решение создателя
// 2026-09-19: «линии не должны выходить на территорию под другим блоком»).
function orthoEdge(a, b, aH) {
  const x1 = a.x + CARD_W/2, y1 = a.y + (aH || CARD_H);   // низ родителя
  const x2 = b.x + CARD_W/2, y2 = b.y;            // верх дочки
  const midY = (y1 + y2) / 2;                     // «улица» между рядами
  ctx.beginPath();
  ctx.moveTo(x1, y1);
  ctx.lineTo(x1, midY);
  ctx.lineTo(x2, midY);
  ctx.lineTo(x2, y2);
  ctx.stroke();
}
function renderGraph() {
  // Ветка «Производители» — дерево построек на канвасе (спека 2026-09-21 §2.5);
  // «Товары» — граф рецептов (как сейчас).
  if (branch === "producers") { renderProdTree(); return; }
  const subset = fullTreeMode() ? null : focusSubset();
  const L = computeLayout(subset, fullTreeMode() ? null : selected);
  const {byId, goods, vis, pos} = L;
  const cat = $("catFilter").value;
  // внутреннее разрешение = реальный CSS-размер × devicePixelRatio (чёткость,
  // регресс от растяжения canvas на 100% — карточки были размытыми)
  const dpr = window.devicePixelRatio || 1;
  const cssW = $("canvasWrap").clientWidth, cssH = $("canvasWrap").clientHeight;
  canvas.width = Math.max(1, Math.round(cssW * dpr));
  canvas.height = Math.max(1, Math.round(cssH * dpr));
  ctx.setTransform(dpr * zoom, 0, 0, dpr * zoom, dpr * panX, dpr * panY);
  ctx.clearRect(-panX/zoom, -panY/zoom, cssW/zoom, cssH/zoom);
  // рёбра (орто)
  ctx.strokeStyle = "#555";
  ctx.lineWidth = 1.5;
  goods.forEach(g => {
    if (!pos[g.id]) return;
    (g.recipe||[]).forEach(s => {
      if (!s.good_id || !pos[s.good_id] || !vis[s.good_id]) return;
      orthoEdge(pos[g.id], pos[s.good_id]);
    });
  });
  // рёбра к пустым слотам (пунктир) — родитель → плейсхолдер
  ctx.setLineDash([4, 3]);
  ctx.strokeStyle = "#666";
  L.slots.forEach(sl => {
    if (!pos[sl.parentId] || !pos[sl.id]) return;
    orthoEdge(pos[sl.parentId], pos[sl.id]);
  });
  ctx.setLineDash([]);
  // обратные рёбра — только выбранного товара (спека 99a.1 §10.7)
  if ($("reverseEdges").checked && selected && pos[selected] && vis[selected]) {
    ctx.strokeStyle = "#a62";
    state.goods.forEach(p => {
      if (p.id === selected || !pos[p.id] || !vis[p.id]) return;
      if ((p.recipe||[]).some(s => s.good_id === selected)) {
        orthoEdge(pos[p.id], pos[selected]);
      }
    });
  }
  // карточки
  goods.forEach(g => {
    const p = pos[g.id];
    if (!p) return;
    const x = p.x, y = p.y;
    const dim = cat && g.kind === "resource"; // приглушённые ресурсы при фильтре
    ctx.fillStyle = dim ? "#262626" : "#3d3d3d";
    ctx.strokeStyle = selected === g.id ? "#8cf" : "#5a5a5a";
    ctx.lineWidth = selected === g.id ? 2 : 1;
    roundRect(x, y, CARD_W, CARD_H, 8);
    ctx.fill(); ctx.stroke();
    ctx.fillStyle = dim ? "#aaa" : "#fff";
    ctx.font = "14px Arial";
    // номер рецепта — вплотную к имени: имя режется с запасом под тег, а тег
    // рисуется всегда целиком (у ресурса рецепта нет — RecipeID = 0)
    const recipeTag = g.recipe_id ? " · рецепт " + g.recipe_id : "";
    const nameText = truncPx(g.name, CARD_W - 16 - ctx.measureText(recipeTag).width, 24);
    ctx.fillText(nameText, x + 8, y + 18);
    if (recipeTag) {
      ctx.fillStyle = dim ? "#888" : "#bbb";
      ctx.fillText(recipeTag, x + 8 + ctx.measureText(nameText).width, y + 18);
    }
    ctx.fillStyle = dim ? "#888" : "#bbb";
    ctx.font = "12px Arial";
    ctx.fillText(truncPx(catName(g.category_id), CARD_W - 16, 22), x + 8, y + 32);
    // бейджи: тир — эффективный из серверного поля (99a.3 §9.3, М5)
    const badges = [];
    badges.push({t: "тир " + g.tier, c: "#5a5a5a"});
    // маркер «производится другой фабрикой» в режиме фабрики (ТЗ §5.3)
    if (isOtherFactoryComponent(g)) badges.push({t: "др. фабрика", c: "#357"});
    ctx.font = "11px Arial";
    let bx = x + 8;
    badges.forEach(b => {
      const w = ctx.measureText(b.t).width + 10;
      ctx.fillStyle = b.c;
      roundRect(bx, y + 40, w, 18, 4); ctx.fill();
      ctx.fillStyle = "#fff";
      ctx.fillText(b.t, bx + 5, y + 53);
      bx += w + 4;
    });
    // Слоты под карточкой НЕ рисуются: заполненный слот = карточка дочки +
    // линия; пустой слот = виртуальная карточка-плейсхолдер того же размера
    // в ряду ниже (рисуется после блока карточек). Решение создателя 2026-09-19.
  });
  // плейсхолдеры пустых слотов — карточки того же размера, пунктир
  L.slots.forEach(sl => {
    const p = pos[sl.id];
    if (!p) return;
    const parent = byId[sl.parentId];
    const parentName = parent ? parent.name : sl.parentId;
    const x = p.x, y = p.y;
    ctx.setLineDash([5, 4]);
    ctx.fillStyle = "#1a1a1a";
    ctx.strokeStyle = "#777";
    ctx.lineWidth = 1.5;
    roundRect(x, y, CARD_W, CARD_H, 8);
    ctx.fill(); ctx.stroke();
    ctx.setLineDash([]);
    ctx.fillStyle = "#999";
    ctx.font = "14px Arial";
    // галка «заполнять ресурсом» (99a Пакет 4, п.8): метка на плейсхолдере
    ctx.fillText(sl.allowResource ? "⚙ ресурс" : "пусто", x + 8, y + 18);
    ctx.font = "12px Arial";
    ctx.fillText(truncPx("слот для «" + parentName + "»", CARD_W - 16, 26), x + 8, y + 32);
  });
}
function trunc(s, n) { return s.length > n ? s.slice(0, n - 1) + "…" : s; }
// truncPx — обрезка по фактической ширине (текст не вылезает за canvas-карточку);
// maxChars — верхняя граница по символам (спека 2026-09-23 §3.3). Вызывать
// после установки ctx.font.
function truncPx(s, maxPx, maxChars) {
  s = String(s == null ? "" : s);
  const cap = Math.min(s.length, maxChars);
  if (ctx.measureText(s).width <= maxPx && s.length <= maxChars) return s;
  let lo = 0, hi = cap;
  while (lo < hi) {
    const mid = (lo + hi + 1) >> 1;
    if (ctx.measureText(s.slice(0, mid) + "…").width <= maxPx) lo = mid; else hi = mid - 1;
  }
  return s.slice(0, lo) + "…";
}
// wrapPx — перенос текста по фактической ширине (ctx.measureText, вызывать
// после установки ctx.font): массив строк. Слово, само шире строки, обрезается
// truncPx (многоточие) — как раньше. Пустая строка → [].
function wrapPx(s, maxPx) {
  s = String(s == null ? "" : s);
  const lines = [];
  let cur = "";
  s.split(" ").forEach(w => {
    const test = cur ? cur + " " + w : w;
    if (ctx.measureText(test).width <= maxPx) { cur = test; return; }
    if (cur) { lines.push(cur); cur = ""; }
    if (ctx.measureText(w).width <= maxPx) { cur = w; return; }
    lines.push(truncPx(w, maxPx, w.length));
  });
  if (cur) lines.push(cur);
  return lines;
}
function roundRect(x, y, w, h, r) {
  ctx.beginPath();
  ctx.moveTo(x + r, y);
  ctx.arcTo(x + w, y, x + w, y + h, r);
  ctx.arcTo(x + w, y + h, x, y + h, r);
  ctx.arcTo(x, y + h, x, y, r);
  ctx.arcTo(x, y, x + w, y, r);
  ctx.closePath();
}
function catName(id) {
  const c = state.categories.find(c => c.id === id);
  return c ? c.name : id;
}

// ---------- тултип карточки ----------
function showTip(ev, id) {
  if (branch === "producers") { showProdTip(ev, id); return; }
  const g = state.goods.find(x => x.id === id);
  if (!g) return;
  const tip = $("tip");
  tip.innerHTML = '<b>' + esc(g.name) + '</b><br>' + esc(catName(g.category_id)) +
                  ' · тир ' + g.tier +
                  (isOtherFactoryComponent(g) ? '<br>производится другой фабрикой' : '');
  const rect = canvas.getBoundingClientRect();
  let x = ev.clientX - rect.left + 14, y = ev.clientY - rect.top + 14;
  if (x + 220 > rect.width) x = ev.clientX - rect.left - 230;
  if (y + 60 > rect.height) y = ev.clientY - rect.top - 60;
  tip.style.left = x + "px"; tip.style.top = y + "px";
  tip.style.display = "block";
}
function hideTip() { $("tip").style.display = "none"; }

// ---------- клики и пан/зум ----------
// hitTest — только карточка {type:"card", id}. Слоты на канвасе не рисуются
// (только линии-рёбра; решение создателя 2026-09-19: «между радаром и тем,
// из чего он сделан, только линии связующие должны быть») — клик по слоту
// убран, выбор составляющей — по её карточке.
function hitTest(ev) {
  const rect = canvas.getBoundingClientRect();
  const mx = (ev.clientX - rect.left - panX) / zoom;
  const my = (ev.clientY - rect.top - panY) / zoom;
  if (branch === "producers") return prodTreeHitTest(mx, my);
  const L = computeLayout(fullTreeMode() ? null : focusSubset(), fullTreeMode() ? null : selected);
  for (const g of L.goods) {
    const p = L.pos[g.id];
    if (!p) continue;
    if (mx >= p.x && mx <= p.x + CARD_W && my >= p.y && my <= p.y + CARD_H) return {type: "card", id: g.id};
  }
  return null;
}
let panning = null, downPos = null, moved = false;
let downOnCard = null; // id карточки под mousedown (для dblclick-fallback: карточка могла уехать после перестройки подграфа)
canvas.addEventListener("mousedown", ev => {
  if (dndActive) return; // во время перетаскивания карточки на канвас пан не стартует (ТЗ §7.3.3)
  hideCtxMenu();
  const hit = hitTest(ev);
  downPos = {x: ev.clientX, y: ev.clientY};
  moved = false;
  // второй клик двойного (detail>1): карточка могла уехать после перестройки
  // подграфа первым кликом — downOnCard первого клика сохраняется для dblclick
  if (!(ev.detail > 1 && downOnCard)) {
    downOnCard = hit && hit.type === "card" ? hit.id : null;
  }
  if (hit && hit.type === "card") {
    if (branch === "producers") {
      // дерево построек (спека 2026-09-21 §2.5): тип/подтип — выбор узла;
      // узел-приглашение — попап создания; фиксированный узел — справка
      const n = prodNodeById(hit.id);
      if (n) { selected = hit.id; renderAll(); centerOn(hit.id); }
      else if (hit.id.indexOf("invite:") === 0) { openProdCreatePopup(hit.id); }
      else { openProdNodePopup(hit.id); }
    } else {
      selected = hit.id; renderAll();
      // центрировать вид на выбранной карточке (зум сохраняется): в фокус-режиме
      // подграф перестраивается и карточка могла уехать за край вьюпорта
      // (требование создателя 2026-09-19); в полном дереве раскладка не меняется —
      // карточка уже видна, центрирование не нужно; pan-драг начинается с пустого
      // места (ветка else) — здесь не срабатывает
      if (!fullTreeMode()) centerOn(hit.id);
    }
  }
  else { panning = {x: ev.clientX, y: ev.clientY}; canvas.style.cursor = "grabbing"; }
});
window.addEventListener("mousemove", ev => {
  if (downPos && (Math.abs(ev.clientX - downPos.x) > 4 || Math.abs(ev.clientY - downPos.y) > 4)) moved = true;
  if (panning) {
    panX += ev.clientX - panning.x;
    panY += ev.clientY - panning.y;
    panning = {x: ev.clientX, y: ev.clientY};
    saveView(); renderGraph();
    return;
  }
  const hit = hitTest(ev);
  if (hit && hit.type === "card") { showTip(ev, hit.id); canvas.style.cursor = "pointer"; }
  else { hideTip(); canvas.style.cursor = "grab"; }
});
window.addEventListener("mouseup", ev => {
  if (panning) { panning = null; canvas.style.cursor = "grab"; }
  // Клик по пустому месту выбор НЕ сбрасывает (создатель 2026-09-19: «по клику
  // вне графа на канвасе не очищать его, мешает») — только pan (mousedown без
  // hit ставит panning). Оверлей «Кликните товар в справочнике» — только при
  // отсутствии выбора изначально (пустой старт), не после кликов.
  downPos = null;
});
// двойной клик по карточке = выбор + попап-деталь (99a.3 §10.3)
canvas.addEventListener("dblclick", ev => {
  const hit = hitTest(ev);
  // hitTest может не найти карточку: первый клик двойного перестроил подграф,
  // карточка уехала из точки клика — fallback на downOnCard (карточка первого клика)
  const id = hit && hit.type === "card" ? hit.id : downOnCard;
  if (id) {
    selected = id;
    // Клик по дереву построек ВСЕГДА открывает карточку постройки: режим
    // справочника («Товары») влияет только на список справа, не на канвас.
    // Через openPopup нельзя — в режиме «Товары» он уводит в товарный попап
    // (идея 2026-09-23, шаг 2).
    if (branch === "producers") openProdNodePopup(id); else openPopup(id);
    renderAll();
  }
  downOnCard = null;
});
canvas.addEventListener("mouseleave", hideTip);
canvas.addEventListener("wheel", ev => {
  if (dndActive) return; // зум колесом во время перетаскивания не срабатывает (ТЗ §7.3.3)
  ev.preventDefault();
  const rect = canvas.getBoundingClientRect();
  const mx = ev.clientX - rect.left, my = ev.clientY - rect.top;
  const factor = ev.deltaY < 0 ? 1.15 : 1 / 1.15;
  const nz = Math.min(4, Math.max(MIN_ZOOM, zoom * factor));
  panX = mx - (mx - panX) * (nz / zoom);
  panY = my - (my - panY) * (nz / zoom);
  zoom = nz;
  saveView(); renderGraph();
});
function saveView() {
  // Вид канваса по ветке: дерево построек — свой zoom/pan (спека §2.5).
  if (branch === "producers") {
    localStorage.setItem("gs_prodZoom", zoom);
    localStorage.setItem("gs_prodPanX", panX);
    localStorage.setItem("gs_prodPanY", panY);
  } else {
    localStorage.setItem("gs_zoom", zoom);
    localStorage.setItem("gs_panX", panX);
    localStorage.setItem("gs_panY", panY);
  }
  localStorage.setItem("gs_viewPos", "1"); // позиция сохранена — при старте не центрировать
}
// Центрирование: показать верхний тир (товары-верхушки, не ресурсы).
// Сбрасывает уехавший pan (напр. сохранённый от старого широкого канваса).
// Только режим «всё дерево» (в фокусе кнопка скрыта); при resize/сворачивании
// панели в фокусе — центрирование на выбранном (99a.3 §10.2).
function centerTop() {
  if (branch === "producers") { centerProdTree(); return; }
  if (!fullTreeMode()) {
    if (selected) { centerOn(selected); return; }
    zoom = 1; panX = 0; panY = 0; saveView(); renderGraph(); return;
  }
  const L = computeLayout();
  const goods = L.goods.filter(g => g.kind !== "resource");
  if (!goods.length) { zoom = 1; panX = 0; panY = 0; saveView(); renderGraph(); return; }
  const rect = canvas.getBoundingClientRect();
  // верхушки — глубина 0 (верхний ряд): товары без родителей
  const depthMemo = {};
  const byId = {};
  state.goods.forEach(g => byId[g.id] = g);
  const top = L.goods.filter(g => g.kind !== "resource" && depthOf(g, byId, depthMemo) === 0);
  const list = top.length ? top : goods;
  const first = list[0], p = L.pos[first.id];
  // масштаб: чтобы верхний ряд поместился в ширину экрана
  const nz = Math.max(MIN_ZOOM, Math.min(1, (rect.width - 40) / (list.length * (CARD_W + GAP_X))));
  zoom = Math.max(MIN_ZOOM, Math.min(4, nz));
  panX = rect.width / 2 - (p.x + CARD_W / 2) * zoom;
  panY = rect.height / 2 - (p.y + CARD_H / 2) * zoom;
  saveView(); renderGraph();
}
// centerProdTree — центрирование дерева построек целиком (высота из раскладки).
function centerProdTree() {
  const L = prodTreeLayout();
  const rect = canvas.getBoundingClientRect();
  // пустой раздел — центрировать нечего
  if (!Object.keys(L.pos).length) { zoom = 1; panX = 0; panY = 0; saveView(); renderGraph(); return; }
  // maxCols из prodTreeLayout — уже пиксели (правый край дерева + GAP_X),
  // поэтому ширина = maxCols − GAP_X (правый край) − PAD (левый край) + 2·PAD.
  const w = L.maxCols - GAP_X + PAD;
  const h = L.treeH + PAD * 2; // сумма высот рядов дерева
  const nz = Math.max(MIN_ZOOM, Math.min(1, Math.min((rect.width - 40) / w, (rect.height - 40) / h)));
  zoom = Math.max(MIN_ZOOM, Math.min(4, nz));
  // центрируем по границам дерева (верхнего узла-«корня» больше нет)
  const cx = PAD + Math.max(0, L.maxCols - GAP_X - PAD) / 2;
  const cy = PAD + L.treeH / 2;
  panX = rect.width / 2 - cx * zoom;
  panY = rect.height / 2 - cy * zoom;
  saveView(); renderGraph();
}
// centerOn — центрировать вид на товаре (зум сохраняется): выбор из
// справочника (99a.3 §10.2, гейт-вопрос 2).
function centerOn(id) {
  if (branch === "producers") {
    const L = prodTreeLayout();
    const p = L.pos[id];
    if (!p) return;
    const rect = canvas.getBoundingClientRect();
    panX = rect.width / 2 - (p.x + CARD_W / 2) * zoom;
    panY = rect.height / 2 - (p.y + (L.h[id] || CARD_H) / 2) * zoom;
    saveView(); renderGraph();
    return;
  }
  const L = computeLayout(fullTreeMode() ? null : focusSubset(), fullTreeMode() ? null : selected);
  const p = L.pos[id];
  if (!p) return;
  const rect = canvas.getBoundingClientRect();
  panX = rect.width / 2 - (p.x + CARD_W / 2) * zoom;
  panY = rect.height / 2 - (p.y + CARD_H / 2) * zoom;
  saveView(); renderGraph();
}
