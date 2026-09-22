# -*- coding: utf-8 -*-
# Растеризация сцены: ПЕРСПЕКТИВНАЯ камера (elevation/yaw вокруг корабля),
# z-buffer по глубине камеры, мировой z отдельно — для AO/теней/карты высот.
# Спайк 4: ракурс 3/4 (elev 0/15/25/40°, yaw ~15°), автокадрирование «корабль
# ~80% кадра». Мировые оси: x — вдоль корабля (нос +x), y — поперёк, z — вверх.
import math

import numpy as np

from .geom import build_triangles
from .palette import (AMBIENT, BG, DIFFUSE, FILL_COL, FILL_DIR, FILL_POWER, KEY_COL,
                      KEY_DIR, KEY_POWER, LIGHT_CINE_AMBIENT, LIGHT_WORLD, RIM_COL,
                      RIM_POWER, RIM_TILT, SIZE)


def camera_transform(elev_deg, yaw_deg):
    """Мир -> базис камеры. Возвращает (tf, light_screen).

    elev=0 — строго сверху (как спайк 3); elev>0 — камера поднимается со
    стороны +y (читается борт). yaw — лёгкий поворот корабля вокруг вертикали.
    tf(x,y,z) -> (screen_x, screen_y, cam_depth); больше depth = ближе к камере.
    light_screen — свет, повёрнутый на yaw, в экранных осях (для падающих теней).
    """
    el = math.radians(elev_deg)
    ya = math.radians(yaw_deg)
    ce, se = math.cos(el), math.sin(el)
    cy, sy = math.cos(ya), math.sin(ya)

    def tf(x, y, z):
        x1 = x * cy - y * sy
        y1 = x * sy + y * cy
        return x1, y1 * ce - z * se, y1 * se + z * ce

    lx = LIGHT_WORLD[0] * cy - LIGHT_WORLD[1] * sy
    ly = LIGHT_WORLD[0] * sy + LIGHT_WORLD[1] * cy
    return tf, (lx, ly)


def camera_basis(elev_deg, yaw_deg):
    """Базис камеры в мировых осях: right (экранный x), up (экранный y),
    toward (от сцены к камере). Согласован с camera_transform."""
    el = math.radians(elev_deg)
    ya = math.radians(yaw_deg)
    ce, se = math.cos(el), math.sin(el)
    cy, sy = math.cos(ya), math.sin(ya)
    right = np.array([cy, sy, 0.0])
    up = np.array([sy * ce, cy * ce, -se])
    toward = np.array([sy * se, cy * se, ce])
    return right, up, toward


def shade_face(n, col, mode, rim_dir):
    """Цвет грани. 'flat' — прежний один направленный свет (A/B-базлайн);
    'cine' — ключевой + заполняющий + контровой (rim), цветной ambient."""
    r, g, b = float(col[0]), float(col[1]), float(col[2])
    if mode == 'flat':
        lam = max(0.0, float(np.dot(n, LIGHT_WORLD)))
        k = AMBIENT + DIFFUSE * lam
        return (r * k, g * k, b * k)
    out = np.array([r, g, b]) * LIGHT_CINE_AMBIENT
    for L, lc, pw in ((KEY_DIR, KEY_COL, KEY_POWER), (FILL_DIR, FILL_COL, FILL_POWER)):
        lam = max(0.0, float(np.dot(n, L)))
        out += np.array([r, g, b]) * np.array(lc) * (pw * lam)
    lam = max(0.0, float(np.dot(n, rim_dir)))
    out += np.array([r, g, b]) * np.array(RIM_COL) * (RIM_POWER * lam)
    return (float(out[0]), float(out[1]), float(out[2]))


def render_buffers(scene, elev_deg, yaw_deg, light='cine', persp=3.0, margin_frac=0.08, margin_min=90.0):
    tris = build_triangles(scene)
    tf, light_screen = camera_transform(elev_deg, yaw_deg)
    _, cam_up, cam_toward = camera_basis(elev_deg, yaw_deg)
    rim = -cam_toward + RIM_TILT * cam_up
    rim_dir = rim / (float(np.linalg.norm(rim)) or 1.0)

    # 1) мир -> базис камеры по всем вершинам
    P = {}
    minx = miny = 1e18
    maxx = maxy = -1e18
    maxd = -1e18
    for t in tris:
        for v in (t[0], t[1], t[2]):
            if v in P:
                continue
            sx, syy, dep = tf(*v)
            P[v] = (sx, syy, dep)
            minx = min(minx, sx); maxx = max(maxx, sx)
            miny = min(miny, syy); maxy = max(maxy, syy)
            maxd = max(maxd, dep)
    cx = 0.5 * (minx + maxx)
    cy = 0.5 * (miny + maxy)
    rad = max(1e-6, 0.5 * math.hypot(maxx - minx, maxy - miny))
    cam_dist = maxd + persp * rad          # камера на depth = cam_dist, смотрит вниз по depth

    # 2) перспективная проекция (фокус = 1), затем единый фит в кадр
    proj = {}
    umin = vmin = 1e18
    umax = vmax = -1e18
    for v, (sx, syy, dep) in P.items():
        k = 1.0 / max(1e-6, cam_dist - dep)
        uu = (sx - cx) * k
        vv = (syy - cy) * k
        proj[v] = (uu, vv)
        umin = min(umin, uu); umax = max(umax, uu)
        vmin = min(vmin, vv); vmax = max(vmax, vv)
    span = max(umax - umin, vmax - vmin, 1e-6)
    margin = max(margin_min, margin_frac * SIZE)
    scale = (SIZE - 2.0 * margin) / span
    mu = 0.5 * (umin + umax)
    mv = 0.5 * (vmin + vmax)

    PX = {}
    for v, (uu, vv) in proj.items():
        PX[v] = ((uu - mu) * scale + SIZE * 0.5,
                 (vv - mv) * scale + SIZE * 0.5,
                 P[v][2])                      # (px, py, cam_depth)

    rgba = np.zeros((SIZE, SIZE, 3), np.float64)
    rgba[:] = BG
    zbuf = np.full((SIZE, SIZE), -1e18)
    zworld = np.zeros((SIZE, SIZE), np.float64)
    tag = np.zeros((SIZE, SIZE), np.int8)
    nbuf = np.zeros((SIZE, SIZE, 3), np.float64)
    nbuf[..., 2] = 1.0

    for t in tris:
        v0, v1, v2, n, col, tg = t
        rgb = np.clip(np.array(shade_face(n, col, light, rim_dir)), 0.0, 255.0)
        fill_tri(rgba, zbuf, zworld, tag, nbuf, PX[v0], PX[v1], PX[v2],
                 (v0[2], v1[2], v2[2]), rgb, n, tg)
    return rgba, zworld, tag, nbuf, zbuf, light_screen


def fill_tri(rgba, zbuf, zworld, tag, nbuf, p0, p1, p2, wz, rgb, n, tg):
    """Сканлайн-растеризация треугольника: z-buffer по глубине камеры (p[2]),
    мировой z (wz) интерполируется отдельно — для AO/теней/карты высот."""
    pts = [p0, p1, p2]
    ys = [p[1] for p in pts]
    y0 = max(0, int(math.floor(min(ys))))
    y1 = min(SIZE - 1, int(math.ceil(max(ys))))
    if y1 < y0:
        return
    edges = [(0, 1), (1, 2), (2, 0)]
    for y in range(y0, y1 + 1):
        yc = y + 0.5
        hits = []
        for ia, ib in edges:
            a, b = pts[ia], pts[ib]
            ay, by = a[1], b[1]
            if ay == by:
                continue
            if min(ay, by) <= yc <= max(ay, by):
                tt = (yc - ay) / (by - ay)
                hits.append((a[0] + tt * (b[0] - a[0]),
                             a[2] + tt * (b[2] - a[2]),
                             wz[ia] + tt * (wz[ib] - wz[ia])))
        if len(hits) < 2:
            continue
        hits.sort(key=lambda h: h[0])
        xa, za, wa = hits[0]
        xb, zb, wb = hits[-1]
        xi0 = max(0, int(math.ceil(xa - 0.5)))
        xi1 = min(SIZE - 1, int(math.floor(xb - 0.5)))
        if xi1 < xi0:
            continue
        xs = np.arange(xi0, xi1 + 1) + 0.5
        if xb > xa:
            k = (xs - xa) / (xb - xa)
            zline = za + k * (zb - za)
            wline = wa + k * (wb - wa)
        else:
            zline = np.full(xi1 - xi0 + 1, za)
            wline = np.full(xi1 - xi0 + 1, wa)
        cur = zbuf[y, xi0:xi1 + 1]
        sel = zline > cur
        if not sel.any():
            continue
        zbuf[y, xi0:xi1 + 1][sel] = zline[sel]
        zworld[y, xi0:xi1 + 1][sel] = wline[sel]
        tag[y, xi0:xi1 + 1][sel] = tg
        nbuf[y, xi0:xi1 + 1][sel] = n
        rgba[y, xi0:xi1 + 1][sel] = rgb
