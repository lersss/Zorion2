// web/static/js/belt/belt_world.js
// Детерминированный мир мини-игры добычи в поясе (спека
// 2026-09-22-пояса-малых-тел-этап-3-добыча §5.1/§8.2, ревизия 4): бесконечный
// вдоль кольца пояс (ось X), поперёк — полоса ±BELT_HALF_WIDTH с линейным
// спадом плотности. Тела порождаются ячейками вокруг корабля (стриминг, кап),
// детерминированно от seed сервера (crc32(belt_id+"|belt")); Math.random не
// используется. Клиентская физика полёта «два стика» (тяга с разгоном, стрейф,
// доворот носа к курсору с ограниченной скоростью, столкновения без урона).
// Сервер знает только агрегат запаса пояса (§5.2, Б8) — «прогресс жилы» здесь
// чисто визуальный.
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

// mix32 — целочисленное перемешивание (splitmix32-класс) для ключа ячейки
// (§8.2): rng_ячейки = mulberry32(seed ^ mix32(cellIndex)). Math.random запрещён.
function mix32(x) {
    let h = x >>> 0;
    h = Math.imul(h ^ (h >>> 16), 0x21f0aaad);
    h = Math.imul(h ^ (h >>> 15), 0x735a2d97);
    return (h ^ (h >>> 15)) >>> 0;
}

// cellSeed — seed ячейки: целочисленный ключ из индексов (вдоль кольца cx,
// поперёк cy) сворачивается в один и перемешивается с seed мира.
function cellSeed(seed, cx, cy) {
    const key = (Math.imul(cx | 0, 0x1f123bb5) ^ Math.imul(cy | 0, 0x27d4eb2f)) >>> 0;
    return (seed ^ mix32(key)) >>> 0;
}

function lerp(a, b, t) { return a + (b - a) * t; }

const num = (v) => (typeof v === 'number' && isFinite(v) ? v : 0);

function normAngle(a) {
    while (a > Math.PI) a -= 2 * Math.PI;
    while (a < -Math.PI) a += 2 * Math.PI;
    return a;
}

// rollVeins — число жил в ячейке (1–2). Ролл отдельный от прочих: _genDebris
// берёт его тем же seed'ом ячейки, чтобы сумма тел держалась спекой (4–6 тел,
// из них жилы 1–2, §8.2).
function rollVeins(rng) {
    return C.BELT_CELL_VEINS_MIN
        + Math.floor(rng() * (C.BELT_CELL_VEINS_MAX - C.BELT_CELL_VEINS_MIN + 1));
}

// BeltWorld — сцена захода: пояс (стриминг тел), корабль, эффекты.
export class BeltWorld {
    constructor(seed, opts) {
        opts = opts || {};
        this.seed = seed >>> 0;
        // Состав пояса для типа жил (F2, спека 2026-09-24 §7.3): лёд «жив»
        // только если ресурс доступен и composition.ice > 0. Иначе тип жилы
        // не роллится ВООБЩЕ (RNG не тратится) — пояс без льда выглядит ровно
        // как до фичи (§8.1, ловушка RNG M5).
        this.iron = num(opts.iron);
        this.ice = num(opts.ice);
        this.iceAvailable = !!opts.iceAvailable && this.ice > 0;
        this.pIce = this.iceAvailable ? this.ice / (this.ice + this.iron) : 0;
        // Гашение жил выработанного ресурса (§8.1): по ним цель не берётся.
        this.depletedRes = { iron: false, ice: false };
        this.rng = mulberry32(this.seed);
        this.fx = mulberry32((this.seed ^ 0x9e3779b9) >>> 0);
        this.stars = this._genStars();
        this.ship = {
            x: 0, y: 0, vx: 0, vy: 0, heading: 0, angVel: 0, throttle: 0,
            thrusting: false, braking: false,
        };
        // Слои пояса (§8.2/§9.2): жилы (1.0 — игровое поле), мелкие обломки
        // (0.6 — декор, только визуальный), далёкая пыль (0.4). Каждый слой
        // стримится ячейками вокруг корабля в своих координатах (параллакс).
        this._layers = {
            a: { kind: 'a', p: 1, cap: C.BELT_ASTEROID_CAP, cells: new Map(), bodies: [] },
            d: { kind: 'd', p: C.PARALLAX_DECOR, cap: Infinity, cells: new Map(), bodies: [] },
            u: { kind: 'u', p: C.PARALLAX_DUST, cap: C.BELT_DUST_CAP, cells: new Map(), bodies: [] },
        };
        this.asteroids = this._layers.a.bodies; // жилы: бурение и столкновения
        this.debris = this._layers.d.bodies; // мелкий декор: визуальный
        this.dust = this._layers.u.bodies; // пятна пыли: визуальный
        this._pool = []; // пул тел: отпущенные возвращаются и переиспользуются
        this.particles = [];
        this.floaters = [];
        this.collided = false; // столкновение в этом кадре — прерывает бурение
        this._stream(); // наполнить пояс вокруг корабля
    }

    // ---- Генерация (детерминированная) ----

    _spawn() {
        return this._pool.pop() || {};
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

    // _genVeins — ячейка жил (1–2 кандидата): крупные тела с блеском руды.
    // Плотность поперёк — линейный спад (§8.2).
    _genVeins(cx, cy) {
        const rng = mulberry32(cellSeed(this.seed, cx, cy));
        const out = [];
        const veins = rollVeins(rng);
        for (let i = 0; i < veins; i++) {
            const y = cy * C.BELT_CELL + rng() * C.BELT_CELL;
            if (rng() > 1 - Math.abs(y) / C.BELT_HALF_WIDTH) continue;
            const x = cx * C.BELT_CELL + rng() * C.BELT_CELL;
            const r = lerp(C.VEIN_RADIUS_MIN, C.VEIN_RADIUS_MAX, rng());
            const pts = 8 + Math.floor(rng() * 4);
            const b = this._spawn();
            const shape = b.shape || (b.shape = []);
            shape.length = pts;
            for (let k = 0; k < pts; k++) shape[k] = 0.72 + rng() * 0.28;
            const glints = b.glints || (b.glints = []);
            const g = 2 + Math.floor(rng() * 3);
            glints.length = g;
            for (let k = 0; k < g; k++) {
                glints[k] = { x: (rng() - 0.5) * r * 0.9, y: (rng() - 0.5) * r * 0.9, r: 1.5 + rng() * 2.5 };
            }
            b.x = x;
            b.y = y;
            b.vx = (rng() - 0.5) * 2 * C.ASTEROID_DRIFT_MAX;
            b.vy = (rng() - 0.5) * 2 * C.ASTEROID_DRIFT_MAX;
            b.r = r;
            b.rot = rng() * Math.PI * 2;
            b.rotSpeed = (rng() - 0.5) * 2 * C.ASTEROID_SPIN_MAX;
            b.vein = true;
            b.drill = 0;
            // Тип жилы (F2): ролл ТОЛЬКО при доступном льде — иначе rng() не
            // вызывается вовсе (инвариант: пояс без льда = до-фичевый мир, §8.1).
            b.res = (this.iceAvailable && rng() < this.pIce) ? 'ice' : 'iron';
            // Спрайт/руда (арт-ТЗ §4.2): индексы и параметры — из того же rng
            // ячейки (детерминизм; Math.random запрещён). shape[]/glints[] выше
            // остаются фолбэком, если файл спрайта не загрузился.
            b.sprite = Math.floor(rng() * C.ROCK_SPRITES.length);
            b.veinPattern = Math.floor(rng() * C.VEIN_SPRITES.length);
            b.veinRot = rng() * Math.PI * 2;
            b.veinScale = 0.7 + rng() * 0.5;
            b.veinRich = 0.35 + rng() * 0.65;
            out.push(b);
        }
        return out;
    }

    // _genDebris — ячейка мелких обломков: декор слоя 0.6. Обломков столько,
    // сколько осталось от суммарного числа тел ячейки (спека 4–6) после жил;
    // полоса и спад считаются в МИРОВЫХ координатах (y / parallax).
    _genDebris(cx, cy) {
        const rng = mulberry32(cellSeed(this.seed ^ 0x51ed, cx, cy));
        const out = [];
        const veins = rollVeins(mulberry32(cellSeed(this.seed, cx, cy)));
        const totalBodies = C.BELT_CELL_BODIES_MIN
            + Math.floor(rng() * (C.BELT_CELL_BODIES_MAX - C.BELT_CELL_BODIES_MIN + 1));
        const total = totalBodies - veins;
        for (let i = 0; i < total; i++) {
            const y = cy * C.BELT_CELL + rng() * C.BELT_CELL;
            if (rng() > 1 - Math.abs(y / C.PARALLAX_DECOR) / C.BELT_HALF_WIDTH) continue;
            const x = cx * C.BELT_CELL + rng() * C.BELT_CELL;
            const r = lerp(C.DECOR_RADIUS_MIN, C.DECOR_RADIUS_MAX, rng());
            const pts = 8 + Math.floor(rng() * 4);
            const b = this._spawn();
            const shape = b.shape || (b.shape = []);
            shape.length = pts;
            for (let k = 0; k < pts; k++) shape[k] = 0.72 + rng() * 0.28;
            if (b.glints) b.glints.length = 0;
            b.x = x;
            b.y = y;
            b.vx = (rng() - 0.5) * 2 * C.ASTEROID_DRIFT_MAX;
            b.vy = (rng() - 0.5) * 2 * C.ASTEROID_DRIFT_MAX;
            b.r = r;
            b.rot = rng() * Math.PI * 2;
            b.rotSpeed = (rng() - 0.5) * 2 * C.ASTEROID_SPIN_MAX;
            b.vein = false;
            b.drill = 0;
            // Тип обломка (лёд/камень) — ролл только при доступном льде, иначе
            // rng() не тратится (инвариант «пояс без льда = до-фичевый мир»).
            b.res = (this.iceAvailable && rng() < this.pIce) ? 'ice' : 'iron';
            // Мелкие обломки — спрайт из DEBRIS_SPRITES/ICE_DEBRIS_SPRITES (1:1);
            // руды у них нет.
            b.sprite = Math.floor(rng() * C.DEBRIS_SPRITES.length);
            b.veinPattern = 0;
            b.veinRot = 0;
            b.veinScale = 1;
            b.veinRich = 0;
            out.push(b);
        }
        return out;
    }

    // _genDustPatch — ячейка мягкой пыли (0–1 пятно): слой 0.4, гаснет поперёк
    // по мировой координате (y / parallax).
    _genDustPatch(cx, cy) {
        const rng = mulberry32(cellSeed(this.seed ^ 0xd057, cx, cy));
        if (rng() > C.BELT_CELL_DUST_CHANCE) return [];
        const y = cy * C.BELT_CELL + rng() * C.BELT_CELL;
        if (rng() > 1 - Math.abs(y / C.PARALLAX_DUST) / C.BELT_HALF_WIDTH) return [];
        const b = this._spawn();
        b.x = cx * C.BELT_CELL + rng() * C.BELT_CELL;
        b.y = y;
        b.r = 120 + rng() * 320;
        b.a = 0.03 + rng() * 0.05;
        return [b];
    }

    // ---- Стриминг тел (§8.2) ----

    // _stream — активные ячейки вокруг корабля (в координатах слоя), дальние
    // отпускаются в пул; тела не накапливаются (кап).
    _stream() {
        this._streamLayer(this._layers.a, (cx, cy) => this._genVeins(cx, cy));
        this._streamLayer(this._layers.d, (cx, cy) => this._genDebris(cx, cy));
        this._streamLayer(this._layers.u, (cx, cy) => this._genDustPatch(cx, cy));
    }

    _streamLayer(layer, gen) {
        const c0x = this.ship.x * layer.p;
        const c0y = this.ship.y * layer.p;
        const minX = Math.floor((c0x - C.BELT_STREAM_RADIUS) / C.BELT_CELL);
        const maxX = Math.floor((c0x + C.BELT_STREAM_RADIUS) / C.BELT_CELL);
        const minY = Math.floor((c0y - C.BELT_STREAM_RADIUS) / C.BELT_CELL);
        const maxY = Math.floor((c0y + C.BELT_STREAM_RADIUS) / C.BELT_CELL);
        let changed = false;
        for (const [key, cell] of layer.cells) {
            if (cell.cx < minX || cell.cx > maxX || cell.cy < minY || cell.cy > maxY) {
                this._release(cell.bodies);
                layer.cells.delete(key);
                changed = true;
            }
        }
        for (let cx = minX; cx <= maxX; cx++) {
            for (let cy = minY; cy <= maxY; cy++) {
                // ячейка целиком вне полосы тел не даёт («за краем камней нет»);
                // полоса — мировая (±BELT_HALF_WIDTH), координаты слоя = world·p.
                if ((cy + 1) * C.BELT_CELL <= -C.BELT_HALF_WIDTH * layer.p) continue;
                if (cy * C.BELT_CELL >= C.BELT_HALF_WIDTH * layer.p) continue;
                const key = cx + ',' + cy;
                if (layer.cells.has(key)) continue;
                layer.cells.set(key, { cx, cy, bodies: gen(cx, cy) });
                changed = true;
            }
        }
        if (changed) this._rebuild(layer);
        this._trim(layer, c0x, c0y);
    }

    _rebuild(layer) {
        const arr = layer.bodies;
        arr.length = 0;
        for (const cell of layer.cells.values()) {
            for (const b of cell.bodies) arr.push(b);
        }
    }

    // _trim — кап слоя: при переполнении отпускаем самые дальние ячейки.
    _trim(layer, c0x, c0y) {
        if (layer.cap === Infinity) return;
        while (layer.bodies.length > layer.cap && layer.cells.size > 0) {
            let farKey = null;
            let farD = -1;
            for (const [key, cell] of layer.cells) {
                const dx = (cell.cx + 0.5) * C.BELT_CELL - c0x;
                const dy = (cell.cy + 0.5) * C.BELT_CELL - c0y;
                const d = dx * dx + dy * dy;
                if (d > farD) { farD = d; farKey = key; }
            }
            const cell = layer.cells.get(farKey);
            this._release(cell.bodies);
            layer.cells.delete(farKey);
            this._rebuild(layer);
        }
    }

    _release(bodies) {
        for (const b of bodies) this._pool.push(b);
    }

    // ---- Физика корабля и мира ----

    update(dt, input) {
        this._updateShip(dt, input);
        this._stream();
        this._updateBodies(dt);
        this._collisions();
        this._updateEffects(dt);
    }

    // _updateShip — схема «два стика» (§5.1): W/S — плавная тяга по носу,
    // A/D — стрейф, мышь — ориентация носа (ограниченная угловая скорость),
    // Shift — тормоз. Границы пятна/отскока нет (пояс бесконечен вдоль кольца).
    _updateShip(dt, input) {
        const s = this.ship;

        // Тяга — плавно меняющаяся величина: W/S ведут к +1/−1, отпускание — сброс.
        let t = s.throttle;
        if (input.forward && !input.back) {
            t = Math.min(1, t + C.SHIP_THROTTLE_RATE_UP * dt);
        } else if (input.back && !input.forward) {
            t = Math.max(-1, t - C.SHIP_THROTTLE_RATE_UP * dt);
        } else {
            const rate = C.SHIP_THROTTLE_RATE_DOWN * dt;
            if (t > 0) t = Math.max(0, t - rate);
            else if (t < 0) t = Math.min(0, t + rate);
        }
        s.throttle = t;

        // Нос доворачивается к курсору с ограниченной угловой скоростью
        // (не мгновенный снап; единственный источник ориентации — мышь, §5.1).
        // input.aimWorld — мировая точка курсора (считает belt_main из камеры).
        if (input.aimWorld) {
            const target = Math.atan2(input.aimWorld.y - s.y, input.aimWorld.x - s.x);
            const diff = normAngle(target - s.heading);
            const abs = Math.abs(diff);
            if (abs < C.SHIP_TURN_DEADZONE) {
                s.angVel *= Math.max(0, 1 - 12 * dt);
                if (Math.abs(s.angVel) < 0.02) s.angVel = 0;
            } else {
                const dir = diff > 0 ? 1 : -1;
                const stop = (s.angVel * s.angVel) / (2 * C.SHIP_TURN_ACCEL);
                if (dir * s.angVel > 0 && abs <= stop) {
                    s.angVel -= dir * C.SHIP_TURN_ACCEL * dt;
                } else {
                    s.angVel += dir * C.SHIP_TURN_ACCEL * dt;
                }
                s.angVel = Math.max(-C.SHIP_TURN_RATE_MAX, Math.min(C.SHIP_TURN_RATE_MAX, s.angVel));
            }
            s.heading = normAngle(s.heading + s.angVel * dt);
        }

        // Тяга прикладывается в мировых осях от ориентации носа.
        const dirX = Math.cos(s.heading);
        const dirY = Math.sin(s.heading);
        const fwd = t >= 0 ? t : t * C.SHIP_REVERSE_FACTOR;
        let strafe = 0;
        if (input.left) strafe -= 1;
        if (input.right) strafe += 1;
        strafe *= C.SHIP_STRAFE_FACTOR;
        s.thrusting = Math.abs(t) > 0.02 || strafe !== 0;
        s.braking = !!input.brake;
        // «Вправо от носа» при экранных осях (y вниз) — поворот на +90°.
        s.vx += (dirX * fwd - dirY * strafe) * C.SHIP_THRUST * dt;
        s.vy += (dirY * fwd + dirX * strafe) * C.SHIP_THRUST * dt;

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
    }

    // _updateBodies — дрейф и вращение жил и обломков; ушедшие поперёк за
    // BELT_HALF_WIDTH дрейфом отпускаются (§8.2).
    _updateBodies(dt) {
        for (const layer of [this._layers.a, this._layers.d]) {
            for (const b of layer.bodies) {
                b.x += b.vx * dt;
                b.y += b.vy * dt;
                b.rot += b.rotSpeed * dt;
            }
            this._cull(layer);
        }
    }

    _cull(layer) {
        let removed = false;
        const halfLayer = C.BELT_HALF_WIDTH * layer.p; // мировая полоса → координаты слоя
        for (const cell of layer.cells.values()) {
            const arr = cell.bodies;
            for (let i = arr.length - 1; i >= 0; i--) {
                if (Math.abs(arr[i].y) > halfLayer) {
                    this._pool.push(arr[i]);
                    arr.splice(i, 1);
                    removed = true;
                }
            }
        }
        if (removed) this._rebuild(layer);
    }

    // Столкновения с жилами — мягкое отталкивание без урона (§5.2, §10.2-A).
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
                s.x = a.x + nx * (min + 0.5);
                s.y = a.y + ny * (min + 0.5);
                const vn = s.vx * nx + s.vy * ny;
                if (vn < -5) this.collided = true;
                if (vn < 0) {
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

    // rayVein — ближайшая крупная жила, чьё тело пересекает луч прицела
    // (из носа вдоль heading; попадание считается по a.r, §5.2). null — цели нет.
    rayVein() {
        const s = this.ship;
        const dx = Math.cos(s.heading);
        const dy = Math.sin(s.heading);
        let best = null;
        let bestT = Infinity;
        for (const a of this.asteroids) {
            // Жилы выработанного ресурса целью не становятся (§8.1): клиент
            // «гасит» их, подсказка у цели работает как «нет жилы» (состояние 0).
            if (a.res && this.depletedRes[a.res]) continue;
            const fx = s.x - a.x;
            const fy = s.y - a.y;
            const b = fx * dx + fy * dy;
            const c = fx * fx + fy * fy - a.r * a.r;
            const disc = b * b - c;
            if (disc < 0) continue;
            const sq = Math.sqrt(disc);
            let t = -b - sq;
            if (t < 0) t = -b + sq; // корабль внутри тела — берём дальнее пересечение
            if (t < 0) continue;
            if (t < bestT) { bestT = t; best = a; }
        }
        return best;
    }

    // distToSurface — расстояние от корабля до поверхности тела (по a.r).
    distToSurface(a) {
        const s = this.ship;
        return Math.hypot(s.x - a.x, s.y - a.y) - a.r;
    }

    // aimedVein — цель бурения: жила под лучом носа, чей край в EXTRACT_RADIUS
    // (§5.2). Отдельного «визуального радиуса» нет — попадание по a.r.
    aimedVein() {
        const a = this.rayVein();
        if (!a) return null;
        return this.distToSurface(a) <= C.EXTRACT_RADIUS ? a : null;
    }

    // drill — удержание бурения у целевой жилы: чисто визуальный прогресс
    // жилы + частицы руды к кораблю (§5.2, Б8).
    drill(target, dt) {
        if (!target) return;
        target.drill = Math.min(1, target.drill + dt * 0.22);
        const base = Math.atan2(this.ship.y - target.y, this.ship.x - target.x);
        // Цвет частиц — по ресурсу цели (лёд — холодный блеск, §8.1/§10.9).
        const pc = target.res === 'ice' ? C.COLORS.iceGlint : C.COLORS.particle;
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
                c: pc,
            });
        }
    }

    // addFloater — всплывающее «+X т» (сцена, §9.1 п.5).
    addFloater(text, x, y) {
        this.floaters.push({ text, x, y, life: 1.1, max: 1.1 });
    }
}
