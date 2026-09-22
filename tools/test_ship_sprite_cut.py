# -*- coding: utf-8 -*-
# Регресс-тесты выреза спрайтов кораблей.
#  1) фикс 2026-09-22 «фон режет корпус насквозь»: тёмный корпус цветом близок к
#     фону и связан с ним узкой щелью — remove_bg_hyst его выедает (баг),
#     remove_bg_edge (барьер по размытой кромке) сохраняет;
#  2) magenta-хромакей (рецепт 2026-09-22): remove_bg_chroma удаляет ровный
#     magenta-фон, НЕ выедает magenta-детали внутри корпуса и даёт мягкую кромку.
#  3) авто-детект фона кадра is_magenta_bg (рамка magenta vs чёрная) — источник
#     выбора порога выреза (рецепт 2026-09-22: фон снова чёрный).
# Запуск: C:\ComfyUI\venv\Scripts\python.exe tools\test_ship_sprite_cut.py
# (или из корня: python tools/test_ship_sprite_cut.py)
import os
import sys
import unittest

import numpy as np
from PIL import Image

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import ship_sprite_cut as sc  # noqa: E402
import ship_recut_pool as recut  # noqa: E402

TOL = 40
EDGE = 20
BLUR = 3.0

# magenta-хромакей: фон m=min(R,B)-G ≈ 105, корпус m ≈ -20 (см. remove_bg_chroma).
CHROMA_TOL = 60
CHROMA_SOFT = 25
CHROMA_BG = (210, 30, 135)
CHROMA_HULL = (40, 60, 80)


def synthetic_hull_in_ring(ring=6, gap=3, hull_val=34, body_val=200, bg_val=30):
    """Тёмный корпус (diff ~12) внутри яркого кольца; единственная связь с фоном —
    узкая щель в кольце. Возвращает (img, hull_center_xy)."""
    a = np.full((220, 220, 3), bg_val, np.uint8)
    a[50:170, 50:170] = body_val               # яркое кольцо-заготовка
    a[50 + ring:170 - ring, 50 + ring:170 - ring] = hull_val   # тёмный корпус
    a[110 - gap:110 + gap, 170 - ring:170] = hull_val          # щель наружу
    return Image.fromarray(a), (110, 110)


def synthetic_magenta(ring=16, body=(60, 160)):
    """Кадр с magenta-фоном (рецепт 2026-09-22): ровный magenta-фон, корпус
    (HULL), мягкая кромка шириной ring (цвет линейно BG→HULL), внутри корпуса —
    заключённая magenta-деталь. Возвращает (img, hull_center_xy)."""
    w = 220
    a = np.full((w, w, 3), CHROMA_BG, np.uint8)
    i0, i1 = body
    # мягкая кромка: кольцо вокруг тела, alpha 0 (фон) → 1 (корпус)
    for i in range(ring):
        alpha = i / float(ring)
        c = tuple(int(round(CHROMA_BG[k] * (1 - alpha) + CHROMA_HULL[k] * alpha))
                  for k in range(3))
        r0, r1 = i0 - ring + i, i1 + ring - i
        a[r0, i0 - ring:i1 + ring] = c
        a[r1 - 1, i0 - ring:i1 + ring] = c
        a[i0 - ring:i1 + ring, r0] = c
        a[i0 - ring:i1 + ring, r1 - 1] = c
    a[i0:i1, i0:i1] = CHROMA_HULL                         # жёсткое тело корпуса
    a[100:120, 100:120] = CHROMA_BG                       # magenta-деталь внутри
    return Image.fromarray(a), (110, 110)


def alpha(img):
    return np.array(img.convert("RGBA"))[:, :, 3]


class TestShipCutRegression(unittest.TestCase):
    def test_dark_hull_kept_by_edge(self):
        img, (cy, cx) = synthetic_hull_in_ring()
        # эталон: корпус должен оставаться непрозрачным в пятачке вокруг центра
        h = alpha(sc.remove_bg_hyst(img, 12, TOL))[cy - 8:cy + 8, cx - 8:cx + 8]
        e = alpha(sc.remove_bg_edge(img, TOL, EDGE, BLUR))[cy - 8:cy + 8, cx - 8:cx + 8]
        # баг зафиксирован: прежний метод (hyst) выедает корпус
        self.assertLess(h.mean(), 20, "hyst должен воспроизводить баг (корпус съеден)")
        # фикс: edge сохраняет корпус
        self.assertGreater(e.mean(), 235, "edge должен сохранять тёмный корпус")

    def test_background_removed_by_edge(self):
        img, _ = synthetic_hull_in_ring()
        e = alpha(sc.remove_bg_edge(img, TOL, EDGE, BLUR))
        # углы — чистый фон → прозрачны
        for sl in (np.s_[0:20, 0:20], np.s_[0:20, -20:], np.s_[-20:, 0:20], np.s_[-20:, -20:]):
            self.assertLess(e[sl].mean(), 5, "фон должен быть удалён (углы прозрачны)")

    def test_flat_background_untouched_pixels_inside_hull(self):
        """Тёмный корпус держится целиком, а не одним пикселем."""
        img, (cy, cx) = synthetic_hull_in_ring()
        e = alpha(sc.remove_bg_edge(img, TOL, EDGE, BLUR))
        # корпус — квадрат со стороной (220-2*50-2*ring); проверяем, что >90% непрозрачно
        inner = e[50 + 6 + 8:170 - 6 - 8, 50 + 6 + 8:170 - 6 - 8]
        self.assertGreater((inner > 40).mean(), 0.9, "корпус должен остаться почти весь")


class TestShipCutChroma(unittest.TestCase):
    """remove_bg_chroma — вырез magenta-хромакея (рецепт 2026-09-22)."""

    def test_flat_magenta_background_removed(self):
        img, _ = synthetic_magenta()
        a = alpha(sc.remove_bg_chroma(img, CHROMA_TOL, CHROMA_SOFT))
        for sl in (np.s_[:15, :15], np.s_[:15, -15:], np.s_[-15:, :15], np.s_[-15:, -15:]):
            self.assertLess(a[sl].mean(), 5, "ровный magenta-фон должен быть прозрачен")

    def test_magenta_detail_inside_hull_kept(self):
        img, (cy, cx) = synthetic_magenta()
        a = alpha(sc.remove_bg_chroma(img, CHROMA_TOL, CHROMA_SOFT))
        self.assertGreater(a[cy - 8:cy + 8, cx - 8:cx + 8].mean(), 235,
                           "magenta-деталь внутри корпуса не должна выедаться (flood от рамки)")

    def test_soft_edge_present(self):
        img, _ = synthetic_magenta()
        a = alpha(sc.remove_bg_chroma(img, CHROMA_TOL, CHROMA_SOFT))
        band = int(((a > 20) & (a < 235)).sum())
        self.assertGreater(band, 50, "мягкая кромка: должны быть полупрозрачные пиксели")


class TestMagentaBgDetect(unittest.TestCase):
    """is_magenta_bg (tools/ship_recut_pool.py) — источник авто-детекта порога
    выреза; Go-порт в студии — postproc.ShipCutTol/isMagentaFrame (рецепт
    2026-09-22, фон снова чёрный: magenta-рамка → tol 100, чёрная → tol 40)."""

    def test_magenta_border(self):
        img = Image.new("RGB", (200, 200), CHROMA_BG)
        self.assertTrue(recut.is_magenta_bg(img), "рамка magenta → фон magenta")

    def test_black_border(self):
        img = Image.new("RGB", (200, 200), (0, 0, 0))
        self.assertFalse(recut.is_magenta_bg(img), "чёрная рамка → фон не magenta")

    def test_dark_grey_border(self):
        img = Image.new("RGB", (200, 200), (40, 40, 40))
        self.assertFalse(recut.is_magenta_bg(img), "серый фон → не magenta")

    def test_magenta_body_on_black_border(self):
        """Magenta-корпус в центре, рамка чёрная → фон не magenta (деталь не решает)."""
        img = Image.new("RGB", (200, 200), (0, 0, 0))
        for y in range(60, 140):
            for x in range(60, 140):
                img.putpixel((x, y), CHROMA_BG)
        self.assertFalse(recut.is_magenta_bg(img))


if __name__ == "__main__":
    unittest.main(verbosity=2)
