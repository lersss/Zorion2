// web/static/js/belt/belt_world.js
// Детерминированный мир мини-игры добычи в поясе (спека
// 2026-09-22-пояса-малых-тел-этап-3-добыча §5.1): от seed сервера
// (crc32(belt_id+"|belt")) — звёздное поле, далёкая пыль и астероиды
// (дрейф + вращение). Клиентская физика полёта (инерция, столкновения без
// урона). Сервер знает только агрегат запаса пояса (§5.2, Б8) — «прогресс
// жилы» здесь чисто визуальный. Math.random не используется (детерминизм).
import * as C from './belt_config.js';

// mulberry32 — детерминированный PRNG (как в surface_world.js).
export function mulberry32(seed) {
    let a = seed >>> 0;
    return function () {
        a |= 0;
        a = (a + 0x6D2B79F5) | 0;
        let t = Math.imul(a ^ (a >>> 15), 1 | a);
        t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
        return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
    };
}

function lerp(a, b, t) { return a + (b - a) * t; }

// lerpAngle — интерполяция угла с учётом перехода через ±π.
function lerpAngle(from, to, t) {
    let d = to - from;
    while (d > Math.PI) d -= 2 * Math.PI;
    while (d < -Math.PI) d += 2 * Math.PI;
    return from + d * t;
}

// BeltWorld — сцена захода: астероиды, корабль, эффекты.
export class BeltWorld {
    constructor(seed) {
        this.seed = seed >>> 0;
        this.rng = mulberry32(this.seed);
        this.fx = mulberry32((this.seed ^ 0x9e3779b9) >>> 0);
        this.asteroids = this._genAsteroids();
        this.stars = this._genStars();
        this.dust = this._genDust();
        this.ship = { x: 0, y: 0, vx: 0, vy: 0, heading: 0, thrusting: false, braking: false };
        this.particles = [];
        this.floaters = [];
        this.collided = false; // столкновение в этом кадре — прерывает бурение
    }

    // ---- Генерация мира (детерминированная) ----

    _genAsteroids() {
        const rng = this.rng;
        const total = C.ASTEROID_MIN + Math.floor(rng() * (C.ASTEROID_MAX - C.ASTEROID_MIN + 1));
        const veins = C.VEIN_MIN + Math.floor(rng() * (C.VEIN_MAX - C.VEIN_MIN + 1));
        const list = [];
        for (let i = 0; i < total; i++) {
            const vein = i < veins;
            const r = vein
                ? lerp(C.VEIN_RADIUS_MIN, C.VEIN_RADIUS_MAX, rng())
                : lerp(C.DECOR_RADIUS_MIN, C.DECOR_RADIUS_MAX, rng());
            const ang = rng() * Math.PI * 2;
            const dist = Math.sqrt(rng()) * Math.max(0, C.FIELD_RADIUS - r - 60);
            const pts = 8 + Math.floor(rng() * 4);
            const shape = [];
            for (let k = 0; k < pts; k++) shape.push(0.72 + rng() * 0.28);
            const glints = [];
            if (vein) {
                const g = 2 + Math.floor(rng() * 3);
                for (let k = 0; k < g; k++) {
                    glints.push({ x: (rng() - 0.5) * r * 0.9, y: (rng() - 0.5) * r * 0.9, r: 1.5 + rng() * 2.5 });
                }
            }
            list.push({
                x: Math.cos(ang) * dist,
                y: Math.sin(ang) * dist,
                vx: (rng() - 0.5) * 2 * C.ASTEROID_DRIFT_MAX,
                vy: (rng() - 0.5) * 2 * C.ASTEROID_DRIFT_MAX,
                r,
                rot: rng() * Math.PI * 2,
                rotSpeed: (rng() - 0.5) * 2 * C.ASTEROID_SPIN_MAX,
                vein,
                shape,
                glints,
                drill: 0,
            });
        }
        return list;
    }

    _genStars() {
        const rng = this.rng;
        const list = [];
        for (let i = 0; i < C.STAR_COUNT; i++) {
            list.push({
                x: rng() * C.STAR_TILE,
                y: rng() * C.STAR_TILE,
                s: 1 + rng() * 1.6,
                a: 0.25 + rng() * 0.6,
                tw: 0.5 + rng() * 2.5,
                ph: rng() * Math.PI * 2,
                c: rng() < 0.2 ? '#ffe9c4' : '#dfe8ff',
            });
        }
        return list;
    }

    _genDust() {
        const rng = this.rng;
        const list = [];
        for (let i = 0; i < C.DUST_COUNT; i++) {
            const ang = rng() * Math.PI * 2;
            const dist = Math.sqrt(rng()) * C.FIELD_RADIUS;
            list.push({
                x: Math.cos(ang) * dist,
                y: Math.sin(ang) * dist,
                r: 120 + rng() * 320,
                a: 0.03 + rng() * 0.05,
            });
        }
        return list;
    }

    // ---- Физика корабля и мира ----

    update(dt, input) {
        const s = this.ship;
        let ax = 0;
        let ay = 0;
        if (input.left) ax -= 1;
        if (input.right) ax += 1;
        if (input.up) ay -= 1;
        if (input.down) ay += 1;
        const len = Math.hypot(ax, ay);
        s.thrusting = len > 0;
        s.braking = !!input.brake;
        if (len > 0) {
            ax /= len;
            ay /= len;
            s.vx += ax * C.SHIP_THRUST * dt;
            s.vy += ay * C.SHIP_THRUST * dt;
            // Наклон/вращение к вектору тяги (космическое ощущение).
            s.heading = lerpAngle(s.heading, Math.atan2(ay, ax), C.SHIP_TURN_LERP);
        }
        // Слабое торможение / Shift-тормоз.
        const damp = s.braking ? C.SHIP_BRAKE : C.SHIP_DRAG;
        const k = Math.max(0, 1 - damp * dt);
        s.vx *= k;
        s.vy *= k;
        const sp = Math.hypot(s.vx, s.vy);
        if (sp > C.SHIP_MAX_SPEED) {
            s.vx = (s.vx / sp) * C.SHIP_MAX_SPEED;
            s.vy = (s.vy / sp) * C.SHIP_MAX_SPEED;
        }
        s.x += s.vx * dt;
        s.y += s.vy * dt;

        // Мягкая граница пятна: упругое торможение у края.
        const d = Math.hypot(s.x, s.y);
        const limit = C.FIELD_RADIUS - C.SHIP_RADIUS;
        if (d > limit) {
            const nx = s.x / d;
            const ny = s.y / d;
            s.x = nx * limit;
            s.y = ny * limit;
            const vn = s.vx * nx + s.vy * ny;
            if (vn > 0) {
                s.vx -= vn * nx * 1.4;
                s.vy -= vn * ny * 1.4;
            }
        }

        this._updateAsteroids(dt);
        this._collisions();
        this._updateEffects(dt);
    }

    _updateAsteroids(dt) {
        for (const a of this.asteroids) {
            a.x += a.vx * dt;
            a.y += a.vy * dt;
            a.rot += a.rotSpeed * dt;
            const d = Math.hypot(a.x, a.y);
            const lim = C.FIELD_RADIUS - a.r;
            if (d > lim) {
                const nx = a.x / d;
                const ny = a.y / d;
                a.x = nx * lim;
                a.y = ny * lim;
                const vn = a.vx * nx + a.vy * ny;
                a.vx -= 2 * vn * nx;
                a.vy -= 2 * vn * ny;
            }
        }
    }

    // Столкновения — мягкое отталкивание без урона (§5.2, решение §10.2-A).
    // Бурение прерывает только РЕАЛЬНЫЙ удар (заметная скорость сближения):
    // «отдых в контакте» (игрок стоит у камня и бурит) не считается ударом,
    // иначе повторный контакт каждый кадр блокировал бы добычу.
    _collisions() {
        this.collided = false;
        const s = this.ship;
        for (const a of this.asteroids) {
            const dx = s.x - a.x;
            const dy = s.y - a.y;
            const d = Math.hypot(dx, dy);
            const min = a.r + C.SHIP_RADIUS;
            if (d < min && d > 0.0001) {
                const nx = dx / d;
                const ny = dy / d;
                // Выталкиваем с малым зазором (не оставляем на грани контакта).
                s.x = a.x + nx * (min + 0.5);
                s.y = a.y + ny * (min + 0.5);
                const vn = s.vx * nx + s.vy * ny;
                if (vn < -5) this.collided = true;
                if (vn < 0) {
                    // Отражение нормальной составляющей с потерей энергии.
                    s.vx -= 1.4 * vn * nx;
                    s.vy -= 1.4 * vn * ny;
                }
            }
        }
    }

    _updateEffects(dt) {
        const parts = [];
        for (const p of this.particles) {
            p.x += p.vx * dt;
            p.y += p.vy * dt;
            p.life -= dt;
            if (p.life > 0) parts.push(p);
        }
        this.particles = parts;
        const fl = [];
        for (const f of this.floaters) {
            f.y -= 18 * dt;
            f.life -= dt;
            if (f.life > 0) fl.push(f);
        }
        this.floaters = fl;
    }

    // ---- Действия ----

    // nearestVein — ближайшая крупная «жила» в радиусе захвата (R_extract от
    // поверхности тела, §5.2). null — цели нет.
    nearestVein() {
        const s = this.ship;
        let best = null;
        let bestD = Infinity;
        for (const a of this.asteroids) {
            if (!a.vein) continue;
            const d = Math.hypot(s.x - a.x, s.y - a.y) - a.r;
            if (d <= C.EXTRACT_RADIUS && d < bestD) {
                bestD = d;
                best = a;
            }
        }
        return best;
    }

    // drill — удержание бурения у целевого астероида: чисто визуальный прогресс
    // жилы + частицы руды к кораблю (§5.2, Б8).
    drill(target, dt) {
        if (!target) return;
        target.drill = Math.min(1, target.drill + dt * 0.22);
        const base = Math.atan2(this.ship.y - target.y, this.ship.x - target.x);
        for (let i = 0; i < 2; i++) {
            const ang = base + (this.fx() - 0.5) * 0.9;
            const sp = 60 + this.fx() * 90;
            this.particles.push({
                x: target.x + Math.cos(ang) * target.r * 0.85,
                y: target.y + Math.sin(ang) * target.r * 0.85,
                vx: Math.cos(ang) * sp,
                vy: Math.sin(ang) * sp,
                life: 0.35 + this.fx() * 0.3,
                max: 0.65,
                r: 1.5 + this.fx() * 2,
            });
        }
    }

    // addFloater — всплывающее «+X т» (сцена, §9.1 п.5).
    addFloater(text, x, y) {
        this.floaters.push({ text, x, y, life: 1.1, max: 1.1 });
    }
}
