// web/static/js/surface/surface_main.js
// Точка входа страницы прогулки (спека 2026-09-21): высадка (land) → брифинг →
// Canvas-игра (ходьба/прыжки/падение, HUD) → «вызвать корабль»/смерть (leave).
// Вход: /surface.html?planet=<uuid> (правый клик по планете → «Высадиться»).
import { CAMERA_LERP, WEATHER_MIN_MS, WEATHER_MAX_MS, PPM, WEATHER, WEATHER_BY_CATEGORY } from './surface_config.js';
import { SurfaceWorld, mulberry32 } from './surface_world.js';
import { drawSky, drawFarRelief, drawTerrain, drawDecor, drawCreatures, drawPlayer, drawWeather } from './surface_render.js';
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
    weatherUntil: 0,
    running: false,
    dead: false,
    leaving: false,
    lastTime: 0,
};

// rng — локальный детерминированный PRNG (mulberry32, спека §12 п.10):
// никакого общего Math.random. Сид — от seed мира + константа погоды.
let rng = null;

function pickWeather() {
    const list = WEATHER_BY_CATEGORY[state.pkg.biome_category] || ['штиль'];
    const id = list[Math.floor(rng() * list.length)];
    const def = WEATHER.find((w) => w.id === id) || { id, particles: 'none' };
    return { id: def.id, particles: def.particles };
}

function scheduleWeather(now) {
    state.weather = pickWeather();
    state.weatherUntil = now + WEATHER_MIN_MS + rng() * (WEATHER_MAX_MS - WEATHER_MIN_MS);
}

function resize(canvas) {
    canvas.width = window.innerWidth;
    canvas.height = window.innerHeight;
}

function frame(now) {
    if (!state.running) return;
    const canvas = document.getElementById('surface-canvas');
    const ctx = canvas.getContext('2d');
    const dt = Math.min(0.05, (now - state.lastTime) / 1000 || 0);
    state.lastTime = now;

    const paused = ui.isPaused();
    if (!paused && !state.dead) {
        state.player.update(dt, state.input);
        if (now > state.weatherUntil) scheduleWeather(now);
    }

    // Камера следует за игроком (сглаживание).
    const tx = state.player.x;
    const ty = state.player.y - canvas.height * 0.08;
    state.camera.x += (tx - state.camera.x) * CAMERA_LERP;
    state.camera.y += (ty - state.camera.y) * CAMERA_LERP;

    drawSky(ctx, canvas.width, canvas.height, state.pkg.sky, state.camera, now);
    drawFarRelief(ctx, state.world, state.camera, canvas.width, canvas.height);
    drawTerrain(ctx, state.world, state.camera, canvas.width, canvas.height);
    drawDecor(ctx, state.world, state.camera, canvas.width, canvas.height);
    drawCreatures(ctx, state.world, state.camera, canvas.width, canvas.height, state.player, now);
    drawPlayer(ctx, state.player, state.camera, canvas.width, canvas.height, now);
    drawWeather(ctx, state.weather, canvas.width, canvas.height, now);

    const hp = serverHp(state.pkg, Date.now());
    ui.updateHUD({
        hp,
        biomeName: state.pkg.biome_name || state.pkg.biome,
        hazard: state.pkg.hazard,
        distanceMeters: state.player.distance / PPM,
        weather: state.weather ? state.weather.id : '',
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
    ui.showLoading('Высадка на поверхность…');
    const res = await land(planetID);
    if (!res.ok) {
        if (res.status === 401) { window.location.href = '/login-page'; return; }
        ui.showLoading('Высадка невозможна: ' + res.error);
        setTimeout(() => { window.location.href = '/map'; }, 2500);
        return;
    }
    state.pkg = res.data;
    state.world = new SurfaceWorld(state.pkg);
    state.player = new Player(state.world, state.pkg.gravity);
    rng = mulberry32((state.pkg.seed ^ 0x5eed) >>> 0);

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
