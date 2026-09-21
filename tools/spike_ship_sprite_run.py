# -*- coding: utf-8 -*-
# СПАЙК 8: концепты спайка 7 -> чистые игровые спрайты 200x200.
# Ветка-победитель спайка 7: чистый txt2img, Juggernaut XL v9, ракурс 3/4.
# Группы:
#   T1 — контроль A/B против спайка 7: те же seed/промпты + якорь ракурса и
#        усиленный негатив (station/ring/...). Сравнение «корабль/станция».
#   R  — расы (humans/coastal/crystallites) по 5 кандидатов, материал из лора
#        docs/gamedesign/races/ships/<slug>.md (поле texture).
#   BG — A/B формулировки «чистого чёрного фона».
# Hi-Res — только финалисты (4x-UltraSharp -> 1536 -> img2img 0.40 -> вырез).
# Постобработка — tools/spike_ship_sprite_cut.py (вырез чёрного фона).
import argparse
import json
import os
import shutil
import subprocess
import sys
import time

from PIL import Image

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from spike_ship_geo_run import (JUGG, PY, ROOT, UPSCALER, _base, download, post,  # noqa: E402
                                refresh_models, wait)
import spike_ship_sprite_cut as cut  # noqa: E402

OUT = os.path.join(ROOT, "ai_drafts", "sprite_ships")
RAW = os.path.join(OUT, "raw")
CUT = os.path.join(ROOT, "tools", "spike_ship_sprite_cut.py")
SIZE = 1024
HIRES = 1536

# Полный корабль в кадре (фикс 2026-09-21): генерация режет корпус у края 1024.
FULL_POS = ("entire spaceship fully visible, complete hull within frame, generous empty margin "
            "around the ship, centered, wide shot, full body")
FULL_NEG = ("cropped, cut off, out of frame, close-up, macro, partial ship, zoomed in, "
            "behind the frame edge")
# Автопроверка кадра: корабль не ближе FRAME_MARGIN px к краю; иначе следующий
# seed (детерминированный перебор, не более FRAME_TRIES попыток).
FRAME_MARGIN = 8
FRAME_TRIES = 6

ANCHOR = "dorsal three-quarter view of a single flying starship, nose pointing right"
BG = "centered, no stars, plain black background"
STYLE = "hard-surface sci-fi game asset, crisp readable silhouette, high contrast edges, dense greeble detail, octane render, artstation, highly detailed, masterpiece"

A_NEG = ("(toy:1.1), plastic, cartoon, cel shading, flat, smooth featureless surface, "
         "airplane, jet, boat, multiple ships, text, watermark, blurry, low detail, "
         "signature, logo, cropped, deformed, ugly, low contrast")
NEG_EXTRA = ("space station, ring, torus, circular disc, front view, symmetrical, planet, "
             "ground, landscape, second ship, out of frame, close-up")

# Спайк 8-бис: жёсткий негатив против морских/авиационных форм (причина «лодочек/самолётиков»).
NEG_HARD = ("boat, ship hull, sailing ship, sail, mast, anchor, water, sea, ocean, harbor, "
            "hull with keel, airplane, aircraft, jet, fighter jet, wings of aircraft, "
            "propeller, runway, airport, atmosphere, sky, clouds, ground")

A_BASE = ("(sci-fi spaceship concept art:1.2), (hard-surface hull:1.15), dense greebles, "
          "panel lines, (engine block:1.1), cinematic lighting, 3/4 top view, centered "
          "single ship, dark background, artstation, octane render, highly detailed")

RACES = {
    "humans": {
        "texture": ("sleek streamlined aerospace starship, wide glass cockpit nose, stabilizer "
                    "wings, paneled white-grey metal hull with ceramic heat shield tiles, riveted "
                    "seams, navigation lights, light blue cockpit glass, cold engine glow, "
                    "brightly lit pale grey hull"),
        "blocked": "tentacle, organic, crystal, pyramid, obelisk, bioluminescent, alien",
        "subjects": ["military frigate", "deep space explorer", "heavy freighter",
                     "sleek interceptor", "scientific cruiser"],
    },
    "coastal": {
        "texture": ("wide flat-bottomed coral-and-stone barge starship, small coral house "
                    "superstructure with warm amber lit windows, stilt supports along the sides, "
                    "woven sail-like cilia, salt-crusted pale hull, turquoise trim, "
                    "brine-hardened organic plating, wet reflective sheen"),
        "blocked": ("pyramid, obelisk, spire, column, prism, lava, sulfur, sulfide, ember, "
                    "ash, magma, obsidian, machine, robot, gear, piston, chrome, mech, pipe, "
                    "engine, rocket, thruster, flame, face, eyes, mouth"),
        "subjects": ["coral barge", "tidal skiff", "salt trader", "shallow-draft houseboat",
                     "reef ferry"],
    },
    "crystallites": {
        "texture": ("thick-walled capsule starship, translucent quartz window at the nose "
                    "revealing a cluster of glowing grey-blue silicon crystals inside, dark "
                    "cooling crust shell with insulating ribs, faceted silicate crystal, "
                    "pale luminescence, pale bright hull"),
        "blocked": ("machine, robot, gear, piston, chrome, mech, automaton, android, golem, "
                    "clockwork, hydraulic, riveted, brass, creature, humanoid, human, face, "
                    "eyes, mouth, head, frost, ice, snow, salt, brine, flame"),
        "subjects": ["crystal seed capsule", "quartz geode carrier", "silicate druse hauler",
                     "molten core vessel", "prism shell craft"],
    },
}
SEED0 = {"humans": 811, "coastal": 821, "crystallites": 831}

# Спайк 8-бис: субъект — ТОЛЬКО космические существительные; расовость задаёт
# материал/палитра/детали (texture из docs/gamedesign/races/ships/<slug>.md), а не
# морское/авиационное слово. Морские слова ушли в NEG_HARD. У coastal из lore-blocked
# убраны engine/rocket/thruster — без дюз звёздный корабль нечитаем (органическая тяга).
RACES2 = {
    "humans": {
        "texture": ("paneled white-grey metal hull with ceramic heat-shield tiles, riveted seams, "
                    "layered armour plates, navigation lights, light blue cockpit glass, cold engine "
                    "glow, industrial greebles, pale grey plating"),
        "blocked": "tentacle, organic, crystal, pyramid, obelisk, bioluminescent, alien",
        "subjects": ["deep-space starship", "stellar cruiser", "interstellar vessel",
                     "deep-space carrier", "stellar dreadnought"],
    },
    "coastal": {
        "texture": ("organic coral-and-salt encrusted hull, translucent tissue panels, soft teal "
                    "bioluminescent glow, warm amber lit windows, brine-hardened pale plating, "
                    "turquoise trim, wet reflective sheen, living grown hull"),
        "blocked": ("pyramid, obelisk, spire, column, prism, lava, sulfur, sulfide, ember, ash, "
                    "magma, obsidian, cinder, pumice, tephra, molten, slag, scorch, smoke, vent, "
                    "smoker, fumarole, caldera, crater, geyser, machine, robot, gear, piston, "
                    "chrome, mech, pipe, flame, face, eyes, mouth"),
        "subjects": ["deep-space starship", "stellar cruiser", "interstellar vessel",
                     "deep-space carrier", "stellar dreadnought"],
    },
    "crystallites": {
        "texture": ("faceted translucent silicate crystal hull, glowing grey-blue crystal core, "
                    "dark cooling crust shell, pale luminescence, frosted quartz panels, "
                    "insulating ribs, pale bright hull"),
        "blocked": ("machine, robot, gear, piston, chrome, mech, automaton, android, golem, "
                    "clockwork, hydraulic, riveted, brass, creature, humanoid, human, face, "
                    "eyes, mouth, head, frost, ice, snow, salt, brine, flame"),
        "subjects": ["deep-space starship", "stellar cruiser", "interstellar vessel",
                     "deep-space carrier", "stellar dreadnought"],
    },
}
SEED0_2 = {"humans": 911, "coastal": 921, "crystallites": 931}

# R3 — coastal v2: из texture убраны «warm amber lit windows» / «translucent tissue»
# (в паре с бирюзой читались как салон машины/субмарины) и добавлена вытянутая
# органическая форма (коралловая обшивка без elongation давала «капсулу/яйцо»).
RACES3 = {
    "coastal": {
        "texture": ("long streamlined organic bioship hull, grown coral-and-shell plating, pale "
                    "salt-white armoured carapace, teal bioluminescent veins running along the "
                    "hull seams, translucent membrane panels, wet organic sheen, trailing "
                    "tendrils at the stern, living grown hull"),
        "blocked": RACES2["coastal"]["blocked"],
        "subjects": RACES2["coastal"]["subjects"],
    },
}
SEED0_3 = {"coastal": 941}
# H — финал людей: имя файла реестра → субъект → seed.
HUMANS_FINAL = [
    ("starship", "deep-space starship", 1201),
    ("cruiser", "stellar cruiser", 1202),
    ("carrier", "deep-space carrier", 1219),
]
NEG_HARD3 = ("car, automobile, vehicle, submarine, torpedo, tank, porthole, egg, jar, "
             "vending machine, crane, truck, bus, train, appliance, helmet")
T1_PLAN = [(101, A_BASE), (202, A_BASE),
           (303, A_BASE + ", (Syd Mead:1.1), (Chris Foss:1.05), Sparth, military frigate"),
           (404, A_BASE + ", (Syd Mead:1.1), (Chris Foss:1.05), Sparth, industrial freighter"),
           (505, A_BASE + ", (Syd Mead:1.1), (Chris Foss:1.05), Sparth, crystal vessel"),
           (606, A_BASE + ", (photorealistic sci-fi film still:1.15), dramatic rim light, volumetric lighting"),
           (707, A_BASE + ", (photorealistic sci-fi film still:1.15), dramatic rim light, volumetric lighting")]
BG_AB = [(816, "on pure black background, isolated, studio lighting"),
         (817, "centered, no stars, plain black background")]


def race_pos(slug, subject, bg=BG):
    return "%s, %s, %s, %s, %s" % (subject, RACES[slug]["texture"], ANCHOR, bg, STYLE)


def race_neg(slug):
    return "%s, %s, %s" % (A_NEG, NEG_EXTRA, RACES[slug]["blocked"])


def race2_pos(slug, subject, bg=BG):
    return "%s, %s, %s, %s, %s" % (subject, RACES2[slug]["texture"], ANCHOR, bg, STYLE)


def race2_neg(slug):
    return "%s, %s, %s, %s" % (A_NEG, NEG_EXTRA, NEG_HARD, RACES2[slug]["blocked"])


def race3_pos(slug, subject, bg=BG):
    return "%s, %s, %s, %s, %s" % (subject, RACES3[slug]["texture"], ANCHOR, bg, STYLE)


def race3_neg(slug):
    return "%s, %s, %s, %s, %s" % (A_NEG, NEG_EXTRA, NEG_HARD, NEG_HARD3, RACES3[slug]["blocked"])


def build_jobs():
    jobs = []
    for seed, text in T1_PLAN:
        jobs.append({"id": "t1_s%d" % seed, "group": "T1", "race": "humans", "seed": seed,
                     "prompt": "%s, %s, %s" % (text, ANCHOR, BG), "neg": "%s, %s" % (A_NEG, NEG_EXTRA),
                     "spike7": "ai_drafts/ai_form/A_jugg_%s_s%d.png" % (
                         "p1" if seed in (101, 202) else "p2" if seed in (303, 404, 505) else "p3", seed)})
    for slug, cfg in RACES.items():
        for i, subject in enumerate(cfg["subjects"]):
            seed = SEED0[slug] + i
            suf = subject.replace(" ", "_")
            jobs.append({"id": "r_%s_%s_s%d" % (slug, suf, seed), "group": "R",
                         "race": slug, "subject": subject, "seed": seed,
                         "prompt": race_pos(slug, subject), "neg": race_neg(slug)})
    for slug, cfg in RACES2.items():
        for i, subject in enumerate(cfg["subjects"]):
            seed = SEED0_2[slug] + i
            suf = subject.replace(" ", "_")
            jobs.append({"id": "r2_%s_%s_s%d" % (slug, suf, seed), "group": "R2",
                         "race": slug, "subject": subject, "seed": seed,
                         "prompt": race2_pos(slug, subject), "neg": race2_neg(slug)})
    for slug, cfg in RACES3.items():
        for i, subject in enumerate(cfg["subjects"]):
            seed = SEED0_3[slug] + i
            suf = subject.replace(" ", "_")
            jobs.append({"id": "r3_%s_%s_s%d" % (slug, suf, seed), "group": "R3",
                         "race": slug, "subject": subject, "seed": seed,
                         "prompt": race3_pos(slug, subject), "neg": race3_neg(slug)})
    for seed, bg in BG_AB:
        jobs.append({"id": "bg_s%d" % seed, "group": "BG", "race": "humans", "seed": seed,
                     "prompt": race_pos("humans", "military frigate", bg=bg),
                     "neg": race_neg("humans")})
    # H — финал расы «люди» (2026-09-21): 3 корабля под имена/ID реестра
    # (race_humans_{starship,cruiser,carrier}), полный кадр + нос вправо.
    for name, subject, seed in HUMANS_FINAL:
        jobs.append({"id": "h_%s_s%d" % (name, seed), "group": "H", "race": "humans",
                     "subject": subject, "seed": seed, "min_elong": 1.3,
                     "prompt": race2_pos("humans", subject), "neg": race2_neg("humans")})
    return jobs


def stage_txt2img(prompt, neg, seed, prefix):
    nodes, mp, cp = _base(JUGG, None, 0.0)
    nodes["2"] = {"class_type": "CLIPTextEncode", "inputs": {"text": prompt, "clip": cp}}
    nodes["3"] = {"class_type": "CLIPTextEncode", "inputs": {"text": neg, "clip": cp}}
    nodes["4"] = {"class_type": "EmptyLatentImage", "inputs": {"width": SIZE, "height": SIZE, "batch_size": 1}}
    nodes["5"] = {"class_type": "KSampler", "inputs": {
        "seed": seed, "steps": 32, "cfg": 6.0, "sampler_name": "dpmpp_2m", "scheduler": "karras",
        "denoise": 1.0, "model": mp, "positive": ["2", 0], "negative": ["3", 0],
        "latent_image": ["4", 0]}}
    nodes["6"] = {"class_type": "VAEDecode", "inputs": {"samples": ["5", 0], "vae": ["1", 2]}}
    nodes["7"] = {"class_type": "SaveImage", "inputs": {"images": ["6", 0], "filename_prefix": prefix}}
    return nodes


def run_job(j):
    t = {}
    prompt = j["prompt"] + ", " + FULL_POS
    neg = j["neg"] + ", " + FULL_NEG
    rejected = 0
    touch = []
    elong = 0.0
    min_elong = j.get("min_elong", 0.0)
    attempt = 0
    path = os.path.join(RAW, j["id"] + ".png")
    while True:
        if attempt:
            path = os.path.join(RAW, "%s_a%d.png" % (j["id"], attempt))
        if not os.path.exists(path):
            t0 = time.time()
            download(wait(post(stage_txt2img(prompt, neg, j["seed"] + attempt, "ss1"))), path)
            t["txt2img"] = round(t.get("txt2img", 0) + time.time() - t0, 1)
        fc = cut.ship_frame_check(Image.open(path), tol=34, margin=FRAME_MARGIN,
                                  min_elong=min_elong)
        touch = fc["touch"]
        elong = fc["elong"]
        if fc["ok"]:
            break
        rejected += 1
        attempt += 1
        if attempt >= FRAME_TRIES:  # все попытки негодны — берём последний кадр
            break
    raw = path
    rec = dict(j)
    rec["prompt"] = prompt
    rec["neg"] = neg
    rec["raw"] = raw
    rec["frame_check"] = {"attempts": attempt + 1, "rejected": rejected,
                          "accepted_seed": j["seed"] + attempt, "touch": touch,
                          "elong": elong}
    rec["model"] = JUGG
    rec["size"] = SIZE
    rec["steps"] = 32
    rec["cfg"] = 6.0
    rec["sampler"] = "dpmpp_2m/karras"
    rec = post_candidate(rec, t)
    return rec


def post_candidate(rec, t, hires=False):
    """Вырез чёрного фона + нормализация (нос вправо, 200x200)."""
    raw = rec.get("hires_raw") or rec["raw"]
    suffix = "_hr_200.png" if hires else "_200.png"
    cand = os.path.join(OUT, rec["id"] + suffix)
    rep = os.path.join(OUT, rec["id"] + ("_hr_cut.json" if hires else "_cut.json"))
    t0 = time.time()
    subprocess.run([PY, CUT, raw, cand, "--method", "hyst", "--tol-close", "12",
                    "--tol-wide", "40", "--fill-holes", "--report", rep], check=True)
    t["cut"] = round(time.time() - t0, 1)
    rec["candidate"] = cand
    rec["cut_method"] = "hyst(12/40)+fill_holes"
    with open(rep, encoding="utf-8") as f:
        rec["cut"] = json.load(f)
    rec["timings"] = t
    return rec


def stage_hires(img_name, prompt, neg, seed, prefix):
    """4x-UltraSharp -> ImageScale HIRES -> img2img denoise 0.40 (детализация)."""
    nodes, mp, cp = _base(JUGG, None, 0.0)
    nodes["2"] = {"class_type": "CLIPTextEncode", "inputs": {"text": prompt, "clip": cp}}
    nodes["3"] = {"class_type": "CLIPTextEncode", "inputs": {"text": neg, "clip": cp}}
    nodes["8"] = {"class_type": "LoadImage", "inputs": {"image": img_name}}
    nodes["12"] = {"class_type": "UpscaleModelLoader", "inputs": {"model_name": UPSCALER}}
    nodes["13"] = {"class_type": "ImageUpscaleWithModel", "inputs": {"upscale_model": ["12", 0], "image": ["8", 0]}}
    nodes["14"] = {"class_type": "ImageScale", "inputs": {"image": ["13", 0], "width": HIRES, "height": HIRES,
                                                          "upscale_method": "lanczos", "crop": "disabled"}}
    nodes["4"] = {"class_type": "VAEEncode", "inputs": {"pixels": ["14", 0], "vae": ["1", 2]}}
    nodes["5"] = {"class_type": "KSampler", "inputs": {
        "seed": seed, "steps": 30, "cfg": 6.0, "sampler_name": "dpmpp_2m", "scheduler": "karras",
        "denoise": 0.40, "model": mp, "positive": ["2", 0], "negative": ["3", 0],
        "latent_image": ["4", 0]}}
    nodes["6"] = {"class_type": "VAEDecode", "inputs": {"samples": ["5", 0], "vae": ["1", 2]}}
    nodes["7"] = {"class_type": "SaveImage", "inputs": {"images": ["6", 0], "filename_prefix": prefix}}
    return nodes


def run_hires(j):
    raw = j["raw"]
    r1 = "ss_%s_hr.png" % j["id"]
    shutil.copyfile(raw, os.path.join(r"C:\ComfyUI\input", r1))
    out = os.path.join(RAW, j["id"] + "_hires.png")
    if not os.path.exists(out):
        pos = j["prompt"] + ", maximal detail, dense surface detail, masterpiece"
        wf = stage_hires(r1, pos, j["neg"], j["seed"], "ss_hr")
        download(wait(post(wf)), out)
    rec = dict(j)
    rec["hires_raw"] = out
    rec["hires_scale"] = HIRES
    rec["hires_denoise"] = 0.40
    return post_candidate(rec, {}, hires=True)


def main():
    ap = argparse.ArgumentParser(description="Спайк 8: концепты -> спрайты")
    ap.add_argument("--jobs", default="")
    ap.add_argument("--group", default="")
    ap.add_argument("--limit", type=int, default=0)
    ap.add_argument("--hires", default="", help="id через запятую — финалисты в Hi-Res")
    args = ap.parse_args()
    os.makedirs(RAW, exist_ok=True)
    refresh_models()
    meta_path = os.path.join(OUT, "meta.json")
    meta = {"model": JUGG, "size": SIZE, "anchor": ANCHOR, "bg": BG,
            "neg": "%s, %s" % (A_NEG, NEG_EXTRA), "entries": []}
    if os.path.exists(meta_path):
        with open(meta_path, encoding="utf-8") as f:
            meta.update({k: v for k, v in json.load(f).items() if k != "entries"})
        with open(meta_path, encoding="utf-8") as f:
            meta["entries"] = json.load(f).get("entries", [])
    entries = meta["entries"]
    done = {e["id"] for e in entries}

    jobs = build_jobs()
    if args.group:
        jobs = [j for j in jobs if j["group"] == args.group]
    if args.jobs:
        want = set(args.jobs.split(","))
        jobs = [j for j in jobs if j["id"] in want]
    if args.limit:
        jobs = jobs[:args.limit]
    frame_rejected = 0
    frame_attempts = 0
    frame_jobs = 0
    for j in jobs:
        if j["id"] in done and os.path.exists(os.path.join(OUT, j["id"] + "_200.png")):
            print("SKIP %s" % j["id"])
            continue
        t0 = time.time()
        rec = run_job(j)
        rec["timings"]["total"] = round(time.time() - t0, 1)
        entries = [e for e in entries if e["id"] != rec["id"]] + [rec]
        meta["entries"] = entries
        with open(meta_path, "w", encoding="utf-8") as f:
            json.dump(meta, f, ensure_ascii=False, indent=1)
        fc = rec.get("frame_check", {})
        frame_rejected += fc.get("rejected", 0)
        frame_attempts += fc.get("attempts", 0)
        frame_jobs += 1
        print("DONE %s: %s frame=%s" % (rec["id"], rec["timings"], fc))
    if frame_jobs:
        avg = float(frame_attempts) / frame_jobs
        print("FRAME STATS: jobs=%d rejected=%d attempts=%d avg=%.2f" % (
            frame_jobs, frame_rejected, frame_attempts, avg))

    if args.hires:
        want = set(args.hires.split(","))
        by_id = {e["id"]: e for e in entries}
        for jid in args.hires.split(","):
            if jid not in want or jid not in by_id:
                continue
            rec = run_hires(by_id[jid])
            entries = [e for e in entries if e["id"] != rec["id"]] + [rec]
            meta["entries"] = entries
            with open(meta_path, "w", encoding="utf-8") as f:
                json.dump(meta, f, ensure_ascii=False, indent=1)
            print("HIRES DONE %s" % jid)
    print("meta: %s" % meta_path)


if __name__ == "__main__":
    main()
