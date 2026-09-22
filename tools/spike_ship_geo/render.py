# -*- coding: utf-8 -*-
# Сборка рендера: spec-JSON (+seed) -> сцена архетипа -> буферы -> пост-эффекты.
import random

from .archetypes import ARCHES
from .geom import Scene
from .palette import SIZE, fnv1a
from .raster import render_buffers
from .shade import make_outputs, post_effects


def build_scene(spec):
    p = spec['params']
    W = min(p['aspect'] * p['y_fill'] * SIZE, 0.84 * SIZE)
    H = p['y_fill'] * SIZE
    fr = ((SIZE - W) / 2.0, (SIZE - H) / 2.0, W, H)
    sc = Scene()
    # Детерминизм (спека §4.3): seed = FNV1a(slug). Архетипы заданы руками, rng
    # заведён под контракт spec-JSON (jitter параметров — полная реализация §12.2).
    rng = random.Random(fnv1a(spec['race']))
    ARCHES[p['archetype']](fr, rng, sc)
    return sc


def render(spec, elev=25.0, yaw=15.0, light='cine'):
    """Ракурс: elevation (0=сверху) и yaw; камера перспективная, кадрирование
    по геометрии (корабль ~80% кадра). light: 'cine' (ключ+заполняющий+rim) или
    'flat' (прежний один направленный свет). Возвращает (цвет, карта глубины
    камеры, карта нормалей)."""
    sc = build_scene(spec)
    rgba, zworld, tag, nbuf, camdepth, light_screen = render_buffers(sc, elev, yaw, light=light)
    fg = tag > 0
    rgba = post_effects(rgba, zworld, tag, fg, light_screen)
    return make_outputs(rgba, zworld, nbuf, fg, camdepth)
