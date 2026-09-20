# -*- coding: utf-8 -*-
# Обработка ИКОНОК биомов: фон по цвету, компоненты >= доли от крупнейшего (мелкие акценты сохраняем,
# мусор убираем), сглаживание, ужим в 64x64 (или --size N), прозрачный фон. Без ориентации.
# Использование: python process_icon.py <in.png> <out.png> [--size 64] [--keep-frac 0.01] [--bg-dark N] [--smooth N]
import sys
from PIL import Image, ImageOps
import numpy as np
from scipy import ndimage

SIZE = 64
PAD = 2
BG_TOL = 40
KEEP_FRAC = 0.01

args = sys.argv[1:]
if '--size' in args:
    SIZE = int(args[args.index('--size') + 1])
if '--keep-frac' in args:
    KEEP_FRAC = float(args[args.index('--keep-frac') + 1])
SMOOTH = 1
if '--smooth' in args:
    SMOOTH = int(args[args.index('--smooth') + 1])
SCALE = 0.9
if '--scale' in args:
    SCALE = float(args[args.index('--scale') + 1])
BG_DARK = None
if '--bg-dark' in args:
    BG_DARK = int(args[args.index('--bg-dark') + 1])


def remove_bg_by_color(img):
    arr = np.array(img.convert('RGB')).astype(np.int16)
    h, w, _ = arr.shape
    bg = np.array([arr[2, 2], arr[2, w - 3], arr[h - 3, 2], arr[h - 3, w - 3]]).mean(axis=0)
    diff = np.abs(arr - bg).sum(axis=2)
    mask = diff <= BG_TOL
    out = np.array(img.convert('RGBA'))
    out[mask] = (0, 0, 0, 0)
    return Image.fromarray(out)


def remove_bg_by_dark(img, threshold):
    arr = np.array(img.convert('RGB')).astype(np.int16)
    mask = arr.mean(axis=2) <= threshold
    out = np.array(img.convert('RGBA'))
    out[mask] = (0, 0, 0, 0)
    return Image.fromarray(out)


def keep_large_components(img, frac=KEEP_FRAC):
    """Оставить компоненты размером >= frac от крупнейшего (мелкие акценты иконки сохраняются)."""
    arr = np.array(img.convert('RGBA'))
    alpha = arr[:, :, 3]
    mask = alpha > 40
    if mask.sum() == 0:
        return img
    labels, num = ndimage.label(mask)
    sizes = ndimage.sum(mask, labels, range(1, num + 1))
    if num <= 1:
        return img
    biggest = sizes.max()
    keep_labels = [i + 1 for i, s in enumerate(sizes) if s >= frac * biggest]
    arr[np.isin(labels, keep_labels, invert=True)] = (0, 0, 0, 0)
    return Image.fromarray(arr)


def smooth_edges(img, radius=1):
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


def fit_canvas(img, size):
    return ImageOps.pad(img, (size, size), color=(0, 0, 0, 0), centering=(0.5, 0.5))


def main(in_path, out_path):
    img = Image.open(in_path)
    if BG_DARK is not None:
        img = remove_bg_by_dark(img, BG_DARK)
    else:
        img = remove_bg_by_color(img)
    img = keep_large_components(img)
    img = crop_to_content(img)
    bbox = img.getbbox()
    if bbox:
        l, t, r, b = bbox
        img = img.crop((max(0, l - PAD), max(0, t - PAD), min(img.width, r + PAD), min(img.height, b + PAD)))
    img = smooth_edges(img, SMOOTH)
    # квадрат с прозрачными полями (пропорции сохраняются), контент ~SCALE от кадра, затем ужим
    side = int(max(img.size) / SCALE)
    img = ImageOps.pad(img, (side, side), color=(0, 0, 0, 0), centering=(0.5, 0.5))
    img = img.resize((SIZE, SIZE), Image.LANCZOS)
    img.save(out_path, 'PNG')
    print('Saved: %s (%dx%d)' % (out_path, img.size[0], img.size[1]))


if __name__ == '__main__':
    main(args[0], args[1])