import { hash1, mulberry32 } from './surface_world_noise.js';
import { CHUNK } from './surface_config.js';
import { decorAllowed, resolveRange, keySeed } from './surface_world_recipe.js';
import { SurfaceWorld } from './surface_world_core.js';
    // decorAt — декор колонки: по рецепту вида (если есть) или легаси-фолбэк.
SurfaceWorld.prototype.decorAt = function(x) {
    if (this.hasView) return this._viewDecorAt(x);
    return this._legacyDecorAt(x);
};

    // _viewDecorAt — декор по рецепту (§3.2/§3.3): правила размещения
    // uniform/clustered, плотность life_density (доля кластерных колонок),
    // фильтр `where` по высоте/склону, выбор примитива взвешенно по p. Всё
    // детерминировано от seed.
SurfaceWorld.prototype._viewDecorAt = function(x) {
    if (!this.decorList.length) return null;
    const col = Math.floor(x);
    const p = this.placement || {};
    if (p.mode === 'clustered') {
        const tile = p.tile || 256;
        const gap = p.gap || 16;
        const t = Math.floor(col / tile);
        const span = Math.max(1, tile - gap);
        const start = t * tile + Math.floor(hash1(t, this.seed ^ 0xc105) * gap);
        if (col < start || col >= start + span) return null; // зазор между кучками
    }
    if (hash1(col, this.seed ^ 0xdec0) >= this.lifeDensity) return null;
    let list = this.decorList;
    if (this._hasWhere) {
        const alt = this.baseY - this.terrainHeight(col);
        const slope = this.terrainHeight(col + 1) - this.terrainHeight(col);
        list = list.filter((d) => decorAllowed(d.where, alt, slope));
        if (!list.length) return null;
    }
    let total = 0;
    for (const d of list) total += (d.p || 0);
    if (total <= 0) return null;
    let u = hash1(col, this.seed ^ 0xd3c1) * total;
    let pick = list[list.length - 1];
    for (const d of list) {
        u -= (d.p || 0);
        if (u < 0) { pick = d; break; }
    }
    // Форвардим ВСЕ параметры примитива (buttress/crown/arms/fronds/blades/
    // capR/plume/…); диапазоны [lo,hi] разворачиваем в число детерминированно
    // от seed (§3.2) — иначе `for (i < [2,3])` не рисует крону/вайи.
    const out = { ...pick, col };
    for (const key of Object.keys(out)) {
        if (key === 'h' || key === 'w') continue;
        const v = out[key];
        if (Array.isArray(v) && v.length === 2 && typeof v[0] === 'number' && typeof v[1] === 'number') {
            out[key] = resolveRange(v, col, (this.seed ^ keySeed(key)) >>> 0, v[0]);
        }
    }
    out.h = resolveRange(pick.h, col, this.seed ^ 0x4848, 24);
    out.w = resolveRange(pick.w, col, this.seed ^ 0x5757, out.h);
    // Лианы (hang, §3.2): привязаны к дереву — только к нему и только
    // детерминированно по колонке.
    if (pick.prim === 'tree' && this.hangList.length) {
        for (const hg of this.hangList) {
            if (hg.from && hg.from !== 'tree') continue;
            if (hash1(col, this.seed ^ 0x11a5) < (hg.p || 0)) {
                out.hang = { prim: hg.prim, len: resolveRange(hg.len, col, this.seed ^ 0x6c6c, 30) };
                break;
            }
        }
    }
    return out;
};

    // _legacyDecorAt — прежний декор по категории (фолбэк 1:1, §2.3/§6 п.10).
SurfaceWorld.prototype._legacyDecorAt = function(x) {
    const col = Math.floor(x);
    if (this.life) {
        const r = hash1(col, this.seed ^ 0xdec0);
        if (r < this.lifeDensity * 0.25) {
            return { kind: 'tree', h: 40 + hash1(col, 0x71) * 60 };
        }
        if (r < this.lifeDensity) {
            return { kind: 'plant', h: 8 + hash1(col, 0x72) * 26 };
        }
        return null;
    }
    // Стерильный мир: редкие споры/лишайники/камни — «одиночество/масштаб».
    const r = hash1(col, this.seed ^ 0xd0d0);
    if (r < 0.02) return { kind: 'lichen', h: 4 + hash1(col, 0x73) * 6 };
    if (r < 0.03) return { kind: 'rock', h: 6 + hash1(col, 0x74) * 14 };
    return null;
};

    // rareDecorAt — редкая декорация-находка (любопытство, §9/§3.3): ≥1 на
    // участок landmark.perRegion (по умолчанию 6000 px); вид — landmark.prim
    // рецепта (например, сухая котловина), иначе легаси-набор; детерминирован
    // участком.
SurfaceWorld.prototype.rareDecorAt = function(x) {
    const lm = (this.view && this.view.landmark) || null;
    const R = (lm && lm.perRegion) || 6000;
    const i = Math.floor(x / R);
    const cx = i * R + R * 0.5;
    if (Math.abs(x - cx) > 30) return null;
    if (lm && lm.prim) return { prim: lm.prim, x: cx };
    const kinds = ['окаменелость', 'кристалл', 'обломок'];
    const kind = kinds[Math.floor(hash1(i, this.seed ^ 0xbeef) * kinds.length) % kinds.length];
    return { kind, x: cx };
};

    // creaturesFor — животные чанка (не бой, §7.3): 2–3 поведения. Якорь —
    // `floorY` (§5 п.12): в гроте/под сводом фауна стоит на полу, не на потолке.
SurfaceWorld.prototype.creaturesFor = function(chunkIndex) {
    if (!this.life || this.lifeDensity < 0.1) return [];
    const rng = mulberry32((this.seed ^ Math.imul(chunkIndex, 0x9e3779b1)) >>> 0);
    const count = Math.floor(rng() * 3 * this.lifeDensity + this.lifeDensity);
    const behaviors = ['graze', 'flee', 'approach', 'herd', 'juvenile'];
    const out = [];
    for (let i = 0; i < count; i++) {
        const x = chunkIndex * CHUNK + rng() * CHUNK;
        out.push({
            x,
            y: this.floorY(x) - 12,
            behavior: behaviors[Math.floor(rng() * behaviors.length) % behaviors.length],
            size: 6 + rng() * 8,
            hue: rng(),
            phase: rng() * Math.PI * 2,
            vx: 0,
        });
    }
    return out;
};