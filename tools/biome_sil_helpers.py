# -*- coding: utf-8 -*-
# Общие палитра и хелперы для силуэтов иконок биомов пачки 2 (стиль B, мини-пейзаж).
import os
from PIL import Image, ImageDraw

OUT = r'C:\Zorion2\ai_drafts\biomes\silhouettes'
SIZE = 1024
CX, CY = SIZE // 2, SIZE // 2
BORDER = (12, 12, 12)

GREEN = (88, 160, 90)
GREEN_D = (50, 110, 60)
GRASS = (120, 170, 80)
BLUE = (70, 130, 200)
BLUE_L = (140, 190, 235)
TEAL = (60, 180, 180)
CYAN = (140, 230, 230)
PURPLE = (150, 100, 200)
MAGENTA = (210, 100, 170)
PINK = (230, 140, 170)
CORAL = (230, 120, 120)
RED = (200, 60, 50)
CRIMSON = (170, 40, 60)
ORANGE = (230, 120, 40)
FIRE = (240, 150, 60)
YELLOW = (220, 190, 70)
SULFUR = (200, 190, 90)
WHITE = (235, 235, 235)
ICE = (190, 225, 240)
ICE_D = (120, 175, 210)
STEEL = (120, 140, 170)
STEEL_D = (80, 95, 120)
BROWN = (130, 95, 60)
TAR = (70, 55, 45)
DARK = (40, 44, 55)
GLASS = (180, 210, 235)
GOLD = (210, 170, 60)

SKY_DAY = (120, 160, 200)
SKY_PALE = (150, 185, 215)
SKY_DUSK = (60, 90, 130)
SKY_TWILIGHT = (70, 60, 110)
SKY_CRIMSON = (165, 95, 80)
SKY_SULFUR = (170, 155, 95)
SKY_TEAL = (80, 130, 130)
SKY_SMOG = (105, 92, 80)
SKY_ASH = (112, 100, 105)
SKY_LAVA = (150, 90, 50)
SKY_ICE = (170, 195, 210)
SKY_NIGHT = (22, 26, 40)
SKY_AMBER = (170, 145, 95)
SKY_VENUS = (200, 150, 80)
CLOUD = (205, 220, 235)
CLOUD_D = (155, 170, 190)

SNOW = (245, 248, 252)
SAND = (214, 178, 118)
SAND_D = (176, 140, 88)
ROCK = (122, 122, 128)
ROCK_D = (86, 86, 94)
ROCK_L = (162, 162, 170)
CARBON = (36, 38, 44)
CARBON_D = (24, 26, 32)
ORE = (150, 105, 70)
RUST = (150, 82, 55)
OBSID = (28, 28, 36)
VENUS = (226, 170, 92)
AMBER = (205, 172, 108)
JUNGLE = (36, 122, 56)
JUNGLE_D = (22, 84, 42)
METAL = (150, 165, 185)
METAL_D = (96, 112, 132)
METAL_L = (205, 218, 236)
RAD = (122, 232, 92)
RAD_D = (66, 176, 58)
DIAM = (212, 236, 246)
DIAM_D = (150, 190, 215)
BONE = (226, 218, 198)
SALT = (238, 240, 238)
SPRING = (132, 178, 120)
AMMONIA = (232, 208, 224)


def new_canvas():
    img = Image.new('RGB', (SIZE, SIZE), (5, 5, 5))
    return img, ImageDraw.Draw(img)


def save(img, name):
    os.makedirs(OUT, exist_ok=True)
    img.save(os.path.join(OUT, name + '.png'))
    print('Saved:', name)


def scene(d, sky, ground, hy):
    d.rectangle([0, 0, SIZE, hy], fill=sky)
    d.rectangle([0, hy, SIZE, SIZE], fill=ground)
    d.line([(0, hy), (SIZE, hy)], fill=BORDER, width=4)


def clouds(d, hy, color, xs, w=110, h=36):
    for cx in xs:
        d.ellipse([cx - w, hy - h, cx + w, hy + h], fill=color)
        d.ellipse([cx - w * 0.55, hy - h * 1.4, cx + w * 0.55, hy + h * 0.4], fill=color)


def tree(d, cx, cy, s, crown, trunk=DARK):
    d.polygon([(cx, cy - s), (cx - s * 0.7, cy), (cx + s * 0.7, cy)], fill=crown)
    d.line([(cx, cy - s * 0.5), (cx, cy + s * 0.2)], fill=trunk, width=max(8, int(s * 0.1)))


def ground3(d, hy, c1, c2, c3):
    """Три слоя грунта с волнистым верхом — чтобы низ не был плоским/однотонным."""
    d.polygon([(0, hy), (CX - 200, hy - 18), (CX + 80, hy + 10), (SIZE, hy - 8), (SIZE, hy + 150), (0, hy + 150)], fill=c1)
    d.line([(0, hy), (CX - 200, hy - 18), (CX + 80, hy + 10), (SIZE, hy - 8)], fill=BORDER, width=4)
    m = hy + 150
    d.polygon([(0, m), (CX - 100, m - 20), (CX + 120, m + 12), (SIZE, m - 6), (SIZE, m + 160), (0, m + 160)], fill=c2)
    d.line([(0, m), (CX - 100, m - 20), (CX + 120, m + 12), (SIZE, m - 6)], fill=BORDER, width=4)
    b = m + 160
    d.polygon([(0, b), (CX - 80, b - 16), (CX + 100, b + 10), (SIZE, b - 4), (SIZE, SIZE), (0, SIZE)], fill=c3)
    d.line([(0, b), (CX - 80, b - 16), (CX + 100, b + 10), (SIZE, b - 4)], fill=BORDER, width=4)


def spike(d, x, base_y, h, w, c):
    d.polygon([(x - w, base_y), (x, base_y - h), (x + w, base_y)], fill=c)
    d.line([(x - w, base_y), (x, base_y - h), (x + w, base_y), (x - w, base_y)], fill=BORDER, width=4)


def boulder(d, x, base_y, w, h, c):
    d.polygon([(x - w, base_y), (x - w * 0.4, base_y - h), (x + w * 0.5, base_y - h * 0.7), (x + w, base_y)], fill=c)
    d.line([(x - w, base_y), (x - w * 0.4, base_y - h), (x + w * 0.5, base_y - h * 0.7), (x + w, base_y)], fill=BORDER, width=4)
