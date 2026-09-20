# -*- coding: utf-8 -*-
# Проверка №17-20: нижняя половина не однотонная (std яркости, число цветов в нижних 40%),
# + покрытие, + небесные тела в верхних 45%.
import os, sys
sys.stdout.reconfigure(encoding='utf-8', errors='replace')
import numpy as np
from PIL import Image
from scipy import ndimage

OUT = r'C:\Zorion2\ai_drafts\biomes\processed_p1B'
names = ['вулканические_поля.png', 'стеклянные_поля.png', 'кристальные_рощи.png', 'кремниевые_рощи.png']

for name in names:
    im = Image.open(os.path.join(OUT, name))
    rgb = np.array(im.convert('RGB')).astype(np.int16)
    alpha = np.array(im)[:, :, 3]
    h, w = rgb.shape[:2]
    # нижние 40% кадра
    y0 = int(h * 0.6)
    bottom = rgb[y0:, :]
    bmean = bottom.mean(axis=2)
    std = bmean.std()
    cols = len(np.unique(bottom.reshape(-1, 3), axis=0))
    # покрытие и непрозрачность
    ys, xs = np.where(alpha > 40)
    cov = ((xs.max() - xs.min() + 1) / w, (ys.max() - ys.min() + 1) / h) if len(xs) else (0, 0)
    opaque = (alpha > 40).mean()
    # небесные тела: яркие округлые пятна в верхних 45%
    sky = (rgb.mean(axis=2) > 170)[:int(h * 0.45), :]
    celestial = 0
    if sky.sum() > 0:
        labels, num = ndimage.label(sky)
        for lab in range(1, num + 1):
            ys2, xs2 = np.where(labels == lab)
            area = len(xs2)
            if area < 12 or area > 300:
                continue
            ww = xs2.max() - xs2.min() + 1
            hh = ys2.max() - ys2.min() + 1
            if ww >= 2 and hh >= 2 and 0.6 < ww / hh < 1.8 and ys2.mean() < int(h * 0.45) * 0.92 and ys2.min() > 1:
                celestial += 1
    status_bottom = 'OK' if (std > 6 and cols > 40) else 'FLAT?'
    status_cel = 'OK' if celestial == 0 else 'FLAG %d' % celestial
    print('%-26s bottom: std=%.1f colors=%d [%s]  cov=(%.2f,%.2f) opaque=%.2f  celestial=%s'
          % (name, std, cols, status_bottom, cov[0], cov[1], opaque, status_cel))