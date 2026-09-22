# -*- coding: utf-8 -*-
# СПАЙК 6: ИИ-пайплайн поверх ОБЪЁМНОГО рендера корабля из SpaceshipGenerator.
# Источник — blender color.png (init для img2img) + depth.png (ControlNet depth).
# Цепочка: Depth ControlNet (xinsir depth, strength 0.8) -> denoise 0.80 ->
#          img2img 0.45 -> Hi-Res (4x-UltraSharp -> 2048 -> img2img 0.40) ->
#          tools/process_ship.py --keep-parts (200x200).
# Промпты: humans — weathered/индустриальный; coastal/crystallites — clean.
# LoRA s3r3n1ty_SDXL_v1_32-000031.safetensors сила 0.6 — одна цепочка (на humans).
# Go/студия не тронуты.
import argparse
import json
import os
import subprocess
import time

from spike_ship_geo_run import (DREAM, PY, ROOT, UPSCALER, _copy, download, post,
                                refresh_models, stage_img2img, stage_upscale, wait)
from spike_ship_geo_run4 import stage_cn_stack

OUT = os.path.join(ROOT, "ai_drafts", "blender_ships")
RAW = os.path.join(OUT, "raw")
LORA = "s3r3n1ty_SDXL_v1_32-000031.safetensors"

RACES = {
    "humans":       {"seed": 1715645486, "prompt": "weathered"},
    "coastal":      {"seed": 1245074369, "prompt": "clean"},
    "crystallites": {"seed": 3729850754, "prompt": "clean"},
}

# Якоря: держим «корабль», режем самолёт/баржу прямо в негативе.
ANCHORS = ("sci-fi spaceship concept art, hard-surface, greebles, panel lines, cinematic lighting, "
           "single ship, centered, three-quarter view, on black background, subtle rim light, "
           "artstation, no text, no watermark")
MATERIAL = {
    "humans":       ("weathered industrial military starship, battle-worn plating, rust streaks and "
                     "scorch marks, exposed machinery, vents, hatches, dense panel lines, scuffed paint"),
    "coastal":      ("organic grown starship, coral and shell plating, seamless tissue-like hull, "
                     "bioluminescent accents, smooth unblemished surfaces"),
    "crystallites": ("faceted crystal starship, translucent frozen plates, ice-crystal structure, "
                     "glowing crystal cores, hard angular facets"),
}
SHIP_NEG = ("text, watermark, signature, blurry, low quality, deformed, ugly, duplicate, cropped, "
            "cut off, human, person, face, portrait, multiple ships, background scenery, stars, "
            "planet, nebula, ground, flat, toy, plastic, cartoon, cel shading, smooth featureless "
            "surface, featureless, plain surface, no detail, low contrast, sticker, logo, blurry "
            "edges, airplane, aircraft, jet, plane, wings, fighter jet, barge, train, locomotive, "
            "bricks, rock, stone, mountain, castle")
NEG_CLEAN = SHIP_NEG + (", rust, dirt, grime, brown mud, camouflage, weathered, rusty, "
                        "military tank, dirty, muddy, sepia, scratches, stains")
NEG_WEATHERED = SHIP_NEG + ", clean pristine glossy showroom, plastic toy"


def pos(slug, prompt):
    lead = "clean futuristic " if prompt == "clean" else "battle-worn "
    return lead + "spaceship, " + MATERIAL[slug] + ", " + ANCHORS


def build_jobs():
    jobs = [
        # humans: базовая цепочка (weathered) + рычаги (clean, LoRA, сила CN, denoise)
        {"id": "humans_base", "race": "humans", "cn": 0.8, "dn1": 0.80},
        {"id": "humans_clean", "race": "humans", "cn": 0.8, "dn1": 0.80, "prompt": "clean"},
        {"id": "humans_lora06", "race": "humans", "cn": 0.8, "dn1": 0.80, "lora": LORA,
         "lora_strength": 0.6},
        {"id": "humans_cn06", "race": "humans", "cn": 0.6, "dn1": 0.80},
        {"id": "humans_cn10", "race": "humans", "cn": 1.0, "dn1": 0.80},
        {"id": "humans_dn065", "race": "humans", "cn": 0.8, "dn1": 0.65},
        # coastal / crystallites: базовая (clean) + сила CN
        {"id": "coastal_base", "race": "coastal", "cn": 0.8, "dn1": 0.80},
        {"id": "coastal_cn06", "race": "coastal", "cn": 0.6, "dn1": 0.80},
        {"id": "coastal_cn10", "race": "coastal", "cn": 1.0, "dn1": 0.80},
        {"id": "crystallites_base", "race": "crystallites", "cn": 0.8, "dn1": 0.80},
        {"id": "crystallites_cn06", "race": "crystallites", "cn": 0.6, "dn1": 0.80},
        {"id": "crystallites_cn10", "race": "crystallites", "cn": 1.0, "dn1": 0.80},
    ]
    for j in jobs:
        j.setdefault("prompt", RACES[j["race"]]["prompt"])
        j.setdefault("lora", None)
        j.setdefault("lora_strength", 0.0)
    return jobs


def run_job(j):
    slug, jid = j["race"], j["id"]
    seed = RACES[slug]["seed"]
    src = os.path.join(RAW, slug, "color.png")
    dep = os.path.join(RAW, slug, "depth.png")
    src_name = "bl_%s.png" % slug
    dep_name = "bl_%s_dep.png" % slug
    _copy(src, src_name)
    _copy(dep, dep_name)

    p1 = pos(slug, j["prompt"])
    p2 = p1 + ", maximal detail, dense surface detail, masterpiece"
    n1 = NEG_CLEAN if j["prompt"] == "clean" else NEG_WEATHERED

    controls = [{"kind": "depth", "strength": j["cn"], "image": dep_name}]
    raw1 = os.path.join(OUT, "bl_%s_1.png" % jid)
    raw2 = os.path.join(OUT, "bl_%s_2.png" % jid)
    raw3 = os.path.join(OUT, "bl_%s_3.png" % jid)
    t = {}
    if not os.path.exists(raw1):
        t0 = time.time()
        wf = stage_cn_stack(src_name, controls, p1, n1, seed, DREAM, j["lora"], j["lora_strength"],
                            denoise=j["dn1"], cfg=6.0, steps=30, prefix="bl_s1")
        download(wait(post(wf)), raw1)
        t["stage1"] = round(time.time() - t0, 1)
    r1 = "bl_%s_r1.png" % jid
    _copy(raw1, r1)
    if not os.path.exists(raw2):
        t0 = time.time()
        wf = stage_img2img(r1, p2, n1, seed, DREAM, j["lora"], j["lora_strength"],
                           denoise=0.45, cfg=6.5, prefix="bl_s2")
        download(wait(post(wf)), raw2)
        t["stage2"] = round(time.time() - t0, 1)
    if not os.path.exists(raw3):
        r2 = "bl_%s_r2.png" % jid
        _copy(raw2, r2)
        t0 = time.time()
        wf = stage_upscale(r2, p2, n1, seed, DREAM, j["lora"], j["lora_strength"],
                           denoise=0.40, cfg=6.0, prefix="bl_s3")
        download(wait(post(wf)), raw3)
        t["stage3"] = round(time.time() - t0, 1)
    cand = os.path.join(OUT, "bl_%s_200.png" % jid)
    t0 = time.time()
    subprocess.run([PY, os.path.join(ROOT, "tools", "process_ship.py"), raw3, cand,
                    "--keep-parts"], check=True)
    t["post"] = round(time.time() - t0, 1)
    return {
        "id": jid, "race": slug, "seed": seed, "prompt": j["prompt"],
        "depth_strength": j["cn"], "denoise1": j["dn1"], "denoise2": 0.45, "denoise3": 0.40,
        "model": DREAM, "lora": j["lora"], "lora_strength": j["lora_strength"],
        "upscaler": UPSCALER, "scale3": 2048, "keep_parts": True,
        "source_color": src, "source_depth": dep, "prompt_text": p1,
        "raw1": raw1, "raw2": raw2, "raw3": raw3, "candidate": cand, "timings": t,
    }


def main():
    ap = argparse.ArgumentParser(description="Спайк 6: ИИ поверх blender-рендера кораблей")
    ap.add_argument("--jobs", default="")
    ap.add_argument("--limit", type=int, default=0)
    args = ap.parse_args()
    os.makedirs(OUT, exist_ok=True)
    refresh_models()
    jobs = build_jobs()
    if args.jobs:
        want = set(args.jobs.split(","))
        jobs = [j for j in jobs if j["id"] in want]
    if args.limit:
        jobs = jobs[:args.limit]
    meta_path = os.path.join(OUT, "meta.json")
    meta = {}
    if os.path.exists(meta_path):
        with open(meta_path, encoding="utf-8") as f:
            meta = json.load(f)
    entries = meta.get("entries", [])
    done = {e["id"] for e in entries}
    for j in jobs:
        cand = os.path.join(OUT, "bl_%s_200.png" % j["id"])
        if j["id"] in done and os.path.exists(cand):
            print("SKIP %s" % j["id"])
            continue
        t0 = time.time()
        rec = run_job(j)
        rec["timings"]["total"] = round(time.time() - t0, 1)
        entries = [e for e in entries if e["id"] != rec["id"]] + [rec]
        meta["entries"] = entries
        with open(meta_path, "w", encoding="utf-8") as f:
            json.dump(meta, f, ensure_ascii=False, indent=1)
        print("DONE %s: %s" % (rec["id"], rec["timings"]))
    print("meta written: %s" % meta_path)


if __name__ == "__main__":
    main()
