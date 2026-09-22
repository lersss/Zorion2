# -*- coding: utf-8 -*-
# Перерезка пула кораблей рас новым методом выреза БЕЗ перегона.
# Сопоставление: ai_drafts/ships_pool/sNN.png <-> сырой кадр ComfyUI
# (C:\ComfyUI\output\ship_pool_*.png) по seed из meta.json (seed == seed
# KSampler в PNG-метаданных; проверено: текущий вырез воспроизводится
# бит-в-бит). Неоднозначные (несколько кадров с тем же seed одного размера) и
# ненайденные НЕ трогаются — попадают в отчёт.
#
# Запуск (сухой прогон с таблицей):
#   python tools/ship_recut_pool.py --dry-run
# Перерезка:
#   python tools/ship_recut_pool.py
import argparse
import glob
import json
import os
import sys

import numpy as np
from PIL import Image

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import ship_sprite_cut as sc  # noqa: E402

DEFAULT_POOL = os.path.join("ai_drafts", "ships_pool")
DEFAULT_COMFY = r"C:\ComfyUI\output"


def index_comfy(comfy_out):
    """seed -> список (path, area) по PNG-метаданным ComfyUI (поле prompt)."""
    by_seed = {}
    for p in sorted(glob.glob(os.path.join(comfy_out, "ship_pool_*.png"))):
        try:
            img = Image.open(p)
            meta = img.info.get("prompt")
            if not meta:
                continue
            d = json.loads(meta)
        except Exception:
            continue
        seeds = set()
        for v in d.values():
            if isinstance(v, dict):
                s = v.get("inputs", {}).get("seed")
                if isinstance(s, int):
                    seeds.add(s)
        for s in seeds:
            by_seed.setdefault(s, []).append((p, img.size[0] * img.size[1]))
    return by_seed


def is_magenta_bg(img, thresh=40):
    """Фон кадра — magenta-хромакей? Смотрим рамку: фон magenta даёт
    «магента-ность» min(R,B)-G заметно > 0 (juggernaut ~85-110), чёрный фон ≈ 0.
    Сравнение с порогом по квартилю, чтобы шум/детали корпуса не решали."""
    arr = np.array(img.convert("RGB")).astype(np.float32)
    h, w = arr.shape[:2]
    b = max(2, min(h, w) // 50)
    border = np.concatenate([arr[:b, :].reshape(-1, 3), arr[-b:, :].reshape(-1, 3),
                             arr[:, :b].reshape(-1, 3), arr[:, -b:].reshape(-1, 3)])
    m = np.minimum(border[:, 0], border[:, 2]) - border[:, 1]
    return bool(np.percentile(m, 75) >= thresh)


def cut(img, tol, edge, canvas, method):
    if method == "hyst":
        c = sc.remove_bg_hyst(img, 12, 40)
    else:
        c = sc.remove_bg_edge(img, tol, edge)
    c = sc.fill_holes(c)
    sprite, _ = sc.normalize(c, canvas=canvas, fill=1.0, no_orient=True)
    return sprite


def frame_corner_residue(img, cut_rgba, frac=0.05, sz=None):
    arr = np.array(cut_rgba.convert("RGBA"))
    h, w = arr.shape[:2]
    if sz is None:
        sz = max(4, int(frac * min(h, w)))
    a = arr[:, :, 3] > 40
    return max(a[:sz, :sz].mean(), a[:sz, -sz:].mean(),
               a[-sz:, :sz].mean(), a[-sz:, -sz:].mean())


def main():
    ap = argparse.ArgumentParser(description="Перерезка пула кораблей (без перегона)")
    ap.add_argument("--pool", default=DEFAULT_POOL)
    ap.add_argument("--comfy-output", default=DEFAULT_COMFY)
    ap.add_argument("--method", default="edge", choices=["edge", "hyst"])
    ap.add_argument("--tol", type=int, default=40,
                    help="tol выреза edge для ЧЁРНО-фоновых кадров (рецепт 2026-09-22: "
                         "tol=100 выедает тёмный корпус, на чёрном фоне рабочий 40)")
    ap.add_argument("--tol-magenta", type=int, default=100,
                    help="tol выреза edge для magenta-фоновых кадров (фон-градиент "
                         "шире, нужен 100)")
    ap.add_argument("--edge", type=int, default=20)
    ap.add_argument("--canvas", type=int, default=200)
    ap.add_argument("--dry-run", action="store_true")
    ap.add_argument("--report", default="")
    args = ap.parse_args()

    pool = args.pool
    meta_path = os.path.join(pool, "meta.json")
    items = json.load(open(meta_path, encoding="utf-8"))
    by_seed = index_comfy(args.comfy_output)

    rows = []
    hdr = "%-9s %-14s %-7s %5s %10s  %8s %8s %7s  %8s %8s" % (
        "file", "race", "bg", "tol", "seed", "area_b", "area_a", "dA", "cor_b", "cor_a")
    print(hdr)
    print("-" * len(hdr))
    problems = []
    for it in items:
        f = it.get("file", "")
        seed = it.get("seed")
        race = it.get("race", "")
        cands = by_seed.get(seed, [])
        # предпочитаем hires (самый крупный кадр); отбрасываем явно мелкие
        if not cands:
            problems.append((f, "сырой кадр не найден (seed %s)" % seed))
            continue
        cands = sorted(cands, key=lambda t: -t[1])
        top_area = cands[0][1]
        top = [c for c in cands if c[1] == top_area]
        if len(top) > 1:
            problems.append((f, "неоднозначно: %d кадров seed %s" % (len(top), seed)))
            continue
        raw_path = top[0][0]
        img = Image.open(raw_path)
        if img.size[0] < 1024:
            problems.append((f, "мелкий кадр %s" % (img.size,)))
            continue
        bg = "magenta" if is_magenta_bg(img) else "black"
        tol = args.tol_magenta if bg == "magenta" else args.tol
        before = cut(img, tol, args.edge, args.canvas, "hyst")
        after = cut(img, tol, args.edge, args.canvas, args.method)
        ab = (np.array(before)[:, :, 3] > 40).mean()
        aa = (np.array(after)[:, :, 3] > 40).mean()
        cor_b = frame_corner_residue(img, sc.fill_holes(sc.remove_bg_hyst(img, 12, 40)))
        cor_a = frame_corner_residue(img, sc.fill_holes(sc.remove_bg_edge(img, tol, args.edge)))
        rows.append((f, race, seed, ab, aa, aa - ab, cor_b, cor_a, raw_path, bg, tol))
        flag = ""
        if aa - ab > 0.02:
            flag = " +hull"
        if cor_a - cor_b > 0.05 and cor_b < 0.1:
            flag += " HAZE!"
        print("%-9s %-14s %-7s %5d %10d  %8.3f %8.3f %+7.3f  %8.3f %8.3f%s" % (
            f, race, bg, tol, seed, ab, aa, aa - ab, cor_b, cor_a, flag))
        if not args.dry_run:
            out = os.path.join(pool, f)
            after.save(out, "PNG")

    print("\nнайдено/перерезано: %d из %d" % (len(rows), len(items)))
    if problems:
        print("ПРОБЛЕМНЫЕ (не тронуты):")
        for f, why in problems:
            print("  %s — %s" % (f, why))
    if args.report:
        json.dump({"rows": [list(r) for r in rows],
                   "problems": problems}, open(args.report, "w", encoding="utf-8"),
                  ensure_ascii=False, indent=1)


if __name__ == "__main__":
    main()
