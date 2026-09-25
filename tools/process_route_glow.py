# -*- coding: utf-8 -*-
# Постобработка светящихся спрайтов маршрута (star_core): альфа ПО ЯРКОСТИ
# (black -> alpha), НЕ вырез по цвету/rembg (ТЗ §4: rembg портит светящиеся объекты).
# Выход: RGBA PNG, чисто белый RGB (тинт/температуру накладывает код) + alpha = яркость.
# Кадрирование по светлому ядру, масштаб до ~88% холста (запас >=6% от края), центр.
# --full: полнокадровая текстура (туман/туманность) — без кадрирования и центрирования,
#         только масштаб до холста; края не обрезаются (маскирует/тинтует код).
# Использование: python process_route_glow.py <in.png> <out.png> [--canvas N] [--lo N] [--hi N] [--full]
import sys
import numpy as np
from PIL import Image, ImageOps

CANVAS = 512
TARGET = 0.88
LO = 34.0     # порог «чёрного» фона (Juggernaut даёт ~26-29, не 0)
HI = 210.0    # яркость, дающая полную непрозрачность
GAMMA = 0.9
FULL = False

if '--canvas' in sys.argv:
    CANVAS = int(sys.argv[sys.argv.index('--canvas') + 1])
if '--lo' in sys.argv:
    LO = float(sys.argv[sys.argv.index('--lo') + 1])
if '--hi' in sys.argv:
    HI = float(sys.argv[sys.argv.index('--hi') + 1])
if '--full' in sys.argv:
    FULL = True


def main(in_path, out_path):
    img = Image.open(in_path).convert('RGB')
    a = np.asarray(img).astype(np.float32)
    lum = 0.299 * a[:, :, 0] + 0.587 * a[:, :, 1] + 0.114 * a[:, :, 2]
    x = np.clip((lum - LO) / (HI - LO), 0.0, 1.0) ** GAMMA
    alpha = (x * 255).astype(np.uint8)
    alpha[alpha < 16] = 0
    white = np.full_like(alpha, 255)
    rgba = np.dstack([white, white, white, alpha])
    out = Image.fromarray(rgba, 'RGBA')

    if FULL:
        out = out.resize((CANVAS, CANVAS), Image.LANCZOS)
        out.save(out_path, 'PNG')
        print('Saved: %s (%dx%d, full)' % (out_path, out.size[0], out.size[1]))
        return

    # bbox по заметной яркости (мягкое гало не обрезаем — только шум фона)
    solid = alpha > 24
    if solid.any():
        ys, xs = np.nonzero(solid)
        out = out.crop((int(xs.min()), int(ys.min()), int(xs.max()) + 1, int(ys.max()) + 1))

    w, h = out.size
    s = (TARGET * CANVAS) / max(w, h)
    out = out.resize((max(1, round(w * s)), max(1, round(h * s))), Image.LANCZOS)
    out = ImageOps.pad(out, (CANVAS, CANVAS), color=(0, 0, 0, 0), centering=(0.5, 0.5))
    out.save(out_path, 'PNG')
    print('Saved: %s (%dx%d)' % (out_path, out.size[0], out.size[1]))


if __name__ == '__main__':
    main(sys.argv[1], sys.argv[2])
