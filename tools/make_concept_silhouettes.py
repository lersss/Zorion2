# -*- coding: utf-8 -*-
# Силуэты кораблей по КОНЦЕПТАМ (корабль в образе слова).
# Цветные модули на чёрном фоне, 1024x1024, вид сверху, нос вправо.
import os
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
FLESH = (205, 175, 140)
CHITIN = (150, 110, 80)
SPINE = (230, 200, 170)
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


# 1. ДОМ — квадратный «домик»: крыша-треугольник, светящиеся окна
def home():
    img, d = new_canvas()
    # корпус-дом (крем) — прямоугольник
    d.rectangle([CX - 300, CY - 150, CX + 250, CY + 150], fill=CREAM, outline=BORDER, width=6)
    # крыша (бронза, треугольник, нос — конёк вправо)
    d.polygon([(CX + 150, CY - 150), (CX + 350, CY - 20), (CX + 350, CY + 20), (CX + 150, CY + 150)], fill=BRONZE)
    border(d, [(CX + 150, CY - 150), (CX + 350, CY - 20), (CX + 350, CY + 20), (CX + 150, CY + 150)])
    # окна (стекло, светящиеся)
    for wx in [-180, -60, 60]:
        d.rectangle([CX + wx, CY - 90, CX + wx + 60, CY - 20], fill=GLASS, outline=BORDER, width=4)
        d.rectangle([CX + wx, CY + 20, CX + wx + 60, CY + 90], fill=GLASS, outline=BORDER, width=4)
    # дверь-дюзы (тёмная с огнём)
    d.rectangle([CX - 320, CY - 50, CX - 270, CY + 50], fill=DARK, outline=BORDER, width=4)
    d.rectangle([CX - 332, CY - 40, CX - 320, CY + 40], fill=FIRE, outline=BORDER, width=2)
    # труба-антенна
    d.rectangle([CX + 80, CY - 190, CX + 100, CY - 150], fill=STEEL, outline=BORDER, width=3)
    save(img, 'sil_home')


# 2. ЦВЕТОК — симметричные лепестки вокруг центра
def flower():
    img, d = new_canvas()
    # лепестки (8 шт, сталь/крем чередуются)
    petals = [
        (-200, -250), (200, -250), (0, -320), (0, 320), (-200, 250), (200, 250), (-320, 0), (320, 0)
    ]
    for i, (px, py) in enumerate(petals):
        fill = STEEL if i % 2 == 0 else IVORY
        d.ellipse([CX + px - 110, CY + py - 110, CX + px + 110, CY + py + 110], fill=fill, outline=BORDER, width=5)
    # центральный диск (золотой)
    d.ellipse([CX - 130, CY - 130, CX + 130, CY + 130], fill=GOLD, outline=BORDER, width=6)
    # сердцевина (стекло, светящаяся)
    d.ellipse([CX - 60, CY - 60, CX + 60, CY + 60], fill=GLASS, outline=BORDER, width=4)
    # задний лепесток-хвост (нейтральный, сталь, без пламени)
    d.polygon([(CX - 280, CY - 60), (CX - 360, CY - 80), (CX - 350, CY - 70), (CX - 270, CY - 40)], fill=STEEL)
    d.polygon([(CX - 280, CY + 60), (CX - 360, CY + 80), (CX - 350, CY + 70), (CX - 270, CY + 40)], fill=STEEL)
    save(img, 'sil_flower')


# 3. ДРАКОН — изогнутое тело, крылья, голова с рогами, хвост
def dragon():
    img, d = new_canvas()
    # тело-змей (крем, изогнутое: несколько сегментов)
    d.polygon([(CX - 380, CY - 30), (CX - 200, CY - 90), (CX + 50, CY - 40), (CX + 250, CY - 15), (CX + 250, CY + 25), (CX + 50, CY + 70), (CX - 200, CY + 30), (CX - 380, CY + 20)], fill=CREAM)
    # сегменты (спинные пластины, бронза)
    for x in range(-300, 200, 80):
        d.polygon([(CX + x, CY - 60), (CX + x + 20, CY - 130), (CX + x + 40, CY - 55)], fill=BRONZE)
    # голова (справа, сталь) с рогами
    d.polygon([(CX + 230, CY - 30), (CX + 360, CY - 60), (CX + 360, CY + 10), (CX + 230, CY + 30)], fill=STEEL)
    border(d, [(CX + 230, CY - 30), (CX + 360, CY - 60), (CX + 360, CY + 10), (CX + 230, CY + 30)])
    d.polygon([(CX + 330, CY - 55), (CX + 380, CY - 110), (CX + 370, CY - 55)], fill=CHITIN)
    d.polygon([(CX + 350, CY - 50), (CX + 400, CY - 95), (CX + 390, CY - 45)], fill=CHITIN)
    # глаз (зелёный)
    d.ellipse([CX + 300, CY - 35, CX + 325, CY - 10], fill=POISON, outline=BORDER, width=3)
    # крылья (вверх/вниз, перепончатые)
    d.polygon([(CX - 100, CY - 60), (CX - 250, CY - 220), (CX - 180, CY - 230), (CX - 40, CY - 80)], fill=STEEL)
    d.polygon([(CX - 100, CY + 60), (CX - 250, CY + 220), (CX - 180, CY + 230), (CX - 40, CY + 80)], fill=STEEL)
    # хвост (влево, с шипом)
    d.polygon([(CX - 380, CY - 10), (CX - 460, CY - 40), (CX - 450, CY - 30), (CX - 380, CY + 5)], fill=CHITIN)
    d.polygon([(CX - 380, CY + 15), (CX - 460, CY + 45), (CX - 450, CY + 35), (CX - 380, CY + 25)], fill=CHITIN)
    # дюзы (огонь из пасти)
    d.rectangle([CX + 360, CY - 8, CX + 400, CY + 8], fill=FIRE, outline=BORDER, width=2)
    save(img, 'sil_dragon')


# 4. ПРИЗРАК — плавная капля с волнистым шлейфом
def ghost():
    img, d = new_canvas()
    # тело-капля (крем, плавное)
    d.polygon([(CX - 260, CY - 60), (CX - 100, CY - 160), (CX + 100, CY - 160), (CX + 280, CY - 40), (CX + 280, CY + 40), (CX + 100, CY + 60)], fill=IVORY)
    # волнистый шлейф (хвост, бронза)
    for i, wy in enumerate([-80, -40, 0, 40, 80]):
        w = 220 - i * 30
        d.polygon([(CX - 100, CY + wy - 25), (CX - 100 - w, CY + wy - 45), (CX - 100 - w, CY + wy + 45), (CX - 100, CY + wy + 25)], fill=BRONZE)
    # глаза-поры (зелёные)
    d.ellipse([CX + 60, CY - 60, CX + 95, CY - 25], fill=POISON, outline=BORDER, width=4)
    d.ellipse([CX + 120, CY - 60, CX + 155, CY - 25], fill=POISON, outline=BORDER, width=4)
    # рот-дыюза (тёмный)
    d.rectangle([CX + 240, CY - 25, CX + 280, CY + 25], fill=DARK, outline=BORDER, width=3)
    save(img, 'sil_ghost')


# 5. СИГАРЕТА — длинный тонкий цилиндр, горящий кончик
def cigarette():
    img, d = new_canvas()
    # корпус (крем, длинный тонкий)
    d.rectangle([CX - 420, CY - 40, CX + 380, CY + 40], fill=CREAM, outline=BORDER, width=6)
    # фильтр (сталь, слева)
    d.rectangle([CX - 460, CY - 40, CX - 420, CY + 40], fill=STEEL, outline=BORDER, width=6)
    # горящий кончик (огонь, справа) + пепел
    d.rectangle([CX + 380, CY - 40, CX + 430, CY + 40], fill=FIRE, outline=BORDER, width=6)
    d.rectangle([CX + 430, CY - 30, CX + 470, CY + 30], fill=DARK, outline=BORDER, width=4)
    # кабина-ободок (стекло)
    d.rectangle([CX + 100, CY - 40, CX + 130, CY + 40], fill=GLASS, outline=BORDER, width=4)
    # кольца (бронза)
    d.line([CX - 100, CY - 40, CX - 100, CY + 40], fill=BRONZE, width=8)
    d.line([CX + 200, CY - 40, CX + 200, CY + 40], fill=BRONZE, width=8)
    save(img, 'sil_cigarette')


# 6. ЛАМПОЧКА — круглая колба, сужение к цоколю, светящаяся нить
def lightbulb():
    img, d = new_canvas()
    # колба (стекло, круг)
    d.ellipse([CX - 220, CY - 220, CX + 220, CY + 220], fill=GLASS, outline=BORDER, width=6)
    # нить накаливания (огонь, спираль)
    d.line([CX - 100, CY + 100, CX + 100, CY - 100], fill=FIRE, width=14)
    d.line([CX - 100, CY - 100, CX + 100, CY + 100], fill=FIRE, width=14)
    # цоколь (сталь, слева-внизу)
    d.polygon([(CX - 200, CY + 130), (CX + 200, CY + 130), (CX + 160, CY + 260), (CX - 160, CY + 260)], fill=STEEL)
    border(d, [(CX - 200, CY + 130), (CX + 200, CY + 130), (CX + 160, CY + 260), (CX - 160, CY + 260)])
    # резьба цоколя (бронза)
    for y in range(150, 250, 20):
        d.line([CX - 190 + (y - 150) // 2, CY + y, CX + 190 - (y - 150) // 2, CY + y], fill=BRONZE, width=6)
    save(img, 'sil_lightbulb')


# 7. КОТ — голова с ушами, тело-овоид, хвост
def cat():
    img, d = new_canvas()
    # голова (круг, крем) — справа
    d.ellipse([CX + 180, CY - 110, CX + 360, CY + 110], fill=CREAM, outline=BORDER, width=6)
    # уши (треугольники, сталь)
    d.polygon([(CX + 200, CY - 100), (CX + 210, CY - 200), (CX + 260, CY - 110)], fill=STEEL)
    border(d, [(CX + 200, CY - 100), (CX + 210, CY - 200), (CX + 260, CY - 110)])
    d.polygon([(CX + 280, CY - 110), (CX + 290, CY - 200), (CX + 340, CY - 100)], fill=STEEL)
    border(d, [(CX + 280, CY - 110), (CX + 290, CY - 200), (CX + 340, CY - 100)])
    # глаза (зелёные)
    d.ellipse([CX + 230, CY - 60, CX + 265, CY - 15], fill=POISON, outline=BORDER, width=4)
    d.ellipse([CX + 290, CY - 60, CX + 325, CY - 15], fill=POISON, outline=BORDER, width=4)
    # нос-дюза (тёмный)
    d.polygon([(CX + 340, CY - 15), (CX + 380, CY + 10), (CX + 340, CY + 35)], fill=DARK)
    # тело (овоид, крем) — слева
    d.ellipse([CX - 360, CY - 100, CX + 180, CY + 100], fill=CREAM, outline=BORDER, width=6)
    # полосы (бронза)
    for x in range(-280, 120, 60):
        d.line([CX + x, CY - 90, CX + x + 15, CY + 90], fill=BRONZE, width=8)
    # хвост (изогнутый, сталь)
    d.polygon([(CX - 360, CY - 20), (CX - 460, CY - 100), (CX - 440, CY - 90), (CX - 350, CY - 5)], fill=STEEL)
    d.polygon([(CX - 360, CY + 20), (CX - 460, CY + 100), (CX - 440, CY + 90), (CX - 350, CY + 5)], fill=STEEL)
    save(img, 'sil_cat')


# 8. ЧЕРЕП — круглая голова, глазницы, зубы
def skull():
    img, d = new_canvas()
    # череп (круг, крем)
    d.ellipse([CX - 240, CY - 240, CX + 240, CY + 240], fill=IVORY, outline=BORDER, width=6)
    # глазницы (тёмные)
    d.ellipse([CX - 130, CY - 120, CX - 20, CY + 10], fill=DARK, outline=BORDER, width=5)
    d.ellipse([CX + 20, CY - 120, CX + 130, CY + 10], fill=DARK, outline=BORDER, width=5)
    # глаза-свечение (зелёные)
    d.ellipse([CX - 100, CY - 95, CX - 50, CY - 45], fill=POISON, outline=BORDER, width=3)
    d.ellipse([CX + 50, CY - 95, CX + 100, CY - 45], fill=POISON, outline=BORDER, width=3)
    # носовая впадина
    d.polygon([(CX - 20, CY + 20), (CX + 20, CY + 20), (CX, CY + 60)], fill=DARK)
    # челюсть с зубами (сталь)
    d.rectangle([CX - 150, CY + 80, CX + 150, CY + 150], fill=STEEL, outline=BORDER, width=5)
    for tx in range(-120, 130, 30):
        d.rectangle([CX + tx, CY + 80, CX + tx + 20, CY + 150], fill=IVORY, outline=BORDER, width=3)
    # дюзы (огонь из глазниц? нет — из челюсти)
    d.rectangle([CX - 40, CY + 150, CX + 40, CY + 180], fill=FIRE, outline=BORDER, width=3)
    save(img, 'sil_skull')


# 9. СТРЕКОЗА — длинное тонкое тело, 4 крыла
def dragonfly():
    img, d = new_canvas()
    # тело (крем, длинное, сегментированное)
    d.rectangle([CX - 380, CY - 25, CX + 380, CY + 25], fill=CREAM, outline=BORDER, width=5)
    for x in range(-320, 340, 60):
        d.line([CX + x, CY - 25, CX + x, CY + 25], fill=BRONZE, width=6)
    # голова (круг, справа) с глазами
    d.ellipse([CX + 370, CY - 40, CX + 440, CY + 40], fill=STEEL, outline=BORDER, width=5)
    d.ellipse([CX + 390, CY - 30, CX + 415, CY - 5], fill=POISON, outline=BORDER, width=3)
    d.ellipse([CX + 390, CY + 5, CX + 415, CY + 30], fill=POISON, outline=BORDER, width=3)
    # 4 крыла (стекло, прозрачные, с прожилками)
    d.polygon([(CX - 200, CY - 25), (CX - 320, CY - 260), (CX - 230, CY - 280), (CX - 120, CY - 30)], fill=GLASS)
    border(d, [(CX - 200, CY - 25), (CX - 320, CY - 260), (CX - 230, CY - 280), (CX - 120, CY - 30)], 4)
    d.polygon([(CX - 60, CY - 25), (CX - 160, CY - 230), (CX - 70, CY - 250), (CX + 20, CY - 30)], fill=GLASS)
    border(d, [(CX - 60, CY - 25), (CX - 160, CY - 230), (CX - 70, CY - 250), (CX + 20, CY - 30)], 4)
    d.polygon([(CX - 200, CY + 25), (CX - 320, CY + 260), (CX - 230, CY + 280), (CX - 120, CY + 30)], fill=GLASS)
    border(d, [(CX - 200, CY + 25), (CX - 320, CY + 260), (CX - 230, CY + 280), (CX - 120, CY + 30)], 4)
    d.polygon([(CX - 60, CY + 25), (CX - 160, CY + 230), (CX - 70, CY + 250), (CX + 20, CY + 30)], fill=GLASS)
    border(d, [(CX - 60, CY + 25), (CX - 160, CY + 230), (CX - 70, CY + 250), (CX + 20, CY + 30)], 4)
    # хвост (тонкий, сталь)
    d.polygon([(CX - 380, CY - 15), (CX - 470, CY - 30), (CX - 460, CY - 20), (CX - 380, CY + 10)], fill=STEEL)
    d.polygon([(CX - 380, CY + 15), (CX - 470, CY + 30), (CX - 460, CY + 20), (CX - 380, CY - 10)], fill=STEEL)
    save(img, 'sil_dragonfly')


# 10. ЧЕРЕПАХА — овальный панцирь с сегментами, голова, ласты
def turtle():
    img, d = new_canvas()
    # панцирь (овал, бронза)
    d.ellipse([CX - 300, CY - 200, CX + 250, CY + 200], fill=BRONZE, outline=BORDER, width=6)
    # сегменты панциря (светлые)
    d.polygon([(CX - 100, CY - 160), (CX + 40, CY), (CX - 100, CY + 160)], fill=GOLD2)
    d.polygon([(CX - 250, CY - 140), (CX - 140, CY), (CX - 250, CY + 140)], fill=GOLD2)
    d.polygon([(CX + 40, CY - 140), (CX + 170, CY), (CX + 40, CY + 140)], fill=GOLD2)
    # голова (крем, справа)
    d.polygon([(CX + 250, CY - 50), (CX + 400, CY - 20), (CX + 400, CY + 20), (CX + 250, CY + 50)], fill=CREAM)
    border(d, [(CX + 250, CY - 50), (CX + 400, CY - 20), (CX + 400, CY + 20), (CX + 250, CY + 50)])
    # глаза
    d.ellipse([CX + 330, CY - 25, CX + 350, CY - 5], fill=DARK, outline=BORDER, width=3)
    d.ellipse([CX + 330, CY + 5, CX + 350, CY + 25], fill=DARK, outline=BORDER, width=3)
    # ласты-крылья (сталь)
    d.polygon([(CX - 200, CY - 180), (CX - 300, CY - 260), (CX - 260, CY - 270), (CX - 150, CY - 190)], fill=STEEL)
    d.polygon([(CX - 200, CY + 180), (CX - 300, CY + 260), (CX - 260, CY + 270), (CX - 150, CY + 190)], fill=STEEL)
    # хвост-корма (тёмный, с дюзами)
    d.polygon([(CX - 300, CY - 40), (CX - 390, CY - 70), (CX - 380, CY - 60), (CX - 300, CY - 15)], fill=DARK)
    d.polygon([(CX - 300, CY + 40), (CX - 390, CY + 70), (CX - 380, CY + 60), (CX - 300, CY + 15)], fill=DARK)
    d.rectangle([CX - 400, CY - 20, CX - 380, CY + 20], fill=FIRE, outline=BORDER, width=3)
    save(img, 'sil_turtle')


if __name__ == '__main__':
    home()
    flower()
    dragon()
    ghost()
    cigarette()
    lightbulb()
    cat()
    skull()
    dragonfly()
    turtle()