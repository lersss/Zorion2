# -*- coding: utf-8 -*-
# Обработка кораблей Juggernaut XL: вырезание фона ПО ЦВЕТУ (фон ровный тёмный ~21,23,26),
# кадрирование, вписывание в 200x200, нормализация ориентации (нос вправо), сглаживание границ.
# Использование: python process_ship.py <in.png> <out.png> [--no-orient] [--smooth N] [--canvas N]
#                [--bg-dark N] [--keep-warm] [--keep-parts]
import sys
from PIL import Image, ImageOps
import numpy as np
from scipy import ndimage

CANVAS = 200
PAD = 6
BG_TOL = 40  # допуск расстояния до цвета фона

# --canvas N: финальный размер кандидата (200 — полный, 100 — эскиз 98c:
# быстрее и легче, ловит форму/стиль до полного прогона).
if '--canvas' in sys.argv:
    i = sys.argv.index('--canvas')
    CANVAS = int(sys.argv[i + 1])

NO_ORIENT = '--no-orient' in sys.argv
SMOOTH = 2
# --bg-dark N: вырезать фон ПО ЯРКОСТИ (всё, что темнее порога N) вместо цвета углов.
# Нужно для живописных аватаров, где фон — тёмный градиент, а не ровный цвет.
BG_DARK = None
if '--bg-dark' in sys.argv:
    i = sys.argv.index('--bg-dark')
    BG_DARK = int(sys.argv[i + 1])

# --keep-warm: НЕ вырезать «тёплые» пиксели (кожа/лицо), даже если близки к фону.
# Решает вырезание одежды цвета фона у портретов людей.
KEEP_WARM = '--keep-warm' in sys.argv

# --keep-parts: сохранять не только крупнейший компонент, но и отделённые тёмным
# швом детали (крыло/спутник/модуль): площадь >= 8 % крупнейшего и центр в
# пределах 0.6 высоты его bbox. По умолчанию выключено (студия зовёт без флага).
KEEP_PARTS = '--keep-parts' in sys.argv
KEEP_PARTS_MIN_SHARE = 0.08
KEEP_PARTS_MARGIN = 0.6


def is_warm_rgb(r, g, b):
    """Тёплый тон (кожа/лицо): R > B и R заметно доминирует. Массив-безопасно."""
    return (r - b > 18) & (r > 60)


def estimate_background(arr):
    """Робастная модель фона: квадратичная поверхность (плоскость/градиент/
    виньетка), подогнанная по периметру кадра (внешние 5 % ширины/высоты).
    Ровный тёмный фон даёт почти константу — результат совпадает со старой
    оценкой по углам; светлый неоднородный фон (градиент) описывается
    поверхностью, и вырез не «съедает» светлый корпус. Двухпроходный
    МНК-фит с отбрасыванием 15 % выбросов (детали корпуса, задевшие рамку)."""
    h, w, _ = arr.shape
    m = max(4, int(0.05 * min(h, w)))
    ring = np.zeros((h, w), bool)
    ring[:m, :] = ring[-m:, :] = True
    ring[:, :m] = ring[:, -m:] = True
    ys, xs = np.nonzero(ring)
    xn = (xs - w * 0.5) / (w * 0.5)
    yn = (ys - h * 0.5) / (h * 0.5)
    terms = np.column_stack([np.ones(ys.size), xn, yn, xn * xn, yn * yn, xn * yn])
    coef = np.zeros((3, 6))
    for ch in range(3):
        y = arr[:, :, ch][ring].astype(np.float64)
        c, *_ = np.linalg.lstsq(terms, y, rcond=None)
        res = np.abs(y - terms @ c)
        keep = res <= np.percentile(res, 85)
        if keep.sum() >= 12:
            c, *_ = np.linalg.lstsq(terms[keep], y[keep], rcond=None)
        coef[ch] = c
    xs_all = np.arange(w, dtype=np.float64)
    ys_all = np.arange(h, dtype=np.float64)
    xn2 = ((xs_all - w * 0.5) / (w * 0.5))[None, :]
    yn2 = ((ys_all - h * 0.5) / (h * 0.5))[:, None]
    plane = np.empty((h, w, 3), np.float32)
    for ch in range(3):
        a, b, cc, d, e, f = coef[ch]
        plane[:, :, ch] = (a + b * xn2 + cc * yn2 + d * xn2 * xn2 + e * yn2 * yn2 + f * xn2 * yn2).astype(np.float32)
    return plane


def remove_bg_by_color(img):
    """Всё, что близко к цвету фона (ровный/градиентный), -> прозрачное."""
    arr = np.array(img.convert('RGB')).astype(np.int16)
    plane = estimate_background(arr)
    diff = np.abs(arr.astype(np.float32) - plane).sum(axis=2)
    mask = diff <= BG_TOL
    out = np.array(img.convert('RGBA'))
    out[mask] = (0, 0, 0, 0)
    return Image.fromarray(out)


def remove_bg_by_dark(img, threshold):
    """Всё, что темнее порога яркости, -> прозрачное (фон-градиент)."""
    arr = np.array(img.convert('RGB')).astype(np.int16)
    brightness = arr.mean(axis=2)
    mask = brightness <= threshold
    if KEEP_WARM:
        r, g, b = arr[:, :, 0], arr[:, :, 1], arr[:, :, 2]
        mask = mask & ~is_warm_rgb(r, g, b)
    out = np.array(img.convert('RGBA'))
    out[mask] = (0, 0, 0, 0)
    return Image.fromarray(out)


def remove_bg_with_keep_warm(img):
    """Как remove_bg_by_color, но НЕ вырезает тёплые пиксели (кожу/лицо)."""
    arr = np.array(img.convert('RGB')).astype(np.int16)
    plane = estimate_background(arr)
    diff = np.abs(arr.astype(np.float32) - plane).sum(axis=2)
    mask = diff <= BG_TOL
    if KEEP_WARM:
        r, g, b = arr[:, :, 0], arr[:, :, 1], arr[:, :, 2]
        mask = mask & ~is_warm_rgb(r, g, b)
    out = np.array(img.convert('RGBA'))
    out[mask] = (0, 0, 0, 0)
    return Image.fromarray(out)


def keep_largest_component(img, keep_parts=False):
    """Оставить только крупнейший связный компонент, остальное -> прозрачное.
    С keep_parts=True дополнительно оставляются детали, отделённые тёмным швом
    (крыло/спутник/модуль): площадь >= 8 % крупнейшего и центр в пределах
    0.6 высоты его bbox. Мелкий «мусор» и далёкие обрывки вырезаются как раньше."""
    arr = np.array(img.convert('RGBA'))
    alpha = arr[:, :, 3]
    mask = alpha > 40
    if mask.sum() == 0:
        return img
    labels, num = ndimage.label(mask)
    if num <= 1:
        return img
    counts = np.bincount(labels.ravel())
    counts[0] = 0
    biggest = int(np.argmax(counts))
    keep = labels == biggest
    if keep_parts:
        ys, xs = np.where(keep)
        x0, y0, x1, y1 = xs.min(), ys.min(), xs.max(), ys.max()
        margin = KEEP_PARTS_MARGIN * (y1 - y0)
        threshold = KEEP_PARTS_MIN_SHARE * counts[biggest]
        for lb in range(1, num + 1):
            if lb == biggest or counts[lb] < threshold:
                continue
            cy, cx = ndimage.center_of_mass(mask, labels, lb)
            if x0 - margin <= cx <= x1 + margin and y0 - margin <= cy <= y1 + margin:
                keep = keep | (labels == lb)
    arr[~keep] = (0, 0, 0, 0)
    return Image.fromarray(arr)


def smooth_edges(img, radius=2):
    """Сгладить границы силуэта: закрытие (дыры) + открытие (зубцы)."""
    arr = np.array(img.convert('RGBA'))
    alpha = arr[:, :, 3]
    mask = alpha > 40
    if mask.sum() == 0:
        return img
    closed = ndimage.binary_closing(mask, structure=np.ones((radius * 2 + 1, radius * 2 + 1)))
    opened = ndimage.binary_opening(closed, structure=np.ones((radius + 1, radius + 1)))
    arr[:, :, 3] = np.where(opened, 255, 0)
    return Image.fromarray(arr)


def crop_to_content(img):
    bbox = img.getbbox()
    if bbox:
        return img.crop(bbox)
    return img


def normalize_orientation(img):
    """Развернуть корабль: горизонтальный, нос вправо."""
    arr = np.array(img.convert('RGBA'))
    alpha = arr[:, :, 3]
    ys, xs = np.where(alpha > 40)
    if len(xs) == 0:
        return img
    w = xs.max() - xs.min()
    h = ys.max() - ys.min()
    if w < h:
        img = img.rotate(-90, expand=True)
        arr = np.array(img.convert('RGBA'))
        alpha = arr[:, :, 3]
        ys, xs = np.where(alpha > 40)
    left = alpha[:, :xs.min() + (xs.max() - xs.min()) // 3].sum()
    right = alpha[:, xs.min() + 2 * (xs.max() - xs.min()) // 3:].sum()
    if left < right:
        img = ImageOps.mirror(img)
    return img


def fit_canvas(img, canvas=CANVAS):
    return ImageOps.pad(img, (canvas, canvas), color=(0, 0, 0, 0), centering=(0.5, 0.5))


def main(in_path, out_path):
    img = Image.open(in_path)
    if BG_DARK is not None:
        img = remove_bg_by_dark(img, BG_DARK)
    elif KEEP_WARM:
        img = remove_bg_with_keep_warm(img)
    else:
        img = remove_bg_by_color(img)
    img = keep_largest_component(img, keep_parts=KEEP_PARTS)
    img = crop_to_content(img)
    bbox = img.getbbox()
    if bbox:
        l, t, r, b = bbox
        img = img.crop((max(0, l - PAD), max(0, t - PAD), min(img.width, r + PAD), min(img.height, b + PAD)))
    if not NO_ORIENT:
        img = normalize_orientation(img)
    img = smooth_edges(img, SMOOTH)
    img = fit_canvas(img, CANVAS)
    img.save(out_path, 'PNG')
    print('Saved: %s (%dx%d)' % (out_path, img.size[0], img.size[1]))


if __name__ == '__main__':
    main(sys.argv[1], sys.argv[2])