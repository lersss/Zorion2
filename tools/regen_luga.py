# -*- coding: utf-8 -*-
# Точечная перегенерация луга_степи (num 6) с гарантией пустого неба.
import os, sys, subprocess
sys.stdout.reconfigure(encoding='utf-8', errors='replace')
sys.path.insert(0, r'C:\Zorion2\tools')
import batch_biomes_p1 as B
import biome_manifest as M

B.OUT_DIR = r'C:\Zorion2\ai_drafts\biomes\batch_p1B'
OUT = r'C:\Zorion2\ai_drafts\biomes\processed_p1B'
PY = r'C:\ComfyUI\venv\Scripts\python.exe'
PROC = r'C:\Zorion2\tools\process_icon.py'

item = M.ITEMS[5]  # луга_степи (num 6)
item['s1'] = ('rolling meadow hills with grass tufts and small yellow flowers, completely empty clear pale sky, '
              'nothing in the sky, no birds, no dots, no objects in the sky' + M.STYLE_B)
seed = 320000

B.log('=== REGEN 6 [%s] seed=%d (empty sky) ===' % (item['id'], seed))
pid = B.post_prompt(B.workflow_controlnet(item['s1'], B.NEG1, os.path.join(B.SIL_DIR, item['sil']), seed))
img1 = B.wait_result(pid)
B.download(img1, os.path.join(B.OUT_DIR, '6_stage1.png'))
pid2 = B.post_prompt(B.workflow_img2img(B.STAGE2_NORMAL, B.NEG2, os.path.join(B.OUT_DIR, '6_stage1.png'), seed + 500))
img2 = B.wait_result(pid2)
B.download(img2, os.path.join(B.OUT_DIR, '6.png'))
B.log('  ok %s' % item['id'])

r = subprocess.run([PY, PROC, os.path.join(B.OUT_DIR, '6.png'),
                    os.path.join(OUT, 'луга_степи.png'), '--size', '48', '--scale', '0.94'], capture_output=True)
print((r.stdout or b'').decode('utf-8', 'replace').strip() or 'processed ok')
print('REGEN DONE')