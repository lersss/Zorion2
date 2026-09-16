# -*- coding: utf-8 -*-
# Силуэты кораблей по КОНЦЕПТАМ — пачка 5 (10 новых слов).
# Цветные модули на чёрном фоне, 1024x1024, вид сверху, нос вправо (асимметрия: дюзы слева).
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


# 1. ВЕЕР — раскрытый веер: рёбра веером вправо, ручка-корма слева
def fan():
    img, d = new_canvas()
    # рёбра (сталь/крем, веером вправо)
    for i in range(7):
        ang = -45 + i * 15
        x1 = CX - 100 + 420 * math.cos(math.radians(ang))
        y1 = CY + 420 * math.sin(math.radians(ang))
        d.line([CX - 100, CY, x1, y1], fill=STEEL if i % 2 == 0 else CREAM, width=18)
    # полотно между рёбрами (крем)
    pts = [(CX - 100, CY)]
    for i in range(7):
        ang = -45 + i * 15
        pts.append((CX - 100 + 400 * math.cos(math.radians(ang)), CY + 400 * math.sin(math.radians(ang))))
    d.polygon(pts, fill=CREAM)
    border(d, pts)
    # ручка (бронза, слева)
    d.rectangle([CX - 240, CY - 25, CX - 100, CY + 25], fill=BRONZE, outline=BORDER, width=5)
    # дюзы (тёмные, на ручке)
    d.rectangle([CX - 260, CY - 15, CX - 240, CY + 15], fill=DARK, outline=BORDER, width=3)
    save(img, 'sil_fan')


# 2. ПОЛУМЕСЯЦ — серп: рога вправо, спинка слева
def crescent():
    img, d = new_canvas()
    # полумесяц (золото, серп с рогами вправо)
    d.ellipse([CX - 340, CY - 340, CX + 340, CY + 340], fill=GOLD, outline=BORDER, width=6)
    # вырез (тёмный, слева — делает серп)
    d.ellipse([CX - 420, CY - 280, CX + 40, CY + 280], fill=(5, 5, 5))
    # рога (заострённые концы вправо)
    d.polygon([(CX + 200, CY - 180), (CX + 380, CY - 120), (CX + 320, CY - 100)], fill=GOLD)
    d.polygon([(CX + 200, CY + 180), (CX + 380, CY + 120), (CX + 320, CY + 100)], fill=GOLD)
    # стекло на спинке
    d.ellipse([CX - 240, CY - 40, CX - 140, CY + 40], fill=GLASS, outline=BORDER, width=4)
    # дюзы (тёмные, на спинке слева)
    d.rectangle([CX - 360, CY - 20, CX - 330, CY + 20], fill=DARK, outline=BORDER, width=4)
    save(img, 'sil_crescent')


# 3. КОМПАС — круг с стрелкой: стрелка вправо (нос), корпус круглый
def compass():
    img, d = new_canvas()
    # корпус (круг, бронза)
    d.ellipse([CX - 300, CY - 300, CX + 300, CY + 300], fill=BRONZE, outline=BORDER, width=8)
    # циферблат (крем)
    d.ellipse([CX - 250, CY - 250, CX + 250, CY + 250], fill=CREAM, outline=BORDER, width=5)
    # деления (золото, по кругу)
    for i in range(16):
        ang = math.radians(i * 22.5)
        x1 = CX + 230 * math.cos(ang); y1 = CY + 230 * math.sin(ang)
        x2 = CX + 210 * math.cos(ang); y2 = CY + 210 * math.sin(ang)
        d.line([x1, y1, x2, y2], fill=GOLD, width=8)
    # стрелка (сталь, нос вправо)
    d.polygon([(CX - 30, CY - 15), (CX + 230, CY - 6), (CX + 230, CY + 6), (CX - 30, CY + 15)], fill=STEEL)
    d.polygon([(CX - 30, CY - 15), (CX - 160, CY - 6), (CX - 160, CY + 6), (CX - 30, CY + 15)], fill=BRONZE)
    # центр (тёмный)
    d.ellipse([CX - 25, CY - 25, CX + 25, CY + 25], fill=DARK, outline=BORDER, width=4)
    # дюзы (тёмные, на задней части стрелки)
    d.rectangle([CX - 180, CY - 12, CX - 160, CY + 12], fill=DARK, outline=BORDER, width=3)
    save(img, 'sil_compass')


# 4. ГРИБ — шляпка с ножкой: шляпка впереди (вправо), ножка-корма (влево)
def mushroom():
    img, d = new_canvas()
    # ножка (крем, слева)
    d.rectangle([CX - 340, CY - 60, CX - 60, CY + 60], fill=CREAM, outline=BORDER, width=5)
    # шляпка (полукруг вправо, бронза/красный)
    d.arc([CX - 160, CY - 260, CX + 300, CY + 260], start=90, end=270, fill=BRONZE, width=60)
    # пятна на шляпке (крем)
    d.ellipse([CX + 40, CY - 160, CX + 110, CY - 90], fill=CREAM, outline=BORDER, width=3)
    d.ellipse([CX + 140, CY - 120, CX + 200, CY - 60], fill=CREAM, outline=BORDER, width=3)
    d.ellipse([CX + 60, CY + 90, CX + 120, CY + 150], fill=CREAM, outline=BORDER, width=3)
    # стекло (глаз на шляпке)
    d.ellipse([CX + 60, CY - 40, CX + 120, CY + 40], fill=GLASS, outline=BORDER, width=4)
    # дюзы (тёмные, на ножке слева)
    d.rectangle([CX - 360, CY - 20, CX - 340, CY + 20], fill=DARK, outline=BORDER, width=3)
    save(img, 'sil_mushroom')


# 5. ПАРУС — треугольный парус с мачтой: парус вправо, корма-мачта слева
def sail():
    img, d = new_canvas()
    # парус (крем, треугольник вправо)
    d.polygon([(CX - 150, CY - 220), (CX + 350, CY), (CX - 150, CY + 220)], fill=CREAM)
    border(d, [(CX - 150, CY - 220), (CX + 350, CY), (CX - 150, CY + 220)])
    # полосы паруса (бронза)
    for i in range(3):
        x = CX - 150 + (350 + 150) * (i + 1) / 4
        d.line([x, -40 + CY - 160 + i * 100, x, 40 + CY + 160 - i * 100], fill=BRONZE, width=6)
    # мачта (сталь, слева)
    d.rectangle([CX - 200, CY - 280, CX - 150, CY + 280], fill=STEEL, outline=BORDER, width=5)
    # корпус-корма (бронза, у мачты)
    d.rectangle([CX - 280, CY - 60, CX - 150, CY + 60], fill=BRONZE, outline=BORDER, width=5)
    # дюзы (тёмные, на корпусе слева)
    d.rectangle([CX - 300, CY - 20, CX - 280, CY + 20], fill=DARK, outline=BORDER, width=3)
    save(img, 'sil_sail')


# 6. КАПЛЯ — капля: острый нос вправо, округлая корма слева
def drop():
    img, d = new_canvas()
    # капля (крем, круглая слева, острый нос вправо)
    d.ellipse([CX - 320, CY - 240, CX + 120, CY + 240], fill=CREAM, outline=BORDER, width=6)
    d.polygon([(CX + 40, CY - 200), (CX + 440, CY), (CX + 40, CY + 200)], fill=CREAM)
    border(d, [(CX + 40, CY - 200), (CX + 440, CY), (CX + 40, CY + 200)])
    # блик (стекло, на корме)
    d.ellipse([CX - 260, CY - 90, CX - 160, CY + 10], fill=GLASS, outline=BORDER, width=4)
    # кольцо (золото, у носа)
    d.line([CX + 200, CY - 60, CX + 200, CY + 60], fill=GOLD, width=10)
    # дюзы (тёмные, на корме слева)
    d.rectangle([CX - 340, CY - 25, CX - 320, CY + 25], fill=DARK, outline=BORDER, width=4)
    save(img, 'sil_drop')


# 7. БУМЕРАНГ — изогнутый: два крыла вперёд (вправо), изгиб-корма (влево)
def boomerang():
    img, d = new_canvas()
    # бумеранг (бронза, дуга с концами вправо)
    d.arc([CX - 380, CY - 380, CX + 380, CY + 380], start=250, end=110, fill=BRONZE, width=50)
    # концы (сталь, вправо)
    d.polygon([(CX + 160, CY - 240), (CX + 320, CY - 120), (CX + 260, CY - 100)], fill=STEEL)
    d.polygon([(CX + 160, CY + 240), (CX + 320, CY + 120), (CX + 260, CY + 100)], fill=STEEL)
    # стекло в изгибе
    d.ellipse([CX - 80, CY - 50, CX + 20, CY + 50], fill=GLASS, outline=BORDER, width=4)
    # дюзы (тёмные, в изгибе слева)
    d.rectangle([CX - 120, CY - 15, CX - 100, CY + 15], fill=DARK, outline=BORDER, width=3)
    save(img, 'sil_boomerang')


# 8. КОЛЬЦО — круглое кольцо с камнем: камень вправо (нос)
def ring():
    img, d = new_canvas()
    # кольцо (золото, круг)
    d.ellipse([CX - 320, CY - 320, CX + 320, CY + 320], outline=GOLD, width=50)
    # оправа (бронза, впереди вправо)
    d.rectangle([CX + 140, CY - 90, CX + 260, CY + 90], fill=BRONZE, outline=BORDER, width=6)
    # камень (стекло/пурпур, впереди)
    d.polygon([(CX + 220, CY - 80), (CX + 340, CY), (CX + 220, CY + 80)], fill=GLASS)
    border(d, [(CX + 220, CY - 80), (CX + 340, CY), (CX + 220, CY + 80)])
    d.ellipse([CX + 240, CY - 30, CX + 300, CY + 30], fill=PURPLE, outline=BORDER, width=3)
    # дюзы (тёмные, сзади на кольце)
    d.rectangle([CX - 320, CY - 20, CX - 280, CY + 20], fill=DARK, outline=BORDER, width=4)
    save(img, 'sil_ring')


# 9. ЛУК — дуга с тетивой: дуга вправо (нос), тетива-корма (влево)
def bow():
    img, d = new_canvas()
    # дуга (сталь, изогнутая, концы вверх/вниз)
    d.arc([CX - 300, CY - 320, CX + 340, CY + 320], start=10, end=170, fill=STEEL, width=40)
    # концы дуги (золото)
    d.ellipse([CX + 250, CY - 260, CX + 310, CY - 200], fill=GOLD, outline=BORDER, width=4)
    d.ellipse([CX + 250, CY + 200, CX + 310, CY + 260], fill=GOLD, outline=BORDER, width=4)
    # тетива (крем, вертикальная слева)
    d.line([CX - 200, CY - 300, CX - 200, CY + 300], fill=CREAM, width=10)
    # стрела-корпус (бронза, горизонтальная через центр)
    d.rectangle([CX - 280, CY - 25, CX + 250, CY + 25], fill=BRONZE, outline=BORDER, width=5)
    # наконечник (сталь, вправо)
    d.polygon([(CX + 250, CY - 20), (CX + 380, CY), (CX + 250, CY + 20)], fill=STEEL)
    # стекло (на тетиве)
    d.ellipse([CX - 230, CY - 40, CX - 170, CY + 40], fill=GLASS, outline=BORDER, width=4)
    # дюзы (тёмные, на тетиве слева)
    d.rectangle([CX - 260, CY - 15, CX - 240, CY + 15], fill=DARK, outline=BORDER, width=3)
    save(img, 'sil_bow')


# 10. ЗЕРКАЛО — круглое зеркало с ручкой: ручка-корма слева
def mirror():
    img, d = new_canvas()
    # зеркало (круг, сталь)
    d.ellipse([CX - 300, CY - 300, CX + 300, CY + 300], fill=STEEL, outline=BORDER, width=8)
    # стекло (внутри)
    d.ellipse([CX - 260, CY - 260, CX + 260, CY + 260], fill=GLASS, outline=BORDER, width=5)
    # блик (крем)
    d.ellipse([CX - 180, CY - 180, CX - 60, CY - 60], fill=CREAM, outline=BORDER, width=3)
    # оправа-камень (золото, впереди)
    d.ellipse([CX + 180, CY - 50, CX + 280, CY + 50], fill=GOLD, outline=BORDER, width=5)
    # ручка (бронза, слева)
    d.rectangle([CX - 440, CY - 30, CX - 300, CY + 30], fill=BRONZE, outline=BORDER, width=5)
    # дюзы (тёмные, на ручке)
    d.rectangle([CX - 460, CY - 18, CX - 440, CY + 18], fill=DARK, outline=BORDER, width=3)
    save(img, 'sil_mirror')


if __name__ == '__main__':
    fan()
    crescent()
    compass()
    mushroom()
    sail()
    drop()
    boomerang()
    ring()
    bow()
    mirror()