# -*- coding: utf-8 -*-
# Силуэты кораблей по КОНЦЕПТАМ — пачка 2 (меч, зонт, снежинка, паук, бабочка, ключ, перо, комета, подкова, часы).
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


# 1. МЕЧ — клинок с гардой, эфесом, лезвие вперёд
def sword():
    img, d = new_canvas()
    # клинок (сталь, длинный, сужается к острию вправо)
    d.polygon([(CX - 300, CY - 20), (CX + 380, CY - 8), (CX + 480, CY), (CX + 380, CY + 8), (CX - 300, CY + 20)], fill=STEEL)
    # дол (середина клинка, светлее)
    d.line([CX - 260, CY, CX + 400, CY], fill=IVORY, width=6)
    # гарда (бронза, поперечная)
    d.rectangle([CX - 320, CY - 90, CX - 290, CY + 90], fill=BRONZE, outline=BORDER, width=5)
    # рукоять (крем)
    d.rectangle([CX - 420, CY - 25, CX - 320, CY + 25], fill=CREAM, outline=BORDER, width=5)
    # навершие (золото)
    d.ellipse([CX - 450, CY - 30, CX - 415, CY + 30], fill=GOLD, outline=BORDER, width=4)
    # дюзы на навершии (тёмные)
    d.rectangle([CX - 470, CY - 15, CX - 450, CY + 15], fill=DARK, outline=BORDER, width=3)
    save(img, 'sil_sword')


# 2. ЗОНТ — купол из сегментов, ручка-хвост
def umbrella():
    img, d = new_canvas()
    # купол (сегменты, чередование крем/сталь)
    for i in range(6):
        a1 = i * 60 - 90
        a2 = a1 + 60
        import math
        pts = [(CX, CY)]
        for ang in range(a1, a2 + 1, 5):
            r = 300
            pts.append((CX + r * math.cos(math.radians(ang)), CY + r * math.sin(math.radians(ang))))
        fill = CREAM if i % 2 == 0 else STEEL
        d.polygon(pts, fill=fill)
        d.line([CX, CY, pts[-1][0], pts[-1][1]], fill=BORDER, width=4)
    # верхушка (золото)
    d.ellipse([CX - 15, CY - 15, CX + 15, CY + 15], fill=GOLD, outline=BORDER, width=3)
    # ручка (изогнутая, бронза) — хвост влево
    d.arc([CX - 420, CY - 100, CX - 300, CY + 100], start=60, end=300, fill=BRONZE, width=18)
    d.rectangle([CX - 430, CY - 15, CX - 300, CY + 15], fill=BRONZE, outline=BORDER, width=4)
    # дюзы на ручке
    d.rectangle([CX - 445, CY - 10, CX - 430, CY + 10], fill=DARK, outline=BORDER, width=3)
    save(img, 'sil_umbrella')


# 3. СНЕЖИНКА — 6 лучей, СПЛЮЩЕННАЯ (вытянута по горизонтали, низкая)
def snowflake():
    img, d = new_canvas()
    import math
    FLAT_Y = 0.55  # сплющивание по вертикали
    # 6 лучей (сталь), вертикальная компонента уменьшена
    for i in range(6):
        ang = math.radians(i * 60)
        dx, dy = math.cos(ang), math.sin(ang) * FLAT_Y
        norm = math.hypot(dx, dy)
        dx, dy = dx / norm, dy / norm
        d.line([CX, CY, CX + dx * 380, CY + dy * 380], fill=STEEL, width=20)
        for branch in [0.55, 0.75]:
            bx, by = CX + dx * 380 * branch, CY + dy * 380 * branch
            d.line([bx, by, bx - dy * 90, by + dx * 90], fill=STEEL, width=12)
            d.line([bx, by, bx + dy * 90, by - dx * 90], fill=STEEL, width=12)
    # центр (золото)
    d.ellipse([CX - 50, CY - 35, CX + 50, CY + 35], fill=GOLD, outline=BORDER, width=5)
    # сердцевина (стекло)
    d.ellipse([CX - 25, CY - 18, CX + 25, CY + 18], fill=GLASS, outline=BORDER, width=3)
    save(img, 'sil_snowflake')


# 4. ПАУК — тело с ногами, голова-нос
def spider():
    img, d = new_canvas()
    # брюшко (овал, крем) — слева
    d.ellipse([CX - 300, CY - 160, CX + 50, CY + 160], fill=CREAM, outline=BORDER, width=6)
    # головогрудь (сталь) — справа
    d.ellipse([CX + 50, CY - 90, CX + 220, CY + 90], fill=STEEL, outline=BORDER, width=6)
    # глаза (зелёные, на головогруди)
    d.ellipse([CX + 170, CY - 50, CX + 200, CY - 20], fill=POISON, outline=BORDER, width=3)
    d.ellipse([CX + 170, CY + 20, CX + 200, CY + 50], fill=POISON, outline=BORDER, width=3)
    # 8 ног (бронза, по 4 с каждой стороны)
    for side in [-1, 1]:
        for i in range(4):
            x0 = CX - 100 + i * 40
            y0 = CY + side * 60
            x1 = x0 - 180
            y1 = CY + side * (60 + 90 + i * 40)
            d.line([x0, y0, x1, y1], fill=BRONZE, width=16)
            d.line([x1, y1, x1 - 40, y1 + side * 25], fill=BRONZE, width=12)
    # дюзы на брюшке (тёмные)
    d.rectangle([CX - 330, CY - 30, CX - 300, CY + 30], fill=DARK, outline=BORDER, width=4)
    save(img, 'sil_spider')


# 5. БАБОЧКА — два верхних и два нижних крыла, тело
def butterfly():
    img, d = new_canvas()
    # верхние крылья (большие, сталь/крем)
    d.polygon([(CX - 60, CY - 30), (CX - 260, CY - 220), (CX - 120, CY - 280), (CX + 60, CY - 40)], fill=STEEL)
    border(d, [(CX - 60, CY - 30), (CX - 260, CY - 220), (CX - 120, CY - 280), (CX + 60, CY - 40)])
    d.polygon([(CX - 60, CY + 30), (CX - 260, CY + 220), (CX - 120, CY + 280), (CX + 60, CY + 40)], fill=STEEL)
    border(d, [(CX - 60, CY + 30), (CX - 260, CY + 220), (CX - 120, CY + 280), (CX + 60, CY + 40)])
    # нижние крылья (меньше, крем)
    d.polygon([(CX + 20, CY - 30), (CX - 120, CY - 200), (CX - 20, CY - 230), (CX + 80, CY - 40)], fill=CREAM)
    border(d, [(CX + 20, CY - 30), (CX - 120, CY - 200), (CX - 20, CY - 230), (CX + 80, CY - 40)])
    d.polygon([(CX + 20, CY + 30), (CX - 120, CY + 200), (CX - 20, CY + 230), (CX + 80, CY + 40)], fill=CREAM)
    border(d, [(CX + 20, CY + 30), (CX - 120, CY + 200), (CX - 20, CY + 230), (CX + 80, CY + 40)])
    # глазки на крыльях (пурпур)
    d.ellipse([CX - 200, CY - 150, CX - 170, CY - 120], fill=PURPLE, outline=BORDER, width=3)
    d.ellipse([CX - 200, CY + 120, CX - 170, CY + 150], fill=PURPLE, outline=BORDER, width=3)
    # тело (тёмное, вытянутое)
    d.ellipse([CX - 120, CY - 30, CX + 300, CY + 30], fill=DARK, outline=BORDER, width=5)
    # голова (круг)
    d.ellipse([CX + 280, CY - 40, CX + 360, CY + 40], fill=DARK, outline=BORDER, width=5)
    # усики (бронза)
    d.line([CX + 320, CY - 30, CX + 360, CY - 90], fill=BRONZE, width=8)
    d.line([CX + 320, CY + 30, CX + 360, CY + 90], fill=BRONZE, width=8)
    # дюзы на голове
    d.rectangle([CX + 360, CY - 15, CX + 385, CY + 15], fill=FIRE, outline=BORDER, width=3)
    save(img, 'sil_butterfly')


# 6. КЛЮЧ — бородка, кольцо-хвост
def key():
    img, d = new_canvas()
    # стержень (крем, длинный)
    d.rectangle([CX - 380, CY - 25, CX + 300, CY + 25], fill=CREAM, outline=BORDER, width=5)
    # бородка (сталь, справа — зубцы)
    d.rectangle([CX + 260, CY - 110, CX + 300, CY + 110], fill=STEEL, outline=BORDER, width=5)
    for ty in [-100, -60, -20, 20, 60, 100]:
        d.rectangle([CX + 300, CY + ty, CX + 360, CY + ty + 35], fill=STEEL, outline=BORDER, width=4)
    # кольцо (золото, слева)
    d.ellipse([CX - 460, CY - 80, CX - 360, CY + 80], fill=GOLD, outline=BORDER, width=20)
    # дюзы на кольце
    d.rectangle([CX - 470, CY - 15, CX - 455, CY + 15], fill=DARK, outline=BORDER, width=3)
    save(img, 'sil_key')


# 7. ПЕРО — овальное перо с бороздкой, стержень-хвост
def feather():
    img, d = new_canvas()
    # опахало (овал, крем/иври)
    d.polygon([(CX - 260, CY - 140), (CX - 100, CY - 180), (CX + 150, CY - 100), (CX + 250, CY - 20), (CX + 250, CY + 20), (CX + 150, CY + 100), (CX - 100, CY + 180), (CX - 260, CY + 140)], fill=IVORY)
    # бороздка (середина, бронза)
    d.polygon([(CX - 260, CY - 20), (CX + 250, CY - 8), (CX + 250, CY + 8), (CX - 260, CY + 20)], fill=BRONZE)
    # секции опахала (сталь, диагонали)
    for i in range(1, 6):
        x = CX - 200 + i * 70
        d.line([x, CY - 130, x + 40, CY + 100], fill=STEEL, width=6)
    # стержень (крем, влево)
    d.rectangle([CX - 460, CY - 15, CX - 260, CY + 15], fill=CREAM, outline=BORDER, width=5)
    # дюзы на конце стержня
    d.rectangle([CX - 475, CY - 10, CX - 460, CY + 10], fill=DARK, outline=BORDER, width=3)
    save(img, 'sil_feather')


# 8. КОМЕТА — ядро + длинный хвост со свечением
def comet():
    img, d = new_canvas()
    # ядро (круг, сталь)
    d.ellipse([CX + 150, CY - 90, CX + 330, CY + 90], fill=STEEL, outline=BORDER, width=6)
    # кратер-кабина (стекло)
    d.ellipse([CX + 190, CY - 40, CX + 260, CY + 40], fill=GLASS, outline=BORDER, width=4)
    # хвост (разворачивающийся влево, крем→прозрачный)
    for i, w in enumerate([60, 90, 130, 170, 210, 250]):
        x0 = CX + 100 - i * 70
        x1 = x0 - 90
        y0 = CY - w
        y1 = CY + w
        fill = CREAM if i < 3 else IVORY
        d.polygon([(x0, CY - w // 2), (x1, y0), (x1, y1), (x0, CY + w // 2)], fill=fill)
    # дюзы (огонь у ядра)
    d.rectangle([CX + 100, CY - 20, CX + 130, CY + 20], fill=FIRE, outline=BORDER, width=3)
    save(img, 'sil_comet')


# 9. ПОДКОВА — дуга, выпуклость ВПРАВО (нос), открыта влево (корма), гвозди
def horseshoe():
    img, d = new_canvas()
    # подкова (дуга, бронза) — выпуклость вправо, концы влево
    d.arc([CX - 380, CY - 330, CX + 330, CY + 330], start=240, end=120, fill=BRONZE, width=60)
    # концы подковы (сталь, слева — корма)
    d.ellipse([CX - 330, CY - 275, CX - 260, CY - 205], fill=STEEL, outline=BORDER, width=5)
    d.ellipse([CX - 330, CY + 205, CX - 260, CY + 275], fill=STEEL, outline=BORDER, width=5)
    # гвозди (золото, по дуге)
    import math
    for ang in range(240, 121, -20):
        r = 330
        gx = CX - 25 + r * math.cos(math.radians(ang))
        gy = CY + r * math.sin(math.radians(ang))
        d.ellipse([gx - 18, gy - 18, gx + 18, gy + 18], fill=GOLD, outline=BORDER, width=3)
    # центральная заплатка (стекло)
    d.ellipse([CX - 30, CY - 60, CX + 90, CY + 60], fill=GLASS, outline=BORDER, width=5)
    # дюзы на концах (тёмные, слева)
    d.rectangle([CX - 350, CY - 240, CX - 330, CY - 220], fill=DARK, outline=BORDER, width=3)
    d.rectangle([CX - 350, CY + 220, CX - 330, CY + 240], fill=DARK, outline=BORDER, width=3)
    save(img, 'sil_horseshoe')


# 10. ЧАСЫ — корпус-циферблат с стрелками
def clock():
    img, d = new_canvas()
    # корпус (круг, золото)
    d.ellipse([CX - 260, CY - 260, CX + 260, CY + 260], fill=GOLD, outline=BORDER, width=8)
    # циферблат (крем)
    d.ellipse([CX - 220, CY - 220, CX + 220, CY + 220], fill=CREAM, outline=BORDER, width=5)
    # деления (бронза)
    import math
    for i in range(12):
        ang = math.radians(i * 30)
        x1 = CX + 200 * math.cos(ang); y1 = CY + 200 * math.sin(ang)
        x2 = CX + 180 * math.cos(ang); y2 = CY + 180 * math.sin(ang)
        d.line([x1, y1, x2, y2], fill=BRONZE, width=10)
    # стрелки (сталь) — часовая вправо (нос), минутная вверх
    d.polygon([(CX - 15, CY - 20), (CX + 160, CY - 6), (CX + 160, CY + 6), (CX - 15, CY + 20)], fill=STEEL)
    d.polygon([(CX - 10, CY - 30), (CX + 10, CY - 170), (CX + 20, CY - 10)], fill=STEEL)
    # центр (тёмный)
    d.ellipse([CX - 20, CY - 20, CX + 20, CY + 20], fill=DARK, outline=BORDER, width=4)
    # дюзы (огонь, снизу)
    d.rectangle([CX - 30, CY + 250, CX + 30, CY + 280], fill=FIRE, outline=BORDER, width=4)
    save(img, 'sil_clock')


if __name__ == '__main__':
    sword()
    umbrella()
    snowflake()
    spider()
    butterfly()
    key()
    feather()
    comet()
    horseshoe()
    clock()