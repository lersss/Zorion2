// web/static/js/surface/surface_boat.js
// Лодка-снаряжение прогулки (ЧК6.3, спека 2026-09-26-мир-прогулки-лодка.md):
// клиентский режим движения игрока — одна на игрока, без персиста/БД (§7.1).
// Обходит `Player.update` своим циклом (§2.1 A): ходьба/плавание (ЧК6.2) не
// меняются. Физика — §3.3, вход/выход — §3.2, запреты — §3.4, отрисовка — §3.5.
import {
    PPM, WALK_SPEED, PLAYER_H,
    BOAT_W, BOAT_HULL, BOAT_DRAFT, BOAT_FACTOR, BOAT_ACCEL, BOAT_DRAG,
    BOAT_MIN_DEPTH, BOARD_REACH, BOAT_BOB_AMP, BOAT_ROCK_AMP, BOAT_BOB_PERIOD_MS,
} from './surface_config.js';

function approach(cur, target, maxDelta) {
    if (cur < target) return Math.min(cur + maxDelta, target);
    if (cur > target) return Math.max(cur - maxDelta, target);
    return cur;
}

// Ширина борта-«трубы» (арт-док `art_surface_boat.md` §3.2): центр корпуса —
// тёмное ложе (±9 логических px), борта — крайние 8 px каждой стороны. При
// посадке/ходе мокрыми обязаны быть борта; центральная колонка (ложе под игроком)
// может быть сухой — посадка с пологого берега (§3.2 cond. 3). Файловая
// константа, не экспорт: аддитивна, второй путь геометрии не заводит.
const BOAT_TUBE_W = 8;

// Палитра код-фолбэка (арт-док §3.3/§6): совпадает со спрайтом @artist, иначе при
// подгрузке PNG лодка «прыгнет».
const BOAT_COLORS = { body: '#c86432', gunwale: '#f0d8c0', floor: '#5a3320', line: '#3f2415' };

// footOffsets — смещения проб footprint по X: шаг 4 px по `[−BOAT_W/2, +BOAT_W/2]`
// с обязательным включением обоих краёв (`BOAT_W=34` не кратен 4 — край `+BOAT_W/2`
// иначе выпал бы из сетки, и лодка «заходила» бы бортом на сушу).
function footOffsets() {
    const half = BOAT_W / 2;
    const offs = [];
    for (let o = -half; o <= half; o += 4) offs.push(o);
    if (offs[offs.length - 1] !== half) offs.push(half);
    return offs;
}

// Ленивая загрузка спрайта (арт @artist, §3.5): нет/не загрузился/вне DOM →
// null, рисуется код-силуэт (фолбэк; кадр не ломается). Node-безопасно.
let _boatSprite = null;
let _boatSpriteTried = false;
function boatSprite() {
    if (_boatSpriteTried) return _boatSprite;
    _boatSpriteTried = true;
    if (typeof Image === 'undefined') return null;
    try {
        const img = new Image();
        img.onload = () => { _boatSprite = img.naturalWidth > 0 ? img : null; };
        img.onerror = () => { _boatSprite = null; };
        img.src = '/static/sprites/surface/boat/boat.png';
    } catch (e) { _boatSprite = null; }
    return null;
}

export class Boat {
    constructor(world) {
        this.world = world;
        this.aboard = false;
        this.x = 0;
        this.y = 0;
        this.vx = 0;
    }

    // _liquidOk — среда разрешает лодку (§3.4): есть жидкость; не лава; не underIce
    // (в т.ч. полыньи — решение создателя 2026-09-26, запрет по режиму).
    _liquidOk(world) {
        const L = world && world.liquid;
        if (!L) return false;
        if (L.medium === 'лава') return false;
        if (L.level && L.level.mode === 'underIce') return false;
        return true;
    }

    // _bodyFree — полный силуэт «лодка + игрок» свободен по `solidAt'` (§3.3/§7.5):
    // X `[x − BOAT_W/2, x + BOAT_W/2]`, Y от макушки (`lv − BOAT_HULL − h/2` = lv−34)
    // до низа осадки (`lv + BOAT_DRAFT`). `solidAt` включает корку underIce — лодка
    // не входит ни корпусом, ни игроком в свод/скалу/void-стенку.
    _bodyFree(world, x, lv) {
        const yTop = lv - BOAT_HULL - PLAYER_H / 2;
        const yBot = lv + BOAT_DRAFT;
        const step = 4;
        for (const o of footOffsets()) {
            const px = x + o;
            for (let py = yTop; py <= yBot + 1e-9; py += step) {
                if (world.solidAt(px, py)) return false;
            }
        }
        return true;
    }

    // _occupyAt — корпус помещается (§3.2 cond. 3, §3.3): footprint (`[x±BOAT_W/2]`,
    // шаг 4 px) — борта-трубы на мокрых колонках `depth ≥ BOAT_MIN_DEPTH`, не в
    // корке; центральное ложе может быть над сухой/мелкой колонкой (посадка с
    // пологого берега); плюс полный силуэт свободен по `solidAt'`.
    _occupyAt(world, x) {
        if (!world || !world.liquid) return false;
        const lv = world.liquidLevel(x);
        if (!isFinite(lv)) return false;
        if (!this._bodyFree(world, x, lv)) return false;
        const half = BOAT_W / 2;
        const tubeIn = half - BOAT_TUBE_W;    // 9: |offset| ≥ 9 — борт-труба
        for (const o of footOffsets()) {
            if (Math.abs(o) < tubeIn) continue;   // центральное ложе — не борт
            const px = x + o;
            const col = Math.floor(px);
            const c = world._liquidCol(col);
            if (!c || c.depth < BOAT_MIN_DEPTH) return false;
            if (world.crustTop(col) !== null) return false;
        }
        return true;
    }

    // _canOccupy — обёртка над _occupyAt для текущего мира (сигнатура §6).
    _canOccupy(nx) {
        return this._occupyAt(this.world, nx);
    }

    // canBoard — поиск ближайшего dx (§3.2 cond. 2): `|dx| ≤ BOARD_REACH`, зеркало
    // рядом (`liquidLevel(player.x + dx)` конечен и `≤ feet + BOARD_REACH`), корпус
    // помещается (`_canOccupy`). Возвращает dx или null; детерминизм — без random.
    canBoard(world, player) {
        const w = world || this.world;
        if (!this._liquidOk(w)) return null;
        const feet = player.y + player.h / 2;
        for (let d = 0; d <= BOARD_REACH; d++) {
            const cands = d === 0 ? [0] : [d, -d];
            for (const dx of cands) {
                const bx = player.x + dx;
                const lv = w.liquidLevel(bx);
                if (!isFinite(lv) || lv > feet + BOARD_REACH) continue;
                if (this._occupyAt(w, bx)) return dx;
            }
        }
        return null;
    }

    // board — посадка (§3.2): центр бокса игрока совмещается с `boat.x = player.x + dx`.
    // Возвращает false, если посадка невозможна.
    board(player) {
        const dx = this.canBoard(this.world, player);
        if (dx === null) return false;
        this.x = player.x + dx;
        this.vx = 0;
        this.aboard = true;
        this.y = this.world.liquidLevel(this.x) - BOAT_HULL;
        player.x = this.x;
        player.y = this.y;
        player.vx = 0;
        player.vy = 0;
        return true;
    }

    // disembark — выход (§3.2): player получает позицию и `vx` лодки, `vy = 0`;
    // лодка «складывается» (персиста нет). Выход разрешён в любом месте воды.
    disembark(player) {
        this.aboard = false;
        player.x = this.x;
        player.y = this.world.liquidLevel(this.x) - BOAT_HULL;
        player.vx = this.vx;
        player.vy = 0;
        if (this.vx !== 0) player.facing = this.vx > 0 ? 1 : -1;
        this.vx = 0;
    }

    // update — физика лодки (§3.3). Вертикаль не применяется (`vy = 0`, `down`/`jump`
    // — no-op). Скорость ×BOAT_FACTOR (спринт отключён); инерция `approach`;
    // мягкий стоп по footprint; `player.distance` накапливается (HUD пути).
    update(dt, input, player) {
        const w = this.world;
        const speed = WALK_SPEED * PPM * BOAT_FACTOR;
        let target = 0;
        if (input.left) target -= speed;
        if (input.right) target += speed;
        const a = target !== 0 ? BOAT_ACCEL : BOAT_DRAG;
        this.vx = approach(this.vx, target, a * dt);
        const nx = this.x + this.vx * dt;
        if (this._canOccupy(nx)) {
            player.distance += Math.abs(nx - this.x);   // HUD «путь» не замирает (§3.3)
            this.x = nx;
        } else {
            this.vx = 0;                                // мягкий стоп у берега/стены
        }
        this.y = w.liquidLevel(this.x) - BOAT_HULL;     // y = liquidLevel(x) − BOAT_HULL
        player.x = this.x;
        player.y = this.y;
        player.vy = 0;
        if (this.vx !== 0) player.facing = this.vx > 0 ? 1 : -1;
        return this;
    }
}

// drawBoatFallback — код-силуэт до спрайта @artist (§3.5, арт-док §6): две
// надувные «трубы»-борта, двухуровневый верх (пики нос/корма, провал аммидшипс),
// тёмное ложе, блок мотора у транца. Геометрия/палитра совпадают со спрайтом.
// Локальные координаты: y=0 — линия воды; +bob применяется снаружи, внутри pivot.
function drawBoatFallback(ctx, bob) {
    const hw = BOAT_W / 2;      // 17
    const top = -14;            // планшир-пик (lv − 14)
    const dip = -9;             // провал аммидшипс (lv − 9)
    const keel = BOAT_DRAFT;    // низ корпуса (lv + BOAT_DRAFT = +6)
    const transom = -13;        // транец (логика x ≈ 4)
    ctx.save();
    ctx.translate(0, bob);
    // Тёмное ложе (внутренняя выемка между труб).
    ctx.beginPath();
    ctx.moveTo(transom + 1, top + 2);
    ctx.lineTo(0, dip);
    ctx.lineTo(hw - 1, top + 2);
    ctx.lineTo(9, keel - 1);
    ctx.lineTo(-9, keel - 1);
    ctx.closePath();
    ctx.fillStyle = BOAT_COLORS.floor;
    ctx.fill();
    // Корпус — две трубы, двухуровневый верх (пики нос/корма, провал аммидшипс).
    ctx.beginPath();
    ctx.moveTo(transom, top + 1);
    ctx.lineTo(0, dip + 1);
    ctx.lineTo(hw, top);
    ctx.lineTo(hw - 2, keel - 4);
    ctx.quadraticCurveTo(hw - 10, keel, 6, keel);
    ctx.quadraticCurveTo(-2, keel, -6, keel - 1);
    ctx.lineTo(transom, 2);
    ctx.closePath();
    ctx.fillStyle = BOAT_COLORS.body;
    ctx.fill();
    ctx.lineWidth = 1.6;
    ctx.strokeStyle = BOAT_COLORS.line;
    ctx.stroke();
    // Планшир (светлый кант) по верхнему контуру.
    ctx.beginPath();
    ctx.moveTo(transom + 1, top + 2);
    ctx.lineTo(0, dip + 2);
    ctx.lineTo(hw - 2, top + 2);
    ctx.lineWidth = 2;
    ctx.strokeStyle = BOAT_COLORS.gunwale;
    ctx.stroke();
    // Блок мотора (3–4 px) у транца слева, над водой.
    ctx.fillStyle = BOAT_COLORS.line;
    ctx.fillRect(-hw, dip + 1, 3, 10);
    ctx.restore();
}

// drawBoat — отрисовка (§3.5). Вызывается между `drawCreatures` и `drawPlayer`
// (игрок рисуется поверх корпуса). Только при `aboard`; сложенная лодка невидима.
// Pivot покачивания/крена — точка на линии воды (`x = boat.x`, `y = liquidLevel`).
export function drawBoat(ctx, world, camera, vw, vh, player, boat, timeMs) {
    if (!boat || !boat.aboard) return;
    const lv = world.liquidLevel(boat.x);
    if (!isFinite(lv)) return;
    const bx = boat.x - camera.x + vw / 2;
    const by = lv - camera.y + vh / 2;
    const phase = ((timeMs || 0) / BOAT_BOB_PERIOD_MS) * Math.PI * 2;
    const bob = Math.sin(phase) * BOAT_BOB_AMP;
    const rock = Math.sin(phase) * BOAT_ROCK_AMP;
    ctx.save();
    ctx.translate(bx, by);   // pivot вращения — линия воды (§3.5)
    ctx.rotate(rock);
    const sprite = boatSprite();
    if (sprite) {
        // Спрайт 204×120 = 6× логики 34×20; линия воды в файле y = 84 (= логика 14).
        // Верх бокса — `lv − 14` (планшир), низ — `lv + BOAT_DRAFT`.
        ctx.drawImage(sprite, -BOAT_W / 2, -14 + bob, BOAT_W, 20);
    } else {
        drawBoatFallback(ctx, bob);
    }
    ctx.restore();
}
