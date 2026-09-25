# -*- coding: utf-8 -*-
# Превью ПУЛА спрайтов мини-игры «Прокладка маршрута» (направление A).
# ТЗ: docs/specs/2026-09-25-маршрут-мини-игра-интерфейс.md §3.7/§4.
# Собирает контактный лист (pool_grid.png), мини-сцену (pool_demo.png) и страницу
# отбора (pool.html) по именам пулов web/static/sprites/route/<тип>_NN.png.
# Звезда — белый luminance (тинт кодом), ЧД/туманность/фактура — grayscale;
# маяк/ложный — нейтральный металл. Колец захвата в PNG нет (их рисует код).
import os
from PIL import Image, ImageDraw, ImageFont

POOL = r'C:\Zorion2\web\static\sprites\route'
OUT = os.path.join(POOL, '_preview')
BG = (5, 7, 15)
TILE = (11, 15, 26)
GOLD = (251, 191, 36)
CYAN = (125, 211, 252)
LAV = (165, 180, 252)
TEXT = (203, 213, 225)
MUTE = (148, 163, 184)

# Палитра map.starColors (web/static/js/config.js) — только для демо тинта.
STAR_TINTS = [('O', '#9bb0ff'), ('G', '#ffd700'), ('K', '#ffa500'),
              ('M', '#ff6348'), ('WD', '#f0f0f0'), ('ns', '#a0d8ef')]
NEB_TINTS = [('индиго', '#6366f1'), ('бирюза', '#22d3ee'), ('пурпур', '#c026d3')]
HAZ_TINT = '#7dd3fc'

TYPES = [
    ('star_core', 5, 'ядро звезды-ФИНИШ', 'белый luminance, 512×512'),
    ('black_hole', 3, 'ФИНИШ-ЧД', 'grayscale, 512×512'),
    ('beacon', 4, 'обязательный маяк', 'металл, 128×128'),
    ('false_signal', 4, 'ложный сигнал', 'металл, 128×128'),
    ('hazard_cloud', 3, 'фактура опасной зоны', 'grayscale-alpha, 512×512'),
    ('nebula_bg', 3, 'фоновая туманность', 'grayscale-alpha, 1024×1024'),
]


def hexc(h):
    h = h.lstrip('#')
    return tuple(int(h[i:i + 2], 16) for i in (0, 2, 4))


def font(size):
    for name in ('arialbd.ttf', 'arial.ttf'):
        try:
            return ImageFont.truetype(name, size)
        except OSError:
            continue
    return ImageFont.load_default()


def load(kind, idx):
    return Image.open(os.path.join(POOL, '%s_%02d.png' % (kind, idx))).convert('RGBA')


def tint(im, color):
    out = Image.new('RGBA', im.size, hexc(color) + (0,))
    out.putalpha(im.split()[3])
    return out


def tile(im, size, bg=TILE):
    if im.size != (size, size):
        im = im.resize((size, size), Image.LANCZOS)
    t = Image.new('RGB', (size, size), bg)
    t.paste(im, (0, 0), im)
    return t


def build_grid():
    cell, lab, cols = 220, 30, 5
    sections = [(_title, count) for _title, count, _d, _f in TYPES]
    rows = sum((c + cols - 1) // cols + 1 for _t, c in sections)
    W = cell * cols
    H = rows * (cell + lab) + 20
    canvas = Image.new('RGB', (W, H), BG)
    d = ImageDraw.Draw(canvas)
    f = font(20)
    y = 14
    for title, count, desc, fmt in TYPES:
        d.text((10, y), '%s — %s (%s)' % (title, desc, fmt), fill=CYAN, font=font(24))
        y += lab + 6
        for i in range(1, count + 1):
            im = load(title, i)
            col = (i - 1) % cols
            row = (i - 1) // cols
            x = col * cell
            yy = y + row * (cell + lab) + lab
            canvas.paste(tile(im, 200), (x + 10, yy))
            d.text((x + 14, yy - 22), '%s_%02d' % (title, i), fill=GOLD, font=f)
        y += ((count + cols - 1) // cols) * (cell + lab)
    canvas.save(os.path.join(OUT, 'pool_grid.png'))
    print('Saved:', os.path.join(OUT, 'pool_grid.png'))


def build_demo():
    W = H = 720
    canvas = Image.new('RGB', (W, H), BG)
    d = ImageDraw.Draw(canvas)
    # виньетка
    vign = Image.new('L', (W, H), 0)
    vd = ImageDraw.Draw(vign)
    vd.ellipse([-160, -160, W + 160, H + 160], fill=90)
    canvas = Image.composite(canvas, Image.new('RGB', (W, H), (0, 0, 0)), vign)

    # дальнейшая туманность (тинт индиго, alpha 0.3)
    neb = load('nebula_bg', 1)
    neb = tint(neb, '#6366f1')
    a = neb.split()[3].point(lambda v: int(v * 0.35))
    neb.putalpha(a)
    neb = neb.resize((W + 120, H + 120), Image.LANCZOS)
    canvas.paste(neb, (-60, -60), neb)

    # поле 1:1
    box = 60
    d.rectangle([box, box, W - box, H - box], outline=(51, 65, 85), width=2)
    for gx in range(box + 100, W - box, 100):
        d.line([(gx, box), (gx, H - box)], fill=(30, 40, 58), width=1)
    for gy in range(box + 100, H - box, 100):
        d.line([(box, gy), (W - box, gy)], fill=(30, 40, 58), width=1)

    # ФИНИШ: star_core_01, тинт G (золото)
    star = tint(load('star_core', 1), '#ffd700').resize((150, 150), Image.LANCZOS)
    canvas.paste(star, (W - 230, 95), star)

    # ЧД: black_hole_01 с тёмным ореолом
    bhh = Image.new('RGBA', (150, 150), (0, 0, 0, 0))
    ImageDraw.Draw(bhh).ellipse([0, 0, 149, 149], fill=(59, 7, 100, 120))
    canvas.paste(bhh, (95, 430), bhh)
    bh = load('black_hole', 1).resize((120, 120), Image.LANCZOS)
    canvas.paste(bh, (110, 445), bh)

    # опасная зона: hazard_cloud_01, холодный тинт
    hz = tint(load('hazard_cloud', 1), HAZ_TINT)
    hz = hz.resize((150, 150), Image.LANCZOS)
    a = hz.split()[3].point(lambda v: int(v * 0.5))
    hz.putalpha(a)
    canvas.paste(hz, (360, 420), hz)
    d.text((360, 578), 'зона «дороже»', fill=MUTE, font=font(18))

    # маяки/ложный в игровых размерах
    b1 = load('beacon', 1).resize((64, 64), Image.LANCZOS)
    b2 = load('beacon', 3).resize((64, 64), Image.LANCZOS)
    fs = load('false_signal', 4).resize((56, 56), Image.LANCZOS)
    canvas.paste(b1, (150, 250), b1)
    canvas.paste(b2, (330, 200), b2)
    canvas.paste(fs, (250, 330), fs)
    d.text((150, 316), 'beacon_01', fill=GOLD, font=font(16))
    d.text((330, 266), 'beacon_03', fill=GOLD, font=font(16))
    d.text((250, 388), 'false_04', fill=(239, 68, 68), font=font(16))

    # путь СТАРТ → ФИНИШ
    d.line([(110, 600), (200, 480), (300, 360), (430, 260), (620, 165)],
           fill=(96, 165, 250), width=4)
    b = load('beacon', 1).resize((40, 40), Image.LANCZOS)
    canvas.paste(b, (90, 580), b)
    d.text((70, 640), 'СТАРТ', fill=TEXT, font=font(18))
    d.text((560, 250), 'ФИНИШ', fill=TEXT, font=font(18))
    canvas.save(os.path.join(OUT, 'pool_demo.png'))
    print('Saved:', os.path.join(OUT, 'pool_demo.png'))


def cards(kind, count, cls=''):
    out = ''
    for i in range(1, count + 1):
        name = '%s_%02d' % (kind, i)
        out += ("<div class='card %s'><div class='num'>%s</div><img src='../%s.png'>"
                "<div class='nm'>%s</div></div>" % (cls, i, name, name))
    return out


def tint_cards(kind, color, label):
    return ("<div class='card sm'><div class='num'>%s</div>"
            "<div class='tint' style='--c:%s'><img src='../%s.png'></div>"
            "<div class='nm'>%s</div></div>" % (label, color, kind, color))


def sizes_cards(kind):
    return ''.join(
        "<div class='card sm'><div class='num'>%dpx</div>"
        "<img src='../%s_01.png' width='%d' height='%d'><div class='nm'>&nbsp;</div></div>"
        % (sz, kind, sz, sz) for sz in (128, 64, 32))


def build_html():
    star_tints = ''.join(tint_cards('star_core_01', c, l) for l, c in STAR_TINTS)
    neb_tints = ''.join(tint_cards('nebula_bg_01', c, l) for l, c in NEB_TINTS)
    haz_tints = ''.join(tint_cards('hazard_cloud_01', c, l)
                        for l, c in [('лёд', '#38bdf8'), ('туман', '#8b5cf6')])
    html = """<!DOCTYPE html><html><head><meta charset='utf-8'>
<title>Zorion — маршрут: пул спрайтов (режим наигрыша)</title>
<style>
 body{background:#05070f;color:#cbd5e1;font-family:'Segoe UI',Roboto,system-ui,sans-serif;padding:24px}
 h1{color:#7dd3fc;margin:0 0 6px} h2{color:#a5b4fc;margin:30px 0 10px}
 .legend{color:#94a3b8;font-size:13px;max-width:1100px;line-height:1.55}
 .grid{display:flex;gap:12px;flex-wrap:wrap;margin-top:10px}
 .card{position:relative;background:#0a0e17;border:1px solid #24344d;border-radius:10px;padding:8px;text-align:center}
 .card img{width:160px;height:160px;display:block}
 .card.sm img{width:96px;height:96px}
 .num{position:absolute;top:3px;left:9px;color:#fbbf24;font-weight:700;font-size:14px}
 .nm{color:#cbd5e1;font-size:12px;margin-top:5px}
 .tint{background:#05070f;border-radius:6px}
 .tint img{width:96px;height:96px;mix-blend-mode:screen;filter:drop-shadow(0 0 6px var(--c))}
 .big{background:#0a0e17;border:1px solid #24344d;border-radius:10px;padding:10px;margin-top:10px}
 .big img{max-width:100%%;display:block}
 .ask{background:#0f172a;border:1px solid #334155;border-radius:10px;padding:14px;margin-top:18px;color:#e2e8f0}
 .ask b{color:#fde047}
 code{color:#7dd3fc}
</style></head><body>
<h1>Маршрут — полный пул спрайтов (направление A, «максимум красоты»)</h1>
<div class='legend'>
 Режим <b>наигрыша</b>: при каждом запуске страница берёт случайный вариант типа.
 Все 22 PNG — прозрачный фон, один холст/якорь на тип, одинаковый запас от края;
 <b>колец захвата в PNG нет</b> (их рисует код по игровому радиусу).
 Звезда — белый luminance (тинт/температуру накладывает код); ЧД, фактура зоны и
 туманность — grayscale; маяк и ложный — нейтральный металл.
</div>
<div class='ask'>
 <b>Создателю:</b> это пул для наигрыша — сравните варианты в игре и назовите победителя
 по каждому типу (после этого выбор замораживается). Ниже — все варианты по типам и
 мини-сцена «как в игре».
</div>

<h2>ФИНИШ-звезда: star_core_01…05 (белый luminance, 512)</h2>
<div class='grid'>%s</div>
<div class='legend'>Тинт кодом из <code>map.starColors</code> (демо на star_core_01):</div>
<div class='grid'>%s</div>

<h2>ФИНИШ-ЧД: black_hole_01…03 (grayscale, 512)</h2>
<div class='grid'>%s</div>

<h2>Глубина фона: nebula_bg_01…03 (grayscale-alpha, 1024; тинт кодом)</h2>
<div class='grid'>%s</div>
<div class='legend'>Тинт палитры §3.1 (демо на nebula_bg_01):</div>
<div class='grid'>%s</div>

<h2>Зона потери времени: hazard_cloud_01…03 (grayscale-alpha, 512; маска по границе зоны)</h2>
<div class='grid'>%s</div>
<div class='legend'>Холодный тинт (демо на hazard_cloud_01):</div>
<div class='grid'>%s</div>

<h2>Обязательный маяк: beacon_01…04 (128)</h2>
<div class='grid'>%s</div>
<h2>beacon — игровые размеры</h2>
<div class='grid'>%s</div>

<h2>Ложный сигнал: false_signal_01…04 (128)</h2>
<div class='grid'>%s</div>
<h2>false_signal — игровые размеры</h2>
<div class='grid'>%s</div>

<h2>Мини-сцена «как в игре»</h2>
<div class='big'><img src='pool_demo.png'></div>
<div class='legend'>Один из вариантов каждого типа: туманность (индиго) + tinted star_core
 + black_hole (тёмный ореол) + hazard (лёд) + beacon/false + путь. Кольца захвата — код.</div>
</body></html>""" % (
        cards('star_core', 5), star_tints,
        cards('black_hole', 3),
        cards('nebula_bg', 3), neb_tints,
        cards('hazard_cloud', 3), haz_tints,
        cards('beacon', 4), sizes_cards('beacon'),
        cards('false_signal', 4), sizes_cards('false_signal'))
    with open(os.path.join(OUT, 'pool.html'), 'w', encoding='utf-8') as fh:
        fh.write(html)
    print('Saved:', os.path.join(OUT, 'pool.html'))


if __name__ == '__main__':
    os.makedirs(OUT, exist_ok=True)
    build_grid()
    build_demo()
    build_html()
