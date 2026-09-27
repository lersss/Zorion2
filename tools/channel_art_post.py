# channel_art_post.py — пост-обработка панелей 2 и 4, сборка contact sheet.
import os, sys, math
import numpy as np
from PIL import Image, ImageDraw
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from channel_art_common import (grade_space, vignette, add_stars, glow_points, PAL,
                                save_contact_sheet, MASTER)

OUT = r"C:\Zorion2\ai_drafts\channel_art"

# ---------- Панель 2: граф агентов ----------
p2 = Image.open(os.path.join(OUT, "p2_raw_b.png")).convert("RGB").resize(MASTER, Image.LANCZOS)
a = np.array(p2).astype(float)
a = grade_space(a, ambient=0.08)
a = vignette(a, strength=0.42, power=2.4)
Image.fromarray(np.clip(a, 0, 255).astype(np.uint8)).save(os.path.join(OUT, "02_ai_agents.png"))
print("saved 02_ai_agents.png")

# ---------- Панель 4: планета в кольце + искры ----------
p4 = Image.open(os.path.join(OUT, "p4_form_6006.png")).convert("RGB").resize(MASTER, Image.LANCZOS)
a = np.array(p4).astype(float)
a = grade_space(a, ambient=0.10)

# искры-агенты вдоль кольца (координаты выверены по сетке на 768x432)
img = Image.fromarray(np.clip(a, 0, 255).astype(np.uint8))

# точки прочитаны по видимому кольцу: левый/задний край, фронт через планету, правый край
agents = [(92, 104, 0.9), (140, 82, 1.0), (232, 196, 0.8), (300, 300, 1.05),
          (420, 338, 0.9), (540, 336, 1.0), (645, 320, 0.85), (700, 290, 0.95)]
humans = [(450, 116, 1.05), (578, 190, 0.95)]
img = glow_points(img, agents, PAL["agent"], radius=2.4, intensity=1.0)
img = glow_points(img, humans, PAL["human"], radius=2.7, intensity=1.0)

arr = np.array(img).astype(float)
arr = vignette(arr, strength=0.52, power=2.1)
Image.fromarray(np.clip(arr, 0, 255).astype(np.uint8)).save(os.path.join(OUT, "04_support.png"))
print("saved 04_support.png")

# ---------- Contact sheet + preview.html ----------
files = [os.path.join(OUT, f) for f in
         ("01_about_game.png", "02_ai_agents.png", "03_on_stream.png", "04_support.png")]
sheet = save_contact_sheet(files, os.path.join(OUT, "contact_sheet.png"), width=320)
print("contact sheet", Image.open(sheet).size)

titles = ["1. Об игре", "2. Космос, который пишет ИИ", "3. Что происходит на стриме", "4. Поддержать разработку"]
html = """<!doctype html><meta charset=utf-8><title>channel art preview</title>
<body style="background:#0A0F1A;color:#C8D6E5;font-family:Segoe UI,sans-serif;margin:24px">
<h2>Zorion — оформление канала (черновик)</h2>
<p>Мастер 768x432, безопасная зона 640x360. Превью ниже — реальная пропорция.</p>
<img src="contact_sheet.png" style="max-width:1400px;width:100%;border:1px solid #1E2A44;border-radius:8px">
<div style="display:flex;gap:16px;margin-top:16px">
""" + "".join(
    f'<figure style="margin:0;flex:1"><img src="{os.path.basename(f)}" style="width:100%;border:1px solid #1E2A44;'
    f'border-radius:6px"><figcaption style="margin-top:6px">{t}</figcaption></figure>'
    for f, t in zip(files, titles)) + """
</div></body>"""
with open(os.path.join(OUT, "preview.html"), "w", encoding="utf-8") as fh:
    fh.write(html)
print("preview.html")
