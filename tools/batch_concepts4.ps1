# batch_concepts4.ps1 — пачка 5 (10 новых слов): fan, crescent, compass, mushroom, sail, drop, boomerang, ring, bow, mirror.
param([int]$Count = 10, [int]$StartSeed = 9000)

. "$PSScriptRoot\comfy_api.ps1"

$stage1 = @{
    "fan" = "sci-fi spaceship SHAPED LIKE AN OPEN FAN, top-down flat view, cream fan panels with steel ribs spreading FORWARD to the right, bronze handle at the left rear with dark engine, horizontal, perfectly flat, no perspective, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "crescent" = "sci-fi spaceship SHAPED LIKE A CRESCENT MOON, top-down flat view, golden crescent with two horns pointing FORWARD to the right, glowing glass on the back curve, dark engine at the left rear, horizontal, perfectly flat, no perspective, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "compass" = "sci-fi spaceship SHAPED LIKE A COMPASS, top-down flat view, round bronze compass case, cream dial with golden marks, steel needle pointing FORWARD to the right, dark engine at the needle rear, horizontal, perfectly flat, no perspective, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "mushroom" = "sci-fi spaceship SHAPED LIKE A MUSHROOM, top-down flat view, bronze cap with cream spots pointing FORWARD to the right, cream stem at the left rear, glass eye on cap, dark engine at stem, horizontal, perfectly flat, no perspective, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "sail" = "sci-fi spaceship SHAPED LIKE A SAIL, top-down flat view, triangular cream sail with bronze stripes spreading FORWARD to the right, steel mast with bronze hull at the left rear, dark engine at hull, horizontal, perfectly flat, no perspective, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "drop" = "sci-fi spaceship SHAPED LIKE A WATER DROP, top-down flat view, cream teardrop hull, round rear with glass shine, pointed nose FORWARD to the right, golden ring, dark engine at the left rear, horizontal, perfectly flat, no perspective, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "boomerang" = "sci-fi spaceship SHAPED LIKE A BOOMERANG, top-down flat view, bronze curved boomerang with two steel tips pointing FORWARD to the right, glass in the bend, dark engine at the left rear, horizontal, perfectly flat, no perspective, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "ring" = "sci-fi spaceship SHAPED LIKE A RING, top-down flat view, golden ring with bronze setting, glowing glass gem with purple core pointing FORWARD to the right, dark engine at the left rear, horizontal, perfectly flat, no perspective, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "bow" = "sci-fi spaceship SHAPED LIKE A BOW, top-down flat view, steel bow arc with golden tips pointing FORWARD to the right, cream bowstring at the left rear, bronze arrow shaft through center, glass, dark engine, horizontal, perfectly flat, no perspective, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "mirror" = "sci-fi spaceship SHAPED LIKE A HAND MIRROR, top-down flat view, round steel mirror with glass surface, cream shine, golden gem at front, bronze handle at the left rear with dark engine, horizontal, perfectly flat, no perspective, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
}

$stage2 = "sci-fi spaceship, top-down view, ENTIRE hull covered with dense mechanical texture, detailed panel lines everywhere, greebles, rivets, vents, hatches, armor plates, weathering, scratches, subtle metal gradients across whole body, glowing details, rich color palette, no orange, no flames, neutral engine nozzles at left rear, maximal detail, masterpiece, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"

$silMap = @{
    "fan" = "sil_fan.png"; "crescent" = "sil_crescent.png"
    "compass" = "sil_compass.png"; "mushroom" = "sil_mushroom.png"
    "sail" = "sil_sail.png"; "drop" = "sil_drop.png"
    "boomerang" = "sil_boomerang.png"; "ring" = "sil_ring.png"
    "bow" = "sil_bow.png"; "mirror" = "sil_mirror.png"
}

$concepts = @($silMap.Keys)
$outDir = "C:\Zorion2\ai_drafts\batch_p5"
$procDir = "C:\Zorion2\ai_drafts\processed_p5"
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

Write-Host "=== Done: $Count ships (pack 5) ==="