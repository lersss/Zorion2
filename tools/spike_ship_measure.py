# -*- coding: utf-8 -*-
# СПАЙК (§12.1): замеры кандидатов/силуэтов — уникальные цвета, плотность краёв
# (градиент > 25), масса вправо от центра bbox, заполнение bbox по Y.
# Только Python, read-only.
import argparse
import glob
import json
import os

import numpy as np
from PIL import Image


def metrics(path):
    im = Image.open(path)
    rgba = np.array(im.convert('RGBA')).astype(np.int16)
    rgb = rgba[:, :, :3]
    h, w, _ = rgb.shape
    if im.mode == 'RGBA' or 'transparency' in im.info or im.mode == 'P':
        mask = rgba[:, :, 3] > 40
    else:
        bg = np.array([rgb[2, 2], rgb[2, w - 3], rgb[h - 3, 2], rgb[h - 3, w - 3]]).mean(axis=0)
        mask = np.abs(rgb - bg).sum(axis=2) > 30
    if mask.sum() == 0:
        return None
    px = rgb[mask]
    colors = len(np.unique(px.reshape(-1, 3), axis=0))
    gray = rgb.mean(axis=2)
    gx = np.zeros_like(gray)
    gy = np.zeros_like(gray)
    gx[:, 1:-1] = gray[:, 2:] - gray[:, :-2]
    gy[1:-1, :] = gray[2:, :] - gray[:-2, :]
    grad = np.sqrt(gx * gx + gy * gy)
    # плотность краёв — доля граничных пикселей ОТ ВСЕГО холста (методика спеки
    # §2: эталон 21 спрайта ≈ 0.091, P10 0.045)
    edges = float((grad > 25).sum()) / float(h * w)
    ys, xs = np.nonzero(mask)
    bx0, bx1, by0, by1 = xs.min(), xs.max(), ys.min(), ys.max()
    cx = (bx0 + bx1) / 2.0
    mass_right = float((xs > cx).sum()) / len(xs)
    y_fill = float(by1 - by0 + 1) / h
    aspect = float(bx1 - bx0 + 1) / max(1, by1 - by0 + 1)
    return {
        'file': os.path.basename(path), 'w': w, 'h': h,
        'colors': colors, 'edges': round(edges, 4),
        'mass_right': round(mass_right, 3), 'y_fill': round(y_fill, 3),
        'aspect': round(aspect, 2),
    }


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('paths', nargs='+')
    args = ap.parse_args()
    rows = []
    for pat in args.paths:
        for p in sorted(glob.glob(pat)):
            m = metrics(p)
            if m:
                rows.append(m)
    print(json.dumps(rows, ensure_ascii=False, indent=1))


if __name__ == '__main__':
    main()
