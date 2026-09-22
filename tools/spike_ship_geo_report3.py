# -*- coding: utf-8 -*-
# СПАЙК 4: метрики прогона «болванка -> арт», сравнение со спайками 1–3, базовым
# пулом и эталоном 21 спрайта; монтаж preview_montage_v3.png и лестница ракурсов
# ladder_angles.png; дописывание метрик в geo_meta.json (entries_v3).
# Считается ПО ФАЙЛАМ (GPU не нужен) — можно перезапускать сколько угодно.
import json
import os

import numpy as np
from PIL import Image, ImageDraw

from spike_ship_geo_report2 import (SPRITES, load_font, metrics, reference, spike_metrics)

ROOT = r"C:\Zorion2"
OUT = os.path.join(ROOT, "ai_drafts", "geometry_ships")
RENDER3 = os.path.join(OUT, "render3")

# колонки монтажа: [рендер 3/4 | лучшие комбинации CN×denoise]
COMBO_COLS = [
    ("d06", "depth 0.6 · dn0.80"),
    ("d08", "depth 0.8 · dn0.80"),
    ("d10", "depth 1.0 · dn0.80"),
    ("c08", "canny 0.8 · dn0.80"),
    ("st", "stack 0.8+0.8 · dn0.80"),
    ("d08n70", "depth 0.8 · dn0.70"),
    ("d08n90", "depth 0.8 · dn0.90"),
]


def render_path(slug, elev=25):
    return os.path.join(RENDER3, "%s_e%02d.png" % (slug, int(elev)))


def montage_v3(entries, path):
    byid = {e["id"]: e for e in entries}
    cell, pad, lbl, cap = 200, 8, 22, 18
    font = load_font(14)
    font_small = load_font(12)
    rows = []
    for slug in ("humans", "coastal"):
        cells = [(render_path(slug, 25), "рендер 3/4 (25°)")]
        for tag, capv in COMBO_COLS:
            e = byid.get("%s_%s" % (slug, tag))
            cells.append((e["candidate"] if e else None, capv))
        rows.append((slug, cells))
    # доп. строки humans: промпты, LoRA, Juggernaut
    extra = [("humans_d08p1", "humans depth0.8 · промпт p1 (concept-art)"),
             ("humans_d08p2", "humans depth0.8 · промпт p2 (hard-surface)"),
             ("humans_d08p3", "humans depth0.8 · промпт p3 (clean, без грязи)"),
             ("coastal_d08p3", "coastal depth0.8 · промпт p3 (clean, без грязи)"),
             ("humans_d08lora", "humans depth0.8 dn0.85 · LoRA s3r3nity 0.6"),
             ("humans_d08_jugg", "humans depth0.8 dn0.80 · Juggernaut XL v9")]
    for jid, label in extra:
        e = byid.get(jid)
        if not e:
            continue
        rows.append((label, [(render_path(e["race"], e["elev"]), "рендер"),
                             (e["candidate"], "результат")]))
    # crystallites (зонд)
    e = byid.get("crystallites_d08")
    if e:
        rows.append(("crystallites · зонд depth0.8 dn0.80",
                     [(render_path("crystallites", 25), "рендер 3/4"),
                      (e["candidate"], "результат")]))
    ncols = max(len(c) for _, c in rows)
    W = pad + ncols * (cell + pad)
    H = pad + len(rows) * (lbl + cell + cap + pad)
    canvas = Image.new('RGB', (W, H), (18, 18, 22))
    d = ImageDraw.Draw(canvas)
    for ri, (label, cells) in enumerate(rows):
        y = pad + ri * (lbl + cell + cap + pad)
        d.text((pad, y), label, fill=(235, 235, 235), font=font)
        for ci, (p, capv) in enumerate(cells):
            x = pad + ci * (cell + pad)
            if p and os.path.exists(p):
                canvas.paste(Image.open(p).convert('RGB').resize((cell, cell), Image.LANCZOS),
                             (x, y + lbl))
            d.text((x, y + lbl + cell + 2), capv, fill=(180, 200, 220), font=font_small)
    canvas.save(path)
    return path


def ladder_angles(entries, path):
    """humans: рендер и результат на углах 0/15/25/40° при фиксированном denoise."""
    byid = {e["id"]: e for e in entries}
    cell, pad, lbl, cap = 300, 10, 24, 20
    font = load_font(16)
    font_small = load_font(13)
    angles = [(0, "humans_e00"), (15, "humans_e15"), (25, "humans_d08"), (40, "humans_e40")]
    W = pad + 4 * (cell + pad)
    H = pad + 2 * (lbl + cell + cap + pad)
    canvas = Image.new('RGB', (W, H), (18, 18, 22))
    d = ImageDraw.Draw(canvas)
    for ri, (kind, y0) in enumerate((("render", 0), ("result", 1))):
        y = pad + ri * (lbl + cell + cap + pad)
        d.text((pad, y), "рендер (спайк 4)" if kind == "render" else "ИИ-результат (depth 0.8 · dn0.80)",
               fill=(235, 235, 235), font=font)
        for ci, (el, jid) in enumerate(angles):
            x = pad + ci * (cell + pad)
            p = render_path("humans", el) if kind == "render" else (
                byid[jid]["candidate"] if jid in byid else None)
            if p and os.path.exists(p):
                canvas.paste(Image.open(p).convert('RGB').resize((cell, cell), Image.LANCZOS),
                             (x, y + lbl))
            d.text((x, y + lbl + cell + 2), "elev %d°" % el, fill=(180, 200, 220), font=font_small)
    canvas.save(path)
    return path


def main():
    meta_path = os.path.join(OUT, "geo_meta.json")
    with open(meta_path, encoding="utf-8") as f:
        meta = json.load(f)
    entries = meta.get("entries_v3", [])
    for e in entries:
        rp = render_path(e["race"], e["elev"])
        e["metrics"] = metrics(e["candidate"], rp)
        if e.get("final_raw") and os.path.exists(e["final_raw"]) and e["final_raw"] != e["candidate"]:
            e["metrics_raw"] = metrics(e["final_raw"], rp)
    meta["entries_v3"] = entries
    meta["reference_sprites"] = reference()
    meta["comparison_spikes"] = spike_metrics()
    meta["spike3_metrics"] = spike3_metrics()
    meta["ships_pool_metrics"] = pool_metrics()
    meta["montage_v3"] = montage_v3(entries, os.path.join(OUT, "preview_montage_v3.png"))
    meta["ladder_angles"] = ladder_angles(entries, os.path.join(OUT, "ladder_angles.png"))
    with open(meta_path, "w", encoding="utf-8") as f:
        json.dump(meta, f, ensure_ascii=False, indent=1)

    ref = meta["reference_sprites"]
    print("REFERENCE 21 sprites: n=%s colors_median=%s edges(canvas)_median=%s edges(fg)_median=%s" % (
        ref["n"], ref["colors_median"], ref["edges_canvas_median"], ref["edges_fg_median"]))
    print("SPIKE3:", {k: (v.get("unique_colors"), v.get("edge_density_fg"), v.get("edge_density_canvas"))
                      for k, v in meta["spike3_metrics"].items()})
    print("SPIKE1:", {k: (v.get("unique_colors"), v.get("edge_density_fg"))
                      for k, v in meta["comparison_spikes"].get("spike1", {}).items()})
    pool = meta["ships_pool_metrics"]
    if pool:
        print("POOL baseline: colors_median=%s edgesF_median=%s" % (
            int(np.median([v["unique_colors"] for v in pool.values()])),
            round(float(np.median([v["edge_density_fg"] for v in pool.values()])), 4)))
    print("--- entries_v3 ---")
    for e in sorted(entries, key=lambda x: x["id"]):
        m = e["metrics"]
        print("%-22s el=%-3s | colors=%-5s edgesF=%.3f edgesC=%.3f comp=%-3s IoU=%-6s" % (
            e["id"], e["elev"], m.get("unique_colors"), m.get("edge_density_fg", 0),
            m.get("edge_density_canvas", 0), m.get("components"), m.get("iou_vs_render")))


def spike3_metrics():
    comp = {}
    r2 = os.path.join(OUT, "render2")
    for slug in ("humans", "coastal", "crystallites"):
        for v in ("V1g", "V2g", "V5"):
            p = os.path.join(OUT, "%s_%s_200.png" % (slug, v))
            if os.path.exists(p):
                comp["%s_%s" % (slug, v)] = metrics(p, os.path.join(r2, "%s_tilt.png" % slug))
    return comp


def pool_metrics():
    """Базовый пул (ai_drafts/ships_pool s01..s12) — сравнение «до». """
    pool = os.path.join(ROOT, "ai_drafts", "ships_pool")
    rows = {}
    for i in range(1, 13):
        p = os.path.join(pool, "s%02d.png" % i)
        if os.path.exists(p):
            rows["s%02d" % i] = metrics(p)
    return rows


if __name__ == "__main__":
    main()
