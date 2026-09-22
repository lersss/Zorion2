# -*- coding: utf-8 -*-
# СПАЙК 2 (§12 спайка): прогон ОБЪЁМНЫХ рендеров кораблей через ComfyUI HTTP API.
# Варианты: V1 (img2img от рендера + ControlNet Canny от рендера, denoise 0.55),
# V2 (img2img от рендера, denoise 0.45), V3 (базлайн: этап 1 от ПЛОСКОГО силуэта,
# как в спайке 1). Для каждого — этап 2 (img2img 0.45) и этап 3 (4x-UltraSharp →
# 2048 → img2img 0.40), затем tools/process_ship.py (200×200). Модель DreamShaper
# XL v1, A/B на Juggernaut XL v9, A/B LoRA на одном корабле. Go/студия не тронуты.
import argparse
import json
import os
import subprocess
import time
import urllib.parse
import urllib.request

COMFY = "http://127.0.0.1:8188"
INPUT_DIR = r"C:\ComfyUI\input"
ROOT = r"C:\Zorion2"
OUT = os.path.join(ROOT, "ai_drafts", "geometry_ships")
FLAT = os.path.join(OUT, "flat")
PY = r"C:\ComfyUI\venv\Scripts\python.exe"
DREAM = "dreamshaper-xl-v1.safetensors"
JUGG = "juggernaut-xl-v9.safetensors"
LORA = "s3r3n1ty_SDXL_v1_32-000031.safetensors"
CN_CANNY = "controlnet-canny-sdxl-1.0.safetensors"
UPSCALER = "4x-UltraSharp.pth"

SHIP_NEG = (
    "text, watermark, blurry, low quality, deformed, ugly, duplicate, cartoon, anime, "
    "human, person, face, eyes, nose, mouth, ears, chin, head, portrait, human anatomy, "
    "limbs, hands, body, flesh, meat, organ, naked, nude, cropped, cut off, floating, "
    "3D perspective view, three-quarter view, side view, flat color, plain surface, "
    "no detail, featureless, toy, plastic, cell shading, sticker, logo, background "
    "scenery, stars, planet, nebula, ground, cast shadow, multiple ships, symmetrical, "
    "blurry edges, low contrast"
)

TAIL = ("top-down flat view, horizontal, nose pointing FORWARD to the right, perfectly flat, "
        "no perspective, game asset, 2D sprite, centered, single ship, on black background, "
        "readable surface structure, subtle rim light, no text, no watermark")

RACES = {
    "humans": {
        "seed": 100001,
        "archetype": "manta_wing",
        "form": ("sleek wing-shaped aerospace craft, stabilizer fins, tail fins, industrial design, "
                 "hard-surface plating, " + TAIL),
        "tex": ("paneled white-grey metal hull with ceramic heat shield tiles, riveted seams, "
                "navigation lights, subtle weathering, light blue cockpit glass, no organic shapes, "
                "no bioluminescence, entire hull covered with dense surface detail, detailed panel "
                "lines everywhere, layered plates and seams, maximal detail, masterpiece, no text, no watermark"),
    },
    "coastal": {
        "seed": 100002,
        "archetype": "flat_barge",
        "form": ("coral house barge, stilt supports along the sides, sail-like cilia along the edges, "
                 "coral and shell structure, trailing tendrils at the left rear, " + TAIL),
        "tex": ("salt-crusted coral and stone hull, brine-hardened organic plating, pale salt glints, "
                "warm amber lit windows, turquoise trim, wet reflective sheen, no machines, no fire, "
                "dense organic surface detail, layered shell plating, maximal detail, masterpiece, "
                "no text, no watermark"),
    },
    "crystallites": {
        "seed": 100003,
        "archetype": "seed_pod",
        "form": ("crystal cluster capsule, glowing spots on the hull, ice-crystal structure, "
                 "translucent frozen surfaces, trailing tendrils at the left rear, " + TAIL),
        "tex": ("faceted grey-blue silicon crystal druse inside a translucent quartz capsule, glowing "
                "crystal cores, pale luminescence, dark cooling crust shell, no machines, no flames, "
                "frosted layered surface detail, fine rime crust, maximal detail, masterpiece, "
                "no text, no watermark"),
    },
}


def geo_prompt(slug):
    """Промпт этапа 1 для V1/V2: форма и цвет уже в рендере — просим материал/фактуру."""
    return RACES[slug]["tex"] + ", " + TAIL


def build_jobs():
    jobs = []
    for slug in ("humans", "coastal", "crystallites"):
        for variant in ("V1", "V2", "V3"):
            jobs.append({"race": slug, "variant": variant, "camera": "tilt", "model": DREAM,
                         "lora": None, "lora_strength": 0.0, "tag": ""})
    # A/B камеры: строго сверху
    jobs.append({"race": "humans", "variant": "V1", "camera": "top", "model": DREAM,
                 "lora": None, "lora_strength": 0.0, "tag": "camtop"})
    # A/B модели: Juggernaut XL v9 (эталон 21 спрайта)
    jobs.append({"race": "humans", "variant": "V1", "camera": "tilt", "model": JUGG,
                 "lora": None, "lora_strength": 0.0, "tag": "jugg"})
    # A/B LoRA на одном корабле
    for s in (0.5, 0.7):
        jobs.append({"race": "humans", "variant": "V1", "camera": "tilt", "model": DREAM,
                     "lora": LORA, "lora_strength": s, "tag": "lora%02d" % int(s * 10)})
    # ЗОНД сверх ТЗ: тот же ControlNet от рендера, но denoise выше — проверяет,
    # добавляет ли SDXL фактуру, пока CN держит кромки (V1 при 0.55 texture не дал).
    for slug in ("humans", "coastal"):
        jobs.append({"race": slug, "variant": "V4", "camera": "tilt", "model": DREAM,
                     "lora": None, "lora_strength": 0.0, "tag": ""})
    for j in jobs:
        j["id"] = "%s_%s%s" % (j["race"], j["variant"], ("_" + j["tag"]) if j["tag"] else "")
    return jobs


def _base(model, lora, lora_strength):
    nodes = {"1": {"class_type": "CheckpointLoaderSimple", "inputs": {"ckpt_name": model}}}
    if lora:
        nodes["20"] = {"class_type": "LoraLoader", "inputs": {
            "lora_name": lora, "strength_model": lora_strength, "strength_clip": lora_strength,
            "model": ["1", 0], "clip": ["1", 1]}}
        return nodes, ["20", 0], ["20", 1]
    return nodes, ["1", 0], ["1", 1]


def stage_cn(img_name, prompt, neg, seed, model, lora, lora_strength, denoise, cfg, cn_strength, prefix):
    nodes, mp, cp = _base(model, lora, lora_strength)
    nodes["2"] = {"class_type": "CLIPTextEncode", "inputs": {"text": prompt, "clip": cp}}
    nodes["3"] = {"class_type": "CLIPTextEncode", "inputs": {"text": neg, "clip": cp}}
    nodes["8"] = {"class_type": "LoadImage", "inputs": {"image": img_name}}
    nodes["12"] = {"class_type": "Canny", "inputs": {"image": ["8", 0],
                                                      "low_threshold": 0.2, "high_threshold": 0.5}}
    nodes["13"] = {"class_type": "ControlNetLoader", "inputs": {"control_net_name": CN_CANNY}}
    nodes["14"] = {"class_type": "ControlNetApplyAdvanced", "inputs": {
        "positive": ["2", 0], "negative": ["3", 0], "control_net": ["13", 0], "image": ["12", 0],
        "strength": cn_strength, "start_percent": 0.0, "end_percent": 1.0}}
    nodes["4"] = {"class_type": "VAEEncode", "inputs": {"pixels": ["8", 0], "vae": ["1", 2]}}
    nodes["5"] = {"class_type": "KSampler", "inputs": {
        "seed": seed, "steps": 30, "cfg": cfg, "sampler_name": "dpmpp_2m", "scheduler": "karras",
        "denoise": denoise, "model": mp, "positive": ["14", 0], "negative": ["14", 1],
        "latent_image": ["4", 0]}}
    nodes["6"] = {"class_type": "VAEDecode", "inputs": {"samples": ["5", 0], "vae": ["1", 2]}}
    nodes["7"] = {"class_type": "SaveImage", "inputs": {"images": ["6", 0], "filename_prefix": prefix}}
    return nodes


def stage_img2img(img_name, prompt, neg, seed, model, lora, lora_strength, denoise, cfg, prefix):
    nodes, mp, cp = _base(model, lora, lora_strength)
    nodes["2"] = {"class_type": "CLIPTextEncode", "inputs": {"text": prompt, "clip": cp}}
    nodes["3"] = {"class_type": "CLIPTextEncode", "inputs": {"text": neg, "clip": cp}}
    nodes["8"] = {"class_type": "LoadImage", "inputs": {"image": img_name}}
    nodes["4"] = {"class_type": "VAEEncode", "inputs": {"pixels": ["8", 0], "vae": ["1", 2]}}
    nodes["5"] = {"class_type": "KSampler", "inputs": {
        "seed": seed, "steps": 30, "cfg": cfg, "sampler_name": "dpmpp_2m", "scheduler": "karras",
        "denoise": denoise, "model": mp, "positive": ["2", 0], "negative": ["3", 0],
        "latent_image": ["4", 0]}}
    nodes["6"] = {"class_type": "VAEDecode", "inputs": {"samples": ["5", 0], "vae": ["1", 2]}}
    nodes["7"] = {"class_type": "SaveImage", "inputs": {"images": ["6", 0], "filename_prefix": prefix}}
    return nodes


def stage_upscale(img_name, prompt, neg, seed, model, lora, lora_strength, denoise, cfg, prefix):
    nodes, mp, cp = _base(model, lora, lora_strength)
    nodes["2"] = {"class_type": "CLIPTextEncode", "inputs": {"text": prompt, "clip": cp}}
    nodes["3"] = {"class_type": "CLIPTextEncode", "inputs": {"text": neg, "clip": cp}}
    nodes["8"] = {"class_type": "LoadImage", "inputs": {"image": img_name}}
    nodes["12"] = {"class_type": "UpscaleModelLoader", "inputs": {"model_name": UPSCALER}}
    nodes["13"] = {"class_type": "ImageUpscaleWithModel", "inputs": {"upscale_model": ["12", 0], "image": ["8", 0]}}
    nodes["14"] = {"class_type": "ImageScale", "inputs": {"image": ["13", 0], "width": 2048, "height": 2048,
                                                          "upscale_method": "lanczos", "crop": "disabled"}}
    nodes["4"] = {"class_type": "VAEEncode", "inputs": {"pixels": ["14", 0], "vae": ["1", 2]}}
    nodes["5"] = {"class_type": "KSampler", "inputs": {
        "seed": seed, "steps": 30, "cfg": cfg, "sampler_name": "dpmpp_2m", "scheduler": "karras",
        "denoise": denoise, "model": mp, "positive": ["2", 0], "negative": ["3", 0],
        "latent_image": ["4", 0]}}
    nodes["6"] = {"class_type": "VAEDecode", "inputs": {"samples": ["5", 0], "vae": ["1", 2]}}
    nodes["7"] = {"class_type": "SaveImage", "inputs": {"images": ["6", 0], "filename_prefix": prefix}}
    return nodes


def _copy(src, name):
    with open(src, "rb") as s:
        with open(os.path.join(INPUT_DIR, name), "wb") as d:
            d.write(s.read())


def post(wf):
    data = json.dumps({"prompt": wf}).encode('utf-8')
    req = urllib.request.Request(COMFY + "/prompt", data=data, headers={"Content-Type": "application/json"})
    return json.load(urllib.request.urlopen(req, timeout=30))["prompt_id"]


def wait(prompt_id, timeout=1200):
    t0 = time.time()
    while time.time() - t0 < timeout:
        try:
            hist = json.load(urllib.request.urlopen(COMFY + "/history/" + prompt_id, timeout=10))
        except Exception:
            time.sleep(2)
            continue
        e = hist.get(prompt_id)
        if e and e.get("status"):
            st = e["status"]
            if st.get("status_str") == "error":
                raise RuntimeError("ComfyUI error: " + json.dumps(st)[:800])
            imgs = (e.get("outputs") or {}).get("7", {}).get("images")
            if imgs and (st.get("completed") or st.get("status_str") == "success"):
                return imgs[0]
        time.sleep(2)
    raise TimeoutError("timeout " + prompt_id)


def download(img, path):
    url = "%s/view?filename=%s&subfolder=%s&type=%s" % (
        COMFY, urllib.parse.quote(img["filename"]),
        urllib.parse.quote(img.get("subfolder", "")), img.get("type", "output"))
    data = urllib.request.urlopen(url, timeout=120).read()
    with open(path, "wb") as f:
        f.write(data)


def refresh_models():
    """Попросить ComfyUI пересканировать models/loras (иначе новая LoRA не видна)."""
    for method in ("POST", "GET"):
        try:
            req = urllib.request.Request(COMFY + "/refresh",
                                         data=b"" if method == "POST" else None, method=method)
            urllib.request.urlopen(req, timeout=30).read()
            return
        except Exception as e:
            last = e
    print("refresh skipped: %s" % last)


def run_job(job):
    slug, variant = job["race"], job["variant"]
    cfg = RACES[slug]
    seed = cfg["seed"]
    jid = job["id"]
    model = job["model"]
    lora = job["lora"]
    strength = job["lora_strength"]
    t = {}
    # вход: объёмный рендер (V1/V2) или плоский силуэт спайка 1 (V3)
    if variant == "V3":
        src = os.path.join(FLAT, slug + ".png")
    else:
        src = os.path.join(OUT, "%s_%s.png" % (slug, job["camera"]))
    src_name = "geo_%s_%s.png" % (slug, job["camera"])
    _copy(src, src_name)
    tex = cfg["tex"]
    raw = {1: os.path.join(OUT, "%s_1.png" % jid),
           2: os.path.join(OUT, "%s_2.png" % jid),
           3: os.path.join(OUT, "%s_3.png" % jid)}
    # --- этап 1 (форма/материал от рендера) ---
    if not os.path.exists(raw[1]):
        t0 = time.time()
        if variant == "V1":
            wf = stage_cn(src_name, geo_prompt(slug), SHIP_NEG, seed, model, lora, strength,
                          denoise=0.55, cfg=6.0, cn_strength=1.0, prefix="geo_s1")
        elif variant == "V4":
            wf = stage_cn(src_name, geo_prompt(slug), SHIP_NEG, seed, model, lora, strength,
                          denoise=0.72, cfg=6.0, cn_strength=1.0, prefix="geo_s1")
        elif variant == "V2":
            wf = stage_img2img(src_name, geo_prompt(slug), SHIP_NEG, seed, model, lora, strength,
                               denoise=0.45, cfg=6.0, prefix="geo_s1")
        else:  # V3 — как спайк 1
            wf = stage_cn(src_name, cfg["form"], SHIP_NEG, seed, model, lora, strength,
                          denoise=0.80, cfg=6.0, cn_strength=1.30, prefix="geo_s1")
        download(wait(post(wf)), raw[1])
        t["t1"] = round(time.time() - t0, 1)
    r1 = "geo_%s_r1.png" % jid
    _copy(raw[1], r1)
    # --- этап 2 (фактура) ---
    if not os.path.exists(raw[2]):
        t0 = time.time()
        wf = stage_img2img(r1, tex, SHIP_NEG, seed, model, lora, strength,
                           denoise=0.45, cfg=6.5, prefix="geo_s2")
        download(wait(post(wf)), raw[2])
        t["t2"] = round(time.time() - t0, 1)
    r2 = "geo_%s_r2.png" % jid
    _copy(raw[2], r2)
    # --- этап 3 (детализация/Hi-Res) ---
    if not os.path.exists(raw[3]):
        t0 = time.time()
        wf = stage_upscale(r2, tex, SHIP_NEG, seed, model, lora, strength,
                           denoise=0.40, cfg=6.0, prefix="geo_s3")
        download(wait(post(wf)), raw[3])
        t["t3"] = round(time.time() - t0, 1)
    # --- постобработка ---
    cand = os.path.join(OUT, "%s_200.png" % jid)
    t0 = time.time()
    subprocess.run([PY, os.path.join(ROOT, "tools", "process_ship.py"), raw[3], cand], check=True)
    t["t4"] = round(time.time() - t0, 1)
    return {
        "id": jid, "race": slug, "archetype": cfg["archetype"], "variant": variant,
        "camera": job["camera"], "model": model, "lora": lora, "lora_strength": strength,
        "seed": seed, "source": src,
        "params": {"stage1_denoise": {"V1": 0.55, "V2": 0.45, "V3": 0.80, "V4": 0.72}[variant],
                   "cn_strength": {"V1": 1.00, "V2": 0.0, "V3": 1.30, "V4": 1.00}[variant],
                   "denoise_2": 0.45, "denoise_3": 0.40, "steps": 30,
                   "upscaler": UPSCALER, "scale": 2048},
        "raw1": raw[1], "raw2": raw[2], "raw3": raw[3], "candidate": cand,
        "timings": t,
    }


def main():
    ap = argparse.ArgumentParser(description="Спайк 2: прогон объёмных кораблей через ComfyUI")
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
    entries = []
    if os.path.exists(meta_path):
        with open(meta_path, encoding="utf-8") as f:
            entries = json.load(f).get("entries", [])
    done = {e["id"] for e in entries}
    for job in jobs:
        if job["id"] in done and os.path.exists(os.path.join(OUT, "%s_200.png" % job["id"])):
            print("SKIP %s" % job["id"])
            continue
        t0 = time.time()
        rec = run_job(job)
        rec["timings"]["total"] = round(time.time() - t0, 1)
        entries = [e for e in entries if e["id"] != rec["id"]] + [rec]
        with open(meta_path, "w", encoding="utf-8") as f:
            json.dump({"entries": entries}, f, ensure_ascii=False, indent=1)
        print("DONE %s: %s" % (rec["id"], rec["timings"]))
    print("meta written: %s" % meta_path)


if __name__ == "__main__":
    main()
