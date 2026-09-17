# batch_races_p8.ps1 — пачка 8: РАЗНООБРАЗИЕ неантропоморфных форм (F4 Серные).
# 5 форм-архетипов: рой, соты, плита, капля, вихрь. Живописный стиль, txt2img.
# Пост: process_ship --no-orient + прижатие к низу (посаженность).
param([int]$StartSeed = 87000)

. "$PSScriptRoot\comfy_api.ps1"

$outDir = "C:\Zorion2\ai_drafts\races_p8"
$procDir = "C:\Zorion2\ai_drafts\races_p8_processed"
New-Item -ItemType Directory -Path $outDir -Force | Out-Null
New-Item -ItemType Directory -Path $procDir -Force | Out-Null

$py = "C:\ComfyUI\venv\Scripts\python.exe"
$log = "$outDir\log.txt"
Add-Content $log "=== races_p8 (form diversity) start $(Get-Date -Format s) ==="

$tail = "anchored by a solid base extending down to the bottom edge of the frame, NOT floating, centered, on black background, masterpiece, game avatar, no text, no watermark"
$neg = "text, watermark, blurry, low quality, deformed, ugly, duplicate, 3D render, cartoon, anime, human, person, face, eyes, nose, mouth, ears, chin, shoulders, head, portrait, symmetrical, human anatomy, building, structure, machine, pipe, chimney, castle, architecture, organ, flesh, meat, ribs, corrugation, wavy, fish, animal, cave, scenery, logo, icon"

$items = @(
    @{ n = "рой (Серные рои 19)"; p = "dramatic cinematic concept art of a swarm of small glowing amber mineral shards, non-organic creature, dense cloud of sharp crystalline fragments clustering together, each shard glowing, forming a loose asymmetric mass, no single shape, chiaroscuro lighting, painterly brushwork, $tail" },
    @{ n = "соты (Серные гнёзда 15)"; p = "dramatic cinematic concept art of a hexagonal honeycomb mineral structure, non-organic creature, dark basalt honeycomb with glowing amber cells, some cells hollow, asymmetric cluster, organic mineral growth, chiaroscuro lighting, painterly brushwork, $tail" },
    @{ n = "плита (Пепельные 21)"; p = "dramatic cinematic concept art of a cracked stone slab form, non-organic creature, layered ash-grey rock plates, glowing amber fissures between layers, monolithic but asymmetric, chiaroscuro lighting, painterly brushwork, $tail" },
    @{ n = "капля (Серные странники 22)"; p = "dramatic cinematic concept art of a molten droplet form, non-organic creature, a single large teardrop of liquid sulfur-gold, smooth glossy surface, internal swirl, dripping from a stem base, chiaroscuro lighting, painterly brushwork, $tail" },
    @{ n = "вихрь (Приливные 43)"; p = "dramatic cinematic concept art of a swirling vortex mineral form, non-organic creature, a twisting spiral of dark teal stone bands with deep blue glow, like a frozen whirlpool, smooth, chiaroscuro lighting, painterly brushwork, $tail" }
)

for ($i = 0; $i -lt $items.Count; $i++) {
    $seed = $StartSeed + $i
    $num = $i + 1
    $it = $items[$i]
    Write-Host "=== $num/5 [$($it.n)] seed=$seed ==="
    Add-Content $log "--- $num/5 [$($it.n)] seed=$seed"
    $fin = "$outDir\v${num}.png"
    $r = Send-ComfyPrompt -Prompt $it.p -NegativePrompt $neg -Width 1024 -Height 1024 -Steps 40 -Cfg 7 -Seed $seed -Model "juggernaut-xl-v9.safetensors" -OutputPath $fin
    if (-not $r) { Add-Content $log "v$num failed"; continue }
    & $py "C:\Zorion2\tools\process_ship.py" $fin "$procDir\v${num}.png" --no-orient
    Add-Content $log "OK v$num"
}

# Прижатие к низу (посаженность) для всех
Add-Content $log "--- bottom-anchoring ---"
& $py -c @"
import sys
from PIL import Image
import numpy as np, os
d = r'C:\Zorion2\ai_drafts\races_p8_processed'
for i in range(1, 6):
    p = os.path.join(d, 'v%d.png' % i)
    im = Image.open(p)
    a = np.array(im); alpha = a[:,:,3]; mask = alpha > 40
    if mask.sum() == 0: continue
    ys, xs = np.where(mask)
    h = ys.max() - ys.min(); w = xs.max() - xs.min()
    # crop bbox
    im2 = im.crop((max(0,xs.min()-4), max(0,ys.min()-4), min(199,xs.max()+4), min(199,ys.max()+4)))
    # если объект занимает мало высоты — увеличим
    target_h = 165
    if im2.height < target_h:
        s = target_h / im2.height
        im2 = im2.resize((max(1,int(im2.width*s)), target_h), Image.LANCZOS)
    canvas = Image.new('RGBA', (200,200), (0,0,0,0))
    x = (200 - im2.width) // 2
    canvas.paste(im2, (x, 200 - im2.height), im2)
    canvas.save(p)
    a2 = np.array(canvas); al2 = a2[:,:,3]; m2 = al2>40
    ys2, xs2 = np.where(m2)
    print('v%d: bbox y=[%d,%d] низ_у_края=%d' % (i, ys2.min(), ys2.max(), m2[-5:,:].sum()))
"@

Add-Content $log "=== races_p8 done $(Get-Date -Format s) ==="
Write-Host "=== Done: 5 avatars (pack 8: form diversity) ==="