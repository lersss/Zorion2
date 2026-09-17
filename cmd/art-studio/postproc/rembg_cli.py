# -*- coding: utf-8 -*-
# rembg_cli.py — вырезка фона через rembg (isnet-general-use).
# Вызов: python rembg_cli.py <in> <out>
# Открывает RGB, remove(...), сохраняет прозрачный PNG (спека 67a.1 §7.1).
import sys
from rembg import remove, new_session
from PIL import Image


def main():
    in_path, out_path = sys.argv[1], sys.argv[2]
    img = Image.open(in_path).convert("RGB")
    sess = new_session("isnet-general-use")
    res = remove(img, session=sess)
    res.save(out_path)


if __name__ == "__main__":
    main()