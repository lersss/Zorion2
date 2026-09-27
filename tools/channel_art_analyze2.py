# channel_art_analyze2.py — точная геометрия диска (P1) и планеты (P4).
import numpy as np
from PIL import Image
import cv2

def satval(im):
    hsv = cv2.cvtColor(np.array(im.convert("RGB")), cv2.COLOR_RGB2HSV)
    return hsv[:, :, 1].astype(int), hsv[:, :, 2].astype(int)

# P1: диск = крупнейший связный насыщенный блоб
im = Image.open(r"C:\Zorion2\tools\e2e\artifacts\arrow-galaxy.png").convert("RGB")
s, v = satval(im)
m = ((s > 70) & (v > 45)).astype(np.uint8)
m = cv2.morphologyEx(m, cv2.MORPH_CLOSE, np.ones((25, 25), np.uint8))
n, lab, st, ce = cv2.connectedComponentsWithStats(m, 8)
k = 1 + int(np.argmax(st[1:, 4]))
x, y, w, hh, area = st[k]
cx, cy = ce[k]
R = (w + hh) / 4.0
print("P1 disc bbox=(%d,%d,%d,%d) c=(%.0f,%.0f) R~%.0f  (w=%d h=%d)" % (x, y, w, hh, cx, cy, R, w, hh))

# подписи внутри диска: яркие малонасыщенные пиксели в круге
yy, xx = np.mgrid[0:im.height, 0:im.width]
incirc = ((xx - cx) ** 2 + (yy - cy) ** 2) < (R * 0.97) ** 2
txt = (v > 140) & (s < 80) & incirc
print("P1 label-ish pixels inside disc =", int(txt.sum()), "median val=%.0f" % np.median(v[txt]) if txt.sum() else "")

# P4: планета — круглый яркий объект. Кандидаты: яркий bbox 405..778, 398..725
im4 = Image.open(r"C:\Zorion2\ai_drafts\planet_concepts\A_orbital.png").convert("RGB")
a4 = np.array(im4).astype(int)
lum = a4.max(axis=2)
bright = (lum > 110).astype(np.uint8)
bright = cv2.morphologyEx(bright, cv2.MORPH_CLOSE, np.ones((15, 15), np.uint8))
n4, lab4, st4, ce4 = cv2.connectedComponentsWithStats(bright, 8)
for k in sorted(range(1, n4), key=lambda k: -st4[k, 4])[:4]:
    x, y, w, hh, area = st4[k]
    print("P4 bright blob area=%6d bbox=(%d,%d,%d,%d) c=(%.0f,%.0f)" % (area, x, y, w, hh, ce4[k][0], ce4[k][1]))
# квадратный артефакт: ищем рамку по градиенту яркости — просто проверим профиль по строкам/столбцам
print("P4 mean lum by col (step64):", [int(lum[:, i * 64:i * 64 + 64].mean()) for i in range(16)])
print("P4 mean lum by row (step64):", [int(lum[i * 64:i * 64 + 64, :].mean()) for i in range(16)])
