# -*- coding: utf-8 -*-
# Силуэты кораблей по КОНЦЕПТАМ — пачка 3+4 (20 новых слов).
# Цветные модули на чёрном фоне, 1024x1024, вид сверху, нос вправо.
import os, math
from PIL import Image, ImageDraw

OUT = r'C:\Zorion2\ai_drafts\silhouettes'
SIZE = 1024
CX, CY = SIZE // 2, SIZE // 2

CREAM = (232, 224, 186)
IVORY = (245, 240, 215)
BRONZE = (140, 90, 45)
STEEL = (120, 140, 170)
DARK = (40, 44, 55)
GLASS = (180, 210, 235)
FIRE = (230, 120, 40)
GOLD = (210, 170, 60)
GOLD2 = (240, 215, 120)
PURPLE = (120, 60, 180)
POISON = (110, 200, 160)
BORDER = (12, 12, 12)


def new_canvas():
    img = Image.new('RGB', (SIZE, SIZE), (5, 5, 5))
    return img, ImageDraw.Draw(img)


def save(img, name):
    os.makedirs(OUT, exist_ok=True)
    img.save(os.path.join(OUT, name + '.png'))
    print('Saved:', name)


def border(d, pts, w=5):
    d.line(pts + [pts[0]], fill=BORDER, width=w)


# 1. ЗВЕЗДА — пятиконечная звезда, толстые лучи
def star():
    img, d = new_canvas()
    pts = []
    for i in range(10):
        ang = -90 + i * 36
        r = 380 if i % 2 == 0 else 150
        pts.append((CX + r * math.cos(math.radians(ang)), CY + r * math.sin(math.radians(ang))))
    d.polygon(pts, fill=GOLD)
    border(d, pts)
    # ядро (стекло)
    d.ellipse([CX - 60, CY - 60, CX + 60, CY + 60], fill=GLASS, outline=BORDER, width=5)
    # дюзы на одном луче (корма слева)
    d.rectangle([CX - 400, CY - 25, CX - 360, CY + 25], fill=DARK, outline=BORDER, width=4)
    save(img, 'sil_star')


# 2. ЯКОРЬ — веретено со штоком, лапы, кольцо
def anchor():
    img, d = new_canvas()
    # веретено (сталь, вертикальное вверх)
    d.rectangle([CX - 30, CY - 280, CX + 30, CY + 120], fill=STEEL, outline=BORDER, width=5)
    # шток (поперечная перекладина, бронза)
    d.rectangle([CX - 200, CY - 210, CX + 200, CY - 160], fill=BRONZE, outline=BORDER, width=5)
    # кольцо сверху (золото)
    d.ellipse([CX - 70, CY - 380, CX + 70, CY - 240], fill=GOLD, outline=BORDER, width=16)
    # лапы (крем, дуга вниз)
    d.arc([CX - 260, CY - 50, CX + 260, CY + 400], start=20, end=160, fill=CREAM, width=50)
    # концы лап (стрелки вверх — нос)
    d.polygon([(CX + 150, CY + 190), (CX + 250, CY + 120), (CX + 230, CY + 150)], fill=STEEL)
    d.polygon([(CX - 150, CY + 190), (CX - 250, CY + 120), (CX - 230, CY + 150)], fill=STEEL)
    # дюзы на лапах
    d.rectangle([CX + 230, CY + 110, CX + 260, CY + 130], fill=FIRE, outline=BORDER, width=3)
    d.rectangle([CX - 260, CY + 110, CX - 230, CY + 130], fill=FIRE, outline=BORDER, width=3)
    save(img, 'sil_anchor')


# 3. КРИСТАЛЛ — ромбовидные грани, острый нос
def crystal():
    img, d = new_canvas()
    # тело-кристалл (сталь, вытянутый ромб)
    d.polygon([(CX - 300, CY - 60), (CX + 200, CY - 90), (CX + 430, CY), (CX + 200, CY + 90), (CX - 300, CY + 60)], fill=STEEL)
    border(d, [(CX - 300, CY - 60), (CX + 200, CY - 90), (CX + 430, CY), (CX + 200, CY + 90), (CX - 300, CY + 60)])
    # грани (крем, диагонали)
    d.polygon([(CX - 300, CY - 60), (CX, CY), (CX - 300, CY + 60)], fill=CREAM)
    border(d, [(CX - 300, CY - 60), (CX, CY), (CX - 300, CY + 60)])
    d.polygon([(CX + 200, CY - 90), (CX, CY), (CX + 200, CY + 90)], fill=IVORY)
    border(d, [(CX + 200, CY - 90), (CX, CY), (CX + 200, CY + 90)])
    # сверкающая сердцевина (стекло)
    d.ellipse([CX - 40, CY - 40, CX + 40, CY + 40], fill=GLASS, outline=BORDER, width=4)
    # дюзы (тёмные, на корме)
    d.rectangle([CX - 330, CY - 25, CX - 300, CY + 25], fill=DARK, outline=BORDER, width=4)
    save(img, 'sil_crystal')


# 4. КОРОНА — асимметричная: основание-обод слева (корма), зубцы вправо (нос)
def crown():
    img, d = new_canvas()
    # основание (золото, вертикальная полоса слева)
    d.rectangle([CX - 360, CY - 260, CX - 260, CY + 260], fill=GOLD, outline=BORDER, width=6)
    # зубцы (5 шт, золото, торчат вправо)
    for i, ay in enumerate([-200, -100, 0, 100, 200]):
        y0 = CY + ay
        d.polygon([(CX - 260, y0 - 45), (CX + 60, y0 - 10), (CX + 60, y0 + 10), (CX - 260, y0 + 45)], fill=GOLD)
        border(d, [(CX - 260, y0 - 45), (CX + 60, y0 - 10), (CX + 60, y0 + 10), (CX - 260, y0 + 45)])
    # соединительная дуга между зубцами (золото)
    d.arc([CX - 260, CY - 300, CX + 60, CY + 300], start=90, end=270, fill=GOLD, width=20)
    # драгоценные камни (стекло/пурпур, на зубцах)
    for i, ay in enumerate([-200, -100, 0, 100, 200]):
        d.ellipse([CX - 120, CY + ay - 25, CX - 60, CY + ay + 25], fill=GLASS if i % 2 == 0 else PURPLE, outline=BORDER, width=3)
    # дюзы (тёмные, на основании слева)
    d.rectangle([CX - 380, CY - 30, CX - 360, CY + 30], fill=DARK, outline=BORDER, width=4)
    save(img, 'sil_crown')


# 5. МЕДУЗА — купол + щупальца
def jellyfish():
    img, d = new_canvas()
    # купол (стекло, полукруг вправо)
    d.arc([CX - 300, CY - 300, CX + 300, CY + 300], start=0, end=180, fill=GLASS, width=40)
    # щупальца (бронза/пурпур, волнистые влево)
    for i in range(6):
        y0 = CY - 200 + i * 80
        d.line([CX - 280, y0, CX - 480, y0 + 30], fill=BRONZE if i % 2 == 0 else PURPLE, width=14)
    # внутреннее свечение (пурпур)
    d.ellipse([CX - 120, CY - 120, CX + 120, CY + 120], fill=PURPLE, outline=BORDER, width=4)
    # дюзы (тёмные, на куполе)
    d.rectangle([CX + 250, CY - 30, CX + 300, CY + 30], fill=DARK, outline=BORDER, width=4)
    save(img, 'sil_jellyfish')


# 6. ОСЬМИНОГ — голова + 8 щупалец
def octopus():
    img, d = new_canvas()
    # голова (купол, крем)
    d.ellipse([CX - 260, CY - 260, CX + 260, CY + 60], fill=CREAM, outline=BORDER, width=6)
    # глаза (зелёные)
    d.ellipse([CX - 160, CY - 160, CX - 90, CY - 90], fill=POISON, outline=BORDER, width=4)
    d.ellipse([CX + 90, CY - 160, CX + 160, CY - 90], fill=POISON, outline=BORDER, width=4)
    # щупальца (8, бронза, вниз-влево)
    for i in range(8):
        ang = 170 + i * 12
        r0, r1 = 240, 480
        x0 = CX + r0 * math.cos(math.radians(ang))
        y0 = CY + r0 * math.sin(math.radians(ang))
        x1 = CX + r1 * math.cos(math.radians(ang))
        y1 = CY + r1 * math.sin(math.radians(ang))
        d.line([x0, y0, x1, y1], fill=BRONZE, width=22)
        d.ellipse([x1 - 20, y1 - 20, x1 + 20, y1 + 20], fill=BRONZE, outline=BORDER, width=3)
    # дюзы (огонь, снизу головы)
    d.rectangle([CX - 30, CY + 40, CX + 30, CY + 70], fill=FIRE, outline=BORDER, width=4)
    save(img, 'sil_octopus')


# 7. УЛИТКА — раковина-спираль + тело
def snail():
    img, d = new_canvas()
    # тело (крем, вправо)
    d.polygon([(CX - 260, CY - 50), (CX + 300, CY - 90), (CX + 420, CY - 30), (CX + 420, CY + 30), (CX + 300, CY + 90), (CX - 260, CY + 50)], fill=CREAM)
    # рожки (сталь)
    d.line([CX + 380, CY - 40, CX + 440, CY - 120], fill=STEEL, width=12)
    d.line([CX + 380, CY + 40, CX + 440, CY + 120], fill=STEEL, width=12)
    # раковина-спираль (бронза, круг слева)
    d.ellipse([CX - 420, CY - 180, CX - 60, CY + 180], fill=BRONZE, outline=BORDER, width=8)
    # витки спирали (золото)
    for r in [120, 90, 60, 30]:
        cx0 = CX - 240
        d.ellipse([cx0 - r, CY - r, cx0 + r, CY + r], outline=GOLD, width=10)
    # дюзы (тёмные, на раковине)
    d.rectangle([CX - 430, CY - 20, CX - 400, CY + 20], fill=DARK, outline=BORDER, width=4)
    save(img, 'sil_snail')


# 8. СКАТ — широкие плавники, длинный хвост
def manta():
    img, d = new_canvas()
    # тело (крем, широкий ромб)
    d.polygon([(CX - 250, CY), (CX + 100, CY - 260), (CX + 350, CY - 80), (CX + 350, CY + 80), (CX + 100, CY + 260), (CX - 250, CY)], fill=CREAM)
    border(d, [(CX - 250, CY), (CX + 100, CY - 260), (CX + 350, CY - 80), (CX + 350, CY + 80), (CX + 100, CY + 260), (CX - 250, CY)])
    # плавники-крылья (сталь, стрелой)
    d.polygon([(CX - 200, CY - 40), (CX - 450, CY - 200), (CX - 400, CY - 210), (CX - 150, CY - 50)], fill=STEEL)
    d.polygon([(CX - 200, CY + 40), (CX - 450, CY + 200), (CX - 400, CY + 210), (CX - 150, CY + 50)], fill=STEEL)
    # глаза (зелёные, впереди)
    d.ellipse([CX + 240, CY - 30, CX + 280, CY + 10], fill=POISON, outline=BORDER, width=3)
    # хвост (длинный, бронза)
    d.line([CX - 250, CY, CX - 480, CY], fill=BRONZE, width=10)
    # дюзы на хвосте
    d.rectangle([CX - 500, CY - 12, CX - 480, CY + 12], fill=FIRE, outline=BORDER, width=3)
    save(img, 'sil_manta')


# 9. МОЛНИЯ — зигзаг
def lightning():
    img, d = new_canvas()
    # зигзаг (золото, толстый)
    d.polygon([(CX - 150, CY - 300), (CX + 50, CY - 300), (CX - 100, CY - 50), (CX + 100, CY - 50), (CX - 200, CY + 300), (CX - 320, CY + 300), (CX - 60, CY - 100), (CX - 250, CY - 100)], fill=GOLD)
    border(d, [(CX - 150, CY - 300), (CX + 50, CY - 300), (CX - 100, CY - 50), (CX + 100, CY - 50), (CX - 200, CY + 300), (CX - 320, CY + 300), (CX - 60, CY - 100), (CX - 250, CY - 100)])
    # свечение (стекло, в центре)
    d.ellipse([CX - 80, CY - 80, CX + 20, CY + 10], fill=GLASS, outline=BORDER, width=4)
    # дюзы (тёмные, на конце)
    d.rectangle([CX - 240, CY + 280, CX - 200, CY + 310], fill=DARK, outline=BORDER, width=4)
    save(img, 'sil_lightning')


# 10. СЕРДЦЕ — сердцевидный корпус
def heart():
    img, d = new_canvas()
    # сердце (крем, две доли)
    d.ellipse([CX - 260, CY - 240, CX - 40, CY + 20], fill=CREAM, outline=BORDER, width=6)
    d.ellipse([CX + 40, CY - 240, CX + 260, CY + 20], fill=CREAM, outline=BORDER, width=6)
    d.polygon([(CX - 260, CY - 80), (CX - 260, CY + 150), (CX, CY + 320), (CX + 260, CY + 150), (CX + 260, CY - 80)], fill=CREAM)
    border(d, [(CX - 260, CY - 80), (CX - 260, CY + 150), (CX, CY + 320), (CX + 260, CY + 150), (CX + 260, CY - 80)])
    # светящаяся полоса (стекло, по центру)
    d.line([CX, CY - 240, CX, CY + 320], fill=GLASS, width=16)
    # дюзы (огонь, на конце)
    d.rectangle([CX - 20, CY + 320, CX + 20, CY + 350], fill=FIRE, outline=BORDER, width=4)
    save(img, 'sil_heart')


# 11. ПИРАМИДА — треугольник с гранями
def pyramid():
    img, d = new_canvas()
    # пирамида (золото, треугольник, вершина вправо)
    d.polygon([(CX - 320, CY - 140), (CX + 400, CY), (CX - 320, CY + 140)], fill=GOLD)
    border(d, [(CX - 320, CY - 140), (CX + 400, CY), (CX - 320, CY + 140)])
    # грани (крем, линии от вершины)
    d.line([CX + 400, CY, CX - 320, CY - 140], fill=CREAM, width=6)
    d.line([CX + 400, CY, CX - 320, CY + 140], fill=CREAM, width=6)
    d.line([CX + 400, CY, CX - 320, CY], fill=CREAM, width=6)
    # глаз (стекло, на грани)
    d.ellipse([CX - 100, CY - 60, CX - 40, CY + 60], fill=GLASS, outline=BORDER, width=4)
    # дюзы (тёмные, на основании)
    d.rectangle([CX - 350, CY - 25, CX - 320, CY + 25], fill=DARK, outline=BORDER, width=4)
    save(img, 'sil_pyramid')


# 12. СПИРАЛЬ — закрученная раковина
def spiral():
    img, d = new_canvas()
    # спираль (бронза, 3 витка)
    for r in [60, 120, 180, 240, 300]:
        d.arc([CX - r, CY - r, CX + r, CY + r], start=0, end=270, fill=BRONZE, width=26)
    # центр (золото)
    d.ellipse([CX - 50, CY - 50, CX + 50, CY + 50], fill=GOLD, outline=BORDER, width=5)
    # стекло в центре
    d.ellipse([CX - 20, CY - 20, CX + 20, CY + 20], fill=GLASS, outline=BORDER, width=3)
    # дюзы (тёмные, сбоку)
    d.rectangle([CX + 250, CY - 30, CX + 300, CY + 30], fill=DARK, outline=BORDER, width=4)
    save(img, 'sil_spiral')


# 13. ТРЕЗУБЕЦ — три зубца на древке
def trident():
    img, d = new_canvas()
    # древко (сталь, длинное)
    d.rectangle([CX - 400, CY - 20, CX + 300, CY + 20], fill=STEEL, outline=BORDER, width=5)
    # три зубца (крем, вправо)
    d.polygon([(CX + 200, CY - 70), (CX + 420, CY - 160), (CX + 420, CY - 100), (CX + 230, CY - 30)], fill=CREAM)
    border(d, [(CX + 200, CY - 70), (CX + 420, CY - 160), (CX + 420, CY - 100), (CX + 230, CY - 30)])
    d.polygon([(CX + 250, CY - 20), (CX + 460, CY - 10), (CX + 460, CY + 10), (CX + 250, CY + 20)], fill=CREAM)
    border(d, [(CX + 250, CY - 20), (CX + 460, CY - 10), (CX + 460, CY + 10), (CX + 250, CY + 20)])
    d.polygon([(CX + 200, CY + 70), (CX + 420, CY + 160), (CX + 420, CY + 100), (CX + 230, CY + 30)], fill=CREAM)
    border(d, [(CX + 200, CY + 70), (CX + 420, CY + 160), (CX + 420, CY + 100), (CX + 230, CY + 30)])
    # дюзы (тёмные, на древке слева)
    d.rectangle([CX - 440, CY - 15, CX - 400, CY + 15], fill=DARK, outline=BORDER, width=4)
    save(img, 'sil_trident')


# 14. ЩИТ — овальный щит с гербом
def shield():
    img, d = new_canvas()
    # щит (сталь, овал)
    d.ellipse([CX - 280, CY - 300, CX + 280, CY + 300], fill=STEEL, outline=BORDER, width=8)
    # обод (бронза)
    d.ellipse([CX - 260, CY - 280, CX + 260, CY + 280], outline=BRONZE, width=14)
    # герб (золото, ромб)
    d.polygon([(CX, CY - 200), (CX + 150, CY), (CX, CY + 200), (CX - 150, CY)], fill=GOLD)
    border(d, [(CX, CY - 200), (CX + 150, CY), (CX, CY + 200), (CX - 150, CY)])
    # стекло в центре герба
    d.ellipse([CX - 50, CY - 50, CX + 50, CY + 50], fill=GLASS, outline=BORDER, width=4)
    # дюзы (тёмные, снизу)
    d.rectangle([CX - 40, CY + 280, CX + 40, CY + 320], fill=DARK, outline=BORDER, width=4)
    save(img, 'sil_shield')


# 15. ЯЙЦО — овальное яйцо
def egg():
    img, d = new_canvas()
    # яйцо (крем, овал)
    d.ellipse([CX - 240, CY - 320, CX + 240, CY + 240], fill=CREAM, outline=BORDER, width=8)
    # пятна (бронза)
    for px, py in [(-100, -120), (80, -180), (120, 40), (-140, 60), (40, 140)]:
        d.ellipse([CX + px - 30, CY + py - 30, CX + px + 30, CY + py + 30], fill=BRONZE, outline=BORDER, width=3)
    # окно-стекло (впереди)
    d.ellipse([CX + 100, CY - 60, CX + 200, CY + 40], fill=GLASS, outline=BORDER, width=4)
    # дюзы (тёмные, сзади)
    d.rectangle([CX - 260, CY - 30, CX - 230, CY + 30], fill=DARK, outline=BORDER, width=4)
    save(img, 'sil_egg')


# 16. ФЕНИКС — расправленные крылья, хвост-пламя
def phoenix():
    img, d = new_canvas()
    # тело (золото, овал)
    d.ellipse([CX - 200, CY - 120, CX + 200, CY + 120], fill=GOLD, outline=BORDER, width=6)
    # крылья (расправленные, золото/крем)
    d.polygon([(CX - 100, CY - 80), (CX - 350, CY - 260), (CX - 260, CY - 280), (CX - 40, CY - 100)], fill=GOLD2)
    border(d, [(CX - 100, CY - 80), (CX - 350, CY - 260), (CX - 260, CY - 280), (CX - 40, CY - 100)])
    d.polygon([(CX - 100, CY + 80), (CX - 350, CY + 260), (CX - 260, CY + 280), (CX - 40, CY + 100)], fill=GOLD2)
    border(d, [(CX - 100, CY + 80), (CX - 350, CY + 260), (CX - 260, CY + 280), (CX - 40, CY + 100)])
    # голова (крем, вправо) с клювом
    d.ellipse([CX + 180, CY - 60, CX + 280, CY + 60], fill=CREAM, outline=BORDER, width=5)
    d.polygon([(CX + 270, CY - 20), (CX + 340, CY), (CX + 270, CY + 20)], fill=FIRE)
    # глаз
    d.ellipse([CX + 220, CY - 30, CX + 245, CY - 5], fill=POISON, outline=BORDER, width=3)
    # хвост-пламя (огонь, влево)
    for i in range(3):
        d.polygon([(CX - 200, CY - 30 + i * 30), (CX - 420, CY - 60 + i * 30), (CX - 420, CY + 30 + i * 30), (CX - 200, CY + 30 + i * 30)], fill=FIRE)
    save(img, 'sil_phoenix')


# 17. СОВА — голова с ушами, большие глаза, тело
def owl():
    img, d = new_canvas()
    # тело (крем, овал)
    d.ellipse([CX - 280, CY - 200, CX + 200, CY + 200], fill=CREAM, outline=BORDER, width=6)
    # голова (круг, вправо)
    d.ellipse([CX + 120, CY - 180, CX + 320, CY + 180], fill=CREAM, outline=BORDER, width=6)
    # уши-рожки (сталь)
    d.polygon([(CX + 140, CY - 170), (CX + 150, CY - 280), (CX + 210, CY - 180)], fill=STEEL)
    border(d, [(CX + 140, CY - 170), (CX + 150, CY - 280), (CX + 210, CY - 180)])
    d.polygon([(CX + 230, CY - 180), (CX + 240, CY - 280), (CX + 300, CY - 170)], fill=STEEL)
    border(d, [(CX + 230, CY - 180), (CX + 240, CY - 280), (CX + 300, CY - 170)])
    # большие глаза (стекло + зелёное)
    d.ellipse([CX + 160, CY - 90, CX + 230, CY - 20], fill=GLASS, outline=BORDER, width=5)
    d.ellipse([CX + 250, CY - 90, CX + 320, CY - 20], fill=GLASS, outline=BORDER, width=5)
    d.ellipse([CX + 185, CY - 65, CX + 215, CY - 35], fill=POISON, outline=BORDER, width=3)
    d.ellipse([CX + 275, CY - 65, CX + 305, CY - 35], fill=POISON, outline=BORDER, width=3)
    # клюв (бронза)
    d.polygon([(CX + 220, CY + 10), (CX + 260, CY + 40), (CX + 220, CY + 70)], fill=BRONZE)
    # дюзы (тёмные, сзади)
    d.rectangle([CX - 310, CY - 25, CX - 280, CY + 25], fill=DARK, outline=BORDER, width=4)
    save(img, 'sil_owl')


# 18. ШЛЕМ — купол с гребнем, забрало
def helmet():
    img, d = new_canvas()
    # купол (сталь)
    d.arc([CX - 300, CY - 300, CX + 300, CY + 300], start=180, end=360, fill=STEEL, width=40)
    # забрало (стекло, впереди вправо)
    d.arc([CX - 150, CY - 150, CX + 400, CY + 150], start=0, end=180, fill=GLASS, width=30)
    # гребень (бронза, сверху)
    d.polygon([(CX - 100, CY - 290), (CX + 100, CY - 290), (CX + 60, CY - 370), (CX - 60, CY - 370)], fill=BRONZE)
    border(d, [(CX - 100, CY - 290), (CX + 100, CY - 290), (CX + 60, CY - 370), (CX - 60, CY - 370)])
    # дюзы (тёмные, снизу)
    d.rectangle([CX - 40, CY + 260, CX + 40, CY + 290], fill=DARK, outline=BORDER, width=4)
    save(img, 'sil_helmet')


# 19. РАКУШКА — веерная ракушка с рёбрами
def shell():
    img, d = new_canvas()
    # ракушка (крем, веер вправо)
    d.polygon([(CX - 350, CY - 160), (CX + 250, CY - 120), (CX + 380, CY), (CX + 250, CY + 120), (CX - 350, CY + 160)], fill=CREAM)
    border(d, [(CX - 350, CY - 160), (CX + 250, CY - 120), (CX + 380, CY), (CX + 250, CY + 120), (CX - 350, CY + 160)])
    # рёбра (бронза, линии от основания)
    for i in range(5):
        ang = -30 + i * 15
        x1 = CX + 380 * math.cos(math.radians(ang))
        y1 = CY + 380 * math.sin(math.radians(ang))
        d.line([CX - 350, CY, x1, y1], fill=BRONZE, width=8)
    # основание (золото, слева)
    d.ellipse([CX - 400, CY - 60, CX - 300, CY + 60], fill=GOLD, outline=BORDER, width=5)
    # дюзы (тёмные, на основании)
    d.rectangle([CX - 420, CY - 15, CX - 400, CY + 15], fill=DARK, outline=BORDER, width=3)
    save(img, 'sil_shell')


# 20. КНИГА — раскрытая книга со страницами
def book():
    img, d = new_canvas()
    # обложки (бронза, две половины)
    d.polygon([(CX, CY), (CX - 300, CY - 220), (CX - 300, CY + 220)], fill=BRONZE)
    border(d, [(CX, CY), (CX - 300, CY - 220), (CX - 300, CY + 220)])
    d.polygon([(CX, CY), (CX + 300, CY - 220), (CX + 300, CY + 220)], fill=BRONZE)
    border(d, [(CX, CY), (CX + 300, CY - 220), (CX + 300, CY + 220)])
    # страницы (крем, внутри)
    d.polygon([(CX, CY), (CX - 280, CY - 200), (CX - 280, CY - 150)], fill=CREAM)
    d.polygon([(CX, CY), (CX + 280, CY - 200), (CX + 280, CY - 150)], fill=CREAM)
    d.polygon([(CX, CY), (CX - 280, CY + 150), (CX - 280, CY + 200)], fill=CREAM)
    d.polygon([(CX, CY), (CX + 280, CY + 150), (CX + 280, CY + 200)], fill=CREAM)
    # корешок (золото)
    d.line([CX, CY - 220, CX, CY + 220], fill=GOLD, width=14)
    # дюзы (тёмные, на корешке)
    d.rectangle([CX - 12, CY + 220, CX + 12, CY + 250], fill=DARK, outline=BORDER, width=3)
    save(img, 'sil_book')


if __name__ == '__main__':
    star()
    anchor()
    crystal()
    crown()
    jellyfish()
    octopus()
    snail()
    manta()
    lightning()
    heart()
    pyramid()
    spiral()
    trident()
    shield()
    egg()
    phoenix()
    owl()
    helmet()
    shell()
    book()