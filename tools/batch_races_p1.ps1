# batch_races_p1.ps1 — пилотная пачка 2 АВАТАРОВ РАС: 9 голов (анфас), по одной на семейство F1-F9.
# Форм-фактор: ГОЛОВА ЦЕЛИКОМ (без бюста; у тварей — короткая шея). Реализм.
# Силуэт = форма по основе расы (твари / абстрактные формы), БЕЗ человеческих глаз и кожи.
# Пост: process_ship.py --no-orient (анфас НЕ разворачиваем).
param([int]$Count = 9, [int]$StartSeed = 30000)

. "$PSScriptRoot\comfy_api.ps1"

$stage1 = @{
    "brimstone" = "realistic close-up of an alien sulfur creature HEAD ONLY, FRONT VIEW, massive dark basalt rock head with curved horns, glowing amber vertical slit eyes, amber cracks in stone, heavy jaw, short neck, centered, single creature head, on black background, game avatar, no text, no watermark"
    "magmite" = "realistic close-up of an alien lava creature HEAD ONLY, FRONT VIEW, dark stone head with glowing orange lava veins, short thick horns, vertical lava slit eyes, heavy jaw, short neck, centered, single creature head, on black background, game avatar, no text, no watermark"
    "magnetar" = "realistic close-up of an alien energy being HEAD ONLY, FRONT VIEW, glowing violet energy sphere, bright white core, cyan magnetic field arcs around, energy spots, centered, single entity, on black background, game avatar, no text, no watermark"
    "oceanid" = "realistic close-up of an alien aquatic crustacean HEAD ONLY, FRONT VIEW, turquoise chitin carapace, small insect-like eyes on stalks, bronze gill plates on sides, claw antennae, beak mouth, short neck, centered, single creature head, on black background, game avatar, no text, no watermark"
    "fungoid" = "realistic close-up of an alien methane fungus HEAD ONLY, FRONT VIEW, large mushroom cap with pale spots, fungal stalk, small side caps, spores around, no face, centered, single fungus, on black background, game avatar, no text, no watermark"
    "geode" = "realistic close-up of an alien silicon crystal HEAD ONLY, FRONT VIEW, faceted geode cluster, tall cream crystal with purple facets, glowing glass core, side crystals, no face, centered, single crystal formation, on black background, game avatar, no text, no watermark"
    "nimbus" = "realistic close-up of an alien gas-giant cloud being HEAD ONLY, FRONT VIEW, translucent cloud dome with white puffs, cyan vortex rings core, small tendrils below, no eyes, centered, single entity, on black background, game avatar, no text, no watermark"
    "fumarole" = "realistic close-up of an alien carbon-dioxide organism HEAD ONLY, FRONT VIEW, grey stone column, white fan-like frill petals on sides, dark vent mouth on top, bronze rings, no face, centered, single organism, on black background, game avatar, no text, no watermark"
    "frostwalker" = "realistic close-up of an alien cryo-ammonia creature HEAD ONLY, FRONT VIEW, tall icy stalagmite crystal, translucent pale blue facets, glowing ice core, side ice spikes, no face, centered, single crystal formation, on black background, game avatar, no text, no watermark"
}

$stage2 = "realistic alien creature head close-up, FRONT VIEW, ENTIRE head covered with detailed alien texture, rock crystal scales, pores, cracks, glowing details, subtle gradients, rich palette, maximal detail, masterpiece, game avatar, centered, single creature head, on black background, no text, no watermark"

$silMap = @{
    "brimstone" = "sil_race_brimstone.png"; "magmite" = "sil_race_magmite.png"
    "magnetar" = "sil_race_magnetar.png"; "oceanid" = "sil_race_oceanid.png"
    "fungoid" = "sil_race_fungoid.png"; "geode" = "sil_race_geode.png"
    "nimbus" = "sil_race_nimbus.png"; "fumarole" = "sil_race_fumarole.png"
    "frostwalker" = "sil_race_frostwalker.png"
}

$concepts = @($silMap.Keys)
$outDir = "C:\Zorion2\ai_drafts\races_p1"
$procDir = "C:\Zorion2\ai_drafts\races_p1_processed"
New-Item -ItemType Directory -Path $outDir -Force | Out-Null
New-Item -ItemType Directory -Path $procDir -Force | Out-Null

$py = "C:\ComfyUI\venv\Scripts\python.exe"
$log = "$outDir\log.txt"
Add-Content $log "=== races_p1 v2 start $(Get-Date -Format s) ==="

for ($i = 0; $i -lt $Count; $i++) {
    $concept = $concepts[$i % $concepts.Count]
    $seed = $StartSeed + $i
    $num = $i + 1
    $silPath = "C:\Zorion2\ai_drafts\silhouettes_races\$($silMap[$concept])"
    Write-Host "=== $num/$Count [$concept] seed=$seed ==="
    Add-Content $log "--- $num/$Count [$concept] seed=$seed"
    $s1 = "$outDir\v${num}_stage1.png"
    $fin = "$outDir\v${num}.png"
    $clean = "$procDir\v${num}.png"

    $r1 = Send-ComfyControlNet -Prompt $stage1[$concept] -SilhouettePath $silPath -Seed $seed -Steps 40 -Cfg 7 -Strength 1.5 -Denoise 0.85 -OutputPath $s1
    if (-not $r1) { Add-Content $log "Stage1 failed v$num"; continue }

    $r2 = Send-ComfyImg2Img -Prompt $stage2 -InputImage $s1 -Seed ($seed + 500) -Steps 40 -Cfg 7.5 -Denoise 0.55 -OutputPath $fin
    if (-not $r2) { Add-Content $log "Stage2 failed v$num"; continue }

    & $py "C:\Zorion2\tools\process_ship.py" $fin $clean --no-orient
    Add-Content $log "OK v$num"
}

Add-Content $log "=== races_p1 v2 done $(Get-Date -Format s) ==="
Write-Host "=== Done: $Count race avatar heads (pilot pack 1 v2, F1-F9) ==="