# batch_races_p5_fix.ps1 — перегенерация «висящих» подходов пачки 5: geometry (v5), material (v6).
param([int]$StartSeed = 71000)

. "$PSScriptRoot\comfy_api.ps1"

$outDir = "C:\Zorion2\ai_drafts\races_p5"
$procDir = "C:\Zorion2\ai_drafts\races_p5_processed"
New-Item -ItemType Directory -Path $outDir -Force | Out-Null
New-Item -ItemType Directory -Path $procDir -Force | Out-Null

$py = "C:\ComfyUI\venv\Scripts\python.exe"
$log = "$outDir\log_fix.txt"
Add-Content $log "=== races_p5_fix start $(Get-Date -Format s) ==="

$stage1 = @{
    "geometry_head" = "abstract geometric alien energy entity HEAD ONLY, FRONT VIEW, pure geometric form, neon triangle frame around glowing violet sphere, concentric energy rings, minimal, symmetrical, WIDE BRIGHT LAVENDER COLLAR TORSO extending down to the bottom edge of frame, firmly anchored, centered, single entity, on black background, game avatar, no text, no watermark"
    "material_head" = "alien head made of MOLTEN GLASS and obsidian, FRONT VIEW, translucent glass drop head with gold veins inside, dark empty eye sockets, glowing core, liquid glass drips, WIDE BROWN COLLAR TORSO extending down to the bottom edge of frame, firmly anchored, not floating, centered, single creature head, on black background, game avatar, no text, no watermark"
}

$stage2 = "realistic alien creature head close-up, FRONT VIEW, ENTIRE head covered with detailed alien texture, scales, pores, cracks, glowing details, subtle gradients, rich palette, maximal detail, masterpiece, game avatar, centered, single creature head, WIDE COLLAR TORSO extending down to the bottom edge of frame, firmly anchored, on black background, no text, no watermark"

$silMap = @{
    "geometry_head" = "sil_race_geometry_head.png"
    "material_head" = "sil_race_material_head.png"
}
$params = @{ "geometry_head" = @(1.3, 0.6); "material_head" = @(0.9, 0.3) }
$outNums = @{ "geometry_head" = 5; "material_head" = 6 }

foreach ($concept in @("geometry_head", "material_head")) {
    $seed = $StartSeed + 100 + (if ($concept -eq "material_head") { 1 } else { 0 })
    $num = $outNums[$concept]
    $strength = $params[$concept][0]
    $cnEnd = $params[$concept][1]
    $silPath = "C:\Zorion2\ai_drafts\silhouettes_races\$($silMap[$concept])"
    Write-Host "=== [$concept] -> v$num seed=$seed str=$strength cnEnd=$cnEnd ==="
    Add-Content $log "--- [$concept] v$num seed=$seed"
    $s1 = "$outDir\v${num}_stage1.png"
    $fin = "$outDir\v${num}.png"

    $r1 = Send-ComfyControlNet -Prompt $stage1[$concept] -SilhouettePath $silPath -Seed $seed -Steps 40 -Cfg 7 -Strength $strength -Denoise 0.85 -CNEnd $cnEnd -OutputPath $s1
    if (-not $r1) { Add-Content $log "Stage1 failed v$num"; continue }
    $r2 = Send-ComfyImg2Img -Prompt $stage2 -InputImage $s1 -Seed ($seed + 500) -Steps 40 -Cfg 7.5 -Denoise 0.55 -OutputPath $fin
    if (-not $r2) { Add-Content $log "Stage2 failed v$num"; continue }
    & $py "C:\Zorion2\tools\process_ship.py" $fin "$procDir\v${num}.png" --no-orient
    Add-Content $log "OK v$num"
}

Add-Content $log "=== races_p5_fix done $(Get-Date -Format s) ==="
Write-Host "=== Done: fix v5, v6 ==="