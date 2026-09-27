# channel_art_p4_twitch.py — панель 4 "Поддержать разработку": кольцо уходит ЗА диск планеты.
# Источник: ai_drafts/channel_art/p4_form_6006.png (Comfy). Выход: .../twitch/panel_04_support.png (320x160)
# + чистый диск планеты без кольца (для аватара и заставки): ai_drafts/channel_art/_p4_planet_clean.png
import numpy as np, cv2, sys, os, math
from PIL import Image, ImageFilter
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from channel_art_common import grade_space, vignette, add_stars, glow_points, PAL

SRC = r"C:\Zorion2\ai_drafts\channel_art\p4_form_6006.png"
OUTDIR = r"C:\Zorion2\ai_drafts\channel_art\twitch"
DBG = r"C:\Zorion2\ai_drafts\channel_art"
os.makedirs(OUTDIR, exist_ok=True)
TW, TH = 320, 160
PCX, PCY, PR = 590, 333, 188          # диск планеты
ECX, ECY, A_AX, B_AX, ANG = 536, 273, 434, 122, 16.7  # эллипс кольца (для стороны дуги)
RING_ELL = ((529.1, 266.5), (233.5, 856.1), 108.0)    # эллипс кольца (cv2.fitEllipse по видимым точкам)


def ring_line_mask(img, inner_r=None):
    g = cv2.cvtColor(img, cv2.COLOR_RGB2GRAY)
    k = cv2.getStructuringElement(cv2.MORPH_ELLIPSE, (11, 11))
    line = np.maximum(cv2.subtract(g, cv2.morphologyEx(g, cv2.MORPH_OPEN, k)),
                      cv2.subtract(cv2.morphologyEx(g, cv2.MORPH_CLOSE, k), g))
    H, W = g.shape
    yy, xx = np.mgrid[0:H, 0:W]
    R = PR - 24 if inner_r is None else inner_r
    inner = ((xx - PCX) ** 2 + (yy - PCY) ** 2) < R ** 2
    m = ((line > 14) & inner).astype(np.uint8)
    n, lab, st, _ = cv2.connectedComponentsWithStats(m, 8)
    keep = np.zeros_like(m)
    for i in range(1, n):
        if st[i, 4] >= 40:
            keep[lab == i] = 1
    return keep


def inpaint_ring(img, far_only):
    keep = ring_line_mask(img)
    if far_only:
        th = math.radians(ANG)
        nx, ny = -math.sin(th), math.cos(th)   # нормаль к ближней дуге
        ys, xs = np.where(keep > 0)
        side = (xs - ECX) * nx + (ys - ECY) * ny
        m = np.zeros_like(keep)
        m[ys[side < -2], xs[side < -2]] = 1
    else:
        m = keep
    m = cv2.dilate(m, np.ones((7, 7), np.uint8), 1)
    return cv2.inpaint(img, m, 6, cv2.INPAINT_TELEA)


base = np.array(Image.open(SRC).convert("RGB"))
H, W = base.shape[:2]

# --- панель: убрать дальнюю дугу (передняя остаётся перед диском) ---
fixed = inpaint_ring(base, far_only=True)
arr = grade_space(fixed.astype(float), ambient=0.10)
img = Image.fromarray(np.clip(arr, 0, 255).astype(np.uint8))

# искры вокруг планеты: по ближней дуге кольца и пара "человеков"
th = math.radians(ANG)
u = np.array([math.cos(th), math.sin(th)]); v = np.array([-math.sin(th), math.cos(th)])
def ring_pt(t, rs=1.0):
    p = np.array([ECX, ECY]) + (A_AX * rs) * math.cos(t) * u + (B_AX * rs) * math.sin(t) * v
    return int(p[0]), int(p[1])
agents = [ring_pt(t, 1.0) + (s,) for t, s in
          [(0.30, 0.9), (0.62, 1.0), (1.05, 0.85), (1.55, 1.0), (2.1, 0.9), (2.7, 0.95), (3.35, 0.85)]]
humans = [ring_pt(1.9, 1.0) + (1.05,), ring_pt(4.0, 1.0) + (0.95,)]
img = glow_points(img, agents, PAL["agent"], radius=2.6, intensity=1.0)
img = glow_points(img, humans, PAL["human"], radius=2.9, intensity=1.0)
arr = vignette(np.array(img).astype(float), strength=0.48, power=2.0)

hi = Image.fromarray(np.clip(arr, 0, 255).astype(np.uint8))
hi.save(DBG + r"\_p4_twitch_hi.png")
y0 = (H - W // 2) // 2
sq = hi.crop((0, y0, W, y0 + W // 2))
small = sq.resize((TW, TH), Image.LANCZOS).filter(ImageFilter.UnsharpMask(radius=1.0, percent=60, threshold=2))
small.save(os.path.join(OUTDIR, "panel_04_support.png"))
print("panel_04", small.size)

# --- чистый диск планеты без кольца (для аватара/заставки) ---
def remove_ring_all(im):
    H, W = im.shape[:2]
    yy, xx = np.mgrid[0:H, 0:W]
    dist = np.sqrt((xx - PCX) ** 2 + (yy - PCY) ** 2)
    keep = ring_line_mask(im, inner_r=PR - 4)      # тонкая линия внутри диска (вкл. края)
    geo = np.zeros((H, W), np.uint8)
    cv2.ellipse(geo, RING_ELL, 1, 13)              # лента кольца — только ВНЕ диска
    geo = (geo.astype(bool) & (dist > PR - 6)).astype(np.uint8)
    m = np.clip(keep + geo, 0, 1).astype(np.uint8)
    m = cv2.dilate(m, np.ones((5, 5), np.uint8), 1)
    return cv2.inpaint(im, m, 4, cv2.INPAINT_TELEA)


clean = remove_ring_all(base)
Image.fromarray(clean).save(os.path.join(DBG, "_p4_planet_clean.png"))
print("_p4_planet_clean.png saved")
