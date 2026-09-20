# -*- coding: utf-8 -*-
# Силуэты ИКОНОК биомов поверхности (стиль А — плоский значок-символ; рекомендован).
# + 4 стиль-теста (леса/метановые моря в стилях B мини-пейзаж и C геральдика).
# Цветные модули на чёрном фоне, 1024x1024, композиция по центру, без ориентации.
import os
from PIL import Image, ImageDraw, ImageFilter

OUT = r'C:\Zorion2\ai_drafts\biomes\silhouettes'
SIZE = 1024
CX, CY = SIZE // 2, SIZE // 2
BORDER = (12, 12, 12)

GREEN = (88, 160, 90)
GREEN_D = (50, 110, 60)
GRASS = (120, 170, 80)
BLUE = (70, 130, 200)
BLUE_L = (140, 190, 235)
TEAL = (60, 180, 180)
CYAN = (140, 230, 230)
PURPLE = (150, 100, 200)
MAGENTA = (210, 100, 170)
PINK = (230, 140, 170)
CORAL = (230, 120, 120)
RED = (200, 60, 50)
CRIMSON = (170, 40, 60)
ORANGE = (230, 120, 40)
FIRE = (240, 150, 60)
YELLOW = (220, 190, 70)
SULFUR = (200, 190, 90)
WHITE = (235, 235, 235)
ICE = (190, 225, 240)
STEEL = (120, 140, 170)
STEEL_D = (80, 95, 120)
BROWN = (130, 95, 60)
TAR = (70, 55, 45)
DARK = (40, 44, 55)
GLASS = (180, 210, 235)
GOLD = (210, 170, 60)


def new_canvas():
    img = Image.new('RGB', (SIZE, SIZE), (5, 5, 5))
    return img, ImageDraw.Draw(img)


def save(img, name):
    os.makedirs(OUT, exist_ok=True)
    img.save(os.path.join(OUT, name + '.png'))
    print('Saved:', name)


def poly(d, pts, fill, w=5):
    d.polygon(pts, fill=fill)
    d.line(pts + [pts[0]], fill=BORDER, width=w)


def tree(d, cx, cy, s, crown, trunk=DARK):
    d.polygon([(cx, cy - s), (cx - s * 0.72, cy), (cx + s * 0.72, cy)], fill=crown)
    d.line([(cx, cy - s * 0.4), (cx, cy + s * 0.3)], fill=trunk, width=max(8, int(s * 0.1)))


def tuft(d, x, y, h, color, w=10):
    for dx in (-1, 0, 1):
        d.line([(x + dx * 8, y), (x + dx * 18, y - h)], fill=color, width=w)


# ============================ СТИЛЬ А — плоский значок-символ ============================

# 1. Леса
def sil_lesa():
    img, d = new_canvas()
    d.rectangle([CX - 340, CY + 210, CX + 340, CY + 250], fill=GREEN_D)
    d.line([(CX - 340, CY + 210), (CX + 340, CY + 210)], fill=BORDER, width=5)
    tree(d, CX - 220, CY + 210, 180, GREEN)
    tree(d, CX, CY + 210, 250, GREEN_D)
    tree(d, CX + 230, CY + 210, 160, GRASS)
    tree(d, CX - 80, CY + 210, 120, GRASS)
    save(img, 'sil_lesa')


# 2. Луга и степи
def sil_luga_steppi():
    img, d = new_canvas()
    d.pieslice([CX - 460, CY - 320, CX - 60, CY + 360], 180, 360, fill=GREEN)
    d.pieslice([CX + 40, CY - 380, CX + 460, CY + 300], 180, 360, fill=GRASS)
    d.line([(CX - 460, CY + 20), (CX - 60, CY + 20)], fill=BORDER, width=5)
    tuft(d, CX - 260, CY + 20, 90, GREEN_D)
    tuft(d, CX - 120, CY + 20, 120, GREEN_D)
    tuft(d, CX + 60, CY - 60, 100, GREEN)
    tuft(d, CX + 200, CY - 60, 130, GREEN)
    for fx, fy, fr in [(-40, CY - 130, 18), (140, CY - 170, 14), (300, CY - 120, 16)]:
        d.ellipse([fx - fr, fy - fr, fx + fr, fy + fr], fill=YELLOW, outline=BORDER, width=4)
    d.ellipse([CX + 180, CY - 340, CX + 280, CY - 240], fill=GOLD, outline=BORDER, width=5)
    save(img, 'sil_luga_steppi')


# 3. Океаны
def sil_okeany():
    img, d = new_canvas()
    for i, (y, c) in enumerate([(CY - 180, BLUE_L), (CY - 60, BLUE), (CY + 60, BLUE), (CY + 180, BLUE_L)]):
        d.polygon([(CX - 400, y + 40), (CX - 400, y - 40), (CX - 200, y - 40),
                   (CX - 120, y + 40), (CX + 60, y - 40), (CX + 180, y + 40),
                   (CX + 360, y - 40), (CX + 400, y - 40), (CX + 400, y + 40)], fill=c)
        d.line([(CX - 400, y + 40), (CX - 400, y - 40), (CX - 200, y - 40),
                (CX - 120, y + 40), (CX + 60, y - 40), (CX + 180, y + 40),
                (CX + 360, y - 40), (CX + 400, y - 40), (CX + 400, y + 40),
                (CX + 400, y + 40)], fill=BORDER, width=5)
        d.line([(CX - 400, y + 40), (CX - 400, y - 40)], fill=BORDER, width=5)
        d.line([(CX + 400, y + 40), (CX + 400, y - 40)], fill=BORDER, width=5)
        d.line([(CX - 200, y - 40), (CX - 120, y + 40)], fill=BORDER, width=5)
        d.line([(CX + 60, y - 40), (CX + 180, y + 40)], fill=BORDER, width=5)
    d.ellipse([CX - 160, CY + 300, CX - 90, CY + 370], fill=WHITE, outline=BORDER, width=4)
    save(img, 'sil_okeany')


# 4. Болота
def sil_bolota():
    img, d = new_canvas()
    d.ellipse([CX - 380, CY - 150, CX + 380, CY + 150], fill=(70, 120, 90))
    d.line([(CX - 380, CY - 150), (CX + 380, CY - 150)], fill=BORDER, width=5)
    d.line([(CX - 380, CY + 150), (CX + 380, CY + 150)], fill=BORDER, width=5)
    d.ellipse([CX - 240, CY - 90, CX - 140, CY + 10], fill=GREEN_D, outline=BORDER, width=4)
    d.ellipse([CX + 40, CY - 60, CX + 160, CY + 60], fill=(50, 100, 75), outline=BORDER, width=4)
    for rx, h in [(-200, 240), (-120, 320), (60, 270), (150, 210), (230, 300)]:
        d.line([(CX + rx, CY + 120), (CX + rx, CY - h)], fill=GREEN_D, width=12)
        d.ellipse([CX + rx - 26, CY - h - 70, CX + rx + 26, CY - h + 10], fill=BROWN, outline=BORDER, width=4)
    save(img, 'sil_bolota')


# 5. Коралловые рифы
def sil_korallovye_rifы():
    img, d = new_canvas()
    d.polygon([(CX - 300, CY + 220), (CX + 300, CY + 220), (CX + 200, CY + 120), (CX - 200, CY + 120)], fill=STEEL_D)
    d.line([(CX - 300, CY + 220), (CX + 300, CY + 220)], fill=BORDER, width=5)
    # ветви кораллов
    for bx, c in [(-220, PINK), (-90, PURPLE), (60, CORAL), (180, MAGENTA)]:
        d.line([(bx, CY + 120), (bx - 40, CY - 60), (bx - 90, CY - 160)], fill=c, width=34)
        d.line([(bx, CY + 120), (bx + 30, CY - 40), (bx + 70, CY - 130)], fill=c, width=26)
        d.line([(bx, CY + 120), (bx + 5, CY - 90)], fill=c, width=30)
    # веер
    d.pieslice([CX - 40, CY - 180, CX + 120, CY + 120], 200, 340, fill=TEAL)
    d.line([(CX - 40, CY + 120), (CX - 40, CY - 80)], fill=BORDER, width=5)
    for a in (220, 250, 280, 310):
        import math
        x0 = CX + 40 * math.cos(math.radians(a))
        y0 = CY - 30 + 40 * math.sin(math.radians(a))
        x1 = CX + 160 * math.cos(math.radians(a))
        y1 = CY - 30 + 160 * math.sin(math.radians(a))
        d.line([(x0, y0), (x1, y1)], fill=BORDER, width=5)
    d.ellipse([-180 + CX, CY - 100, -40 + CX, CY + 60], fill=BLUE_L, outline=BORDER, width=4)
    save(img, 'sil_korallovye_rifы')


# 6. Светящиеся чащи
def sil_svetyashchiesya_chashchi():
    img, d = new_canvas()
    d.rectangle([CX - 340, CY + 200, CX + 340, CY + 240], fill=DARK)
    tree(d, CX - 200, CY + 200, 200, GREEN_D)
    tree(d, CX + 60, CY + 200, 260, GREEN)
    tree(d, CX + 260, CY + 200, 170, GREEN_D)
    for gx, gy in [(-200, CY - 80), (-130, CY + 10), (60, CY - 150), (140, CY - 40), (250, CY - 70), (30, CY - 60)]:
        d.ellipse([gx - 22, gy - 22, gx + 22, gy + 22], fill=CYAN, outline=BORDER, width=4)
    for gx, gy in [(-60, CY - 120), (190, CY - 20)]:
        d.ellipse([gx - 14, gy - 14, gx + 14, gy + 14], fill=WHITE, outline=BORDER, width=3)
    save(img, 'sil_svetyashchiesya_chashchi')


# 7. Багровые степи
def sil_bagrovye_steppi():
    img, d = new_canvas()
    d.rectangle([CX - 380, CY + 230, CX + 380, CY + 270], fill=CRIMSON)
    tuft(d, CX - 260, CY + 230, 140, CRIMSON)
    tuft(d, CX - 140, CY + 230, 190, RED)
    tuft(d, CX + 20, CY + 230, 160, CRIMSON)
    tuft(d, CX + 160, CY + 230, 210, RED)
    tuft(d, CX + 290, CY + 230, 130, CRIMSON)
    # изгибы ветра
    for wy, wl in [(CY + 80, 260), (CY + 40, 340), (CY + 0, 200)]:
        d.line([(CX - wl, wy), (CX + wl, wy - 40)], fill=RED, width=16)
    save(img, 'sil_bagrovye_steppi')


# 8. Кислотные дебри
def sil_kislotnye_debri():
    img, d = new_canvas()
    d.ellipse([CX - 300, CY + 120, CX + 300, CY + 280], fill=(110, 190, 90))
    d.line([(CX - 300, CY + 120), (CX + 300, CY + 120)], fill=BORDER, width=5)
    # шипы-растения
    for px, h, c in [(-240, 260, (140, 220, 90)), (-140, 330, (110, 200, 160)), (-40, 240, (140, 220, 90)),
                     (80, 310, (110, 200, 160)), (200, 270, (140, 220, 90))]:
        d.polygon([(px - 20, CY + 120), (px + 20, CY + 120), (px, CY + 120 - h)], fill=c)
        d.line([(px - 20, CY + 120), (px + 20, CY + 120), (px, CY + 120 - h), (px - 20, CY + 120)], fill=BORDER, width=5)
    # капли кислоты
    for bx, by in [(-180, CY + 40), (10, CY + 70), (150, CY + 20)]:
        d.ellipse([bx - 22, by - 22, bx + 22, by + 22], fill=(160, 240, 140), outline=BORDER, width=4)
    save(img, 'sil_kislotnye_debri')


# 9. Серные поля
def sil_sernye_polya():
    img, d = new_canvas()
    d.rectangle([CX - 380, CY + 200, CX + 380, CY + 250], fill=SULFUR)
    # кристаллы серы
    for cx0, h, w in [(-250, 260, 90), (-150, 340, 110), (-40, 280, 80), (80, 360, 120), (200, 250, 90), (290, 300, 100)]:
        d.polygon([(cx0 - w, CY + 200), (cx0, CY + 200 - h), (cx0 + w, CY + 200)], fill=YELLOW)
        d.line([(cx0 - w, CY + 200), (cx0, CY + 200 - h), (cx0 + w, CY + 200), (cx0 - w, CY + 200)], fill=BORDER, width=5)
        d.line([(cx0 - w // 2, CY + 200), (cx0, CY + 200 - h * 0.6)], fill=BORDER, width=4)
        d.line([(cx0 + w // 2, CY + 200), (cx0, CY + 200 - h * 0.6)], fill=BORDER, width=4)
    d.ellipse([CX - 80, CY - 260, CX + 40, CY - 140], fill=YELLOW, outline=BORDER, width=5)
    d.ellipse([CX + 120, CY - 240, CX + 220, CY - 140], fill=SULFUR, outline=BORDER, width=5)
    save(img, 'sil_sernye_polya')


# 10. Хемосинтетические сады
def sil_khemosinteticheskie_sady():
    img, d = new_canvas()
    d.rectangle([CX - 360, CY + 180, CX + 360, CY + 230], fill=DARK)
    # трубы-дымоходы
    for tx, h, c in [(-260, 260, PURPLE), (-150, 340, MAGENTA), (-40, 230, PURPLE),
                     (70, 320, TEAL), (180, 280, PURPLE), (280, 240, MAGENTA)]:
        d.rectangle([tx - 28, CY + 180 - h, tx + 28, CY + 180], fill=c, outline=BORDER, width=5)
        d.ellipse([tx - 34, CY + 180 - h - 16, tx + 34, CY + 180 - h + 16], fill=c, outline=BORDER, width=4)
    # пузыри-свечение
    for bx, by in [(-120, CY - 20), (10, CY - 60), (140, CY - 10)]:
        d.ellipse([bx - 18, by - 18, bx + 18, by + 18], fill=CYAN, outline=BORDER, width=4)
    save(img, 'sil_khemosinteticheskie_sady')


# 11. Химический иней — вертикальная композиция: узкая плита + высокие шипы + гексагоны
def sil_khimicheskiy_iney():
    img, d = new_canvas()
    d.rectangle([CX - 180, CY + 160, CX + 180, CY + 210], fill=STEEL_D)
    # высокие шипы (вертикаль)
    for sx, h in [(-120, 380), (0, 460), (120, 350), (-60, 300), (60, 300)]:
        base_y = CY + 160
        d.polygon([(sx - 18, base_y), (sx + 18, base_y), (sx, base_y - h)], fill=WHITE)
        d.line([(sx - 18, base_y), (sx + 18, base_y), (sx, base_y - h), (sx - 18, base_y)], fill=BORDER, width=5)
    # гексагональные кристаллы между шипами
    for kx, s in [(-160, 70), (160, 80), (0, 55)]:
        pts = []
        for i in range(6):
            import math
            ang = math.radians(60 * i - 30)
            pts.append((kx + s * math.cos(ang), CY - 60 + s * math.sin(ang)))
        d.polygon(pts, fill=ICE, outline=BORDER, width=4)
        d.line([(pts[0][0], pts[0][1]), (pts[3][0], pts[3][1])], fill=BORDER, width=3)
    # свисающие сосульки вниз
    for sx, h in [(-90, 130), (90, 100), (0, 160)]:
        d.polygon([(sx - 12, CY + 210), (sx + 12, CY + 210), (sx, CY + 210 + h)], fill=WHITE)
        d.line([(sx - 12, CY + 210), (sx + 12, CY + 210), (sx, CY + 210 + h), (sx - 12, CY + 210)], fill=BORDER, width=4)
    save(img, 'sil_khimicheskiy_iney')


# 12. Метановые моря
def sil_metanovye_morya():
    img, d = new_canvas()
    for i, (y, c) in enumerate([(CY - 150, ICE), (CY - 50, (150, 200, 220)), (CY + 50, (150, 200, 220)), (CY + 150, ICE)]):
        d.polygon([(CX - 400, y + 35), (CX - 400, y - 35), (CX - 160, y - 35),
                   (CX - 60, y + 35), (CX + 120, y - 35), (CX + 240, y + 35),
                   (CX + 400, y - 35), (CX + 400, y + 35)], fill=c)
        d.line([(CX - 400, y + 35), (CX - 400, y - 35), (CX - 160, y - 35),
                (CX - 60, y + 35), (CX + 120, y - 35), (CX + 240, y + 35),
                (CX + 400, y - 35), (CX + 400, y + 35), (CX + 400, y + 35)], fill=BORDER, width=5)
        d.line([(CX - 160, y - 35), (CX - 60, y + 35)], fill=BORDER, width=5)
        d.line([(CX + 120, y - 35), (CX + 240, y + 35)], fill=BORDER, width=5)
    d.ellipse([CX - 300, CY - 260, CX - 180, CY - 140], fill=WHITE, outline=BORDER, width=4)
    save(img, 'sil_metanovye_morya')


# 13. Углеводородные равнины
def sil_uglevodorodnye_ravniny():
    img, d = new_canvas()
    d.rectangle([CX - 400, CY + 60, CX + 400, CY + 260], fill=(60, 55, 50))
    d.line([(CX - 400, CY + 60), (CX + 400, CY + 60)], fill=BORDER, width=6)
    # вентиляционная башня (высота композиции)
    d.rectangle([CX - 30, CY + 60 - 300, CX + 30, CY + 60], fill=STEEL, outline=BORDER, width=5)
    d.line([(CX - 30, CY + 60 - 300), (CX + 30, CY + 60 - 300)], fill=BORDER, width=5)
    d.ellipse([CX - 52, CY + 60 - 320, CX + 52, CY + 60 - 280], fill=STEEL, outline=BORDER, width=4)
    d.rectangle([CX - 16, CY + 60 - 340, CX + 16, CY + 60 - 310], fill=STEEL_D, outline=BORDER, width=3)
    d.polygon([(CX - 260, CY + 260), (CX - 260, CY + 80), (CX - 120, CY + 80), (CX - 120, CY + 260)], fill=TAR)
    d.line([(CX - 260, CY + 80), (CX - 120, CY + 80)], fill=BORDER, width=5)
    d.ellipse([CX + 20, CY + 130, CX + 180, CY + 250], fill=TAR, outline=BORDER, width=5)
    d.ellipse([CX + 30, CY + 140, CX + 170, CY + 240], fill=(50, 42, 38), outline=BORDER, width=4)
    d.ellipse([CX - 230, CY + 110, CX - 150, CY + 190], fill=(50, 42, 38), outline=BORDER, width=4)
    d.line([(CX - 200, CY + 150), (CX - 170, CY + 150)], fill=WHITE, width=6)
    d.line([(CX + 90, CY + 180), (CX + 130, CY + 180)], fill=WHITE, width=6)
    save(img, 'sil_uglevodorodnye_ravniny')


# 14. Инеевые рощи
def sil_ineevye_roshchi():
    img, d = new_canvas()
    d.rectangle([CX - 340, CY + 220, CX + 340, CY + 260], fill=ICE)
    tree(d, CX - 220, CY + 220, 200, (220, 240, 245))
    tree(d, CX + 10, CY + 220, 260, ICE)
    tree(d, CX + 240, CY + 220, 170, (220, 240, 245))
    d.ellipse([CX - 160, CY - 60, CX - 90, CY + 10], fill=WHITE, outline=BORDER, width=4)
    d.ellipse([CX + 130, CY - 110, CX + 200, CY - 40], fill=WHITE, outline=BORDER, width=4)
    save(img, 'sil_ineevye_roshchi')


# 15. Лавовые поля
def sil_lavovye_polya():
    img, d = new_canvas()
    d.rectangle([CX - 400, CY + 140, CX + 400, CY + 260], fill=DARK)
    d.polygon([(CX - 360, CY + 140), (CX - 360, CY + 40), (CX - 220, CY + 40), (CX - 220, CY + 140)], fill=DARK)
    d.polygon([(CX - 100, CY + 140), (CX - 100, CY + 60), (CX + 40, CY + 60), (CX + 40, CY + 140)], fill=DARK)
    d.polygon([(CX + 180, CY + 140), (CX + 180, CY + 10), (CX + 320, CY + 10), (CX + 320, CY + 140)], fill=DARK)
    # трещины с лавой
    d.polygon([(CX - 330, CY + 240), (CX - 330, CY + 180), (CX - 180, CY + 220), (CX - 180, CY + 260)], fill=FIRE)
    d.line([(CX - 330, CY + 180), (CX - 180, CY + 220)], fill=BORDER, width=4)
    d.polygon([(CX - 130, CY + 260), (CX - 130, CY + 190), (CX + 40, CY + 170), (CX + 40, CY + 240)], fill=ORANGE)
    d.polygon([(CX + 90, CY + 200), (CX + 90, CY + 120), (CX + 260, CY + 150), (CX + 260, CY + 230)], fill=FIRE)
    d.line([(CX + 90, CY + 120), (CX + 260, CY + 150)], fill=BORDER, width=4)
    d.ellipse([CX - 240, CY + 40, CX - 160, CY + 120], fill=FIRE, outline=BORDER, width=4)
    save(img, 'sil_lavovye_polya')


# 16. Магмовый океан
def sil_magmovyy_okean():
    img, d = new_canvas()
    for i, (y, c) in enumerate([(CY - 160, RED), (CY - 50, FIRE), (CY + 60, ORANGE), (CY + 170, RED)]):
        d.polygon([(CX - 400, y + 45), (CX - 400, y - 45), (CX - 170, y - 45),
                   (CX - 70, y + 45), (CX + 110, y - 45), (CX + 230, y + 45),
                   (CX + 400, y - 45), (CX + 400, y + 45)], fill=c)
        d.line([(CX - 400, y + 45), (CX - 400, y - 45), (CX - 170, y - 45),
                (CX - 70, y + 45), (CX + 110, y - 45), (CX + 230, y + 45),
                (CX + 400, y - 45), (CX + 400, y + 45), (CX + 400, y + 45)], fill=BORDER, width=5)
        d.line([(CX - 170, y - 45), (CX - 70, y + 45)], fill=BORDER, width=5)
        d.line([(CX + 110, y - 45), (CX + 230, y + 45)], fill=BORDER, width=5)
        d.line([(CX - 300, y - 20), (CX - 240, y - 20)], fill=(250, 200, 90), width=8)
        d.line([(CX + 40, y + 10), (CX + 100, y + 10)], fill=(250, 200, 90), width=8)
    save(img, 'sil_magmovyy_okean')


# 17. Вулканические поля
def sil_vulkanicheskie_polya():
    img, d = new_canvas()
    d.rectangle([CX - 400, CY + 210, CX + 400, CY + 260], fill=DARK)
    # конусы вулканов
    d.polygon([(CX - 280, CY + 210), (CX - 200, CY - 60), (CX - 120, CY + 210)], fill=(60, 50, 55))
    d.line([(CX - 280, CY + 210), (CX - 200, CY - 60), (CX - 120, CY + 210), (CX - 280, CY + 210)], fill=BORDER, width=5)
    d.ellipse([CX - 220, CY - 80, CX - 180, CY - 40], fill=FIRE, outline=BORDER, width=4)
    d.polygon([(CX - 40, CY + 210), (CX + 60, CY + 30), (CX + 160, CY + 210)], fill=(70, 58, 62))
    d.line([(CX - 40, CY + 210), (CX + 60, CY + 30), (CX + 160, CY + 210), (CX - 40, CY + 210)], fill=BORDER, width=5)
    d.ellipse([CX + 40, CY + 10, CX + 80, CY + 50], fill=ORANGE, outline=BORDER, width=4)
    # потоки лавы
    d.line([(CX - 200, CY - 20), (CX - 240, CY + 210)], fill=FIRE, width=18)
    d.line([(CX + 60, CY + 60), (CX + 20, CY + 210)], fill=ORANGE, width=14)
    d.ellipse([CX + 240, CY + 120, CX + 320, CY + 200], fill=ORANGE, outline=BORDER, width=4)
    save(img, 'sil_vulkanicheskie_polya')


# 18. Стеклянные поля
def sil_steklyannye_polya():
    img, d = new_canvas()
    d.rectangle([CX - 360, CY + 220, CX + 360, CY + 260], fill=STEEL_D)
    # осколки стекла
    for sx, h, w in [(-250, 280, 70), (-140, 350, 90), (-30, 250, 60), (90, 330, 80), (210, 270, 65)]:
        d.polygon([(sx - w, CY + 220), (sx, CY + 220 - h), (sx + w, CY + 220)], fill=GLASS)
        d.line([(sx - w, CY + 220), (sx, CY + 220 - h), (sx + w, CY + 220), (sx - w, CY + 220)], fill=BORDER, width=5)
        d.line([(sx - w, CY + 220), (sx, CY + 220 - h * 0.55)], fill=(220, 235, 250), width=4)
        d.line([(sx + w, CY + 220), (sx, CY + 220 - h * 0.55)], fill=BORDER, width=4)
    d.ellipse([CX - 300, CY - 120, CX - 220, CY - 40], fill=GLASS, outline=BORDER, width=4)
    d.ellipse([CX + 150, CY - 140, CX + 230, CY - 60], fill=ICE, outline=BORDER, width=4)
    save(img, 'sil_steklyannye_polya')


# 19. Кристальные рощи
def sil_kristalnye_roshchi():
    img, d = new_canvas()
    d.rectangle([CX - 360, CY + 210, CX + 360, CY + 250], fill=DARK)
    for kx, h, c in [(-260, 300, PURPLE), (-150, 380, CYAN), (-50, 260, PURPLE),
                     (60, 360, MAGENTA), (170, 290, CYAN), (270, 330, PURPLE)]:
        d.polygon([(kx - 34, CY + 210), (kx, CY + 210 - h), (kx + 34, CY + 210)], fill=c)
        d.line([(kx - 34, CY + 210), (kx, CY + 210 - h), (kx + 34, CY + 210), (kx - 34, CY + 210)], fill=BORDER, width=5)
        d.line([(kx, CY + 210 - h), (kx, CY + 210)], fill=BORDER, width=4)
        d.line([(kx - 34, CY + 210), (kx, CY + 210 - h * 0.5)], fill=(255, 240, 250), width=3)
    d.ellipse([-160 + CX, CY - 150, -60 + CX, CY - 50], fill=CYAN, outline=BORDER, width=4)
    save(img, 'sil_kristalnye_roshchi')


# 20. Кремниевые рощи
def sil_kremnievye_roshchi():
    img, d = new_canvas()
    d.rectangle([CX - 360, CY + 220, CX + 360, CY + 260], fill=STEEL_D)
    # кремниевые шпили
    for sx, h, w in [(-260, 240, 60), (-150, 340, 80), (-40, 280, 55), (80, 360, 90), (200, 250, 60)]:
        d.polygon([(sx - w, CY + 220), (sx, CY + 220 - h), (sx + w, CY + 220)], fill=STEEL)
        d.line([(sx - w, CY + 220), (sx, CY + 220 - h), (sx + w, CY + 220), (sx - w, CY + 220)], fill=BORDER, width=5)
        d.line([(sx - w, CY + 220), (sx, CY + 220 - h * 0.5)], fill=(170, 195, 215), width=4)
        d.line([(sx + w, CY + 220), (sx, CY + 220 - h * 0.5)], fill=BORDER, width=4)
    d.ellipse([-300 + CX, CY - 60, -220 + CX, CY + 20], fill=TEAL, outline=BORDER, width=4)
    save(img, 'sil_kremnievye_roshchi')


# 21. Металлические щетинные поля
def sil_metallicheskie_shchetinnye_polya():
    img, d = new_canvas()
    d.rectangle([CX - 380, CY + 230, CX + 380, CY + 270], fill=STEEL_D)
    # щетинки
    import random
    random.seed(7)
    for _ in range(26):
        bx = random.randint(-350, 350)
        bh = random.randint(120, 300)
        d.line([(CX + bx, CY + 230), (CX + bx + 8, CY + 230 - bh)], fill=STEEL, width=8)
    for bx, bh in [(-300, 330), (-160, 370), (20, 350), (180, 380), (320, 300)]:
        d.line([(CX + bx, CY + 230), (CX + bx + 8, CY + 230 - bh)], fill=(200, 215, 235), width=10)
    save(img, 'sil_metallicheskie_shchetinnye_polya')


# 22. Струнные рощи
def sil_strunnye_roshchi():
    img, d = new_canvas()
    d.rectangle([CX - 340, CY + 240, CX + 340, CY + 270], fill=DARK)
    # тонкие струны
    for i, sx in enumerate(range(-280, 300, 40)):
        sway = (i % 3) * 10 - 10
        d.line([(CX + sx, CY + 240), (CX + sx + sway, CY - 220)], fill=(90, 100, 120), width=6)
    # подсвеченные струны
    for sx in (-200, -40, 120, 240):
        d.line([(CX + sx, CY + 240), (CX + sx, CY - 220)], fill=CYAN, width=8)
    d.ellipse([-260 + CX, CY - 90, -180 + CX, CY - 10], fill=CYAN, outline=BORDER, width=4)
    d.ellipse([80 + CX, CY - 140, 160 + CX, CY - 60], fill=WHITE, outline=BORDER, width=4)
    save(img, 'sil_strunnye_roshchi')


# 23. Терминаторная зона
def sil_terminatornaya_zona():
    img, d = new_canvas()
    # светлая сторона (верх) и тёмная (низ), граница изогнутая
    d.rectangle([CX - 400, CY - 260, CX + 400, CY - 30], fill=GRASS)
    d.rectangle([CX - 400, CY + 60, CX + 400, CY + 260], fill=DARK)
    d.pieslice([CX - 260, CY - 40, CX + 260, CY + 300], 0, 180, fill=DARK)
    d.pieslice([CX - 260, CY - 40, CX + 260, CY + 300], 180, 360, fill=GRASS)
    d.line([(CX - 400, CY - 30), (CX - 260, CY - 30)], fill=BORDER, width=6)
    d.line([(CX + 260, CY - 30), (CX + 400, CY - 30)], fill=BORDER, width=6)
    # звёзды на тёмной стороне
    for sx, sy in [(-120, CY + 140), (40, CY + 90), (180, CY + 160), (260, CY + 60)]:
        d.ellipse([CX + sx - 12, CY + sy - 12, CX + sx + 12, CY + sy + 12], fill=WHITE, outline=BORDER, width=3)
    # солнце на светлой стороне
    d.ellipse([CX - 300, CY - 200, CX - 180, CY - 80], fill=GOLD, outline=BORDER, width=5)
    save(img, 'sil_terminatornaya_zona')


# ============================ СТИЛЬ-ТЕСТЫ (леса и метановые моря в B и C) ============================

# B — мини-пейзаж с атмосферой: горизонт, небо, полоса леса, земля
def sil_lesa_B():
    img, d = new_canvas()
    d.rectangle([CX - 400, CY - 300, CX + 400, CY - 60], fill=(60, 110, 170))
    d.rectangle([CX - 400, CY - 60, CX + 400, CY + 300], fill=GREEN_D)
    d.ellipse([CX + 200, CY - 240, CX + 300, CY - 140], fill=GOLD, outline=BORDER, width=5)
    for tx in range(-340, 360, 80):
        tree(d, CX + tx, CY - 60, 120 + (tx % 3) * 30, GREEN if tx % 2 == 0 else GREEN_D)
    d.ellipse([CX - 300, CY - 180, CX - 230, CY - 110], fill=WHITE, outline=BORDER, width=4)
    d.ellipse([CX - 180, CY - 250, CX - 130, CY - 200], fill=WHITE, outline=BORDER, width=4)
    save(img, 'sil_lesa_B')


# C — геральдика: круглая бляха с символом
def sil_lesa_C():
    img, d = new_canvas()
    d.ellipse([CX - 360, CY - 360, CX + 360, CY + 360], fill=(90, 60, 40))
    d.ellipse([CX - 320, CY - 320, CX + 320, CY + 320], fill=(160, 130, 80), outline=BORDER, width=8)
    tree(d, CX, CY + 130, 260, GREEN)
    d.ellipse([CX - 120, CY - 40, CX - 20, CY + 60], fill=GOLD, outline=BORDER, width=4)
    save(img, 'sil_lesa_C')


def sil_metanovye_morya_B():
    img, d = new_canvas()
    d.rectangle([CX - 400, CY - 300, CX + 400, CY + 40], fill=(40, 70, 90))
    d.rectangle([CX - 400, CY + 40, CX + 400, CY + 300], fill=(120, 170, 195))
    d.ellipse([CX - 260, CY - 220, CX - 160, CY - 120], fill=ICE, outline=BORDER, width=4)
    d.ellipse([CX + 220, CY - 260, CX + 300, CY - 180], fill=(200, 225, 240), outline=BORDER, width=4)
    for i, (y, c) in enumerate([(CY + 80, (150, 200, 220)), (CY + 160, (120, 175, 200)), (CY + 240, ICE)]):
        d.polygon([(CX - 400, y + 30), (CX - 400, y - 30), (CX - 120, y - 30),
                   (CX - 20, y + 30), (CX + 160, y - 30), (CX + 300, y + 30),
                   (CX + 400, y - 30), (CX + 400, y + 30)], fill=c)
        d.line([(CX - 400, y + 30), (CX - 400, y - 30), (CX - 120, y - 30),
                (CX - 20, y + 30), (CX + 160, y - 30), (CX + 300, y + 30),
                (CX + 400, y - 30), (CX + 400, y + 30), (CX + 400, y + 30)], fill=BORDER, width=5)
        d.line([(CX - 120, y - 30), (CX - 20, y + 30)], fill=BORDER, width=5)
        d.line([(CX + 160, y - 30), (CX + 300, y + 30)], fill=BORDER, width=5)
    save(img, 'sil_metanovye_morya_B')


def sil_metanovye_morya_C():
    img, d = new_canvas()
    d.ellipse([CX - 360, CY - 360, CX + 360, CY + 360], fill=(40, 60, 80))
    d.ellipse([CX - 320, CY - 320, CX + 320, CY + 320], fill=(100, 150, 180), outline=BORDER, width=8)
    for i, (y, c) in enumerate([(CY - 60, ICE), (CY + 30, (150, 200, 220)), (CY + 120, ICE)]):
        d.polygon([(CX - 260, y + 30), (CX - 260, y - 30), (CX - 80, y - 30),
                   (CX + 20, y + 30), (CX + 160, y - 30), (CX + 260, y + 30),
                   (CX + 260, y + 30)], fill=c)
        d.line([(CX - 260, y + 30), (CX - 260, y - 30), (CX - 80, y - 30),
                (CX + 20, y + 30), (CX + 160, y - 30), (CX + 260, y + 30),
                (CX + 260, y + 30)], fill=BORDER, width=5)
        d.line([(CX - 80, y - 30), (CX + 20, y + 30)], fill=BORDER, width=5)
        d.line([(CX + 160, y - 30), (CX + 260, y + 30)], fill=BORDER, width=5)
    d.ellipse([CX - 220, CY - 220, CX - 140, CY - 140], fill=WHITE, outline=BORDER, width=4)
    save(img, 'sil_metanovye_morya_C')


if __name__ == '__main__':
    sil_lesa()
    sil_luga_steppi()
    sil_okeany()
    sil_bolota()
    sil_korallovye_rifы()
    sil_svetyashchiesya_chashchi()
    sil_bagrovye_steppi()
    sil_kislotnye_debri()
    sil_sernye_polya()
    sil_khemosinteticheskie_sady()
    sil_khimicheskiy_iney()
    sil_metanovye_morya()
    sil_uglevodorodnye_ravniny()
    sil_ineevye_roshchi()
    sil_lavovye_polya()
    sil_magmovyy_okean()
    sil_vulkanicheskie_polya()
    sil_steklyannye_polya()
    sil_kristalnye_roshchi()
    sil_kremnievye_roshchi()
    sil_metallicheskie_shchetinnye_polya()
    sil_strunnye_roshchi()
    sil_terminatornaya_zona()
    sil_lesa_B()
    sil_lesa_C()
    sil_metanovye_morya_B()
    sil_metanovye_morya_C()
    print('ALL SILHOUETTES DONE')