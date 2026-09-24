// web/static/js/modal/ui_state.js
// Чистые помощники неразрушающей перерисовки правой панели модалки (баг
// 2026-09-25: автообновление карточки сбрасывало состояние интерфейса).
// Состояние (раскрытые инлайн-детали, выбранный слот магазина) собирается перед
// перерисовкой и возвращается после неё. Модуль не трогает DOM/сеть — только
// вычисления, поэтому исполняется в Node (web/frontend_ui_state_test.go).

// mergeDetailIds — обновить список раскрытых id по текущему рендеру.
// present — [{ id, expanded }] присутствующих в DOM блоков; id, которых в
// текущем рендере нет, сохраняют прежнее значение (перерисовка другой вкладки
// не должна терять раскрытое на этой). Возвращает массив id.
export function mergeDetailIds(stored, present) {
    const set = new Set((stored || []).map(String));
    (present || []).forEach(it => {
        if (!it || it.id == null) return;
        if (it.expanded) set.add(String(it.id));
        else set.delete(String(it.id));
    });
    return Array.from(set);
}

// expandDetailIds — какие из присутствующих id раскрыть после рендера: только
// сохранённые ранее (сравнение по строковому ключу).
export function expandDetailIds(stored, presentIds) {
    const set = new Set((stored || []).map(String));
    return (presentIds || []).filter(id => set.has(String(id)));
}

// resolveMarketSlot — выбранный универсальный слот магазина после перерисовки:
// сохранённый, если он ещё есть среди ключей; иначе первый; ключей нет — null.
// Один источник и для разметки (marketHtml), и для памяти (initMarket).
export function resolveMarketSlot(keys, selectedSlot) {
    const list = Array.isArray(keys) ? keys : [];
    return (selectedSlot && list.indexOf(selectedSlot) !== -1) ? selectedSlot : (list[0] || null);
}

// isEditableControl — редактируемый контрол (маркер «в форме работают»):
// input/select/textarea или contenteditable. Кнопки/ссылки — не редактируемые.
// Используется правой панелью модалки, пока в ней стоит фокус: очередной тик
// автообновления откладывается, чтобы не выбить курсор и не закрыть открытый
// выпадающий список (решение создателя 2026-09-25). Чистая функция — под node-тест.
export function isEditableControl(el) {
    if (!el) return false;
    const tag = String(el.tagName || '').toUpperCase();
    if (tag === 'INPUT' || tag === 'SELECT' || tag === 'TEXTAREA') return true;
    return el.isContentEditable === true;
}
