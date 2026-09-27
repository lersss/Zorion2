# channel_art_analyze.py — замеры геометрии баз для 4 панелей оформления канала.
# Только чтение исходников, ничего не пишет.
import numpy as np
from PIL import Image
import cv2

def hsv(img):
    return cv2.cvtColor(np.array(img.convert("RGB")), cv2.COLOR_RGB2HSV)

# ---- Panel 1: arrow-galaxy.png ----
im = Image.open(r"C:\Zorion2\tools\e2e\artifacts\arrow-galaxy.png").convert("RGB")
a = np.array(im)
h = hsv(im)
sat = h[:, :, 1].astype(int)
val = h[:, :, 2].astype(int)
# "цветные" ячейки: заметная насыщенность
colored = (sat > 60) & (val > 40)
ys, xs = np.where(colored)
print("P1 colored bbox x[%d..%d] y[%d..%d]" % (xs.min(), xs.max(), ys.min(), ys.max()))
print("P1 colored centroid (%.0f, %.0f)  count=%d" % (xs.mean(), ys.mean(), colored.sum()))
# основные ряды/колонки: плотность цветных по строкам/столбцам
rows = colored.sum(axis=1); cols = colored.sum(axis=0)
r_idx = np.where(rows > colored.shape[1] * 0.02)[0]
c_idx = np.where(cols > colored.shape[0] * 0.02)[0]
print("P1 dense rows %d..%d  dense cols %d..%d" % (r_idx.min(), r_idx.max(), c_idx.min(), c_idx.max()))
# яркие малонасыщенные пиксели = подписи (белёсый текст) внутри диска
bright_text = (val > 150) & (sat < 70)
print("P1 bright-low-sat pixels (labels?) =", int(bright_text.sum()))

# ---- Panel 3: belt-ice-scene-mock.png ----
im3 = Image.open(r"C:\Zorion2\tools\e2e\artifacts\belt-ice-scene-mock.png").convert("RGB")
a3 = np.array(im3)
lum3 = a3.max(axis=2)
mask3 = lum3 > 25
ys3, xs3 = np.where(mask3)
print("P3 content bbox x[%d..%d] y[%d..%d]" % (xs3.min(), xs3.max(), ys3.min(), ys3.max()))
# плотность по сетке 8x8
H, W = mask3.shape
gy, gx = 6, 8
for j in range(gy):
    row = ""
    for i in range(gx):
        blk = mask3[j*H//gy:(j+1)*H//gy, i*W//gx:(i+1)*W//gx]
        row += "%5.2f " % blk.mean()
    print("P3 grid", row)
# крупные блобы (по площади) центры
n, lab, stats, cent = cv2.connectedComponentsWithStats(mask3.astype(np.uint8), 8)
blobs = sorted(range(1, n), key=lambda k: -stats[k, 4])[:12]
for k in blobs:
    x, y, w, hh, area = stats[k]
    print("P3 blob area=%6d bbox=(%4d,%4d,%4d,%4d) c=(%.0f,%.0f)" % (area, x, y, w, hh, cent[k][0], cent[k][1]))

# ---- Panel 4: A_orbital.png ----
im4 = Image.open(r"C:\Zorion2\ai_drafts\planet_concepts\A_orbital.png").convert("RGB")
a4 = np.array(im4)
h4 = hsv(im4)
# планета: яркий объект; ищем "тёплые" пиксели (R>G>B) и бирюзу
R, G, B = a4[:, :, 0].astype(int), a4[:, :, 1].astype(int), a4[:, :, 2].astype(int)
warm = (R > G + 8) & (G > B + 8) & (R > 80)
teal = (G > R + 12) & (B > R + 8) & (G > 90)
bright = a4.max(axis=2) > 90
for nm, m in [("warm", warm), ("teal", teal), ("bright", bright)]:
    if m.sum() > 0:
        ys4, xs4 = np.where(m)
        print("P4 %s bbox x[%d..%d] y[%d..%d] count=%d" % (nm, xs4.min(), xs4.max(), ys4.min(), ys4.max(), m.sum()))
# диск планеты = крупнейший яркий блоб
n4, lab4, st4, ce4 = cv2.connectedComponentsWithStats(bright.astype(np.uint8), 8)
b4 = sorted(range(1, n4), key=lambda k: -st4[k, 4])[:6]
for k in b4:
    x, y, w, hh, area = st4[k]
    print("P4 blob area=%6d bbox=(%4d,%4d,%4d,%4d) c=(%.0f,%.0f)" % (area, x, y, w, hh, ce4[k][0], ce4[k][1]))
