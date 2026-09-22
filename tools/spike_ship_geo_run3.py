# -*- coding: utf-8 -*-
# СПАЙК 4 (главное): пайплайн «серая болванка -> красивый арт».
# Гипотеза: «+500%» даёт не низкий denoise (там ИИ бережёт нашу заливку), а
# ВЫСОКИЙ denoise 0.70–0.90 + наш рендер как подсказка формы через ControlNet.
# Вход — рендеры render3/ (спайк 4: перспектива, 3/4-ракурс, elevation/yaw).
# Сетка на humans/coastal: CN depth (0.6/0.8/1.0) × denoise (0.70/0.80/0.90),
# canny 0.8, stacked (depth+canny), 2 формулировки промпта, LoRA s3r3nity 0.6,
# лестница ракурсов (0/15/25/40°) при фиксированном denoise. Этап 2 (фактура)
# как в спайке 3; этап 3 (Hi-Res) — только на финалистах. Go/студия не тронуты.
import argparse
import json
import os
import subprocess
import time

from spike_ship_geo_run import (JUGG, PY, RACES, ROOT, UPSCALER, _base, _copy, download,
                                post, refresh_models, stage_img2img, stage_upscale, wait)

OUT = os.path.join(ROOT, "ai_drafts", "geometry_ships")
RENDER3 = os.path.join(OUT, "render3")
DREAM = "dreamshaper-xl-v1.safetensors"
LORA = "s3r3n1ty_SDXL_v1_32-000031.safetensors"
CN_CANNY = "controlnet-canny-sdxl-1.0.safetensors"
CN_DEPTH = "xinsir-controlnet-depth-sdxl-1.0\\diffusion_pytorch_model.safetensors"

# Концепт-арт-якоря (3/4-ракурс; в отличие от спайка 3 плоский top-down запрещён
# уже в позитиве, поэтому в негативе нет «three-quarter view / side view»).
TAIL3 = ("sci-fi concept art, three-quarter view, cinematic lighting, hard-surface greebles, "
         "panel lines, greebled hull, detailed surface structure, single ship, centered, "
         "on black background, subtle rim light, no text, no watermark")

SHIP_NEG3 = ("text, watermark, signature, blurry, low quality, deformed, ugly, duplicate, "
             "cropped, cut off, human, person, face, portrait, multiple ships, background "
             "scenery, stars, planet, nebula, ground, flat, toy, plastic, cartoon, cel shading, "
             "cel shaded, smooth featureless surface, featureless, plain surface, no detail, "
             "low contrast, sticker, logo, blurry edges")

ESSENCE = {
    "humans": "a sleek winged human military spacecraft",
    "coastal": "an organic coral-and-shell house barge starship",
    "crystallites": "a faceted crystal capsule starship",
}


def prompt_for(slug, key):
    tex = RACES[slug]["tex"]
    if key == "p1":   # concept-art
        return "sci-fi concept art of " + ESSENCE[slug] + ", " + tex + ", " + TAIL3 + \
               ", artstation trending, octane render, ultra detailed, masterpiece"
    if key == "p2":   # hard-surface greebles
        return tex + ", hard-surface greebled hull, dense mechanical detail, panel lines, rivets, " + \
               "layered armor plates, cinematic rim light, " + TAIL3
    if key == "p3":   # чистый «концепт», без ржавчины/грязи
        clean = tex.replace("subtle weathering, ", "").replace("wet reflective sheen, ", "")
        return "clean futuristic " + ESSENCE[slug] + ", " + clean + \
               ", polished panels, cyan glowing accents, crisp panel lines, " + TAIL3
    return tex + ", " + TAIL3      # p0


# негатив для p3: убрать «танковую» грязь, к которой скатывался DreamShaper
NEG_CLEAN = SHIP_NEG3 + (", rust, dirt, grime, brown mud, camouflage, weathered, rusty, "
                         "military tank, dirty, muddy, sepia")


def cn_depth(s):
    return [{"kind": "depth", "cn": CN_DEPTH, "strength": s}]


def cn_canny(s):
    return [{"kind": "canny", "cn": CN_CANNY, "strength": s}]


def cn_stack(sd, sc):
    return [{"kind": "depth", "cn": CN_DEPTH, "strength": sd},
            {"kind": "canny", "cn": CN_CANNY, "strength": sc}]


def stage_cn_multi(src_name, depth_name, prompt, neg, seed, model, lora, lora_strength,
                   denoise, cfg, controls, prefix):
    """ControlNet-стек от рендера: canny от src, depth от depth-карты; цепочкой
    ControlNetApplyAdvanced (stacked = depth + canny одновременно)."""
    nodes, mp, cp = _base(model, lora, lora_strength)
    nodes["2"] = {"class_type": "CLIPTextEncode", "inputs": {"text": prompt, "clip": cp}}
    nodes["3"] = {"class_type": "CLIPTextEncode", "inputs": {"text": neg, "clip": cp}}
    nodes["8"] = {"class_type": "LoadImage", "inputs": {"image": src_name}}
    pos, neg_in = ["2", 0], ["3", 0]
    nid = 30
    for c in controls:
        if c["kind"] == "canny":
            nodes[str(nid)] = {"class_type": "Canny", "inputs": {
                "image": ["8", 0], "low_threshold": 0.2, "high_threshold": 0.5}}
        else:
            nodes[str(nid)] = {"class_type": "LoadImage", "inputs": {"image": depth_name}}
        nodes[str(nid + 1)] = {"class_type": "ControlNetLoader",
                               "inputs": {"control_net_name": c["cn"]}}
        app = str(nid + 2)
        nodes[app] = {"class_type": "ControlNetApplyAdvanced", "inputs": {
            "positive": pos, "negative": neg_in, "control_net": [str(nid + 1), 0],
            "image": [str(nid), 0], "strength": c["strength"], "start_percent": 0.0, "end_percent": 1.0}}
        pos, neg_in = [app, 0], [app, 1]
        nid += 3
    nodes["4"] = {"class_type": "VAEEncode", "inputs": {"pixels": ["8", 0], "vae": ["1", 2]}}
    nodes["5"] = {"class_type": "KSampler", "inputs": {
        "seed": seed, "steps": 30, "cfg": cfg, "sampler_name": "dpmpp_2m", "scheduler": "karras",
        "denoise": denoise, "model": mp, "positive": pos, "negative": neg_in,
        "latent_image": ["4", 0]}}
    nodes["6"] = {"class_type": "VAEDecode", "inputs": {"samples": ["5", 0], "vae": ["1", 2]}}
    nodes["7"] = {"class_type": "SaveImage", "inputs": {"images": ["6", 0], "filename_prefix": prefix}}
    return nodes


def job(slug, tag, elev, controls, denoise, pkey, stage3=False, model=DREAM, lora=None,
        lora_strength=0.0, neg=None):
    return {"id": "%s_%s" % (slug, tag), "race": slug, "tag": tag, "elev": elev,
            "controls": controls, "denoise": denoise, "prompt": pkey, "stage3": stage3,
            "model": model, "lora": lora, "lora_strength": lora_strength, "neg": neg}


def build_jobs():
    jobs = []
    for slug in ("humans", "coastal"):
        for s, t in ((0.6, "d06"), (0.8, "d08"), (1.0, "d10")):      # глубина: сила CN
            jobs.append(job(slug, t, 25, cn_depth(s), 0.80, "p0"))
        jobs.append(job(slug, "c08", 25, cn_canny(0.8), 0.80, "p0"))
        jobs.append(job(slug, "st", 25, cn_stack(0.8, 0.8), 0.80, "p0"))
        jobs.append(job(slug, "d08n70", 25, cn_depth(0.8), 0.70, "p0"))   # denoise-свип
        jobs.append(job(slug, "d08n90", 25, cn_depth(0.8), 0.90, "p0"))
    # формулировки промпта (humans, depth 0.8 / denoise 0.80)
    jobs.append(job("humans", "d08p1", 25, cn_depth(0.8), 0.80, "p1"))
    jobs.append(job("humans", "d08p2", 25, cn_depth(0.8), 0.80, "p2"))
    jobs.append(job("humans", "d08p3", 25, cn_depth(0.8), 0.80, "p3", neg=NEG_CLEAN))
    jobs.append(job("coastal", "d08p3", 25, cn_depth(0.8), 0.80, "p3", neg=NEG_CLEAN))
    # LoRA s3r3nity (0.6) при высоком denoise
    jobs.append(job("humans", "d08lora", 25, cn_depth(0.8), 0.85, "p0",
                    lora=LORA, lora_strength=0.6))
    # A/B модели: Juggernaut XL v9 на том же варианте
    jobs.append(job("humans", "d08_jugg", 25, cn_depth(0.8), 0.80, "p0", model=JUGG))
    # лестница ракурсов humans (25° — это d08)
    for el in (0, 15, 40):
        jobs.append(job("humans", "e%02d" % el, el, cn_depth(0.8), 0.80, "p0"))
    # зонд crystallites
    jobs.append(job("crystallites", "d08", 25, cn_depth(0.8), 0.80, "p0"))
    return jobs


def _cn_names(slug, elev):
    src = os.path.join(RENDER3, "%s_e%02d.png" % (slug, int(elev)))
    dep = os.path.join(RENDER3, "%s_e%02d_depth.png" % (slug, int(elev)))
    n_src = "geo3_%s_e%02d.png" % (slug, int(elev))
    n_dep = "geo3_%s_e%02d_depth.png" % (slug, int(elev))
    _copy(src, n_src)
    _copy(dep, n_dep)
    return n_src, n_dep


def run_job(j):
    slug, jid = j["race"], j["id"]
    seed = RACES[slug]["seed"]
    src_name, dep_name = _cn_names(slug, j["elev"])
    tex = RACES[slug]["tex"]
    prompt = prompt_for(slug, j["prompt"])
    neg = j.get("neg") or SHIP_NEG3
    raw = {1: os.path.join(OUT, "%s_1.png" % jid),
           2: os.path.join(OUT, "%s_2.png" % jid),
           3: os.path.join(OUT, "%s_3.png" % jid)}
    t = {}
    # --- этап 1: ControlNet от рендера, высокий denoise (ИИ перерисовывает) ---
    if not os.path.exists(raw[1]):
        t0 = time.time()
        wf = stage_cn_multi(src_name, dep_name, prompt, neg, seed, j["model"],
                            j["lora"], j["lora_strength"], denoise=j["denoise"], cfg=6.0,
                            controls=j["controls"], prefix="geo3_s1")
        download(wait(post(wf)), raw[1])
        t["t1"] = round(time.time() - t0, 1)
    r1 = "geo3_%s_r1.png" % jid
    _copy(raw[1], r1)
    # --- этап 2: фактура (img2img 0.45), как в спайке 3 ---
    if not os.path.exists(raw[2]):
        t0 = time.time()
        wf = stage_img2img(r1, tex + ", " + TAIL3, neg, seed, j["model"],
                           j["lora"], j["lora_strength"], denoise=0.45, cfg=6.5, prefix="geo3_s2")
        download(wait(post(wf)), raw[2])
        t["t2"] = round(time.time() - t0, 1)
    # --- этап 3 (Hi-Res) — только финалисты ---
    if j["stage3"]:
        r2 = "geo3_%s_r2.png" % jid
        _copy(raw[2], r2)
        if not os.path.exists(raw[3]):
            t0 = time.time()
            wf = stage_upscale(r2, tex + ", " + TAIL3, neg, seed, j["model"],
                               j["lora"], j["lora_strength"], denoise=0.40, cfg=6.0, prefix="geo3_s3")
            download(wait(post(wf)), raw[3])
            t["t3"] = round(time.time() - t0, 1)
    final = raw[3] if j["stage3"] and os.path.exists(raw[3]) else raw[2]
    # --- постобработка (--keep-parts) ---
    cand = os.path.join(OUT, "%s_200.png" % jid)
    t0 = time.time()
    subprocess.run([PY, os.path.join(ROOT, "tools", "process_ship.py"), final, cand, "--keep-parts"],
                   check=True)
    t["t4"] = round(time.time() - t0, 1)
    return {
        "id": jid, "race": slug, "archetype": RACES[slug]["archetype"], "tag": j["tag"],
        "elev": j["elev"], "model": j["model"], "lora": j["lora"],
        "lora_strength": j["lora_strength"], "seed": seed,
        "source": os.path.join(RENDER3, "%s_e%02d.png" % (slug, int(j["elev"]))),
        "depth_source": os.path.join(RENDER3, "%s_e%02d_depth.png" % (slug, int(j["elev"]))),
        "params": {"controls": j["controls"], "stage1_denoise": j["denoise"],
                   "prompt": j["prompt"], "denoise_2": 0.45,
                   "stage3": bool(j["stage3"]), "steps": 30,
                   "upscaler": UPSCALER if j["stage3"] else None,
                   "scale": 2048 if j["stage3"] else 1536, "keep_parts": True},
        "raw1": raw[1], "raw2": raw[2], "raw3": raw[3] if j["stage3"] else None,
        "final_raw": final, "candidate": cand, "timings": t,
    }


def main():
    ap = argparse.ArgumentParser(description="Спайк 4: пайплайн «болванка -> арт» через ComfyUI")
    ap.add_argument("--limit", type=int, default=0)
    ap.add_argument("--jobs", default="")
    ap.add_argument("--stage3", default="", help="id финалистов, для которых гнать этап 3")
    args = ap.parse_args()
    os.makedirs(OUT, exist_ok=True)
    refresh_models()
    jobs = build_jobs()
    s3 = {x for x in args.stage3.split(",") if x}
    for j in jobs:
        if j["id"] in s3:
            j["stage3"] = True
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
    entries = meta.get("entries_v3", [])
    done = {e["id"] for e in entries}
    for j in jobs:
        cand = os.path.join(OUT, "%s_200.png" % j["id"])
        raw3 = os.path.join(OUT, "%s_3.png" % j["id"])
        if j["id"] in done and os.path.exists(cand) and (not j["stage3"] or os.path.exists(raw3)):
            print("SKIP %s" % j["id"])
            continue
        t0 = time.time()
        rec = run_job(j)
        rec["timings"]["total"] = round(time.time() - t0, 1)
        entries = [e for e in entries if e["id"] != rec["id"]] + [rec]
        meta["entries_v3"] = entries
        with open(meta_path, "w", encoding="utf-8") as f:
            json.dump(meta, f, ensure_ascii=False, indent=1)
        print("DONE %s: %s" % (rec["id"], rec["timings"]))
    print("meta written: %s" % meta_path)


if __name__ == "__main__":
    main()
