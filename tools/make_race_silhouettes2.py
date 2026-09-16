# -*- coding: utf-8 -*-
# Силуэты АВАТАРОВ РАС — пачка 2: human (Люди), brimstone_bg (Курильщик с фоном среды),
# fumarole a/b/c/d (4 подхода), geode a/b/c (3 улучшения). 1024x1024, анфас, голова целиком.
import os, math
from PIL import Image, ImageDraw

OUT = r'C:\Zorion2\ai_drafts\silhouettes_races'
SIZE = 1024
CX, CY = SIZE // 2, SIZE // 2

BORDER = (12, 12, 12)

SKIN_HUMAN = (222, 184, 135)
HAIR = (52, 46, 40)
SKIN_BASALT = (60, 55, 50)
SKIN_GREY = (150, 155, 160)
SKIN_CRYSTAL = (232, 224, 186)
AMBER = (230, 150, 40)
DARK = (40, 44, 55)
LAVA = (230, 90, 30)
GLASS = (180, 210, 235)
PURPLE = (140, 90, 180)
BRONZE = (140, 90, 45)
WHITE = (235, 235, 235)
SMOKE = (85, 88, 95)


def new_canvas():
    img = Image.new('RGB', (SIZE, SIZE), (5, 5, 5))
    return img, ImageDraw.Draw(img)


def save(img, name):
    os.makedirs(OUT, exist_ok=True)
    img.save(os.path.join(OUT, name + '.png'))
    print('Saved:', name)


def border(d, pts, w=5):
    d.line(pts + [pts[0]], fill=BORDER, width=w)


# 1. ЛЮДИ — человеческое лицо, анфас, нейтральные черты, голова + шея
def human():
    img, d = new_canvas()
    # шея
    d.polygon([(CX - 90, CY + 200), (CX + 90, CY + 200), (CX + 70, CY + 320), (CX - 70, CY + 320)], fill=SKIN_HUMAN)
    border(d, [(CX - 90, CY + 200), (CX + 90, CY + 200), (CX + 70, CY + 320), (CX - 70, CY + 320)], 5)
    # овал лица (крупно)
    d.ellipse([CX - 230, CY - 330, CX + 230, CY + 260], fill=SKIN_HUMAN, outline=BORDER, width=6)
    # волосы (тёмные, сверху)
    d.pieslice([CX - 245, CY - 350, CX + 245, CY + 100], start=180, end=360, fill=HAIR)
    d.arc([CX - 245, CY - 350, CX + 245, CY + 100], start=180, end=360, fill=BORDER, width=6)
    # уши
    d.ellipse([CX - 260, CY - 90, CX - 200, CY + 10], fill=SKIN_HUMAN, outline=BORDER, width=5)
    d.ellipse([CX + 200, CY - 90, CX + 260, CY + 10], fill=SKIN_HUMAN, outline=BORDER, width=5)
    # брови
    d.line([CX - 150, CY - 120, CX - 50, CY - 130], fill=HAIR, width=10)
    d.line([CX + 50, CY - 130, CX + 150, CY - 120], fill=HAIR, width=10)
    # глаза (белок + радужка + зрачок)
    for s in (-1, 1):
        d.ellipse([CX + s * 140 - 55, CY - 70, CX + s * 140 + 55, CY + 30], fill=(245, 245, 245), outline=BORDER, width=4)
        d.ellipse([CX + s * 140 - 28, CY - 45, CX + s * 140 + 28, CY + 5], fill=(90, 120, 150), outline=BORDER, width=3)
        d.ellipse([CX + s * 140 - 14, CY - 32, CX + s * 140 + 14, CY - 8], fill=DARK)
    # нос
    d.line([CX, CY - 40, CX - 12, CY + 60], fill=(205, 165, 120), width=8)
    d.line([CX - 12, CY + 60, CX + 18, CY + 55], fill=(205, 165, 120), width=8)
    # губы
    d.ellipse([CX - 60, CY + 80, CX + 60, CY + 120], fill=(200, 130, 120), outline=BORDER, width=4)
    d.line([CX - 55, CY + 92, CX + 55, CY + 92], fill=BORDER, width=4)
    # скулы (лёгкая тень)
    d.line([CX - 200, CY + 40, CX - 140, CY + 90], fill=(205, 165, 120), width=6)
    d.line([CX + 200, CY + 40, CX + 140, CY + 90], fill=(205, 165, 120), width=6)
    save(img, 'sil_race_human')


# 2. КУРИЛЬЩИК С ФОНОМ СРЕДЫ — голова как v1 + серные источники, дым, янтарное свечение сзади
def brimstone_bg():
    img, d = new_canvas()
    # ФОН: серные трубы-источники по бокам
    d.rectangle([0, CY + 100, 260, SIZE], fill=(45, 42, 38))
    d.rectangle([SIZE - 260, CY + 100, SIZE, SIZE], fill=(45, 42, 38))
    for s in (-1, 1):
        x0 = 0 if s < 0 else SIZE - 260
        for i, yy in enumerate(range(CY + 200, SIZE, 120)):
            d.ellipse([x0 + 30, yy, x0 + 230, yy + 90], fill=(45, 42, 38), outline=AMBER, width=3)
    # клубы дыма (серые пятна по краям)
    for (dx, dy, r) in [(-360, -180, 90), (360, -240, 80), (-380, 80, 70), (380, 20, 85), (-330, -320, 60), (330, -340, 65)]:
        d.ellipse([CX + dx - r, CY + dy - r, CX + dx + r, CY + dy + r], fill=SMOKE, outline=BORDER, width=4)
    # янтарное свечение снизу (подсветка)
    d.ellipse([CX - 200, CY + 300, CX + 200, CY + 480], fill=(60, 45, 30), outline=BORDER, width=4)
    # ГОЛОВА (как v1)
    d.ellipse([CX - 280, CY - 320, CX + 280, CY + 260], fill=SKIN_BASALT, outline=BORDER, width=6)
    d.ellipse([CX - 170, CY + 180, CX + 170, CY + 340], fill=(45, 42, 38), outline=BORDER, width=6)
    for s in (-1, 1):
        d.arc([CX + s * 140 - 140, CY - 420, CX + s * 140 + 140, CY - 260], start=30 if s > 0 else 210,
              end=150 if s > 0 else 330, fill=(90, 80, 70), width=24)
    for s in (-1, 1):
        d.polygon([(CX + s * 130 - 22, CY - 60), (CX + s * 130 + 22, CY - 60), (CX + s * 130, CY + 40)], fill=AMBER)
        border(d, [(CX + s * 130 - 22, CY - 60), (CX + s * 130 + 22, CY - 60), (CX + s * 130, CY + 40)], 4)
    d.line([CX, CY + 80, CX - 120, CY + 180], fill=AMBER, width=6)
    d.line([CX, CY + 80, CX + 120, CY + 180], fill=AMBER, width=6)
    d.line([CX - 200, CY - 100, CX - 120, CY - 40], fill=AMBER, width=5)
    d.line([CX - 90, CY + 230, CX + 90, CY + 230], fill=DARK, width=8)
    d.polygon([(CX - 110, CY + 340), (CX + 110, CY + 340), (CX + 90, CY + 420), (CX - 90, CY + 420)], fill=SKIN_BASALT)
    border(d, [(CX - 110, CY + 340), (CX + 110, CY + 340), (CX + 90, CY + 420), (CX - 90, CY + 420)], 5)
    save(img, 'sil_race_brimstone_bg')


# 3. ФУМАРОЛЬНИК A — газовый столб с многослойными веерами
def fumarole_a():
    img, d = new_canvas()
    d.rectangle([CX - 90, CY - 300, CX + 90, CY + 260], fill=SKIN_GREY, outline=BORDER, width=6)
    for s in (-1, 1):
        for i, ang in enumerate([-55, -35, -15, 15, 35, 55]):
            x0, y0 = CX + s * 90, CY - 140 + i * 40
            x1 = x0 + s * 230 * math.cos(math.radians(ang))
            y1 = y0 + 230 * math.sin(math.radians(ang)) * (0.45 if abs(ang) > 40 else 1.0)
            d.polygon([(x0, y0), (x1 - s * 16, y1 - 8), (x1 + s * 16, y1 + 8)], fill=WHITE)
            border(d, [(x0, y0), (x1 - s * 16, y1 - 8), (x1 + s * 16, y1 + 8)], 4)
    d.ellipse([CX - 45, CY - 330, CX + 45, CY - 260], fill=DARK, outline=BORDER, width=4)
    d.line([CX - 90, CY - 60, CX + 90, CY - 60], fill=BRONZE, width=6)
    d.line([CX - 85, CY + 80, CX + 85, CY + 80], fill=BRONZE, width=6)
    # нижний веер (подложка)
    for s in (-1, 1):
        d.polygon([(CX + s * 60, CY + 200), (CX + s * 200, CY + 320), (CX + s * 20, CY + 340)], fill=SKIN_GREY)
        border(d, [(CX + s * 60, CY + 200), (CX + s * 200, CY + 320), (CX + s * 20, CY + 340)], 4)
    save(img, 'sil_race_fumarole_a')


# 4. ФУМАРОЛЬНИК B — ветвистый крио-коралл (трубы-ветви)
def fumarole_b():
    img, d = new_canvas()
    # главный ствол
    d.polygon([(CX - 55, CY + 300), (CX - 30, CY - 280), (CX + 30, CY - 280), (CX + 55, CY + 300)], fill=SKIN_GREY)
    border(d, [(CX - 55, CY + 300), (CX - 30, CY - 280), (CX + 30, CY - 280), (CX + 55, CY + 300)], 6)
    # ветви (по 3 с каждой стороны, вверх)
    for s in (-1, 1):
        for i, (hgt, ang) in enumerate([(-150, 25), (-60, 15), (30, 5)]):
            x0 = CX + s * 30
            y0 = CY + hgt
            x1 = x0 + s * (150 + i * 40)
            y1 = y0 - 120 - i * 30
            d.line([x0, y0, x1, y1], fill=SKIN_GREY, width=16)
            d.ellipse([x1 - 40, y1 - 40, x1 + 40, y1 + 40], fill=WHITE, outline=BORDER, width=4)
    # жерла-поры на стволе
    for i, yy in enumerate(range(CY - 180, CY + 240, 90)):
        d.ellipse([CX - 22, yy, CX + 22, yy + 40], fill=DARK, outline=BORDER, width=3)
    save(img, 'sil_race_fumarole_b')


# 5. ФУМАРОЛЬНИК C — вулканический конус-жерло с дымом
def fumarole_c():
    img, d = new_canvas()
    # конус
    d.polygon([(CX - 300, CY + 280), (CX + 300, CY + 280), (CX + 110, CY - 260), (CX - 110, CY - 260)], fill=SKIN_GREY)
    border(d, [(CX - 300, CY + 280), (CX + 300, CY + 280), (CX + 110, CY - 260), (CX - 110, CY - 260)], 6)
    # жерло (тёмное)
    d.ellipse([CX - 90, CY - 300, CX + 90, CY - 200], fill=DARK, outline=BORDER, width=5)
    # дым из жерла (серые клубы)
    for (dx, dy, r) in [(-50, -340, 60), (40, -380, 75), (-10, -440, 55), (80, -310, 45)]:
        d.ellipse([CX + dx - r, CY + dy - r, CX + dx + r, CY + dy + r], fill=SMOKE, outline=BORDER, width=4)
    # слои на конусе
    d.line([CX - 240, CY + 160, CX + 240, CY + 160], fill=BRONZE, width=7)
    d.line([CX - 170, CY + 40, CX + 170, CY + 40], fill=BRONZE, width=6)
    save(img, 'sil_race_fumarole_c')


# 6. ФУМАРОЛЬНИК D — трубный орган (набор вертикальных труб разной высоты)
def fumarole_d():
    img, d = new_canvas()
    tubes = [(-250, -260, 520), (-150, -320, 580), (-50, -380, 640), (50, -360, 620), (150, -300, 560), (250, -220, 480)]
    for (dx, top, hgt) in tubes:
        x0, y0 = CX + dx - 38, CY + top
        x1, y1 = CX + dx + 38, CY + top + hgt
        d.rectangle([x0, y0, x1, y1], fill=SKIN_GREY, outline=BORDER, width=5)
        d.ellipse([x0 - 6, y0 - 20, x1 + 6, y0 + 20], fill=DARK, outline=BORDER, width=4)
        d.line([x0, y0 + hgt // 3, x1, y0 + hgt // 3], fill=BRONZE, width=5)
    # нижняя плита
    d.rectangle([CX - 320, CY + 300, CX + 320, CY + 400], fill=SKIN_GREY, outline=BORDER, width=6)
    save(img, 'sil_race_fumarole_d')


# 7. КРИСТАЛЛИТ A — гранёный кристаллический «череп» (голова-многогранник)
def geode_a():
    img, d = new_canvas()
    # голова-октаэдр (крупный)
    d.polygon([(CX, CY - 380), (CX + 240, CY - 20), (CX, CY + 260), (CX - 240, CY - 20)], fill=SKIN_CRYSTAL)
    border(d, [(CX, CY - 380), (CX + 240, CY - 20), (CX, CY + 260), (CX - 240, CY - 20)], 6)
    # боковые грани
    d.polygon([(CX, CY - 380), (CX + 240, CY - 20), (CX + 160, CY + 60)], fill=PURPLE)
    border(d, [(CX, CY - 380), (CX + 240, CY - 20), (CX + 160, CY + 60)], 5)
    d.polygon([(CX, CY - 380), (CX - 240, CY - 20), (CX - 160, CY + 60)], fill=PURPLE)
    border(d, [(CX, CY - 380), (CX - 240, CY - 20), (CX - 160, CY + 60)], 5)
    # грани-глаза (ромбы, светящиеся)
    for s in (-1, 1):
        d.polygon([(CX + s * 110, CY - 110), (CX + s * 160, CY - 30), (CX + s * 110, CY + 50), (CX + s * 60, CY - 30)], fill=GLASS)
        border(d, [(CX + s * 110, CY - 110), (CX + s * 160, CY - 30), (CX + s * 110, CY + 50), (CX + s * 60, CY - 30)], 4)
    # ядро
    d.ellipse([CX - 55, CY + 90, CX + 55, CY + 160], fill=GLASS, outline=BORDER, width=4)
    # шея-друза
    d.polygon([(CX - 70, CY + 240), (CX + 70, CY + 240), (CX + 50, CY + 360), (CX - 50, CY + 360)], fill=SKIN_CRYSTAL)
    border(d, [(CX - 70, CY + 240), (CX + 70, CY + 240), (CX + 50, CY + 360), (CX - 50, CY + 360)], 5)
    save(img, 'sil_race_geode_a')


# 8. КРИСТАЛЛИТ B — друза: центральный кристалл + боковые, грани-глаза по бокам
def geode_b():
    img, d = new_canvas()
    # центральный кристалл
    d.polygon([(CX, CY - 420), (CX + 170, CY - 60), (CX, CY + 40), (CX - 170, CY - 60)], fill=SKIN_CRYSTAL)
    border(d, [(CX, CY - 420), (CX + 170, CY - 60), (CX, CY + 40), (CX - 170, CY - 60)], 6)
    # боковые кристаллы
    for s in (-1, 1):
        d.polygon([(CX + s * 240, CY - 160), (CX + s * 350, CY + 30), (CX + s * 260, CY + 140), (CX + s * 160, CY + 50)], fill=SKIN_CRYSTAL)
        border(d, [(CX + s * 240, CY - 160), (CX + s * 350, CY + 30), (CX + s * 260, CY + 140), (CX + s * 160, CY + 50)], 5)
        d.polygon([(CX + s * 240, CY - 160), (CX + s * 350, CY + 30), (CX + s * 290, CY + 70)], fill=PURPLE)
        border(d, [(CX + s * 240, CY - 160), (CX + s * 350, CY + 30), (CX + s * 290, CY + 70)], 4)
    # глаза-грани (ромбы, в центральном кристалле)
    for s in (-1, 1):
        d.polygon([(CX + s * 70, CY - 120), (CX + s * 100, CY - 70), (CX + s * 70, CY - 20), (CX + s * 40, CY - 70)], fill=GLASS)
        border(d, [(CX + s * 70, CY - 120), (CX + s * 100, CY - 70), (CX + s * 70, CY - 20), (CX + s * 40, CY - 70)], 4)
    # ядро
    d.ellipse([CX - 45, CY + 10, CX + 45, CY + 70], fill=GLASS, outline=BORDER, width=4)
    # основание-друза
    d.polygon([(CX - 260, CY + 140), (CX + 260, CY + 140), (CX + 300, CY + 300), (CX - 300, CY + 300)], fill=SKIN_CRYSTAL)
    border(d, [(CX - 260, CY + 140), (CX + 260, CY + 140), (CX + 300, CY + 300), (CX - 300, CY + 300)], 6)
    save(img, 'sil_race_geode_b')


# 9. КРИСТАЛЛИТ C — кристаллическая голова с шеей-друзой, тёплые грани (без «глаз»)
def geode_c():
    img, d = new_canvas()
    # голова-кристалл (огранка, тёплый крем + фиолетовые боковые)
    d.polygon([(CX, CY - 360), (CX + 210, CY - 60), (CX, CY + 180), (CX - 210, CY - 60)], fill=SKIN_CRYSTAL)
    border(d, [(CX, CY - 360), (CX + 210, CY - 60), (CX, CY + 180), (CX - 210, CY - 60)], 6)
    # грани
    for s in (-1, 1):
        d.polygon([(CX, CY - 360), (CX + s * 210, CY - 60), (CX + s * 130, CY + 40)], fill=PURPLE)
        border(d, [(CX, CY - 360), (CX + s * 210, CY - 60), (CX + s * 130, CY + 40)], 5)
    # «лицо» — светящееся ядро вертикальное (без ромбов-глаз)
    d.ellipse([CX - 40, CY - 160, CX + 40, CY - 20], fill=GLASS, outline=BORDER, width=4)
    # шипы сверху
    for s in (-1, 1):
        d.polygon([(CX + s * 80, CY - 320), (CX + s * 120, CY - 420), (CX + s * 150, CY - 300)], fill=SKIN_CRYSTAL)
        border(d, [(CX + s * 80, CY - 320), (CX + s * 120, CY - 420), (CX + s * 150, CY - 300)], 5)
    # шея-друза (кристаллы вниз)
    d.polygon([(CX - 80, CY + 160), (CX + 80, CY + 160), (CX + 60, CY + 330), (CX - 60, CY + 330)], fill=SKIN_CRYSTAL)
    border(d, [(CX - 80, CY + 160), (CX + 80, CY + 160), (CX + 60, CY + 330), (CX - 60, CY + 330)], 5)
    for s in (-1, 1):
        d.polygon([(CX + s * 60, CY + 220), (CX + s * 130, CY + 300), (CX + s * 20, CY + 330)], fill=PURPLE)
        border(d, [(CX + s * 60, CY + 220), (CX + s * 130, CY + 300), (CX + s * 20, CY + 330)], 4)
    save(img, 'sil_race_geode_c')


if __name__ == '__main__':
    human()
    brimstone_bg()
    fumarole_a()
    fumarole_b()
    fumarole_c()
    fumarole_d()
    geode_a()
    geode_b()
    geode_c()