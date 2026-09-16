# batch_races_p2.ps1 — пачка 2 аватаров рас: human (Люди), brimstone_bg (Курильщик с фоном среды),
# fumarole a/b/c/d (4 подхода), geode a/b/c (3 улучшения). Итого 9.
# human/fumarole*/geode* -> process_ship --no-orient (прозрачный фон).
# brimstone_bg -> process_race_bg (фон среды СОХРАНЯЕТСЯ, для «телевизора» связи).
param([int]$Count = 9, [int]$StartSeed = 40000)

. "$PSScriptRoot\comfy_api.ps1"

$stage1 = @{
    "human" = "realistic close-up of a HUMAN HEAD ONLY, FRONT VIEW, neutral human face, short dark hair, brown eyes, subtle skin texture, calm expression, head and short neck, centered, single person, on black background, game avatar, no text, no watermark"
    "brimstone_bg" = "realistic portrait of an alien sulfur creature, FRONT VIEW, massive dark basalt rock head with curved horns, glowing amber vertical slit eyes, amber cracks in stone, heavy jaw, IN FRONT OF a dark sulfur vent field background, volcanic smoke, amber glow from vents, black smoker chimneys, cinematic communication screen, centered, on dark background, game avatar, no text, no watermark"
    "fumarole_a" = "realistic close-up of an alien carbon-dioxide organism HEAD ONLY, FRONT VIEW, tall grey stone column, layered white fan frills on sides, dark vent mouth on top, bronze rings, no face, centered, single organism, on black background, game avatar, no text, no watermark"
    "fumarole_b" = "realistic close-up of an alien carbon-dioxide organism HEAD ONLY, FRONT VIEW, branching grey coral-like stalk, white pore orifices on branches, dark vent pores on trunk, no face, centered, single organism, on black background, game avatar, no text, no watermark"
    "fumarole_c" = "realistic close-up of an alien carbon-dioxide organism HEAD ONLY, FRONT VIEW, volcanic cone with dark vent mouth on top, grey smoke puffs rising, bronze strata layers on cone, no face, centered, single organism, on black background, game avatar, no text, no watermark"
    "fumarole_d" = "realistic close-up of an alien carbon-dioxide organism HEAD ONLY, FRONT VIEW, pipe organ cluster, several vertical grey tubes of different heights, dark openings on top, bronze rings, grey base plate, no face, centered, single organism, on black background, game avatar, no text, no watermark"
    "geode_a" = "realistic close-up of an alien silicon crystal HEAD ONLY, FRONT VIEW, faceted octahedron head, cream crystal with purple side facets, glowing diamond-shaped eyes, glowing core, crystal neck, no skin, centered, single crystal formation, on black background, game avatar, no text, no watermark"
    "geode_b" = "realistic close-up of an alien silicon crystal HEAD ONLY, FRONT VIEW, crystal druse cluster, tall central cream crystal, side crystals, rhombus glowing eyes on central crystal, glowing core, no skin, centered, single crystal formation, on black background, game avatar, no text, no watermark"
    "geode_c" = "realistic close-up of an alien silicon crystal HEAD ONLY, FRONT VIEW, faceted crystal head with warm cream facets, purple side facets, vertical glowing core instead of eyes, crystal spikes on top, crystal neck with small side crystals, no face, centered, single crystal formation, on black background, game avatar, no text, no watermark"
}

$stage2 = "realistic alien creature head close-up, FRONT VIEW, ENTIRE head covered with detailed alien texture, rock crystal scales, pores, cracks, glowing details, subtle gradients, rich palette, maximal detail, masterpiece, game avatar, centered, single creature head, on black background, no text, no watermark"

$silMap = @{
    "human" = "sil_race_human.png"; "brimstone_bg" = "sil_race_brimstone_bg.png"
    "fumarole_a" = "sil_race_fumarole_a.png"; "fumarole_b" = "sil_race_fumarole_b.png"
    "fumarole_c" = "sil_race_fumarole_c.png"; "fumarole_d" = "sil_race_fumarole_d.png"
    "geode_a" = "sil_race_geode_a.png"; "geode_b" = "sil_race_geode_b.png"
    "geode_c" = "sil_race_geode_c.png"
}

$withBg = @("brimstone_bg")  # эти НЕ режем по фону

$concepts = @($silMap.Keys)
$outDir = "C:\Zorion2\ai_drafts\races_p2"
$procDir = "C:\Zorion2\ai_drafts\races_p2_processed"
New-Item -ItemType Directory -Path $outDir -Force | Out-Null
New-Item -ItemType Directory -Path $procDir -Force | Out-Null

$py = "C:\ComfyUI\venv\Scripts\python.exe"
$log = "$outDir\log.txt"
Add-Content $log "=== races_p2 start $(Get-Date -Format s) ==="

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

    if ($withBg -contains $concept) {
        & $py "C:\Zorion2\tools\process_race_bg.py" $fin $clean 512
    } else {
        & $py "C:\Zorion2\tools\process_ship.py" $fin $clean --no-orient
    }
    Add-Content $log "OK v$num"
}

Add-Content $log "=== races_p2 done $(Get-Date -Format s) ==="
Write-Host "=== Done: $Count race avatars (pack 2) ==="