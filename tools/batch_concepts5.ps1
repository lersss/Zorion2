# batch_concepts5.ps1 — пачка 6 (6 новых слов): alien, predator, crater, volcano, star_celestial, shark.
param([int]$Count = 6, [int]$StartSeed = 10000)

. "$PSScriptRoot\comfy_api.ps1"

$stage1 = @{
    "alien" = "sci-fi spaceship SHAPED LIKE AN ALIEN HEAD, top-down flat view, large grey alien cranium, two big almond-shaped dark eyes with green glow, narrow chin pointing FORWARD to the right, dark engine at the left rear, horizontal, perfectly flat, no perspective, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "predator" = "sci-fi spaceship SHAPED LIKE A PREDATOR HEAD, top-down flat view, wide steel skull with four ivory mandibles pointing FORWARD to the right, golden glowing eyes, bronze dreads at the left rear, dark engine, horizontal, perfectly flat, no perspective, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "crater" = "sci-fi spaceship SHAPED LIKE A MOON CRATER, top-down flat view, round grey hull, large ring crater with dark center and glowing glass core, small secondary craters, dark engine at the left rear, horizontal, perfectly flat, no perspective, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "volcano" = "sci-fi spaceship SHAPED LIKE A VOLCANO, top-down flat view, bronze volcanic cone pointing FORWARD to the right, dark crater at tip, orange lava flowing down to the left, dark engine at base, horizontal, perfectly flat, no perspective, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "star_celestial" = "sci-fi spaceship SHAPED LIKE A CELESTIAL STAR, top-down flat view, glowing golden sphere with fiery prominences radiating outward, cream surface spots, bright glass core, dark engine at the left rear, horizontal, perfectly flat, no perspective, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "shark" = "sci-fi spaceship SHAPED LIKE A SHARK, top-down flat view, long steel shark body, dorsal fin on top, pectoral fins, forked tail at the left rear with fire engine, green eye, bronze gills, nose pointing FORWARD to the right, horizontal, perfectly flat, no perspective, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
}

$stage2 = "sci-fi spaceship, top-down view, ENTIRE hull covered with dense mechanical texture, detailed panel lines everywhere, greebles, rivets, vents, hatches, armor plates, weathering, scratches, subtle metal gradients across whole body, glowing details, rich color palette, no orange, no flames, neutral engine nozzles at left rear, maximal detail, masterpiece, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"

$silMap = @{
    "alien" = "sil_alien.png"; "predator" = "sil_predator.png"
    "crater" = "sil_crater.png"; "volcano" = "sil_volcano.png"
    "star_celestial" = "sil_star_celestial.png"; "shark" = "sil_shark.png"
}

$concepts = @($silMap.Keys)
$outDir = "C:\Zorion2\ai_drafts\batch_p6"
$procDir = "C:\Zorion2\ai_drafts\processed_p6"
New-Item -ItemType Directory -Path $outDir -Force | Out-Null
New-Item -ItemType Directory -Path $procDir -Force | Out-Null

$py = "C:\ComfyUI\venv\Scripts\python.exe"

for ($i = 0; $i -lt $Count; $i++) {
    $concept = $concepts[$i % $concepts.Count]
    $seed = $StartSeed + $i
    $num = $i + 1
    $silPath = "C:\Zorion2\ai_drafts\silhouettes\$($silMap[$concept])"
    Write-Host "=== $num/$Count [$concept] seed=$seed ==="
    $s1 = "$outDir\v${num}_stage1.png"
    $fin = "$outDir\v${num}.png"
    $clean = "$procDir\v${num}.png"

    $r1 = Send-ComfyControlNet -Prompt $stage1[$concept] -SilhouettePath $silPath -Seed $seed -Steps 40 -Cfg 7 -Strength 1.5 -Denoise 0.85 -OutputPath $s1
    if (-not $r1) { Write-Host "Stage1 failed v$num"; continue }

    $r2 = Send-ComfyImg2Img -Prompt $stage2 -InputImage $s1 -Seed ($seed + 500) -Steps 40 -Cfg 7.5 -Denoise 0.55 -OutputPath $fin
    if (-not $r2) { Write-Host "Stage2 failed v$num"; continue }

    & $py "C:\Zorion2\tools\process_ship.py" $fin $clean
}

Write-Host "=== Done: $Count ships (pack 6) ==="