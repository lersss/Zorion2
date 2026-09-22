# -*- coding: utf-8 -*-
# СПАЙК (спека 2026-09-21-ships-quality-rework §12.1): черновой рендер силуэтов
# кораблей — секции + светотень + внутренние швы + модули с объёмом +
# деталь-графика (§5.1). Только Python, Go не затрагивается.
# Три архетипа: flat_barge (coastal), manta_wing (humans), seed_pod (crystallites).
# Выход: 1024x1024, фон (5,5,5), вид сверху, нос вправо, асимметрия,
# seed = FNV1a(slug) (детерминизм). Цель: много внутренних кромок для Canny.
import argparse
import json
import os
import random

import numpy as np
from PIL import Image, ImageDraw
from scipy import ndimage

SIZE = 1024
BG = (5, 5, 5)
BORDER = (12, 12, 12)
CREAM = (232, 224, 186)
COLD_HULL = (170, 205, 235)
STEEL = (120, 140, 170)
BRONZE = (140, 90, 45)
DARK = (40, 44, 55)
LIGHT = (180, 210, 235)
LIGHT_HI = (230, 242, 252)
FIRE = (230, 120, 40)
COLD = (120, 220, 235)
CORAL = (220, 120, 110)


def fnv1a(slug):
    h = 0x811C9DC5
    for ch in slug.encode('utf-8'):
        h ^= ch
        h = (h * 0x01000193) & 0xFFFFFFFF
    return h


def mul(c, k):
    return tuple(int(max(0, min(255, round(v * k)))) for v in c)


def pmask(points):
    m = Image.new('L', (SIZE, SIZE), 0)
    ImageDraw.Draw(m).polygon([tuple(p) for p in points], fill=255)
    return np.array(m) > 0


def rmask(box, radius):
    m = Image.new('L', (SIZE, SIZE), 0)
    ImageDraw.Draw(m).rounded_rectangle([int(v) for v in box], radius=radius, fill=255)
    return np.array(m) > 0


def emask(box):
    m = Image.new('L', (SIZE, SIZE), 0)
    ImageDraw.Draw(m).ellipse([int(v) for v in box], fill=255)
    return np.array(m) > 0


def paint_region(img, mask, color, section=None, volume=False):
    """Залить маску с секциями (×1.10 нос / ×0.88 корма) и светом (×1.20 сверху /
    ×0.78 снизу); volume=True добавляет свет слева / тень справа (модули).
    Секции и свет — два независимых слоя с разными амплитудами (§5.2)."""
    ys, xs = np.nonzero(mask)
    if len(ys) == 0:
        return
    bx0, bx1, by0, by1 = xs.min(), xs.max(), ys.min(), ys.max()
    L = max(1, bx1 - bx0)
    cyy = (by0 + by1) / 2.0
    xx = np.arange(SIZE)[None, :]
    yy = np.arange(SIZE)[:, None]
    sec = np.ones((SIZE, SIZE), np.float64)
    if section is not None:
        nk, sk, b1, b2, slope = section
        fr = (xx - bx0) / L
        eff = fr - slope * (yy - cyy) / L
        sec = np.where(eff > b2, nk, np.where(eff < b1, sk, 1.0))
    top = np.full(SIZE, -1)
    bot = np.full(SIZE, -1)
    for x in np.unique(xs):
        rows = ys[xs == x]
        top[x], bot[x] = rows.min(), rows.max()
    hc = np.maximum(1, bot - top)
    band = np.ones((SIZE, SIZE), np.float64)
    band[(yy >= top[None, :] + 0.10 * hc[None, :]) & (yy <= top[None, :] + 0.16 * hc[None, :])] = 1.20
    band[(yy >= bot[None, :] - 0.12 * hc[None, :]) & (yy <= bot[None, :] - 0.04 * hc[None, :])] = 0.78
    if volume:
        ww = bx1 - bx0
        left = (xx >= bx0) & (xx <= bx0 + 0.22 * ww)
        right = (xx >= bx1 - 0.22 * ww) & (xx <= bx1)
        band = np.where(left, 1.25, np.where(right, 0.78, band))
    fac = sec * band * mask
    col = np.array(color, np.float64)
    out = np.clip(fac[ys, xs][:, None] * col[None, :], 0, 255)
    img[ys, xs] = out


def paint_module(img, mask, color, outline=4):
    """Модуль с обводкой BORDER и объёмом (свет верх-лево / тень низ-право)."""
    er = ndimage.binary_erosion(mask, iterations=outline)
    img[mask & ~er] = BORDER
    paint_region(img, er, color, volume=True)


def draw_polygon(d, points, fill, outline=None, width=3):
    d.polygon([tuple(p) for p in points], fill=fill, outline=outline, width=width)


# --- Архетипы: hull-маска + список модулей (маска, цвет) ---

def arch_flat_barge(fr, rng):
    x0, y0, W, H = fr
    def X(f): return x0 + W * f
    def Y(f): return y0 + H * f
    hull = pmask([
        (X(0.00), Y(0.05)), (X(0.14), Y(0.01)), (X(0.58), Y(0.05)),
        (X(0.86), Y(0.22)), (X(1.00), Y(0.50)), (X(0.86), Y(0.78)),
        (X(0.58), Y(0.95)), (X(0.14), Y(0.99)), (X(0.00), Y(0.95)),
    ])
    mods = []
    # надстройка-ярусы (2 ступени)
    mods.append((rmask([X(0.34), Y(0.30), X(0.62), Y(0.70)], 10), STEEL))
    mods.append((rmask([X(0.40), Y(0.42), X(0.56), Y(0.58)], 8), mul(STEEL, 1.15)))
    # борт-фальшборт по кромкам
    mods.append((rmask([X(0.06), Y(0.00), X(0.72), Y(0.06)], 4), BRONZE))
    mods.append((rmask([X(0.06), Y(0.94), X(0.72), Y(1.00)], 4), BRONZE))
    return hull, mods


def arch_manta_wing(fr, rng):
    x0, y0, W, H = fr
    def X(f): return x0 + W * f
    def Y(f): return y0 + H * f
    # тело-капсула с сужением к носу (не скруглённый прямоугольник, §5.2)
    hull = pmask([
        (X(0.16), Y(0.34)), (X(0.36), Y(0.30)), (X(0.58), Y(0.32)),
        (X(0.80), Y(0.36)), (X(1.00), Y(0.50)), (X(0.80), Y(0.64)),
        (X(0.58), Y(0.68)), (X(0.36), Y(0.70)), (X(0.16), Y(0.66)),
    ])
    mods = []
    # широкие стреловидные крылья (асимметрия: нижнее длиннее)
    mods.append((pmask([(X(0.46), Y(0.32)), (X(0.20), Y(0.00)), (X(0.40), Y(0.05)),
                        (X(0.70), Y(0.35))]), STEEL))
    mods.append((pmask([(X(0.46), Y(0.68)), (X(0.14), Y(1.00)), (X(0.42), Y(0.95)),
                        (X(0.72), Y(0.65))]), STEEL))
    # дюзы у кормы (слева)
    for i in range(3):
        ny = Y(0.38 + i * 0.10)
        mods.append((rmask([X(0.06), ny, X(0.16), ny + H * 0.06], 5), DARK))
    # кабина-стекло у носа (справа)
    mods.append((rmask([X(0.80), Y(0.40), X(0.94), Y(0.60)], 22), DARK))
    return hull, mods


def arch_seed_pod(fr, rng):
    x0, y0, W, H = fr
    def X(f): return x0 + W * f
    def Y(f): return y0 + H * f
    hull = rmask([X(0.04), Y(0.14), X(0.88), Y(0.86)], 60)
    # лепестки-створки кормы (слева)
    for i, fy in enumerate((0.22, 0.50, 0.78)):
        lean = 0.02 * (i - 1)
        hull |= pmask([(X(0.16), Y(fy - 0.10)), (X(0.00), Y(fy + lean - 0.16)),
                       (X(0.00), Y(fy + lean + 0.16)), (X(0.16), Y(fy + 0.10))])
    mods = []
    # сопла у кормы
    mods.append((emask([X(0.02), Y(0.42), X(0.14), Y(0.58)]), FIRE if False else STEEL))
    return hull, mods


ARCHES = {
    'flat_barge': arch_flat_barge,
    'manta_wing': arch_manta_wing,
    'seed_pod': arch_seed_pod,
}


def detail_graphics(img, hull, rng, base):
    """Деталь-графика (§5.1 п.10): штрихи-панели, люки, иллюминаторы, решётки.
    Внутри клип-маски корпуса (эрозия 8 px), детерминированно от seed."""
    inner = ndimage.binary_erosion(hull, iterations=8)
    ys, xs = np.nonzero(inner)
    if len(ys) == 0:
        return
    ov = Image.new('RGBA', (SIZE, SIZE), (0, 0, 0, 0))
    d = ImageDraw.Draw(ov)
    dark_sec = mul(base, 0.88)
    light_lit = mul(base, 1.20)
    for _ in range(rng.randint(10, 18)):               # штрихи-панели
        i = rng.randrange(len(ys))
        x, y = int(xs[i]), int(ys[i])
        w = rng.randint(6, 40); h = rng.randint(2, 6)
        if rng.random() < 0.5:
            w, h = h, w
        d.rectangle([x, y, x + w, y + h], fill=mul(base, 1 + rng.uniform(-0.15, 0.15)))
    for _ in range(rng.randint(4, 7)):                 # люки
        i = rng.randrange(len(ys))
        x, y = int(xs[i]), int(ys[i])
        w = rng.randint(10, 24); h = rng.randint(8, 18)
        d.rectangle([x, y, x + w, y + h], fill=dark_sec, outline=BORDER, width=2)
        d.rectangle([x + 2, y + 2, x + w - 2, y + 3], fill=light_lit)
    for _ in range(rng.randint(2, 4)):                 # иллюминаторы
        i = rng.randrange(len(ys))
        x, y = int(xs[i]), int(ys[i])
        r = rng.randint(4, 7)
        d.ellipse([x - r, y - r, x + r, y + r], fill=LIGHT)
    for _ in range(rng.randint(1, 2)):                 # решётки
        i = rng.randrange(len(ys))
        x, y = int(xs[i]), int(ys[i])
        for k in range(rng.randint(3, 5)):
            d.line([x + k * 6, y, x + k * 6, y + 22], fill=dark_sec, width=2)
    a = np.array(ov).astype(np.float64)
    a[..., 3] = np.where(inner, a[..., 3], 0)
    alpha = a[..., 3:4] / 255.0
    img[:] = img * (1 - alpha) + a[..., :3] * alpha


def seams(dr, hull, fr, rng, base):
    """Внутренние швы (§5.1 п.5): 3-4 поперечных + 1-3 продольных; 2-3 пары
    BORDER+light (двойная кромка). Рисуется в клип-маске корпуса."""
    x0, y0, W, H = fr
    ys, xs = np.nonzero(hull)
    by0, by1 = ys.min(), ys.max()
    dark_sec = mul(base, 0.88)
    for f in (0.32, 0.52, 0.72):
        x = x0 + W * f
        dr.line([x, by0 + 6, x - W * 0.05, by1 - 6], fill=dark_sec, width=4)
    for f in (0.35, 0.62):
        y = y0 + H * f
        dr.line([x0 + W * 0.06, y, x0 + W * 0.90, y], fill=mul(base, 1.08), width=3)
    for f in (0.44, 0.66):
        x = x0 + W * f
        dr.line([x, by0 + 10, x - W * 0.04, by1 - 10], fill=BORDER, width=2)
        dr.line([x + 3, by0 + 10, x - W * 0.04 + 3, by1 - 10], fill=mul(base, 1.20), width=2)


def render(spec):
    slug = spec['race']
    p = spec['params']
    rng = random.Random(fnv1a(slug))
    base = COLD_HULL if p.get('tone') == 'cold_hull' else CREAM
    W = min(p['aspect'] * p['y_fill'] * SIZE, 0.84 * SIZE)
    H = p['y_fill'] * SIZE
    fr = ((SIZE - W) / 2.0, (SIZE - H) / 2.0, W, H)
    hull, mods = ARCHES[p['archetype']](fr, rng)
    img = np.tile(np.array(BG, np.float64), (SIZE, SIZE, 1))
    slope = rng.uniform(0.12, 0.30)
    paint_region(img, hull, base, section=(1.10, 0.88, 0.35, 0.70, slope))
    # швы и деталь-графика внутри корпуса
    ov = Image.new('RGBA', (SIZE, SIZE), (0, 0, 0, 0))
    seams(ImageDraw.Draw(ov), hull, fr, rng, base)
    inner = ndimage.binary_erosion(hull, iterations=8)
    a = np.array(ov).astype(np.float64)
    a[..., 3] = np.where(inner, a[..., 3], 0)
    alpha = a[..., 3:4] / 255.0
    img = img * (1 - alpha) + a[..., :3] * alpha
    detail_graphics(img, hull, rng, base)
    # модули с объёмом (поверх корпуса и деталей)
    for m, col in mods:
        paint_module(img, m, col)
    # светлые акценты: кабина/ядро
    if p['archetype'] == 'manta_wing':
        cx, cy = fr[0] + fr[2] * 0.78, fr[1] + fr[3] * 0.5
        acc = emask([cx, cy - 26, cx + 52, cy + 26])
        img[acc] = LIGHT
        img[emask([cx - 6, cy - 22, cx + 12, cy + 2])] = LIGHT_HI
    if p['archetype'] == 'seed_pod':
        cx, cy = fr[0] + fr[2] * 0.72, fr[1] + fr[3] * 0.5
        img[emask([cx - 22, cy - 22, cx + 22, cy + 22])] = LIGHT
    if p['archetype'] == 'flat_barge':
        for i in range(4):
            cx = fr[0] + fr[2] * (0.16 + i * 0.16)
            img[emask([cx - 7, fr[1] + fr[3] * 0.5 - 7, cx + 7, fr[1] + fr[3] * 0.5 + 7])] = CORAL
    out = Image.fromarray(np.clip(img, 0, 255).astype(np.uint8), 'RGB')
    # обводка корпуса BORDER 5 px по внешней кромке
    edge = hull & ~ndimage.binary_erosion(hull, iterations=5)
    arr = np.array(out)
    arr[edge] = BORDER
    return Image.fromarray(arr, 'RGB'), fr


def main():
    ap = argparse.ArgumentParser(description='Спайк: силуэты кораблей (черновик §5)')
    ap.add_argument('--spec', required=True)
    ap.add_argument('--out', required=True)
    args = ap.parse_args()
    with open(args.spec, encoding='utf-8') as f:
        specs = json.load(f)
    os.makedirs(args.out, exist_ok=True)
    for spec in specs:
        img, _ = render(spec)
        path = os.path.join(args.out, spec['race'] + '.png')
        img.save(path)
        print('Saved: %s' % path)


if __name__ == '__main__':
    main()
