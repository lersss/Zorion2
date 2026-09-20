#!/usr/bin/env python3
# tools/make_planet_preview_montage.py
#
# Монтаж картинок планет (режим full, size=big) для приёмки доработки
# картинки планеты (спека 2026-09-21): сетка с подписями + детальный вид
# (землеподобная + гигант с кольцами).
#
# Использование: python tools/make_planet_preview_montage.py
# Вход:  C:\Users\admin\AppData\Local\Temp\opencode\previews\*.png
# Выход: ai_drafts/planet_images/preview_v3_montage.png
#        ai_drafts/planet_images/preview_v3_detail.png
import os
from PIL import Image, ImageDraw, ImageFont

SRC = r"C:\Users\admin\AppData\Local\Temp\opencode\previews"
OUT_DIR = r"C:\Zorion2\ai_drafts\planet_images"

# (файл, имя, тип)
PLANETS = [
    ("virno_earth.png", "Virno", "землеподобная"),
    ("odvilif_ice.png", "Odvilif", "ледяная"),
    ("rinrin_lava.png", "Rinrin", "вулканическая"),
    ("ognemsem_desert.png", "Ognemsem", "пустынная"),
    ("sununjar_ocean.png", "Sununjar", "океаническая"),
    ("renelar_organic.png", "Renelar", "органик"),
    ("benurdra_glass.png", "Benurdra", "стеклянная (Венера-режим)"),
    ("calmersor_dead.png", "Calmersor", "мёртвая"),
    ("nithe_radio.png", "Nithe", "радиоактивная"),
    ("ifgriic_rocky.png", "Ifgriic", "скалистая"),
    ("eisevax_gas2.png", "Eisevax", "газовый гигант + кольца"),
    ("challaexan_gas1.png", "Challaexan", "газовый гигант + кольца"),
]

DETAIL = [
    ("virno_earth.png", "Virno", "землеподобная"),
    ("eisevax_gas2.png", "Eisevax", "газовый гигант + кольца"),
]


def load(name):
    img = Image.open(os.path.join(SRC, name)).convert("RGBA")
    return img


def font(size):
    for cand in ("C:\\Windows\\Fonts\\arial.ttf", "C:\\Windows\\Fonts\\segoeui.ttf"):
        if os.path.exists(cand):
            return ImageFont.truetype(cand, size)
    return ImageFont.load_default()


def caption(draw, xy, text, fnt, fill=(230, 235, 245)):
    draw.text(xy, text, font=fnt, fill=fill)


def make_montage():
    cell = 512
    pad = 14
    cols, rows = 4, 3
    label_h = 52
    W = cols * cell + (cols + 1) * pad
    H = rows * (cell + label_h) + (rows + 1) * pad
    canvas = Image.new("RGBA", (W, H), (13, 15, 24, 255))
    draw = ImageDraw.Draw(canvas)
    fnt_name = font(26)
    fnt_type = font(20)

    for i, (fname, name, ptype) in enumerate(PLANETS):
        r, c = divmod(i, cols)
        x = pad + c * (cell + pad)
        y = pad + r * (cell + label_h + pad)
        img = load(fname)
        canvas.paste(img, (x, y))
        caption(draw, (x + 6, y + cell + 6), name, fnt_name)
        caption(draw, (x + 6, y + cell + 32), ptype, fnt_type, fill=(150, 160, 180))

    os.makedirs(OUT_DIR, exist_ok=True)
    out = os.path.join(OUT_DIR, "preview_v3_montage.png")
    canvas.convert("RGB").save(out)
    print("montage ->", out)


def make_detail():
    cell = 512
    pad = 20
    label_h = 56
    W = 2 * cell + 3 * pad
    H = cell + label_h + 2 * pad
    canvas = Image.new("RGBA", (W, H), (13, 15, 24, 255))
    draw = ImageDraw.Draw(canvas)
    fnt_name = font(30)
    fnt_type = font(22)

    for i, (fname, name, ptype) in enumerate(DETAIL):
        x = pad + i * (cell + pad)
        y = pad
        img = load(fname)
        canvas.paste(img, (x, y))
        caption(draw, (x + 8, y + cell + 8), name, fnt_name)
        caption(draw, (x + 8, y + cell + 38), ptype, fnt_type, fill=(150, 160, 180))

    os.makedirs(OUT_DIR, exist_ok=True)
    out = os.path.join(OUT_DIR, "preview_v3_detail.png")
    canvas.convert("RGB").save(out)
    print("detail ->", out)


if __name__ == "__main__":
    make_montage()
    make_detail()