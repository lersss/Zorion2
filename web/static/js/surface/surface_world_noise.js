// mulberry32 — локальный PRNG (не общий Math.random).
export function mulberry32(a) {
    return function () {
        a |= 0; a = (a + 0x6D2B79F5) | 0;
        let t = Math.imul(a ^ (a >>> 15), 1 | a);
        t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
        return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
    };
}

// hash1 — единственный хеш-механизм мира; экспортируется для частиц погоды
// (surface_weather.js §5.1 п.2): второй хеш дал бы разные миры при одном seed.
export function hash1(i, seed) {
    let h = Math.imul((i | 0) ^ (seed | 0), 2654435761);
    h ^= h >>> 15;
    h = Math.imul(h, 2246822519);
    h ^= h >>> 13;
    return (h >>> 0) / 4294967296;
}

export function hash2(x, y, seed) {
    let h = Math.imul((x | 0) ^ (seed | 0), 374761393);
    h = Math.imul(h ^ (y | 0), 668265263);
    h ^= h >>> 13;
    h = Math.imul(h, 1274126177);
    h ^= h >>> 16;
    return (h >>> 0) / 4294967296;
}

const fade = (t) => t * t * (3 - 2 * t);
export const clamp01 = (v) => (v < 0 ? 0 : v > 1 ? 1 : v);
export function smoothstep(a, b, x) {
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

export function noise2(x, y, seed) {
    const ix = Math.floor(x), iy = Math.floor(y);
    const fx = x - ix, fy = y - iy;
    const a = hash2(ix, iy, seed);
    const b = hash2(ix + 1, iy, seed);
    const c = hash2(ix, iy + 1, seed);
    const d = hash2(ix + 1, iy + 1, seed);
    const u = fade(fx), v = fade(fy);
    return (a + (b - a) * u) * (1 - v) + (c + (d - c) * u) * v;
}

export function fbm1(x, seed, octaves) {
    let amp = 0.5, freq = 1, sum = 0, norm = 0;
    for (let o = 0; o < octaves; o++) {
        sum += amp * noise1(x * freq, seed + o * 1013);
        norm += amp;
        amp *= 0.5;
        freq *= 2;
    }
    return sum / norm;
}

// num — число из рецепта или дефолт.
export function num(v, def) {
    return typeof v === 'number' ? v : def;
}

// primRange — [lo,hi] по индексу фичи i (детерминированно от seed) или число.
export function primRange(v, i, seed, def) {
    if (typeof v === 'number') return v;
    if (Array.isArray(v) && v.length === 2 && typeof v[0] === 'number' && typeof v[1] === 'number') {
        return v[0] + hash1(i, seed) * (v[1] - v[0]);
    }
    return def;
}
