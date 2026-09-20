# -*- coding: utf-8 -*-
# Перерисовка №17-20 (вулканические_поля, стеклянные_поля, кристальные_рощи, кремниевые_рощи):
# проработанный ПЕРЕДНИЙ ПЛАН — рельеф, слои грунта, передние объекты, градиенты.
# Кастомный этап 2 с акцентом на текстуру переднего плана.
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

NEW_PROMPTS = {
    16: ('volcanic fields, complete landscape: two large volcano cones with glowing craters, lava streams '
         'flowing into a layered lava lake with dark crust islands, smoke plumes, relief ridges of dark rock '
         'with glowing cracks, detailed rocky foreground'),
    17: ('glass fields, complete landscape of large overlapping faceted glass shards with bright glints, '
         'translucent blue glass, layered textured ground, foreground littered with small glass shards and '
         'glint lines, depth'),
    18: ('crystal grove, crystal trees with thick trunks and branching crystal tops, crystal roots at bases, '
         'glowing veins in the layered rocky ground, boulders with facets in the foreground, twilight atmosphere, '
         'complete environment'),
    19: ('silicon grove, three layered dunes with strata lines, spires, large faceted monolith, boulders and '
         'small spires on the foreground dune, fracture lines, hazy teal atmosphere, complete environment'),
}

SEED0 = 810000

for idx, num in [(16, 17), (17, 18), (18, 19), (19, 20)]:
    item = M.ITEMS[idx]
    item['s1'] = NEW_PROMPTS[idx] + M.STYLE_B
    seed = SEED0 + (num - 1) * 3
    stage2 = (STAGE2_FG + ', glowing lava') if item.get('lava') else STAGE2_FG
    B.log('=== REDRAW-FG %d [%s] seed=%d ===' % (num, item['id'], seed))
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

print('REDRAW-FG DONE')