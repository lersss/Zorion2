# batch_concepts2.ps1 — пачка 2: меч, зонт, снежинка, паук, бабочка, ключ, перо, комета, подкова, часы.
param(
    [int]$Count = 10,
    [int]$StartSeed = 400
)

. "$PSScriptRoot\comfy_api.ps1"

$stage1 = @{
    "sword" = "sci-fi spaceship SHAPED LIKE A SWORD, top-down view, long steel blade with light fuller, bronze crossguard, cream grip, golden pommel with dark engine, pointed tip forward, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "umbrella" = "sci-fi spaceship SHAPED LIKE AN UMBRELLA, top-down view, circular canopy of alternating cream and steel segments, golden tip at center, curved bronze handle with dark engine, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "snowflake" = "sci-fi spaceship SHAPED LIKE A SNOWFLAKE, top-down view, six-fold symmetric steel rays with branches, golden center, glowing glass core, crystalline, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "spider" = "sci-fi spaceship SHAPED LIKE A SPIDER, top-down view, cream abdomen, steel cephalothorax, eight curved bronze legs, glowing green eyes, dark engines at rear, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "butterfly" = "sci-fi spaceship SHAPED LIKE A BUTTERFLY, top-down view, two large steel upper wings, two smaller cream lower wings, purple eye spots, dark elongated body, bronze antennae, orange engine at head, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "key" = "sci-fi spaceship SHAPED LIKE A KEY, top-down view, long cream shaft, steel bit with teeth pointing forward, golden ring at rear with dark engine, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "feather" = "sci-fi spaceship SHAPED LIKE A FEATHER, top-down view, ivory vane with bronze shaft down middle, steel section lines, cream quill at rear with dark engine, smooth, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "comet" = "sci-fi spaceship SHAPED LIKE A COMET, top-down view, round steel nucleus with glass cockpit, long tapering cream tail with glow, orange engine at nucleus, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "horseshoe" = "sci-fi spaceship SHAPED LIKE A HORSESHOE, top-down view, bronze horseshoe arc open forward, steel tips, golden nail studs along arc, glass center patch, dark engines at tips, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "clock" = "sci-fi spaceship SHAPED LIKE A CLOCK, top-down view, round golden case, cream clock face with bronze hour marks, steel hour hand pointing forward, steel minute hand, dark center, orange engine at bottom, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
}

$stage2 = "sci-fi spaceship, top-down view, ENTIRE hull covered with dense mechanical texture, detailed panel lines everywhere, greebles, rivets, vents, hatches, armor plates, weathering, scratches, subtle metal gradients across whole body, glowing details, rich color palette, no orange, no flames, neutral engine blocks at rear, maximal detail, masterpiece, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"

$silMap = @{
    "sword" = "sil_sword.png"
    "umbrella" = "sil_umbrella.png"
    "snowflake" = "sil_snowflake.png"
    "spider" = "sil_spider.png"
    "butterfly" = "sil_butterfly.png"
    "key" = "sil_key.png"
    "feather" = "sil_feather.png"
    "comet" = "sil_comet.png"
    "horseshoe" = "sil_horseshoe.png"
    "clock" = "sil_clock.png"
}

$concepts = @($silMap.Keys)
$outDir = "C:\Zorion2\ai_drafts\batch_concepts2"
$procDir = "C:\Zorion2\ai_drafts\processed_concepts2"
New-Item -ItemType Directory -Path $outDir -Force | Out-Null
New-Item -ItemType Directory -Path $procDir -Force | Out-Null

$py = "C:\ComfyUI\venv\Scripts\python.exe"

for ($i = 0; $i -lt $Count; $i++) {
    $concept = $concepts[$i % $concepts.Count]
    $seed = $StartSeed + $i
    $num = $i + 1
    $silPath = "C:\Zorion2\ai_drafts\silhouettes\$($silMap[$concept])"
    Write-Host "=== Variant $num/$Count [concept=$concept] seed=$seed ==="
    $s1 = "$outDir\v${num}_stage1.png"
    $fin = "$outDir\v${num}.png"
    $clean = "$procDir\v${num}.png"

    $r1 = Send-ComfyControlNet -Prompt $stage1[$concept] -SilhouettePath $silPath -Seed $seed -Steps 40 -Cfg 7 -Strength 1.5 -Denoise 0.85 -OutputPath $s1
    if (-not $r1) { Write-Host "Stage1 failed v$num"; continue }

    $r2 = Send-ComfyImg2Img -Prompt $stage2 -InputImage $s1 -Seed ($seed + 500) -Steps 40 -Cfg 7.5 -Denoise 0.55 -OutputPath $fin
    if (-not $r2) { Write-Host "Stage2 failed v$num"; continue }

    & $py "C:\Zorion2\tools\process_ship.py" $fin $clean
}

Write-Host "=== Done: $Count concept ships in $procDir ==="