# -*- coding: utf-8 -*-
# Пост-обработка пачки 1 иконок биомов (стиль B): process_icon.py -> 48x48, id биома = имя файла.
# + леса_64.png (сравнение размера), + preview.html (сетка, сравнение 48/64, демо 24px в строке текста).
import os, subprocess, sys
sys.stdout.reconfigure(encoding='utf-8', errors='replace')
from biome_manifest import ITEMS

PY = r'C:\ComfyUI\venv\Scripts\python.exe'
PROC = r'C:\Zorion2\tools\process_icon.py'
BATCH = r'C:\Zorion2\ai_drafts\biomes\batch_p1B'
OUT = r'C:\Zorion2\ai_drafts\biomes\processed_p1B'
os.makedirs(OUT, exist_ok=True)


def run_icon(num, out_name, size, scale='0.94'):
    src = os.path.join(BATCH, '%d.png' % num)
    dst = os.path.join(OUT, out_name)
    r = subprocess.run([PY, PROC, src, dst, '--size', str(size), '--scale', scale, '--bg-dark', '20'], capture_output=True)
    out = r.stdout.decode('utf-8', errors='replace').strip()
    err = r.stderr.decode('utf-8', errors='replace').strip()
    if r.returncode != 0:
        print('FAIL %s: %s' % (out_name, err))
        return False
    print(out)
    return True


done = 0
failed = []
for i, item in enumerate(ITEMS):
    num = i + 1
    if run_icon(num, item['id'] + '.png', 48):
        done += 1
    else:
        failed.append(item['id'])
# вариант 64x64 для сравнения размера (леса)
run_icon(1, 'леса_64.png', 64)

# ---------- preview.html ----------
cards = []
for i, item in enumerate(ITEMS):
    num = i + 1
    cards.append("<div class='card'><div class='num'>%d</div><img src='processed_p1B/%s.png'><div class='name'>%s</div></div>" % (num, item['id'], item['name']))

size_block = (
    "<h2>Размер: 48×48 vs 64×64 (леса)</h2>"
    "<div class='sizecomp'>"
    "<div class='card'><div class='cap'>48×48</div><img src='processed_p1B/леса.png'></div>"
    "<div class='card'><div class='cap'>64×64</div><img src='processed_p1B/леса_64.png'></div>"
    "</div>"
)

# демо: иконка ~24px в строке текста (как в UI рядом с именем биома)
demo_biomes = ['леса', 'океаны', 'лавовые_поля', 'кристальные_рощи', 'метановые_моря', 'серные_поля']
demo_rows = []
for bid in demo_biomes:
    name = next(x['name'] for x in ITEMS if x['id'] == bid)
    demo_rows.append(
        "<div class='demorow'><img src='processed_p1B/%s.png'><span>%s</span></div>" % (bid, name))
demo_block = (
    "<h2>Иконка ~24px в строке текста (рядом с именем биома)</h2>"
    "<div class='demowrap'>%s</div>" % ''.join(demo_rows)
)

html = """<!DOCTYPE html><html><head><meta charset='utf-8'><title>Zorion — иконки биомов (пачка 1, стиль B)</title>
<style>body{background:#0a0e17;color:#cbd5e1;font-family:sans-serif;padding:20px}
h1{color:#7dd3fc}h2{color:#a5b4fc;margin-top:30px}
.grid{display:grid;grid-template-columns:repeat(5,1fr);gap:12px;margin-top:10px}
.card{text-align:center;border:1px solid #334155;border-radius:10px;padding:10px;background:#111827;position:relative}
.card img{width:48px;height:48px;image-rendering:pixelated;border-radius:6px}
.num{position:absolute;top:4px;left:8px;color:#fbbf24;font-weight:bold;font-size:12px}
.name{color:#94a3b8;font-size:11px;margin-top:6px;min-height:26px}
.sizecomp{display:flex;gap:12px}
.sizecomp .card img{width:96px;height:96px;border-radius:6px}
.cap{color:#fbbf24;font-size:12px;margin-bottom:6px}
.demowrap{display:flex;flex-direction:column;gap:8px;max-width:420px}
.demorow{display:flex;align-items:center;gap:8px;background:#111827;border:1px solid #334155;border-radius:8px;padding:6px 10px}
.demorow img{width:24px;height:24px;image-rendering:pixelated;border-radius:4px}
.demorow span{color:#cbd5e1;font-size:13px}
.legend{color:#64748b;font-size:12px;margin-top:6px}</style></head><body>
<h1>Zorion — иконки биомов поверхности, пачка 1 — стиль B (мини-пейзаж), %d шт</h1>
<div class='legend'>Вся пачка в стиле B. Небо чистое — без солнц/лун/звёзд/планет (требование создателя). Финальный размер — 48×48; сравнение с 64 и вид 24px в тексте — ниже.</div>
<div class='grid'>%s</div>
%s
%s
</body></html>""" % (len(ITEMS), ''.join(cards), size_block, demo_block)

with open(os.path.join(r'C:\Zorion2\ai_drafts\biomes', 'preview.html'), 'w', encoding='utf-8') as f:
    f.write(html)

print('DONE: %d/%d processed, failed=%s' % (done, len(ITEMS), failed))