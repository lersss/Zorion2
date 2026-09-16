# -*- coding: utf-8 -*-
import sys
from PIL import Image
import numpy as np

path = sys.argv[1]
img = Image.open(path).convert('RGB')
arr = np.array(img)
hsv = np.array(img.convert('HSV'))
flat = arr.reshape(-1, 3).mean(axis=1).reshape(arr.shape[:2])
mask = flat > 90
ys, xs = np.where(mask)
if len(xs) == 0:
    print('no object')
    sys.exit()
w = xs.max() - xs.min()
h = ys.max() - ys.min()
print('ratio: %.2f' % (w / h))
for label, xf in [('нос', 0.95), ('перед', 0.75), ('центр', 0.5), ('корма', 0.25), ('хвост', 0.05)]:
    x = int(xs.min() + w * xf)
    zm = mask.copy()
    zm[:, :x - 15] = False
    zm[:, x + 15:] = False
    if zm.sum() > 200:
        rgb = arr[zm].reshape(-1, 3).mean(axis=0).astype(int)
        h_mean = hsv[zm].reshape(-1, 3)[:, 0].mean()
        print('%s: RGB[%d,%d,%d] H=%.0f' % (label, rgb[0], rgb[1], rgb[2], h_mean))
    else:
        print('%s: (пусто)' % label)
# верх/низ (крылья)
for label, yf in [('верх(крыло)', 0.1), ('низ(крыло)', 0.9)]:
    y = int(ys.min() + h * yf)
    zm = mask.copy()
    zm[:y - 15, :] = False
    zm[y + 15:, :] = False
    if zm.sum() > 200:
        rgb = arr[zm].reshape(-1, 3).mean(axis=0).astype(int)
        print('%s: RGB[%d,%d,%d]' % (label, rgb[0], rgb[1], rgb[2]))