# -*- coding: utf-8 -*-
# Силуэты АВАТАРОВ РАС — пачка 3: ТВАРИ по материалу (стиль пачки 1).
# Кристаллит (геоде): cryst_beast_a (горгулья-грань), cryst_beast_b (череп-грань).
# Фумарольник (фумароль): fum_beast_a (веерная жаба), fum_beast_b (трубоголов).
# 1024x1024, анфас, голова + короткая шея, цветные модули, чёрный фон.
import os, math
from PIL import Image, ImageDraw

OUT = r'C:\Zorion2\ai_drafts\silhouettes_races'
SIZE = 1024
CX, CY = SIZE // 2, SIZE // 2

BORDER = (12, 12, 12)
SKIN_CRYSTAL = (232, 224, 186)
SKIN_GREY = (150, 155, 160)
PURPLE = (140, 90, 180)
GLASS = (180, 210, 235)
DARK = (40, 44, 55)
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


def neck(d, top_y, skin, w=110, h=80):
    d.polygon([(CX - w, top_y), (CX + w, top_y), (CX + w - 30, top_y + h), (CX - w + 30, top_y + h)], fill=skin)
    border(d, [(CX - w, top_y), (CX + w, top_y), (CX + w - 30, top_y + h), (CX - w + 30, top_y + h)], 5)


# 1. КРИСТАЛЛИТ A — кристальная ГОРГУЛЬЯ: гранёная голова, рога-кристаллы, глаза-ромбы, пасть с зубцами
def cryst_beast_a():
    img, d = new_canvas()
    # голова-октаэдр (широкая)
    d.polygon([(CX, CY - 340), (CX + 250, CY - 40), (CX, CY + 240), (CX - 250, CY - 40)], fill=SKIN_CRYSTAL)
    border(d, [(CX, CY - 340), (CX + 250, CY - 40), (CX, CY + 240), (CX - 250, CY - 40)], 6)
    # боковые грани (фиолет)
    d.polygon([(CX, CY - 340), (CX + 250, CY - 40), (CX + 170, CY + 60)], fill=PURPLE)
    border(d, [(CX, CY - 340), (CX + 250, CY - 40), (CX + 170, CY + 60)], 5)
    d.polygon([(CX, CY - 340), (CX - 250, CY - 40), (CX - 170, CY + 60)], fill=PURPLE)
    border(d, [(CX, CY - 340), (CX - 250, CY - 40), (CX - 170, CY + 60)], 5)
    # рога-кристаллы (наклонные)
    for s in (-1, 1):
        d.polygon([(CX + s * 150, CY - 260), (CX + s * 250, CY - 400), (CX + s * 260, CY - 220)], fill=SKIN_CRYSTAL)
        border(d, [(CX + s * 150, CY - 260), (CX + s * 250, CY - 400), (CX + s * 260, CY - 220)], 5)
        d.polygon([(CX + s * 150, CY - 260), (CX + s * 250, CY - 400), (CX + s * 220, CY - 300)], fill=PURPLE)
        border(d, [(CX + s * 150, CY - 260), (CX + s * 250, CY - 400), (CX + s * 220, CY - 300)], 4)
    # глаза-ромбы (светящиеся)
    for s in (-1, 1):
        d.polygon([(CX + s * 110, CY - 100), (CX + s * 160, CY - 20), (CX + s * 110, CY + 60), (CX + s * 60, CY - 20)], fill=GLASS)
        border(d, [(CX + s * 110, CY - 100), (CX + s * 160, CY - 20), (CX + s * 110, CY + 60), (CX + s * 60, CY - 20)], 4)
    # пасть с зубцами-кристаллами
    d.polygon([(CX - 110, CY + 150), (CX + 110, CY + 150), (CX + 70, CY + 220), (CX - 70, CY + 220)], fill=DARK)
    border(d, [(CX - 110, CY + 150), (CX + 110, CY + 150), (CX + 70, CY + 220), (CX - 70, CY + 220)], 5)
    for s in (-1, 1):
        for i, dx in enumerate([-70, -20, 30]):
            d.polygon([(CX + s * dx, CY + 150), (CX + s * dx + 18, CY + 150), (CX + s * dx + 9, CY + 100)], fill=SKIN_CRYSTAL)
            border(d, [(CX + s * dx, CY + 150), (CX + s * dx + 18, CY + 150), (CX + s * dx + 9, CY + 100)], 3)
    # ядро на лбу
    d.ellipse([CX - 35, CY - 200, CX + 35, CY - 130], fill=GLASS, outline=BORDER, width=4)
    neck(d, CY + 240, SKIN_CRYSTAL)
    save(img, 'sil_race_cryst_beast_a')


# 2. КРИСТАЛЛИТ B — кристальный ЧЕРЕП: гранёная округлая голова, глазницы-ромбы, гребень
def cryst_beast_b():
    img, d = new_canvas()
    # череп (гранёный, округлый)
    d.ellipse([CX - 250, CY - 320, CX + 250, CY + 250], fill=SKIN_CRYSTAL, outline=BORDER, width=6)
    # грани на черепе
    d.polygon([(CX - 250, CY - 100), (CX, CY - 300), (CX + 250, CY - 100)], fill=PURPLE)
    border(d, [(CX - 250, CY - 100), (CX, CY - 300), (CX + 250, CY - 100)], 5)
    d.polygon([(CX - 250, CY + 100), (CX, CY - 80), (CX + 250, CY + 100)], fill=PURPLE)
    border(d, [(CX - 250, CY + 100), (CX, CY - 80), (CX + 250, CY + 100)], 5)
    # гребень-кристалл сверху
    for s in (-1, 1):
        d.polygon([(CX + s * 60, CY - 290), (CX + s * 110, CY - 400), (CX + s * 140, CY - 260)], fill=SKIN_CRYSTAL)
        border(d, [(CX + s * 60, CY - 290), (CX + s * 110, CY - 400), (CX + s * 140, CY - 260)], 5)
    # глазницы-ромбы (светящиеся, глубокие)
    for s in (-1, 1):
        d.polygon([(CX + s * 120 - 55, CY - 60), (CX + s * 120, CY - 10), (CX + s * 120 + 55, CY - 60), (CX + s * 120, CY - 110)], fill=DARK)
        border(d, [(CX + s * 120 - 55, CY - 60), (CX + s * 120, CY - 10), (CX + s * 120 + 55, CY - 60), (CX + s * 120, CY - 110)], 5)
        d.ellipse([CX + s * 120 - 20, CY - 75, CX + s * 120 + 20, CY - 35], fill=GLASS)
    # нос-грани
    d.polygon([(CX, CY - 20), (CX + 30, CY + 60), (CX, CY + 90), (CX - 30, CY + 60)], fill=SKIN_CRYSTAL)
    border(d, [(CX, CY - 20), (CX + 30, CY + 60), (CX, CY + 90), (CX - 30, CY + 60)], 4)
    # зубы-кристаллы (пасть)
    d.polygon([(CX - 100, CY + 140), (CX + 100, CY + 140), (CX + 60, CY + 210), (CX - 60, CY + 210)], fill=DARK)
    border(d, [(CX - 100, CY + 140), (CX + 100, CY + 140), (CX + 60, CY + 210), (CX - 60, CY + 210)], 5)
    for s in (-1, 1):
        d.polygon([(CX + s * 55, CY + 140), (CX + s * 85, CY + 140), (CX + s * 70, CY + 95)], fill=WHITE)
        border(d, [(CX + s * 55, CY + 140), (CX + s * 85, CY + 140), (CX + s * 70, CY + 95)], 3)
    neck(d, CY + 250, SKIN_CRYSTAL)
    save(img, 'sil_race_cryst_beast_b')


# 3. ФУМАРОЛЬНИК A — веерная ЖАБА: широкая плоская голова, веерные жабры-воротник, выпученные глаза
def fum_beast_a():
    img, d = new_canvas()
    # широкая голова-жаба
    d.ellipse([CX - 290, CY - 240, CX + 290, CY + 240], fill=SKIN_GREY, outline=BORDER, width=6)
    # веерный воротник (жабры) по бокам
    for s in (-1, 1):
        for i, ang in enumerate([-50, -30, -10, 10, 30, 50]):
            x0, y0 = CX + s * 280, CY - 60 + i * 25
            x1 = x0 + s * 170 * math.cos(math.radians(ang))
            y1 = y0 + 170 * math.sin(math.radians(ang)) * (0.5 if abs(ang) > 40 else 1.0)
            d.polygon([(x0, y0), (x1 - s * 16, y1 - 8), (x1 + s * 16, y1 + 8)], fill=WHITE)
            border(d, [(x0, y0), (x1 - s * 16, y1 - 8), (x1 + s * 16, y1 + 8)], 4)
    # выпученные глаза (на макушке)
    for s in (-1, 1):
        d.ellipse([CX + s * 120 - 45, CY - 230, CX + s * 120 + 45, CY - 150], fill=WHITE, outline=BORDER, width=5)
        d.ellipse([CX + s * 120 - 25, CY - 210, CX + s * 120 + 25, CY - 170], fill=DARK)
        d.ellipse([CX + s * 120 - 10, CY - 198, CX + s * 120 + 10, CY - 182], fill=GLASS)
    # широкая пасть-трещина
    d.polygon([(CX - 140, CY + 80), (CX + 140, CY + 80), (CX + 90, CY + 190), (CX - 90, CY + 190)], fill=DARK)
    border(d, [(CX - 140, CY + 80), (CX + 140, CY + 80), (CX + 90, CY + 190), (CX - 90, CY + 190)], 5)
    # борозды-жабры на голове
    for s in (-1, 1):
        d.line([CX + s * 70, CY - 40, CX + s * 170, CY - 10], fill=BRONZE, width=6)
    neck(d, CY + 240, SKIN_GREY, w=120, h=80)
    save(img, 'sil_race_fum_beast_a')


# 4. ФУМАРОЛЬНИК B — ТРУБОГОЛОВ: голова-монстр с трубчатыми жерлами-глазами и пастью-жерлом
def fum_beast_b():
    img, d = new_canvas()
    # голова (каменная, широкая)
    d.ellipse([CX - 250, CY - 300, CX + 250, CY + 250], fill=SKIN_GREY, outline=BORDER, width=6)
    # глаза-трубы (выступающие жерла)
    for s in (-1, 1):
        d.rectangle([CX + s * 100 - 30, CY - 160, CX + s * 100 + 30, CY - 40], fill=SKIN_GREY, outline=BORDER, width=5)
        d.ellipse([CX + s * 100 - 30, CY - 185, CX + s * 100 + 30, CY - 135], fill=DARK, outline=BORDER, width=4)
    # пасть-жерло (круглое тёмное)
    d.ellipse([CX - 85, CY + 80, CX + 85, CY + 210], fill=DARK, outline=BORDER, width=6)
    # кольца на голове (как труба)
    d.line([CX - 200, CY - 100, CX + 200, CY - 100], fill=BRONZE, width=6)
    d.line([CX - 220, CY + 40, CX + 220, CY + 40], fill=BRONZE, width=6)
    # дым из пасти
    for (dx, dy, r) in [(-40, 260, 45), (30, 300, 55), (-10, 350, 40)]:
        d.ellipse([CX + dx - r, CY + dy - r, CX + dx + r, CY + dy + r], fill=SMOKE, outline=BORDER, width=4)
    # боковые трубы-выросты
    for s in (-1, 1):
        d.rectangle([CX + s * 250 - 20, CY - 60, CX + s * 250 + 20, CY + 60], fill=SKIN_GREY, outline=BORDER, width=4)
        d.ellipse([CX + s * 250 - 20, CY - 85, CX + s * 250 + 20, CY - 35], fill=DARK, outline=BORDER, width=3)
    neck(d, CY + 250, SKIN_GREY, w=110, h=80)
    save(img, 'sil_race_fum_beast_b')


if __name__ == '__main__':
    cryst_beast_a()
    cryst_beast_b()
    fum_beast_a()
    fum_beast_b()