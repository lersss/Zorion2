# -*- coding: utf-8 -*-
# Силуэты-мастера астероидов пояса (массовая пачка, направление 2 «фотокамень+жила»).
# ТЗ: docs/gamedesign/art/art_belt_asteroids.md §3.2.
# 1024x1024, фон (5,5,5), одно тело ~80% холста, центрировано, ТОНОВАЯ подложка
# (основа/тень/свет/границы) — иначе Canny даёт только внешний контур.
# Формы неправильные, несимметричные, с вмятинами. Никакого txt2img — только форма.
# Вся внутренняя детализация (крáтеры, трещины, сколы) обрезается по маске тела.
#
# ТЕХНИКА (находка пилота): вмятины/крáтеры — ТОЛЬКО тонкая тёмная дуга-кромка БЕЗ
# заливки и без светлого кольца. Заливка/кольцо заставляют SDXL лепить крáтер
# выпуклым шаром/валуном. Дуга даёт Canny внутреннюю структуру, а саму вмятину
# рисует SDXL по промпту.
#
# Формы rock_01 и rock_03 (смягчённая) — из одобренного пилота; rock_02 заменяет
# «плоскую плиту» пилота новым угловатым силуэтом с пластами.
import os, math, random
from PIL import Image, ImageDraw, ImageChops

OUT = r'C:\Zorion2\ai_drafts\belt_asteroids\silhouettes'
SIZE = 1024
CX = CY = SIZE // 2
TARGET = 0.80 * SIZE  # ~819 px по длинной стороне (§3.2: ~80% холста, запас >=90px)

BG = (5, 5, 5)           # фон
BASE = (107, 114, 128)   # #6b7280 основа
SHADOW = (43, 48, 58)    # #2b303a тень
LIGHT = (139, 147, 161)  # #8b93a1 свет
DARK = (20, 22, 26)      # #14161a границы/трещины
EDGE = (69, 76, 88)      # #454c58 кромка


def new_canvas():
    img = Image.new('RGB', (SIZE, SIZE), BG)
    return img, ImageDraw.Draw(img)


def save(img, name):
    os.makedirs(OUT, exist_ok=True)
    path = os.path.join(OUT, name + '.png')
    img.save(path)
    print('Saved:', path)


def outline(rng, n=18, harmonics=((2, 0.13), (3, 0.09), (5, 0.06), (7, 0.04)),
            bumps=2, dents=3, noise=0.05, facet=0.0):
    """Неправильный несимметричный контур: гармоники + бугры + вмятины.
    facet>0 — «огранка»: часть углов тянем наружу, получаются прямые рёбра."""
    phases = [(k, a, rng.uniform(0, 2 * math.pi)) for k, a in harmonics]
    radii = []
    for i in range(n):
        ang = 2 * math.pi * i / n
        r = 1.0
        for k, a, ph in phases:
            r += a * math.sin(k * ang + ph)
        r += rng.uniform(-noise, noise)
        radii.append(r)
    for _ in range(dents):
        c = rng.randrange(n)
        radii[c] *= rng.uniform(0.68, 0.80)
        radii[(c - 1) % n] *= 0.90
        radii[(c + 1) % n] *= 0.90
    for _ in range(bumps):
        c = rng.randrange(n)
        radii[c] *= rng.uniform(1.10, 1.22)
        radii[(c - 1) % n] *= 1.05
        radii[(c + 1) % n] *= 1.05
    if facet > 0:
        for i in range(n):
            if rng.random() < 0.3:
                radii[i] *= 1.0 + facet * rng.uniform(0.5, 1.0)
    pts = []
    for i in range(n):
        ang = 2 * math.pi * i / n
        r = radii[i]
        pts.append((math.cos(ang) * r, math.sin(ang) * r))
    return pts


def chaikin(pts, iterations=1):
    """Срезание углов — смягчает жёсткие «гранёные» рёбра (rock_03)."""
    for _ in range(iterations):
        out = []
        n = len(pts)
        for i in range(n):
            p, q = pts[i], pts[(i + 1) % n]
            out.append((0.75 * p[0] + 0.25 * q[0], 0.75 * p[1] + 0.25 * q[1]))
            out.append((0.25 * p[0] + 0.75 * q[0], 0.25 * p[1] + 0.75 * q[1]))
        pts = out
    return pts


def chip(pts, rng, span=5, inset=0.55):
    """Отколотая треть: V-образный вырез (глубже к середине дуги) — читается как
    крупный скол в силуэте."""
    n = len(pts)
    i0 = rng.randrange(n)
    idx = [(i0 + k) % n for k in range(span + 1)]
    a, b = pts[idx[0]], pts[idx[-1]]
    cx, cy = centroid(pts)
    for k, i in enumerate(idx):
        t = k / (len(idx) - 1)
        depth = inset * (1.0 - abs(t - 0.5) * 2.0)  # максимум в середине
        px = a[0] + (b[0] - a[0]) * t
        py = a[1] + (b[1] - a[1]) * t
        pts[i] = (px + (cx - px) * depth, py + (cy - py) * depth)
    return pts


def fit(pts, aspect=1.0, target=TARGET):
    """Масштабировать контур по длинной стороне к target и центрировать."""
    xs = [p[0] * aspect for p in pts]
    ys = [p[1] for p in pts]
    w = max(xs) - min(xs)
    h = max(ys) - min(ys)
    s = target / max(w, h)
    cx = (max(xs) + min(xs)) / 2
    cy = (max(ys) + min(ys)) / 2
    return [(CX + (x - cx) * s, CY + (y - cy) * s) for x, y in zip(xs, ys)]


def centroid(pts):
    return sum(p[0] for p in pts) / len(pts), sum(p[1] for p in pts) / len(pts)


def scale_about(pts, k, dx=0.0, dy=0.0):
    cx, cy = centroid(pts)
    return [(cx + (x - cx) * k + dx, cy + (y - cy) * k + dy) for x, y in pts]


def body_mask(pts):
    m = Image.new('L', (SIZE, SIZE), 0)
    ImageDraw.Draw(m).polygon(pts, fill=255)
    return m


def paste_clipped(img, detail, mask):
    """Наложить RGBA-детали только там, где тело (маска)."""
    alpha = ImageChops.multiply(detail.split()[3], mask)
    detail.putalpha(alpha)
    img.paste(detail, (0, 0), detail)


def draw_rim(d, cx, cy, r, rng):
    """Крáтер — тонкая тёмная дуга-кромка (см. ТЕХНИКА в шапке файла)."""
    start = rng.uniform(0, 360)
    d.arc([cx - r, cy - r, cx + r, cy + r], start=start,
          end=start + rng.uniform(200, 320), fill=DARK, width=5)
    d.arc([cx - r * 0.55, cy - r * 0.55, cx + r * 0.55, cy + r * 0.55],
          start=start + 40, end=start + rng.uniform(160, 260), fill=SHADOW, width=3)


def draw_cracks(d, pts, rng, count=3):
    """Трещины: тёмные ломаные ВНУТРИ тела (не доходят до границы)."""
    cx, cy = centroid(pts)
    maxr = max(math.hypot(x - cx, y - cy) for x, y in pts)
    for _ in range(count):
        ang = rng.uniform(0, 2 * math.pi)
        x0 = cx + math.cos(ang) * maxr * rng.uniform(0.05, 0.25)
        y0 = cy + math.sin(ang) * maxr * rng.uniform(0.05, 0.25)
        x1 = cx + math.cos(ang) * maxr * rng.uniform(0.52, 0.70)
        y1 = cy + math.sin(ang) * maxr * rng.uniform(0.52, 0.70)
        midx = (x0 + x1) / 2 + rng.uniform(-0.14, 0.14) * maxr
        midy = (y0 + y1) / 2 + rng.uniform(-0.14, 0.14) * maxr
        d.line([(x0, y0), (midx, midy), (x1, y1)], fill=DARK, width=rng.choice([5, 6, 8]))
        if rng.random() < 0.7:
            bx = midx + (x1 - x0) * 0.25
            by = midy + (y1 - y0) * 0.25
            d.line([(midx, midy), (bx + rng.uniform(-0.15, 0.15) * maxr,
                                   by + rng.uniform(-0.15, 0.15) * maxr)],
                   fill=DARK, width=4)


def draw_speckles(d, pts, rng, count=14):
    """Мелкая тёмная россыпь — SDXL читает как мелкие крáтеры/поры."""
    cx, cy = centroid(pts)
    maxr = max(math.hypot(x - cx, y - cy) for x, y in pts)
    for _ in range(count):
        ang = rng.uniform(0, 2 * math.pi)
        fr = rng.uniform(0.05, 0.60)
        px = cx + math.cos(ang) * maxr * fr
        py = cy + math.sin(ang) * maxr * fr
        r = maxr * rng.uniform(0.010, 0.030)
        d.ellipse([px - r, py - r, px + r, py + r], fill=SHADOW)


def draw_body(rng, pts, speck=14, rims=(3, 5), rim_scale=(0.08, 0.17),
              cracks=3, plate=False):
    """Тоновая подложка (светлая кромка сверху, основа) + вмятины/трещины/скол."""
    img, d = new_canvas()
    mask = body_mask(pts)
    cx, cy = centroid(pts)
    maxr = max(math.hypot(x - cx, y - cy) for x, y in pts)

    d.polygon(pts, fill=LIGHT, outline=None)
    d.polygon(scale_about(pts, 0.90, dy=14), fill=BASE, outline=None)

    if plate:
        detail = Image.new('RGBA', (SIZE, SIZE), (0, 0, 0, 0))
        dd = ImageDraw.Draw(detail)
        cut = [(cx + maxr * 0.30, cy + maxr * 0.05),
               (cx + maxr * 0.95, cy + maxr * 0.30),
               (cx + maxr * 0.72, cy + maxr * 0.80),
               (cx + maxr * 0.10, cy + maxr * 0.70)]
        dd.polygon(cut, fill=SHADOW + (255,), outline=DARK + (255,), width=6)
        paste_clipped(img, detail, mask)

    detail = Image.new('RGBA', (SIZE, SIZE), (0, 0, 0, 0))
    dd = ImageDraw.Draw(detail)
    draw_speckles(dd, pts, rng, count=speck)
    for _ in range(rng.randint(*rims)):
        ang = rng.uniform(0, 2 * math.pi)
        fr = rng.uniform(0.10, 0.50)
        px = cx + math.cos(ang) * maxr * fr
        py = cy + math.sin(ang) * maxr * fr
        draw_rim(dd, px, py, maxr * rng.uniform(*rim_scale), rng)
    paste_clipped(img, detail, mask)

    detail = Image.new('RGBA', (SIZE, SIZE), (0, 0, 0, 0))
    dd = ImageDraw.Draw(detail)
    draw_cracks(dd, pts, rng, count=cracks)
    paste_clipped(img, detail, mask)

    d.line(list(pts) + [pts[0]], fill=DARK, width=7)
    return img


# --- одобренные пилотные формы (1 и 3) -------------------------------------------------

def rock_01(seed):
    """Пилот 1 — округлая «картофелина» с крáтерами (берём как есть)."""
    rng = random.Random(seed)
    pts = fit(outline(rng, n=18, bumps=2, dents=3, noise=0.05), aspect=1.08)
    save(draw_body(rng, pts), 'rock_01')


def rock_03(seed):
    """Пилот 3 — гранёная, но рёбра СМЯГЧЕНЫ (chaikin), без «шестиугольника»."""
    rng = random.Random(seed)
    pts = outline(rng, n=15, harmonics=((2, 0.08), (3, 0.12), (5, 0.07), (7, 0.05)),
                  bumps=1, dents=4, noise=0.06, facet=0.05)
    pts = chaikin(pts, 1)
    pts = fit(pts, aspect=1.15)
    save(draw_body(rng, pts), 'rock_03')


# --- новые формы -----------------------------------------------------------------------

def rock_02(seed):
    """Угловатый с пластами (замена пилотной «плоской плиты»): рёбра + скол-пласт."""
    rng = random.Random(seed)
    pts = outline(rng, n=12, harmonics=((2, 0.09), (3, 0.13), (5, 0.06)),
                  bumps=2, dents=3, noise=0.03, facet=0.18)
    pts = fit(pts, aspect=1.45)
    save(draw_body(rng, pts, speck=12, rims=(3, 4), rim_scale=(0.07, 0.14),
                   cracks=3, plate=True), 'rock_02')


def rock_04(seed):
    """Удлинённая/угловатая: длинное сужающееся тело."""
    rng = random.Random(seed)
    pts = outline(rng, n=13, harmonics=((2, 0.13), (3, 0.10), (5, 0.06), (7, 0.04)),
                  bumps=2, dents=3, noise=0.05, facet=0.08)
    pts = fit(pts, aspect=1.60)
    save(draw_body(rng, pts, speck=26, rims=(5, 7), rim_scale=(0.06, 0.13),
                   cracks=3), 'rock_04')


def rock_05(seed):
    """«Картофелина» без граней: округлая, но неправильная и несимметричная."""
    rng = random.Random(seed)
    pts = outline(rng, n=16, harmonics=((2, 0.11), (3, 0.07), (5, 0.05), (7, 0.03)),
                  bumps=3, dents=3, noise=0.06, facet=0.0)
    pts = chaikin(pts, 1)
    pts = fit(pts, aspect=1.12)
    save(draw_body(rng, pts, speck=30, rims=(5, 7), rim_scale=(0.05, 0.11),
                   cracks=3), 'rock_05')


def rock_06(seed):
    """Тело с отколотой третью: округлое тело + угловатый вырез-скол (V)."""
    rng = random.Random(seed)
    pts = outline(rng, n=16, harmonics=((2, 0.11), (3, 0.09), (5, 0.06), (7, 0.04)),
                  bumps=3, dents=3, noise=0.06, facet=0.0)
    pts = chaikin(pts, 1)
    pts = chip(pts, rng, span=4, inset=0.46)
    pts = fit(pts, aspect=1.18)
    save(draw_body(rng, pts, speck=22, rims=(4, 6), rim_scale=(0.08, 0.16),
                   cracks=2), 'rock_06')


def rock_07(seed):
    """Зернистая/пористая: много мелких вмятин и пор."""
    rng = random.Random(seed)
    pts = outline(rng, n=14, harmonics=((2, 0.10), (3, 0.10), (5, 0.06), (7, 0.04)),
                  bumps=2, dents=3, noise=0.05, facet=0.04)
    pts = fit(pts, aspect=1.05)
    save(draw_body(rng, pts, speck=46, rims=(5, 7), rim_scale=(0.05, 0.12),
                   cracks=2), 'rock_07')


def rock_08(seed):
    """Сплюснутая с крупными вмятинами."""
    rng = random.Random(seed)
    pts = outline(rng, n=18, harmonics=((2, 0.16), (3, 0.07), (5, 0.05)),
                  bumps=1, dents=5, noise=0.05, facet=0.03)
    pts = fit(pts, aspect=1.35)
    save(draw_body(rng, pts, speck=10, rims=(4, 6), rim_scale=(0.10, 0.20),
                   cracks=3), 'rock_08')


# --- мелкие обломки (64x64 в игре) -----------------------------------------------------

def debris_01(seed):
    rng = random.Random(seed)
    pts = outline(rng, n=11, harmonics=((2, 0.14), (3, 0.10), (5, 0.06)),
                  bumps=1, dents=2, noise=0.06, facet=0.06)
    pts = fit(pts, aspect=1.25)
    save(draw_body(rng, pts, speck=6, rims=(2, 3), rim_scale=(0.12, 0.22),
                   cracks=2), 'debris_01')


def debris_02(seed):
    rng = random.Random(seed)
    pts = outline(rng, n=12, harmonics=((2, 0.12), (3, 0.11), (5, 0.05)),
                  bumps=2, dents=2, noise=0.06, facet=0.05)
    pts = chip(pts, rng, span=4, inset=0.40)
    pts = fit(pts, aspect=1.10)
    save(draw_body(rng, pts, speck=6, rims=(2, 3), rim_scale=(0.12, 0.22),
                   cracks=2), 'debris_02')


def debris_03(seed):
    rng = random.Random(seed)
    pts = outline(rng, n=10, harmonics=((2, 0.16), (3, 0.09), (5, 0.06)),
                  bumps=1, dents=3, noise=0.06, facet=0.12)
    pts = fit(pts, aspect=1.35)
    save(draw_body(rng, pts, speck=6, rims=(2, 3), rim_scale=(0.12, 0.22),
                   cracks=2), 'debris_03')


if __name__ == '__main__':
    rock_01(20260923)
    rock_02(20260926)
    rock_03(20260925)
    rock_04(20260927)
    rock_05(20260928)
    rock_06(20260929)
    rock_07(20260930)
    rock_08(20260931)
    debris_01(20260932)
    debris_02(20260933)
    debris_03(20260934)
