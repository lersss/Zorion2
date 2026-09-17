# -*- coding: utf-8 -*-
# race_gen.py — генератор АВАТАРОВ РАС с рандомом по осям (неантропоморфные).
# По одному семейству за раз: python race_gen.py F2 [count]
# Конфиг семейства задаёт: форма-архетипы, материалы, свечения, фон, негатив.
# Голых/людей НЕТ. В пул кладётся прозрачный PNG (rembg) + показ на фоне.
import sys, os, json, random, time, urllib.request

COMFY = "http://127.0.0.1:8188"
POOL = r"C:\Zorion2\ai_drafts\races_pool"
STATUS = os.path.join(POOL, "status.json")
STOP_FLAG = os.path.join(POOL, "stop.flag")
os.makedirs(POOL, exist_ok=True)

from race_forms import forms_for, ANTHRO_FORMS  # большой словарь форм (tools/race_forms.py)

# ---------- КОНФИГИ СЕМЕЙСТВ ----------
# Структура: FAMILIES[семейство] = { "name", "races": [ (имя_расы, формы[], материалы[], свечения[]), ... ] }
FAMILIES = {
    "F2": {
        "name": "Крио-аммиачные",
        "races": [
            ("5", "5 Аммиачники",  # жидкий NH3, моря
             ["a single molten ammonia droplet form", "a smooth liquid ammonia pool rising upward",
              "a rounded ammonia liquid mass", "a teardrop of liquid ammonia"],
             ["liquid ammonia, translucent pale blue", "liquid ammonia with frost rim"],
             ["soft cyan glow inside", "faint blue inner light", "pale white sheen"]),
            ("6", "6 Крио-лесные",  # крио-флора, NH3-моря
             ["a branching ice crystal forest", "a coral-like ice growth",
              "a frost crystal with many branches", "an ice fern structure"],
             ["frosted ammonia ice", "crystalline methane ice", "pale blue ice"],
             ["glowing pale blue tips", "soft cyan glow between branches", "white inner glow"]),
            ("7", "7 Ледяные пастухи",  # CO2-льды, твёрдая фаза
             ["a flat ice plateau with ridges", "a frozen carbon-dioxide dune",
              "a cracked ice slab formation", "a layered frost plain"],
             ["carbon-dioxide frost, white-grey", "frozen CO2 ice, pale blue-grey", "dry ice crust"],
             ["faint white glow from cracks", "soft blue glow in fissures", "no glow, matte frost"]),
            ("8", "8 Туманники",  # NH3-туманы, высокое давление
             ["a swirling ammonia fog vortex", "a dense cloud of ammonia mist",
              "a towering fog column", "a misty spiral"],
             ["ammonia fog, translucent grey-blue", "dense ammonia mist, pale"],
             ["soft cyan light inside the fog", "glowing core in the mist", "faint teal glow"]),
            ("9", "9 Крио-рои",  # коллективные, роевая охота
             ["a swarm of small ice shards", "a cluster of frost fragments",
              "a roiling mass of ice crystals", "a cloud of frozen needles"],
             ["small ice shards, pale blue", "frost fragments, white-blue", "methane ice splinters"],
             ["each shard glowing faintly", "soft blue glow in the swarm", "pale shimmer"]),
            ("48", "48 Крио-небесные",  # атмосферы холодных гигантов
             ["a floating atmospheric crystal", "a levitating frost prism",
              "a hovering ice gem cluster", "a drifting crystalline form"],
             ["translucent methane ice, pale", "atmospheric frost crystal, blue-white", "glassy ice"],
             ["glowing pale core", "soft cyan inner light", "white luminosity"]),
        ],
        "extra": [
            "no face, no eyes, no mouth",
            "no face, no eyes, no mouth, no human features",
            "no face, no eyes, no mouth, asymmetric",
        ],
        "anchor": [
            "anchored by a solid ice base extending to the bottom edge of the frame",
            "anchored by a mineral base extending down, NOT floating",
        ],
        "scene": "on flat dark grey background, on flat dark blue background",
        "neg": ("text, watermark, blurry, low quality, deformed, ugly, duplicate, 3D render, "
                "cartoon, anime, human, person, face, eyes, nose, mouth, ears, chin, shoulders, "
                "head, portrait, symmetrical, human anatomy, building, structure, machine, pipe, "
                "chimney, castle, architecture, organ, flesh, meat, ribs, corrugation, wavy, "
                "fish, animal, cave, scenery, logo, icon, pattern, seamless, full frame, "
                "creature, lifeform, being, monster, creature, crystal, crystalline, shard, "
                "spike, prism, gem, mineral, gemstone, naked, nude, blue, green, magenta, red, "
                "cropped, cut off, floating, no base"),
    },
}

# ---------- ГЕНЕРАЦИЯ ----------


def build_prompt_wide(rng, fam, race_idx, fam_id, anthro=False):
    """Кандидаты эталона — ШИРОКИЙ поиск ПО ВСЕМУ СЕМЕЙСТВУ.
    race_idx=None: раса выбирается случайно (материал/свечение от неё).
    anthro=False: абстрактные минеральные структуры.
    anthro=True:  гуманоиды ИЗ МАТЕРИАЛА расы."""
    if race_idx is None:
        race_idx = rng.randrange(len(fam["races"]))
    race_id, race_name, _forms, materials, glows = fam["races"][race_idx]
    mat = rng.choice(materials)
    glow = rng.choice(glows)
    scene = rng.choice(fam["scene"].split(", "))
    if anthro:
        form = rng.choice(ANTHRO_FORMS)
        return (f"realistic portrait of an alien humanoid race, FRONT VIEW, face looking directly at viewer, "
                f"{form} made of {mat}, {glow}, natural skin texture, head and shoulders, "
                f"torso extending down below the frame, anchored, "
                f"centered, {scene}, game avatar, no text, no watermark"), (race_id, race_name)
    form = rng.choice(forms_for(fam_id))
    return (f"dramatic cinematic concept art of an ABSTRACT OBJECT, FRONT VIEW, "
            f"made of {mat}, {form}, {glow}, "
            f"no face, no eyes, no mouth, no human features, asymmetric, "
            f"anchored by a solid base extending to the bottom edge of the frame, "
            f"centered, {scene}, masterpiece, game avatar, no text, no watermark"), (race_id, race_name)

def build_prompt(rng, fam, race_idx=None):
    if race_idx is None:
        race_idx = rng.randrange(len(fam["races"]))
    race_id, race_name, forms, materials, glows = fam["races"][race_idx]
    mat = rng.choice(materials)
    glow = rng.choice(glows)
    form = rng.choice(forms)
    extra = rng.choice(fam["extra"])
    anchor = rng.choice(fam["anchor"])
    scene = rng.choice(fam["scene"].split(", "))
    return (f"dramatic cinematic concept art of a non-organic alien creature, FRONT VIEW, "
            f"made of {mat}, {form}, {glow}, {extra}, {anchor}, centered, {scene}, "
            f"masterpiece, game avatar, no text, no watermark"), (race_id, race_name)


STEPS = 28  # Juggernaut XL: 28 шагов достаточно, 40 было медленнее


def submit(prompt, neg, seed, out_path):
    wf = {
        "1": {"class_type": "CheckpointLoaderSimple", "inputs": {"ckpt_name": "juggernaut-xl-v9.safetensors"}},
        "2": {"class_type": "CLIPTextEncode", "inputs": {"text": prompt, "clip": ["1", 1]}},
        "3": {"class_type": "CLIPTextEncode", "inputs": {"text": neg, "clip": ["1", 1]}},
        "4": {"class_type": "EmptyLatentImage", "inputs": {"width": 1024, "height": 1024, "batch_size": 1}},
        "5": {"class_type": "KSampler", "inputs": {
            "seed": seed, "steps": STEPS, "cfg": 7.0, "sampler_name": "dpmpp_2m",
            "scheduler": "karras", "denoise": 1.0,
            "model": ["1", 0], "positive": ["2", 0], "negative": ["3", 0], "latent_image": ["4", 0]}},
        "6": {"class_type": "VAEDecode", "inputs": {"samples": ["5", 0], "vae": ["1", 2]}},
        "7": {"class_type": "SaveImage", "inputs": {"images": ["6", 0], "filename_prefix": "race_pool"}},
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


def submit_ref(prompt, neg, seed, ref_path, out_path, ref_name="race_ref.png", denoise=0.35):
    """img2img вариация ОТ ЭТАЛОНА (ref.png): форма/свет меняются, база сохраняется."""
    import shutil
    comfy_in = r"C:\ComfyUI\input"
    shutil.copy2(ref_path, os.path.join(comfy_in, ref_name))
    wf = {
        "1": {"class_type": "CheckpointLoaderSimple", "inputs": {"ckpt_name": "juggernaut-xl-v9.safetensors"}},
        "2": {"class_type": "CLIPTextEncode", "inputs": {"text": prompt, "clip": ["1", 1]}},
        "3": {"class_type": "CLIPTextEncode", "inputs": {"text": neg, "clip": ["1", 1]}},
        "8": {"class_type": "LoadImage", "inputs": {"image": ref_name}},
        "4": {"class_type": "VAEEncode", "inputs": {"pixels": ["8", 0], "vae": ["1", 2]}},
        "5": {"class_type": "KSampler", "inputs": {
            "seed": seed, "steps": STEPS, "cfg": 7.5, "sampler_name": "dpmpp_2m",
            "scheduler": "karras", "denoise": denoise,
            "model": ["1", 0], "positive": ["2", 0], "negative": ["3", 0], "latent_image": ["4", 0]}},
        "6": {"class_type": "VAEDecode", "inputs": {"samples": ["5", 0], "vae": ["1", 2]}},
        "7": {"class_type": "SaveImage", "inputs": {"images": ["6", 0], "filename_prefix": "race_pool"}},
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
    from rembg import remove, new_session
    from PIL import Image
    img = Image.open(in_path).convert("RGB")
    sess = new_session("isnet-general-use")
    res = remove(img, session=sess)
    res.save(out_path)


def write_status(d):
    with open(STATUS, "w", encoding="utf-8") as f:
        json.dump(d, f, ensure_ascii=False)


def main():
    fam_id = sys.argv[1] if len(sys.argv) > 1 else "F2"
    count = int(sys.argv[2]) if len(sys.argv) > 2 else 8
    fam = FAMILIES[fam_id]
    rng = random.Random()

    # РЕЖИМ ВАРИАЦИЙ ОТ ЭТАЛОНА РАСЫ: race_gen.py F2 ref 5 8 [denoise]
    if len(sys.argv) > 2 and sys.argv[2] == "ref":
        race_id = sys.argv[3] if len(sys.argv) > 3 else "5"
        count = int(sys.argv[4]) if len(sys.argv) > 4 else 8
        denoise = float(sys.argv[5]) if len(sys.argv) > 5 else 0.35
        race_idx = None
        for k, r in enumerate(fam["races"]):
            if r[0] == race_id:
                race_idx = k
                break
        if race_idx is None:
            print(f"Нет расы {race_id} в {fam_id}")
            return
        REF = os.path.join(POOL, f"ref_{fam_id}_r{race_id}.png")
        if not os.path.exists(REF):
            write_status({"running": False, "done": 0, "total": 0, "current": f"НЕТ эталона расы {race_id}"})
            print(f"НЕТ эталона ref_{fam_id}_r{race_id}.png — сначала сделай эталон")
            return
        # очистить пул r*.png и meta.json (ref_* и ref_cands/ не трогаем) —
        # приём во время генерации не должен попадать в старую мету
        for f in os.listdir(POOL):
            if f.startswith("r") and not f.startswith("ref_") and f.endswith(".png"):
                os.remove(os.path.join(POOL, f))
        mp = os.path.join(POOL, "meta.json")
        if os.path.exists(mp):
            os.remove(mp)
        write_status({"running": True, "done": 0, "total": count, "current": f"вариации расы {race_id}..."})
        for i in range(count):
            seed = rng.randint(1, 999999999)
            prompt, (rid, rname) = build_prompt(rng, fam, race_idx=race_idx)
            out = os.path.join(POOL, f"r{i + 1:02d}.png")
            write_status({"running": True, "done": i, "total": count, "current": f"вариант {rname}"})
            print(f"=== ref-вариация {i + 1}/{count} [{fam_id}] раса {rname} seed={seed} ===")
            if submit_ref(prompt, fam["neg"], seed, REF, out, f"ref_{fam_id}_r{race_id}.png", denoise):
                remove_bg(out, out)
                meta = {"file": os.path.basename(out), "seed": seed, "family": fam_id,
                        "race": rname, "race_id": rid, "prompt": prompt, "ref": True}
                mp = os.path.join(POOL, "meta.json")
                all_meta = []
                if os.path.exists(mp):
                    try:
                        all_meta = json.load(open(mp, encoding="utf-8"))
                    except Exception:
                        pass
                all_meta.append(meta)
                with open(mp, "w", encoding="utf-8") as f:
                    json.dump(all_meta, f, ensure_ascii=False, indent=1)
                print("   OK", out)
            else:
                print("   FAIL")
        write_status({"running": False, "done": count, "total": count, "current": "готово"})
        print("=== ref-вариации done:", count, "===")
        return

    # РЕЖИМ КАНДИДАТОВ ЭТАЛОНА: race_gen.py F2 3 refcand [race_id]
    # Кандидаты — ШИРОКИЙ поиск: формы ВСЕХ рас семейства (если раса не задана),
    # чтобы было из чего выбрать эталон. Каждый кандидат метится своей расой.
    if len(sys.argv) > 3 and sys.argv[3] == "refcand":
        # генерация ПО ВСЕМУ СЕМЕЙСТВУ (раса выбирается случайно в build_prompt_wide)
        # anthro-флаг идёт в argv[4] (студия шлёт [fam, n, "refcand", "anthro"])
        anthro = len(sys.argv) > 4 and sys.argv[4] == "anthro"
        race_idx = None
        refdir = os.path.join(POOL, "ref_cands")
        os.makedirs(refdir, exist_ok=True)
        cand_meta = {}
        if os.path.exists(os.path.join(refdir, "meta.json")):
            try:
                cand_meta = json.load(open(os.path.join(refdir, "meta.json"), encoding="utf-8"))
            except Exception:
                pass
        write_status({"running": True, "done": 0, "total": count, "current": "кандидаты эталона (широкий поиск)..."})
        from concurrent.futures import ThreadPoolExecutor
        jobs = []
        for i in range(count):
            seed = rng.randint(1, 999999999)
            prompt, (rid, rname) = build_prompt_wide(rng, fam, race_idx, fam_id, anthro)
            raw = os.path.join(POOL, f"_raw_ref_{i + 1:02d}.png")
            out = os.path.join(refdir, f"c{i + 1:02d}.png")
            jobs.append((i, seed, prompt, rid, rname, raw, out))
        import threading
        stop_lock = threading.Event()
        def workc(j):
            if os.path.exists(STOP_FLAG):
                stop_lock.set()
                return None
            i, seed, prompt, rid, rname, raw, out = j
            return (i, seed, prompt, rid, rname, raw, out, submit(prompt, fam["neg"], seed, raw))
        done = 0
        with ThreadPoolExecutor(max_workers=2) as ex:
            for res in ex.map(workc, jobs):
                if res is None:
                    break
                i, seed, prompt, rid, rname, raw, out, ok = res
                print(f"=== кандидат {i + 1}/{count} [{fam_id}] {rname} seed={seed} ===")
                if ok:
                    remove_bg(raw, out)
                    os.remove(raw)
                    cand_meta[f"c{i + 1:02d}.png"] = {"race_id": rid, "race": rname, "seed": seed, "prompt": prompt}
                    with open(os.path.join(refdir, "meta.json"), "w", encoding="utf-8") as f:
                        json.dump(cand_meta, f, ensure_ascii=False, indent=1)
                    print("   OK", out)
                else:
                    print("   FAIL")
                done += 1
                write_status({"running": True, "done": done, "total": count, "current": f"кандидат {rname}"})
        if os.path.exists(STOP_FLAG):
            os.remove(STOP_FLAG)
        write_status({"running": False, "done": done, "total": count, "current": "остановлено" if done < count else "готово"})
        print("=== кандидаты done:", count, "===")
        return

    meta = []
    write_status({"running": True, "done": 0, "total": count, "current": "старт..."})
    jobs = []
    for i in range(count):
        seed = rng.randint(1, 999999999)
        prompt, (rid, rname) = build_prompt(rng, fam)
        raw = os.path.join(POOL, f"_raw_{i + 1:02d}.png")
        out = os.path.join(POOL, f"r{i + 1:02d}.png")
        jobs.append((i, seed, prompt, rid, rname, raw, out))
    from concurrent.futures import ThreadPoolExecutor
    done = 0
    import threading
    stop_lock = threading.Event()
    def work(j):
        if os.path.exists(STOP_FLAG):
            stop_lock.set()
            return None
        i, seed, prompt, rid, rname, raw, out = j
        ok = submit(prompt, fam["neg"], seed, raw)
        return (i, seed, prompt, rid, rname, raw, out, ok)
    with ThreadPoolExecutor(max_workers=2) as ex:
        for res in ex.map(work, jobs):
            if res is None:
                break
            i, seed, prompt, rid, rname, raw, out, ok = res
            print(f"=== {i + 1}/{count} [{fam_id}] {rname} seed={seed} ===")
            if ok:
                remove_bg(raw, out)
                os.remove(raw)
                meta.append({"file": os.path.basename(out), "seed": seed, "family": fam_id, "race": rname, "race_id": rid, "prompt": prompt})
                print("   OK", out)
            else:
                print("   FAIL")
            done += 1
            write_status({"running": True, "done": done, "total": count, "current": f"{rname}"})
    if os.path.exists(STOP_FLAG):
        os.remove(STOP_FLAG)
    write_status({"running": False, "done": done, "total": count, "current": "остановлено" if done < count else "готово"})
    with open(os.path.join(POOL, "meta.json"), "w", encoding="utf-8") as f:
        json.dump(meta, f, ensure_ascii=False, indent=1)
    print("=== done:", count, "===")


if __name__ == "__main__":
    main()