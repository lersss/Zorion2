import { PLAYER_W, PLAYER_H, SPAWN_STEP, SPAWN_WINDOW, SPAWN_WINDOW_MAX } from './surface_config.js';
import { SurfaceWorld } from './surface_world_core.js';
    // ==================== СПАВН (ЧК6.2 §6.2) ====================
    //
    // spawnX/spawnY — стартовая колонка: «пол выше зеркала» (`floorY < liquidLevel`)
    // — это и сухой берег, и верх ледовой корки `underIce`; колонка не задета 2D-формами
    // (`±2·PLAYER_W`, тот же запас, что у worldgen-защиты `_formStage1Ok`); коробка
    // спавна свободна по `solidAt'`. Вне жидкости (`liquid` null) — x=0 (прежнее
    // поведение). Фолбэки (§6.2): расширенное окно → зеркало-фолбэк (плавучесть,
    // `floatY`) → ближайшая колонка по полу. Считается лениво и один раз (мемо).

    // ensureSpawn — посчитать стартовую колонку (мемо). Probe-миры выходимости
    // (`_skipEscape`/`_stage2Active`) пропускают расчёт: их p.x/p.y всё равно
    // перезаписываются, а `spawnX` нужен лишь как безопасное число (§6.2).
SurfaceWorld.prototype.ensureSpawn = function() {
    if (this._spawnReady) return;
    if (this._skipEscape || this._stage2Active || !this.liquid) {
        this._spawnX = 0;
        this._spawnY = this.floorY(0) - PLAYER_H / 2 - 2;
        this._spawnReady = true;
        return;
    }
    const s = this._scanSpawn(SPAWN_WINDOW / 2, false)
        || this._scanSpawn(SPAWN_WINDOW_MAX / 2, false)
        || this._scanSpawn(SPAWN_WINDOW_MAX / 2, true)
        || this._scanFloor(SPAWN_WINDOW_MAX / 2);
    this._spawnX = s ? s.x : 0;
    this._spawnY = s ? s.y : (this.floorY(0) - PLAYER_H / 2 - 2);
    this._spawnReady = true;
};

Object.defineProperty(SurfaceWorld.prototype, "spawnX", { configurable: true, get: function() {
    this.ensureSpawn();
    return this._spawnX;
} });

Object.defineProperty(SurfaceWorld.prototype, "spawnY", { configurable: true, get: function() {
    this.ensureSpawn();
    return this._spawnY;
} });

    // _scanSpawn — перебор от x=0 в обе стороны шагом SPAWN_STEP. mirror=true —
    // зеркало-фолбэк (игрок плавает): колонка с водой (или полынья подо льдом) и
    // без форм, `y = floatY`. Иначе — сухая/корковая колонка с пробой коробки.
SurfaceWorld.prototype._scanSpawn = function(halfWindow, mirror) {
    for (let d = 0; d <= halfWindow; d += SPAWN_STEP) {
        for (const x of (d === 0 ? [0] : [d, -d])) {
            const hit = mirror ? this._spawnMirrorAt(x) : this._spawnDryAt(x);
            if (hit) return hit;
        }
    }
    return null;
};

SurfaceWorld.prototype._spawnDryAt = function(x) {
    if (!(this.floorY(x) < this.liquidLevel(x))) return false;
    if (!this._columnFormFree(x)) return false;
    const y = this.floorY(x) - PLAYER_H / 2 - 2;
    if (!this._spawnBoxFree(x, y)) return false;
    return { x, y };
};

SurfaceWorld.prototype._spawnMirrorAt = function(x) {
    if (!this._columnFormFree(x)) return false;
    if (this._liquidCrust) {
        if (!this.polynyaAt(x) || !this._liquidCol(Math.floor(x))) return false;
    } else if (this.liquidDepth(x) < this.liquid.level.minDepth) {
        return false;
    }
    return { x, y: this.floatY(x) };
};

    // _scanFloor — последний фолбэк (§6.2 п.4, гарантии нет): ближайшая колонка по полу.
SurfaceWorld.prototype._scanFloor = function(halfWindow) {
    for (let d = 0; d <= halfWindow; d += SPAWN_STEP) {
        for (const x of (d === 0 ? [0] : [d, -d])) {
            if (this.floorY(x) < this.liquidLevel(x)) return { x, y: this.floorY(x) - PLAYER_H / 2 - 2 };
        }
    }
    return null;
};

    // _columnFormFree — ни один интервал 2D-формы не перекрывает `x ± 2·PLAYER_W`
    // (тот же запас, что у `_formStage1Ok`). Берётся из уже имеющихся интервалов
    // форм (`_collectFormIntervals`); для колонок без форм условие истинно.
SurfaceWorld.prototype._columnFormFree = function(x) {
    if (!this.forms) return true;
    const half = 2 * PLAYER_W;
    for (let d = -half; d <= half; d += 1) {
        if (this._collectFormIntervals(x + d, undefined).length) return false;
    }
    return true;
};

    // _spawnBoxFree — коробка спавна (w×h вокруг центра x,y) свободна по `solidAt'`
    // (углы + центр), тем же принципом, что `_blockedX`/`_blockedUp`.
SurfaceWorld.prototype._spawnBoxFree = function(x, y) {
    const hw = PLAYER_W / 2 - 0.5, hh = PLAYER_H / 2 - 0.5;
    return !(this.solidAt(x - hw, y - hh) || this.solidAt(x + hw, y - hh)
        || this.solidAt(x - hw, y + hh) || this.solidAt(x + hw, y + hh)
        || this.solidAt(x, y));
};
