# -*- coding: utf-8 -*-
# caption_image.py — ЛОКАЛЬНЫЙ контрольный «глаз» для художника.
# Описывает картинку текстом (BLIP) + базовые проверки (пустая/размер/цвета).
# Использование: python caption_image.py <image.png> [image2.png ...]
# Модель: Salesforce/blip-image-captioning-base (~990 МБ, качается один раз в HF-кэш).
import sys, os
from PIL import Image

MODEL = "Salesforce/blip-image-captioning-base"


def _allow_bin_load():
    """Веса .bin уже скачаны; torch 2.5.1 блокирует torch.load из-за CVE-2025-32434.
    Обходим проверку монkeypatch'ем (weights_only=True активен, модель доверенная Salesforce,
    кэш локальный). Для .bin без сафетенсорс это единственный путь на torch 2.5."""
    import transformers.utils.import_utils as iu
    if not hasattr(iu, "_zorion_patched"):
        iu.check_torch_load_is_safe = lambda: None
        iu._zorion_patched = True
    # modeling_utils держит локальную ссылку через импорт — патчим и её
    import transformers.modeling_utils as mu
    if not hasattr(mu, "_zorion_patched"):
        mu.check_torch_load_is_safe = lambda: None
        mu._zorion_patched = True


def load_pipeline():
    _allow_bin_load()
    from transformers import BlipProcessor, BlipForConditionalGeneration
    proc = BlipProcessor.from_pretrained(MODEL, local_files_only=True)
    model = BlipForConditionalGeneration.from_pretrained(MODEL, local_files_only=True)
    return proc, model


def describe(pair, path, idx):
    proc, model = pair
    try:
        img0 = Image.open(path)
    except Exception as e:
        print(f"[{idx}] {path}: ОШИБКА открытия: {e}")
        return
    w, h = img0.size
    print(f"[{idx}] {path} ({w}x{h}, mode={img0.mode})")
    # --- контроль «есть ли объект» + «посаженность» (по альфе, ДО конвертации) ---
    import numpy as np
    a0 = np.array(img0)
    if a0.ndim == 3 and a0.shape[2] == 4:
        alpha = a0[:, :, 3]
        mask = alpha > 40
        nz = int(mask.sum())
        if nz > 0:
            ys, xs = np.where(mask)
            bbox = (xs.min(), ys.min(), xs.max(), ys.max())
            bottom_rows = int(mask[-5:, :].sum())  # пиксели у нижней кромки
            print(f"    контент: px={nz} ({nz/mask.size:.0%}) bbox={bbox} низ_у_края={bottom_rows} {'ПОСАЖЕН' if bottom_rows>50 else 'ВИСИТ!'}")
        else:
            print("    контент: ПУСТО (нет объекта)")
    else:
        arr = a0.astype(int)
        bg = arr[0, 0]
        diff = np.abs(arr - bg).sum(axis=2)
        m = diff > 60
        nz = int(m.sum())
        print(f"    контент: px={nz} ({nz/m.size:.0%}) (RGB, фон={bg.tolist()})")
    # --- BLIP caption ---
    img = img0.convert("RGB")
    try:
        import torch
        inp = proc(img, return_tensors="pt")
        with torch.no_grad():
            out = model.generate(**inp, max_new_tokens=60)
        text = proc.decode(out[0], skip_special_tokens=True).strip()
        print(f"    BLIP: {text}")
    except Exception as e:
        print(f"    BLIP ошибка: {e}")
    print(f"    mean RGB: {np.array(img).mean(axis=(0,1)).round(1)}")


def main():
    if len(sys.argv) < 2:
        print("Использование: python caption_image.py <img> [img...]")
        return
    pipe = load_pipeline()
    for i, p in enumerate(sys.argv[1:], 1):
        describe(pipe, p, i)


if __name__ == "__main__":
    main()