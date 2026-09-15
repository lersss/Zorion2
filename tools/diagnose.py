# -*- coding: utf-8 -*-
from PIL import Image
import os

src = r'C:\Zorion2\web\static\sprites'
for name in ['organic', 'military', 'sleek', 'industrial']:
    raw = os.path.join(src, 'ship_%s_ai_raw.png' % name)
    if not os.path.exists(raw):
        raw = os.path.join(src, 'ship_%s_ai.png' % name)
    img = Image.open(raw).convert('RGB')
    w, h = img.size
    px = img.load()
    corners = [px[0, 0], px[w - 1, 0], px[0, h - 1], px[w - 1, h - 1], px[w // 2, 0], px[0, h // 2], px[w - 1, h // 2]]
    min_x, min_y, max_x, max_y = w, h, -1, -1
    for y in range(0, h, 2):
        for x in range(0, w, 2):
            r, g, b = px[x, y]
            if r > 60 or g > 60 or b > 60 or (abs(r - g) > 30 or abs(r - b) > 30 or abs(g - b) > 30):
                if x < min_x: min_x = x
                if x > max_x: max_x = x
                if y < min_y: min_y = y
                if y > max_y: max_y = y
    print('%s: size=%dx%d corners=%s' % (name, w, h, corners))
    print('  bbox obj: x[%d..%d] y[%d..%d]  (w=%d, h=%d)' % (min_x, max_x, min_y, max_y, max_x - min_x, max_y - min_y))
    if max_x < 0:
        print('  NO OBJECT FOUND')
