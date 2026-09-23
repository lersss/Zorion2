# batch_belt_pilot.ps1 — пилотная пачка астероидов пояса (направление 2).
# ТЗ: docs/gamedesign/art/art_belt_asteroids.md §3.3 (этап 1 форма, этап 2 текстура),
# §3.6 (постобработка: вырез по цвету, крупнейший компонент, 256x256, --no-orient).
# 3 формы-мастера x 2 варианта текстуры (a — матовая пыльная, b — трещиноватая/сколотая).
# Параметры этапов и промпты — ровно по §3.3.
param(
    [int]$StartSeed = 90001,
    [string]$Stage2SeedBase = "91001",
    [string]$Stage2SeedBaseB = "92001",
    [string[]]$Only = @()
)

. "$PSScriptRoot\comfy_api.ps1"

$neg = "station, vehicle, spaceship, rings, engine, laser, machinery, building, plant, water, snow, ice sheet, text, watermark"

$stage1Core = "single irregular asteroid, rough rocky surface, regolith, craters, fractures, chipped edges, top-down flat view, neutral soft lighting, game asset, 2D sprite, centered, on black background, no text, no watermark"

# Вариант (a) — матовая пыльная: ровнее, светлее.
$stage2a = "ENTIRE asteroid covered with fine dust and regolith, soft matte gray-brown stone, gentle smooth gradients, subtle craters, weathered dusty surface, pale lighter tone, no metal, no machinery, no plants, no fire, maximal detail, masterpiece"
# Вариант (b) — трещиноватая/сколотая: темнее, контрастнее.
$stage2b = "ENTIRE asteroid covered with deep fractures, cracks, chipped edges, broken rugged surface, sharp crevices, dark contrast, heavily cratered stone, no metal, no machinery, no plants, no fire, darker tone, maximal detail, masterpiece"

$silDir  = "C:\Zorion2\ai_drafts\belt_asteroids\silhouettes"
$s1Dir   = "C:\Zorion2\ai_drafts\belt_asteroids\stage1"
$s2Dir   = "C:\Zorion2\ai_drafts\belt_asteroids\stage2"
$procDir = "C:\Zorion2\ai_drafts\belt_asteroids\sprites"
New-Item -ItemType Directory -Path $s1Dir, $s2Dir, $procDir -Force | Out-Null

$py = "C:\ComfyUI\venv\Scripts\python.exe"
$manifest = @()
$bodies = @("rock_01", "rock_02", "rock_03")
if ($Only.Count -gt 0) { $bodies = @($bodies | Where-Object { $Only -contains $_ }) }

for ($i = 0; $i -lt $bodies.Count; $i++) {
    $body = $bodies[$i]
    $idx = [array]::IndexOf(@("rock_01", "rock_02", "rock_03"), $body)
    $seed1 = $StartSeed + $idx
    $seedA = [int]$Stage2SeedBase + $idx
    $seedB = [int]$Stage2SeedBaseB + $idx
    $silSeed = @{ "rock_01" = 20260923; "rock_02" = 20260924; "rock_03" = 20260925 }[$body]
    $sil = "$silDir\$body.png"
    $st1 = "$s1Dir\${body}_stage1.png"

    Write-Host "=== $($i+1)/$($bodies.Count) [$body] stage1 seed=$seed1, A seed=$seedA, B seed=$seedB ==="

    $r1 = Send-ComfyControlNet -Prompt $stage1Core -SilhouettePath $sil -NegativePrompt $neg `
        -Seed $seed1 -Steps 40 -Cfg 7 -Strength 1.5 -Denoise 0.85 -OutputPath $st1
    if (-not $r1) { Write-Host "Stage1 FAILED $body"; continue }

    foreach ($v in @(@{k = "a"; p = $stage2a; s = $seedA; d = 0.50},
                     @{k = "b"; p = $stage2b; s = $seedB; d = 0.55})) {
        $fin = "$s2Dir\${body}$($v.k).png"
        $clean = "$procDir\${body}$($v.k).png"
        $r2 = Send-ComfyImg2Img -Prompt $v.p -InputImage $st1 -NegativePrompt $neg `
            -Seed $v.s -Steps 40 -Cfg 7.5 -Denoise $v.d -OutputPath $fin
        if (-not $r2) { Write-Host "Stage2 FAILED $body$($v.k)"; continue }
        & $py "C:\Zorion2\tools\process_belt_rock.py" $fin $clean --canvas 256
        $manifest += [pscustomobject]@{
            name = "$body$($v.k)"
            silhouette = "$body.png"
            silhouette_seed = $silSeed
            variant = $v.k
            stage1 = @{
                prompt = $stage1Core; negative = $neg; seed = $seed1; steps = 40
                cfg = 7; strength = 1.5; denoise = 0.85
                model = "juggernaut-xl-v9.safetensors"
                controlnet = "controlnet-canny-sdxl-1.0.safetensors"
                canny_low = 0.2; canny_high = 0.5
            }
            stage2 = @{
                prompt = $v.p; negative = $neg; seed = $v.s; steps = 40
                cfg = 7.5; denoise = $v.d
                model = "juggernaut-xl-v9.safetensors"
            }
            post = @{ tool = "process_belt_rock.py"; canvas = 256; pad = 5; orient = "no" }
        }
    }
}

Write-Host "=== Done: pilot belt asteroids ==="
$json = $manifest | ConvertTo-Json -Depth 6
[System.IO.File]::WriteAllText("C:\Zorion2\ai_drafts\belt_asteroids\manifest.json", $json,
    (New-Object System.Text.UTF8Encoding($false)))
