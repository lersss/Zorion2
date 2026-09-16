# -*- coding: utf-8 -*-
# Силуэты АВАТАРОВ РАС — пачка 4: ЭКСПЕРИМЕНТ с форм-словами (как корабли: «SHAPED LIKE X»).
# 8 новых слов: cross, diamond, bell, clover, wave, prism, knot, gear.
# Главное: НИЗ «ПОСАЖЕН» — шея/воротник/плечи уходят за НИЖНИЙ край холста (1024),
# чтобы голова не «висела в воздухе» (после кадрирования низ станет краем картинки).
import os, math
from PIL import Image, ImageDraw

OUT = r'C:\Zorion2\ai_drafts\silhouettes_races'
SIZE = 1024
CX, CY = SIZE // 2, SIZE // 2

BORDER = (12, 12, 12)
SKIN_CRYSTAL = (232, 224, 186)
SKIN_GREY = (150, 155, 160)
SKIN_BASALT = (60, 55, 50)
SKIN_FUNGI = (200, 175, 120)
SKIN_WATER = (120, 185, 190)
PURPLE = (140, 90, 180)
GLASS = (180, 210, 235)
DARK = (40, 44, 55)
BRONZE = (140, 90, 45)
WHITE = (235, 235, 235)
AMBER = (230, 150, 40)
SMOKE = (85, 88, 95)
POISON = (110, 200, 160)


def new_canvas():
    img = Image.new('RGB', (SIZE, SIZE), (5, 5, 5))
    return img, ImageDraw.Draw(img)


def save(img, name):
    os.makedirs(OUT, exist_ok=True)
    img.save(os.path.join(OUT, name + '.png'))
    print('Saved:', name)


def border(d, pts, w=5):
    d.line(pts + [pts[0]], fill=BORDER, width=w)


def neck_to_bottom(d, top_y, skin, w=140):
    """Шея/воротник от top_y ДО НИЖНЕГО края холста — «посаженный» низ."""
    d.polygon([(CX - w, top_y), (CX + w, top_y), (CX + w - 40, SIZE), (CX - w + 40, SIZE)], fill=skin)
    d.line([(CX - w, top_y), (CX + w, top_y)], fill=BORDER, width=5)


# 1. CROSS (F6 кристалл) — голова-крест: вертикальный гранёный кристалл + крылья-перекладина
def cross_beast():
    img, d = new_canvas()
    # вертикальный кристалл (голова)
    d.polygon([(CX, CY - 420), (CX + 120, CY - 100), (CX, CY + 120), (CX - 120, CY - 100)], fill=SKIN_CRYSTAL)
    border(d, [(CX, CY - 420), (CX + 120, CY - 100), (CX, CY + 120), (CX - 120, CY - 100)], 6)
    d.polygon([(CX, CY - 420), (CX + 120, CY - 100), (CX + 80, CY - 60)], fill=PURPLE)
    border(d, [(CX, CY - 420), (CX + 120, CY - 100), (CX + 80, CY - 60)], 5)
    d.polygon([(CX, CY - 420), (CX - 120, CY - 100), (CX - 80, CY - 60)], fill=PURPLE)
    border(d, [(CX, CY - 420), (CX - 120, CY - 100), (CX - 80, CY - 60)], 5)
    # перекладина-крылья (горизонтальная)
    d.polygon([(CX - 330, CY - 120), (CX + 330, CY - 120), (CX + 330, CY - 40), (CX - 330, CY - 40)], fill=SKIN_CRYSTAL)
    border(d, [(CX - 330, CY - 120), (CX + 330, CY - 120), (CX + 330, CY - 40), (CX - 330, CY - 40)], 6)
    # глаза-ромбы на перекладине
    for s in (-1, 1):
        d.polygon([(CX + s * 150, CY - 110), (CX + s * 190, CY - 80), (CX + s * 150, CY - 50), (CX + s * 110, CY - 80)], fill=GLASS)
        border(d, [(CX + s * 150, CY - 110), (CX + s * 190, CY - 80), (CX + s * 150, CY - 50), (CX + s * 110, CY - 80)], 4)
    # ядро на макушке
    d.ellipse([CX - 30, CY - 300, CX + 30, CY - 240], fill=GLASS, outline=BORDER, width=4)
    neck_to_bottom(d, CY + 120, SKIN_CRYSTAL, w=130)
    save(img, 'sil_race_cross_beast')


# 2. DIAMOND (F6 кристалл) — ромбовидная гранёная голова, глаза-щели, пасть
def diamond_beast():
    img, d = new_canvas()
    # голова-ромб
    d.polygon([(CX, CY - 380), (CX + 250, CY - 40), (CX, CY + 260), (CX - 250, CY - 40)], fill=SKIN_CRYSTAL)
    border(d, [(CX, CY - 380), (CX + 250, CY - 40), (CX, CY + 260), (CX - 250, CY - 40)], 6)
    # грани
    d.polygon([(CX, CY - 380), (CX + 250, CY - 40), (CX + 160, CY + 40)], fill=PURPLE)
    border(d, [(CX, CY - 380), (CX + 250, CY - 40), (CX + 160, CY + 40)], 5)
    d.polygon([(CX, CY - 380), (CX - 250, CY - 40), (CX - 160, CY + 40)], fill=PURPLE)
    border(d, [(CX, CY - 380), (CX - 250, CY - 40), (CX - 160, CY + 40)], 5)
    # глаза-щели (вертикальные, светящиеся)
    for s in (-1, 1):
        d.polygon([(CX + s * 110 - 18, CY - 80), (CX + s * 110 + 18, CY - 80), (CX + s * 110, CY + 20)], fill=GLASS)
        border(d, [(CX + s * 110 - 18, CY - 80), (CX + s * 110 + 18, CY - 80), (CX + s * 110, CY + 20)], 4)
    # пасть
    d.polygon([(CX - 70, CY + 150), (CX + 70, CY + 150), (CX + 40, CY + 210), (CX - 40, CY + 210)], fill=DARK)
    border(d, [(CX - 70, CY + 150), (CX + 70, CY + 150), (CX + 40, CY + 210), (CX - 40, CY + 210)], 5)
    neck_to_bottom(d, CY + 260, SKIN_CRYSTAL, w=130)
    save(img, 'sil_race_diamond_beast')


# 3. BELL (F8 камень) — голова-колокол: широкая снизу, жерло-пасть, глаза-трубы
def bell_beast():
    img, d = new_canvas()
    # колокол (широкая нижняя часть)
    d.polygon([(CX, CY - 320), (CX + 130, CY - 120), (CX + 280, CY + 160), (CX - 280, CY + 160), (CX - 130, CY - 120)], fill=SKIN_GREY)
    border(d, [(CX, CY - 320), (CX + 130, CY - 120), (CX + 280, CY + 160), (CX - 280, CY + 160), (CX - 130, CY - 120)], 6)
    # жерло-пасть (широкое, внизу колокола)
    d.ellipse([CX - 120, CY + 100, CX + 120, CY + 220], fill=DARK, outline=BORDER, width=6)
    # глаза-трубы сверху
    for s in (-1, 1):
        d.rectangle([CX + s * 90 - 25, CY - 250, CX + s * 90 + 25, CY - 140], fill=SKIN_GREY, outline=BORDER, width=5)
        d.ellipse([CX + s * 90 - 25, CY - 275, CX + s * 90 + 25, CY - 225], fill=DARK, outline=BORDER, width=4)
    # кольца на колоколе
    d.line([CX - 240, CY - 20, CX + 240, CY - 20], fill=BRONZE, width=6)
    d.line([CX - 270, CY + 70, CX + 270, CY + 70], fill=BRONZE, width=6)
    # язык колокола (выступ снизу)
    d.polygon([(CX - 50, CY + 220), (CX + 50, CY + 220), (CX + 30, CY + 300), (CX - 30, CY + 300)], fill=SKIN_GREY)
    border(d, [(CX - 50, CY + 220), (CX + 50, CY + 220), (CX + 30, CY + 300), (CX - 30, CY + 300)], 5)
    neck_to_bottom(d, CY + 300, SKIN_GREY, w=120)
    save(img, 'sil_race_bell_beast')


# 4. CLOVER (F3 гриб) — голова-трилистник: три шляпки, глаза между ними
def clover_beast():
    img, d = new_canvas()
    # три шляпки (клевер)
    for (dx, dy) in [(-170, -140), (170, -140), (0, 40)]:
        d.pieslice([CX + dx - 180, CY + dy - 200, CX + dx + 180, CY + dy + 60], start=180, end=360, fill=(190, 160, 100))
        d.arc([CX + dx - 180, CY + dy - 200, CX + dx + 180, CY + dy + 60], start=180, end=360, fill=BORDER, width=6)
    # пятна
    for (dx, dy) in [(-170, -160), (170, -160), (0, 10)]:
        d.ellipse([CX + dx - 70, CY + dy - 130, CX + dx + 10, CY + dy - 60], fill=(230, 210, 160), outline=BORDER, width=4)
    # глаза (между шляпками, в центре)
    for s in (-1, 1):
        d.ellipse([CX + s * 40 - 22, CY - 70, CX + s * 40 + 22, CY - 10], fill=DARK, outline=BORDER, width=4)
        d.ellipse([CX + s * 40 - 10, CY - 56, CX + s * 40 + 10, CY - 24], fill=POISON)
    # ножка-шея (вниз до края)
    d.rectangle([CX - 90, CY + 60, CX + 90, CY + 220], fill=SKIN_FUNGI, outline=BORDER, width=6)
    # споры
    for (dx, dy) in [(-120, 170), (120, 170), (0, 200)]:
        d.ellipse([CX + dx - 20, CY + dy - 20, CX + dx + 20, CY + dy + 20], fill=(230, 210, 160), outline=BORDER, width=4)
    neck_to_bottom(d, CY + 220, SKIN_FUNGI, w=110)
    save(img, 'sil_race_clover_beast')


# 5. WAVE (F1 вода) — голова с волнообразным гребнем, жабры, глаза-бусинки
def wave_beast():
    img, d = new_canvas()
    # голова (овальная, водная)
    d.ellipse([CX - 230, CY - 260, CX + 230, CY + 220], fill=SKIN_WATER, outline=BORDER, width=6)
    # волновой гребень (плавник-волна сверху)
    for i, (dx, hgt) in enumerate([(-180, 130), (-90, 200), (0, 250), (90, 200), (180, 130)]):
        x0 = CX + dx
        d.polygon([(x0 - 55, CY - 250), (x0 + 55, CY - 250), (x0, CY - 250 - hgt)], fill=SKIN_WATER)
        border(d, [(x0 - 55, CY - 250), (x0 + 55, CY - 250), (x0, CY - 250 - hgt)], 5)
    # глаза-бусинки на стебельках
    for s in (-1, 1):
        d.line([CX + s * 120, CY - 60, CX + s * 180, CY - 140], fill=SKIN_WATER, width=10)
        d.ellipse([CX + s * 180 - 30, CY - 180, CX + s * 180 + 30, CY - 120], fill=DARK, outline=BORDER, width=4)
        d.ellipse([CX + s * 180 - 12, CY - 160, CX + s * 180 + 12, CY - 136], fill=POISON)
    # жабры-волны по бокам (дуги)
    for s in (-1, 1):
        for i in range(3):
            x0 = CX + s * 200
            y0 = CY - 20 + i * 45
            d.arc([x0 - 50, y0 - 25, x0 + 50, y0 + 45], start=90 if s > 0 else 270, end=180 if s > 0 else 360,
                  fill=BRONZE, width=6)
    # рот (волна)
    d.line([CX - 60, CY + 120, CX - 20, CY + 100, CX + 20, CY + 130, CX + 60, CY + 110], fill=DARK, width=6)
    neck_to_bottom(d, CY + 220, SKIN_WATER, w=120)
    save(img, 'sil_race_wave_beast')


# 6. PRISM (F6 кристалл) — треугольная призма-голова, ядро, грани
def prism_beast():
    img, d = new_canvas()
    # треугольная призма (голова)
    d.polygon([(CX, CY - 380), (CX + 280, CY + 120), (CX - 280, CY + 120)], fill=SKIN_CRYSTAL)
    border(d, [(CX, CY - 380), (CX + 280, CY + 120), (CX - 280, CY + 120)], 6)
    # вертикальные грани
    d.polygon([(CX, CY - 380), (CX + 280, CY + 120), (CX, CY + 150)], fill=PURPLE)
    border(d, [(CX, CY - 380), (CX + 280, CY + 120), (CX, CY + 150)], 5)
    d.polygon([(CX, CY - 380), (CX - 280, CY + 120), (CX, CY + 150)], fill=PURPLE)
    border(d, [(CX, CY - 380), (CX - 280, CY + 120), (CX, CY + 150)], 5)
    # центральная грань-лицо
    d.polygon([(CX, CY - 300), (CX + 160, CY + 40), (CX, CY + 140), (CX - 160, CY + 40)], fill=SKIN_CRYSTAL)
    border(d, [(CX, CY - 300), (CX + 160, CY + 40), (CX, CY + 140), (CX - 160, CY + 40)], 5)
    # глаза-щели
    for s in (-1, 1):
        d.polygon([(CX + s * 70 - 16, CY - 60), (CX + s * 70 + 16, CY - 60), (CX + s * 70, CY + 10)], fill=GLASS)
        border(d, [(CX + s * 70 - 16, CY - 60), (CX + s * 70 + 16, CY - 60), (CX + s * 70, CY + 10)], 4)
    # ядро
    d.ellipse([CX - 25, CY + 60, CX + 25, CY + 110], fill=GLASS, outline=BORDER, width=4)
    neck_to_bottom(d, CY + 150, SKIN_CRYSTAL, w=130)
    save(img, 'sil_race_prism_beast')


# 7. KNOT (F8 камень) — голова-узел: переплетённые трубы, жерла
def knot_beast():
    img, d = new_canvas()
    # узел из труб (переплетение)
    d.ellipse([CX - 200, CY - 200, CX + 200, CY + 200], fill=SKIN_GREY, outline=BORDER, width=6)
    # трубы, переплетённые крест-накрест
    for s in (-1, 1):
        d.line([CX + s * 60, CY - 240, CX - s * 60, CY + 240], fill=SKIN_GREY, width=34)
    d.line([CX - 240, CY - 60, CX + 240, CY + 60], fill=SKIN_GREY, width=34)
    d.line([CX - 240, CY + 60, CX + 240, CY - 60], fill=SKIN_GREY, width=34)
    # жерла на концах труб
    for (dx, dy) in [(-240, -60), (240, 60), (-240, 60), (240, -60), (0, -240), (0, 240)]:
        d.ellipse([CX + dx - 40, CY + dy - 40, CX + dx + 40, CY + dy + 40], fill=DARK, outline=BORDER, width=4)
    # глаза-щели в центре
    for s in (-1, 1):
        d.polygon([(CX + s * 55 - 15, CY - 45), (CX + s * 55 + 15, CY - 45), (CX + s * 55, CY + 25)], fill=AMBER)
        border(d, [(CX + s * 55 - 15, CY - 45), (CX + s * 55 + 15, CY - 45), (CX + s * 55, CY + 25)], 4)
    # кольца
    d.ellipse([CX - 110, CY - 110, CX + 110, CY + 110], outline=BRONZE, width=6)
    neck_to_bottom(d, CY + 240, SKIN_GREY, w=120)
    save(img, 'sil_race_knot_beast')


# 8. GEAR (F4 сера) — голова-шестерня: зубчатый контур, янтарные глаза-щели
def gear_beast():
    img, d = new_canvas()
    # зубчатый контур (шестерня): 8 зубьев
    for i in range(8):
        ang = i * 45
        x0 = CX + 190 * math.cos(math.radians(ang))
        y0 = CY + 190 * math.sin(math.radians(ang))
        x1 = CX + 290 * math.cos(math.radians(ang))
        y1 = CY + 290 * math.sin(math.radians(ang))
        xp = CX + 320 * math.cos(math.radians(ang))
        yp = CY + 320 * math.sin(math.radians(ang))
        x2 = CX + 230 * math.cos(math.radians(ang + 22))
        y2 = CY + 230 * math.sin(math.radians(ang + 22))
        x3 = CX + 230 * math.cos(math.radians(ang - 22))
        y3 = CY + 230 * math.sin(math.radians(ang - 22))
        d.polygon([(x0, y0), (x1, y1), (xp, yp), (x2, y2), (x3, y3)], fill=SKIN_BASALT)
        border(d, [(x0, y0), (x1, y1), (xp, yp), (x2, y2), (x3, y3)], 5)
    # центральный диск (голова)
    d.ellipse([CX - 210, CY - 210, CX + 210, CY + 210], fill=SKIN_BASALT, outline=BORDER, width=6)
    # отверстия шестерни (тёмные круги)
    for i in range(4):
        ang = i * 90 + 45
        dx = 130 * math.cos(math.radians(ang))
        dy = 130 * math.sin(math.radians(ang))
        d.ellipse([CX + dx - 35, CY + dy - 35, CX + dx + 35, CY + dy + 35], fill=(45, 42, 38), outline=BORDER, width=4)
    # янтарные глаза-щели в центре
    for s in (-1, 1):
        d.polygon([(CX + s * 70 - 16, CY - 45), (CX + s * 70 + 16, CY - 45), (CX + s * 70, CY + 35)], fill=AMBER)
        border(d, [(CX + s * 70 - 16, CY - 45), (CX + s * 70 + 16, CY - 45), (CX + s * 70, CY + 35)], 4)
    # пасть-щель
    d.line([CX - 50, CY + 100, CX + 50, CY + 100], fill=DARK, width=7)
    neck_to_bottom(d, CY + 210, SKIN_BASALT, w=140)
    save(img, 'sil_race_gear_beast')


if __name__ == '__main__':
    cross_beast()
    diamond_beast()
    bell_beast()
    clover_beast()
    wave_beast()
    prism_beast()
    knot_beast()
    gear_beast()