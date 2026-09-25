// web/static/js/surface/surface_player.js
// Физика игрока прогулки (спека 2026-09-21 §7.4): ходьба/спринт/прыжок,
// гравитация с клипом 0.2–2.5, плавание. HP — серверно-авторитетный (§8.7):
// клиент только отображает формулу от landed_at + hazard.total.
import {
    PPM, WALK_SPEED, SPRINT_SPEED, JUMP_SPEED,
    G_CLAMP_MIN, G_CLAMP_MAX, G_EARTH, PLAYER_H, PLAYER_W,
    BUOY_K, BUOY_DAMP, SWIM_TERM, SWIM_FACTOR, SWIM_ACCEL,
    DIVE_SPEED, DIVE_ACCEL, ASCEND_SPEED,
} from './surface_config.js';

function approach(cur, target, maxDelta) {
    if (cur < target) return Math.min(cur + maxDelta, target);
    if (cur > target) return Math.max(cur - maxDelta, target);
    return cur;
}

export class Player {
    constructor(world, gravityG) {
        this.world = world;
        this.vx = 0;
        this.vy = 0;
        this.w = PLAYER_W;
        this.h = PLAYER_H;
        // Спавн — на стартовой колонке мира (§6.2): сухой берег/корка (`floorY`) или
        // зеркало-фолбэк (`floatY`). Числа считает мир (одна реализация); пробы
        // (`Object.create`) выходят из расчёта дёшево. Нет чисел (урезанный probe) —
        // прежняя точка x=0, центр на h/2+2 выше пола `floorY` (§5 п.12).
        if (typeof world.ensureSpawn === 'function') world.ensureSpawn();
        this.x = Number.isFinite(world.spawnX) ? world.spawnX : 0;
        this.y = Number.isFinite(world.spawnY) ? world.spawnY : world.floorY(this.x) - this.h / 2 - 2;
        this.onGround = false;
        this.facing = 1;
        this.distance = 0;
        this.gClip = Math.max(G_CLAMP_MIN, Math.min(G_CLAMP_MAX, gravityG || 1));
        this.gravity = G_EARTH * this.gClip;
        this.terminal = 60 * PPM;
    }

    solidAt(px, py) {
        return this.world.solidAt(px, py);
    }

    // Угловая коллизия (§5 п.4): выборка бокса w×h углами + центром вместо
    // точечной пробы. Числа (PPM/скорости/гравитация/w/h) не меняются — меняется
    // только выборка. SKIN — микро-отступ боковых проб, чтобы корпус, стоящий
    // вплотную к стене, не «прилипал» к её грани (стена и точка кромки — не
    // перекрытие).
    // Горизонталь: верх бокса (голова/грудь) + центр; НИЗ не выбирается — кромка
    // стопы на склоне всегда чуть в земле, это не стена, а подъём (его разрешает
    // вертикаль); полная выборка бокса запирала бы игрока на любом подъёме.
    _blockedX(nx) {
        const hw = this.w * 0.5, hh = this.h * 0.5;
        return this.solidAt(nx - hw, this.y - hh)
            || this.solidAt(nx + hw, this.y - hh)
            || this.solidAt(nx - hw, this.y - hh * 0.5)
            || this.solidAt(nx + hw, this.y - hh * 0.5)
            || this.solidAt(nx, this.y - hh * 0.5);
    }

    // Опора (падение): оба нижних угла + центр низа — корпус опирается на склон,
    // а не проваливается углом в породу; при опоре точка кромки не считается
    // (SKIN), иначе стойка вплотную к стене «поднимала» бы игрока по её грани.
    _blockedDown() {
        const hw = this.w * 0.5 - 0.5, by = this.y + this.h * 0.5;
        return this.solidAt(this.x - hw, by) || this.solidAt(this.x + hw, by) || this.solidAt(this.x, by);
    }

    // _blockedDownNear — опора с допуском контакта (§6.1 «стоять на дну»):
    // разрешение столкновений оставляет зазор < 1 px над полом (`while` шагает по
    // 1 px), поэтому `_blockedDown` мигает каждый кадр. В жидкости это срывало бы
    // игрока с дна плавучестью; допуск (1.5 px) возвращает устойчивую опору.
    // Применяется ТОЛЬКО в жидкости — вне неё поведение прежнее (§9 п.17).
    _blockedDownNear() {
        const hw = this.w * 0.5 - 0.5, by = this.y + this.h * 0.5 + 1.5;
        return this.solidAt(this.x - hw, by) || this.solidAt(this.x + hw, by) || this.solidAt(this.x, by);
    }

    // Потолок (подъём): оба верхних угла + центр верха — удар в тонкий свод/потолок.
    _blockedUp() {
        const hw = this.w * 0.5 - 0.5, ty = this.y - this.h * 0.5;
        return this.solidAt(this.x - hw, ty) || this.solidAt(this.x + hw, ty) || this.solidAt(this.x, ty);
    }

    update(dt, input) {
        // Признак жидкости — единственный: центр бокса в теле жидкости (§6.1);
        // физика и отрисовка читают один уровень `liquidLevel` (§9 п.14).
        const inLiq = this.world.liquidAt(this.x, this.y) !== '';

        // Голова в твёрдом подо льдом (низ корки `underIce`) — вытолкнуть вниз ДО
        // горизонтали: иначе `_blockedX` считает углы головы твёрдыми и запирает
        // игрока (§6.6 — застревание под коркой недопустимо), а `_blockedUp` при
        // `vy ≥ 0` не срабатывает (прижатый к низу корки/мели игрок остаётся
        // вмурован). Условие — над колонкой есть лёд (`crustTop`) или игрок в
        // жидкости: на суше и в открытой воде поведение прежнее.
        if (this._blockedUp() && (inLiq || this.world.crustTop(this.x) !== null)) {
            let guard = 0;
            while (this._blockedUp() && guard++ < 64) this.y += 1;
            if (this.vy < 0) this.vy = 0;
        }

        // Горизонталь: в жидкости — SWIM_FACTOR и SWIM_ACCEL (спринт выключен);
        // вне жидкости — прежние WALK/SPRINT и ускорение (§6.1, §9 п.17).
        const speed = inLiq ? WALK_SPEED * PPM * SWIM_FACTOR : (input.sprint ? SPRINT_SPEED : WALK_SPEED) * PPM;
        const accel = inLiq ? SWIM_ACCEL : (this.onGround ? 12 : 4) * PPM;
        let target = 0;
        if (input.left) target -= speed;
        if (input.right) target += speed;
        this.vx = approach(this.vx, target, accel * dt);

        if (inLiq) {
            // Три режима вертикали (§6.1): ныряние `down`, плавучесть-пружина,
            // всплытие «прыжком». Гравитация в жидкости ×0.12 (инвариант).
            const G = this.gravity * 0.12 * PPM;
            if (input.down) {
                this.vy = Math.min(this.vy + (G + DIVE_ACCEL) * dt, DIVE_SPEED);
            } else {
                let a = G;
                if (!this.onGround) a += BUOY_K * (this.world.floatY(this.x) - this.y) - BUOY_DAMP * this.vy;
                this.vy += a * dt;
                if (input.jump) this.vy = -ASCEND_SPEED;
            }
            if (this.vy > SWIM_TERM) this.vy = SWIM_TERM;
            else if (this.vy < -SWIM_TERM) this.vy = -SWIM_TERM;
        } else {
            this.vy += this.gravity * PPM * dt;
            if (this.vy > this.terminal) this.vy = this.terminal;
        }

        if (this.vx !== 0) this.facing = this.vx > 0 ? 1 : -1;

        // Горизонталь: стена — отказ, иначе движение (угловая выборка бокса, §5 п.4).
        const nx = this.x + this.vx * dt;
        if (!this._blockedX(nx)) {
            this.distance += Math.abs(nx - this.x);
            this.x = nx;
        } else {
            this.vx = 0;
        }

        // Вертикаль: падение/подъём с разрешением столкновений (кромки бокса).
        this.y += this.vy * dt;
        if (this.vy >= 0 && this._blockedDown()) {
            let guard = 0;
            while (this._blockedDown() && guard++ < 64) this.y -= 1;
            this.vy = 0;
            this.onGround = true;
        } else {
            this.onGround = false;
        }
        if (this.vy < 0 && this._blockedUp()) {
            let guard = 0;
            while (this._blockedUp() && guard++ < 64) this.y += 1;
            this.vy = 0;
        }
        // Опора на дно в жидкости (§6.1): удерживаем `onGround` в пределах допуска
        // контакта, иначе плавучесть срывала бы игрока со дна на первом кадре.
        if (inLiq && this.vy >= 0 && !this.onGround && this._blockedDownNear()) {
            this.vy = 0;
            this.onGround = true;
        }

        if (input.jump && this.onGround && !inLiq) {
            this.vy = -JUMP_SPEED * PPM;
            this.onGround = false;
        }

        return this;
    }
}

// serverHp — серверная формула HP (§8.7): max(0, 100 − total·Δt). Клиент урон
// не считает и не присылает — только отображает это значение. Верхний клам
// 100 — landed_at округлён вверх и может быть на доли секунды впереди часов
// клиента (иначе просадка/перелёт полоски на старте).
export function serverHp(pkg, nowMs) {
    const landed = Date.parse(pkg.landed_at);
    if (!isFinite(landed)) return 100;
    const dt = Math.max(0, (nowMs - landed) / 1000);
    const hp = 100 - (pkg.hazard ? pkg.hazard.total : 0) * dt;
    if (hp > 100) return 100;
    return hp < 0 ? 0 : hp;
}
