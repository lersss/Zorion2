# planet_redmap.py — карта «красноты» (r - b) и яркости в центре диска: артефакты?
import sys
import numpy as np
from PIL import Image

im = Image.open(sys.argv[1]).convert("RGB")
a = np.array(im).astype(int)
r, g, b = a[:, :, 0], a[:, :, 1], a[:, :, 2]
redness = r - b
bright = a.sum(axis=2)

h, w = a.shape[:2]
cx, cy = w // 2, h // 2
R = 400

# Средняя краснота в центральном круге
mask = np.zeros((h, w), bool)
yy, xx = np.ogrid[:h, :w]
mask = (xx - cx) ** 2 + (yy - cy) ** 2 <= R * R
print(f"center red mean: {redness[mask].mean():.1f} (r-b)")
print(f"center bright mean: {bright[mask].mean():.0f}")

# Квадранты диска
for name, (yc, xc) in {"up-left": (cy - R // 2, cx - R // 2), "up-right": (cy - R // 2, cx + R // 2),
                        "down-left": (cy + R // 2, cx - R // 2), "down-right": (cy + R // 2, cx + R // 2)}.items():
    m = (xx - xc) ** 2 + (yy - yc) ** 2 <= (R // 2) ** 2
    rr = redness[m].mean()
    br = bright[m].mean()
    # доминирующий цвет
    rb = (b[m] - r[m]).mean()
    gr = (g[m] - r[m]).mean()
    dom = "blue" if rb > 15 and rb > gr else ("green" if gr > 15 else ("red" if rr > 25 else "mixed/dark"))
    print(f"  {name}: red={rr:.0f} bright={br:.0f} dom={dom}")

# Глобально: % пикселей очень красных в диске
red_mask = mask & (redness > 40)
print(f"strong red pixels in disk: {red_mask.sum()} ({red_mask.sum()/mask.sum():.2%})")