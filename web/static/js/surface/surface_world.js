// web/static/js/surface/surface_world.js
// Процедурный бесконечный мир одного биома (спека 2026-09-21 §7.2): ландшафт
// (fbm 2 слоя), пещеры (порог 2D-шума), формации по biome_category (В9), декор,
// жизнь (если life). Детерминирован от seed — один и тот же мир при повторе.
// Никакого Math.random: локальный PRNG.
import { CHUNK, FORMATIONS, PPM } from './surface_config.js';

// mulberry32 — локальный PRNG (не общий Math.random).
export function mulberry32(a) {
    return function () {
        a |= 0; a = (a + 0x6D2B79F5) | 0;
        let t = Math.imul(a ^ (a >>> 15), 1 | a);
        t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
        return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
    };
}

function hash1(i, seed) {
    let h = Math.imul((i | 0) ^ (seed | 0), 2654435761);
    h ^= h >>> 15;
    h = Math.imul(h, 2246822519);
    h ^= h >>> 13;
    return (h >>> 0) / 4294967296;
}

function hash2(x, y, seed) {
    let h = Math.imul((x | 0) ^ (seed | 0), 374761393);
    h = Math.imul(h ^ (y | 0), 668265263);
    h ^= h >>> 13;
    h = Math.imul(h, 1274126177);
    h ^= h >>> 16;
    return (h >>> 0) / 4294967296;
}

const fade = (t) => t * t * (3 - 2 * t);
const clamp01 = (v) => (v < 0 ? 0 : v > 1 ? 1 : v);
function smoothstep(a, b, x) {
    const t = clamp01((x - a) / (b - a));
    return t * t * (3 - 2 * t);
}

function noise1(x, seed) {
    const i = Math.floor(x);
    const f = x - i;
    const a = hash1(i, seed);
    const b = hash1(i + 1, seed);
    return a + (b - a) * fade(f);
}

function noise2(x, y, seed) {
    const ix = Math.floor(x), iy = Math.floor(y);
    const fx = x - ix, fy = y - iy;
    const a = hash2(ix, iy, seed);
    const b = hash2(ix + 1, iy, seed);
    const c = hash2(ix, iy + 1, seed);
    const d = hash2(ix + 1, iy + 1, seed);
    const u = fade(fx), v = fade(fy);
    return (a + (b - a) * u) * (1 - v) + (c + (d - c) * u) * v;
}

function fbm1(x, seed, octaves) {
    let amp = 0.5, freq = 1, sum = 0, norm = 0;
    for (let o = 0; o < octaves; o++) {
        sum += amp * noise1(x * freq, seed + o * 1013);
        norm += amp;
        amp *= 0.5;
        freq *= 2;
    }
    return sum / norm;
}

// Плотность жизни по категории биома (§7.3): биосфера/вода — плотно,
// литосфера/крио — скудно (споры, лишайники).
const LIFE_DENSITY = {
    'биосфера': 0.40, 'вода': 0.35, 'экзотика': 0.16,
    'литосфера': 0.06, 'крио': 0.06, 'вулканизм': 0.03,
};

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
        this.lifeDensity = this.life ? (LIFE_DENSITY[this.category] || 0.08) : 0;
    }

    _formationForRegion(idx) {
        const h = hash1(idx, this.seed ^ 0x51ed);
        return this.formations[Math.floor(h * this.formations.length) % this.formations.length];
    }

    // formationBlend — плавная (непрерывная) смесь формаций соседних регионов:
    // резкий скачок высоты на границе региона читался бы как обрыв мира.
    formationBlend(x) {
        const R = this.region;
        const i = Math.floor(x / R);
        const local = (x - i * R) / R;
        const prev = this._formationForRegion(i - 1);
        const cur = this._formationForRegion(i);
        const next = this._formationForRegion(i + 1);
        const wPrev = 1 - smoothstep(0, 0.18, local);
        const wNext = smoothstep(0.82, 1, local);
        const wCur = Math.max(0, 1 - wPrev - wNext);
        const mix = (k) => prev[k] * wPrev + cur[k] * wCur + next[k] * wNext;
        return {
            id: cur.id,
            ridge: mix('ridge'),
            flatten: mix('flatten'),
            offset: mix('offset'),
            caves: mix('caves'),
            float: cur.float,
        };
    }

    terrainHeight(x) {
        const f = this.formationBlend(x);
        const large = (fbm1(x * 0.0015, this.seed, 2) - 0.5) * 230 * (0.5 + 0.8 * f.ridge);
        const detail = (fbm1(x * 0.02, this.seed ^ 0x9e37, 3) - 0.5) * 80 * (1 - 0.7 * f.flatten);
        return this.baseY + f.offset - large - detail;
    }

    caveValue(x, y) {
        const n = noise2(x * 0.012, y * 0.02, this.seed ^ 0x1234);
        const n2 = noise2(x * 0.03, y * 0.05, this.seed ^ 0x77);
        return n * 0.7 + n2 * 0.3;
    }

    isCave(x, y) {
        const d = y - this.terrainHeight(x);
        if (d < 8) return false; // тонкая корка поверхности держит игрока
        const f = this.formationBlend(x);
        const threshold = 0.66 - 0.10 * f.caves;
        const depthBonus = Math.min(0.12, d / 3000);
        return this.caveValue(x, y) > threshold - depthBonus;
    }

    isSolid(x, y) {
        const th = this.terrainHeight(x);
        if (y >= th) return !this.isCave(x, y);
        const f = this.formationBlend(x);
        if (f.float) {
            const n = noise2(x * 0.01, y * 0.01, this.seed ^ 0xa5a5);
            if (n > 0.72 && y > th - 260) return true; // висячие скалы/арки
        }
        return false;
    }

    // decorAt — декор колонки (растительность/лишайники/камни).
    decorAt(x) {
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
    }

    // rareDecorAt — редкая декорация-находка (любопытство, §9): ≥1 на участок
    // 6000 px; вид детерминирован участком.
    rareDecorAt(x) {
        const R = 6000;
        const i = Math.floor(x / R);
        const cx = i * R + R * 0.5;
        if (Math.abs(x - cx) > 30) return null;
        const kinds = ['окаменелость', 'кристалл', 'обломок'];
        const kind = kinds[Math.floor(hash1(i, this.seed ^ 0xbeef) * kinds.length) % kinds.length];
        return { kind, x: cx };
    }

    // creaturesFor — животные чанка (не бой, §7.3): 2–3 поведения.
    creaturesFor(chunkIndex) {
        if (!this.life || this.lifeDensity < 0.1) return [];
        const rng = mulberry32((this.seed ^ Math.imul(chunkIndex, 0x9e3779b1)) >>> 0);
        const count = Math.floor(rng() * 3 * this.lifeDensity + this.lifeDensity);
        const behaviors = ['graze', 'flee', 'approach', 'herd', 'juvenile'];
        const out = [];
        for (let i = 0; i < count; i++) {
            const x = chunkIndex * CHUNK + rng() * CHUNK;
            out.push({
                x,
                y: this.terrainHeight(x) - 12,
                behavior: behaviors[Math.floor(rng() * behaviors.length) % behaviors.length],
                size: 6 + rng() * 8,
                hue: rng(),
                phase: rng() * Math.PI * 2,
                vx: 0,
            });
        }
        return out;
    }
}

// Утилиты цвета (палитра биома → оттенки рельефа).
export function parseHex(hex) {
    const h = hex && hex[0] === '#' ? hex.slice(1) : '8a7a6a';
    return {
        r: parseInt(h.slice(0, 2), 16) || 0,
        g: parseInt(h.slice(2, 4), 16) || 0,
        b: parseInt(h.slice(4, 6), 16) || 0,
    };
}

export function shade(hex, factor) {
    const c = parseHex(hex);
    const f = (v) => Math.max(0, Math.min(255, Math.round(v * factor)));
    return `rgb(${f(c.r)},${f(c.g)},${f(c.b)})`;
}

export function rgba(hex, alpha) {
    const c = parseHex(hex);
    return `rgba(${c.r},${c.g},${c.b},${alpha})`;
}

export { PPM };
