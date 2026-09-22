// web/static/js/surface/surface_ui.js
// HUD/брифинг/смерть/пауза прогулки (спека 2026-09-21 §5.2/§7.5/§11).
// Все значения — из пакета прогулки (§7.1); HP — серверная формула (§8.7).
import { WEATHER_IDS, WEATHER_VISUALS } from './surface_config.js';
const $ = (id) => document.getElementById(id);

function fmtTemp(celsius) {
    const sign = celsius >= 0 ? '+' : '';
    return sign + celsius.toFixed(0) + ' °C';
}

// suitVerdict — приговор скафандра по осям (брифинг §5.2): °C — только °C.
function suitVerdict(pkg) {
    const [tMin, tMax] = pkg.suit.temp_comfort_k;
    const [pMin, pMax] = pkg.suit.pressure_comfort_atm;
    const tC = pkg.temperature - 273.15;
    if (pkg.temperature > tMax) {
        return { level: 'bad', text: `Жарко: ${fmtTemp(tC)}` };
    }
    if (pkg.temperature < tMin) {
        return { level: 'bad', text: `Холодно: ${fmtTemp(tC)}` };
    }
    if (pkg.pressure_atm > pMax) {
        return { level: 'bad', text: `Высокое давление: ${pkg.pressure_atm.toFixed(1)} атм` };
    }
    if (pkg.pressure_atm > 0 && pkg.pressure_atm < pMin) {
        return { level: 'bad', text: `Разреженная атмосфера: ${pkg.pressure_atm.toFixed(2)} атм` };
    }
    return { level: 'ok', text: 'Комфортно' };
}

function hazardRow(label, value, unit) {
    const active = value > 0.0001;
    const color = active ? '#f97316' : '#64748b';
    const val = active ? ` · ${value.toFixed(3)} HP/с` : '';
    return `<span style="color:${color}; margin-right:12px;">${label}${val}${unit || ''}</span>`;
}

// showBriefing — первый экран прогулки (§5.2 п.2): биом, доля, приговор, оси.
export function showBriefing(pkg, handlers) {
    const verdict = suitVerdict(pkg);
    const verdictColor = verdict.level === 'ok' ? '#4ade80' : '#f97316';
    const cat = pkg.biome_category || '';
    const axes = [
        hazardRow('🌡', pkg.hazard.temperature),
        hazardRow('⏲', pkg.hazard.pressure),
        hazardRow('☢', pkg.hazard.radiation),
        hazardRow('☣', pkg.hazard.toxic),
    ].join('');
    $('briefing-content').innerHTML = `
        <div style="display:flex; align-items:center; gap:12px; margin-bottom:12px;">
            <span style="display:inline-block; width:28px; height:28px; border-radius:8px; background:${pkg.biome_color}; border:1px solid #475569;"></span>
            <div>
                <div style="font-size:1.4rem; font-weight:600;">${pkg.biome_name || pkg.biome}</div>
                <div style="color:#94a3b8; font-size:0.9rem;">${cat}${pkg.biome_share ? ` · ${pkg.biome_share.toFixed(1)} % поверхности` : ''}</div>
            </div>
        </div>
        ${pkg.biome_description ? `<p style="color:#cbd5e1; line-height:1.5;">${pkg.biome_description}</p>` : ''}
        <p style="margin:10px 0;">Планета: <strong>${pkg.planet_name || ''}</strong> · гравитация ${(pkg.gravity || 0).toFixed(2)} g · температура ${fmtTemp(pkg.temperature - 273.15)}</p>
        <p style="margin:6px 0; color:#94a3b8;">Профиль опасности:</p>
        <p style="margin:4px 0 12px 0;">${axes}</p>
        <p style="margin:8px 0; color:${verdictColor}; font-weight:600;">Скафандр: ${verdict.text}</p>
        <p style="margin:8px 0; color:#94a3b8; font-size:0.9rem;">${pkg.life ? 'На планете есть жизнь — присмотритесь.' : 'Мир стерилен: тишина, ветер, пыль.'}</p>
    `;
    $('brief-continue').onclick = handlers.onContinue;
    $('brief-leave').onclick = handlers.onLeave;
    $('briefing').style.display = 'flex';
}

export function hideBriefing() { $('briefing').style.display = 'none'; }

export function showLoading(text) {
    $('loading-text').textContent = text || 'Загрузка…';
    $('loading').style.display = 'flex';
}
export function hideLoading() { $('loading').style.display = 'none'; }

export function showHUD() { $('hud').style.display = 'block'; }

// ---- Админский переключатель погоды (идея 2026-09-21 §3) ----
// Вторая строка .hud-center под #hud-weather: подпись + чипы «авто» и всех
// явлений WEATHER_IDS. У игрока строки нет ВООБЩЕ (функция не вызывается).
// Кликабельность даёт существующее правило `#hud button { pointer-events:auto }`.
const WEATHER_CHIP_CSS = 'background:rgba(15,23,42,0.72); border:1px solid #334155; border-radius:8px; padding:3px 10px; font:inherit; font-size:0.75rem; color:#cbd5e1; cursor:pointer;';
const WEATHER_CHIP_ON = { border: 'rgba(250,204,21,0.5)', color: '#facc15', background: 'rgba(250,204,21,0.12)' };
const WEATHER_CHIP_OFF = { border: '#334155', color: '#cbd5e1', background: 'rgba(15,23,42,0.72)' };

function paintWeatherChip(btn, active) {
    const c = active ? WEATHER_CHIP_ON : WEATHER_CHIP_OFF;
    btn.style.borderColor = c.border;
    btn.style.color = c.color;
    btn.style.background = c.background;
}

export function showWeatherToggle(onPick) {
    if ($('hud-weather-admin')) return;
    const center = document.querySelector('.hud-center');
    if (!center) return;
    const wrap = document.createElement('div');
    wrap.id = 'hud-weather-admin';
    wrap.style.cssText = 'display:flex; flex-wrap:wrap; align-items:center; justify-content:center; gap:6px; margin-top:6px;';
    const label = document.createElement('span');
    label.textContent = '⚙ погода (админ)';
    label.style.cssText = 'color:#64748b; font-size:0.75rem;';
    wrap.appendChild(label);
    // Подпись чипа — человеческое имя явления (`label`, UI §3/§5); `data-weather`
    // остаётся id (контракт переключения). Неизвестный id → текст = id.
    const items = [{ id: '', label: 'авто', title: 'Погода меняется сама, раз в 2–4 мин — как у игрока' }]
        .concat(WEATHER_IDS.map((id) => ({ id, label: (WEATHER_VISUALS[id] && WEATHER_VISUALS[id].label) || id })));
    items.forEach((it) => {
        const btn = document.createElement('button');
        btn.type = 'button';
        btn.dataset.weather = it.id;
        btn.textContent = it.label;
        if (it.title) btn.title = it.title;
        btn.style.cssText = WEATHER_CHIP_CSS;
        paintWeatherChip(btn, false);
        btn.addEventListener('mouseenter', () => { if (btn.dataset.active !== '1') btn.style.background = '#334155'; });
        btn.addEventListener('mouseleave', () => paintWeatherChip(btn, btn.dataset.active === '1'));
        btn.addEventListener('click', () => onPick(it.id));
        wrap.appendChild(btn);
    });
    center.appendChild(wrap);
}

// setWeatherToggleActive — подсветка активного чипа ('' = «авто»).
export function setWeatherToggleActive(activeId) {
    const wrap = $('hud-weather-admin');
    if (!wrap) return;
    wrap.querySelectorAll('button').forEach((btn) => {
        const active = btn.dataset.weather === activeId;
        btn.dataset.active = active ? '1' : '';
        paintWeatherChip(btn, active);
    });
}

// ---- Админский переключатель фазы суток (спека 2026-09-22 §6.5) ----
// Вторая строка .hud-center под #hud-weather-admin: «сутки (админ)» + «авто» и
// 4 фазы. Стиль чипов — тот же, что у погоды (не дублируем). У игрока строки нет.
const ENV_PHASES = [
    { id: 'рассвет', label: 'Рассвет' },
    { id: 'день', label: 'День' },
    { id: 'закат', label: 'Закат' },
    { id: 'ночь', label: 'Ночь' },
];
const ENV_LABELS = { 'рассвет': 'Рассвет', 'день': 'День', 'закат': 'Закат', 'ночь': 'Ночь' };

// envPhaseLabel — тихая подпись фазы у игрока (§6.2, вариант B).
export function envPhaseLabel(phase) { return ENV_LABELS[phase] || ''; }

export function showEnvToggle(onPick) {
    if ($('hud-env-admin')) return;
    const center = document.querySelector('.hud-center');
    if (!center) return;
    const wrap = document.createElement('div');
    wrap.id = 'hud-env-admin';
    wrap.style.cssText = 'display:flex; flex-wrap:wrap; align-items:center; justify-content:center; gap:6px; margin-top:6px;';
    const label = document.createElement('span');
    label.textContent = 'сутки (админ)';
    label.style.cssText = 'color:#64748b; font-size:0.75rem;';
    wrap.appendChild(label);
    const items = [{ id: '', label: 'авто', title: 'Сутки идут сами, как у игрока' }]
        .concat(ENV_PHASES.map((p) => ({ id: p.id, label: p.label, title: 'Зафиксировать фазу до конца прогулки' })));
    items.forEach((it) => {
        const btn = document.createElement('button');
        btn.type = 'button';
        btn.dataset.env = it.id;
        btn.textContent = it.label;
        if (it.title) btn.title = it.title;
        btn.style.cssText = WEATHER_CHIP_CSS;
        paintWeatherChip(btn, false);
        btn.addEventListener('mouseenter', () => { if (btn.dataset.active !== '1') btn.style.background = '#334155'; });
        btn.addEventListener('mouseleave', () => paintWeatherChip(btn, btn.dataset.active === '1'));
        btn.addEventListener('click', () => onPick(it.id));
        wrap.appendChild(btn);
    });
    center.appendChild(wrap);
}

// setEnvToggleActive — подсветка активного чипа фазы ('' = «авто»).
export function setEnvToggleActive(activeId) {
    const wrap = $('hud-env-admin');
    if (!wrap) return;
    wrap.querySelectorAll('button').forEach((btn) => {
        const active = btn.dataset.env === activeId;
        btn.dataset.active = active ? '1' : '';
        paintWeatherChip(btn, active);
    });
}

// updateHUD — полоса HP (серверная формула), имя биома, оси, путь, погода, фаза.
export function updateHUD(state) {
    const { hp, biomeName, hazard, distanceMeters, weather, env } = state;
    const pct = Math.max(0, Math.min(100, hp));
    const fill = $('hp-fill');
    fill.style.width = pct.toFixed(1) + '%';
    fill.style.background = pct > 50 ? '#4ade80' : pct > 20 ? '#facc15' : '#ef4444';
    $('hp-text').textContent = Math.round(pct) + ' / 100';
    $('hud-biome').textContent = biomeName || '';
    $('hud-distance').textContent = Math.round(distanceMeters) + ' м';
    $('hud-weather').textContent = weather || '';
    $('hud-env').textContent = env || '';

    const axes = [
        ['🌡', hazard.temperature],
        ['⏲', hazard.pressure],
        ['☢', hazard.radiation],
        ['☣', hazard.toxic],
    ];
    $('hud-axes').innerHTML = axes.map(([icon, v]) =>
        `<span title="${v > 0.0001 ? v.toFixed(3) + ' HP/с' : 'нет'} "
              style="opacity:${v > 0.0001 ? 1 : 0.35}; margin-right:6px;">${icon}</span>`
    ).join('');
}

export function showDeath(cause, onReturn) {
    $('death-cause').textContent = cause ? `Причина: ${cause}` : 'Прогулка окончена.';
    $('death-return').onclick = onReturn;
    $('death').style.display = 'flex';
}
export function hideDeath() { $('death').style.display = 'none'; }

export function showPause(onResume, onLeave) {
    $('pause-resume').onclick = onResume;
    $('pause-leave').onclick = onLeave;
    $('pause').style.display = 'flex';
}
export function hidePause() { $('pause').style.display = 'none'; }
export function isPaused() { return $('pause').style.display === 'flex'; }

let toastTimer = null;
export function notify(msg) {
    const el = $('toast');
    el.textContent = msg;
    el.style.display = 'block';
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => { el.style.display = 'none'; }, 3000);
}
