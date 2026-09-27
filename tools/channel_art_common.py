# channel_art_common.py — общие утилиты для 4 панелей оформления канала.
# Палитра, виньетка, звёздная пыль, сборка contact sheet. Ничего игрового не трогает.
import numpy as np
from PIL import Image, ImageDraw, ImageFilter

PAL = {
    "space": (0x0A, 0x0F, 0x1A),
    "far": (0x14, 0x1B, 0x2E),
    "haze": (0x1E, 0x2A, 0x44),
    "dust": (0xC8, 0xD6, 0xE5),
    "core": (0xEA, 0xF6, 0xFF),
    "violet": (0x7C, 0x6C, 0xFF),
    "pink": (0xFF, 0x6B, 0x9D),
    "teal": (0x4D, 0xD6, 0xB3),
    "amber": (0xF5, 0xA6, 0x23),
    "agcyan": (0x5A, 0xC8, 0xFA),
    "orange": (0xFF, 0x8A, 0x5C),
    "magenta": (0xB3, 0x88, 0xFF),
    "green": (0x69, 0xF0, 0xAE),
    "yellow": (0xFF, 0xD7, 0x40),
    "azure": (0x40, 0xC4, 0xFF),
    "body_light": (0xE8, 0xC7, 0x9A),
    "body_terra": (0xC9, 0x88, 0x4F),
    "body_shadow": (0x3A, 0x2A, 0x22),
    "atmo_halo": (0xA8, 0xF0, 0xE2),
    "agent": (0xFF, 0xD7, 0x40),
    "human": (0x5A, 0xC8, 0xFA),
}

MASTER = (768, 432)


def hex_rgb(h):
    h = h.lstrip("#")
    return tuple(int(h[i:i + 2], 16) for i in (0, 2, 4))


def vignette(arr, strength=0.55, power=2.2):
    h, w = arr.shape[:2]
    yy, xx = np.mgrid[0:h, 0:w]
    nx = (xx - w / 2) / (w / 2)
    ny = (yy - h / 2) / (h / 2)
    r = np.sqrt(nx * nx + ny * ny)
    r = np.clip(r / 1.45, 0, 1)
    m = 1.0 - strength * (r ** power)
    return np.clip(arr.astype(float) * m[..., None], 0, 255)


def add_stars(arr, count=260, seed=7, max_lum=200, size_p=0.012):
    rng = np.random.default_rng(seed)
    h, w = arr.shape[:2]
    out = arr.copy().astype(float)
    for _ in range(count):
        x = rng.integers(0, w); y = rng.integers(0, h)
        if rng.random() < 0.15:
            lum = rng.uniform(90, max_lum)
            rad = 1.0
        else:
            lum = rng.uniform(20, 110)
            rad = 0.6
        out[y, x] = np.maximum(out[y, x], lum)
        if rng.random() < 0.05:
            for dx, dy in ((1, 0), (-1, 0), (0, 1), (0, -1)):
                if 0 <= x + dx < w and 0 <= y + dy < h:
                    out[y + dy, x + dx] = np.maximum(out[y + dy, x + dx], lum * 0.55)
    return np.clip(out, 0, 255)


def grade_space(arr, ambient=0.06):
    """Подтянуть тёмный фон к палитре #0A0F1A, не трогая яркие объекты."""
    a = arr.astype(float)
    lum = a.max(axis=2, keepdims=True)
    dark = np.clip(1.0 - lum / 60.0, 0, 1)
    target = np.array(PAL["space"], float)
    a = a * (1 - dark) + (target * (1 - ambient) + a * ambient) * dark
    return np.clip(a, 0, 255)


def glow_points(img, pts, color, radius=4, intensity=1.0):
    """Мягкие искры-свечения в точках pts=[(x,y,scale)]."""
    h, w = img.size[1], img.size[0]
    layer = Image.new("RGB", (w, h), (0, 0, 0))
    d = ImageDraw.Draw(layer)
    col = tuple(int(c * intensity) for c in color)
    for (x, y, s) in pts:
        r = radius * s
        d.ellipse([x - r * 3, y - r * 3, x + r * 3, y + r * 3], fill=tuple(int(c * 0.28) for c in col))
        d.ellipse([x - r * 1.6, y - r * 1.6, x + r * 1.6, y + r * 1.6], fill=tuple(int(c * 0.7) for c in col))
        d.ellipse([x - r, y - r, x + r, y + r], fill=col)
    layer = layer.filter(ImageFilter.GaussianBlur(radius * 0.9))
    # аддитивно
    base = np.array(img).astype(float)
    add = np.array(layer).astype(float)
    return Image.fromarray(np.clip(base + add, 0, 255).astype(np.uint8))


def save_contact_sheet(paths, out_path, gutter=16, bg=(10, 15, 26), width=320, labels=None):
    imgs = [Image.open(p).convert("RGB") for p in paths]
    scale = width / imgs[0].width
    tw, th = width, int(round(imgs[0].height * scale))
    n = len(imgs)
    W = n * tw + (n + 1) * gutter
    H = th + 2 * gutter
    sheet = Image.new("RGB", (W, H), bg)
    draw = ImageDraw.Draw(sheet)
    for i, im in enumerate(imgs):
        t = im.resize((tw, th), Image.LANCZOS)
        x = gutter + i * (tw + gutter)
        sheet.paste(t, (x, gutter))
        draw.rectangle([x - 1, gutter - 1, x + tw, gutter + th], outline=(30, 42, 68))
    sheet.save(out_path)
    return out_path
