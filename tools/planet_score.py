# planet_score.py — оценка картинки планеты: близость диска к центру, радиус в кадре,
# наличие синих океанов и зелёных континентов. Вывод: "score info"
import sys, math
from PIL import Image

im = Image.open(sys.argv[1]).convert("RGB")
w, h = im.size
px = im.load()

# Центр масс ярких пикселей
pts = []
step = 4
for y in range(0, h, step):
    for x in range(0, w, step):
        if sum(px[x, y]) > 300:
            pts.append((x, y))
if not pts:
    print("-1 empty"); sys.exit()

cx = sum(p[0] for p in pts) / len(pts)
cy = sum(p[1] for p in pts) / len(pts)
maxr = max(((p[0]-cx)**2 + (p[1]-cy)**2) ** 0.5 for p in pts)

# Отклонение центра от (512,512)
off = math.hypot(cx - 512, cy - 512)
# Радиус: хотим 280-430 (диск в кадре, не на весь кадр)
r_score = 0
if 260 <= maxr <= 440:
    r_score = 1.0
elif 200 <= maxr <= 500:
    r_score = 0.6
# Палитра внутри диска
counts = {"blue": 0, "green": 0, "white": 0, "dark": 0}
total = 0
step2 = 5
for y in range(int(cy - maxr), int(cy + maxr), step2):
    for x in range(int(cx - maxr), int(cx + maxr), step2):
        if 0 <= x < w and 0 <= y < h and (x-cx)**2 + (y-cy)**2 <= maxr*maxr:
            p = px[x, y]
            s = sum(p)
            total += 1
            if p[2] > p[0] + 25 and p[2] > p[1] + 15:
                counts["blue"] += 1
            elif p[1] > p[0] + 20 and p[1] > p[2] + 15:
                counts["green"] += 1
            elif p[0] > 200 and p[1] > 200 and p[2] > 200:
                counts["white"] += 1
            elif s < 120:
                counts["dark"] += 1

blue_r = counts["blue"] / max(total, 1)
green_r = counts["green"] / max(total, 1)
white_r = counts["white"] / max(total, 1)
dark_r = counts["dark"] / max(total, 1)

# Оценка: центр в кадре (макс 40), радиус (макс 20), биомы (макс 40)
score = 40 * max(0, 1 - off / 400) + 20 * r_score
score += 30 * min(blue_r / 0.2, 1.0) + 30 * min(green_r / 0.08, 1.0)
score -= 15 * max(0, dark_r - 0.5)

info = f"center=({cx:.0f},{cy:.0f}) off={off:.0f} r={maxr:.0f} blue={blue_r:.2%} green={green_r:.2%} white={white_r:.2%} dark={dark_r:.2%}"
print(f"{score:.1f} {info}")