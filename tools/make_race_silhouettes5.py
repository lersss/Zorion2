# -*- coding: utf-8 -*-
# Силуэты АВАТАРОВ РАС — пачка 5: КАРДИНАЛЬНО РАЗНЫЕ ПОДХОДЫ (нащупываем стиль).
# Строгое правило ОДНО: низ «посажен» — шея/туловище уходят за нижний край холста.
# 6 ControlNet-подходов: organic, techno, material, geometry, emblem, character.
# (Живописный и макро — txt2img, в batch_races_p5.ps1, силуэтов не требуют.)
import os, math
from PIL import Image, ImageDraw

OUT = r'C:\Zorion2\ai_drafts\silhouettes_races'
SIZE = 1024
CX, CY = SIZE // 2, SIZE // 2

BORDER = (12, 12, 12)
SKIN_WATER = (120, 185, 190)
SKIN_MAGMA = (70, 55, 50)
SKIN_CRYSTAL = (232, 224, 186)
SKIN_NEON = (150, 80, 220)
SKIN_FUNGI = (200, 175, 120)
SKIN_GREY = (150, 155, 160)
PURPLE = (140, 90, 180)
GLASS = (180, 210, 235)
DARK = (40, 44, 55)
BRONZE = (140, 90, 45)
WHITE = (235, 235, 235)
AMBER = (230, 150, 40)
LAVA = (230, 90, 30)
POISON = (110, 200, 160)
CYAN = (120, 220, 235)


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
    d.polygon([(CX - w, top_y), (CX + w, top_y), (CX + w - 40, SIZE), (CX - w + 40, SIZE)], fill=skin)
    d.line([(CX - w, top_y), (CX + w, top_y)], fill=BORDER, width=5)


# 1. ORGANIC (F1 вода) — мокрая амёбоподобная голова: мембраны, щупальца, без жёстких граней
def organic_head():
    img, d = new_canvas()
    # амёба-голова (неровная, несколько эллипсов)
    d.ellipse([CX - 220, CY - 260, CX + 220, CY + 160], fill=SKIN_WATER, outline=BORDER, width=6)
    d.ellipse([CX - 300, CY - 160, CX - 120, CY + 60], fill=SKIN_WATER, outline=BORDER, width=6)
    d.ellipse([CX + 120, CY - 180, CX + 300, CY + 40], fill=SKIN_WATER, outline=BORDER, width=6)
    d.ellipse([CX - 180, CY - 320, CX + 40, CY - 160], fill=SKIN_WATER, outline=BORDER, width=6)
    # мембранные крылья (полупрозрачные намёки — тонкие дуги)
    for s in (-1, 1):
        d.arc([CX + s * 220 - 160, CY - 320, CX + s * 220 + 160, CY + 20], start=20 if s > 0 else 200,
              end=160 if s > 0 else 340, fill=GLASS, width=8)
    # глаза (мягкие, светящиеся точки)
    for s in (-1, 1):
        d.ellipse([CX + s * 80 - 30, CY - 80, CX + s * 80 + 30, CY - 20], fill=DARK, outline=BORDER, width=4)
        d.ellipse([CX + s * 80 - 14, CY - 66, CX + s * 80 + 14, CY - 34], fill=POISON)
    # рот (мокрая складка)
    d.line([CX - 45, CY + 60, CX + 45, CY + 60], fill=DARK, width=6)
    # щупальца вниз (к шее)
    for s in (-1, 1):
        for i in range(3):
            x0 = CX + s * (40 + i * 45)
            d.line([x0, CY + 150, x0 + s * 30, CY + 260 + i * 20], fill=SKIN_WATER, width=10)
    neck_to_bottom(d, CY + 230, SKIN_WATER, w=130)
    save(img, 'sil_race_organic_head')


# 2. TECHNO (F5 лава) — голова-механизм: коробка-череп, трубы, шестерни, лавовые жилы
def techno_head():
    img, d = new_canvas()
    # коробка-череп
    d.rectangle([CX - 200, CY - 280, CX + 200, CY + 160], fill=(55, 60, 70), outline=BORDER, width=6)
    # лавовые жилы на коробке
    d.line([CX - 160, CY - 240, CX - 40, CY - 100], fill=LAVA, width=7)
    d.line([CX + 40, CY - 200, CX + 160, CY - 60], fill=LAVA, width=7)
    # глаза-лампы (яркие, круглые)
    for s in (-1, 1):
        d.ellipse([CX + s * 90 - 35, CY - 90, CX + s * 90 + 35, CY - 20], fill=DARK, outline=BORDER, width=4)
        d.ellipse([CX + s * 90 - 15, CY - 70, CX + s * 90 + 15, CY - 40], fill=LAVA)
    # шестерни по бокам
    for s in (-1, 1):
        for i in range(6):
            ang = i * 60 + 30
            x0 = CX + s * 240 + 40 * math.cos(math.radians(ang))
            y0 = CY - 100 + 40 * math.sin(math.radians(ang))
            d.ellipse([x0 - 12, y0 - 12, x0 + 12, y0 + 12], fill=BRONZE, outline=BORDER, width=3)
        d.ellipse([CX + s * 240 - 30, CY - 140, CX + s * 240 + 30, CY - 80], fill=BRONZE, outline=BORDER, width=5)
    # трубы сверху
    for s in (-1, 1):
        d.rectangle([CX + s * 60 - 15, CY - 380, CX + s * 60 + 15, CY - 260], fill=(55, 60, 70), outline=BORDER, width=4)
        d.ellipse([CX + s * 60 - 15, CY - 405, CX + s * 60 + 15, CY - 355], fill=DARK, outline=BORDER, width=3)
    # рот-рельса
    d.rectangle([CX - 90, CY + 80, CX + 90, CY + 110], fill=DARK, outline=BORDER, width=4)
    neck_to_bottom(d, CY + 160, (55, 60, 70), w=140)
    save(img, 'sil_race_techno_head')


# 3. MATERIAL (F6 кристалл) — голова из расплавленного стекла/кварца: капля, прожилки золота
def material_head():
    img, d = new_canvas()
    # капля-голова (стекло)
    d.ellipse([CX - 240, CY - 300, CX + 240, CY + 100], fill=(220, 210, 235), outline=BORDER, width=6)
    d.polygon([(CX - 240, CY + 60), (CX + 240, CY + 60), (CX + 90, CY + 200), (CX - 90, CY + 200)], fill=(220, 210, 235))
    border(d, [(CX - 240, CY + 60), (CX + 240, CY + 60), (CX + 90, CY + 200), (CX - 90, CY + 200)], 6)
    # золотые прожилки (внутри)
    for s in (-1, 1):
        d.line([CX + s * 60, CY - 180, CX + s * 180, CY - 40], fill=AMBER, width=6)
        d.line([CX + s * 180, CY - 40, CX + s * 120, CY + 60], fill=AMBER, width=5)
    d.line([CX - 40, CY - 220, CX + 40, CY - 120], fill=AMBER, width=5)
    # глаза-пустоты (тёмные, в стекле)
    for s in (-1, 1):
        d.ellipse([CX + s * 90 - 28, CY - 70, CX + s * 90 + 28, CY - 14], fill=DARK, outline=BORDER, width=4)
    # светящееся ядро
    d.ellipse([CX - 25, CY + 40, CX + 25, CY + 90], fill=GLASS, outline=BORDER, width=4)
    # ПОСАЖЕННЫЙ НИЗ: широкий контрастный воротник-туловище до края
    d.polygon([(CX - 230, CY + 140), (CX + 230, CY + 140), (CX + 210, SIZE), (CX - 210, SIZE)], fill=(150, 120, 90))
    d.line([(CX - 230, CY + 140), (CX + 230, CY + 140)], fill=BORDER, width=6)
    d.line([(CX - 230, CY + 140), (CX - 210, SIZE)], fill=BORDER, width=5)
    d.line([(CX + 230, CY + 140), (CX + 210, SIZE)], fill=BORDER, width=5)
    save(img, 'sil_race_material_head')


# 4. GEOMETRY (F9 энергия) — чистая геометрическая форма: треугольник + круг, линии, без лица
def geometry_head():
    img, d = new_canvas()
    # внешний треугольник (энергетическая рамка)
    d.polygon([(CX, CY - 360), (CX + 280, CY + 220), (CX - 280, CY + 220)], outline=CYAN, width=10)
    # внутренний круг (ядро)
    d.ellipse([CX - 140, CY - 160, CX + 140, CY + 120], fill=SKIN_NEON, outline=BORDER, width=6)
    # концентрические кольца
    d.ellipse([CX - 90, CY - 110, CX + 90, CY + 70], outline=WHITE, width=6)
    d.ellipse([CX - 45, CY - 65, CX + 45, CY + 25], outline=CYAN, width=5)
    # линии-лучи от углов
    for ang in [210, 330, 90]:
        x0 = CX + 260 * math.cos(math.radians(ang))
        y0 = CY + 260 * math.sin(math.radians(ang))
        x1 = CX + 340 * math.cos(math.radians(ang))
        y1 = CY + 340 * math.sin(math.radians(ang))
        d.line([x0, y0, x1, y1], fill=CYAN, width=8)
    # точки на круге
    for i in range(8):
        ang = i * 45
        dx = 130 * math.cos(math.radians(ang))
        dy = 130 * math.sin(math.radians(ang)) - 20
        d.ellipse([CX + dx - 10, CY + dy - 10, CX + dx + 10, CY + dy + 10], fill=WHITE)
    # ПОСАЖЕННЫЙ НИЗ: широкий светлый воротник-туловище (контрастный, до края)
    d.polygon([(CX - 240, CY + 180), (CX + 240, CY + 180), (CX + 220, SIZE), (CX - 220, SIZE)], fill=(150, 130, 190))
    d.line([(CX - 240, CY + 180), (CX + 240, CY + 180)], fill=BORDER, width=6)
    d.line([(CX - 240, CY + 180), (CX - 220, SIZE)], fill=BORDER, width=5)
    d.line([(CX + 240, CY + 180), (CX + 220, SIZE)], fill=BORDER, width=5)
    save(img, 'sil_race_geometry_head')


# 5. EMBLEM (F3 гриб) — геральдический герб: симметричный щит-гриб с мантией-воротником
def emblem_head():
    img, d = new_canvas()
    # мантия-воротник (симметричные крылья вниз)
    for s in (-1, 1):
        d.polygon([(CX, CY + 60), (CX + s * 320, CY + 40), (CX + s * 200, CY + 260), (CX, CY + 220)], fill=(140, 120, 80))
        border(d, [(CX, CY + 60), (CX + s * 320, CY + 40), (CX + s * 200, CY + 260), (CX, CY + 220)], 5)
    # щит (центральный, грибной)
    d.polygon([(CX, CY - 320), (CX + 170, CY - 100), (CX + 140, CY + 80), (CX - 140, CY + 80), (CX - 170, CY - 100)], fill=(190, 160, 100))
    border(d, [(CX, CY - 320), (CX + 170, CY - 100), (CX + 140, CY + 80), (CX - 140, CY + 80), (CX - 170, CY - 100)], 6)
    # шляпка-купол на щите
    d.pieslice([CX - 180, CY - 380, CX + 180, CY - 120], start=180, end=360, fill=(200, 170, 110))
    d.arc([CX - 180, CY - 380, CX + 180, CY - 120], start=180, end=360, fill=BORDER, width=6)
    d.ellipse([CX - 80, CY - 320, CX + 10, CY - 250], fill=(230, 210, 160), outline=BORDER, width=4)
    # глаза-символы (ромбы)
    for s in (-1, 1):
        d.polygon([(CX + s * 70 - 25, CY - 40), (CX + s * 70, CY - 10), (CX + s * 70 + 25, CY - 40), (CX + s * 70, CY - 70)], fill=DARK)
        border(d, [(CX + s * 70 - 25, CY - 40), (CX + s * 70, CY - 10), (CX + s * 70 + 25, CY - 40), (CX + s * 70, CY - 70)], 4)
    # кисточка-подвес
    d.ellipse([CX - 30, CY + 60, CX + 30, CY + 120], fill=(190, 160, 100), outline=BORDER, width=4)
    neck_to_bottom(d, CY + 220, (140, 120, 80), w=110)
    save(img, 'sil_race_emblem_head')


# 6. CHARACTER (F8 углерод) — голова с ХАРАКТЕРОМ: оскаленная пасть-жерло, горящие глаза, агрессия
def character_head():
    img, d = new_canvas()
    # голова (каменная, широкая, наклонённая вперёд)
    d.ellipse([CX - 260, CY - 280, CX + 260, CY + 200], fill=SKIN_GREY, outline=BORDER, width=6)
    # глаза-щели (злые, наклонные)
    for s in (-1, 1):
        d.polygon([(CX + s * 110 - 50, CY - 80), (CX + s * 110 + 50, CY - 20), (CX + s * 110 + 20, CY + 10), (CX + s * 110 - 20, CY - 40)], fill=DARK)
        border(d, [(CX + s * 110 - 50, CY - 80), (CX + s * 110 + 50, CY - 20), (CX + s * 110 + 20, CY + 10), (CX + s * 110 - 20, CY - 40)], 4)
        d.ellipse([CX + s * 110 - 12, CY - 55, CX + s * 110 + 12, CY - 25], fill=AMBER)
    # оскаленная пасть-жерло (широкая, с зубами)
    d.polygon([(CX - 130, CY + 90), (CX + 130, CY + 90), (CX + 90, CY + 200), (CX - 90, CY + 200)], fill=DARK)
    border(d, [(CX - 130, CY + 90), (CX + 130, CY + 90), (CX + 90, CY + 200), (CX - 90, CY + 200)], 6)
    # зубы (острые)
    for s in (-1, 1):
        for i, dx in enumerate([-90, -40, 10, 60]):
            d.polygon([(CX + s * dx, CY + 90), (CX + s * dx + 20, CY + 90), (CX + s * dx + 10, CY + 40)], fill=WHITE)
            border(d, [(CX + s * dx, CY + 90), (CX + s * dx + 20, CY + 90), (CX + s * dx + 10, CY + 40)], 3)
    # брови-гребни (агрессия)
    for s in (-1, 1):
        d.polygon([(CX + s * 150 - 60, CY - 130), (CX + s * 150 + 60, CY - 100), (CX + s * 150 + 30, CY - 80), (CX + s * 150 - 30, CY - 100)], fill=SKIN_GREY)
        border(d, [(CX + s * 150 - 60, CY - 130), (CX + s * 150 + 60, CY - 100), (CX + s * 150 + 30, CY - 80), (CX + s * 150 - 30, CY - 100)], 4)
    neck_to_bottom(d, CY + 200, SKIN_GREY, w=150)
    save(img, 'sil_race_character_head')


if __name__ == '__main__':
    organic_head()
    techno_head()
    material_head()
    geometry_head()
    emblem_head()
    character_head()