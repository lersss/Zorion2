# gen_200.ps1 — массовая генерация 200 кораблей (26 концептов, случайные seed).
# Двухэтапный процесс + обработка. Лог в log-файл.
# Продолжение: -StartNum N -StartSeed S (продолжит с варианта N, seed = S + N - 1 + (i-1))
param([int]$Count = 200, [int]$StartSeed = 5000, [int]$StartNum = 1)

. "$PSScriptRoot\comfy_api.ps1"

$logPath = "C:\Zorion2\ai_drafts\gen200_log.txt"
function Log($msg) {
    $line = "[{0:HH:mm:ss}] {1}" -f (Get-Date), $msg
    Add-Content -Path $logPath -Value $line
    Write-Host $line
}

Log "=== Start gen $Count ==="

$stage1 = @{
    "arrow" = "sci-fi spaceship, top-down view, cream ivory metal hull, glass cockpit at the front nose, steel grey wings with light tips, bronze tail fins, small fins near wings, antenna at nose, dark engine blocks with orange exhaust at the rear, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "cruiser" = "sci-fi spaceship, top-down view, heavy cruiser, wide massive hull with central tower superstructure, glass cockpit at front, steel armor plates, bronze stern, dark engine battery with orange exhaust at rear, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "hawk" = "sci-fi spaceship, top-down view, flying wing fighter, HUGE swept wings spanning almost full width, narrow body, glass cockpit, bronze wing tips, thin tail, dark engines with orange exhaust at rear, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "freighter" = "sci-fi spaceship, top-down view, industrial freighter, boxy hull, glass cockpit at front, TWO HUGE round dark engines at rear with orange exhaust, bronze stern frame, steel ribs, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "protoss" = "sci-fi spaceship, top-down view, elegant alien protoss vessel, golden hull, symmetric diamond-wing shape, glowing blue-purple energy core, curved blades, purple accents, high-tech, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "zerg" = "sci-fi spaceship, top-down view, organic bio-alien zerg creature, chitinous shell, fleshy segments, horns and fangs at front, poison green glowing eyes, organic curved wings, dark chitin, asymmetrical organic, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "home" = "sci-fi spaceship SHAPED LIKE A HOUSE, top-down view, rectangular home with roof, glowing windows as viewports, chimney antenna, bronze roof, cream walls, engine door at back with orange exhaust, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "flower" = "sci-fi spaceship SHAPED LIKE A FLOWER, top-down view, symmetrical petals as wings, golden center disc, glowing glass core, steel and ivory petals, NO engine flame, neutral tail petals, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "dragon" = "sci-fi spaceship SHAPED LIKE A DRAGON, top-down view, long serpentine body with segmented spine plates, head with horns and glowing green eye, leathery wings, tail with spikes, fire breath engine at mouth, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "ghost" = "sci-fi spaceship SHAPED LIKE A GHOST, top-down view, smooth flowing teardrop body with wavy ethereal tail, glowing green eyes, dark mouth engine, ivory spectral hull, eerie glow, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "cigarette" = "sci-fi spaceship SHAPED LIKE A CIGARETTE, top-down view, long thin cylindrical hull, burning orange tip at front, steel filter at back, glass band, bronze rings, dark smoke trail, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "lightbulb" = "sci-fi spaceship SHAPED LIKE A LIGHTBULB, top-down view, round glass bulb with glowing filament, steel base with threads, bright warm glow, elegant, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "cat" = "sci-fi spaceship SHAPED LIKE A CAT, top-down view, round head with pointy ears, glowing green eyes, dark nose engine, oval body with bronze stripes, curved tail, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "skull" = "sci-fi spaceship SHAPED LIKE A SKULL, top-down view, round ivory skull hull, dark eye sockets with green glow, jaw with teeth, orange fire engine, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "dragonfly" = "sci-fi spaceship SHAPED LIKE A DRAGONFLY, top-down view, long segmented body, four transparent glass wings with veins, round head with glowing green eyes, thin steel tail, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "turtle" = "sci-fi spaceship SHAPED LIKE A TURTLE, top-down view, oval bronze shell with golden segments, head with dark eyes, steel flippers, tail engine with orange fire, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "sword" = "sci-fi spaceship SHAPED LIKE A SWORD, top-down view, long steel blade with light fuller, bronze crossguard, cream grip, golden pommel with dark engine, pointed tip forward, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "umbrella" = "sci-fi spaceship SHAPED LIKE AN UMBRELLA, top-down view, circular canopy of alternating cream and steel segments, golden tip at center, curved bronze handle with dark engine, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "snowflake" = "sci-fi spaceship SHAPED LIKE A SNOWFLAKE, top-down view, FLATTENED wide snowflake, six-fold symmetric steel rays spread HORIZONTALLY, low profile, golden center, glowing glass core, crystalline, wide and low, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "spider" = "sci-fi spaceship SHAPED LIKE A SPIDER, top-down view, cream abdomen, steel cephalothorax, eight curved bronze legs, glowing green eyes, dark engines at rear, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "butterfly" = "sci-fi spaceship SHAPED LIKE A BUTTERFLY, top-down view, two large steel upper wings, two smaller cream lower wings, purple eye spots, dark elongated body, bronze antennae, orange engine at head, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "key" = "sci-fi spaceship SHAPED LIKE A KEY, top-down view, long cream shaft, steel bit with teeth pointing forward, golden ring at rear with dark engine, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "feather" = "sci-fi spaceship SHAPED LIKE A FEATHER, top-down view, ivory vane with bronze shaft down middle, steel section lines, cream quill at rear with dark engine, smooth, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "comet" = "sci-fi spaceship SHAPED LIKE A COMET, top-down view, round steel nucleus with glass cockpit, long tapering cream tail with glow, orange engine at nucleus, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "horseshoe" = "sci-fi spaceship SHAPED LIKE A HORSESHOE, top-down view, bronze horseshoe arc bulging FORWARD to the right, open ends pointing BACK left, steel tips, golden nail studs along arc, glass center patch, dark engines at the back tips, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "clock" = "sci-fi spaceship SHAPED LIKE A CLOCK, top-down view, round golden case, cream clock face with bronze hour marks, steel hour hand pointing forward, steel minute hand, dark center, orange engine at bottom, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
}

$stage2 = "sci-fi spaceship, top-down view, ENTIRE hull covered with dense mechanical texture, detailed panel lines everywhere, greebles, rivets, vents, hatches, armor plates, weathering, scratches, subtle metal gradients across whole body, glowing details, rich color palette, no orange, no flames, neutral engine blocks at rear, maximal detail, masterpiece, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"

$silMap = @{
    "arrow" = "sil_arrow_modules.png"; "cruiser" = "sil_cruiser_modules.png"
    "hawk" = "sil_hawk_modules.png"; "freighter" = "sil_freighter_modules.png"
    "protoss" = "sil_protoss.png"; "zerg" = "sil_zerg.png"
    "home" = "sil_home.png"; "flower" = "sil_flower.png"
    "dragon" = "sil_dragon.png"; "ghost" = "sil_ghost.png"
    "cigarette" = "sil_cigarette.png"; "lightbulb" = "sil_lightbulb.png"
    "cat" = "sil_cat.png"; "skull" = "sil_skull.png"
    "dragonfly" = "sil_dragonfly.png"; "turtle" = "sil_turtle.png"
    "sword" = "sil_sword.png"; "umbrella" = "sil_umbrella.png"
    "snowflake" = "sil_snowflake.png"; "spider" = "sil_spider.png"
    "butterfly" = "sil_butterfly.png"; "key" = "sil_key.png"
    "feather" = "sil_feather.png"; "comet" = "sil_comet.png"
    "horseshoe" = "sil_horseshoe.png"; "clock" = "sil_clock.png"
}

$concepts = @($silMap.Keys)
$outDir = "C:\Zorion2\ai_drafts\gen200"
$procDir = "C:\Zorion2\ai_drafts\gen200_clean"
New-Item -ItemType Directory -Path $outDir -Force | Out-Null
New-Item -ItemType Directory -Path $procDir -Force | Out-Null

$py = "C:\ComfyUI\venv\Scripts\python.exe"

for ($i = $StartNum; $i -le $Count; $i++) {
    $concept = $concepts[($i - 1) % $concepts.Count]
    $seed = $StartSeed + $i - 1
    $num = $i
    $silPath = "C:\Zorion2\ai_drafts\silhouettes\$($silMap[$concept])"
    Log "Variant $num/$Count [$concept] seed=$seed"
    $s1 = "$outDir\v${num}_stage1.png"
    $fin = "$outDir\v${num}.png"
    $clean = "$procDir\v${num}.png"

    $r1 = Send-ComfyControlNet -Prompt $stage1[$concept] -SilhouettePath $silPath -Seed $seed -Steps 40 -Cfg 7 -Strength 1.5 -Denoise 0.85 -OutputPath $s1
    if (-not $r1) { Log "Stage1 FAILED v$num"; continue }

    $r2 = Send-ComfyImg2Img -Prompt $stage2 -InputImage $s1 -Seed ($seed + 500) -Steps 40 -Cfg 7.5 -Denoise 0.55 -OutputPath $fin
    if (-not $r2) { Log "Stage2 FAILED v$num"; continue }

    & $py "C:\Zorion2\tools\process_ship.py" $fin $clean | Out-Null
}

Log "=== Done: $Count ships ==="