# planet_same.py — сравнение расклада биомов двух картинок: сетка 6x6, класс клетки
# (blue/green/white/tan/red/dark/bright), доля совпадений.
# Использование: python planet_same.py <a.png> <b.png>
import sys
import numpy as np
from PIL import Image

def classify_cell(a, x0, y0, x1, y1):
    sub = a[y0:y1, x0:x1]
    r = sub[:, :, 0].astype(int)
    g = sub[:, :, 1].astype(int)
    b = sub[:, :, 2].astype(int)
    s = r + g + b
    n = sub.shape[0] * sub.shape[1]
    classes = {}
    if (b > r + 25).mean() > 0.35: classes["blue"] = (b > r + 25).mean()
    if (g > r + 20).mean() > 0.3: classes["green"] = (g > r + 20).mean()
    if (r > 200).mean() > 0.35 and (g > 200).mean() > 0.35 and (b > 200).mean() > 0.35: classes["white"] = (r > 200).mean()
    if (r > 150).mean() > 0.3 and (r > g + 25).mean() > 0.3 and (r > b + 25).mean() > 0.3: classes["tan"] = (r > 150).mean()
    if (r > 100).mean() > 0.25 and (r > b + 40).mean() > 0.25: classes["red"] = (r > 100).mean()
    if (s < 130).mean() > 0.6: classes["dark"] = (s < 130).mean()
    if classes:
        return max(classes, key=classes.get)
    return "bright" if s.mean() > 400 else "dark"

def grid(path):
    im = Image.open(path).convert("RGB")
    a = np.array(im)
    h, w = a.shape[:2]
    cx, cy = w // 2, h // 2
    R = 400
    yy, xx = np.ogrid[:h, :w]
    mask = (xx - cx) ** 2 + (yy - cy) ** 2 <= R * R
    a[~mask] = 0  # за пределами диска — фон
    cells = []
    for gy in range(6):
        row = []
        for gx in range(6):
            x0 = cx - R + gx * 2 * R // 6
            y0 = cy - R + gy * 2 * R // 6
            x1 = cx - R + (gx + 1) * 2 * R // 6
            y1 = cy - R + (gy + 1) * 2 * R // 6
            row.append(classify_cell(a, x0, y0, x1, y1))
        cells.append(row)
    return cells

def show(cells):
    for row in cells:
        print("  " + " ".join(f"{c[:4]:<4}" for c in row))

a = grid(sys.argv[1])
b = grid(sys.argv[2])
print("A:")
show(a)
print("B:")
show(b)
same = sum(1 for ra, rb in zip(a, b) for ca, cb in zip(ra, rb) if ca == cb)
total = 36
print(f"same cells: {same}/{total} = {same/total:.0%}")