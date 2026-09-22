# -*- coding: utf-8 -*-
# СПАЙК 5: дожать «красоту» на существующих ресурсах.
#  - Normal ControlNet в стек с Depth (xinsir union + SetUnionControlNetType);
#  - кино-свет (ключ+заполняющий+rim) в НАШЕМ рендере (render4_cine / render4_flat);
#  - выразительная болванка (фаски/скругления/крепление двигателей) — render4;
#  - 3 набора промптов (clean / weathered / concept-art) с весами и якорями;
#  - многопроходность: база (denoise 0.80) -> отдельный проход детализации
#    (апскейл 1.5-2x + img2img 0.30-0.40) против текущего (0.45 + Hi-Res 0.40).
# Все прогоны — с tools/process_ship.py --keep-parts. Go/студия не тронуты.
import argparse
import json
import os
import subprocess
import time

from spike_ship_geo_run import (INPUT_DIR, JUGG, PY, RACES, ROOT, UPSCALER, _base, _copy,
                                download, post, refresh_models, stage_img2img, stage_upscale, wait)

OUT = os.path.join(ROOT, "ai_drafts", "geometry_ships")
CINE = os.path.join(OUT, "render4_cine")
FLAT = os.path.join(OUT, "render4_flat")
DREAM = "dreamshaper-xl-v1.safetensors"
LORA_DIR = r"C:\ComfyUI\models\loras"
CN_CANNY = "controlnet-canny-sdxl-1.0.safetensors"
CN_DEPTH = "xinsir-controlnet-depth-sdxl-1.0\\diffusion_pytorch_model.safetensors"
CN_UNION = "xinsir-controlnet-union-sdxl-1.0\\diffusion_pytorch_model.safetensors"
BASE_SIZE = 1536

SHIP_NEG3 = ("text, watermark, signature, blurry, low quality, deformed, ugly, duplicate, "
             "cropped, cut off, human, person, face, portrait, multiple ships, background "
             "scenery, stars, planet, nebula, ground, flat, toy, plastic, cartoon, cel shading, "
             "cel shaded, smooth featureless surface, featureless, plain surface, no detail, "
             "low contrast, sticker, logo, blurry edges")
NEG_CLEAN = SHIP_NEG3 + (", rust, dirt, grime, brown mud, camouflage, weathered, rusty, "
                         "military tank, dirty, muddy, sepia, scratches, stains")
NEG_WEATHERED = SHIP_NEG3 + ", clean pristine glossy showroom, plastic toy"

ESSENCE = {
    "humans": "a sleek winged human military spacecraft",
    "coastal": "an organic coral-and-shell house barge starship",
    "crystallites": "a faceted crystal capsule starship",
}

# Материальные якоря — по предикату «раса без машин» (спека 2026-09-21 §4.5/§6.2).
MATERIAL = {
    "humans": "industrial design, hard-surface plating, machined metal panels, riveted seams, "
              "hard-surface greebles",
    "coastal": "grown organic plating, coral and shell structure, seamless tissue-like surface, "
               "bioluminescent accents",
    "crystallites": "ice-crystal structure, translucent frozen surfaces, faceted frozen plates, "
                    "glowing crystal cores",
}


def pos_base(slug, key):
    common = ("sci-fi spaceship concept art, three-quarter view, cinematic lighting, "
              "single ship, centered, on black background, subtle rim light, artstation, "
              "no text, no watermark")
    mat = MATERIAL[slug]
    if key == "clean":
        return ("clean futuristic " + ESSENCE[slug] + ", " + mat +
                ", (polished pristine hull:1.2), (crisp white and cyan ceramic panels:1.25), "
                "cyan glowing accents, (unblemished smooth surfaces:1.1), " + common)
    if key == "weathered":
        return ("battle-worn " + ESSENCE[slug] + ", " + mat +
                ", (industrial grit:1.15), (rust streaks and scorch marks:1.1), exposed "
                "machinery, vents, hatches, dense panel lines, scuffed paint, " + common)
    return "concept art of " + ESSENCE[slug] + ", " + RACES[slug]["tex"] + ", " + common


def neg_for(key):
    if key == "clean":
        return NEG_CLEAN
    if key == "weathered":
        return NEG_WEATHERED
    return SHIP_NEG3


def stage_cn_stack(src_name, controls, prompt, neg, seed, model, lora, lora_strength,
                   denoise, cfg, steps, prefix):
    """ControlNet-стек от рендера: depth (xinsir depth), normal (union + type),
    canny — в любом порядке; цепочка ControlNetApplyAdvanced."""
    nodes, mp, cp = _base(model, lora, lora_strength)
    nodes["2"] = {"class_type": "CLIPTextEncode", "inputs": {"text": prompt, "clip": cp}}
    nodes["3"] = {"class_type": "CLIPTextEncode", "inputs": {"text": neg, "clip": cp}}
    nodes["8"] = {"class_type": "LoadImage", "inputs": {"image": src_name}}
    pos, neg_in = ["2", 0], ["3", 0]
    nid = 40
    for c in controls:
        kind = c["kind"]
        img_node = str(nid)
        if kind == "canny":
            nodes[str(nid)] = {"class_type": "Canny", "inputs": {
                "image": ["8", 0], "low_threshold": 0.2, "high_threshold": 0.5}}
            cn_file = CN_CANNY
        else:
            nodes[str(nid)] = {"class_type": "LoadImage", "inputs": {"image": c["image"]}}
            cn_file = CN_DEPTH if kind == "depth" else CN_UNION
        nodes[str(nid + 1)] = {"class_type": "ControlNetLoader",
                               "inputs": {"control_net_name": cn_file}}
        cn_out = [str(nid + 1), 0]
        if kind == "normal":
            nodes[str(nid + 2)] = {"class_type": "SetUnionControlNetType",
                                   "inputs": {"control_net": cn_out, "type": "normal"}}
            cn_out = [str(nid + 2), 0]
            app = str(nid + 3)
        else:
            app = str(nid + 2)
        nodes[app] = {"class_type": "ControlNetApplyAdvanced", "inputs": {
            "positive": pos, "negative": neg_in, "control_net": cn_out,
            "image": [img_node, 0], "strength": c["strength"],
            "start_percent": 0.0, "end_percent": 1.0}}
        pos, neg_in = [app, 0], [app, 1]
        nid += 4
    nodes["4"] = {"class_type": "VAEEncode", "inputs": {"pixels": ["8", 0], "vae": ["1", 2]}}
    nodes["5"] = {"class_type": "KSampler", "inputs": {
        "seed": seed, "steps": steps, "cfg": cfg, "sampler_name": "dpmpp_2m", "scheduler": "karras",
        "denoise": denoise, "model": mp, "positive": pos, "negative": neg_in,
        "latent_image": ["4", 0]}}
    nodes["6"] = {"class_type": "VAEDecode", "inputs": {"samples": ["5", 0], "vae": ["1", 2]}}
    nodes["7"] = {"class_type": "SaveImage", "inputs": {"images": ["6", 0], "filename_prefix": prefix}}
    return nodes


def stage_detail(img_name, prompt, neg, seed, model, lora, lora_strength, scale, denoise,
                 cfg, steps, prefix):
    """Проход детализации: 4x-UltraSharp -> ImageScale (scale*x) -> img2img."""
    size = int(round(BASE_SIZE * scale / 8.0) * 8)
    nodes, mp, cp = _base(model, lora, lora_strength)
    nodes["2"] = {"class_type": "CLIPTextEncode", "inputs": {"text": prompt, "clip": cp}}
    nodes["3"] = {"class_type": "CLIPTextEncode", "inputs": {"text": neg, "clip": cp}}
    nodes["8"] = {"class_type": "LoadImage", "inputs": {"image": img_name}}
    nodes["12"] = {"class_type": "UpscaleModelLoader", "inputs": {"model_name": UPSCALER}}
    nodes["13"] = {"class_type": "ImageUpscaleWithModel",
                   "inputs": {"upscale_model": ["12", 0], "image": ["8", 0]}}
    nodes["14"] = {"class_type": "ImageScale", "inputs": {
        "image": ["13", 0], "width": size, "height": size, "upscale_method": "lanczos",
        "crop": "disabled"}}
    nodes["4"] = {"class_type": "VAEEncode", "inputs": {"pixels": ["14", 0], "vae": ["1", 2]}}
    nodes["5"] = {"class_type": "KSampler", "inputs": {
        "seed": seed, "steps": steps, "cfg": cfg, "sampler_name": "dpmpp_2m", "scheduler": "karras",
        "denoise": denoise, "model": mp, "positive": ["2", 0], "negative": ["3", 0],
        "latent_image": ["4", 0]}}
    nodes["6"] = {"class_type": "VAEDecode", "inputs": {"samples": ["5", 0], "vae": ["1", 2]}}
    nodes["7"] = {"class_type": "SaveImage", "inputs": {"images": ["6", 0], "filename_prefix": prefix}}
    return nodes


def cn(depth=0.0, normal=0.0, canny=0.0):
    out = []
    if depth > 0:
        out.append({"kind": "depth", "strength": depth})
    if normal > 0:
        out.append({"kind": "normal", "strength": normal})
    if canny > 0:
        out.append({"kind": "canny", "strength": canny})
    return out


def job(slug, tag, controls, prompt="concept", render="cine", elev=25, denoise1=0.80,
        pass_mode="cur", detail_scale=1.5, detail_denoise=0.35, stage3=False,
        model=DREAM, lora=None, lora_strength=0.0):
    return {"id": "%s_%s" % (slug, tag), "race": slug, "tag": tag, "elev": elev, "render": render,
            "controls": controls, "denoise1": denoise1, "prompt": prompt, "pass": pass_mode,
            "detail_scale": detail_scale, "detail_denoise": detail_denoise, "stage3": stage3,
            "model": model, "lora": lora, "lora_strength": lora_strength}


def build_jobs(best=None):
    best = best or {}
    bd, bn, bp = best.get("depth", 0.8), best.get("normal", 0.0), best.get("prompt", "concept")
    jobs = []
    # 1) CN-стек: depth vs depth+normal (cinema-свет, concept-промпт)
    for slug in ("humans", "coastal", "crystallites"):
        jobs.append(job(slug, "d08", cn(depth=0.8)))
    jobs += [job("humans", "n06", cn(depth=0.8, normal=0.6)),
             job("humans", "n08", cn(depth=0.8, normal=0.8)),
             job("humans", "n10", cn(depth=0.8, normal=1.0)),
             job("coastal", "n08", cn(depth=0.8, normal=0.8)),
             job("crystallites", "n08", cn(depth=0.8, normal=0.8))]
    # 2) свет: flat vs cine при dn0.80 и dn0.55 (humans_d08 = cine/dn0.80 — уже есть)
    jobs += [job("humans", "flat08", cn(depth=0.8), render="flat"),
             job("humans", "flat055", cn(depth=0.8), render="flat", denoise1=0.55),
             job("humans", "cine055", cn(depth=0.8), denoise1=0.55)]
    # 3) промпты: clean / weathered (concept = humans_d08)
    jobs += [job("humans", "clean", cn(depth=0.8), prompt="clean"),
             job("humans", "weath", cn(depth=0.8), prompt="weathered"),
             job("coastal", "clean", cn(depth=0.8), prompt="clean"),
             job("crystallites", "clean", cn(depth=0.8), prompt="clean")]
    # 4) проходность: detail-проход vs текущий (0.45 + Hi-Res 0.40)
    # scale 2.0 (3072px) на 16 ГБ зависает — снят; контроль 2048 (1.33x) против Hi-Res.
    jobs += [job("humans", "det15", cn(depth=0.8), pass_mode="detail", detail_scale=1.5,
                 detail_denoise=0.35),
             job("humans", "det13", cn(depth=0.8), pass_mode="detail", detail_scale=1.333,
                 detail_denoise=0.35),
             job("humans", "d08_u", cn(depth=0.8), stage3=True)]
    # 5) финалисты (лучшая комбинация + Hi-Res) — задаются --best
    bcnt = cn(depth=bd, normal=bn)
    for slug, tag in (("humans", "final"), ("coastal", "final"), ("crystallites", "final")):
        jobs.append(job(slug, tag, bcnt, prompt=bp, stage3=True))
    jobs += [job("humans", "final_flat", bcnt, prompt=bp, render="flat", stage3=True),
             job("humans", "e40", bcnt, prompt=bp, elev=40, stage3=True)]
    return jobs


def _render_names(j, slug):
    base = CINE if j["render"] == "cine" else FLAT
    src = os.path.join(base, "%s_e%02d.png" % (slug, int(j["elev"])))
    dep = os.path.join(base, "%s_e%02d_depth.png" % (slug, int(j["elev"])))
    nrm = os.path.join(base, "%s_e%02d_normal.png" % (slug, int(j["elev"])))
    n_src = "v4_%s_%s.png" % (slug, j["render"])
    n_dep = "v4_%s_%s_dep.png" % (slug, j["render"])
    n_nrm = "v4_%s_%s_nrm.png" % (slug, j["render"])
    _copy(src, n_src)
    _copy(dep, n_dep)
    _copy(nrm, n_nrm)
    return n_src, n_dep, n_nrm


def run_job(j):
    slug, jid = j["race"], j["id"]
    seed = RACES[slug]["seed"]
    src_name, dep_name, nrm_name = _render_names(j, slug)
    p1 = pos_base(slug, j["prompt"])
    p2 = p1 + ", maximal detail, dense surface detail, masterpiece"
    neg = neg_for(j["prompt"])
    raw = {1: os.path.join(OUT, "v4_%s_1.png" % jid),
           2: os.path.join(OUT, "v4_%s_2.png" % jid),
           3: os.path.join(OUT, "v4_%s_3.png" % jid)}
    stage = {}
    for c in j["controls"]:
        if c["kind"] == "depth":
            c["image"] = dep_name
        elif c["kind"] == "normal":
            c["image"] = nrm_name
    t = {}
    # --- этап 1: форма через CN-стек, высокий denoise ---
    if not os.path.exists(raw[1]):
        t0 = time.time()
        wf = stage_cn_stack(src_name, j["controls"], p1, neg, seed, j["model"], j["lora"],
                            j["lora_strength"], denoise=j["denoise1"], cfg=6.0, steps=30,
                            prefix="v4_s1")
        download(wait(post(wf)), raw[1])
        t["t1"] = round(time.time() - t0, 1)
    r1 = "v4_%s_r1.png" % jid
    _copy(raw[1], r1)
    stage["stage1_denoise"] = j["denoise1"]
    stage["stage1_prompt"] = p1
    if j["pass"] == "detail":
        # --- база (denoise 0.80) -> отдельный проход детализации (апскейл + img2img) ---
        stage["mode"] = "detail"
        stage["detail_scale"] = j["detail_scale"]
        stage["detail_denoise"] = j["detail_denoise"]
        if not os.path.exists(raw[3]):
            t0 = time.time()
            wf = stage_detail(r1, p2, neg, seed, j["model"], j["lora"], j["lora_strength"],
                              j["detail_scale"], denoise=j["detail_denoise"], cfg=6.0, steps=30,
                              prefix="v4_sd")
            download(wait(post(wf)), raw[3])
            t["t3"] = round(time.time() - t0, 1)
        final = raw[3]
    else:
        # --- этап 2: текстура (img2img 0.45) ---
        stage["mode"] = "cur"
        stage["denoise_2"] = 0.45
        if not os.path.exists(raw[2]):
            t0 = time.time()
            wf = stage_img2img(r1, p2, neg, seed, j["model"], j["lora"], j["lora_strength"],
                               denoise=0.45, cfg=6.5, prefix="v4_s2")
            download(wait(post(wf)), raw[2])
            t["t2"] = round(time.time() - t0, 1)
        if j["stage3"]:
            # --- этап 3: Hi-Res (4x-UltraSharp -> 2048 -> img2img 0.40) ---
            r2 = "v4_%s_r2.png" % jid
            _copy(raw[2], r2)
            stage["denoise_3"] = 0.40
            stage["upscaler"] = UPSCALER
            stage["scale3"] = 2048
            if not os.path.exists(raw[3]):
                t0 = time.time()
                wf = stage_upscale(r2, p2, neg, seed, j["model"], j["lora"], j["lora_strength"],
                                   denoise=0.40, cfg=6.0, prefix="v4_s3")
                download(wait(post(wf)), raw[3])
                t["t3"] = round(time.time() - t0, 1)
            final = raw[3]
        else:
            final = raw[2]
    # --- постобработка (--keep-parts) ---
    cand = os.path.join(OUT, "v4_%s_200.png" % jid)
    t0 = time.time()
    subprocess.run([PY, os.path.join(ROOT, "tools", "process_ship.py"), final, cand, "--keep-parts"],
                   check=True)
    t["t4"] = round(time.time() - t0, 1)
    return {
        "id": jid, "race": slug, "archetype": RACES[slug]["archetype"], "tag": j["tag"],
        "elev": j["elev"], "render": j["render"], "render_dir": CINE if j["render"] == "cine" else FLAT,
        "model": j["model"], "lora": j["lora"], "lora_strength": j["lora_strength"], "seed": seed,
        "source": os.path.join(CINE if j["render"] == "cine" else FLAT,
                               "%s_e%02d.png" % (slug, int(j["elev"]))),
        "controls": [{"kind": c["kind"], "strength": c["strength"]} for c in j["controls"]],
        "prompt_key": j["prompt"], "params": stage, "keep_parts": True,
        "raw1": raw[1], "raw2": raw[2] if j["pass"] == "cur" else None,
        "raw3": raw[3] if (j["stage3"] or j["pass"] == "detail") else None,
        "final_raw": final, "candidate": cand, "timings": t,
    }


def main():
    ap = argparse.ArgumentParser(description="Спайк 5: CN normal, свет, промпты, проходность")
    ap.add_argument("--group", default="", help="cn,light,prompt,pass,final (пусто = все)")
    ap.add_argument("--jobs", default="")
    ap.add_argument("--limit", type=int, default=0)
    ap.add_argument("--best-depth", type=float, default=0.8)
    ap.add_argument("--best-normal", type=float, default=0.0)
    ap.add_argument("--best-prompt", default="concept")
    args = ap.parse_args()
    os.makedirs(OUT, exist_ok=True)
    refresh_models()
    best = {"depth": args.best_depth, "normal": args.best_normal, "prompt": args.best_prompt}
    jobs = build_jobs(best)
    groups = {
        "cn": {"humans_d08", "humans_n06", "humans_n08", "humans_n10", "coastal_d08",
               "coastal_n08", "crystallites_d08", "crystallites_n08"},
        "light": {"humans_flat08", "humans_flat055", "humans_cine055"},
        "prompt": {"humans_clean", "humans_weath", "coastal_clean", "crystallites_clean"},
        "pass": {"humans_det15", "humans_det13", "humans_d08_u"},
        "final": {"humans_final", "coastal_final", "crystallites_final", "humans_final_flat",
                  "humans_e40"},
    }
    if args.group:
        want = set()
        for g in args.group.split(","):
            want |= groups.get(g, set())
        jobs = [j for j in jobs if j["id"] in want]
    if args.jobs:
        w = set(args.jobs.split(","))
        jobs = [j for j in jobs if j["id"] in w]
    if args.limit:
        jobs = jobs[:args.limit]
    meta_path = os.path.join(OUT, "geo_meta.json")
    meta = {}
    if os.path.exists(meta_path):
        with open(meta_path, encoding="utf-8") as f:
            meta = json.load(f)
    entries = meta.get("entries_v4", [])
    done = {e["id"] for e in entries}
    for j in jobs:
        cand = os.path.join(OUT, "v4_%s_200.png" % j["id"])
        if j["id"] in done and os.path.exists(cand):
            print("SKIP %s" % j["id"])
            continue
        t0 = time.time()
        rec = run_job(j)
        rec["timings"]["total"] = round(time.time() - t0, 1)
        entries = [e for e in entries if e["id"] != rec["id"]] + [rec]
        meta["entries_v4"] = entries
        with open(meta_path, "w", encoding="utf-8") as f:
            json.dump(meta, f, ensure_ascii=False, indent=1)
        print("DONE %s: %s" % (rec["id"], rec["timings"]))
    print("meta written: %s" % meta_path)


if __name__ == "__main__":
    main()
