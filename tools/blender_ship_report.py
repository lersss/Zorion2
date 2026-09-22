# -*- coding: utf-8 -*-
# СПАЙК 6, шаг F: монтаж-сравнение и метрики.
# Ряды = расы; колонки = [blender 3/4 | ИИ 200x200 | прежний результат v4 200x200].
# Метрики на кандидата: уникальные цвета, плотность краёв, IoU силуэта с рендером.
# Пишет ai_drafts/blender_ships/preview_montage.png и report.json (+ meta.json.report).
import argparse
import json
import os
import subprocess
import sys

import numpy as np
from PIL import Image

ROOT = r"C:\Zorion2"
PY = r"C:\ComfyUI\venv\Scripts\python.exe"
OUT = os.path.join(ROOT, "ai_drafts", "blender_ships")
RAW = os.path.join(OUT, "raw")
V4 = os.path.join(ROOT, "ai_drafts", "geometry_ships")
TILE = 512
RACES = ["humans", "coastal", "crystallites"]


def alpha_mask(path, size=200):
    img = Image.open(path).convert("RGBA")
    if img.size != (size, size):
        img = img.resize((size, size), Image.LANCZOS)
    return np.array(img)[:, :, 3] > 40


def render_silhouette(slug):
    """Силуэт blender-рендера, нормализованный тем же процессором (200x200)."""
    src = os.path.join(RAW, slug, "color.png")
    dst = os.path.join(OUT, "_sil_%s_200.png" % slug)
    if not os.path.exists(dst):
        subprocess.run([PY, os.path.join(ROOT, "tools", "process_ship.py"), src, dst], check=True)
    return dst


def metrics(path):
    """Уникальные цвета (5 бит/канал) и плотность краёв внутри силуэта.
    Все картинки приводятся к одной длинной стороне 200 px — иначе плотность
    краёв несравнима между 1536-рендером и 200-кандидатом."""
    img = Image.open(path).convert("RGBA")
    img.thumbnail((200, 200), Image.LANCZOS)
    arr = np.array(img)
    mask = arr[:, :, 3] > 40
    if mask.sum() == 0:
        return {"unique_colors": 0, "edge_density": 0.0, "coverage": 0.0}
    rgb = arr[:, :, :3]
    q = (rgb[mask] >> 3).astype(np.int32)
    uniq = len(np.unique(q[:, 0] * 1024 + q[:, 1] * 32 + q[:, 2]))
    gray = rgb.mean(axis=2)
    gx = np.zeros_like(gray)
    gy = np.zeros_like(gray)
    gx[:, 1:-1] = gray[:, 2:] - gray[:, :-2]
    gy[1:-1, :] = gray[2:, :] - gray[:-2, :]
    edge = np.hypot(gx, gy) > 40
    cover = mask.mean()
    return {"unique_colors": int(uniq),
            "edge_density": round(float(edge[mask].mean()), 4),
            "coverage": round(float(cover), 4)}


def iou(a, b):
    return round(float((a & b).sum()) / max(1, (a | b).sum()), 4)


def iou_best_flip(a, b):
    """IoU с точностью до зеркала: process_ship нормализует ориентацию каждой
    картинки независимо ('нос вправо'), и для асимметричных силуэтов выбор
    стороны может разойтись. Ориентация — конвенция, не форма, поэтому берём
    лучшую из двух; прямое значение тоже возвращаем для протокола."""
    direct = iou(a, b)
    flipped = iou(np.fliplr(a), b)
    return max(direct, flipped), direct, flipped


def main():
    ap = argparse.ArgumentParser(description="Спайк 6: монтаж-сравнение + метрики")
    ap.add_argument("--best", default="humans=humans_base,coastal=coastal_base,"
                                      "crystallites=crystallites_base",
                    help="slug=job_id, выбранный вариант ИИ на расу")
    args = ap.parse_args()
    best = dict(p.split("=") for p in args.best.split(",") if p.strip())

    rows = []
    report = {}
    for slug in RACES:
        blender = os.path.join(RAW, slug, "color.png")
        jid = best.get(slug, "%s_base" % slug)
        ai = os.path.join(OUT, "bl_%s_200.png" % jid)
        v4 = os.path.join(V4, "v4_%s_final_200.png" % slug)
        sil = render_silhouette(slug)
        sil_m = alpha_mask(sil)
        ai_best, ai_direct, ai_flip = iou_best_flip(alpha_mask(ai), sil_m)
        v4_iou = None
        if os.path.exists(v4):
            v4_iou, v4_direct, v4_flip = iou_best_flip(alpha_mask(v4), sil_m)
        m_bl = metrics(blender)
        m_ai = metrics(ai)
        m_v4 = metrics(v4) if os.path.exists(v4) else None
        rec = {
            "race": slug, "ai_job": jid,
            "blender": blender, "ai": ai, "v4": v4 if os.path.exists(v4) else None,
            "silhouette_200": sil,
            "metrics": {"blender": m_bl, "ai": m_ai, "v4": m_v4},
            # IoU силуэта с blender-рендером (с точностью до зеркала — см. iou_best_flip).
            "iou": {"ai_vs_render": ai_best, "ai_direct": ai_direct, "ai_flipped": ai_flip,
                    "v4_vs_render": v4_iou},
        }
        report[slug] = rec
        rows.append(rec)

    sheet = Image.new("RGB", (3 * TILE, len(rows) * TILE), (16, 16, 20))
    for r, rec in enumerate(rows):
        cells = [(rec["blender"], "blender: %s" % rec["race"]),
                 (rec["ai"], "AI: %s" % rec["ai_job"]),
                 (rec["v4"], "v4: %s_final" % rec["race"])]
        for c, (path, label) in enumerate(cells):
            if not path or not os.path.exists(path):
                continue
            img = Image.open(path).convert("RGBA")
            bg = Image.new("RGBA", img.size, (10, 10, 14, 255))
            img = Image.alpha_composite(bg, img).convert("RGB")
            img.thumbnail((TILE - 10, TILE - 10), Image.LANCZOS)
            sheet.paste(img, (c * TILE + (TILE - img.width) // 2,
                              r * TILE + (TILE - img.height) // 2))
    sheet.save(os.path.join(OUT, "preview_montage.png"))

    report_path = os.path.join(OUT, "report.json")
    with open(report_path, "w", encoding="utf-8") as f:
        json.dump({"columns": ["blender", "ai", "v4_final"], "races": report},
                  f, ensure_ascii=False, indent=1)
    meta_path = os.path.join(OUT, "meta.json")
    meta = {}
    if os.path.exists(meta_path):
        with open(meta_path, encoding="utf-8") as f:
            meta = json.load(f)
    meta["report"] = report
    with open(meta_path, "w", encoding="utf-8") as f:
        json.dump(meta, f, ensure_ascii=False, indent=1)
    print("preview_montage: %s" % os.path.join(OUT, "preview_montage.png"))
    print("report: %s" % report_path)
    for slug, rec in report.items():
        print("%-14s AI colors=%-5d edges=%.3f | blender colors=%-5d edges=%.3f | "
              "v4 colors=%s edges=%s | IoU ai=%.3f v4=%s"
              % (slug, rec["metrics"]["ai"]["unique_colors"], rec["metrics"]["ai"]["edge_density"],
                 rec["metrics"]["blender"]["unique_colors"], rec["metrics"]["blender"]["edge_density"],
                 rec["metrics"]["v4"]["unique_colors"] if rec["metrics"]["v4"] else "-",
                 rec["metrics"]["v4"]["edge_density"] if rec["metrics"]["v4"] else "-",
                 rec["iou"]["ai_vs_render"],
                 rec["iou"]["v4_vs_render"] if rec["iou"]["v4_vs_render"] is not None else "-"))


if __name__ == "__main__":
    main()
