# -*- coding: utf-8 -*-
# Палитра, свет и числовые помощники рендера кораблей (спайк 3).
# Палитра — art_ships.md §2.1 (+ coral из спеки 2026-09-21 §11.1д).
import numpy as np

SIZE = 1536
BG = (5, 5, 5)

CREAM = (232, 224, 186)
COLD_HULL = (170, 205, 235)
STEEL = (120, 140, 170)
BRONZE = (140, 90, 45)
DARK = (40, 44, 55)
LIGHT = (180, 210, 235)
LIGHT_HI = (230, 242, 252)

# Свет плоский (спайк 3-4): один направленный сверху-слева. Вектор — В СТОРОНУ света.
LIGHT_WORLD = np.array([-0.42, -0.52, 0.74], dtype=np.float64)
LIGHT_WORLD /= np.linalg.norm(LIGHT_WORLD)
AMBIENT = 0.34
DIFFUSE = 0.80

# Кино-свет (спайк 5): ключевой + заполняющий + контровой (rim). Направления —
# В СТОРОНУ света (мировые оси; rim считается от камеры в raster.py).
LIGHT_CINE_AMBIENT = 0.20
KEY_DIR = LIGHT_WORLD                       # ключевой: сверху-слева, тёплый
KEY_COL = (1.00, 0.97, 0.90)
KEY_POWER = 0.86
FILL_DIR = np.array([0.62, 0.34, 0.18], dtype=np.float64)   # заполняющий: справа-снизу, холодный
FILL_DIR /= np.linalg.norm(FILL_DIR)
FILL_COL = (0.42, 0.55, 0.78)
FILL_POWER = 0.34
RIM_COL = (0.58, 0.78, 1.00)                # контровой: холодный ободок сзади
RIM_POWER = 0.55
RIM_TILT = 0.55                             # наклон rim-света вверх от линии камеры


def fnv1a(slug):
    h = 0x811C9DC5
    for ch in slug.encode('utf-8'):
        h ^= ch
        h = (h * 0x01000193) & 0xFFFFFFFF
    return h


def mul(c, k):
    return tuple(int(max(0, min(255, round(v * k)))) for v in c)


def clamp255(c):
    return tuple(int(max(0, min(255, round(v)))) for v in c)
