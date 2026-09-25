# -*- coding: utf-8 -*-
# Постобработка несветящихся спрайтов маршрута (beacon / false_signal): ЖЁСТКИЙ вырез
# фона (ТЗ §4: маяки/ложные — чёткий силуэт). Переиспользует функции tools/process_ship.py.
# Выход: RGBA PNG, финальный 128x128, объект ~86% холста (запас >=6% от края), центр,
# без нормализации поворота (у маяка «носа» нет).
# Использование: python process_route_icon.py <in.png> <out.png> [--canvas N]
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from PIL import Image, ImageOps
import process_ship as ps

CANVAS = 128
TARGET = 0.86
PAD = 5

if '--canvas' in sys.argv:
    CANVAS = int(sys.argv[sys.argv.index('--canvas') + 1])


def main(in_path, out_path):
    img = Image.open(in_path)
    img = ps.remove_bg_by_color(img)
    img = ps.keep_largest_component(img, keep_parts=True)
    img = ps.crop_to_content(img)
    bbox = img.getbbox()
    if bbox:
        l, t, r, b = bbox
        img = img.crop((max(0, l - PAD), max(0, t - PAD),
                        min(img.width, r + PAD), min(img.height, b + PAD)))
    img = ps.smooth_edges(img, 2)
    w, h = img.size
    s = (TARGET * CANVAS) / max(w, h)
    img = img.resize((max(1, round(w * s)), max(1, round(h * s))), Image.LANCZOS)
    img = ImageOps.pad(img, (CANVAS, CANVAS), color=(0, 0, 0, 0), centering=(0.5, 0.5))
    img.save(out_path, 'PNG')
    print('Saved: %s (%dx%d)' % (out_path, img.size[0], img.size[1]))


if __name__ == '__main__':
    main(sys.argv[1], sys.argv[2])
