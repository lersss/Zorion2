import { hash1, smoothstep, primRange } from './surface_world_noise.js';
import { PLAYER_W, PLAYER_H } from './surface_config.js';
import { SurfaceWorld } from './surface_world_core.js';
import { Player } from './surface_player.js';
    // ==================== 2D-ФОРМЫ (Э5.2/Э5.3, §3) ====================
    //
    // Вклад 2D-форм рецепта (relief.forms, §3.1) двух знаков: аддитивные несущие
    // (`overhang`: плита-арка `arch=1` / козырёк `arch=0`; вал `crater`) добавляют
    // материал, вычитающие (`void`, впадина `crater`) убирают его. Порядок поля —
    // §2.2: `solidAt = (baseSolid ∨ formsAdditive) ∧ ¬formsSubtractive`.
    // Инстансы размещаются детерминированно по регионам (как primSpike: соседние
    // регионы base−1…base+1, только hash1/mulberry32, Math.random запрещён, §5
    // п.2); формы статичны. `relief.scale` к формам НЕ применяется (абсолютные px).
    // Вычитающие формы проходят фильтр выходимости/защиты спавна (§3.6): форма,
    // из которой игрок не выходит (или чей footprint задевает спавн), НЕ
    // применяется — ловушки не остаётся.

    // _collectFormIntervals — интервалы ТВЁРДЫХ форм колонки x: [{add, top, bottom}, …].
    // thx — предвычисленный `terrainHeight(x)` (растр передаёт кэш; физика —
    // undefined, считается здесь). Один источник геометрии для растра
    // (`formSpans`) и физики (`formsAdditive`/`formsSubtractive`) — второй
    // реализации формы нет (§2.1). `crack2d` сюда НЕ входит: решением гейта
    // 2026-09-25 он переведён в косметику (viewOnly, `crackSpans`) — сквозная
    // трещина и тёмная расселина в 2D-виде неотличимы, физику не трогаем
    // (§5.1 п.5, дефект D1). `_onlyInstance` (probe выходимости, §3.6) — перебор
    // инстансов одной формы без фильтра; `_fi` — исходный индекс формы в рецепте
    // (probe кладёт форму в массив из одного элемента — индекс переносится явно).
SurfaceWorld.prototype._collectFormIntervals = function(x, thx) {
    const out = [];
    const forms = this.forms;
    if (!forms || !forms.length) return out;
    if (thx === undefined) thx = this.terrainHeight(x);
    const R = this.region;
    const regionBase = Math.floor(x / R);
    for (let fi = 0; fi < forms.length; fi++) {
        const f = forms[fi];
        const prim = f.prim;
        // Операторы поля: overhang/crater (аддитив) + void/crater-впадина
        // (вычитающие). crack2d — косметика (`crackSpans`), сюда не входит.
        if (prim !== 'overhang' && prim !== 'void' && prim !== 'crater') continue;
        // Индекс формы — её место в рецепте: он задаёт соль `sBase` и через неё
        // ВСЮ геометрию (`primRange`). Probe выходимости кладёт форму в массив
        // из одного элемента, поэтому исходный индекс переносится на форму
        // явно (`_fi`) — иначе симуляция шла бы по чужой геометрии выреза, а
        // боевое применение — по своей (баг ревью Э5.3, §3.6).
        const fiEff = (f._fi != null) ? f._fi : fi;
        const formSalt = Math.imul(fiEff + 1, 0x9e3779b1) >>> 0;
        const sBase = (this.seed ^ formSalt) >>> 0;
        if (f._onlyInstance) {
            this._formColumn(out, f, fiEff, sBase, f._onlyInstance, x, thx);
            continue;
        }
        const rawN = f.perRegion != null ? f.perRegion : 1;
        for (let rr = regionBase - 1; rr <= regionBase + 1; rr++) {
            const n = Math.max(0, Math.round(primRange(rawN, rr, (sBase ^ 0x0f01) >>> 0, 1)));
            for (let k = 0; k < n; k++) {
                const idx = rr * 131 + k;
                const c = rr * R + hash1(idx, (sBase ^ 0x0f02) >>> 0) * R;
                const inst = { rr, k, idx, c };
                if (prim !== 'overhang' && !this._formInstanceOk(fi, f, sBase, inst)) continue;
                this._formColumn(out, f, fi, sBase, inst, x, thx);
            }
        }
    }
    return out;
};

    // _formColumn — интервалы одной ИНСТАНСИ формы в колонке x. Общая геометрия
    // боевого поля и probe-симуляции выходимости (`_onlyInstance`).
SurfaceWorld.prototype._formColumn = function(out, f, fi, sBase, inst, x, thx) {
    const idx = inst.idx;
    if (f.prim === 'overhang') {
        const w = primRange(f.w, idx, (sBase ^ 0x0f03) >>> 0, 40);
        const h = primRange(f.h, idx, (sBase ^ 0x0f04) >>> 0, 20);
        const taper = Math.max(0, Math.min(0.95, primRange(f.taper, idx, (sBase ^ 0x0f05) >>> 0, 0)));
        if (f.arch === 1) {
            // Замкнутая арка/плита-свод: висит НАД землёй (§3.2, знак
            // исправлен в Э5.2, y растёт вниз): подошва = th(x) − opening,
            // верх = th(x) − opening − h_eff; `taper` утоньшает плиту к замку.
            if (w <= 0) return;
            const opening = primRange(f.opening, idx, (sBase ^ 0x0f06) >>> 0, 60);
            const dx = Math.abs(x - inst.c);
            if (dx > w) return;
            const u = dx / w;
            const hEff = h * (1 - taper * (1 - u));
            out.push({ add: true, top: thx - opening - hEff, bottom: thx - opening });
            return;
        }
        // Консоль/козырёк/шляпа: опорная колонка x₀ = c, подошва = th(x₀),
        // вынос `reach` в сторону `dir`, `taper` к краю (§3.2). Просвета-
        // параметра нет — он производен от рельефа под выносом.
        const reach = Math.max(1, primRange(f.reach, idx, (sBase ^ 0x0f07) >>> 0, 60));
        const dir = f.dir === -1 ? -1 : 1;
        const d = (x - inst.c) * dir;
        if (d < 0 || d > reach) return;
        // Кэш опорной высоты инстанса: `terrainHeight(c)` — дорогой fbm, а
        // столбцов под одним козырьком сотни; без кэша генерация чанка
        // взлетает в разы (§2.4, бюджет ≤2×).
        const ck = fi + ':' + inst.rr + ':' + inst.k;
        const support = this._formSupport || (this._formSupport = new Map());
        let th0 = support.get(ck);
        if (th0 === undefined) {
            th0 = this.terrainHeight(inst.c);
            if (support.size > 20000) support.clear();
            support.set(ck, th0);
        }
        const t = d / reach;
        const hEff = h * (1 - taper * t);
        out.push({ add: true, top: th0 - hEff, bottom: th0 });
        return;
    }
    if (f.prim === 'void') {
        this._voidColumn(out, f, sBase, inst, x, thx);
        return;
    }
    this._craterColumn(out, f, sBase, inst, x, thx);
};

    // _voidColumn — вычитающий `void` (Э5.3, §3.6): открытая ниша/чаша/трубка.
    // `w` — ПОЛУширина; профиль `1 − smoothstep(0,1,u)` даёт плоское дно и
    // плавные стенки (нулевой уклон у края) — выходит пешком. `depth` — в породу;
    // `h` (свободная высота выреза) — явный вертикальный размер отдельно стоящей
    // полости. В дельтах Э5.3 (в т.ч. у труб `tube`) `h` НЕ задаётся — глубина
    // берётся из `depth`; заданный `h` лишь клампит срез сверху.
SurfaceWorld.prototype._voidColumn = function(out, f, sBase, inst, x, thx) {
    const idx = inst.idx;
    const w = primRange(f.w, idx, (sBase ^ 0x0f11) >>> 0, 60);
    if (w <= 0) return;
    const dx = Math.abs(x - inst.c);
    if (dx >= w) return;
    const depth = primRange(f.depth, idx, (sBase ^ 0x0f12) >>> 0, 40);
    const u = dx / w;
    let cut = depth * (1 - smoothstep(0, 1, u));
    if (f.h != null) {
        const hFree = primRange(f.h, idx, (sBase ^ 0x0f13) >>> 0, depth);
        if (hFree < cut) cut = hFree;
    }
    if (cut <= 0) return;
    out.push({ add: false, top: thx, bottom: thx + cut });
};

    // _craterColumn — `crater` (Э5.3, M9): одна запись даёт вал (аддитив) и
    // впадину (вычитающая). Фактическую геометрию чаши задают `depth` + `wallDeg`
    // (заложение = depth/tan(wallDeg), фактический уклон = wallDeg); `d` — полная
    // ширина кольца, клампится вверх до `2·(rim + depth/tan(wallDeg))`, чтобы
    // объявленный уклон не расходился с фактическим. `talus` (осыпь) сглаживает
    // стенку (линейный профиль → smoothstep у основания). Вал — кольцо
    // [rB, dEff/2] высотой `rim` у кромки чаши, сходящее на нет наружу.
    // Дельта биома `кратеры` использует смягчённый `wallDeg [26,34]` вместо
    // научного ориентира 30–40° (§3.2): мягче стенка = вернее выходимость
    // пешком (§3.6); «объявленный уклон = фактический» сохраняется (диапазон —
    // ориентир, не жёсткая граница). D3 @tester — расхождение с §3.2/§A.2, на
    // согласование @designer.
    // Дельта также несёт `relief.caves: 0.3` (пресет `скальные пустоши` — 0.7):
    // D2 @tester, в §3.6 не заявлено; осознанное смягчение пещерности под кольцевой
    // вал (видимость/выходимость), внесение в спеку — за @designer. НЕ удалять без
    // решения гейта.
SurfaceWorld.prototype._craterColumn = function(out, f, sBase, inst, x, thx) {
    const idx = inst.idx;
    const depth = primRange(f.depth, idx, (sBase ^ 0x0f22) >>> 0, 60);
    if (depth <= 0) return;
    const rim = primRange(f.rim, idx, (sBase ^ 0x0f23) >>> 0, 12);
    const wallDeg = Math.max(1, Math.min(89, primRange(f.wallDeg, idx, (sBase ^ 0x0f24) >>> 0, 35)));
    const talus = Math.max(0, Math.min(1, primRange(f.talus, idx, (sBase ^ 0x0f25) >>> 0, 0)));
    const wallRun = depth / Math.tan(wallDeg * Math.PI / 180);
    const dRaw = primRange(f.d, idx, (sBase ^ 0x0f21) >>> 0, 400);
    const dEff = Math.max(dRaw, 2 * (rim + wallRun));
    const half = dEff / 2;
    const dx = Math.abs(x - inst.c);
    if (dx > half) return;
    const rB = half - rim;                     // кромка чаши = основание вала
    const rFloor = Math.max(0, rB - wallRun);  // плоское дно чаши
    if (dx <= rB + 1e-9) {
        let t = rFloor >= rB - 1e-6 ? 1 : (rB - dx) / (rB - rFloor);
        if (t < 0) t = 0; else if (t > 1) t = 1;
        // `talus` (осыпь) сглаживает стенку, но НЕ круче `wallDeg`: берём min
        // линейного профиля и smoothstep (≤ линейного) — фактический уклон не
        // превышает объявленный, чаша по-прежнему укладывается в `d`.
        const wall = talus > 0 ? t * (1 - talus) + Math.min(t, smoothstep(0, 1, t)) * talus : t;
        const cut = depth * wall;
        if (cut > 0) out.push({ add: false, top: thx, bottom: thx + cut });
    }
    if (rim > 0 && dx >= rB - 1e-9) {
        const band = Math.max(1e-6, half - rB);
        const s = Math.min(1, Math.max(0, (dx - rB) / band));
        const raise = rim * (1 - smoothstep(0, 1, s));
        if (raise > 0) out.push({ add: true, top: thx - raise, bottom: thx });
    }
};

    // _formFootprint — горизонтальный полуразмер формы от центра инстанса
    // (void: `w`; crater: `dEff/2`). Для защиты спавна (§3.6) и стартов симуляции.
SurfaceWorld.prototype._formFootprint = function(f, sBase, inst) {
    const idx = inst.idx;
    if (f.prim === 'void') return Math.max(0, primRange(f.w, idx, (sBase ^ 0x0f11) >>> 0, 60));
    if (f.prim === 'crater') {
        const depth = primRange(f.depth, idx, (sBase ^ 0x0f22) >>> 0, 60);
        const rim = primRange(f.rim, idx, (sBase ^ 0x0f23) >>> 0, 12);
        const wallDeg = Math.max(1, Math.min(89, primRange(f.wallDeg, idx, (sBase ^ 0x0f24) >>> 0, 35)));
        const wallRun = depth / Math.tan(wallDeg * Math.PI / 180);
        const dRaw = primRange(f.d, idx, (sBase ^ 0x0f21) >>> 0, 400);
        return Math.max(dRaw, 2 * (rim + wallRun)) / 2;
    }
    return 0;
};

    // _formInstanceOk — можно ли применять инстанс вычитающей формы (§3.6). Две
    // ступени (дефект D1 прогона @tester Э5.3): (1) ПООДИНОЧНАЯ — защита спавна
    // (footprint не пересекает `x_spawn ± 2·PLAYER_W`; позиционного исключения у
    // `where` нет — правило размещения в коде), сетка узкого прохода и симуляция
    // выходимости на поле с ОДНОЙ этой формой; (2) ПОЛНОЕ ПОЛЕ ОКРЕСТНОСТИ — та же
    // симуляция, но на поле §2.1 (база с пещерами ⊕ формы рецепта): одиночный
    // прогон не видел ловушки, которую даёт СОЧЕТАНИЕ вычитающих форм (две каверны
    // в сумме запирают игрока — e2e-W7: спуск 81.8 px при максимуме одиночной
    // `void grotto` 60). Отказ любой ступени → форма НЕ применяется (откат к базе),
    // ловушки не остаётся. Мемо на мир.
SurfaceWorld.prototype._formInstanceOk = function(fi, f, sBase, inst) {
    if (this._skipEscape) return true;
    // Полевой probe (ступень 2): прочие формы решаются только ПООДИНОЧНОЙ
    // ступенью — иначе взаимная рекурсия (форма A проверяла бы поле с B, B — с
    // A). Откат-ловушек не теряется: форма, не прошедшая поодиночную ступень, в
    // поле не входит.
    if (this._stage2Active) return this._formStage1Ok(fi, f, sBase, inst);
    const key = fi + ':' + inst.rr + ':' + inst.k;
    const cache = this._formOk || (this._formOk = new Map());
    if (cache.has(key)) return cache.get(key);
    let ok = this._formStage1Ok(fi, f, sBase, inst);
    // Ступень 2 — только если рядом (в пределах досягаемости игрока) есть ДРУГОЙ
    // инстанс вычитающей формы: тогда поле окрестности может отличаться от
    // одиночного. Нет соседа — поля совпадают, повторная симуляция не нужна
    // (цена ≤2× baseline, §5 п.7).
    if (ok && this._hasSubtractiveNeighbor(fi, inst, this._formFootprint(f, sBase, inst))) {
        // Тот же ПОЛНЫЙ пучок стартов, что в ступени 1 (включая фланцы ±0.45):
        // иначе наихудший фланец в полном поле не проверялся бы (D4).
        ok = this._simulateEscape(f, fi, sBase, inst, this._fullFieldProbe());
    }
    if (cache.size > 40000) cache.clear();
    cache.set(key, ok);
    return ok;
};

    // _hasSubtractiveNeighbor — есть ли ДРУГОЙ инстанс вычитающей формы (`void`/
    // `crater`), чей вырез СОПРИКАСАЕТСЯ с вырезом этой формы (расстояние центров
    // ≤ сумма полуразмеров). Только тогда два выреза складываются в общую ловушку;
    // разреженные каверны друг на друга не влияют (соседнюю игрок покидает так же,
    // как её собственную — ступень 1 её уже проверила). Перебор дешёвый (без
    // симуляции) — гейт платной ступени 2 (§5 п.7, ≤2×).
SurfaceWorld.prototype._hasSubtractiveNeighbor = function(fi, inst, fp) {
    const forms = this.forms;
    if (!forms || !forms.length) return false;
    const R = this.region;
    const regionBase = Math.floor(inst.c / R);
    const reach = fp;
    for (let fj = 0; fj < forms.length; fj++) {
        const g = forms[fj];
        if (g.prim !== 'void' && g.prim !== 'crater') continue;
        const sB = (this.seed ^ Math.imul(fj + 1, 0x9e3779b1)) >>> 0;
        const rawN = g.perRegion != null ? g.perRegion : 1;
        for (let rr = regionBase - 1; rr <= regionBase + 1; rr++) {
            const n = Math.max(0, Math.round(primRange(rawN, rr, (sB ^ 0x0f01) >>> 0, 1)));
            for (let k = 0; k < n; k++) {
                if (fj === fi && rr === inst.rr && k === inst.k) continue;
                const idx = rr * 131 + k;
                const c = rr * R + hash1(idx, (sB ^ 0x0f02) >>> 0) * R;
                const gfp = this._formFootprint(g, sB, { rr, k, idx, c });
                if (Math.abs(c - inst.c) <= reach + gfp) return true;
            }
        }
    }
    return false;
};

    // _formStage1Ok — первая ступень (§3.6): защита спавна + сетка узкого прохода +
    // выходимость на поле с ОДНОЙ этой формой. Мемо отдельно от `_formOk`: значение
    // ступени НЕ зависит от контекста, поэтому полевой probe (ступень 2)
    // переиспользует его как «форму вообще можно разместить».
SurfaceWorld.prototype._formStage1Ok = function(fi, f, sBase, inst) {
    const key = fi + ':' + inst.rr + ':' + inst.k;
    const cache = this._formStage1 || (this._formStage1 = new Map());
    if (cache.has(key)) return cache.get(key);
    let ok = true;
    const fp = this._formFootprint(f, sBase, inst);
    const spawnHalf = 2 * PLAYER_W;
    if (inst.c + fp >= -spawnHalf && inst.c - fp <= spawnHalf) ok = false;
    // Сетка безопасности для ЗАМКНУТОЙ полости (§3.6): проход `open` обязан
    // быть не уже PLAYER_W+2 и не ниже PLAYER_H+2. Открытые чаши проверяет
    // симуляция (вертикального потолка у них нет).
    if (ok && f.prim === 'void' && f.open !== true) {
        const w = primRange(f.w, inst.idx, (sBase ^ 0x0f11) >>> 0, 60);
        const hFree = f.h != null ? primRange(f.h, inst.idx, (sBase ^ 0x0f13) >>> 0, 0) : Infinity;
        if (2 * w < PLAYER_W + 2 || hFree < PLAYER_H + 2) ok = false;
    }
    if (ok) ok = this._simulateEscape(f, fi, sBase, inst);
    if (cache.size > 40000) cache.clear();
    cache.set(key, ok);
    return ok;
};

    // _fullFieldProbe — поле ОКРЕСТНОСТИ для ступени 2 (§2.1): все формы рецепта
    // (аддитивные — всегда; вычитающие — только прошедшие поодиночную ступень),
    // база с пещерами и полосой float. `_stage2Active` переключает `_formInstanceOk`
    // на поодиночную ступень для прочих форм — взаимной рекурсии нет.
SurfaceWorld.prototype._fullFieldProbe = function() {
    const probe = Object.create(this);
    probe._stage2Active = true;
    // Кэши — ЛОКАЛЬНЫЕ (объект-прототип не должен писать в мемо мира): probe
    // подменяет `terrainHeight` сеткой, а `liquidAt`/`liquidLevel` физики (ЧК6.2)
    // пишут в `_liqColMemo`/`_basinMemo`; общий мемо получал бы значения чужой
    // сетки (дефект порядка запросов колонок, `_LIQUID` недетерминирован).
    probe._formSupport = null;
    probe._liqColMemo = null;
    probe._basinMemo = null;
    return probe;
};

    // _probeFor — probe-мир выходимости: ТОЛЬКО один инстанс формы, без фильтра
    // (`_skipEscape`). Исходный индекс формы (`_fi`) переносится явно, чтобы соль
    // `sBase` и вся геометрия (`primRange`) совпали с боевым применением (§3.6):
    // при сдвиге формы на индекс 0 вырез считался бы по чужой геометрии (баг ревью).
    // Единый конструктор probe — его же используют проверки T12/T13/S6, поэтому
    // они мерят боевую геометрию, а не копию со сдвигом.
SurfaceWorld.prototype._probeFor = function(fi, f, inst) {
    const probe = Object.create(this);
    probe.forms = [Object.assign({}, f, { _onlyInstance: inst, _fi: fi })];
    probe._skipEscape = true;
    // Кэши — локальные (см. `_fullFieldProbe`): probe не пишет в мемо мира.
    probe._formSupport = null;
    probe._liqColMemo = null;
    probe._basinMemo = null;
    return probe;
};

    // _simulateEscape — выходимость инстанса (§3.6): probe-мир с ТОЛЬКО этим
    // инстансом (плюс прочие формы рецепта, `_skipEscape`), реальная физика Player
    // (`surface_player.js`) — до 10 с/60 Гц из наихудших точек (дно и КРАЯ выреза,
    // пучок стартов по всей ширине footprint) и набора стратегий (в обе стороны, с
    // прыжком и без). Выход — достиг ПОЛКИ ВНЕ выреза этой формы (`cutAt ≤ 1.5`):
    // приземление на дно собственного выреза выходом не считается — иначе ловушка
    // у фланга (форма на крутом склоне/игле) проходила бы проверку от центра.
    // Домен ограничен окрестностью формы (движение уводит от центра).
    //
    // Цена (§5 п.7, ≤2×): профиль probe — кусочно-постоянная сетка 1 px (мемо
    // `terrainHeight`): без неё каждый `solidAt` inner-цикла заново считал бы fbm
    // (профиль дорог, а поле твёрдости — часть кадра генерации чанка). Провал ниже
    // ФОРМЫ (пол выреза) и застой по x обрывают стратегию досрочно — типичный отказ
    // (каверна под чашей) виден за единицы шагов, а не за полные 10 с.
SurfaceWorld.prototype._simulateEscape = function(f, fi, sBase, inst, field, fracs) {
    // Поле симуляции: по умолчанию (ступень 1) — probe ТОЛЬКО с испытуемой
    // формой (`_onlyInstance`): выход проверяется из выреза ЭТОЙ формы; отказ
    // чужой формы не отвергает эту (нет цепной реакции). Ступень 2 передаёт
    // полное поле окрестности (`_fullFieldProbe`) — сочетание форм ловится там.
    // Набор стартов ОБЕИХ ступеней — полный пучок (дно + фланцы ±0.45/±0.9).
    // `_probeFor` сохраняет исходный индекс формы — геометрия probe = боевой.
    const probe = field || this._probeFor(fi, f, inst);
    const fp = this._formFootprint(f, sBase, inst);
    // Профиль probe — ЛИНЕЙНАЯ ИНТЕРПОЛЯЦИЯ между целыми колонками (один fbm
    // на колонку — та же цена, что у прежнего округления, но без «полочек»):
    // округление делало профиль кусочно-постоянным, и на крутом склоне игрок
    // вставал на 1-px ступеньку, которой в РЕАЛЬНОМ поле нет, — фильтр ЛОЖНО
    // пропускал форму. Дефект D4: `пещерный_мир_с_потолком` seed 987654,
    // `void` c=143.8, старт frac=0.45 — настоящее (непрерывное) поле не
    // выпускало НИ ОДНОЙ стратегией (STUCK/STUCK/STUCK/DOMAIN), а округлённое
    // «выходило» влево по ступеньке. Интерполяция воспроизводит непрерывный
    // профиль при той же цене (§5 п.7).
    const realTH = this.terrainHeight.bind(this);
    // Плоский массив по домену симуляции вместо Map: `terrainHeight` зовётся
    // ~10 раз за шаг физики, и Map-индексация дороже массива. Домен — старты
    // внутри footprint ± домен обрыва `fp·4`; за его границей считаем напрямую
    // (без кэша) — значения те же, интерполяция та же.
    const thBase = Math.floor(inst.c - fp * 6);
    const thSpan = Math.ceil(fp * 12) + 8;
    const thArr = new Array(thSpan).fill(null);
    const thAt = (k) => {
        const i = k - thBase;
        if (i < 0 || i >= thSpan) return realTH(k);
        let v = thArr[i];
        if (v === null) { v = realTH(k); thArr[i] = v; }
        return v;
    };
    probe.terrainHeight = (x) => {
        const f = Math.floor(x), t = x - f;
        return t === 0 ? thAt(f) : thAt(f) + (thAt(f + 1) - thAt(f)) * t;
    };
    // Кэш интервалов форм по ТОЧНОЙ x (не округление — геометрия та же): за кадр
    // физики `solidAt` спрашивает одни и те же x по 2–4 раза (углы бокса), а на
    // полном поле (ступень 2) `_collectFormIntervals` перебирает все формы —
    // это и есть основная цена проверки (§5 п.7, ≤2×).
    const ivMemo = new Map();
    const realCollect = SurfaceWorld.prototype._collectFormIntervals;
    probe._collectFormIntervals = (x, thx) => {
        let v = ivMemo.get(x);
        if (v === undefined) {
            v = realCollect.call(probe, x, thx !== undefined ? thx : probe.terrainHeight(x));
            ivMemo.set(x, v);
        }
        return v;
    };
    // cutAt — глубина выреза ФОРМЫ в колонке (без пещер): быстрый признак
    // «провалился ниже формы» (каверна/провал).
    const scratch = [];
    const cutMemo = new Map();
    const cutAt = (x) => {
        const k = Math.round(x);
        let c = cutMemo.get(k);
        if (c === undefined) {
            scratch.length = 0;
            this._formColumn(scratch, f, fi, sBase, inst, k, probe.terrainHeight(k));
            c = 0;
            for (const it of scratch) if (!it.add) c = Math.max(c, it.bottom - it.top);
            cutMemo.set(k, c);
        }
        return c;
    };
    // Старты — наихудшие точки выреза (§3.6 п.2): дно (центр) и оба дальних
    // угла с серединами флангов. Один центр недостаточен — форма на крутом
    // склоне/игле даёт ловушку у края, а от центра игрок выходит. Стартуем
    // только там, где есть вырез (`cutAt > 1`).
    const starts = [];
    for (const frac of (fracs || [-0.9, -0.45, 0, 0.45, 0.9])) {
        const sx = inst.c + frac * fp;
        if (cutAt(sx) > 1) starts.push(sx);
    }
    if (!starts.length) starts.push(inst.c);
    const strategies = [[1, false], [-1, false], [1, true], [-1, true]];
    // Критерий §3.6: выйти обязана КАЖДАЯ стартовая точка (дно и фланги) —
    // иначе ловушка у фланга прошла бы от центра. Провал любой = форма не
    // применяется (откат к базе).
    for (const sx of starts) {
        let escaped = false;
        for (const [dir, jump] of strategies) {
            const p = new Player(probe, 1);
            p.x = sx;
            p.vx = 0;
            p.vy = 0;
            p.y = probe.floorY(sx) - p.h / 2 - 2;
            const startX = p.x;
            let stuck = 0;
            const input = { left: dir < 0, right: dir > 0, jump, sprint: false };
            for (let step = 0; step < 600; step++) {
                const prevX = p.x;
                p.update(1 / 60, input);
                // Выход — твёрдая опора на полу ВНЕ выреза формы (cutAt ≤ 1.5):
                // приземление на собственное дно/стенку вырезом не считается.
                if (p.onGround && p.y <= probe.terrainHeight(p.x) + 2.5
                    && cutAt(p.x) <= 1.5) { escaped = true; break; }
                // провал ниже формы (каверна под вырезом) — стратегия тупиковая
                if (p.y > probe.terrainHeight(p.x) + cutAt(p.x) + 12) break;
                // нет горизонтального прогресса — стенка/застой
                stuck = Math.abs(p.x - prevX) < 0.05 ? stuck + 1 : 0;
                if (stuck > 150) break;
                if (Math.abs(p.x - startX) > fp * 4) break; // вышли далеко — домен закрыт
            }
            if (escaped) break;
        }
        if (!escaped) return false;
    }
    return true;
};

    // formSpans — интервалы форм колонки для растра (§2.3): {add, sub} или null
    // (столбец формой не задет). Единственный источник геометрии форм.
SurfaceWorld.prototype.formSpans = function(x, thx) {
    const iv = this._collectFormIntervals(x, thx);
    if (!iv.length) return null;
    const add = [], sub = [];
    for (const it of iv) (it.add ? add : sub).push({ top: it.top, bottom: it.bottom });
    return { add, sub };
};

    // crackSpans — КОСМЕТИЧЕСКИЕ интервалы трещин crack2d колонки (решение гейта
    // 2026-09-25): тёмный клин поверх рельефа, ВНЕ равенства по твёрдому телу
    // (§2.1) — физику не трогает (дефект D1: наклонная трещина запирала игрока,
    // §5.1 п.5). Геометрия та же, что раньше вычитала твёрдое (`w`/`depth`/`taper`/
    // `tilt`, детерминированно от seed); соль инстанса берётся от индекса формы
    // `fi`, как в `_collectFormIntervals`, — положение трещин сохранено.
SurfaceWorld.prototype.crackSpans = function(x, thx) {
    const iv = this._collectCrackIntervals(x, thx);
    return iv.length ? iv : null;
};

SurfaceWorld.prototype._collectCrackIntervals = function(x, thx) {
    const out = [];
    const forms = this.forms;
    if (!forms || !forms.length) return out;
    if (thx === undefined) thx = this.terrainHeight(x);
    const R = this.region;
    const regionBase = Math.floor(x / R);
    for (let fi = 0; fi < forms.length; fi++) {
        const f = forms[fi];
        if (f.prim !== 'crack2d') continue;
        const formSalt = Math.imul(fi + 1, 0x9e3779b1) >>> 0;
        const sBase = (this.seed ^ formSalt) >>> 0;
        const rawN = f.perRegion != null ? f.perRegion : 1;
        for (let rr = regionBase - 1; rr <= regionBase + 1; rr++) {
            const n = Math.max(0, Math.round(primRange(rawN, rr, (sBase ^ 0x0f01) >>> 0, 1)));
            for (let k = 0; k < n; k++) {
                const idx = rr * 131 + k;
                const c = rr * R + hash1(idx, (sBase ^ 0x0f02) >>> 0) * R;
                // Клин шире вверху, сужение `taper`, наклон `tilt`. Полная ширина
                // `w` (без прежнего клампа PLAYER_W−2, ставшего ненужным): форма
                // косметична, игрока не заглатывает.
                const wFull = primRange(f.w, idx, (sBase ^ 0x0f08) >>> 0, 10);
                const depth = Math.max(1, primRange(f.depth, idx, (sBase ^ 0x0f09) >>> 0, 100));
                const taper = Math.max(0, Math.min(0.95, primRange(f.taper, idx, (sBase ^ 0x0f0a) >>> 0, 0.5)));
                const tiltDeg = primRange(f.tilt, idx, (sBase ^ 0x0f0b) >>> 0, 0);
                const tiltSign = hash1(idx, (sBase ^ 0x0f0c) >>> 0) < 0.5 ? -1 : 1;
                const a = wFull * 0.5;
                const b = a * taper / depth;
                const kk = tiltSign * Math.tan(tiltDeg * Math.PI / 180);
                const denom = kk - b;
                let dyLo, dyHi;
                if (Math.abs(denom) < 1e-9) {
                    // Вертикальные стенки (taper≈0, tilt≈0): ширина постоянна.
                    if (Math.abs(x - c) >= a) continue;
                    dyLo = 0;
                    dyHi = depth;
                } else {
                    // Границы клина в глубину (линейны): |x − c − k·dy| < a − b·dy.
                    const d1 = (x - c + a) / denom;
                    const d2 = (x - c - a) / denom;
                    dyLo = Math.min(d1, d2);
                    dyHi = Math.max(d1, d2);
                }
                if (dyHi < 0) continue;
                if (dyLo < 0) dyLo = 0;
                if (dyHi > depth) dyHi = depth;
                if (dyHi <= dyLo) continue;
                out.push({ top: thx + dyLo, bottom: thx + dyHi });
            }
        }
    }
    return out;
};

    // formsSolid — аддитивный вклад 2D-форм (прежнее имя контракта §2.1/§3.1).
SurfaceWorld.prototype.formsSolid = function(x, y) {
    return this.formsAdditive(x, y);
};

    // formsAdditive/formsSubtractive — точечная проба знака формы (физика).
SurfaceWorld.prototype.formsAdditive = function(x, y) {
    if (!this.forms) return false;
    const iv = this._collectFormIntervals(x, undefined);
    for (const it of iv) if (it.add && y >= it.top && y <= it.bottom) return true;
    return false;
};

SurfaceWorld.prototype.formsSubtractive = function(x, y) {
    if (!this.forms) return false;
    const iv = this._collectFormIntervals(x, undefined);
    for (const it of iv) if (!it.add && y >= it.top && y <= it.bottom) return true;
    return false;
};
