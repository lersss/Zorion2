# -*- coding: utf-8 -*-
# СПАЙК 6: пакетный рендер кораблей SpaceshipGenerator разными seed'ами +
# контактный лист для отбора выразительных форм.
# ВАЖНО: Blender вызывается ОДИН раз на весь список (batch-режим драйвера
# --tasks), а не подпроцессом на корабль — иначе сторож зацикливания режет
# сессию. Обёртка над tools/blender_ship_render.py.
# Использование:
#   python tools/blender_ship_seeds.py --races humans,coastal,crystallites \
#       --variants 6 --out ai_drafts/blender_ships/probe --size 768
#   python tools/blender_ship_seeds.py --pairs humans=1715645483,coastal=1245074369 \
#       --out ai_drafts/blender_ships/raw --size 1536
#   python tools/blender_ship_seeds.py --seeds 1,2,3 --out ai_drafts/blender_ships/probe
# Seed расы выводится детерминированно (FNV-1a от slug) + смещение варианта.
# Характер расы задаётся пресетом RACE_GEN (машинная/органическая/кристаллическая).
import argparse
import json
import os
import subprocess
import sys

from PIL import Image, ImageDraw

ROOT = r"C:\Zorion2"
BLENDER = r"C:\Blender\blender-4.5.14-windows-x64\blender.exe"
DRIVER = os.path.join(ROOT, "tools", "blender_ship_render.py")
TILE = 384

# Пресеты «характера расы» — флаги генератора (имена сверены с
# spaceship_generator.generate_spaceship):
#   машинная  — стремительнее/симметричнее (больше сегментов корпуса, симметрия),
#   органическая — асимметричнее/мягче (много асимметрии, без горизонтальной
#                  симметрии, без фасок),
#   кристаллическая — гранёнее/блочнее (средний корпус, мало асимметрии).
RACE_GEN = {
    "humans":       ["--hull-min", "5", "--hull-max", "9", "--asym-min", "1", "--asym-max", "3"],
    "coastal":      ["--hull-min", "3", "--hull-max", "5", "--asym-min", "3", "--asym-max", "7",
                     "--no-hsym", "--no-bevel"],
    "crystallites": ["--hull-min", "4", "--hull-max", "7", "--asym-min", "1", "--asym-max", "2"],
}


def fnv1a(text):
    h = 2166136261
    for ch in text.encode("utf-8"):
        h ^= ch
        h = (h * 16777619) & 0xFFFFFFFF
    return h


def blender_batch(tasks_path, size, elev, yaw):
    """ОДИН запуск Blender на весь пакет задач."""
    cmd = [BLENDER, "--background", "--factory-startup", "--python", DRIVER, "--",
           "--tasks", tasks_path, "--size", str(size),
           "--elev", str(elev), "--yaw", str(yaw)]
    print("BLENDER batch: %d задач, size=%d" % (len(json.load(open(tasks_path, encoding="utf-8"))), size))
    res = subprocess.run(cmd, capture_output=True, text=True, encoding="utf-8", errors="replace")
    out = (res.stdout or "") + (res.stderr or "")
    for line in out.splitlines():
        if "RENDER_OK" in line or "RENDER_FAIL" in line or "BATCH_DONE" in line or "Error" in line:
            print("  " + line.strip())
    if res.returncode != 0:
        print("BLENDER_FAIL rc=%s\n%s" % (res.returncode, out[-2500:]))
    return res.returncode == 0


def build_montage(items, path, cols):
    rows = (len(items) + cols - 1) // cols
    sheet = Image.new("RGB", (cols * TILE, rows * TILE), (18, 18, 22))
    draw = ImageDraw.Draw(sheet)
    for i, it in enumerate(items):
        img = Image.open(os.path.join(it["dir"], "color.png")).convert("RGBA")
        bg = Image.new("RGBA", img.size, (12, 12, 16, 255))
        img = Image.alpha_composite(bg, img).convert("RGB")
        img.thumbnail((TILE - 8, TILE - 8), Image.LANCZOS)
        x = (i % cols) * TILE + (TILE - img.width) // 2
        y = (i // cols) * TILE + (TILE - img.height) // 2
        sheet.paste(img, (x, y))
        draw.text(((i % cols) * TILE + 6, (i // cols) * TILE + 6), it["label"], fill=(255, 220, 120))
    sheet.save(path)
    print("MONTAGE %s (%d tiles, %dx%d)" % (path, len(items), cols, rows))


def build_tasks(args):
    tasks = []
    fnv = {}
    if args.seeds:
        for s in args.seeds.split(","):
            s = s.strip()
            if s:
                tasks.append({"seed": int(s), "label": s, "race": "",
                              "out": os.path.join(args.out, "seed%08d" % (int(s) & 0xFFFFFFFF))})
    for slug in [x.strip() for x in args.races.split(",") if x.strip()]:
        base = fnv1a(slug)
        fnv[slug] = base
        for v in range(args.variants):
            seed = base + v
            tasks.append({"seed": seed, "label": "%s+%d" % (slug, v), "race": slug,
                          "out": os.path.join(args.out, "seed%08d" % (seed & 0xFFFFFFFF))})
    for pair in [x.strip() for x in args.pairs.split(",") if x.strip()]:
        slug, _, seed = pair.partition("=")
        slug, seed = slug.strip(), int(seed)
        tasks.append({"seed": seed, "label": "%s=%d" % (slug, seed), "race": slug,
                      "out": os.path.join(args.out, slug)})
    for t in tasks:
        t["gen_args"] = RACE_GEN.get(t["race"], [])
        t["out"] = os.path.abspath(t["out"])
    return tasks, fnv


def main():
    ap = argparse.ArgumentParser(description="Пакетный рендер кораблей + контактный лист")
    ap.add_argument("--seeds", default="")
    ap.add_argument("--races", default="")
    ap.add_argument("--pairs", default="", help="slug=seed,... (финалы в <out>/<slug>/)")
    ap.add_argument("--variants", type=int, default=5)
    ap.add_argument("--out", required=True)
    ap.add_argument("--size", type=int, default=768)
    ap.add_argument("--elev", type=float, default=25.0)
    ap.add_argument("--yaw", type=float, default=15.0)
    ap.add_argument("--cols", type=int, default=0, help="0 = авто (варианты в ряд)")
    args = ap.parse_args()

    tasks, fnv = build_tasks(args)
    if not tasks:
        ap.error("нужен --seeds, --races или --pairs")

    os.makedirs(args.out, exist_ok=True)
    tasks_path = os.path.join(os.path.abspath(args.out), "_tasks.json")
    with open(tasks_path, "w", encoding="utf-8") as f:
        json.dump(tasks, f, ensure_ascii=False, indent=1)
    blender_batch(tasks_path, args.size, args.elev, args.yaw)

    items = []
    for t in tasks:
        if os.path.exists(os.path.join(t["out"], "color.png")):
            items.append({"seed": t["seed"], "label": t["label"], "race": t["race"], "dir": t["out"]})
        else:
            print("MISS seed=%d out=%s" % (t["seed"], t["out"]))
    cols = args.cols or (args.variants if args.races else (len(tasks) if args.pairs else 5))
    build_montage(items, os.path.join(args.out, "probe_montage.png"), cols)
    with open(os.path.join(args.out, "probe_meta.json"), "w", encoding="utf-8") as f:
        json.dump({"size": args.size, "elev": args.elev, "yaw": args.yaw,
                   "race_gen": RACE_GEN, "fnv1a": fnv, "items": items},
                  f, ensure_ascii=False, indent=1)
    print("probe_meta: %s" % os.path.join(args.out, "probe_meta.json"))


if __name__ == "__main__":
    main()
