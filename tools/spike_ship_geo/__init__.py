# -*- coding: utf-8 -*-
"""Пакет объёмного рендера кораблей (спайк 3–4): геометрия, растеризация
(перспектива, 3/4-ракурс), пост-эффекты, слой гриблзов и архетипы.
Обёртка — tools/spike_ship_geo.py."""
from .archetypes import ARCHES
from .render import build_scene, render

__all__ = ['ARCHES', 'build_scene', 'render']
