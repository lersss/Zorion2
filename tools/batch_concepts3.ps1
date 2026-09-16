# batch_concepts3.ps1 — 20 НОВЫХ концептов (пачка 3+4).
# star, anchor, crystal, crown, jellyfish, octopus, snail, manta, lightning, heart,
# pyramid, spiral, trident, shield, egg, phoenix, owl, helmet, shell, book.
param([int]$Count = 20, [int]$StartSeed = 8000)

. "$PSScriptRoot\comfy_api.ps1"

$stage1 = @{
    "star" = "sci-fi spaceship SHAPED LIKE A FIVE-POINTED STAR, top-down view, golden star hull with thick rays, glowing glass core, dark engine at one ray, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "anchor" = "sci-fi spaceship SHAPED LIKE AN ANCHOR, top-down view, steel shaft with bronze crossbar, golden ring at top, cream curved flukes pointing forward, fire engines at flukes, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "crystal" = "sci-fi spaceship SHAPED LIKE A CRYSTAL, top-down view, long faceted diamond hull, steel and ivory facets, glass core, sharp pointed nose, dark engine at back, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "crown" = "sci-fi spaceship SHAPED LIKE A CROWN, top-down view, golden crown with five points, jeweled gems, glowing glass and purple stones, dark engine at base, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "jellyfish" = "sci-fi spaceship SHAPED LIKE A JELLYFISH, top-down view, translucent glass dome, glowing purple core, flowing bronze tentacles trailing behind, dark engine at front, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "octopus" = "sci-fi spaceship SHAPED LIKE AN OCTOPUS, top-down view, cream bulbous head with glowing green eyes, eight curved bronze tentacles, orange engine at bottom, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "snail" = "sci-fi spaceship SHAPED LIKE A SNAIL, top-down view, cream slug body with steel antennae pointing forward, spiral bronze shell with golden whorls, dark engine at shell rear, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "manta" = "sci-fi spaceship SHAPED LIKE A MANTA RAY, top-down view, wide cream diamond body, steel swept wings, glowing green eyes at front, long thin bronze tail with fire engine, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "lightning" = "sci-fi spaceship SHAPED LIKE A LIGHTNING BOLT, top-down view, jagged golden zigzag hull, glowing glass center, dark engine at the tail tip, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "heart" = "sci-fi spaceship SHAPED LIKE A HEART, top-down view, cream heart hull with two lobes, glowing glass stripe down center, orange engine at the point, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "pyramid" = "sci-fi spaceship SHAPED LIKE A PYRAMID, top-down view, golden triangle with cream facet lines, glowing glass eye on face, dark engine at base, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "spiral" = "sci-fi spaceship SHAPED LIKE A SPIRAL, top-down view, bronze spiral shell with three whorls, golden center with glass core, dark engine at edge, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "trident" = "sci-fi spaceship SHAPED LIKE A TRIDENT, top-down view, long steel shaft, three cream prongs pointing forward, dark engine at shaft rear, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "shield" = "sci-fi spaceship SHAPED LIKE A SHIELD, top-down view, oval steel shield with bronze rim, golden heraldic diamond with glass core, dark engine at bottom, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "egg" = "sci-fi spaceship SHAPED LIKE AN EGG, top-down view, smooth cream egg hull with bronze spots, glass window at front, dark engine at rear, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "phoenix" = "sci-fi spaceship SHAPED LIKE A PHOENIX, top-down view, golden body with spread wings, cream head with fire beak, glowing green eye, fiery tail feathers, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "owl" = "sci-fi spaceship SHAPED LIKE AN OWL, top-down view, cream owl body with round head, steel ear tufts, large glass eyes with green glow, bronze beak, dark engine at rear, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "helmet" = "sci-fi spaceship SHAPED LIKE A HELMET, top-down view, steel dome helmet with bronze crest, glass visor at front, dark engine at bottom, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "shell" = "sci-fi spaceship SHAPED LIKE A SHELL, top-down view, cream fan shell with bronze ribs spreading forward, golden base at rear, dark engine at base, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
    "book" = "sci-fi spaceship SHAPED LIKE AN OPEN BOOK, top-down view, two bronze covers with cream pages, golden spine, dark engine at spine rear, symmetrical, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"
}

$stage2 = "sci-fi spaceship, top-down view, ENTIRE hull covered with dense mechanical texture, detailed panel lines everywhere, greebles, rivets, vents, hatches, armor plates, weathering, scratches, subtle metal gradients across whole body, glowing details, rich color palette, no orange, no flames, neutral engine blocks at rear, maximal detail, masterpiece, game asset, 2D sprite, centered, single ship, on black background, no text, no watermark"

$silMap = @{
    "star" = "sil_star.png"; "anchor" = "sil_anchor.png"
    "crystal" = "sil_crystal.png"; "crown" = "sil_crown.png"
    "jellyfish" = "sil_jellyfish.png"; "octopus" = "sil_octopus.png"
    "snail" = "sil_snail.png"; "manta" = "sil_manta.png"
    "lightning" = "sil_lightning.png"; "heart" = "sil_heart.png"
    "pyramid" = "sil_pyramid.png"; "spiral" = "sil_spiral.png"
    "trident" = "sil_trident.png"; "shield" = "sil_shield.png"
    "egg" = "sil_egg.png"; "phoenix" = "sil_phoenix.png"
    "owl" = "sil_owl.png"; "helmet" = "sil_helmet.png"
    "shell" = "sil_shell.png"; "book" = "sil_book.png"
}

$concepts = @($silMap.Keys)
$outDir = "C:\Zorion2\ai_drafts\batch_new"
$procDir = "C:\Zorion2\ai_drafts\processed_new"
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

Write-Host "=== Done: $Count new concept ships ==="