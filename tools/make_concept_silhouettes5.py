# -*- coding: utf-8 -*-
# Силуэты кораблей по КОНЦЕПТАМ — пачка 6 (6 новых слов).
# alien, predator, crater, volcano, star_celestial, shark.
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


# 1. ЧУЖОЙ (alien) — голова пришельца: большая голова, миндалевидные чёрные глаза, узкий подбородок
def alien():
    img, d = new_canvas()
    # голова (крем, большая грушевидная, узкий подбородок вправо)
    d.ellipse([CX - 260, CY - 260, CX + 180, CY + 180], fill=CREAM, outline=BORDER, width=6)
    d.polygon([(CX + 60, CY - 150), (CX + 340, CY - 30), (CX + 340, CY + 30), (CX + 60, CY + 150)], fill=CREAM)
    border(d, [(CX + 60, CY - 150), (CX + 340, CY - 30), (CX + 340, CY + 30), (CX + 60, CY + 150)])
    # большие миндалевидные глаза (тёмные, с зелёным свечением)
    d.polygon([(CX - 160, CY - 120), (CX - 20, CY - 60), (CX - 20, CY + 60), (CX - 160, CY + 120)], fill=DARK)
    border(d, [(CX - 160, CY - 120), (CX - 20, CY - 60), (CX - 20, CY + 60), (CX - 160, CY + 120)], 4)
    d.polygon([(CX + 20, CY - 120), (CX + 160, CY - 60), (CX + 160, CY + 60), (CX + 20, CY + 120)], fill=DARK)
    border(d, [(CX + 20, CY - 120), (CX + 160, CY - 60), (CX + 160, CY + 60), (CX + 20, CY + 120)], 4)
    d.ellipse([CX - 130, CY - 70, CX - 80, CY + 70], fill=POISON, outline=BORDER, width=3)
    d.ellipse([CX + 60, CY - 70, CX + 110, CY + 70], fill=POISON, outline=BORDER, width=3)
    # дюзы (тёмные, на затылке слева)
    d.rectangle([CX - 280, CY - 30, CX - 260, CY + 30], fill=DARK, outline=BORDER, width=4)
    save(img, 'sil_alien')


# 2. ХИЩНИК (predator) — голова хищника с мандибулами (клыки-челюсти вправо)
def predator():
    img, d = new_canvas()
    # череп (сталь, широкий)
    d.ellipse([CX - 280, CY - 220, CX + 200, CY + 220], fill=STEEL, outline=BORDER, width=6)
    # мандибулы (клыки, 4 шт, вправо — нос)
    for i, ay in enumerate([-160, -60, 60, 160]):
        y0 = CY + ay
        d.polygon([(CX + 100, y0 - 20), (CX + 380, y0 - 45), (CX + 380, y0 - 15), (CX + 100, y0 + 20)], fill=IVORY)
        border(d, [(CX + 100, y0 - 20), (CX + 380, y0 - 45), (CX + 380, y0 - 15), (CX + 100, y0 + 20)])
    # глаза (жёлтые/зелёные)
    d.ellipse([CX - 160, CY - 160, CX - 90, CY - 90], fill=GOLD, outline=BORDER, width=4)
    d.ellipse([CX - 160, CY + 90, CX - 90, CY + 160], fill=GOLD, outline=BORDER, width=4)
    # дреды (бронза, на затылке слева)
    for i in range(4):
        d.line([CX - 260, CY - 100 + i * 60, CX - 420, CY - 120 + i * 60], fill=BRONZE, width=14)
    # дюзы (тёмные, на затылке)
    d.rectangle([CX - 290, CY - 20, CX - 270, CY + 20], fill=DARK, outline=BORDER, width=4)
    save(img, 'sil_predator')


# 3. КРАТЕР — круглое тело с кольцевым кратером (как луна)
def crater():
    img, d = new_canvas()
    # тело (серое, круг)
    d.ellipse([CX - 320, CY - 320, CX + 320, CY + 320], fill=STEEL, outline=BORDER, width=8)
    # кратер (кольцо, бронза)
    d.ellipse([CX - 180, CY - 180, CX + 100, CY + 180], outline=BRONZE, width=26)
    # внутренность кратера (тёмная)
    d.ellipse([CX - 140, CY - 140, CX + 60, CY + 140], fill=DARK, outline=BORDER, width=5)
    # светящееся ядро (стекло)
    d.ellipse([CX - 80, CY - 60, CX + 20, CY + 60], fill=GLASS, outline=BORDER, width=4)
    # вторичные кратеры (мелкие)
    d.ellipse([CX - 260, CY - 220, CX - 220, CY - 180], fill=DARK, outline=BORDER, width=3)
    d.ellipse([CX + 180, CY + 160, CX + 220, CY + 200], fill=DARK, outline=BORDER, width=3)
    # дюзы (тёмные, слева)
    d.rectangle([CX - 340, CY - 20, CX - 320, CY + 20], fill=DARK, outline=BORDER, width=4)
    save(img, 'sil_crater')


# 4. ВУЛКАН — конус с кратером и лавой, конус вправо
def volcano():
    img, d = new_canvas()
    # конус (бронза, треугольник вправо)
    d.polygon([(CX - 280, CY - 180), (CX + 300, CY), (CX - 280, CY + 180)], fill=BRONZE)
    border(d, [(CX - 280, CY - 180), (CX + 300, CY), (CX - 280, CY + 180)])
    # склоны (сталь, линии)
    d.line([CX + 300, CY, CX - 280, CY - 180], fill=STEEL, width=6)
    d.line([CX + 300, CY, CX - 280, CY + 180], fill=STEEL, width=6)
    # кратер (тёмный, у вершины)
    d.ellipse([CX + 140, CY - 40, CX + 240, CY + 40], fill=DARK, outline=BORDER, width=5)
    # лава (огонь, течёт влево)
    d.line([CX + 180, CY, CX - 200, CY], fill=FIRE, width=20)
    d.line([CX - 200, CY, CX - 260, CY - 60], fill=FIRE, width=12)
    d.line([CX - 200, CY, CX - 260, CY + 60], fill=FIRE, width=12)
    # дюзы (тёмные, на основании слева)
    d.rectangle([CX - 300, CY - 15, CX - 280, CY + 15], fill=DARK, outline=BORDER, width=3)
    save(img, 'sil_volcano')


# 5. ЗВЕЗДА (небесное тело) — светящийся шар с протуберанцами и короной
def star_celestial():
    img, d = new_canvas()
    # шар (золото, светящийся)
    d.ellipse([CX - 240, CY - 240, CX + 240, CY + 240], fill=GOLD, outline=BORDER, width=6)
    # корона/протуберанцы (огонь, по краям)
    for i in range(8):
        ang = i * 45
        x0 = CX + 240 * math.cos(math.radians(ang))
        y0 = CY + 240 * math.sin(math.radians(ang))
        x1 = CX + 340 * math.cos(math.radians(ang))
        y1 = CY + 340 * math.sin(math.radians(ang))
        d.line([x0, y0, x1, y1], fill=FIRE, width=18)
    # поверхность (крем, пятна)
    d.ellipse([CX - 180, CY - 160, CX - 80, CY - 60], fill=CREAM, outline=BORDER, width=3)
    d.ellipse([CX + 60, CY + 40, CX + 160, CY + 140], fill=CREAM, outline=BORDER, width=3)
    # ядро (стекло, светящееся)
    d.ellipse([CX - 60, CY - 60, CX + 60, CY + 60], fill=GLASS, outline=BORDER, width=4)
    # дюзы (тёмные, слева)
    d.rectangle([CX - 260, CY - 20, CX - 240, CY + 20], fill=DARK, outline=BORDER, width=4)
    save(img, 'sil_star_celestial')


# 6. АКУЛА — тело акулы: плавник, хвост, голова вправо
def shark():
    img, d = new_canvas()
    # тело (сталь, вытянутое, сужение к носу вправо)
    d.polygon([(CX - 360, CY - 60), (CX + 200, CY - 50), (CX + 420, CY - 10), (CX + 420, CY + 10), (CX + 200, CY + 50), (CX - 360, CY + 60)], fill=STEEL)
    border(d, [(CX - 360, CY - 60), (CX + 200, CY - 50), (CX + 420, CY - 10), (CX + 420, CY + 10), (CX + 200, CY + 50), (CX - 360, CY + 60)])
    # спинной плавник (тёмный, в центре)
    d.polygon([(CX - 100, CY - 55), (CX - 40, CY - 180), (CX + 20, CY - 50)], fill=DARK)
    border(d, [(CX - 100, CY - 55), (CX - 40, CY - 180), (CX + 20, CY - 50)])
    # грудные плавники (сталь)
    d.polygon([(CX + 80, CY - 50), (CX + 40, CY - 140), (CX + 100, CY - 60)], fill=STEEL)
    d.polygon([(CX + 80, CY + 50), (CX + 40, CY + 140), (CX + 100, CY + 60)], fill=STEEL)
    # хвост (раздвоенный, влево)
    d.polygon([(CX - 360, CY - 30), (CX - 480, CY - 100), (CX - 450, CY - 90), (CX - 340, CY - 20)], fill=STEEL)
    d.polygon([(CX - 360, CY + 30), (CX - 480, CY + 100), (CX - 450, CY + 90), (CX - 340, CY + 20)], fill=STEEL)
    # глаз (зелёный)
    d.ellipse([CX + 280, CY - 30, CX + 310, CY], fill=POISON, outline=BORDER, width=3)
    # жабры (бронза)
    d.line([CX + 80, CY - 45, CX + 80, CY + 45], fill=BRONZE, width=6)
    d.line([CX + 110, CY - 45, CX + 110, CY + 45], fill=BRONZE, width=6)
    # дюзы (огонь, на хвосте)
    d.rectangle([CX - 490, CY - 20, CX - 480, CY + 20], fill=FIRE, outline=BORDER, width=3)
    save(img, 'sil_shark')


if __name__ == '__main__':
    alien()
    predator()
    crater()
    volcano()
    star_celestial()
    shark()