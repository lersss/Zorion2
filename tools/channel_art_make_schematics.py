# channel_art_make_schematics.py — заготовки для панели 2 (граф) и 4 (планета+кольцо).
# Пишет p2_schematic.png (1024x576), p4_latent.png и p4_control.png (1024x576).
import os, math
import numpy as np
from PIL import Image, ImageDraw, ImageFilter
import sys
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from channel_art_common import PAL

OUT = r"C:\Zorion2\ai_drafts\channel_art"
W, H = 1024, 576


def bg_canvas():
    return Image.new("RGB", (W, H), PAL["space"])


# ---------- P2: инженерный чертёж ----------
def p2():
    img = bg_canvas()
    d = ImageDraw.Draw(img, "RGBA")
    # сетка (за кадром — на весь холст)
    step, gc = 56, PAL["haze"]
    for x in range(0, W + 1, step):
        d.line([(x, 0), (x, H)], fill=gc + (30,), width=1)
    for y in range(0, H + 1, step):
        d.line([(0, y), (W, y)], fill=gc + (30,), width=1)

    cx, cy, rxo, ryo = 512, 300, 530, 185
    # орбита
    d.ellipse([cx - rxo, cy - ryo, cx + rxo, cy + ryo], outline=PAL["agcyan"] + (170,), width=4)
    # пунктирные связи между узлами (по орбите)
    ang = [42, 95, 142, 222, 315]
    types = ["agent", "human", "agent", "core", "agent"]
    ntype = {"agent": PAL["agent"], "human": PAL["human"], "core": PAL["core"]}
    pts = [(cx + rxo * math.cos(math.radians(a)), cy - ryo * math.sin(math.radians(a))) for a in ang]
    edges = [(0, 1), (1, 2), (2, 3), (3, 4), (4, 0), (1, 3)]
    for (i, j) in edges:
        x1, y1 = pts[i]; x2, y2 = pts[j]
        L = math.hypot(x2 - x1, y2 - y1)
        nseg = max(2, int(L / 16))
        for s in range(nseg):
            t0 = s / nseg; t1 = t0 + 0.55 / nseg
            xa, ya = x1 + (x2 - x1) * t0, y1 + (y2 - y1) * t0
            xb, yb = x1 + (x2 - x1) * t1, y1 + (y2 - y1) * t1
            d.line([(xa, ya), (xb, yb)], fill=PAL["teal"] + (190,), width=2)

    img = img.filter(ImageFilter.GaussianBlur(0.6))
    # свечение узлов
    glow = Image.new("RGB", (W, H), (0, 0, 0))
    gd = ImageDraw.Draw(glow)
    order = [4, 1, 3, 2, 0]
    for i in order:
        x, y = pts[i]
        col = ntype[types[i]]
        for rr, al in ((46, 0.20), (26, 0.5), (12, 0.9), (6, 1.0)):
            gd.ellipse([x - rr, y - rr, x + rr, y + rr], fill=tuple(int(c * al) for c in col))
    glow = glow.filter(ImageFilter.GaussianBlur(10))
    img = Image.fromarray(np.clip(np.array(img).astype(int) + np.array(glow).astype(int) * 0.9, 0, 255).astype(np.uint8))
    img.save(os.path.join(OUT, "p2_schematic.png"))
    print("p2_schematic.png", img.size)


# ---------- P4: латент (база) и контроль (круг+кольцо) ----------
def p4():
    base = Image.open(r"C:\Zorion2\ai_drafts\planet_concepts\A_orbital.png").convert("RGB")
    sq = base.crop((240, 225, 780, 775))            # содержимое внутри шва
    sc = H / sq.height
    sq = sq.resize((int(sq.width * sc), int(sq.height * sc)), Image.LANCZOS)
    from channel_art_common import add_stars
    canvas = Image.fromarray(add_stars(np.array(bg_canvas(), float), count=340, seed=5, max_lum=170).astype(np.uint8))
    ox = (W - sq.width) // 2
    canvas.paste(sq, (ox, 0))
    Image.fromarray(np.array(canvas)).save(os.path.join(OUT, "p4_latent.png"))

    # контроль: белый диск планеты + наклонное кольцо
    ctrl = Image.new("RGB", (W, H), (0, 0, 0))
    cd = ImageDraw.Draw(ctrl)
    pcx, pcy, pr = 590, 330, 204
    cd.ellipse([pcx - pr, pcy - pr, pcx + pr, pcy + pr], fill=(255, 255, 255))
    # наклонное кольцо (эллипс, повёрнутый на -18°)
    ring = Image.new("L", (W, H), 0)
    rd = ImageDraw.Draw(ring)
    rrx, rry = 430, 118
    rd.ellipse([W // 2 - rrx, H // 2 - rry, W // 2 + rrx, H // 2 + rry], outline=255, width=5)
    ring = ring.rotate(-18, resample=Image.BICUBIC, center=(pcx, pcy))
    ctrl = Image.fromarray(np.clip(np.array(ctrl).max(axis=2) + np.array(ring), 0, 255).astype(np.uint8)).convert("RGB")
    ctrl.save(os.path.join(OUT, "p4_control.png"))
    print("p4_latent.png, p4_control.png", ctrl.size)


# ---------- P4b: силуэт планеты (латент) + тонкий контур (контроль) ----------
def p4b():
    pcx, pcy, pr = 590, 330, 200
    rrx, rry = 430, 118
    ang = -18

    # контроль: только тонкие контуры (круг планеты + кольцо) на чёрном
    ctrl = Image.new("RGB", (W, H), (0, 0, 0))
    cd = ImageDraw.Draw(ctrl)
    cd.ellipse([pcx - pr, pcy - pr, pcx + pr, pcy + pr], outline=(255, 255, 255), width=5)
    ring = Image.new("L", (W, H), 0)
    rd = ImageDraw.Draw(ring)
    rd.ellipse([W // 2 - rrx, H // 2 - rry, W // 2 + rrx, H // 2 + rry], outline=255, width=4)
    ring = ring.rotate(ang, resample=Image.BICUBIC, center=(pcx, pcy))
    ctrl = Image.fromarray(np.clip(np.array(ctrl).max(axis=2) + np.array(ring), 0, 255).astype(np.uint8)).convert("RGB")
    ctrl.save(os.path.join(OUT, "p4_ctrl_outline.png"))

    # силуэт-латент: тёплое тело + бирюзовая атмосфера по лимбу + кольцо + солнце у края
    sil = bg_canvas()
    sd = ImageDraw.Draw(sil, "RGBA")
    # солнце у правого края — мягкое тёплое свечение
    for rr, al in ((360, 10), (280, 16), (200, 24), (130, 34)):
        sd.ellipse([W - 40 - rr, H // 2 - rr, W - 40 + rr, H // 2 + rr], fill=PAL["amber"] + (al,))
    # кольцо (за планетой и перед ней — рисуем до и после)
    ringimg = Image.new("RGBA", (W, H), (0, 0, 0, 0))
    rd2 = ImageDraw.Draw(ringimg)
    rd2.ellipse([W // 2 - rrx, H // 2 - rry, W // 2 + rrx, H // 2 + rry],
                outline=PAL["dust"] + (150,), width=5)
    rd2.ellipse([W // 2 - rrx - 3, H // 2 - rry - 3, W // 2 + rrx + 3, H // 2 + rry + 3],
                outline=PAL["amber"] + (90,), width=3)
    ringimg = ringimg.rotate(ang, resample=Image.BICUBIC, center=(pcx, pcy))
    sil = Image.alpha_composite(sil.convert("RGBA"), ringimg)
    sd = ImageDraw.Draw(sil, "RGBA")
    # атмосфера по лимбу (бирюзовый ореол) -> тело
    sd.ellipse([pcx - pr - 12, pcy - pr - 12, pcx + pr + 12, pcy + pr + 12], fill=PAL["atmo_halo"] + (150,))
    sd.ellipse([pcx - pr, pcy - pr, pcx + pr, pcy + pr], fill=PAL["body_terra"] + (255,))
    # передняя часть кольца
    sil = Image.alpha_composite(sil, ringimg)
    sil = sil.filter(ImageFilter.GaussianBlur(3)).convert("RGB")
    sil.save(os.path.join(OUT, "p4_sil.png"))
    print("p4_sil.png, p4_ctrl_outline.png", sil.size)


if __name__ == "__main__":
    p2()
    p4()
    p4b()
