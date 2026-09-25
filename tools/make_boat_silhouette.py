# -*- coding: utf-8 -*-
# Силуэт-мастер лодки прогулки (ЧК6.3 «Мир прогулки»).
# ТЗ: docs/gamedesign/art/art_surface_boat.md §4.2 (ассет-спека §3.2/§3.3).
#
# 1024x1024, фон чёрный (5,5,5). Лодка рисуется в суб-рамке R = (36,232)-(988,792),
# 952x560 = ровно 34:20. Постобработка (process_surface_boat.py) режет РОВНО R и
# масштабирует в 204x120, поэтому линия воды садится точно в y=84:
#   логика y=14 -> холст Y=232+14*28=624 -> (624-232)/560*120 = 84.
# Форму задаёт ТОЛЬКО силуэт (ControlNet Canny); цвет/текстуру даёт SDXL.
#
# ВАЖНО (находка 2026-09-26): внешний контур #3f2415 по чёрному слишком тёмный —
# Canny его не ловит, и SDXL «усаживает» лодку внутрь кадра (~82% по Y) и гасит
# мотор. Поэтому внешняя кромка силуэта — контрастный тон RIM (Canny видит границу),
# а тёмный контур #3f2415 остаётся тонкой внутренней линией-кромкой.
#
# Палитра (§3.3): тело #c86432, планшир #f0d8c0, тёмное ложе #5a3320, контур #3f2415.
import os, math
from PIL import Image, ImageDraw, ImageChops

OUT = r'C:\Zorion2\ai_drafts\surface_boat\silhouette'
SIZE = 1024
SS = 2  # супер-сэмплинг для гладких краёв

BG = (5, 5, 5)
RX0, RY0, RX1, RY1 = 36, 232, 988, 792
S = (RX1 - RX0) / 34.0  # 28 px на логическую единицу
assert abs((RX1 - RX0) / (RY1 - RY0) - 34.0 / 20.0) < 1e-9

# §3.3
BODY = (200, 100, 50)       # #c86432
GUNW = (240, 216, 192)      # #f0d8c0
WELL = (90, 51, 32)         # #5a3320
CONTOUR = (63, 36, 21)      # #3f2415
BODY_SH = (150, 72, 36)     # тень низа трубы
RIM = (226, 110, 46)         # контрастная внешняя кромка (ярче тела; для Canny)
MOTOR = (86, 80, 74)        # тёмный блок мотора
MOTOR_CAP = (150, 144, 136) # светлая крышка мотора


def P(pts):
    return [((RX0 + x * S) * SS, (RY0 + y * S) * SS) for x, y in pts]


def W(logic_px):
    return max(1, int(round(logic_px * S * SS)))


def centroid(pts):
    return sum(p[0] for p in pts) / len(pts), sum(p[1] for p in pts) / len(pts)


def offset_toward(pts, target, dist):
    tx, ty = target
    out = []
    for x, y in pts:
        dx, dy = tx - x, ty - y
        l = math.hypot(dx, dy) or 1.0
        out.append((x + dx / l * dist, y + dy / l * dist))
    return out


# --- геометрия (§3.2). Видимый силуэт занимает ровно 0..34 x 0..20.
TOP = [
    (4.1, 0.85),    # пик кормы (верх транца)
    (5.6, 2.1),
    (7.8, 3.3),
    (10.4, 4.3),
    (13.2, 4.9),
    (15.6, 5.0),
    (17.0, 5.0),    # провал аммидшипс
    (19.0, 4.8),
    (21.6, 4.2),
    (24.4, 3.4),
    (27.0, 2.3),
    (29.6, 1.3),
    (31.4, 0.85),   # пик носа
    (32.6, 1.2),
    (33.6, 2.8),
    (33.9, 4.4),    # остриё носа
]
BOTTOM = [
    (33.9, 4.4),
    (33.0, 7.4),
    (31.6, 10.8),
    (29.8, 14.0),
    (27.4, 16.8),
    (24.6, 18.6),
    (21.6, 19.5),
    (18.4, 19.7),   # киль
    (15.4, 19.5),
    (12.4, 18.9),
    (9.4, 17.9),
    (6.8, 16.7),
    (4.2, 15.2),
]
HULL = TOP + BOTTOM

# Тёмное ложе для ног (компактнее прежнего; стопы игрока lv-6 = y=8 внутри)
WELL_POLY = [
    (10.2, 7.6),
    (12.0, 7.1),
    (15.0, 7.0),
    (18.0, 7.0),
    (21.0, 7.2),
    (23.4, 7.8),
    (24.2, 8.9),
    (23.4, 11.4),
    (21.0, 12.3),
    (18.0, 12.6),
    (15.0, 12.5),
    (12.0, 11.9),
    (10.1, 10.3),
]

# Подвесной мотор у транца слева: блок 4 логических px (x 0.9..4.9), над водой
MOTOR_POLY = [
    (0.9, 3.0),
    (1.9, 2.3),
    (4.9, 2.5),
    (4.9, 13.8),
    (1.7, 13.5),
    (0.9, 12.2),
]
MOTOR_CAP_POLY = [
    (1.1, 2.9),
    (2.1, 2.55),
    (4.7, 2.75),
    (4.7, 4.6),
    (1.1, 4.4),
]


def main():
    os.makedirs(OUT, exist_ok=True)
    big = Image.new('RGB', (SIZE * SS, SIZE * SS), BG)
    d = ImageDraw.Draw(big)
    c = centroid(HULL)

    hull_mask = Image.new('L', big.size, 0)
    ImageDraw.Draw(hull_mask).polygon(P(HULL), fill=255)

    # 1. тело
    d.polygon(P(HULL), fill=BODY)

    # 2. светлый планшир
    band = Image.new('RGBA', big.size, (0, 0, 0, 0))
    ImageDraw.Draw(band).line(P(offset_toward(TOP, c, 1.25)),
                              fill=GUNW + (255,), width=W(1.6), joint='curve')
    band.putalpha(ImageChops.multiply(band.split()[3], hull_mask))
    big.paste(band, (0, 0), band)

    # 3. тень низа трубы
    sh = Image.new('RGBA', big.size, (0, 0, 0, 0))
    ImageDraw.Draw(sh).line(P(offset_toward(BOTTOM, c, 1.1)),
                            fill=BODY_SH + (255,), width=W(1.1), joint='curve')
    sh.putalpha(ImageChops.multiply(sh.split()[3], hull_mask))
    big.paste(sh, (0, 0), sh)

    # 4. тёмное ложе + светлая внутренняя кромка трубы (не «дыра»)
    wl = Image.new('RGBA', big.size, (0, 0, 0, 0))
    wd = ImageDraw.Draw(wl)
    wd.polygon(P(WELL_POLY), fill=WELL + (255,), outline=BODY_SH + (255,), width=W(0.55))
    wl.putalpha(ImageChops.multiply(wl.split()[3], hull_mask))
    big.paste(wl, (0, 0), wl)

    # 5. швы и рёбра трубы
    seams = Image.new('RGBA', big.size, (0, 0, 0, 0))
    sd = ImageDraw.Draw(seams)
    sd.line(P(offset_toward(BOTTOM, c, 2.4)), fill=BODY_SH + (255,),
            width=W(0.5), joint='curve')
    for x, y0, y1 in ((6.8, 4.4, 14.8), (27.6, 4.0, 13.6)):
        sd.line(P([(x, y0), (x, y1)]), fill=BODY_SH + (255,), width=W(0.4))
    sd.line(P([(4.35, 1.6), (4.35, 15.2)]), fill=RIM + (255,), width=W(0.45))
    seams.putalpha(ImageChops.multiply(seams.split()[3], hull_mask))
    big.paste(seams, (0, 0), seams)

    # 6. мотор (серый блок + светлая крышка), поверх тела
    mo = Image.new('RGBA', big.size, (0, 0, 0, 0))
    md = ImageDraw.Draw(mo)
    md.polygon(P(MOTOR_POLY), fill=MOTOR + (255,),
               outline=RIM + (255,), width=W(0.6))
    md.polygon(P(MOTOR_CAP_POLY), fill=MOTOR_CAP + (255,))
    big.paste(mo, (0, 0), mo)

    # 7. контрастная внешняя кромка (Canny) + тёмный контур-кромка внутрь
    d.line(P(HULL + [HULL[0]]), fill=RIM + (255,), width=W(1.6), joint='curve')
    d.line(P(MOTOR_POLY + [MOTOR_POLY[0]]), fill=RIM + (255,),
           width=W(1.6), joint='curve')
    d.line(P(offset_toward(HULL, c, 0.95) + [offset_toward(HULL, c, 0.95)[0]]),
           fill=CONTOUR + (255,), width=W(0.45), joint='curve')

    out = big.resize((SIZE, SIZE), Image.LANCZOS)
    path = os.path.join(OUT, 'boat.png')
    out.save(path)
    print('Saved: %s (%dx%d), R=(%d,%d)-(%d,%d)' % (path, SIZE, SIZE, RX0, RY0, RX1, RY1))


if __name__ == '__main__':
    main()
