# channel_art_twitch_extra.py — аватар 256x256 и заставка оффлайна 1920x1080 для Twitch.
# Стиль/палитра — из общего комплекта (channel_art_common). Выход: ai_drafts/channel_art/twitch/.
import numpy as np, cv2, sys, os, math
from PIL import Image, ImageDraw, ImageFilter
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from channel_art_common import PAL, grade_space, vignette, add_stars

DBG = r"C:\Zorion2\ai_drafts\channel_art"
OUTDIR = os.path.join(DBG, "twitch")
os.makedirs(OUTDIR, exist_ok=True)
CLEAN = np.array(Image.open(os.path.join(DBG, "p4_noring.png")).convert("RGB"))
FIXED = np.array(Image.open(os.path.join(DBG, "_p4_twitch_hi.png")).convert("RGB"))
PCX, PCY, PR = 586, 327, 194       # центр/радиус диска в p4_noring.png


def soft_circle_alpha(size, cx, cy, r_in, r_out):
    yy, xx = np.mgrid[0:size, 0:size]
    d = np.sqrt((xx - cx) ** 2 + (yy - cy) ** 2)
    return np.clip((r_out - d) / max(r_out - r_in, 1), 0, 1)


# ================= АВАТАР 256x256 =================
def avatar():
    S = 256
    rng = np.random.default_rng(4)
    base = np.zeros((S, S, 3), np.float32)
    # мягкий радиальный градиент + бирюзовая дымка за планетой
    yy, xx = np.mgrid[0:S, 0:S]
    d = np.sqrt((xx - 128) ** 2 + (yy - 128) ** 2) / 150.0
    base[:] = np.array(PAL["far"], float)
    base *= (1 - np.clip(d, 0, 1))[..., None] * 0.5 + 0.5
    halo = np.clip(1.0 - d, 0, 1)[..., None] * np.array(PAL["teal"], float) * 0.22
    base = base + halo
    base = add_stars(base, count=140, seed=9, max_lum=180)

    # планета: кроп 2*Hc, вписан так, чтобы атмосфера уложилась в круг R~98
    Hc = 298
    crop = CLEAN[PCY - Hc:PCY + Hc, PCX - Hc:PCX + Hc]
    pl = Image.fromarray(crop).resize((S, S), Image.LANCZOS)
    pl = np.array(pl).astype(np.float32)
    alpha = soft_circle_alpha(S, 128, 128, 99, 150)[..., None]
    comp = base * (1 - alpha) + pl * alpha
    out = vignette(comp, strength=0.35, power=2.0)
    Image.fromarray(np.clip(out, 0, 255).astype(np.uint8)).save(os.path.join(OUTDIR, "avatar_256.png"))
    print("avatar_256.png")


# ================= ЗАСТАВКА 1920x1080 =================
def banner():
    W, H = 1920, 1080
    yy, xx = np.mgrid[0:H, 0:W]
    # фон: тёмный космос + дымка к центру
    base = np.zeros((H, W, 3), np.float32)
    base[:] = np.array(PAL["space"], float)
    d = np.sqrt(((xx - W * 0.5) / (W * 0.62)) ** 2 + ((yy - H * 0.5) / (H * 0.75)) ** 2)
    haze = np.clip(1.0 - d, 0, 1)[..., None]
    base = base + haze * np.array(PAL["haze"], float) * 0.30
    base = base + haze * np.array(PAL["teal"], float) * 0.05
    base = add_stars(base, count=1100, seed=21, max_lum=210)

    canvas = Image.fromarray(np.clip(base, 0, 255).astype(np.uint8)).convert("RGB")

    # --- мозаика галактики слева (полный диск), уходит в глубину ---
    mos = Image.open(os.path.join(DBG, "_p1_disk_hi.png")).convert("RGBA")
    mh, mw = mos.size
    scale = 700.0 / mw
    ms = mos.resize((int(mw * scale), int(mh * scale)), Image.LANCZOS)
    a = np.array(ms).astype(np.float32)
    mh2, mw2 = a.shape[:2]
    yy2, xx2 = np.mgrid[0:mh2, 0:mw2]
    radn = np.sqrt((xx2 - mw2 / 2) ** 2 + (yy2 - mh2 / 2) ** 2) / (mw2 / 2)
    depth = np.clip(1.0 - 0.55 * np.clip(radn, 0, 1) ** 2, 0.30, 1.0)
    a[:, :, :3] = a[:, :, :3] * depth[..., None]
    layer = Image.fromarray(np.clip(a, 0, 255).astype(np.uint8), "RGBA")
    DCX, DCY = 520, 555
    canvas.paste(layer, (int(DCX - mw2 / 2), int(DCY - mh2 / 2)), layer)

    # --- орбиты: тонкие эллипсы через центр ---
    ov = Image.new("RGBA", (W, H), (0, 0, 0, 0))
    od = ImageDraw.Draw(ov)
    od.ellipse([980 - 700, 540 - 230, 980 + 700, 540 + 230], outline=PAL["agcyan"] + (110,), width=2)
    od.ellipse([980 - 900, 540 - 305, 980 + 900, 540 + 305], outline=PAL["teal"] + (85,), width=2)
    canvas = Image.alpha_composite(canvas.convert("RGBA"), ov).convert("RGB")

    # --- мелкая планета в кольце справа (из принятой панели 4, кольцо уже за диском) ---
    crop = FIXED[80:540, 70:1010]
    ph, pw = crop.shape[:2]
    tw = 600
    planet = Image.fromarray(crop).resize((tw, int(ph * tw / pw)), Image.LANCZOS)
    pn = np.array(planet).astype(np.float32)
    phh, pww = pn.shape[:2]
    yy3, xx3 = np.mgrid[0:phh, 0:pww]
    edge = np.minimum(np.minimum(xx3, pww - 1 - xx3), np.minimum(yy3, phh - 1 - yy3))
    lum3 = pn.max(axis=2)
    palpha = np.clip(edge / 50.0, 0, 1) * np.clip((lum3 - 26.0) / 22.0, 0, 1)
    player = Image.fromarray(np.clip(pn, 0, 255).astype(np.uint8)).convert("RGBA")
    player.putalpha(Image.fromarray((palpha * 255).astype(np.uint8)))
    canvas.paste(player, (1370 - pww // 2, 545 - phh // 2), player)

    # --- узлы-искры на орбите (рифма с панелью «граф») ---
    from channel_art_common import glow_points
    pts_agent = [(700, 400, 1.0), (1180, 470, 0.9), (1330, 760, 0.85)]
    pts_human = [(940, 300, 1.0), (1560, 560, 0.9)]
    canvas = glow_points(canvas, pts_agent, PAL["agent"], radius=3.0, intensity=1.0)
    canvas = glow_points(canvas, pts_human, PAL["human"], radius=3.2, intensity=1.0)

    arr = vignette(np.array(canvas).astype(float), strength=0.50, power=2.1)
    Image.fromarray(np.clip(arr, 0, 255).astype(np.uint8)).save(
        os.path.join(OUTDIR, "offline_banner_1920x1080.png"))
    print("offline_banner_1920x1080.png")


if __name__ == "__main__":
    avatar()
    banner()
