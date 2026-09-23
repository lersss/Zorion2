# -*- coding: utf-8 -*-
# Слой руды для астероидов пояса (массовая пачка). ТЗ: art_belt_asteroids.md §3.5.
# 4 grayscale-альфа PNG 256x256, прозрачный фон, БЕЛЫЙ = руда, без цвета
# (тинт/свечение задаёт клиент из COLORS.glint — аддитивно).
#   vein_crack — «прожилка»   (тонкая ломаная с ветвлением и разной толщиной)
#   vein_nest  — «гнездо»     (скопление с внутренней структурой: жилки/кристаллы)
#   vein_seam  — «шов»        (полоса с ветвлением, неровной толщиной)
#   vein_speck — «вкрапления» (мелкая россыпь — оставлена как одобрена в пилоте)
import os, math, random
import numpy as np
from PIL import Image, ImageDraw, ImageFilter

OUT = r'C:\Zorion2\ai_drafts\belt_asteroids\veins'
S = 256
MARGIN = 20


def soft(layer, radius=2, cutoff=None):
    layer = layer.filter(ImageFilter.GaussianBlur(radius))
    if cutoff is not None:
        layer = layer.point(lambda v: 0 if v < cutoff else v)
    return layer


def to_rgba(layer):
    """L-маска -> RGBA: белый цвет, альфа = маска."""
    white = Image.new('RGBA', (S, S), (255, 255, 255, 0))
    white.putalpha(layer)
    return white


def polyline(d, pts, w0, w1, fill=255):
    """Ломаная с ПЕРЕМЕННОЙ толщиной от w0 к w1 (тонкие прожилки/швы)."""
    n = len(pts) - 1
    for i in range(n):
        w = w0 + (w1 - w0) * (i / max(1, n - 1))
        d.line([pts[i], pts[i + 1]], fill=fill, width=max(1, int(round(w))))


def branch(layer, rng, x, y, ang, length, width, depth, wobble=0.45):
    """Ветвящаяся жилка: путь с дрожанием угла, сужением к концу и ответвлениями."""
    steps = rng.randint(4, 7)
    pts = [(x, y)]
    seg = length / steps
    for _ in range(steps):
        ang += rng.uniform(-wobble, wobble)
        x += math.cos(ang) * seg
        y += math.sin(ang) * seg
        x = max(2, min(S - 2, x))
        y = max(2, min(S - 2, y))
        pts.append((x, y))
    polyline(ImageDraw.Draw(layer), pts, width, max(1.0, width * 0.35))
    if depth <= 0:
        return
    for i in range(1, len(pts) - 1):
        if rng.random() < 0.45:
            ba = ang + rng.choice([-1, 1]) * rng.uniform(0.5, 1.1)
            branch(layer, rng, pts[i][0], pts[i][1], ba,
                   length * rng.uniform(0.35, 0.6), max(1.0, width * 0.55),
                   depth - 1, wobble)


def vein_crack(seed):
    """Прожилка: тонкая главная жила + ветвление, толщина 4->1.5."""
    rng = random.Random(seed)
    layer = Image.new('L', (S, S), 0)
    y = S // 2 + rng.randint(-30, 30)
    branch(layer, rng, MARGIN, y, rng.uniform(-0.25, 0.25), S - 2 * MARGIN, 4.0, 2)
    return to_rgba(soft(layer, 2, cutoff=24))


def vein_seam(seed):
    """Шов: полоса-жила с неровной толщиной + ветвление и короткие зубцы."""
    rng = random.Random(seed)
    layer = Image.new('L', (S, S), 0)
    d = ImageDraw.Draw(layer)
    y = S // 2
    pts = []
    n = 9
    for i in range(n + 1):
        x = MARGIN + (S - 2 * MARGIN) * i / n
        y = max(MARGIN, min(S - MARGIN, y + rng.randint(-30, 30)))
        pts.append((x, y))
    polyline(d, pts, 9.0, 9.0)            # основная полоса (тоньше пилота)
    polyline(d, pts, 5.0, 5.0)            # ядро с утолщением
    # зубцы/кристаллы поперёк полосы
    for i in range(1, len(pts) - 1):
        if rng.random() < 0.75:
            px, py = pts[i]
            side = rng.choice([-1, 1])
            ln = rng.randint(8, 24)
            d.line([(px, py), (px + rng.randint(-8, 8), py + side * ln)],
                   fill=255, width=rng.choice([2, 3]))
    # ответвления
    for _ in range(3):
        i = rng.randrange(1, len(pts) - 1)
        branch(layer, rng, pts[i][0], pts[i][1],
               rng.choice([-1, 1]) * rng.uniform(0.5, 1.0),
               rng.randint(40, 80), 2.5, 1)
    return to_rgba(soft(layer, 2, cutoff=20))


def vein_nest(seed):
    """Гнездо: скопление с ВНУТРЕННЕЙ структурой — жилки и кристаллы внутри."""
    rng = random.Random(seed)
    base = Image.new('L', (S, S), 0)
    db = ImageDraw.Draw(base)
    cx, cy = S // 2, S // 2
    for _ in range(rng.randint(4, 6)):
        px = cx + rng.randint(-48, 48)
        py = cy + rng.randint(-48, 48)
        r = rng.randint(18, 38)
        db.ellipse([px - r, py - r, px + r, py + r], fill=88)
    base = base.filter(ImageFilter.GaussianBlur(8))

    layer = Image.new('L', (S, S), 0)
    # внутренние жилки (ярче фона гнезда) — переплетение
    for _ in range(3):
        branch(layer, rng, cx + rng.randint(-40, 40), cy + rng.randint(-40, 40),
               rng.uniform(0, 2 * math.pi), rng.randint(50, 90), 3.0, 1)
    d = ImageDraw.Draw(layer)
    # кристаллы — яркие ромбы
    for _ in range(rng.randint(6, 10)):
        px = cx + rng.randint(-55, 55)
        py = cy + rng.randint(-55, 55)
        r = rng.randint(4, 9)
        d.polygon([(px, py - r), (px + r * 0.7, py), (px, py + r), (px - r * 0.7, py)],
                  fill=255)
    # мягкое смешение: берём максимум яркости (гнездо + яркие жилки/кристаллы)
    a = np.maximum(np.array(base), np.array(layer.filter(ImageFilter.GaussianBlur(1.5))))
    return to_rgba(Image.fromarray(a))


def vein_speck(seed):
    """Вкрапления: мелкая россыпь (одобрена в пилоте — без изменений)."""
    rng = random.Random(seed)
    layer = Image.new('L', (S, S), 0)
    d = ImageDraw.Draw(layer)
    for _ in range(rng.randint(45, 70)):
        px = rng.randint(MARGIN, S - MARGIN)
        py = rng.randint(MARGIN, S - MARGIN)
        r = rng.randint(2, 7)
        d.ellipse([px - r, py - r, px + r, py + r], fill=rng.choice([255, 235, 200]))
    return to_rgba(soft(layer, 2))


if __name__ == '__main__':
    os.makedirs(OUT, exist_ok=True)
    for name, fn, seed in [('vein_crack', vein_crack, 40111),
                           ('vein_nest', vein_nest, 40112),
                           ('vein_seam', vein_seam, 40113),
                           ('vein_speck', vein_speck, 40104)]:
        path = os.path.join(OUT, name + '.png')
        fn(seed).save(path)
        print('Saved:', path)
