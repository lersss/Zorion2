# -*- coding: utf-8 -*-
# СПАЙК 8-БИС: отчёт по кандидатам R2 (космические субъекты) — контрольный лист,
# метрики (цвета, края, вырез, ориентация) и итоговый монтаж
# ai_drafts/sprite_ships/preview_montage.png (ряды = расы: концепт | спрайт | эталон)
# + строка «что было» (blender_ships). Пишет metrics.json и дополняет meta.json.
# Консоль — ASCII (Windows cp866 ломает кириллицу, PITFALLS).
import argparse
import colorsys
import json
import math
import os
import sys

import numpy as np
from PIL import Image, ImageDraw, ImageFont

ROOT = r"C:\Zorion2"
OUT = os.path.join(ROOT, "ai_drafts", "sprite_ships")
SPRITES = os.path.join(ROOT, "web", "static", "sprites")
BLENDER = os.path.join(ROOT, "ai_drafts", "blender_ships")
RACES = ("humans", "coastal", "crystallites")

# Итоговый рецепт для 60 рас (спайк 8-бис): фиксированное ядро + подстановки от расы.
RECIPE_60 = {
    "model": "juggernaut-xl-v9.safetensors",
    "mode": "txt2img 1024x1024, steps 32, cfg 6.0, dpmpp_2m/karras, denoise 1.0",
    "fixed": {
        "anchor": "dorsal three-quarter view of a single flying starship, nose pointing right",
        "bg": "centered, no stars, plain black background",
        "style": ("hard-surface sci-fi game asset, crisp readable silhouette, high contrast edges, "
                  "dense greeble detail, octane render, artstation, highly detailed, masterpiece"),
        "subject_pool": ["deep-space starship", "stellar cruiser", "interstellar vessel",
                         "deep-space carrier", "stellar dreadnought"],
        "neg_form": ("space station, ring, torus, circular disc, front view, symmetrical, planet, "
                     "landscape, second ship, toy, plastic, cartoon, flat, blurry"),
        "neg_sea_air": ("boat, ship hull, sailing ship, sail, mast, anchor, water, sea, ocean, "
                        "harbor, hull with keel, airplane, aircraft, jet, fighter jet, wings of "
                        "aircraft, propeller, runway, airport, atmosphere, sky, clouds, ground"),
        "cut": "hyst tol_close=12 tol_wide=40 + fill_holes; normalize 200x200 fill=1.0 (100%), nose right (taper)",
        "hires": "4x-UltraSharp -> 1536 -> img2img denoise 0.40 -> cut",
    },
    "per_race": {
        "material_texture": "поле texture из docs/gamedesign/races/ships/<slug>.md — задаёт материал/палитру/детали",
        "blocked": "поле blocked из того же файла, минус термы, гасящие звёздную форму (engine/rocket/thruster у рас без машин)",
        "rule": "расовость — через материал/палитру, НЕ через морское/авиационное существительное",
        "elongation": "органическим/биотическим расам добавлять 'long streamlined ... hull' (без elongation коралл/ткань даёт «капсулу/яйцо»)",
    },
    "prompt_template": "{subject}, {race.texture}, {anchor}, {bg}, {style}",
    "negative_template": "{neg_form}, {neg_sea_air}, {race.blocked}",
    "tested": {"races": ["humans", "coastal", "crystallites"], "txt2img": 20, "finalists": 9},
}


def _font(size=15):
    for name in ("arial.ttf", "DejaVuSans.ttf"):
        try:
            return ImageFont.truetype(name, size)
        except Exception:
            continue
    return ImageFont.load_default()


def load_meta():
    with open(os.path.join(OUT, "meta.json"), encoding="utf-8") as f:
        return json.load(f)


def save_meta(meta):
    with open(os.path.join(OUT, "meta.json"), "w", encoding="utf-8") as f:
        json.dump(meta, f, ensure_ascii=False, indent=1)


def r2_entries(meta, group="R2"):
    return [e for e in meta["entries"] if e["group"] == group]


def sprite_path(e):
    """Финальный спрайт: Hi-Res, если есть, иначе базовый вырез 200x200."""
    for suf in ("_hr_200.png", "_200.png"):
        p = os.path.join(OUT, e["id"] + suf)
        if os.path.exists(p):
            return p, suf
    return None, None


def mean_hue_sat(path, step=97):
    im = np.array(Image.open(path).convert("RGBA"))
    a = im[:, :, 3] > 40
    rgb = im[:, :, :3][a]
    if len(rgb) == 0:
        return 0.0, 0.0, 0.0
    rgb = rgb[::step] / 255.0
    ang = np.array([colorsys.rgb_to_hsv(*p)[0] for p in rgb]) * 2 * math.pi
    sat = np.array([colorsys.rgb_to_hsv(*p)[1] for p in rgb])
    val = np.array([colorsys.rgb_to_hsv(*p)[2] for p in rgb])
    w = sat + 1e-6
    x = float((np.cos(ang) * w).sum())
    y = float((np.sin(ang) * w).sum())
    hue = (math.atan2(y, x) % (2 * math.pi)) / (2 * math.pi)
    return hue, float(sat.mean()), float(val.mean())


def build_ref_index():
    idx = []
    for name in sorted(os.listdir(SPRITES)):
        if not name.lower().endswith(".png"):
            continue
        p = os.path.join(SPRITES, name)
        idx.append((name, p) + mean_hue_sat(p))
    return idx


def nearest_ref(idx, hue, sat, val):
    """Ближайший эталон по колористике: серый (sat<0.15) — самый серый, иначе по тону."""
    if sat < 0.15:
        best = min(idx, key=lambda r: r[3])
    else:
        def d(r):
            dh = abs(r[2] - hue)
            dh = min(dh, 1.0 - dh)
            return dh * 1.0 + abs(r[3] - sat) * 0.15
        best = min(idx, key=d)
    return best[0], best[1]


def metrics(path):
    im = np.array(Image.open(path).convert("RGBA"))
    a = im[:, :, 3] > 40
    rgb = im[:, :, :3][a]
    uniq = np.unique(rgb.reshape(-1, 3), axis=0)
    q = (rgb // 16 * 16)
    cols, counts = np.unique(q, axis=0, return_counts=True)
    order = np.argsort(-counts)[:3]
    top = ["#%02x%02x%02x" % tuple(int(c) for c in cols[i]) for i in order]
    gray = np.array(Image.open(path).convert("L"), float)
    gx, gy = np.gradient(gray)
    mag = np.hypot(gx, gy)
    hue, sat, val = mean_hue_sat(path)
    return {"n_colors": int(len(uniq)), "top3": top,
            "mean_hue": round(hue, 3), "mean_sat": round(sat, 3), "mean_val": round(val, 3),
            "edge_sharp": round(float(mag[a].mean()), 2),
            "solid_share": round(float(a.mean()), 4)}


def _fit(path, side, bg=(18, 18, 22, 255)):
    im = Image.open(path).convert("RGBA")
    im.thumbnail((side, side), Image.LANCZOS)
    canvas = Image.new("RGBA", (side, side), bg)
    canvas.alpha_composite(im, ((side - im.width) // 2, (side - im.height) // 2))
    return canvas


def contact_sheet(entries, out_path, cell=260, cols=5):
    hdr = 34
    lab = 62
    races = [r for r in RACES if any(e["race"] == r for e in entries)]
    W = cols * cell + lab
    H = len(races) * (cell + lab) + hdr
    sheet = Image.new("RGB", (W, H), (24, 24, 28))
    d = ImageDraw.Draw(sheet)
    f = _font(15)
    d.text((8, 10), "SPIKE 8-BIS candidates: 3 races x 5 space-subject prompts",
           fill=(235, 235, 240), font=f)
    for r, race in enumerate(races):
        y0 = hdr + r * (cell + lab)
        d.text((8, y0 + cell // 2), race, fill=(255, 210, 90), font=f)
        row = sorted([x for x in entries if x["race"] == race], key=lambda x: x["seed"])
        for c in range(cols):
            cellimg = Image.new("RGB", (cell, cell + lab), (34, 34, 40))
            if c < len(row):
                e = row[c]
                raw = e.get("raw")
                if raw and os.path.exists(raw):
                    ci = _fit(raw, cell)
                    cellimg.paste(ci, (0, 0), ci)
                d.text((6, cell + 4), "%s  seed %s" % (e.get("subject", "")[:22], e["seed"]),
                       fill=(220, 220, 225), font=f)
                d.text((6, cell + 22), e["id"], fill=(150, 150, 160), font=f)
            sheet.paste(cellimg, (lab + c * cell, y0))
    sheet.save(out_path, "PNG")
    return out_path


def montage(entries, finalists, ref_idx, out_path):
    """Ряды = расы (концепт 1024 | спрайт 200 | эталон); снизу — строка «что было»."""
    cs, ss, rs = 300, 200, 200
    gap = 14
    lab = 250
    col_w = cs + ss + rs + 2 * gap
    row_h = max(cs, ss, rs) + 26
    now_rows = len(finalists)
    was_rows = len(RACES)
    hdr = 40
    title_h = 30
    W = lab + col_w + 20
    H = title_h + hdr + now_rows * row_h + 40 + title_h + was_rows * row_h + 20
    im = Image.new("RGB", (W, H), (24, 24, 28))
    d = ImageDraw.Draw(im)
    f = _font(16)
    fs = _font(13)
    d.text((10, 8), "SPIKE 8-BIS: concept -> sprite 200x200 -> reference  (NOW)",
           fill=(235, 235, 240), font=f)
    y = title_h
    for x, t in ((lab, "concept 1024"), (lab + cs + gap, "sprite 200"), (lab + cs + gap + ss + gap, "reference")):
        d.text((x + 4, y + 10), t, fill=(255, 210, 90), font=fs)
    y += hdr
    by_id = {e["id"]: e for e in entries}
    for fid in finalists:
        if fid not in by_id:
            continue
        e = by_id[fid]
        sp, _ = sprite_path(e)
        sp = sp or os.path.join(OUT, fid + "_200.png")
        hue, sat, val = mean_hue_sat(sp)
        ref_name, ref_path = nearest_ref(ref_idx, hue, sat, val)
        e["reference"] = ref_name
        e["metrics"] = metrics(sp)
        o = e.get("cut", {}).get("orient", {})
        e["orient_flag"] = ("ambiguous" if o.get("ambiguous")
                            else ("mirrored" if o.get("mirrored") else "nose-right"))
        d.text((8, y + row_h // 2 - 20), e["race"], fill=(255, 210, 90), font=f)
        d.text((8, y + row_h // 2), "%s  s%s" % (e.get("subject", ""), e["seed"]),
               fill=(220, 220, 225), font=fs)
        if e.get("run_verdict"):
            d.text((8, y + row_h // 2 + 18), e["run_verdict"], fill=(130, 220, 130), font=fs)
        raw = e.get("raw")
        if raw and os.path.exists(raw):
            ci = _fit(raw, cs, (34, 34, 40, 255))
            im.paste(ci, (lab, y), ci)
        si = _fit(sp, ss, (34, 34, 40, 255))
        im.paste(si, (lab + cs + gap, y), si)
        ri = _fit(ref_path, rs, (34, 34, 40, 255))
        im.paste(ri, (lab + cs + gap + ss + gap, y), ri)
        d.text((lab + cs + gap + ss + gap, y + rs + 2), ref_name, fill=(150, 150, 160), font=fs)
        y += row_h
    # --- «что было» ---
    d.text((10, y + 8), "WAS (blender_ships): flat procedural hulls", fill=(235, 235, 240), font=f)
    y += title_h
    for x, t in ((lab, "base"), (lab + cs + gap, "cn06"), (lab + cs + gap + ss + gap, "cn10")):
        d.text((x + 4, y + 2), t, fill=(255, 210, 90), font=fs)
    y += 18
    for race in RACES:
        d.text((8, y + row_h // 2), race, fill=(255, 210, 90), font=f)
        for k, tag in enumerate(("base", "cn06", "cn10")):
            p = os.path.join(BLENDER, "bl_%s_%s_200.png" % (race, tag))
            if os.path.exists(p):
                side = cs if k == 0 else ss
                ci = _fit(p, side, (34, 34, 40, 255))
                x = lab if k == 0 else (lab + cs + gap if k == 1 else lab + cs + gap + ss + gap)
                im.paste(ci, (x, y), ci)
                d.text((x + 2, y + ci.height + 2), "bl_%s_%s" % (race, tag), fill=(150, 150, 160), font=fs)
        y += row_h
    im.save(out_path, "PNG")
    return out_path


def main():
    ap = argparse.ArgumentParser(description="Спайк 8-бис: отчёт/монтаж R2")
    ap.add_argument("--mode", default="montage", choices=["contact", "montage"])
    ap.add_argument("--group", default="R2", help="группа записей meta.json (contact)")
    ap.add_argument("--out", default="", help="имя файла отчёта (по умолчанию по группе)")
    ap.add_argument("--finalists", default="", help="id через запятую (только montage)")
    ap.add_argument("--verdicts", default="", help="id=вердикт;id=вердикт (в meta)")
    args = ap.parse_args()
    meta = load_meta()
    entries = r2_entries(meta, args.group)
    if args.verdicts:
        vd = dict(p.split("=", 1) for p in args.verdicts.split(";") if "=" in p)
        for e in meta["entries"]:
            if e["id"] in vd:
                e["run_verdict"] = vd[e["id"]]
        save_meta(meta)
        print("verdicts set: %d" % len(vd))
        if args.mode == "contact":
            return
    if args.mode == "contact":
        p = contact_sheet(entries, os.path.join(OUT, args.out or ("contact_%s.png" % args.group.lower())))
        print("contact: %s (%d)" % (p, len(entries)))
        return
    # монтаж собирает финалистов из обеих групп (R2 — humans/crystallites, R3 — coastal v2)
    entries = [e for e in meta["entries"] if e["group"] in ("R2", "R3")]
    finalists = [x for x in args.finalists.split(",") if x]
    if not finalists:
        # по умолчанию — все R2, у кого есть вырез
        finalists = [e["id"] for e in entries if sprite_path(e)[0]]
    ref_idx = build_ref_index()
    p = montage(entries, finalists, ref_idx, os.path.join(OUT, "preview_montage.png"))
    meta["bg"] = "centered, no stars, plain black background"
    meta["neg_hard"] = ("boat, ship hull, sailing ship, sail, mast, anchor, water, sea, ocean, "
                        "harbor, hull with keel, airplane, aircraft, jet, fighter jet, wings of "
                        "aircraft, propeller, runway, airport, atmosphere, sky, clouds, ground")
    meta["recipe_60"] = RECIPE_60
    save_meta(meta)
    rows = []
    for e in meta["entries"]:
        if e["id"] in finalists and "metrics" in e:
            m = e["metrics"]
            rows.append({"id": e["id"], "race": e["race"], "subject": e.get("subject"),
                         "seed": e["seed"], "verdict": e.get("run_verdict", ""),
                         "n_colors": m["n_colors"], "top3": m["top3"],
                         "bg_left": e.get("cut", {}).get("stats", {}).get("bg_left"),
                         "hull_eaten": e.get("cut", {}).get("stats", {}).get("hull_eaten"),
                         "edge_sharp": m["edge_sharp"],
                         "orient_ambiguous": e.get("cut", {}).get("orient", {}).get("ambiguous"),
                         "orient_flag": e.get("orient_flag"),
                         "mirrored": e.get("cut", {}).get("orient", {}).get("mirrored"),
                         "reference": e.get("reference")})
    with open(os.path.join(OUT, "metrics.json"), "w", encoding="utf-8") as f:
        json.dump(rows, f, ensure_ascii=False, indent=1)
    print("montage: %s" % p)
    print(json.dumps(rows, ensure_ascii=True, indent=1))


if __name__ == "__main__":
    main()
