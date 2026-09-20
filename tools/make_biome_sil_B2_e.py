# -*- coding: utf-8 -*-
# Часть E пачки 2: №29-34 (металлические_рощи .. пещерный_мир_с_потолком). Стиль B.
from biome_sil_helpers import *  # noqa: F401,F403


def s_metallicheskie_roshchi():
    img, d = new_canvas()
    scene(d, (138, 146, 156), METAL_D, 520)
    clouds(d, 180, (150, 158, 168), [-260, 100, 340], w=120)
    for tx in range(-320, 340, 100):
        s = 130 + (tx % 4) * 30
        d.rectangle([CX + tx - 10, 520 - s, CX + tx + 10, 520], fill=METAL, outline=BORDER, width=3)
        d.polygon([(CX + tx - 50, 520 - s), (CX + tx, 520 - s - 60), (CX + tx + 50, 520 - s)], fill=METAL_L)
        d.line([(CX + tx - 50, 520 - s), (CX + tx, 520 - s - 60), (CX + tx + 50, 520 - s), (CX + tx - 50, 520 - s)], fill=BORDER, width=4)
        d.line([(CX + tx, 520 - s - 60), (CX + tx, 520 - s)], fill=BORDER, width=3)
    ground3(d, 580, (108, 122, 142), (90, 102, 122), (72, 82, 100))
    save(img, 'silB2_металлические_рощи')


def s_radiatsionnye_kovry():
    img, d = new_canvas()
    scene(d, (48, 58, 52), (28, 44, 38), 480)
    ground3(d, 520, (34, 56, 46), (26, 44, 36), (20, 34, 28))
    for x, y, w, h in [(-300, 600, 170, 40), (-40, 660, 220, 48), (240, 720, 180, 44), (-160, 800, 200, 46)]:
        d.polygon([(CX + x - w, y), (CX + x, y - h), (CX + x + w, y), (CX + x, y + h)], fill=RAD, outline=BORDER, width=4)
        for vx in (-w * 0.5, 0, w * 0.5):
            d.line([(CX + x + vx, y - h * 0.4), (CX + x + vx, y + h * 0.4)], fill=RAD_D, width=5)
    save(img, 'silB2_радиационные_ковры')


def s_karbidno_almaznye_zarosli():
    img, d = new_canvas()
    scene(d, (36, 40, 52), (26, 28, 36), 480)
    ground3(d, 520, (34, 36, 44), (26, 28, 36), (18, 20, 28))
    for x, h, w in [(-320, 300, 60), (-160, 400, 78), (20, 330, 64), (200, 380, 72), (360, 280, 56)]:
        d.polygon([(CX + x - w, 620), (CX + x, 620 - h), (CX + x + w, 620)], fill=(46, 48, 60))
        d.line([(CX + x - w, 620), (CX + x, 620 - h), (CX + x + w, 620), (CX + x - w, 620)], fill=BORDER, width=4)
        d.line([(CX + x - w, 620), (CX + x, 620 - h * 0.55)], fill=DIAM, width=5)
        d.line([(CX + x, 620 - h), (CX + x + w, 620)], fill=DIAM_D, width=4)
    for gx in (-260, -40, 180, 340):
        d.polygon([(CX + gx - 30, 830), (CX + gx, 770), (CX + gx + 30, 830)], fill=(40, 42, 52), outline=BORDER, width=4)
    save(img, 'silB2_карбидно-алмазные_заросли')


def s_radiatsionnye_pustoshchi():
    img, d = new_canvas()
    scene(d, (44, 48, 44), (34, 38, 34), 480)
    ground3(d, 520, (42, 46, 40), (32, 36, 32), (24, 28, 24))
    for gx, gy, s in [(-280, 640, 60), (-40, 700, 80), (210, 680, 55), (350, 780, 65), (60, 830, 50)]:
        d.polygon([(CX + gx - s, gy), (CX + gx, gy - s), (CX + gx + s, gy)], fill=(60, 64, 58), outline=BORDER, width=4)
        d.ellipse([CX + gx - s * 0.2, gy - s * 0.8, CX + gx + s * 0.2, gy - s * 0.4], fill=RAD, outline=BORDER, width=3)
    d.line([(CX - 200, 720), (CX - 100, 780), (CX + 40, 760)], fill=RAD_D, width=6)
    d.line([(CX + 120, 860), (CX + 240, 900)], fill=RAD_D, width=6)
    save(img, 'silB2_радиационные_пустоши')


def s_pruzhinnaya_tundra():
    img, d = new_canvas()
    scene(d, (176, 196, 190), (110, 140, 100), 500)
    clouds(d, 170, CLOUD, [-260, 100, 340], w=120)
    ground3(d, 560, (120, 150, 108), (100, 128, 92), (82, 108, 76))
    for x, h in [(-300, 200), (-140, 260), (40, 220), (200, 250), (350, 190)]:
        bx = CX + x
        top = 560
        d.line([(bx, top), (bx, top - h)], fill=SPRING, width=9)
        turns = 6
        for i in range(turns):
            y0 = top - h + i * (h / turns)
            d.arc([bx - 30, y0, bx + 30, y0 + h / turns], 0, 180, fill=SPRING, width=8)
        d.ellipse([bx - 30, top - h - 24, bx + 30, top - h + 16], fill=(150, 200, 130), outline=BORDER, width=3)
    save(img, 'silB2_пружинная_тундра')


def s_peshchernyy_mir():
    img, d = new_canvas()
    d.rectangle([0, 0, SIZE, 300], fill=(30, 30, 36))
    d.rectangle([0, 300, SIZE, SIZE], fill=(80, 72, 66))
    # потолок пещеры
    d.polygon([(0, 0), (SIZE, 0), (SIZE, 180), (CX + 240, 250), (CX + 40, 180), (CX - 160, 260), (CX - 360, 190), (0, 250)], fill=(36, 34, 40))
    d.line([(0, 250), (CX - 360, 190), (CX - 160, 260), (CX + 40, 180), (CX + 240, 250), (SIZE, 180)], fill=BORDER, width=5)
    # сталактиты
    for x, h in [(-320, 260), (-160, 340), (0, 300), (160, 360), (320, 250)]:
        d.polygon([(CX + x - 26, 250), (CX + x, 250 + h), (CX + x + 26, 250)], fill=(48, 46, 54))
        d.line([(CX + x - 26, 250), (CX + x, 250 + h), (CX + x + 26, 250)], fill=BORDER, width=4)
    ground3(d, 620, (96, 86, 76), (78, 70, 62), (60, 54, 48))
    # сталагмиты снизу
    for x, h in [(-260, 160), (-60, 200), (200, 180)]:
        d.polygon([(CX + x - 34, 820), (CX + x, 820 - h), (CX + x + 34, 820)], fill=(104, 92, 80), outline=BORDER, width=4)
    save(img, 'silB2_пещерный_мир_с_потолком')


def run():
    s_metallicheskie_roshchi()
    s_radiatsionnye_kovry()
    s_karbidno_almaznye_zarosli()
    s_radiatsionnye_pustoshchi()
    s_pruzhinnaya_tundra()
    s_peshchernyy_mir()


if __name__ == '__main__':
    run()
