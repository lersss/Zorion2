// web/static/js/belt/belt_input.js
// Ввод мини-игры добычи в поясе (UI-спека
// 2026-09-22-пояса-малых-тел-этап-3-добыча-ui §4.2 п.7): тяга по 4
// направлениям (WASD/стрелки), тормоз — Shift, добыча — Space, Esc — пауза.
// Клавиши пишутся в переданный объект input (читает физика и легенда HUD).
const KEY_MAP = {
    ArrowUp: 'up', KeyW: 'up', ArrowDown: 'down', KeyS: 'down',
    ArrowLeft: 'left', KeyA: 'left', ArrowRight: 'right', KeyD: 'right',
    ShiftLeft: 'brake', ShiftRight: 'brake', Space: 'space',
};

// bindInput — навешивает обработчики; handlers.onEscape — реакция на Esc
// (пауза/продолжить). Повторный вызов не предусмотрен (страница одна).
export function bindInput(input, handlers) {
    window.addEventListener('keydown', (e) => {
        if (e.code === 'Escape') {
            if (handlers && handlers.onEscape) handlers.onEscape();
            return;
        }
        const k = KEY_MAP[e.code];
        if (k) {
            input[k] = true;
            if (e.code === 'Space' || e.code.startsWith('Arrow')) e.preventDefault();
        }
    });
    window.addEventListener('keyup', (e) => {
        const k = KEY_MAP[e.code];
        if (k) input[k] = false;
    });
}
