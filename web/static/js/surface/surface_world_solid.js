import { FLOAT_SPAN, FLOAT_GAP, CHUNK_TOP_MARGIN, CHUNK_HEIGHT } from './surface_config.js';
import { SurfaceWorld } from './surface_world_core.js';
    // solidAt — единое поле твёрдости (контракт §2.1, порядок §2.2: аддитивные →
    // вычитающие). Нет форм — только база (нулевая цена для прочих биомов).
    // Формы считаются ОДИН раз на пробу (thx + интервалы: иначе дорогой fbm
    // множится на число проверок знака — бюджет генерации чанка, §2.4).
SurfaceWorld.prototype.solidAt = function(x, y) {
    // Корка underIce — часть эффективного поля `solidAt'` (ЧК6 §3.3): ледовая
    // плита несущая, по ней ходят. Вне underIce `crustAt` — no-op (нулевая цена).
    if (this.crustAt(x, y)) return true;
    const forms = this.forms;
    if (!forms) return this.baseSolid(x, y);
    const thx = this.terrainHeight(x);
    const iv = this._collectFormIntervals(x, thx);
    for (const it of iv) if (!it.add && y >= it.top && y <= it.bottom) return false;
    if (this._baseSolidAt(x, y, thx)) return true;
    for (const it of iv) if (it.add && y >= it.top && y <= it.bottom) return true;
    return false;
};

    // isSolid — алиас solidAt (§2.1): прежнее имя сохранено для потребителей
    // (растр, тесты, инструменты), реализация одна.
SurfaceWorld.prototype.isSolid = function(x, y) {
    return this.solidAt(x, y);
};

    // columnSpans — аналитические интервалы БАЗЫ столбца (класс A, §2.3):
    // [{top, bottom}, …] сверху вниз. База — интервал [terrainHeight, +∞); полоса
    // float — отдельный интервал над рельефом; последний интервал ВСЕГДА база.
    // Пещеры — 2D-маска (класс A их не выражает), накладываются поверх. 2D-формы
    // идут ОТДЕЛЬНЫМ путём (`formSpans`, §2.3 — «задетые формой столбцы»), чтобы
    // не ломать контракт базы; единственный источник твёрдости — `solidAt`.
SurfaceWorld.prototype.columnSpans = function(x) {
    const th = this.terrainHeight(x);
    const spans = [{ top: th, bottom: Infinity }];
    const f = this.formationBlend(x);
    if (f.float) {
        // Границы полосы — те же константы, что у физики (FLOAT_SPAN/GAP):
        // интервал очерчивает диапазон сканирования, твёрдость внутри решает
        // `solidAt` (шум).
        spans.unshift({ top: th - FLOAT_SPAN, bottom: th - FLOAT_GAP, float: true });
    }
    return spans;
};

    // skyTop — «поверхность неба» (§2.1): МИНИМАЛЬНЫЙ y в пределах растра, где
    // solidAt(x,y)=true (верх твёрдого поля, ВКЛЮЧАЯ висящую плиту arch=1, вал
    // crater и полосу float; погода/осадки — §5 п.12). Идём от кандидата сверху вниз
    // ДО первой реально твёрдой точки: зарытый козырёк arch=0 (top НИЖЕ th) верх не
    // занижает — над ним всё равно твёрдая корка; вскрытая вычитающей формой
    // (void/crater) корка не даёт ложного «твёрдого». `void` верх НЕ поднимает.
SurfaceWorld.prototype.skyTop = function(x) {
    const th = this.terrainHeight(x);
    const iv = this.forms ? this._collectFormIntervals(x, th) : null;
    let top = Infinity;
    // Висящие аддитивные формы: их верх — граница твёрдого (берём, только если
    // точка реально твёрдая: вычитающая форма могла её вскрыть). Зарытый
    // козырёк arch=0 (top НИЖЕ th) станет кандидатом, но его перебьёт твёрдая
    // корка th — верх ниже неё не опускается.
    if (iv) for (const it of iv) {
        if (it.add && it.top < top && this.solidAt(x, it.top)) top = it.top;
    }
    // Полоса float: первое твёрдое вниз от её верхней границы (шум).
    const f = this.formationBlend(x);
    if (f.float) {
        for (let wy = th - FLOAT_SPAN; wy < th - FLOAT_GAP; wy += 1) {
            if (this.solidAt(x, wy)) { if (wy < top) top = wy; break; }
        }
    }
    // Корка поверхности: твёрдая, если её не вскрыла вычитающая форма
    // (Э5.3: void/crater). Вскрыта — верх базы опускается на низ вскрывающего
    // интервала (`bottom` включителен — точка ещё воздух), и уже оттуда ищем
    // твёрдое (тонкий срез <1 px иначе пропускался бы шагом скана, LOW @tester).
    let baseTop = th;
    if (iv) for (const it of iv) {
        if (!it.add && it.top <= baseTop + 1e-9 && it.bottom + 1e-9 > baseTop) baseTop = it.bottom + 1e-9;
    }
    if (this._baseSolidAt(x, baseTop, th)) {
        if (baseTop < top) top = baseTop;
    } else {
        const bottom = this.baseY - CHUNK_TOP_MARGIN + CHUNK_HEIGHT;
        for (let wy = baseTop; wy <= bottom; wy += 1) {
            if (this.solidAt(x, wy)) { if (wy < top) top = wy; break; }
        }
    }
    // Корка underIce — верх неба над водой (§3.2): погода ложится на лёд;
    // над полыньёй корки нет — верх падает к грунту/полу.
    const cr = this.crustTop(x);
    if (cr !== null && cr < top) top = cr;
    return top === Infinity ? th : top;
};

    // _groundFloorY — «пол/опора» БЕЗ ледовой корки (§2.1): верх твёрдого,
    // СВЯЗАННОГО с базой (землёй). Публичный `floorY` добавляет корку underIce
    // (ЧК6 §3.2): по ней ходят, её верх = опора; `bedY` (дно жидкости) — этот,
    // без корки (N1).
    // Висящие плиты (overhang arch=1) и полоса float в опору НЕ входят (арка — не
    // пол под игроком), а КРЕПЛЁННЫЕ к базе аддитивные формы (вал `crater`,
    // козырёк arch=0 в опорной колонке) поднимают опору (S4): интервал считаем
    // связанным, если его низ доходит до текущего верха твёрдого. Вычитающие
    // формы (`void`, впадина `crater`) опускают опору ниже своего низа.
    // КОНТРАКТ: `solidAt(floorY)=true` — вычитающие интервалы включительны, их
    // `bottom` ещё воздух, поэтому точку доводим до твёрдой. Для обычной колонки
    // floorY = terrainHeight. Якоря (корабль/фауна/спавн) — §5 п.12.
SurfaceWorld.prototype._groundFloorY = function(x) {
    const th = this.terrainHeight(x);
    if (!this.forms) return th;
    let y = th;
    const iv = this._collectFormIntervals(x, th);
    // Вскрывающие базу вычитающие формы (void/crater) опускают опору за свой
    // низ (интервал включительный: `bottom` — ещё воздух).
    for (const it of iv) {
        if (it.add) continue;
        if (it.top <= y + 1e-9 && it.bottom + 1e-9 > y) y = it.bottom + 1e-9;
    }
    // Креплённые к базе аддитивные формы (вал crater): поднимают опору.
    let changed = true;
    while (changed) {
        changed = false;
        for (const it of iv) {
            if (!it.add) continue;
            if (it.bottom + 1e-9 >= y && it.top < y - 1e-9) { y = it.top; changed = true; }
        }
    }
    if (this.solidAt(x, y)) return y;
    // Страховка (перекрытие вычитающих форм/пещеры): первая твёрдая точка вниз
    // тем же шагом и с той же дробной границы, что у `skyTop`, — иначе пол и
    // верх разъезжались бы на дробную часть (`skyTop > floorY`).
    const bottom = this.baseY - CHUNK_TOP_MARGIN + CHUNK_HEIGHT;
    for (let wy = y; wy <= bottom; wy += 1) if (this.solidAt(x, wy)) return wy;
    return y;
};
