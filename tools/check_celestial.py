# -*- coding: utf-8 -*-
# Эвристическая проверка «небесных тел»: ищем в верхних 45% кадра изолированные яркие округлые пятна
# (луна/солнце/планета/звезда-блик). Облака обычно крупнее и нерегулярные — не должны флагаться.
import os, sys
sys.stdout.reconfigure(encoding='utf-8', errors='replace')
import numpy as np
from PIL import Image
from scipy import ndimage

D = r'C:\Zorion2\ai_drafts\biomes\batch_p1B'
H, W = 1024, 1024
SKY_H = int(H * 0.45)  # верхние 45% — «небо»

flags = []
for f in sorted(os.listdir(D)):
    if not f.endswith('.png'):
        continue
    a = np.array(Image.open(os.path.join(D, f)).convert('RGB')).astype(np.int16)
    bright = a.mean(axis=2) > 170
    sky = bright[:SKY_H, :]
    if sky.sum() == 0:
        continue
    labels, num = ndimage.label(sky)
    for lab in range(1, num + 1):
        ys, xs = np.where(labels == lab)
        area = len(xs)
        if area < 800 or area > 45000:
            continue
        w = xs.max() - xs.min() + 1
        h = ys.max() - ys.min() + 1
        if w < 8 or h < 8:
            continue
        aspect = w / h
        cy = ys.mean()
        # округлый + центр выше 45% кадра + не прижат к верху (не полоса неба)
        if 0.6 < aspect < 1.8 and cy < SKY_H * 0.92 and ys.min() > 20:
            flags.append((f, lab, area, round(aspect, 2), (xs.min(), ys.min(), xs.max(), ys.max())))

print('celestial-body flags: %d' % len(flags))
for fl in flags:
    print('  %s: comp=%d area=%d aspect=%s bbox=%s' % fl)
if not flags:
    print('NONE — небо чистое (по эвристике)')