# -*- coding: utf-8 -*-
# Силуэты ИКОНОК биомов в стиле B (мини-пейзаж): горизонт, небо/атмосфера, среда-фон заполняет кадр.
# ВАЖНО: в небе НЕТ небесных тел (солнце/луны/звёзды/планеты) — только облака, дымка, атмосфера.
# 1024x1024, чёрный фон, композиция по центру.
import os
from PIL import Image, ImageDraw

OUT = r'C:\Zorion2\ai_drafts\biomes\silhouettes'
SIZE = 1024
CX, CY = SIZE // 2, SIZE // 2
BORDER = (12, 12, 12)

GREEN = (88, 160, 90)
GREEN_D = (50, 110, 60)
GRASS = (120, 170, 80)
BLUE = (70, 130, 200)
BLUE_L = (140, 190, 235)
TEAL = (60, 180, 180)
CYAN = (140, 230, 230)
PURPLE = (150, 100, 200)
MAGENTA = (210, 100, 170)
PINK = (230, 140, 170)
CORAL = (230, 120, 120)
RED = (200, 60, 50)
CRIMSON = (170, 40, 60)
ORANGE = (230, 120, 40)
FIRE = (240, 150, 60)
YELLOW = (220, 190, 70)
SULFUR = (200, 190, 90)
WHITE = (235, 235, 235)
ICE = (190, 225, 240)
STEEL = (120, 140, 170)
STEEL_D = (80, 95, 120)
BROWN = (130, 95, 60)
TAR = (70, 55, 45)
DARK = (40, 44, 55)
GLASS = (180, 210, 235)
GOLD = (210, 170, 60)

SKY_DAY = (120, 160, 200)
SKY_PALE = (150, 185, 215)
SKY_DUSK = (60, 90, 130)
SKY_TWILIGHT = (70, 60, 110)
SKY_CRIMSON = (165, 95, 80)
SKY_SULFUR = (170, 155, 95)
SKY_TEAL = (80, 130, 130)
SKY_SMOG = (105, 92, 80)
SKY_ASH = (112, 100, 105)
SKY_LAVA = (150, 90, 50)
SKY_ICE = (170, 195, 210)
SKY_NIGHT = (22, 26, 40)
CLOUD = (205, 220, 235)
CLOUD_D = (155, 170, 190)


def new_canvas():
    img = Image.new('RGB', (SIZE, SIZE), (5, 5, 5))
    return img, ImageDraw.Draw(img)


def save(img, name):
    os.makedirs(OUT, exist_ok=True)
    img.save(os.path.join(OUT, name + '.png'))
    print('Saved:', name)


def scene(d, sky, ground, hy):
    d.rectangle([0, 0, SIZE, hy], fill=sky)
    d.rectangle([0, hy, SIZE, SIZE], fill=ground)
    d.line([(0, hy), (SIZE, hy)], fill=BORDER, width=4)


def clouds(d, hy, color, xs, w=110, h=36):
    for cx in xs:
        d.ellipse([cx - w, hy - h, cx + w, hy + h], fill=color)
        d.ellipse([cx - w * 0.55, hy - h * 1.4, cx + w * 0.55, hy + h * 0.4], fill=color)


def tri(d, x, y, s, fill):
    d.polygon([(x, y - s), (x - s * 0.7, y), (x + s * 0.7, y)], fill=fill)
    d.line([(x, y - s), (x - s * 0.7, y), (x + s * 0.7, y), (x, y - s)], fill=BORDER, width=4)


def tree(d, cx, cy, s, crown, trunk=DARK):
    d.polygon([(cx, cy - s), (cx - s * 0.7, cy), (cx + s * 0.7, cy)], fill=crown)
    d.line([(cx, cy - s * 0.5), (cx, cy + s * 0.2)], fill=trunk, width=max(8, int(s * 0.1)))


# 1. Леса — густой лес в два ряда, подлесок, слоистый грунт, валежник
def silB_lesa():
    img, d = new_canvas()
    scene(d, SKY_PALE, GREEN_D, 600)
    clouds(d, 200, CLOUD, [-300, 60, 340], w=130)
    # гуще: два ряда деревьев
    for tx in range(-360, 380, 70):
        tree(d, CX + tx, 600, 90 + (tx % 4) * 25, GREEN if tx % 2 == 0 else GREEN_D)
    for tx in range(-320, 340, 120):
        tree(d, CX + tx, 490, 60, (55, 105, 62))
    # СЛОИСТЫЙ ГРУНТ
    d.polygon([(0, 640), (CX - 200, 620), (CX + 80, 650), (SIZE, 630), (SIZE, 760), (0, 760)], fill=(62, 112, 72))
    d.line([(0, 640), (CX - 200, 620), (CX + 80, 650), (SIZE, 630)], fill=BORDER, width=4)
    d.polygon([(0, 760), (CX - 100, 740), (CX + 120, 770), (SIZE, 750), (SIZE, SIZE), (0, SIZE)], fill=(45, 90, 55))
    d.line([(0, 760), (CX - 100, 740), (CX + 120, 770), (SIZE, 750)], fill=BORDER, width=4)
    # ПОДЛЕСОК: кусты
    for bx, bh in [(-330, 70), (-190, 90), (-70, 60), (70, 85), (200, 65), (330, 75)]:
        d.ellipse([bx - 34, 700 - bh, bx + 34, 700], fill=GRASS)
        d.line([(bx - 34, 700 - bh), (bx + 34, 700 - bh)], fill=BORDER, width=3)
    # КОРНИ у деревьев
    for tx in (-320, -180, -40, 90, 220, 350):
        d.line([(CX + tx, 600), (CX + tx - 22, 645)], fill=DARK, width=10)
    # ВАЛЕЖНИК: поваленное бревно
    d.polygon([(CX - 260, 800), (CX - 150, 818), (CX - 30, 792), (CX - 20, 776), (CX - 140, 797), (CX - 250, 782)], fill=BROWN)
    d.line([(CX - 260, 800), (CX - 150, 818), (CX - 30, 792)], fill=BORDER, width=4)
    d.ellipse([CX - 158, 812, CX - 142, 828], fill=(160, 120, 80))
    # трава на переднем плане
    for gx, gh in [(-340, 45), (-150, 55), (30, 48), (220, 60), (350, 42)]:
        d.line([(CX + gx, 760), (CX + gx + 10, 760 - gh)], fill=GRASS, width=7)
    save(img, 'silB_леса')


# 2. Луга и степи — холмы, трава, цветы
def silB_luga_steppi():
    img, d = new_canvas()
    scene(d, SKY_PALE, GRASS, 520)
    clouds(d, 190, CLOUD, [-260, 120, 330], w=120)
    d.pieslice([CX - 500, CY - 40, CX - 40, CY + 420], 180, 360, fill=GREEN)
    d.pieslice([CX + 60, CY - 120, CX + 500, CY + 380], 180, 360, fill=GREEN_D)
    for fx, fy in [(-260, CY + 40), (-120, CY + 80), (80, CY - 40), (220, CY + 10)]:
        d.ellipse([fx - 16, fy - 16, fx + 16, fy + 16], fill=YELLOW, outline=BORDER, width=3)
    for gx, gh in [(-320, 70), (-180, 90), (140, 80), (300, 65)]:
        d.line([(CX + gx, CY + 150), (CX + gx + 12, CY + 150 - gh)], fill=GREEN_D, width=8)
    save(img, 'silB_луга_степи')


# 3. Океаны — морской горизонт, волны
def silB_okeany():
    img, d = new_canvas()
    scene(d, SKY_PALE, BLUE, 430)
    clouds(d, 170, CLOUD, [-240, 180, 330], w=120)
    for i, (y, c) in enumerate([(500, BLUE_L), (610, BLUE), (720, BLUE), (830, BLUE_L), (930, BLUE)]):
        d.polygon([(0, y + 35), (0, y - 35), (240, y - 35), (330, y + 35), (520, y - 35), (640, y + 35), (840, y - 35), (SIZE, y - 35), (SIZE, y + 35)], fill=c)
        d.line([(0, y + 35), (0, y - 35), (240, y - 35), (330, y + 35), (520, y - 35), (640, y + 35), (840, y - 35), (SIZE, y - 35), (SIZE, y + 35), (SIZE, y + 35)], fill=BORDER, width=4)
        d.line([(240, y - 35), (330, y + 35)], fill=BORDER, width=4)
        d.line([(520, y - 35), (640, y + 35)], fill=BORDER, width=4)
    d.line([(0, 430), (SIZE, 430)], fill=WHITE, width=10)
    save(img, 'silB_океаны')


# 4. Болота — мутная вода, камыши, туман
def silB_bolota():
    img, d = new_canvas()
    scene(d, (110, 140, 120), (70, 120, 90), 480)
    d.rectangle([0, 480, SIZE, SIZE], fill=(60, 110, 85))
    for rx, h in [(-300, 210), (-160, 280), (40, 230), (200, 180), (310, 250)]:
        d.line([(CX + rx, 640), (CX + rx, 640 - h)], fill=GREEN_D, width=12)
        d.ellipse([CX + rx - 22, 640 - h - 60, CX + rx + 22, 640 - h + 10], fill=BROWN, outline=BORDER, width=3)
    d.rectangle([0, 640, SIZE, SIZE], fill=(70, 120, 90))
    d.rectangle([0, 560, SIZE, 620], fill=CLOUD_D)
    d.rectangle([0, 700, SIZE, 730], fill=CLOUD_D)
    save(img, 'silB_болота')


# 5. Коралловые рифы — мелкое море, кораллы над водой
def silB_korallovye_rifы():
    img, d = new_canvas()
    scene(d, SKY_PALE, TEAL, 460)
    d.rectangle([0, 460, SIZE, SIZE], fill=(50, 140, 140))
    d.polygon([(0, 720), (SIZE, 660), (SIZE, 800), (0, 800)], fill=(40, 120, 120))
    for bx, c in [(-260, PINK), (-120, PURPLE), (60, CORAL), (200, MAGENTA)]:
        d.line([(bx + CX, 700), (bx + CX - 30, 560), (bx + CX - 70, 480)], fill=c, width=28)
        d.line([(bx + CX, 700), (bx + CX + 25, 580), (bx + CX + 55, 500)], fill=c, width=20)
    d.pieslice([CX - 40, 470, CX + 110, 700], 190, 350, fill=TEAL)
    d.line([(CX - 40, 700), (CX - 40, 560)], fill=BORDER, width=4)
    d.ellipse([CX - 320, 760, CX - 220, 840], fill=BLUE_L, outline=BORDER, width=4)
    save(img, 'silB_коралловые_рифы')


# 6. Светящиеся чащи — гуще: больше светящихся деревьев, светящиеся кусты, слоистый грунт
def silB_svetyashchiesya_chashchi():
    img, d = new_canvas()
    scene(d, SKY_DUSK, (40, 60, 60), 520)
    # дальний тёмный ряд
    for tx in range(-340, 360, 120):
        tree(d, CX + tx, 440, 65, (35, 55, 55))
    # ближний ряд: больше деревьев
    for tx, s in [(-330, 105), (-200, 140), (-70, 95), (70, 150), (200, 115), (330, 85)]:
        tree(d, CX + tx, 520, s, GREEN_D)
    # МНОГО светящихся точек по всей чащe
    for gx, gy in [(-220, 380), (-130, 470), (-40, 350), (60, 330), (160, 420), (270, 370), (-280, 480), (30, 530), (220, 470), (330, 430), (-90, 540), (120, 560)]:
        d.ellipse([gx - 18, gy - 18, gx + 18, gy + 18], fill=CYAN, outline=BORDER, width=3)
    for gx, gy in [(-60, 300), (200, 310), (-180, 340)]:
        d.ellipse([gx - 12, gy - 12, gx + 12, gy + 12], fill=WHITE, outline=BORDER, width=3)
    # СЛОИСТЫЙ ГРУНТ
    d.polygon([(0, 620), (CX - 180, 600), (CX + 60, 630), (SIZE, 610), (SIZE, 740), (0, 740)], fill=(48, 68, 68))
    d.line([(0, 620), (CX - 180, 600), (CX + 60, 630), (SIZE, 610)], fill=BORDER, width=4)
    d.polygon([(0, 740), (CX - 80, 720), (CX + 100, 750), (SIZE, 730), (SIZE, SIZE), (0, SIZE)], fill=(35, 52, 52))
    d.line([(0, 740), (CX - 80, 720), (CX + 100, 750), (SIZE, 730)], fill=BORDER, width=4)
    # СВЕТЯЩИЕСЯ КУСТЫ на переднем плане (больше!)
    for bx, bh in [(-340, 70), (-210, 95), (-70, 60), (90, 85), (240, 75), (350, 55)]:
        d.ellipse([bx - 36, 700 - bh, bx + 36, 700], fill=(42, 66, 66))
        d.line([(bx - 36, 700 - bh), (bx + 36, 700 - bh)], fill=BORDER, width=3)
        for gx2 in (-20, 0, 20):
            d.ellipse([bx + gx2 - 7, 700 - bh - 12, bx + gx2 + 7, 700 - bh + 2], fill=CYAN, outline=BORDER, width=2)
    # светящиеся жилы в грунте
    d.line([(CX - 240, 660), (CX - 160, 705)], fill=CYAN, width=5)
    d.line([(CX + 130, 650), (CX + 210, 695)], fill=CYAN, width=5)
    d.rectangle([0, 620, SIZE, 650], fill=CLOUD_D)
    save(img, 'silB_светящиеся_чащи')


# 7. Багровые степи — красная степь, дымка
def silB_bagrovye_steppi():
    img, d = new_canvas()
    scene(d, SKY_CRIMSON, CRIMSON, 480)
    for wy, wl in [(300, 420), (240, 540), (180, 300)]:
        d.line([(CX - wl, wy), (CX + wl, wy - 40)], fill=(200, 110, 90), width=14)
    for tx, th in [(-300, 110), (-160, 150), (40, 120), (200, 160), (320, 100)]:
        d.line([(CX + tx, 480), (CX + tx + 14, 480 - th)], fill=RED, width=10)
    d.rectangle([0, 760, SIZE, SIZE], fill=CRIMSON)
    save(img, 'silB_багровые_степи')


# 8. Кислотные дебри — шипы, слоистый грунт, лужа с пузырями/бликами, растения в воде, валежник
def silB_kislotnye_debri():
    img, d = new_canvas()
    scene(d, (120, 160, 110), (90, 150, 80), 500)
    for px, h in [(-280, 200), (-160, 260), (-40, 170), (90, 240), (220, 200)]:
        d.polygon([(CX + px - 16, 500), (CX + px + 16, 500), (CX + px, 500 - h)], fill=(140, 220, 90))
        d.line([(CX + px - 16, 500), (CX + px + 16, 500), (CX + px, 500 - h), (CX + px - 16, 500)], fill=BORDER, width=4)
    # СЛОИСТЫЙ ГРУНТ
    d.polygon([(0, 590), (CX - 180, 570), (CX + 80, 600), (SIZE, 580), (SIZE, 710), (0, 710)], fill=(105, 165, 95))
    d.line([(0, 590), (CX - 180, 570), (CX + 80, 600), (SIZE, 580)], fill=BORDER, width=4)
    d.polygon([(0, 710), (CX - 80, 690), (CX + 100, 720), (SIZE, 700), (SIZE, SIZE), (0, SIZE)], fill=(88, 142, 80))
    d.line([(0, 710), (CX - 80, 690), (CX + 100, 720), (SIZE, 700)], fill=BORDER, width=4)
    # ЛУЖА кислоты: слои + пузыри + блики
    d.ellipse([CX - 290, 640, CX + 270, 840], fill=(110, 190, 90))
    d.ellipse([CX - 250, 660, CX + 230, 820], fill=(132, 205, 105))
    d.ellipse([CX - 210, 680, CX + 190, 800], fill=(152, 222, 122))
    for bx, by, br in [(-190, 730, 12), (-110, 765, 9), (30, 715, 14), (130, 755, 10), (70, 795, 8), (-40, 790, 7)]:
        d.ellipse([bx - br, by - br, bx + br, by + br], fill=(205, 252, 185), outline=BORDER, width=2)
    d.line([(CX - 260, 705), (CX - 200, 705)], fill=(235, 255, 215), width=5)
    d.line([(CX + 90, 725), (CX + 160, 725)], fill=(235, 255, 215), width=5)
    # растения в воде
    for rx in (-240, -70, 160):
        d.line([(rx, 830), (rx, 700)], fill=(60, 140, 80), width=8)
        d.ellipse([rx - 15, 688, rx + 15, 712], fill=(60, 140, 80), outline=BORDER, width=2)
    # валежник у края
    d.line([(CX + 210, 770), (CX + 320, 815)], fill=BROWN, width=15)
    d.line([(CX - 340, 790), (CX - 260, 820)], fill=BROWN, width=12)
    save(img, 'silB_кислотные_дебри')


# 9. Серные поля — кристаллы серы, паровые жерла
def silB_sernye_polya():
    img, d = new_canvas()
    scene(d, SKY_SULFUR, SULFUR, 470)
    for cx0, h, w in [(-270, 190, 70), (-150, 260, 85), (-30, 200, 60), (100, 280, 95), (230, 180, 70)]:
        d.polygon([(CX + cx0 - w, 470), (CX + cx0, 470 - h), (CX + cx0 + w, 470)], fill=YELLOW)
        d.line([(CX + cx0 - w, 470), (CX + cx0, 470 - h), (CX + cx0 + w, 470), (CX + cx0 - w, 470)], fill=BORDER, width=4)
        d.line([(CX + cx0, 470 - h), (CX + cx0, 470)], fill=BORDER, width=4)
    for vx in (-200, 40, 260):
        d.line([(CX + vx, 470), (CX + vx, 380)], fill=CLOUD_D, width=10)
        d.ellipse([CX + vx - 40, 350, CX + vx + 40, 400], fill=CLOUD_D)
    d.rectangle([0, 760, SIZE, SIZE], fill=SULFUR)
    save(img, 'silB_серные_поля')


# 10. Хемосинтетические сады — тёмные дымоходы со свечением
def silB_khemosinteticheskie_sady():
    img, d = new_canvas()
    scene(d, SKY_NIGHT, (35, 40, 50), 470)
    for tx, h, c in [(-280, 220, PURPLE), (-160, 300, MAGENTA), (-40, 180, PURPLE), (90, 280, TEAL), (210, 230, PURPLE)]:
        d.rectangle([CX + tx - 24, 470 - h, CX + tx + 24, 470], fill=c, outline=BORDER, width=4)
        d.ellipse([CX + tx - 30, 470 - h - 14, CX + tx + 30, 470 - h + 14], fill=c, outline=BORDER, width=3)
    for bx, by in [(-120, 620), (30, 560), (170, 640)]:
        d.ellipse([bx - 16, by - 16, bx + 16, by + 16], fill=CYAN, outline=BORDER, width=3)
    d.rectangle([0, 720, SIZE, SIZE], fill=(30, 35, 45))
    save(img, 'silB_хемосинтетические_сады')


# 11. Химический иней — шипы, слоистая наледь-короста, камни с наледью, трещины, сосульки
def silB_khimicheskiy_iney():
    img, d = new_canvas()
    scene(d, SKY_ICE, STEEL_D, 560)
    for sx, h in [(-280, 240), (-160, 320), (-40, 210), (90, 290), (220, 250)]:
        d.polygon([(CX + sx - 16, 560), (CX + sx + 16, 560), (CX + sx, 560 - h)], fill=WHITE)
        d.line([(CX + sx - 16, 560), (CX + sx + 16, 560), (CX + sx, 560 - h), (CX + sx - 16, 560)], fill=BORDER, width=4)
    for kx, s in [(-300, 55), (140, 60), (0, 40)]:
        pts = []
        for i in range(6):
            import math
            ang = math.radians(60 * i - 30)
            pts.append((CX + kx + s * math.cos(ang), 460 + s * math.sin(ang)))
        d.polygon(pts, fill=ICE, outline=BORDER, width=3)
    # СЛОИСТАЯ НАЛЕДЬ-КОРОСТА
    d.polygon([(0, 640), (CX - 160, 620), (CX + 80, 650), (SIZE, 630), (SIZE, 750), (0, 750)], fill=(125, 145, 160))
    d.line([(0, 640), (CX - 160, 620), (CX + 80, 650), (SIZE, 630)], fill=BORDER, width=4)
    d.polygon([(0, 750), (CX - 80, 730), (CX + 100, 760), (SIZE, 740), (SIZE, SIZE), (0, SIZE)], fill=(100, 118, 132))
    d.line([(0, 750), (CX - 80, 730), (CX + 100, 760), (SIZE, 740)], fill=BORDER, width=4)
    # ТРЕЩИНЫ во льду
    d.line([(CX - 250, 680), (CX - 190, 730)], fill=BORDER, width=5)
    d.line([(CX - 190, 730), (CX - 150, 770)], fill=BORDER, width=5)
    d.line([(CX + 100, 670), (CX + 170, 720)], fill=BORDER, width=5)
    d.line([(CX + 170, 720), (CX + 210, 755)], fill=BORDER, width=5)
    # КАМНИ С НАЛЕДЬЮ на переднем плане
    for rx, rw, rh in [(-340, 62, 45), (-150, 82, 55), (40, 72, 40), (210, 92, 60), (350, 58, 38)]:
        d.polygon([(rx, 810), (rx + rw // 2, 810 - rh), (rx + rw, 810)], fill=(150, 170, 185))
        d.line([(rx, 810), (rx + rw // 2, 810 - rh), (rx + rw, 810)], fill=BORDER, width=4)
        d.line([(rx, 810), (rx + rw // 2, 810 - rh * 0.5)], fill=WHITE, width=3)
    # сосульки, свисающие с камней
    for sx in (-320, -120, 90, 300):
        d.line([(sx, 810), (sx + 7, 870)], fill=ICE, width=9)
    save(img, 'silB_химический_иней')


# 12. Метановые моря — горизонт метанового моря, БЕЗ ЛУН
def silB_metanovye_morya():
    img, d = new_canvas()
    scene(d, (110, 155, 175), (120, 170, 195), 430)
    d.rectangle([0, 430, SIZE, SIZE], fill=(100, 155, 180))
    for i, (y, c) in enumerate([(520, ICE), (630, (150, 200, 220)), (740, (130, 180, 205)), (850, ICE)]):
        d.polygon([(0, y + 35), (0, y - 35), (250, y - 35), (340, y + 35), (530, y - 35), (650, y + 35), (850, y - 35), (SIZE, y - 35), (SIZE, y + 35)], fill=c)
        d.line([(0, y + 35), (0, y - 35), (250, y - 35), (340, y + 35), (530, y - 35), (650, y + 35), (850, y - 35), (SIZE, y - 35), (SIZE, y + 35), (SIZE, y + 35)], fill=BORDER, width=4)
        d.line([(250, y - 35), (340, y + 35)], fill=BORDER, width=4)
        d.line([(530, y - 35), (650, y + 35)], fill=BORDER, width=4)
    d.line([(0, 430), (SIZE, 430)], fill=WHITE, width=10)
    clouds(d, 210, CLOUD_D, [-320, 120, 360], w=120, h=30)
    save(img, 'silB_метановые_моря')


# 13. Углеводородные равнины — башня, слоистый грунт, несколько луж тара с бликами, камни, трещины
def silB_uglevodorodnye_ravniny():
    img, d = new_canvas()
    scene(d, SKY_SMOG, (70, 60, 50), 420)
    # вентиляционная башня
    d.rectangle([CX - 26, 250, CX + 26, 420], fill=STEEL, outline=BORDER, width=4)
    d.line([(CX - 26, 250), (CX + 26, 250)], fill=BORDER, width=4)
    d.ellipse([CX - 44, 220, CX + 44, 280], fill=STEEL, outline=BORDER, width=3)
    d.rectangle([CX - 12, 190, CX + 12, 230], fill=STEEL_D, outline=BORDER, width=3)
    # СЛОИСТЫЙ ГРУНТ (3 плана)
    d.polygon([(0, 470), (CX - 200, 450), (CX + 80, 480), (SIZE, 460), (SIZE, 610), (0, 610)], fill=(88, 72, 60))
    d.line([(0, 470), (CX - 200, 450), (CX + 80, 480), (SIZE, 460)], fill=BORDER, width=4)
    d.polygon([(0, 610), (CX - 100, 590), (CX + 120, 620), (SIZE, 600), (SIZE, 760), (0, 760)], fill=(72, 60, 50))
    d.line([(0, 610), (CX - 100, 590), (CX + 120, 620), (SIZE, 600)], fill=BORDER, width=4)
    d.polygon([(0, 760), (CX - 80, 740), (CX + 100, 770), (SIZE, 750), (SIZE, SIZE), (0, SIZE)], fill=(58, 48, 41))
    d.line([(0, 760), (CX - 80, 740), (CX + 100, 770), (SIZE, 750)], fill=BORDER, width=4)
    # ЛУЖИ тара с бликами (три)
    d.ellipse([CX - 310, 650, CX - 110, 790], fill=TAR, outline=BORDER, width=4)
    d.line([(CX - 265, 710), (CX - 195, 710)], fill=WHITE, width=6)
    d.line([(CX - 240, 730), (CX - 200, 730)], fill=(130, 120, 110), width=5)
    d.ellipse([CX + 50, 630, CX + 220, 770], fill=TAR, outline=BORDER, width=4)
    d.line([(CX + 100, 700), (CX + 170, 700)], fill=WHITE, width=6)
    d.ellipse([CX + 240, 780, CX + 390, 900], fill=TAR, outline=BORDER, width=4)
    d.line([(CX + 285, 845), (CX + 345, 845)], fill=WHITE, width=5)
    # КАМНИ и трещины
    d.polygon([(CX - 130, 810), (CX - 60, 775), (CX + 20, 795), (CX - 20, 840)], fill=(98, 80, 64))
    d.line([(CX - 130, 810), (CX - 60, 775), (CX + 20, 795)], fill=BORDER, width=4)
    d.line([(CX + 150, 555), (CX + 210, 605)], fill=BORDER, width=5)
    d.line([(CX + 210, 605), (CX + 245, 650)], fill=BORDER, width=5)
    d.line([(CX - 340, 700), (CX - 290, 740)], fill=BORDER, width=5)
    save(img, 'silB_углеводородные_равнины')


# 14. Инеевые рощи — иней на деревьях, белое небо
def silB_ineevye_roshchi():
    img, d = new_canvas()
    scene(d, (200, 225, 235), (180, 210, 220), 520)
    clouds(d, 200, WHITE, [-280, 80, 340], w=130)
    for tx in range(-330, 350, 110):
        tree(d, CX + tx, 520, 100 + (tx % 3) * 30, ICE)
    d.rectangle([0, 760, SIZE, SIZE], fill=WHITE)
    save(img, 'silB_инеевые_рощи')


# 15. Лавовые поля — лава в трещинах, дымное небо (огонь допустим)
def silB_lavovye_polya():
    img, d = new_canvas()
    scene(d, SKY_LAVA, DARK, 460)
    d.rectangle([0, 460, SIZE, SIZE], fill=(40, 38, 40))
    d.polygon([(0, 640), (240, 600), (260, 680), (520, 620), (540, 720), (SIZE, 660), (SIZE, SIZE), (0, SIZE)], fill=DARK)
    d.polygon([(60, 660), (60, 610), (240, 600), (260, 680)], fill=FIRE)
    d.polygon([(380, 630), (380, 580), (520, 620), (540, 720)], fill=FIRE)
    d.line([(280, 700), (460, 660)], fill=ORANGE, width=16)
    d.line([(0, 780), (SIZE, 760)], fill=FIRE, width=14)
    d.ellipse([140, 520, 220, 590], fill=FIRE, outline=BORDER, width=4)
    save(img, 'silB_лавовые_поля')


# 16. Магмовый океан — раскалённое море, светящееся небо (огонь допустим)
def silB_magmovyy_okean():
    img, d = new_canvas()
    scene(d, SKY_LAVA, RED, 420)
    d.rectangle([0, 420, SIZE, SIZE], fill=ORANGE)
    for i, (y, c) in enumerate([(510, RED), (620, FIRE), (730, ORANGE), (840, RED)]):
        d.polygon([(0, y + 40), (0, y - 40), (250, y - 40), (340, y + 40), (530, y - 40), (650, y + 40), (850, y - 40), (SIZE, y - 40), (SIZE, y + 40)], fill=c)
        d.line([(0, y + 40), (0, y - 40), (250, y - 40), (340, y + 40), (530, y - 40), (650, y + 40), (850, y - 40), (SIZE, y - 40), (SIZE, y + 40), (SIZE, y + 40)], fill=BORDER, width=4)
        d.line([(250, y - 40), (340, y + 40)], fill=BORDER, width=4)
        d.line([(530, y - 40), (650, y + 40)], fill=BORDER, width=4)
    d.line([(0, 420), (SIZE, 420)], fill=(250, 190, 90), width=12)
    save(img, 'silB_магмовый_океан')


# 17. Вулканические поля — конусы, потоки лавы, рельефные гряды, слоистое лавовое озеро (огонь допустим)
def silB_vulkanicheskie_polya():
    img, d = new_canvas()
    scene(d, SKY_ASH, (55, 50, 55), 470)
    # дымовые столбы над кратерами
    for sx in (-160, 200):
        d.ellipse([CX + sx - 60, 190, CX + sx + 60, 300], fill=CLOUD_D)
        d.ellipse([CX + sx - 40, 140, CX + sx + 40, 240], fill=CLOUD_D)
        d.line([CX + sx, 300, CX + sx, 400], fill=CLOUD_D, width=16)
    # три конуса с кратерами
    d.polygon([(CX - 350, 470), (CX - 220, 210), (CX - 90, 470)], fill=(70, 58, 62))
    d.line([(CX - 350, 470), (CX - 220, 210), (CX - 90, 470), (CX - 350, 470)], fill=BORDER, width=4)
    d.ellipse([CX - 245, 190, CX - 195, 240], fill=FIRE, outline=BORDER, width=3)
    d.polygon([(CX - 50, 470), (CX + 90, 170), (CX + 230, 470)], fill=(80, 65, 70))
    d.line([(CX - 50, 470), (CX + 90, 170), (CX + 230, 470), (CX - 50, 470)], fill=BORDER, width=4)
    d.ellipse([CX + 65, 150, CX + 115, 200], fill=FIRE, outline=BORDER, width=3)
    d.polygon([(CX + 260, 470), (CX + 325, 330), (CX + 390, 470)], fill=(60, 50, 55))
    d.line([(CX + 260, 470), (CX + 325, 330), (CX + 390, 470), (CX + 260, 470)], fill=BORDER, width=4)
    d.ellipse([CX + 305, 315, CX + 345, 355], fill=ORANGE, outline=BORDER, width=3)
    # потоки лавы вниз
    d.line([(CX - 220, 250), (CX - 310, 470)], fill=FIRE, width=16)
    d.line([(CX + 90, 210), (CX + 0, 470)], fill=ORANGE, width=14)
    d.line([(CX + 325, 370), (CX + 280, 470)], fill=FIRE, width=10)
    # РЕЛЬЕФ: две гряды скал разного тона между горизонтом и озером
    d.polygon([(0, 620), (CX - 280, 570), (CX - 60, 650), (CX + 220, 585), (SIZE, 640), (SIZE, 790), (0, 790)], fill=(68, 60, 62))
    d.line([(0, 620), (CX - 280, 570), (CX - 60, 650), (CX + 220, 585), (SIZE, 640)], fill=BORDER, width=5)
    d.polygon([(0, 690), (CX - 180, 630), (CX + 60, 680), (SIZE, 635), (SIZE, 790), (0, 790)], fill=(47, 43, 47))
    d.line([(0, 690), (CX - 180, 630), (CX + 60, 680), (SIZE, 635)], fill=BORDER, width=5)
    # трещины с лавой в грядах
    d.line([(CX - 190, 600), (CX - 130, 670)], fill=FIRE, width=8)
    d.line([(CX + 130, 610), (CX + 190, 680)], fill=ORANGE, width=8)
    d.line([(CX + 260, 655), (CX + 310, 710)], fill=FIRE, width=6)
    # ЛАВОВОЕ ОЗЕРО: три слоя + корка-острова + блики
    d.polygon([(0, 800), (CX - 120, 770), (CX + 80, 810), (SIZE, 780), (SIZE, SIZE), (0, SIZE)], fill=FIRE)
    d.line([(0, 800), (CX - 120, 770), (CX + 80, 810), (SIZE, 780)], fill=BORDER, width=5)
    d.polygon([(0, 860), (CX - 60, 835), (CX + 130, 870), (SIZE, 845), (SIZE, SIZE), (0, SIZE)], fill=ORANGE)
    d.line([(0, 860), (CX - 60, 835), (CX + 130, 870), (SIZE, 845)], fill=BORDER, width=4)
    d.polygon([(0, 925), (CX + 60, 905), (SIZE, 920), (SIZE, SIZE), (0, SIZE)], fill=RED)
    d.ellipse([CX - 230, 825, CX - 130, 890], fill=(52, 46, 49), outline=BORDER, width=4)
    d.ellipse([CX + 100, 835, CX + 195, 900], fill=(56, 49, 51), outline=BORDER, width=4)
    d.line([(CX - 40, 875), (CX + 40, 870)], fill=(255, 220, 120), width=8)
    d.line([(CX + 210, 890), (CX + 270, 885)], fill=(255, 220, 120), width=7)
    save(img, 'silB_вулканические_поля')


# 18. Стеклянные поля — наложенные гранёные осколки, блики, слоистый грунт, россыпь на переднем плане
def silB_steklyannye_polya():
    img, d = new_canvas()
    scene(d, (120, 165, 195), (90, 110, 130), 480)
    # СЛОИСТЫЙ ГРУНТ (градиент тонов)
    d.polygon([(0, 550), (CX - 160, 530), (CX + 100, 560), (SIZE, 545), (SIZE, 660), (0, 660)], fill=(85, 105, 125))
    d.line([(0, 550), (CX - 160, 530), (CX + 100, 560), (SIZE, 545)], fill=BORDER, width=4)
    d.polygon([(0, 660), (CX - 80, 640), (CX + 120, 670), (SIZE, 650), (SIZE, SIZE), (0, SIZE)], fill=(72, 92, 112))
    d.line([(0, 660), (CX - 80, 640), (CX + 120, 670), (SIZE, 650)], fill=BORDER, width=4)
    # дальний ряд осколков
    for sx, h, w in [(-300, 160, 45), (-180, 220, 60), (-50, 140, 38), (90, 200, 52), (240, 160, 42)]:
        d.polygon([(CX + sx - w, 480), (CX + sx, 480 - h), (CX + sx + w, 480)], fill=GLASS)
        d.line([(CX + sx - w, 480), (CX + sx, 480 - h), (CX + sx + w, 480), (CX + sx - w, 480)], fill=BORDER, width=4)
        d.line([(CX + sx - w, 480), (CX + sx, 480 - h * 0.5)], fill=(220, 235, 250), width=3)
    # передние большие осколки (слева и справа)
    d.polygon([(CX - 380, 640), (CX - 290, 250), (CX - 130, 640)], fill=GLASS)
    d.line([(CX - 380, 640), (CX - 290, 250), (CX - 130, 640), (CX - 380, 640)], fill=BORDER, width=5)
    d.line([(CX - 290, 250), (CX - 255, 640)], fill=(235, 245, 255), width=6)
    d.line([(CX - 380, 640), (CX - 290, 250)], fill=BORDER, width=5)
    d.polygon([(CX + 110, 620), (CX + 230, 180), (CX + 370, 620)], fill=(160, 200, 230))
    d.line([(CX + 110, 620), (CX + 230, 180), (CX + 370, 620), (CX + 110, 620)], fill=BORDER, width=5)
    d.line([(CX + 230, 180), (CX + 240, 620)], fill=(235, 245, 255), width=6)
    d.line([(CX + 230, 180), (CX + 110, 620)], fill=BORDER, width=5)
    # РОССЫПЬ мелких осколков на переднем плане
    for sx, h, w, c in [(-340, 75, 22, ICE), (-250, 95, 26, GLASS), (-150, 60, 18, ICE), (-40, 105, 28, GLASS),
                        (60, 70, 20, ICE), (160, 100, 27, GLASS), (280, 80, 23, ICE), (370, 60, 17, GLASS)]:
        d.polygon([(sx, 760), (sx + 12, 760 - h), (sx + 24, 760)], fill=c)
        d.line([(sx, 760), (sx + 12, 760 - h), (sx + 24, 760), (sx, 760)], fill=BORDER, width=3)
        d.line([(sx, 760), (sx + 12, 760 - h * 0.5)], fill=(240, 248, 255), width=2)
    # блики-грани в нижней части
    d.line([(CX - 200, 700), (CX - 150, 735)], fill=(235, 245, 255), width=5)
    d.line([(CX + 60, 710), (CX + 110, 745)], fill=(235, 245, 255), width=5)
    d.line([(CX + 260, 720), (CX + 300, 750)], fill=(235, 245, 255), width=4)
    # дымка
    d.rectangle([0, 600, SIZE, 635], fill=CLOUD_D)
    save(img, 'silB_стеклянные_поля')


# 19. Кристальные рощи — кристаллы-деревья, корни, светящиеся жилы, валуны, слоистый грунт
def silB_kristalnye_roshchi():
    img, d = new_canvas()
    scene(d, SKY_TWILIGHT, DARK, 480)

    def crystal_tree(x, base_y, s, c1, c2):
        d.polygon([(x - s * 0.13, base_y), (x, base_y - s * 2.1), (x + s * 0.13, base_y)], fill=c2)
        d.line([(x - s * 0.13, base_y), (x, base_y - s * 2.1), (x + s * 0.13, base_y)], fill=BORDER, width=4)
        for t, w in [(0.55, 0.62), (0.85, 0.44), (1.15, 0.26)]:
            y = base_y - s * t
            d.polygon([(x - s * w, y), (x, y - s * 0.55), (x + s * w, y)], fill=c1)
            d.line([(x - s * w, y), (x, y - s * 0.55), (x + s * w, y)], fill=BORDER, width=4)
        d.polygon([(x - s * 0.1, base_y - s * 1.6), (x, base_y - s * 2.05), (x + s * 0.1, base_y - s * 1.6)], fill=c1)

    crystal_tree(CX - 270, 480, 130, PURPLE, MAGENTA)
    crystal_tree(CX - 60, 480, 170, CYAN, PURPLE)
    crystal_tree(CX + 190, 480, 145, PURPLE, MAGENTA)
    # СЛОИСТЫЙ ГРУНТ (градиент тонов)
    d.polygon([(0, 540), (CX - 210, 520), (CX + 60, 550), (SIZE, 530), (SIZE, 650), (0, 650)], fill=(52, 57, 70))
    d.line([(0, 540), (CX - 210, 520), (CX + 60, 550), (SIZE, 530)], fill=BORDER, width=4)
    d.polygon([(0, 650), (CX - 110, 630), (CX + 80, 660), (SIZE, 645), (SIZE, SIZE), (0, SIZE)], fill=(36, 40, 50))
    d.line([(0, 650), (CX - 110, 630), (CX + 80, 660), (SIZE, 645)], fill=BORDER, width=4)
    # КОРНИ-кристаллы у оснований деревьев
    for bx, c in [(CX - 330, CYAN), (CX - 200, PURPLE), (CX + 30, CYAN), (CX + 160, PURPLE), (CX + 290, CYAN)]:
        d.polygon([(bx - 12, 600), (bx, 600 - 70), (bx + 12, 600)], fill=c)
        d.line([(bx - 12, 600), (bx, 600 - 70), (bx + 12, 600)], fill=BORDER, width=3)
    # СВЕТЯЩИЕСЯ ЖИЛЫ в грунте
    d.line([(CX - 290, 560), (CX - 220, 610)], fill=CYAN, width=5)
    d.line([(CX - 180, 630), (CX - 90, 680)], fill=PURPLE, width=5)
    d.line([(CX + 60, 570), (CX + 150, 620)], fill=CYAN, width=5)
    # ВАЛУНЫ с гранями на переднем плане
    d.polygon([(CX - 330, 800), (CX - 250, 720), (CX - 150, 750), (CX - 170, 830)], fill=(58, 62, 74))
    d.line([(CX - 330, 800), (CX - 250, 720), (CX - 150, 750), (CX - 170, 830)], fill=BORDER, width=4)
    d.line([(CX - 250, 720), (CX - 170, 830)], fill=(120, 130, 150), width=4)
    d.polygon([(CX + 170, 780), (CX + 260, 700), (CX + 360, 740), (CX + 320, 820)], fill=(50, 54, 66))
    d.line([(CX + 170, 780), (CX + 260, 700), (CX + 360, 740), (CX + 320, 820)], fill=BORDER, width=4)
    d.line([(CX + 260, 700), (CX + 320, 820)], fill=(120, 130, 150), width=4)
    # малые кристаллы в грунте
    for sx, h in [(CX - 80, 60), (CX + 110, 75), (CX + 30, 50)]:
        d.polygon([(sx - 12, 700), (sx, 700 - h), (sx + 12, 700)], fill=CYAN)
        d.line([(sx - 12, 700), (sx, 700 - h), (sx + 12, 700)], fill=BORDER, width=3)
    # дымка
    d.rectangle([0, 525, SIZE, 555], fill=CLOUD_D)
    save(img, 'silB_кристальные_рощи')


# 20. Кремниевые рощи — слоистые дюны (3 плана), шпили, монолит, страты, изломы
def silB_kremnievye_roshchi():
    img, d = new_canvas()
    scene(d, SKY_TEAL, STEEL_D, 480)
    # задняя дюна + средняя дюна
    d.pieslice([CX - 520, 300, CX - 60, 760], 180, 360, fill=(95, 115, 135))
    d.line([(CX - 520, 530), (CX - 60, 530)], fill=BORDER, width=4)
    d.pieslice([CX - 40, 380, CX + 520, 780], 180, 360, fill=(105, 125, 145))
    d.line([(CX - 40, 580), (CX + 520, 580)], fill=BORDER, width=4)
    # ПЕРЕДНЯЯ дюна (третий план, темнее)
    d.pieslice([CX - 120, 520, CX + 620, 950], 180, 360, fill=(88, 106, 126))
    d.line([(CX - 120, 735), (CX + 620, 735)], fill=BORDER, width=4)
    # СТРАТЫ (слоистые линии) на дюнах
    for wy in (610, 670, 730, 790, 850):
        d.line([(40, wy), (180, wy - 18), (340, wy - 8), (520, wy - 24), (SIZE - 40, wy - 12)], fill=(70, 88, 108), width=6)
    # шпили на средней дюне
    for sx, h, w in [(-250, 190, 40), (-120, 260, 50), (30, 150, 34), (160, 230, 46)]:
        d.polygon([(CX + sx - w, 530), (CX + sx, 530 - h), (CX + sx + w, 530)], fill=STEEL)
        d.line([(CX + sx - w, 530), (CX + sx, 530 - h), (CX + sx + w, 530), (CX + sx - w, 530)], fill=BORDER, width=4)
        d.line([(CX + sx - w, 530), (CX + sx, 530 - h * 0.5)], fill=(170, 195, 215), width=3)
    # монолит слева
    d.polygon([(CX - 390, 700), (CX - 320, 420), (CX - 220, 380), (CX - 140, 700)], fill=(120, 135, 155))
    d.line([(CX - 390, 700), (CX - 320, 420), (CX - 220, 380), (CX - 140, 700), (CX - 390, 700)], fill=BORDER, width=5)
    d.line([(CX - 320, 420), (CX - 220, 380)], fill=BORDER, width=5)
    d.line([(CX - 320, 420), (CX - 140, 700)], fill=(180, 200, 220), width=4)
    # ОБЛОМКИ-глыбы на передней дюне
    for gx, gw, gh in [(40, 70, 55), (220, 90, 70), (430, 60, 45), (620, 80, 60)]:
        d.polygon([(gx, 820), (gx + gw // 2, 820 - gh), (gx + gw, 820)], fill=(112, 128, 148))
        d.line([(gx, 820), (gx + gw // 2, 820 - gh), (gx + gw, 820), (gx, 820)], fill=BORDER, width=4)
        d.line([(gx, 820), (gx + gw // 2, 820 - gh * 0.5)], fill=(175, 195, 215), width=3)
    # малые шпили на передней дюне
    for sx, h in [(-60, 90), (140, 110)]:
        d.polygon([(CX + sx - 14, 735), (CX + sx, 735 - h), (CX + sx + 14, 735)], fill=STEEL)
        d.line([(CX + sx - 14, 735), (CX + sx, 735 - h), (CX + sx + 14, 735)], fill=BORDER, width=3)
    # ИЗЛОМЫ-трещины
    d.line([(CX + 250, 660), (CX + 330, 720)], fill=BORDER, width=6)
    d.line([(CX + 330, 720), (CX + 380, 760)], fill=BORDER, width=6)
    d.line([(CX - 120, 640), (CX - 40, 690)], fill=BORDER, width=6)
    d.line([(CX - 40, 690), (CX + 10, 740)], fill=BORDER, width=6)
    # дымка
    d.rectangle([0, 600, SIZE, 635], fill=CLOUD_D)
    save(img, 'silB_кремниевые_рощи')


# 21. Металлические щетинные поля — дальние щетинки, КРУПНЫЕ передние иглы, слоистый грунт, плиты
def silB_metallicheskie_shchetinnye_polya():
    img, d = new_canvas()
    scene(d, (135, 145, 155), STEEL_D, 500)
    import random
    random.seed(11)
    for _ in range(26):
        bx = random.randint(-420, 420)
        bh = random.randint(80, 220)
        d.line([(CX + bx, 500), (CX + bx + 6, 500 - bh)], fill=STEEL, width=7)
    for bx, bh in [(-300, 260), (-140, 300), (60, 280), (220, 310)]:
        d.line([(CX + bx, 500), (CX + bx + 6, 500 - bh)], fill=(200, 215, 235), width=9)
    # СЛОИСТЫЙ ГРУНТ
    d.polygon([(0, 610), (CX - 160, 590), (CX + 80, 620), (SIZE, 600), (SIZE, 730), (0, 730)], fill=(98, 112, 130))
    d.line([(0, 610), (CX - 160, 590), (CX + 80, 620), (SIZE, 600)], fill=BORDER, width=4)
    d.polygon([(0, 730), (CX - 80, 710), (CX + 100, 740), (SIZE, 720), (SIZE, SIZE), (0, SIZE)], fill=(80, 92, 108))
    d.line([(0, 730), (CX - 80, 710), (CX + 100, 740), (SIZE, 720)], fill=BORDER, width=4)
    # КРУПНЫЕ передние иглы (толстые, с бликом)
    for bx, bh in [(-330, 170), (-190, 240), (-50, 140), (90, 210), (240, 180), (370, 120)]:
        d.polygon([(bx, 770), (bx + 16, 770 - bh), (bx + 32, 770)], fill=(150, 168, 188))
        d.line([(bx, 770), (bx + 16, 770 - bh), (bx + 32, 770), (bx, 770)], fill=BORDER, width=4)
        d.line([(bx, 770), (bx + 16, 770 - bh * 0.5)], fill=(215, 228, 245), width=3)
    # сломанные иглы на земле
    d.line([(CX - 260, 810), (CX - 170, 845)], fill=STEEL, width=11)
    d.line([(CX + 30, 820), (CX + 150, 850)], fill=STEEL, width=11)
    d.line([(CX + 230, 800), (CX + 310, 835)], fill=STEEL, width=10)
    # плиты-камни
    d.polygon([(CX - 390, 860), (CX - 330, 820), (CX - 250, 830), (CX - 270, 875)], fill=(110, 124, 142))
    d.line([(CX - 390, 860), (CX - 330, 820), (CX - 250, 830)], fill=BORDER, width=4)
    d.polygon([(CX + 200, 850), (CX + 280, 810), (CX + 370, 825), (CX + 340, 870)], fill=(105, 118, 136))
    d.line([(CX + 200, 850), (CX + 280, 810), (CX + 370, 825)], fill=BORDER, width=4)
    save(img, 'silB_металлические_щетинные_поля')


# 22. Струнные рощи — струны до низа, слоистый грунт, свёрнутые волокна, свечение у земли
def silB_strunnye_roshchi():
    img, d = new_canvas()
    scene(d, SKY_DUSK, (50, 60, 70), 500)
    # струны: вверх и вниз до самого низа
    for i, sx in enumerate(range(-400, 420, 40)):
        sway = (i % 3) * 12 - 12
        d.line([(CX + sx, 500), (CX + sx + sway, 200)], fill=(90, 100, 120), width=5)
        d.line([(CX + sx, 500), (CX + sx + sway * 2, 840)], fill=(70, 80, 100), width=5)
    # подсвеченные струны (яркие, до низа)
    for sx in (-280, -90, 90, 280):
        d.line([(CX + sx, 500), (CX + sx, 180)], fill=CYAN, width=7)
        d.line([(CX + sx, 500), (CX + sx + 12, 840)], fill=(120, 200, 210), width=6)
    # СЛОИСТЫЙ ГРУНТ
    d.polygon([(0, 630), (CX - 160, 610), (CX + 80, 640), (SIZE, 620), (SIZE, 750), (0, 750)], fill=(64, 74, 86))
    d.line([(0, 630), (CX - 160, 610), (CX + 80, 640), (SIZE, 620)], fill=BORDER, width=4)
    d.polygon([(0, 750), (CX - 80, 730), (CX + 100, 760), (SIZE, 740), (SIZE, SIZE), (0, SIZE)], fill=(46, 55, 65))
    d.line([(0, 750), (CX - 80, 730), (CX + 100, 760), (SIZE, 740)], fill=BORDER, width=4)
    # СВЕРНУВШИЕСЯ волокна-спирали на земле
    for cx2, cy2 in [(-260, 780), (-50, 820), (180, 790)]:
        d.ellipse([cx2 - 32, cy2 - 16, cx2 + 32, cy2 + 16], fill=(90, 100, 120))
        d.line([(cx2 - 32, cy2 - 16), (cx2 + 32, cy2 - 16)], fill=BORDER, width=3)
        d.line([(cx2 - 32, cy2), (cx2 + 32, cy2)], fill=(150, 165, 185), width=4)
        d.line([(cx2 - 20, cy2 + 8), (cx2 + 20, cy2 + 8)], fill=(110, 122, 140), width=4)
    # светящиеся точки у земли (свечение струн у низа)
    for gx, gy in [(-200, 700), (40, 745), (250, 690)]:
        d.ellipse([gx - 13, gy - 13, gx + 13, gy + 13], fill=CYAN, outline=BORDER, width=3)
    d.ellipse([-30 + CX, 640, 40 + CX, 710], fill=CYAN, outline=BORDER, width=3)
    save(img, 'silB_струнные_рощи')


# 23. Терминаторная зона — граница дня и ночи, чистое небо БЕЗ звёзд
def silB_terminatornaya_zona():
    img, d = new_canvas()
    d.rectangle([0, 0, CX - 100, 700], fill=SKY_DAY)
    d.rectangle([CX + 100, 0, SIZE, 700], fill=SKY_NIGHT)
    d.rectangle([0, 700, SIZE, SIZE], fill=GRASS)
    d.pieslice([CX - 180, 500, CX + 180, 900], 0, 180, fill=SKY_NIGHT)
    d.pieslice([CX - 180, 500, CX + 180, 900], 180, 360, fill=SKY_DAY)
    d.line([(CX - 180, 700), (CX + 180, 700)], fill=BORDER, width=5)
    clouds(d, 240, CLOUD, [-260, 60, 300], w=110)
    d.rectangle([0, 780, SIZE, SIZE], fill=(90, 130, 80))
    save(img, 'silB_терминаторная_зона')


if __name__ == '__main__':
    silB_lesa()
    silB_luga_steppi()
    silB_okeany()
    silB_bolota()
    silB_korallovye_rifы()
    silB_svetyashchiesya_chashchi()
    silB_bagrovye_steppi()
    silB_kislotnye_debri()
    silB_sernye_polya()
    silB_khemosinteticheskie_sady()
    silB_khimicheskiy_iney()
    silB_metanovye_morya()
    silB_uglevodorodnye_ravniny()
    silB_ineevye_roshchi()
    silB_lavovye_polya()
    silB_magmovyy_okean()
    silB_vulkanicheskie_polya()
    silB_steklyannye_polya()
    silB_kristalnye_roshchi()
    silB_kremnievye_roshchi()
    silB_metallicheskie_shchetinnye_polya()
    silB_strunnye_roshchi()
    silB_terminatornaya_zona()
    print('ALL STYLE-B SILHOUETTES DONE')