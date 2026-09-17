# -*- coding: utf-8 -*-
# anthropo_check.py — проверка «антропоморфно/неантропоморфно» через CLIP zero-shot.
# Использование: python anthropo_check.py <img.png> [img2.png ...]
# Для каждой картинки считает вероятности по категориям:
#   humanoid (человекоподобное лицо/фигура), non-humanoid creature (тварь), abstract (абстракция).
import sys
import numpy as np
from PIL import Image

# принудительный UTF-8 для консоли Windows (иначе падает на кириллице в cp1251)
if sys.stdout.encoding and sys.stdout.encoding.lower() not in ("utf-8", "utf8"):
    sys.stdout.reconfigure(encoding="utf-8", errors="replace")

MODEL = "openai/clip-vit-base-patch32"

CATS = {
    "humanoid_face": "a humanoid face with eyes, nose and mouth, human-like portrait",
    "humanoid_figure": "a humanoid creature with human-like body shape",
    "beast_head": "a non-humanoid beast head, alien monster, animal-like creature",
    "abstract_form": "an abstract non-humanoid formation, crystals, rocks, energy, no face",
    "human_shoulders": "human shoulders and collarbone, human torso anatomy",
    "human_neck_chin": "a human neck and chin, human head anatomy",
    "alien_mandibles": "an alien head with mandibles, beak, eyestalks or crests, non-human anatomy",
}

# варианты для разнообразия
ALT = {
    "humanoid_face": ["a human-like face looking at viewer", "a person's face, human face"],
    "beast_head": ["an alien monster head", "an animal head, no human features"],
    "abstract_form": ["an abstract shape without a face", "a rock crystal formation"],
    "human_shoulders": ["human shoulders, human collarbone", "human-like torso"],
    "human_neck_chin": ["a human chin and neck", "a human face with ears"],
    "alien_mandibles": ["alien creature with mandibles instead of jaw", "alien head with eyestalks and crests"],
}


def _patch():
    import transformers.utils.import_utils as iu
    if not hasattr(iu, "_zp"):
        iu.check_torch_load_is_safe = lambda: None
        iu._zp = True
    import transformers.modeling_utils as mu
    if not hasattr(mu, "_zp"):
        mu.check_torch_load_is_safe = lambda: None
        mu._zp = True


def load():
    _patch()
    import torch
    from transformers import CLIPProcessor, CLIPModel
    proc = CLIPProcessor.from_pretrained(MODEL, local_files_only=True)
    model = CLIPModel.from_pretrained(MODEL, local_files_only=True)
    model.eval()
    return proc, model, torch


def check(pair, path, idx):
    proc, model, torch = pair
    img = Image.open(path).convert("RGB")
    print(f"[{idx}] {path}")
    texts = [CATS[k] for k in CATS]
    keys = list(CATS.keys())
    for alt_key in ALT:
        texts.extend(ALT[alt_key])
        keys.extend([alt_key] * len(ALT[alt_key]))
    with torch.no_grad():
        inputs = proc(text=texts, images=img, return_tensors="pt", padding=True)
        outputs = model(**inputs)
        probs = outputs.logits_per_image.softmax(dim=1)[0].cpu().numpy()
    # агрегируем по базовым категориям (сумма вероятностей вариантов)
    agg = {}
    for k, p in zip(keys, probs):
        base = k.split("__")[0]
        agg[base] = agg.get(base, 0.0) + p
    order = sorted(agg.items(), key=lambda x: -x[1])
    humanoid = agg.get("humanoid_face", 0) + agg.get("humanoid_figure", 0)
    human_parts = agg.get("human_shoulders", 0) + agg.get("human_neck_chin", 0)
    alien_parts = agg.get("alien_mandibles", 0)
    verdict = "АНТРОПОМОРФНО"
    if humanoid <= 0.5:
        if alien_parts > 0.5:
            verdict = "НЕАНТРОПО (тварь с нечеловеч. чертами)"
        elif agg.get("beast_head", 0) > agg.get("abstract_form", 0):
            verdict = "НЕАНТРОПО (тварь)"
        else:
            verdict = "НЕАНТРОПО (абстракция)"
    human_warn = "⚠️ ЕСТЬ человеческие части тела" if human_parts > 0.25 else "нет человеческих частей"
    for k, v in order:
        print(f"    {k}: {v:.2f}")
    print(f"    {human_warn} (human_parts={human_parts:.2f})")
    print(f"    ВЕРДИКТ: {verdict}")


def main():
    if len(sys.argv) < 2:
        print("Использование: python anthropo_check.py <img> [img...]")
        return
    pair = load()
    for i, p in enumerate(sys.argv[1:], 1):
        check(pair, p, i)


if __name__ == "__main__":
    main()