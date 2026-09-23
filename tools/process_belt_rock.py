# -*- coding: utf-8 -*-
# Постобработка спрайтов астероидов пояса (пилот, art_belt_asteroids.md §3.6).
# Переиспользует функции tools/process_ship.py, добавляя ЗАЛИВКУ ВНУТРЕННИХ ДЫР:
# у камня тёмные трещины/вмятины местами так же черны, как фон, и вырез по цвету
# оставляет в теле сквозные дырки. Дырки, не связанные с внешним фоном, заливаем
# цветом ближайшего непрозрачного пикселя.
# Финальный размер 256x256, RGBA, прозрачный фон, без нормализации поворота (--no-orient).
# Использование: python process_belt_rock.py <in.png> <out.png> [--canvas N]
import os, sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import numpy as np
from PIL import Image
from scipy import ndimage
import process_ship as ps

CANVAS = 256
PAD = 5
if '--canvas' in sys.argv:
    CANVAS = int(sys.argv[sys.argv.index('--canvas') + 1])
if '--pad' in sys.argv:
    PAD = int(sys.argv[sys.argv.index('--pad') + 1])


def fill_holes(img):
    """Внутренние прозрачные дырки (не связанные с фоном) — цветом ближайшего
    непрозрачного пикселя. Внешний фон не трогаем."""
    arr = np.array(img.convert('RGBA'))
    opaque = arr[:, :, 3] > 40
    if opaque.all() or not opaque.any():
        return img
    inv = ~opaque
    labels, _ = ndimage.label(inv)
    border = set(np.unique(np.concatenate([labels[0, :], labels[-1, :],
                                           labels[:, 0], labels[:, -1]])))
    border.discard(0)
    holes = inv & ~np.isin(labels, list(border)) if border else inv
    if not holes.any():
        return img
    _, ind = ndimage.distance_transform_edt(inv, return_indices=True)
    arr[holes] = arr[ind[0][holes], ind[1][holes]]
    return Image.fromarray(arr)


def main(in_path, out_path):
    img = Image.open(in_path)
    img = ps.remove_bg_by_color(img)
    img = ps.keep_largest_component(img)
    img = fill_holes(img)
    img = ps.crop_to_content(img)
    bbox = img.getbbox()
    if bbox:
        l, t, r, b = bbox
        img = img.crop((max(0, l - PAD), max(0, t - PAD),
                        min(img.width, r + PAD), min(img.height, b + PAD)))
    img = ps.smooth_edges(img, 2)
    img = ps.fit_canvas(img, CANVAS)
    img.save(out_path, 'PNG')
    print('Saved: %s (%dx%d)' % (out_path, img.size[0], img.size[1]))


if __name__ == '__main__':
    main(sys.argv[1], sys.argv[2])
