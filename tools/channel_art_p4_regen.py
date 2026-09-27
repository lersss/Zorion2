# channel_art_p4_regen.py — планета без кольца через ComfyUI (контроль: только круг диска).
import os, sys
import numpy as np
from PIL import Image, ImageDraw
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from channel_art_comfy import generate

OUT = r"C:\Zorion2\ai_drafts\channel_art"
W, H = 1024, 576
PCX, PCY, PR = 590, 333, 188

ctrl = Image.new("RGB", (W, H), (0, 0, 0))
ctrl_path = os.path.join(OUT, "p4_black_ctrl.png")
ctrl.save(ctrl_path)

POS = ("A single warm terrestrial planet, warm golden rocky surface, thin turquoise atmosphere "
       "glow along the limb, soft warm sunlight from the upper left, deep dark space background with "
       "faint stars, serene cinematic postcard, soft gradients, no ring, no rings, no belt, no text, "
       "no watermark, no letters")
NEG = ("text, letters, numbers, watermark, signature, logo, ring, rings, belt, band, "
       "multiple planets, blurry, low quality, clutter, 3d perspective")

generate(POS, NEG, ctrl_path, os.path.join(OUT, "p4_form_6006.png"),
         os.path.join(OUT, "p4_noring.png"),
         steps=40, cfg=7.0, denoise=0.72, cn_strength=0.30, seed=8113)
print("done")
