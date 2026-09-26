import { hash1, fbm1, noise2, smoothstep, num } from './surface_world_noise.js';
import { FLOAT_SPAN, FLOAT_GAP } from './surface_config.js';
import { primHeight } from './surface_world_prims.js';
import { SurfaceWorld } from './surface_world_core.js';

SurfaceWorld.prototype._formationForRegion = function (idx) {
    const h = hash1(idx, this.seed ^ 0x51ed);
    return this.formations[Math.floor(h * this.formations.length) % this.formations.length];
};

// formationBlend — плавная (непрерывная) смесь формации региона с предыдущей:
// на границе обе стороны дают одну высоту. С прежней тройкой prev+cur+next
// на границе скачок до ~128 px — «обрыв мира», а парящий камень повисал бы
// ниже игрока (упор в стену, идея 2026-09-21 §4). Смешиваем на входе региона
// (prev→cur) — так высота в точке спавна x=0 (local=0) не меняется.
SurfaceWorld.prototype.formationBlend = function (x) {
    // Рецепт формы (§3.1): базовые скаляры заданы рецептом и постоянны; регион
    // нужен только для `share` слоёв. Нет рецепта — прежняя смесь формаций.
    if (this.relief) {
        const r = this.relief;
        return {
            id: this.biome,
            ridge: num(r.ridge, 0),
            flatten: num(r.flatten, 0),
            offset: num(r.offset, 0),
            caves: num(r.caves, 0),
            float: !!r.float,
            base: num(r.base, 1),
        };
    }
    const R = this.region;
    const i = Math.floor(x / R);
    const local = (x - i * R) / R;
    const prev = this._formationForRegion(i - 1);
    const cur = this._formationForRegion(i);
    const wPrev = 1 - smoothstep(0, 0.18, local);
    const wCur = 1 - wPrev;
    const mix = (k) => prev[k] * wPrev + cur[k] * wCur;
    return {
        id: cur.id,
        ridge: mix('ridge'),
        flatten: mix('flatten'),
        offset: mix('offset'),
        caves: mix('caves'),
        float: cur.float,
        base: 1,
    };
};

// layerHeight — Δy слоя (§3.1): 0, если слой выключен по региону (share < 1).
SurfaceWorld.prototype.layerHeight = function (l, x) {
    const share = num(l.share, 1);
    if (share < 1 && hash1(Math.floor(x / this.region), this.seed ^ 0x5a5a) >= share) return 0;
    return primHeight(l.prim, x, l, this.seed, this.region);
};

// sumLayers — сумма Δy слоёв стека.
SurfaceWorld.prototype.sumLayers = function (layers, x) {
    let s = 0;
    for (const l of layers) s += this.layerHeight(l, x);
    return s;
};

// terrainHeight — ФИЗИЧЕСКИЙ профиль: база (ridge/flatten/offset + fbm) + стек
// relief.layers (без viewOnly), общий множитель relief.scale (§3.1, §6 п.6).
// Нет рецепта — прежняя формула 1:1 (фолбэк §6 п.10).
SurfaceWorld.prototype.terrainHeight = function (x) {
    const f = this.formationBlend(x);
    const large = (fbm1(x * 0.0015, this.seed, 2) - 0.5) * 230 * (0.5 + 0.8 * f.ridge) * f.base;
    const detail = (fbm1(x * 0.02, this.seed ^ 0x9e37, 3) - 0.5) * 80 * (1 - 0.7 * f.flatten);
    // scale — общий множитель амплитуд (база + деталь + слои), как в бюджете
    // §4.3/§6 п.6 и в серверном валидаторе declaredReliefTop. Фолбэк: scale = 1.
    // База вычитается (крупное положительное = выше), а слои ПРИБАВЛЯЮТСЯ: у
    // примитивов Δy отрицательна = вверх (§3.1), поэтому формула спеки —
    // `baseY + offset − large − detail + Σ layers`. Внутри отрицаемого `rise`
    // слои складывать нельзя — форма переворачивается (гребень = впадина).
    let y = this.baseY + f.offset - this.reliefScale * (large + detail);
    if (this.physLayers) y += this.reliefScale * this.sumLayers(this.physLayers, x);
    return y;
};

// viewHeight — профиль ОТРИСОВКИ: физический профиль + viewOnly-слои (рябь
// песков, §3.1). Физика (isSolid/terrainHeight) их не знает.
SurfaceWorld.prototype.viewHeight = function (x) {
    if (!this.viewLayers || !this.viewLayers.length) return this.terrainHeight(x);
    return this.terrainHeight(x) + this.reliefScale * this.sumLayers(this.viewLayers, x);
};

// surfaceY — верх отрисовки поверхности: минимум физического и видового профиля
// (растр — надмножество твёрдой области, §6 п.4: viewOnly-рябь поднимает гребни,
// но не оставляет непокрашенной физическую кромку).
SurfaceWorld.prototype.surfaceY = function (x) {
    if (!this.viewLayers || !this.viewLayers.length) return this.terrainHeight(x);
    return Math.min(this.terrainHeight(x), this.viewHeight(x));
};

// farHeight — свой низкочастотный профиль дальнего плана (идея 2026-09-21
// §2.2): только крупная составляющая, без мелкой ряби ближнего рельефа, и
// форма своя (не сжатая копия terrainHeight). Детерминирован от seed —
// локальные хеши, без Math.random. Масштаб согласован с ближним рельефом.
SurfaceWorld.prototype.farHeight = function (x) {
    const large = fbm1(x * 0.0008, this.seed ^ 0x7a11, 2);
    const mid = fbm1(x * 0.0022, this.seed ^ 0x3c05, 2);
    return this.baseY - 220 - (large - 0.5) * 300 - (mid - 0.5) * 90;
};

SurfaceWorld.prototype.caveValue = function (x, y) {
    const n = noise2(x * 0.012, y * 0.02, this.seed ^ 0x1234);
    const n2 = noise2(x * 0.03, y * 0.05, this.seed ^ 0x77);
    return n * 0.7 + n2 * 0.3;
};

// caveAt — isCave с ЗАРАНЕЕ вычисленным профилем th: растр (консервативная
// маска) вызывает её на каждую пробу и не должен пересчитывать terrainHeight
// (fbm) в цикле — иначе шаг 1 px у границы пещеры упирается в бюджет
// генерации чанка. Поведение идентично `isCave(x, y)`.
SurfaceWorld.prototype.caveAt = function (x, y, th) {
    const d = y - th;
    if (d < 8) return false; // тонкая корка поверхности держит игрока
    const f = this.formationBlend(x);
    const threshold = 0.66 - 0.10 * f.caves;
    const depthBonus = Math.min(0.12, d / 3000);
    return this.caveValue(x, y) > threshold - depthBonus;
};

SurfaceWorld.prototype.isCave = function (x, y) {
    return this.caveAt(x, y, this.terrainHeight(x));
};

// ==================== ПОЛЕ ТВЁРДОСТИ (Э5.1, §2) ====================
//
// solidAt — ЕДИНСТВЕННЫЙ источник твёрдости (§2.1): база ⊕ формы. И физика
// (Player), и растр чанка читают только его — второй реализации коллизии/
// растеризации нет. В Э5.1 формы пусты → поле равно текущему миру 1:1.

// baseSolid — базовое твёрдое тело: профиль без пещер (`y ≥ terrainHeight`)
// плюс полоса парящей породы (float). Пещеры (`isCave`, 2D-шум) ВЫЧИТАЮТ
// твёрдое из базы — это часть тела, не косметика (S1-bis). Формы (relief2d)
// сюда не входят — их добавляет formsSolid (§2.2).
SurfaceWorld.prototype.baseSolid = function (x, y) {
    return this._baseSolidAt(x, y, this.terrainHeight(x));
};

// _baseSolidAt — база с ПРЕДвычисленным профилем th: без повторного fbm там,
// где профиль уже есть (растр/единое поле solidAt). Поведение идентично
// `baseSolid` (caveAt(x, y, terrainHeight(x))).
SurfaceWorld.prototype._baseSolidAt = function (x, y, th) {
    if (y >= th) return !this.caveAt(x, y, th);
    const f = this.formationBlend(x);
    if (f.float) {
        const n = noise2(x * 0.01, y * 0.01, this.seed ^ 0xa5a5);
        // Парящая порода не доходит до земли: зазор FLOAT_GAP (> роста
        // игрока) — под камнем всегда проход, стен «до земли» нет (§4 п.2).
        if (n > 0.72 && y > th - FLOAT_SPAN && y < th - FLOAT_GAP) return true;
    }
    return false;
};
