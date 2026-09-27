# channel_art_p3_twitch.py — панель 3 "Что на стриме": объём и подсветка краёв у астероидов.
# Источник (только чтение): tools/e2e/artifacts/belt-ice-scene-mock.png. Выход: .../twitch/panel_03_on_stream.png (320x160).
import numpy as np, cv2, sys, os
from PIL import Image, ImageFilter
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from channel_art_common import grade_space, vignette, add_stars, PAL

SRC = r"C:\Zorion2\tools\e2e\artifacts\belt-ice-scene-mock.png"
OUTDIR = r"C:\Zorion2\ai_drafts\channel_art\twitch"
DBG = r"C:\Zorion2\ai_drafts\channel_art"
os.makedirs(OUTDIR, exist_ok=True)
TW, TH = 320, 160

# кроп 2:1 центрального кластера
CX, CY, CW = 675, 500, 1120
a = np.array(Image.open(SRC).convert("RGB"))
y0 = CY - CW // 4
crop = a[y0:y0 + CW // 2, CX - CW // 2:CX + CW // 2].copy()
H, W = crop.shape[:2]

content = (crop.max(axis=2) > 22).astype(np.uint8)
# убрать мелкий обломочный мусор у краёв
n, lab, st, ce = cv2.connectedComponentsWithStats(content, 8)
removed = 0
MARGIN = 44
for k in range(1, n):
    x, y, w, h, area = st[k]
    near = x < MARGIN or y < MARGIN or x + w > W - MARGIN or y + h > H - MARGIN
    if near and area < 5500:
        crop[lab == k] = (0, 0, 0)
        removed += 1
content = (crop.max(axis=2) > 22).astype(np.uint8)
print("removed edge debris:", removed)

# ---------- объём: псевдо-нормали из distance transform ----------
yy, xx = np.mgrid[0:H, 0:W].astype(np.float32)
img = crop.astype(np.float32)
L = np.array([-0.52, -0.60, 0.61]); L /= np.linalg.norm(L)

n2, lab2, st2, ce2 = cv2.connectedComponentsWithStats(content, 8)
glow_layer = np.zeros((H, W, 3), np.float32)
base_light = img.copy()
for k in range(1, n2):
    if st2[k, 4] < 60:
        continue
    m = (lab2 == k).astype(np.uint8)
    d = cv2.distanceTransform(m, cv2.DIST_L2, 5).astype(np.float32)
    dmax = d.max()
    if dmax < 2:
        continue
    hgt = np.sqrt(np.clip(d / dmax, 0, 1))
    # сгладим высоту внутри компонента для плавных нормалей
    hblur = cv2.GaussianBlur(hgt, (0, 0), max(1.5, dmax * 0.18))
    gx = cv2.Sobel(hblur, cv2.CV_32F, 1, 0, ksize=5)
    gy = cv2.Sobel(hblur, cv2.CV_32F, 0, 1, ksize=5)
    sc = 2.4 / max(dmax, 6.0)
    nx, ny, nz = -gx * sc, -gy * sc, np.ones_like(hgt)
    nrm = np.sqrt(nx * nx + ny * ny + nz * nz)
    nx, ny, nz = nx / nrm, ny / nrm, nz / nrm
    ndl = np.clip(nx * L[0] + ny * L[1] + nz * L[2], 0, 1)
    shade = 0.34 + 0.95 * ndl
    # блик/подсветка края (fresnel)
    fres = np.clip((1.0 - nz) * 3.2, 0, 1) * ndl
    # цвет: лёд (синеватый) -> бирюзовый край, камень -> тёплый
    r, g, b = img[:, :, 0], img[:, :, 1], img[:, :, 2]
    ice = (b > r + 12)
    rim_col = np.where(ice[..., None], np.array(PAL["atmo_halo"], np.float32),
                       np.array(PAL["amber"], np.float32))
    region = m.astype(bool)
    shaded = img * shade[..., None]
    shaded = shaded + (fres * 0.45)[..., None] * rim_col * region[..., None]
    base_light[region] = shaded[region]
    # мягкое внешнее свечение вокруг компонента (слабое, чтобы не читалось как наклейка)
    halo = cv2.GaussianBlur(m.astype(np.float32), (0, 0), 9)
    halo = np.clip(halo * 1.4, 0, 1) * (1 - m)
    glow_layer += halo[..., None] * (rim_col * 0.16)

arr = np.clip(base_light + glow_layer, 0, 255)
arr = grade_space(arr, ambient=0.09)
# дальний план мягче: лёгкое размытие фона усиливает ощущение сцены
blur = cv2.GaussianBlur(arr.astype(np.float32), (0, 0), 1.2)
arr = arr * 0.75 + blur * 0.25
arr = add_stars(arr, count=300, seed=23, max_lum=170)
# бирюзовая дымка
h2, w2 = arr.shape[:2]
yy2, xx2 = np.mgrid[0:h2, 0:w2]
dd = np.sqrt(((xx2 - w2 * .52) / (w2 * .8)) ** 2 + ((yy2 - h2 * .55) / (h2 * .9)) ** 2)
arr = arr + np.clip(1.0 - dd, 0, 1)[..., None] * np.array(PAL["teal"], float) * 0.06
arr = vignette(arr, strength=0.48, power=2.1)
hi = Image.fromarray(np.clip(arr, 0, 255).astype(np.uint8))
hi.save(DBG + r"\_p3_twitch_hi.png")
small = hi.resize((TW, TH), Image.LANCZOS).filter(ImageFilter.UnsharpMask(radius=1.0, percent=60, threshold=2))
small.save(os.path.join(OUTDIR, "panel_03_on_stream.png"))
print("panel_03", small.size)
