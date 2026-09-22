# -*- coding: utf-8 -*-
# СПАЙК 3: метрики прогона с гриблзами, сравнение со спайками 1–2 и эталоном
# (21 спрайт), монтаж preview_montage_v2.png, дописывание метрик в geo_meta.json.
# Считается ПО ФАЙЛАМ (GPU не нужен) — можно перезапускать сколько угодно.
import glob
import json
import os

import numpy as np
from PIL import Image, ImageDraw, ImageFont
from scipy import ndimage

ROOT = r"C:\Zorion2"
OUT = os.path.join(ROOT, "ai_drafts", "geometry_ships")
RENDER2 = os.path.join(OUT, "render2")
SPIKE1 = os.path.join(ROOT, "ai_drafts", "spike_ships")
SPRITES = os.path.join(ROOT, "web", "static", "sprites")
BG = np.array([5, 5, 5])


def load_font(size=14):
    for name in ("arial.ttf", "segoeui.ttf"):
        try:
            return ImageFont.truetype(name, size)
        except Exception:
            continue
    return ImageFont.load_default()


def fg_mask_render(path):
    arr = np.array(Image.open(path).convert('RGB')).astype(np.int16)
    return np.abs(arr - BG).sum(axis=2) > 30


def fg_mask_alpha(path):
    img = Image.open(path)
    arr = np.array(img.convert('RGBA'))
    alpha = arr[:, :, 3]
    if alpha.min() < 255:
        return alpha > 40
    rgb = arr[:, :, :3].astype(np.int16)
    h, w, _ = rgb.shape
    m = max(4, int(0.06 * min(h, w)))
    ring = np.concatenate([rgb[:m].reshape(-1, 3), rgb[-m:].reshape(-1, 3),
                           rgb[:, :m].reshape(-1, 3), rgb[:, -m:].reshape(-1, 3)])
    bg = np.median(ring, axis=0)
    mask = np.abs(rgb - bg).sum(axis=2) > 40
    labels, num = ndimage.label(mask)
    if num > 1:
        border = set(np.unique(np.concatenate([labels[0], labels[-1], labels[:, 0], labels[:, -1]])))
        border.discard(0)
        for lb in border:
            mask[labels == lb] = False
    return mask


def crop_mask(mask, size=256, mirror=False):
    ys, xs = np.nonzero(mask)
    if len(xs) == 0:
        return np.zeros((size, size), bool)
    m = mask[ys.min():ys.max() + 1, xs.min():xs.max() + 1]
    if mirror:
        m = m[:, ::-1]
    im = Image.fromarray((m * 255).astype(np.uint8), 'L').resize((size, size), Image.NEAREST)
    return np.array(im) > 127


def iou(m1, m2):
    inter = np.logical_and(m1, m2).sum()
    union = np.logical_or(m1, m2).sum()
    return float(inter) / float(union) if union else 0.0


def shape_iou(result_path, render_path):
    r = crop_mask(fg_mask_alpha(result_path))
    s = fg_mask_render(render_path)
    return round(max(iou(r, crop_mask(s)), iou(r, crop_mask(s, mirror=True))), 4)


def components(path):
    a = np.array(Image.open(path).convert('RGBA'))
    m = a[:, :, 3] > 40
    if m.sum() == 0:
        return 0
    _, n = ndimage.label(m)
    return int(n)


def metrics(png_path, render_path=None):
    im = Image.open(png_path).convert('RGBA')
    arr = np.array(im)
    alpha = arr[:, :, 3] > 40
    if alpha.sum() == 0:
        return {}
    rgb = arr[:, :, :3].astype(np.float64)
    gray = rgb.mean(axis=2)
    cols = np.unique(rgb[alpha].astype(np.uint8).reshape(-1, 3), axis=0)
    # градиент — как в tools/spike_ship_measure.py (не нормированный на 2, порог 25):
    # числа сравнимы со спекой §2 и спайками 1–2
    h, w = gray.shape
    gx = np.zeros_like(gray)
    gy = np.zeros_like(gray)
    gx[:, 1:-1] = gray[:, 2:] - gray[:, :-2]
    gy[1:-1, :] = gray[2:, :] - gray[:-2, :]
    mag = np.sqrt(gx * gx + gy * gy) * alpha
    out = {"unique_colors": int(cols.shape[0]),
           "edge_density_canvas": round(float((mag > 25).sum()) / float(h * w), 4),
           "edge_density_fg": round(float((mag[alpha] > 25).mean()), 4),
           "lit_range": round(float(np.percentile(gray[alpha], 95) - np.percentile(gray[alpha], 5)), 1),
           "components": components(png_path)}
    if render_path and os.path.exists(render_path):
        out["iou_vs_render"] = shape_iou(png_path, render_path)
    return out


def median_of(vals):
    vals = [v for v in vals if v is not None]
    return round(float(np.median(vals)), 4) if vals else None


def reference():
    """Эталон — 21 принятый спрайт игрока (200×200)."""
    rows = []
    for p in sorted(glob.glob(os.path.join(SPRITES, "*.png"))):
        rows.append(metrics(p))
    return {
        "n": len(rows),
        "colors_median": median_of([r["unique_colors"] for r in rows]),
        "edges_canvas_median": median_of([r["edge_density_canvas"] for r in rows]),
        "edges_fg_median": median_of([r["edge_density_fg"] for r in rows]),
    }


def montage(entries, path):
    byid = {e["id"]: e for e in entries}
    cell, pad, lbl, cap = 200, 8, 22, 18
    font = load_font(14)
    font_small = load_font(12)
    cols = ["рендер+гриблзы", "V1 (CN 0.55)", "V2 (img2img 0.45)", "V5 (img2img 0.40)"]
    rows = []
    for slug in ("humans", "coastal", "crystallites"):
        cells = [(os.path.join(RENDER2, "%s_tilt.png" % slug), cols[0])]
        for v, capv in (("V1g", cols[1]), ("V2g", cols[2]), ("V5", cols[3])):
            e = byid.get("%s_%s" % (slug, v))
            cells.append((e["candidate"] if e else None, capv))
        rows.append((slug, cells))
    e = byid.get("humans_V1g_jugg")
    if e:
        rows.append(("humans · Juggernaut XL v9", [(os.path.join(RENDER2, "humans_tilt.png"), "рендер+гриблзы"),
                                                   (e["candidate"], "V1g · Juggernaut")]))
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
                canvas.paste(Image.open(p).convert('RGB').resize((cell, cell), Image.LANCZOS), (x, y + lbl))
            d.text((x, y + lbl + cell + 2), capv, fill=(180, 200, 220), font=font_small)
    canvas.save(path)
    return path


def spike_metrics():
    comp = {}
    for slug in ("humans", "coastal", "crystallites"):
        p = os.path.join(SPIKE1, "%s_200.png" % slug)
        if os.path.exists(p):
            comp.setdefault("spike1", {})[slug] = metrics(p)
    for slug in ("humans", "coastal", "crystallites"):
        for v in ("V1", "V2", "V3"):
            p = os.path.join(OUT, "%s_%s_200.png" % (slug, v))
            if os.path.exists(p):
                comp.setdefault("spike2", {})["%s_%s" % (slug, v)] = metrics(
                    p, os.path.join(OUT, "%s_tilt.png" % slug))
    return comp


def main():
    meta_path = os.path.join(OUT, "geo_meta.json")
    with open(meta_path, encoding="utf-8") as f:
        meta = json.load(f)
    entries = meta.get("entries_v2", [])
    for e in entries:
        rp = os.path.join(RENDER2, "%s_tilt.png" % e["race"])
        e["metrics"] = metrics(e["candidate"], rp)
        e["metrics_raw3"] = metrics(e["raw3"], rp)
    meta["entries_v2"] = entries
    meta["reference_sprites"] = reference()
    meta["comparison_spikes"] = spike_metrics()
    meta["montage_v2"] = montage(entries, os.path.join(OUT, "preview_montage_v2.png"))
    with open(meta_path, "w", encoding="utf-8") as f:
        json.dump(meta, f, ensure_ascii=False, indent=1)
    ref = meta["reference_sprites"]
    print("REFERENCE 21 sprites: colors_median=%s edges(canvas)_median=%s edges(fg)_median=%s" % (
        ref["colors_median"], ref["edges_canvas_median"], ref["edges_fg_median"]))
    print("Greebles:", meta.get("greeble_counts"))
    for e in sorted(entries, key=lambda x: x["id"]):
        m, r = e["metrics"], e["metrics_raw3"]
        print("%-22s %-6s | colors=%-5s edgesC=%.3f edgesF=%.3f IoU=%-6s comp=%s | colors=%-5s edgesC=%.3f IoU=%s" % (
            e["id"], e["model"].split('-')[0],
            m.get("unique_colors"), m.get("edge_density_canvas", 0), m.get("edge_density_fg", 0),
            m.get("iou_vs_render"), m.get("components"),
            r.get("unique_colors"), r.get("edge_density_canvas", 0), r.get("iou_vs_render")))
    print("spike1:", {k: (v.get("unique_colors"), v.get("edge_density_canvas"), v.get("edge_density_fg"))
                      for k, v in meta["comparison_spikes"].get("spike1", {}).items()})
    print("spike2:", {k: (v.get("unique_colors"), v.get("edge_density_canvas"), v.get("edge_density_fg")) 
                      for k, v in meta["comparison_spikes"].get("spike2", {}).items()})


if __name__ == "__main__":
    main()
