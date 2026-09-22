# -*- coding: utf-8 -*-
# СПАЙК 3: прогон ОБЪЁМНЫХ рендеров С ГРИБЛЗАМИ через ComfyUI HTTP API.
# Рендеры берутся из ai_drafts/geometry_ships/render2/ (спайк 3: деталь-слой).
# Варианты: V1g (ControlNet Canny от рендера, denoise 0.55), V2g (img2img 0.45),
# V5 (img2img 0.40 — «ИИ только наводит материал»), Juggernaut XL v9 на humans.
# Для каждого — этап 2 (img2img 0.45) и этап 3 (4x-UltraSharp → 2048 → img2img
# 0.40), затем tools/process_ship.py --keep-parts (200×200). Go/студия не тронуты.
import argparse
import json
import os
import subprocess
import time

from spike_ship_geo_run import (INPUT_DIR, JUGG, PY, RACES, ROOT, SHIP_NEG, TAIL, UPSCALER,
                                _copy, download, post, refresh_models, stage_cn, stage_img2img,
                                stage_upscale, wait)

OUT = os.path.join(ROOT, "ai_drafts", "geometry_ships")
RENDER = os.path.join(OUT, "render2")
SPECS_FILE = os.path.join(ROOT, "ai_drafts", "spike_ships", "spike_specs.json")
DREAM = "dreamshaper-xl-v1.safetensors"

# вариант -> (способ этапа 1, denoise, cn_strength)
VARIANTS = {
    "V1g": ("cn", 0.55, 1.0),
    "V2g": ("img2img", 0.45, 0.0),
    "V5": ("img2img", 0.40, 0.0),
}


def geo_prompt(slug):
    return RACES[slug]["tex"] + ", " + TAIL


def build_jobs():
    jobs = []
    for slug in ("humans", "coastal", "crystallites"):
        for v in ("V1g", "V2g", "V5"):
            jobs.append({"race": slug, "variant": v, "model": DREAM, "tag": ""})
    # A/B модели: Juggernaut XL v9 (эталон 21 спрайта) на humans
    jobs.append({"race": "humans", "variant": "V1g", "model": JUGG, "tag": "jugg"})
    for j in jobs:
        j["id"] = "%s_%s%s" % (j["race"], j["variant"], ("_" + j["tag"]) if j["tag"] else "")
    return jobs


def run_job(job):
    slug, variant = job["race"], job["variant"]
    cfg = RACES[slug]
    seed = cfg["seed"]
    jid = job["id"]
    model = job["model"]
    mode, denoise, cn = VARIANTS[variant]
    t = {}
    src = os.path.join(RENDER, "%s_tilt.png" % slug)
    src_name = "geo2_%s_tilt.png" % slug
    _copy(src, src_name)
    tex = cfg["tex"]
    raw = {1: os.path.join(OUT, "%s_1.png" % jid),
           2: os.path.join(OUT, "%s_2.png" % jid),
           3: os.path.join(OUT, "%s_3.png" % jid)}
    # --- этап 1 (материал/фактура от рендера) ---
    if not os.path.exists(raw[1]):
        t0 = time.time()
        if mode == "cn":
            wf = stage_cn(src_name, geo_prompt(slug), SHIP_NEG, seed, model, None, 0.0,
                          denoise=denoise, cfg=6.0, cn_strength=cn, prefix="geo2_s1")
        else:
            wf = stage_img2img(src_name, geo_prompt(slug), SHIP_NEG, seed, model, None, 0.0,
                               denoise=denoise, cfg=6.0, prefix="geo2_s1")
        download(wait(post(wf)), raw[1])
        t["t1"] = round(time.time() - t0, 1)
    r1 = "geo2_%s_r1.png" % jid
    _copy(raw[1], r1)
    # --- этап 2 (фактура) ---
    if not os.path.exists(raw[2]):
        t0 = time.time()
        wf = stage_img2img(r1, tex, SHIP_NEG, seed, model, None, 0.0,
                           denoise=0.45, cfg=6.5, prefix="geo2_s2")
        download(wait(post(wf)), raw[2])
        t["t2"] = round(time.time() - t0, 1)
    r2 = "geo2_%s_r2.png" % jid
    _copy(raw[2], r2)
    # --- этап 3 (детализация/Hi-Res) ---
    if not os.path.exists(raw[3]):
        t0 = time.time()
        wf = stage_upscale(r2, tex, SHIP_NEG, seed, model, None, 0.0,
                           denoise=0.40, cfg=6.0, prefix="geo2_s3")
        download(wait(post(wf)), raw[3])
        t["t3"] = round(time.time() - t0, 1)
    # --- постобработка (--keep-parts: крыло/спутник, отделённые тёмным швом) ---
    cand = os.path.join(OUT, "%s_200.png" % jid)
    t0 = time.time()
    subprocess.run([PY, os.path.join(ROOT, "tools", "process_ship.py"), raw[3], cand, "--keep-parts"],
                   check=True)
    t["t4"] = round(time.time() - t0, 1)
    return {
        "id": jid, "race": slug, "archetype": cfg["archetype"], "variant": variant,
        "model": model, "seed": seed, "source": src,
        "params": {"stage1_mode": mode, "stage1_denoise": denoise, "cn_strength": cn,
                   "denoise_2": 0.45, "denoise_3": 0.40, "steps": 30,
                   "upscaler": UPSCALER, "scale": 2048, "keep_parts": True},
        "raw1": raw[1], "raw2": raw[2], "raw3": raw[3], "candidate": cand,
        "timings": t,
    }


def greeble_counts():
    from spike_ship_geo import build_scene
    with open(SPECS_FILE, encoding="utf-8") as f:
        specs = json.load(f)
    return {s["race"]: build_scene(s).greeble_count for s in specs}


def main():
    ap = argparse.ArgumentParser(description="Спайк 3: прогон объёмных кораблей (гриблзы)")
    ap.add_argument("--limit", type=int, default=0)
    ap.add_argument("--jobs", default="")
    args = ap.parse_args()
    os.makedirs(OUT, exist_ok=True)
    refresh_models()
    jobs = build_jobs()
    if args.jobs:
        want = set(args.jobs.split(","))
        jobs = [j for j in jobs if j["id"] in want]
    if args.limit:
        jobs = jobs[:args.limit]
    meta_path = os.path.join(OUT, "geo_meta.json")
    meta = {}
    if os.path.exists(meta_path):
        with open(meta_path, encoding="utf-8") as f:
            meta = json.load(f)
    entries = meta.get("entries_v2", [])
    meta["greeble_counts"] = greeble_counts()
    done = {e["id"] for e in entries}
    for job in jobs:
        if job["id"] in done and os.path.exists(os.path.join(OUT, "%s_200.png" % job["id"])):
            print("SKIP %s" % job["id"])
            continue
        t0 = time.time()
        rec = run_job(job)
        rec["timings"]["total"] = round(time.time() - t0, 1)
        entries = [e for e in entries if e["id"] != rec["id"]] + [rec]
        meta["entries_v2"] = entries
        with open(meta_path, "w", encoding="utf-8") as f:
            json.dump(meta, f, ensure_ascii=False, indent=1)
        print("DONE %s: %s" % (rec["id"], rec["timings"]))
    print("meta written: %s" % meta_path)


if __name__ == "__main__":
    main()
