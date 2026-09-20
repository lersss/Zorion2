# -*- coding: utf-8 -*-
# Генератор силуэтов кораблей рас (спека 2026-09-20-ships-races-generator §3.1).
# Вход: разобранный spec-JSON от Go-парсера (--spec=<path>; парсер ТЗ живёт
# в Go — TDD-требование, отклонение от буквы спеки «--race=<slug>»).
# Выход: 1024×1024, фон (5,5,5), вид сверху, нос вправо, запас ~90 px
# (масштаб ~82% + центрирование), палитра art_ships.md §2.1, границы (12,12,12).
# Использование: python make_ship_silhouettes.py --spec=<spec.json> --out=<out.png>
import argparse
import json
import os
import random

from PIL import Image, ImageDraw

SIZE = 1024
W = int(SIZE * 0.82)   # 839 — ширина корабля (~82% холста)
H = int(SIZE * 0.45)   # 460 — высота (аспект ~1.8:1)
X0 = (SIZE - W) // 2   # 92 — левый край (запас ~90 px)
Y0 = (SIZE - H) // 2   # 282

BG = (5, 5, 5)
CREAM = (232, 224, 186)   # корпус (тёплый крем/пепел)
COLD_HULL = (170, 205, 235)  # холодный корпус (бледно-голубой, диагноз визуального аудита)
STEEL = (120, 140, 170)   # крылья
BRONZE = (140, 90, 45)    # хвост
DARK = (40, 44, 55)       # дюзы
LIGHT = (180, 210, 235)   # кабина
FIRE = (230, 120, 40)     # огонь (тёплое свечение)
COLD = (120, 220, 235)    # голубое (холодное свечение)
BORDER = (12, 12, 12)

COLOR_MAP = {
    'cream': CREAM, 'steel': STEEL, 'bronze': BRONZE, 'dark': DARK,
    'light': LIGHT, 'fire': FIRE, 'cold': COLD, 'cold_hull': COLD_HULL,
}


def new_canvas():
    img = Image.new('RGB', (SIZE, SIZE), BG)
    return img, ImageDraw.Draw(img)


# --- Библиотека форм (13 шт, параметрические: нос справа) ---
# Каждая форма принимает цвет корпуса (hull): крем по умолчанию, холодный
# бледно-голубой — для холодных рас (диагноз визуального аудита).
# Общие требования (диагноз визуального аудита, 2026-09-20): каждая форма —
# выразительный характерный силуэт (не овал): яркая асимметрия «нос справа,
# корма слева» (масса справа заметно ≠ 50%, диапазон на форму — в комментарии
# и пиксельном тесте), разнообразная заполненность bbox по Y (у форм с
# крыльями/оперением размах по Y заметный), палитра art_ships §2.1 (корпус
# крем/холодный, крылья сталь, хвост бронза, дюзы тёмные, кабина светлая,
# границы (12,12,12)), запас ~90 px от краёв.

def draw_wing(d, hull=CREAM):
    # крыло (humans): корпус-фюзеляж (широкий, чуть левее центра) + носовой
    # конус (справа) + стальные стреловидные крылья с размахом по Y (~90% H).
    # Дюзы слева, кабина справа. Корма-форма: масса ~40-48% справа (крылья
    # уходят назад-влево), y-протяжённость ≥ 40% холста.
    d.rounded_rectangle([X0 + W * 0.05, Y0 + H * 0.28, X0 + W * 0.85, Y0 + H * 0.72],
                        radius=40, fill=hull, outline=BORDER, width=5)
    d.polygon([
        (X0 + W * 0.85, Y0 + H * 0.42), (X0 + W, Y0 + H * 0.5),
        (X0 + W * 0.85, Y0 + H * 0.58),
    ], fill=hull, outline=BORDER, width=5)
    d.polygon([
        (X0 + W * 0.5, Y0 + H * 0.4), (X0 + W * 0.06, Y0 + H * 0.04),
        (X0 + W * 0.14, Y0 + H * 0.22), (X0 + W * 0.5, Y0 + H * 0.4),
    ], fill=STEEL, outline=BORDER, width=5)
    d.polygon([
        (X0 + W * 0.5, Y0 + H * 0.6), (X0 + W * 0.06, Y0 + H * 0.96),
        (X0 + W * 0.14, Y0 + H * 0.78), (X0 + W * 0.5, Y0 + H * 0.6),
    ], fill=STEEL, outline=BORDER, width=5)


def draw_capsule(d, hull=CREAM):
    # капсула: широкий скруглённый корпус-корма (слева) + чёткий нос-конус
    # (справа). Не гладкий овал: сужение к носу, расширение к корме.
    # Корма-форма: масса ~40-48% справа.
    d.ellipse([X0, Y0 + H * 0.12, X0 + W * 0.85, Y0 + H * 0.88],
              fill=hull, outline=BORDER, width=5)
    d.polygon([
        (X0 + W * 0.85, Y0 + H * 0.38), (X0 + W, Y0 + H * 0.5),
        (X0 + W * 0.85, Y0 + H * 0.62),
    ], fill=hull, outline=BORDER, width=5)


def draw_barge(d, hull=CREAM):
    # баржа: широкая плоскодонная корма (слева), обрубленный нос (справа),
    # стальная надстройка-палуба (второй ярус) в средней части.
    # Корма-форма: масса ~40-48% справа.
    d.rounded_rectangle([X0 + W * 0.02, Y0 + H * 0.28, X0 + W * 0.95, Y0 + H * 0.72],
                        radius=30, fill=hull, outline=BORDER, width=5)
    d.rounded_rectangle([X0 + W * 0.15, Y0 + H * 0.4, X0 + W * 0.55, Y0 + H * 0.6],
                        radius=20, fill=STEEL, outline=BORDER, width=5)


def draw_crucible(d, hull=CREAM):
    # тигель: корпус, расширяющийся кверху (широкий верх, узкая устойчивая
    # база), уши-ручки (сталь) по бокам, нос-выступ справа.
    # Нос-форма: масса ~55-65% справа.
    d.polygon([
        (X0 + W * 0.25, Y0 + H * 0.18), (X0 + W * 0.87, Y0 + H * 0.18),
        (X0 + W * 0.73, Y0 + H * 0.82), (X0 + W * 0.37, Y0 + H * 0.82),
    ], fill=hull, outline=BORDER, width=5)
    d.ellipse([X0 + W * 0.42, Y0 + H * 0.02, X0 + W * 0.58, Y0 + H * 0.18],
              fill=STEEL, outline=BORDER, width=5)
    d.ellipse([X0 + W * 0.42, Y0 + H * 0.82, X0 + W * 0.58, Y0 + H * 0.98],
              fill=STEEL, outline=BORDER, width=5)
    d.polygon([
        (X0 + W * 0.87, Y0 + H * 0.32), (X0 + W, Y0 + H * 0.5),
        (X0 + W * 0.87, Y0 + H * 0.68),
    ], fill=hull, outline=BORDER, width=5)


def draw_envelope(d, hull=CREAM):
    # оболочка/баллон: округлый корпус-баллон (чуть левее центра) +
    # бронзовое хвостовое оперение слева (вверх и вниз).
    # Корма-форма: масса ~40-48% справа.
    d.ellipse([X0 + W * 0.07, Y0 + H * 0.15, X0 + W * 0.87, Y0 + H * 0.85],
              fill=hull, outline=BORDER, width=5)
    d.polygon([
        (X0 + W * 0.12, Y0 + H * 0.3), (X0, Y0 + H * 0.08),
        (X0 + W * 0.05, Y0 + H * 0.35), (X0 + W * 0.12, Y0 + H * 0.3),
    ], fill=BRONZE, outline=BORDER, width=5)
    d.polygon([
        (X0 + W * 0.12, Y0 + H * 0.7), (X0, Y0 + H * 0.92),
        (X0 + W * 0.05, Y0 + H * 0.65), (X0 + W * 0.12, Y0 + H * 0.7),
    ], fill=BRONZE, outline=BORDER, width=5)


def draw_vessel(d, hull=CREAM):
    # сосуд: бутыль на боку — широкое тело (эллипс), узкое горлышко-нос
    # (справа), суженное дно (слева). Корма-форма: масса ~40-48% справа.
    d.ellipse([X0 + W * 0.1, Y0 + H * 0.15, X0 + W * 0.75, Y0 + H * 0.85],
              fill=hull, outline=BORDER, width=5)
    d.rectangle([X0 + W * 0.75, Y0 + H * 0.4, X0 + W * 0.97, Y0 + H * 0.6],
                fill=hull, outline=BORDER, width=5)
    d.polygon([
        (X0 + W * 0.1, Y0 + H * 0.3), (X0, Y0 + H * 0.5),
        (X0 + W * 0.1, Y0 + H * 0.7),
    ], fill=hull, outline=BORDER, width=5)


def draw_flask(d, hull=CREAM):
    # колба: широкое круглое тело, узкое горлышко-нос (справа), плоское дно
    # (слева). Корма-форма: масса ~40-48% справа.
    d.rounded_rectangle([X0 + W * 0.1, Y0 + H * 0.2, X0 + W * 0.8, Y0 + H * 0.8],
                        radius=40, fill=hull, outline=BORDER, width=5)
    d.rectangle([X0 + W * 0.8, Y0 + H * 0.42, X0 + W * 0.95, Y0 + H * 0.58],
                fill=hull, outline=BORDER, width=5)


def draw_drop(d, hull=CREAM):
    # капля: каплевидная — заострённый нос (справа), широкая скруглённая
    # корма (слева). Яркая асимметрия. Корма-форма: масса ~40-48% справа.
    d.polygon([
        (X0 + W, Y0 + H * 0.5), (X0 + W * 0.72, Y0 + H * 0.3),
        (X0 + W * 0.45, Y0 + H * 0.16), (X0 + W * 0.2, Y0 + H * 0.18),
        (X0 + W * 0.1, Y0 + H * 0.36), (X0 + W * 0.1, Y0 + H * 0.64),
        (X0 + W * 0.2, Y0 + H * 0.82), (X0 + W * 0.45, Y0 + H * 0.84),
        (X0 + W * 0.72, Y0 + H * 0.7),
    ], fill=hull, outline=BORDER, width=5)


def draw_wedge(d, hull=CREAM):
    # клин: треугольный в плане — широкая корма (слева), острый нос (справа).
    # Корма-форма: масса ~38-48% справа.
    d.polygon([
        (X0 + W * 0.25, Y0 + H * 0.15), (X0 + W * 0.25, Y0 + H * 0.85),
        (X0 + W, Y0 + H * 0.5),
    ], fill=hull, outline=BORDER, width=5)


def draw_disc(d, hull=CREAM):
    # диск: сплюснутый (малый размах по Y), яйцевидный — шире к корме (слева),
    # выступ-кабина (сталь) в носу справа, дюзы по корме слева.
    # Корма-форма: масса ~42-48% справа.
    d.ellipse([X0 + W * 0.02, Y0 + H * 0.3, X0 + W * 0.8, Y0 + H * 0.7],
              fill=hull, outline=BORDER, width=5)
    d.rounded_rectangle([X0 + W * 0.78, Y0 + H * 0.38, X0 + W * 0.95, Y0 + H * 0.62],
                        radius=20, fill=STEEL, outline=BORDER, width=5)


def draw_ring(d, hull=CREAM):
    # кольцо: круг-обод с центральной ступицей-корпусом (чуть правее центра)
    # и носом-выступом (справа). Нос-форма: масса ~52-60% справа.
    cx, cy = X0 + W * 0.5, Y0 + H * 0.5
    r_out = int(W * 0.38)
    r_in = int(W * 0.2)
    d.ellipse([cx - r_out, cy - r_out, cx + r_out, cy + r_out],
              fill=hull, outline=BORDER, width=5)
    d.ellipse([cx - r_in, cy - r_in, cx + r_in, cy + r_in],
              fill=BG, outline=BORDER, width=5)
    r_hub = int(W * 0.11)
    hx = cx + int(W * 0.02)
    d.ellipse([hx - r_hub, cy - r_hub, hx + r_hub, cy + r_hub],
              fill=hull, outline=BORDER, width=5)
    d.rounded_rectangle([cx + r_out - 20, cy - int(H * 0.22), cx + r_out + int(W * 0.12), cy + int(H * 0.22)],
                        radius=25, fill=hull, outline=BORDER, width=5)


def draw_sphere(d, hull=CREAM):
    # шар: круглый корпус (смещён вправо), кабина-выступ (сталь) в носу
    # справа, хвостовые стабилизаторы (бронза) слева.
    # Нос-форма: масса ~55-62% справа.
    r = int(W * 0.32)
    cx, cy = X0 + W * 0.56, Y0 + H * 0.5
    d.ellipse([cx - r, cy - r, cx + r, cy + r], fill=hull, outline=BORDER, width=5)
    d.rounded_rectangle([cx + r - 15, cy - int(H * 0.2), cx + r + int(W * 0.11), cy + int(H * 0.2)],
                        radius=25, fill=STEEL, outline=BORDER, width=5)
    d.polygon([
        (cx - r + 10, cy - 10), (X0 + W * 0.02, cy - int(H * 0.3)),
        (X0 + W * 0.02, cy - int(H * 0.12)), (cx - r + 10, cy - 10),
    ], fill=BRONZE, outline=BORDER, width=5)
    d.polygon([
        (cx - r + 10, cy + 10), (X0 + W * 0.02, cy + int(H * 0.3)),
        (X0 + W * 0.02, cy + int(H * 0.12)), (cx - r + 10, cy + 10),
    ], fill=BRONZE, outline=BORDER, width=5)


def draw_swarm(d, hull=CREAM):
    # рой: россыпь мелких модулей-осколков в общем контуре (не сплошной блоб):
    # плотный рой в носу (справа), редкий шлейф к корме (слева).
    # Нос-форма: масса ~55-65% справа.
    mods = [
        (0.82, 0.5, 30), (0.78, 0.3, 26), (0.74, 0.7, 28), (0.7, 0.45, 34),
        (0.66, 0.6, 24), (0.62, 0.25, 30), (0.58, 0.75, 26),
        (0.52, 0.42, 32), (0.48, 0.62, 28), (0.44, 0.2, 24), (0.4, 0.8, 26),
        (0.32, 0.5, 30), (0.26, 0.3, 22), (0.2, 0.7, 24), (0.14, 0.45, 20),
    ]
    for fx, fy, r in mods:
        x = X0 + int(W * fx)
        y = Y0 + int(H * fy)
        d.ellipse([x - r, y - r, x + r, y + r], fill=hull, outline=BORDER, width=4)


FORM_FUNCS = {
    'wing': draw_wing, 'capsule': draw_capsule, 'barge': draw_barge,
    'crucible': draw_crucible, 'envelope': draw_envelope, 'vessel': draw_vessel,
    'flask': draw_flask, 'drop': draw_drop, 'wedge': draw_wedge,
    'disc': draw_disc, 'ring': draw_ring, 'sphere': draw_sphere,
    'swarm': draw_swarm,
}


# --- Модули (поверх формы, контрастные цвета палитры) ---

def draw_modules(d, modules):
    for m in modules:
        typ = m.get('type', '')
        col = COLOR_MAP.get(m.get('color', 'steel'), STEEL)
        if typ == 'wings':
            # стальные полигоны по бокам
            d.polygon([
                (X0 + W * 0.3, Y0 + H * 0.05), (X0 + W * 0.55, Y0 + H * 0.12),
                (X0 + W * 0.55, Y0 + H * 0.3), (X0 + W * 0.3, Y0 + H * 0.2),
            ], fill=col, outline=BORDER, width=4)
            d.polygon([
                (X0 + W * 0.3, Y0 + H * 0.95), (X0 + W * 0.55, Y0 + H * 0.88),
                (X0 + W * 0.55, Y0 + H * 0.7), (X0 + W * 0.3, Y0 + H * 0.8),
            ], fill=col, outline=BORDER, width=4)
        elif typ == 'tail':
            # оперение слева
            d.polygon([
                (X0, Y0 + H * 0.3), (X0 + W * 0.12, Y0 + H * 0.42),
                (X0 + W * 0.12, Y0 + H * 0.58), (X0, Y0 + H * 0.7),
            ], fill=col, outline=BORDER, width=4)
        elif typ == 'nozzles':
            # тёмные блоки слева
            for i in range(3):
                ny = Y0 + H * (0.35 + i * 0.15)
                d.rectangle([X0 + W * 0.02, ny, X0 + W * 0.1, ny + H * 0.1],
                            fill=col, outline=BORDER, width=3)
        elif typ == 'cockpit':
            # светлая кабина справа
            d.rounded_rectangle([X0 + W * 0.78, Y0 + H * 0.3, X0 + W * 0.96, Y0 + H * 0.7],
                                radius=30, fill=col, outline=BORDER, width=4)
        elif typ == 'glow':
            # свечение: точки по корпусу
            for i in range(5):
                gx = X0 + W * (0.2 + i * 0.15)
                gy = Y0 + H * (0.3 + (i % 3) * 0.2)
                d.ellipse([gx - 12, gy - 12, gx + 12, gy + 12], fill=col)
        elif typ == 'channels':
            # каналы: линии вдоль корпуса
            for i in range(3):
                cy = Y0 + H * (0.35 + i * 0.15)
                d.line([X0 + W * 0.15, cy, X0 + W * 0.85, cy], fill=col, width=6)
        elif typ == 'stilts':
            # сваи: линии по бокам
            for i in range(3):
                sx = X0 + W * (0.2 + i * 0.25)
                d.line([sx, Y0 + H * 0.1, sx, Y0 + H * 0.9], fill=col, width=8)
        elif typ == 'tentacles':
            # щупальца: линии от кормы влево
            for i in range(4):
                ty = Y0 + H * (0.3 + i * 0.13)
                d.line([X0, ty, X0 - 60, ty + 30 * (i % 2)], fill=col, width=6)
        elif typ == 'sails':
            # паруса: дуги по кромкам
            for s in (-1, 1):
                d.arc([X0 + W * 0.2, Y0 + H * 0.1, X0 + W * 0.8, Y0 + H * 0.9],
                      start=0 if s > 0 else 180, end=180 if s > 0 else 360,
                      fill=col, width=6)
        elif typ == 'gills':
            # жабры: линии по бокам
            for i in range(4):
                gy = Y0 + H * (0.2 + i * 0.15)
                d.line([X0 + W * 0.15, gy, X0 + W * 0.35, gy], fill=col, width=5)
        elif typ == 'bubbles':
            # пузыри: круги на корпусе
            for i in range(4):
                bx = X0 + W * (0.25 + i * 0.15)
                by = Y0 + H * (0.3 + (i % 2) * 0.3)
                d.ellipse([bx - 15, by - 15, bx + 15, by + 15],
                          fill=col, outline=BORDER, width=3)
        elif typ == 'trails':
            # шлейфы: линии за кормой влево
            for i in range(3):
                ty = Y0 + H * (0.35 + i * 0.15)
                d.line([X0, ty, X0 - 80, ty + 20 * (i % 2)], fill=col, width=5)
        elif typ == 'patches':
            # заплаты: светлые пятна-эллипсы по бортам с тёмной границей
            for i in range(4):
                px = X0 + W * (0.15 + i * 0.11)
                py = Y0 + H * (0.25 + (i % 2) * 0.5)
                d.ellipse([px - 25, py - 16, px + 25, py + 16],
                          fill=col, outline=BORDER, width=3)


def draw_light_nose(d):
    # светлая секция носа справа (если кабины-модуля нет)
    d.rounded_rectangle([X0 + W * 0.8, Y0 + H * 0.32, X0 + W * 0.97, Y0 + H * 0.68],
                        radius=25, fill=LIGHT, outline=BORDER, width=4)


def draw_fire(d):
    # FIRE-модуль: тёплое свечение у дюз (слева)
    for i in range(3):
        ny = Y0 + H * (0.35 + i * 0.15)
        d.polygon([
            (X0 + W * 0.02, ny + H * 0.02), (X0 - 50, ny + H * 0.05),
            (X0 - 50, ny + H * 0.08), (X0 + W * 0.02, ny + H * 0.1),
        ], fill=FIRE)


def main():
    ap = argparse.ArgumentParser(description='Силуэт корабля расы из spec-JSON')
    ap.add_argument('--spec', required=True, help='путь к spec-JSON (от Go-парсера)')
    ap.add_argument('--out', required=True, help='путь к выходному PNG')
    args = ap.parse_args()

    with open(args.spec, encoding='utf-8') as f:
        spec = json.load(f)

    img, d = new_canvas()
    form = spec.get('form', 'capsule')
    draw_fn = FORM_FUNCS.get(form, draw_capsule)
    # цвет корпуса: модуль hull (крем по умолчанию, холодный бледно-голубой
    # для холодных рас — диагноз визуального аудита)
    hull_col = CREAM
    for m in spec.get('modules') or []:
        if m.get('type') == 'hull':
            hull_col = COLOR_MAP.get(m.get('color', 'cream'), CREAM)
            break
    draw_fn(d, hull_col)
    draw_modules(d, spec.get('modules') or [])
    has_cockpit = any(m.get('type') == 'cockpit' for m in spec.get('modules') or [])
    if spec.get('light_nose') and not has_cockpit:
        draw_light_nose(d)
    if spec.get('warm_glow'):
        draw_fire(d)

    os.makedirs(os.path.dirname(args.out), exist_ok=True)
    img.save(args.out)
    print('Saved: %s (%s)' % (args.out, form))


if __name__ == '__main__':
    main()