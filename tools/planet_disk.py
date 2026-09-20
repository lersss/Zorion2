# planet_disk.py — точный поиск диска планеты (центр масс яркости, радиус) и палитра внутри.
import sys
from PIL import Image

im = Image.open(sys.argv[1]).convert("RGB")
w, h = im.size
px = im.load()
print(f"size: {w}x{h}")

# 1. Центр масс ярких пикселей (освещённая часть диска)
pts = []
step = 4
for y in range(0, h, step):
    for x in range(0, w, step):
        p = px[x, y]
        if sum(p) > 330:  # ярко — освещённая поверхность/звезда
            pts.append((x, y, sum(p)))
if not pts:
    print("no bright pixels"); sys.exit()
sx = sum(p[0] for p in pts) / len(pts)
sy = sum(p[1] for p in pts) / len(pts)
cx, cy = int(sx), int(sy)
print(f"bright centroid: ({cx},{cy})")

# 2. Радиус: среднее расстояние до границы. Найдём max расстояние от центра до яркого пикселя
maxr = max(((p[0]-cx)**2 + (p[1]-cy)**2) ** 0.5 for p in pts)
print(f"max bright radius: {maxr:.0f}px")

# 3. Палитра внутри диска радиусом maxr (но не считая фон)
counts = {"blue": 0, "green": 0, "white": 0, "tan": 0, "red": 0, "dark": 0, "other": 0}
total = 0
step2 = 5
for y in range(cy - int(maxr), cy + int(maxr), step2):
    for x in range(cx - int(maxr), cx + int(maxr), step2):
        if 0 <= x < w and 0 <= y < h and (x - cx) ** 2 + (y - cy) ** 2 <= maxr * maxr:
            p = px[x, y]
            s = sum(p)
            total += 1
            if s < 120:
                counts["dark"] += 1
            elif p[2] > p[0] + 25 and p[2] > p[1] + 15:
                counts["blue"] += 1
            elif p[1] > p[0] + 20 and p[1] > p[2] + 15:
                counts["green"] += 1
            elif p[0] > 200 and p[1] > 200 and p[2] > 200:
                counts["white"] += 1
            elif p[0] > 150 and p[0] > p[1] + 25 and p[0] > p[2] + 25:
                counts["tan"] += 1
            elif p[0] > 110 and p[0] > p[2] + 45 and p[1] < p[0] - 15:
                counts["red"] += 1
            else:
                counts["other"] += 1
print("palette in disk (incl. night side as dark):")
for k, v in counts.items():
    print(f"  {k}: {v / max(total,1):.2%}")