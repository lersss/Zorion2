# -*- coding: utf-8 -*-
# Вырез спрайта корабля расы + нормализация (нос вправо, 200x200).
# Промотирован из tools/spike_ship_sprite_cut.py (рецепт 2026-09-21, тот же
# функционал) — вызывается арт-студией (cmd/art-studio/postproc.ShipSpriteCut).
# Фон кандидатов Juggernaut XL — почти ровный чёрный (углы ~5-10). Вырез:
#   black — порог по яркости + связность с рамкой (фон = тёмное, касающееся края);
#   color — вырез по цвету фона (process_ship.remove_bg_by_color, BG_TOL=40);
#   dark  — всё темнее порога (process_ship.remove_bg_by_dark);
#   hyst  — гистерезис по плоскости фона (tol_close/tol_wide) — рецепт студии;
#   rembg — U2Net (для сравнения; art_ships.md §3.3: режет корпус).
# Метрика сравнения (compare): сколько фона осталось / сколько корпуса съедено.
# Режимы: обычный (вырез+нормализация), --orient-only (подсказка носа,
# /ships/auto), --frame-check (автопроверка кадра: край/вытянутость, джоб
# кораблей).
import argparse
import json
import math
import os
import sys

import numpy as np
from PIL import Image, ImageOps
from scipy import ndimage

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import process_ship as ps  # noqa: E402

CANVAS = 200
PAD = 6


def black_bg_mask(rgb, bg_max):
    """Фон = тёмные пиксели, связные с рамкой кадра. Возвращает bool-маску фона."""
    br = rgb.max(axis=2)
    dark = br <= bg_max
    labels, num = ndimage.label(dark)
    if num == 0:
        return np.zeros(dark.shape, bool)
    border = np.unique(np.concatenate([labels[0, :], labels[-1, :],
                                       labels[:, 0], labels[:, -1]]))
    border = border[border != 0]
    if border.size == 0:
        return np.zeros(dark.shape, bool)
    return np.isin(labels, border)


def remove_bg_black(img, bg_max=12, min_share=0.02):
    """Вырез чёрного фона по яркости + связности с рамкой (не трогает тёмный корпус,
    не касающийся края). Возвращает RGBA."""
    rgb = np.array(img.convert('RGB'))
    bg = black_bg_mask(rgb, bg_max)
    if bg.mean() < min_share:  # фон не распознан — не режем по яркости
        bg = np.zeros(bg.shape, bool)
    out = np.array(img.convert('RGBA'))
    out[bg] = (0, 0, 0, 0)
    return Image.fromarray(out)


def fill_holes(img, max_share=0.05):
    """Залить внутренние дыры (прозрачное, не связное с рамкой) площадью до
    max_share от холста. Убирает «съеденные» тёмные панели внутри корпуса."""
    arr = np.array(img.convert('RGBA'))
    solid = arr[:, :, 3] > 40
    holes = ~solid
    labels, num = ndimage.label(holes)
    if num == 0:
        return img
    border = np.unique(np.concatenate([labels[0, :], labels[-1, :],
                                       labels[:, 0], labels[:, -1]]))
    limit = max_share * solid.size
    fill = np.zeros(solid.shape, bool)
    for lb in range(1, num + 1):
        if lb in border:
            continue
        m = labels == lb
        if m.sum() <= limit:
            fill |= m
    if not fill.any() or not solid.any():
        return img
    # цвет заливки — средний тон непрозрачных пикселей корпуса
    mean_rgb = arr[:, :, :3][solid].mean(axis=0).astype(np.uint8)
    arr[fill, 0], arr[fill, 1], arr[fill, 2] = mean_rgb
    arr[fill, 3] = 255
    return Image.fromarray(arr)


def cut_rembg(img):
    from rembg import remove
    return remove(img.convert('RGBA'))


def mask_alpha(img):
    arr = np.array(img.convert('RGBA'))
    return arr[:, :, 3] > 40, arr


def _principal_angle(al):
    """Угол главной оси силуэта (радианы, координаты изображения, y вниз)."""
    ys, xs = np.where(al)
    x = xs.astype(np.float64)
    y = ys.astype(np.float64)
    xm, ym = x.mean(), y.mean()
    cxx = ((x - xm) ** 2).mean()
    cyy = ((y - ym) ** 2).mean()
    cxy = ((x - xm) * (y - ym)).mean()
    return 0.5 * math.atan2(2.0 * cxy, cxx - cyy)


def _glow_mask(al, arr):
    """Двигательный признак: яркое насыщенное свечение (сопла/дюзы) — корма.
    Светло-серый корпус отсекается низкой насыщенностью."""
    rgb = arr[:, :, :3].astype(np.float32)
    mx = rgb.max(axis=2)
    mn = rgb.min(axis=2)
    sat = (mx - mn) / np.maximum(mx, 1.0)
    lum = 0.299 * rgb[:, :, 0] + 0.587 * rgb[:, :, 1] + 0.114 * rgb[:, :, 2]
    return al & (lum > 140.0) & (sat > 0.25)


def _engine_mass(glow_band):
    """Масса «сопел» в полосе: только компактные заполненные пятна
    (круглые дюзы), а не вытянутое стекло кабины/блики корпуса."""
    labels, num = ndimage.label(glow_band)
    mass = 0
    for lb in range(1, num + 1):
        m = labels == lb
        a = int(m.sum())
        if a < 20:
            continue
        ys, xs = np.where(m)
        bw = xs.max() - xs.min() + 1
        bh = ys.max() - ys.min() + 1
        if bw < 1 or bh < 1:
            continue
        ar = bw / float(bh)
        if 0.5 <= ar <= 2.5 and a / float(bw * bh) >= 0.45:
            mass += a
    return mass


def profile_orientation(img):
    """Геометрия корабля (детерминированно):
      1) главная ось силуэта (PCA) — поворот к горизонтали (нос по длинной оси);
      2) профиль ширины вдоль оси — нос там, где сечение уже и резко сходится;
      3) двигательный признак (яркое свечение сопел = корма) при обнаружении
         перебивает taper.
    Возвращает dict с решением и диагностикой (angle, taper, sharp_*, glow_*)."""
    al, arr = mask_alpha(img)
    if not al.any():
        return {"rotate": False, "angle": 0.0, "mirror": False,
                "ambiguous": True, "reason": "empty"}
    angle = math.degrees(_principal_angle(al))
    # нормализуем к (-45, 45]: главная ось горизонтальна
    while angle > 45.0:
        angle -= 90.0
    while angle <= -45.0:
        angle += 90.0
    rot_img = img
    if abs(angle) > 1.0:
        rot_img = img.rotate(angle, expand=True, resample=Image.BICUBIC,
                             fillcolor=(0, 0, 0, 0))
    al, arr = mask_alpha(rot_img)
    ys, xs = np.where(al)
    if len(xs) == 0:
        return {"rotate": False, "angle": 0.0, "mirror": False,
                "ambiguous": True, "reason": "empty-after-rotate"}
    x0, x1 = xs.min(), xs.max()
    w = x1 - x0 + 1
    h = ys.max() - ys.min() + 1
    # самокоррекция знака поворота: главная ось обязана стать шире высоты
    # (90° — знак не важен: нос определяется уже после поворота и при
    # необходимости зеркалится, ориентир — отсутствие направления у PCA)
    if w < h:
        angle += 90.0
        rot_img = img.rotate(angle, expand=True, resample=Image.BICUBIC,
                             fillcolor=(0, 0, 0, 0))
        al, arr = mask_alpha(rot_img)
        ys, xs = np.where(al)
        x0, x1 = xs.min(), xs.max()
        w = x1 - x0 + 1
        h = ys.max() - ys.min() + 1
    n = max(1, w)
    prof = al.sum(axis=0).astype(np.float64)

    def band(a, b):
        return float(prof[x0 + a:x0 + b].mean()) if b > a else 0.0

    tip = max(1, int(round(n * 0.05)))
    left_tip = float(prof[x0:x0 + tip].mean())
    right_tip = float(prof[x1 - tip + 1:x1 + 1].mean())
    left_band = band(int(n * 0.12), int(n * 0.30))
    right_band = float(prof[x1 - int(n * 0.30) + 1:x1 - int(n * 0.12) + 1].mean())
    sharp_left = (left_band - left_tip) / max(1.0, left_band)
    sharp_right = (right_band - right_tip) / max(1.0, right_band)

    glow = _glow_mask(al, arr)
    fw = max(2, int(round(n * 0.22)))
    glow_left = _engine_mass(glow[:, x0:x0 + fw])
    glow_right = _engine_mass(glow[:, x1 - fw + 1:x1 + 1])
    glow_max = max(glow_left, glow_right)
    glow_min = min(glow_left, glow_right)
    engine = False
    if glow_max >= 20 and glow_max >= 1.8 * max(1, glow_min):
        engine = True
        nose_side = 'right' if glow_left > glow_right else 'left'
    else:
        nose_side = 'left' if sharp_left > sharp_right else 'right'
    taper = (left_band - right_band) / max(1.0, max(left_band, right_band))
    if engine:
        ambiguous = False
        reason = 'glow-engine'
    elif abs(sharp_left - sharp_right) < 0.10:
        ambiguous = True
        reason = 'sharp<0.10 (ambiguous)'
    else:
        ambiguous = False
        reason = 'sharpness'
    return {"rotate": bool(abs(angle) > 1.0), "angle": round(float(angle), 2),
            "w": int(w), "h": int(h),
            "left_thick": round(left_band, 1), "right_thick": round(right_band, 1),
            "taper": round(taper, 3),
            "sharp_left": round(sharp_left, 3), "sharp_right": round(sharp_right, 3),
            "glow_left": glow_left, "glow_right": glow_right, "engine": bool(engine),
            "nose_side": nose_side,
            "mirror": bool(nose_side == 'left'),
            "ambiguous": bool(ambiguous), "reason": reason}


def orient_nose_right(img, allow_mirror=True):
    """Повернуть главную ось горизонтально и носом вправо (нос = узкий
    сходящийся конец; сопла/свечение = корма). Возвращает (img, info)."""
    info = profile_orientation(img)
    angle = info.get("angle", 0.0)
    if abs(angle) > 1.0:
        img = img.rotate(angle, expand=True, resample=Image.BICUBIC,
                         fillcolor=(0, 0, 0, 0))
        info["rotated"] = True
    else:
        info["rotated"] = False
    if allow_mirror and info.get("mirror") and not info.get("ambiguous"):
        img = ImageOps.mirror(img)
        info["mirrored"] = True
    else:
        info["mirrored"] = False
    return img, info


def ship_touch_sides(img, tol=34, margin=8):
    """Стороны кадра, которых касается корабль (в пределах margin px от края).
    Фон — плоскость по периметру (process_ship.estimate_background)."""
    rgb = np.array(img.convert('RGB')).astype(np.float32)
    plane = ps.estimate_background(rgb.astype(np.int16))
    diff = np.abs(rgb - plane).sum(axis=2)
    m = diff > tol
    sides = []
    if m[:margin, :].any():
        sides.append('top')
    if m[-margin:, :].any():
        sides.append('bottom')
    if m[:, :margin].any():
        sides.append('left')
    if m[:, -margin:].any():
        sides.append('right')
    return sides


def ship_frame_check(img, tol=34, margin=8, min_elong=0.0):
    """Автопроверка сырого кадра до выреза:
      touch — стороны, к которым корабль подошёл ближе margin px (отбраковка);
      elong — вытянутость силуэта (λ1/λ2 главных осей, PCA): фронтальный
              симметричный вид ≈ 1.0, 3/4-вид вдоль оси ≥ 1.3;
      ok    — кадр годен (не касается края и, если задан порог, вытянут).
    Возвращает dict."""
    rgb = np.array(img.convert('RGB')).astype(np.float32)
    plane = ps.estimate_background(rgb.astype(np.int16))
    m = np.abs(rgb - plane).sum(axis=2) > tol
    sides = []
    if m[:margin, :].any():
        sides.append('top')
    if m[-margin:, :].any():
        sides.append('bottom')
    if m[:, :margin].any():
        sides.append('left')
    if m[:, -margin:].any():
        sides.append('right')
    elong = 0.0
    ys, xs = np.where(m)
    if len(xs) > 10:
        x = xs.astype(np.float64)
        y = ys.astype(np.float64)
        xm, ym = x.mean(), y.mean()
        cxx = ((x - xm) ** 2).mean()
        cyy = ((y - ym) ** 2).mean()
        cxy = ((x - xm) * (y - ym)).mean()
        tr = cxx + cyy
        det = cxx * cyy - cxy * cxy
        disc = math.sqrt(max(0.0, tr * tr / 4.0 - det))
        l1 = tr / 2.0 + disc
        l2 = tr / 2.0 - disc
        elong = float(l1 / max(l2, 1e-9))
    ok = (not sides) and (elong >= min_elong)
    return {"touch": sides, "elong": round(elong, 2), "ok": bool(ok)}


def normalize(img, canvas=CANVAS, fill=1.0, no_orient=False, smooth=2, pad=PAD):
    """Кадрирование по содержимому, ориентация, сглаживание, вписывание в canvas.
    fill<1 — оставить поля (корабль занимает fill от длинной стороны)."""
    img = ps.keep_largest_component(img, keep_parts=True)
    img = ps.crop_to_content(img)
    bbox = img.getbbox()
    if bbox:
        l, t, r, b = bbox
        img = img.crop((max(0, l - pad), max(0, t - pad),
                        min(img.width, r + pad), min(img.height, b + pad)))
    info = {}
    if not no_orient:
        img, info = orient_nose_right(img)
        img = ps.crop_to_content(img)
    img = ps.smooth_edges(img, smooth)
    side = max(img.size)
    target = int(round(canvas * (fill if fill < 1.0 else 1.0)))
    if side > 0 and target < side:
        img = img.resize((max(1, round(img.width * target / side)),
                          max(1, round(img.height * target / side))), Image.LANCZOS)
    out = ImageOps.pad(img, (canvas, canvas), color=(0, 0, 0, 0), centering=(0.5, 0.5))
    return out, info


def remove_bg_flat(img, tol=16, require_border=True, min_share=0.02):
    """Вырез РОВНОГО фона по модели фона (квадратичная поверхность по периметру):
    фон = пиксели в пределах tol (сумма |Δ| по каналам) от плоскости фона.
    require_border — фон обязан быть связным с рамкой кадра (не трогаем тёмный
    корпус внутри силуэта). Juggernaut даёт идеально ровный фон (~19)."""
    rgb = np.array(img.convert('RGB')).astype(np.float32)
    plane = ps.estimate_background(rgb.astype(np.int16))
    diff = np.abs(rgb - plane).sum(axis=2)
    bg = diff <= tol
    if require_border:
        labels, num = ndimage.label(bg)
        if num:
            border = np.unique(np.concatenate([labels[0, :], labels[-1, :],
                                               labels[:, 0], labels[:, -1]]))
            border = border[border != 0]
            bg = np.isin(labels, border) if border.size else np.zeros_like(bg)
        else:
            bg = np.zeros_like(bg)
    if bg.mean() < min_share:
        bg = np.zeros_like(bg)
    out = np.array(img.convert('RGBA'))
    out[bg] = (0, 0, 0, 0)
    return Image.fromarray(out)


def remove_bg_hyst(img, tol_close=14, tol_wide=34, min_share=0.02):
    """Вырез фона с гистерезисом: фон — компоненты, связные с рамкой (широкий
    допуск tol_wide), но только те, где есть хотя бы один «наверняка фон»
    (tol_close). Убирает мягкую тень/градиент вокруг корабля, не выедая корпус:
    сильная кромка корабля (diff >> tol_wide) разрывает компоненту."""
    rgb = np.array(img.convert('RGB')).astype(np.float32)
    plane = ps.estimate_background(rgb.astype(np.int16))
    diff = np.abs(rgb - plane).sum(axis=2)
    wide = diff <= tol_wide
    labels, num = ndimage.label(wide)
    if num == 0:
        return img
    border = np.unique(np.concatenate([labels[0, :], labels[-1, :],
                                       labels[:, 0], labels[:, -1]]))
    border = border[border != 0]
    if border.size == 0:
        return img
    close_counts = np.bincount(labels[diff <= tol_close].ravel(), minlength=num + 1)
    keep = border[close_counts[border] > 0]
    bg = np.isin(labels, keep)
    if bg.mean() < min_share:
        return img
    out = np.array(img.convert('RGBA'))
    out[bg] = (0, 0, 0, 0)
    return Image.fromarray(out)


def compare(raw_path, cut_rgba, bg_tol=8, hull_delta=60):
    """Числа выреза: доля оставшегося фона и съеденного корпуса.
    Определённый фон — в пределах bg_tol от плоскости фона и связный с рамкой;
    определённый корпус — отклонение от плоскости фона больше hull_delta.
    Сравнение в исходном разрешении сырья."""
    rgb = np.array(Image.open(raw_path).convert('RGB')).astype(np.float32)
    arr = np.array(cut_rgba.convert('RGBA'))
    if arr.shape[:2] != rgb.shape[:2]:
        return {"error": "size mismatch"}
    plane = ps.estimate_background(rgb.astype(np.int16))
    diff = np.abs(rgb - plane).sum(axis=2)
    solid = arr[:, :, 3] > 40
    def_bg = remove_bg_flat(Image.open(raw_path), tol=bg_tol, min_share=0.0)
    def_bg = np.array(def_bg)[:, :, 3] == 0
    hull = diff > hull_delta
    bg_left = float((def_bg & solid).sum()) / max(1, int(def_bg.sum()))
    hull_eaten = float((hull & ~solid).sum()) / max(1, int(hull.sum()))
    return {"bg_left": round(bg_left, 4), "hull_eaten": round(hull_eaten, 4),
            "def_bg_share": round(float(def_bg.mean()), 4),
            "hull_share": round(float(hull.mean()), 4),
            "solid_share": round(float(solid.mean()), 4)}


def main():
    ap = argparse.ArgumentParser(description="Спайк 8: вырез+нормализация спрайта")
    ap.add_argument("src")
    ap.add_argument("out", nargs="?", help="выходной PNG (не нужен с --orient-only)")
    ap.add_argument("--method", default="flat",
                    choices=["flat", "hyst", "black", "color", "dark", "rembg"])
    ap.add_argument("--tol", type=int, default=16, help="tol выреза ровного фона (flat)")
    ap.add_argument("--tol-close", type=int, default=14)
    ap.add_argument("--tol-wide", type=int, default=34)
    ap.add_argument("--bg-max", type=int, default=12)
    ap.add_argument("--dark", type=int, default=15)
    ap.add_argument("--fill", type=float, default=1.0)
    ap.add_argument("--fill-holes", action="store_true")
    ap.add_argument("--no-orient", action="store_true")
    ap.add_argument("--canvas", type=int, default=CANVAS,
                    help="сторона квадратного холста спрайта (200 — полный, 100 — эскиз)")
    ap.add_argument("--report", default="")
    ap.add_argument("--orient-only", action="store_true",
                    help="только определить ориентацию (profile_orientation) в "
                         "--report, спрайт не создавать (подсказка приёмки студии)")
    ap.add_argument("--frame-check", action="store_true",
                    help="автопроверка кадра (край/вытянутость) в --report, спрайт "
                         "не создавать (джоб кораблей студии)")
    ap.add_argument("--margin", type=int, default=8,
                    help="запас от края кадра для --frame-check (px)")
    ap.add_argument("--min-elong", type=float, default=0.0,
                    help="минимальная вытянутость силуэта (λ1/λ2) для --frame-check")
    ap.add_argument("--frame-tol", type=int, default=34,
                    help="допуск отклонения от плоскости фона для --frame-check")
    args = ap.parse_args()

    img = Image.open(args.src)
    # frame-check: автопроверка сырого кадра до выреза (джоб кораблей).
    if args.frame_check:
        fc = ship_frame_check(img, tol=args.frame_tol, margin=args.margin,
                              min_elong=args.min_elong)
        if args.report:
            with open(args.report, "w", encoding="utf-8") as f:
                json.dump(fc, f, ensure_ascii=False, indent=1)
        print(json.dumps(fc, ensure_ascii=False))
        return
    # orient-only: подсказка авто-носа для студии (art-studio /ships/auto);
    # единственный источник детекции — эта же функция, что у конвейера.
    if args.orient_only:
        report = {"orient": profile_orientation(img)}
        if args.report:
            with open(args.report, "w", encoding="utf-8") as f:
                json.dump(report, f, ensure_ascii=False, indent=1)
        print(json.dumps(report, ensure_ascii=False))
        return
    if args.method == "flat":
        cut = remove_bg_flat(img, args.tol)
    elif args.method == "hyst":
        cut = remove_bg_hyst(img, args.tol_close, args.tol_wide)
    elif args.method == "black":
        cut = remove_bg_black(img, args.bg_max)
    elif args.method == "color":
        cut = ps.remove_bg_by_color(img)
    elif args.method == "dark":
        cut = ps.remove_bg_by_dark(img, args.dark)
    else:
        cut = cut_rembg(img)
    if args.fill_holes:
        cut = fill_holes(cut)
    stats = compare(args.src, cut)
    sprite, info = normalize(cut, canvas=args.canvas, fill=args.fill,
                             no_orient=args.no_orient)
    sprite.save(args.out, "PNG")
    line = "%s -> %s | %s | %s" % (os.path.basename(args.src), args.out, args.method, stats)
    print(line)
    if args.report:
        with open(args.report, "w", encoding="utf-8") as f:
            json.dump({"src": args.src, "method": args.method, "stats": stats,
                       "orient": info}, f, ensure_ascii=False, indent=1)


if __name__ == "__main__":
    main()
