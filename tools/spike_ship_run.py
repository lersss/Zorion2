# -*- coding: utf-8 -*-
# СПАЙК (§12.1): прогон силуэтов через ТЕКУЩИЙ конвейер напрямую по ComfyUI HTTP
# API (Go и арт-студия не затрагиваются). Этап 1 (ControlNet Canny + img2img) ->
# этап 2 (img2img) -> этап 3 (4x-UltraSharp -> 2048 -> img2img) -> process_ship.py.
# Параметры — спека 2026-09-21 §6.1, модель dreamshaper-xl-v1.
import json
import os
import subprocess
import time
import urllib.parse
import urllib.request

COMFY = "http://127.0.0.1:8188"
INPUT_DIR = r"C:\ComfyUI\input"
ROOT = r"C:\Zorion2"
OUT = os.path.join(ROOT, "ai_drafts", "spike_ships")
SIL_DIR = os.path.join(OUT, "silhouettes")
PY = r"C:\ComfyUI\venv\Scripts\python.exe"
MODEL = "dreamshaper-xl-v1.safetensors"

SHIP_NEG = (
    "text, watermark, blurry, low quality, deformed, ugly, duplicate, cartoon, anime, "
    "human, person, face, eyes, nose, mouth, ears, chin, head, portrait, human anatomy, "
    "limbs, hands, body, flesh, meat, organ, naked, nude, cropped, cut off, floating, "
    "3D perspective view, three-quarter view, side view, flat color, plain surface, "
    "no detail, featureless, toy, plastic, cell shading, sticker, logo, background "
    "scenery, stars, planet, nebula, ground, cast shadow, multiple ships, symmetrical, "
    "blurry edges, low contrast"
)

RACES = {
    "humans": {
        "seed": 100001,
        "archetype": "manta_wing",
        "prompt1": ("sleek wing-shaped aerospace craft, stabilizer fins, tail fins, "
                    "industrial design, hard-surface plating, top-down flat view, horizontal, "
                    "nose pointing FORWARD to the right, engine at the LEFT rear, perfectly flat, "
                    "no perspective, game asset, 2D sprite, centered, single ship, on black "
                    "background, readable surface structure, subtle rim light, no text, no watermark"),
        "prompt2": ("paneled white-grey metal hull with ceramic heat shield tiles, riveted seams, "
                    "navigation lights, subtle weathering, light blue cockpit glass, no organic "
                    "shapes, no bioluminescence, entire hull covered with dense surface detail, "
                    "detailed panel lines everywhere, layered plates and seams, maximal detail, "
                    "masterpiece, no text, no watermark"),
    },
    "coastal": {
        "seed": 100002,
        "archetype": "flat_barge",
        "prompt1": ("coral house barge, stilt supports along the sides, sail-like cilia along the "
                    "edges, coral and shell structure, top-down flat view, horizontal, nose pointing "
                    "FORWARD to the right, trailing tendrils at the left rear, perfectly flat, "
                    "no perspective, game asset, 2D sprite, centered, single ship, on black "
                    "background, readable surface structure, subtle rim light, no text, no watermark"),
        "prompt2": ("salt-crusted coral and stone hull, brine-hardened organic plating, pale salt "
                    "glints, warm amber lit windows, turquoise trim, wet reflective sheen, no "
                    "machines, no fire, dense organic surface detail, layered shell plating, "
                    "maximal detail, masterpiece, no text, no watermark"),
    },
    "crystallites": {
        "seed": 100003,
        "archetype": "seed_pod",
        "prompt1": ("crystal cluster capsule, glowing spots on the hull, ice-crystal structure, "
                    "translucent frozen surfaces, top-down flat view, horizontal, nose pointing "
                    "FORWARD to the right, trailing tendrils at the left rear, perfectly flat, "
                    "no perspective, game asset, 2D sprite, centered, single ship, on black "
                    "background, readable surface structure, subtle rim light, no text, no watermark"),
        "prompt2": ("faceted grey-blue silicon crystal druse inside a translucent quartz capsule, "
                    "glowing crystal cores, pale luminescence, dark cooling crust shell, no "
                    "machines, no flames, frosted layered surface detail, fine rime crust, "
                    "maximal detail, masterpiece, no text, no watermark"),
    },
}


def _copy_to_input(src, name):
    with open(src, "rb") as s:
        with open(os.path.join(INPUT_DIR, name), "wb") as d:
            d.write(s.read())


def post(wf):
    data = json.dumps({"prompt": wf}).encode('utf-8')
    req = urllib.request.Request(COMFY + "/prompt", data=data,
                                 headers={"Content-Type": "application/json"})
    return json.load(urllib.request.urlopen(req, timeout=30))["prompt_id"]


def wait(prompt_id, timeout=900):
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
                raise RuntimeError("ComfyUI error: " + json.dumps(st))
            imgs = (e.get("outputs") or {}).get("7", {}).get("images")
            if imgs and (st.get("completed") or st.get("status_str") == "success"):
                return imgs[0]
        time.sleep(2)
    raise TimeoutError("timeout " + prompt_id)


def download(img, path):
    url = "%s/view?filename=%s&subfolder=%s&type=%s" % (
        COMFY, urllib.parse.quote(img["filename"]),
        urllib.parse.quote(img.get("subfolder", "")), img.get("type", "output"))
    data = urllib.request.urlopen(url, timeout=60).read()
    with open(path, "wb") as f:
        f.write(data)
    return len(data)


def stage1(sil_name, prompt, neg, seed):
    return {
        "1": {"class_type": "CheckpointLoaderSimple", "inputs": {"ckpt_name": MODEL}},
        "2": {"class_type": "CLIPTextEncode", "inputs": {"text": prompt, "clip": ["1", 1]}},
        "3": {"class_type": "CLIPTextEncode", "inputs": {"text": neg, "clip": ["1", 1]}},
        "8": {"class_type": "LoadImage", "inputs": {"image": sil_name}},
        "12": {"class_type": "Canny", "inputs": {"image": ["8", 0], "low_threshold": 0.2, "high_threshold": 0.5}},
        "13": {"class_type": "ControlNetLoader", "inputs": {"control_net_name": "controlnet-canny-sdxl-1.0.safetensors"}},
        "14": {"class_type": "ControlNetApplyAdvanced", "inputs": {
            "positive": ["2", 0], "negative": ["3", 0], "control_net": ["13", 0], "image": ["12", 0],
            "strength": 1.30, "start_percent": 0.0, "end_percent": 1.0}},
        "4": {"class_type": "VAEEncode", "inputs": {"pixels": ["8", 0], "vae": ["1", 2]}},
        "5": {"class_type": "KSampler", "inputs": {
            "seed": seed, "steps": 30, "cfg": 6.0, "sampler_name": "dpmpp_2m", "scheduler": "karras",
            "denoise": 0.80, "model": ["1", 0], "positive": ["14", 0], "negative": ["14", 1],
            "latent_image": ["4", 0]}},
        "6": {"class_type": "VAEDecode", "inputs": {"samples": ["5", 0], "vae": ["1", 2]}},
        "7": {"class_type": "SaveImage", "inputs": {"images": ["6", 0], "filename_prefix": "spike_s1"}},
    }


def stage2(img_name, prompt, neg, seed):
    return {
        "1": {"class_type": "CheckpointLoaderSimple", "inputs": {"ckpt_name": MODEL}},
        "2": {"class_type": "CLIPTextEncode", "inputs": {"text": prompt, "clip": ["1", 1]}},
        "3": {"class_type": "CLIPTextEncode", "inputs": {"text": neg, "clip": ["1", 1]}},
        "8": {"class_type": "LoadImage", "inputs": {"image": img_name}},
        "4": {"class_type": "VAEEncode", "inputs": {"pixels": ["8", 0], "vae": ["1", 2]}},
        "5": {"class_type": "KSampler", "inputs": {
            "seed": seed, "steps": 30, "cfg": 6.5, "sampler_name": "dpmpp_2m", "scheduler": "karras",
            "denoise": 0.45, "model": ["1", 0], "positive": ["2", 0], "negative": ["3", 0],
            "latent_image": ["4", 0]}},
        "6": {"class_type": "VAEDecode", "inputs": {"samples": ["5", 0], "vae": ["1", 2]}},
        "7": {"class_type": "SaveImage", "inputs": {"images": ["6", 0], "filename_prefix": "spike_s2"}},
    }


def stage3(img_name, prompt, neg, seed):
    return {
        "1": {"class_type": "CheckpointLoaderSimple", "inputs": {"ckpt_name": MODEL}},
        "2": {"class_type": "CLIPTextEncode", "inputs": {"text": prompt, "clip": ["1", 1]}},
        "3": {"class_type": "CLIPTextEncode", "inputs": {"text": neg, "clip": ["1", 1]}},
        "8": {"class_type": "LoadImage", "inputs": {"image": img_name}},
        "12": {"class_type": "UpscaleModelLoader", "inputs": {"model_name": "4x-UltraSharp.pth"}},
        "13": {"class_type": "ImageUpscaleWithModel", "inputs": {"upscale_model": ["12", 0], "image": ["8", 0]}},
        "14": {"class_type": "ImageScale", "inputs": {"image": ["13", 0], "width": 2048, "height": 2048,
                                                      "upscale_method": "lanczos", "crop": "disabled"}},
        "4": {"class_type": "VAEEncode", "inputs": {"pixels": ["14", 0], "vae": ["1", 2]}},
        "5": {"class_type": "KSampler", "inputs": {
            "seed": seed, "steps": 30, "cfg": 6.0, "sampler_name": "dpmpp_2m", "scheduler": "karras",
            "denoise": 0.40, "model": ["1", 0], "positive": ["2", 0], "negative": ["3", 0],
            "latent_image": ["4", 0]}},
        "6": {"class_type": "VAEDecode", "inputs": {"samples": ["5", 0], "vae": ["1", 2]}},
        "7": {"class_type": "SaveImage", "inputs": {"images": ["6", 0], "filename_prefix": "spike_s3"}},
    }


def main():
    meta = []
    for slug, cfg in RACES.items():
        sil = os.path.join(SIL_DIR, slug + ".png")
        sil_name = "spike_" + slug + ".png"
        with open(sil, "rb") as src:
            with open(os.path.join(INPUT_DIR, sil_name), "wb") as dst:
                dst.write(src.read())
        seed = cfg["seed"]
        rec = {"race": slug, "archetype": cfg["archetype"], "seed": seed, "model": MODEL,
               "params": {"denoise_1": 0.80, "cn_strength": 1.30, "steps_1": 30, "cfg_1": 6.0,
                          "denoise_2": 0.45, "steps_2": 30, "cfg_2": 6.5,
                          "detail_denoise": 0.40, "detail_steps": 30, "detail_cfg": 6.0,
                          "upscaler": "4x-UltraSharp.pth", "scale": 2048},
               "prompt1": cfg["prompt1"], "prompt2": cfg["prompt2"], "timings": {}}
        # этап 1
        t0 = time.time()
        pid = post(stage1(sil_name, cfg["prompt1"], SHIP_NEG, seed))
        img = wait(pid)
        raw1 = os.path.join(OUT, "raw_%s_1.png" % slug)
        download(img, raw1)
        r1_name = "spike_%s_r1.png" % slug
        _copy_to_input(raw1, r1_name)
        rec["timings"]["t1"] = round(time.time() - t0, 1)
        # этап 2
        t0 = time.time()
        pid = post(stage2(r1_name, cfg["prompt2"], SHIP_NEG, seed))
        img2 = wait(pid)
        raw2 = os.path.join(OUT, "raw_%s_2.png" % slug)
        download(img2, raw2)
        r2_name = "spike_%s_r2.png" % slug
        _copy_to_input(raw2, r2_name)
        rec["timings"]["t2"] = round(time.time() - t0, 1)
        # этап 3
        t0 = time.time()
        pid = post(stage3(r2_name, cfg["prompt2"], SHIP_NEG, seed))
        img3 = wait(pid)
        raw3 = os.path.join(OUT, "raw_%s_3.png" % slug)
        download(img3, raw3)
        rec["timings"]["t3"] = round(time.time() - t0, 1)
        # постобработка
        t0 = time.time()
        cand = os.path.join(OUT, "%s_200.png" % slug)
        subprocess.run([PY, os.path.join(ROOT, "tools", "process_ship.py"), raw3, cand], check=True)
        rec["timings"]["t4"] = round(time.time() - t0, 1)
        rec["candidate"] = cand
        meta.append(rec)
        print("DONE %s: %s" % (slug, rec["timings"]))
    with open(os.path.join(OUT, "spike_meta.json"), "w", encoding="utf-8") as f:
        json.dump(meta, f, ensure_ascii=False, indent=1)
    print("meta written")


if __name__ == "__main__":
    main()
