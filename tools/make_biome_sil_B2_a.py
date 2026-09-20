# -*- coding: utf-8 -*-
# Часть A пачки 2: литосфера №1-7 (горы .. танцующие_пески). Стиль B.
from biome_sil_helpers import *  # noqa: F401,F403


def s_gori():
    img, d = new_canvas()
    scene(d, SKY_PALE, ROCK_D, 620)
    clouds(d, 180, CLOUD, [-300, 60, 340], w=130)
    for px, ph, pw, c in [(-330, 250, 130, ROCK), (-160, 360, 150, ROCK_L), (60, 300, 140, ROCK), (250, 340, 150, ROCK_L), (400, 220, 120, ROCK_D)]:
        d.polygon([(CX + px - pw, 620), (CX + px, 620 - ph), (CX + px + pw, 620)], fill=c)
        d.line([(CX + px - pw, 620), (CX + px, 620 - ph), (CX + px + pw, 620)], fill=BORDER, width=4)
        d.polygon([(CX + px - 30, 620 - ph * 0.72), (CX + px, 620 - ph), (CX + px + 34, 620 - ph * 0.72)], fill=SNOW)
    ground3(d, 640, (105, 100, 105), (80, 78, 86), (62, 62, 70))
    for gx in (-300, -60, 200, 370):
        d.line([(CX + gx, 840), (CX + gx + 12, 810)], fill=(70, 70, 78), width=7)
    save(img, 'silB2_горы')


def s_peski_pustyni():
    img, d = new_canvas()
    scene(d, (206, 178, 132), SAND, 460)
    d.ellipse([-300, 120, 500, 320], fill=(224, 200, 158))
    d.pieslice([CX - 620, 300, CX + 40, 900], 180, 360, fill=SAND)
    d.line([(CX - 620, 600), (CX + 40, 600)], fill=BORDER, width=4)
    d.pieslice([CX - 80, 420, CX + 620, 980], 180, 360, fill=SAND_D)
    d.line([(CX - 80, 700), (CX + 620, 700)], fill=BORDER, width=4)
    for y in (760, 830, 900):
        d.line([(60, y), (300, y - 24), (560, y - 8), (SIZE - 40, y - 26)], fill=(196, 158, 104), width=7)
    save(img, 'silB2_пески_пустыни')


def s_kratery():
    img, d = new_canvas()
    scene(d, (72, 72, 82), ROCK_D, 470)
    d.polygon([(0, 470), (CX - 320, 420), (CX - 40, 500), (CX + 260, 430), (SIZE, 490), (SIZE, 560), (0, 560)], fill=(96, 94, 100))
    d.line([(0, 470), (CX - 320, 420), (CX - 40, 500), (CX + 260, 430), (SIZE, 490)], fill=BORDER, width=5)
    ground3(d, 600, (92, 90, 96), (72, 70, 78), (54, 54, 62))
    # крупные кратеры с валиком (светлый обод) и тёмным дном
    for cx0, cy0, r in [(-270, 720, 130), (60, 800, 175), (330, 690, 110)]:
        d.ellipse([CX + cx0 - r - 34, cy0 - r * 0.55 - 20, CX + cx0 + r + 34, cy0 + r * 0.55 + 20], fill=(120, 116, 122))
        d.ellipse([CX + cx0 - r - 34, cy0 - r * 0.55 - 20, CX + cx0 + r + 34, cy0 + r * 0.55 + 20], outline=BORDER, width=6)
        d.ellipse([CX + cx0 - r, cy0 - r * 0.5, CX + cx0 + r, cy0 + r * 0.5], fill=(64, 62, 68))
        d.ellipse([CX + cx0 - r, cy0 - r * 0.5, CX + cx0 + r, cy0 + r * 0.5], outline=BORDER, width=5)
        d.ellipse([CX + cx0 - r * 0.78, cy0 - r * 0.4, CX + cx0 + r * 0.78, cy0 + r * 0.4], fill=(40, 39, 44))
        # выбросы по валику
        for exx in (-1, 0, 1):
            d.ellipse([CX + cx0 + exx * r * 0.9 - 22, cy0 - r * 0.55 - 30, CX + cx0 + exx * r * 0.9 + 22, cy0 - r * 0.55 + 2], fill=(130, 126, 132), outline=BORDER, width=3)
    save(img, 'silB2_кратеры')


def s_kamennye_pustoshchi():
    img, d = new_canvas()
    scene(d, (125, 122, 126), ROCK, 470)
    d.polygon([(0, 470), (CX - 300, 410), (CX - 40, 500), (CX + 260, 420), (SIZE, 480), (SIZE, 570), (0, 570)], fill=(110, 108, 114))
    d.line([(0, 470), (CX - 300, 410), (CX - 40, 500), (CX + 260, 420), (SIZE, 480)], fill=BORDER, width=5)
    ground3(d, 570, (100, 98, 104), (82, 80, 88), (64, 63, 70))
    for bx, bw, bh, c in [(-340, 70, 60, (120, 118, 124)), (-170, 55, 45, (96, 94, 102)), (40, 90, 75, (128, 126, 132)), (230, 65, 55, (100, 98, 106)), (370, 50, 40, (90, 88, 96))]:
        boulder(d, CX + bx, 820, bw, bh, c)
    d.line([(CX - 60, 880), (CX + 120, 900)], fill=BORDER, width=6)
    save(img, 'silB2_каменные_пустоши')


def s_solyanye_chashi():
    img, d = new_canvas()
    scene(d, (216, 224, 228), SALT, 430)
    d.rectangle([0, 430, SIZE, SIZE], fill=(232, 236, 236))
    d.polygon([(0, 500), (CX + 80, 470), (SIZE, 510), (SIZE, 590), (0, 590)], fill=(224, 230, 230))
    d.line([(0, 500), (CX + 80, 470), (SIZE, 510)], fill=BORDER, width=4)
    # сеть полигональных трещин с тёмным контуром (видно на белом)
    for a in range(-420, 460, 140):
        d.line([(CX + a, 600), (CX + a + 70, 720), (CX + a + 20, 860), (CX + a + 80, 1000)], fill=(176, 188, 194), width=8)
    for y in (640, 760, 880):
        d.line([(0, y), (300, y - 26), (640, y - 8), (SIZE, y - 28)], fill=(176, 188, 194), width=8)
    # соляные чаши-блюдца с тёмным ободом и дном
    for x, y, w, h in [(-300, 700, 150, 80), (40, 780, 190, 96), (300, 690, 130, 70)]:
        d.ellipse([CX + x - w, y - h, CX + x + w, y + h], fill=(196, 204, 208))
        d.ellipse([CX + x - w, y - h, CX + x + w, y + h], outline=BORDER, width=6)
        d.ellipse([CX + x - w * 0.7, y - h * 0.6, CX + x + w * 0.7, y + h * 0.6], fill=(246, 248, 248))
    # соляные столбики
    for x, h in [(-160, 120), (210, 150)]:
        d.polygon([(CX + x - 34, 780), (CX + x, 780 - h), (CX + x + 34, 780)], fill=(244, 246, 246), outline=BORDER, width=4)
    save(img, 'silB2_соляные_чаши')


def s_rakovinnaya_pustynya():
    img, d = new_canvas()
    scene(d, (208, 198, 176), BONE, 460)
    d.polygon([(0, 520), (CX - 260, 490), (CX + 40, 540), (SIZE, 500), (SIZE, 600), (0, 600)], fill=(210, 202, 182))
    d.line([(0, 520), (CX - 260, 490), (CX + 40, 540), (SIZE, 500)], fill=BORDER, width=4)
    ground3(d, 600, (218, 210, 190), (198, 190, 170), (176, 168, 150))
    # крупные раковины: заполненные спирали с рёбрами, тёмный контур — читаются в миниатюре
    def shell(x, y, s, c, edge):
        d.pieslice([x - s, y - s, x + s, y + s], 200, 350, fill=c)
        d.arc([x - s, y - s, x + s, y + s], 200, 350, fill=edge, width=max(6, s // 12))
        for rr in (0.66, 0.38):
            d.arc([x - s * rr, y - s * rr, x + s * rr, y + s * rr], 200, 350, fill=edge, width=max(5, s // 14))
        d.arc([x - s * 0.16, y - s * 0.16, x + s * 0.16, y + s * 0.16], 200, 350, fill=edge, width=5)
    shell(CX - 260, 720, 160, (238, 228, 206), (150, 138, 116))
    shell(CX + 70, 660, 120, (228, 218, 196), (150, 138, 116))
    shell(CX + 300, 800, 150, (238, 228, 206), (150, 138, 116))
    shell(CX - 70, 850, 100, (222, 212, 190), (150, 138, 116))
    save(img, 'silB2_раковинная_пустыня')


def s_tantsuyushchie_peski():
    img, d = new_canvas()
    scene(d, (198, 172, 128), SAND_D, 470)
    d.pieslice([CX - 620, 360, CX + 40, 940], 180, 360, fill=SAND)
    d.line([(CX - 620, 650), (CX + 40, 650)], fill=BORDER, width=4)
    d.pieslice([CX - 80, 480, CX + 620, 1000], 180, 360, fill=(190, 152, 100))
    d.line([(CX - 80, 740), (CX + 620, 740)], fill=BORDER, width=4)
    for x, h, w in [(-240, 420, 40), (-40, 520, 52), (180, 460, 46), (330, 340, 34)]:
        d.polygon([(CX + x - w, 660), (CX + x - w * 0.4, 660 - h * 0.5), (CX + x, 660 - h), (CX + x + w * 0.5, 660 - h * 0.5), (CX + x + w, 660)], fill=(228, 198, 150))
        d.line([(CX + x - w, 660), (CX + x - w * 0.4, 660 - h * 0.5), (CX + x, 660 - h), (CX + x + w * 0.5, 660 - h * 0.5), (CX + x + w, 660)], fill=BORDER, width=4)
    for y in (820, 880):
        d.line([(40, y), (340, y - 22), (700, y - 6), (SIZE - 40, y - 24)], fill=(200, 164, 112), width=7)
    save(img, 'silB2_танцующие_пески')


def run():
    s_gori()
    s_peski_pustyni()
    s_kratery()
    s_kamennye_pustoshchi()
    s_solyanye_chashi()
    s_rakovinnaya_pustynya()
    s_tantsuyushchie_peski()


if __name__ == '__main__':
    run()
