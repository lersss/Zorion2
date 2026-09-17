# -*- coding: utf-8 -*-
# segment_face.py — программная сегментация ЛИЦА для txt2img-портретов людей.
# Фон у живописных портретов градиентный (process_ship не режет), поэтому:
# 1) маска КОЖИ: тёплые тона (HSV: hue 5-25, sat > 25, val > 45)
# 2) маска ВОЛОС: тёмные пиксели ВЫШЕ центра лица (val < 70)
# 3) расширение (dilation) + крупнейший связный компонент + сглаживание.
# Использование: python segment_face.py <in.png> <out.png> [--tune]
import sys
import numpy as np
from PIL import Image, ImageFilter
from scipy import ndimage


def mask_skin(img):
    """Маска кожи: HSV hue в диапазоне кожи."""
    hsv = np.array(img.convert('HSV')).astype(np.int16)
    h, s, v = hsv[:, :, 0], hsv[:, :, 1], hsv[:, :, 2]
    skin = (h > 3) & (h < 30) & (s > 30) & (v > 45)
    return skin


def mask_hair(img):
    """Маска волос: тёмные пиксели (верхняя половина кадра)."""
    a = np.array(img.convert('RGB')).astype(np.int16)
    v = a.mean(axis=2)
    h, w = v.shape
    dark = v < 80
    top_half = np.zeros_like(dark)
    top_half[:h // 2, :] = True
    return dark & top_half


def dilate(mask, r=6):
    return ndimage.binary_dilation(mask, structure=np.ones((r * 2 + 1, r * 2 + 1)))


def keep_largest(mask):
    labels, num = ndimage.label(mask)
    if num <= 1:
        return mask
    sizes = ndimage.sum(mask, labels, range(1, num + 1))
    keep = np.argmax(sizes) + 1
    return labels == keep


def main():
    inp, outp = sys.argv[1], sys.argv[2]
    img = Image.open(inp).convert('RGB')
    skin = mask_skin(img)
    hair = mask_hair(img)
    face = skin | hair
    face = keep_largest(face)
    face = dilate(face, r=5)
    face = ndimage.binary_fill_holes(face)
    face = ndimage.binary_opening(face, structure=np.ones((3, 3)))

    rgba = np.array(img.convert('RGBA'))
    rgba[~face] = (0, 0, 0, 0)
    out_img = Image.fromarray(rgba)
    # crop + pad 200
    bbox = out_img.getbbox()
    if bbox:
        out_img = out_img.crop(bbox)
    canvas = Image.new('RGBA', (200, 200), (0, 0, 0, 0))
    canvas.paste(out_img, ((200 - out_img.width) // 2, (200 - out_img.height) // 2), out_img)
    canvas.save(outp)
    a = np.array(canvas)
    m = a[:, :, 3] > 40
    ys, xs = np.where(m)
    print("Saved: %s  (прозрачных %.0f%%, bbox x=[%d,%d] y=[%d,%d])" % (
        outp, (a[:, :, 3] == 0).mean() * 100, xs.min(), xs.max(), ys.min(), ys.max()))


if __name__ == '__main__':
    main()