# planet_scan.py — карта яркости по кадру + грубая палитра в 4 квадрантах диска.
import sys
from PIL import Image

im = Image.open(sys.argv[1]).convert("RGB")
w, h = im.size
px = im.load()
print(f"size: {w}x{h}")

# Сетка 8x8 яркости (сумма RGB)
print("brightness grid (8x8):")
for gy in range(8):
    row = []
    for gx in range(8):
        s = 0
        for y in range(gy * h // 8, (gy + 1) * h // 8, 16):
            for x in range(gx * w // 8, (gx + 1) * w // 8, 16):
                p = px[x, y]
                s += sum(p)
        row.append(f"{s // 1000:3d}")
    print("  " + " ".join(row))

# Найдём диск: столбец/строка с максимальной яркостью -> центр и радиус
# Яркость по вертикальной линии через центр
def line_bright(x, y0, y1):
    s = 0
    n = 0
    for y in range(y0, y1, 8):
        s += sum(px[x, y]); n += 1
    return s // max(n, 1)

def col_bright(y, x0, x1):
    s = 0
    n = 0
    for x in range(x0, x1, 8):
        s += sum(px[x, y]); n += 1
    return s // max(n, 1)

best_col = max(range(0, w, 32), key=lambda x: line_bright(x, h // 4, 3 * h // 4))
best_row = max(range(0, h, 32), key=lambda y: col_bright(y, w // 4, 3 * w // 4))
print(f"brightest center: col={best_col}, row={best_row}")

# Радиус: от центра идём по строке, пока яркость > порога
cx, cy = best_col, best_row
bg = 200
# ищем границу вправо
r = 0
for x in range(cx, w):
    if sum(px[x, cy]) > 400:
        r = x - cx
print(f"approx disk radius right: {r}px (of {w})")

# Палитра внутри диска радиусом r (сэмпл)
counts = {"blue": 0, "green": 0, "white": 0, "tan": 0, "red": 0, "other": 0}
total = 0
step = 6
for y in range(cy - r, cy + r, step):
    for x in range(cx - r, cx + r, step):
        if (x - cx) ** 2 + (y - cy) ** 2 > r * r:
            continue
        p = px[x, y]
        total += 1
        if p[2] > p[0] + 25 and p[2] > p[1] + 15:
            counts["blue"] += 1
        elif p[1] > p[0] + 20 and p[1] > p[2] + 15:
            counts["green"] += 1
        elif p[0] > 210 and p[1] > 210 and p[2] > 210:
            counts["white"] += 1
        elif p[0] > 150 and p[0] > p[1] + 25 and p[0] > p[2] + 25:
            counts["tan"] += 1
        elif p[0] > 120 and p[0] > p[2] + 40 and p[1] < p[0] - 20:
            counts["red"] += 1
        else:
            counts["other"] += 1
print("palette inside disk:")
for k, v in counts.items():
    print(f"  {k}: {v / max(total,1):.2%}")