# channel_art_comfy.py — минимальный клиент ComfyUI API (Python, requests).
# Строит workflow ControlNet+img2img с РАЗНЫМИ изображениями контроля и латента.
import os, time, json, shutil, requests

COMFY = "http://127.0.0.1:8188"
INPUT_DIR = r"C:\ComfyUI\input"
MODEL = "juggernaut-xl-v9.safetensors"
CN_CANNY = "controlnet-canny-sdxl-1.0.safetensors"


def stage(path):
    name = os.path.basename(path)
    os.makedirs(INPUT_DIR, exist_ok=True)
    shutil.copyfile(path, os.path.join(INPUT_DIR, name))
    return name


def build(model, pos, neg, ctrl_name, lat_name, steps, cfg, denoise, cn_strength,
          seed, canny_low=0.2, canny_high=0.5):
    return {
        "1": {"class_type": "CheckpointLoaderSimple", "inputs": {"ckpt_name": model}},
        "2": {"class_type": "CLIPTextEncode", "inputs": {"text": pos, "clip": ["1", 1]}},
        "3": {"class_type": "CLIPTextEncode", "inputs": {"text": neg, "clip": ["1", 1]}},
        "8": {"class_type": "LoadImage", "inputs": {"image": ctrl_name}},
        "9": {"class_type": "Canny", "inputs": {"image": ["8", 0], "low_threshold": canny_low, "high_threshold": canny_high}},
        "10": {"class_type": "ControlNetLoader", "inputs": {"control_net_name": CN_CANNY}},
        "11": {"class_type": "ControlNetApply", "inputs": {"conditioning": ["2", 0], "control_net": ["10", 0], "image": ["9", 0], "strength": cn_strength}},
        "12": {"class_type": "LoadImage", "inputs": {"image": lat_name}},
        "4": {"class_type": "VAEEncode", "inputs": {"pixels": ["12", 0], "vae": ["1", 2]}},
        "5": {"class_type": "KSampler", "inputs": {"seed": seed, "steps": steps, "cfg": cfg, "sampler_name": "dpmpp_2m", "scheduler": "karras", "denoise": denoise, "model": ["1", 0], "positive": ["11", 0], "negative": ["3", 0], "latent_image": ["4", 0]}},
        "6": {"class_type": "VAEDecode", "inputs": {"samples": ["5", 0], "vae": ["1", 2]}},
        "7": {"class_type": "SaveImage", "inputs": {"images": ["6", 0], "filename_prefix": "zorion_channel"}},
    }


def generate(pos, neg, control_path, latent_path, out_path, steps=40, cfg=7.0,
             denoise=0.6, cn_strength=1.3, seed=12345, canny_low=0.2, canny_high=0.5):
    ctrl = stage(control_path)
    lat = stage(latent_path)
    wf = build(MODEL, pos, neg, ctrl, lat, steps, cfg, denoise, cn_strength, seed, canny_low, canny_high)
    r = requests.post(f"{COMFY}/prompt", json={"prompt": wf}, timeout=30)
    r.raise_for_status()
    pid = r.json()["prompt_id"]
    print("queued", pid)
    for _ in range(300):
        time.sleep(2)
        h = requests.get(f"{COMFY}/history/{pid}", timeout=10).json()
        e = h.get(pid)
        if e and e.get("status"):
            st = e["status"]
            if st.get("status_str") == "error" or st.get("completed") is False and "error" in str(st):
                raise RuntimeError("ComfyUI error: " + json.dumps(st)[:500])
            if st.get("status_str") == "success" or st.get("completed"):
                imgs = e.get("outputs", {}).get("7", {}).get("images", [])
                if imgs:
                    info = imgs[0]
                    url = f"{COMFY}/view?filename={info['filename']}&subfolder={info.get('subfolder','')}&type={info['type']}"
                    data = requests.get(url, timeout=60).content
                    with open(out_path, "wb") as f:
                        f.write(data)
                    print("saved", out_path, len(data), "bytes")
                    return out_path
                break
    raise RuntimeError("no image / timeout for " + pid)
