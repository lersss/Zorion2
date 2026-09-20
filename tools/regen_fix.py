# -*- coding: utf-8 -*-
# Исправление: реген луга_степи (num 2) с пустым небом и светящиеся_чащи (num 6) с оригинальным промптом.
import os, sys, subprocess
sys.stdout.reconfigure(encoding='utf-8', errors='replace')
sys.path.insert(0, r'C:\Zorion2\tools')
import batch_biomes_p1 as B
import biome_manifest as M

B.OUT_DIR = r'C:\Zorion2\ai_drafts\biomes\batch_p1B'
OUT = r'C:\Zorion2\ai_drafts\biomes\processed_p1B'
PY = r'C:\ComfyUI\venv\Scripts\python.exe'
PROC = r'C:\Zorion2\tools\process_icon.py'


def regen(num, item, seed):
    B.log('=== REGEN %d [%s] seed=%d ===' % (num, item['id'], seed))
    pid = B.post_prompt(B.workflow_controlnet(item['s1'], B.NEG1, os.path.join(B.SIL_DIR, item['sil']), seed))
    img1 = B.wait_result(pid)
    B.download(img1, os.path.join(B.OUT_DIR, '%d_stage1.png' % num))
    stage2 = B.STAGE2_LAVA if item.get('lava') else B.STAGE2_NORMAL
    pid2 = B.post_prompt(B.workflow_img2img(stage2, B.NEG2, os.path.join(B.OUT_DIR, '%d_stage1.png' % num), seed + 500))
    img2 = B.wait_result(pid2)
    B.download(img2, os.path.join(B.OUT_DIR, '%d.png' % num))
    B.log('  ok %s' % item['id'])


def proc(num, out_name):
    r = subprocess.run([PY, PROC, os.path.join(B.OUT_DIR, '%d.png' % num),
                        os.path.join(OUT, out_name), '--size', '48', '--scale', '0.94'], capture_output=True)
    print((r.stdout or b'').decode('utf-8', 'replace').strip() or ('OK %s' % out_name))


# луга_степи (num 2): пустое небо
M.ITEMS[1]['s1'] = ('rolling meadow hills with grass tufts and small yellow flowers, completely empty clear pale sky, '
                    'nothing in the sky, no birds, no dots, no objects in the sky' + M.STYLE_B)
regen(2, M.ITEMS[1], 420000)
proc(2, 'луга_степи.png')

# светящиеся_чащи (num 6): оригинальный промпт из манифеста
regen(6, M.ITEMS[5], 430000)
proc(6, 'светящиеся_чащи.png')

print('FIX DONE')