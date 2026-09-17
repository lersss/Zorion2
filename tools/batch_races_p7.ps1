# batch_races_p7.ps1 — пачка 7: СПЕКТР антропоморфности на F4 Серные (9 рас).
# Живописный стиль (txt2img). 2 антропо + 3 полу-твари + 4 сильно отличающихся.
# Для неантропо: негатив с запретом человеческой анатомии + асимметрия.
param([int]$StartSeed = 85000)

. "$PSScriptRoot\comfy_api.ps1"

$outDir = "C:\Zorion2\ai_drafts\races_p7"
$procDir = "C:\Zorion2\ai_drafts\races_p7_processed"
New-Item -ItemType Directory -Path $outDir -Force | Out-Null
New-Item -ItemType Directory -Path $procDir -Force | Out-Null

$py = "C:\ComfyUI\venv\Scripts\python.exe"
$log = "$outDir\log.txt"
Add-Content $log "=== races_p7 (F4 spectrum) start $(Get-Date -Format s) ==="

$styleTail = "head and shoulders, torso extending down below the frame, anchored, centered, on black background, masterpiece, game avatar, no text, no watermark"
$negHuman = "text, watermark, blurry, low quality, deformed, ugly, duplicate, extra fingers, 3D render, cartoon, anime, flat, plain, floating, cut off head, no body, disembodied"
$negAntiHuman = "text, watermark, blurry, low quality, deformed, ugly, duplicate, extra fingers, 3D render, cartoon, anime, flat, plain, floating, cut off head, no body, disembodied, human anatomy, human face, human shoulders, collarbone, human proportions, symmetrical human face, nose, human ears"

# Расы F4 (спектра): индекс -> (имя, промпт, негатив)
$items = @(
    @{ n = "20 Преисподние"; neg = $negHuman; p = "dramatic cinematic concept art portrait of an alien sulfur humanoid demon, FRONT VIEW, humanoid face with dark basalt skin, glowing amber vertical slit eyes, curved horns, amber cracks in stone-like skin, noble sinister expression, chiaroscuro lighting, painterly brushwork, $styleTail" },
    @{ n = "22 Серные странники"; neg = $negHuman; p = "dramatic cinematic concept art portrait of a nomadic alien sulfur humanoid, FRONT VIEW, weathered humanoid face, pale sulfur-crusted skin, amber eyes, braided tendrils instead of hair, travel-worn cloth collar, determined calm expression, chiaroscuro lighting, painterly brushwork, $styleTail" },
    @{ n = "15 Серные гнёзда"; neg = $negAntiHuman; p = "dramatic cinematic concept art portrait of an alien sulfur hive creature, FRONT VIEW, hive-nest head with hexagonal honeycomb sockets, small multiple eyes, mandibles instead of jaw, insectoid, asymmetric, chiaroscuro lighting, painterly brushwork, $styleTail" },
    @{ n = "17 Серные степи"; neg = $negAntiHuman; p = "dramatic cinematic concept art portrait of an alien sulfur plains beast, FRONT VIEW, horned beast head with rough skin, wide amber glowing eyes, cracked muzzle, heavy brow, semi-humanoid posture, chiaroscuro lighting, painterly brushwork, $styleTail" },
    @{ n = "21 Пепельные"; neg = $negAntiHuman; p = "dramatic cinematic concept art portrait of an alien sulfur ash creature, FRONT VIEW, ash-covered head with mask-like face, hollow dark eyes, cracked gray-white skin, hood of drifting ash, no human ears, chiaroscuro lighting, painterly brushwork, $styleTail" },
    @{ n = "16 Курильщики"; neg = $negAntiHuman; p = "dramatic cinematic concept art of an alien black smoker creature, ABSTRACT HEAD, asymmetric chimney-like head, single vent mouth on one side, one large eye on the other, smoke pouring from vents, no human features, chiaroscuro lighting, painterly brushwork, $styleTail" },
    @{ n = "18 Вулканиты"; neg = $negAntiHuman; p = "dramatic cinematic concept art of an alien volcanic creature, ABSTRACT HEAD, volcanic cone head with crater instead of face, glowing lava cracks, no eyes, no mouth, stone plates, asymmetric, chiaroscuro lighting, painterly brushwork, $styleTail" },
    @{ n = "19 Серные рои"; neg = $negAntiHuman; p = "dramatic cinematic concept art of an alien sulfur swarm creature, ABSTRACT HEAD, cluster of many small glowing eyes across the head, no single face, no nose no mouth, chitin plates, asymmetric swarm mass, chiaroscuro lighting, painterly brushwork, $styleTail" },
    @{ n = "43 Приливные"; neg = $negAntiHuman; p = "dramatic cinematic concept art of an alien tidal creature, ABSTRACT HEAD, asymmetric shell-like head, one large eye, maw on the side, barnacle growths, wet sheen, no human features, chiaroscuro lighting, painterly brushwork, $styleTail" }
)

for ($i = 0; $i -lt $items.Count; $i++) {
    $seed = $StartSeed + $i
    $num = $i + 1
    $it = $items[$i]
    Write-Host "=== $num/9 [$($it.n)] seed=$seed ==="
    Add-Content $log "--- $num/9 [$($it.n)] seed=$seed"
    $fin = "$outDir\v${num}.png"
    $r = Send-ComfyPrompt -Prompt $it.p -NegativePrompt $it.neg -Width 1024 -Height 1024 -Steps 40 -Cfg 7 -Seed $seed -Model "juggernaut-xl-v9.safetensors" -OutputPath $fin
    if (-not $r) { Add-Content $log "v$num failed"; continue }
    & $py "C:\Zorion2\tools\process_ship.py" $fin "$procDir\v${num}.png" --no-orient
    Add-Content $log "OK v$num"
}

Add-Content $log "=== races_p7 done $(Get-Date -Format s) ==="
Write-Host "=== Done: 9 avatars (pack 7: F4 sulfur spectrum) ==="