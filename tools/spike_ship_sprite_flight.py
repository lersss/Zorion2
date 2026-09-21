# -*- coding: utf-8 -*-
# Артефакт-превью полёта (2026-09-21): финальные спрайты кораблей расы «люди»
# в РЕАЛЬНОМ игровом масштабе полёта, повёрнутые на 0..315° (шаг 45°), с
# пламенем двигателя как в игре (map_render.js drawFlight: градиент
# rgba(147,197,253,.9)->rgba(59,130,246,.6)->rgba(59,130,246,0), длина 1.1*s,
# спрайт 3.2*s), на тёмном фоне карты (#0f172a).
# Цель — видеть, как спрайт читается в повороте и не «плывёт» ли нос
# (нос/пламя при полёте вправо). Вывод: ai_drafts/sprite_ships/flight_preview.png.
import math
import os

from PIL import Image, ImageDraw, ImageFont

ROOT = r"C:\Zorion2"
SPRITES = os.path.join(ROOT, "web", "static", "sprites")
OUT = os.path.join(ROOT, "ai_drafts", "sprite_ships")
SHIP_SIZE = 2.8          # web/static/js/config.js: shipSize (реальный масштаб)
SS = 8                   # супер-сэмплинг растеризации
BG = (15, 23, 42, 255)   # #0f172a — фон карты
CELL = 100
LAB = 190
ANGLES = list(range(0, 360, 45))
SHIPS = ["race_humans_starship.png", "race_humans_cruiser.png", "race_humans_carrier.png"]


def _font(size=13):
    for n in ("arial.ttf", "DejaVuSans.ttf"):
        try:
            return ImageFont.truetype(n, size)
        except Exception:
            continue
    return ImageFont.load_default()


def _lerp(a, b, t):
    return tuple(int(round(a[i] + (b[i] - a[i]) * t)) for i in range(4))


def render_ship(sprite, angle_deg, ship_size=SHIP_SIZE):
    """Спрайт с пламенем в локальных координатах (нос вправо) → поворот на
    angle_deg → RGBA-тайл реального масштаба ship_size*3.2 px."""
    s = ship_size
    flame_len = s * 1.1
    half = 1.6 * s
    radius = half + flame_len + 1.0
    side = int(math.ceil(radius * 2 * SS))
    im = Image.new("RGBA", (side, side), (0, 0, 0, 0))
    d = ImageDraw.Draw(im)
    cx = cy = side // 2
    S = s * SS
    grad = [(147, 197, 253, 229), (59, 130, 246, 153), (59, 130, 246, 0)]
    x0 = -0.8 * S
    steps = 60
    for i in range(steps + 1):
        t = i / float(steps)
        if t < 0.35:
            c = _lerp(grad[0], grad[1], t / 0.35)
        else:
            c = _lerp(grad[1], grad[2], (t - 0.35) / 0.65)
        x = x0 - flame_len * SS * t
        hh = 0.28 * S * (1.0 - t)
        d.line([(cx + x, cy - hh), (cx + x, cy + hh)], fill=c, width=max(1, SS // 4))
    spr = sprite.convert("RGBA").resize((int(round(3.2 * S)), int(round(3.2 * S))), Image.LANCZOS)
    im.alpha_composite(spr, (int(cx - 1.6 * S), int(cy - 1.6 * S)))
    im = im.rotate(-angle_deg, resample=Image.BICUBIC)
    return im.resize((int(round(side / float(SS))), int(round(side / float(SS)))), Image.LANCZOS)


def cell_tile(sprite, angle, zoom):
    tile = Image.new("RGBA", (CELL, CELL), BG)
    sh = render_ship(sprite, angle)
    if zoom > 1:
        sh = sh.resize((sh.width * zoom, sh.height * zoom), Image.NEAREST)
    tile.alpha_composite(sh, (max(0, (CELL - sh.width) // 2), max(0, (CELL - sh.height) // 2)))
    return tile


def main():
    zoom_levels = ((1, "x1 real"), (4, "x4 zoom"))
    W = LAB + CELL * len(ANGLES)
    H = 30 + len(SHIPS) * (CELL * len(zoom_levels) + 8 * len(zoom_levels) + 34) + 20
    im = Image.new("RGB", (W, H), (10, 15, 26))
    d = ImageDraw.Draw(im)
    f = _font(14)
    fs = _font(12)
    d.text((8, 8), "FLIGHT PREVIEW — humans ships, real in-game scale (shipSize=%.1f, sprite=%.1f px); "
                   "nose must point RIGHT under thrust" % (SHIP_SIZE, SHIP_SIZE * 3.2),
           fill=(235, 235, 240), font=f)
    y = 30
    for ship in SHIPS:
        sprite = Image.open(os.path.join(SPRITES, ship)).convert("RGBA")
        for zoom, tag in zoom_levels:
            d.text((6, y + CELL // 2 - 8), ship.replace("race_humans_", "").replace(".png", ""),
                   fill=(255, 210, 90), font=f)
            d.text((6, y + CELL // 2 + 8), tag, fill=(150, 170, 200), font=fs)
            for i, a in enumerate(ANGLES):
                tile = cell_tile(sprite, a, zoom)
                im.paste(tile, (LAB + i * CELL, y), tile)
                d.text((LAB + i * CELL + 3, y + 2), "%d" % a, fill=(120, 140, 170), font=fs)
            y += CELL + 8
        y += 34
    im.save(os.path.join(OUT, "flight_preview.png"), "PNG")
    print("flight_preview: %s (%dx%d)" % (os.path.join(OUT, "flight_preview.png"), W, H))


if __name__ == "__main__":
    main()
