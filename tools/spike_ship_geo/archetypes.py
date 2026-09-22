# -*- coding: utf-8 -*-
# Архетипы кораблей (нос вправо, корма слева) + структурный слой гриблзов.
# Порт трёх архетипов спайка 2 из tools/spike_ship_geo.py; гриблзы/панельные
# линии — спайк 3 (деталь-слой). Координаты: fr=(x0,y0,W,H), hh=H.
from . import greebles as gb
from .geom import point_in_poly, reg_poly
from .palette import (BRONZE, COLD_HULL, CREAM, DARK, LIGHT, LIGHT_HI, STEEL, mul)


def _shrink(poly, f):
    """Контур, втянутый к центроиду на долю f — верхняя (плоская) грань фаски:
    по ней клипуются гриблзы, чтобы не «плавали» над наклонной кромкой."""
    cx = sum(p[0] for p in poly) / len(poly)
    cy = sum(p[1] for p in poly) / len(poly)
    k = 1.0 - f
    return [(cx + (p[0] - cx) * k, cy + (p[1] - cy) * k) for p in poly]


def arch_manta_wing(fr, rng, sc):
    x0, y0, W, H = fr
    def X(f): return x0 + W * f
    def Y(f): return y0 + H * f
    hh = H  # базовая единица высоты
    # корпус-капсула с сужением к носу + фаска верхней кромки (скруглённое сечение)
    body = [(X(0.14), Y(0.36)), (X(0.34), Y(0.31)), (X(0.56), Y(0.32)),
            (X(0.80), Y(0.37)), (X(0.99), Y(0.50)), (X(0.80), Y(0.63)),
            (X(0.56), Y(0.68)), (X(0.34), Y(0.69)), (X(0.14), Y(0.64))]
    sc.add_bevel(body, 0.0, 0.070 * hh, CREAM, bevel=0.30, steps=2, fr=fr)
    # крылья (стреловидные, размах асимметричен: нижнее длиннее)
    wing_up = [(X(0.46), Y(0.30)), (X(0.18), Y(0.02)), (X(0.40), Y(0.06)), (X(0.70), Y(0.33))]
    wing_dn = [(X(0.46), Y(0.70)), (X(0.12), Y(0.99)), (X(0.42), Y(0.95)), (X(0.72), Y(0.67))]
    sc.add_bevel(wing_up, 0.0, 0.026 * hh, STEEL, bevel=0.45, steps=1, fr=fr)
    sc.add_bevel(wing_dn, 0.0, 0.026 * hh, STEEL, bevel=0.45, steps=1, fr=fr)
    # кили-плавники на законцовках крыльев (тонкие, торчат вбок)
    sc.add([(X(0.24), Y(0.055)), (X(0.19), Y(0.030)), (X(0.23), Y(0.02)), (X(0.27), Y(0.045))],
           0.026 * hh, 0.055 * hh, DARK, section=False)
    sc.add([(X(0.22), Y(0.955)), (X(0.155), Y(0.985)), (X(0.20), Y(0.995)), (X(0.25), Y(0.965))],
           0.026 * hh, 0.055 * hh, DARK, section=False)
    # кормовой блок-переходник: двигатели ПРИМЫКАЮТ к корпусу (не висят в пустоте)
    sc.add_bevel([(X(0.055), Y(0.33)), (X(0.20), Y(0.31)),
                  (X(0.20), Y(0.69)), (X(0.055), Y(0.67))],
                 0.0, 0.062 * hh, mul(CREAM, 0.92), bevel=0.22, steps=2, fr=fr)
    # горизонтальные стабилизаторы на корме
    for sgn in (-1, 1):
        sc.add_bevel([(X(0.09), Y(0.50)), (X(0.19), Y(0.50)),
                      (X(0.23), Y(0.50) + sgn * 0.17 * H), (X(0.11), Y(0.50) + sgn * 0.17 * H)],
                     0.022 * hh, 0.036 * hh, STEEL, bevel=0.42, steps=1, section=False)
    # дюзы у кормы (3, посажены на переходник)
    for i in range(3):
        cy = Y(0.40 + i * 0.10)
        sc.add_bevel(reg_poly(X(0.095), cy, W * 0.032, H * 0.030, 12),
                     0.0, 0.098 * hh, DARK, bevel=0.30, steps=2, fr=fr)
    # палуба-надстройка
    sc.add_bevel([(X(0.34), Y(0.40)), (X(0.58), Y(0.40)), (X(0.58), Y(0.60)), (X(0.34), Y(0.60))],
                 0.070 * hh, 0.115 * hh, mul(CREAM, 0.96), bevel=0.25, steps=1, fr=fr)
    sc.add_bevel([(X(0.40), Y(0.44)), (X(0.52), Y(0.44)), (X(0.52), Y(0.56)), (X(0.40), Y(0.56))],
                 0.115 * hh, 0.150 * hh, STEEL, bevel=0.28, steps=1, fr=fr)
    # центральный гребень-киль вдоль оси (отличает «крылатый» от прочих)
    sc.add_bevel([(X(0.30), Y(0.475)), (X(0.74), Y(0.475)),
                  (X(0.74), Y(0.525)), (X(0.30), Y(0.525))],
                 0.150 * hh, 0.190 * hh, mul(STEEL, 1.05), bevel=0.40, steps=2, section=False)
    # башни/антенны
    sc.add(reg_poly(X(0.46), Y(0.50), W * 0.012, H * 0.03, 6), 0.190 * hh, 0.245 * hh, DARK, fr=fr)
    sc.add(reg_poly(X(0.66), Y(0.44), W * 0.010, H * 0.02, 6), 0.070 * hh, 0.105 * hh, STEEL, fr=fr)
    # кабина-фонарь у носа (светлая)
    sc.add_bevel(reg_poly(X(0.86), Y(0.50), W * 0.055, H * 0.075, 12),
                 0.070 * hh, 0.104 * hh, LIGHT, bevel=0.35, steps=2, fr=fr)
    sc.add(reg_poly(X(0.83), Y(0.47), W * 0.022, H * 0.030, 10), 0.104 * hh, 0.116 * hh, LIGHT_HI, fr=fr)
    # мелкие надстройки
    for f in (0.24, 0.72):
        sc.add_bevel([(X(f), Y(0.44)), (X(f + 0.05), Y(0.44)), (X(f + 0.05), Y(0.56)), (X(f), Y(0.56))],
                     0.070 * hh, 0.088 * hh, mul(STEEL, 1.10), bevel=0.25, steps=1, fr=fr)
    _greebles_manta(sc, rng, fr, W, H, hh, body, wing_up, wing_dn)


def _greebles_manta(sc, rng, fr, W, H, hh, body, wing_up, wing_dn):
    x0, y0, _, _ = fr
    def X(f): return x0 + W * f
    def Y(f): return y0 + H * f

    body_top = _shrink(body, 0.30)          # плоская верхняя грань фаски корпуса

    def in_body(x, y):
        return point_in_poly(x, y, body_top)

    def in_wing(x, y):
        return point_in_poly(x, y, _shrink(wing_up, 0.45)) or point_in_poly(x, y, _shrink(wing_dn, 0.45))

    deck_z = 0.070 * hh
    gs = 0.017 * W
    small = gb.MIX_SMALL
    # ряды вдоль бортов на корпусе (впереди и позади надстройки) — крупнее и реже
    for fy in (0.385, 0.615):
        gb.place_row(sc, rng, X(0.17), X(0.32), Y(fy), 3, deck_z, 0.014 * hh,
                     mul(STEEL, 1.05), small, jitter=0.012 * H, hw=gs, hh=gs * 0.8, clip=in_body)
        gb.place_row(sc, rng, X(0.60), X(0.79), Y(fy), 4, deck_z, 0.014 * hh,
                     mul(STEEL, 1.02), small, jitter=0.012 * H, hw=gs, hh=gs * 0.8, clip=in_body)
    # симметричные пары танков по бортам
    for f in (0.24, 0.70):
        gb.place_pair(sc, rng, X(f), Y(0.50), H * 0.12, deck_z, 0.022 * hh,
                      mul(CREAM, 0.95), gb.add_tank, hw=gs * 1.2, hh=gs, clip=in_body)
    # кластер контейнеров/радиаторов у надстройки
    gb.place_cluster(sc, rng, X(0.61), Y(0.50), W * 0.045, H * 0.09, 4, deck_z, 0.018 * hh,
                     DARK, [gb.add_container, gb.add_lip, gb.add_radiator], hw=gs * 1.05, hh=gs * 0.9, clip=in_body)
    # ряды по кромкам палуб крыльев
    for fy in (0.14, 0.86):
        gb.place_row(sc, rng, X(0.30), X(0.60), Y(fy), 4, 0.026 * hh, 0.012 * hh,
                     mul(STEEL, 0.95), [gb.add_vent, gb.add_hatch, gb.add_box],
                     jitter=0.010 * H, hw=gs * 0.75, hh=gs * 0.7, clip=in_wing)
    # тех-ниши по бортам корпуса
    for sgn in (-1, 1):
        ny = Y(0.50) + sgn * H * 0.19
        if in_body(X(0.42), ny):
            gb.add_niche(sc, X(0.42), ny, gs * 1.5, gs * 0.5, deck_z, 0.010 * hh, STEEL)
    # панельные линии по верхним граням
    gb.deck_panels(sc, X(0.16), X(0.80), Y(0.36), Y(0.64), deck_z + 0.5, 5,
                   mul(CREAM, 0.5), lw=max(3.0, 0.006 * W), clip=in_body)
    for fy in (0.06, 0.79):
        gb.deck_panels(sc, X(0.28), X(0.64), Y(fy), Y(fy + 0.16), 0.026 * hh + 0.5, 3,
                       mul(STEEL, 0.5), lw=max(3.0, 0.005 * W), clip=in_wing)


def arch_flat_barge(fr, rng, sc):
    x0, y0, W, H = fr
    def X(f): return x0 + W * f
    def Y(f): return y0 + H * f
    hh = H
    hull = [(X(0.00), Y(0.10)), (X(0.14), Y(0.03)), (X(0.58), Y(0.05)),
            (X(0.86), Y(0.22)), (X(1.00), Y(0.50)), (X(0.86), Y(0.78)),
            (X(0.58), Y(0.95)), (X(0.14), Y(0.97)), (X(0.00), Y(0.90))]
    sc.add_bevel(hull, 0.0, 0.085 * hh, CREAM, bevel=0.22, steps=2, fr=fr)
    # фальшборт: тонкие борта ВНУТРИ корпуса (артефакт спайка 1 — подвешенный
    # борт вне корпуса + бронзовые подпалины по кромкам — устранён)
    sc.add_bevel([(X(0.06), Y(0.09)), (X(0.70), Y(0.10)), (X(0.70), Y(0.15)), (X(0.06), Y(0.14))],
                 0.085 * hh, 0.105 * hh, mul(STEEL, 1.05), bevel=0.40, steps=1, section=False)
    sc.add_bevel([(X(0.06), Y(0.86)), (X(0.70), Y(0.85)), (X(0.70), Y(0.90)), (X(0.06), Y(0.91))],
                 0.085 * hh, 0.105 * hh, mul(STEEL, 1.05), bevel=0.40, steps=1, section=False)
    # надстройка-ярусы (3 ступени, не сцентрированы)
    sc.add_bevel([(X(0.32), Y(0.28)), (X(0.64), Y(0.26)), (X(0.64), Y(0.74)), (X(0.32), Y(0.72))],
                 0.085 * hh, 0.155 * hh, STEEL, bevel=0.22, steps=2, fr=fr)
    sc.add_bevel([(X(0.40), Y(0.38)), (X(0.58), Y(0.37)), (X(0.58), Y(0.63)), (X(0.40), Y(0.62))],
                 0.155 * hh, 0.215 * hh, mul(STEEL, 1.12), bevel=0.28, steps=2, fr=fr)
    sc.add_bevel([(X(0.45), Y(0.45)), (X(0.54), Y(0.45)), (X(0.54), Y(0.55)), (X(0.45), Y(0.55))],
                 0.215 * hh, 0.250 * hh, CREAM, bevel=0.30, steps=2, fr=fr)
    # мачты + парус
    sc.add(reg_poly(X(0.22), Y(0.30), W * 0.010, H * 0.02, 6), 0.085 * hh, 0.240 * hh, BRONZE, fr=fr)
    sc.add(reg_poly(X(0.22), Y(0.70), W * 0.010, H * 0.02, 6), 0.085 * hh, 0.240 * hh, BRONZE, fr=fr)
    sc.add([(X(0.19), Y(0.30)), (X(0.25), Y(0.30)), (X(0.25), Y(0.70)), (X(0.19), Y(0.70))],
           0.230 * hh, 0.243 * hh, mul(CREAM, 1.05), fr=fr)
    # сваи-опоры: утолщённые у основания, торчат из-под борта (не «провода»)
    for f in (0.20, 0.36, 0.52):
        for sy in (0.03, 0.97):
            yy = Y(sy)
            sc.add([(X(f - 0.012), yy), (X(f + 0.012), yy),
                    (X(f + 0.026), yy + (H * 0.05 if sy > 0.5 else -H * 0.05)),
                    (X(f - 0.026), yy + (H * 0.05 if sy > 0.5 else -H * 0.05))],
                   0.085 * hh, 0.135 * hh, DARK, fr=fr)
    # руль-корма
    sc.add_bevel([(X(0.00), Y(0.40)), (X(0.06), Y(0.42)), (X(0.06), Y(0.58)), (X(0.00), Y(0.60))],
                 0.0, 0.115 * hh, mul(STEEL, 0.9), bevel=0.25, steps=1, fr=fr)
    # кормовой переходник + дюзы (примыкают к корпусу)
    sc.add_bevel([(X(0.015), Y(0.35)), (X(0.10), Y(0.33)), (X(0.10), Y(0.67)), (X(0.015), Y(0.65))],
                 0.0, 0.072 * hh, mul(STEEL, 0.95), bevel=0.25, steps=1, fr=fr)
    for i in range(2):
        cy = Y(0.40 + i * 0.20)
        sc.add_bevel(reg_poly(X(0.045), cy, W * 0.020, H * 0.035, 10),
                     0.0, 0.095 * hh, DARK, bevel=0.30, steps=2, fr=fr)
    # иллюминаторы-трубы по палубе
    for f in (0.72, 0.82):
        sc.add(reg_poly(X(f), Y(0.50), W * 0.014, H * 0.05, 8), 0.085 * hh, 0.130 * hh, LIGHT, fr=fr)
    # палубные люки и ящики (ворк-детали, дают внутренние кромки)
    sc.add_bevel([(X(0.18), Y(0.34)), (X(0.28), Y(0.34)), (X(0.28), Y(0.52)), (X(0.18), Y(0.52))],
                 0.085 * hh, 0.110 * hh, mul(STEEL, 0.94), bevel=0.25, steps=1, fr=fr)
    sc.add_bevel([(X(0.74), Y(0.30)), (X(0.90), Y(0.28)), (X(0.90), Y(0.42)), (X(0.74), Y(0.44))],
                 0.085 * hh, 0.108 * hh, mul(CREAM, 0.90), bevel=0.25, steps=1, fr=fr)
    sc.add_bevel([(X(0.70), Y(0.62)), (X(0.86), Y(0.60)), (X(0.86), Y(0.72)), (X(0.70), Y(0.74))],
                 0.085 * hh, 0.112 * hh, mul(STEEL, 1.08), bevel=0.25, steps=1, fr=fr)
    # лебёдка у носа
    sc.add(reg_poly(X(0.92), Y(0.50), W * 0.016, H * 0.06, 8), 0.085 * hh, 0.120 * hh, DARK, fr=fr)
    _greebles_barge(sc, rng, fr, W, H, hh, hull)


def _greebles_barge(sc, rng, fr, W, H, hh, hull):
    x0, y0, _, _ = fr
    def X(f): return x0 + W * f
    def Y(f): return y0 + H * f

    hull_top = _shrink(hull, 0.22)          # плоская верхняя грань фаски корпуса

    def in_hull(x, y):
        return point_in_poly(x, y, hull_top)

    deck_z = 0.085 * hh
    gs = 0.016 * W
    small = gb.MIX_SMALL
    # носовая палуба — ряды вдоль (крупнее и реже)
    for fy in (0.34, 0.66):
        gb.place_row(sc, rng, X(0.66), X(0.86), Y(fy), 3, deck_z, 0.014 * hh,
                     mul(STEEL, 1.02), small, jitter=0.02 * H, hw=gs, hh=gs * 0.8, clip=in_hull)
    gb.place_cluster(sc, rng, X(0.76), Y(0.50), W * 0.04, H * 0.08, 4, deck_z, 0.017 * hh,
                     DARK, gb.MIX_DECK, hw=gs * 1.05, hh=gs, clip=in_hull)
    # кормовая палуба — два ряда
    for fy in (0.32, 0.68):
        gb.place_row(sc, rng, X(0.08), X(0.30), Y(fy), 3, deck_z, 0.014 * hh,
                     mul(STEEL, 0.98), small, jitter=0.02 * H, hw=gs, hh=gs * 0.8, clip=in_hull)
    # борта — длинные ряды по кромкам палубы
    for fy in (0.20, 0.80):
        gb.place_row(sc, rng, X(0.10), X(0.66), Y(fy), 4, deck_z, 0.013 * hh,
                     mul(STEEL, 1.05), small, jitter=0.018 * H, hw=gs * 0.9, hh=gs * 0.75, clip=in_hull)
    # симметричные пары баков-цилиндров вдоль корпуса
    for f, dy in ((0.16, 0.20), (0.62, 0.16)):
        gb.place_pair(sc, rng, X(f), Y(0.50), H * dy, deck_z, 0.022 * hh,
                      mul(CREAM, 0.95), gb.add_tank, hw=gs * 1.1, hh=gs * 0.9, clip=in_hull)
    # тех-ниши у надстройки
    for sgn in (-1, 1):
        ny = Y(0.50) + sgn * H * 0.21
        if in_hull(X(0.36), ny):
            gb.add_niche(sc, X(0.36), ny, gs * 1.6, gs * 0.5, deck_z, 0.010 * hh, STEEL)
    # панельные линии по палубам
    gb.deck_panels(sc, X(0.64), X(0.88), Y(0.20), Y(0.80), deck_z + 0.5, 4,
                   mul(CREAM, 0.5), lw=max(3.0, 0.006 * W), clip=in_hull)
    gb.deck_panels(sc, X(0.06), X(0.30), Y(0.10), Y(0.90), deck_z + 0.5, 4,
                   mul(CREAM, 0.5), lw=max(3.0, 0.006 * W), clip=in_hull)
    gb.deck_panels_y(sc, X(0.08), X(0.64), Y(0.17), Y(0.24), deck_z + 0.5, 3,
                     mul(STEEL, 0.5), lw=max(3.0, 0.005 * W), clip=in_hull)
    gb.deck_panels_y(sc, X(0.08), X(0.64), Y(0.76), Y(0.83), deck_z + 0.5, 3,
                     mul(STEEL, 0.5), lw=max(3.0, 0.005 * W), clip=in_hull)


def arch_seed_pod(fr, rng, sc):
    x0, y0, W, H = fr
    def X(f): return x0 + W * f
    def Y(f): return y0 + H * f
    hh = H
    hull = reg_poly(X(0.48), Y(0.50), W * 0.40, H * 0.40, 24)
    # капсула со СКРУГЛЁННЫМ поперечным сечением: высокая фаска (2 ступени)
    sc.add_bevel(hull, 0.0, 0.105 * hh, COLD_HULL, bevel=0.32, steps=2, fr=fr)
    # лепестки-створки кормы (три отдельные створки, разной высоты)
    for i, fy in enumerate((0.20, 0.50, 0.80)):
        lean = 0.03 * (i - 1)
        sc.add_bevel([(X(0.20), Y(fy - 0.09)), (X(0.00), Y(fy + lean - 0.13)),
                      (X(0.00), Y(fy + lean + 0.13)), (X(0.20), Y(fy + 0.09))],
                     0.0, (0.030 + 0.008 * (i == 1)) * hh, mul(COLD_HULL, 0.92),
                     bevel=0.35, steps=1, fr=fr)
    # продольные рёбра по верху (фаска — читаются как грани капсулы)
    for fy in (0.26, 0.38, 0.50, 0.62, 0.74):
        sc.add_bevel([(X(0.22), Y(fy - 0.012)), (X(0.74), Y(fy - 0.012)),
                      (X(0.74), Y(fy + 0.012)), (X(0.22), Y(fy + 0.012))],
                     0.105 * hh, 0.140 * hh, mul(COLD_HULL, 1.12),
                     bevel=0.55, steps=2, section=False)
    # сопло/дюза на переходном кольце (примыкает к капсуле и створкам)
    sc.add_bevel(reg_poly(X(0.16), Y(0.50), W * 0.11, H * 0.20, 16),
                 0.0, 0.085 * hh, mul(COLD_HULL, 0.86), bevel=0.35, steps=2, fr=fr)
    sc.add_bevel(reg_poly(X(0.09), Y(0.50), W * 0.045, H * 0.10, 12),
                 0.0, 0.13 * hh, STEEL, bevel=0.30, steps=2, fr=fr)
    sc.add(reg_poly(X(0.06), Y(0.50), W * 0.025, H * 0.055, 10), 0.13 * hh, 0.16 * hh, DARK, fr=fr)
    # купол-кабина (светлый)
    sc.add_bevel(reg_poly(X(0.68), Y(0.50), W * 0.055, H * 0.13, 12),
                 0.105 * hh, 0.170 * hh, LIGHT, bevel=0.45, steps=2, fr=fr)
    sc.add(reg_poly(X(0.66), Y(0.47), W * 0.022, H * 0.05, 10), 0.170 * hh, 0.185 * hh, LIGHT_HI, fr=fr)
    # гранёные кристаллы-надстройки
    for (fx, fy, r) in ((0.30, 0.30, 0.03), (0.34, 0.70, 0.028), (0.56, 0.34, 0.024)):
        sc.add_bevel(reg_poly(X(fx), Y(fy), W * r, H * r * 2.2, 6),
                     0.105 * hh, 0.180 * hh, mul(COLD_HULL, 1.22), bevel=0.55, steps=2, fr=fr)
    _greebles_pod(sc, rng, fr, W, H, hh, hull)


def _greebles_pod(sc, rng, fr, W, H, hh, hull):
    x0, y0, _, _ = fr
    def X(f): return x0 + W * f
    def Y(f): return y0 + H * f

    hull_top = _shrink(hull, 0.32)          # плоская верхняя грань фаски капсулы

    def in_hull(x, y):
        return point_in_poly(x, y, hull_top)

    deck_z = 0.105 * hh
    gs = 0.015 * W
    small = gb.MIX_SMALL
    # ряды в промежутках между продольными рёбрами (крупнее и реже)
    for fy in (0.32, 0.50, 0.68):
        gb.place_row(sc, rng, X(0.24), X(0.72), Y(fy), 4, deck_z, 0.013 * hh,
                     mul(COLD_HULL, 1.05), small, jitter=0.008 * H, hw=gs * 0.9, hh=gs * 0.8, clip=in_hull)
    # кластер у купола
    gb.place_cluster(sc, rng, X(0.58), Y(0.50), W * 0.05, H * 0.09, 4, deck_z, 0.017 * hh,
                     DARK, gb.MIX_DECK, hw=gs * 1.05, hh=gs * 0.9, clip=in_hull)
    # кластеры по телу капсулы
    for (fx, fy) in ((0.40, 0.42), (0.40, 0.58)):
        gb.place_cluster(sc, rng, X(fx), Y(fy), W * 0.035, H * 0.06, 3, deck_z, 0.015 * hh,
                         mul(STEEL, 1.05), small, hw=gs, hh=gs * 0.8, clip=in_hull)
    # симметричные пары баков
    for f in (0.36, 0.62):
        gb.place_pair(sc, rng, X(f), Y(0.50), H * 0.24, deck_z, 0.020 * hh,
                      mul(COLD_HULL, 0.95), gb.add_tank, hw=gs * 1.2, hh=gs, clip=in_hull)
    # тех-ниши по бортам
    for sgn in (-1, 1):
        ny = Y(0.50) + sgn * H * 0.30
        if in_hull(X(0.48), ny):
            gb.add_niche(sc, X(0.48), ny, gs * 1.6, gs * 0.5, deck_z, 0.010 * hh, STEEL)
    # панельные линии по верхней грани (внутри эллипса)
    gb.deck_panels(sc, X(0.22), X(0.74), Y(0.30), Y(0.70), deck_z + 0.5, 5,
                   mul(COLD_HULL, 0.5), lw=max(3.0, 0.006 * W), clip=in_hull)


ARCHES = {
    'manta_wing': arch_manta_wing,
    'flat_barge': arch_flat_barge,
    'seed_pod': arch_seed_pod,
}
