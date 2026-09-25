// web/static/js/surface/surface_player.js
// Физика игрока прогулки (спека 2026-09-21 §7.4): ходьба/спринт/прыжок,
// гравитация с клипом 0.2–2.5, плавание. HP — серверно-авторитетный (§8.7):
// клиент только отображает формулу от landed_at + hazard.total.
import {
    PPM, WALK_SPEED, SPRINT_SPEED, JUMP_SPEED,
    G_CLAMP_MIN, G_CLAMP_MAX, G_EARTH, PLAYER_H, PLAYER_W,
} from './surface_config.js';

function approach(cur, target, maxDelta) {
    if (cur < target) return Math.min(cur + maxDelta, target);
    if (cur > target) return Math.max(cur - maxDelta, target);
    return cur;
}

export class Player {
    constructor(world, gravityG) {
        this.world = world;
        this.x = 0;
        this.vx = 0;
        this.vy = 0;
        this.w = PLAYER_W;
        this.h = PLAYER_H;
        // Спавн — на корке поверхности: центр на h/2+2 выше ПОЛА `floorY` (§5 п.12).
        // Иначе ноги (th+10) ниже 8-px корки, и игрок проваливается в пещеру под
        // спавном, откуда не выйти (не запираться на спавне, идея 2026-09-21 §4 п.3).
        // На обычной колонке floorY = terrainHeight («спавн на корке»); вычитающая
        // форма, задевающая спавн (±2·PLAYER_W), не применяется (§3.6).
        this.y = world.floorY(0) - this.h / 2 - 2;
        this.onGround = false;
        this.facing = 1;
        this.distance = 0;
        // Вода: уровень для гидросферных биомов (плавание §7.4).
        this.waterY = world.category === 'вода' ? world.baseY + 170 : Infinity;
        this.gClip = Math.max(G_CLAMP_MIN, Math.min(G_CLAMP_MAX, gravityG || 1));
        this.gravity = G_EARTH * this.gClip;
        this.terminal = 60 * PPM;
    }

    inWater() {
        return this.y + this.h * 0.5 > this.waterY;
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

    // Потолок (подъём): оба верхних угла + центр верха — удар в тонкий свод/потолок.
    _blockedUp() {
        const hw = this.w * 0.5 - 0.5, ty = this.y - this.h * 0.5;
        return this.solidAt(this.x - hw, ty) || this.solidAt(this.x + hw, ty) || this.solidAt(this.x, ty);
    }

    update(dt, input) {
        const speed = (input.sprint ? SPRINT_SPEED : WALK_SPEED) * PPM;
        let target = 0;
        if (input.left) target -= speed;
        if (input.right) target += speed;
        const accel = (this.onGround ? 12 : 4) * PPM;
        this.vx = approach(this.vx, target, accel * dt);

        const swimming = this.inWater();
        const g = swimming ? this.gravity * 0.12 : this.gravity;
        this.vy += g * PPM * dt;
        const term = swimming ? 8 * PPM : this.terminal;
        if (this.vy > term) this.vy = term;
        if (swimming && input.jump) this.vy = -2.5 * PPM;

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

        if (input.jump && this.onGround && !swimming) {
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
