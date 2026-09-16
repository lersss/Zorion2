# -*- coding: utf-8 -*-
# Обработка аватара расы С ФОНОМ СРЕДЫ (вариант «телевизор связи», Star Control):
# фон НЕ вырезается — среда (дым, источники) остаётся частью арта.
# Только кадрирование по контенту + вписывание в квадрат.
# Использование: python process_race_bg.py <in.png> <out.png> [size=512]
import sys
from PIL import Image, ImageOps
import numpy as np

DEFAULT_SIZE = 512
PAD = 12


def crop_to_content(img):
    arr = np.array(img.convert('RGB')).astype(np.int16)
    h, w, _ = arr.shape
    # фон — цвет углов; контент = всё, что заметно отличается
    bg = np.array([arr[2, 2], arr[2, w - 3], arr[h - 3, 2], arr[h - 3, w - 3]]).mean(axis=0)
    diff = np.abs(arr - bg).sum(axis=2)
    mask = diff > 60
    if mask.sum() == 0:
        return img
    ys, xs = np.where(mask)
    l, t, r, b = xs.min(), ys.min(), xs.max(), ys.max()
    l = max(0, l - PAD); t = max(0, t - PAD); r = min(w, r + PAD); b = min(h, b + PAD)
    return img.crop((l, t, r, b))


def main(in_path, out_path, size):
    img = Image.open(in_path)
    img = crop_to_content(img)
    img = ImageOps.pad(img, (size, size), color=(0, 0, 0), centering=(0.5, 0.5))
    img.save(out_path, 'PNG')
    print('Saved: %s (%dx%d)' % (out_path, img.size[0], img.size[1]))


if __name__ == '__main__':
    size = int(sys.argv[3]) if len(sys.argv) > 3 else DEFAULT_SIZE
    main(sys.argv[1], sys.argv[2], size)