# -*- coding: utf-8 -*-
# human_gen.py — генератор людей с рандомом по осям (раса/возраст/одежда/броня/шлем).
# Сыплет в ai_drafts/humans_pool/. Голых НЕТ (правило). Вертикальный кадр 832x1216.
# Использование: python human_gen.py [count=8]
# Оси рандома: пол, тон кожи (раса), возраст, причёска, тип одежды, цвет формы,
# броня/шлем/аксессуары, выражение, сцена.
import sys, os, json, random, time, urllib.request

COMFY = "http://127.0.0.1:8188"
POOL = r"C:\Zorion2\ai_drafts\humans_pool"
STATUS = os.path.join(POOL, "status.json")
os.makedirs(POOL, exist_ok=True)


def write_status(d):
    with open(STATUS, "w", encoding="utf-8") as f:
        json.dump(d, f, ensure_ascii=False)

# ---- ОСИ РАНДОМА ----
GENDER = ["man", "woman"]
SKIN = [
    "light skin", "fair skin", "olive skin", "brown skin", "dark skin", "black skin",
    "pale skin", "tan skin", "deep brown skin", "warm bronze skin",
    # расовые группы — явно
    "African features with dark skin", "African features with brown skin",
    "East Asian features with light skin", "East Asian features with fair skin",
    "South Asian features with brown skin", "South Asian features with dark skin",
    "Middle-Eastern features with olive skin", "Middle-Eastern features with tan skin",
    "Latino features with tan skin", "Latino features with brown skin",
    "Native American features with bronze skin",
    "Polynesian features with tan skin",
    "mixed-race features", "Caucasian features with pale skin",
]
AGE = [
    "young adult", "adult in their 30s", "middle-aged", "older",
    "youthful", "worn weathered face", "veteran",
]
HAIR = [
    "short dark hair", "short blond hair", "shoulder-length brown hair",
    "black hair in a bun", "curly dark hair", "long dark hair tied back",
    "grey hair", "shaved head", "red hair", "braided hair", "short curly hair",
]
BEARD = ["clean shaven", "light stubble", "full beard", "grey beard", "goatee", "moustache"]
CLOTHES = [
    "navy uniform with high collar", "grey jumpsuit with zipper",
    "black tactical uniform with collar", "white uniform with insignia",
    "brown uniform with gold trim", "orange flight suit",
    "dark purple uniform with high collar", "simple grey uniform",
    "blue coveralls", "leather jacket over shirt",
]
ARMOR = [
    "", "", "", "",  # часто без брони
    "with lightweight armor plates", "with armored collar",
    "with shoulder pads", "with tech harness",
]
HELMET = [
    "", "", "", "", "", "",  # часто без шлема
    "wearing a futuristic helmet with transparent visor, face visible",
    "wearing a open space helmet",
    "wearing a pilot helmet",
]
EXPR = [
    "neutral expression", "calm expression", "confident expression",
    "stern expression", "gentle expression", "serious expression",
    "hopeful expression", "worn tired expression",
]
FACIAL = [
    "angular jawline", "round face", "high cheekbones", "narrow face",
    "broad nose", "thin lips", "full lips", "deep-set eyes", "almond-shaped eyes",
    "prominent brow", "soft features", "sharp features", "wide-set eyes",
    "strong nose bridge", "freckles", "dimples", "high forehead", "square jaw",
]
SCENE = ["on flat dark grey background", "on flat dark blue background"]

NEG = ("text, watermark, blurry, low quality, deformed, ugly, duplicate, extra fingers, "
       "3D render, cartoon, anime, gradient, vignette, glow, haze, environment, cockpit, "
       "spaceship, room, background details, naked, nude, shirtless, topless, blue, green, "
       "magenta, red, cropped, cut off, out of frame, side view, profile, "
       "head cut off, top of head cut, cropped head, head cropped")


def build_prompt(rng):
    g = rng.choice(GENDER)
    skin = rng.choice(SKIN)
    age = rng.choice(AGE)
    hair = rng.choice(HAIR)
    beard = rng.choice(BEARD) if g == "man" and rng.random() < 0.6 else ""
    clothes = rng.choice(CLOTHES)
    armor = rng.choice(ARMOR)
    helmet = rng.choice(HELMET)
    expr = rng.choice(EXPR)
    facial = rng.choice(FACIAL) + (", " + rng.choice(FACIAL) if rng.random() < 0.4 else "")
    scene = rng.choice(SCENE)
    beard_txt = f", {beard}" if beard else ""
    prompt = (f"realistic portrait of a human {g}, FRONT VIEW, face looking directly at viewer, "
              f"{skin}, {age}, {hair}{beard_txt}, {facial}, {expr}, wearing {clothes} {armor} {helmet}, "
              f"FULL head visible with empty space above, head NOT cropped, head and upper chest, "
              f"{scene}, game avatar, no text, no watermark")
    return g, prompt


def submit(prompt, neg, seed, out_path):
    wf = {
        "1": {"class_type": "CheckpointLoaderSimple", "inputs": {"ckpt_name": "juggernaut-xl-v9.safetensors"}},
        "2": {"class_type": "CLIPTextEncode", "inputs": {"text": prompt, "clip": ["1", 1]}},
        "3": {"class_type": "CLIPTextEncode", "inputs": {"text": neg, "clip": ["1", 1]}},
        "4": {"class_type": "EmptyLatentImage", "inputs": {"width": 1024, "height": 1024, "batch_size": 1}},
        "5": {"class_type": "KSampler", "inputs": {
            "seed": seed, "steps": 28, "cfg": 7.0, "sampler_name": "dpmpp_2m",
            "scheduler": "karras", "denoise": 1.0,
            "model": ["1", 0], "positive": ["2", 0], "negative": ["3", 0], "latent_image": ["4", 0]}},
        "6": {"class_type": "VAEDecode", "inputs": {"samples": ["5", 0], "vae": ["1", 2]}},
        "7": {"class_type": "SaveImage", "inputs": {"images": ["6", 0], "filename_prefix": "human_pool"}},
    }
    req = urllib.request.Request(COMFY + "/prompt",
                                 data=json.dumps({"prompt": wf}).encode(),
                                 headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=30) as r:
        pid = json.loads(r.read())["prompt_id"]
    for _ in range(180):
        time.sleep(2)
        try:
            with urllib.request.urlopen(COMFY + "/history/" + pid, timeout=5) as r:
                h = json.loads(r.read())
            e = h.get(pid)
            if e and e.get("status", {}).get("status_str") == "success":
                img = e["outputs"]["7"]["images"][0]
                url = f"{COMFY}/view?filename={img['filename']}&subfolder={img['subfolder']}&type={img['type']}"
                urllib.request.urlretrieve(url, out_path)
                return True
            if e and e.get("status", {}).get("status_str") == "error":
                return False
        except Exception:
            pass
    return False


def remove_bg(in_path, out_path):
    """rembg-вырезка фона (isnet). В пул кладём прозрачный PNG."""
    from rembg import remove, new_session
    from PIL import Image
    img = Image.open(in_path).convert("RGB")
    sess = new_session("isnet-general-use")
    res = remove(img, session=sess)
    res.save(out_path)
    return out_path


def main():
    count = int(sys.argv[1]) if len(sys.argv) > 1 else 8
    rng = random.Random()
    meta = []
    write_status({"running": True, "done": 0, "total": count, "current": "старт..."})
    from concurrent.futures import ThreadPoolExecutor
    jobs = []
    for i in range(count):
        seed = rng.randint(1, 999999999)
        gender, prompt = build_prompt(rng)
        raw = os.path.join(POOL, f"_raw_{i + 1:02d}.png")
        sex = "m" if gender == "man" else "f"
        out = os.path.join(POOL, f"h{i + 1:02d}_{sex}.png")
        jobs.append((i, seed, gender, sex, prompt, raw, out))
    def work(j):
        i, seed, gender, sex, prompt, raw, out = j
        return (i, seed, gender, sex, prompt, raw, out, submit(prompt, NEG, seed, raw))
    done = 0
    with ThreadPoolExecutor(max_workers=2) as ex:
        for i, seed, gender, sex, prompt, raw, out, ok in ex.map(work, jobs):
            print(f"=== {i + 1}/{count} [{sex}] seed={seed} ===")
            write_status({"running": True, "done": done, "total": count, "current": f"{sex}: {prompt[:80]}"})
            if ok:
                remove_bg(raw, out)
                os.remove(raw)
                meta.append({"file": os.path.basename(out), "seed": seed, "gender": gender, "prompt": prompt})
                print("   OK", out)
            else:
                print("   FAIL")
            done += 1
    write_status({"running": False, "done": count, "total": count, "current": "готово"})
    with open(os.path.join(POOL, "meta.json"), "w", encoding="utf-8") as f:
        json.dump(meta, f, ensure_ascii=False, indent=1)
    print("=== done:", count, "===")


if __name__ == "__main__":
    main()