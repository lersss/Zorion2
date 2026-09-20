# -*- coding: utf-8 -*-
# Точечная перегенерация проблемных иконок пачки 1 + переобработка.
# Использует функции batch_biomes_p1, манифест из biome_manifest (с переопределёнными промптами).
import os, sys
sys.stdout.reconfigure(encoding='utf-8', errors='replace')
sys.path.insert(0, r'C:\Zorion2\tools')
import batch_biomes_p1 as B
import biome_manifest as M
import subprocess

B.OUT_DIR = r'C:\Zorion2\ai_drafts\biomes\batch_p1'
PY = r'C:\ComfyUI\venv\Scripts\python.exe'
PROC = r'C:\Zorion2\tools\process_icon.py'
OUT = r'C:\Zorion2\ai_drafts\biomes\processed_p1'

# переопределяем промпты для проблемных итемов (индексы 4 и 16 в списке, num 5 и 17)
M.ITEMS[4]['s1'] = ('three large tall pine trees, giant bold green triangle crowns filling most of the frame, '
                    'thick dark trunks, wide dark ground bar' + M.STYLE_A +
                    ', very large in frame, fills most of the canvas, clear margins on black background')
M.ITEMS[16]['s1'] = ('brown cracked hydrocarbon plain with glossy black tar pools, bright white gloss highlights, '
                     'strong contrast between light brown ground and black pools' + M.STYLE_A +
                     ', very large in frame, fills most of the canvas, clear margins on black background')

SEED0 = 90000


def regen(num, item):
    seed = SEED0 + (num - 1) * 3
    s1_path = os.path.join(B.OUT_DIR, '%d_stage1.png' % num)
    fin_path = os.path.join(B.OUT_DIR, '%d.png' % num)
    sil_path = os.path.join(B.SIL_DIR, item['sil'])
    stage2 = B.STAGE2_LAVA if item.get('lava') else B.STAGE2_NORMAL
    B.log('=== REGEN %d [%s] seed=%d ===' % (num, item['id'], seed))
    pid = B.post_prompt(B.workflow_controlnet(item['s1'], B.NEG1, sil_path, seed))
    img1 = B.wait_result(pid)
    B.download(img1, s1_path)
    pid2 = B.post_prompt(B.workflow_img2img(stage2, B.NEG2, s1_path, seed + 500))
    img2 = B.wait_result(pid2)
    B.download(img2, fin_path)
    B.log('  ok %s (%d bytes)' % (item['id'], os.path.getsize(fin_path)))


def proc(num, out_name, extra=()):
    src = os.path.join(B.OUT_DIR, '%d.png' % num)
    dst = os.path.join(OUT, out_name)
    r = subprocess.run([PY, PROC, src, dst, '--size', '64', '--scale', '0.9'] + list(extra), capture_output=True)
    print((r.stdout or b'').decode('utf-8', 'replace').strip() or ('OK %s' % out_name))
    return r.returncode == 0


if __name__ == '__main__':
    # широкие силуэты -> выше (луга_степи 6, химический_иней 15, углеводородные_равнины 17)
    for num in (6, 15):
        M.ITEMS[num - 1]['s1'] = M.ITEMS[num - 1]['s1'] + ', very large in frame, fills most of the canvas, clear margins on black background'
        regen(num, M.ITEMS[num - 1])
        proc(num, M.ITEMS[num - 1]['id'] + '.png')
    regen(17, M.ITEMS[16])
    proc(17, 'углеводородные_равнины.png')
    # metan_B переобрабатываем через --bg-dark (тёмное небо -> прозрачность)
    proc(3, 'metan_B.png', ['--bg-dark', '60'])
    print('REGEN DONE')