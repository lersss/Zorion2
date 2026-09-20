# -*- coding: utf-8 -*-
import re, os, sys
sys.stdout.reconfigure(encoding='utf-8', errors='replace')
base = r'C:\Zorion2\ai_drafts\biomes'
p = os.path.join(base, 'preview.html')
html = open(p, encoding='utf-8').read()
imgs = re.findall(r"<img src='([^']+)'>", html)
missing = [i for i in imgs if not os.path.exists(os.path.join(base, i))]
nums = re.findall(r"<div class='num'>(\d+)</div>", html)
print('preview img refs:', len(imgs))
print('missing files:', missing if missing else 'NONE')
print('numbered cards:', len(nums), 'first/last:', nums[0], nums[-1])
print('preview size: %d bytes' % len(html))
print('has 48px comparison:', 'леса_48.png' in html)
print('has style tags:', 'стиль B' in html and 'стиль C' in html)