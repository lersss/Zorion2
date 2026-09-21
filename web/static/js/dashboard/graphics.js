// web/static/js/dashboard/graphics.js
// Вкладка «⚙️ Графика» дашборда (идея 2026-09-22 «внешний вид звёзд»,
// UI @uidesigner §1–§8). Настройки вида карты миров хранятся только в
// localStorage (сервер не участвует) — на каждом компьютере свои, без срока
// давности; карта их только читает. Пять вкладок дашборда — ожидаемое
// обновление QA-чеклиста B-MAP-14 (не регрессия).
//
// Модуль не трогает document/localStorage на верхнем уровне (Node-граф фронта,
// web/frontend_test.go): вся работа — внутри initGraphicsSettings().

// Наборы эффектов вида звёзд — в чистом модуле данных map/star_presets.js
// (единый источник с картой, чтобы PRESETS и PRESET_EFFECTS не расходились).
// Импорт безопасен: модуль без DOM/localStorage; star_render.js и map/config.js
// сюда тянуть нельзя (config.js на верхнем уровне берёт #mapCanvas).
import { PRESET_EFFECTS } from '../map/star_presets.js';

// ==================== КЛЮЧИ И ЗНАЧЕНИЯ ====================

// Существующие ключи не переименовываем (иначе выбор игроков сбросится);
// starVisualTwinkle/starVisualLite — новые (идея §5).
const KEY_PRESET = 'starVisualPreset';
const KEY_LITE = 'starVisualLite';
const KEY_TWINKLE = 'starVisualTwinkle';
const KEY_IGNITE = 'starVisualIgnite';
const KEY_ADDITIVE = 'starVisualAdditive';
const KEY_EXOTIC = 'starVisualExotic';
const KEY_PETALS = 'starVisualPetals';
const KEY_RADAR = 'radarBoundaryVariant';

// Наборы эффектов вида звёзд (§3): смена вида сбрасывает чекбоксы к набору
// этого вида; дальнейшие правки — явные и сохраняются. Определения — в
// map/star_presets.js (PRESET_EFFECTS), здесь только константы вкладки.
const PRESET_DEFAULT = 'sprite';   // решение создателя 2026-09-22, гейт 2
const RADAR_DEFAULT = '1';         // «туман»
const RADAR_VALUES = ['1', '2', '3', '4'];

// ==================== localStorage (try/catch, §7) ====================

// safeGet/safeSet — storage может быть недоступен (приватный режим) или
// вычищен: падать нельзя, но и молчать нельзя — ошибку показывает строка-статус.
function safeGet(key) {
    try { return localStorage.getItem(key); } catch (e) { return null; }
}

function safeSet(key, value) {
    try { localStorage.setItem(key, value); return true; } catch (e) { return false; }
}

// ==================== ЧТЕНИЕ С НОРМАЛИЗАЦИЕЙ (§7) ====================

// normalizedPreset — валидный вид звёзд; неизвестное значение нормализуется к
// дефолту и перезаписывается. «Корона» (crown) — валидный вид (реализована).
function normalizedPreset() {
    const v = safeGet(KEY_PRESET);
    if (PRESET_EFFECTS[v]) return v;
    if (v !== null && v !== PRESET_DEFAULT) safeSet(KEY_PRESET, PRESET_DEFAULT);
    return PRESET_DEFAULT;
}

// normalizedRadar — вариант границы радара 1–4; вне диапазона → «туман».
function normalizedRadar() {
    const v = safeGet(KEY_RADAR);
    if (RADAR_VALUES.includes(v)) return v;
    if (v !== null && v !== RADAR_DEFAULT) safeSet(KEY_RADAR, RADAR_DEFAULT);
    return RADAR_DEFAULT;
}

// effectOn — состояние эффекта: явный ключ игрока, иначе набор вида звёзд.
function effectOn(key, fallback) {
    const v = safeGet(key);
    if (v === '1') return true;
    if (v === '0') return false;
    return fallback;
}

// ==================== ИНИЦИАЛИЗАЦИЯ ВКЛАДКИ ====================

// initGraphicsSettings — обвязка контролов вкладки. Вызывается из index.html
// после загрузки DOM (модуль остаётся Node-безопасным).
export function initGraphicsSettings() {
    const root = document.getElementById('tab-graphics');
    if (!root) return;

    const el = {
        liteOff: document.getElementById('graphicsLiteOff'),
        liteOn: document.getElementById('graphicsLiteOn'),
        presetRadios: root.querySelectorAll('input[name="graphicsPreset"]'),
        radarRadios: root.querySelectorAll('input[name="graphicsRadar"]'),
        twinkle: document.getElementById('graphicsTwinkle'),
        ignite: document.getElementById('graphicsIgnite'),
        additive: document.getElementById('graphicsAdditive'),
        exotic: document.getElementById('graphicsExotic'),
        petals: document.getElementById('graphicsPetals'),
        viewBlock: document.getElementById('graphics-view-block'),
        effectsBlock: document.getElementById('graphics-effects-block'),
        liteNote: document.getElementById('graphics-lite-note'),
        status: document.getElementById('graphics-status'),
        resetBtn: document.getElementById('graphicsResetBtn'),
        openMapBtn: document.getElementById('graphicsOpenMapBtn'),
    };
    if (!el.liteOff || !el.presetRadios.length) return;

    let storageOk = true;

    function persist(key, value) {
        if (!safeSet(key, value)) storageOk = false;
        return storageOk;
    }

    function status(ok) {
        if (!el.status) return;
        el.status.textContent = !ok || !storageOk
            ? 'Настройки не сохраняются в этом браузере'
            : '✅ Сохранено';
    }

    // syncUI — отражает localStorage в контролах (§7): валидные значения, режим
    // «Для слабых ПК», приглушение блоков 2–3.
    function syncUI() {
        const preset = normalizedPreset();
        const lite = safeGet(KEY_LITE) === '1';
        const fx = PRESET_EFFECTS[preset];

        el.liteOn.checked = lite;
        el.liteOff.checked = !lite;
        el.presetRadios.forEach(r => {
            r.checked = r.value === preset;
            r.disabled = lite;
        });
        el.radarRadios.forEach(r => { r.checked = r.value === normalizedRadar(); });

        el.twinkle.checked = effectOn(KEY_TWINKLE, fx.twinkle);
        el.ignite.checked = effectOn(KEY_IGNITE, fx.ignite);
        el.additive.checked = effectOn(KEY_ADDITIVE, fx.additive);
        el.exotic.checked = effectOn(KEY_EXOTIC, fx.exotic);
        el.petals.checked = effectOn(KEY_PETALS, fx.petals);

        [el.viewBlock, el.effectsBlock].forEach(b => b && b.classList.toggle('graphics-block-off', lite));
        [el.twinkle, el.ignite, el.additive, el.exotic, el.petals].forEach(c => { c.disabled = lite; });
        if (el.liteNote) el.liteNote.hidden = !lite;
    }

    // writePreset — смена вида звёзд: пишем пресет и сбрасываем эффекты к его
    // набору (§3). Ручные правки после этого — явные и сохраняются.
    function writePreset(preset) {
        const fx = PRESET_EFFECTS[preset];
        if (!fx) return;
        persist(KEY_PRESET, preset);
        persist(KEY_TWINKLE, fx.twinkle ? '1' : '0');
        persist(KEY_IGNITE, fx.ignite ? '1' : '0');
        persist(KEY_ADDITIVE, fx.additive ? '1' : '0');
        persist(KEY_EXOTIC, fx.exotic ? '1' : '0');
        persist(KEY_PETALS, fx.petals ? '1' : '0');
        syncUI();
        status(true);
    }

    el.liteOff.addEventListener('change', () => {
        persist(KEY_LITE, el.liteOff.checked ? '0' : '1');
        syncUI();
        status(true);
    });
    el.liteOn.addEventListener('change', () => {
        persist(KEY_LITE, el.liteOn.checked ? '1' : '0');
        syncUI();
        status(true);
    });

    el.presetRadios.forEach(r => {
        r.addEventListener('change', () => { if (r.checked) writePreset(r.value); });
    });

    const effectBoxes = [
        [el.twinkle, KEY_TWINKLE],
        [el.ignite, KEY_IGNITE],
        [el.additive, KEY_ADDITIVE],
        [el.exotic, KEY_EXOTIC],
        [el.petals, KEY_PETALS],
    ];
    effectBoxes.forEach(([box, key]) => {
        box.addEventListener('change', () => {
            persist(key, box.checked ? '1' : '0');
            status(true);
        });
    });

    el.radarRadios.forEach(r => {
        r.addEventListener('change', () => {
            if (!r.checked) return;
            persist(KEY_RADAR, r.value);
            status(true);
        });
    });

    if (el.openMapBtn) {
        el.openMapBtn.addEventListener('click', () => { window.location.href = '/map'; });
    }

    if (el.resetBtn) {
        el.resetBtn.addEventListener('click', () => {
            persist(KEY_PRESET, PRESET_DEFAULT);
            persist(KEY_LITE, '0');
            persist(KEY_RADAR, RADAR_DEFAULT);
            const fx = PRESET_EFFECTS[PRESET_DEFAULT];
            persist(KEY_TWINKLE, fx.twinkle ? '1' : '0');
            persist(KEY_IGNITE, fx.ignite ? '1' : '0');
            persist(KEY_ADDITIVE, fx.additive ? '1' : '0');
            persist(KEY_EXOTIC, fx.exotic ? '1' : '0');
            persist(KEY_PETALS, fx.petals ? '1' : '0');
            syncUI();
            status(true);
        });
    }

    syncUI();
}
