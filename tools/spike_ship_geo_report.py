# -*- coding: utf-8 -*-
# СПАЙК 2 (§3): метрики результатов объёмного прогона, сравнение со спайком 1 и
# базовым пулом, монтаж preview_montage.png, запись метрик в geo_meta.json.
# Считается ПО ФАЙЛАМ (GPU не нужен) — можно перезапускать сколько угодно.
import json
import os

import numpy as np
from PIL import Image, ImageDraw, ImageFont
from scipy import ndimage

ROOT = r"C:\Zorion2"
OUT = os.path.join(ROOT, "ai_drafts", "geometry_ships")
SPIKE1 = os.path.join(ROOT, "ai_drafts", "spike_ships")
POOL = os.path.join(ROOT, "ai_drafts", "ships_pool")
BG = np.array([5, 5, 5])


def load_font(size=14):
    for name in ("arial.ttf", "segoeui.ttf"):
        try:
            return ImageFont.truetype(name, size)
        except Exception:
            continue
    return ImageFont.load_default()


def fg_mask_render(path):
    """Силуэт рендера: всё, что не близко к фону (5,5,5)."""
    arr = np.array(Image.open(path).convert('RGB')).astype(np.int16)
    return np.abs(arr - BG).sum(axis=2) > 30


def fg_mask_alpha(path):
    """Маска результата: alpha, если она есть; иначе — вырез фона по цвету углов
    (как process_ship BG_TOL=40) — raw-файлы ComfyUI идут без alpha."""
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
    labels, num = ndimage.label(mask)      # фон-пятно касается рамки -> выкинуть
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
    """IoU формы «результат vs рендер» (кроп bbox, 256², max по зеркалу)."""
    r = crop_mask(fg_mask_alpha(result_path))
    s = fg_mask_render(render_path)
    s1 = crop_mask(s)
    s2 = crop_mask(s, mirror=True)
    return round(max(iou(r, s1), iou(r, s2)), 4)


def metrics(png_path, render_path=None):
    im = Image.open(png_path).convert('RGBA')
    arr = np.array(im)
    alpha = arr[:, :, 3] > 40
    rgb = arr[:, :, :3].astype(np.float64)
    gray = rgb.mean(axis=2)
    opaque = int(alpha.sum())
    if opaque == 0:
        return {}
    cols = np.unique(rgb[alpha].astype(np.uint8).reshape(-1, 3), axis=0)
    gy, gx = np.gradient(gray)
    mag = np.hypot(gx, gy)
    mag = mag * alpha
    edges = float((mag[alpha] > 25).mean())
    dark = float((gray[alpha] < 60).mean())
    bright = float((gray[alpha] > 180).mean())
    out = {"unique_colors": int(cols.shape[0]), "edge_density": round(edges, 4),
           "dark_share": round(dark, 4), "bright_share": round(bright, 4),
           "lit_range": round(float(np.percentile(gray[alpha], 95) - np.percentile(gray[alpha], 5)), 1)}
    if render_path and os.path.exists(render_path):
        out["iou_vs_render"] = shape_iou(png_path, render_path)
    return out


def montage(entries, path):
    cell = 200
    pad = 8
    lbl = 22
    font = load_font(14)
    font_small = load_font(12)
    byid = {e["id"]: e for e in entries}
    rows = []
    for slug in ("humans", "coastal", "crystallites"):
        cells = [(os.path.join(OUT, "%s_tilt.png" % slug), "объёмный рендер (наклон)")]
        for v in ("V1", "V2", "V3"):
            e = byid.get("%s_%s" % (slug, v))
            if e:
                cells.append((e["candidate"], "%s %s" % (v, "базлайн" if v == "V3" else "")))
        rows.append((slug, cells))
    extras = [("humans_V1_camtop", "humans V1, камера 0°"),
              ("humans_V1_jugg", "humans V1, Juggernaut XL v9"),
              ("humans_V1_lora05", "humans V1, LoRA 0.5"),
              ("humans_V1_lora07", "humans V1, LoRA 0.7"),
              ("humans_V4", "humans V4, denoise 0.72 (зонд)"),
              ("coastal_V4", "coastal V4, denoise 0.72 (зонд)")]
    for jid, label in extras:
        e = byid.get(jid)
        if not e:
            continue
        cam = os.path.join(OUT, "%s_%s.png" % (e["race"], e["camera"]))
        rows.append((label, [(cam, "рендер"), (e["candidate"], e["variant"])]))
    ncols = max(len(c) for _, c in rows)
    gap = 12
    cap_h = 18
    row_h = lbl + cell + cap_h + gap
    W = pad + ncols * (cell + pad)
    H = pad + len(rows) * row_h
    canvas = Image.new('RGB', (W, H), (18, 18, 22))
    d = ImageDraw.Draw(canvas)
    for ri, (label, cells) in enumerate(rows):
        y = pad + ri * row_h
        d.text((pad, y), label, fill=(235, 235, 235), font=font)
        for ci, (p, cap) in enumerate(cells):
            x = pad + ci * (cell + pad)
            if os.path.exists(p):
                th = Image.open(p).convert('RGB').resize((cell, cell), Image.LANCZOS)
                canvas.paste(th, (x, y + lbl))
            d.text((x, y + lbl + cell + 2), cap, fill=(180, 200, 220), font=font_small)
    canvas.save(path)
    return path


def comparison():
    comp = {"spike1": [], "ships_pool": []}
    for slug in ("humans", "coastal", "crystallites"):
        p = os.path.join(SPIKE1, "%s_200.png" % slug)
        if os.path.exists(p):
            comp["spike1"].append({"race": slug, **metrics(p, os.path.join(SPIKE1, "silhouettes", slug + ".png"))})
    for i in range(1, 13):
        p = os.path.join(POOL, "s%02d.png" % i)
        if os.path.exists(p):
            comp["ships_pool"].append({"name": "s%02d" % i, **metrics(p)})
    return comp


def main():
    meta_path = os.path.join(OUT, "geo_meta.json")
    with open(meta_path, encoding="utf-8") as f:
        meta = json.load(f)
    entries = meta.get("entries", [])
    for e in entries:
        m = metrics(e["candidate"], e.get("source"))
        e["metrics"] = m
        e["metrics_raw3"] = metrics(e["raw3"], e.get("source"))
    meta["entries"] = entries
    meta["comparison"] = comparison()
    mpath = montage(entries, os.path.join(OUT, "preview_montage.png"))
    meta["montage"] = mpath
    with open(meta_path, "w", encoding="utf-8") as f:
        json.dump(meta, f, ensure_ascii=False, indent=1)
    print("montage: %s" % mpath)
    for e in entries:
        m, r = e["metrics"], e["metrics_raw3"]
        print("%-22s %-6s colors=%-5s edges=%.3f IoU200=%-6s | raw3: colors=%-5s edges=%.3f IoU=%s" % (
            e["id"], e["model"].split('-')[0],
            m.get("unique_colors"), m.get("edge_density", 0), m.get("iou_vs_render"),
            r.get("unique_colors"), r.get("edge_density", 0), r.get("iou_vs_render")))


if __name__ == "__main__":
    main()
