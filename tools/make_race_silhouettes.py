# -*- coding: utf-8 -*-
# Силуэты АВАТАРОВ РАС — пилот 2: ГОЛОВА ЦЕЛИКОМ (без бюста; у тварей — короткая шея).
# 1024x1024, чёрный фон, цветные модули. Формы строго по основе расы из спеки 99.2.21:
# твари для биологических семейств, абстрактные формы для кристаллов/газа/энергии.
# БЕЗ «человеческих» глаз и кожи (глаза создают «лицо» -> ксеноморф).
# Слова: oceanid, frostwalker, fungoid, brimstone, magmite, geode, nimbus, fumarole, magnetar.
import os, math
from PIL import Image, ImageDraw

OUT = r'C:\Zorion2\ai_drafts\silhouettes_races'
SIZE = 1024
CX, CY = SIZE // 2, SIZE // 2

BORDER = (12, 12, 12)

SKIN_WATER = (120, 185, 190)     # F1 водные: бирюзовый хитин
SKIN_FROST = (170, 210, 235)     # F2 крио: лёд
SKIN_FUNGI = (200, 175, 120)     # F3 метановые: грибной крем
SKIN_BASALT = (60, 55, 50)       # F4 серные: базальт
SKIN_MAGMA = (70, 55, 50)        # F5 терморедокс: раскалённый камень
SKIN_CRYSTAL = (232, 224, 186)   # F6 кремниевые: кристальный крем
SKIN_CLOUD = (170, 200, 225)     # F7 небесные: облачный
SKIN_GREY = (150, 155, 160)      # F8 углекислые: серый
SKIN_NEON = (150, 80, 220)       # F9 экзотика: неон-фиолет

DARK = (40, 44, 55)
AMBER = (230, 150, 40)
LAVA = (230, 90, 30)
GOLD = (210, 170, 60)
POISON = (110, 200, 160)
GLASS = (180, 210, 235)
BRONZE = (140, 90, 45)
PURPLE = (140, 90, 180)
WHITE = (235, 235, 235)
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


def neck(d, top_y, skin, w=120, h=90):
    """Короткий кусок шеи под головой (только для тварей)."""
    d.polygon([(CX - w, top_y), (CX + w, top_y), (CX + w - 30, top_y + h), (CX - w + 30, top_y + h)], fill=skin)
    border(d, [(CX - w, top_y), (CX + w, top_y), (CX + w - 30, top_y + h), (CX - w + 30, top_y + h)], 5)


# 1. F4 Серные — КУРИЛЬЩИК: массивная каменная башка, рога, янтарные щели-глаза, челюсть
def brimstone():
    img, d = new_canvas()
    # череп (базальт, широкий)
    d.ellipse([CX - 280, CY - 320, CX + 280, CY + 260], fill=SKIN_BASALT, outline=BORDER, width=6)
    # нижняя челюсть (тёмная, массивная)
    d.ellipse([CX - 170, CY + 180, CX + 170, CY + 340], fill=(45, 42, 38), outline=BORDER, width=6)
    # рога (загнутые, из макушки)
    for s in (-1, 1):
        d.arc([CX + s * 140 - 140, CY - 420, CX + s * 140 + 140, CY - 260], start=30 if s > 0 else 210,
              end=150 if s > 0 else 330, fill=(90, 80, 70), width=24)
    # янтарные щели-глаза (узкие, вертикальные — НЕ человеческие)
    for s in (-1, 1):
        d.polygon([(CX + s * 130 - 22, CY - 60), (CX + s * 130 + 22, CY - 60), (CX + s * 130, CY + 40)], fill=AMBER)
        border(d, [(CX + s * 130 - 22, CY - 60), (CX + s * 130 + 22, CY - 60), (CX + s * 130, CY + 40)], 4)
    # трещины с янтарным свечением
    d.line([CX, CY + 80, CX - 120, CY + 180], fill=AMBER, width=6)
    d.line([CX, CY + 80, CX + 120, CY + 180], fill=AMBER, width=6)
    d.line([CX - 200, CY - 100, CX - 120, CY - 40], fill=AMBER, width=5)
    # пасть (тёмная щель)
    d.line([CX - 90, CY + 230, CX + 90, CY + 230], fill=DARK, width=8)
    neck(d, CY + 340, SKIN_BASALT, w=110, h=80)
    save(img, 'sil_race_brimstone')


# 2. F5 Терморедокс — ЛАВОВИК: каменная голова-обломок, лавовые прожилки, рога
def magmite():
    img, d = new_canvas()
    # голова (камень, широкая, тяжёлая)
    d.ellipse([CX - 260, CY - 300, CX + 260, CY + 280], fill=SKIN_MAGMA, outline=BORDER, width=6)
    # рога (короткие толстые)
    for s in (-1, 1):
        d.polygon([(CX + s * 150, CY - 260), (CX + s * 210, CY - 360), (CX + s * 230, CY - 220)], fill=(50, 40, 38))
        border(d, [(CX + s * 150, CY - 260), (CX + s * 210, CY - 360), (CX + s * 230, CY - 220)], 5)
    # лавовые щели-глаза (вертикальные)
    for s in (-1, 1):
        d.polygon([(CX + s * 120 - 20, CY - 70), (CX + s * 120 + 20, CY - 70), (CX + s * 120, CY + 30)], fill=LAVA)
        border(d, [(CX + s * 120 - 20, CY - 70), (CX + s * 120 + 20, CY - 70), (CX + s * 120, CY + 30)], 4)
    # лавовые прожилки
    d.line([CX, CY + 60, CX - 130, CY + 170], fill=LAVA, width=7)
    d.line([CX, CY + 60, CX + 130, CY + 170], fill=LAVA, width=7)
    d.line([CX - 70, CY - 120, CX - 180, CY - 60], fill=LAVA, width=6)
    d.line([CX + 70, CY - 120, CX + 180, CY - 60], fill=LAVA, width=6)
    # пасть (тёмная)
    d.ellipse([CX - 60, CY + 200, CX + 60, CY + 250], fill=DARK, outline=BORDER, width=5)
    neck(d, CY + 280, SKIN_MAGMA, w=110, h=80)
    save(img, 'sil_race_magmite')


# 3. F9 Экзотика — МАГНЕТАР: энергетический шар с магнитными дугами (без лица)
def magnetar():
    img, d = new_canvas()
    # энергетический шар
    d.ellipse([CX - 300, CY - 320, CX + 300, CY + 280], fill=SKIN_NEON, outline=BORDER, width=6)
    # магнитные дуги
    for s in (-1, 1):
        d.arc([CX + s * 140 - 300, CY - 430, CX + s * 140 + 300, CY - 80], start=15 if s > 0 else 195,
              end=165 if s > 0 else 345, fill=CYAN, width=12)
    # яркое ядро
    d.ellipse([CX - 110, CY - 120, CX + 110, CY + 100], fill=WHITE, outline=BORDER, width=4)
    # энергетические пятна
    d.ellipse([CX - 240, CY - 220, CX - 130, CY - 110], fill=PURPLE, outline=BORDER, width=4)
    d.ellipse([CX + 130, CY - 240, CX + 240, CY - 130], fill=CYAN, outline=BORDER, width=4)
    d.ellipse([CX - 230, CY + 140, CX - 110, CY + 240], fill=CYAN, outline=BORDER, width=4)
    save(img, 'sil_race_magnetar')


# 4. F1 Водные — ОКЕАНИДА: панцирная голова ракообразного, клешни-усы, жабры-пластины
def oceanid():
    img, d = new_canvas()
    # панцирь (хитин, широкий)
    d.ellipse([CX - 260, CY - 300, CX + 260, CY + 220], fill=SKIN_WATER, outline=BORDER, width=6)
    # жабры-пластины по бокам
    for s in (-1, 1):
        for i in range(4):
            x0 = CX + s * 220
            y0 = CY - 60 + i * 45
            d.arc([x0 - 60, y0 - 30, x0 + 60, y0 + 60], start=90 if s > 0 else 270, end=180 if s > 0 else 360,
                  fill=BRONZE, width=6)
    # глаза-бусинки на стебельках (маленькие, насекомоподобные)
    for s in (-1, 1):
        d.line([CX + s * 120, CY - 40, CX + s * 190, CY - 120], fill=SKIN_WATER, width=10)
        d.ellipse([CX + s * 190 - 30, CY - 160, CX + s * 190 + 30, CY - 100], fill=DARK, outline=BORDER, width=4)
        d.ellipse([CX + s * 190 - 12, CY - 140, CX + s * 190 + 12, CY - 116], fill=POISON)
    # клешни-антенны
    for s in (-1, 1):
        d.line([CX + s * 240, CY + 40, CX + s * 340, CY - 60], fill=SKIN_WATER, width=12)
        d.line([CX + s * 340, CY - 60, CX + s * 300, CY - 130], fill=SKIN_WATER, width=10)
    # рот-клюв (треугольник)
    d.polygon([(CX - 35, CY + 170), (CX + 35, CY + 170), (CX, CY + 215)], fill=DARK)
    border(d, [(CX - 35, CY + 170), (CX + 35, CY + 170), (CX, CY + 215)], 4)
    neck(d, CY + 220, SKIN_WATER, w=100, h=80)
    save(img, 'sil_race_oceanid')


# 5. F3 Метановые — ГРИБОВИК: грибная колония (шляпки + мицелий), без лица
def fungoid():
    img, d = new_canvas()
    # центральная шляпка (большой купол)
    d.pieslice([CX - 320, CY - 380, CX + 320, CY + 40], start=180, end=360, fill=(190, 160, 100))
    d.arc([CX - 320, CY - 380, CX + 320, CY + 40], start=180, end=360, fill=BORDER, width=6)
    # пятна на шляпке
    d.ellipse([CX - 220, CY - 300, CX - 120, CY - 230], fill=(230, 210, 160), outline=BORDER, width=4)
    d.ellipse([CX + 60, CY - 330, CX + 170, CY - 250], fill=(230, 210, 160), outline=BORDER, width=4)
    d.ellipse([CX - 50, CY - 360, CX + 40, CY - 290], fill=(230, 210, 160), outline=BORDER, width=4)
    # ножка (короткая)
    d.rectangle([CX - 130, CY - 20, CX + 130, CY + 120], fill=SKIN_FUNGI, outline=BORDER, width=6)
    # боковые шляпки (меньше)
    for s in (-1, 1):
        d.pieslice([CX + s * 300 - 160, CY - 200, CX + s * 300 + 160, CY + 60], start=180, end=360, fill=(200, 170, 110))
        d.arc([CX + s * 300 - 160, CY - 200, CX + s * 300 + 160, CY + 60], start=180, end=360, fill=BORDER, width=5)
        d.ellipse([CX + s * 300 - 80, CY - 160, CX + s * 300 + 10, CY - 100], fill=(230, 210, 160), outline=BORDER, width=4)
    # споры вокруг ножки
    for (dx, dy) in [(-150, 100), (150, 100), (0, 130), (-90, 140), (90, 140)]:
        d.ellipse([CX + dx - 20, CY + dy - 20, CX + dx + 20, CY + dy + 20], fill=(230, 210, 160), outline=BORDER, width=4)
    save(img, 'sil_race_fungoid')


# 6. F6 Кремниевые — КРИСТАЛЛИТ: друза кристаллов (чистая форма, без лица)
def geode():
    img, d = new_canvas()
    # центральный кристалл (высокий)
    d.polygon([(CX, CY - 420), (CX + 160, CY - 40), (CX, CY + 60), (CX - 160, CY - 40)], fill=SKIN_CRYSTAL)
    border(d, [(CX, CY - 420), (CX + 160, CY - 40), (CX, CY + 60), (CX - 160, CY - 40)], 6)
    d.polygon([(CX, CY - 420), (CX + 160, CY - 40), (CX + 120, CY + 30)], fill=PURPLE)
    border(d, [(CX, CY - 420), (CX + 160, CY - 40), (CX + 120, CY + 30)], 5)
    d.polygon([(CX, CY - 420), (CX - 160, CY - 40), (CX - 120, CY + 30)], fill=PURPLE)
    border(d, [(CX, CY - 420), (CX - 160, CY - 40), (CX - 120, CY + 30)], 5)
    # боковые кристаллы
    for s in (-1, 1):
        d.polygon([(CX + s * 230, CY - 140), (CX + s * 340, CY + 60), (CX + s * 250, CY + 150), (CX + s * 150, CY + 60)], fill=SKIN_CRYSTAL)
        border(d, [(CX + s * 230, CY - 140), (CX + s * 340, CY + 60), (CX + s * 250, CY + 150), (CX + s * 150, CY + 60)], 5)
        d.polygon([(CX + s * 230, CY - 140), (CX + s * 340, CY + 60), (CX + s * 280, CY + 90)], fill=PURPLE)
        border(d, [(CX + s * 230, CY - 140), (CX + s * 340, CY + 60), (CX + s * 280, CY + 90)], 4)
    # светящееся ядро
    d.ellipse([CX - 50, CY + 10, CX + 50, CY + 70], fill=GLASS, outline=BORDER, width=4)
    save(img, 'sil_race_geode')


# 7. F7 Небесные — ОБЛАЧНИК: облачный купол-вихрь, щупальца-струи, БЕЗ глаз
def nimbus():
    img, d = new_canvas()
    # купол (облако)
    d.ellipse([CX - 300, CY - 320, CX + 300, CY + 80], fill=SKIN_CLOUD, outline=BORDER, width=6)
    # облачные клочья
    d.ellipse([CX - 340, CY - 240, CX - 170, CY - 130], fill=WHITE, outline=BORDER, width=5)
    d.ellipse([CX + 170, CY - 280, CX + 340, CY - 140], fill=WHITE, outline=BORDER, width=5)
    d.ellipse([CX - 140, CY - 350, CX + 80, CY - 240], fill=WHITE, outline=BORDER, width=5)
    d.ellipse([CX - 240, CY + 30, CX - 60, CY + 120], fill=WHITE, outline=BORDER, width=5)
    d.ellipse([CX + 60, CY + 10, CX + 240, CY + 100], fill=WHITE, outline=BORDER, width=5)
    # вихревое ядро (спираль, без «глаз»)
    for i in range(4):
        r = 50 + i * 35
        d.ellipse([CX - r, CY - 60 - r, CX + r, CY - 60 + r], outline=CYAN, width=6)
    # щупальца-струи вниз
    for s in (-1, 1):
        for i in range(3):
            x0 = CX + s * (60 + i * 70)
            d.line([x0, CY + 60, x0, CY + 200 + i * 25], fill=SKIN_CLOUD, width=9)
            d.ellipse([x0 - 12, CY + 190 + i * 25, x0 + 12, CY + 214 + i * 25], fill=GLASS, outline=BORDER, width=3)
    save(img, 'sil_race_nimbus')


# 8. F8 Углекислые — ФУМАРОЛЬНИК: веерный коралл/фумарола (столб + веера), без лица
def fumarole():
    img, d = new_canvas()
    # центральный столб-труба
    d.rectangle([CX - 90, CY - 300, CX + 90, CY + 220], fill=SKIN_GREY, outline=BORDER, width=6)
    # веерные лепестки по бокам (5 с каждой стороны)
    for s in (-1, 1):
        for i, ang in enumerate([-45, -22, 0, 22, 45]):
            x0, y0 = CX + s * 90, CY - 60 + i * 30
            x1 = x0 + s * 190 * math.cos(math.radians(ang))
            y1 = y0 + 190 * math.sin(math.radians(ang)) * (0.5 if abs(ang) > 35 else 1.0)
            d.polygon([(x0, y0), (x1 - s * 18, y1 - 8), (x1 + s * 18, y1 + 8)], fill=WHITE)
            border(d, [(x0, y0), (x1 - s * 18, y1 - 8), (x1 + s * 18, y1 + 8)], 4)
    # выход фумаролы (тёмное жерло сверху)
    d.ellipse([CX - 45, CY - 320, CX + 45, CY - 260], fill=DARK, outline=BORDER, width=4)
    # кольца на столбе
    d.line([CX - 90, CY - 80, CX + 90, CY - 80], fill=BRONZE, width=6)
    d.line([CX - 85, CY + 60, CX + 85, CY + 60], fill=BRONZE, width=6)
    save(img, 'sil_race_fumarole')


# 9. F2 Крио-аммиачные — ЛЕДЯНОЙ СТРАННИК: ледяной сталагмит-кристалл (без лица)
def frostwalker():
    img, d = new_canvas()
    # основной сталагмит
    d.polygon([(CX - 150, CY + 200), (CX - 100, CY - 240), (CX, CY - 380), (CX + 100, CY - 240), (CX + 150, CY + 200)], fill=SKIN_FROST)
    border(d, [(CX - 150, CY + 200), (CX - 100, CY - 240), (CX, CY - 380), (CX + 100, CY - 240), (CX + 150, CY + 200)], 6)
    # боковые грани (тёмнее)
    d.polygon([(CX, CY - 380), (CX + 100, CY - 240), (CX + 90, CY + 60)], fill=GLASS)
    border(d, [(CX, CY - 380), (CX + 100, CY - 240), (CX + 90, CY + 60)], 5)
    d.polygon([(CX, CY - 380), (CX - 100, CY - 240), (CX - 90, CY + 60)], fill=GLASS)
    border(d, [(CX, CY - 380), (CX - 100, CY - 240), (CX - 90, CY + 60)], 5)
    # боковые шипы
    for s in (-1, 1):
        d.polygon([(CX + s * 140, CY - 60), (CX + s * 280, CY - 200), (CX + s * 220, CY - 30)], fill=SKIN_FROST)
        border(d, [(CX + s * 140, CY - 60), (CX + s * 280, CY - 200), (CX + s * 220, CY - 30)], 5)
    # светящееся ядро (внутри, вертикальное)
    d.ellipse([CX - 40, CY - 120, CX + 40, CY - 20], fill=GLASS, outline=BORDER, width=4)
    # основание-лёд
    d.ellipse([CX - 180, CY + 160, CX + 180, CY + 260], fill=SKIN_FROST, outline=BORDER, width=5)
    save(img, 'sil_race_frostwalker')


if __name__ == '__main__':
    brimstone()
    magmite()
    magnetar()
    oceanid()
    fungoid()
    geode()
    nimbus()
    fumarole()
    frostwalker()