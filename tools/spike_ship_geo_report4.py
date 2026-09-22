# -*- coding: utf-8 -*-
# СПАЙК 5: метрики прогона (CN-стек × промпт × свет × проходность), монтаж
# preview_montage_v4.png и сравнительная таблица. Считается ПО ФАЙЛАМ (GPU не
# нужен). Дописывает metrics в geo_meta.json (entries_v4).
import json
import os

import numpy as np
from PIL import Image, ImageDraw

from spike_ship_geo_report2 import SPRITES, load_font, metrics, reference

ROOT = r"C:\Zorion2"
OUT = os.path.join(ROOT, "ai_drafts", "geometry_ships")
CINE = os.path.join(OUT, "render4_cine")
FLAT = os.path.join(OUT, "render4_flat")


def cand(jid):
    return os.path.join(OUT, "v4_%s_200.png" % jid)


def rend(slug, light="cine", elev=25):
    base = CINE if light == "cine" else FLAT
    return os.path.join(base, "%s_e%02d.png" % (slug, int(elev)))


MONTAGE_ROWS = [
    ("humans · CN-стек (cine, dn0.80)", [
        (rend("humans"), "рендер 3/4"),
        (cand("humans_d08"), "depth 0.8"),
        (cand("humans_n06"), "d0.8 + normal 0.6"),
        (cand("humans_n08"), "d0.8 + normal 0.8"),
        (cand("humans_n10"), "d0.8 + normal 1.0")]),
    ("coastal · CN-стек", [
        (rend("coastal"), "рендер"),
        (cand("coastal_d08"), "depth 0.8"),
        (cand("coastal_n08"), "d0.8 + normal 0.8"),
        (cand("coastal_clean"), "depth 0.8 · clean")]),
    ("crystallites · CN-стек", [
        (rend("crystallites"), "рендер"),
        (cand("crystallites_d08"), "depth 0.8"),
        (cand("crystallites_n08"), "d0.8 + normal 0.8"),
        (cand("crystallites_clean"), "depth 0.8 · clean")]),
    ("humans · свет и denoise", [
        (rend("humans", "cine"), "рендер cine"),
        (rend("humans", "flat"), "рендер flat"),
        (cand("humans_d08"), "cine dn0.80"),
        (cand("humans_flat08"), "flat dn0.80"),
        (cand("humans_cine055"), "cine dn0.55"),
        (cand("humans_flat055"), "flat dn0.55")]),
    ("humans · промпты (depth 0.8)", [
        (rend("humans"), "рендер"),
        (cand("humans_d08"), "concept-art"),
        (cand("humans_clean"), "clean"),
        (cand("humans_weath"), "weathered/gritty")]),
    ("humans · проходность (prompt=concept)", [
        (cand("humans_d08"), "база+0.45 (без HiRes)"),
        (cand("humans_d08_u"), "0.45 + HiRes 2048/0.40"),
        (cand("humans_det13"), "detail 2048/0.35"),
        (cand("humans_det15"), "detail 2304/0.35")]),
    ("финалисты (depth0.8 · clean + HiRes)", [
        (cand("humans_final"), "humans cine 25°"),
        (cand("humans_final_flat"), "humans flat 25°"),
        (cand("humans_e40"), "humans cine 40°"),
        (cand("coastal_final"), "coastal"),
        (cand("crystallites_final"), "crystallites")]),
    ("сравнение со спайком 4 (тот же корабль)", [
        (os.path.join(OUT, "humans_d08p3_200.png"), "spike4 humans (clean)"),
        (cand("humans_final"), "spike5 humans")]),
]


def montage_v4(path):
    cell, pad, lbl, cap = 220, 8, 22, 18
    font = load_font(14)
    font_small = load_font(12)
    ncols = max(len(c) for _, c in MONTAGE_ROWS)
    W = pad + ncols * (cell + pad)
    H = pad + len(MONTAGE_ROWS) * (lbl + cell + cap + pad)
    canvas = Image.new('RGB', (W, H), (18, 18, 22))
    d = ImageDraw.Draw(canvas)
    for ri, (label, cells) in enumerate(MONTAGE_ROWS):
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


def main():
    meta_path = os.path.join(OUT, "geo_meta.json")
    with open(meta_path, encoding="utf-8") as f:
        meta = json.load(f)
    entries = meta.get("entries_v4", [])
    for e in entries:
        rp = rend(e["race"], e.get("render", "cine"), e["elev"])
        e["metrics"] = metrics(e["candidate"], rp)
        if e.get("final_raw") and os.path.exists(e["final_raw"]) and e["final_raw"] != e["candidate"]:
            e["metrics_raw"] = metrics(e["final_raw"], rp)
    meta["entries_v4"] = entries
    meta["reference_sprites"] = reference()
    meta["montage_v4"] = montage_v4(os.path.join(OUT, "preview_montage_v4.png"))
    with open(meta_path, "w", encoding="utf-8") as f:
        json.dump(meta, f, ensure_ascii=False, indent=1)
    ref = meta["reference_sprites"]
    print("REFERENCE 21 sprites: colors_median=%s edgesF_median=%s" % (
        ref["colors_median"], ref["edges_fg_median"]))
    print("%-22s %-10s %-6s %-7s %-7s %-7s %s" % (
        "id", "controls", "colors", "edgesF", "edgesC", "IoU", "time_s"))
    for e in sorted(entries, key=lambda x: x["id"]):
        m = e.get("metrics") or {}
        ctrl = "+".join("%s%.2f" % (c["kind"][0], c["strength"]) for c in e["controls"])
        print("%-22s %-10s %-6s %-7s %-7s %-7s %s" % (
            e["id"], ctrl, m.get("unique_colors"), m.get("edge_density_fg"),
            m.get("edge_density_canvas"), m.get("iou_vs_render"),
            e["timings"].get("total")))


if __name__ == "__main__":
    main()
