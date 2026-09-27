# channel_art_gen.py — генерация панелей 2 (граф) и 4 (планета в кольце) через ComfyUI.
import os, sys, time
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from channel_art_comfy import generate

OUT = r"C:\Zorion2\ai_drafts\channel_art"

P2_POS = ("Technical orbital navigation chart, thin engineering grid, one large elliptical orbit "
          "line, five glowing nodes on the orbit connected by dashed trajectories, dark navy space "
          "background, cyan and amber glowing lines, minimal blueprint aesthetic, subtle bloom, flat, "
          "no text, no letters, no numbers, no watermark, dark space game UI")
P2_NEG = "text, letters, numbers, watermark, signature, logo, photo, clutter, 3d perspective"

P4_POS = ("A single warm terrestrial planet with a thin ring, soft turquoise atmosphere glow along "
          "the limb, warm golden sunlight from the edge of frame, deep dark space background with "
          "faint stars, tiny glowing sparks orbiting along the ring, serene cinematic postcard, soft "
          "gradients, no text, no watermark, no letters")
P4_NEG = ("text, letters, numbers, watermark, signature, logo, photo, clutter, 3d perspective, "
          "blurry, low quality, multiple planets")

jobs = [
    ("p2_raw_a.png", P2_POS, P2_NEG, os.path.join(OUT, "p2_schematic.png"), os.path.join(OUT, "p2_schematic.png"), 0.60, 1.35, 1001),
    ("p2_raw_b.png", P2_POS, P2_NEG, os.path.join(OUT, "p2_schematic.png"), os.path.join(OUT, "p2_schematic.png"), 0.60, 1.35, 2002),
    ("p4_raw_a.png", P4_POS, P4_NEG, os.path.join(OUT, "p4_control.png"), os.path.join(OUT, "p4_latent.png"), 0.52, 1.25, 3001),
]

for name, pos, neg, ctrl, lat, den, stg, seed in jobs:
    t = time.time()
    out = os.path.join(OUT, name)
    print("=== %s seed=%d denoise=%.2f ===" % (name, seed, den))
    generate(pos, neg, ctrl, lat, out, steps=40, cfg=7.0, denoise=den, cn_strength=stg, seed=seed)
    print("   %.1fs" % (time.time() - t))
