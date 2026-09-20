# -*- coding: utf-8 -*-
# Перерисовка 7 иконок с плоским низом (леса, светящиеся_чащи, кислотные_дебри, химический_иней,
# углеводородные_равнины, металлические_щетинные_поля, струнные_рощи): проработанный передний план.
import os, sys, subprocess
sys.stdout.reconfigure(encoding='utf-8', errors='replace')
sys.path.insert(0, r'C:\Zorion2\tools')
import batch_biomes_p1 as B
import biome_manifest as M

B.OUT_DIR = r'C:\Zorion2\ai_drafts\biomes\batch_p1B'
OUT = r'C:\Zorion2\ai_drafts\biomes\processed_p1B'
PY = r'C:\ComfyUI\venv\Scripts\python.exe'
PROC = r'C:\Zorion2\tools\process_icon.py'

STAGE2_FG = ('miniature landscape scene, ENTIRE scene covered with rich painterly texture, '
             'detailed textured foreground terrain with relief and rocks, layered ground planes, '
             'soft gradients, atmospheric haze, clean empty sky, no sun, no moon, no planets, no stars, '
             'no celestial bodies, game icon, centered, on black background, no text, no watermark, '
             'maximal detail, masterpiece')

# (индекс в манифесте, номер) -> промпт этапа 1
JOBS = {
    0: ('dense pine forest, two rows of dark green trees, undergrowth bushes, fallen log, '
        'layered forest ground with roots and grass tufts, pale sky'),
    5: ('dense glowing thicket, MANY glowing trees, glowing bushes in the foreground, lots of cyan glow dots, '
        'layered dark ground, glowing veins in the soil, twilight'),
    7: ('acidic thicket, spiky acid-green plants, layered green ground, acid puddle with bubbles and highlights, '
        'plants growing from the water, fallen logs, green haze'),
    10: ('chemical frost, tall white frost spikes, layered ice crust, rocks covered with frost, cracks in the ice, '
         'icicles hanging, pale icy sky'),
    12: ('hydrocarbon plains, vent tower, layered dark ground, several black tar pools with gloss highlights, '
         'rocks, cracks, smog sky'),
    20: ('metal bristle field, dense steel needles in the distance, LARGE foreground needles with highlights, '
         'layered ground, broken needles on the ground, rock slabs, grey overcast sky'),
    21: ('string grove, thin glowing strings descending all the way to the ground, layered ground, '
         'coiled fibers on the ground, glow near the ground, dusk sky'),
}

SEED0 = 910000

for idx, (prompt) in JOBS.items():
    item = M.ITEMS[idx]
    num = idx + 1
    item['s1'] = prompt + M.STYLE_B
    seed = SEED0 + num * 3
    stage2 = (STAGE2_FG + ', glowing lava') if item.get('lava') else STAGE2_FG
    B.log('=== REDRAW-FG3 %d [%s] seed=%d ===' % (num, item['id'], seed))
    pid = B.post_prompt(B.workflow_controlnet(item['s1'], B.NEG1, os.path.join(B.SIL_DIR, item['sil']), seed))
    img1 = B.wait_result(pid)
    B.download(img1, os.path.join(B.OUT_DIR, '%d_stage1.png' % num))
    pid2 = B.post_prompt(B.workflow_img2img(stage2, B.NEG2, os.path.join(B.OUT_DIR, '%d_stage1.png' % num), seed + 500))
    img2 = B.wait_result(pid2)
    B.download(img2, os.path.join(B.OUT_DIR, '%d.png' % num))
    B.log('  ok %s' % item['id'])
    r = subprocess.run([PY, PROC, os.path.join(B.OUT_DIR, '%d.png' % num),
                        os.path.join(OUT, item['id'] + '.png'), '--size', '48', '--scale', '0.94', '--bg-dark', '20'], capture_output=True)
    print((r.stdout or b'').decode('utf-8', 'replace').strip() or ('OK %s' % item['id']))

print('REDRAW-FG3 DONE')