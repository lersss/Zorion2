# make_planet_silhouette.py — силуэт планеты для ControlNet: чёрный фон + круглый диск
# с МЯГКОЙ «картой» биомов (синие океаны, зелёные континенты, белые шапки, размытие).
# Без тёмного кольца-обводки (иначе Canny читает «иллюминатор»). Диск ~56% кадра, центр.
# Использование: python make_planet_silhouette.py <out.png> [seed]
import sys, math, random
from PIL import Image, ImageDraw, ImageFilter

out = sys.argv[1]
seed = int(sys.argv[2]) if len(sys.argv) > 2 else 42
random.seed(seed)

W = H = 1024
im = Image.new("RGB", (W, H), (5, 5, 5))
d = ImageDraw.Draw(im)

cx, cy = W // 2, H // 2
R = 290  # радиус диска -> диаметр 580 (~57% кадра)

OCEAN = (30, 90, 160)
GREEN = (80, 130, 60)
SAND = (200, 170, 100)
ICE = (230, 238, 246)

d.ellipse([cx - R, cy - R, cx + R, cy + R], fill=OCEAN)

def blobby(cx0, cy0, r, color, n=10, jitter=0.4):
    pts = []
    for i in range(n):
        a = 2 * math.pi * i / n + random.uniform(-0.3, 0.3)
        rr = r * (1 - jitter + random.random() * 2 * jitter)
        pts.append((cx0 + rr * math.cos(a), cy0 + rr * math.sin(a)))
    d.polygon(pts, fill=color)

# Континенты — крупные мягкие пятна
for (ccx, ccy, rr) in [
    (cx - 115, cy - 35, 95),
    (cx + 65, cy - 115, 72),
    (cx + 135, cy + 65, 82),
    (cx - 25, cy + 135, 68),
    (cx + 20, cy - 25, 48),
]:
    blobby(ccx, ccy, rr, GREEN, n=10)
    blobby(ccx + random.randint(-20, 20), ccy + random.randint(-20, 20), rr * 0.35, SAND, n=8)

# Полярные шапки — сегменты сверху/снизу с неровным краем
for sgn in (1, -1):
    y_edge = cy - int(R * 0.22) if sgn == 1 else cy + int(R * 0.22)
    d.pieslice([cx - R, cy - R, cx + R, cy + R],
               90 if sgn == 1 else 270, 270 if sgn == 1 else 90, fill=ICE)
    step = 10
    for x in range(cx - R + 8, cx + R - 8, step):
        yy = y_edge + random.randint(-16, 16)
        y_top = cy - R if sgn == 1 else cy + R
        d.rectangle([x, min(y_top, yy) - 2, x + step, max(y_top, yy) + 2], fill=ICE)

# Размытие — мягкие границы для Canny (не «окно», а природные переходы)
im = im.filter(ImageFilter.GaussianBlur(6))

im.save(out)
print(f"saved {out} ({W}x{H})")