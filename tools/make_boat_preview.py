# -*- coding: utf-8 -*-
# Витрина пачки лодки прогулки (ЧК6.3). ТЗ: art_surface_boat.md §4.7.
# preview.html + preview_grid.png: каждый кандидат 6x (204x120) и в игровом размере
# (34x20 x ZOOM 1.8 = 61x36) на фонах 6 сред; метка линии воды; бокс игрока 12x28
# (стопы на lv-6); сравнительная полоса «кандидат | код-фолбэк».
import os
from PIL import Image, ImageDraw, ImageFont

ROOT = r'C:\Zorion2\ai_drafts\surface_boat'
SPR = os.path.join(ROOT, 'sprites')
OUT_HTML = os.path.join(ROOT, 'preview.html')
OUT_GRID = os.path.join(ROOT, 'preview_grid.png')

ZOOM = 1.8
WATER_LY = 14           # линия воды в логике (файл y=84 = 14*6)
PLAYER_W, PLAYER_H, PLAYER_FEET_LY = 12, 28, 8
GW, GH = int(round(34 * ZOOM)), int(round(20 * ZOOM))   # 61 x 36
WATER_GY = int(round(WATER_LY * ZOOM))                  # 25
PLAYER_GW, PLAYER_GH = int(round(PLAYER_W * ZOOM)), int(round(PLAYER_H * ZOOM))
PLAYER_FEET_G = int(round(PLAYER_FEET_LY * ZOOM))

# (подпись, ASCII-ключ файла, цвет) — ключ ASCII, чтобы имена файлов были безопасны
MEDIA = [
    ('вода', 'water', '#2a7fd0'), ('метан', 'methane', '#2fbfa8'),
    ('аммиак', 'ammonia', '#d98cc8'), ('CO2', 'co2', '#d8c04a'),
    ('тёмный', 'dark', '#05070f'), ('лава (не используется)', 'lava', '#ff6a1a'),
]
CAND = ['boat_%02d' % i for i in range(1, 9)]
BG = (10, 12, 20)
GOLD = (251, 191, 36)
DIM = (148, 163, 184)

# Фолбэк-геометрия (спека §3.5 / арт-док §6): те же цвета и габарит.
FB_TOP = [(4.1, 0.85), (5.6, 2.1), (7.8, 3.3), (10.4, 4.3), (13.2, 4.9), (15.6, 5.0),
          (17.0, 5.0), (19.0, 4.8), (21.6, 4.2), (24.4, 3.4), (27.0, 2.3),
          (29.6, 1.3), (31.4, 0.85), (32.6, 1.2), (33.6, 2.8), (33.9, 4.4)]
FB_BOTTOM = [(33.9, 4.4), (33.0, 7.4), (31.6, 10.8), (29.8, 14.0), (27.4, 16.8),
             (24.6, 18.6), (21.6, 19.5), (18.4, 19.7), (15.4, 19.5), (12.4, 18.9),
             (9.4, 17.9), (6.8, 16.7), (4.2, 15.2)]
FB_WELL = [(10.2, 7.6), (12.0, 7.1), (15.0, 7.0), (18.0, 7.0), (21.0, 7.2),
           (23.4, 7.8), (24.2, 8.9), (23.4, 11.4), (21.0, 12.3), (18.0, 12.6),
           (15.0, 12.5), (12.0, 11.9), (10.1, 10.3)]
FB_MOTOR = [(0.9, 3.0), (1.9, 2.3), (4.9, 2.5), (4.9, 13.8), (1.7, 13.5), (0.9, 12.2)]


def font(size):
    for name in ('arialbd.ttf', 'arial.ttf'):
        try:
            return ImageFont.truetype(name, size)
        except OSError:
            continue
    return ImageFont.load_default()


def hex2rgb(h):
    h = h.lstrip('#')
    return tuple(int(h[i:i + 2], 16) for i in (0, 2, 4))


def scaled(pts):
    return [(x * 6, y * 6) for x, y in pts]


def spr(name):
    return Image.open(os.path.join(SPR, name + '.png')).convert('RGBA')


def big_view(im):
    """6x как есть (204x120) + метка линии воды."""
    W, H, top = im.width, im.height + 20, 12
    canvas = Image.new('RGB', (W, H), (8, 10, 18))
    canvas.paste(im, (0, top), im)
    d = ImageDraw.Draw(canvas)
    wy = top + 84
    for x in range(0, W, 10):
        d.line([(x, wy), (min(x + 5, W), wy)], fill=(125, 211, 252), width=1)
    d.text((4, 0), 'вода y=84', fill=(125, 211, 252), font=font(11))
    return canvas


def game_tile(spr204, bg_hex, player=True):
    """Спрайт в игровом размере (61x36) на фоне среды, линия воды, бокс игрока."""
    W, H, gy = 120, 104, 70
    tile = Image.new('RGBA', (W, H), hex2rgb(bg_hex) + (255,))
    # затенение под водой
    sh = Image.new('RGBA', (W, H - gy), (0, 0, 0, 55))
    tile.alpha_composite(sh, (0, gy))
    top = gy - WATER_GY
    boat = spr204.resize((GW, GH), Image.LANCZOS)
    tile.alpha_composite(boat, ((W - GW) // 2, top))
    d = ImageDraw.Draw(tile)
    d.line([(0, gy), (W, gy)], fill=(255, 255, 255, 150), width=1)
    if player:
        cx = W // 2
        feet = top + PLAYER_FEET_G
        x0, x1 = cx - PLAYER_GW // 2, cx + PLAYER_GW // 2
        y0, y1 = feet - PLAYER_GH, feet
        d.rectangle([x0, y0, x1, y1], outline=(248, 250, 252, 230), width=2)
        d.line([(x0 + 1, y0 + 1), (x1 - 1, y0 + 1)], fill=(56, 189, 248, 230), width=2)
    return tile.convert('RGB')


def fallback_204():
    """Схематичный код-фолбэк (та же геометрия/палитра, спека §3.5). Для сравнения."""
    im = Image.new('RGBA', (204, 120), (0, 0, 0, 0))
    d = ImageDraw.Draw(im)
    body, gunw, floor, cont = (200, 100, 50), (240, 216, 192), (90, 51, 32), (63, 36, 21)
    hull = scaled(FB_TOP + FB_BOTTOM)
    d.polygon(hull, fill=body, outline=cont, width=9)
    d.line(scaled(FB_TOP), fill=gunw, width=8, joint='curve')
    d.polygon(scaled(FB_WELL), fill=floor, outline=cont, width=4)
    d.polygon(scaled(FB_MOTOR), fill=(72, 64, 58), outline=cont, width=5)
    return im


def build_grid():
    f = font(22)
    fsm = font(14)
    bw = 204
    view_h = 140
    tile_w, tile_h = 120, 104
    row_gap = 12
    row_h = max(view_h, tile_h + 20) + row_gap
    W = 44 + bw + 16 + len(MEDIA) * (tile_w + 6) + 90
    H = 62 + len(CAND) * row_h + 380
    canvas = Image.new('RGB', (W, H), BG)
    d = ImageDraw.Draw(canvas)
    d.text((20, 10), 'Лодка прогулки (ЧК6.3) — кандидаты 1–8: 6x и игровой размер (61x36)',
           fill=(125, 211, 252), font=f)
    d.text((20, 34), 'линия воды y=84 (файл) = y=25 (игра); бокс игрока 12x28, стопы на lv-6',
           fill=DIM, font=fsm)

    y = 62
    for i, name in enumerate(CAND):
        if not os.path.exists(os.path.join(SPR, name + '.png')):
            continue
        s = spr(name)
        d.text((20, y), str(i + 1), fill=GOLD, font=f)
        canvas.paste(big_view(s), (44, y))
        x = 44 + bw + 16
        for mname, mkey, mhex in MEDIA:
            canvas.paste(game_tile(s, mhex), (x, y))
            d.text((x, y + tile_h + 2), mname, fill=DIM, font=fsm)
            x += tile_w + 6
        y += row_h

    # сравнительная полоса: кандидат | код-фолбэк (игровой размер, фон вода)
    d.text((20, y + 4), 'Сравнение в игровом размере (x2): кандидат 1 | код-фолбэк (вода)',
           fill=(125, 211, 252), font=f)
    y += 34
    t = game_tile(spr(CAND[0]), '#2a7fd0', player=False)
    fbt = game_tile(fallback_204(), '#2a7fd0', player=False)
    canvas.paste(t.resize((t.width * 2, t.height * 2), Image.NEAREST), (44, y))
    canvas.paste(fbt.resize((fbt.width * 2, fbt.height * 2), Image.NEAREST),
                 (44 + t.width * 2 + 24, y))
    d.text((44, y + t.height * 2 + 6), 'кандидат 1 (игра)', fill=DIM, font=fsm)
    d.text((44 + t.width * 2 + 24, y + t.height * 2 + 6),
           'код-фолбэк (игра, эмуляция спеки §3.5)', fill=DIM, font=fsm)
    y += t.height * 2 + 34

    d.text((20, y), 'Фолбэк 6x (эмуляция геометрии/палитры спеки §3.5 — рисует @developer)',
           fill=(125, 211, 252), font=f)
    y += 26
    canvas.paste(big_view(fallback_204()), (44, y))
    y += view_h + 10

    canvas.crop((0, 0, W, y)).save(OUT_GRID)
    print('Saved:', OUT_GRID)


def build_html():
    cards = []
    for i, name in enumerate(CAND, 1):
        if not os.path.exists(os.path.join(SPR, name + '.png')):
            continue
        tiles = ''.join(
            "<figure class='tile' style='background:%s'><img src='_prev/%s_%s.png' "
            "width='%d' height='%d'><figcaption>%s</figcaption></figure>"
            % (mhex, name, mkey, GW, GH, mname)
            for mname, mkey, mhex in MEDIA)
        cards.append(
            "<div class='card'><div class='num'>%d</div>"
            "<div class='rowa'><img class='big6' src='sprites/%s.png'><div class='cap'>"
            "<b>%s</b><br>204&times;120 (6&times;)</div></div>"
            "<div class='rowb'>%s</div></div>" % (i, name, name, tiles))
    body = ''.join(cards)
    fb = "<div class='cmp'><div><img class='big6' src='_prev/fallback.png'><div class='cap'>фолбэк 6&times;</div></div>" \
         "<div><img src='_prev/boat_01_water.png' width='%d' height='%d' style='image-rendering:pixelated'>" \
         "<div class='cap'>кандидат 1 (игра)</div></div>" \
         "<div><img src='_prev/fallback_water.png' width='%d' height='%d' style='image-rendering:pixelated'>" \
         "<div class='cap'>фолбэк (игра, эмуляция §3.5)</div></div></div>" % (GW, GH, GW, GH)
    html = """<!DOCTYPE html><html><head><meta charset='utf-8'>
<title>Zorion — лодка прогулки (ЧК6.3), кандидаты 1–8</title>
<style>
 body{background:#080b14;color:#cbd5e1;font-family:Segoe UI,sans-serif;padding:24px}
 h1{color:#7dd3fc;margin:0 0 6px} h2{color:#a5b4fc;margin:28px 0 10px}
 .legend{color:#94a3b8;font-size:13px;max-width:1180px;line-height:1.55}
 .card{background:#0a0e17;border:1px solid #24344d;border-radius:10px;padding:10px;margin:10px 0}
 .num{display:inline-block;color:#fbbf24;font-weight:700;font-size:18px;margin-right:10px}
 .rowa{display:inline-block;vertical-align:top}
 .rowb{display:inline-flex;gap:8px;flex-wrap:wrap;margin-top:8px}
 .big6{width:306px;height:180px;background:#05070f;border-radius:6px;image-rendering:auto}
 .cap{color:#94a3b8;font-size:12px;margin-top:4px}
 .tile{margin:0;text-align:center;border-radius:8px;padding:0;overflow:hidden}
 .tile img{display:block;margin:8px auto 0;image-rendering:pixelated;background:transparent}
 .tile figcaption{color:#e2e8f0;font-size:11px;padding:2px 0 4px}
 .cmp{display:flex;gap:22px;align-items:flex-start;flex-wrap:wrap}
</style></head><body>
<h1>Лодка прогулки — кандидаты 1–8 (один силуэт, разные seed/denoise этапа 2)</h1>
<div class='legend'>
 Финал: <code>web/static/sprites/surface/boat/boat.png</code>, 204&times;120 RGBA, вода y=84,
 низ корпуса y=120, центр x=102. Слева — 6&times; «как есть»; справа — <b>игровой размер</b>
 (34&times;20 &times; ZOOM 1.8 = 61&times;36) на фонах сред; белая линия — уровень воды,
 белый бокс 12&times;28 — игрок (стопы на lv&minus;6), синяя риска — плечи/кант.
 <b>Лава — только тест читаемости, лодке не используется.</b><br>
 <b>Отбор:</b> пришлите номера (напр. «оставить 3, 7; выкинуть 1, 5») — финал соберу из одного.
</div>
<h2>Кандидаты</h2>
%s
<h2>Сравнение с код-фолбэком</h2>
%s
</body></html>""" % (body, fb)
    with open(OUT_HTML, 'w', encoding='utf-8') as fh:
        fh.write(html)
    print('Saved:', OUT_HTML)


def build_prev_pngs():
    """Мелкие PNG-тайлы для HTML (игровой размер по средам)."""
    out = os.path.join(ROOT, '_prev')
    os.makedirs(out, exist_ok=True)
    for name in CAND:
        if not os.path.exists(os.path.join(SPR, name + '.png')):
            continue
        s = spr(name)
        for mname, mkey, mhex in MEDIA:
            game_tile(s, mhex).save(os.path.join(out, '%s_%s.png' % (name, mkey)))
    fb = fallback_204()
    big_view(fb).save(os.path.join(out, 'fallback.png'))
    for mname, mkey, mhex in MEDIA:
        game_tile(fb, mhex, player=False).save(
            os.path.join(out, 'fallback_%s.png' % mkey))
    print('Saved: _prev/')


if __name__ == '__main__':
    build_prev_pngs()
    build_grid()
    build_html()
