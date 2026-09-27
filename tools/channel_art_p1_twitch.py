# channel_art_p1_twitch.py — панель 1 "Об игре" для Twitch: усиленная мозаика, без светлого пятна.
# Источник (только чтение): tools/e2e/artifacts/arrow-galaxy.png.
# Выход: twitch/panel_01_about_game.png (320x160) + _p1_disk_hi.png (полный диск для заставки).
import numpy as np, cv2, sys, os
from PIL import Image, ImageFilter
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from channel_art_common import grade_space, vignette, add_stars

SRC = r"C:\Zorion2\tools\e2e\artifacts\arrow-galaxy.png"
OUTDIR = r"C:\Zorion2\ai_drafts\channel_art\twitch"
DBG = r"C:\Zorion2\ai_drafts\channel_art"
os.makedirs(OUTDIR, exist_ok=True)
CX, CY = 844, 362
SRCIMG = np.array(Image.open(SRC).convert("RGB"))


def enhance(crop, ui_centers, mask_ui=True):
    H, W = crop.shape[:2]
    hsv = cv2.cvtColor(crop, cv2.COLOR_RGB2HSV)
    s, v = hsv[:, :, 1].astype(int), hsv[:, :, 2].astype(int)
    lum = crop.max(axis=2).astype(np.uint8)
    k = cv2.getStructuringElement(cv2.MORPH_ELLIPSE, (9, 9))
    tophat = cv2.subtract(lum, cv2.morphologyEx(lum, cv2.MORPH_OPEN, k))
    label = (((tophat > 13) & (s < 72)) | ((v > 118) & (s < 92))).astype(np.uint8)

    # UI: кнопки зума
    ui = np.zeros_like(label)
    if mask_ui:
        for (cx, cy) in ui_centers:
            cv2.circle(ui, (cx, cy), 28, 1, -1)

    mask = np.clip(label + ui, 0, 1).astype(np.uint8)
    mask = cv2.dilate(mask, np.ones((3, 3), np.uint8), 1)
    clean = cv2.inpaint(crop, mask, 4, cv2.INPAINT_TELEA)

    # диск мозаики (по уже очищенному изображению)
    hsv_c = cv2.cvtColor(clean, cv2.COLOR_RGB2HSV)
    sc, vc = hsv_c[:, :, 1].astype(int), hsv_c[:, :, 2].astype(int)
    mos = ((sc > 90) & (vc > 35)).astype(np.uint8)
    mos = cv2.morphologyEx(mos, cv2.MORPH_CLOSE, np.ones((31, 31), np.uint8))
    n, lab, st, _ = cv2.connectedComponentsWithStats(mos, 8)
    kk = 1 + int(np.argmax(st[1:, 4]))
    disk = np.zeros_like(mos); disk[lab == kk] = 1
    disk = cv2.morphologyEx(disk, cv2.MORPH_OPEN, np.ones((61, 61), np.uint8))  # срезать тонкие привески/UI
    n, lab, st, _ = cv2.connectedComponentsWithStats(disk, 8)
    kk = 1 + int(np.argmax(st[1:, 4]))
    disk = np.zeros_like(disk); disk[lab == kk] = 1
    disk = cv2.morphologyEx(disk, cv2.MORPH_CLOSE, np.ones((31, 31), np.uint8))

    # мягкое светлое пятно ретуши вне диска
    lumf = clean.max(axis=2).astype(float)
    glow = cv2.GaussianBlur(np.clip(lumf - 24, 0, None), (0, 0), 12)
    hif = np.abs(lumf - cv2.GaussianBlur(lumf, (0, 0), 2))
    w = np.clip(1.0 - hif / 14.0, 0, 1)
    outd = np.clip(cv2.GaussianBlur((1 - disk).astype(np.float32), (0, 0), 6) * 1.6, 0, 1)
    clean = np.clip(clean.astype(float) - (w * outd * glow)[..., None] * 0.95, 0, 255).astype(np.uint8)

    # усиление заливки ячеек
    hsv2 = cv2.cvtColor(clean, cv2.COLOR_RGB2HSV).astype(float)
    hsv2[:, :, 1] = np.clip(hsv2[:, :, 1] * 1.30 + 8, 0, 255)
    hsv2[:, :, 2] = np.clip(hsv2[:, :, 2] * 1.15 + 8, 0, 255)
    vivid = cv2.cvtColor(hsv2.astype(np.uint8), cv2.COLOR_HSV2RGB).astype(float)

    g = cv2.cvtColor(clean, cv2.COLOR_RGB2GRAY)
    th = cv2.subtract(g, cv2.morphologyEx(g, cv2.MORPH_OPEN, cv2.getStructuringElement(cv2.MORPH_ELLIPSE, (7, 7))))
    bloom = cv2.GaussianBlur(np.clip(th.astype(float) * 1.9, 0, 255), (0, 0), 3.0)
    vivid = vivid + bloom[..., None] * 0.55
    arr = np.clip(((vivid / 255.0) - 0.5) * 1.16 + 0.5, 0, 1) * 255.0
    arr = grade_space(arr, ambient=0.10)
    arr = add_stars(arr, count=max(120, W // 3), seed=11, max_lum=185)
    return vignette(arr, strength=0.42, power=2.2), disk


# ---- панель 2:1 ----
CW, CH = 730, 410
crop = SRCIMG[CY - CH // 2:CY + CH // 2, CX - CW // 2:CX + CW // 2]
arr, _ = enhance(crop, [(1110 - (CX - CW // 2), y) for y in (309, 359, 405)])
hi = Image.fromarray(np.clip(arr, 0, 255).astype(np.uint8))
y0 = (CH - CW // 2) // 2
sq = hi.crop((0, y0, CW, y0 + CW // 2))
sq.resize((320, 160), Image.LANCZOS).filter(
    ImageFilter.UnsharpMask(radius=1.0, percent=70, threshold=2)).save(
    os.path.join(OUTDIR, "panel_01_about_game.png"))
print("panel_01 ok")

# ---- полный диск (для заставки), с прозрачностью по маске диска ----
SQ = 700
crop2 = SRCIMG[CY - SQ // 2:CY + SQ // 2, CX - SQ // 2:CX + SQ // 2]
arr2, disk2 = enhance(crop2, [(1110 - (CX - SQ // 2), y) for y in (309, 359, 405)], mask_ui=False)
# гарантированно убрать из альфы области интерфейса карты (верх/право/низ)
disk2[:100, :] = 0
disk2[:, 610:] = 0
disk2[-50:, :] = 0
alpha = cv2.GaussianBlur(disk2.astype(np.float32), (0, 0), 14)
alpha = np.clip(alpha * 1.35, 0, 1)
rgba = np.dstack([np.clip(arr2, 0, 255).astype(np.uint8),
                  (alpha * 255).astype(np.uint8)])
Image.fromarray(rgba, "RGBA").save(os.path.join(DBG, "_p1_disk_hi.png"))
print("disk asset ok")
