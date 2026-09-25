// web/static/js/surface/surface_main.js
// Точка входа страницы прогулки (спека 2026-09-21): высадка (land) → брифинг →
// Canvas-игра (ходьба/прыжки/падение, HUD) → «вызвать корабль»/смерть (leave).
// Вход: /surface.html?planet=<uuid> (правый клик по планете → «Высадиться»).
import { CAMERA_LERP, WEATHER_MIN_MS, WEATHER_MAX_MS, PPM, ZOOM } from './surface_config.js';
import { SurfaceWorld } from './surface_world.js';
import { drawSky, drawFarRelief, drawHorizon, drawTerrain, drawDecor, drawShip, drawCreatures, drawPlayer } from './surface_render.js';
import { pickWeatherRun, drawWeatherBack, drawWeatherMid, drawWeatherFront, drawEmissive, weatherLabel } from './surface_weather.js';
import { SurfaceEnvironment, drawEnvironmentBack, drawEnvironmentMid, drawEnvironmentFront } from './surface_environment.js';
import { Player, serverHp } from './surface_player.js';
import { land, leave } from './surface_net.js';
import * as ui from './surface_ui.js';

const state = {
    pkg: null,
    world: null,
    player: null,
    camera: { x: 0, y: 0 },
    input: { left: false, right: false, jump: false, sprint: false },
    weather: null,
    weatherCycle: 0,
    forcedWeather: null, // админский выбор погоды: null = «авто» (идея 2026-09-21)
    env: null,           // слой среды — сутки (спека 2026-09-22 §6), f(seed, elapsed)
    running: false,
    dead: false,
    leaving: false,
    lastTime: 0,
};

// pickWeatherRun (surface_weather.js §4.4) — цикловой жребий из физики планеты
// (локальный mulberry32 внутри; Math.random запрещён, спека §12 п.10).
// Кроссфейд 0.6–1.2 с (§8) — между прошлым и новым явлением.
function scheduleWeather(now) {
    const prev = state.weather;
    const run = pickWeatherRun(state.pkg, state.weatherCycle, prev ? prev.id : null);
    const duration = WEATHER_MIN_MS + run.windowFrac * (WEATHER_MAX_MS - WEATHER_MIN_MS);
    const prevRun = prev ? { id: prev.id, variant: prev.variant, params: prev.params } : null;
    state.weather = {
        id: run.id,
        variant: run.variant,
        params: run.params,
        t0: now,
        until: now + duration,
        crossFrom: prevRun,
        crossT: prevRun ? now : null,
    };
    state.weatherCycle += 1;
}

// isAdminRole — роль игрока из пакета (§7.1, идея 2026-09-21): админский
// переключатель погоды — только admin/skycomposer.
function isAdminRole() {
    return !!state.pkg && (state.pkg.role === 'admin' || state.pkg.role === 'skycomposer');
}

// setWeather — админский выбор погоды: '' → «авто» (штатный цикл 2–4 мин);
// иначе выбранное явление держится до конца прогулки (таймер его не сменяет),
// полное с первого кадра, без кроссфейда (спека погоды §8).
// Сброс при перезагрузке — не храним (идея 2026-09-21 §3).
function setWeather(id) {
    if (id) {
        const run = pickWeatherRun(state.pkg, state.weatherCycle, null, id);
        state.weather = {
            id: run.id,
            variant: run.variant,
            params: run.params,
            t0: performance.now(),
            until: Infinity,
            crossFrom: null,
            crossT: null,
        };
        state.forcedWeather = id;
    } else {
        state.forcedWeather = null;
        scheduleWeather(performance.now());
    }
    ui.setWeatherToggleActive(id || '');
}

// setEnv — админский выбор фазы суток (§6.5): '' → «авто» (цикл от seed/elapsed);
// иначе фаза держится до конца прогулки, полный вид с первого кадра.
function setEnv(id) {
    if (!state.env) return;
    state.env.forced = id || null;
    ui.setEnvToggleActive(id || '');
}

function resize(canvas) {
    // Канвас в device-пикселях (devicePixelRatio) — резкость на HiDPI; логика
    // отрисовки остаётся в CSS-пикселях (vw/vh), базовый масштаб — в frame.
    const dpr = window.devicePixelRatio || 1;
    canvas.width = Math.round(window.innerWidth * dpr);
    canvas.height = Math.round(window.innerHeight * dpr);
    canvas.style.width = window.innerWidth + 'px';
    canvas.style.height = window.innerHeight + 'px';
}

function frame(now) {
    if (!state.running) return;
    const canvas = document.getElementById('surface-canvas');
    const ctx = canvas.getContext('2d');
    const dt = Math.min(0.05, (now - state.lastTime) / 1000 || 0);
    state.lastTime = now;

    const vw = window.innerWidth;
    const vh = window.innerHeight;
    const dpr = vw > 0 ? canvas.width / vw : 1;

    const paused = ui.isPaused();
    if (!paused && !state.dead) {
        state.player.update(dt, state.input);
        // Сутки идут только в активной прогулке (как таймер погоды) — elapsed
        // от старта после брифинга, фаза = f(seed, elapsed).
        state.env.elapsed += dt * 1000;
        if (state.weather && now > state.weather.until) scheduleWeather(now);
    }

    // Камера следует за игроком (сглаживание).
    const tx = state.player.x;
    const ty = state.player.y - vh * 0.08;
    state.camera.x += (tx - state.camera.x) * CAMERA_LERP;
    state.camera.y += (ty - state.camera.y) * CAMERA_LERP;

    // Базовый масштаб CSS→device (DPR) + сглаживание кэшированных чанков при
    // апскейле зума (мягкость умеренная, см. отчёт).
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.imageSmoothingEnabled = true;
    ctx.imageSmoothingQuality = 'high';

    drawSky(ctx, vw, vh, state.pkg.sky, state.camera, now, state.env);
    // Среда-фон (звёзды) — вне трансформа ZOOM, как sky (спека §6.3 п.4).
    drawEnvironmentBack(ctx, state.world, state.camera, vw, vh, state.env, state.weather);

    // Мир и игрок — под общим визуальным масштабом (идея 2026-09-22 §8.2):
    // translate → scale → translate вокруг центра экрана. Физика не затронута.
    // Погода и среда — проходами между слоями мира (спека §6.3 п.4), тем же
    // трансформом (§6.5): зум и devicePixelRatio повторно не применяются.
    ctx.save();
    ctx.translate(vw / 2, vh / 2);
    ctx.scale(ZOOM, ZOOM);
    ctx.translate(-vw / 2, -vh / 2);
    drawWeatherBack(ctx, state.world, state.camera, vw, vh, state.weather, state.env);
    drawFarRelief(ctx, state.world, state.camera, vw, vh);
    // Ярусы горизонта (§3.6, §6 п.1) — строго между farRelief и environmentMid.
    drawHorizon(ctx, state.world, state.camera, vw, vh, state.env);
    drawEnvironmentMid(ctx, state.world, state.camera, vw, vh, state.env);
    drawWeatherMid(ctx, state.world, state.camera, vw, vh, state.weather, state.env);
    drawTerrain(ctx, state.world, state.camera, vw, vh);
    drawDecor(ctx, state.world, state.camera, vw, vh);
    // Корабль игрока — парящая декорация у точки спавна, позади игрока (ЧК-ship).
    drawShip(ctx, state.world, state.camera, vw, vh, state.pkg, now);
    drawCreatures(ctx, state.world, state.camera, vw, vh, state.player, now);
    drawPlayer(ctx, state.player, state.camera, vw, vh, now);
    drawWeatherFront(ctx, state.world, state.camera, vw, vh, state.weather, state.env);
    drawEnvironmentFront(ctx, state.world, state.camera, vw, vh, state.env);
    // Финальный эмиссивный проход — после переднего тинта среды, чтобы свет
    // (разряд/вспышка грозы, glow свечения, сияние) не гасился ночью (§6.3 п.4).
    drawEmissive(ctx, state.world, state.camera, vw, vh, state.weather, state.env);
    ctx.restore();

    const hp = serverHp(state.pkg, Date.now());
    const weatherText = state.weather
        ? weatherLabel(state.weather) + (isAdminRole() ? ' · ' + (state.forcedWeather ? 'вручную' : 'авто') : '')
        : '';
    ui.updateHUD({
        hp,
        biomeName: state.pkg.biome_name || state.pkg.biome,
        hazard: state.pkg.hazard,
        distanceMeters: state.player.distance / PPM,
        weather: weatherText,
        env: state.env ? ui.envPhaseLabel(state.env.phase()) : '',
    });

    if (hp <= 0 && !state.dead) onDeath();

    requestAnimationFrame(frame);
}

async function onDeath() {
    if (state.dead) return;
    state.dead = true;
    state.running = false;
    const res = await leave();
    const cause = (res.ok && res.data && res.data.cause) ? res.data.cause : 'среда';
    ui.showDeath(cause, () => { window.location.href = '/map'; });
}

async function callShip() {
    if (state.leaving) return;
    state.leaving = true;
    ui.notify('Вызываю корабль…');
    const res = await leave();
    if (!res.ok) {
        state.leaving = false;
        ui.notify('Не удалось вызвать корабль: ' + res.error);
        return;
    }
    window.location.href = '/map';
}

function bindInput() {
    const map = { ArrowLeft: 'left', ArrowRight: 'right', KeyA: 'left', KeyD: 'right', Space: 'jump', ShiftLeft: 'sprint', ShiftRight: 'sprint' };
    window.addEventListener('keydown', (e) => {
        if (e.code === 'Escape') {
            if (state.dead) return;
            if (ui.isPaused()) { ui.hidePause(); } else { ui.showPause(() => ui.hidePause(), callShip); }
            return;
        }
        const k = map[e.code];
        if (k) { state.input[k] = true; if (e.code === 'Space') e.preventDefault(); }
    });
    window.addEventListener('keyup', (e) => {
        const k = map[e.code];
        if (k) state.input[k] = false;
    });
}

async function boot() {
    const params = new URLSearchParams(window.location.search);
    const planetID = params.get('planet');
    if (!planetID) {
        window.location.href = '/map';
        return;
    }
    const biome = params.get('biome') || '';
    ui.showLoading('Высадка на поверхность…');
    const res = await land(planetID, biome);
    if (!res.ok) {
        if (res.status === 401) { window.location.href = '/login-page'; return; }
        ui.showLoading('Высадка невозможна: ' + res.error);
        setTimeout(() => { window.location.href = '/map'; }, 2500);
        return;
    }
    state.pkg = res.data;
    state.world = new SurfaceWorld(state.pkg);
    state.player = new Player(state.world, state.pkg.gravity);
    state.env = new SurfaceEnvironment(state.pkg);
    // e2e-снимки прогулки (tools/e2e/surface-walk-field-check.js, W7): поле и
    // игрок — для проверки реального входа под свод/в грот. Как `__beltWorld`.
    window.__surface = state;

    const canvas = document.getElementById('surface-canvas');
    resize(canvas);
    window.addEventListener('resize', () => resize(canvas));
    bindInput();
    document.getElementById('ship-call-btn').onclick = callShip;

    // Прелоадер скрывается при показе брифинга (брифинг — отдельный слой):
    // иначе #loading висит поверх и перехватывает клики по «Продолжить».
    ui.hideLoading();
    ui.showBriefing(state.pkg, {
        onContinue: () => {
            ui.hideBriefing();
            ui.hideLoading();
            ui.showHUD();
            if (isAdminRole()) {
                ui.showWeatherToggle(setWeather);
                ui.setWeatherToggleActive('');
                ui.showEnvToggle(setEnv);
                ui.setEnvToggleActive('');
            }
            state.env.elapsed = 0;   // сутки стартуют от первого кадра после брифинга
            scheduleWeather(performance.now());
            state.running = true;
            state.dead = false;
            state.lastTime = performance.now();
            requestAnimationFrame(frame);
        },
        onLeave: callShip,
    });
}

boot();
