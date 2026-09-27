# channel_art_analyze3.py — диск P1 через профиль плотности насыщенных пикселей.
import numpy as np
from PIL import Image
import cv2

im = Image.open(r"C:\Zorion2\tools\e2e\artifacts\arrow-galaxy.png").convert("RGB")
a = np.array(im)
hsv = cv2.cvtColor(a, cv2.COLOR_RGB2HSV)
s, v = hsv[:, :, 1].astype(int), hsv[:, :, 2].astype(int)
colored = (s > 70) & (v > 45)
rows = colored.sum(axis=1)
cols = colored.sum(axis=0)
# диск: полоса, где цветных пикселей много (ячейки идут плотно)
th = 60
r_on = np.where(rows > th)[0]
c_on = np.where(cols > th)[0]
print("P1 rows>60 : %d..%d" % (r_on.min(), r_on.max()))
print("P1 cols>60 : %d..%d" % (c_on.min(), c_on.max()))
# уточнить по половине максимума
rmax = rows.max(); cmax = cols.max()
r_half = np.where(rows > rmax * 0.30)[0]
c_half = np.where(cols > cmax * 0.30)[0]
print("P1 rows>30%%: %d..%d (max %d)" % (r_half.min(), r_half.max(), rmax))
print("P1 cols>30%%: %d..%d (max %d)" % (c_half.min(), c_half.max(), cmax))
cy = (r_half.min() + r_half.max()) / 2
cx = (c_half.min() + c_half.max()) / 2
print("P1 disc center ~ (%.0f, %.0f)  size=(%d x %d)" % (cx, cy, c_half.max() - c_half.min(), r_half.max() - r_half.min()))
print("rows sample:", [int(rows[i]) for i in range(0, 800, 50)])

# самые насыщенные (ячейки) — границы диска по 5/95 перцентилю координат
ys, xs = np.where(colored)
print("P1 colored p5..p95 x: %d..%d  y: %d..%d" % (np.percentile(xs, 5), np.percentile(xs, 95), np.percentile(ys, 5), np.percentile(ys, 95)))

# P4 планета: bbox диска (из прошлого прогона) 411..778 x, 408..698 y
# найдём точнее — блоб после открытия
im4 = Image.open(r"C:\Zorion2\ai_drafts\planet_concepts\A_orbital.png").convert("RGB")
a4 = np.array(im4)
lum = a4.astype(int).max(axis=2)
b = (lum > 110).astype(np.uint8)
b = cv2.morphologyEx(b, cv2.MORPH_OPEN, np.ones((9, 9), np.uint8))
n, lab, st, ce = cv2.connectedComponentsWithStats(b, 8)
for k in sorted(range(1, n), key=lambda k: -st[k, 4])[:4]:
    x, y, w, hh, area = st[k]
    print("P4 open blob area=%6d bbox=(%d,%d,%d,%d) c=(%.0f,%.0f) R~%.0f" % (area, x, y, w, hh, ce[k][0], ce[k][1], (w + hh) / 4))
