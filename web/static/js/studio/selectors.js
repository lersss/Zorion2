"use strict";
// ---------- селекторы скопа ветки «Товары» (ТЗ §3/§4/§8) ----------
// renderGoodsFactorySel — список конкретных фабрик с товарной категорией,
// optgroup по родителю; первый пункт — «Все товары (обзор)». Значение сохраняется.
function renderGoodsFactorySel() {
  const sel = $("goodsFactorySel");
  if (!sel) return;
  const facs = concreteGoodsFactories();
  const groups = {}, order = [];
  facs.forEach(p => {
    const key = p.parent_name || "—";
    if (!groups[key]) { groups[key] = []; order.push(key); }
    groups[key].push(p);
  });
  let html = '<option value="">Все товары (обзор)</option>';
  order.forEach(k => {
    const list = groups[k].slice().sort((a, b) => {
      const ca = catName(a.category_id), cb = catName(b.category_id);
      if (ca !== cb) return ca.localeCompare(cb);
      return factoryStepRank(a) - factoryStepRank(b);
    });
    html += '<optgroup label="' + esc(k) + '">' + list.map(p =>
      '<option value="' + p.id + '">' + esc(catName(p.category_id)) + ' · ' + esc(factoryLevelLabel(p)) + (p.hidden ? ' · скрыта' : '') + '</option>').join("") + '</optgroup>';
  });
  sel.innerHTML = html;
  if (!facs.length) {
    sel.disabled = true;
    sel.title = "Конкретных фабрик нет — создайте в разделе «Постройки»";
    goodsFactory = "";
  } else {
    sel.disabled = false;
    sel.title = "Граф выбранной фабрики (замыкание вниз)";
  }
  if (goodsFactory && !facs.some(p => String(p.id) === String(goodsFactory))) {
    goodsFactory = "";
    localStorage.removeItem("gs_goodsFactory"); // удалённая фабрика в ключе не ломает экран (ТЗ §9 п.11)
  }
  sel.value = goodsFactory;
}
// renderGoodsFamilySel — семейства F0–F10 из racesData (ТЗ §4).
function renderGoodsFamilySel() {
  const sel = $("goodsFamilySel");
  if (!sel) return;
  const fams = racesData ? racesData.families : [];
  sel.innerHTML = '<option value="">Все семейства</option>' + fams.map(f =>
    '<option value="' + f.id + '">' + esc(familyLabelById(f.id)) + '</option>').join("");
  if (goodsFamily && !fams.some(f => f.id === goodsFamily)) {
    goodsFamily = "";
    localStorage.removeItem("gs_goodsFamily");
  }
  sel.value = goodsFamily;
}
