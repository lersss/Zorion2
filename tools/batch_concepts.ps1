# batch_concepts.ps1 — генерация 10 кораблей в образах слов.
# Каждый вариант: свой концепт (дом/цветок/дракон/...), двухэтапный процесс.
param(
    [int]$Count = 10,
    [int]$StartSeed = 300
)

. "$PSScriptRoot\comfy_api.ps1"

$stage1 = @{
    "home" = "sci-fi spaceship SHAPED LIKE A HOUSE, top-down view, rectangular home with roof, glowing windows as viewports, chimney antenna, bronze roof, cream walls, engine door at back with orange exhaust, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "flower" = "sci-fi spaceship SHAPED LIKE A FLOWER, top-down view, symmetrical petals as wings, golden center disc, glowing glass core, steel and ivory petals, engine below with orange exhaust, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "dragon" = "sci-fi spaceship SHAPED LIKE A DRAGON, top-down view, long serpentine body with segmented spine plates, head with horns and glowing green eye, leathery wings, tail with spikes, fire breath engine at mouth, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "ghost" = "sci-fi spaceship SHAPED LIKE A GHOST, top-down view, smooth flowing teardrop body with wavy ethereal tail, glowing green eyes, dark mouth engine, ivory spectral hull, eerie glow, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "cigarette" = "sci-fi spaceship SHAPED LIKE A CIGARETTE, top-down view, long thin cylindrical hull, burning orange tip at front, steel filter at back, glass band, bronze rings, dark smoke trail, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "lightbulb" = "sci-fi spaceship SHAPED LIKE A LIGHTBULB, top-down view, round glass bulb with glowing filament, steel base with threads, bright warm glow, elegant, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "cat" = "sci-fi spaceship SHAPED LIKE A CAT, top-down view, round head with pointy ears, glowing green eyes, dark nose engine, oval body with bronze stripes, curved tail, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "skull" = "sci-fi spaceship SHAPED LIKE A SKULL, top-down view, round ivory skull hull, dark eye sockets with green glow, jaw with teeth, orange fire engine, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "dragonfly" = "sci-fi spaceship SHAPED LIKE A DRAGONFLY, top-down view, long segmented body, four transparent glass wings with veins, round head with glowing green eyes, thin steel tail, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "turtle" = "sci-fi spaceship SHAPED LIKE A TURTLE, top-down view, oval bronze shell with golden segments, head with dark eyes, steel flippers, tail engine with orange fire, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
}

$stage2 = "sci-fi spaceship, top-down view, ENTIRE hull covered with dense mechanical texture, detailed panel lines everywhere, greebles, rivets, vents, hatches, armor plates, weathering, scratches, subtle metal gradients across whole body, glowing details, rich color palette, no orange, no flames, neutral engine blocks at rear, maximal detail, masterpiece, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"

$silMap = @{
    "home" = "sil_home.png"
    "flower" = "sil_flower.png"
    "dragon" = "sil_dragon.png"
    "ghost" = "sil_ghost.png"
    "cigarette" = "sil_cigarette.png"
    "lightbulb" = "sil_lightbulb.png"
    "cat" = "sil_cat.png"
    "skull" = "sil_skull.png"
    "dragonfly" = "sil_dragonfly.png"
    "turtle" = "sil_turtle.png"
}

$concepts = @($silMap.Keys)
$outDir = "C:\Zorion2\ai_drafts\batch_concepts"
$procDir = "C:\Zorion2\ai_drafts\processed_concepts"
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