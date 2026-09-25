# -*- coding: utf-8 -*-
# Постобработка спрайта лодки прогулки (ЧК6.3). ТЗ: art_surface_boat.md §4.5.
#
# Отличие от process_ship.py / process_belt_rock.py: НЕ кроп по bbox + центрирование
# в квадрат (это сломало бы точную линию воды). Режем РОВНО фиксированную рамку R
# (карту кадра) и масштабируем в 204x120, поэтому вода садится в y=84.
#
# Использование:
#   python process_surface_boat.py <in.png> <out.png> [--frame x0,y0,x1,y1] [--tol N]
import os, sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import numpy as np
from PIL import Image
from scipy import ndimage
import process_ship as ps

DEFAULT_FRAME = (36, 232, 988, 792)  # R в холсте 1024 (аспект 34:20)
OUT_W, OUT_H = 204, 120
WATER_Y = 84
BASE_SRC = 1024.0

frame = DEFAULT_FRAME
if '--frame' in sys.argv:
    frame = tuple(int(t) for t in sys.argv[sys.argv.index('--frame') + 1].split(','))
tol = 40
if '--tol' in sys.argv:
    tol = int(sys.argv[sys.argv.index('--tol') + 1])


def fill_holes(img):
    """Внутренние прозрачные дырки (не связанные с фоном) — цветом ближайшего
    непрозрачного пикселя. Тёмное ложе/швы не должны стать сквозными."""
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
    ps.BG_TOL = tol
    img = ps.remove_bg_by_color(img)
    img = ps.keep_largest_component(img, keep_parts=True)
    img = fill_holes(img)
    img = ps.smooth_edges(img, 2)

    # Рез ровно рамки R (при Hi-Res координаты масштабируются пропорционально).
    sx = img.width / BASE_SRC
    x0, y0, x1, y1 = frame
    box = (int(round(x0 * sx)), int(round(y0 * sx)),
           int(round(x1 * sx)), int(round(y1 * sx)))
    img = img.crop(box)
    img = img.resize((OUT_W, OUT_H), Image.LANCZOS)  # --no-orient: нос вправо задан силуэтом

    d = os.path.dirname(out_path)
    if d:
        os.makedirs(d, exist_ok=True)
    img.save(out_path, 'PNG')

    arr = np.array(img.convert('RGBA'))
    water = int((arr[WATER_Y, :, 3] > 40).sum())
    ys = np.where(arr[:, :, 3] > 40)[0]
    bottom = int(ys.max()) if ys.size else -1
    print('Saved: %s (%dx%d) water_y=%d opaque=%d bottom_row=%d'
          % (out_path, img.size[0], img.size[1], WATER_Y, water, bottom))
    if img.size != (OUT_W, OUT_H):
        print('WARN: size != %dx%d' % (OUT_W, OUT_H))
    if water < 3:
        print('WARN: no hull pixels on water line y=%d' % WATER_Y)


if __name__ == '__main__':
    main(sys.argv[1], sys.argv[2])
