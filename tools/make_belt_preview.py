# -*- coding: utf-8 -*-
# Превью-сетка массовой пачки астероидов пояса: preview.html + preview_grid.png.
# ТЗ: art_belt_asteroids.md §3.7. Все 16 спрайтов камня + 3 обломка + 4 паттерна руды.
import os
from PIL import Image, ImageDraw, ImageFont, ImageChops

ROOT = r'C:\Zorion2\ai_drafts\belt_asteroids'
SPR = os.path.join(ROOT, 'sprites')
SIL = os.path.join(ROOT, 'silhouettes')
VEIN = os.path.join(ROOT, 'veins')
DEMO = os.path.join(ROOT, 'veins_demo')
BG = (5, 7, 15)          # COLORS.bg сцены пояса
GLINT = (253, 230, 138)  # COLORS.glint
GOLD = (251, 191, 36)

ROCKS = [(str(i * 2 + 1), 'rock_%02da' % (i + 1), 'пыльная') for i in range(8)]
ROCKS += [(str(i * 2 + 2), 'rock_%02db' % (i + 1), 'трещиноватая') for i in range(8)]
ROCKS.sort(key=lambda x: int(x[0]))
FORM_NOTE = {1: 'округлая кратерная (пилот)', 3: 'пласты/углы', 5: 'гранёная, смягчённая (пилот)',
             7: 'удлинённая', 9: 'картофелина', 11: 'отколотая треть', 13: 'пористая', 15: 'сплюснутая'}
DEBRIS = [('D1', 'debris_01'), ('D2', 'debris_02'), ('D3', 'debris_03')]
VEINS = [('V1', 'vein_crack', 'прожилка'), ('V2', 'vein_nest', 'гнездо'),
         ('V3', 'vein_seam', 'шов'), ('V4', 'vein_speck', 'вкрапления')]


def font(size):
    for name in ('arialbd.ttf', 'arial.ttf'):
        try:
            return ImageFont.truetype(name, size)
        except OSError:
            continue
    return ImageFont.load_default()


def tinted_vein(rock, vein_path, alpha=0.9):
    """Руда (тинт glint) поверх камня, ОБРЕЗАННАЯ по альфе камня — как в игре."""
    rock = rock.convert('RGBA')
    v = Image.open(vein_path).convert('RGBA').resize(rock.size, Image.LANCZOS)
    clip = ImageChops.multiply(v.split()[3], rock.split()[3])
    colored = Image.new('RGBA', rock.size, GLINT + (0,))
    colored.putalpha(clip.point(lambda x: int(x * alpha)))
    out = rock.copy()
    out.alpha_composite(colored)
    return out


def build_vein_demos():
    os.makedirs(DEMO, exist_ok=True)
    rock = Image.open(os.path.join(SPR, 'rock_01a.png'))
    for _num, name, _v in VEINS:
        out = tinted_vein(rock, os.path.join(VEIN, name + '.png'))
        path = os.path.join(DEMO, 'rock_01a_%s.png' % name)
        out.save(path)
    print('vein demos:', len(VEINS))


def build_grid():
    cell, label_h = 300, 32
    scale_deb = 3
    W = cell * 4
    H = 4 * (cell + label_h) + 3 * (64 * scale_deb + label_h) + (cell + label_h)
    canvas = Image.new('RGB', (W, H), BG)
    d = ImageDraw.Draw(canvas)
    f = font(24)

    for i, (num, name, _var) in enumerate(ROCKS):
        x = (i % 4) * cell + (cell - 256) // 2
        y = (i // 4) * (cell + label_h) + label_h
        spr = Image.open(os.path.join(SPR, name + '.png')).convert('RGBA')
        canvas.paste(spr, (x, y), spr)
        d.text(((i % 4) * cell + 12, (i // 4) * (cell + label_h) + 4), num, fill=GOLD, font=f)

    y0 = 4 * (cell + label_h)
    for i, (num, name) in enumerate(DEBRIS):
        spr = Image.open(os.path.join(SPR, name + '.png')).convert('RGBA')
        spr = spr.resize((64 * scale_deb, 64 * scale_deb), Image.NEAREST)
        x = i * (64 * scale_deb + label_h) + 20
        canvas.paste(spr, (x, y0 + label_h), spr)
        d.text((x, y0 + 2), '%s (x%d)' % (num, scale_deb), fill=GOLD, font=f)

    base_rock = Image.open(os.path.join(SPR, 'rock_01a.png'))
    y0 += 64 * scale_deb + label_h
    for i, (num, name, _vn) in enumerate(VEINS):
        x = i * cell + (cell - 256) // 2
        demo = tinted_vein(base_rock, os.path.join(VEIN, name + '.png')).convert('RGBA')
        tile = Image.new('RGBA', (256, 256), BG + (255,))
        tile.alpha_composite(demo)
        canvas.paste(tile.convert('RGB'), (x, y0 + label_h))
        d.text((i * cell + 12, y0 + 4), num, fill=GOLD, font=f)

    path = os.path.join(ROOT, 'preview_grid.png')
    canvas.save(path)
    print('Saved:', path)


def build_html():
    rock_cards = ''.join(
        "<div class='card'><div class='num'>%s</div><img src='sprites/%s.png'>"
        "<div class='nm'>%s</div><div class='var'>%s</div></div>" % (n, s, s, v)
        for n, s, v in ROCKS)
    form_cards = ''.join(
        "<div class='card sm'><div class='num'>F%d</div><img src='silhouettes/rock_%02d.png'>"
        "<div class='nm'>%s</div></div>" % (i + 1, i + 1, FORM_NOTE.get(i * 2 + 1, ''))
        for i in range(8))
    deb_cards = ''.join(
        "<div class='card sm'><div class='num'>%s</div><img src='sprites/%s.png'>"
        "<div class='nm'>%s</div></div>" % (n, s, n) for n, s in DEBRIS)
    vein_cards = ''.join(
        "<div class='card sm'><div class='num'>%s</div><div class='veintile'>"
        "<img src='veins/%s.png'></div><div class='nm'>%s</div></div>" % (n, s, v)
        for n, s, v in VEINS)
    vein_tint = ''.join(
        "<div class='card sm'><div class='num'>%s</div>"
        "<img src='veins_demo/rock_01a_%s.png'><div class='nm'>%s + glint</div></div>"
        % (n, s, v) for n, s, v in VEINS)

    html = """<!DOCTYPE html><html><head><meta charset='utf-8'>
<title>Zorion — астероиды пояса, массовая пачка</title>
<style>
 body{background:#080b14;color:#cbd5e1;font-family:Segoe UI,sans-serif;padding:24px}
 h1{color:#7dd3fc;margin:0 0 6px} h2{color:#a5b4fc;margin:30px 0 10px}
 .legend{color:#94a3b8;font-size:13px;max-width:1150px;line-height:1.5}
 .grid{display:flex;gap:12px;flex-wrap:wrap;margin-top:10px}
 .card{position:relative;background:#0a0e17;border:1px solid #24344d;border-radius:10px;padding:8px;text-align:center}
 .card img{width:190px;height:190px;display:block}
 .card.sm img{width:120px;height:120px}
 .card.sm .veintile{width:120px;height:120px}
 .num{position:absolute;top:3px;left:9px;color:#fbbf24;font-weight:700;font-size:14px}
 .nm{color:#cbd5e1;font-size:12px;margin-top:5px}
 .var{color:#64748b;font-size:11px}
 .veintile{background:#0a0e17;border:1px dashed #334155;border-radius:8px}
 .veintile img{width:100%%;height:100%%}
 .scale{display:flex;align-items:flex-end;gap:16px;background:#0a0e17;border:1px solid #24344d;
        border-radius:10px;padding:14px;margin-top:10px}
 .scale figure{margin:0;text-align:center}
 .scale img{background:#05070f;border-radius:6px}
 .scale figcaption{color:#64748b;font-size:11px;margin-top:5px}
</style></head><body>
<h1>Астероиды пояса — массовая пачка (8 форм &times; 2 варианта + 3 обломка + слой руды)</h1>
<div class='legend'>
 <b>Вариант a</b> — «матовая пыльная» (светлее), <b>вариант b</b> — «трещиноватая/сколотая»
 (темнее, контрастнее). Слой руды — белый = руда, тинт и свечение задаёт код (<code>glint</code>).
 Обломки в игре рисуются 16–48 px (в сетке показаны &times;3). Все спрайты 256&times;256 (обломки 64&times;64),
 RGBA, прозрачный фон, без метаданных поворота.
</div>

<h2>16 спрайтов камня (1–16)</h2>
<div class='grid'>%s</div>

<h2>Формы-силуэты (F1–F8)</h2>
<div class='grid'>%s</div>

<h2>Мелкие обломки (D1–D3, 64&times;64)</h2>
<div class='grid'>%s</div>

<h2>Слой руды — 4 паттерна (V1–V4)</h2>
<div class='grid'>%s</div>

<h2>Слой руды на камне (тинт glint, обрезан по камню)</h2>
<div class='grid'>%s</div>

<h2>Камень в игровом масштабе на фоне пояса</h2>
<div class='scale'>
 <figure><img src='sprites/rock_01a.png' width='128'><figcaption>128px</figcaption></figure>
 <figure><img src='sprites/rock_01a.png' width='64'><figcaption>64px</figcaption></figure>
 <figure><img src='sprites/rock_02b.png' width='128'><figcaption>128px</figcaption></figure>
 <figure><img src='sprites/rock_02b.png' width='64'><figcaption>64px</figcaption></figure>
 <figure><img src='sprites/debris_01.png' width='48'><figcaption>48px (обломок)</figcaption></figure>
 <figure><img src='sprites/debris_01.png' width='16'><figcaption>16px (обломок)</figcaption></figure>
</div>
</body></html>""" % (rock_cards, form_cards, deb_cards, vein_cards, vein_tint)

    path = os.path.join(ROOT, 'preview.html')
    with open(path, 'w', encoding='utf-8') as fh:
        fh.write(html)
    print('Saved:', path)


if __name__ == '__main__':
    build_vein_demos()
    build_grid()
    build_html()
