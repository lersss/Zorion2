# -*- coding: utf-8 -*-
# Генерация ЦВЕТНЫХ силуэтов кораблей для ControlNet (img2img + Canny).
# Каждый модуль корабля — свой цвет, между модулями тёмные границы-линии.
# Фон чёрный. 1024x1024, вид сверху, нос вправо.
import os
from PIL import Image, ImageDraw

OUT = r'C:\Zorion2\ai_drafts\silhouettes'
SIZE = 1024
CX, CY = SIZE // 2, SIZE // 2

# Цвета модулей (RGB) — контрастные, чтобы img2img не сгладил
CREAM = (232, 224, 186)      # корпус
IVORY = (245, 240, 215)      # светлые панели
BRONZE = (140, 90, 45)       # хвост/вставки (тёмно-бронзовый)
STEEL = (120, 140, 170)      # крылья (сине-стальной)
DARK = (40, 44, 55)          # двигатели (почти чёрные)
GLASS = (180, 210, 235)      # кабина-стекло (светло-голубое)
FIRE = (230, 120, 40)        # свечение дюз (оранжевое)
BORDER = (12, 12, 12)        # границы модулей


def new_canvas():
    img = Image.new('RGB', (SIZE, SIZE), (5, 5, 5))
    return img, ImageDraw.Draw(img)


def save(img, name):
    os.makedirs(OUT, exist_ok=True)
    img.save(os.path.join(OUT, name + '.png'))
    print('Saved:', name)


def border(d, pts, w=5):
    d.line(pts + [pts[0]], fill=BORDER, width=w)


# 1. СТРЕЛА: корпус-клин (крем), кабина-стекло (голубое), дельта-крылья (сталь), движки (тёмный), двойной хвост (бронза), кили/панели
def arrow():
    img, d = new_canvas()
    # крылья (под корпусом, сталь) — ступенчатые
    poly_pts = [(CX - 220, CY - 30), (CX - 80, CY - 200), (CX - 60, CY - 200), (CX - 160, CY - 30)]
    d.polygon(poly_pts, fill=STEEL); border(d, poly_pts)
    poly_pts = [(CX - 220, CY + 30), (CX - 80, CY + 200), (CX - 60, CY + 200), (CX - 160, CY + 30)]
    d.polygon(poly_pts, fill=STEEL); border(d, poly_pts)
    # законцовки крыльев (сталь светлее)
    d.polygon([(CX - 90, CY - 190), (CX - 70, CY - 205), (CX - 50, CY - 190), (CX - 70, CY - 175)], fill=IVORY)
    d.polygon([(CX - 90, CY + 190), (CX - 70, CY + 205), (CX - 50, CY + 190), (CX - 70, CY + 175)], fill=IVORY)
    # двойной хвост (бронза) — с килями
    poly_pts = [(CX - 300, CY - 40), (CX - 420, CY - 90), (CX - 400, CY - 85), (CX - 280, CY - 30)]
    d.polygon(poly_pts, fill=BRONZE); border(d, poly_pts)
    poly_pts = [(CX - 300, CY + 40), (CX - 420, CY + 90), (CX - 400, CY + 85), (CX - 280, CY + 30)]
    d.polygon(poly_pts, fill=BRONZE); border(d, poly_pts)
    # двигатели (тёмные блоки у кормы) + оранжевое свечение
    d.rectangle([CX - 320, CY - 45, CX - 260, CY - 5], fill=DARK, outline=BORDER, width=4)
    d.rectangle([CX - 320, CY + 5, CX - 260, CY + 45], fill=DARK, outline=BORDER, width=4)
    d.rectangle([CX - 340, CY - 38, CX - 320, CY - 12], fill=FIRE, outline=BORDER, width=2)
    d.rectangle([CX - 340, CY + 12, CX - 320, CY + 38], fill=FIRE, outline=BORDER, width=2)
    # корпус (крем)
    poly_pts = [(CX - 300, CY - 40), (CX + 350, CY - 10), (CX + 400, CY), (CX + 350, CY + 10), (CX - 300, CY + 40)]
    d.polygon(poly_pts, fill=CREAM); border(d, poly_pts)
    # панели корпуса (иври, продольные линии)
    d.line([CX - 200, CY - 18, CX + 200, CY - 6], fill=IVORY, width=6)
    d.line([CX - 200, CY + 18, CX + 200, CY + 6], fill=IVORY, width=6)
    # боковые кили-перегородки (бронза, перед крыльями)
    d.rectangle([CX - 140, CY - 55, CX - 110, CY - 40], fill=BRONZE, outline=BORDER, width=3)
    d.rectangle([CX - 140, CY + 40, CX - 110, CY + 55], fill=BRONZE, outline=BORDER, width=3)
    # кабина-стекло на носу (светлое) + антенна
    poly_pts = [(CX + 330, CY - 12), (CX + 420, CY), (CX + 330, CY + 12)]
    d.polygon(poly_pts, fill=GLASS); border(d, poly_pts)
    d.line([CX + 430, CY, CX + 470, CY], fill=STEEL, width=8)
    save(img, 'sil_arrow_modules')


# 3. ЯСТРЕБ — «летающее крыло»: огромные стреловидные крылья на всю ширину, узкое тело
def hawk():
    img, d = new_canvas()
    # ОГРОМНЫЕ стреловидные крылья (размах почти весь холст)
    poly_pts = [(CX - 100, CY - 20), (CX + 120, CY - 320), (CX + 180, CY - 320), (CX + 60, CY - 20)]
    d.polygon(poly_pts, fill=STEEL); border(d, poly_pts)
    poly_pts = [(CX - 100, CY + 20), (CX + 120, CY + 320), (CX + 180, CY + 320), (CX + 60, CY + 20)]
    d.polygon(poly_pts, fill=STEEL); border(d, poly_pts)
    # законцовки крыльев (бронза)
    d.polygon([(CX + 140, CY - 315), (CX + 190, CY - 320), (CX + 150, CY - 290)], fill=BRONZE)
    d.polygon([(CX + 140, CY + 315), (CX + 190, CY + 320), (CX + 150, CY + 290)], fill=BRONZE)
    # узкое тело-веретено (крем)
    poly_pts = [(CX - 300, CY - 28), (CX + 380, CY - 10), (CX + 420, CY), (CX + 380, CY + 10), (CX - 300, CY + 28)]
    d.polygon(poly_pts, fill=CREAM); border(d, poly_pts)
    # кабина-стекло (голубое)
    poly_pts = [(CX + 350, CY - 8), (CX + 420, CY), (CX + 350, CY + 8)]
    d.polygon(poly_pts, fill=GLASS); border(d, poly_pts)
    # хвостовой веер (бронза, широкий)
    poly_pts = [(CX - 300, CY - 25), (CX - 420, CY - 90), (CX - 410, CY - 85), (CX - 290, CY - 20)]
    d.polygon(poly_pts, fill=BRONZE); border(d, poly_pts)
    poly_pts = [(CX - 300, CY + 25), (CX - 420, CY + 90), (CX - 410, CY + 85), (CX - 290, CY + 20)]
    d.polygon(poly_pts, fill=BRONZE); border(d, poly_pts)
    # дюзы на корме
    d.rectangle([CX - 320, CY - 30, CX - 285, CY - 2], fill=DARK, outline=BORDER, width=3)
    d.rectangle([CX - 320, CY + 2, CX - 285, CY + 30], fill=DARK, outline=BORDER, width=3)
    d.rectangle([CX - 332, CY - 24, CX - 320, CY - 8], fill=FIRE, outline=BORDER, width=2)
    d.rectangle([CX - 332, CY + 8, CX - 320, CY + 24], fill=FIRE, outline=BORDER, width=2)
    save(img, 'sil_hawk_modules')


# 4. КРЕЙСЕР — широкий массивный блок, надстройка-башня по центру, тупой нос, кормовая батарея
def cruiser():
    img, d = new_canvas()
    # широкий корпус-блок (крем)
    d.rectangle([CX - 360, CY - 160, CX + 320, CY + 160], fill=CREAM, outline=BORDER, width=6)
    # надстройка-башня по центру (сталь)
    d.rectangle([CX - 120, CY - 100, CX + 60, CY + 100], fill=STEEL, outline=BORDER, width=5)
    # центральная башенка (тёмная)
    d.rectangle([CX - 60, CY - 55, CX + 30, CY + 55], fill=DARK, outline=BORDER, width=4)
    # кабина-стекло на надстройке
    d.rectangle([CX + 10, CY - 40, CX + 60, CY + 40], fill=GLASS, outline=BORDER, width=3)
    # носовая бронеплита (сталь, тупой скос)
    d.polygon([(CX + 320, CY - 140), (CX + 420, CY - 70), (CX + 420, CY + 70), (CX + 320, CY + 140)], fill=STEEL)
    border(d, [(CX + 320, CY - 140), (CX + 420, CY - 70), (CX + 420, CY + 70), (CX + 320, CY + 140)])
    # кормовая батарея двигателей (тёмная, широкая)
    d.rectangle([CX - 400, CY - 130, CX - 360, CY + 130], fill=DARK, outline=BORDER, width=5)
    # дюзы (оранжевые, ряд)
    d.rectangle([CX - 410, CY - 120, CX - 400, CY - 60], fill=FIRE, outline=BORDER, width=2)
    d.rectangle([CX - 410, CY - 40, CX - 400, CY + 40], fill=FIRE, outline=BORDER, width=2)
    d.rectangle([CX - 410, CY + 60, CX - 400, CY + 120], fill=FIRE, outline=BORDER, width=2)
    # боковые выступы (бронза)
    d.rectangle([CX - 300, CY - 175, CX - 220, CY - 160], fill=BRONZE, outline=BORDER, width=4)
    d.rectangle([CX - 300, CY + 160, CX - 220, CY + 175], fill=BRONZE, outline=BORDER, width=4)
    save(img, 'sil_cruiser_modules')


# 4. ГРУЗОВИК: коробка (крем), тупой нос (синий), два движка (тёмный), кормовая рама (бронза)
def freighter():
    img, d = new_canvas()
    # два больших двигателя (тёмные круги)
    d.ellipse([CX - 430, CY - 170, CX - 320, CY - 60], fill=DARK, outline=BORDER, width=6)
    d.ellipse([CX - 430, CY + 60, CX - 320, CY + 170], fill=DARK, outline=BORDER, width=6)
    # корпус-коробка (крем)
    d.rectangle([CX - 380, CY - 110, CX + 250, CY + 110], fill=CREAM, outline=BORDER, width=6)
    # тупой нос (синий)
    poly_pts = [(CX + 250, CY - 90), (CX + 330, CY - 60), (CX + 330, CY + 60), (CX + 250, CY + 90)]
    d.polygon(poly_pts, fill=GLASS); border(d, poly_pts)
    # кормовая рама (бронза)
    d.rectangle([CX - 380, CY - 100, CX - 330, CY + 100], fill=BRONZE, outline=BORDER, width=4)
    # рёбра жёсткости (иври)
    d.line([CX - 100, CY - 110, CX - 100, CY + 110], fill=IVORY, width=6)
    d.line([CX + 50, CY - 110, CX + 50, CY + 110], fill=IVORY, width=6)
    save(img, 'sil_freighter_modules')


# 5. ПРОТОСС — симметричный элегантный «диск-крыло», золотой, изогнутые кромки, светящиеся синие ядра
def protoss():
    img, d = new_canvas()
    GOLD = (210, 170, 60)
    GOLD2 = (240, 215, 120)
    PURPLE = (120, 60, 180)
    # центральное тело — ромбовидный диск (золотой)
    d.polygon([(CX - 260, CY), (CX - 80, CY - 220), (CX + 200, CY - 60), (CX + 280, CY), (CX + 200, CY + 60), (CX - 80, CY + 220)], fill=GOLD)
    # внешние «лезвия»-крылья (золото светлее), стрелой назад
    d.polygon([(CX - 120, CY - 180), (CX - 260, CY - 320), (CX - 180, CY - 330), (CX - 60, CY - 200)], fill=GOLD2)
    d.polygon([(CX - 120, CY + 180), (CX - 260, CY + 320), (CX - 180, CY + 330), (CX - 60, CY + 200)], fill=GOLD2)
    # передние лезвия (узкие, вперёд)
    d.polygon([(CX + 160, CY - 80), (CX + 320, CY - 140), (CX + 300, CY - 120), (CX + 180, CY - 50)], fill=GOLD2)
    d.polygon([(CX + 160, CY + 80), (CX + 320, CY + 140), (CX + 300, CY + 120), (CX + 180, CY + 50)], fill=GOLD2)
    # центральная кабина-ядро (светящееся сине-фиолетовое)
    d.ellipse([CX - 30, CY - 60, CX + 50, CY + 60], fill=GLASS, outline=BORDER, width=4)
    d.ellipse([CX + 10, CY - 25, CX + 40, CY + 25], fill=PURPLE, outline=BORDER, width=3)
    # кормовые крылья-пластины (пурпур)
    d.polygon([(CX - 240, CY - 40), (CX - 340, CY - 80), (CX - 330, CY - 60), (CX - 220, CY - 20)], fill=PURPLE)
    d.polygon([(CX - 240, CY + 40), (CX - 340, CY + 80), (CX - 330, CY + 60), (CX - 220, CY + 20)], fill=PURPLE)
    # золотые накладки по краям тела
    d.line([CX - 200, CY, CX + 240, CY], fill=GOLD2, width=8)
    save(img, 'sil_protoss')


# 6. ЗЕРГ — асимметричный био-органический, панцирь, клыки, сегменты
def zerg():
    img, d = new_canvas()
    FLESH = (205, 175, 140)    # панцирь/плоть
    CHITIN = (150, 110, 80)    # тёмный хитин
    SPINE = (230, 200, 170)    # светлые сегменты
    POISON = (110, 200, 160)   # ядовито-зелёное свечение
    # тело — изогнутая «личинка» (плоть), широкая корма, сужение к носу
    d.polygon([(CX - 340, CY - 70), (CX - 200, CY - 130), (CX + 100, CY - 50), (CX + 320, CY - 10), (CX + 360, CY), (CX + 320, CY + 10), (CX + 100, CY + 50), (CX - 200, CY + 130), (CX - 340, CY + 70)], fill=FLESH)
    # сегменты панциря (светлые полосы поперёк)
    for x in range(-200, 250, 90):
        d.line([CX + x, CY - 70, CX + x + 20, CY + 60], fill=SPINE, width=10)
    # хитиновые наросты-клыки спереди (рога)
    d.polygon([(CX + 320, CY - 20), (CX + 420, CY - 60), (CX + 400, CY - 50), (CX + 330, CY - 10)], fill=CHITIN)
    d.polygon([(CX + 320, CY + 20), (CX + 420, CY + 60), (CX + 400, CY + 50), (CX + 330, CY + 10)], fill=CHITIN)
    # носовой шип (клык)
    d.polygon([(CX + 340, CY - 6), (CX + 460, CY), (CX + 340, CY + 6)], fill=CHITIN)
    # боковые щупальца-крылья (органические, изогнутые)
    d.polygon([(CX - 120, CY - 110), (CX - 200, CY - 230), (CX - 160, CY - 235), (CX - 80, CY - 120)], fill=CHITIN)
    d.polygon([(CX - 120, CY + 110), (CX - 200, CY + 230), (CX - 160, CY + 235), (CX - 80, CY + 120)], fill=CHITIN)
    # хвостовой шип (корма)
    d.polygon([(CX - 340, CY - 40), (CX - 440, CY - 70), (CX - 430, CY - 60), (CX - 340, CY - 20)], fill=CHITIN)
    d.polygon([(CX - 340, CY + 40), (CX - 440, CY + 70), (CX - 430, CY + 60), (CX - 340, CY + 20)], fill=CHITIN)
    # ядовитые глаза/поры (зелёные)
    d.ellipse([CX + 150, CY - 20, CX + 180, CY + 20], fill=POISON, outline=BORDER, width=3)
    d.ellipse([CX - 80, CY - 25, CX - 55, CY + 25], fill=POISON, outline=BORDER, width=3)
    save(img, 'sil_zerg')


if __name__ == '__main__':
    arrow()
    cruiser()
    hawk()
    freighter()
    protoss()
    zerg()