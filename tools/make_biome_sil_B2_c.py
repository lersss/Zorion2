# -*- coding: utf-8 -*-
# Часть C пачки 2: вода №15-21 (озёра_реки .. co2_океаны). Стиль B.
from biome_sil_helpers import *  # noqa: F401,F403


def s_ozera_reki():
    img, d = new_canvas()
    scene(d, SKY_PALE, GREEN_D, 470)
    clouds(d, 170, CLOUD, [-280, 60, 340], w=130)
    d.pieslice([CX - 620, 380, CX + 40, 940], 180, 360, fill=(78, 140, 84))
    d.line([(CX - 620, 660), (CX + 40, 660)], fill=BORDER, width=4)
    ground3(d, 660, (86, 148, 92), (68, 122, 78), (52, 100, 64))
    d.ellipse([CX - 340, 620, CX + 120, 830], fill=BLUE, outline=BORDER, width=5)
    d.ellipse([CX - 300, 645, CX + 60, 800], fill=BLUE_L)
    d.polygon([(CX + 160, 830), (CX + 220, 700), (CX + 260, 600), (CX + 300, 600), (CX + 260, 720), (CX + 240, 830)], fill=BLUE)
    d.line([(CX + 160, 830), (CX + 220, 700), (CX + 260, 600)], fill=BORDER, width=4)
    d.line([(CX + 300, 600), (CX + 260, 720), (CX + 240, 830)], fill=BORDER, width=4)
    save(img, 'silB2_озёра_реки')


def s_melkovodya_zalivnye():
    img, d = new_canvas()
    scene(d, (168, 196, 214), (120, 150, 130), 430)
    clouds(d, 160, CLOUD, [-260, 120, 340], w=120)
    d.rectangle([0, 430, SIZE, SIZE], fill=(150, 175, 160))
    for y, c in [(520, (130, 168, 180)), (640, (112, 154, 172)), (760, (96, 138, 160)), (880, (110, 150, 168))]:
        d.polygon([(0, y + 30), (0, y - 30), (250, y - 30), (340, y + 30), (530, y - 30), (660, y + 30), (850, y - 30), (SIZE, y - 30), (SIZE, y + 30)], fill=c)
        d.line([(0, y + 30), (0, y - 30), (250, y - 30), (340, y + 30), (530, y - 30), (660, y + 30), (850, y - 30), (SIZE, y - 30), (SIZE, y + 30)], fill=BORDER, width=4)
    d.ellipse([CX - 340, 560, CX - 120, 640], fill=(196, 174, 132), outline=BORDER, width=4)
    d.ellipse([CX + 80, 700, CX + 340, 790], fill=(196, 174, 132), outline=BORDER, width=4)
    d.ellipse([CX - 180, 840, CX + 60, 920], fill=(186, 164, 124), outline=BORDER, width=4)
    save(img, 'silB2_мелководья_заливные')


def s_planktonnye_morya():
    img, d = new_canvas()
    scene(d, SKY_TEAL, (30, 110, 110), 440)
    clouds(d, 180, (110, 170, 170), [-240, 100, 340], w=130)
    for y, c in [(540, (36, 140, 130)), (660, (44, 165, 140)), (780, (54, 190, 150)), (890, (44, 165, 140))]:
        d.polygon([(0, y + 34), (0, y - 34), (250, y - 34), (340, y + 34), (530, y - 34), (660, y + 34), (850, y - 34), (SIZE, y - 34), (SIZE, y + 34)], fill=c)
        d.line([(0, y + 34), (0, y - 34), (250, y - 34), (340, y + 34), (530, y - 34), (660, y + 34), (850, y - 34), (SIZE, y - 34), (SIZE, y + 34)], fill=BORDER, width=4)
    d.line([(0, 440), (SIZE, 440)], fill=(140, 230, 200), width=10)
    for y in (600, 720, 840):
        d.line([(80, y), (300, y - 18), (600, y - 4), (SIZE - 60, y - 20)], fill=(150, 240, 190), width=7)
    save(img, 'silB2_планктонные_моря')


def s_termalnye_terrasy():
    img, d = new_canvas()
    scene(d, (200, 214, 220), (210, 200, 180), 440)
    clouds(d, 170, WHITE, [-260, 100, 340], w=120)
    for y, c in [(560, (120, 190, 200)), (660, (150, 210, 190)), (760, (200, 180, 150)), (860, (140, 200, 205))]:
        d.polygon([(0, y), (CX - 260, y - 20), (CX + 220, y + 16), (SIZE, y - 10), (SIZE, y + 100), (0, y + 100)], fill=c)
        d.line([(0, y), (CX - 260, y - 20), (CX + 220, y + 16), (SIZE, y - 10)], fill=(80, 150, 160), width=7)
        d.line([(0, y + 100), (CX - 260, y + 80), (CX + 220, y + 116), (SIZE, y + 90)], fill=BORDER, width=4)
    for x in (-220, 60, 300):
        for py in (360, 300, 240):
            d.ellipse([CX + x - 50, py - 26, CX + x + 50, py + 26], fill=(226, 234, 238))
    save(img, 'silB2_термальные_террасы')


def s_ammiachnye_krio_okeany():
    img, d = new_canvas()
    scene(d, (208, 190, 208), AMMONIA, 440)
    clouds(d, 180, (222, 206, 222), [-240, 100, 340], w=130)
    for y, c in [(540, (226, 202, 220)), (650, (216, 192, 212)), (760, (206, 182, 204)), (870, (216, 192, 212))]:
        d.polygon([(0, y + 34), (0, y - 34), (250, y - 34), (340, y + 34), (530, y - 34), (660, y + 34), (850, y - 34), (SIZE, y - 34), (SIZE, y + 34)], fill=c)
        d.line([(0, y + 34), (0, y - 34), (250, y - 34), (340, y + 34), (530, y - 34), (660, y + 34), (850, y - 34), (SIZE, y - 34), (SIZE, y + 34)], fill=BORDER, width=4)
    d.line([(0, 440), (SIZE, 440)], fill=(244, 226, 238), width=10)
    for x, y, w, h in [(-300, 620, 90, 40), (-80, 720, 110, 46), (180, 640, 100, 42), (330, 800, 90, 38)]:
        d.polygon([(CX + x - w, y), (CX + x - w * 0.3, y - h), (CX + x + w * 0.6, y - h * 0.6), (CX + x + w, y)], fill=(248, 238, 246), outline=BORDER, width=4)
    save(img, 'silB2_аммиачные_крио-океаны')


def s_podlednye_okeany():
    img, d = new_canvas()
    scene(d, SKY_ICE, ICE, 430)
    clouds(d, 170, WHITE, [-240, 100, 340], w=120)
    d.polygon([(0, 430), (SIZE, 430), (SIZE, 620), (CX + 180, 640), (CX + 60, 720), (CX - 120, 700), (CX - 300, 620), (0, 610)], fill=(212, 232, 244))
    d.line([(0, 430), (SIZE, 430)], fill=BORDER, width=4)
    d.line([(0, 610), (CX - 300, 620), (CX - 120, 700), (CX + 60, 720), (CX + 180, 640), (SIZE, 620)], fill=BORDER, width=5)
    d.polygon([(CX - 120, 700), (CX + 60, 720), (CX + 220, 900), (CX - 180, 940)], fill=(28, 66, 110))
    d.line([(CX - 120, 700), (CX + 60, 720), (CX + 220, 900)], fill=BORDER, width=6)
    d.line([(CX + 60, 720), (CX - 180, 940)], fill=BORDER, width=6)
    d.line([(CX - 60, 790), (CX + 30, 800)], fill=(120, 180, 220), width=6)
    for x, h in [(-300, 140), (-180, 100), (260, 130), (360, 90)]:
        d.polygon([(CX + x - 50, 640), (CX + x, 640 - h), (CX + x + 50, 640)], fill=(226, 240, 248), outline=BORDER, width=4)
    save(img, 'silB2_подлёдные_океаны')


def s_co2_okeany():
    img, d = new_canvas()
    scene(d, SKY_AMBER, AMBER, 440)
    clouds(d, 180, (220, 200, 150), [-240, 100, 340], w=130)
    for y, c in [(540, (200, 168, 100)), (650, (188, 156, 92)), (760, (176, 146, 84)), (870, (188, 156, 92))]:
        d.polygon([(0, y + 34), (0, y - 34), (250, y - 34), (340, y + 34), (530, y - 34), (660, y + 34), (850, y - 34), (SIZE, y - 34), (SIZE, y + 34)], fill=c)
        d.line([(0, y + 34), (0, y - 34), (250, y - 34), (340, y + 34), (530, y - 34), (660, y + 34), (850, y - 34), (SIZE, y - 34), (SIZE, y + 34)], fill=BORDER, width=4)
    d.line([(0, 440), (SIZE, 440)], fill=(238, 214, 150), width=10)
    d.rectangle([0, 500, SIZE, 545], fill=(206, 184, 138))
    save(img, 'silB2_co2_океаны')


def run():
    s_ozera_reki()
    s_melkovodya_zalivnye()
    s_planktonnye_morya()
    s_termalnye_terrasy()
    s_ammiachnye_krio_okeany()
    s_podlednye_okeany()
    s_co2_okeany()


if __name__ == '__main__':
    run()
