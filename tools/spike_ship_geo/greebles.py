# -*- coding: utf-8 -*-
# Слой «гриблзов» (спайк 3 §5): 40–120 мелких объёмных деталей на корпусе,
# размещённых СТРУКТУРНО — ряды вдоль бортов, симметричные пары, кластеры у
# надстройки, детали по кромкам палуб. Типы: танк/бак, люк, вентиляционная
# решётка, труба, антенна/шпиль, панель с бортиком, контейнер, радиатор,
# техническая ниша, панельная линия на верхней грани. Все — призмы Scene с
# высотой; свет и AO приходят из общего конвейера.
# Детерминизм: единственный источник случайности — random.Random(seed) вызывающей
# стороны; порядок обращений фиксирован.
# Клип: placers/панели принимают `clip(x, y)` (точка внутри маски корпуса) —
# деталь вне корпуса не ставится, панельная линия рвётся на отрезки внутри маски
# (запрет «проводов через холст»).
from .geom import reg_poly
from .palette import mul


def add_box(sc, cx, cy, hw, hh, z0, h, color, fr=None):
    sc.add([(cx - hw, cy - hh), (cx + hw, cy - hh),
            (cx + hw, cy + hh), (cx - hw, cy + hh)], z0, z0 + h, color)
    sc.greeble_count += 1


def add_tank(sc, cx, cy, hw, hh, z0, h, color, fr=None):
    """Танк/бак — цилиндр (эллипс-призма)."""
    sc.add(reg_poly(cx, cy, hw, hh, 12), z0, z0 + h, color)
    sc.greeble_count += 1


def add_hatch(sc, cx, cy, hw, hh, z0, h, color, fr=None):
    """Люк — низкая плита с фаской (светлая уменьшенная крышка сверху)."""
    sc.add([(cx - hw, cy - hh), (cx + hw, cy - hh), (cx + hw, cy + hh), (cx - hw, cy + hh)], z0, z0 + h, color)
    sc.add([(cx - hw * 0.7, cy - hh * 0.7), (cx + hw * 0.7, cy - hh * 0.7),
            (cx + hw * 0.7, cy + hh * 0.7), (cx - hw * 0.7, cy + hh * 0.7)],
           z0 + h, z0 + h * 1.45, mul(color, 1.16))
    sc.greeble_count += 1


def add_vent(sc, cx, cy, hw, hh, z0, h, color, fr=None, ribs=4):
    """Вентиляционная решётка — набор тонких рёбер на плите."""
    sc.add([(cx - hw, cy - hh), (cx + hw, cy - hh), (cx + hw, cy + hh), (cx - hw, cy + hh)],
           z0, z0 + h * 0.5, mul(color, 0.85))
    rw = 2.0 * hw / (ribs + 0.5)
    for i in range(ribs):
        xx = cx - hw + rw * (i + 0.75)
        sc.add([(xx - rw * 0.28, cy - hh), (xx + rw * 0.28, cy - hh),
                (xx + rw * 0.28, cy + hh), (xx - rw * 0.28, cy + hh)],
               z0 + h * 0.5, z0 + h, mul(color, 1.12))
    sc.greeble_count += 1


def add_pipe(sc, cx, cy, hw, hh, z0, h, color, fr=None):
    """Труба/трубопровод — вытянутый бокс."""
    add_box(sc, cx, cy, hw, hh, z0, h, color, fr)


def add_antenna(sc, cx, cy, hw, hh, z0, h, color, fr=None):
    """Антенна/шпиль — тонкая гранёная стойка."""
    sc.add(reg_poly(cx, cy, hw * 0.35, hh * 0.35, 6), z0, z0 + h, color)
    sc.greeble_count += 1


def add_lip(sc, cx, cy, hw, hh, z0, h, color, fr=None):
    """Панель с бортиком по периметру."""
    sc.add([(cx - hw, cy - hh), (cx + hw, cy - hh), (cx + hw, cy + hh), (cx - hw, cy + hh)], z0, z0 + h, color)
    t = max(1.0, hw * 0.2)
    for dy in (-1, 1):
        yy = cy + dy * (hh - t)
        sc.add([(cx - hw, yy - t), (cx + hw, yy - t), (cx + hw, yy + t), (cx - hw, yy + t)],
               z0 + h, z0 + h * 1.5, mul(color, 1.2))
    sc.greeble_count += 1


def add_container(sc, cx, cy, hw, hh, z0, h, color, fr=None):
    sc.add([(cx - hw, cy - hh), (cx + hw, cy - hh), (cx + hw, cy + hh), (cx - hw, cy + hh)], z0, z0 + h, color)
    sc.add([(cx - hw * 0.85, cy - hh * 0.85), (cx + hw * 0.85, cy - hh * 0.85),
            (cx + hw * 0.85, cy + hh * 0.85), (cx - hw * 0.85, cy + hh * 0.85)],
           z0 + h, z0 + h * 1.15, mul(color, 1.14))
    sc.greeble_count += 1


def add_radiator(sc, cx, cy, hw, hh, z0, h, color, fr=None, fins=5):
    """Радиатор — основание + частые тонкие рёбра."""
    sc.add([(cx - hw, cy - hh), (cx + hw, cy - hh), (cx + hw, cy + hh), (cx - hw, cy + hh)],
           z0, z0 + h * 0.4, mul(color, 0.9))
    for i in range(fins):
        xx = cx - hw + 2.0 * hw * (i + 0.5) / fins
        fw = max(1.0, hw * 0.06)
        sc.add([(xx - fw, cy - hh), (xx + fw, cy - hh), (xx + fw, cy + hh), (xx - fw, cy + hh)],
               z0 + h * 0.4, z0 + h, mul(color, 1.1))
    sc.greeble_count += 1


def add_niche(sc, cx, cy, hw, hh, z0, h, color, fr=None):
    """Техническая ниша — утопленная тёмная коробка."""
    sc.add([(cx - hw, cy - hh), (cx + hw, cy - hh), (cx + hw, cy + hh), (cx - hw, cy + hh)],
           z0, z0 + h, mul(color, 0.55))
    sc.greeble_count += 1


# наборы-миксы для рядов/кластеров
MIX_SMALL = [add_hatch, add_vent, add_pipe, add_lip, add_antenna, add_box]
MIX_DECK = [add_container, add_lip, add_radiator, add_hatch, add_tank]


def place_row(sc, rng, x0, x1, cy, n, z0, h, color, makers, fr=None, jitter=0.0, hw=6.0, hh=6.0, clip=None):
    """Ряд деталей вдоль оси X (борт/палуба) с детерминированным джиттером."""
    if n <= 0:
        return
    for i in range(n):
        cx = x0 + (x1 - x0) * (i + 0.5) / n
        cyy = cy + (rng.random() - 0.5) * 2.0 * jitter
        k = 0.75 + 0.5 * rng.random()
        maker = makers[rng.randrange(len(makers))]
        if clip is not None and not clip(cx, cyy):
            continue
        maker(sc, cx, cyy, hw * k, hh * k, z0, h, color, fr)


def place_row_y(sc, rng, cx, y0, y1, n, z0, h, color, makers, fr=None, jitter=0.0, hw=6.0, hh=6.0, clip=None):
    """Ряд деталей вдоль оси Y (поперёк корабля)."""
    if n <= 0:
        return
    for i in range(n):
        cy = y0 + (y1 - y0) * (i + 0.5) / n
        cxx = cx + (rng.random() - 0.5) * 2.0 * jitter
        k = 0.75 + 0.5 * rng.random()
        maker = makers[rng.randrange(len(makers))]
        if clip is not None and not clip(cxx, cy):
            continue
        maker(sc, cxx, cy, hw * k, hh * k, z0, h, color, fr)


def place_cluster(sc, rng, cx, cy, rx, ry, n, z0, h, color, makers, fr=None, hw=6.0, hh=6.0, clip=None):
    """Кластер деталей в прямоугольнике вокруг центра (у надстройки и т.п.)."""
    for _ in range(n):
        cxx = cx + (rng.random() - 0.5) * 2.0 * rx
        cyy = cy + (rng.random() - 0.5) * 2.0 * ry
        k = 0.7 + 0.6 * rng.random()
        maker = makers[rng.randrange(len(makers))]
        if clip is not None and not clip(cxx, cyy):
            continue
        maker(sc, cxx, cyy, hw * k, hh * k, z0, h, color, fr)


def place_pair(sc, rng, cx, cy, dy, z0, h, color, maker, fr=None, hw=6.0, hh=6.0, clip=None):
    """Симметричная пара деталей относительно осевой линии корпуса."""
    for sgn in (-1, 1):
        k = 0.8 + 0.4 * rng.random()
        y = cy + sgn * dy
        if clip is not None and not clip(cx, y):
            continue
        maker(sc, cx, y, hw * k, hh * k, z0, h, color, fr)


def _runs(x0, x1, inside, step):
    """Непрерывные отрезки [xa,xb] вдоль X, где середина отрезка внутри маски."""
    runs = []
    x = x0
    while x < x1 - 1e-9:
        seg = min(step, x1 - x)
        if inside(x + seg * 0.5):
            if runs and abs(runs[-1][1] - x) < 1e-6:
                runs[-1][1] = x + seg
            else:
                runs.append([x, x + seg])
        x += seg
    return runs


def deck_panels(sc, x0, x1, y0, y1, z, n, color, lw=5.0, clip=None):
    """n тёмных панельных линий поперёк палубы [x0..x1]×[y0..y1] на высоте z.
    С clip каждая линия рвётся на отрезки, лежащие внутри маски корпуса."""
    for i in range(n):
        yy = y0 + (i + 1) * (y1 - y0) / (n + 1)
        if clip is None:
            sc.add([(x0, yy), (x1, yy), (x1, yy + lw), (x0, yy + lw)], z, z + 1.2, color)
        else:
            for xa, xb in _runs(x0, x1, lambda x: clip(x, yy + lw * 0.5), step=max(3.0, lw)):
                sc.add([(xa, yy), (xb, yy), (xb, yy + lw), (xa, yy + lw)], z, z + 1.2, color)


def deck_panels_y(sc, x0, x1, y0, y1, z, n, color, lw=5.0, clip=None):
    """n тёмных панельных линий вдоль палубы (по X), рвутся по маске при clip."""
    for i in range(n):
        xx = x0 + (i + 1) * (x1 - x0) / (n + 1)
        if clip is None:
            sc.add([(xx, y0), (xx + lw, y0), (xx + lw, y1), (xx, y1)], z, z + 1.2, color)
        else:
            runs = []
            y = y0
            while y < y1 - 1e-9:
                seg = min(max(3.0, lw), y1 - y)
                if clip(xx + lw * 0.5, y + seg * 0.5):
                    if runs and abs(runs[-1][1] - y) < 1e-6:
                        runs[-1][1] = y + seg
                    else:
                        runs.append([y, y + seg])
                y += seg
            for ya, yb in runs:
                sc.add([(xx, ya), (xx + lw, ya), (xx + lw, yb), (xx, yb)], z, z + 1.2, color)
