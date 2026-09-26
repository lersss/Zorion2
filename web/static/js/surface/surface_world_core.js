import { num } from './surface_world_noise.js';
import { FORMATIONS } from './surface_config.js';
import { resolvePalette } from './surface_world_color.js';
import { VIEW_SCHEMA_VERSION, HORIZON_MAX_LAYERS, LIFE_DENSITY, LIVING_DECOR, resolveLiquid, keySeed } from './surface_world_recipe.js';

export class SurfaceWorld {
    constructor(pkg) {
        this.seed = (pkg.seed | 0) >>> 0;
        this.category = pkg.biome_category || 'литосфера';
        this.life = !!pkg.life;
        this.biome = pkg.biome;
        this.color = pkg.biome_color || '#8a7a6a';
        this.baseY = 300;
        this.region = 1400;
        this.formations = FORMATIONS[this.category] || FORMATIONS['литосфера'];
        // Рецепт вида (спека 2026-09-23 §2.6): применяется только когда сервер
        // отдал резолвленный рецепт (view_source === 'catalog') И версия схемы
        // знакома клиенту (view_version === VIEW_SCHEMA_VERSION, §2.6 —
        // «клиент умеет игнорировать незнакомую версию»). Иначе — фолбэк 1:1 на
        // FORMATIONS/LIFE_DENSITY (старые пакеты/биомы без рецепта/новая схема).
        this.view = (pkg.biome_view && pkg.view_source === 'catalog'
            && pkg.view_version === VIEW_SCHEMA_VERSION) ? pkg.biome_view : null;
        this.hasView = !!this.view;
        // _viewKey — идентичность рецепта для сброса кэша чанков (§2.5): версия
        // схемы + хеш relief. Правка рецепта (hot-reload) меняет ключ — старый
        // растр не переиспользуется. Рецепт — константа мира (в сессии не меняется).
        const relKey = (this.view && this.view.relief) || null;
        this._viewKey = (pkg.view_version || 0) + ':' + keySeed(relKey ? JSON.stringify(relKey) : '');
        this.palette = resolvePalette(this.view, this.color);
        this.placement = (this.view && this.view.placement) || null;
        const rawDecor = (this.view && Array.isArray(this.view.decor)) ? this.view.decor : [];
        // Живой декор гейтится по pkg.life (§3.2): нет жизни → нет живого декора.
        this.decorList = this.life ? rawDecor : rawDecor.filter((d) => !LIVING_DECOR.has(d.prim));
        this.hangList = (this.view && Array.isArray(this.view.hang)) ? this.view.hang : [];
        this._hasWhere = this.decorList.some((d) => d.where);
        this.lifeDensity = this.view
            ? (typeof this.view.life_density === 'number' ? this.view.life_density : 0)
            : (this.life ? (LIFE_DENSITY[this.category] || 0.08) : 0);
        // Ярусный горизонт (§3.6): 2 пояса из рецепта; нет рецепта — ярусов нет
        // (фолбэк 1:1). Потолок 2 (§6 п.8) — лишние пояса игнорируются.
        const hz = this.view && this.view.horizon;
        this.horizon = (hz && Array.isArray(hz.layers)) ? hz.layers.slice(0, HORIZON_MAX_LAYERS) : null;
        // Профиль формы (§3.1, Э3): базовые скаляры рецепта (ridge/flatten/offset/
        // caves/float/base) + стек слоёв; `scale` — общий множитель амплитуд.
        const rel = (this.view && this.view.relief) || null;
        this.relief = rel;
        // 2D-формы (relief.forms, спека Э5 §3.1): список операторов relief2d
        // (Э5.2 — overhang/crack2d). Нет рецепта/списка — форм нет (фолбэк 1:1).
        this.forms = (rel && Array.isArray(rel.forms) && rel.forms.length) ? rel.forms : null;
        this.reliefScale = rel ? Math.max(0, num(rel.scale, 1)) : 1;
        const layers = (rel && Array.isArray(rel.layers)) ? rel.layers : null;
        // viewOnly-слои — только отрисовка (viewHeight), физика их не знает (§3.1).
        this.physLayers = layers ? layers.filter((l) => !l.viewOnly) : null;
        this.viewLayers = layers ? layers.filter((l) => !!l.viewOnly) : null;
        // Снеговая линия (§4.3): px ниже ЛОКАЛЬНОГО максимума профиля; абсолютной
        // шкалы высот в прогулке нет — линия относительная (осознанное отклонение).
        this.snowLine = (this.view && typeof this.view.snowLine === 'number') ? this.view.snowLine : null;
        this.snowLineShadow = num(this.view && this.view.snowLineShadow, 0.85);
        // Слой жидкости (ЧК6 §3): резолвленный рецепт из пакета (топ-уровневое
        // `liquid`, иначе — `liquid` внутри biome_view). null — жидкости нет
        // (сухой биом/старый пакет → прежний вид 1:1). Мир статичен: мемо колонок.
        this.liquid = resolveLiquid(pkg, this.view);
        this._liquidCrust = !!this.liquid && this.liquid.level.mode === 'underIce';
        this._liqColMemo = null;
        this._basinMemo = null;
        this._polyMemo = null;
        // Стартовая колонка (ЧК6.2 §6.2) не считается здесь: расчёт дорог (форма,
        // fbm) и не должен попадать в стоимость генерации чанка. `spawnX`/`spawnY`
        // — ленивые геттеры (мемо); probe-миры выходят из `ensureSpawn` дёшево.
        this._spawnReady = false;
    }
}
