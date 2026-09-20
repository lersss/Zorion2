# analyze_planet.py — процедурная проверка картинки планеты (размер, диск в центре, палитра).
# Запуск: python tools/analyze_planet.py <path.png>
import sys
from PIL import Image

def analyze(path):
    im = Image.open(path).convert("RGB")
    w, h = im.size
    print(f"size: {w}x{h}")
    px = im.load()
    # Фон по углам (космос)
    corners = [px[2, 2], px[w - 3, 2], px[2, h - 3], px[w - 3, h - 3]]
    bg = tuple(sum(c[i] for c in corners) // 4 for i in range(3))
    print(f"bg(corners): {bg}")
    # Центральная область — есть ли «планета» (ярче фона)
    cx, cy = w // 2, h // 2
    r = min(w, h) // 4
    bright = 0
    total = 0
    for y in range(cy - r, cy + r, 4):
        for x in range(cx - r, cx + r, 4):
            p = px[x, y]
            total += 1
            if sum(p) > sum(bg) + 60:
                bright += 1
    print(f"center disk bright ratio: {bright}/{total} = {bright / max(total,1):.2f}")
    # Палитра по зонам (сэмпл внутри диска)
    counts = {"blue": 0, "green": 0, "white": 0, "tan": 0, "dark": 0}
    total = 0
    for y in range(cy - r, cy + r, 4):
        for x in range(cx - r, cx + r, 4):
            p = px[x, y]
            s = sum(p)
            if s < 100:
                counts["dark"] += 1
            elif p[2] > p[0] + 20 and p[2] > p[1] + 10:
                counts["blue"] += 1
            elif p[1] > p[0] + 15 and p[1] > p[2] + 10:
                counts["green"] += 1
            elif p[0] > 200 and p[1] > 200 and p[2] > 200:
                counts["white"] += 1
            elif p[0] > p[1] + 20 and p[0] > p[2] + 20:
                counts["tan"] += 1
            else:
                counts["dark"] += 1
            total += 1
    for k, v in counts.items():
        print(f"  {k}: {v / max(total,1):.2%}")

if __name__ == "__main__":
    analyze(sys.argv[1])