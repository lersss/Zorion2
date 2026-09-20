# -*- coding: utf-8 -*-
# Метрика «низ однотонный» по всей пачке: std яркости и число цветов в нижних 40% кадра.
import os, sys
sys.stdout.reconfigure(encoding='utf-8', errors='replace')
import numpy as np
from PIL import Image

OUT = r'C:\Zorion2\ai_drafts\biomes\processed_p1B'
rows = []
for f in sorted(os.listdir(OUT)):
    if not f.endswith('.png') or f.endswith('_64.png'):
        continue
    im = Image.open(os.path.join(OUT, f))
    rgb = np.array(im.convert('RGB')).astype(np.int16)
    h, w = rgb.shape[:2]
    bottom = rgb[int(h * 0.6):, :]
    bmean = bottom.mean(axis=2)
    std = bmean.std()
    cols = len(np.unique(bottom.reshape(-1, 3), axis=0))
    rows.append((std, cols, f))

print('%-30s %8s %8s %s' % ('id', 'std', 'colors', 'status'))
for std, cols, f in sorted(rows):
    status = 'OK' if (std > 6 and cols > 40) else 'FLAT'
    print('%-30s %8.1f %8d %s' % (f, std, cols, status))