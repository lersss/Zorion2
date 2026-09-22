# -*- coding: utf-8 -*-
# Геометрия сцены: 2D-полигоны (XY, x — вдоль корабля, нос +x), призмы и
# триангуляция для растеризации. Порт из tools/spike_ship_geo.py (спайк 2).
import math

import numpy as np


def reg_poly(cx, cy, rx, ry, n, rot=0.0):
    """Правильный n-угольник (эллипс-приближение) — детали, сопла, ядра."""
    return [(cx + rx * math.cos(rot + 2 * math.pi * i / n),
             cy + ry * math.sin(rot + 2 * math.pi * i / n)) for i in range(n)]


def area2(poly):
    s = 0.0
    for i in range(len(poly)):
        x1, y1 = poly[i]
        x2, y2 = poly[(i + 1) % len(poly)]
        s += x1 * y2 - x2 * y1
    return s


def ccw(poly):
    return poly if area2(poly) > 0 else list(reversed(poly))


def point_in_poly(x, y, poly):
    """Точка внутри простого многоугольника (ray casting). Границы — включительно
    «внутри» с точностью до погрешности растеризации."""
    inside = False
    n = len(poly)
    for i in range(n):
        x1, y1 = poly[i]
        x2, y2 = poly[(i + 1) % n]
        if (y1 > y) != (y2 > y):
            xin = x1 + (y - y1) * (x2 - x1) / (y2 - y1)
            if x < xin:
                inside = not inside
    return inside


def height_note(poly, fr):
    """Доля длины корпуса (0 — корма, 1 — нос) по центру деталей."""
    x0, y0, W, H = fr
    xs = [p[0] for p in poly]
    return (sum(xs) / len(xs) - x0) / max(1.0, W)


class Scene:
    """Набор 3D-деталей: тело = нижний и верхний контуры XY + низ/верх по Z + цвет.
    Призма (pb == pt) или фрустум (pt втянут — фаска/скругление кромки)."""

    def __init__(self):
        self.parts = []
        self.zmax = 0.0
        self.zmin = 0.0
        self.greeble_count = 0

    def _section_color(self, poly, color, section, fr):
        # Секции корпуса (рисунок, не свет): нос светлее, корма темнее.
        if not (section and fr is not None):
            return color
        f = height_note(poly, fr)
        if f > 0.66:
            return _mul(color, 1.10)
        if f < 0.34:
            return _mul(color, 0.88)
        return color

    def _push(self, pb, pt, z0, z1, color):
        self.parts.append({'pb': pb, 'pt': pt, 'z0': float(z0), 'z1': float(z1), 'color': color})
        self.zmax = max(self.zmax, z1)
        self.zmin = min(self.zmin, z0)

    def add(self, poly, z0, z1, color, section=True, fr=None):
        poly = ccw([(float(x), float(y)) for x, y in poly])
        if len(poly) < 3:
            return
        self._push(poly, poly, z0, z1, self._section_color(poly, color, section, fr))

    def add_bevel(self, poly, z0, z1, color, bevel=0.20, steps=2, section=True, fr=None):
        """Тело с фаской/скруглением верхней кромки: призма высотой (1-bevel) +
        `steps` ступеней фрустумов, верхнее сечение втянуто к центроиду на `bevel`.
        Даёт наклонные верхние грани — depth/normal несут настоящую форму."""
        poly = ccw([(float(x), float(y)) for x, y in poly])
        if len(poly) < 3:
            return
        col = self._section_color(poly, color, section, fr)
        b = max(0.0, min(0.9, float(bevel)))
        steps = max(1, int(steps))
        if b <= 0.0 or z1 <= z0:
            self._push(poly, poly, z0, z1, col)
            return
        body_top = z1 - (z1 - z0) * b
        self._push(poly, poly, z0, body_top, col)
        for i in range(steps):
            f0, f1 = float(i) / steps, float(i + 1) / steps
            self._push(_inset(poly, b * f0), _inset(poly, b * f1),
                       body_top + (z1 - body_top) * f0, body_top + (z1 - body_top) * f1, col)


def _inset(poly, f):
    """Втянуть многоугольник к центроиду на долю f (фаска верхней кромки)."""
    if f <= 0.0:
        return poly
    cx = sum(p[0] for p in poly) / len(poly)
    cy = sum(p[1] for p in poly) / len(poly)
    k = 1.0 - f
    return [(cx + (p[0] - cx) * k, cy + (p[1] - cy) * k) for p in poly]


def _mul(c, k):
    return tuple(int(max(0, min(255, round(v * k)))) for v in c)


def _tri(v0, v1, v2):
    return (v0, v1, v2)


def build_triangles(scene):
    """Тело (призма или фрустум) → треугольники (верх + бока). Нормали — мировые,
    наружу; наклонная боковая грань фрустума даёт нормаль с вертикальной компонентой."""
    tris = []
    for part in scene.parts:
        pb, pt, z0, z1 = part['pb'], part['pt'], part['z0'], part['z1']
        col = part['color']
        cx = sum(p[0] for p in pt) / len(pt)
        cy = sum(p[1] for p in pt) / len(pt)
        # верхняя грань (нормаль +z): веер от центроида
        ctop = (cx, cy, z1)
        for i in range(len(pt)):
            a = (pt[i][0], pt[i][1], z1)
            b = (pt[(i + 1) % len(pt)][0], pt[(i + 1) % len(pt)][1], z1)
            tris.append(_tri(ctop, a, b) + (np.array([0.0, 0.0, 1.0]), col, 1))
        # боковые грани (наружу); для призмы совпадает с (dy, -dx, 0)
        for i in range(len(pb)):
            j = (i + 1) % len(pb)
            a0 = (pb[i][0], pb[i][1], z0)
            b0 = (pb[j][0], pb[j][1], z0)
            a1 = (pt[i][0], pt[i][1], z1)
            b1 = (pt[j][0], pt[j][1], z1)
            n = np.cross(np.array([b0[0] - a0[0], b0[1] - a0[1], b0[2] - a0[2]]),
                         np.array([a1[0] - a0[0], a1[1] - a0[1], a1[2] - a0[2]]))
            ln = float(np.linalg.norm(n))
            n = n / ln if ln > 1e-9 else np.array([0.0, 0.0, 1.0])
            tris.append(_tri(a0, b0, b1) + (n, col, 2))
            tris.append(_tri(a0, b1, a1) + (n, col, 2))
    return tris
