# -*- coding: utf-8 -*-
# Часть D пачки 2: крио №22-25 (ледники .. сухой_лёд) + биосфера №26-28 (джунгли .. блуждающие_рощи). Стиль B.
from biome_sil_helpers import *  # noqa: F401,F403


def s_ledniki():
    img, d = new_canvas()
    scene(d, (188, 210, 226), (160, 195, 220), 460)
    clouds(d, 180, WHITE, [-260, 80, 340], w=130)
    for px, ph, pw, c in [(-300, 260, 120, (170, 205, 230)), (-80, 360, 140, (196, 224, 242)), (180, 300, 130, (180, 212, 234)), (350, 220, 100, (160, 198, 226))]:
        d.polygon([(CX + px - pw, 620), (CX + px, 620 - ph), (CX + px + pw, 620)], fill=c)
        d.line([(CX + px - pw, 620), (CX + px, 620 - ph), (CX + px + pw, 620)], fill=BORDER, width=4)
        d.line([(CX + px, 620 - ph), (CX + px - 20, 620)], fill=(120, 170, 205), width=5)
    ground3(d, 620, (180, 210, 234), (156, 190, 218), (132, 170, 202))
    for x in (-240, -40, 180, 340):
        d.line([(CX + x, 720), (CX + x + 30, 820), (CX + x + 10, 900)], fill=(90, 140, 180), width=6)
    save(img, 'silB2_ледники')


def s_myorzlye_gazy():
    img, d = new_canvas()
    scene(d, (176, 198, 214), (150, 178, 198), 470)
    clouds(d, 170, (198, 216, 230), [-260, 100, 340], w=120)
    ground3(d, 520, (170, 198, 216), (146, 176, 198), (124, 156, 180))
    # мёрзлые бугры с ледяными жерлами, из которых рвётся пар
    for x, mh in [(-290, 230), (-30, 300), (240, 250)]:
        bx = CX + x
        d.polygon([(bx - 150, 700), (bx - 90, 700 - mh * 0.5), (bx, 700 - mh * 0.72), (bx + 90, 700 - mh * 0.5), (bx + 150, 700)], fill=(204, 222, 236), outline=BORDER, width=4)
        # жерло-воронка
        d.polygon([(bx - 52, 700 - mh * 0.72), (bx - 20, 700 - mh * 0.9), (bx + 20, 700 - mh * 0.9), (bx + 52, 700 - mh * 0.72)], fill=(58, 68, 82))
        d.line([(bx - 52, 700 - mh * 0.72), (bx - 20, 700 - mh * 0.9), (bx + 20, 700 - mh * 0.9), (bx + 52, 700 - mh * 0.72)], fill=BORDER, width=4)
        # столб пара клубами
        for i in range(4):
            py = 700 - mh * 0.95 - i * 90
            pw = 70 - i * 8
            d.ellipse([bx - pw, py - 40, bx + pw, py + 40], fill=(196, 212, 226), outline=BORDER, width=3)
    save(img, 'silB2_мёрзлые_газы')


def s_azotno_ledyanaya_tundra():
    img, d = new_canvas()
    scene(d, (200, 216, 230), (178, 202, 222), 470)
    clouds(d, 170, (222, 234, 244), [-260, 100, 340], w=120)
    # полигональные мерзлотные клинья с приподнятыми валиками и синими морозными пятнами
    ground3(d, 500, (150, 178, 204), (132, 162, 192), (114, 146, 178))
    for gx, gy, s in [(-300, 620, 130), (-60, 560, 120), (200, 640, 140), (60, 800, 150), (330, 820, 130), (-180, 860, 140)]:
        pts = [(CX + gx, gy - s), (CX + gx + s, gy), (CX + gx + s * 0.6, gy + s * 0.8), (CX + gx - s * 0.6, gy + s * 0.8), (CX + gx - s, gy)]
        d.polygon(pts, fill=(196, 214, 230), outline=BORDER, width=4)
        d.polygon([(CX + gx, gy - s * 0.72), (CX + gx + s * 0.72, gy), (CX + gx + s * 0.42, gy + s * 0.6), (CX + gx - s * 0.42, gy + s * 0.6), (CX + gx - s * 0.72, gy)], fill=(150, 190, 224), outline=BORDER, width=3)
        d.ellipse([CX + gx - s * 0.3, gy - s * 0.2, CX + gx + s * 0.3, gy + s * 0.25], fill=(226, 240, 250), outline=BORDER, width=3)
    save(img, 'silB2_азотно-ледяная_тундра')


def s_sukhoy_lyod():
    img, d = new_canvas()
    scene(d, (196, 208, 218), (176, 192, 206), 460)
    ground3(d, 520, (200, 214, 224), (176, 192, 208), (152, 170, 190))
    for x, w, h in [(-300, 120, 90), (-80, 150, 110), (180, 130, 95), (340, 110, 80)]:
        d.polygon([(CX + x - w, 800), (CX + x - w * 0.7, 800 - h), (CX + x + w * 0.6, 800 - h * 0.8), (CX + x + w, 800)], fill=(232, 238, 244), outline=BORDER, width=4)
    d.rectangle([0, 780, SIZE, 860], fill=(208, 218, 228))
    d.rectangle([0, 900, SIZE, 960], fill=(198, 208, 220))
    save(img, 'silB2_сухой_лёд')


def s_dzhungli():
    img, d = new_canvas()
    scene(d, (110, 150, 110), JUNGLE_D, 560)
    clouds(d, 180, (150, 180, 150), [-260, 100, 340], w=130)
    for tx in range(-360, 380, 90):
        tree(d, CX + tx, 600, 110 + (tx % 5) * 22, JUNGLE if tx % 2 == 0 else JUNGLE_D)
    for tx in range(-320, 340, 110):
        tree(d, CX + tx, 500, 70, (28, 96, 48))
    ground3(d, 640, (40, 108, 52), (30, 84, 42), (22, 66, 34))
    for lx in (-300, -140, 30, 200, 340):
        d.line([(CX + lx, 560), (CX + lx - 20, 700), (CX + lx + 10, 800)], fill=(60, 140, 70), width=7)
    for fx in (-330, -180, 60, 240, 380):
        d.ellipse([CX + fx - 44, 780, CX + fx + 44, 850], fill=(52, 130, 62), outline=BORDER, width=4)
    save(img, 'silB2_джунгли')


def s_tikhie_roshchi():
    img, d = new_canvas()
    scene(d, SKY_DUSK, (60, 78, 86), 500)
    clouds(d, 200, CLOUD_D, [-260, 100, 340], w=120, h=30)
    for tx in range(-340, 360, 80):
        s = 200 + (tx % 4) * 40
        d.line([(CX + tx, 500), (CX + tx + 6, 500 - s)], fill=(58, 76, 84), width=9)
        d.ellipse([CX + tx - 46, 500 - s - 30, CX + tx + 46, 500 - s + 30], fill=(52, 96, 74))
        d.line([(CX + tx - 46, 500 - s), (CX + tx + 46, 500 - s)], fill=BORDER, width=3)
    ground3(d, 560, (74, 92, 92), (58, 74, 76), (44, 58, 62))
    save(img, 'silB2_тихие_рощи_низкой_гравитации')


def s_bluzhdayushchie_roshchi():
    img, d = new_canvas()
    scene(d, SKY_SMOG, (120, 108, 88), 490)
    clouds(d, 180, CLOUD_D, [-240, 100, 340], w=120, h=30)
    for tx, s, tilt in [(-320, 130, -40), (-140, 160, 30), (60, 140, -30), (260, 170, 44), (390, 120, 20)]:
        bx = CX + tx
        topx = bx + tilt
        d.line([(bx, 560), (topx, 560 - s)], fill=BROWN, width=16)
        d.ellipse([topx - 60, 560 - s - 46, topx + 60, 560 - s + 34], fill=(58, 110, 64))
        d.line([(topx - 60, 560 - s), (topx + 60, 560 - s)], fill=BORDER, width=3)
        d.line([(bx, 560), (bx - 30, 620)], fill=BROWN, width=13)
        d.line([(bx, 560), (bx + 34, 625)], fill=BROWN, width=13)
        d.line([(bx, 560), (bx + 4, 635)], fill=(90, 66, 40), width=12)
    ground3(d, 600, (132, 120, 98), (112, 100, 82), (92, 82, 68))
    save(img, 'silB2_блуждающие_рощи')


def run():
    s_ledniki()
    s_myorzlye_gazy()
    s_azotno_ledyanaya_tundra()
    s_sukhoy_lyod()
    s_dzhungli()
    s_tikhie_roshchi()
    s_bluzhdayushchie_roshchi()


if __name__ == '__main__':
    run()
