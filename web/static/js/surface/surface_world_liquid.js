import { hash1, hash2 } from './surface_world_noise.js';
import { PLAYER_H, FLOAT_SUBMERGE, LIQUID_ICE_CLEAR_MIN, LIQUID_POLYNYA_MIN_W } from './surface_config.js';
import { SurfaceWorld } from './surface_world_core.js';
    // ==================== СЛОЙ ЖИДКОСТИ (ЧК6, §3) ====================
    //
    // Жидкость — ОТДЕЛЬНЫЙ слой, не твёрдость: `liquidAt` ∩ `solidAt' = ∅` (кроме
    // корки underIce — единственного места, где слой меняет твёрдость, §3.3).
    // Одна реализация уровня (`liquidLevel`) читается и физикой, и отрисовкой (§9 п.3).

    // floorY — «пол/опора» (§2.1 + ЧК6 §3.2): на underIce опора — верх ледовой
    // корки (по ней ходят, она несущая), иначе — верх грунта (без корки).
SurfaceWorld.prototype.floorY = function(x) {
    const cr = this.crustTop(x);
    if (cr !== null) return cr;
    return this._groundFloorY(x);
};

    // _polyWindow — ОКНО полыньи периода `n` (или null), уже приведённое к воде.
    // Номинальное окно [pos, pos+w) детерминировано от seed (`hash1`/`hash2`, §3.2/N3)
    // и пересекается с проходимой водой (`bedY − lv ≥ LIQUID_ICE_CLEAR_MIN`); берём
    // САМЫЙ ШИРОКИЙ непрерывный кусок. Короче `LIQUID_POLYNYA_MIN_W` — окна нет вовсе
    // (лёд непрерывно): остаток от мелководья не влезет в бокс игрока, а тонкая дырка
    // в плите хуже её отсутствия. Мемо по периоду `n`: значение зависит только от `n`,
    // порядок запросов колонок не влияет (OBS-1, §3.1).
SurfaceWorld.prototype._polyWindow = function(n) {
    const memo = this._polyMemo || (this._polyMemo = new Map());
    const hit = memo.get(n);
    if (hit !== undefined) return hit;
    const p = this.liquid.level.polynya;
    const wMin = Math.min(p.w[0], p.w[1]);
    const wMax = Math.max(p.w[0], p.w[1]);
    const pos = n * p.gap + hash1(n, (this.seed ^ 0x7011) >>> 0) * Math.max(0, p.gap - wMax);
    const w = wMin + hash2(n, 0, (this.seed ^ 0x7022) >>> 0) * Math.max(0, wMax - wMin);
    const end = pos + w;
    let bx = -1, bw = 0, cs = -1;
    for (let x = pos; x < end; x++) {
        const c = this._liquidCol(Math.floor(x));
        if (c && c.bd - c.lv >= LIQUID_ICE_CLEAR_MIN) {
            if (cs < 0) cs = x;
        } else if (cs >= 0) {
            if (x - cs > bw) { bw = x - cs; bx = cs; }
            cs = -1;
        }
    }
    if (cs >= 0 && end - cs > bw) { bw = end - cs; bx = cs; }
    const out = bw >= LIQUID_POLYNYA_MIN_W ? { x: bx, w: bw } : null;
    if (memo.size > 40000) memo.clear();
    memo.set(n, out);
    return out;
}
;

    // polynyaAt — полынья underIce (окно без корки, вход под лёд; §3.2/N3):
    // детерминирована от seed, ширина `w ≥ PLAYER_W+2` задана данными, окно стоит
    // только над проходимой водой (`_polyWindow`). Вне underIce полыней нет.
SurfaceWorld.prototype.polynyaAt = function(x) {
    if (!this._liquidCrust) return false;
    const win = this._polyWindow(Math.floor(x / this.liquid.level.polynya.gap));
    return win !== null && x >= win.x && x < win.x + win.w;
};

    // liquidLevel — уровень зеркала (§3.2): global/underIce — `baseY + offset`
    // (константа); basin — уровень локальной впадины. Infinity — жидкости нет.
SurfaceWorld.prototype.liquidLevel = function(x) {
    const L = this.liquid;
    if (!L) return Infinity;
    if (L.level.mode === 'basin') return this._basinLevel(x);
    return this.baseY + L.level.offset;
};

    // _basinLevel — уровень впадины (§3.2/M7): заполняем до низшей точки перелива —
    // вода поднимается над полом `floorY` до НИЗШЕЙ из двух стенок чаши (max двух
    // «верхов» стенок, `_basinRim`); окно `level.window`. Мемо (мир статичен).
SurfaceWorld.prototype._basinLevel = function(x) {
    const memo = this._basinMemo || (this._basinMemo = new Map());
    const key = Math.floor(x / 2);
    const hit = memo.get(key);
    if (hit !== undefined) return hit;
    // Уровень считаем на ПРЕДСТАВИТЕЛЕ корзины 2 px (`key*2`), а не на первом
    // запрошенном x: иначе результат зависел от порядка обхода колонок
    // (`liquidDepth(x)` ≠ после `liquidDepth(x+1)`) — «нарисовано ≠ плывётся».
    const bx = key * 2;
    const lv = Math.max(this._basinRim(bx, -1), this._basinRim(bx, 1));
    if (memo.size > 60000) memo.clear();
    memo.set(key, lv);
    return lv;
};

    // _basinRim — ВЕРХ стенки чаши в сторону dir: минимум `floorY` по окну window
    // (y растёт вниз, поэтому верх = минимальный y). Уровень — НИЗШАЯ из двух
    // стенок (max двух «верхов»), см. `_basinLevel`. Окно включает саму x: на
    // монотонном склоне сторона «вниз» даёт `floorY(x)` → сухо.
SurfaceWorld.prototype._basinRim = function(x, dir) {
    const win = this.liquid.level.window;
    const step = 4;
    let m = this.floorY(x);
    for (let d = step; d <= win; d += step) {
        const cur = this.floorY(x + dir * d);
        if (cur < m) m = cur;
    }
    return m;
};
    // _iceBottom — НИЗ ледовой корки подлёдной колонки `c` (`_liquidCol`, несёт `bd`).
    // Плавучий лёд — низ на зеркале. Мелководье (просвет между зеркалом и дном меньше
    // `LIQUID_ICE_CLEAR_MIN`) — лёд СЕЛ НА ДНО: низ = `bedY`, воды в колонке не
    // остаётся. Так свободное место подо льдом либо 0, либо ≥ габарита игрока:
    // промежуточной щели, куда бокс 28 px не проходит ни в одну сторону, не бывает
    // (хвост №84). Колонка с коркой всегда затоплена (`bd > lv`), иначе нет ни воды,
    // ни льда: `_liquidCol` отсекает сушу по `minDepth`.
SurfaceWorld.prototype._iceBottom = function(c) {
    return c.bd - c.lv < LIQUID_ICE_CLEAR_MIN ? c.bd : c.lv;
}
;

    // crustTop — верх ледовой корки underIce (§3.2): `liquidLevel − iceH` на
    // затопленной колонке вне полыньи; null — корки нет. Корка исключена из
    // `bedY` (N1), но входит в `floorY`/`skyTop`/`solidAt'`.
SurfaceWorld.prototype.crustTop = function(x) {
    if (!this._liquidCrust) return null;
    const c = this._liquidCol(Math.floor(x));
    if (!c || this.polynyaAt(x)) return null;
    return c.lv - this.liquid.level.iceH;
}
;

    // crustBottom — НИЗ ледовой корки (null — корки нет): зеркало у плавучей плиты,
    // `bedY` у севшей на дно. Верх — `crustTop`.
SurfaceWorld.prototype.crustBottom = function(x) {
    if (!this._liquidCrust) return null;
    const c = this._liquidCol(Math.floor(x));
    if (!c || this.polynyaAt(x)) return null;
    return this._iceBottom(c);
}
;
    // crustAt — точка внутри корки underIce (твёрдая плита, §3.2): полоса
    // [crustTop, crustBottom] вне полыней — у мелководья плита толще, до дна. Флаг
    // `_inCrust` отключает корку при вычислении дна (`bedY`), чтобы не было рекурсии.
SurfaceWorld.prototype.crustAt = function(x, y) {
    if (!this._liquidCrust || this._inCrust) return false;
    const c = this._liquidCol(Math.floor(x));
    if (!c || this.polynyaAt(x)) return false;
    return y >= c.lv - this.liquid.level.iceH && y <= this._iceBottom(c);
}
;

    // bedY — дно жидкости (§3.1): верх грунта, СВЯЗАННОГО с базой, ниже зеркала;
    // корка underIce в bedY НЕ входит (N1) — её верх = `floorY`. = пол без корки.
SurfaceWorld.prototype.bedY = function(x) {
    if (!this.liquid) return Infinity;
    if (!this._liquidCrust) return this._groundFloorY(x);
    this._inCrust = true;
    const bd = this._groundFloorY(x);
    this._inCrust = false;
    return bd;
};

    // _liquidCol — кэш колонки жидкости {lv, depth, bd} (мир статичен; растр и
    // фронтальный проход зовут многократно). null — жидкости в колонке нет
    // (сухо/лужа мельче `minDepth`). Ключ — целый x (1-px разрешение).
SurfaceWorld.prototype._liquidCol = function(xi) {
    const L = this.liquid;
    if (!L) return null;
    const memo = this._liqColMemo || (this._liqColMemo = new Map());
    let c = memo.get(xi);
    if (c !== undefined) return c;
    const lv = this.liquidLevel(xi);
    if (lv === Infinity) {
        c = null;
    } else {
        const bd = this.bedY(xi);
        const raw = Math.max(0, bd - lv);
        const depth = L.level.maxDepth > 0 ? Math.min(raw, L.level.maxDepth) : raw;
        c = depth >= L.level.minDepth ? { lv, bd, depth } : null;
    }
    if (memo.size > 40000) memo.clear();
    memo.set(xi, c);
    return c;
};

    // liquidAt — жидкость в точке (§3.1): `liquidLevel ≤ y ≤ bedY` ∧ ¬`solidAt'`,
    // где `solidAt' = solidAt ∨ crust`. Вода не внутри камня/корки; «ходьба по
    // воде» невозможна. Возвращает имя среды или "" (нет жидкости).
SurfaceWorld.prototype.liquidAt = function(x, y) {
    const L = this.liquid;
    if (!L) return '';
    const c = this._liquidCol(Math.floor(x));
    if (!c) return '';
    if (y < c.lv || y > c.lv + c.depth) return '';
    if (this.solidAt(x, y)) return '';
    return L.medium;
};

    // liquidDepth — глубина жидкости колонки (§3.1): max(0, bedY − liquidLevel),
    // урезанная `maxDepth`; 0 — сухо.
SurfaceWorld.prototype.liquidDepth = function(x) {
    const c = this._liquidCol(Math.floor(x));
    return c ? c.depth : 0;
};

    // floatY — линия плавучести (§6.1): координата центра в равновесии «лежу на
    // воде»; голова — на `h·FLOAT_SUBMERGE` выше зеркала. Одна реализация для
    // физики (Player) и рендера — второго пути нет.
SurfaceWorld.prototype.floatY = function(x) {
    return this.liquidLevel(x) + PLAYER_H * FLOAT_SUBMERGE;
};
