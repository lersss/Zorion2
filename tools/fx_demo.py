# -*- coding: utf-8 -*-
# fx_demo.py — демонстрация процедурных эффектов на одном аватаре (для сравнения).
# Каждый эффект сохраняется отдельным GIF. НЕ формат релиза — показ идей.
# Эффекты: glow, radial, hue, sparkles, vortex, breathing.
# Использование: python fx_demo.py <in.png> <out_dir>
import sys, os, math
import numpy as np
from PIL import Image, ImageFilter

FRAMES = 24
DUR = 1000 // 24


def base(arr, mask):
    """Светящиеся пиксели = яркие."""
    rgb = arr[:, :, :3].astype(np.int16)
    return (rgb.mean(axis=2) > 140) & mask


def frames_glow(arr, mask):
    out = []
    for i in range(FRAMES):
        t = 2 * math.pi * i / FRAMES
        f = arr.copy().astype(np.float32)
        f[mask, :3] *= (1 + 0.4 * math.sin(t))
        out.append(np.clip(f, 0, 255).astype(np.uint8))
    return out


def frames_radial(arr, mask):
    """Радиальный ореол вокруг непрозрачной формы, пульсирующий."""
    alpha = arr[:, :, 3]
    solid = alpha > 40
    # дистанционное свечение: расширяем solid
    glow_src = Image.fromarray((solid * 255).astype(np.uint8), 'L').filter(ImageFilter.MaxFilter(7))
    glow_arr = np.array(glow_src) > 0
    out = []
    for i in range(FRAMES):
        t = 2 * math.pi * i / FRAMES
        f = arr.copy().astype(np.float32)
        amp = 0.5 + 0.3 * math.sin(t)
        halo = glow_arr & ~solid
        # оранжевый ореол
        f[halo, 0] += 80 * amp
        f[halo, 1] += 40 * amp
        f[halo, 2] += 20 * amp
        f[halo, 3] = np.maximum(f[halo, 3], 90)
        out.append(np.clip(f, 0, 255).astype(np.uint8))
    return out


def frames_hue(arr, mask):
    """Медленный сдвиг оттенка светящихся пикселей (янтарь -> золото)."""
    from PIL import Image as I
    out = []
    for i in range(FRAMES):
        shift = int(12 * math.sin(2 * math.pi * i / FRAMES))
        f = arr.copy()
        img = I.fromarray(f)
        hsv = img.convert('HSV')
        h, s, v = hsv.split()
        h = h.point(lambda x: (x + shift) % 255)
        img2 = I.merge('HSV', (h, s, v)).convert('RGBA')
        a2 = np.array(img2)
        a2[~mask] = arr[~mask]  # не трогаем не-светящиеся
        out.append(a2)
    return out


def frames_sparkles(arr, mask, seed=7):
    """Редкие светящиеся искры, вспыхивающие вокруг формы."""
    rng = np.random.RandomState(seed)
    h, w = arr.shape[:2]
    ys, xs = np.where(mask)
    cy, cx = int(ys.mean()), int(xs.mean())
    out = []
    for i in range(FRAMES):
        f = arr.copy()
        n = 6
        for _ in range(n):
            ang = rng.uniform(0, 2 * math.pi)
            r = rng.uniform(30, 70)
            px = int(cx + r * math.cos(ang))
            py = int(cy + r * math.sin(ang))
            if 0 <= px < w and 0 <= py < h:
                br = rng.uniform(60, 130)
                f[py, px, :3] = np.minimum(f[py, px, :3].astype(int) + br, 255)
                f[py, px, 3] = 255
        out.append(f)
    return out


def frames_vortex(arr, mask):
    """Внутренний завиток: светящиеся кольца вращаются ВНУТРИ формы."""
    h, w = arr.shape[:2]
    ys, xs = np.where(mask)
    cy, cx = int(ys.mean()), int(xs.mean())
    out = []
    for i in range(FRAMES):
        f = arr.copy()
        rot = i * (2 * math.pi / FRAMES)
        for rr in range(20, 90, 20):
            for a in range(0, 8):
                ang = a * math.pi / 4 + rot
                px = int(cx + rr * math.cos(ang))
                py = int(cy + rr * math.sin(ang))
                if 0 <= px < w and 0 <= py < h and mask[py, px]:
                    f[py, px, :3] = np.minimum(f[py, px, :3].astype(int) + 60, 255)
        out.append(f)
    return out


def frames_breathing(arr, mask):
    """Очень лёгкий zoom 1-2% (глубина, незаметен как движение)."""
    base_im = Image.fromarray(arr)
    out = []
    for i in range(FRAMES):
        s = 1.0 + 0.015 * math.sin(2 * math.pi * i / FRAMES)
        nw, nh = int(200 * s), int(200 * s)
        im2 = base_im.resize((nw, nh), Image.LANCZOS)
        canvas = Image.new('RGBA', (200, 200), (0, 0, 0, 0))
        canvas.paste(im2, ((200 - nw) // 2, (200 - nh) // 2), im2)
        out.append(np.array(canvas))
    return out


def save_gif(frames, path):
    imgs = [Image.fromarray(f, 'RGBA') for f in frames]
    imgs[0].save(path, save_all=True, append_images=imgs[1:], duration=DUR, loop=0, disposal=2)


def main():
    if len(sys.argv) < 3:
        print("Использование: python fx_demo.py <in.png> <out_dir>")
        return
    inp, outd = sys.argv[1], sys.argv[2]
    os.makedirs(outd, exist_ok=True)
    arr = np.array(Image.open(inp).convert('RGBA'))
    alpha = arr[:, :, 3]
    solid = alpha > 40
    glowm = base(arr, solid)
    name = os.path.splitext(os.path.basename(inp))[0]

    fx = {
        'glow': lambda: frames_glow(arr, glowm),
        'radial': lambda: frames_radial(arr, solid),
        'hue': lambda: frames_hue(arr, glowm),
        'sparkles': lambda: frames_sparkles(arr, solid),
        'vortex': lambda: frames_vortex(arr, solid),
        'breathing': lambda: frames_breathing(arr, solid),
    }
    for k, fn in fx.items():
        p = os.path.join(outd, f"{name}_{k}.gif")
        save_gif(fn(), p)
        print("Saved:", p, round(os.path.getsize(p) / 1024, 1), "KB")


if __name__ == '__main__':
    main()