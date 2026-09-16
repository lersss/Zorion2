# batch_races_p4.ps1 — пачка 4: ЭКСПЕРИМЕНТ с форм-словами (как корабли).
# 8 новых слов: cross, diamond, bell, clover, wave, prism, knot, gear.
# ВАЖНО: низ «посажен» — шея/воротник уходит за нижний край кадра (не висит в воздухе).
param([int]$StartSeed = 60000)

. "$PSScriptRoot\comfy_api.ps1"

$stage1 = @{
    "cross_beast" = "realistic close-up of an alien silicon crystal creature HEAD ONLY, FRONT VIEW, head SHAPED LIKE A CROSS, tall vertical crystal with wide horizontal crystal crossbar, glowing rhombus eyes on the crossbar, glowing core on top, faceted cream and purple crystal, neck and collar extending down below the frame, anchored, not floating, centered, single creature head, on black background, game avatar, no text, no watermark"
    "diamond_beast" = "realistic close-up of an alien silicon crystal creature HEAD ONLY, FRONT VIEW, head SHAPED LIKE A DIAMOND, faceted rhombus crystal head, glowing vertical slit eyes, dark jaw, neck and collar extending down below the frame, anchored, not floating, centered, single creature head, on black background, game avatar, no text, no watermark"
    "bell_beast" = "realistic close-up of an alien carbon-dioxide creature HEAD ONLY, FRONT VIEW, head SHAPED LIKE A BELL, wide grey stone bell head with bronze rings, pipe-like vent eyes on top, round dark vent mouth below, bell tongue, neck and collar extending down below the frame, anchored, not floating, centered, single creature head, on black background, game avatar, no text, no watermark"
    "clover_beast" = "realistic close-up of an alien methane fungus creature HEAD ONLY, FRONT VIEW, head SHAPED LIKE A CLOVER, three mushroom caps, dark eyes between caps with green glow, pale spots, fungal stalk neck extending down below the frame, anchored, not floating, centered, single creature head, on black background, game avatar, no text, no watermark"
    "wave_beast" = "realistic close-up of an alien aquatic creature HEAD ONLY, FRONT VIEW, head with a WAVE-LIKE crest fin on top, turquoise smooth skin, eyes on stalks, bronze gill arcs, wavy mouth, neck and collar extending down below the frame, anchored, not floating, centered, single creature head, on black background, game avatar, no text, no watermark"
    "prism_beast" = "realistic close-up of an alien silicon crystal creature HEAD ONLY, FRONT VIEW, head SHAPED LIKE A TRIANGULAR PRISM, faceted crystal prism head, vertical glowing slit eyes, glowing core, cream and purple facets, neck and collar extending down below the frame, anchored, not floating, centered, single creature head, on black background, game avatar, no text, no watermark"
    "knot_beast" = "realistic close-up of an alien carbon-dioxide creature HEAD ONLY, FRONT VIEW, head SHAPED LIKE A KNOT, interwoven grey stone tubes, round vent openings at tube ends, amber slit eyes in center, bronze ring, neck and collar extending down below the frame, anchored, not floating, centered, single creature head, on black background, game avatar, no text, no watermark"
    "gear_beast" = "realistic close-up of an alien sulfur creature HEAD ONLY, FRONT VIEW, head SHAPED LIKE A GEAR, dark basalt gear with teeth, round dark holes, glowing amber vertical slit eyes, dark mouth slit, neck and collar extending down below the frame, anchored, not floating, centered, single creature head, on black background, game avatar, no text, no watermark"
}

$stage2 = "realistic alien creature head close-up, FRONT VIEW, ENTIRE head covered with detailed alien texture, rock crystal scales, pores, cracks, glowing details, subtle gradients, rich palette, maximal detail, masterpiece, game avatar, centered, single creature head, neck and collar extending down below the frame, anchored, on black background, no text, no watermark"

$silMap = @{
    "cross_beast" = "sil_race_cross_beast.png"; "diamond_beast" = "sil_race_diamond_beast.png"
    "bell_beast" = "sil_race_bell_beast.png"; "clover_beast" = "sil_race_clover_beast.png"
    "wave_beast" = "sil_race_wave_beast.png"; "prism_beast" = "sil_race_prism_beast.png"
    "knot_beast" = "sil_race_knot_beast.png"; "gear_beast" = "sil_race_gear_beast.png"
}

$concepts = @($silMap.Keys)
$outDir = "C:\Zorion2\ai_drafts\races_p4"
$procDir = "C:\Zorion2\ai_drafts\races_p4_processed"
New-Item -ItemType Directory -Path $outDir -Force | Out-Null
New-Item -ItemType Directory -Path $procDir -Force | Out-Null

$py = "C:\ComfyUI\venv\Scripts\python.exe"
$log = "$outDir\log.txt"
Add-Content $log "=== races_p4 start $(Get-Date -Format s) ==="

for ($i = 0; $i -lt 8; $i++) {
    $concept = $concepts[$i]
    $seed = $StartSeed + $i
    $num = $i + 1
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

Add-Content $log "=== races_p4 done $(Get-Date -Format s) ==="
Write-Host "=== Done: 8 avatars (pack 4: form-words experiment) ==="