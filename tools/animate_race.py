# -*- coding: utf-8 -*-
# animate_race.py — ПРОЦЕДУРНАЯ АНИМАЦИЯ аватара расы (эксперимент).
# Из статичного 200x200 RGBA делает GIF: пульс свечения + лёгкое покачивание.
# НЕ игровой код — арт-тулза для проверки «жизни» форм.
#
# Как определять, что анимировать (правило для релиза):
#   1. ПУЛЬС СВЕЧЕНИЯ: яркие пиксели (глаза, жилы, ячейки, ядра, трещины) — их яркость
#      модулируется синусом. Работает если в арте есть «свет» (glow-маска).
#   2. ПОКАЧИВАНИЕ (bob): весь аватар плавно дышит ±N px по вертикали. Даёт «живость»
#      как портреты в Star Control. Для «посаженных» аватаров слабый (база не отрывается).
#   3. ДРЕЙФ ЧАСТЕЙ: для составных форм (рой, соты, пыль) — отдельные элементы можно
#      двигать. Определяется программно: разбить маску на компоненты, мелкие сдвигать.
#
# Использование: python animate_race.py <in.png> <out.gif> [--bob 3] [--glow 0.4] [--frames 24]
import sys
import numpy as np
from PIL import Image

BOB = 3        # амплитуда покачивания, px
GLOW = 0.4     # сила пульса свечения (0..1)
FRAMES = 24    # кадров (для 30 fps = 0.8 c цикла)
THRESH = 140   # порог яркости «светящегося» пикселя (0..255)


def glow_mask(arr):
    """Маска светящихся пикселей: яркие (R+G+B)/3 > THRESH."""
    rgb = arr[:, :, :3].astype(np.int16)
    return (rgb.mean(axis=2) > THRESH)


def pulse_glow(arr, mask, t):
    """Модуляция яркости светящихся пикселей: *(1 + GLOW*sin(t))."""
    out = arr.copy().astype(np.float32)
    if mask.sum() == 0:
        return out.astype(np.uint8)
    factor = 1.0 + GLOW * np.sin(t)
    out[mask, :3] *= factor
    return np.clip(out, 0, 255).astype(np.uint8)


def make_frames(img, bob, glow, frames):
    arr = np.array(img)
    h, w = arr.shape[:2]
    mask = glow_mask(arr)
    out = []
    for i in range(frames):
        t = 2 * np.pi * i / frames
        # пульс свечения
        f = pulse_glow(arr, mask, t)
        # покачивание
        dy = int(round(bob * np.sin(t)))
        canvas = np.zeros_like(f)
        if dy >= 0:
            canvas[dy:, :] = f[:h - dy, :]
        else:
            canvas[:h + dy, :] = f[-dy:, :]
        out.append(Image.fromarray(canvas, 'RGBA'))
    return out


def main():
    if len(sys.argv) < 3:
        print("Использование: python animate_race.py <in.png> <out.gif> [--bob 3] [--glow 0.4] [--frames 24]")
        return
    inp, outp = sys.argv[1], sys.argv[2]
    bob, glow, frames = BOB, GLOW, FRAMES
    if '--bob' in sys.argv: bob = float(sys.argv[sys.argv.index('--bob') + 1])
    if '--glow' in sys.argv: glow = float(sys.argv[sys.argv.index('--glow') + 1])
    if '--frames' in sys.argv: frames = int(sys.argv[sys.argv.index('--frames') + 1])

    img = Image.open(inp).convert('RGBA')
    frames_list = make_frames(img, bob, glow, frames)
    frames_list[0].save(outp, save_all=True, append_images=frames_list[1:],
                        duration=1000 // 30, loop=0, disposal=2)
    print("Saved: %s (%d frames, bob=%d, glow=%.1f)" % (outp, len(frames_list), bob, glow))


if __name__ == '__main__':
    main()