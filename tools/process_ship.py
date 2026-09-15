# -*- coding: utf-8 -*-
# Обработка кораблей Juggernaut XL: вырезание фона ПО ЦВЕТУ (фон ровный тёмный ~21,23,26),
# кадрирование, вписывание в 200x200, нормализация ориентации (нос вправо).
# Использование: python process_ship.py <in.png> <out.png>
import sys
from PIL import Image, ImageOps
import numpy as np
from scipy import ndimage

CANVAS = 200
PAD = 6
BG_TOL = 40  # допуск расстояния до цвета фона


def keep_largest_component(img):
    """Оставить только крупнейший связный компонент, остальное -> прозрачное."""
    arr = np.array(img.convert('RGBA'))
    alpha = arr[:, :, 3]
    mask = alpha > 40
    if mask.sum() == 0:
        return img
    labels, num = ndimage.label(mask)
    sizes = ndimage.sum(mask, labels, range(1, num + 1))
    if num <= 1:
        return img
    keep = np.argmax(sizes) + 1
    arr[labels != keep] = (0, 0, 0, 0)
    return Image.fromarray(arr)


def remove_bg_by_color(img):
    """Всё, что близко к цвету фона (тёмный ровный), -> прозрачное."""
    arr = np.array(img.convert('RGB')).astype(np.int16)
    h, w, _ = arr.shape
    # цвет фона = средний по углам
    bg = np.array([arr[2, 2], arr[2, w - 3], arr[h - 3, 2], arr[h - 3, w - 3]]).mean(axis=0)
    # расстояние каждого пикселя до bg
    diff = np.abs(arr - bg).sum(axis=2)
    mask = diff <= BG_TOL  # фон
    out = np.array(img.convert('RGBA'))
    out[mask] = (0, 0, 0, 0)
    return Image.fromarray(out)


def crop_to_content(img):
    bbox = img.getbbox()
    if bbox:
        return img.crop(bbox)
    return img


def normalize_orientation(img):
    """Развернуть корабль: горизонтальный, нос вправо."""
    arr = np.array(img.convert('RGBA'))
    alpha = arr[:, :, 3]
    ys, xs = np.where(alpha > 40)
    if len(xs) == 0:
        return img
    w = xs.max() - xs.min()
    h = ys.max() - ys.min()
    if w < h:
        # вертикальный -> повернуть
        img = img.rotate(-90, expand=True)
        arr = np.array(img.convert('RGBA'))
        alpha = arr[:, :, 3]
        ys, xs = np.where(alpha > 40)
    # теперь горизонтальный: нос = сторона с меньшей массой на концах
    left = alpha[:, :xs.min() + (xs.max() - xs.min()) // 3].sum()
    right = alpha[:, xs.min() + 2 * (xs.max() - xs.min()) // 3:].sum()
    if left < right:
        # нос уже справа? нет: если слева масса меньше, значит нос слева -> отразить
        img = ImageOps.mirror(img)
    return img


def fit_canvas(img, canvas=CANVAS):
    return ImageOps.pad(img, (canvas, canvas), color=(0, 0, 0, 0), centering=(0.5, 0.5))


def main(in_path, out_path):
    img = Image.open(in_path)
    img = remove_bg_by_color(img)
    img = keep_largest_component(img)
    img = crop_to_content(img)
    bbox = img.getbbox()
    if bbox:
        l, t, r, b = bbox
        img = img.crop((max(0, l - PAD), max(0, t - PAD), min(img.width, r + PAD), min(img.height, b + PAD)))
    img = normalize_orientation(img)
    img = fit_canvas(img, CANVAS)
    img.save(out_path, 'PNG')
    print('Saved: %s (%dx%d)' % (out_path, img.size[0], img.size[1]))


if __name__ == '__main__':
    main(sys.argv[1], sys.argv[2])