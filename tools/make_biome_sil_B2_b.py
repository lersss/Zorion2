# -*- coding: utf-8 -*-
# Часть B пачки 2: литосфера №8-14 (грозовая_степь .. криовулканические_поля). Стиль B.
from biome_sil_helpers import *  # noqa: F401,F403


def s_grozovaya_step():
    img, d = new_canvas()
    scene(d, (52, 54, 70), (104, 74, 60), 500)
    # тяжёлые тучи с подсвеченным низом
    clouds(d, 130, (38, 40, 54), [-300, 20, 340], w=170, h=54)
    clouds(d, 220, (62, 64, 82), [-180, 160, 400], w=140, h=40)
    clouds(d, 250, (120, 118, 130), [-120, 240], w=110, h=26)
    # широкие яркие молнии
    d.line([(CX - 230, 240), (CX - 180, 340), (CX - 225, 355), (CX - 150, 500)], fill=(255, 250, 190), width=16)
    d.line([(CX + 140, 200), (CX + 90, 320), (CX + 150, 340), (CX + 70, 500)], fill=(255, 250, 190), width=14)
    d.line([(CX - 20, 260), (CX + 10, 330), (CX - 20, 345), (CX + 20, 440)], fill=(240, 236, 180), width=9)
    ground3(d, 560, (126, 80, 62), (104, 66, 52), (82, 52, 44))
    # прибитая ветром трава
    for gx, gh in [(-340, 120), (-200, 150), (-60, 110), (90, 145), (230, 115), (380, 90)]:
        d.line([(CX + gx, 780), (CX + gx + 40, 700 - gh + 40)], fill=(150, 96, 70), width=10)
        d.line([(CX + gx, 820), (CX + gx + 54, 760 - gh + 40)], fill=(132, 84, 62), width=9)
    save(img, 'silB2_грозовая_степь')


def s_uglerodnye_nagorya():
    img, d = new_canvas()
    scene(d, (60, 62, 70), CARBON_D, 470)
    for px, ph, pw, c in [(-300, 220, 120, CARBON), (-120, 320, 140, (48, 50, 58)), (100, 260, 130, CARBON), (300, 300, 140, (44, 46, 54))]:
        d.polygon([(CX + px - pw, 470), (CX + px, 470 - ph), (CX + px + pw, 470)], fill=c)
        d.line([(CX + px - pw, 470), (CX + px, 470 - ph), (CX + px + pw, 470)], fill=BORDER, width=4)
    ground3(d, 560, (52, 54, 62), (38, 40, 48), (26, 28, 34))
    for gx in (-260, 0, 240):
        d.line([(CX + gx, 800), (CX + gx + 40, 830)], fill=(70, 72, 80), width=6)
    save(img, 'silB2_углеродные_нагорья')


def s_metallorudnye_vozvyshennosti():
    img, d = new_canvas()
    scene(d, (122, 120, 126), (100, 86, 78), 470)
    for px, ph, pw, c in [(-280, 200, 130, (118, 100, 88)), (-60, 300, 150, (132, 112, 96)), (170, 230, 130, (110, 94, 82)), (340, 180, 110, (122, 104, 90))]:
        d.polygon([(CX + px - pw, 470), (CX + px - pw * 0.3, 470 - ph * 0.7), (CX + px, 470 - ph), (CX + px + pw * 0.4, 470 - ph * 0.6), (CX + px + pw, 470)], fill=c)
        d.line([(CX + px - pw, 470), (CX + px - pw * 0.3, 470 - ph * 0.7), (CX + px, 470 - ph), (CX + px + pw * 0.4, 470 - ph * 0.6), (CX + px + pw, 470)], fill=BORDER, width=4)
        d.line([(CX + px - pw * 0.3, 470 - ph * 0.7), (CX + px + 10, 470 - ph * 0.35)], fill=RUST, width=6)
    ground3(d, 580, (128, 110, 96), (106, 90, 78), (86, 74, 64))
    for gx, gy in [(-240, 700), (60, 760), (280, 800)]:
        d.ellipse([CX + gx - 26, gy - 26, CX + gx + 26, gy + 26], fill=ORE, outline=BORDER, width=4)
    save(img, 'silB2_металлорудные_возвышенности')


def s_metallicheskie_polya():
    img, d = new_canvas()
    scene(d, (128, 134, 142), METAL_D, 450)
    clouds(d, 160, (150, 158, 168), [-260, 80, 320], w=130)
    d.polygon([(0, 480), (CX - 220, 460), (CX + 60, 500), (SIZE, 470), (SIZE, SIZE), (0, SIZE)], fill=(112, 126, 146))
    d.line([(0, 480), (CX - 220, 460), (CX + 60, 500), (SIZE, 470)], fill=BORDER, width=5)
    for y in (580, 680, 780, 880):
        d.line([(0, y + 20), (280, y - 10), (SIZE, y + 14)], fill=(88, 100, 118), width=8)
    d.line([(CX - 300, 520), (CX - 300, 900)], fill=(88, 100, 118), width=8)
    d.line([(CX + 200, 540), (CX + 200, 920)], fill=(88, 100, 118), width=8)
    d.line([(CX - 360, 620), (CX + 60, 640)], fill=(88, 100, 118), width=8)
    d.polygon([(CX - 360, 700), (CX - 140, 680), (CX - 100, 720), (CX - 320, 745)], fill=METAL)
    d.line([(CX - 360, 700), (CX - 140, 680), (CX - 100, 720), (CX - 320, 745), (CX - 360, 700)], fill=BORDER, width=5)
    d.polygon([(CX + 40, 760), (CX + 300, 730), (CX + 360, 780), (CX + 90, 815)], fill=(132, 148, 170))
    d.line([(CX + 40, 760), (CX + 300, 730), (CX + 360, 780), (CX + 90, 815), (CX + 40, 760)], fill=BORDER, width=5)
    save(img, 'silB2_металлические_поля')


def s_obsidianovye_polya():
    img, d = new_canvas()
    scene(d, (50, 50, 60), OBSID, 460)
    clouds(d, 180, (74, 72, 84), [-220, 140, 360], w=120, h=30)
    ground3(d, 500, (44, 44, 54), (32, 32, 42), (22, 22, 30))
    for sx, h, w in [(-320, 260, 70), (-150, 340, 90), (40, 220, 60), (210, 300, 80), (370, 200, 55)]:
        d.polygon([(CX + sx - w, 640), (CX + sx, 640 - h), (CX + sx + w, 640)], fill=(34, 34, 46))
        d.line([(CX + sx - w, 640), (CX + sx, 640 - h), (CX + sx + w, 640), (CX + sx - w, 640)], fill=BORDER, width=4)
        d.line([(CX + sx - w, 640), (CX + sx, 640 - h * 0.5)], fill=(95, 95, 120), width=4)
        d.line([(CX + sx, 640 - h), (CX + sx + w, 640)], fill=(70, 70, 90), width=4)
    for gx in (-260, -60, 150, 340):
        d.line([(CX + gx, 800), (CX + gx + 24, 780)], fill=(90, 90, 115), width=5)
    save(img, 'silB2_обсидиановые_поля')


def s_venerianskie_ploskogorya():
    img, d = new_canvas()
    scene(d, SKY_VENUS, VENUS, 460)
    clouds(d, 180, (226, 180, 110), [-280, 80, 340], w=140, h=40)
    d.polygon([(0, 460), (CX - 240, 360), (CX - 40, 470), (CX + 220, 370), (SIZE, 460), (SIZE, 620), (0, 620)], fill=(216, 158, 84))
    d.line([(0, 460), (CX - 240, 360), (CX - 40, 470), (CX + 220, 370), (SIZE, 460)], fill=BORDER, width=5)
    d.polygon([(0, 620), (CX - 200, 600), (CX + 60, 640), (SIZE, 610), (SIZE, SIZE), (0, SIZE)], fill=(198, 142, 74))
    d.line([(0, 620), (CX - 200, 600), (CX + 60, 640), (SIZE, 610)], fill=BORDER, width=5)
    for x in (-300, -100, 120, 320):
        d.line([(CX + x, 640), (CX + x + 20, 820)], fill=(170, 120, 62), width=6)
    d.polygon([(0, 760), (CX - 80, 740), (CX + 120, 780), (SIZE, 750), (SIZE, SIZE), (0, SIZE)], fill=(176, 124, 64))
    d.line([(0, 760), (CX - 80, 740), (CX + 120, 780), (SIZE, 750)], fill=BORDER, width=4)
    save(img, 'silB2_венерианские_плоскогорья')


def s_kriovulkanicheskie_polya():
    img, d = new_canvas()
    scene(d, SKY_ICE, (150, 190, 215), 470)
    ground3(d, 520, (168, 205, 228), (140, 180, 208), (118, 158, 188))
    for x, h in [(-260, 380), (40, 470), (300, 340)]:
        d.polygon([(CX + x - 40, 620), (CX + x, 260), (CX + x + 40, 620)], fill=(226, 240, 250))
        d.line([(CX + x - 40, 620), (CX + x, 260), (CX + x + 40, 620)], fill=BORDER, width=5)
        for py in (300, 240, 180):
            d.ellipse([CX + x - 60, py - 30, CX + x + 60, py + 30], fill=(210, 228, 242))
    for bx, bw, bh in [(-340, 70, 60), (-120, 80, 70), (200, 75, 65), (370, 60, 50)]:
        d.polygon([(CX + bx - bw, 830), (CX + bx, 830 - bh), (CX + bx + bw, 830)], fill=(200, 226, 242), outline=BORDER, width=4)
    save(img, 'silB2_криовулканические_поля')


def run():
    s_grozovaya_step()
    s_uglerodnye_nagorya()
    s_metallorudnye_vozvyshennosti()
    s_metallicheskie_polya()
    s_obsidianovye_polya()
    s_venerianskie_ploskogorya()
    s_kriovulkanicheskie_polya()


if __name__ == '__main__':
    run()
