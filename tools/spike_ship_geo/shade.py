# -*- coding: utf-8 -*-
# Пост-эффекты рендера: AO, падающие тени, фаски, жёсткие линии на стыках.
# Спайк 4: размеры фильтров масштабируются под SIZE (1536); направление
# падающей тени — свет, повёрнутый на yaw (экранные оси). Порт спайка 3.
import math

import numpy as np
from PIL import Image
from scipy import ndimage

from .palette import LIGHT_WORLD, SIZE

SCALE = SIZE / 1024.0


def _odd(v):
    v = max(3, int(round(v)))
    return v if v % 2 == 1 else v + 1


def ambient_occlusion(zworld, fg):
    hmax = ndimage.maximum_filter(np.where(fg, zworld, -1e4), size=_odd(25 * SCALE))
    occ = np.clip(hmax - zworld, 0, None)
    ao = 1.0 - 0.62 * np.clip(occ / 55.0, 0, 1)
    ao = ndimage.gaussian_filter(ao, 1.0 * SCALE)
    return np.clip(ao, 0.42, 1.0)


def cast_shadow(zworld, fg, light_screen):
    """Тень от приподнятых деталей: ищем более высокую поверхность В СТОРОНУ света."""
    lx, ly = light_screen
    ln = math.hypot(lx, ly) or 1.0
    ux, uy = lx / ln, ly / ln
    src = np.where(fg, zworld, -1e4)
    excess = np.zeros((SIZE, SIZE), np.float64)
    step = max(2, int(round(4 * SCALE)))
    for k in range(3, int(round(70 * SCALE)), step):
        ox = int(round(ux * k))
        oy = int(round(uy * k))
        shifted = np.roll(np.roll(src, -oy, axis=0), -ox, axis=1)
        excess = np.maximum(excess, shifted - zworld)
    shadow = np.clip(excess / 45.0, 0, 1)
    return ndimage.gaussian_filter(shadow, 2.0 * SCALE)


def hard_creases(zworld, fg):
    """Маска перепадов высот (стыки деталей) — тонкая тёмная линия-панель."""
    src = np.where(fg, zworld, -1e4)
    zmax3 = ndimage.maximum_filter(src, size=3)
    zmin3 = ndimage.minimum_filter(np.where(fg, zworld, 1e4), size=3)
    return fg & ((zmax3 - zmin3) > 1.5)


def post_effects(rgba, zworld, tag, fg, light_screen):
    ao = ambient_occlusion(zworld, fg)
    rgba *= ao[..., None]
    shadow = cast_shadow(zworld, fg, light_screen)
    top = tag == 1
    rgba[top] *= (1.0 - 0.50 * shadow[top])[:, None]
    # тёмный шов по перепадам высот деталей (жёсткая кромка вместо мыла)
    crease = hard_creases(zworld, fg) & (tag == 1)
    rgba[crease] *= 0.74
    # фаски/блики по верхним кромкам (top рядом с боковой гранью)
    side = tag == 2
    side_d = ndimage.binary_dilation(side, iterations=max(1, int(round(2 * SCALE))))
    bevel = top & side_d
    rgba[bevel] = np.clip(rgba[bevel] * 1.26 + 18.0, 0, 255)
    return np.clip(rgba, 0, 255)


def normals_from_depth(camdepth, fg):
    """Карта нормалей В ЭКРАННОМ ПРОСТРАНСТВЕ из карты глубины камеры: нормаль
    поверхности = normalize(-dZ/dx, -dZ/dy, 1). Не зависит от мировой ориентации
    камеры (в отличие от прежней — та давала плоские нормали +Z у всех верхних
    граней). Фон заполняется ближайшим пикселем, градиент клипуется."""
    d = np.where(fg, camdepth, np.nan)
    if fg.any():
        idx = ndimage.distance_transform_edt(~fg, return_distances=False, return_indices=True)
        filled = d[tuple(idx)]
    else:
        filled = np.zeros_like(d)
    gy, gx = np.gradient(filled)
    lim = 3.0
    gx = np.clip(gx, -lim, lim)
    gy = np.clip(gy, -lim, lim)
    nx, ny = -gx, -gy
    nz = np.ones_like(nx)
    ln = np.sqrt(nx * nx + ny * ny + nz * nz)
    ln[ln < 1e-9] = 1.0
    nrm = np.zeros((SIZE, SIZE, 3), np.float64)
    nrm[..., 2] = 1.0
    nrm[..., 0] = np.where(fg, nx / ln, 0.0)
    nrm[..., 1] = np.where(fg, ny / ln, 0.0)
    nrm[..., 2] = np.where(fg, nz / ln, 1.0)
    return nrm


def make_outputs(rgba, zworld, nbuf, fg, camdepth):
    out = Image.fromarray(np.clip(rgba, 0, 255).astype(np.uint8), 'RGB')
    # карта глубины КАМЕРЫ (для ControlNet Depth): ближе к камере — светлее
    # (MiDaS-подобная нормировка, белое = близко).
    depth = np.zeros((SIZE, SIZE), np.float64)
    if fg.any():
        d = camdepth[fg]
        dmin, dmax = float(d.min()), float(d.max())
        rng = max(1e-6, dmax - dmin)
        depth[fg] = 12.0 + 243.0 * (camdepth[fg] - dmin) / rng
    depth_img = Image.fromarray(np.clip(depth, 0, 255).astype(np.uint8), 'L')
    nrm = normals_from_depth(camdepth, fg)
    nimg = Image.fromarray((np.clip(nrm * 0.5 + 0.5, 0, 1) * 255.0).astype(np.uint8), 'RGB')
    return out, depth_img, nimg
