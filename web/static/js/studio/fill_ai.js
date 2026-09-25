"use strict";
// ---------- fill: «заполнить комплектующие» (спека iterC §6) ----------
// Двухфазный: POST fill → ИИ думает (асинхронно) → proposals в state →
// попап «Предложения ИИ» (авто-открытие в fetchState) → apply/cancel.
async function fillGood() {
  const g = state.goods.find(x => x.id === popupGoodId);
  if (g && !(g.recipe || []).some(s => !s.good_id)) {
    showReport(["Сначала добавьте слот («+ слот»)"]);
    return;
  }
  const r = await api("POST", "/studio/api/goods/" + popupGoodId + "/fill");
  if (r) showReport(["ИИ думает… — предложения появятся в попапе"]);
  fetchState();
}
function openProposalsPopup() {
  proposalsAccepted = {}; // новый набор предложений — свежий выбор
  $("proposalsPopup").style.display = "flex";
  $("proposalsOverlay").style.display = "block";
  renderProposalsPopup();
}
function closeProposalsPopup() {
  $("proposalsPopup").style.display = "none";
  $("proposalsOverlay").style.display = "none";
}
// попап «Предложения ИИ» (спека iterC §6.5): строка на каждый proposal —
// kind=link: «Заполнить слот N готовым товаром „Z"» + переключатель
// [Заполнить ✓/Пропустить]; kind=new: «Создать новый товар „X"» + селект
// товарных категорий (дефолт — category_id если валидна, иначе категория
// родителя) + пометка «категория не найдена»; футер «Применить выбранное
// (N)» (disabled при N=0) / «Отмена».
function renderProposalsPopup() {
  const el = $("proposalsPopupBody");
  const props = state.proposals || [];
  const good = state.goods.find(x => x.id === state.proposals_good_id);
  $("proposalsPopupTitle").textContent = "Предложения ИИ для «" + (good ? good.name : state.proposals_good_id) + "»";
  const goodCats = state.categories.filter(c => c.kind === "good");
  const parentCat = good ? good.category_id : null;
  // вычистить устаревшие индексы (новый набор предложений после повторного fill)
  Object.keys(proposalsAccepted).forEach(i => { if (Number(i) >= props.length) delete proposalsAccepted[i]; });
  let html = '<div class="hint" style="margin-bottom:8px">применятся только принятые пункты</div>';
  props.forEach((p, i) => {
    if (!proposalsAccepted[i]) {
      proposalsAccepted[i] = {
        accepted: true,
        category_id: p.kind === "new" ? (p.category_valid ? p.category_id : parentCat) : null
      };
    }
    const a = proposalsAccepted[i];
    if (p.kind === "link") {
      html += '<div class="slot" style="margin-bottom:8px">' +
        '<label style="display:block;margin-bottom:4px;cursor:pointer"><input type="checkbox"' + (a.accepted ? ' checked' : '') + ' onchange="toggleProposal(' + i + ', this.checked)"> Заполнить слот ' + p.slot + ' готовым товаром «' + esc(p.link_name || p.name) + '»</label>' +
        (p.reason ? '<div class="sreason">' + esc(p.reason) + '</div>' : '') +
        '</div>';
    } else {
      const catOpts = goodCats.map(c => '<option value="' + c.id + '"' + (c.id === a.category_id ? ' selected' : '') + '>' + esc(c.name) + '</option>').join("");
      const warn = !p.category_valid ? '<div style="color:#f66;font-size:12px;margin-top:2px">категория «' + esc(p.category) + '» не найдена — выбрана категория родителя</div>' : '';
      html += '<div class="slot" style="margin-bottom:8px">' +
        '<label style="display:block;margin-bottom:4px;cursor:pointer"><input type="checkbox"' + (a.accepted ? ' checked' : '') + ' onchange="toggleProposal(' + i + ', this.checked)"> Создать новый товар «' + esc(p.name) + '»</label>' +
        '<select onchange="proposalCategory(' + i + ', this.value)">' + catOpts + '</select>' + warn +
        (p.reason ? '<div class="sreason">' + esc(p.reason) + '</div>' : '') +
        '</div>';
    }
  });
  const n = Object.values(proposalsAccepted).filter(a => a.accepted).length;
  html += '<div class="btns">' +
    '<button class="gen" id="proposalsApplyBtn" onclick="applyProposals()"' + (n === 0 ? ' disabled' : '') + '>Применить выбранное (' + n + ')</button>' +
    '<button onclick="cancelProposals()">Отмена</button>' +
    '</div>';
  el.innerHTML = html;
}
function toggleProposal(i, checked) {
  if (proposalsAccepted[i]) proposalsAccepted[i].accepted = checked;
  renderProposalsPopup();
}
function proposalCategory(i, val) {
  if (proposalsAccepted[i]) proposalsAccepted[i].category_id = Number(val);
}
// «Применить выбранное»: собрать accepted = [{i, category_id}] (i — индекс в
// state.proposals, category_id — выбранный селект для kind=new) → apply
async function applyProposals() {
  const accepted = [];
  Object.keys(proposalsAccepted).forEach(i => {
    const a = proposalsAccepted[i];
    if (!a.accepted) return;
    const item = { i: Number(i) };
    const p = (state.proposals || [])[i];
    if (p && p.kind === "new") item.category_id = a.category_id;
    accepted.push(item);
  });
  if (!accepted.length) return;
  const r = await api("POST", "/studio/api/goods/" + state.proposals_good_id + "/fill/apply", { accepted });
  if (r) { closeProposalsPopup(); fetchState(); }
}
// «Отмена»: cancel → proposals сброшены (попап не переоткроется опросом)
async function cancelProposals() {
  const r = await api("POST", "/studio/api/goods/" + state.proposals_good_id + "/fill/cancel");
  if (r) { closeProposalsPopup(); fetchState(); }
}
// описание каталога (спека 2026-09-21-каталог-описание §9.1): ввод → PUT
// /goods/{id} {description}; пустой ввод = очистка (И3); ошибка → откат рендера
async function setDescription(val) {
  const r = await api("PUT", "/studio/api/goods/" + popupGoodId, {description: val});
  if (r) fetchState(); else renderPopup();
}
// «предложить описание» в попапе (§9.2): одиночный прогон по записи
async function describeGood() {
  const r = await api("POST", "/studio/api/descriptions/fill", {good_ids: [String(popupGoodId)]});
  if (r) showReport(["ИИ думает… — предложение появится в попапе"]);
  fetchState();
}
// ---------- попап «Описания ИИ» (спека 2026-09-21-каталог-описание §9.4) ----------
function openDescPopup() {
  descAccepted = {}; // новый набор предложений — свежий выбор
  $("descPopup").style.display = "flex";
  $("descOverlay").style.display = "block";
  renderDescPopup();
}
function closeDescPopup() {
  $("descPopup").style.display = "none";
  $("descOverlay").style.display = "none";
}
// renderDescPopup — строки предложений (редактируемый textarea + чекбокс
// «принять»), прогресс и футер «Применить принятые (K)» / «Отмена».
function renderDescPopup() {
  const el = $("descPopupBody");
  const props = state.desc_proposals || [];
  const done = state.desc_done || 0;
  const total = state.desc_total || props.length;
  $("descPopupTitle").textContent = "Описания ИИ (" + props.length + " из " + total + ")" +
    (state.desc_generating ? " · идёт прогон…" : "");
  // вычистить устаревшие индексы (набор растёт порциями во время прогона)
  Object.keys(descAccepted).forEach(i => { if (Number(i) >= props.length) delete descAccepted[i]; });
  let html = '<div class="hint" style="margin-bottom:8px">применятся только принятые пункты' +
    (state.desc_generating ? ' · обработано ' + done + ' из ' + total : '') + '</div>';
  props.forEach((p, i) => {
    if (!descAccepted[i]) descAccepted[i] = {accepted: !!p.text, text: p.text || ""};
    const a = descAccepted[i];
    html += '<div class="slot" style="margin-bottom:8px">' +
      '<label style="display:block;margin-bottom:4px;cursor:pointer"><input type="checkbox"' + (a.accepted ? ' checked' : '') + ' onchange="toggleDesc(' + i + ', this.checked)"> ' + esc(p.name) + (p.kind === "resource" ? ' · ресурс' : '') + '</label>' +
      '<textarea rows="2" onchange="descText(' + i + ', this.value)">' + esc(a.text) + '</textarea>' +
      '</div>';
  });
  const k = Object.values(descAccepted).filter(a => a.accepted).length;
  html += '<div class="btns">' +
    '<button class="gen" onclick="applyDesc()"' + (k === 0 ? ' disabled' : '') + '>Применить принятые (' + k + ')</button>' +
    '<button onclick="cancelDesc()">Отмена</button>' +
    '</div>';
  el.innerHTML = html;
}
function toggleDesc(i, checked) {
  if (descAccepted[i]) descAccepted[i].accepted = checked;
  renderDescPopup();
}
function descText(i, val) {
  if (descAccepted[i]) descAccepted[i].text = val;
}
// «Применить принятые» → POST apply {accepted:[{i, text}]} → закрыть, fetchState
async function applyDesc() {
  const accepted = [];
  Object.keys(descAccepted).forEach(i => {
    const a = descAccepted[i];
    if (!a.accepted) return;
    accepted.push({i: Number(i), text: a.text});
  });
  if (!accepted.length) return;
  const r = await api("POST", "/studio/api/descriptions/apply", {accepted});
  if (r) { closeDescPopup(); fetchState(); }
}
// «Отмена» → POST cancel: при идущем прогоне он останавливается, предложения
// сохраняются (И7), попап остаётся; при простое — отказ от набора: сервер
// выбрасывает предложения, попап закрывается и не всплывает после перезагрузки
async function cancelDesc() {
  const r = await api("POST", "/studio/api/descriptions/cancel", {});
  if (!r) return;
  const out = await r.json(); // {cancelled, discarded, done, total}
  const discarded = !!out && out.discarded === "true";
  if (discarded) closeDescPopup();
  showReport([discarded ? "Набор предложений отброшен" : "Прогон остановлен — предложения сохранены"]);
  fetchState();
}
// пакетный прогон «где пусто» (§9.6): подтверждение → fill {scope:"missing"}
function runDescBatch() {
  const n = descMissingCount();
  if (!n) return;
  openModal(
    '<h3>Описания ИИ</h3>' +
    '<div>Предложить описания для ' + n + ' записей? Прогон идёт пачками и может занять несколько минут.</div>',
    [
      {label: "Запустить", cls: "gen", primary: true, action: startDescBatch},
      {label: "Отмена", cls: "", action: closeModal}
    ]
  );
}
async function startDescBatch() {
  closeModal();
  const r = await api("POST", "/studio/api/descriptions/fill", {scope: "missing"});
  if (r) showReport(["ИИ думает… — предложения появятся в попапе"]);
  fetchState();
}
// descMissingCount — записей каталога без описания (для кнопки §9.6)
function descMissingCount() {
  return state.goods.filter(g => !g.description).length;
}
// updateDescBatchBtn — счётчик/доступность кнопки пакетного прогона.
// F5 (спека 2026-09-24 §4): при помощнике не running кнопка выключена.
function updateDescBatchBtn() {
  const btn = $("btnDescBatch");
  if (!btn) return;
  const n = descMissingCount();
  btn.textContent = "описания ИИ: " + n + " без описания";
  btn.disabled = n === 0 || state.generating || !aiRunning();
  btn.title = !aiRunning() ? "Сначала запустите помощника" : "ИИ предложит описания всем записям без описания";
}
// ---------- локальный ИИ-помощник (opencode serve): индикатор и кнопки ----------
// Спека 2026-09-24-студия-управление-локальным-ии §4: состояние — из state.ai
// (GET /studio/api/state), тексты серверные (detail), кнопки — от state+managed.
function aiState() { return (state && state.ai) || {state: "stopped", managed: false, detail: "ИИ: недоступен"}; }
function aiRunning() { return aiState().state === "running"; }
function updateAIStatus() {
  const txt = $("aiStatusText");
  if (!txt) return;
  const a = aiState();
  txt.textContent = a.detail || "ИИ: —";
  const start = $("btnAiStart"), stop = $("btnAiStop");
  const managed = !!a.managed;
  const busy = !!(state.generating || state.desc_generating);
  start.disabled = !(managed && a.state === "stopped");
  stop.disabled = !(managed && (a.state === "running" || a.state === "starting")) || busy;
  stop.title = busy ? "идёт прогон — сначала остановите прогон" : "Остановить локальный ИИ-помощник";
}
// aiAction — POST start/stop → немедленный GET status → перерисовка; ошибку
// показываем человеческим текстом (api/showReport), не сырой простыней.
async function aiAction(path) {
  const r = await api("POST", "/studio/api/ai/" + path);
  if (r) {
    const s = await api("GET", "/studio/api/ai/status");
    if (s) { state.ai = await s.json(); updateAIStatus(); }
  }
  fetchState();
}
function aiStart() { aiAction("start"); }
function aiStop() { aiAction("stop"); }

// Защита несохранённого текста от авто-обновления (спека §9.7): авто-перерисовка
// пропускается, если фокус в textarea попапа, а значение отличается от модельного.
function descInputDirty() {
  const el = $("descInput");
  if (!el || document.activeElement !== el) return false;
  const g = state.goods.find(x => x.id === popupGoodId);
  return !!g && el.value !== (g.description || "");
}
function descPopupDirty() {
  const body = $("descPopupBody");
  if (!body) return false;
  const ae = document.activeElement;
  if (!ae || ae.tagName !== "TEXTAREA" || !body.contains(ae)) return false;
  const i = [...body.querySelectorAll("textarea")].indexOf(ae);
  const p = (state.desc_proposals || [])[i];
  return !!p && ae.value !== (p.text || "");
}
function refreshDescPopup() { if (!descPopupDirty()) renderDescPopup(); }
