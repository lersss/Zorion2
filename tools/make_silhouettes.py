# -*- coding: utf-8 -*-
# Генерация силуэтов кораблей для ControlNet Canny (SDXL).
# Каждый силуэт: белый корабль на чёрном фоне, 1024x1024, вид сверху, нос вправо.
# Canny возьмёт контуры. Сохраняет в ai_drafts/silhouettes/
import os
from PIL import Image, ImageDraw

OUT = r'C:\Zorion2\ai_drafts\silhouettes'
SIZE = 1024
CX, CY = SIZE // 2, SIZE // 2


def new_canvas():
    img = Image.new('L', (SIZE, SIZE), 0)
    return img, ImageDraw.Draw(img)


def save(img, name):
    os.makedirs(OUT, exist_ok=True)
    path = os.path.join(OUT, name + '.png')
    img.save(path)
    print('Saved:', path)


def poly(d, pts, fill=255):
    d.polygon(pts, fill=fill)


# 1. СТРЕЛА — длинный клин, дельта-крылья, двойной хвост
def arrow():
    img, d = new_canvas()
    # корпус: длинный клин (нос вправо)
    poly(d, [(CX - 300, CY - 40), (CX + 420, CY - 8), (CX + 420, CY + 8), (CX - 300, CY + 40)])
    # дельта-крылья
    poly(d, [(CX - 220, CY - 30), (CX - 80, CY - 200), (CX - 60, CY - 200), (CX - 160, CY - 30)])
    poly(d, [(CX - 220, CY + 30), (CX - 80, CY + 200), (CX - 60, CY + 200), (CX - 160, CY + 30)])
    # нос-игла
    poly(d, [(CX + 350, CY - 12), (CX + 510, CY), (CX + 350, CY + 12)])
    # двойной хвост
    poly(d, [(CX - 300, CY - 40), (CX - 420, CY - 90), (CX - 400, CY - 85), (CX - 280, CY - 30)])
    poly(d, [(CX - 300, CY + 40), (CX - 420, CY + 90), (CX - 400, CY + 85), (CX - 280, CY + 30)])
    save(img, 'sil_arrow')


# 2. КРЕЙСЕР — массивный корпус, бронированные борта, срезанный нос
def cruiser():
    img, d = new_canvas()
    # корпус: широкий, ступенчатый
    poly(d, [(CX - 350, CY - 120), (CX + 250, CY - 90), (CX + 380, CY - 40), (CX + 380, CY + 40), (CX + 250, CY + 90), (CX - 350, CY + 120)])
    # ступени брони (вырезы по бортам - рисуем тёмным)
    poly(d, [(CX - 100, CY - 120), (CX + 50, CY - 105), (CX + 50, CY - 100), (CX - 100, CY - 115)], fill=0)
    poly(d, [(CX - 100, CY + 120), (CX + 50, CY + 105), (CX + 50, CY + 100), (CX - 100, CY + 115)], fill=0)
    # нос: срезанный клин
    poly(d, [(CX + 320, CY - 55), (CX + 440, CY - 20), (CX + 440, CY + 20), (CX + 320, CY + 55)])
    # корма: квадратный
    poly(d, [(CX - 350, CY - 100), (CX - 420, CY - 70), (CX - 420, CY + 70), (CX - 350, CY + 100)])
    save(img, 'sil_cruiser')


# 3. ЯСТРЕБ — стреловидный, изогнутые крылья, тонкий нос
def hawk():
    img, d = new_canvas()
    # корпус: тонкий веретено
    poly(d, [(CX - 300, CY - 35), (CX + 380, CY - 10), (CX + 420, CY), (CX + 380, CY + 10), (CX - 300, CY + 35)])
    # крылья стрелой назад
    poly(d, [(CX - 200, CY - 25), (CX - 60, CY - 260), (CX - 30, CY - 255), (CX - 130, CY - 25)])
    poly(d, [(CX - 200, CY + 25), (CX - 60, CY + 260), (CX - 30, CY + 255), (CX - 130, CY + 25)])
    # законцовки крыльев
    poly(d, [(CX - 90, CY - 250), (CX - 70, CY - 265), (CX - 50, CY - 250), (CX - 70, CY - 235)])
    poly(d, [(CX - 90, CY + 250), (CX - 70, CY + 265), (CX - 50, CY + 250), (CX - 70, CY + 235)])
    # нос: длинная игла
    poly(d, [(CX + 350, CY - 8), (CX + 480, CY), (CX + 350, CY + 8)])
    save(img, 'sil_hawk')


# 4. ГРУЗОВИК — коробчатый, два больших двигателя, широкая корма
def freighter():
    img, d = new_canvas()
    # корпус: прямоугольный блок
    poly(d, [(CX - 380, CY - 110), (CX + 250, CY - 110), (CX + 250, CY + 110), (CX - 380, CY + 110)])
    # тупой нос
    poly(d, [(CX + 250, CY - 90), (CX + 330, CY - 60), (CX + 330, CY + 60), (CX + 250, CY + 90)])
    # два больших двигателя на корме (круглые)
    d.ellipse([CX - 430, CY - 170, CX - 320, CY - 60], fill=255)
    d.ellipse([CX - 430, CY + 60, CX - 320, CY + 170], fill=255)
    # кормовая перекладина
    poly(d, [(CX - 380, CY - 100), (CX - 330, CY - 100), (CX - 330, CY + 100), (CX - 380, CY + 100)])
    save(img, 'sil_freighter')


if __name__ == '__main__':
    arrow()
    cruiser()
    hawk()
    freighter()