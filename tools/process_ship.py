# -*- coding: utf-8 -*-
# Обработка кораблей: rembg (прозрачный фон) + кадрирование + ресайз 200x200 + нормализация ориентации (нос вправо)
import os, sys
from PIL import Image, ImageOps
import numpy as np
from rembg import remove

CANVAS = 200
PAD = 6


def remove_bg(img):
    out = remove(img)  # RGBA, фон прозрачный
    return out


def crop_to_content(img):
    bbox = img.getbbox()
    if bbox:
        return img.crop(bbox)
    return img


def orientation_side(img):
    """Определить, где 'нос' (максимально вытянутая сторона). Возвращает 'left'/'right'/'up'/'down'."""
    arr = np.array(img.convert('RGBA'))
    alpha = arr[:, :, 3]
    ys, xs = np.where(alpha > 40)
    if len(xs) == 0:
        return None
    # распределение массы по горизонтали: если масса смещена вправо -> нос вправо
    # Также используем ширину: корабль должен быть горизонтален.
    return 'horizontal' if (xs.max() - xs.min()) >= (ys.max() - ys.min()) else 'vertical'


def normalize_orientation(img):
    """Развернуть корабль так, чтобы нос смотрел вправо (по вытянутой оси + масса)."""
    arr = np.array(img.convert('RGBA'))
    alpha = arr[:, :, 3]
    ys, xs = np.where(alpha > 40)
    if len(xs) == 0:
        return img
    w = xs.max() - xs.min()
    h = ys.max() - ys.min()

    # Определяем ось и направление
    if w >= h:
        # горизонтальный: масса вправо = нос вправо. Если масса слева - отразить.
        left_mass = alpha[:, :xs.min() + w // 2].sum()
        right_mass = alpha[:, xs.min() + w // 2:].sum()
        if left_mass > right_mass:
            img = ImageOps.mirror(img)
    else:
        # вертикальный: крутим. Нос вверх -> повернуть на 90 по часовой (вправо).
        top_mass = alpha[:ys.min() + h // 2, :].sum()
        bot_mass = alpha[ys.min() + h // 2:, :].sum()
        # нос тот, где меньше масса (сужающаяся часть) - для корабля нос узкий, корма широкая
        if top_mass < bot_mass:
            # нос сверху -> повернуть на 90 по часовой -> нос вправо
            img = img.rotate(-90, expand=True)
        else:
            # нос снизу -> повернуть на 270 по часовой -> нос вправо
            img = img.rotate(90, expand=True)
    return img


def fit_canvas(img, canvas=CANVAS):
    return ImageOps.pad(img, (canvas, canvas), color=(0, 0, 0, 0), centering=(0.5, 0.5))


def main(in_path, out_path):
    img = Image.open(in_path).convert('RGBA')
    img = remove_bg(img)
    img = crop_to_content(img)
    # pad вокруг корабля
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
