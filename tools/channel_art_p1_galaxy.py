# channel_art_p1_galaxy.py — панель 1 "Об игре": сборка кадра карты миров.
# Источник: tools/e2e/artifacts/arrow-galaxy.png (скриншот игры). Только чтение.
# Кадрируем диск Вороного (bleeds за верх/низ), убираем подписи регионов и UI, виньетка, 768x432.
import numpy as np
import cv2
from PIL import Image
import sys, os
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from channel_art_common import grade_space, vignette, add_stars, MASTER

SRC = r"C:\Zorion2\tools\e2e\artifacts\arrow-galaxy.png"
OUTDIR = r"C:\Zorion2\ai_drafts\channel_art"

CX, CY = 844, 362          # центр диска
CW, CH = 730, 410          # кроп: диск (610x505) выходит за верх/низ, по бокам воздух ~60px

im = Image.open(SRC).convert("RGB")
a = np.array(im)
hsv = cv2.cvtColor(a, cv2.COLOR_RGB2HSV)
s, v = hsv[:, :, 1].astype(int), hsv[:, :, 2].astype(int)
lum = a.max(axis=2).astype(np.uint8)

# --- маска подписей: тонкие светлые малонасыщенные штрихи + светлый текст ---
k = cv2.getStructuringElement(cv2.MORPH_ELLIPSE, (9, 9))
tophat = cv2.subtract(lum, cv2.morphologyEx(lum, cv2.MORPH_OPEN, k))
m_thin = (tophat > 13) & (s < 72)
m_bright = (v > 118) & (s < 92)
lab_mask = (m_thin | m_bright).astype(np.uint8)
n, lab, st, ce = cv2.connectedComponentsWithStats(lab_mask, 8)
keep = np.zeros_like(lab_mask)
for kk in range(1, n):
    x, y, w, hh, area = st[kk]
    if area >= 3:
        keep[lab == kk] = 1

# --- UI-кнопки зума справа (три круглые кнопки) ---
ui = np.zeros_like(keep)
for cyb in (466, 516, 562):
    cv2.circle(ui, (1110, cyb), 25, 1, -1)

mask = np.clip(keep + ui, 0, 1).astype(np.uint8)
mask = cv2.dilate(mask, np.ones((3, 3), np.uint8), 1)
print("mask pixels=%d (label=%d ui=%d)" % (mask.sum(), keep.sum(), ui.sum()))

clean = cv2.inpaint(a, mask, 4, cv2.INPAINT_TELEA)

box = (CX - CW // 2, CY - CH // 2, CX + CW // 2, CY + CH // 2)
Image.fromarray(clean).crop(box).save(os.path.join(OUTDIR, "p1_crop_raw.png"))
Image.fromarray(a).crop(box).save(os.path.join(OUTDIR, "p1_crop_before.png"))

fin = Image.fromarray(clean).crop(box).resize(MASTER, Image.LANCZOS)
arr = np.array(fin).astype(float)
arr = grade_space(arr, ambient=0.12)
arr = add_stars(arr, count=110, seed=11, max_lum=150)
arr = vignette(arr, strength=0.55, power=2.2)
Image.fromarray(np.clip(arr, 0, 255).astype(np.uint8)).save(os.path.join(OUTDIR, "01_about_game.png"))
print("saved", Image.open(os.path.join(OUTDIR, "01_about_game.png")).size)
