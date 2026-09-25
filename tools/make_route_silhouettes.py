# -*- coding: utf-8 -*-
# Силуэты-мастера для ассетов мини-игры «Прокладка маршрута» (направление A).
# ТЗ: docs/specs/2026-09-25-маршрут-мини-игра-интерфейс.md §4 (промпт-ядра beacon /
# false_signal). 1024x1024, фон (5,5,5), тоновая подложка (Canny берёт внутреннюю
# структуру). Маяк — спутник-ориентир с тарелкой и панелями; ложный сигнал — «та же
# семья», но сломан (надкусанная тарелка, кривая мачта, отбитая панель, трещина).
# Ориентация не нормируется: у маяка «носа» нет.
import os
from PIL import Image, ImageDraw

OUT = r'C:\Zorion2\ai_drafts\route\silhouettes'
SIZE = 1024

BG = (5, 5, 5)
LIGHT = (205, 212, 224)
BASE = (150, 158, 172)
SHADOW = (86, 94, 110)
DARK = (18, 20, 26)


def new_canvas():
    img = Image.new('RGB', (SIZE, SIZE), BG)
    return img, ImageDraw.Draw(img)


def save(img, name):
    os.makedirs(OUT, exist_ok=True)
    path = os.path.join(OUT, name + '.png')
    img.save(path)
    print('Saved:', path)


def panel(d, x0, y0, x1, y1, cols, rows):
    d.rectangle([x0, y0, x1, y1], fill=LIGHT, outline=DARK, width=6)
    d.rectangle([x0 + 12, y0 + 12, x1 - 12, y1 - 12], fill=BASE, outline=None)
    for i in range(1, cols):
        x = x0 + (x1 - x0) * i / cols
        d.line([(x, y0 + 12), (x, y1 - 12)], fill=DARK, width=4)
    for j in range(1, rows):
        y = y0 + (y1 - y0) * j / rows
        d.line([(x0 + 12, y), (x1 - 12, y)], fill=DARK, width=4)


def hub(d, cx, cy, w, h):
    w2, h2 = w / 2, h / 2
    c = min(w, h) * 0.26
    pts = [(cx - w2 + c, cy - h2), (cx + w2 - c, cy - h2), (cx + w2, cy - h2 + c),
           (cx + w2, cy + h2 - c), (cx + w2 - c, cy + h2), (cx - w2 + c, cy + h2),
           (cx - w2, cy + h2 - c), (cx - w2, cy - h2 + c)]
    d.polygon(pts, fill=LIGHT, outline=DARK, width=7)
    d.polygon([(cx - w2 * 0.72 + c * 0.6, cy - h2 * 0.72 + 8),
               (cx + w2 * 0.72 - c * 0.6, cy - h2 * 0.72 + 8),
               (cx + w2 * 0.72, cy + h2 * 0.72 - 8),
               (cx - w2 * 0.72, cy + h2 * 0.72 - 8)], fill=BASE, outline=None)
    # грань-люк по центру
    d.rectangle([cx - w * 0.20, cy - h * 0.16, cx + w * 0.20, cy + h * 0.16],
                fill=SHADOW, outline=DARK, width=5)


def dish(d, cx, cy, r):
    d.ellipse([cx - r, cy - r * 0.78, cx + r, cy + r * 0.78], fill=LIGHT, outline=DARK, width=7)
    d.ellipse([cx - r * 0.70, cy - r * 0.54, cx + r * 0.70, cy + r * 0.54],
              fill=SHADOW, outline=DARK, width=4)
    d.line([(cx, cy), (cx, cy - r * 1.65)], fill=BASE, width=9)
    d.ellipse([cx - 15, cy - r * 1.65 - 15, cx + 15, cy - r * 1.65 + 15],
              fill=LIGHT, outline=DARK, width=5)


def mast(d, x0, y0, x1, y1, w=18):
    d.line([(x0, y0), (x1, y1)], fill=BASE, width=w)
    d.line([(x0 - w / 2, y0), (x1 - w / 2, y1)], fill=DARK, width=4)
    d.line([(x0 + w / 2, y0), (x1 + w / 2, y1)], fill=DARK, width=4)


def beacon_light(d, cx, cy, r, spikes=8, on=True):
    if on:
        for k in range(spikes):
            import math
            a = 2 * math.pi * k / spikes
            d.line([(cx + math.cos(a) * r * 0.7, cy + math.sin(a) * r * 0.7),
                    (cx + math.cos(a) * r * 2.3, cy + math.sin(a) * r * 2.3)],
                   fill=LIGHT, width=7)
        d.ellipse([cx - r, cy - r, cx + r, cy + r], fill=(255, 255, 255),
                  outline=DARK, width=5)
    else:
        # погасший/битый огонь — рваный неправильный сгусток
        d.polygon([(cx - r * 1.1, cy - r * 0.2), (cx - r * 0.2, cy - r * 1.1),
                   (cx + r * 0.9, cy - r * 0.5), (cx + r * 0.4, cy + r * 0.9),
                   (cx - r * 0.6, cy + r * 1.0)], fill=SHADOW, outline=DARK, width=6)


def bite(d, cx, cy, r, seed=0):
    """Надкус/скол на тарелке: вырезаем рваный клин фоном."""
    import random
    rng = random.Random(seed)
    a0 = rng.uniform(0, 6.28)
    pts = [(cx, cy)]
    for k in range(4):
        a = a0 + k * rng.uniform(0.5, 0.9)
        rr = r * rng.uniform(1.05, 1.45)
        pts.append((cx + __import__('math').cos(a) * rr, cy + __import__('math').sin(a) * rr))
    d.polygon(pts, fill=BG)


def crack(d, x0, y0, x1, y1, seed=0):
    import random, math
    rng = random.Random(seed)
    n = 6
    pts = []
    for i in range(n + 1):
        t = i / n
        px = x0 + (x1 - x0) * t + rng.uniform(-24, 24)
        py = y0 + (y1 - y0) * t + rng.uniform(-24, 24)
        pts.append((px, py))
    d.line(pts, fill=DARK, width=9)


def beacon_01():
    img, d = new_canvas()
    cx, cy = SIZE // 2, SIZE // 2 + 40
    panel(d, cx - 470, cy - 70, cx - 200, cy + 70, 4, 2)   # левая панель
    panel(d, cx + 210, cy - 90, cx + 460, cy + 90, 4, 3)   # правая панель (больше)
    d.line([(cx - 200, cy), (cx - 150, cy)], fill=DARK, width=12)
    d.line([(cx + 150, cy), (cx + 210, cy)], fill=DARK, width=12)
    hub(d, cx, cy, 300, 230)
    mast(d, cx + 90, cy - 110, cx + 130, cy - 330, w=20)
    dish(d, cx + 200, cy - 400, 150)
    beacon_light(d, cx - 110, cy - 250, 34, spikes=8, on=True)
    mast(d, cx - 110, cy - 110, cx - 110, cy - 235, w=16)
    save(img, 'beacon_01')


def beacon_02():
    img, d = new_canvas()
    cx, cy = SIZE // 2, SIZE // 2 + 30
    panel(d, cx - 430, cy - 100, cx - 240, cy + 100, 3, 3)  # маленькая левая
    panel(d, cx + 190, cy - 120, cx + 480, cy + 120, 5, 4)  # большая правая
    d.line([(cx - 240, cy), (cx - 160, cy)], fill=DARK, width=12)
    d.line([(cx + 150, cy), (cx + 190, cy)], fill=DARK, width=12)
    hub(d, cx, cy, 320, 260)
    mast(d, cx - 70, cy - 130, cx - 110, cy - 360, w=20)
    dish(d, cx - 190, cy - 430, 160)
    mast(d, cx + 80, cy - 130, cx + 80, cy - 280, w=16)
    beacon_light(d, cx + 80, cy - 300, 30, spikes=6, on=True)
    save(img, 'beacon_02')


def false_01():
    img, d = new_canvas()
    cx, cy = SIZE // 2, SIZE // 2 + 40
    # правая панель отбита — короткий обрубок
    panel(d, cx - 470, cy - 70, cx - 200, cy + 70, 4, 2)
    d.line([(cx + 150, cy), (cx + 230, cy)], fill=DARK, width=12)
    d.polygon([(cx + 230, cy - 55), (cx + 300, cy - 20), (cx + 250, cy + 40),
               (cx + 230, cy + 30)], fill=LIGHT, outline=DARK, width=6)
    hub(d, cx, cy, 300, 230)
    crack(d, cx - 120, cy - 90, cx + 100, cy + 100, seed=7)
    # мачта кривая, тарелка надкусана
    mast(d, cx + 90, cy - 110, cx + 150, cy - 250, w=20)
    mast(d, cx + 150, cy - 250, cx + 120, cy - 350, w=18)
    dish(d, cx + 170, cy - 400, 150)
    bite(d, cx + 250, cy - 430, 150, seed=3)
    beacon_light(d, cx - 110, cy - 250, 30, spikes=8, on=False)
    mast(d, cx - 110, cy - 110, cx - 110, cy - 235, w=16)
    save(img, 'false_01')


def false_02():
    img, d = new_canvas()
    cx, cy = SIZE // 2, SIZE // 2 + 30
    panel(d, cx - 430, cy - 100, cx - 240, cy + 100, 3, 3)
    # правая панель погнута вниз
    panel(d, cx + 190, cy - 40, cx + 470, cy + 180, 5, 4)
    d.line([(cx - 240, cy), (cx - 160, cy)], fill=DARK, width=12)
    d.line([(cx + 150, cy), (cx + 190, cy + 40)], fill=DARK, width=12)
    hub(d, cx, cy, 320, 260)
    crack(d, cx + 40, cy - 120, cx - 90, cy + 130, seed=11)
    crack(d, cx - 140, cy - 40, cx + 120, cy + 30, seed=12)
    mast(d, cx - 70, cy - 130, cx - 130, cy - 300, w=20)
    dish(d, cx - 210, cy - 400, 160)
    bite(d, cx - 150, cy - 470, 160, seed=5)
    beacon_light(d, cx + 80, cy - 250, 28, spikes=6, on=False)
    mast(d, cx + 80, cy - 130, cx + 80, cy - 235, w=16)
    save(img, 'false_02')


if __name__ == '__main__':
    beacon_01()
    beacon_02()
    false_01()
    false_02()
