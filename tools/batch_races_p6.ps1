# batch_races_p6.ps1 — пачка 6: ЖИВОПИСНЫЙ стиль по семействам (txt2img, без силуэта).
# По одному представителю на семейство F1-F9. Промпт как у принятых v1/v2 пачки 5.
# Пост: process_ship --no-orient. Проверка: caption_image.py (BLIP + посаженность).
param([int]$StartSeed = 80000)

. "$PSScriptRoot\comfy_api.ps1"

$outDir = "C:\Zorion2\ai_drafts\races_p6"
$procDir = "C:\Zorion2\ai_drafts\races_p6_processed"
New-Item -ItemType Directory -Path $outDir -Force | Out-Null
New-Item -ItemType Directory -Path $procDir -Force | Out-Null

$py = "C:\ComfyUI\venv\Scripts\python.exe"
$log = "$outDir\log.txt"
Add-Content $log "=== races_p6 start $(Get-Date -Format s) ==="

# Стиль (общий хвост промпта — живописный концепт-арт, как v1/v2 пачки 5)
$styleTail = "head and shoulders, torso extending down below the frame, anchored, centered, on black background, masterpiece, game avatar, no text, no watermark"

# Промпты по семействам (основа из спекы 99.2.21)
$prompts = @(
    # 1: F1 Океанида (вода, бирюзовый, жабры, плавники)
    "dramatic cinematic concept art portrait of an aquatic alien humanoid, FRONT VIEW, turquoise smooth skin, gill slits on neck, fin-like ears, wide dark eyes, wet glossy skin with water droplets, chiaroscuro lighting, painterly brushwork, $styleTail",
    # 2: F3 Грибовик (метан, шляпка, мицелий)
    "dramatic cinematic concept art portrait of an alien methane fungus creature, FRONT VIEW, large mushroom cap head with pale spots, fungal stalk neck, spores floating, dim cool lighting, chiaroscuro, painterly brushwork, $styleTail",
    # 3: F5 Лавовик (терморедокс, лава, камень)
    "dramatic cinematic concept art portrait of an alien lava creature, FRONT VIEW, dark volcanic rock head with glowing orange lava veins, short horns, lava glow eyes, embers rising, chiaroscuro, painterly brushwork, $styleTail",
    # 4: F6 Кристаллит (кремний, грани, стекло)
    "dramatic cinematic concept art portrait of an alien silicon crystal creature, FRONT VIEW, faceted crystalline head of translucent quartz, glowing inner core, sharp crystal edges, specular highlights, chiaroscuro, painterly brushwork, $styleTail",
    # 5: F7 Небесный (атмосфера гиганта, облачный купол)
    "dramatic cinematic concept art portrait of an alien gas-giant cloud being, FRONT VIEW, translucent cloud dome head, swirling vortex inside, wisps of atmosphere, soft rim light, chiaroscuro, painterly brushwork, $styleTail",
    # 6: F8 Фумарольник (CO2, веера, жерло)
    "dramatic cinematic concept art portrait of an alien carbon-dioxide creature, FRONT VIEW, grey stone head with white fan frill collar, dark vent mouth, faint smoke wisps, chiaroscuro, painterly brushwork, $styleTail",
    # 7: F9 Магнетар (излучение, энергия)
    "dramatic cinematic concept art portrait of an alien energy being, FRONT VIEW, glowing violet energy head, magnetic field arcs, bright pulsing core, radiant energy, chiaroscuro, painterly brushwork, $styleTail",
    # 8: F1 Люди (человеческое лицо)
    "dramatic cinematic concept art portrait of a human, FRONT VIEW, neutral male face, short dark hair, calm expression, natural skin texture, chiaroscuro lighting, painterly brushwork, $styleTail"
)

$negArt = "text, watermark, blurry, low quality, deformed, ugly, duplicate, extra fingers, side view, profile, three-quarter view, 3D render, cartoon, anime, flat, plain, floating, cut off head, no body, disembodied"

for ($i = 0; $i -lt 8; $i++) {
    $seed = $StartSeed + $i
    $num = $i + 1
    Write-Host "=== $num/8 [family] seed=$seed ==="
    Add-Content $log "--- $num/8 seed=$seed"
    $fin = "$outDir\v${num}.png"
    $r = Send-ComfyPrompt -Prompt $prompts[$i] -NegativePrompt $negArt -Width 1024 -Height 1024 -Steps 40 -Cfg 7 -Seed $seed -Model "juggernaut-xl-v9.safetensors" -OutputPath $fin
    if (-not $r) { Add-Content $log "v$num failed"; continue }
    & $py "C:\Zorion2\tools\process_ship.py" $fin "$procDir\v${num}.png" --no-orient
    Add-Content $log "OK v$num"
}

Add-Content $log "=== races_p6 done $(Get-Date -Format s) ==="
Write-Host "=== Done: 8 avatars (pack 6: painterly by family) ==="