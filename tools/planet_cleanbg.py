# planet_cleanbg.py — пост-обработка референса планеты: гарантированный чистый
# тёмный космос вокруг диска + редкие мелкие звёзды. Убирает «странный фон»
# (пятна, разводы, ореолы, окна) независимо от того, что нарисовала SDXL.
#
# Логика:
#   1. Диск = крупнейший связный компонент маски «яркое/цветное».
#   2. Маска диска = круг вокруг центра компонента, радиус + LMB (сохраняет
#      тонкое лимб-свечение атмосферы у края — оно часть диска, не фона).
#   3. Если радиус > MAX_R — кадр масштабируется вокруг центра диска, чтобы диск
#      занимал ~76% ширины кадра (цель ТЗ: 70–80%), фон вокруг остаётся.
#   4. Всё вне маски заливается ровным почти чёрным + редкие мелкие звёзды
#      (детерминированный seed, не попадают на диск).
#
# Использование: python planet_cleanbg.py <input.png> <output.png> [seed] [limb_px] [max_r]
import sys
import numpy as np
from scipy import ndimage
from PIL import Image, ImageDraw

src, out = sys.argv[1], sys.argv[2]
seed = int(sys.argv[3]) if len(sys.argv) > 3 else 20260920
limb = int(sys.argv[4]) if len(sys.argv) > 4 else 35      # px сохранения лимб-свечения
max_r = int(sys.argv[5]) if len(sys.argv) > 5 else 390    # целевой радиус диска в px

im = Image.open(src).convert("RGB")
a = np.array(im).astype(int)
h, w = a.shape[:2]

# --- 1. Маска «яркое/цветное»: диск + свечение + любые пятна фона ---
bright = a.sum(axis=2)
sat = a.max(axis=2) - a.min(axis=2)
mask = (bright > 90) | (sat > 45)

# Морфология: убираем одиночные звёзды, сращиваем близкие части (облака/лимб)
mask = ndimage.binary_closing(mask, structure=np.ones((5, 5)), iterations=2)
mask = ndimage.binary_opening(mask, structure=np.ones((3, 3)), iterations=1)

lab, n = ndimage.label(mask)
if n == 0:
    print("no disk found"); sys.exit(1)
sizes = ndimage.sum(mask, lab, range(1, n + 1))
big = int(np.argmax(sizes)) + 1
disk_mask = lab == big

# --- 2. Центр и радиус диска ---
ys, xs = np.where(disk_mask)
cx, cy = float(xs.mean()), float(ys.mean())
R = float(np.sqrt((xs - cx) ** 2 + (ys - cy) ** 2).max())
print(f"disk center=({cx:.0f},{cy:.0f}) radius={R:.0f} -> target {max_r}px limb_keep={limb}px")

# --- 2.5. Если диск больше целевого — ресайз всего кадра, вставка по центру ---
if R > max_r:
    scale = max_r / R
    new_w, new_h = max(1, int(w * scale)), max(1, int(h * scale))
    im_small = Image.fromarray(a.astype(np.uint8)).resize((new_w, new_h), Image.LANCZOS)
    sm = np.array(im_small).astype(int)
    canvas_src = np.zeros((h, w, 3), dtype=int) + [3, 4, 7]
    ox = (w - new_w) // 2
    oy = (h - new_h) // 2
    canvas_src[oy:oy + new_h, ox:ox + new_w] = sm
    a = canvas_src
    cx = w / 2 + (cx - w / 2) * scale
    cy = h / 2 + (cy - h / 2) * scale
    R = R * scale
    print(f"  scaled to radius={R:.0f} center=({cx:.0f},{cy:.0f})")

# Круговая маска диска с сохранением лимба
yy, xx = np.mgrid[0:h, 0:w]
dist = np.sqrt((xx - cx) ** 2 + (yy - cy) ** 2)
keep = dist <= (R + limb)

# --- 3. Чистый космос + звёзды ---
bg = np.array([3, 4, 7], dtype=np.uint8)  # почти чёрный с лёгким холодным отливом
canvas = np.full((h, w, 3), bg, dtype=np.uint8)

# Звёзды: редкие, мелкие, не на диске
rng = np.random.default_rng(seed)
n_stars = 320
stars = []
while len(stars) < n_stars:
    sx, sy = rng.integers(0, w), rng.integers(0, h)
    if dist[sy, sx] <= R + 8:  # не на диск и не на лимб
        continue
    # параллакс-яркость: часть яркие, часть тусклые
    br = rng.integers(120, 256) if rng.random() < 0.35 else rng.integers(60, 130)
    stars.append((sx, sy, br))

# Рисуем звёзды размытым пятном 1-2 px
draw_canvas = Image.fromarray(canvas)
d = ImageDraw.Draw(draw_canvas)
for sx, sy, br in stars:
    size = 1 if br < 130 else 2
    d.ellipse([sx - size, sy - size, sx + size, sy + size], fill=(br, br, min(255, br + 20)))
canvas = np.array(draw_canvas)

# Вставляем диск (с лимбом) поверх космоса
out_arr = canvas.copy()
out_arr[keep] = a[keep].astype(np.uint8)

Image.fromarray(out_arr).save(out)
print(f"saved {out}")