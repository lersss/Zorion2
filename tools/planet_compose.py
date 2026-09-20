# planet_compose.py — сборка «полного диска в центре»: обрезает квадрат вокруг центра
# масс яркости кандидата (диск крупнее кадра), вписывает в круг R на чёрный фон 1024x1024.
# Использование: python planet_compose.py <input.png> <output.png> [radius]
import sys
import numpy as np
from PIL import Image

src = sys.argv[1]
out = sys.argv[2]
R = int(sys.argv[3]) if len(sys.argv) > 3 else 300

im = Image.open(src).convert("RGB")
a = np.array(im).astype(int)
bright = a.sum(axis=2)

# Центр масс ярких пикселей (центр диска, диск крупнее кадра)
ys, xs = np.where(bright > 250)
if len(xs) == 0:
    print("no disk found"); sys.exit(1)
cx = xs.mean()
cy = ys.mean()
print(f"bright centroid=({cx:.0f},{cy:.0f})")

# Квадрат вокруг центра: половина стороны = радиус R кадра + запас на атмосферу/облака
# Берём с запасом 1.35R (атмосфера, свечение) — диск впишется в круг R.
half = int(R * 1.35)
w, h = im.size
x0 = max(0, int(cx - half)); x1 = min(w, int(cx + half))
y0 = max(0, int(cy - half)); y1 = min(h, int(cy + half))
im2 = im.crop((x0, y0, x1, y1))

# Масштаб: сторона кадра (2*half) -> 2*R*1.12 (планета чуть меньше круга)
scale = (2 * R * 1.12) / im2.width
im2 = im2.resize((int(im2.width * scale), int(im2.height * scale)), Image.LANCZOS)

canvas = Image.new("RGB", (1024, 1024), (3, 3, 3))
canvas.paste(im2, (512 - im2.width // 2, 512 - im2.height // 2))
canvas.save(out)
print(f"saved {out} (disk ~{R*2}px of 1024)")