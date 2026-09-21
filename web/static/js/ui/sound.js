// web/static/js/ui/sound.js
// Звук отклика интерфейса и звук полёта — проигрывание аудиофайлов
// (web/static/audio/, Kenney CC0) через Web Audio.
//
// Почему Web Audio, а не <audio loop> (ловушка 8): обычный <audio loop> вставляет
// разрыв ~0.3 с на стыке повтора — гул «рвётся». AudioBufferSourceNode с loop = true
// зацикливает бесшовно; плюс буферы декодируются заранее (нет задержки на первом
// проигрывании) и AudioContext.resume() по первому жесту чинит автоплей/F5 (ловушка 7).
//
// Безопасность для Node-графа админки (web/frontend_test.go):
// TestAdminFrontendLoadsInNode исполняет граф admin/main.js (→ ui/toast.js →
// этот модуль) в Node под DOM-стабом без AudioContext. Поэтому ни одного обращения
// к document/AudioContext/fetch на верхнем уровне — всё лениво, внутри функций,
// под guard'ом. Guard — именно typeof AudioContext (в стабе frontend_test.go
// window === globalThis, проверка window не спасает). Ошибки resume()/загрузки
// глушатся в try/catch + p.catch(()=>{}) — ни одного unhandled rejection (иначе
// падает e2e-смоук).
//
// Админка молчит: звук играет только на странице карты, которая явно вызывает
// activateSound(); до активации все play-функции — no-op.

const STORAGE_KEY = 'zorion.sound';
const DEFAULT_VOLUME = 0.4;
const HUM_FADE_SEC = 0.2;

// Карта ролей → файлы (единственное место с путями; вызывающие используют роль).
// Выбор создателя по демо-странице (2026-09-21): interaction / confirm / error.
const ROLE_FILE = {
    ui_open: '/static/audio/interaction.ogg',
    ui_select: '/static/audio/interaction.ogg',
    ui_success: '/static/audio/confirm.ogg',
    ui_error: '/static/audio/error.ogg',
    flight_start: '/static/audio/confirm.ogg',
    flight_arrive: '/static/audio/confirm.ogg',
    flight_hum: '/static/audio/flight_hum.ogg',
};

let activated = false;
let enabled = true;
let volume = DEFAULT_VOLUME;
let settingsLoaded = false;

let ctx = null;            // AudioContext (лениво)
let masterGain = null;     // мастер-громкость (вкл/выкл + volume)
const buffers = {};        // src → AudioBuffer (кэш)
const loading = {};        // src → Promise<AudioBuffer|null>
let humRequested = false;  // гул логически нужен (startFlightHum без stop)
let humSource = null;      // текущий зацикленный AudioBufferSourceNode
let humGain = null;        // GainNode гула (для плавного затухания)
let gestureHandler = null; // одноразовый слушатель первого жеста (F5/resume)

// ==================== СОСТОЯНИЕ (localStorage) ====================

function loadSettingsOnce() {
    if (settingsLoaded) return;
    settingsLoaded = true;
    try {
        if (typeof localStorage === 'undefined') return;
        const raw = localStorage.getItem(STORAGE_KEY);
        if (!raw) return;
        const saved = JSON.parse(raw);
        if (saved && typeof saved === 'object') {
            if (typeof saved.enabled === 'boolean') enabled = saved.enabled;
            if (typeof saved.volume === 'number' && isFinite(saved.volume)) {
                volume = Math.min(1, Math.max(0, saved.volume));
            }
        }
    } catch (e) { /* localStorage недоступен/битый JSON — дефолты */ }
}

function saveSettings() {
    try {
        if (typeof localStorage === 'undefined') return;
        localStorage.setItem(STORAGE_KEY, JSON.stringify({ enabled, volume }));
    } catch (e) { /* тихий сбой */ }
}

// ==================== АУДИО-КОНТЕКСТ (лениво) ====================

function audioAvailable() {
    return typeof AudioContext !== 'undefined';
}

function getAudioCtor() {
    if (typeof AudioContext !== 'undefined') return AudioContext;
    if (typeof window !== 'undefined' && window.webkitAudioContext) return window.webkitAudioContext;
    return null;
}

function ensureContext() {
    if (ctx) return ctx;
    const Ctor = getAudioCtor();
    if (!Ctor) return null;
    try {
        ctx = new Ctor();
    } catch (e) {
        ctx = null;
        return null;
    }
    try {
        masterGain = ctx.createGain();
        masterGain.gain.value = enabled ? volume : 0;
        masterGain.connect(ctx.destination);
    } catch (e) {
        masterGain = null;
    }
    return ctx;
}

function resumeContext() {
    try {
        if (ctx && ctx.state === 'suspended' && typeof ctx.resume === 'function') {
            const p = ctx.resume();
            if (p && typeof p.catch === 'function') p.catch(() => {});
        }
    } catch (e) { /* тихий сбой */ }
}

// ==================== БУФЕРЫ (ленивая загрузка + предзагрузка) ====================

function loadBuffer(src) {
    if (buffers[src]) return Promise.resolve(buffers[src]);
    if (loading[src]) return loading[src];
    const c = ensureContext();
    if (!c || !src || typeof fetch !== 'function') return Promise.resolve(null);
    const p = fetch(src)
        .then(r => {
            if (!r.ok) throw new Error('HTTP ' + r.status);
            return r.arrayBuffer();
        })
        .then(ab => c.decodeAudioData(ab))
        .then(buf => {
            buffers[src] = buf;
            return buf;
        })
        .catch(() => null);
    loading[src] = p;
    return p;
}

// preloadAll — прогреть буферы всех ролей (уникальные файлы) заранее, чтобы не
// было задержки на первом проигрывании.
function preloadAll() {
    const seen = {};
    for (const role in ROLE_FILE) {
        const src = ROLE_FILE[role];
        if (seen[src]) continue;
        seen[src] = true;
        loadBuffer(src);
    }
}

// ==================== ВОСПРОИЗВЕДЕНИЕ ====================

function playBuffer(buf) {
    if (!ctx || !masterGain || !buf) return;
    try {
        const src = ctx.createBufferSource();
        src.buffer = buf;
        src.connect(masterGain);
        src.start(0);
    } catch (e) { /* тихий сбой звука */ }
}

// playSound — разовый короткий звук по роли. No-op до активации страницы,
// при выключенном звуке и без аудио-окружения.
export function playSound(role) {
    if (!activated || !enabled) return;
    const src = ROLE_FILE[role];
    if (!src) return;
    if (!ensureContext() || !masterGain) return;
    resumeContext();
    const buf = buffers[src];
    if (buf) {
        playBuffer(buf);
        return;
    }
    // Не прогрет — догружаем и играем, когда буфер будет готов.
    loadBuffer(src).then(b => {
        if (b && activated && enabled) playBuffer(b);
    });
}

// playToast — сигнал на тост. В2=А: ошибка → ui_error, успех → ui_success;
// info молчит. В админке (нет активации) — no-op.
export function playToast(type) {
    if (type === 'error') playSound('ui_error');
    else if (type === 'success') playSound('ui_success');
}

// ==================== ГУЛ ПОЛЁТА (Web Audio loop, фикс F5) ====================

function startHumSource(buf) {
    if (!ctx || !masterGain || humSource) return;
    try {
        const src = ctx.createBufferSource();
        src.buffer = buf;
        src.loop = true;             // бесшовное зацикливание (ловушка 8)
        const g = ctx.createGain();
        g.gain.value = 1;
        src.connect(g);
        g.connect(masterGain);
        src.start(0);
        humSource = src;
        humGain = g;
    } catch (e) {
        humSource = null;
        humGain = null;
    }
}

// tryStartHum — поднять гул, если он нужен и ещё не звучит. Идемпотентно.
// Если контекст suspended — источник всё равно стартует и зазвучит после
// resume() на первом жесте (фикс F5, ловушка 7).
function tryStartHum() {
    if (!humRequested || !enabled || !activated || humSource) return;
    if (!ensureContext() || !masterGain) return;
    const src = ROLE_FILE.flight_hum;
    const buf = buffers[src];
    if (buf) {
        startHumSource(buf);
        return;
    }
    loadBuffer(src).then(b => {
        if (b && humRequested && enabled && !humSource) startHumSource(b);
    });
}

// installGestureRetry / removeGestureRetry — одноразовый слушатель первого
// жеста игрока (автоплей/F5): вешается при активации, снимается после жеста.
// На жесте делает resume() контекста и поднимает «ожидающий» гул.
function installGestureRetry() {
    if (gestureHandler) return;
    if (typeof document === 'undefined' || typeof document.addEventListener !== 'function') return;
    gestureHandler = () => {
        removeGestureRetry();
        resumeContext();
        if (humRequested && !humSource) tryStartHum();
    };
    try {
        document.addEventListener('pointerdown', gestureHandler);
        document.addEventListener('keydown', gestureHandler);
        document.addEventListener('touchstart', gestureHandler);
    } catch (e) { /* тихо */ }
}

function removeGestureRetry() {
    if (!gestureHandler) return;
    const h = gestureHandler;
    gestureHandler = null;
    try {
        document.removeEventListener('pointerdown', h);
        document.removeEventListener('keydown', h);
        document.removeEventListener('touchstart', h);
    } catch (e) { /* тихо */ }
}

// startFlightHum — зацикленный гул полёта. Идемпотентно: пока гул звучит,
// повторный вызов ничего не делает (не наслаивает) — ловушка 6.
export function startFlightHum() {
    if (!activated) return;
    humRequested = true;
    tryStartHum();
}

// fadeHum — плавное затухание гула (короткий ramp, без щелчка) и стоп источника.
function fadeHum() {
    if (!humSource) return;
    const src = humSource;
    const g = humGain;
    humSource = null;
    humGain = null;
    try {
        const now = ctx ? ctx.currentTime : 0;
        if (g && g.gain) {
            g.gain.cancelScheduledValues(now);
            g.gain.setValueAtTime(g.gain.value, now);
            g.gain.linearRampToValueAtTime(0, now + HUM_FADE_SEC);
        }
        src.stop(now + HUM_FADE_SEC + 0.05);
    } catch (e) { /* тихо */ }
}

// stopFlightHum — конец полёта: гасим гул и снимаем «ожидание».
export function stopFlightHum() {
    humRequested = false;
    fadeHum();
}

// playArrival — прибытие: стоп гула + один сигнал flight_arrive (confirm.ogg).
export function playArrival() {
    stopFlightHum();
    playSound('flight_arrive');
}

// ==================== УПРАВЛЕНИЕ ЗВУКОМ ====================

// activateSound — явная активация звука на странице (вызывает карта при
// инициализации). До неё все play-функции — no-op: админка (тот же ui/toast.js)
// гарантированно молчит. Создаёт контекст, греет буферы и ставит одноразовый
// retry на первый жест (resume + подъём «ожидающего» гула).
export function activateSound() {
    activated = true;
    ensureContext();
    preloadAll();
    installGestureRetry();
}

export function isSoundEnabled() {
    loadSettingsOnce();
    return enabled;
}

export function setSoundEnabled(on) {
    loadSettingsOnce();
    enabled = !!on;
    if (masterGain) {
        try { masterGain.gain.value = enabled ? volume : 0; } catch (e) { /* тихо */ }
    }
    if (enabled) tryStartHum();
    saveSettings();
}

export function toggleSound() {
    setSoundEnabled(!isSoundEnabled());
    return enabled;
}

export function setSoundVolume(v) {
    loadSettingsOnce();
    const n = Number(v);
    if (!isFinite(n)) return;
    volume = Math.min(1, Math.max(0, n));
    if (masterGain) {
        try { masterGain.gain.value = enabled ? volume : 0; } catch (e) { /* тихо */ }
    }
    saveSettings();
}

// ==================== DOM-ОБВЯЗКА ====================

// initSoundToggle — кнопка вкл/выкл звука в шапке карты: отражает состояние и
// переключает его. Выбор переживает F5 (localStorage).
export function initSoundToggle(buttonId) {
    if (typeof document === 'undefined' || typeof document.getElementById !== 'function') return;
    const btn = document.getElementById(buttonId || 'soundToggleBtn');
    if (!btn) return;
    const render = () => {
        const on = isSoundEnabled();
        btn.textContent = on ? '🔊 Звук' : '🔇 Звук';
        btn.title = on ? 'Звук включён — выключить' : 'Звук выключен — включить';
    };
    render();
    btn.addEventListener('click', () => {
        toggleSound();
        render();
    });
}
