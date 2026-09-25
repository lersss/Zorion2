// web/static/js/map/flight.js
// Общая логика перелёта + панель в шапке карты.
// Используется и картой (tooltip/контекстное меню), и модалкой системы (ПКМ по звезде).

import { state, elements } from './config.js';
import { draw } from './map_render.js';
import { fetchWorldByID, handleUnauthorized, resetFlightReloadTimer, loadUserData } from './data.js';
import { notifyError } from '../ui/toast.js';
import { playSound, startFlightHum, playArrival } from '../ui/sound.js';

// ensureWorld — берёт мир из кэша или подгружает по ID (даже вне текущего кадра).
async function ensureWorld(id, token) {
    let w = state.worlds.find(x => x.id === id);
    if (!w) {
        w = await fetchWorldByID(id, token);
        if (w) state.worlds.push(w);
    }
    return w;
}

// ==================== ПАНЕЛЬ ПЕРЕЛЁТА ====================

export function showFlightPanel(fromName, toName) {
    const panel = document.getElementById('flight-panel');
    if (panel) panel.classList.add('flying');
    const from = document.getElementById('flightFrom');
    const to = document.getElementById('flightTo');
    if (from) from.textContent = fromName || '—';
    if (to) to.textContent = toName || '—';
    updateFlightPanel();
}

export function updateFlightPanel() {
    const bar = document.getElementById('flightBar');
    const time = document.getElementById('flightTime');
    const from = document.getElementById('flightFrom');
    const to = document.getElementById('flightTo');
    if (!bar || !time) return;
    if (state.isFlying) {
        const elapsed = (Date.now() - state.flyStartTime) / 1000;
        const p = Math.min(Math.max(elapsed / (state.flyDuration || 1), 0), 1);
        bar.style.width = Math.round(p * 100) + '%';
        const remaining = Math.max(0, state.flyDuration - elapsed);
        time.textContent = `${Math.ceil(remaining)} сек`;
        if (from) from.textContent = state.flyFrom ? state.flyFrom.name || '—' : '—';
        if (to) to.textContent = state.flyTo ? state.flyTo.name || '—' : '—';
    } else {
        bar.style.width = '0%';
        time.textContent = '—';
        if (from) from.textContent = '—';
        if (to) to.textContent = '—';
    }
    renderFlightBoost();
}

// ==================== УСКОРИТЕЛЬ (ЧК3, спека §3.4/§4.4) ====================

let boostWired = false; // обработчик клика навешен один раз
let boostLast = null;   // последнее отрисованное состояние (не трогаем DOM без нужды)
let meRefreshPending = false;
let accelAutoRefreshDone = false; // авто-refresh на истёкший откат — однократно (защита от цикла /me)

// fmtMMSS — секунды в «mm:ss» (таймер отката; спека §2/§4.4).
function fmtMMSS(sec) {
    const s = Math.max(0, Math.round(sec));
    return Math.floor(s / 60) + ':' + String(s % 60).padStart(2, '0');
}

// cooldownLeftS — локальный остаток отката от якоря (момент окончания, мс).
function cooldownLeftS(until) {
    if (!until) return 0;
    return Math.max(0, Math.ceil((until - Date.now()) / 1000));
}

// segmentRemainingS — остаток текущего сегмента; вне полёта null.
function segmentRemainingS() {
    if (!state.isFlying) return null;
    return Math.max(0, state.flyDuration - (Date.now() - state.flyStartTime) / 1000);
}

// minRemainingOfferS — порог показа ускорителя из параметров ТЕКУЩЕГО модуля
// (ship_catalog.params), фолбэк 180 (осн. §3.1); не из /me.accelerator.
function minRemainingOfferS() {
    const cat = Array.isArray(state.shipCatalog) ? state.shipCatalog : [];
    const id = (state.accelMe && state.accelMe.module_id)
        || (state.accel && state.accel.module_id);
    const item = id ? cat.find(i => i.id === id) : null;
    const v = item && item.params && Number(item.params.min_remaining_offer_s);
    return v > 0 ? v : 180;
}

// refreshMeForBoost — перечитать /me и перерисовать кнопку (visibilitychange и
// истечение локального отката). Идемпотентно: без наложенных запросов.
function refreshMeForBoost() {
    if (meRefreshPending) return;
    meRefreshPending = true;
    loadUserData(true)
        .then(() => renderFlightBoost())
        .catch(() => {})
        .finally(() => { meRefreshPending = false; });
}

// meBoostState — состояние кнопки из /me.accelerator + ship_catalog (best-effort,
// когда свежего /travel нет: возврат с карты/после мини-игры; сервер — авторитет).
function meBoostState() {
    const me = state.accelMe;
    if (!me || !me.module_id) return { show: false };
    if (me.active) {
        return {
            show: true, disabled: true,
            text: 'Ускорение уже действует',
            note: 'Смена цели или прибытие сбросит — тогда можно снова',
        };
    }
    const left = cooldownLeftS(state.accelMeCooldownUntil);
    if (left > 0) {
        return {
            show: true, disabled: true,
            text: 'Откат ' + fmtMMSS(left),
            note: 'Откат идёт — ускоритель перезаряжается',
        };
    }
    const remain = segmentRemainingS();
    if (remain != null && remain < minRemainingOfferS()) {
        return { show: true, disabled: true, text: 'Перелёт слишком короткий', note: '' };
    }
    return { show: true, disabled: false, text: 'Ускорить', note: '' };
}

// boostButtonState — приоритет: свежий /travel (state.accel), иначе /me.
function boostButtonState() {
    const accel = state.accel;
    if (accel) {
        if (accel.available) {
            return { show: true, disabled: false, text: 'Ускорить', note: '' };
        }
        switch (accel.reason) {
            case 'already_active':
                return {
                    show: true, disabled: true,
                    text: 'Ускорение уже действует',
                    note: 'Смена цели или прибытие сбросит — тогда можно снова',
                };
            case 'too_short':
                return { show: true, disabled: true, text: 'Перелёт слишком короткий', note: '' };
            case 'cooldown': {
                const left = cooldownLeftS(state.accelCooldownUntil);
                if (left > 0) {
                    return {
                        show: true, disabled: true,
                        text: 'Откат ' + fmtMMSS(left),
                        note: 'Откат идёт — ускоритель перезаряжается',
                    };
                }
                // Откат из /travel досчитан до нуля — уточняем по /me, но не
                // более одного раза: при отсутствии модуля/деградации прячем
                // блок, а не крутим опрос /me каждый кадр (защита от цикла).
                const me = meBoostState();
                if (me.show) return me;
                if (!accelAutoRefreshDone) {
                    accelAutoRefreshDone = true;
                    refreshMeForBoost();
                    return { show: true, disabled: true, text: 'Откат 0:00', note: 'Обновляем…' };
                }
                return { show: false };
            }
            default:
                // no_module / unknown_game — панель ускоритель не предлагает.
                return { show: false };
        }
    }
    return meBoostState();
}

// renderFlightBoost — отрисовать блок #flight-boost по текущему состоянию.
export function renderFlightBoost() {
    const wrap = document.getElementById('flight-boost');
    const btn = document.getElementById('flight-boost-btn');
    const note = document.getElementById('flight-boost-note');
    if (!wrap || !btn || !note) return;
    const st = boostButtonState();
    const show = !!st.show;
    const text = st.text || '';
    const noteText = st.note || '';
    const disabled = !!st.disabled;
    if (show && !boostWired) {
        btn.addEventListener('click', () => { window.location.href = '/route.html'; });
        boostWired = true;
    }
    const last = boostLast;
    if (last && last.show === show && last.text === text
        && last.note === noteText && last.disabled === disabled) {
        return;
    }
    boostLast = { show, text, note: noteText, disabled };
    wrap.classList.toggle('visible', show);
    if (!show) {
        note.textContent = '';
        return;
    }
    btn.disabled = disabled;
    btn.textContent = text;
    note.textContent = noteText;
}

// Перечитываем /me при возврате во вкладку во время полёта (спека §3.4):
// откат/готовность могли измениться, пока вкладка была скрыта.
document.addEventListener('visibilitychange', () => {
    if (document.visibilityState !== 'visible') return;
    if (!state.isFlying) return;
    if (state.accel && state.accel.reason !== 'cooldown') return;
    // Возврат во вкладку — явный повод перечитать /me ещё раз (якорь сбрасываем).
    accelAutoRefreshDone = false;
    refreshMeForBoost();
});


export function hideFlightPanel() {
    // Панель скрывается, когда полёт завершён.
    const panel = document.getElementById('flight-panel');
    if (panel) panel.classList.remove('flying');
    // Нет полёта — нет действия ускорителя: чистим блок и состояние (ЧК3).
    state.accel = null;
    state.accelMe = null;
    state.accelCooldownUntil = 0;
    state.accelMeCooldownUntil = 0;
    boostLast = null;
    accelAutoRefreshDone = false;
    updateFlightPanel();
    // Конец межзвёздного сегмента. Композитный маршрут (маркер compositeRoute)
    // продолжается внутрисистемным сегментом (99.2.30/99.2.27): на стыке
    // сегментов гул не глушим и «прибытие» не сигналим — оно только в конце
    // маршрута (ловушка 5), чтобы не дублировать тост notifyInfo('🚀 Прибыли…').
    if (!sessionStorage.getItem('compositeRoute')) {
        playArrival();
    }
}

// showFlightLabel — шапка карты в полёте: «В полёте: From → To» (61a).
export function showFlightLabel(fromName, toName) {
    const prefix = document.getElementById('currentWorldPrefix');
    const name = document.getElementById('currentWorldName');
    if (prefix) prefix.textContent = 'В полёте:';
    if (name) name.textContent = (fromName || '—') + ' → ' + (toName || '—');
}

// ==================== НАЧАЛО ПЕРЕЛЁТА ====================

// startFlight — отправляет /travel, подгружает миры, запускает полёт и показывает панель.
// destination — необязательная цель композитного маршрута (спека 99.2.30 §3.1):
// {object_type: 'planet'|'satellite'|'companion', object_id} — тело {world_id, destination}.
// Возвращает true при успехе, false при ошибке.
export async function startFlight(worldId, token, destination) {
    if (!worldId) {
        notifyError('Не выбран мир назначения');
        return false;
    }
    if (!token) {
        handleUnauthorized();
        return false;
    }
    try {
        const res = await fetch('/travel', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'Authorization': 'Bearer ' + token
            },
            body: JSON.stringify(destination
                ? { world_id: worldId, destination }
                : { world_id: worldId })
        });
        const text = await res.text();
        if (!res.ok) {
            notifyError('Ошибка: ' + text);
            return false;
        }
        const data = JSON.parse(text);

        const [fromWorld, toWorld] = await Promise.all([
            ensureWorld(data.from, token),
            ensureWorld(data.to, token),
        ]);
        if (!fromWorld || !toWorld) {
            notifyError('Не удалось загрузить данные миров для перелёта');
            return false;
        }

        state.flyFrom = fromWorld;
        state.flyTo = toWorld;
        state.flyDuration = data.duration;
        state.flyStartX = data.start_x;
        state.flyStartY = data.start_y;
        state.flyStartTime = data.start_time;
        state.isFlying = true;
        // Блок ускорителя из /travel (ЧК3, спека §3.4): отдаётся на обоих
        // путях (оба 202). Причина/доступность — для кнопки «Ускорить»;
        // cooldown_remaining_s — якорь локального таймера отката.
        state.accel = data.accelerator || null;
        state.accelCooldownUntil = (state.accel && state.accel.cooldown_remaining_s > 0)
            ? Date.now() + state.accel.cooldown_remaining_s * 1000
            : 0;
        accelAutoRefreshDone = false;
        // Фикс 33c: новый полёт начинается с чистого полётного таймера
        // перезапросов — первая подгрузка кластеров не пропускается гардом.
        resetFlightReloadTimer();

        elements.tooltip.classList.remove('active');
        showFlightPanel(fromWorld.name, toWorld.name);
        showFlightLabel(fromWorld.name, toWorld.name);
        // Звук полёта: старт (flight_start) + зацикленный гул (flight_hum).
        // startFlightHum идемпотентен — повторный старт (редирект/разворот 61a)
        // не наслаивает гул (ловушка 6).
        playSound('flight_start');
        startFlightHum();
        draw();
        return true;
    } catch (e) {
        notifyError('Ошибка: ' + e.message);
        return false;
    }
}

// returnToMapInFlight — «вернулся на карту в полёте» (спека 99.2.30 §6.7):
// слежение как кнопка «🎯 Найти меня» карты (centerBtn.click() — followShip +
// sessionStorage.followShip + centerOnAgent + подсветка, events.js:515-525).
// Общий для «Перелететь» и «Лететь · через систему» — единое поведение
// «лететь из модалки = камера карты ведёт корабль» (гейт создателя, идея §8
// п.7в). В админке (карты нет) — no-op.
export function returnToMapInFlight() {
    const centerBtn = document.getElementById('centerBtn');
    if (centerBtn) centerBtn.click();
}
