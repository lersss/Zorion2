# -*- coding: utf-8 -*-
# anim_all.py — мягкие демо-анимации для всех принятых аватаров.
# Принцип: НИКАКОГО движения формы. Только мягкие световые эффекты:
#   glow (пульс свечения, амплитуда ~0.18), hue (сдвиг оттенка ±6°),
#   sparkles (редкие искры, только для неантропоморфных),
#   spin (оборот 360° — только для вихрей).
# Каждому аватару — уместный набор из карты.
import os, math
import numpy as np
from PIL import Image

SRC = r'C:\Zorion2\ai_drafts\final_accepted'
OUT = r'C:\Zorion2\ai_drafts\final_accepted\anim'
N = 60  # кадров
DUR = 1000 // 24

# карта: файл -> (эффекты, комментарий)
MAP = {
    'race_f1_humans_male.png': ('glow_soft', 'едва заметное дыхание света'),
    'race_f1_humans_female.png': ('glow_soft', 'едва заметное дыхание света'),
    'race_f1_humans_elder.png': ('glow_soft', 'едва заметное дыхание света'),
    'race_f1_humans_young.png': ('glow_soft', 'едва заметное дыхание света'),
    'race_f1_water_humanoid.png': ('glow', 'глаза/жабры мягко дышат'),
    'race_f2_frostwalker.png': ('glow', 'ледяное ядро пульсирует'),
    'race_frostwalker.png': ('glow', 'кристаллы мерцают'),
    'race_f4_brimstone.png': ('glow_hue', 'янтарные глаза и трещины'),
    'race_f4_smokers.png': ('glow_hue', 'янтарные жилы в минерале'),
    'race_f4_sulfur_beast.png': ('glow', 'глаза-щели дышат'),
    'race_f4_tidal_vortex1.png': ('spin', 'полный оборот + мягкий glow'),
    'race_f4_tidal_vortex2.png': ('spin', 'полный оборот + мягкий glow'),
    'race_f6_crystal_druse.png': ('glow_hue', 'грани переливаются'),
    'race_f6_silicon_humanoid.png': ('glow', 'кристальные глаза'),
    'race_f9_energy_being.png': ('glow_hue_sparkles', 'энергия + искры'),
    'race_f9_energy_humanoid.png': ('glow_hue', 'светящееся лицо'),
    'race_fumarole_b.png': ('glow_sparkles', 'жерла + дым-искры'),
    'race_fungoid.png': ('glow_sparkles', 'споры вокруг гриба'),
    'race_magmite.png': ('glow_hue', 'лавовые трещины'),
    'race_magnetar.png': ('glow_hue_sparkles', 'дуги + ядро + искры'),
    'race_nimbus.png': ('glow', 'облачное ядро дышит'),
    'race_oceanid.png': ('glow', 'глаза-бусинки'),
}


def soft_glow(arr, mask, amp=0.18):
    """Мягкий пульс свечения."""
    out = []
    for i in range(N):
        t = 2 * math.pi * i / N
        f = arr.copy().astype(np.float32)
        if mask.sum() > 0:
            f[mask, :3] *= (1 + amp * math.sin(t))
        out.append(np.clip(f, 0, 255).astype(np.uint8))
    return out


def soft_hue(arr, mask, shift_deg=6):
    """Мягкий сдвиг оттенка светящихся пикселей."""
    out = []
    for i in range(N):
        t = 2 * math.pi * i / N
        shift = int(shift_deg / 360 * 255 * math.sin(t))
        img = Image.fromarray(arr)
        hsv = img.convert('HSV')
        h, s, v = hsv.split()
        h = h.point(lambda x: (x + shift) % 255)
        img2 = Image.merge('HSV', (h, s, v)).convert('RGBA')
        f = np.array(img2)
        f[~mask] = arr[~mask]
        out.append(f)
    return out


def rare_sparkles(arr, solid, n=4, seed=7):
    """Редкие искры (мягко)."""
    rng = np.random.RandomState(seed)
    h, w = arr.shape[:2]
    ys, xs = np.where(solid)
    cy, cx = int(ys.mean()), int(xs.mean())
    out = []
    for i in range(N):
        f = arr.copy()
        for _ in range(n):
            ang = rng.uniform(0, 2 * math.pi)
            r = rng.uniform(25, 70)
            px = int(cx + r * math.cos(ang)); py = int(cy + r * math.sin(ang))
            if 0 <= px < w and 0 <= py < h:
                br = rng.uniform(30, 70)
                f[py, px, :3] = np.minimum(f[py, px, :3].astype(int) + br, 255)
                f[py, px, 3] = 255
        out.append(f)
    return out


def spin360(arr, base_frames):
    """Полный оборот поверх базовых кадров (для вихрей)."""
    out = []
    for i in range(N):
        ang = i * (360.0 / N)
        out.append(Image.fromarray(base_frames[i]).rotate(ang, resample=Image.BICUBIC, expand=False))
    return [np.array(x) for x in out]


def glow_mask(arr, solid):
    rgb = arr[:, :, :3].astype(np.int16)
    return (rgb.mean(axis=2) > 140) & solid


def build(recipe, arr, solid):
    gm = glow_mask(arr, solid)
    if recipe == 'glow_soft':
        # сверхмягкий: амплитуда 0.10, только для людей
        return soft_glow(arr, gm, amp=0.10)
    # база: мягкий glow (везде)
    base = soft_glow(arr, gm)
    if recipe == 'glow':
        return base
    if recipe == 'glow_hue':
        hu = soft_hue(arr, gm)
        # смешиваем: hue меняет цвет, glow меняет яркость -> берём hue-кадры и добавляем glow-пульс
        out = []
        for i in range(N):
            f = hu[i].copy().astype(np.float32)
            f[gm, :3] *= (1 + 0.18 * math.sin(2 * math.pi * i / N))
            out.append(np.clip(f, 0, 255).astype(np.uint8))
        return out
    if recipe == 'glow_sparkles':
        sp = rare_sparkles(arr, solid)
        out = []
        for i in range(N):
            f = sp[i].copy().astype(np.float32)
            f[gm, :3] *= (1 + 0.18 * math.sin(2 * math.pi * i / N))
            out.append(np.clip(f, 0, 255).astype(np.uint8))
        return out
    if recipe == 'glow_hue_sparkles':
        hu = soft_hue(arr, gm)
        sp = rare_sparkles(arr, solid)
        out = []
        for i in range(N):
            f = hu[i].copy().astype(np.float32)
            f[gm, :3] *= (1 + 0.18 * math.sin(2 * math.pi * i / N))
            # редкие искры
            f[sp[i][:, :, 3] > 240] = sp[i][sp[i][:, :, 3] > 240]
            out.append(np.clip(f, 0, 255).astype(np.uint8))
        return out
    if recipe == 'spin':
        base = soft_glow(arr, gm)
        return spin360(arr, base)
    return base


def main():
    os.makedirs(OUT, exist_ok=True)
    for fname, (recipe, comment) in MAP.items():
        p = os.path.join(SRC, fname)
        if not os.path.exists(p):
            print('нет файла:', fname); continue
        arr = np.array(Image.open(p).convert('RGBA'))
        alpha = arr[:, :, 3]
        solid = alpha > 40
        frames = build(recipe, arr, solid)
        base = os.path.splitext(fname)[0]
        out = os.path.join(OUT, base + '.gif')
        imgs = [Image.fromarray(f, 'RGBA') for f in frames]
        imgs[0].save(out, save_all=True, append_images=imgs[1:], duration=DUR, loop=0, disposal=2)
        print('OK %-32s %s  (%d KB)' % (base, comment, round(os.path.getsize(out) / 1024)))


if __name__ == '__main__':
    main()