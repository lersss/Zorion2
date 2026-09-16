# batch_races_p3.ps1 — пачка 3: ЛЮДИ через txt2img (БЕЗ силуэта; Juggernaut XL рисует лица сам)
# + ТВАРИ по материалу: Кристаллит x2, Фумарольник x2 (ControlNet, стиль пачки 1).
# Всего 8: v1-v4 люди, v5-v8 твари.
param([int]$StartSeed = 50000)

. "$PSScriptRoot\comfy_api.ps1"

$outDir = "C:\Zorion2\ai_drafts\races_p3"
$procDir = "C:\Zorion2\ai_drafts\races_p3_processed"
New-Item -ItemType Directory -Path $outDir -Force | Out-Null
New-Item -ItemType Directory -Path $procDir -Force | Out-Null

$py = "C:\ComfyUI\venv\Scripts\python.exe"
$log = "$outDir\log.txt"
Add-Content $log "=== races_p3 start $(Get-Date -Format s) ==="

# --- ЛЮДИ: txt2img, анфас, нейтральное лицо, разные варианты ---
# Негатив: НЕ запрещаем front view / facing camera (это анфас!). Запрещаем мусор и ракурсы сбоку.
$negHuman = "text, watermark, blurry, low quality, deformed, ugly, duplicate, extra fingers, side view, profile, three-quarter view, 3D render, cartoon, anime"

$humanPrompts = @(
    "realistic portrait of a human man, FRONT VIEW, face looking directly at viewer, short dark hair, neutral expression, natural skin texture, subtle pores, head and shoulders, centered, single person, on black background, game avatar, no text, no watermark",
    "realistic portrait of a human woman, FRONT VIEW, face looking directly at viewer, shoulder-length brown hair, calm neutral expression, natural skin texture, head and shoulders, centered, single person, on black background, game avatar, no text, no watermark",
    "realistic portrait of an older human man with grey beard, FRONT VIEW, face looking directly at viewer, weathered skin, neutral expression, head and shoulders, centered, single person, on black background, game avatar, no text, no watermark",
    "realistic portrait of a young human, FRONT VIEW, face looking directly at viewer, short blond hair, neutral expression, natural skin texture, head and shoulders, centered, single person, on black background, game avatar, no text, no watermark"
)

for ($i = 0; $i -lt 4; $i++) {
    $seed = $StartSeed + $i
    $num = $i + 1
    Write-Host "=== $num/8 [human txt2img] seed=$seed ==="
    Add-Content $log "--- $num/8 human seed=$seed"
    $fin = "$outDir\v${num}.png"
    $r = Send-ComfyPrompt -Prompt $humanPrompts[$i] -NegativePrompt $negHuman -Width 1024 -Height 1024 -Steps 40 -Cfg 7 -Seed $seed -Model "juggernaut-xl-v9.safetensors" -OutputPath $fin
    if (-not $r) { Add-Content $log "human v$num failed"; continue }
    & $py "C:\Zorion2\tools\process_ship.py" $fin "$procDir\v${num}.png" --no-orient
    Add-Content $log "OK v$num"
}

# --- ТВАРИ: ControlNet + img2img + post ---
$stage1 = @{
    "cryst_beast_a" = "realistic close-up of an alien silicon crystal creature HEAD ONLY, FRONT VIEW, faceted gargoyle head made of cream and purple crystal, crystal horns, glowing rhombus crystal eyes, open jaw with crystal teeth, glowing forehead core, short crystal neck, centered, single creature head, on black background, game avatar, no text, no watermark"
    "cryst_beast_b" = "realistic close-up of an alien silicon crystal creature HEAD ONLY, FRONT VIEW, faceted crystal skull head, rounded crystal cranium, glowing rhombus eye sockets, crystal crest on top, crystal teeth, short crystal neck, centered, single creature head, on black background, game avatar, no text, no watermark"
    "fum_beast_a" = "realistic close-up of an alien carbon-dioxide creature HEAD ONLY, FRONT VIEW, wide flat toad-like stone head, white fan gill collar around neck, bulging eyes on top of head, wide dark fissure mouth, bronze gill lines, short neck, centered, single creature head, on black background, game avatar, no text, no watermark"
    "fum_beast_b" = "realistic close-up of an alien carbon-dioxide creature HEAD ONLY, FRONT VIEW, stone monster head with pipe-like vent eyes, round dark vent mouth, smoke puffs rising, bronze rings on head, side vent tubes, short neck, centered, single creature head, on black background, game avatar, no text, no watermark"
}

$stage2 = "realistic alien creature head close-up, FRONT VIEW, ENTIRE head covered with detailed alien texture, rock crystal scales, pores, cracks, glowing details, subtle gradients, rich palette, maximal detail, masterpiece, game avatar, centered, single creature head, on black background, no text, no watermark"

$silMap = @{
    "cryst_beast_a" = "sil_race_cryst_beast_a.png"; "cryst_beast_b" = "sil_race_cryst_beast_b.png"
    "fum_beast_a" = "sil_race_fum_beast_a.png"; "fum_beast_b" = "sil_race_fum_beast_b.png"
}

$concepts = @($silMap.Keys)
for ($i = 0; $i -lt 4; $i++) {
    $concept = $concepts[$i]
    $seed = $StartSeed + 100 + $i
    $num = $i + 5
    $silPath = "C:\Zorion2\ai_drafts\silhouettes_races\$($silMap[$concept])"
    Write-Host "=== $num/8 [$concept] seed=$seed ==="
    Add-Content $log "--- $num/8 [$concept] seed=$seed"
    $s1 = "$outDir\v${num}_stage1.png"
    $fin = "$outDir\v${num}.png"

    $r1 = Send-ComfyControlNet -Prompt $stage1[$concept] -SilhouettePath $silPath -Seed $seed -Steps 40 -Cfg 7 -Strength 1.5 -Denoise 0.85 -OutputPath $s1
    if (-not $r1) { Add-Content $log "Stage1 failed v$num"; continue }
    $r2 = Send-ComfyImg2Img -Prompt $stage2 -InputImage $s1 -Seed ($seed + 500) -Steps 40 -Cfg 7.5 -Denoise 0.55 -OutputPath $fin
    if (-not $r2) { Add-Content $log "Stage2 failed v$num"; continue }
    & $py "C:\Zorion2\tools\process_ship.py" $fin "$procDir\v${num}.png" --no-orient
    Add-Content $log "OK v$num"
}

Add-Content $log "=== races_p3 done $(Get-Date -Format s) ==="
Write-Host "=== Done: 8 avatars (pack 3: 4 humans + 4 beasts) ==="