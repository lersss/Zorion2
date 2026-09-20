# -*- coding: utf-8 -*-
# Обработка кораблей Juggernaut XL: вырезание фона ПО ЦВЕТУ (фон ровный тёмный ~21,23,26),
# кадрирование, вписывание в 200x200, нормализация ориентации (нос вправо), сглаживание границ.
# Использование: python process_ship.py <in.png> <out.png> [--no-orient] [--smooth N] [--canvas N]
import sys
from PIL import Image, ImageOps
import numpy as np
from scipy import ndimage

CANVAS = 200
PAD = 6
BG_TOL = 40  # допуск расстояния до цвета фона

# --canvas N: финальный размер кандидата (200 — полный, 100 — эскиз 98c:
# быстрее и легче, ловит форму/стиль до полного прогона).
if '--canvas' in sys.argv:
    i = sys.argv.index('--canvas')
    CANVAS = int(sys.argv[i + 1])

NO_ORIENT = '--no-orient' in sys.argv
SMOOTH = 2
# --bg-dark N: вырезать фон ПО ЯРКОСТИ (всё, что темнее порога N) вместо цвета углов.
# Нужно для живописных аватаров, где фон — тёмный градиент, а не ровный цвет.
BG_DARK = None
if '--bg-dark' in sys.argv:
    i = sys.argv.index('--bg-dark')
    BG_DARK = int(sys.argv[i + 1])

# --keep-warm: НЕ вырезать «тёплые» пиксели (кожа/лицо), даже если близки к фону.
# Решает вырезание одежды цвета фона у портретов людей.
KEEP_WARM = '--keep-warm' in sys.argv


def is_warm_rgb(r, g, b):
    """Тёплый тон (кожа/лицо): R > B и R заметно доминирует. Массив-безопасно."""
    return (r - b > 18) & (r > 60)


def remove_bg_by_color(img):
    """Всё, что близко к цвету фона (тёмный ровный), -> прозрачное."""
    arr = np.array(img.convert('RGB')).astype(np.int16)
    h, w, _ = arr.shape
    bg = np.array([arr[2, 2], arr[2, w - 3], arr[h - 3, 2], arr[h - 3, w - 3]]).mean(axis=0)
    diff = np.abs(arr - bg).sum(axis=2)
    mask = diff <= BG_TOL
    out = np.array(img.convert('RGBA'))
    out[mask] = (0, 0, 0, 0)
    return Image.fromarray(out)


def remove_bg_by_dark(img, threshold):
    """Всё, что темнее порога яркости, -> прозрачное (фон-градиент)."""
    arr = np.array(img.convert('RGB')).astype(np.int16)
    brightness = arr.mean(axis=2)
    mask = brightness <= threshold
    if KEEP_WARM:
        r, g, b = arr[:, :, 0], arr[:, :, 1], arr[:, :, 2]
        mask = mask & ~is_warm_rgb(r, g, b)
    out = np.array(img.convert('RGBA'))
    out[mask] = (0, 0, 0, 0)
    return Image.fromarray(out)


def remove_bg_with_keep_warm(img):
    """Как remove_bg_by_color, но НЕ вырезает тёплые пиксели (кожу/лицо)."""
    arr = np.array(img.convert('RGB')).astype(np.int16)
    h, w, _ = arr.shape
    bg = np.array([arr[2, 2], arr[2, w - 3], arr[h - 3, 2], arr[h - 3, w - 3]]).mean(axis=0)
    diff = np.abs(arr - bg).sum(axis=2)
    mask = diff <= BG_TOL
    if KEEP_WARM:
        r, g, b = arr[:, :, 0], arr[:, :, 1], arr[:, :, 2]
        mask = mask & ~is_warm_rgb(r, g, b)
    out = np.array(img.convert('RGBA'))
    out[mask] = (0, 0, 0, 0)
    return Image.fromarray(out)


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


def smooth_edges(img, radius=2):
    """Сгладить границы силуэта: закрытие (дыры) + открытие (зубцы)."""
    arr = np.array(img.convert('RGBA'))
    alpha = arr[:, :, 3]
    mask = alpha > 40
    if mask.sum() == 0:
        return img
    closed = ndimage.binary_closing(mask, structure=np.ones((radius * 2 + 1, radius * 2 + 1)))
    opened = ndimage.binary_opening(closed, structure=np.ones((radius + 1, radius + 1)))
    arr[:, :, 3] = np.where(opened, 255, 0)
    return Image.fromarray(arr)


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
        img = img.rotate(-90, expand=True)
        arr = np.array(img.convert('RGBA'))
        alpha = arr[:, :, 3]
        ys, xs = np.where(alpha > 40)
    left = alpha[:, :xs.min() + (xs.max() - xs.min()) // 3].sum()
    right = alpha[:, xs.min() + 2 * (xs.max() - xs.min()) // 3:].sum()
    if left < right:
        img = ImageOps.mirror(img)
    return img


def fit_canvas(img, canvas=CANVAS):
    return ImageOps.pad(img, (canvas, canvas), color=(0, 0, 0, 0), centering=(0.5, 0.5))


def main(in_path, out_path):
    img = Image.open(in_path)
    if BG_DARK is not None:
        img = remove_bg_by_dark(img, BG_DARK)
    elif KEEP_WARM:
        img = remove_bg_with_keep_warm(img)
    else:
        img = remove_bg_by_color(img)
    img = keep_largest_component(img)
    img = crop_to_content(img)
    bbox = img.getbbox()
    if bbox:
        l, t, r, b = bbox
        img = img.crop((max(0, l - PAD), max(0, t - PAD), min(img.width, r + PAD), min(img.height, b + PAD)))
    if not NO_ORIENT:
        img = normalize_orientation(img)
    img = smooth_edges(img, SMOOTH)
    img = fit_canvas(img, CANVAS)
    img.save(out_path, 'PNG')
    print('Saved: %s (%dx%d)' % (out_path, img.size[0], img.size[1]))


if __name__ == '__main__':
    main(sys.argv[1], sys.argv[2])