"use strict";
// ---------- справочник предметов (спека 2026-09-20-фабрики §4.2) ----------
function renderItemSprav() {
  const sc = $("spravList").scrollTop;
  const q = $("spravSearch").value.trim().toLowerCase();
  const list = state.items.filter(it => !q || it.name.toLowerCase().includes(q));
  $("spravTitle").textContent = "Предметы · " + list.length;
  $("spravList").innerHTML = list.map(it =>
    '<div class="card" onclick="itemClick(' + it.id + ')" ondblclick="itemDblClick(' + it.id + ')">' +
    '<div class="nm">' + esc(it.name) + '<button class="info" onclick="event.stopPropagation();openItemPopup(' + it.id + ')" title="деталь (без смены выбора)">ⓘ</button></div>' +
    '<div class="meta">' + esc(it.slot_type) + '</div>' +
    '</div>'
  ).join("") || '<div class="empty-note">нет предметов</div>';
  $("spravList").scrollTop = sc;
}
function itemClick(id) { selected = id; renderAll(); }
function itemDblClick(id) { selected = id; openItemPopup(id); renderAll(); }
function openItemPopup(id) {
  popupGoodId = id;
  resetPopupPos();
  $("detailPopup").style.display = "flex";
  $("detailOverlay").style.display = "block";
  renderItemPopup();
}
function renderItemPopup() {
  const el = $("popupBody");
  if (!popupGoodId) return;
  const it = state.items.find(x => x.id === popupGoodId);
  if (!it) { closePopup(); return; }
  $("popupTitle").textContent = it.name;
  let html = '';
  html += '<div class="dname"><span class="rename" onclick="startItemRename()" title="переименовать">' + esc(it.name) + '</span></div>';
  html += '<div class="dmeta">тип слота: <select class="dcat" onchange="changeItemSlot(this.value)">' +
    ["чертёж", "модуль", "инструмент", "сертификат"].map(s => '<option value="' + s + '"' + (s === it.slot_type ? ' selected' : '') + '>' + esc(s) + '</option>').join("") +
    '</select></div>';
  html += codeNote(it.code); // метка переноса — справочно, только чтение (§3.3)
  html += '<div class="dmeta">unlocks (предмет-рецепт): <textarea id="itemUnlocks" rows="2">' + esc(jsonText(it.unlocks)) + '</textarea></div>';
  html += '<div class="dmeta">params: <textarea id="itemParams" rows="2">' + esc(jsonText(it.params)) + '</textarea></div>';
  html += '<div class="btns"><button class="gen" onclick="saveItemJSON()">сохранить JSON</button></div>';
  html += '<div class="btns">';
  html += '<button class="del" onclick="askDeleteItem()">удалить</button>';
  html += '</div>';
  el.innerHTML = html;
}
function startItemRename() {
  const it = state.items.find(x => x.id === popupGoodId);
  if (!it) return;
  const el = $("popupBody").querySelector(".dname");
  el.innerHTML = '<input id="renameInput" type="text" value="' + esc(it.name) + '" style="width:100%;box-sizing:border-box">';
  const inp = $("renameInput");
  inp.focus(); inp.select();
  inp.addEventListener("keydown", ev => {
    if (ev.key === "Enter") commitItemRename();
    if (ev.key === "Escape") renderItemPopup();
  });
  inp.addEventListener("blur", commitItemRename);
}
async function commitItemRename() {
  const inp = $("renameInput");
  if (!inp) return;
  const name = inp.value.trim();
  if (!name) { renderItemPopup(); return; }
  const r = await api("PUT", "/studio/api/items/" + popupGoodId, {name});
  if (r) fetchState(); else renderItemPopup();
}
async function changeItemSlot(v) {
  const r = await api("PUT", "/studio/api/items/" + popupGoodId, {slot_type: v});
  if (r) fetchState();
}
async function saveItemJSON() {
  const body = {};
  const fields = [["unlocks", "itemUnlocks"], ["params", "itemParams"]];
  for (const [key, elId] of fields) {
    const v = $(elId).value.trim();
    if (v === "") { body[key] = null; continue; }
    try { JSON.parse(v); } catch (e) { showReport(["ошибка: " + key + " — невалидный JSON"]); return; }
    body[key] = v;
  }
  const r = await api("PUT", "/studio/api/items/" + popupGoodId, body);
  if (r) fetchState();
}
function askDeleteItem() {
  const it = state.items.find(x => x.id === popupGoodId);
  if (!it) return;
  openModal(
    '<h3>Удалить предмет</h3>' +
    '<div>Удалить предмет «' + esc(it.name) + '»? Связи с постройками удалятся.</div>',
    [
      {label: "Удалить", cls: "del", primary: true, action: doDeleteItem},
      {label: "Отмена", cls: "", action: closeModal}
    ]
  );
}
async function doDeleteItem() {
  closeModal();
  const r = await api("DELETE", "/studio/api/items/" + popupGoodId);
  if (selected === popupGoodId) selected = null;
  closePopup();
  if (r) fetchState();
}
async function itemAdd() {
  const nameEl = $("itemNewName");
  if (!nameEl) return; // форма доступна только из открытого попапа
  const name = nameEl.value.trim();
  const slot = $("itemNewSlot").value;
  if (!name) return;
  const r = await api("POST", "/studio/api/items", {name, slot_type: slot});
  if (!r) return; // ошибка — попап остаётся открытым с введённым именем
  const it = await r.json();
  nameEl.value = "";
  selected = it.id;
  closeModal();
  fetchState();
}

// клик по пилюле тира: пересборка списка без сброса scroll/выбора/попапа (99a.3-ui §5.3)
function toggleTier(t, checked) {
  if (checked) tierSel.add(t); else tierSel.delete(t);
  saveTierSel();
  renderSprav();
}
// пилюля «все» (создатель 2026-09-19): что-то выбрано → сброс (все тиры);
// пусто → выбрать все актуальные тиры (фиксированные 0..8 + тиры выше 8, если есть)
function toggleAllTiers() {
  if (tierSel.size > 0) {
    tierSel.clear();
  } else {
    spravTiers().forEach(t => tierSel.add(t));
  }
  saveTierSel();
  renderSprav();
}
// сохранение выбора пилюль в localStorage (восстановление при старте)
function saveTierSel() {
  localStorage.setItem("gs_tierSel", JSON.stringify([...tierSel]));
}
// клик по карточке = выбор на канвасе + подграф + авто-центрирование; попап НЕ открывается (99a.3 §8.3)
function spravClick(id) {
  selected = id;
  renderAll();
  if (!fullTreeMode()) centerOn(id);
}
// двойной клик = выбор + попап (99a.3 §10.3)
function spravDblClick(id) {
  selected = id;
  openPopup(id);
  renderAll();
  if (!fullTreeMode()) centerOn(id);
}
function dragStart(ev, id) { ev.dataTransfer.setData("text/plain", id); }
