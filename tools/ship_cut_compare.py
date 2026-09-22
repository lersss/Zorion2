# -*- coding: utf-8 -*-
# Сравнение методов выреза спрайта корабля на ОДНОМ сыром кадре, с нормализацией
# студии. Колонки: raw | hyst 12/40 (прежний рецепт) | edge 40/20 | chroma 60/25
# | edge 100/20 (рецепт 2026-09-22: magenta-фон + edge с увеличенным tol).
# Назначение — визуальная приёмка: magenta-фон возвращает корпус (не режет
# насквозь), а chroma на градиентном фоне оставляет ореол.
# Запуск:
#   python tools/ship_cut_compare.py --out DIR raw1.png [raw2.png ...]
import argparse
import os
import sys

from PIL import Image, ImageDraw

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import ship_sprite_cut as sc  # noqa: E402

# (заголовок, метод, параметры) — колонки сравнения.
COLUMNS = [
    ("hyst 12/40", "hyst", (12, 40)),
    ("edge 40/20", "edge", (40, 20)),
    ("chroma 60/25", "chroma", (60, 25)),
    ("edge 100/20", "edge", (100, 20)),
]


def cut_sprite(img, method, params, canvas):
    """Вырез + нормализация (как студийный конвейер) для метода hyst/edge/chroma."""
    if method == "hyst":
        cut = sc.remove_bg_hyst(img, params[0], params[1])
    elif method == "chroma":
        cut = sc.remove_bg_chroma(img, params[0], params[1])
    else:
        cut = sc.remove_bg_edge(img, params[0], params[1])
    cut = sc.fill_holes(cut)
    sprite, _ = sc.normalize(cut, canvas=canvas, fill=1.0, no_orient=True)
    return sprite


def on_grey(img, size):
    g = Image.new("RGB", img.size, (128, 128, 128))
    g.paste(img, (0, 0), img)
    return g.resize((size, size))


def main():
    ap = argparse.ArgumentParser(
        description="Сравнение методов выреза корабля (hyst/edge/chroma)")
    ap.add_argument("--out", required=True, help="каталог для PNG-сравнений")
    ap.add_argument("--canvas", type=int, default=200)
    ap.add_argument("--cell", type=int, default=260)
    ap.add_argument("raw", nargs="+", help="сырые кадры Juggernaut (PNG)")
    args = ap.parse_args()
    os.makedirs(args.out, exist_ok=True)

    for path in args.raw:
        stem = os.path.splitext(os.path.basename(path))[0]
        img = Image.open(path)
        cell = args.cell
        cols = len(COLUMNS) + 1
        board = Image.new("RGB", (cell * cols + 6 * (cols + 1), cell + 26), (18, 18, 18))
        d = ImageDraw.Draw(board)
        board.paste(img.convert("RGB").resize((cell, cell)), (6, 22))
        d.text((6, 5), "raw", fill=(255, 255, 0))
        for i, (label, method, params) in enumerate(COLUMNS):
            x = cell * (i + 1) + 6 * (i + 2)
            sprite = cut_sprite(img, method, params, args.canvas)
            board.paste(on_grey(sprite, cell), (x, 22))
            color = (120, 255, 120) if label.startswith("edge 100") else (255, 200, 120)
            d.text((x, 5), label, fill=color)
        out = os.path.join(args.out, stem + "_methods.png")
        board.save(out)
        print("saved %s" % out)


if __name__ == "__main__":
    main()
