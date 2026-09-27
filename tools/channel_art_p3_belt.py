# channel_art_p3_belt.py — панель 3 "Что на стриме": сборка кадра пояса астероидов.
# Источник: tools/e2e/artifacts/belt-ice-scene-mock.png (1600x900, без HUD/текста). Только чтение.
# Кадрируем центральный кластер, убираем мелкий обломочный мусор у краёв, пыль/дымка/виньетка, 768x432.
import numpy as np
import cv2
from PIL import Image, ImageFilter
import sys, os
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from channel_art_common import grade_space, vignette, add_stars, PAL, MASTER

SRC = r"C:\Zorion2\tools\e2e\artifacts\belt-ice-scene-mock.png"
OUTDIR = r"C:\Zorion2\ai_drafts\channel_art"

CX, CY, CW, CH = 675, 500, 1120, 630
BOX = (CX - CW // 2, CY - CH // 2, CX + CW // 2, CY + CH // 2)

im = Image.open(SRC).convert("RGB")
a = np.array(im)

# --- убрать мелкие обломки у краёв кропа ---
crop = a[BOX[1]:BOX[3], BOX[0]:BOX[2]].copy()
H, W = crop.shape[:2]
content = (crop.max(axis=2) > 22).astype(np.uint8)
n, lab, st, ce = cv2.connectedComponentsWithStats(content, 8)
removed = 0
MARGIN = 48
for k in range(1, n):
    x, y, w, h, area = st[k]
    near = x < MARGIN or y < MARGIN or x + w > W - MARGIN or y + h > H - MARGIN
    if near and area < 6000:
        crop[lab == k] = (0, 0, 0)
        removed += 1
print("removed edge debris components:", removed)

# --- фон в палитру + пыль + бирюзовая дымка + виньетка ---
fin = Image.fromarray(crop).resize(MASTER, Image.LANCZOS)
arr = np.array(fin).astype(float)
arr = grade_space(arr, ambient=0.10)
arr = add_stars(arr, count=420, seed=23, max_lum=165)

# бирюзовая дымка (лёгкая, для единства комплекта)
h, w = arr.shape[:2]
yy, xx = np.mgrid[0:h, 0:w]
d = np.sqrt(((xx - w * 0.52) / (w * 0.8)) ** 2 + ((yy - h * 0.55) / (h * 0.9)) ** 2)
haze = np.clip(1.0 - d, 0, 1)[..., None] * np.array(PAL["teal"], float) * 0.07
arr = arr + haze

arr = vignette(arr, strength=0.60, power=2.0)
out = Image.fromarray(np.clip(arr, 0, 255).astype(np.uint8))
out.save(os.path.join(OUTDIR, "03_on_stream.png"))
Image.fromarray(a[BOX[1]:BOX[3], BOX[0]:BOX[2]]).save(os.path.join(OUTDIR, "p3_crop_before.png"))
print("saved 03_on_stream.png", out.size)
