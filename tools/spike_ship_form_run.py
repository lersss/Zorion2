# -*- coding: utf-8 -*-
# СПАЙК 7: "форма корабля от ИИ" — две ветки.
#  A) чистый txt2img (без ControlNet и без нашего рендера): DreamShaper XL v1 /
#     Juggernaut XL v9, ракурс 3/4 сверху, одиночный корабль по центру, 1024 px;
#     3 промпта (база / якоря стиля + субъект / фотореализм).
#  B) наша геометрия как СЛАБАЯ подсказка: blender color.png + ControlNet Depth
#     strength 0.30 / 0.45, denoise 0.90 -> img2img 0.45 -> Hi-Res 2048 0.40 ->
#     tools/process_ship.py --keep-parts.
# Результаты в ai_drafts/ai_form. Go/студия не тронуты. LoRA — 2 прогона ветки A.
import argparse
import json
import os
import subprocess
import time

from spike_ship_geo_run import (DREAM, JUGG, PY, ROOT, UPSCALER, _base, _copy, download,
                                post, refresh_models, stage_img2img, stage_upscale, wait)
from spike_ship_geo_run4 import stage_cn_stack
from spike_ship_blender_run import NEG_CLEAN, NEG_WEATHERED, RAW as BL_RAW, pos

OUT = os.path.join(ROOT, "ai_drafts", "ai_form")
LORA = "s3r3n1ty_SDXL_v1_32-000031.safetensors"
A_SIZE = 1024

A_NEG = ("(toy:1.1), plastic, cartoon, cel shading, flat, smooth featureless surface, "
         "airplane, jet, boat, multiple ships, text, watermark, blurry, low detail, "
         "signature, logo, cropped, deformed, ugly, low contrast")

A_BASE = ("(sci-fi spaceship concept art:1.2), (hard-surface hull:1.15), dense greebles, "
          "panel lines, (engine block:1.1), cinematic lighting, 3/4 top view, centered "
          "single ship, dark background, artstation, octane render, highly detailed")

A_PROMPTS = {
    # p1 — база; p2 — якоря стиля + субъект (по одному на прогон); p3 — фотореализм.
    "p1": A_BASE,
    "p2": A_BASE + ", (Syd Mead:1.1), (Chris Foss:1.05), Sparth, %s",
    "p3": A_BASE + ", (photorealistic sci-fi film still:1.15), dramatic rim light, volumetric lighting",
}
A_SUBJECTS = {"frigate": "military frigate", "freighter": "industrial freighter",
              "crystal": "crystal vessel"}

# Ветка B: расы и seed — как в спайке 6 (сравнение с bl_*_200).
B_RACES = {"humans": 1715645486, "coastal": 1245074369, "crystallites": 3729850754}
B_STRENGTHS = [0.30, 0.45]


def stage_txt2img(prompt, neg, seed, model, lora, lora_strength, prefix):
    nodes, mp, cp = _base(model, lora, lora_strength)
    nodes["2"] = {"class_type": "CLIPTextEncode", "inputs": {"text": prompt, "clip": cp}}
    nodes["3"] = {"class_type": "CLIPTextEncode", "inputs": {"text": neg, "clip": cp}}
    nodes["4"] = {"class_type": "EmptyLatentImage", "inputs": {
        "width": A_SIZE, "height": A_SIZE, "batch_size": 1}}
    nodes["5"] = {"class_type": "KSampler", "inputs": {
        "seed": seed, "steps": 32, "cfg": 6.0, "sampler_name": "dpmpp_2m", "scheduler": "karras",
        "denoise": 1.0, "model": mp, "positive": ["2", 0], "negative": ["3", 0],
        "latent_image": ["4", 0]}}
    nodes["6"] = {"class_type": "VAEDecode", "inputs": {"samples": ["5", 0], "vae": ["1", 2]}}
    nodes["7"] = {"class_type": "SaveImage", "inputs": {"images": ["6", 0], "filename_prefix": prefix}}
    return nodes


def build_a_jobs():
    jobs = []
    plan = [("p1", None, 101), ("p1", None, 202),
            ("p2", "frigate", 303), ("p2", "freighter", 404), ("p2", "crystal", 505),
            ("p3", None, 606), ("p3", None, 707)]
    for model, short in ((DREAM, "dream"), (JUGG, "jugg")):
        for pkey, subj, seed in plan:
            tag = pkey + ("_" + subj if subj else "")
            pos_txt = A_PROMPTS[pkey] % A_SUBJECTS[subj] if subj else A_PROMPTS[pkey]
            jobs.append({"branch": "A", "id": "A_%s_%s_s%d" % (short, tag, seed),
                         "model": model, "model_short": short, "pkey": pkey, "subject": subj,
                         "seed": seed, "prompt": pos_txt, "neg": A_NEG, "lora": None,
                         "lora_strength": 0.0})
    # LoRA-проба: 1-2 картинки ветки A, сила 0.6.
    jobs.append({"branch": "A", "id": "A_dream_p1_s101_lora", "model": DREAM, "model_short": "dream",
                 "pkey": "p1", "subject": None, "seed": 101, "prompt": A_PROMPTS["p1"],
                 "neg": A_NEG, "lora": LORA, "lora_strength": 0.6})
    jobs.append({"branch": "A", "id": "A_jugg_p1_s202_lora", "model": JUGG, "model_short": "jugg",
                 "pkey": "p1", "subject": None, "seed": 202, "prompt": A_PROMPTS["p1"],
                 "neg": A_NEG, "lora": LORA, "lora_strength": 0.6})
    return jobs


def build_b_jobs():
    jobs = []
    for slug in B_RACES:
        for s in B_STRENGTHS:
            jobs.append({"branch": "B", "id": "B_%s_cn%03d" % (slug, int(round(s * 100))),
                         "race": slug, "strength": s, "seed": B_RACES[slug],
                         "model": DREAM, "lora": None, "lora_strength": 0.0})
    return jobs


def run_a(j):
    raw = os.path.join(OUT, j["id"] + ".png")
    t = {}
    if not os.path.exists(raw):
        t0 = time.time()
        wf = stage_txt2img(j["prompt"], j["neg"], j["seed"], j["model"], j["lora"],
                           j["lora_strength"], prefix="af_s1")
        download(wait(post(wf)), raw)
        t["txt2img"] = round(time.time() - t0, 1)
    cand = os.path.join(OUT, j["id"] + "_200.png")
    t0 = time.time()
    subprocess.run([PY, os.path.join(ROOT, "tools", "process_ship.py"), raw, cand, "--keep-parts"],
                   check=True)
    t["post"] = round(time.time() - t0, 1)
    return {"id": j["id"], "branch": "A", "model": j["model"], "model_short": j["model_short"],
            "pkey": j["pkey"], "subject": j["subject"], "seed": j["seed"], "prompt": j["prompt"],
            "negative": j["neg"], "lora": j["lora"], "lora_strength": j["lora_strength"],
            "size": A_SIZE, "steps": 32, "cfg": 6.0, "sampler": "dpmpp_2m/karras",
            "raw": raw, "candidate": cand, "timings": t}


def run_b(j):
    slug, seed = j["race"], j["seed"]
    src = os.path.join(BL_RAW, slug, "color.png")
    dep = os.path.join(BL_RAW, slug, "depth.png")
    src_name = "af_%s.png" % slug
    dep_name = "af_%s_dep.png" % slug
    _copy(src, src_name)
    _copy(dep, dep_name)
    prompt = "weathered" if slug == "humans" else "clean"
    p1 = pos(slug, prompt)
    p2 = p1 + ", maximal detail, dense surface detail, masterpiece"
    neg = NEG_CLEAN if prompt == "clean" else NEG_WEATHERED
    raw1 = os.path.join(OUT, j["id"] + "_1.png")
    raw2 = os.path.join(OUT, j["id"] + "_2.png")
    raw3 = os.path.join(OUT, j["id"] + "_3.png")
    t = {}
    if not os.path.exists(raw1):
        t0 = time.time()
        wf = stage_cn_stack(src_name, [{"kind": "depth", "strength": j["strength"],
                                        "image": dep_name}], p1, neg, seed, DREAM, None, 0.0,
                            denoise=0.90, cfg=6.0, steps=30, prefix="af_b1")
        download(wait(post(wf)), raw1)
        t["stage1"] = round(time.time() - t0, 1)
    r1 = "af_%s_r1.png" % j["id"]
    _copy(raw1, r1)
    if not os.path.exists(raw2):
        t0 = time.time()
        wf = stage_img2img(r1, p2, neg, seed, DREAM, None, 0.0, denoise=0.45, cfg=6.5,
                           prefix="af_b2")
        download(wait(post(wf)), raw2)
        t["stage2"] = round(time.time() - t0, 1)
    r2 = "af_%s_r2.png" % j["id"]
    _copy(raw2, r2)
    if not os.path.exists(raw3):
        t0 = time.time()
        wf = stage_upscale(r2, p2, neg, seed, DREAM, None, 0.0, denoise=0.40, cfg=6.0,
                           prefix="af_b3")
        download(wait(post(wf)), raw3)
        t["stage3"] = round(time.time() - t0, 1)
    cand = os.path.join(OUT, j["id"] + "_200.png")
    t0 = time.time()
    subprocess.run([PY, os.path.join(ROOT, "tools", "process_ship.py"), raw3, cand,
                    "--keep-parts"], check=True)
    t["post"] = round(time.time() - t0, 1)
    return {"id": j["id"], "branch": "B", "race": slug, "depth_strength": j["strength"],
            "denoise1": 0.90, "denoise2": 0.45, "denoise3": 0.40, "model": DREAM,
            "lora": None, "seed": seed, "prompt": p1, "negative": neg,
            "source_color": src, "source_depth": dep, "upscaler": UPSCALER, "scale3": 2048,
            "keep_parts": True, "raw1": raw1, "raw2": raw2, "raw3": raw3,
            "candidate": cand, "timings": t}


def main():
    ap = argparse.ArgumentParser(description="Спайк 7: форма корабля от ИИ (A txt2img / B weak CN)")
    ap.add_argument("--branch", default="", help="A / B (пусто = обе)")
    ap.add_argument("--jobs", default="")
    ap.add_argument("--limit", type=int, default=0)
    args = ap.parse_args()
    os.makedirs(OUT, exist_ok=True)
    refresh_models()
    jobs = []
    if args.branch in ("", "A"):
        jobs += build_a_jobs()
    if args.branch in ("", "B"):
        jobs += build_b_jobs()
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
        cand = os.path.join(OUT, j["id"] + ("_200.png" if j["branch"] == "A" else "_200.png"))
        if j["id"] in done and os.path.exists(cand):
            print("SKIP %s" % j["id"])
            continue
        t0 = time.time()
        rec = (run_a if j["branch"] == "A" else run_b)(j)
        rec["timings"]["total"] = round(time.time() - t0, 1)
        entries = [e for e in entries if e["id"] != rec["id"]] + [rec]
        meta["entries"] = entries
        with open(meta_path, "w", encoding="utf-8") as f:
            json.dump(meta, f, ensure_ascii=False, indent=1)
        print("DONE %s: %s" % (rec["id"], rec["timings"]))
    print("meta written: %s" % meta_path)


if __name__ == "__main__":
    main()
