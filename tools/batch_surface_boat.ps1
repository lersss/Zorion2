# batch_surface_boat.ps1 — пачка лодки прогулки (ЧК6.3).
# ТЗ: docs/gamedesign/art/art_surface_boat.md §4.3 (этап 1 форма), §4.4 (этап 2
# текстура), §4.6 (пачка: один силуэт -> 6-8 кандидатов, разные seed этапа 2).
#
# Один силуэт -> N кандидатов (boat_01..boat_NN), лог/продолжение (-StartNum).
# В негативах НЕТ слов motor/engine/outboard/propeller (решение создателя).
param(
    [int]$StartNum = 1,
    [int]$Count = 8,
    [int]$Stage1Seed = 20260926,
    [int]$Stage2SeedBase = 77001,
    [string]$Only = "",
    [switch]$RedoStage1,
    [double[]]$Denoises = @(0.45, 0.50, 0.55)
)

. "$PSScriptRoot\comfy_api.ps1"

$root = "C:\Zorion2\ai_drafts\surface_boat"
$silDir = "$root\silhouette"; $s1Dir = "$root\stage1"
$s2Dir = "$root\stage2"; $procDir = "$root\sprites"
New-Item -ItemType Directory -Path $s1Dir, $s2Dir, $procDir -Force | Out-Null

if (-not (Test-ComfyReady)) { Write-Error "ComfyUI недоступен на http://127.0.0.1:8188"; return }

$py = "C:\ComfyUI\venv\Scripts\python.exe"
$log = "$root\batch.log"

$sil = "$silDir\boat.png"
if (-not (Test-Path $sil)) {
    Write-Host "Силуэт не найден — генерирую make_boat_silhouette.py"
    & $py "C:\Zorion2\tools\make_boat_silhouette.py"
}

# --- Промпты (§4.3/§4.4), дословно. БЕЗ motor/engine/outboard/propeller в негативе.
$neg1 = "top view, aerial view, front view, three-quarter view, sail, mast, cabin, windshield, multiple boats, ship, yacht, ferry, fishing boat, wooden rowboat, canoe, kayak, water, waves, ocean, splash, beach, dock, mooring, ropes, people, text, watermark"
$p1 = "single small inflatable rescue dinghy, side profile view, one inflated rubber pontoon tube, open shallow interior, pointed bow raised at the RIGHT, flat transom with a compact outboard motor block at the LEFT, flat orthographic side view, no perspective, game asset, 2D sprite, centered, on black background, no text, no watermark"

$neg2 = "top view, aerial view, front view, three-quarter view, ship, yacht, sail, mast, cabin, windshield, water, waves, ocean, splash, reflection, ground shadow, cast shadow, glossy plastic, chrome, metal hull, rust, wood planks, rivets, ropes, people, text, watermark"
$p2 = "ENTIRE small inflatable rescue dinghy of durable orange rubber fabric, smooth inflated pontoon tube, fabric seams and weave, reinforced pale gunwale, matte weathered rubber, subtle soft highlights, dark interior floor, compact outboard motor at the stern, no water, no reflection, no ground shadow, maximal detail, masterpiece"

$model = "juggernaut-xl-v9.safetensors"
$controlnet = "controlnet-canny-sdxl-1.0.safetensors"

# --- Этап 1 (форма): img2img + ControlNet Canny, один на всю пачку (§4.6).
$st1 = "$s1Dir\boat_stage1.png"
if ($RedoStage1 -or -not (Test-Path $st1)) {
    Write-Host "=== Этап 1 (форма) seed=$Stage1Seed ==="
    $r1 = Send-ComfyControlNet -Prompt $p1 -SilhouettePath $sil -NegativePrompt $neg1 `
        -Seed $Stage1Seed -Steps 40 -Cfg 7 -Strength 1.5 -Denoise 0.85 `
        -Model $model -ControlNet $controlnet -OutputPath $st1
    if (-not $r1) { Write-Error "Stage1 FAILED"; return }
    Add-Content $log "$(Get-Date -Format s) stage1 seed=$Stage1Seed -> $st1"
}

$onlyList = @()
if ($Only) { $onlyList = @($Only -split ',' | ForEach-Object { $_.Trim() } | Where-Object { $_ }) }

$sprites = @()
$last = $StartNum + $Count - 1
for ($i = $StartNum; $i -le $last; $i++) {
    $num = "{0:D2}" -f $i
    if ($onlyList.Count -gt 0 -and ($onlyList -notcontains $num)) { continue }
    $seed2 = $Stage2SeedBase + $i
    $dn = $Denoises[($i - 1) % $Denoises.Count]
    $st2 = "$s2Dir\boat_$num.png"
    $fin = "$procDir\boat_$num.png"

    Write-Host "=== $num/$last [boat_$num] stage2 seed=$seed2 denoise=$dn ==="
    $r2 = Send-ComfyImg2Img -Prompt $p2 -InputImage $st1 -NegativePrompt $neg2 `
        -Seed $seed2 -Steps 40 -Cfg 7.5 -Denoise $dn -Model $model -OutputPath $st2
    if (-not $r2) { Add-Content $log "$(Get-Date -Format s) boat_$num FAILED stage2"; continue }

    & $py "C:\Zorion2\tools\process_surface_boat.py" $st2 $fin --frame 36,232,988,792
    Add-Content $log "$(Get-Date -Format s) boat_$num stage2 seed=$seed2 denoise=$dn -> $fin"
    $sprites += [pscustomobject]@{
        name = "boat_$num"; stage1_seed = $Stage1Seed
        stage2_seed = $seed2; stage2_denoise = $dn; file = "sprites/boat_$num.png"
    }
}

Write-Host "=== Done: $($sprites.Count) кандидатов ==="
if ($onlyList.Count -gt 0) { Write-Host "Partial run (-Only): manifest not written"; return }

$manifest = [ordered]@{
    batch = "surface_boat"
    date = "2026-09-26"
    asset = "web/static/sprites/surface/boat/boat.png"
    model = $model; controlnet = $controlnet
    geometry = [ordered]@{
        size = "204x120 (6x логики 34x20)"; water_line = "y=84"; hull_bottom = "y=120"
        axis_x = 102; frame = "R=(36,232)-(988,792)"
    }
    prompts = [ordered]@{ stage1 = $p1; negative1 = $neg1; stage2 = $p2; negative2 = $neg2 }
    params = [ordered]@{
        stage1 = @{ steps = 40; cfg = 7; strength = 1.5; denoise = 0.85; canny_low = 0.2; canny_high = 0.5 }
        stage2 = @{ steps = 40; cfg = 7.5; denoises = $Denoises; sampler = "dpmpp_2m"; scheduler = "karras" }
    }
    sprites = $sprites
}
$json = $manifest | ConvertTo-Json -Depth 8
[System.IO.File]::WriteAllText("$root\manifest.json", $json, (New-Object System.Text.UTF8Encoding($false)))
Write-Host "Manifest: $root\manifest.json"
