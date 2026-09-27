# channel_art_twitch_sheet.py — panel_02 из принятого мастера + контактный лист и preview.html.
import os
from PIL import Image, ImageDraw, ImageFilter

DBG = r"C:\Zorion2\ai_drafts\channel_art"
TW = os.path.join(DBG, "twitch")
os.makedirs(TW, exist_ok=True)

# --- panel_02: принятый 768x432 -> 320x160 (кроп 2:1 + Lanczos) ---
src = Image.open(os.path.join(DBG, "final", "02_ai_agents.png")).convert("RGB")
W, H = src.size
h2 = W // 2
y0 = (H - h2) // 2
p2 = src.crop((0, y0, W, y0 + h2)).resize((320, 160), Image.LANCZOS)
# лёгкий шарпен, чтобы тонкие линии узлов/пунктира не размылись при сжатии
p2 = p2.filter(ImageFilter.UnsharpMask(radius=1.0, percent=55, threshold=2))
p2.save(os.path.join(TW, "panel_02_ai_agents.png"))
print("panel_02", p2.size)

# --- контактный лист 1:1 ---
panels = [os.path.join(TW, f) for f in
          ("panel_01_about_game.png", "panel_02_ai_agents.png",
           "panel_03_on_stream.png", "panel_04_support.png")]
avatar = os.path.join(TW, "avatar_256.png")
banner_path = os.path.join(TW, "offline_banner_1920x1080.png")

M, G, CAP = 24, 16, 18
pw, ph = 320, 160
Wc = M * 2 + 4 * pw + 3 * G
av = Image.open(avatar).convert("RGB")
bn = Image.open(banner_path).convert("RGB")
bn_w = Wc - M * 2 - G - av.width
bn = bn.resize((bn_w, int(round(bn_w * bn.height / bn.width))), Image.LANCZOS)
row1_h = CAP + ph
row2_h = max(av.height, CAP + bn.height)
Hc = M + row1_h + G + row2_h + M
sheet = Image.new("RGB", (Wc, Hc), (10, 15, 26))
d = ImageDraw.Draw(sheet)

for i, p in enumerate(panels):
    im = Image.open(p).convert("RGB")
    x = M + i * (pw + G)
    sheet.paste(im, (x, M))
    d.rectangle([x - 1, M - 1, x + pw, M + ph], outline=(30, 42, 68))
    d.text((x + 2, M + ph + 3), "%d  %dx%d" % (i + 1, im.width, im.height), fill=(150, 170, 200))

y2 = M + row1_h + G
sheet.paste(av, (M, y2))
d.rectangle([M - 1, y2 - 1, M + av.width, y2 + av.height], outline=(30, 42, 68))
d.text((M + 2, y2 + av.height + 3), "avatar 256x256", fill=(150, 170, 200))
bx = M + av.width + G
sheet.paste(bn, (bx, y2))
d.rectangle([bx - 1, y2 - 1, bx + bn.width, y2 + bn.height], outline=(30, 42, 68))
d.text((bx + 2, y2 + bn.height + 3), "offline 1920x1080 (scaled)", fill=(150, 170, 200))

sheet.save(os.path.join(TW, "contact_sheet.png"))
print("contact_sheet.png", sheet.size)

titles = ["1. Об игре", "2. Космос, который пишет ИИ", "3. Что происходит на стриме", "4. Поддержать разработку"]
html = """<!doctype html><meta charset=utf-8><title>Twitch channel art</title>
<body style="background:#0A0F1A;color:#C8D6E5;font-family:Segoe UI,sans-serif;margin:24px">
<h2>Zorion - оформление Twitch (черновик)</h2>
<p>Панели 320x160 (1:1), аватар 256x256, заставка оффлайна 1920x1080.</p>
<div style="display:flex;gap:16px;flex-wrap:wrap">
""" + "".join(
    f'<figure style="margin:0"><img src="{os.path.basename(p)}" style="width:{pw}px;height:{ph}px;'
    f'border:1px solid #1E2A44;border-radius:4px"><figcaption>{t}</figcaption></figure>'
    for p, t in zip(panels, titles)) + """
</div>
<h3>Аватар</h3><img src="avatar_256.png" style="border:1px solid #1E2A44;border-radius:8px">
<h3>Заставка оффлайна (уменьшена)</h3>
<img src="offline_banner_1920x1080.png" style="max-width:1200px;width:100%;border:1px solid #1E2A44;border-radius:8px">
</body>"""
with open(os.path.join(TW, "preview.html"), "w", encoding="utf-8") as fh:
    fh.write(html)
print("preview.html")
