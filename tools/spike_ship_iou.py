# -*- coding: utf-8 -*-
# СПАЙК (§12.1): IoU формы «кандидат (альфа-маска) vs силуэт» — количественная
# проверка «ControlNet держит форму». Кроп bbox -> 256x256 -> IoU. Read-only.
import sys

import numpy as np
from PIL import Image


def shape_mask(path):
    im = Image.open(path)
    a = np.array(im.convert('RGBA'))
    h, w, _ = a.shape
    if a[:, :, 3].min() < 255:
        m = a[:, :, 3] > 40
    else:
        rgb = a[:, :, :3].astype(np.int16)
        bg = np.array([rgb[2, 2], rgb[2, w - 3], rgb[h - 3, 2], rgb[h - 3, w - 3]]).mean(axis=0)
        m = np.abs(rgb - bg).sum(axis=2) > 30
    ys, xs = np.nonzero(m)
    m = m[ys.min():ys.max() + 1, xs.min():xs.max() + 1]
    img = Image.fromarray((m * 255).astype(np.uint8)).resize((256, 256), Image.NEAREST)
    return np.array(img) > 127


def iou(a, b):
    return float((a & b).sum()) / float((a | b).sum())


if __name__ == '__main__':
    pairs = [sys.argv[i:i + 2] for i in range(1, len(sys.argv), 2)]
    for cand, sil in pairs:
        print('%s vs %s: IoU=%.3f' % (cand.split('\\')[-1], sil.split('\\')[-1],
                                      iou(shape_mask(cand), shape_mask(sil))))
