// web/static/js/belt/belt_input.js
// Ввод мини-игры добычи в поясе (спека
// 2026-09-22-пояса-малых-тел-этап-3-добыча §5.1, ревизия 4; UI-спека §4.2 п.7):
// «два стика» — W/S тяга, A/D стрейф, мышь — ориентация носа, ЛКМ (удержание) —
// добыча, Space — дублёр ЛКМ, Shift — тормоз, Esc — пауза. ПКМ в сцене
// отключена (контекстное меню подавлено). Значения пишутся в объект input
// (читает физика и легенда HUD).
const KEY_MAP = {
    KeyW: 'forward', KeyS: 'back', KeyA: 'left', KeyD: 'right',
    ShiftLeft: 'brake', ShiftRight: 'brake', Space: 'space',
};

// drillHeld — кнопка добычи: ЛКМ или её дублёр Space (§5.1).
export function drillHeld(input) {
    return !!(input.space || input.mouseDown);
}

// bindInput — навешивает обработчики; handlers.onEscape — реакция на Esc
// (пауза/продолжить), handlers.canvas — канвас сцены (подавление ПКМ).
// Повторный вызов не предусмотрен (страница одна).
export function bindInput(input, handlers) {
    const canvas = handlers && handlers.canvas;
    window.addEventListener('keydown', (e) => {
        if (e.code === 'Escape') {
            if (handlers && handlers.onEscape) handlers.onEscape();
            return;
        }
        const k = KEY_MAP[e.code];
        if (k) {
            input[k] = true;
            if (e.code === 'Space') e.preventDefault();
        }
    });
    window.addEventListener('keyup', (e) => {
        const k = KEY_MAP[e.code];
        if (k) input[k] = false;
    });
    // Мышь: позиция курсора — цель для носа; ЛКМ — добыча.
    window.addEventListener('mousemove', (e) => {
        input.aim = { x: e.clientX, y: e.clientY };
    });
    // Курсор ушёл из окна — наведения носа нет (легенда гаснет, §4.2 п.7).
    window.addEventListener('mouseleave', () => { input.aim = null; });
    window.addEventListener('mousedown', (e) => {
        // ЛКМ по сцене — добыча; клик по HUD-кнопкам бурение не запускает.
        if (e.button === 0 && (!canvas || e.target === canvas)) input.mouseDown = true;
    });
    window.addEventListener('mouseup', (e) => {
        if (e.button === 0) input.mouseDown = false;
    });
    // ПКМ в сцене отключена — контекстное меню на канвасе подавлено (§5.2).
    if (canvas) canvas.addEventListener('contextmenu', (e) => e.preventDefault());
}
