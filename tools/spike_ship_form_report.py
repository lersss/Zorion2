# -*- coding: utf-8 -*-
# СПАЙК 7, шаг отчёта: монтаж-сравнение + метрики.
# Ряды = модель/промпт (ветка A) и раса/сила CN (ветка B); внизу — «что было»
# (bl_*_200 спайка 6 и v4_*_final_200 спайка 5). Метрики — цвета/края/покрытие
# (blender_ship_report.metrics) + полные метрики сырья (spike_ship_measure.metrics),
# для ветки B ещё IoU силуэта с blender-рендером.
# Пишет ai_drafts/ai_form/{preview_montage,branch_a,branch_b}.png и report.json.
import json
import os
import subprocess
import sys

from PIL import Image, ImageDraw, ImageFont

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from blender_ship_report import alpha_mask, iou_best_flip, metrics, render_silhouette  # noqa: E402
from spike_ship_measure import metrics as raw_metrics  # noqa: E402

ROOT = r"C:\Zorion2"
PY = r"C:\ComfyUI\venv\Scripts\python.exe"
OUT = os.path.join(ROOT, "ai_drafts", "ai_form")
BL = os.path.join(ROOT, "ai_drafts", "blender_ships")
GEO = os.path.join(ROOT, "ai_drafts", "geometry_ships")
TILE = 420
COLS = 3
BAR = 26
LABEL = 20
RACES = ["humans", "coastal", "crystallites"]
BL_BEST = {"humans": "humans_cn10", "coastal": "coastal_base", "crystallites": "crystallites_base"}


def font(size):
    for name in ("arial.ttf", "segoeui.ttf", "calibri.ttf"):
        p = os.path.join(r"C:\Windows\Fonts", name)
        if os.path.exists(p):
            return ImageFont.truetype(p, size)
    return ImageFont.load_default()


def load_rgb(path, box):
    img = Image.open(path).convert("RGBA")
    bg = Image.new("RGBA", img.size, (10, 10, 14, 255))
    img = Image.alpha_composite(bg, img).convert("RGB")
    img.thumbnail((box, box), Image.LANCZOS)
    return img


def compose(rows, out_path, title):
    width = COLS * TILE
    height = 40 + sum(BAR + TILE for _ in rows)
    sheet = Image.new("RGB", (width, height), (16, 16, 20))
    d = ImageDraw.Draw(sheet)
    d.text((8, 10), title, fill=(235, 235, 235), font=font(22))
    y = 40
    f_bar, f_lab = font(17), font(14)
    for r in rows:
        d.rectangle([0, y, width, y + BAR - 1], fill=(30, 34, 46))
        d.text((8, y + 4), r["title"], fill=(200, 220, 255), font=f_bar)
        y += BAR
        for c, (path, label) in enumerate(r["cells"]):
            if not path or not os.path.exists(path):
                continue
            img = load_rgb(path, TILE - 8)
            x = c * TILE + (TILE - img.width) // 2
            sheet.paste(img, (x, y + (TILE - img.height) // 2))
            if label:
                d.text((c * TILE + 6, y + TILE - 16), label, fill=(180, 190, 200), font=f_lab)
        y += TILE
    sheet.save(out_path)
    return out_path


def main():
    with open(os.path.join(OUT, "meta.json"), encoding="utf-8") as f:
        entries = json.load(f)["entries"]
    by_id = {e["id"]: e for e in entries}

    def a(sid):
        e = by_id[sid]
        return e["raw"], "%s s%d" % (e["pkey"], e["seed"])

    def b(sid):
        e = by_id[sid]
        return e["raw3"], "cn%.2f" % e["depth_strength"]

    rows = [
        {"title": "A / DreamShaper XL v1 / p1 base concept", "cells": [a("A_dream_p1_s101"), a("A_dream_p1_s202")]},
        {"title": "A / DreamShaper / p2 style anchors + subject",
         "cells": [a("A_dream_p2_frigate_s303"), a("A_dream_p2_freighter_s404"), a("A_dream_p2_crystal_s505")]},
        {"title": "A / DreamShaper / p3 photoreal film still", "cells": [a("A_dream_p3_s606"), a("A_dream_p3_s707")]},
        {"title": "A / Juggernaut XL v9 / p1 base concept", "cells": [a("A_jugg_p1_s101"), a("A_jugg_p1_s202")]},
        {"title": "A / Juggernaut / p2 style anchors + subject",
         "cells": [a("A_jugg_p2_frigate_s303"), a("A_jugg_p2_freighter_s404"), a("A_jugg_p2_crystal_s505")]},
        {"title": "A / Juggernaut / p3 photoreal film still", "cells": [a("A_jugg_p3_s606"), a("A_jugg_p3_s707")]},
        {"title": "A / LoRA s3r3n1ty strength 0.6 (p1)",
         "cells": [a("A_dream_p1_s101_lora"), a("A_jugg_p1_s202_lora")]},
        {"title": "B / weak Depth CN (denoise 0.90) - humans",
         "cells": [b("B_humans_cn030"), b("B_humans_cn045")]},
        {"title": "B / weak Depth CN - coastal",
         "cells": [b("B_coastal_cn030"), b("B_coastal_cn045")]},
        {"title": "B / weak Depth CN - crystallites",
         "cells": [b("B_crystallites_cn030"), b("B_crystallites_cn045")]},
        {"title": "WHAT WAS / Blender render + CN 0.8 (spike 6, 200x200)",
         "cells": [(os.path.join(BL, "bl_%s_200.png" % BL_BEST[r]), r) for r in RACES]},
        {"title": "WHAT WAS / geometry v4 final (spike 5, 200x200)",
         "cells": [(os.path.join(GEO, "v4_%s_final_200.png" % r), r) for r in RACES]},
    ]
    compose(rows, os.path.join(OUT, "preview_montage.png"),
            "SPIKE 7 - ship form: A txt2img vs B weak-CN vs previous")

    compose(rows[:7], os.path.join(OUT, "branch_a.png"), "SPIKE 7 branch A - pure txt2img")
    compose(rows[7:10], os.path.join(OUT, "branch_b.png"), "SPIKE 7 branch B - weak Depth ControlNet")

    # --- метрики ---
    report = {"branch_a": {}, "branch_b": {}}
    for sid, e in by_id.items():
        raw_path = e.get("raw") or e.get("raw3")
        base = {"id": sid, "model": e.get("model"), "seed": e.get("seed"),
                "prompt": e.get("prompt"), "lora": e.get("lora"),
                "candidate": e["candidate"], "raw": raw_path,
                "m_candidate": metrics(e["candidate"]), "m_raw": raw_metrics(raw_path)}
        if e["branch"] == "A":
            base["pkey"] = e["pkey"]
            base["subject"] = e["subject"]
            report["branch_a"][sid] = base
        else:
            base["race"] = e["race"]
            base["depth_strength"] = e["depth_strength"]
            sil = render_silhouette(e["race"])
            best, direct, flip = iou_best_flip(alpha_mask(e["candidate"]), alpha_mask(sil))
            base["iou_vs_render"] = {"best": best, "direct": direct, "flipped": flip}
            report["branch_b"][sid] = base

    with open(os.path.join(OUT, "report.json"), "w", encoding="utf-8") as f:
        json.dump(report, f, ensure_ascii=False, indent=1)

    print("=== BRANCH A ===")
    for sid in sorted(report["branch_a"]):
        r = report["branch_a"][sid]
        print("%-34s colors=%-5d edges=%.3f cover=%.3f raw_colors=%-5d"
              % (sid, r["m_candidate"]["unique_colors"], r["m_candidate"]["edge_density"],
                 r["m_candidate"]["coverage"], r["m_raw"]["colors"]))
    print("=== BRANCH B ===")
    for sid in sorted(report["branch_b"]):
        r = report["branch_b"][sid]
        print("%-24s colors=%-5d edges=%.3f cover=%.3f IoU=%.3f"
              % (sid, r["m_candidate"]["unique_colors"], r["m_candidate"]["edge_density"],
                 r["m_candidate"]["coverage"], r["iou_vs_render"]["best"]))
    print("montage: %s" % os.path.join(OUT, "preview_montage.png"))


if __name__ == "__main__":
    main()
